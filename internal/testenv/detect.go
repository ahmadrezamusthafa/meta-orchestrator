// Package testenv gives a task's evidence run the datastores its repositories need — a throwaway
// MySQL, PostgreSQL, SQL Server, Redis or MongoDB in Docker, with fresh credentials and empty
// databases — so the agent can start the app and its test suites without the operator handing it
// database credentials, and never against a shared or local database it does not own.
//
// Detection reads the manifests of every common stack (Ruby, Go, Node, Python, PHP, JVM, .NET,
// Elixir, Rust, Dart), the framework's own database settings (database.yml, Prisma, Django,
// Spring, appsettings.json, the .env template's driver and URL schemes) and compose files, at the
// repository root and in the sub-projects of a monorepo.
package testenv

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kind is a datastore the orchestrator can start.
type Kind string

const (
	MySQL    Kind = "mysql"
	Postgres Kind = "postgres"
	MSSQL    Kind = "mssql"
	Redis    Kind = "redis"
	Mongo    Kind = "mongo"
)

// kindOrder is the order services start and are listed in.
var kindOrder = []Kind{MySQL, Postgres, MSSQL, Redis, Mongo}

// Label is the service's display name.
func (k Kind) Label() string {
	switch k {
	case MySQL:
		return "MySQL"
	case Postgres:
		return "PostgreSQL"
	case MSSQL:
		return "SQL Server"
	case Redis:
		return "Redis"
	case Mongo:
		return "MongoDB"
	}
	return string(k)
}

func (k Kind) relational() bool { return k == MySQL || k == Postgres || k == MSSQL }

// Stack flags change how a variable is written for the framework that reads it.
const (
	StackRuby   = "ruby"   // Rails: mysql2:// URLs, and DATABASE_URL would override every environment
	StackSpring = "spring" // SPRING_DATASOURCE_* and SPRING_DATA_* through relaxed binding
	StackDotnet = "dotnet" // ConnectionStrings__<name>
	StackPrisma = "prisma" // Prisma's sqlserver:// URL form
)

// Needs is what one repository needs to run locally.
type Needs struct {
	Kinds       []Kind          // in kindOrder
	Images      map[Kind]string // image from the repository's compose file, when it names one
	EnvKeys     []string        // variables its env templates (.env.example, …) and schemas declare
	URLKeys     []string        // variables that hold a whole connection URL whatever their name (Prisma's env("…"))
	ConnStrings []string        // .NET connection string names (appsettings*.json)
	Stacks      map[string]bool
	Setup       []string // how this repository creates its schema and seeds, e.g. "bin/rails db:prepare"
	Projects    []string // sub-projects read, relative to the repository root ("." is the root)
}

// Empty reports whether the repository needs no datastore.
func (n Needs) Empty() bool { return len(n.Kinds) == 0 }

// Ruby reports whether the repository is a Ruby app.
func (n Needs) Ruby() bool { return n.Stacks[StackRuby] }

// signal: files (a glob relative to a project dir) and the words in them that mean a datastore.
// Words are matched case-insensitively.
type signal struct {
	glob  string
	kinds map[Kind][]string
}

var (
	pyDrivers = map[Kind][]string{MySQL: {"mysqlclient", "pymysql", "aiomysql", "mysql-connector", "asyncmy"},
		Postgres: {"psycopg", "asyncpg", "pg8000"}, MSSQL: {"pyodbc", "pymssql", "mssql-django"},
		Redis: {"redis", "django-rq"}, Mongo: {"pymongo", "motor", "mongoengine", "beanie"}}
	jvmDrivers = map[Kind][]string{MySQL: {"mysql-connector", "mariadb-java-client", "r2dbc-mysql"},
		Postgres: {"org.postgresql", "r2dbc-postgresql"}, MSSQL: {"mssql-jdbc", "r2dbc-mssql"},
		Redis: {"spring-boot-starter-data-redis", "jedis", "lettuce-core", "redisson"},
		Mongo: {"spring-boot-starter-data-mongodb", "mongodb-driver"}}
	dotnetDrivers = map[Kind][]string{MySQL: {"mysqlconnector", "pomelo.entityframeworkcore.mysql", "mysql.data"},
		Postgres: {"npgsql"}, MSSQL: {"microsoft.entityframeworkcore.sqlserver", "microsoft.data.sqlclient", "system.data.sqlclient"},
		Redis: {"stackexchange.redis", "microsoft.extensions.caching.stackexchangeredis"}, Mongo: {"mongodb.driver"}}
)

var signals = []signal{
	// Ruby
	{"config/database.yml", map[Kind][]string{MySQL: {"mysql2", "trilogy"}, Postgres: {"postgresql", "postgis"}, MSSQL: {"sqlserver"}}},
	{"Gemfile", map[Kind][]string{MySQL: {"'mysql2'", "\"mysql2\"", "'trilogy'", "\"trilogy\""}, Postgres: {"'pg'", "\"pg\""},
		MSSQL: {"activerecord-sqlserver-adapter"}, Redis: {"'redis'", "\"redis\"", "'sidekiq'", "\"sidekiq\"", "'resque'"},
		Mongo: {"'mongoid'", "\"mongoid\""}}},
	// Go
	{"go.mod", map[Kind][]string{MySQL: {"go-sql-driver/mysql", "gorm.io/driver/mysql"},
		Postgres: {"github.com/lib/pq", "github.com/jackc/pgx", "gorm.io/driver/postgres", "pgdialect"},
		MSSQL:    {"go-mssqldb", "gorm.io/driver/sqlserver"},
		Redis:    {"go-redis/redis", "redis/go-redis", "gomodule/redigo", "redis/rueidis", "hibiken/asynq"}, Mongo: {"go.mongodb.org/mongo-driver"}}},
	// Node (dependency names only, see nodeDeps)
	{"package.json", map[Kind][]string{MySQL: {"\"mysql\"", "\"mysql2\"", "\"mariadb\""}, Postgres: {"\"pg\"", "\"postgres\"", "\"pg-promise\""},
		MSSQL: {"\"mssql\"", "\"tedious\""}, Redis: {"\"redis\"", "\"ioredis\"", "\"bullmq\"", "\"bull\"", "\"@nestjs/bull\""},
		Mongo: {"\"mongodb\"", "\"mongoose\"", "\"@nestjs/mongoose\""}}},
	{"prisma/schema.prisma", prismaKinds},
	{"schema.prisma", prismaKinds},
	// Python
	{"requirements*.txt", pyDrivers},
	{"requirements/*.txt", pyDrivers},
	{"pyproject.toml", pyDrivers},
	{"Pipfile", pyDrivers},
	{"setup.py", pyDrivers},
	{"*/settings.py", djangoEngines},
	{"*/settings/*.py", djangoEngines},
	// PHP: Laravel and Symfony decide from the .env template (see templateKinds)
	{"composer.json", map[Kind][]string{Redis: {"predis/predis", "phpredis"}, Mongo: {"mongodb/mongodb", "mongodb/laravel-mongodb", "jenssegers/mongodb"}}},
	// JVM
	{"pom.xml", jvmDrivers},
	{"build.gradle", jvmDrivers},
	{"build.gradle.kts", jvmDrivers},
	{"gradle/libs.versions.toml", jvmDrivers},
	{"src/main/resources/application*.properties", springProps},
	{"src/main/resources/application*.y*ml", springProps},
	// .NET
	{"*.csproj", dotnetDrivers},
	{"*.fsproj", dotnetDrivers},
	{"*/*.csproj", dotnetDrivers},
	// Elixir
	{"mix.exs", map[Kind][]string{MySQL: {":myxql"}, Postgres: {":postgrex"}, MSSQL: {":tds"}, Redis: {":redix"}, Mongo: {":mongodb_driver"}}},
	// Rust
	{"Cargo.toml", map[Kind][]string{MySQL: {"mysql"}, Postgres: {"postgres"}, MSSQL: {"tiberius", "mssql"}, Redis: {"redis"}, Mongo: {"mongodb"}}},
	// Dart (server)
	{"pubspec.yaml", map[Kind][]string{MySQL: {"mysql1:", "mysql_client:"}, Postgres: {"\n  postgres:"}, Redis: {"\n  redis:"}, Mongo: {"mongo_dart:"}}},
}

var (
	prismaKinds   = map[Kind][]string{MySQL: {`"mysql"`}, Postgres: {`"postgresql"`, `"postgres"`, `"cockroachdb"`}, MSSQL: {`"sqlserver"`}, Mongo: {`"mongodb"`}}
	djangoEngines = map[Kind][]string{MySQL: {"django.db.backends.mysql"}, Postgres: {"django.db.backends.postgresql", "django.contrib.gis.db.backends.postgis"},
		MSSQL: {"mssql"}, Redis: {"django_redis", "redis://"}}
	springProps = map[Kind][]string{MySQL: {"jdbc:mysql", "jdbc:mariadb"}, Postgres: {"jdbc:postgresql"}, MSSQL: {"jdbc:sqlserver"},
		Redis: {"spring.data.redis", "spring.redis", "redis:"}, Mongo: {"mongodb://", "spring.data.mongodb"}}
)

// manifests mark a project directory: the root, or a sub-project of a monorepo.
var manifests = []string{"Gemfile", "go.mod", "package.json", "requirements.txt", "pyproject.toml", "Pipfile", "setup.py",
	"manage.py", "composer.json", "pom.xml", "build.gradle", "build.gradle.kts", "mix.exs", "Cargo.toml", "pubspec.yaml",
	"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml", "*.csproj", "*.fsproj", "*.sln"}

// skipDirs are never descended into: dependencies, build output and tool caches.
var skipDirs = map[string]bool{"node_modules": true, "vendor": true, "dist": true, "build": true, "out": true, "bin": true, "obj": true,
	"target": true, "tmp": true, "log": true, "coverage": true, "__pycache__": true, "venv": true, "deps": true, "_build": true,
	"public": true, "assets": true, "docs": true, "Pods": true}

const (
	maxDepth    = 2  // sub-project levels below the root (apps/api, services/billing)
	maxProjects = 40 // sub-projects read per repository
)

var composeFiles = []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml",
	"docker-compose.dev.yml", "docker-compose.local.yml", "docker-compose.test.yml"}

// envTemplates are the files that declare a project's environment variables.
var envTemplates = []string{".env.template", ".env.example", ".env.sample", ".env.dist", ".env.defaults", ".env.test.example",
	"env.example", "example.env", ".env.development.example", ".env.local.example", ".env.test", ".env.testing", ".env"}

// committedEnv are env files whose values may be real: only their names are read.
var committedEnv = map[string]bool{".env": true, ".env.test": true, ".env.testing": true}

// Detect reads what the repository at dir needs, from its root and its sub-projects.
func Detect(dir string) Needs {
	n := Needs{Images: map[Kind]string{}, Stacks: map[string]bool{}}
	found := map[Kind]bool{}
	keys := map[string]bool{}
	for _, rel := range projectDirs(dir) {
		p := filepath.Join(dir, rel)
		n.Projects = append(n.Projects, filepath.ToSlash(rel))
		for k := range detectProject(p) {
			found[k] = true
		}
		for _, f := range composeFiles {
			for k, img := range composeImages(filepath.Join(p, f)) {
				found[k] = true
				if n.Images[k] == "" {
					n.Images[k] = img
				}
			}
		}
		for k := range templateKinds(p) {
			found[k] = true
		}
		for _, key := range envKeys(p) {
			if !keys[key] {
				keys[key] = true
				n.EnvKeys = append(n.EnvKeys, key)
			}
		}
		n.URLKeys = appendNew(n.URLKeys, prismaURLKeys(p)...)
		n.ConnStrings = appendNew(n.ConnStrings, connectionStringNames(p)...)
		for s := range stacks(p) {
			n.Stacks[s] = true
		}
		n.Setup = appendNew(n.Setup, setupHints(p, rel)...)
	}
	for _, k := range kindOrder {
		if found[k] {
			n.Kinds = append(n.Kinds, k)
		}
	}
	return n
}

// projectDirs lists the root and the folders up to maxDepth below it that hold a manifest.
func projectDirs(root string) []string {
	out := []string{"."}
	queue := []string{"."}
	for depth := 0; depth < maxDepth && len(queue) > 0; depth++ {
		var next []string
		for _, rel := range queue {
			entries, err := os.ReadDir(filepath.Join(root, rel))
			if err != nil {
				continue
			}
			for _, e := range entries {
				name := e.Name()
				if !e.IsDir() || skipDirs[name] || strings.HasPrefix(name, ".") {
					continue
				}
				sub := filepath.Join(rel, name)
				if hasManifest(filepath.Join(root, sub)) && len(out) < maxProjects {
					out = append(out, sub)
				}
				next = append(next, sub)
			}
		}
		queue = next
	}
	return out
}

func hasManifest(dir string) bool {
	for _, m := range manifests {
		if matches, _ := filepath.Glob(filepath.Join(dir, m)); len(matches) > 0 {
			return true
		}
	}
	return false
}

// detectProject matches the signals of one project directory.
func detectProject(dir string) map[Kind]bool {
	found := map[Kind]bool{}
	for _, s := range signals {
		paths, _ := filepath.Glob(filepath.Join(dir, filepath.FromSlash(s.glob)))
		for _, p := range paths {
			raw, err := readSmall(p)
			if err != nil {
				continue
			}
			text := strings.ToLower(raw)
			if filepath.Base(p) == "package.json" {
				text = nodeDeps([]byte(raw))
			}
			for k, words := range s.kinds {
				for _, w := range words {
					if strings.Contains(text, strings.ToLower(w)) {
						found[k] = true
						break
					}
				}
			}
		}
	}
	return found
}

// readSmall reads a file up to 512 KiB, so a vendored blob never slows detection.
func readSmall(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, 512<<10)
	n, _ := f.Read(buf)
	return string(buf[:n]), nil
}

// nodeDeps is the dependency names of a package.json, so a script or a description that
// mentions "redis" does not count.
func nodeDeps(raw []byte) string {
	var pkg struct {
		Deps    map[string]interface{} `json:"dependencies"`
		Dev     map[string]interface{} `json:"devDependencies"`
		Opt     map[string]interface{} `json:"optionalDependencies"`
		Scripts map[string]string      `json:"scripts"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return ""
	}
	var b strings.Builder
	for _, deps := range []map[string]interface{}{pkg.Deps, pkg.Dev, pkg.Opt} {
		for name := range deps {
			b.WriteString("\"" + strings.ToLower(name) + "\"\n")
		}
	}
	return b.String()
}

// composeImages maps the datastores a compose file runs to their images.
func composeImages(path string) map[Kind]string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if yaml.Unmarshal(raw, &doc) != nil {
		return nil
	}
	names := make([]string, 0, len(doc.Services))
	for name := range doc.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	out := map[Kind]string{}
	for _, name := range names {
		img := doc.Services[name].Image
		if k, ok := imageKind(img); ok && out[k] == "" && !strings.Contains(img, "${") {
			out[k] = img
		}
	}
	return out
}

// imageKind recognises the official datastore images (mysql:8.0, bitnami/postgresql, redis:7-alpine,
// mcr.microsoft.com/mssql/server:2022-latest).
func imageKind(image string) (Kind, bool) {
	repo := strings.ToLower(image)
	if i := strings.LastIndex(repo, ":"); i > strings.LastIndex(repo, "/") {
		repo = repo[:i]
	}
	if strings.HasSuffix(repo, "mssql/server") || strings.Contains(repo, "azure-sql-edge") {
		return MSSQL, true
	}
	repo = repo[strings.LastIndex(repo, "/")+1:]
	switch repo {
	case "mysql", "mariadb", "percona", "percona-server":
		return MySQL, true
	case "postgres", "postgresql", "postgis", "timescaledb":
		return Postgres, true
	case "redis", "redis-stack", "redis-stack-server", "valkey", "keydb":
		return Redis, true
	case "mongo", "mongodb", "mongodb-community-server":
		return Mongo, true
	}
	return "", false
}

var envLineRe = regexp.MustCompile(`^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$`)

// envLines calls fn with each variable an env file declares. value is "" for a committed env
// file, whose values are never read.
func envLines(dir string, fn func(name, key, value string)) {
	for _, name := range envTemplates {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if m := envLineRe.FindStringSubmatch(sc.Text()); m != nil {
				v := ""
				if !committedEnv[name] {
					v = strings.Trim(strings.TrimSpace(m[2]), `"'`)
				}
				fn(name, m[1], v)
			}
		}
		f.Close()
	}
}

// envKeys lists the variable names the project's env files declare.
func envKeys(dir string) []string {
	var out []string
	seen := map[string]bool{}
	envLines(dir, func(_, key, _ string) {
		if !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	})
	return out
}

// templateKinds reads the engine an env template chooses: Laravel's DB_CONNECTION=pgsql, a
// TypeORM DB_TYPE, or the scheme of DATABASE_URL (Symfony, Prisma, dj-database-url). Only the
// scheme or the driver word is looked at, never a credential.
func templateKinds(dir string) map[Kind]bool {
	out := map[Kind]bool{}
	envLines(dir, func(_, key, value string) {
		if value == "" {
			return
		}
		t := tokensOf(key)
		last := strings.ToUpper(key[strings.LastIndex(key, "_")+1:])
		v := strings.ToLower(value)
		switch {
		case (t["DB"] || t["DATABASE"]) && (last == "CONNECTION" || last == "DRIVER" || last == "DIALECT" || last == "ADAPTER" ||
			last == "TYPE" || last == "ENGINE" || last == "CLIENT"):
			if k, ok := engineKind(v); ok {
				out[k] = true
			}
		case last == "URL" || last == "URI" || last == "DSN":
			if i := strings.Index(v, "://"); i > 0 {
				if k, ok := engineKind(v[:i]); ok {
					out[k] = true
				}
			}
		}
	})
	return out
}

func engineKind(v string) (Kind, bool) {
	switch {
	case strings.Contains(v, "mysql") || strings.Contains(v, "mariadb"):
		return MySQL, true
	case strings.Contains(v, "postgres") || v == "pgsql" || v == "pg" || strings.Contains(v, "postgis"):
		return Postgres, true
	case strings.Contains(v, "sqlsrv") || strings.Contains(v, "mssql") || strings.Contains(v, "sqlserver"):
		return MSSQL, true
	case strings.HasPrefix(v, "redis") || v == "rediss":
		return Redis, true
	case strings.HasPrefix(v, "mongodb"):
		return Mongo, true
	}
	return "", false
}

var prismaEnvRe = regexp.MustCompile(`(?:url|directUrl|shadowDatabaseUrl)\s*=\s*env\(\s*"([A-Za-z_][A-Za-z0-9_]*)"\s*\)`)

// prismaURLKeys are the variables a Prisma schema reads its connection URL from.
func prismaURLKeys(dir string) []string {
	var out []string
	for _, rel := range []string{"prisma/schema.prisma", "schema.prisma"} {
		raw, err := readSmall(filepath.Join(dir, rel))
		if err != nil {
			continue
		}
		for _, m := range prismaEnvRe.FindAllStringSubmatch(raw, -1) {
			if !strings.Contains(strings.ToLower(m[0]), "shadow") {
				out = appendNew(out, m[1])
			}
		}
	}
	return out
}

// connectionStringNames are the ConnectionStrings a .NET project's appsettings declare.
func connectionStringNames(dir string) []string {
	var out []string
	paths, _ := filepath.Glob(filepath.Join(dir, "appsettings*.json"))
	more, _ := filepath.Glob(filepath.Join(dir, "*", "appsettings*.json"))
	for _, p := range append(paths, more...) {
		raw, err := readSmall(p)
		if err != nil {
			continue
		}
		var doc struct {
			ConnectionStrings map[string]interface{} `json:"ConnectionStrings"`
		}
		if json.Unmarshal([]byte(stripJSONComments(raw)), &doc) != nil {
			continue
		}
		for name := range doc.ConnectionStrings {
			out = appendNew(out, name)
		}
	}
	sort.Strings(out)
	return out
}

var jsonCommentRe = regexp.MustCompile(`(?m)^\s*//.*$`)

func stripJSONComments(s string) string { return jsonCommentRe.ReplaceAllString(s, "") }

// stacks are the frameworks whose variables need a form of their own.
func stacks(dir string) map[string]bool {
	out := map[string]bool{}
	if exists(dir, "Gemfile") {
		out[StackRuby] = true
	}
	for _, f := range []string{"pom.xml", "build.gradle", "build.gradle.kts"} {
		if raw, err := readSmall(filepath.Join(dir, f)); err == nil && strings.Contains(raw, "spring-boot") {
			out[StackSpring] = true
		}
	}
	if exists(dir, "*.csproj") || exists(dir, "*.fsproj") || exists(dir, "*/*.csproj") || exists(dir, "*.sln") {
		out[StackDotnet] = true
	}
	if exists(dir, "prisma/schema.prisma") || exists(dir, "schema.prisma") {
		out[StackPrisma] = true
	}
	return out
}

func exists(dir, glob string) bool {
	m, _ := filepath.Glob(filepath.Join(dir, filepath.FromSlash(glob)))
	return len(m) > 0
}

func contains(dir, glob string, words ...string) bool {
	paths, _ := filepath.Glob(filepath.Join(dir, filepath.FromSlash(glob)))
	for _, p := range paths {
		raw, err := readSmall(p)
		if err != nil {
			continue
		}
		for _, w := range words {
			if strings.Contains(raw, w) {
				return true
			}
		}
	}
	return false
}

func appendNew(list []string, vs ...string) []string {
	for _, v := range vs {
		dup := false
		for _, have := range list {
			dup = dup || have == v
		}
		if !dup {
			list = append(list, v)
		}
	}
	return list
}

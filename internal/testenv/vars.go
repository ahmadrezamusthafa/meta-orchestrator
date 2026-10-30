package testenv

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Service is a running datastore of the task's test environment.
type Service struct {
	Kind      Kind   `json:"kind"`
	Image     string `json:"image"`
	Container string `json:"container"`
	Port      int    `json:"port"` // on 127.0.0.1
	User      string `json:"user,omitempty"`
	password  string
}

const host = "127.0.0.1"

// dbNames are the two databases each repository gets in a relational service.
var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

func dbNames(repo string) (dev, test string) {
	base := strings.Trim(nonWord.ReplaceAllString(strings.ToLower(repo), "_"), "_")
	if base == "" {
		base = "app"
	}
	return base + "_development", base + "_test"
}

// url is the service's address in the form most clients take; a Ruby app's MySQL URL names the
// mysql2 adapter, as Rails expects, and Prisma writes SQL Server its own way.
func (s Service) url(db string, n Needs) string {
	u := url.URL{Host: fmt.Sprintf("%s:%d", host, s.Port), Path: "/" + db}
	switch s.Kind {
	case MySQL:
		u.Scheme, u.User = "mysql", url.UserPassword(s.User, s.password)
		if n.Ruby() {
			u.Scheme = "mysql2"
		}
	case Postgres:
		u.Scheme, u.User, u.RawQuery = "postgres", url.UserPassword(s.User, s.password), "sslmode=disable"
		if n.Stacks[StackPrisma] {
			u.Scheme = "postgresql"
		}
	case MSSQL:
		if n.Stacks[StackPrisma] {
			return fmt.Sprintf("sqlserver://%s:%d;database=%s;user=%s;password=%s;trustServerCertificate=true", host, s.Port, db, s.User, s.password)
		}
		u.Scheme, u.User, u.Path = "sqlserver", url.UserPassword(s.User, s.password), ""
		u.RawQuery = url.Values{"database": {db}, "encrypt": {"disable"}}.Encode()
	case Redis:
		u.Scheme, u.Path = "redis", "/0"
	case Mongo:
		u.Scheme = "mongodb"
	}
	return u.String()
}

// dsn is the Go driver's form of a MySQL address; other kinds use their URL.
func (s Service) dsn(db string, n Needs) string {
	if s.Kind == MySQL {
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&multiStatements=true", s.User, s.password, host, s.Port, db)
	}
	return s.url(db, n)
}

// jdbc is the JDBC URL of a relational service (Spring and other JVM apps).
func (s Service) jdbc(db string) string {
	switch s.Kind {
	case MySQL:
		return fmt.Sprintf("jdbc:mysql://%s:%d/%s?allowPublicKeyRetrieval=true&useSSL=false", host, s.Port, db)
	case Postgres:
		return fmt.Sprintf("jdbc:postgresql://%s:%d/%s", host, s.Port, db)
	case MSSQL:
		return fmt.Sprintf("jdbc:sqlserver://%s:%d;databaseName=%s;encrypt=false", host, s.Port, db)
	}
	return ""
}

// ado is the ADO.NET connection string (.NET's ConnectionStrings).
func (s Service) ado(db string) string {
	switch s.Kind {
	case MySQL:
		return fmt.Sprintf("Server=%s;Port=%d;Database=%s;User ID=%s;Password=%s", host, s.Port, db, s.User, s.password)
	case Postgres:
		return fmt.Sprintf("Host=%s;Port=%d;Database=%s;Username=%s;Password=%s", host, s.Port, db, s.User, s.password)
	case MSSQL:
		return fmt.Sprintf("Server=%s,%d;Database=%s;User Id=%s;Password=%s;TrustServerCertificate=True", host, s.Port, db, s.User, s.password)
	case Redis:
		return fmt.Sprintf("%s:%d", host, s.Port)
	}
	return s.url(db, Needs{})
}

// varRole is what a variable of an env template holds.
type varRole int

const (
	roleNone varRole = iota
	roleHost
	rolePort
	roleUser
	rolePassword
	roleName
	roleURL
	roleDSN
)

func tokensOf(key string) map[string]bool {
	out := map[string]bool{}
	for _, t := range strings.Split(strings.ToUpper(key), "_") {
		if t != "" {
			out[t] = true
		}
	}
	return out
}

// classify says which service a template variable belongs to and what it holds. A relational
// variable names its engine (MYSQL_HOST, PG_PORT) or is generic (DB_HOST, DATABASE_URL), which
// then means the repository's own relational database.
func classify(key string, primary Kind) (Kind, varRole, bool) {
	t := tokensOf(key)
	last := strings.ToUpper(key[strings.LastIndex(key, "_")+1:])
	var kind Kind
	switch {
	case t["REDIS"] || t["VALKEY"]:
		kind = Redis
	case t["MONGO"] || t["MONGODB"]:
		kind = Mongo
	case t["MYSQL"] || t["MARIADB"]:
		kind = MySQL
	case t["PG"] || t["POSTGRES"] || t["POSTGRESQL"]:
		kind = Postgres
	case t["MSSQL"] || t["SQLSERVER"]:
		kind = MSSQL
	case t["DB"] || t["DATABASE"]:
		kind = primary
	}
	if kind == "" {
		return "", roleNone, false
	}
	role := roleNone
	switch last {
	case "HOST", "HOSTNAME", "ADDR", "SERVER":
		role = roleHost
	case "PORT":
		role = rolePort
	case "USER", "USERNAME":
		role = roleUser
	case "PASS", "PASSWORD", "PWD":
		role = rolePassword
	case "NAME", "DATABASE", "DBNAME", "SCHEMA":
		role = roleName
	case "DB":
		if len(t) > 1 {
			role = roleName // MYSQL_DB, MONGO_DB
		}
	case "URL", "URI":
		role = roleURL
	case "STRING", "CONNECTION", "CONNSTR":
		if t["CONNECTION"] && last != "CONNECTION" || last == "CONNSTR" {
			role = roleURL // DB_CONNECTION_STRING; DB_CONNECTION alone is Laravel's driver name
		}
	case "DSN":
		role = roleDSN
	}
	if kind == Redis && role == roleName {
		return "", roleNone, false // REDIS_DB is a number
	}
	return kind, role, role != roleNone
}

// RepoEnv is the environment one repository runs with: its template variables pointed at the
// test environment, plus the variables the database clients read on their own.
type RepoEnv struct {
	Name  string            `json:"name"`
	Dirs  []string          `json:"dirs"` // the repository's checkouts: a command run under one gets Vars
	Vars  map[string]string `json:"vars"`
	DBs   []string          `json:"databases,omitempty"`
	Setup []string          `json:"setup,omitempty"` // how it creates its schema and seeds (see setupHints)
}

// repoVars builds a repository's variables from its needs and the running services.
func repoVars(name string, n Needs, svc map[Kind]*Service) RepoEnv {
	env := RepoEnv{Name: name, Vars: map[string]string{}, Setup: n.Setup}
	dev, test := dbNames(name)
	var primary Kind
	for _, k := range n.Kinds {
		if k.relational() && svc[k] != nil {
			primary = k
			env.DBs = []string{dev, test}
			break
		}
	}
	db := func(k Kind, key string) string {
		switch {
		case k == Redis:
			return ""
		case tokensOf(key)["TEST"]:
			return test
		}
		return dev
	}
	for _, key := range n.EnvKeys {
		k, role, ok := classify(key, primary)
		s := svc[k]
		if !ok || s == nil {
			continue
		}
		switch role {
		case roleHost:
			env.Vars[key] = host
		case rolePort:
			env.Vars[key] = fmt.Sprint(s.Port)
		case roleUser:
			if s.User != "" {
				env.Vars[key] = s.User
			}
		case rolePassword:
			env.Vars[key] = s.password
		case roleName:
			env.Vars[key] = db(k, key)
		case roleURL:
			env.Vars[key] = s.url(db(k, key), n)
		case roleDSN:
			env.Vars[key] = s.dsn(db(k, key), n)
		}
	}
	rel := svc[primary]
	for _, key := range n.URLKeys { // Prisma's env("…"), whatever its name
		if rel != nil {
			env.Vars[key] = rel.url(db(primary, key), n)
		}
	}
	// Clients read these themselves, so a config file without a port or password still reaches
	// the service: libmysqlclient (mysql2, the mysql CLI) and libpq (pg, psql, most Postgres drivers).
	if s := svc[MySQL]; s != nil && primary == MySQL {
		setDefault(env.Vars, "MYSQL_HOST", host)
		setDefault(env.Vars, "MYSQL_TCP_PORT", fmt.Sprint(s.Port))
		setDefault(env.Vars, "MYSQL_PWD", s.password)
	}
	if s := svc[Postgres]; s != nil && primary == Postgres {
		for k, v := range map[string]string{"PGHOST": host, "PGPORT": fmt.Sprint(s.Port), "PGUSER": s.User,
			"PGPASSWORD": s.password, "PGDATABASE": dev} {
			setDefault(env.Vars, k, v)
		}
	}
	// DATABASE_URL is the convention of most stacks (Prisma, sqlx, diesel, dj-database-url, Ecto,
	// Symfony, Laravel's DB_URL, 12-factor Go and Node apps). Rails is left out: there it would
	// override the database of every environment, the test one included.
	if rel != nil && !n.Ruby() {
		setDefault(env.Vars, "DATABASE_URL", rel.url(dev, n))
		setDefault(env.Vars, "TEST_DATABASE_URL", rel.url(test, n))
	}
	for _, k := range n.Kinds {
		if s := svc[k]; s != nil && !k.relational() {
			setDefault(env.Vars, strings.ToUpper(string(k))+"_URL", s.url(map[Kind]string{Mongo: dev}[k], n))
		}
	}
	if n.Stacks[StackSpring] {
		springVars(env.Vars, n, svc, rel, dev)
	}
	if n.Stacks[StackDotnet] {
		for _, name := range n.ConnStrings {
			if s := connStringService(name, n, svc, rel); s != nil {
				d := dev
				if tokensOf(strings.ToUpper(name))["TEST"] || strings.Contains(strings.ToLower(name), "test") {
					d = test
				}
				setDefault(env.Vars, "ConnectionStrings__"+name, s.ado(d))
			}
		}
	}
	return env
}

// springVars point a Spring Boot app at the services through relaxed binding, which outranks
// application.yml: SPRING_DATASOURCE_URL is spring.datasource.url.
func springVars(vars map[string]string, n Needs, svc map[Kind]*Service, rel *Service, dev string) {
	if rel != nil {
		setDefault(vars, "SPRING_DATASOURCE_URL", rel.jdbc(dev))
		setDefault(vars, "SPRING_DATASOURCE_USERNAME", rel.User)
		setDefault(vars, "SPRING_DATASOURCE_PASSWORD", rel.password)
	}
	if s := svc[Redis]; s != nil {
		for _, p := range []string{"SPRING_DATA_REDIS", "SPRING_REDIS"} { // Boot 3, Boot 2
			setDefault(vars, p+"_HOST", host)
			setDefault(vars, p+"_PORT", fmt.Sprint(s.Port))
		}
	}
	if s := svc[Mongo]; s != nil {
		setDefault(vars, "SPRING_DATA_MONGODB_URI", s.url(dev, n))
	}
}

// connStringService picks the service a .NET connection string names: Redis or Mongo by its
// name, else the relational database.
func connStringService(name string, n Needs, svc map[Kind]*Service, rel *Service) *Service {
	l := strings.ToLower(name)
	switch {
	case strings.Contains(l, "redis") || strings.Contains(l, "cache"):
		return svc[Redis]
	case strings.Contains(l, "mongo"):
		return svc[Mongo]
	}
	return rel
}

func setDefault(m map[string]string, k, v string) {
	if _, ok := m[k]; !ok {
		m[k] = v
	}
}

package testenv

import (
	"strings"
	"testing"
)

// stackCase is a minimal project of one stack, what it must be found to need, and what its
// variables and setup must say.
type stackCase struct {
	name  string
	files map[string]string
	kinds []Kind
	vars  map[string]string // exact values, or "~sub" for a value that contains sub
	none  []string          // variables that must not be set
	setup string            // a setup hint that must be offered
}

var stackCases = []stackCase{
	{name: "spring boot maven on mysql and redis",
		files: map[string]string{
			"pom.xml": "<parent><artifactId>spring-boot-starter-parent</artifactId></parent><dependency><artifactId>mysql-connector-j</artifactId></dependency>" +
				"<dependency><artifactId>spring-boot-starter-data-redis</artifactId></dependency><dependency><artifactId>flyway-core</artifactId></dependency>",
			"src/main/resources/application.yml": "spring:\n  datasource:\n    url: jdbc:mysql://localhost:3306/shop\n"},
		kinds: []Kind{MySQL, Redis},
		vars: map[string]string{"SPRING_DATASOURCE_URL": "~jdbc:mysql://127.0.0.1:3306/svc_development", "SPRING_DATASOURCE_USERNAME": "root",
			"SPRING_DATA_REDIS_PORT": "6379", "SPRING_REDIS_HOST": "127.0.0.1"},
		setup: "Flyway/Liquibase"},
	{name: "gradle kotlin on postgres",
		files: map[string]string{"build.gradle.kts": `plugins { id("org.springframework.boot") }` + "\n" + `dependencies { runtimeOnly("org.postgresql:postgresql") }` + "\n// spring-boot"},
		kinds: []Kind{Postgres}, vars: map[string]string{"SPRING_DATASOURCE_URL": "~jdbc:postgresql://", "PGPORT": "5432"}},
	{name: ".net on postgres with a redis cache",
		files: map[string]string{
			"Shop.sln":                 "",
			"src/Api/Api.csproj":       `<PackageReference Include="Npgsql.EntityFrameworkCore.PostgreSQL" /><PackageReference Include="Microsoft.EntityFrameworkCore.Design" /><PackageReference Include="StackExchange.Redis" />`,
			"src/Api/appsettings.json": "{\n  // local\n  \"ConnectionStrings\": {\"Default\": \"Host=localhost\", \"RedisCache\": \"localhost\"}\n}"},
		kinds: []Kind{Postgres, Redis},
		vars:  map[string]string{"ConnectionStrings__Default": "~Host=127.0.0.1;Port=5432;Database=svc_development;Username=postgres", "ConnectionStrings__RedisCache": "127.0.0.1:6379"},
		setup: "in src/Api/: dotnet ef database update"},
	{name: ".net on sql server",
		files: map[string]string{"Api.csproj": `<PackageReference Include="Microsoft.EntityFrameworkCore.SqlServer" />`, "appsettings.json": `{"ConnectionStrings":{"AppDb":"x"}}`},
		kinds: []Kind{MSSQL}, vars: map[string]string{"ConnectionStrings__AppDb": "~Server=127.0.0.1,1433;Database=svc_development;User Id=sa", "DATABASE_URL": "~sqlserver://"}},
	{name: "laravel on postgres",
		files: map[string]string{"composer.json": `{"require":{"laravel/framework":"^11"},"scripts":{"db-seed":"x"}}`, "artisan": "",
			".env.example": "DB_CONNECTION=pgsql\nDB_HOST=127.0.0.1\nDB_PORT=5432\nDB_DATABASE=laravel\nDB_USERNAME=root\nDB_PASSWORD=\nREDIS_HOST=127.0.0.1\n"},
		kinds: []Kind{Postgres},
		vars:  map[string]string{"DB_DATABASE": "svc_development", "DB_PORT": "5432", "DB_USERNAME": "postgres"},
		none:  []string{"DB_CONNECTION", "REDIS_HOST"}, setup: "php artisan migrate"},
	{name: "symfony picks the engine from DATABASE_URL",
		files: map[string]string{"composer.json": `{"require":{"doctrine/doctrine-migrations-bundle":"*"}}`, "bin/console": "",
			".env.dist": `DATABASE_URL="postgresql://app:pass@127.0.0.1:5432/app?serverVersion=16"`},
		kinds: []Kind{Postgres}, vars: map[string]string{"DATABASE_URL": "~postgres://postgres:"}, setup: "doctrine:migrations:migrate"},
	{name: "django on postgres",
		files: map[string]string{"manage.py": "", "requirements.txt": "Django==5.0\npsycopg2-binary\ncelery[redis]\n",
			"mysite/settings.py": "DATABASES = {'default': {'ENGINE': 'django.db.backends.postgresql'}}"},
		kinds: []Kind{Postgres, Redis}, vars: map[string]string{"DATABASE_URL": "~postgres://", "REDIS_URL": "redis://127.0.0.1:6379/0"},
		setup: "python manage.py migrate"},
	{name: "fastapi with alembic on mysql",
		files: map[string]string{"pyproject.toml": "[project]\ndependencies = [\"fastapi\", \"sqlalchemy\", \"asyncmy\"]\n", "alembic.ini": ""},
		kinds: []Kind{MySQL}, vars: map[string]string{"DATABASE_URL": "~mysql://root:", "MYSQL_TCP_PORT": "3306"}, setup: "alembic upgrade head"},
	{name: "node monorepo with prisma in a sub-project",
		files: map[string]string{
			"package.json":                         `{"name":"mono","workspaces":["apps/*"]}`,
			"apps/web/package.json":                `{"dependencies":{"react":"18"}}`,
			"apps/api/package.json":                `{"dependencies":{"@prisma/client":"5","ioredis":"5"},"scripts":{"db:seed":"tsx seed.ts","test":"vitest"}}`,
			"apps/api/prisma/schema.prisma":        "datasource db {\n  provider = \"mysql\"\n  url = env(\"PRIMARY_DB\")\n  shadowDatabaseUrl = env(\"SHADOW_DB\")\n}",
			"apps/api/.env.example":                "PRIMARY_DB=\nREDIS_URL=\n",
			"apps/api/node_modules/x/package.json": `{"dependencies":{"mongodb":"6"}}`},
		kinds: []Kind{MySQL, Redis},
		vars:  map[string]string{"PRIMARY_DB": "~mysql://root:", "REDIS_URL": "redis://127.0.0.1:6379/0"},
		none:  []string{"SHADOW_DB", "MONGO_URL"}, setup: "in apps/api/: npx prisma migrate deploy"},
	{name: "nestjs typeorm on postgres via DB_TYPE",
		files: map[string]string{"package.json": `{"dependencies":{"@nestjs/core":"10","typeorm":"0.3"}}`, ".env.sample": "DB_TYPE=postgres\nDB_HOST=\nDB_NAME=\n"},
		kinds: []Kind{Postgres}, vars: map[string]string{"DB_NAME": "svc_development"}, setup: "TypeORM"},
	{name: "go on sql server with a make target",
		files: map[string]string{"go.mod": "module x\nrequire github.com/microsoft/go-mssqldb v1.7.0\n", "Makefile": "build:\n\tgo build\nmigrate-up:\n\tmigrate up\n",
			".env.example": "MSSQL_DSN=\n"},
		kinds: []Kind{MSSQL}, vars: map[string]string{"MSSQL_DSN": "~sqlserver://sa:"}, setup: "make migrate-up"},
	{name: "go on mysql with a DSN",
		files: map[string]string{"go.mod": "module x\nrequire gorm.io/driver/mysql v1.5.0\n", "config/.keep": "", ".env.example": "DB_DSN=\nTEST_DB_DSN=\n"},
		kinds: []Kind{MySQL}, vars: map[string]string{"DB_DSN": "~@tcp(127.0.0.1:3306)/svc_development?parseTime=true", "TEST_DB_DSN": "~/svc_test?"}},
	{name: "phoenix on postgres",
		files: map[string]string{"mix.exs": "defp deps do [{:phoenix, \"~> 1.7\"}, {:ecto_sql, \"~> 3.10\"}, {:postgrex, \">= 0.0.0\"}, {:redix, \"~> 1.1\"}] end"},
		kinds: []Kind{Postgres, Redis}, vars: map[string]string{"DATABASE_URL": "~postgres://"}, setup: "mix ecto.setup"},
	{name: "rust sqlx on postgres",
		files: map[string]string{"Cargo.toml": "[dependencies]\nsqlx = { version = \"0.7\", features = [\"postgres\", \"runtime-tokio\"] }\n", "migrations/0001_init.sql": ""},
		kinds: []Kind{Postgres}, vars: map[string]string{"DATABASE_URL": "~postgres://"}, setup: "sqlx migrate run"},
	{name: "dart server on postgres",
		files: map[string]string{"pubspec.yaml": "name: api\ndependencies:\n  shelf: ^1.4.0\n  postgres: ^3.0.0\n"},
		kinds: []Kind{Postgres}},
	{name: "mongoid rails app keeps DATABASE_URL out",
		files: map[string]string{"Gemfile": "gem 'rails'\ngem 'pg'\ngem 'mongoid'\n", "bin/rails": ""},
		kinds: []Kind{Postgres, Mongo}, vars: map[string]string{"MONGO_URL": "mongodb://127.0.0.1:27017/svc_development"},
		none: []string{"DATABASE_URL"}, setup: "bin/rails db:prepare"},
	{name: "sqlite and static sites need nothing",
		files: map[string]string{"package.json": `{"dependencies":{"better-sqlite3":"9","vite":"5"}}`, "index.html": ""}},
	{name: "compose names the image version",
		files: map[string]string{"docker-compose.yml": "services:\n  db:\n    image: postgres:15.4\n  cache:\n    image: valkey/valkey:7.2\n  sql:\n    image: mcr.microsoft.com/mssql/server:2019-latest\n"},
		kinds: []Kind{Postgres, MSSQL, Redis}},
}

func TestStacks(t *testing.T) {
	ports := map[Kind]int{MySQL: 3306, Postgres: 5432, MSSQL: 1433, Redis: 6379, Mongo: 27017}
	for _, c := range stackCases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range c.files {
				write(t, dir, name, body)
			}
			n := Detect(dir)
			if got, want := kindsString(n.Kinds), kindsString(c.kinds); got != want {
				t.Fatalf("kinds = %s, want %s (projects %v)", got, want, n.Projects)
			}
			svc := map[Kind]*Service{}
			for _, k := range n.Kinds {
				svc[k] = &Service{Kind: k, Port: ports[k], User: map[Kind]string{MySQL: "root", Postgres: "postgres", MSSQL: "sa"}[k], password: "Ev1secret"}
			}
			env := repoVars("svc", n, svc)
			for k, want := range c.vars {
				got, ok := env.Vars[k]
				if sub, partial := strings.CutPrefix(want, "~"); (partial && !strings.Contains(got, sub)) || (!partial && got != want) || !ok {
					t.Errorf("%s = %q, want %q\nall: %v", k, got, want, env.Vars)
				}
			}
			for _, k := range c.none {
				if v, ok := env.Vars[k]; ok {
					t.Errorf("%s must not be set, got %q", k, v)
				}
			}
			if c.setup != "" && !strings.Contains(strings.Join(n.Setup, "\n"), c.setup) {
				t.Errorf("setup %v is missing %q", n.Setup, c.setup)
			}
		})
	}
}

func TestComposeImagesAreUsed(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "docker-compose.yml", "services:\n  db:\n    image: postgres:15.4\n  sql:\n    image: mcr.microsoft.com/mssql/server:2019-latest\n")
	_, kinds, images := Plan([]Repo{{Name: "x", Dirs: []string{dir}}})
	if kindsString(kinds) != "postgres,mssql" || images[Postgres] != "postgres:15.4" || images[MSSQL] != "mcr.microsoft.com/mssql/server:2019-latest" {
		t.Fatalf("kinds=%v images=%v", kinds, images)
	}
}

func kindsString(ks []Kind) string {
	var s []string
	for _, k := range ks {
		s = append(s, string(k))
	}
	return strings.Join(s, ",")
}

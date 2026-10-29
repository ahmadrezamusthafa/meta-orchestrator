package testenv

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The schema has to exist before the app or its suites can run. setupHints names the way each
// project creates and seeds it — the framework's own migration command, or the Makefile, npm or
// Composer task the team wrote for it — so the agent runs that instead of guessing.

const maxHints = 5 // per project

// hint is a framework's migration command, offered when its marker is present.
type hint struct {
	when func(dir string) bool
	cmd  string
}

var hints = []hint{
	{func(d string) bool {
		return exists(d, "Gemfile") && (exists(d, "bin/rails") || exists(d, "config/database.yml"))
	},
		"bin/rails db:prepare && RAILS_ENV=test bin/rails db:prepare (then db:seed or the specs' factories)"},
	{func(d string) bool {
		return exists(d, "Gemfile") && exists(d, "db/migrations") && !exists(d, "config/database.yml")
	},
		"bundle exec rake db:migrate (Sequel/Hanami)"},
	{func(d string) bool { return exists(d, "artisan") }, "php artisan migrate --seed (and --env=testing for the test database)"},
	{func(d string) bool {
		return exists(d, "bin/console") && contains(d, "composer.json", "doctrine/doctrine-migrations-bundle")
	},
		"php bin/console doctrine:migrations:migrate -n (add --env=test for the test database)"},
	{func(d string) bool { return exists(d, "manage.py") }, "python manage.py migrate (then loaddata or a seed command)"},
	{func(d string) bool { return exists(d, "alembic.ini") }, "alembic upgrade head"},
	{func(d string) bool { return exists(d, "prisma/schema.prisma") || exists(d, "schema.prisma") },
		"npx prisma migrate deploy (npx prisma db push when there are no migrations) && npx prisma db seed"},
	{func(d string) bool { return exists(d, "knexfile.*") }, "npx knex migrate:latest && npx knex seed:run"},
	{func(d string) bool {
		return exists(d, ".sequelizerc") || contains(d, "package.json", "\"sequelize-cli\"")
	},
		"npx sequelize-cli db:migrate && npx sequelize-cli db:seed:all"},
	{func(d string) bool { return contains(d, "package.json", "\"typeorm\"") }, "the TypeORM migration:run script (or synchronize in the test config)"},
	{func(d string) bool { return contains(d, "package.json", "\"drizzle-kit\"") }, "npx drizzle-kit migrate (or push)"},
	{func(d string) bool { return contains(d, "mix.exs", ":ecto") }, "mix ecto.setup && MIX_ENV=test mix ecto.create && MIX_ENV=test mix ecto.migrate"},
	{func(d string) bool {
		return contains(d, "*.csproj", "EntityFrameworkCore") || contains(d, "*/*.csproj", "EntityFrameworkCore")
	},
		"dotnet ef database update (install it with dotnet tool install --global dotnet-ef when missing)"},
	{func(d string) bool {
		return contains(d, "pom.xml", "flyway", "liquibase") || contains(d, "build.gradle", "flyway", "liquibase") ||
			contains(d, "build.gradle.kts", "flyway", "liquibase")
	}, "Flyway/Liquibase migrations run when the Spring app starts; for tests use the same variables"},
	{func(d string) bool { return exists(d, "diesel.toml") }, "diesel migration run"},
	{func(d string) bool { return contains(d, "Cargo.toml", "sqlx") && exists(d, "migrations") }, "sqlx migrate run"},
	{func(d string) bool { return contains(d, "go.mod", "pressly/goose") }, `goose -dir <migrations dir> <driver> "$DATABASE_URL" up`},
	{func(d string) bool { return contains(d, "go.mod", "golang-migrate/migrate") },
		`migrate -path <migrations dir> -database "$DATABASE_URL" up`},
	{func(d string) bool { return exists(d, "atlas.hcl") }, `atlas migrate apply --url "$DATABASE_URL"`},
	{func(d string) bool { return exists(d, "dbmate.*") || contains(d, ".env.example", "DBMATE_") }, "dbmate up"},
}

// taskRe names a Makefile, npm or Composer task that sets up a database.
var taskRe = regexp.MustCompile(`(?i)(migrat|seed|^db[:_-]|[:_-]db$|[:_-]db[:_-]|setup|schema|fixtures)`)

var makeTargetRe = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9_.:/-]*)\s*:([^=]|$)`)

// setupHints lists how the project at dir (rel from the repository root) creates its schema.
func setupHints(dir, rel string) []string {
	var out []string
	for _, h := range hints {
		if h.when(dir) {
			out = append(out, h.cmd)
		}
	}
	out = append(out, makeTargets(dir)...)
	out = append(out, scriptTasks(dir, "package.json", "npm run ")...)
	out = append(out, scriptTasks(dir, "composer.json", "composer run ")...)
	if len(out) == 0 {
		for _, d := range []string{"db/migrate", "db/migrations", "migrations", "sql", "db/schema.sql", "schema.sql"} {
			if exists(dir, d) {
				out = append(out, fmt.Sprintf("apply %s with the database's own client (mysql / psql / sqlcmd)", d))
				break
			}
		}
	}
	if len(out) > maxHints {
		out = out[:maxHints]
	}
	if rel != "." && rel != "" {
		for i := range out {
			out[i] = fmt.Sprintf("in %s/: %s", filepath.ToSlash(rel), out[i])
		}
	}
	return out
}

func makeTargets(dir string) []string {
	f, err := os.Open(filepath.Join(dir, "Makefile"))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := makeTargetRe.FindStringSubmatch(sc.Text()); m != nil && !strings.HasPrefix(m[1], ".") && taskRe.MatchString(m[1]) {
			out = appendNew(out, "make "+m[1])
		}
	}
	return out
}

func scriptTasks(dir, file, prefix string) []string {
	raw, err := readSmall(filepath.Join(dir, file))
	if err != nil {
		return nil
	}
	var doc struct {
		Scripts map[string]interface{} `json:"scripts"`
	}
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return nil
	}
	var out []string
	for name := range doc.Scripts {
		if taskRe.MatchString(name) && !strings.Contains(name, "test") {
			out = append(out, prefix+name)
		}
	}
	sort.Strings(out)
	return out
}

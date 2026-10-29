package testenv

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// railsRepo is shaped like a Rails app on MySQL with Sidekiq: the case that used to end in
// "Not run: no usable test database".
func railsRepo(t *testing.T) string {
	dir := t.TempDir()
	write(t, dir, "Gemfile", "gem 'rails'\ngem 'mysql2'\ngem 'sidekiq'\n")
	write(t, dir, "config/database.yml", "default: &default\n  adapter: mysql2\n  host: <%= ENV['DB_HOST'] %>\n")
	write(t, dir, "docker-compose.yml", "services:\n  mysql:\n    image: mysql:8.0\n  redis:\n    image: redis:7-alpine\n  kafka:\n    image: confluentinc/cp-kafka:7.5.0\n")
	write(t, dir, ".env.template", "DB_HOST=\nDB_USERNAME=\nDB_PASSWORD=\nDB_TIMEOUT=5000\nDB_NAME=\nDB_TEST_NAME=\nREDIS_URL=\nREPLICA_DB_HOST=\nKAFKA_BROKERS=\n")
	return dir
}

func TestDetect(t *testing.T) {
	n := Detect(railsRepo(t))
	if got := n.Kinds; len(got) != 2 || got[0] != MySQL || got[1] != Redis {
		t.Fatalf("kinds = %v, want [mysql redis]", got)
	}
	if n.Images[MySQL] != "mysql:8.0" || n.Images[Redis] != "redis:7-alpine" || !n.Ruby() {
		t.Fatalf("images = %v ruby=%v", n.Images, n.Ruby())
	}

	front := t.TempDir()
	write(t, front, "package.json", `{"name":"web","description":"talks to redis via the API","dependencies":{"react":"18"}}`)
	if n := Detect(front); !n.Empty() {
		t.Fatalf("a frontend needs nothing, got %v", n.Kinds)
	}

	goSvc := t.TempDir()
	write(t, goSvc, "go.mod", "module x\nrequire github.com/jackc/pgx/v5 v5.5.0\n")
	write(t, goSvc, ".env.example", "export PG_DSN=\nDATABASE_URL=\n")
	if n := Detect(goSvc); len(n.Kinds) != 1 || n.Kinds[0] != Postgres || len(n.EnvKeys) != 2 {
		t.Fatalf("go service: %+v", n)
	}
}

func TestRepoVars(t *testing.T) {
	n := Detect(railsRepo(t))
	svc := map[Kind]*Service{
		MySQL: {Kind: MySQL, Port: 55001, User: "root", password: "pw"},
		Redis: {Kind: Redis, Port: 55002},
	}
	env := repoVars("billing", n, svc)
	want := map[string]string{
		"DB_HOST": "127.0.0.1", "DB_USERNAME": "root", "DB_PASSWORD": "pw", "DB_NAME": "billing_development",
		"DB_TEST_NAME": "billing_test", "REPLICA_DB_HOST": "127.0.0.1", "REDIS_URL": "redis://127.0.0.1:55002/0",
		"MYSQL_TCP_PORT": "55001", "MYSQL_HOST": "127.0.0.1", "MYSQL_PWD": "pw",
	}
	for k, v := range want {
		if env.Vars[k] != v {
			t.Errorf("%s = %q, want %q", k, env.Vars[k], v)
		}
	}
	for _, k := range []string{"DB_TIMEOUT", "KAFKA_BROKERS"} {
		if _, ok := env.Vars[k]; ok {
			t.Errorf("%s must be left to the repository", k)
		}
	}

	pg := &Service{Kind: Postgres, Port: 55003, User: "postgres", password: "pw"}
	g := repoVars("svc", Needs{Kinds: []Kind{Postgres}, EnvKeys: []string{"DATABASE_URL", "TEST_DATABASE_URL", "PG_DSN"}}, map[Kind]*Service{Postgres: pg})
	if g.Vars["DATABASE_URL"] != "postgres://postgres:pw@127.0.0.1:55003/svc_development?sslmode=disable" ||
		!strings.Contains(g.Vars["TEST_DATABASE_URL"], "/svc_test?") || g.Vars["PGPORT"] != "55003" {
		t.Fatalf("postgres vars: %v", g.Vars)
	}
}

// fakeDocker answers like the docker CLI for a daemon with no containers yet.
type fakeDocker struct {
	mu    sync.Mutex
	calls [][]string
	down  bool
	have  map[string]string // container → "running|image"
}

func (f *fakeDocker) run(_ context.Context, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, args)
	switch args[0] {
	case "info":
		if f.down {
			return "", errors.New("Cannot connect to the Docker daemon")
		}
		return "27.0.0", nil
	case "inspect":
		if v, ok := f.have[args[len(args)-1]]; ok {
			return v, nil
		}
		return "", errors.New("No such object")
	case "run":
		name := args[3]
		f.have[name] = "true|" + args[len(args)-1]
		if strings.HasPrefix(args[len(args)-1], "--") {
			f.have[name] = "true|" + args[len(args)-2]
		}
	case "port":
		if strings.HasPrefix(args[2], "3306") {
			return "127.0.0.1:55001\n", nil
		}
		return "127.0.0.1:55002\n", nil
	}
	return "", nil
}

func (f *fakeDocker) count(verb string) int {
	n := 0
	for _, c := range f.calls {
		if c[0] == verb {
			n++
		}
	}
	return n
}

func TestEnsureStartsReusesAndTearsDown(t *testing.T) {
	repo := railsRepo(t)
	fd := &fakeDocker{have: map[string]string{}}
	m := &Manager{Docker: fd.run, Root: t.TempDir()}
	repos := []Repo{{Name: "billing", Dirs: []string{repo}}, {Name: "web", Dirs: []string{t.TempDir()}}}

	env, err := m.Ensure(context.Background(), "TASK-7", repos)
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Services) != 2 || fd.count("run") != 2 || len(env.Repos) != 1 {
		t.Fatalf("services=%d runs=%d repos=%d", len(env.Services), fd.count("run"), len(env.Repos))
	}
	for _, c := range fd.calls {
		if strings.Contains(strings.Join(c, " "), env.Services[0].password) {
			t.Fatalf("a password reached the docker command line: %v", c)
		}
	}
	info, err := os.Stat(env.File)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("env.json: %v %v", info, err)
	}
	var doc struct {
		Repos  []RepoEnv `json:"repos"`
		Redact []string  `json:"redact"`
	}
	raw, _ := os.ReadFile(env.File)
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.Redact) != 1 || doc.Repos[0].Dirs[0] != repo {
		t.Fatalf("env.json = %s (%v)", raw, err)
	}
	if sh, _ := os.ReadFile(env.ShellFile("billing")); !strings.Contains(string(sh), "export DB_TEST_NAME='billing_test'") {
		t.Fatalf("shell file:\n%s", sh)
	}

	// The next turn reuses the running containers and the same credentials.
	again, err := m.Ensure(context.Background(), "TASK-7", repos)
	if err != nil {
		t.Fatal(err)
	}
	if fd.count("run") != 2 || len(again.Reused) != 2 || again.Secrets()[0] != env.Secrets()[0] {
		t.Fatalf("not reused: runs=%d reused=%v", fd.count("run"), again.Reused)
	}

	if err := m.Teardown(context.Background(), "TASK-7"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(env.File)); !os.IsNotExist(err) {
		t.Fatal("state left behind after teardown")
	}
}

func TestEnsureCreatesDatabasesInEveryEngine(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "docker-compose.yml", "services:\n  db:\n    image: postgres:16\n  sql:\n    image: mcr.microsoft.com/mssql/server:2022-latest\n")
	fd := &fakeDocker{have: map[string]string{}}
	m := &Manager{Docker: fd.run, Root: t.TempDir()}
	if _, err := m.Ensure(context.Background(), "TASK-2", []Repo{{Name: "shop", Dirs: []string{dir}}}); err != nil {
		t.Fatal(err)
	}
	created := map[string]bool{}
	for _, c := range fd.calls {
		if c[0] == "exec" && strings.Contains(c[len(c)-1], "shop_test") {
			created[c[1]] = true
		}
	}
	if !created["mo-testenv-task-2-postgres"] || !created["mo-testenv-task-2-mssql"] {
		t.Fatalf("databases created in %v, want postgres and mssql", created)
	}
}

func TestEnsureWithoutDocker(t *testing.T) {
	m := &Manager{Docker: (&fakeDocker{down: true}).run, Root: t.TempDir()}
	_, err := m.Ensure(context.Background(), "TASK-1", []Repo{{Name: "billing", Dirs: []string{railsRepo(t)}}})
	if !errors.Is(err, ErrNoDocker) {
		t.Fatalf("err = %v, want ErrNoDocker", err)
	}
	env, err := m.Ensure(context.Background(), "TASK-1", []Repo{{Name: "web", Dirs: []string{t.TempDir()}}})
	if env != nil || err != nil {
		t.Fatalf("nothing needed: env=%v err=%v", env, err)
	}
}

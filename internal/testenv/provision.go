package testenv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Docker runs the docker CLI and returns its standard output.
type Docker func(ctx context.Context, args ...string) (string, error)

// CLI is Docker on the real docker command.
func CLI(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return string(out), fmt.Errorf("docker %s: %s", args[0], strings.TrimSpace(lastLine(string(ee.Stderr))))
		}
		return string(out), fmt.Errorf("docker %s: %w", args[0], err)
	}
	return string(out), nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	return s[strings.LastIndex(s, "\n")+1:]
}

// ErrNoDocker means the Docker daemon is not reachable, so no service can start.
var ErrNoDocker = errors.New("docker is not running")

// Repo is a repository of the task: its name and the directories it is checked out in.
type Repo struct {
	Name string
	Dirs []string // the first is read for its needs
}

// Env is a task's running test environment.
type Env struct {
	Services []*Service `json:"services"`
	Repos    []RepoEnv  `json:"repos"`
	File     string     `json:"-"` // JSON for the evidence recorder: each repository's variables, and the values to redact
	Reused   []Kind     `json:"-"` // services that were already running
}

// ShellFile is where the repository's variables are, as a file a shell sources.
func (e *Env) ShellFile(repo string) string {
	return filepath.Join(filepath.Dir(e.File), safe(repo)+".env.sh")
}

// Secrets are the values the evidence must never show.
func (e *Env) Secrets() []string {
	var out []string
	for _, s := range e.Services {
		if s.password != "" {
			out = append(out, s.password)
		}
	}
	return out
}

// Manager starts and removes the test environments, one per task, with state under Root.
type Manager struct {
	Docker Docker
	Root   string        // per-task state: credentials and env files, never served or committed
	Wait   time.Duration // how long a service may take to accept connections (default 3 min)
	// Progress, when set, hears about the slow steps (an image download) as they start.
	Progress func(msg string)
}

func (m *Manager) progress(format string, args ...interface{}) {
	if m.Progress != nil {
		m.Progress(fmt.Sprintf(format, args...))
	}
}

const labelKey = "meta-orchestrator.testenv"

// defaults are the images used when the repository's compose file names none.
var defaults = map[Kind]string{MySQL: "mysql:8.0", Postgres: "postgres:16-alpine", MSSQL: "mcr.microsoft.com/mssql/server:2022-latest",
	Redis: "redis:7-alpine", Mongo: "mongo:7"}

var containerPort = map[Kind]int{MySQL: 3306, Postgres: 5432, MSSQL: 1433, Redis: 6379, Mongo: 27017}

type state struct {
	Password string          `json:"password"`
	Images   map[Kind]string `json:"images"` // the image each container was started from
}

var safeRe = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func safe(s string) string { return strings.Trim(safeRe.ReplaceAllString(s, "-"), "-.") }

func (m *Manager) dir(taskID string) string { return filepath.Join(m.Root, safe(taskID)) }

func containerName(taskID string, k Kind) string {
	return "mo-testenv-" + strings.ToLower(safe(taskID)) + "-" + string(k)
}

// Plan lists what the repositories need, and the image each service runs.
func Plan(repos []Repo) (map[string]Needs, []Kind, map[Kind]string) {
	needs := map[string]Needs{}
	images := map[Kind]string{}
	want := map[Kind]bool{}
	for _, r := range repos {
		if len(r.Dirs) == 0 {
			continue
		}
		n := Detect(r.Dirs[0])
		needs[r.Name] = n
		for _, k := range n.Kinds {
			want[k] = true
			if images[k] == "" {
				images[k] = n.Images[k]
			}
		}
	}
	var kinds []Kind
	for _, k := range kindOrder {
		if want[k] {
			kinds = append(kinds, k)
			if images[k] == "" {
				images[k] = defaults[k]
			}
		}
	}
	return needs, kinds, images
}

// Ensure starts (or reuses) the services the repositories need and writes their environment.
// It returns nil, nil when no repository needs one.
func (m *Manager) Ensure(ctx context.Context, taskID string, repos []Repo) (*Env, error) {
	needs, kinds, images := Plan(repos)
	if len(kinds) == 0 {
		return nil, nil
	}
	if _, err := m.Docker(ctx, "info", "--format", "{{.ServerVersion}}"); err != nil {
		return nil, fmt.Errorf("%w (%v)", ErrNoDocker, err)
	}
	dir := m.dir(taskID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	st := m.loadState(dir)
	if st.Password == "" {
		st.Password = randomHex(12)
		st.Images = map[Kind]string{}
	}
	env := &Env{File: filepath.Join(dir, "env.json")}
	svc := map[Kind]*Service{}
	for _, k := range kinds {
		s, reused, err := m.ensureService(ctx, taskID, dir, k, images[k], &st)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k.Label(), err)
		}
		svc[k] = s
		env.Services = append(env.Services, s)
		if reused {
			env.Reused = append(env.Reused, k)
		}
	}
	if err := m.saveState(dir, st); err != nil {
		return nil, err
	}
	for _, r := range repos {
		n, ok := needs[r.Name]
		if !ok || n.Empty() {
			continue
		}
		re := repoVars(r.Name, n, svc)
		re.Dirs = r.Dirs
		for _, k := range n.Kinds { // every engine it uses: MSSQL_* may sit beside a generic DB_* on Postgres
			if s := svc[k]; s != nil && k.relational() {
				if err := m.createDatabases(ctx, s, re.DBs); err != nil {
					return nil, fmt.Errorf("%s databases for %s: %w", k.Label(), r.Name, err)
				}
			}
		}
		env.Repos = append(env.Repos, re)
	}
	return env, m.writeEnv(env)
}

// ensureService reuses the task's container when it runs the same image, else starts a new one,
// and waits until it accepts connections.
func (m *Manager) ensureService(ctx context.Context, taskID, dir string, k Kind, image string, st *state) (*Service, bool, error) {
	name := containerName(taskID, k)
	s := &Service{Kind: k, Image: image, Container: name, password: st.Password}
	switch k {
	case MySQL:
		s.User = "root"
	case Postgres:
		s.User = "postgres"
	case MSSQL:
		s.User = "sa"
		s.password = "Ev1" + st.Password // SQL Server wants upper and lower case letters and digits
	case Redis, Mongo:
		s.password = ""
	}
	reused := false
	out, err := m.Docker(ctx, "inspect", "-f", "{{.State.Running}}|{{.Config.Image}}", name)
	running, img, _ := strings.Cut(strings.TrimSpace(out), "|")
	switch {
	case err == nil && img == image && st.Images[k] == image:
		reused = true
		if running != "true" {
			if _, err := m.Docker(ctx, "start", name); err != nil {
				return nil, false, err
			}
		}
	default:
		if err == nil {
			_, _ = m.Docker(ctx, "rm", "-f", "-v", name)
		}
		if _, err := m.Docker(ctx, "image", "inspect", "-f", "{{.Id}}", image); err != nil {
			m.progress("Downloading %s for %s — only the first time; it can take a few minutes.", image, k.Label())
		}
		if err := m.run(ctx, taskID, dir, s); err != nil {
			return nil, false, err
		}
		st.Images[k] = image
	}
	port, err := m.port(ctx, name, containerPort[k])
	if err != nil {
		return nil, false, err
	}
	s.Port = port
	if err := m.waitReady(ctx, s); err != nil {
		return nil, false, err
	}
	return s, reused, nil
}

// run starts the container, its credentials passed in a private env file rather than on the
// command line, its port published on 127.0.0.1 only.
func (m *Manager) run(ctx context.Context, taskID, dir string, s *Service) error {
	args := []string{"run", "-d", "--name", s.Container, "--label", labelKey + "=" + taskID,
		"-p", fmt.Sprintf("%s::%d", host, containerPort[s.Kind])}
	var envs []string
	switch s.Kind {
	case MySQL:
		envs = []string{"MYSQL_ROOT_PASSWORD=" + s.password, "MARIADB_ROOT_PASSWORD=" + s.password}
	case Postgres:
		envs = []string{"POSTGRES_PASSWORD=" + s.password, "POSTGRES_USER=" + s.User}
	case MSSQL:
		envs = []string{"ACCEPT_EULA=Y", "MSSQL_SA_PASSWORD=" + s.password, "MSSQL_PID=Developer"}
		args = append(args, "--platform", "linux/amd64") // the only platform it is built for; emulated on Apple silicon
	}
	if len(envs) > 0 {
		f := filepath.Join(dir, string(s.Kind)+".docker.env")
		if err := os.WriteFile(f, []byte(strings.Join(envs, "\n")+"\n"), 0o600); err != nil {
			return err
		}
		defer os.Remove(f)
		args = append(args, "--env-file", f)
	}
	args = append(args, s.Image)
	if s.Kind == MySQL && strings.HasPrefix(strings.ToLower(s.Image), "mysql:8.0") {
		// Older client libraries (mysql2 built on libmysqlclient 5.x) cannot speak caching_sha2.
		args = append(args, "--default-authentication-plugin=mysql_native_password")
	}
	_, err := m.Docker(ctx, args...)
	return err
}

func (m *Manager) port(ctx context.Context, name string, inner int) (int, error) {
	out, err := m.Docker(ctx, "port", name, fmt.Sprintf("%d/tcp", inner))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if p, err := strconv.Atoi(line[strings.LastIndex(line, ":")+1:]); err == nil && p > 0 {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no published port for %s in %q", name, out)
}

// probe is the command, run in the container, that succeeds once the service accepts TCP
// connections — not while an image's first-start script runs it on a socket only.
func probe(k Kind) string {
	switch k {
	case MySQL:
		return `MYSQL_PWD="${MYSQL_ROOT_PASSWORD:-$MARIADB_ROOT_PASSWORD}"; export MYSQL_PWD; ` +
			`(mysqladmin ping -h127.0.0.1 -uroot --silent || mariadb-admin ping -h127.0.0.1 -uroot --silent) >/dev/null 2>&1 && ` +
			`(mysql -h127.0.0.1 -uroot -e 'select 1' || mariadb -h127.0.0.1 -uroot -e 'select 1') >/dev/null 2>&1`
	case Postgres:
		return `pg_isready -h 127.0.0.1 -U "$POSTGRES_USER" >/dev/null 2>&1`
	case MSSQL:
		return sqlcmd + ` -Q "select 1" >/dev/null 2>&1`
	case Redis:
		return `redis-cli ping | grep -q PONG`
	case Mongo:
		return `(mongosh --quiet --eval 'db.adminCommand({ping:1}).ok' || mongo --quiet --eval 'db.adminCommand({ping:1}).ok') 2>/dev/null | grep -q 1`
	}
	return "true"
}

// sqlcmd runs SQL Server's client from whichever tools folder the image has (18 needs -C to
// trust the self-signed certificate).
// The password comes from the container's own environment, never from this command line.
const sqlcmd = `export SQLCMDPASSWORD="$MSSQL_SA_PASSWORD"; q() { if [ -x /opt/mssql-tools18/bin/sqlcmd ]; then ` +
	`/opt/mssql-tools18/bin/sqlcmd -C -S 127.0.0.1 -U sa -b "$@"; else /opt/mssql-tools/bin/sqlcmd -S 127.0.0.1 -U sa -b "$@"; fi; }; q`

func (m *Manager) waitReady(ctx context.Context, s *Service) error {
	wait := m.Wait
	if wait == 0 {
		wait = 3 * time.Minute
		if s.Kind == MSSQL {
			wait = 6 * time.Minute // emulated on Apple silicon
		}
	}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	for {
		if _, err := m.Docker(ctx, "exec", s.Container, "sh", "-c", probe(s.Kind)); err == nil {
			return nil
		}
		if out, err := m.Docker(ctx, "inspect", "-f", "{{.State.Running}}", s.Container); err == nil && strings.TrimSpace(out) == "false" {
			logs, _ := m.Docker(context.Background(), "logs", "--tail", "5", s.Container)
			return fmt.Errorf("the container stopped while starting: %s", strings.TrimSpace(lastLine(logs)))
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("not accepting connections after %s", wait)
		case <-time.After(time.Second):
		}
	}
}

// createDatabases makes the repository's empty databases (names are [a-z0-9_] only).
func (m *Manager) createDatabases(ctx context.Context, s *Service, dbs []string) error {
	var script string
	switch s.Kind {
	case MySQL:
		var sql []string
		for _, d := range dbs {
			sql = append(sql, "CREATE DATABASE IF NOT EXISTS "+d+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;")
		}
		script = `MYSQL_PWD="${MYSQL_ROOT_PASSWORD:-$MARIADB_ROOT_PASSWORD}"; export MYSQL_PWD; ` +
			fmt.Sprintf(`mysql -h127.0.0.1 -uroot -e '%[1]s' 2>/dev/null || mariadb -h127.0.0.1 -uroot -e '%[1]s'`, strings.Join(sql, " "))
	case Postgres:
		var parts []string
		for _, d := range dbs {
			parts = append(parts, fmt.Sprintf(`psql -U "$POSTGRES_USER" -tAc "select 1 from pg_database where datname='%[1]s'" | grep -q 1 || `+
				`psql -U "$POSTGRES_USER" -qc 'create database %[1]s'`, d))
		}
		script = strings.Join(parts, " && ")
	case MSSQL:
		var sql []string
		for _, d := range dbs {
			sql = append(sql, fmt.Sprintf("IF DB_ID('%[1]s') IS NULL CREATE DATABASE [%[1]s];", d))
		}
		script = sqlcmd + ` -Q "` + strings.Join(sql, " ") + `"`
	default:
		return nil
	}
	_, err := m.Docker(ctx, "exec", s.Container, "sh", "-c", script)
	return err
}

// Has reports whether the task has a test environment to remove.
func (m *Manager) Has(taskID string) bool {
	_, err := os.Stat(m.dir(taskID))
	return err == nil
}

// Teardown removes the task's containers, their data and its state.
func (m *Manager) Teardown(ctx context.Context, taskID string) error {
	out, err := m.Docker(ctx, "ps", "-aq", "--filter", "label="+labelKey+"="+taskID)
	if err == nil {
		if ids := strings.Fields(out); len(ids) > 0 {
			_, err = m.Docker(ctx, append([]string{"rm", "-f", "-v"}, ids...)...)
		}
	}
	_ = os.RemoveAll(m.dir(taskID))
	return err
}

func (m *Manager) loadState(dir string) state {
	var st state
	if raw, err := os.ReadFile(filepath.Join(dir, "state.json")); err == nil {
		_ = json.Unmarshal(raw, &st)
	}
	if st.Images == nil {
		st.Images = map[Kind]string{}
	}
	return st
}

func (m *Manager) saveState(dir string, st state) error {
	raw, _ := json.Marshal(st)
	return os.WriteFile(filepath.Join(dir, "state.json"), raw, 0o600)
}

// writeEnv writes the recorder's env.json and a shell file per repository, readable only by the
// operator's user.
func (m *Manager) writeEnv(env *Env) error {
	doc := struct {
		Repos  []RepoEnv `json:"repos"`
		Redact []string  `json:"redact"`
	}{env.Repos, env.Secrets()}
	raw, _ := json.MarshalIndent(doc, "", "  ")
	if err := os.WriteFile(env.File, raw, 0o600); err != nil {
		return err
	}
	for _, r := range env.Repos {
		keys := make([]string, 0, len(r.Vars))
		for k := range r.Vars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString("# Test environment of " + r.Name + " — throwaway local services. Source it: . <this file>\n")
		for _, k := range keys {
			fmt.Fprintf(&b, "export %s='%s'\n", k, strings.ReplaceAll(r.Vars[k], "'", `'\''`))
		}
		if err := os.WriteFile(env.ShellFile(r.Name), []byte(b.String()), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

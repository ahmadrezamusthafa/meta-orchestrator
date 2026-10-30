package api

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/testenv"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// The test environment: before the evidence run, the orchestrator starts the datastores the task's
// repositories need (MySQL, PostgreSQL, Redis, MongoDB) as throwaway Docker containers with fresh
// credentials and empty databases, and hands the agent their variables. The agent then starts the
// app and its suites against them instead of stopping at "no usable test database" or asking the
// operator for credentials. The containers live as long as the task and are removed with it.

func (r *Router) testEnvManager() *testenv.Manager {
	return &testenv.Manager{Docker: testenv.CLI, Root: filepath.Join(r.cfg.RootDir, ".sdlc", "testenv")}
}

// testEnvProgress is a context value: the console line a slow step of the setup reports to.
type testEnvProgress struct{}

// testEnvRepos are the task's repositories, each read from its worktree when it has one.
func (r *Router) testEnvRepos(ctx context.Context, t *types.Task) []testenv.Repo {
	var out []testenv.Repo
	for _, wt := range r.describeWorktrees(ctx, t) {
		repo := testenv.Repo{Name: wt.Repo}
		if wt.Exists && wt.Dir != "" {
			repo.Dirs = append(repo.Dirs, wt.Dir)
		}
		if wt.Source != "" {
			repo.Dirs = append(repo.Dirs, wt.Source)
		}
		out = append(out, repo)
	}
	return out
}

// provisionTestEnv starts (or reuses) the task's test environment; nil when no repository needs one.
var provisionTestEnv = func(r *Router, ctx context.Context, t *types.Task, starting func([]testenv.Kind, map[testenv.Kind]string)) (*testenv.Env, error) {
	repos := r.testEnvRepos(ctx, t)
	if _, kinds, images := testenv.Plan(repos); len(kinds) > 0 {
		starting(kinds, images)
	}
	m := r.testEnvManager()
	m.Progress, _ = ctx.Value(testEnvProgress{}).(func(string))
	return m.Ensure(ctx, t.ID, repos)
}

// teardownTestEnv removes the task's containers; it runs in the background when a task ends.
var teardownTestEnv = func(r *Router, taskID string) {
	m := r.testEnvManager()
	if !m.Has(taskID) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_ = m.Teardown(ctx, taskID)
}

// prepareTestEnv readies the test environment for an evidence run and reports it on the console.
func (r *Router) prepareTestEnv(ctx context.Context, t *types.Task, turnID string) (*testenv.Env, string) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Minute) // first image downloads, SQL Server emulated on Apple silicon
	defer cancel()
	ctx = context.WithValue(ctx, testEnvProgress{}, func(msg string) {
		r.addEntry(t.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: turnID, Content: "⏳ " + msg})
	})
	env, err := provisionTestEnv(r, ctx, t, func(kinds []testenv.Kind, images map[testenv.Kind]string) {
		var names []string
		for _, k := range kinds {
			names = append(names, fmt.Sprintf("%s (%s)", k.Label(), images[k]))
		}
		r.addEntry(t.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: turnID, Content: "🧪 Preparing the test environment: " +
			strings.Join(names, ", ") + " in Docker, with fresh credentials and empty databases, so the agent can run the app and its tests end to end."})
	})
	switch {
	case errors.Is(err, testenv.ErrNoDocker):
		msg := "Docker is not running, so the orchestrator could not start a throwaway test database for this task. " +
			"Start Docker Desktop and press Record evidence again — no credentials needed."
		r.addEntry(t.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: turnID, Content: "⚠ " + msg})
		return nil, msg
	case err != nil:
		msg := fmt.Sprintf("The throwaway test environment could not start (%v).", err)
		r.addEntry(t.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: turnID, Content: "⚠ " + msg +
			" The agent is told to start the repository's own docker compose services instead."})
		return nil, msg
	case env == nil:
		return nil, ""
	}
	var parts []string
	for _, s := range env.Services {
		parts = append(parts, fmt.Sprintf("%s on 127.0.0.1:%d", s.Kind.Label(), s.Port))
	}
	msg := "✓ Test environment ready: " + strings.Join(parts, ", ")
	for _, re := range env.Repos {
		if len(re.DBs) > 0 {
			msg += fmt.Sprintf("; %s gets databases %s", re.Name, strings.Join(re.DBs, " and "))
		}
	}
	if len(env.Reused) == len(env.Services) {
		msg += " (kept from the last run)"
	}
	r.addEntry(t.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: turnID, Content: msg +
		". It is removed when the task is completed or deleted."})
	return env, ""
}

// testEnvBrief tells the agent how to use the test environment, without any credential in it.
func testEnvBrief(env *testenv.Env, problem string) string {
	var b strings.Builder
	b.WriteString("\n### Test environment (provided — do not ask for database credentials)\n")
	if env == nil {
		if problem != "" {
			b.WriteString(problem + " Start the datastores the app needs from the repository's own docker compose file (only those services, " +
				"never production data), point the app at them through environment variables, and seed fake data. Only when that is " +
				"impossible too, mark the affected cases blocked with the exact reason.\n")
		}
		return b.String()
	}
	b.WriteString("The orchestrator started throwaway local services for this task, reserved for it alone — create, migrate, seed and wipe them freely:\n")
	for _, s := range env.Services {
		fmt.Fprintf(&b, "- %s (%s) on 127.0.0.1:%d\n", s.Kind.Label(), s.Image, s.Port)
	}
	b.WriteString("Each repository's variables point at them (database names, user, password, URLs, and the MYSQL_*/PG* defaults the client " +
		"libraries read). Load them in every command that runs the app, a migration or a test suite (the Bash tool does not keep variables " +
		"between calls), e.g. `. <file> && bundle exec rspec`:\n")
	for _, re := range env.Repos {
		keys := make([]string, 0, len(re.Vars))
		for k := range re.Vars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintf(&b, "- %s: `. %s` — sets %s\n", re.Name, shellQuote(env.ShellFile(re.Name)), strings.Join(keys, ", "))
	}
	b.WriteString("The evidence recorder loads them on its own for every `run` and `start` step whose `cwd` is in that repository, and blurs " +
		"the password everywhere. Never print the file or the variables, and never copy the values into the repository, a storyboard or a commit.\n" +
		"Before recording, create the schema and seed fake data that matches each case's preconditions, the repository's own way:\n")
	for _, re := range env.Repos {
		if len(re.Setup) > 0 {
			fmt.Fprintf(&b, "- %s: %s\n", re.Name, strings.Join(re.Setup, "; "))
		} else {
			fmt.Fprintf(&b, "- %s: no migration tool was recognised — read its README, Makefile or CI config for how it creates the schema\n", re.Name)
		}
	}
	b.WriteString("Where the app's configuration does not read these variables (a hard-coded config/test.exs, a config.yaml), pass the values the way " +
		"it does accept them for the run — a command-line flag, a config override file outside git, an env-specific profile — never by editing " +
		"committed configuration. Only if the app's env loader overrides the environment, copy them into an untracked, git-ignored local env file " +
		"(e.g. .env.test.local) for the run and delete it afterwards. " +
		"A database problem is therefore never a reason to skip the suites or the walkthroughs: fix the setup and run them.\n")
	return b.String()
}

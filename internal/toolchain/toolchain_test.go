package toolchain

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/stack"
)

// fakeHost has the given binaries; run answers from versions, failing for those in broken.
func fakeHost(goos string, bins map[string]bool, broken map[string]string) host {
	return host{
		goos: goos,
		lookPath: func(name string) (string, error) {
			if bins[name] {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		run: func(ctx context.Context, name string, args ...string) (string, error) {
			bin := strings.TrimPrefix(name, "/usr/bin/")
			if msg, ok := broken[bin]; ok {
				return msg + "\n", errors.New("exit status 1")
			}
			return "\n" + bin + " version 1.2.3\nmore\n", nil
		},
		getenv: func(string) string { return "" },
		isDir:  func(string) bool { return false },
	}
}

func newTestToolchain(h host) *Toolchain {
	c := &Checker{host: h, ttl: time.Minute, seen: map[string]Status{}}
	return &Toolchain{Checker: c, Installer: NewInstaller(c)}
}

func TestCheckRunsTheToolNotJustPath(t *testing.T) {
	tc := newTestToolchain(fakeHost("darwin", map[string]bool{"go": true, "java": true},
		map[string]string{"java": "The operation couldn’t be completed. Unable to locate a Java Runtime."}))
	goTool, _ := Lookup("go")
	if s := tc.Checker.Check(goTool); !s.Installed || s.Version != "go version 1.2.3" {
		t.Fatalf("go status = %+v", s)
	}
	java, _ := Lookup("java")
	s := tc.Checker.Check(java)
	if s.Installed || !strings.Contains(s.Problem, "Unable to locate a Java Runtime") {
		t.Fatalf("the macOS java stub is not a JDK: %+v", s)
	}
	sdk, _ := Lookup("android-sdk")
	if tc.Checker.Check(sdk).Installed {
		t.Fatal("no ANDROID_HOME and no default SDK folder means no SDK")
	}
}

func TestReportRequiredFirstWithInstallMethodsForThisHost(t *testing.T) {
	tc := newTestToolchain(fakeHost("darwin", map[string]bool{"brew": true, "go": true}, nil))
	reqs := []Requirement{
		{Tool: "actionlint", Reasons: []string{"svc: lint workflows"}},
		{Tool: "tofu", Required: true, Reasons: []string{"infra: validate"}},
		{Tool: "go", Required: true, Reasons: []string{"svc: build"}},
		{Tool: "java", Required: true, Reasons: []string{"ledger: build"}},
	}
	rep := tc.Report(reqs, false)
	var order []string
	for _, it := range rep.Items {
		order = append(order, it.ID)
	}
	if got := strings.Join(order, ","); got != "go,java,tofu,actionlint" {
		t.Fatalf("order = %s, want required (catalog order) before recommended", got)
	}
	if rep.Ready || strings.Join(rep.Missing, ",") != "java,tofu" || strings.Join(rep.MissingRecommended, ",") != "actionlint" {
		t.Fatalf("missing = %v / %v", rep.Missing, rep.MissingRecommended)
	}
	byID := map[string]Item{}
	for _, it := range rep.Items {
		byID[it.ID] = it
	}
	tofu := byID["tofu"]
	if tofu.Recommended < 0 || !tofu.Methods[0].Auto || tofu.Methods[0].Command != "brew install opentofu" {
		t.Fatalf("tofu should offer one-click Homebrew: %+v", tofu.Methods)
	}
	for _, m := range tofu.Methods {
		if m.Manager == "snap" {
			t.Fatal("Linux-only methods are not offered on macOS")
		}
	}
	java := byID["java"]
	if java.Methods[java.Recommended].Auto {
		t.Fatal("the JDK cask asks for a password, so it is a copy-and-run command, not one click")
	}
	if al := byID["actionlint"]; al.Methods[al.Recommended].Manager != "brew" {
		t.Fatalf("actionlint prefers brew when both brew and go exist: %+v", al.Methods)
	}

	noBrew := newTestToolchain(fakeHost("darwin", map[string]bool{}, nil)).Report([]Requirement{{Tool: "terragrunt", Required: true}}, false)
	if it := noBrew.Items[0]; it.Recommended != -1 || !strings.Contains(it.Methods[0].Unavailable, "https://brew.sh") {
		t.Fatalf("without Homebrew the operator is pointed at brew.sh and the docs: %+v", it)
	}
	if all := tc.Report(nil, true); len(all.Items) != len(Catalog) || !all.Ready {
		t.Fatal("the catalog view lists every tool and nothing is required")
	}
}

func TestRequirementsMergeAcrossRepos(t *testing.T) {
	reqs := Requirements([]stack.Repo{
		{Name: "a", Profile: stack.Profile{}},
	})
	if len(reqs) != 0 {
		t.Fatalf("no stack, no tools: %v", reqs)
	}
}

// fakeExec returns a command that prints lines and exits with err.
func fakeExec(lines []string, err error, onRun func()) func(context.Context, []string) (io.ReadCloser, func() error, error) {
	return func(ctx context.Context, argv []string) (io.ReadCloser, func() error, error) {
		pr, pw := io.Pipe()
		done := make(chan error, 1)
		go func() {
			for _, l := range lines {
				_, _ = io.WriteString(pw, l+"\n")
			}
			if onRun != nil {
				onRun()
			}
			pw.Close()
			done <- err
		}()
		return pr, func() error { return <-done }, nil
	}
}

func waitJob(t *testing.T, in *Installer, id string) *Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if j := in.Job(id); j.State != JobRunning {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("install did not finish")
	return nil
}

func TestInstallerRunsVerifiesAndReports(t *testing.T) {
	bins := map[string]bool{"brew": true}
	tc := newTestToolchain(fakeHost("darwin", bins, nil))
	tc.Installer.exec = fakeExec([]string{"==> Downloading opentofu", "==> Pouring opentofu"}, nil, func() { bins["tofu"] = true })

	job, err := tc.Installer.Start("tofu", 0)
	if err != nil {
		t.Fatal(err)
	}
	done := waitJob(t, tc.Installer, job.ID)
	if done.State != JobSucceeded || done.Status == nil || !done.Status.Installed || !strings.Contains(done.Message, "OpenTofu is installed") {
		t.Fatalf("job = %+v", done)
	}
	if done.Log[0] != "$ brew install opentofu" || !strings.Contains(strings.Join(done.Log, "\n"), "Pouring opentofu") {
		t.Fatalf("log = %v", done.Log)
	}
	if tc.Installer.Latest("tofu").ID != job.ID {
		t.Fatal("the report shows the tool's latest job")
	}
}

func TestInstallerFailuresAreExplained(t *testing.T) {
	tc := newTestToolchain(fakeHost("darwin", map[string]bool{"brew": true}, nil))
	tc.Installer.exec = fakeExec([]string{"Error: No available formula"}, errors.New("exit status 1"), nil)
	job, _ := tc.Installer.Start("terragrunt", 0)
	if done := waitJob(t, tc.Installer, job.ID); done.State != JobFailed || !strings.Contains(done.Message, "install failed") {
		t.Fatalf("failed command: %+v", done)
	}

	// The command succeeds but the binary is still not visible: say how to fix PATH.
	tc.Installer.exec = fakeExec(nil, nil, nil)
	job, _ = tc.Installer.Start("zed", 0)
	if done := waitJob(t, tc.Installer, job.ID); done.State != JobFailed || !strings.Contains(done.Message, "PATH") {
		t.Fatalf("installed but invisible: %+v", done)
	}

	if _, err := tc.Installer.Start("java", 0); !errors.Is(err, ErrNotAuto) {
		t.Fatalf("a password-prompting method is never run by the orchestrator: %v", err)
	}
	if _, err := tc.Installer.Start("nope", 0); !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("unknown tool: %v", err)
	}
	if _, err := tc.Installer.Start("pnpm", 0); err == nil || !strings.Contains(err.Error(), "Node.js") {
		t.Fatalf("npm methods need Node first: %v", err)
	}

	block := make(chan struct{})
	tc.Installer.exec = fakeExec(nil, nil, func() { <-block })
	first, err := tc.Installer.Start("gradle", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tc.Installer.Start("gradle", 0); !errors.Is(err, ErrBusy) {
		t.Fatalf("a second install of the same tool waits: %v", err)
	}
	close(block)
	waitJob(t, tc.Installer, first.ID)
}

func TestCatalogIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range Catalog {
		if seen[tool.ID] {
			t.Errorf("duplicate tool %s", tool.ID)
		}
		seen[tool.ID] = true
		if tool.Docs == "" || !strings.HasPrefix(tool.Docs, "https://") {
			t.Errorf("%s needs an https docs link", tool.ID)
		}
		if (tool.Binary == "") == (len(tool.SDKEnv) == 0) {
			t.Errorf("%s is either a command or an SDK folder", tool.ID)
		}
		for _, m := range tool.Methods {
			if m.Auto && (m.Command[0] == "sudo" || m.Manager == "apt" || m.Manager == "snap") {
				t.Errorf("%s: a sudo method can never be one-click", tool.ID)
			}
		}
	}
	// Every tool a stack can ask for is in the catalog.
	for _, id := range []string{"go", "golangci-lint", "node", "pnpm", "yarn", "bun", "java", "maven", "gradle", "android-sdk",
		"flutter", "dart", "tofu", "terragrunt", "zed", "actionlint", "docker"} {
		if !seen[id] {
			t.Errorf("stack needs %s but the catalog has no entry", id)
		}
	}
}

func TestStartCommandStreamsBothOutputsAndExitCode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, wait, err := startCommand(ctx, []string{"sh", "-c", "echo out; echo err >&2; test -z \"$PS1\"; exit 3"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(out)
	if err := wait(); err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Fatalf("exit = %v", err)
	}
	if got := string(b); !strings.Contains(got, "out") || !strings.Contains(got, "err") {
		t.Fatalf("output = %q", got)
	}
	if _, _, err := startCommand(ctx, []string{"definitely-not-a-command-xyz"}); err == nil {
		t.Fatal("a missing binary fails to start")
	}
}

// A check that started before an install finished must not cache its stale answer over the
// install's invalidation.
func TestInvalidateWinsOverACheckInFlight(t *testing.T) {
	var mu sync.Mutex
	installed := false
	started, release := make(chan struct{}), make(chan struct{})
	h := fakeHost("darwin", map[string]bool{"golangci-lint": true}, nil)
	h.run = func(ctx context.Context, name string, args ...string) (string, error) {
		mu.Lock()
		ok := installed
		mu.Unlock()
		if !ok {
			close(started)
			<-release
			return "", errors.New("exit status 1")
		}
		return "golangci-lint 1.0\n", nil
	}
	tc := newTestToolchain(h)
	lint, _ := Lookup("golangci-lint")
	done := make(chan Status)
	go func() { done <- tc.Checker.Check(lint) }()
	<-started
	mu.Lock()
	installed = true // the install finishes while the old check still runs
	mu.Unlock()
	tc.Checker.Invalidate()
	close(release)
	if s := <-done; s.Installed {
		t.Fatalf("the in-flight check saw the old machine: %+v", s)
	}
	if s := tc.Checker.Check(lint); !s.Installed {
		t.Fatalf("a check after the invalidation reused the stale answer: %+v", s)
	}
}

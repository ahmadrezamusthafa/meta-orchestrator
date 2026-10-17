package security

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIsTestPath(t *testing.T) {
	for rel, want := range map[string]bool{
		"internal/fsm/state_test.go":         true,
		"web/src/views/Checkout.spec.ts":     true,
		"web/src/lib/money.test.tsx":         true,
		"tests/e2e/checkout.ts":              true,
		"web/__tests__/App.ts":               true,
		"e2e/login.ts":                       true,
		"internal/fsm/testdata/golden.json":  true,
		"spec/models/order_spec.rb":          true,
		"playwright.config.ts":               true,
		".sdlc/artifacts/TASK-1/notes.md":    true,
		"src/test/java/OrderTest.java":       true,
		"app/OrderServiceTest.java":          true,
		"tests_helper/conftest.py":           false,
		"internal/fsm/state.go":              false,
		"web/src/views/Checkout.vue":         false,
		"src/contest/latest.java":            false,
		"internal/specialist/handler.go":     false,
		"README.md":                          false,
		"web/src/testimonials/Testimony.vue": false,
	} {
		if got := IsTestPath(rel); got != want {
			t.Errorf("IsTestPath(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestCheckWritePermittedResolvesAgainstWorkspace(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "internal", "fsm"), 0o755)
	m := NewWriteLockManager()
	if err := m.CheckWritePermitted("T", "", "/anything.go"); err != nil {
		t.Fatalf("unlocked task must not be restricted: %v", err)
	}
	m.LockTask("T", root)
	for _, p := range []string{"internal/fsm/state.go", filepath.Join(root, "internal/fsm/state.go"), "../other/state_test.go",
		"tests/../internal/x.go", "/etc/passwd", "~/x_test.go"} {
		if err := m.CheckWritePermitted("T", "", p); !errors.Is(err, ErrWriteLockActive) {
			t.Errorf("write to %q should be refused, got %v", p, err)
		}
	}
	for _, p := range []string{"internal/fsm/state_test.go", filepath.Join(root, "tests", "e2e", "checkout.spec.ts"), ".sdlc/plan.md"} {
		if err := m.CheckWritePermitted("T", "", p); err != nil {
			t.Errorf("write to %q should be allowed: %v", p, err)
		}
	}
	if err := m.CheckWritePermitted("T", "tests", "helper.go"); err != nil {
		t.Errorf("relative to a test dir should be allowed: %v", err)
	}
	m.UnlockTask("T")
	if m.IsLocked("T") || m.CheckWritePermitted("T", "", "internal/fsm/state.go") != nil {
		t.Fatal("unlock should lift the lock")
	}
}

func TestInterceptToolCall(t *testing.T) {
	root := t.TempDir()
	m := NewWriteLockManager()
	in := NewPolyglotSkillInterceptor(m)
	m.LockTask("T", root)
	type call struct {
		tool  string
		input map[string]interface{}
	}
	bash := func(cmd string) call { return call{"Bash", map[string]interface{}{"command": cmd}} }
	deny := []call{
		{"Edit", map[string]interface{}{"file_path": filepath.Join(root, "src/App.vue")}},
		{"Write", map[string]interface{}{"file_path": "internal/api/router.go"}},
		{"NotebookEdit", map[string]interface{}{"notebook_path": "analysis.ipynb"}},
		{"mcp__filesystem__write_file", map[string]interface{}{"path": "src/views/Checkout.vue"}},
		{"Write", map[string]interface{}{}},
		bash("echo 'hack' > src/App.vue"),
		bash("npm test 2>&1 | tee build.log"),
		bash("sed -i '' 's/a/b/' internal/x.go"),
		bash("cd tests && rm ../main.go"),
		bash("mv src/a.go tests/a_test.go"),
		bash("cp tests/fixture.json internal/data.json"),
		bash("git checkout -- internal/api/router.go"),
		bash("git checkout ."),
		bash("git stash"),
		bash("find . -name '*.go' -delete"),
		bash("cat <<'EOF' >> pkg/x.go\nEOF"),
	}
	for _, c := range deny {
		if err := in.InterceptToolCall("T", c.tool, c.input); !errors.Is(err, ErrWriteLockActive) {
			t.Errorf("%s %v should be refused, got %v", c.tool, c.input, err)
		}
	}
	allow := []call{
		{"Write", map[string]interface{}{"file_path": filepath.Join(root, "tests/e2e/checkout.spec.ts")}},
		{"Edit", map[string]interface{}{"file_path": "internal/api/router_test.go"}},
		{"Read", map[string]interface{}{"file_path": "internal/api/router.go"}},
		bash("go test ./... 2>&1 | tail -50"),
		bash("npx playwright test > /dev/null 2>&1"),
		bash("echo '{}' > tests/fixtures/order.json && sed -i 's/a/b/' e2e/login.spec.ts"),
		bash("mkdir -p tests/e2e && cp fixtures/a.json tests/e2e/"),
		bash("git status && git diff -- internal/"),
		bash("grep -rn 'x > y' internal/"),
	}
	for _, c := range allow {
		if err := in.InterceptToolCall("T", c.tool, c.input); err != nil {
			t.Errorf("%s %v should be allowed: %v", c.tool, c.input, err)
		}
	}
	m.UnlockTask("T")
	if err := in.InterceptToolCall("T", "Write", map[string]interface{}{"file_path": "internal/api/router.go"}); err != nil {
		t.Fatalf("unlocked: %v", err)
	}
}

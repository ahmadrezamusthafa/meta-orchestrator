package uat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// SeedFence is the fenced block language of the test-data setup script the UAT stage emits.
const SeedFence = "uat-seed"

var seedBlock = regexp.MustCompile("(?s)```" + SeedFence + `(?:[ \t]+([A-Za-z0-9_+-]+))?[ \t]*\n(.*?)\n\s*` + "```")
var seedRun = regexp.MustCompile(`(?m)^\s*(?:#|//|--)\s*run:\s*(.+?)\s*$`)

// Seed is a script an engineer runs once in the UAT environment to create every test record the
// plan needs. It prints one NAME=value line per placeholder, which is pasted into the task's test
// data.
type Seed struct {
	Language string `json:"language,omitempty"`
	Run      string `json:"run,omitempty"` // the command from the script's "# run:" line
	Script   string `json:"script"`
}

// ExtractSeed finds the ```uat-seed block in the stage output.
func ExtractSeed(content string) (*Seed, bool) {
	m := seedBlock.FindStringSubmatch(content)
	if m == nil || strings.TrimSpace(m[2]) == "" {
		return nil, false
	}
	s := &Seed{Language: m[1], Script: m[2]}
	if r := seedRun.FindStringSubmatch(m[2]); r != nil {
		s.Run = r[1]
	}
	return s, true
}

// SeedApplyEnv is the switch a seed script must require before it writes anything. Without it the
// script runs as a dry run: every lookup and query executes, so a broken query fails before any
// record exists.
const SeedApplyEnv = "UAT_SEED_APPLY"

// SeedIssue is one problem found in a seed script without running it.
type SeedIssue struct {
	Severity string `json:"severity"` // "error" blocks running it; "warning" needs a human look
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

// SeedSyntax is the result of parsing (never executing) the script with its language's checker.
type SeedSyntax struct {
	Checked bool   `json:"checked"` // false when no checker is installed for the language
	OK      bool   `json:"ok"`
	Tool    string `json:"tool,omitempty"`
	Message string `json:"message,omitempty"`
}

// SeedCheck is everything the orchestrator can verify about a seed script before an engineer runs it.
type SeedCheck struct {
	Issues []SeedIssue `json:"issues"`
	Syntax SeedSyntax  `json:"syntax"`
}

// Safe reports whether nothing blocks running the script.
func (c SeedCheck) Safe() bool {
	if c.Syntax.Checked && !c.Syntax.OK {
		return false
	}
	for _, i := range c.Issues {
		if i.Severity == "error" {
			return false
		}
	}
	return true
}

var (
	prodGuard  = regexp.MustCompile(`(?i)production`)
	credential = regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|token|api[_-]?key|authorization)\b['"]?\s*(=>|=|:)\s*['"][^'"\s$#{}]{4,}['"]`)
	bulkWrite  = regexp.MustCompile(`\.(delete_all|destroy_all|update_all)\b|(?i)\b(truncate\s+table|drop\s+table|delete\s+from)\b`)
	joinsCall  = regexp.MustCompile(`\.(joins|includes|eager_load|left_joins|left_outer_joins)\(`)
	mergeCall  = regexp.MustCompile(`\.merge\(`)
	// A raw SQL fragment whose first column is not table-qualified: where('name LIKE ?').
	rawUnqualified = regexp.MustCompile(`\.(where|order|group|having|pluck)\(\s*['"]\s*[a-z_]+\s*(=|<|>|!=|<>|LIKE|like|IN|in|IS|is|ASC|asc|DESC|desc|,|['"])`)
)

// statements splits a script into logical statements: a line ending in "." or starting with "."
// continues the previous one, so a multi-line ActiveRecord chain is checked as one query.
func statements(script string) []struct {
	line int
	text string
} {
	var out []struct {
		line int
		text string
	}
	for i, l := range strings.Split(script, "\n") {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//") {
			continue
		}
		if n := len(out); n > 0 && (strings.HasPrefix(t, ".") || strings.HasSuffix(out[n-1].text, ".") ||
			strings.HasSuffix(out[n-1].text, "(") || strings.HasSuffix(out[n-1].text, ",") || strings.HasSuffix(out[n-1].text, "\\")) {
			out[n-1].text += " " + t
			continue
		}
		out = append(out, struct {
			line int
			text string
		}{i + 1, t})
	}
	return out
}

// Lint finds problems that make a seed script unsafe or likely to fail, without running it.
func (s *Seed) Lint() []SeedIssue {
	issues := []SeedIssue{}
	add := func(sev string, line int, msg string) {
		issues = append(issues, SeedIssue{Severity: sev, Line: line, Message: msg})
	}
	if !prodGuard.MatchString(s.Script) {
		add("error", 0, "no production guard: the script must abort when it detects a production environment before it touches any data")
	}
	if !strings.Contains(s.Script, SeedApplyEnv) {
		add("error", 0, "no dry-run switch: the script must only write when "+SeedApplyEnv+"=1 is set, and otherwise run every lookup and query read-only")
	}
	if s.Run == "" {
		add("warning", 0, "no \"# run: <command>\" line, so the panel cannot show how to run it")
	}
	for _, st := range statements(s.Script) {
		if m := credential.FindString(st.text); m != "" {
			add("error", st.line, "looks like a hard-coded credential; read it from ENV instead")
		}
		if bulkWrite.MatchString(st.text) {
			add("error", st.line, "bulk write or delete: a seed script may only create and change the records it owns, one by one")
		}
		if joinsCall.MatchString(st.text) && mergeCall.MatchString(st.text) {
			add("warning", st.line, "merges another model's scope into a join: if that scope uses raw SQL with an unqualified column (e.g. where('name LIKE ?')), "+
				"MySQL fails with \"Column … is ambiguous\". Use a table-qualified condition instead")
		}
		if joinsCall.MatchString(st.text) && rawUnqualified.MatchString(st.text) {
			add("error", st.line, "raw SQL column without its table name in a joined query: qualify it (e.g. 'apps.name LIKE ?') or use a hash / Arel condition")
		}
	}
	return issues
}

// syntaxTools parses a script without running it.
var syntaxTools = map[string][]string{
	"ruby":   {"ruby", "-c"},
	"rb":     {"ruby", "-c"},
	"python": {"python3", "-m", "py_compile"},
	"py":     {"python3", "-m", "py_compile"},
	"bash":   {"bash", "-n"},
	"sh":     {"bash", "-n"},
	"js":     {"node", "--check"},
	"node":   {"node", "--check"},
}

var seedExt = map[string]string{"ruby": ".rb", "rb": ".rb", "python": ".py", "py": ".py", "bash": ".sh", "sh": ".sh", "js": ".mjs", "node": ".mjs"}

// CheckSyntax parses the script with its language's checker. It never executes the script.
func (s *Seed) CheckSyntax(ctx context.Context) SeedSyntax {
	lang := strings.ToLower(s.Language)
	tool, ok := syntaxTools[lang]
	if !ok {
		return SeedSyntax{Message: fmt.Sprintf("no syntax checker for %q", s.Language)}
	}
	if _, err := exec.LookPath(tool[0]); err != nil {
		return SeedSyntax{Tool: tool[0], Message: tool[0] + " is not installed on the orchestrator host"}
	}
	dir, err := os.MkdirTemp("", "uat-seed-")
	if err != nil {
		return SeedSyntax{Message: err.Error()}
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "uat_seed"+seedExt[lang])
	if err := os.WriteFile(path, []byte(s.Script), 0o600); err != nil {
		return SeedSyntax{Message: err.Error()}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool[0], append(tool[1:], path)...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	runErr := cmd.Run()
	msg := strings.TrimSpace(strings.ReplaceAll(out.String(), path, "uat_seed"+seedExt[lang]))
	if len(msg) > 600 {
		msg = msg[:600] + "…"
	}
	res := SeedSyntax{Checked: true, OK: runErr == nil, Tool: strings.Join(tool, " "), Message: msg}
	if runErr != nil && msg == "" {
		res.Message = runErr.Error()
	}
	return res
}

var seedChecks sync.Map // sha256(language+script) → SeedCheck

// Check lints and syntax-checks the script, caching the result per script content.
func (s *Seed) Check(ctx context.Context) SeedCheck {
	sum := sha256.Sum256([]byte(s.Language + "\x00" + s.Script))
	key := hex.EncodeToString(sum[:])
	if c, ok := seedChecks.Load(key); ok {
		return c.(SeedCheck)
	}
	c := SeedCheck{Issues: s.Lint(), Syntax: s.CheckSyntax(ctx)}
	seedChecks.Store(key, c)
	return c
}

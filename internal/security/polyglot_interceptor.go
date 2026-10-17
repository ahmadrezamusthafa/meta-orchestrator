package security

import (
	"fmt"
	"path"
	"strings"
)

// PolyglotSkillInterceptor enforces a task's write lock on the agent's tool calls: native file
// tools (Edit, Write, MultiEdit, NotebookEdit), MCP file-writing tools and shell commands.
type PolyglotSkillInterceptor struct {
	lockMgr *WriteLockManager
}

// NewPolyglotSkillInterceptor creates an interceptor.
func NewPolyglotSkillInterceptor(lockMgr *WriteLockManager) *PolyglotSkillInterceptor {
	return &PolyglotSkillInterceptor{lockMgr: lockMgr}
}

// fileWriteTools are native tools that write the file named in their input.
var fileWriteTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true}

// mcpWriteSuffixes name MCP tools (mcp__<server>__<tool>) that write a file.
var mcpWriteSuffixes = []string{"write_file", "edit_file", "write_to_file", "replace_file_content", "create_file", "move_file", "delete_file"}

// IsFileWrite reports whether the tool call writes a file named in its input.
func IsFileWrite(toolName string) bool {
	if fileWriteTools[toolName] {
		return true
	}
	lower := strings.ToLower(toolName)
	for _, s := range mcpWriteSuffixes {
		if lower == s || strings.HasSuffix(lower, "__"+s) {
			return true
		}
	}
	return false
}

func inputPaths(input map[string]interface{}) []string {
	var out []string
	for _, k := range []string{"file_path", "notebook_path", "path", "target_file", "destination", "source"} {
		if v, ok := input[k].(string); ok && v != "" {
			out = append(out, v)
		}
	}
	return out
}

// InterceptToolCall checks one tool call against the task's write lock. It returns an error
// wrapping ErrWriteLockActive when the call would modify a protected file, and nil otherwise
// (including when the task is not locked).
func (i *PolyglotSkillInterceptor) InterceptToolCall(taskID, toolName string, input map[string]interface{}) error {
	if !i.lockMgr.IsLocked(taskID) {
		return nil
	}
	if IsFileWrite(toolName) {
		paths := inputPaths(input)
		if len(paths) == 0 {
			return fmt.Errorf("%w: %s names no file to check", ErrWriteLockActive, toolName)
		}
		for _, p := range paths {
			if err := i.lockMgr.CheckWritePermitted(taskID, "", p); err != nil {
				return err
			}
		}
		return nil
	}
	if toolName == "Bash" {
		cmd, _ := input["command"].(string)
		return i.checkShell(taskID, cmd)
	}
	return nil
}

// checkShell refuses a shell command that writes, moves or deletes non-test files. It follows cd
// within the command so relative paths resolve where the command runs them.
func (i *PolyglotSkillInterceptor) checkShell(taskID, cmd string) error {
	dir := ""
	for _, seg := range shellSegments(cmd) {
		targets, wholeTree, next := writeTargets(seg, dir)
		dir = next
		if wholeTree != "" {
			return fmt.Errorf("%w: `%s` %s", ErrWriteLockActive, strings.Join(seg, " "), wholeTree)
		}
		for _, t := range targets {
			if t == "/dev/null" || t == "/dev/stdout" || t == "/dev/stderr" {
				continue
			}
			if err := i.lockMgr.CheckWritePermitted(taskID, dir, t); err != nil {
				return fmt.Errorf("%w (shell command `%s`)", err, strings.TrimSpace(cmd))
			}
		}
	}
	return nil
}

// shellSegments splits a command into simple commands (on ; && || | & and newlines), each a list of
// unquoted words. Redirection operators are kept as their own words.
func shellSegments(cmd string) [][]string {
	var segs [][]string
	var words []string
	var cur strings.Builder
	inWord := false
	flushWord := func() {
		if inWord {
			words = append(words, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	flushSeg := func() {
		flushWord()
		if len(words) > 0 {
			segs = append(segs, words)
		}
		words = nil
	}
	rs := []rune(cmd)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\'':
			inWord = true
			for i++; i < len(rs) && rs[i] != '\''; i++ {
				cur.WriteRune(rs[i])
			}
		case c == '"':
			inWord = true
			for i++; i < len(rs) && rs[i] != '"'; i++ {
				if rs[i] == '\\' && i+1 < len(rs) {
					i++
				}
				cur.WriteRune(rs[i])
			}
		case c == '\\' && i+1 < len(rs):
			inWord = true
			i++
			cur.WriteRune(rs[i])
		case c == ';' || c == '\n' || c == '|' || (c == '&' && !(i+1 < len(rs) && rs[i+1] == '>')):
			flushSeg()
		case c == '>':
			// Keep fd prefixes (2>, &>) and >> together as one operator word.
			op := ">"
			if inWord && (cur.String() == "1" || cur.String() == "2" || cur.String() == "&") {
				op = cur.String() + ">"
				cur.Reset()
				inWord = false
			}
			flushWord()
			if i+1 < len(rs) && rs[i+1] == '>' {
				op += ">"
				i++
			}
			if i+1 < len(rs) && rs[i+1] == '&' { // >&2 duplicates a descriptor, writes no file
				i++
				for i+1 < len(rs) && rs[i+1] >= '0' && rs[i+1] <= '9' {
					i++
				}
				continue
			}
			words = append(words, op)
		case c == ' ' || c == '\t':
			flushWord()
		default:
			if c == '&' { // &> redirect: the > case handles it next
				flushWord()
				cur.WriteRune(c)
				inWord = true
				continue
			}
			inWord = true
			cur.WriteRune(c)
		}
	}
	flushSeg()
	return segs
}

func isRedirect(w string) bool {
	return w == ">" || w == ">>" || w == "1>" || w == "2>" || w == "&>" || w == "1>>" || w == "2>>" || w == "&>>"
}

func nonFlags(args []string) []string {
	var out []string
	dashdash := false
	for _, a := range args {
		if !dashdash && a == "--" {
			dashdash = true
			continue
		}
		if a == "" || (!dashdash && strings.HasPrefix(a, "-")) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// writeTargets lists the files a simple command writes. wholeTree explains a command that changes
// the working tree in a way that cannot be narrowed to files. next is the directory for the
// following segment (after a cd).
func writeTargets(words []string, dir string) (targets []string, wholeTree, next string) {
	next = dir
	var args []string
	for i := 0; i < len(words); i++ {
		if isRedirect(words[i]) {
			if i+1 < len(words) {
				targets = append(targets, words[i+1])
				i++
			}
			continue
		}
		args = append(args, words[i])
	}
	for len(args) > 0 && strings.Contains(args[0], "=") && !strings.HasPrefix(args[0], "-") { // FOO=bar cmd
		args = args[1:]
	}
	for len(args) > 0 && (args[0] == "sudo" || args[0] == "command" || args[0] == "env" || args[0] == "exec") {
		args = args[1:]
	}
	if len(args) == 0 {
		return targets, "", next
	}
	name, rest := path.Base(args[0]), args[1:]
	switch name {
	case "cd", "pushd":
		if p := nonFlags(rest); len(p) > 0 {
			if path.IsAbs(p[0]) || dir == "" {
				next = p[0]
			} else {
				next = path.Join(dir, p[0])
			}
		}
	case "rm", "rmdir", "touch", "truncate", "shred", "unlink", "mkdir":
		targets = append(targets, nonFlags(rest)...)
	case "mv":
		targets = append(targets, nonFlags(rest)...) // the sources disappear too
	case "cp", "install", "ln", "rsync":
		if p := nonFlags(rest); len(p) > 0 {
			targets = append(targets, p[len(p)-1])
		}
	case "tee":
		targets = append(targets, nonFlags(rest)...)
	case "sed", "perl":
		inPlace := false
		for _, a := range rest {
			if a == "--in-place" || strings.HasPrefix(a, "--in-place=") || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "i")) {
				inPlace = true
			}
		}
		if inPlace {
			if p := nonFlags(rest); len(p) > 1 {
				targets = append(targets, p[1:]...) // the first operand is the script
			} else {
				wholeTree = "edits files in place"
			}
		}
	case "git":
		sub := nonFlags(rest)
		if len(sub) == 0 {
			break
		}
		switch sub[0] {
		case "checkout", "restore":
			idx := -1
			for i, a := range rest {
				if a == "--" {
					idx = i
				}
			}
			newBranch := false
			for _, a := range rest {
				newBranch = newBranch || a == "-b" || a == "-B"
			}
			switch {
			case idx >= 0:
				targets = append(targets, rest[idx+1:]...)
			case !newBranch:
				// Without "--" an operand is a path or a ref; switching refs rewrites source too.
				targets = append(targets, sub[1:]...)
			}
			for _, t := range targets {
				if t == "." || t == "*" {
					wholeTree = "restores files across the working tree"
				}
			}
		case "reset":
			for _, a := range rest {
				if a == "--hard" || a == "--merge" || a == "--keep" {
					wholeTree = "resets the working tree"
				}
			}
		case "clean", "stash", "apply", "am", "merge", "rebase", "cherry-pick", "revert", "pull":
			wholeTree = "changes files across the working tree"
		}
	case "patch":
		wholeTree = "patches files that cannot be checked in advance"
	case "xargs", "find":
		for _, a := range rest {
			base := path.Base(a)
			if a == "-delete" || base == "rm" || base == "mv" || base == "cp" || base == "sed" || base == "tee" || base == "truncate" {
				wholeTree = "modifies files chosen at run time"
			}
		}
	}
	return targets, wholeTree, next
}

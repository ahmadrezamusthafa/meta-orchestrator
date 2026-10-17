package stack

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// trackerPatterns recognise links to a project-management tool, or the task IDs its git
// integration puts in branch names and commit messages.
var trackerPatterns = []struct {
	id string
	re *regexp.Regexp
}{
	{"clickup", regexp.MustCompile(`(?i)app\.clickup\.com/|sharing\.clickup\.com/|\bCU-[0-9a-z]{6,12}\b`)},
	{"trello", regexp.MustCompile(`(?i)trello\.com/[bc]/[A-Za-z0-9]{8}`)},
	{"asana", regexp.MustCompile(`(?i)app\.asana\.com/\d+/`)},
	{"monday", regexp.MustCompile(`(?i)[a-z0-9-]+\.monday\.com/boards/\d+`)},
	{"linear", regexp.MustCompile(`(?i)linear\.app/[a-z0-9-]+/(issue|project)/`)},
	{"notion", regexp.MustCompile(`(?i)www\.notion\.so/|[a-z0-9-]+\.notion\.site/`)},
}

// trackerFile reports whether a file is where a team links its task board: the README, the
// contribution guide, PR/MR templates, a commit template or a CI workflow.
func trackerFile(rel string) bool {
	base := strings.ToLower(path.Base(rel))
	dir := strings.ToLower(path.Dir(rel))
	switch {
	case dir == "." && (strings.HasPrefix(base, "readme") || strings.HasPrefix(base, "contributing")):
		return true
	case strings.Contains(base, "pull_request_template") || strings.Contains(base, "merge_request_template"):
		return true
	case base == ".gitmessage" || base == "bitbucket-pipelines.yml" || base == ".gitlab-ci.yml":
		return true
	case strings.HasPrefix(dir, ".github"):
		return strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".md")
	}
	return false
}

// trackerRefs lists the project-management tools the repository links to, from those files and
// the subjects of recent commits and checkouts in its reflog (read from disk, git is not run).
func trackerRefs(root string, files []string, reader *contentReader) []string {
	var texts []string
	for _, rel := range files {
		if trackerFile(rel) {
			texts = append(texts, reader.read(rel))
		}
	}
	for _, rel := range []string{".gitmessage", ".github/pull_request_template.md"} {
		texts = append(texts, reader.read(rel)) // hidden files the scan skips
	}
	texts = append(texts, recentReflog(root))

	var found []string
	for _, p := range trackerPatterns {
		for _, t := range texts {
			if p.re.MatchString(t) {
				found = append(found, p.id)
				break
			}
		}
	}
	return found
}

// recentReflog returns the end of HEAD's reflog: its lines carry commit subjects and the names of
// branches checked out ("checkout: moving from main to CU-86c2xyz-fix").
func recentReflog(root string) string {
	gitdir, _, ok := gitDirs(root)
	if !ok {
		return ""
	}
	f, err := os.Open(filepath.Join(gitdir, "logs", "HEAD"))
	if err != nil {
		return ""
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > maxRead {
		_, _ = f.Seek(st.Size()-maxRead, io.SeekStart)
	}
	b, _ := io.ReadAll(io.LimitReader(f, maxRead))
	var subjects strings.Builder
	for _, line := range strings.Split(string(b), "\n") {
		// "<old> <new> <author> <time> <tz>\t<message>": keep only the message, never the author.
		if i := strings.IndexByte(line, '\t'); i >= 0 {
			subjects.WriteString(line[i+1:])
			subjects.WriteByte('\n')
		}
	}
	return subjects.String()
}

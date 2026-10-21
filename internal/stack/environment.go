package stack

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Production can differ from the machine the agent runs on: a database reached through a firewall,
// a gateway in front of the API, managed identity instead of a local secret. The brief points the
// agent at the repository's architecture documents and passes on the constraints the operator
// wrote in .sdlc/constraints.md, so a change made to work locally does not break those paths.

// ConstraintsFile is the operator's list of environment facts, relative to the repository root.
var ConstraintsFile = filepath.Join(".sdlc", "constraints.md")

const (
	maxConstraints = 4 << 10 // bytes of ConstraintsFile passed to the agent
	maxArchDocs    = 3
)

func isCompose(base string) bool {
	for _, g := range composeFiles {
		if ok, _ := path.Match(g, base); ok {
			return true
		}
	}
	return false
}

// isArchDoc matches ARCHITECTURE.md and architecture*.md at the root or in a top-level folder
// (docs/architecture.md), whatever the case.
func isArchDoc(rel string) bool {
	base := strings.ToLower(path.Base(rel))
	return depth(dirOf(rel)) <= 1 && strings.HasPrefix(base, "architecture") && strings.HasSuffix(base, ".md")
}

func readConstraints(root string) string {
	f, err := os.Open(filepath.Join(root, ConstraintsFile))
	if err != nil {
		return ""
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, maxConstraints))
	return strings.TrimSpace(string(b))
}

// environmentBrief is the brief's section on what differs outside this machine.
func environmentBrief(repos []Repo) string {
	var docs, rules []string
	for _, r := range repos {
		for _, d := range r.Profile.archDocs {
			docs = append(docs, r.Name+"/"+d)
		}
		if c := r.Profile.constraints; c != "" {
			rules = append(rules, "From "+r.Name+"/"+filepath.ToSlash(ConstraintsFile)+":\n"+c)
		}
	}
	if len(docs) == 0 && len(rules) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Environments\nProduction can reach things differently from this machine (proxies, firewalls, gateways, managed identity). " +
		"Keep each environment's connection settings in its own configuration; never change a production value to match local, and never add a direct path around one.\n")
	if len(docs) > 0 {
		b.WriteString("Read before changing configuration, networking or data access: " + strings.Join(docs, ", ") + "\n")
	}
	if len(rules) > 0 {
		b.WriteString("Constraints set by the operator — follow them over anything local suggests:\n\n" + strings.Join(rules, "\n\n") + "\n")
	}
	return b.String()
}

package stack

import (
	"bufio"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// gitHosts reads the hosts of the repository's remotes (github.com, bitbucket.org, …) from its git
// config without running git. A worktree's .git file points at its main repository's config.
func gitHosts(root string) []string {
	gitPath := filepath.Join(root, ".git")
	st, err := os.Stat(gitPath)
	if err != nil {
		return nil
	}
	config := filepath.Join(gitPath, "config")
	if !st.IsDir() {
		b, err := os.ReadFile(gitPath)
		if err != nil {
			return nil
		}
		gitdir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(b)), "gitdir:"))
		if !filepath.IsAbs(gitdir) {
			gitdir = filepath.Join(root, gitdir)
		}
		common := gitdir
		if c, err := os.ReadFile(filepath.Join(gitdir, "commondir")); err == nil {
			common = strings.TrimSpace(string(c))
			if !filepath.IsAbs(common) {
				common = filepath.Join(gitdir, common)
			}
		}
		config = filepath.Join(common, "config")
	}
	f, err := os.Open(config)
	if err != nil {
		return nil
	}
	defer f.Close()
	var hosts []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "url") {
			continue
		}
		if i := strings.Index(line, "="); i > 0 && strings.TrimSpace(line[:i]) == "url" {
			if h := remoteHost(strings.TrimSpace(line[i+1:])); h != "" {
				addUnique(&hosts, h, maxDirs)
			}
		}
	}
	return hosts
}

// remoteHost extracts the host from https://host/…, ssh://git@host/… or scp-style git@host:org/repo.
func remoteHost(remote string) string {
	if strings.Contains(remote, "://") {
		if u, err := url.Parse(remote); err == nil {
			return strings.ToLower(u.Hostname())
		}
		return ""
	}
	if at := strings.Index(remote, "@"); at >= 0 {
		remote = remote[at+1:]
	}
	if colon := strings.Index(remote, ":"); colon > 0 {
		return strings.ToLower(remote[:colon])
	}
	return ""
}

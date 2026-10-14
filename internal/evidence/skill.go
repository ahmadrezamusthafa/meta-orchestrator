package evidence

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The built-in test evidence skill: SKILL.md plus the recorder script, written to disk so the
// agent can read the instructions and run the recorder like any skill from a library.

//go:embed skill
var skillFS embed.FS

// SkillName is the built-in evidence skill, used when the operator picks no skill of their own.
const SkillName = "test-evidence-video"

// SkillDescription is the one-line purpose of the built-in skill.
const SkillDescription = "Built in: runs every affected test suite for real, then records one narrated video per test case " +
	"(test commands with real output, browser walkthroughs of the running app) plus evidence.json."

// RecorderScript is the recorder's path inside the installed skill directory.
const RecorderScript = "scripts/record-evidence.mjs"

// SkillInstructions is the body of the built-in SKILL.md (frontmatter removed).
func SkillInstructions() string {
	raw, _ := skillFS.ReadFile("skill/SKILL.md")
	s := string(raw)
	if strings.HasPrefix(s, "---\n") {
		if end := strings.Index(s[4:], "\n---\n"); end >= 0 {
			s = s[4+end+5:]
		}
	}
	return strings.TrimSpace(s)
}

// InstallSkill writes the built-in skill into dir, replacing files whose content changed.
func InstallSkill(dir string) error {
	return fs.WalkDir(skillFS, "skill", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(p, "skill"), "/")))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := skillFS.ReadFile(p)
		if err != nil {
			return err
		}
		if cur, err := os.ReadFile(target); err == nil && bytes.Equal(cur, data) {
			return nil
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// ManifestFile is the evidence manifest inside the evidence directory.
const ManifestFile = "evidence.json"

// Part is one step of a recorded case.
type Part struct {
	Action   string `json:"action"`
	Caption  string `json:"caption,omitempty"`
	Status   string `json:"status,omitempty"`
	Error    string `json:"error,omitempty"`
	Command  string `json:"command,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Seconds  int    `json:"seconds,omitempty"`
	Log      string `json:"log,omitempty"`
	URL      string `json:"url,omitempty"`
}

// Case is one test case and its video.
type Case struct {
	ID         string   `json:"id"`
	Title      string   `json:"title,omitempty"`
	Covers     []string `json:"covers,omitempty"`
	Expected   string   `json:"expected,omitempty"`
	Status     string   `json:"status"` // passed, failed, blocked
	Video      string   `json:"video,omitempty"`
	Poster     string   `json:"poster,omitempty"`
	Notes      string   `json:"notes,omitempty"`
	Parts      []Part   `json:"parts,omitempty"`
	RecordedAt string   `json:"recorded_at,omitempty"`
	// VideoMissing is set by Collect when the manifest names a video that is not in the directory.
	VideoMissing bool `json:"video_missing,omitempty"`
}

// Suite is one full test-suite run of a repository.
type Suite struct {
	Repository string `json:"repository"`
	Command    string `json:"command,omitempty"`
	Status     string `json:"status"`
	Passed     *int   `json:"passed,omitempty"` // nil when the run did not report counts
	Failed     *int   `json:"failed,omitempty"`
	Log        string `json:"log,omitempty"`
}

// Manifest is evidence.json: written by the recorder, or by the agent for a library skill.
type Manifest struct {
	Title       string  `json:"title,omitempty"`
	GeneratedBy string  `json:"generated_by,omitempty"`
	UpdatedAt   string  `json:"updated_at,omitempty"`
	Cases       []Case  `json:"cases"`
	Suites      []Suite `json:"suites,omitempty"`
}

// Report is what an evidence directory holds.
type Report struct {
	Manifest      *Manifest `json:"manifest,omitempty"`
	ManifestError string    `json:"manifest_error,omitempty"`
	Videos        []string  `json:"videos"` // paths relative to the evidence directory
	Passed        int       `json:"passed"`
	Failed        int       `json:"failed"` // failed or blocked cases, and failed suites
}

// Status summarizes the report: "missing" (no video), "failed" or "passed".
func (r Report) Status() string {
	switch {
	case len(r.Videos) == 0:
		return "missing"
	case r.Failed > 0:
		return "failed"
	default:
		return "passed"
	}
}

var videoExts = map[string]bool{".mp4": true, ".webm": true, ".mov": true}

// IsVideo reports whether name is a video file the evidence view can play.
func IsVideo(name string) bool { return videoExts[strings.ToLower(filepath.Ext(name))] }

// cleanRel keeps a manifest path only when it stays inside the evidence directory.
func cleanRel(p string) string {
	p = filepath.ToSlash(filepath.Clean(strings.TrimSpace(p)))
	if p == "." || p == "" || strings.HasPrefix(p, "../") || p == ".." || strings.HasPrefix(p, "/") || strings.Contains(p, "\x00") {
		return ""
	}
	return p
}

// Collect reads the evidence directory: every video in it, and the manifest when there is one.
func Collect(dir string) Report {
	rep := Report{Videos: []string{}}
	present := map[string]bool{}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && p != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && IsVideo(d.Name()) {
			rel, _ := filepath.Rel(dir, p)
			rel = filepath.ToSlash(rel)
			present[rel] = true
			rep.Videos = append(rep.Videos, rel)
		}
		return nil
	})
	sort.Strings(rep.Videos)

	raw, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		return rep
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		rep.ManifestError = "evidence.json is not valid JSON: " + err.Error()
		return rep
	}
	for i := range m.Cases {
		c := &m.Cases[i]
		c.Video, c.Poster = cleanRel(c.Video), cleanRel(c.Poster)
		c.VideoMissing = c.Video != "" && !present[c.Video]
		if c.Status == "passed" {
			rep.Passed++
		} else {
			rep.Failed++
		}
	}
	for _, s := range m.Suites {
		if s.Status != "passed" {
			rep.Failed++
		}
	}
	rep.Manifest = &m
	return rep
}

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
const SkillDescription = "Built in: proves the feature works as specified, end to end — plans the evidence from the spec " +
	"(user journeys, acceptance test cases), then records a narrated video per end-to-end flow and per acceptance case in the running " +
	"system (browser walkthroughs and API calls, every action and check annotated), the full test suites as regression, and the spec coverage."

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
	Note     string `json:"note,omitempty"`   // what the step proves
	Status   string `json:"status,omitempty"` // passed, failed, skipped (not run after a failure), info (a note card)
	Verified bool   `json:"verified,omitempty"`
	Error    string `json:"error,omitempty"`
	Command  string `json:"command,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Seconds  int    `json:"seconds,omitempty"`
	Summary  string `json:"summary,omitempty"` // e.g. test counts, or the HTTP status of an API call
	Log      string `json:"log,omitempty"`
	URL      string `json:"url,omitempty"`
	Method   string `json:"method,omitempty"`
	// HTTPStatus is the response status of an API step.
	HTTPStatus int `json:"http_status,omitempty"`
	// At is when the step starts in the case's video, in seconds.
	At *float64 `json:"at,omitempty"`
	// Shot is the annotated screenshot of the step, relative to the evidence directory.
	Shot string `json:"shot,omitempty"`
}

// Repo is a repository under test: the commit the evidence was recorded on.
type Repo struct {
	Name        string `json:"name"`
	Branch      string `json:"branch,omitempty"`
	Commit      string `json:"commit,omitempty"`
	Uncommitted bool   `json:"uncommitted,omitempty"`
}

// Environment is what a case was recorded against.
type Environment struct {
	BaseURL  string `json:"base_url,omitempty"`
	Browser  string `json:"browser,omitempty"`
	Viewport string `json:"viewport,omitempty"`
	Repos    []Repo `json:"repos,omitempty"`
}

// Case kinds: what a case proves.
const (
	KindFlow       = "flow"       // a complete user journey of the feature, end to end
	KindAcceptance = "acceptance" // one acceptance test case from the spec
	KindSuite      = "suite"      // a full test-suite run, as regression
)

// Case is one test case and its video.
type Case struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind,omitempty"`
	Title      string   `json:"title,omitempty"`
	Covers     []string `json:"covers,omitempty"`
	Given      string   `json:"given,omitempty"`
	Expected   string   `json:"expected,omitempty"`
	Status     string   `json:"status"` // passed, failed, blocked
	Video      string   `json:"video,omitempty"`
	Poster     string   `json:"poster,omitempty"`
	Notes      string   `json:"notes,omitempty"`
	Parts      []Part   `json:"parts,omitempty"`
	RecordedAt string   `json:"recorded_at,omitempty"`
	DurationS  int      `json:"duration_s,omitempty"`
	// ResultAt is when the result card starts in the video, in seconds.
	ResultAt    *float64     `json:"result_at,omitempty"`
	Environment *Environment `json:"environment,omitempty"`
	// Issues are the browser's console errors and failing requests during the case.
	Issues []string `json:"issues,omitempty"`
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
	Skipped    *int   `json:"skipped,omitempty"`
	Seconds    int    `json:"seconds,omitempty"`
	Log        string `json:"log,omitempty"`
	Case       string `json:"case,omitempty"` // the case whose video shows this run
}

// Criterion is an acceptance criterion the evidence must prove, listed in evidence.json when the
// task has no ATDD cases (the agent derives them from the PRD).
type Criterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Chapter is where a part of the presentation starts, in seconds.
type Chapter struct {
	ID    string  `json:"id"`
	Title string  `json:"title"`
	At    float64 `json:"at"`
}

// Summary is the presentation video: the whole evidence in one video — verdict, spec coverage, a
// chapter per case with its key steps, and the results.
type Summary struct {
	Video      string    `json:"video,omitempty"`
	Poster     string    `json:"poster,omitempty"`
	DurationS  int       `json:"duration_s,omitempty"`
	Chapters   []Chapter `json:"chapters,omitempty"`
	RecordedAt string    `json:"recorded_at,omitempty"`
	// VideoMissing is set by Collect when the video is not in the directory.
	VideoMissing bool `json:"video_missing,omitempty"`
}

// SpecFile lists the acceptance cases the evidence must prove, written by the orchestrator for
// the recorder's presentation.
const SpecFile = "spec.json"

// Manifest is evidence.json: written by the recorder, or by the agent for a library skill.
type Manifest struct {
	Title       string      `json:"title,omitempty"`
	Criteria    []Criterion `json:"criteria,omitempty"`
	GeneratedBy string      `json:"generated_by,omitempty"`
	UpdatedAt   string      `json:"updated_at,omitempty"`
	Cases       []Case      `json:"cases"`
	Suites      []Suite     `json:"suites,omitempty"`
	Summary     *Summary    `json:"summary,omitempty"`
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

// KindOf infers a case's kind from its id: SUITE-… is a suite run, FLOW-… a user journey, and
// anything else an acceptance case.
func KindOf(id string) string {
	switch u := strings.ToUpper(id); {
	case strings.HasPrefix(u, "SUITE"):
		return KindSuite
	case strings.HasPrefix(u, "FLOW"):
		return KindFlow
	}
	return KindAcceptance
}

// Proves reports whether the case is evidence for the spec item id: its own id, or one it covers.
func (c Case) Proves(id string) bool {
	if strings.EqualFold(c.ID, id) {
		return true
	}
	for _, v := range c.Covers {
		if strings.EqualFold(strings.TrimSpace(v), id) {
			return true
		}
	}
	return false
}

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
		if c.Kind == "" {
			c.Kind = KindOf(c.ID)
		}
		c.VideoMissing = c.Video != "" && !present[c.Video]
		for j := range c.Parts {
			c.Parts[j].Shot, c.Parts[j].Log = cleanRel(c.Parts[j].Shot), cleanRel(c.Parts[j].Log)
		}
		if c.Status == "passed" {
			rep.Passed++
		} else {
			rep.Failed++
		}
	}
	for i := range m.Suites {
		s := &m.Suites[i]
		s.Log = cleanRel(s.Log)
		if s.Status != "passed" {
			rep.Failed++
		}
	}
	if sm := m.Summary; sm != nil {
		sm.Video, sm.Poster = cleanRel(sm.Video), cleanRel(sm.Poster)
		sm.VideoMissing = sm.Video != "" && !present[sm.Video]
	}
	rep.Manifest = &m
	return rep
}

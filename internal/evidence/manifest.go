package evidence

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The signed evidence manifest: a SHA-256 of every file in the evidence directory and the real
// results from evidence.json, signed with HMAC-SHA256. Verify recomputes both and names every file
// that changed, disappeared or appeared since it was signed. HMAC proves the manifest was written
// by a holder of the key; anyone with the key (the orchestrator's host) can re-sign.

// Signed manifest files, written into the evidence directory and excluded from the file list.
const (
	SignedManifestFile = "evidence-manifest.json"
	ManifestMarkdown   = "EVIDENCE.md"
)

// SigningKeyEnv overrides the signing key; otherwise a random key is kept at SigningKeyFile.
const SigningKeyEnv = "MO_EVIDENCE_SIGNING_KEY"

// SigningKeyFile is the generated key's path under the orchestrator root.
var SigningKeyFile = filepath.Join(".sdlc", "evidence-signing.key")

// FileDigest is one file of the evidence directory.
type FileDigest struct {
	Path   string `json:"path"` // relative, slash-separated
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Kind   string `json:"kind"` // video, screenshot, manifest, other
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// CaseResult is one case as evidence.json reports it.
type CaseResult struct {
	ID     string `json:"id"`
	Kind   string `json:"kind,omitempty"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status"`
	Video  string `json:"video,omitempty"`
}

// SuiteResult is one suite run as evidence.json reports it.
type SuiteResult struct {
	Repository string `json:"repository"`
	Command    string `json:"command,omitempty"`
	Status     string `json:"status"`
	Passed     *int   `json:"passed,omitempty"`
	Failed     *int   `json:"failed,omitempty"`
}

// ManifestBody is the signed content.
type ManifestBody struct {
	Version       int           `json:"version"`
	TaskID        string        `json:"task_id"`
	GeneratedAt   string        `json:"generated_at"`
	Status        string        `json:"status"` // Report.Status: passed, failed, missing
	Passed        int           `json:"passed"`
	Failed        int           `json:"failed"`
	ManifestError string        `json:"manifest_error,omitempty"`
	Cases         []CaseResult  `json:"cases"`
	Suites        []SuiteResult `json:"suites,omitempty"`
	Files         []FileDigest  `json:"files"`
}

// SignedManifest is evidence-manifest.json.
type SignedManifest struct {
	Body      ManifestBody `json:"body"`
	Algorithm string       `json:"algorithm"`
	Signature string       `json:"signature"`
}

// EvidenceManifestBuilder builds, signs and verifies evidence manifests.
type EvidenceManifestBuilder struct {
	secretKey []byte
	shots     *ScreenshotService
}

// NewEvidenceManifestBuilder creates a builder with the given key; an empty key is refused.
func NewEvidenceManifestBuilder(secretKey []byte) (*EvidenceManifestBuilder, error) {
	if len(secretKey) == 0 {
		return nil, errors.New("evidence signing key is empty")
	}
	return &EvidenceManifestBuilder{secretKey: secretKey, shots: NewScreenshotService()}, nil
}

// LoadOrCreateSigningKey returns the key from MO_EVIDENCE_SIGNING_KEY, or else the key stored at
// rootDir/.sdlc/evidence-signing.key, generating 32 random bytes there (mode 0600) the first time.
func LoadOrCreateSigningKey(rootDir string) ([]byte, error) {
	if k := strings.TrimSpace(os.Getenv(SigningKeyEnv)); k != "" {
		return []byte(k), nil
	}
	path := filepath.Join(rootDir, SigningKeyFile)
	if raw, err := os.ReadFile(path); err == nil {
		key, err := hex.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil || len(key) < 16 {
			return nil, fmt.Errorf("%s is not a valid signing key", path)
		}
		return key, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) { // created concurrently: use that one
		return LoadOrCreateSigningKey(rootDir)
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.WriteString(hex.EncodeToString(key) + "\n"); err != nil {
		return nil, err
	}
	return key, nil
}

// sign is the HMAC-SHA256 of the canonical (JSON) serialization of the body.
func (b *EvidenceManifestBuilder) sign(body ManifestBody) (string, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, b.secretKey)
	mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// digestFiles fingerprints every file in dir except hidden ones and the signed manifest itself.
func (b *EvidenceManifestBuilder) digestFiles(dir string) ([]FileDigest, error) {
	out := []FileDigest{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && p != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if rel == SignedManifestFile || rel == ManifestMarkdown {
			return nil
		}
		fd := FileDigest{Path: rel, Kind: "other"}
		switch {
		case IsScreenshot(rel):
			shot, err := b.shots.Describe(p, rel)
			if err != nil {
				return err
			}
			fd.Kind, fd.SHA256, fd.Size, fd.Width, fd.Height = "screenshot", shot.SHA256Hash, shot.SizeBytes, shot.Width, shot.Height
			out = append(out, fd)
			return nil
		case IsVideo(rel):
			fd.Kind = "video"
		case rel == ManifestFile:
			fd.Kind = "manifest"
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if fd.SHA256, err = hashFile(p); err != nil {
			return err
		}
		fd.Size = info.Size()
		out = append(out, fd)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

// Build fingerprints the evidence directory and signs the results evidence.json reports.
func (b *EvidenceManifestBuilder) Build(taskID, dir string) (*SignedManifest, error) {
	rep := Collect(dir)
	files, err := b.digestFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("fingerprint evidence files: %w", err)
	}
	body := ManifestBody{Version: 1, TaskID: taskID, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Status: rep.Status(),
		Passed: rep.Passed, Failed: rep.Failed, ManifestError: rep.ManifestError, Cases: []CaseResult{}, Files: files}
	if m := rep.Manifest; m != nil {
		for _, c := range m.Cases {
			body.Cases = append(body.Cases, CaseResult{ID: c.ID, Kind: c.Kind, Title: c.Title, Status: c.Status, Video: c.Video})
		}
		for _, s := range m.Suites {
			body.Suites = append(body.Suites, SuiteResult{Repository: s.Repository, Command: s.Command, Status: s.Status, Passed: s.Passed, Failed: s.Failed})
		}
	}
	sig, err := b.sign(body)
	if err != nil {
		return nil, err
	}
	return &SignedManifest{Body: body, Algorithm: "HMAC-SHA256", Signature: sig}, nil
}

// Write builds the manifest and writes evidence-manifest.json and EVIDENCE.md into dir.
func (b *EvidenceManifestBuilder) Write(taskID, dir string) (*SignedManifest, error) {
	m, err := b.Build(taskID, dir)
	if err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, SignedManifestFile), raw, 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestMarkdown), []byte(RenderMarkdown(m)), 0o644); err != nil {
		return nil, err
	}
	return m, nil
}

// RenderMarkdown is the human-readable EVIDENCE.md: what the manifest says, nothing more.
func RenderMarkdown(m *SignedManifest) string {
	body := m.Body
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Test evidence: %s\n\n", body.TaskID)
	fmt.Fprintf(&sb, "Generated %s · status **%s** · %d passed · %d failed or blocked\n\n", body.GeneratedAt, body.Status, body.Passed, body.Failed)
	if body.ManifestError != "" {
		fmt.Fprintf(&sb, "> evidence.json could not be read: %s\n\n", body.ManifestError)
	}
	sb.WriteString("## Cases\n\n")
	if len(body.Cases) == 0 {
		sb.WriteString("No cases are listed in evidence.json.\n\n")
	} else {
		sb.WriteString("| Case | Kind | Status | Video | Title |\n|---|---|---|---|---|\n")
		for _, c := range body.Cases {
			fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n", mdCell(c.ID), mdCell(c.Kind), mdCell(c.Status), mdCell(c.Video), mdCell(c.Title))
		}
		sb.WriteString("\n")
	}
	if len(body.Suites) > 0 {
		sb.WriteString("## Suites\n\n| Repository | Status | Passed | Failed | Command |\n|---|---|---|---|---|\n")
		for _, s := range body.Suites {
			fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n", mdCell(s.Repository), mdCell(s.Status), intCell(s.Passed), intCell(s.Failed), mdCell(s.Command))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("## Files\n\n| File | Kind | Size | Dimensions | SHA-256 |\n|---|---|---|---|---|\n")
	for _, f := range body.Files {
		dims := ""
		if f.Width > 0 {
			dims = fmt.Sprintf("%dx%d", f.Width, f.Height)
		}
		fmt.Fprintf(&sb, "| %s | %s | %d | %s | `%s` |\n", mdCell(f.Path), f.Kind, f.Size, dims, f.SHA256)
	}
	fmt.Fprintf(&sb, "\n## Signature\n\n%s over the canonical JSON of `body` in %s: `%s`\n\n", m.Algorithm, SignedManifestFile, m.Signature)
	sb.WriteString("Verify with GET /api/v1/tasks/{id}/evidence/verify — it recomputes every file hash and the signature.\n")
	return sb.String()
}

func mdCell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", `\|`), "\n", " ")
}

func intCell(p *int) string {
	if p == nil {
		return ""
	}
	return fmt.Sprint(*p)
}

// VerifyResult is what Verify found.
type VerifyResult struct {
	Signed         bool     `json:"signed"`          // a signed manifest exists
	SignatureValid bool     `json:"signature_valid"` // the manifest body is unchanged since it was signed
	FilesUnchanged bool     `json:"files_unchanged"` // every file matches its recorded hash, none added or removed
	Valid          bool     `json:"valid"`
	Changed        []string `json:"changed"`
	Missing        []string `json:"missing"`
	Added          []string `json:"added"`
	SignedAt       string   `json:"signed_at,omitempty"`
	Error          string   `json:"error,omitempty"`
	ManifestFile   string   `json:"manifest_file"`
	RecordedFiles  int      `json:"recorded_files"`
}

// Verify recomputes the signature of dir's signed manifest and the hash of every file in dir.
func (b *EvidenceManifestBuilder) Verify(dir string) VerifyResult {
	res := VerifyResult{ManifestFile: SignedManifestFile, Changed: []string{}, Missing: []string{}, Added: []string{}}
	raw, err := os.ReadFile(filepath.Join(dir, SignedManifestFile))
	if err != nil {
		res.Error = "no signed evidence manifest: it is written when the evidence turn ends"
		return res
	}
	res.Signed = true
	var m SignedManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		res.Error = "the signed manifest is not valid JSON: " + err.Error()
		return res
	}
	res.SignedAt, res.RecordedFiles = m.Body.GeneratedAt, len(m.Body.Files)
	if sig, err := b.sign(m.Body); err == nil {
		res.SignatureValid = hmac.Equal([]byte(sig), []byte(m.Signature))
	}
	current, err := b.digestFiles(dir)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	now := map[string]string{}
	for _, f := range current {
		now[f.Path] = f.SHA256
	}
	for _, f := range m.Body.Files {
		h, ok := now[f.Path]
		switch {
		case !ok:
			res.Missing = append(res.Missing, f.Path)
		case h != f.SHA256:
			res.Changed = append(res.Changed, f.Path)
		}
		delete(now, f.Path)
	}
	for p := range now {
		res.Added = append(res.Added, p)
	}
	sort.Strings(res.Added)
	res.FilesUnchanged = len(res.Changed)+len(res.Missing)+len(res.Added) == 0
	res.Valid = res.SignatureValid && res.FilesUnchanged
	return res
}

package evidence

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
}

func evidenceFixture(t *testing.T) string {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "AC-1.mp4"), []byte("video-1"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "AC-2.mp4"), []byte("video-2"), 0o644)
	writePNG(t, filepath.Join(dir, "shots", "AC-1-01.png"), 320, 200)
	_ = os.WriteFile(filepath.Join(dir, ManifestFile), []byte(`{"cases":[
		{"id":"AC-1","title":"Sign in","status":"passed","video":"AC-1.mp4"},
		{"id":"AC-2","title":"Reject bad card","status":"failed","video":"AC-2.mp4"}],
		"suites":[{"repository":"api","status":"passed","passed":12,"failed":0}]}`), 0o644)
	return dir
}

func TestManifestReportsRealResultsAndFiles(t *testing.T) {
	dir := evidenceFixture(t)
	b, err := NewEvidenceManifestBuilder([]byte("test-key-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := b.Write("TASK-9", dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Body.Status != "failed" || m.Body.Passed != 1 || m.Body.Failed != 1 || len(m.Body.Cases) != 2 || m.Body.Cases[1].Status != "failed" {
		t.Fatalf("body must reflect evidence.json: %+v", m.Body)
	}
	paths := map[string]FileDigest{}
	for _, f := range m.Body.Files {
		paths[f.Path] = f
	}
	for _, p := range []string{"AC-1.mp4", "AC-2.mp4", "shots/AC-1-01.png", ManifestFile} {
		if paths[p].SHA256 == "" {
			t.Errorf("no hash for %s", p)
		}
	}
	if s := paths["shots/AC-1-01.png"]; s.Kind != "screenshot" || s.Width != 320 || s.Height != 200 {
		t.Errorf("screenshot must carry its real dimensions: %+v", s)
	}
	if _, ok := paths[SignedManifestFile]; ok {
		t.Error("the signed manifest must not list itself")
	}
	md, _ := os.ReadFile(filepath.Join(dir, ManifestMarkdown))
	if !strings.Contains(string(md), "| AC-2 | acceptance | failed |") || strings.Contains(string(md), "All") {
		t.Errorf("EVIDENCE.md must report the failure honestly:\n%s", md)
	}

	if v := b.Verify(dir); !v.Valid {
		t.Fatalf("fresh manifest must verify: %+v", v)
	}

	// A changed video, a removed screenshot and an extra file are each named.
	_ = os.WriteFile(filepath.Join(dir, "AC-1.mp4"), []byte("video-1-edited"), 0o644)
	_ = os.Remove(filepath.Join(dir, "shots", "AC-1-01.png"))
	_ = os.WriteFile(filepath.Join(dir, "extra.mp4"), []byte("x"), 0o644)
	v := b.Verify(dir)
	if v.Valid || !v.SignatureValid || strings.Join(v.Changed, ",") != "AC-1.mp4" || strings.Join(v.Missing, ",") != "shots/AC-1-01.png" ||
		strings.Join(v.Added, ",") != "extra.mp4" {
		t.Fatalf("tampering not reported: %+v", v)
	}
}

func TestManifestSignatureCoversTheWholeBody(t *testing.T) {
	dir := evidenceFixture(t)
	b, _ := NewEvidenceManifestBuilder([]byte("test-key-0123456789abcdef"))
	if _, err := b.Write("TASK-9", dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, SignedManifestFile)
	raw, _ := os.ReadFile(path)
	var m SignedManifest
	_ = json.Unmarshal(raw, &m)
	m.Body.Cases[1].Status = "passed" // flip a failure into a pass
	m.Body.Failed, m.Body.Passed = 0, 2
	raw, _ = json.Marshal(m)
	_ = os.WriteFile(path, raw, 0o644)
	if v := b.Verify(dir); v.SignatureValid || v.Valid {
		t.Fatalf("an edited manifest body must fail the signature: %+v", v)
	}

	other, _ := NewEvidenceManifestBuilder([]byte("another-key-0123456789"))
	if _, err := b.Write("TASK-9", dir); err != nil {
		t.Fatal(err)
	}
	if v := other.Verify(dir); v.SignatureValid {
		t.Fatal("a different key must not verify")
	}
	if _, err := NewEvidenceManifestBuilder(nil); err == nil {
		t.Fatal("an empty key must be refused")
	}
}

func TestSigningKeyIsGeneratedOnceWithPrivateMode(t *testing.T) {
	root := t.TempDir()
	t.Setenv(SigningKeyEnv, "")
	k1, err := LoadOrCreateSigningKey(root)
	if err != nil || len(k1) != 32 {
		t.Fatalf("key: %d %v", len(k1), err)
	}
	st, err := os.Stat(filepath.Join(root, SigningKeyFile))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode: %v %v", st, err)
	}
	k2, _ := LoadOrCreateSigningKey(root)
	if string(k1) != string(k2) {
		t.Fatal("the stored key must be reused")
	}
	t.Setenv(SigningKeyEnv, "from-env-key")
	if k, _ := LoadOrCreateSigningKey(root); string(k) != "from-env-key" {
		t.Fatal("the environment key must take precedence")
	}
}

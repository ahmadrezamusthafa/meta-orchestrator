package evidence

import (
	"strings"
	"testing"
)

func TestEvidenceManifestAndHMACSigning(t *testing.T) {
	builder := NewEvidenceManifestBuilder("secure-test-secret-key-1234")

	taskID := "task-ev-900"
	touched := map[string]string{
		"frontend-portal": "a1b2c3d4",
		"backend-api":     "e5f6a7b8",
	}

	doc, err := builder.BuildManifest(
		taskID,
		"bmad",
		touched,
		"https://vault.internal/videos/run_01.mp4",
		[]string{"step_01.png", "step_02.png"},
		14,
	)
	if err != nil {
		t.Fatalf("failed to build manifest: %v", err)
	}

	if !strings.Contains(doc, "cryptographic_signature:") {
		t.Errorf("manifest missing cryptographic_signature header")
	}
	if !strings.Contains(doc, "task-ev-900") {
		t.Errorf("manifest missing task ID")
	}
	if !strings.Contains(doc, "HMAC-SHA256:") {
		t.Errorf("manifest missing HMAC signature block")
	}

	// Test Signature verification & Tampering resistance
	payload := "task-ev-900:bmad:2026-09-24T12:00:00Z:https://vault.internal/videos/run_01.mp4"
	sig := builder.ComputeSignature(payload)

	// Valid signature
	if !builder.VerifySignature(payload, sig) {
		t.Errorf("expected valid signature to verify")
	}

	// 1-character tamper
	tamperedPayload := "task-ev-901:bmad:2026-09-24T12:00:00Z:https://vault.internal/videos/run_01.mp4"
	if builder.VerifySignature(tamperedPayload, sig) {
		t.Errorf("expected tampered payload to FAIL verification")
	}
}

package api

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/evidence"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// After an evidence turn, in the background and one task at a time: WebM videos without an MP4
// copy are transcoded to H.264 MP4 (portable for pull-request attachments; the Evidence tab plays
// the MP4 once it exists), then the evidence directory is fingerprinted into a signed manifest
// (evidence-manifest.json + EVIDENCE.md) that GET …/evidence/verify checks.

// evidenceJobSlot bounds evidence finalization to one job at a time across all tasks.
var evidenceJobSlot = make(chan struct{}, 1)

// transcodeTimeout bounds one video's transcode.
const transcodeTimeout = 10 * time.Minute

// finalizeEvidenceAsync starts finalization when the evidence directory holds videos.
func (r *Router) finalizeEvidenceAsync(taskID, turnID, dir string) {
	if len(evidence.Collect(dir).Videos) == 0 {
		return
	}
	r.evidenceJobs.Add(1)
	go func() {
		defer r.evidenceJobs.Done()
		evidenceJobSlot <- struct{}{}
		defer func() { <-evidenceJobSlot }()
		r.finalizeEvidence(context.Background(), taskID, turnID, dir)
	}()
}

// finalizeEvidence transcodes pending WebM videos and writes the signed manifest, reporting both.
func (r *Router) finalizeEvidence(ctx context.Context, taskID, turnID, dir string) {
	var lines []string
	if pending := evidence.PendingMP4(dir); len(pending) > 0 {
		rec := evidence.NewVideoRecorder(r.broadcast)
		if !evidence.NewTranscoder().Available() {
			lines = append(lines, fmt.Sprintf("ℹ %d video%s stay WebM (%s): ffmpeg is not installed, so no MP4 copies were made. "+
				"The Evidence tab plays WebM; install ffmpeg for MP4s to attach to pull requests.", len(pending), plural(len(pending)), strings.Join(pending, ", ")))
		} else {
			var done []string
			for _, v := range pending {
				tctx, cancel := context.WithTimeout(ctx, transcodeTimeout)
				_, err := rec.FinalizeRecording(tctx, taskID, dir, filepath.Join(dir, filepath.FromSlash(v)))
				cancel()
				if err != nil {
					lines = append(lines, fmt.Sprintf("⚠ Could not transcode %s to MP4: %v", v, err))
					continue
				}
				done = append(done, evidence.MP4Path(v))
			}
			if len(done) > 0 {
				lines = append(lines, fmt.Sprintf("🎞 Transcoded %d WebM video%s to MP4: %s.", len(done), plural(len(done)), strings.Join(done, ", ")))
			}
		}
	}

	key, err := evidence.LoadOrCreateSigningKey(r.cfg.RootDir)
	var m *evidence.SignedManifest
	if err == nil {
		var b *evidence.EvidenceManifestBuilder
		if b, err = evidence.NewEvidenceManifestBuilder(key); err == nil {
			m, err = b.Write(taskID, dir)
		}
	}
	if err != nil {
		lines = append(lines, fmt.Sprintf("⚠ The evidence manifest was not signed: %v", err))
	} else {
		lines = append(lines, fmt.Sprintf("🔏 Evidence manifest signed: %d file%s fingerprinted with SHA-256, results as evidence.json reports them "+
			"(%d passed, %d failed or blocked) — %s and %s. Verify it from the Evidence tab.", len(m.Body.Files), plural(len(m.Body.Files)),
			m.Body.Passed, m.Body.Failed, evidence.ManifestMarkdown, evidence.SignedManifestFile))
		r.mu.Lock()
		if t, ok := r.tasks[taskID]; ok {
			if t.Metadata == nil {
				t.Metadata = map[string]string{}
			}
			t.Metadata["evidence_manifest"] = m.Body.GeneratedAt
		}
		r.mu.Unlock()
	}
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: turnID, Content: strings.Join(lines, "\n")})
}

// handleEvidenceVerify serves GET /api/v1/tasks/{id}/evidence/verify: recomputes every file hash
// and the manifest signature, and names the files that changed, disappeared or appeared.
func (r *Router) handleEvidenceVerify(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required for evidence verify")
		return
	}
	taskID := req.PathValue("id")
	if r.taskSnapshot(taskID) == nil {
		r.writeError(w, http.StatusNotFound, fmt.Sprintf("Task %s not found", taskID))
		return
	}
	key, err := evidence.LoadOrCreateSigningKey(r.cfg.RootDir)
	if err != nil {
		r.writeError(w, http.StatusInternalServerError, "evidence signing key unavailable: "+err.Error())
		return
	}
	b, err := evidence.NewEvidenceManifestBuilder(key)
	if err != nil {
		r.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	r.writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": taskID, "verify": b.Verify(r.evidenceDir(taskID))})
}

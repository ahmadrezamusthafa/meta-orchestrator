package telemetry

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Handler serves /api/v1/telemetry/summary and /api/v1/telemetry/trends.
type Handler struct {
	ds  *Datastore
	now func() time.Time
}

// NewHandler creates the telemetry REST handler.
func NewHandler(ds *Datastore) *Handler {
	return &Handler{ds: ds, now: time.Now}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}
	window, _, err := ParseWindow(req.URL.Query().Get("window"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	repo := strings.TrimSpace(req.URL.Query().Get("repo"))

	switch strings.TrimSuffix(req.URL.Path, "/") {
	case "/api/v1/telemetry/summary":
		writeJSON(w, http.StatusOK, Summarize(h.ds, window, repo, h.now()))
	case "/api/v1/telemetry/trends":
		writeJSON(w, http.StatusOK, ComputeTrends(h.ds, window, repo, h.now()))
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Not found"})
	}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

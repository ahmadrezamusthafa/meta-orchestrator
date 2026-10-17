package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/packages"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// packagesManifest is the package manifest, relative to RootDir.
var packagesManifest = filepath.Join("configs", "packages.json")

// packageManager returns the configured package manager, or one over RootDir's manifest with its
// state in packages.DefaultHome(). Jobs are broadcast as "package.job" events.
func (r *Router) packageManager() *packages.Manager {
	r.pkgOnce.Do(func() {
		m := r.cfg.Packages
		if m == nil {
			m, _ = packages.Load(filepath.Join(r.cfg.RootDir, packagesManifest),
				packages.Options{Home: packages.DefaultHome(), Toolchain: r.toolchain})
		}
		if m != nil {
			m.SetOnJob(func(j packages.Job) { r.broadcast("", types.EventPackageJob, j) })
		}
		r.pkgMgr = m
	})
	return r.pkgMgr
}

type packageActionRequest struct {
	Version string `json:"version"`
}

// handlePackages: GET /api/v1/packages lists every package with its status on this machine.
func (r *Router) handlePackages(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	m := r.packageManager()
	if m == nil {
		r.writeError(w, http.StatusServiceUnavailable, "package manager unavailable")
		return
	}
	r.writeJSON(w, http.StatusOK, m.List())
}

// handlePackageAction serves everything below /api/v1/packages/.
func (r *Router) handlePackageAction(w http.ResponseWriter, req *http.Request) {
	m := r.packageManager()
	if m == nil {
		r.writeError(w, http.StatusServiceUnavailable, "package manager unavailable")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(req.URL.Path, "/api/v1/packages/"), "/"), "/")
	switch {
	case len(parts) == 1 && parts[0] == "setup":
		if !requirePost(r, w, req) {
			return
		}
		res, err := m.Setup()
		if err != nil {
			r.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.writeJSON(w, http.StatusAccepted, res)
		return
	case len(parts) == 1 && parts[0] == "recheck":
		if !requirePost(r, w, req) {
			return
		}
		r.writeJSON(w, http.StatusOK, m.Recheck())
		return
	case len(parts) == 2 && parts[0] == "jobs":
		if req.Method != http.MethodGet {
			r.writeError(w, http.StatusMethodNotAllowed, "GET required")
			return
		}
		if j := m.Job(parts[1]); j != nil {
			r.writeJSON(w, http.StatusOK, j)
			return
		}
		r.writeError(w, http.StatusNotFound, "unknown job "+parts[1])
		return
	case len(parts) != 2:
		r.writeError(w, http.StatusNotFound, "unknown packages endpoint")
		return
	}

	id, op := parts[0], parts[1]
	if !packages.ValidID(id) || m.Manifest().Get(id) == nil {
		r.writeError(w, http.StatusNotFound, "unknown package "+id)
		return
	}
	if op == "versions" {
		if req.Method != http.MethodGet {
			r.writeError(w, http.StatusMethodNotAllowed, "GET required")
			return
		}
		pre := req.URL.Query().Get("prerelease")
		vs, latest, err := m.Versions(req.Context(), id, pre == "1" || pre == "true")
		if err != nil {
			r.writeJSON(w, http.StatusBadGateway, map[string]interface{}{"error": err.Error(), "versions": []string{}, "latest": latest})
			return
		}
		r.writeJSON(w, http.StatusOK, map[string]interface{}{"versions": vs, "latest": latest})
		return
	}
	if !requirePost(r, w, req) {
		return
	}
	var body packageActionRequest
	if raw, _ := io.ReadAll(io.LimitReader(req.Body, 1<<16)); len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			r.writeError(w, http.StatusBadRequest, `expected {"version": "<version>"} or an empty body`)
			return
		}
	}
	var job *packages.Job
	var err error
	switch op {
	case "install":
		job, err = m.Install(id, body.Version)
	case "update":
		job, err = m.Update(id)
	case "rollback":
		job, err = m.Rollback(id, body.Version)
	case "uninstall":
		job, err = m.Uninstall(id)
	default:
		r.writeError(w, http.StatusNotFound, "unknown package action "+op)
		return
	}
	r.writePackageJob(w, job, err)
}

func requirePost(r *Router, w http.ResponseWriter, req *http.Request) bool {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return false
	}
	return true
}

func (r *Router) writePackageJob(w http.ResponseWriter, job *packages.Job, err error) {
	var manual *packages.ManualError
	switch {
	case err == nil:
		r.writeJSON(w, http.StatusAccepted, job)
	case errors.As(err, &manual):
		r.writeJSON(w, http.StatusNotImplemented, map[string]interface{}{"error": manual.Reason, "manual": manual.Manual})
	case errors.Is(err, packages.ErrBusy):
		r.writeJSON(w, http.StatusConflict, map[string]interface{}{"error": err.Error(), "job": job})
	case errors.Is(err, packages.ErrRollbackUnsupported):
		r.writeJSON(w, http.StatusNotImplemented, map[string]interface{}{"error": err.Error() + ": system package managers keep no previous versions; use a version manager such as nvm/fnm for Node"})
	case errors.Is(err, packages.ErrUnknownPackage):
		r.writeError(w, http.StatusNotFound, err.Error())
	default:
		r.writeError(w, http.StatusBadRequest, err.Error())
	}
}

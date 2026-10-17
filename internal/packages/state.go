package packages

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// KeptVersion is a managed install of an npm-cli or go-cli package.
type KeptVersion struct {
	Version      string    `json:"version"`
	InstalledAt  time.Time `json:"installed_at"`
	LastActiveAt time.Time `json:"last_active_at,omitempty"`
}

// Snapshot is a saved footprint of an npx-installer or claude-plugin package.
type Snapshot struct {
	Version   string    `json:"version"` // "" when it could not be read
	File      string    `json:"file"`    // tar.gz, relative to the package's state folder
	CreatedAt time.Time `json:"created_at"`
	Reason    string    `json:"reason"` // "before update", …
	// Plugin: the plugin's installed_plugins.json entries and cache folder (relative to home).
	PluginEntries json.RawMessage `json:"plugin_entries,omitempty"`
	PluginPath    string          `json:"plugin_path,omitempty"`
}

// HistoryEntry is one finished action.
type HistoryEntry struct {
	At      time.Time `json:"at"`
	Action  string    `json:"action"`
	From    string    `json:"from"`
	To      string    `json:"to"`
	OK      bool      `json:"ok"`
	Message string    `json:"message"`
}

// PackageState is what the orchestrator remembers about one package.
type PackageState struct {
	Active    string         `json:"active,omitempty"`
	Versions  []KeptVersion  `json:"versions,omitempty"`
	Snapshots []Snapshot     `json:"snapshots,omitempty"`
	History   []HistoryEntry `json:"history,omitempty"` // oldest first
}

const (
	maxHistory      = 50
	keepInactive    = 3 // managed versions kept besides the active one
	keepSnapshots   = 5
	stateFileMode   = 0o600
	stateFolderMode = 0o700
)

type stateFile struct {
	Packages map[string]*PackageState `json:"packages"`
}

// store persists PackageStates in <home>/packages/state.json.
type store struct {
	path string
	mu   sync.Mutex
	data stateFile
}

func openStore(path string) (*store, error) {
	s := &store{path: path, data: stateFile{Packages: map[string]*PackageState{}}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return s, err
	}
	if s.data.Packages == nil {
		s.data.Packages = map[string]*PackageState{}
	}
	return s, nil
}

// get returns a deep copy of id's state.
func (s *store) get(id string) PackageState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyState(s.data.Packages[id])
}

func copyState(p *PackageState) PackageState {
	if p == nil {
		return PackageState{}
	}
	return PackageState{Active: p.Active,
		Versions:  append([]KeptVersion(nil), p.Versions...),
		Snapshots: append([]Snapshot(nil), p.Snapshots...),
		History:   append([]HistoryEntry(nil), p.History...)}
}

// update changes id's state and writes the file.
func (s *store) update(id string, fn func(*PackageState)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.data.Packages[id]
	if p == nil {
		p = &PackageState{}
		s.data.Packages[id] = p
	}
	fn(p)
	if over := len(p.History) - maxHistory; over > 0 {
		p.History = append([]HistoryEntry(nil), p.History[over:]...)
	}
	return s.saveLocked()
}

func (s *store) saveLocked() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, raw, stateFileMode)
}

// writeFileAtomic writes data to a temporary file next to path and renames it over path, so a
// reader never sees a half-written file.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), stateFolderMode); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op after the rename
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (p *PackageState) kept(version string) *KeptVersion {
	for i := range p.Versions {
		if p.Versions[i].Version == version {
			return &p.Versions[i]
		}
	}
	return nil
}

// previousVersion is the kept version that was active most recently, other than the active one.
func (p *PackageState) previousVersion() string {
	best, bestAt := "", time.Time{}
	for _, v := range p.Versions {
		if v.Version == p.Active {
			continue
		}
		at := v.LastActiveAt
		if at.IsZero() {
			at = v.InstalledAt
		}
		if best == "" || at.After(bestAt) {
			best, bestAt = v.Version, at
		}
	}
	return best
}

// pruneVersions drops all but the keepInactive most recently used inactive versions and returns
// the dropped ones.
func (p *PackageState) pruneVersions() []string {
	var inactive []KeptVersion
	var keep []KeptVersion
	for _, v := range p.Versions {
		if v.Version == p.Active {
			keep = append(keep, v)
		} else {
			inactive = append(inactive, v)
		}
	}
	sort.SliceStable(inactive, func(i, j int) bool { return lastUse(inactive[i]).After(lastUse(inactive[j])) })
	var dropped []string
	for i, v := range inactive {
		if i < keepInactive {
			keep = append(keep, v)
		} else {
			dropped = append(dropped, v.Version)
		}
	}
	sort.SliceStable(keep, func(i, j int) bool { return keep[i].InstalledAt.Before(keep[j].InstalledAt) })
	p.Versions = keep
	return dropped
}

func lastUse(v KeptVersion) time.Time {
	if v.LastActiveAt.After(v.InstalledAt) {
		return v.LastActiveAt
	}
	return v.InstalledAt
}

// snapshotFor picks the snapshot to restore: the newest one of version, or with version "" the
// newest whose version differs from current (falling back to the newest of all).
func (p *PackageState) snapshotFor(version, current string) *Snapshot {
	for i := len(p.Snapshots) - 1; i >= 0; i-- {
		s := &p.Snapshots[i]
		if version != "" && s.Version == version {
			return s
		}
		if version == "" && s.Version != current {
			return s
		}
	}
	if version == "" && len(p.Snapshots) > 0 {
		return &p.Snapshots[len(p.Snapshots)-1]
	}
	return nil
}

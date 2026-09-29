package telemetry

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Table names of the telemetry store.
const (
	TableTokens  = "telemetry_tokens"
	TableRuns    = "telemetry_runs"
	TableMethods = "telemetry_methods"
)

// Filter narrows a table scan. Shadow-benchmark rows are excluded unless requested.
type Filter struct {
	Since         time.Time
	Until         time.Time
	Repo          string
	IncludeShadow bool
	OnlyShadow    bool
}

func (f Filter) matchTime(t time.Time) bool {
	if !f.Since.IsZero() && t.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && !t.Before(f.Until) {
		return false
	}
	return true
}

func (f Filter) matchShadow(shadow bool) bool {
	if f.OnlyShadow {
		return shadow
	}
	return f.IncludeShadow || !shadow
}

// MethodStat is one telemetry_methods row: rolling totals per (method, stage, complexity, tier).
type MethodStat struct {
	Method          string    `json:"method"`
	StageID         string    `json:"stage_id"`
	Complexity      string    `json:"complexity"`
	Tier            string    `json:"tier"`
	Runs            int       `json:"runs"`
	Successes       int       `json:"successes"`
	FirstPasses     int       `json:"first_passes"`
	TotalTokens     int64     `json:"total_tokens"`
	TotalCostUSD    float64   `json:"total_cost_usd"`
	TotalDurationUS int64     `json:"total_duration_us"`
	LastRunAt       time.Time `json:"last_run_at"`
}

// Datastore is an append-only, time-ordered telemetry store with optional JSONL persistence.
// Rows are kept in memory sorted by timestamp, so window scans binary-search the start offset.
type Datastore struct {
	mu      sync.RWMutex
	dir     string
	tokens  []TokenEvent
	runs    []RunRecord
	methods map[string]*MethodStat
	files   map[string]*os.File
}

// NewMemoryDatastore creates a non-persistent store.
func NewMemoryDatastore() *Datastore {
	return &Datastore{methods: make(map[string]*MethodStat), files: make(map[string]*os.File)}
}

// OpenDatastore opens (or creates) a store persisted as JSONL files under dir.
func OpenDatastore(dir string) (*Datastore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("telemetry datastore: %w", err)
	}
	ds := NewMemoryDatastore()
	ds.dir = dir

	if err := loadJSONL(filepath.Join(dir, TableTokens+".jsonl"), func(line []byte) error {
		var ev TokenEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return err
		}
		ds.insertToken(ev)
		return nil
	}); err != nil {
		return nil, err
	}
	if err := loadJSONL(filepath.Join(dir, TableRuns+".jsonl"), func(line []byte) error {
		var rec RunRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return err
		}
		ds.insertRun(rec)
		return nil
	}); err != nil {
		return nil, err
	}
	for _, table := range []string{TableTokens, TableRuns} {
		f, err := os.OpenFile(filepath.Join(dir, table+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			ds.Close()
			return nil, fmt.Errorf("telemetry datastore: %w", err)
		}
		ds.files[table] = f
	}
	return ds, nil
}

func loadJSONL(path string, fn func([]byte) error) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("telemetry datastore: %w", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		// A torn trailing write (crash mid-append) is skipped rather than failing startup.
		_ = fn(line)
	}
	return sc.Err()
}

// Close flushes and closes persistence files.
func (d *Datastore) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	var firstErr error
	for k, f := range d.files {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(d.files, k)
	}
	return firstErr
}

// RecordTokens inserts a telemetry_tokens row (implements TokenSink).
func (d *Datastore) RecordTokens(ev TokenEvent) error {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.insertToken(ev)
	return d.appendLocked(TableTokens, ev)
}

// RecordRun inserts a telemetry_runs row and updates telemetry_methods (implements RunSink).
func (d *Datastore) RecordRun(rec RunRecord) error {
	if rec.FinishedAt.IsZero() {
		rec.FinishedAt = time.Now()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.insertRun(rec)
	return d.appendLocked(TableRuns, rec)
}

func (d *Datastore) appendLocked(table string, row interface{}) error {
	f, ok := d.files[table]
	if !ok {
		return nil
	}
	data, err := json.Marshal(row)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

func (d *Datastore) insertToken(ev TokenEvent) {
	n := len(d.tokens)
	if n == 0 || !ev.Timestamp.Before(d.tokens[n-1].Timestamp) {
		d.tokens = append(d.tokens, ev)
		return
	}
	i := sort.Search(n, func(i int) bool { return d.tokens[i].Timestamp.After(ev.Timestamp) })
	d.tokens = append(d.tokens, TokenEvent{})
	copy(d.tokens[i+1:], d.tokens[i:])
	d.tokens[i] = ev
}

func (d *Datastore) insertRun(rec RunRecord) {
	n := len(d.runs)
	if n == 0 || !rec.FinishedAt.Before(d.runs[n-1].FinishedAt) {
		d.runs = append(d.runs, rec)
	} else {
		i := sort.Search(n, func(i int) bool { return d.runs[i].FinishedAt.After(rec.FinishedAt) })
		d.runs = append(d.runs, RunRecord{})
		copy(d.runs[i+1:], d.runs[i:])
		d.runs[i] = rec
	}

	key := strings.Join([]string{rec.Method, rec.StageID, rec.Complexity, rec.Tier}, "|")
	m, ok := d.methods[key]
	if !ok {
		m = &MethodStat{Method: rec.Method, StageID: rec.StageID, Complexity: rec.Complexity, Tier: rec.Tier}
		d.methods[key] = m
	}
	m.Runs++
	if rec.Success {
		m.Successes++
	}
	if rec.FirstPass {
		m.FirstPasses++
	}
	m.TotalTokens += rec.TotalTokens()
	m.TotalCostUSD += rec.CostUSD
	m.TotalDurationUS += rec.DurationUS
	if rec.FinishedAt.After(m.LastRunAt) {
		m.LastRunAt = rec.FinishedAt
	}
}

// Tokens scans telemetry_tokens.
func (d *Datastore) Tokens(f Filter) []TokenEvent {
	d.mu.RLock()
	defer d.mu.RUnlock()
	start := 0
	if !f.Since.IsZero() {
		start = sort.Search(len(d.tokens), func(i int) bool { return !d.tokens[i].Timestamp.Before(f.Since) })
	}
	var out []TokenEvent
	for _, ev := range d.tokens[start:] {
		if !f.matchTime(ev.Timestamp) || !f.matchShadow(ev.Shadow) || (f.Repo != "" && ev.Repo != f.Repo) {
			continue
		}
		out = append(out, ev)
	}
	return out
}

// Runs scans telemetry_runs.
func (d *Datastore) Runs(f Filter) []RunRecord {
	d.mu.RLock()
	defer d.mu.RUnlock()
	start := 0
	if !f.Since.IsZero() {
		start = sort.Search(len(d.runs), func(i int) bool { return !d.runs[i].FinishedAt.Before(f.Since) })
	}
	var out []RunRecord
	for _, r := range d.runs[start:] {
		if !f.matchTime(r.FinishedAt) || !f.matchShadow(r.Shadow) || (f.Repo != "" && r.Repo != f.Repo) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// MethodStats returns the telemetry_methods table sorted by key.
func (d *Datastore) MethodStats() []MethodStat {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]MethodStat, 0, len(d.methods))
	for _, m := range d.methods {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Method != b.Method {
			return a.Method < b.Method
		}
		if a.StageID != b.StageID {
			return a.StageID < b.StageID
		}
		if a.Complexity != b.Complexity {
			return a.Complexity < b.Complexity
		}
		return a.Tier < b.Tier
	})
	return out
}

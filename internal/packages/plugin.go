package packages

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// claude-plugin packages are managed with `claude plugin …`. Claude Code keeps no previous plugin
// versions, so before an update or uninstall the orchestrator snapshots the plugin's cache folder
// and its entry in ~/.claude/plugins/installed_plugins.json; a rollback restores both. Editing
// Claude Code's own state file is best effort, and Claude Code must be restarted afterwards.

type pluginListing struct {
	at      time.Time
	entries []pluginListEntry
	err     error
}

type pluginListEntry struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Scope       string `json:"scope"`
	InstallPath string `json:"installPath"`
}

type pluginEntry struct {
	raw         json.RawMessage // the plugin's whole entry list in installed_plugins.json
	version     string
	installPath string
}

func (m *Manager) pluginsFile() string {
	return filepath.Join(m.opts.ClaudeDir, "plugins", "installed_plugins.json")
}

// listPlugins runs `claude plugin list --json`, cached for a minute. Without the claude binary it
// reads installed_plugins.json instead.
func (m *Manager) listPlugins() ([]pluginListEntry, error) {
	m.pluginMu.Lock()
	defer m.pluginMu.Unlock()
	if m.pluginList != nil && time.Since(m.pluginList.at) < pluginListTTL {
		return m.pluginList.entries, m.pluginList.err
	}
	l := &pluginListing{at: time.Now()}
	if _, err := exec.LookPath("claude"); err == nil {
		out, err := m.output(m.ctx, []string{"claude", "plugin", "list", "--json"}, m.opts.UserHome)
		if err == nil {
			i := indexJSON(out)
			if i < 0 {
				err = errors.New("`claude plugin list --json` printed no JSON")
			} else if jerr := json.Unmarshal([]byte(out[i:]), &l.entries); jerr != nil {
				err = fmt.Errorf("`claude plugin list --json` printed unreadable JSON: %v", jerr)
			}
		}
		l.err = err
	} else {
		l.entries, l.err = m.pluginsFromFile()
	}
	m.pluginList = l
	return l.entries, l.err
}

func indexJSON(s string) int {
	for i, c := range s {
		if c == '[' || c == '{' {
			return i
		}
	}
	return -1
}

// pluginsFromFile lists installed_plugins.json, preferring user-scope entries.
func (m *Manager) pluginsFromFile() ([]pluginListEntry, error) {
	f, err := m.readPluginsFile()
	if err != nil || f == nil {
		return nil, err
	}
	var out []pluginListEntry
	for id, raw := range f.plugins {
		var list []pluginListEntry
		if json.Unmarshal(raw, &list) != nil {
			continue
		}
		for _, e := range list {
			e.ID = id
			out = append(out, e)
		}
	}
	return out, nil
}

type pluginsFile struct {
	top     map[string]json.RawMessage
	plugins map[string]json.RawMessage
}

func (m *Manager) readPluginsFile() (*pluginsFile, error) {
	raw, err := os.ReadFile(m.pluginsFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	f := &pluginsFile{}
	if err := json.Unmarshal(raw, &f.top); err != nil {
		return nil, fmt.Errorf("installed_plugins.json: %w", err)
	}
	f.plugins = map[string]json.RawMessage{}
	if p, ok := f.top["plugins"]; ok {
		if err := json.Unmarshal(p, &f.plugins); err != nil {
			return nil, fmt.Errorf("installed_plugins.json plugins: %w", err)
		}
	}
	return f, nil
}

// pluginEntries returns the plugin's entries in installed_plugins.json, or nil.
func (m *Manager) pluginEntries(id string) (*pluginEntry, error) {
	f, err := m.readPluginsFile()
	if err != nil || f == nil {
		return nil, err
	}
	raw, ok := f.plugins[id]
	if !ok {
		return nil, nil
	}
	var list []pluginListEntry
	if err := json.Unmarshal(raw, &list); err != nil || len(list) == 0 {
		return nil, nil
	}
	e := pickPluginEntry(list)
	return &pluginEntry{raw: raw, version: e.Version, installPath: e.InstallPath}, nil
}

func pickPluginEntry(list []pluginListEntry) pluginListEntry {
	for _, e := range list {
		if e.Scope == "user" {
			return e
		}
	}
	return list[0]
}

func (m *Manager) detectPlugin(s *Spec) detection {
	d := detection{status: StatusNotInstalled}
	list, err := m.listPlugins()
	if err != nil {
		d.status, d.problem = StatusError, err.Error()
		return d
	}
	var mine []pluginListEntry
	for _, e := range list {
		if e.ID == s.Package {
			mine = append(mine, e)
		}
	}
	if len(mine) == 0 {
		return d
	}
	e := pickPluginEntry(mine)
	d.status, d.location, d.managed = StatusInstalled, e.InstallPath, true
	d.version = e.Version
	if ValidateVersion(d.version) != nil {
		d.version = VersionUnknown
	}
	return d
}

// restorePluginEntries rewrites only this plugin's entry in installed_plugins.json, keeping a
// .bak copy of the file, and re-enables the plugin when it had been uninstalled.
func (m *Manager) restorePluginEntries(r *jobRun, sn Snapshot) error {
	if len(sn.PluginEntries) == 0 {
		return errors.New("the snapshot has no installed_plugins.json entry")
	}
	path := m.pluginsFile()
	f, err := m.readPluginsFile()
	if err != nil {
		return err
	}
	if f == nil {
		f = &pluginsFile{top: map[string]json.RawMessage{"version": json.RawMessage("2")}, plugins: map[string]json.RawMessage{}}
	} else {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := writeFileAtomic(path+".bak", raw, 0o600); err != nil {
			return fmt.Errorf("backing up installed_plugins.json: %w", err)
		}
		r.log("Saved a copy of %s as %s.bak", path, path)
	}
	_, wasInstalled := f.plugins[r.spec.Package]
	f.plugins[r.spec.Package] = sn.PluginEntries
	pl, err := json.Marshal(f.plugins)
	if err != nil {
		return err
	}
	f.top["plugins"] = pl
	out, err := json.MarshalIndent(f.top, "", "  ")
	if err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	if err := writeFileAtomic(path, out, mode); err != nil {
		return err
	}
	r.log("Rewrote the %s entry in %s.", r.spec.Package, path)
	if !wasInstalled {
		if err := r.run([]string{"claude", "plugin", "enable", r.spec.Package}, nil, m.opts.UserHome); err != nil {
			r.log("Could not re-enable the plugin (%v); enable it with /plugin in Claude Code.", err)
		}
	}
	return nil
}

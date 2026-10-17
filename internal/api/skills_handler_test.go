package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/connectors"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/skills"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestSkillsAPIEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	resolver := skills.NewMultiSourceSkillResolver(tempDir)

	router := NewRouter(RouterConfig{
		RootDir:       tempDir,
		SkillResolver: resolver,
	})

	// 1. GET /api/v1/skills - list all skills with details & compatibility
	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/skills returned %d", rec.Code)
	}

	var allSkills []*types.UniversalSkillContract
	if err := json.Unmarshal(rec.Body.Bytes(), &allSkills); err != nil {
		t.Fatalf("failed to parse skills response: %v", err)
	}
	if len(allSkills) == 0 {
		t.Fatalf("expected built-in skills to be present")
	}

	// Verify compatibility is populated
	firstSkill := allSkills[0]
	if firstSkill.Compatibility == nil {
		t.Errorf("expected Compatibility report to be populated on skill %s", firstSkill.Name)
	}
	if !firstSkill.Enabled {
		t.Errorf("expected skill %s to be enabled by default", firstSkill.Name)
	}

	// 2. POST /api/v1/skills/{name}/toggle - disable a skill
	togglePayload, _ := json.Marshal(map[string]bool{"enabled": false})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/skills/resolve_symlinks/toggle", bytes.NewReader(togglePayload))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST toggle returned %d: %s", rec.Code, rec.Body.String())
	}

	// Verify resolve_symlinks is now disabled
	req = httptest.NewRequest(http.MethodGet, "/api/v1/skills?enabled_only=true", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var activeSkills []*types.UniversalSkillContract
	_ = json.Unmarshal(rec.Body.Bytes(), &activeSkills)
	for _, s := range activeSkills {
		if s.Name == "resolve_symlinks" {
			t.Errorf("disabled skill should not be present in active query")
		}
	}

	// Re-enable it
	togglePayload, _ = json.Marshal(map[string]bool{"enabled": true})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/skills/resolve_symlinks/toggle", bytes.NewReader(togglePayload))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST re-enable toggle returned %d", rec.Code)
	}

	// 3. Pre-flight compatibility check: POST /api/v1/skills/check-compatibility
	externalDir := t.TempDir()
	skillDir := filepath.Join(externalDir, "claude_qa_check")
	_ = os.MkdirAll(skillDir, 0755)
	skillMD := `---
name: claude_qa_check
description: Autonomous testing validator for pull requests
timeout_seconds: 60
---
# QA Checklist
Run smoke tests and verify zero regressions.
`
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644)

	checkPayload, _ := json.Marshal(map[string]string{"path": externalDir})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/skills/check-compatibility", bytes.NewReader(checkPayload))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("check-compatibility returned %d", rec.Code)
	}
	var checkResp struct {
		Compatible      bool                            `json:"compatible"`
		DiscoveredCount int                             `json:"discovered_count"`
		Skills          []*types.UniversalSkillContract `json:"skills"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &checkResp); err != nil {
		t.Fatalf("failed to decode check-compatibility response: %v", err)
	}
	if !checkResp.Compatible || checkResp.DiscoveredCount != 1 {
		t.Errorf("unexpected pre-flight result: %+v", checkResp)
	}

	// 4. Register custom source: POST /api/v1/skills/sources
	regPayload, _ := json.Marshal(map[string]string{
		"name":   "QA Claude Skills",
		"path":   externalDir,
		"format": "claude",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/skills/sources", bytes.NewReader(regPayload))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/skills/sources returned %d: %s", rec.Code, rec.Body.String())
	}
	var regResp struct {
		Source          skills.SkillSource `json:"source"`
		DiscoveredCount int                `json:"discovered_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &regResp); err != nil {
		t.Fatalf("failed to decode register response: %v", err)
	}
	if regResp.Source.Name != "QA Claude Skills" || regResp.DiscoveredCount != 1 {
		t.Errorf("unexpected registered source response: %+v", regResp)
	}

	// 5. List sources: GET /api/v1/skills/sources
	req = httptest.NewRequest(http.MethodGet, "/api/v1/skills/sources", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/skills/sources returned %d", rec.Code)
	}
	var sources []skills.SkillSource
	_ = json.Unmarshal(rec.Body.Bytes(), &sources)
	if len(sources) != 1 {
		t.Errorf("expected 1 source registered, got %d", len(sources))
	}

	// 6. Rescan: POST /api/v1/skills/rescan
	req = httptest.NewRequest(http.MethodPost, "/api/v1/skills/rescan", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/skills/rescan returned %d", rec.Code)
	}

	// 7. Delete source: DELETE /api/v1/skills/sources/{id}
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/skills/sources/"+regResp.Source.ID, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE source returned %d", rec.Code)
	}

	// Verify source deleted
	if len(resolver.ListSources()) != 0 {
		t.Errorf("expected source to be deleted")
	}
}

// MCP server skills and the MCP export carry the servers' environments: browsers only ever see
// their credentials masked.
func TestSkillsAndMCPExportNeverShowCredentials(t *testing.T) {
	for _, k := range []string{"SLACK_BOT_TOKEN", "SLACK_TOKEN"} {
		t.Setenv(k, "")
	}
	root := t.TempDir()
	const fileSecret, connectorSecret = "ghp_projectfile_secret_1234", "xoxb-connector-secret-5678"
	mcpFile := `{"mcpServers":{"gh":{"command":"github-mcp-server","args":["stdio","--token","` + fileSecret + `"],` +
		`"env":{"GITHUB_PERSONAL_ACCESS_TOKEN":"` + fileSecret + `","GITHUB_TOOLSETS":"repos"}}}}`
	cm := connectors.NewManager(root)
	t.Cleanup(cm.StopPeriodicPinger)
	slack, _ := cm.GetConnector("slack")
	cp := *slack
	cp.Enabled = true
	cp.MCP = &types.MCPConfig{Enabled: true, Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-slack"}, Transport: "stdio",
		Env: map[string]string{"SLACK_BOT_TOKEN": connectorSecret, "SLACK_TEAM_ID": "T1"}}
	if _, err := cm.UpdateConnector(cp); err != nil {
		t.Fatal(err)
	}
	// Saving a connector rewrites .mcp.json; the project's own file is written afterwards.
	if err := os.WriteFile(filepath.Join(root, ".mcp.json"), []byte(mcpFile), 0o600); err != nil {
		t.Fatal(err)
	}
	r := NewRouter(RouterConfig{RootDir: root, SkillResolver: skills.NewMultiSourceSkillResolver(root), ConnectorsManager: cm})

	for _, path := range []string{"/api/v1/skills", "/api/v1/skills/gh", "/api/v1/skills/slack", "/api/v1/connectors/mcp-config"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
		body := rec.Body.String()
		if strings.Contains(body, fileSecret) || strings.Contains(body, connectorSecret) {
			t.Errorf("GET %s shows a credential in clear", path)
		}
		if path == "/api/v1/skills/gh" && !strings.Contains(body, "••••1234") {
			t.Errorf("GET %s should show the masked token: %s", path, body)
		}
	}
}

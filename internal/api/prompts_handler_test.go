package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/registry"
)

func TestPromptsAPIEndpoints(t *testing.T) {
	tmpDir := t.TempDir()
	promptReg := registry.NewPromptRegistry(tmpDir)

	router := NewRouter(RouterConfig{
		PromptRegistry: promptReg,
		RootDir:        tmpDir,
	})

	// 1. GET /api/v1/prompts
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/prompts", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK from GET /api/v1/prompts, got %d", rr.Code)
	}

	var prompts []registry.PromptItemDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &prompts); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if len(prompts) == 0 {
		t.Fatalf("Expected default prompt templates, got 0")
	}

	// 2. Check compatibility against billing repo if exists
	billingPromptsDir := "/Users/rezamekari/Projects/go/src/bitbucket.org/mid-kelola-indonesia/billing/.github/prompts"
	if _, err := os.Stat(billingPromptsDir); err == nil {
		checkBody, _ := json.Marshal(map[string]string{
			"path": billingPromptsDir,
		})
		reqCheck, _ := http.NewRequest(http.MethodPost, "/api/v1/prompts/check-compatibility", bytes.NewBuffer(checkBody))
		rrCheck := httptest.NewRecorder()
		router.ServeHTTP(rrCheck, reqCheck)

		if rrCheck.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK from check-compatibility, got %d", rrCheck.Code)
		}

		var checkResp struct {
			Compatible      bool                    `json:"compatible"`
			DiscoveredCount int                     `json:"discovered_count"`
			Templates       []registry.PromptItemDTO `json:"templates"`
		}
		if err := json.Unmarshal(rrCheck.Body.Bytes(), &checkResp); err != nil {
			t.Fatalf("Failed to decode check compatibility response: %v", err)
		}

		if !checkResp.Compatible || checkResp.DiscoveredCount < 15 {
			t.Errorf("Expected >=15 compatible templates in billing repo, got %d (compat: %v)",
				checkResp.DiscoveredCount, checkResp.Compatible)
		}

		// 3. Register billing prompts source
		regBody, _ := json.Marshal(map[string]string{
			"name": "Billing GitHub Prompts",
			"path": billingPromptsDir,
		})
		reqReg, _ := http.NewRequest(http.MethodPost, "/api/v1/prompts/sources", bytes.NewBuffer(regBody))
		rrReg := httptest.NewRecorder()
		router.ServeHTTP(rrReg, reqReg)

		if rrReg.Code != http.StatusCreated {
			t.Fatalf("Expected 201 Created from POST /api/v1/prompts/sources, got %d: %s", rrReg.Code, rrReg.Body.String())
		}

		var regResp struct {
			Source          registry.PromptSource   `json:"source"`
			DiscoveredCount int                     `json:"discovered_count"`
			Templates       []registry.PromptItemDTO `json:"templates"`
		}
		if err := json.Unmarshal(rrReg.Body.Bytes(), &regResp); err != nil {
			t.Fatalf("Failed to decode register response: %v", err)
		}

		if regResp.Source.TemplateCount == 0 {
			t.Errorf("Expected non-zero templates in registered source")
		}

		// 4. Render one of the registered billing templates
		renderBody, _ := json.Marshal(map[string]interface{}{
			"template_id": "prd-to-jira-user-story",
			"parameters": map[string]string{
				"parent_issue_key": "MIB-9999",
			},
		})
		reqRender, _ := http.NewRequest(http.MethodPost, "/api/v1/prompts/render", bytes.NewBuffer(renderBody))
		rrRender := httptest.NewRecorder()
		router.ServeHTTP(rrRender, reqRender)

		if rrRender.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK from POST /api/v1/prompts/render, got %d: %s", rrRender.Code, rrRender.Body.String())
		}

		var renderResp struct {
			TemplateID string `json:"template_id"`
			Rendered   string `json:"rendered"`
		}
		if err := json.Unmarshal(rrRender.Body.Bytes(), &renderResp); err != nil {
			t.Fatalf("Failed to decode render response: %v", err)
		}

		if !strings.Contains(renderResp.Rendered, "PRD to Jira User Story") {
			t.Errorf("Rendered output missing expected title")
		}

		// 5. Unregister source
		reqDel, _ := http.NewRequest(http.MethodDelete, "/api/v1/prompts/sources/"+regResp.Source.ID, nil)
		rrDel := httptest.NewRecorder()
		router.ServeHTTP(rrDel, reqDel)

		if rrDel.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK from DELETE source, got %d", rrDel.Code)
		}
	}
}

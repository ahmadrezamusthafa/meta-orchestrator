package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Rollbacks change the shared tool list while other requests read it.
func TestToolRollbackDoesNotRaceWithReads(t *testing.T) {
	defaultToolsMu.Lock()
	saved := append([]ToolDTO(nil), defaultTools...)
	defaultToolsMu.Unlock()
	t.Cleanup(func() {
		defaultToolsMu.Lock()
		defaultTools = saved
		defaultToolsMu.Unlock()
	})
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})
	do := func(method, path, body string) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Errorf("%s %s = %d", method, path, rec.Code)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); do(http.MethodPost, "/api/v1/tools/rollback", `{"tool_id":"bmad","target_version":"v2.3.9"}`) }()
		go func() { defer wg.Done(); do(http.MethodGet, "/api/v1/tools", "") }()
		go func() { defer wg.Done(); do(http.MethodPost, "/api/v1/tools/resolve-matrix", "") }()
	}
	wg.Wait()
}

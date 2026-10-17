package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeGeminiKey looks like a Gemini key to the driver; it is not a real credential.
const fakeGeminiKey = "AIzaFAKE-test-only"

// The key must never travel in the URL: a transport error quotes the URL, and that error is
// written to the task console, broadcast over WebSocket and persisted in the transcript.
func TestAntigravityKeyStaysOutOfURLAndErrors(t *testing.T) {
	var gotQuery, gotHeader, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery, gotHeader, gotPath = r.URL.RawQuery, r.Header.Get("x-goog-api-key"), r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}`))
	}))
	defer srv.Close()

	d := NewAntigravityDriver(fakeGeminiKey, srv.URL)
	if _, err := d.Complete(context.Background(), &LLMRequest{Model: "gemini-x?alt=1", Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if strings.Contains(gotQuery, fakeGeminiKey) || gotHeader != fakeGeminiKey {
		t.Fatalf("key must be sent in the x-goog-api-key header only (query %q, header set %v)", gotQuery, gotHeader != "")
	}
	if !strings.Contains(gotPath, "gemini-x%3Falt=1") {
		t.Fatalf("model must be escaped into one path segment, got path %q", gotPath)
	}

	srv.Close() // now the request fails in the transport
	_, err := d.Complete(context.Background(), &LLMRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err == nil || strings.Contains(err.Error(), fakeGeminiKey) {
		t.Fatalf("transport error must not carry the key: %v", err != nil)
	}
}

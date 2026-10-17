package localaccess

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAllowedHost(t *testing.T) {
	g := New([]string{"devbox.lan"})
	for host, want := range map[string]bool{
		"localhost:8080":     true,
		"127.0.0.1:8080":     true,
		"127.0.0.1":          true,
		"[::1]:8080":         true,
		"app.localhost:5173": true,
		"devbox.lan:8080":    true,
		"evil.example:8080":  false,
		"192.168.1.10:8080":  false,
		"localhost.evil.com": false,
		"":                   false,
	} {
		if got := g.AllowedHost(host); got != want {
			t.Errorf("AllowedHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestAllowedOrigin(t *testing.T) {
	g := New(nil)
	for origin, want := range map[string]bool{
		"":                          true,
		"http://localhost:5173":     true,
		"http://127.0.0.1:8080":     true,
		"https://evil.example":      false,
		"null":                      false,
		"file://":                   false,
		"http://localhost.evil.com": false,
	} {
		if got := g.AllowedOrigin(origin); got != want {
			t.Errorf("AllowedOrigin(%q) = %v, want %v", origin, got, want)
		}
	}
}

func TestMiddleware(t *testing.T) {
	g := New(nil)
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name, host, origin, method string
		want                       int
		wantCORS                   bool
	}{
		{"loopback, no origin", "localhost:8080", "", http.MethodGet, http.StatusOK, false},
		{"loopback origin", "localhost:8080", "http://localhost:5173", http.MethodPost, http.StatusOK, true},
		{"preflight", "localhost:8080", "http://localhost:5173", http.MethodOptions, http.StatusNoContent, true},
		{"cross-site origin", "localhost:8080", "https://evil.example", http.MethodPost, http.StatusForbidden, false},
		{"dns rebinding host", "evil.example:8080", "", http.MethodGet, http.StatusForbidden, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, "/api/v1/tasks", nil)
			req.Host = c.host
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d", rec.Code, c.want)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); (got != "") != c.wantCORS {
				t.Fatalf("Access-Control-Allow-Origin = %q, wantCORS %v", got, c.wantCORS)
			}
			if c.wantCORS && rec.Header().Get("Access-Control-Allow-Origin") == "*" {
				t.Fatal("wildcard CORS must not be sent")
			}
		})
	}
}

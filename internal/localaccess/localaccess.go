// Package localaccess keeps the daemon reachable only from this machine's browser.
//
// The daemon runs agents with shell access, installs tools and holds provider and connector
// credentials, so a web page open in the operator's browser must not be able to drive it.
// Requests are refused when their Host header is not a loopback name (DNS rebinding) or when a
// browser sends an Origin that is not loopback (cross-site requests and WebSocket hijacking).
// Extra hostnames can be allowed with MO_ALLOWED_HOSTS (comma-separated, e.g. a LAN name), for
// operators who deliberately expose the daemon with MO_LISTEN_ADDR.
package localaccess

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// DefaultListenAddr binds the daemon to loopback only.
const DefaultListenAddr = "127.0.0.1:8080"

// ListenAddr returns MO_LISTEN_ADDR, or DefaultListenAddr when it is unset.
func ListenAddr() string {
	if addr := strings.TrimSpace(os.Getenv("MO_LISTEN_ADDR")); addr != "" {
		return addr
	}
	return DefaultListenAddr
}

// Guard decides which hosts and origins may reach the daemon.
type Guard struct {
	extra map[string]bool
}

// FromEnv builds a guard that allows loopback plus the hostnames in MO_ALLOWED_HOSTS.
func FromEnv() *Guard {
	return New(strings.Split(os.Getenv("MO_ALLOWED_HOSTS"), ","))
}

// New builds a guard that allows loopback plus the given hostnames (ports are ignored).
func New(extraHosts []string) *Guard {
	g := &Guard{extra: map[string]bool{}}
	for _, h := range extraHosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			g.extra[hostname(h)] = true
		}
	}
	return g
}

// AllowedHost reports whether a Host header value (host or host:port) may reach the daemon.
func (g *Guard) AllowedHost(hostport string) bool {
	h := strings.ToLower(hostname(hostport))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
		return true
	}
	return g.extra[h]
}

// AllowedOrigin reports whether a browser Origin header may call the daemon. Requests without
// an Origin (curl, the Vite proxy, top-level navigations) are allowed; the Host check still
// applies to them.
func (g *Guard) AllowedOrigin(origin string) bool {
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	return g.AllowedHost(u.Host)
}

// CheckRequest reports whether r passes both the Host and the Origin checks.
func (g *Guard) CheckRequest(r *http.Request) bool {
	return g.AllowedHost(r.Host) && g.AllowedOrigin(r.Header.Get("Origin"))
}

// Middleware refuses requests that fail CheckRequest and answers CORS for allowed origins only.
func (g *Guard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.CheckRequest(r) {
			http.Error(w, "forbidden: the daemon only accepts requests from this machine", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// hostname strips a port and IPv6 brackets from host[:port].
func hostname(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return strings.TrimSuffix(strings.TrimPrefix(hostport, "["), "]")
}

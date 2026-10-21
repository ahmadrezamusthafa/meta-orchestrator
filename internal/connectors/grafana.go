package connectors

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Grafana signs in one of three ways, chosen by extra_settings.auth_mode: a service account token
// (the default), a username and password (basic auth, e.g. admin on a local Grafana), or
// anonymously (a Grafana with [auth.anonymous] enabled). Whatever the credential may do, the MCP
// server runs with grafanaReadOnly, so agents never get write or admin tools.
const (
	GrafanaAuthToken     = "token"
	GrafanaAuthBasic     = "basic"
	GrafanaAuthAnonymous = "anonymous"
)

// grafanaReadOnly are mcp-grafana's flags that drop its create/update and admin tools.
var grafanaReadOnly = []string{"--disable-write", "--disable-admin"}

// grafanaEnv are the variables passed into the Docker form of the server.
var grafanaEnv = []string{"GRAFANA_URL", "GRAFANA_SERVICE_ACCOUNT_TOKEN", "GRAFANA_USERNAME", "GRAFANA_PASSWORD", "GRAFANA_ORG_ID"}

// grafanaSecrets are the credential variables; the ones the chosen mode does not use are removed.
var grafanaSecrets = []string{"GRAFANA_SERVICE_ACCOUNT_TOKEN", "GRAFANA_API_KEY", "GRAFANA_PASSWORD"}

func grafanaAuthMode(it *types.ConnectorItem) string {
	switch mode, _ := it.ExtraSettings["auth_mode"].(string); mode {
	case GrafanaAuthBasic, GrafanaAuthAnonymous:
		return mode
	}
	return GrafanaAuthToken
}

func grafanaSetting(it *types.ConnectorItem, key string) string {
	switch v := it.ExtraSettings[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return fmt.Sprint(int64(v))
	case bool:
		if v {
			return "true"
		}
	}
	return ""
}

func grafanaSkipTLS(it *types.ConnectorItem) bool {
	return grafanaSetting(it, "tls_skip_verify") == "true"
}

// grafanaURL is the base URL with a scheme: "127.0.0.1:3000" becomes http://127.0.0.1:3000 (a
// local Grafana), any other host without one https://.
func grafanaURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" || strings.Contains(raw, "://") {
		return raw
	}
	host := raw
	if i := strings.IndexAny(host, "/"); i >= 0 {
		host = host[:i]
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if isLoopbackHost(host) {
		return "http://" + raw
	}
	return "https://" + raw
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// grafanaURLFor is the URL the server is given: a container reaches the host's loopback Grafana
// through host.docker.internal.
func grafanaURLFor(base string, docker bool) string {
	u, err := url.Parse(base)
	if !docker || err != nil || !isLoopbackHost(u.Hostname()) {
		return base
	}
	host := "host.docker.internal"
	if p := u.Port(); p != "" {
		host += ":" + p
	}
	u.Host = host
	return u.String()
}

// grafanaCredentialsLocked returns the username and the secret (token or password) for the mode.
func (m *Manager) grafanaCredentialsLocked(s stackConnector, it *types.ConnectorItem) (string, string) {
	switch grafanaAuthMode(it) {
	case GrafanaAuthBasic:
		user := strings.TrimSpace(it.Username)
		if user == "" {
			user, _ = resolveEnvValue("GRAFANA_USERNAME")
		}
		pass := ""
		if it.APIToken != "" && !masked(it.APIToken) {
			pass = it.APIToken
		} else if it.MCP != nil && !masked(it.MCP.Env["GRAFANA_PASSWORD"]) {
			pass = strings.TrimSpace(it.MCP.Env["GRAFANA_PASSWORD"])
		}
		if pass == "" {
			pass, _ = resolveEnvValue("GRAFANA_PASSWORD")
		}
		return user, pass
	case GrafanaAuthAnonymous:
		return "", ""
	}
	tok, _ := m.stackTokenLocked(s, it)
	return "", tok
}

// grafanaRuntimeLocked fills the server's URL, credential and settings for the auth mode. It is
// false when a setting the mode needs is missing.
func (m *Manager) grafanaRuntimeLocked(s stackConnector, it *types.ConnectorItem, mcp *types.MCPConfig) bool {
	base := grafanaURL(it.BaseURL)
	if base == "" {
		return false
	}
	for _, k := range grafanaSecrets {
		delete(mcp.Env, k)
	}
	delete(mcp.Env, "GRAFANA_USERNAME")
	mcp.Env["GRAFANA_URL"] = grafanaURLFor(base, mcp.Command == "docker")
	user, secret := m.grafanaCredentialsLocked(s, it)
	switch grafanaAuthMode(it) {
	case GrafanaAuthBasic:
		if user == "" || secret == "" {
			return false
		}
		mcp.Env["GRAFANA_USERNAME"], mcp.Env["GRAFANA_PASSWORD"] = user, secret
	case GrafanaAuthToken:
		if secret == "" {
			return false
		}
		mcp.Env[s.tokenKey] = secret
	}
	if org := grafanaSetting(it, "org_id"); org != "" {
		mcp.Env["GRAFANA_ORG_ID"] = org
	} else {
		delete(mcp.Env, "GRAFANA_ORG_ID")
	}
	if grafanaSkipTLS(it) && !containsArg(mcp.Args, "--tls-skip-verify") {
		mcp.Args = append(mcp.Args, "--tls-skip-verify")
	}
	return true
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// checkGrafana signs in the way the connector is set to and lists one dashboard. It returns who
// signed in and the server, or the problem to show.
func (m *Manager) checkGrafana(ctx context.Context, it *types.ConnectorItem) (who, info, problem string) {
	base := grafanaURL(it.BaseURL)
	if base == "" {
		return "", "", "Grafana URL is required (e.g. https://grafana.example.com or http://127.0.0.1:3000)"
	}
	s, _ := lookupStack("grafana")
	m.mu.RLock()
	user, secret := m.grafanaCredentialsLocked(s, it)
	m.mu.RUnlock()
	mode := grafanaAuthMode(it)
	headers := map[string]string{}
	switch mode {
	case GrafanaAuthBasic:
		if user == "" || secret == "" {
			return "", "", "A Grafana username and password are required (or set GRAFANA_USERNAME and GRAFANA_PASSWORD)"
		}
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+secret))
	case GrafanaAuthToken:
		if secret == "" {
			return "", "", "A Grafana service account token is required (Administration › Users and access › Service accounts; or set GRAFANA_SERVICE_ACCOUNT_TOKEN)"
		}
		headers["Authorization"] = "Bearer " + secret
	}
	if org := grafanaSetting(it, "org_id"); org != "" {
		headers["X-Grafana-Org-Id"] = org
	}
	client := m.client
	if grafanaSkipTLS(it) {
		client = insecureClient(m.client)
	}
	get := func(path string) (int, string, error) {
		return doRequest(ctx, client, http.MethodGet, base+path, headers, "")
	}

	code, body, err := get("/api/search?limit=1")
	switch {
	case err != nil:
		hint := ""
		if strings.Contains(err.Error(), "certificate") {
			hint = " — for a self-signed certificate, turn on Skip TLS verification"
		}
		return "", "", "Could not reach Grafana: " + err.Error() + hint
	case (code == http.StatusUnauthorized || code == http.StatusForbidden) && mode == GrafanaAuthAnonymous:
		return "", "", fmt.Sprintf("Grafana refused anonymous access (HTTP %d) — enable [auth.anonymous] in Grafana, or sign in with a token or a username and password", code)
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return "", "", fmt.Sprintf("Grafana rejected the %s (HTTP %d)", map[string]string{GrafanaAuthBasic: "username or password", GrafanaAuthToken: "token"}[mode], code)
	case code != http.StatusOK:
		return "", "", fmt.Sprintf("Grafana answered HTTP %d: %s", code, firstLine(body))
	}

	switch mode {
	case GrafanaAuthBasic:
		who = user
		if code, body, err := get("/api/user"); err == nil && code == http.StatusOK {
			var u struct {
				Login          string `json:"login"`
				IsGrafanaAdmin bool   `json:"isGrafanaAdmin"`
			}
			if json.Unmarshal([]byte(body), &u) == nil && u.Login != "" {
				who = u.Login
				if u.IsGrafanaAdmin {
					who += " (Grafana admin — agents still get read-only tools)"
				}
			}
		}
	case GrafanaAuthAnonymous:
		who = "anonymous"
	default:
		who = "service account"
	}
	return who, base, ""
}

func insecureClient(c *http.Client) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // the operator turned verification off for this Grafana
	out := *c
	out.Transport = tr
	return &out
}

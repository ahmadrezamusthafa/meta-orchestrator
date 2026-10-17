package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Project-management connectors (ClickUp, Trello, Asana, monday.com): the settings their MCP
// servers need besides the token, and the API call that proves the token works.

// projectSettings fills the non-secret settings a project-management server needs into env. It is
// false when a required one is missing, so the server is not started only to fail every call.
func projectSettings(it *types.ConnectorItem, env map[string]string) bool {
	switch it.ID {
	case "clickup":
		team := clickUpTeam(it)
		if team == "" {
			return false
		}
		env["CLICKUP_TEAM_ID"] = team
	case "trello":
		key := trelloKey(it)
		if key == "" {
			return false
		}
		env["TRELLO_API_KEY"] = key
		if board := strings.TrimSpace(it.TargetEntity); board != "" {
			env["TRELLO_BOARD_ID"] = board
		}
	}
	return true
}

func clickUpTeam(it *types.ConnectorItem) string {
	if team := strings.TrimSpace(it.TargetEntity); team != "" {
		return team
	}
	team, _ := resolveEnvValue("CLICKUP_TEAM_ID")
	return team
}

// trelloKey is the Trello API key (public, unlike the token), kept in the Username field.
func trelloKey(it *types.ConnectorItem) string {
	if key := strings.TrimSpace(it.Username); key != "" {
		return key
	}
	key, _ := resolveEnvValue("TRELLO_API_KEY")
	return key
}

// checkProjectCredential calls the service's "who am I" endpoint with the token. It returns who
// the token belongs to and what it reaches, or the reason it does not work.
func (m *Manager) checkProjectCredential(ctx context.Context, it *types.ConnectorItem, token string) (who, info, problem string) {
	switch it.ID {
	case "clickup":
		return m.checkClickUp(ctx, it, token)
	case "trello":
		return m.checkTrello(ctx, it, token)
	case "asana":
		return m.checkAsana(ctx, it, token)
	case "monday":
		return m.checkMonday(ctx, token)
	}
	return "", "", ""
}

func rejected(service string, code int, body string) string {
	if code == http.StatusUnauthorized || code == http.StatusForbidden {
		return fmt.Sprintf("%s rejected the token (HTTP %d) — check it is current and copied in full", service, code)
	}
	return fmt.Sprintf("%s answered HTTP %d: %s", service, code, firstLine(body))
}

func (m *Manager) checkClickUp(ctx context.Context, it *types.ConnectorItem, token string) (string, string, string) {
	if token == "" {
		return "", "", "A ClickUp personal API token is required (Settings › Apps › API Token, starts with pk_; or set CLICKUP_API_KEY)"
	}
	auth := map[string]string{"Authorization": token} // ClickUp personal tokens are sent without "Bearer"
	code, body, err := m.doJSON(ctx, http.MethodGet, "https://api.clickup.com/api/v2/user", auth, "")
	if err != nil {
		return "", "", "Could not reach ClickUp: " + err.Error()
	}
	if code != http.StatusOK {
		return "", "", rejected("ClickUp", code, body)
	}
	var user struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	_ = json.Unmarshal([]byte(body), &user)

	code, body, err = m.doJSON(ctx, http.MethodGet, "https://api.clickup.com/api/v2/team", auth, "")
	if err != nil || code != http.StatusOK {
		return "", "", "ClickUp accepted the token but did not list its workspaces"
	}
	var teams struct {
		Teams []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"teams"`
	}
	_ = json.Unmarshal([]byte(body), &teams)
	var found []string
	want := clickUpTeam(it)
	for _, t := range teams.Teams {
		if t.ID == want {
			return user.User.Username, "workspace " + t.Name, ""
		}
		found = append(found, fmt.Sprintf("%s (%s)", t.ID, t.Name))
	}
	if want == "" {
		return "", "", "Enter the workspace ID the agents should read. This token reaches: " + strings.Join(found, ", ")
	}
	return "", "", fmt.Sprintf("The token cannot reach workspace %s. It reaches: %s", want, strings.Join(found, ", "))
}

func (m *Manager) checkTrello(ctx context.Context, it *types.ConnectorItem, token string) (string, string, string) {
	key := trelloKey(it)
	if key == "" {
		return "", "", "A Trello API key is required (trello.com/power-ups/admin › your Power-Up › API key; or set TRELLO_API_KEY)"
	}
	if token == "" {
		return "", "", "A Trello token is required — authorize it with scope=read from the link next to your API key (or set TRELLO_TOKEN)"
	}
	// The OAuth header keeps the token out of the URL, and so out of any error message.
	auth := map[string]string{"Authorization": fmt.Sprintf(`OAuth oauth_consumer_key="%s", oauth_token="%s"`, key, token)}
	code, body, err := m.doJSON(ctx, http.MethodGet, "https://api.trello.com/1/members/me?fields=username", auth, "")
	if err != nil {
		return "", "", "Could not reach Trello: " + err.Error()
	}
	if code != http.StatusOK {
		return "", "", rejected("Trello", code, body)
	}
	var me struct {
		Username string `json:"username"`
	}
	_ = json.Unmarshal([]byte(body), &me)
	info := "token scope checked"
	if code, body, err := m.doJSON(ctx, http.MethodGet, "https://api.trello.com/1/tokens/"+token+"?fields=permissions", auth, ""); err == nil && code == http.StatusOK {
		var t struct {
			Permissions []struct {
				Write bool `json:"write"`
			} `json:"permissions"`
		}
		_ = json.Unmarshal([]byte(body), &t)
		info = "read-only token"
		for _, p := range t.Permissions {
			if p.Write {
				info = "token can also write — authorize one with scope=read for read-only agents"
				break
			}
		}
	}
	if board := strings.TrimSpace(it.TargetEntity); board != "" {
		code, _, err := m.doJSON(ctx, http.MethodGet, "https://api.trello.com/1/boards/"+board+"?fields=name", auth, "")
		if err != nil || code != http.StatusOK {
			return "", "", fmt.Sprintf("The token cannot open board %s (HTTP %d)", board, code)
		}
	}
	return me.Username, info, ""
}

func (m *Manager) checkAsana(ctx context.Context, it *types.ConnectorItem, token string) (string, string, string) {
	if token == "" {
		return "", "", "An Asana personal access token is required (My settings › Apps › Developer apps; or set ASANA_ACCESS_TOKEN)"
	}
	code, body, err := m.doJSON(ctx, http.MethodGet, "https://app.asana.com/api/1.0/users/me?opt_fields=name,workspaces.name",
		map[string]string{"Authorization": "Bearer " + token}, "")
	if err != nil {
		return "", "", "Could not reach Asana: " + err.Error()
	}
	if code != http.StatusOK {
		return "", "", rejected("Asana", code, body)
	}
	var me struct {
		Data struct {
			Name       string `json:"name"`
			Workspaces []struct {
				GID  string `json:"gid"`
				Name string `json:"name"`
			} `json:"workspaces"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(body), &me)
	var names []string
	want := strings.TrimSpace(it.TargetEntity)
	for _, w := range me.Data.Workspaces {
		if want != "" && (w.GID == want || strings.EqualFold(w.Name, want)) {
			return me.Data.Name, "workspace " + w.Name, ""
		}
		names = append(names, w.Name)
	}
	if want != "" {
		return "", "", fmt.Sprintf("The token cannot reach workspace %s. It reaches: %s", want, strings.Join(names, ", "))
	}
	return me.Data.Name, "workspaces " + strings.Join(names, ", "), ""
}

func (m *Manager) checkMonday(ctx context.Context, token string) (string, string, string) {
	if token == "" {
		return "", "", "A monday.com API token is required (avatar › Developers › My access tokens; or set MONDAY_API_TOKEN)"
	}
	code, body, err := m.doJSON(ctx, http.MethodPost, "https://api.monday.com/v2",
		map[string]string{"Authorization": token, "Content-Type": "application/json"}, `{"query":"{ me { name } account { name } }"}`)
	if err != nil {
		return "", "", "Could not reach monday.com: " + err.Error()
	}
	var v struct {
		Data struct {
			Me struct {
				Name string `json:"name"`
			} `json:"me"`
			Account struct {
				Name string `json:"name"`
			} `json:"account"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal([]byte(body), &v)
	if code != http.StatusOK || len(v.Errors) > 0 || v.Data.Me.Name == "" {
		return "", "", rejected("monday.com", code, body)
	}
	return v.Data.Me.Name, "account " + v.Data.Account.Name, ""
}

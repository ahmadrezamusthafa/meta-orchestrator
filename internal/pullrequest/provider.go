package pullrequest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Credentials authenticate against the hosting provider's REST API.
type Credentials struct {
	Username string // Bitbucket account email / username; unused for GitHub
	Token    string // Bitbucket API token or app password; GitHub personal access token
}

// Request is one pull request to open (or refresh) on a repository.
type Request struct {
	Owner        string
	Repo         string
	SourceBranch string
	TargetBranch string
	Title        string
	Body         string
}

// Result reports the opened pull request.
type Result struct {
	URL     string `json:"url"`
	Number  int    `json:"number"`
	Updated bool   `json:"updated"` // an open pull request for the branch already existed and was refreshed
}

// Client opens pull requests. BaseURL overrides the provider API root (tests).
type Client struct {
	HTTP    *http.Client
	BaseURL string
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// Open creates the pull request, or updates the title and description of the open pull request
// already filed for the same source branch.
func (c *Client) Open(ctx context.Context, provider string, creds Credentials, req Request) (*Result, error) {
	if strings.TrimSpace(creds.Token) == "" {
		return nil, fmt.Errorf("no %s credentials — connect %s in Connectors", provider, provider)
	}
	switch provider {
	case "bitbucket":
		return c.openBitbucket(ctx, creds, req)
	case "github":
		return c.openGitHub(ctx, creds, req)
	}
	return nil, fmt.Errorf("opening pull requests on %q is not supported — open it manually with the generated description", provider)
}

func (c *Client) base(def string) string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return def
}

func (c *Client) call(ctx context.Context, method, endpoint string, auth func(*http.Request), payload, out interface{}) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Meta-Orchestrator")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	auth(req)
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s — %s", method, stripQuery(endpoint), resp.Status, apiMessage(raw))
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func stripQuery(u string) string {
	if i := strings.Index(u, "?"); i >= 0 {
		return u[:i]
	}
	return u
}

// apiMessage extracts the provider's error text without echoing whole response bodies.
func apiMessage(raw []byte) string {
	var e struct {
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(raw, &e) == nil {
		msg := e.Error.Message
		if msg == "" {
			msg = e.Message
		}
		for _, x := range e.Errors {
			if x.Message != "" {
				msg += "; " + x.Message
			}
		}
		if msg != "" {
			return msg
		}
	}
	s := strings.TrimSpace(string(raw))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// --- Bitbucket Cloud ---------------------------------------------------------------------------

type bbPR struct {
	ID    int `json:"id"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}

func (c *Client) openBitbucket(ctx context.Context, creds Credentials, r Request) (*Result, error) {
	auth := func(req *http.Request) {
		if creds.Username != "" {
			req.SetBasicAuth(creds.Username, creds.Token)
		} else {
			req.Header.Set("Authorization", "Bearer "+creds.Token)
		}
	}
	root := fmt.Sprintf("%s/repositories/%s/%s/pullrequests", c.base("https://api.bitbucket.org/2.0"),
		url.PathEscape(r.Owner), url.PathEscape(r.Repo))

	q := fmt.Sprintf(`source.branch.name="%s" AND state="OPEN"`, r.SourceBranch)
	var list struct {
		Values []bbPR `json:"values"`
	}
	if err := c.call(ctx, http.MethodGet, root+"?q="+url.QueryEscape(q), auth, nil, &list); err != nil {
		return nil, err
	}
	payload := map[string]interface{}{
		"title":       r.Title,
		"description": r.Body,
		"destination": map[string]interface{}{"branch": map[string]string{"name": r.TargetBranch}},
	}
	if len(list.Values) > 0 {
		var pr bbPR
		if err := c.call(ctx, http.MethodPut, fmt.Sprintf("%s/%d", root, list.Values[0].ID), auth, payload, &pr); err != nil {
			return nil, err
		}
		return &Result{URL: pr.Links.HTML.Href, Number: pr.ID, Updated: true}, nil
	}
	payload["source"] = map[string]interface{}{"branch": map[string]string{"name": r.SourceBranch}}
	payload["close_source_branch"] = true
	var pr bbPR
	if err := c.call(ctx, http.MethodPost, root, auth, payload, &pr); err != nil {
		return nil, err
	}
	return &Result{URL: pr.Links.HTML.Href, Number: pr.ID}, nil
}

// --- GitHub ------------------------------------------------------------------------------------

type ghPR struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
}

func (c *Client) openGitHub(ctx context.Context, creds Credentials, r Request) (*Result, error) {
	auth := func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+creds.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	root := fmt.Sprintf("%s/repos/%s/%s/pulls", c.base("https://api.github.com"), url.PathEscape(r.Owner), url.PathEscape(r.Repo))

	var open []ghPR
	q := url.Values{"state": {"open"}, "head": {r.Owner + ":" + r.SourceBranch}}
	if err := c.call(ctx, http.MethodGet, root+"?"+q.Encode(), auth, nil, &open); err != nil {
		return nil, err
	}
	if len(open) > 0 {
		var pr ghPR
		payload := map[string]string{"title": r.Title, "body": r.Body, "base": r.TargetBranch}
		if err := c.call(ctx, http.MethodPatch, fmt.Sprintf("%s/%d", root, open[0].Number), auth, payload, &pr); err != nil {
			return nil, err
		}
		return &Result{URL: pr.HTMLURL, Number: pr.Number, Updated: true}, nil
	}
	var pr ghPR
	payload := map[string]string{"title": r.Title, "body": r.Body, "head": r.SourceBranch, "base": r.TargetBranch}
	if err := c.call(ctx, http.MethodPost, root, auth, payload, &pr); err != nil {
		return nil, err
	}
	return &Result{URL: pr.HTMLURL, Number: pr.Number}, nil
}

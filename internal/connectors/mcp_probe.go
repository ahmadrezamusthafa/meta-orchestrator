package connectors

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// ProbeResult is what an MCP server told us about itself.
type ProbeResult struct {
	Server    string   `json:"server"`
	Version   string   `json:"version,omitempty"`
	Tools     int      `json:"tools"`
	ToolNames []string `json:"tool_names,omitempty"` // the first few, for display
}

// probeTimeout covers a first start that downloads the server (npx, uvx, docker pull).
const probeTimeout = 90 * time.Second

// ProbeMCP starts a stdio MCP server, performs the initialize handshake and lists its tools, then
// stops it. It proves the server really starts with these credentials, which a PATH check cannot.
func ProbeMCP(ctx context.Context, cfg types.MCPConfig) (*ProbeResult, error) {
	if t := strings.ToLower(cfg.Transport); t != "" && t != "stdio" {
		return nil, fmt.Errorf("only stdio MCP servers can be probed (this one uses %s)", cfg.Transport)
	}
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, errors.New("the MCP server command is empty")
	}
	if _, err := exec.LookPath(cfg.Command); err != nil {
		return nil, fmt.Errorf("`%s` is not installed or not on PATH", cfg.Command)
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &tailBuffer{max: 4 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("could not start `%s`: %w", cfg.Command, err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	lines := make(chan []byte)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64<<10), 8<<20) // tools/list answers can be large
		for sc.Scan() {
			b := append([]byte(nil), sc.Bytes()...)
			select {
			case lines <- b:
			case <-ctx.Done():
				return
			}
		}
	}()

	send := func(msg map[string]interface{}) error {
		b, _ := json.Marshal(msg)
		_, err := stdin.Write(append(b, '\n'))
		return err
	}
	await := func(id int) (json.RawMessage, error) {
		for {
			select {
			case <-ctx.Done():
				return nil, failure("timed out waiting for the MCP server", stderr)
			case line, ok := <-lines:
				if !ok {
					return nil, failure("the MCP server exited", stderr)
				}
				var resp struct {
					ID     *int            `json:"id"`
					Result json.RawMessage `json:"result"`
					Error  *struct {
						Message string `json:"message"`
					} `json:"error"`
				}
				if json.Unmarshal(line, &resp) != nil || resp.ID == nil || *resp.ID != id {
					continue // log lines, notifications and other replies
				}
				if resp.Error != nil {
					return nil, fmt.Errorf("the MCP server answered with an error: %s", resp.Error.Message)
				}
				return resp.Result, nil
			}
		}
	}

	if err := send(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]interface{}{
		"protocolVersion": "2025-03-26", "capabilities": map[string]interface{}{},
		"clientInfo": map[string]string{"name": "meta-orchestrator", "version": "1.0.0"},
	}}); err != nil {
		return nil, failure("could not talk to the MCP server", stderr)
	}
	raw, err := await(1)
	if err != nil {
		return nil, err
	}
	var init struct {
		ServerInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	_ = json.Unmarshal(raw, &init)
	res := &ProbeResult{Server: init.ServerInfo.Name, Version: init.ServerInfo.Version}

	_ = send(map[string]interface{}{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if err := send(map[string]interface{}{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]interface{}{}}); err != nil {
		return res, nil
	}
	raw, err = await(2)
	if err != nil {
		return nil, err
	}
	var list struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	_ = json.Unmarshal(raw, &list)
	res.Tools = len(list.Tools)
	for i, t := range list.Tools {
		if i == 8 {
			break
		}
		res.ToolNames = append(res.ToolNames, t.Name)
	}
	return res, nil
}

func failure(what string, stderr *tailBuffer) error {
	if tail := lastLines(stderr.String(), 3); tail != "" {
		return fmt.Errorf("%s: %s", what, tail)
	}
	return errors.New(what)
}

func lastLines(s string, n int) string {
	var keep []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			keep = append(keep, l)
		}
	}
	if len(keep) > n {
		keep = keep[len(keep)-n:]
	}
	out := strings.Join(keep, " | ")
	if len(out) > 400 {
		out = out[len(out)-400:]
	}
	return out
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf.Write(p)
	if over := t.buf.Len() - t.max; over > 0 {
		t.buf.Next(over)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buf.String()
}

var _ io.Writer = (*tailBuffer)(nil)

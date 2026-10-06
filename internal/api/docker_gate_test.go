package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
)

func TestHeavyDockerCommand(t *testing.T) {
	heavy := []string{"docker build -t app .", "docker compose up -d", "docker-compose -f e2e.yml up --build",
		"cd web && docker compose -p e2e build api", "sudo docker run --rm node:20 npm test", "DOCKER_BUILDKIT=1 docker build .",
		"docker pull postgres:16", "docker exec app rails db:migrate", "podman run alpine", "docker logs -f app"}
	light := []string{"docker ps", "docker compose ps", "docker compose -f x.yml config", "docker images | grep app",
		"docker inspect app", "git status", "echo docker build", "grep -r docker ."}
	for _, c := range heavy {
		if !heavyDockerCommand(c) {
			t.Errorf("%q should be heavy", c)
		}
	}
	for _, c := range light {
		if heavyDockerCommand(c) {
			t.Errorf("%q should not take a Docker slot", c)
		}
	}
}

func TestLimitContainers(t *testing.T) {
	if got := limitContainers("docker run --rm node:20 npm test", "2"); got != "docker run --cpus=2 --rm node:20 npm test" {
		t.Fatalf("got %q", got)
	}
	if got := limitContainers("docker run --cpus=4 app", "2"); got != "docker run --cpus=4 app" {
		t.Fatalf("an explicit limit must be kept: %q", got)
	}
	if got := limitContainers("docker run app", "0"); got != "docker run app" {
		t.Fatalf("\"0\" disables the limit: %q", got)
	}
}

func allowAll(context.Context, llm.ApprovalRequest) llm.ApprovalDecision {
	return llm.ApprovalDecision{Allow: true}
}

func TestDockerGateLimitsConcurrentCommands(t *testing.T) {
	g := newDockerGate(DockerLimits{MaxConcurrent: 1, CPUs: "1"})
	var notes []string
	approve := g.wrap(allowAll, "TASK-1", "turn-1", func(s string) { notes = append(notes, s) })
	run := func(id, cmd string) llm.ApprovalDecision {
		return approve(context.Background(), llm.ApprovalRequest{ToolName: "Bash", ToolUseID: id,
			Input: map[string]interface{}{"command": cmd, "run_in_background": true}})
	}

	d := run("t1", "docker run app")
	if !d.Allow || d.UpdatedInput["command"] != "docker run --cpus=1 app" || d.UpdatedInput["run_in_background"] != false {
		t.Fatalf("first command: %+v", d)
	}
	if d := run("t2", "docker ps"); !d.Allow {
		t.Fatal("a light command must not wait for a slot")
	}

	done := make(chan llm.ApprovalDecision)
	go func() { done <- run("t3", "docker compose build") }()
	select {
	case <-done:
		t.Fatal("second heavy command must wait while the slot is held")
	case <-time.After(100 * time.Millisecond):
	}
	g.release("t1")
	select {
	case d := <-done:
		if !d.Allow {
			t.Fatalf("queued command: %+v", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued command did not start after the slot was freed")
	}
	if len(notes) == 0 || !strings.Contains(strings.Join(notes, "\n"), "Docker limit reached") {
		t.Fatalf("operator was not told about the wait: %v", notes)
	}

	g.releaseTurn("turn-1")
	if len(g.slots) != 0 || len(g.held) != 0 {
		t.Fatalf("slots left after turn end: %d / %v", len(g.slots), g.held)
	}
}

func TestDockerGateGivesUpWhenTheTurnStops(t *testing.T) {
	g := newDockerGate(DockerLimits{MaxConcurrent: 1})
	approve := g.wrap(allowAll, "TASK-1", "turn-1", func(string) {})
	approve(context.Background(), llm.ApprovalRequest{ToolName: "Bash", ToolUseID: "t1", Input: map[string]interface{}{"command": "docker build ."}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if d := approve(ctx, llm.ApprovalRequest{ToolName: "Bash", ToolUseID: "t2", Input: map[string]interface{}{"command": "docker build ."}}); d.Allow {
		t.Fatal("a stopped turn must not start the command")
	}
	if _, ok := g.held["t2"]; ok {
		t.Fatal("cancelled wait must not hold a slot")
	}
}

func TestDeniedDockerCommandTakesNoSlot(t *testing.T) {
	g := newDockerGate(DockerLimits{MaxConcurrent: 1})
	deny := func(context.Context, llm.ApprovalRequest) llm.ApprovalDecision { return llm.ApprovalDecision{} }
	g.wrap(deny, "TASK-1", "turn-1", func(string) {})(context.Background(),
		llm.ApprovalRequest{ToolName: "Bash", ToolUseID: "t1", Input: map[string]interface{}{"command": "docker build ."}})
	if len(g.slots) != 0 {
		t.Fatal("a denied command must not hold a slot")
	}
}

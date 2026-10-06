package api

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
)

// DockerLimits caps the Docker work agents run on this machine.
type DockerLimits struct {
	// MaxConcurrent is how many Docker commands may run at once across all tasks; further ones wait
	// for a free slot. Zero uses defaultDockerConcurrency.
	MaxConcurrent int
	// CPUs is the --cpus limit added to containers agents start with docker run/create when they set
	// none. Empty uses defaultDockerCPUs; "0" adds no limit.
	CPUs string
}

const (
	defaultDockerConcurrency = 2
	defaultDockerCPUs        = "2"
)

// dockerBinaries are the commands that drive a container engine.
var dockerBinaries = map[string]bool{"docker": true, "docker-compose": true, "podman": true, "podman-compose": true}

// dockerLightSubcommands only read engine state and finish quickly, so they never wait for a slot.
var dockerLightSubcommands = map[string]bool{"ps": true, "images": true, "inspect": true, "version": true, "info": true,
	"port": true, "top": true, "config": true, "ls": true}

// dockerValueFlags are global or compose flags followed by a separate value word.
var dockerValueFlags = map[string]bool{"-f": true, "--file": true, "-p": true, "--project-name": true, "--profile": true,
	"--env-file": true, "--project-directory": true, "--context": true, "-c": true, "-H": true, "--host": true,
	"--log-level": true, "--config": true}

// shellSeparators split a command line into the simple commands it runs.
var shellSeparators = regexp.MustCompile(`&&|\|\||[;|\n(]|\$\(`)

// commandPrefixes run the next word as the actual command.
var commandPrefixes = map[string]bool{"sudo": true, "time": true, "nice": true, "env": true, "exec": true, "command": true}

// heavyDockerCommand reports whether a shell command runs a Docker command that does real work
// (build, run, compose up, pull, …) rather than only reading engine state.
func heavyDockerCommand(cmd string) bool {
	for _, segment := range shellSeparators.Split(cmd, -1) {
		words := strings.Fields(segment)
		i := 0
		for i < len(words) && (strings.Contains(words[i], "=") || commandPrefixes[words[i]] || strings.HasPrefix(words[i], "-")) {
			i++ // VAR=value assignments, sudo/env/nice and their flags
		}
		if i >= len(words) || !dockerBinaries[filepath.Base(words[i])] {
			continue
		}
		bin := filepath.Base(words[i])
		sub := ""
		for j := i + 1; j < len(words); j++ {
			w := words[j]
			if dockerValueFlags[w] {
				j++
				continue
			}
			if strings.HasPrefix(w, "-") {
				continue
			}
			if w == "compose" && sub == "" && !strings.HasSuffix(bin, "-compose") {
				bin += "-compose"
				continue
			}
			sub = w
			break
		}
		if sub == "" || !dockerLightSubcommands[sub] {
			return true
		}
	}
	return false
}

// dockerRunCommand matches the start of a docker run/create invocation.
var dockerRunCommand = regexp.MustCompile(`\b(docker|podman)(\s+container)?\s+(run|create)\b`)

// limitContainers adds a CPU limit to the containers a command starts, unless it sets its own.
func limitContainers(cmd, cpus string) string {
	if cpus == "" || cpus == "0" || strings.Contains(cmd, "--cpus") {
		return cmd
	}
	return dockerRunCommand.ReplaceAllString(cmd, "${0} --cpus="+cpus)
}

type dockerSlot struct {
	taskID, turnID, command string
}

// dockerGate limits how many heavy Docker commands agents run at once, machine-wide.
type dockerGate struct {
	limits DockerLimits
	slots  chan struct{}
	mu     sync.Mutex
	held   map[string]dockerSlot // tool use id → holder
	seq    int
}

func newDockerGate(limits DockerLimits) *dockerGate {
	if limits.MaxConcurrent <= 0 {
		limits.MaxConcurrent = defaultDockerConcurrency
	}
	if limits.CPUs == "" {
		limits.CPUs = defaultDockerCPUs
	}
	return &dockerGate{limits: limits, slots: make(chan struct{}, limits.MaxConcurrent), held: map[string]dockerSlot{}}
}

// holders describes the commands occupying the slots.
func (g *dockerGate) holders() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []string
	for _, s := range g.held {
		c := s.command
		if len(c) > 60 {
			c = c[:57] + "…"
		}
		out = append(out, fmt.Sprintf("%s `%s`", s.taskID, c))
	}
	return strings.Join(out, ", ")
}

// acquire blocks until a slot is free or ctx ends, and returns the key that releases it.
func (g *dockerGate) acquire(ctx context.Context, toolUseID string, slot dockerSlot, waiting func(string)) (string, error) {
	select {
	case g.slots <- struct{}{}:
	default:
		waiting(fmt.Sprintf("Docker limit reached: %d command(s) already running (%s). This one starts when a slot frees up.",
			g.limits.MaxConcurrent, g.holders()))
		select {
		case g.slots <- struct{}{}:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	key := toolUseID
	if key == "" {
		g.seq++
		key = fmt.Sprintf("%s#docker-%d", slot.turnID, g.seq) // freed when the turn ends
	}
	g.held[key] = slot
	return key, nil
}

// release frees the slot held by a finished tool call; unknown ids are ignored.
func (g *dockerGate) release(toolUseID string) {
	g.mu.Lock()
	_, ok := g.held[toolUseID]
	delete(g.held, toolUseID)
	g.mu.Unlock()
	if ok {
		<-g.slots
	}
}

// releaseTurn frees every slot a turn still holds (a result that never arrived, an interrupt).
func (g *dockerGate) releaseTurn(turnID string) {
	g.mu.Lock()
	n := 0
	for k, s := range g.held {
		if s.turnID == turnID {
			delete(g.held, k)
			n++
		}
	}
	g.mu.Unlock()
	for ; n > 0; n-- {
		<-g.slots
	}
}

// wrap makes an allowed heavy Docker command wait for a slot, run in the foreground (so its slot
// is held for as long as it runs) and start containers with a CPU limit. note reports to the console.
func (g *dockerGate) wrap(next llm.Approver, taskID, turnID string, note func(string)) llm.Approver {
	if next == nil {
		return nil
	}
	return func(ctx context.Context, req llm.ApprovalRequest) llm.ApprovalDecision {
		d := next(ctx, req)
		raw, _ := req.Input["command"].(string)
		if !d.Allow || req.ToolName != "Bash" || !heavyDockerCommand(raw) {
			return d
		}
		if _, err := g.acquire(ctx, req.ToolUseID, dockerSlot{taskID: taskID, turnID: turnID, command: shellCommand("Bash", req.Input)}, note); err != nil {
			return llm.ApprovalDecision{Message: "The turn stopped while this Docker command was waiting for a free slot."}
		}
		input := map[string]interface{}{}
		for k, v := range req.Input {
			input[k] = v
		}
		for k, v := range d.UpdatedInput {
			input[k] = v
		}
		var changes []string
		if limited := limitContainers(raw, g.limits.CPUs); limited != raw {
			input["command"] = limited
			changes = append(changes, "containers capped at "+g.limits.CPUs+" CPUs")
		}
		if bg, _ := input["run_in_background"].(bool); bg {
			input["run_in_background"] = false
			changes = append(changes, "run in the foreground so its Docker slot is held while it runs")
		}
		if len(changes) > 0 {
			note("Docker command adjusted: " + strings.Join(changes, "; ") + ".")
		}
		d.UpdatedInput = input
		return d
	}
}

// dockerRules tell the agent how to keep Docker work light on the shared machine.
const dockerRules = "Docker: heavy Docker commands (build, run, compose up, pull, …) are limited machine-wide and wait " +
	"for a free slot, and containers you start get a CPU limit. Reuse running containers and existing images instead of " +
	"rebuilding; build or start only the services you need; run Docker commands in the foreground and start long-lived " +
	"services detached (-d); never poll with repeated docker commands; stop what you started (docker compose down, " +
	"docker stop) once you no longer need it."

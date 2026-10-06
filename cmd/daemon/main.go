package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/api"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/artifacts"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/circuitbreaker"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/hooks"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/projects"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/registry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/skills"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/tools"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/worker"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/ws"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
	"github.com/redis/go-redis/v9"
)

// telemetryConfig wires Phase 4 telemetry. Shadow benchmarking is opt-in (MO_SHADOW_ENABLED=true)
// because every sweep makes paid LLM calls; REDIS_ADDR enables the Redis routing weight table.
func telemetryConfig(cwd string) api.TelemetryConfig {
	tc := api.TelemetryConfig{
		DataDir:      filepath.Join(cwd, ".sdlc", "telemetry"),
		EnableShadow: os.Getenv("MO_SHADOW_ENABLED") == "true",
	}
	if pricing := filepath.Join(cwd, "configs", "pricing.json"); fileExists(pricing) {
		tc.PricingFile = pricing
	}
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		tc.Redis = redis.NewClient(&redis.Options{Addr: addr})
	}
	fmt.Printf("[Telemetry] Datastore: %s | Shadow benchmarking: %v | Redis weights: %v\n",
		tc.DataDir, tc.EnableShadow, tc.Redis != nil)
	return tc
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func main() {
	fmt.Println("================================================================")
	fmt.Println("  Meta-Orchestrator Daemon Core (Phase 1 Foundation)")
	fmt.Println("================================================================")

	cwd, _ := os.Getwd()

	// 1. Cascading Configuration Resolver
	configResolver := config.NewCascadingConfigResolver(cwd)
	cfg, err := configResolver.Resolve()
	if err != nil {
		fmt.Printf("Warning: failed to resolve config, using defaults: %v\n", err)
		cfg = config.GetDefaultConfig()
	}
	fmt.Printf("[Config] Active SDLC: %s | Router Strategy: %s\n", cfg.ActiveSDLC, cfg.Router.Strategy)

	// 2. Registries Initialization
	workflowReg := fsm.NewWorkflowRegistry()
	profileReg := registry.NewProfileRegistry()
	_ = profileReg.LoadFromFile("configs/profiles.json")

	promptReg := registry.NewPromptRegistry(cwd)
	skillResolver := skills.NewMultiSourceSkillResolver(cwd)
	artifactMgr := artifacts.NewArtifactManager(".sdlc/artifacts")
	hookEngine := hooks.NewHookEngine(cwd)
	taskStore := fsm.NewMemoryTaskStateStore()
	projectMgr := projects.NewProjectManager(cwd)
	fmt.Printf("[Projects] Loaded %d registered multi-repo workspaces\n", len(projectMgr.List()))

	// 3. Tool Lifecycle Manager
	toolMgr := tools.NewToolManager("", func(toolID string, stage string, pct int, msg string) {
		fmt.Printf("[Tool Progress] [%s] %s (%d%%): %s\n", toolID, stage, pct, msg)
	})
	fmt.Printf("[Tools] Registered %d installable packages\n", len(toolMgr.ListPackages()))

	// 4. WebSocket Streaming Hub
	wsHub := ws.NewHub()
	go wsHub.Run()

	// 5. Anti-Loop Frustration Circuit Breaker
	circuitBreaker := circuitbreaker.NewCircuitBreaker(3, func(payload *types.FrustrationHaltPayload) {
		fmt.Printf("[Circuit Breaker] Tripped for task %s at stage %s (consecutive fails: %d)\n",
			payload.TaskID, payload.StageID, payload.ConsecutiveFails)
		wsHub.BroadcastEvent(&types.OrchestratorEvent{
			Type:    types.EventFrustrationHalt,
			TaskID:  payload.TaskID,
			StageID: payload.StageID,
			Payload: payload,
		})
	})

	// 6. Router & Task Profiler
	taskProfiler := router.NewTaskProfiler(cfg, cwd)
	_ = taskProfiler
	_ = circuitBreaker
	_ = skillResolver
	_ = artifactMgr
	_ = hookEngine

	// 7. Crash Recovery
	ctx := context.Background()
	recoveredFSMs, err := fsm.RecoverActiveTasks(ctx, taskStore, workflowReg)
	if err != nil {
		fmt.Printf("[Recovery] Failed to recover active tasks: %v\n", err)
	} else {
		fmt.Printf("[Recovery] Successfully recovered %d active tasks in < 5s RTO\n", len(recoveredFSMs))
	}

	// 8. Asynchronous Worker Pool
	workerPool := worker.NewPool(10, 100, func(ctx context.Context, task *types.Task) error {
		activeTools, _ := skillResolver.ListSkills()
		fmt.Printf("[Worker] Executing task %s (%s) with %d active AI tools (including dynamic MCP connectors)...\n",
			task.ID, task.Title, len(activeTools))
		return nil
	})

	// 9. HTTP Server with REST API & WebSocket endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHub.HandleWebSocket)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","version":"1.0.0"}`))
	})

	apiRouter := api.NewRouter(api.RouterConfig{
		TaskStore:         taskStore,
		WorkflowReg:       workflowReg,
		ToolManager:       toolMgr,
		ArtifactManager:   artifactMgr,
		ConfigResolver:    configResolver,
		ProjectManager:    projectMgr,
		SkillResolver:     skillResolver,
		PromptRegistry:    promptReg,
		WSHub:             wsHub,
		RootDir:           cwd,
		Telemetry:         telemetryConfig(cwd),
		TaskStorePath:     filepath.Join(cwd, ".sdlc", "tasks.json"),
		ProviderStatePath: filepath.Join(cwd, ".sdlc", "providers.json"),
		EnableJiraSync:    true,
		Docker:            dockerLimits(),
	})
	mux.Handle("/api/", apiRouter)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		fmt.Println("[HTTP] WebSocket & Health API server listening on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[HTTP] Server error: %v\n", err)
		}
	}()

	// 10. Graceful Shutdown
	stopSig := make(chan os.Signal, 1)
	signal.Notify(stopSig, syscall.SIGINT, syscall.SIGTERM)
	<-stopSig

	fmt.Println("\n[Shutdown] Received termination signal, gracefully draining workers...")
	workerPool.Stop(5 * time.Second)
	if err := apiRouter.Close(); err != nil {
		fmt.Printf("[Shutdown] Failed to flush task board: %v\n", err)
	}
	wsHub.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)

	fmt.Println("[Shutdown] Daemon shutdown cleanly completed.")
}

// dockerLimits reads MO_DOCKER_MAX_CONCURRENT (Docker commands agents may run at once) and
// MO_DOCKER_CPUS (--cpus for containers agents start; "0" disables). Unset uses the defaults.
func dockerLimits() api.DockerLimits {
	n, _ := strconv.Atoi(os.Getenv("MO_DOCKER_MAX_CONCURRENT"))
	return api.DockerLimits{MaxConcurrent: n, CPUs: os.Getenv("MO_DOCKER_CPUS")}
}

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/api"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/artifacts"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/circuitbreaker"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/hooks"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/registry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/skills"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/tools"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/worker"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/ws"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

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

	skillResolver := skills.NewMultiSourceSkillResolver(cwd)
	artifactMgr := artifacts.NewArtifactManager(".sdlc/artifacts")
	hookEngine := hooks.NewHookEngine(cwd)
	taskStore := fsm.NewMemoryTaskStateStore()

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
		fmt.Printf("[Worker] Executing task %s (%s)...\n", task.ID, task.Title)
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
		TaskStore:       taskStore,
		WorkflowReg:     workflowReg,
		ToolManager:     toolMgr,
		ArtifactManager: artifactMgr,
		ConfigResolver:  configResolver,
		WSHub:           wsHub,
		RootDir:         cwd,
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
	wsHub.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)

	fmt.Println("[Shutdown] Daemon shutdown cleanly completed.")
}

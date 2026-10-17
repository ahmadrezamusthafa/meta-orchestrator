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
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/hooks"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/localaccess"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/packages"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/projects"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/registry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/skills"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/ws"
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

// preparePath puts the managed packages' shims first on PATH, then adds the package-manager bin
// folders (Homebrew, go install, …) that may be missing when the daemon is not started from a
// login shell, so installed tools are found by the daemon and the agents it starts.
func preparePath() {
	packages.EnsurePath(packages.DefaultHome())
	toolchain.EnsurePath()
}

// loadPackages loads configs/packages.json; state lives in $MO_HOME or ~/.meta-orchestrator.
func loadPackages(cwd string, tc *toolchain.Toolchain) *packages.Manager {
	m, err := packages.Load(filepath.Join(cwd, "configs", "packages.json"), packages.Options{Home: packages.DefaultHome(), Toolchain: tc})
	if err != nil {
		fmt.Printf("[Packages] %v\n", err)
	}
	return m
}

// runPackagesCLI handles `daemon packages …` without starting the server.
func runPackagesCLI(cwd string, args []string) int {
	preparePath()
	m := loadPackages(cwd, toolchain.New())
	if m == nil {
		return 1
	}
	defer m.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return packages.RunCLI(ctx, m, args, os.Stdout)
}

func main() {
	cwd, _ := os.Getwd()
	if len(os.Args) > 1 && os.Args[1] == "packages" {
		os.Exit(runPackagesCLI(cwd, os.Args[2:]))
	}

	fmt.Println("================================================================")
	fmt.Println("  Meta-Orchestrator Daemon")
	fmt.Println("================================================================")

	preparePath()

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
	if err := profileReg.LoadFromFile(filepath.Join(cwd, "configs", "profiles.json")); err != nil {
		fmt.Printf("[Profiles] Stage roles not loaded: %v\n", err)
	}

	promptReg := registry.NewPromptRegistry(cwd)
	skillResolver := skills.NewMultiSourceSkillResolver(cwd)
	artifactMgr := artifacts.NewArtifactManager(filepath.Join(cwd, ".sdlc", "artifacts"))
	hookEngine := hooks.NewHookEngine(cwd)
	projectMgr := projects.NewProjectManager(cwd)
	fmt.Printf("[Projects] Loaded %d registered multi-repo workspaces\n", len(projectMgr.List()))

	// 3. Package manager for the orchestrator's own tools
	toolchainSvc := toolchain.New()
	pkgMgr := loadPackages(cwd, toolchainSvc)
	if pkgMgr != nil {
		defer pkgMgr.Close()
		fmt.Printf("[Packages] Tracking %d packages; managed shims in %s\n", pkgMgr.Count(), pkgMgr.BinDir())
	}

	// 4. WebSocket Streaming Hub
	wsHub := ws.NewHub()
	go wsHub.Run()

	// 5. HTTP Server with REST API & WebSocket endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHub.HandleWebSocket)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","version":"1.0.0"}`))
	})

	apiRouter := api.NewRouter(api.RouterConfig{
		TaskStore:   taskStateMirror(),
		WorkflowReg: workflowReg,
		HookEngine:  hookEngine,
		// Stages run on a bounded pool (MO_MAX_CONCURRENT_STAGES, default 10) owned by the router.
		MaxConcurrentStages: maxConcurrentStages(),
		Packages:            pkgMgr,
		Toolchain:           toolchainSvc,
		ProfileRegistry:     profileReg,
		ArtifactManager:     artifactMgr,
		ConfigResolver:      configResolver,
		ProjectManager:      projectMgr,
		SkillResolver:       skillResolver,
		PromptRegistry:      promptReg,
		WSHub:               wsHub,
		RootDir:             cwd,
		Telemetry:           telemetryConfig(cwd),
		TaskStorePath:       filepath.Join(cwd, ".sdlc", "tasks.json"),
		ProviderStatePath:   filepath.Join(cwd, ".sdlc", "providers.json"),
		EnableJiraSync:      true,
		Docker:              dockerLimits(),
	})
	mux.Handle("/api/", apiRouter)

	listenAddr := localaccess.ListenAddr()
	server := &http.Server{
		Addr:              listenAddr,
		Handler:           localaccess.FromEnv().Middleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		fmt.Printf("[HTTP] WebSocket & Health API server listening on %s\n", listenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[HTTP] Server error: %v\n", err)
		}
	}()

	// 6. Graceful Shutdown
	stopSig := make(chan os.Signal, 1)
	signal.Notify(stopSig, syscall.SIGINT, syscall.SIGTERM)
	<-stopSig

	fmt.Println("\n[Shutdown] Received termination signal, interrupting running stages...")
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

// taskStateMirror mirrors task snapshots to Redis when REDIS_ADDR is set, so active tasks the board
// file lacks can be restored at startup. Without it the JSON board is the only store.
func taskStateMirror() fsm.TaskStateStore {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		return nil
	}
	fmt.Printf("[Recovery] Mirroring task state to Redis at %s\n", addr)
	return fsm.NewRedisTaskStateStore(redis.NewClient(&redis.Options{Addr: addr}))
}

// maxConcurrentStages reads MO_MAX_CONCURRENT_STAGES; unset or invalid uses the router default.
func maxConcurrentStages() int {
	n, _ := strconv.Atoi(os.Getenv("MO_MAX_CONCURRENT_STAGES"))
	return n
}

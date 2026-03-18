package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	kratoslog "github.com/go-kratos/kratos/v2/log"

	"github.com/redhat/mini-rbac-go/internal/api/middleware"
	"github.com/redhat/mini-rbac-go/internal/api/v2"
	"github.com/redhat/mini-rbac-go/internal/application/service"
	"github.com/redhat/mini-rbac-go/internal/infrastructure"
	"github.com/redhat/mini-rbac-go/internal/infrastructure/database"
	"github.com/redhat/mini-rbac-go/internal/infrastructure/kessel"
)

var (
	configPath = flag.String("config", "config/config.yaml", "path to config file")
	// Version info - can be set at build time with -ldflags
	version = "dev"
	commit  = "unknown"
)

func main() {
	flag.Parse()

	// Load configuration
	cfg, err := infrastructure.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("Starting %s %s in %s mode\n", cfg.App.Name, cfg.App.Version, cfg.App.Env)
	fmt.Printf("HTTP server: %s\n", cfg.Server.HTTP.Addr)
	fmt.Printf("Database: %s:%d/%s\n", cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName)
	fmt.Printf("Kessel Relations API: %s\n", cfg.Kessel.RelationsAPI.Address())
	fmt.Printf("Replication enabled: %v\n", cfg.App.ReplicationEnabled)

	// Initialize logger
	logger := kratoslog.NewStdLogger(os.Stdout)

	// Initialize database
	fmt.Println("\n[1/5] Initializing database...")
	db, err := database.New(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Auto-migrate schemas
	fmt.Println("[2/5] Running database migrations...")
	if err := db.AutoMigrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize Kessel client
	fmt.Println("[3/5] Initializing Kessel relations-api client...")
	var replicator service.Replicator
	if cfg.App.ReplicationEnabled {
		kesselClient, err := kessel.NewClient(&cfg.Kessel.RelationsAPI)
		if err != nil {
			log.Printf("Warning: Failed to initialize Kessel client: %v", err)
			log.Println("Continuing with replication disabled...")
			replicator = kessel.NewNoopReplicator()
		} else {
			defer kesselClient.Close()
			replicator = kessel.NewReplicator(kesselClient, true)
		}
	} else {
		replicator = kessel.NewNoopReplicator()
	}

	// Initialize repositories
	roleRepo := database.NewRoleRepository(db.DB)
	tenantRepo := database.NewTenantRepository(db.DB)
	groupRepo := database.NewGroupRepository(db.DB)
	principalRepo := database.NewPrincipalRepository(db.DB)
	bindingRepo := database.NewRoleBindingRepository(db.DB)
	workspaceRepo := database.NewWorkspaceRepository(db.DB)

	// Initialize services
	fmt.Println("[4/5] Initializing application services...")
	roleService := service.NewRoleV2Service(roleRepo, bindingRepo, replicator, db.DB)
	groupService := service.NewGroupService(groupRepo, principalRepo, replicator, db.DB)
	bindingService := service.NewRoleBindingService(bindingRepo, roleRepo, groupRepo, principalRepo, replicator, db.DB)
	workspaceService := service.NewWorkspaceService(workspaceRepo, replicator, db.DB)

	// Initialize handlers
	roleHandler := v2.NewRoleHandler(roleService, logger)
	groupHandler := v2.NewGroupHandler(groupService, logger)
	bindingHandler := v2.NewBindingHandler(bindingService, logger)
	workspaceHandler := v2.NewWorkspaceHandler(workspaceService, logger)
	statusHandler := v2.NewStatusHandler(cfg, commit)

	// Setup router
	fmt.Println("[5/5] Setting up HTTP routes...")
	mux := http.NewServeMux()

	// Public V2 API routes
	router := v2.NewRouter(roleHandler, groupHandler, bindingHandler, workspaceHandler, statusHandler, logger)
	router.RegisterRoutes(mux)

	// Wrap with middleware chain
	// 1. Workspace bootstrap (ensures root/default workspaces exist)
	workspaceBootstrap := middleware.NewWorkspaceBootstrapMiddleware(workspaceService)
	var handler http.Handler = mux
	handler = workspaceBootstrap.Handler(handler)

	// 2. Logging middleware (outermost - logs all requests)
	loggingMiddleware := v2.NewLoggingMiddleware(logger)
	handler = loggingMiddleware(handler)

	// Setup HTTP server
	server := &http.Server{
		Addr:         cfg.Server.HTTP.Addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		fmt.Printf("\n✅ Server started successfully!\n")
		fmt.Printf("📍 Listening on %s\n", cfg.Server.HTTP.Addr)
		fmt.Printf("🔗 Status: http://%s/api/status\n", cfg.Server.HTTP.Addr)
		fmt.Printf("🔗 Health: http://%s/health\n", cfg.Server.HTTP.Addr)
		fmt.Printf("\nPublic API:\n")
		fmt.Printf("  🔗 Roles API: http://%s/api/rbac/v2/roles\n", cfg.Server.HTTP.Addr)
		fmt.Printf("  🔗 Groups API: http://%s/api/rbac/v2/groups\n", cfg.Server.HTTP.Addr)
		fmt.Printf("  🔗 Role Bindings API: http://%s/api/rbac/v2/role-bindings\n", cfg.Server.HTTP.Addr)
		fmt.Printf("  🔗 Workspaces API: http://%s/api/rbac/v2/workspaces\n", cfg.Server.HTTP.Addr)
		fmt.Printf("  📖 OpenAPI Spec: http://%s/api/rbac/v2/openapi.json\n\n", cfg.Server.HTTP.Addr)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n🛑 Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	// Used to avoid "declared and not used" errors
	_ = tenantRepo

	fmt.Println("✅ Server stopped gracefully")
}

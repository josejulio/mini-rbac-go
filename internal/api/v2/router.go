package v2

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-kratos/kratos/v2/log"
)

// Router sets up HTTP routes for V2 API
type Router struct {
	roleHandler      *RoleHandler
	groupHandler     *GroupHandler
	bindingHandler   *BindingHandler
	workspaceHandler *WorkspaceHandler
	statusHandler    *StatusHandler
	logger           *log.Helper
}

// NewRouter creates a new router with all handlers
func NewRouter(
	roleHandler *RoleHandler,
	groupHandler *GroupHandler,
	bindingHandler *BindingHandler,
	workspaceHandler *WorkspaceHandler,
	statusHandler *StatusHandler,
	logger log.Logger,
) *Router {
	return &Router{
		roleHandler:      roleHandler,
		groupHandler:     groupHandler,
		bindingHandler:   bindingHandler,
		workspaceHandler: workspaceHandler,
		statusHandler:    statusHandler,
		logger:           log.NewHelper(logger),
	}
}

// RegisterRoutes registers all V2 API routes
func (r *Router) RegisterRoutes(mux *http.ServeMux) {
	// Note: Middleware is applied at the server level in main.go
	// Note: Endpoints are registered with and without trailing slashes for compatibility

	// Role routes
	mux.HandleFunc("/api/rbac/v2/roles", r.handleRoles)
	mux.HandleFunc("/api/rbac/v2/roles/", r.handleRoleWithID)
	mux.HandleFunc("/api/rbac/v2/roles/:batchDelete", r.roleHandler.BatchDelete)
	mux.HandleFunc("/api/rbac/v2/roles/:batchDelete/", r.roleHandler.BatchDelete)

	// Group routes
	mux.HandleFunc("/api/rbac/v2/groups", r.handleGroups)
	mux.HandleFunc("/api/rbac/v2/groups/", r.handleGroupWithID)

	// Role binding routes
	mux.HandleFunc("/api/rbac/v2/role-bindings", r.handleRoleBindings)
	mux.HandleFunc("/api/rbac/v2/role-bindings/", r.handleRoleBindings)
	mux.HandleFunc("/api/rbac/v2/role-bindings/:batchCreate", r.bindingHandler.BatchCreate)
	mux.HandleFunc("/api/rbac/v2/role-bindings/:batchCreate/", r.bindingHandler.BatchCreate)
	mux.HandleFunc("/api/rbac/v2/role-bindings/by-subject", r.bindingHandler.BySubject)
	mux.HandleFunc("/api/rbac/v2/role-bindings/by-subject/", r.bindingHandler.BySubject)

	// Workspace routes
	mux.HandleFunc("/api/rbac/v2/workspaces", r.handleWorkspaces)
	mux.HandleFunc("/api/rbac/v2/workspaces/", r.handleWorkspaceWithID)

	// OpenAPI spec
	mux.HandleFunc("/api/rbac/v2/openapi.json", r.serveOpenAPI)
	mux.HandleFunc("/api/rbac/v2/openapi.json/", r.serveOpenAPI)

	// Status and health
	mux.HandleFunc("/api/status", r.statusHandler.GetStatus)
	mux.HandleFunc("/api/status/", r.statusHandler.GetStatus)
	mux.HandleFunc("/health", r.statusHandler.GetHealth)
	mux.HandleFunc("/health/", r.statusHandler.GetHealth)
}

// Role route handlers
func (r *Router) handleRoles(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.roleHandler.ListRoles(w, req)
	case http.MethodPost:
		r.roleHandler.CreateRole(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (r *Router) handleRoleWithID(w http.ResponseWriter, req *http.Request) {
	// Handle collection endpoint with trailing slash
	if req.URL.Path == "/api/rbac/v2/roles/" {
		r.handleRoles(w, req)
		return
	}

	switch req.Method {
	case http.MethodGet:
		r.roleHandler.GetRole(w, req)
	case http.MethodPut:
		r.roleHandler.UpdateRole(w, req)
	case http.MethodDelete:
		r.roleHandler.DeleteRole(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Group route handlers
func (r *Router) handleGroups(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.groupHandler.ListGroups(w, req)
	case http.MethodPost:
		r.groupHandler.CreateGroup(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (r *Router) handleGroupWithID(w http.ResponseWriter, req *http.Request) {
	// Handle collection endpoint with trailing slash
	if req.URL.Path == "/api/rbac/v2/groups/" {
		r.handleGroups(w, req)
		return
	}

	// Check if this is a principals operation
	if len(req.URL.Path) > len("/api/rbac/v2/groups/") {
		if req.URL.Path[len(req.URL.Path)-11:] == "/principals" {
			switch req.Method {
			case http.MethodGet:
				r.groupHandler.ListPrincipals(w, req)
			case http.MethodPost:
				r.groupHandler.AddPrincipals(w, req)
			case http.MethodDelete:
				r.groupHandler.RemovePrincipals(w, req)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
	}

	// Regular group operations
	switch req.Method {
	case http.MethodGet:
		r.groupHandler.GetGroup(w, req)
	case http.MethodPut:
		r.groupHandler.UpdateGroup(w, req)
	case http.MethodDelete:
		r.groupHandler.DeleteGroup(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Role binding route handlers
func (r *Router) handleRoleBindings(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.bindingHandler.ListBindings(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Workspace route handlers
func (r *Router) handleWorkspaces(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.workspaceHandler.ListWorkspaces(w, req)
	case http.MethodPost:
		r.workspaceHandler.CreateWorkspace(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (r *Router) handleWorkspaceWithID(w http.ResponseWriter, req *http.Request) {
	// Handle collection endpoint with trailing slash
	if req.URL.Path == "/api/rbac/v2/workspaces/" {
		r.handleWorkspaces(w, req)
		return
	}

	// Check if this is a move operation
	if len(req.URL.Path) > len("/api/rbac/v2/workspaces/") {
		if len(req.URL.Path) >= 5 && req.URL.Path[len(req.URL.Path)-5:] == "/move" {
			if req.Method == http.MethodPost {
				r.workspaceHandler.MoveWorkspace(w, req)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
	}

	// Regular workspace operations
	switch req.Method {
	case http.MethodGet:
		r.workspaceHandler.GetWorkspace(w, req)
	case http.MethodPut:
		r.workspaceHandler.UpdateWorkspace(w, req)
	case http.MethodDelete:
		r.workspaceHandler.DeleteWorkspace(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// serveOpenAPI serves the OpenAPI specification
func (r *Router) serveOpenAPI(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the OpenAPI spec file
	specPath := filepath.Join("api", "openapi.json")
	data, err := os.ReadFile(specPath)
	if err != nil {
		r.logger.Errorf("Failed to read OpenAPI spec: %v", err)
		http.Error(w, "OpenAPI spec not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

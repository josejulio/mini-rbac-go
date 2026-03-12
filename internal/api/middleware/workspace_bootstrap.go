package middleware

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"
)

// WorkspaceBootstrapper is an interface for bootstrapping tenant workspaces
type WorkspaceBootstrapper interface {
	EnsureBuiltInWorkspaces(ctx context.Context, tenantID uuid.UUID) error
}

// WorkspaceBootstrapMiddleware ensures tenant workspaces exist before handling requests
type WorkspaceBootstrapMiddleware struct {
	workspaceService WorkspaceBootstrapper
	bootstrapped     map[string]bool // tenantID -> bootstrapped
	mu               sync.RWMutex
}

// NewWorkspaceBootstrapMiddleware creates a new workspace bootstrap middleware
func NewWorkspaceBootstrapMiddleware(workspaceService WorkspaceBootstrapper) *WorkspaceBootstrapMiddleware {
	return &WorkspaceBootstrapMiddleware{
		workspaceService: workspaceService,
		bootstrapped:     make(map[string]bool),
	}
}

// Handler wraps an http.Handler with workspace bootstrap logic
func (m *WorkspaceBootstrapMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract tenant ID from header
		tenantIDStr := r.Header.Get("TENANT_ID")

		// If no tenant ID, use null UUID (default tenant)
		var tenantID uuid.UUID
		if tenantIDStr == "" {
			tenantID = uuid.UUID{} // null UUID
		} else {
			var err error
			tenantID, err = uuid.Parse(tenantIDStr)
			if err != nil {
				// Invalid tenant ID, continue anyway with null UUID
				tenantID = uuid.UUID{}
			}
		}

		// Check if already bootstrapped (in-memory cache)
		tenantKey := tenantID.String()
		m.mu.RLock()
		alreadyBootstrapped := m.bootstrapped[tenantKey]
		m.mu.RUnlock()

		if !alreadyBootstrapped {
			// Bootstrap workspaces for this tenant
			if err := m.workspaceService.EnsureBuiltInWorkspaces(r.Context(), tenantID); err != nil {
				// Log error but don't fail the request
				// The error might be due to concurrent bootstrap or temporary DB issue
				fmt.Printf("[WorkspaceBootstrap] Warning: failed to bootstrap workspaces for tenant %s: %v\n", tenantKey, err)
			}

			// Mark as bootstrapped
			m.mu.Lock()
			m.bootstrapped[tenantKey] = true
			m.mu.Unlock()
		}

		// Continue to next handler
		next.ServeHTTP(w, r)
	})
}

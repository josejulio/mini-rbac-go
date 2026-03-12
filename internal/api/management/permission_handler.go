package management

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	kratoslog "github.com/go-kratos/kratos/v2/log"

	"github.com/redhat/mini-rbac-go/internal/domain/role"
)

// PermissionHandler handles internal permission management endpoints
type PermissionHandler struct {
	permissionRepo role.PermissionRepository
	logger         *kratoslog.Helper
}

// NewPermissionHandler creates a new permission handler
func NewPermissionHandler(permissionRepo role.PermissionRepository, logger kratoslog.Logger) *PermissionHandler {
	return &PermissionHandler{
		permissionRepo: permissionRepo,
		logger:         kratoslog.NewHelper(logger),
	}
}

// CreatePermissionRequest represents the request to create a permission
type CreatePermissionRequest struct {
	Application  string `json:"application"`
	ResourceType string `json:"resource_type"`
	Verb         string `json:"verb"`
}

// PermissionResponse represents a permission response
type PermissionResponse struct {
	ID           uint   `json:"id"`
	Application  string `json:"application"`
	ResourceType string `json:"resource_type"`
	Verb         string `json:"verb"`
	V1String     string `json:"v1_string"`
	V2String     string `json:"v2_string"`
}

// BulkCreateRequest for creating multiple permissions at once
type BulkCreateRequest struct {
	Permissions []CreatePermissionRequest `json:"permissions"`
}

// BulkCreateResponse for bulk creation
type BulkCreateResponse struct {
	Created []PermissionResponse `json:"created"`
	Skipped []string             `json:"skipped"` // Already exist
}

// CreatePermission creates a new permission
func (h *PermissionHandler) CreatePermission(w http.ResponseWriter, r *http.Request) {
	var req CreatePermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate
	if err := h.validatePermission(req); err != nil {
		h.respondError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Create permission
	perm := &role.Permission{
		Application:  req.Application,
		ResourceType: req.ResourceType,
		Verb:         req.Verb,
	}

	if err := h.permissionRepo.Create(perm); err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			h.respondError(w, "Permission already exists", http.StatusConflict)
			return
		}
		h.logger.Errorf("Failed to create permission: %v", err)
		h.respondError(w, "Failed to create permission", http.StatusInternalServerError)
		return
	}

	h.respondJSON(w, h.toResponse(perm), http.StatusCreated)
}

// BulkCreatePermissions creates multiple permissions at once
func (h *PermissionHandler) BulkCreatePermissions(w http.ResponseWriter, r *http.Request) {
	var req BulkCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Permissions) == 0 {
		h.respondError(w, "No permissions provided", http.StatusBadRequest)
		return
	}

	response := BulkCreateResponse{
		Created: []PermissionResponse{},
		Skipped: []string{},
	}

	for _, permReq := range req.Permissions {
		// Validate
		if err := h.validatePermission(permReq); err != nil {
			response.Skipped = append(response.Skipped, permReq.Application+":"+permReq.ResourceType+":"+permReq.Verb+" (invalid)")
			continue
		}

		perm := &role.Permission{
			Application:  permReq.Application,
			ResourceType: permReq.ResourceType,
			Verb:         permReq.Verb,
		}

		if err := h.permissionRepo.Create(perm); err != nil {
			if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
				response.Skipped = append(response.Skipped, perm.String()+" (exists)")
			} else {
				response.Skipped = append(response.Skipped, perm.String()+" (error)")
			}
			continue
		}

		response.Created = append(response.Created, h.toResponse(perm))
	}

	h.respondJSON(w, response, http.StatusCreated)
}

// ListPermissions lists all permissions
func (h *PermissionHandler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	permissions, err := h.permissionRepo.List(0, 1000)
	if err != nil {
		h.logger.Errorf("Failed to list permissions: %v", err)
		h.respondError(w, "Failed to list permissions", http.StatusInternalServerError)
		return
	}

	response := make([]PermissionResponse, len(permissions))
	for i, perm := range permissions {
		response[i] = h.toResponse(perm)
	}

	h.respondJSON(w, map[string]interface{}{
		"permissions": response,
		"count":       len(response),
	}, http.StatusOK)
}

// Helper functions

func (h *PermissionHandler) validatePermission(req CreatePermissionRequest) error {
	if req.Application == "" {
		return fmt.Errorf("application is required")
	}
	if req.ResourceType == "" {
		return fmt.Errorf("resource_type is required")
	}
	if req.Verb == "" {
		return fmt.Errorf("verb is required")
	}
	// Basic format validation
	if strings.ContainsAny(req.Application, ":@#") {
		return fmt.Errorf("application contains invalid characters")
	}
	if strings.ContainsAny(req.ResourceType, ":@#") {
		return fmt.Errorf("resource_type contains invalid characters")
	}
	if strings.ContainsAny(req.Verb, ":@#") {
		return fmt.Errorf("verb contains invalid characters")
	}
	return nil
}

func (h *PermissionHandler) toResponse(perm *role.Permission) PermissionResponse {
	return PermissionResponse{
		ID:           perm.ID,
		Application:  perm.Application,
		ResourceType: perm.ResourceType,
		Verb:         perm.Verb,
		V1String:     perm.String(),
		V2String:     perm.V2String(),
	}
}

func (h *PermissionHandler) respondJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *PermissionHandler) respondError(w http.ResponseWriter, message string, status int) {
	h.respondJSON(w, map[string]interface{}{
		"error":  message,
		"status": status,
	}, status)
}

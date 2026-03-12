package v2

import (
	"encoding/json"
	"net/http"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"

	"github.com/redhat/mini-rbac-go/internal/application/service"
	"github.com/redhat/mini-rbac-go/internal/domain/role"
)

// RoleHandler handles role-related HTTP requests
type RoleHandler struct {
	roleService *service.RoleV2Service
	logger      *log.Helper
}

// NewRoleHandler creates a new role handler
func NewRoleHandler(roleService *service.RoleV2Service, logger log.Logger) *RoleHandler {
	return &RoleHandler{
		roleService: roleService,
		logger:      log.NewHelper(logger),
	}
}

// CreateRole handles POST /api/rbac/v2/roles
func (h *RoleHandler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req CreateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warnf("Failed to decode CreateRole request: %v", err)
		h.writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Validate request
	if req.Name == "" {
		h.logger.Warn("CreateRole: missing required field 'name'")
		h.writeError(w, http.StatusBadRequest, "Validation failed", "name is required")
		return
	}
	if len(req.Permissions) == 0 {
		h.logger.Warn("CreateRole: no permissions provided")
		h.writeError(w, http.StatusBadRequest, "Validation failed", "at least one permission is required")
		return
	}

	tenantID := ExtractTenantID(r)

	// Convert permissions
	permissions := make([]map[string]string, len(req.Permissions))
	for i, p := range req.Permissions {
		permissions[i] = map[string]string{
			"application":   p.Application,
			"resource_type": p.ResourceType,
			"permission":    p.Permission,
		}
	}

	desc := req.Description
	roleV2, err := h.roleService.Create(r.Context(), service.CreateRoleInput{
		Name:        req.Name,
		Description: &desc,
		Permissions: permissions,
		TenantID:    tenantID,
	})
	if err != nil {
		h.logger.Errorf("Failed to create role '%s': %v", req.Name, err)
		h.writeError(w, http.StatusInternalServerError, "Failed to create role", err.Error())
		return
	}

	h.logger.Infof("Created role: id=%s, name=%s", roleV2.UUID, roleV2.Name)
	h.writeJSON(w, http.StatusCreated, h.toRoleResponse(roleV2))
}

// ListRoles handles GET /api/rbac/v2/roles
func (h *RoleHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	tenantID := ExtractTenantID(r)

	// TODO: Parse pagination params
	offset := 0
	limit := 20

	roles, err := h.roleService.List(r.Context(), tenantID, offset, limit)
	if err != nil {
		h.logger.Errorf("Failed to list roles: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to list roles", err.Error())
		return
	}

	response := PaginatedResponse{
		Meta: PaginationMeta{Count: len(roles)},
		Data: h.toRoleResponses(roles),
	}

	h.writeJSON(w, http.StatusOK, response)
}

// GetRole handles GET /api/rbac/v2/roles/{id}
func (h *RoleHandler) GetRole(w http.ResponseWriter, r *http.Request) {
	// Extract role ID from path
	// TODO: Use proper router parameter extraction
	roleIDStr := r.URL.Path[len("/api/rbac/v2/roles/"):]
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		h.logger.Warnf("GetRole: invalid UUID format '%s': %v", roleIDStr, err)
		h.writeError(w, http.StatusBadRequest, "Invalid role ID", err.Error())
		return
	}

	tenantID := ExtractTenantID(r)

	roleV2, err := h.roleService.Get(r.Context(), roleID, tenantID)
	if err != nil {
		h.logger.Errorf("Failed to get role: %v", err)
		h.writeError(w, http.StatusNotFound, "Role not found", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, h.toRoleResponse(roleV2))
}

// UpdateRole handles PUT /api/rbac/v2/roles/{id}
func (h *RoleHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	roleIDStr := r.URL.Path[len("/api/rbac/v2/roles/"):]
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid role ID", err.Error())
		return
	}

	var req UpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	tenantID := ExtractTenantID(r)

	permissions := make([]map[string]string, len(req.Permissions))
	for i, p := range req.Permissions {
		permissions[i] = map[string]string{
			"application":   p.Application,
			"resource_type": p.ResourceType,
			"permission":    p.Permission,
		}
	}

	desc := req.Description
	roleV2, err := h.roleService.Update(r.Context(), service.UpdateRoleInput{
		UUID:        roleID,
		Name:        req.Name,
		Description: &desc,
		Permissions: permissions,
		TenantID:    tenantID,
	})
	if err != nil {
		h.logger.Errorf("Failed to update role: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to update role", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, h.toRoleResponse(roleV2))
}

// DeleteRole handles DELETE /api/rbac/v2/roles/{id}
func (h *RoleHandler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	roleIDStr := r.URL.Path[len("/api/rbac/v2/roles/"):]
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid role ID", err.Error())
		return
	}

	tenantID := ExtractTenantID(r)

	if err := h.roleService.Delete(r.Context(), roleID, tenantID); err != nil {
		h.logger.Errorf("Failed to delete role: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to delete role", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// BatchDelete handles POST /api/rbac/v2/roles/:batchDelete
func (h *RoleHandler) BatchDelete(w http.ResponseWriter, r *http.Request) {
	var req BatchDeleteRolesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warnf("Failed to decode BatchDelete request: %v", err)
		h.writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Validate request
	if len(req.IDs) == 0 {
		h.logger.Warn("BatchDelete: no role IDs provided")
		h.writeError(w, http.StatusBadRequest, "Validation failed", "at least one role ID is required")
		return
	}

	if len(req.IDs) > 100 {
		h.logger.Warnf("BatchDelete: too many IDs (%d), maximum is 100", len(req.IDs))
		h.writeError(w, http.StatusBadRequest, "Validation failed", "maximum 100 roles allowed per batch delete")
		return
	}

	// Parse UUIDs
	roleUUIDs := make([]uuid.UUID, len(req.IDs))
	for i, idStr := range req.IDs {
		roleUUID, err := uuid.Parse(idStr)
		if err != nil {
			h.logger.Warnf("BatchDelete: invalid UUID format '%s': %v", idStr, err)
			h.writeError(w, http.StatusBadRequest, "Invalid role ID", err.Error())
			return
		}
		roleUUIDs[i] = roleUUID
	}

	tenantID := ExtractTenantID(r)

	if err := h.roleService.BatchDelete(r.Context(), roleUUIDs, tenantID); err != nil {
		h.logger.Errorf("Failed to batch delete %d roles: %v", len(roleUUIDs), err)
		h.writeError(w, http.StatusInternalServerError, "Failed to delete roles", err.Error())
		return
	}

	h.logger.Infof("Batch deleted %d roles", len(roleUUIDs))
	w.WriteHeader(http.StatusNoContent)
}

// Helper methods

func (h *RoleHandler) toRoleResponse(r *role.RoleV2) RoleResponse {
	permissions := make([]PermissionDTO, len(r.Permissions))
	for i, p := range r.Permissions {
		permissions[i] = PermissionDTO{
			Application:  p.Application,
			ResourceType: p.ResourceType,
			Permission:   p.Verb,
		}
	}

	desc := ""
	if r.Description != nil {
		desc = *r.Description
	}

	return RoleResponse{
		ID:           r.UUID.String(),
		Name:         r.Name,
		Description:  desc,
		Permissions:  permissions,
		LastModified: r.Modified.Format("2006-01-02T15:04:05Z"),
	}
}

func (h *RoleHandler) toRoleResponses(roles []*role.RoleV2) []RoleResponse {
	responses := make([]RoleResponse, len(roles))
	for i, r := range roles {
		responses[i] = h.toRoleResponse(r)
	}
	return responses
}

func (h *RoleHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *RoleHandler) writeError(w http.ResponseWriter, status int, title, detail string) {
	h.writeJSON(w, status, ErrorResponse{
		Title:  title,
		Detail: detail,
		Status: status,
	})
}

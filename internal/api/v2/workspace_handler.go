package v2

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"

	"github.com/redhat/mini-rbac-go/internal/application/service"
	"github.com/redhat/mini-rbac-go/internal/domain/workspace"
)

// WorkspaceHandler handles workspace-related HTTP requests
type WorkspaceHandler struct {
	workspaceService *service.WorkspaceService
	logger           *log.Helper
}

// NewWorkspaceHandler creates a new workspace handler
func NewWorkspaceHandler(workspaceService *service.WorkspaceService, logger log.Logger) *WorkspaceHandler {
	return &WorkspaceHandler{
		workspaceService: workspaceService,
		logger:           log.NewHelper(logger),
	}
}

// CreateWorkspace handles POST /api/rbac/v2/workspaces
func (h *WorkspaceHandler) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req CreateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warnf("Failed to decode CreateWorkspace request: %v", err)
		h.writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Validate request
	if req.Name == "" {
		h.logger.Warn("CreateWorkspace: missing required field 'name'")
		h.writeError(w, http.StatusBadRequest, "Validation failed", "name is required")
		return
	}

	tenantID := ExtractTenantID(r)

	// Parse parent ID if provided
	var parentID *uuid.UUID
	if req.ParentID != "" {
		pid, err := uuid.Parse(req.ParentID)
		if err != nil {
			h.logger.Warnf("CreateWorkspace: invalid parent UUID format '%s': %v", req.ParentID, err)
			h.writeError(w, http.StatusBadRequest, "Invalid parent ID", err.Error())
			return
		}
		parentID = &pid
	}

	// Default to standard type if not specified
	wsType := workspace.WorkspaceTypeStandard
	if req.Type != "" {
		wsType = workspace.WorkspaceType(req.Type)
	}

	ws, err := h.workspaceService.Create(r.Context(), service.CreateWorkspaceInput{
		Name:        req.Name,
		Description: StringPtr(req.Description),
		Type:        wsType,
		ParentID:    parentID,
		TenantID:    tenantID,
	})
	if err != nil {
		h.logger.Errorf("Failed to create workspace '%s': %v", req.Name, err)
		h.writeError(w, http.StatusInternalServerError, "Failed to create workspace", err.Error())
		return
	}

	h.logger.Infof("Created workspace: id=%s, name=%s, type=%s", ws.ID, ws.Name, ws.Type)
	h.writeJSON(w, http.StatusCreated, h.toWorkspaceResponse(ws))
}

// ListWorkspaces handles GET /api/rbac/v2/workspaces
func (h *WorkspaceHandler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	tenantID := ExtractTenantID(r)

	offset := 0
	limit := 20

	workspaces, err := h.workspaceService.List(r.Context(), tenantID, offset, limit)
	if err != nil {
		h.logger.Errorf("Failed to list workspaces: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to list workspaces", err.Error())
		return
	}

	response := PaginatedResponse{
		Meta: PaginationMeta{Count: len(workspaces)},
		Data: h.toWorkspaceResponses(workspaces),
	}

	h.writeJSON(w, http.StatusOK, response)
}

// GetWorkspace handles GET /api/rbac/v2/workspaces/{id}?include_ancestry=true
func (h *WorkspaceHandler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceIDStr := h.extractIDFromPath(r.URL.Path, "/api/rbac/v2/workspaces/")
	workspaceID, err := uuid.Parse(workspaceIDStr)
	if err != nil {
		h.logger.Warnf("GetWorkspace: invalid UUID format '%s': %v", workspaceIDStr, err)
		h.writeError(w, http.StatusBadRequest, "Invalid workspace ID", err.Error())
		return
	}

	tenantID := ExtractTenantID(r)

	// Check if include_ancestry query parameter is set
	includeAncestry := r.URL.Query().Get("include_ancestry") == "true"

	ws, err := h.workspaceService.Get(r.Context(), workspaceID, tenantID)
	if err != nil {
		h.logger.Errorf("Failed to get workspace: %v", err)
		h.writeError(w, http.StatusNotFound, "Workspace not found", err.Error())
		return
	}

	response := h.toWorkspaceResponse(ws)

	// Include ancestry if requested
	if includeAncestry {
		ancestors, err := h.workspaceService.GetAncestors(r.Context(), workspaceID, tenantID)
		if err != nil {
			h.logger.Errorf("Failed to get workspace ancestors: %v", err)
			h.writeError(w, http.StatusInternalServerError, "Failed to get workspace ancestors", err.Error())
			return
		}

		ancestryItems := make([]WorkspaceAncestryItem, len(ancestors))
		for i, a := range ancestors {
			ancestryItems[i] = WorkspaceAncestryItem{
				ID:       a.ID.String(),
				Name:     a.Name,
				ParentID: func() string {
					if a.ParentID != nil {
						return a.ParentID.String()
					}
					return ""
				}(),
			}
		}
		response.Ancestry = ancestryItems
	}

	h.writeJSON(w, http.StatusOK, response)
}

// UpdateWorkspace handles PUT /api/rbac/v2/workspaces/{id}
func (h *WorkspaceHandler) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceIDStr := h.extractIDFromPath(r.URL.Path, "/api/rbac/v2/workspaces/")
	workspaceID, err := uuid.Parse(workspaceIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid workspace ID", err.Error())
		return
	}

	var req UpdateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	tenantID := ExtractTenantID(r)

	ws, err := h.workspaceService.Update(r.Context(), service.UpdateWorkspaceInput{
		ID:          workspaceID,
		Name:        req.Name,
		Description: StringPtr(req.Description),
		TenantID:    tenantID,
	})
	if err != nil {
		h.logger.Errorf("Failed to update workspace: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to update workspace", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, h.toWorkspaceResponse(ws))
}

// DeleteWorkspace handles DELETE /api/rbac/v2/workspaces/{id}
func (h *WorkspaceHandler) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceIDStr := h.extractIDFromPath(r.URL.Path, "/api/rbac/v2/workspaces/")
	workspaceID, err := uuid.Parse(workspaceIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid workspace ID", err.Error())
		return
	}

	tenantID := ExtractTenantID(r)

	if err := h.workspaceService.Delete(r.Context(), workspaceID, tenantID); err != nil {
		h.logger.Errorf("Failed to delete workspace: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to delete workspace", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// MoveWorkspace handles POST /api/rbac/v2/workspaces/{id}/move?parent_id=new-parent-uuid
func (h *WorkspaceHandler) MoveWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceIDStr := h.extractIDFromPath(r.URL.Path, "/api/rbac/v2/workspaces/")
	workspaceIDStr = strings.TrimSuffix(workspaceIDStr, "/move")
	workspaceID, err := uuid.Parse(workspaceIDStr)
	if err != nil {
		h.logger.Warnf("MoveWorkspace: invalid workspace UUID format '%s': %v", workspaceIDStr, err)
		h.writeError(w, http.StatusBadRequest, "Invalid workspace ID", err.Error())
		return
	}

	// Get parent_id from query parameter
	parentIDStr := r.URL.Query().Get("parent_id")
	if parentIDStr == "" {
		h.logger.Warn("MoveWorkspace: missing required query parameter 'parent_id'")
		h.writeError(w, http.StatusBadRequest, "Validation failed", "parent_id query parameter is required")
		return
	}

	parentID, err := uuid.Parse(parentIDStr)
	if err != nil {
		h.logger.Warnf("MoveWorkspace: invalid parent UUID format '%s': %v", parentIDStr, err)
		h.writeError(w, http.StatusBadRequest, "Invalid parent_id", err.Error())
		return
	}

	tenantID := ExtractTenantID(r)

	ws, err := h.workspaceService.Move(r.Context(), workspaceID, parentID, tenantID)
	if err != nil {
		h.logger.Errorf("Failed to move workspace %s to parent %s: %v", workspaceID, parentID, err)
		h.writeError(w, http.StatusBadRequest, "Failed to move workspace", err.Error())
		return
	}

	h.logger.Infof("Moved workspace: id=%s, new_parent=%s", workspaceID, parentID)
	h.writeJSON(w, http.StatusOK, h.toWorkspaceResponse(ws))
}

// Helper methods

func (h *WorkspaceHandler) toWorkspaceResponse(ws *workspace.Workspace) WorkspaceResponse {
	desc := ""
	if ws.Description != nil {
		desc = *ws.Description
	}

	var parentID string
	if ws.ParentID != nil {
		parentID = ws.ParentID.String()
	}

	return WorkspaceResponse{
		ID:          ws.ID.String(),
		Name:        ws.Name,
		Description: desc,
		Type:        string(ws.Type),
		ParentID:    parentID,
		CreatedAt:   ws.Created.Format("2006-01-02T15:04:05Z"),
		ModifiedAt:  ws.Modified.Format("2006-01-02T15:04:05Z"),
	}
}

func (h *WorkspaceHandler) toWorkspaceResponses(workspaces []*workspace.Workspace) []WorkspaceResponse {
	responses := make([]WorkspaceResponse, len(workspaces))
	for i, ws := range workspaces {
		responses[i] = h.toWorkspaceResponse(ws)
	}
	return responses
}

func (h *WorkspaceHandler) extractIDFromPath(path, prefix string) string {
	return strings.TrimPrefix(path, prefix)
}

func (h *WorkspaceHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *WorkspaceHandler) writeError(w http.ResponseWriter, status int, title, detail string) {
	h.writeJSON(w, status, ErrorResponse{
		Title:  title,
		Detail: detail,
		Status: status,
	})
}

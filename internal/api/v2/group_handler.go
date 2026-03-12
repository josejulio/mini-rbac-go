package v2

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"

	"github.com/redhat/mini-rbac-go/internal/application/service"
	"github.com/redhat/mini-rbac-go/internal/domain/group"
)

// GroupHandler handles group-related HTTP requests
type GroupHandler struct {
	groupService *service.GroupService
	logger       *log.Helper
}

// NewGroupHandler creates a new group handler
func NewGroupHandler(groupService *service.GroupService, logger log.Logger) *GroupHandler {
	return &GroupHandler{
		groupService: groupService,
		logger:       log.NewHelper(logger),
	}
}

// CreateGroup handles POST /api/rbac/v2/groups
func (h *GroupHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warnf("Failed to decode CreateGroup request: %v", err)
		h.writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Validate request
	if req.Name == "" {
		h.logger.Warn("CreateGroup: missing required field 'name'")
		h.writeError(w, http.StatusBadRequest, "Validation failed", "name is required")
		return
	}

	tenantID := ExtractTenantID(r)

	g, err := h.groupService.Create(r.Context(), service.CreateGroupInput{
		Name:        req.Name,
		Description: StringPtr(req.Description),
		TenantID:    tenantID,
	})
	if err != nil {
		h.logger.Errorf("Failed to create group '%s': %v", req.Name, err)
		h.writeError(w, http.StatusInternalServerError, "Failed to create group", err.Error())
		return
	}

	h.logger.Infof("Created group: id=%s, name=%s", g.UUID, g.Name)
	h.writeJSON(w, http.StatusCreated, h.toGroupResponse(g))
}

// ListGroups handles GET /api/rbac/v2/groups
func (h *GroupHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	tenantID := ExtractTenantID(r)

	offset := 0
	limit := 20

	groups, err := h.groupService.List(r.Context(), tenantID, offset, limit)
	if err != nil {
		h.logger.Errorf("Failed to list groups: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to list groups", err.Error())
		return
	}

	response := PaginatedResponse{
		Meta: PaginationMeta{Count: len(groups)},
		Data: h.toGroupResponses(groups),
	}

	h.writeJSON(w, http.StatusOK, response)
}

// GetGroup handles GET /api/rbac/v2/groups/{id}
func (h *GroupHandler) GetGroup(w http.ResponseWriter, r *http.Request) {
	groupIDStr := h.extractIDFromPath(r.URL.Path, "/api/rbac/v2/groups/")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Warnf("GetGroup: invalid UUID format '%s': %v", groupIDStr, err)
		h.writeError(w, http.StatusBadRequest, "Invalid group ID", err.Error())
		return
	}

	tenantID := ExtractTenantID(r)

	g, err := h.groupService.Get(r.Context(), groupID, tenantID)
	if err != nil {
		h.logger.Errorf("Failed to get group: %v", err)
		h.writeError(w, http.StatusNotFound, "Group not found", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, h.toGroupResponse(g))
}

// UpdateGroup handles PUT /api/rbac/v2/groups/{id}
func (h *GroupHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	groupIDStr := h.extractIDFromPath(r.URL.Path, "/api/rbac/v2/groups/")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid group ID", err.Error())
		return
	}

	var req UpdateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	tenantID := ExtractTenantID(r)

	g, err := h.groupService.Update(r.Context(), service.UpdateGroupInput{
		UUID:        groupID,
		Name:        req.Name,
		Description: StringPtr(req.Description),
		TenantID:    tenantID,
	})
	if err != nil {
		h.logger.Errorf("Failed to update group: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to update group", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, h.toGroupResponse(g))
}

// DeleteGroup handles DELETE /api/rbac/v2/groups/{id}
func (h *GroupHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	groupIDStr := h.extractIDFromPath(r.URL.Path, "/api/rbac/v2/groups/")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid group ID", err.Error())
		return
	}

	tenantID := ExtractTenantID(r)

	if err := h.groupService.Delete(r.Context(), groupID, tenantID); err != nil {
		h.logger.Errorf("Failed to delete group: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to delete group", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AddPrincipals handles POST /api/rbac/v2/groups/{id}/principals
func (h *GroupHandler) AddPrincipals(w http.ResponseWriter, r *http.Request) {
	groupIDStr := h.extractIDFromPath(r.URL.Path, "/api/rbac/v2/groups/")
	// Remove /principals suffix
	groupIDStr = strings.TrimSuffix(groupIDStr, "/principals")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Warnf("AddPrincipals: invalid UUID format '%s': %v", groupIDStr, err)
		h.writeError(w, http.StatusBadRequest, "Invalid group ID", err.Error())
		return
	}

	var req AddPrincipalsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warnf("Failed to decode AddPrincipals request: %v", err)
		h.writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Validate request
	if len(req.Principals) == 0 {
		h.logger.Warn("AddPrincipals: no principals provided")
		h.writeError(w, http.StatusBadRequest, "Validation failed", "at least one principal is required")
		return
	}

	tenantID := ExtractTenantID(r)

	if err := h.groupService.AddPrincipals(r.Context(), service.AddPrincipalsInput{
		GroupUUID: groupID,
		UserIDs:   req.Principals,
		TenantID:  tenantID,
	}); err != nil {
		h.logger.Errorf("Failed to add %d principals to group %s: %v", len(req.Principals), groupID, err)
		h.writeError(w, http.StatusInternalServerError, "Failed to add principals", err.Error())
		return
	}

	h.logger.Infof("Added %d principals to group: id=%s", len(req.Principals), groupID)

	// Fetch and return the updated group
	g, err := h.groupService.Get(r.Context(), groupID, tenantID)
	if err != nil {
		h.logger.Errorf("Failed to get updated group: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to get updated group", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, h.toGroupResponse(g))
}

// RemovePrincipals handles DELETE /api/rbac/v2/groups/{id}/principals
func (h *GroupHandler) RemovePrincipals(w http.ResponseWriter, r *http.Request) {
	groupIDStr := h.extractIDFromPath(r.URL.Path, "/api/rbac/v2/groups/")
	groupIDStr = strings.TrimSuffix(groupIDStr, "/principals")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid group ID", err.Error())
		return
	}

	var req RemovePrincipalsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	tenantID := ExtractTenantID(r)

	if err := h.groupService.RemovePrincipals(r.Context(), service.RemovePrincipalsInput{
		GroupUUID: groupID,
		UserIDs:   req.Principals,
		TenantID:  tenantID,
	}); err != nil {
		h.logger.Errorf("Failed to remove principals: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to remove principals", err.Error())
		return
	}

	// Fetch and return the updated group
	g, err := h.groupService.Get(r.Context(), groupID, tenantID)
	if err != nil {
		h.logger.Errorf("Failed to get updated group: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to get updated group", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, h.toGroupResponse(g))
}

// ListPrincipals handles GET /api/rbac/v2/groups/{id}/principals
func (h *GroupHandler) ListPrincipals(w http.ResponseWriter, r *http.Request) {
	groupIDStr := h.extractIDFromPath(r.URL.Path, "/api/rbac/v2/groups/")
	groupIDStr = strings.TrimSuffix(groupIDStr, "/principals")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Warnf("ListPrincipals: invalid UUID format '%s': %v", groupIDStr, err)
		h.writeError(w, http.StatusBadRequest, "Invalid group ID", err.Error())
		return
	}

	tenantID := ExtractTenantID(r)

	// Get the group with principals loaded
	g, err := h.groupService.Get(r.Context(), groupID, tenantID)
	if err != nil {
		h.logger.Errorf("Failed to get group: %v", err)
		h.writeError(w, http.StatusNotFound, "Group not found", err.Error())
		return
	}

	// Convert principals to response format
	principals := make([]PrincipalResponse, len(g.Principals))
	for i, p := range g.Principals {
		principals[i] = PrincipalResponse{
			UserID: p.UserID,
		}
	}

	response := PrincipalListResponse{
		Meta: PaginationMeta{Count: len(principals)},
		Data: principals,
	}

	h.logger.Infof("Listed %d principals for group: id=%s", len(principals), groupID)
	h.writeJSON(w, http.StatusOK, response)
}

// Helper methods

func (h *GroupHandler) toGroupResponse(g *group.Group) GroupResponse {
	desc := ""
	if g.Description != nil {
		desc = *g.Description
	}

	return GroupResponse{
		ID:          g.UUID.String(),
		Name:        g.Name,
		Description: desc,
		CreatedAt:   g.Created.Format("2006-01-02T15:04:05Z"),
		ModifiedAt:  g.Modified.Format("2006-01-02T15:04:05Z"),
	}
}

func (h *GroupHandler) toGroupResponses(groups []*group.Group) []GroupResponse {
	responses := make([]GroupResponse, len(groups))
	for i, g := range groups {
		responses[i] = h.toGroupResponse(g)
	}
	return responses
}

func (h *GroupHandler) extractIDFromPath(path, prefix string) string {
	return strings.TrimPrefix(path, prefix)
}

func (h *GroupHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *GroupHandler) writeError(w http.ResponseWriter, status int, title, detail string) {
	h.writeJSON(w, status, ErrorResponse{
		Title:  title,
		Detail: detail,
		Status: status,
	})
}

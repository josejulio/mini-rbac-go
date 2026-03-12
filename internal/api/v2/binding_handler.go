package v2

import (
	"encoding/json"
	"net/http"

	"github.com/go-kratos/kratos/v2/log"

	"github.com/redhat/mini-rbac-go/internal/application/service"
	"github.com/redhat/mini-rbac-go/internal/domain/rolebinding"
)

// BindingHandler handles role binding HTTP requests
type BindingHandler struct {
	bindingService *service.RoleBindingService
	logger         *log.Helper
}

// NewBindingHandler creates a new binding handler
func NewBindingHandler(bindingService *service.RoleBindingService, logger log.Logger) *BindingHandler {
	return &BindingHandler{
		bindingService: bindingService,
		logger:         log.NewHelper(logger),
	}
}

// BatchCreate handles POST /api/rbac/v2/role-bindings/:batchCreate
func (h *BindingHandler) BatchCreate(w http.ResponseWriter, r *http.Request) {
	var req BatchCreateRoleBindingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warnf("Failed to decode BatchCreate request: %v", err)
		h.writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Validate request
	if len(req.Requests) == 0 {
		h.logger.Warn("BatchCreate: no binding requests provided")
		h.writeError(w, http.StatusBadRequest, "Validation failed", "at least one binding request is required")
		return
	}

	if len(req.Requests) > 100 {
		h.logger.Warnf("BatchCreate: too many requests (%d), maximum is 100", len(req.Requests))
		h.writeError(w, http.StatusBadRequest, "Validation failed", "maximum 100 bindings allowed per batch")
		return
	}

	tenantID := ExtractTenantID(r)

	// Convert to service input
	var createRequests []service.CreateBindingRequest
	for i, item := range req.Requests {
		// Validate required fields
		if item.Resource.ID == "" || item.Resource.Type == "" {
			h.logger.Warnf("BatchCreate: request %d missing resource info", i)
			h.writeError(w, http.StatusBadRequest, "Validation failed", "resource id and type are required")
			return
		}
		if item.Subject.ID == "" || item.Subject.Type == "" {
			h.logger.Warnf("BatchCreate: request %d missing subject info", i)
			h.writeError(w, http.StatusBadRequest, "Validation failed", "subject id and type are required")
			return
		}
		if item.Role.ID == "" {
			h.logger.Warnf("BatchCreate: request %d missing role id", i)
			h.writeError(w, http.StatusBadRequest, "Validation failed", "role id is required")
			return
		}

		createRequests = append(createRequests, service.CreateBindingRequest{
			RoleID:       item.Role.ID,
			ResourceType: item.Resource.Type,
			ResourceID:   item.Resource.ID,
			SubjectType:  item.Subject.Type,
			SubjectID:    item.Subject.ID,
			TenantID:     tenantID,
		})
	}

	created, err := h.bindingService.BatchCreate(r.Context(), createRequests)
	if err != nil {
		h.logger.Errorf("Failed to batch create bindings: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to create role bindings", err.Error())
		return
	}

	// Build response
	var responseItems []RoleBindingItemResponse
	for _, c := range created {
		responseItems = append(responseItems, RoleBindingItemResponse{
			Role:     BindingRoleResponse{ID: c.RoleUUID.String()},
			Subject:  BindingSubjectResponse{ID: c.SubjectUUID.String(), Type: c.SubjectType},
			Resource: BindingResourceResponse{ID: c.ResourceID},
		})
	}

	h.logger.Infof("Batch created %d role bindings", len(created))
	h.writeJSON(w, http.StatusCreated, BatchCreateRoleBindingsResponse{
		RoleBindings: responseItems,
	})
}

// ListBindings handles GET /api/rbac/v2/role-bindings
func (h *BindingHandler) ListBindings(w http.ResponseWriter, r *http.Request) {
	roleID := r.URL.Query().Get("role_id")
	resourceType := r.URL.Query().Get("resource_type")
	resourceID := r.URL.Query().Get("resource_id")
	subjectType := r.URL.Query().Get("subject_type")
	subjectID := r.URL.Query().Get("subject_id")

	tenantID := ExtractTenantID(r)

	var bindings []*rolebinding.RoleBinding
	var err error

	// Currently only supports filtering by resource
	if resourceType != "" && resourceID != "" {
		bindings, err = h.bindingService.ListForResource(r.Context(), resourceType, resourceID, tenantID)
	} else {
		bindings, err = h.bindingService.ListForTenant(r.Context(), tenantID, 0, 100)
	}

	if err != nil {
		h.logger.Errorf("Failed to list bindings: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to list bindings", err.Error())
		return
	}

	// Convert to response items
	var items []RoleBindingListItem
	for _, b := range bindings {
		// Apply filters
		if roleID != "" {
			if b.Role.UUID.String() != roleID {
				continue
			}
		}

		// For each subject in the binding
		for _, g := range b.Groups {
			if subjectType != "" && subjectType != "group" {
				continue
			}
			if subjectID != "" && g.UUID.String() != subjectID {
				continue
			}

			items = append(items, RoleBindingListItem{
				Role:     BindingRoleResponse{ID: b.Role.UUID.String()},
				Subject:  BindingSubjectResponse{ID: g.UUID.String(), Type: "group"},
				Resource: BindingResourceResponse{ID: b.ResourceID},
			})
		}
	}

	response := RoleBindingListResponse{
		Meta: PaginationMeta{Count: len(items)},
		Data: items,
	}

	h.writeJSON(w, http.StatusOK, response)
}

// BySubject handles GET/PUT /api/rbac/v2/role-bindings/by-subject
func (h *BindingHandler) BySubject(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.listBySubject(w, r)
	} else if r.Method == http.MethodPut {
		h.updateBySubject(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// listBySubject handles GET /api/rbac/v2/role-bindings/by-subject
func (h *BindingHandler) listBySubject(w http.ResponseWriter, r *http.Request) {
	resourceType := r.URL.Query().Get("resource_type")
	resourceID := r.URL.Query().Get("resource_id")
	subjectType := r.URL.Query().Get("subject_type")
	subjectID := r.URL.Query().Get("subject_id")

	// Validate required parameters
	if resourceType == "" {
		h.logger.Warn("ListBySubject: missing required parameter 'resource_type'")
		h.writeError(w, http.StatusBadRequest, "Validation failed", "resource_type is required")
		return
	}
	if resourceID == "" {
		h.logger.Warn("ListBySubject: missing required parameter 'resource_id'")
		h.writeError(w, http.StatusBadRequest, "Validation failed", "resource_id is required")
		return
	}

	tenantID := ExtractTenantID(r)

	subjects, err := h.bindingService.ListBySubject(r.Context(), resourceType, resourceID, tenantID, subjectType, subjectID)
	if err != nil {
		h.logger.Errorf("Failed to list bindings by subject: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to list bindings", err.Error())
		return
	}

	// Convert to response
	var items []RoleBindingBySubjectResponse
	for _, s := range subjects {
		var roles []BindingRoleResponse
		for _, r := range s.Roles {
			roles = append(roles, BindingRoleResponse{ID: r.UUID.String()})
		}

		items = append(items, RoleBindingBySubjectResponse{
			Subject:  BindingSubjectResponse{ID: s.SubjectUUID.String(), Type: s.SubjectType},
			Roles:    roles,
			Resource: BindingResourceResponse{ID: s.ResourceID},
		})
	}

	response := PaginatedResponse{
		Meta: PaginationMeta{Count: len(items)},
		Data: items,
	}

	h.writeJSON(w, http.StatusOK, response)
}

// updateBySubject handles PUT /api/rbac/v2/role-bindings/by-subject
func (h *BindingHandler) updateBySubject(w http.ResponseWriter, r *http.Request) {
	resourceType := r.URL.Query().Get("resource_type")
	resourceID := r.URL.Query().Get("resource_id")
	subjectType := r.URL.Query().Get("subject_type")
	subjectID := r.URL.Query().Get("subject_id")

	// Validate required parameters
	if resourceType == "" || resourceID == "" || subjectType == "" || subjectID == "" {
		h.logger.Warn("UpdateBySubject: missing required parameters")
		h.writeError(w, http.StatusBadRequest, "Validation failed", "resource_type, resource_id, subject_type, and subject_id are required")
		return
	}

	var req UpdateRoleBindingBySubjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warnf("Failed to decode UpdateBySubject request: %v", err)
		h.writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	tenantID := ExtractTenantID(r)

	// Extract role IDs (empty array is allowed - means remove all bindings)
	var roleIDs []string
	for _, r := range req.Roles {
		if r.ID == "" {
			h.logger.Warn("UpdateBySubject: role missing id")
			h.writeError(w, http.StatusBadRequest, "Validation failed", "role id is required")
			return
		}
		roleIDs = append(roleIDs, r.ID)
	}

	result, err := h.bindingService.UpdateForSubject(r.Context(), resourceType, resourceID, subjectType, subjectID, roleIDs, tenantID)
	if err != nil {
		h.logger.Errorf("Failed to update bindings for subject: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to update role bindings", err.Error())
		return
	}

	// Build response
	var roles []BindingRoleResponse
	for _, r := range result.Roles {
		roles = append(roles, BindingRoleResponse{ID: r.UUID.String()})
	}

	response := RoleBindingBySubjectResponse{
		Subject:  BindingSubjectResponse{ID: result.SubjectUUID.String(), Type: result.SubjectType},
		Roles:    roles,
		Resource: BindingResourceResponse{ID: result.ResourceID},
	}

	h.logger.Infof("Updated role bindings for subject %s on resource %s/%s: %d roles", subjectID, resourceType, resourceID, len(roles))
	h.writeJSON(w, http.StatusOK, response)
}

// Helper methods

func (h *BindingHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *BindingHandler) writeError(w http.ResponseWriter, status int, title, detail string) {
	h.writeJSON(w, status, ErrorResponse{
		Title:  title,
		Detail: detail,
		Status: status,
	})
}

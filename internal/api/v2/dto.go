package v2

import (
	"net/http"

	"github.com/google/uuid"
)

// Common response structures

// ErrorResponse represents an error response
type ErrorResponse struct {
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	Status int    `json:"status"`
}

// PaginationMeta represents pagination metadata
type PaginationMeta struct {
	Count int `json:"count"`
}

// PaginatedResponse wraps data with pagination metadata
type PaginatedResponse struct {
	Meta PaginationMeta `json:"meta"`
	Data interface{}    `json:"data"`
}

// Role DTOs

// PermissionDTO represents a permission in V2 format
type PermissionDTO struct {
	Application  string `json:"application"`
	ResourceType string `json:"resource_type"`
	Permission   string `json:"permission"`
}

// CreateRoleRequest represents a request to create a role
type CreateRoleRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Permissions []PermissionDTO `json:"permissions"`
}

// UpdateRoleRequest represents a request to update a role
type UpdateRoleRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Permissions []PermissionDTO `json:"permissions"`
}

// BatchDeleteRolesRequest represents a request to delete multiple roles
type BatchDeleteRolesRequest struct {
	IDs []string `json:"ids"` // UUIDs of roles to delete (1-100)
}

// RoleResponse represents a role in the response
type RoleResponse struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Permissions  []PermissionDTO `json:"permissions,omitempty"`
	LastModified string          `json:"last_modified,omitempty"`
}

// Group DTOs

// CreateGroupRequest represents a request to create a group
type CreateGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// UpdateGroupRequest represents a request to update a group
type UpdateGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// GroupResponse represents a group in the response
type GroupResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created,omitempty"`
	ModifiedAt  string `json:"modified,omitempty"`
}

// AddPrincipalsRequest represents a request to add principals to a group
type AddPrincipalsRequest struct {
	Principals []string `json:"principals"` // List of user IDs
}

// RemovePrincipalsRequest represents a request to remove principals from a group
type RemovePrincipalsRequest struct {
	Principals []string `json:"principals"` // List of user IDs
}

// PrincipalResponse represents a principal in the response
type PrincipalResponse struct {
	UserID string `json:"user_id"`
}

// PrincipalListResponse represents a list of principals
type PrincipalListResponse struct {
	Meta PaginationMeta      `json:"meta"`
	Data []PrincipalResponse `json:"data"`
}

// RoleBinding DTOs

// BatchCreateRoleBindingsRequest represents a request to create multiple role bindings
type BatchCreateRoleBindingsRequest struct {
	Requests []CreateBindingItemRequest `json:"requests"`
}

// CreateBindingItemRequest represents a single role binding creation request
type CreateBindingItemRequest struct {
	Resource BindingResourceRequest `json:"resource"`
	Subject  BindingSubjectRequest  `json:"subject"`
	Role     BindingRoleRequest     `json:"role"`
}

// BindingResourceRequest represents resource info in a binding request
type BindingResourceRequest struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// BindingSubjectRequest represents subject info in a binding request
type BindingSubjectRequest struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "group" or "user"
}

// BindingRoleRequest represents role info in a binding request
type BindingRoleRequest struct {
	ID string `json:"id"`
}

// BatchCreateRoleBindingsResponse represents the response for batch create
type BatchCreateRoleBindingsResponse struct {
	RoleBindings []RoleBindingItemResponse `json:"role_bindings"`
}

// RoleBindingItemResponse represents a single created role binding
type RoleBindingItemResponse struct {
	Role     BindingRoleResponse     `json:"role"`
	Subject  BindingSubjectResponse  `json:"subject"`
	Resource BindingResourceResponse `json:"resource"`
}

// BindingRoleResponse represents role in binding response
type BindingRoleResponse struct {
	ID string `json:"id"`
}

// BindingSubjectResponse represents subject in binding response
type BindingSubjectResponse struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// BindingResourceResponse represents resource in binding response
type BindingResourceResponse struct {
	ID string `json:"id"`
}

// RoleBindingBySubjectResponse represents a subject with their roles on a resource
type RoleBindingBySubjectResponse struct {
	Subject  BindingSubjectResponse  `json:"subject"`
	Roles    []BindingRoleResponse   `json:"roles"`
	Resource BindingResourceResponse `json:"resource"`
}

// UpdateRoleBindingBySubjectRequest represents request to update roles for a subject
type UpdateRoleBindingBySubjectRequest struct {
	Roles []BindingRoleRequest `json:"roles"`
}

// RoleBindingListResponse represents a list of role bindings
type RoleBindingListResponse struct {
	Meta PaginationMeta          `json:"meta"`
	Data []RoleBindingListItem   `json:"data"`
}

// RoleBindingListItem represents a role binding in list view
type RoleBindingListItem struct {
	Role     BindingRoleResponse     `json:"role"`
	Subject  BindingSubjectResponse  `json:"subject"`
	Resource BindingResourceResponse `json:"resource"`
}

// Workspace DTOs

// CreateWorkspaceRequest represents a request to create a workspace
type CreateWorkspaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ParentID    string `json:"parent_id,omitempty"`   // UUID of parent workspace (defaults to default workspace if not provided)
}

// UpdateWorkspaceRequest represents a request to update a workspace
type UpdateWorkspaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// WorkspaceResponse represents a workspace in the response
type WorkspaceResponse struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description,omitempty"`
	Type        string                   `json:"type"`
	ParentID    string                   `json:"parent_id,omitempty"`
	CreatedAt   string                   `json:"created,omitempty"`
	ModifiedAt  string                   `json:"modified,omitempty"`
	Ancestry    []WorkspaceAncestryItem  `json:"ancestry,omitempty"` // Included when include_ancestry=true
}

// WorkspaceAncestryItem represents an ancestor workspace in the hierarchy
type WorkspaceAncestryItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id,omitempty"`
}

// Helper functions

// StringPtr returns a pointer to a string
func StringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ParseUUID parses a string to UUID
func ParseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// ParseUUIDs parses a slice of strings to UUIDs
func ParseUUIDs(ss []string) ([]uuid.UUID, error) {
	uuids := make([]uuid.UUID, len(ss))
	for i, s := range ss {
		u, err := uuid.Parse(s)
		if err != nil {
			return nil, err
		}
		uuids[i] = u
	}
	return uuids, nil
}

// ExtractTenantID extracts tenant ID from the TENANT_ID header
// Returns null UUID (00000000-0000-0000-0000-000000000000) if header is not present or invalid
func ExtractTenantID(r *http.Request) uuid.UUID {
	tenantIDStr := r.Header.Get("TENANT_ID")
	if tenantIDStr == "" {
		// Return null UUID as default
		return uuid.UUID{}
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		// Invalid UUID, return null UUID
		return uuid.UUID{}
	}

	return tenantID
}

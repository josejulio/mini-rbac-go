package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/redhat/mini-rbac-go/internal/domain/common"
	"github.com/redhat/mini-rbac-go/internal/domain/workspace"
	"github.com/redhat/mini-rbac-go/internal/infrastructure/kessel"
)

// WorkspaceService handles business logic for Workspace operations
type WorkspaceService struct {
	workspaceRepo workspace.Repository
	replicator    Replicator
	db            *gorm.DB
}

// NewWorkspaceService creates a new WorkspaceService
func NewWorkspaceService(
	workspaceRepo workspace.Repository,
	replicator Replicator,
	db *gorm.DB,
) *WorkspaceService {
	return &WorkspaceService{
		workspaceRepo: workspaceRepo,
		replicator:    replicator,
		db:            db,
	}
}

// CreateWorkspaceInput contains data for creating a workspace
type CreateWorkspaceInput struct {
	Name        string
	Description *string
	Type        workspace.WorkspaceType
	ParentID    *uuid.UUID
	TenantID    uuid.UUID
}

// Create creates a new workspace
func (s *WorkspaceService) Create(ctx context.Context, input CreateWorkspaceInput) (*workspace.Workspace, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Validate parent exists if provided
	if input.ParentID != nil {
		parent, err := s.workspaceRepo.FindByID(*input.ParentID)
		if err != nil {
			return nil, fmt.Errorf("parent workspace not found: %w", err)
		}

		// Validate parent is in same tenant
		if parent.TenantID != input.TenantID {
			return nil, fmt.Errorf("parent workspace must be in the same tenant")
		}

		// Validate parent type constraints
		if input.Type == workspace.WorkspaceTypeDefault && parent.Type != workspace.WorkspaceTypeRoot {
			return nil, fmt.Errorf("default workspace must have root workspace as parent")
		}
	}

	newWorkspace := &workspace.Workspace{
		ID:          uuid.New(),
		Name:        input.Name,
		Description: input.Description,
		Type:        input.Type,
		ParentID:    input.ParentID,
		TenantID:    input.TenantID,
	}

	if err := newWorkspace.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	if err := s.workspaceRepo.Create(newWorkspace); err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Replicate to Kessel - create parent relationship
	if newWorkspace.ParentID != nil {
		parentTuple, err := createWorkspaceParentRelation(newWorkspace.ID, *newWorkspace.ParentID)
		if err != nil {
			return nil, fmt.Errorf("failed to create parent relation tuple: %w", err)
		}

		event := &kessel.ReplicationEvent{
			EventType: "workspace.created",
			Info: map[string]interface{}{
				"workspace_id": newWorkspace.ID.String(),
				"tenant_id":    newWorkspace.TenantID.String(),
			},
			Add:    []*common.RelationTuple{parentTuple},
			Remove: []*common.RelationTuple{},
		}

		if err := s.replicator.Replicate(ctx, event); err != nil {
			return nil, fmt.Errorf("failed to replicate workspace creation: %w", err)
		}
	}

	return newWorkspace, nil
}

// UpdateWorkspaceInput contains data for updating a workspace
type UpdateWorkspaceInput struct {
	ID          uuid.UUID
	Name        string
	Description *string
	TenantID    uuid.UUID
}

// Update updates an existing workspace
// Note: This currently only updates name and description. Parent updates should use a separate Move operation.
func (s *WorkspaceService) Update(ctx context.Context, input UpdateWorkspaceInput) (*workspace.Workspace, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	existingWorkspace, err := s.workspaceRepo.FindByID(input.ID)
	if err != nil {
		return nil, fmt.Errorf("workspace not found: %w", err)
	}

	if existingWorkspace.TenantID != input.TenantID {
		return nil, fmt.Errorf("workspace not found in tenant")
	}

	existingWorkspace.Name = input.Name
	existingWorkspace.Description = input.Description

	if err := s.workspaceRepo.Update(existingWorkspace); err != nil {
		return nil, fmt.Errorf("failed to update workspace: %w", err)
	}

	// Note: Name/description changes don't affect relations, so no replication needed
	// Parent changes would be handled by a separate Move operation

	return existingWorkspace, nil
}

// Delete deletes a workspace by ID
func (s *WorkspaceService) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	existingWorkspace, err := s.workspaceRepo.FindByID(id)
	if err != nil {
		return fmt.Errorf("workspace not found: %w", err)
	}

	if existingWorkspace.TenantID != tenantID {
		return fmt.Errorf("workspace not found in tenant")
	}

	// Prevent deletion of root and default workspaces
	if existingWorkspace.Type == workspace.WorkspaceTypeRoot {
		return fmt.Errorf("cannot delete root workspace")
	}
	if existingWorkspace.Type == workspace.WorkspaceTypeDefault {
		return fmt.Errorf("cannot delete default workspace")
	}

	// Replicate deletion to Kessel - remove parent relationship
	if existingWorkspace.ParentID != nil {
		parentTuple, err := createWorkspaceParentRelation(existingWorkspace.ID, *existingWorkspace.ParentID)
		if err != nil {
			return fmt.Errorf("failed to create parent relation tuple: %w", err)
		}

		event := &kessel.ReplicationEvent{
			EventType: "workspace.deleted",
			Info: map[string]interface{}{
				"workspace_id": existingWorkspace.ID.String(),
				"tenant_id":    existingWorkspace.TenantID.String(),
			},
			Add:    []*common.RelationTuple{},
			Remove: []*common.RelationTuple{parentTuple},
		}

		if err := s.replicator.Replicate(ctx, event); err != nil {
			return fmt.Errorf("failed to replicate workspace deletion: %w", err)
		}
	}

	if err := s.workspaceRepo.Delete(id); err != nil {
		return fmt.Errorf("failed to delete workspace: %w", err)
	}

	return nil
}

// List lists workspaces for a tenant
func (s *WorkspaceService) List(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]*workspace.Workspace, error) {
	return s.workspaceRepo.ListForTenant(tenantID, offset, limit)
}

// Get retrieves a single workspace by ID
func (s *WorkspaceService) Get(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*workspace.Workspace, error) {
	ws, err := s.workspaceRepo.FindByID(id)
	if err != nil {
		return nil, err
	}

	if ws.TenantID != tenantID {
		return nil, fmt.Errorf("workspace not found in tenant")
	}

	return ws, nil
}

// GetRoot retrieves the root workspace for a tenant
func (s *WorkspaceService) GetRoot(ctx context.Context, tenantID uuid.UUID) (*workspace.Workspace, error) {
	return s.workspaceRepo.FindRoot(tenantID)
}

// GetDefault retrieves the default workspace for a tenant
func (s *WorkspaceService) GetDefault(ctx context.Context, tenantID uuid.UUID) (*workspace.Workspace, error) {
	return s.workspaceRepo.FindDefault(tenantID)
}

// GetAncestors retrieves all ancestor workspaces
func (s *WorkspaceService) GetAncestors(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) ([]*workspace.Workspace, error) {
	ws, err := s.workspaceRepo.FindByID(id)
	if err != nil {
		return nil, err
	}

	if ws.TenantID != tenantID {
		return nil, fmt.Errorf("workspace not found in tenant")
	}

	return s.workspaceRepo.GetAncestors(id)
}

// GetDescendants retrieves all descendant workspaces
func (s *WorkspaceService) GetDescendants(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) ([]*workspace.Workspace, error) {
	ws, err := s.workspaceRepo.FindByID(id)
	if err != nil {
		return nil, err
	}

	if ws.TenantID != tenantID {
		return nil, fmt.Errorf("workspace not found in tenant")
	}

	return s.workspaceRepo.GetDescendants(id)
}

// Move moves a workspace to a new parent
func (s *WorkspaceService) Move(ctx context.Context, workspaceID, newParentID, tenantID uuid.UUID) (*workspace.Workspace, error) {
	// Start transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Fetch workspace
	ws, err := s.workspaceRepo.FindByID(workspaceID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("workspace not found: %w", err)
	}

	if ws.TenantID != tenantID {
		tx.Rollback()
		return nil, fmt.Errorf("workspace not found in tenant")
	}

	// Verify it's a standard workspace
	if ws.Type != workspace.WorkspaceTypeStandard {
		tx.Rollback()
		return nil, fmt.Errorf("only standard workspaces can be moved")
	}

	// Fetch new parent workspace
	newParent, err := s.workspaceRepo.FindByID(newParentID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("parent workspace not found: %w", err)
	}

	if newParent.TenantID != tenantID {
		tx.Rollback()
		return nil, fmt.Errorf("parent workspace not found in tenant")
	}

	// Prevent moving under own descendant
	descendants, err := s.workspaceRepo.GetDescendants(workspaceID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to get descendants: %w", err)
	}

	for _, desc := range descendants {
		if desc.ID == newParentID {
			tx.Rollback()
			return nil, fmt.Errorf("cannot move workspace under its own descendant")
		}
	}

	// Check hierarchy depth limits
	newParentAncestors, err := s.workspaceRepo.GetAncestors(newParentID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to get parent ancestors: %w", err)
	}

	// Get max descendant depth of the workspace being moved
	maxDescDepth := 0
	if len(descendants) > 0 {
		// Simple approximation - would need recursive depth calculation for accuracy
		maxDescDepth = len(descendants)
	}

	// Total depth = new parent depth + 1 (the workspace itself) + max descendant depth
	totalDepth := len(newParentAncestors) + 1 + maxDescDepth
	const maxHierarchyDepth = 20 // Matching Python's WORKSPACE_HIERARCHY_DEPTH_LIMIT

	if totalDepth > maxHierarchyDepth {
		tx.Rollback()
		return nil, fmt.Errorf("moving workspace would exceed maximum hierarchy depth of %d", maxHierarchyDepth)
	}

	// Store old parent for replication
	oldParentID := ws.ParentID

	// Update parent
	ws.ParentID = &newParentID

	if err := s.workspaceRepo.Update(ws); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update workspace: %w", err)
	}

	// Generate replication tuples
	var tuplesToAdd []*common.RelationTuple
	var tuplesToRemove []*common.RelationTuple

	// Remove old parent relation if it existed
	if oldParentID != nil {
		oldTuple, err := createWorkspaceParentRelation(workspaceID, *oldParentID)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create old parent tuple: %w", err)
		}
		tuplesToRemove = append(tuplesToRemove, oldTuple)
	}

	// Add new parent relation
	newTuple, err := createWorkspaceParentRelation(workspaceID, newParentID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create new parent tuple: %w", err)
	}
	tuplesToAdd = append(tuplesToAdd, newTuple)

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Replicate to Kessel
	if err := s.replicator.Replicate(ctx, &kessel.ReplicationEvent{
		EventType: "move_workspace",
		Info: map[string]interface{}{
			"workspace_id":    workspaceID.String(),
			"old_parent_id":   func() string { if oldParentID != nil { return oldParentID.String() }; return "" }(),
			"new_parent_id":   newParentID.String(),
			"org_id":          tenantID.String(),
		},
		Add:    tuplesToAdd,
		Remove: tuplesToRemove,
	}); err != nil {
		fmt.Printf("[WorkspaceService] Warning: failed to replicate workspace move: %v\n", err)
	}

	return ws, nil
}

// Helper functions for replication

// createWorkspaceParentRelation creates a parent relationship tuple for a workspace
// Generates: workspace:{child-id}#parent@workspace:{parent-id}
func createWorkspaceParentRelation(childID, parentID uuid.UUID) (*common.RelationTuple, error) {
	workspaceType := common.ObjectType{
		Namespace: "rbac",
		Name:      "workspace",
	}

	resource, err := common.NewObjectReference(workspaceType, childID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to create resource reference: %w", err)
	}

	subject, err := common.NewObjectReference(workspaceType, parentID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to create subject reference: %w", err)
	}

	tuple, err := common.NewRelationTuple(
		*resource,
		"parent",
		*common.NewSubjectReference(*subject, nil),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create relation tuple: %w", err)
	}

	return tuple, nil
}

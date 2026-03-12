package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/redhat/mini-rbac-go/internal/domain/common"
	"github.com/redhat/mini-rbac-go/internal/domain/group"
	"github.com/redhat/mini-rbac-go/internal/domain/role"
	"github.com/redhat/mini-rbac-go/internal/domain/rolebinding"
	"github.com/redhat/mini-rbac-go/internal/infrastructure/kessel"
)

// RoleBindingService handles business logic for RoleBinding operations
// Mirrors the Python RoleBindingService
type RoleBindingService struct {
	bindingRepo rolebinding.Repository
	roleRepo    role.RoleRepository
	groupRepo   group.Repository
	replicator  Replicator
	db          *gorm.DB
}

// NewRoleBindingService creates a new RoleBindingService
func NewRoleBindingService(
	bindingRepo rolebinding.Repository,
	roleRepo role.RoleRepository,
	groupRepo group.Repository,
	replicator Replicator,
	db *gorm.DB,
) *RoleBindingService {
	return &RoleBindingService{
		bindingRepo: bindingRepo,
		roleRepo:    roleRepo,
		groupRepo:   groupRepo,
		replicator:  replicator,
		db:          db,
	}
}

// AssignRoleInput contains data for assigning a role to subjects on a resource
type AssignRoleInput struct {
	RoleUUID     uuid.UUID
	ResourceType string
	ResourceID   string
	SubjectType  string // "group" or "user"
	SubjectUUIDs []uuid.UUID
	TenantID     uuid.UUID
}

// AssignRole assigns a role to subjects on a resource
func (s *RoleBindingService) AssignRole(ctx context.Context, input AssignRoleInput) (*rolebinding.RoleBinding, error) {
	// Validate inputs
	if input.ResourceType == "" {
		return nil, fmt.Errorf("resource_type is required")
	}
	if input.ResourceID == "" {
		return nil, fmt.Errorf("resource_id is required")
	}
	if len(input.SubjectUUIDs) == 0 {
		return nil, fmt.Errorf("at least one subject is required")
	}

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

	// Fetch role
	roleV2, err := s.roleRepo.FindByUUID(input.RoleUUID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("role not found: %w", err)
	}

	// Find or create binding for this role + resource
	existingBinding, err := s.bindingRepo.FindForResourceAndRole(
		input.ResourceType,
		input.ResourceID,
		roleV2.ID,
		input.TenantID,
	)

	var roleBinding *rolebinding.RoleBinding
	var bindingCreated bool

	if err != nil {
		// Create new binding
		roleBinding = &rolebinding.RoleBinding{
			UUID:         uuid.New(), // Generate UUID explicitly
			RoleID:       roleV2.ID,
			Role:         roleV2,
			ResourceType: input.ResourceType,
			ResourceID:   input.ResourceID,
			TenantID:     input.TenantID,
		}

		if err := s.bindingRepo.Create(roleBinding); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create role binding: %w", err)
		}

		bindingCreated = true
	} else {
		roleBinding = existingBinding
		bindingCreated = false
	}

	// Fetch subjects based on type
	var groups []*group.Group
	if input.SubjectType == "group" {
		fetchedGroups, err := s.groupRepo.FindByUUIDs(input.SubjectUUIDs)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to fetch groups: %w", err)
		}
		if len(fetchedGroups) != len(input.SubjectUUIDs) {
			tx.Rollback()
			return nil, fmt.Errorf("some groups not found")
		}

		// Add groups to binding
		roleBinding.Groups = append(roleBinding.Groups, fetchedGroups...)
		groups = fetchedGroups
	} else {
		tx.Rollback()
		return nil, fmt.Errorf("unsupported subject type: %s (only 'group' is currently supported)", input.SubjectType)
	}

	// Update binding with new subjects
	if err := s.bindingRepo.Update(roleBinding); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update role binding: %w", err)
	}

	// Generate replication tuples
	var tuplesToAdd []*common.RelationTuple

	// If binding was created, add binding tuples (role + resource)
	if bindingCreated {
		bindingTuples, err := roleBinding.BindingTuples()
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to generate binding tuples: %w", err)
		}
		tuplesToAdd = append(tuplesToAdd, bindingTuples...)
	}

	// Add subject tuples for each group
	for _, g := range groups {
		subjectTuple, err := roleBinding.GroupSubjectTuple(g)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to generate subject tuple: %w", err)
		}
		tuplesToAdd = append(tuplesToAdd, subjectTuple)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Replicate to Kessel
	if len(tuplesToAdd) > 0 {
		if err := s.replicator.Replicate(ctx, &kessel.ReplicationEvent{
			EventType: "assign_role",
			Info: map[string]interface{}{
				"role_uuid":     input.RoleUUID.String(),
				"resource_type": input.ResourceType,
				"resource_id":   input.ResourceID,
				"org_id":        input.TenantID.String(),
			},
			Add:    tuplesToAdd,
			Remove: nil,
		}); err != nil {
			fmt.Printf("[RoleBindingService] Warning: failed to replicate role assignment: %v\n", err)
		}
	}

	return roleBinding, nil
}

// UnassignRoleInput contains data for unassigning a role from subjects on a resource
type UnassignRoleInput struct {
	RoleUUID     uuid.UUID
	ResourceType string
	ResourceID   string
	SubjectType  string
	SubjectUUIDs []uuid.UUID
	TenantID     uuid.UUID
}

// UnassignRole removes a role assignment from subjects on a resource
func (s *RoleBindingService) UnassignRole(ctx context.Context, input UnassignRoleInput) error {
	// Start transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Fetch role
	roleV2, err := s.roleRepo.FindByUUID(input.RoleUUID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("role not found: %w", err)
	}

	// Find binding
	roleBinding, err := s.bindingRepo.FindForResourceAndRole(
		input.ResourceType,
		input.ResourceID,
		roleV2.ID,
		input.TenantID,
	)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("role binding not found: %w", err)
	}

	// Remove subjects based on type
	var groupsToRemove []*group.Group
	if input.SubjectType == "group" {
		// Build set of UUIDs to remove
		toRemove := make(map[uuid.UUID]bool)
		for _, uuid := range input.SubjectUUIDs {
			toRemove[uuid] = true
		}

		// Filter out groups to remove and track them
		var remainingGroups []*group.Group
		for _, g := range roleBinding.Groups {
			if toRemove[g.UUID] {
				groupsToRemove = append(groupsToRemove, g)
			} else {
				remainingGroups = append(remainingGroups, g)
			}
		}

		roleBinding.Groups = remainingGroups
	} else {
		tx.Rollback()
		return fmt.Errorf("unsupported subject type: %s", input.SubjectType)
	}

	// Check if binding is now orphaned (no subjects)
	isOrphaned := len(roleBinding.Groups) == 0 && len(roleBinding.Principals) == 0

	// Generate replication tuples
	var tuplesToRemoveList []*common.RelationTuple

	if isOrphaned {
		// Delete the binding and remove all tuples
		if err := s.bindingRepo.Delete(roleBinding.UUID); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to delete role binding: %w", err)
		}

		// Remove binding tuples (role + resource)
		bindingTuples, err := roleBinding.BindingTuples()
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to generate binding tuples: %w", err)
		}
		tuplesToRemoveList = append(tuplesToRemoveList, bindingTuples...)
	} else {
		// Update binding
		if err := s.bindingRepo.Update(roleBinding); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to update role binding: %w", err)
		}
	}

	// Remove subject tuples for each removed group
	for _, g := range groupsToRemove {
		subjectTuple, err := roleBinding.GroupSubjectTuple(g)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to generate subject tuple: %w", err)
		}
		tuplesToRemoveList = append(tuplesToRemoveList, subjectTuple)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Replicate to Kessel
	if len(tuplesToRemoveList) > 0 {
		if err := s.replicator.Replicate(ctx, &kessel.ReplicationEvent{
			EventType: "unassign_role",
			Info: map[string]interface{}{
				"role_uuid":     input.RoleUUID.String(),
				"resource_type": input.ResourceType,
				"resource_id":   input.ResourceID,
				"org_id":        input.TenantID.String(),
			},
			Add:    nil,
			Remove: tuplesToRemoveList,
		}); err != nil {
			fmt.Printf("[RoleBindingService] Warning: failed to replicate role unassignment: %v\n", err)
		}
	}

	return nil
}

// ListForResource lists role bindings for a specific resource
func (s *RoleBindingService) ListForResource(
	ctx context.Context,
	resourceType string,
	resourceID string,
	tenantID uuid.UUID,
) ([]*rolebinding.RoleBinding, error) {
	return s.bindingRepo.FindForResource(resourceType, resourceID, tenantID)
}

// ListForTenant lists all role bindings for a tenant
func (s *RoleBindingService) ListForTenant(
	ctx context.Context,
	tenantID uuid.UUID,
	offset int,
	limit int,
) ([]*rolebinding.RoleBinding, error) {
	return s.bindingRepo.ListForTenant(tenantID, offset, limit)
}

// Get retrieves a single role binding by UUID
func (s *RoleBindingService) Get(ctx context.Context, bindingUUID uuid.UUID, tenantID uuid.UUID) (*rolebinding.RoleBinding, error) {
	b, err := s.bindingRepo.FindByUUID(bindingUUID)
	if err != nil {
		return nil, err
	}

	if b.TenantID != tenantID {
		return nil, fmt.Errorf("role binding not found in tenant")
	}

	return b, nil
}

// CreateBindingRequest represents a single binding creation request
type CreateBindingRequest struct {
	RoleID       string
	ResourceType string
	ResourceID   string
	SubjectType  string
	SubjectID    string
	TenantID     uuid.UUID
}

// CreatedBinding represents a successfully created binding
type CreatedBinding struct {
	RoleUUID     uuid.UUID
	RoleName     string
	SubjectUUID  uuid.UUID
	SubjectType  string
	ResourceID   string
	ResourceType string
}

// BatchCreate creates multiple role bindings
func (s *RoleBindingService) BatchCreate(ctx context.Context, requests []CreateBindingRequest) ([]CreatedBinding, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("at least one binding request is required")
	}
	if len(requests) > 100 {
		return nil, fmt.Errorf("maximum 100 bindings allowed per batch")
	}

	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var created []CreatedBinding
	var tuplesToAdd []*common.RelationTuple

	for _, req := range requests {
		roleUUID, err := uuid.Parse(req.RoleID)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("invalid role UUID '%s': %w", req.RoleID, err)
		}

		subjectUUID, err := uuid.Parse(req.SubjectID)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("invalid subject UUID '%s': %w", req.SubjectID, err)
		}

		// Fetch role
		roleV2, err := s.roleRepo.FindByUUID(roleUUID)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("role not found: %w", err)
		}

		// Find or create binding
		existingBinding, err := s.bindingRepo.FindForResourceAndRole(
			req.ResourceType,
			req.ResourceID,
			roleV2.ID,
			req.TenantID,
		)

		var roleBinding *rolebinding.RoleBinding
		var bindingCreated bool

		if err != nil {
			// Create new binding
			roleBinding = &rolebinding.RoleBinding{
				UUID:         uuid.New(),
				RoleID:       roleV2.ID,
				Role:         roleV2,
				ResourceType: req.ResourceType,
				ResourceID:   req.ResourceID,
				TenantID:     req.TenantID,
			}

			if err := s.bindingRepo.Create(roleBinding); err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("failed to create role binding: %w", err)
			}

			// Add binding tuples
			bindingTuples, err := roleBinding.BindingTuples()
			if err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("failed to generate binding tuples: %w", err)
			}
			tuplesToAdd = append(tuplesToAdd, bindingTuples...)
			bindingCreated = true
		} else {
			roleBinding = existingBinding
			bindingCreated = false
		}

		// Add subject based on type
		if req.SubjectType == "group" {
			g, err := s.groupRepo.FindByUUID(subjectUUID)
			if err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("group not found: %w", err)
			}

			roleBinding.Groups = append(roleBinding.Groups, g)

			if err := s.bindingRepo.Update(roleBinding); err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("failed to update role binding: %w", err)
			}

			// Add subject tuple
			subjectTuple, err := roleBinding.GroupSubjectTuple(g)
			if err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("failed to generate subject tuple: %w", err)
			}
			tuplesToAdd = append(tuplesToAdd, subjectTuple)

			created = append(created, CreatedBinding{
				RoleUUID:     roleV2.UUID,
				RoleName:     roleV2.Name,
				SubjectUUID:  g.UUID,
				SubjectType:  "group",
				ResourceID:   req.ResourceID,
				ResourceType: req.ResourceType,
			})
		} else {
			tx.Rollback()
			return nil, fmt.Errorf("unsupported subject type: %s", req.SubjectType)
		}

		_ = bindingCreated
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Replicate to Kessel
	if len(tuplesToAdd) > 0 {
		if err := s.replicator.Replicate(ctx, &kessel.ReplicationEvent{
			EventType: "batch_create_bindings",
			Info: map[string]interface{}{
				"count": len(requests),
			},
			Add:    tuplesToAdd,
			Remove: nil,
		}); err != nil {
			fmt.Printf("[RoleBindingService] Warning: failed to replicate batch create: %v\n", err)
		}
	}

	return created, nil
}

// SubjectWithRoles represents a subject and their roles on a resource
type SubjectWithRoles struct {
	SubjectUUID  uuid.UUID
	SubjectType  string
	Roles        []RoleInfo
	ResourceID   string
	ResourceType string
}

// RoleInfo contains role information
type RoleInfo struct {
	UUID uuid.UUID
	Name string
}

// ListBySubject lists role bindings grouped by subject for a resource
func (s *RoleBindingService) ListBySubject(
	ctx context.Context,
	resourceType string,
	resourceID string,
	tenantID uuid.UUID,
	subjectType string,
	subjectID string,
) ([]SubjectWithRoles, error) {
	bindings, err := s.bindingRepo.FindForResource(resourceType, resourceID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bindings: %w", err)
	}

	// Group by subject
	subjectMap := make(map[uuid.UUID]*SubjectWithRoles)

	for _, binding := range bindings {
		for _, g := range binding.Groups {
			// Apply subject filters if provided
			if subjectType != "" && subjectType != "group" {
				continue
			}
			if subjectID != "" {
				subjectUUID, _ := uuid.Parse(subjectID)
				if g.UUID != subjectUUID {
					continue
				}
			}

			if _, exists := subjectMap[g.UUID]; !exists {
				subjectMap[g.UUID] = &SubjectWithRoles{
					SubjectUUID:  g.UUID,
					SubjectType:  "group",
					Roles:        []RoleInfo{},
					ResourceID:   resourceID,
					ResourceType: resourceType,
				}
			}

			subjectMap[g.UUID].Roles = append(subjectMap[g.UUID].Roles, RoleInfo{
				UUID: binding.Role.UUID,
				Name: binding.Role.Name,
			})
		}
	}

	// Convert map to slice
	result := make([]SubjectWithRoles, 0, len(subjectMap))
	for _, subject := range subjectMap {
		result = append(result, *subject)
	}

	return result, nil
}

// UpdateForSubject replaces all role bindings for a subject on a resource
// Empty roleIDs array removes all bindings for the subject
func (s *RoleBindingService) UpdateForSubject(
	ctx context.Context,
	resourceType string,
	resourceID string,
	subjectType string,
	subjectID string,
	roleIDs []string,
	tenantID uuid.UUID,
) (*SubjectWithRoles, error) {
	subjectUUID, err := uuid.Parse(subjectID)
	if err != nil {
		return nil, fmt.Errorf("invalid subject UUID: %w", err)
	}

	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Fetch subject
	if subjectType != "group" {
		tx.Rollback()
		return nil, fmt.Errorf("unsupported subject type: %s", subjectType)
	}

	g, err := s.groupRepo.FindByUUID(subjectUUID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("subject not found: %w", err)
	}

	// Find all existing bindings for this resource and subject
	resourceBindings, err := s.bindingRepo.FindForResource(resourceType, resourceID, tenantID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to fetch bindings: %w", err)
	}

	var tuplesToAdd []*common.RelationTuple
	var tuplesToRemove []*common.RelationTuple

	// Remove subject from all existing bindings
	for _, binding := range resourceBindings {
		hasSubject := false
		for _, bg := range binding.Groups {
			if bg.UUID == subjectUUID {
				hasSubject = true
				break
			}
		}

		if hasSubject {
			// Remove subject from binding
			var remainingGroups []*group.Group
			for _, bg := range binding.Groups {
				if bg.UUID != subjectUUID {
					remainingGroups = append(remainingGroups, bg)
				}
			}
			binding.Groups = remainingGroups

			// Generate tuple to remove
			subjectTuple, err := binding.GroupSubjectTuple(g)
			if err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("failed to generate subject tuple: %w", err)
			}
			tuplesToRemove = append(tuplesToRemove, subjectTuple)

			// Check if binding is orphaned
			if len(binding.Groups) == 0 && len(binding.Principals) == 0 {
				// Delete binding
				if err := s.bindingRepo.Delete(binding.UUID); err != nil {
					tx.Rollback()
					return nil, fmt.Errorf("failed to delete binding: %w", err)
				}

				// Remove binding tuples
				bindingTuples, err := binding.BindingTuples()
				if err != nil {
					tx.Rollback()
					return nil, fmt.Errorf("failed to generate binding tuples: %w", err)
				}
				tuplesToRemove = append(tuplesToRemove, bindingTuples...)
			} else {
				// Update binding
				if err := s.bindingRepo.Update(binding); err != nil {
					tx.Rollback()
					return nil, fmt.Errorf("failed to update binding: %w", err)
				}
			}
		}
	}

	// Add subject to new role bindings
	var resultRoles []RoleInfo

	for _, roleIDStr := range roleIDs {
		roleUUID, err := uuid.Parse(roleIDStr)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("invalid role UUID '%s': %w", roleIDStr, err)
		}

		// Fetch role
		roleV2, err := s.roleRepo.FindByUUID(roleUUID)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("role not found: %w", err)
		}

		// Find or create binding
		existingBinding, err := s.bindingRepo.FindForResourceAndRole(
			resourceType,
			resourceID,
			roleV2.ID,
			tenantID,
		)

		var roleBinding *rolebinding.RoleBinding
		var bindingCreated bool

		if err != nil {
			// Create new binding
			roleBinding = &rolebinding.RoleBinding{
				UUID:         uuid.New(),
				RoleID:       roleV2.ID,
				Role:         roleV2,
				ResourceType: resourceType,
				ResourceID:   resourceID,
				TenantID:     tenantID,
			}

			if err := s.bindingRepo.Create(roleBinding); err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("failed to create role binding: %w", err)
			}

			// Add binding tuples
			bindingTuples, err := roleBinding.BindingTuples()
			if err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("failed to generate binding tuples: %w", err)
			}
			tuplesToAdd = append(tuplesToAdd, bindingTuples...)
			bindingCreated = true
		} else {
			roleBinding = existingBinding
			bindingCreated = false
		}

		// Add subject to binding
		roleBinding.Groups = append(roleBinding.Groups, g)

		if err := s.bindingRepo.Update(roleBinding); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to update role binding: %w", err)
		}

		// Add subject tuple
		subjectTuple, err := roleBinding.GroupSubjectTuple(g)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to generate subject tuple: %w", err)
		}
		tuplesToAdd = append(tuplesToAdd, subjectTuple)

		resultRoles = append(resultRoles, RoleInfo{
			UUID: roleV2.UUID,
			Name: roleV2.Name,
		})

		_ = bindingCreated
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Replicate to Kessel
	if len(tuplesToAdd) > 0 || len(tuplesToRemove) > 0 {
		if err := s.replicator.Replicate(ctx, &kessel.ReplicationEvent{
			EventType: "update_subject_bindings",
			Info: map[string]interface{}{
				"resource_type": resourceType,
				"resource_id":   resourceID,
				"subject_type":  subjectType,
				"subject_id":    subjectID,
			},
			Add:    tuplesToAdd,
			Remove: tuplesToRemove,
		}); err != nil {
			fmt.Printf("[RoleBindingService] Warning: failed to replicate update: %v\n", err)
		}
	}

	return &SubjectWithRoles{
		SubjectUUID:  g.UUID,
		SubjectType:  "group",
		Roles:        resultRoles,
		ResourceID:   resourceID,
		ResourceType: resourceType,
	}, nil
}

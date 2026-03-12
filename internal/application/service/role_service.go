package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/redhat/mini-rbac-go/internal/domain/common"
	"github.com/redhat/mini-rbac-go/internal/domain/role"
	"github.com/redhat/mini-rbac-go/internal/domain/rolebinding"
	"github.com/redhat/mini-rbac-go/internal/infrastructure/kessel"
)

// RoleV2Service handles business logic for RoleV2 operations
// Mirrors the Python RoleV2Service
type RoleV2Service struct {
	roleRepo    role.RoleRepository
	bindingRepo rolebinding.Repository
	replicator  Replicator
	db          *gorm.DB
}

// Replicator interface for relation replication
type Replicator interface {
	Replicate(ctx context.Context, event *kessel.ReplicationEvent) error
}

// NewRoleV2Service creates a new RoleV2Service
func NewRoleV2Service(
	roleRepo role.RoleRepository,
	bindingRepo rolebinding.Repository,
	replicator Replicator,
	db *gorm.DB,
) *RoleV2Service {
	return &RoleV2Service{
		roleRepo:    roleRepo,
		bindingRepo: bindingRepo,
		replicator:  replicator,
		db:          db,
	}
}

// CreateRoleInput contains data for creating a role
type CreateRoleInput struct {
	Name        string
	Description *string
	Permissions []map[string]string // V2 format: [{application, resource_type, permission}]
	TenantID    uuid.UUID
}

// Create creates a new custom role with permissions
func (s *RoleV2Service) Create(ctx context.Context, input CreateRoleInput) (*role.RoleV2, error) {
	// Validate inputs
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if input.Description == nil || *input.Description == "" {
		return nil, fmt.Errorf("description is required")
	}
	if len(input.Permissions) == 0 {
		return nil, fmt.Errorf("permissions are required")
	}

	// Parse permissions from input
	permissions := make([]role.PermissionValue, 0, len(input.Permissions))
	for _, permData := range input.Permissions {
		app, appOk := permData["application"]
		resType, resOk := permData["resource_type"]
		verb, verbOk := permData["permission"]

		if !appOk || !resOk || !verbOk {
			return nil, fmt.Errorf("invalid permission format: missing application, resource_type, or permission")
		}

		permissions = append(permissions, role.PermissionValue{
			Application:  app,
			ResourceType: resType,
			Verb:         verb,
		})
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

	// Create role
	newRole := &role.RoleV2{
		UUID:        uuid.New(),
		Name:        input.Name,
		Description: input.Description,
		Type:        role.RoleTypeCustom,
		TenantID:    input.TenantID,
		Permissions: permissions,
	}

	// Create within transaction
	if err := tx.Create(newRole).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create role: %w", err)
	}

	// Generate replication tuples
	tuplesToAdd, tuplesToRemove, err := newRole.ReplicationTuples(nil, permissions)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to generate replication tuples: %w", err)
	}

	// Replicate BEFORE commit (Kessel validates permissions)
	if err := s.replicator.Replicate(ctx, &kessel.ReplicationEvent{
		EventType: "create_custom_role",
		Info: map[string]interface{}{
			"role_uuid": newRole.UUID.String(),
			"org_id":    input.TenantID.String(),
		},
		Add:    tuplesToAdd,
		Remove: tuplesToRemove,
	}); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("replication failed: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		// Kessel has orphaned tuples, but this is rare
		fmt.Printf("[RoleV2Service] ERROR: DB commit failed after replication: %v\n", err)
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return newRole, nil
}

// UpdateRoleInput contains data for updating a role
type UpdateRoleInput struct {
	UUID        uuid.UUID
	Name        string
	Description *string
	Permissions []map[string]string
	TenantID    uuid.UUID
}

// Update updates an existing custom role
func (s *RoleV2Service) Update(ctx context.Context, input UpdateRoleInput) (*role.RoleV2, error) {
	// Validate inputs
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if input.Description == nil || *input.Description == "" {
		return nil, fmt.Errorf("description is required")
	}
	if len(input.Permissions) == 0 {
		return nil, fmt.Errorf("permissions are required")
	}

	// Parse permissions from input
	newPermissions := make([]role.PermissionValue, 0, len(input.Permissions))
	for _, permData := range input.Permissions {
		app, appOk := permData["application"]
		resType, resOk := permData["resource_type"]
		verb, verbOk := permData["permission"]

		if !appOk || !resOk || !verbOk {
			return nil, fmt.Errorf("invalid permission format: missing application, resource_type, or permission")
		}

		newPermissions = append(newPermissions, role.PermissionValue{
			Application:  app,
			ResourceType: resType,
			Verb:         verb,
		})
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

	// Fetch existing role with permissions
	var existingRole role.RoleV2
	if err := tx.First(&existingRole, "uuid = ?", input.UUID).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("role not found: %w", err)
	}

	// Verify it's a custom role
	if existingRole.Type != role.RoleTypeCustom {
		tx.Rollback()
		return nil, fmt.Errorf("only custom roles can be updated")
	}

	// Verify tenant
	if existingRole.TenantID != input.TenantID {
		tx.Rollback()
		return nil, fmt.Errorf("role not found in tenant")
	}

	// Capture old permissions for replication
	oldPermissions := existingRole.Permissions

	// Update role
	existingRole.Update(input.Name, input.Description)
	existingRole.Permissions = newPermissions

	if err := tx.Save(&existingRole).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update role: %w", err)
	}

	// Generate replication tuples
	tuplesToAdd, tuplesToRemove, err := existingRole.ReplicationTuples(oldPermissions, newPermissions)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to generate replication tuples: %w", err)
	}

	// Replicate BEFORE commit (Kessel validates permissions)
	if err := s.replicator.Replicate(ctx, &kessel.ReplicationEvent{
		EventType: "update_custom_role",
		Info: map[string]interface{}{
			"role_uuid": existingRole.UUID.String(),
			"org_id":    input.TenantID.String(),
		},
		Add:    tuplesToAdd,
		Remove: tuplesToRemove,
	}); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("replication failed: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		// Kessel has orphaned tuples, but this is rare
		fmt.Printf("[RoleV2Service] ERROR: DB commit failed after replication: %v\n", err)
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &existingRole, nil
}

// Delete deletes a custom role by UUID
func (s *RoleV2Service) Delete(ctx context.Context, roleUUID uuid.UUID, tenantID uuid.UUID) error {
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

	// Fetch role with permissions
	var existingRole role.RoleV2
	if err := tx.First(&existingRole, "uuid = ?", roleUUID).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("role not found: %w", err)
	}

	// Verify it's a custom role
	if existingRole.Type != role.RoleTypeCustom {
		tx.Rollback()
		return fmt.Errorf("only custom roles can be deleted")
	}

	// Verify tenant
	if existingRole.TenantID != tenantID {
		tx.Rollback()
		return fmt.Errorf("role not found in tenant")
	}

	// Find all role bindings for this role
	var bindings []rolebinding.RoleBinding
	if err := tx.Where("role_id = ?", existingRole.ID).Find(&bindings).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to find role bindings: %w", err)
	}

	// Collect binding tuples for replication before deletion
	var bindingTuplesToRemove []*common.RelationTuple
	for _, binding := range bindings {
		// Get all tuples for the binding (role, resource, and subjects)
		tuples, err := binding.AllTuples()
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to generate binding tuples: %w", err)
		}
		bindingTuplesToRemove = append(bindingTuplesToRemove, tuples...)
	}

	// Delete all role bindings
	for i := range bindings {
		binding := &bindings[i]
		// Clear associations before deleting (many-to-many join tables)
		if err := tx.Model(binding).Association("Groups").Clear(); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to clear binding groups: %w", err)
		}
		if err := tx.Model(binding).Association("Principals").Clear(); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to clear binding principals: %w", err)
		}
		if err := tx.Delete(binding).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to delete role binding: %w", err)
		}
	}

	// Generate replication tuples for role permissions (all permissions removed)
	_, roleTuplesToRemove, err := existingRole.ReplicationTuples(existingRole.Permissions, nil)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to generate replication tuples: %w", err)
	}

	// Delete role
	if err := tx.Delete(&existingRole).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete role: %w", err)
	}

	// Replicate BEFORE commit - combine binding and role tuples
	allTuplesToRemove := append(bindingTuplesToRemove, roleTuplesToRemove...)
	if err := s.replicator.Replicate(ctx, &kessel.ReplicationEvent{
		EventType: "delete_custom_role",
		Info: map[string]interface{}{
			"role_uuid":      roleUUID.String(),
			"org_id":         tenantID.String(),
			"bindings_count": len(bindings),
		},
		Add:    nil,
		Remove: allTuplesToRemove,
	}); err != nil {
		tx.Rollback()
		return fmt.Errorf("replication failed: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		// Kessel has orphaned deletes, but this is rare
		fmt.Printf("[RoleV2Service] ERROR: DB commit failed after replication: %v\n", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// BatchDelete deletes multiple custom roles by UUID atomically
func (s *RoleV2Service) BatchDelete(ctx context.Context, roleUUIDs []uuid.UUID, tenantID uuid.UUID) error {
	if len(roleUUIDs) == 0 {
		return fmt.Errorf("at least one role UUID is required")
	}
	if len(roleUUIDs) > 100 {
		return fmt.Errorf("maximum 100 roles allowed per batch delete")
	}

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

	var allTuplesToRemove []*common.RelationTuple
	var notFoundIDs []uuid.UUID
	var nonCustomRoleIDs []uuid.UUID

	// Validate and process each role
	for _, roleUUID := range roleUUIDs {
		// Fetch role with permissions
		var existingRole role.RoleV2
		if err := tx.First(&existingRole, "uuid = ?", roleUUID).Error; err != nil {
			notFoundIDs = append(notFoundIDs, roleUUID)
			continue
		}

		// Verify it's a custom role
		if existingRole.Type != role.RoleTypeCustom {
			nonCustomRoleIDs = append(nonCustomRoleIDs, roleUUID)
			continue
		}

		// Verify tenant
		if existingRole.TenantID != tenantID {
			notFoundIDs = append(notFoundIDs, roleUUID)
			continue
		}

		// Find all role bindings for this role
		var bindings []rolebinding.RoleBinding
		if err := tx.Where("role_id = ?", existingRole.ID).Find(&bindings).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to find role bindings for role %s: %w", roleUUID, err)
		}

		// Collect binding tuples for replication before deletion
		for _, binding := range bindings {
			tuples, err := binding.AllTuples()
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to generate binding tuples: %w", err)
			}
			allTuplesToRemove = append(allTuplesToRemove, tuples...)
		}

		// Delete all role bindings
		for i := range bindings {
			binding := &bindings[i]
			// Clear associations before deleting (many-to-many join tables)
			if err := tx.Model(binding).Association("Groups").Clear(); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to clear binding groups: %w", err)
			}
			if err := tx.Model(binding).Association("Principals").Clear(); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to clear binding principals: %w", err)
			}
			if err := tx.Delete(binding).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to delete role binding: %w", err)
			}
		}

		// Generate replication tuples for role (all permissions removed)
		_, tuplesToRemove, err := existingRole.ReplicationTuples(existingRole.Permissions, nil)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to generate replication tuples: %w", err)
		}

		allTuplesToRemove = append(allTuplesToRemove, tuplesToRemove...)

		// Delete role
		if err := tx.Delete(&existingRole).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to delete role %s: %w", roleUUID, err)
		}
	}

	// Check for errors
	if len(notFoundIDs) > 0 {
		tx.Rollback()
		return fmt.Errorf("roles not found: %v", notFoundIDs)
	}
	if len(nonCustomRoleIDs) > 0 {
		tx.Rollback()
		return fmt.Errorf("cannot delete non-custom roles: %v", nonCustomRoleIDs)
	}

	// Replicate BEFORE commit
	if len(allTuplesToRemove) > 0 {
		if err := s.replicator.Replicate(ctx, &kessel.ReplicationEvent{
			EventType: "batch_delete_custom_roles",
			Info: map[string]interface{}{
				"count":  len(roleUUIDs),
				"org_id": tenantID.String(),
			},
			Add:    nil,
			Remove: allTuplesToRemove,
		}); err != nil {
			tx.Rollback()
			return fmt.Errorf("replication failed: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		// Kessel has orphaned deletes, but this is rare
		fmt.Printf("[RoleV2Service] ERROR: DB commit failed after replication: %v\n", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// List lists roles for a tenant
func (s *RoleV2Service) List(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]*role.RoleV2, error) {
	return s.roleRepo.ListForTenant(tenantID, offset, limit)
}

// Get retrieves a single role by UUID
func (s *RoleV2Service) Get(ctx context.Context, roleUUID uuid.UUID, tenantID uuid.UUID) (*role.RoleV2, error) {
	roleV2, err := s.roleRepo.FindByUUID(roleUUID)
	if err != nil {
		return nil, err
	}

	// Verify tenant access (allow access to roles in tenant or public tenant)
	// For now, simplified check
	if roleV2.TenantID != tenantID {
		// In real implementation, check if it's a seeded/platform role
		return nil, fmt.Errorf("role not found in tenant")
	}

	return roleV2, nil
}

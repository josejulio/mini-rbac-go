package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/redhat/mini-rbac-go/internal/domain/group"
	"github.com/redhat/mini-rbac-go/internal/infrastructure/kessel"
)

// GroupService handles business logic for Group operations
type GroupService struct {
	groupRepo     group.Repository
	principalRepo group.PrincipalRepository
	replicator    Replicator
	db            *gorm.DB
}

// NewGroupService creates a new GroupService
func NewGroupService(
	groupRepo group.Repository,
	principalRepo group.PrincipalRepository,
	replicator Replicator,
	db *gorm.DB,
) *GroupService {
	return &GroupService{
		groupRepo:     groupRepo,
		principalRepo: principalRepo,
		replicator:    replicator,
		db:            db,
	}
}

// CreateGroupInput contains data for creating a group
type CreateGroupInput struct {
	Name        string
	Description *string
	TenantID    uuid.UUID
}

// Create creates a new group
func (s *GroupService) Create(ctx context.Context, input CreateGroupInput) (*group.Group, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	newGroup := &group.Group{
		UUID:        uuid.New(), // Generate UUID explicitly
		Name:        input.Name,
		Description: input.Description,
		TenantID:    input.TenantID,
	}

	if err := newGroup.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	if err := s.groupRepo.Create(newGroup); err != nil {
		return nil, fmt.Errorf("failed to create group: %w", err)
	}

	// Note: No replication needed for empty group creation
	// Membership replication happens when principals are added

	return newGroup, nil
}

// UpdateGroupInput contains data for updating a group
type UpdateGroupInput struct {
	UUID        uuid.UUID
	Name        string
	Description *string
	TenantID    uuid.UUID
}

// Update updates an existing group
func (s *GroupService) Update(ctx context.Context, input UpdateGroupInput) (*group.Group, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	existingGroup, err := s.groupRepo.FindByUUID(input.UUID)
	if err != nil {
		return nil, fmt.Errorf("group not found: %w", err)
	}

	if existingGroup.TenantID != input.TenantID {
		return nil, fmt.Errorf("group not found in tenant")
	}

	existingGroup.Name = input.Name
	existingGroup.Description = input.Description

	if err := s.groupRepo.Update(existingGroup); err != nil {
		return nil, fmt.Errorf("failed to update group: %w", err)
	}

	return existingGroup, nil
}

// Delete deletes a group by UUID
func (s *GroupService) Delete(ctx context.Context, groupUUID uuid.UUID, tenantID uuid.UUID) error {
	existingGroup, err := s.groupRepo.FindByUUID(groupUUID)
	if err != nil {
		return fmt.Errorf("group not found: %w", err)
	}

	if existingGroup.TenantID != tenantID {
		return fmt.Errorf("group not found in tenant")
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

	// Get current principals for replication
	// Note: In real implementation, would preload via GORM
	oldPrincipals := existingGroup.Principals

	// Generate replication tuples (all memberships removed)
	_, tuplesToRemove, err := existingGroup.ReplicationTuples(oldPrincipals, nil)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to generate replication tuples: %w", err)
	}

	// Clear associations before deleting (many-to-many join table)
	if err := tx.Model(existingGroup).Association("Principals").Clear(); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to clear group principals: %w", err)
	}

	// Delete group
	if err := tx.Delete(existingGroup).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete group: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Replicate to Kessel
	if len(tuplesToRemove) > 0 {
		if err := s.replicator.Replicate(ctx, &kessel.ReplicationEvent{
			EventType: "delete_group",
			Info: map[string]interface{}{
				"group_uuid": groupUUID.String(),
				"org_id":     tenantID.String(),
			},
			Add:    nil,
			Remove: tuplesToRemove,
		}); err != nil {
			fmt.Printf("[GroupService] Warning: failed to replicate group deletion: %v\n", err)
		}
	}

	return nil
}

// List lists groups for a tenant
func (s *GroupService) List(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]*group.Group, error) {
	return s.groupRepo.ListForTenant(tenantID, offset, limit)
}

// Get retrieves a single group by UUID
func (s *GroupService) Get(ctx context.Context, groupUUID uuid.UUID, tenantID uuid.UUID) (*group.Group, error) {
	g, err := s.groupRepo.FindByUUID(groupUUID)
	if err != nil {
		return nil, err
	}

	if g.TenantID != tenantID {
		return nil, fmt.Errorf("group not found in tenant")
	}

	return g, nil
}

// AddPrincipalsInput contains data for adding principals to a group
type AddPrincipalsInput struct {
	GroupUUID uuid.UUID
	UserIDs   []string
	TenantID  uuid.UUID
}

// AddPrincipals adds principals to a group
func (s *GroupService) AddPrincipals(ctx context.Context, input AddPrincipalsInput) error {
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

	// Fetch group
	var g group.Group
	if err := tx.Preload("Principals").First(&g, "uuid = ?", input.GroupUUID).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("group not found: %w", err)
	}

	if g.TenantID != input.TenantID {
		tx.Rollback()
		return fmt.Errorf("group not found in tenant")
	}

	// Fetch or create principals
	var newPrincipals []*group.Principal
	for _, userID := range input.UserIDs {
		var p group.Principal
		err := tx.Where("user_id = ?", userID).First(&p).Error
		if err != nil {
			// Create principal if not exists
			p = group.Principal{
				UserID:   userID,
				Type:     group.PrincipalTypeUser,
				TenantID: input.TenantID,
			}
			if err := tx.Create(&p).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to create principal: %w", err)
			}
		}
		newPrincipals = append(newPrincipals, &p)
	}

	// Get existing principals
	oldPrincipals := g.Principals

	// Update group principals (append new ones) using Association for many-to-many
	if err := tx.Model(&g).Association("Principals").Append(newPrincipals); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to add principals to group: %w", err)
	}

	// Build new principals list for replication delta computation
	// Note: Don't append to g.Principals directly as Association may have already updated it
	allPrincipals := append([]*group.Principal{}, oldPrincipals...)
	allPrincipals = append(allPrincipals, newPrincipals...)

	// Generate replication tuples (only new principals)
	tuplesToAdd, _, err := g.ReplicationTuples(oldPrincipals, allPrincipals)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to generate replication tuples: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Replicate to Kessel
	if len(tuplesToAdd) > 0 {
		if err := s.replicator.Replicate(ctx, &kessel.ReplicationEvent{
			EventType: "add_principals_to_group",
			Info: map[string]interface{}{
				"group_uuid": input.GroupUUID.String(),
				"org_id":     input.TenantID.String(),
			},
			Add:    tuplesToAdd,
			Remove: nil,
		}); err != nil {
			fmt.Printf("[GroupService] Warning: failed to replicate principal addition: %v\n", err)
		}
	}

	return nil
}

// RemovePrincipalsInput contains data for removing principals from a group
type RemovePrincipalsInput struct {
	GroupUUID uuid.UUID
	UserIDs   []string
	TenantID  uuid.UUID
}

// RemovePrincipals removes principals from a group
func (s *GroupService) RemovePrincipals(ctx context.Context, input RemovePrincipalsInput) error {
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

	// Fetch group
	var g group.Group
	if err := tx.Preload("Principals").First(&g, "uuid = ?", input.GroupUUID).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("group not found: %w", err)
	}

	if g.TenantID != input.TenantID {
		tx.Rollback()
		return fmt.Errorf("group not found in tenant")
	}

	// Get existing principals
	oldPrincipals := g.Principals

	// Build set of user IDs to remove
	toRemove := make(map[string]bool)
	for _, userID := range input.UserIDs {
		toRemove[userID] = true
	}

	// Filter out principals to remove
	var remainingPrincipals []*group.Principal
	for _, p := range g.Principals {
		if !toRemove[p.UserID] {
			remainingPrincipals = append(remainingPrincipals, p)
		}
	}

	// Update group principals using Association for many-to-many
	if err := tx.Model(&g).Association("Principals").Replace(remainingPrincipals); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update group principals: %w", err)
	}
	g.Principals = remainingPrincipals

	// Generate replication tuples
	_, tuplesToRemove, err := g.ReplicationTuples(oldPrincipals, g.Principals)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to generate replication tuples: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Replicate to Kessel
	if len(tuplesToRemove) > 0 {
		if err := s.replicator.Replicate(ctx, &kessel.ReplicationEvent{
			EventType: "remove_principals_from_group",
			Info: map[string]interface{}{
				"group_uuid": input.GroupUUID.String(),
				"org_id":     input.TenantID.String(),
			},
			Add:    nil,
			Remove: tuplesToRemove,
		}); err != nil {
			fmt.Printf("[GroupService] Warning: failed to replicate principal removal: %v\n", err)
		}
	}

	return nil
}

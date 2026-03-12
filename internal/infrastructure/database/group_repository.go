package database

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/redhat/mini-rbac-go/internal/domain/group"
)

// GroupRepository implements the group.Repository interface
type GroupRepository struct {
	db *gorm.DB
}

// NewGroupRepository creates a new group repository
func NewGroupRepository(db *gorm.DB) group.Repository {
	return &GroupRepository{db: db}
}

// Create creates a new group
func (r *GroupRepository) Create(g *group.Group) error {
	return r.db.Create(g).Error
}

// Update updates an existing group
func (r *GroupRepository) Update(g *group.Group) error {
	return r.db.Save(g).Error
}

// Delete deletes a group by UUID
func (r *GroupRepository) Delete(id uuid.UUID) error {
	// First, find the group
	var g group.Group
	if err := r.db.Where("uuid = ?", id).First(&g).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("group not found: %s", id)
		}
		return err
	}

	// Clear all associations (many-to-many relationships)
	if err := r.db.Model(&g).Association("Principals").Clear(); err != nil {
		return fmt.Errorf("failed to clear principal associations: %w", err)
	}

	// Now delete the group
	if err := r.db.Delete(&g).Error; err != nil {
		return fmt.Errorf("failed to delete group: %w", err)
	}

	return nil
}

// FindByUUID finds a group by UUID with principals preloaded
func (r *GroupRepository) FindByUUID(id uuid.UUID) (*group.Group, error) {
	var g group.Group
	err := r.db.Preload("Principals").Where("uuid = ?", id).First(&g).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("group not found: %s", id)
		}
		return nil, err
	}
	return &g, nil
}

// FindByUUIDs finds multiple groups by UUIDs
func (r *GroupRepository) FindByUUIDs(ids []uuid.UUID) ([]*group.Group, error) {
	var groups []*group.Group
	err := r.db.Preload("Principals").Where("uuid IN ?", ids).Find(&groups).Error
	return groups, err
}

// ListForTenant lists groups for a tenant with pagination
func (r *GroupRepository) ListForTenant(tenantID uuid.UUID, offset, limit int) ([]*group.Group, error) {
	var groups []*group.Group
	err := r.db.Preload("Principals").
		Where("tenant_id = ?", tenantID).
		Offset(offset).
		Limit(limit).
		Order("name ASC, modified DESC").
		Find(&groups).Error
	return groups, err
}

// FindPlatformDefault finds the platform default group
func (r *GroupRepository) FindPlatformDefault() (*group.Group, error) {
	var g group.Group
	err := r.db.Preload("Principals").
		Where("platform_default = ?", true).
		First(&g).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("platform default group not found")
		}
		return nil, err
	}
	return &g, nil
}

// FindAdminDefault finds the admin default group
func (r *GroupRepository) FindAdminDefault() (*group.Group, error) {
	var g group.Group
	err := r.db.Preload("Principals").
		Where("admin_default = ?", true).
		First(&g).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("admin default group not found")
		}
		return nil, err
	}
	return &g, nil
}

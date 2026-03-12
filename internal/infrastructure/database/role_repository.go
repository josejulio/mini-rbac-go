package database

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/redhat/mini-rbac-go/internal/domain/role"
)

// RoleRepository implements the role.Repository interface
type RoleRepository struct {
	db *gorm.DB
}

// NewRoleRepository creates a new role repository
func NewRoleRepository(db *gorm.DB) role.RoleRepository {
	return &RoleRepository{db: db}
}

// Create creates a new role
func (r *RoleRepository) Create(roleV2 *role.RoleV2) error {
	return r.db.Create(roleV2).Error
}

// Update updates an existing role
func (r *RoleRepository) Update(roleV2 *role.RoleV2) error {
	return r.db.Save(roleV2).Error
}

// Delete deletes a role by UUID
func (r *RoleRepository) Delete(id uuid.UUID) error {
	result := r.db.Where("uuid = ?", id).Delete(&role.RoleV2{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete role: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("role not found: %s", id)
	}
	return nil
}

// FindByUUID finds a role by UUID
func (r *RoleRepository) FindByUUID(id uuid.UUID) (*role.RoleV2, error) {
	var roleV2 role.RoleV2
	err := r.db.Where("uuid = ?", id).First(&roleV2).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("role not found: %s", id)
		}
		return nil, err
	}
	return &roleV2, nil
}

// FindByUUIDs finds multiple roles by UUIDs
func (r *RoleRepository) FindByUUIDs(ids []uuid.UUID) ([]*role.RoleV2, error) {
	var roles []*role.RoleV2
	err := r.db.Where("uuid IN ?", ids).Find(&roles).Error
	return roles, err
}

// ListForTenant lists roles for a tenant with pagination
func (r *RoleRepository) ListForTenant(tenantID uuid.UUID, offset, limit int) ([]*role.RoleV2, error) {
	var roles []*role.RoleV2
	err := r.db.Where("tenant_id = ?", tenantID).
		Offset(offset).
		Limit(limit).
		Order("name ASC, modified DESC").
		Find(&roles).Error
	return roles, err
}

// FilterByName filters roles by exact name match
func (r *RoleRepository) FilterByName(tenantID uuid.UUID, name string) ([]*role.RoleV2, error) {
	var roles []*role.RoleV2
	err := r.db.Where("tenant_id = ? AND name = ?", tenantID, name).
		Find(&roles).Error
	return roles, err
}

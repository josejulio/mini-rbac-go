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
	// First, find the role
	var roleV2 role.RoleV2
	if err := r.db.Where("uuid = ?", id).First(&roleV2).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("role not found: %s", id)
		}
		return err
	}

	// Clear all associations (many-to-many relationships)
	if err := r.db.Model(&roleV2).Association("Permissions").Clear(); err != nil {
		return fmt.Errorf("failed to clear permission associations: %w", err)
	}

	// Now delete the role
	if err := r.db.Delete(&roleV2).Error; err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}

	return nil
}

// FindByUUID finds a role by UUID with permissions preloaded
func (r *RoleRepository) FindByUUID(id uuid.UUID) (*role.RoleV2, error) {
	var roleV2 role.RoleV2
	err := r.db.Preload("Permissions").Where("uuid = ?", id).First(&roleV2).Error
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
	err := r.db.Preload("Permissions").Where("uuid IN ?", ids).Find(&roles).Error
	return roles, err
}

// ListForTenant lists roles for a tenant with pagination
func (r *RoleRepository) ListForTenant(tenantID uuid.UUID, offset, limit int) ([]*role.RoleV2, error) {
	var roles []*role.RoleV2
	err := r.db.Preload("Permissions").
		Where("tenant_id = ?", tenantID).
		Offset(offset).
		Limit(limit).
		Order("name ASC, modified DESC").
		Find(&roles).Error
	return roles, err
}

// FilterByName filters roles by exact name match
func (r *RoleRepository) FilterByName(tenantID uuid.UUID, name string) ([]*role.RoleV2, error) {
	var roles []*role.RoleV2
	err := r.db.Preload("Permissions").
		Where("tenant_id = ? AND name = ?", tenantID, name).
		Find(&roles).Error
	return roles, err
}

// PermissionRepository implements the role.PermissionRepository interface
type PermissionRepository struct {
	db *gorm.DB
}

// NewPermissionRepository creates a new permission repository
func NewPermissionRepository(db *gorm.DB) role.PermissionRepository {
	return &PermissionRepository{db: db}
}

// Create creates a new permission
func (r *PermissionRepository) Create(permission *role.Permission) error {
	return r.db.Create(permission).Error
}

// FindByV1String finds a permission by v1 string format
func (r *PermissionRepository) FindByV1String(v1String string) (*role.Permission, error) {
	pv, err := role.ParseV1Permission(v1String)
	if err != nil {
		return nil, err
	}

	var perm role.Permission
	err = r.db.Where("application = ? AND resource_type = ? AND verb = ?",
		pv.Application, pv.ResourceType, pv.Verb).First(&perm).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("permission not found: %s", v1String)
		}
		return nil, err
	}
	return &perm, nil
}

// FindByV2String finds a permission by v2 string format
func (r *PermissionRepository) FindByV2String(v2String string) (*role.Permission, error) {
	pv, err := role.ParseV2Permission(v2String)
	if err != nil {
		return nil, err
	}

	var perm role.Permission
	err = r.db.Where("application = ? AND resource_type = ? AND verb = ?",
		pv.Application, pv.ResourceType, pv.Verb).First(&perm).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("permission not found: %s", v2String)
		}
		return nil, err
	}
	return &perm, nil
}

// ResolveFromV2Data resolves permissions from v2 API data format
func (r *PermissionRepository) ResolveFromV2Data(v2Data []map[string]string) ([]*role.Permission, error) {
	var permissions []*role.Permission

	for _, data := range v2Data {
		app, ok := data["application"]
		if !ok {
			return nil, fmt.Errorf("missing application field in permission data")
		}
		resource, ok := data["resource_type"]
		if !ok {
			return nil, fmt.Errorf("missing resource_type field in permission data")
		}
		verb, ok := data["permission"]
		if !ok {
			return nil, fmt.Errorf("missing permission field in permission data")
		}

		var perm role.Permission
		err := r.db.Where("application = ? AND resource_type = ? AND verb = ?",
			app, resource, verb).First(&perm).Error

		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, fmt.Errorf("permission not found: %s:%s:%s", app, resource, verb)
			}
			return nil, err
		}

		permissions = append(permissions, &perm)
	}

	return permissions, nil
}

// List lists permissions with pagination
func (r *PermissionRepository) List(offset, limit int) ([]*role.Permission, error) {
	var permissions []*role.Permission
	err := r.db.Offset(offset).Limit(limit).Find(&permissions).Error
	return permissions, err
}

package database

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/redhat/mini-rbac-go/internal/domain/rolebinding"
)

// RoleBindingRepository implements the rolebinding.Repository interface
type RoleBindingRepository struct {
	db *gorm.DB
}

// NewRoleBindingRepository creates a new role binding repository
func NewRoleBindingRepository(db *gorm.DB) rolebinding.Repository {
	return &RoleBindingRepository{db: db}
}

// Create creates a new role binding
func (r *RoleBindingRepository) Create(binding *rolebinding.RoleBinding) error {
	return r.db.Create(binding).Error
}

// Update updates an existing role binding
func (r *RoleBindingRepository) Update(binding *rolebinding.RoleBinding) error {
	return r.db.Save(binding).Error
}

// Delete deletes a role binding by UUID
func (r *RoleBindingRepository) Delete(id uuid.UUID) error {
	// First, find the role binding
	var binding rolebinding.RoleBinding
	if err := r.db.Where("uuid = ?", id).First(&binding).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("role binding not found: %s", id)
		}
		return err
	}

	// Clear all associations (many-to-many relationships)
	if err := r.db.Model(&binding).Association("Groups").Clear(); err != nil {
		return fmt.Errorf("failed to clear group associations: %w", err)
	}
	if err := r.db.Model(&binding).Association("Principals").Clear(); err != nil {
		return fmt.Errorf("failed to clear principal associations: %w", err)
	}

	// Now delete the role binding
	if err := r.db.Delete(&binding).Error; err != nil {
		return fmt.Errorf("failed to delete role binding: %w", err)
	}

	return nil
}

// FindByUUID finds a role binding by UUID with associations preloaded
func (r *RoleBindingRepository) FindByUUID(id uuid.UUID) (*rolebinding.RoleBinding, error) {
	var binding rolebinding.RoleBinding
	err := r.db.Preload("Role").
		Preload("Groups").
		Preload("Principals").
		Where("uuid = ?", id).
		First(&binding).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("role binding not found: %s", id)
		}
		return nil, err
	}
	return &binding, nil
}

// FindByUUIDs finds multiple role bindings by UUIDs
func (r *RoleBindingRepository) FindByUUIDs(ids []uuid.UUID) ([]*rolebinding.RoleBinding, error) {
	var bindings []*rolebinding.RoleBinding
	err := r.db.Preload("Role").
		Preload("Groups").
		Preload("Principals").
		Where("uuid IN ?", ids).
		Find(&bindings).Error
	return bindings, err
}

// FindByRole finds all role bindings for a specific role
func (r *RoleBindingRepository) FindByRole(roleID uint) ([]*rolebinding.RoleBinding, error) {
	var bindings []*rolebinding.RoleBinding
	err := r.db.Preload("Role").
		Preload("Groups").
		Preload("Principals").
		Where("role_id = ?", roleID).
		Find(&bindings).Error
	return bindings, err
}

// FindForResource finds all role bindings for a specific resource
func (r *RoleBindingRepository) FindForResource(resourceType, resourceID string, tenantID uuid.UUID) ([]*rolebinding.RoleBinding, error) {
	var bindings []*rolebinding.RoleBinding
	err := r.db.Preload("Role").
		Preload("Groups").
		Preload("Principals").
		Where("resource_type = ? AND resource_id = ? AND tenant_id = ?", resourceType, resourceID, tenantID).
		Find(&bindings).Error
	return bindings, err
}

// FindForResourceAndRole finds a specific role binding for resource and role
func (r *RoleBindingRepository) FindForResourceAndRole(resourceType, resourceID string, roleID uint, tenantID uuid.UUID) (*rolebinding.RoleBinding, error) {
	var binding rolebinding.RoleBinding
	err := r.db.Preload("Role").
		Preload("Groups").
		Preload("Principals").
		Where("resource_type = ? AND resource_id = ? AND role_id = ? AND tenant_id = ?",
			resourceType, resourceID, roleID, tenantID).
		First(&binding).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("role binding not found for resource %s:%s and role %d", resourceType, resourceID, roleID)
		}
		return nil, err
	}
	return &binding, nil
}

// FindOrphaned finds role bindings where the role has been deleted
func (r *RoleBindingRepository) FindOrphaned() ([]*rolebinding.RoleBinding, error) {
	var bindings []*rolebinding.RoleBinding
	err := r.db.Preload("Groups").
		Preload("Principals").
		Joins("LEFT JOIN roles ON roles.id = role_bindings.role_id").
		Where("roles.id IS NULL").
		Find(&bindings).Error
	return bindings, err
}

// ListForTenant lists role bindings for a tenant with pagination
func (r *RoleBindingRepository) ListForTenant(tenantID uuid.UUID, offset, limit int) ([]*rolebinding.RoleBinding, error) {
	var bindings []*rolebinding.RoleBinding
	err := r.db.Preload("Role").
		Preload("Groups").
		Preload("Principals").
		Where("tenant_id = ?", tenantID).
		Offset(offset).
		Limit(limit).
		Order("modified DESC").
		Find(&bindings).Error
	return bindings, err
}

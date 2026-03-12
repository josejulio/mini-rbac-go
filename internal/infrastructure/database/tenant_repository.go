package database

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/redhat/mini-rbac-go/internal/domain/tenant"
)

// TenantRepository implements the tenant.Repository interface
type TenantRepository struct {
	db *gorm.DB
}

// NewTenantRepository creates a new tenant repository
func NewTenantRepository(db *gorm.DB) tenant.Repository {
	return &TenantRepository{db: db}
}

// Create creates a new tenant
func (r *TenantRepository) Create(t *tenant.Tenant) error {
	return r.db.Create(t).Error
}

// FindByOrgID finds a tenant by org ID
func (r *TenantRepository) FindByOrgID(orgID string) (*tenant.Tenant, error) {
	var t tenant.Tenant
	err := r.db.Where("org_id = ?", orgID).First(&t).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("tenant not found: %s", orgID)
		}
		return nil, err
	}
	return &t, nil
}

// FindByID finds a tenant by ID
func (r *TenantRepository) FindByID(id uint) (*tenant.Tenant, error) {
	var t tenant.Tenant
	err := r.db.First(&t, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("tenant not found: %d", id)
		}
		return nil, err
	}
	return &t, nil
}

// List lists tenants with pagination
func (r *TenantRepository) List(offset, limit int) ([]*tenant.Tenant, error) {
	var tenants []*tenant.Tenant
	err := r.db.Offset(offset).Limit(limit).Find(&tenants).Error
	return tenants, err
}

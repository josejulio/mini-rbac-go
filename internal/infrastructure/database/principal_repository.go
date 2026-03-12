package database

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/redhat/mini-rbac-go/internal/domain/group"
)

// PrincipalRepository implements the group.PrincipalRepository interface
type PrincipalRepository struct {
	db *gorm.DB
}

// NewPrincipalRepository creates a new principal repository
func NewPrincipalRepository(db *gorm.DB) group.PrincipalRepository {
	return &PrincipalRepository{db: db}
}

// Create creates a new principal
func (r *PrincipalRepository) Create(principal *group.Principal) error {
	return r.db.Create(principal).Error
}

// FindByUserID finds a principal by user ID
func (r *PrincipalRepository) FindByUserID(userID string) (*group.Principal, error) {
	var principal group.Principal
	err := r.db.Where("user_id = ?", userID).First(&principal).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("principal not found: %s", userID)
		}
		return nil, err
	}
	return &principal, nil
}

// FindByUserIDs finds multiple principals by user IDs
func (r *PrincipalRepository) FindByUserIDs(userIDs []string) ([]*group.Principal, error) {
	var principals []*group.Principal
	err := r.db.Where("user_id IN ?", userIDs).Find(&principals).Error
	return principals, err
}

// ListForTenant lists principals for a tenant with pagination
func (r *PrincipalRepository) ListForTenant(tenantID uuid.UUID, offset, limit int) ([]*group.Principal, error) {
	var principals []*group.Principal
	err := r.db.Where("tenant_id = ?", tenantID).
		Offset(offset).
		Limit(limit).
		Order("user_id ASC").
		Find(&principals).Error
	return principals, err
}

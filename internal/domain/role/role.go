package role

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redhat/mini-rbac-go/internal/domain/common"
)

// RoleType represents the type of a role
type RoleType string

const (
	RoleTypeCustom   RoleType = "custom"
	RoleTypeSeeded   RoleType = "seeded"
	RoleTypePlatform RoleType = "platform"
)

// RoleV2 represents a V2 role in the system
// Mirrors the Python RoleV2 model
type RoleV2 struct {
	ID          uint         `gorm:"primarykey"`
	UUID        uuid.UUID    `gorm:"type:uuid;uniqueIndex;not null"`
	Name        string       `gorm:"size:175;not null;index:idx_role_name_tenant,unique"`
	Description *string      `gorm:"type:text"`
	Type        RoleType     `gorm:"size:20;not null;index"`
	TenantID    uuid.UUID    `gorm:"type:uuid;not null;index:idx_role_name_tenant,unique"`
	Permissions []Permission `gorm:"many2many:role_permissions;"`
	Created     time.Time    `gorm:"autoCreateTime"`
	Modified    time.Time    `gorm:"autoUpdateTime"`
}

// TableName specifies the table name for GORM
func (RoleV2) TableName() string {
	return "roles_v2"
}

// BeforeCreate generates a UUID if not set
func (r *RoleV2) BeforeCreate() error {
	if r.UUID == uuid.Nil {
		r.UUID = uuid.New()
	}
	return nil
}

// Validate performs domain validation
func (r *RoleV2) Validate() error {
	if r.Name == "" || strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

// PermissionTuple generates a single permission tuple for custom roles
// Format: rbac/role:<uuid>#<permission>@rbac/principal:*
func (r *RoleV2) PermissionTuple(permission *Permission) (*common.RelationTuple, error) {
	if r.Type != RoleTypeCustom {
		return nil, fmt.Errorf("permission tuples only supported for custom roles")
	}

	roleType, err := common.NewObjectType("rbac", "role")
	if err != nil {
		return nil, err
	}

	resource, err := common.NewObjectReference(*roleType, r.UUID.String())
	if err != nil {
		return nil, err
	}

	principalType, err := common.NewObjectType("rbac", "principal")
	if err != nil {
		return nil, err
	}

	principalRef, err := common.NewObjectReference(*principalType, "*")
	if err != nil {
		return nil, err
	}

	subject := common.NewSubjectReference(*principalRef, nil)

	return common.NewRelationTuple(*resource, permission.V2String(), *subject)
}

// ReplicationTuples computes the delta (tuples to add vs. remove) for a role mutation
// Mirrors the Python CustomRoleV2.replication_tuples static method
func (r *RoleV2) ReplicationTuples(oldPermissions, newPermissions []*Permission) (toAdd, toRemove []*common.RelationTuple, err error) {
	if r.Type != RoleTypeCustom {
		return nil, nil, fmt.Errorf("replication only supported for custom roles")
	}

	// Convert to maps for set operations
	oldSet := make(map[uint]bool)
	for _, p := range oldPermissions {
		oldSet[p.ID] = true
	}

	newSet := make(map[uint]bool)
	for _, p := range newPermissions {
		newSet[p.ID] = true
	}

	// Permissions to add (in new but not in old)
	for _, p := range newPermissions {
		if !oldSet[p.ID] {
			tuple, err := r.PermissionTuple(p)
			if err != nil {
				return nil, nil, err
			}
			toAdd = append(toAdd, tuple)
		}
	}

	// Permissions to remove (in old but not in new)
	for _, p := range oldPermissions {
		if !newSet[p.ID] {
			tuple, err := r.PermissionTuple(p)
			if err != nil {
				return nil, nil, err
			}
			toRemove = append(toRemove, tuple)
		}
	}

	return toAdd, toRemove, nil
}

// Update updates mutable attributes of the role
func (r *RoleV2) Update(name string, description *string) {
	r.Name = name
	r.Description = description
}

// Repository defines the interface for role persistence
type RoleRepository interface {
	Create(role *RoleV2) error
	Update(role *RoleV2) error
	Delete(id uuid.UUID) error
	FindByUUID(id uuid.UUID) (*RoleV2, error)
	FindByUUIDs(ids []uuid.UUID) ([]*RoleV2, error)
	ListForTenant(tenantID uuid.UUID, offset, limit int) ([]*RoleV2, error)
	FilterByName(tenantID uuid.UUID, name string) ([]*RoleV2, error)
}

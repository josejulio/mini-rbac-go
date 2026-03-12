package tenant

import (
	"time"

	"github.com/google/uuid"
)

// Tenant represents an organization/account in the system
// Simplified version of the Python Tenant model
type Tenant struct {
	ID       uint      `gorm:"primarykey"`
	OrgID    string    `gorm:"uniqueIndex;size:255;not null"`
	Created  time.Time `gorm:"autoCreateTime"`
	Modified time.Time `gorm:"autoUpdateTime"`
}

// TableName specifies the table name for GORM
func (Tenant) TableName() string {
	return "tenants"
}

// TenantResourceID returns the resource ID for use in relation tuples
// Format: org:<org_id>
func (t *Tenant) TenantResourceID() string {
	return "org:" + t.OrgID
}

// Repository defines the interface for tenant persistence
type Repository interface {
	Create(tenant *Tenant) error
	FindByOrgID(orgID string) (*Tenant, error)
	FindByID(id uint) (*Tenant, error)
	List(offset, limit int) ([]*Tenant, error)
}

// TenantMapping stores deterministic UUIDs for tenant resources
// Mirrors the Python TenantMapping model
type TenantMapping struct {
	ID                      uint      `gorm:"primarykey"`
	TenantID                uint      `gorm:"uniqueIndex;not null"`
	Tenant                  *Tenant   `gorm:"foreignKey:TenantID"`
	DefaultAdminTenantUUID  uuid.UUID `gorm:"type:uuid;not null"`
	DefaultAdminRootUUID    uuid.UUID `gorm:"type:uuid;not null"`
	DefaultAdminDefaultUUID uuid.UUID `gorm:"type:uuid;not null"`
	DefaultUserTenantUUID   uuid.UUID `gorm:"type:uuid;not null"`
	DefaultUserRootUUID     uuid.UUID `gorm:"type:uuid;not null"`
	DefaultUserDefaultUUID  uuid.UUID `gorm:"type:uuid;not null"`
	Created                 time.Time `gorm:"autoCreateTime"`
}

// TableName specifies the table name for GORM
func (TenantMapping) TableName() string {
	return "tenant_mappings"
}

// DefaultAccessType represents the type of default access
type DefaultAccessType string

const (
	DefaultAccessTypeUser  DefaultAccessType = "user"
	DefaultAccessTypeAdmin DefaultAccessType = "admin"
)

// Scope represents the scope level for default bindings
type Scope string

const (
	ScopeTenant  Scope = "tenant"
	ScopeRoot    Scope = "root"
	ScopeDefault Scope = "default"
)

// DefaultRoleBindingUUIDFor returns the UUID for a default role binding
func (m *TenantMapping) DefaultRoleBindingUUIDFor(accessType DefaultAccessType, scope Scope) uuid.UUID {
	switch accessType {
	case DefaultAccessTypeAdmin:
		switch scope {
		case ScopeTenant:
			return m.DefaultAdminTenantUUID
		case ScopeRoot:
			return m.DefaultAdminRootUUID
		case ScopeDefault:
			return m.DefaultAdminDefaultUUID
		}
	case DefaultAccessTypeUser:
		switch scope {
		case ScopeTenant:
			return m.DefaultUserTenantUUID
		case ScopeRoot:
			return m.DefaultUserRootUUID
		case ScopeDefault:
			return m.DefaultUserDefaultUUID
		}
	}
	return uuid.Nil
}

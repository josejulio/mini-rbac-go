package role

import (
	"fmt"
	"strings"
)

// Permission represents a permission in the system
// Mirrors the Python Permission model
type Permission struct {
	ID           uint   `gorm:"primarykey"`
	Application  string `gorm:"size:255;not null;index:idx_permission_unique,unique"`
	ResourceType string `gorm:"size:255;not null;index:idx_permission_unique,unique"`
	Verb         string `gorm:"size:255;not null;index:idx_permission_unique,unique"`
}

// TableName specifies the table name for GORM
func (Permission) TableName() string {
	return "permissions"
}

// String returns the v1 permission string format
// Format: application:resource_type:verb
func (p *Permission) String() string {
	return fmt.Sprintf("%s:%s:%s", p.Application, p.ResourceType, p.Verb)
}

// V2String returns the v2 permission string format
// Format: application_resource_type_verb
func (p *Permission) V2String() string {
	return fmt.Sprintf("%s_%s_%s", p.Application, p.ResourceType, p.Verb)
}

// PermissionValue represents a parsed permission value
// Used for validation and conversion between v1 and v2 formats
type PermissionValue struct {
	Application  string
	ResourceType string
	Verb         string
}

// ParseV1Permission parses a v1 permission string (app:resource:verb)
func ParseV1Permission(s string) (*PermissionValue, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid v1 permission format: %s (expected app:resource:verb)", s)
	}
	return &PermissionValue{
		Application:  parts[0],
		ResourceType: parts[1],
		Verb:         parts[2],
	}, nil
}

// ParseV2Permission parses a v2 permission string (app_resource_verb)
func ParseV2Permission(s string) (*PermissionValue, error) {
	parts := strings.Split(s, "_")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid v2 permission format: %s (expected app_resource_verb)", s)
	}
	// Handle cases where resource type might have underscores
	// Take first part as app, last as verb, middle as resource
	return &PermissionValue{
		Application:  parts[0],
		ResourceType: strings.Join(parts[1:len(parts)-1], "_"),
		Verb:         parts[len(parts)-1],
	}, nil
}

// V1String returns the v1 format
func (pv *PermissionValue) V1String() string {
	return fmt.Sprintf("%s:%s:%s", pv.Application, pv.ResourceType, pv.Verb)
}

// V2String returns the v2 format
func (pv *PermissionValue) V2String() string {
	return fmt.Sprintf("%s_%s_%s", pv.Application, pv.ResourceType, pv.Verb)
}

// Repository defines the interface for permission persistence
type PermissionRepository interface {
	Create(permission *Permission) error
	FindByV1String(v1String string) (*Permission, error)
	FindByV2String(v2String string) (*Permission, error)
	ResolveFromV2Data(v2Data []map[string]string) ([]*Permission, error)
	List(offset, limit int) ([]*Permission, error)
}

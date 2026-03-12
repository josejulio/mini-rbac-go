package role

import (
	"fmt"
	"strings"
)

// PermissionValue represents a permission (no database storage)
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

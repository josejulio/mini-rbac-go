package group

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PrincipalType represents the type of a principal
type PrincipalType string

const (
	PrincipalTypeUser         PrincipalType = "user"
	PrincipalTypeServiceAccount PrincipalType = "service-account"
)

// Principal represents a user or service account
// Mirrors the Python Principal model (simplified)
type Principal struct {
	ID       uint          `gorm:"primarykey"`
	UserID   string        `gorm:"uniqueIndex;size:255;not null"`
	Type     PrincipalType `gorm:"size:50;not null;index"`
	TenantID uuid.UUID     `gorm:"type:uuid;not null;index"`
	Created  time.Time     `gorm:"autoCreateTime"`
	Modified time.Time     `gorm:"autoUpdateTime"`
}

// TableName specifies the table name for GORM
func (Principal) TableName() string {
	return "principals"
}

// Validate performs domain validation
func (p *Principal) Validate() error {
	if p.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if p.Type == "" {
		return fmt.Errorf("type is required")
	}
	return nil
}

// ToPrincipalResourceID converts user_id to the resource ID format for relations
// Mirrors Python: Principal.user_id_to_principal_resource_id
func (p *Principal) ToPrincipalResourceID() string {
	return UserIDToPrincipalResourceID(p.UserID)
}

// UserIDToPrincipalResourceID converts a user ID to principal resource ID
// Replaces '@' with '|' to avoid issues with SpiceDB syntax
func UserIDToPrincipalResourceID(userID string) string {
	return strings.ReplaceAll(userID, "@", "|")
}

// PrincipalResourceIDToUserID converts principal resource ID back to user ID
func PrincipalResourceIDToUserID(resourceID string) string {
	return strings.ReplaceAll(resourceID, "|", "@")
}

// PrincipalRepository defines the interface for principal persistence
type PrincipalRepository interface {
	Create(principal *Principal) error
	FindByUserID(userID string) (*Principal, error)
	FindByUserIDs(userIDs []string) ([]*Principal, error)
	ListForTenant(tenantID uuid.UUID, offset, limit int) ([]*Principal, error)
}

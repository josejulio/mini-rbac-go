package group

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redhat/mini-rbac-go/internal/domain/common"
)

// Group represents a group of principals
// Mirrors the Python Group model
type Group struct {
	ID              uint         `gorm:"primarykey"`
	UUID            uuid.UUID    `gorm:"type:uuid;uniqueIndex;not null"`
	Name            string       `gorm:"size:255;not null"`
	Description     *string      `gorm:"type:text"`
	TenantID        uuid.UUID    `gorm:"type:uuid;not null;index"`
	PlatformDefault bool         `gorm:"default:false;not null"`
	AdminDefault    bool         `gorm:"default:false;not null"`
	Principals      []*Principal `gorm:"many2many:group_principals;"`
	Created         time.Time    `gorm:"autoCreateTime"`
	Modified        time.Time    `gorm:"autoUpdateTime"`
}

// TableName specifies the table name for GORM
func (Group) TableName() string {
	return "groups"
}

// BeforeCreate generates a UUID if not set
func (g *Group) BeforeCreate() error {
	if g.UUID == uuid.Nil {
		g.UUID = uuid.New()
	}
	return nil
}

// Validate performs domain validation
func (g *Group) Validate() error {
	if g.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

// MemberRelation returns "member" - the relation used for group membership
func (g *Group) MemberRelation() string {
	return "member"
}

// GroupMemberTuple generates a membership tuple for a principal
// Format: rbac/group:<group_uuid>#member@rbac/principal:<principal_id>
func (g *Group) GroupMemberTuple(principal *Principal) (*common.RelationTuple, error) {
	groupType, err := common.NewObjectType("rbac", "group")
	if err != nil {
		return nil, err
	}

	resource, err := common.NewObjectReference(*groupType, g.UUID.String())
	if err != nil {
		return nil, err
	}

	principalType, err := common.NewObjectType("rbac", "principal")
	if err != nil {
		return nil, err
	}

	principalResourceID := principal.ToPrincipalResourceID()
	principalRef, err := common.NewObjectReference(*principalType, principalResourceID)
	if err != nil {
		return nil, err
	}

	subject := common.NewSubjectReference(*principalRef, nil)

	return common.NewRelationTuple(*resource, "member", *subject)
}

// ReplicationTuples computes the delta for group membership changes
func (g *Group) ReplicationTuples(oldPrincipals, newPrincipals []*Principal) (toAdd, toRemove []*common.RelationTuple, err error) {
	// Convert to maps for set operations
	oldSet := make(map[string]bool)
	for _, p := range oldPrincipals {
		oldSet[p.UserID] = true
	}

	newSet := make(map[string]bool)
	for _, p := range newPrincipals {
		newSet[p.UserID] = true
	}

	// Principals to add (in new but not in old)
	for _, p := range newPrincipals {
		if !oldSet[p.UserID] {
			tuple, err := g.GroupMemberTuple(p)
			if err != nil {
				return nil, nil, err
			}
			toAdd = append(toAdd, tuple)
		}
	}

	// Principals to remove (in old but not in new)
	for _, p := range oldPrincipals {
		if !newSet[p.UserID] {
			tuple, err := g.GroupMemberTuple(p)
			if err != nil {
				return nil, nil, err
			}
			toRemove = append(toRemove, tuple)
		}
	}

	return toAdd, toRemove, nil
}

// Repository defines the interface for group persistence
type Repository interface {
	Create(group *Group) error
	Update(group *Group) error
	Delete(id uuid.UUID) error
	FindByUUID(id uuid.UUID) (*Group, error)
	FindByUUIDs(ids []uuid.UUID) ([]*Group, error)
	ListForTenant(tenantID uuid.UUID, offset, limit int) ([]*Group, error)
	FindPlatformDefault() (*Group, error)
	FindAdminDefault() (*Group, error)
}

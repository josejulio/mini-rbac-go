package rolebinding

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redhat/mini-rbac-go/internal/domain/common"
	"github.com/redhat/mini-rbac-go/internal/domain/group"
	"github.com/redhat/mini-rbac-go/internal/domain/role"
)

// RoleBinding represents a role assignment to subjects on a resource
// Mirrors the Python RoleBinding model
type RoleBinding struct {
	ID           uint        `gorm:"primarykey"`
	UUID         uuid.UUID   `gorm:"type:uuid;uniqueIndex;not null"`
	RoleID       uint        `gorm:"not null;index:idx_unique_binding,unique"`
	Role         *role.RoleV2 `gorm:"foreignKey:RoleID"`
	ResourceType string      `gorm:"size:256;not null;index:idx_unique_binding,unique"`
	ResourceID   string      `gorm:"size:256;not null;index:idx_unique_binding,unique"`
	TenantID     uuid.UUID   `gorm:"type:uuid;not null;index:idx_unique_binding,unique"`
	Created      time.Time   `gorm:"autoCreateTime"`
	Modified     time.Time   `gorm:"autoUpdateTime"`

	// Many-to-many relationships
	Groups     []*group.Group     `gorm:"many2many:role_binding_groups;"`
	Principals []*group.Principal `gorm:"many2many:role_binding_principals;"`
}

// TableName specifies the table name for GORM
func (RoleBinding) TableName() string {
	return "role_bindings"
}

// BeforeCreate generates a UUID if not set
func (rb *RoleBinding) BeforeCreate() error {
	if rb.UUID == uuid.Nil {
		rb.UUID = uuid.New()
	}
	return nil
}

// ResourceTypePair returns the (namespace, name) pair for the resource type
// Convention: resource_type "workspace" maps to ("rbac", "workspace")
func (rb *RoleBinding) ResourceTypePair() (string, string) {
	return "rbac", rb.ResourceType
}

// RoleRelationTuple generates the role relation tuple
// Format: rbac/role_binding:<uuid>#role@rbac/role:<role_uuid>
func (rb *RoleBinding) RoleRelationTuple() (*common.RelationTuple, error) {
	bindingType, _ := common.NewObjectType("rbac", "role_binding")
	resource, _ := common.NewObjectReference(*bindingType, rb.UUID.String())

	roleType, _ := common.NewObjectType("rbac", "role")
	roleRef, _ := common.NewObjectReference(*roleType, rb.Role.UUID.String())

	subject := common.NewSubjectReference(*roleRef, nil)

	return common.NewRelationTuple(*resource, "role", *subject)
}

// ResourceBindingTuple generates the resource binding tuple
// Format: rbac/<resource_type>:<resource_id>#binding@rbac/role_binding:<uuid>
func (rb *RoleBinding) ResourceBindingTuple() (*common.RelationTuple, error) {
	ns, name := rb.ResourceTypePair()
	resourceType, _ := common.NewObjectType(ns, name)
	resource, _ := common.NewObjectReference(*resourceType, rb.ResourceID)

	bindingType, _ := common.NewObjectType("rbac", "role_binding")
	bindingRef, _ := common.NewObjectReference(*bindingType, rb.UUID.String())

	subject := common.NewSubjectReference(*bindingRef, nil)

	return common.NewRelationTuple(*resource, "binding", *subject)
}

// GroupSubjectTuple generates a subject tuple for a group
// Format: rbac/role_binding:<uuid>#subject@rbac/group:<group_uuid>#member
func (rb *RoleBinding) GroupSubjectTuple(g *group.Group) (*common.RelationTuple, error) {
	bindingType, _ := common.NewObjectType("rbac", "role_binding")
	resource, _ := common.NewObjectReference(*bindingType, rb.UUID.String())

	groupType, _ := common.NewObjectType("rbac", "group")
	groupRef, _ := common.NewObjectReference(*groupType, g.UUID.String())

	memberRelation := "member"
	subject := common.NewSubjectReference(*groupRef, &memberRelation)

	return common.NewRelationTuple(*resource, "subject", *subject)
}

// PrincipalSubjectTuple generates a subject tuple for a principal
// Format: rbac/role_binding:<uuid>#subject@rbac/principal:<principal_resource_id>
func (rb *RoleBinding) PrincipalSubjectTuple(p *group.Principal) (*common.RelationTuple, error) {
	bindingType, _ := common.NewObjectType("rbac", "role_binding")
	resource, _ := common.NewObjectReference(*bindingType, rb.UUID.String())

	principalType, _ := common.NewObjectType("rbac", "principal")
	principalResourceID := p.ToPrincipalResourceID()
	principalRef, _ := common.NewObjectReference(*principalType, principalResourceID)

	subject := common.NewSubjectReference(*principalRef, nil)

	return common.NewRelationTuple(*resource, "subject", *subject)
}

// BindingTuples returns the two binding-level tuples (role + resource)
func (rb *RoleBinding) BindingTuples() ([]*common.RelationTuple, error) {
	roleTuple, err := rb.RoleRelationTuple()
	if err != nil {
		return nil, err
	}

	resourceTuple, err := rb.ResourceBindingTuple()
	if err != nil {
		return nil, err
	}

	return []*common.RelationTuple{roleTuple, resourceTuple}, nil
}

// AllTuples returns the complete set of tuples for this binding
// (role, resource, and all subject tuples)
func (rb *RoleBinding) AllTuples() ([]*common.RelationTuple, error) {
	tuples, err := rb.BindingTuples()
	if err != nil {
		return nil, err
	}

	for _, g := range rb.Groups {
		tuple, err := rb.GroupSubjectTuple(g)
		if err != nil {
			return nil, err
		}
		tuples = append(tuples, tuple)
	}

	for _, p := range rb.Principals {
		tuple, err := rb.PrincipalSubjectTuple(p)
		if err != nil {
			return nil, err
		}
		tuples = append(tuples, tuple)
	}

	return tuples, nil
}

// SubjectTuple returns the subject tuple for either a group or principal
func (rb *RoleBinding) SubjectTuple(subject interface{}) (*common.RelationTuple, error) {
	switch s := subject.(type) {
	case *group.Group:
		return rb.GroupSubjectTuple(s)
	case *group.Principal:
		return rb.PrincipalSubjectTuple(s)
	default:
		return nil, fmt.Errorf("unsupported subject type: %T", subject)
	}
}

// ReplicationTuplesInput holds the changeset for computing replication tuples
type ReplicationTuplesInput struct {
	BindingsCreated      []*RoleBinding
	BindingsDeleted      []*RoleBinding
	SubjectLinkedTo      []*RoleBinding
	SubjectUnlinkedFrom  []*RoleBinding
	Subject              interface{} // *group.Group or *group.Principal
}

// ComputeReplicationTuples computes the minimal diff for a role-binding changeset
// Mirrors the Python RoleBinding.replication_tuples static method
func ComputeReplicationTuples(input ReplicationTuplesInput) (toAdd, toRemove []*common.RelationTuple, err error) {
	// New bindings: role + resource binding tuples
	for _, binding := range input.BindingsCreated {
		tuples, err := binding.BindingTuples()
		if err != nil {
			return nil, nil, err
		}
		toAdd = append(toAdd, tuples...)
	}

	// Deleted bindings: role + resource binding tuples
	for _, binding := range input.BindingsDeleted {
		tuples, err := binding.BindingTuples()
		if err != nil {
			return nil, nil, err
		}
		toRemove = append(toRemove, tuples...)
	}

	// Subject linked: subject tuple per binding
	for _, binding := range input.SubjectLinkedTo {
		tuple, err := binding.SubjectTuple(input.Subject)
		if err != nil {
			return nil, nil, err
		}
		toAdd = append(toAdd, tuple)
	}

	// Subject unlinked: subject tuple per binding
	for _, binding := range input.SubjectUnlinkedFrom {
		tuple, err := binding.SubjectTuple(input.Subject)
		if err != nil {
			return nil, nil, err
		}
		toRemove = append(toRemove, tuple)
	}

	return toAdd, toRemove, nil
}

// Repository defines the interface for role binding persistence
type Repository interface {
	Create(binding *RoleBinding) error
	Update(binding *RoleBinding) error
	Delete(id uuid.UUID) error
	FindByUUID(id uuid.UUID) (*RoleBinding, error)
	FindByUUIDs(ids []uuid.UUID) ([]*RoleBinding, error)
	FindByRole(roleID uint) ([]*RoleBinding, error)
	FindForResource(resourceType, resourceID string, tenantID uuid.UUID) ([]*RoleBinding, error)
	FindForResourceAndRole(resourceType, resourceID string, roleID uint, tenantID uuid.UUID) (*RoleBinding, error)
	FindOrphaned() ([]*RoleBinding, error)
	ListForTenant(tenantID uuid.UUID, offset, limit int) ([]*RoleBinding, error)
}

package common

import (
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
)

// ObjectType represents a resource or subject type (namespace + name)
// Mirrors the Python RelationTuple ObjectType
type ObjectType struct {
	Namespace string
	Name      string
}

var typeRegex = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// NewObjectType creates and validates an ObjectType
func NewObjectType(namespace, name string) (*ObjectType, error) {
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if !typeRegex.MatchString(name) {
		return nil, fmt.Errorf("name must be composed of alphanumeric characters and underscores, got: %s", name)
	}
	return &ObjectType{
		Namespace: namespace,
		Name:      name,
	}, nil
}

// ObjectReference represents a reference to a resource or subject (type + id)
type ObjectReference struct {
	Type ObjectType
	ID   string
}

var idRegex = regexp.MustCompile(`^(([a-zA-Z0-9/_|\-=+.]{1,})|\*)$`)

// NewObjectReference creates and validates an ObjectReference
func NewObjectReference(objType ObjectType, id string) (*ObjectReference, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	if !idRegex.MatchString(id) {
		return nil, fmt.Errorf("invalid id format: %s", id)
	}
	return &ObjectReference{
		Type: objType,
		ID:   id,
	}, nil
}

// SubjectReference represents a reference to a subject with optional relation
type SubjectReference struct {
	Subject  ObjectReference
	Relation *string
}

// NewSubjectReference creates a SubjectReference
func NewSubjectReference(subject ObjectReference, relation *string) *SubjectReference {
	return &SubjectReference{
		Subject:  subject,
		Relation: relation,
	}
}

// RelationTuple represents a SpiceDB/Kessel relation tuple
// Mirrors the Python RelationTuple domain model
type RelationTuple struct {
	Resource ObjectReference
	Relation string
	Subject  SubjectReference
}

// NewRelationTuple creates and validates a RelationTuple
func NewRelationTuple(resource ObjectReference, relation string, subject SubjectReference) (*RelationTuple, error) {
	if relation == "" {
		return nil, fmt.Errorf("relation is required")
	}
	if resource.ID == "*" {
		return nil, fmt.Errorf("resource.id cannot be '*' (asterisk is only allowed for subjects)")
	}
	return &RelationTuple{
		Resource: resource,
		Relation: relation,
		Subject:  subject,
	}, nil
}

// Stringify returns a string representation of the tuple
func (t *RelationTuple) Stringify() string {
	subjectPart := fmt.Sprintf("%s/%s:%s",
		t.Subject.Subject.Type.Namespace,
		t.Subject.Subject.Type.Name,
		t.Subject.Subject.ID)

	if t.Subject.Relation != nil {
		subjectPart += "#" + *t.Subject.Relation
	}

	return fmt.Sprintf("%s/%s:%s#%s@%s",
		t.Resource.Type.Namespace,
		t.Resource.Type.Name,
		t.Resource.ID,
		t.Relation,
		subjectPart)
}

// BaseModel provides common fields for all models
type BaseModel struct {
	ID       uint      `gorm:"primarykey"`
	Created  time.Time `gorm:"autoCreateTime"`
	Modified time.Time `gorm:"autoUpdateTime"`
}

// TenantAwareModel extends BaseModel with tenant support
type TenantAwareModel struct {
	BaseModel
	TenantID uuid.UUID `gorm:"type:uuid;index;not null"`
}

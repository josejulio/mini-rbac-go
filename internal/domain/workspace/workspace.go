package workspace

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// WorkspaceType represents the type of a workspace
type WorkspaceType string

const (
	WorkspaceTypeStandard       WorkspaceType = "standard"
	WorkspaceTypeDefault        WorkspaceType = "default"
	WorkspaceTypeRoot           WorkspaceType = "root"
	WorkspaceTypeUngroupedHosts WorkspaceType = "ungrouped-hosts"
)

// Workspace represents a hierarchical workspace
// Mirrors the Python Workspace model
type Workspace struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	Name        string         `gorm:"size:255;not null;index"`
	Description *string        `gorm:"size:255"`
	Type        WorkspaceType  `gorm:"size:20;not null;index"`
	ParentID    *uuid.UUID     `gorm:"type:uuid;index"`
	Parent      *Workspace     `gorm:"foreignKey:ParentID"`
	TenantID    uuid.UUID      `gorm:"type:uuid;not null;index"`
	Created     time.Time      `gorm:"autoCreateTime"`
	Modified    time.Time      `gorm:"autoUpdateTime"`
}

// TableName specifies the table name for GORM
func (Workspace) TableName() string {
	return "workspaces"
}

// BeforeCreate generates a UUID if not set
func (w *Workspace) BeforeCreate() error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	return nil
}

// Validate performs domain validation
func (w *Workspace) Validate() error {
	if w.Name == "" {
		return fmt.Errorf("name is required")
	}

	if w.Type == WorkspaceTypeRoot {
		if w.ParentID != nil {
			return fmt.Errorf("root workspace must not have a parent")
		}
	} else {
		if w.ParentID == nil && w.Type != WorkspaceTypeDefault {
			return fmt.Errorf("%s workspaces must have a parent", w.Type)
		}
	}

	if w.Type == WorkspaceTypeDefault && w.ParentID != nil {
		// Note: We can't validate parent type here without loading it
		// That validation should be done in the service/repository layer
	}

	if w.ParentID != nil && *w.ParentID == w.ID {
		return fmt.Errorf("workspace cannot be its own parent")
	}

	return nil
}

// Repository defines the interface for workspace persistence
type Repository interface {
	Create(workspace *Workspace) error
	Update(workspace *Workspace) error
	Delete(id uuid.UUID) error
	FindByID(id uuid.UUID) (*Workspace, error)
	FindByIDs(ids []uuid.UUID) ([]*Workspace, error)
	FindRoot(tenantID uuid.UUID) (*Workspace, error)
	FindDefault(tenantID uuid.UUID) (*Workspace, error)
	ListForTenant(tenantID uuid.UUID, offset, limit int) ([]*Workspace, error)
	GetAncestors(id uuid.UUID) ([]*Workspace, error)
	GetDescendants(id uuid.UUID) ([]*Workspace, error)
}

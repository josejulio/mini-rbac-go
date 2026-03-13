package database

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/redhat/mini-rbac-go/internal/domain/workspace"
)

// WorkspaceRepository implements the workspace.Repository interface
type WorkspaceRepository struct {
	db *gorm.DB
}

// NewWorkspaceRepository creates a new workspace repository
func NewWorkspaceRepository(db *gorm.DB) workspace.Repository {
	return &WorkspaceRepository{db: db}
}

// Create creates a new workspace
func (r *WorkspaceRepository) Create(ws *workspace.Workspace) error {
	return r.db.Create(ws).Error
}

// Update updates an existing workspace
func (r *WorkspaceRepository) Update(ws *workspace.Workspace) error {
	return r.db.Save(ws).Error
}

// Delete deletes a workspace by ID
func (r *WorkspaceRepository) Delete(id uuid.UUID) error {
	// Check if workspace has children
	var count int64
	if err := r.db.Model(&workspace.Workspace{}).Where("parent_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("cannot delete workspace with children")
	}

	result := r.db.Where("id = ?", id).Delete(&workspace.Workspace{})
	if result.RowsAffected == 0 {
		return fmt.Errorf("workspace not found: %s", id)
	}
	return result.Error
}

// FindByID finds a workspace by ID with parent preloaded
func (r *WorkspaceRepository) FindByID(id uuid.UUID) (*workspace.Workspace, error) {
	var ws workspace.Workspace
	err := r.db.Preload("Parent").Where("id = ?", id).First(&ws).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("workspace not found: %s", id)
		}
		return nil, err
	}
	return &ws, nil
}

// FindByIDs finds multiple workspaces by IDs
func (r *WorkspaceRepository) FindByIDs(ids []uuid.UUID) ([]*workspace.Workspace, error) {
	var workspaces []*workspace.Workspace
	err := r.db.Preload("Parent").Where("id IN ?", ids).Find(&workspaces).Error
	return workspaces, err
}

// FindRoot finds the root workspace for a tenant
func (r *WorkspaceRepository) FindRoot(tenantID uuid.UUID) (*workspace.Workspace, error) {
	var ws workspace.Workspace
	err := r.db.Where("tenant_id = ? AND type = ?", tenantID, workspace.WorkspaceTypeRoot).First(&ws).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("root workspace not found for tenant")
		}
		return nil, err
	}
	return &ws, nil
}

// FindDefault finds the default workspace for a tenant
func (r *WorkspaceRepository) FindDefault(tenantID uuid.UUID) (*workspace.Workspace, error) {
	var ws workspace.Workspace
	err := r.db.Where("tenant_id = ? AND type = ?", tenantID, workspace.WorkspaceTypeDefault).First(&ws).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("default workspace not found for tenant")
		}
		return nil, err
	}
	return &ws, nil
}

// ListForTenant lists workspaces for a tenant with pagination and optional type filter
func (r *WorkspaceRepository) ListForTenant(tenantID uuid.UUID, workspaceType *workspace.WorkspaceType, offset, limit int) ([]*workspace.Workspace, error) {
	var workspaces []*workspace.Workspace
	query := r.db.Preload("Parent").Where("tenant_id = ?", tenantID)

	// Apply type filter if provided
	if workspaceType != nil {
		query = query.Where("type = ?", *workspaceType)
	}

	err := query.
		Offset(offset).
		Limit(limit).
		Order("name ASC, modified DESC").
		Find(&workspaces).Error
	return workspaces, err
}

// GetAncestors returns all ancestor workspaces from the given workspace up to root
func (r *WorkspaceRepository) GetAncestors(id uuid.UUID) ([]*workspace.Workspace, error) {
	var ancestors []*workspace.Workspace
	currentID := id

	for {
		var ws workspace.Workspace
		err := r.db.Preload("Parent").Where("id = ?", currentID).First(&ws).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				break
			}
			return nil, err
		}

		if ws.ParentID == nil {
			// Reached root
			break
		}

		var parent workspace.Workspace
		err = r.db.Where("id = ?", *ws.ParentID).First(&parent).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				break
			}
			return nil, err
		}

		ancestors = append(ancestors, &parent)
		currentID = *ws.ParentID

		// Safety check to prevent infinite loops
		if len(ancestors) > 100 {
			return nil, fmt.Errorf("workspace hierarchy too deep")
		}
	}

	return ancestors, nil
}

// GetDescendants returns all descendant workspaces (children, grandchildren, etc.)
func (r *WorkspaceRepository) GetDescendants(id uuid.UUID) ([]*workspace.Workspace, error) {
	var descendants []*workspace.Workspace

	// Recursive CTE query to get all descendants
	query := `
		WITH RECURSIVE workspace_tree AS (
			SELECT id, name, description, type, parent_id, tenant_id, created, modified
			FROM workspaces
			WHERE parent_id = ?
			UNION ALL
			SELECT w.id, w.name, w.description, w.type, w.parent_id, w.tenant_id, w.created, w.modified
			FROM workspaces w
			INNER JOIN workspace_tree wt ON w.parent_id = wt.id
		)
		SELECT * FROM workspace_tree ORDER BY name
	`

	err := r.db.Raw(query, id).Scan(&descendants).Error
	return descendants, err
}

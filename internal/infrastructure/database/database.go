package database

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/redhat/mini-rbac-go/internal/domain/rolebinding"
	"github.com/redhat/mini-rbac-go/internal/domain/group"
	"github.com/redhat/mini-rbac-go/internal/domain/role"
	"github.com/redhat/mini-rbac-go/internal/domain/tenant"
	"github.com/redhat/mini-rbac-go/internal/domain/workspace"
	"github.com/redhat/mini-rbac-go/internal/infrastructure"
)

// Database wraps the GORM database connection
type Database struct {
	DB *gorm.DB
}

// New creates a new database connection
func New(cfg *infrastructure.DatabaseConfig) (*Database, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying SQL DB for connection pooling
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying SQL DB: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	return &Database{DB: db}, nil
}

// AutoMigrate runs database migrations for all models
func (d *Database) AutoMigrate() error {
	// Order matters due to foreign key relationships
	return d.DB.AutoMigrate(
		&tenant.Tenant{},
		&tenant.TenantMapping{},
		&role.Permission{},
		&role.RoleV2{},
		&group.Principal{},
		&group.Group{},
		&workspace.Workspace{},
		&rolebinding.RoleBinding{},
	)
}

// Close closes the database connection
func (d *Database) Close() error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Ping checks if the database is accessible
func (d *Database) Ping() error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return sqlDB.PingContext(ctx)
}

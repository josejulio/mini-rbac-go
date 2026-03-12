package service_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/redhat/mini-rbac-go/internal/application/service"
	"github.com/redhat/mini-rbac-go/internal/domain/group"
	"github.com/redhat/mini-rbac-go/internal/domain/role"
	"github.com/redhat/mini-rbac-go/internal/domain/rolebinding"
	"github.com/redhat/mini-rbac-go/internal/infrastructure/database"
)

var _ = Describe("RoleV2Service", func() {
	var (
		db           *gorm.DB
		roleService  *service.RoleV2Service
		roleRepo     role.RoleRepository
		bindingRepo  rolebinding.Repository
		replicator   *mockReplicator
		testTenantID uuid.UUID
		ctx          context.Context
	)

	BeforeEach(func() {
		var err error
		// Use in-memory SQLite for tests
		db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())

		// Auto-migrate tables
		err = db.AutoMigrate(
			&role.RoleV2{},
			&rolebinding.RoleBinding{},
			&group.Group{},
			&group.Principal{},
		)
		Expect(err).NotTo(HaveOccurred())

		// Initialize repositories
		roleRepo = database.NewRoleRepository(db)
		bindingRepo = database.NewRoleBindingRepository(db)

		// Initialize mock replicator
		replicator = &mockReplicator{
			shouldFail:     false,
			capturedEvents: nil,
		}

		// Initialize services
		roleService = service.NewRoleV2Service(roleRepo, bindingRepo, replicator, db)

		testTenantID = uuid.New()
		ctx = context.Background()
	})

	AfterEach(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	})

	Describe("Create", func() {
		Context("when replication succeeds", func() {
			It("should create role and commit transaction", func() {
				desc := "Test role"
				input := service.CreateRoleInput{
					Name:        "TestRole",
					Description: &desc,
					Permissions: []map[string]string{
						{"application": "inventory", "resource_type": "hosts", "permission": "read"},
					},
					TenantID: testTenantID,
				}

				createdRole, err := roleService.Create(ctx, input)

				Expect(err).NotTo(HaveOccurred())
				Expect(createdRole).NotTo(BeNil())
				Expect(createdRole.Name).To(Equal("TestRole"))

				// Verify role persisted to database
				var dbRole role.RoleV2
				err = db.First(&dbRole, "uuid = ?", createdRole.UUID).Error
				Expect(err).NotTo(HaveOccurred())
				Expect(dbRole.Name).To(Equal("TestRole"))

				// Verify replication was called
				Expect(replicator.capturedEvents).To(HaveLen(1))
				Expect(replicator.capturedEvents[0].EventType).To(Equal("create_custom_role"))
			})
		})

		Context("when replication fails", func() {
			It("should rollback transaction and not persist role", func() {
				replicator.shouldFail = true

				desc := "Test role"
				input := service.CreateRoleInput{
					Name:        "TestRole",
					Description: &desc,
					Permissions: []map[string]string{
						{"application": "inventory", "resource_type": "hosts", "permission": "read"},
					},
					TenantID: testTenantID,
				}

				createdRole, err := roleService.Create(ctx, input)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("replication failed"))
				Expect(createdRole).To(BeNil())

				// Verify role was NOT persisted to database
				var count int64
				db.Model(&role.RoleV2{}).Where("tenant_id = ?", testTenantID).Count(&count)
				Expect(count).To(Equal(int64(0)))
			})
		})
	})

	Describe("Update", func() {
		var existingRole *role.RoleV2

		BeforeEach(func() {
			desc := "Original role"
			input := service.CreateRoleInput{
				Name:        "OriginalRole",
				Description: &desc,
				Permissions: []map[string]string{
					{"application": "inventory", "resource_type": "hosts", "permission": "read"},
				},
				TenantID: testTenantID,
			}
			var err error
			existingRole, err = roleService.Create(ctx, input)
			Expect(err).NotTo(HaveOccurred())

			// Reset replicator events
			replicator.capturedEvents = nil
		})

		Context("when replication fails", func() {
			It("should rollback transaction and not update role", func() {
				replicator.shouldFail = true

				newDesc := "Updated role"
				updateInput := service.UpdateRoleInput{
					UUID:        existingRole.UUID,
					Name:        "UpdatedRole",
					Description: &newDesc,
					Permissions: []map[string]string{
						{"application": "inventory", "resource_type": "hosts", "permission": "write"},
					},
					TenantID: testTenantID,
				}

				updatedRole, err := roleService.Update(ctx, updateInput)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("replication failed"))
				Expect(updatedRole).To(BeNil())

				// Verify role was NOT updated in database
				var dbRole role.RoleV2
				err = db.First(&dbRole, "uuid = ?", existingRole.UUID).Error
				Expect(err).NotTo(HaveOccurred())
				Expect(dbRole.Name).To(Equal("OriginalRole"))
				Expect(*dbRole.Description).To(Equal("Original role"))
			})
		})
	})

	Describe("Delete", func() {
		var existingRole *role.RoleV2
		var testGroup *group.Group
		var testBinding *rolebinding.RoleBinding

		BeforeEach(func() {
			// Create a role
			desc := "Test role for deletion"
			input := service.CreateRoleInput{
				Name:        "RoleToDelete",
				Description: &desc,
				Permissions: []map[string]string{
					{"application": "inventory", "resource_type": "hosts", "permission": "read"},
				},
				TenantID: testTenantID,
			}
			var err error
			existingRole, err = roleService.Create(ctx, input)
			Expect(err).NotTo(HaveOccurred())

			// Create a group
			testGroup = &group.Group{
				UUID:     uuid.New(),
				Name:     "TestGroup",
				TenantID: testTenantID,
			}
			err = db.Create(testGroup).Error
			Expect(err).NotTo(HaveOccurred())

			// Create a role binding with the role
			testBinding = &rolebinding.RoleBinding{
				UUID:         uuid.New(),
				RoleID:       existingRole.ID,
				Role:         existingRole,
				ResourceType: "workspace",
				ResourceID:   "default",
				TenantID:     testTenantID,
			}
			err = db.Create(testBinding).Error
			Expect(err).NotTo(HaveOccurred())

			// Add group to binding using Association API
			err = db.Model(testBinding).Association("Groups").Append(testGroup)
			Expect(err).NotTo(HaveOccurred())

			// Reset replicator
			replicator.capturedEvents = nil
		})

		Context("when role has bindings", func() {
			It("should delete role and all associated bindings", func() {
				err := roleService.Delete(ctx, existingRole.UUID, testTenantID)

				Expect(err).NotTo(HaveOccurred())

				// Verify role was deleted
				var count int64
				db.Model(&role.RoleV2{}).Where("uuid = ?", existingRole.UUID).Count(&count)
				Expect(count).To(Equal(int64(0)))

				// Verify bindings were deleted
				db.Model(&rolebinding.RoleBinding{}).Where("uuid = ?", testBinding.UUID).Count(&count)
				Expect(count).To(Equal(int64(0)))

				// Verify join table entries were cleared
				var joinCount int64
				db.Table("role_binding_groups").Where("role_binding_id = ?", testBinding.ID).Count(&joinCount)
				Expect(joinCount).To(Equal(int64(0)))
			})
		})

		Context("when replication fails", func() {
			It("should rollback transaction and not delete role or bindings", func() {
				replicator.shouldFail = true

				err := roleService.Delete(ctx, existingRole.UUID, testTenantID)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("replication failed"))

				// Verify role still exists
				var dbRole role.RoleV2
				err = db.First(&dbRole, "uuid = ?", existingRole.UUID).Error
				Expect(err).NotTo(HaveOccurred())

				// Verify binding still exists
				var dbBinding rolebinding.RoleBinding
				err = db.First(&dbBinding, "uuid = ?", testBinding.UUID).Error
				Expect(err).NotTo(HaveOccurred())

				// Verify join table entry still exists
				var joinCount int64
				db.Table("role_binding_groups").Where("role_binding_id = ?", testBinding.ID).Count(&joinCount)
				Expect(joinCount).To(Equal(int64(1)))
			})
		})
	})

	Describe("BatchDelete", func() {
		var role1, role2 *role.RoleV2

		BeforeEach(func() {
			desc := "Role 1"
			input1 := service.CreateRoleInput{
				Name:        "Role1",
				Description: &desc,
				Permissions: []map[string]string{
					{"application": "inventory", "resource_type": "hosts", "permission": "read"},
				},
				TenantID: testTenantID,
			}
			var err error
			role1, err = roleService.Create(ctx, input1)
			Expect(err).NotTo(HaveOccurred())

			desc2 := "Role 2"
			input2 := service.CreateRoleInput{
				Name:        "Role2",
				Description: &desc2,
				Permissions: []map[string]string{
					{"application": "inventory", "resource_type": "groups", "permission": "read"},
				},
				TenantID: testTenantID,
			}
			role2, err = roleService.Create(ctx, input2)
			Expect(err).NotTo(HaveOccurred())

			// Reset replicator
			replicator.capturedEvents = nil
		})

		Context("when replication fails", func() {
			It("should rollback transaction and not delete any roles", func() {
				replicator.shouldFail = true

				err := roleService.BatchDelete(ctx, []uuid.UUID{role1.UUID, role2.UUID}, testTenantID)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("replication failed"))

				// Verify both roles still exist
				var count int64
				db.Model(&role.RoleV2{}).Where("tenant_id = ?", testTenantID).Count(&count)
				Expect(count).To(Equal(int64(2)))
			})
		})
	})
})

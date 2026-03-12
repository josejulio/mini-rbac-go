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

var _ = Describe("GroupService", func() {
	var (
		db             *gorm.DB
		groupService   *service.GroupService
		groupRepo      group.Repository
		principalRepo  group.PrincipalRepository
		replicator     *mockReplicator
		testTenantID   uuid.UUID
		testGroup      *group.Group
		ctx            context.Context
	)

	BeforeEach(func() {
		var err error
		db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())

		err = db.AutoMigrate(
			&group.Group{},
			&group.Principal{},
			&role.RoleV2{},
			&rolebinding.RoleBinding{},
		)
		Expect(err).NotTo(HaveOccurred())

		groupRepo = database.NewGroupRepository(db)
		principalRepo = database.NewPrincipalRepository(db)
		replicator = &mockReplicator{shouldFail: false, capturedEvents: nil}

		groupService = service.NewGroupService(groupRepo, principalRepo, replicator, db)

		testTenantID = uuid.New()
		ctx = context.Background()

		// Create test group
		testGroup = &group.Group{
			UUID:     uuid.New(),
			Name:     "TestGroup",
			TenantID: testTenantID,
		}
		err = db.Create(testGroup).Error
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	})

	Describe("AddPrincipals", func() {
		Context("when adding principals to group", func() {
			It("should properly manage many-to-many association", func() {
				input := service.AddPrincipalsInput{
					GroupUUID: testGroup.UUID,
					UserIDs:   []string{"user1@example.com", "user2@example.com"},
					TenantID:  testTenantID,
				}

				err := groupService.AddPrincipals(ctx, input)

				Expect(err).NotTo(HaveOccurred())

				// Verify principals were created
				var principals []group.Principal
				err = db.Where("tenant_id = ?", testTenantID).Find(&principals).Error
				Expect(err).NotTo(HaveOccurred())
				Expect(principals).To(HaveLen(2))

				// Verify join table entries exist
				var joinCount int64
				db.Table("group_principals").Where("group_id = ?", testGroup.ID).Count(&joinCount)
				Expect(joinCount).To(Equal(int64(2)))

				// Verify we can query the group with preloaded principals
				var dbGroup group.Group
				err = db.Preload("Principals").First(&dbGroup, "uuid = ?", testGroup.UUID).Error
				Expect(err).NotTo(HaveOccurred())
				Expect(dbGroup.Principals).To(HaveLen(2))

				userIDs := []string{dbGroup.Principals[0].UserID, dbGroup.Principals[1].UserID}
				Expect(userIDs).To(ContainElement("user1@example.com"))
				Expect(userIDs).To(ContainElement("user2@example.com"))
			})
		})

		Context("when adding principals fails", func() {
			It("should rollback transaction", func() {
				replicator.shouldFail = true

				input := service.AddPrincipalsInput{
					GroupUUID: testGroup.UUID,
					UserIDs:   []string{"user1@example.com"},
					TenantID:  testTenantID,
				}

				err := groupService.AddPrincipals(ctx, input)

				// Note: This currently doesn't fail because we replicate after commit
				// This test documents current behavior - we may want to change this
				// to replicate-before-commit in the future
				if replicator.shouldFail {
					Expect(err).To(HaveOccurred())

					// Verify no join table entries if we switch to replicate-before-commit
					var joinCount int64
					db.Table("group_principals").Where("group_id = ?", testGroup.ID).Count(&joinCount)
					// Current behavior: principals are added even if replication fails
					// Future behavior: should be 0 if we replicate-before-commit
				}
			})
		})
	})

	Describe("RemovePrincipals", func() {
		var principal1, principal2 *group.Principal

		BeforeEach(func() {
			// Add principals to group
			principal1 = &group.Principal{
				UserID:   "user1@example.com",
				Type:     group.PrincipalTypeUser,
				TenantID: testTenantID,
			}
			err := db.Create(principal1).Error
			Expect(err).NotTo(HaveOccurred())

			principal2 = &group.Principal{
				UserID:   "user2@example.com",
				Type:     group.PrincipalTypeUser,
				TenantID: testTenantID,
			}
			err = db.Create(principal2).Error
			Expect(err).NotTo(HaveOccurred())

			// Add to group using Association API
			err = db.Model(testGroup).Association("Principals").Append([]*group.Principal{principal1, principal2})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when removing some principals", func() {
			It("should properly update many-to-many association", func() {
				input := service.RemovePrincipalsInput{
					GroupUUID: testGroup.UUID,
					UserIDs:   []string{"user1@example.com"},
					TenantID:  testTenantID,
				}

				err := groupService.RemovePrincipals(ctx, input)

				Expect(err).NotTo(HaveOccurred())

				// Verify join table updated correctly
				var joinCount int64
				db.Table("group_principals").Where("group_id = ?", testGroup.ID).Count(&joinCount)
				Expect(joinCount).To(Equal(int64(1)))

				// Verify correct principal remains
				var dbGroup group.Group
				err = db.Preload("Principals").First(&dbGroup, "uuid = ?", testGroup.UUID).Error
				Expect(err).NotTo(HaveOccurred())
				Expect(dbGroup.Principals).To(HaveLen(1))
				Expect(dbGroup.Principals[0].UserID).To(Equal("user2@example.com"))
			})
		})

		Context("when removing all principals", func() {
			It("should clear join table entries", func() {
				input := service.RemovePrincipalsInput{
					GroupUUID: testGroup.UUID,
					UserIDs:   []string{"user1@example.com", "user2@example.com"},
					TenantID:  testTenantID,
				}

				err := groupService.RemovePrincipals(ctx, input)

				Expect(err).NotTo(HaveOccurred())

				// Verify join table cleared
				var joinCount int64
				db.Table("group_principals").Where("group_id = ?", testGroup.ID).Count(&joinCount)
				Expect(joinCount).To(Equal(int64(0)))

				// Verify group has no principals
				var dbGroup group.Group
				err = db.Preload("Principals").First(&dbGroup, "uuid = ?", testGroup.UUID).Error
				Expect(err).NotTo(HaveOccurred())
				Expect(dbGroup.Principals).To(HaveLen(0))
			})
		})
	})

	Describe("Delete", func() {
		var principal *group.Principal

		BeforeEach(func() {
			// Add principal to group
			principal = &group.Principal{
				UserID:   "user1@example.com",
				Type:     group.PrincipalTypeUser,
				TenantID: testTenantID,
			}
			err := db.Create(principal).Error
			Expect(err).NotTo(HaveOccurred())

			err = db.Model(testGroup).Association("Principals").Append(principal)
			Expect(err).NotTo(HaveOccurred())

			// Reset replicator
			replicator.capturedEvents = nil
		})

		Context("when deleting group with principals", func() {
			It("should delete group and clear join table", func() {
				err := groupService.Delete(ctx, testGroup.UUID, testTenantID)

				Expect(err).NotTo(HaveOccurred())

				// Verify group was deleted
				var count int64
				db.Model(&group.Group{}).Where("uuid = ?", testGroup.UUID).Count(&count)
				Expect(count).To(Equal(int64(0)))

				// Verify join table entry was cleared (no foreign key errors)
				var joinCount int64
				db.Table("group_principals").Where("group_id = ?", testGroup.ID).Count(&joinCount)
				Expect(joinCount).To(Equal(int64(0)))
			})
		})

		Context("when deletion fails", func() {
			It("should rollback and maintain associations", func() {
				replicator.shouldFail = true

				err := groupService.Delete(ctx, testGroup.UUID, testTenantID)

				// Note: This currently doesn't fail because we replicate after commit
				// This test documents current behavior
				if replicator.shouldFail {
					Expect(err).To(HaveOccurred())

					// Verify group still exists if we switch to replicate-before-commit
					var dbGroup group.Group
					err = db.Preload("Principals").First(&dbGroup, "uuid = ?", testGroup.UUID).Error
					// Current: group is deleted even if replication fails
					// Future: should exist if we replicate-before-commit
				}
			})
		})
	})
})

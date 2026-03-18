package service_test

import (
	"context"
	"fmt"

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

var _ = Describe("RoleBindingService", func() {
	var (
		db             *gorm.DB
		bindingService *service.RoleBindingService
		roleRepo       role.RoleRepository
		bindingRepo    rolebinding.Repository
		groupRepo      group.Repository
		principalRepo  group.PrincipalRepository
		replicator     *mockReplicator
		testTenantID   uuid.UUID
		testRole       *role.RoleV2
		testGroup      *group.Group
		ctx            context.Context
	)

	BeforeEach(func() {
		var err error
		db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		Expect(err).NotTo(HaveOccurred())

		err = db.AutoMigrate(
			&role.RoleV2{},
			&rolebinding.RoleBinding{},
			&group.Group{},
			&group.Principal{},
		)
		Expect(err).NotTo(HaveOccurred())

		roleRepo = database.NewRoleRepository(db)
		bindingRepo = database.NewRoleBindingRepository(db)
		groupRepo = database.NewGroupRepository(db)
		principalRepo = database.NewPrincipalRepository(db)
		replicator = &mockReplicator{shouldFail: false, capturedEvents: nil}

		bindingService = service.NewRoleBindingService(bindingRepo, roleRepo, groupRepo, principalRepo, replicator, db)

		testTenantID = uuid.New()
		ctx = context.Background()

		// Create test role
		desc := "Test role"
		testRole = &role.RoleV2{
			UUID:        uuid.New(),
			Name:        "TestRole",
			Description: &desc,
			Type:        role.RoleTypeCustom,
			TenantID:    testTenantID,
			Permissions: []role.PermissionValue{
				{Application: "inventory", ResourceType: "hosts", Verb: "read"},
			},
		}
		err = db.Create(testRole).Error
		Expect(err).NotTo(HaveOccurred())

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

	Describe("AssignRole", func() {
		Context("when adding groups to a binding", func() {
			It("should properly manage many-to-many association", func() {
				input := service.AssignRoleInput{
					RoleUUID:     testRole.UUID,
					ResourceType: "workspace",
					ResourceID:   "default",
					SubjectType:  "group",
					SubjectIDs:   []string{testGroup.UUID.String()},
					TenantID:     testTenantID,
				}

				binding, err := bindingService.AssignRole(ctx, input)

				Expect(err).NotTo(HaveOccurred())
				Expect(binding).NotTo(BeNil())

				// Verify join table entry exists
				var joinCount int64
				db.Table("role_binding_groups").Where("role_binding_id = ? AND group_id = ?", binding.ID, testGroup.ID).Count(&joinCount)
				Expect(joinCount).To(Equal(int64(1)))

				// Verify we can query the binding with preloaded groups
				var dbBinding rolebinding.RoleBinding
				err = db.Preload("Groups").First(&dbBinding, "uuid = ?", binding.UUID).Error
				Expect(err).NotTo(HaveOccurred())
				Expect(dbBinding.Groups).To(HaveLen(1))
				Expect(dbBinding.Groups[0].UUID).To(Equal(testGroup.UUID))
			})
		})

		Context("when adding users to a binding", func() {
			It("should create principals and manage many-to-many association", func() {
				input := service.AssignRoleInput{
					RoleUUID:     testRole.UUID,
					ResourceType: "workspace",
					ResourceID:   "default",
					SubjectType:  "user",
					SubjectIDs:   []string{"alice", "bob"},
					TenantID:     testTenantID,
				}

				binding, err := bindingService.AssignRole(ctx, input)

				Expect(err).NotTo(HaveOccurred())
				Expect(binding).NotTo(BeNil())

				// Verify principals were created
				var principals []group.Principal
				err = db.Where("user_id IN ? AND tenant_id = ?", []string{"alice", "bob"}, testTenantID).Find(&principals).Error
				Expect(err).NotTo(HaveOccurred())
				Expect(principals).To(HaveLen(2))

				// Verify join table entries exist
				var joinCount int64
				db.Table("role_binding_principals").Where("role_binding_id = ?", binding.ID).Count(&joinCount)
				Expect(joinCount).To(Equal(int64(2)))

				// Verify we can query the binding with preloaded principals
				var dbBinding rolebinding.RoleBinding
				err = db.Preload("Principals").First(&dbBinding, "uuid = ?", binding.UUID).Error
				Expect(err).NotTo(HaveOccurred())
				Expect(dbBinding.Principals).To(HaveLen(2))

				userIDs := []string{dbBinding.Principals[0].UserID, dbBinding.Principals[1].UserID}
				Expect(userIDs).To(ContainElements("alice", "bob"))
			})

			It("should reuse existing principals", func() {
				// Create a principal first
				principal := &group.Principal{
					UserID:   "alice",
					Type:     group.PrincipalTypeUser,
					TenantID: testTenantID,
				}
				err := db.Create(principal).Error
				Expect(err).NotTo(HaveOccurred())

				input := service.AssignRoleInput{
					RoleUUID:     testRole.UUID,
					ResourceType: "workspace",
					ResourceID:   "default",
					SubjectType:  "user",
					SubjectIDs:   []string{"alice"},
					TenantID:     testTenantID,
				}

				binding, err := bindingService.AssignRole(ctx, input)

				Expect(err).NotTo(HaveOccurred())
				Expect(binding).NotTo(BeNil())

				// Verify only one principal exists
				var count int64
				db.Model(&group.Principal{}).Where("user_id = ? AND tenant_id = ?", "alice", testTenantID).Count(&count)
				Expect(count).To(Equal(int64(1)))
			})
		})
	})

	Describe("UnassignRole", func() {
		var testBinding *rolebinding.RoleBinding

		BeforeEach(func() {
			// Create a binding with a group
			input := service.AssignRoleInput{
				RoleUUID:     testRole.UUID,
				ResourceType: "workspace",
				ResourceID:   "default",
				SubjectType:  "group",
				SubjectIDs:   []string{testGroup.UUID.String()},
				TenantID:     testTenantID,
			}
			var err error
			testBinding, err = bindingService.AssignRole(ctx, input)
			Expect(err).NotTo(HaveOccurred())

			// Reset replicator
			replicator.capturedEvents = nil
		})

		Context("when removing last group from binding", func() {
			It("should delete binding and clear join table", func() {
				input := service.UnassignRoleInput{
					RoleUUID:     testRole.UUID,
					ResourceType: "workspace",
					ResourceID:   "default",
					SubjectType:  "group",
					SubjectIDs:   []string{testGroup.UUID.String()},
					TenantID:     testTenantID,
				}

				err := bindingService.UnassignRole(ctx, input)

				Expect(err).NotTo(HaveOccurred())

				// Verify binding was deleted
				var count int64
				db.Model(&rolebinding.RoleBinding{}).Where("uuid = ?", testBinding.UUID).Count(&count)
				Expect(count).To(Equal(int64(0)))

				// Verify join table entry was cleared (no foreign key errors)
				var joinCount int64
				db.Table("role_binding_groups").Where("role_binding_id = ?", testBinding.ID).Count(&joinCount)
				Expect(joinCount).To(Equal(int64(0)))
			})
		})

		Context("when removing users from binding", func() {
			It("should delete binding and clear join table when last user removed", func() {
				// Create a binding with users
				input := service.AssignRoleInput{
					RoleUUID:     testRole.UUID,
					ResourceType: "workspace",
					ResourceID:   "test-workspace",
					SubjectType:  "user",
					SubjectIDs:   []string{"alice", "bob"},
					TenantID:     testTenantID,
				}
				binding, err := bindingService.AssignRole(ctx, input)
				Expect(err).NotTo(HaveOccurred())

				// Remove all users
				unassignInput := service.UnassignRoleInput{
					RoleUUID:     testRole.UUID,
					ResourceType: "workspace",
					ResourceID:   "test-workspace",
					SubjectType:  "user",
					SubjectIDs:   []string{"alice", "bob"},
					TenantID:     testTenantID,
				}

				err = bindingService.UnassignRole(ctx, unassignInput)

				Expect(err).NotTo(HaveOccurred())

				// Verify binding was deleted
				var count int64
				db.Model(&rolebinding.RoleBinding{}).Where("uuid = ?", binding.UUID).Count(&count)
				Expect(count).To(Equal(int64(0)))

				// Verify join table entries were cleared
				var joinCount int64
				db.Table("role_binding_principals").Where("role_binding_id = ?", binding.ID).Count(&joinCount)
				Expect(joinCount).To(Equal(int64(0)))
			})

			It("should keep binding when some users remain", func() {
				// Create a binding with users
				input := service.AssignRoleInput{
					RoleUUID:     testRole.UUID,
					ResourceType: "workspace",
					ResourceID:   "test-workspace",
					SubjectType:  "user",
					SubjectIDs:   []string{"alice", "bob", "charlie"},
					TenantID:     testTenantID,
				}
				binding, err := bindingService.AssignRole(ctx, input)
				Expect(err).NotTo(HaveOccurred())

				// Remove only one user
				unassignInput := service.UnassignRoleInput{
					RoleUUID:     testRole.UUID,
					ResourceType: "workspace",
					ResourceID:   "test-workspace",
					SubjectType:  "user",
					SubjectIDs:   []string{"alice"},
					TenantID:     testTenantID,
				}

				err = bindingService.UnassignRole(ctx, unassignInput)

				Expect(err).NotTo(HaveOccurred())

				// Verify binding still exists
				var dbBinding rolebinding.RoleBinding
				err = db.Preload("Principals").First(&dbBinding, "uuid = ?", binding.UUID).Error
				Expect(err).NotTo(HaveOccurred())
				Expect(dbBinding.Principals).To(HaveLen(2))

				userIDs := []string{dbBinding.Principals[0].UserID, dbBinding.Principals[1].UserID}
				Expect(userIDs).To(ContainElements("bob", "charlie"))
				Expect(userIDs).NotTo(ContainElement("alice"))
			})
		})
	})

	Describe("UpdateForSubject", func() {
		var group2 *group.Group
		var role2 *role.RoleV2

		BeforeEach(func() {
			// Create another group and role
			group2 = &group.Group{
				UUID:     uuid.New(),
				Name:     "TestGroup2",
				TenantID: testTenantID,
			}
			err := db.Create(group2).Error
			Expect(err).NotTo(HaveOccurred())

			desc := "Test role 2"
			role2 = &role.RoleV2{
				UUID:        uuid.New(),
				Name:        "TestRole2",
				Description: &desc,
				Type:        role.RoleTypeCustom,
				TenantID:    testTenantID,
				Permissions: []role.PermissionValue{
					{Application: "inventory", ResourceType: "groups", Verb: "read"},
				},
			}
			err = db.Create(role2).Error
			Expect(err).NotTo(HaveOccurred())

			// Create initial bindings for testGroup
			input := service.AssignRoleInput{
				RoleUUID:     testRole.UUID,
				ResourceType: "workspace",
				ResourceID:   "default",
				SubjectType:  "group",
				SubjectIDs:   []string{testGroup.UUID.String()},
				TenantID:     testTenantID,
			}
			_, err = bindingService.AssignRole(ctx, input)
			Expect(err).NotTo(HaveOccurred())

			// Reset replicator
			replicator.capturedEvents = nil
		})

		Context("when removing all roles from subject", func() {
			It("should delete bindings and clear join tables", func() {
				result, err := bindingService.UpdateForSubject(
					ctx,
					"workspace",
					"default",
					"group",
					testGroup.UUID.String(),
					[]string{}, // Empty roles array
					testTenantID,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Roles).To(HaveLen(0))

				// Verify all bindings for this subject were removed
				var bindings []rolebinding.RoleBinding
				err = db.Preload("Groups").Where("resource_type = ? AND resource_id = ? AND tenant_id = ?",
					"workspace", "default", testTenantID).Find(&bindings).Error
				Expect(err).NotTo(HaveOccurred())

				// Check no bindings have this group
				for _, b := range bindings {
					for _, g := range b.Groups {
						Expect(g.UUID).NotTo(Equal(testGroup.UUID))
					}
				}

				// Verify no orphaned join table entries
				var joinCount int64
				db.Table("role_binding_groups").
					Joins("JOIN groups ON groups.id = role_binding_groups.group_id").
					Where("groups.uuid = ?", testGroup.UUID).
					Count(&joinCount)
				Expect(joinCount).To(Equal(int64(0)))
			})
		})

		Context("when replacing roles for subject", func() {
			It("should properly update associations", func() {
				result, err := bindingService.UpdateForSubject(
					ctx,
					"workspace",
					"default",
					"group",
					testGroup.UUID.String(),
					[]string{role2.UUID.String()}, // Replace with role2
					testTenantID,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Roles).To(HaveLen(1))
				Expect(result.Roles[0].UUID).To(Equal(role2.UUID))

				// Verify old binding removed, new binding added
				var bindings []rolebinding.RoleBinding
				err = db.Preload("Groups").Preload("Role").Where(
					"resource_type = ? AND resource_id = ? AND tenant_id = ?",
					"workspace", "default", testTenantID,
				).Find(&bindings).Error
				Expect(err).NotTo(HaveOccurred())

				// Find binding with testGroup
				var foundBinding *rolebinding.RoleBinding
				for i := range bindings {
					for _, g := range bindings[i].Groups {
						if g.UUID == testGroup.UUID {
							foundBinding = &bindings[i]
							break
						}
					}
				}

				Expect(foundBinding).NotTo(BeNil())
				Expect(foundBinding.Role.UUID).To(Equal(role2.UUID))

				// Verify join table has correct entries
				var joinCount int64
				db.Table("role_binding_groups").
					Joins("JOIN groups ON groups.id = role_binding_groups.group_id").
					Where("groups.uuid = ?", testGroup.UUID).
					Count(&joinCount)
				Expect(joinCount).To(Equal(int64(1)))
			})
		})

		Context("when assigning role to group with principals", func() {
			It("should not send duplicate tuples or group membership tuples", func() {
				// Setup: Create a group with principals (simulating user's scenario)
				groupWithPrincipals := &group.Group{
					UUID:     uuid.New(),
					Name:     "Group With Principals",
					TenantID: testTenantID,
				}
				err := db.Create(groupWithPrincipals).Error
				Expect(err).NotTo(HaveOccurred())

				// Add principals to the group
				principal1 := &group.Principal{UserID: "admin", Type: group.PrincipalTypeUser, TenantID: testTenantID}
				principal2 := &group.Principal{UserID: "jdoe", Type: group.PrincipalTypeUser, TenantID: testTenantID}
				err = db.Create(principal1).Error
				Expect(err).NotTo(HaveOccurred())
				err = db.Create(principal2).Error
				Expect(err).NotTo(HaveOccurred())

				err = db.Model(groupWithPrincipals).Association("Principals").Append([]*group.Principal{principal1, principal2})
				Expect(err).NotTo(HaveOccurred())

				// Reset the replicator to track calls
				replicator.shouldFail = false
				replicator.capturedEvents = nil

				// Act: Assign role to group on workspace
				workspaceID := uuid.New()
				result, err := bindingService.UpdateForSubject(
					ctx,
					"rbac/workspace",
					workspaceID.String(),
					"group",
					groupWithPrincipals.UUID.String(),
					[]string{testRole.UUID.String()},
					testTenantID,
				)

				// Assert
				if err != nil {
					GinkgoWriter.Printf("\n[TEST] Error occurred: %v\n", err)
					Fail(fmt.Sprintf("UpdateForSubject failed: %v", err))
				}
				if result == nil {
					Fail("UpdateForSubject returned nil result with no error")
				}
				Expect(result.Roles).To(HaveLen(1))

				// Check replication events
				Expect(replicator.capturedEvents).To(HaveLen(1))
				event := replicator.capturedEvents[0]

				// Print all tuples for debugging
				GinkgoWriter.Printf("\n[TEST] Tuples sent to replicator:\n")
				for i, t := range event.Add {
					GinkgoWriter.Printf("  ADD[%d]: %s\n", i, t.Stringify())
				}
				for i, t := range event.Remove {
					GinkgoWriter.Printf("  REMOVE[%d]: %s\n", i, t.Stringify())
				}

				// Verify no duplicates in Add tuples
				addTupleStrings := make(map[string]int)
				for _, t := range event.Add {
					tupleStr := t.Stringify()
					addTupleStrings[tupleStr]++
				}

				// Check for duplicates
				for tupleStr, count := range addTupleStrings {
					Expect(count).To(Equal(1), "Duplicate tuple found: "+tupleStr)
				}

				// Verify tuple types are correct (should be role binding tuples, not group membership tuples)
				for _, t := range event.Add {
					// Role binding tuples should have resource type "role_binding" or "workspace", not "group"
					Expect(t.Resource.Type.Name).NotTo(Equal("group"),
						"Found group membership tuple instead of role binding tuple: "+t.Stringify())

					// Should be one of: role_binding, workspace
					Expect(t.Resource.Type.Name).To(Or(
						Equal("role_binding"),
						Equal("workspace"),
					), "Unexpected resource type in tuple: "+t.Stringify())
				}
			})
		})

		Context("when updating user subject bindings", func() {
			It("should properly manage principals and associations", func() {
				// Create initial binding for user
				result, err := bindingService.UpdateForSubject(
					ctx,
					"workspace",
					"user-workspace",
					"user",
					"alice",
					[]string{testRole.UUID.String()},
					testTenantID,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Roles).To(HaveLen(1))
				Expect(result.SubjectType).To(Equal("user"))

				// Verify principal was created
				var principal group.Principal
				err = db.Where("user_id = ? AND tenant_id = ?", "alice", testTenantID).First(&principal).Error
				Expect(err).NotTo(HaveOccurred())

				// Verify join table entry exists
				var joinCount int64
				db.Table("role_binding_principals").
					Joins("JOIN principals ON principals.id = role_binding_principals.principal_id").
					Where("principals.user_id = ?", "alice").
					Count(&joinCount)
				Expect(joinCount).To(Equal(int64(1)))

				// Update to different role
				result, err = bindingService.UpdateForSubject(
					ctx,
					"workspace",
					"user-workspace",
					"user",
					"alice",
					[]string{role2.UUID.String()},
					testTenantID,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Roles).To(HaveLen(1))
				Expect(result.Roles[0].UUID).To(Equal(role2.UUID))

				// Verify still only one principal record
				var count int64
				db.Model(&group.Principal{}).Where("user_id = ? AND tenant_id = ?", "alice", testTenantID).Count(&count)
				Expect(count).To(Equal(int64(1)))
			})

			It("should remove all bindings when empty role array provided", func() {
				// Create initial binding for user
				_, err := bindingService.UpdateForSubject(
					ctx,
					"workspace",
					"user-workspace",
					"user",
					"bob",
					[]string{testRole.UUID.String()},
					testTenantID,
				)
				Expect(err).NotTo(HaveOccurred())

				// Remove all roles
				result, err := bindingService.UpdateForSubject(
					ctx,
					"workspace",
					"user-workspace",
					"user",
					"bob",
					[]string{},
					testTenantID,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Roles).To(HaveLen(0))

				// Verify no join table entries remain
				var joinCount int64
				db.Table("role_binding_principals").
					Joins("JOIN principals ON principals.id = role_binding_principals.principal_id").
					Where("principals.user_id = ?", "bob").
					Count(&joinCount)
				Expect(joinCount).To(Equal(int64(0)))
			})
		})
	})

	Describe("BatchCreate", func() {
		Context("when creating multiple bindings with groups", func() {
			It("should properly manage many-to-many associations", func() {
				requests := []service.CreateBindingRequest{
					{
						RoleID:       testRole.UUID.String(),
						ResourceType: "workspace",
						ResourceID:   "default",
						SubjectType:  "group",
						SubjectID:    testGroup.UUID.String(),
						TenantID:     testTenantID,
					},
				}

				created, err := bindingService.BatchCreate(ctx, requests)

				Expect(err).NotTo(HaveOccurred())
				Expect(created).To(HaveLen(1))

				// Verify join table entry exists
				var joinCount int64
				db.Table("role_binding_groups").
					Joins("JOIN groups ON groups.id = role_binding_groups.group_id").
					Where("groups.uuid = ?", testGroup.UUID).
					Count(&joinCount)
				Expect(joinCount).To(Equal(int64(1)))
			})
		})

		Context("when creating multiple bindings with users", func() {
			It("should create principals and manage associations", func() {
				requests := []service.CreateBindingRequest{
					{
						RoleID:       testRole.UUID.String(),
						ResourceType: "workspace",
						ResourceID:   "default",
						SubjectType:  "user",
						SubjectID:    "alice",
						TenantID:     testTenantID,
					},
					{
						RoleID:       testRole.UUID.String(),
						ResourceType: "workspace",
						ResourceID:   "workspace-2",
						SubjectType:  "user",
						SubjectID:    "bob",
						TenantID:     testTenantID,
					},
				}

				created, err := bindingService.BatchCreate(ctx, requests)

				Expect(err).NotTo(HaveOccurred())
				Expect(created).To(HaveLen(2))

				// Verify principals were created
				var principals []group.Principal
				err = db.Where("user_id IN ? AND tenant_id = ?", []string{"alice", "bob"}, testTenantID).Find(&principals).Error
				Expect(err).NotTo(HaveOccurred())
				Expect(principals).To(HaveLen(2))

				// Verify join table entries exist
				var joinCount int64
				db.Table("role_binding_principals").
					Joins("JOIN principals ON principals.id = role_binding_principals.principal_id").
					Where("principals.user_id IN ?", []string{"alice", "bob"}).
					Count(&joinCount)
				Expect(joinCount).To(Equal(int64(2)))
			})

			It("should reuse existing principals in batch create", func() {
				// Pre-create a principal
				principal := &group.Principal{
					UserID:   "alice",
					Type:     group.PrincipalTypeUser,
					TenantID: testTenantID,
				}
				err := db.Create(principal).Error
				Expect(err).NotTo(HaveOccurred())

				requests := []service.CreateBindingRequest{
					{
						RoleID:       testRole.UUID.String(),
						ResourceType: "workspace",
						ResourceID:   "default",
						SubjectType:  "user",
						SubjectID:    "alice",
						TenantID:     testTenantID,
					},
				}

				created, err := bindingService.BatchCreate(ctx, requests)

				Expect(err).NotTo(HaveOccurred())
				Expect(created).To(HaveLen(1))

				// Verify only one principal exists
				var count int64
				db.Model(&group.Principal{}).Where("user_id = ? AND tenant_id = ?", "alice", testTenantID).Count(&count)
				Expect(count).To(Equal(int64(1)))
			})
		})

		Context("when creating bindings with mixed subjects", func() {
			It("should handle both groups and users", func() {
				requests := []service.CreateBindingRequest{
					{
						RoleID:       testRole.UUID.String(),
						ResourceType: "workspace",
						ResourceID:   "default",
						SubjectType:  "group",
						SubjectID:    testGroup.UUID.String(),
						TenantID:     testTenantID,
					},
					{
						RoleID:       testRole.UUID.String(),
						ResourceType: "workspace",
						ResourceID:   "default",
						SubjectType:  "user",
						SubjectID:    "alice",
						TenantID:     testTenantID,
					},
				}

				created, err := bindingService.BatchCreate(ctx, requests)

				Expect(err).NotTo(HaveOccurred())
				Expect(created).To(HaveLen(2))

				// Verify the binding has both a group and a principal
				var binding rolebinding.RoleBinding
				err = db.Preload("Groups").Preload("Principals").
					Where("resource_type = ? AND resource_id = ? AND tenant_id = ?",
						"workspace", "default", testTenantID).
					First(&binding).Error
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.Groups).To(HaveLen(1))
				Expect(binding.Principals).To(HaveLen(1))
				Expect(binding.Groups[0].UUID).To(Equal(testGroup.UUID))
				Expect(binding.Principals[0].UserID).To(Equal("alice"))
			})
		})
	})

	Describe("ListBySubject", func() {
		BeforeEach(func() {
			// Create bindings with both groups and users
			input1 := service.AssignRoleInput{
				RoleUUID:     testRole.UUID,
				ResourceType: "workspace",
				ResourceID:   "default",
				SubjectType:  "group",
				SubjectIDs:   []string{testGroup.UUID.String()},
				TenantID:     testTenantID,
			}
			_, err := bindingService.AssignRole(ctx, input1)
			Expect(err).NotTo(HaveOccurred())

			input2 := service.AssignRoleInput{
				RoleUUID:     testRole.UUID,
				ResourceType: "workspace",
				ResourceID:   "default",
				SubjectType:  "user",
				SubjectIDs:   []string{"alice", "bob"},
				TenantID:     testTenantID,
			}
			_, err = bindingService.AssignRole(ctx, input2)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when listing all subjects", func() {
			It("should return both groups and users", func() {
				subjects, err := bindingService.ListBySubject(
					ctx,
					"workspace",
					"default",
					testTenantID,
					"",
					"",
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(subjects).To(HaveLen(3)) // 1 group + 2 users

				// Check we have one group and two users
				groupCount := 0
				userCount := 0
				for _, s := range subjects {
					if s.SubjectType == "group" {
						groupCount++
					} else if s.SubjectType == "user" {
						userCount++
					}
				}
				Expect(groupCount).To(Equal(1))
				Expect(userCount).To(Equal(2))
			})
		})

		Context("when filtering by subject type", func() {
			It("should return only groups when subject_type is group", func() {
				subjects, err := bindingService.ListBySubject(
					ctx,
					"workspace",
					"default",
					testTenantID,
					"group",
					"",
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(subjects).To(HaveLen(1))
				Expect(subjects[0].SubjectType).To(Equal("group"))
			})

			It("should return only users when subject_type is user", func() {
				subjects, err := bindingService.ListBySubject(
					ctx,
					"workspace",
					"default",
					testTenantID,
					"user",
					"",
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(subjects).To(HaveLen(2))
				for _, s := range subjects {
					Expect(s.SubjectType).To(Equal("user"))
				}
			})
		})

		Context("when filtering by subject ID", func() {
			It("should return specific user", func() {
				subjects, err := bindingService.ListBySubject(
					ctx,
					"workspace",
					"default",
					testTenantID,
					"user",
					"alice",
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(subjects).To(HaveLen(1))
				Expect(subjects[0].SubjectType).To(Equal("user"))
			})
		})
	})
})

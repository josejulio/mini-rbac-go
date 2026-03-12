package role_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	"github.com/redhat/mini-rbac-go/internal/domain/role"
)

func TestRole(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Role Domain Suite")
}

var _ = Describe("RoleV2", func() {
	var (
		testRole        *role.RoleV2
		testPermissions []role.Permission
	)

	BeforeEach(func() {
		desc := "Test role description"
		testRole = &role.RoleV2{
			UUID:        uuid.New(),
			Name:        "Test Role",
			Description: &desc,
			Type:        role.RoleTypeCustom,
			TenantID:    uuid.New(),
		}

		testPermissions = []role.Permission{
			{ID: 1, Application: "inventory", ResourceType: "hosts", Verb: "read"},
			{ID: 2, Application: "inventory", ResourceType: "hosts", Verb: "write"},
			{ID: 3, Application: "inventory", ResourceType: "groups", Verb: "read"},
		}
	})

	Describe("Validation", func() {
		Context("when name is empty", func() {
			It("should return an error", func() {
				testRole.Name = ""
				err := testRole.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("name is required"))
			})
		})

		Context("when name is only whitespace", func() {
			It("should return an error", func() {
				testRole.Name = "   "
				err := testRole.Validate()
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when name is valid", func() {
			It("should not return an error", func() {
				err := testRole.Validate()
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("ReplicationTuples", func() {
		Context("for a custom role", func() {
			It("should generate correct tuples when adding permissions", func() {
				// Convert to pointers
				permPtrs := make([]*role.Permission, len(testPermissions))
				for i := range testPermissions {
					permPtrs[i] = &testPermissions[i]
				}

				toAdd, toRemove, err := testRole.ReplicationTuples(nil, permPtrs)

				Expect(err).NotTo(HaveOccurred())
				Expect(toAdd).To(HaveLen(3))
				Expect(toRemove).To(HaveLen(0))

				// Verify tuple format
				tuple := toAdd[0]
				Expect(tuple.Resource.Type.Namespace).To(Equal("rbac"))
				Expect(tuple.Resource.Type.Name).To(Equal("role"))
				Expect(tuple.Resource.ID).To(Equal(testRole.UUID.String()))
				Expect(tuple.Subject.Subject.Type.Name).To(Equal("principal"))
				Expect(tuple.Subject.Subject.ID).To(Equal("*"))
			})

			It("should generate correct tuples when removing permissions", func() {
				permPtrs := make([]*role.Permission, len(testPermissions))
				for i := range testPermissions {
					permPtrs[i] = &testPermissions[i]
				}

				toAdd, toRemove, err := testRole.ReplicationTuples(permPtrs, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(toAdd).To(HaveLen(0))
				Expect(toRemove).To(HaveLen(3))
			})

			It("should generate delta tuples when updating permissions", func() {
				oldPerms := []*role.Permission{&testPermissions[0], &testPermissions[1]}
				newPerms := []*role.Permission{&testPermissions[1], &testPermissions[2]}

				toAdd, toRemove, err := testRole.ReplicationTuples(oldPerms, newPerms)

				Expect(err).NotTo(HaveOccurred())
				Expect(toAdd).To(HaveLen(1))    // groups:read added
				Expect(toRemove).To(HaveLen(1)) // hosts:read removed
			})
		})

		Context("for a non-custom role", func() {
			It("should return an error", func() {
				testRole.Type = role.RoleTypePlatform
				_, _, err := testRole.ReplicationTuples(nil, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("only supported for custom roles"))
			})
		})
	})

	Describe("PermissionTuple", func() {
		It("should generate correct tuple for a permission", func() {
			tuple, err := testRole.PermissionTuple(&testPermissions[0])

			Expect(err).NotTo(HaveOccurred())
			Expect(tuple).NotTo(BeNil())
			Expect(tuple.Resource.Type.Namespace).To(Equal("rbac"))
			Expect(tuple.Resource.Type.Name).To(Equal("role"))
			Expect(tuple.Resource.ID).To(Equal(testRole.UUID.String()))
			Expect(tuple.Relation).To(Equal("inventory_hosts_read"))
			Expect(tuple.Subject.Subject.Type.Namespace).To(Equal("rbac"))
			Expect(tuple.Subject.Subject.Type.Name).To(Equal("principal"))
			Expect(tuple.Subject.Subject.ID).To(Equal("*"))
		})
	})
})

var _ = Describe("Permission", func() {
	var testPerm *role.Permission

	BeforeEach(func() {
		testPerm = &role.Permission{
			Application:  "inventory",
			ResourceType: "hosts",
			Verb:         "read",
		}
	})

	Describe("String (v1 format)", func() {
		It("should return correct format", func() {
			result := testPerm.String()
			Expect(result).To(Equal("inventory:hosts:read"))
		})
	})

	Describe("V2String", func() {
		It("should return correct format", func() {
			result := testPerm.V2String()
			Expect(result).To(Equal("inventory_hosts_read"))
		})
	})

	Describe("ParseV1Permission", func() {
		It("should parse valid v1 permission string", func() {
			pv, err := role.ParseV1Permission("inventory:hosts:read")
			Expect(err).NotTo(HaveOccurred())
			Expect(pv.Application).To(Equal("inventory"))
			Expect(pv.ResourceType).To(Equal("hosts"))
			Expect(pv.Verb).To(Equal("read"))
		})

		It("should return error for invalid format", func() {
			_, err := role.ParseV1Permission("invalid")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ParseV2Permission", func() {
		It("should parse valid v2 permission string", func() {
			pv, err := role.ParseV2Permission("inventory_hosts_read")
			Expect(err).NotTo(HaveOccurred())
			Expect(pv.Application).To(Equal("inventory"))
			Expect(pv.ResourceType).To(Equal("hosts"))
			Expect(pv.Verb).To(Equal("read"))
		})

		It("should handle resource types with underscores", func() {
			pv, err := role.ParseV2Permission("inventory_host_groups_read")
			Expect(err).NotTo(HaveOccurred())
			Expect(pv.Application).To(Equal("inventory"))
			Expect(pv.ResourceType).To(Equal("host_groups"))
			Expect(pv.Verb).To(Equal("read"))
		})
	})
})

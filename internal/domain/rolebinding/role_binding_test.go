package rolebinding_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	"github.com/redhat/mini-rbac-go/internal/domain/rolebinding"
	"github.com/redhat/mini-rbac-go/internal/domain/group"
	"github.com/redhat/mini-rbac-go/internal/domain/role"
)

func TestRoleBinding(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RoleBinding Domain Suite")
}

var _ = Describe("RoleBinding", func() {
	var (
		testBinding   *rolebinding.RoleBinding
		testRole      *role.RoleV2
		testGroup     *group.Group
		testPrincipal *group.Principal
		tenantID      uuid.UUID
	)

	BeforeEach(func() {
		tenantID = uuid.New()
		desc := "Test role"
		testRole = &role.RoleV2{
			UUID:        uuid.New(),
			Name:        "Test Role",
			Description: &desc,
			Type:        role.RoleTypeCustom,
			TenantID:    tenantID,
		}

		testGroup = &group.Group{
			UUID:     uuid.New(),
			Name:     "Test Group",
			TenantID: tenantID,
		}

		testPrincipal = &group.Principal{
			UserID:   "test-user-123",
			TenantID: tenantID,
		}

		testBinding = &rolebinding.RoleBinding{
			UUID:         uuid.New(),
			RoleID:       testRole.ID,
			Role:         testRole,
			ResourceType: "workspace",
			ResourceID:   "workspace-123",
			TenantID:     tenantID,
		}
	})

	Describe("ResourceTypePair", func() {
		It("should return correct namespace and name", func() {
			ns, name := testBinding.ResourceTypePair()
			Expect(ns).To(Equal("rbac"))
			Expect(name).To(Equal("workspace"))
		})
	})

	Describe("RoleRelationTuple", func() {
		It("should generate correct role tuple", func() {
			tuple, err := testBinding.RoleRelationTuple()

			Expect(err).NotTo(HaveOccurred())
			Expect(tuple).NotTo(BeNil())
			Expect(tuple.Resource.Type.Namespace).To(Equal("rbac"))
			Expect(tuple.Resource.Type.Name).To(Equal("role_binding"))
			Expect(tuple.Resource.ID).To(Equal(testBinding.UUID.String()))
			Expect(tuple.Relation).To(Equal("role"))
			Expect(tuple.Subject.Subject.Type.Namespace).To(Equal("rbac"))
			Expect(tuple.Subject.Subject.Type.Name).To(Equal("role"))
			Expect(tuple.Subject.Subject.ID).To(Equal(testRole.UUID.String()))
			Expect(tuple.Subject.Relation).To(BeNil())
		})
	})

	Describe("ResourceBindingTuple", func() {
		It("should generate correct resource binding tuple", func() {
			tuple, err := testBinding.ResourceBindingTuple()

			Expect(err).NotTo(HaveOccurred())
			Expect(tuple).NotTo(BeNil())
			Expect(tuple.Resource.Type.Namespace).To(Equal("rbac"))
			Expect(tuple.Resource.Type.Name).To(Equal("workspace"))
			Expect(tuple.Resource.ID).To(Equal("workspace-123"))
			Expect(tuple.Relation).To(Equal("binding"))
			Expect(tuple.Subject.Subject.Type.Namespace).To(Equal("rbac"))
			Expect(tuple.Subject.Subject.Type.Name).To(Equal("role_binding"))
			Expect(tuple.Subject.Subject.ID).To(Equal(testBinding.UUID.String()))
		})
	})

	Describe("GroupSubjectTuple", func() {
		It("should generate correct group subject tuple", func() {
			tuple, err := testBinding.GroupSubjectTuple(testGroup)

			Expect(err).NotTo(HaveOccurred())
			Expect(tuple).NotTo(BeNil())
			Expect(tuple.Resource.Type.Namespace).To(Equal("rbac"))
			Expect(tuple.Resource.Type.Name).To(Equal("role_binding"))
			Expect(tuple.Resource.ID).To(Equal(testBinding.UUID.String()))
			Expect(tuple.Relation).To(Equal("subject"))
			Expect(tuple.Subject.Subject.Type.Namespace).To(Equal("rbac"))
			Expect(tuple.Subject.Subject.Type.Name).To(Equal("group"))
			Expect(tuple.Subject.Subject.ID).To(Equal(testGroup.UUID.String()))
			Expect(tuple.Subject.Relation).NotTo(BeNil())
			Expect(*tuple.Subject.Relation).To(Equal("member"))
		})
	})

	Describe("PrincipalSubjectTuple", func() {
		It("should generate correct principal subject tuple", func() {
			tuple, err := testBinding.PrincipalSubjectTuple(testPrincipal)

			Expect(err).NotTo(HaveOccurred())
			Expect(tuple).NotTo(BeNil())
			Expect(tuple.Resource.Type.Namespace).To(Equal("rbac"))
			Expect(tuple.Resource.Type.Name).To(Equal("role_binding"))
			Expect(tuple.Resource.ID).To(Equal(testBinding.UUID.String()))
			Expect(tuple.Relation).To(Equal("subject"))
			Expect(tuple.Subject.Subject.Type.Namespace).To(Equal("rbac"))
			Expect(tuple.Subject.Subject.Type.Name).To(Equal("principal"))
			Expect(tuple.Subject.Subject.ID).To(Equal(testPrincipal.ToPrincipalResourceID()))
			Expect(tuple.Subject.Relation).To(BeNil())
		})
	})

	Describe("BindingTuples", func() {
		It("should return role and resource tuples", func() {
			tuples, err := testBinding.BindingTuples()

			Expect(err).NotTo(HaveOccurred())
			Expect(tuples).To(HaveLen(2))

			// First tuple should be role tuple
			Expect(tuples[0].Relation).To(Equal("role"))
			Expect(tuples[0].Resource.Type.Name).To(Equal("role_binding"))

			// Second tuple should be resource binding tuple
			Expect(tuples[1].Relation).To(Equal("binding"))
			Expect(tuples[1].Resource.Type.Name).To(Equal("workspace"))
		})
	})

	Describe("AllTuples", func() {
		Context("with groups and principals", func() {
			BeforeEach(func() {
				testBinding.Groups = []*group.Group{testGroup}
				testBinding.Principals = []*group.Principal{testPrincipal}
			})

			It("should return all tuples including subjects", func() {
				tuples, err := testBinding.AllTuples()

				Expect(err).NotTo(HaveOccurred())
				Expect(tuples).To(HaveLen(4)) // 2 binding + 1 group + 1 principal

				// Count tuple types
				roleCount := 0
				bindingCount := 0
				subjectCount := 0

				for _, tuple := range tuples {
					switch tuple.Relation {
					case "role":
						roleCount++
					case "binding":
						bindingCount++
					case "subject":
						subjectCount++
					}
				}

				Expect(roleCount).To(Equal(1))
				Expect(bindingCount).To(Equal(1))
				Expect(subjectCount).To(Equal(2))
			})
		})

		Context("without subjects", func() {
			It("should return only binding tuples", func() {
				tuples, err := testBinding.AllTuples()

				Expect(err).NotTo(HaveOccurred())
				Expect(tuples).To(HaveLen(2))
			})
		})
	})

	Describe("SubjectTuple", func() {
		It("should generate tuple for group subject", func() {
			tuple, err := testBinding.SubjectTuple(testGroup)

			Expect(err).NotTo(HaveOccurred())
			Expect(tuple.Subject.Subject.Type.Name).To(Equal("group"))
		})

		It("should generate tuple for principal subject", func() {
			tuple, err := testBinding.SubjectTuple(testPrincipal)

			Expect(err).NotTo(HaveOccurred())
			Expect(tuple.Subject.Subject.Type.Name).To(Equal("principal"))
		})

		It("should return error for unsupported type", func() {
			_, err := testBinding.SubjectTuple("invalid")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported subject type"))
		})
	})
})

var _ = Describe("ComputeReplicationTuples", func() {
	var (
		binding1 *rolebinding.RoleBinding
		binding2 *rolebinding.RoleBinding
		testRole *role.RoleV2
		testGroup *group.Group
		tenantID uuid.UUID
	)

	BeforeEach(func() {
		tenantID = uuid.New()
		desc := "Test role"
		testRole = &role.RoleV2{
			UUID:        uuid.New(),
			Name:        "Test Role",
			Description: &desc,
			Type:        role.RoleTypeCustom,
			TenantID:    tenantID,
		}

		testGroup = &group.Group{
			UUID:     uuid.New(),
			Name:     "Test Group",
			TenantID: tenantID,
		}

		binding1 = &rolebinding.RoleBinding{
			UUID:         uuid.New(),
			Role:         testRole,
			ResourceType: "workspace",
			ResourceID:   "ws-1",
			TenantID:     tenantID,
		}

		binding2 = &rolebinding.RoleBinding{
			UUID:         uuid.New(),
			Role:         testRole,
			ResourceType: "workspace",
			ResourceID:   "ws-2",
			TenantID:     tenantID,
		}
	})

	Context("when creating bindings", func() {
		It("should generate tuples to add", func() {
			input := rolebinding.ReplicationTuplesInput{
				BindingsCreated: []*rolebinding.RoleBinding{binding1},
			}

			toAdd, toRemove, err := rolebinding.ComputeReplicationTuples(input)

			Expect(err).NotTo(HaveOccurred())
			Expect(toAdd).To(HaveLen(2)) // role + resource binding
			Expect(toRemove).To(HaveLen(0))
		})
	})

	Context("when deleting bindings", func() {
		It("should generate tuples to remove", func() {
			input := rolebinding.ReplicationTuplesInput{
				BindingsDeleted: []*rolebinding.RoleBinding{binding1},
			}

			toAdd, toRemove, err := rolebinding.ComputeReplicationTuples(input)

			Expect(err).NotTo(HaveOccurred())
			Expect(toAdd).To(HaveLen(0))
			Expect(toRemove).To(HaveLen(2)) // role + resource binding
		})
	})

	Context("when linking subject to bindings", func() {
		It("should generate subject tuples to add", func() {
			input := rolebinding.ReplicationTuplesInput{
				SubjectLinkedTo: []*rolebinding.RoleBinding{binding1, binding2},
				Subject:         testGroup,
			}

			toAdd, toRemove, err := rolebinding.ComputeReplicationTuples(input)

			Expect(err).NotTo(HaveOccurred())
			Expect(toAdd).To(HaveLen(2)) // one subject tuple per binding
			Expect(toRemove).To(HaveLen(0))

			// Verify both are subject tuples
			for _, tuple := range toAdd {
				Expect(tuple.Relation).To(Equal("subject"))
			}
		})
	})

	Context("when unlinking subject from bindings", func() {
		It("should generate subject tuples to remove", func() {
			input := rolebinding.ReplicationTuplesInput{
				SubjectUnlinkedFrom: []*rolebinding.RoleBinding{binding1},
				Subject:             testGroup,
			}

			toAdd, toRemove, err := rolebinding.ComputeReplicationTuples(input)

			Expect(err).NotTo(HaveOccurred())
			Expect(toAdd).To(HaveLen(0))
			Expect(toRemove).To(HaveLen(1))
			Expect(toRemove[0].Relation).To(Equal("subject"))
		})
	})

	Context("with complex changeset", func() {
		It("should compute correct delta", func() {
			binding3 := &rolebinding.RoleBinding{
				UUID:         uuid.New(),
				Role:         testRole,
				ResourceType: "workspace",
				ResourceID:   "ws-3",
				TenantID:     tenantID,
			}

			input := rolebinding.ReplicationTuplesInput{
				BindingsCreated:     []*rolebinding.RoleBinding{binding3},
				BindingsDeleted:     []*rolebinding.RoleBinding{binding1},
				SubjectLinkedTo:     []*rolebinding.RoleBinding{binding2},
				SubjectUnlinkedFrom: []*rolebinding.RoleBinding{binding1},
				Subject:             testGroup,
			}

			toAdd, toRemove, err := rolebinding.ComputeReplicationTuples(input)

			Expect(err).NotTo(HaveOccurred())
			// toAdd: 2 (binding3 role+resource) + 1 (subject linked to binding2) = 3
			Expect(toAdd).To(HaveLen(3))
			// toRemove: 2 (binding1 role+resource) + 1 (subject unlinked from binding1) = 3
			Expect(toRemove).To(HaveLen(3))
		})
	})
})

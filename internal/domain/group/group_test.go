package group_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	"github.com/redhat/mini-rbac-go/internal/domain/group"
)

func TestGroup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Group Domain Suite")
}

var _ = Describe("Group", func() {
	var (
		testGroup      *group.Group
		testPrincipal1 *group.Principal
		testPrincipal2 *group.Principal
		testPrincipal3 *group.Principal
		tenantID       uuid.UUID
	)

	BeforeEach(func() {
		tenantID = uuid.New()
		desc := "Test group description"
		testGroup = &group.Group{
			UUID:        uuid.New(),
			Name:        "Test Group",
			Description: &desc,
			TenantID:    tenantID,
		}

		testPrincipal1 = &group.Principal{
			UserID:   "user-1",
			TenantID: tenantID,
		}

		testPrincipal2 = &group.Principal{
			UserID:   "user-2",
			TenantID: tenantID,
		}

		testPrincipal3 = &group.Principal{
			UserID:   "user-3",
			TenantID: tenantID,
		}
	})

	Describe("Validation", func() {
		Context("when name is empty", func() {
			It("should return an error", func() {
				testGroup.Name = ""
				err := testGroup.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("name is required"))
			})
		})

		Context("when name is valid", func() {
			It("should not return an error", func() {
				err := testGroup.Validate()
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("MemberRelation", func() {
		It("should return 'member'", func() {
			relation := testGroup.MemberRelation()
			Expect(relation).To(Equal("member"))
		})
	})

	Describe("GroupMemberTuple", func() {
		It("should generate correct membership tuple", func() {
			tuple, err := testGroup.GroupMemberTuple(testPrincipal1)

			Expect(err).NotTo(HaveOccurred())
			Expect(tuple).NotTo(BeNil())
			Expect(tuple.Resource.Type.Namespace).To(Equal("rbac"))
			Expect(tuple.Resource.Type.Name).To(Equal("group"))
			Expect(tuple.Resource.ID).To(Equal(testGroup.UUID.String()))
			Expect(tuple.Relation).To(Equal("member"))
			Expect(tuple.Subject.Subject.Type.Namespace).To(Equal("rbac"))
			Expect(tuple.Subject.Subject.Type.Name).To(Equal("principal"))
			Expect(tuple.Subject.Subject.ID).To(Equal(testPrincipal1.ToPrincipalResourceID()))
			Expect(tuple.Subject.Relation).To(BeNil())
		})

		It("should generate different tuples for different principals", func() {
			tuple1, err1 := testGroup.GroupMemberTuple(testPrincipal1)
			tuple2, err2 := testGroup.GroupMemberTuple(testPrincipal2)

			Expect(err1).NotTo(HaveOccurred())
			Expect(err2).NotTo(HaveOccurred())
			Expect(tuple1.Subject.Subject.ID).NotTo(Equal(tuple2.Subject.Subject.ID))
		})
	})

	Describe("ReplicationTuples", func() {
		Context("when adding principals", func() {
			It("should generate tuples to add", func() {
				newPrincipals := []*group.Principal{testPrincipal1, testPrincipal2}

				toAdd, toRemove, err := testGroup.ReplicationTuples(nil, newPrincipals)

				Expect(err).NotTo(HaveOccurred())
				Expect(toAdd).To(HaveLen(2))
				Expect(toRemove).To(HaveLen(0))

				// Verify all tuples are membership tuples
				for _, tuple := range toAdd {
					Expect(tuple.Relation).To(Equal("member"))
					Expect(tuple.Resource.ID).To(Equal(testGroup.UUID.String()))
				}
			})
		})

		Context("when removing principals", func() {
			It("should generate tuples to remove", func() {
				oldPrincipals := []*group.Principal{testPrincipal1, testPrincipal2}

				toAdd, toRemove, err := testGroup.ReplicationTuples(oldPrincipals, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(toAdd).To(HaveLen(0))
				Expect(toRemove).To(HaveLen(2))

				// Verify all tuples are membership tuples
				for _, tuple := range toRemove {
					Expect(tuple.Relation).To(Equal("member"))
				}
			})
		})

		Context("when updating principals", func() {
			It("should compute delta correctly", func() {
				oldPrincipals := []*group.Principal{testPrincipal1, testPrincipal2}
				newPrincipals := []*group.Principal{testPrincipal2, testPrincipal3}

				toAdd, toRemove, err := testGroup.ReplicationTuples(oldPrincipals, newPrincipals)

				Expect(err).NotTo(HaveOccurred())
				Expect(toAdd).To(HaveLen(1))    // principal3 added
				Expect(toRemove).To(HaveLen(1)) // principal1 removed

				// Verify the added principal is principal3
				Expect(toAdd[0].Subject.Subject.ID).To(Equal(testPrincipal3.ToPrincipalResourceID()))

				// Verify the removed principal is principal1
				Expect(toRemove[0].Subject.Subject.ID).To(Equal(testPrincipal1.ToPrincipalResourceID()))
			})
		})

		Context("when no changes", func() {
			It("should return empty slices", func() {
				principals := []*group.Principal{testPrincipal1, testPrincipal2}

				toAdd, toRemove, err := testGroup.ReplicationTuples(principals, principals)

				Expect(err).NotTo(HaveOccurred())
				Expect(toAdd).To(HaveLen(0))
				Expect(toRemove).To(HaveLen(0))
			})
		})

		Context("with empty old and new", func() {
			It("should return empty slices", func() {
				toAdd, toRemove, err := testGroup.ReplicationTuples(nil, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(toAdd).To(HaveLen(0))
				Expect(toRemove).To(HaveLen(0))
			})
		})
	})
})

var _ = Describe("Principal", func() {
	var testPrincipal *group.Principal

	BeforeEach(func() {
		testPrincipal = &group.Principal{
			UserID:   "test-user-123",
			Type:     group.PrincipalTypeUser,
			TenantID: uuid.New(),
		}
	})

	Describe("ToPrincipalResourceID", func() {
		It("should return the user ID", func() {
			resourceID := testPrincipal.ToPrincipalResourceID()
			Expect(resourceID).To(Equal("test-user-123"))
		})

		It("should convert @ to | for SpiceDB compatibility", func() {
			testPrincipal.UserID = "user@example.com"
			resourceID := testPrincipal.ToPrincipalResourceID()
			Expect(resourceID).To(Equal("user|example.com"))
		})
	})

	Describe("Validation", func() {
		Context("when user ID is empty", func() {
			It("should return an error", func() {
				testPrincipal.UserID = ""
				err := testPrincipal.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("user_id is required"))
			})
		})

		Context("when user ID is valid", func() {
			It("should not return an error", func() {
				err := testPrincipal.Validate()
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})

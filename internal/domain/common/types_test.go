package common_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhat/mini-rbac-go/internal/domain/common"
)

func TestCommon(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Common Domain Suite")
}

var _ = Describe("ObjectType", func() {
	Describe("NewObjectType", func() {
		Context("with valid parameters", func() {
			It("should create an ObjectType", func() {
				objType, err := common.NewObjectType("rbac", "role")
				Expect(err).NotTo(HaveOccurred())
				Expect(objType).NotTo(BeNil())
				Expect(objType.Namespace).To(Equal("rbac"))
				Expect(objType.Name).To(Equal("role"))
			})

			It("should accept underscores in name", func() {
				objType, err := common.NewObjectType("rbac", "role_binding")
				Expect(err).NotTo(HaveOccurred())
				Expect(objType.Name).To(Equal("role_binding"))
			})

			It("should accept alphanumeric names", func() {
				objType, err := common.NewObjectType("app", "resource123")
				Expect(err).NotTo(HaveOccurred())
				Expect(objType.Name).To(Equal("resource123"))
			})
		})

		Context("with invalid parameters", func() {
			It("should return error when namespace is empty", func() {
				_, err := common.NewObjectType("", "role")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("namespace is required"))
			})

			It("should return error when name is empty", func() {
				_, err := common.NewObjectType("rbac", "")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("name is required"))
			})

			It("should return error for invalid characters in name", func() {
				_, err := common.NewObjectType("rbac", "role-binding")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be composed of alphanumeric"))
			})

			It("should return error for special characters", func() {
				_, err := common.NewObjectType("rbac", "role@binding")
				Expect(err).To(HaveOccurred())
			})
		})
	})
})

var _ = Describe("ObjectReference", func() {
	var objType *common.ObjectType

	BeforeEach(func() {
		objType, _ = common.NewObjectType("rbac", "role")
	})

	Describe("NewObjectReference", func() {
		Context("with valid parameters", func() {
			It("should create an ObjectReference with UUID", func() {
				ref, err := common.NewObjectReference(*objType, "550e8400-e29b-41d4-a716-446655440000")
				Expect(err).NotTo(HaveOccurred())
				Expect(ref).NotTo(BeNil())
				Expect(ref.Type.Name).To(Equal("role"))
				Expect(ref.ID).To(Equal("550e8400-e29b-41d4-a716-446655440000"))
			})

			It("should accept alphanumeric IDs", func() {
				ref, err := common.NewObjectReference(*objType, "abc123")
				Expect(err).NotTo(HaveOccurred())
				Expect(ref.ID).To(Equal("abc123"))
			})

			It("should accept IDs with allowed special characters", func() {
				ref, err := common.NewObjectReference(*objType, "resource_id-123/test")
				Expect(err).NotTo(HaveOccurred())
				Expect(ref.ID).To(Equal("resource_id-123/test"))
			})

			It("should accept wildcard for subjects", func() {
				ref, err := common.NewObjectReference(*objType, "*")
				Expect(err).NotTo(HaveOccurred())
				Expect(ref.ID).To(Equal("*"))
			})

			It("should accept IDs with pipe character", func() {
				ref, err := common.NewObjectReference(*objType, "user|123")
				Expect(err).NotTo(HaveOccurred())
				Expect(ref.ID).To(Equal("user|123"))
			})

			It("should accept IDs with equals and plus", func() {
				ref, err := common.NewObjectReference(*objType, "id=123+456")
				Expect(err).NotTo(HaveOccurred())
				Expect(ref.ID).To(Equal("id=123+456"))
			})
		})

		Context("with invalid parameters", func() {
			It("should return error when ID is empty", func() {
				_, err := common.NewObjectReference(*objType, "")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("id is required"))
			})

			It("should return error for invalid characters", func() {
				_, err := common.NewObjectReference(*objType, "id@test")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid id format"))
			})
		})
	})
})

var _ = Describe("SubjectReference", func() {
	var subjectRef *common.ObjectReference

	BeforeEach(func() {
		objType, _ := common.NewObjectType("rbac", "principal")
		subjectRef, _ = common.NewObjectReference(*objType, "user-123")
	})

	Describe("NewSubjectReference", func() {
		Context("without relation", func() {
			It("should create a SubjectReference", func() {
				ref := common.NewSubjectReference(*subjectRef, nil)
				Expect(ref).NotTo(BeNil())
				Expect(ref.Subject.ID).To(Equal("user-123"))
				Expect(ref.Relation).To(BeNil())
			})
		})

		Context("with relation", func() {
			It("should create a SubjectReference with relation", func() {
				relation := "member"
				ref := common.NewSubjectReference(*subjectRef, &relation)
				Expect(ref).NotTo(BeNil())
				Expect(ref.Relation).NotTo(BeNil())
				Expect(*ref.Relation).To(Equal("member"))
			})
		})
	})
})

var _ = Describe("RelationTuple", func() {
	var (
		resourceRef *common.ObjectReference
		subjectRef  *common.SubjectReference
	)

	BeforeEach(func() {
		resourceType, _ := common.NewObjectType("rbac", "role")
		resourceRef, _ = common.NewObjectReference(*resourceType, "role-123")

		principalType, _ := common.NewObjectType("rbac", "principal")
		principalRef, _ := common.NewObjectReference(*principalType, "user-456")
		subjectRef = common.NewSubjectReference(*principalRef, nil)
	})

	Describe("NewRelationTuple", func() {
		Context("with valid parameters", func() {
			It("should create a RelationTuple", func() {
				tuple, err := common.NewRelationTuple(*resourceRef, "owner", *subjectRef)
				Expect(err).NotTo(HaveOccurred())
				Expect(tuple).NotTo(BeNil())
				Expect(tuple.Resource.ID).To(Equal("role-123"))
				Expect(tuple.Relation).To(Equal("owner"))
				Expect(tuple.Subject.Subject.ID).To(Equal("user-456"))
			})
		})

		Context("with invalid parameters", func() {
			It("should return error when relation is empty", func() {
				_, err := common.NewRelationTuple(*resourceRef, "", *subjectRef)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("relation is required"))
			})

			It("should return error when resource ID is wildcard", func() {
				resourceType, _ := common.NewObjectType("rbac", "role")
				wildcardResource, _ := common.NewObjectReference(*resourceType, "*")
				_, err := common.NewRelationTuple(*wildcardResource, "owner", *subjectRef)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("resource.id cannot be '*'"))
			})
		})
	})

	Describe("Stringify", func() {
		Context("without subject relation", func() {
			It("should format tuple correctly", func() {
				tuple, _ := common.NewRelationTuple(*resourceRef, "owner", *subjectRef)
				str := tuple.Stringify()
				Expect(str).To(Equal("rbac/role:role-123#owner@rbac/principal:user-456"))
			})
		})

		Context("with subject relation", func() {
			It("should include subject relation in format", func() {
				groupType, _ := common.NewObjectType("rbac", "group")
				groupRef, _ := common.NewObjectReference(*groupType, "group-789")
				memberRelation := "member"
				groupSubject := common.NewSubjectReference(*groupRef, &memberRelation)

				tuple, _ := common.NewRelationTuple(*resourceRef, "viewer", *groupSubject)
				str := tuple.Stringify()
				Expect(str).To(Equal("rbac/role:role-123#viewer@rbac/group:group-789#member"))
			})
		})

		Context("with wildcard subject", func() {
			It("should format with asterisk", func() {
				principalType, _ := common.NewObjectType("rbac", "principal")
				wildcardSubject, _ := common.NewObjectReference(*principalType, "*")
				wildcardSubjectRef := common.NewSubjectReference(*wildcardSubject, nil)

				tuple, _ := common.NewRelationTuple(*resourceRef, "can_read", *wildcardSubjectRef)
				str := tuple.Stringify()
				Expect(str).To(Equal("rbac/role:role-123#can_read@rbac/principal:*"))
			})
		})

		Context("with complex IDs", func() {
			It("should handle UUIDs correctly", func() {
				roleType, _ := common.NewObjectType("rbac", "role")
				roleRef, _ := common.NewObjectReference(*roleType, "550e8400-e29b-41d4-a716-446655440000")

				principalType, _ := common.NewObjectType("rbac", "principal")
				principalRef, _ := common.NewObjectReference(*principalType, "f47ac10b-58cc-4372-a567-0e02b2c3d479")
				principalSubject := common.NewSubjectReference(*principalRef, nil)

				tuple, _ := common.NewRelationTuple(*roleRef, "inventory_hosts_read", *principalSubject)
				str := tuple.Stringify()
				Expect(str).To(Equal("rbac/role:550e8400-e29b-41d4-a716-446655440000#inventory_hosts_read@rbac/principal:f47ac10b-58cc-4372-a567-0e02b2c3d479"))
			})
		})
	})
})

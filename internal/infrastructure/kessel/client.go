package kessel

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	v1 "github.com/project-kessel/relations-api/api/kessel/relations/v1beta1"
	"github.com/redhat/mini-rbac-go/internal/domain/common"
	"github.com/redhat/mini-rbac-go/internal/infrastructure"
)

// Client wraps the Kessel relations-api gRPC client
type Client struct {
	conn         *grpc.ClientConn
	checkClient  v1.KesselCheckServiceClient
	lookupClient v1.KesselLookupServiceClient
	tupleClient  v1.KesselTupleServiceClient
	timeout      time.Duration
}

// NewClient creates a new Kessel relations-api client
func NewClient(cfg *infrastructure.RelationsAPIConfig) (*Client, error) {
	var opts []grpc.DialOption

	if !cfg.TLSEnabled {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(cfg.Address(), opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	return &Client{
		conn:         conn,
		checkClient:  v1.NewKesselCheckServiceClient(conn),
		lookupClient: v1.NewKesselLookupServiceClient(conn),
		tupleClient:  v1.NewKesselTupleServiceClient(conn),
		timeout:      cfg.Timeout,
	}, nil
}

// Close closes the gRPC connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// WriteRelationships writes (adds) relation tuples to Kessel
func (c *Client) WriteRelationships(ctx context.Context, tuples []*common.RelationTuple) error {
	if len(tuples) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Convert domain tuples to protobuf relationships
	relationships := make([]*v1.Relationship, 0, len(tuples))
	for _, tuple := range tuples {
		rel := &v1.Relationship{
			Resource: &v1.ObjectReference{
				Type: &v1.ObjectType{
					Namespace: tuple.Resource.Type.Namespace,
					Name:      tuple.Resource.Type.Name,
				},
				Id: tuple.Resource.ID,
			},
			Relation: tuple.Relation,
			Subject: &v1.SubjectReference{
				Subject: &v1.ObjectReference{
					Type: &v1.ObjectType{
						Namespace: tuple.Subject.Subject.Type.Namespace,
						Name:      tuple.Subject.Subject.Type.Name,
					},
					Id: tuple.Subject.Subject.ID,
				},
			},
		}

		if tuple.Subject.Relation != nil {
			rel.Subject.Relation = tuple.Subject.Relation
		}

		relationships = append(relationships, rel)
	}

	req := &v1.CreateTuplesRequest{
		Upsert: true, // Allow overwriting existing tuples
		Tuples: relationships,
	}

	_, err := c.tupleClient.CreateTuples(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to write relationships: %w", err)
	}

	return nil
}

// DeleteRelationships deletes (removes) relation tuples from Kessel
// Note: Kessel API uses filters for deletion. For simplicity, we delete each tuple individually.
func (c *Client) DeleteRelationships(ctx context.Context, tuples []*common.RelationTuple) error {
	if len(tuples) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Delete each tuple individually using filters
	for _, tuple := range tuples {
		filter := &v1.RelationTupleFilter{
			ResourceNamespace: &tuple.Resource.Type.Namespace,
			ResourceType:      &tuple.Resource.Type.Name,
			ResourceId:        &tuple.Resource.ID,
			Relation:          &tuple.Relation,
			SubjectFilter: &v1.SubjectFilter{
				SubjectNamespace: &tuple.Subject.Subject.Type.Namespace,
				SubjectType:      &tuple.Subject.Subject.Type.Name,
				SubjectId:        &tuple.Subject.Subject.ID,
			},
		}

		if tuple.Subject.Relation != nil {
			filter.SubjectFilter.Relation = tuple.Subject.Relation
		}

		req := &v1.DeleteTuplesRequest{
			Filter: filter,
		}

		_, err := c.tupleClient.DeleteTuples(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to delete relationship %s: %w", tuple.Stringify(), err)
		}
	}

	return nil
}

// LookupSubjects finds all subjects of a given type related to a resource
// Mirrors the Python lookup_binding_subjects function
func (c *Client) LookupSubjects(
	ctx context.Context,
	resourceType string,
	resourceID string,
	relation string,
	subjectType string,
) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Parse resource type (may include namespace)
	namespace, name := parseResourceType(resourceType)

	req := &v1.LookupSubjectsRequest{
		Resource: &v1.ObjectReference{
			Type: &v1.ObjectType{
				Namespace: namespace,
				Name:      name,
			},
			Id: resourceID,
		},
		Relation: relation,
		SubjectType: &v1.ObjectType{
			Namespace: "rbac",
			Name:      subjectType,
		},
	}

	stream, err := c.lookupClient.LookupSubjects(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup subjects: %w", err)
	}

	subjectIDs := make(map[string]bool)
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, fmt.Errorf("error receiving lookup response: %w", err)
		}

		if resp.Subject != nil && resp.Subject.Subject != nil {
			subjectIDs[resp.Subject.Subject.Id] = true
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(subjectIDs))
	for id := range subjectIDs {
		result = append(result, id)
	}

	return result, nil
}

// parseResourceType parses a resource type string into namespace and name
// Format: "namespace/name" or just "name" (defaults to "rbac" namespace)
func parseResourceType(resourceType string) (namespace, name string) {
	// Simple parsing - if contains '/', split it
	for i, ch := range resourceType {
		if ch == '/' {
			return resourceType[:i], resourceType[i+1:]
		}
	}
	// No namespace specified, default to "rbac"
	return "rbac", resourceType
}

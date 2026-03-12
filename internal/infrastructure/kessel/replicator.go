package kessel

import (
	"context"
	"fmt"
	"log"

	"github.com/redhat/mini-rbac-go/internal/domain/common"
)

// Replicator handles replication of relation tuples to Kessel
// Simplified version of Python's OutboxReplicator - direct sync instead of outbox pattern
type Replicator struct {
	client  *Client
	enabled bool
}

// NewReplicator creates a new replicator
func NewReplicator(client *Client, enabled bool) *Replicator {
	return &Replicator{
		client:  client,
		enabled: enabled,
	}
}

// ReplicationEvent represents a set of relation changes to replicate
type ReplicationEvent struct {
	EventType string
	Info      map[string]interface{}
	Add       []*common.RelationTuple
	Remove    []*common.RelationTuple
}

// Replicate processes a replication event
// If replication is disabled, this is a no-op
func (r *Replicator) Replicate(ctx context.Context, event *ReplicationEvent) error {
	if !r.enabled {
		log.Printf("[Replicator] Replication disabled, skipping event: %s", event.EventType)
		return nil
	}

	if len(event.Add) == 0 && len(event.Remove) == 0 {
		log.Printf("[Replicator] Empty event, nothing to replicate: %s", event.EventType)
		return nil
	}

	log.Printf("[Replicator] Replicating event: %s (add=%d, remove=%d)",
		event.EventType, len(event.Add), len(event.Remove))

	// Write additions
	if len(event.Add) > 0 {
		if err := r.client.WriteRelationships(ctx, event.Add); err != nil {
			return fmt.Errorf("failed to write relationships: %w", err)
		}
		log.Printf("[Replicator] Added %d relationships", len(event.Add))
	}

	// Write deletions
	if len(event.Remove) > 0 {
		if err := r.client.DeleteRelationships(ctx, event.Remove); err != nil {
			return fmt.Errorf("failed to delete relationships: %w", err)
		}
		log.Printf("[Replicator] Removed %d relationships", len(event.Remove))
	}

	log.Printf("[Replicator] Successfully replicated event: %s", event.EventType)
	return nil
}

// NoopReplicator is a replicator that does nothing (for testing or when replication is disabled)
type NoopReplicator struct{}

// NewNoopReplicator creates a no-op replicator
func NewNoopReplicator() *NoopReplicator {
	return &NoopReplicator{}
}

// Replicate does nothing
func (r *NoopReplicator) Replicate(ctx context.Context, event *ReplicationEvent) error {
	log.Printf("[NoopReplicator] Skipping replication for event: %s", event.EventType)
	return nil
}

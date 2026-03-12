package service_test

import (
	"context"
	"fmt"

	"github.com/redhat/mini-rbac-go/internal/infrastructure/kessel"
)

// Mock replicator that can be configured to fail
type mockReplicator struct {
	shouldFail     bool
	capturedEvents []*kessel.ReplicationEvent
}

func (m *mockReplicator) Replicate(ctx context.Context, event *kessel.ReplicationEvent) error {
	m.capturedEvents = append(m.capturedEvents, event)
	if m.shouldFail {
		return fmt.Errorf("mock replication failure")
	}
	return nil
}

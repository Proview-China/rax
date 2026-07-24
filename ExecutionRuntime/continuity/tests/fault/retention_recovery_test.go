package fault_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestRetentionDurableLostRepliesInspectOriginalFactWithoutRepeatingMutation(t *testing.T) {
	backend := memory.New()
	store := &retentionFaultStoreV1{RetentionStore: backend, loseCreate: true, loseCAS: true}
	clock := &testkit.Clock{Time: time.Date(2026, 7, 18, 9, 10, 0, 0, time.UTC)}
	manager, err := domain.NewRetentionManager(store, clock)
	if err != nil {
		t.Fatal(err)
	}
	created, err := manager.Create(context.Background(), "fault-retention-object", "fault-retention-policy", "sensitive")
	if err != nil {
		t.Fatal(err)
	}
	clock.Time = clock.Time.Add(time.Second)
	transitioned, err := manager.Transition(context.Background(), created.ObjectID, contract.RetentionPrivacyRequired, "fault-privacy-evidence")
	if err != nil {
		t.Fatal(err)
	}
	current, err := manager.Inspect(context.Background(), created.ObjectID)
	if err != nil || current != transitioned || current.Revision != 2 || store.createCalls != 1 || store.casCalls != 1 {
		t.Fatalf("Retention recovery drifted: current=%+v transitioned=%+v create=%d cas=%d err=%v", current, transitioned, store.createCalls, store.casCalls, err)
	}
}

type retentionFaultStoreV1 struct {
	ports.RetentionStore
	loseCreate, loseCAS   bool
	createCalls, casCalls int
}

func (s *retentionFaultStoreV1) CreateRetention(ctx context.Context, fact contract.RetentionFact) error {
	s.createCalls++
	if err := s.RetentionStore.CreateRetention(ctx, fact); err != nil {
		return err
	}
	if s.loseCreate {
		s.loseCreate = false
		return errors.New("injected Retention create reply loss")
	}
	return nil
}

func (s *retentionFaultStoreV1) CASRetention(ctx context.Context, expected uint64, fact contract.RetentionFact) error {
	s.casCalls++
	if err := s.RetentionStore.CASRetention(ctx, expected, fact); err != nil {
		return err
	}
	if s.loseCAS {
		s.loseCAS = false
		return errors.New("injected Retention CAS reply loss")
	}
	return nil
}

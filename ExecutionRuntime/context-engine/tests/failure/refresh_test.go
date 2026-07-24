package failure_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

type unknownAfterApplyStoreV1 struct {
	contextports.ContextTurnRefreshOwnerBackendV1
	once bool
}

func (s *unknownAfterApplyStoreV1) ApplyContextTurnRefreshCurrentCASV1(ctx context.Context, commit contract.ContextTurnRefreshCommitV1) (contract.ContextTurnRefreshResultV1, error) {
	result, err := s.ContextTurnRefreshOwnerBackendV1.ApplyContextTurnRefreshCurrentCASV1(ctx, commit)
	if err != nil {
		return result, err
	}
	if !s.once {
		s.once = true
		return contract.ContextTurnRefreshResultV1{}, contract.ErrUnknown
	}
	return result, nil
}

func TestRefreshLostApplyReplyRecoversOnlyByInspect(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	store := &unknownAfterApplyStoreV1{ContextTurnRefreshOwnerBackendV1: fixture.Store}
	service, err := kernel.NewContextTurnRefreshServiceV1(store, fixture.ToolReader, fixture.Parent.Content, fixture.Clock.Now, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.RefreshContextTurnV1(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	apply, err := contract.SealApplyContextTurnRefreshRequestV1(contract.ApplyContextTurnRefreshRequestV1{AttemptRef: prepared.AttemptRef, PendingDomainResultRef: prepared.PendingDomainResultRef, ExpectedCurrent: fixture.Request.ExpectedCurrent, CheckedUnixNano: fixture.Now.UnixNano(), NotAfterUnixNano: prepared.ExpiresUnixNano}, fixture.Now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ApplyContextTurnRefreshV1(context.Background(), apply); !errors.Is(err, contract.ErrUnknown) {
		t.Fatalf("err=%v", err)
	}
	inspected, err := service.InspectContextTurnRefreshV1(context.Background(), contract.InspectContextTurnRefreshRequestV1{AttemptRef: prepared.AttemptRef})
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Status != contract.ContextTurnRefreshAppliedV1 || inspected.Current == nil {
		t.Fatal("lost reply was not recoverable by exact Inspect")
	}
	if _, err = service.ApplyContextTurnRefreshV1(context.Background(), apply); !errors.Is(err, contract.ErrInspectOnly) || !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("second Apply must be inspect-only conflict: %v", err)
	}
	if _, err = service.RefreshContextTurnV1(context.Background(), fixture.Request); !errors.Is(err, contract.ErrInspectOnly) || !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("second Reserve must be inspect-only conflict: %v", err)
	}
	again, err := service.InspectContextTurnRefreshV1(context.Background(), contract.InspectContextTurnRefreshRequestV1{AttemptRef: prepared.AttemptRef})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, inspected) {
		t.Fatal("Inspect after rejected writes did not return exact applied result")
	}
}

func TestRefreshDeadlinePreservedWithZeroState(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err = fixture.Service.RefreshContextTurnV1(ctx, fixture.Request); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	attempt := contract.FactRef{ID: fixture.Request.RefreshAttemptID, Revision: 1, Digest: fixture.Request.Digest}
	if _, err = fixture.Service.InspectContextTurnRefreshV1(context.Background(), contract.InspectContextTurnRefreshRequestV1{AttemptRef: attempt}); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("deadline wrote state: %v", err)
	}
}

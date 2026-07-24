package contract_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestGovernedSessionV4CreateCASLostReplyAndVersionConflict(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	v2, candidate := testkit.GovernedFactsV2(now)
	creating := sealSessionV4(t, contract.GovernedSessionV4{ID: v2.ID, Revision: 1, Run: v2.Run, Endpoint: v2.Endpoint, Phase: contract.SessionCreatingV2, CreatedUnixNano: v2.CreatedUnixNano, UpdatedUnixNano: v2.UpdatedUnixNano})
	store := fakes.NewGovernedStoreV2()
	store.LoseNextSessionV4CreateReply = true
	if _, err := store.CreateSessionV4(context.Background(), creating); err == nil {
		t.Fatal("V4 create reply loss was not reported")
	}
	inspected, err := store.InspectSessionV4(context.Background(), creating.Run, creating.ID)
	if err != nil || !reflect.DeepEqual(inspected, creating) {
		t.Fatalf("V4 create Inspect was not exact: %#v / %v", inspected, err)
	}
	if replayed, err := store.CreateSessionV4(context.Background(), creating); err != nil || !reflect.DeepEqual(replayed, creating) {
		t.Fatalf("V4 exact create replay failed: %#v / %v", replayed, err)
	}
	if _, err := store.CreateSessionV2(context.Background(), v2); err == nil {
		t.Fatal("V2 occupied an existing V4 key")
	}
	v3 := sealCreatingSessionV3(t, v2)
	if _, err := store.CreateSessionV3(context.Background(), v3); err == nil {
		t.Fatal("V3 occupied an existing V4 key")
	}

	candidateRef, err := candidate.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	next := creating.Clone()
	next.Revision++
	next.Phase = contract.SessionWaitingModelDispatchV2
	next.Turn = 1
	next.Candidate = &candidateRef
	next.UpdatedUnixNano++
	next = sealSessionV4(t, next)
	request := sealCASV4(t, creating, next)
	store.LoseNextSessionV4CASReply = true
	if _, err := store.CompareAndSwapSessionV4(context.Background(), request); err == nil {
		t.Fatal("V4 CAS reply loss was not reported")
	}
	if replayed, err := store.CompareAndSwapSessionV4(context.Background(), request); err != nil || !reflect.DeepEqual(replayed, next) {
		t.Fatalf("V4 exact successor replay failed: %#v / %v", replayed, err)
	}
	alternative := next.Clone()
	alternative.UpdatedUnixNano++
	alternative = sealSessionV4(t, alternative)
	if _, err := store.CompareAndSwapSessionV4(context.Background(), sealCASV4(t, creating, alternative)); err == nil {
		t.Fatal("V4 valid but non-exact lost-reply successor was accepted")
	}
}

func TestGovernedSessionV4TypedNilIsUnavailable(t *testing.T) {
	var store *fakes.GovernedStoreV2
	if _, err := store.CreateSessionV4(context.Background(), contract.GovernedSessionV4{}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("Create error=%v", err)
	}
	if _, err := store.InspectSessionV4(context.Background(), contract.RunRef{}, "session"); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("Inspect error=%v", err)
	}
	if _, err := store.CompareAndSwapSessionV4(context.Background(), contract.SessionCASRequestV4{}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("CAS error=%v", err)
	}
}

func TestGovernedSessionV4SharesConflictDomainWhenV2OrV3ArrivesFirst(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	v2, _ := testkit.GovernedFactsV2(now)
	v4 := sealSessionV4(t, contract.GovernedSessionV4{ID: v2.ID, Revision: 1, Run: v2.Run, Endpoint: v2.Endpoint, Phase: contract.SessionCreatingV2, CreatedUnixNano: v2.CreatedUnixNano, UpdatedUnixNano: v2.UpdatedUnixNano})

	v2First := fakes.NewGovernedStoreV2()
	if _, err := v2First.CreateSessionV2(context.Background(), v2); err != nil {
		t.Fatal(err)
	}
	if _, err := v2First.CreateSessionV4(context.Background(), v4); err == nil {
		t.Fatal("V4 occupied a V2-first key")
	}

	v3First := fakes.NewGovernedStoreV2()
	v3 := sealCreatingSessionV3(t, v2)
	if _, err := v3First.CreateSessionV3(context.Background(), v3); err != nil {
		t.Fatal(err)
	}
	if _, err := v3First.CreateSessionV4(context.Background(), v4); err == nil {
		t.Fatal("V4 occupied a V3-first key")
	}
}

func sealSessionV4(t *testing.T, v contract.GovernedSessionV4) contract.GovernedSessionV4 {
	t.Helper()
	sealed, err := contract.SealGovernedSessionV4(v)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}
func sealCASV4(t *testing.T, current, next contract.GovernedSessionV4) contract.SessionCASRequestV4 {
	t.Helper()
	sealed, err := contract.SealSessionCASRequestV4(contract.SessionCASRequestV4{Run: current.Run, SessionID: current.ID, ExpectedRevision: current.Revision, ExpectedDigest: current.Digest, Next: next})
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

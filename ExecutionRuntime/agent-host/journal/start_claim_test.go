package journal_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
)

type lostClaimStore struct {
	inner  *journal.MemoryHostStartClaimStoreV1
	commit bool
	err    error
	calls  int
}

func (s *lostClaimStore) ClaimOrInspectHostStartV1(ctx context.Context, value contract.HostStartClaimV1) (contract.HostStartClaimV1, error) {
	s.calls++
	if s.commit {
		_, _ = s.inner.ClaimOrInspectHostStartV1(ctx, value)
	}
	return contract.HostStartClaimV1{}, s.err
}

func (s *lostClaimStore) InspectHostStartClaimV1(ctx context.Context, hostID, startID string) (contract.HostStartClaimV1, error) {
	return s.inner.InspectHostStartClaimV1(ctx, hostID, startID)
}

func (s *lostClaimStore) InspectHostStartClaimCurrentV1(ctx context.Context, expected contract.HostStartClaimRefV1) (contract.HostStartClaimV1, error) {
	return s.inner.InspectHostStartClaimCurrentV1(ctx, expected)
}

func TestHostStartAdmissionRecoversLostReplyByExactInspect(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	desired := claim(t, contract.ContractVersionV2, now)
	store := &lostClaimStore{inner: journal.NewMemoryHostStartClaimStoreV1(), commit: true, err: contract.NewError(contract.ErrorUnknownOutcome, "lost_reply", "reply lost")}
	admission, err := journal.NewHostStartAdmissionV1(store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	actual, err := admission.ClaimV1(context.Background(), desired)
	if err != nil || actual.Digest != desired.Digest || store.calls != 1 {
		t.Fatalf("actual=%+v calls=%d err=%v", actual, store.calls, err)
	}
}

func TestHostStartAdmissionDoesNotRetryUnprovenUnknown(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	desired := claim(t, contract.ContractVersionV2, now)
	store := &lostClaimStore{inner: journal.NewMemoryHostStartClaimStoreV1(), err: contract.NewError(contract.ErrorUnknownOutcome, "lost_reply", "reply lost")}
	admission, _ := journal.NewHostStartAdmissionV1(store, func() time.Time { return now })
	if _, err := admission.ClaimV1(context.Background(), desired); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("error=%v", err)
	}
	if store.calls != 1 {
		t.Fatalf("claim calls=%d", store.calls)
	}
}

func TestHostStartAdmissionRejectsExpiredAndClockRegressionBeforeWrite(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	desired := claim(t, contract.ContractVersionV2, now)
	for _, checked := range []time.Time{now.Add(-time.Nanosecond), now.Add(2 * time.Hour)} {
		store := &lostClaimStore{inner: journal.NewMemoryHostStartClaimStoreV1()}
		admission, _ := journal.NewHostStartAdmissionV1(store, func() time.Time { return checked })
		if _, err := admission.ClaimV1(context.Background(), desired); !contract.HasCode(err, contract.ErrorPrecondition) {
			t.Fatalf("checked=%v error=%v", checked, err)
		}
		if store.calls != 0 {
			t.Fatalf("checked=%v calls=%d", checked, store.calls)
		}
	}
}

func TestHostStartAdmissionV1RejectsV3BeforePermissivePortWrite(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	input := startInputJournalFixtureV3(t, now, "config-v3")
	desired, err := input.ClaimV1()
	if err != nil {
		t.Fatal(err)
	}
	// lostClaimStore is deliberately permissive: it records every mutation
	// attempt and would return success. Admission itself must keep it untouched.
	store := &lostClaimStore{inner: journal.NewMemoryHostStartClaimStoreV1()}
	admission, err := journal.NewHostStartAdmissionV1(store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = admission.ClaimV1(context.Background(), desired); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("V3 through V1 admission=%v", err)
	}
	if store.calls != 0 {
		t.Fatalf("permissive V1 port writes=%d", store.calls)
	}
}

package journal_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
)

type claimV3FaultStore struct {
	inner  *journal.MemoryHostStartClaimStoreV3
	commit bool
	err    error
	calls  atomic.Uint64
}

func (s *claimV3FaultStore) ClaimOrInspectHostStartV3(ctx context.Context, c contract.HostStartClaimV1, i contract.HostStartClaimInputV3) (contract.HostStartClaimInputBindingV3, error) {
	s.calls.Add(1)
	if s.commit {
		_, _ = s.inner.ClaimOrInspectHostStartV3(ctx, c, i)
	}
	return contract.HostStartClaimInputBindingV3{}, s.err
}
func (s *claimV3FaultStore) ClaimOrInspectHostStartV1(ctx context.Context, c contract.HostStartClaimV1) (contract.HostStartClaimV1, error) {
	return s.inner.ClaimOrInspectHostStartV1(ctx, c)
}
func (s *claimV3FaultStore) InspectHostStartClaimV1(ctx context.Context, h, id string) (contract.HostStartClaimV1, error) {
	return s.inner.InspectHostStartClaimV1(ctx, h, id)
}
func (s *claimV3FaultStore) InspectHostStartClaimCurrentV1(ctx context.Context, r contract.HostStartClaimRefV1) (contract.HostStartClaimV1, error) {
	return s.inner.InspectHostStartClaimCurrentV1(ctx, r)
}
func (s *claimV3FaultStore) InspectHostStartClaimInputV3(ctx context.Context, r contract.HostStartClaimRefV1) (contract.HostStartClaimInputBindingV3, error) {
	return s.inner.InspectHostStartClaimInputV3(ctx, r)
}

func TestHostStartAdmissionV3LostReplyExactRecoveryAndUnknown(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	input := startInputJournalFixtureV3(t, now, "config")
	for _, tc := range []struct {
		name   string
		commit bool
		wantOK bool
	}{{"committed", true, true}, {"not-committed", false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			store := &claimV3FaultStore{inner: journal.NewMemoryHostStartClaimStoreV3(), commit: tc.commit, err: contract.NewError(contract.ErrorUnknownOutcome, "lost_reply", "reply lost")}
			admission, _ := journal.NewHostStartAdmissionV3(store, func() time.Time { return now })
			got, err := admission.ClaimV3(context.Background(), input)
			if tc.wantOK {
				if err != nil || got.Input.ContentDigest != input.ContentDigest {
					t.Fatalf("got=%+v err=%v", got, err)
				}
			} else if !contract.HasCode(err, contract.ErrorUnknownOutcome) {
				t.Fatalf("err=%v", err)
			}
			if store.calls.Load() != 1 {
				t.Fatalf("writes=%d", store.calls.Load())
			}
		})
	}
}

func TestHostStartV3CannotBypassAtomicPortThroughV1Mutation(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	input := startInputJournalFixtureV3(t, now, "config")
	desired, _ := input.ClaimV1()
	store := journal.NewMemoryHostStartClaimStoreV3()
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), desired); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("V1 bypass=%v", err)
	}
	if _, err := store.InspectHostStartClaimV1(context.Background(), desired.HostID, desired.StartID); !contract.HasCode(err, contract.ErrorNotFound) {
		t.Fatalf("partial claim=%v", err)
	}
	binding, err := store.ClaimOrInspectHostStartV3(context.Background(), desired, input)
	ref, _ := desired.CurrentRefV1()
	if err != nil || binding.ClaimRef != ref {
		t.Fatalf("atomic=%+v %v", binding, err)
	}
}

func TestHostStartV1V2V3PermanentBidirectionalConflict(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	input := startInputJournalFixtureV3(t, now, "config")
	v3, _ := input.ClaimV1()
	v1 := claim(t, contract.ContractVersionV1, now)
	v2 := claim(t, contract.ContractVersionV2, now)
	store := journal.NewMemoryHostStartClaimStoreV3()
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), v1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimOrInspectHostStartV3(context.Background(), v3, input); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("V1 then V3=%v", err)
	}
	store = journal.NewMemoryHostStartClaimStoreV3()
	if _, err := store.ClaimOrInspectHostStartV3(context.Background(), v3, input); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), v2); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("V3 then V2=%v", err)
	}
}

func TestSixtyFourHostStartV3CandidatesLinearizeOneExactInput(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	store := journal.NewMemoryHostStartClaimStoreV3()
	var successes, conflicts atomic.Uint64
	var wg sync.WaitGroup
	for n := 0; n < 64; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			input := startInputJournalFixtureV3(t, now, fmt.Sprintf("config-%d", n))
			claim, _ := input.ClaimV1()
			_, err := store.ClaimOrInspectHostStartV3(context.Background(), claim, input)
			switch {
			case err == nil:
				successes.Add(1)
			case contract.HasCode(err, contract.ErrorConflict):
				conflicts.Add(1)
			default:
				t.Errorf("candidate %d: %v", n, err)
			}
		}(n)
	}
	wg.Wait()
	if successes.Load() != 1 || conflicts.Load() != 63 {
		t.Fatalf("success=%d conflict=%d", successes.Load(), conflicts.Load())
	}
}

func startInputJournalFixtureV3(t *testing.T, now time.Time, label string) contract.HostStartClaimInputV3 {
	t.Helper()
	digest := func(v string) contract.DigestV1 {
		d, e := contract.DigestJSONV1(v)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	input, err := contract.SealHostStartClaimInputV3(contract.HostStartClaimInputV3{HostID: "host-1", StartID: "start-1", DeploymentCurrentRef: contract.HostDeploymentCurrentRefV1{HostID: "host-1", DeploymentID: "deployment-1", Revision: 1, BootstrapDigest: digest("bootstrap"), ExpiresUnixNano: now.Add(2 * time.Hour).UnixNano(), Digest: digest("deployment")}, HostConfigDigest: digest(label), DefinitionSourceRef: contract.ExactRefV1{Kind: "praxis.agent-definition/source-current", ID: "source-1", Revision: 1, Digest: digest("source")}, RequestedOperation: contract.HostStartOperationStartV1, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return input
}

package journal_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
)

func TestHostStartClaimStoreRejectsV1V2AndExpiredContentReplacement(t *testing.T) {
	store := journal.NewMemoryHostStartClaimStoreV1()
	now := time.Unix(1_900_000_000, 0)
	v1 := claim(t, contract.ContractVersionV1, now)
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), v1); err != nil {
		t.Fatal(err)
	}
	if got, err := store.ClaimOrInspectHostStartV1(context.Background(), v1); err != nil || got.Digest != v1.Digest {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	v2 := claim(t, contract.ContractVersionV2, now)
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), v2); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("cross-version error=%v", err)
	}
	replacement := v1
	replacement.CreatedUnixNano = now.Add(2 * time.Hour).UnixNano()
	replacement.ExpiresUnixNano = now.Add(3 * time.Hour).UnixNano()
	replacement.Digest = ""
	var err error
	replacement, err = contract.SealHostStartClaimV1(replacement)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), replacement); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("expired replacement error=%v", err)
	}
	got, err := store.InspectHostStartClaimV1(context.Background(), v1.HostID, v1.StartID)
	if err != nil || got.Digest != v1.Digest {
		t.Fatalf("permanent claim got=%+v err=%v", got, err)
	}
}

func TestHostStartClaimStoreLinearizesOneOf64DifferentClaims(t *testing.T) {
	store := journal.NewMemoryHostStartClaimStoreV1()
	now := time.Unix(1_900_000_000, 0)
	var successes atomic.Int64
	var conflicts atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			candidate := claim(t, contract.ContractVersionV1, now.Add(time.Duration(i)*time.Nanosecond))
			if _, err := store.ClaimOrInspectHostStartV1(context.Background(), candidate); err == nil {
				successes.Add(1)
			} else if contract.HasCode(err, contract.ErrorConflict) {
				conflicts.Add(1)
			} else {
				t.Errorf("unexpected error=%v", err)
			}
		}(i)
	}
	wg.Wait()
	if successes.Load() != 1 || conflicts.Load() != 63 {
		t.Fatalf("success=%d conflicts=%d", successes.Load(), conflicts.Load())
	}
}

func TestHostStartClaimStoreTypedNilAndNilContextFailClosed(t *testing.T) {
	var store *journal.MemoryHostStartClaimStoreV1
	if _, err := store.InspectHostStartClaimV1(context.Background(), "host-1", "start-1"); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("typed nil error=%v", err)
	}
	store = journal.NewMemoryHostStartClaimStoreV1()
	if _, err := store.InspectHostStartClaimV1(nil, "host-1", "start-1"); !contract.HasCode(err, contract.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
}

func TestHostStartClaimCurrentReaderRequiresExactTypedRef(t *testing.T) {
	store := journal.NewMemoryHostStartClaimStoreV1()
	value := claim(t, contract.ContractVersionV2, time.Unix(1_900_000_000, 0))
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	expected, err := value.CurrentRefV1()
	if err != nil {
		t.Fatal(err)
	}
	actual, err := store.InspectHostStartClaimCurrentV1(context.Background(), expected)
	if err != nil || actual.Digest != value.Digest {
		t.Fatalf("actual=%+v err=%v", actual, err)
	}
	forged := expected
	forged.ExpiresUnixNano++
	if _, err := store.InspectHostStartClaimCurrentV1(context.Background(), forged); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("forged ref error=%v", err)
	}
}

func claim(t *testing.T, hostVersion string, now time.Time) contract.HostStartClaimV1 {
	t.Helper()
	digest, _ := contract.DigestJSONV1("config")
	sourceDigest, _ := contract.DigestJSONV1("source")
	value, err := contract.SealHostStartClaimV1(contract.HostStartClaimV1{ContractVersion: contract.HostStartClaimContractVersionV1, HostContractVersion: hostVersion, HostID: "host-1", StartID: "start-1", ConfigDigest: digest, DefinitionSourceRef: contract.ExactRefV1{Kind: "praxis.agent-definition/source-current", ID: "source-1", Revision: 1, Digest: sourceDigest}, RequestedOperation: contract.HostStartOperationStartV1, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

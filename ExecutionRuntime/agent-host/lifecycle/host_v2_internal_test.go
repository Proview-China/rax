package lifecycle

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
)

func TestHostV2EnsurePhaseTreatsLaterPhaseAsRecoverable(t *testing.T) {
	now := time.Unix(1_950_000_000, 0)
	store := journal.NewMemoryHostJournalStoreV2()
	coordinator, err := journal.NewCoordinatorV2(store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	value := hostJournalFixtureV2(t, now, contract.HostAssociatingGenerationV2)
	if _, err = store.CreateHostJournalV2(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	host := &HostV2{config: ConfigV2{Journal: coordinator, JournalFacts: store, Clock: func() time.Time { return now }}}
	actual, err := host.ensurePhaseV2(context.Background(), value, contract.HostValidatingV2)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Digest != value.Digest || actual.Phase != contract.HostAssociatingGenerationV2 {
		t.Fatalf("later phase mutated during recovery: %+v", actual)
	}
}

func TestTimedOwnerCallV2RejectsClockRollbackAfterEveryOwnerCall(t *testing.T) {
	now := time.Unix(1_950_000_100, 0)
	var calls atomic.Int64
	host := &HostV2{config: ConfigV2{Clock: func() time.Time {
		if calls.Add(1)%2 == 1 {
			return now
		}
		return now.Add(-time.Nanosecond)
	}}}
	for _, owner := range []string{"compile", "binding", "control", "activation", "generation", "ready"} {
		t.Run(owner, func(t *testing.T) {
			_, err := timedOwnerCallV2(host, now.Add(-time.Second).UnixNano(), func() (contract.HostOperationCoordinateV2, error) {
				return hostCoordinateFixtureV2(t, owner), nil
			}, func(value contract.HostOperationCoordinateV2) contract.HostOperationCoordinateV2 { return value }, func(contract.HostOperationCoordinateV2, time.Time, error) error { return nil })
			if !contract.HasCode(err, contract.ErrorPrecondition) {
				t.Fatalf("%s rollback err=%v", owner, err)
			}
		})
	}
}

func TestPublicationAttemptsRequireAllFiveExactResults(t *testing.T) {
	now := time.Unix(1_950_000_200, 0)
	value := hostJournalFixtureV2(t, now, contract.HostCompilingV2)
	for _, step := range []string{"generation", "manifest", "graph", "handoff", "commit"} {
		attempt, err := contract.SealHostOperationAttemptV2(contract.HostOperationAttemptV2{ContractVersion: contract.HostJournalContractVersionV2, AttemptID: "publish-publication-" + step, Revision: 2, OperationKind: "praxis.agent-host/assembly-publication-" + step, Phase: contract.HostCompilingV2, RequestDigest: hostDigestFixtureV2(t, "request-"+step), Inputs: []contract.HostOperationCoordinateV2{hostCoordinateFixtureV2(t, "input-"+step)}, State: contract.HostOperationResultRecordedV2, Result: ptrCoordinateV2(hostCoordinateFixtureV2(t, step)), CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
		if err != nil {
			t.Fatal(err)
		}
		value.Operations = append(value.Operations, attempt)
	}
	value.Digest = ""
	value, _ = contract.SealHostJournalV2(value)
	complete, err := publicationAttemptsCompleteV2(value, "publish")
	if err != nil || !complete {
		t.Fatalf("complete=%v err=%v", complete, err)
	}
	value.Operations = append([]contract.HostOperationAttemptV2(nil), value.Operations[1:]...)
	value.Digest = ""
	value, _ = contract.SealHostJournalV2(value)
	if _, err = publicationAttemptsCompleteV2(value, "publish"); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("commit without all predecessors err=%v", err)
	}
}

func TestHostV2OwnerIntentSixtyFourIndependentHostsDispatchOnce(t *testing.T) {
	now := time.Unix(1_950_000_300, 0)
	store := journal.NewMemoryHostJournalStoreV2()
	value := hostJournalFixtureV2(t, now, contract.HostBindingV2)
	if _, err := store.CreateHostJournalV2(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	request := hostStartRequestFixtureV2(t, now)
	result := hostCoordinateFixtureV2(t, "binding-result")
	var starts atomic.Int64
	var mu sync.Mutex
	visible := false
	start := func(context.Context) (contract.HostOperationCoordinateV2, error) {
		starts.Add(1)
		mu.Lock()
		visible = true
		mu.Unlock()
		return result, nil
	}
	inspect := func(context.Context) (contract.HostOperationCoordinateV2, error) {
		mu.Lock()
		defer mu.Unlock()
		if !visible {
			return contract.HostOperationCoordinateV2{}, contract.NewError(contract.ErrorNotFound, "fixture_missing", "fixture result missing")
		}
		return result, nil
	}
	var wait sync.WaitGroup
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			coordinator, _ := journal.NewCoordinatorV2(store, func() time.Time { return now })
			host := &HostV2{config: ConfigV2{Journal: coordinator, JournalFacts: store, Clock: func() time.Time { return now }}}
			_ = host.runOwnerStepV2(context.Background(), request, contract.HostBindingV2, "fixture/binding", "binding", []contract.HostOperationCoordinateV2{hostCoordinateFixtureV2(t, "input")}, start, inspect)
		}()
	}
	wait.Wait()
	if starts.Load() != 1 {
		t.Fatalf("start calls=%d", starts.Load())
	}
}

func TestHostV2InspectAcceptedOriginUsesExactClaim(t *testing.T) {
	now := time.Unix(1_950_000_400, 0)
	store := journal.NewMemoryHostJournalStoreV2()
	value := hostJournalFixtureV2(t, now, contract.HostAcceptedV2)
	if _, err := store.CreateHostJournalV2(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	coordinator, _ := journal.NewCoordinatorV2(store, func() time.Time { return now })
	host := &HostV2{config: ConfigV2{Journal: coordinator}}
	request := contract.InspectRequestV2{HostID: value.HostID, StartID: value.StartID, StartClaimRef: value.StartClaimRef, RequestDigest: hostDigestFixtureV2(t, "start-request")}
	if _, err := host.InspectV2(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	drift := request
	drift.StartClaimRef.Digest = hostDigestFixtureV2(t, "other-claim")
	if _, err := host.InspectV2(context.Background(), drift); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("claim drift err=%v", err)
	}
}

func hostJournalFixtureV2(t *testing.T, now time.Time, phase contract.HostPhaseV2) contract.HostJournalV2 {
	t.Helper()
	value, err := contract.SealHostJournalV2(contract.HostJournalV2{ContractVersion: contract.HostJournalContractVersionV2, HostID: "host-v2-test", StartID: "start-v2-test", Revision: 1, Phase: phase, StartClaimRef: contract.ExactRefV1{Kind: "praxis.agent-host/start-claim", ID: "claim-v2-test", Revision: 1, Digest: hostDigestFixtureV2(t, "claim")}, ConfigDigest: hostDigestFixtureV2(t, "config"), CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func hostStartRequestFixtureV2(t *testing.T, now time.Time) contract.StartRequestV2 {
	t.Helper()
	value, err := contract.SealStartRequestV2(contract.StartRequestV2{StartID: "start-v2-test", Config: contract.HostConfigV1{ContractVersion: contract.ContractVersionV1, HostID: "host-v2-test", DefinitionSourceRef: "definition-source", StatePlaneBindings: []string{"state-plane"}, ProviderEndpointRefs: []string{"provider"}, SecretBrokerRef: "secret-broker", CatalogRef: "catalog", ResolutionFactsRef: "resolution", RuntimeServiceRefs: []string{"runtime"}, ListenRef: "listen", DiagnosticsPolicyRef: "diagnostics"}, DefinitionSourceCurrent: contract.ExactRefV1{Kind: "praxis.agent-definition/source", ID: "definition-source", Revision: 1, Digest: hostDigestFixtureV2(t, "definition-source")}, RequestedAtUnixNano: now.Add(-time.Second).UnixNano(), RequestedNotAfterUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func hostCoordinateFixtureV2(t *testing.T, id string) contract.HostOperationCoordinateV2 {
	t.Helper()
	return contract.HostOperationCoordinateV2{ContractKind: "fixture/v1", OwnerID: "fixture.owner", ID: id, Revision: 1, Digest: hostDigestFixtureV2(t, id)}
}

func ptrCoordinateV2(value contract.HostOperationCoordinateV2) *contract.HostOperationCoordinateV2 {
	return &value
}

func hostDigestFixtureV2(t *testing.T, value string) contract.DigestV1 {
	t.Helper()
	digest, err := contract.DigestJSONV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

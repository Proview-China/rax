package fakes

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGovernedStoreV2LostRepliesRecoverOnlyByExactInspect(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	store := NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	session, candidate := governedFactsFixtureV2(t, now)
	store.LoseNextSessionCreateReply = true
	if _, err := store.CreateSessionV2(context.Background(), session); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected lost session reply: %v", err)
	}
	inspected, err := store.InspectSessionV2(context.Background(), session.Run, session.ID)
	if err != nil || digestSessionV2(inspected) != digestSessionV2(session) {
		t.Fatalf("session exact inspect failed: %v", err)
	}
	conflict := session
	conflict.UpdatedUnixNano++
	if _, err := store.CreateSessionV2(context.Background(), conflict); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("different session replay accepted: %v", err)
	}

	store.LoseNextCandidateCreateReply = true
	if _, err := store.CreateCandidateV2(context.Background(), candidate); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected lost candidate reply: %v", err)
	}
	observedCandidate, err := store.InspectCandidateV2(context.Background(), candidate.Run, candidate.ID)
	if err != nil || digestCandidateV2(observedCandidate) != digestCandidateV2(candidate) {
		t.Fatalf("candidate exact inspect failed: %v", err)
	}
	changed := candidate
	changed.ContextRef = "context-other"
	if _, err := store.CreateCandidateV2(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("different candidate replay accepted: %v", err)
	}
}

func TestGovernedStoreV2CASLinearizesOneConcurrentTransition(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	store := NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	session, candidate := governedFactsFixtureV2(t, now)
	if _, err := store.CreateSessionV2(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateCandidateV2(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	ref, err := candidate.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	next := session
	next.Revision, next.Phase, next.Turn, next.Candidate, next.UpdatedUnixNano = 2, contract.SessionWaitingModelDispatchV2, 1, &ref, now.Add(time.Second).UnixNano()

	var successes atomic.Int32
	var wait sync.WaitGroup
	for range 100 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := store.CompareAndSwapSessionV2(context.Background(), harnessports.SessionCASRequestV2{ExpectedRevision: 1, Next: next}); err == nil {
				successes.Add(1)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 {
		t.Fatalf("CAS linearized %d transitions, want one", successes.Load())
	}
	current, err := store.InspectSessionV2(context.Background(), session.Run, session.ID)
	if err != nil || current.Revision != 2 || current.Phase != contract.SessionWaitingModelDispatchV2 {
		t.Fatalf("unexpected current session: %#v err=%v", current, err)
	}
}

func TestGovernedStoreV2CASReplyLossNeverRedispatchesStateTransition(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	store := NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	session, candidate := governedFactsFixtureV2(t, now)
	_, _ = store.CreateSessionV2(context.Background(), session)
	_, _ = store.CreateCandidateV2(context.Background(), candidate)
	ref, _ := candidate.RefV2()
	next := session
	next.Revision, next.Phase, next.Turn, next.Candidate, next.UpdatedUnixNano = 2, contract.SessionWaitingModelDispatchV2, 1, &ref, now.Add(time.Second).UnixNano()
	store.LoseNextSessionCASReply = true
	if _, err := store.CompareAndSwapSessionV2(context.Background(), harnessports.SessionCASRequestV2{ExpectedRevision: 1, Next: next}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected CAS reply loss: %v", err)
	}
	current, err := store.InspectSessionV2(context.Background(), session.Run, session.ID)
	if err != nil || digestSessionV2(current) != digestSessionV2(next) {
		t.Fatalf("lost CAS reply was not inspectable: %v", err)
	}
	if _, err := store.CompareAndSwapSessionV2(context.Background(), harnessports.SessionCASRequestV2{ExpectedRevision: 1, Next: next}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("lost reply caused a second transition: %v", err)
	}
}

func TestGovernedStoreV2RejectsSecondActiveRunInSameExecutionScope(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	store := NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	first, _ := governedFactsFixtureV2(t, now)
	if _, err := store.CreateSessionV2(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ID = "session-2"
	second.Run.RunID = "run-2"
	if _, err := store.CreateSessionV2(context.Background(), second); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("second active run in same scope accepted: %v", err)
	}
}

func TestGovernedStoreV2ClonesCallerAndReaderOwnedMutableValues(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	store := NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	session, candidate := governedFactsFixtureV2(t, now)
	lookup := session.Run
	lookup.Scope.SandboxLease = &core.SandboxLeaseRef{ID: session.Run.Scope.SandboxLease.ID, Epoch: session.Run.Scope.SandboxLease.Epoch}
	originalByte := candidate.Input.Inline[0]
	if _, err := store.CreateSessionV2(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateCandidateV2(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	candidate.Input.Inline[0] ^= 0xff
	candidate.Run.Scope.SandboxLease.Epoch++
	read, err := store.InspectCandidateV2(context.Background(), lookup, "candidate-1")
	if err != nil {
		t.Fatal(err)
	}
	if read.Input.Inline[0] != originalByte || read.Run.Scope.SandboxLease.Epoch != 1 {
		t.Fatal("caller mutation escaped into candidate store")
	}
	read.Input.Inline[0] ^= 0xff
	read.Run.Scope.SandboxLease.Epoch++
	again, err := store.InspectCandidateV2(context.Background(), lookup, "candidate-1")
	if err != nil || again.Input.Inline[0] != originalByte || again.Run.Scope.SandboxLease.Epoch != 1 {
		t.Fatal("reader mutation escaped into candidate store")
	}
}

func governedFactsFixtureV2(t *testing.T, now time.Time) (contract.GovernedSessionV2, contract.ModelTurnCandidateV2) {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: digest("plan")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox-1", Epoch: 1}, AuthorityEpoch: 1}
	harnessBinding := runtimeports.ProviderBindingRefV2{BindingSetID: "set-1", BindingSetRevision: 1, ComponentID: "custom/combined", ManifestDigest: digest("manifest"), ArtifactDigest: digest("artifact"), Capability: "praxis/harness-execution"}
	endpoint, err := contract.NewEndpointRefV2("endpoint-1", scope, harnessBinding)
	if err != nil {
		t.Fatal(err)
	}
	run := contract.RunRef{Scope: scope, RunID: "run-1"}
	session := contract.GovernedSessionV2{ContractVersion: contract.GovernedContractVersionV2, ID: "session-1", Revision: 1, Run: run, Endpoint: endpoint, Phase: contract.SessionCreatingV2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}
	payload := []byte(`{"input":1}`)
	schema := runtimeports.SchemaRefV2{Namespace: "custom", Name: "model-input", Version: "1.0.0", MediaType: "application/json", ContentDigest: digest("schema")}
	provider := harnessBinding
	provider.Capability = "praxis/model-turn"
	candidate := contract.ModelTurnCandidateV2{ContractVersion: contract.GovernedContractVersionV2, ID: "candidate-1", Revision: 1, Run: run, Endpoint: endpoint, SessionRef: session.ID, ExpectedSessionRevision: 1, Turn: 1, Kind: contract.CandidateInitialTurnV2, Input: runtimeports.OpaquePayloadV2{Schema: schema, ContentDigest: core.DigestBytes(payload), Length: uint64(len(payload)), Inline: payload, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "custom/default-limit", Digest: digest("limit")}}, ContextRef: "context-1", ContextDigest: digest("context"), Provider: provider, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	return session, candidate
}

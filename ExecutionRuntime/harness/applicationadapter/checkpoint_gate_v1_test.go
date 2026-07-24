package applicationadapter_test

import (
	"context"
	"testing"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/applicationadapter"
	harnesscontract "github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/reference"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCheckpointGateApplicationAdapterExactAcquireBindRelease(t *testing.T) {
	fixture := newCheckpointAdapterFixtureV1(t, "happy")
	acquired, err := fixture.adapter.AcquireCheckpointGateV1(context.Background(), fixture.request)
	if err != nil || acquired.State != appcontract.CheckpointGateAcquiredV1 || acquired.Gate.Owner != fixture.gateOwner || acquired.Snapshot.Owner != fixture.snapshotOwner {
		t.Fatalf("acquired=%#v err=%v", acquired, err)
	}
	bound, err := fixture.adapter.BindCheckpointGateRuntimeV1(context.Background(), appcontract.BindCheckpointGateRuntimeRequestV1{Gate: acquired, Attempt: fixture.runtime.Attempt, Barrier: fixture.runtime.Barrier, EffectCut: fixture.runtime.EffectCut})
	if err != nil || bound.State != appcontract.CheckpointGateBoundV1 || bound.RuntimeAttempt == nil || *bound.RuntimeAttempt != fixture.runtime.Attempt {
		t.Fatalf("bound=%#v err=%v", bound, err)
	}
	inspected, err := fixture.adapter.InspectCheckpointGateV1(context.Background(), bound.Gate)
	if err != nil || inspected.Digest != bound.Digest {
		t.Fatalf("exact Inspect=%#v err=%v", inspected, err)
	}
	released, err := fixture.adapter.ReleaseCheckpointGateV1(context.Background(), bound, fixture.runtime.Attempt)
	if err != nil || released.State != appcontract.CheckpointGateReleasedV1 || released.Gate.Revision != 3 {
		t.Fatalf("released=%#v err=%v", released, err)
	}
}

func TestCheckpointGateApplicationAdapterRejectsOwnerAndScopeDriftBeforeHarness(t *testing.T) {
	fixture := newCheckpointAdapterFixtureV1(t, "drift")
	ownerDrift := fixture.request
	ownerDrift.Subject.Owner.Capability = "praxis.harness/other"
	if _, err := fixture.adapter.AcquireCheckpointGateV1(context.Background(), ownerDrift); err == nil {
		t.Fatal("subject Owner drift was accepted")
	}
	if _, err := fixture.store.InspectCheckpointGateCurrentV1(context.Background(), harnesscontract.RunRef{Scope: fixture.request.Scope, RunID: fixture.request.RunID}); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("rejected Owner drift called Harness mutation: %v", err)
	}
	scopeDrift := fixture.request
	scopeDrift.Subject.ScopeDigest = core.DigestBytes([]byte("another-scope"))
	if _, err := fixture.adapter.AcquireCheckpointGateV1(context.Background(), scopeDrift); err == nil {
		t.Fatal("subject Scope drift was accepted")
	}
}

type checkpointAdapterFixtureV1 struct {
	adapter       *applicationadapter.CheckpointGateApplicationAdapterV1
	store         *reference.CheckpointGateStoreV1
	request       appcontract.AcquireCheckpointGateRequestV1
	runtime       harnesscontract.CheckpointRuntimeBindingV1
	gateOwner     runtimeports.ProviderBindingRefV2
	snapshotOwner runtimeports.ProviderBindingRefV2
}

func newCheckpointAdapterFixtureV1(t *testing.T, suffix string) checkpointAdapterFixtureV1 {
	t.Helper()
	now := time.Unix(1_900_100_000, 0)
	v2, _ := testkit.GovernedFactsV2(now)
	creating, err := harnesscontract.SealGovernedSessionV4(harnesscontract.GovernedSessionV4{ID: "adapter-session-" + suffix, Revision: 1, Run: v2.Run, Endpoint: v2.Endpoint, Phase: harnesscontract.SessionCreatingV2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	sessions := fakes.NewGovernedStoreV2()
	if _, err := sessions.CreateSessionV4(context.Background(), creating); err != nil {
		t.Fatal(err)
	}
	terminal := creating.Clone()
	terminal.Revision, terminal.Phase, terminal.CompletionClaim, terminal.UpdatedUnixNano = 2, harnesscontract.SessionTerminalV2, harnesscontract.ClaimCancelled, now.Add(time.Nanosecond).UnixNano()
	terminal, err = harnesscontract.SealGovernedSessionV4(terminal)
	if err != nil {
		t.Fatal(err)
	}
	cas, err := harnesscontract.SealSessionCASRequestV4(harnesscontract.SessionCASRequestV4{Run: creating.Run, SessionID: creating.ID, ExpectedRevision: creating.Revision, ExpectedDigest: creating.Digest, Next: terminal})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.CompareAndSwapSessionV4(context.Background(), cas); err != nil {
		t.Fatal(err)
	}
	attempt := runtimeports.CheckpointAttemptRefV2{TenantID: creating.Run.Scope.Identity.TenantID, ID: "adapter-attempt-" + suffix, Revision: 2, Digest: core.DigestBytes([]byte("adapter-attempt-" + suffix))}
	barrier := runtimeports.CheckpointBarrierLeaseRefV2{TenantID: attempt.TenantID, ID: "adapter-barrier-" + suffix, AttemptID: attempt.ID, Revision: 1, Digest: core.DigestBytes([]byte("adapter-barrier-" + suffix)), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	cutAttempt := attempt
	cutAttempt.Revision = 1
	cutAttempt.Digest = core.DigestBytes([]byte("adapter-attempt-before-cut-" + suffix))
	cut := runtimeports.EffectCutRefV2{ID: "adapter-cut-" + suffix, Revision: 1, Attempt: cutAttempt, RootDigest: core.DigestBytes([]byte("adapter-cut-root-" + suffix)), Watermark: 1, Digest: core.DigestBytes([]byte("adapter-cut-" + suffix))}
	consistency := runtimeports.CheckpointConsistencyRefV2{ID: "adapter-consistency-" + suffix, Revision: 1, Attempt: attempt, Digest: core.DigestBytes([]byte("adapter-consistency-" + suffix))}
	terminalCurrent, err := runtimeports.SealCheckpointAttemptTerminalCurrentProjectionV2(runtimeports.CheckpointAttemptTerminalCurrentProjectionV2{Attempt: attempt, Barrier: barrier, TerminalState: runtimeports.CheckpointAttemptConsistentV2, Consistency: &consistency, CheckedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	store := reference.NewCheckpointGateStoreV1()
	controller, err := kernel.NewCheckpointGateControllerV1(sessions, store, checkpointTerminalReaderV1{projection: terminalCurrent}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	subjectOwner := checkpointOwnerV1("praxis.harness/session-owner", "praxis.harness/session-current-v4")
	gateOwner := checkpointOwnerV1("praxis.harness/checkpoint-gate-owner", "praxis.harness/checkpoint-gate-v1")
	snapshotOwner := checkpointOwnerV1("praxis.harness/checkpoint-snapshot-owner", "praxis.harness/checkpoint-snapshot-v1")
	adapter, err := applicationadapter.NewCheckpointGateApplicationAdapterV1(applicationadapter.CheckpointGateApplicationAdapterConfigV1{Gates: controller, SubjectOwner: subjectOwner, GateOwner: gateOwner, SnapshotOwner: snapshotOwner, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(creating.Run.Scope)
	if err != nil {
		t.Fatal(err)
	}
	subject := appcontract.CheckpointExternalExactRefV1{ContractVersion: harnesscontract.GovernedContractVersionV4, ExactSchemaRef: "praxis.harness/governed-session-fact/v4", FactKind: "governed_session_fact_v4", Schema: checkpointSchemaV1("governed-session", "4.0.0"), Owner: subjectOwner, TenantID: attempt.TenantID, ScopeDigest: scopeDigest, RunID: creating.Run.RunID, ID: terminal.ID, Revision: terminal.Revision, Digest: terminal.Digest}
	request := appcontract.AcquireCheckpointGateRequestV1{StableID: "adapter-gate-" + suffix, IntentDigest: core.DigestBytes([]byte("adapter-intent-" + suffix)), Scope: creating.Run.Scope, RunID: creating.Run.RunID, Subject: subject, RequestedNotAfter: now.Add(time.Minute).UnixNano()}
	return checkpointAdapterFixtureV1{adapter: adapter, store: store, request: request, runtime: harnesscontract.CheckpointRuntimeBindingV1{Attempt: attempt, Barrier: barrier, EffectCut: cut}, gateOwner: gateOwner, snapshotOwner: snapshotOwner}
}

type checkpointTerminalReaderV1 struct {
	projection runtimeports.CheckpointAttemptTerminalCurrentProjectionV2
}

func (r checkpointTerminalReaderV1) InspectCheckpointAttemptTerminalCurrentV2(context.Context, runtimeports.CheckpointAttemptRefV2) (runtimeports.CheckpointAttemptTerminalCurrentProjectionV2, error) {
	return r.projection, nil
}

func checkpointOwnerV1(component runtimeports.ComponentIDV2, capability runtimeports.CapabilityNameV2) runtimeports.ProviderBindingRefV2 {
	return runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-checkpoint-adapter", BindingSetRevision: 1, ComponentID: component, ManifestDigest: core.DigestBytes([]byte("manifest-" + string(component))), ArtifactDigest: core.DigestBytes([]byte("artifact-" + string(component))), Capability: capability}
}

func checkpointSchemaV1(name, version string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.harness", Name: name, Version: version, MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("praxis.harness.schema/" + name + "/" + version))}
}

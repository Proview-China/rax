package checkpointintegration_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	continuityadapter "github.com/Proview-China/rax/ExecutionRuntime/continuity/applicationadapter"
	continuitycontract "github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	continuitydomain "github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	continuityfakes "github.com/Proview-China/rax/ExecutionRuntime/continuity/fakes"
	continuityruntime "github.com/Proview-China/rax/ExecutionRuntime/continuity/runtimeadapter"
	continuitymemory "github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
	harnessadapter "github.com/Proview-China/rax/ExecutionRuntime/harness/applicationadapter"
	harnesscontract "github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessfakes "github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	harnesskernel "github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	harnessreference "github.com/Proview-China/rax/ExecutionRuntime/harness/reference"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimekernel "github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	sandboxadapter "github.com/Proview-China/rax/ExecutionRuntime/sandbox/applicationadapter"
)

func TestCheckpointFirstV1ClosesTwoOwnersThroughContinuityAndRuntime(t *testing.T) {
	fixture := newCheckpointFirstFixtureV1(t, "happy")
	result, err := fixture.coordinator.RunCheckpointV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Gate.State != appcontract.CheckpointGateReleasedV1 || len(result.Participants) != 2 {
		t.Fatalf("checkpoint vertical slice did not release two-owner Gate: %+v", result)
	}
	terminal, err := fixture.runtime.InspectCheckpointAttemptTerminalCurrentV2(context.Background(), result.TerminalAttempt)
	if err != nil || terminal.TerminalState != runtimeports.CheckpointAttemptConsistentV2 || terminal.Consistency == nil || *terminal.Consistency != result.Consistency {
		t.Fatalf("Runtime did not own exact terminal Consistency: %+v err=%v", terminal, err)
	}
	seal, err := fixture.manifests.InspectCheckpointManifestSealV1(context.Background(), appcontract.InspectCheckpointManifestSealRequestV1{Ref: result.ManifestSeal})
	if err != nil || seal != result.ManifestSeal {
		t.Fatalf("Continuity did not retain exact immutable Seal: %+v err=%v", seal, err)
	}
	gateCurrent, err := fixture.gateStore.InspectCheckpointGateCurrentV1(context.Background(), harnesscontract.RunRef{Scope: fixture.request.Gate.Scope, RunID: fixture.request.Gate.RunID})
	if err != nil || gateCurrent.State != harnesscontract.CheckpointGateReleasedV1 || gateCurrent.Ref.Revision != 3 {
		t.Fatalf("Harness did not retain released Gate current: %+v err=%v", gateCurrent, err)
	}
}

// TestCheckpointFirstV1LostParticipantReplyInspectsOriginalAttempt proves the
// only allowed Unknown recovery: inspect the original Participant attempt.
func TestCheckpointFirstV1LostParticipantReplyInspectsOriginalAttempt(t *testing.T) {
	fixture := newCheckpointFirstFixtureV1(t, "lost")
	fixture.phases.loseReplyFor = "praxis/sandbox"
	result, err := fixture.coordinator.RunCheckpointV1(context.Background(), fixture.request)
	if err != nil || result.Gate.State != appcontract.CheckpointGateReleasedV1 {
		t.Fatalf("lost Participant reply was not recovered by exact Inspect: %+v err=%v", result, err)
	}
	if fixture.phases.inspectCalls["praxis/sandbox"] == 0 || fixture.phases.commitCalls["praxis/sandbox"] != 1 {
		t.Fatalf("Unknown recovery retried execution: commits=%v inspects=%v", fixture.phases.commitCalls, fixture.phases.inspectCalls)
	}
}

func TestCheckpointFirstV1LostRuntimeAndManifestRepliesConvergeByInspect(t *testing.T) {
	t.Run("runtime_create", func(t *testing.T) {
		fixture := newCheckpointFirstFixtureV1(t, "lost-runtime-create")
		fixture.runtimeStore.LoseNextCheckpointReplyV2()
		result, err := fixture.coordinator.RunCheckpointV1(context.Background(), fixture.request)
		if err != nil || result.Gate.State != appcontract.CheckpointGateReleasedV1 {
			t.Fatalf("lost Runtime create reply did not converge: %+v err=%v", result, err)
		}
	})
	t.Run("runtime_consistency", func(t *testing.T) {
		fixture := newCheckpointFirstFixtureV1(t, "lost-runtime-consistency")
		fixture.phases.loseRuntimeConsistencyReply = true
		result, err := fixture.coordinator.RunCheckpointV1(context.Background(), fixture.request)
		if err != nil || result.Gate.State != appcontract.CheckpointGateReleasedV1 {
			t.Fatalf("lost Runtime Consistency reply did not converge: %+v err=%v", result, err)
		}
	})
	for _, operation := range []continuityfakes.CheckpointMutationV2{continuityfakes.CheckpointCreateManifestV2, continuityfakes.CheckpointCASManifestV2, continuityfakes.CheckpointCreateSealV2} {
		t.Run(string(operation), func(t *testing.T) {
			fixture := newCheckpointFirstFixtureV1(t, "lost-"+string(operation))
			fixture.manifestFaults.LoseNextSuccessfulReply(operation, errors.New("injected Manifest reply loss"))
			result, err := fixture.coordinator.RunCheckpointV1(context.Background(), fixture.request)
			if err != nil || result.Gate.State != appcontract.CheckpointGateReleasedV1 {
				t.Fatalf("lost Continuity %s reply did not converge: %+v err=%v", operation, result, err)
			}
		})
	}
}

func TestCheckpointFirstV1PartialParticipantCannotCreateSealOrConsistency(t *testing.T) {
	fixture := newCheckpointFirstFixtureV1(t, "partial")
	fixture.phases.failBeforeCommitFor = "praxis/sandbox"
	if _, err := fixture.coordinator.RunCheckpointV1(context.Background(), fixture.request); err == nil {
		t.Fatal("partial Participant set produced a successful checkpoint")
	}
	gate, err := fixture.gateStore.InspectCheckpointGateCurrentV1(context.Background(), harnesscontract.RunRef{Scope: fixture.request.Gate.Scope, RunID: fixture.request.Gate.RunID})
	if err != nil || gate.State != harnesscontract.CheckpointGateBoundV1 {
		t.Fatalf("partial checkpoint did not remain fenced for diagnosis: %+v err=%v", gate, err)
	}
	bundle, err := fixture.runtime.InspectCheckpointAttemptV2(context.Background(), runtimeports.InspectCheckpointAttemptRequestV2{TenantID: fixture.request.RuntimeCreate.Scope.Identity.TenantID, AttemptID: fixture.request.RuntimeCreate.AttemptID})
	if err != nil || bundle.Attempt.State == runtimeports.CheckpointAttemptConsistentV2 || bundle.Attempt.Consistency != nil {
		t.Fatalf("partial checkpoint mutated Runtime Consistency: %+v err=%v", bundle, err)
	}
}

type checkpointFirstFixtureV1 struct {
	coordinator    *application.CheckpointCoordinatorV1
	request        appcontract.StartCheckpointCoordinationRequestV1
	runtime        runtimekernel.CheckpointGovernanceGatewayV2
	manifests      *continuityadapter.CheckpointManifestApplicationAdapterV1
	gateStore      *harnessreference.CheckpointGateStoreV1
	phases         *checkpointPhaseBridgeV1
	runtimeStore   *runtimefakes.CheckpointStoreV2
	manifestFaults *continuityfakes.CheckpointManifestGovernanceV2
}

func newCheckpointFirstFixtureV1(t *testing.T, suffix string) checkpointFirstFixtureV1 {
	t.Helper()
	now := time.Unix(1_900_200_000, 0).UTC()
	clock := func() time.Time { return now }
	v2, _ := testkit.GovernedFactsV2(now)
	creating, err := harnesscontract.SealGovernedSessionV4(harnesscontract.GovernedSessionV4{ID: "session-" + suffix, Revision: 1, Run: v2.Run, Endpoint: v2.Endpoint, Phase: harnesscontract.SessionCreatingV2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	sessions := harnessfakes.NewGovernedStoreV2()
	if _, err = sessions.CreateSessionV4(context.Background(), creating); err != nil {
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
	if _, err = sessions.CompareAndSwapSessionV4(context.Background(), cas); err != nil {
		t.Fatal(err)
	}

	scope := terminal.Run.Scope
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	runIdentity := digestV1("run-identity-" + suffix)
	create := checkpointCreateRequestV1(scope, scopeDigest, terminal.Run.RunID, runIdentity, suffix, now)
	harnessOwner := ownerV1("praxis/harness", "praxis.harness/checkpoint-participant-v1")
	sandboxOwner := ownerV1("praxis/sandbox", "praxis.sandbox/checkpoint-participant-v1")
	participants := []runtimeports.CheckpointParticipantRefV2{
		{ID: "participant-harness-" + suffix, Owner: harnessOwner, Digest: digestV1("participant-harness-" + suffix)},
		{ID: "participant-sandbox-" + suffix, Owner: sandboxOwner, Digest: digestV1("participant-sandbox-" + suffix)},
	}
	sort.Slice(participants, func(i, j int) bool { return participants[i].ID < participants[j].ID })

	owners := &checkpointRuntimeCurrentsV1{now: now, create: create, effectRoot: digestV1("effect-root-" + suffix), participantRoot: create.ParticipantSetCertification.Digest, participants: participants, closures: map[string]runtimeports.CheckpointParticipantClosureRefV2{}, guards: map[string]runtimeports.CheckpointParticipantBranchGuardRefV2{}}
	policy := checkpointPolicyV1(create.BarrierPolicy, now)
	runtimeStore := runtimefakes.NewCheckpointStoreV2()
	branchStore := runtimefakes.NewCheckpointParticipantBranchStoreV2()
	continuityBackend := continuitymemory.NewWithClock(clock)
	manifestController, err := continuitydomain.NewCheckpointManifestControllerV2(continuityBackend)
	if err != nil {
		t.Fatal(err)
	}
	manifestOwner := continuityOwnerV1("checkpoint_manifest_fact_v2")
	sealOwner := continuityOwnerV1("checkpoint_manifest_seal_fact_v2")
	runtimeOwner := continuitycontract.OwnerBinding{BindingSetID: "binding-set-runtime", BindingRevision: 1, ComponentID: "praxis/runtime", ManifestDigest: string(digestV1("runtime-manifest")), ArtifactDigest: string(digestV1("runtime-artifact")), Capability: "checkpoint-governance-v2", FactKind: "checkpoint_attempt_fact_v2"}
	manifestFaults, err := continuityfakes.NewCheckpointManifestGovernanceV2(manifestController)
	if err != nil {
		t.Fatal(err)
	}
	manifests, err := continuityadapter.NewCheckpointManifestApplicationAdapterV1(continuityadapter.CheckpointManifestApplicationAdapterConfigV1{Manifests: manifestFaults, ManifestOwner: manifestOwner, SealOwner: sealOwner, RuntimeOwner: runtimeOwner, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	runtimeGateway := runtimekernel.CheckpointGovernanceGatewayV2{Facts: runtimeStore, Policies: checkpointPolicyReaderV1{projection: policy}, Runs: owners, Inputs: owners, Effects: owners, Participants: owners, Closures: owners, Branches: branchStore, Manifests: continuityruntime.CheckpointManifestSealReaderV2{Manifests: manifestFaults}, Diagnostics: checkpointFinalizationOwnersV1{}, Residuals: checkpointFinalizationOwnersV1{}, Clock: clock}

	gateStore := harnessreference.NewCheckpointGateStoreV1()
	gateController, err := harnesskernel.NewCheckpointGateControllerV1(sessions, gateStore, runtimeGateway, clock)
	if err != nil {
		t.Fatal(err)
	}
	subjectOwner := ownerV1("praxis/harness", "praxis.harness/session-current-v4")
	gateOwner := ownerV1("praxis/harness", "praxis.harness/checkpoint-gate-v1")
	snapshotOwner := ownerV1("praxis/harness", "praxis.harness/checkpoint-snapshot-v1")
	gates, err := harnessadapter.NewCheckpointGateApplicationAdapterV1(harnessadapter.CheckpointGateApplicationAdapterConfigV1{Gates: gateController, SubjectOwner: subjectOwner, GateOwner: gateOwner, SnapshotOwner: snapshotOwner, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	subject := exactRefV1(scope.Identity.TenantID, scopeDigest, terminal.Run.RunID, terminal.ID, terminal.Revision, terminal.Digest, "praxis.harness/governed-session-fact/v4", "governed_session_fact_v4", subjectOwner)
	subject.ContractVersion = harnesscontract.GovernedContractVersionV4
	subject.Schema = runtimeports.SchemaRefV2{Namespace: "praxis.harness", Name: "governed-session", Version: "4.0.0", MediaType: "application/json", ContentDigest: digestV1("praxis.harness.schema/governed-session/4.0.0")}
	gateRequest := appcontract.AcquireCheckpointGateRequestV1{StableID: "gate-" + suffix, IntentDigest: digestV1("gate-intent-" + suffix), Scope: scope, RunID: terminal.Run.RunID, Subject: subject, RequestedNotAfter: now.Add(8 * time.Minute).UnixNano()}

	phases := &checkpointPhaseBridgeV1{now: now, scope: scope, runID: terminal.Run.RunID, branches: branchStore, owners: owners, runtimeStore: runtimeStore, commits: map[string]appcontract.CheckpointParticipantCommitV1{}, commitCalls: map[string]int{}, inspectCalls: map[string]int{}}
	harnessCurrent := checkpointOwnerCurrentReaderV1{now: now, owner: harnessOwner, factKind: "harness_checkpoint_participant_fact_v1", exactSnapshot: true}
	sandboxCurrent := checkpointOwnerCurrentReaderV1{now: now, owner: sandboxOwner, factKind: "sandbox_checkpoint_participant_fact_v1"}
	harnessParticipant, err := harnessadapter.NewCheckpointParticipantApplicationAdapterV1(harnessadapter.CheckpointParticipantApplicationAdapterConfigV1{ParticipantID: participants[0].ID, Current: harnessCurrent, Phases: phases, Clock: clock})
	if participants[0].Owner.ComponentID != "praxis/harness" {
		harnessParticipant, err = harnessadapter.NewCheckpointParticipantApplicationAdapterV1(harnessadapter.CheckpointParticipantApplicationAdapterConfigV1{ParticipantID: participants[1].ID, Current: harnessCurrent, Phases: phases, Clock: clock})
	}
	if err != nil {
		t.Fatal(err)
	}
	sandboxID := participants[0].ID
	if participants[0].Owner.ComponentID != "praxis/sandbox" {
		sandboxID = participants[1].ID
	}
	sandboxParticipant, err := sandboxadapter.NewCheckpointParticipantApplicationAdapterV1(sandboxadapter.CheckpointParticipantApplicationAdapterConfigV1{ParticipantID: sandboxID, Current: sandboxCurrent, Phases: phases, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	harnessID := participants[0].ID
	if participants[0].Owner.ComponentID != "praxis/harness" {
		harnessID = participants[1].ID
	}
	coordinator, err := application.NewCheckpointCoordinatorV1(application.CheckpointCoordinatorConfigV1{Gates: gates, Runtime: runtimeGateway, Effects: owners, ParticipantSet: owners, Closures: owners, Inputs: checkpointManifestInputsV1{now: now, tenantID: scope.Identity.TenantID, scopeDigest: scopeDigest, runID: terminal.Run.RunID}, Participants: map[string]applicationports.CheckpointParticipantDriverV1{harnessID: harnessParticipant, sandboxID: sandboxParticipant}, Manifests: manifests, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	request := appcontract.StartCheckpointCoordinationRequestV1{StableID: "checkpoint-" + suffix, Gate: gateRequest, RuntimeCreate: create, CutID: "cut-" + suffix, ManifestID: "manifest-" + suffix, ManifestSealID: "seal-" + suffix, NotAfter: now.Add(8 * time.Minute).UnixNano()}
	return checkpointFirstFixtureV1{coordinator: coordinator, request: request, runtime: runtimeGateway, manifests: manifests, gateStore: gateStore, phases: phases, runtimeStore: runtimeStore, manifestFaults: manifestFaults}
}

type checkpointRuntimeCurrentsV1 struct {
	mu              sync.Mutex
	now             time.Time
	create          runtimeports.CreateCheckpointAttemptRequestV2
	effectRoot      core.Digest
	participantRoot core.Digest
	participants    []runtimeports.CheckpointParticipantRefV2
	closures        map[string]runtimeports.CheckpointParticipantClosureRefV2
	guards          map[string]runtimeports.CheckpointParticipantBranchGuardRefV2
}

func (o *checkpointRuntimeCurrentsV1) InspectCheckpointRunCurrentV2(_ context.Context, scope core.ExecutionScope, runID core.AgentRunID) (runtimeports.CheckpointRunCurrentProjectionV2, error) {
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		return runtimeports.CheckpointRunCurrentProjectionV2{}, err
	}
	return runtimeports.SealCheckpointRunCurrentProjectionV2(runtimeports.CheckpointRunCurrentProjectionV2{RunID: runID, Revision: o.create.ExpectedRunRevision, Status: core.RunRunning, RunStableIdentityDigest: o.create.RunStableIdentityDigest, ExecutionScopeDigest: scopeDigest, CheckedUnixNano: o.now.UnixNano(), ExpiresUnixNano: o.now.Add(9 * time.Minute).UnixNano()}, o.now)
}

func (o *checkpointRuntimeCurrentsV1) InspectCheckpointAttemptInputsCurrentV2(_ context.Context, attempt runtimeports.CheckpointAttemptRefV2) (runtimeports.CheckpointAttemptInputsCurrentProjectionV2, error) {
	expires := o.now.Add(9 * time.Minute).UnixNano()
	current := func(kind, id string) runtimeports.CheckpointCurrentInputRefV2 {
		return runtimeports.CheckpointCurrentInputRefV2{Kind: runtimeports.NamespacedNameV2(kind), ID: id, Revision: 1, Digest: digestV1(kind + ":" + id), CheckedUnixNano: o.now.UnixNano(), ExpiresUnixNano: expires}
	}
	authority := runtimeports.AuthorityBindingRefV2{Ref: "authority:" + attempt.ID, Revision: 1, Epoch: 1, Digest: digestV1("authority-" + attempt.ID)}
	return runtimeports.SealCheckpointAttemptInputsCurrentProjectionV2(runtimeports.CheckpointAttemptInputsCurrentProjectionV2{
		AttemptID: attempt.ID, TenantID: attempt.TenantID,
		Run: current("praxis.runtime/run-current", string(o.create.RunID)), RunID: o.create.RunID, RunStableIdentityDigest: o.create.RunStableIdentityDigest,
		Generation: current("praxis.runtime/generation-current", o.create.Generation.ID), GenerationArtifact: o.create.Generation, GenerationBinding: o.create.GenerationBinding,
		Binding: current("praxis.runtime/binding-current", o.create.BindingSet.ID), BindingSet: o.create.BindingSet,
		ParticipantCertification: current("praxis.runtime/participant-current", o.create.ParticipantSetCertification.ID), ParticipantSetCertification: o.create.ParticipantSetCertification,
		WorkflowCurrent: current("praxis.runtime/workflow-current", o.create.Workflow.ID), Workflow: o.create.Workflow,
		Authority: current("praxis.runtime/authority-current", authority.Ref), AuthorityRef: authority,
		CheckedUnixNano: o.now.UnixNano(), ExpiresUnixNano: expires,
	}, o.now)
}

func (o *checkpointRuntimeCurrentsV1) InspectCheckpointEffectInventoryCurrentV2(_ context.Context, attempt runtimeports.CheckpointAttemptRefV2, barrier runtimeports.CheckpointBarrierLeaseRefV2) (runtimeports.CheckpointEffectInventoryCurrentProjectionV2, error) {
	return runtimeports.SealCheckpointEffectInventoryCurrentProjectionV2(runtimeports.CheckpointEffectInventoryCurrentProjectionV2{Attempt: attempt, Barrier: barrier, RootDigest: o.effectRoot, Watermark: 1, Entries: []runtimeports.EffectCutEntryV2{}, CheckedUnixNano: o.now.UnixNano(), ExpiresUnixNano: o.now.Add(8 * time.Minute).UnixNano()}, o.now)
}

func (o *checkpointRuntimeCurrentsV1) InspectCheckpointParticipantSetCurrentV2(_ context.Context, attempt runtimeports.CheckpointAttemptRefV2, certification runtimeports.CheckpointParticipantSetCertificationRefV2) (runtimeports.CheckpointParticipantSetCurrentProjectionV2, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	projection := runtimeports.CheckpointParticipantSetCurrentProjectionV2{ContractVersion: runtimeports.CheckpointGovernanceContractVersionV2, Attempt: attempt, Certification: certification, RootDigest: o.participantRoot, Watermark: 1, Participants: append([]runtimeports.CheckpointParticipantRefV2(nil), o.participants...), CheckedUnixNano: o.now.UnixNano(), ExpiresUnixNano: o.now.Add(8 * time.Minute).UnixNano()}
	digest, err := projection.DigestV2()
	if err != nil {
		return runtimeports.CheckpointParticipantSetCurrentProjectionV2{}, err
	}
	projection.ProjectionDigest = digest
	return projection, projection.Validate(o.now)
}

func (o *checkpointRuntimeCurrentsV1) InspectCheckpointParticipantClosureCurrentV2(_ context.Context, attempt runtimeports.CheckpointAttemptRefV2, participant runtimeports.CheckpointParticipantRefV2) (runtimeports.CheckpointParticipantClosureCurrentProjectionV2, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	closure, ok := o.closures[participant.ID]
	if !ok {
		return runtimeports.CheckpointParticipantClosureCurrentProjectionV2{}, core.NewError(core.ErrorNotFound, core.ReasonCheckpointInconsistent, "checkpoint Participant closure not found")
	}
	projection := runtimeports.CheckpointParticipantClosureCurrentProjectionV2{ContractVersion: runtimeports.CheckpointGovernanceContractVersionV2, Attempt: attempt, Participant: participant, Closure: closure, BranchGuard: o.guards[participant.ID], CheckedUnixNano: o.now.UnixNano(), ExpiresUnixNano: o.now.Add(8 * time.Minute).UnixNano()}
	digest, err := projection.DigestV2()
	if err != nil {
		return runtimeports.CheckpointParticipantClosureCurrentProjectionV2{}, err
	}
	projection.ProjectionDigest = digest
	return projection, projection.Validate(o.now)
}

type checkpointPolicyReaderV1 struct {
	projection runtimeports.CheckpointBarrierPolicyCurrentProjectionV2
}

func (r checkpointPolicyReaderV1) InspectCheckpointBarrierPolicyCurrentV2(context.Context, runtimeports.CheckpointBarrierPolicyRefV2) (runtimeports.CheckpointBarrierPolicyCurrentProjectionV2, error) {
	return r.projection, nil
}

type checkpointOwnerCurrentReaderV1 struct {
	now           time.Time
	owner         runtimeports.ProviderBindingRefV2
	factKind      string
	exactSnapshot bool
}

func (r checkpointOwnerCurrentReaderV1) InspectCheckpointParticipantOwnerCurrentV1(_ context.Context, work appcontract.CheckpointParticipantWorkRequestV1) (appcontract.CheckpointParticipantOwnerCandidateV1, error) {
	fact := exactRefV1(work.Gate.TenantID, work.Gate.ScopeDigest, work.Gate.RunID, "fact-"+work.Participant.ID, 1, digestV1("fact-"+work.Participant.ID), "praxis.checkpoint/participant-fact/v1", r.factKind, r.owner)
	snapshot := exactRefV1(work.Gate.TenantID, work.Gate.ScopeDigest, work.Gate.RunID, "snapshot-"+work.Participant.ID, 1, digestV1("snapshot-"+work.Participant.ID), "praxis.checkpoint/participant-snapshot/v1", r.factKind+"_snapshot", r.owner)
	if r.exactSnapshot {
		snapshot = work.Snapshot
	}
	coverage := exactRefV1(work.Gate.TenantID, work.Gate.ScopeDigest, work.Gate.RunID, "coverage-"+work.Participant.ID, 1, digestV1("coverage-"+work.Participant.ID), "praxis.checkpoint/participant-coverage/v1", r.factKind+"_coverage", r.owner)
	candidate := appcontract.CheckpointParticipantOwnerCandidateV1{Participant: work.Participant, ParticipantFact: fact, Snapshot: snapshot, Coverage: coverage, CheckedUnixNano: r.now.UnixNano(), ExpiresUnixNano: work.NotAfter}
	return appcontract.SealCheckpointParticipantOwnerCandidateV1(candidate, work, r.now)
}

type checkpointPhaseBridgeV1 struct {
	mu                          sync.Mutex
	now                         time.Time
	scope                       core.ExecutionScope
	runID                       core.AgentRunID
	branches                    *runtimefakes.CheckpointParticipantBranchStoreV2
	owners                      *checkpointRuntimeCurrentsV1
	runtimeStore                *runtimefakes.CheckpointStoreV2
	commits                     map[string]appcontract.CheckpointParticipantCommitV1
	commitCalls                 map[string]int
	inspectCalls                map[string]int
	loseReplyFor                string
	failBeforeCommitFor         string
	loseRuntimeConsistencyReply bool
}

func (b *checkpointPhaseBridgeV1) CommitCheckpointParticipantPhaseV1(ctx context.Context, work appcontract.CheckpointParticipantWorkRequestV1, candidate appcontract.CheckpointParticipantOwnerCandidateV1) (appcontract.CheckpointParticipantCommitV1, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	component := string(work.Participant.Owner.ComponentID)
	b.commitCalls[component]++
	if b.failBeforeCommitFor == component {
		return appcontract.CheckpointParticipantCommitV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "injected Participant pre-commit failure")
	}
	if existing, ok := b.commits[work.Participant.ID]; ok {
		return existing.Clone(), nil
	}
	closure, _, err := runtimefakes.BuildCommittedCheckpointParticipantClosureV2(b.scope, b.runID, work.Attempt, work.Barrier, work.EffectCut, work.Participant, work.Participant.ID, b.now)
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	branch, err := b.branches.SelectCheckpointParticipantBranchV2(ctx, runtimeports.SelectCheckpointParticipantBranchRequestV2{Attempt: work.Attempt, Participant: work.Participant, Terminal: *closure.Terminal, SelectedAt: b.now.UnixNano()})
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	evidence := exactRefV1(work.Gate.TenantID, work.Gate.ScopeDigest, work.Gate.RunID, "evidence-"+work.Participant.ID, 1, digestV1("evidence-"+work.Participant.ID), "praxis.runtime/evidence-record/v3", "evidence_record_v3", work.Participant.Owner)
	commit := appcontract.CheckpointParticipantCommitV1{RuntimeClosure: closure, ParticipantFact: candidate.ParticipantFact, Snapshot: candidate.Snapshot, Coverage: candidate.Coverage, Evidence: []appcontract.CheckpointExternalExactRefV1{evidence}, Residuals: []appcontract.CheckpointExternalExactRefV1{}}
	if err := commit.Validate(work.Participant); err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	b.owners.mu.Lock()
	b.owners.closures[work.Participant.ID] = closure
	b.owners.guards[work.Participant.ID] = branch.Ref
	b.owners.mu.Unlock()
	b.commits[work.Participant.ID] = commit.Clone()
	if b.loseRuntimeConsistencyReply && len(b.commits) == len(b.owners.participants) {
		b.runtimeStore.LoseNextCheckpointReplyV2()
		b.loseRuntimeConsistencyReply = false
	}
	if b.loseReplyFor == component {
		return appcontract.CheckpointParticipantCommitV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "injected Participant reply loss")
	}
	return commit.Clone(), nil
}

func (b *checkpointPhaseBridgeV1) InspectCheckpointParticipantPhaseV1(_ context.Context, attempt runtimeports.CheckpointAttemptRefV2, participant runtimeports.CheckpointParticipantRefV2) (appcontract.CheckpointParticipantCommitV1, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.inspectCalls[string(participant.Owner.ComponentID)]++
	commit, ok := b.commits[participant.ID]
	if !ok || commit.RuntimeClosure.Participant != participant || commit.RuntimeClosure.Prepare.DomainResult.Attempt.ID != attempt.ID {
		return appcontract.CheckpointParticipantCommitV1{}, core.NewError(core.ErrorNotFound, core.ReasonCheckpointInconsistent, "original Participant attempt not found")
	}
	return commit.Clone(), nil
}

type checkpointManifestInputsV1 struct {
	now         time.Time
	tenantID    core.TenantID
	scopeDigest core.Digest
	runID       core.AgentRunID
}

func (r checkpointManifestInputsV1) InspectCheckpointManifestInputCurrentV1(_ context.Context, attempt runtimeports.CheckpointAttemptRefV2, _ runtimeports.CheckpointBarrierLeaseRefV2, _ runtimeports.EffectCutRefV2) (appcontract.CheckpointManifestInputCurrentProjectionV1, error) {
	owner := ownerV1("praxis/continuity", "praxis.continuity/timeline-current-v1")
	ref := func(id, schema, kind string) appcontract.CheckpointExternalExactRefV1 {
		return exactRefV1(r.tenantID, r.scopeDigest, r.runID, id, 1, digestV1(id), schema, kind, owner)
	}
	projection := appcontract.CheckpointManifestInputCurrentProjectionV1{
		Attempt:            attempt,
		Timeline:           appcontract.CheckpointTimelineCutV1{LedgerScopeDigest: digestV1("timeline-scope"), LedgerSequence: 1, EvidenceRecord: ref("timeline-evidence", "praxis.runtime/evidence-record/v3", "evidence_record_v3")},
		ContextGeneration:  ref("context-generation", "praxis.context/generation-fact/v1", "context_generation_fact_v1"),
		ContextFrames:      []appcontract.CheckpointExternalExactRefV1{ref("context-frame", "praxis.context/frame-fact/v1", "context_frame_fact_v1")},
		AttemptSettlements: []appcontract.CheckpointAttemptSettlementClosureV1{},
		Memory:             []appcontract.CheckpointExternalExactRefV1{ref("memory-snapshot", "praxis.memory/snapshot-fact/v1", "memory_snapshot_fact_v1")},
		Knowledge:          []appcontract.CheckpointExternalExactRefV1{ref("knowledge-snapshot", "praxis.knowledge/snapshot-fact/v1", "knowledge_snapshot_fact_v1")},
		CheckedUnixNano:    r.now.UnixNano(), ExpiresUnixNano: r.now.Add(8 * time.Minute).UnixNano(),
	}
	return appcontract.SealCheckpointManifestInputCurrentProjectionV1(projection, r.now)
}

type checkpointFinalizationOwnersV1 struct{}

func (checkpointFinalizationOwnersV1) SealCheckpointDiagnosticsForFinalizationV2(context.Context, runtimeports.CheckpointAttemptRefV2, runtimeports.EffectCutRefV2, runtimeports.CheckpointFinalizationCutRefV2) (runtimeports.CheckpointDiagnosticsFinalizationSealRefV2, error) {
	return runtimeports.CheckpointDiagnosticsFinalizationSealRefV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "consistent slice does not finalize diagnostics")
}
func (checkpointFinalizationOwnersV1) InspectCheckpointDiagnosticsFinalizationSealCurrentV2(context.Context, runtimeports.CheckpointDiagnosticsFinalizationSealRefV2) (runtimeports.CheckpointDiagnosticsFinalizationSealProjectionV2, error) {
	return runtimeports.CheckpointDiagnosticsFinalizationSealProjectionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "consistent slice does not inspect diagnostics")
}
func (checkpointFinalizationOwnersV1) SealCheckpointResidualsForFinalizationV2(context.Context, runtimeports.CheckpointAttemptRefV2, runtimeports.EffectCutRefV2, runtimeports.CheckpointFinalizationCutRefV2) (runtimeports.CheckpointResidualsFinalizationSealRefV2, error) {
	return runtimeports.CheckpointResidualsFinalizationSealRefV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "consistent slice does not finalize residuals")
}
func (checkpointFinalizationOwnersV1) InspectCheckpointResidualsFinalizationSealCurrentV2(context.Context, runtimeports.CheckpointResidualsFinalizationSealRefV2) (runtimeports.CheckpointResidualsFinalizationSealProjectionV2, error) {
	return runtimeports.CheckpointResidualsFinalizationSealProjectionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "consistent slice does not inspect residuals")
}

func checkpointCreateRequestV1(scope core.ExecutionScope, scopeDigest core.Digest, runID core.AgentRunID, runIdentity core.Digest, suffix string, now time.Time) runtimeports.CreateCheckpointAttemptRequestV2 {
	policy := runtimeports.CheckpointBarrierPolicyRefV2{ID: "policy-" + suffix, Revision: 1, Digest: digestV1("policy-" + suffix), SemanticDigest: digestV1("policy-semantic-" + suffix)}
	return runtimeports.CreateCheckpointAttemptRequestV2{
		AttemptID: "attempt-" + suffix, BarrierID: "barrier-" + suffix, IdempotencyKey: "create-" + suffix,
		Scope: scope, ScopeDigest: scopeDigest, RunID: runID, RunStableIdentityDigest: runIdentity,
		Generation:                  runtimeports.GenerationArtifactRefV1{ID: "generation-" + suffix, Revision: 1, Digest: digestV1("generation"), InputDigest: digestV1("input"), ManifestDigest: digestV1("manifest"), GraphDigest: digestV1("graph"), CatalogDigest: digestV1("catalog")},
		GenerationBinding:           runtimeports.GenerationBindingAssociationRefV1{ID: "generation-binding-" + suffix, Revision: 1, Digest: digestV1("generation-binding")},
		BindingSet:                  runtimeports.RunBindingSetRefV2{ID: "binding-set-" + suffix, Revision: 1, Digest: digestV1("binding-set"), SemanticDigest: digestV1("binding-set-semantic")},
		ParticipantSetCertification: runtimeports.CheckpointParticipantSetCertificationRefV2{ID: "participant-set-" + suffix, Revision: 1, Digest: digestV1("participant-set-" + suffix)},
		Workflow:                    runtimeports.CheckpointWorkflowRefV2{ID: "workflow-" + suffix, Revision: 1, Digest: digestV1("workflow"), NotAfter: now.Add(9 * time.Minute).UnixNano()},
		BarrierPolicy:               policy, ExpectedRunRevision: 1, AcquiredDispatchWatermark: 1,
	}
}

func checkpointPolicyV1(ref runtimeports.CheckpointBarrierPolicyRefV2, now time.Time) runtimeports.CheckpointBarrierPolicyCurrentProjectionV2 {
	projection, err := runtimeports.SealCheckpointBarrierPolicyCurrentProjectionV2(runtimeports.CheckpointBarrierPolicyCurrentProjectionV2{Ref: ref, MaxBarrierTTLUnixNano: int64(9 * time.Minute), MaxReconciliationTTLUnixNano: int64(5 * time.Minute), UnknownAtDeadlineMode: runtimeports.CheckpointUnknownAtDeadlineIndeterminateV2, AbsoluteNotAfterUnixNano: now.Add(10 * time.Minute).UnixNano(), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(9 * time.Minute).UnixNano()}, now)
	if err != nil {
		panic(err)
	}
	return projection
}

func ownerV1(component runtimeports.ComponentIDV2, capability runtimeports.CapabilityNameV2) runtimeports.ProviderBindingRefV2 {
	return runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-" + string(component), BindingSetRevision: 1, ComponentID: component, ManifestDigest: digestV1("manifest-" + string(component)), ArtifactDigest: digestV1("artifact-" + string(component)), Capability: capability}
}

func exactRefV1(tenantID core.TenantID, scope core.Digest, runID core.AgentRunID, id string, revision core.Revision, digest core.Digest, exactSchema, factKind string, owner runtimeports.ProviderBindingRefV2) appcontract.CheckpointExternalExactRefV1 {
	name := "checkpoint-exact-ref"
	return appcontract.CheckpointExternalExactRefV1{ContractVersion: "1.0.0", ExactSchemaRef: exactSchema, FactKind: factKind, Schema: runtimeports.SchemaRefV2{Namespace: "praxis.checkpoint", Name: name, Version: "1.0.0", MediaType: "application/json", ContentDigest: digestV1("schema-" + name)}, Owner: owner, TenantID: tenantID, ScopeDigest: scope, RunID: runID, ID: id, Revision: revision, Digest: digest}
}

func continuityOwnerV1(factKind string) continuitycontract.OwnerBinding {
	return continuitycontract.OwnerBinding{BindingSetID: "binding-set-continuity", BindingRevision: 1, ComponentID: continuitycontract.ContinuityComponentID, ManifestDigest: "continuity-manifest-digest-v1", ArtifactDigest: "continuity-artifact-digest-v1", Capability: continuitycontract.CheckpointManifestCapabilityV2, FactKind: factKind}
}

func digestV1(value string) core.Digest { return core.DigestBytes([]byte(value)) }

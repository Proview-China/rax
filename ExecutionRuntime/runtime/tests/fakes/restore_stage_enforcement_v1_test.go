package fakes_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type restoreStageDispatchReaderV1 struct {
	value ports.CurrentOperationDispatchAuthorizationV4
}

func (r *restoreStageDispatchReaderV1) IssueOperationDispatchV4(context.Context, ports.IssueGovernedOperationDispatchRequestV4) (ports.CurrentOperationDispatchAuthorizationV4, error) {
	panic("Restore Stage test reader cannot issue")
}

func (r *restoreStageDispatchReaderV1) InspectOperationDispatchRecordV4(context.Context, ports.InspectOperationDispatchRecordRequestV4) (ports.OperationDispatchRecordV4, error) {
	return r.value.Record, nil
}

func (r *restoreStageDispatchReaderV1) InspectCurrentOperationDispatchV4(_ context.Context, request ports.InspectCurrentOperationDispatchRequestV4) (ports.CurrentOperationDispatchAuthorizationV4, error) {
	legacy := r.value.Record.Permit.LegacyPermit
	if !ports.SameOperationSubjectV3(request.Inspect.Operation, legacy.Operation) || request.Inspect.EffectID != legacy.IntentID || request.Inspect.PermitID != legacy.ID || request.AdmissionDigest != r.value.Record.Permit.Admission.Digest || request.ReviewAuthorization != r.value.ReviewAuthorization {
		return ports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "Restore Stage dispatch current coordinates differ")
	}
	return r.value, nil
}

func (r *restoreStageDispatchReaderV1) BeginOperationDispatchV4(context.Context, ports.BeginGovernedOperationDispatchRequestV4) (ports.CurrentOperationDispatchAuthorizationV4, error) {
	panic("Restore Stage test reader cannot begin")
}

type restoreStageSandboxReaderV1 struct {
	mu    sync.Mutex
	value ports.RestoreStageSandboxCurrentProjectionV1
}

func (r *restoreStageSandboxReaderV1) InspectRestoreStageSandboxCurrentV1(_ context.Context, request ports.InspectRestoreStageSandboxCurrentRequestV1) (ports.RestoreStageSandboxCurrentProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value := r.value
	if request.Validate() != nil || !ports.SameOperationSubjectV3(request.Operation, value.Operation) || request.DispatchAttempt != value.DispatchAttempt || request.SandboxAttempt != value.SandboxAttempt || request.Identity != value.Identity || request.Provider != value.Provider {
		return ports.RestoreStageSandboxCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonProviderBindingStale, "Restore Stage Sandbox current coordinates differ")
	}
	return value, nil
}

type restoreStageEnforcementFixtureV1 struct {
	now     time.Time
	gateway control.RestoreStageEnforcementGatewayV1
	store   *fakes.RestoreStageEnforcementStoreV1
	reader  *restoreStageSandboxReaderV1
	prepare ports.EnforceRestoreStageDispatchRequestV1
}

func TestRestoreStageEnforcementPrepareExecuteLostReplyConcurrentAndCurrent(t *testing.T) {
	fixture := newRestoreStageEnforcementFixtureV1(t, "flow")
	fixture.store.LoseNextAppendReplyV1()
	prepared := enforceRestoreStageConcurrentlyV1(t, fixture.gateway, fixture.prepare, 64)
	if prepared.Phase != ports.OperationDispatchEnforcementPrepareV4 || fixture.store.CommitCountV1() != 1 {
		t.Fatalf("prepare did not linearize once: ref=%+v commits=%d", prepared, fixture.store.CommitCountV1())
	}
	if current, err := fixture.gateway.InspectCurrentRestoreStageDispatchEnforcementV1(context.Background(), fixture.prepare.Operation, prepared); err != nil || current != prepared {
		t.Fatalf("prepare current Inspect failed: %+v err=%v", current, err)
	}
	if recovered, err := fixture.gateway.InspectRestoreStageDispatchEnforcementByRequestV1(context.Background(), fixture.prepare); err != nil || recovered != prepared {
		t.Fatalf("prepare request recovery failed: %+v err=%v", recovered, err)
	}
	changed := fixture.prepare
	changed.SnapshotArtifact.ID += "-splice"
	if _, err := fixture.gateway.InspectRestoreStageDispatchEnforcementByRequestV1(context.Background(), changed); err == nil {
		t.Fatal("changed exact request recovered another prepare enforcement")
	}

	execute := fixture.prepare
	execute.Phase = ports.OperationDispatchEnforcementExecuteV4
	execute.ExpectedJournalRevision = 1
	execute.Prepare = &prepared
	preparedAttempt := fixture.reader.value.Prepared
	execute.Prepared = &preparedAttempt
	fixture.store.LoseNextAppendReplyV1()
	executed := enforceRestoreStageConcurrentlyV1(t, fixture.gateway, execute, 64)
	if executed.Phase != ports.OperationDispatchEnforcementExecuteV4 || executed.PrepareReceiptDigest != prepared.ReceiptDigest || executed.PreparedAttemptDigest != preparedAttempt.Digest || fixture.store.CommitCountV1() != 2 {
		t.Fatalf("execute did not close exact prepare once: ref=%+v commits=%d", executed, fixture.store.CommitCountV1())
	}
	if current, err := fixture.gateway.InspectCurrentOperationProviderExecuteEnforcementV1(context.Background(), execute.Operation, executed); err != nil || current != executed {
		t.Fatalf("execute current Inspect failed: %+v err=%v", current, err)
	}
	if recovered, err := fixture.gateway.InspectRestoreStageDispatchEnforcementByRequestV1(context.Background(), execute); err != nil || recovered != executed {
		t.Fatalf("execute request recovery failed: %+v err=%v", recovered, err)
	}
}

func TestRestoreStageEnforcementRejectsScopeFenceAndPreparedSplice(t *testing.T) {
	fixture := newRestoreStageEnforcementFixtureV1(t, "splice")
	cases := []struct {
		name   string
		mutate func(*ports.EnforceRestoreStageDispatchRequestV1)
	}{
		{name: "identity", mutate: func(request *ports.EnforceRestoreStageDispatchRequestV1) { request.Identity.TargetFenceEpoch++ }},
		{name: "sandbox_projection", mutate: func(request *ports.EnforceRestoreStageDispatchRequestV1) {
			request.SandboxProjectionDigest = core.DigestBytes([]byte("other-sandbox-projection"))
		}},
		{name: "permit", mutate: func(request *ports.EnforceRestoreStageDispatchRequestV1) {
			request.PermitDigest = core.DigestBytes([]byte("other-permit"))
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := fixture.prepare
			testCase.mutate(&request)
			if _, err := fixture.gateway.EnforceRestoreStageDispatchV1(context.Background(), request); err == nil {
				t.Fatal("spliced Restore Stage enforcement was accepted")
			}
			if fixture.store.CommitCountV1() != 0 {
				t.Fatal("failed enforcement changed the journal")
			}
		})
	}

	prepared, err := fixture.gateway.EnforceRestoreStageDispatchV1(context.Background(), fixture.prepare)
	if err != nil {
		t.Fatal(err)
	}
	execute := fixture.prepare
	execute.Phase = ports.OperationDispatchEnforcementExecuteV4
	execute.ExpectedJournalRevision = 1
	execute.Prepare = &prepared
	changed := fixture.reader.value.Prepared
	changed.BundleDigest = core.DigestBytes([]byte("other-bundle"))
	changed, err = ports.SealRestoreStagePreparedAttemptRefV1(changed)
	if err != nil {
		t.Fatal(err)
	}
	execute.Prepared = &changed
	if _, err := fixture.gateway.EnforceRestoreStageDispatchV1(context.Background(), execute); err == nil {
		t.Fatal("changed prepared attempt reached execute enforcement")
	}
	if fixture.store.CommitCountV1() != 1 {
		t.Fatal("failed execute changed the journal")
	}
}

func enforceRestoreStageConcurrentlyV1(t *testing.T, gateway control.RestoreStageEnforcementGatewayV1, request ports.EnforceRestoreStageDispatchRequestV1, workers int) ports.OperationDispatchEnforcementPhaseRefV4 {
	t.Helper()
	results := make(chan ports.OperationDispatchEnforcementPhaseRefV4, workers)
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := gateway.EnforceRestoreStageDispatchV1(context.Background(), request)
			results <- result
			errors <- err
		}()
	}
	wait.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("exact concurrent enforcement failed: %v", err)
		}
	}
	var winner ports.OperationDispatchEnforcementPhaseRefV4
	for result := range results {
		if winner == (ports.OperationDispatchEnforcementPhaseRefV4{}) {
			winner = result
		} else if result != winner {
			t.Fatal("exact concurrent enforcement returned different refs")
		}
	}
	return winner
}

func newRestoreStageEnforcementFixtureV1(t *testing.T, suffix string) restoreStageEnforcementFixtureV1 {
	t.Helper()
	base := newOperationEnforcementFixtureV4(t, "restore-stage-"+suffix)
	now := base.effect.now
	current, err := base.dispatch.gateway.InspectCurrentOperationDispatchV4(context.Background(), ports.InspectCurrentOperationDispatchRequestV4{Inspect: ports.InspectOperationDispatchRecordRequestV4{Operation: base.prepare.Operation, EffectID: base.prepare.EffectID, PermitID: base.prepare.PermitID}, AdmissionDigest: base.prepare.AdmissionDigest, ReviewAuthorization: base.prepare.ReviewAuthorization})
	if err != nil {
		t.Fatal(err)
	}
	restoreAttempt := ports.RestoreAttemptRefV2{TenantID: base.prepare.Operation.ExecutionScope.Identity.TenantID, ID: "restore-attempt-" + suffix, Revision: 2, Digest: core.DigestBytes([]byte("restore-attempt-" + suffix))}
	operation := base.prepare.Operation
	sourceInstance := operation.ExecutionScope.Instance
	operation.ExecutionScope.Instance = core.InstanceRef{ID: core.AgentInstanceID("target-" + suffix), Epoch: sourceInstance.Epoch + 1}
	targetLease := core.SandboxLeaseRef{ID: core.SandboxLeaseID("target-lease-" + suffix), Epoch: operation.ExecutionScope.Instance.Epoch}
	operation.ExecutionScope.SandboxLease = &targetLease
	operation.ExecutionScope.AuthorityEpoch = operation.ExecutionScope.Instance.Epoch
	operation.ExecutionScopeDigest, err = ports.ExecutionScopeDigestV2(operation.ExecutionScope)
	if err != nil {
		t.Fatal(err)
	}
	operation.Kind = ports.RestoreStageOperationKindV1
	operation.RunID = ""
	operation.ActivationAttemptID = ""
	operation.CustomOperationID = restoreAttempt.ID
	operation.CurrentProjectionRef = "restore-stage-current-" + suffix
	operation.CurrentProjectionDigest = core.DigestBytes([]byte(operation.CurrentProjectionRef))
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	legacy := current.Record.Permit.LegacyPermit
	legacy.Operation = operation
	fence := current.Record.Fence
	fence.Scope = operation.ExecutionScope
	legacy.FenceDigest, err = ports.DigestOperationExecutionFenceV3(fence, operation)
	if err != nil {
		t.Fatal(err)
	}
	admission := current.Record.Permit.Admission
	admission.Admission.OperationDigest = operationDigest
	admission, err = ports.SealOperationAuthorizedAdmissionV4(admission)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := ports.SealOperationDispatchPermitV4(ports.OperationDispatchPermitV4{LegacyPermit: legacy, Admission: admission})
	if err != nil {
		t.Fatal(err)
	}
	record := current.Record
	record.Permit = permit
	record.PermitDigest = permit.Digest
	record.Fence = fence
	record, err = ports.SealOperationDispatchRecordV4(record)
	if err != nil {
		t.Fatal(err)
	}
	current.Record = record
	if err := current.Validate(); err != nil {
		t.Fatal(err)
	}

	identity := ports.RestoreIdentityReservationV2{SourceInstance: sourceInstance, TargetInstance: operation.ExecutionScope.Instance, TargetLease: *operation.ExecutionScope.SandboxLease, TargetFenceEpoch: operation.ExecutionScope.AuthorityEpoch}
	eligibility := ports.RestoreEligibilityRefV2{TenantID: restoreAttempt.TenantID, ID: "restore-eligibility-" + suffix, Revision: 1, Digest: core.DigestBytes([]byte("restore-eligibility-" + suffix)), ExpiresUnixNano: now.Add(7 * time.Second).UnixNano()}
	snapshot := restoreStageEnforcementExternalRefV1(restoreAttempt.TenantID, "snapshot-"+suffix)
	dispatchAttempt := ports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: legacy.IntentID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest, PermitID: legacy.ID, PermitRevision: record.Revision, PermitDigest: record.PermitDigest, AttemptID: legacy.AttemptID}
	sandboxAttempt := ports.OperationDispatchSandboxFactRefV4{ID: dispatchAttempt.AttemptID, Revision: 1, Digest: core.DigestBytes([]byte("sandbox-attempt-" + suffix)), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano()}
	provider := legacy.EnforcementPoint
	prepared, err := ports.SealRestoreStagePreparedAttemptRefV1(ports.RestoreStagePreparedAttemptRefV1{SandboxAttempt: sandboxAttempt, OperationDigest: operationDigest, EffectID: legacy.IntentID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest, DispatchAttempt: dispatchAttempt, Provider: provider, BundleDigest: core.DigestBytes([]byte("bundle-" + suffix)), PreparedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := ports.SealRestoreStageSandboxCurrentProjectionV1(ports.RestoreStageSandboxCurrentProjectionV1{Operation: operation, OperationDigest: operationDigest, EffectID: legacy.IntentID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest, DispatchAttempt: dispatchAttempt, SandboxAttempt: sandboxAttempt, RestoreAttempt: restoreAttempt, Eligibility: eligibility, Identity: identity, SnapshotArtifact: snapshot, BundleProjectionDigest: core.DigestBytes([]byte("bundle-projection-" + suffix)), BundleDigest: prepared.BundleDigest, Provider: provider, Prepared: prepared, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: prepared.ExpiresUnixNano}, now)
	if err != nil {
		t.Fatalf("seal Sandbox projection: %v operation=%v dispatch=%v sandbox=%v restore=%v eligibility=%v identity=%v snapshot=%v provider=%v prepared=%v", err, operation.Validate(), dispatchAttempt.Validate(), sandboxAttempt.Validate(), restoreAttempt.Validate(), eligibility.Validate(), identity.Validate(), snapshot.Validate(), provider.Validate(), prepared.Validate())
	}
	store := fakes.NewRestoreStageEnforcementStoreV1()
	reader := &restoreStageSandboxReaderV1{value: sandbox}
	gateway := control.RestoreStageEnforcementGatewayV1{Dispatch: &restoreStageDispatchReaderV1{value: current}, Sandbox: reader, Facts: store, Clock: func() time.Time { return now }}
	request := ports.EnforceRestoreStageDispatchRequestV1{Operation: operation, EffectID: legacy.IntentID, PermitID: legacy.ID, ExpectedPermitFactRevision: record.Revision, PermitDigest: record.PermitDigest, AdmissionDigest: admission.Digest, ReviewAuthorization: current.ReviewAuthorization, DispatchAttempt: dispatchAttempt, SandboxAttempt: sandboxAttempt, SandboxProjectionDigest: sandbox.ProjectionDigest, RestoreAttempt: restoreAttempt, Eligibility: eligibility, Identity: identity, SnapshotArtifact: snapshot, Verifier: provider, Phase: ports.OperationDispatchEnforcementPrepareV4}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	return restoreStageEnforcementFixtureV1{now: now, gateway: gateway, store: store, reader: reader, prepare: request}
}

func restoreStageEnforcementExternalRefV1(tenant core.TenantID, id string) ports.CheckpointExternalExactFactRefV2 {
	return ports.CheckpointExternalExactFactRefV2{
		ContractVersion: "praxis.sandbox/snapshot-artifact/v2",
		SchemaRef:       "praxis.sandbox/snapshot-artifact-schema/v2",
		Owner: ports.CheckpointManifestSealOwnerBindingV2{
			BindingSetID: "sandbox-binding", BindingRevision: 1, ComponentID: "praxis/sandbox",
			ManifestDigest: string(core.DigestBytes([]byte("sandbox-manifest"))), ArtifactDigest: string(core.DigestBytes([]byte("sandbox-artifact-code"))),
			Capability: "snapshot-artifact-current", FactKind: "snapshot-artifact-fact",
		},
		TenantID: string(tenant), ID: id, Revision: 1, Digest: string(core.DigestBytes([]byte(id))), ScopeDigest: string(core.DigestBytes([]byte("source-scope"))),
	}
}

var _ ports.OperationGovernancePortV4 = (*restoreStageDispatchReaderV1)(nil)
var _ ports.RestoreStageSandboxCurrentReaderV1 = (*restoreStageSandboxReaderV1)(nil)

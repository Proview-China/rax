package fakes_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type checkpointDispatchSandboxReaderV1 struct {
	mu    sync.Mutex
	value ports.CheckpointRestoreDispatchSandboxCurrentProjectionV1
}

func (r *checkpointDispatchSandboxReaderV1) InspectCheckpointRestoreDispatchSandboxCurrentV1(_ context.Context, operation ports.OperationSubjectV3, effectID core.EffectIntentID, reservation ports.CheckpointParticipantPhaseReservationRefV2) (ports.CheckpointRestoreDispatchSandboxCurrentProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !ports.SameOperationSubjectV3(r.value.Operation, operation) || r.value.EffectID != effectID || r.value.Reservation.Ref != reservation {
		return ports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonCheckpointInconsistent, "checkpoint Sandbox projection not found")
	}
	return r.value, nil
}

func (r *checkpointDispatchSandboxReaderV1) replace(value ports.CheckpointRestoreDispatchSandboxCurrentProjectionV1) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value = value
}

func TestCheckpointRestoreDispatchEnforcementPrepareExecuteLostReplyAndCurrentInspect(t *testing.T) {
	base := newOperationEnforcementFixtureV4(t, "checkpoint-enforcement")
	projection := checkpointDispatchProjectionV1(t, base)
	reader := &checkpointDispatchSandboxReaderV1{value: projection}
	gateway := control.CheckpointRestoreDispatchEnforcementGatewayV1{Dispatch: base.dispatch.gateway, Sandbox: reader, Facts: base.effect.store, Clock: func() time.Time { return base.effect.now }}
	prepare := checkpointEnforcementRequestV1(base, projection)

	base.effect.store.LoseNextEnforcementV4Reply()
	prepared, err := gateway.EnforceCurrentCheckpointRestoreDispatchV1(context.Background(), prepare)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Phase.Phase != ports.OperationDispatchEnforcementPrepareV4 || prepared.Journal.Revision != 1 || prepared.Journal.Prepare.CheckpointSandbox == nil || base.effect.store.EnforcementV4CommitCount() != 1 {
		t.Fatalf("checkpoint prepare did not persist one specialized receipt: %#v", prepared)
	}
	inspected, err := gateway.InspectCurrentCheckpointRestoreDispatchV1(context.Background(), ports.InspectCurrentCheckpointRestoreDispatchRequestV1{
		Operation: prepare.Operation, EffectID: prepare.EffectID, PermitID: prepare.PermitID, Phase: ports.OperationDispatchEnforcementPrepareV4,
		PermitDigest: prepare.PermitDigest, AdmissionDigest: prepare.AdmissionDigest, ReviewAuthorization: prepare.ReviewAuthorization,
		Reservation: prepare.Reservation, SandboxProjectionDigest: prepare.SandboxProjectionDigest,
	})
	if err != nil || inspected.Phase != prepared.Phase {
		t.Fatalf("checkpoint current prepare Inspect failed: %#v err=%v", inspected, err)
	}

	preparedAttempt := checkpointPreparedAttemptV1(t, base, prepared)
	executeProjection := projection
	executeProjection.Stage = ports.CheckpointRestoreDispatchSandboxPreExecuteV1
	executeProjection.PrepareEnforcement = &prepared.Phase
	executeProjection.PreparedAttempt = preparedAttempt
	executeProjection.ExpiresUnixNano = preparedAttempt.ExpiresUnixNano
	executeProjection, err = ports.SealCheckpointRestoreDispatchSandboxCurrentProjectionV1(executeProjection, base.effect.now)
	if err != nil {
		t.Fatal(err)
	}
	reader.replace(executeProjection)
	execute := prepare
	execute.Phase = ports.OperationDispatchEnforcementExecuteV4
	execute.ExpectedJournalRevision = 1
	execute.Prepare = &prepared.Phase
	execute.PreparedAttempt = preparedAttempt
	execute.SandboxProjectionDigest = executeProjection.ProjectionDigest
	base.effect.store.LoseNextEnforcementV4Reply()
	executed, err := gateway.EnforceCurrentCheckpointRestoreDispatchV1(context.Background(), execute)
	if err != nil {
		t.Fatal(err)
	}
	if executed.Phase.Phase != ports.OperationDispatchEnforcementExecuteV4 || executed.Journal.Revision != 2 || executed.Journal.Execute.CheckpointSandbox == nil || base.effect.store.EnforcementV4CommitCount() != 2 {
		t.Fatalf("checkpoint execute did not append one specialized receipt: %#v", executed)
	}
}

func TestCheckpointRestoreDispatchEnforcementDriftFailsBeforeJournalWrite(t *testing.T) {
	base := newOperationEnforcementFixtureV4(t, "checkpoint-enforcement-drift")
	projection := checkpointDispatchProjectionV1(t, base)
	reader := &checkpointDispatchSandboxReaderV1{value: projection}
	gateway := control.CheckpointRestoreDispatchEnforcementGatewayV1{Dispatch: base.dispatch.gateway, Sandbox: reader, Facts: base.effect.store, Clock: func() time.Time { return base.effect.now }}
	request := checkpointEnforcementRequestV1(base, projection)

	drifted := projection
	drifted.Participant.Revision++
	drifted.Participant.Digest = core.DigestBytes([]byte("drifted-participant"))
	drifted, err := ports.SealCheckpointRestoreDispatchSandboxCurrentProjectionV1(drifted, base.effect.now)
	if err != nil {
		t.Fatal(err)
	}
	reader.replace(drifted)
	if _, err := gateway.EnforceCurrentCheckpointRestoreDispatchV1(context.Background(), request); err == nil {
		t.Fatal("changed checkpoint Sandbox current projection produced an Enforcement receipt")
	}
	if base.effect.store.EnforcementV4CommitCount() != 0 {
		t.Fatal("failed checkpoint exact-current validation changed the Enforcement journal")
	}
}

func checkpointDispatchProjectionV1(t *testing.T, base *operationEnforcementFixtureV4) ports.CheckpointRestoreDispatchSandboxCurrentProjectionV1 {
	t.Helper()
	now := base.effect.now
	expires := now.Add(8 * time.Second).UnixNano()
	ref := func(id string) ports.OperationDispatchSandboxFactRefV4 {
		return ports.OperationDispatchSandboxFactRefV4{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id)), ExpiresUnixNano: expires}
	}
	operation := base.prepare.Operation
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	checkpointAttempt := ports.CheckpointAttemptRefV2{TenantID: operation.ExecutionScope.Identity.TenantID, ID: "checkpoint-attempt-enforcement", Revision: 1, Digest: core.DigestBytes([]byte("checkpoint-attempt-enforcement"))}
	barrier := ports.CheckpointBarrierLeaseRefV2{TenantID: checkpointAttempt.TenantID, ID: "checkpoint-barrier-enforcement", AttemptID: checkpointAttempt.ID, Revision: 1, Digest: core.DigestBytes([]byte("checkpoint-barrier-enforcement")), ExpiresUnixNano: expires}
	cut := ports.EffectCutRefV2{ID: "checkpoint-cut-enforcement", Revision: 1, Attempt: checkpointAttempt, RootDigest: core.DigestBytes([]byte("checkpoint-cut-root")), Watermark: 1, Count: 0, Digest: core.DigestBytes([]byte("checkpoint-cut-enforcement"))}
	participant := ports.CheckpointParticipantRefV2{ID: "sandbox-checkpoint-participant", Owner: base.prepare.Verifier, Digest: core.DigestBytes([]byte("sandbox-checkpoint-participant"))}
	reservationRef := ports.CheckpointParticipantPhaseReservationRefV2{ID: "checkpoint-reservation-enforcement", Revision: 1, Digest: core.DigestBytes([]byte("checkpoint-reservation-enforcement")), ExpiresUnixNano: expires}
	generation := ports.GenerationBindingAssociationRefV1{ID: "checkpoint-generation-enforcement", Revision: 1, Digest: core.DigestBytes([]byte("checkpoint-generation-enforcement"))}
	reservation := ports.CheckpointParticipantPhaseReservationCurrentProjectionV2{
		ContractVersion: ports.CheckpointParticipantReservationContractVersionV2, Ref: reservationRef,
		Participant: participant, OwnerBinding: base.prepare.Verifier, Phase: ports.CheckpointPhasePrepareV2,
		Attempt: checkpointAttempt, Barrier: barrier, EffectCut: cut, Operation: operation, OperationDigest: operationDigest,
		EffectID: base.prepare.EffectID, EffectKind: "praxis.sandbox/checkpoint", IntentDigest: base.dispatch.issue.Admission.IntentDigest,
		Domain:     ports.CheckpointParticipantDomainReservationRefV2{ID: "checkpoint-domain-reservation-enforcement", Revision: 1, Digest: core.DigestBytes([]byte("checkpoint-domain-reservation-enforcement"))},
		Generation: generation, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	}
	reservation.ProjectionDigest, err = reservation.DigestV2()
	if err != nil || reservation.Validate(now) != nil {
		t.Fatalf("seal checkpoint reservation projection: %v validate=%v", err, reservation.Validate(now))
	}
	lease := *operation.ExecutionScope.SandboxLease
	projection, err := ports.SealCheckpointRestoreDispatchSandboxCurrentProjectionV1(ports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{
		Operation: operation, OperationDigest: operationDigest, EffectID: base.prepare.EffectID,
		IntentRevision: base.effect.intent.Revision, IntentDigest: base.dispatch.issue.Admission.IntentDigest,
		Reservation: reservation, SandboxReservation: ref(reservationRef.ID), Participant: ref(participant.ID), DispatchAttempt: ref(base.prepare.AttemptID),
		RuntimeLease: ports.OperationDispatchRuntimeLeaseBindingV4{Ref: ref("checkpoint-runtime-lease-enforcement"), Lease: lease, Instance: operation.ExecutionScope.Instance, FenceEpoch: operation.ExecutionScope.AuthorityEpoch, ScopeDigest: operation.ExecutionScopeDigest, ObservedRevision: 1},
		Requirement:  ref("checkpoint-requirement-enforcement"), Policy: ref("checkpoint-policy-enforcement"), Workspace: ref("checkpoint-workspace-enforcement"),
		Placement: ref("checkpoint-placement-enforcement"), Backend: ref("checkpoint-backend-enforcement"), Slot: ref("checkpoint-slot-enforcement"),
		Generation: generation, Verifier: base.prepare.Verifier, Stage: ports.CheckpointRestoreDispatchSandboxPrePrepareV1,
		Watermarks: []ports.CheckpointRestoreDispatchWatermarkV1{{SourceID: "checkpoint-source-enforcement", SourceEpoch: 1, Sequence: 1}},
		Current:    true, ProjectionRevision: 1, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func checkpointEnforcementRequestV1(base *operationEnforcementFixtureV4, projection ports.CheckpointRestoreDispatchSandboxCurrentProjectionV1) ports.EnforceCurrentCheckpointRestoreDispatchRequestV1 {
	return ports.EnforceCurrentCheckpointRestoreDispatchRequestV1{
		Operation: base.prepare.Operation, EffectID: base.prepare.EffectID, PermitID: base.prepare.PermitID,
		ExpectedPermitFactRevision: base.prepare.ExpectedPermitFactRevision, PermitDigest: base.prepare.PermitDigest,
		AdmissionDigest: base.prepare.AdmissionDigest, ReviewAuthorization: base.prepare.ReviewAuthorization,
		AttemptID: base.prepare.AttemptID, Phase: ports.OperationDispatchEnforcementPrepareV4,
		Reservation: projection.Reservation.Ref, SandboxProjectionDigest: projection.ProjectionDigest, Verifier: base.prepare.Verifier,
	}
}

func checkpointPreparedAttemptV1(t *testing.T, base *operationEnforcementFixtureV4, prepared ports.CurrentCheckpointRestoreDispatchEnforcementV1) *ports.PreparedProviderAttemptRefV2 {
	t.Helper()
	legacy := prepared.Dispatch.Record.Permit.LegacyPermit
	legacyDigest, err := legacy.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	delegation := ports.ExecutionDelegationRefV2{ID: "delegation-checkpoint-enforcement", Revision: 1, Digest: core.DigestBytes([]byte("delegation-checkpoint-enforcement"))}
	id, err := ports.DerivePreparedProviderAttemptIDV2(delegation.ID, legacy.ID, legacy.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	value, err := ports.SealPreparedProviderAttemptRefV2(ports.PreparedProviderAttemptRefV2{
		ID: id, Revision: 1, DeclaredDelegation: delegation, OperationDigest: mustOperationDigestForEnforcementV4(t, legacy.Operation),
		IntentID: legacy.IntentID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest,
		PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest, AttemptID: legacy.AttemptID,
		Provider: legacy.EnforcementPoint, PayloadSchema: legacy.PayloadSchema, PayloadDigest: legacy.PayloadDigest, PayloadRevision: legacy.PayloadRevision,
		PreparedUnixNano: base.effect.now.UnixNano(), ExpiresUnixNano: base.effect.now.Add(7 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &value
}

var _ ports.CheckpointRestoreDispatchSandboxCurrentReaderV1 = (*checkpointDispatchSandboxReaderV1)(nil)
var _ ports.CheckpointRestoreDispatchEnforcementGovernancePortV1 = control.CheckpointRestoreDispatchEnforcementGatewayV1{}

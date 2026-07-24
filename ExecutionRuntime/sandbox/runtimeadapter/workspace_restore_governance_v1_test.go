package runtimeadapter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceRestoreGovernanceReaderV1MapsExactRuntimeCurrent(t *testing.T) {
	fixture := restoreStageAdapterFixtureV1(t)
	reader, err := NewWorkspaceRestoreGovernanceReaderV1(fixture.coordinates, fixture.current, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	projection, err := reader.InspectWorkspaceRestoreGovernanceCurrentV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if projection.TenantID != fixture.request.TenantID || projection.RuntimeRestoreAttempt != fixture.request.RuntimeRestoreAttempt || projection.RestoreEligibility != fixture.request.RestoreEligibility || projection.Target != fixture.request.Target || projection.EnforcementRef.ID != fixture.runtime.ExecuteEnforcement.AttemptID || projection.ExpiresUnixNano != fixture.runtime.ExpiresUnixNano {
		t.Fatalf("mapped projection drifted: %#v", projection)
	}
	if fixture.coordinates.calls != 1 || fixture.current.calls != 1 {
		t.Fatalf("coordinates=%d current=%d", fixture.coordinates.calls, fixture.current.calls)
	}
}

func TestWorkspaceRestoreGovernanceReaderV1RejectsSpliceTTLAndUnavailable(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*restoreStageAdapterFixture)
	}{
		{name: "restore-attempt", mutate: func(v *restoreStageAdapterFixture) { v.request.RuntimeRestoreAttempt.ID = "other" }},
		{name: "eligibility", mutate: func(v *restoreStageAdapterFixture) { v.request.RestoreEligibility.Revision++ }},
		{name: "instance", mutate: func(v *restoreStageAdapterFixture) { v.request.Target.InstanceEpoch++ }},
		{name: "lease", mutate: func(v *restoreStageAdapterFixture) { v.request.Target.LeaseID = "other" }},
		{name: "fence", mutate: func(v *restoreStageAdapterFixture) { v.request.Target.FenceEpoch++ }},
		{name: "scope", mutate: func(v *restoreStageAdapterFixture) {
			v.request.Target.ScopeDigest = strings.Repeat("a", contract.DigestSizeHex)
		}},
		{name: "target-ttl-extension", mutate: func(v *restoreStageAdapterFixture) { v.request.Target.ExpiresUnixNano = v.runtime.ExpiresUnixNano + 1 }},
		{name: "artifact", mutate: func(v *restoreStageAdapterFixture) {
			v.request.SnapshotArtifactFactRef.Digest = strings.Repeat("b", contract.DigestSizeHex)
		}},
		{name: "coordinate-drift", mutate: func(v *restoreStageAdapterFixture) { v.coordinates.value.PermitID = "other-permit" }},
		{name: "runtime-unavailable", mutate: func(v *restoreStageAdapterFixture) { v.current.err = errors.New("runtime current unavailable") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := restoreStageAdapterFixtureV1(t)
			test.mutate(fixture)
			reader, err := NewWorkspaceRestoreGovernanceReaderV1(fixture.coordinates, fixture.current, func() time.Time { return fixture.now })
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reader.InspectWorkspaceRestoreGovernanceCurrentV1(context.Background(), fixture.request); err == nil {
				t.Fatal("drifted Restore Stage current was accepted")
			}
		})
	}
}

func TestWorkspaceRestoreGovernanceReaderV1ExactExpiryAndTypedNil(t *testing.T) {
	fixture := restoreStageAdapterFixtureV1(t)
	reader, err := NewWorkspaceRestoreGovernanceReaderV1(fixture.coordinates, fixture.current, func() time.Time { return time.Unix(0, fixture.runtime.ExpiresUnixNano) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.InspectWorkspaceRestoreGovernanceCurrentV1(context.Background(), fixture.request); err == nil {
		t.Fatal("Restore Stage current remained usable at exact expiry")
	}
	var coordinates *restoreStageCoordinateFakeV1
	if _, err := NewWorkspaceRestoreGovernanceReaderV1(coordinates, fixture.current, time.Now); err == nil {
		t.Fatal("typed-nil Restore Stage coordinate reader was accepted")
	}
	var current *restoreStageCurrentFakeV1
	if _, err := NewWorkspaceRestoreGovernanceReaderV1(fixture.coordinates, current, time.Now); err == nil {
		t.Fatal("typed-nil Runtime current Gateway was accepted")
	}
}

type restoreStageAdapterFixture struct {
	now         time.Time
	runtime     runtimeports.RestoreStageGovernanceCurrentProjectionV1
	request     contract.WorkspaceRestoreStageRequestV1
	coordinates *restoreStageCoordinateFakeV1
	current     *restoreStageCurrentFakeV1
}

func restoreStageAdapterFixtureV1(t *testing.T) *restoreStageAdapterFixture {
	t.Helper()
	now := time.Unix(1_960_000_000, 0)
	tenant := runtimecore.TenantID("tenant-restore")
	attempt := runtimeports.RestoreAttemptRefV2{TenantID: tenant, ID: "restore-attempt", Revision: 2, Digest: runtimecore.DigestBytes([]byte("restore-attempt"))}
	eligibility := runtimeports.RestoreEligibilityRefV2{TenantID: tenant, ID: "restore-eligibility", Revision: 1, Digest: runtimecore.DigestBytes([]byte("restore-eligibility")), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	identity := runtimeports.RestoreIdentityReservationV2{SourceInstance: runtimecore.InstanceRef{ID: "source", Epoch: 1}, TargetInstance: runtimecore.InstanceRef{ID: "target", Epoch: 2}, TargetLease: runtimecore.SandboxLeaseRef{ID: "lease", Epoch: 2}, TargetFenceEpoch: 2}
	scope := runtimecore.ExecutionScope{Identity: runtimecore.AgentIdentityRef{TenantID: tenant, ID: "identity", Epoch: 1}, Lineage: runtimecore.LineageRef{ID: "lineage", PlanDigest: runtimecore.DigestBytes([]byte("plan"))}, Instance: identity.TargetInstance, SandboxLease: &identity.TargetLease, AuthorityEpoch: identity.TargetFenceEpoch}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.RestoreStageOperationKindV1, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, CustomOperationID: attempt.ID, SubjectRevision: 1, CurrentProjectionRef: "restore-current", CurrentProjectionDigest: runtimecore.DigestBytes([]byte("current")), CurrentProjectionRevision: 7}
	operationDigest, _ := operation.DigestV3()
	intentDigest := runtimecore.DigestBytes([]byte("intent"))
	admission := runtimeports.OperationEffectAdmissionReceiptV3{OperationDigest: operationDigest, EffectID: "restore-effect", IntentRevision: 1, IntentDigest: intentDigest, FactRevision: 3, State: "accepted"}
	authorization := runtimeports.OperationReviewAuthorizationRefV4{ID: "authorization", Revision: 4, Digest: runtimecore.DigestBytes([]byte("authorization"))}
	permitDigest := runtimecore.DigestBytes([]byte("permit"))
	dispatch := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: admission.EffectID, IntentRevision: 1, IntentDigest: intentDigest, PermitID: "permit", PermitRevision: 5, PermitDigest: permitDigest, AttemptID: "dispatch-attempt"}
	enforcement := runtimeports.OperationDispatchEnforcementPhaseRefV4{OperationDigest: operationDigest, EffectID: admission.EffectID, PermitID: dispatch.PermitID, PermitFactRevision: dispatch.PermitRevision, PermitDigest: permitDigest, AdmissionDigest: runtimecore.DigestBytes([]byte("admission")), ReviewAuthorization: authorization, AttemptID: dispatch.AttemptID, SandboxAttempt: runtimeports.OperationDispatchSandboxFactRefV4{ID: dispatch.AttemptID, Revision: 1, Digest: runtimecore.DigestBytes([]byte("sandbox-attempt")), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}, Phase: runtimeports.OperationDispatchEnforcementExecuteV4, ReceiptDigest: runtimecore.DigestBytes([]byte("receipt")), JournalRevision: 2, ValidatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(), PrepareReceiptDigest: runtimecore.DigestBytes([]byte("prepare")), PreparedAttemptDigest: runtimecore.DigestBytes([]byte("prepared"))}
	artifact := runtimeports.CheckpointExternalExactFactRefV2{ContractVersion: "praxis.sandbox/snapshot-artifact/v2", SchemaRef: "praxis.sandbox/snapshot-artifact-schema/v2", Owner: runtimeports.CheckpointManifestSealOwnerBindingV2{BindingSetID: "sandbox-binding", BindingRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: string(runtimecore.DigestBytes([]byte("manifest"))), ArtifactDigest: string(runtimecore.DigestBytes([]byte("artifact-code"))), Capability: "snapshot-artifact-current", FactKind: "snapshot-artifact-fact"}, TenantID: string(tenant), ID: "artifact", Revision: 2, Digest: string(runtimecore.DigestBytes([]byte("artifact"))), ScopeDigest: string(scopeDigest)}
	runtimeProjection, err := runtimeports.SealRestoreStageGovernanceCurrentProjectionV1(runtimeports.RestoreStageGovernanceCurrentProjectionV1{RestoreAttempt: attempt, Eligibility: eligibility, Identity: identity, Operation: operation, EffectID: admission.EffectID, EffectRevision: 1, IntentDigest: intentDigest, Admission: admission, DispatchAdmissionDigest: enforcement.AdmissionDigest, Authorization: authorization, PermitID: dispatch.PermitID, PermitFactRevision: dispatch.PermitRevision, PermitDigest: permitDigest, BeginRecordRevision: 6, BeginRecordDigest: runtimecore.DigestBytes([]byte("begin")), DispatchAttempt: dispatch, ExecuteEnforcement: enforcement, MaterializationDigest: runtimecore.DigestBytes([]byte("materialization")), SnapshotArtifact: artifact, CheckedUnixNano: enforcement.ValidatedUnixNano, ExpiresUnixNano: enforcement.ExpiresUnixNano}, now)
	if err != nil {
		t.Fatal(err)
	}
	expires := runtimeProjection.ExpiresUnixNano
	strip := func(value string) string { return strings.TrimPrefix(value, "sha256:") }
	request := contract.WorkspaceRestoreStageRequestV1{
		TenantID:                string(tenant),
		DispatchAttemptID:       dispatch.AttemptID,
		RuntimeRestoreAttempt:   contract.SnapshotArtifactExactRefV2{TypeURL: "praxis.runtime/restore-attempt/v2", Version: 2, ID: attempt.ID, Revision: uint64(attempt.Revision), DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: restoreAttemptDigestDomainV2, Digest: strip(string(attempt.Digest)), ExpiresUnixNano: expires},
		RestoreEligibility:      contract.SnapshotArtifactExactRefV2{TypeURL: "praxis.runtime/restore-eligibility/v2", Version: 2, ID: eligibility.ID, Revision: uint64(eligibility.Revision), DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: restoreEligibilityDigestDomainV2, Digest: strip(string(eligibility.Digest)), ExpiresUnixNano: eligibility.ExpiresUnixNano},
		Target:                  contract.RuntimeLeaseBinding{TenantID: string(tenant), InstanceID: string(identity.TargetInstance.ID), InstanceEpoch: uint64(identity.TargetInstance.Epoch), LeaseID: string(identity.TargetLease.ID), LeaseEpoch: uint64(identity.TargetLease.Epoch), FenceEpoch: uint64(identity.TargetFenceEpoch), ScopeDigest: string(scopeDigest), ObservedRevision: uint64(operation.CurrentProjectionRevision), ExpiresUnixNano: expires},
		SnapshotArtifactFactRef: contract.SnapshotArtifactExactRefV2{TypeURL: contract.SnapshotArtifactFactTypeURL, Version: contract.SnapshotArtifactVersion, ID: artifact.ID, Revision: uint64(artifact.Revision), DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: contract.SnapshotArtifactFactDomain, Digest: strip(artifact.Digest), ExpiresUnixNano: expires},
		RequestedNotAfter:       expires,
	}
	coordinates := runtimeports.InspectRestoreStageGovernanceCurrentRequestV1{RestoreAttempt: attempt, Eligibility: eligibility, Operation: operation, EffectID: admission.EffectID, Admission: admission, Authorization: authorization, PermitID: dispatch.PermitID, DispatchAttempt: dispatch, ExecuteEnforcement: enforcement, SnapshotArtifact: artifact}
	return &restoreStageAdapterFixture{now: now, runtime: runtimeProjection, request: request, coordinates: &restoreStageCoordinateFakeV1{value: coordinates}, current: &restoreStageCurrentFakeV1{value: runtimeProjection}}
}

type restoreStageCoordinateFakeV1 struct {
	value runtimeports.InspectRestoreStageGovernanceCurrentRequestV1
	err   error
	calls int
}

func (r *restoreStageCoordinateFakeV1) ReadRestoreStageCoordinatesV1(context.Context, contract.WorkspaceRestoreStageRequestV1) (runtimeports.InspectRestoreStageGovernanceCurrentRequestV1, error) {
	r.calls++
	return r.value, r.err
}

type restoreStageCurrentFakeV1 struct {
	value runtimeports.RestoreStageGovernanceCurrentProjectionV1
	err   error
	calls int
}

func (r *restoreStageCurrentFakeV1) InspectRestoreStageGovernanceCurrentV1(context.Context, runtimeports.InspectRestoreStageGovernanceCurrentRequestV1) (runtimeports.RestoreStageGovernanceCurrentProjectionV1, error) {
	r.calls++
	return r.value, r.err
}

var _ RestoreStageCoordinateReaderV1 = (*restoreStageCoordinateFakeV1)(nil)
var _ runtimeports.RestoreStageGovernanceCurrentPortV1 = (*restoreStageCurrentFakeV1)(nil)
var _ = ports.ErrConflict

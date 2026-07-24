package applicationadapter

import (
	"context"
	"reflect"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	harnesscontract "github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type CheckpointGateApplicationAdapterConfigV1 struct {
	Gates         harnessports.CheckpointGateGovernancePortV1
	SubjectOwner  runtimeports.ProviderBindingRefV2
	GateOwner     runtimeports.ProviderBindingRefV2
	SnapshotOwner runtimeports.ProviderBindingRefV2
	Clock         func() time.Time
}

type CheckpointGateApplicationAdapterV1 struct {
	config CheckpointGateApplicationAdapterConfigV1
}

func NewCheckpointGateApplicationAdapterV1(config CheckpointGateApplicationAdapterConfigV1) (*CheckpointGateApplicationAdapterV1, error) {
	if checkpointAdapterNilV1(config.Gates) || config.SubjectOwner.Validate() != nil || config.GateOwner.Validate() != nil || config.SnapshotOwner.Validate() != nil || config.Clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Harness checkpoint Application adapter dependencies are required")
	}
	return &CheckpointGateApplicationAdapterV1{config: config}, nil
}

func (a *CheckpointGateApplicationAdapterV1) AcquireCheckpointGateV1(ctx context.Context, request appcontract.AcquireCheckpointGateRequestV1) (appcontract.CheckpointGateCommitV1, error) {
	now := a.config.Clock()
	if err := request.Validate(now); err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	if err := a.validateExternalV1(request.Subject, harnesscontract.GovernedContractVersionV4, checkpointSubjectSchemaV1(), a.config.SubjectOwner); err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	gate, snapshot, err := a.config.Gates.AcquireCheckpointGateV1(ctx, harnesscontract.AcquireCheckpointGateRequestV1{
		StableID: request.StableID, IntentDigest: request.IntentDigest,
		Run: harnesscontract.RunRef{Scope: request.Scope, RunID: request.RunID}, SessionID: request.Subject.ID,
		ExpectedSessionRevision: request.Subject.Revision, ExpectedSessionDigest: request.Subject.Digest,
		RequestedNotAfter: request.RequestedNotAfter,
	})
	if err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	return a.projectGateV1(request.Subject, gate, snapshot)
}

func (a *CheckpointGateApplicationAdapterV1) BindCheckpointGateRuntimeV1(ctx context.Context, request appcontract.BindCheckpointGateRuntimeRequestV1) (appcontract.CheckpointGateCommitV1, error) {
	now := a.config.Clock()
	if err := request.Gate.Validate(now); err != nil || request.Gate.State != appcontract.CheckpointGateAcquiredV1 || request.Attempt.Validate() != nil || request.Barrier.Validate() != nil || request.EffectCut.Validate() != nil {
		if err != nil {
			return appcontract.CheckpointGateCommitV1{}, err
		}
		return appcontract.CheckpointGateCommitV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCheckpointInconsistent, "Harness checkpoint Gate bind request is invalid")
	}
	gateRef, err := a.gateRefV1(request.Gate.Gate)
	if err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	bound, err := a.config.Gates.BindCheckpointGateRuntimeV1(ctx, harnesscontract.BindCheckpointGateRuntimeRequestV1{Expected: gateRef, Runtime: harnesscontract.CheckpointRuntimeBindingV1{Attempt: request.Attempt, Barrier: request.Barrier, EffectCut: request.EffectCut}})
	if err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	snapshot, err := a.config.Gates.InspectHarnessCheckpointSnapshotV1(ctx, bound.Snapshot)
	if err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	return a.projectGateV1(request.Gate.Subject, bound, snapshot)
}

func (a *CheckpointGateApplicationAdapterV1) InspectCheckpointGateV1(ctx context.Context, ref appcontract.CheckpointExternalExactRefV1) (appcontract.CheckpointGateCommitV1, error) {
	gateRef, err := a.gateRefV1(ref)
	if err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	gate, err := a.config.Gates.InspectCheckpointGateV1(ctx, gateRef)
	if err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	snapshot, err := a.config.Gates.InspectHarnessCheckpointSnapshotV1(ctx, gate.Snapshot)
	if err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	subject := a.subjectRefV1(gate.Request.Run, snapshot.Session)
	return a.projectGateV1(subject, gate, snapshot)
}

func (a *CheckpointGateApplicationAdapterV1) InvalidateCheckpointGateV1(ctx context.Context, ref appcontract.CheckpointExternalExactRefV1) (appcontract.CheckpointGateCommitV1, error) {
	gateRef, err := a.gateRefV1(ref)
	if err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	gate, err := a.config.Gates.InvalidateCheckpointGateV1(ctx, harnesscontract.InvalidateCheckpointGateRequestV1{Expected: gateRef})
	if err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	snapshot, err := a.config.Gates.InspectHarnessCheckpointSnapshotV1(ctx, gate.Snapshot)
	if err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	return a.projectGateV1(a.subjectRefV1(gate.Request.Run, snapshot.Session), gate, snapshot)
}

func (a *CheckpointGateApplicationAdapterV1) ReleaseCheckpointGateV1(ctx context.Context, commit appcontract.CheckpointGateCommitV1, attempt runtimeports.CheckpointAttemptRefV2) (appcontract.CheckpointGateCommitV1, error) {
	now := a.config.Clock()
	if err := commit.Validate(now); err != nil || commit.State != appcontract.CheckpointGateBoundV1 || commit.RuntimeAttempt == nil || commit.RuntimeAttempt.TenantID != attempt.TenantID || commit.RuntimeAttempt.ID != attempt.ID || attempt.Revision < commit.RuntimeAttempt.Revision {
		if err != nil {
			return appcontract.CheckpointGateCommitV1{}, err
		}
		return appcontract.CheckpointGateCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Harness checkpoint Gate release is not exact")
	}
	gateRef, err := a.gateRefV1(commit.Gate)
	if err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	released, err := a.config.Gates.ReleaseCheckpointGateV1(ctx, harnesscontract.ReleaseCheckpointGateRequestV1{Expected: gateRef, TerminalAttempt: attempt})
	if err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	snapshot, err := a.config.Gates.InspectHarnessCheckpointSnapshotV1(ctx, released.Snapshot)
	if err != nil {
		return appcontract.CheckpointGateCommitV1{}, err
	}
	return a.projectGateV1(commit.Subject, released, snapshot)
}

func (a *CheckpointGateApplicationAdapterV1) projectGateV1(subject appcontract.CheckpointExternalExactRefV1, gate harnesscontract.CheckpointGateFactV1, snapshot harnesscontract.HarnessCheckpointSnapshotFactV1) (appcontract.CheckpointGateCommitV1, error) {
	if gate.Validate() != nil || snapshot.Validate() != nil || gate.Snapshot != snapshot.Ref || gate.Request.ExpectedSessionDigest != subject.Digest || gate.Request.ExpectedSessionRevision != subject.Revision || gate.Request.SessionID != subject.ID {
		return appcontract.CheckpointGateCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Harness checkpoint Gate projection is not exact")
	}
	now := a.config.Clock()
	gateRef := a.externalRefV1(gate.Request.Run, harnesscontract.CheckpointGateContractVersionV1, "praxis.harness/checkpoint-gate-fact/v1", "checkpoint_gate_fact_v1", checkpointGateSchemaV1(), a.config.GateOwner, gate.Ref.ID, gate.Ref.Revision, gate.Ref.Digest)
	snapshotRef := a.externalRefV1(gate.Request.Run, harnesscontract.CheckpointGateContractVersionV1, "praxis.harness/checkpoint-snapshot-fact/v1", "checkpoint_snapshot_fact_v1", checkpointSnapshotSchemaV1(), a.config.SnapshotOwner, snapshot.Ref.ID, snapshot.Ref.Revision, snapshot.Ref.Digest)
	result := appcontract.CheckpointGateCommitV1{IntentDigest: gate.Request.IntentDigest, Subject: subject, Gate: gateRef, Snapshot: snapshotRef, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: gate.ExpiresUnixNano}
	switch gate.State {
	case harnesscontract.CheckpointGateAcquiredV1:
		result.State = appcontract.CheckpointGateAcquiredV1
	case harnesscontract.CheckpointGateBoundV1:
		result.State = appcontract.CheckpointGateBoundV1
	case harnesscontract.CheckpointGateInvalidatedV1:
		result.State = appcontract.CheckpointGateInvalidatedV1
	case harnesscontract.CheckpointGateReleasedV1:
		result.State = appcontract.CheckpointGateReleasedV1
	default:
		return appcontract.CheckpointGateCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "Harness checkpoint Gate state is unknown")
	}
	if gate.Runtime != nil {
		attempt, barrier, cut := gate.Runtime.Attempt, gate.Runtime.Barrier, gate.Runtime.EffectCut
		result.RuntimeAttempt, result.RuntimeBarrier, result.RuntimeEffectCut = &attempt, &barrier, &cut
	}
	return appcontract.SealCheckpointGateCommitV1(result, now)
}

func (a *CheckpointGateApplicationAdapterV1) subjectRefV1(run harnesscontract.RunRef, session harnesscontract.GovernedSessionV4) appcontract.CheckpointExternalExactRefV1 {
	return a.externalRefV1(run, harnesscontract.GovernedContractVersionV4, "praxis.harness/governed-session-fact/v4", "governed_session_fact_v4", checkpointSubjectSchemaV1(), a.config.SubjectOwner, session.ID, session.Revision, session.Digest)
}

func (a *CheckpointGateApplicationAdapterV1) externalRefV1(run harnesscontract.RunRef, version, exactSchemaRef, factKind string, schema runtimeports.SchemaRefV2, owner runtimeports.ProviderBindingRefV2, id string, revision core.Revision, digest core.Digest) appcontract.CheckpointExternalExactRefV1 {
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(run.Scope)
	return appcontract.CheckpointExternalExactRefV1{ContractVersion: version, ExactSchemaRef: exactSchemaRef, FactKind: factKind, Schema: schema, Owner: owner, TenantID: run.Scope.Identity.TenantID, ScopeDigest: scopeDigest, RunID: run.RunID, ID: id, Revision: revision, Digest: digest}
}

func (a *CheckpointGateApplicationAdapterV1) gateRefV1(ref appcontract.CheckpointExternalExactRefV1) (harnesscontract.CheckpointGateRefV1, error) {
	if err := a.validateExternalV1(ref, harnesscontract.CheckpointGateContractVersionV1, checkpointGateSchemaV1(), a.config.GateOwner); err != nil {
		return harnesscontract.CheckpointGateRefV1{}, err
	}
	return harnesscontract.CheckpointGateRefV1{ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest}, nil
}

func (a *CheckpointGateApplicationAdapterV1) validateExternalV1(ref appcontract.CheckpointExternalExactRefV1, version string, schema runtimeports.SchemaRefV2, owner runtimeports.ProviderBindingRefV2) error {
	exactSchema, factKind := "praxis.harness/governed-session-fact/v4", "governed_session_fact_v4"
	if version == harnesscontract.CheckpointGateContractVersionV1 {
		switch schema.Name {
		case "checkpoint-gate":
			exactSchema, factKind = "praxis.harness/checkpoint-gate-fact/v1", "checkpoint_gate_fact_v1"
		case "checkpoint-snapshot":
			exactSchema, factKind = "praxis.harness/checkpoint-snapshot-fact/v1", "checkpoint_snapshot_fact_v1"
		}
	}
	if ref.Validate() != nil || ref.ContractVersion != version || ref.ExactSchemaRef != exactSchema || ref.FactKind != factKind || !reflect.DeepEqual(ref.Schema, schema) || ref.Owner != owner {
		return core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "Harness checkpoint external exact ref owner or schema drifted")
	}
	return nil
}

func checkpointSubjectSchemaV1() runtimeports.SchemaRefV2 {
	return checkpointSchemaV1("governed-session", "4.0.0")
}
func checkpointGateSchemaV1() runtimeports.SchemaRefV2 {
	return checkpointSchemaV1("checkpoint-gate", "1.0.0")
}
func checkpointSnapshotSchemaV1() runtimeports.SchemaRefV2 {
	return checkpointSchemaV1("checkpoint-snapshot", "1.0.0")
}
func checkpointSchemaV1(name, version string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.harness", Name: name, Version: version, MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("praxis.harness.schema/" + name + "/" + version))}
}

func checkpointAdapterNilV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

var _ applicationports.CheckpointGatePortV1 = (*CheckpointGateApplicationAdapterV1)(nil)

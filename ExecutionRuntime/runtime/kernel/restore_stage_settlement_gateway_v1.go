package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RestoreStageSettlementGatewayV1 is additive and restore-specific. It does
// not expand Operation Evidence V3 or checkpoint Settlement V5 closed tables.
type RestoreStageSettlementGatewayV1 struct {
	Facts      ports.RestoreStageSettlementFactPortV1
	Governance ports.RestoreStageGovernanceCurrentPortV1
	Domains    ports.RestoreStageDomainResultCurrentReaderV1
	Evidence   ports.EvidenceRecordReaderV2
	Clock      func() time.Time
}

func (g RestoreStageSettlementGatewayV1) SettleRestoreStageV1(ctx context.Context, submission ports.RestoreStageSettlementSubmissionV1) (ports.RestoreStageSettlementRefV1, error) {
	if err := submission.Validate(); err != nil {
		return ports.RestoreStageSettlementRefV1{}, err
	}
	for _, dependency := range []struct {
		value any
		name  string
	}{
		{value: g.Facts, name: "Restore Stage Settlement Owner"},
		{value: g.Governance, name: "Restore Stage Governance current Reader"},
		{value: g.Domains, name: "Sandbox Restore Stage DomainResult current Reader"},
		{value: g.Evidence, name: "Runtime Evidence exact Reader"},
		{value: g.Clock, name: "Restore Stage Settlement clock"},
	} {
		if err := requireCheckpointDependencyV2(dependency.value, dependency.name); err != nil {
			return ports.RestoreStageSettlementRefV1{}, err
		}
	}
	now := g.Clock()
	if now.IsZero() || now.UnixNano() != submission.SettledUnixNano {
		return ports.RestoreStageSettlementRefV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Restore Stage Settlement time is not the current Runtime clock")
	}
	s1, err := g.readRestoreStageSettlementClosureV1(ctx, submission, now)
	if err != nil {
		return ports.RestoreStageSettlementRefV1{}, err
	}
	fresh := g.Clock()
	if fresh.IsZero() || fresh.Before(now) {
		return ports.RestoreStageSettlementRefV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Restore Stage Settlement clock regressed before commit")
	}
	s2, err := g.readRestoreStageSettlementClosureV1(ctx, submission, fresh)
	if err != nil {
		return ports.RestoreStageSettlementRefV1{}, err
	}
	if s1 != s2 {
		return ports.RestoreStageSettlementRefV1{}, restoreStageConflictV1("Restore Stage Settlement S1/S2 closure changed")
	}
	fact, err := ports.SealRestoreStageSettlementFactV1(ports.RestoreStageSettlementFactV1{Submission: submission})
	if err != nil {
		return ports.RestoreStageSettlementRefV1{}, err
	}
	committed, createErr := g.Facts.CreateRestoreStageSettlementV1(ctx, fact)
	if createErr != nil {
		committed, err = g.Facts.InspectRestoreStageSettlementV1(context.WithoutCancel(ctx), submission.ID)
		if err != nil {
			return ports.RestoreStageSettlementRefV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectSettlementMissing, "Restore Stage Settlement create outcome cannot be inspected")
		}
	}
	if committed.Validate() != nil || committed != fact {
		return ports.RestoreStageSettlementRefV1{}, restoreStageConflictV1("Restore Stage Settlement create-once winner differs")
	}
	return committed.RefV1(), nil
}

type restoreStageSettlementSnapshotV1 struct {
	Governance core.Digest
	Domain     core.Digest
	Evidence   core.Digest
}

func (g RestoreStageSettlementGatewayV1) readRestoreStageSettlementClosureV1(ctx context.Context, submission ports.RestoreStageSettlementSubmissionV1, now time.Time) (restoreStageSettlementSnapshotV1, error) {
	request := ports.InspectRestoreStageGovernanceCurrentRequestV1{
		RestoreAttempt: submission.RestoreAttempt, Eligibility: submission.Eligibility, Operation: submission.Operation, EffectID: submission.EffectID,
		Admission: submission.Governance.Admission, Authorization: submission.Governance.Authorization, PermitID: submission.Governance.PermitID,
		DispatchAttempt: submission.Governance.DispatchAttempt, ExecuteEnforcement: submission.Governance.ExecuteEnforcement, SnapshotArtifact: submission.Governance.SnapshotArtifact,
	}
	governance, err := g.Governance.InspectRestoreStageGovernanceCurrentV1(ctx, request)
	if err != nil {
		return restoreStageSettlementSnapshotV1{}, err
	}
	if err := governance.Validate(now); err != nil {
		return restoreStageSettlementSnapshotV1{}, err
	}
	if governance != submission.Governance {
		return restoreStageSettlementSnapshotV1{}, restoreStageConflictV1("Restore Stage Settlement governance projection is not exact current")
	}
	domain, err := g.Domains.InspectRestoreStageDomainResultCurrentV1(ctx, submission.DomainResult)
	if err != nil {
		return restoreStageSettlementSnapshotV1{}, err
	}
	if err := domain.Validate(now); err != nil {
		return restoreStageSettlementSnapshotV1{}, err
	}
	if !ports.SameRestoreStageDomainResultFactRefV1(domain.Fact, submission.DomainResult) {
		return restoreStageSettlementSnapshotV1{}, restoreStageConflictV1("Restore Stage Settlement DomainResult projection drifted")
	}
	record, err := g.Evidence.InspectRecord(ctx, submission.Evidence)
	if err != nil {
		return restoreStageSettlementSnapshotV1{}, err
	}
	if record.Ref != submission.Evidence {
		return restoreStageSettlementSnapshotV1{}, restoreStageConflictV1("Restore Stage Settlement Evidence exact ref drifted")
	}
	if err := ports.ValidateRestoreStageEvidenceRecordV1(record, submission.DomainResult); err != nil {
		return restoreStageSettlementSnapshotV1{}, err
	}
	return restoreStageSettlementSnapshotV1{Governance: governance.ProjectionDigest, Domain: domain.ProjectionDigest, Evidence: record.Ref.RecordDigest}, nil
}

func (g RestoreStageSettlementGatewayV1) InspectRestoreStageSettlementV1(ctx context.Context, id string) (ports.RestoreStageSettlementFactV1, error) {
	if err := requireCheckpointDependencyV2(g.Facts, "Restore Stage Settlement Owner"); err != nil {
		return ports.RestoreStageSettlementFactV1{}, err
	}
	fact, err := g.Facts.InspectRestoreStageSettlementV1(ctx, id)
	if err != nil {
		return ports.RestoreStageSettlementFactV1{}, err
	}
	return fact, fact.Validate()
}

func (g RestoreStageSettlementGatewayV1) InspectCurrentRestoreStageSettlementV1(ctx context.Context, operation ports.OperationSubjectV3, effectID core.EffectIntentID) (ports.RestoreStageSettlementRefV1, error) {
	if operation.Validate() != nil || effectID == "" {
		return ports.RestoreStageSettlementRefV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore Stage Settlement current Inspect coordinates are incomplete")
	}
	if err := requireCheckpointDependencyV2(g.Facts, "Restore Stage Settlement Owner"); err != nil {
		return ports.RestoreStageSettlementRefV1{}, err
	}
	fact, err := g.Facts.InspectRestoreStageSettlementByEffectV1(ctx, operation, effectID)
	if err != nil {
		return ports.RestoreStageSettlementRefV1{}, err
	}
	if fact.Validate() != nil || !ports.SameOperationSubjectV3(fact.Submission.Operation, operation) || fact.Submission.EffectID != effectID {
		return ports.RestoreStageSettlementRefV1{}, restoreStageConflictV1("Restore Stage Settlement current Inspect drifted")
	}
	return fact.RefV1(), nil
}

var _ ports.RestoreStageSettlementGovernancePortV1 = RestoreStageSettlementGatewayV1{}

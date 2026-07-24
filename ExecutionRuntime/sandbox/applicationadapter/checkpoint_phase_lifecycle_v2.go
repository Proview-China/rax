package applicationadapter

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// CheckpointPhaseExecutionPlanV2 is a host-owned exact association. It grants
// no authority: every Runtime and Sandbox current input is independently read
// again before either Provider execution or Owner CAS.
type CheckpointPhaseExecutionPlanV2 struct {
	ContractVersion      string                                             `json:"contract_version"`
	Revision             runtimecore.Revision                               `json:"revision"`
	Work                 appcontract.CheckpointParticipantWorkRequestV1     `json:"work"`
	ParticipantBootstrap contract.CheckpointParticipantFact                 `json:"participant_bootstrap"`
	Reservation          contract.CheckpointPhaseReservation                `json:"reservation"`
	Provider             CheckpointProviderPlanV1                           `json:"provider"`
	SettlementID         string                                             `json:"settlement_id"`
	OwnerCandidate       *appcontract.CheckpointParticipantOwnerCandidateV1 `json:"owner_candidate,omitempty"`
	EvidenceExternal     appcontract.CheckpointExternalExactRefV1           `json:"evidence_external"`
	SnapshotCapture      *CheckpointSnapshotCapturePlanV2                   `json:"snapshot_capture,omitempty"`
	ExpiresUnixNano      int64                                              `json:"expires_unix_nano"`
	Digest               runtimecore.Digest                                 `json:"digest"`
}

const CheckpointPhaseExecutionPlanContractVersionV2 = "praxis.sandbox/checkpoint-phase-execution-plan/v2"

func SealCheckpointPhaseExecutionPlanV2(plan CheckpointPhaseExecutionPlanV2, now time.Time) (CheckpointPhaseExecutionPlanV2, error) {
	if plan.SnapshotCapture != nil {
		capture := plan.SnapshotCapture.Clone()
		slices.Sort(capture.Included)
		slices.Sort(capture.DeclaredExcluded)
		slices.SortFunc(capture.ResidualRefs, compareCheckpointSnapshotRefV2)
		plan.SnapshotCapture = &capture
	}
	plan.ContractVersion = CheckpointPhaseExecutionPlanContractVersionV2
	plan.Revision = 1
	plan.ExpiresUnixNano = minimumInt64(plan.Work.NotAfter, plan.Reservation.Meta.ExpiresUnixNano, plan.Provider.NotAfter.UnixNano(), plan.ParticipantBootstrap.Meta.ExpiresUnixNano)
	plan.Digest = ""
	digest, err := checkpointPhaseExecutionPlanDigestV2(plan)
	if err != nil {
		return CheckpointPhaseExecutionPlanV2{}, err
	}
	plan.Digest = digest
	return plan, plan.ValidateCurrent(now)
}

func (plan CheckpointPhaseExecutionPlanV2) ValidateCurrent(now time.Time) error {
	if plan.ContractVersion != CheckpointPhaseExecutionPlanContractVersionV2 || plan.Revision != 1 || plan.ExpiresUnixNano <= 0 || now.IsZero() || !now.Before(time.Unix(0, plan.ExpiresUnixNano)) || plan.Digest.Validate() != nil {
		return errors.New("checkpoint phase execution plan envelope is incomplete or stale")
	}
	phase, err := checkpointRuntimePhaseV2(plan.Reservation.Phase)
	if err != nil {
		return err
	}
	if err := validateCheckpointPhaseExecutionPlanV2(plan, plan.Work.Attempt, plan.Work.Participant, phase, now); err != nil {
		return err
	}
	digest, err := checkpointPhaseExecutionPlanDigestV2(plan)
	if err != nil || digest != plan.Digest {
		return fmt.Errorf("%w: checkpoint phase execution plan digest drifted", ports.ErrConflict)
	}
	return nil
}

func checkpointPhaseExecutionPlanDigestV2(plan CheckpointPhaseExecutionPlanV2) (runtimecore.Digest, error) {
	plan.Digest = ""
	return runtimecore.CanonicalJSONDigest("praxis.sandbox.checkpoint-phase-execution-plan", "2.0.0", "CheckpointPhaseExecutionPlanV2", plan)
}

// CheckpointPhaseExecutionPlanReaderV2 accepts only exact Attempt,
// Participant, and phase coordinates. A caller cannot pass a snapshot or a
// current=true assertion through this seam.
type CheckpointPhaseExecutionPlanReaderV2 interface {
	InspectCheckpointPhaseExecutionPlanV2(context.Context, runtimeports.CheckpointAttemptRefV2, runtimeports.CheckpointParticipantRefV2, runtimeports.CheckpointParticipantPhaseV2) (CheckpointPhaseExecutionPlanV2, error)
}

type GovernedCheckpointParticipantPhaseLifecycleConfigV2 struct {
	Composition *CheckpointProductionCompositionV2
	Plans       CheckpointPhaseExecutionPlanReaderV2
	Capture     *CheckpointSnapshotCaptureV2
	Workspace   ports.WorkspaceCheckpointParticipantOwnerPortV2
}

// GovernedCheckpointParticipantPhaseLifecycleV2 closes the production phase
// chain. Begin/Reservation never authorize a Provider call: the actual-point
// boundary still performs the two Runtime enforcement reads immediately
// before Provider Prepare and ExecutePrepared.
type GovernedCheckpointParticipantPhaseLifecycleV2 struct {
	composition *CheckpointProductionCompositionV2
	plans       CheckpointPhaseExecutionPlanReaderV2
	capture     *CheckpointSnapshotCaptureV2
	workspace   ports.WorkspaceCheckpointParticipantOwnerPortV2
}

func NewGovernedCheckpointParticipantPhaseLifecycleV2(config GovernedCheckpointParticipantPhaseLifecycleConfigV2) (*GovernedCheckpointParticipantPhaseLifecycleV2, error) {
	if config.Composition == nil || config.Composition.Store == nil || config.Composition.Controller == nil || config.Composition.ActualPoint == nil || config.Composition.DomainResultOwner == nil || config.Composition.PhaseResultCurrent == nil || config.Composition.RuntimeReservations == nil || config.Composition.RuntimeSettlements == nil || config.Composition.Clock == nil || nilLike(config.Plans) || config.Capture == nil || nilLike(config.Workspace) {
		return nil, errors.New("checkpoint phase lifecycle requires the full production composition and exact plan Reader")
	}
	return &GovernedCheckpointParticipantPhaseLifecycleV2{composition: config.Composition, plans: config.Plans, capture: config.Capture, workspace: config.Workspace}, nil
}

func (a *GovernedCheckpointParticipantPhaseLifecycleV2) PrepareCheckpointParticipantPhaseV1(ctx context.Context, work appcontract.CheckpointParticipantWorkRequestV1) (runtimeports.CheckpointParticipantPhaseClosureRefV2, error) {
	plan, err := a.readPlan(ctx, work.Attempt, work.Participant, runtimeports.CheckpointPhasePrepareV2)
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	if !reflect.DeepEqual(plan.Work, work) {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, fmt.Errorf("%w: checkpoint prepare plan changed Application work", ports.ErrConflict)
	}
	closure, err := a.executePhase(ctx, plan, nil)
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	if plan.SnapshotCapture == nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, fmt.Errorf("%w: checkpoint prepare Snapshot capture plan is absent", ports.ErrConflict)
	}
	artifact, err := a.capture.CapturePreparedCheckpointSnapshotV2(ctx, plan.Reservation.Meta.Ref(), plan.Work, *plan.SnapshotCapture)
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	phase, err := a.composition.Store.InspectCheckpointPhaseFactByReservation(ctx, plan.Reservation.Meta.Ref())
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	request, err := checkpointWorkspacePreparationRequestV2(plan, phase, artifact, a.composition.Clock())
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	if _, err := a.workspace.PrepareWorkspaceCheckpointParticipantV2(ctx, &request); err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	return closure, nil
}

func (a *GovernedCheckpointParticipantPhaseLifecycleV2) InspectCheckpointParticipantPrepareV1(ctx context.Context, attempt runtimeports.CheckpointAttemptRefV2, participant runtimeports.CheckpointParticipantRefV2) (runtimeports.CheckpointParticipantPhaseClosureRefV2, error) {
	plan, err := a.readPlan(ctx, attempt, participant, runtimeports.CheckpointPhasePrepareV2)
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	return a.inspectPhase(ctx, plan)
}

func (a *GovernedCheckpointParticipantPhaseLifecycleV2) CommitCheckpointParticipantPhaseV1(ctx context.Context, work appcontract.CheckpointParticipantWorkRequestV1, prepared runtimeports.CheckpointParticipantPhaseClosureRefV2, candidate appcontract.CheckpointParticipantOwnerCandidateV1) (appcontract.CheckpointParticipantCommitV1, error) {
	if prepared.Validate() != nil || prepared.Phase != runtimeports.CheckpointPhasePrepareV2 {
		return appcontract.CheckpointParticipantCommitV1{}, fmt.Errorf("%w: checkpoint commit lacks exact prepared closure", ports.ErrConflict)
	}
	plan, err := a.readPlan(ctx, work.Attempt, work.Participant, runtimeports.CheckpointPhaseCommitV2)
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	if !reflect.DeepEqual(plan.Work, work) || plan.OwnerCandidate == nil || !reflect.DeepEqual(*plan.OwnerCandidate, candidate) {
		return appcontract.CheckpointParticipantCommitV1{}, fmt.Errorf("%w: checkpoint commit plan changed work or Owner candidate", ports.ErrConflict)
	}
	terminal, err := a.executePhase(ctx, plan, &prepared)
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	preparePlan, err := a.readPlan(ctx, work.Attempt, work.Participant, runtimeports.CheckpointPhasePrepareV2)
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	return checkpointParticipantCommitFromPlansV2(prepared, terminal, candidate, []appcontract.CheckpointExternalExactRefV1{preparePlan.EvidenceExternal, plan.EvidenceExternal})
}

func (a *GovernedCheckpointParticipantPhaseLifecycleV2) InspectCheckpointParticipantPhaseV1(ctx context.Context, attempt runtimeports.CheckpointAttemptRefV2, participant runtimeports.CheckpointParticipantRefV2) (appcontract.CheckpointParticipantCommitV1, error) {
	preparePlan, err := a.readPlan(ctx, attempt, participant, runtimeports.CheckpointPhasePrepareV2)
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	prepared, err := a.inspectPhase(ctx, preparePlan)
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	commitPlan, err := a.readPlan(ctx, attempt, participant, runtimeports.CheckpointPhaseCommitV2)
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	if commitPlan.OwnerCandidate == nil {
		return appcontract.CheckpointParticipantCommitV1{}, fmt.Errorf("%w: checkpoint commit Owner candidate is absent", ports.ErrNotFound)
	}
	terminal, err := a.inspectPhase(ctx, commitPlan)
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	return checkpointParticipantCommitFromPlansV2(prepared, terminal, *commitPlan.OwnerCandidate, []appcontract.CheckpointExternalExactRefV1{preparePlan.EvidenceExternal, commitPlan.EvidenceExternal})
}

func (a *GovernedCheckpointParticipantPhaseLifecycleV2) readPlan(ctx context.Context, attempt runtimeports.CheckpointAttemptRefV2, participant runtimeports.CheckpointParticipantRefV2, phase runtimeports.CheckpointParticipantPhaseV2) (CheckpointPhaseExecutionPlanV2, error) {
	if a == nil || nilLike(ctx) || attempt.Validate() != nil || participant.Validate() != nil {
		return CheckpointPhaseExecutionPlanV2{}, errors.New("checkpoint phase plan coordinates are invalid")
	}
	plan, err := a.plans.InspectCheckpointPhaseExecutionPlanV2(ctx, attempt, participant, phase)
	if err != nil {
		return CheckpointPhaseExecutionPlanV2{}, err
	}
	if err := plan.ValidateCurrent(a.composition.Clock()); err != nil {
		return CheckpointPhaseExecutionPlanV2{}, err
	}
	if err := validateCheckpointPhaseExecutionPlanV2(plan, attempt, participant, phase, a.composition.Clock()); err != nil {
		return CheckpointPhaseExecutionPlanV2{}, err
	}
	return plan, nil
}

func (a *GovernedCheckpointParticipantPhaseLifecycleV2) executePhase(ctx context.Context, plan CheckpointPhaseExecutionPlanV2, previous *runtimeports.CheckpointParticipantPhaseClosureRefV2) (runtimeports.CheckpointParticipantPhaseClosureRefV2, error) {
	now := a.composition.Clock()
	runtimeReservation, err := a.inspectRuntimeReservation(ctx, plan, now)
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	if recovered, inspectErr := a.inspectPhase(ctx, plan); inspectErr == nil {
		return recovered, nil
	}
	if plan.Reservation.Phase == contract.CheckpointPhasePrepare {
		if err := a.composition.Store.CreateCheckpointParticipant(ctx, plan.ParticipantBootstrap); err != nil {
			return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
		}
	}
	if _, err := a.composition.Controller.ReservePhase(ctx, &plan.Reservation); err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	projection, result, err := a.composition.ActualPoint.ExecuteAndBindCheckpointPhaseV2(ctx, plan.Reservation.Meta.Ref(), plan.Reservation.Phase, plan.Provider)
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	domain, err := a.composition.DomainResultOwner.RecordCheckpointPhaseDomainResultV2(ctx, &contract.RecordCheckpointPhaseDomainResultRequestV2{ReservationRef: plan.Reservation.Meta.Ref(), ExpectedProjectionDigest: projection.ProjectionDigest, RequestedNotAfter: plan.Work.NotAfter})
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	mappedDomain := checkpointRuntimeDomainResultV2(domain, runtimeReservation)
	domainCurrent, err := a.composition.DomainResultCurrent.ReadCheckpointDomainResultCurrentV2(ctx, mappedDomain)
	if err != nil || domainCurrent.Ref != mappedDomain {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, fmt.Errorf("%w: checkpoint DomainResult is not exact current", ports.ErrConflict)
	}
	phaseRef, err := checkpointRuntimePhaseResultRefV2(domain, runtimeReservation)
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	phaseCurrent, err := a.composition.PhaseResultCurrent.InspectCheckpointParticipantPhaseCurrentV2(ctx, phaseRef)
	if err != nil || phaseCurrent.Ref != phaseRef {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, fmt.Errorf("%w: checkpoint phase result is not exact current", ports.ErrConflict)
	}
	if !checkpointEvidenceExternalMatchesV2(plan.EvidenceExternal, result.Consumption) {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, fmt.Errorf("%w: checkpoint Evidence external mapping drifted", ports.ErrConflict)
	}
	legacy := result.ExecuteCurrent.Dispatch.Record.Permit.LegacyPermit
	submission := runtimeports.OperationCheckpointRestoreSettlementSubmissionV5{
		ID: plan.SettlementID, Operation: runtimeReservation.Operation, OperationDigest: runtimeReservation.OperationDigest,
		EffectID: runtimeReservation.EffectID, ExpectedEffectRevision: legacy.IntentRevision,
		CheckpointAttempt: runtimeReservation.Attempt, Phase: runtimeReservation.Phase, ParticipantFact: phaseRef,
		Reservation: runtimeReservation.Ref, DomainResult: mappedDomain, Evidence: result.Consumption,
		Handoff: result.Handoff, DispatchAttempt: result.Handoff.Attempt, Enforcement: result.ExecuteCurrent.Phase,
		Owner: runtimeReservation.OwnerBinding, SettledUnixNano: a.composition.Clock().UnixNano(),
	}
	if err := submission.Validate(); err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	settlement, settleErr := a.composition.RuntimeSettlements.SettleCheckpointPhaseV5(ctx, submission)
	inspection, inspectErr := a.composition.RuntimeSettlements.InspectCheckpointPhaseSettlementCurrentV5(context.WithoutCancel(ctx), runtimeports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5{Operation: submission.Operation, EffectID: submission.EffectID})
	if inspectErr != nil || inspection.Validate() != nil || !reflect.DeepEqual(inspection.Bundle.Submission, submission) {
		if settleErr != nil {
			return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, errors.Join(settleErr, inspectErr)
		}
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, fmt.Errorf("%w: Runtime checkpoint Settlement is not exact current", ports.ErrConflict)
	}
	if settlement != inspection.Bundle.Settlement {
		if settleErr == nil {
			return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, fmt.Errorf("%w: Runtime checkpoint Settlement reply differs from current", ports.ErrConflict)
		}
		settlement = inspection.Bundle.Settlement
	}
	localSettlement := contract.Ref{ID: settlement.ID, Revision: uint64(settlement.Revision), Digest: trimCheckpointRuntimeDigestV2(settlement.Digest)}
	fact, err := a.composition.DomainResultOwner.ApplyCheckpointPhaseSettlementV2(ctx, &contract.CheckpointPhaseApplySettlementV2{DomainResultRef: domain.ExactRef(), RuntimeSettlementRef: localSettlement})
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	closure, err := checkpointRuntimeClosureFromAppliedV2(runtimeReservation, phaseRef, mappedDomain, result.Consumption, settlement, fact)
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	if previous != nil && (runtimeReservation.PreviousPhase == nil || *runtimeReservation.PreviousPhase != *previous) {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, fmt.Errorf("%w: checkpoint successor replaced prepared closure", ports.ErrConflict)
	}
	return closure, nil
}

func (a *GovernedCheckpointParticipantPhaseLifecycleV2) inspectPhase(ctx context.Context, plan CheckpointPhaseExecutionPlanV2) (runtimeports.CheckpointParticipantPhaseClosureRefV2, error) {
	now := a.composition.Clock()
	runtimeReservation, err := a.inspectRuntimeReservation(ctx, plan, now)
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	reservation, err := a.composition.Store.InspectCheckpointPhaseReservation(ctx, plan.Reservation.Meta.Ref())
	if err != nil || !reflect.DeepEqual(reservation, plan.Reservation) {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, fmt.Errorf("%w: checkpoint Sandbox reservation is not exact", ports.ErrConflict)
	}
	domain, err := a.composition.Store.InspectCheckpointPhaseDomainResultByReservationV2(ctx, reservation.Meta.Ref())
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	if err := domain.ValidateCurrent(now); err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, fmt.Errorf("%w: checkpoint DomainResult is stale", ports.ErrStale)
	}
	mappedDomain := checkpointRuntimeDomainResultV2(domain, runtimeReservation)
	inspection, err := a.composition.RuntimeSettlements.InspectCheckpointPhaseSettlementCurrentV5(ctx, runtimeports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5{Operation: runtimeReservation.Operation, EffectID: runtimeReservation.EffectID})
	if err != nil || inspection.Validate() != nil || inspection.Bundle.Submission.DomainResult != mappedDomain || inspection.Bundle.Submission.Reservation != runtimeReservation.Ref {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, fmt.Errorf("%w: checkpoint Runtime Settlement cannot be recovered exactly", ports.ErrUnknownOutcome)
	}
	fact, err := a.composition.Store.InspectCheckpointPhaseFactByReservation(ctx, reservation.Meta.Ref())
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	if err := fact.ValidateCurrent(now); err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, fmt.Errorf("%w: checkpoint phase Fact is stale", ports.ErrStale)
	}
	phaseRef := inspection.Bundle.Submission.ParticipantFact
	if !checkpointEvidenceExternalMatchesV2(plan.EvidenceExternal, inspection.Bundle.Submission.Evidence) {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, fmt.Errorf("%w: checkpoint recovered Evidence mapping drifted", ports.ErrConflict)
	}
	return checkpointRuntimeClosureFromAppliedV2(runtimeReservation, phaseRef, mappedDomain, inspection.Bundle.Submission.Evidence, inspection.Bundle.Settlement, fact)
}

func (a *GovernedCheckpointParticipantPhaseLifecycleV2) inspectRuntimeReservation(ctx context.Context, plan CheckpointPhaseExecutionPlanV2, now time.Time) (runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2, error) {
	phase, err := checkpointRuntimePhaseV2(plan.Reservation.Phase)
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2{}, err
	}
	expected := runtimeports.CheckpointParticipantPhaseReservationRefV2{ID: plan.Reservation.Meta.ID, Revision: runtimecore.Revision(plan.Reservation.Meta.Revision), Digest: checkpointRuntimeDigestV2(plan.Reservation.Meta.Digest), ExpiresUnixNano: plan.Reservation.Meta.ExpiresUnixNano}
	current, err := a.composition.RuntimeReservations.InspectCheckpointParticipantPhaseReservationCurrentV2(ctx, expected, phase)
	if err != nil || current.Validate(now) != nil {
		return runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2{}, fmt.Errorf("%w: Runtime checkpoint Reservation is unavailable", ports.ErrStale)
	}
	if current.Ref != plan.Provider.Reservation || current.Attempt != plan.Work.Attempt || current.Barrier != plan.Work.Barrier || current.EffectCut != plan.Work.EffectCut || current.Participant != plan.Work.Participant || current.Phase != plan.Provider.Phase || current.EffectID != plan.Provider.Prepare.EffectID || !runtimeports.SameOperationSubjectV3(current.Operation, plan.Provider.Prepare.Operation) {
		return runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint plan crosses Runtime Reservation", ports.ErrConflict)
	}
	return current, nil
}

func validateCheckpointPhaseExecutionPlanV2(plan CheckpointPhaseExecutionPlanV2, attempt runtimeports.CheckpointAttemptRefV2, participant runtimeports.CheckpointParticipantRefV2, phase runtimeports.CheckpointParticipantPhaseV2, now time.Time) error {
	if plan.Work.Validate(now) != nil || plan.Work.Attempt != attempt || plan.Work.Participant != participant || plan.ParticipantBootstrap.ValidateCurrent(now) != nil || plan.Reservation.ValidateCurrent(now) != nil || validateCheckpointProviderPlanV1(plan.Provider) != nil || strings.TrimSpace(plan.SettlementID) == "" || plan.EvidenceExternal.Validate() != nil || plan.ExpiresUnixNano > plan.Work.NotAfter || plan.ExpiresUnixNano > plan.Reservation.Meta.ExpiresUnixNano || plan.ExpiresUnixNano > plan.Provider.NotAfter.UnixNano() || plan.ExpiresUnixNano > plan.ParticipantBootstrap.Meta.ExpiresUnixNano {
		return errors.New("checkpoint phase execution plan is incomplete or stale")
	}
	localPhase, err := checkpointLocalPhaseV2(phase)
	if err != nil || plan.Reservation.Phase != localPhase || plan.Provider.Phase != phase || plan.Reservation.TenantID != string(attempt.TenantID) || plan.Reservation.AttemptID != attempt.ID || plan.Reservation.ParticipantRef.ID != participant.ID || checkpointRuntimeDigestV2(plan.Reservation.ParticipantRef.Digest) != participant.Digest || plan.Provider.CheckpointAttempt != attempt || plan.Provider.Barrier != plan.Work.Barrier || plan.Provider.EffectCut != plan.Work.EffectCut {
		return fmt.Errorf("%w: checkpoint phase execution plan identity drifted", ports.ErrConflict)
	}
	if phase == runtimeports.CheckpointPhasePrepareV2 {
		if plan.OwnerCandidate != nil || plan.SnapshotCapture == nil || plan.SnapshotCapture.ValidateCurrent(now) != nil || plan.Reservation.PreviousPresence != contract.CheckpointAbsent {
			return errors.New("checkpoint prepare plan cannot contain a commit candidate or predecessor")
		}
	} else if phase == runtimeports.CheckpointPhaseCommitV2 {
		if plan.OwnerCandidate == nil || plan.SnapshotCapture != nil || plan.OwnerCandidate.Validate(plan.Work, now) != nil || plan.Reservation.PreviousPresence != contract.CheckpointPresent {
			return errors.New("checkpoint commit plan requires exact Owner candidate and predecessor")
		}
	}
	return nil
}

func checkpointRuntimeDomainResultV2(domain contract.CheckpointPhaseDomainResultV2, reservation runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2) runtimeports.CheckpointParticipantDomainResultRefV2 {
	return runtimeports.CheckpointParticipantDomainResultRefV2{ID: domain.Meta.ID, Revision: runtimecore.Revision(domain.Meta.Revision), Kind: "praxis.sandbox/checkpoint-phase-domain-result", Attempt: reservation.Attempt, Participant: reservation.Participant, Phase: reservation.Phase, Operation: reservation.Operation, OperationDigest: reservation.OperationDigest, Digest: checkpointRuntimeDigestV2(domain.Meta.Digest)}
}

func checkpointRuntimePhaseResultRefV2(domain contract.CheckpointPhaseDomainResultV2, reservation runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2) (runtimeports.CheckpointParticipantPhaseRefV2, error) {
	state := runtimeports.CheckpointParticipantUnknownV2
	switch domain.State {
	case contract.CheckpointPhasePrepared:
		state = runtimeports.CheckpointParticipantPreparedV2
	case contract.CheckpointPhaseCommitted:
		state = runtimeports.CheckpointParticipantCommittedV2
	case contract.CheckpointPhaseAborted:
		state = runtimeports.CheckpointParticipantAbortedV2
	case contract.CheckpointPhaseFailed:
		state = runtimeports.CheckpointParticipantFailedV2
	case contract.CheckpointPhaseNotApplied:
		state = runtimeports.CheckpointParticipantNotAppliedV2
	case contract.CheckpointPhaseUnknown, contract.CheckpointPhaseIndeterminate:
	default:
		return runtimeports.CheckpointParticipantPhaseRefV2{}, errors.New("checkpoint DomainResult state cannot form a Runtime phase ref")
	}
	digest, err := runtimecore.CanonicalJSONDigest("praxis.sandbox.checkpoint-phase-result", "2.0.0", "CheckpointParticipantPhaseResultRefV2", struct {
		Domain      contract.SnapshotArtifactExactRefV2
		Reservation runtimeports.CheckpointParticipantPhaseReservationRefV2
		Phase       runtimeports.CheckpointParticipantPhaseV2
		State       runtimeports.CheckpointParticipantPhaseStateV2
	}{domain.ExactRef(), reservation.Ref, reservation.Phase, state})
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseRefV2{}, err
	}
	value := runtimeports.CheckpointParticipantPhaseRefV2{ID: domain.Meta.ID + "-owner-phase-result", Revision: runtimecore.Revision(domain.Meta.Revision), Phase: reservation.Phase, State: state, Digest: digest}
	return value, value.Validate()
}

func checkpointRuntimeClosureFromAppliedV2(reservation runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2, phase runtimeports.CheckpointParticipantPhaseRefV2, domain runtimeports.CheckpointParticipantDomainResultRefV2, evidence runtimeports.CheckpointRestoreEvidenceConsumptionRefV1, settlement runtimeports.OperationCheckpointRestoreSettlementRefV5, fact contract.CheckpointPhaseFact) (runtimeports.CheckpointParticipantPhaseClosureRefV2, error) {
	apply := runtimeports.CheckpointParticipantApplySettlementRefV2{ID: fact.ApplySettlementRef.ID, Revision: runtimecore.Revision(fact.ApplySettlementRef.Revision), Participant: reservation.Participant, Phase: reservation.Phase, SettlementID: settlement.ID, Digest: checkpointRuntimeDigestV2(fact.ApplySettlementRef.Digest)}
	value := runtimeports.CheckpointParticipantPhaseClosureRefV2{ID: fact.Meta.ID + "-runtime-closure", Phase: reservation.Phase, Reservation: reservation.Ref, PhaseFact: phase, DomainResult: domain, Evidence: evidence, Settlement: settlement, ApplySettlement: apply}
	digest, err := value.DigestV2()
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseClosureRefV2{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

func checkpointParticipantCommitFromPlansV2(prepared, terminal runtimeports.CheckpointParticipantPhaseClosureRefV2, candidate appcontract.CheckpointParticipantOwnerCandidateV1, evidence []appcontract.CheckpointExternalExactRefV1) (appcontract.CheckpointParticipantCommitV1, error) {
	closure := runtimeports.CheckpointParticipantClosureRefV2{ID: terminal.ID + "-participant", Participant: terminal.DomainResult.Participant, Prepare: prepared, Terminal: &terminal}
	digest, err := closure.DigestV2()
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	closure.Digest = digest
	result := appcontract.CheckpointParticipantCommitV1{RuntimeClosure: closure, ParticipantFact: candidate.ParticipantFact, Snapshot: candidate.Snapshot, Coverage: candidate.Coverage, Evidence: append([]appcontract.CheckpointExternalExactRefV1(nil), evidence...), Residuals: []appcontract.CheckpointExternalExactRefV1{}}
	return result, result.Validate(candidate.Participant)
}

func checkpointEvidenceExternalMatchesV2(external appcontract.CheckpointExternalExactRefV1, evidence runtimeports.CheckpointRestoreEvidenceConsumptionRefV1) bool {
	return external.ID == evidence.ID && external.Revision == runtimecore.Revision(evidence.Revision) && external.Digest == evidence.Digest
}

func checkpointRuntimePhaseV2(phase contract.CheckpointPhase) (runtimeports.CheckpointParticipantPhaseV2, error) {
	switch phase {
	case contract.CheckpointPhasePrepare:
		return runtimeports.CheckpointPhasePrepareV2, nil
	case contract.CheckpointPhaseCommit:
		return runtimeports.CheckpointPhaseCommitV2, nil
	case contract.CheckpointPhaseAbort:
		return runtimeports.CheckpointPhaseAbortV2, nil
	default:
		return "", errors.New("checkpoint phase is invalid")
	}
}

func checkpointLocalPhaseV2(phase runtimeports.CheckpointParticipantPhaseV2) (contract.CheckpointPhase, error) {
	switch phase {
	case runtimeports.CheckpointPhasePrepareV2:
		return contract.CheckpointPhasePrepare, nil
	case runtimeports.CheckpointPhaseCommitV2:
		return contract.CheckpointPhaseCommit, nil
	case runtimeports.CheckpointPhaseAbortV2:
		return contract.CheckpointPhaseAbort, nil
	default:
		return "", errors.New("Runtime checkpoint phase is invalid")
	}
}

func checkpointRuntimeDigestV2(value string) runtimecore.Digest {
	if strings.HasPrefix(value, "sha256:") {
		return runtimecore.Digest(value)
	}
	return runtimecore.Digest("sha256:" + value)
}

func trimCheckpointRuntimeDigestV2(value runtimecore.Digest) string {
	return strings.TrimPrefix(string(value), "sha256:")
}

func checkpointWorkspacePreparationRequestV2(plan CheckpointPhaseExecutionPlanV2, phase contract.CheckpointPhaseFact, artifact contract.CommitSnapshotArtifactResultV2, now time.Time) (contract.PrepareWorkspaceCheckpointParticipantRequestV2, error) {
	if plan.SnapshotCapture == nil || phase.ValidateShape() != nil || artifact.Fact.ValidateShape() != nil || artifact.CurrentIndex.ValidateShape() != nil || artifact.CurrentIndex.ArtifactFactRef.Ref == nil || !contract.SameSnapshotArtifactExactRef(*artifact.CurrentIndex.ArtifactFactRef.Ref, artifact.Fact.ExactRef()) {
		return contract.PrepareWorkspaceCheckpointParticipantRequestV2{}, fmt.Errorf("%w: checkpoint workspace preparation lacks exact applied facts", ports.ErrConflict)
	}
	request := contract.PrepareWorkspaceCheckpointParticipantRequestV2{
		StableID: plan.SnapshotCapture.WorkspaceStableID, TenantID: string(plan.Work.Attempt.TenantID), ScopeDigest: trimCheckpointRuntimeDigestV2(plan.Work.Gate.ScopeDigest), RunID: string(plan.Work.Gate.RunID),
		CheckpointAttemptRef: checkpointLocalRefV2(plan.Work.Attempt.ID, uint64(plan.Work.Attempt.Revision), plan.Work.Attempt.Digest),
		BarrierRef:           checkpointLocalRefV2(plan.Work.Barrier.ID, uint64(plan.Work.Barrier.Revision), plan.Work.Barrier.Digest),
		EffectCutRef:         checkpointLocalRefV2(plan.Work.EffectCut.ID, uint64(plan.Work.EffectCut.Revision), plan.Work.EffectCut.Digest),
		ParticipantID:        plan.Work.Participant.ID, ParticipantDigest: trimCheckpointRuntimeDigestV2(plan.Work.Participant.Digest), PreparedPhaseFactRef: phase.Meta.Ref(),
		SnapshotArtifactFactRef: artifact.Fact.ExactRef(), CoveragePolicyRef: plan.SnapshotCapture.CoveragePolicyRef, RequestedNotAfter: minimumInt64(plan.Work.NotAfter, artifact.Fact.Meta.ExpiresUnixNano, artifact.CurrentIndex.CurrentIndexRef.ExpiresUnixNano),
	}
	return request, request.ValidateCurrent(now)
}

func checkpointLocalRefV2(id string, revision uint64, digest runtimecore.Digest) contract.Ref {
	return contract.Ref{ID: id, Revision: revision, Digest: trimCheckpointRuntimeDigestV2(digest)}
}

var _ CheckpointParticipantPhaseLifecyclePortV1 = (*GovernedCheckpointParticipantPhaseLifecycleV2)(nil)

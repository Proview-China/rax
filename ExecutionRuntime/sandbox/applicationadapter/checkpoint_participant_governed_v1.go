package applicationadapter

import (
	"context"
	"reflect"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// CheckpointParticipantPhaseLifecyclePortV1 is the Sandbox-owned sequencing
// seam between Application coordination and the already-versioned Runtime
// checkpoint phase, Evidence, and Settlement contracts. Implementations must
// complete the governed prepare phase before exposing Owner current state.
// It is not a Provider or a Runtime Fact writer.
type CheckpointParticipantPhaseLifecyclePortV1 interface {
	PrepareCheckpointParticipantPhaseV1(context.Context, appcontract.CheckpointParticipantWorkRequestV1) (runtimeports.CheckpointParticipantPhaseClosureRefV2, error)
	InspectCheckpointParticipantPrepareV1(context.Context, runtimeports.CheckpointAttemptRefV2, runtimeports.CheckpointParticipantRefV2) (runtimeports.CheckpointParticipantPhaseClosureRefV2, error)
	CommitCheckpointParticipantPhaseV1(context.Context, appcontract.CheckpointParticipantWorkRequestV1, runtimeports.CheckpointParticipantPhaseClosureRefV2, appcontract.CheckpointParticipantOwnerCandidateV1) (appcontract.CheckpointParticipantCommitV1, error)
	InspectCheckpointParticipantPhaseV1(context.Context, runtimeports.CheckpointAttemptRefV2, runtimeports.CheckpointParticipantRefV2) (appcontract.CheckpointParticipantCommitV1, error)
}

type GovernedCheckpointParticipantApplicationAdapterConfigV1 struct {
	ParticipantID string
	Current       applicationports.CheckpointParticipantOwnerCurrentReaderV1
	Lifecycle     CheckpointParticipantPhaseLifecyclePortV1
	Clock         func() time.Time
}

// GovernedCheckpointParticipantApplicationAdapterV1 makes the missing order
// explicit: prepare/capture -> Owner S1 -> commit -> Owner S2. The legacy
// reference-only adapter remains available for tests that already start from
// pre-existing Owner facts, but production-capable wiring must use this type.
type GovernedCheckpointParticipantApplicationAdapterV1 struct {
	config GovernedCheckpointParticipantApplicationAdapterConfigV1
}

func NewGovernedCheckpointParticipantApplicationAdapterV1(config GovernedCheckpointParticipantApplicationAdapterConfigV1) (*GovernedCheckpointParticipantApplicationAdapterV1, error) {
	if config.ParticipantID == "" || checkpointParticipantNilV1(config.Current) || checkpointParticipantLifecycleNilV1(config.Lifecycle) || config.Clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Sandbox governed checkpoint Participant adapter dependencies are required")
	}
	return &GovernedCheckpointParticipantApplicationAdapterV1{config: config}, nil
}

func (a *GovernedCheckpointParticipantApplicationAdapterV1) CompleteCheckpointParticipantV1(ctx context.Context, work appcontract.CheckpointParticipantWorkRequestV1) (appcontract.CheckpointParticipantCommitV1, error) {
	now := a.config.Clock()
	if err := work.Validate(now); err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	if work.Participant.ID != a.config.ParticipantID {
		return appcontract.CheckpointParticipantCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "Sandbox checkpoint Participant route is wrong")
	}

	prepared, err := a.config.Lifecycle.PrepareCheckpointParticipantPhaseV1(ctx, work)
	if err != nil && checkpointParticipantRetryableV1(err) {
		prepared, err = a.config.Lifecycle.InspectCheckpointParticipantPrepareV1(context.WithoutCancel(ctx), work.Attempt, work.Participant)
	}
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	if err := validatePreparedCheckpointClosureV1(prepared, work); err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}

	s1, err := a.config.Current.InspectCheckpointParticipantOwnerCurrentV1(ctx, work)
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	if err := s1.Validate(work, a.config.Clock()); err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}

	commit, err := a.config.Lifecycle.CommitCheckpointParticipantPhaseV1(ctx, work, prepared, s1)
	if err != nil && checkpointParticipantRetryableV1(err) {
		commit, err = a.config.Lifecycle.InspectCheckpointParticipantPhaseV1(context.WithoutCancel(ctx), work.Attempt, work.Participant)
	}
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	if err := commit.ValidateForAttemptV1(work.Participant, work.Attempt); err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	if commit.RuntimeClosure.Prepare != prepared {
		return appcontract.CheckpointParticipantCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Sandbox checkpoint commit replaced the exact prepared closure")
	}

	fresh := a.config.Clock()
	s2, err := a.config.Current.InspectCheckpointParticipantOwnerCurrentV1(ctx, work)
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	if err := s2.Validate(work, fresh); err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	if s2.ProjectionDigest != s1.ProjectionDigest || commit.ParticipantFact != s2.ParticipantFact || commit.Snapshot != s2.Snapshot || commit.Coverage != s2.Coverage {
		return appcontract.CheckpointParticipantCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Sandbox checkpoint Participant Owner current changed between S1 and S2")
	}
	return commit.Clone(), nil
}

func (a *GovernedCheckpointParticipantApplicationAdapterV1) InspectCheckpointParticipantV1(ctx context.Context, attempt runtimeports.CheckpointAttemptRefV2, participant runtimeports.CheckpointParticipantRefV2) (appcontract.CheckpointParticipantCommitV1, error) {
	if attempt.Validate() != nil || participant.Validate() != nil || participant.ID != a.config.ParticipantID {
		return appcontract.CheckpointParticipantCommitV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Sandbox checkpoint Participant Inspect is invalid")
	}
	commit, err := a.config.Lifecycle.InspectCheckpointParticipantPhaseV1(ctx, attempt, participant)
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	return commit.Clone(), commit.ValidateForAttemptV1(participant, attempt)
}

func validatePreparedCheckpointClosureV1(prepared runtimeports.CheckpointParticipantPhaseClosureRefV2, work appcontract.CheckpointParticipantWorkRequestV1) error {
	if prepared.Validate() != nil || prepared.Phase != runtimeports.CheckpointPhasePrepareV2 || prepared.PhaseFact.State != runtimeports.CheckpointParticipantPreparedV2 || prepared.DomainResult.Attempt != work.Attempt || prepared.DomainResult.Participant != work.Participant || prepared.ApplySettlement.Participant != work.Participant {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Sandbox checkpoint prepare returned another Attempt, Participant, or state")
	}
	return nil
}

func checkpointParticipantRetryableV1(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)
}

func checkpointParticipantLifecycleNilV1(value CheckpointParticipantPhaseLifecyclePortV1) bool {
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

var _ applicationports.CheckpointParticipantDriverV1 = (*GovernedCheckpointParticipantApplicationAdapterV1)(nil)

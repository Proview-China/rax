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

type CheckpointParticipantApplicationAdapterConfigV1 struct {
	ParticipantID string
	Current       applicationports.CheckpointParticipantOwnerCurrentReaderV1
	Phases        applicationports.CheckpointParticipantPhaseCommitPortV1
	Clock         func() time.Time
}

type CheckpointParticipantApplicationAdapterV1 struct {
	config CheckpointParticipantApplicationAdapterConfigV1
}

func NewCheckpointParticipantApplicationAdapterV1(config CheckpointParticipantApplicationAdapterConfigV1) (*CheckpointParticipantApplicationAdapterV1, error) {
	if config.ParticipantID == "" || checkpointParticipantNilV1(config.Current) || checkpointParticipantNilV1(config.Phases) || config.Clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Sandbox checkpoint Participant adapter dependencies are required")
	}
	return &CheckpointParticipantApplicationAdapterV1{config: config}, nil
}

func (a *CheckpointParticipantApplicationAdapterV1) CompleteCheckpointParticipantV1(ctx context.Context, work appcontract.CheckpointParticipantWorkRequestV1) (appcontract.CheckpointParticipantCommitV1, error) {
	now := a.config.Clock()
	if err := work.Validate(now); err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	if work.Participant.ID != a.config.ParticipantID {
		return appcontract.CheckpointParticipantCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "Sandbox checkpoint Participant route is wrong")
	}
	s1, err := a.config.Current.InspectCheckpointParticipantOwnerCurrentV1(ctx, work)
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	if err := s1.Validate(work, now); err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	commit, err := a.config.Phases.CommitCheckpointParticipantPhaseV1(ctx, work, s1)
	if err != nil && (core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)) {
		commit, err = a.config.Phases.InspectCheckpointParticipantPhaseV1(context.WithoutCancel(ctx), work.Attempt, work.Participant)
	}
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	if err := commit.ValidateForAttemptV1(work.Participant, work.Attempt); err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	fresh := a.config.Clock()
	s2, err := a.config.Current.InspectCheckpointParticipantOwnerCurrentV1(ctx, work)
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	if err := s2.Validate(work, fresh); err != nil || s2.ProjectionDigest != s1.ProjectionDigest || commit.ParticipantFact != s2.ParticipantFact || commit.Snapshot != s2.Snapshot || commit.Coverage != s2.Coverage {
		if err != nil {
			return appcontract.CheckpointParticipantCommitV1{}, err
		}
		return appcontract.CheckpointParticipantCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Sandbox checkpoint Participant Owner current changed between S1 and S2")
	}
	return commit.Clone(), nil
}

func (a *CheckpointParticipantApplicationAdapterV1) InspectCheckpointParticipantV1(ctx context.Context, attempt runtimeports.CheckpointAttemptRefV2, participant runtimeports.CheckpointParticipantRefV2) (appcontract.CheckpointParticipantCommitV1, error) {
	if attempt.Validate() != nil || participant.Validate() != nil || participant.ID != a.config.ParticipantID {
		return appcontract.CheckpointParticipantCommitV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Sandbox checkpoint Participant Inspect is invalid")
	}
	commit, err := a.config.Phases.InspectCheckpointParticipantPhaseV1(ctx, attempt, participant)
	if err != nil {
		return appcontract.CheckpointParticipantCommitV1{}, err
	}
	return commit.Clone(), commit.ValidateForAttemptV1(participant, attempt)
}

func checkpointParticipantNilV1(value any) bool {
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

var _ applicationports.CheckpointParticipantDriverV1 = (*CheckpointParticipantApplicationAdapterV1)(nil)

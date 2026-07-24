package application

import (
	"context"
	"reflect"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type CheckpointCoordinatorConfigV1 struct {
	Gates          applicationports.CheckpointGatePortV1
	Runtime        runtimeports.CheckpointGovernancePortV2
	Effects        runtimeports.CheckpointEffectInventoryCurrentReaderV2
	ParticipantSet runtimeports.CheckpointParticipantSetCurrentReaderV2
	Closures       runtimeports.CheckpointParticipantClosureCurrentReaderV2
	Inputs         applicationports.CheckpointManifestInputCurrentReaderV1
	Participants   map[string]applicationports.CheckpointParticipantDriverV1
	Manifests      applicationports.CheckpointManifestPortV1
	Clock          func() time.Time
}

type CheckpointCoordinatorV1 struct{ config CheckpointCoordinatorConfigV1 }

type CheckpointCoordinationResultV1 struct {
	Gate            contract.CheckpointGateCommitV1          `json:"gate"`
	Attempt         runtimeports.CheckpointAttemptRefV2      `json:"attempt"`
	TerminalAttempt runtimeports.CheckpointAttemptRefV2      `json:"terminal_attempt"`
	Barrier         runtimeports.CheckpointBarrierLeaseRefV2 `json:"barrier"`
	EffectCut       runtimeports.EffectCutRefV2              `json:"effect_cut"`
	Participants    []contract.CheckpointParticipantCommitV1 `json:"participants"`
	ManifestSeal    runtimeports.CheckpointManifestSealRefV2 `json:"manifest_seal"`
	Consistency     runtimeports.CheckpointConsistencyRefV2  `json:"consistency"`
}

func NewCheckpointCoordinatorV1(config CheckpointCoordinatorConfigV1) (*CheckpointCoordinatorV1, error) {
	if checkpointNilV1(config.Gates) || checkpointNilV1(config.Runtime) || checkpointNilV1(config.Effects) || checkpointNilV1(config.ParticipantSet) || checkpointNilV1(config.Closures) || checkpointNilV1(config.Inputs) || checkpointNilV1(config.Manifests) || config.Clock == nil || len(config.Participants) < 2 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "checkpoint coordinator requires Gate, Runtime, current Readers, Manifest and at least two Participant Owners")
	}
	for id, driver := range config.Participants {
		if id == "" || checkpointNilV1(driver) {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "checkpoint Participant registry is incomplete")
		}
	}
	return &CheckpointCoordinatorV1{config: config}, nil
}

func (c *CheckpointCoordinatorV1) RunCheckpointV1(ctx context.Context, request contract.StartCheckpointCoordinationRequestV1) (CheckpointCoordinationResultV1, error) {
	now := c.config.Clock()
	if err := request.Validate(now); err != nil {
		return CheckpointCoordinationResultV1{}, err
	}
	gate, err := c.config.Gates.AcquireCheckpointGateV1(ctx, request.Gate)
	if err != nil {
		return CheckpointCoordinationResultV1{}, err
	}
	if gate.State == contract.CheckpointGateInvalidatedV1 {
		return CheckpointCoordinationResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint Gate was invalidated before Runtime create")
	}

	var attempt runtimeports.CheckpointAttemptRefV2
	var barrier runtimeports.CheckpointBarrierLeaseRefV2
	var cut runtimeports.EffectCutRefV2
	switch gate.State {
	case contract.CheckpointGateAcquiredV1:
		bundle, createErr := c.config.Runtime.CreateCheckpointAttemptV2(ctx, request.RuntimeCreate)
		if createErr != nil {
			// An unavailable/indeterminate create may have crossed the Runtime
			// linearization point. Keep the Gate fenced; recovery may only Inspect
			// the same Attempt identity. Invalidate only a definitive rejection.
			if !checkpointRecoverableV1(createErr) {
				_, _ = c.config.Gates.InvalidateCheckpointGateV1(context.WithoutCancel(ctx), gate.Gate)
			}
			return CheckpointCoordinationResultV1{}, createErr
		}
		inventory, inspectErr := c.config.Effects.InspectCheckpointEffectInventoryCurrentV2(ctx, bundle.Attempt.RefV2(), bundle.Barrier.RefV2())
		if inspectErr != nil || inventory.Validate(c.config.Clock()) != nil {
			return CheckpointCoordinationResultV1{}, firstCheckpointErrorV1(inspectErr, core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint Effect inventory is not current"))
		}
		frozen, freezeErr := c.config.Runtime.FreezeCheckpointEffectCutV2(ctx, runtimeports.FreezeCheckpointEffectCutRequestV2{Attempt: bundle.Attempt.RefV2(), Barrier: bundle.Barrier.RefV2(), ExpectedAttemptRevision: bundle.Attempt.Revision, ExpectedBarrierRevision: bundle.Barrier.Revision, EffectInventoryRoot: inventory.RootDigest, EffectInventoryWatermark: inventory.Watermark, ExpectedEffectCount: uint64(len(inventory.Entries)), IdempotencyKey: request.CutID})
		if freezeErr != nil {
			return CheckpointCoordinationResultV1{}, freezeErr
		}
		attempt, barrier, cut = frozen.Attempt.RefV2(), bundle.Barrier.RefV2(), frozen.Cut.Ref
		gate, err = c.config.Gates.BindCheckpointGateRuntimeV1(ctx, contract.BindCheckpointGateRuntimeRequestV1{Gate: gate, Attempt: attempt, Barrier: barrier, EffectCut: cut})
		if err != nil {
			return CheckpointCoordinationResultV1{}, err
		}
	case contract.CheckpointGateBoundV1, contract.CheckpointGateReleasedV1:
		if gate.RuntimeAttempt == nil || gate.RuntimeBarrier == nil || gate.RuntimeEffectCut == nil {
			return CheckpointCoordinationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "progressed checkpoint Gate lost Runtime binding")
		}
		attempt, barrier, cut = *gate.RuntimeAttempt, *gate.RuntimeBarrier, *gate.RuntimeEffectCut
	default:
		return CheckpointCoordinationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Gate state is unsupported")
	}

	inputS1, err := c.config.Inputs.InspectCheckpointManifestInputCurrentV1(ctx, attempt, barrier, cut)
	if err != nil || inputS1.Validate(c.config.Clock()) != nil {
		return CheckpointCoordinationResultV1{}, firstCheckpointErrorV1(err, core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint Manifest inputs S1 are not current"))
	}
	setS1, err := c.config.ParticipantSet.InspectCheckpointParticipantSetCurrentV2(ctx, attempt, request.RuntimeCreate.ParticipantSetCertification)
	if err != nil || setS1.Validate(c.config.Clock()) != nil || setS1.Attempt != attempt || setS1.Certification != request.RuntimeCreate.ParticipantSetCertification {
		return CheckpointCoordinationResultV1{}, firstCheckpointErrorV1(err, core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint Participant set S1 is not current"))
	}

	commits := make([]contract.CheckpointParticipantCommitV1, 0, len(setS1.Participants))
	closures := make([]runtimeports.CheckpointParticipantClosureRefV2, 0, len(setS1.Participants))
	for _, participant := range setS1.Participants {
		driver, ok := c.config.Participants[participant.ID]
		if !ok || checkpointNilV1(driver) {
			return CheckpointCoordinationResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "certified checkpoint Participant Owner adapter is unavailable")
		}
		work := contract.CheckpointParticipantWorkRequestV1{Attempt: attempt, Barrier: barrier, EffectCut: cut, Participant: participant, Gate: gate.Gate, Snapshot: gate.Snapshot, NotAfter: minCheckpointNanosV1(request.NotAfter, gate.ExpiresUnixNano, barrier.ExpiresUnixNano)}
		commit, completeErr := driver.CompleteCheckpointParticipantV1(ctx, work)
		if completeErr != nil && checkpointRecoverableV1(completeErr) {
			commit, completeErr = driver.InspectCheckpointParticipantV1(context.WithoutCancel(ctx), attempt, participant)
		}
		if completeErr != nil {
			return CheckpointCoordinationResultV1{}, completeErr
		}
		if err := commit.ValidateForAttemptV1(participant, attempt); err != nil {
			return CheckpointCoordinationResultV1{}, err
		}
		current, currentErr := c.config.Closures.InspectCheckpointParticipantClosureCurrentV2(ctx, attempt, participant)
		if currentErr != nil || current.Validate(c.config.Clock()) != nil || current.Attempt != attempt || current.Participant != participant || current.Closure != commit.RuntimeClosure {
			return CheckpointCoordinationResultV1{}, firstCheckpointErrorV1(currentErr, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Participant closure is not exact current"))
		}
		commits, closures = append(commits, commit.Clone()), append(closures, commit.RuntimeClosure)
	}
	sort.Slice(commits, func(i, j int) bool { return commits[i].RuntimeClosure.ID < commits[j].RuntimeClosure.ID })
	sort.Slice(closures, func(i, j int) bool { return closures[i].ID < closures[j].ID })

	setS2, err := c.config.ParticipantSet.InspectCheckpointParticipantSetCurrentV2(ctx, attempt, request.RuntimeCreate.ParticipantSetCertification)
	if err != nil || setS2.Validate(c.config.Clock()) != nil || setS2.Attempt != attempt || setS2.Certification != request.RuntimeCreate.ParticipantSetCertification || setS2.ProjectionDigest != setS1.ProjectionDigest {
		return CheckpointCoordinationResultV1{}, firstCheckpointErrorV1(err, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Participant set changed between S1 and S2"))
	}
	inputS2, err := c.config.Inputs.InspectCheckpointManifestInputCurrentV1(ctx, attempt, barrier, cut)
	if err != nil || inputS2.Validate(c.config.Clock()) != nil || inputS2.ProjectionDigest != inputS1.ProjectionDigest {
		return CheckpointCoordinationResultV1{}, firstCheckpointErrorV1(err, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Manifest inputs changed between S1 and S2"))
	}

	seal, err := c.config.Manifests.CreateCheckpointManifestSealV1(ctx, contract.CreateCheckpointManifestSealRequestV1{StableID: request.ManifestID, SealID: request.ManifestSealID, IdempotencyKey: request.StableID, Scope: request.RuntimeCreate.Scope, RunStableIdentityDigest: request.RuntimeCreate.RunStableIdentityDigest, Attempt: attempt, Barrier: barrier, EffectCut: cut, Gate: gate.Gate, Snapshot: gate.Snapshot, ParticipantSet: setS2, Closures: closures, Input: inputS2, Participants: commits, RequestedNotAfter: request.NotAfter})
	if err != nil {
		return CheckpointCoordinationResultV1{}, err
	}
	consistent, err := c.config.Runtime.CommitCheckpointConsistencyAndCloseBarrierV2(ctx, runtimeports.CommitCheckpointConsistencyRequestV2{Attempt: attempt, Barrier: barrier, ExpectedAttemptRevision: attempt.Revision, ExpectedBarrierRevision: barrier.Revision, EffectCut: cut, ManifestSeal: seal, ExpectedParticipantRoot: setS2.RootDigest, ExpectedParticipantWatermark: setS2.Watermark, ExpectedParticipantCount: uint64(len(setS2.Participants)), IdempotencyKey: request.StableID})
	if err != nil {
		return CheckpointCoordinationResultV1{}, err
	}
	if gate.State != contract.CheckpointGateReleasedV1 {
		gate, err = c.config.Gates.ReleaseCheckpointGateV1(ctx, gate, consistent.Attempt.RefV2())
		if err != nil {
			return CheckpointCoordinationResultV1{}, err
		}
	}
	return CheckpointCoordinationResultV1{Gate: gate, Attempt: attempt, TerminalAttempt: consistent.Attempt.RefV2(), Barrier: barrier, EffectCut: cut, Participants: commits, ManifestSeal: seal, Consistency: consistent.Consistency.Ref}, nil
}

func checkpointRecoverableV1(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)
}
func checkpointNilV1(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
func minCheckpointNanosV1(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}
func firstCheckpointErrorV1(actual, fallback error) error {
	if actual != nil {
		return actual
	}
	return fallback
}

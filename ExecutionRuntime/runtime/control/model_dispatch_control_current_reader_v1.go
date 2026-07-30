package control

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const MaxModelDispatchControlCurrentTTLV1 = time.Second

// ModelDispatchRunFactCurrentReaderV1 is the narrow Run fact capability used
// by the Model dispatch-control current adapter.
type ModelDispatchRunFactCurrentReaderV1 interface {
	InspectRun(context.Context, core.ExecutionScope, core.AgentRunID) (core.AgentRunRecord, error)
}

// ModelDispatchCommandFactCurrentReaderV1 is the narrow command fact
// capability used by the Model dispatch-control current adapter.
type ModelDispatchCommandFactCurrentReaderV1 interface {
	ReadDesiredState(context.Context, core.ExecutionScope) (ports.DesiredStateSnapshotV2, error)
	ListCommands(context.Context, core.ExecutionScope) ([]ports.ApplicationCommandRecordV2, error)
}

type modelDispatchControlCurrentReaderV1 struct {
	runs     ModelDispatchRunFactCurrentReaderV1
	commands ModelDispatchCommandFactCurrentReaderV1
	clock    func() time.Time
}

type modelDispatchControlSourceSnapshotV1 struct {
	Run         core.AgentRunRecord               `json:"run"`
	Desired     ports.DesiredStateSnapshotV2      `json:"desired_state"`
	LastCommand *ports.ApplicationCommandRecordV2 `json:"last_command,omitempty"`
	State       ModelDispatchControlStateV1       `json:"state"`
}

// NewModelDispatchControlCurrentReaderV1 returns only the read-only current
// capability. RunFactPort and ApplicationCommandFactPortV2 satisfy the narrow
// input interfaces structurally, but their mutation methods are not retained.
func NewModelDispatchControlCurrentReaderV1(
	runs ModelDispatchRunFactCurrentReaderV1,
	commands ModelDispatchCommandFactCurrentReaderV1,
	clock func() time.Time,
) (ModelDispatchControlCurrentReaderV1, error) {
	if modelDispatchControlReaderDependencyNilV1(runs) || modelDispatchControlReaderDependencyNilV1(commands) || clock == nil {
		return nil, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Model dispatch-control current readers are incomplete")
	}
	return &modelDispatchControlCurrentReaderV1{runs: runs, commands: commands, clock: clock}, nil
}

func (r *modelDispatchControlCurrentReaderV1) InspectModelDispatchControlCurrentV1(
	ctx context.Context,
	operation ports.OperationSubjectV3,
	effectID core.EffectIntentID,
) (ModelDispatchControlCurrentProjectionV1, error) {
	if ctx == nil {
		return ModelDispatchControlCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model dispatch-control context is nil")
	}
	if err := ctx.Err(); err != nil {
		return ModelDispatchControlCurrentProjectionV1{}, err
	}
	if err := operation.Validate(); err != nil {
		return ModelDispatchControlCurrentProjectionV1{}, err
	}
	if operation.Kind != ports.OperationScopeRunV3 || strings.TrimSpace(string(operation.RunID)) == "" || strings.TrimSpace(string(effectID)) == "" {
		return ModelDispatchControlCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model dispatch-control requires one Run operation and Effect")
	}

	firstNow := r.clock()
	if firstNow.IsZero() {
		return ModelDispatchControlCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model dispatch-control clock is zero")
	}
	first, err := r.readModelDispatchControlSourceV1(ctx, operation, firstNow)
	if err != nil {
		return ModelDispatchControlCurrentProjectionV1{}, err
	}
	firstDigest, err := digestModelDispatchControlSourceV1(first)
	if err != nil {
		return ModelDispatchControlCurrentProjectionV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return ModelDispatchControlCurrentProjectionV1{}, err
	}

	secondNow := r.clock()
	if secondNow.IsZero() || secondNow.Before(firstNow) {
		return ModelDispatchControlCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model dispatch-control clock regressed before S2")
	}
	second, err := r.readModelDispatchControlSourceV1(ctx, operation, secondNow)
	if err != nil {
		return ModelDispatchControlCurrentProjectionV1{}, err
	}
	secondDigest, err := digestModelDispatchControlSourceV1(second)
	if err != nil {
		return ModelDispatchControlCurrentProjectionV1{}, err
	}
	if firstDigest != secondDigest {
		return ModelDispatchControlCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model dispatch-control facts drifted between S1 and S2")
	}
	if err := ctx.Err(); err != nil {
		return ModelDispatchControlCurrentProjectionV1{}, err
	}

	sealNow := r.clock()
	if sealNow.IsZero() || sealNow.Before(secondNow) {
		return ModelDispatchControlCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model dispatch-control seal clock regressed")
	}
	if err := ctx.Err(); err != nil {
		return ModelDispatchControlCurrentProjectionV1{}, err
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		return ModelDispatchControlCurrentProjectionV1{}, err
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(second.Run.Scope)
	if err != nil {
		return ModelDispatchControlCurrentProjectionV1{}, err
	}
	lastCommandID := ""
	if second.LastCommand != nil {
		lastCommandID = second.LastCommand.Envelope.ID
	}
	expires := sealNow.Add(MaxModelDispatchControlCurrentTTLV1)
	if !expires.After(sealNow) || expires.UnixNano() <= sealNow.UnixNano() {
		return ModelDispatchControlCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model dispatch-control TTL overflowed")
	}
	return SealModelDispatchControlCurrentProjectionV1(ModelDispatchControlCurrentProjectionV1{
		OperationDigest:      operationDigest,
		EffectID:             effectID,
		RunID:                second.Run.ID,
		ExecutionScopeDigest: scopeDigest,
		RunRevision:          second.Run.Revision,
		DesiredStateRevision: second.Desired.Revision,
		LastCommandID:        lastCommandID,
		State:                second.State,
		WatermarkDigest:      secondDigest,
		CheckedUnixNano:      sealNow.UnixNano(),
		ExpiresUnixNano:      expires.UnixNano(),
	})
}

func (r *modelDispatchControlCurrentReaderV1) readModelDispatchControlSourceV1(
	ctx context.Context,
	operation ports.OperationSubjectV3,
	now time.Time,
) (modelDispatchControlSourceSnapshotV1, error) {
	if err := ctx.Err(); err != nil {
		return modelDispatchControlSourceSnapshotV1{}, err
	}
	run, err := r.runs.InspectRun(ctx, operation.ExecutionScope, operation.RunID)
	if err != nil {
		return modelDispatchControlSourceSnapshotV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return modelDispatchControlSourceSnapshotV1{}, err
	}
	if err := run.Validate(); err != nil {
		return modelDispatchControlSourceSnapshotV1{}, err
	}
	if run.ID != operation.RunID || !ports.SameExecutionScopeV2(run.Scope, operation.ExecutionScope) || run.StartedAt.After(now) || (!run.EndedAt.IsZero() && run.EndedAt.After(now)) {
		return modelDispatchControlSourceSnapshotV1{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "Model dispatch-control Run does not match the operation")
	}

	desired, err := r.commands.ReadDesiredState(ctx, operation.ExecutionScope)
	if err != nil {
		return modelDispatchControlSourceSnapshotV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return modelDispatchControlSourceSnapshotV1{}, err
	}
	if err := validateModelDispatchDesiredStateV1(desired, operation.ExecutionScope); err != nil {
		return modelDispatchControlSourceSnapshotV1{}, err
	}

	commands, err := r.commands.ListCommands(ctx, operation.ExecutionScope)
	if err != nil {
		return modelDispatchControlSourceSnapshotV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return modelDispatchControlSourceSnapshotV1{}, err
	}
	last, err := exactModelDispatchLastCommandV1(commands, desired, now)
	if err != nil {
		return modelDispatchControlSourceSnapshotV1{}, err
	}
	state, err := deriveModelDispatchControlStateV1(run, desired, last)
	if err != nil {
		return modelDispatchControlSourceSnapshotV1{}, err
	}
	return modelDispatchControlSourceSnapshotV1{Run: run, Desired: desired, LastCommand: last, State: state}, nil
}

func validateModelDispatchDesiredStateV1(desired ports.DesiredStateSnapshotV2, scope core.ExecutionScope) error {
	if err := desired.Scope.Validate(); err != nil {
		return err
	}
	if !ports.SameExecutionScopeV2(desired.Scope, scope) || desired.Revision == 0 || !ports.ValidDesiredExecutionStateV2(desired.Desired) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model dispatch-control desired state is malformed or belongs to another scope")
	}
	if desired.Revision == 1 && desired.LastCommandID != "" {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "initial desired state cannot name a command")
	}
	if desired.Revision > 1 && strings.TrimSpace(desired.LastCommandID) == "" {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "advanced desired state requires its exact last command")
	}
	return nil
}

func exactModelDispatchLastCommandV1(
	records []ports.ApplicationCommandRecordV2,
	desired ports.DesiredStateSnapshotV2,
	now time.Time,
) (*ports.ApplicationCommandRecordV2, error) {
	if now.IsZero() {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model dispatch-control validation time is zero")
	}
	if desired.LastCommandID == "" {
		if len(records) != 0 {
			return nil, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "initial desired state has unexpected command history")
		}
		return nil, nil
	}
	records = append([]ports.ApplicationCommandRecordV2(nil), records...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].Revision == records[j].Revision {
			return records[i].Envelope.ID < records[j].Envelope.ID
		}
		return records[i].Revision < records[j].Revision
	})
	seenIDs := make(map[string]struct{}, len(records))
	seenRevisions := make(map[core.Revision]struct{}, len(records))
	var last *ports.ApplicationCommandRecordV2
	for index := range records {
		record := records[index]
		if err := validateModelDispatchCommandRecordV1(record, now); err != nil {
			return nil, err
		}
		if !ports.SameExecutionScopeV2(record.Envelope.Target, desired.Scope) {
			return nil, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "command history contains another execution scope")
		}
		if err := core.CheckExecutionPreconditions(record.Envelope.Preconditions, core.CurrentExecutionFacts{Scope: record.Envelope.Target, Revision: record.Revision - 1}); err != nil {
			return nil, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "command history preconditions do not match its target")
		}
		if _, exists := seenIDs[record.Envelope.ID]; exists {
			return nil, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "command history contains a duplicate command identity")
		}
		if _, exists := seenRevisions[record.Revision]; exists {
			return nil, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "command history contains a duplicate revision")
		}
		seenIDs[record.Envelope.ID] = struct{}{}
		seenRevisions[record.Revision] = struct{}{}
		if record.Revision > desired.Revision {
			return nil, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "command history advanced beyond desired state")
		}
		if record.Envelope.ID == desired.LastCommandID {
			value := record
			last = &value
		}
	}
	if last == nil || last.Revision != desired.Revision {
		return nil, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "desired state does not bind one exact last command")
	}
	return last, nil
}

func validateModelDispatchCommandRecordV1(record ports.ApplicationCommandRecordV2, now time.Time) error {
	if err := record.Envelope.Validate(); err != nil {
		return err
	}
	if record.Revision == 0 || record.RecordedAt.IsZero() || record.RecordedAt.Before(record.Envelope.SubmittedAt) || !record.RecordedAt.Before(record.Envelope.ExpiresAt) || record.RecordedAt.After(now) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "command record time or revision is invalid")
	}
	if record.Envelope.Preconditions.Revision+1 != record.Revision {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "command record does not follow its expected desired-state revision")
	}
	if !validModelDispatchCommandStatusV1(record.Status) {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "command record status is invalid")
	}
	return nil
}

func deriveModelDispatchControlStateV1(
	run core.AgentRunRecord,
	desired ports.DesiredStateSnapshotV2,
	last *ports.ApplicationCommandRecordV2,
) (ModelDispatchControlStateV1, error) {
	if run.Status != core.RunRunning {
		return ModelDispatchControlIndeterminateV1, nil
	}
	if last == nil {
		if desired.Desired == ports.DesiredRunningV2 {
			return ModelDispatchControlDispatchableV1, nil
		}
		return ModelDispatchControlIndeterminateV1, nil
	}
	switch last.Status {
	case ports.ApplicationCommandRejectedV2, ports.ApplicationCommandSupersededV2, ports.ApplicationCommandInvalidatedV2, ports.ApplicationCommandIndeterminateV2:
		return ModelDispatchControlIndeterminateV1, nil
	case ports.ApplicationCommandAcceptedV2, ports.ApplicationCommandExecutingV2, ports.ApplicationCommandCompletedV2:
	default:
		return ModelDispatchControlIndeterminateV1, nil
	}
	switch last.Envelope.Kind {
	case ports.ApplicationCommandCancelRunV2:
		if desired.Desired == ports.DesiredRunningV2 {
			return ModelDispatchControlCancelRequestedV1, nil
		}
	case ports.ApplicationCommandFenceV2:
		if desired.Desired == ports.DesiredFencedV2 {
			return ModelDispatchControlFencedV1, nil
		}
	case ports.ApplicationCommandRevokeV2:
		if desired.Desired == ports.DesiredFencedV2 {
			return ModelDispatchControlRevokedV1, nil
		}
	case ports.ApplicationCommandStopInstanceV2:
		if desired.Desired == ports.DesiredStoppedV2 {
			return ModelDispatchControlIndeterminateV1, nil
		}
	case ports.ApplicationCommandStartV2, ports.ApplicationCommandResumeV2, ports.ApplicationCommandProvideInputV2, ports.ApplicationCommandApproveEffectV2, ports.ApplicationCommandDenyEffectV2:
		if desired.Desired == ports.DesiredRunningV2 {
			return ModelDispatchControlDispatchableV1, nil
		}
	}
	return "", core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "last command and desired state are inconsistent")
}

func validModelDispatchCommandStatusV1(status ports.ApplicationCommandStatusV2) bool {
	switch status {
	case ports.ApplicationCommandAcceptedV2, ports.ApplicationCommandRejectedV2, ports.ApplicationCommandExecutingV2,
		ports.ApplicationCommandCompletedV2, ports.ApplicationCommandSupersededV2, ports.ApplicationCommandInvalidatedV2,
		ports.ApplicationCommandIndeterminateV2:
		return true
	default:
		return false
	}
}

func digestModelDispatchControlSourceV1(source modelDispatchControlSourceSnapshotV1) (core.Digest, error) {
	return core.CanonicalJSONDigest(
		"praxis.runtime.model-dispatch-control-current-source",
		ModelDispatchControlCurrentContractVersionV1,
		"ModelDispatchControlSourceSnapshotV1",
		source,
	)
}

func modelDispatchControlReaderDependencyNilV1(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}

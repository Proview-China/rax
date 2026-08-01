package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type workspaceReadViewCurrentReaderV2 interface {
	InspectWorkspaceViewCurrentV1(context.Context, contract.Ref) (contract.WorkspaceView, error)
}

type workspaceReadCommandOwnerRepositoryV2 interface {
	ApplyWorkspaceReadCommandPublicationV2(
		context.Context,
		ownerworkspaceread.AuthorizedCommandPublicationV2,
	) (contract.WorkspaceReadCommandOwnerCurrentV2, bool, error)
	InspectStoredWorkspaceReadCommandExactV1(
		context.Context,
		contract.Ref,
	) (contract.WorkspaceReadCommandV1, error)
	InspectStoredWorkspaceReadCommandPublicationExactV2(
		context.Context,
		contract.Ref,
	) (contract.WorkspaceReadCommandPublicationV2, error)
	InspectStoredWorkspaceReadCommandOwnerCurrentExactV2(
		context.Context,
		contract.Ref,
	) (contract.WorkspaceReadCommandOwnerCurrentV2, error)
	InspectStoredWorkspaceReadCommandOwnerCurrentByCommandV2(
		context.Context,
		contract.Ref,
	) (contract.WorkspaceReadCommandOwnerCurrentV2, error)
	InspectStoredWorkspaceReadCommandOwnerHistoryV2(
		context.Context,
		string,
		contract.Ref,
	) ([]contract.WorkspaceReadCommandOwnerCurrentV2, error)
}

type WorkspaceReadCommandOwnerV2 struct {
	sources    sandboxports.WorkspaceReadSourceCurrentReaderV2
	effects    runtimeports.ControlledOperationEffectCurrentReaderV2
	prepared   runtimeports.ControlledOperationPreparedCurrentReaderV2
	workspaces workspaceReadViewCurrentReaderV2
	repository workspaceReadCommandOwnerRepositoryV2
	clock      func() time.Time
}

func NewWorkspaceReadCommandOwnerV2(
	sources sandboxports.WorkspaceReadSourceCurrentReaderV2,
	effects runtimeports.ControlledOperationEffectCurrentReaderV2,
	prepared runtimeports.ControlledOperationPreparedCurrentReaderV2,
	workspaces workspaceReadViewCurrentReaderV2,
	repository workspaceReadCommandOwnerRepositoryV2,
	clock func() time.Time,
) (*WorkspaceReadCommandOwnerV2, error) {
	if nilLikeWorkspaceReadInspectionV2(sources) ||
		nilLikeWorkspaceReadInspectionV2(effects) ||
		nilLikeWorkspaceReadInspectionV2(prepared) ||
		nilLikeWorkspaceReadInspectionV2(workspaces) ||
		nilLikeWorkspaceReadInspectionV2(repository) ||
		clock == nil {
		return nil, runtimecore.NewError(
			runtimecore.ErrorInvalidArgument,
			runtimecore.ReasonInvalidReference,
			"workspace read command Owner dependencies are incomplete",
		)
	}
	return &WorkspaceReadCommandOwnerV2{
		sources: sources, effects: effects, prepared: prepared,
		workspaces: workspaces, repository: repository, clock: clock,
	}, nil
}

func (o *WorkspaceReadCommandOwnerV2) EnsureWorkspaceReadCommandV2(
	ctx context.Context,
	request sandboxports.EnsureWorkspaceReadCommandRequestV2,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	if o == nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if ctx == nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, errors.New("workspace read command Ensure context is required")
	}
	if err := request.Validate(); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	s1, err := o.readWorkspaceReadCommandInputsV2(ctx, request.SourceCommand)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	s2, err := o.readWorkspaceReadCommandInputsV2(ctx, request.SourceCommand)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	if err = validateWorkspaceReadCommandInputPairV2(s1, s2); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	commitNow := o.clock()
	if commitNow.IsZero() || commitNow.Before(s2.checked) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if err = s2.validateCurrent(commitNow); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	semantic, err := contract.SealWorkspaceReadCommandPublicationSemanticV2(
		s2.source,
		s2.effect,
		s2.prepared,
		s2.workspace,
		commitNow,
	)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	candidateCommand, err := contract.SealWorkspaceReadPublishedCommandV2(semantic, commitNow)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	candidatePublication, err := contract.SealWorkspaceReadCommandPublicationV2(
		contract.WorkspaceReadCommandPublicationV2{Semantic: semantic},
		candidateCommand,
		commitNow,
	)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}

	existing, inspectErr := o.repository.InspectStoredWorkspaceReadCommandOwnerCurrentByCommandV2(
		ctx,
		candidateCommand.Meta.Ref(),
	)
	switch {
	case inspectErr == nil:
		return o.refreshOrReuseWorkspaceReadCommandV2(
			ctx,
			candidateCommand,
			candidatePublication,
			existing,
			s2,
			commitNow,
		)
	case !errors.Is(inspectErr, sandboxports.ErrNotFound):
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, inspectErr
	default:
		return o.createInitialWorkspaceReadCommandV2(
			ctx,
			candidateCommand,
			candidatePublication,
			s2,
			commitNow,
		)
	}
}

type workspaceReadCommandInputsV2 struct {
	expectedSource contract.WorkspaceReadSourceCommandRefV2
	source         contract.WorkspaceReadSourceCurrentProjectionV2
	effect         runtimeports.ControlledOperationEffectCurrentProjectionV2
	prepared       runtimeports.ControlledOperationPreparedCurrentProjectionV2
	workspace      contract.WorkspaceView
	checked        time.Time
}

func (o *WorkspaceReadCommandOwnerV2) readWorkspaceReadCommandInputsV2(
	ctx context.Context,
	sourceRef contract.WorkspaceReadSourceCommandRefV2,
) (workspaceReadCommandInputsV2, error) {
	start := o.clock()
	if start.IsZero() {
		return workspaceReadCommandInputsV2{}, sandboxports.ErrConflict
	}
	source, err := o.sources.InspectWorkspaceReadSourceCurrentV2(ctx, sourceRef)
	if err != nil {
		return workspaceReadCommandInputsV2{}, err
	}
	source, err = detachWorkspaceReadCommandInputV2(source)
	if err != nil {
		return workspaceReadCommandInputsV2{}, err
	}
	effect, err := o.effects.InspectCurrentControlledOperationEffectV2(
		ctx,
		source.Operation,
		source.RuntimeAttempt.EffectID,
	)
	if err != nil {
		return workspaceReadCommandInputsV2{}, err
	}
	effect, err = detachWorkspaceReadCommandInputV2(effect)
	if err != nil {
		return workspaceReadCommandInputsV2{}, err
	}
	prepared, err := o.prepared.InspectCurrentControlledOperationPreparedV2(ctx, source.Prepared)
	if err != nil {
		return workspaceReadCommandInputsV2{}, err
	}
	prepared, err = detachWorkspaceReadCommandInputV2(prepared)
	if err != nil {
		return workspaceReadCommandInputsV2{}, err
	}
	workspace, err := o.workspaces.InspectWorkspaceViewCurrentV1(ctx, source.WorkspaceView)
	if err != nil {
		return workspaceReadCommandInputsV2{}, err
	}
	workspace, err = detachWorkspaceReadCommandInputV2(workspace)
	if err != nil {
		return workspaceReadCommandInputsV2{}, err
	}
	checked := o.clock()
	if checked.IsZero() || checked.Before(start) {
		return workspaceReadCommandInputsV2{}, sandboxports.ErrConflict
	}
	result := workspaceReadCommandInputsV2{
		expectedSource: sourceRef,
		source:         source, effect: effect, prepared: prepared,
		workspace: workspace, checked: checked,
	}
	return result, result.validateCurrent(checked)
}

func (v workspaceReadCommandInputsV2) validateCurrent(now time.Time) error {
	if err := v.source.ValidateCurrent(v.expectedSource, now); err != nil {
		return err
	}
	if err := v.effect.Validate(now); err != nil {
		return err
	}
	if err := v.prepared.Validate(); err != nil ||
		v.prepared.CheckedUnixNano > now.UnixNano() ||
		now.UnixNano() >= v.prepared.ExpiresUnixNano {
		return sandboxports.ErrConflict
	}
	return v.workspace.ValidateCurrent(now)
}

func validateWorkspaceReadCommandInputPairV2(
	s1, s2 workspaceReadCommandInputsV2,
) error {
	if s1.expectedSource != s2.expectedSource ||
		!reflect.DeepEqual(s1.source, s2.source) ||
		!reflect.DeepEqual(s1.effect, s2.effect) ||
		!reflect.DeepEqual(s1.prepared, s2.prepared) ||
		!reflect.DeepEqual(s1.workspace, s2.workspace) ||
		s2.checked.Before(s1.checked) {
		return sandboxports.ErrConflict
	}
	return nil
}

func detachWorkspaceReadCommandInputV2[T any](value T) (T, error) {
	var clone T
	body, err := json.Marshal(value)
	if err != nil {
		return clone, sandboxports.ErrConflict
	}
	if err = runtimecore.DecodeStrictJSON(body, &clone); err != nil {
		return clone, sandboxports.ErrConflict
	}
	return clone, nil
}

func workspaceReadCommandOwnerCurrentBodyV2(
	command contract.WorkspaceReadCommandV1,
	publication contract.WorkspaceReadCommandPublicationV2,
	inputs workspaceReadCommandInputsV2,
) contract.WorkspaceReadCommandOwnerCurrentV2 {
	workspaceDigest, _ := contract.Digest("workspace-read-workspace-semantic-v2", inputs.workspace)
	return contract.WorkspaceReadCommandOwnerCurrentV2{
		Command:                         command.Meta.Ref(),
		Publication:                     publication.Meta.Ref(),
		PublicationSemanticDigest:       publication.Semantic.Digest,
		SourceCommand:                   publication.Semantic.SourceCommand,
		SourceSemanticDigest:            publication.Semantic.SourceSemanticDigest,
		SourceProjectionDigest:          inputs.source.ProjectionDigest,
		SourceCheckedUnixNano:           inputs.source.CheckedUnixNano,
		SourceExpiresUnixNano:           inputs.source.ExpiresUnixNano,
		RuntimeEffectProjectionDigest:   inputs.effect.Digest,
		RuntimeEffectCheckedUnixNano:    inputs.effect.CheckedUnixNano,
		RuntimeEffectExpiresUnixNano:    inputs.effect.ExpiresUnixNano,
		RuntimePreparedProjectionDigest: inputs.prepared.ProjectionDigest,
		RuntimePreparedCheckedUnixNano:  inputs.prepared.CheckedUnixNano,
		RuntimePreparedExpiresUnixNano:  inputs.prepared.ExpiresUnixNano,
		WorkspaceView:                   inputs.workspace.Meta.Ref(),
		WorkspaceSemanticDigest:         workspaceDigest,
		WorkspaceCheckedUnixNano:        inputs.checked.UnixNano(),
		WorkspaceExpiresUnixNano:        inputs.workspace.Meta.ExpiresUnixNano,
		WorkspaceLeaseExpiresUnixNano:   inputs.workspace.Lease.ExpiresUnixNano,
		SemanticNotAfterUnixNano:        publication.Semantic.SemanticNotAfterUnixNano,
	}
}

func (o *WorkspaceReadCommandOwnerV2) createInitialWorkspaceReadCommandV2(
	ctx context.Context,
	command contract.WorkspaceReadCommandV1,
	publication contract.WorkspaceReadCommandPublicationV2,
	inputs workspaceReadCommandInputsV2,
	commitNow time.Time,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	body := workspaceReadCommandOwnerCurrentBodyV2(command, publication, inputs)
	current, err := contract.SealInitialWorkspaceReadCommandOwnerCurrentV2(body, commitNow)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	capability, err := ownerworkspaceread.NewInitialCommandPublicationV2(
		command, publication, current,
		inputs.source, inputs.effect, inputs.prepared, inputs.workspace,
		commitNow,
	)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	stored, _, err := o.repository.ApplyWorkspaceReadCommandPublicationV2(ctx, capability)
	if err != nil {
		return o.recoverWorkspaceReadCommandCommitV2(
			ctx, current, nil, inputs, commitNow, err,
		)
	}
	return o.validateStoredWorkspaceReadCommandV2(ctx, stored, inputs, commitNow)
}

func (o *WorkspaceReadCommandOwnerV2) refreshOrReuseWorkspaceReadCommandV2(
	ctx context.Context,
	candidateCommand contract.WorkspaceReadCommandV1,
	candidatePublication contract.WorkspaceReadCommandPublicationV2,
	existing contract.WorkspaceReadCommandOwnerCurrentV2,
	inputs workspaceReadCommandInputsV2,
	commitNow time.Time,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	storedCommand, err := o.repository.InspectStoredWorkspaceReadCommandExactV1(ctx, existing.Command)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	storedPublication, err := o.repository.InspectStoredWorkspaceReadCommandPublicationExactV2(ctx, existing.Publication)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	if storedCommand.Meta.Ref() != candidateCommand.Meta.Ref() ||
		storedPublication.Meta.Ref() != candidatePublication.Meta.Ref() ||
		contract.ValidateWorkspaceReadCommandOwnerClosureV2(storedCommand, storedPublication, existing) != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if err = contract.ValidateWorkspaceReadCommandOwnerFreshClosureV2(
		existing, inputs.source, inputs.effect, inputs.prepared, inputs.workspace, commitNow,
	); err == nil {
		return o.validateStoredWorkspaceReadCommandV2(ctx, existing, inputs, commitNow)
	}
	body := workspaceReadCommandOwnerCurrentBodyV2(storedCommand, storedPublication, inputs)
	next, err := contract.SealNextWorkspaceReadCommandOwnerCurrentV2(body, existing, commitNow)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	capability, err := ownerworkspaceread.NewRefreshCommandPublicationV2(
		storedCommand, storedPublication, existing, next,
		inputs.source, inputs.effect, inputs.prepared, inputs.workspace,
		commitNow,
	)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	stored, _, err := o.repository.ApplyWorkspaceReadCommandPublicationV2(ctx, capability)
	if err != nil {
		return o.recoverWorkspaceReadCommandCommitV2(
			ctx, next, &existing, inputs, commitNow, err,
		)
	}
	return o.validateStoredWorkspaceReadCommandV2(ctx, stored, inputs, commitNow)
}

func (o *WorkspaceReadCommandOwnerV2) validateStoredWorkspaceReadCommandV2(
	ctx context.Context,
	current contract.WorkspaceReadCommandOwnerCurrentV2,
	inputs workspaceReadCommandInputsV2,
	notBefore time.Time,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	if o == nil || ctx == nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if err := current.ValidateShape(); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	command, err := o.repository.InspectStoredWorkspaceReadCommandExactV1(ctx, current.Command)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, referencedWorkspaceReadCommandErrorV2(err)
	}
	publication, err := o.repository.InspectStoredWorkspaceReadCommandPublicationExactV2(ctx, current.Publication)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, referencedWorkspaceReadCommandErrorV2(err)
	}
	pointer, err := o.repository.InspectStoredWorkspaceReadCommandOwnerCurrentByCommandV2(ctx, current.Command)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, referencedWorkspaceReadCommandErrorV2(err)
	}
	if pointer.Meta.Ref() != current.Meta.Ref() ||
		pointer.Command != current.Command ||
		pointer.Publication != current.Publication ||
		err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if err = contract.ValidateWorkspaceReadCommandOwnerClosureV2(command, publication, current); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	now := o.clock()
	if now.IsZero() || now.Before(notBefore) || now.Before(inputs.checked) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if err = contract.ValidateWorkspaceReadCommandOwnerFreshClosureV2(
		current, inputs.source, inputs.effect, inputs.prepared, inputs.workspace, now,
	); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	finalPointer, err := o.repository.InspectStoredWorkspaceReadCommandOwnerCurrentByCommandV2(ctx, current.Command)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, referencedWorkspaceReadCommandErrorV2(err)
	}
	if finalPointer.Meta.Ref() != current.Meta.Ref() ||
		finalPointer.Command != current.Command ||
		finalPointer.Publication != current.Publication {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	return current, nil
}

func (o *WorkspaceReadCommandOwnerV2) recoverWorkspaceReadCommandCommitV2(
	ctx context.Context,
	candidate contract.WorkspaceReadCommandOwnerCurrentV2,
	expected *contract.WorkspaceReadCommandOwnerCurrentV2,
	inputs workspaceReadCommandInputsV2,
	now time.Time,
	commitErr error,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	if !errors.Is(commitErr, sandboxports.ErrUnknownOutcome) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, commitErr
	}
	recoveryNow := o.clock()
	if recoveryNow.IsZero() || recoveryNow.Before(now) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if recoveryNow.UnixNano() >= candidate.ExpiresUnixNano {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, commitErr
	}
	recoveryTimeout := 2 * time.Second
	remaining := time.Duration(candidate.ExpiresUnixNano - recoveryNow.UnixNano())
	if remaining < recoveryTimeout {
		recoveryTimeout = remaining
	}
	if recoveryTimeout <= 0 {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, commitErr
	}
	recoveryContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), recoveryTimeout)
	defer cancel()
	if expected == nil {
		return o.recoverInitialWorkspaceReadCommandCommitV2(
			recoveryContext, candidate, commitErr, recoveryNow,
		)
	}
	return o.recoverRefreshWorkspaceReadCommandCommitV2(
		recoveryContext, candidate, *expected, commitErr, recoveryNow,
	)
}

func (o *WorkspaceReadCommandOwnerV2) recoverInitialWorkspaceReadCommandCommitV2(
	ctx context.Context,
	candidate contract.WorkspaceReadCommandOwnerCurrentV2,
	commitErr error,
	notBefore time.Time,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	command, commandErr := o.repository.InspectStoredWorkspaceReadCommandExactV1(ctx, candidate.Command)
	publication, publicationErr := o.repository.InspectStoredWorkspaceReadCommandPublicationExactV2(ctx, candidate.Publication)
	current, currentErr := o.repository.InspectStoredWorkspaceReadCommandOwnerCurrentExactV2(ctx, candidate.Meta.Ref())
	for _, inspectErr := range []error{commandErr, publicationErr, currentErr} {
		if inspectErr != nil && !errors.Is(inspectErr, sandboxports.ErrNotFound) {
			return contract.WorkspaceReadCommandOwnerCurrentV2{}, inspectErr
		}
	}
	found := []bool{commandErr == nil, publicationErr == nil, currentErr == nil}
	if !found[0] && !found[1] && !found[2] {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, commitErr
	}
	if !found[0] || !found[1] || !found[2] ||
		command.Meta.Ref() != candidate.Command ||
		publication.Meta.Ref() != candidate.Publication ||
		current.Meta.Ref() != candidate.Meta.Ref() ||
		!reflect.DeepEqual(current, candidate) ||
		contract.ValidateWorkspaceReadCommandOwnerClosureV2(command, publication, current) != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	return o.validateRecoveredWorkspaceReadCommandCurrentV2(current, notBefore)
}

func (o *WorkspaceReadCommandOwnerV2) recoverRefreshWorkspaceReadCommandCommitV2(
	ctx context.Context,
	candidate contract.WorkspaceReadCommandOwnerCurrentV2,
	expected contract.WorkspaceReadCommandOwnerCurrentV2,
	commitErr error,
	notBefore time.Time,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	current, currentErr := o.repository.InspectStoredWorkspaceReadCommandOwnerCurrentExactV2(ctx, candidate.Meta.Ref())
	if errors.Is(currentErr, sandboxports.ErrNotFound) {
		pointer, pointerErr := o.repository.InspectStoredWorkspaceReadCommandOwnerCurrentByCommandV2(ctx, candidate.Command)
		if pointerErr != nil {
			if errors.Is(pointerErr, sandboxports.ErrNotFound) {
				return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
			}
			return contract.WorkspaceReadCommandOwnerCurrentV2{}, pointerErr
		}
		if pointer.Command != candidate.Command ||
			pointer.Publication != candidate.Publication {
			return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
		}
		if pointer.Meta.Ref() == expected.Meta.Ref() {
			return contract.WorkspaceReadCommandOwnerCurrentV2{}, commitErr
		}
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if currentErr != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, currentErr
	}
	pointer, pointerErr := o.repository.InspectStoredWorkspaceReadCommandOwnerCurrentByCommandV2(ctx, candidate.Command)
	if pointerErr != nil {
		if errors.Is(pointerErr, sandboxports.ErrNotFound) {
			return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
		}
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, pointerErr
	}
	if pointer.Command != candidate.Command ||
		pointer.Publication != candidate.Publication {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if pointer.Meta.Ref() == expected.Meta.Ref() {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if pointer.Meta.Ref() != candidate.Meta.Ref() ||
		current.Meta.Ref() != candidate.Meta.Ref() ||
		!reflect.DeepEqual(current, candidate) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	return o.validateRecoveredWorkspaceReadCommandCurrentV2(current, notBefore)
}

func (o *WorkspaceReadCommandOwnerV2) validateRecoveredWorkspaceReadCommandCurrentV2(
	current contract.WorkspaceReadCommandOwnerCurrentV2,
	notBefore time.Time,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	finalNow := o.clock()
	if finalNow.IsZero() || finalNow.Before(notBefore) {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if err := current.ValidateCurrent(finalNow); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	return current, nil
}

func (o *WorkspaceReadCommandOwnerV2) InspectWorkspaceReadCommandPublicationExactV2(
	ctx context.Context,
	exact contract.Ref,
) (contract.WorkspaceReadCommandPublicationV2, error) {
	if o == nil || ctx == nil {
		return contract.WorkspaceReadCommandPublicationV2{}, sandboxports.ErrConflict
	}
	if err := exact.ValidateShape("workspace read Command publication"); err != nil {
		return contract.WorkspaceReadCommandPublicationV2{}, err
	}
	publication, err := o.repository.InspectStoredWorkspaceReadCommandPublicationExactV2(ctx, exact)
	if err != nil {
		return contract.WorkspaceReadCommandPublicationV2{}, err
	}
	if publication.Meta.Ref() != exact {
		return contract.WorkspaceReadCommandPublicationV2{}, sandboxports.ErrConflict
	}
	return publication, nil
}

func (o *WorkspaceReadCommandOwnerV2) InspectWorkspaceReadCommandOwnerCurrentExactV2(
	ctx context.Context,
	exact contract.Ref,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	if o == nil || ctx == nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if err := exact.ValidateShape("workspace read Command Owner current"); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	current, err := o.repository.InspectStoredWorkspaceReadCommandOwnerCurrentExactV2(ctx, exact)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	if current.Meta.Ref() != exact {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	return o.inspectFreshWorkspaceReadCommandCurrentV2(ctx, current)
}

func (o *WorkspaceReadCommandOwnerV2) InspectWorkspaceReadCommandOwnerCurrentByCommandV2(
	ctx context.Context,
	command contract.Ref,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	if o == nil || ctx == nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if err := command.ValidateShape("workspace read Command"); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	current, err := o.repository.InspectStoredWorkspaceReadCommandOwnerCurrentByCommandV2(ctx, command)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	if current.Command != command {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	return o.inspectFreshWorkspaceReadCommandCurrentV2(ctx, current)
}

// InspectWorkspaceReadCommandExactV1 preserves the immutable v18 historical
// reader. It validates the exact stored body but never treats it as current or
// restores physical execution eligibility.
func (o *WorkspaceReadCommandOwnerV2) InspectWorkspaceReadCommandExactV1(
	ctx context.Context,
	exact contract.Ref,
) (contract.WorkspaceReadCommandV1, error) {
	if o == nil || ctx == nil {
		return contract.WorkspaceReadCommandV1{}, sandboxports.ErrConflict
	}
	if err := exact.ValidateShape("workspace read Command"); err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	command, err := o.repository.InspectStoredWorkspaceReadCommandExactV1(ctx, exact)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	if command.Meta.Ref() != exact || command.ValidateShape() != nil {
		return contract.WorkspaceReadCommandV1{}, sandboxports.ErrConflict
	}
	return command, nil
}

func (o *WorkspaceReadCommandOwnerV2) inspectFreshWorkspaceReadCommandCurrentV2(
	ctx context.Context,
	current contract.WorkspaceReadCommandOwnerCurrentV2,
) (contract.WorkspaceReadCommandOwnerCurrentV2, error) {
	if o == nil || ctx == nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if err := current.ValidateShape(); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	pointer, err := o.repository.InspectStoredWorkspaceReadCommandOwnerCurrentByCommandV2(ctx, current.Command)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, referencedWorkspaceReadCommandErrorV2(err)
	}
	if pointer.Meta.Ref() != current.Meta.Ref() ||
		pointer.Command != current.Command ||
		pointer.Publication != current.Publication {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	command, err := o.repository.InspectStoredWorkspaceReadCommandExactV1(ctx, current.Command)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, referencedWorkspaceReadCommandErrorV2(err)
	}
	publication, err := o.repository.InspectStoredWorkspaceReadCommandPublicationExactV2(ctx, current.Publication)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, referencedWorkspaceReadCommandErrorV2(err)
	}
	if err = contract.ValidateWorkspaceReadCommandOwnerClosureV2(command, publication, current); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	history, err := o.repository.InspectStoredWorkspaceReadCommandOwnerHistoryV2(
		ctx,
		current.Meta.ID,
		current.Command,
	)
	if err != nil || validateWorkspaceReadCommandOwnerHistoryV2(
		command,
		publication,
		history,
		pointer,
	) != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	inputs, err := o.readWorkspaceReadCommandInputsV2(ctx, current.SourceCommand)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	finalPointer, err := o.repository.InspectStoredWorkspaceReadCommandOwnerCurrentByCommandV2(ctx, current.Command)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, referencedWorkspaceReadCommandErrorV2(err)
	}
	now := o.clock()
	if now.IsZero() || now.Before(inputs.checked) ||
		finalPointer.Meta.Ref() != current.Meta.Ref() ||
		finalPointer.Command != current.Command ||
		finalPointer.Publication != current.Publication {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	finalHistory, err := o.repository.InspectStoredWorkspaceReadCommandOwnerHistoryV2(
		ctx,
		current.Meta.ID,
		current.Command,
	)
	if err != nil || validateWorkspaceReadCommandOwnerHistoryV2(
		command,
		publication,
		finalHistory,
		finalPointer,
	) != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	if err = contract.ValidateWorkspaceReadCommandOwnerFreshClosureV2(
		current, inputs.source, inputs.effect, inputs.prepared, inputs.workspace, now,
	); err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, err
	}
	s3Pointer, err := o.repository.InspectStoredWorkspaceReadCommandOwnerCurrentByCommandV2(ctx, current.Command)
	if err != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, referencedWorkspaceReadCommandErrorV2(err)
	}
	if s3Pointer.Meta.Ref() != current.Meta.Ref() ||
		s3Pointer.Command != current.Command ||
		s3Pointer.Publication != current.Publication {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	s3History, err := o.repository.InspectStoredWorkspaceReadCommandOwnerHistoryV2(
		ctx,
		current.Meta.ID,
		current.Command,
	)
	if err != nil || validateWorkspaceReadCommandOwnerHistoryV2(
		command,
		publication,
		s3History,
		s3Pointer,
	) != nil {
		return contract.WorkspaceReadCommandOwnerCurrentV2{}, sandboxports.ErrConflict
	}
	return current, nil
}

func validateWorkspaceReadCommandOwnerHistoryV2(
	command contract.WorkspaceReadCommandV1,
	publication contract.WorkspaceReadCommandPublicationV2,
	history []contract.WorkspaceReadCommandOwnerCurrentV2,
	pointer contract.WorkspaceReadCommandOwnerCurrentV2,
) error {
	if len(history) == 0 {
		return sandboxports.ErrConflict
	}
	for index, current := range history {
		if current.Meta.Revision != uint64(index+1) ||
			contract.ValidateWorkspaceReadCommandOwnerClosureV2(command, publication, current) != nil {
			return sandboxports.ErrConflict
		}
		if index == 0 {
			if current.Meta.CreatedUnixNano != current.CheckedUnixNano {
				return sandboxports.ErrConflict
			}
			continue
		}
		expected, err := contract.SealNextWorkspaceReadCommandOwnerCurrentV2(
			current,
			history[index-1],
			time.Unix(0, current.CheckedUnixNano),
		)
		if err != nil || !reflect.DeepEqual(expected, current) {
			return sandboxports.ErrConflict
		}
	}
	if pointer.Meta.Ref() != history[len(history)-1].Meta.Ref() ||
		!reflect.DeepEqual(pointer, history[len(history)-1]) {
		return sandboxports.ErrConflict
	}
	return nil
}

// InspectWorkspaceReadCommandCurrentV1 is the legacy compatibility shim. A
// raw V1 Command is never current by itself: the v19 Publication/OwnerCurrent
// closure and fresh upstream owner reads must both pass.
func (o *WorkspaceReadCommandOwnerV2) InspectWorkspaceReadCommandCurrentV1(
	ctx context.Context,
	exact contract.Ref,
) (contract.WorkspaceReadCommandV1, error) {
	if o == nil || ctx == nil {
		return contract.WorkspaceReadCommandV1{}, sandboxports.ErrConflict
	}
	current, err := o.InspectWorkspaceReadCommandOwnerCurrentByCommandV2(ctx, exact)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	if current.Command != exact {
		return contract.WorkspaceReadCommandV1{}, sandboxports.ErrConflict
	}
	command, err := o.repository.InspectStoredWorkspaceReadCommandExactV1(ctx, current.Command)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, referencedWorkspaceReadCommandErrorV2(err)
	}
	if command.Meta.Ref() != exact || command.Meta.Ref() != current.Command {
		return contract.WorkspaceReadCommandV1{}, sandboxports.ErrConflict
	}
	finalCurrent, err := o.inspectFreshWorkspaceReadCommandCurrentV2(ctx, current)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	if finalCurrent.Meta.Ref() != current.Meta.Ref() {
		return contract.WorkspaceReadCommandV1{}, sandboxports.ErrConflict
	}
	finalCommand, err := o.repository.InspectStoredWorkspaceReadCommandExactV1(ctx, exact)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, referencedWorkspaceReadCommandErrorV2(err)
	}
	finalInputs, err := o.readWorkspaceReadCommandInputsV2(ctx, finalCurrent.SourceCommand)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	pointer, err := o.repository.InspectStoredWorkspaceReadCommandOwnerCurrentByCommandV2(ctx, exact)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, referencedWorkspaceReadCommandErrorV2(err)
	}
	finalNow := o.clock()
	if finalNow.IsZero() ||
		finalNow.Before(finalInputs.checked) ||
		finalCommand.Meta.Ref() != exact ||
		finalCommand.Meta.Ref() != finalCurrent.Command ||
		pointer.Meta.Ref() != finalCurrent.Meta.Ref() {
		return contract.WorkspaceReadCommandV1{}, sandboxports.ErrConflict
	}
	if err = contract.ValidateWorkspaceReadCommandOwnerFreshClosureV2(
		finalCurrent,
		finalInputs.source,
		finalInputs.effect,
		finalInputs.prepared,
		finalInputs.workspace,
		finalNow,
	); err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	if err = finalCommand.ValidateCurrent(finalNow); err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	return finalCommand, nil
}

func (o *WorkspaceReadCommandOwnerV2) InspectWorkspaceReadPublishedCommandCurrentV2(
	ctx context.Context,
	exact contract.Ref,
) (contract.WorkspaceReadCommandV1, error) {
	return o.InspectWorkspaceReadCommandCurrentV1(ctx, exact)
}

func referencedWorkspaceReadCommandErrorV2(err error) error {
	if errors.Is(err, sandboxports.ErrNotFound) {
		return sandboxports.ErrConflict
	}
	return err
}

var _ sandboxports.WorkspaceReadCommandEnsurePortV2 = (*WorkspaceReadCommandOwnerV2)(nil)
var _ sandboxports.WorkspaceReadCommandPublicationExactReaderV2 = (*WorkspaceReadCommandOwnerV2)(nil)
var _ sandboxports.WorkspaceReadCommandOwnerCurrentReaderV2 = (*WorkspaceReadCommandOwnerV2)(nil)
var _ sandboxports.WorkspaceReadCommandCurrentReaderV1 = (*WorkspaceReadCommandOwnerV2)(nil)
var _ sandboxports.WorkspaceReadPublishedCommandCurrentReaderV2 = (*WorkspaceReadCommandOwnerV2)(nil)
var _ sandboxports.WorkspaceReadCommandExactReaderV1 = (*WorkspaceReadCommandOwnerV2)(nil)

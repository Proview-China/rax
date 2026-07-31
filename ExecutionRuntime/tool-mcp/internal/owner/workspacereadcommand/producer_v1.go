package workspacereadcommand

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	tooladapter "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/applicationadapter"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	ownerrepo "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/owner/workspacereadcommandrepo"
)

const defaultRecoveryTimeoutV1 = 5 * time.Second

// CreateRequestV1 carries only exact lookup coordinates into the Tool-owned
// producer. None of its bodies are authoritative; every body is reread from
// its owning store before a command can be created.
type CreateRequestV1 struct {
	RequestKey        applicationcontract.SingleCallToolActionInspectKeyV2
	Binding           toolcontract.SingleCallToolActionBindingCurrentRefV2
	Action            toolcontract.ObjectRef
	Operation         runtimeports.OperationSubjectV3
	Prepared          runtimeports.PreparedProviderAttemptRefV2
	RuntimeAttempt    runtimeports.OperationDispatchAttemptRefV3
	RequestedNotAfter int64
}

func (r CreateRequestV1) validate() error {
	if r.RequestKey.Validate() != nil || r.Binding.Validate() != nil ||
		r.Action.Validate() != nil || r.Operation.Validate() != nil ||
		r.Prepared.Validate() != nil || r.RuntimeAttempt.Validate() != nil ||
		r.RequestedNotAfter <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "workspace.read execution command lookup coordinates are invalid")
	}
	operationDigest, err := r.Operation.DigestV3()
	if err != nil || operationDigest != r.Prepared.OperationDigest ||
		operationDigest != r.RuntimeAttempt.OperationDigest ||
		r.Prepared.IntentID != r.RuntimeAttempt.EffectID ||
		r.Prepared.IntentRevision != r.RuntimeAttempt.IntentRevision ||
		r.Prepared.IntentDigest != r.RuntimeAttempt.IntentDigest ||
		r.Prepared.PermitID != r.RuntimeAttempt.PermitID ||
		r.Prepared.PermitRevision != r.RuntimeAttempt.PermitRevision ||
		r.Prepared.PermitDigest != r.RuntimeAttempt.PermitDigest ||
		r.Prepared.AttemptID != r.RuntimeAttempt.AttemptID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "workspace.read execution command lookup coordinates drifted")
	}
	return nil
}

type ClockV1 interface {
	Now() time.Time
}

// ProducerV1 is Tool-owner internal by Go package boundary. It is the only
// constructor of command facts and current projections.
type ProducerV1 struct {
	claims         tooladapter.ToolOwnerSingleCallClaimStoreV2
	states         tooladapter.ToolOwnerSingleCallExecutionStateStoreV2
	bindings       tooladapter.SingleCallToolActionBindingCurrentReaderV2
	inputContracts toolcontract.ToolInputContractCurrentReaderV1
	registry       toolcontract.ToolRegistryObjectCurrentReaderV1
	effects        runtimeports.ControlledOperationEffectCurrentReaderV2
	prepared       runtimeports.ControlledOperationPreparedCurrentReaderV2
	repository     ownerrepo.RepositoryV1
	clock          ClockV1
	recovery       time.Duration
	clockMu        sync.Mutex
	lastNow        time.Time
}

func NewProducerV1(
	claims tooladapter.ToolOwnerSingleCallClaimStoreV2,
	states tooladapter.ToolOwnerSingleCallExecutionStateStoreV2,
	bindings tooladapter.SingleCallToolActionBindingCurrentReaderV2,
	inputContracts toolcontract.ToolInputContractCurrentReaderV1,
	registry toolcontract.ToolRegistryObjectCurrentReaderV1,
	effects runtimeports.ControlledOperationEffectCurrentReaderV2,
	prepared runtimeports.ControlledOperationPreparedCurrentReaderV2,
	repository ownerrepo.RepositoryV1,
	clock ClockV1,
) (*ProducerV1, error) {
	return NewProducerWithRecoveryTimeoutV1(
		claims, states, bindings, inputContracts, registry, effects, prepared,
		repository, clock, defaultRecoveryTimeoutV1,
	)
}

func NewProducerWithRecoveryTimeoutV1(
	claims tooladapter.ToolOwnerSingleCallClaimStoreV2,
	states tooladapter.ToolOwnerSingleCallExecutionStateStoreV2,
	bindings tooladapter.SingleCallToolActionBindingCurrentReaderV2,
	inputContracts toolcontract.ToolInputContractCurrentReaderV1,
	registry toolcontract.ToolRegistryObjectCurrentReaderV1,
	effects runtimeports.ControlledOperationEffectCurrentReaderV2,
	prepared runtimeports.ControlledOperationPreparedCurrentReaderV2,
	repository ownerrepo.RepositoryV1,
	clock ClockV1,
	recovery time.Duration,
) (*ProducerV1, error) {
	for _, dependency := range []any{claims, states, bindings, inputContracts, registry, effects, prepared, repository, clock} {
		if nilDependencyV1(dependency) {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "workspace.read execution command producer dependencies are incomplete")
		}
	}
	if recovery <= 0 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "workspace.read execution command recovery timeout is invalid")
	}
	return &ProducerV1{
		claims: claims, states: states, bindings: bindings, inputContracts: inputContracts,
		registry: registry, effects: effects, prepared: prepared, repository: repository,
		clock: clock, recovery: recovery,
	}, nil
}

func (p *ProducerV1) CreateOrInspectV1(
	ctx context.Context,
	request CreateRequestV1,
) (toolcontract.WorkspaceReadExecutionCommandV1, error) {
	if ctx == nil || p == nil || request.validate() != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "workspace.read execution command creation is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	historical, inspectErr := p.repository.InspectWorkspaceReadExecutionCommandByRuntimeAttemptV1(
		ctx, request.RuntimeAttempt,
	)
	if inspectErr == nil {
		if err := validateHistoricalAgainstRequestV1(historical, request); err != nil {
			return toolcontract.WorkspaceReadExecutionCommandV1{}, err
		}
		return toolcontract.CloneWorkspaceReadExecutionCommandV1(historical), nil
	}
	if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, inspectErr
	}
	first, _, err := p.readSnapshotV1(ctx, request)
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	second, _, err := p.readSnapshotV1(ctx, request)
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	if !sameSnapshotV1(first, second) {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "workspace.read execution command sources changed during S1/S2")
	}
	commitNow, err := p.nextNowV1()
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	if err = second.validateCurrentV1(request, commitNow); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	fact, err := buildFactV1(request, second, commitNow.UnixNano())
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	winner, _, err := p.repository.CreateWorkspaceReadExecutionCommandOwnedV1(
		ctx, ownerrepo.NewWriteCapabilityV1(), fact,
	)
	if err == nil {
		same, compareErr := toolcontract.SameWorkspaceReadExecutionCommandStableClosureV1(winner, fact)
		if compareErr != nil {
			return toolcontract.WorkspaceReadExecutionCommandV1{}, compareErr
		}
		if !same {
			return toolcontract.WorkspaceReadExecutionCommandV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "workspace.read execution command winner differs from authoritative fact")
		}
		return toolcontract.CloneWorkspaceReadExecutionCommandV1(winner), nil
	}
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.recovery)
	defer cancel()
	recovered, inspectErr := p.repository.InspectWorkspaceReadExecutionCommandExactV1(recoveryCtx, fact.Ref)
	if inspectErr != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	if !reflect.DeepEqual(recovered, fact) {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "workspace.read execution command recovery found another fact")
	}
	return toolcontract.CloneWorkspaceReadExecutionCommandV1(recovered), nil
}

func (p *ProducerV1) InspectWorkspaceReadExecutionCommandExactV1(
	ctx context.Context,
	exact toolcontract.WorkspaceReadExecutionCommandRefV1,
) (toolcontract.WorkspaceReadExecutionCommandV1, error) {
	return p.repository.InspectWorkspaceReadExecutionCommandExactV1(ctx, exact)
}

func (p *ProducerV1) InspectWorkspaceReadExecutionCommandByRuntimeAttemptV1(
	ctx context.Context,
	attempt runtimeports.OperationDispatchAttemptRefV3,
) (toolcontract.WorkspaceReadExecutionCommandV1, error) {
	return p.repository.InspectWorkspaceReadExecutionCommandByRuntimeAttemptV1(ctx, attempt)
}

func (p *ProducerV1) InspectWorkspaceReadExecutionCommandCurrentV1(
	ctx context.Context,
	exact toolcontract.WorkspaceReadExecutionCommandRefV1,
) (toolcontract.WorkspaceReadExecutionCommandCurrentV1, error) {
	if ctx == nil || p == nil || exact.Validate() != nil {
		return toolcontract.WorkspaceReadExecutionCommandCurrentV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "workspace.read execution command current request is invalid")
	}
	fact, err := p.repository.InspectWorkspaceReadExecutionCommandExactV1(ctx, exact)
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandCurrentV1{}, err
	}
	request := requestFromFactV1(fact)
	first, _, err := p.readSnapshotV1(ctx, request)
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandCurrentV1{}, err
	}
	second, _, err := p.readSnapshotV1(ctx, request)
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandCurrentV1{}, err
	}
	if !sameSnapshotV1(first, second) {
		return toolcontract.WorkspaceReadExecutionCommandCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "workspace.read execution command current sources changed during S1/S2")
	}
	finalNow, err := p.nextNowV1()
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandCurrentV1{}, err
	}
	if err = second.validateCurrentV1(request, finalNow); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandCurrentV1{}, err
	}
	if err = validateFactAgainstSnapshotV1(fact, request, second, finalNow); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandCurrentV1{}, err
	}
	expires := fact.NotAfterUnixNano
	for _, upper := range []int64{
		finalNow.Add(toolcontract.MaxWorkspaceReadExecutionCommandCurrentTTLV1).UnixNano(),
		second.binding.ExpiresUnixNano,
		second.input.ExpiresUnixNano,
		second.tool.ExpiresUnixNano,
		second.effect.ExpiresUnixNano,
		second.prepared.ExpiresUnixNano,
	} {
		if upper < expires {
			expires = upper
		}
	}
	current := toolcontract.WorkspaceReadExecutionCommandCurrentV1{
		ContractVersion:                toolcontract.WorkspaceReadExecutionCommandContractVersionV1,
		Fact:                           toolcontract.CloneWorkspaceReadExecutionCommandV1(fact),
		ToolCurrentProjectionDigest:    second.tool.ProjectionDigest,
		ToolCurrentCheckedUnixNano:     second.tool.CheckedUnixNano,
		RuntimeEffectCurrentDigest:     second.effect.Digest,
		RuntimeEffectCheckedUnixNano:   second.effect.CheckedUnixNano,
		RuntimePreparedCurrentDigest:   second.prepared.ProjectionDigest,
		RuntimePreparedCheckedUnixNano: second.prepared.CheckedUnixNano,
		CheckedUnixNano:                finalNow.UnixNano(),
		ExpiresUnixNano:                expires,
	}
	current.Digest, err = current.ComputeDigestV1()
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandCurrentV1{}, err
	}
	if err = current.ValidateCurrent(exact, finalNow); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandCurrentV1{}, err
	}
	return current, nil
}

type snapshotV1 struct {
	claim      tooladapter.ToolOwnerSingleCallClaimRecordV2
	state      tooladapter.ToolOwnerSingleCallExecutionStateV2
	operation  runtimeports.OperationSubjectV3
	binding    tooladapter.SingleCallToolActionBindingCurrentProjectionV2
	input      toolcontract.ToolInputContractCurrentProjectionV1
	descriptor toolcontract.ToolDescriptor
	tool       toolcontract.ToolRegistryObjectCurrentProjectionV1
	effect     runtimeports.ControlledOperationEffectCurrentProjectionV2
	prepared   runtimeports.ControlledOperationPreparedCurrentProjectionV2
}

func (p *ProducerV1) readSnapshotV1(
	ctx context.Context,
	request CreateRequestV1,
) (snapshotV1, time.Time, error) {
	claim, err := p.claims.InspectToolOwnerSingleCallClaimV2(ctx, request.RequestKey)
	if err != nil {
		return snapshotV1{}, time.Time{}, err
	}
	operation := claim.Input.Request.Action.PendingSubject.Binding.OwnerInputs.ModelTurnOperation
	binding, err := p.bindings.InspectExactSingleCallToolActionBindingCurrentV2(
		ctx,
		tooladapter.SingleCallToolActionBindingInspectExactRequestV2{
			ApplicationRequest:       claim.Input.Request,
			SourceSubject:            claim.Input.Request.Action.PendingSubject,
			RequestedExpiresUnixNano: claim.Input.Binding.RequestedExpiresUnixNano,
			Expected:                 request.Binding,
		},
	)
	if err != nil {
		return snapshotV1{}, time.Time{}, err
	}
	state, err := p.states.InspectExecutionStateV2(ctx, request.RequestKey)
	if err != nil {
		return snapshotV1{}, time.Time{}, err
	}
	candidate := binding.CandidateClosure.Candidate
	inputResolve := inputResolveFromProjectionV1(binding.CandidateClosure.InputContract)
	input, err := p.inputContracts.InspectExactToolInputContractCurrentV1(
		ctx,
		toolcontract.ToolInputContractInspectExactRequestV1{
			ResolveRequest: inputResolve,
			Expected:       candidate.InputContractCurrentRef,
		},
	)
	if err != nil {
		return snapshotV1{}, time.Time{}, err
	}
	descriptor, toolCurrent, err := p.registry.InspectExactToolDescriptorCurrentV1(
		ctx, candidate.Tool, candidate.ToolCurrent,
	)
	if err != nil {
		return snapshotV1{}, time.Time{}, err
	}
	effect, err := p.effects.InspectCurrentControlledOperationEffectV2(
		ctx, request.Operation, request.RuntimeAttempt.EffectID,
	)
	if err != nil {
		return snapshotV1{}, time.Time{}, err
	}
	prepared, err := p.prepared.InspectCurrentControlledOperationPreparedV2(ctx, request.Prepared)
	if err != nil {
		return snapshotV1{}, time.Time{}, err
	}
	result := snapshotV1{
		claim: claim, state: state, operation: operation,
		binding:    tooladapter.CloneSingleCallToolActionBindingCurrentProjectionV2(binding),
		input:      toolcontract.CloneToolInputContractCurrentProjectionV1(input),
		descriptor: descriptor, tool: toolCurrent,
		effect: effect, prepared: prepared,
	}
	result, err = cloneSnapshotV1(result)
	if err != nil {
		return snapshotV1{}, time.Time{}, err
	}
	validatedAt, err := p.nextNowV1()
	if err != nil {
		return snapshotV1{}, time.Time{}, err
	}
	if err = result.validateCurrentV1(request, validatedAt); err != nil {
		return snapshotV1{}, time.Time{}, err
	}
	return result, validatedAt, nil
}

func (s snapshotV1) validateCurrentV1(request CreateRequestV1, now time.Time) error {
	if err := request.validate(); err != nil {
		return err
	}
	if err := s.claim.Validate(); err != nil {
		return err
	}
	if err := s.state.Validate(); err != nil || s.state.State != tooladapter.ToolOwnerExecutionStartCommittedV2 {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "workspace.read execution command requires exact start_committed Tool state")
	}
	if err := s.binding.ValidateCurrent(now); err != nil {
		return err
	}
	if err := s.input.ValidateCurrent(now); err != nil {
		return err
	}
	if err := s.descriptor.Validate(); err != nil {
		return err
	}
	if err := s.tool.ValidateCurrent(now); err != nil {
		return err
	}
	if err := s.effect.Validate(now); err != nil {
		return err
	}
	if s.effect.State != toolcontract.WorkspaceReadExecutionDispatchIntentV1 {
		return core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "workspace.read execution command requires Runtime dispatch_intent state")
	}
	if err := s.prepared.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < s.prepared.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "workspace.read execution command Runtime prepared clock regressed")
	}
	if !now.Before(time.Unix(0, s.prepared.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "workspace.read execution command Runtime prepared current expired")
	}
	expectedKey, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(s.claim.Input.Request)
	if err != nil {
		return err
	}
	inputDigest, err := tooladapter.ComputeToolOwnerSingleCallExecutionInputDigestV2(s.claim.Input)
	if err != nil {
		return err
	}
	candidate := s.binding.CandidateClosure.Candidate
	descriptorRef := toolcontract.ObjectRef{
		ID: string(s.descriptor.ID), Revision: s.descriptor.Revision, Digest: s.descriptor.Digest,
	}
	if expectedKey != request.RequestKey ||
		s.claim.Claim.BindingRef != request.Binding ||
		s.claim.Input.Binding.Ref != request.Binding ||
		s.binding.Ref != request.Binding ||
		s.binding.CandidateRef != request.Action ||
		candidate.ObjectRef() != request.Action ||
		s.state.RequestKey != request.RequestKey ||
		s.state.ClaimRef != (toolcontract.ObjectRef{ID: s.claim.Claim.ID, Revision: s.claim.Claim.Revision, Digest: s.claim.Claim.Digest}) ||
		s.state.BindingRef != request.Binding ||
		s.state.ExecutionInputDigest != inputDigest ||
		s.state.ExecutionAttemptID == "" ||
		!runtimeports.SameOperationSubjectV3(s.operation, request.Operation) ||
		candidate.SourceCandidate.CallName != toolcontract.CoreToolWorkspaceReadV1 ||
		candidate.InputContractCurrentRef != s.input.Ref ||
		candidate.ToolCurrent != s.tool.Ref ||
		candidate.Tool != s.tool.Object ||
		candidate.Tool != descriptorRef ||
		candidate.InputSchema != s.descriptor.InputSchema ||
		!toolcontract.ContainsName(s.descriptor.EffectKinds, runtimeports.NamespacedNameV2(candidate.EffectKind)) ||
		s.binding.CandidateClosure.InputContract.Ref != s.input.Ref ||
		s.binding.CandidateClosure.ToolCurrent.Ref != s.tool.Ref ||
		!reflect.DeepEqual(s.binding.CandidateClosure.InputContract, s.input) ||
		!sameToolRegistryStableClosureV1(s.binding.CandidateClosure.ToolCurrent, s.tool) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "workspace.read execution command Tool source closure drifted")
	}
	if err = candidate.ValidateAgainstInputContract(s.input); err != nil {
		return err
	}
	intent := s.effect.Intent
	if !runtimeports.SameOperationSubjectV3(intent.Operation, request.Operation) ||
		intent.ID != request.RuntimeAttempt.EffectID ||
		intent.Revision != request.RuntimeAttempt.IntentRevision ||
		s.effect.IntentDigest != request.RuntimeAttempt.IntentDigest ||
		intent.Target != candidate.ID ||
		intent.Review.CandidateDigest != candidate.Digest ||
		intent.Review.CandidateRevision != candidate.Revision ||
		intent.Kind != candidate.EffectKind ||
		intent.ActionScopeDigest != candidate.OperationScopeDigest ||
		!reflect.DeepEqual(intent.Payload, candidate.Payload) ||
		intent.PayloadRevision != candidate.PayloadRevision ||
		intent.Provider != s.prepared.Snapshot.Prepared.Provider ||
		intent.Idempotency.Key != candidate.IdempotencyKey ||
		!containsOwnerV1(intent.Owners, candidate.ExpectedOwner) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "workspace.read execution command Runtime Effect differs from Tool Candidate")
	}
	if s.prepared.Snapshot.Prepared != request.Prepared ||
		!toolcontract.SameWorkspaceReadExecutionRuntimeAttemptV1(s.prepared.Snapshot.Attempt, request.RuntimeAttempt) ||
		request.RuntimeAttempt.Delegation == nil ||
		!reflect.DeepEqual(*request.RuntimeAttempt.Delegation, s.prepared.Snapshot.Delegation) ||
		s.prepared.Snapshot.OperationDigest != request.RuntimeAttempt.OperationDigest ||
		s.prepared.Snapshot.EffectID != intent.ID ||
		s.prepared.Snapshot.IntentRevision != intent.Revision ||
		s.prepared.Snapshot.IntentDigest != s.effect.IntentDigest ||
		s.prepared.Snapshot.ProviderBinding != intent.Provider ||
		s.prepared.Snapshot.PayloadSchema != candidate.InputSchema ||
		s.prepared.Snapshot.PayloadDigest != candidate.Payload.ContentDigest ||
		s.prepared.Snapshot.PayloadRevision != candidate.PayloadRevision ||
		candidate.ExpectedOwner.ComponentID != s.prepared.Snapshot.ProviderBinding.ComponentID ||
		candidate.ExpectedOwner.ManifestDigest != s.prepared.Snapshot.ProviderBinding.ManifestDigest ||
		s.prepared.Snapshot.ProviderBinding.Capability != runtimeports.CapabilityNameV2(candidate.EffectKind) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "workspace.read execution command Runtime Prepared closure drifted")
	}
	for _, lower := range []int64{
		s.claim.Claim.CreatedUnixNano,
		s.state.UpdatedUnixNano,
		s.binding.CheckedUnixNano,
		candidate.CreatedUnixNano,
		s.input.CheckedUnixNano,
		s.tool.CheckedUnixNano,
		s.effect.CheckedUnixNano,
		s.prepared.Snapshot.Prepared.PreparedUnixNano,
		s.prepared.CheckedUnixNano,
	} {
		if now.UnixNano() < lower {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "workspace.read execution command clock predates an authoritative source")
		}
	}
	for _, upper := range []int64{
		request.RequestedNotAfter,
		s.claim.Input.Request.ExpiresUnixNano,
		s.state.ExpiresUnixNano,
		s.binding.ExpiresUnixNano,
		candidate.RequestedExpiresUnixNano,
		s.input.ExpiresUnixNano,
		s.tool.ExpiresUnixNano,
		s.effect.ExpiresUnixNano,
		s.prepared.Snapshot.Prepared.ExpiresUnixNano,
		s.prepared.ExpiresUnixNano,
	} {
		if !now.Before(time.Unix(0, upper)) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "workspace.read execution command source expired")
		}
	}
	return nil
}

func sameToolRegistryStableClosureV1(
	left toolcontract.ToolRegistryObjectCurrentProjectionV1,
	right toolcontract.ToolRegistryObjectCurrentProjectionV1,
) bool {
	return left.Ref == right.Ref &&
		left.Source == right.Source &&
		left.Object == right.Object &&
		left.RegistryOwner == right.RegistryOwner
}

func buildFactV1(
	request CreateRequestV1,
	s snapshotV1,
	createdUnixNano int64,
) (toolcontract.WorkspaceReadExecutionCommandV1, error) {
	candidate := s.binding.CandidateClosure.Candidate
	source, err := toolcontract.SealWorkspaceReadExecutionCommandSourceV1(
		toolcontract.WorkspaceReadExecutionCommandSourceV1{
			RequestKey: request.RequestKey,
			ClaimRef: toolcontract.ObjectRef{
				ID: s.claim.Claim.ID, Revision: s.claim.Claim.Revision, Digest: s.claim.Claim.Digest,
			},
			ExecutionStateRef:      s.state.RefV2(),
			ExecutionStateKind:     toolcontract.WorkspaceReadExecutionStartCommittedV1,
			ExecutionInputDigest:   s.state.ExecutionInputDigest,
			ToolExecutionAttemptID: s.state.ExecutionAttemptID,
			BindingCurrent:         s.binding.Ref,
			Candidate:              candidate.ObjectRef(),
			CandidateClosureDigest: s.binding.CandidateClosure.ClosureDigest,
			InputContractCurrent:   s.input.Ref,
			Tool:                   candidate.Tool,
			ToolCurrent:            s.tool.Ref,
			Owner:                  candidate.ExpectedOwner,
		},
	)
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	ttl, err := toolcontract.SealWorkspaceReadExecutionCommandTTLClosureV1(
		toolcontract.WorkspaceReadExecutionCommandTTLClosureV1{
			ClaimCreatedUnixNano:        s.claim.Claim.CreatedUnixNano,
			StateUpdatedUnixNano:        s.state.UpdatedUnixNano,
			BindingCheckedUnixNano:      s.binding.CheckedUnixNano,
			CandidateCreatedUnixNano:    candidate.CreatedUnixNano,
			InputCheckedUnixNano:        s.input.CheckedUnixNano,
			PreparedUnixNano:            s.prepared.Snapshot.Prepared.PreparedUnixNano,
			RequestedNotAfterUnixNano:   request.RequestedNotAfter,
			RequestExpiresUnixNano:      s.claim.Input.Request.ExpiresUnixNano,
			StateExpiresUnixNano:        s.state.ExpiresUnixNano,
			BindingExpiresUnixNano:      s.binding.ExpiresUnixNano,
			CandidateExpiresUnixNano:    candidate.RequestedExpiresUnixNano,
			InputExpiresUnixNano:        s.input.ExpiresUnixNano,
			EffectIntentExpiresUnixNano: s.effect.Intent.ExpiresUnixNano,
			PreparedExpiresUnixNano:     s.prepared.Snapshot.Prepared.ExpiresUnixNano,
		},
	)
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	operationDigest, err := request.Operation.DigestV3()
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	return toolcontract.SealWorkspaceReadExecutionCommandV1(
		toolcontract.WorkspaceReadExecutionCommandV1{
			Source:    source,
			Operation: request.Operation, OperationDigest: operationDigest,
			Prepared:                  s.prepared.Snapshot.Prepared,
			PreparedSemanticDigest:    s.prepared.Snapshot.SemanticDigest,
			RuntimeAttempt:            s.prepared.Snapshot.Attempt,
			RuntimeAttemptDigest:      mustAttemptDigestV1(s.prepared.Snapshot.Attempt),
			RuntimeEffectIntentDigest: s.effect.IntentDigest,
			RuntimeEffectFactRevision: s.effect.FactRevision,
			RuntimeEffectState:        s.effect.State,
			PayloadSchema:             s.prepared.Snapshot.PayloadSchema,
			PayloadDigest:             s.prepared.Snapshot.PayloadDigest,
			PayloadRevision:           s.prepared.Snapshot.PayloadRevision,
			TTL:                       ttl, CreatedUnixNano: createdUnixNano, NotAfterUnixNano: ttl.EffectiveNotAfterUnixNano,
		},
	)
}

func validateFactAgainstSnapshotV1(
	fact toolcontract.WorkspaceReadExecutionCommandV1,
	request CreateRequestV1,
	s snapshotV1,
	now time.Time,
) error {
	if err := fact.ValidateCurrent(now); err != nil {
		return err
	}
	expected, err := buildFactV1(request, s, fact.CreatedUnixNano)
	if err != nil {
		return err
	}
	same, err := toolcontract.SameWorkspaceReadExecutionCommandStableClosureV1(expected, fact)
	if err != nil {
		return err
	}
	if !same {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "workspace.read execution command differs from current authoritative sources")
	}
	return nil
}

func requestFromFactV1(fact toolcontract.WorkspaceReadExecutionCommandV1) CreateRequestV1 {
	return CreateRequestV1{
		RequestKey: fact.Source.RequestKey, Binding: fact.Source.BindingCurrent,
		Action: fact.Source.Candidate, Operation: fact.Operation, Prepared: fact.Prepared,
		RuntimeAttempt:    toolcontract.CloneWorkspaceReadExecutionCommandV1(fact).RuntimeAttempt,
		RequestedNotAfter: fact.TTL.RequestedNotAfterUnixNano,
	}
}

func inputResolveFromProjectionV1(
	input toolcontract.ToolInputContractCurrentProjectionV1,
) toolcontract.ToolInputContractResolveRequestV1 {
	binding := input.BindingSubject
	return toolcontract.ToolInputContractResolveRequestV1{
		ApplicationRequestID:       binding.ApplicationRequestID,
		ApplicationRequestRevision: binding.ApplicationRequestRevision,
		ApplicationRequestDigest:   binding.ApplicationRequestDigest,
		PendingAction:              binding.PendingAction,
		OperationScopeDigest:       binding.OperationScopeDigest,
		ProviderBinding:            binding.ProviderBinding,
		ExpectedOwner:              binding.ExpectedOwner,
		Surface:                    binding.Surface,
		CallName:                   binding.SurfaceEntry.ModelName,
		Capability:                 binding.Capability,
		Tool:                       binding.Tool,
		InputSchema:                binding.InputSchema,
		RequestedExpiresUnixNano:   input.RequestedExpiresUnixNano,
	}
}

func sameSnapshotV1(left, right snapshotV1) bool {
	leftStable, leftErr := left.stableSemanticV1()
	rightStable, rightErr := right.stableSemanticV1()
	return leftErr == nil && rightErr == nil && reflect.DeepEqual(leftStable, rightStable)
}

// stableSemanticV1 removes only the short-lived current envelopes that their
// owners are allowed to re-sign on every read. It retains every immutable
// source body and semantic digest, so same-Ref body drift still fails closed.
func (s snapshotV1) stableSemanticV1() (snapshotSemanticV1, error) {
	cloned, err := cloneSnapshotV1(s)
	if err != nil {
		return snapshotSemanticV1{}, err
	}
	return snapshotSemanticV1{
		Claim: cloned.claim, State: cloned.state, Operation: cloned.operation,
		Binding: cloned.binding, Input: cloned.input, Descriptor: cloned.descriptor,
		ToolRef: cloned.tool.Ref, ToolSource: cloned.tool.Source,
		ToolObject: cloned.tool.Object, ToolRegistryOwner: cloned.tool.RegistryOwner,
		EffectIntent: cloned.effect.Intent, EffectIntentDigest: cloned.effect.IntentDigest,
		EffectFactRevision: cloned.effect.FactRevision, EffectState: cloned.effect.State,
		PreparedSnapshot: cloned.prepared.Snapshot,
	}, nil
}

type snapshotSemanticV1 struct {
	Claim              tooladapter.ToolOwnerSingleCallClaimRecordV2
	State              tooladapter.ToolOwnerSingleCallExecutionStateV2
	Operation          runtimeports.OperationSubjectV3
	Binding            tooladapter.SingleCallToolActionBindingCurrentProjectionV2
	Input              toolcontract.ToolInputContractCurrentProjectionV1
	Descriptor         toolcontract.ToolDescriptor
	ToolRef            toolcontract.ToolRegistryObjectCurrentRefV1
	ToolSource         toolcontract.ToolRegistryRecordSourceV1
	ToolObject         toolcontract.ObjectRef
	ToolRegistryOwner  core.OwnerRef
	EffectIntent       runtimeports.OperationEffectIntentV3
	EffectIntentDigest core.Digest
	EffectFactRevision core.Revision
	EffectState        string
	PreparedSnapshot   runtimeports.ControlledOperationPreparedSemanticSnapshotV2
}

func cloneSnapshotV1(value snapshotV1) (snapshotV1, error) {
	payload, err := json.Marshal(value.exportedV1())
	if err != nil {
		return snapshotV1{}, err
	}
	var exported snapshotExportV1
	if err = json.Unmarshal(payload, &exported); err != nil {
		return snapshotV1{}, err
	}
	return exported.snapshotV1(), nil
}

type snapshotExportV1 struct {
	Claim      tooladapter.ToolOwnerSingleCallClaimRecordV2
	State      tooladapter.ToolOwnerSingleCallExecutionStateV2
	Operation  runtimeports.OperationSubjectV3
	Binding    tooladapter.SingleCallToolActionBindingCurrentProjectionV2
	Input      toolcontract.ToolInputContractCurrentProjectionV1
	Descriptor toolcontract.ToolDescriptor
	Tool       toolcontract.ToolRegistryObjectCurrentProjectionV1
	Effect     runtimeports.ControlledOperationEffectCurrentProjectionV2
	Prepared   runtimeports.ControlledOperationPreparedCurrentProjectionV2
}

func (s snapshotV1) exportedV1() snapshotExportV1 {
	return snapshotExportV1{
		Claim: s.claim, State: s.state, Operation: s.operation, Binding: s.binding,
		Input: s.input, Descriptor: s.descriptor, Tool: s.tool, Effect: s.effect,
		Prepared: s.prepared,
	}
}

func (s snapshotExportV1) snapshotV1() snapshotV1 {
	return snapshotV1{
		claim: s.Claim, state: s.State, operation: s.Operation, binding: s.Binding,
		input: s.Input, descriptor: s.Descriptor, tool: s.Tool, effect: s.Effect,
		prepared: s.Prepared,
	}
}

func validateHistoricalAgainstRequestV1(
	fact toolcontract.WorkspaceReadExecutionCommandV1,
	request CreateRequestV1,
) error {
	if err := fact.Validate(); err != nil {
		return err
	}
	if fact.Source.RequestKey != request.RequestKey ||
		fact.Source.BindingCurrent != request.Binding ||
		fact.Source.Candidate != request.Action ||
		!runtimeports.SameOperationSubjectV3(fact.Operation, request.Operation) ||
		fact.Prepared != request.Prepared ||
		!toolcontract.SameWorkspaceReadExecutionRuntimeAttemptV1(fact.RuntimeAttempt, request.RuntimeAttempt) ||
		fact.TTL.RequestedNotAfterUnixNano != request.RequestedNotAfter {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "workspace.read execution command historical request coordinates drifted")
	}
	return nil
}

func containsOwnerV1(values []runtimeports.EffectOwnerRefV2, expected runtimeports.EffectOwnerRefV2) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func mustAttemptDigestV1(attempt runtimeports.OperationDispatchAttemptRefV3) core.Digest {
	digest, _ := toolcontract.DigestWorkspaceReadExecutionRuntimeAttemptV1(attempt)
	return digest
}

func (p *ProducerV1) nextNowV1() (time.Time, error) {
	p.clockMu.Lock()
	defer p.clockMu.Unlock()
	now := p.clock.Now()
	if now.IsZero() || !p.lastNow.IsZero() && now.Before(p.lastNow) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "workspace.read execution command owner clock regressed")
	}
	p.lastNow = now
	return now, nil
}

func nilDependencyV1(value any) bool {
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

var _ toolcontract.WorkspaceReadExecutionCommandExactReaderV1 = (*ProducerV1)(nil)
var _ toolcontract.WorkspaceReadExecutionCommandAttemptReaderV1 = (*ProducerV1)(nil)
var _ toolcontract.WorkspaceReadExecutionCommandCurrentReaderV1 = (*ProducerV1)(nil)

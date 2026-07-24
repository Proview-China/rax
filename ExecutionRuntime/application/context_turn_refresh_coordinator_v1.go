package application

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const contextRefreshAttemptKindV1 = runtimeports.NamespacedNameV2("application/context-attempt")

type ContextTurnRefreshCoordinatorConfigV1 struct {
	Context   applicationports.ContextTurnRefreshPortV1
	Memory    applicationports.ContextOwnerSourceReaderV1
	Knowledge applicationports.ContextOwnerSourceReaderV1
	Clock     func() time.Time
}

type ContextTurnRefreshCoordinatorV1 struct {
	config ContextTurnRefreshCoordinatorConfigV1
	gates  singleCallCoordinatorGateV2
}

type ContextTurnRefreshCoordinationRequestV1 struct {
	ID                    string
	ExecutionScopeDigest  core.Digest
	RunID                 core.AgentRunID
	SourceSession         contract.SingleCallSessionCoordinateV1
	SessionApplicability  contract.SingleCallSessionApplicabilitySourceCoordinateV1
	SourceTurn            contract.SingleCallTurnCoordinateV1
	TurnApplicability     contract.SingleCallTurnApplicabilitySourceCoordinateV1
	OpaqueContextRequest  []byte
	Memory                *contract.ContextOwnerSourceRequestV1
	Knowledge             *contract.ContextOwnerSourceRequestV1
	RequestedNotAfterNano int64
}

func NewContextTurnRefreshCoordinatorV1(config ContextTurnRefreshCoordinatorConfigV1) (*ContextTurnRefreshCoordinatorV1, error) {
	if nilInterfaceV1(config.Context) || (nilInterfaceV1(config.Memory) && nilInterfaceV1(config.Knowledge)) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "context refresh coordinator requires Context and at least one Owner reader")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &ContextTurnRefreshCoordinatorV1{config: config, gates: singleCallCoordinatorGateV2{entries: make(map[string]*singleCallCoordinatorGateEntryV2)}}, nil
}

func (c *ContextTurnRefreshCoordinatorV1) CoordinateContextTurnRefreshV1(ctx context.Context, request ContextTurnRefreshCoordinationRequestV1) (contract.ContextTurnRefreshResultV1, error) {
	if c == nil || ctx == nil {
		return contract.ContextTurnRefreshResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "context refresh coordinator is nil")
	}
	release := c.gates.acquire(request.ID + "\x00" + string(request.ExecutionScopeDigest))
	defer release()
	now := c.config.Clock()
	if now.IsZero() || request.SourceTurn.Ordinal == ^uint32(0) || request.RequestedNotAfterNano <= 0 || !now.Before(time.Unix(0, request.RequestedNotAfterNano)) || (request.Memory == nil && request.Knowledge == nil) {
		return contract.ContextTurnRefreshResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context refresh coordination request is incomplete or expired")
	}
	memoryS1, err := c.inspectOwner(ctx, c.config.Memory, request.Memory, contract.ContextOwnerMemoryV1, now)
	if err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	knowledgeS1, err := c.inspectOwner(ctx, c.config.Knowledge, request.Knowledge, contract.ContextOwnerKnowledgeV1, now)
	if err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	prepare, err := contract.SealContextTurnRefreshPrepareRequestV1(contract.ContextTurnRefreshPrepareRequestV1{
		ID: request.ID, ExecutionScopeDigest: request.ExecutionScopeDigest, RunID: request.RunID,
		SourceSession: request.SourceSession, SessionApplicability: request.SessionApplicability,
		SourceTurn: request.SourceTurn, TurnApplicability: request.TurnApplicability, ExpectedTargetTurn: request.SourceTurn.Ordinal + 1,
		OpaqueContextRequest: request.OpaqueContextRequest, MemoryRequest: request.Memory, KnowledgeRequest: request.Knowledge, Memory: memoryS1, Knowledge: knowledgeS1,
		RequestedNotAfterNano: request.RequestedNotAfterNano,
	})
	if err != nil || prepare.ValidateCurrent(now) != nil {
		return contract.ContextTurnRefreshResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context refresh prepare envelope could not be sealed")
	}
	prepared, prepareErr := c.config.Context.PrepareContextTurnRefreshV1(ctx, prepare)
	if prepareErr != nil {
		inspected, inspectErr := c.inspectOriginal(context.WithoutCancel(ctx), prepare)
		if inspectErr != nil {
			return contract.ContextTurnRefreshResultV1{}, prepareErr
		}
		if inspected.State == contract.ContextTurnRefreshAppliedStateV1 {
			return inspected, nil
		}
		prepared, err = inspected.PreparedV1()
		if err != nil {
			return contract.ContextTurnRefreshResultV1{}, prepareErr
		}
	}
	now = c.config.Clock()
	if now.IsZero() || prepared.ValidateCurrent(now) != nil {
		return contract.ContextTurnRefreshResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "context refresh prepared projection expired before S2")
	}
	memoryRequestS2, memoryS2, err := c.inspectS2(ctx, c.config.Memory, request.Memory, memoryS1, now)
	if err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	knowledgeRequestS2, knowledgeS2, err := c.inspectS2(ctx, c.config.Knowledge, request.Knowledge, knowledgeS1, now)
	if err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	apply, err := contract.SealContextTurnRefreshApplyRequestV1(contract.ContextTurnRefreshApplyRequestV1{Prepared: prepared, MemoryRequest: memoryRequestS2, KnowledgeRequest: knowledgeRequestS2, Memory: memoryS2, Knowledge: knowledgeS2, RequestedNotAfterNano: request.RequestedNotAfterNano})
	if err != nil || apply.ValidateCurrent(now) != nil {
		return contract.ContextTurnRefreshResultV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context refresh S2 could not be bound to the prepared proof")
	}
	result, applyErr := c.config.Context.ApplyContextTurnRefreshV1(ctx, apply)
	if applyErr == nil {
		if result.Validate() != nil || result.State != contract.ContextTurnRefreshAppliedStateV1 {
			return contract.ContextTurnRefreshResultV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "Context returned a non-applied or invalid result")
		}
		return result, nil
	}
	inspected, inspectErr := c.inspectOriginal(context.WithoutCancel(ctx), prepare)
	if inspectErr == nil && inspected.State == contract.ContextTurnRefreshAppliedStateV1 {
		return inspected, nil
	}
	return contract.ContextTurnRefreshResultV1{}, applyErr
}

func (c *ContextTurnRefreshCoordinatorV1) inspectOwner(ctx context.Context, reader applicationports.ContextOwnerSourceReaderV1, request *contract.ContextOwnerSourceRequestV1, owner contract.ContextOwnerKindV1, now time.Time) (*contract.ContextOwnerSourceEnvelopeV1, error) {
	if request == nil {
		return nil, nil
	}
	if nilInterfaceV1(reader) || request.Owner != owner || request.Phase != contract.ContextSourceCheckS1V1 || request.ValidateCurrent(now) != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context refresh Owner source is not wired or not S1")
	}
	projection, err := reader.InspectContextOwnerSourceCurrentV1(ctx, *request)
	if err != nil {
		return nil, err
	}
	after := c.config.Clock()
	if after.IsZero() || after.Before(now) || projection.ValidateCurrent(after) != nil || projection.Owner != owner {
		return nil, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "context refresh Owner S1 projection drifted")
	}
	return &projection, nil
}

func (c *ContextTurnRefreshCoordinatorV1) inspectS2(ctx context.Context, reader applicationports.ContextOwnerSourceReaderV1, original *contract.ContextOwnerSourceRequestV1, s1 *contract.ContextOwnerSourceEnvelopeV1, now time.Time) (*contract.ContextOwnerSourceRequestV1, *contract.ContextOwnerSourceEnvelopeV1, error) {
	if original == nil {
		return nil, nil, nil
	}
	s2 := *original
	s2.Phase = contract.ContextSourceCheckS2V1
	s2.ExpectedOwnerClosure = s1.StableClosureDigest
	s2.ExpectedStableDigest = s1.StableAssociationDigest
	s2.Digest = ""
	sealed, err := contract.SealContextOwnerSourceRequestV1(s2)
	if err != nil {
		return nil, nil, err
	}
	projection, err := reader.InspectContextOwnerSourceCurrentV1(ctx, sealed)
	if err != nil {
		return nil, nil, err
	}
	after := c.config.Clock()
	if after.IsZero() || after.Before(now) || projection.ValidateCurrent(after) != nil || projection.Phase != contract.ContextSourceCheckS2V1 || projection.StableAssociationDigest != s1.StableAssociationDigest {
		return nil, nil, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "context refresh Owner stable association drifted at S2")
	}
	return &sealed, &projection, nil
}

func (c *ContextTurnRefreshCoordinatorV1) inspectOriginal(ctx context.Context, prepare contract.ContextTurnRefreshPrepareRequestV1) (contract.ContextTurnRefreshResultV1, error) {
	inspect, err := contract.SealContextTurnRefreshInspectRequestV1(contract.ContextTurnRefreshInspectRequestV1{AttemptRef: contract.ContextRefreshExactRefV1{Kind: contextRefreshAttemptKindV1, ID: prepare.ID, Revision: 1, Digest: prepare.Digest}})
	if err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	return c.config.Context.InspectContextTurnRefreshV1(ctx, inspect)
}

func nilInterfaceV1(value any) bool {
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

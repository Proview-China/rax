package modelinvokeradapter

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const MaxInvocationMaterialContextPairAdapterTTLV2 = 30 * time.Second

// InvocationMaterialContextPairAdapterV2 proves an exact, current Context
// Material/Frame pair and a strict Context-owned lowering into Model's public
// Instructions/Input body. It writes no Context or Model fact.
type InvocationMaterialContextPairAdapterV2 struct {
	owner       contract.OwnerRef
	source      contextports.ContextModelInputSourceCurrentReaderV1
	sourceOwner contextModelInputSourceOwnerV2
	clock       func() time.Time
	maxTTL      time.Duration
}

type contextModelInputSourceOwnerV2 interface {
	ContextOwnerRefV1() contract.OwnerRef
}

func NewInvocationMaterialContextPairAdapterV2(
	owner contract.OwnerRef,
	source contextports.ContextModelInputSourceCurrentReaderV1,
	clock func() time.Time,
	maxTTL time.Duration,
) (*InvocationMaterialContextPairAdapterV2, error) {
	if owner.Validate() != nil || nilLikeContextOwnerBindingV1(source) ||
		clock == nil || maxTTL <= 0 || maxTTL > MaxInvocationMaterialContextPairAdapterTTLV2 {
		return nil, fmt.Errorf("%w: invocation material Context pair adapter dependencies", contract.ErrInvalid)
	}
	ownerBound, ok := source.(contextModelInputSourceOwnerV2)
	if !ok || ownerBound.ContextOwnerRefV1() != owner {
		return nil, fmt.Errorf("%w: invocation material Context source Owner binding", contract.ErrConflict)
	}
	return &InvocationMaterialContextPairAdapterV2{
		owner: owner, source: source, sourceOwner: ownerBound, clock: clock, maxTTL: maxTTL,
	}, nil
}

var _ modelinvoker.InvocationMaterialContextPairExactReaderV2 = (*InvocationMaterialContextPairAdapterV2)(nil)

func (a *InvocationMaterialContextPairAdapterV2) InspectExactInvocationContextPairV2(
	ctx context.Context,
	frame modelinvoker.InvocationMaterialExactSourceRefV1,
	material modelinvoker.InvocationMaterialExactSourceRefV1,
	expectedMappedInput core.Digest,
) (modelinvoker.InvocationMaterialContextPairProjectionV2, error) {
	if ctx == nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, fmt.Errorf("%w: invocation material Context pair context", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, err
	}
	if a == nil || a.owner.Validate() != nil || nilLikeContextOwnerBindingV1(a.source) ||
		a.sourceOwner == nil || a.clock == nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, fmt.Errorf("%w: invocation material Context pair adapter", contract.ErrUnavailable)
	}
	if frame.Validate() != nil || material.Validate() != nil || expectedMappedInput.Validate() != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, fmt.Errorf("%w: invocation material Context pair request", contract.ErrInvalid)
	}
	if frame.Kind != modelinvoker.InvocationMaterialContextFrameKindV2 ||
		material.Kind != modelinvoker.InvocationMaterialContextMaterialKindV2 ||
		frame.Owner != material.Owner {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, fmt.Errorf("%w: invocation material Context pair role", contract.ErrConflict)
	}
	contextOwner := modelinvoker.ContextOwnerRef{
		ComponentID:   a.owner.ComponentID,
		BindingDigest: core.Digest(a.owner.BindingDigest),
	}
	_, neutralOwner, err := modelinvoker.MapContextOwnerRefToNeutralOwnerV1(contextOwner)
	if err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, err
	}
	if frame.Owner != neutralOwner || material.Owner != neutralOwner ||
		a.sourceOwner.ContextOwnerRefV1() != a.owner {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, fmt.Errorf("%w: invocation material Context pair Owner", contract.ErrConflict)
	}
	contextRequest, err := a.contextSourceRequestV2(frame, material)
	if err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, err
	}
	started := a.clock()
	if started.IsZero() {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, fmt.Errorf("%w: invocation material Context pair clock", contract.ErrInvalid)
	}
	contextRequest.CheckedUnixNano = started.UnixNano()
	contextRequest.NotAfterUnixNano = started.Add(a.maxTTL).UnixNano()
	contextRequest, err = contract.SealContextModelInputSourceCurrentRequestV1(contextRequest)
	if err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, err
	}

	s1, err := a.source.InspectContextModelInputSourceCurrentV1(ctx, contextRequest)
	if err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, err
	}
	observed := a.clock()
	if err := validateContextPairClockV2(started, observed, contextRequest.NotAfterUnixNano); err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, err
	}
	if err := s1.ValidateAgainst(contextRequest, observed.UnixNano()); err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, fmt.Errorf("%w: invocation material Context S1 projection", contract.ErrConflict)
	}
	instructions1, input1, mapped1, err := lowerContextModelInputBodyV2(s1.Material)
	if err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, err
	}
	if mapped1 != expectedMappedInput {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, fmt.Errorf("%w: invocation material Context S1 mapped input", contract.ErrConflict)
	}
	if err := ctx.Err(); err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, err
	}

	s2, err := a.source.InspectContextModelInputSourceCurrentV1(ctx, contextRequest)
	if err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, err
	}
	completed := a.clock()
	if err := validateContextPairClockV2(observed, completed, contextRequest.NotAfterUnixNano); err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, err
	}
	if err := s2.ValidateAgainst(contextRequest, completed.UnixNano()); err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, fmt.Errorf("%w: invocation material Context S2 projection", contract.ErrConflict)
	}
	instructions2, input2, mapped2, err := lowerContextModelInputBodyV2(s2.Material)
	if err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, err
	}
	if mapped2 != expectedMappedInput || mapped2 != mapped1 ||
		!reflect.DeepEqual(instructions1, instructions2) ||
		!reflect.DeepEqual(input1, input2) ||
		!sameContextModelInputProjectionFactsV2(s1, s2) ||
		a.sourceOwner.ContextOwnerRefV1() != a.owner {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, fmt.Errorf("%w: invocation material Context S1/S2 drift", contract.ErrConflict)
	}
	if err := ctx.Err(); err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, err
	}
	expires := minContextOwnerBindingExpiryV1(
		contextRequest.NotAfterUnixNano,
		s1.ExpiresUnixNano,
		s2.ExpiresUnixNano,
	)
	if completed.UnixNano() >= expires {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, fmt.Errorf("%w: invocation material Context pair TTL crossing", contract.ErrExpired)
	}
	projection, err := modelinvoker.SealInvocationMaterialContextPairProjectionV2(
		modelinvoker.InvocationMaterialContextPairProjectionV2{
			ContextFrame:             frame,
			ContextMaterial:          material,
			ContextMappedInputDigest: mapped2,
			CheckedUnixNano:          completed.UnixNano(),
			ExpiresUnixNano:          expires,
		},
	)
	if err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, err
	}
	if err := projection.ValidateCurrentV2(frame, material, expectedMappedInput, completed); err != nil {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{}, err
	}
	return projection, nil
}

func (a *InvocationMaterialContextPairAdapterV2) contextSourceRequestV2(
	frame modelinvoker.InvocationMaterialExactSourceRefV1,
	material modelinvoker.InvocationMaterialExactSourceRefV1,
) (contract.ContextModelInputSourceCurrentRequestV1, error) {
	materialRef := contract.ContextModelInputMaterialRefV1{
		ID: material.ID, Revision: uint64(material.Revision), Digest: contract.Digest(material.Digest),
	}
	materialSource, err := contract.ContextModelInputMaterialExactSourceV1(a.owner, materialRef)
	if err != nil {
		return contract.ContextModelInputSourceCurrentRequestV1{}, err
	}
	frameRef := contract.FactRef{
		ID: frame.ID, Revision: uint64(frame.Revision), Digest: contract.Digest(frame.Digest),
	}
	frameSource, err := contract.ContextFrameExactSourceV1(a.owner, frameRef)
	if err != nil {
		return contract.ContextModelInputSourceCurrentRequestV1{}, err
	}
	return contract.ContextModelInputSourceCurrentRequestV1{
		Owner: a.owner, Material: materialSource, Frame: frameSource,
	}, nil
}

func lowerContextModelInputBodyV2(
	material contract.ContextModelInputMaterialV1,
) ([]modelinvoker.Instruction, []modelinvoker.InputItem, core.Digest, error) {
	if material.Validate() != nil {
		return nil, nil, "", fmt.Errorf("%w: invocation material Context Material", contract.ErrInvalid)
	}
	instructions := make([]modelinvoker.Instruction, 0)
	input := make([]modelinvoker.InputItem, 0)
	for _, segment := range material.OrderedSegments {
		switch segment.Channel {
		case contract.ContextModelInputInstructionV1:
			if segment.Encoding != contract.ContextModelInputUTF8V1 ||
				(segment.Role != contract.ContextModelInputRoleSystemV1 &&
					segment.Role != contract.ContextModelInputRoleDeveloperV1) ||
				strings.TrimSpace(string(segment.Content)) == "" {
				return nil, nil, "", fmt.Errorf("%w: unsupported Context instruction lowering", contract.ErrConflict)
			}
			role, err := modelRoleFromContextV2(segment.Role)
			if err != nil {
				return nil, nil, "", err
			}
			instructions = append(instructions, modelinvoker.Instruction{Role: role, Text: string(segment.Content)})
		case contract.ContextModelInputMessageV1:
			if segment.Encoding != contract.ContextModelInputUTF8V1 ||
				(segment.Role != contract.ContextModelInputRoleUserV1 &&
					segment.Role != contract.ContextModelInputRoleAssistantV1) ||
				strings.TrimSpace(string(segment.Content)) == "" {
				return nil, nil, "", fmt.Errorf("%w: unsupported Context input message lowering", contract.ErrConflict)
			}
			role, err := modelRoleFromContextV2(segment.Role)
			if err != nil {
				return nil, nil, "", err
			}
			input = append(input, modelinvoker.MessageInput(role, string(segment.Content)))
		case contract.ContextModelInputFunctionCallV1:
			if segment.Encoding != contract.ContextModelInputCanonicalJSONV1 ||
				segment.Role != contract.ContextModelInputRoleAssistantV1 ||
				!strictCanonicalJSONObjectV2(segment.Content) {
				return nil, nil, "", fmt.Errorf("%w: unsupported Context function call lowering", contract.ErrConflict)
			}
			input = append(input, modelinvoker.FunctionCallInput(
				segment.CallID,
				segment.Name,
				json.RawMessage(segment.Content),
			))
		default:
			return nil, nil, "", fmt.Errorf("%w: unsupported Context model input Channel", contract.ErrConflict)
		}
	}
	digest, err := modelinvoker.DigestGovernedModelTurnContextBodyV2(instructions, input)
	if err != nil {
		return nil, nil, "", err
	}
	return instructions, input, digest, nil
}

func modelRoleFromContextV2(role contract.ContextModelInputRoleV1) (modelinvoker.Role, error) {
	switch role {
	case contract.ContextModelInputRoleSystemV1:
		return modelinvoker.RoleSystem, nil
	case contract.ContextModelInputRoleDeveloperV1:
		return modelinvoker.RoleDeveloper, nil
	case contract.ContextModelInputRoleUserV1:
		return modelinvoker.RoleUser, nil
	case contract.ContextModelInputRoleAssistantV1:
		return modelinvoker.RoleAssistant, nil
	default:
		return "", fmt.Errorf("%w: unsupported Context model input Role", contract.ErrConflict)
	}
}

func strictCanonicalJSONObjectV2(payload []byte) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(payload, &object) == nil && object != nil
}

func sameContextModelInputProjectionFactsV2(
	left contract.ContextModelInputSourceCurrentProjectionV1,
	right contract.ContextModelInputSourceCurrentProjectionV1,
) bool {
	return left.Owner == right.Owner &&
		left.MaterialSource == right.MaterialSource &&
		reflect.DeepEqual(left.Material, right.Material) &&
		left.FrameSource == right.FrameSource &&
		left.Frame.FrameRef == right.Frame.FrameRef &&
		left.Frame.Current == right.Frame.Current
}

func validateContextPairClockV2(previous, current time.Time, notAfterUnixNano int64) error {
	if current.IsZero() {
		return fmt.Errorf("%w: invocation material Context pair clock", contract.ErrInvalid)
	}
	if current.Before(previous) {
		return fmt.Errorf("%w: invocation material Context pair clock rollback", contract.ErrConflict)
	}
	if current.UnixNano() >= notAfterUnixNano {
		return fmt.Errorf("%w: invocation material Context pair lifetime", contract.ErrExpired)
	}
	return nil
}

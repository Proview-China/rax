package applicationadapter

import (
	"bytes"
	"context"
	"reflect"
	"strconv"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

const pendingActionCurrentKindV1 runtimeports.NamespacedNameV2 = "praxis.harness/pending-action"

type SingleCallToolActionCandidateResolveRequestV1 struct {
	ApplicationRequest applicationcontract.SingleCallToolActionRequestV2                `json:"application_request"`
	ApplicationInput   applicationcontract.SingleCallToolActionInputCurrentProjectionV2 `json:"application_input"`
	ModelProjection    modelinvoker.ToolCallCandidateObservationProjectionV1            `json:"model_projection"`
	SourceSubject      applicationcontract.SingleCallPendingActionSubjectCoordinateV2   `json:"source_subject"`
	Route              runtimeports.ControlledOperationProviderRouteCurrentProjectionV2 `json:"route"`
	ProviderCurrent    runtimeports.ProviderBindingCurrentProjectionV2                  `json:"provider_current"`
}

type SingleCallToolActionCandidateSourceProjectionV1 struct {
	Candidate  toolcontract.ActionCandidateV2    `json:"candidate"`
	Surface    toolcontract.ToolSurfaceManifest  `json:"surface"`
	Capability toolcontract.CapabilityDescriptor `json:"capability"`
	Tool       toolcontract.ToolDescriptor       `json:"tool"`
}

type SingleCallToolActionCandidateSourceV1 interface {
	ResolveSingleCallToolActionCandidateSourceV1(context.Context, SingleCallToolActionCandidateResolveRequestV1) (SingleCallToolActionCandidateSourceProjectionV1, error)
	InspectSingleCallToolActionCandidateSourceV1(context.Context, toolcontract.ObjectRef) (SingleCallToolActionCandidateSourceProjectionV1, error)
}

type SingleCallToolActionCandidateCurrentClosureV1 struct {
	Candidate              toolcontract.ActionCandidateV2 `json:"candidate"`
	PendingActionCurrent   toolcontract.OwnerCurrentRefV1 `json:"pending_action_current"`
	SurfaceCurrent         toolcontract.OwnerCurrentRefV1 `json:"surface_current"`
	CapabilityCurrent      toolcontract.OwnerCurrentRefV1 `json:"capability_current"`
	ToolCurrent            toolcontract.OwnerCurrentRefV1 `json:"tool_current"`
	InputSchemaCurrent     toolcontract.OwnerCurrentRefV1 `json:"input_schema_current"`
	SourceCandidateCurrent toolcontract.OwnerCurrentRefV1 `json:"source_candidate_current"`
	ClosureDigest          core.Digest                    `json:"closure_digest"`
}

func (c SingleCallToolActionCandidateCurrentClosureV1) DigestV1() (core.Digest, error) {
	c = cloneCandidateClosureV1(c)
	c.ClosureDigest = ""
	return core.CanonicalJSONDigest(singleCallBindingDomainV1, singleCallBindingContractVersionV1, "SingleCallToolActionCandidateCurrentClosureV1", c)
}

func (c SingleCallToolActionCandidateCurrentClosureV1) Validate() error {
	if err := c.Candidate.Validate(); err != nil {
		return err
	}
	refs := []toolcontract.OwnerCurrentRefV1{c.PendingActionCurrent, c.SurfaceCurrent, c.CapabilityCurrent, c.ToolCurrent, c.InputSchemaCurrent, c.SourceCandidateCurrent}
	created := time.Unix(0, c.Candidate.CreatedUnixNano)
	for _, ref := range refs {
		if err := ref.Validate(created); err != nil {
			return err
		}
	}
	if c.PendingActionCurrent != c.Candidate.PendingActionCurrent || c.SurfaceCurrent != c.Candidate.SurfaceCurrent || c.CapabilityCurrent != c.Candidate.CapabilityCurrent || c.ToolCurrent != c.Candidate.ToolCurrent || c.InputSchemaCurrent != c.Candidate.InputSchemaCurrent || c.SourceCandidateCurrent != c.Candidate.SourceCandidateCurrent {
		return bindingConflictV1("candidate current closure differs from sealed Candidate")
	}
	digest, err := c.DigestV1()
	if err != nil || digest != c.ClosureDigest {
		return bindingConflictV1("candidate current closure digest drifted")
	}
	return nil
}

func sealCandidateClosureV1(candidate toolcontract.ActionCandidateV2) (SingleCallToolActionCandidateCurrentClosureV1, error) {
	c := SingleCallToolActionCandidateCurrentClosureV1{
		Candidate: cloneActionCandidateV2(candidate), PendingActionCurrent: candidate.PendingActionCurrent,
		SurfaceCurrent: candidate.SurfaceCurrent, CapabilityCurrent: candidate.CapabilityCurrent,
		ToolCurrent: candidate.ToolCurrent, InputSchemaCurrent: candidate.InputSchemaCurrent,
		SourceCandidateCurrent: candidate.SourceCandidateCurrent,
	}
	digest, err := c.DigestV1()
	if err != nil {
		return SingleCallToolActionCandidateCurrentClosureV1{}, err
	}
	c.ClosureDigest = digest
	return c, c.Validate()
}

type SingleCallToolActionCandidateCurrentResolverV1 interface {
	ResolveSingleCallToolActionCandidateCurrentV1(context.Context, SingleCallToolActionCandidateResolveRequestV1) (SingleCallToolActionCandidateCurrentClosureV1, error)
	InspectSingleCallToolActionCandidateCurrentV1(context.Context, toolcontract.ObjectRef) (SingleCallToolActionCandidateCurrentClosureV1, error)
}

type CandidateCurrentResolverV1 struct {
	source SingleCallToolActionCandidateSourceV1
}

func NewCandidateCurrentResolverV1(source SingleCallToolActionCandidateSourceV1) (*CandidateCurrentResolverV1, error) {
	if isNilFlowDependencyV1(source) {
		return nil, bindingInvalidV1("candidate source is required")
	}
	return &CandidateCurrentResolverV1{source: source}, nil
}

func (r *CandidateCurrentResolverV1) ResolveSingleCallToolActionCandidateCurrentV1(ctx context.Context, request SingleCallToolActionCandidateResolveRequestV1) (SingleCallToolActionCandidateCurrentClosureV1, error) {
	if ctx == nil {
		return SingleCallToolActionCandidateCurrentClosureV1{}, bindingInvalidV1("context is required")
	}
	if r == nil || isNilFlowDependencyV1(r.source) {
		return SingleCallToolActionCandidateCurrentClosureV1{}, bindingUnavailableV1("candidate source is unavailable")
	}
	return SingleCallToolActionCandidateCurrentClosureV1{}, bindingUnavailableV1("candidate current resolution is hard blocked until PendingAction revision and Schema/Surface current authority are frozen")
}

func (r *CandidateCurrentResolverV1) InspectSingleCallToolActionCandidateCurrentV1(ctx context.Context, exact toolcontract.ObjectRef) (SingleCallToolActionCandidateCurrentClosureV1, error) {
	if ctx == nil {
		return SingleCallToolActionCandidateCurrentClosureV1{}, bindingInvalidV1("context is required")
	}
	if r == nil || isNilFlowDependencyV1(r.source) {
		return SingleCallToolActionCandidateCurrentClosureV1{}, bindingUnavailableV1("candidate source is unavailable")
	}
	return SingleCallToolActionCandidateCurrentClosureV1{}, bindingUnavailableV1("candidate current inspection is hard blocked until PendingAction revision and Schema/Surface current authority are frozen")
}

func validateCandidateResolveRequestV1(request SingleCallToolActionCandidateResolveRequestV1) error {
	now := time.Unix(0, request.ApplicationInput.CheckedUnixNano)
	if err := request.ApplicationRequest.Validate(); err != nil {
		return err
	}
	if err := request.SourceSubject.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(request.SourceSubject, request.ApplicationRequest.Action.PendingSubject) {
		return bindingConflictV1("candidate request source subject drifted")
	}
	if err := request.ApplicationInput.ValidateFor(request.ApplicationRequest, now); err != nil {
		return err
	}
	if err := request.ModelProjection.Validate(); err != nil {
		return err
	}
	if err := request.Route.ValidateCurrent(request.SourceSubject.Binding.OwnerInputs.RouteCurrent, request.SourceSubject.Binding.OwnerInputs.RouteMatrix, now); err != nil {
		return err
	}
	if err := request.ProviderCurrent.ValidateCurrent(request.Route.ProviderBinding, now); err != nil {
		return err
	}
	if request.ProviderCurrent.Ref != request.Route.ProviderBinding {
		return bindingConflictV1("candidate request Provider current drifted from Route")
	}
	return nil
}

func validateCandidateSourceProjectionV1(source SingleCallToolActionCandidateSourceProjectionV1, request *SingleCallToolActionCandidateResolveRequestV1) error {
	if err := source.Candidate.Validate(); err != nil {
		return err
	}
	if err := source.Surface.Validate(); err != nil {
		return err
	}
	if err := source.Capability.Validate(); err != nil {
		return err
	}
	if err := source.Tool.ValidateAgainst(source.Capability); err != nil {
		return err
	}
	if source.Candidate.SurfaceCurrent.ID != source.Surface.ID || source.Candidate.SurfaceCurrent.Revision != source.Surface.Revision || source.Candidate.SurfaceCurrent.Digest != source.Surface.Digest || source.Candidate.Capability.ID != string(source.Capability.ID) || source.Candidate.Capability.Revision != source.Capability.Revision || source.Candidate.Capability.Digest != source.Capability.Digest || source.Candidate.Tool.ID != string(source.Tool.ID) || source.Candidate.Tool.Revision != source.Tool.Revision || source.Candidate.Tool.Digest != source.Tool.Digest || source.Candidate.InputSchema != source.Tool.InputSchema {
		return bindingConflictV1("candidate source objects drifted")
	}
	if request == nil {
		return nil
	}
	if len(request.ModelProjection.Observation.Calls) != 1 {
		return bindingConflictV1("Model projection does not contain exactly one call")
	}
	call := request.ModelProjection.Observation.Calls[0]
	identity := request.SourceSubject.Identity
	proof := request.ApplicationInput.HarnessCurrent.IdentityCurrent.Projection
	modelRef := request.ModelProjection.Ref
	if call.Ordinal != 0 || !identity.CallOrdinalPresent || identity.CallOrdinalValue != 0 ||
		modelRef.ID != proof.ProjectionID || modelRef.Revision != proof.ProjectionRevision || modelRef.Digest != proof.ProjectionDigest ||
		modelRef.InvocationID != proof.InvocationID || modelRef.InvocationDigest != proof.InvocationDigest || modelRef.ObservationDigest != proof.ObservationDigest ||
		modelRef.Source.ResponseID != proof.SourceResponseID || modelRef.Source.SourceSequence != proof.SourceSequence ||
		proof.ProjectionContractVersion != request.ModelProjection.ContractVersion || proof.CallOrdinal != call.Ordinal ||
		proof.CallID != call.CallID || proof.CallName != call.Name || !bytes.Equal(proof.CanonicalArguments, call.CanonicalArguments) ||
		proof.CanonicalArgumentsLength != uint64(len(call.CanonicalArguments)) || proof.CanonicalArgumentsDigest != core.DigestBytes(call.CanonicalArguments) ||
		identity.ModelProjectionID != modelRef.ID || identity.ModelProjectionRevision != modelRef.Revision || identity.ModelProjectionDigest != modelRef.Digest ||
		identity.ModelInvocationID != modelRef.InvocationID || identity.ModelInvocationDigest != modelRef.InvocationDigest || identity.ModelObservationDigest != modelRef.ObservationDigest ||
		identity.ModelSourceResponseID != modelRef.Source.ResponseID || identity.ModelSourceSequence != modelRef.Source.SourceSequence ||
		identity.CallID != call.CallID || identity.CallName != call.Name || identity.CanonicalArgumentsDigest != core.DigestBytes(call.CanonicalArguments) {
		return bindingConflictV1("Model projection does not exactly bind the Application identity proof")
	}
	var entry *toolcontract.ToolSurfaceEntry
	for index := range source.Surface.Entries {
		candidate := &source.Surface.Entries[index]
		if candidate.ModelName == call.Name {
			if entry != nil {
				return bindingConflictV1("surface resolves one call name more than once")
			}
			entry = candidate
		}
	}
	if entry == nil || !entry.Allowed || entry.Visibility != toolcontract.SurfaceVisible || entry.Capability != source.Candidate.Capability || entry.Tool != source.Candidate.Tool || entry.InputSchema != source.Candidate.InputSchema {
		return bindingConflictV1("surface does not uniquely admit the requested call")
	}
	subject := request.SourceSubject
	expectedOwner := effectOwnerFromRouteV1(request.Route)
	candidate := source.Candidate
	if candidate.TenantID != request.ApplicationRequest.Action.ExecutionScope.Identity.TenantID || candidate.RunID != string(subject.Run.RunID) || candidate.SessionID != subject.SessionID || candidate.TurnID != strconv.FormatUint(uint64(subject.Turn), 10) {
		return bindingConflictV1("candidate execution coordinates drifted")
	}
	if candidate.PendingAction.ID != subject.PendingActionRef || candidate.PendingAction.RequestDigest != subject.PendingActionDigest || candidate.SourceCandidate.ID != identity.SourceCandidateID || candidate.SourceCandidate.Revision != identity.SourceCandidateRevision || candidate.SourceCandidate.Digest != identity.SourceCandidateDigest || candidate.Capability.ID != string(identity.Capability) || candidate.InputSchema != identity.PayloadSchema || candidate.Payload.Schema != identity.PayloadSchema || candidate.Payload.ContentDigest != identity.PayloadContentDigest || candidate.OperationScopeDigest != request.ApplicationRequest.Action.ExecutionScopeDigest || candidate.EffectKind != runtimeports.EffectKindV2(runtimeports.OperationScopeEvidenceActionEffectKindV3) || candidate.ExpectedOwner != expectedOwner {
		return bindingConflictV1("candidate source drifted from Application/Route coordinates")
	}
	if candidate.Payload.Validate() != nil || candidate.Payload.Ref != "" || candidate.Payload.Inline == nil || !bytes.Equal(candidate.Payload.Inline, call.CanonicalArguments) || !bytes.Equal(candidate.Payload.Inline, proof.CanonicalArguments) || core.DigestBytes(candidate.Payload.Inline) != proof.CanonicalArgumentsDigest {
		return bindingConflictV1("candidate payload is not the exact canonical Model call")
	}
	pending := candidate.PendingActionCurrent
	if pending.Kind != pendingActionCurrentKindV1 || pending.ID != subject.PendingActionRef || pending.Revision != candidate.PendingAction.Revision || pending.Digest != subject.PendingActionDigest || pending.Owner != expectedOwner || pending.CheckedUnixNano != request.ApplicationInput.HarnessCurrent.CheckedUnixNano || pending.ExpiresUnixNano != request.ApplicationInput.HarnessCurrent.ExpiresUnixNano {
		return bindingConflictV1("candidate PendingAction current does not bind the S1 Application/Route facts")
	}
	for _, current := range []toolcontract.OwnerCurrentRefV1{candidate.SurfaceCurrent, candidate.CapabilityCurrent, candidate.ToolCurrent, candidate.InputSchemaCurrent, candidate.SourceCandidateCurrent} {
		if current.Owner != expectedOwner {
			return bindingConflictV1("candidate current owner drifted from Route Provider")
		}
	}
	return nil
}

func effectOwnerFromRouteV1(route runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) runtimeports.EffectOwnerRefV2 {
	return runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: route.ProviderBinding.ComponentID, ManifestDigest: route.ProviderBinding.ManifestDigest}
}

func cloneCandidateResolveRequestV1(request SingleCallToolActionCandidateResolveRequestV1) SingleCallToolActionCandidateResolveRequestV1 {
	request.ApplicationInput = applicationcontract.CloneSingleCallToolActionInputCurrentProjectionV2(request.ApplicationInput)
	request.ModelProjection = request.ModelProjection.Clone()
	return request
}

func cloneActionCandidateV2(candidate toolcontract.ActionCandidateV2) toolcontract.ActionCandidateV2 {
	candidate.Payload.Inline = append([]byte(nil), candidate.Payload.Inline...)
	return candidate
}

func cloneCandidateClosureV1(closure SingleCallToolActionCandidateCurrentClosureV1) SingleCallToolActionCandidateCurrentClosureV1 {
	closure.Candidate = cloneActionCandidateV2(closure.Candidate)
	return closure
}

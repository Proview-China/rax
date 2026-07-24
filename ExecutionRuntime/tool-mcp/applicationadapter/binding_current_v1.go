package applicationadapter

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

const (
	singleCallBindingContractVersionV1 = "praxis.tool.single-call-action-binding-current/v1"
	singleCallBindingDomainV1          = "praxis.tool"
	singleCallBindingKindV1            = runtimeports.NamespacedNameV2("praxis.tool/single-call-action-binding-current")
	singleCallBindingIDPrefixV1        = "single-call-tool-binding:v1:"
)

type SingleCallToolActionBindingSubjectV1 struct {
	ContractVersion            string                                                         `json:"contract_version"`
	Domain                     string                                                         `json:"domain"`
	Kind                       runtimeports.NamespacedNameV2                                  `json:"kind"`
	ApplicationRequestID       string                                                         `json:"application_request_id"`
	ApplicationRequestRevision core.Revision                                                  `json:"application_request_revision"`
	ApplicationRequestDigest   core.Digest                                                    `json:"application_request_digest"`
	ActionCoordinateDigest     core.Digest                                                    `json:"action_coordinate_digest"`
	ExecutionScope             core.ExecutionScope                                            `json:"execution_scope"`
	ExecutionScopeDigest       core.Digest                                                    `json:"execution_scope_digest"`
	SourceSubject              applicationcontract.SingleCallPendingActionSubjectCoordinateV2 `json:"source_subject"`
	Digest                     core.Digest                                                    `json:"digest"`
}

func (s SingleCallToolActionBindingSubjectV1) DigestV1() (core.Digest, error) {
	s.Digest = ""
	return core.CanonicalJSONDigest(singleCallBindingDomainV1, singleCallBindingContractVersionV1, "SingleCallToolActionBindingSubjectV1", s)
}

func (s SingleCallToolActionBindingSubjectV1) Validate() error {
	if s.ContractVersion != singleCallBindingContractVersionV1 || s.Domain != singleCallBindingDomainV1 || s.Kind != singleCallBindingKindV1 || s.ApplicationRequestID == "" || s.ApplicationRequestRevision != 1 || s.ApplicationRequestDigest.Validate() != nil || s.ActionCoordinateDigest.Validate() != nil || s.ExecutionScope.Validate() != nil || s.ExecutionScopeDigest.Validate() != nil || s.SourceSubject.Validate() != nil {
		return bindingInvalidV1("binding subject is incomplete")
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(s.ExecutionScope)
	if err != nil || scopeDigest != s.ExecutionScopeDigest || !runtimeports.SameExecutionScopeV2(s.ExecutionScope, s.SourceSubject.Run.ExecutionScope) {
		return bindingConflictV1("binding subject scope drifted")
	}
	digest, err := s.DigestV1()
	if err != nil || digest != s.Digest {
		return bindingConflictV1("binding subject digest drifted")
	}
	return nil
}

type SingleCallToolActionBindingCurrentRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r SingleCallToolActionBindingCurrentRefV1) Validate() error {
	if !validBindingIDV1(r.ID) || r.Revision != 1 || r.Digest.Validate() != nil {
		return bindingInvalidV1("binding current ref is invalid")
	}
	return nil
}

type SingleCallToolActionBindingIssuanceSubjectV1 struct {
	ContractVersion          string                               `json:"contract_version"`
	BindingSubject           SingleCallToolActionBindingSubjectV1 `json:"binding_subject"`
	RequestedExpiresUnixNano int64                                `json:"requested_expires_unix_nano"`
	Digest                   core.Digest                          `json:"digest"`
}

func (s SingleCallToolActionBindingIssuanceSubjectV1) DigestV1() (core.Digest, error) {
	s.Digest = ""
	return core.CanonicalJSONDigest(singleCallBindingDomainV1, singleCallBindingContractVersionV1, "SingleCallToolActionBindingIssuanceSubjectV1", s)
}

func (s SingleCallToolActionBindingIssuanceSubjectV1) Validate() error {
	if s.ContractVersion != singleCallBindingContractVersionV1 || s.BindingSubject.Validate() != nil || s.RequestedExpiresUnixNano < 0 {
		return bindingInvalidV1("binding issuance subject is incomplete")
	}
	digest, err := s.DigestV1()
	if err != nil || digest != s.Digest {
		return bindingConflictV1("binding issuance subject digest drifted")
	}
	return nil
}

type SingleCallToolActionBindingResolveRequestV1 struct {
	ApplicationRequest       applicationcontract.SingleCallToolActionRequestV2              `json:"application_request"`
	SourceSubject            applicationcontract.SingleCallPendingActionSubjectCoordinateV2 `json:"source_subject"`
	RequestedExpiresUnixNano int64                                                          `json:"requested_expires_unix_nano"`
}

type SingleCallToolActionBindingInspectByIssuanceRequestV1 = SingleCallToolActionBindingResolveRequestV1

type SingleCallToolActionBindingInspectExactRequestV1 struct {
	Issuance SingleCallToolActionBindingInspectByIssuanceRequestV1 `json:"issuance"`
	Expected SingleCallToolActionBindingCurrentRefV1               `json:"expected"`
}

type SingleCallToolActionBindingCurrentProjectionV1 struct {
	ContractVersion          string                                                           `json:"contract_version"`
	Domain                   string                                                           `json:"domain"`
	Kind                     runtimeports.NamespacedNameV2                                    `json:"kind"`
	Ref                      SingleCallToolActionBindingCurrentRefV1                          `json:"ref"`
	Subject                  SingleCallToolActionBindingSubjectV1                             `json:"subject"`
	IssuanceSubject          SingleCallToolActionBindingIssuanceSubjectV1                     `json:"issuance_subject"`
	RequestedExpiresUnixNano int64                                                            `json:"requested_expires_unix_nano"`
	ApplicationInput         applicationcontract.SingleCallToolActionInputCurrentProjectionV2 `json:"application_input"`
	ModelProjection          modelinvoker.ToolCallCandidateObservationProjectionV1            `json:"model_projection"`
	CallOrdinal              uint32                                                           `json:"call_ordinal"`
	CallID                   string                                                           `json:"call_id"`
	CallName                 string                                                           `json:"call_name"`
	CanonicalArgumentsDigest core.Digest                                                      `json:"canonical_arguments_digest"`
	Candidate                toolcontract.ActionCandidateV2                                   `json:"candidate"`
	Association              runtimeports.GenerationBindingAssociationFactV1                  `json:"association"`
	Generation               runtimeports.GenerationCurrentProjectionV1                       `json:"generation"`
	Route                    runtimeports.ControlledOperationProviderRouteCurrentProjectionV2 `json:"route"`
	Provider                 runtimeports.ProviderBindingRefV2                                `json:"provider"`
	ProviderCurrent          runtimeports.ProviderBindingCurrentProjectionV2                  `json:"provider_current"`
	CheckedUnixNano          int64                                                            `json:"checked_unix_nano"`
	ExpiresUnixNano          int64                                                            `json:"expires_unix_nano"`
	ProjectionDigest         core.Digest                                                      `json:"projection_digest"`
}

func (p SingleCallToolActionBindingCurrentProjectionV1) DigestV1() (core.Digest, error) {
	p = cloneBindingProjectionV1(p)
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(singleCallBindingDomainV1, singleCallBindingContractVersionV1, "SingleCallToolActionBindingCurrentProjectionV1", p)
}

func (p SingleCallToolActionBindingCurrentProjectionV1) Validate() error {
	if p.ContractVersion != singleCallBindingContractVersionV1 || p.Domain != singleCallBindingDomainV1 || p.Kind != singleCallBindingKindV1 || p.Ref.Validate() != nil || p.Subject.Validate() != nil || p.IssuanceSubject.Validate() != nil || p.RequestedExpiresUnixNano < 0 || p.ApplicationInput.Digest.Validate() != nil || p.ModelProjection.Validate() != nil || p.CallOrdinal != 0 || p.CallID == "" || p.CallName == "" || p.CanonicalArgumentsDigest.Validate() != nil || p.Candidate.Validate() != nil || p.Association.Validate() != nil || p.Generation.Validate() != nil || p.Route.Validate() != nil || p.Provider.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return bindingInvalidV1("binding current projection is incomplete")
	}
	if !reflect.DeepEqual(p.Subject, p.IssuanceSubject.BindingSubject) || p.RequestedExpiresUnixNano != p.IssuanceSubject.RequestedExpiresUnixNano || p.Ref.ID != bindingIDForIssuanceV1(p.IssuanceSubject) || p.Ref.Revision != 1 || p.Provider != p.Route.ProviderBinding || p.ProviderCurrent.Ref != p.Provider {
		return bindingConflictV1("binding current projection repeated fields drifted")
	}
	if p.RequestedExpiresUnixNano > 0 && p.ExpiresUnixNano > p.RequestedExpiresUnixNano {
		return bindingConflictV1("binding current projection exceeds the issuance TTL")
	}
	if len(p.ModelProjection.Observation.Calls) != 1 || p.ModelProjection.Observation.Calls[0].Ordinal != 0 || p.ModelProjection.Observation.Calls[0].CallID != p.CallID || p.ModelProjection.Observation.Calls[0].Name != p.CallName || core.DigestBytes(p.ModelProjection.Observation.Calls[0].CanonicalArguments) != p.CanonicalArgumentsDigest || p.Candidate.Payload.ContentDigest != p.CanonicalArgumentsDigest || !bytes.Equal(p.Candidate.Payload.Inline, p.ModelProjection.Observation.Calls[0].CanonicalArguments) {
		return bindingConflictV1("binding current canonical call drifted")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.Ref.Digest || digest != p.ProjectionDigest {
		return bindingConflictV1("binding current projection digest drifted")
	}
	return nil
}

type SingleCallToolActionBindingCurrentReaderV1 interface {
	ResolveCurrentSingleCallToolActionBindingV1(context.Context, SingleCallToolActionBindingResolveRequestV1) (SingleCallToolActionBindingCurrentProjectionV1, error)
	InspectSingleCallToolActionBindingByIssuanceV1(context.Context, SingleCallToolActionBindingInspectByIssuanceRequestV1) (SingleCallToolActionBindingCurrentProjectionV1, error)
	InspectExactSingleCallToolActionBindingCurrentV1(context.Context, SingleCallToolActionBindingInspectExactRequestV1) (SingleCallToolActionBindingCurrentProjectionV1, error)
}

type BindingCurrentReaderV1 struct {
	application applicationports.SingleCallToolActionInputCurrentReaderV2
	model       modelinvoker.ToolCallCandidateObservationProjectionReaderV1
	candidate   SingleCallToolActionCandidateCurrentResolverV1
	association runtimeports.GenerationBindingAssociationCurrentReaderV1
	generation  runtimeports.GenerationCurrentReaderV1
	route       runtimeports.ControlledOperationProviderRouteCurrentReaderV2
	provider    runtimeports.ProviderBindingCurrentnessPortV2
	store       SingleCallToolActionBindingLeaseStoreV1
	clock       ClockV1
}

func NewBindingCurrentReaderV1(application applicationports.SingleCallToolActionInputCurrentReaderV2, model modelinvoker.ToolCallCandidateObservationProjectionReaderV1, candidate SingleCallToolActionCandidateCurrentResolverV1, association runtimeports.GenerationBindingAssociationCurrentReaderV1, generation runtimeports.GenerationCurrentReaderV1, route runtimeports.ControlledOperationProviderRouteCurrentReaderV2, provider runtimeports.ProviderBindingCurrentnessPortV2, store SingleCallToolActionBindingLeaseStoreV1, clock ClockV1) (*BindingCurrentReaderV1, error) {
	for name, dependency := range map[string]any{"application input reader": application, "Model projection reader": model, "candidate resolver": candidate, "association reader": association, "generation reader": generation, "route reader": route, "Provider reader": provider, "lease store": store, "clock": clock} {
		if isNilFlowDependencyV1(dependency) {
			return nil, bindingInvalidV1(name + " is required")
		}
	}
	return nil, bindingUnavailableV1("binding current reader is hard blocked until PendingAction revision and Schema/Surface current authority are frozen")
}

type bindingSnapshotV1 struct {
	input       applicationcontract.SingleCallToolActionInputCurrentProjectionV2
	model       modelinvoker.ToolCallCandidateObservationProjectionV1
	association runtimeports.GenerationBindingAssociationFactV1
	generation  runtimeports.GenerationCurrentProjectionV1
	route       runtimeports.ControlledOperationProviderRouteCurrentProjectionV2
	provider    runtimeports.ProviderBindingCurrentProjectionV2
	candidate   SingleCallToolActionCandidateCurrentClosureV1
	bounds      []int64
}

func (r *BindingCurrentReaderV1) ResolveCurrentSingleCallToolActionBindingV1(ctx context.Context, request SingleCallToolActionBindingResolveRequestV1) (SingleCallToolActionBindingCurrentProjectionV1, error) {
	if err := r.readyV1(ctx); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	subject, issuance, err := sealBindingIssuanceV1(request)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	id := bindingIDForIssuanceV1(issuance)
	existing, inspectErr := r.store.InspectSingleCallToolActionBindingLeaseByIssuanceIDV1(ctx, id)
	if inspectErr == nil {
		return r.validateStoredCurrentV1(ctx, request, existing)
	}
	if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return SingleCallToolActionBindingCurrentProjectionV1{}, inspectErr
	}
	start, err := r.freshV1(time.Time{})
	if err != nil || request.ApplicationRequest.ValidateCurrent(start) != nil {
		if err != nil {
			return SingleCallToolActionBindingCurrentProjectionV1{}, err
		}
		return SingleCallToolActionBindingCurrentProjectionV1{}, request.ApplicationRequest.ValidateCurrent(start)
	}
	s1, last, err := r.readSnapshotV1(ctx, request, nil, start)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	s2, last, err := r.readSnapshotV1(ctx, request, &s1, last)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	nowS2, err := r.freshV1(last)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	bounds := append([]int64{request.ApplicationRequest.ExpiresUnixNano, s1.input.ExpiresUnixNano, s2.input.ExpiresUnixNano, s1.candidate.Candidate.CurrentExpiresUnixNano()}, append(s1.bounds, s2.bounds...)...)
	if request.RequestedExpiresUnixNano > 0 {
		bounds = append(bounds, request.RequestedExpiresUnixNano)
	}
	expires := minPositiveV1(bounds...)
	if expires <= nowS2.UnixNano() {
		return SingleCallToolActionBindingCurrentProjectionV1{}, bindingExpiredV1("binding lease window crossed before issuance")
	}
	finalNow, err := r.freshV1(nowS2)
	if err != nil || !finalNow.Before(time.Unix(0, expires)) {
		if err != nil {
			return SingleCallToolActionBindingCurrentProjectionV1{}, err
		}
		return SingleCallToolActionBindingCurrentProjectionV1{}, bindingExpiredV1("binding lease window crossed before create")
	}
	call := s2.model.Observation.Calls[0]
	projection := SingleCallToolActionBindingCurrentProjectionV1{ContractVersion: singleCallBindingContractVersionV1, Domain: singleCallBindingDomainV1, Kind: singleCallBindingKindV1, Subject: subject, IssuanceSubject: issuance, RequestedExpiresUnixNano: request.RequestedExpiresUnixNano, ApplicationInput: s2.input, ModelProjection: s2.model, CallOrdinal: call.Ordinal, CallID: call.CallID, CallName: call.Name, CanonicalArgumentsDigest: core.DigestBytes(call.CanonicalArguments), Candidate: s1.candidate.Candidate, Association: s2.association, Generation: s2.generation, Route: s2.route, Provider: s2.route.ProviderBinding, ProviderCurrent: s2.provider, CheckedUnixNano: nowS2.UnixNano(), ExpiresUnixNano: expires}
	projection.Ref = SingleCallToolActionBindingCurrentRefV1{ID: id, Revision: 1}
	digest, err := projection.DigestV1()
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	projection.Ref.Digest, projection.ProjectionDigest = digest, digest
	if err := projection.Validate(); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	created, err := r.store.CreateSingleCallToolActionBindingLeaseOnceV1(ctx, projection)
	if err == nil {
		return created, nil
	}
	// A create reply can be lost only after the store linearized. Recover by the
	// stable issuance ID; never issue a second create or re-resolve a candidate.
	recovered, inspectErr := r.store.InspectSingleCallToolActionBindingLeaseByIssuanceIDV1(context.WithoutCancel(ctx), id)
	if inspectErr != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	return r.validateStoredCurrentV1(context.WithoutCancel(ctx), request, recovered)
}

func (r *BindingCurrentReaderV1) InspectSingleCallToolActionBindingByIssuanceV1(ctx context.Context, request SingleCallToolActionBindingInspectByIssuanceRequestV1) (SingleCallToolActionBindingCurrentProjectionV1, error) {
	if err := r.readyV1(ctx); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	_, issuance, err := sealBindingIssuanceV1(request)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	projection, err := r.store.InspectSingleCallToolActionBindingLeaseByIssuanceIDV1(ctx, bindingIDForIssuanceV1(issuance))
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	return r.validateStoredCurrentV1(ctx, request, projection)
}

func (r *BindingCurrentReaderV1) InspectExactSingleCallToolActionBindingCurrentV1(ctx context.Context, request SingleCallToolActionBindingInspectExactRequestV1) (SingleCallToolActionBindingCurrentProjectionV1, error) {
	if err := r.readyV1(ctx); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	if err := request.Expected.Validate(); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	_, issuance, err := sealBindingIssuanceV1(request.Issuance)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	if request.Expected.ID != bindingIDForIssuanceV1(issuance) {
		return SingleCallToolActionBindingCurrentProjectionV1{}, bindingConflictV1("expected binding ref names another issuance")
	}
	projection, err := r.store.InspectExactSingleCallToolActionBindingLeaseV1(ctx, request.Expected)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	return r.validateStoredCurrentV1(ctx, request.Issuance, projection)
}

func (r *BindingCurrentReaderV1) validateStoredCurrentV1(ctx context.Context, request SingleCallToolActionBindingResolveRequestV1, projection SingleCallToolActionBindingCurrentProjectionV1) (SingleCallToolActionBindingCurrentProjectionV1, error) {
	if err := projection.Validate(); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	_, issuance, err := sealBindingIssuanceV1(request)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(projection.IssuanceSubject, issuance) {
		return SingleCallToolActionBindingCurrentProjectionV1{}, bindingConflictV1("stored binding lease belongs to another issuance")
	}
	now, err := r.freshV1(time.Time{})
	if err != nil || !now.Before(time.Unix(0, projection.ExpiresUnixNano)) {
		if err != nil {
			return SingleCallToolActionBindingCurrentProjectionV1{}, err
		}
		return SingleCallToolActionBindingCurrentProjectionV1{}, bindingExpiredV1("stored binding lease expired")
	}
	s1 := bindingSnapshotV1{input: projection.ApplicationInput, model: projection.ModelProjection, association: projection.Association, generation: projection.Generation, route: projection.Route, provider: projection.ProviderCurrent, candidate: SingleCallToolActionCandidateCurrentClosureV1{Candidate: projection.Candidate, PendingActionCurrent: projection.Candidate.PendingActionCurrent, SurfaceCurrent: projection.Candidate.SurfaceCurrent, CapabilityCurrent: projection.Candidate.CapabilityCurrent, ToolCurrent: projection.Candidate.ToolCurrent, InputSchemaCurrent: projection.Candidate.InputSchemaCurrent, SourceCandidateCurrent: projection.Candidate.SourceCandidateCurrent}}
	s1.candidate.ClosureDigest, _ = s1.candidate.DigestV1()
	_, _, err = r.readSnapshotV1(ctx, request, &s1, now)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	final, err := r.freshV1(now)
	if err != nil || !final.Before(time.Unix(0, projection.ExpiresUnixNano)) {
		if err != nil {
			return SingleCallToolActionBindingCurrentProjectionV1{}, err
		}
		return SingleCallToolActionBindingCurrentProjectionV1{}, bindingExpiredV1("stored binding lease crossed TTL during Inspect")
	}
	return cloneBindingProjectionV1(projection), nil
}

func (r *BindingCurrentReaderV1) readSnapshotV1(ctx context.Context, request SingleCallToolActionBindingResolveRequestV1, first *bindingSnapshotV1, after time.Time) (bindingSnapshotV1, time.Time, error) {
	var out bindingSnapshotV1
	now, err := r.freshV1(after)
	if err != nil {
		return out, after, err
	}
	if err := request.ApplicationRequest.ValidateCurrent(now); err != nil {
		return out, now, err
	}
	input, err := r.application.InspectSingleCallToolActionInputCurrentV2(ctx, request.ApplicationRequest)
	if err != nil {
		return out, now, err
	}
	now, err = r.freshV1(now)
	if err != nil {
		return out, now, err
	}
	if err := input.ValidateFor(request.ApplicationRequest, now); err != nil {
		return out, now, err
	}
	out.input = applicationcontract.CloneSingleCallToolActionInputCurrentProjectionV2(input)
	out.bounds = append(out.bounds, input.ExpiresUnixNano)
	identity := request.SourceSubject.Identity
	modelRef := modelinvoker.ToolCallCandidateObservationRefV1{ID: identity.ModelProjectionID, Revision: identity.ModelProjectionRevision, Digest: identity.ModelProjectionDigest, InvocationID: identity.ModelInvocationID, InvocationDigest: identity.ModelInvocationDigest, ObservationDigest: identity.ModelObservationDigest, Source: modelinvoker.ToolCallCandidateObservationSourceCoordinateV1{SourceSequence: identity.ModelSourceSequence, ResponseID: identity.ModelSourceResponseID}}
	model, err := r.model.InspectExactProjectionV1(ctx, modelRef)
	if err != nil {
		return out, now, err
	}
	now, err = r.freshV1(now)
	if err != nil {
		return out, now, err
	}
	if err := validateModelForBindingV1(model, modelRef, input, request.SourceSubject); err != nil {
		return out, now, err
	}
	out.model = model.Clone()
	associationRef := request.SourceSubject.Binding.OwnerInputs.GenerationBindingAssociation
	association, err := r.association.InspectCurrentGenerationBindingAssociationV1(ctx, associationRef.ID)
	if err != nil {
		return out, now, err
	}
	now, err = r.freshV1(now)
	if err != nil {
		return out, now, err
	}
	if err := validateAssociationCurrentV1(association, associationRef, now); err != nil {
		return out, now, err
	}
	out.association = cloneAssociationV1(association)
	out.bounds = append(out.bounds, association.ExpiresUnixNano)
	generation, err := r.generation.InspectGenerationCurrentV1(ctx, association.Candidate.Generation.Generation)
	if err != nil {
		return out, now, err
	}
	now, err = r.freshV1(now)
	if err != nil {
		return out, now, err
	}
	if err := generation.ValidateCurrent(association.Candidate.Generation.Generation, now); err != nil || !reflect.DeepEqual(generation, association.Candidate.Generation) {
		if err != nil {
			return out, now, err
		}
		return out, now, bindingConflictV1("generation current drifted from Association")
	}
	out.generation = cloneGenerationV1(generation)
	out.bounds = append(out.bounds, generation.ExpiresUnixNano)
	matrix := request.SourceSubject.Binding.OwnerInputs.RouteMatrix
	route, err := r.route.InspectCurrentControlledOperationProviderRouteV2(ctx, request.SourceSubject.Binding.OwnerInputs.RouteCurrent, matrix)
	if err != nil {
		return out, now, err
	}
	now, err = r.freshV1(now)
	if err != nil {
		return out, now, err
	}
	if err := route.ValidateCurrent(request.SourceSubject.Binding.OwnerInputs.RouteCurrent, matrix, now); err != nil {
		return out, now, err
	}
	if err := validateRouteBindingV1(route, association, generation); err != nil {
		return out, now, err
	}
	out.route = cloneRouteV1(route)
	out.bounds = append(out.bounds, route.ExpiresUnixNano)
	provider, err := r.provider.InspectProviderBindingCurrentV2(ctx, route.ProviderBinding)
	if err != nil {
		return out, now, err
	}
	now, err = r.freshV1(now)
	if err != nil {
		return out, now, err
	}
	if err := provider.ValidateCurrent(route.ProviderBinding, now); err != nil {
		return out, now, err
	}
	if provider.BindingSetDigest != route.BindingSetDigest || provider.BindingSetSemanticDigest != route.BindingSetSemanticDigest {
		return out, now, bindingConflictV1("Provider current drifted from Route BindingSet")
	}
	out.provider = provider
	out.bounds = append(out.bounds, provider.ExpiresUnixNano)
	if first == nil {
		candidate, err := r.candidate.ResolveSingleCallToolActionCandidateCurrentV1(ctx, SingleCallToolActionCandidateResolveRequestV1{ApplicationRequest: request.ApplicationRequest, ApplicationInput: input, ModelProjection: model, SourceSubject: request.SourceSubject, Route: route, ProviderCurrent: provider})
		if err != nil {
			return out, now, err
		}
		now, err = r.freshV1(now)
		if err != nil {
			return out, now, err
		}
		if err := candidate.Validate(); err != nil || candidate.Candidate.CreatedUnixNano > now.UnixNano() || !now.Before(time.Unix(0, candidate.Candidate.CurrentExpiresUnixNano())) {
			if err != nil {
				return out, now, err
			}
			return out, now, bindingExpiredV1("candidate is not current at S1")
		}
		out.candidate = cloneCandidateClosureV1(candidate)
		out.bounds = append(out.bounds, candidate.Candidate.CurrentExpiresUnixNano())
		return out, now, nil
	}
	if err := compareStableRefreshV1(*first, out); err != nil {
		return out, now, err
	}
	exact := toolcontract.ObjectRef{ID: first.candidate.Candidate.ID, Revision: first.candidate.Candidate.Revision, Digest: first.candidate.Candidate.Digest}
	candidate, err := r.candidate.InspectSingleCallToolActionCandidateCurrentV1(ctx, exact)
	if err != nil {
		return out, now, err
	}
	now, err = r.freshV1(now)
	if err != nil {
		return out, now, err
	}
	if !reflect.DeepEqual(candidate, first.candidate) {
		return out, now, bindingConflictV1("candidate exact Inspect drifted from S1")
	}
	if err := validateS2PendingCurrentV1(first.candidate.Candidate.PendingActionCurrent, input, effectOwnerFromRouteV1(route), now); err != nil {
		return out, now, err
	}
	out.candidate = cloneCandidateClosureV1(candidate)
	out.bounds = append(out.bounds, candidate.Candidate.CurrentExpiresUnixNano())
	return out, now, nil
}

func (r *BindingCurrentReaderV1) readyV1(ctx context.Context) error {
	if ctx == nil {
		return bindingInvalidV1("context is required")
	}
	if r == nil || isNilFlowDependencyV1(r.application) || isNilFlowDependencyV1(r.model) || isNilFlowDependencyV1(r.candidate) || isNilFlowDependencyV1(r.association) || isNilFlowDependencyV1(r.generation) || isNilFlowDependencyV1(r.route) || isNilFlowDependencyV1(r.provider) || isNilFlowDependencyV1(r.store) || isNilFlowDependencyV1(r.clock) {
		return bindingUnavailableV1("binding current reader dependency is unavailable")
	}
	return nil
}

func (r *BindingCurrentReaderV1) freshV1(after time.Time) (time.Time, error) {
	now := r.clock.Now()
	if now.IsZero() || !after.IsZero() && now.Before(after) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "binding current clock regressed")
	}
	return now, nil
}

func sealBindingIssuanceV1(request SingleCallToolActionBindingResolveRequestV1) (SingleCallToolActionBindingSubjectV1, SingleCallToolActionBindingIssuanceSubjectV1, error) {
	if request.RequestedExpiresUnixNano < 0 || request.ApplicationRequest.Validate() != nil || request.SourceSubject.Validate() != nil || !reflect.DeepEqual(request.SourceSubject, request.ApplicationRequest.Action.PendingSubject) {
		return SingleCallToolActionBindingSubjectV1{}, SingleCallToolActionBindingIssuanceSubjectV1{}, bindingInvalidV1("binding issuance request is invalid")
	}
	subject := SingleCallToolActionBindingSubjectV1{ContractVersion: singleCallBindingContractVersionV1, Domain: singleCallBindingDomainV1, Kind: singleCallBindingKindV1, ApplicationRequestID: request.ApplicationRequest.ID, ApplicationRequestRevision: request.ApplicationRequest.Revision, ApplicationRequestDigest: request.ApplicationRequest.Digest, ActionCoordinateDigest: request.ApplicationRequest.Action.Digest, ExecutionScope: request.ApplicationRequest.Action.ExecutionScope, ExecutionScopeDigest: request.ApplicationRequest.Action.ExecutionScopeDigest, SourceSubject: request.SourceSubject}
	digest, err := subject.DigestV1()
	if err != nil {
		return subject, SingleCallToolActionBindingIssuanceSubjectV1{}, err
	}
	subject.Digest = digest
	if err := subject.Validate(); err != nil {
		return subject, SingleCallToolActionBindingIssuanceSubjectV1{}, err
	}
	issuance := SingleCallToolActionBindingIssuanceSubjectV1{ContractVersion: singleCallBindingContractVersionV1, BindingSubject: subject, RequestedExpiresUnixNano: request.RequestedExpiresUnixNano}
	digest, err = issuance.DigestV1()
	if err != nil {
		return subject, issuance, err
	}
	issuance.Digest = digest
	return subject, issuance, issuance.Validate()
}

func bindingIDForIssuanceV1(subject SingleCallToolActionBindingIssuanceSubjectV1) string {
	return singleCallBindingIDPrefixV1 + strings.TrimPrefix(string(subject.Digest), "sha256:")
}
func validBindingIDV1(id string) bool {
	return strings.HasPrefix(id, singleCallBindingIDPrefixV1) && len(strings.TrimPrefix(id, singleCallBindingIDPrefixV1)) == 64
}

func validateModelForBindingV1(p modelinvoker.ToolCallCandidateObservationProjectionV1, exact modelinvoker.ToolCallCandidateObservationRefV1, input applicationcontract.SingleCallToolActionInputCurrentProjectionV2, subject applicationcontract.SingleCallPendingActionSubjectCoordinateV2) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Ref != exact || len(p.Observation.Calls) != 1 {
		return bindingConflictV1("Model exact projection ref or call count drifted")
	}
	call := p.Observation.Calls[0]
	identity := subject.Identity
	proof := input.HarnessCurrent.IdentityCurrent.Projection
	if call.Ordinal != 0 || !identity.CallOrdinalPresent || identity.CallOrdinalValue != 0 || proof.CallOrdinal != 0 || proof.ProjectionID != p.Ref.ID || proof.ProjectionRevision != p.Ref.Revision || proof.ProjectionDigest != p.Ref.Digest || proof.InvocationID != p.Ref.InvocationID || proof.InvocationDigest != p.Ref.InvocationDigest || proof.ObservationDigest != p.Ref.ObservationDigest || proof.SourceResponseID != p.Ref.Source.ResponseID || proof.SourceSequence != p.Ref.Source.SourceSequence || proof.CallID != call.CallID || proof.CallName != call.Name || !bytes.Equal(proof.CanonicalArguments, call.CanonicalArguments) || proof.CanonicalArgumentsLength != uint64(len(call.CanonicalArguments)) || proof.CanonicalArgumentsDigest != core.DigestBytes(call.CanonicalArguments) || identity.CallID != call.CallID || identity.CallName != call.Name || identity.CanonicalArgumentsDigest != proof.CanonicalArgumentsDigest {
		return bindingConflictV1("Model projection drifted from Application identity")
	}
	return nil
}

func validateAssociationCurrentV1(f runtimeports.GenerationBindingAssociationFactV1, exact runtimeports.GenerationBindingAssociationRefV1, now time.Time) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if f.RefV1() != exact {
		return bindingConflictV1("Association exact ref drifted")
	}
	if f.State != runtimeports.GenerationBindingAssociationActiveV1 || now.IsZero() || !now.Before(time.Unix(0, f.ExpiresUnixNano)) {
		return bindingExpiredV1("Association is not current")
	}
	return nil
}

func validateRouteBindingV1(route runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, association runtimeports.GenerationBindingAssociationFactV1, generation runtimeports.GenerationCurrentProjectionV1) error {
	binding := association.Candidate.Binding
	if route.Generation != generation.Generation || route.BindingSetID != binding.BindingSetID || route.BindingSetRevision != binding.BindingSetRevision || route.BindingSetDigest != binding.BindingSetDigest || route.BindingSetSemanticDigest != binding.BindingSetSemanticDigest || route.BindingSetCurrentnessDigest != binding.CurrentnessDigest || route.ProviderBinding.BindingSetID != route.BindingSetID || route.ProviderBinding.BindingSetRevision != route.BindingSetRevision || route.ProviderBinding.Capability != runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3) {
		return bindingConflictV1("Route drifted from Association/Generation")
	}
	count := 0
	for _, manifest := range generation.ComponentManifests {
		if manifest.ComponentID == route.ProviderBinding.ComponentID && manifest.ManifestDigest == route.ProviderBinding.ManifestDigest && manifest.ArtifactDigest == route.ProviderBinding.ArtifactDigest {
			count++
		}
	}
	if count != 1 {
		return bindingConflictV1("Route Provider is not uniquely present in Generation manifests")
	}
	return nil
}

func compareStableRefreshV1(s1, s2 bindingSnapshotV1) error {
	if s2.input.CheckedUnixNano < s1.input.CheckedUnixNano || s2.model.Ref != s1.model.Ref || !reflect.DeepEqual(s2.model, s1.model) || s2.association.RefV1() != s1.association.RefV1() || s2.generation.Generation != s1.generation.Generation || s2.route.Ref != s1.route.Ref || s2.route.ProviderBinding != s1.route.ProviderBinding || s2.provider.Ref != s1.provider.Ref {
		return bindingConflictV1("S2 stable coordinates drifted from S1")
	}
	a, b := s1.input.HarnessCurrent.Subject, s2.input.HarnessCurrent.Subject
	if a.Digest != b.Digest || a.PendingActionRef != b.PendingActionRef || a.PendingActionDigest != b.PendingActionDigest || a.Binding.Digest != b.Binding.Digest || a.Identity.Digest != b.Identity.Digest {
		return bindingConflictV1("S2 PendingAction identity drifted from S1")
	}
	return nil
}

func validateS2PendingCurrentV1(s1 toolcontract.OwnerCurrentRefV1, input applicationcontract.SingleCallToolActionInputCurrentProjectionV2, owner runtimeports.EffectOwnerRefV2, now time.Time) error {
	subject := input.HarnessCurrent.Subject
	if s1.Kind != pendingActionCurrentKindV1 || s1.ID != subject.PendingActionRef || s1.Digest != subject.PendingActionDigest || s1.Owner != owner || input.HarnessCurrent.CheckedUnixNano < s1.CheckedUnixNano || now.IsZero() || !now.Before(time.Unix(0, input.HarnessCurrent.ExpiresUnixNano)) {
		return bindingConflictV1("S2 PendingAction current drifted from sealed Candidate")
	}
	return nil
}

func minPositiveV1(values ...int64) int64 {
	var min int64
	for _, v := range values {
		if v > 0 && (min == 0 || v < min) {
			min = v
		}
	}
	return min
}

func cloneAssociationV1(value runtimeports.GenerationBindingAssociationFactV1) runtimeports.GenerationBindingAssociationFactV1 {
	value.Candidate.Generation.ComponentManifests = append([]runtimeports.GenerationComponentManifestRefV1(nil), value.Candidate.Generation.ComponentManifests...)
	return value
}
func cloneGenerationV1(value runtimeports.GenerationCurrentProjectionV1) runtimeports.GenerationCurrentProjectionV1 {
	value.ComponentManifests = append([]runtimeports.GenerationComponentManifestRefV1(nil), value.ComponentManifests...)
	return value
}
func cloneRouteV1(value runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) runtimeports.ControlledOperationProviderRouteCurrentProjectionV2 {
	return value
}
func cloneBindingProjectionV1(p SingleCallToolActionBindingCurrentProjectionV1) SingleCallToolActionBindingCurrentProjectionV1 {
	p.ApplicationInput = applicationcontract.CloneSingleCallToolActionInputCurrentProjectionV2(p.ApplicationInput)
	p.ModelProjection = p.ModelProjection.Clone()
	p.Candidate = cloneActionCandidateV2(p.Candidate)
	p.Association = cloneAssociationV1(p.Association)
	p.Generation = cloneGenerationV1(p.Generation)
	p.Route = cloneRouteV1(p.Route)
	return p
}

func bindingInvalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}
func bindingConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}
func bindingUnavailableV1(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, message)
}
func bindingExpiredV1(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, message)
}

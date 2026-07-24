package kernel

import (
	"context"
	"reflect"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type CommittedPendingActionReaderV3 struct {
	sessions      harnessports.SessionFactPortV4
	candidates    harnessports.CandidateFactPortV2
	domainResults harnessports.SettledTurnDomainResultReaderV3
	models        modelinvoker.ToolCallCandidateObservationProjectionReaderV1
	settlements   runtimeports.OperationSettlementCurrentReaderV3
	associations  runtimeports.GenerationBindingAssociationGovernancePortV1
	generations   runtimeports.GenerationCurrentReaderV1
	routes        runtimeports.ControlledOperationProviderRouteCurrentReaderV2
	bindings      runtimeports.ProviderBindingCurrentnessPortV2
	contexts      runtimeports.OperationScopeEvidenceApplicabilityCurrentReaderV3
	clock         func() time.Time
}

var _ harnessports.CommittedPendingActionReaderV3 = (*CommittedPendingActionReaderV3)(nil)

func NewCommittedPendingActionReaderV3(s harnessports.SessionFactPortV4, c harnessports.CandidateFactPortV2, d harnessports.SettledTurnDomainResultReaderV3, m modelinvoker.ToolCallCandidateObservationProjectionReaderV1, settlement runtimeports.OperationSettlementCurrentReaderV3, a runtimeports.GenerationBindingAssociationGovernancePortV1, g runtimeports.GenerationCurrentReaderV1, route runtimeports.ControlledOperationProviderRouteCurrentReaderV2, binding runtimeports.ProviderBindingCurrentnessPortV2, contextReader runtimeports.OperationScopeEvidenceApplicabilityCurrentReaderV3, clock func() time.Time) (*CommittedPendingActionReaderV3, error) {
	deps := []any{s, c, d, m, settlement, a, g, route, binding, contextReader}
	for _, dep := range deps {
		if isNilDependencyV3(dep) {
			return nil, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "committed PendingAction V3 reader dependency is unavailable")
		}
	}
	if clock == nil {
		return nil, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "committed PendingAction V3 reader clock is unavailable")
	}
	return &CommittedPendingActionReaderV3{s, c, d, m, settlement, a, g, route, binding, contextReader, clock}, nil
}

func (r *CommittedPendingActionReaderV3) InspectCommittedPendingActionCurrentV3(ctx context.Context, request contract.CommittedPendingActionCurrentRequestV3) (contract.CommittedPendingActionCurrentV3, error) {
	if r == nil || isNilDependencyV3(r.sessions) || isNilDependencyV3(r.candidates) || isNilDependencyV3(r.domainResults) || isNilDependencyV3(r.models) || isNilDependencyV3(r.settlements) || isNilDependencyV3(r.associations) || isNilDependencyV3(r.generations) || isNilDependencyV3(r.routes) || isNilDependencyV3(r.bindings) || isNilDependencyV3(r.contexts) || r.clock == nil {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "committed PendingAction V3 reader is unavailable")
	}
	nowS1 := r.clock()
	if err := request.Validate(nowS1); err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	s1, err := r.inspectSessionV4(ctx, request)
	if err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	binding := s1.ApplicationBinding.Clone()
	base := binding.Base
	inputs := binding.OwnerCurrentInputs
	operationDigest, err := inputs.ModelTurnOperation.DigestV3()
	if err != nil || operationDigest != base.ModelTurnSettlementRef.Attempt.OperationDigest {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "committed PendingAction operation and Settlement attempt differ")
	}

	candidate, err := r.candidates.InspectCandidateV2(ctx, s1.Run, base.PendingAction.SourceCandidate.ID)
	if err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	if err = candidate.Validate(nowS1); err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	candidateRef, err := candidate.RefV2()
	scopeDigest, scopeErr := runtimeports.ExecutionScopeDigestV2(candidate.Run.Scope)
	if err != nil || scopeErr != nil || candidateRef != base.PendingAction.SourceCandidate || candidate.Run.RunID != s1.Run.RunID || !runtimeports.SameExecutionScopeV2(candidate.Run.Scope, s1.Run.Scope) || scopeDigest != request.Subject.Base.ExecutionScopeDigest || candidate.SessionRef != s1.ID || candidate.Turn != s1.Turn {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction Candidate drifted")
	}
	fact, err := r.domainResults.InspectExact(ctx, base.DomainResultFactRef)
	if err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	if err = fact.Validate(); err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	factRef, err := fact.RefV3()
	if err != nil || factRef != base.DomainResultFactRef || factRef.IdentityRef != base.IdentityRef {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction DomainResult or Identity drifted")
	}
	if fact.SourceKey.ExecutionScopeDigest != request.Subject.Base.ExecutionScopeDigest || fact.SourceKey.RunID != string(s1.Run.RunID) || fact.SourceKey.SessionID != s1.ID || fact.SourceKey.Turn != s1.Turn || fact.SourceKey.Candidate != candidateRef {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction source lineage drifted")
	}
	projection, err := r.models.InspectExactProjectionV1(ctx, fact.ModelProjection)
	if err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	if err = projection.Validate(); err != nil || projection.Ref != fact.ModelProjection || len(projection.Observation.Calls) != 1 {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction Model projection drifted")
	}
	call := projection.Observation.Calls[0]
	if fact.Identity.CallOrdinal.Validate() != nil || call.Ordinal != fact.Identity.CallOrdinal.Value || call.CallID != fact.Identity.CallID || call.Name != fact.Identity.CallName || core.DigestBytes(call.CanonicalArguments) != fact.Identity.CanonicalArgumentsDigest || fact.Identity.PendingActionRef != base.PendingAction.Ref || fact.Identity.PendingActionRequestDigest != base.PendingAction.RequestDigest || fact.Identity.PayloadSchema != base.PendingAction.Payload.Schema || fact.Identity.PayloadContentDigest != base.PendingAction.Payload.ContentDigest || fact.Identity.Capability != base.PendingAction.Capability || fact.Identity.SourceCandidate != base.PendingAction.SourceCandidate {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction Model call mapping drifted")
	}
	settlement, err := r.settlements.InspectOperationSettlementV3(ctx, inputs.ModelTurnOperation, base.ModelTurnSettlementRef.Attempt.EffectID)
	if err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	if err = settlement.Validate(); err != nil || !reflect.DeepEqual(settlement, base.ModelTurnSettlementRef) {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "committed PendingAction Settlement drifted")
	}

	association, err := r.associations.InspectCurrentGenerationBindingAssociationV1(ctx, inputs.GenerationBindingAssociation.ID)
	if err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	if err = association.Validate(); err != nil || association.RefV1() != inputs.GenerationBindingAssociation || association.State != runtimeports.GenerationBindingAssociationActiveV1 || !nowS1.Before(time.Unix(0, association.ExpiresUnixNano)) {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "committed PendingAction association is not exact current")
	}
	generation, err := r.generations.InspectGenerationCurrentV1(ctx, association.Candidate.Generation.Generation)
	if err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	if err = generation.ValidateCurrent(association.Candidate.Generation.Generation, nowS1); err != nil || !reflect.DeepEqual(generation, association.Candidate.Generation) {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "committed PendingAction Generation drifted")
	}
	route, err := r.routes.InspectCurrentControlledOperationProviderRouteV2(ctx, inputs.RouteCurrent, inputs.RouteMatrix)
	if err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	if err = route.ValidateCurrent(inputs.RouteCurrent, inputs.RouteMatrix, nowS1); err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	if route.Generation != association.Candidate.Generation.Generation {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "committed PendingAction Route Generation drifted")
	}
	set := association.Candidate.Binding
	if err = set.ValidateCurrent(route.BindingSetID, route.BindingSetRevision, nowS1); err != nil || set.BindingSetDigest != route.BindingSetDigest || set.BindingSetSemanticDigest != route.BindingSetSemanticDigest || set.CurrentnessDigest != route.BindingSetCurrentnessDigest {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "committed PendingAction BindingSet drifted")
	}

	roleBindings := []providerBindingRoleV3{
		{Role: providerBindingRoleSessionEndpointV3, Ref: s1.Endpoint.Binding},
		{Role: providerBindingRoleCandidateProviderV3, Ref: candidate.Provider},
		{Role: providerBindingRoleIdentitySettlementOwnerV3, Ref: fact.Identity.SettlementOwner},
		{Role: providerBindingRoleToolAdapterV3, Ref: route.ToolAdapterBinding},
		{Role: providerBindingRoleGatewayV3, Ref: route.GatewayBinding},
		{Role: providerBindingRoleProviderTransportV3, Ref: route.ProviderTransportBinding},
		{Role: providerBindingRolePreparedReaderV3, Ref: route.PreparedReaderBinding},
		{Role: providerBindingRoleBoundaryReaderV3, Ref: route.BoundaryReaderBinding},
		{Role: providerBindingRoleProviderInspectV3, Ref: route.ProviderInspectBinding},
		{Role: providerBindingRoleProviderV3, Ref: route.ProviderBinding},
	}
	roleGroups, err := normalizeProviderBindingRolesV3(roleBindings)
	if err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	for _, group := range roleGroups {
		if group.Ref.BindingSetID != set.BindingSetID || group.Ref.BindingSetRevision != set.BindingSetRevision {
			return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "committed PendingAction role Binding is outside exact set")
		}
	}
	expires := []int64{nowS1.Add(MaxCommittedPendingActionProjectionTTLV1).UnixNano(), candidate.ExpiresUnixNano, fact.Identity.NotAfterUnixNano, association.ExpiresUnixNano, generation.ExpiresUnixNano, route.ExpiresUnixNano}
	roleExpiries, err := inspectProviderBindingRoleGroupsV3(ctx, r.bindings, roleGroups, set, nowS1)
	if err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	expires = append(expires, roleExpiries...)
	if inputs.ContextApplicability.Kind != runtimeports.OperationScopeEvidenceContextParentKindV3 {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceScopeConflict, "committed PendingAction Context Kind drifted")
	}
	contextProjection, err := r.contexts.InspectOperationScopeEvidenceApplicabilityCurrentV3(ctx, inputs.ContextApplicability)
	if err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	if err = contextProjection.Validate(inputs.ContextApplicability, inputs.ModelTurnOperation.ExecutionScopeDigest, nowS1); err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	expires = append(expires, contextProjection.ExpiresUnixNano)
	s2, err := r.inspectSessionV4(ctx, request)
	if err != nil {
		return contract.CommittedPendingActionCurrentV3{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction Session changed between S1 and S2")
	}
	nowS2 := r.clock()
	if nowS2.Before(nowS1) {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "committed PendingAction clock regressed")
	}
	if request.RequestedNotAfterUnixNano > 0 {
		expires = append(expires, request.RequestedNotAfterUnixNano)
	}
	expiry := expires[0]
	for _, v := range expires[1:] {
		if v < expiry {
			expiry = v
		}
	}
	if !nowS2.Before(time.Unix(0, expiry)) {
		return contract.CommittedPendingActionCurrentV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "committed PendingAction owner currentness expired")
	}
	current := contract.CommittedPendingActionCurrentV3{Run: s2.Run, ExecutionScopeDigest: request.Subject.Base.ExecutionScopeDigest, SessionID: s2.ID, SessionRevision: s2.Revision, SessionDigest: s2.Digest, Phase: s2.Phase, Turn: s2.Turn, PendingAction: *s2.PendingAction, ApplicationBinding: *s2.ApplicationBinding, CheckedUnixNano: nowS2.UnixNano(), ExpiresUnixNano: expiry}
	return contract.SealCommittedPendingActionCurrentV3(current, request, nowS2)
}

func (r *CommittedPendingActionReaderV3) inspectSessionV4(ctx context.Context, request contract.CommittedPendingActionCurrentRequestV3) (contract.GovernedSessionV4, error) {
	s, err := r.sessions.InspectSessionV4(ctx, request.Subject.Base.Run, request.Subject.Base.SessionID)
	if err != nil {
		return contract.GovernedSessionV4{}, err
	}
	if err = s.Validate(); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	b := request.Subject.Base
	scopeDigest, scopeErr := runtimeports.ExecutionScopeDigestV2(s.Run.Scope)
	if scopeErr != nil || s.Run.RunID != b.Run.RunID || !runtimeports.SameExecutionScopeV2(s.Run.Scope, b.Run.Scope) || scopeDigest != b.ExecutionScopeDigest || s.ID != b.SessionID || s.Revision != b.SessionRevision || s.Digest != b.SessionDigest || s.Phase != contract.SessionWaitingActionV2 || s.Turn != b.Turn || s.PendingAction == nil || s.ApplicationBinding == nil || !reflect.DeepEqual(*s.PendingAction, request.Subject.ApplicationBinding.Base.PendingAction) || !reflect.DeepEqual(*s.ApplicationBinding, request.Subject.ApplicationBinding) {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction V4 Session differs")
	}
	return s.Clone(), nil
}
func isNilDependencyV3(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	}
	return false
}

type providerBindingRoleV3 struct {
	Role string                            `json:"role"`
	Ref  runtimeports.ProviderBindingRefV2 `json:"ref"`
}

const (
	providerBindingRoleSessionEndpointV3         = "session_endpoint"
	providerBindingRoleCandidateProviderV3       = "candidate_provider"
	providerBindingRoleIdentitySettlementOwnerV3 = "identity_settlement_owner"
	providerBindingRoleToolAdapterV3             = "tool_adapter"
	providerBindingRoleGatewayV3                 = "gateway"
	providerBindingRoleProviderTransportV3       = "provider_transport"
	providerBindingRolePreparedReaderV3          = "prepared_reader"
	providerBindingRoleBoundaryReaderV3          = "boundary_reader"
	providerBindingRoleProviderInspectV3         = "provider_inspect"
	providerBindingRoleProviderV3                = "provider"
)

var closedProviderBindingRolesV3 = []string{
	providerBindingRoleSessionEndpointV3,
	providerBindingRoleCandidateProviderV3,
	providerBindingRoleIdentitySettlementOwnerV3,
	providerBindingRoleToolAdapterV3,
	providerBindingRoleGatewayV3,
	providerBindingRoleProviderTransportV3,
	providerBindingRolePreparedReaderV3,
	providerBindingRoleBoundaryReaderV3,
	providerBindingRoleProviderInspectV3,
	providerBindingRoleProviderV3,
}

type providerBindingRoleGroupV3 struct {
	Ref   runtimeports.ProviderBindingRefV2
	Roles []providerBindingRoleV3
	key   string
}

func normalizeProviderBindingRolesV3(values []providerBindingRoleV3) ([]providerBindingRoleGroupV3, error) {
	if len(values) != len(closedProviderBindingRolesV3) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "committed PendingAction role Binding closure is incomplete")
	}
	expected := make(map[string]struct{}, len(closedProviderBindingRolesV3))
	for _, role := range closedProviderBindingRolesV3 {
		expected[role] = struct{}{}
	}
	groups := make(map[string]*providerBindingRoleGroupV3, len(values))
	for _, value := range values {
		if _, ok := expected[value.Role]; !ok || value.Ref.Validate() != nil {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "committed PendingAction role Binding name or ref is invalid")
		}
		delete(expected, value.Role)
		keyDigest, err := core.CanonicalJSONDigest("praxis.harness.committed-pending-action-role-binding", contract.CommittedPendingActionCurrentContractVersionV3, "ProviderBindingRefV2", value.Ref)
		if err != nil {
			return nil, err
		}
		key := string(keyDigest)
		group, ok := groups[key]
		if !ok {
			group = &providerBindingRoleGroupV3{Ref: value.Ref, key: key}
			groups[key] = group
		} else if group.Ref != value.Ref {
			return nil, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "committed PendingAction Binding refs collided canonically")
		}
		group.Roles = append(group.Roles, value)
	}
	if len(expected) != 0 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "committed PendingAction role Binding closure is incomplete")
	}
	result := make([]providerBindingRoleGroupV3, 0, len(groups))
	for _, group := range groups {
		sort.Slice(group.Roles, func(i, j int) bool { return group.Roles[i].Role < group.Roles[j].Role })
		result = append(result, *group)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].key < result[j].key })
	return result, nil
}

func inspectProviderBindingRoleGroupsV3(ctx context.Context, reader runtimeports.ProviderBindingCurrentnessPortV2, groups []providerBindingRoleGroupV3, set runtimeports.GenerationBindingSetCurrentProjectionV1, now time.Time) ([]int64, error) {
	expires := make([]int64, 0, len(groups))
	for _, group := range groups {
		current, err := reader.InspectProviderBindingCurrentV2(ctx, group.Ref)
		if err != nil {
			return nil, err
		}
		if err = current.ValidateCurrent(group.Ref, now); err != nil || current.BindingSetDigest != set.BindingSetDigest || current.BindingSetSemanticDigest != set.BindingSetSemanticDigest {
			return nil, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "committed PendingAction Provider Binding projection drifted")
		}
		for _, role := range group.Roles {
			if role.Ref != group.Ref {
				return nil, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "committed PendingAction role did not resolve to its grouped Binding")
			}
		}
		expires = append(expires, current.ExpiresUnixNano)
	}
	return expires, nil
}

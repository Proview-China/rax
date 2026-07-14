package control

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type IssueGovernedDispatchRequestV2 struct {
	EffectID               core.EffectIntentID `json:"effect_id"`
	ExpectedEffectRevision core.Revision       `json:"expected_effect_revision"`
	PermitID               string              `json:"permit_id"`
	AttemptID              string              `json:"attempt_id"`
	PermitTTL              time.Duration       `json:"permit_ttl"`
}

type BeginGovernedDispatchRequestV2 struct {
	EffectID               core.EffectIntentID `json:"effect_id"`
	ExpectedEffectRevision core.Revision       `json:"expected_effect_revision"`
	PermitID               string              `json:"permit_id"`
	ExpectedPermitRevision core.Revision       `json:"expected_permit_revision"`
}

// GovernanceDispatchGatewayV2 is the host-side final pre-dispatch gate. It
// owns no component business result and reaches no provider itself.
type GovernanceDispatchGatewayV2 struct {
	Effects        EffectFactPortV2
	Bindings       BindingFactPortV2
	Budgets        BudgetFactPortV2
	IdentityLeases IdentityLeaseFactPort
	Authority      ports.AuthorityFactReaderV2
	Policies       ports.DispatchPolicyFactReaderV2
	CurrentScopes  ports.ExecutionScopeFactReaderV2
	Review         ports.ReviewFactReaderV2
	Credentials    ports.CredentialLeaseFactReaderV2
	Clock          func() time.Time
}

func (g GovernanceDispatchGatewayV2) Issue(ctx context.Context, request IssueGovernedDispatchRequestV2) (IssueDispatchPermitResultV2, error) {
	if err := g.validate(); err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	if strings.TrimSpace(request.PermitID) == "" || strings.TrimSpace(request.AttemptID) == "" || request.ExpectedEffectRevision == 0 || request.PermitTTL <= 0 || request.PermitTTL > ports.MaxDispatchPermitTTL {
		return IssueDispatchPermitResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "permit id, single attempt, effect revision and bounded TTL are required")
	}
	now := g.Clock()
	if now.IsZero() {
		return IssueDispatchPermitResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "gateway clock returned zero")
	}
	effect, err := g.Effects.InspectEffect(ctx, request.EffectID)
	if err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	if effect.Revision != request.ExpectedEffectRevision || effect.State != EffectAccepted || !now.Before(time.Unix(0, effect.Intent.ExpiresUnixNano)) {
		return IssueDispatchPermitResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "gateway requires current accepted unexpired effect")
	}
	identityLease, err := g.IdentityLeases.InspectIdentityLease(ctx, effect.Intent.Scope.Identity.TenantID, effect.Intent.Scope.Identity.ID)
	if err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	if identityLease.State != IdentityLeaseActive || identityLease.Identity != effect.Intent.Scope.Identity || identityLease.Lineage != effect.Intent.Scope.Lineage || identityLease.AuthorityEpoch != effect.Intent.Scope.AuthorityEpoch || !now.Before(identityLease.ExpiresAt) {
		return IssueDispatchPermitResultV2{}, core.NewError(core.ErrorForbidden, core.ReasonStaleIdentityEpoch, "identity execution lease is not current for effect dispatch")
	}
	set, err := g.Bindings.InspectBindingSet(ctx, effect.Intent.Provider.BindingSetID)
	if err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	if set.State != BindingSetActive || set.Revision != effect.Intent.Provider.BindingSetRevision || !now.Before(time.Unix(0, set.ExpiresUnixNano)) {
		return IssueDispatchPermitResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "provider binding set is inactive, expired or revised")
	}
	provider, ownerRefs, err := resolveEffectBindingV2(set, effect.Intent)
	if err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	if provider.ComponentID != effect.Intent.Provider.ComponentID || provider.ManifestDigest != effect.Intent.Provider.ManifestDigest || provider.ArtifactDigest != effect.Intent.Provider.ArtifactDigest || !effectOwnerSetsEqualV2(ownerRefs, effect.Intent.Owners) {
		return IssueDispatchPermitResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "provider or effect owners drifted from current binding set")
	}
	authority, err := g.Authority.InspectDispatchAuthority(ctx, effect.Intent.Authority.Ref)
	if err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	if err := authority.ValidateCurrent(effect.Intent.Authority, effect.Intent.Scope, effect.Intent.ActionScopeDigest, now); err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	policy, err := g.Policies.InspectDispatchPolicy(ctx, effect.Intent.Policy.Ref)
	if err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	if err := policy.ValidateCurrent(effect.Intent.Policy, effect.Intent, request.PermitTTL, now); err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	review, err := g.Review.InspectDispatchReview(ctx, effect.Intent.Review.Ref)
	if err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	if err := review.ValidateCurrent(effect.Intent.Review, effect.Intent, now); err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	budget, err := g.Budgets.InspectBudgetBinding(ctx, effect.Intent.Budget.Ref)
	if err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	if err := budget.ValidateCurrent(effect.Intent.Budget, effect.Intent, now); err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	credentialFacts := make([]ports.CredentialLeaseFactV2, 0, len(effect.Intent.CredentialLeases))
	for _, expected := range effect.Intent.CredentialLeases {
		credential, err := g.Credentials.InspectCredentialLease(ctx, expected.Ref)
		if err != nil {
			return IssueDispatchPermitResultV2{}, err
		}
		if err := credential.ValidateCurrent(expected, now); err != nil {
			return IssueDispatchPermitResultV2{}, err
		}
		credentialFacts = append(credentialFacts, credential)
	}
	credentialDigest, err := ports.DigestCredentialLeaseFactsV2(credentialFacts)
	if err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	capabilityDigest, err := set.CapabilityGrantDigestV2()
	if err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	currentScope, err := g.CurrentScopes.InspectCurrentExecutionScope(ctx, effect.Intent.CurrentScope.Ref)
	if err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	if err := currentScope.ValidateCurrent(effect.Intent.CurrentScope, effect.Intent, capabilityDigest, now); err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	permitExpires := now.Add(request.PermitTTL)
	for _, limit := range []time.Time{time.Unix(0, effect.Intent.ExpiresUnixNano), identityLease.ExpiresAt, time.Unix(0, set.ExpiresUnixNano), time.Unix(0, authority.ExpiresUnixNano), time.Unix(0, policy.ExpiresUnixNano), time.Unix(0, review.ExpiresUnixNano), time.Unix(0, budget.ExpiresUnixNano), time.Unix(0, currentScope.ExpiresUnixNano)} {
		if limit.Before(permitExpires) {
			permitExpires = limit
		}
	}
	for _, credential := range credentialFacts {
		limit := time.Unix(0, credential.ExpiresUnixNano)
		if limit.Before(permitExpires) {
			permitExpires = limit
		}
	}
	if !permitExpires.After(now) {
		return IssueDispatchPermitResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitExpired, "a current governing fact expires at the permit issue boundary")
	}
	boundary := core.FenceBoundaryActivation
	if currentScope.Scope.SandboxLease != nil {
		boundary = core.FenceBoundaryInstance
	}
	fence := core.ExecutionFence{BoundaryScope: boundary, Scope: currentScope.Scope, CapabilityGrantDigest: capabilityDigest, EffectIntentID: effect.Intent.ID, EffectIntentRevision: effect.Intent.Revision, CanonicalPayloadDigest: effect.Intent.Payload.ContentDigest, ExpiresAt: permitExpires}
	fenceDigest, err := ports.DigestExecutionFenceV2(fence)
	if err != nil {
		return IssueDispatchPermitResultV2{}, err
	}
	intentDigest, _ := effect.Intent.DigestV2()
	permit := ports.DispatchPermitV2{
		ContractVersion: ports.EffectContractVersionV2, ID: request.PermitID, Revision: 1, AttemptID: request.AttemptID,
		IntentID: effect.Intent.ID, IntentRevision: effect.Intent.Revision, IntentDigest: intentDigest,
		PayloadSchema: effect.Intent.Payload.Schema, PayloadDigest: effect.Intent.Payload.ContentDigest, PayloadRevision: effect.Intent.PayloadRevision,
		Scope: effect.Intent.Scope, RunID: effect.Intent.RunID, ConflictDomain: effect.Intent.ConflictDomain,
		Provider: effect.Intent.Provider, EnforcementPoint: effect.Intent.Provider, Authority: effect.Intent.Authority, Review: effect.Intent.Review, Budget: effect.Intent.Budget, Policy: effect.Intent.Policy, CurrentScope: effect.Intent.CurrentScope,
		ReviewVerdictDigest: review.VerdictDigest, ReviewVerdictRevision: review.VerdictRevision,
		ReviewSatisfactionRef: review.SatisfactionRef, ReviewSatisfactionDigest: review.SatisfactionDigest, ReviewSatisfactionRevision: review.SatisfactionRevision,
		CapabilityGrantDigest: capabilityDigest, CredentialGrantDigest: credentialDigest, FenceDigest: fenceDigest, Idempotency: effect.Intent.Idempotency,
		IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: permitExpires.UnixNano(),
	}
	return g.Effects.IssueDispatchPermit(ctx, IssueDispatchPermitRequestV2{EffectID: effect.Intent.ID, ExpectedEffectRevision: effect.Revision, Permit: permit, Fence: fence})
}

// Begin is the final host governance gate. Application code must call this
// method, never the fact owner's raw BeginDispatch CAS primitive.
func (g GovernanceDispatchGatewayV2) Begin(ctx context.Context, request BeginGovernedDispatchRequestV2) (DispatchPermitFactV2, error) {
	if err := g.validate(); err != nil {
		return DispatchPermitFactV2{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return DispatchPermitFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "gateway clock returned zero")
	}
	effect, err := g.Effects.InspectEffect(ctx, request.EffectID)
	if err != nil {
		return DispatchPermitFactV2{}, err
	}
	permit, err := g.Effects.InspectDispatchPermit(ctx, request.PermitID)
	if err != nil {
		return DispatchPermitFactV2{}, err
	}
	if effect.Revision != request.ExpectedEffectRevision || effect.State != EffectDispatchIntent || permit.Revision != request.ExpectedPermitRevision || permit.State != DispatchPermitIssued || permit.EffectFactRevision != effect.Revision || effect.DispatchPermitID != permit.Permit.ID || !now.Before(time.Unix(0, permit.Permit.ExpiresUnixNano)) {
		return DispatchPermitFactV2{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "governed begin requires the current issued permit and dispatch intent revisions")
	}
	identityLease, err := g.IdentityLeases.InspectIdentityLease(ctx, effect.Intent.Scope.Identity.TenantID, effect.Intent.Scope.Identity.ID)
	if err != nil || identityLease.State != IdentityLeaseActive || identityLease.Identity != effect.Intent.Scope.Identity || identityLease.Lineage != effect.Intent.Scope.Lineage || identityLease.AuthorityEpoch != effect.Intent.Scope.AuthorityEpoch || !now.Before(identityLease.ExpiresAt) {
		return DispatchPermitFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonStaleIdentityEpoch, "identity lease drifted after permit issue")
	}
	set, err := g.Bindings.InspectBindingSet(ctx, effect.Intent.Provider.BindingSetID)
	if err != nil || set.State != BindingSetActive || set.Revision != effect.Intent.Provider.BindingSetRevision || !now.Before(time.Unix(0, set.ExpiresUnixNano)) {
		return DispatchPermitFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "binding set drifted after permit issue")
	}
	provider, owners, err := resolveEffectBindingV2(set, effect.Intent)
	if err != nil || provider.ComponentID != effect.Intent.Provider.ComponentID || provider.ManifestDigest != effect.Intent.Provider.ManifestDigest || provider.ArtifactDigest != effect.Intent.Provider.ArtifactDigest || !effectOwnerSetsEqualV2(owners, effect.Intent.Owners) {
		return DispatchPermitFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "provider or owner assignment drifted after permit issue")
	}
	capabilityDigest, err := set.CapabilityGrantDigestV2()
	if err != nil {
		return DispatchPermitFactV2{}, err
	}
	authority, err := g.Authority.InspectDispatchAuthority(ctx, effect.Intent.Authority.Ref)
	if err != nil || authority.ValidateCurrent(effect.Intent.Authority, effect.Intent.Scope, effect.Intent.ActionScopeDigest, now) != nil {
		return DispatchPermitFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "authority drifted after permit issue")
	}
	remainingTTL := time.Unix(0, permit.Permit.ExpiresUnixNano).Sub(now)
	policy, err := g.Policies.InspectDispatchPolicy(ctx, effect.Intent.Policy.Ref)
	if err != nil || policy.ValidateCurrent(effect.Intent.Policy, effect.Intent, remainingTTL, now) != nil {
		return DispatchPermitFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "dispatch policy drifted after permit issue")
	}
	review, err := g.Review.InspectDispatchReview(ctx, effect.Intent.Review.Ref)
	if err != nil {
		return DispatchPermitFactV2{}, err
	}
	if err := review.ValidateCurrent(effect.Intent.Review, effect.Intent, now); err != nil {
		return DispatchPermitFactV2{}, err
	}
	if review.VerdictDigest != permit.Permit.ReviewVerdictDigest || review.VerdictRevision != permit.Permit.ReviewVerdictRevision || review.SatisfactionRef != permit.Permit.ReviewSatisfactionRef || review.SatisfactionDigest != permit.Permit.ReviewSatisfactionDigest || review.SatisfactionRevision != permit.Permit.ReviewSatisfactionRevision {
		return DispatchPermitFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review verdict changed after permit issue")
	}
	budget, err := g.Budgets.InspectBudgetBinding(ctx, effect.Intent.Budget.Ref)
	if err != nil || budget.ValidateCurrent(effect.Intent.Budget, effect.Intent, now) != nil {
		return DispatchPermitFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBudgetBindingStale, "budget drifted after permit issue")
	}
	credentialFacts := make([]ports.CredentialLeaseFactV2, 0, len(effect.Intent.CredentialLeases))
	for _, expected := range effect.Intent.CredentialLeases {
		credential, err := g.Credentials.InspectCredentialLease(ctx, expected.Ref)
		if err != nil || credential.ValidateCurrent(expected, now) != nil {
			return DispatchPermitFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonCredentialLeaseMissing, "credential lease drifted after permit issue")
		}
		credentialFacts = append(credentialFacts, credential)
	}
	credentialDigest, err := ports.DigestCredentialLeaseFactsV2(credentialFacts)
	if err != nil || credentialDigest != permit.Permit.CredentialGrantDigest {
		return DispatchPermitFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCredentialLeaseMissing, "credential lease set drifted after permit issue")
	}
	currentScope, err := g.CurrentScopes.InspectCurrentExecutionScope(ctx, effect.Intent.CurrentScope.Ref)
	if err != nil || currentScope.ValidateCurrent(effect.Intent.CurrentScope, effect.Intent, capabilityDigest, now) != nil {
		return DispatchPermitFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "instance, sandbox, run or projection scope drifted after permit issue")
	}
	current := ports.DispatchCurrentFactsV2{Scope: currentScope.Scope, CapabilityGrantDigest: capabilityDigest, CredentialGrantDigest: credentialDigest, Provider: effect.Intent.Provider, EnforcementPoint: effect.Intent.Provider, Authority: effect.Intent.Authority, Review: effect.Intent.Review, ReviewVerdictDigest: review.VerdictDigest, ReviewVerdictRevision: review.VerdictRevision, ReviewSatisfactionRef: review.SatisfactionRef, ReviewSatisfactionDigest: review.SatisfactionDigest, ReviewSatisfactionRevision: review.SatisfactionRevision, Budget: effect.Intent.Budget, Policy: effect.Intent.Policy, CurrentScope: effect.Intent.CurrentScope, FenceDigest: permit.Permit.FenceDigest}
	if err := ports.ValidateDispatchAtExecutionPointV2(permit.Permit, effect.Intent, permit.Fence, current, now); err != nil {
		return DispatchPermitFactV2{}, err
	}
	return g.Effects.BeginDispatch(ctx, BeginDispatchRequestV2{EffectID: effect.Intent.ID, ExpectedEffectRevision: effect.Revision, PermitID: permit.Permit.ID, ExpectedPermitRevision: permit.Revision})
}

func (g GovernanceDispatchGatewayV2) validate() error {
	if g.Effects == nil || g.Bindings == nil || g.Budgets == nil || g.IdentityLeases == nil || g.Authority == nil || g.Policies == nil || g.CurrentScopes == nil || g.Review == nil || g.Credentials == nil || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "gateway requires effect, binding, budget, identity, authority, policy, review, credential and clock ports")
	}
	return nil
}

func resolveEffectBindingV2(set BindingSetFactV2, intent ports.EffectIntentV2) (BindingMemberV2, []ports.EffectOwnerRefV2, error) {
	members := make(map[ports.ComponentIDV2]BindingMemberV2, len(set.Members))
	var provider BindingMemberV2
	providerFound := false
	for _, member := range set.Members {
		members[member.ComponentID] = member
		if member.ComponentID == intent.Provider.ComponentID {
			provider = member
			providerFound = true
		}
	}
	if !providerFound {
		return BindingMemberV2{}, nil, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonProviderBindingStale, "provider is absent from current binding set")
	}
	capabilityFound := false
	for _, grant := range provider.Grants {
		if grant.Capability == intent.Provider.Capability {
			capabilityFound = true
			break
		}
	}
	if !capabilityFound {
		return BindingMemberV2{}, nil, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownCapability, "provider capability has no bound grant")
	}
	owners := make([]ports.EffectOwnerRefV2, 0, 3)
	for _, assignment := range provider.Owners {
		ownerMember, exists := members[assignment.OwnerComponentID]
		if !exists {
			return BindingMemberV2{}, nil, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonOwnerMissing, "effect owner is absent from current binding set")
		}
		owners = append(owners, ports.EffectOwnerRefV2{Role: assignment.Role, ComponentID: assignment.OwnerComponentID, ManifestDigest: ownerMember.ManifestDigest})
	}
	sort.Slice(owners, func(i, j int) bool { return owners[i].Role < owners[j].Role })
	return provider, owners, nil
}

func effectOwnerSetsEqualV2(left, right []ports.EffectOwnerRefV2) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type EffectRecoveryActionV2 string

const (
	RecoveryRevalidatePreDispatch EffectRecoveryActionV2 = "revalidate_pre_dispatch"
	RecoveryRejectPreDispatch     EffectRecoveryActionV2 = "reject_pre_dispatch"
	RecoveryCreateInspectEffect   EffectRecoveryActionV2 = "create_inspect_effect"
	RecoveryInspectSettlement     EffectRecoveryActionV2 = "inspect_settlement"
	RecoveryNoEffectAction        EffectRecoveryActionV2 = "no_effect_action"
)

type EffectRecoveryDecisionV2 struct {
	Action        EffectRecoveryActionV2 `json:"action"`
	AutomaticSafe bool                   `json:"automatic_safe"`
}

func PlanEffectRecoveryV2(effect EffectFactV2, permit *DispatchPermitFactV2, now time.Time) (EffectRecoveryDecisionV2, error) {
	if err := effect.Validate(); err != nil {
		return EffectRecoveryDecisionV2{}, err
	}
	if now.IsZero() || now.UnixNano() < effect.UpdatedUnixNano {
		return EffectRecoveryDecisionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "effect recovery clock regressed")
	}
	if effect.State == EffectDispatchIntent {
		if permit == nil {
			return EffectRecoveryDecisionV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonDispatchPermitInvalid, "dispatch intent is missing its permit fact")
		}
		if err := permit.Validate(); err != nil {
			return EffectRecoveryDecisionV2{}, err
		}
		if permit.State == DispatchPermitIssued {
			if !now.Before(time.Unix(0, permit.Permit.ExpiresUnixNano)) || !now.Before(time.Unix(0, effect.Intent.ExpiresUnixNano)) {
				return EffectRecoveryDecisionV2{Action: RecoveryRejectPreDispatch, AutomaticSafe: true}, nil
			}
			return EffectRecoveryDecisionV2{Action: RecoveryRevalidatePreDispatch, AutomaticSafe: false}, nil
		}
		if permit.State == DispatchPermitExpired || permit.State == DispatchPermitRevoked {
			return EffectRecoveryDecisionV2{Action: RecoveryRejectPreDispatch, AutomaticSafe: true}, nil
		}
		if permit.State == DispatchPermitBegun {
			return EffectRecoveryDecisionV2{Action: RecoveryCreateInspectEffect, AutomaticSafe: false}, nil
		}
	}
	if effect.State == EffectUnknownOutcome {
		return EffectRecoveryDecisionV2{Action: RecoveryCreateInspectEffect, AutomaticSafe: false}, nil
	}
	if effect.State == EffectDispatched {
		return EffectRecoveryDecisionV2{Action: RecoveryInspectSettlement, AutomaticSafe: false}, nil
	}
	return EffectRecoveryDecisionV2{Action: RecoveryNoEffectAction, AutomaticSafe: true}, nil
}

package ports

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	AgentActivationOwnerContractVersionV2 = "praxis.runtime.agent-activation-owner-contracts/v2"
	agentActivationOwnerCanonicalDomainV2 = "praxis.runtime.agent-activation-owner-contracts"
)

// AgentActivationOwnerFactKindV2 is closed.  A kind is part of every exact
// Ref digest domain so structurally similar Owner facts cannot be type-punned.
type AgentActivationOwnerFactKindV2 string

const (
	AgentActivationAttemptFactKindV2            AgentActivationOwnerFactKindV2 = "activation_attempt"
	AgentActivationIdentityLeaseFactKindV2      AgentActivationOwnerFactKindV2 = "identity_lease"
	AgentActivationBudgetBindingFactKindV2      AgentActivationOwnerFactKindV2 = "budget_binding"
	AgentActivationBudgetPolicyFactKindV2       AgentActivationOwnerFactKindV2 = "budget_not_required_policy"
	AgentActivationCommitFactKindV2             AgentActivationOwnerFactKindV2 = "activation_commit"
	AgentActivationSandboxReservationFactKindV2 AgentActivationOwnerFactKindV2 = "sandbox_reservation"
	AgentActivationSandboxLeaseFactKindV2       AgentActivationOwnerFactKindV2 = "sandbox_lease"
	AgentActivationSandboxActiveFactKindV2      AgentActivationOwnerFactKindV2 = "sandbox_activation"
	AgentActivationPreflightFactKindV2          AgentActivationOwnerFactKindV2 = "harness_preflight"
	AgentActivationExecutionOpenFactKindV2      AgentActivationOwnerFactKindV2 = "harness_execution_open"
	AgentActivationEndpointFactKindV2           AgentActivationOwnerFactKindV2 = "harness_endpoint"
	AgentActivationExecutionReadyFactKindV2     AgentActivationOwnerFactKindV2 = "harness_execution_ready"
	AgentActivationEffectIntentFactKindV2       AgentActivationOwnerFactKindV2 = "effect_intent"
	AgentActivationFenceFactKindV2              AgentActivationOwnerFactKindV2 = "execution_fence"
)

func (k AgentActivationOwnerFactKindV2) Validate() error {
	switch k {
	case AgentActivationAttemptFactKindV2, AgentActivationIdentityLeaseFactKindV2,
		AgentActivationBudgetBindingFactKindV2, AgentActivationBudgetPolicyFactKindV2,
		AgentActivationCommitFactKindV2, AgentActivationSandboxReservationFactKindV2,
		AgentActivationSandboxLeaseFactKindV2, AgentActivationSandboxActiveFactKindV2,
		AgentActivationPreflightFactKindV2, AgentActivationExecutionOpenFactKindV2,
		AgentActivationEndpointFactKindV2, AgentActivationExecutionReadyFactKindV2,
		AgentActivationEffectIntentFactKindV2, AgentActivationFenceFactKindV2:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation Owner fact kind is invalid")
	}
}

// AgentActivationExactFactRefV2 is the common wire shape. Public APIs below
// use nominal named forms, never this shape directly. Runtime owns the carrier
// type only; Owner remains the semantic Fact owner.
type AgentActivationExactFactRefV2 struct {
	Kind            AgentActivationOwnerFactKindV2 `json:"kind"`
	Owner           core.OwnerRef                  `json:"owner"`
	ContractVersion string                         `json:"contract_version"`
	ID              string                         `json:"id"`
	Revision        core.Revision                  `json:"revision"`
	Digest          core.Digest                    `json:"digest"`
	ExpiresUnixNano int64                          `json:"expires_unix_nano"`
}

func (r AgentActivationExactFactRefV2) validateFor(kind AgentActivationOwnerFactKindV2) error {
	if r.Kind != kind || r.Owner.Validate() != nil || strings.TrimSpace(r.ContractVersion) == "" || r.ContractVersion != strings.TrimSpace(r.ContractVersion) || len(r.ContractVersion) > 160 || validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || r.Digest.Validate() != nil || r.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation exact Owner fact Ref is incomplete or type-punned")
	}
	return nil
}

// Nominal exact refs deliberately remain distinct even though their JSON
// shapes match. Every Validate also checks its closed kind discriminator.
type AgentActivationAttemptRefV2 AgentActivationExactFactRefV2
type AgentActivationIdentityLeaseRefV2 AgentActivationExactFactRefV2
type AgentActivationBudgetBindingRefV2 AgentActivationExactFactRefV2
type AgentActivationBudgetNotRequiredPolicyRefV2 AgentActivationExactFactRefV2
type AgentActivationCommitRefV2 AgentActivationExactFactRefV2
type AgentActivationSandboxReservationRefV2 AgentActivationExactFactRefV2
type AgentActivationSandboxLeaseFactRefV2 AgentActivationExactFactRefV2
type AgentActivationSandboxActiveRefV2 AgentActivationExactFactRefV2
type AgentActivationPreflightRefV2 AgentActivationExactFactRefV2
type AgentActivationExecutionOpenRefV2 AgentActivationExactFactRefV2
type AgentActivationEndpointRefV2 AgentActivationExactFactRefV2
type AgentActivationExecutionReadyRefV2 AgentActivationExactFactRefV2
type AgentActivationEffectIntentRefV2 AgentActivationExactFactRefV2
type AgentActivationFenceRefV2 AgentActivationExactFactRefV2

func (r AgentActivationAttemptRefV2) Validate() error {
	return AgentActivationExactFactRefV2(r).validateFor(AgentActivationAttemptFactKindV2)
}
func (r AgentActivationIdentityLeaseRefV2) Validate() error {
	return AgentActivationExactFactRefV2(r).validateFor(AgentActivationIdentityLeaseFactKindV2)
}
func (r AgentActivationBudgetBindingRefV2) Validate() error {
	return AgentActivationExactFactRefV2(r).validateFor(AgentActivationBudgetBindingFactKindV2)
}
func (r AgentActivationBudgetNotRequiredPolicyRefV2) Validate() error {
	return AgentActivationExactFactRefV2(r).validateFor(AgentActivationBudgetPolicyFactKindV2)
}
func (r AgentActivationCommitRefV2) Validate() error {
	return AgentActivationExactFactRefV2(r).validateFor(AgentActivationCommitFactKindV2)
}
func (r AgentActivationSandboxReservationRefV2) Validate() error {
	return AgentActivationExactFactRefV2(r).validateFor(AgentActivationSandboxReservationFactKindV2)
}
func (r AgentActivationSandboxLeaseFactRefV2) Validate() error {
	return AgentActivationExactFactRefV2(r).validateFor(AgentActivationSandboxLeaseFactKindV2)
}
func (r AgentActivationSandboxActiveRefV2) Validate() error {
	return AgentActivationExactFactRefV2(r).validateFor(AgentActivationSandboxActiveFactKindV2)
}
func (r AgentActivationPreflightRefV2) Validate() error {
	return AgentActivationExactFactRefV2(r).validateFor(AgentActivationPreflightFactKindV2)
}
func (r AgentActivationExecutionOpenRefV2) Validate() error {
	return AgentActivationExactFactRefV2(r).validateFor(AgentActivationExecutionOpenFactKindV2)
}
func (r AgentActivationEndpointRefV2) Validate() error {
	return AgentActivationExactFactRefV2(r).validateFor(AgentActivationEndpointFactKindV2)
}
func (r AgentActivationExecutionReadyRefV2) Validate() error {
	return AgentActivationExactFactRefV2(r).validateFor(AgentActivationExecutionReadyFactKindV2)
}
func (r AgentActivationEffectIntentRefV2) Validate() error {
	return AgentActivationExactFactRefV2(r).validateFor(AgentActivationEffectIntentFactKindV2)
}
func (r AgentActivationFenceRefV2) Validate() error {
	return AgentActivationExactFactRefV2(r).validateFor(AgentActivationFenceFactKindV2)
}

// AgentActivationProposedScopeV2 cannot carry a Sandbox lease. It freezes the
// identity, lineage, instance and authority that Commit must preserve.
type AgentActivationProposedScopeV2 struct {
	Identity       core.AgentIdentityRef `json:"identity"`
	Lineage        core.LineageRef       `json:"lineage"`
	Instance       core.InstanceRef      `json:"instance"`
	AuthorityEpoch core.Epoch            `json:"authority_epoch"`
}

func (s AgentActivationProposedScopeV2) Validate() error {
	if s.Identity.Validate() != nil || s.Lineage.Validate() != nil || s.Instance.Validate() != nil || s.AuthorityEpoch == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "proposed activation scope is incomplete")
	}
	return nil
}

func (s AgentActivationProposedScopeV2) DigestV2() (core.Digest, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(agentActivationOwnerCanonicalDomainV2, AgentActivationOwnerContractVersionV2, "AgentActivationProposedScopeV2", s)
}

func ValidateAgentActivationCommittedScopeV2(proposed AgentActivationProposedScopeV2, lease core.SandboxLeaseRef, committed core.ExecutionScope) error {
	if proposed.Validate() != nil || lease.Validate() != nil || committed.Validate() != nil || committed.SandboxLease == nil || committed.Identity != proposed.Identity || committed.Lineage != proposed.Lineage || committed.Instance != proposed.Instance || committed.AuthorityEpoch != proposed.AuthorityEpoch || *committed.SandboxLease != lease {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "committed activation scope changed the proposed scope or exact Sandbox lease")
	}
	return nil
}

type AgentActivationDispatchModeV2 string

const (
	AgentActivationDispatchForbiddenV2 AgentActivationDispatchModeV2 = "forbidden"
	AgentActivationDispatchRequiredV2  AgentActivationDispatchModeV2 = "required"
)

// AgentActivationDispatchAuthorizationV2 is a closed tagged union. Preflight,
// admission snapshot, identity/budget and commit use forbidden; Allocate,
// Activate and Open use required with exact Intent and Fence refs.
type AgentActivationDispatchAuthorizationV2 struct {
	Mode          AgentActivationDispatchModeV2     `json:"mode"`
	Intent        *AgentActivationEffectIntentRefV2 `json:"intent,omitempty"`
	Fence         *AgentActivationFenceRefV2        `json:"fence,omitempty"`
	BindingDigest core.Digest                       `json:"binding_digest"`
}

func (a AgentActivationDispatchAuthorizationV2) DigestV2() (core.Digest, error) {
	copy := a
	copy.BindingDigest = ""
	return core.CanonicalJSONDigest(agentActivationOwnerCanonicalDomainV2, AgentActivationOwnerContractVersionV2, "AgentActivationDispatchAuthorizationV2/"+string(a.Mode), copy)
}

func SealAgentActivationDispatchAuthorizationV2(a AgentActivationDispatchAuthorizationV2) (AgentActivationDispatchAuthorizationV2, error) {
	provided := a.BindingDigest
	a.BindingDigest = ""
	digest, err := a.DigestV2()
	if err != nil {
		return AgentActivationDispatchAuthorizationV2{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationDispatchAuthorizationV2{}, activationConflictV2("dispatch authorization supplied another digest")
	}
	a.BindingDigest = digest
	return a, a.Validate()
}

func (a AgentActivationDispatchAuthorizationV2) Validate() error {
	switch a.Mode {
	case AgentActivationDispatchForbiddenV2:
		if a.Intent != nil || a.Fence != nil {
			return activationConflictV2("forbidden dispatch carried Intent or Fence")
		}
	case AgentActivationDispatchRequiredV2:
		if a.Intent == nil || a.Fence == nil || a.Intent.Validate() != nil || a.Fence.Validate() != nil {
			return activationInvalidV2("required dispatch lacks exact Intent or Fence")
		}
		if AgentActivationExactFactRefV2(*a.Intent).ExpiresUnixNano != AgentActivationExactFactRefV2(*a.Fence).ExpiresUnixNano {
			return activationConflictV2("Intent and Fence current windows drifted")
		}
	default:
		return activationInvalidV2("dispatch authorization mode is invalid")
	}
	digest, err := a.DigestV2()
	if err != nil || digest != a.BindingDigest {
		return activationConflictV2("dispatch authorization digest drifted")
	}
	return nil
}

type AgentActivationStableInvocationV2 struct {
	ActivationID              string                                 `json:"activation_id"`
	AttemptID                 string                                 `json:"attempt_id"`
	RequestDigest             core.Digest                            `json:"request_digest"`
	Dispatch                  AgentActivationDispatchAuthorizationV2 `json:"dispatch"`
	RequestedNotAfterUnixNano int64                                  `json:"requested_not_after_unix_nano"`
	InvocationDigest          core.Digest                            `json:"invocation_digest"`
}

func (i AgentActivationStableInvocationV2) DigestV2() (core.Digest, error) {
	copy := i
	copy.InvocationDigest = ""
	return core.CanonicalJSONDigest(agentActivationOwnerCanonicalDomainV2, AgentActivationOwnerContractVersionV2, "AgentActivationStableInvocationV2", copy)
}

func SealAgentActivationStableInvocationV2(i AgentActivationStableInvocationV2) (AgentActivationStableInvocationV2, error) {
	provided := i.InvocationDigest
	i.InvocationDigest = ""
	digest, err := i.DigestV2()
	if err != nil {
		return AgentActivationStableInvocationV2{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationStableInvocationV2{}, activationConflictV2("activation invocation supplied another digest")
	}
	i.InvocationDigest = digest
	return i, i.Validate()
}

func (i AgentActivationStableInvocationV2) Validate() error {
	if validateEvidenceIDV2(i.ActivationID) != nil || validateEvidenceIDV2(i.AttemptID) != nil || i.RequestDigest.Validate() != nil || i.Dispatch.Validate() != nil || i.RequestedNotAfterUnixNano <= 0 {
		return activationInvalidV2("stable activation invocation is incomplete")
	}
	if i.Dispatch.Mode == AgentActivationDispatchRequiredV2 {
		intent, fence := AgentActivationExactFactRefV2(*i.Dispatch.Intent), AgentActivationExactFactRefV2(*i.Dispatch.Fence)
		if i.RequestedNotAfterUnixNano > intent.ExpiresUnixNano || i.RequestedNotAfterUnixNano > fence.ExpiresUnixNano {
			return activationExpiredV2("activation invocation exceeds Intent or Fence current")
		}
	}
	digest, err := i.DigestV2()
	if err != nil || digest != i.InvocationDigest {
		return activationConflictV2("activation invocation digest drifted")
	}
	return nil
}

type AgentActivationBudgetModeV2 string

const (
	AgentActivationBudgetRequiredV2    AgentActivationBudgetModeV2 = "required"
	AgentActivationBudgetNotRequiredV2 AgentActivationBudgetModeV2 = "operation_not_required"
)

// AgentActivationBudgetDecisionV2 is closed: either a Budget fact or an
// explicit Policy fact, never an empty/default observation.
type AgentActivationBudgetDecisionV2 struct {
	Mode              AgentActivationBudgetModeV2                  `json:"mode"`
	Budget            *AgentActivationBudgetBindingRefV2           `json:"budget,omitempty"`
	NotRequiredPolicy *AgentActivationBudgetNotRequiredPolicyRefV2 `json:"not_required_policy,omitempty"`
	DecisionDigest    core.Digest                                  `json:"decision_digest"`
}

func (d AgentActivationBudgetDecisionV2) DigestV2() (core.Digest, error) {
	copy := d
	copy.DecisionDigest = ""
	return core.CanonicalJSONDigest(agentActivationOwnerCanonicalDomainV2, AgentActivationOwnerContractVersionV2, "AgentActivationBudgetDecisionV2/"+string(d.Mode), copy)
}
func SealAgentActivationBudgetDecisionV2(d AgentActivationBudgetDecisionV2) (AgentActivationBudgetDecisionV2, error) {
	provided := d.DecisionDigest
	d.DecisionDigest = ""
	digest, err := d.DigestV2()
	if err != nil {
		return AgentActivationBudgetDecisionV2{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationBudgetDecisionV2{}, activationConflictV2("budget decision supplied another digest")
	}
	d.DecisionDigest = digest
	return d, d.Validate()
}
func (d AgentActivationBudgetDecisionV2) Validate() error {
	switch d.Mode {
	case AgentActivationBudgetRequiredV2:
		if d.Budget == nil || d.Budget.Validate() != nil || d.NotRequiredPolicy != nil {
			return activationInvalidV2("required Budget decision shape is invalid")
		}
	case AgentActivationBudgetNotRequiredV2:
		if d.NotRequiredPolicy == nil || d.NotRequiredPolicy.Validate() != nil || d.Budget != nil {
			return activationInvalidV2("not-required Budget decision lacks exact Policy")
		}
	default:
		return activationInvalidV2("Budget decision mode is invalid")
	}
	digest, err := d.DigestV2()
	if err != nil || digest != d.DecisionDigest {
		return activationConflictV2("Budget decision digest drifted")
	}
	return nil
}

type AgentActivationAttemptStageV2 string

const (
	AgentActivationAttemptProposedV2              AgentActivationAttemptStageV2 = "proposed"
	AgentActivationAttemptSnapshotFrozenV2        AgentActivationAttemptStageV2 = "snapshot_frozen"
	AgentActivationAttemptIdentityBudgetCurrentV2 AgentActivationAttemptStageV2 = "identity_budget_current"
	AgentActivationAttemptSandboxReservedV2       AgentActivationAttemptStageV2 = "sandbox_reserved"
	AgentActivationAttemptCommittedV2             AgentActivationAttemptStageV2 = "committed"
)

func (s AgentActivationAttemptStageV2) Validate() error {
	switch s {
	case AgentActivationAttemptProposedV2, AgentActivationAttemptSnapshotFrozenV2, AgentActivationAttemptIdentityBudgetCurrentV2, AgentActivationAttemptSandboxReservedV2, AgentActivationAttemptCommittedV2:
		return nil
	}
	return activationInvalidV2("activation attempt stage is invalid")
}

type AgentActivationAttemptCurrentProjectionV2 struct {
	ContractVersion     string                         `json:"contract_version"`
	Ref                 AgentActivationAttemptRefV2    `json:"ref"`
	ActivationID        string                         `json:"activation_id"`
	AttemptID           string                         `json:"attempt_id"`
	ProposedScope       AgentActivationProposedScopeV2 `json:"proposed_scope"`
	ProposedScopeDigest core.Digest                    `json:"proposed_scope_digest"`
	Stage               AgentActivationAttemptStageV2  `json:"stage"`
	Current             bool                           `json:"current"`
	CheckedUnixNano     int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano     int64                          `json:"expires_unix_nano"`
	ProjectionDigest    core.Digest                    `json:"projection_digest"`
}

func (p AgentActivationAttemptCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest(agentActivationOwnerCanonicalDomainV2, AgentActivationOwnerContractVersionV2, "AgentActivationAttemptCurrentProjectionV2", copy)
}
func SealAgentActivationAttemptCurrentProjectionV2(p AgentActivationAttemptCurrentProjectionV2) (AgentActivationAttemptCurrentProjectionV2, error) {
	p.ContractVersion = AgentActivationOwnerContractVersionV2
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	digest, err := p.DigestV2()
	if err != nil {
		return AgentActivationAttemptCurrentProjectionV2{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationAttemptCurrentProjectionV2{}, activationConflictV2("activation Attempt projection supplied another digest")
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}
func (p AgentActivationAttemptCurrentProjectionV2) Validate() error {
	if p.ContractVersion != AgentActivationOwnerContractVersionV2 || p.Ref.Validate() != nil || validateEvidenceIDV2(p.ActivationID) != nil || validateEvidenceIDV2(p.AttemptID) != nil || p.ProposedScope.Validate() != nil || p.ProposedScopeDigest.Validate() != nil || p.Stage.Validate() != nil || !p.Current || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ExpiresUnixNano != AgentActivationExactFactRefV2(p.Ref).ExpiresUnixNano {
		return activationInvalidV2("activation Attempt current projection is incomplete")
	}
	d, err := p.ProposedScope.DigestV2()
	if err != nil || d != p.ProposedScopeDigest {
		return activationConflictV2("activation Attempt proposed scope drifted")
	}
	d, err = p.DigestV2()
	if err != nil || d != p.ProjectionDigest {
		return activationConflictV2("activation Attempt current projection drifted")
	}
	return nil
}
func (p AgentActivationAttemptCurrentProjectionV2) ValidateCurrent(expected AgentActivationAttemptRefV2, now time.Time) error {
	if p.Validate() != nil || expected.Validate() != nil || p.Ref != expected {
		return activationConflictV2("activation Attempt exact Ref drifted")
	}
	return validateActivationWindowV2(p.CheckedUnixNano, p.ExpiresUnixNano, now)
}

type AgentActivationIdentityLeaseStateV2 string

const (
	AgentActivationIdentityLeaseReservedV2 AgentActivationIdentityLeaseStateV2 = "reserved"
	AgentActivationIdentityLeaseActiveV2   AgentActivationIdentityLeaseStateV2 = "active"
)

type AgentActivationIdentityLeaseCurrentProjectionV2 struct {
	ContractVersion     string                              `json:"contract_version"`
	Ref                 AgentActivationIdentityLeaseRefV2   `json:"ref"`
	ActivationID        string                              `json:"activation_id"`
	Attempt             AgentActivationAttemptRefV2         `json:"activation_attempt"`
	ProposedScopeDigest core.Digest                         `json:"proposed_scope_digest"`
	Identity            core.AgentIdentityRef               `json:"identity"`
	Lineage             core.LineageRef                     `json:"lineage"`
	Instance            core.InstanceRef                    `json:"instance"`
	AuthorityEpoch      core.Epoch                          `json:"authority_epoch"`
	State               AgentActivationIdentityLeaseStateV2 `json:"state"`
	Current             bool                                `json:"current"`
	CheckedUnixNano     int64                               `json:"checked_unix_nano"`
	ExpiresUnixNano     int64                               `json:"expires_unix_nano"`
	ProjectionDigest    core.Digest                         `json:"projection_digest"`
}

func (p AgentActivationIdentityLeaseCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest(agentActivationOwnerCanonicalDomainV2, AgentActivationOwnerContractVersionV2, "AgentActivationIdentityLeaseCurrentProjectionV2", copy)
}
func SealAgentActivationIdentityLeaseCurrentProjectionV2(p AgentActivationIdentityLeaseCurrentProjectionV2) (AgentActivationIdentityLeaseCurrentProjectionV2, error) {
	p.ContractVersion = AgentActivationOwnerContractVersionV2
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	d, e := p.DigestV2()
	if e != nil {
		return AgentActivationIdentityLeaseCurrentProjectionV2{}, e
	}
	if provided != "" && provided != d {
		return AgentActivationIdentityLeaseCurrentProjectionV2{}, activationConflictV2("identity lease projection supplied another digest")
	}
	p.ProjectionDigest = d
	return p, p.Validate()
}
func (p AgentActivationIdentityLeaseCurrentProjectionV2) Validate() error {
	validState := p.State == AgentActivationIdentityLeaseReservedV2 || p.State == AgentActivationIdentityLeaseActiveV2
	if p.ContractVersion != AgentActivationOwnerContractVersionV2 || p.Ref.Validate() != nil || validateEvidenceIDV2(p.ActivationID) != nil || p.Attempt.Validate() != nil || p.ProposedScopeDigest.Validate() != nil || p.Identity.Validate() != nil || p.Lineage.Validate() != nil || p.Instance.Validate() != nil || p.AuthorityEpoch == 0 || !validState || !p.Current || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ExpiresUnixNano != AgentActivationExactFactRefV2(p.Ref).ExpiresUnixNano {
		return activationInvalidV2("identity lease current projection is incomplete")
	}
	d, e := p.DigestV2()
	if e != nil || d != p.ProjectionDigest {
		return activationConflictV2("identity lease current projection drifted")
	}
	return nil
}
func (p AgentActivationIdentityLeaseCurrentProjectionV2) ValidateCurrent(expected AgentActivationIdentityLeaseRefV2, now time.Time) error {
	if p.Validate() != nil || expected.Validate() != nil || p.Ref != expected {
		return activationConflictV2("identity lease exact Ref drifted")
	}
	return validateActivationWindowV2(p.CheckedUnixNano, p.ExpiresUnixNano, now)
}

type AgentActivationBudgetCurrentProjectionV2 struct {
	ContractVersion  string                          `json:"contract_version"`
	ActivationID     string                          `json:"activation_id"`
	Attempt          AgentActivationAttemptRefV2     `json:"activation_attempt"`
	Decision         AgentActivationBudgetDecisionV2 `json:"decision"`
	Current          bool                            `json:"current"`
	CheckedUnixNano  int64                           `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                           `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                     `json:"projection_digest"`
}

func (p AgentActivationBudgetCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest(agentActivationOwnerCanonicalDomainV2, AgentActivationOwnerContractVersionV2, "AgentActivationBudgetCurrentProjectionV2", copy)
}
func SealAgentActivationBudgetCurrentProjectionV2(p AgentActivationBudgetCurrentProjectionV2) (AgentActivationBudgetCurrentProjectionV2, error) {
	p.ContractVersion = AgentActivationOwnerContractVersionV2
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	d, e := p.DigestV2()
	if e != nil {
		return AgentActivationBudgetCurrentProjectionV2{}, e
	}
	if provided != "" && provided != d {
		return AgentActivationBudgetCurrentProjectionV2{}, activationConflictV2("Budget projection supplied another digest")
	}
	p.ProjectionDigest = d
	return p, p.Validate()
}
func (p AgentActivationBudgetCurrentProjectionV2) Validate() error {
	if p.ContractVersion != AgentActivationOwnerContractVersionV2 || validateEvidenceIDV2(p.ActivationID) != nil || p.Attempt.Validate() != nil || p.Decision.Validate() != nil || !p.Current || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return activationInvalidV2("Budget current projection is incomplete")
	}
	var limit int64
	if p.Decision.Budget != nil {
		limit = AgentActivationExactFactRefV2(*p.Decision.Budget).ExpiresUnixNano
	} else {
		limit = AgentActivationExactFactRefV2(*p.Decision.NotRequiredPolicy).ExpiresUnixNano
	}
	if p.ExpiresUnixNano > limit {
		return activationExpiredV2("Budget projection exceeds decision current")
	}
	d, e := p.DigestV2()
	if e != nil || d != p.ProjectionDigest {
		return activationConflictV2("Budget current projection drifted")
	}
	return nil
}
func (p AgentActivationBudgetCurrentProjectionV2) ValidateCurrent(now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return validateActivationWindowV2(p.CheckedUnixNano, p.ExpiresUnixNano, now)
}

func validateActivationWindowV2(checkedUnixNano, expiresUnixNano int64, now time.Time) error {
	if now.IsZero() || now.UnixNano() < checkedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "agent activation current clock regressed")
	}
	if now.UnixNano() >= expiresUnixNano {
		return activationExpiredV2("agent activation current projection expired")
	}
	return nil
}

func activationInvalidV2(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}

func activationConflictV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}

func activationExpiredV2(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, message)
}

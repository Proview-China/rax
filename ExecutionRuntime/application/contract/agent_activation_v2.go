package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	AgentActivationContractVersionV2 = "praxis.application.agent-activation/v2"
	agentActivationCanonicalDomainV2 = "praxis.application.agent-activation"
)

// AgentActivationStepV2 is closed. Its order is part of the canonical contract.
type AgentActivationStepV2 string

const (
	AgentActivationPreflightV2       AgentActivationStepV2 = "preflight"
	AgentActivationSnapshotV2        AgentActivationStepV2 = "snapshot"
	AgentActivationIdentityBudgetV2  AgentActivationStepV2 = "identity_budget"
	AgentActivationSandboxAllocateV2 AgentActivationStepV2 = "sandbox_allocate"
	AgentActivationCommitV2          AgentActivationStepV2 = "activation_commit"
	AgentActivationSandboxActivateV2 AgentActivationStepV2 = "sandbox_activate"
	AgentActivationExecutionOpenV2   AgentActivationStepV2 = "execution_open"
	AgentActivationReadyInspectV2    AgentActivationStepV2 = "ready_inspect"
)

var agentActivationStepOrderV2 = [...]AgentActivationStepV2{
	AgentActivationPreflightV2,
	AgentActivationSnapshotV2,
	AgentActivationIdentityBudgetV2,
	AgentActivationSandboxAllocateV2,
	AgentActivationCommitV2,
	AgentActivationSandboxActivateV2,
	AgentActivationExecutionOpenV2,
	AgentActivationReadyInspectV2,
}

func AgentActivationStepOrderV2() []AgentActivationStepV2 {
	result := make([]AgentActivationStepV2, len(agentActivationStepOrderV2))
	copy(result, agentActivationStepOrderV2[:])
	return result
}

func (s AgentActivationStepV2) Validate() error {
	for _, value := range agentActivationStepOrderV2 {
		if s == value {
			return nil
		}
	}
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Agent activation V2 step is invalid")
}

// ProposedActivationScopeV2 is intentionally not core.ExecutionScope. A lease
// cannot exist before ActivationCommit; the proposed Instance is nevertheless
// fixed so Commit cannot swap the instance identity.
type ProposedActivationScopeV2 struct {
	Identity       core.AgentIdentityRef `json:"identity"`
	Lineage        core.LineageRef       `json:"lineage"`
	Instance       core.InstanceRef      `json:"instance"`
	AuthorityEpoch core.Epoch            `json:"authority_epoch"`
}

func (s ProposedActivationScopeV2) Validate() error {
	if err := s.Identity.Validate(); err != nil {
		return err
	}
	if err := s.Lineage.Validate(); err != nil {
		return err
	}
	if err := s.Instance.Validate(); err != nil {
		return err
	}
	if s.AuthorityEpoch == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Proposed activation authority epoch is required")
	}
	return nil
}

func (s ProposedActivationScopeV2) DigestV2() (core.Digest, error) {
	return core.CanonicalJSONDigest(agentActivationCanonicalDomainV2, AgentActivationContractVersionV2, "ProposedActivationScopeV2", s)
}

type AgentActivationStartRequestV2 struct {
	ContractVersion           string                         `json:"contract_version"`
	ActivationID              string                         `json:"activation_id"`
	IdempotencyKey            string                         `json:"idempotency_key"`
	ProposedScope             ProposedActivationScopeV2      `json:"proposed_scope"`
	DefinitionCurrent         runtimeports.OwnerCurrentRefV1 `json:"definition_current"`
	PlanCurrent               runtimeports.OwnerCurrentRefV1 `json:"plan_current"`
	AssemblyCurrent           runtimeports.OwnerCurrentRefV1 `json:"assembly_current"`
	BindingSetCurrent         runtimeports.OwnerCurrentRefV1 `json:"binding_set_current"`
	AuthorityCurrent          runtimeports.OwnerCurrentRefV1 `json:"authority_current"`
	PolicyCurrent             runtimeports.OwnerCurrentRefV1 `json:"policy_current"`
	RequirementDigest         core.Digest                    `json:"requirement_digest"`
	ProbeBudget               uint32                         `json:"probe_budget"`
	RequestedNotAfterUnixNano int64                          `json:"requested_not_after_unix_nano"`
	RequestDigest             core.Digest                    `json:"request_digest"`
}

func (r AgentActivationStartRequestV2) inputRefsV2() []runtimeports.OwnerCurrentRefV1 {
	return []runtimeports.OwnerCurrentRefV1{r.DefinitionCurrent, r.PlanCurrent, r.AssemblyCurrent, r.BindingSetCurrent, r.AuthorityCurrent, r.PolicyCurrent}
}

func (r AgentActivationStartRequestV2) Validate() error {
	if r.ContractVersion != AgentActivationContractVersionV2 || !validAgentLifecycleIDV1(r.ActivationID) || !validAgentLifecycleIDV1(r.IdempotencyKey) || r.ProbeBudget == 0 || r.RequestedNotAfterUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation V2 Start request is incomplete")
	}
	if err := r.ProposedScope.Validate(); err != nil {
		return err
	}
	if err := r.RequirementDigest.Validate(); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, ref := range r.inputRefsV2() {
		if err := ref.Validate(); err != nil {
			return err
		}
		key := ref.Owner.Domain + "\x00" + string(ref.Owner.ID) + "\x00" + ref.ContractVersion + "\x00" + ref.ID
		if _, ok := seen[key]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Agent activation V2 Start input roles alias")
		}
		seen[key] = struct{}{}
		if r.RequestedNotAfterUnixNano > ref.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent activation V2 Start window exceeds an input current")
		}
	}
	digest, err := AgentActivationStartRequestDigestV2(r)
	if err != nil || digest != r.RequestDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation V2 Start digest drifted")
	}
	return nil
}

func AgentActivationStartRequestDigestV2(r AgentActivationStartRequestV2) (core.Digest, error) {
	r.ContractVersion = AgentActivationContractVersionV2
	r.RequestDigest = ""
	return core.CanonicalJSONDigest(agentActivationCanonicalDomainV2, AgentActivationContractVersionV2, "AgentActivationStartRequestV2", r)
}

func SealAgentActivationStartRequestV2(r AgentActivationStartRequestV2) (AgentActivationStartRequestV2, error) {
	if r.ContractVersion != "" && r.ContractVersion != AgentActivationContractVersionV2 {
		return AgentActivationStartRequestV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation V2 Start version drifted")
	}
	r.ContractVersion = AgentActivationContractVersionV2
	provided := r.RequestDigest
	r.RequestDigest = ""
	digest, err := AgentActivationStartRequestDigestV2(r)
	if err != nil {
		return AgentActivationStartRequestV2{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationStartRequestV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent activation V2 Start supplied another digest")
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

type AgentActivationCoordinationRefV2 struct {
	ActivationID       string        `json:"activation_id"`
	Revision           core.Revision `json:"revision"`
	Digest             core.Digest   `json:"digest"`
	StartRequestDigest core.Digest   `json:"start_request_digest"`
}

func (r AgentActivationCoordinationRefV2) Validate() error {
	if !validAgentLifecycleIDV1(r.ActivationID) || r.Revision == 0 || r.Digest.Validate() != nil || r.StartRequestDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation coordination Ref is incomplete")
	}
	return nil
}

type AgentActivationStepResultRefV2 struct {
	ActivationID  string                `json:"activation_id"`
	Step          AgentActivationStepV2 `json:"step"`
	AttemptID     string                `json:"attempt_id"`
	RequestDigest core.Digest           `json:"request_digest"`
	ResultDigest  core.Digest           `json:"result_digest"`
}

func (r AgentActivationStepResultRefV2) Validate() error {
	if !validAgentLifecycleIDV1(r.ActivationID) || r.Step.Validate() != nil || !validAgentLifecycleIDV1(r.AttemptID) || r.RequestDigest.Validate() != nil || r.ResultDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation step result Ref is incomplete")
	}
	return nil
}

// AgentActivationDispatchBindingV2 carries exact refs issued by their Owners.
// Application validates and forwards them but cannot create either fact.
type AgentActivationDispatchBindingV2 struct {
	IntentCurrent runtimeports.OwnerCurrentRefV1 `json:"intent_current"`
	FenceCurrent  runtimeports.OwnerCurrentRefV1 `json:"fence_current"`
}

func (b AgentActivationDispatchBindingV2) Validate() error {
	if err := b.IntentCurrent.Validate(); err != nil {
		return err
	}
	if err := b.FenceCurrent.Validate(); err != nil {
		return err
	}
	if b.IntentCurrent.ID == b.FenceCurrent.ID && b.IntentCurrent.Owner == b.FenceCurrent.Owner {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Intent and Fence currents alias")
	}
	return nil
}

// ValidateFor additionally binds the dispatch permission window to the exact
// current Intent and Fence coordinates. A valid coordinate outside either
// current window is not dispatchable.
func (b AgentActivationDispatchBindingV2) ValidateFor(requestedNotAfterUnixNano int64) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if requestedNotAfterUnixNano <= 0 || requestedNotAfterUnixNano > b.IntentCurrent.ExpiresUnixNano || requestedNotAfterUnixNano > b.FenceCurrent.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent activation dispatch window exceeds Intent or Fence current")
	}
	return nil
}

type AgentActivationBudgetDispositionV2 string

const (
	AgentActivationBudgetCurrentV2     AgentActivationBudgetDispositionV2 = "budget_current"
	AgentActivationBudgetNotRequiredV2 AgentActivationBudgetDispositionV2 = "operation_not_required"
)

// AgentActivationBudgetProofV2 is a closed union. Application preserves the
// exact Runtime-owned current coordinate but never upgrades it into a Budget
// or Policy fact. Nominal Runtime Budget/Policy refs remain a separate public
// Port delta; this union prevents the old two-fields-at-once ambiguity now.
type AgentActivationBudgetProofV2 struct {
	Disposition       AgentActivationBudgetDispositionV2 `json:"disposition"`
	BudgetCurrent     *runtimeports.OwnerCurrentRefV1    `json:"budget_current,omitempty"`
	NotRequiredPolicy *runtimeports.OwnerCurrentRefV1    `json:"not_required_policy,omitempty"`
}

func (p AgentActivationBudgetProofV2) Validate() error {
	budget, notRequired := p.BudgetCurrent != nil, p.NotRequiredPolicy != nil
	switch p.Disposition {
	case AgentActivationBudgetCurrentV2:
		if !budget || notRequired || p.BudgetCurrent.Validate() != nil {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Budget-current proof branch drifted")
		}
	case AgentActivationBudgetNotRequiredV2:
		if budget || !notRequired || p.NotRequiredPolicy.Validate() != nil {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Budget-not-required proof branch drifted")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Agent activation budget disposition is invalid")
	}
	return nil
}

func (p AgentActivationBudgetProofV2) CurrentV2() runtimeports.OwnerCurrentRefV1 {
	if p.BudgetCurrent != nil {
		return *p.BudgetCurrent
	}
	if p.NotRequiredPolicy != nil {
		return *p.NotRequiredPolicy
	}
	return runtimeports.OwnerCurrentRefV1{}
}

type AgentActivationStepInputsV2 struct {
	ProposedScope ProposedActivationScopeV2         `json:"proposed_scope"`
	Predecessor   *AgentActivationStepResultRefV2   `json:"predecessor,omitempty"`
	Authority     *runtimeports.OwnerCurrentRefV1   `json:"authority_current,omitempty"`
	Policy        *runtimeports.OwnerCurrentRefV1   `json:"policy_current,omitempty"`
	Dispatch      *AgentActivationDispatchBindingV2 `json:"dispatch,omitempty"`
	InputDigest   core.Digest                       `json:"input_digest"`
}

func (i AgentActivationStepInputsV2) ValidateFor(step AgentActivationStepV2) error {
	if err := i.ProposedScope.Validate(); err != nil {
		return err
	}
	if step == AgentActivationPreflightV2 {
		if i.Predecessor != nil || i.Authority != nil || i.Policy != nil || i.Dispatch != nil {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Preflight inputs contain a forbidden predecessor or dispatch")
		}
	} else if i.Predecessor == nil || i.Predecessor.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation step predecessor is required")
	}
	if step == AgentActivationIdentityBudgetV2 {
		if i.Authority == nil || i.Policy == nil || i.Authority.Validate() != nil || i.Policy.Validate() != nil || i.Dispatch != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Identity/Budget requires Authority and Policy without dispatch")
		}
	} else if i.Authority != nil || i.Policy != nil {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Authority or Policy appeared on another activation step")
	}
	requiresDispatch := step == AgentActivationSandboxAllocateV2 || step == AgentActivationSandboxActivateV2 || step == AgentActivationExecutionOpenV2
	if requiresDispatch != (i.Dispatch != nil) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "Agent activation dispatch requirement drifted")
	}
	if i.Dispatch != nil {
		if err := i.Dispatch.Validate(); err != nil {
			return err
		}
	}
	digest, err := AgentActivationStepInputsDigestV2(i, step)
	if err != nil || digest != i.InputDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation step inputs digest drifted")
	}
	return nil
}

func AgentActivationStepInputsDigestV2(i AgentActivationStepInputsV2, step AgentActivationStepV2) (core.Digest, error) {
	i.InputDigest = ""
	return core.CanonicalJSONDigest(agentActivationCanonicalDomainV2, AgentActivationContractVersionV2, "AgentActivationStepInputsV2/"+string(step), i)
}

func SealAgentActivationStepInputsV2(i AgentActivationStepInputsV2, step AgentActivationStepV2) (AgentActivationStepInputsV2, error) {
	provided := i.InputDigest
	i.InputDigest = ""
	digest, err := AgentActivationStepInputsDigestV2(i, step)
	if err != nil {
		return AgentActivationStepInputsV2{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationStepInputsV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent activation step inputs supplied another digest")
	}
	i.InputDigest = digest
	return i, i.ValidateFor(step)
}

type AgentActivationStepRequestV2 struct {
	ContractVersion           string                           `json:"contract_version"`
	Coordination              AgentActivationCoordinationRefV2 `json:"coordination_ref"`
	InvocationSequence        uint32                           `json:"invocation_sequence"`
	InvocationEventDigest     core.Digest                      `json:"invocation_event_digest"`
	Step                      AgentActivationStepV2            `json:"step"`
	AttemptID                 string                           `json:"attempt_id"`
	Inputs                    AgentActivationStepInputsV2      `json:"inputs"`
	RequestedNotAfterUnixNano int64                            `json:"requested_not_after_unix_nano"`
	RequestDigest             core.Digest                      `json:"request_digest"`
}

func (r AgentActivationStepRequestV2) Validate() error {
	if r.ContractVersion != AgentActivationContractVersionV2 || r.Coordination.Validate() != nil || r.InvocationSequence == 0 || r.InvocationEventDigest.Validate() != nil || r.Step.Validate() != nil || !validAgentLifecycleIDV1(r.AttemptID) || r.RequestedNotAfterUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation step V2 request is incomplete")
	}
	if err := r.Inputs.ValidateFor(r.Step); err != nil {
		return err
	}
	if r.Inputs.Dispatch != nil {
		if err := r.Inputs.Dispatch.ValidateFor(r.RequestedNotAfterUnixNano); err != nil {
			return err
		}
	}
	attempt, err := DeriveAgentActivationStepAttemptIDV2(r.Coordination.ActivationID, r.Coordination.StartRequestDigest, r.Step)
	if err != nil || attempt != r.AttemptID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation step V2 attempt drifted")
	}
	digest, err := AgentActivationStepRequestDigestV2(r)
	if err != nil || digest != r.RequestDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation step V2 request digest drifted")
	}
	return nil
}

func DeriveAgentActivationStepAttemptIDV2(activationID string, startDigest core.Digest, step AgentActivationStepV2) (string, error) {
	if !validAgentLifecycleIDV1(activationID) || startDigest.Validate() != nil || step.Validate() != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation step V2 attempt inputs are incomplete")
	}
	digest, err := core.CanonicalJSONDigest(agentActivationCanonicalDomainV2, AgentActivationContractVersionV2, "AgentActivationStepAttemptIDV2", struct {
		ActivationID string                `json:"activation_id"`
		StartDigest  core.Digest           `json:"start_digest"`
		Step         AgentActivationStepV2 `json:"step"`
	}{activationID, startDigest, step})
	if err != nil {
		return "", err
	}
	return "activation-v2-step-" + strings.TrimPrefix(string(digest), "sha256:")[:32], nil
}

func AgentActivationStepRequestDigestV2(r AgentActivationStepRequestV2) (core.Digest, error) {
	r.ContractVersion = AgentActivationContractVersionV2
	r.RequestDigest = ""
	return core.CanonicalJSONDigest(agentActivationCanonicalDomainV2, AgentActivationContractVersionV2, "AgentActivationStepRequestV2", r)
}

func SealAgentActivationStepRequestV2(r AgentActivationStepRequestV2) (AgentActivationStepRequestV2, error) {
	r.ContractVersion = AgentActivationContractVersionV2
	attempt, err := DeriveAgentActivationStepAttemptIDV2(r.Coordination.ActivationID, r.Coordination.StartRequestDigest, r.Step)
	if err != nil {
		return AgentActivationStepRequestV2{}, err
	}
	if r.AttemptID != "" && r.AttemptID != attempt {
		return AgentActivationStepRequestV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation step V2 supplied another attempt")
	}
	r.AttemptID = attempt
	provided := r.RequestDigest
	r.RequestDigest = ""
	digest, err := AgentActivationStepRequestDigestV2(r)
	if err != nil {
		return AgentActivationStepRequestV2{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationStepRequestV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent activation step V2 supplied another request digest")
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

// AgentActivationStepProofV2 is a closed neutral proof envelope. Every field
// is an exact public reference; it never embeds an Owner fact or write handle.
type AgentActivationStepProofV2 struct {
	PrimaryCurrent   runtimeports.OwnerCurrentRefV1  `json:"primary_current"`
	SecondaryCurrent *runtimeports.OwnerCurrentRefV1 `json:"secondary_current,omitempty"`
	Lease            *core.SandboxLeaseRef           `json:"sandbox_lease,omitempty"`
	CommittedScope   *core.ExecutionScope            `json:"committed_scope,omitempty"`
	EndpointCurrent  *runtimeports.OwnerCurrentRefV1 `json:"endpoint_current,omitempty"`
	Budget           *AgentActivationBudgetProofV2   `json:"budget,omitempty"`
	CheckedUnixNano  int64                           `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                           `json:"expires_unix_nano"`
	ProofDigest      core.Digest                     `json:"proof_digest"`
}

func (p AgentActivationStepProofV2) ValidateFor(step AgentActivationStepV2, proposed ProposedActivationScopeV2) error {
	if err := step.Validate(); err != nil {
		return err
	}
	if err := proposed.Validate(); err != nil {
		return err
	}
	if err := p.PrimaryCurrent.Validate(); err != nil {
		return err
	}
	if p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ExpiresUnixNano > p.PrimaryCurrent.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent activation proof current window is invalid")
	}
	for _, ref := range []*runtimeports.OwnerCurrentRefV1{p.SecondaryCurrent, p.EndpointCurrent} {
		if ref != nil {
			if err := ref.Validate(); err != nil {
				return err
			}
			if p.ExpiresUnixNano > ref.ExpiresUnixNano {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent activation proof exceeds a secondary current")
			}
		}
	}
	if p.Budget != nil {
		if err := p.Budget.Validate(); err != nil {
			return err
		}
		budgetCurrent := p.Budget.CurrentV2()
		if p.ExpiresUnixNano > budgetCurrent.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent activation proof exceeds Budget/Policy current")
		}
	}
	if p.Lease != nil {
		if err := p.Lease.Validate(); err != nil {
			return err
		}
	}
	if p.CommittedScope != nil {
		if err := p.CommittedScope.Validate(); err != nil {
			return err
		}
		if p.CommittedScope.Identity != proposed.Identity || p.CommittedScope.Lineage != proposed.Lineage || p.CommittedScope.Instance != proposed.Instance || p.CommittedScope.AuthorityEpoch != proposed.AuthorityEpoch || p.CommittedScope.SandboxLease == nil || p.Lease == nil || *p.CommittedScope.SandboxLease != *p.Lease {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Committed scope changed proposed identity, lineage, instance, authority or lease")
		}
	}
	if err := p.validateShapeV2(step); err != nil {
		return err
	}
	digest, err := AgentActivationStepProofDigestV2(p, step)
	if err != nil || digest != p.ProofDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation proof digest drifted")
	}
	return nil
}

func (p AgentActivationStepProofV2) validateShapeV2(step AgentActivationStepV2) error {
	secondary, lease, committed, endpoint, budget := p.SecondaryCurrent != nil, p.Lease != nil, p.CommittedScope != nil, p.EndpointCurrent != nil, p.Budget != nil
	valid := false
	switch step {
	case AgentActivationPreflightV2, AgentActivationSnapshotV2:
		valid = !secondary && !lease && !committed && !endpoint && !budget
	case AgentActivationIdentityBudgetV2:
		valid = secondary && !lease && !committed && !endpoint && budget
	case AgentActivationSandboxAllocateV2:
		valid = secondary && lease && !committed && !endpoint && !budget
	case AgentActivationCommitV2:
		valid = secondary && lease && committed && !endpoint && !budget
	case AgentActivationSandboxActivateV2:
		valid = secondary && lease && !committed && !endpoint && !budget
	case AgentActivationExecutionOpenV2:
		valid = secondary && lease && !committed && endpoint && !budget
	case AgentActivationReadyInspectV2:
		valid = secondary && lease && !committed && endpoint && !budget
	}
	if !valid {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation step proof shape is not valid for its role")
	}
	return nil
}

func AgentActivationStepProofDigestV2(p AgentActivationStepProofV2, step AgentActivationStepV2) (core.Digest, error) {
	p.ProofDigest = ""
	return core.CanonicalJSONDigest(agentActivationCanonicalDomainV2, AgentActivationContractVersionV2, "AgentActivationStepProofV2/"+string(step), p)
}

func SealAgentActivationStepProofV2(p AgentActivationStepProofV2, step AgentActivationStepV2, proposed ProposedActivationScopeV2) (AgentActivationStepProofV2, error) {
	provided := p.ProofDigest
	p.ProofDigest = ""
	digest, err := AgentActivationStepProofDigestV2(p, step)
	if err != nil {
		return AgentActivationStepProofV2{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationStepProofV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent activation proof supplied another digest")
	}
	p.ProofDigest = digest
	return p, p.ValidateFor(step, proposed)
}

type AgentActivationStepResultV2 struct {
	ContractVersion string                         `json:"contract_version"`
	Ref             AgentActivationStepResultRefV2 `json:"ref"`
	Proof           AgentActivationStepProofV2     `json:"proof"`
	ResultDigest    core.Digest                    `json:"result_digest"`
}

func (r AgentActivationStepResultV2) ValidateFor(request AgentActivationStepRequestV2, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if r.ContractVersion != AgentActivationContractVersionV2 || r.Ref.ActivationID != request.Coordination.ActivationID || r.Ref.Step != request.Step || r.Ref.AttemptID != request.AttemptID || r.Ref.RequestDigest != request.RequestDigest || r.Ref.ResultDigest != r.ResultDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation step result does not bind its request")
	}
	if err := r.Ref.Validate(); err != nil {
		return err
	}
	if err := r.Proof.ValidateFor(request.Step, request.Inputs.ProposedScope); err != nil {
		return err
	}
	if r.Proof.ExpiresUnixNano > request.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent activation step result exceeds request window")
	}
	if now.IsZero() || now.UnixNano() < r.Proof.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Agent activation step result clock regressed")
	}
	if !now.Before(time.Unix(0, r.Proof.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent activation step result expired")
	}
	digest, err := AgentActivationStepResultDigestV2(r)
	if err != nil || digest != r.ResultDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation step result digest drifted")
	}
	return nil
}

func AgentActivationStepResultDigestV2(r AgentActivationStepResultV2) (core.Digest, error) {
	r.ContractVersion = AgentActivationContractVersionV2
	r.Ref.ResultDigest = ""
	r.ResultDigest = ""
	return core.CanonicalJSONDigest(agentActivationCanonicalDomainV2, AgentActivationContractVersionV2, "AgentActivationStepResultV2", r)
}

func SealAgentActivationStepResultV2(r AgentActivationStepResultV2, request AgentActivationStepRequestV2) (AgentActivationStepResultV2, error) {
	r.ContractVersion = AgentActivationContractVersionV2
	r.Ref.ActivationID = request.Coordination.ActivationID
	r.Ref.Step = request.Step
	r.Ref.AttemptID = request.AttemptID
	r.Ref.RequestDigest = request.RequestDigest
	providedRef, providedResult := r.Ref.ResultDigest, r.ResultDigest
	r.Ref.ResultDigest, r.ResultDigest = "", ""
	digest, err := AgentActivationStepResultDigestV2(r)
	if err != nil {
		return AgentActivationStepResultV2{}, err
	}
	if (providedRef != "" && providedRef != digest) || (providedResult != "" && providedResult != digest) {
		return AgentActivationStepResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent activation step result supplied another digest")
	}
	r.Ref.ResultDigest, r.ResultDigest = digest, digest
	return r, nil
}

type AgentActivationStepEventStateV2 string

const (
	AgentActivationStepIntentRecordedV2     AgentActivationStepEventStateV2 = "intent_recorded"
	AgentActivationStepInvocationRecordedV2 AgentActivationStepEventStateV2 = "invocation_recorded"
	AgentActivationStepOutcomeUnknownV2     AgentActivationStepEventStateV2 = "outcome_unknown"
	AgentActivationStepResultRecordedV2     AgentActivationStepEventStateV2 = "result_recorded"
)

type AgentActivationStepEventV2 struct {
	Sequence         uint32                          `json:"sequence"`
	Step             AgentActivationStepV2           `json:"step"`
	State            AgentActivationStepEventStateV2 `json:"state"`
	AttemptID        string                          `json:"attempt_id"`
	RequestDigest    core.Digest                     `json:"request_digest"`
	Request          *AgentActivationStepRequestV2   `json:"request,omitempty"`
	Result           *AgentActivationStepResultV2    `json:"result,omitempty"`
	RecordedUnixNano int64                           `json:"recorded_unix_nano"`
	Digest           core.Digest                     `json:"digest"`
}

func (e AgentActivationStepEventV2) Validate() error {
	if e.Sequence == 0 || e.Step.Validate() != nil || !validAgentLifecycleIDV1(e.AttemptID) || e.RequestDigest.Validate() != nil || e.RecordedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation step event V2 is incomplete")
	}
	switch e.State {
	case AgentActivationStepIntentRecordedV2:
		if e.Request != nil || e.Result != nil {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Activation intent event carried invocation or result content")
		}
	case AgentActivationStepInvocationRecordedV2, AgentActivationStepOutcomeUnknownV2:
		if e.Request == nil || e.Request.Validate() != nil || e.Request.Step != e.Step || e.Request.AttemptID != e.AttemptID || e.Request.RequestDigest != e.RequestDigest || e.Result != nil {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Activation invocation event request drifted")
		}
	case AgentActivationStepResultRecordedV2:
		if e.Request == nil || e.Request.Validate() != nil || e.Request.Step != e.Step || e.Request.AttemptID != e.AttemptID || e.Request.RequestDigest != e.RequestDigest || e.Result == nil || e.Result.Ref.Step != e.Step || e.Result.Ref.AttemptID != e.AttemptID || e.Result.Ref.RequestDigest != e.RequestDigest {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Activation result event drifted")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Agent activation V2 event state is invalid")
	}
	digest, err := AgentActivationStepEventDigestV2(e)
	if err != nil || digest != e.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation V2 event digest drifted")
	}
	return nil
}

func AgentActivationStepEventDigestV2(e AgentActivationStepEventV2) (core.Digest, error) {
	e.Digest = ""
	return core.CanonicalJSONDigest(agentActivationCanonicalDomainV2, AgentActivationContractVersionV2, "AgentActivationStepEventV2", e)
}

func SealAgentActivationStepEventV2(e AgentActivationStepEventV2) (AgentActivationStepEventV2, error) {
	provided := e.Digest
	e.Digest = ""
	digest, err := AgentActivationStepEventDigestV2(e)
	if err != nil {
		return AgentActivationStepEventV2{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationStepEventV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent activation V2 event supplied another digest")
	}
	e.Digest = digest
	return e, e.Validate()
}

type AgentActivationResultV2 struct {
	ContractVersion       string                         `json:"contract_version"`
	ActivationID          string                         `json:"activation_id"`
	StartRequestDigest    core.Digest                    `json:"start_request_digest"`
	ExecutionScope        core.ExecutionScope            `json:"execution_scope"`
	ExecutionScopeDigest  core.Digest                    `json:"execution_scope_digest"`
	ActivationCurrent     runtimeports.OwnerCurrentRefV1 `json:"activation_current"`
	SandboxActiveCurrent  runtimeports.OwnerCurrentRefV1 `json:"sandbox_active_current"`
	ExecutionOpenCurrent  runtimeports.OwnerCurrentRefV1 `json:"execution_open_current"`
	EndpointCurrent       runtimeports.OwnerCurrentRefV1 `json:"endpoint_current"`
	ExecutionReadyCurrent runtimeports.OwnerCurrentRefV1 `json:"execution_ready_current"`
	CheckedUnixNano       int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano       int64                          `json:"expires_unix_nano"`
	ResultDigest          core.Digest                    `json:"result_digest"`
}

func (r AgentActivationResultV2) ValidateFor(request AgentActivationStartRequestV2, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if r.ContractVersion != AgentActivationContractVersionV2 || r.ActivationID != request.ActivationID || r.StartRequestDigest != request.RequestDigest || r.ExecutionScope.Validate() != nil || r.ExecutionScope.SandboxLease == nil || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano || r.ExpiresUnixNano > request.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation V2 final result is incomplete or drifted")
	}
	if r.ExecutionScope.Identity != request.ProposedScope.Identity || r.ExecutionScope.Lineage != request.ProposedScope.Lineage || r.ExecutionScope.Instance != request.ProposedScope.Instance || r.ExecutionScope.AuthorityEpoch != request.ProposedScope.AuthorityEpoch {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation V2 final scope changed the proposed scope")
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(r.ExecutionScope)
	if err != nil || scopeDigest != r.ExecutionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation V2 final scope digest drifted")
	}
	for _, ref := range []runtimeports.OwnerCurrentRefV1{r.ActivationCurrent, r.SandboxActiveCurrent, r.ExecutionOpenCurrent, r.EndpointCurrent, r.ExecutionReadyCurrent} {
		if err := ref.Validate(); err != nil {
			return err
		}
		if r.ExpiresUnixNano > ref.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent activation V2 final result exceeds an Owner current")
		}
	}
	if now.IsZero() || now.UnixNano() < r.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Agent activation V2 final result clock regressed")
	}
	if !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent activation V2 final result expired")
	}
	digest, err := AgentActivationResultDigestV2(r)
	if err != nil || digest != r.ResultDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation V2 final result digest drifted")
	}
	return nil
}

func AgentActivationResultDigestV2(r AgentActivationResultV2) (core.Digest, error) {
	r.ContractVersion = AgentActivationContractVersionV2
	r.ResultDigest = ""
	return core.CanonicalJSONDigest(agentActivationCanonicalDomainV2, AgentActivationContractVersionV2, "AgentActivationResultV2", r)
}

func SealAgentActivationResultV2(r AgentActivationResultV2, request AgentActivationStartRequestV2) (AgentActivationResultV2, error) {
	r.ContractVersion = AgentActivationContractVersionV2
	r.ActivationID = request.ActivationID
	r.StartRequestDigest = request.RequestDigest
	provided := r.ResultDigest
	r.ResultDigest = ""
	digest, err := AgentActivationResultDigestV2(r)
	if err != nil {
		return AgentActivationResultV2{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent activation V2 final result supplied another digest")
	}
	r.ResultDigest = digest
	return r, nil
}

type AgentActivationVersionClaimV2 struct {
	ActivationID       string        `json:"activation_id"`
	ClaimedVersion     string        `json:"claimed_contract_version"`
	StartRequestDigest core.Digest   `json:"start_request_digest"`
	CoordinationID     string        `json:"coordination_id"`
	InitialRevision    core.Revision `json:"initial_revision"`
	InitialFactDigest  core.Digest   `json:"initial_fact_digest"`
	CreatedUnixNano    int64         `json:"created_unix_nano"`
	ClaimDigest        core.Digest   `json:"claim_digest"`
}

func (c AgentActivationVersionClaimV2) ValidateFor(f AgentActivationCoordinationFactV2) error {
	if c.ActivationID != f.ActivationID || c.ClaimedVersion != AgentActivationContractVersionV2 || c.StartRequestDigest != f.Request.RequestDigest || c.CoordinationID != f.ActivationID || c.InitialRevision != 1 || c.CreatedUnixNano != f.CreatedUnixNano || c.InitialFactDigest != f.initialDigestWithoutClaimV2() {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation V2 version claim drifted from its initial Fact")
	}
	digest, err := AgentActivationVersionClaimDigestV2(c)
	if err != nil || digest != c.ClaimDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation V2 version claim digest drifted")
	}
	return nil
}

func AgentActivationVersionClaimDigestV2(c AgentActivationVersionClaimV2) (core.Digest, error) {
	c.ClaimDigest = ""
	return core.CanonicalJSONDigest(agentActivationCanonicalDomainV2, AgentActivationContractVersionV2, "AgentActivationVersionClaimV2", c)
}

type AgentActivationCoordinationFactV2 struct {
	ContractVersion string                        `json:"contract_version"`
	ActivationID    string                        `json:"activation_id"`
	Revision        core.Revision                 `json:"revision"`
	Request         AgentActivationStartRequestV2 `json:"request"`
	Claim           AgentActivationVersionClaimV2 `json:"version_claim"`
	Events          []AgentActivationStepEventV2  `json:"events"`
	Result          *AgentActivationResultV2      `json:"result,omitempty"`
	CreatedUnixNano int64                         `json:"created_unix_nano"`
	UpdatedUnixNano int64                         `json:"updated_unix_nano"`
	ExpiresUnixNano int64                         `json:"expires_unix_nano"`
	Digest          core.Digest                   `json:"digest"`
}

func NewAgentActivationCoordinationFactV2(request AgentActivationStartRequestV2, first AgentActivationStepEventV2, now time.Time) (AgentActivationCoordinationFactV2, error) {
	if err := request.Validate(); err != nil {
		return AgentActivationCoordinationFactV2{}, err
	}
	if now.IsZero() || now.UnixNano() <= 0 || !now.Before(time.Unix(0, request.RequestedNotAfterUnixNano)) {
		return AgentActivationCoordinationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Agent activation V2 initial clock is invalid")
	}
	f := AgentActivationCoordinationFactV2{
		ContractVersion: AgentActivationContractVersionV2, ActivationID: request.ActivationID, Revision: 1,
		Request: request, Events: []AgentActivationStepEventV2{first}, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: request.RequestedNotAfterUnixNano,
	}
	initial, err := f.initialDigestWithoutClaimV2WithError()
	if err != nil {
		return AgentActivationCoordinationFactV2{}, err
	}
	f.Claim = AgentActivationVersionClaimV2{ActivationID: request.ActivationID, ClaimedVersion: AgentActivationContractVersionV2, StartRequestDigest: request.RequestDigest, CoordinationID: request.ActivationID, InitialRevision: 1, InitialFactDigest: initial, CreatedUnixNano: now.UnixNano()}
	f.Claim.ClaimDigest, err = AgentActivationVersionClaimDigestV2(f.Claim)
	if err != nil {
		return AgentActivationCoordinationFactV2{}, err
	}
	return SealAgentActivationCoordinationFactV2(f)
}

func (f AgentActivationCoordinationFactV2) RefV2() AgentActivationCoordinationRefV2 {
	return AgentActivationCoordinationRefV2{ActivationID: f.ActivationID, Revision: f.Revision, Digest: f.Digest, StartRequestDigest: f.Request.RequestDigest}
}

func (f AgentActivationCoordinationFactV2) Validate() error {
	if f.ContractVersion != AgentActivationContractVersionV2 || f.ActivationID != f.Request.ActivationID || f.Request.Validate() != nil || f.Revision == 0 || int(f.Revision) != len(f.Events) || len(f.Events) == 0 || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano || f.ExpiresUnixNano != f.Request.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation V2 coordination Fact is incomplete")
	}
	if err := f.Claim.ValidateFor(f); err != nil {
		return err
	}
	stepIndex := 0
	state := AgentActivationStepEventStateV2("")
	var predecessor *AgentActivationStepResultRefV2
	var activeRequest *AgentActivationStepRequestV2
	lastTime := int64(0)
	for index, event := range f.Events {
		if err := event.Validate(); err != nil {
			return err
		}
		if event.Sequence != uint32(index+1) || stepIndex >= len(agentActivationStepOrderV2) || event.Step != agentActivationStepOrderV2[stepIndex] || event.RecordedUnixNano < lastTime {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation V2 event order drifted")
		}
		lastTime = event.RecordedUnixNano
		if event.AttemptID != mustAttemptIDV2(f.ActivationID, f.Request.RequestDigest, event.Step) {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation V2 event attempt drifted")
		}
		switch state {
		case "":
			if event.State != AgentActivationStepIntentRecordedV2 {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Agent activation V2 step must begin with intent")
			}
			inputs, err := buildAgentActivationStepInputsV2(f.Request, event.Step, predecessor)
			if err != nil {
				return err
			}
			baseDigest, err := agentActivationStepBaseDigestV2(f.Request, event.Step, inputs)
			if err != nil || event.RequestDigest != baseDigest {
				return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation V2 intent base digest drifted")
			}
			activeRequest = nil
		case AgentActivationStepIntentRecordedV2:
			if event.State != AgentActivationStepInvocationRecordedV2 {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Agent activation V2 intent must persist invocation")
			}
			expectedCoordination, err := agentActivationCoordinationPrefixRefV2(f, index)
			if err != nil {
				return err
			}
			if event.Request.InvocationSequence != event.Sequence || event.Request.InvocationEventDigest != f.Events[index-1].Digest || event.Request.Coordination != expectedCoordination {
				return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation invocation did not bind the exact preceding intent and coordination Fact")
			}
			if err := validateActivationInvocationRequestV2(f, *event.Request, predecessor); err != nil {
				return err
			}
			copy := *event.Request
			activeRequest = &copy
		case AgentActivationStepInvocationRecordedV2:
			if event.State != AgentActivationStepOutcomeUnknownV2 && event.State != AgentActivationStepResultRecordedV2 {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Agent activation V2 invocation successor is invalid")
			}
			if activeRequest == nil || event.Request.RequestDigest != activeRequest.RequestDigest {
				return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation V2 invocation request changed")
			}
		case AgentActivationStepOutcomeUnknownV2:
			if event.State != AgentActivationStepResultRecordedV2 {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Unknown activation outcome may only resolve by Inspect")
			}
			if activeRequest == nil || event.Request.RequestDigest != activeRequest.RequestDigest {
				return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Unknown activation outcome request changed")
			}
		}
		state = event.State
		if event.State == AgentActivationStepResultRecordedV2 {
			if event.Result == nil {
				return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Result event is empty")
			}
			if activeRequest == nil {
				return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Result event has no persisted invocation request")
			}
			if err := event.Result.ValidateFor(*activeRequest, time.Unix(0, event.RecordedUnixNano)); err != nil {
				return err
			}
			predecessor = &event.Result.Ref
			stepIndex++
			state = ""
			activeRequest = nil
		}
	}
	complete := stepIndex == len(agentActivationStepOrderV2) && state == ""
	if complete != (f.Result != nil) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation V2 completion and result disagree")
	}
	if f.Result != nil {
		if err := f.Result.ValidateFor(f.Request, time.Unix(0, f.UpdatedUnixNano)); err != nil {
			return err
		}
	}
	if f.UpdatedUnixNano != f.Events[len(f.Events)-1].RecordedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation V2 Fact clock does not bind its last event")
	}
	digest, err := AgentActivationCoordinationFactDigestV2(f)
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation V2 coordination Fact digest drifted")
	}
	return nil
}

func (f AgentActivationCoordinationFactV2) initialDigestWithoutClaimV2() core.Digest {
	digest, _ := f.initialDigestWithoutClaimV2WithError()
	return digest
}

func (f AgentActivationCoordinationFactV2) initialDigestWithoutClaimV2WithError() (core.Digest, error) {
	if len(f.Events) == 0 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Initial activation Fact requires its first event")
	}
	return core.CanonicalJSONDigest(agentActivationCanonicalDomainV2, AgentActivationContractVersionV2, "AgentActivationInitialFactV2", struct {
		ActivationID string                        `json:"activation_id"`
		Request      AgentActivationStartRequestV2 `json:"request"`
		First        AgentActivationStepEventV2    `json:"first_event"`
		Created      int64                         `json:"created_unix_nano"`
	}{f.ActivationID, f.Request, f.Events[0], f.CreatedUnixNano})
}

func AgentActivationCoordinationFactDigestV2(f AgentActivationCoordinationFactV2) (core.Digest, error) {
	f.ContractVersion = AgentActivationContractVersionV2
	f.Events = append([]AgentActivationStepEventV2{}, f.Events...)
	f.Digest = ""
	return core.CanonicalJSONDigest(agentActivationCanonicalDomainV2, AgentActivationContractVersionV2, "AgentActivationCoordinationFactV2", f)
}

func SealAgentActivationCoordinationFactV2(f AgentActivationCoordinationFactV2) (AgentActivationCoordinationFactV2, error) {
	f.ContractVersion = AgentActivationContractVersionV2
	f.Events = append([]AgentActivationStepEventV2{}, f.Events...)
	provided := f.Digest
	f.Digest = ""
	digest, err := AgentActivationCoordinationFactDigestV2(f)
	if err != nil {
		return AgentActivationCoordinationFactV2{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationCoordinationFactV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent activation V2 Fact supplied another digest")
	}
	f.Digest = digest
	return f, f.Validate()
}

func ValidateAgentActivationCoordinationTransitionV2(current, next AgentActivationCoordinationFactV2) error {
	if current.Validate() != nil || next.Validate() != nil || current.ActivationID != next.ActivationID || current.Request.RequestDigest != next.Request.RequestDigest || current.Claim.ClaimDigest != next.Claim.ClaimDigest || next.Revision != current.Revision+1 || len(next.Events) != len(current.Events)+1 || next.CreatedUnixNano != current.CreatedUnixNano || next.ExpiresUnixNano != current.ExpiresUnixNano || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Agent activation V2 successor coordinates drifted")
	}
	for index := range current.Events {
		if current.Events[index].Digest != next.Events[index].Digest {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Agent activation V2 history is not append-only")
		}
	}
	if current.Result != nil {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "Completed Agent activation V2 Fact is immutable")
	}
	return nil
}

func buildAgentActivationStepInputsV2(request AgentActivationStartRequestV2, step AgentActivationStepV2, predecessor *AgentActivationStepResultRefV2) (AgentActivationStepInputsV2, error) {
	inputs := AgentActivationStepInputsV2{ProposedScope: request.ProposedScope, Predecessor: predecessor}
	if step == AgentActivationIdentityBudgetV2 {
		inputs.Authority = &request.AuthorityCurrent
		inputs.Policy = &request.PolicyCurrent
	}
	// Dispatch refs are supplied by the Owner-facing adapter immediately before
	// invocation; Fact validation only rebuilds persisted non-dispatch inputs.
	if step == AgentActivationSandboxAllocateV2 || step == AgentActivationSandboxActivateV2 || step == AgentActivationExecutionOpenV2 {
		return inputs, nil
	}
	return SealAgentActivationStepInputsV2(inputs, step)
}

func agentActivationStepBaseDigestV2(request AgentActivationStartRequestV2, step AgentActivationStepV2, inputs AgentActivationStepInputsV2) (core.Digest, error) {
	copy := inputs
	copy.Dispatch = nil
	copy.InputDigest = ""
	return core.CanonicalJSONDigest(agentActivationCanonicalDomainV2, AgentActivationContractVersionV2, "AgentActivationStepBaseV2", struct {
		ActivationID string                      `json:"activation_id"`
		StartDigest  core.Digest                 `json:"start_digest"`
		Step         AgentActivationStepV2       `json:"step"`
		Inputs       AgentActivationStepInputsV2 `json:"inputs"`
	}{request.ActivationID, request.RequestDigest, step, copy})
}

// AgentActivationStepIntentBaseDigestV2 is exported so durable stores and
// conformance kits can construct and verify the initial write-ahead event.
func AgentActivationStepIntentBaseDigestV2(request AgentActivationStartRequestV2, step AgentActivationStepV2, predecessor *AgentActivationStepResultRefV2) (core.Digest, error) {
	inputs, err := buildAgentActivationStepInputsV2(request, step, predecessor)
	if err != nil {
		return "", err
	}
	return agentActivationStepBaseDigestV2(request, step, inputs)
}

func validateActivationInvocationRequestV2(f AgentActivationCoordinationFactV2, request AgentActivationStepRequestV2, predecessor *AgentActivationStepResultRefV2) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Coordination.ActivationID != f.ActivationID || request.Coordination.StartRequestDigest != f.Request.RequestDigest || request.Inputs.ProposedScope != f.Request.ProposedScope || !sameActivationResultRefV2(request.Inputs.Predecessor, predecessor) || request.RequestedNotAfterUnixNano != f.Request.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation V2 invocation request does not bind the coordination Fact")
	}
	if request.Step == AgentActivationIdentityBudgetV2 && (request.Inputs.Authority == nil || request.Inputs.Policy == nil || *request.Inputs.Authority != f.Request.AuthorityCurrent || *request.Inputs.Policy != f.Request.PolicyCurrent) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation V2 identity inputs drifted")
	}
	return nil
}

// agentActivationCoordinationPrefixRefV2 deterministically reconstructs the
// exact append-only Fact immediately before events[eventCount]. It is used by
// aggregate validation so a persisted invocation cannot masquerade as having
// been prepared from a different revision/digest that happened to share the
// same ActivationID and Start digest.
func agentActivationCoordinationPrefixRefV2(f AgentActivationCoordinationFactV2, eventCount int) (AgentActivationCoordinationRefV2, error) {
	if eventCount <= 0 || eventCount > len(f.Events) {
		return AgentActivationCoordinationRefV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation coordination prefix is invalid")
	}
	prefix := f
	prefix.Events = append([]AgentActivationStepEventV2{}, f.Events[:eventCount]...)
	prefix.Revision = core.Revision(eventCount)
	prefix.Result = nil
	prefix.UpdatedUnixNano = prefix.Events[eventCount-1].RecordedUnixNano
	prefix.Digest = ""
	digest, err := AgentActivationCoordinationFactDigestV2(prefix)
	if err != nil {
		return AgentActivationCoordinationRefV2{}, err
	}
	return AgentActivationCoordinationRefV2{
		ActivationID:       prefix.ActivationID,
		Revision:           prefix.Revision,
		Digest:             digest,
		StartRequestDigest: prefix.Request.RequestDigest,
	}, nil
}

func sameActivationResultRefV2(left, right *AgentActivationStepResultRefV2) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func mustAttemptIDV2(activationID string, startDigest core.Digest, step AgentActivationStepV2) string {
	id, _ := DeriveAgentActivationStepAttemptIDV2(activationID, startDigest, step)
	return id
}

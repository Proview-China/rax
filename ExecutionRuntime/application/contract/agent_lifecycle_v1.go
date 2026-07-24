package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	AgentLifecycleContractVersionV1 = "praxis.application.agent-lifecycle/v1"
	MaxAgentLifecycleIDBytesV1      = 160
)

type AgentActivationStartRequestV1 struct {
	ContractVersion           string                         `json:"contract_version"`
	ActivationID              string                         `json:"activation_id"`
	AttemptID                 string                         `json:"attempt_id"`
	IdempotencyKey            string                         `json:"idempotency_key"`
	DefinitionCurrent         runtimeports.OwnerCurrentRefV1 `json:"definition_current"`
	PlanCurrent               runtimeports.OwnerCurrentRefV1 `json:"plan_current"`
	AssemblyCurrent           runtimeports.OwnerCurrentRefV1 `json:"assembly_current"`
	BindingSetCurrent         runtimeports.OwnerCurrentRefV1 `json:"binding_set_current"`
	AuthorityCurrent          runtimeports.OwnerCurrentRefV1 `json:"authority_current"`
	PolicyCurrent             runtimeports.OwnerCurrentRefV1 `json:"policy_current"`
	BudgetCurrent             runtimeports.OwnerCurrentRefV1 `json:"budget_current"`
	CredentialCurrent         runtimeports.OwnerCurrentRefV1 `json:"credential_current"`
	SandboxAdapterBinding     runtimeports.OwnerCurrentRefV1 `json:"sandbox_adapter_binding"`
	ExecutionAdapterBinding   runtimeports.OwnerCurrentRefV1 `json:"execution_adapter_binding"`
	RequestedNotAfterUnixNano int64                          `json:"requested_not_after_unix_nano"`
	RequestDigest             core.Digest                    `json:"request_digest"`
}

func (r AgentActivationStartRequestV1) inputRefs() []runtimeports.OwnerCurrentRefV1 {
	return []runtimeports.OwnerCurrentRefV1{
		r.DefinitionCurrent, r.PlanCurrent, r.AssemblyCurrent, r.BindingSetCurrent,
		r.AuthorityCurrent, r.PolicyCurrent, r.BudgetCurrent, r.CredentialCurrent,
		r.SandboxAdapterBinding, r.ExecutionAdapterBinding,
	}
}

func (r AgentActivationStartRequestV1) Validate() error {
	if r.ContractVersion != AgentLifecycleContractVersionV1 || !validAgentLifecycleIDV1(r.ActivationID) || !validAgentLifecycleIDV1(r.AttemptID) || !validAgentLifecycleIDV1(r.IdempotencyKey) || r.RequestedNotAfterUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation request identity or requested window is incomplete")
	}
	minimum := r.RequestedNotAfterUnixNano
	seen := make(map[string]struct{}, len(r.inputRefs()))
	for _, ref := range r.inputRefs() {
		if err := ref.Validate(); err != nil {
			return err
		}
		key := ref.Owner.Domain + "\x00" + string(ref.Owner.ID) + "\x00" + ref.ContractVersion + "\x00" + ref.ID
		if _, exists := seen[key]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Agent activation input roles alias one Owner current")
		}
		seen[key] = struct{}{}
		if ref.ExpiresUnixNano < minimum {
			minimum = ref.ExpiresUnixNano
		}
	}
	if r.RequestedNotAfterUnixNano > minimum {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent activation requested window exceeds an exact input current")
	}
	digest, err := AgentActivationStartRequestDigestV1(r)
	if err != nil || digest != r.RequestDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation request digest drifted")
	}
	return nil
}

func AgentActivationStartRequestDigestV1(r AgentActivationStartRequestV1) (core.Digest, error) {
	r.ContractVersion = AgentLifecycleContractVersionV1
	r.RequestDigest = ""
	return core.CanonicalJSONDigest("praxis.application.agent-lifecycle", AgentLifecycleContractVersionV1, "AgentActivationStartRequestV1", r)
}

func SealAgentActivationStartRequestV1(r AgentActivationStartRequestV1) (AgentActivationStartRequestV1, error) {
	if r.ContractVersion != "" && r.ContractVersion != AgentLifecycleContractVersionV1 {
		return AgentActivationStartRequestV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation request contract version drifted")
	}
	r.ContractVersion = AgentLifecycleContractVersionV1
	provided := r.RequestDigest
	r.RequestDigest = ""
	digest, err := AgentActivationStartRequestDigestV1(r)
	if err != nil {
		return AgentActivationStartRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationStartRequestV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent activation request supplied a wrong digest")
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

type AgentActivationResultV1 struct {
	ContractVersion       string                         `json:"contract_version"`
	Ref                   runtimeports.OwnerCurrentRefV1 `json:"ref"`
	ActivationID          string                         `json:"activation_id"`
	AttemptID             string                         `json:"attempt_id"`
	RequestDigest         core.Digest                    `json:"request_digest"`
	ExecutionScope        core.ExecutionScope            `json:"execution_scope"`
	ExecutionScopeDigest  core.Digest                    `json:"execution_scope_digest"`
	ActivationCurrent     runtimeports.OwnerCurrentRefV1 `json:"activation_current"`
	SandboxLease          core.SandboxLeaseRef           `json:"sandbox_lease"`
	SandboxLeaseCurrent   runtimeports.OwnerCurrentRefV1 `json:"sandbox_lease_current"`
	SandboxActiveCurrent  runtimeports.OwnerCurrentRefV1 `json:"sandbox_active_current"`
	ExecutionReadyCurrent runtimeports.OwnerCurrentRefV1 `json:"execution_ready_current"`
	CheckedUnixNano       int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano       int64                          `json:"expires_unix_nano"`
	ResultDigest          core.Digest                    `json:"result_digest"`
}

func (r AgentActivationResultV1) outputRefs() []runtimeports.OwnerCurrentRefV1 {
	return []runtimeports.OwnerCurrentRefV1{r.ActivationCurrent, r.SandboxLeaseCurrent, r.SandboxActiveCurrent, r.ExecutionReadyCurrent}
}

func (r AgentActivationResultV1) Validate() error {
	if r.ContractVersion != AgentLifecycleContractVersionV1 || !validAgentLifecycleIDV1(r.ActivationID) || !validAgentLifecycleIDV1(r.AttemptID) || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation result identity or current window is incomplete")
	}
	if err := r.Ref.Validate(); err != nil {
		return err
	}
	if r.Ref.Owner != agentLifecycleOwnerV1() || r.Ref.ContractVersion != AgentLifecycleContractVersionV1 || r.Ref.ID != r.ActivationID+"/result" || r.Ref.Revision != 1 || r.Ref.ExpiresUnixNano != r.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation result exact Ref drifted")
	}
	if err := r.RequestDigest.Validate(); err != nil {
		return err
	}
	if err := r.ExecutionScope.Validate(); err != nil {
		return err
	}
	if err := r.SandboxLease.Validate(); err != nil {
		return err
	}
	if r.ExecutionScope.SandboxLease == nil || *r.ExecutionScope.SandboxLease != r.SandboxLease {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation ExecutionScope does not bind the returned Sandbox lease")
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(r.ExecutionScope)
	if err != nil || scopeDigest != r.ExecutionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation ExecutionScope digest drifted")
	}
	minimum := r.ExpiresUnixNano
	for _, ref := range r.outputRefs() {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.ExpiresUnixNano < minimum {
			minimum = ref.ExpiresUnixNano
		}
	}
	if r.ExpiresUnixNano > minimum {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent activation result TTL exceeds an exact output current")
	}
	digest, err := AgentActivationResultDigestV1(r)
	if err != nil || digest != r.ResultDigest || digest != r.Ref.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation result digest drifted")
	}
	return nil
}

func (r AgentActivationResultV1) ValidateFor(request AgentActivationStartRequestV1, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if r.ActivationID != request.ActivationID || r.AttemptID != request.AttemptID || r.RequestDigest != request.RequestDigest || r.ExpiresUnixNano > request.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation result does not bind the exact request")
	}
	if now.IsZero() || now.UnixNano() < r.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Agent activation result clock regressed")
	}
	if !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent activation result expired")
	}
	return nil
}

func AgentActivationResultDigestV1(r AgentActivationResultV1) (core.Digest, error) {
	r.ContractVersion = AgentLifecycleContractVersionV1
	r.Ref.Digest = ""
	r.ResultDigest = ""
	return core.CanonicalJSONDigest("praxis.application.agent-lifecycle", AgentLifecycleContractVersionV1, "AgentActivationResultV1", r)
}

func SealAgentActivationResultV1(r AgentActivationResultV1) (AgentActivationResultV1, error) {
	if r.ContractVersion != "" && r.ContractVersion != AgentLifecycleContractVersionV1 {
		return AgentActivationResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation result contract version drifted")
	}
	r.ContractVersion = AgentLifecycleContractVersionV1
	r.Ref.Owner = agentLifecycleOwnerV1()
	r.Ref.ContractVersion = AgentLifecycleContractVersionV1
	r.Ref.ID = r.ActivationID + "/result"
	r.Ref.Revision = 1
	r.Ref.ExpiresUnixNano = r.ExpiresUnixNano
	providedRef, providedResult := r.Ref.Digest, r.ResultDigest
	r.Ref.Digest, r.ResultDigest = "", ""
	digest, err := AgentActivationResultDigestV1(r)
	if err != nil {
		return AgentActivationResultV1{}, err
	}
	if (providedRef != "" && providedRef != digest) || (providedResult != "" && providedResult != digest) {
		return AgentActivationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent activation result supplied a wrong digest")
	}
	r.Ref.Digest, r.ResultDigest = digest, digest
	return r, r.Validate()
}

type AgentTerminationRequestV1 struct {
	ContractVersion           string                         `json:"contract_version"`
	StopID                    string                         `json:"stop_id"`
	AttemptID                 string                         `json:"attempt_id"`
	IdempotencyKey            string                         `json:"idempotency_key"`
	ActivationResult          runtimeports.OwnerCurrentRefV1 `json:"activation_result"`
	ActivationCurrent         runtimeports.OwnerCurrentRefV1 `json:"activation_current"`
	StopPolicyCurrent         runtimeports.OwnerCurrentRefV1 `json:"stop_policy_current"`
	ExecutionScope            core.ExecutionScope            `json:"execution_scope"`
	ExecutionScopeDigest      core.Digest                    `json:"execution_scope_digest"`
	SandboxLease              core.SandboxLeaseRef           `json:"sandbox_lease"`
	RequestedNotAfterUnixNano int64                          `json:"requested_not_after_unix_nano"`
	RequestDigest             core.Digest                    `json:"request_digest"`
}

func (r AgentTerminationRequestV1) Validate() error {
	if r.ContractVersion != AgentLifecycleContractVersionV1 || !validAgentLifecycleIDV1(r.StopID) || !validAgentLifecycleIDV1(r.AttemptID) || !validAgentLifecycleIDV1(r.IdempotencyKey) || r.RequestedNotAfterUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent termination request identity or requested window is incomplete")
	}
	minimum := r.RequestedNotAfterUnixNano
	for _, ref := range []runtimeports.OwnerCurrentRefV1{r.ActivationResult, r.ActivationCurrent, r.StopPolicyCurrent} {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.ExpiresUnixNano < minimum {
			minimum = ref.ExpiresUnixNano
		}
	}
	if r.RequestedNotAfterUnixNano > minimum {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent termination requested window exceeds an exact input current")
	}
	if err := r.ExecutionScope.Validate(); err != nil {
		return err
	}
	if err := r.SandboxLease.Validate(); err != nil {
		return err
	}
	if r.ExecutionScope.SandboxLease == nil || *r.ExecutionScope.SandboxLease != r.SandboxLease {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent termination scope does not bind its Sandbox lease")
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(r.ExecutionScope)
	if err != nil || scopeDigest != r.ExecutionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent termination ExecutionScope digest drifted")
	}
	digest, err := AgentTerminationRequestDigestV1(r)
	if err != nil || digest != r.RequestDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent termination request digest drifted")
	}
	return nil
}

func AgentTerminationRequestDigestV1(r AgentTerminationRequestV1) (core.Digest, error) {
	r.ContractVersion = AgentLifecycleContractVersionV1
	r.RequestDigest = ""
	return core.CanonicalJSONDigest("praxis.application.agent-lifecycle", AgentLifecycleContractVersionV1, "AgentTerminationRequestV1", r)
}

func SealAgentTerminationRequestV1(r AgentTerminationRequestV1) (AgentTerminationRequestV1, error) {
	if r.ContractVersion != "" && r.ContractVersion != AgentLifecycleContractVersionV1 {
		return AgentTerminationRequestV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent termination request contract version drifted")
	}
	r.ContractVersion = AgentLifecycleContractVersionV1
	provided := r.RequestDigest
	r.RequestDigest = ""
	digest, err := AgentTerminationRequestDigestV1(r)
	if err != nil {
		return AgentTerminationRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentTerminationRequestV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent termination request supplied a wrong digest")
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

type AgentTerminationStateV1 string

const (
	AgentTerminationStoppedV1       AgentTerminationStateV1 = "stopped"
	AgentTerminationIndeterminateV1 AgentTerminationStateV1 = "indeterminate"
)

type AgentTerminationResidualV1 struct {
	ResidualID     string                         `json:"residual_id"`
	Owner          core.OwnerRef                  `json:"owner"`
	InspectCurrent runtimeports.OwnerCurrentRefV1 `json:"inspect_current"`
	Digest         core.Digest                    `json:"digest"`
}

func (r AgentTerminationResidualV1) Validate() error {
	if !validAgentLifecycleIDV1(r.ResidualID) || r.Owner.Validate() != nil || r.InspectCurrent.Validate() != nil || r.InspectCurrent.Owner != r.Owner {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent termination residual is incomplete")
	}
	digest, err := AgentTerminationResidualDigestV1(r)
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent termination residual digest drifted")
	}
	return nil
}

func AgentTerminationResidualDigestV1(r AgentTerminationResidualV1) (core.Digest, error) {
	r.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.agent-lifecycle", AgentLifecycleContractVersionV1, "AgentTerminationResidualV1", r)
}

func SealAgentTerminationResidualV1(r AgentTerminationResidualV1) (AgentTerminationResidualV1, error) {
	provided := r.Digest
	r.Digest = ""
	digest, err := AgentTerminationResidualDigestV1(r)
	if err != nil {
		return AgentTerminationResidualV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentTerminationResidualV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent termination residual supplied a wrong digest")
	}
	r.Digest = digest
	return r, r.Validate()
}

type AgentTerminationResultV1 struct {
	ContractVersion      string                         `json:"contract_version"`
	Ref                  runtimeports.OwnerCurrentRefV1 `json:"ref"`
	StopID               string                         `json:"stop_id"`
	AttemptID            string                         `json:"attempt_id"`
	RequestDigest        core.Digest                    `json:"request_digest"`
	ExecutionScope       core.ExecutionScope            `json:"execution_scope"`
	ExecutionScopeDigest core.Digest                    `json:"execution_scope_digest"`
	ActivationCurrent    runtimeports.OwnerCurrentRefV1 `json:"activation_current"`
	SandboxLease         core.SandboxLeaseRef           `json:"sandbox_lease"`
	TerminationCurrent   runtimeports.OwnerCurrentRefV1 `json:"termination_current"`
	State                AgentTerminationStateV1        `json:"state"`
	Residuals            []AgentTerminationResidualV1   `json:"residuals"`
	CheckedUnixNano      int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                          `json:"expires_unix_nano"`
	ResultDigest         core.Digest                    `json:"result_digest"`
}

func (r AgentTerminationResultV1) Validate() error {
	if r.ContractVersion != AgentLifecycleContractVersionV1 || !validAgentLifecycleIDV1(r.StopID) || !validAgentLifecycleIDV1(r.AttemptID) || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent termination result identity or current window is incomplete")
	}
	if r.Ref.Validate() != nil || r.Ref.Owner != agentLifecycleOwnerV1() || r.Ref.ContractVersion != AgentLifecycleContractVersionV1 || r.Ref.ID != r.StopID+"/result" || r.Ref.Revision != 1 || r.Ref.ExpiresUnixNano != r.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent termination result exact Ref drifted")
	}
	if r.RequestDigest.Validate() != nil || r.ExecutionScope.Validate() != nil || r.ExecutionScopeDigest.Validate() != nil || r.ActivationCurrent.Validate() != nil || r.SandboxLease.Validate() != nil || r.TerminationCurrent.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent termination result closure is incomplete")
	}
	if r.ExecutionScope.SandboxLease == nil || *r.ExecutionScope.SandboxLease != r.SandboxLease {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent termination result scope does not bind its Sandbox lease")
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(r.ExecutionScope)
	if err != nil || scopeDigest != r.ExecutionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent termination result ExecutionScope digest drifted")
	}
	if r.ExpiresUnixNano > r.ActivationCurrent.ExpiresUnixNano || r.ExpiresUnixNano > r.TerminationCurrent.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent termination result TTL exceeds an exact output current")
	}
	switch r.State {
	case AgentTerminationStoppedV1:
		if len(r.Residuals) != 0 {
			return core.NewError(core.ErrorConflict, core.ReasonRemoteResidualUnresolved, "stopped Agent termination cannot retain residuals")
		}
	case AgentTerminationIndeterminateV1:
		if len(r.Residuals) == 0 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRemoteResidualUnresolved, "indeterminate Agent termination requires an inspectable residual")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Agent termination result state is invalid")
	}
	previous := ""
	for _, residual := range r.Residuals {
		if err := residual.Validate(); err != nil {
			return err
		}
		if residual.ResidualID <= previous {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Agent termination residuals must be sorted and unique")
		}
		previous = residual.ResidualID
	}
	digest, err := AgentTerminationResultDigestV1(r)
	if err != nil || digest != r.ResultDigest || digest != r.Ref.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent termination result digest drifted")
	}
	return nil
}

func (r AgentTerminationResultV1) ValidateFor(request AgentTerminationRequestV1, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if r.StopID != request.StopID || r.AttemptID != request.AttemptID || r.RequestDigest != request.RequestDigest || r.ExecutionScopeDigest != request.ExecutionScopeDigest || r.SandboxLease != request.SandboxLease || r.ExpiresUnixNano > request.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent termination result does not bind the exact request")
	}
	if now.IsZero() || now.UnixNano() < r.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Agent termination result clock regressed")
	}
	if !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent termination result expired")
	}
	return nil
}

func AgentTerminationResultDigestV1(r AgentTerminationResultV1) (core.Digest, error) {
	r.ContractVersion = AgentLifecycleContractVersionV1
	r.Ref.Digest = ""
	r.ResultDigest = ""
	if r.Residuals == nil {
		r.Residuals = []AgentTerminationResidualV1{}
	}
	return core.CanonicalJSONDigest("praxis.application.agent-lifecycle", AgentLifecycleContractVersionV1, "AgentTerminationResultV1", r)
}

func SealAgentTerminationResultV1(r AgentTerminationResultV1) (AgentTerminationResultV1, error) {
	if r.ContractVersion != "" && r.ContractVersion != AgentLifecycleContractVersionV1 {
		return AgentTerminationResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent termination result contract version drifted")
	}
	r.ContractVersion = AgentLifecycleContractVersionV1
	r.Residuals = append([]AgentTerminationResidualV1{}, r.Residuals...)
	for index := range r.Residuals {
		sealed, err := SealAgentTerminationResidualV1(r.Residuals[index])
		if err != nil {
			return AgentTerminationResultV1{}, err
		}
		r.Residuals[index] = sealed
	}
	sortTerminationResidualsV1(r.Residuals)
	r.Ref.Owner = agentLifecycleOwnerV1()
	r.Ref.ContractVersion = AgentLifecycleContractVersionV1
	r.Ref.ID = r.StopID + "/result"
	r.Ref.Revision = 1
	r.Ref.ExpiresUnixNano = r.ExpiresUnixNano
	providedRef, providedResult := r.Ref.Digest, r.ResultDigest
	r.Ref.Digest, r.ResultDigest = "", ""
	digest, err := AgentTerminationResultDigestV1(r)
	if err != nil {
		return AgentTerminationResultV1{}, err
	}
	if (providedRef != "" && providedRef != digest) || (providedResult != "" && providedResult != digest) {
		return AgentTerminationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent termination result supplied a wrong digest")
	}
	r.Ref.Digest, r.ResultDigest = digest, digest
	return r, r.Validate()
}

func sortTerminationResidualsV1(values []AgentTerminationResidualV1) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j].ResidualID < values[j-1].ResidualID; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func agentLifecycleOwnerV1() core.OwnerRef {
	return core.OwnerRef{Domain: "praxis.application", ID: core.OwnerID("application")}
}

func validAgentLifecycleIDV1(value string) bool {
	return strings.TrimSpace(value) != "" && value == strings.TrimSpace(value) && len(value) <= MaxAgentLifecycleIDBytesV1
}

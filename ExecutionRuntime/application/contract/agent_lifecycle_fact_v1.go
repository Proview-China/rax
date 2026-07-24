package contract

import (
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"time"
)

const AgentLifecycleFactContractVersionV1 = "praxis.application.agent-lifecycle-fact/v1"

type AgentLifecycleFactStateV1 string

const (
	AgentLifecycleFactActiveV1        AgentLifecycleFactStateV1 = "active"
	AgentLifecycleFactStoppedV1       AgentLifecycleFactStateV1 = "stopped"
	AgentLifecycleFactIndeterminateV1 AgentLifecycleFactStateV1 = "indeterminate"
)

// AgentLifecycleFactV1 is the additive durable aggregate for one activation.
// Revision one records the exact activation request/result. Revision two may
// only append the exact termination request/result; a terminated aggregate is
// immutable.
type AgentLifecycleFactV1 struct {
	ContractVersion string                        `json:"contract_version"`
	LifecycleID     string                        `json:"lifecycle_id"`
	Revision        core.Revision                 `json:"revision"`
	PreviousDigest  core.Digest                   `json:"previous_digest,omitempty"`
	State           AgentLifecycleFactStateV1     `json:"state"`
	StartRequest    AgentActivationStartRequestV1 `json:"start_request"`
	Activation      AgentActivationResultV1       `json:"activation"`
	StopRequest     *AgentTerminationRequestV1    `json:"stop_request,omitempty"`
	Termination     *AgentTerminationResultV1     `json:"termination,omitempty"`
	CheckedUnixNano int64                         `json:"checked_unix_nano"`
	ExpiresUnixNano int64                         `json:"expires_unix_nano"`
	Digest          core.Digest                   `json:"digest"`
}

func (f AgentLifecycleFactV1) Validate() error {
	if f.ContractVersion != AgentLifecycleFactContractVersionV1 || !validAgentLifecycleIDV1(f.LifecycleID) || f.LifecycleID != f.StartRequest.ActivationID || f.StartRequest.Validate() != nil || f.Activation.ValidateFor(f.StartRequest, time.Unix(0, f.Activation.CheckedUnixNano)) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent lifecycle Fact is incomplete")
	}
	if f.Activation.ActivationID != f.LifecycleID || f.Activation.AttemptID != f.StartRequest.AttemptID || f.Activation.RequestDigest != f.StartRequest.RequestDigest || f.Activation.ExpiresUnixNano > f.StartRequest.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent lifecycle activation drifted from its exact request")
	}
	switch f.State {
	case AgentLifecycleFactActiveV1:
		if f.Revision != 1 || f.PreviousDigest != "" || f.StopRequest != nil || f.Termination != nil || f.CheckedUnixNano != f.Activation.CheckedUnixNano || f.ExpiresUnixNano != f.Activation.ExpiresUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "active Agent lifecycle Fact must be revision one")
		}
	case AgentLifecycleFactStoppedV1, AgentLifecycleFactIndeterminateV1:
		if f.Revision != 2 || f.PreviousDigest.Validate() != nil || f.StopRequest == nil || f.Termination == nil || f.StopRequest.Validate() != nil || f.Termination.ValidateFor(*f.StopRequest, time.Unix(0, f.Termination.CheckedUnixNano)) != nil {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "terminated Agent lifecycle Fact must be revision two")
		}
		if f.StopRequest.ActivationResult != f.Activation.Ref || f.StopRequest.ActivationCurrent != f.Activation.ActivationCurrent || f.StopRequest.ExecutionScopeDigest != f.Activation.ExecutionScopeDigest || f.StopRequest.SandboxLease != f.Activation.SandboxLease || f.Termination.ActivationCurrent != f.StopRequest.ActivationCurrent || f.Termination.StopID != f.StopRequest.StopID || f.Termination.AttemptID != f.StopRequest.AttemptID || f.Termination.RequestDigest != f.StopRequest.RequestDigest || f.Termination.State != AgentTerminationStateV1(f.State) || f.CheckedUnixNano != f.Termination.CheckedUnixNano || f.ExpiresUnixNano != f.Termination.ExpiresUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent lifecycle termination drifted from its exact activation or request")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Agent lifecycle Fact state is invalid")
	}
	if f.CheckedUnixNano <= 0 || f.ExpiresUnixNano <= f.CheckedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent lifecycle Fact current window is invalid")
	}
	digest, err := AgentLifecycleFactDigestV1(f)
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent lifecycle Fact digest drifted")
	}
	return nil
}

func AgentLifecycleFactDigestV1(f AgentLifecycleFactV1) (core.Digest, error) {
	f.ContractVersion = AgentLifecycleFactContractVersionV1
	f.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.agent-lifecycle-fact", AgentLifecycleFactContractVersionV1, "AgentLifecycleFactV1", f)
}

func SealAgentLifecycleFactV1(f AgentLifecycleFactV1) (AgentLifecycleFactV1, error) {
	f.ContractVersion = AgentLifecycleFactContractVersionV1
	provided := f.Digest
	f.Digest = ""
	digest, err := AgentLifecycleFactDigestV1(f)
	if err != nil {
		return AgentLifecycleFactV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentLifecycleFactV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent lifecycle Fact supplied a wrong digest")
	}
	f.Digest = digest
	return f, f.Validate()
}

func ValidateAgentLifecycleFactTransitionV1(current, next AgentLifecycleFactV1) error {
	if current.Validate() != nil || next.Validate() != nil || current.LifecycleID != next.LifecycleID || current.State != AgentLifecycleFactActiveV1 || next.State == AgentLifecycleFactActiveV1 || next.Revision != current.Revision+1 || next.PreviousDigest != current.Digest || next.StartRequest.RequestDigest != current.StartRequest.RequestDigest || next.Activation.ResultDigest != current.Activation.ResultDigest || next.CheckedUnixNano < current.CheckedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Agent lifecycle successor coordinates drifted")
	}
	return nil
}

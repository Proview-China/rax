package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const AgentActivationCoordinationContractVersionV1 = "praxis.application.agent-activation-coordination/v1"

type AgentActivationStepV1 string

const (
	AgentActivationPreflightV1       AgentActivationStepV1 = "preflight"
	AgentActivationSnapshotV1        AgentActivationStepV1 = "snapshot"
	AgentActivationIdentityBudgetV1  AgentActivationStepV1 = "identity_budget"
	AgentActivationSandboxAllocateV1 AgentActivationStepV1 = "sandbox_allocate"
	AgentActivationCommitV1          AgentActivationStepV1 = "activation_commit"
	AgentActivationSandboxActivateV1 AgentActivationStepV1 = "sandbox_activate"
	AgentActivationExecutionOpenV1   AgentActivationStepV1 = "execution_open"
	AgentActivationReadyInspectV1    AgentActivationStepV1 = "ready_inspect"

	AgentActivationSandboxReservedQuarantinedV1 = "reserved_quarantined"
)

var agentActivationStepOrderV1 = [...]AgentActivationStepV1{
	AgentActivationPreflightV1,
	AgentActivationSnapshotV1,
	AgentActivationIdentityBudgetV1,
	AgentActivationSandboxAllocateV1,
	AgentActivationCommitV1,
	AgentActivationSandboxActivateV1,
	AgentActivationExecutionOpenV1,
	AgentActivationReadyInspectV1,
}

func AgentActivationStepOrderV1() []AgentActivationStepV1 {
	result := make([]AgentActivationStepV1, len(agentActivationStepOrderV1))
	copy(result, agentActivationStepOrderV1[:])
	return result
}

func (s AgentActivationStepV1) Validate() error {
	for _, candidate := range agentActivationStepOrderV1 {
		if s == candidate {
			return nil
		}
	}
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Agent activation step is invalid")
}

type AgentActivationStepRequestV1 struct {
	ContractVersion           string                `json:"contract_version"`
	ActivationID              string                `json:"activation_id"`
	StartRequestDigest        core.Digest           `json:"start_request_digest"`
	Step                      AgentActivationStepV1 `json:"step"`
	AttemptID                 string                `json:"attempt_id"`
	PredecessorResultDigest   core.Digest           `json:"predecessor_result_digest,omitempty"`
	RequestedNotAfterUnixNano int64                 `json:"requested_not_after_unix_nano"`
	RequestDigest             core.Digest           `json:"request_digest"`
}

func (r AgentActivationStepRequestV1) Validate() error {
	if r.ContractVersion != AgentActivationCoordinationContractVersionV1 || !validAgentLifecycleIDV1(r.ActivationID) || r.StartRequestDigest.Validate() != nil || r.Step.Validate() != nil || !validAgentLifecycleIDV1(r.AttemptID) || r.RequestedNotAfterUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation step request is incomplete")
	}
	expectedAttempt, err := DeriveAgentActivationStepAttemptIDV1(r.ActivationID, r.StartRequestDigest, r.Step)
	if err != nil || expectedAttempt != r.AttemptID {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Agent activation step attempt is not fixed by the Start request")
	}
	if r.Step == AgentActivationPreflightV1 {
		if r.PredecessorResultDigest != "" {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation Preflight cannot have a predecessor")
		}
	} else if r.PredecessorResultDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidDigest, "Agent activation step predecessor result is required")
	}
	digest, err := AgentActivationStepRequestDigestV1(r)
	if err != nil || digest != r.RequestDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation step request digest drifted")
	}
	return nil
}

func DeriveAgentActivationStepAttemptIDV1(activationID string, startDigest core.Digest, step AgentActivationStepV1) (string, error) {
	if !validAgentLifecycleIDV1(activationID) || startDigest.Validate() != nil || step.Validate() != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation step attempt inputs are incomplete")
	}
	digest, err := core.CanonicalJSONDigest("praxis.application.agent-activation-coordination", AgentActivationCoordinationContractVersionV1, "AgentActivationStepAttemptIDV1", struct {
		ActivationID       string                `json:"activation_id"`
		StartRequestDigest core.Digest           `json:"start_request_digest"`
		Step               AgentActivationStepV1 `json:"step"`
	}{activationID, startDigest, step})
	if err != nil {
		return "", err
	}
	return "activation-step-" + strings.TrimPrefix(string(digest), "sha256:")[:32], nil
}

func AgentActivationStepRequestDigestV1(r AgentActivationStepRequestV1) (core.Digest, error) {
	r.ContractVersion = AgentActivationCoordinationContractVersionV1
	r.RequestDigest = ""
	return core.CanonicalJSONDigest("praxis.application.agent-activation-coordination", AgentActivationCoordinationContractVersionV1, "AgentActivationStepRequestV1", r)
}

func SealAgentActivationStepRequestV1(r AgentActivationStepRequestV1) (AgentActivationStepRequestV1, error) {
	r.ContractVersion = AgentActivationCoordinationContractVersionV1
	attempt, err := DeriveAgentActivationStepAttemptIDV1(r.ActivationID, r.StartRequestDigest, r.Step)
	if err != nil {
		return AgentActivationStepRequestV1{}, err
	}
	if r.AttemptID != "" && r.AttemptID != attempt {
		return AgentActivationStepRequestV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation step supplied another attempt")
	}
	r.AttemptID = attempt
	provided := r.RequestDigest
	r.RequestDigest = ""
	digest, err := AgentActivationStepRequestDigestV1(r)
	if err != nil {
		return AgentActivationStepRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationStepRequestV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent activation step request supplied a wrong digest")
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

type AgentActivationStepResultV1 struct {
	ContractVersion       string                          `json:"contract_version"`
	ActivationID          string                          `json:"activation_id"`
	Step                  AgentActivationStepV1           `json:"step"`
	AttemptID             string                          `json:"attempt_id"`
	RequestDigest         core.Digest                     `json:"request_digest"`
	Current               runtimeports.OwnerCurrentRefV1  `json:"current"`
	SandboxState          string                          `json:"sandbox_state,omitempty"`
	SandboxLease          *core.SandboxLeaseRef           `json:"sandbox_lease,omitempty"`
	SandboxLeaseCurrent   *runtimeports.OwnerCurrentRefV1 `json:"sandbox_lease_current,omitempty"`
	ExecutionScope        *core.ExecutionScope            `json:"execution_scope,omitempty"`
	ActivationCurrent     *runtimeports.OwnerCurrentRefV1 `json:"activation_current,omitempty"`
	SandboxActiveCurrent  *runtimeports.OwnerCurrentRefV1 `json:"sandbox_active_current,omitempty"`
	ExecutionReadyCurrent *runtimeports.OwnerCurrentRefV1 `json:"execution_ready_current,omitempty"`
	CheckedUnixNano       int64                           `json:"checked_unix_nano"`
	ExpiresUnixNano       int64                           `json:"expires_unix_nano"`
	ResultDigest          core.Digest                     `json:"result_digest"`
}

func (r AgentActivationStepResultV1) ValidateFor(request AgentActivationStepRequestV1, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if r.ContractVersion != AgentActivationCoordinationContractVersionV1 || r.ActivationID != request.ActivationID || r.Step != request.Step || r.AttemptID != request.AttemptID || r.RequestDigest != request.RequestDigest || r.Current.Validate() != nil || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano || r.ExpiresUnixNano > request.RequestedNotAfterUnixNano || r.ExpiresUnixNano > r.Current.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation step result does not bind its exact live request")
	}
	if now.IsZero() || now.UnixNano() < r.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Agent activation step result clock regressed")
	}
	if !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent activation step result expired")
	}
	if err := r.validateStepPayloadV1(); err != nil {
		return err
	}
	digest, err := AgentActivationStepResultDigestV1(r)
	if err != nil || digest != r.ResultDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation step result digest drifted")
	}
	return nil
}

func (r AgentActivationStepResultV1) validateStepPayloadV1() error {
	switch r.Step {
	case AgentActivationSandboxAllocateV1:
		if r.SandboxState != AgentActivationSandboxReservedQuarantinedV1 || r.SandboxLease == nil || r.SandboxLeaseCurrent == nil || r.SandboxLease.Validate() != nil || r.SandboxLeaseCurrent.Validate() != nil || r.ExecutionScope != nil || r.ActivationCurrent != nil || r.SandboxActiveCurrent != nil || r.ExecutionReadyCurrent != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Sandbox Allocate must return only reserved_quarantined lease current")
		}
	case AgentActivationCommitV1:
		if r.SandboxState != "" || r.SandboxLease == nil || r.ExecutionScope == nil || r.ActivationCurrent == nil || r.SandboxLease.Validate() != nil || r.ExecutionScope.Validate() != nil || r.ActivationCurrent.Validate() != nil || r.ExecutionScope.SandboxLease == nil || *r.ExecutionScope.SandboxLease != *r.SandboxLease || r.SandboxLeaseCurrent != nil || r.SandboxActiveCurrent != nil || r.ExecutionReadyCurrent != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "ActivationCommit must return the committed ExecutionScope and Activation current")
		}
	case AgentActivationSandboxActivateV1:
		if r.SandboxState != "active" || r.SandboxLease == nil || r.SandboxActiveCurrent == nil || r.SandboxLease.Validate() != nil || r.SandboxActiveCurrent.Validate() != nil || r.SandboxLeaseCurrent != nil || r.ExecutionScope != nil || r.ActivationCurrent != nil || r.ExecutionReadyCurrent != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Sandbox Activate must return only active current for the committed lease")
		}
	case AgentActivationReadyInspectV1:
		if r.ExecutionReadyCurrent == nil || r.ExecutionReadyCurrent.Validate() != nil || r.SandboxState != "" || r.SandboxLease != nil || r.SandboxLeaseCurrent != nil || r.ExecutionScope != nil || r.ActivationCurrent != nil || r.SandboxActiveCurrent != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Ready Inspect must return only independent Execution ready current")
		}
	default:
		if r.SandboxState != "" || r.SandboxLease != nil || r.SandboxLeaseCurrent != nil || r.ExecutionScope != nil || r.ActivationCurrent != nil || r.SandboxActiveCurrent != nil || r.ExecutionReadyCurrent != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Agent activation step returned payload owned by another step")
		}
	}
	for _, ref := range []*runtimeports.OwnerCurrentRefV1{r.SandboxLeaseCurrent, r.ActivationCurrent, r.SandboxActiveCurrent, r.ExecutionReadyCurrent} {
		if ref != nil && r.ExpiresUnixNano > ref.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent activation step TTL exceeds a returned current")
		}
	}
	return nil
}

func AgentActivationStepResultDigestV1(r AgentActivationStepResultV1) (core.Digest, error) {
	r.ContractVersion = AgentActivationCoordinationContractVersionV1
	r.ResultDigest = ""
	return core.CanonicalJSONDigest("praxis.application.agent-activation-coordination", AgentActivationCoordinationContractVersionV1, "AgentActivationStepResultV1", r)
}

func SealAgentActivationStepResultV1(r AgentActivationStepResultV1) (AgentActivationStepResultV1, error) {
	r.ContractVersion = AgentActivationCoordinationContractVersionV1
	provided := r.ResultDigest
	r.ResultDigest = ""
	digest, err := AgentActivationStepResultDigestV1(r)
	if err != nil {
		return AgentActivationStepResultV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationStepResultV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent activation step result supplied a wrong digest")
	}
	r.ResultDigest = digest
	return r, nil
}

type AgentActivationStepEventStateV1 string

const (
	AgentActivationStepIntentRecordedV1     AgentActivationStepEventStateV1 = "intent_recorded"
	AgentActivationStepInvocationRecordedV1 AgentActivationStepEventStateV1 = "invocation_recorded"
	AgentActivationStepOutcomeUnknownV1     AgentActivationStepEventStateV1 = "outcome_unknown"
	AgentActivationStepResultRecordedV1     AgentActivationStepEventStateV1 = "result_recorded"
)

type AgentActivationStepEventV1 struct {
	Sequence         uint32                          `json:"sequence"`
	Step             AgentActivationStepV1           `json:"step"`
	State            AgentActivationStepEventStateV1 `json:"state"`
	AttemptID        string                          `json:"attempt_id"`
	RequestDigest    core.Digest                     `json:"request_digest"`
	Result           *AgentActivationStepResultV1    `json:"result,omitempty"`
	RecordedUnixNano int64                           `json:"recorded_unix_nano"`
	Digest           core.Digest                     `json:"digest"`
}

func (e AgentActivationStepEventV1) Validate() error {
	if e.Sequence == 0 || e.Step.Validate() != nil || !validAgentLifecycleIDV1(e.AttemptID) || e.RequestDigest.Validate() != nil || e.RecordedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation step event is incomplete")
	}
	switch e.State {
	case AgentActivationStepIntentRecordedV1, AgentActivationStepInvocationRecordedV1, AgentActivationStepOutcomeUnknownV1:
		if e.Result != nil {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "non-result Agent activation event carried a result")
		}
	case AgentActivationStepResultRecordedV1:
		if e.Result == nil || e.Result.Step != e.Step || e.Result.AttemptID != e.AttemptID || e.Result.RequestDigest != e.RequestDigest {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation result event drifted from its attempt")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Agent activation step event state is invalid")
	}
	digest, err := AgentActivationStepEventDigestV1(e)
	if err != nil || digest != e.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation step event digest drifted")
	}
	return nil
}

func AgentActivationStepEventDigestV1(e AgentActivationStepEventV1) (core.Digest, error) {
	e.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.agent-activation-coordination", AgentActivationCoordinationContractVersionV1, "AgentActivationStepEventV1", e)
}

func SealAgentActivationStepEventV1(e AgentActivationStepEventV1) (AgentActivationStepEventV1, error) {
	provided := e.Digest
	e.Digest = ""
	digest, err := AgentActivationStepEventDigestV1(e)
	if err != nil {
		return AgentActivationStepEventV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationStepEventV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent activation step event supplied a wrong digest")
	}
	e.Digest = digest
	return e, e.Validate()
}

type AgentActivationCoordinationFactV1 struct {
	ContractVersion string                        `json:"contract_version"`
	ActivationID    string                        `json:"activation_id"`
	Revision        core.Revision                 `json:"revision"`
	Request         AgentActivationStartRequestV1 `json:"request"`
	Events          []AgentActivationStepEventV1  `json:"events"`
	Result          *AgentActivationResultV1      `json:"result,omitempty"`
	Digest          core.Digest                   `json:"digest"`
}

func (f AgentActivationCoordinationFactV1) Validate() error {
	if f.ContractVersion != AgentActivationCoordinationContractVersionV1 || f.ActivationID != f.Request.ActivationID || f.Request.Validate() != nil || f.Revision == 0 || int(f.Revision) != len(f.Events) || len(f.Events) == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent activation coordination Fact is incomplete")
	}
	stepIndex := 0
	state := AgentActivationStepEventStateV1("")
	predecessor := core.Digest("")
	var lastRecorded int64
	for index, event := range f.Events {
		if err := event.Validate(); err != nil {
			return err
		}
		if event.Sequence != uint32(index+1) || stepIndex >= len(agentActivationStepOrderV1) || event.Step != agentActivationStepOrderV1[stepIndex] || event.RecordedUnixNano < lastRecorded {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation event order, step or clock drifted")
		}
		stepRequest, err := SealAgentActivationStepRequestV1(AgentActivationStepRequestV1{
			ActivationID: f.ActivationID, StartRequestDigest: f.Request.RequestDigest, Step: event.Step,
			PredecessorResultDigest: predecessor, RequestedNotAfterUnixNano: f.Request.RequestedNotAfterUnixNano,
		})
		if err != nil || event.AttemptID != stepRequest.AttemptID || event.RequestDigest != stepRequest.RequestDigest {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation event does not bind its fixed step request")
		}
		lastRecorded = event.RecordedUnixNano
		switch state {
		case "":
			if event.State != AgentActivationStepIntentRecordedV1 {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Agent activation step must begin with persisted intent")
			}
		case AgentActivationStepIntentRecordedV1:
			if event.State != AgentActivationStepInvocationRecordedV1 {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Agent activation intent must persist invocation before Owner call")
			}
		case AgentActivationStepInvocationRecordedV1:
			if event.State != AgentActivationStepOutcomeUnknownV1 && event.State != AgentActivationStepResultRecordedV1 {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Agent activation invocation has an invalid successor")
			}
		case AgentActivationStepOutcomeUnknownV1:
			if event.State != AgentActivationStepResultRecordedV1 {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "unknown Agent activation outcome may only resolve by exact Inspect")
			}
		}
		state = event.State
		if event.State == AgentActivationStepResultRecordedV1 {
			if err := event.Result.ValidateFor(stepRequest, time.Unix(0, event.RecordedUnixNano)); err != nil {
				return err
			}
			predecessor = event.Result.ResultDigest
			stepIndex++
			state = ""
		}
	}
	complete := stepIndex == len(agentActivationStepOrderV1) && state == ""
	if complete != (f.Result != nil) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation coordination completion and result disagree")
	}
	if f.Result != nil && (f.Result.ActivationID != f.ActivationID || f.Result.RequestDigest != f.Request.RequestDigest || f.Result.Validate() != nil) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation coordination result drifted")
	}
	digest, err := AgentActivationCoordinationFactDigestV1(f)
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "Agent activation coordination Fact digest drifted")
	}
	return nil
}

func AgentActivationCoordinationFactDigestV1(f AgentActivationCoordinationFactV1) (core.Digest, error) {
	f.ContractVersion = AgentActivationCoordinationContractVersionV1
	f.Digest = ""
	if f.Events == nil {
		f.Events = []AgentActivationStepEventV1{}
	}
	return core.CanonicalJSONDigest("praxis.application.agent-activation-coordination", AgentActivationCoordinationContractVersionV1, "AgentActivationCoordinationFactV1", f)
}

func SealAgentActivationCoordinationFactV1(f AgentActivationCoordinationFactV1) (AgentActivationCoordinationFactV1, error) {
	f.ContractVersion = AgentActivationCoordinationContractVersionV1
	f.Events = append([]AgentActivationStepEventV1{}, f.Events...)
	provided := f.Digest
	f.Digest = ""
	digest, err := AgentActivationCoordinationFactDigestV1(f)
	if err != nil {
		return AgentActivationCoordinationFactV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentActivationCoordinationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent activation coordination Fact supplied a wrong digest")
	}
	f.Digest = digest
	return f, f.Validate()
}

func ValidateAgentActivationCoordinationTransitionV1(current, next AgentActivationCoordinationFactV1) error {
	if current.Validate() != nil || next.Validate() != nil || current.ActivationID != next.ActivationID || current.Request.RequestDigest != next.Request.RequestDigest || next.Revision != current.Revision+1 || len(next.Events) != len(current.Events)+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Agent activation coordination successor coordinates drifted")
	}
	for index := range current.Events {
		if current.Events[index].Digest != next.Events[index].Digest {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Agent activation coordination history is not append-only")
		}
	}
	if current.Result != nil {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "completed Agent activation coordination is immutable")
	}
	return nil
}

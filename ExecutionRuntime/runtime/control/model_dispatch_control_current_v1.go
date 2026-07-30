package control

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const ModelDispatchControlCurrentContractVersionV1 = "1.0.0"

type ModelDispatchControlStateV1 string

const (
	ModelDispatchControlDispatchableV1    ModelDispatchControlStateV1 = "dispatchable"
	ModelDispatchControlCancelRequestedV1 ModelDispatchControlStateV1 = "cancel_requested"
	ModelDispatchControlFencedV1          ModelDispatchControlStateV1 = "fenced"
	ModelDispatchControlRevokedV1         ModelDispatchControlStateV1 = "revoked"
	ModelDispatchControlIndeterminateV1   ModelDispatchControlStateV1 = "indeterminate"
)

// ModelDispatchControlCurrentProjectionV1 is Runtime-owned current truth for
// Run cancel/fence/revoke and desired-state watermarks. It is intentionally not
// derived from Context cancellation.
type ModelDispatchControlCurrentProjectionV1 struct {
	ContractVersion      string                      `json:"contract_version"`
	OperationDigest      core.Digest                 `json:"operation_digest"`
	EffectID             core.EffectIntentID         `json:"effect_id"`
	RunID                core.AgentRunID             `json:"run_id"`
	ExecutionScopeDigest core.Digest                 `json:"execution_scope_digest"`
	RunRevision          core.Revision               `json:"run_revision"`
	DesiredStateRevision core.Revision               `json:"desired_state_revision"`
	LastCommandID        string                      `json:"last_command_id"`
	State                ModelDispatchControlStateV1 `json:"state"`
	WatermarkDigest      core.Digest                 `json:"watermark_digest"`
	CheckedUnixNano      int64                       `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                       `json:"expires_unix_nano"`
	ProjectionDigest     core.Digest                 `json:"projection_digest"`
}

func (p ModelDispatchControlCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ModelDispatchControlCurrentContractVersionV1 || p.EffectID == "" || p.RunID == "" || p.RunRevision == 0 || p.DesiredStateRevision == 0 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model dispatch control current projection identity is incomplete")
	}
	switch p.State {
	case ModelDispatchControlDispatchableV1, ModelDispatchControlCancelRequestedV1, ModelDispatchControlFencedV1, ModelDispatchControlRevokedV1, ModelDispatchControlIndeterminateV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Model dispatch control state is invalid")
	}
	for _, digest := range []core.Digest{p.OperationDigest, p.ExecutionScopeDigest, p.WatermarkDigest, p.ProjectionDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	expected, err := p.DigestV1()
	if err != nil || expected != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Model dispatch control current projection digest drifted")
	}
	return nil
}

func (p ModelDispatchControlCurrentProjectionV1) ValidateCurrent(operation ports.OperationSubjectV3, effectID core.EffectIntentID, run core.AgentRunRecord, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		return err
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(run.Scope)
	if err != nil {
		return err
	}
	if p.State != ModelDispatchControlDispatchableV1 || p.OperationDigest != operationDigest || p.EffectID != effectID || p.RunID != run.ID || p.RunRevision != run.Revision || p.ExecutionScopeDigest != scopeDigest || now.IsZero() || p.CheckedUnixNano > now.UnixNano() || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Model dispatch is cancelled, fenced, revoked, stale or indeterminate")
	}
	return nil
}

func (p ModelDispatchControlCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.model-dispatch-control-current", ModelDispatchControlCurrentContractVersionV1, "ModelDispatchControlCurrentProjectionV1", copy)
}

func SealModelDispatchControlCurrentProjectionV1(p ModelDispatchControlCurrentProjectionV1) (ModelDispatchControlCurrentProjectionV1, error) {
	p.ContractVersion = ModelDispatchControlCurrentContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return ModelDispatchControlCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

type ModelDispatchControlCurrentReaderV1 interface {
	InspectModelDispatchControlCurrentV1(context.Context, ports.OperationSubjectV3, core.EffectIntentID) (ModelDispatchControlCurrentProjectionV1, error)
}

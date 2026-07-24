package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	AgentExecutionAvailabilityContractVersionV1 = "praxis.runtime.agent-execution-availability/v1"
	agentExecutionAvailabilityCanonicalDomainV1 = "praxis.runtime.agent-execution-availability"
)

// AgentExecutionAvailabilityStateV1 is deliberately smaller than Host state.
// Runtime only needs to know whether an exact Host-owned epoch admits new work.
type AgentExecutionAvailabilityStateV1 string

const (
	AgentExecutionAvailabilityReadyV1  AgentExecutionAvailabilityStateV1 = "ready"
	AgentExecutionAvailabilityFencedV1 AgentExecutionAvailabilityStateV1 = "fenced"
)

// OwnerCurrentRefV1 is a neutral exact coordinate. Runtime owns this nominal
// carrier, not the referenced Owner fact or its current pointer.
type OwnerCurrentRefV1 struct {
	Owner           core.OwnerRef `json:"owner"`
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r OwnerCurrentRefV1) Validate() error {
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.ContractVersion) == "" || r.ContractVersion != strings.TrimSpace(r.ContractVersion) || len(r.ContractVersion) > 160 || invalidH4IDV1(r.ID) || r.Revision == 0 || r.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Owner current exact Ref is incomplete")
	}
	return r.Digest.Validate()
}

type AgentExecutionAvailabilityRefV1 struct {
	Owner           core.OwnerRef `json:"owner"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Epoch           core.Epoch    `json:"epoch"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r AgentExecutionAvailabilityRefV1) Validate() error {
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	if invalidH4IDV1(r.ID) || r.Revision == 0 || r.Epoch == 0 || r.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent execution availability Ref is incomplete")
	}
	return r.Digest.Validate()
}

// AgentExecutionAvailabilityProjectionV1 is Host-owned current truth exposed
// through a Runtime-neutral type so Runtime never imports Host.
type AgentExecutionAvailabilityProjectionV1 struct {
	ContractVersion  string                            `json:"contract_version"`
	Ref              AgentExecutionAvailabilityRefV1   `json:"ref"`
	HostID           string                            `json:"host_id"`
	StartID          string                            `json:"start_id"`
	SystemReady      OwnerCurrentRefV1                 `json:"system_ready_current"`
	State            AgentExecutionAvailabilityStateV1 `json:"state"`
	CheckedUnixNano  int64                             `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                             `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                       `json:"projection_digest"`
}

func (p AgentExecutionAvailabilityProjectionV1) Validate() error {
	if p.ContractVersion != AgentExecutionAvailabilityContractVersionV1 || invalidH4IDV1(p.HostID) || invalidH4IDV1(p.StartID) || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent execution availability projection is incomplete")
	}
	if p.State != AgentExecutionAvailabilityReadyV1 && p.State != AgentExecutionAvailabilityFencedV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Agent execution availability state is invalid")
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	if err := p.SystemReady.Validate(); err != nil {
		return err
	}
	if p.Ref.ExpiresUnixNano != p.ExpiresUnixNano || (p.State == AgentExecutionAvailabilityReadyV1 && p.ExpiresUnixNano > p.SystemReady.ExpiresUnixNano) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Agent execution availability exceeds its exact current inputs")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest || digest != p.Ref.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent execution availability digest drifted")
	}
	return nil
}

func (p AgentExecutionAvailabilityProjectionV1) ValidateCurrent(expected AgentExecutionAvailabilityRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if p.Ref != expected {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent execution availability exact Ref drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Agent execution availability clock regressed")
	}
	if p.State != AgentExecutionAvailabilityReadyV1 || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonFencedInstance, "Agent execution availability is fenced or expired")
	}
	return nil
}

func (p AgentExecutionAvailabilityProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.Ref.Digest = ""
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest(agentExecutionAvailabilityCanonicalDomainV1, AgentExecutionAvailabilityContractVersionV1, "AgentExecutionAvailabilityProjectionV1", copy)
}

func SealAgentExecutionAvailabilityProjectionV1(p AgentExecutionAvailabilityProjectionV1) (AgentExecutionAvailabilityProjectionV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != AgentExecutionAvailabilityContractVersionV1 {
		return AgentExecutionAvailabilityProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent execution availability contract version drifted")
	}
	p.ContractVersion = AgentExecutionAvailabilityContractVersionV1
	p.Ref.ExpiresUnixNano = p.ExpiresUnixNano
	providedRefDigest := p.Ref.Digest
	providedProjectionDigest := p.ProjectionDigest
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return AgentExecutionAvailabilityProjectionV1{}, err
	}
	if (providedRefDigest != "" && providedRefDigest != digest) || (providedProjectionDigest != "" && providedProjectionDigest != digest) {
		return AgentExecutionAvailabilityProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Agent execution availability supplied a wrong digest")
	}
	p.Ref.Digest = digest
	p.ProjectionDigest = digest
	return p, p.Validate()
}

// ValidateAgentExecutionAvailabilityTransitionV1 freezes the epoch fence.
// A ready epoch may renew or fence only at the same epoch. A fenced Start is
// terminal and cannot be reopened by a later projection.
func ValidateAgentExecutionAvailabilityTransitionV1(current, next AgentExecutionAvailabilityProjectionV1) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.Ref.Owner != next.Ref.Owner || current.Ref.ID != next.Ref.ID || current.HostID != next.HostID || current.StartID != next.StartID || current.Ref.Revision+1 != next.Ref.Revision {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Agent execution availability successor coordinates drifted")
	}
	if next.CheckedUnixNano < current.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Agent execution availability successor clock regressed")
	}
	if current.State == AgentExecutionAvailabilityFencedV1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonFencedInstance, "fenced Agent execution availability is terminal")
	}
	switch next.State {
	case AgentExecutionAvailabilityFencedV1:
		if next.Ref.Epoch != current.Ref.Epoch || next.SystemReady != current.SystemReady {
			return core.NewError(core.ErrorConflict, core.ReasonStaleInstanceEpoch, "fence must close the current availability epoch")
		}
	case AgentExecutionAvailabilityReadyV1:
		if next.Ref.Epoch != current.Ref.Epoch || next.ExpiresUnixNano <= next.CheckedUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonStaleInstanceEpoch, "ready renewal must preserve the live availability epoch")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Agent execution availability successor state is invalid")
	}
	return nil
}

type AgentExecutionAvailabilityCurrentReaderV1 interface {
	InspectAgentExecutionAvailabilityCurrentV1(context.Context, AgentExecutionAvailabilityRefV1) (AgentExecutionAvailabilityProjectionV1, error)
}

func invalidH4IDV1(value string) bool {
	return strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || len(value) > 160
}

package core

import "strings"

type (
	TenantID          string
	AgentIdentityID   string
	InstanceLineageID string
	AgentInstanceID   string
	SandboxLeaseID    string
	AgentRunID        string
	EffectIntentID    string
	OwnerID           string
	Epoch             uint64
	Revision          uint64
)

type AgentIdentityRef struct {
	TenantID TenantID        `json:"tenant_id"`
	ID       AgentIdentityID `json:"agent_identity_id"`
	Epoch    Epoch           `json:"identity_epoch"`
}

func (r AgentIdentityRef) Validate() error {
	if blank(string(r.TenantID)) || blank(string(r.ID)) || r.Epoch == 0 {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "identity requires tenant, id and non-zero epoch")
	}
	return nil
}

type LineageRef struct {
	ID         InstanceLineageID `json:"instance_lineage_id"`
	PlanDigest Digest            `json:"resolved_plan_digest"`
}

func (r LineageRef) Validate() error {
	if blank(string(r.ID)) {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "lineage id is required")
	}
	return r.PlanDigest.Validate()
}

type InstanceRef struct {
	ID    AgentInstanceID `json:"instance_id"`
	Epoch Epoch           `json:"instance_epoch"`
}

func (r InstanceRef) Validate() error {
	if blank(string(r.ID)) || r.Epoch == 0 {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "instance requires id and non-zero epoch")
	}
	return nil
}

type SandboxLeaseRef struct {
	ID    SandboxLeaseID `json:"sandbox_lease_id"`
	Epoch Epoch          `json:"lease_epoch"`
}

func (r SandboxLeaseRef) Validate() error {
	if blank(string(r.ID)) || r.Epoch == 0 {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "sandbox lease requires id and non-zero epoch")
	}
	return nil
}

type ExecutionScope struct {
	Identity       AgentIdentityRef `json:"identity"`
	Lineage        LineageRef       `json:"lineage"`
	Instance       InstanceRef      `json:"instance"`
	SandboxLease   *SandboxLeaseRef `json:"sandbox_lease,omitempty"`
	AuthorityEpoch Epoch            `json:"authority_epoch"`
}

func (s ExecutionScope) Validate() error {
	if err := s.Identity.Validate(); err != nil {
		return err
	}
	if err := s.Lineage.Validate(); err != nil {
		return err
	}
	if err := s.Instance.Validate(); err != nil {
		return err
	}
	if s.SandboxLease != nil {
		if err := s.SandboxLease.Validate(); err != nil {
			return err
		}
	}
	if s.AuthorityEpoch == 0 {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "authority epoch is required")
	}
	return nil
}

type OwnerRef struct {
	Domain string  `json:"domain"`
	ID     OwnerID `json:"id"`
}

func (r OwnerRef) Validate() error {
	if blank(r.Domain) || blank(string(r.ID)) {
		return NewError(ErrorInvalidArgument, ReasonOwnerMissing, "owner domain and id are required")
	}
	return nil
}

// EffectOwnership makes both authority points explicit without transferring
// either domain meaning to Runtime. The two owners may be the same service.
type EffectOwnership struct {
	IntentOwner     OwnerRef `json:"intent_owner"`
	SettlementOwner OwnerRef `json:"settlement_owner"`
}

func (o EffectOwnership) Validate() error {
	if err := o.IntentOwner.Validate(); err != nil {
		return err
	}
	return o.SettlementOwner.Validate()
}

func blank(value string) bool { return strings.TrimSpace(value) == "" }

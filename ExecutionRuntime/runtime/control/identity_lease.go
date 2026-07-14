package control

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type IdentityLeaseState string

const (
	IdentityLeaseReserved IdentityLeaseState = "reserved"
	IdentityLeaseActive   IdentityLeaseState = "active"
	IdentityLeaseRevoked  IdentityLeaseState = "revoked"
	IdentityLeaseExpired  IdentityLeaseState = "expired"
	IdentityLeaseReleased IdentityLeaseState = "released"
)

type IdentityExecutionLease struct {
	ID                  string                `json:"identity_execution_lease_id"`
	Identity            core.AgentIdentityRef `json:"identity"`
	Lineage             core.LineageRef       `json:"lineage"`
	ActivationAttemptID string                `json:"activation_attempt_id"`
	State               IdentityLeaseState    `json:"state"`
	AuthorityEpoch      core.Epoch            `json:"authority_epoch"`
	ExpiresAt           time.Time             `json:"expires_at"`
	Revision            core.Revision         `json:"revision"`
}

type ReserveIdentityLeaseRequest struct {
	TenantID              core.TenantID        `json:"tenant_id"`
	IdentityID            core.AgentIdentityID `json:"agent_identity_id"`
	ExpectedIdentityEpoch core.Epoch           `json:"expected_identity_epoch"`
	Lineage               core.LineageRef      `json:"lineage"`
	ActivationAttemptID   string               `json:"activation_attempt_id"`
	AuthorityEpoch        core.Epoch           `json:"authority_epoch"`
	ExpiresAt             time.Time            `json:"expires_at"`
}

func (r ReserveIdentityLeaseRequest) Validate(now time.Time) error {
	if strings.TrimSpace(string(r.TenantID)) == "" || strings.TrimSpace(string(r.IdentityID)) == "" || strings.TrimSpace(r.ActivationAttemptID) == "" || r.AuthorityEpoch == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "identity, activation attempt and authority epoch are required")
	}
	if err := r.Lineage.Validate(); err != nil {
		return err
	}
	if now.IsZero() || !r.ExpiresAt.After(now) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "lease expiry must be in the future")
	}
	return nil
}

type RenewIdentityLeaseRequest struct {
	LeaseID          string        `json:"lease_id"`
	ExpectedRevision core.Revision `json:"expected_revision"`
	AuthorityEpoch   core.Epoch    `json:"authority_epoch"`
	ExpiresAt        time.Time     `json:"expires_at"`
}

type EndIdentityLeaseRequest struct {
	LeaseID          string        `json:"lease_id"`
	ExpectedRevision core.Revision `json:"expected_revision"`
	Reason           string        `json:"reason"`
}

// IdentityLeaseFactPort is the V1 single-holder authority for one
// AgentIdentity. Reserve is exclusive but grants only activation/cleanup;
// Activation is deliberately absent from this Port. General execution
// authority is granted only by admission.ActivationFactPort.CommitActivation,
// which atomically binds the Instance and advances this lease.
type IdentityLeaseFactPort interface {
	ReserveIdentityLease(context.Context, ReserveIdentityLeaseRequest) (IdentityExecutionLease, error)
	RenewIdentityLease(context.Context, RenewIdentityLeaseRequest) (IdentityExecutionLease, error)
	RevokeIdentityLease(context.Context, EndIdentityLeaseRequest) (IdentityExecutionLease, error)
	ReleaseIdentityLease(context.Context, EndIdentityLeaseRequest) (IdentityExecutionLease, error)
	InspectIdentityLease(context.Context, core.TenantID, core.AgentIdentityID) (IdentityExecutionLease, error)
}

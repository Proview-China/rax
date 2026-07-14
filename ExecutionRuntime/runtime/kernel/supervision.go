package kernel

import (
	"crypto/sha256"
	"encoding/binary"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type SupervisionMode string

const (
	SupervisionNormal      SupervisionMode = "normal"
	SupervisionBackoff     SupervisionMode = "backoff"
	SupervisionQuarantined SupervisionMode = "quarantined"
	SupervisionFenced      SupervisionMode = "fenced"
)

type SupervisionAction string

const (
	SupervisionNone         SupervisionAction = "none"
	SupervisionInspect      SupervisionAction = "inspect"
	SupervisionRenewLease   SupervisionAction = "renew_lease"
	SupervisionQuarantine   SupervisionAction = "quarantine"
	SupervisionFenceAndStop SupervisionAction = "fence_and_stop"
)

// SupervisionPolicy has no defaults. A deployment must explicitly choose and
// review every duration; Runtime does not smuggle a production SLA into code.
type SupervisionPolicy struct {
	InspectionInterval     time.Duration `json:"inspection_interval"`
	LeaseRenewLeadTime     time.Duration `json:"lease_renew_lead_time"`
	RetryBaseDelay         time.Duration `json:"retry_base_delay"`
	RetryMaxDelay          time.Duration `json:"retry_max_delay"`
	DeterministicSpread    time.Duration `json:"deterministic_spread"`
	MaxConsecutiveFailures uint32        `json:"max_consecutive_failures"`
}

func (p SupervisionPolicy) Validate() error {
	if p.InspectionInterval <= 0 || p.LeaseRenewLeadTime <= 0 || p.RetryBaseDelay <= 0 || p.RetryMaxDelay < p.RetryBaseDelay || p.MaxConsecutiveFailures == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSupervisionPolicyMissing, "all supervision timing and failure bounds must be explicitly configured")
	}
	if p.DeterministicSpread < 0 || p.DeterministicSpread > p.InspectionInterval {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSupervisionPolicyMissing, "deterministic spread must fit within the inspection interval")
	}
	return nil
}

// Digest binds a persisted supervision record to the exact reviewed policy.
// A policy change therefore becomes an explicit migration instead of silently
// changing the timing or failure budget of an already-running instance.
func (p SupervisionPolicy) Digest() (core.Digest, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	return core.DigestJSON(p)
}

type SupervisionRecord struct {
	Scope                   core.ExecutionScope `json:"scope"`
	PolicyDigest            core.Digest         `json:"policy_digest"`
	Mode                    SupervisionMode     `json:"mode"`
	ConsecutiveFailures     uint32              `json:"consecutive_failures"`
	LastSuccessfulInspectAt time.Time           `json:"last_successful_inspect_at"`
	NextActionAt            time.Time           `json:"next_action_at"`
	IdentityLeaseExpiresAt  time.Time           `json:"identity_lease_expires_at"`
	UpdatedAt               time.Time           `json:"updated_at"`
	Revision                core.Revision       `json:"revision"`
}

func NewSupervisionRecord(scope core.ExecutionScope, policyDigest core.Digest, leaseExpiresAt, now time.Time, policy SupervisionPolicy) (SupervisionRecord, error) {
	if err := scope.Validate(); err != nil {
		return SupervisionRecord{}, err
	}
	if scope.SandboxLease == nil {
		return SupervisionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "supervision requires an active sandbox lease binding")
	}
	if err := policyDigest.Validate(); err != nil {
		return SupervisionRecord{}, err
	}
	if err := validateSupervisionPolicyBinding(policyDigest, policy); err != nil {
		return SupervisionRecord{}, err
	}
	if now.IsZero() || !leaseExpiresAt.After(now.Add(policy.LeaseRenewLeadTime)) {
		return SupervisionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonIdentityLeaseStateInvalid, "identity lease lifetime is too short for the configured renewal lead time")
	}
	record := SupervisionRecord{
		Scope: scope, PolicyDigest: policyDigest, Mode: SupervisionNormal,
		LastSuccessfulInspectAt: now, IdentityLeaseExpiresAt: leaseExpiresAt, UpdatedAt: now, Revision: 1,
	}
	record.NextActionAt = nextInspection(record, now, policy)
	return record, nil
}

func (r SupervisionRecord) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if r.Scope.SandboxLease == nil || r.Revision == 0 || r.LastSuccessfulInspectAt.IsZero() || r.NextActionAt.IsZero() || r.IdentityLeaseExpiresAt.IsZero() || r.UpdatedAt.IsZero() || r.UpdatedAt.Before(r.LastSuccessfulInspectAt) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "supervision record is incomplete")
	}
	if err := r.PolicyDigest.Validate(); err != nil {
		return err
	}
	switch r.Mode {
	case SupervisionNormal:
		if r.ConsecutiveFailures != 0 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "normal supervision cannot retain failure count")
		}
	case SupervisionBackoff:
		if r.ConsecutiveFailures == 0 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "backoff requires a failure count")
		}
	case SupervisionQuarantined, SupervisionFenced:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown supervision mode")
	}
	return nil
}

type SupervisionDecision struct {
	Action        SupervisionAction `json:"action"`
	AutomaticSafe bool              `json:"automatic_safe"`
	Reason        string            `json:"reason"`
}

// EvaluateSupervision is side-effect free. Lease expiry and renewal always
// dominate routine health inspection so a busy host cannot starve authority
// maintenance with lower-priority work.
func EvaluateSupervision(record SupervisionRecord, now time.Time, policy SupervisionPolicy) (SupervisionDecision, error) {
	if err := record.Validate(); err != nil {
		return SupervisionDecision{}, err
	}
	if err := validateSupervisionPolicyBinding(record.PolicyDigest, policy); err != nil {
		return SupervisionDecision{}, err
	}
	if now.IsZero() {
		return SupervisionDecision{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "current time is required")
	}
	if record.Mode == SupervisionFenced {
		return supervisionDecision(SupervisionNone, true, "instance is already fenced"), nil
	}
	if !now.Before(record.IdentityLeaseExpiresAt) {
		return supervisionDecision(SupervisionFenceAndStop, true, "identity execution lease has expired"), nil
	}
	if !now.Before(record.IdentityLeaseExpiresAt.Add(-policy.LeaseRenewLeadTime)) {
		return supervisionDecision(SupervisionRenewLease, true, "identity execution lease entered its explicit renewal window"), nil
	}
	if record.Mode == SupervisionQuarantined {
		if !now.Before(record.NextActionAt) {
			return supervisionDecision(SupervisionInspect, false, "quarantined state requires authoritative inspection"), nil
		}
		return supervisionDecision(SupervisionNone, true, "quarantine inspection is backoff-limited"), nil
	}
	if !now.Before(record.NextActionAt) {
		return supervisionDecision(SupervisionInspect, true, "scheduled health inspection is due"), nil
	}
	return supervisionDecision(SupervisionNone, true, "no supervision action is due"), nil
}

type SupervisionSignal string

const (
	SignalHealthy            SupervisionSignal = "healthy"
	SignalTransientFailure   SupervisionSignal = "transient_failure"
	SignalStateUnknown       SupervisionSignal = "state_unknown"
	SignalLeaseRenewed       SupervisionSignal = "lease_renewed"
	SignalLeaseRevisionStale SupervisionSignal = "lease_revision_stale"
	SignalAuthorityRevoked   SupervisionSignal = "authority_revoked"
	SignalLeaseExpired       SupervisionSignal = "lease_expired"
)

type SupervisionUpdate struct {
	Signal            SupervisionSignal `json:"signal"`
	ObservedAt        time.Time         `json:"observed_at"`
	NewLeaseExpiresAt time.Time         `json:"new_lease_expires_at,omitempty"`
}

// ApplySupervisionUpdate returns the next persistable record and the immediate
// control decision. Callers must persist the record with revision CAS before
// dispatching the decision.
func ApplySupervisionUpdate(record SupervisionRecord, update SupervisionUpdate, policy SupervisionPolicy) (SupervisionRecord, SupervisionDecision, error) {
	if err := record.Validate(); err != nil {
		return SupervisionRecord{}, SupervisionDecision{}, err
	}
	if err := validateSupervisionPolicyBinding(record.PolicyDigest, policy); err != nil {
		return SupervisionRecord{}, SupervisionDecision{}, err
	}
	if update.ObservedAt.IsZero() {
		return SupervisionRecord{}, SupervisionDecision{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "supervision observation time is required")
	}
	if update.ObservedAt.Before(record.UpdatedAt) {
		return SupervisionRecord{}, SupervisionDecision{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonLateSupervisionSignal, "late supervision signal cannot mutate the current record")
	}
	if record.Mode == SupervisionFenced && update.Signal != SignalAuthorityRevoked && update.Signal != SignalLeaseExpired {
		return SupervisionRecord{}, SupervisionDecision{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonFencedInstance, "fenced supervision cannot regain execution authority")
	}
	if !update.ObservedAt.Before(record.IdentityLeaseExpiresAt) && update.Signal != SignalAuthorityRevoked && update.Signal != SignalLeaseExpired {
		return SupervisionRecord{}, SupervisionDecision{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonIdentityLeaseStateInvalid, "expired identity lease cannot be revived by a late supervision result")
	}
	next := record
	next.Revision++
	next.UpdatedAt = update.ObservedAt

	switch update.Signal {
	case SignalHealthy:
		next.Mode = SupervisionNormal
		next.ConsecutiveFailures = 0
		next.LastSuccessfulInspectAt = update.ObservedAt
		next.NextActionAt = nextInspection(next, update.ObservedAt, policy)
		return next, supervisionDecision(SupervisionNone, true, "health evidence restored normal supervision"), nil
	case SignalTransientFailure:
		next.ConsecutiveFailures = saturatingIncrement(next.ConsecutiveFailures)
		next.NextActionAt = update.ObservedAt.Add(retryDelay(next, policy))
		if next.ConsecutiveFailures >= policy.MaxConsecutiveFailures {
			next.Mode = SupervisionQuarantined
			return next, supervisionDecision(SupervisionQuarantine, true, "bounded transient failure budget was exhausted"), nil
		}
		next.Mode = SupervisionBackoff
		return next, supervisionDecision(SupervisionNone, true, "transient failure entered bounded exponential backoff"), nil
	case SignalStateUnknown:
		next.Mode = SupervisionQuarantined
		next.ConsecutiveFailures = saturatingIncrement(next.ConsecutiveFailures)
		next.NextActionAt = update.ObservedAt.Add(retryDelay(next, policy))
		return next, supervisionDecision(SupervisionQuarantine, true, "component state is unknown and cannot advance lifecycle"), nil
	case SignalLeaseRenewed:
		if !update.NewLeaseExpiresAt.After(update.ObservedAt.Add(policy.LeaseRenewLeadTime)) {
			return SupervisionRecord{}, SupervisionDecision{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonIdentityLeaseStateInvalid, "renewed lease does not provide the configured safety window")
		}
		next.IdentityLeaseExpiresAt = update.NewLeaseExpiresAt
		next.NextActionAt = nextInspection(next, update.ObservedAt, policy)
		return next, supervisionDecision(SupervisionNone, true, "lease renewal was authoritatively confirmed"), nil
	case SignalLeaseRevisionStale:
		next.Mode = SupervisionQuarantined
		next.NextActionAt = update.ObservedAt
		return next, supervisionDecision(SupervisionInspect, false, "stale lease revision must inspect the linearized fact before retry"), nil
	case SignalAuthorityRevoked, SignalLeaseExpired:
		next.Mode = SupervisionFenced
		next.NextActionAt = update.ObservedAt
		return next, supervisionDecision(SupervisionFenceAndStop, true, "execution authority is no longer valid"), nil
	default:
		return SupervisionRecord{}, SupervisionDecision{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown supervision signal")
	}
}

func nextInspection(record SupervisionRecord, now time.Time, policy SupervisionPolicy) time.Time {
	return now.Add(policy.InspectionInterval + deterministicSpread(record, policy.DeterministicSpread))
}

func validateSupervisionPolicyBinding(want core.Digest, policy SupervisionPolicy) error {
	got, err := policy.Digest()
	if err != nil {
		return err
	}
	if got != want {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonSupervisionPolicyDrift, "supervision policy does not match the persisted policy digest")
	}
	return nil
}

func retryDelay(record SupervisionRecord, policy SupervisionPolicy) time.Duration {
	delay := policy.RetryBaseDelay
	for count := uint32(1); count < record.ConsecutiveFailures && delay < policy.RetryMaxDelay; count++ {
		if delay > policy.RetryMaxDelay/2 {
			delay = policy.RetryMaxDelay
			break
		}
		delay *= 2
	}
	if delay > policy.RetryMaxDelay {
		delay = policy.RetryMaxDelay
	}
	spread := deterministicSpread(record, policy.DeterministicSpread)
	if spread > policy.RetryMaxDelay-delay {
		return policy.RetryMaxDelay
	}
	return delay + spread
}

func deterministicSpread(record SupervisionRecord, window time.Duration) time.Duration {
	if window <= 0 {
		return 0
	}
	payload := string(record.Scope.Identity.TenantID) + "/" + string(record.Scope.Identity.ID) + "/" + string(record.Scope.Instance.ID) + "/" + string(record.PolicyDigest)
	digest := sha256.Sum256([]byte(payload))
	return time.Duration(binary.BigEndian.Uint64(digest[:8]) % uint64(window))
}

func saturatingIncrement(value uint32) uint32 {
	if value == ^uint32(0) {
		return value
	}
	return value + 1
}

func supervisionDecision(action SupervisionAction, automatic bool, reason string) SupervisionDecision {
	return SupervisionDecision{Action: action, AutomaticSafe: automatic, Reason: reason}
}

package contract

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type EffectKind string

const (
	EffectAllocate        EffectKind = "praxis.sandbox/allocate"
	EffectActivate        EffectKind = "praxis.sandbox/activate"
	EffectOpen            EffectKind = "praxis.sandbox/open"
	EffectCancel          EffectKind = "praxis.sandbox/cancel"
	EffectClose           EffectKind = "praxis.sandbox/close"
	EffectFence           EffectKind = "praxis.sandbox/fence"
	EffectRelease         EffectKind = "praxis.sandbox/release"
	EffectInspect         EffectKind = "praxis.sandbox/inspect"
	EffectCleanup         EffectKind = "praxis.sandbox/cleanup"
	EffectWorkspaceCommit EffectKind = "praxis.sandbox/workspace-commit"
)

func (k EffectKind) Validate() error {
	switch k {
	case EffectAllocate, EffectActivate, EffectOpen, EffectCancel, EffectClose, EffectFence, EffectRelease, EffectInspect, EffectCleanup, EffectWorkspaceCommit:
		return nil
	default:
		return fmt.Errorf("effect kind %q is unsupported by the Sandbox lifecycle contract", k)
	}
}

func (k EffectKind) ConflictDomain() string {
	switch k {
	case EffectCancel:
		return "praxis.sandbox/execution-control"
	case EffectInspect:
		return "praxis.sandbox/inspection"
	case EffectWorkspaceCommit:
		return "praxis.sandbox/workspace-commit"
	default:
		return "praxis.sandbox/environment-lifecycle"
	}
}

type RuntimeLeaseBinding struct {
	TenantID         string `json:"tenant_id"`
	InstanceID       string `json:"instance_id"`
	InstanceEpoch    uint64 `json:"instance_epoch"`
	LeaseID          string `json:"lease_id"`
	LeaseEpoch       uint64 `json:"lease_epoch"`
	FenceEpoch       uint64 `json:"fence_epoch"`
	ScopeDigest      string `json:"scope_digest"`
	ObservedRevision uint64 `json:"observed_revision"`
	ExpiresUnixNano  int64  `json:"expires_unix_nano"`
}

func (b RuntimeLeaseBinding) ValidateShape() error {
	if strings.TrimSpace(b.TenantID) == "" || strings.TrimSpace(b.InstanceID) == "" || strings.TrimSpace(b.LeaseID) == "" {
		return errors.New("tenant, instance, and lease ids are required")
	}
	if b.InstanceEpoch == 0 || b.LeaseEpoch == 0 || b.FenceEpoch == 0 || b.ObservedRevision == 0 {
		return errors.New("instance, lease, fence, and observed revisions must be positive")
	}
	if !ValidDigest(b.ScopeDigest) {
		return errors.New("runtime lease scope digest is invalid")
	}
	if b.ExpiresUnixNano <= 0 {
		return errors.New("runtime lease binding expiry is required")
	}
	return nil
}

func (b RuntimeLeaseBinding) ValidateCurrent(now time.Time) error {
	if err := b.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() >= b.ExpiresUnixNano {
		return errors.New("runtime lease binding is expired")
	}
	return nil
}

func SameRuntimeLeaseBinding(a, b RuntimeLeaseBinding) bool {
	return a == b
}

type DomainReservation struct {
	Meta                       Meta                `json:"meta"`
	State                      CurrentFactState    `json:"state"`
	OperationID                string              `json:"operation_id"`
	EffectID                   string              `json:"effect_id"`
	IntentRevision             uint64              `json:"intent_revision"`
	IntentDigest               string              `json:"intent_digest"`
	AttemptID                  string              `json:"attempt_id"`
	AttemptRef                 Ref                 `json:"attempt_ref"`
	Kind                       EffectKind          `json:"kind"`
	OperationSubjectDigest     string              `json:"operation_subject_digest"`
	ConflictDomain             string              `json:"conflict_domain"`
	ConflictScopeDigest        string              `json:"conflict_scope_digest"`
	Lease                      RuntimeLeaseBinding `json:"lease"`
	RuntimeLeaseBindingRef     Ref                 `json:"runtime_lease_binding_ref"`
	GenerationBindingRef       Ref                 `json:"generation_binding_association_ref"`
	RequirementRef             Ref                 `json:"requirement_ref"`
	PolicyRef                  Ref                 `json:"policy_ref"`
	PlacementRef               Ref                 `json:"placement_ref"`
	BackendRef                 Ref                 `json:"backend_ref"`
	SlotRef                    Ref                 `json:"slot_ref"`
	ProviderBinding            ProviderBindingRef  `json:"provider_binding"`
	ExpectedProjectionRevision uint64              `json:"expected_projection_revision"`
	RunID                      string              `json:"run_id,omitempty"`
}

func (r DomainReservation) ValidateShape() error {
	if err := r.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := r.State.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.OperationID) == "" || strings.TrimSpace(r.EffectID) == "" || strings.TrimSpace(r.AttemptID) == "" || r.IntentRevision == 0 || !ValidDigest(r.IntentDigest) {
		return errors.New("operation, effect, intent, and attempt coordinates are required")
	}
	if err := r.Kind.Validate(); err != nil {
		return err
	}
	if !ValidDigest(r.OperationSubjectDigest) {
		return errors.New("operation subject digest is invalid")
	}
	if r.ConflictDomain != r.Kind.ConflictDomain() {
		return fmt.Errorf("conflict domain %q does not match effect kind", r.ConflictDomain)
	}
	if !ValidDigest(r.ConflictScopeDigest) {
		return errors.New("tenant-stable conflict scope digest is required")
	}
	if err := r.Lease.ValidateShape(); err != nil {
		return err
	}
	for name, ref := range map[string]Ref{
		"attempt":               r.AttemptRef,
		"runtime lease binding": r.RuntimeLeaseBindingRef,
		"generation binding":    r.GenerationBindingRef,
		"requirement":           r.RequirementRef,
		"policy":                r.PolicyRef,
		"placement":             r.PlacementRef,
		"backend":               r.BackendRef,
		"slot":                  r.SlotRef,
	} {
		if err := ref.ValidateShape(name + " ref"); err != nil {
			return err
		}
	}
	if err := r.ProviderBinding.ValidateShape(); err != nil {
		return err
	}
	if r.ExpectedProjectionRevision == 0 {
		return errors.New("expected projection revision is required")
	}
	if effectRequiresRun(r.Kind) && strings.TrimSpace(r.RunID) == "" {
		return fmt.Errorf("effect %q requires an exact run id", r.Kind)
	}
	return nil
}

func (r DomainReservation) ValidateCurrent(now time.Time) error {
	if err := r.ValidateShape(); err != nil {
		return err
	}
	if err := r.Meta.ValidateCurrent(now); err != nil {
		return err
	}
	if r.State != CurrentFactActive {
		return errors.New("domain reservation is not active")
	}
	return r.Lease.ValidateCurrent(now)
}

func effectRequiresRun(kind EffectKind) bool {
	return kind == EffectCancel || kind == EffectWorkspaceCommit
}

type Observation struct {
	Meta                 Meta   `json:"meta"`
	ReservationRef       Ref    `json:"reservation_ref"`
	OperationID          string `json:"operation_id"`
	AttemptID            string `json:"attempt_id"`
	SourceRegistrationID string `json:"source_registration_id"`
	SourceEpoch          uint64 `json:"source_epoch"`
	SourceSequence       uint64 `json:"source_sequence"`
	PayloadDigest        string `json:"payload_digest"`
	ReceiptRef           Ref    `json:"receipt_ref"`
	EvidenceRefs         []Ref  `json:"evidence_refs"`
	ObservedState        string `json:"observed_state"`
}

func (o Observation) ValidateShape() error {
	if err := o.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := o.ReservationRef.ValidateShape("reservation ref"); err != nil {
		return err
	}
	if strings.TrimSpace(o.OperationID) == "" || strings.TrimSpace(o.AttemptID) == "" || strings.TrimSpace(o.SourceRegistrationID) == "" {
		return errors.New("operation, attempt, and source registration ids are required")
	}
	if o.SourceEpoch == 0 || o.SourceSequence == 0 || !ValidDigest(o.PayloadDigest) {
		return errors.New("source epoch, sequence, and payload digest are required")
	}
	if err := o.ReceiptRef.ValidateShape("receipt ref"); err != nil {
		return err
	}
	if len(o.EvidenceRefs) == 0 {
		return errors.New("formal evidence refs are required")
	}
	for _, ref := range o.EvidenceRefs {
		if err := ref.ValidateShape("evidence ref"); err != nil {
			return err
		}
	}
	if strings.TrimSpace(o.ObservedState) == "" {
		return errors.New("observed state is required")
	}
	return nil
}

func (o Observation) ValidateCurrent(now time.Time) error {
	if err := o.ValidateShape(); err != nil {
		return err
	}
	return o.Meta.ValidateCurrent(now)
}

type Disposition string

const (
	DispositionConfirmedApplied    Disposition = "confirmed_applied"
	DispositionConfirmedNotApplied Disposition = "confirmed_not_applied"
	DispositionFailed              Disposition = "failed"
	DispositionUnknown             Disposition = "unknown"
	DispositionResidual            Disposition = "residual"
)

func (d Disposition) Validate() error {
	switch d {
	case DispositionConfirmedApplied, DispositionConfirmedNotApplied, DispositionFailed, DispositionUnknown, DispositionResidual:
		return nil
	default:
		return fmt.Errorf("unsupported disposition %q", d)
	}
}

type InspectionFact struct {
	Meta                  Meta           `json:"meta"`
	ReservationRef        Ref            `json:"reservation_ref"`
	ObservationRef        Ref            `json:"observation_ref"`
	OperationID           string         `json:"operation_id"`
	AttemptID             string         `json:"attempt_id"`
	Disposition           Disposition    `json:"disposition"`
	Coverage              []string       `json:"coverage"`
	EvidenceRefs          []Ref          `json:"evidence_refs"`
	Residuals             []Residual     `json:"residuals,omitempty"`
	Cleanup               *CleanupReport `json:"cleanup,omitempty"`
	WorkspaceChangeSetRef *Ref           `json:"workspace_change_set_ref,omitempty"`
}

func (i InspectionFact) ValidateShape() error {
	if err := i.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := i.ReservationRef.ValidateShape("reservation ref"); err != nil {
		return err
	}
	if err := i.ObservationRef.ValidateShape("observation ref"); err != nil {
		return err
	}
	if strings.TrimSpace(i.OperationID) == "" || strings.TrimSpace(i.AttemptID) == "" {
		return errors.New("operation and attempt ids are required")
	}
	if err := i.Disposition.Validate(); err != nil {
		return err
	}
	if len(i.Coverage) == 0 || len(i.EvidenceRefs) == 0 {
		return errors.New("inspection coverage and evidence refs are required")
	}
	if err := ValidateSortedUnique(i.Coverage, "inspection coverage"); err != nil {
		return err
	}
	for _, ref := range i.EvidenceRefs {
		if err := ref.ValidateShape("inspection evidence ref"); err != nil {
			return err
		}
	}
	for _, residual := range i.Residuals {
		if err := residual.ValidateShape(); err != nil {
			return err
		}
	}
	if i.Cleanup != nil {
		if err := i.Cleanup.ValidateShape(); err != nil {
			return err
		}
	}
	if i.WorkspaceChangeSetRef != nil {
		if err := i.WorkspaceChangeSetRef.ValidateShape("workspace change set ref"); err != nil {
			return err
		}
	}
	return nil
}

func (i InspectionFact) ValidateCurrent(now time.Time) error {
	if err := i.ValidateShape(); err != nil {
		return err
	}
	return i.Meta.ValidateCurrent(now)
}

type DomainResultPayload struct {
	AllocationConfirmed   bool           `json:"allocation_confirmed,omitempty"`
	ActivationConfirmed   bool           `json:"activation_confirmed,omitempty"`
	OpenConfirmed         bool           `json:"open_confirmed,omitempty"`
	ExecutionQuiesced     bool           `json:"execution_quiesced,omitempty"`
	EnvironmentClosed     bool           `json:"environment_closed,omitempty"`
	FenceConfirmed        bool           `json:"fence_confirmed,omitempty"`
	ReleaseConfirmed      bool           `json:"release_confirmed,omitempty"`
	Cleanup               *CleanupReport `json:"cleanup,omitempty"`
	WorkspaceChangeSetRef *Ref           `json:"workspace_change_set_ref,omitempty"`
}

type SandboxDomainResultFact struct {
	Meta           Meta                `json:"meta"`
	ReservationRef Ref                 `json:"reservation_ref"`
	InspectionRef  Ref                 `json:"inspection_ref"`
	OperationID    string              `json:"operation_id"`
	AttemptID      string              `json:"attempt_id"`
	Kind           EffectKind          `json:"kind"`
	Disposition    Disposition         `json:"disposition"`
	Lease          RuntimeLeaseBinding `json:"lease"`
	Payload        DomainResultPayload `json:"payload"`
	EvidenceRefs   []Ref               `json:"evidence_refs"`
}

func (f SandboxDomainResultFact) ValidateShape() error {
	if err := f.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := f.ReservationRef.ValidateShape("reservation ref"); err != nil {
		return err
	}
	if err := f.InspectionRef.ValidateShape("inspection ref"); err != nil {
		return err
	}
	if strings.TrimSpace(f.OperationID) == "" || strings.TrimSpace(f.AttemptID) == "" {
		return errors.New("operation and attempt ids are required")
	}
	if err := f.Kind.Validate(); err != nil {
		return err
	}
	if err := f.Disposition.Validate(); err != nil {
		return err
	}
	if err := f.Lease.ValidateShape(); err != nil {
		return err
	}
	if len(f.EvidenceRefs) == 0 {
		return errors.New("domain result evidence refs are required")
	}
	for _, ref := range f.EvidenceRefs {
		if err := ref.ValidateShape("domain result evidence ref"); err != nil {
			return err
		}
	}
	return validatePayloadForKind(f.Kind, f.Disposition, f.Payload)
}

func (f SandboxDomainResultFact) ValidateCurrent(now time.Time) error {
	if err := f.ValidateShape(); err != nil {
		return err
	}
	return f.Meta.ValidateCurrent(now)
}

func validatePayloadForKind(kind EffectKind, disposition Disposition, payload DomainResultPayload) error {
	if payload.Cleanup != nil {
		if err := payload.Cleanup.ValidateShape(); err != nil {
			return err
		}
	}
	if payload.WorkspaceChangeSetRef != nil {
		if err := payload.WorkspaceChangeSetRef.ValidateShape("workspace change set ref"); err != nil {
			return err
		}
	}
	if disposition != DispositionConfirmedApplied {
		if payloadClaimCount(payload) != 0 {
			return errors.New("non-applied domain result cannot assert applied payload")
		}
		return nil
	}
	claims := payloadClaimCount(payload)
	valid := claims == 1
	switch kind {
	case EffectAllocate:
		valid = valid && payload.AllocationConfirmed
	case EffectActivate:
		valid = valid && payload.ActivationConfirmed
	case EffectOpen:
		valid = valid && payload.OpenConfirmed
	case EffectCancel:
		valid = valid && payload.ExecutionQuiesced
	case EffectClose:
		valid = valid && payload.EnvironmentClosed
	case EffectFence:
		valid = valid && payload.FenceConfirmed
	case EffectRelease:
		valid = valid && payload.ReleaseConfirmed
	case EffectCleanup:
		valid = valid && payload.Cleanup != nil
	case EffectWorkspaceCommit:
		valid = valid && payload.WorkspaceChangeSetRef != nil
	case EffectInspect:
		valid = claims == 0 || (claims == 1 && payload.ExecutionQuiesced)
	}
	if !valid {
		return fmt.Errorf("applied result payload does not match effect kind %q", kind)
	}
	return nil
}

func payloadClaimCount(payload DomainResultPayload) int {
	count := 0
	for _, claimed := range []bool{
		payload.AllocationConfirmed,
		payload.ActivationConfirmed,
		payload.OpenConfirmed,
		payload.ExecutionQuiesced,
		payload.EnvironmentClosed,
		payload.FenceConfirmed,
		payload.ReleaseConfirmed,
		payload.Cleanup != nil,
		payload.WorkspaceChangeSetRef != nil,
	} {
		if claimed {
			count++
		}
	}
	return count
}

// RuntimeOperationSettlementRef is an opaque exact-binding coordinate. It
// intentionally carries no Runtime disposition, outcome, authority, or state.
type RuntimeOperationSettlementRef struct {
	OpaqueRef       Ref    `json:"opaque_ref"`
	OperationID     string `json:"operation_id"`
	AttemptID       string `json:"attempt_id"`
	DomainResultRef Ref    `json:"domain_result_ref"`
}

func (r RuntimeOperationSettlementRef) ValidateShape() error {
	if err := r.OpaqueRef.ValidateShape("opaque runtime operation settlement ref"); err != nil {
		return err
	}
	if err := r.DomainResultRef.ValidateShape("domain result ref"); err != nil {
		return err
	}
	if strings.TrimSpace(r.OperationID) == "" || strings.TrimSpace(r.AttemptID) == "" {
		return errors.New("runtime operation and attempt ids are required")
	}
	return nil
}

type EnvironmentProjection struct {
	Meta                Meta                `json:"meta"`
	Lease               RuntimeLeaseBinding `json:"lease"`
	Allocated           bool                `json:"allocated"`
	Activated           bool                `json:"activated"`
	Open                bool                `json:"open"`
	ExecutionQuiesced   bool                `json:"execution_quiesced"`
	EnvironmentClosed   bool                `json:"environment_closed"`
	Fenced              bool                `json:"fenced"`
	Released            bool                `json:"released"`
	Cleanup             CleanupReport       `json:"cleanup"`
	LastDomainResultRef Ref                 `json:"last_domain_result_ref"`
	LastSettlementRef   Ref                 `json:"last_settlement_ref"`
}

func (p EnvironmentProjection) ValidateShape() error {
	if err := p.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := p.Lease.ValidateShape(); err != nil {
		return err
	}
	if p.Open && (!p.Activated || p.EnvironmentClosed) {
		return errors.New("open projection requires activated and not closed environment")
	}
	if p.Activated && !p.Allocated {
		return errors.New("activated projection requires allocation")
	}
	if p.EnvironmentClosed && !p.ExecutionQuiesced {
		return errors.New("environment closed cannot imply execution quiesced")
	}
	if p.Released && (!p.EnvironmentClosed || !p.Fenced) {
		return errors.New("released projection requires closed and fenced environment")
	}
	if (p.LastDomainResultRef.ID == "") != (p.LastSettlementRef.ID == "") {
		return errors.New("domain result and runtime settlement refs must be applied together")
	}
	if p.LastDomainResultRef.ID != "" {
		if err := p.LastDomainResultRef.ValidateShape("last domain result ref"); err != nil {
			return err
		}
		if err := p.LastSettlementRef.ValidateShape("last runtime settlement ref"); err != nil {
			return err
		}
	}
	if p.Cleanup.Processes != "" || p.Cleanup.FileMounts != "" || p.Cleanup.Network != "" || p.Cleanup.Secrets != "" || p.Cleanup.BackgroundTasks != "" || p.Cleanup.RemoteContinuation != "" || p.Cleanup.ProviderRetention != "" || len(p.Cleanup.EvidenceRefs) != 0 {
		if err := p.Cleanup.ValidateShape(); err != nil {
			return err
		}
	}
	return nil
}

func (p EnvironmentProjection) ValidateCurrent(now time.Time) error {
	if err := p.ValidateShape(); err != nil {
		return err
	}
	if err := p.Meta.ValidateCurrent(now); err != nil {
		return err
	}
	return p.Lease.ValidateCurrent(now)
}

type RunCompletionProjection struct {
	ExecutionQuiesced bool `json:"execution_quiesced"`
}

type TerminationReport struct {
	EnvironmentClosed bool          `json:"environment_closed"`
	Fenced            bool          `json:"fenced"`
	Released          bool          `json:"released"`
	Cleanup           CleanupReport `json:"cleanup"`
}

func (p EnvironmentProjection) RunCompletion() RunCompletionProjection {
	return RunCompletionProjection{ExecutionQuiesced: p.ExecutionQuiesced}
}

func (p EnvironmentProjection) Termination() TerminationReport {
	return TerminationReport{
		EnvironmentClosed: p.EnvironmentClosed,
		Fenced:            p.Fenced,
		Released:          p.Released,
		Cleanup:           p.Cleanup,
	}
}

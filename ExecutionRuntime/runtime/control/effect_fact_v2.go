package control

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type EffectFactStateV2 string

const (
	EffectProposed       EffectFactStateV2 = "proposed"
	EffectAccepted       EffectFactStateV2 = "accepted"
	EffectDispatchIntent EffectFactStateV2 = "dispatch_intent"
	EffectDispatched     EffectFactStateV2 = "dispatched"
	EffectUnknownOutcome EffectFactStateV2 = "unknown_outcome"
	EffectSettled        EffectFactStateV2 = "settled"
	EffectCompensated    EffectFactStateV2 = "compensated"
	EffectRejected       EffectFactStateV2 = "rejected"
)

type DispatchPermitStateV2 string

const (
	DispatchPermitIssued  DispatchPermitStateV2 = "issued"
	DispatchPermitBegun   DispatchPermitStateV2 = "begun"
	DispatchPermitExpired DispatchPermitStateV2 = "expired"
	DispatchPermitRevoked DispatchPermitStateV2 = "revoked"
)

type RemoteResidualStateV2 string

const (
	RemoteResidualNotRequired     RemoteResidualStateV2 = "not_required"
	RemoteResidualPending         RemoteResidualStateV2 = "pending"
	RemoteResidualConfirmedAbsent RemoteResidualStateV2 = "confirmed_absent"
	RemoteResidualPresent         RemoteResidualStateV2 = "present"
	RemoteResidualUnknown         RemoteResidualStateV2 = "unknown"
)

type EffectCleanupStateV2 string

const (
	EffectCleanupNotRequired   EffectCleanupStateV2 = "not_required"
	EffectCleanupPending       EffectCleanupStateV2 = "pending"
	EffectCleanupComplete      EffectCleanupStateV2 = "complete"
	EffectCleanupFailed        EffectCleanupStateV2 = "failed"
	EffectCleanupIndeterminate EffectCleanupStateV2 = "indeterminate"
)

type ProviderDispatchReceiptV2 struct {
	PermitID             string                     `json:"permit_id"`
	PermitDigest         core.Digest                `json:"permit_digest"`
	AttemptID            string                     `json:"attempt_id"`
	IntentID             core.EffectIntentID        `json:"intent_id"`
	IntentRevision       core.Revision              `json:"intent_revision"`
	Provider             ports.ProviderBindingRefV2 `json:"provider_binding"`
	ProviderOperationRef string                     `json:"provider_operation_ref,omitempty"`
	ReceiptRef           string                     `json:"receipt_ref,omitempty"`
	ObservationDigest    core.Digest                `json:"observation_digest"`
	ObservedUnixNano     int64                      `json:"observed_unix_nano"`
}

type EffectSettlementDispositionV2 string

const (
	SettlementConfirmedApplied    EffectSettlementDispositionV2 = "confirmed_applied"
	SettlementConfirmedNotApplied EffectSettlementDispositionV2 = "confirmed_not_applied"
	SettlementConfirmedFailed     EffectSettlementDispositionV2 = "confirmed_failed"
)

type EffectSettlementFactV2 struct {
	Owner                      ports.EffectOwnerRefV2        `json:"owner"`
	Disposition                EffectSettlementDispositionV2 `json:"disposition"`
	ReceiptRef                 string                        `json:"receipt_ref"`
	EvidenceDigest             core.Digest                   `json:"evidence_digest"`
	InspectionIntentID         core.EffectIntentID           `json:"inspection_intent_id,omitempty"`
	InspectionIntentRevision   core.Revision                 `json:"inspection_intent_revision,omitempty"`
	InspectionSettlementDigest core.Digest                   `json:"inspection_settlement_digest,omitempty"`
	DomainResult               *ports.OpaquePayloadV2        `json:"domain_result,omitempty"`
	SettledUnixNano            int64                         `json:"settled_unix_nano"`
}

type CompensationCompletionV2 struct {
	EffectID         core.EffectIntentID `json:"effect_id"`
	EffectRevision   core.Revision       `json:"effect_revision"`
	SettlementDigest core.Digest         `json:"settlement_digest"`
}

type EffectResolutionCompletionV2 struct {
	EffectID         core.EffectIntentID `json:"effect_id"`
	EffectRevision   core.Revision       `json:"effect_revision"`
	SettlementDigest core.Digest         `json:"settlement_digest"`
}

type EffectFactV2 struct {
	Intent               ports.EffectIntentV2          `json:"intent"`
	IntentDigest         core.Digest                   `json:"intent_digest"`
	State                EffectFactStateV2             `json:"state"`
	Revision             core.Revision                 `json:"revision"`
	DispatchPermitID     string                        `json:"dispatch_permit_id,omitempty"`
	DispatchPermitDigest core.Digest                   `json:"dispatch_permit_digest,omitempty"`
	DispatchReceipt      *ProviderDispatchReceiptV2    `json:"dispatch_receipt,omitempty"`
	Settlement           *EffectSettlementFactV2       `json:"settlement,omitempty"`
	Compensation         *CompensationCompletionV2     `json:"compensation,omitempty"`
	RemoteResidual       RemoteResidualStateV2         `json:"remote_residual"`
	ResidualResolution   *EffectResolutionCompletionV2 `json:"residual_resolution,omitempty"`
	Cleanup              EffectCleanupStateV2          `json:"cleanup"`
	CleanupResolution    *EffectResolutionCompletionV2 `json:"cleanup_resolution,omitempty"`
	RejectionReason      core.ReasonCode               `json:"rejection_reason,omitempty"`
	UpdatedUnixNano      int64                         `json:"updated_unix_nano"`
}

type DispatchPermitFactV2 struct {
	Permit             ports.DispatchPermitV2      `json:"permit"`
	PermitDigest       core.Digest                 `json:"permit_digest"`
	Fence              core.ExecutionFence         `json:"fence"`
	State              DispatchPermitStateV2       `json:"state"`
	Revision           core.Revision               `json:"revision"`
	EffectFactRevision core.Revision               `json:"effect_fact_revision"`
	BegunUnixNano      int64                       `json:"begun_unix_nano,omitempty"`
	Enforcement        *ports.EnforcementReceiptV2 `json:"enforcement_receipt,omitempty"`
}

type EffectFactCASRequestV2 struct {
	ExpectedRevision core.Revision `json:"expected_revision"`
	Next             EffectFactV2  `json:"next"`
}

type IssueDispatchPermitRequestV2 struct {
	EffectID               core.EffectIntentID    `json:"effect_id"`
	ExpectedEffectRevision core.Revision          `json:"expected_effect_revision"`
	Permit                 ports.DispatchPermitV2 `json:"permit"`
	Fence                  core.ExecutionFence    `json:"fence"`
}

type IssueDispatchPermitResultV2 struct {
	Effect EffectFactV2         `json:"effect"`
	Permit DispatchPermitFactV2 `json:"permit"`
}

type BeginDispatchRequestV2 struct {
	EffectID               core.EffectIntentID `json:"effect_id"`
	ExpectedEffectRevision core.Revision       `json:"expected_effect_revision"`
	PermitID               string              `json:"permit_id"`
	ExpectedPermitRevision core.Revision       `json:"expected_permit_revision"`
}

type RecordEnforcementReceiptRequestV2 struct {
	PermitID               string                     `json:"permit_id"`
	ExpectedPermitRevision core.Revision              `json:"expected_permit_revision"`
	Receipt                ports.EnforcementReceiptV2 `json:"receipt"`
}

type DispatchPermitFactCASRequestV2 struct {
	PermitID         string               `json:"permit_id"`
	ExpectedRevision core.Revision        `json:"expected_revision"`
	Next             DispatchPermitFactV2 `json:"next"`
}

type EffectFactPortV2 interface {
	CreateEffect(context.Context, EffectFactV2) (EffectFactV2, error)
	InspectEffect(context.Context, core.EffectIntentID) (EffectFactV2, error)
	InspectEffectByIdempotency(context.Context, ports.EffectStableScopeClassV2, core.Digest, string) (EffectFactV2, error)
	InspectConflictDomain(context.Context, ports.ConflictDomainBindingV2) (EffectFactV2, error)
	CompareAndSwapEffect(context.Context, EffectFactCASRequestV2) (EffectFactV2, error)
	IssueDispatchPermit(context.Context, IssueDispatchPermitRequestV2) (IssueDispatchPermitResultV2, error)
	InspectDispatchPermit(context.Context, string) (DispatchPermitFactV2, error)
	BeginDispatch(context.Context, BeginDispatchRequestV2) (DispatchPermitFactV2, error)
	RecordEnforcementReceipt(context.Context, RecordEnforcementReceiptRequestV2) (DispatchPermitFactV2, error)
	CompareAndSwapDispatchPermit(context.Context, DispatchPermitFactCASRequestV2) (DispatchPermitFactV2, error)
}

type EffectTransitionContextV2 struct {
	PermitBegun              bool
	DispatchReceiptMatched   bool
	SettlementOwnerMatched   bool
	UnknownInspectionSettled bool
	CompensationSettled      bool
	ResidualInspectSettled   bool
	CleanupEffectSettled     bool
}

func NewProposedEffectFactV2(intent ports.EffectIntentV2, now time.Time) (EffectFactV2, error) {
	if now.IsZero() || !now.Before(time.Unix(0, intent.ExpiresUnixNano)) {
		return EffectFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonEffectIntentMissing, "proposed effect requires injected time before intent expiry")
	}
	digest, err := intent.DigestV2()
	if err != nil {
		return EffectFactV2{}, err
	}
	residual := RemoteResidualNotRequired
	if intent.MayLeaveRemoteResidual {
		residual = RemoteResidualPending
	}
	cleanup := EffectCleanupNotRequired
	if intent.RequiresCleanup {
		cleanup = EffectCleanupPending
	}
	fact := EffectFactV2{Intent: intent, IntentDigest: digest, State: EffectProposed, Revision: 1, RemoteResidual: residual, Cleanup: cleanup, UpdatedUnixNano: now.UnixNano()}
	return fact, fact.Validate()
}

func (f EffectFactV2) Validate() error {
	if err := f.Intent.Validate(); err != nil {
		return err
	}
	digest, err := f.Intent.DigestV2()
	if err != nil {
		return err
	}
	if digest != f.IntentDigest || f.Revision == 0 || f.UpdatedUnixNano <= 0 || !validEffectFactStateV2(f.State) || !validRemoteResidualStateV2(f.RemoteResidual) || !validEffectCleanupStateV2(f.Cleanup) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectStateConflict, "effect fact identity, state, closure dimensions or update time are invalid")
	}
	if f.DispatchPermitID == "" {
		if f.DispatchPermitDigest != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "permit digest requires permit id")
		}
	} else if err := f.DispatchPermitDigest.Validate(); err != nil {
		return err
	}
	if f.DispatchReceipt != nil {
		if err := f.DispatchReceipt.Validate(); err != nil {
			return err
		}
	}
	if f.Settlement != nil {
		if err := f.Settlement.Validate(); err != nil {
			return err
		}
	}
	if f.Compensation != nil {
		if err := f.Compensation.Validate(); err != nil {
			return err
		}
	}
	if f.ResidualResolution != nil {
		if err := f.ResidualResolution.Validate(); err != nil {
			return err
		}
	}
	if f.CleanupResolution != nil {
		if err := f.CleanupResolution.Validate(); err != nil {
			return err
		}
	}
	if (f.RemoteResidual == RemoteResidualNotRequired || f.RemoteResidual == RemoteResidualPending) != (f.ResidualResolution == nil) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRemoteResidualUnresolved, "remote residual conclusion must bind an independent inspection effect")
	}
	if (f.Cleanup == EffectCleanupNotRequired || f.Cleanup == EffectCleanupPending) != (f.CleanupResolution == nil) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCleanupEvidenceIncomplete, "cleanup conclusion must bind an independent cleanup effect")
	}
	switch f.State {
	case EffectProposed, EffectAccepted:
		if f.DispatchPermitID != "" || f.DispatchReceipt != nil || f.Settlement != nil || f.Compensation != nil || f.RejectionReason != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "pre-dispatch effect contains later-stage facts")
		}
	case EffectDispatchIntent:
		if f.DispatchPermitID == "" || f.DispatchReceipt != nil || f.Settlement != nil || f.Compensation != nil || f.RejectionReason != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "dispatch_intent requires only an issued permit")
		}
	case EffectDispatched, EffectUnknownOutcome:
		if f.DispatchPermitID == "" || f.Settlement != nil || f.Compensation != nil || f.RejectionReason != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "post-begin effect contains inconsistent authority facts")
		}
		if f.State == EffectDispatched && f.DispatchReceipt == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "dispatched effect requires a provider receipt observation")
		}
	case EffectSettled:
		if f.Settlement == nil || f.Compensation != nil || f.RejectionReason != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "settled effect requires authoritative settlement only")
		}
	case EffectCompensated:
		if f.Settlement == nil || f.Compensation == nil || f.RejectionReason != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonCompensationIncomplete, "compensated effect requires original settlement and independent compensation settlement")
		}
	case EffectRejected:
		if f.RejectionReason == "" || f.DispatchReceipt != nil || f.Settlement != nil || f.Compensation != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "pre-dispatch rejection requires a reason and no post-dispatch claims")
		}
	}
	return nil
}

func ValidateEffectFactTransitionV2(current, next EffectFactV2, context EffectTransitionContextV2, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < current.UpdatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "effect transition clock regressed")
	}
	// IntentDigest is the canonical, domain-separated identity. Comparing the Go
	// representation as well would incorrectly reject contract-equivalent
	// nil/empty sets after a storage round trip.
	if current.Intent.ID != next.Intent.ID || current.IntentDigest != next.IntentDigest || next.Revision != current.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "effect intent is immutable and revision must advance once")
	}
	if next.UpdatedUnixNano != now.UnixNano() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "effect transition update time must come from the injected clock")
	}
	if current.DispatchPermitID != "" && (next.DispatchPermitID != current.DispatchPermitID || next.DispatchPermitDigest != current.DispatchPermitDigest) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "effect dispatch permit binding is immutable")
	}
	if current.DispatchReceipt != nil && !reflect.DeepEqual(current.DispatchReceipt, next.DispatchReceipt) || current.Settlement != nil && !reflect.DeepEqual(current.Settlement, next.Settlement) || current.Compensation != nil && !reflect.DeepEqual(current.Compensation, next.Compensation) || current.RejectionReason != "" && current.RejectionReason != next.RejectionReason {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "recorded dispatch, settlement, compensation and rejection facts are immutable")
	}
	if err := validateClosureTransitionV2(current, next, context); err != nil {
		return err
	}
	valid := false
	mainStateChanged := current.State != next.State
	if !mainStateChanged {
		if current.State == EffectProposed || current.State == EffectAccepted || current.State == EffectDispatchIntent || current.State == EffectRejected || current.RemoteResidual == next.RemoteResidual && current.Cleanup == next.Cleanup {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "same-state CAS must advance an orthogonal residual or cleanup dimension")
		}
		valid = true
	}
	if mainStateChanged {
		switch current.State {
		case EffectProposed:
			valid = next.State == EffectAccepted || next.State == EffectRejected
		case EffectAccepted:
			valid = next.State == EffectDispatchIntent || next.State == EffectRejected
		case EffectDispatchIntent:
			valid = next.State == EffectDispatched || next.State == EffectUnknownOutcome || next.State == EffectRejected
			if next.State == EffectRejected && context.PermitBegun {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "post-begin effect cannot be safely rejected")
			}
			if (next.State == EffectDispatched || next.State == EffectUnknownOutcome) && !context.PermitBegun {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "provider outcome requires begun dispatch write-ahead")
			}
			if next.State == EffectDispatched && !context.DispatchReceiptMatched {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "provider receipt must match the exact begun permit and attempt")
			}
		case EffectDispatched, EffectUnknownOutcome:
			valid = next.State == EffectSettled
			if valid && !context.SettlementOwnerMatched {
				return core.NewError(core.ErrorForbidden, core.ReasonSettlementOwnerMismatch, "settlement requires the binding settlement owner")
			}
			if valid && current.State == EffectUnknownOutcome && !context.UnknownInspectionSettled {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "unknown outcome may settle only through an independently settled exact inspection effect")
			}
		case EffectSettled:
			valid = next.State == EffectCompensated
			if valid && !context.CompensationSettled {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonCompensationIncomplete, "original effect cannot be compensated before the independent compensation effect settles")
			}
		case EffectCompensated:
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "compensated main state is terminal; only orthogonal closure dimensions may advance")
		case EffectRejected:
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "compensated or rejected effect state is terminal")
		}
	}
	if !valid {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "effect state transition is not allowed")
	}
	return nil
}

func (p DispatchPermitFactV2) Validate() error {
	if err := p.Permit.Validate(); err != nil {
		return err
	}
	digest, err := p.Permit.DigestV2()
	if err != nil {
		return err
	}
	if digest != p.PermitDigest || p.Revision == 0 || p.EffectFactRevision == 0 || !validDispatchPermitStateV2(p.State) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "permit fact identity, state and effect revision are invalid")
	}
	if err := p.Fence.Validate(); err != nil {
		return err
	}
	fenceDigest, err := ports.DigestExecutionFenceV2(p.Fence)
	if err != nil || fenceDigest != p.Permit.FenceDigest || p.Fence.EffectIntentID != p.Permit.IntentID || p.Fence.EffectIntentRevision != p.Permit.IntentRevision || p.Fence.CanonicalPayloadDigest != p.Permit.PayloadDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "permit fact must persist the gateway-constructed exact fence")
	}
	if p.State == DispatchPermitIssued && (p.BegunUnixNano != 0 || p.Enforcement != nil) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "issued permit cannot contain begin evidence")
	}
	if p.State == DispatchPermitBegun {
		if p.BegunUnixNano <= 0 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "begun permit requires write-ahead time")
		}
		if p.Enforcement != nil {
			if err := p.Enforcement.Validate(); err != nil {
				return err
			}
			if p.Enforcement.PermitID != p.Permit.ID || p.Enforcement.PermitRevision != p.Permit.Revision || p.Enforcement.AttemptID != p.Permit.AttemptID || p.Enforcement.PermitDigest != p.PermitDigest || p.Enforcement.Verifier != p.Permit.EnforcementPoint {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "enforcement receipt is not bound to the begun permit attempt")
			}
		}
	}
	return nil
}

func ValidateDispatchPermitTransitionV2(current, next DispatchPermitFactV2, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(current.Permit, next.Permit) || current.PermitDigest != next.PermitDigest || !reflect.DeepEqual(current.Fence, next.Fence) || current.EffectFactRevision != next.EffectFactRevision || next.Revision != current.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "permit identity is immutable and revision must advance once")
	}
	if now.IsZero() || now.UnixNano() < current.Permit.IssuedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "permit transition clock regressed")
	}
	if current.State == DispatchPermitBegun && next.State == DispatchPermitBegun {
		if current.Enforcement != nil {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "enforcement receipt is immutable once recorded")
		}
		if next.Enforcement == nil || next.BegunUnixNano != current.BegunUnixNano || now.UnixNano() < current.BegunUnixNano || next.Enforcement.ValidatedAt < current.BegunUnixNano || next.Enforcement.ValidatedAt > now.UnixNano() {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "enforcement receipt must be produced after begin and no later than the injected fact time")
		}
		return nil
	}
	if current.State != DispatchPermitIssued {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitConsumed, "dispatch permit may be consumed or invalidated only once")
	}
	if next.State == DispatchPermitBegun {
		if !now.Before(time.Unix(0, current.Permit.ExpiresUnixNano)) || next.BegunUnixNano != now.UnixNano() {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitExpired, "permit expired at or before begin boundary")
		}
		return nil
	}
	if next.State == DispatchPermitExpired {
		if now.Before(time.Unix(0, current.Permit.ExpiresUnixNano)) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitExpired, "permit cannot be marked expired before its exact boundary")
		}
		return nil
	}
	if next.State == DispatchPermitRevoked {
		return nil
	}
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "permit transition is not allowed")
}

func (r ProviderDispatchReceiptV2) Validate() error {
	if strings.TrimSpace(r.PermitID) == "" || strings.TrimSpace(r.AttemptID) == "" || strings.TrimSpace(string(r.IntentID)) == "" || r.IntentRevision == 0 || r.ObservedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "provider receipt observation requires attempt id and time")
	}
	if err := r.PermitDigest.Validate(); err != nil {
		return err
	}
	if err := r.Provider.Validate(); err != nil {
		return err
	}
	return r.ObservationDigest.Validate()
}

func (s EffectSettlementFactV2) Validate() error {
	if s.Owner.Role != ports.OwnerSettlement || strings.TrimSpace(s.ReceiptRef) == "" || s.SettledUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "settlement requires the exact settlement owner, outcome, receipt and time")
	}
	if s.Disposition != SettlementConfirmedApplied && s.Disposition != SettlementConfirmedNotApplied && s.Disposition != SettlementConfirmedFailed {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectUnknownOutcome, "settlement disposition must be a closed Runtime-governance value")
	}
	if err := s.Owner.ManifestDigest.Validate(); err != nil {
		return err
	}
	if err := s.EvidenceDigest.Validate(); err != nil {
		return err
	}
	inspection := strings.TrimSpace(string(s.InspectionIntentID)) != "" || s.InspectionIntentRevision != 0 || s.InspectionSettlementDigest != ""
	if inspection && (strings.TrimSpace(string(s.InspectionIntentID)) == "" || s.InspectionIntentRevision == 0 || s.InspectionSettlementDigest.Validate() != nil) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectUnknownOutcome, "remote settlement inspection must bind its independent effect intent revision")
	}
	if s.DomainResult != nil {
		if err := s.DomainResult.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c CompensationCompletionV2) Validate() error {
	if strings.TrimSpace(string(c.EffectID)) == "" || c.EffectRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCompensationIncomplete, "compensation effect id and revision are required")
	}
	return c.SettlementDigest.Validate()
}

func (c EffectResolutionCompletionV2) Validate() error {
	if strings.TrimSpace(string(c.EffectID)) == "" || c.EffectRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCleanupEvidenceIncomplete, "resolution effect id and revision are required")
	}
	return c.SettlementDigest.Validate()
}

func (f EffectFactV2) ConflictDomainOccupied() bool {
	if f.State == EffectRejected {
		return false
	}
	if f.State != EffectSettled && f.State != EffectCompensated {
		return true
	}
	residualClosed := f.RemoteResidual == RemoteResidualNotRequired || f.RemoteResidual == RemoteResidualConfirmedAbsent
	cleanupClosed := f.Cleanup == EffectCleanupNotRequired || f.Cleanup == EffectCleanupComplete
	return !residualClosed || !cleanupClosed
}

func validateClosureTransitionV2(current, next EffectFactV2, context EffectTransitionContextV2) error {
	if current.ResidualResolution != nil && !reflect.DeepEqual(current.ResidualResolution, next.ResidualResolution) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "remote residual resolution is immutable once recorded")
	}
	if current.CleanupResolution != nil && !reflect.DeepEqual(current.CleanupResolution, next.CleanupResolution) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "cleanup resolution is immutable once recorded")
	}
	if current.RemoteResidual != next.RemoteResidual {
		valid := current.RemoteResidual == RemoteResidualPending && (next.RemoteResidual == RemoteResidualConfirmedAbsent || next.RemoteResidual == RemoteResidualPresent || next.RemoteResidual == RemoteResidualUnknown) ||
			current.RemoteResidual == RemoteResidualUnknown && (next.RemoteResidual == RemoteResidualConfirmedAbsent || next.RemoteResidual == RemoteResidualPresent) ||
			current.RemoteResidual == RemoteResidualPresent && next.RemoteResidual == RemoteResidualConfirmedAbsent
		if !valid || !context.ResidualInspectSettled || current.ResidualResolution != nil || next.ResidualResolution == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRemoteResidualUnresolved, "remote residual change requires an independently settled inspection effect")
		}
	} else if !reflect.DeepEqual(current.ResidualResolution, next.ResidualResolution) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "residual resolution cannot change without a monotonic residual state transition")
	}
	if current.Cleanup != next.Cleanup {
		valid := current.Cleanup == EffectCleanupPending && (next.Cleanup == EffectCleanupComplete || next.Cleanup == EffectCleanupFailed || next.Cleanup == EffectCleanupIndeterminate)
		if !valid || !context.CleanupEffectSettled || current.CleanupResolution != nil || next.CleanupResolution == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonCleanupEvidenceIncomplete, "cleanup conclusion requires an independently settled cleanup effect")
		}
	} else if !reflect.DeepEqual(current.CleanupResolution, next.CleanupResolution) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "cleanup resolution cannot change without a monotonic cleanup state transition")
	}
	return nil
}

func validEffectFactStateV2(value EffectFactStateV2) bool {
	switch value {
	case EffectProposed, EffectAccepted, EffectDispatchIntent, EffectDispatched, EffectUnknownOutcome, EffectSettled, EffectCompensated, EffectRejected:
		return true
	default:
		return false
	}
}

func validDispatchPermitStateV2(value DispatchPermitStateV2) bool {
	return value == DispatchPermitIssued || value == DispatchPermitBegun || value == DispatchPermitExpired || value == DispatchPermitRevoked
}

func validRemoteResidualStateV2(value RemoteResidualStateV2) bool {
	switch value {
	case RemoteResidualNotRequired, RemoteResidualPending, RemoteResidualConfirmedAbsent, RemoteResidualPresent, RemoteResidualUnknown:
		return true
	default:
		return false
	}
}

func validEffectCleanupStateV2(value EffectCleanupStateV2) bool {
	switch value {
	case EffectCleanupNotRequired, EffectCleanupPending, EffectCleanupComplete, EffectCleanupFailed, EffectCleanupIndeterminate:
		return true
	default:
		return false
	}
}

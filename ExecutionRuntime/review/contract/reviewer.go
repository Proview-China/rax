package contract

import (
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func ComputeReviewEvidenceDigestV1(evidence []runtimeports.ReviewEvidenceRefV2) (core.Digest, error) {
	if len(evidence) > MaxListItemsV1 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review evidence exceeds its bound")
	}
	copyValue := append([]runtimeports.ReviewEvidenceRefV2{}, evidence...)
	sort.Slice(copyValue, func(i, j int) bool { return copyValue[i].Ref < copyValue[j].Ref })
	for i, value := range copyValue {
		if err := value.Validate(); err != nil {
			return "", err
		}
		if i > 0 && copyValue[i-1].Ref == value.Ref {
			return "", core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "review evidence ref is duplicated")
		}
	}
	return seal("ReviewEvidenceSetV1", copyValue)
}

type AssignmentStateV1 string

const (
	AssignmentOfferedV1  AssignmentStateV1 = "offered"
	AssignmentClaimedV1  AssignmentStateV1 = "claimed"
	AssignmentReleasedV1 AssignmentStateV1 = "released"
	AssignmentExpiredV1  AssignmentStateV1 = "expired"
	AssignmentRevokedV1  AssignmentStateV1 = "revoked"
)

type ReviewerAssignmentV1 struct {
	FactIdentityV1
	CaseID               string                                   `json:"case_id"`
	CaseRevision         core.Revision                            `json:"case_revision"`
	RoundID              string                                   `json:"round_id"`
	RoundRevision        core.Revision                            `json:"round_revision"`
	RoundDigest          core.Digest                              `json:"round_digest"`
	TargetID             string                                   `json:"target_id"`
	TargetRevision       core.Revision                            `json:"target_revision"`
	TargetDigest         core.Digest                              `json:"target_digest"`
	Route                RouteV1                                  `json:"route"`
	ReviewerID           string                                   `json:"reviewer_id"`
	ReviewerAuthority    runtimeports.AuthorityBindingRefV2       `json:"reviewer_authority"`
	ReviewerBinding      runtimeports.ReviewComponentBindingRefV2 `json:"reviewer_binding"`
	Capability           string                                   `json:"capability"`
	State                AssignmentStateV1                        `json:"state"`
	LeaseHolder          string                                   `json:"lease_holder,omitempty"`
	LeaseExpiresUnixNano int64                                    `json:"lease_expires_unix_nano,omitempty"`
	ExpiresUnixNano      int64                                    `json:"expires_unix_nano"`
}

func (a ReviewerAssignmentV1) digestValue() ReviewerAssignmentV1 { a.Digest = ""; return a }
func (a ReviewerAssignmentV1) validateShape() error {
	if err := a.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if invalidID(a.CaseID) || a.CaseRevision == 0 || invalidID(a.RoundID) || a.RoundRevision == 0 || a.RoundDigest.Validate() != nil || invalidID(a.TargetID) || a.TargetRevision == 0 || a.TargetDigest.Validate() != nil || invalidID(a.ReviewerID) || invalidText(a.Capability) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "reviewer assignment is incomplete")
	}
	if a.Route != RouteHumanV1 && a.Route != RouteAutoV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "assignment route is unsupported")
	}
	if err := a.ReviewerAuthority.Validate(); err != nil {
		return err
	}
	if err := a.ReviewerBinding.Validate(); err != nil {
		return err
	}
	switch a.State {
	case AssignmentOfferedV1, AssignmentClaimedV1, AssignmentReleasedV1, AssignmentExpiredV1, AssignmentRevokedV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "assignment state is unsupported")
	}
	if a.State == AssignmentClaimedV1 {
		if invalidID(a.LeaseHolder) || a.LeaseExpiresUnixNano <= a.UpdatedUnixNano || a.LeaseExpiresUnixNano > a.ExpiresUnixNano {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonStaleLeaseRevision, "claimed assignment requires a bounded lease")
		}
	} else if a.LeaseHolder != "" || a.LeaseExpiresUnixNano != 0 {
		return core.NewError(core.ErrorConflict, core.ReasonStaleLeaseRevision, "unclaimed assignment cannot retain a lease")
	}
	return ValidateExpires(a.CreatedUnixNano, a.ExpiresUnixNano)
}
func SealReviewerAssignmentV1(a ReviewerAssignmentV1) (ReviewerAssignmentV1, error) {
	a.ContractVersion = ContractVersionV1
	a.Digest = ""
	if err := a.validateShape(); err != nil {
		return ReviewerAssignmentV1{}, err
	}
	d, err := seal("ReviewerAssignmentV1", a.digestValue())
	if err != nil {
		return ReviewerAssignmentV1{}, err
	}
	a.Digest = d
	return a, a.Validate()
}
func (a ReviewerAssignmentV1) Validate() error {
	if err := a.validateShape(); err != nil {
		return err
	}
	return validateSealed("ReviewerAssignmentV1", a.digestValue(), a.Digest)
}

type ResolutionV1 string

const (
	ResolutionAcceptV1               ResolutionV1 = "accept"
	ResolutionConditionalV1          ResolutionV1 = "conditional_acceptance"
	ResolutionRequestChangesV1       ResolutionV1 = "request_changes"
	ResolutionEscalateHumanV1        ResolutionV1 = "escalate_human"
	ResolutionRejectV1               ResolutionV1 = "reject"
	ResolutionInsufficientEvidenceV1 ResolutionV1 = "insufficient_evidence"
)

type AttestationV1 struct {
	FactIdentityV1
	IdempotencyKey        string                                   `json:"idempotency_key"`
	CaseID                string                                   `json:"case_id"`
	CaseRevision          core.Revision                            `json:"case_revision"`
	RoundID               string                                   `json:"round_id"`
	RoundRevision         core.Revision                            `json:"round_revision"`
	RoundDigest           core.Digest                              `json:"round_digest"`
	AssignmentID          string                                   `json:"assignment_id"`
	AssignmentRevision    core.Revision                            `json:"assignment_revision"`
	AssignmentDigest      core.Digest                              `json:"assignment_digest"`
	TargetID              string                                   `json:"target_id"`
	TargetRevision        core.Revision                            `json:"target_revision"`
	TargetDigest          core.Digest                              `json:"target_digest"`
	ContextFrameDigest    core.Digest                              `json:"context_frame_digest"`
	Route                 RouteV1                                  `json:"route"`
	ReviewerID            string                                   `json:"reviewer_id"`
	ReviewerAuthority     runtimeports.AuthorityBindingRefV2       `json:"reviewer_authority"`
	ReviewerBinding       runtimeports.ReviewComponentBindingRefV2 `json:"reviewer_binding"`
	Resolution            ResolutionV1                             `json:"resolution"`
	ReasonCodes           []string                                 `json:"reason_codes"`
	FindingRefs           []string                                 `json:"finding_refs"`
	Evidence              []runtimeports.ReviewEvidenceRefV2       `json:"evidence"`
	EvidenceDigest        core.Digest                              `json:"evidence_digest"`
	Conditions            []runtimeports.ReviewConditionV2         `json:"conditions,omitempty"`
	ConditionsDigest      core.Digest                              `json:"conditions_digest,omitempty"`
	DomainApplySettlement *DomainApplySettlementRefV1              `json:"domain_apply_settlement,omitempty"`
	ReviewerAttemptID     string                                   `json:"reviewer_attempt_id,omitempty"`
	ReviewerResultDigest  core.Digest                              `json:"reviewer_result_digest,omitempty"`
	AutoProvenance        *AutoReviewerAttestationProvenanceV1     `json:"auto_provenance,omitempty"`
	ObservedUnixNano      int64                                    `json:"observed_unix_nano"`
	ExpiresUnixNano       int64                                    `json:"expires_unix_nano"`
}

// AutoReviewerAttestationProvenanceV1 preserves the exact Review-owned
// invocation closure that was truthfully applied through Runtime V4. It is
// audit provenance only and grants no dispatch or execution authority.
type AutoReviewerAttestationProvenanceV1 struct {
	Attempt     ExactResourceRefV1                     `json:"attempt"`
	Observation AutoReviewerInvocationObservationRefV1 `json:"observation"`
	Rubric      ExactResourceRefV1                     `json:"rubric"`
}

func (p AutoReviewerAttestationProvenanceV1) Validate() error {
	if err := p.Attempt.Validate(); err != nil {
		return err
	}
	if err := p.Observation.Validate(); err != nil {
		return err
	}
	return p.Rubric.Validate()
}

func (a AttestationV1) digestValue() AttestationV1 { a.Digest = ""; return a }
func (a AttestationV1) validateShape() error {
	if err := a.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if a.Revision != 1 || invalidID(a.IdempotencyKey) || invalidID(a.CaseID) || a.CaseRevision == 0 || invalidID(a.RoundID) || a.RoundRevision == 0 || a.RoundDigest.Validate() != nil || invalidID(a.AssignmentID) || a.AssignmentRevision == 0 || a.AssignmentDigest.Validate() != nil || invalidID(a.TargetID) || a.TargetRevision == 0 || a.TargetDigest.Validate() != nil || a.ContextFrameDigest.Validate() != nil || invalidID(a.ReviewerID) || a.ObservedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review attestation identity is incomplete")
	}
	if a.Route != RouteHumanV1 && a.Route != RouteAutoV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "attestation route is unsupported")
	}
	if err := a.ReviewerAuthority.Validate(); err != nil {
		return err
	}
	if err := a.ReviewerBinding.Validate(); err != nil {
		return err
	}
	switch a.Resolution {
	case ResolutionAcceptV1, ResolutionConditionalV1, ResolutionRequestChangesV1, ResolutionEscalateHumanV1, ResolutionRejectV1, ResolutionInsufficientEvidenceV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "attestation resolution is unsupported")
	}
	if len(a.ReasonCodes) == 0 || len(a.ReasonCodes) > MaxListItemsV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "attestation reason codes are empty or exceed their bound")
	}
	if !sort.StringsAreSorted(a.ReasonCodes) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "attestation reason codes must be sorted")
	}
	for i, v := range a.ReasonCodes {
		if invalidText(v) || (i > 0 && a.ReasonCodes[i-1] == v) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "attestation reason codes are invalid or duplicated")
		}
	}
	if len(a.FindingRefs) > MaxListItemsV1 || !sort.StringsAreSorted(a.FindingRefs) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "attestation finding refs must be bounded and sorted")
	}
	for i, v := range a.FindingRefs {
		if invalidID(v) || (i > 0 && a.FindingRefs[i-1] == v) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "attestation finding refs are invalid or duplicated")
		}
	}
	if len(a.Evidence) > MaxListItemsV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "attestation evidence exceeds its bound")
	}
	if !sort.SliceIsSorted(a.Evidence, func(i, j int) bool { return a.Evidence[i].Ref < a.Evidence[j].Ref }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "attestation evidence must be sorted")
	}
	for _, e := range a.Evidence {
		if err := e.Validate(); err != nil {
			return err
		}
	}
	if a.EvidenceDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidDigest, "attestation evidence digest is invalid")
	}
	expectedEvidence, err := ComputeReviewEvidenceDigestV1(a.Evidence)
	if err != nil {
		return err
	}
	if expectedEvidence != a.EvidenceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "attestation evidence digest drifted")
	}
	if a.Route == RouteAutoV1 {
		if a.DomainApplySettlement == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "auto attestation requires exact domain ApplySettlement")
		}
		if err := a.DomainApplySettlement.Validate(); err != nil {
			return err
		}
		if a.DomainApplySettlement.State != DomainApplyAppliedV1 || invalidID(a.ReviewerAttemptID) || a.ReviewerResultDigest.Validate() != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "auto attestation requires an applied settlement and exact reviewer result")
		}
		if a.AutoProvenance != nil {
			if err := a.AutoProvenance.Validate(); err != nil {
				return err
			}
		}
		if a.DomainApplySettlement.RuntimeContractVersion == runtimeports.OperationSettlementContractVersionV4 && a.AutoProvenance == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "Runtime V4 auto attestation requires exact Review invocation provenance")
		}
	} else if a.DomainApplySettlement != nil {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "human attestation cannot claim model invocation settlement")
	} else if a.ReviewerAttemptID != "" || a.ReviewerResultDigest != "" || a.AutoProvenance != nil {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "human attestation cannot claim reviewer invocation result")
	}
	if err := validateConditionsSetV2Compat(a.Conditions, a.ConditionsDigest, a.Resolution == ResolutionConditionalV1); err != nil {
		return err
	}
	if a.ObservedUnixNano < a.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "attestation observation predates creation")
	}
	return ValidateExpires(a.CreatedUnixNano, a.ExpiresUnixNano)
}
func SealAttestationV1(a AttestationV1) (AttestationV1, error) {
	a.ReasonCodes = append([]string(nil), a.ReasonCodes...)
	a.FindingRefs = append([]string(nil), a.FindingRefs...)
	a.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), a.Evidence...)
	a.Conditions = append([]runtimeports.ReviewConditionV2(nil), a.Conditions...)
	a.ContractVersion = ContractVersionV1
	a.Digest = ""
	sort.Slice(a.Evidence, func(i, j int) bool { return a.Evidence[i].Ref < a.Evidence[j].Ref })
	sortConditionsV2(a.Conditions)
	if len(a.Conditions) > 0 && a.ConditionsDigest == "" {
		var err error
		a.ConditionsDigest, err = runtimeports.DigestReviewConditionsV2(a.Conditions)
		if err != nil {
			return AttestationV1{}, err
		}
	}
	if err := validateConditionsSetV2(a.Conditions, a.ConditionsDigest, a.Resolution == ResolutionConditionalV1); err != nil {
		return AttestationV1{}, err
	}
	if err := a.validateShape(); err != nil {
		return AttestationV1{}, err
	}
	d, err := seal("AttestationV1", a.digestValue())
	if err != nil {
		return AttestationV1{}, err
	}
	a.Digest = d
	return a, a.Validate()
}

// ValidateProductionConditionsV2 rejects the legacy digest-only conditional
// shape. Historical V1 facts remain readable through Validate, but every new
// Owner mutation and current projection must call this strict validator.
func (a AttestationV1) ValidateProductionConditionsV2() error {
	if err := a.Validate(); err != nil {
		return err
	}
	if err := validateConditionsSetV2(a.Conditions, a.ConditionsDigest, a.Resolution == ResolutionConditionalV1); err != nil {
		return err
	}
	for _, condition := range a.Conditions {
		if condition.ExpiresUnixNano <= a.ObservedUnixNano || a.ExpiresUnixNano > condition.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "attestation exceeds an exact condition TTL")
		}
	}
	return nil
}

// ValidateProductionAutoProvenanceV4 is the strict production admission for
// an Attestation. Historical Runtime V3 Auto facts remain readable through
// Validate, but cannot be newly recorded, decided or projected as current
// authorization material. Production Auto requires the exact truthfully
// applied Runtime V4 inspection and Review-owned invocation provenance.
func (a AttestationV1) ValidateProductionAutoProvenanceV4() error {
	if err := a.ValidateProductionConditionsV2(); err != nil {
		return err
	}
	if a.Route != RouteAutoV1 {
		return nil
	}
	if a.DomainApplySettlement == nil || a.DomainApplySettlement.RuntimeContractVersion != runtimeports.OperationSettlementContractVersionV4 || a.DomainApplySettlement.RuntimeInspectionDigest.Validate() != nil || a.AutoProvenance == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "production Auto Attestation requires exact Runtime V4 settlement inspection and invocation provenance")
	}
	return a.AutoProvenance.Validate()
}
func (a AttestationV1) Validate() error {
	if err := a.validateShape(); err != nil {
		return err
	}
	return validateSealed("AttestationV1", a.digestValue(), a.Digest)
}

type FindingStatusV1 string

const (
	FindingOpenV1               FindingStatusV1 = "open"
	FindingAddressedCandidateV1 FindingStatusV1 = "addressed_candidate"
	FindingSupersededV1         FindingStatusV1 = "superseded"
	FindingDismissedV1          FindingStatusV1 = "dismissed_with_authority"
)

type FindingV1 struct {
	FactIdentityV1
	CaseID          string                             `json:"case_id"`
	CaseRevision    core.Revision                      `json:"case_revision"`
	RoundID         string                             `json:"round_id"`
	RoundRevision   core.Revision                      `json:"round_revision"`
	RoundDigest     core.Digest                        `json:"round_digest"`
	TargetID        string                             `json:"target_id"`
	TargetRevision  core.Revision                      `json:"target_revision"`
	TargetDigest    core.Digest                        `json:"target_digest"`
	Category        string                             `json:"category"`
	Priority        string                             `json:"priority"`
	Anchor          string                             `json:"anchor"`
	Claim           string                             `json:"claim"`
	Impact          string                             `json:"impact"`
	Evidence        []runtimeports.ReviewEvidenceRefV2 `json:"evidence"`
	Status          FindingStatusV1                    `json:"status"`
	ExpiresUnixNano int64                              `json:"expires_unix_nano"`
}

func (f FindingV1) digestValue() FindingV1 { f.Digest = ""; return f }
func (f FindingV1) validateShape() error {
	if err := f.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if f.Revision != 1 || invalidID(f.CaseID) || f.CaseRevision == 0 || invalidID(f.RoundID) || f.RoundRevision == 0 || f.RoundDigest.Validate() != nil || invalidID(f.TargetID) || f.TargetRevision == 0 || f.TargetDigest.Validate() != nil || invalidText(f.Category) || invalidText(f.Priority) || invalidText(f.Anchor) || invalidText(f.Claim) || invalidText(f.Impact) || len(f.Evidence) == 0 || len(f.Evidence) > MaxListItemsV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review finding is incomplete")
	}
	for _, e := range f.Evidence {
		if err := e.Validate(); err != nil {
			return err
		}
	}
	if !sort.SliceIsSorted(f.Evidence, func(i, j int) bool { return f.Evidence[i].Ref < f.Evidence[j].Ref }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "finding evidence must be sorted")
	}
	switch f.Status {
	case FindingOpenV1, FindingAddressedCandidateV1, FindingSupersededV1, FindingDismissedV1:
		return ValidateExpires(f.CreatedUnixNano, f.ExpiresUnixNano)
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "finding status is unsupported")
	}
}
func SealFindingV1(f FindingV1) (FindingV1, error) {
	f.ContractVersion = ContractVersionV1
	f.Digest = ""
	sort.Slice(f.Evidence, func(i, j int) bool { return f.Evidence[i].Ref < f.Evidence[j].Ref })
	if err := f.validateShape(); err != nil {
		return FindingV1{}, err
	}
	d, err := seal("FindingV1", f.digestValue())
	if err != nil {
		return FindingV1{}, err
	}
	f.Digest = d
	return f, f.Validate()
}
func (f FindingV1) Validate() error {
	if err := f.validateShape(); err != nil {
		return err
	}
	return validateSealed("FindingV1", f.digestValue(), f.Digest)
}

func ComputeFindingSetDigestV1(findings []FindingV1) (core.Digest, error) {
	if len(findings) > MaxListItemsV1 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "finding set exceeds its bound")
	}
	copyValue := append([]FindingV1{}, findings...)
	sort.Slice(copyValue, func(i, j int) bool { return copyValue[i].ID < copyValue[j].ID })
	type findingRef struct {
		ID     string      `json:"id"`
		Digest core.Digest `json:"digest"`
	}
	refs := make([]findingRef, 0, len(copyValue))
	for i, finding := range copyValue {
		if err := finding.Validate(); err != nil {
			return "", err
		}
		if i > 0 && copyValue[i-1].ID == finding.ID {
			return "", core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "finding id is duplicated")
		}
		refs = append(refs, findingRef{ID: finding.ID, Digest: finding.Digest})
	}
	return seal("FindingSetV1", refs)
}

type TraceEventV1 string

const (
	TraceRequestedV1  TraceEventV1 = "requested"
	TraceAdmittedV1   TraceEventV1 = "admitted"
	TraceRoutedV1     TraceEventV1 = "routed"
	TraceAssignedV1   TraceEventV1 = "assigned"
	TraceStartedV1    TraceEventV1 = "review_started"
	TraceFindingV1    TraceEventV1 = "finding_observed"
	TraceAttestedV1   TraceEventV1 = "attestation_recorded"
	TraceEscalatedV1  TraceEventV1 = "escalated"
	TraceVerdictV1    TraceEventV1 = "verdict_recorded"
	TraceExpiredV1    TraceEventV1 = "expired"
	TraceRevokedV1    TraceEventV1 = "revoked"
	TraceCancelledV1  TraceEventV1 = "cancelled"
	TraceSupersededV1 TraceEventV1 = "superseded"
	TraceResolvedV1   TraceEventV1 = "resolved"
)

type TraceFactV1 struct {
	FactIdentityV1
	CaseID         string        `json:"case_id"`
	CaseRevision   core.Revision `json:"case_revision"`
	TargetID       string        `json:"target_id"`
	TargetRevision core.Revision `json:"target_revision"`
	TargetDigest   core.Digest   `json:"target_digest"`
	Event          TraceEventV1  `json:"event"`
	SourceID       string        `json:"source_id"`
	SourceEpoch    core.Epoch    `json:"source_epoch"`
	SourceSequence uint64        `json:"source_sequence"`
	CausationID    string        `json:"causation_id"`
	CorrelationID  string        `json:"correlation_id"`
	FactRefs       []string      `json:"fact_refs"`
}

func (t TraceFactV1) digestValue() TraceFactV1 { t.Digest = ""; return t }
func (t TraceFactV1) validateShape() error {
	if err := t.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if t.Revision != 1 || invalidID(t.CaseID) || t.CaseRevision == 0 || invalidID(t.TargetID) || t.TargetRevision == 0 || t.TargetDigest.Validate() != nil || invalidID(t.SourceID) || t.SourceEpoch == 0 || t.SourceSequence == 0 || invalidID(t.CausationID) || invalidID(t.CorrelationID) || len(t.FactRefs) > MaxListItemsV1 || !sort.StringsAreSorted(t.FactRefs) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review trace fact is incomplete")
	}
	for i, ref := range t.FactRefs {
		if invalidID(ref) || (i > 0 && t.FactRefs[i-1] == ref) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "review trace fact refs are invalid or duplicated")
		}
	}
	switch t.Event {
	case TraceRequestedV1, TraceAdmittedV1, TraceRoutedV1, TraceAssignedV1, TraceStartedV1, TraceFindingV1, TraceAttestedV1, TraceEscalatedV1, TraceVerdictV1, TraceExpiredV1, TraceRevokedV1, TraceCancelledV1, TraceSupersededV1, TraceResolvedV1:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "review trace event is unsupported")
	}
}
func SealTraceFactV1(t TraceFactV1) (TraceFactV1, error) {
	t.ContractVersion = ContractVersionV1
	t.Digest = ""
	if err := t.validateShape(); err != nil {
		return TraceFactV1{}, err
	}
	d, err := seal("TraceFactV1", t.digestValue())
	if err != nil {
		return TraceFactV1{}, err
	}
	t.Digest = d
	return t, t.Validate()
}
func (t TraceFactV1) Validate() error {
	if err := t.validateShape(); err != nil {
		return err
	}
	return validateSealed("TraceFactV1", t.digestValue(), t.Digest)
}

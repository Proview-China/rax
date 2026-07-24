package contract

import (
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const DecisionCurrentContractV1 = "praxis.review/decision-current-v1"

// ReviewerBindingCurrentV1 is a Review-owned read model over the exact public
// ReviewComponentBindingRefV2. It is not a Binding fact and grants no
// capability. A Binding Owner adapter must populate it from an exact current
// Inspect; Review has no production adapter for that missing public seam.
type ReviewerBindingCurrentV1 struct {
	Binding          runtimeports.ReviewComponentBindingRefV2  `json:"binding"`
	ProjectionRef    runtimeports.ReviewBindingProjectionRefV1 `json:"projection_ref,omitempty"`
	CheckedUnixNano  int64                                     `json:"checked_unix_nano,omitempty"`
	ProjectionDigest core.Digest                               `json:"projection_digest,omitempty"`
	Current          bool                                      `json:"current"`
	ExpiresUnixNano  int64                                     `json:"expires_unix_nano"`
}

func (v ReviewerBindingCurrentV1) Validate(expected runtimeports.ReviewComponentBindingRefV2, now time.Time) error {
	if err := v.Binding.Validate(); err != nil {
		return err
	}
	if !v.Current || v.Binding != expected || now.IsZero() || v.ExpiresUnixNano <= 0 || !now.Before(time.Unix(0, v.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "reviewer Binding is absent, expired or drifted")
	}
	if v.ProjectionRef != (runtimeports.ReviewBindingProjectionRefV1{}) {
		if v.ProjectionRef.Validate() != nil || v.CheckedUnixNano <= 0 || v.CheckedUnixNano >= v.ExpiresUnixNano || v.ProjectionDigest != v.ProjectionRef.Digest || now.UnixNano() < v.CheckedUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "reviewer Binding exact-current proof is incomplete or stale")
		}
	}
	return nil
}

// DecisionEvidenceCurrentV1 binds one exact Review evidence ref to an
// independently inspected Evidence Owner fact and its current lifetime. It is
// read-only and is not Evidence admission or a new authoritative Evidence fact.
type DecisionEvidenceCurrentV1 struct {
	Review           runtimeports.ReviewEvidenceRefV2              `json:"review"`
	ApplicabilityRef runtimeports.ReviewEvidenceApplicabilityRefV1 `json:"applicability_ref,omitempty"`
	Record           runtimeports.EvidenceRecordRefV2              `json:"record,omitempty"`
	OwnerFact        runtimeports.EvidenceOwnerFactRefV2           `json:"owner_fact,omitempty"`
	CheckedUnixNano  int64                                         `json:"checked_unix_nano,omitempty"`
	ProjectionDigest core.Digest                                   `json:"projection_digest,omitempty"`
	Current          bool                                          `json:"current"`
	ExpiresUnixNano  int64                                         `json:"expires_unix_nano"`
}

func (v DecisionEvidenceCurrentV1) Validate(now time.Time) error {
	if err := v.Review.Validate(); err != nil {
		return err
	}
	if !v.Current || now.IsZero() || v.ExpiresUnixNano <= 0 || !now.Before(time.Unix(0, v.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "review Evidence is absent, expired or not current")
	}
	if v.ApplicabilityRef != (runtimeports.ReviewEvidenceApplicabilityRefV1{}) {
		if v.ApplicabilityRef.Validate() != nil || v.Record.Validate() != nil || v.CheckedUnixNano <= 0 || v.CheckedUnixNano >= v.ExpiresUnixNano || v.ProjectionDigest != v.ApplicabilityRef.Digest || now.UnixNano() < v.CheckedUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "review Evidence exact-current proof is incomplete or stale")
		}
		if v.OwnerFact != (runtimeports.EvidenceOwnerFactRefV2{}) {
			return v.OwnerFact.Validate()
		}
		return nil
	}
	// Legacy reference/test fixtures predate the public applicability Reader.
	// Production composition must always populate ApplicabilityRef.
	if err := v.OwnerFact.Validate(); err != nil {
		return err
	}
	return nil
}

// DecisionExternalCurrentProofV1 preserves the exact immutable projections
// that were S1/S2-validated by the independently owned public Readers. It is
// evidence of the read cut only; it grants no Authority, Permit or execution.
type DecisionExternalCurrentProofV1 struct {
	Policy            runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1    `json:"policy"`
	ActorAuthority    runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1 `json:"actor_authority"`
	ReviewerAuthority runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1 `json:"reviewer_authority"`
	Scope             runtimeports.ReviewDecisionScopeCurrentProjectionRefV1     `json:"scope"`
	Binding           runtimeports.ReviewBindingProjectionRefV1                  `json:"binding"`
	Digest            core.Digest                                                `json:"digest"`
}

func (p DecisionExternalCurrentProofV1) digestValue() DecisionExternalCurrentProofV1 {
	p.Digest = ""
	return p
}

func (p DecisionExternalCurrentProofV1) DigestV1() (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.review.decision-external-current", DecisionCurrentContractV1, "DecisionExternalCurrentProofV1", p.digestValue())
}

func SealDecisionExternalCurrentProofV1(p DecisionExternalCurrentProofV1) (DecisionExternalCurrentProofV1, error) {
	for _, err := range []error{p.Policy.Validate(), p.ActorAuthority.Validate(), p.ReviewerAuthority.Validate(), p.Scope.Validate(), p.Binding.Validate()} {
		if err != nil {
			return DecisionExternalCurrentProofV1{}, err
		}
	}
	p.Digest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return DecisionExternalCurrentProofV1{}, err
	}
	p.Digest = digest
	return p, nil
}

func (p DecisionExternalCurrentProofV1) Validate() error {
	for _, err := range []error{p.Policy.Validate(), p.ActorAuthority.Validate(), p.ReviewerAuthority.Validate(), p.Scope.Validate(), p.Binding.Validate(), p.Digest.Validate()} {
		if err != nil {
			return err
		}
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "decision external current proof digest drifted")
	}
	return nil
}

// DecisionCurrentSnapshotV1 is one immutable, linearized read model. Review
// facts are exact stored facts; Policy/Authority/Scope/Binding/Evidence are
// read-only projections supplied by their public Owner adapters. Digest proves
// snapshot integrity only and is never Runtime authority.
type DecisionCurrentSnapshotV1 struct {
	Revision          core.Revision                             `json:"revision"`
	Target            TargetSnapshotV1                          `json:"target"`
	Case              ReviewCaseV1                              `json:"case"`
	Round             ReviewRoundV1                             `json:"round"`
	Rubric            RubricDefinitionV1                        `json:"rubric"`
	Assignment        ReviewerAssignmentV1                      `json:"assignment"`
	Attestation       AttestationV1                             `json:"attestation"`
	Findings          []FindingV1                               `json:"findings"`
	ApplySettlement   *DomainApplySettlementFactV1              `json:"apply_settlement,omitempty"`
	DomainResult      *ReviewerInvocationResultFactV1           `json:"domain_result,omitempty"`
	Policy            runtimeports.ReviewPolicyFactV2           `json:"policy"`
	ActorAuthority    runtimeports.OperationGovernanceFactRefV3 `json:"actor_authority"`
	ReviewerAuthority runtimeports.OperationGovernanceFactRefV3 `json:"reviewer_authority"`
	Scope             runtimeports.OperationGovernanceFactRefV3 `json:"scope"`
	Binding           ReviewerBindingCurrentV1                  `json:"binding"`
	Evidence          []DecisionEvidenceCurrentV1               `json:"evidence"`
	ExternalProof     *DecisionExternalCurrentProofV1           `json:"external_proof,omitempty"`
	Current           bool                                      `json:"current"`
	ExpiresUnixNano   int64                                     `json:"expires_unix_nano"`
	Digest            core.Digest                               `json:"digest"`
}

func (s DecisionCurrentSnapshotV1) digestValue() DecisionCurrentSnapshotV1 {
	s.Digest = ""
	s.Findings = append([]FindingV1(nil), s.Findings...)
	s.Evidence = append([]DecisionEvidenceCurrentV1(nil), s.Evidence...)
	if s.ExternalProof != nil {
		proof := *s.ExternalProof
		s.ExternalProof = &proof
	}
	sort.Slice(s.Findings, func(i, j int) bool { return s.Findings[i].ID < s.Findings[j].ID })
	sort.Slice(s.Evidence, func(i, j int) bool { return s.Evidence[i].Review.Ref < s.Evidence[j].Review.Ref })
	return s
}

func (s DecisionCurrentSnapshotV1) DigestV1() (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.review.decision-current", DecisionCurrentContractV1, "DecisionCurrentSnapshotV1", s.digestValue())
}

func SealDecisionCurrentSnapshotV1(s DecisionCurrentSnapshotV1) (DecisionCurrentSnapshotV1, error) {
	s = s.digestValue()
	digest, err := s.DigestV1()
	if err != nil {
		return DecisionCurrentSnapshotV1{}, err
	}
	s.Digest = digest
	return s, nil
}

func (s DecisionCurrentSnapshotV1) ValidateEnvelope(now time.Time) error {
	if s.Revision == 0 || !s.Current || now.IsZero() || s.ExpiresUnixNano <= 0 || !now.Before(time.Unix(0, s.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "decision current snapshot is absent or expired")
	}
	if err := s.Digest.Validate(); err != nil {
		return err
	}
	digest, err := s.DigestV1()
	if err != nil || digest != s.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "decision current snapshot digest drifted")
	}
	return nil
}

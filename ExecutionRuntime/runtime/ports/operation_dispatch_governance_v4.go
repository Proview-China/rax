package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const OperationDispatchGovernanceContractVersionV4 = "4.0.0"

// OperationAuthorizedAdmissionV4 is the post-Review dispatch admission. The
// earlier V3 Intent admission remains immutable and is deliberately not
// rewritten after Review completes.
type OperationAuthorizedAdmissionV4 struct {
	ContractVersion              string                            `json:"contract_version"`
	Admission                    OperationEffectAdmissionReceiptV3 `json:"effect_admission"`
	Authorization                OperationReviewAuthorizationRefV4 `json:"review_authorization"`
	PayloadSchema                SchemaRefV2                       `json:"payload_schema"`
	PayloadDigest                core.Digest                       `json:"payload_digest"`
	PayloadRevision              core.Revision                     `json:"payload_revision"`
	ReviewProjectionDigest       core.Digest                       `json:"review_projection_digest"`
	ReviewCurrentnessDigest      core.Digest                       `json:"review_currentness_digest"`
	LegacyReviewProjectionDigest core.Digest                       `json:"legacy_review_projection_digest"`
	GovernanceSnapshotDigest     core.Digest                       `json:"governance_snapshot_digest"`
	AuthorizationFenceDigest     core.Digest                       `json:"authorization_fence_digest"`
	ExpiresUnixNano              int64                             `json:"expires_unix_nano"`
	Digest                       core.Digest                       `json:"digest"`
}

func (a OperationAuthorizedAdmissionV4) Validate() error {
	if a.ContractVersion != OperationDispatchGovernanceContractVersionV4 || a.PayloadRevision == 0 || a.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectAuthorizationMissing, "V4 dispatch admission identity and TTL are incomplete")
	}
	if err := a.Admission.Validate(); err != nil {
		return err
	}
	if err := a.Authorization.Validate(); err != nil {
		return err
	}
	if err := a.PayloadSchema.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{a.PayloadDigest, a.ReviewProjectionDigest, a.ReviewCurrentnessDigest, a.LegacyReviewProjectionDigest, a.GovernanceSnapshotDigest, a.AuthorizationFenceDigest, a.Digest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	digest, err := a.DigestV4()
	if err != nil || digest != a.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V4 dispatch admission digest drifted")
	}
	return nil
}

func (a OperationAuthorizedAdmissionV4) DigestV4() (core.Digest, error) {
	copy := a
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-governance", OperationDispatchGovernanceContractVersionV4, "OperationAuthorizedAdmissionV4", copy)
}

func SealOperationAuthorizedAdmissionV4(a OperationAuthorizedAdmissionV4) (OperationAuthorizedAdmissionV4, error) {
	a.ContractVersion = OperationDispatchGovernanceContractVersionV4
	a.Digest = ""
	digest, err := a.DigestV4()
	if err != nil {
		return OperationAuthorizedAdmissionV4{}, err
	}
	a.Digest = digest
	return a, a.Validate()
}

func DigestOperationReviewAuthorizationV3(value OperationReviewAuthorizationV3) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.runtime.operation-effect", OperationEffectContractVersionV3, "OperationReviewAuthorizationV3", value)
}

func SameOperationReviewAuthorizationV3(left, right OperationReviewAuthorizationV3) bool {
	leftDigest, leftErr := DigestOperationReviewAuthorizationV3(left)
	rightDigest, rightErr := DigestOperationReviewAuthorizationV3(right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

// OperationReviewAuthorizationV3Covers reports whether a current V3 review
// projection preserves every authoritative source bound by an authorization
// projection. The current aggregate TTL may be later because the V4
// Authorization deliberately narrows it; no source ref or source TTL may
// otherwise drift.
func OperationReviewAuthorizationV3Covers(current, authorized OperationReviewAuthorizationV3) bool {
	if current.ExpiresUnixNano < authorized.ExpiresUnixNano {
		return false
	}
	current.ExpiresUnixNano = authorized.ExpiresUnixNano
	return SameOperationReviewAuthorizationV3(current, authorized)
}

func (a OperationAuthorizedAdmissionV4) ValidateAgainstAuthorization(fact OperationReviewAuthorizationFactV4, now time.Time) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if err := fact.Validate(); err != nil {
		return err
	}
	if fact.State != OperationReviewAuthorizationActiveV4 || now.IsZero() || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "V4 Review Authorization is not current")
	}
	operationDigest, err := fact.Intent.Operation.DigestV3()
	if err != nil {
		return err
	}
	if a.Authorization != fact.RefV4() || a.Admission.OperationDigest != operationDigest || a.Admission.EffectID != fact.Intent.IntentID || a.Admission.IntentRevision != fact.Intent.IntentRevision || a.Admission.IntentDigest != fact.Intent.IntentDigest || a.Admission.FactRevision != fact.Intent.EffectFactRevision || a.PayloadSchema != fact.Intent.PayloadSchema || a.PayloadDigest != fact.Intent.PayloadDigest || a.PayloadRevision != fact.Intent.PayloadRevision || a.ReviewProjectionDigest != fact.Review.ProjectionDigest || a.ReviewCurrentnessDigest != fact.Review.CurrentnessDigest || a.GovernanceSnapshotDigest != fact.Governance.SnapshotDigest || a.AuthorizationFenceDigest != fact.FenceDigest || a.ExpiresUnixNano != fact.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "V4 dispatch admission does not bind the exact accepted Effect and Review Authorization")
	}
	return nil
}

// OperationDispatchPermitV4 wraps the complete V3 transport projection while
// making the immutable V4 Review Authorization Fact part of the Permit digest.
// The nested V3 value is compatibility data, not a substitute for V4 current
// inspection.
type OperationDispatchPermitV4 struct {
	ContractVersion string                         `json:"contract_version"`
	LegacyPermit    OperationDispatchPermitV3      `json:"legacy_permit_v3"`
	Admission       OperationAuthorizedAdmissionV4 `json:"dispatch_admission"`
	Digest          core.Digest                    `json:"digest"`
}

func (p OperationDispatchPermitV4) Validate() error {
	if p.ContractVersion != OperationDispatchGovernanceContractVersionV4 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V4 operation Permit contract version is invalid")
	}
	if err := p.LegacyPermit.Validate(); err != nil {
		return err
	}
	if err := p.Admission.Validate(); err != nil {
		return err
	}
	legacyReviewDigest, err := DigestOperationReviewAuthorizationV3(p.LegacyPermit.ReviewAuthorization)
	if err != nil {
		return err
	}
	if p.LegacyPermit.IntentID != p.Admission.Admission.EffectID || p.LegacyPermit.IntentRevision != p.Admission.Admission.IntentRevision || p.LegacyPermit.IntentDigest != p.Admission.Admission.IntentDigest || p.LegacyPermit.PayloadSchema != p.Admission.PayloadSchema || p.LegacyPermit.PayloadDigest != p.Admission.PayloadDigest || p.LegacyPermit.PayloadRevision != p.Admission.PayloadRevision || p.LegacyPermit.GovernanceSnapshotDigest != p.Admission.GovernanceSnapshotDigest || legacyReviewDigest != p.Admission.LegacyReviewProjectionDigest || p.LegacyPermit.ExpiresUnixNano > p.Admission.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V4 operation Permit does not bind one exact dispatch admission")
	}
	digest, err := p.DigestV4()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V4 operation Permit digest drifted")
	}
	return nil
}

func (p OperationDispatchPermitV4) DigestV4() (core.Digest, error) {
	copy := p
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-governance", OperationDispatchGovernanceContractVersionV4, "OperationDispatchPermitV4", copy)
}

func SealOperationDispatchPermitV4(p OperationDispatchPermitV4) (OperationDispatchPermitV4, error) {
	p.ContractVersion = OperationDispatchGovernanceContractVersionV4
	p.Digest = ""
	digest, err := p.DigestV4()
	if err != nil {
		return OperationDispatchPermitV4{}, err
	}
	p.Digest = digest
	return p, p.Validate()
}

func (p OperationDispatchPermitV4) ValidateAgainstAuthorization(fact OperationReviewAuthorizationFactV4, fence core.ExecutionFence, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := p.Admission.ValidateAgainstAuthorization(fact, now); err != nil {
		return err
	}
	legacy, err := fact.CompatibilityProjectionV3(now)
	if err != nil {
		return err
	}
	if !OperationReviewAuthorizationV3Covers(p.LegacyPermit.ReviewAuthorization, legacy) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "V4 Permit legacy projection does not come from the exact Authorization")
	}
	if p.LegacyPermit.GovernanceSnapshotDigest != fact.Governance.SnapshotDigest || p.LegacyPermit.CapabilityGrantDigest != fact.Governance.CapabilityGrantDigest || p.LegacyPermit.CredentialGrantDigest != fact.Governance.CredentialGrantDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "V4 Permit governance grants drifted from Review Authorization")
	}
	if err := validateOperationDispatchFenceV4(fence, fact.Fence); err != nil {
		return err
	}
	return nil
}

func validateOperationDispatchFenceV4(permit, authorization core.ExecutionFence) error {
	if err := permit.Validate(); err != nil {
		return err
	}
	if err := authorization.Validate(); err != nil {
		return err
	}
	if permit.BoundaryScope != authorization.BoundaryScope || !SameExecutionScopeV2(permit.Scope, authorization.Scope) || permit.CapabilityGrantDigest != authorization.CapabilityGrantDigest || permit.EffectIntentID != authorization.EffectIntentID || permit.EffectIntentRevision != authorization.EffectIntentRevision || permit.CanonicalPayloadDigest != authorization.CanonicalPayloadDigest || permit.ExpiresAt.After(authorization.ExpiresAt) {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "V4 Permit Fence widened or drifted from Review Authorization")
	}
	return nil
}

type OperationDispatchPermitStateV4 string

const (
	OperationPermitIssuedV4  OperationDispatchPermitStateV4 = "issued"
	OperationPermitBegunV4   OperationDispatchPermitStateV4 = "begun"
	OperationPermitExpiredV4 OperationDispatchPermitStateV4 = "expired"
	OperationPermitRevokedV4 OperationDispatchPermitStateV4 = "revoked"
)

// OperationDispatchEnforcementRefV4 reserves the exact future enforcement
// binding. This delta does not provide a Provider Prepare/Execute path and
// therefore never populates it.
type OperationDispatchEnforcementRefV4 struct {
	PermitID            string                            `json:"permit_id"`
	PermitRevision      core.Revision                     `json:"permit_revision"`
	PermitDigest        core.Digest                       `json:"permit_digest"`
	AttemptID           string                            `json:"attempt_id"`
	ReviewAuthorization OperationReviewAuthorizationRefV4 `json:"review_authorization"`
	ReceiptDigest       core.Digest                       `json:"receipt_digest"`
	RecordedRevision    core.Revision                     `json:"recorded_revision"`
}

func (r OperationDispatchEnforcementRefV4) Validate() error {
	if validateEvidenceIDV2(r.PermitID) != nil || r.PermitRevision == 0 || validateEvidenceIDV2(r.AttemptID) != nil || r.RecordedRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V4 Enforcement ref identity is incomplete")
	}
	if err := r.PermitDigest.Validate(); err != nil {
		return err
	}
	if err := r.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	return r.ReceiptDigest.Validate()
}

// OperationDispatchRecordV4 is historical persisted truth. Its existence does
// not prove that Review or any other governance source remains current.
type OperationDispatchRecordV4 struct {
	Permit             OperationDispatchPermitV4          `json:"permit"`
	PermitDigest       core.Digest                        `json:"permit_digest"`
	Fence              core.ExecutionFence                `json:"fence"`
	State              OperationDispatchPermitStateV4     `json:"state"`
	Revision           core.Revision                      `json:"revision"`
	EffectFactRevision core.Revision                      `json:"effect_fact_revision"`
	BegunUnixNano      int64                              `json:"begun_unix_nano,omitempty"`
	Enforcement        *OperationDispatchEnforcementRefV4 `json:"enforcement,omitempty"`
	Digest             core.Digest                        `json:"digest"`
}

func (r OperationDispatchRecordV4) Validate() error {
	if err := r.Permit.Validate(); err != nil {
		return err
	}
	if r.Revision == 0 || r.EffectFactRevision == 0 || r.PermitDigest != r.Permit.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "V4 Permit record identity or digest drifted")
	}
	fenceDigest, err := DigestOperationExecutionFenceV3(r.Fence, r.Permit.LegacyPermit.Operation)
	if err != nil || fenceDigest != r.Permit.LegacyPermit.FenceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "V4 Permit record Fence drifted")
	}
	switch r.State {
	case OperationPermitIssuedV4:
		if r.BegunUnixNano != 0 || r.Enforcement != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "issued V4 Permit carries later facts")
		}
	case OperationPermitBegunV4:
		if r.BegunUnixNano <= 0 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "begun V4 Permit lacks begin time")
		}
	case OperationPermitExpiredV4, OperationPermitRevokedV4:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V4 Permit state is invalid")
	}
	if r.Enforcement != nil {
		if err := r.Enforcement.Validate(); err != nil {
			return err
		}
		legacy := r.Permit.LegacyPermit
		if r.Enforcement.PermitID != legacy.ID || r.Enforcement.PermitRevision != legacy.Revision || r.Enforcement.PermitDigest != r.PermitDigest || r.Enforcement.AttemptID != legacy.AttemptID || r.Enforcement.ReviewAuthorization != r.Permit.Admission.Authorization || r.Enforcement.RecordedRevision != r.Revision {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V4 Enforcement ref belongs to another Permit or Authorization")
		}
	}
	digest, err := r.DigestV4()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V4 Permit record digest drifted")
	}
	return nil
}

func (r OperationDispatchRecordV4) DigestV4() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-governance", OperationDispatchGovernanceContractVersionV4, "OperationDispatchRecordV4", copy)
}

func SealOperationDispatchRecordV4(r OperationDispatchRecordV4) (OperationDispatchRecordV4, error) {
	r.Digest = ""
	digest, err := r.DigestV4()
	if err != nil {
		return OperationDispatchRecordV4{}, err
	}
	r.Digest = digest
	return r, r.Validate()
}

// CurrentOperationDispatchAuthorizationV4 is a checked point-in-time envelope.
// It is neither an Enforcement receipt nor proof that a later Provider call is
// still authorized.
type CurrentOperationDispatchAuthorizationV4 struct {
	Record                   OperationDispatchRecordV4         `json:"record"`
	ReviewAuthorization      OperationReviewAuthorizationRefV4 `json:"review_authorization"`
	ReviewProjectionDigest   core.Digest                       `json:"review_projection_digest"`
	ReviewCurrentnessDigest  core.Digest                       `json:"review_currentness_digest"`
	GovernanceSnapshotDigest core.Digest                       `json:"governance_snapshot_digest"`
	CheckedUnixNano          int64                             `json:"checked_unix_nano"`
}

func (a CurrentOperationDispatchAuthorizationV4) Validate() error {
	if err := a.Record.Validate(); err != nil {
		return err
	}
	if err := a.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{a.ReviewProjectionDigest, a.ReviewCurrentnessDigest, a.GovernanceSnapshotDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	admission := a.Record.Permit.Admission
	if a.CheckedUnixNano <= 0 || (a.Record.State != OperationPermitIssuedV4 && a.Record.State != OperationPermitBegunV4) || a.ReviewAuthorization != admission.Authorization || a.ReviewProjectionDigest != admission.ReviewProjectionDigest || a.ReviewCurrentnessDigest != admission.ReviewCurrentnessDigest || a.GovernanceSnapshotDigest != admission.GovernanceSnapshotDigest || a.CheckedUnixNano >= a.Record.Permit.LegacyPermit.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "current V4 dispatch envelope drifted from persisted authorization")
	}
	return nil
}

type IssueGovernedOperationDispatchRequestV4 struct {
	Operation              OperationSubjectV3                `json:"operation"`
	EffectID               core.EffectIntentID               `json:"effect_id"`
	ExpectedEffectRevision core.Revision                     `json:"expected_effect_revision"`
	Admission              OperationEffectAdmissionReceiptV3 `json:"effect_admission"`
	ReviewAuthorization    OperationReviewAuthorizationRefV4 `json:"review_authorization"`
	PermitID               string                            `json:"permit_id"`
	AttemptID              string                            `json:"attempt_id"`
	PermitTTL              time.Duration                     `json:"permit_ttl"`
}

func (r IssueGovernedOperationDispatchRequestV4) Validate() error {
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.EffectID)) != nil || r.ExpectedEffectRevision == 0 || validateEvidenceIDV2(r.PermitID) != nil || validateEvidenceIDV2(r.AttemptID) != nil || r.PermitTTL <= 0 || r.PermitTTL > MaxDispatchPermitTTL {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V4 Issue requires exact Effect, Permit, attempt and bounded TTL")
	}
	if err := r.Admission.Validate(); err != nil {
		return err
	}
	return r.ReviewAuthorization.Validate()
}

type InspectOperationDispatchRecordRequestV4 struct {
	Operation OperationSubjectV3  `json:"operation"`
	EffectID  core.EffectIntentID `json:"effect_id"`
	PermitID  string              `json:"permit_id"`
}

func (r InspectOperationDispatchRecordRequestV4) Validate() error {
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.EffectID)) != nil || validateEvidenceIDV2(r.PermitID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V4 dispatch Inspect requires Effect and Permit identities")
	}
	return nil
}

type InspectCurrentOperationDispatchRequestV4 struct {
	Inspect             InspectOperationDispatchRecordRequestV4 `json:"inspect"`
	AdmissionDigest     core.Digest                             `json:"admission_digest"`
	ReviewAuthorization OperationReviewAuthorizationRefV4       `json:"review_authorization"`
}

func (r InspectCurrentOperationDispatchRequestV4) Validate() error {
	if err := r.Inspect.Validate(); err != nil {
		return err
	}
	if err := r.AdmissionDigest.Validate(); err != nil {
		return err
	}
	return r.ReviewAuthorization.Validate()
}

type BeginGovernedOperationDispatchRequestV4 struct {
	Operation                  OperationSubjectV3                `json:"operation"`
	EffectID                   core.EffectIntentID               `json:"effect_id"`
	ExpectedEffectRevision     core.Revision                     `json:"expected_effect_revision"`
	PermitID                   string                            `json:"permit_id"`
	ExpectedPermitFactRevision core.Revision                     `json:"expected_permit_fact_revision"`
	AdmissionDigest            core.Digest                       `json:"admission_digest"`
	ReviewAuthorization        OperationReviewAuthorizationRefV4 `json:"review_authorization"`
}

func (r BeginGovernedOperationDispatchRequestV4) Validate() error {
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.EffectID)) != nil || r.ExpectedEffectRevision == 0 || validateEvidenceIDV2(r.PermitID) != nil || r.ExpectedPermitFactRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V4 Begin requires exact Effect and Permit Fact revisions")
	}
	if err := r.AdmissionDigest.Validate(); err != nil {
		return err
	}
	return r.ReviewAuthorization.Validate()
}

type OperationGovernancePortV4 interface {
	IssueOperationDispatchV4(context.Context, IssueGovernedOperationDispatchRequestV4) (CurrentOperationDispatchAuthorizationV4, error)
	InspectOperationDispatchRecordV4(context.Context, InspectOperationDispatchRecordRequestV4) (OperationDispatchRecordV4, error)
	InspectCurrentOperationDispatchV4(context.Context, InspectCurrentOperationDispatchRequestV4) (CurrentOperationDispatchAuthorizationV4, error)
	BeginOperationDispatchV4(context.Context, BeginGovernedOperationDispatchRequestV4) (CurrentOperationDispatchAuthorizationV4, error)
}

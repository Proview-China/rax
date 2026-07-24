package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const OperationDispatchGovernanceContractVersionV5 = "5.0.0"

// OperationAuthorizedAdmissionV5 binds one accepted Effect to a nominal V5
// Review Authorization. It never projects quorum/not-required into V4/V3.
type OperationAuthorizedAdmissionV5 struct {
	ContractVersion          string                              `json:"contract_version"`
	Admission                OperationEffectAdmissionReceiptV3   `json:"effect_admission"`
	Authorization            OperationReviewAuthorizationRefV5   `json:"review_authorization"`
	AuthorizationBasis       OperationReviewAuthorizationBasisV5 `json:"review_authorization_basis"`
	PayloadSchema            SchemaRefV2                         `json:"payload_schema"`
	PayloadDigest            core.Digest                         `json:"payload_digest"`
	PayloadRevision          core.Revision                       `json:"payload_revision"`
	ReviewProjectionDigest   core.Digest                         `json:"review_projection_digest"`
	ReviewCurrentnessDigest  core.Digest                         `json:"review_currentness_digest"`
	GovernanceSnapshotDigest core.Digest                         `json:"governance_snapshot_digest"`
	AuthorizationFenceDigest core.Digest                         `json:"authorization_fence_digest"`
	ExpiresUnixNano          int64                               `json:"expires_unix_nano"`
	Digest                   core.Digest                         `json:"digest"`
}

func (a OperationAuthorizedAdmissionV5) Validate() error {
	if a.ContractVersion != OperationDispatchGovernanceContractVersionV5 || a.PayloadRevision == 0 || a.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectAuthorizationMissing, "V5 dispatch admission identity and TTL are incomplete")
	}
	if err := a.Admission.Validate(); err != nil {
		return err
	}
	if err := a.Authorization.Validate(); err != nil {
		return err
	}
	if err := validateOperationReviewAuthorizationBasisV5(a.AuthorizationBasis); err != nil {
		return err
	}
	if err := a.PayloadSchema.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{a.PayloadDigest, a.ReviewProjectionDigest, a.ReviewCurrentnessDigest, a.GovernanceSnapshotDigest, a.AuthorizationFenceDigest, a.Digest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	digest, err := a.DigestV5()
	if err != nil || digest != a.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V5 dispatch admission digest drifted")
	}
	return nil
}

func (a OperationAuthorizedAdmissionV5) DigestV5() (core.Digest, error) {
	copy := a
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-governance", OperationDispatchGovernanceContractVersionV5, "OperationAuthorizedAdmissionV5", copy)
}

func SealOperationAuthorizedAdmissionV5(a OperationAuthorizedAdmissionV5) (OperationAuthorizedAdmissionV5, error) {
	a.ContractVersion = OperationDispatchGovernanceContractVersionV5
	a.Digest = ""
	digest, err := a.DigestV5()
	if err != nil {
		return OperationAuthorizedAdmissionV5{}, err
	}
	a.Digest = digest
	return a, a.Validate()
}

func OperationReviewCurrentnessDigestV5(value OperationReviewCurrentProjectionV5) (core.Digest, error) {
	switch value.Basis {
	case OperationReviewBasisAcceptedQuorumV5, OperationReviewBasisConditionalQuorumSatisfiedV5:
		if value.Quorum == nil || value.PolicyNotRequired != nil {
			break
		}
		if err := value.Quorum.CurrentnessDigest.Validate(); err != nil {
			return "", err
		}
		return value.Quorum.CurrentnessDigest, nil
	case OperationReviewBasisPolicyNotRequiredV5:
		if value.PolicyNotRequired == nil || value.Quorum != nil {
			break
		}
		if err := value.PolicyNotRequired.CurrentnessDigest.Validate(); err != nil {
			return "", err
		}
		return value.PolicyNotRequired.CurrentnessDigest, nil
	}
	return "", core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "V5 Review currentness branch is invalid")
}

func (a OperationAuthorizedAdmissionV5) ValidateAgainstAuthorization(fact OperationReviewAuthorizationFactV5, now time.Time) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if err := fact.Validate(); err != nil {
		return err
	}
	if fact.State != OperationReviewAuthorizationActiveV5 || now.IsZero() || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "V5 Review Authorization is not current")
	}
	currentness, err := OperationReviewCurrentnessDigestV5(fact.Review)
	if err != nil {
		return err
	}
	operationDigest, err := fact.Intent.Operation.DigestV3()
	if err != nil {
		return err
	}
	if a.Authorization != fact.RefV5() || a.AuthorizationBasis != fact.Review.Basis || a.Admission.OperationDigest != operationDigest || a.Admission.EffectID != fact.Intent.IntentID || a.Admission.IntentRevision != fact.Intent.IntentRevision || a.Admission.IntentDigest != fact.Intent.IntentDigest || a.Admission.FactRevision != fact.Intent.EffectFactRevision || a.PayloadSchema != fact.Intent.PayloadSchema || a.PayloadDigest != fact.Intent.PayloadDigest || a.PayloadRevision != fact.Intent.PayloadRevision || a.ReviewProjectionDigest != fact.Review.ProjectionDigest || a.ReviewCurrentnessDigest != currentness || a.GovernanceSnapshotDigest != fact.Governance.SnapshotDigest || a.AuthorizationFenceDigest != fact.FenceDigest || a.ExpiresUnixNano != fact.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "V5 dispatch admission does not bind the exact accepted Effect and Review Authorization")
	}
	return nil
}

type OperationDispatchPermitV5 struct {
	ContractVersion          string                              `json:"contract_version"`
	ID                       string                              `json:"id"`
	Revision                 core.Revision                       `json:"revision"`
	AttemptID                string                              `json:"attempt_id"`
	IntentID                 core.EffectIntentID                 `json:"intent_id"`
	IntentRevision           core.Revision                       `json:"intent_revision"`
	IntentDigest             core.Digest                         `json:"intent_digest"`
	Operation                OperationSubjectV3                  `json:"operation"`
	PayloadSchema            SchemaRefV2                         `json:"payload_schema"`
	PayloadDigest            core.Digest                         `json:"payload_digest"`
	PayloadRevision          core.Revision                       `json:"payload_revision"`
	ConflictDomain           ConflictDomainBindingV2             `json:"conflict_domain"`
	Provider                 ProviderBindingRefV2                `json:"provider"`
	EnforcementPoint         ProviderBindingRefV2                `json:"enforcement_point"`
	Authority                AuthorityBindingRefV2               `json:"authority"`
	Review                   OperationReviewBindingRefV3         `json:"review"`
	Budget                   OperationBudgetBindingRefV3         `json:"budget"`
	Policy                   OperationPolicyBindingRefV3         `json:"policy"`
	Authorization            OperationReviewAuthorizationRefV5   `json:"review_authorization"`
	AuthorizationBasis       OperationReviewAuthorizationBasisV5 `json:"review_authorization_basis"`
	CapabilityGrantDigest    core.Digest                         `json:"capability_grant_digest"`
	CredentialGrantDigest    core.Digest                         `json:"credential_grant_digest"`
	GovernanceSnapshotDigest core.Digest                         `json:"governance_snapshot_digest"`
	FenceDigest              core.Digest                         `json:"fence_digest"`
	Idempotency              IdempotencyBindingV2                `json:"idempotency"`
	IssuedUnixNano           int64                               `json:"issued_unix_nano"`
	ExpiresUnixNano          int64                               `json:"expires_unix_nano"`
	Admission                OperationAuthorizedAdmissionV5      `json:"dispatch_admission"`
	Digest                   core.Digest                         `json:"digest"`
}

func (p OperationDispatchPermitV5) Validate() error {
	if p.ContractVersion != OperationDispatchGovernanceContractVersionV5 || validateEvidenceIDV2(p.ID) != nil || p.Revision != 1 || validateEvidenceIDV2(p.AttemptID) != nil || validateEvidenceIDV2(string(p.IntentID)) != nil || p.IntentRevision == 0 || p.PayloadRevision == 0 || p.IssuedUnixNano <= 0 || p.ExpiresUnixNano <= p.IssuedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 Permit identity, revision or TTL is invalid")
	}
	if err := p.Operation.Validate(); err != nil {
		return err
	}
	if err := p.PayloadSchema.Validate(); err != nil {
		return err
	}
	if err := p.Provider.Validate(); err != nil {
		return err
	}
	if err := p.EnforcementPoint.Validate(); err != nil {
		return err
	}
	if err := p.Authority.Validate(); err != nil {
		return err
	}
	if err := p.Review.Validate(); err != nil {
		return err
	}
	if err := p.Budget.Validate(); err != nil {
		return err
	}
	if err := p.Policy.Validate(); err != nil {
		return err
	}
	if err := p.Authorization.Validate(); err != nil {
		return err
	}
	if err := validateOperationReviewAuthorizationBasisV5(p.AuthorizationBasis); err != nil {
		return err
	}
	if err := p.Admission.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{p.IntentDigest, p.PayloadDigest, p.CapabilityGrantDigest, p.CredentialGrantDigest, p.GovernanceSnapshotDigest, p.FenceDigest, p.Digest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if p.Admission.Authorization != p.Authorization || p.Admission.AuthorizationBasis != p.AuthorizationBasis || p.Admission.Admission.EffectID != p.IntentID || p.Admission.Admission.IntentRevision != p.IntentRevision || p.Admission.Admission.IntentDigest != p.IntentDigest || p.Admission.PayloadSchema != p.PayloadSchema || p.Admission.PayloadDigest != p.PayloadDigest || p.Admission.PayloadRevision != p.PayloadRevision || p.Admission.GovernanceSnapshotDigest != p.GovernanceSnapshotDigest || p.ExpiresUnixNano > p.Admission.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 Permit does not bind one exact dispatch admission")
	}
	digest, err := p.DigestV5()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V5 Permit digest drifted")
	}
	return nil
}

func (p OperationDispatchPermitV5) DigestV5() (core.Digest, error) {
	copy := p
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-governance", OperationDispatchGovernanceContractVersionV5, "OperationDispatchPermitV5", copy)
}
func SealOperationDispatchPermitV5(p OperationDispatchPermitV5) (OperationDispatchPermitV5, error) {
	p.ContractVersion = OperationDispatchGovernanceContractVersionV5
	p.Digest = ""
	d, e := p.DigestV5()
	if e != nil {
		return OperationDispatchPermitV5{}, e
	}
	p.Digest = d
	return p, p.Validate()
}

func (p OperationDispatchPermitV5) ValidateAgainstAuthorization(fact OperationReviewAuthorizationFactV5, fence core.ExecutionFence, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := p.Admission.ValidateAgainstAuthorization(fact, now); err != nil {
		return err
	}
	if p.Authorization != fact.RefV5() || p.AuthorizationBasis != fact.Review.Basis || p.IntentID != fact.Intent.IntentID || p.IntentRevision != fact.Intent.IntentRevision || p.IntentDigest != fact.Intent.IntentDigest || !SameOperationSubjectV3(p.Operation, fact.Intent.Operation) || p.PayloadSchema != fact.Intent.PayloadSchema || p.PayloadDigest != fact.Intent.PayloadDigest || p.PayloadRevision != fact.Intent.PayloadRevision || p.Provider != fact.Intent.Provider || p.EnforcementPoint != fact.Intent.Provider || p.Authority != fact.Intent.Authority || p.Review != fact.Intent.ReviewBinding || p.Budget.Ref != fact.Governance.Budget.Ref || p.Budget.Revision != fact.Governance.Budget.Revision || p.Budget.Digest != fact.Governance.Budget.Digest || p.Policy != fact.Intent.DispatchPolicy || p.CapabilityGrantDigest != fact.Governance.CapabilityGrantDigest || p.CredentialGrantDigest != fact.Governance.CredentialGrantDigest || p.GovernanceSnapshotDigest != fact.Governance.SnapshotDigest || p.FenceDigest != mustFenceDigestV5(fence, p.Operation) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "V5 Permit drifted from Review Authorization")
	}
	return validateOperationDispatchFenceV5(fence, fact.Fence)
}

func mustFenceDigestV5(fence core.ExecutionFence, operation OperationSubjectV3) core.Digest {
	d, _ := DigestOperationExecutionFenceV3(fence, operation)
	return d
}
func validateOperationDispatchFenceV5(permit, authorization core.ExecutionFence) error {
	if err := permit.Validate(); err != nil {
		return err
	}
	if err := authorization.Validate(); err != nil {
		return err
	}
	if permit.BoundaryScope != authorization.BoundaryScope || !SameExecutionScopeV2(permit.Scope, authorization.Scope) || permit.CapabilityGrantDigest != authorization.CapabilityGrantDigest || permit.EffectIntentID != authorization.EffectIntentID || permit.EffectIntentRevision != authorization.EffectIntentRevision || permit.CanonicalPayloadDigest != authorization.CanonicalPayloadDigest || permit.ExpiresAt.After(authorization.ExpiresAt) {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "V5 Permit Fence widened or drifted from Review Authorization")
	}
	return nil
}

type OperationDispatchPermitStateV5 string

const (
	OperationPermitIssuedV5  OperationDispatchPermitStateV5 = "issued"
	OperationPermitBegunV5   OperationDispatchPermitStateV5 = "begun"
	OperationPermitExpiredV5 OperationDispatchPermitStateV5 = "expired"
	OperationPermitRevokedV5 OperationDispatchPermitStateV5 = "revoked"
)

type OperationDispatchRecordV5 struct {
	Permit             OperationDispatchPermitV5      `json:"permit"`
	PermitDigest       core.Digest                    `json:"permit_digest"`
	Fence              core.ExecutionFence            `json:"fence"`
	State              OperationDispatchPermitStateV5 `json:"state"`
	Revision           core.Revision                  `json:"revision"`
	EffectFactRevision core.Revision                  `json:"effect_fact_revision"`
	BegunUnixNano      int64                          `json:"begun_unix_nano,omitempty"`
	Digest             core.Digest                    `json:"digest"`
}

func (r OperationDispatchRecordV5) Validate() error {
	if err := r.Permit.Validate(); err != nil {
		return err
	}
	if r.Revision == 0 || r.EffectFactRevision == 0 || r.PermitDigest != r.Permit.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "V5 Permit record identity drifted")
	}
	fd, err := DigestOperationExecutionFenceV3(r.Fence, r.Permit.Operation)
	if err != nil || fd != r.Permit.FenceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "V5 Permit record Fence drifted")
	}
	switch r.State {
	case OperationPermitIssuedV5:
		if r.Revision != 1 || r.BegunUnixNano != 0 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "issued V5 Permit carries later facts")
		}
	case OperationPermitBegunV5:
		if r.Revision != 2 || r.BegunUnixNano <= 0 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "begun V5 Permit lacks begin time")
		}
	case OperationPermitExpiredV5, OperationPermitRevokedV5:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 Permit state is invalid")
	}
	d, e := r.DigestV5()
	if e != nil || d != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V5 Permit record digest drifted")
	}
	return nil
}
func (r OperationDispatchRecordV5) DigestV5() (core.Digest, error) {
	r.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-governance", OperationDispatchGovernanceContractVersionV5, "OperationDispatchRecordV5", r)
}
func SealOperationDispatchRecordV5(r OperationDispatchRecordV5) (OperationDispatchRecordV5, error) {
	r.Digest = ""
	d, e := r.DigestV5()
	if e != nil {
		return OperationDispatchRecordV5{}, e
	}
	r.Digest = d
	return r, r.Validate()
}

type CurrentOperationDispatchAuthorizationV5 struct {
	Record                   OperationDispatchRecordV5           `json:"record"`
	ReviewAuthorization      OperationReviewAuthorizationRefV5   `json:"review_authorization"`
	AuthorizationBasis       OperationReviewAuthorizationBasisV5 `json:"review_authorization_basis"`
	ReviewProjectionDigest   core.Digest                         `json:"review_projection_digest"`
	ReviewCurrentnessDigest  core.Digest                         `json:"review_currentness_digest"`
	GovernanceSnapshotDigest core.Digest                         `json:"governance_snapshot_digest"`
	CheckedUnixNano          int64                               `json:"checked_unix_nano"`
	ExpiresUnixNano          int64                               `json:"expires_unix_nano"`
	Digest                   core.Digest                         `json:"digest"`
}

func (a CurrentOperationDispatchAuthorizationV5) Validate() error {
	if err := a.Record.Validate(); err != nil {
		return err
	}
	if err := a.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	if err := validateOperationReviewAuthorizationBasisV5(a.AuthorizationBasis); err != nil {
		return err
	}
	for _, d := range []core.Digest{a.ReviewProjectionDigest, a.ReviewCurrentnessDigest, a.GovernanceSnapshotDigest, a.Digest} {
		if err := d.Validate(); err != nil {
			return err
		}
	}
	ad := a.Record.Permit.Admission
	if a.CheckedUnixNano <= 0 || a.ExpiresUnixNano <= a.CheckedUnixNano || (a.Record.State != OperationPermitIssuedV5 && a.Record.State != OperationPermitBegunV5) || a.ReviewAuthorization != ad.Authorization || a.AuthorizationBasis != ad.AuthorizationBasis || a.ReviewProjectionDigest != ad.ReviewProjectionDigest || a.ReviewCurrentnessDigest != ad.ReviewCurrentnessDigest || a.GovernanceSnapshotDigest != ad.GovernanceSnapshotDigest || a.ExpiresUnixNano > a.Record.Permit.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "current V5 dispatch envelope drifted")
	}
	d, e := a.DigestV5()
	if e != nil || d != a.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "current V5 dispatch digest drifted")
	}
	return nil
}
func (a CurrentOperationDispatchAuthorizationV5) DigestV5() (core.Digest, error) {
	a.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-dispatch-governance", OperationDispatchGovernanceContractVersionV5, "CurrentOperationDispatchAuthorizationV5", a)
}
func SealCurrentOperationDispatchAuthorizationV5(a CurrentOperationDispatchAuthorizationV5) (CurrentOperationDispatchAuthorizationV5, error) {
	a.Digest = ""
	d, e := a.DigestV5()
	if e != nil {
		return CurrentOperationDispatchAuthorizationV5{}, e
	}
	a.Digest = d
	return a, a.Validate()
}

type IssueGovernedOperationDispatchRequestV5 struct {
	Operation              OperationSubjectV3                  `json:"operation"`
	EffectID               core.EffectIntentID                 `json:"effect_id"`
	ExpectedEffectRevision core.Revision                       `json:"expected_effect_revision"`
	Admission              OperationEffectAdmissionReceiptV3   `json:"effect_admission"`
	ReviewAuthorization    OperationReviewAuthorizationRefV5   `json:"review_authorization"`
	AuthorizationBasis     OperationReviewAuthorizationBasisV5 `json:"review_authorization_basis"`
	PermitID               string                              `json:"permit_id"`
	AttemptID              string                              `json:"attempt_id"`
	PermitTTL              time.Duration                       `json:"permit_ttl"`
}

func (r IssueGovernedOperationDispatchRequestV5) Validate() error {
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.EffectID)) != nil || r.ExpectedEffectRevision == 0 || validateEvidenceIDV2(r.PermitID) != nil || validateEvidenceIDV2(r.AttemptID) != nil || r.PermitTTL <= 0 || r.PermitTTL > MaxDispatchPermitTTL {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 Issue request is incomplete")
	}
	if err := r.Admission.Validate(); err != nil {
		return err
	}
	if err := r.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	return validateOperationReviewAuthorizationBasisV5(r.AuthorizationBasis)
}

type InspectOperationDispatchRecordRequestV5 struct {
	Operation OperationSubjectV3  `json:"operation"`
	EffectID  core.EffectIntentID `json:"effect_id"`
	PermitID  string              `json:"permit_id"`
}

func (r InspectOperationDispatchRecordRequestV5) Validate() error {
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.EffectID)) != nil || validateEvidenceIDV2(r.PermitID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 dispatch Inspect key is incomplete")
	}
	return nil
}

type InspectCurrentOperationDispatchRequestV5 struct {
	Inspect             InspectOperationDispatchRecordRequestV5 `json:"inspect"`
	AdmissionDigest     core.Digest                             `json:"admission_digest"`
	ReviewAuthorization OperationReviewAuthorizationRefV5       `json:"review_authorization"`
	AuthorizationBasis  OperationReviewAuthorizationBasisV5     `json:"review_authorization_basis"`
}

func (r InspectCurrentOperationDispatchRequestV5) Validate() error {
	if err := r.Inspect.Validate(); err != nil {
		return err
	}
	if err := r.AdmissionDigest.Validate(); err != nil {
		return err
	}
	if err := r.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	return validateOperationReviewAuthorizationBasisV5(r.AuthorizationBasis)
}

type BeginGovernedOperationDispatchRequestV5 struct {
	Operation                  OperationSubjectV3                  `json:"operation"`
	EffectID                   core.EffectIntentID                 `json:"effect_id"`
	ExpectedEffectRevision     core.Revision                       `json:"expected_effect_revision"`
	PermitID                   string                              `json:"permit_id"`
	ExpectedPermitFactRevision core.Revision                       `json:"expected_permit_fact_revision"`
	AdmissionDigest            core.Digest                         `json:"admission_digest"`
	ReviewAuthorization        OperationReviewAuthorizationRefV5   `json:"review_authorization"`
	AuthorizationBasis         OperationReviewAuthorizationBasisV5 `json:"review_authorization_basis"`
}

func (r BeginGovernedOperationDispatchRequestV5) Validate() error {
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.EffectID)) != nil || r.ExpectedEffectRevision == 0 || validateEvidenceIDV2(r.PermitID) != nil || r.ExpectedPermitFactRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 Begin request is incomplete")
	}
	if err := r.AdmissionDigest.Validate(); err != nil {
		return err
	}
	if err := r.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	return validateOperationReviewAuthorizationBasisV5(r.AuthorizationBasis)
}

type OperationGovernancePortV5 interface {
	IssueOperationDispatchV5(context.Context, IssueGovernedOperationDispatchRequestV5) (CurrentOperationDispatchAuthorizationV5, error)
	InspectOperationDispatchRecordV5(context.Context, InspectOperationDispatchRecordRequestV5) (OperationDispatchRecordV5, error)
	InspectCurrentOperationDispatchV5(context.Context, InspectCurrentOperationDispatchRequestV5) (CurrentOperationDispatchAuthorizationV5, error)
	BeginOperationDispatchV5(context.Context, BeginGovernedOperationDispatchRequestV5) (CurrentOperationDispatchAuthorizationV5, error)
}

func validateOperationReviewAuthorizationBasisV5(b OperationReviewAuthorizationBasisV5) error {
	switch b {
	case OperationReviewBasisAcceptedQuorumV5, OperationReviewBasisConditionalQuorumSatisfiedV5, OperationReviewBasisPolicyNotRequiredV5:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "V5 Review Authorization basis is unsupported")
	}
}

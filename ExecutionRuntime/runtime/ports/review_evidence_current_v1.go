package ports

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ReviewEvidenceCurrentContractVersionV1 = "1.0.0"
	reviewEvidenceCurrentCanonicalDomainV1 = "praxis.runtime.review-evidence-current/v1"
)

// ReviewEvidenceTargetRefV1 is the exact Review-owned target coordinate. It
// does not duplicate the target fact or grant authority over that fact.
type ReviewEvidenceTargetRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ReviewEvidenceTargetRefV1) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision == 0 {
		return reviewEvidenceInvalidV1("Review evidence target requires exact identity and revision")
	}
	return r.Digest.Validate()
}

// ReviewEvidenceApplicabilitySubjectV1 is the complete stable identity used
// to resolve an Evidence applicability projection. In particular, neither a
// ReviewEvidenceRef nor an Evidence record alone may be used as a weak lookup.
type ReviewEvidenceApplicabilitySubjectV1 struct {
	TenantID          core.TenantID             `json:"tenant_id"`
	Target            ReviewEvidenceTargetRefV1 `json:"target"`
	RunID             core.AgentRunID           `json:"run_id"`
	Scope             core.ExecutionScope       `json:"scope"`
	ActionScopeDigest core.Digest               `json:"action_scope_digest"`
	ReviewEvidence    ReviewEvidenceRefV2       `json:"review_evidence"`
}

func (s ReviewEvidenceApplicabilitySubjectV1) Validate() error {
	if strings.TrimSpace(string(s.TenantID)) == "" || strings.TrimSpace(string(s.RunID)) == "" {
		return reviewEvidenceInvalidV1("Review evidence subject requires tenant and run")
	}
	if err := s.Target.Validate(); err != nil {
		return err
	}
	if err := s.Scope.Validate(); err != nil {
		return err
	}
	if s.Scope.Identity.TenantID != s.TenantID {
		return reviewEvidenceScopeV1("Review evidence tenant drifted from execution scope")
	}
	if err := s.ActionScopeDigest.Validate(); err != nil {
		return err
	}
	return s.ReviewEvidence.Validate()
}

func DigestReviewEvidenceApplicabilitySubjectV1(s ReviewEvidenceApplicabilitySubjectV1) (core.Digest, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(reviewEvidenceCurrentCanonicalDomainV1, ReviewEvidenceCurrentContractVersionV1, "ReviewEvidenceApplicabilitySubjectV1", s)
}

type ReviewEvidenceApplicabilityProjectionIDInputV1 struct {
	SubjectDigest core.Digest `json:"subject_digest"`
}

func DeriveReviewEvidenceApplicabilityProjectionIDV1(subjectDigest core.Digest) (string, error) {
	if err := subjectDigest.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(reviewEvidenceCurrentCanonicalDomainV1, ReviewEvidenceCurrentContractVersionV1, "ReviewEvidenceApplicabilityProjectionIDV1", ReviewEvidenceApplicabilityProjectionIDInputV1{SubjectDigest: subjectDigest})
	return string(digest), err
}

func DeriveReviewEvidenceApplicabilityCurrentIndexIDV1(subjectDigest core.Digest) (string, error) {
	if err := subjectDigest.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(reviewEvidenceCurrentCanonicalDomainV1, ReviewEvidenceCurrentContractVersionV1, "ReviewEvidenceApplicabilityCurrentIndexIDV1", ReviewEvidenceApplicabilityProjectionIDInputV1{SubjectDigest: subjectDigest})
	return string(digest), err
}

type ReviewEvidenceApplicabilityRefV1 struct {
	ProjectionID  string        `json:"projection_id"`
	Revision      core.Revision `json:"revision"`
	SubjectDigest core.Digest   `json:"subject_digest"`
	Digest        core.Digest   `json:"digest"`
}

func (r ReviewEvidenceApplicabilityRefV1) Validate() error {
	if strings.TrimSpace(r.ProjectionID) == "" || r.Revision == 0 || r.SubjectDigest.Validate() != nil || r.Digest.Validate() != nil {
		return reviewEvidenceInvalidV1("Review evidence applicability ref is incomplete")
	}
	id, err := DeriveReviewEvidenceApplicabilityProjectionIDV1(r.SubjectDigest)
	if err != nil {
		return err
	}
	if r.ProjectionID != id {
		return reviewEvidenceConflictV1("Review evidence applicability projection ID drifted")
	}
	return nil
}

// ReviewEvidenceApplicabilityProjectionV1 is immutable. Checked, Expires and
// ProjectionDigest are sealed once; reads return a deep clone and validate
// currentness with caller-supplied time instead of re-sealing the projection.
type ReviewEvidenceApplicabilityProjectionV1 struct {
	ContractVersion string                               `json:"contract_version"`
	Ref             ReviewEvidenceApplicabilityRefV1     `json:"ref"`
	Subject         ReviewEvidenceApplicabilitySubjectV1 `json:"subject"`
	SubjectDigest   core.Digest                          `json:"subject_digest"`
	Previous        *ReviewEvidenceApplicabilityRefV1    `json:"previous,omitempty"`

	EvidenceSubject           EvidenceSubjectKeyV1             `json:"evidence_subject"`
	EvidenceSubjectProjection EvidenceSubjectProjectionRefV1   `json:"evidence_subject_projection"`
	EvidenceSubjectSnapshot   EvidenceSubjectCurrentSnapshotV1 `json:"evidence_subject_snapshot"`
	Record                    EvidenceRecordRefV2              `json:"record"`
	TrustClass                EvidenceTrustClassV2             `json:"trust_class"`
	OwnerFact                 *EvidenceOwnerFactRefV2          `json:"owner_fact,omitempty"`

	CheckedUnixNano  int64       `json:"checked_unix_nano"`
	ExpiresUnixNano  int64       `json:"expires_unix_nano"`
	ProjectionDigest core.Digest `json:"projection_digest"`
}

func (p ReviewEvidenceApplicabilityProjectionV1) Validate() error {
	if p.ContractVersion != ReviewEvidenceCurrentContractVersionV1 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return reviewEvidenceInvalidV1("Review evidence applicability projection version or TTL is invalid")
	}
	if err := p.Subject.Validate(); err != nil {
		return err
	}
	subjectDigest, err := DigestReviewEvidenceApplicabilitySubjectV1(p.Subject)
	if err != nil {
		return err
	}
	if p.SubjectDigest != subjectDigest || p.Ref.SubjectDigest != subjectDigest {
		return reviewEvidenceConflictV1("Review evidence applicability subject digest drifted")
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	if p.Previous == nil {
		if p.Ref.Revision != 1 {
			return reviewEvidenceConflictV1("first Review evidence applicability projection must be revision one")
		}
	} else if p.Previous.Validate() != nil || p.Previous.ProjectionID != p.Ref.ProjectionID || p.Previous.Revision+1 != p.Ref.Revision {
		return reviewEvidenceConflictV1("Review evidence applicability history is not contiguous")
	}
	if err := p.EvidenceSubjectSnapshot.Validate(); err != nil {
		return err
	}
	snapshot := p.EvidenceSubjectSnapshot.Projection
	if !reflect.DeepEqual(p.EvidenceSubject, snapshot.Subject) || p.EvidenceSubjectProjection != snapshot.Ref || p.Record != snapshot.Record || p.Record != p.EvidenceSubject.Record || p.TrustClass != snapshot.TrustClass {
		return reviewEvidenceConflictV1("Review evidence applicability drifted from exact Evidence subject snapshot")
	}
	if err := validateEvidenceTrustV2(p.TrustClass); err != nil {
		return err
	}
	if snapshot.CustomClass != p.Subject.ReviewEvidence.Classification || snapshot.CandidateDigest != p.Subject.ReviewEvidence.Digest {
		return reviewEvidenceConflictV1("Review evidence nominal ref drifted from Evidence classification or candidate digest")
	}
	if snapshot.ExecutionScope.Validate() != nil || !SameExecutionScopeV2(snapshot.ExecutionScope, p.Subject.Scope) || snapshot.ActionScopeDigest != p.Subject.ActionScopeDigest || snapshot.LedgerScope.TenantID != p.Subject.TenantID || snapshot.LedgerScope.RunID != p.Subject.RunID {
		return reviewEvidenceScopeV1("Review evidence applicability scope or run drifted from Evidence subject")
	}
	if p.ExpiresUnixNano > snapshot.ExpiresUnixNano {
		return reviewEvidenceStaleV1("Review evidence applicability TTL exceeds Evidence subject TTL")
	}
	if p.TrustClass == EvidenceTrustAuthoritativeFact {
		if p.OwnerFact == nil || snapshot.OwnerFact == nil || !reflect.DeepEqual(*p.OwnerFact, *snapshot.OwnerFact) || p.OwnerFact.Validate() != nil {
			return reviewEvidenceTrustV1("authoritative Review evidence requires the exact Owner fact")
		}
	} else if p.OwnerFact != nil || snapshot.OwnerFact != nil {
		return reviewEvidenceTrustV1("non-authoritative Review evidence cannot carry an Owner fact")
	}
	copy := CloneReviewEvidenceApplicabilityProjectionV1(p)
	copy.Ref.Digest = ""
	copy.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest(reviewEvidenceCurrentCanonicalDomainV1, ReviewEvidenceCurrentContractVersionV1, "ReviewEvidenceApplicabilityProjectionV1", copy)
	if err != nil {
		return err
	}
	if digest != p.Ref.Digest || digest != p.ProjectionDigest {
		return reviewEvidenceConflictV1("Review evidence applicability body and ref digest drifted")
	}
	return nil
}

func (p ReviewEvidenceApplicabilityProjectionV1) ValidateCurrent(expected ReviewEvidenceApplicabilityRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if p.Ref != expected {
		return reviewEvidenceConflictV1("Review evidence applicability current ref drifted")
	}
	if now.IsZero() {
		return reviewEvidenceInvalidV1("Review evidence applicability current validation requires time")
	}
	if now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review evidence applicability clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return reviewEvidenceStaleV1("Review evidence applicability expired")
	}
	return nil
}

func SealReviewEvidenceApplicabilityProjectionV1(p ReviewEvidenceApplicabilityProjectionV1) (ReviewEvidenceApplicabilityProjectionV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != ReviewEvidenceCurrentContractVersionV1 {
		return ReviewEvidenceApplicabilityProjectionV1{}, reviewEvidenceInvalidV1("Review evidence applicability contract version is invalid")
	}
	p.ContractVersion = ReviewEvidenceCurrentContractVersionV1
	subjectDigest, err := DigestReviewEvidenceApplicabilitySubjectV1(p.Subject)
	if err != nil {
		return ReviewEvidenceApplicabilityProjectionV1{}, err
	}
	if p.SubjectDigest != "" && p.SubjectDigest != subjectDigest || p.Ref.SubjectDigest != "" && p.Ref.SubjectDigest != subjectDigest {
		return ReviewEvidenceApplicabilityProjectionV1{}, reviewEvidenceConflictV1("Review evidence applicability supplied wrong subject digest")
	}
	p.SubjectDigest, p.Ref.SubjectDigest = subjectDigest, subjectDigest
	id, err := DeriveReviewEvidenceApplicabilityProjectionIDV1(subjectDigest)
	if err != nil {
		return ReviewEvidenceApplicabilityProjectionV1{}, err
	}
	if p.Ref.ProjectionID != "" && p.Ref.ProjectionID != id {
		return ReviewEvidenceApplicabilityProjectionV1{}, reviewEvidenceConflictV1("Review evidence applicability supplied wrong projection ID")
	}
	p.Ref.ProjectionID = id
	providedRefDigest, providedDigest := p.Ref.Digest, p.ProjectionDigest
	p.Ref.Digest, p.ProjectionDigest = "", ""
	digest, err := core.CanonicalJSONDigest(reviewEvidenceCurrentCanonicalDomainV1, ReviewEvidenceCurrentContractVersionV1, "ReviewEvidenceApplicabilityProjectionV1", p)
	if err != nil {
		return ReviewEvidenceApplicabilityProjectionV1{}, err
	}
	if providedRefDigest != "" && providedRefDigest != digest || providedDigest != "" && providedDigest != digest {
		return ReviewEvidenceApplicabilityProjectionV1{}, reviewEvidenceConflictV1("Review evidence applicability supplied wrong projection digest")
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p, p.Validate()
}

func CloneReviewEvidenceApplicabilityProjectionV1(p ReviewEvidenceApplicabilityProjectionV1) ReviewEvidenceApplicabilityProjectionV1 {
	return cloneReviewEvidenceCurrentV1(p)
}

type ReviewEvidenceApplicabilityCurrentIndexRefV1 struct {
	IndexID           string                            `json:"index_id"`
	Revision          core.Revision                     `json:"revision"`
	SubjectDigest     core.Digest                       `json:"subject_digest"`
	Previous          *ReviewEvidenceApplicabilityRefV1 `json:"previous,omitempty"`
	CurrentProjection ReviewEvidenceApplicabilityRefV1  `json:"current_projection"`
	HighestRevision   core.Revision                     `json:"highest_revision"`
	Digest            core.Digest                       `json:"digest"`
}

func (r ReviewEvidenceApplicabilityCurrentIndexRefV1) Validate() error {
	if r.Revision == 0 || r.HighestRevision == 0 || r.SubjectDigest.Validate() != nil || r.CurrentProjection.Validate() != nil || r.Digest.Validate() != nil {
		return reviewEvidenceInvalidV1("Review evidence applicability current index is incomplete")
	}
	id, err := DeriveReviewEvidenceApplicabilityCurrentIndexIDV1(r.SubjectDigest)
	if err != nil {
		return err
	}
	if r.IndexID != id || r.Revision != r.CurrentProjection.Revision || r.HighestRevision != r.Revision || r.SubjectDigest != r.CurrentProjection.SubjectDigest {
		return reviewEvidenceConflictV1("Review evidence applicability current index coordinates drifted")
	}
	if r.Revision == 1 {
		if r.Previous != nil {
			return reviewEvidenceConflictV1("first Review evidence applicability index cannot have previous projection")
		}
	} else if r.Previous == nil || r.Previous.Validate() != nil || r.Previous.ProjectionID != r.CurrentProjection.ProjectionID || r.Previous.Revision+1 != r.Revision {
		return reviewEvidenceConflictV1("Review evidence applicability current index history is not contiguous")
	}
	copy := cloneReviewEvidenceCurrentV1(r)
	copy.Digest = ""
	digest, err := core.CanonicalJSONDigest(reviewEvidenceCurrentCanonicalDomainV1, ReviewEvidenceCurrentContractVersionV1, "ReviewEvidenceApplicabilityCurrentIndexRefV1", copy)
	if err != nil {
		return err
	}
	if digest != r.Digest {
		return reviewEvidenceConflictV1("Review evidence applicability current index digest drifted")
	}
	return nil
}

func SealReviewEvidenceApplicabilityCurrentIndexRefV1(r ReviewEvidenceApplicabilityCurrentIndexRefV1) (ReviewEvidenceApplicabilityCurrentIndexRefV1, error) {
	id, err := DeriveReviewEvidenceApplicabilityCurrentIndexIDV1(r.SubjectDigest)
	if err != nil {
		return ReviewEvidenceApplicabilityCurrentIndexRefV1{}, err
	}
	if r.IndexID != "" && r.IndexID != id {
		return ReviewEvidenceApplicabilityCurrentIndexRefV1{}, reviewEvidenceConflictV1("Review evidence applicability supplied wrong index ID")
	}
	r.IndexID = id
	provided := r.Digest
	r.Digest = ""
	digest, err := core.CanonicalJSONDigest(reviewEvidenceCurrentCanonicalDomainV1, ReviewEvidenceCurrentContractVersionV1, "ReviewEvidenceApplicabilityCurrentIndexRefV1", r)
	if err != nil {
		return ReviewEvidenceApplicabilityCurrentIndexRefV1{}, err
	}
	if provided != "" && provided != digest {
		return ReviewEvidenceApplicabilityCurrentIndexRefV1{}, reviewEvidenceConflictV1("Review evidence applicability supplied wrong index digest")
	}
	r.Digest = digest
	return r, r.Validate()
}

type ReviewEvidenceApplicabilityCurrentSnapshotV1 struct {
	ContractVersion string                                       `json:"contract_version"`
	Projection      ReviewEvidenceApplicabilityProjectionV1      `json:"projection"`
	CurrentIndex    ReviewEvidenceApplicabilityCurrentIndexRefV1 `json:"current_index"`
}

func (s ReviewEvidenceApplicabilityCurrentSnapshotV1) Validate() error {
	if s.ContractVersion != ReviewEvidenceCurrentContractVersionV1 || s.Projection.Validate() != nil || s.CurrentIndex.Validate() != nil || s.CurrentIndex.CurrentProjection != s.Projection.Ref {
		return reviewEvidenceConflictV1("Review evidence applicability snapshot is not an exact current pair")
	}
	return nil
}

func (s ReviewEvidenceApplicabilityCurrentSnapshotV1) ValidateCurrent(expected ReviewEvidenceApplicabilityRefV1, now time.Time) error {
	if err := s.Validate(); err != nil {
		return err
	}
	return s.Projection.ValidateCurrent(expected, now)
}

func CloneReviewEvidenceApplicabilityCurrentSnapshotV1(s ReviewEvidenceApplicabilityCurrentSnapshotV1) ReviewEvidenceApplicabilityCurrentSnapshotV1 {
	return cloneReviewEvidenceCurrentV1(s)
}

type ResolveReviewEvidenceApplicabilityCurrentRequestV1 struct {
	ContractVersion string                               `json:"contract_version"`
	Subject         ReviewEvidenceApplicabilitySubjectV1 `json:"subject"`
}

func (r ResolveReviewEvidenceApplicabilityCurrentRequestV1) Validate() error {
	if r.ContractVersion != ReviewEvidenceCurrentContractVersionV1 {
		return reviewEvidenceInvalidV1("Review evidence applicability resolve request version is invalid")
	}
	return r.Subject.Validate()
}

// ReviewEvidenceApplicabilityCurrentReaderV1 has a closed error surface:
// invalid_argument/invalid_reference, not_found/evidence_source_missing,
// conflict/evidence_conflict, precondition_failed with evidence stale, trust,
// scope or clock reasons, indeterminate/evidence_unavailable, and
// unavailable/evidence_unavailable. Lost replies are recovered only by an
// exact Inspect call; Resolve starts a new S1 and does not claim the old reply.
// Resolve is S1: the Owner atomically reads its applicability current index,
// exact immutable projection and the EvidenceSubject current index/snapshot.
// InspectCurrent is S2: it repeats those current reads for the exact Ref and
// rejects either index or underlying Evidence drift. InspectHistorical reads
// solely by exact Ref and never borrows either current index. Every successful
// method returns a deep clone of Owner state.
type ReviewEvidenceApplicabilityCurrentReaderV1 interface {
	ResolveReviewEvidenceApplicabilityCurrentV1(context.Context, ResolveReviewEvidenceApplicabilityCurrentRequestV1) (ReviewEvidenceApplicabilityCurrentSnapshotV1, error)
	InspectCurrentReviewEvidenceApplicabilityV1(context.Context, ReviewEvidenceApplicabilityRefV1) (ReviewEvidenceApplicabilityCurrentSnapshotV1, error)
	InspectHistoricalReviewEvidenceApplicabilityV1(context.Context, ReviewEvidenceApplicabilityRefV1) (ReviewEvidenceApplicabilityProjectionV1, error)
}

type PublishReviewEvidenceApplicabilityRequestV1 struct {
	ContractVersion      string                                        `json:"contract_version"`
	Projection           ReviewEvidenceApplicabilityProjectionV1       `json:"projection"`
	ExpectedCurrentIndex *ReviewEvidenceApplicabilityCurrentIndexRefV1 `json:"expected_current_index,omitempty"`
	NextCurrentIndex     ReviewEvidenceApplicabilityCurrentIndexRefV1  `json:"next_current_index"`
	RequestDigest        core.Digest                                   `json:"request_digest"`
}

func (r PublishReviewEvidenceApplicabilityRequestV1) Validate() error {
	if r.ContractVersion != ReviewEvidenceCurrentContractVersionV1 || r.Projection.Validate() != nil || r.NextCurrentIndex.Validate() != nil || r.NextCurrentIndex.CurrentProjection != r.Projection.Ref {
		return reviewEvidenceInvalidV1("Review evidence applicability publish request is incomplete")
	}
	if r.Projection.Ref.Revision == 1 {
		if r.ExpectedCurrentIndex != nil || r.Projection.Previous != nil {
			return reviewEvidenceConflictV1("first Review evidence applicability publish cannot replace current")
		}
	} else if r.ExpectedCurrentIndex == nil || r.ExpectedCurrentIndex.Validate() != nil || r.Projection.Previous == nil || r.ExpectedCurrentIndex.CurrentProjection != *r.Projection.Previous || r.NextCurrentIndex.Previous == nil || *r.NextCurrentIndex.Previous != r.ExpectedCurrentIndex.CurrentProjection {
		return reviewEvidenceConflictV1("Review evidence applicability publish CAS expectations drifted")
	}
	copy := cloneReviewEvidenceCurrentV1(r)
	copy.RequestDigest = ""
	digest, err := core.CanonicalJSONDigest(reviewEvidenceCurrentCanonicalDomainV1, ReviewEvidenceCurrentContractVersionV1, "PublishReviewEvidenceApplicabilityRequestV1", copy)
	if err != nil {
		return err
	}
	if digest != r.RequestDigest {
		return reviewEvidenceConflictV1("Review evidence applicability publish request digest drifted")
	}
	return nil
}

func SealPublishReviewEvidenceApplicabilityRequestV1(r PublishReviewEvidenceApplicabilityRequestV1) (PublishReviewEvidenceApplicabilityRequestV1, error) {
	if r.ContractVersion != "" && r.ContractVersion != ReviewEvidenceCurrentContractVersionV1 {
		return PublishReviewEvidenceApplicabilityRequestV1{}, reviewEvidenceInvalidV1("Review evidence applicability publish request version is invalid")
	}
	r.ContractVersion = ReviewEvidenceCurrentContractVersionV1
	provided := r.RequestDigest
	r.RequestDigest = ""
	digest, err := core.CanonicalJSONDigest(reviewEvidenceCurrentCanonicalDomainV1, ReviewEvidenceCurrentContractVersionV1, "PublishReviewEvidenceApplicabilityRequestV1", r)
	if err != nil {
		return PublishReviewEvidenceApplicabilityRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return PublishReviewEvidenceApplicabilityRequestV1{}, reviewEvidenceConflictV1("Review evidence applicability supplied wrong request digest")
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

type ReviewEvidenceApplicabilityPublishReceiptV1 struct {
	ContractVersion   string                                       `json:"contract_version"`
	PublishID         string                                       `json:"publish_id"`
	RequestDigest     core.Digest                                  `json:"request_digest"`
	Projection        ReviewEvidenceApplicabilityRefV1             `json:"projection"`
	CurrentIndex      ReviewEvidenceApplicabilityCurrentIndexRefV1 `json:"current_index"`
	CommittedUnixNano int64                                        `json:"committed_unix_nano"`
	ReceiptDigest     core.Digest                                  `json:"receipt_digest"`
}

func (r ReviewEvidenceApplicabilityPublishReceiptV1) Validate() error {
	if r.ContractVersion != ReviewEvidenceCurrentContractVersionV1 || strings.TrimSpace(r.PublishID) == "" || r.RequestDigest.Validate() != nil || r.Projection.Validate() != nil || r.CurrentIndex.Validate() != nil || r.CurrentIndex.CurrentProjection != r.Projection || r.CommittedUnixNano <= 0 || r.ReceiptDigest.Validate() != nil {
		return reviewEvidenceInvalidV1("Review evidence applicability publish receipt is incomplete")
	}
	if r.PublishID != string(r.RequestDigest) {
		return reviewEvidenceConflictV1("Review evidence applicability publish ID drifted")
	}
	copy := cloneReviewEvidenceCurrentV1(r)
	copy.ReceiptDigest = ""
	digest, err := core.CanonicalJSONDigest(reviewEvidenceCurrentCanonicalDomainV1, ReviewEvidenceCurrentContractVersionV1, "ReviewEvidenceApplicabilityPublishReceiptV1", copy)
	if err != nil {
		return err
	}
	if digest != r.ReceiptDigest {
		return reviewEvidenceConflictV1("Review evidence applicability publish receipt digest drifted")
	}
	return nil
}

func SealReviewEvidenceApplicabilityPublishReceiptV1(r ReviewEvidenceApplicabilityPublishReceiptV1) (ReviewEvidenceApplicabilityPublishReceiptV1, error) {
	if r.ContractVersion != "" && r.ContractVersion != ReviewEvidenceCurrentContractVersionV1 {
		return ReviewEvidenceApplicabilityPublishReceiptV1{}, reviewEvidenceInvalidV1("Review evidence applicability publish receipt version is invalid")
	}
	r.ContractVersion = ReviewEvidenceCurrentContractVersionV1
	if r.PublishID != "" && r.PublishID != string(r.RequestDigest) {
		return ReviewEvidenceApplicabilityPublishReceiptV1{}, reviewEvidenceConflictV1("Review evidence applicability supplied wrong publish ID")
	}
	r.PublishID = string(r.RequestDigest)
	provided := r.ReceiptDigest
	r.ReceiptDigest = ""
	digest, err := core.CanonicalJSONDigest(reviewEvidenceCurrentCanonicalDomainV1, ReviewEvidenceCurrentContractVersionV1, "ReviewEvidenceApplicabilityPublishReceiptV1", r)
	if err != nil {
		return ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if provided != "" && provided != digest {
		return ReviewEvidenceApplicabilityPublishReceiptV1{}, reviewEvidenceConflictV1("Review evidence applicability supplied wrong receipt digest")
	}
	r.ReceiptDigest = digest
	return r, r.Validate()
}

// ReviewEvidenceApplicabilityOwnerPublisherV1 is restricted to the Runtime
// Evidence applicability Owner. Review consumers receive only the Reader.
// Indeterminate publish replies recover by Inspect with the same PublishID;
// they never authorize a differently keyed mutation.
// Publish atomically writes immutable history and CASes the full current index;
// no receipt may escape unless both changes commit.
type ReviewEvidenceApplicabilityOwnerPublisherV1 interface {
	PublishReviewEvidenceApplicabilityV1(context.Context, PublishReviewEvidenceApplicabilityRequestV1) (ReviewEvidenceApplicabilityPublishReceiptV1, error)
	InspectReviewEvidenceApplicabilityPublishV1(context.Context, string) (ReviewEvidenceApplicabilityPublishReceiptV1, error)
}

func CloneReviewEvidenceApplicabilityPublishReceiptV1(r ReviewEvidenceApplicabilityPublishReceiptV1) ReviewEvidenceApplicabilityPublishReceiptV1 {
	return cloneReviewEvidenceCurrentV1(r)
}

func cloneReviewEvidenceCurrentV1[T any](value T) T {
	payload, _ := json.Marshal(value)
	var cloned T
	_ = json.Unmarshal(payload, &cloned)
	return cloned
}

func reviewEvidenceInvalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}

func reviewEvidenceConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, message)
}

func reviewEvidenceScopeV1(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceScopeConflict, message)
}

func reviewEvidenceTrustV1(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceTrustInvalid, message)
}

func reviewEvidenceStaleV1(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, message)
}

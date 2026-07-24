package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ReviewBindingAuthoritativeCurrentContractV1 = "praxis.runtime.review-binding-authoritative-current/v1"
	reviewBindingCurrentCanonicalDomainV1       = "praxis.runtime.review-binding-current"
)

// ReviewBindingSubjectV1 is the exact Review-owned Assignment/Target subject
// presented to the Runtime Binding Owner. The Binding Owner validates this
// nominal but does not own or reconstruct the referenced Review facts.
type ReviewBindingSubjectV1 struct {
	TenantID           core.TenantID `json:"tenant_id"`
	AssignmentID       string        `json:"assignment_id"`
	AssignmentRevision core.Revision `json:"assignment_revision"`
	AssignmentDigest   core.Digest   `json:"assignment_digest"`
	ReviewerID         string        `json:"reviewer_id"`
	TargetID           string        `json:"target_id"`
	TargetRevision     core.Revision `json:"target_revision"`
	TargetDigest       core.Digest   `json:"target_digest"`
}

func (s ReviewBindingSubjectV1) Validate() error {
	if strings.TrimSpace(string(s.TenantID)) == "" || validateEvidenceIDV2(s.AssignmentID) != nil || s.AssignmentRevision == 0 || validateEvidenceIDV2(s.ReviewerID) != nil || validateEvidenceIDV2(s.TargetID) != nil || s.TargetRevision == 0 {
		return reviewBindingInvalidV1("Review Binding subject is incomplete")
	}
	if s.AssignmentDigest.Validate() != nil || s.TargetDigest.Validate() != nil {
		return reviewBindingInvalidV1("Review Binding subject digests are invalid")
	}
	return nil
}

type ReviewBindingProjectionRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ReviewBindingProjectionRefV1) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || r.Digest.Validate() != nil {
		return reviewBindingInvalidV1("Review Binding projection ref is incomplete")
	}
	return nil
}

// ReviewBindingSetExactRefV1 is a narrow carrier of Binding Owner seals. It is
// not a copy or alias of the private BindingSet fact.
type ReviewBindingSetExactRefV1 struct {
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	SemanticDigest  core.Digest   `json:"semantic_digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r ReviewBindingSetExactRefV1) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || r.ExpiresUnixNano <= 0 || r.Digest.Validate() != nil || r.SemanticDigest.Validate() != nil {
		return reviewBindingInvalidV1("Review BindingSet exact ref is incomplete")
	}
	return nil
}

// ReviewBindingProjectionIdentityInputV1 is the only canonical identity body
// for a stable Review Binding Projection ID.
type ReviewBindingProjectionIdentityInputV1 struct {
	Source  ReviewComponentBindingRefV2 `json:"source"`
	Subject ReviewBindingSubjectV1      `json:"subject"`
}

func (i ReviewBindingProjectionIdentityInputV1) Validate() error {
	if i.Source.Validate() != nil || i.Subject.Validate() != nil {
		return reviewBindingInvalidV1("Review Binding projection identity is incomplete")
	}
	return nil
}

func DeriveReviewBindingProjectionIDV1(i ReviewBindingProjectionIdentityInputV1) (string, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(reviewBindingCurrentCanonicalDomainV1, ReviewBindingAuthoritativeCurrentContractV1, "ReviewBindingProjectionIdentityInputV1", i)
	return string(digest), err
}

type ReviewBindingConsumerAssociationIdentityInputV1 struct {
	ConsumerComponentID ComponentIDV2    `json:"consumer_component_id"`
	ConsumerCapability  CapabilityNameV2 `json:"consumer_capability"`
	SourceComponentID   ComponentIDV2    `json:"source_component_id"`
	SourceCapability    CapabilityNameV2 `json:"source_capability"`
}

func (i ReviewBindingConsumerAssociationIdentityInputV1) Validate() error {
	if ValidateNamespacedNameV2(NamespacedNameV2(i.ConsumerComponentID)) != nil || ValidateNamespacedNameV2(NamespacedNameV2(i.ConsumerCapability)) != nil || ValidateNamespacedNameV2(NamespacedNameV2(i.SourceComponentID)) != nil || ValidateNamespacedNameV2(NamespacedNameV2(i.SourceCapability)) != nil {
		return reviewBindingInvalidV1("Review Binding Consumer association identity is incomplete")
	}
	return nil
}

func DeriveReviewBindingConsumerAssociationIDV1(i ReviewBindingConsumerAssociationIdentityInputV1) (string, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(reviewBindingCurrentCanonicalDomainV1, ReviewBindingAuthoritativeCurrentContractV1, "ReviewBindingConsumerAssociationIdentityInputV1", i)
	return string(digest), err
}

type ReviewBindingConsumerAssociationRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ReviewBindingConsumerAssociationRefV1) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || r.Digest.Validate() != nil {
		return reviewBindingInvalidV1("Review Binding Consumer association ref is incomplete")
	}
	return nil
}

type ReviewBindingConsumerAssociationCurrentProjectionV1 struct {
	ContractVersion  string                                `json:"contract_version"`
	Ref              ReviewBindingConsumerAssociationRefV1 `json:"ref"`
	Consumer         ProviderBindingRefV2                  `json:"consumer"`
	Source           ReviewComponentBindingRefV2           `json:"source"`
	Current          bool                                  `json:"current"`
	CheckedUnixNano  int64                                 `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                 `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                           `json:"projection_digest"`
}

func (p ReviewBindingConsumerAssociationCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ReviewBindingAuthoritativeCurrentContractV1 || p.Ref.Validate() != nil || p.Consumer.Validate() != nil || p.Source.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil {
		return reviewBindingInvalidV1("Review Binding Consumer association projection is incomplete")
	}
	identity := ReviewBindingConsumerAssociationIdentityInputV1{
		ConsumerComponentID: p.Consumer.ComponentID,
		ConsumerCapability:  p.Consumer.Capability,
		SourceComponentID:   p.Source.ComponentID,
		SourceCapability:    p.Source.Capability,
	}
	id, err := DeriveReviewBindingConsumerAssociationIDV1(identity)
	if err != nil || id != p.Ref.ID {
		return reviewBindingConflictV1("Review Binding Consumer association ID drifted")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.Ref.Digest || digest != p.ProjectionDigest {
		return reviewBindingConflictV1("Review Binding Consumer association digest drifted")
	}
	return nil
}

func (p ReviewBindingConsumerAssociationCurrentProjectionV1) ValidateCurrent(expected ReviewBindingConsumerAssociationRefV1, expectedConsumer ProviderBindingRefV2, expectedSource ReviewComponentBindingRefV2, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if expected.Validate() != nil || expectedConsumer.Validate() != nil || expectedSource.Validate() != nil {
		return reviewBindingInvalidV1("Review Binding Consumer association expected coordinates are invalid")
	}
	if p.Ref != expected || p.Consumer != expectedConsumer || p.Source != expectedSource {
		return reviewBindingConflictV1("Review Binding Consumer association current coordinates drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Binding Consumer association clock regressed")
	}
	if !p.Current || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Review Binding Consumer association is inactive or expired")
	}
	return nil
}

func (p ReviewBindingConsumerAssociationCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.Ref.Digest = ""
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest(reviewBindingCurrentCanonicalDomainV1, ReviewBindingAuthoritativeCurrentContractV1, "ReviewBindingConsumerAssociationCurrentProjectionV1", copy)
}

func SealReviewBindingConsumerAssociationCurrentProjectionV1(p ReviewBindingConsumerAssociationCurrentProjectionV1) (ReviewBindingConsumerAssociationCurrentProjectionV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != ReviewBindingAuthoritativeCurrentContractV1 {
		return ReviewBindingConsumerAssociationCurrentProjectionV1{}, reviewBindingInvalidV1("Review Binding Consumer association contract version is invalid")
	}
	p.ContractVersion = ReviewBindingAuthoritativeCurrentContractV1
	identity := ReviewBindingConsumerAssociationIdentityInputV1{ConsumerComponentID: p.Consumer.ComponentID, ConsumerCapability: p.Consumer.Capability, SourceComponentID: p.Source.ComponentID, SourceCapability: p.Source.Capability}
	id, err := DeriveReviewBindingConsumerAssociationIDV1(identity)
	if err != nil {
		return ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if p.Ref.ID != "" && p.Ref.ID != id {
		return ReviewBindingConsumerAssociationCurrentProjectionV1{}, reviewBindingConflictV1("Review Binding Consumer association supplied wrong ID")
	}
	p.Ref.ID = id
	providedRefDigest, providedProjectionDigest := p.Ref.Digest, p.ProjectionDigest
	p.Ref.Digest, p.ProjectionDigest = "", ""
	digest, err := p.DigestV1()
	if err != nil {
		return ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if (providedRefDigest != "" && providedRefDigest != digest) || (providedProjectionDigest != "" && providedProjectionDigest != digest) {
		return ReviewBindingConsumerAssociationCurrentProjectionV1{}, reviewBindingConflictV1("Review Binding Consumer association supplied wrong digest")
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p, p.Validate()
}

type ReviewBindingMemberCurrentRefV1 struct {
	ComponentID                 ComponentIDV2 `json:"component_id"`
	BindingID                   string        `json:"binding_id"`
	BindingRevision             core.Revision `json:"binding_revision"`
	BindingFactDigest           core.Digest   `json:"binding_fact_digest"`
	ManifestDigest              core.Digest   `json:"manifest_digest"`
	ArtifactDigest              core.Digest   `json:"artifact_digest"`
	SetGrantSetDigest           core.Digest   `json:"set_grant_set_digest"`
	FactGrantSetDigest          core.Digest   `json:"fact_grant_set_digest"`
	BindingFactExpiresUnixNano  int64         `json:"binding_fact_expires_unix_nano"`
	SetGrantMinExpiresUnixNano  int64         `json:"set_grant_min_expires_unix_nano"`
	FactGrantMinExpiresUnixNano int64         `json:"fact_grant_min_expires_unix_nano"`
}

func (r ReviewBindingMemberCurrentRefV1) Validate() error {
	if ValidateNamespacedNameV2(NamespacedNameV2(r.ComponentID)) != nil || validateEvidenceIDV2(r.BindingID) != nil || r.BindingRevision == 0 || r.BindingFactExpiresUnixNano <= 0 || r.SetGrantMinExpiresUnixNano <= 0 || r.FactGrantMinExpiresUnixNano <= 0 {
		return reviewBindingInvalidV1("Review Binding member current ref is incomplete")
	}
	for _, d := range []core.Digest{r.BindingFactDigest, r.ManifestDigest, r.ArtifactDigest, r.SetGrantSetDigest, r.FactGrantSetDigest} {
		if d.Validate() != nil {
			return reviewBindingInvalidV1("Review Binding member current ref digest is invalid")
		}
	}
	if r.SetGrantSetDigest != r.FactGrantSetDigest || r.SetGrantMinExpiresUnixNano != r.FactGrantMinExpiresUnixNano {
		return reviewBindingConflictV1("Review Binding member Set and Fact Grant closure drifted")
	}
	return nil
}

type ReviewBindingSelectedGrantRefV1 struct {
	ComponentID     ComponentIDV2    `json:"component_id"`
	BindingID       string           `json:"binding_id"`
	BindingRevision core.Revision    `json:"binding_revision"`
	Capability      CapabilityNameV2 `json:"capability"`
	SetGrantDigest  core.Digest      `json:"set_grant_digest"`
	FactGrantDigest core.Digest      `json:"fact_grant_digest"`
	ExpiresUnixNano int64            `json:"expires_unix_nano"`
}

func (r ReviewBindingSelectedGrantRefV1) Validate() error {
	if ValidateNamespacedNameV2(NamespacedNameV2(r.ComponentID)) != nil || validateEvidenceIDV2(r.BindingID) != nil || r.BindingRevision == 0 || ValidateNamespacedNameV2(NamespacedNameV2(r.Capability)) != nil || r.SetGrantDigest.Validate() != nil || r.FactGrantDigest.Validate() != nil || r.ExpiresUnixNano <= 0 {
		return reviewBindingInvalidV1("Review Binding selected Grant ref is incomplete")
	}
	if r.SetGrantDigest != r.FactGrantDigest {
		return reviewBindingConflictV1("Review Binding selected Set and Fact Grant drifted")
	}
	return nil
}

// ReviewBindingAuthoritativeClosureInputV1 is the immutable canonical closure
// body. Raw BindingSet, BindingFact and Grant values remain Owner-private.
type ReviewBindingAuthoritativeClosureInputV1 struct {
	Source              ReviewComponentBindingRefV2                         `json:"source"`
	BindingSet          ReviewBindingSetExactRefV1                          `json:"binding_set"`
	Members             []ReviewBindingMemberCurrentRefV1                   `json:"members"`
	SelectedGrant       ReviewBindingSelectedGrantRefV1                     `json:"selected_grant"`
	ConsumerAssociation ReviewBindingConsumerAssociationCurrentProjectionV1 `json:"consumer_association"`
	ConsumerBinding     ProviderBindingCurrentProjectionV2                  `json:"consumer_binding"`
	ExpiresUnixNano     int64                                               `json:"expires_unix_nano"`
}

func (i ReviewBindingAuthoritativeClosureInputV1) Validate() error {
	if i.Source.Validate() != nil || i.BindingSet.Validate() != nil || len(i.Members) == 0 || i.SelectedGrant.Validate() != nil || i.ConsumerAssociation.Validate() != nil || validateProviderBindingProjectionShapeV1(i.ConsumerBinding) != nil || i.ExpiresUnixNano <= 0 {
		return reviewBindingInvalidV1("Review Binding authoritative closure is incomplete")
	}
	if i.Source.BindingSetID != i.BindingSet.ID || i.Source.BindingSetRevision != i.BindingSet.Revision || i.ConsumerAssociation.Source != i.Source || i.ConsumerAssociation.Consumer != i.ConsumerBinding.Ref || !i.ConsumerAssociation.Current || i.ConsumerBinding.State != ProviderBindingCurrentActiveV2 {
		return reviewBindingConflictV1("Review Binding authoritative closure coordinates drifted")
	}
	minimum := i.BindingSet.ExpiresUnixNano
	foundSelected := false
	lastComponent := ""
	for index, member := range i.Members {
		if err := member.Validate(); err != nil {
			return err
		}
		component := string(member.ComponentID)
		if index > 0 && component <= lastComponent {
			return reviewBindingInvalidV1("Review Binding members must be sorted and unique")
		}
		lastComponent = component
		minimum = minReviewBindingExpiryV1(minimum, member.BindingFactExpiresUnixNano, member.SetGrantMinExpiresUnixNano, member.FactGrantMinExpiresUnixNano)
		if member.ComponentID == i.SelectedGrant.ComponentID && member.BindingID == i.SelectedGrant.BindingID && member.BindingRevision == i.SelectedGrant.BindingRevision {
			if foundSelected {
				return reviewBindingConflictV1("Review Binding selected Grant matches multiple members")
			}
			foundSelected = true
			if member.ManifestDigest != i.Source.ManifestDigest || member.ArtifactDigest != i.Source.ArtifactDigest {
				return reviewBindingConflictV1("Review Binding selected member and Source drifted")
			}
		}
	}
	if !foundSelected || i.SelectedGrant.ComponentID != i.Source.ComponentID || i.SelectedGrant.Capability != i.Source.Capability {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Review Binding Source has no unique selected Grant")
	}
	minimum = minReviewBindingExpiryV1(minimum, i.SelectedGrant.ExpiresUnixNano, i.ConsumerAssociation.ExpiresUnixNano, i.ConsumerBinding.ExpiresUnixNano)
	if i.ExpiresUnixNano != minimum {
		return reviewBindingConflictV1("Review Binding authoritative closure does not carry the true minimum TTL")
	}
	return nil
}

func (i ReviewBindingAuthoritativeClosureInputV1) DigestV1() (core.Digest, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	copy := i
	copy.Members = append([]ReviewBindingMemberCurrentRefV1(nil), i.Members...)
	return core.CanonicalJSONDigest(reviewBindingCurrentCanonicalDomainV1, ReviewBindingAuthoritativeCurrentContractV1, "ReviewBindingAuthoritativeClosureInputV1", copy)
}

type ReviewBindingCurrentStateV1 string

const (
	ReviewBindingCurrentActiveV1     ReviewBindingCurrentStateV1 = "active"
	ReviewBindingCurrentRevokedV1    ReviewBindingCurrentStateV1 = "revoked"
	ReviewBindingCurrentExpiredV1    ReviewBindingCurrentStateV1 = "expired"
	ReviewBindingCurrentSupersededV1 ReviewBindingCurrentStateV1 = "superseded"
)

type ReviewBindingCurrentProjectionV1 struct {
	ContractVersion string                       `json:"contract_version"`
	Ref             ReviewBindingProjectionRefV1 `json:"ref"`
	Source          ReviewComponentBindingRefV2  `json:"source"`
	Subject         ReviewBindingSubjectV1       `json:"subject"`
	State           ReviewBindingCurrentStateV1  `json:"state"`
	Current         bool                         `json:"current"`

	BindingSetID              string        `json:"binding_set_id"`
	BindingSetRevision        core.Revision `json:"binding_set_revision"`
	BindingSetDigest          core.Digest   `json:"binding_set_digest"`
	BindingSetSemanticDigest  core.Digest   `json:"binding_set_semantic_digest"`
	BindingSetExpiresUnixNano int64         `json:"binding_set_expires_unix_nano"`

	Members             []ReviewBindingMemberCurrentRefV1                   `json:"members"`
	SelectedGrant       ReviewBindingSelectedGrantRefV1                     `json:"selected_grant"`
	ConsumerAssociation ReviewBindingConsumerAssociationCurrentProjectionV1 `json:"consumer_association"`
	ConsumerBinding     ProviderBindingCurrentProjectionV2                  `json:"consumer_binding"`
	ClosureDigest       core.Digest                                         `json:"closure_digest"`

	CheckedUnixNano  int64       `json:"checked_unix_nano"`
	ExpiresUnixNano  int64       `json:"expires_unix_nano"`
	ProjectionDigest core.Digest `json:"projection_digest"`
}

func (p ReviewBindingCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ReviewBindingAuthoritativeCurrentContractV1 || p.Ref.Validate() != nil || p.Source.Validate() != nil || p.Subject.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ClosureDigest.Validate() != nil || p.ProjectionDigest.Validate() != nil {
		return reviewBindingInvalidV1("Review Binding current projection is incomplete")
	}
	if !validReviewBindingStateV1(p.State) || (p.State == ReviewBindingCurrentActiveV1) != p.Current {
		return reviewBindingInvalidV1("Review Binding State and Current truth values are invalid")
	}
	identity := ReviewBindingProjectionIdentityInputV1{Source: p.Source, Subject: p.Subject}
	id, err := DeriveReviewBindingProjectionIDV1(identity)
	if err != nil || id != p.Ref.ID {
		return reviewBindingConflictV1("Review Binding projection ID drifted")
	}
	closure := p.closureInputV1()
	if err := closure.Validate(); err != nil {
		return err
	}
	closureDigest, err := closure.DigestV1()
	if err != nil || closureDigest != p.ClosureDigest {
		return reviewBindingConflictV1("Review Binding authoritative closure digest drifted")
	}
	if p.ExpiresUnixNano != closure.ExpiresUnixNano {
		return reviewBindingConflictV1("Review Binding projection TTL drifted from its authoritative closure")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.Ref.Digest || digest != p.ProjectionDigest {
		return reviewBindingConflictV1("Review Binding projection digest drifted")
	}
	return nil
}

func (p ReviewBindingCurrentProjectionV1) ValidateCurrent(expectedRef ReviewBindingProjectionRefV1, expectedSource ReviewComponentBindingRefV2, expectedSubject ReviewBindingSubjectV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if expectedRef.Validate() != nil || expectedSource.Validate() != nil || expectedSubject.Validate() != nil {
		return reviewBindingInvalidV1("Review Binding expected current coordinates are invalid")
	}
	if p.Ref != expectedRef || p.Source != expectedSource || p.Subject != expectedSubject {
		return reviewBindingConflictV1("Review Binding current coordinates drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Binding current clock regressed")
	}
	if p.State != ReviewBindingCurrentActiveV1 || !p.Current || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Review Binding projection is inactive or expired")
	}
	if err := p.ConsumerAssociation.ValidateCurrent(p.ConsumerAssociation.Ref, p.ConsumerBinding.Ref, p.Source, now); err != nil {
		return err
	}
	if err := p.ConsumerBinding.ValidateCurrent(p.ConsumerBinding.Ref, now); err != nil {
		return err
	}
	return nil
}

func (p ReviewBindingCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p.CloneV1()
	copy.Ref.Digest = ""
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest(reviewBindingCurrentCanonicalDomainV1, ReviewBindingAuthoritativeCurrentContractV1, "ReviewBindingCurrentProjectionV1", copy)
}

func (p ReviewBindingCurrentProjectionV1) CloneV1() ReviewBindingCurrentProjectionV1 {
	p.Members = append([]ReviewBindingMemberCurrentRefV1(nil), p.Members...)
	return p
}

func SealReviewBindingCurrentProjectionV1(p ReviewBindingCurrentProjectionV1) (ReviewBindingCurrentProjectionV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != ReviewBindingAuthoritativeCurrentContractV1 {
		return ReviewBindingCurrentProjectionV1{}, reviewBindingInvalidV1("Review Binding contract version is invalid")
	}
	p.ContractVersion = ReviewBindingAuthoritativeCurrentContractV1
	if p.Ref.Revision == 0 {
		return ReviewBindingCurrentProjectionV1{}, reviewBindingInvalidV1("Review Binding projection revision is required")
	}
	id, err := DeriveReviewBindingProjectionIDV1(ReviewBindingProjectionIdentityInputV1{Source: p.Source, Subject: p.Subject})
	if err != nil {
		return ReviewBindingCurrentProjectionV1{}, err
	}
	if p.Ref.ID != "" && p.Ref.ID != id {
		return ReviewBindingCurrentProjectionV1{}, reviewBindingConflictV1("Review Binding projection supplied wrong ID")
	}
	p.Ref.ID = id
	closure := p.closureInputV1()
	closureDigest, err := closure.DigestV1()
	if err != nil {
		return ReviewBindingCurrentProjectionV1{}, err
	}
	if p.ClosureDigest != "" && p.ClosureDigest != closureDigest {
		return ReviewBindingCurrentProjectionV1{}, reviewBindingConflictV1("Review Binding projection supplied wrong closure digest")
	}
	p.ClosureDigest = closureDigest
	providedRefDigest, providedProjectionDigest := p.Ref.Digest, p.ProjectionDigest
	p.Ref.Digest, p.ProjectionDigest = "", ""
	digest, err := p.DigestV1()
	if err != nil {
		return ReviewBindingCurrentProjectionV1{}, err
	}
	if (providedRefDigest != "" && providedRefDigest != digest) || (providedProjectionDigest != "" && providedProjectionDigest != digest) {
		return ReviewBindingCurrentProjectionV1{}, reviewBindingConflictV1("Review Binding projection supplied wrong digest")
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p, p.Validate()
}

func (p ReviewBindingCurrentProjectionV1) closureInputV1() ReviewBindingAuthoritativeClosureInputV1 {
	return ReviewBindingAuthoritativeClosureInputV1{
		Source: p.Source,
		BindingSet: ReviewBindingSetExactRefV1{
			ID: p.BindingSetID, Revision: p.BindingSetRevision, Digest: p.BindingSetDigest,
			SemanticDigest: p.BindingSetSemanticDigest, ExpiresUnixNano: p.BindingSetExpiresUnixNano,
		},
		Members: append([]ReviewBindingMemberCurrentRefV1(nil), p.Members...), SelectedGrant: p.SelectedGrant,
		ConsumerAssociation: p.ConsumerAssociation, ConsumerBinding: p.ConsumerBinding, ExpiresUnixNano: p.ExpiresUnixNano,
	}
}

type ResolveReviewBindingCurrentRequestV1 struct {
	Source  ReviewComponentBindingRefV2 `json:"source"`
	Subject ReviewBindingSubjectV1      `json:"subject"`
}

func (r ResolveReviewBindingCurrentRequestV1) Validate() error {
	return ReviewBindingProjectionIdentityInputV1{Source: r.Source, Subject: r.Subject}.Validate()
}

type InspectReviewBindingProjectionRequestV1 struct {
	Ref             ReviewBindingProjectionRefV1 `json:"ref"`
	ExpectedSource  ReviewComponentBindingRefV2  `json:"expected_source"`
	ExpectedSubject ReviewBindingSubjectV1       `json:"expected_subject"`
}

func (r InspectReviewBindingProjectionRequestV1) Validate() error {
	if r.Ref.Validate() != nil || r.ExpectedSource.Validate() != nil || r.ExpectedSubject.Validate() != nil {
		return reviewBindingInvalidV1("Review Binding historical Inspect request is incomplete")
	}
	id, err := DeriveReviewBindingProjectionIDV1(ReviewBindingProjectionIdentityInputV1{Source: r.ExpectedSource, Subject: r.ExpectedSubject})
	if err != nil || id != r.Ref.ID {
		return reviewBindingConflictV1("Review Binding historical Inspect coordinates drifted")
	}
	return nil
}

type InspectCurrentReviewBindingRequestV1 struct {
	ExpectedRef     ReviewBindingProjectionRefV1 `json:"expected_ref"`
	ExpectedSource  ReviewComponentBindingRefV2  `json:"expected_source"`
	ExpectedSubject ReviewBindingSubjectV1       `json:"expected_subject"`
}

func (r InspectCurrentReviewBindingRequestV1) Validate() error {
	return InspectReviewBindingProjectionRequestV1{Ref: r.ExpectedRef, ExpectedSource: r.ExpectedSource, ExpectedSubject: r.ExpectedSubject}.Validate()
}

type ReviewBindingProjectionPublishRefV1 struct {
	ID     string      `json:"id"`
	Digest core.Digest `json:"digest"`
}

func (r ReviewBindingProjectionPublishRefV1) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Digest.Validate() != nil || r.ID != string(r.Digest) {
		return reviewBindingInvalidV1("Review Binding projection Publish ref is incomplete")
	}
	return nil
}

type CreateReviewBindingProjectionCommandInputV1 struct {
	Source      ReviewComponentBindingRefV2           `json:"source"`
	Subject     ReviewBindingSubjectV1                `json:"subject"`
	Association ReviewBindingConsumerAssociationRefV1 `json:"association"`
}

func (i CreateReviewBindingProjectionCommandInputV1) Validate() error {
	if i.Source.Validate() != nil || i.Subject.Validate() != nil || i.Association.Validate() != nil {
		return reviewBindingInvalidV1("Create Review Binding projection input is incomplete")
	}
	return nil
}

type CompareAndSwapReviewBindingProjectionCommandInputV1 struct {
	ExpectedCurrent ReviewBindingProjectionRefV1          `json:"expected_current"`
	Source          ReviewComponentBindingRefV2           `json:"source"`
	Subject         ReviewBindingSubjectV1                `json:"subject"`
	Association     ReviewBindingConsumerAssociationRefV1 `json:"association"`
}

func (i CompareAndSwapReviewBindingProjectionCommandInputV1) Validate() error {
	if i.ExpectedCurrent.Validate() != nil || i.Source.Validate() != nil || i.Subject.Validate() != nil || i.Association.Validate() != nil {
		return reviewBindingInvalidV1("CAS Review Binding projection input is incomplete")
	}
	id, err := DeriveReviewBindingProjectionIDV1(ReviewBindingProjectionIdentityInputV1{Source: i.Source, Subject: i.Subject})
	if err != nil || id != i.ExpectedCurrent.ID {
		return reviewBindingConflictV1("CAS Review Binding projection ExpectedCurrent binds another identity")
	}
	return nil
}

type CreateReviewBindingProjectionRequestV1 struct {
	PublishRef ReviewBindingProjectionPublishRefV1         `json:"publish_ref"`
	Input      CreateReviewBindingProjectionCommandInputV1 `json:"input"`
}

func (r CreateReviewBindingProjectionRequestV1) Validate() error {
	if r.PublishRef.Validate() != nil || r.Input.Validate() != nil {
		return reviewBindingInvalidV1("Create Review Binding projection request is incomplete")
	}
	expected, err := DeriveCreateReviewBindingProjectionPublishRefV1(r.Input)
	if err != nil || expected != r.PublishRef {
		return reviewBindingConflictV1("Create Review Binding projection Publish ref drifted")
	}
	return nil
}

type CompareAndSwapReviewBindingProjectionRequestV1 struct {
	PublishRef ReviewBindingProjectionPublishRefV1                 `json:"publish_ref"`
	Input      CompareAndSwapReviewBindingProjectionCommandInputV1 `json:"input"`
}

func (r CompareAndSwapReviewBindingProjectionRequestV1) Validate() error {
	if r.PublishRef.Validate() != nil || r.Input.Validate() != nil {
		return reviewBindingInvalidV1("CAS Review Binding projection request is incomplete")
	}
	expected, err := DeriveCompareAndSwapReviewBindingProjectionPublishRefV1(r.Input)
	if err != nil || expected != r.PublishRef {
		return reviewBindingConflictV1("CAS Review Binding projection Publish ref drifted")
	}
	return nil
}

func DeriveCreateReviewBindingProjectionPublishRefV1(i CreateReviewBindingProjectionCommandInputV1) (ReviewBindingProjectionPublishRefV1, error) {
	if err := i.Validate(); err != nil {
		return ReviewBindingProjectionPublishRefV1{}, err
	}
	digest, err := core.CanonicalJSONDigest(reviewBindingCurrentCanonicalDomainV1, ReviewBindingAuthoritativeCurrentContractV1, "CreateReviewBindingProjectionCommandInputV1", i)
	return ReviewBindingProjectionPublishRefV1{ID: string(digest), Digest: digest}, err
}

func DeriveCompareAndSwapReviewBindingProjectionPublishRefV1(i CompareAndSwapReviewBindingProjectionCommandInputV1) (ReviewBindingProjectionPublishRefV1, error) {
	if err := i.Validate(); err != nil {
		return ReviewBindingProjectionPublishRefV1{}, err
	}
	digest, err := core.CanonicalJSONDigest(reviewBindingCurrentCanonicalDomainV1, ReviewBindingAuthoritativeCurrentContractV1, "CompareAndSwapReviewBindingProjectionCommandInputV1", i)
	return ReviewBindingProjectionPublishRefV1{ID: string(digest), Digest: digest}, err
}

type ReviewBindingProjectionPublishReceiptV1 struct {
	ContractVersion string                              `json:"contract_version"`
	PublishRef      ReviewBindingProjectionPublishRefV1 `json:"publish_ref"`
	Projection      ReviewBindingProjectionRefV1        `json:"projection"`
	CurrentIndex    ReviewBindingProjectionRefV1        `json:"current_index"`
	HighestRevision core.Revision                       `json:"highest_revision"`
	Digest          core.Digest                         `json:"digest"`
}

func (r ReviewBindingProjectionPublishReceiptV1) Validate() error {
	if r.ContractVersion != ReviewBindingAuthoritativeCurrentContractV1 || r.PublishRef.Validate() != nil || r.Projection.Validate() != nil || r.CurrentIndex.Validate() != nil || r.HighestRevision == 0 || r.Digest.Validate() != nil {
		return reviewBindingInvalidV1("Review Binding projection Publish receipt is incomplete")
	}
	if r.Projection != r.CurrentIndex || r.HighestRevision != r.Projection.Revision {
		return reviewBindingConflictV1("Review Binding projection Publish receipt atomic closure drifted")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return reviewBindingConflictV1("Review Binding projection Publish receipt digest drifted")
	}
	return nil
}

func (r ReviewBindingProjectionPublishReceiptV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest(reviewBindingCurrentCanonicalDomainV1, ReviewBindingAuthoritativeCurrentContractV1, "ReviewBindingProjectionPublishReceiptV1", copy)
}

func SealReviewBindingProjectionPublishReceiptV1(r ReviewBindingProjectionPublishReceiptV1) (ReviewBindingProjectionPublishReceiptV1, error) {
	if r.ContractVersion != "" && r.ContractVersion != ReviewBindingAuthoritativeCurrentContractV1 {
		return ReviewBindingProjectionPublishReceiptV1{}, reviewBindingInvalidV1("Review Binding projection Publish receipt contract version is invalid")
	}
	r.ContractVersion = ReviewBindingAuthoritativeCurrentContractV1
	provided := r.Digest
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if provided != "" && provided != digest {
		return ReviewBindingProjectionPublishReceiptV1{}, reviewBindingConflictV1("Review Binding projection Publish receipt supplied wrong digest")
	}
	r.Digest = digest
	return r, r.Validate()
}

type ReviewBindingAuthoritativeCurrentReaderV1 interface {
	ResolveCurrentReviewBindingV1(context.Context, ResolveReviewBindingCurrentRequestV1) (ReviewBindingProjectionRefV1, error)
	InspectReviewBindingProjectionV1(context.Context, InspectReviewBindingProjectionRequestV1) (ReviewBindingCurrentProjectionV1, error)
	InspectCurrentReviewBindingV1(context.Context, InspectCurrentReviewBindingRequestV1) (ReviewBindingCurrentProjectionV1, error)
}

type ReviewBindingConsumerAssociationCurrentReaderV1 interface {
	InspectCurrentReviewBindingConsumerAssociationV1(context.Context, ReviewBindingConsumerAssociationRefV1) (ReviewBindingConsumerAssociationCurrentProjectionV1, error)
}

// ReviewBindingProjectionPublisherV1 is held only by the Runtime Binding Owner
// control path. Review and production consumers receive the Reader interface.
type ReviewBindingProjectionPublisherV1 interface {
	CreateReviewBindingProjectionV1(context.Context, CreateReviewBindingProjectionRequestV1) (ReviewBindingProjectionPublishReceiptV1, error)
	CompareAndSwapReviewBindingProjectionV1(context.Context, CompareAndSwapReviewBindingProjectionRequestV1) (ReviewBindingProjectionPublishReceiptV1, error)
	InspectReviewBindingProjectionPublishV1(context.Context, ReviewBindingProjectionPublishRefV1) (ReviewBindingProjectionPublishReceiptV1, error)
}

func validateProviderBindingProjectionShapeV1(p ProviderBindingCurrentProjectionV2) error {
	if p.ContractVersion != ProviderBindingCurrentnessContractVersionV2 || p.Ref.Validate() != nil || p.BindingSetDigest.Validate() != nil || p.BindingSetSemanticDigest.Validate() != nil || validateEvidenceIDV2(p.BindingID) != nil || p.BindingRevision == 0 || p.GrantDigest.Validate() != nil || p.ProjectionDigest.Validate() != nil || p.IssuedUnixNano <= 0 || p.ExpiresUnixNano <= p.IssuedUnixNano {
		return reviewBindingInvalidV1("Review Binding Consumer current projection is incomplete")
	}
	if p.State != ProviderBindingCurrentActiveV2 && p.State != ProviderBindingCurrentRevokedV2 && p.State != ProviderBindingCurrentExpiredV2 {
		return reviewBindingInvalidV1("Review Binding Consumer current state is invalid")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return reviewBindingConflictV1("Review Binding Consumer current projection digest drifted")
	}
	return nil
}

func validReviewBindingStateV1(state ReviewBindingCurrentStateV1) bool {
	switch state {
	case ReviewBindingCurrentActiveV1, ReviewBindingCurrentRevokedV1, ReviewBindingCurrentExpiredV1, ReviewBindingCurrentSupersededV1:
		return true
	default:
		return false
	}
}

func minReviewBindingExpiryV1(first int64, rest ...int64) int64 {
	minimum := first
	for _, value := range rest {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func reviewBindingInvalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}

func reviewBindingConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}

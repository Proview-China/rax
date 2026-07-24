package ports

import (
	"bytes"
	"context"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ReviewArtifactCurrentContractV2        = "praxis.runtime.review-artifact-current/v2"
	ReviewEnvironmentCurrentContractV2     = "praxis.runtime.review-environment-current/v2"
	ReviewValidationScopeCurrentContractV2 = "praxis.runtime.review-validation-scope-current/v2"

	reviewArtifactCurrentDomainV2        = "praxis.runtime.review-artifact-current"
	reviewEnvironmentCurrentDomainV2     = "praxis.runtime.review-environment-current"
	reviewValidationScopeCurrentDomainV2 = "praxis.runtime.review-validation-scope-current"
)

type ReviewGroundingCurrentStateV2 string

const (
	ReviewGroundingCurrentActiveV2     ReviewGroundingCurrentStateV2 = "active"
	ReviewGroundingCurrentRevokedV2    ReviewGroundingCurrentStateV2 = "revoked"
	ReviewGroundingCurrentSupersededV2 ReviewGroundingCurrentStateV2 = "superseded"
)

type ReviewGroundingOwnerRefV2 struct {
	Binding        ReviewComponentBindingRefV2 `json:"binding"`
	SourceContract NamespacedNameV2            `json:"source_contract"`
}

func (r ReviewGroundingOwnerRefV2) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	return ValidateNamespacedNameV2(r.SourceContract)
}

type ReviewArtifactExactSourceRefV2 struct {
	Kind        NamespacedNameV2          `json:"kind"`
	Owner       ReviewGroundingOwnerRefV2 `json:"owner"`
	TenantID    core.TenantID             `json:"tenant_id"`
	ID          string                    `json:"id"`
	Revision    core.Revision             `json:"revision"`
	Digest      core.Digest               `json:"digest"`
	ScopeDigest core.Digest               `json:"scope_digest"`
}

func (r ReviewArtifactExactSourceRefV2) Validate() error {
	return validateGroundingExactV2(r.Kind, r.Owner, r.TenantID, r.ID, r.Revision, r.Digest, r.ScopeDigest)
}

type ReviewEnvironmentExactRefV2 struct {
	Kind        NamespacedNameV2          `json:"kind"`
	Owner       ReviewGroundingOwnerRefV2 `json:"owner"`
	TenantID    core.TenantID             `json:"tenant_id"`
	ID          string                    `json:"id"`
	Revision    core.Revision             `json:"revision"`
	Digest      core.Digest               `json:"digest"`
	ScopeDigest core.Digest               `json:"scope_digest"`
}

func (r ReviewEnvironmentExactRefV2) Validate() error {
	return validateGroundingExactV2(r.Kind, r.Owner, r.TenantID, r.ID, r.Revision, r.Digest, r.ScopeDigest)
}

type ReviewValidationScopeSourceIdentityV2 struct {
	Kind     NamespacedNameV2 `json:"kind"`
	TenantID core.TenantID    `json:"tenant_id"`
	ID       string           `json:"id"`
}

func (r ReviewValidationScopeSourceIdentityV2) Validate() error {
	if ValidateNamespacedNameV2(r.Kind) != nil || strings.TrimSpace(string(r.TenantID)) == "" || strings.TrimSpace(r.ID) == "" {
		return groundingInvalidV2("validation scope source identity is incomplete")
	}
	return nil
}

type ReviewValidationScopeExactRefV2 struct {
	Source      ReviewValidationScopeSourceIdentityV2 `json:"source"`
	Owner       ReviewGroundingOwnerRefV2             `json:"owner"`
	Revision    core.Revision                         `json:"revision"`
	Digest      core.Digest                           `json:"digest"`
	ScopeDigest core.Digest                           `json:"scope_digest"`
}

func (r ReviewValidationScopeExactRefV2) Validate() error {
	if r.Source.Validate() != nil || r.Owner.Validate() != nil || r.Revision == 0 || r.Digest.Validate() != nil || r.ScopeDigest.Validate() != nil {
		return groundingInvalidV2("validation scope exact ref is incomplete")
	}
	return nil
}

type ReviewArtifactLocatorV2 struct {
	Kind          NamespacedNameV2 `json:"kind"`
	Schema        SchemaRefV2      `json:"schema"`
	Payload       OpaquePayloadV2  `json:"payload"`
	LocatorDigest core.Digest      `json:"locator_digest"`
}

func (v ReviewArtifactLocatorV2) Validate() error {
	if ValidateNamespacedNameV2(v.Kind) != nil || v.Schema.Validate() != nil || v.Payload.Validate() != nil {
		return groundingInvalidV2("artifact locator is incomplete")
	}
	if v.Payload.Schema != v.Schema {
		return groundingConflictV2("artifact locator payload schema drifted")
	}
	digest, err := DigestReviewArtifactLocatorV2(v)
	if err != nil {
		return err
	}
	if digest != v.LocatorDigest {
		return groundingDigestConflictV2("artifact locator digest drifted")
	}
	return nil
}

func DigestReviewArtifactLocatorV2(v ReviewArtifactLocatorV2) (core.Digest, error) {
	copy := v
	copy.LocatorDigest = ""
	if ValidateNamespacedNameV2(copy.Kind) != nil || copy.Schema.Validate() != nil || copy.Payload.Validate() != nil || copy.Payload.Schema != copy.Schema {
		return "", groundingInvalidV2("artifact locator body is incomplete")
	}
	return core.CanonicalJSONDigest("praxis.review.artifact-locator/body/v2", "2.0.0", "ReviewArtifactLocatorV2", copy)
}

func SealReviewArtifactLocatorV2(v ReviewArtifactLocatorV2) (ReviewArtifactLocatorV2, error) {
	provided := v.LocatorDigest
	v.LocatorDigest = ""
	digest, err := DigestReviewArtifactLocatorV2(v)
	if err != nil {
		return ReviewArtifactLocatorV2{}, err
	}
	if provided != "" && provided != digest {
		return ReviewArtifactLocatorV2{}, groundingDigestConflictV2("artifact locator supplied digest drifted")
	}
	v.LocatorDigest = digest
	return v, v.Validate()
}

type ReviewArtifactCurrentProjectionRefV2 struct {
	ID            string        `json:"id"`
	Revision      core.Revision `json:"revision"`
	SubjectDigest core.Digest   `json:"subject_digest"`
	Digest        core.Digest   `json:"digest"`
}
type ReviewEnvironmentCurrentProjectionRefV2 struct {
	ID            string        `json:"id"`
	Revision      core.Revision `json:"revision"`
	SubjectDigest core.Digest   `json:"subject_digest"`
	Digest        core.Digest   `json:"digest"`
}
type ReviewValidationScopeCurrentProjectionRefV2 struct {
	ID            string        `json:"id"`
	Revision      core.Revision `json:"revision"`
	SubjectDigest core.Digest   `json:"subject_digest"`
	Digest        core.Digest   `json:"digest"`
}

func (r ReviewArtifactCurrentProjectionRefV2) Validate() error {
	return validateGroundingProjectionRefV2(r.ID, r.Revision, r.SubjectDigest, r.Digest)
}
func (r ReviewEnvironmentCurrentProjectionRefV2) Validate() error {
	return validateGroundingProjectionRefV2(r.ID, r.Revision, r.SubjectDigest, r.Digest)
}
func (r ReviewValidationScopeCurrentProjectionRefV2) Validate() error {
	return validateGroundingProjectionRefV2(r.ID, r.Revision, r.SubjectDigest, r.Digest)
}

type ReviewArtifactCurrentProjectionIdentityInputV2 struct {
	Expected ReviewArtifactExactSourceRefV2 `json:"expected"`
}
type ReviewEnvironmentCurrentProjectionIdentityInputV2 struct {
	Expected ReviewEnvironmentExactRefV2 `json:"expected"`
}
type ReviewValidationScopeCurrentProjectionIdentityInputV2 struct {
	Source ReviewValidationScopeSourceIdentityV2 `json:"source"`
}

func (i ReviewArtifactCurrentProjectionIdentityInputV2) Validate() error {
	return i.Expected.Validate()
}
func (i ReviewEnvironmentCurrentProjectionIdentityInputV2) Validate() error {
	return i.Expected.Validate()
}
func (i ReviewValidationScopeCurrentProjectionIdentityInputV2) Validate() error {
	return i.Source.Validate()
}

func DeriveReviewArtifactCurrentProjectionIDV2(i ReviewArtifactCurrentProjectionIdentityInputV2) (string, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	d, err := core.CanonicalJSONDigest(reviewArtifactCurrentDomainV2, ReviewArtifactCurrentContractV2, "ReviewArtifactCurrentProjectionIdentityInputV2", i)
	return string(d), err
}
func DeriveReviewEnvironmentCurrentProjectionIDV2(i ReviewEnvironmentCurrentProjectionIdentityInputV2) (string, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	d, err := core.CanonicalJSONDigest(reviewEnvironmentCurrentDomainV2, ReviewEnvironmentCurrentContractV2, "ReviewEnvironmentCurrentProjectionIdentityInputV2", i)
	return string(d), err
}
func DeriveReviewValidationScopeCurrentProjectionIDV2(i ReviewValidationScopeCurrentProjectionIdentityInputV2) (string, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	d, err := core.CanonicalJSONDigest(reviewValidationScopeCurrentDomainV2, ReviewValidationScopeCurrentContractV2, "ReviewValidationScopeCurrentProjectionIdentityInputV2", i)
	return string(d), err
}

type ReviewArtifactCurrentSubjectV2 struct {
	Expected ReviewArtifactExactSourceRefV2 `json:"expected"`
	Anchors  []ReviewArtifactLocatorV2      `json:"anchors"`
}

func (s ReviewArtifactCurrentSubjectV2) Validate() error {
	if s.Expected.Validate() != nil || len(s.Anchors) == 0 || len(s.Anchors) > MaxReviewEvidenceV2 || !sort.SliceIsSorted(s.Anchors, func(i, j int) bool { return locatorKeyV2(s.Anchors[i]) < locatorKeyV2(s.Anchors[j]) }) {
		return groundingInvalidCanonicalV2("artifact current subject is incomplete or non-canonical")
	}
	for i, anchor := range s.Anchors {
		if err := anchor.Validate(); err != nil {
			return err
		}
		if i > 0 && locatorKeyV2(s.Anchors[i-1]) == locatorKeyV2(anchor) {
			return groundingDuplicateV2("artifact locator is duplicated")
		}
	}
	return nil
}

type ReviewEnvironmentCurrentSubjectV2 struct {
	Expected ReviewEnvironmentExactRefV2 `json:"expected"`
}

func (s ReviewEnvironmentCurrentSubjectV2) Validate() error { return s.Expected.Validate() }

type ReviewValidationScopeCurrentSubjectV2 struct {
	Expected                        ReviewValidationScopeExactRefV2 `json:"expected"`
	CoveredArtifactLocatorSetDigest core.Digest                     `json:"covered_artifact_locator_set_digest"`
	EvidenceSetDigest               core.Digest                     `json:"evidence_set_digest"`
}

func (s ReviewValidationScopeCurrentSubjectV2) Validate() error {
	if s.Expected.Validate() != nil || s.CoveredArtifactLocatorSetDigest.Validate() != nil || s.EvidenceSetDigest.Validate() != nil {
		return groundingInvalidV2("validation scope current subject is incomplete")
	}
	return nil
}

type ReviewArtifactCurrentProjectionV2 struct {
	ContractVersion  string                               `json:"contract_version"`
	Ref              ReviewArtifactCurrentProjectionRefV2 `json:"ref"`
	Subject          ReviewArtifactCurrentSubjectV2       `json:"subject"`
	Source           ReviewArtifactExactSourceRefV2       `json:"source"`
	OwnerBinding     ReviewBindingCurrentProjectionV1     `json:"owner_binding"`
	LocatorSetDigest core.Digest                          `json:"locator_set_digest"`
	State            ReviewGroundingCurrentStateV2        `json:"state"`
	Current          bool                                 `json:"current"`
	CheckedUnixNano  int64                                `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                          `json:"projection_digest"`
}

func (p ReviewArtifactCurrentProjectionV2) Clone() ReviewArtifactCurrentProjectionV2 {
	p.Subject.Anchors = cloneLocatorsV2(p.Subject.Anchors)
	p.OwnerBinding = p.OwnerBinding.CloneV1()
	return p
}
func (p ReviewArtifactCurrentProjectionV2) Validate() error {
	if p.ContractVersion != ReviewArtifactCurrentContractV2 || p.Ref.Validate() != nil || p.Subject.Validate() != nil || p.Source != p.Subject.Expected || p.OwnerBinding.Validate() != nil || p.OwnerBinding.Source != p.Source.Owner.Binding || p.LocatorSetDigest.Validate() != nil || !validGroundingStateV2(p.State, p.Current) || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ExpiresUnixNano > p.OwnerBinding.ExpiresUnixNano {
		return groundingInvalidV2("artifact current projection is incomplete")
	}
	id, _ := DeriveReviewArtifactCurrentProjectionIDV2(ReviewArtifactCurrentProjectionIdentityInputV2{Expected: p.Source})
	subjectDigest, err := core.CanonicalJSONDigest(reviewArtifactCurrentDomainV2, ReviewArtifactCurrentContractV2, "ReviewArtifactCurrentSubjectV2", p.Subject)
	if err != nil {
		return err
	}
	locatorDigest, err := digestLocatorSetV2(p.Subject.Anchors)
	if err != nil {
		return err
	}
	if p.Ref.ID != id || p.Ref.SubjectDigest != subjectDigest || p.LocatorSetDigest != locatorDigest {
		return groundingConflictV2("artifact current coordinates drifted")
	}
	digest, err := DigestReviewArtifactCurrentProjectionV2(p)
	if err != nil {
		return err
	}
	if digest != p.Ref.Digest || digest != p.ProjectionDigest {
		return groundingDigestConflictV2("artifact current digest drifted")
	}
	return nil
}
func (p ReviewArtifactCurrentProjectionV2) ValidateCurrent(ref ReviewArtifactCurrentProjectionRefV2, subject ReviewArtifactCurrentSubjectV2, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Ref != ref || !reflect.DeepEqual(p.Subject, subject) {
		return groundingConflictV2("artifact current exact coordinates drifted")
	}
	return validateGroundingCurrentV2(p.State, p.Current, p.CheckedUnixNano, p.ExpiresUnixNano, now)
}
func DigestReviewArtifactCurrentProjectionV2(p ReviewArtifactCurrentProjectionV2) (core.Digest, error) {
	copy := p.Clone()
	copy.Ref.Digest, copy.ProjectionDigest = "", ""
	return core.CanonicalJSONDigest(reviewArtifactCurrentDomainV2, ReviewArtifactCurrentContractV2, "ReviewArtifactCurrentProjectionV2", copy)
}
func SealReviewArtifactCurrentProjectionV2(p ReviewArtifactCurrentProjectionV2) (ReviewArtifactCurrentProjectionV2, error) {
	p.ContractVersion = ReviewArtifactCurrentContractV2
	p.Subject.Anchors = cloneLocatorsV2(p.Subject.Anchors)
	sort.Slice(p.Subject.Anchors, func(i, j int) bool { return locatorKeyV2(p.Subject.Anchors[i]) < locatorKeyV2(p.Subject.Anchors[j]) })
	id, err := DeriveReviewArtifactCurrentProjectionIDV2(ReviewArtifactCurrentProjectionIdentityInputV2{Expected: p.Source})
	if err != nil {
		return ReviewArtifactCurrentProjectionV2{}, err
	}
	if p.Ref.ID != "" && p.Ref.ID != id {
		return ReviewArtifactCurrentProjectionV2{}, groundingDigestConflictV2("artifact current supplied ID drifted")
	}
	p.Ref.ID = id
	subjectDigest, err := core.CanonicalJSONDigest(reviewArtifactCurrentDomainV2, ReviewArtifactCurrentContractV2, "ReviewArtifactCurrentSubjectV2", p.Subject)
	if err != nil {
		return ReviewArtifactCurrentProjectionV2{}, err
	}
	p.Ref.SubjectDigest = subjectDigest
	p.LocatorSetDigest, err = digestLocatorSetV2(p.Subject.Anchors)
	if err != nil {
		return ReviewArtifactCurrentProjectionV2{}, err
	}
	p.Ref.Digest, p.ProjectionDigest = "", ""
	digest, err := DigestReviewArtifactCurrentProjectionV2(p)
	if err != nil {
		return ReviewArtifactCurrentProjectionV2{}, err
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p, p.Validate()
}

type ReviewEnvironmentCurrentProjectionV2 struct {
	ContractVersion  string                                  `json:"contract_version"`
	Ref              ReviewEnvironmentCurrentProjectionRefV2 `json:"ref"`
	Subject          ReviewEnvironmentCurrentSubjectV2       `json:"subject"`
	Source           ReviewEnvironmentExactRefV2             `json:"source"`
	OwnerBinding     ReviewBindingCurrentProjectionV1        `json:"owner_binding"`
	OwnerLeaseDigest core.Digest                             `json:"owner_lease_digest"`
	State            ReviewGroundingCurrentStateV2           `json:"state"`
	Current          bool                                    `json:"current"`
	CheckedUnixNano  int64                                   `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                   `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                             `json:"projection_digest"`
}

func (p ReviewEnvironmentCurrentProjectionV2) Clone() ReviewEnvironmentCurrentProjectionV2 {
	p.OwnerBinding = p.OwnerBinding.CloneV1()
	return p
}
func (p ReviewEnvironmentCurrentProjectionV2) Validate() error {
	if p.ContractVersion != ReviewEnvironmentCurrentContractV2 || p.Ref.Validate() != nil || p.Subject.Validate() != nil || p.Source != p.Subject.Expected || p.OwnerBinding.Validate() != nil || p.OwnerBinding.Source != p.Source.Owner.Binding || p.OwnerLeaseDigest.Validate() != nil || !validGroundingStateV2(p.State, p.Current) || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ExpiresUnixNano > p.OwnerBinding.ExpiresUnixNano {
		return groundingInvalidV2("environment current projection is incomplete")
	}
	id, _ := DeriveReviewEnvironmentCurrentProjectionIDV2(ReviewEnvironmentCurrentProjectionIdentityInputV2{Expected: p.Source})
	subjectDigest, err := core.CanonicalJSONDigest(reviewEnvironmentCurrentDomainV2, ReviewEnvironmentCurrentContractV2, "ReviewEnvironmentCurrentSubjectV2", p.Subject)
	if err != nil {
		return err
	}
	if p.Ref.ID != id || p.Ref.SubjectDigest != subjectDigest {
		return groundingConflictV2("environment current coordinates drifted")
	}
	digest, err := DigestReviewEnvironmentCurrentProjectionV2(p)
	if err != nil {
		return err
	}
	if digest != p.Ref.Digest || digest != p.ProjectionDigest {
		return groundingDigestConflictV2("environment current digest drifted")
	}
	return nil
}
func (p ReviewEnvironmentCurrentProjectionV2) ValidateCurrent(ref ReviewEnvironmentCurrentProjectionRefV2, subject ReviewEnvironmentCurrentSubjectV2, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Ref != ref || p.Subject != subject {
		return groundingConflictV2("environment current exact coordinates drifted")
	}
	return validateGroundingCurrentV2(p.State, p.Current, p.CheckedUnixNano, p.ExpiresUnixNano, now)
}
func DigestReviewEnvironmentCurrentProjectionV2(p ReviewEnvironmentCurrentProjectionV2) (core.Digest, error) {
	copy := p.Clone()
	copy.Ref.Digest, copy.ProjectionDigest = "", ""
	return core.CanonicalJSONDigest(reviewEnvironmentCurrentDomainV2, ReviewEnvironmentCurrentContractV2, "ReviewEnvironmentCurrentProjectionV2", copy)
}
func SealReviewEnvironmentCurrentProjectionV2(p ReviewEnvironmentCurrentProjectionV2) (ReviewEnvironmentCurrentProjectionV2, error) {
	p.ContractVersion = ReviewEnvironmentCurrentContractV2
	id, err := DeriveReviewEnvironmentCurrentProjectionIDV2(ReviewEnvironmentCurrentProjectionIdentityInputV2{Expected: p.Source})
	if err != nil {
		return ReviewEnvironmentCurrentProjectionV2{}, err
	}
	if p.Ref.ID != "" && p.Ref.ID != id {
		return ReviewEnvironmentCurrentProjectionV2{}, groundingDigestConflictV2("environment current supplied ID drifted")
	}
	p.Ref.ID = id
	p.Ref.SubjectDigest, err = core.CanonicalJSONDigest(reviewEnvironmentCurrentDomainV2, ReviewEnvironmentCurrentContractV2, "ReviewEnvironmentCurrentSubjectV2", p.Subject)
	if err != nil {
		return ReviewEnvironmentCurrentProjectionV2{}, err
	}
	p.Ref.Digest, p.ProjectionDigest = "", ""
	digest, err := DigestReviewEnvironmentCurrentProjectionV2(p)
	if err != nil {
		return ReviewEnvironmentCurrentProjectionV2{}, err
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p, p.Validate()
}

type ReviewValidationScopeCurrentProjectionV2 struct {
	ContractVersion                 string                                      `json:"contract_version"`
	Ref                             ReviewValidationScopeCurrentProjectionRefV2 `json:"ref"`
	Subject                         ReviewValidationScopeCurrentSubjectV2       `json:"subject"`
	Source                          ReviewValidationScopeExactRefV2             `json:"source"`
	OwnerBinding                    ReviewBindingCurrentProjectionV1            `json:"owner_binding"`
	ValidationMethod                SchemaRefV2                                 `json:"validation_method"`
	CoveredArtifactLocatorSetDigest core.Digest                                 `json:"covered_artifact_locator_set_digest"`
	EvidenceSetDigest               core.Digest                                 `json:"evidence_set_digest"`
	State                           ReviewGroundingCurrentStateV2               `json:"state"`
	Current                         bool                                        `json:"current"`
	CheckedUnixNano                 int64                                       `json:"checked_unix_nano"`
	ExpiresUnixNano                 int64                                       `json:"expires_unix_nano"`
	ProjectionDigest                core.Digest                                 `json:"projection_digest"`
}

func (p ReviewValidationScopeCurrentProjectionV2) Clone() ReviewValidationScopeCurrentProjectionV2 {
	p.OwnerBinding = p.OwnerBinding.CloneV1()
	return p
}
func (p ReviewValidationScopeCurrentProjectionV2) Validate() error {
	if p.ContractVersion != ReviewValidationScopeCurrentContractV2 || p.Ref.Validate() != nil || p.Subject.Validate() != nil || !reflect.DeepEqual(p.Source, p.Subject.Expected) || p.OwnerBinding.Validate() != nil || p.OwnerBinding.Source != p.Source.Owner.Binding || p.ValidationMethod.Validate() != nil || p.CoveredArtifactLocatorSetDigest != p.Subject.CoveredArtifactLocatorSetDigest || p.EvidenceSetDigest != p.Subject.EvidenceSetDigest || !validGroundingStateV2(p.State, p.Current) || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ExpiresUnixNano > p.OwnerBinding.ExpiresUnixNano {
		return groundingInvalidV2("validation scope current projection is incomplete")
	}
	id, _ := DeriveReviewValidationScopeCurrentProjectionIDV2(ReviewValidationScopeCurrentProjectionIdentityInputV2{Source: p.Source.Source})
	subjectDigest, err := core.CanonicalJSONDigest(reviewValidationScopeCurrentDomainV2, ReviewValidationScopeCurrentContractV2, "ReviewValidationScopeCurrentSubjectV2", p.Subject)
	if err != nil {
		return err
	}
	if p.Ref.ID != id || p.Ref.SubjectDigest != subjectDigest {
		return groundingConflictV2("validation scope current coordinates drifted")
	}
	digest, err := DigestReviewValidationScopeCurrentProjectionV2(p)
	if err != nil {
		return err
	}
	if digest != p.Ref.Digest || digest != p.ProjectionDigest {
		return groundingDigestConflictV2("validation scope current digest drifted")
	}
	return nil
}
func (p ReviewValidationScopeCurrentProjectionV2) ValidateCurrent(ref ReviewValidationScopeCurrentProjectionRefV2, subject ReviewValidationScopeCurrentSubjectV2, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Ref != ref || !reflect.DeepEqual(p.Subject, subject) {
		return groundingConflictV2("validation scope current exact coordinates drifted")
	}
	return validateGroundingCurrentV2(p.State, p.Current, p.CheckedUnixNano, p.ExpiresUnixNano, now)
}
func DigestReviewValidationScopeCurrentProjectionV2(p ReviewValidationScopeCurrentProjectionV2) (core.Digest, error) {
	copy := p.Clone()
	copy.Ref.Digest, copy.ProjectionDigest = "", ""
	return core.CanonicalJSONDigest(reviewValidationScopeCurrentDomainV2, ReviewValidationScopeCurrentContractV2, "ReviewValidationScopeCurrentProjectionV2", copy)
}
func SealReviewValidationScopeCurrentProjectionV2(p ReviewValidationScopeCurrentProjectionV2) (ReviewValidationScopeCurrentProjectionV2, error) {
	p.ContractVersion = ReviewValidationScopeCurrentContractV2
	id, err := DeriveReviewValidationScopeCurrentProjectionIDV2(ReviewValidationScopeCurrentProjectionIdentityInputV2{Source: p.Source.Source})
	if err != nil {
		return ReviewValidationScopeCurrentProjectionV2{}, err
	}
	if p.Ref.ID != "" && p.Ref.ID != id {
		return ReviewValidationScopeCurrentProjectionV2{}, groundingDigestConflictV2("validation scope current supplied ID drifted")
	}
	p.Ref.ID = id
	p.Ref.SubjectDigest, err = core.CanonicalJSONDigest(reviewValidationScopeCurrentDomainV2, ReviewValidationScopeCurrentContractV2, "ReviewValidationScopeCurrentSubjectV2", p.Subject)
	if err != nil {
		return ReviewValidationScopeCurrentProjectionV2{}, err
	}
	p.Ref.Digest, p.ProjectionDigest = "", ""
	digest, err := DigestReviewValidationScopeCurrentProjectionV2(p)
	if err != nil {
		return ReviewValidationScopeCurrentProjectionV2{}, err
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p, p.Validate()
}

type ReviewArtifactCurrentReaderV2 interface {
	ResolveCurrentReviewArtifactV2(context.Context, ReviewArtifactCurrentSubjectV2) (ReviewArtifactCurrentProjectionRefV2, error)
	InspectCurrentReviewArtifactV2(context.Context, ReviewArtifactCurrentSubjectV2, ReviewArtifactCurrentProjectionRefV2) (ReviewArtifactCurrentProjectionV2, error)
	InspectHistoricalReviewArtifactV2(context.Context, ReviewArtifactCurrentProjectionRefV2) (ReviewArtifactCurrentProjectionV2, error)
}
type ReviewEnvironmentCurrentReaderV2 interface {
	ResolveCurrentReviewEnvironmentV2(context.Context, ReviewEnvironmentCurrentSubjectV2) (ReviewEnvironmentCurrentProjectionRefV2, error)
	InspectCurrentReviewEnvironmentV2(context.Context, ReviewEnvironmentCurrentSubjectV2, ReviewEnvironmentCurrentProjectionRefV2) (ReviewEnvironmentCurrentProjectionV2, error)
	InspectHistoricalReviewEnvironmentV2(context.Context, ReviewEnvironmentCurrentProjectionRefV2) (ReviewEnvironmentCurrentProjectionV2, error)
}
type ReviewValidationScopeCurrentReaderV2 interface {
	ResolveCurrentReviewValidationScopeV2(context.Context, ReviewValidationScopeCurrentSubjectV2) (ReviewValidationScopeCurrentProjectionRefV2, error)
	InspectCurrentReviewValidationScopeV2(context.Context, ReviewValidationScopeCurrentSubjectV2, ReviewValidationScopeCurrentProjectionRefV2) (ReviewValidationScopeCurrentProjectionV2, error)
	InspectHistoricalReviewValidationScopeV2(context.Context, ReviewValidationScopeCurrentProjectionRefV2) (ReviewValidationScopeCurrentProjectionV2, error)
}

type ReviewValidationScopeOwnerAssociationRefV2 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ReviewValidationScopeOwnerAssociationRefV2) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision == 0 || r.Digest.Validate() != nil {
		return groundingInvalidV2("validation scope Owner association ref is incomplete")
	}
	return nil
}

type ReviewValidationScopeOwnerAssociationSubjectV2 struct {
	Source ReviewValidationScopeSourceIdentityV2 `json:"source"`
}

func (s ReviewValidationScopeOwnerAssociationSubjectV2) Validate() error { return s.Source.Validate() }

type ReviewValidationScopeOwnerAssociationCurrentProjectionV2 struct {
	ContractVersion  string                                         `json:"contract_version"`
	Ref              ReviewValidationScopeOwnerAssociationRefV2     `json:"ref"`
	Subject          ReviewValidationScopeOwnerAssociationSubjectV2 `json:"subject"`
	Owner            ReviewGroundingOwnerRefV2                      `json:"owner"`
	Current          bool                                           `json:"current"`
	CheckedUnixNano  int64                                          `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                          `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                                    `json:"projection_digest"`
}

func (p ReviewValidationScopeOwnerAssociationCurrentProjectionV2) Clone() ReviewValidationScopeOwnerAssociationCurrentProjectionV2 {
	return p
}
func (p ReviewValidationScopeOwnerAssociationCurrentProjectionV2) Validate() error {
	if p.ContractVersion != ReviewValidationScopeCurrentContractV2 || p.Ref.Validate() != nil || p.Subject.Validate() != nil || p.Owner.Validate() != nil || !p.Current || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return groundingInvalidV2("validation scope Owner association is incomplete")
	}
	id, err := core.CanonicalJSONDigest(reviewValidationScopeCurrentDomainV2, ReviewValidationScopeCurrentContractV2, "ReviewValidationScopeOwnerAssociationSubjectV2", p.Subject)
	if err != nil {
		return err
	}
	if p.Ref.ID != string(id) {
		return groundingConflictV2("validation scope Owner association ID drifted")
	}
	copy := p
	copy.Ref.Digest, copy.ProjectionDigest = "", ""
	digest, err := core.CanonicalJSONDigest(reviewValidationScopeCurrentDomainV2, ReviewValidationScopeCurrentContractV2, "ReviewValidationScopeOwnerAssociationCurrentProjectionV2", copy)
	if err != nil {
		return err
	}
	if digest != p.Ref.Digest || digest != p.ProjectionDigest {
		return groundingDigestConflictV2("validation scope Owner association digest drifted")
	}
	return nil
}
func (p ReviewValidationScopeOwnerAssociationCurrentProjectionV2) ValidateCurrent(ref ReviewValidationScopeOwnerAssociationRefV2, subject ReviewValidationScopeOwnerAssociationSubjectV2, owner ReviewGroundingOwnerRefV2, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Ref != ref || p.Subject != subject || p.Owner != owner {
		return groundingConflictV2("validation scope Owner association coordinates drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "validation scope Owner association clock regressed")
	}
	if !p.Current || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "validation scope Owner association expired")
	}
	return nil
}

type ReviewValidationScopeOwnerAssociationCurrentReaderV2 interface {
	ResolveCurrentReviewValidationScopeOwnerAssociationV2(context.Context, ReviewValidationScopeOwnerAssociationSubjectV2) (ReviewValidationScopeOwnerAssociationRefV2, error)
	InspectCurrentReviewValidationScopeOwnerAssociationV2(context.Context, ReviewValidationScopeOwnerAssociationSubjectV2, ReviewValidationScopeOwnerAssociationRefV2) (ReviewValidationScopeOwnerAssociationCurrentProjectionV2, error)
	InspectHistoricalReviewValidationScopeOwnerAssociationV2(context.Context, ReviewValidationScopeOwnerAssociationRefV2) (ReviewValidationScopeOwnerAssociationCurrentProjectionV2, error)
}

type ReviewArtifactCurrentProjectionPublishRefV2 struct {
	ID     string      `json:"id"`
	Digest core.Digest `json:"digest"`
}
type CreateReviewArtifactCurrentProjectionRequestV2 struct {
	PublishRef ReviewArtifactCurrentProjectionPublishRefV2 `json:"publish_ref"`
	Value      ReviewArtifactCurrentProjectionV2           `json:"value"`
}
type CompareAndSwapReviewArtifactCurrentProjectionRequestV2 struct {
	PublishRef ReviewArtifactCurrentProjectionPublishRefV2 `json:"publish_ref"`
	Expected   ReviewArtifactCurrentProjectionRefV2        `json:"expected"`
	Value      ReviewArtifactCurrentProjectionV2           `json:"value"`
}
type ReviewArtifactCurrentProjectionPublishReceiptV2 struct {
	ContractVersion string                                      `json:"contract_version"`
	PublishRef      ReviewArtifactCurrentProjectionPublishRefV2 `json:"publish_ref"`
	Projection      ReviewArtifactCurrentProjectionRefV2        `json:"projection"`
	CurrentIndex    ReviewArtifactCurrentProjectionRefV2        `json:"current_index"`
	HighestRevision core.Revision                               `json:"highest_revision"`
	Digest          core.Digest                                 `json:"digest"`
}

type ReviewEnvironmentCurrentProjectionPublishRefV2 struct {
	ID     string      `json:"id"`
	Digest core.Digest `json:"digest"`
}
type CreateReviewEnvironmentCurrentProjectionRequestV2 struct {
	PublishRef ReviewEnvironmentCurrentProjectionPublishRefV2 `json:"publish_ref"`
	Value      ReviewEnvironmentCurrentProjectionV2           `json:"value"`
}
type CompareAndSwapReviewEnvironmentCurrentProjectionRequestV2 struct {
	PublishRef ReviewEnvironmentCurrentProjectionPublishRefV2 `json:"publish_ref"`
	Expected   ReviewEnvironmentCurrentProjectionRefV2        `json:"expected"`
	Value      ReviewEnvironmentCurrentProjectionV2           `json:"value"`
}
type ReviewEnvironmentCurrentProjectionPublishReceiptV2 struct {
	ContractVersion string                                         `json:"contract_version"`
	PublishRef      ReviewEnvironmentCurrentProjectionPublishRefV2 `json:"publish_ref"`
	Projection      ReviewEnvironmentCurrentProjectionRefV2        `json:"projection"`
	CurrentIndex    ReviewEnvironmentCurrentProjectionRefV2        `json:"current_index"`
	HighestRevision core.Revision                                  `json:"highest_revision"`
	Digest          core.Digest                                    `json:"digest"`
}

type ReviewValidationScopeCurrentProjectionPublishRefV2 struct {
	ID     string      `json:"id"`
	Digest core.Digest `json:"digest"`
}
type CreateReviewValidationScopeCurrentProjectionRequestV2 struct {
	PublishRef ReviewValidationScopeCurrentProjectionPublishRefV2 `json:"publish_ref"`
	Value      ReviewValidationScopeCurrentProjectionV2           `json:"value"`
}
type CompareAndSwapReviewValidationScopeCurrentProjectionRequestV2 struct {
	PublishRef ReviewValidationScopeCurrentProjectionPublishRefV2 `json:"publish_ref"`
	Expected   ReviewValidationScopeCurrentProjectionRefV2        `json:"expected"`
	Value      ReviewValidationScopeCurrentProjectionV2           `json:"value"`
}
type ReviewValidationScopeCurrentProjectionPublishReceiptV2 struct {
	ContractVersion string                                             `json:"contract_version"`
	PublishRef      ReviewValidationScopeCurrentProjectionPublishRefV2 `json:"publish_ref"`
	Projection      ReviewValidationScopeCurrentProjectionRefV2        `json:"projection"`
	CurrentIndex    ReviewValidationScopeCurrentProjectionRefV2        `json:"current_index"`
	HighestRevision core.Revision                                      `json:"highest_revision"`
	Digest          core.Digest                                        `json:"digest"`
}

func (r ReviewArtifactCurrentProjectionPublishRefV2) Validate() error {
	return validateGroundingPublishRefV2(r.ID, r.Digest)
}
func (r ReviewEnvironmentCurrentProjectionPublishRefV2) Validate() error {
	return validateGroundingPublishRefV2(r.ID, r.Digest)
}
func (r ReviewValidationScopeCurrentProjectionPublishRefV2) Validate() error {
	return validateGroundingPublishRefV2(r.ID, r.Digest)
}
func (r CreateReviewArtifactCurrentProjectionRequestV2) Validate() error {
	if r.PublishRef.Validate() != nil || r.Value.Validate() != nil {
		return groundingInvalidV2("artifact current create request is incomplete")
	}
	if r.Value.Ref.Revision != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "artifact current first revision must be one")
	}
	expected, err := DeriveCreateReviewArtifactCurrentProjectionPublishRefV2(r.Value)
	if err != nil || expected != r.PublishRef {
		return groundingConflictV2("artifact current create PublishRef drifted")
	}
	return nil
}
func (r CompareAndSwapReviewArtifactCurrentProjectionRequestV2) Validate() error {
	if r.PublishRef.Validate() != nil || r.Expected.Validate() != nil || r.Value.Validate() != nil {
		return groundingInvalidV2("artifact current CAS request is incomplete")
	}
	if r.Value.Ref.ID != r.Expected.ID || r.Value.Ref.Revision != r.Expected.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "artifact current CAS revision drifted")
	}
	expected, err := DeriveCompareAndSwapReviewArtifactCurrentProjectionPublishRefV2(r.Expected, r.Value)
	if err != nil || expected != r.PublishRef {
		return groundingConflictV2("artifact current CAS PublishRef drifted")
	}
	return nil
}
func (r CreateReviewEnvironmentCurrentProjectionRequestV2) Validate() error {
	if r.PublishRef.Validate() != nil || r.Value.Validate() != nil {
		return groundingInvalidV2("environment current create request is incomplete")
	}
	if r.Value.Ref.Revision != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "environment current first revision must be one")
	}
	expected, err := DeriveCreateReviewEnvironmentCurrentProjectionPublishRefV2(r.Value)
	if err != nil || expected != r.PublishRef {
		return groundingConflictV2("environment current create PublishRef drifted")
	}
	return nil
}
func (r CompareAndSwapReviewEnvironmentCurrentProjectionRequestV2) Validate() error {
	if r.PublishRef.Validate() != nil || r.Expected.Validate() != nil || r.Value.Validate() != nil {
		return groundingInvalidV2("environment current CAS request is incomplete")
	}
	if r.Value.Ref.ID != r.Expected.ID || r.Value.Ref.Revision != r.Expected.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "environment current CAS revision drifted")
	}
	expected, err := DeriveCompareAndSwapReviewEnvironmentCurrentProjectionPublishRefV2(r.Expected, r.Value)
	if err != nil || expected != r.PublishRef {
		return groundingConflictV2("environment current CAS PublishRef drifted")
	}
	return nil
}
func (r CreateReviewValidationScopeCurrentProjectionRequestV2) Validate() error {
	if r.PublishRef.Validate() != nil || r.Value.Validate() != nil {
		return groundingInvalidV2("validation scope current create request is incomplete")
	}
	if r.Value.Ref.Revision != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "validation scope current first revision must be one")
	}
	expected, err := DeriveCreateReviewValidationScopeCurrentProjectionPublishRefV2(r.Value)
	if err != nil || expected != r.PublishRef {
		return groundingConflictV2("validation scope current create PublishRef drifted")
	}
	return nil
}
func (r CompareAndSwapReviewValidationScopeCurrentProjectionRequestV2) Validate() error {
	if r.PublishRef.Validate() != nil || r.Expected.Validate() != nil || r.Value.Validate() != nil {
		return groundingInvalidV2("validation scope current CAS request is incomplete")
	}
	if r.Value.Ref.ID != r.Expected.ID || r.Value.Ref.Revision != r.Expected.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "validation scope current CAS revision drifted")
	}
	expected, err := DeriveCompareAndSwapReviewValidationScopeCurrentProjectionPublishRefV2(r.Expected, r.Value)
	if err != nil || expected != r.PublishRef {
		return groundingConflictV2("validation scope current CAS PublishRef drifted")
	}
	return nil
}

func (r ReviewArtifactCurrentProjectionPublishReceiptV2) Validate() error {
	if r.ContractVersion != ReviewArtifactCurrentContractV2 || r.PublishRef.Validate() != nil || r.Projection.Validate() != nil || r.CurrentIndex.Validate() != nil || r.Projection != r.CurrentIndex || r.HighestRevision != r.Projection.Revision || r.Digest.Validate() != nil {
		return groundingInvalidV2("artifact current Publish receipt is incomplete")
	}
	d, err := r.DigestV2()
	if err != nil || d != r.Digest {
		return groundingDigestConflictV2("artifact current Publish receipt digest drifted")
	}
	return nil
}
func (r ReviewArtifactCurrentProjectionPublishReceiptV2) DigestV2() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest(reviewArtifactCurrentDomainV2, ReviewArtifactCurrentContractV2, "ReviewArtifactCurrentProjectionPublishReceiptV2", copy)
}
func SealReviewArtifactCurrentProjectionPublishReceiptV2(r ReviewArtifactCurrentProjectionPublishReceiptV2) (ReviewArtifactCurrentProjectionPublishReceiptV2, error) {
	r.ContractVersion, r.Digest = ReviewArtifactCurrentContractV2, ""
	d, err := r.DigestV2()
	if err != nil {
		return ReviewArtifactCurrentProjectionPublishReceiptV2{}, err
	}
	r.Digest = d
	return r, r.Validate()
}
func (r ReviewEnvironmentCurrentProjectionPublishReceiptV2) Validate() error {
	if r.ContractVersion != ReviewEnvironmentCurrentContractV2 || r.PublishRef.Validate() != nil || r.Projection.Validate() != nil || r.CurrentIndex.Validate() != nil || r.Projection != r.CurrentIndex || r.HighestRevision != r.Projection.Revision || r.Digest.Validate() != nil {
		return groundingInvalidV2("environment current Publish receipt is incomplete")
	}
	d, err := r.DigestV2()
	if err != nil || d != r.Digest {
		return groundingDigestConflictV2("environment current Publish receipt digest drifted")
	}
	return nil
}
func (r ReviewEnvironmentCurrentProjectionPublishReceiptV2) DigestV2() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest(reviewEnvironmentCurrentDomainV2, ReviewEnvironmentCurrentContractV2, "ReviewEnvironmentCurrentProjectionPublishReceiptV2", copy)
}
func SealReviewEnvironmentCurrentProjectionPublishReceiptV2(r ReviewEnvironmentCurrentProjectionPublishReceiptV2) (ReviewEnvironmentCurrentProjectionPublishReceiptV2, error) {
	r.ContractVersion, r.Digest = ReviewEnvironmentCurrentContractV2, ""
	d, err := r.DigestV2()
	if err != nil {
		return ReviewEnvironmentCurrentProjectionPublishReceiptV2{}, err
	}
	r.Digest = d
	return r, r.Validate()
}
func (r ReviewValidationScopeCurrentProjectionPublishReceiptV2) Validate() error {
	if r.ContractVersion != ReviewValidationScopeCurrentContractV2 || r.PublishRef.Validate() != nil || r.Projection.Validate() != nil || r.CurrentIndex.Validate() != nil || r.Projection != r.CurrentIndex || r.HighestRevision != r.Projection.Revision || r.Digest.Validate() != nil {
		return groundingInvalidV2("validation scope current Publish receipt is incomplete")
	}
	d, err := r.DigestV2()
	if err != nil || d != r.Digest {
		return groundingDigestConflictV2("validation scope current Publish receipt digest drifted")
	}
	return nil
}
func (r ReviewValidationScopeCurrentProjectionPublishReceiptV2) DigestV2() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest(reviewValidationScopeCurrentDomainV2, ReviewValidationScopeCurrentContractV2, "ReviewValidationScopeCurrentProjectionPublishReceiptV2", copy)
}
func SealReviewValidationScopeCurrentProjectionPublishReceiptV2(r ReviewValidationScopeCurrentProjectionPublishReceiptV2) (ReviewValidationScopeCurrentProjectionPublishReceiptV2, error) {
	r.ContractVersion, r.Digest = ReviewValidationScopeCurrentContractV2, ""
	d, err := r.DigestV2()
	if err != nil {
		return ReviewValidationScopeCurrentProjectionPublishReceiptV2{}, err
	}
	r.Digest = d
	return r, r.Validate()
}

func DeriveCreateReviewArtifactCurrentProjectionPublishRefV2(v ReviewArtifactCurrentProjectionV2) (ReviewArtifactCurrentProjectionPublishRefV2, error) {
	return deriveArtifactPublishRefV2("CreateReviewArtifactCurrentProjectionRequestV2", struct {
		Value ReviewArtifactCurrentProjectionV2 `json:"value"`
	}{v})
}
func DeriveCompareAndSwapReviewArtifactCurrentProjectionPublishRefV2(e ReviewArtifactCurrentProjectionRefV2, v ReviewArtifactCurrentProjectionV2) (ReviewArtifactCurrentProjectionPublishRefV2, error) {
	return deriveArtifactPublishRefV2("CompareAndSwapReviewArtifactCurrentProjectionRequestV2", struct {
		Expected ReviewArtifactCurrentProjectionRefV2 `json:"expected"`
		Value    ReviewArtifactCurrentProjectionV2    `json:"value"`
	}{e, v})
}
func DeriveCreateReviewEnvironmentCurrentProjectionPublishRefV2(v ReviewEnvironmentCurrentProjectionV2) (ReviewEnvironmentCurrentProjectionPublishRefV2, error) {
	d, err := derivePublishDigestV2(reviewEnvironmentCurrentDomainV2, ReviewEnvironmentCurrentContractV2, "CreateReviewEnvironmentCurrentProjectionRequestV2", struct {
		Value ReviewEnvironmentCurrentProjectionV2 `json:"value"`
	}{v})
	return ReviewEnvironmentCurrentProjectionPublishRefV2{ID: string(d), Digest: d}, err
}
func DeriveCompareAndSwapReviewEnvironmentCurrentProjectionPublishRefV2(e ReviewEnvironmentCurrentProjectionRefV2, v ReviewEnvironmentCurrentProjectionV2) (ReviewEnvironmentCurrentProjectionPublishRefV2, error) {
	d, err := derivePublishDigestV2(reviewEnvironmentCurrentDomainV2, ReviewEnvironmentCurrentContractV2, "CompareAndSwapReviewEnvironmentCurrentProjectionRequestV2", struct {
		Expected ReviewEnvironmentCurrentProjectionRefV2 `json:"expected"`
		Value    ReviewEnvironmentCurrentProjectionV2    `json:"value"`
	}{e, v})
	return ReviewEnvironmentCurrentProjectionPublishRefV2{ID: string(d), Digest: d}, err
}
func DeriveCreateReviewValidationScopeCurrentProjectionPublishRefV2(v ReviewValidationScopeCurrentProjectionV2) (ReviewValidationScopeCurrentProjectionPublishRefV2, error) {
	d, err := derivePublishDigestV2(reviewValidationScopeCurrentDomainV2, ReviewValidationScopeCurrentContractV2, "CreateReviewValidationScopeCurrentProjectionRequestV2", struct {
		Value ReviewValidationScopeCurrentProjectionV2 `json:"value"`
	}{v})
	return ReviewValidationScopeCurrentProjectionPublishRefV2{ID: string(d), Digest: d}, err
}
func DeriveCompareAndSwapReviewValidationScopeCurrentProjectionPublishRefV2(e ReviewValidationScopeCurrentProjectionRefV2, v ReviewValidationScopeCurrentProjectionV2) (ReviewValidationScopeCurrentProjectionPublishRefV2, error) {
	d, err := derivePublishDigestV2(reviewValidationScopeCurrentDomainV2, ReviewValidationScopeCurrentContractV2, "CompareAndSwapReviewValidationScopeCurrentProjectionRequestV2", struct {
		Expected ReviewValidationScopeCurrentProjectionRefV2 `json:"expected"`
		Value    ReviewValidationScopeCurrentProjectionV2    `json:"value"`
	}{e, v})
	return ReviewValidationScopeCurrentProjectionPublishRefV2{ID: string(d), Digest: d}, err
}

// Publisher interfaces are Owner-only mutation surfaces. Review production
// composition accepts the Reader interfaces above and never these publishers.
type ReviewArtifactCurrentProjectionPublisherV2 interface {
	CreateReviewArtifactCurrentProjectionV2(context.Context, CreateReviewArtifactCurrentProjectionRequestV2) (ReviewArtifactCurrentProjectionPublishReceiptV2, error)
	CompareAndSwapReviewArtifactCurrentProjectionV2(context.Context, CompareAndSwapReviewArtifactCurrentProjectionRequestV2) (ReviewArtifactCurrentProjectionPublishReceiptV2, error)
	InspectReviewArtifactCurrentProjectionPublishV2(context.Context, ReviewArtifactCurrentProjectionPublishRefV2) (ReviewArtifactCurrentProjectionPublishReceiptV2, error)
}
type ReviewEnvironmentCurrentProjectionPublisherV2 interface {
	CreateReviewEnvironmentCurrentProjectionV2(context.Context, CreateReviewEnvironmentCurrentProjectionRequestV2) (ReviewEnvironmentCurrentProjectionPublishReceiptV2, error)
	CompareAndSwapReviewEnvironmentCurrentProjectionV2(context.Context, CompareAndSwapReviewEnvironmentCurrentProjectionRequestV2) (ReviewEnvironmentCurrentProjectionPublishReceiptV2, error)
	InspectReviewEnvironmentCurrentProjectionPublishV2(context.Context, ReviewEnvironmentCurrentProjectionPublishRefV2) (ReviewEnvironmentCurrentProjectionPublishReceiptV2, error)
}
type ReviewValidationScopeCurrentProjectionPublisherV2 interface {
	CreateReviewValidationScopeCurrentProjectionV2(context.Context, CreateReviewValidationScopeCurrentProjectionRequestV2) (ReviewValidationScopeCurrentProjectionPublishReceiptV2, error)
	CompareAndSwapReviewValidationScopeCurrentProjectionV2(context.Context, CompareAndSwapReviewValidationScopeCurrentProjectionRequestV2) (ReviewValidationScopeCurrentProjectionPublishReceiptV2, error)
	InspectReviewValidationScopeCurrentProjectionPublishV2(context.Context, ReviewValidationScopeCurrentProjectionPublishRefV2) (ReviewValidationScopeCurrentProjectionPublishReceiptV2, error)
}

func validateGroundingExactV2(kind NamespacedNameV2, owner ReviewGroundingOwnerRefV2, tenant core.TenantID, id string, revision core.Revision, digest, scope core.Digest) error {
	if ValidateNamespacedNameV2(kind) != nil || owner.Validate() != nil || strings.TrimSpace(string(tenant)) == "" || strings.TrimSpace(id) == "" || revision == 0 || digest.Validate() != nil || scope.Validate() != nil {
		return groundingInvalidV2("grounding exact ref is incomplete")
	}
	return nil
}
func validateGroundingProjectionRefV2(id string, revision core.Revision, subject, digest core.Digest) error {
	if strings.TrimSpace(id) == "" || revision == 0 || subject.Validate() != nil || digest.Validate() != nil {
		return groundingInvalidV2("grounding projection ref is incomplete")
	}
	return nil
}
func validateGroundingPublishRefV2(id string, digest core.Digest) error {
	if strings.TrimSpace(id) == "" || digest.Validate() != nil || id != string(digest) {
		return groundingInvalidV2("grounding projection PublishRef is incomplete")
	}
	return nil
}
func derivePublishDigestV2(domain, contract, discriminator string, value any) (core.Digest, error) {
	return core.CanonicalJSONDigest(domain, contract, discriminator, value)
}
func deriveArtifactPublishRefV2(discriminator string, value any) (ReviewArtifactCurrentProjectionPublishRefV2, error) {
	d, err := derivePublishDigestV2(reviewArtifactCurrentDomainV2, ReviewArtifactCurrentContractV2, discriminator, value)
	return ReviewArtifactCurrentProjectionPublishRefV2{ID: string(d), Digest: d}, err
}
func validGroundingStateV2(state ReviewGroundingCurrentStateV2, current bool) bool {
	switch state {
	case ReviewGroundingCurrentActiveV2:
		return current
	case ReviewGroundingCurrentRevokedV2, ReviewGroundingCurrentSupersededV2:
		return !current
	default:
		return false
	}
}
func validateGroundingCurrentV2(state ReviewGroundingCurrentStateV2, current bool, checked, expires int64, now time.Time) error {
	if now.IsZero() || now.UnixNano() < checked {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review grounding current clock regressed")
	}
	if state != ReviewGroundingCurrentActiveV2 || !current || !now.Before(time.Unix(0, expires)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review grounding projection is inactive or expired")
	}
	return nil
}
func locatorKeyV2(v ReviewArtifactLocatorV2) string {
	return string(v.Kind) + "\x00" + v.Schema.Key() + "\x00" + string(v.LocatorDigest)
}
func cloneLocatorsV2(values []ReviewArtifactLocatorV2) []ReviewArtifactLocatorV2 {
	out := append([]ReviewArtifactLocatorV2(nil), values...)
	for i := range out {
		out[i].Payload.Inline = bytes.Clone(out[i].Payload.Inline)
	}
	return out
}
func digestLocatorSetV2(values []ReviewArtifactLocatorV2) (core.Digest, error) {
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return "", err
		}
	}
	return core.CanonicalJSONDigest("praxis.review.artifact-locator-set/v2", "2.0.0", "ReviewArtifactLocatorSetV2", cloneLocatorsV2(values))
}
func groundingInvalidV2(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}
func groundingInvalidCanonicalV2(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, message)
}
func groundingDuplicateV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, message)
}
func groundingConflictV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}
func groundingDigestConflictV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, message)
}

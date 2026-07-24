package contract

import (
	"errors"
	"slices"
	"strings"
	"time"
)

const (
	SnapshotStorageArtifactTypeURL = "praxis.sandbox/snapshot-storage-artifact-ref/v2"
	SnapshotStorageArtifactDomain  = "praxis.sandbox/snapshot-storage-artifact-ref/body/v2"
	SnapshotArtifactFactTypeURL    = "praxis.sandbox/snapshot-artifact-fact-ref/v2"
	SnapshotArtifactFactDomain     = "praxis.sandbox/snapshot-artifact-fact/body/v2"
)

type SnapshotArtifactFactState string

const SnapshotArtifactAvailable SnapshotArtifactFactState = "available"

// SnapshotStorageArtifactRefV2 is an opaque, content-addressed storage
// coordinate. Backend locators and credentials are intentionally absent.
type SnapshotStorageArtifactRefV2 struct {
	TypeURL                  string                     `json:"type_url"`
	Version                  uint32                     `json:"version"`
	StorageArtifactID        string                     `json:"storage_artifact_id"`
	Revision                 uint64                     `json:"revision"`
	DigestAlgorithm          string                     `json:"digest_algorithm"`
	DigestDomain             string                     `json:"digest_domain"`
	Digest                   string                     `json:"digest"`
	TenantID                 string                     `json:"tenant_id"`
	DataDomain               string                     `json:"data_domain"`
	StorageNamespaceExactRef SnapshotArtifactExactRefV2 `json:"storage_namespace_exact_ref"`
	ContentDigest            string                     `json:"content_digest"`
	SchemaRef                Ref                        `json:"schema_ref"`
	Length                   uint64                     `json:"length"`
	EncryptionFactRef        Ref                        `json:"encryption_fact_ref"`
	ResidencyFactRef         Ref                        `json:"residency_fact_ref"`
	CreatedUnixNano          int64                      `json:"created_unix_nano"`
	ExpiresUnixNano          int64                      `json:"expires_unix_nano"`
}

func SealSnapshotStorageArtifactRefV2(value SnapshotStorageArtifactRefV2) (SnapshotStorageArtifactRefV2, error) {
	value.TypeURL = SnapshotStorageArtifactTypeURL
	value.Version = SnapshotArtifactVersion
	value.DigestAlgorithm = SnapshotArtifactDigestSHA256
	value.DigestDomain = SnapshotStorageArtifactDomain
	value.Digest = ""
	if err := validateSnapshotStorageArtifactBodyV2(value); err != nil {
		return SnapshotStorageArtifactRefV2{}, err
	}
	digest, err := Digest(SnapshotStorageArtifactDomain, value)
	if err != nil {
		return SnapshotStorageArtifactRefV2{}, err
	}
	value.Digest = digest
	return value, value.ValidateShape()
}

func (v SnapshotStorageArtifactRefV2) ValidateShape() error {
	if err := validateSnapshotStorageArtifactBodyV2(v); err != nil {
		return err
	}
	canonical := v
	canonical.Digest = ""
	digest, err := Digest(SnapshotStorageArtifactDomain, canonical)
	if err != nil {
		return err
	}
	if digest != v.Digest {
		return errors.New("snapshot storage artifact digest mismatch")
	}
	return nil
}

func (v SnapshotStorageArtifactRefV2) ValidateCurrent(now time.Time) error {
	if err := v.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < v.CreatedUnixNano || now.UnixNano() >= v.ExpiresUnixNano {
		return errors.New("snapshot storage artifact is stale")
	}
	return nil
}

func validateSnapshotStorageArtifactBodyV2(v SnapshotStorageArtifactRefV2) error {
	if v.TypeURL != SnapshotStorageArtifactTypeURL || v.Version != SnapshotArtifactVersion || v.DigestAlgorithm != SnapshotArtifactDigestSHA256 || v.DigestDomain != SnapshotStorageArtifactDomain || strings.TrimSpace(v.StorageArtifactID) == "" || v.Revision == 0 || strings.TrimSpace(v.TenantID) == "" || strings.TrimSpace(v.DataDomain) == "" || !ValidDigest(v.ContentDigest) || v.Length == 0 || v.CreatedUnixNano <= 0 || v.ExpiresUnixNano <= v.CreatedUnixNano {
		return errors.New("snapshot storage artifact contract is incomplete")
	}
	if err := v.StorageNamespaceExactRef.ValidateShape("snapshot storage namespace"); err != nil {
		return err
	}
	if v.StorageNamespaceExactRef.ExpiresUnixNano < v.ExpiresUnixNano {
		return errors.New("snapshot storage artifact outlives its namespace")
	}
	for name, ref := range map[string]Ref{"schema": v.SchemaRef, "encryption fact": v.EncryptionFactRef, "residency fact": v.ResidencyFactRef} {
		if err := ref.ValidateShape("snapshot storage artifact " + name); err != nil {
			return err
		}
	}
	return nil
}

func (v SnapshotStorageArtifactRefV2) ExactRef() SnapshotArtifactExactRefV2 {
	return SnapshotArtifactExactRefV2{TypeURL: v.TypeURL, Version: v.Version, ID: v.StorageArtifactID, Revision: v.Revision, DigestAlgorithm: v.DigestAlgorithm, DigestDomain: v.DigestDomain, Digest: v.Digest, ExpiresUnixNano: v.ExpiresUnixNano}
}

// SnapshotArtifactFactV2 is the immutable Sandbox Owner fact. Observation and
// Receipt refs remain provenance and never become the fact by themselves.
type SnapshotArtifactFactV2 struct {
	Meta                   Meta                         `json:"meta"`
	TenantID               string                       `json:"tenant_id"`
	DataDomain             string                       `json:"data_domain"`
	ReservationFactRef     SnapshotArtifactExactRefV2   `json:"reservation_fact_ref"`
	ArtifactSubjectRef     SnapshotArtifactSubjectRefV2 `json:"artifact_subject_ref"`
	StorageArtifactRef     SnapshotStorageArtifactRefV2 `json:"storage_artifact_ref"`
	SchemaRef              Ref                          `json:"schema_ref"`
	ContentDigest          string                       `json:"content_digest"`
	Length                 uint64                       `json:"length"`
	EncryptionFactRef      Ref                          `json:"encryption_fact_ref"`
	ResidencyFactRef       Ref                          `json:"residency_fact_ref"`
	ProviderObservationRef Ref                          `json:"provider_observation_ref"`
	ProviderReceiptRef     Ref                          `json:"provider_receipt_ref"`
	FormalEvidenceRefs     []Ref                        `json:"formal_evidence_refs"`
	OwnerInspectionRef     Ref                          `json:"owner_inspection_ref"`
	SourceAttemptRef       Ref                          `json:"source_attempt_ref"`
	RequestedNotAfter      int64                        `json:"requested_not_after"`
	State                  SnapshotArtifactFactState    `json:"state"`
}

func SealSnapshotArtifactFactV2(value SnapshotArtifactFactV2) (SnapshotArtifactFactV2, error) {
	value.FormalEvidenceRefs = append([]Ref(nil), value.FormalEvidenceRefs...)
	slices.SortFunc(value.FormalEvidenceRefs, compareSnapshotArtifactRefV2)
	value.Meta.Digest = ""
	if err := validateSnapshotArtifactFactBodyV2(value); err != nil {
		return SnapshotArtifactFactV2{}, err
	}
	digest, err := Digest(SnapshotArtifactFactDomain, value)
	if err != nil {
		return SnapshotArtifactFactV2{}, err
	}
	value.Meta.Digest = digest
	return value, value.ValidateShape()
}

func (v SnapshotArtifactFactV2) ValidateShape() error {
	if err := v.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := validateSnapshotArtifactFactBodyV2(v); err != nil {
		return err
	}
	canonical := v
	canonical.FormalEvidenceRefs = append([]Ref(nil), v.FormalEvidenceRefs...)
	canonical.Meta.Digest = ""
	digest, err := Digest(SnapshotArtifactFactDomain, canonical)
	if err != nil {
		return err
	}
	if digest != v.Meta.Digest {
		return errors.New("snapshot artifact fact digest mismatch")
	}
	return nil
}

func (v SnapshotArtifactFactV2) ValidateCurrent(now time.Time) error {
	if err := v.ValidateShape(); err != nil {
		return err
	}
	return v.Meta.ValidateCurrent(now)
}

func validateSnapshotArtifactFactBodyV2(v SnapshotArtifactFactV2) error {
	if strings.TrimSpace(v.TenantID) == "" || strings.TrimSpace(v.DataDomain) == "" || v.State != SnapshotArtifactAvailable || v.RequestedNotAfter <= 0 || !ValidDigest(v.ContentDigest) || v.Length == 0 || len(v.FormalEvidenceRefs) == 0 {
		return errors.New("snapshot artifact fact is incomplete")
	}
	if err := v.ReservationFactRef.ValidateShape("snapshot artifact reservation fact"); err != nil {
		return err
	}
	if v.ReservationFactRef.TypeURL != SnapshotArtifactReservationFactTypeURL || v.ReservationFactRef.DigestDomain != SnapshotArtifactReservationFactDomain {
		return errors.New("snapshot artifact fact has wrong reservation fact type")
	}
	if err := v.ArtifactSubjectRef.ValidateShape(); err != nil {
		return err
	}
	if err := v.StorageArtifactRef.ValidateShape(); err != nil {
		return err
	}
	for name, ref := range map[string]Ref{"schema": v.SchemaRef, "encryption fact": v.EncryptionFactRef, "residency fact": v.ResidencyFactRef, "provider observation": v.ProviderObservationRef, "provider receipt": v.ProviderReceiptRef, "owner inspection": v.OwnerInspectionRef, "source attempt": v.SourceAttemptRef} {
		if err := ref.ValidateShape("snapshot artifact " + name); err != nil {
			return err
		}
	}
	if !slices.IsSortedFunc(v.FormalEvidenceRefs, compareSnapshotArtifactRefV2) {
		return errors.New("snapshot artifact evidence refs are not canonical")
	}
	for index, ref := range v.FormalEvidenceRefs {
		if err := ref.ValidateShape("snapshot artifact evidence"); err != nil {
			return err
		}
		if index > 0 && SameRef(v.FormalEvidenceRefs[index-1], ref) {
			return errors.New("snapshot artifact evidence refs contain duplicates")
		}
	}
	if v.TenantID != v.ArtifactSubjectRef.TenantID || v.TenantID != v.StorageArtifactRef.TenantID || v.DataDomain != v.ArtifactSubjectRef.DataDomain || v.DataDomain != v.StorageArtifactRef.DataDomain || !SameRef(v.SchemaRef, v.StorageArtifactRef.SchemaRef) || v.ContentDigest != v.StorageArtifactRef.ContentDigest || v.Length != v.StorageArtifactRef.Length || !SameRef(v.EncryptionFactRef, v.StorageArtifactRef.EncryptionFactRef) || !SameRef(v.ResidencyFactRef, v.StorageArtifactRef.ResidencyFactRef) {
		return errors.New("snapshot artifact fact storage binding mismatch")
	}
	if v.Meta.ExpiresUnixNano > v.RequestedNotAfter || v.Meta.ExpiresUnixNano > v.ReservationFactRef.ExpiresUnixNano || v.Meta.ExpiresUnixNano > v.ArtifactSubjectRef.ExpiresUnixNano || v.Meta.ExpiresUnixNano > v.StorageArtifactRef.ExpiresUnixNano {
		return errors.New("snapshot artifact fact TTL exceeds an upstream bound")
	}
	return nil
}

func (v SnapshotArtifactFactV2) ExactRef() SnapshotArtifactExactRefV2 {
	return SnapshotArtifactExactRefV2{TypeURL: SnapshotArtifactFactTypeURL, Version: SnapshotArtifactVersion, ID: v.Meta.ID, Revision: v.Meta.Revision, DigestAlgorithm: SnapshotArtifactDigestSHA256, DigestDomain: SnapshotArtifactFactDomain, Digest: v.Meta.Digest, ExpiresUnixNano: v.Meta.ExpiresUnixNano}
}

func SnapshotArtifactFactMatchesCommitRequestV2(v SnapshotArtifactFactV2, r CommitSnapshotArtifactRequestV2) bool {
	if !SameSnapshotArtifactExactRef(v.StorageArtifactRef.ExactRef(), r.StorageArtifactRef.ExactRef()) || !SameRef(v.ProviderObservationRef, r.ProviderObservationRef) || !SameRef(v.ProviderReceiptRef, r.ProviderReceiptRef) || !SameRef(v.OwnerInspectionRef, r.OwnerInspectionRef) || !SameRef(v.SourceAttemptRef, r.SourceAttemptRef) || v.RequestedNotAfter != r.RequestedNotAfter || len(v.FormalEvidenceRefs) != len(r.FormalEvidenceRefs) {
		return false
	}
	for index := range v.FormalEvidenceRefs {
		if !SameRef(v.FormalEvidenceRefs[index], r.FormalEvidenceRefs[index]) {
			return false
		}
	}
	return true
}

func compareSnapshotArtifactRefV2(a, b Ref) int {
	if result := strings.Compare(a.ID, b.ID); result != 0 {
		return result
	}
	if a.Revision < b.Revision {
		return -1
	}
	if a.Revision > b.Revision {
		return 1
	}
	return strings.Compare(a.Digest, b.Digest)
}

// CommitSnapshotArtifactRequestV2 carries only exact coordinates. The Owner
// must re-read them through SnapshotArtifactCommitCurrentReaderV2 before CAS.
type CommitSnapshotArtifactRequestV2 struct {
	ReservationRef         SnapshotArtifactExactRefV2     `json:"reservation_ref"`
	ExpectedAggregateRef   SnapshotArtifactAggregateRefV2 `json:"expected_aggregate_ref"`
	StorageArtifactRef     SnapshotStorageArtifactRefV2   `json:"storage_artifact_ref"`
	ProviderObservationRef Ref                            `json:"provider_observation_ref"`
	ProviderReceiptRef     Ref                            `json:"provider_receipt_ref"`
	FormalEvidenceRefs     []Ref                          `json:"formal_evidence_refs"`
	OwnerInspectionRef     Ref                            `json:"owner_inspection_ref"`
	SourceAttemptRef       Ref                            `json:"source_attempt_ref"`
	RequestedNotAfter      int64                          `json:"requested_not_after"`
}

func (r CommitSnapshotArtifactRequestV2) Clone() CommitSnapshotArtifactRequestV2 {
	r.FormalEvidenceRefs = append([]Ref(nil), r.FormalEvidenceRefs...)
	return r
}

func (r CommitSnapshotArtifactRequestV2) ValidateCurrent(now time.Time) error {
	if err := r.ReservationRef.ValidateCurrent("snapshot artifact commit reservation", now); err != nil {
		return err
	}
	if err := r.ExpectedAggregateRef.ValidateShape(); err != nil {
		return err
	}
	if err := r.StorageArtifactRef.ValidateCurrent(now); err != nil {
		return err
	}
	for name, ref := range map[string]Ref{"provider observation": r.ProviderObservationRef, "provider receipt": r.ProviderReceiptRef, "owner inspection": r.OwnerInspectionRef, "source attempt": r.SourceAttemptRef} {
		if err := ref.ValidateShape("snapshot artifact commit " + name); err != nil {
			return err
		}
	}
	if len(r.FormalEvidenceRefs) == 0 || !slices.IsSortedFunc(r.FormalEvidenceRefs, compareSnapshotArtifactRefV2) {
		return errors.New("snapshot artifact commit evidence refs are empty or non-canonical")
	}
	for index, ref := range r.FormalEvidenceRefs {
		if err := ref.ValidateShape("snapshot artifact commit evidence"); err != nil {
			return err
		}
		if index > 0 && SameRef(r.FormalEvidenceRefs[index-1], ref) {
			return errors.New("snapshot artifact commit evidence refs contain duplicates")
		}
	}
	if r.RequestedNotAfter <= 0 || now.IsZero() || now.UnixNano() >= r.RequestedNotAfter {
		return errors.New("snapshot artifact commit request is stale")
	}
	return nil
}

type SnapshotArtifactCommitCurrentProjectionV2 struct {
	TenantID               string                         `json:"tenant_id"`
	DataDomain             string                         `json:"data_domain"`
	ReservationRef         SnapshotArtifactExactRefV2     `json:"reservation_ref"`
	ExpectedAggregateRef   SnapshotArtifactAggregateRefV2 `json:"expected_aggregate_ref"`
	StorageArtifactRef     SnapshotStorageArtifactRefV2   `json:"storage_artifact_ref"`
	ProviderObservationRef Ref                            `json:"provider_observation_ref"`
	ProviderReceiptRef     Ref                            `json:"provider_receipt_ref"`
	FormalEvidenceRefs     []Ref                          `json:"formal_evidence_refs"`
	OwnerInspectionRef     Ref                            `json:"owner_inspection_ref"`
	SourceAttemptRef       Ref                            `json:"source_attempt_ref"`
	CheckedUnixNano        int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano        int64                          `json:"expires_unix_nano"`
	ProjectionDigest       string                         `json:"projection_digest"`
}

func SealSnapshotArtifactCommitCurrentProjectionV2(value SnapshotArtifactCommitCurrentProjectionV2, now time.Time) (SnapshotArtifactCommitCurrentProjectionV2, error) {
	value.FormalEvidenceRefs = append([]Ref(nil), value.FormalEvidenceRefs...)
	slices.SortFunc(value.FormalEvidenceRefs, compareSnapshotArtifactRefV2)
	value.ProjectionDigest = ""
	digest, err := Digest("praxis.sandbox/snapshot-artifact-commit-current/body/v2", value)
	if err != nil {
		return SnapshotArtifactCommitCurrentProjectionV2{}, err
	}
	value.ProjectionDigest = digest
	return value, value.ValidateCurrent(now)
}

func (v SnapshotArtifactCommitCurrentProjectionV2) ValidateCurrent(now time.Time) error {
	request := CommitSnapshotArtifactRequestV2{ReservationRef: v.ReservationRef, ExpectedAggregateRef: v.ExpectedAggregateRef, StorageArtifactRef: v.StorageArtifactRef, ProviderObservationRef: v.ProviderObservationRef, ProviderReceiptRef: v.ProviderReceiptRef, FormalEvidenceRefs: append([]Ref(nil), v.FormalEvidenceRefs...), OwnerInspectionRef: v.OwnerInspectionRef, SourceAttemptRef: v.SourceAttemptRef, RequestedNotAfter: v.ExpiresUnixNano}
	if strings.TrimSpace(v.TenantID) == "" || strings.TrimSpace(v.DataDomain) == "" || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano || now.IsZero() || now.UnixNano() < v.CheckedUnixNano || !now.Before(time.Unix(0, v.ExpiresUnixNano)) || !ValidDigest(v.ProjectionDigest) {
		return errors.New("snapshot artifact commit current projection is stale or incomplete")
	}
	if err := request.ValidateCurrent(now); err != nil {
		return err
	}
	canonical := v
	canonical.ProjectionDigest = ""
	digest, err := Digest("praxis.sandbox/snapshot-artifact-commit-current/body/v2", canonical)
	if err != nil || digest != v.ProjectionDigest {
		return errors.New("snapshot artifact commit current projection digest mismatch")
	}
	return nil
}

func (v SnapshotArtifactCommitCurrentProjectionV2) MatchesRequest(r CommitSnapshotArtifactRequestV2) bool {
	if !SameSnapshotArtifactExactRef(v.ReservationRef, r.ReservationRef) || !SameSnapshotArtifactAggregateRef(v.ExpectedAggregateRef, r.ExpectedAggregateRef) || v.StorageArtifactRef != r.StorageArtifactRef || !SameRef(v.ProviderObservationRef, r.ProviderObservationRef) || !SameRef(v.ProviderReceiptRef, r.ProviderReceiptRef) || !SameRef(v.OwnerInspectionRef, r.OwnerInspectionRef) || !SameRef(v.SourceAttemptRef, r.SourceAttemptRef) || len(v.FormalEvidenceRefs) != len(r.FormalEvidenceRefs) {
		return false
	}
	for index := range v.FormalEvidenceRefs {
		if !SameRef(v.FormalEvidenceRefs[index], r.FormalEvidenceRefs[index]) {
			return false
		}
	}
	return true
}

type SnapshotArtifactAvailableBundleV2 struct {
	ExpectedCurrentIndexRef SnapshotArtifactExactRefV2              `json:"expected_current_index_ref"`
	Fact                    SnapshotArtifactFactV2                  `json:"fact"`
	Entry                   SnapshotArtifactAggregateEntryV2        `json:"entry"`
	Envelope                SnapshotArtifactAggregateEnvelopeV2     `json:"envelope"`
	CurrentIndex            SnapshotArtifactAggregateCurrentIndexV2 `json:"current_index"`
	OwnerClockWatermark     int64                                   `json:"owner_clock_watermark"`
}

func (b SnapshotArtifactAvailableBundleV2) ValidateShape() error {
	if err := b.ExpectedCurrentIndexRef.ValidateShape("snapshot artifact expected current index"); err != nil {
		return err
	}
	if err := b.Fact.ValidateShape(); err != nil {
		return err
	}
	if err := b.Entry.ValidateShape(); err != nil {
		return err
	}
	if err := b.Envelope.ValidateShape(); err != nil {
		return err
	}
	if err := b.CurrentIndex.ValidateShape(); err != nil {
		return err
	}
	if b.OwnerClockWatermark <= 0 || b.OwnerClockWatermark != b.CurrentIndex.OwnerClockWatermark || b.Entry.EntryKind != SnapshotArtifactEntryArtifact || !SameSnapshotArtifactExactRef(b.Fact.ExactRef(), b.Entry.FactRef) || !SameSnapshotArtifactExactRef(b.Entry.ExactRef(), b.Envelope.AppliedEntryRef) || b.Envelope.ArtifactFactRef.Ref == nil || !SameSnapshotArtifactExactRef(b.Fact.ExactRef(), *b.Envelope.ArtifactFactRef.Ref) || b.CurrentIndex.ArtifactFactRef.Ref == nil || !SameSnapshotArtifactExactRef(b.Fact.ExactRef(), *b.CurrentIndex.ArtifactFactRef.Ref) || !SameSnapshotArtifactAggregateRef(b.Envelope.AggregateRef, b.CurrentIndex.HeadAggregateEnvelopeRef) || b.ExpectedCurrentIndexRef.ID != b.CurrentIndex.CurrentIndexRef.ID || b.ExpectedCurrentIndexRef.Revision+1 != b.CurrentIndex.CurrentIndexRef.Revision {
		return errors.New("snapshot artifact available bundle binding mismatch")
	}
	return nil
}

type CommitSnapshotArtifactResultV2 struct {
	Fact         SnapshotArtifactFactV2                  `json:"fact"`
	CurrentIndex SnapshotArtifactAggregateCurrentIndexV2 `json:"current_index"`
	Created      bool                                    `json:"created"`
}

type InspectSnapshotArtifactFactRequestV2 struct {
	ExpectedRef SnapshotArtifactExactRefV2 `json:"expected_ref"`
}

func (r InspectSnapshotArtifactFactRequestV2) ValidateShape() error {
	if err := r.ExpectedRef.ValidateShape("snapshot artifact fact inspect"); err != nil {
		return err
	}
	if r.ExpectedRef.TypeURL != SnapshotArtifactFactTypeURL || r.ExpectedRef.DigestDomain != SnapshotArtifactFactDomain {
		return errors.New("snapshot artifact fact inspect has wrong exact type")
	}
	return nil
}

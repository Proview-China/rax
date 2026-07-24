package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	SnapshotArtifactOwnerV2 = "praxis.sandbox/snapshot-artifact-owner/v2"

	SnapshotArtifactSubjectIdentityTypeURL  = "praxis.sandbox/snapshot-artifact-subject-identity/v2"
	SnapshotArtifactSubjectIdentityDomain   = "praxis.sandbox/snapshot-artifact-subject-identity/body/v2"
	SnapshotArtifactSubjectRefTypeURL       = "praxis.sandbox/snapshot-artifact-subject-ref/v2"
	SnapshotArtifactSubjectRefDomain        = "praxis.sandbox/snapshot-artifact-subject-ref/body/v2"
	SnapshotArtifactReservationTypeURL      = "praxis.sandbox/snapshot-artifact-reservation/v2"
	SnapshotArtifactReservationDomain       = "praxis.sandbox/snapshot-artifact-reservation/body/v2"
	SnapshotArtifactReservationFactTypeURL  = "praxis.sandbox/snapshot-artifact-reservation-fact/v2"
	SnapshotArtifactReservationFactDomain   = "praxis.sandbox/snapshot-artifact-reservation-fact/body/v2"
	SnapshotArtifactEntryTypeURL            = "praxis.sandbox/snapshot-artifact-entry/v2"
	SnapshotArtifactEntryDomain             = "praxis.sandbox/snapshot-artifact-entry/body/v2"
	SnapshotArtifactAggregateRefTypeURL     = "praxis.sandbox/snapshot-artifact-aggregate-ref/v2"
	SnapshotArtifactAggregateRefDomain      = "praxis.sandbox/snapshot-artifact-aggregate-ref/body/v2"
	SnapshotArtifactCurrentIndexTypeURL     = "praxis.sandbox/snapshot-artifact-current-index/v2"
	SnapshotArtifactCurrentIndexDomain      = "praxis.sandbox/snapshot-artifact-current-index/body/v2"
	SnapshotArtifactCurrentProjectionDomain = "praxis.sandbox/snapshot-artifact-current-projection/body/v2"

	SnapshotArtifactVersion       = uint32(2)
	SnapshotArtifactDigestSHA256  = "sha256"
	SnapshotArtifactSubjectKind   = "praxis.sandbox/snapshot-artifact-subject"
	SnapshotArtifactAggregateKind = "praxis.sandbox/snapshot-artifact-aggregate"
)

type SnapshotArtifactPresence string

const (
	SnapshotArtifactAbsent  SnapshotArtifactPresence = "absent"
	SnapshotArtifactPresent SnapshotArtifactPresence = "present"
)

func (p SnapshotArtifactPresence) Validate() error {
	switch p {
	case SnapshotArtifactAbsent, SnapshotArtifactPresent:
		return nil
	default:
		return errors.New("snapshot artifact presence must be explicitly absent or present")
	}
}

type SnapshotArtifactExactRefV2 struct {
	TypeURL         string `json:"type_url"`
	Version         uint32 `json:"version"`
	ID              string `json:"id"`
	Revision        uint64 `json:"revision"`
	DigestAlgorithm string `json:"digest_algorithm"`
	DigestDomain    string `json:"digest_domain"`
	Digest          string `json:"digest"`
	ExpiresUnixNano int64  `json:"expires_unix_nano"`
}

func (r SnapshotArtifactExactRefV2) ValidateShape(name string) error {
	if strings.TrimSpace(r.TypeURL) == "" || r.Version == 0 || strings.TrimSpace(r.ID) == "" || r.Revision == 0 {
		return fmt.Errorf("%s exact identity is incomplete", name)
	}
	if r.DigestAlgorithm != SnapshotArtifactDigestSHA256 || strings.TrimSpace(r.DigestDomain) == "" || !ValidDigest(r.Digest) {
		return fmt.Errorf("%s exact digest contract is invalid", name)
	}
	if r.ExpiresUnixNano <= 0 {
		return fmt.Errorf("%s exact expiry is required", name)
	}
	return nil
}

func (r SnapshotArtifactExactRefV2) ValidateCurrent(name string, now time.Time) error {
	if err := r.ValidateShape(name); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() >= r.ExpiresUnixNano {
		return fmt.Errorf("%s exact ref is expired", name)
	}
	return nil
}

func SameSnapshotArtifactExactRef(a, b SnapshotArtifactExactRefV2) bool {
	return a == b
}

type SnapshotArtifactOptionalExactRefV2 struct {
	Presence SnapshotArtifactPresence    `json:"presence"`
	Ref      *SnapshotArtifactExactRefV2 `json:"ref,omitempty"`
}

func (r SnapshotArtifactOptionalExactRefV2) ValidateShape(name string) error {
	if err := r.Presence.Validate(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if r.Presence == SnapshotArtifactAbsent {
		if r.Ref != nil {
			return fmt.Errorf("%s is absent but carries a ref", name)
		}
		return nil
	}
	if r.Ref == nil {
		return fmt.Errorf("%s is present but carries a nil ref", name)
	}
	return r.Ref.ValidateShape(name + " ref")
}

type SnapshotArtifactStableSourceKeyV2 struct {
	TenantID        string `json:"tenant_id"`
	DataDomain      string `json:"data_domain"`
	SourceAttemptID string `json:"source_attempt_id"`
}

func (k SnapshotArtifactStableSourceKeyV2) ValidateShape() error {
	if strings.TrimSpace(k.TenantID) == "" || strings.TrimSpace(k.DataDomain) == "" || strings.TrimSpace(k.SourceAttemptID) == "" {
		return errors.New("snapshot artifact stable source key is incomplete")
	}
	return nil
}

func SnapshotArtifactStableSourceKeyDigest(k SnapshotArtifactStableSourceKeyV2) (string, error) {
	if err := k.ValidateShape(); err != nil {
		return "", err
	}
	return Digest("snapshot-artifact-stable-source-key-v2", k)
}

type SnapshotArtifactSubjectIdentityV2 struct {
	TypeURL             string `json:"type_url"`
	Version             uint32 `json:"version"`
	Owner               string `json:"owner"`
	Kind                string `json:"kind"`
	ArtifactAggregateID string `json:"artifact_aggregate_id"`
	TenantID            string `json:"tenant_id"`
	DataDomain          string `json:"data_domain"`
	ReservationID       string `json:"reservation_id"`
	SourceAttemptID     string `json:"source_attempt_id"`
	DigestAlgorithm     string `json:"digest_algorithm"`
	DigestDomain        string `json:"digest_domain"`
	StableSubjectDigest string `json:"stable_subject_digest"`
}

func SealSnapshotArtifactSubjectIdentityV2(value SnapshotArtifactSubjectIdentityV2) (SnapshotArtifactSubjectIdentityV2, error) {
	value.TypeURL = SnapshotArtifactSubjectIdentityTypeURL
	value.Version = SnapshotArtifactVersion
	value.Owner = SnapshotArtifactOwnerV2
	value.Kind = SnapshotArtifactSubjectKind
	value.DigestAlgorithm = SnapshotArtifactDigestSHA256
	value.DigestDomain = SnapshotArtifactSubjectIdentityDomain
	value.StableSubjectDigest = ""
	if err := validateSnapshotArtifactSubjectIdentityBody(value); err != nil {
		return SnapshotArtifactSubjectIdentityV2{}, err
	}
	digest, err := Digest(SnapshotArtifactSubjectIdentityDomain, value)
	if err != nil {
		return SnapshotArtifactSubjectIdentityV2{}, err
	}
	value.StableSubjectDigest = digest
	return value, value.ValidateShape()
}

func (v SnapshotArtifactSubjectIdentityV2) ValidateShape() error {
	if err := validateSnapshotArtifactSubjectIdentityBody(v); err != nil {
		return err
	}
	if !ValidDigest(v.StableSubjectDigest) {
		return errors.New("snapshot artifact stable subject digest is invalid")
	}
	canonical := v
	canonical.StableSubjectDigest = ""
	digest, err := Digest(SnapshotArtifactSubjectIdentityDomain, canonical)
	if err != nil {
		return err
	}
	if digest != v.StableSubjectDigest {
		return errors.New("snapshot artifact stable subject digest mismatch")
	}
	return nil
}

func validateSnapshotArtifactSubjectIdentityBody(v SnapshotArtifactSubjectIdentityV2) error {
	if v.TypeURL != SnapshotArtifactSubjectIdentityTypeURL || v.Version != SnapshotArtifactVersion || v.Owner != SnapshotArtifactOwnerV2 || v.Kind != SnapshotArtifactSubjectKind || v.DigestAlgorithm != SnapshotArtifactDigestSHA256 || v.DigestDomain != SnapshotArtifactSubjectIdentityDomain {
		return errors.New("snapshot artifact stable subject contract is invalid")
	}
	if strings.TrimSpace(v.ArtifactAggregateID) == "" || strings.TrimSpace(v.TenantID) == "" || strings.TrimSpace(v.DataDomain) == "" || strings.TrimSpace(v.ReservationID) == "" || strings.TrimSpace(v.SourceAttemptID) == "" {
		return errors.New("snapshot artifact stable subject identity is incomplete")
	}
	return nil
}

type SnapshotArtifactSubjectRefV2 struct {
	TypeURL             string `json:"type_url"`
	Version             uint32 `json:"version"`
	Owner               string `json:"owner"`
	Kind                string `json:"kind"`
	ArtifactAggregateID string `json:"artifact_aggregate_id"`
	Revision            uint64 `json:"revision"`
	TenantID            string `json:"tenant_id"`
	DataDomain          string `json:"data_domain"`
	ReservationID       string `json:"reservation_id"`
	SourceAttemptID     string `json:"source_attempt_id"`
	SchemaRef           Ref    `json:"schema_ref"`
	DigestAlgorithm     string `json:"digest_algorithm"`
	DigestDomain        string `json:"digest_domain"`
	StableSubjectDigest string `json:"stable_subject_digest"`
	SubjectDigest       string `json:"subject_digest"`
	ExpiresUnixNano     int64  `json:"expires_unix_nano"`
}

func SealSnapshotArtifactSubjectRefV2(value SnapshotArtifactSubjectRefV2) (SnapshotArtifactSubjectRefV2, error) {
	value.TypeURL = SnapshotArtifactSubjectRefTypeURL
	value.Version = SnapshotArtifactVersion
	value.Owner = SnapshotArtifactOwnerV2
	value.Kind = SnapshotArtifactSubjectKind
	value.DigestAlgorithm = SnapshotArtifactDigestSHA256
	value.DigestDomain = SnapshotArtifactSubjectRefDomain
	value.SubjectDigest = ""
	if err := validateSnapshotArtifactSubjectRefBody(value); err != nil {
		return SnapshotArtifactSubjectRefV2{}, err
	}
	digest, err := Digest(SnapshotArtifactSubjectRefDomain, value)
	if err != nil {
		return SnapshotArtifactSubjectRefV2{}, err
	}
	value.SubjectDigest = digest
	return value, value.ValidateShape()
}

func (v SnapshotArtifactSubjectRefV2) ValidateShape() error {
	if err := validateSnapshotArtifactSubjectRefBody(v); err != nil {
		return err
	}
	if !ValidDigest(v.SubjectDigest) {
		return errors.New("snapshot artifact subject digest is invalid")
	}
	identity, err := SealSnapshotArtifactSubjectIdentityV2(SnapshotArtifactSubjectIdentityV2{
		ArtifactAggregateID: v.ArtifactAggregateID,
		TenantID:            v.TenantID,
		DataDomain:          v.DataDomain,
		ReservationID:       v.ReservationID,
		SourceAttemptID:     v.SourceAttemptID,
	})
	if err != nil || identity.StableSubjectDigest != v.StableSubjectDigest {
		return errors.New("snapshot artifact subject stable identity mismatch")
	}
	canonical := v
	canonical.SubjectDigest = ""
	digest, err := Digest(SnapshotArtifactSubjectRefDomain, canonical)
	if err != nil {
		return err
	}
	if digest != v.SubjectDigest {
		return errors.New("snapshot artifact subject digest mismatch")
	}
	return nil
}

func (v SnapshotArtifactSubjectRefV2) ValidateCurrent(now time.Time) error {
	if err := v.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() >= v.ExpiresUnixNano {
		return errors.New("snapshot artifact subject ref is expired")
	}
	return nil
}

func validateSnapshotArtifactSubjectRefBody(v SnapshotArtifactSubjectRefV2) error {
	if v.TypeURL != SnapshotArtifactSubjectRefTypeURL || v.Version != SnapshotArtifactVersion || v.Owner != SnapshotArtifactOwnerV2 || v.Kind != SnapshotArtifactSubjectKind || v.DigestAlgorithm != SnapshotArtifactDigestSHA256 || v.DigestDomain != SnapshotArtifactSubjectRefDomain || v.Revision == 0 || v.ExpiresUnixNano <= 0 {
		return errors.New("snapshot artifact subject ref contract is invalid")
	}
	if strings.TrimSpace(v.ArtifactAggregateID) == "" || strings.TrimSpace(v.TenantID) == "" || strings.TrimSpace(v.DataDomain) == "" || strings.TrimSpace(v.ReservationID) == "" || strings.TrimSpace(v.SourceAttemptID) == "" || !ValidDigest(v.StableSubjectDigest) {
		return errors.New("snapshot artifact subject ref identity is incomplete")
	}
	return v.SchemaRef.ValidateShape("snapshot artifact subject schema")
}

func (v SnapshotArtifactSubjectRefV2) ExactRef() SnapshotArtifactExactRefV2 {
	return SnapshotArtifactExactRefV2{TypeURL: v.TypeURL, Version: v.Version, ID: v.ArtifactAggregateID, Revision: v.Revision, DigestAlgorithm: v.DigestAlgorithm, DigestDomain: v.DigestDomain, Digest: v.SubjectDigest, ExpiresUnixNano: v.ExpiresUnixNano}
}

type SnapshotArtifactOptionalAggregateRefV2 struct {
	Presence SnapshotArtifactPresence        `json:"presence"`
	Ref      *SnapshotArtifactAggregateRefV2 `json:"ref,omitempty"`
}

func (r SnapshotArtifactOptionalAggregateRefV2) ValidateShape(name string) error {
	if err := r.Presence.Validate(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if r.Presence == SnapshotArtifactAbsent {
		if r.Ref != nil {
			return fmt.Errorf("%s is absent but carries a ref", name)
		}
		return nil
	}
	if r.Ref == nil {
		return fmt.Errorf("%s is present but carries a nil ref", name)
	}
	return r.Ref.ValidateShape()
}

type ReserveArtifactRequestV2 struct {
	TenantID              string                                 `json:"tenant_id"`
	DataDomain            string                                 `json:"data_domain"`
	SourceOperationID     string                                 `json:"source_operation_id"`
	SourceEffectID        string                                 `json:"source_effect_id"`
	SourceAttemptRef      Ref                                    `json:"source_attempt_ref"`
	SchemaRef             Ref                                    `json:"schema_ref"`
	ExpectedContentDigest string                                 `json:"expected_content_digest"`
	RetentionPolicyRef    Ref                                    `json:"retention_policy_ref"`
	EncryptionPolicyRef   Ref                                    `json:"encryption_policy_ref"`
	ResidencyPolicyRef    Ref                                    `json:"residency_policy_ref"`
	ExpectedAggregateRef  SnapshotArtifactOptionalAggregateRefV2 `json:"expected_aggregate_ref"`
	RequestedNotAfter     int64                                  `json:"requested_not_after"`
}

func (r ReserveArtifactRequestV2) StableSourceKey() SnapshotArtifactStableSourceKeyV2 {
	return SnapshotArtifactStableSourceKeyV2{TenantID: r.TenantID, DataDomain: r.DataDomain, SourceAttemptID: r.SourceAttemptRef.ID}
}

func (r ReserveArtifactRequestV2) ValidateShape() error {
	if strings.TrimSpace(r.TenantID) == "" || strings.TrimSpace(r.DataDomain) == "" || strings.TrimSpace(r.SourceOperationID) == "" || strings.TrimSpace(r.SourceEffectID) == "" {
		return errors.New("snapshot artifact reserve coordinates are incomplete")
	}
	for name, ref := range map[string]Ref{
		"source attempt":    r.SourceAttemptRef,
		"schema":            r.SchemaRef,
		"retention policy":  r.RetentionPolicyRef,
		"encryption policy": r.EncryptionPolicyRef,
		"residency policy":  r.ResidencyPolicyRef,
	} {
		if err := ref.ValidateShape("snapshot artifact " + name + " ref"); err != nil {
			return err
		}
	}
	if !ValidDigest(r.ExpectedContentDigest) {
		return errors.New("snapshot artifact expected content digest is invalid")
	}
	if err := r.ExpectedAggregateRef.ValidateShape("snapshot artifact expected aggregate"); err != nil {
		return err
	}
	if r.ExpectedAggregateRef.Presence != SnapshotArtifactAbsent {
		return errors.New("initial snapshot artifact reserve requires expected aggregate absent")
	}
	if r.RequestedNotAfter <= 0 {
		return errors.New("snapshot artifact requested not after is required")
	}
	return r.StableSourceKey().ValidateShape()
}

func (r ReserveArtifactRequestV2) ValidateCurrent(now time.Time) error {
	if err := r.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() >= r.RequestedNotAfter {
		return errors.New("snapshot artifact reserve request is expired")
	}
	return nil
}

type SnapshotArtifactReservationV2 struct {
	Meta                  Meta                                   `json:"meta"`
	TenantID              string                                 `json:"tenant_id"`
	DataDomain            string                                 `json:"data_domain"`
	SourceOperationID     string                                 `json:"source_operation_id"`
	SourceEffectID        string                                 `json:"source_effect_id"`
	SourceAttemptRef      Ref                                    `json:"source_attempt_ref"`
	SchemaRef             Ref                                    `json:"schema_ref"`
	ExpectedContentDigest string                                 `json:"expected_content_digest"`
	RetentionPolicyRef    Ref                                    `json:"retention_policy_ref"`
	EncryptionPolicyRef   Ref                                    `json:"encryption_policy_ref"`
	ResidencyPolicyRef    Ref                                    `json:"residency_policy_ref"`
	ExpectedAggregateRef  SnapshotArtifactOptionalAggregateRefV2 `json:"expected_aggregate_ref"`
	RequestedNotAfter     int64                                  `json:"requested_not_after"`
	SubjectIdentity       SnapshotArtifactSubjectIdentityV2      `json:"subject_identity"`
	SubjectRef            SnapshotArtifactSubjectRefV2           `json:"subject_ref"`
}

func SealSnapshotArtifactReservationV2(value SnapshotArtifactReservationV2) (SnapshotArtifactReservationV2, error) {
	value.Meta.Digest = ""
	if err := validateSnapshotArtifactReservationBody(value); err != nil {
		return SnapshotArtifactReservationV2{}, err
	}
	digest, err := Digest(SnapshotArtifactReservationDomain, value)
	if err != nil {
		return SnapshotArtifactReservationV2{}, err
	}
	value.Meta.Digest = digest
	return value, value.ValidateShape()
}

func (v SnapshotArtifactReservationV2) ValidateShape() error {
	if err := v.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := validateSnapshotArtifactReservationBody(v); err != nil {
		return err
	}
	canonical := v
	canonical.Meta.Digest = ""
	digest, err := Digest(SnapshotArtifactReservationDomain, canonical)
	if err != nil {
		return err
	}
	if digest != v.Meta.Digest {
		return errors.New("snapshot artifact reservation digest mismatch")
	}
	return nil
}

func (v SnapshotArtifactReservationV2) ValidateCurrent(now time.Time) error {
	if err := v.ValidateShape(); err != nil {
		return err
	}
	return v.Meta.ValidateCurrent(now)
}

func validateSnapshotArtifactReservationBody(v SnapshotArtifactReservationV2) error {
	request := ReserveArtifactRequestV2{
		TenantID: v.TenantID, DataDomain: v.DataDomain, SourceOperationID: v.SourceOperationID,
		SourceEffectID: v.SourceEffectID, SourceAttemptRef: v.SourceAttemptRef, SchemaRef: v.SchemaRef,
		ExpectedContentDigest: v.ExpectedContentDigest, RetentionPolicyRef: v.RetentionPolicyRef,
		EncryptionPolicyRef: v.EncryptionPolicyRef, ResidencyPolicyRef: v.ResidencyPolicyRef,
		ExpectedAggregateRef: v.ExpectedAggregateRef, RequestedNotAfter: v.RequestedNotAfter,
	}
	if err := request.ValidateShape(); err != nil {
		return err
	}
	if err := v.SubjectIdentity.ValidateShape(); err != nil {
		return err
	}
	if err := v.SubjectRef.ValidateShape(); err != nil {
		return err
	}
	if v.Meta.ID != v.SubjectIdentity.ReservationID || v.SubjectRef.ReservationID != v.Meta.ID || v.SubjectIdentity.ArtifactAggregateID != v.SubjectRef.ArtifactAggregateID || v.SubjectIdentity.StableSubjectDigest != v.SubjectRef.StableSubjectDigest || v.SubjectRef.TenantID != v.TenantID || v.SubjectRef.DataDomain != v.DataDomain || v.SubjectRef.SourceAttemptID != v.SourceAttemptRef.ID {
		return errors.New("snapshot artifact reservation subject binding mismatch")
	}
	if v.Meta.ExpiresUnixNano > v.RequestedNotAfter || v.SubjectRef.ExpiresUnixNano > v.Meta.ExpiresUnixNano {
		return errors.New("snapshot artifact reservation TTL exceeds an upstream bound")
	}
	return nil
}

func (v SnapshotArtifactReservationV2) ExactRef() SnapshotArtifactExactRefV2 {
	return SnapshotArtifactExactRefV2{TypeURL: SnapshotArtifactReservationTypeURL, Version: SnapshotArtifactVersion, ID: v.Meta.ID, Revision: v.Meta.Revision, DigestAlgorithm: SnapshotArtifactDigestSHA256, DigestDomain: SnapshotArtifactReservationDomain, Digest: v.Meta.Digest, ExpiresUnixNano: v.Meta.ExpiresUnixNano}
}

func SnapshotArtifactReservationMatchesRequest(v SnapshotArtifactReservationV2, request ReserveArtifactRequestV2) bool {
	return v.TenantID == request.TenantID && v.DataDomain == request.DataDomain && v.SourceOperationID == request.SourceOperationID && v.SourceEffectID == request.SourceEffectID && SameRef(v.SourceAttemptRef, request.SourceAttemptRef) && SameRef(v.SchemaRef, request.SchemaRef) && v.ExpectedContentDigest == request.ExpectedContentDigest && SameRef(v.RetentionPolicyRef, request.RetentionPolicyRef) && SameRef(v.EncryptionPolicyRef, request.EncryptionPolicyRef) && SameRef(v.ResidencyPolicyRef, request.ResidencyPolicyRef) && v.ExpectedAggregateRef.Presence == request.ExpectedAggregateRef.Presence && v.ExpectedAggregateRef.Ref == nil && request.ExpectedAggregateRef.Ref == nil && v.RequestedNotAfter == request.RequestedNotAfter
}

type SnapshotArtifactReservationFactV2 struct {
	Meta               Meta                         `json:"meta"`
	TenantID           string                       `json:"tenant_id"`
	ReservationRef     SnapshotArtifactExactRefV2   `json:"reservation_ref"`
	ArtifactSubjectRef SnapshotArtifactSubjectRefV2 `json:"artifact_subject_ref"`
	RequestedNotAfter  int64                        `json:"requested_not_after"`
}

func SealSnapshotArtifactReservationFactV2(value SnapshotArtifactReservationFactV2) (SnapshotArtifactReservationFactV2, error) {
	value.Meta.Digest = ""
	if err := validateSnapshotArtifactReservationFactBody(value); err != nil {
		return SnapshotArtifactReservationFactV2{}, err
	}
	digest, err := Digest(SnapshotArtifactReservationFactDomain, value)
	if err != nil {
		return SnapshotArtifactReservationFactV2{}, err
	}
	value.Meta.Digest = digest
	return value, value.ValidateShape()
}

func (v SnapshotArtifactReservationFactV2) ValidateShape() error {
	if err := v.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := validateSnapshotArtifactReservationFactBody(v); err != nil {
		return err
	}
	canonical := v
	canonical.Meta.Digest = ""
	digest, err := Digest(SnapshotArtifactReservationFactDomain, canonical)
	if err != nil {
		return err
	}
	if digest != v.Meta.Digest {
		return errors.New("snapshot artifact reservation fact digest mismatch")
	}
	return nil
}

func validateSnapshotArtifactReservationFactBody(v SnapshotArtifactReservationFactV2) error {
	if strings.TrimSpace(v.TenantID) == "" || v.RequestedNotAfter <= 0 {
		return errors.New("snapshot artifact reservation fact is incomplete")
	}
	if err := v.ReservationRef.ValidateShape("snapshot artifact reservation fact reservation"); err != nil {
		return err
	}
	if v.ReservationRef.TypeURL != SnapshotArtifactReservationTypeURL || v.ReservationRef.DigestDomain != SnapshotArtifactReservationDomain {
		return errors.New("snapshot artifact reservation fact has wrong reservation ref type")
	}
	if err := v.ArtifactSubjectRef.ValidateShape(); err != nil {
		return err
	}
	if v.Meta.ExpiresUnixNano > v.RequestedNotAfter || v.Meta.ExpiresUnixNano > v.ReservationRef.ExpiresUnixNano || v.Meta.ExpiresUnixNano > v.ArtifactSubjectRef.ExpiresUnixNano {
		return errors.New("snapshot artifact reservation fact TTL exceeds an upstream bound")
	}
	return nil
}

func (v SnapshotArtifactReservationFactV2) ExactRef() SnapshotArtifactExactRefV2 {
	return SnapshotArtifactExactRefV2{TypeURL: SnapshotArtifactReservationFactTypeURL, Version: SnapshotArtifactVersion, ID: v.Meta.ID, Revision: v.Meta.Revision, DigestAlgorithm: SnapshotArtifactDigestSHA256, DigestDomain: SnapshotArtifactReservationFactDomain, Digest: v.Meta.Digest, ExpiresUnixNano: v.Meta.ExpiresUnixNano}
}

type SnapshotArtifactEntryKind string

const (
	SnapshotArtifactEntryReservation SnapshotArtifactEntryKind = "reservation"
	SnapshotArtifactEntryArtifact    SnapshotArtifactEntryKind = "artifact"
)

type SnapshotArtifactAggregateEntryV2 struct {
	Meta                Meta                               `json:"meta"`
	TenantID            string                             `json:"tenant_id"`
	ArtifactAggregateID string                             `json:"artifact_aggregate_id"`
	ArtifactSubjectRef  SnapshotArtifactSubjectRefV2       `json:"artifact_subject_ref"`
	EntryKind           SnapshotArtifactEntryKind          `json:"entry_kind"`
	FactRef             SnapshotArtifactExactRefV2         `json:"fact_ref"`
	PreviousEntryRef    SnapshotArtifactOptionalExactRefV2 `json:"previous_entry_ref"`
	RequestedNotAfter   int64                              `json:"requested_not_after"`
}

func SealSnapshotArtifactAggregateEntryV2(value SnapshotArtifactAggregateEntryV2) (SnapshotArtifactAggregateEntryV2, error) {
	value.Meta.Digest = ""
	if err := validateSnapshotArtifactAggregateEntryBody(value); err != nil {
		return SnapshotArtifactAggregateEntryV2{}, err
	}
	digest, err := Digest(SnapshotArtifactEntryDomain, value)
	if err != nil {
		return SnapshotArtifactAggregateEntryV2{}, err
	}
	value.Meta.Digest = digest
	return value, value.ValidateShape()
}

func (v SnapshotArtifactAggregateEntryV2) ValidateShape() error {
	if err := v.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := validateSnapshotArtifactAggregateEntryBody(v); err != nil {
		return err
	}
	canonical := v
	canonical.Meta.Digest = ""
	digest, err := Digest(SnapshotArtifactEntryDomain, canonical)
	if err != nil {
		return err
	}
	if digest != v.Meta.Digest {
		return errors.New("snapshot artifact entry digest mismatch")
	}
	return nil
}

func validateSnapshotArtifactAggregateEntryBody(v SnapshotArtifactAggregateEntryV2) error {
	if strings.TrimSpace(v.TenantID) == "" || strings.TrimSpace(v.ArtifactAggregateID) == "" || v.RequestedNotAfter <= 0 {
		return errors.New("snapshot artifact entry is incomplete")
	}
	if err := v.ArtifactSubjectRef.ValidateShape(); err != nil {
		return err
	}
	if v.ArtifactAggregateID != v.ArtifactSubjectRef.ArtifactAggregateID || v.TenantID != v.ArtifactSubjectRef.TenantID {
		return errors.New("snapshot artifact entry subject binding mismatch")
	}
	if err := v.FactRef.ValidateShape("snapshot artifact entry fact"); err != nil {
		return err
	}
	if err := v.PreviousEntryRef.ValidateShape("snapshot artifact previous entry"); err != nil {
		return err
	}
	if v.Meta.ExpiresUnixNano > v.RequestedNotAfter {
		return errors.New("snapshot artifact entry exceeds its caller bound")
	}
	switch v.EntryKind {
	case SnapshotArtifactEntryReservation:
		if v.FactRef.TypeURL != SnapshotArtifactReservationFactTypeURL || v.FactRef.DigestDomain != SnapshotArtifactReservationFactDomain || v.PreviousEntryRef.Presence != SnapshotArtifactAbsent || v.Meta.Revision != 1 {
			return errors.New("initial snapshot artifact reservation entry has a wrong fact, predecessor, or revision")
		}
	case SnapshotArtifactEntryArtifact:
		if v.FactRef.TypeURL != SnapshotArtifactFactTypeURL || v.FactRef.DigestDomain != SnapshotArtifactFactDomain || v.PreviousEntryRef.Presence != SnapshotArtifactPresent || v.PreviousEntryRef.Ref == nil || v.PreviousEntryRef.Ref.TypeURL != SnapshotArtifactEntryTypeURL || v.PreviousEntryRef.Ref.DigestDomain != SnapshotArtifactEntryDomain || v.Meta.Revision != 2 {
			return errors.New("snapshot artifact available entry has a wrong fact, predecessor, or revision")
		}
	default:
		return errors.New("snapshot artifact entry kind is unsupported")
	}
	return nil
}

func (v SnapshotArtifactAggregateEntryV2) ExactRef() SnapshotArtifactExactRefV2 {
	return SnapshotArtifactExactRefV2{TypeURL: SnapshotArtifactEntryTypeURL, Version: SnapshotArtifactVersion, ID: v.Meta.ID, Revision: v.Meta.Revision, DigestAlgorithm: SnapshotArtifactDigestSHA256, DigestDomain: SnapshotArtifactEntryDomain, Digest: v.Meta.Digest, ExpiresUnixNano: v.Meta.ExpiresUnixNano}
}

type SnapshotArtifactAggregateRefV2 struct {
	TypeURL         string `json:"type_url"`
	Version         uint32 `json:"version"`
	Owner           string `json:"owner"`
	Kind            string `json:"kind"`
	AggregateID     string `json:"aggregate_id"`
	Revision        uint64 `json:"revision"`
	TenantID        string `json:"tenant_id"`
	DataDomain      string `json:"data_domain"`
	SchemaRef       Ref    `json:"schema_ref"`
	DigestAlgorithm string `json:"digest_algorithm"`
	DigestDomain    string `json:"digest_domain"`
	Digest          string `json:"digest"`
	ExpiresUnixNano int64  `json:"expires_unix_nano"`
}

func (r SnapshotArtifactAggregateRefV2) ValidateShape() error {
	if r.TypeURL != SnapshotArtifactAggregateRefTypeURL || r.Version != SnapshotArtifactVersion || r.Owner != SnapshotArtifactOwnerV2 || r.Kind != SnapshotArtifactAggregateKind || r.DigestAlgorithm != SnapshotArtifactDigestSHA256 || r.DigestDomain != SnapshotArtifactAggregateRefDomain || strings.TrimSpace(r.AggregateID) == "" || r.Revision == 0 || strings.TrimSpace(r.TenantID) == "" || strings.TrimSpace(r.DataDomain) == "" || !ValidDigest(r.Digest) || r.ExpiresUnixNano <= 0 {
		return errors.New("snapshot artifact aggregate ref contract is invalid")
	}
	return r.SchemaRef.ValidateShape("snapshot artifact aggregate schema")
}

func (r SnapshotArtifactAggregateRefV2) ExactRef() SnapshotArtifactExactRefV2 {
	return SnapshotArtifactExactRefV2{TypeURL: r.TypeURL, Version: r.Version, ID: r.AggregateID, Revision: r.Revision, DigestAlgorithm: r.DigestAlgorithm, DigestDomain: r.DigestDomain, Digest: r.Digest, ExpiresUnixNano: r.ExpiresUnixNano}
}

func SameSnapshotArtifactAggregateRef(a, b SnapshotArtifactAggregateRefV2) bool {
	return a == b
}

type SnapshotArtifactAggregateState string

const (
	SnapshotArtifactAggregateReserved           SnapshotArtifactAggregateState = "reserved"
	SnapshotArtifactAggregateAvailable          SnapshotArtifactAggregateState = "available"
	SnapshotArtifactAggregateDeletionInProgress SnapshotArtifactAggregateState = "deletion_in_progress"
	SnapshotArtifactAggregateDeleted            SnapshotArtifactAggregateState = "deleted"
	SnapshotArtifactAggregateIndeterminate      SnapshotArtifactAggregateState = "indeterminate"
)

func (s SnapshotArtifactAggregateState) Validate() error {
	switch s {
	case SnapshotArtifactAggregateReserved, SnapshotArtifactAggregateAvailable, SnapshotArtifactAggregateDeletionInProgress, SnapshotArtifactAggregateDeleted, SnapshotArtifactAggregateIndeterminate:
		return nil
	default:
		return errors.New("snapshot artifact aggregate state is invalid")
	}
}

type SnapshotArtifactAggregateEnvelopeV2 struct {
	AggregateRef                     SnapshotArtifactAggregateRefV2         `json:"aggregate_ref"`
	RequestedNotAfter                int64                                  `json:"requested_not_after"`
	PreviousAggregateRef             SnapshotArtifactOptionalAggregateRefV2 `json:"previous_aggregate_ref"`
	AppliedEntryRef                  SnapshotArtifactExactRefV2             `json:"applied_entry_ref"`
	ReservationFactRef               SnapshotArtifactExactRefV2             `json:"reservation_fact_ref"`
	ArtifactFactRef                  SnapshotArtifactOptionalExactRefV2     `json:"artifact_fact_ref"`
	RetentionApplicationFactRef      SnapshotArtifactOptionalExactRefV2     `json:"retention_application_fact_ref"`
	ActiveDeletionAttemptFactRef     SnapshotArtifactOptionalExactRefV2     `json:"active_deletion_attempt_fact_ref"`
	LastClosedDeletionAttemptFactRef SnapshotArtifactOptionalExactRefV2     `json:"last_closed_deletion_attempt_fact_ref"`
	TerminalTombstoneRef             SnapshotArtifactOptionalExactRefV2     `json:"terminal_tombstone_ref"`
	AggregateState                   SnapshotArtifactAggregateState         `json:"aggregate_state"`
}

func SealSnapshotArtifactAggregateEnvelopeV2(value SnapshotArtifactAggregateEnvelopeV2) (SnapshotArtifactAggregateEnvelopeV2, error) {
	value.AggregateRef.TypeURL = SnapshotArtifactAggregateRefTypeURL
	value.AggregateRef.Version = SnapshotArtifactVersion
	value.AggregateRef.Owner = SnapshotArtifactOwnerV2
	value.AggregateRef.Kind = SnapshotArtifactAggregateKind
	value.AggregateRef.DigestAlgorithm = SnapshotArtifactDigestSHA256
	value.AggregateRef.DigestDomain = SnapshotArtifactAggregateRefDomain
	value.AggregateRef.Digest = ""
	if err := validateSnapshotArtifactAggregateEnvelopeBody(value); err != nil {
		return SnapshotArtifactAggregateEnvelopeV2{}, err
	}
	digest, err := Digest(SnapshotArtifactAggregateRefDomain, value)
	if err != nil {
		return SnapshotArtifactAggregateEnvelopeV2{}, err
	}
	value.AggregateRef.Digest = digest
	return value, value.ValidateShape()
}

func (v SnapshotArtifactAggregateEnvelopeV2) ValidateShape() error {
	if err := v.AggregateRef.ValidateShape(); err != nil {
		return err
	}
	if err := validateSnapshotArtifactAggregateEnvelopeBody(v); err != nil {
		return err
	}
	canonical := v
	canonical.AggregateRef.Digest = ""
	digest, err := Digest(SnapshotArtifactAggregateRefDomain, canonical)
	if err != nil {
		return err
	}
	if digest != v.AggregateRef.Digest {
		return errors.New("snapshot artifact aggregate digest mismatch")
	}
	return nil
}

func validateSnapshotArtifactAggregateEnvelopeBody(v SnapshotArtifactAggregateEnvelopeV2) error {
	if v.AggregateRef.ExpiresUnixNano <= 0 || v.RequestedNotAfter <= 0 || v.AggregateRef.ExpiresUnixNano > v.RequestedNotAfter {
		return errors.New("snapshot artifact aggregate envelope TTL is invalid")
	}
	if err := v.PreviousAggregateRef.ValidateShape("snapshot artifact previous aggregate"); err != nil {
		return err
	}
	if err := v.AppliedEntryRef.ValidateShape("snapshot artifact applied entry"); err != nil {
		return err
	}
	if v.AppliedEntryRef.TypeURL != SnapshotArtifactEntryTypeURL || v.AppliedEntryRef.DigestDomain != SnapshotArtifactEntryDomain {
		return errors.New("snapshot artifact aggregate has wrong entry ref type")
	}
	if err := v.ReservationFactRef.ValidateShape("snapshot artifact aggregate reservation fact"); err != nil {
		return err
	}
	if v.ReservationFactRef.TypeURL != SnapshotArtifactReservationFactTypeURL || v.ReservationFactRef.DigestDomain != SnapshotArtifactReservationFactDomain {
		return errors.New("snapshot artifact aggregate has wrong reservation fact ref type")
	}
	for name, optional := range map[string]SnapshotArtifactOptionalExactRefV2{
		"artifact fact":                v.ArtifactFactRef,
		"retention application":        v.RetentionApplicationFactRef,
		"active deletion attempt":      v.ActiveDeletionAttemptFactRef,
		"last closed deletion attempt": v.LastClosedDeletionAttemptFactRef,
		"terminal tombstone":           v.TerminalTombstoneRef,
	} {
		if err := optional.ValidateShape("snapshot artifact aggregate " + name); err != nil {
			return err
		}
	}
	if err := v.AggregateState.Validate(); err != nil {
		return err
	}
	switch v.AggregateState {
	case SnapshotArtifactAggregateReserved:
		if v.AggregateRef.Revision != 1 || v.PreviousAggregateRef.Presence != SnapshotArtifactAbsent || v.ArtifactFactRef.Presence != SnapshotArtifactAbsent || v.RetentionApplicationFactRef.Presence != SnapshotArtifactAbsent || v.ActiveDeletionAttemptFactRef.Presence != SnapshotArtifactAbsent || v.LastClosedDeletionAttemptFactRef.Presence != SnapshotArtifactAbsent || v.TerminalTombstoneRef.Presence != SnapshotArtifactAbsent {
			return errors.New("reserved snapshot artifact aggregate carries a successor fact")
		}
	case SnapshotArtifactAggregateAvailable:
		if v.AggregateRef.Revision != 2 || v.PreviousAggregateRef.Presence != SnapshotArtifactPresent || v.PreviousAggregateRef.Ref == nil || v.PreviousAggregateRef.Ref.AggregateID != v.AggregateRef.AggregateID || v.PreviousAggregateRef.Ref.Revision+1 != v.AggregateRef.Revision || v.ArtifactFactRef.Presence != SnapshotArtifactPresent || v.ArtifactFactRef.Ref == nil || v.ArtifactFactRef.Ref.TypeURL != SnapshotArtifactFactTypeURL || v.ArtifactFactRef.Ref.DigestDomain != SnapshotArtifactFactDomain || v.RetentionApplicationFactRef.Presence != SnapshotArtifactAbsent || v.ActiveDeletionAttemptFactRef.Presence != SnapshotArtifactAbsent || v.LastClosedDeletionAttemptFactRef.Presence != SnapshotArtifactAbsent || v.TerminalTombstoneRef.Presence != SnapshotArtifactAbsent {
			return errors.New("available snapshot artifact aggregate has an invalid predecessor or fact presence")
		}
	default:
		return errors.New("snapshot artifact aggregate state is unsupported in the capture slice")
	}
	return nil
}

type SnapshotArtifactAggregateCurrentIndexV2 struct {
	CurrentIndexRef                SnapshotArtifactExactRefV2         `json:"current_index_ref"`
	ArtifactAggregateID            string                             `json:"artifact_aggregate_id"`
	ArtifactSubjectRef             SnapshotArtifactSubjectRefV2       `json:"artifact_subject_ref"`
	HeadAggregateEnvelopeRef       SnapshotArtifactAggregateRefV2     `json:"head_aggregate_envelope_ref"`
	AggregateState                 SnapshotArtifactAggregateState     `json:"aggregate_state"`
	ReservationCurrentRef          SnapshotArtifactOptionalExactRefV2 `json:"reservation_current_ref"`
	ArtifactFactRef                SnapshotArtifactOptionalExactRefV2 `json:"artifact_fact_ref"`
	RetentionApplicationCurrentRef SnapshotArtifactOptionalExactRefV2 `json:"retention_application_current_ref"`
	ActiveDeletionAttemptFactRef   SnapshotArtifactOptionalExactRefV2 `json:"active_deletion_attempt_fact_ref"`
	TerminalTombstoneRef           SnapshotArtifactOptionalExactRefV2 `json:"terminal_tombstone_ref"`
	ActiveTTLClosureDigest         string                             `json:"active_ttl_closure_digest"`
	OwnerClockWatermark            int64                              `json:"owner_clock_watermark"`
	CheckedUnixNano                int64                              `json:"checked_unix_nano"`
	RequestedNotAfter              int64                              `json:"requested_not_after"`
}

func SealSnapshotArtifactAggregateCurrentIndexV2(value SnapshotArtifactAggregateCurrentIndexV2) (SnapshotArtifactAggregateCurrentIndexV2, error) {
	value.CurrentIndexRef.TypeURL = SnapshotArtifactCurrentIndexTypeURL
	value.CurrentIndexRef.Version = SnapshotArtifactVersion
	value.CurrentIndexRef.DigestAlgorithm = SnapshotArtifactDigestSHA256
	value.CurrentIndexRef.DigestDomain = SnapshotArtifactCurrentIndexDomain
	value.CurrentIndexRef.Digest = ""
	if err := validateSnapshotArtifactAggregateCurrentIndexBody(value); err != nil {
		return SnapshotArtifactAggregateCurrentIndexV2{}, err
	}
	digest, err := Digest(SnapshotArtifactCurrentIndexDomain, value)
	if err != nil {
		return SnapshotArtifactAggregateCurrentIndexV2{}, err
	}
	value.CurrentIndexRef.Digest = digest
	return value, value.ValidateShape()
}

func (v SnapshotArtifactAggregateCurrentIndexV2) ValidateShape() error {
	if err := v.CurrentIndexRef.ValidateShape("snapshot artifact current index"); err != nil {
		return err
	}
	if v.CurrentIndexRef.TypeURL != SnapshotArtifactCurrentIndexTypeURL || v.CurrentIndexRef.DigestDomain != SnapshotArtifactCurrentIndexDomain {
		return errors.New("snapshot artifact current index has wrong type")
	}
	if err := validateSnapshotArtifactAggregateCurrentIndexBody(v); err != nil {
		return err
	}
	canonical := v
	canonical.CurrentIndexRef.Digest = ""
	digest, err := Digest(SnapshotArtifactCurrentIndexDomain, canonical)
	if err != nil {
		return err
	}
	if digest != v.CurrentIndexRef.Digest {
		return errors.New("snapshot artifact current index digest mismatch")
	}
	return nil
}

func (v SnapshotArtifactAggregateCurrentIndexV2) ValidateCurrent(now time.Time) error {
	if err := v.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < v.CheckedUnixNano || now.UnixNano() < v.OwnerClockWatermark {
		return errors.New("snapshot artifact current index was checked in the future")
	}
	return v.CurrentIndexRef.ValidateCurrent("snapshot artifact current index", now)
}

func validateSnapshotArtifactAggregateCurrentIndexBody(v SnapshotArtifactAggregateCurrentIndexV2) error {
	if strings.TrimSpace(v.CurrentIndexRef.ID) == "" || v.CurrentIndexRef.Revision == 0 || v.CurrentIndexRef.ExpiresUnixNano <= 0 || strings.TrimSpace(v.ArtifactAggregateID) == "" || v.OwnerClockWatermark <= 0 || v.CheckedUnixNano <= 0 || v.RequestedNotAfter <= 0 || v.CheckedUnixNano < v.OwnerClockWatermark || v.CurrentIndexRef.ExpiresUnixNano > v.RequestedNotAfter || !ValidDigest(v.ActiveTTLClosureDigest) {
		return errors.New("snapshot artifact current index body is incomplete")
	}
	if err := v.ArtifactSubjectRef.ValidateShape(); err != nil {
		return err
	}
	if err := v.HeadAggregateEnvelopeRef.ValidateShape(); err != nil {
		return err
	}
	if v.ArtifactAggregateID != v.ArtifactSubjectRef.ArtifactAggregateID || v.ArtifactAggregateID != v.HeadAggregateEnvelopeRef.AggregateID {
		return errors.New("snapshot artifact current index aggregate identity mismatch")
	}
	if v.ArtifactSubjectRef.TenantID != v.HeadAggregateEnvelopeRef.TenantID || v.ArtifactSubjectRef.DataDomain != v.HeadAggregateEnvelopeRef.DataDomain {
		return errors.New("snapshot artifact current index tenant or domain mismatch")
	}
	if err := v.AggregateState.Validate(); err != nil {
		return err
	}
	for name, optional := range map[string]SnapshotArtifactOptionalExactRefV2{
		"reservation":     v.ReservationCurrentRef,
		"artifact":        v.ArtifactFactRef,
		"retention":       v.RetentionApplicationCurrentRef,
		"active deletion": v.ActiveDeletionAttemptFactRef,
		"terminal":        v.TerminalTombstoneRef,
	} {
		if err := optional.ValidateShape("snapshot artifact current index " + name); err != nil {
			return err
		}
	}
	if v.RetentionApplicationCurrentRef.Presence != SnapshotArtifactAbsent || v.ActiveDeletionAttemptFactRef.Presence != SnapshotArtifactAbsent || v.TerminalTombstoneRef.Presence != SnapshotArtifactAbsent {
		return errors.New("snapshot artifact capture current index carries unsupported successor state")
	}
	switch v.AggregateState {
	case SnapshotArtifactAggregateReserved:
		if v.ReservationCurrentRef.Presence != SnapshotArtifactPresent || v.ArtifactFactRef.Presence != SnapshotArtifactAbsent || v.ReservationCurrentRef.Ref == nil || v.ReservationCurrentRef.Ref.TypeURL != SnapshotArtifactReservationFactTypeURL || v.ReservationCurrentRef.Ref.DigestDomain != SnapshotArtifactReservationFactDomain || v.CurrentIndexRef.ExpiresUnixNano > v.ReservationCurrentRef.Ref.ExpiresUnixNano || v.CurrentIndexRef.ExpiresUnixNano > v.ArtifactSubjectRef.ExpiresUnixNano {
			return errors.New("reserved snapshot artifact current index extends active TTL or carries the wrong reservation type")
		}
	case SnapshotArtifactAggregateAvailable:
		if v.ReservationCurrentRef.Presence != SnapshotArtifactAbsent || v.ArtifactFactRef.Presence != SnapshotArtifactPresent || v.ArtifactFactRef.Ref == nil || v.ArtifactFactRef.Ref.TypeURL != SnapshotArtifactFactTypeURL || v.ArtifactFactRef.Ref.DigestDomain != SnapshotArtifactFactDomain || v.HeadAggregateEnvelopeRef.Revision != 2 || v.CurrentIndexRef.Revision != 2 || v.CurrentIndexRef.ExpiresUnixNano > v.ArtifactFactRef.Ref.ExpiresUnixNano || v.CurrentIndexRef.ExpiresUnixNano > v.ArtifactSubjectRef.ExpiresUnixNano || v.CurrentIndexRef.ExpiresUnixNano > v.HeadAggregateEnvelopeRef.ExpiresUnixNano {
			return errors.New("available snapshot artifact current index has invalid presence or TTL")
		}
	default:
		return errors.New("snapshot artifact current state is unsupported in the capture slice")
	}
	return nil
}

type SnapshotArtifactAggregateCurrentProjectionV2 struct {
	AggregateCurrentIndexRef SnapshotArtifactExactRefV2         `json:"aggregate_current_index_ref"`
	HeadAggregateEnvelopeRef SnapshotArtifactAggregateRefV2     `json:"head_aggregate_envelope_ref"`
	ArtifactSubjectRef       SnapshotArtifactSubjectRefV2       `json:"artifact_subject_ref"`
	ReservationFactRef       SnapshotArtifactExactRefV2         `json:"reservation_fact_ref"`
	ArtifactFactRef          SnapshotArtifactOptionalExactRefV2 `json:"artifact_fact_ref"`
	AggregateState           SnapshotArtifactAggregateState     `json:"aggregate_state"`
	ActiveTTLClosureDigest   string                             `json:"active_ttl_closure_digest"`
	ProjectionDigest         string                             `json:"projection_digest"`
	OwnerComputedCurrent     bool                               `json:"owner_computed_current"`
	CheckedUnixNano          int64                              `json:"checked_unix_nano"`
	RequestedNotAfter        int64                              `json:"requested_not_after"`
	ExpiresUnixNano          int64                              `json:"expires_unix_nano"`
}

func SealSnapshotArtifactAggregateCurrentProjectionV2(value SnapshotArtifactAggregateCurrentProjectionV2) (SnapshotArtifactAggregateCurrentProjectionV2, error) {
	if value.ArtifactFactRef.Presence == "" && value.ArtifactFactRef.Ref == nil {
		value.ArtifactFactRef.Presence = SnapshotArtifactAbsent
	}
	value.ProjectionDigest = ""
	if err := validateSnapshotArtifactCurrentProjectionBody(value); err != nil {
		return SnapshotArtifactAggregateCurrentProjectionV2{}, err
	}
	digest, err := Digest(SnapshotArtifactCurrentProjectionDomain, value)
	if err != nil {
		return SnapshotArtifactAggregateCurrentProjectionV2{}, err
	}
	value.ProjectionDigest = digest
	return value, value.ValidateShape()
}

func (v SnapshotArtifactAggregateCurrentProjectionV2) ValidateShape() error {
	if err := validateSnapshotArtifactCurrentProjectionBody(v); err != nil {
		return err
	}
	if !ValidDigest(v.ProjectionDigest) {
		return errors.New("snapshot artifact current projection digest is invalid")
	}
	canonical := v
	canonical.ProjectionDigest = ""
	digest, err := Digest(SnapshotArtifactCurrentProjectionDomain, canonical)
	if err != nil {
		return err
	}
	if digest != v.ProjectionDigest {
		return errors.New("snapshot artifact current projection digest mismatch")
	}
	return nil
}

func (v SnapshotArtifactAggregateCurrentProjectionV2) ValidateCurrent(now time.Time) error {
	if err := v.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < v.CheckedUnixNano {
		return errors.New("snapshot artifact current projection was checked in the future")
	}
	if now.UnixNano() >= v.ExpiresUnixNano {
		return errors.New("snapshot artifact current projection is expired")
	}
	return nil
}

func validateSnapshotArtifactCurrentProjectionBody(v SnapshotArtifactAggregateCurrentProjectionV2) error {
	if err := v.AggregateCurrentIndexRef.ValidateShape("snapshot artifact projection current index"); err != nil {
		return err
	}
	if v.AggregateCurrentIndexRef.TypeURL != SnapshotArtifactCurrentIndexTypeURL || v.AggregateCurrentIndexRef.DigestDomain != SnapshotArtifactCurrentIndexDomain {
		return errors.New("snapshot artifact projection has wrong current index ref type")
	}
	if err := v.HeadAggregateEnvelopeRef.ValidateShape(); err != nil {
		return err
	}
	if err := v.ArtifactSubjectRef.ValidateShape(); err != nil {
		return err
	}
	if err := v.ReservationFactRef.ValidateShape("snapshot artifact projection reservation fact"); err != nil {
		return err
	}
	if v.ReservationFactRef.TypeURL != SnapshotArtifactReservationFactTypeURL || v.ReservationFactRef.DigestDomain != SnapshotArtifactReservationFactDomain {
		return errors.New("snapshot artifact projection has wrong reservation fact ref type")
	}
	if err := v.AggregateState.Validate(); err != nil {
		return err
	}
	if err := v.ArtifactFactRef.ValidateShape("snapshot artifact projection artifact fact"); err != nil {
		return err
	}
	if !v.OwnerComputedCurrent || !ValidDigest(v.ActiveTTLClosureDigest) || v.CheckedUnixNano <= 0 || v.RequestedNotAfter <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano || v.ExpiresUnixNano > v.RequestedNotAfter || v.ExpiresUnixNano > v.AggregateCurrentIndexRef.ExpiresUnixNano {
		return errors.New("snapshot artifact current projection is invalid")
	}
	switch v.AggregateState {
	case SnapshotArtifactAggregateReserved:
		if v.ArtifactFactRef.Presence != SnapshotArtifactAbsent || v.ExpiresUnixNano > v.ReservationFactRef.ExpiresUnixNano {
			return errors.New("reserved snapshot artifact projection has invalid fact presence or TTL")
		}
	case SnapshotArtifactAggregateAvailable:
		if v.ArtifactFactRef.Presence != SnapshotArtifactPresent || v.ArtifactFactRef.Ref == nil || v.ArtifactFactRef.Ref.TypeURL != SnapshotArtifactFactTypeURL || v.ArtifactFactRef.Ref.DigestDomain != SnapshotArtifactFactDomain || v.ExpiresUnixNano > v.ArtifactFactRef.Ref.ExpiresUnixNano {
			return errors.New("available snapshot artifact projection has invalid fact presence or TTL")
		}
	default:
		return errors.New("snapshot artifact projection state is unsupported in the capture slice")
	}
	if v.HeadAggregateEnvelopeRef.AggregateID != v.ArtifactSubjectRef.ArtifactAggregateID {
		return errors.New("snapshot artifact current projection aggregate mismatch")
	}
	return nil
}

type SnapshotArtifactReservedBundleV2 struct {
	StableKey           SnapshotArtifactStableSourceKeyV2       `json:"stable_key"`
	Reservation         SnapshotArtifactReservationV2           `json:"reservation"`
	ReservationFact     SnapshotArtifactReservationFactV2       `json:"reservation_fact"`
	Entry               SnapshotArtifactAggregateEntryV2        `json:"entry"`
	Envelope            SnapshotArtifactAggregateEnvelopeV2     `json:"envelope"`
	CurrentIndex        SnapshotArtifactAggregateCurrentIndexV2 `json:"current_index"`
	OwnerClockWatermark int64                                   `json:"owner_clock_watermark"`
}

func (b SnapshotArtifactReservedBundleV2) ValidateShape() error {
	if err := b.StableKey.ValidateShape(); err != nil {
		return err
	}
	if err := b.Reservation.ValidateShape(); err != nil {
		return err
	}
	if err := b.ReservationFact.ValidateShape(); err != nil {
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
	if b.OwnerClockWatermark <= 0 || b.OwnerClockWatermark != b.CurrentIndex.OwnerClockWatermark || b.StableKey != b.Reservation.StableSourceKey() || !SameSnapshotArtifactExactRef(b.Reservation.ExactRef(), b.ReservationFact.ReservationRef) || !SameSnapshotArtifactExactRef(b.ReservationFact.ExactRef(), b.Entry.FactRef) || !SameSnapshotArtifactExactRef(b.Entry.ExactRef(), b.Envelope.AppliedEntryRef) || !SameSnapshotArtifactExactRef(b.ReservationFact.ExactRef(), b.Envelope.ReservationFactRef) || !SameSnapshotArtifactAggregateRef(b.Envelope.AggregateRef, b.CurrentIndex.HeadAggregateEnvelopeRef) || b.CurrentIndex.ReservationCurrentRef.Ref == nil || !SameSnapshotArtifactExactRef(b.ReservationFact.ExactRef(), *b.CurrentIndex.ReservationCurrentRef.Ref) {
		return errors.New("snapshot artifact reserved bundle binding mismatch")
	}
	return nil
}

func (v SnapshotArtifactReservationV2) StableSourceKey() SnapshotArtifactStableSourceKeyV2 {
	return SnapshotArtifactStableSourceKeyV2{TenantID: v.TenantID, DataDomain: v.DataDomain, SourceAttemptID: v.SourceAttemptRef.ID}
}

type ReserveArtifactResultV2 struct {
	Reservation  SnapshotArtifactReservationV2           `json:"reservation"`
	CurrentIndex SnapshotArtifactAggregateCurrentIndexV2 `json:"current_index"`
	Created      bool                                    `json:"created"`
}

type InspectSnapshotArtifactReservationRequestV2 struct {
	ExpectedRef SnapshotArtifactExactRefV2 `json:"expected_ref"`
}

func (r InspectSnapshotArtifactReservationRequestV2) ValidateShape() error {
	if err := r.ExpectedRef.ValidateShape("snapshot artifact reservation inspect"); err != nil {
		return err
	}
	if r.ExpectedRef.TypeURL != SnapshotArtifactReservationTypeURL || r.ExpectedRef.DigestDomain != SnapshotArtifactReservationDomain {
		return errors.New("snapshot artifact reservation inspect has wrong ref type")
	}
	return nil
}

type InspectSnapshotArtifactReservationByStableKeyRequestV2 struct {
	StableKey SnapshotArtifactStableSourceKeyV2 `json:"stable_key"`
}

func (r InspectSnapshotArtifactReservationByStableKeyRequestV2) ValidateShape() error {
	return r.StableKey.ValidateShape()
}

type InspectSnapshotArtifactAggregateHistoricalRequestV2 struct {
	ExpectedRef SnapshotArtifactAggregateRefV2 `json:"expected_ref"`
}

func (r InspectSnapshotArtifactAggregateHistoricalRequestV2) ValidateShape() error {
	return r.ExpectedRef.ValidateShape()
}

type InspectSnapshotArtifactEntryHistoricalRequestV2 struct {
	ExpectedRef SnapshotArtifactExactRefV2 `json:"expected_ref"`
}

func (r InspectSnapshotArtifactEntryHistoricalRequestV2) ValidateShape() error {
	if err := r.ExpectedRef.ValidateShape("snapshot artifact entry inspect"); err != nil {
		return err
	}
	if r.ExpectedRef.TypeURL != SnapshotArtifactEntryTypeURL || r.ExpectedRef.DigestDomain != SnapshotArtifactEntryDomain {
		return errors.New("snapshot artifact entry inspect has wrong ref type")
	}
	return nil
}

type InspectSnapshotArtifactAggregateCurrentRequestV2 struct {
	ArtifactAggregateID  string                                 `json:"artifact_aggregate_id"`
	ExpectedAggregateRef SnapshotArtifactOptionalAggregateRefV2 `json:"expected_aggregate_ref"`
	RequestedNotAfter    int64                                  `json:"requested_not_after"`
}

func (r InspectSnapshotArtifactAggregateCurrentRequestV2) ValidateShape() error {
	if strings.TrimSpace(r.ArtifactAggregateID) == "" || r.RequestedNotAfter <= 0 {
		return errors.New("snapshot artifact current inspect coordinates are incomplete")
	}
	return r.ExpectedAggregateRef.ValidateShape("snapshot artifact current expected aggregate")
}

func (r InspectSnapshotArtifactAggregateCurrentRequestV2) ValidateCurrent(now time.Time) error {
	if err := r.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() >= r.RequestedNotAfter {
		return errors.New("snapshot artifact current inspect request is expired")
	}
	return nil
}

func cloneSnapshotArtifact[T any](value T) (T, error) {
	var zero T
	payload, err := json.Marshal(value)
	if err != nil {
		return zero, err
	}
	var cloned T
	if err := json.Unmarshal(payload, &cloned); err != nil {
		return zero, err
	}
	return cloned, nil
}

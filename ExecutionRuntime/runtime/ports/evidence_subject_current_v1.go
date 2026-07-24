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
	EvidenceSubjectCurrentContractVersionV1 = "1.0.0"
	EvidenceSubjectCurrentCanonicalDomainV1 = "praxis.runtime.evidence-subject-current"
	EvidenceSubjectReaderCapabilityV1       = CapabilityNameV2("praxis.runtime/read-evidence-subject-current")
	evidenceSubjectNoCurrentSentinelV1      = "no-current-v1"
)

type EvidenceSubjectKeyV1 struct {
	Record EvidenceRecordRefV2 `json:"record"`
	Source EvidenceSourceKeyV2 `json:"source"`
}

func (k EvidenceSubjectKeyV1) Validate() error {
	if err := k.Record.Validate(); err != nil {
		return err
	}
	return k.Source.Validate()
}

func DigestEvidenceSubjectKeyV1(k EvidenceSubjectKeyV1) (core.Digest, error) {
	if err := k.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectKeyV1", k)
}

type EvidenceSubjectProjectionIDInputV1 struct {
	SubjectKeyDigest core.Digest `json:"subject_key_digest"`
}

type EvidenceSubjectCurrentIndexIDInputV1 struct {
	SubjectKeyDigest core.Digest `json:"subject_key_digest"`
}

func DeriveEvidenceSubjectProjectionIDV1(subject core.Digest) (string, error) {
	if err := subject.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectProjectionIDV1", EvidenceSubjectProjectionIDInputV1{SubjectKeyDigest: subject})
	return string(digest), err
}

func DeriveEvidenceSubjectCurrentIndexIDV1(subject core.Digest) (string, error) {
	if err := subject.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectCurrentIndexIDV1", EvidenceSubjectCurrentIndexIDInputV1{SubjectKeyDigest: subject})
	return string(digest), err
}

type EvidenceSubjectProjectionRefV1 struct {
	ProjectionID     string        `json:"projection_id"`
	Revision         core.Revision `json:"revision"`
	SubjectKeyDigest core.Digest   `json:"subject_key_digest"`
	OwnerWatermark   core.Revision `json:"owner_watermark"`
	Digest           core.Digest   `json:"digest"`
}

func (r EvidenceSubjectProjectionRefV1) Validate() error {
	if strings.TrimSpace(r.ProjectionID) == "" || r.Revision == 0 || r.OwnerWatermark == 0 {
		return evidenceSubjectInvalidV1("Evidence subject Projection ref is incomplete")
	}
	if err := r.SubjectKeyDigest.Validate(); err != nil {
		return err
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	expected, err := DeriveEvidenceSubjectProjectionIDV1(r.SubjectKeyDigest)
	if err != nil {
		return err
	}
	if expected != r.ProjectionID {
		return evidenceSubjectConflictV1("Evidence subject Projection ID drifted")
	}
	return nil
}

type EvidenceSourceRegistrationRefV1 struct {
	RegistrationID      string           `json:"registration_id"`
	Revision            core.Revision    `json:"revision"`
	FactDigest          core.Digest      `json:"fact_digest"`
	ConfigurationDigest core.Digest      `json:"configuration_digest"`
	SourceID            NamespacedNameV2 `json:"source_id"`
	SourceEpoch         core.Epoch       `json:"source_epoch"`
}

func (r EvidenceSourceRegistrationRefV1) Validate() error {
	if strings.TrimSpace(r.RegistrationID) == "" || r.Revision == 0 || r.SourceEpoch == 0 {
		return evidenceSubjectInvalidV1("Evidence source Registration ref is incomplete")
	}
	if err := r.FactDigest.Validate(); err != nil {
		return err
	}
	if err := r.ConfigurationDigest.Validate(); err != nil {
		return err
	}
	return ValidateNamespacedNameV2(r.SourceID)
}

type EvidenceSubjectReaderCapabilityRefV1 struct {
	Name                           CapabilityNameV2 `json:"name"`
	BindingRevision                core.Revision    `json:"binding_revision"`
	GrantDigest                    core.Digest      `json:"grant_digest"`
	BindingCurrentProjectionDigest core.Digest      `json:"binding_current_projection_digest"`
	IssuedUnixNano                 int64            `json:"issued_unix_nano"`
	ExpiresUnixNano                int64            `json:"expires_unix_nano"`
}

func (r EvidenceSubjectReaderCapabilityRefV1) Validate() error {
	if r.Name != EvidenceSubjectReaderCapabilityV1 || r.BindingRevision == 0 || r.IssuedUnixNano <= 0 || r.ExpiresUnixNano <= r.IssuedUnixNano {
		return evidenceSubjectInvalidV1("Evidence subject Reader capability is incomplete")
	}
	if err := r.GrantDigest.Validate(); err != nil {
		return err
	}
	return r.BindingCurrentProjectionDigest.Validate()
}

type EvidenceSubjectReaderBindingRefV1 struct {
	Binding                  ProviderBindingRefV2                 `json:"binding"`
	BindingSetDigest         core.Digest                          `json:"binding_set_digest"`
	BindingSetSemanticDigest core.Digest                          `json:"binding_set_semantic_digest"`
	BindingID                string                               `json:"binding_id"`
	Capability               EvidenceSubjectReaderCapabilityRefV1 `json:"capability"`
}

func (r EvidenceSubjectReaderBindingRefV1) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if r.Binding.Capability != EvidenceSubjectReaderCapabilityV1 || r.Binding.Capability != r.Capability.Name || strings.TrimSpace(r.BindingID) == "" {
		return core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "Evidence subject Reader binding lacks the closed capability")
	}
	if err := r.BindingSetDigest.Validate(); err != nil {
		return err
	}
	if err := r.BindingSetSemanticDigest.Validate(); err != nil {
		return err
	}
	return r.Capability.Validate()
}

func EvidenceSubjectReaderBindingFromCurrentV1(p ProviderBindingCurrentProjectionV2) (EvidenceSubjectReaderBindingRefV1, error) {
	if p.Ref.Capability != EvidenceSubjectReaderCapabilityV1 {
		return EvidenceSubjectReaderBindingRefV1{}, core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "Binding does not grant Evidence subject current reads")
	}
	r := EvidenceSubjectReaderBindingRefV1{
		Binding:                  p.Ref,
		BindingSetDigest:         p.BindingSetDigest,
		BindingSetSemanticDigest: p.BindingSetSemanticDigest,
		BindingID:                p.BindingID,
		Capability: EvidenceSubjectReaderCapabilityRefV1{
			Name: p.Ref.Capability, BindingRevision: p.BindingRevision, GrantDigest: p.GrantDigest,
			BindingCurrentProjectionDigest: p.ProjectionDigest, IssuedUnixNano: p.IssuedUnixNano, ExpiresUnixNano: p.ExpiresUnixNano,
		},
	}
	return r, r.Validate()
}

type EvidenceSubjectConsumerAssociationIDInputV1 struct {
	Principal            core.OwnerRef    `json:"principal"`
	ConsumerComponentID  ComponentIDV2    `json:"consumer_component_id"`
	ConsumerCapability   CapabilityNameV2 `json:"consumer_capability"`
	ExecutionScopeDigest core.Digest      `json:"execution_scope_digest"`
}

type EvidenceSubjectConsumerAssociationRefV1 struct {
	AssociationID        string               `json:"association_id"`
	Revision             core.Revision        `json:"revision"`
	Principal            core.OwnerRef        `json:"principal"`
	Consumer             ProviderBindingRefV2 `json:"consumer"`
	ExecutionScopeDigest core.Digest          `json:"execution_scope_digest"`
	Digest               core.Digest          `json:"digest"`
}

func DeriveEvidenceSubjectConsumerAssociationIDV1(input EvidenceSubjectConsumerAssociationIDInputV1) (string, error) {
	if err := input.Principal.Validate(); err != nil {
		return "", err
	}
	if ValidateNamespacedNameV2(NamespacedNameV2(input.ConsumerComponentID)) != nil || ValidateNamespacedNameV2(NamespacedNameV2(input.ConsumerCapability)) != nil || input.ExecutionScopeDigest.Validate() != nil {
		return "", evidenceSubjectInvalidV1("Evidence subject Consumer association identity is incomplete")
	}
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectConsumerAssociationIDV1", input)
	return string(digest), err
}

func (r EvidenceSubjectConsumerAssociationRefV1) Validate() error {
	if r.Revision == 0 || r.Consumer.Validate() != nil || r.ExecutionScopeDigest.Validate() != nil || r.Digest.Validate() != nil {
		return evidenceSubjectInvalidV1("Evidence subject Consumer association ref is incomplete")
	}
	input := EvidenceSubjectConsumerAssociationIDInputV1{Principal: r.Principal, ConsumerComponentID: r.Consumer.ComponentID, ConsumerCapability: r.Consumer.Capability, ExecutionScopeDigest: r.ExecutionScopeDigest}
	expectedID, err := DeriveEvidenceSubjectConsumerAssociationIDV1(input)
	if err != nil {
		return err
	}
	if r.AssociationID != expectedID {
		return evidenceSubjectConflictV1("Evidence subject Consumer association ID drifted")
	}
	copy := r
	copy.Digest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectConsumerAssociationRefV1", copy)
	if err != nil {
		return err
	}
	if digest != r.Digest {
		return evidenceSubjectConflictV1("Evidence subject Consumer association digest drifted")
	}
	return nil
}

func SealEvidenceSubjectConsumerAssociationRefV1(r EvidenceSubjectConsumerAssociationRefV1) (EvidenceSubjectConsumerAssociationRefV1, error) {
	input := EvidenceSubjectConsumerAssociationIDInputV1{Principal: r.Principal, ConsumerComponentID: r.Consumer.ComponentID, ConsumerCapability: r.Consumer.Capability, ExecutionScopeDigest: r.ExecutionScopeDigest}
	id, err := DeriveEvidenceSubjectConsumerAssociationIDV1(input)
	if err != nil {
		return EvidenceSubjectConsumerAssociationRefV1{}, err
	}
	if r.AssociationID != "" && r.AssociationID != id {
		return EvidenceSubjectConsumerAssociationRefV1{}, evidenceSubjectConflictV1("Evidence subject Consumer association supplied wrong ID")
	}
	r.AssociationID = id
	provided := r.Digest
	r.Digest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectConsumerAssociationRefV1", r)
	if err != nil {
		return EvidenceSubjectConsumerAssociationRefV1{}, err
	}
	if provided != "" && provided != digest {
		return EvidenceSubjectConsumerAssociationRefV1{}, evidenceSubjectConflictV1("Evidence subject Consumer association supplied wrong digest")
	}
	r.Digest = digest
	return r, r.Validate()
}

type EvidenceSubjectConsumerAssociationCurrentProjectionV1 struct {
	ContractVersion      string                                  `json:"contract_version"`
	Ref                  EvidenceSubjectConsumerAssociationRefV1 `json:"ref"`
	Principal            core.OwnerRef                           `json:"principal"`
	Consumer             ProviderBindingRefV2                    `json:"consumer"`
	ExecutionScopeDigest core.Digest                             `json:"execution_scope_digest"`
	BindingCurrent       ProviderBindingCurrentProjectionV2      `json:"binding_current"`
	CheckedUnixNano      int64                                   `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                                   `json:"expires_unix_nano"`
	ProjectionDigest     core.Digest                             `json:"projection_digest"`
}

func (p EvidenceSubjectConsumerAssociationCurrentProjectionV1) Validate(now time.Time) error {
	if p.ContractVersion != EvidenceSubjectCurrentContractVersionV1 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.Ref.Validate() != nil {
		return evidenceSubjectInvalidV1("Evidence subject Consumer association projection is incomplete")
	}
	if p.Ref.Principal != p.Principal || p.Ref.Consumer != p.Consumer || p.Ref.ExecutionScopeDigest != p.ExecutionScopeDigest || p.Consumer != p.BindingCurrent.Ref {
		return evidenceSubjectConflictV1("Evidence subject Consumer association projection drifted")
	}
	if err := p.BindingCurrent.ValidateCurrent(p.Consumer, now); err != nil {
		return err
	}
	if p.ExpiresUnixNano > p.BindingCurrent.ExpiresUnixNano || now.IsZero() || now.Before(time.Unix(0, p.CheckedUnixNano)) || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Evidence subject Consumer association is stale")
	}
	copy := p
	copy.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectConsumerAssociationCurrentProjectionV1", copy)
	if err != nil {
		return err
	}
	if digest != p.ProjectionDigest {
		return evidenceSubjectConflictV1("Evidence subject Consumer association projection digest drifted")
	}
	return nil
}

func SealEvidenceSubjectConsumerAssociationCurrentProjectionV1(p EvidenceSubjectConsumerAssociationCurrentProjectionV1) (EvidenceSubjectConsumerAssociationCurrentProjectionV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != EvidenceSubjectCurrentContractVersionV1 {
		return EvidenceSubjectConsumerAssociationCurrentProjectionV1{}, evidenceSubjectInvalidV1("Evidence subject Consumer association projection version is invalid")
	}
	p.ContractVersion = EvidenceSubjectCurrentContractVersionV1
	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectConsumerAssociationCurrentProjectionV1", p)
	if err != nil {
		return EvidenceSubjectConsumerAssociationCurrentProjectionV1{}, err
	}
	if provided != "" && provided != digest {
		return EvidenceSubjectConsumerAssociationCurrentProjectionV1{}, evidenceSubjectConflictV1("Evidence subject Consumer association projection supplied wrong digest")
	}
	p.ProjectionDigest = digest
	return p, nil
}

type EvidenceSubjectConsumerAssociationCurrentReaderV1 interface {
	InspectEvidenceSubjectConsumerAssociationCurrentV1(context.Context, EvidenceSubjectConsumerAssociationRefV1) (EvidenceSubjectConsumerAssociationCurrentProjectionV1, error)
}

type EvidenceTombstoneRefV1 struct {
	Record   EvidenceRecordRefV2 `json:"record"`
	Source   EvidenceSourceKeyV2 `json:"source"`
	Revision core.Revision       `json:"revision"`
	Digest   core.Digest         `json:"digest"`
}

func (r EvidenceTombstoneRefV1) Validate() error {
	if err := r.Record.Validate(); err != nil {
		return err
	}
	if err := r.Source.Validate(); err != nil {
		return err
	}
	if r.Revision != 1 {
		return evidenceSubjectInvalidV1("Evidence Tombstone ref must be revision one")
	}
	return r.Digest.Validate()
}

type EvidenceTombstoneAbsenceRefV1 struct {
	SubjectKeyDigest core.Digest   `json:"subject_key_digest"`
	Revision         core.Revision `json:"revision"`
	OwnerWatermark   core.Revision `json:"owner_watermark"`
	Digest           core.Digest   `json:"digest"`
}

func (r EvidenceTombstoneAbsenceRefV1) Validate() error {
	if r.Revision == 0 || r.OwnerWatermark == 0 || r.SubjectKeyDigest.Validate() != nil || r.Digest.Validate() != nil {
		return evidenceSubjectInvalidV1("Evidence Tombstone absence ref is incomplete")
	}
	copy := r
	copy.Digest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceTombstoneAbsenceRefV1", copy)
	if err != nil {
		return err
	}
	if digest != r.Digest {
		return evidenceSubjectConflictV1("Evidence Tombstone absence digest drifted")
	}
	return nil
}

func SealEvidenceTombstoneAbsenceRefV1(r EvidenceTombstoneAbsenceRefV1) (EvidenceTombstoneAbsenceRefV1, error) {
	provided := r.Digest
	r.Digest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceTombstoneAbsenceRefV1", r)
	if err != nil {
		return EvidenceTombstoneAbsenceRefV1{}, err
	}
	if provided != "" && provided != digest {
		return EvidenceTombstoneAbsenceRefV1{}, evidenceSubjectConflictV1("Evidence Tombstone absence supplied wrong digest")
	}
	r.Digest = digest
	return r, r.Validate()
}

type EvidenceReadabilityPolicyStateV1 string

const (
	EvidenceReadabilityPolicyActiveV1  EvidenceReadabilityPolicyStateV1 = "active"
	EvidenceReadabilityPolicyRevokedV1 EvidenceReadabilityPolicyStateV1 = "revoked"
	EvidenceReadabilityPolicyExpiredV1 EvidenceReadabilityPolicyStateV1 = "expired"
)

type EvidenceReadabilityPolicyRefV1 struct {
	PolicyID             string                           `json:"policy_id"`
	Revision             core.Revision                    `json:"revision"`
	Digest               core.Digest                      `json:"digest"`
	Owner                EvidenceProducerBindingRefV2     `json:"owner"`
	SubjectKeyDigest     core.Digest                      `json:"subject_key_digest"`
	ExecutionScopeDigest core.Digest                      `json:"execution_scope_digest"`
	Consumer             ProviderBindingRefV2             `json:"consumer"`
	AllowRead            bool                             `json:"allow_read"`
	State                EvidenceReadabilityPolicyStateV1 `json:"state"`
	ExpiresUnixNano      int64                            `json:"expires_unix_nano"`
}

func (r EvidenceReadabilityPolicyRefV1) Validate() error {
	if strings.TrimSpace(r.PolicyID) == "" || r.Revision == 0 || r.ExpiresUnixNano <= 0 || r.Owner.Validate() != nil || r.Consumer.Validate() != nil || r.SubjectKeyDigest.Validate() != nil || r.ExecutionScopeDigest.Validate() != nil || r.Digest.Validate() != nil {
		return evidenceSubjectInvalidV1("Evidence Readability policy ref is incomplete")
	}
	switch r.State {
	case EvidenceReadabilityPolicyActiveV1, EvidenceReadabilityPolicyRevokedV1, EvidenceReadabilityPolicyExpiredV1:
	default:
		return evidenceSubjectInvalidV1("Evidence Readability policy state is unknown")
	}
	copy := r
	copy.Digest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceReadabilityPolicyRefV1", copy)
	if err != nil {
		return err
	}
	if digest != r.Digest {
		return evidenceSubjectConflictV1("Evidence Readability policy digest drifted")
	}
	return nil
}

func SealEvidenceReadabilityPolicyRefV1(r EvidenceReadabilityPolicyRefV1) (EvidenceReadabilityPolicyRefV1, error) {
	provided := r.Digest
	r.Digest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceReadabilityPolicyRefV1", r)
	if err != nil {
		return EvidenceReadabilityPolicyRefV1{}, err
	}
	if provided != "" && provided != digest {
		return EvidenceReadabilityPolicyRefV1{}, evidenceSubjectConflictV1("Evidence Readability policy supplied wrong digest")
	}
	r.Digest = digest
	return r, r.Validate()
}

type EvidenceTombstonePresenceV1 string

const (
	EvidenceTombstoneAbsentSealedV1 EvidenceTombstonePresenceV1 = "tombstone_absent_sealed"
	EvidenceTombstonePresentV1      EvidenceTombstonePresenceV1 = "tombstone_present"
)

func (v EvidenceTombstonePresenceV1) Validate() error {
	if v != EvidenceTombstoneAbsentSealedV1 && v != EvidenceTombstonePresentV1 {
		return evidenceSubjectInvalidV1("Evidence Tombstone presence is unknown")
	}
	return nil
}

type EvidenceSubjectReadabilityV1 string

const (
	EvidenceSubjectReadableV1         EvidenceSubjectReadabilityV1 = "readable"
	EvidenceSubjectTombstonedV1       EvidenceSubjectReadabilityV1 = "tombstoned"
	EvidenceSubjectPolicyDeniedV1     EvidenceSubjectReadabilityV1 = "policy_denied"
	EvidenceSubjectRetentionExpiredV1 EvidenceSubjectReadabilityV1 = "retention_expired"
	EvidenceSubjectSourceInactiveV1   EvidenceSubjectReadabilityV1 = "source_inactive"
)

func (v EvidenceSubjectReadabilityV1) Validate() error {
	switch v {
	case EvidenceSubjectReadableV1, EvidenceSubjectTombstonedV1, EvidenceSubjectPolicyDeniedV1, EvidenceSubjectRetentionExpiredV1, EvidenceSubjectSourceInactiveV1:
		return nil
	default:
		return evidenceSubjectInvalidV1("Evidence subject Readability is unknown")
	}
}

type EvidenceSubjectCurrentIndexRefV1 struct {
	IndexID            string                          `json:"index_id"`
	Revision           core.Revision                   `json:"revision"`
	SubjectKeyDigest   core.Digest                     `json:"subject_key_digest"`
	PreviousProjection *EvidenceSubjectProjectionRefV1 `json:"previous_projection,omitempty"`
	CurrentProjection  EvidenceSubjectProjectionRefV1  `json:"current_projection"`
	OwnerWatermark     core.Revision                   `json:"owner_watermark"`
	Digest             core.Digest                     `json:"digest"`
}

func (r EvidenceSubjectCurrentIndexRefV1) Validate() error {
	if r.Revision == 0 || r.OwnerWatermark == 0 || r.SubjectKeyDigest.Validate() != nil || r.Digest.Validate() != nil || r.CurrentProjection.Validate() != nil {
		return evidenceSubjectInvalidV1("Evidence subject Current Index is incomplete")
	}
	id, err := DeriveEvidenceSubjectCurrentIndexIDV1(r.SubjectKeyDigest)
	if err != nil {
		return err
	}
	if r.IndexID != id || r.Revision != r.CurrentProjection.Revision || r.SubjectKeyDigest != r.CurrentProjection.SubjectKeyDigest || r.OwnerWatermark != r.CurrentProjection.OwnerWatermark {
		return evidenceSubjectConflictV1("Evidence subject Current Index coordinates drifted")
	}
	if r.Revision == 1 {
		if r.PreviousProjection != nil {
			return evidenceSubjectConflictV1("first Evidence subject Index cannot have previous Projection")
		}
	} else if r.PreviousProjection == nil || r.PreviousProjection.Validate() != nil || r.PreviousProjection.ProjectionID != r.CurrentProjection.ProjectionID || r.PreviousProjection.Revision+1 != r.Revision {
		return evidenceSubjectConflictV1("Evidence subject Index history is not contiguous")
	}
	copy := cloneEvidenceSubjectV1(r)
	copy.Digest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectCurrentIndexRefV1", copy)
	if err != nil {
		return err
	}
	if digest != r.Digest {
		return evidenceSubjectConflictV1("Evidence subject Current Index digest drifted")
	}
	return nil
}

func SealEvidenceSubjectCurrentIndexRefV1(r EvidenceSubjectCurrentIndexRefV1) (EvidenceSubjectCurrentIndexRefV1, error) {
	id, err := DeriveEvidenceSubjectCurrentIndexIDV1(r.SubjectKeyDigest)
	if err != nil {
		return EvidenceSubjectCurrentIndexRefV1{}, err
	}
	if r.IndexID != "" && r.IndexID != id {
		return EvidenceSubjectCurrentIndexRefV1{}, evidenceSubjectConflictV1("Evidence subject Index supplied wrong ID")
	}
	r.IndexID = id
	provided := r.Digest
	r.Digest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectCurrentIndexRefV1", r)
	if err != nil {
		return EvidenceSubjectCurrentIndexRefV1{}, err
	}
	if provided != "" && provided != digest {
		return EvidenceSubjectCurrentIndexRefV1{}, evidenceSubjectConflictV1("Evidence subject Index supplied wrong digest")
	}
	r.Digest = digest
	return r, r.Validate()
}

func evidenceSubjectInvalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}

func evidenceSubjectConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, message)
}

func cloneEvidenceSubjectV1[T any](value T) T {
	payload, _ := json.Marshal(value)
	var cloned T
	_ = json.Unmarshal(payload, &cloned)
	return cloned
}

func sameEvidenceSubjectV1(left, right any) bool { return reflect.DeepEqual(left, right) }

type EvidenceSubjectCurrentProjectionV1 struct {
	ContractVersion      string                          `json:"contract_version"`
	Ref                  EvidenceSubjectProjectionRefV1  `json:"ref"`
	Subject              EvidenceSubjectKeyV1            `json:"subject"`
	SubjectKeyDigest     core.Digest                     `json:"subject_key_digest"`
	PreviousProjection   *EvidenceSubjectProjectionRefV1 `json:"previous_projection,omitempty"`
	Record               EvidenceRecordRefV2             `json:"record"`
	Source               EvidenceSourceKeyV2             `json:"source"`
	CandidateDigest      core.Digest                     `json:"candidate_digest"`
	PreviousRecordDigest core.Digest                     `json:"previous_record_digest"`

	Registration                 EvidenceSourceRegistrationRefV1  `json:"registration"`
	RegistrationState            EvidenceSourceStateV2            `json:"registration_state"`
	RegistrationExpiresUnixNano  int64                            `json:"registration_expires_unix_nano"`
	SourcePolicy                 EvidenceSourcePolicyBindingRefV2 `json:"source_policy"`
	SourcePolicyState            EvidenceSourcePolicyStateV2      `json:"source_policy_state"`
	SourcePolicyOwner            EvidenceProducerBindingRefV2     `json:"source_policy_owner"`
	SourcePolicyAuthority        AuthorityBindingRefV2            `json:"source_policy_authority"`
	SourcePolicyAuthorityCurrent DispatchAuthorityFactV2          `json:"source_policy_authority_current"`
	SourcePolicyExpiresUnixNano  int64                            `json:"source_policy_expires_unix_nano"`

	LedgerScope            EvidenceLedgerScopeV2                `json:"ledger_scope"`
	LedgerScopeDigest      core.Digest                          `json:"ledger_scope_digest"`
	ExecutionScope         core.ExecutionScope                  `json:"execution_scope"`
	ExecutionScopeDigest   core.Digest                          `json:"execution_scope_digest"`
	CurrentScope           ExecutionScopeBindingRefV2           `json:"current_scope"`
	CurrentScopeWatermark  core.Revision                        `json:"current_scope_watermark"`
	ExecutionScopeCurrent  ExecutionScopeCurrentFactV2          `json:"execution_scope_current"`
	Producer               EvidenceProducerBindingRefV2         `json:"producer"`
	ProducerBindingCurrent ProviderBindingCurrentProjectionV2   `json:"producer_binding_current"`
	Authority              AuthorityBindingRefV2                `json:"authority"`
	AuthorityCurrent       DispatchAuthorityFactV2              `json:"authority_current"`
	ActionScopeDigest      core.Digest                          `json:"action_scope_digest"`
	Consumer               ProviderBindingRefV2                 `json:"consumer"`
	ReaderBinding          EvidenceSubjectReaderBindingRefV1    `json:"reader_binding"`
	ReaderCapability       EvidenceSubjectReaderCapabilityRefV1 `json:"reader_capability"`

	TrustClass       EvidenceTrustClassV2        `json:"trust_class"`
	ClaimKind        core.RunCompletionClaimKind `json:"claim_kind,omitempty"`
	EventKind        NamespacedNameV2            `json:"event_kind"`
	CustomClass      NamespacedNameV2            `json:"custom_class"`
	Payload          EvidencePayloadRefV2        `json:"payload"`
	Causation        []EvidenceCausationRefV2    `json:"causation"`
	CorrelationID    string                      `json:"correlation_id"`
	OwnerFact        *EvidenceOwnerFactRefV2     `json:"owner_fact,omitempty"`
	HistoricalSource *EvidenceHistoricalSourceV2 `json:"historical_source,omitempty"`
	ObservedUnixNano int64                       `json:"observed_unix_nano"`
	IngestedUnixNano int64                       `json:"ingested_unix_nano"`

	Presence          EvidenceTombstonePresenceV1    `json:"presence"`
	Readability       EvidenceSubjectReadabilityV1   `json:"readability"`
	Tombstone         *EvidenceTombstoneRefV1        `json:"tombstone,omitempty"`
	TombstoneAbsence  *EvidenceTombstoneAbsenceRefV1 `json:"tombstone_absence,omitempty"`
	ReadabilityPolicy EvidenceReadabilityPolicyRefV1 `json:"readability_policy"`
	CheckedUnixNano   int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                          `json:"expires_unix_nano"`
	ProjectionDigest  core.Digest                    `json:"projection_digest"`
}

func (p EvidenceSubjectCurrentProjectionV1) Validate() error {
	if p.ContractVersion != EvidenceSubjectCurrentContractVersionV1 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return evidenceSubjectInvalidV1("Evidence subject Current Projection version or TTL is invalid")
	}
	if err := p.Subject.Validate(); err != nil {
		return err
	}
	subjectDigest, err := DigestEvidenceSubjectKeyV1(p.Subject)
	if err != nil {
		return err
	}
	if p.SubjectKeyDigest != subjectDigest || p.Record != p.Subject.Record || p.Source != p.Subject.Source || p.Ref.SubjectKeyDigest != subjectDigest {
		return evidenceSubjectConflictV1("Evidence subject identity drifted across Projection")
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	if p.PreviousProjection == nil {
		if p.Ref.Revision != 1 {
			return evidenceSubjectConflictV1("Evidence subject first Projection must be revision one")
		}
	} else if p.PreviousProjection.Validate() != nil || p.PreviousProjection.ProjectionID != p.Ref.ProjectionID || p.PreviousProjection.Revision+1 != p.Ref.Revision {
		return evidenceSubjectConflictV1("Evidence subject Projection history is not contiguous")
	}
	if p.Causation == nil {
		return evidenceSubjectInvalidV1("Evidence subject Causation must use canonical empty slice")
	}
	if err := p.Presence.Validate(); err != nil {
		return err
	}
	if err := p.Readability.Validate(); err != nil {
		return err
	}
	if err := validateEvidenceSubjectPresencePairV1(p); err != nil {
		return err
	}
	copy := CloneEvidenceSubjectCurrentProjectionV1(p)
	copy.Ref.Digest = ""
	copy.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectCurrentProjectionV1", copy)
	if err != nil {
		return err
	}
	if digest != p.Ref.Digest || digest != p.ProjectionDigest {
		return evidenceSubjectConflictV1("Evidence subject Projection body and Ref digest drifted")
	}
	return nil
}

func SealEvidenceSubjectCurrentProjectionV1(p EvidenceSubjectCurrentProjectionV1) (EvidenceSubjectCurrentProjectionV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != EvidenceSubjectCurrentContractVersionV1 {
		return EvidenceSubjectCurrentProjectionV1{}, evidenceSubjectInvalidV1("Evidence subject Projection contract version is invalid")
	}
	p.ContractVersion = EvidenceSubjectCurrentContractVersionV1
	if p.Causation == nil {
		p.Causation = []EvidenceCausationRefV2{}
	}
	subjectDigest, err := DigestEvidenceSubjectKeyV1(p.Subject)
	if err != nil {
		return EvidenceSubjectCurrentProjectionV1{}, err
	}
	if p.SubjectKeyDigest != "" && p.SubjectKeyDigest != subjectDigest {
		return EvidenceSubjectCurrentProjectionV1{}, evidenceSubjectConflictV1("Evidence subject Projection supplied wrong subject digest")
	}
	p.SubjectKeyDigest = subjectDigest
	if p.Record != (EvidenceRecordRefV2{}) && p.Record != p.Subject.Record {
		return EvidenceSubjectCurrentProjectionV1{}, evidenceSubjectConflictV1("Evidence subject Projection supplied wrong Record")
	}
	if p.Source != (EvidenceSourceKeyV2{}) && p.Source != p.Subject.Source {
		return EvidenceSubjectCurrentProjectionV1{}, evidenceSubjectConflictV1("Evidence subject Projection supplied wrong Source")
	}
	p.Record, p.Source = p.Subject.Record, p.Subject.Source
	id, err := DeriveEvidenceSubjectProjectionIDV1(subjectDigest)
	if err != nil {
		return EvidenceSubjectCurrentProjectionV1{}, err
	}
	if p.Ref.ProjectionID != "" && p.Ref.ProjectionID != id {
		return EvidenceSubjectCurrentProjectionV1{}, evidenceSubjectConflictV1("Evidence subject Projection supplied wrong ID")
	}
	p.Ref.ProjectionID = id
	if p.Ref.SubjectKeyDigest != "" && p.Ref.SubjectKeyDigest != subjectDigest {
		return EvidenceSubjectCurrentProjectionV1{}, evidenceSubjectConflictV1("Evidence subject Projection Ref supplied wrong subject digest")
	}
	p.Ref.SubjectKeyDigest = subjectDigest
	providedRefDigest, providedDigest := p.Ref.Digest, p.ProjectionDigest
	p.Ref.Digest, p.ProjectionDigest = "", ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectCurrentProjectionV1", p)
	if err != nil {
		return EvidenceSubjectCurrentProjectionV1{}, err
	}
	if (providedRefDigest != "" && providedRefDigest != digest) || (providedDigest != "" && providedDigest != digest) {
		return EvidenceSubjectCurrentProjectionV1{}, evidenceSubjectConflictV1("Evidence subject Projection supplied wrong digest")
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p, p.Validate()
}

func CloneEvidenceSubjectCurrentProjectionV1(p EvidenceSubjectCurrentProjectionV1) EvidenceSubjectCurrentProjectionV1 {
	return cloneEvidenceSubjectV1(p)
}

func DecodeEvidenceSubjectCurrentProjectionV1(payload []byte) (EvidenceSubjectCurrentProjectionV1, error) {
	var value EvidenceSubjectCurrentProjectionV1
	if err := core.DecodeStrictJSON(payload, &value); err != nil {
		return EvidenceSubjectCurrentProjectionV1{}, err
	}
	return value, value.Validate()
}

func validateEvidenceSubjectPresencePairV1(p EvidenceSubjectCurrentProjectionV1) error {
	if p.Presence == EvidenceTombstonePresentV1 {
		if p.Readability != EvidenceSubjectTombstonedV1 || p.Tombstone == nil || p.TombstoneAbsence != nil || p.Tombstone.Validate() != nil || p.Tombstone.Record != p.Record || p.Tombstone.Source != p.Source {
			return evidenceSubjectInvalidV1("present Tombstone requires exact tombstoned branch")
		}
		return nil
	}
	if p.Readability == EvidenceSubjectTombstonedV1 || p.Tombstone != nil || p.TombstoneAbsence == nil {
		return evidenceSubjectInvalidV1("sealed absence cannot carry tombstoned branch")
	}
	return p.TombstoneAbsence.Validate()
}

type EvidenceSubjectRecordRegistrationCurrentRequestV1 struct {
	ContractVersion string               `json:"contract_version"`
	Subject         EvidenceSubjectKeyV1 `json:"subject"`
}

func (r EvidenceSubjectRecordRegistrationCurrentRequestV1) Validate() error {
	if r.ContractVersion != EvidenceSubjectCurrentContractVersionV1 {
		return evidenceSubjectInvalidV1("Evidence subject Record request version is invalid")
	}
	return r.Subject.Validate()
}

type EvidenceSubjectRecordRegistrationCurrentResultV1 struct {
	ContractVersion  string                           `json:"contract_version"`
	Subject          EvidenceSubjectKeyV1             `json:"subject"`
	Record           EvidenceLedgerRecordV2           `json:"record"`
	Registration     EvidenceSourceRegistrationFactV2 `json:"registration"`
	CheckedUnixNano  int64                            `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                            `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                      `json:"projection_digest"`
}

func (r EvidenceSubjectRecordRegistrationCurrentResultV1) Validate(now time.Time) error {
	if r.ContractVersion != EvidenceSubjectCurrentContractVersionV1 || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano || r.Subject.Validate() != nil || r.Record.Validate() != nil || r.Registration.Validate() != nil {
		return evidenceSubjectInvalidV1("Evidence subject Record current result is incomplete")
	}
	if r.Record.Ref != r.Subject.Record || r.Record.Candidate.RegistrationID != r.Subject.Source.RegistrationID || r.Record.Candidate.SourceEpoch != r.Subject.Source.SourceEpoch || r.Record.Candidate.SourceSequence != r.Subject.Source.SourceSequence || r.Registration.ID != r.Subject.Source.RegistrationID || r.Registration.SourceEpoch != r.Subject.Source.SourceEpoch {
		return evidenceSubjectConflictV1("Evidence subject Record current result drifted")
	}
	if now.IsZero() || now.Before(time.Unix(0, r.CheckedUnixNano)) || !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Evidence subject Record current result expired")
	}
	copy := r
	copy.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectRecordRegistrationCurrentResultV1", copy)
	if err != nil || digest != r.ProjectionDigest {
		return evidenceSubjectConflictV1("Evidence subject Record current result digest drifted")
	}
	return nil
}

func SealEvidenceSubjectRecordRegistrationCurrentResultV1(r EvidenceSubjectRecordRegistrationCurrentResultV1) (EvidenceSubjectRecordRegistrationCurrentResultV1, error) {
	if r.ContractVersion != "" && r.ContractVersion != EvidenceSubjectCurrentContractVersionV1 {
		return EvidenceSubjectRecordRegistrationCurrentResultV1{}, evidenceSubjectInvalidV1("Evidence subject Record result version is invalid")
	}
	r.ContractVersion = EvidenceSubjectCurrentContractVersionV1
	provided := r.ProjectionDigest
	r.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectRecordRegistrationCurrentResultV1", r)
	if err != nil {
		return EvidenceSubjectRecordRegistrationCurrentResultV1{}, err
	}
	if provided != "" && provided != digest {
		return EvidenceSubjectRecordRegistrationCurrentResultV1{}, evidenceSubjectConflictV1("Evidence subject Record result supplied wrong digest")
	}
	r.ProjectionDigest = digest
	return r, nil
}

type EvidenceSubjectRecordRegistrationCurrentReaderV1 interface {
	InspectEvidenceSubjectRecordRegistrationCurrentV1(context.Context, EvidenceSubjectRecordRegistrationCurrentRequestV1) (EvidenceSubjectRecordRegistrationCurrentResultV1, error)
}

type EvidenceSubjectPresenceReadabilityCurrentRequestV1 struct {
	ContractVersion              string               `json:"contract_version"`
	Subject                      EvidenceSubjectKeyV1 `json:"subject"`
	ExpectedConsumer             ProviderBindingRefV2 `json:"expected_consumer"`
	ExpectedExecutionScopeDigest core.Digest          `json:"expected_execution_scope_digest"`
	ExpectedOwnerWatermark       core.Revision        `json:"expected_owner_watermark"`
}

func (r EvidenceSubjectPresenceReadabilityCurrentRequestV1) Validate() error {
	if r.ContractVersion != EvidenceSubjectCurrentContractVersionV1 || r.ExpectedConsumer.Validate() != nil || r.ExpectedExecutionScopeDigest.Validate() != nil || r.ExpectedOwnerWatermark == 0 {
		return evidenceSubjectInvalidV1("Evidence subject Presence request is incomplete")
	}
	return r.Subject.Validate()
}

type EvidenceSubjectPresenceReadabilityCurrentResultV1 struct {
	ContractVersion   string                         `json:"contract_version"`
	Subject           EvidenceSubjectKeyV1           `json:"subject"`
	SubjectKeyDigest  core.Digest                    `json:"subject_key_digest"`
	Presence          EvidenceTombstonePresenceV1    `json:"presence"`
	Readability       EvidenceSubjectReadabilityV1   `json:"readability"`
	Tombstone         *EvidenceTombstoneRefV1        `json:"tombstone,omitempty"`
	TombstoneAbsence  *EvidenceTombstoneAbsenceRefV1 `json:"tombstone_absence,omitempty"`
	ReadabilityPolicy EvidenceReadabilityPolicyRefV1 `json:"readability_policy"`
	OwnerWatermark    core.Revision                  `json:"owner_watermark"`
	CheckedUnixNano   int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                          `json:"expires_unix_nano"`
	ProjectionDigest  core.Digest                    `json:"projection_digest"`
}

func (r EvidenceSubjectPresenceReadabilityCurrentResultV1) Validate(request EvidenceSubjectPresenceReadabilityCurrentRequestV1, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if r.ContractVersion != EvidenceSubjectCurrentContractVersionV1 || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano || r.OwnerWatermark == 0 || r.Subject.Validate() != nil || r.Presence.Validate() != nil || r.Readability.Validate() != nil || r.ReadabilityPolicy.Validate() != nil {
		return evidenceSubjectInvalidV1("Evidence subject Presence current result is incomplete")
	}
	digest, err := DigestEvidenceSubjectKeyV1(r.Subject)
	if err != nil {
		return err
	}
	if !sameEvidenceSubjectV1(r.Subject, request.Subject) || r.SubjectKeyDigest != digest || r.OwnerWatermark != request.ExpectedOwnerWatermark || r.ReadabilityPolicy.SubjectKeyDigest != digest || r.ReadabilityPolicy.ExecutionScopeDigest != request.ExpectedExecutionScopeDigest || r.ReadabilityPolicy.Consumer != request.ExpectedConsumer {
		return evidenceSubjectConflictV1("Evidence subject Presence current coordinates drifted")
	}
	if !r.ReadabilityPolicy.AllowRead || r.ReadabilityPolicy.State != EvidenceReadabilityPolicyActiveV1 {
		return core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "Evidence subject Readability policy does not authorize reads")
	}
	projection := EvidenceSubjectCurrentProjectionV1{Subject: r.Subject, Record: r.Subject.Record, Source: r.Subject.Source, Presence: r.Presence, Readability: r.Readability, Tombstone: r.Tombstone, TombstoneAbsence: r.TombstoneAbsence}
	if err := validateEvidenceSubjectPresencePairV1(projection); err != nil {
		return err
	}
	if now.IsZero() || now.Before(time.Unix(0, r.CheckedUnixNano)) || !now.Before(time.Unix(0, r.ExpiresUnixNano)) || !now.Before(time.Unix(0, r.ReadabilityPolicy.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Evidence subject Presence current result expired")
	}
	copy := cloneEvidenceSubjectV1(r)
	copy.ProjectionDigest = ""
	expected, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectPresenceReadabilityCurrentResultV1", copy)
	if err != nil || expected != r.ProjectionDigest {
		return evidenceSubjectConflictV1("Evidence subject Presence current result digest drifted")
	}
	return nil
}

func SealEvidenceSubjectPresenceReadabilityCurrentResultV1(r EvidenceSubjectPresenceReadabilityCurrentResultV1) (EvidenceSubjectPresenceReadabilityCurrentResultV1, error) {
	if r.ContractVersion != "" && r.ContractVersion != EvidenceSubjectCurrentContractVersionV1 {
		return EvidenceSubjectPresenceReadabilityCurrentResultV1{}, evidenceSubjectInvalidV1("Evidence subject Presence result version is invalid")
	}
	r.ContractVersion = EvidenceSubjectCurrentContractVersionV1
	provided := r.ProjectionDigest
	r.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectPresenceReadabilityCurrentResultV1", r)
	if err != nil {
		return EvidenceSubjectPresenceReadabilityCurrentResultV1{}, err
	}
	if provided != "" && provided != digest {
		return EvidenceSubjectPresenceReadabilityCurrentResultV1{}, evidenceSubjectConflictV1("Evidence subject Presence result supplied wrong digest")
	}
	r.ProjectionDigest = digest
	return r, nil
}

type EvidenceSubjectPresenceReadabilityCurrentReaderV1 interface {
	InspectEvidenceSubjectPresenceReadabilityCurrentV1(context.Context, EvidenceSubjectPresenceReadabilityCurrentRequestV1) (EvidenceSubjectPresenceReadabilityCurrentResultV1, error)
}

type EvidenceSubjectCurrentLookupRequestV1 struct {
	ContractVersion              string                           `json:"contract_version"`
	Subject                      EvidenceSubjectKeyV1             `json:"subject"`
	ExpectedConsumer             ProviderBindingRefV2             `json:"expected_consumer"`
	ExpectedExecutionScopeDigest core.Digest                      `json:"expected_execution_scope_digest"`
	ExpectedSourcePolicy         EvidenceSourcePolicyBindingRefV2 `json:"expected_source_policy"`
}

func (r EvidenceSubjectCurrentLookupRequestV1) Validate() error {
	if r.ContractVersion != EvidenceSubjectCurrentContractVersionV1 || r.ExpectedConsumer.Validate() != nil || r.ExpectedExecutionScopeDigest.Validate() != nil || r.ExpectedSourcePolicy.Validate() != nil {
		return evidenceSubjectInvalidV1("Evidence subject Current lookup is incomplete")
	}
	return r.Subject.Validate()
}

type EvidenceSubjectCurrentSnapshotV1 struct {
	ContractVersion string                             `json:"contract_version"`
	Projection      EvidenceSubjectCurrentProjectionV1 `json:"projection"`
	CurrentIndex    EvidenceSubjectCurrentIndexRefV1   `json:"current_index"`
}

func (s EvidenceSubjectCurrentSnapshotV1) Validate() error {
	if s.ContractVersion != EvidenceSubjectCurrentContractVersionV1 {
		return evidenceSubjectInvalidV1("Evidence subject Snapshot version is invalid")
	}
	if err := s.Projection.Validate(); err != nil {
		return err
	}
	if err := s.CurrentIndex.Validate(); err != nil {
		return err
	}
	if !sameEvidenceSubjectV1(s.CurrentIndex.CurrentProjection, s.Projection.Ref) {
		return evidenceSubjectConflictV1("Evidence subject Snapshot Index does not reference its Projection")
	}
	return nil
}

type EvidenceSubjectCurrentValidationRequestV1 struct {
	ContractVersion              string                               `json:"contract_version"`
	Subject                      EvidenceSubjectKeyV1                 `json:"subject"`
	ExpectedProjection           EvidenceSubjectProjectionRefV1       `json:"expected_projection"`
	ExpectedCurrentIndex         EvidenceSubjectCurrentIndexRefV1     `json:"expected_current_index"`
	ExpectedRegistration         EvidenceSourceRegistrationRefV1      `json:"expected_registration"`
	ExpectedReaderBinding        EvidenceSubjectReaderBindingRefV1    `json:"expected_reader_binding"`
	ExpectedReaderCapability     EvidenceSubjectReaderCapabilityRefV1 `json:"expected_reader_capability"`
	ExpectedConsumer             ProviderBindingRefV2                 `json:"expected_consumer"`
	ExpectedExecutionScopeDigest core.Digest                          `json:"expected_execution_scope_digest"`
	ExpectedSourcePolicy         EvidenceSourcePolicyBindingRefV2     `json:"expected_source_policy"`
}

func (r EvidenceSubjectCurrentValidationRequestV1) Validate() error {
	if r.ContractVersion != EvidenceSubjectCurrentContractVersionV1 || r.Subject.Validate() != nil || r.ExpectedProjection.Validate() != nil || r.ExpectedCurrentIndex.Validate() != nil || r.ExpectedRegistration.Validate() != nil || r.ExpectedReaderBinding.Validate() != nil || r.ExpectedReaderCapability.Validate() != nil || r.ExpectedConsumer.Validate() != nil || r.ExpectedExecutionScopeDigest.Validate() != nil || r.ExpectedSourcePolicy.Validate() != nil {
		return evidenceSubjectInvalidV1("Evidence subject Current validation request is incomplete")
	}
	if !sameEvidenceSubjectV1(r.ExpectedCurrentIndex.CurrentProjection, r.ExpectedProjection) || r.ExpectedReaderBinding.Capability != r.ExpectedReaderCapability || r.ExpectedReaderBinding.Binding != r.ExpectedConsumer {
		return evidenceSubjectConflictV1("Evidence subject Current validation expectations are inconsistent")
	}
	return nil
}

type EvidenceSubjectCurrentReaderV1 interface {
	InspectEvidenceSubjectProjectionV1(context.Context, EvidenceSubjectProjectionRefV1) (EvidenceSubjectCurrentProjectionV1, error)
	InspectEvidenceSubjectCurrentV1(context.Context, EvidenceSubjectCurrentLookupRequestV1) (EvidenceSubjectCurrentSnapshotV1, error)
	ValidateEvidenceSubjectCurrentV1(context.Context, EvidenceSubjectCurrentValidationRequestV1) (EvidenceSubjectCurrentSnapshotV1, error)
}

type EvidenceSubjectCurrentFactPortV1 interface {
	InspectEvidenceSubjectProjectionFactV1(context.Context, EvidenceSubjectProjectionRefV1) (EvidenceSubjectCurrentProjectionV1, error)
	InspectEvidenceSubjectCurrentIndexV1(context.Context, core.Digest) (EvidenceSubjectCurrentIndexRefV1, error)
	PublishEvidenceSubjectMutationV1(context.Context, EvidenceSubjectMutationCommitV1, EvidenceSubjectCurrentProjectionV1, EvidenceSubjectCurrentIndexRefV1) (EvidenceSubjectMutationCommitV1, error)
	InspectEvidenceSubjectMutationV1(context.Context, EvidenceSubjectMutationKeyV1) (EvidenceSubjectMutationCommitV1, error)
}

type EvidenceSubjectMutationKindV1 string

const (
	EvidenceSubjectMutationSourceRegistrationAdvanceV1 EvidenceSubjectMutationKindV1 = "source_registration_advance"
	EvidenceSubjectMutationSourcePolicyAdvanceV1       EvidenceSubjectMutationKindV1 = "source_policy_advance"
	EvidenceSubjectMutationTombstoneCreateV1           EvidenceSubjectMutationKindV1 = "tombstone_create"
	EvidenceSubjectMutationReadabilityPolicyAdvanceV1  EvidenceSubjectMutationKindV1 = "readability_policy_advance"
)

func (k EvidenceSubjectMutationKindV1) Validate() error {
	switch k {
	case EvidenceSubjectMutationSourceRegistrationAdvanceV1, EvidenceSubjectMutationSourcePolicyAdvanceV1, EvidenceSubjectMutationTombstoneCreateV1, EvidenceSubjectMutationReadabilityPolicyAdvanceV1:
		return nil
	default:
		return evidenceSubjectInvalidV1("Evidence subject Mutation kind is unknown")
	}
}

type EvidenceSubjectMutationRequestV1 struct {
	ContractVersion           string                            `json:"contract_version"`
	Subject                   EvidenceSubjectKeyV1              `json:"subject"`
	Kind                      EvidenceSubjectMutationKindV1     `json:"kind"`
	ExpectedCurrentIndex      *EvidenceSubjectCurrentIndexRefV1 `json:"expected_current_index,omitempty"`
	ExpectedCurrentProjection *EvidenceSubjectProjectionRefV1   `json:"expected_current_projection,omitempty"`
	Registration              *EvidenceSourceRegistrationRefV1  `json:"registration,omitempty"`
	SourcePolicy              *EvidenceSourcePolicyBindingRefV2 `json:"source_policy,omitempty"`
	Tombstone                 *EvidenceTombstoneRefV1           `json:"tombstone,omitempty"`
	ReadabilityPolicy         *EvidenceReadabilityPolicyRefV1   `json:"readability_policy,omitempty"`
	RequestDigest             core.Digest                       `json:"request_digest"`
}

func (r EvidenceSubjectMutationRequestV1) Validate() error {
	if r.ContractVersion != EvidenceSubjectCurrentContractVersionV1 || r.Subject.Validate() != nil || r.Kind.Validate() != nil || r.RequestDigest.Validate() != nil {
		return evidenceSubjectInvalidV1("Evidence subject Mutation request is incomplete")
	}
	if (r.ExpectedCurrentIndex == nil) != (r.ExpectedCurrentProjection == nil) {
		return evidenceSubjectInvalidV1("Evidence subject Mutation expected refs must be both absent or present")
	}
	if r.ExpectedCurrentIndex != nil {
		if r.ExpectedCurrentIndex.Validate() != nil || r.ExpectedCurrentProjection.Validate() != nil || !sameEvidenceSubjectV1(r.ExpectedCurrentIndex.CurrentProjection, *r.ExpectedCurrentProjection) {
			return evidenceSubjectConflictV1("Evidence subject Mutation expected refs drifted")
		}
	}
	nonNil := 0
	if r.Registration != nil {
		nonNil++
	}
	if r.SourcePolicy != nil {
		nonNil++
	}
	if r.Tombstone != nil {
		nonNil++
	}
	if r.ReadabilityPolicy != nil {
		nonNil++
	}
	if nonNil != 1 {
		return evidenceSubjectInvalidV1("Evidence subject Mutation requires exactly one typed payload")
	}
	switch r.Kind {
	case EvidenceSubjectMutationSourceRegistrationAdvanceV1:
		if r.Registration == nil || r.Registration.Validate() != nil {
			return evidenceSubjectInvalidV1("source Registration mutation payload is invalid")
		}
	case EvidenceSubjectMutationSourcePolicyAdvanceV1:
		if r.SourcePolicy == nil || r.SourcePolicy.Validate() != nil {
			return evidenceSubjectInvalidV1("source Policy mutation payload is invalid")
		}
	case EvidenceSubjectMutationTombstoneCreateV1:
		if r.Tombstone == nil || r.Tombstone.Validate() != nil {
			return evidenceSubjectInvalidV1("Tombstone mutation payload is invalid")
		}
	case EvidenceSubjectMutationReadabilityPolicyAdvanceV1:
		if r.ReadabilityPolicy == nil || r.ReadabilityPolicy.Validate() != nil {
			return evidenceSubjectInvalidV1("Readability mutation payload is invalid")
		}
	}
	copy := cloneEvidenceSubjectV1(r)
	copy.RequestDigest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectMutationRequestV1", copy)
	if err != nil || digest != r.RequestDigest {
		return evidenceSubjectConflictV1("Evidence subject Mutation request digest drifted")
	}
	return nil
}

func SealEvidenceSubjectMutationRequestV1(r EvidenceSubjectMutationRequestV1) (EvidenceSubjectMutationRequestV1, error) {
	if r.ContractVersion != "" && r.ContractVersion != EvidenceSubjectCurrentContractVersionV1 {
		return EvidenceSubjectMutationRequestV1{}, evidenceSubjectInvalidV1("Evidence subject Mutation request version is invalid")
	}
	r.ContractVersion = EvidenceSubjectCurrentContractVersionV1
	provided := r.RequestDigest
	r.RequestDigest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectMutationRequestV1", r)
	if err != nil {
		return EvidenceSubjectMutationRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return EvidenceSubjectMutationRequestV1{}, evidenceSubjectConflictV1("Evidence subject Mutation request supplied wrong digest")
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

type EvidenceSubjectMutationStableKeyInputV1 struct {
	SubjectKeyDigest          core.Digest                       `json:"subject_key_digest"`
	Kind                      EvidenceSubjectMutationKindV1     `json:"kind"`
	ExpectedCurrentIndex      *EvidenceSubjectCurrentIndexRefV1 `json:"expected_current_index,omitempty"`
	ExpectedCurrentProjection *EvidenceSubjectProjectionRefV1   `json:"expected_current_projection,omitempty"`
	FirstCreateSentinel       string                            `json:"first_create_sentinel,omitempty"`
}

type EvidenceSubjectMutationKeyV1 struct {
	ContractVersion           string                            `json:"contract_version"`
	MutationID                string                            `json:"mutation_id"`
	SubjectKeyDigest          core.Digest                       `json:"subject_key_digest"`
	Kind                      EvidenceSubjectMutationKindV1     `json:"kind"`
	ExpectedCurrentIndex      *EvidenceSubjectCurrentIndexRefV1 `json:"expected_current_index,omitempty"`
	ExpectedCurrentProjection *EvidenceSubjectProjectionRefV1   `json:"expected_current_projection,omitempty"`
	StableKeyDigest           core.Digest                       `json:"stable_key_digest"`
	RequestDigest             core.Digest                       `json:"request_digest"`
}

func DeriveEvidenceSubjectMutationKeyV1(request EvidenceSubjectMutationRequestV1) (EvidenceSubjectMutationKeyV1, error) {
	if err := request.Validate(); err != nil {
		return EvidenceSubjectMutationKeyV1{}, err
	}
	subjectDigest, err := DigestEvidenceSubjectKeyV1(request.Subject)
	if err != nil {
		return EvidenceSubjectMutationKeyV1{}, err
	}
	input := EvidenceSubjectMutationStableKeyInputV1{SubjectKeyDigest: subjectDigest, Kind: request.Kind, ExpectedCurrentIndex: cloneEvidenceSubjectV1(request.ExpectedCurrentIndex), ExpectedCurrentProjection: cloneEvidenceSubjectV1(request.ExpectedCurrentProjection)}
	if request.ExpectedCurrentIndex == nil {
		input.FirstCreateSentinel = evidenceSubjectNoCurrentSentinelV1
	}
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectMutationStableKeyV1", input)
	if err != nil {
		return EvidenceSubjectMutationKeyV1{}, err
	}
	return EvidenceSubjectMutationKeyV1{ContractVersion: EvidenceSubjectCurrentContractVersionV1, MutationID: string(digest), SubjectKeyDigest: subjectDigest, Kind: request.Kind, ExpectedCurrentIndex: cloneEvidenceSubjectV1(request.ExpectedCurrentIndex), ExpectedCurrentProjection: cloneEvidenceSubjectV1(request.ExpectedCurrentProjection), StableKeyDigest: digest, RequestDigest: request.RequestDigest}, nil
}

func (k EvidenceSubjectMutationKeyV1) Validate() error {
	if k.ContractVersion != EvidenceSubjectCurrentContractVersionV1 || k.Kind.Validate() != nil || k.SubjectKeyDigest.Validate() != nil || k.StableKeyDigest.Validate() != nil || k.RequestDigest.Validate() != nil || k.MutationID != string(k.StableKeyDigest) {
		return evidenceSubjectInvalidV1("Evidence subject Mutation key is incomplete")
	}
	if (k.ExpectedCurrentIndex == nil) != (k.ExpectedCurrentProjection == nil) {
		return evidenceSubjectInvalidV1("Evidence subject Mutation key expected refs disagree")
	}
	input := EvidenceSubjectMutationStableKeyInputV1{SubjectKeyDigest: k.SubjectKeyDigest, Kind: k.Kind, ExpectedCurrentIndex: cloneEvidenceSubjectV1(k.ExpectedCurrentIndex), ExpectedCurrentProjection: cloneEvidenceSubjectV1(k.ExpectedCurrentProjection)}
	if k.ExpectedCurrentIndex == nil {
		input.FirstCreateSentinel = evidenceSubjectNoCurrentSentinelV1
	}
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectMutationStableKeyV1", input)
	if err != nil || digest != k.StableKeyDigest {
		return evidenceSubjectConflictV1("Evidence subject Mutation stable key drifted")
	}
	return nil
}

type EvidenceSubjectMutationCommitV1 struct {
	ContractVersion            string                            `json:"contract_version"`
	Key                        EvidenceSubjectMutationKeyV1      `json:"key"`
	Request                    EvidenceSubjectMutationRequestV1  `json:"request"`
	Subject                    EvidenceSubjectKeyV1              `json:"subject"`
	RequestDigest              core.Digest                       `json:"request_digest"`
	ExpectedPreviousIndex      *EvidenceSubjectCurrentIndexRefV1 `json:"expected_previous_index,omitempty"`
	ExpectedPreviousProjection *EvidenceSubjectProjectionRefV1   `json:"expected_previous_projection,omitempty"`
	NewProjection              EvidenceSubjectProjectionRefV1    `json:"new_projection"`
	NewIndex                   EvidenceSubjectCurrentIndexRefV1  `json:"new_index"`
	CommittedUnixNano          int64                             `json:"committed_unix_nano"`
	CommitDigest               core.Digest                       `json:"commit_digest"`
}

func (c EvidenceSubjectMutationCommitV1) Validate() error {
	if c.ContractVersion != EvidenceSubjectCurrentContractVersionV1 || c.Key.Validate() != nil || c.Request.Validate() != nil || c.Subject.Validate() != nil || c.NewProjection.Validate() != nil || c.NewIndex.Validate() != nil || c.CommittedUnixNano <= 0 || c.CommitDigest.Validate() != nil {
		return evidenceSubjectInvalidV1("Evidence subject Mutation Commit is incomplete")
	}
	if c.RequestDigest != c.Request.RequestDigest || c.RequestDigest != c.Key.RequestDigest || !sameEvidenceSubjectV1(c.Request.Subject, c.Subject) || !sameEvidenceSubjectV1(c.Request.ExpectedCurrentIndex, c.ExpectedPreviousIndex) || !sameEvidenceSubjectV1(c.Request.ExpectedCurrentProjection, c.ExpectedPreviousProjection) || !sameEvidenceSubjectV1(c.NewIndex.CurrentProjection, c.NewProjection) {
		return evidenceSubjectConflictV1("Evidence subject Mutation Commit relationships drifted")
	}
	copy := cloneEvidenceSubjectV1(c)
	copy.CommitDigest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectMutationCommitV1", copy)
	if err != nil || digest != c.CommitDigest {
		return evidenceSubjectConflictV1("Evidence subject Mutation Commit digest drifted")
	}
	return nil
}

func SealEvidenceSubjectMutationCommitV1(c EvidenceSubjectMutationCommitV1) (EvidenceSubjectMutationCommitV1, error) {
	if c.ContractVersion != "" && c.ContractVersion != EvidenceSubjectCurrentContractVersionV1 {
		return EvidenceSubjectMutationCommitV1{}, evidenceSubjectInvalidV1("Evidence subject Mutation Commit version is invalid")
	}
	c.ContractVersion = EvidenceSubjectCurrentContractVersionV1
	provided := c.CommitDigest
	c.CommitDigest = ""
	digest, err := core.CanonicalJSONDigest(EvidenceSubjectCurrentCanonicalDomainV1, EvidenceSubjectCurrentContractVersionV1, "EvidenceSubjectMutationCommitV1", c)
	if err != nil {
		return EvidenceSubjectMutationCommitV1{}, err
	}
	if provided != "" && provided != digest {
		return EvidenceSubjectMutationCommitV1{}, evidenceSubjectConflictV1("Evidence subject Mutation Commit supplied wrong digest")
	}
	c.CommitDigest = digest
	return c, c.Validate()
}

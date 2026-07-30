package contract

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	buildercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ComponentFactoryContractVersionV2         = "praxis.agent-host/component-factory/v2"
	ComponentFactoryDescriptorObjectKindV2    = "praxis.agent-host/ComponentFactoryDescriptorV2"
	ComponentFactoryConformanceObjectKindV2   = "praxis.agent-host/ComponentFactoryConformanceCurrentV2"
	ComponentFactoryPreflightRequestKindV2    = "praxis.agent-host/ComponentFactoryPreflightRequestV2"
	ComponentFactoryPreflightReceiptKindV2    = "praxis.agent-host/ComponentFactoryPreflightReceiptV2"
	ComponentFactoryRegistrationObjectKindV2  = "praxis.agent-host/ComponentFactoryRegistrationV2"
	ComponentFactoryImplementationOwnerV2     = "owner_executable"
	ComponentFactoryProviderAccessNoneV2      = "none"
	ComponentFactoryConstructionTrustedGoV2   = "trusted_in_process_go"
	ComponentFactoryPreflightReceiptRefKindV2 = "praxis.agent-host/component-factory-preflight"
	ComponentFactoryPreflightAttemptPrefixV2  = "component-preflight/"
	ComponentFactoryStartAttemptPrefixV2      = "component-start/"
	MaxComponentFactoryDependenciesV2         = 256
	MaxComponentFactoryResourcesV2            = 256
)

type ComponentFactoryRefV2 struct {
	FactoryID string   `json:"factory_id"`
	Revision  uint64   `json:"revision"`
	Digest    DigestV1 `json:"digest"`
}

func (ref ComponentFactoryRefV2) Validate() error {
	if err := ValidateIdentifierV1("component factory id", ref.FactoryID); err != nil {
		return err
	}
	if ref.Revision == 0 {
		return NewError(ErrorInvalidArgument, "component_factory_revision_invalid", "component factory revision must be positive")
	}
	return ref.Digest.Validate()
}

type ComponentSchemaRefV2 struct {
	Namespace     string   `json:"namespace"`
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	MediaType     string   `json:"media_type"`
	ContentDigest DigestV1 `json:"content_digest"`
}

func (ref ComponentSchemaRefV2) Validate() error {
	for field, value := range map[string]string{
		"schema namespace": ref.Namespace,
		"schema name":      ref.Name,
		"schema version":   ref.Version,
	} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	if !validComponentSchemaMediaTypeV2(ref.MediaType) {
		return NewError(ErrorInvalidArgument, "component_schema_media_type_invalid", "component schema media type is invalid")
	}
	return ref.ContentDigest.Validate()
}

func validComponentSchemaMediaTypeV2(value string) bool {
	slash := strings.IndexByte(value, '/')
	if !utf8.ValidString(value) ||
		value == "" ||
		len(value) > 128 ||
		value != strings.ToLower(value) ||
		strings.Count(value, "/") != 1 ||
		slash <= 0 ||
		slash >= len(value)-1 ||
		strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range []byte(value) {
		if !((character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("!#$&^_.+-/", rune(character))) {
			return false
		}
	}
	return true
}

type ComponentCleanupContractV2 struct {
	Ref             ExactRefV1           `json:"ref"`
	OwnerCapability string               `json:"owner_capability"`
	RequestSchema   ComponentSchemaRefV2 `json:"request_schema"`
	ResultSchema    ComponentSchemaRefV2 `json:"result_schema"`
	Digest          DigestV1             `json:"digest"`
}

func componentCleanupContractDigestV2(value ComponentCleanupContractV2) (DigestV1, error) {
	value.Digest = ""
	return DigestJSONV1(struct {
		Domain string                     `json:"domain"`
		Type   string                     `json:"type"`
		Body   ComponentCleanupContractV2 `json:"body"`
	}{"praxis.agent-host.component-factory-v2", "ComponentCleanupContractV2", value})
}

func SealComponentCleanupContractV2(value ComponentCleanupContractV2) (ComponentCleanupContractV2, error) {
	provided := value.Digest
	value.Digest = ""
	digest, err := componentCleanupContractDigestV2(value)
	if err != nil {
		return ComponentCleanupContractV2{}, err
	}
	if provided != "" && provided != digest {
		return ComponentCleanupContractV2{}, NewError(ErrorConflict, "component_cleanup_contract_digest_drift", "component cleanup contract supplied a wrong digest")
	}
	value.Digest = digest
	return value, value.Validate()
}

func (value ComponentCleanupContractV2) Validate() error {
	if err := value.Ref.Validate(); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("cleanup owner capability", value.OwnerCapability); err != nil {
		return err
	}
	if err := value.RequestSchema.Validate(); err != nil {
		return err
	}
	if err := value.ResultSchema.Validate(); err != nil {
		return err
	}
	want, err := componentCleanupContractDigestV2(value)
	if err != nil || want != value.Digest {
		return NewError(ErrorConflict, "component_cleanup_contract_digest_drift", "component cleanup contract digest drifted")
	}
	return nil
}

// ComponentFactoryDescriptorV2 is sealed declaration/admission metadata. Its
// implementation, provider, reference-only, trust and schema fields do not
// prove the source or provenance of the Go value that supplied the descriptor.
type ComponentFactoryDescriptorV2 struct {
	ContractVersion  string                     `json:"contract_version"`
	ObjectKind       string                     `json:"object_kind"`
	Ref              ComponentFactoryRefV2      `json:"ref"`
	ModuleRef        string                     `json:"module_ref"`
	ArtifactDigest   DigestV1                   `json:"artifact_digest"`
	ConstructionMode string                     `json:"construction_mode"`
	InputSchema      ComponentSchemaRefV2       `json:"input_schema"`
	OutputCapability string                     `json:"output_capability"`
	Lifecycle        string                     `json:"lifecycle"`
	CleanupContract  ComponentCleanupContractV2 `json:"cleanup_contract"`
	TrustRef         ExactRefV1                 `json:"trust_ref"`
	Implementation   string                     `json:"implementation"`
	ProviderAccess   string                     `json:"provider_access"`
	ReferenceOnly    bool                       `json:"reference_only"`
	DescriptorDigest DigestV1                   `json:"descriptor_digest"`
}

func componentFactoryDescriptorDigestV2(value ComponentFactoryDescriptorV2) (DigestV1, error) {
	value.Ref.Digest = ""
	value.DescriptorDigest = ""
	return DigestJSONV1(struct {
		Domain string                       `json:"domain"`
		Type   string                       `json:"type"`
		Body   ComponentFactoryDescriptorV2 `json:"body"`
	}{"praxis.agent-host.component-factory-v2", ComponentFactoryDescriptorObjectKindV2, value})
}

func SealComponentFactoryDescriptorV2(value ComponentFactoryDescriptorV2) (ComponentFactoryDescriptorV2, error) {
	if value.ContractVersion != "" && value.ContractVersion != ComponentFactoryContractVersionV2 {
		return ComponentFactoryDescriptorV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "component factory contract version drifted")
	}
	if value.ObjectKind != "" && value.ObjectKind != ComponentFactoryDescriptorObjectKindV2 {
		return ComponentFactoryDescriptorV2{}, NewError(ErrorInvalidArgument, "object_kind_mismatch", "component factory object kind drifted")
	}
	value.ContractVersion = ComponentFactoryContractVersionV2
	value.ObjectKind = ComponentFactoryDescriptorObjectKindV2
	providedRef, providedDescriptor := value.Ref.Digest, value.DescriptorDigest
	value.Ref.Digest, value.DescriptorDigest = "", ""
	digest, err := componentFactoryDescriptorDigestV2(value)
	if err != nil {
		return ComponentFactoryDescriptorV2{}, err
	}
	if (providedRef != "" && providedRef != digest) || (providedDescriptor != "" && providedDescriptor != digest) {
		return ComponentFactoryDescriptorV2{}, NewError(ErrorConflict, "component_factory_descriptor_digest_drift", "component factory descriptor supplied a wrong digest")
	}
	value.Ref.Digest, value.DescriptorDigest = digest, digest
	return value, value.Validate()
}

func (value ComponentFactoryDescriptorV2) Validate() error {
	if value.ContractVersion != ComponentFactoryContractVersionV2 || value.ObjectKind != ComponentFactoryDescriptorObjectKindV2 {
		return NewError(ErrorInvalidArgument, "component_factory_descriptor_discriminator_invalid", "component factory descriptor discriminator is invalid")
	}
	if err := value.Ref.Validate(); err != nil {
		return err
	}
	for field, item := range map[string]string{
		"module ref": value.ModuleRef, "construction mode": value.ConstructionMode,
		"output capability": value.OutputCapability, "lifecycle": value.Lifecycle,
	} {
		if err := ValidateIdentifierV1(field, item); err != nil {
			return err
		}
	}
	if err := value.ArtifactDigest.Validate(); err != nil {
		return err
	}
	if value.ConstructionMode != ComponentFactoryConstructionTrustedGoV2 {
		return NewError(ErrorPrecondition, "component_factory_construction_mode_forbidden", "component factory declaration must select the admitted in-process Go construction mode")
	}
	if err := value.InputSchema.Validate(); err != nil {
		return err
	}
	if err := value.CleanupContract.Validate(); err != nil {
		return err
	}
	if err := value.TrustRef.Validate(); err != nil {
		return err
	}
	if value.Implementation != ComponentFactoryImplementationOwnerV2 || value.ProviderAccess != ComponentFactoryProviderAccessNoneV2 || value.ReferenceOnly {
		return NewError(ErrorPrecondition, "component_factory_not_executable", "component factory declaration is explicitly reference-only, raw-provider, or non-owner")
	}
	want, err := componentFactoryDescriptorDigestV2(value)
	if err != nil || want != value.Ref.Digest || want != value.DescriptorDigest {
		return NewError(ErrorConflict, "component_factory_descriptor_digest_drift", "component factory descriptor digest drifted")
	}
	return nil
}

type ComponentFactoryConformanceCurrentRefV2 struct {
	ConformanceID   string   `json:"conformance_id"`
	Revision        uint64   `json:"revision"`
	Digest          DigestV1 `json:"digest"`
	ExpiresUnixNano int64    `json:"expires_unix_nano"`
}

func (ref ComponentFactoryConformanceCurrentRefV2) Validate() error {
	if err := ValidateIdentifierV1("component factory conformance id", ref.ConformanceID); err != nil {
		return err
	}
	if ref.Revision == 0 || ref.ExpiresUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "component_factory_conformance_ref_incomplete", "component factory conformance Ref is incomplete")
	}
	return ref.Digest.Validate()
}

// ComponentFactoryEvidenceCurrentV2 seals an Owner-published evidence
// declaration and validity window. It is not implementation provenance.
type ComponentFactoryEvidenceCurrentV2 struct {
	Ref             ExactRefV1 `json:"ref"`
	CheckedUnixNano int64      `json:"checked_unix_nano"`
	ExpiresUnixNano int64      `json:"expires_unix_nano"`
	Digest          DigestV1   `json:"digest"`
}

func componentFactoryEvidenceCurrentDigestV2(value ComponentFactoryEvidenceCurrentV2) (DigestV1, error) {
	value.Digest = ""
	return DigestJSONV1(struct {
		Domain string                            `json:"domain"`
		Type   string                            `json:"type"`
		Body   ComponentFactoryEvidenceCurrentV2 `json:"body"`
	}{"praxis.agent-host.component-factory-v2", "ComponentFactoryEvidenceCurrentV2", value})
}

func SealComponentFactoryEvidenceCurrentV2(value ComponentFactoryEvidenceCurrentV2) (ComponentFactoryEvidenceCurrentV2, error) {
	provided := value.Digest
	value.Digest = ""
	digest, err := componentFactoryEvidenceCurrentDigestV2(value)
	if err != nil {
		return ComponentFactoryEvidenceCurrentV2{}, err
	}
	if provided != "" && provided != digest {
		return ComponentFactoryEvidenceCurrentV2{}, NewError(ErrorConflict, "component_factory_evidence_digest_drift", "component factory evidence supplied a wrong digest")
	}
	value.Digest = digest
	return value, value.Validate()
}

func (value ComponentFactoryEvidenceCurrentV2) Validate() error {
	if err := value.Ref.Validate(); err != nil {
		return err
	}
	if value.CheckedUnixNano <= 0 || value.ExpiresUnixNano <= value.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "component_factory_evidence_incomplete", "component factory evidence current is incomplete")
	}
	want, err := componentFactoryEvidenceCurrentDigestV2(value)
	if err != nil || want != value.Digest {
		return NewError(ErrorConflict, "component_factory_evidence_digest_drift", "component factory evidence digest drifted")
	}
	return nil
}

func (value ComponentFactoryEvidenceCurrentV2) ValidateCurrent(now time.Time) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < value.CheckedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "component factory evidence clock regressed")
	}
	if now.UnixNano() >= value.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "component_factory_evidence_expired", "component factory evidence expired")
	}
	return nil
}

// ComponentFactoryConformanceCurrentV2 is sealed admission metadata. Its
// implementation/provider/reference-only fields and evidence Refs are
// declarations, not verified implementation provenance.
type ComponentFactoryConformanceCurrentV2 struct {
	ContractVersion       string                                  `json:"contract_version"`
	ObjectKind            string                                  `json:"object_kind"`
	Ref                   ComponentFactoryConformanceCurrentRefV2 `json:"ref"`
	FactoryRef            ComponentFactoryRefV2                   `json:"factory_ref"`
	DescriptorDigest      DigestV1                                `json:"descriptor_digest"`
	Certification         ComponentFactoryEvidenceCurrentV2       `json:"certification"`
	StaticImportEvidence  ComponentFactoryEvidenceCurrentV2       `json:"static_import_evidence"`
	NoRawProviderEvidence ComponentFactoryEvidenceCurrentV2       `json:"no_raw_provider_evidence"`
	ZeroEffectEvidence    ComponentFactoryEvidenceCurrentV2       `json:"zero_effect_evidence"`
	Implementation        string                                  `json:"implementation"`
	ProviderAccess        string                                  `json:"provider_access"`
	ReferenceOnly         bool                                    `json:"reference_only"`
	CheckedUnixNano       int64                                   `json:"checked_unix_nano"`
	ExpiresUnixNano       int64                                   `json:"expires_unix_nano"`
	ProjectionDigest      DigestV1                                `json:"projection_digest"`
}

func componentFactoryConformanceDigestV2(value ComponentFactoryConformanceCurrentV2) (DigestV1, error) {
	value.Ref.Digest = ""
	value.ProjectionDigest = ""
	return DigestJSONV1(struct {
		Domain string                               `json:"domain"`
		Type   string                               `json:"type"`
		Body   ComponentFactoryConformanceCurrentV2 `json:"body"`
	}{"praxis.agent-host.component-factory-v2", ComponentFactoryConformanceObjectKindV2, value})
}

func SealComponentFactoryConformanceCurrentV2(value ComponentFactoryConformanceCurrentV2) (ComponentFactoryConformanceCurrentV2, error) {
	value.ContractVersion = ComponentFactoryContractVersionV2
	value.ObjectKind = ComponentFactoryConformanceObjectKindV2
	providedRef, providedProjection := value.Ref.Digest, value.ProjectionDigest
	value.Ref.Digest, value.ProjectionDigest = "", ""
	digest, err := componentFactoryConformanceDigestV2(value)
	if err != nil {
		return ComponentFactoryConformanceCurrentV2{}, err
	}
	if (providedRef != "" && providedRef != digest) || (providedProjection != "" && providedProjection != digest) {
		return ComponentFactoryConformanceCurrentV2{}, NewError(ErrorConflict, "component_factory_conformance_digest_drift", "component factory conformance supplied a wrong digest")
	}
	value.Ref.Digest, value.ProjectionDigest = digest, digest
	return value, value.Validate()
}

func (value ComponentFactoryConformanceCurrentV2) Validate() error {
	if value.ContractVersion != ComponentFactoryContractVersionV2 || value.ObjectKind != ComponentFactoryConformanceObjectKindV2 {
		return NewError(ErrorInvalidArgument, "component_factory_conformance_discriminator_invalid", "component factory conformance discriminator is invalid")
	}
	if err := value.Ref.Validate(); err != nil {
		return err
	}
	if err := value.FactoryRef.Validate(); err != nil {
		return err
	}
	if value.DescriptorDigest != value.FactoryRef.Digest {
		return NewError(ErrorConflict, "component_factory_conformance_descriptor_drift", "component factory conformance descriptor digest drifted")
	}
	minimum := int64(0)
	for _, evidence := range []ComponentFactoryEvidenceCurrentV2{
		value.Certification, value.StaticImportEvidence, value.NoRawProviderEvidence, value.ZeroEffectEvidence,
	} {
		if err := evidence.Validate(); err != nil {
			return err
		}
		if value.CheckedUnixNano < evidence.CheckedUnixNano {
			return NewError(ErrorPrecondition, "component_factory_conformance_clock_regression", "component factory conformance predates its evidence current")
		}
		if minimum == 0 || evidence.ExpiresUnixNano < minimum {
			minimum = evidence.ExpiresUnixNano
		}
	}
	if value.CheckedUnixNano <= 0 || value.ExpiresUnixNano <= value.CheckedUnixNano ||
		value.Ref.ExpiresUnixNano != value.ExpiresUnixNano || value.ExpiresUnixNano > minimum {
		return NewError(ErrorPrecondition, "component_factory_conformance_ttl_drift", "component factory conformance validity window drifted")
	}
	if value.Implementation != ComponentFactoryImplementationOwnerV2 || value.ProviderAccess != ComponentFactoryProviderAccessNoneV2 || value.ReferenceOnly {
		return NewError(ErrorPrecondition, "component_factory_conformance_not_executable", "component factory conformance declaration is explicitly reference-only, raw-provider, or non-owner")
	}
	want, err := componentFactoryConformanceDigestV2(value)
	if err != nil || want != value.Ref.Digest || want != value.ProjectionDigest {
		return NewError(ErrorConflict, "component_factory_conformance_digest_drift", "component factory conformance digest drifted")
	}
	return nil
}

func (value ComponentFactoryConformanceCurrentV2) ValidateCurrent(expected ComponentFactoryConformanceCurrentRefV2, now time.Time) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Ref != expected {
		return NewError(ErrorConflict, "component_factory_conformance_ref_drift", "component factory conformance exact Ref drifted")
	}
	if now.IsZero() || now.UnixNano() < value.CheckedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "component factory conformance clock regressed")
	}
	if now.UnixNano() >= value.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "component_factory_conformance_expired", "component factory conformance expired")
	}
	return nil
}

type ComponentResourceRefV2 struct {
	OwnerDomain     string   `json:"owner_domain"`
	OwnerID         string   `json:"owner_id"`
	ID              string   `json:"id"`
	Revision        uint64   `json:"revision"`
	Digest          DigestV1 `json:"digest"`
	Kind            string   `json:"kind"`
	ScopeDigest     DigestV1 `json:"scope_digest"`
	ExpiresUnixNano int64    `json:"expires_unix_nano"`
}

func (ref ComponentResourceRefV2) Validate() error {
	for field, value := range map[string]string{
		"resource owner domain": ref.OwnerDomain, "resource owner id": ref.OwnerID,
		"resource id": ref.ID, "resource kind": ref.Kind,
	} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	if ref.Revision == 0 || ref.ExpiresUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "component_resource_ref_incomplete", "component resource Ref is incomplete")
	}
	if err := ref.Digest.Validate(); err != nil {
		return err
	}
	return ref.ScopeDigest.Validate()
}

type ComponentInstanceRefV2 struct {
	OwnerID         string   `json:"owner_id"`
	InstanceID      string   `json:"instance_id"`
	Revision        uint64   `json:"revision"`
	Digest          DigestV1 `json:"digest"`
	Capability      string   `json:"capability"`
	ExpiresUnixNano int64    `json:"expires_unix_nano"`
}

func (ref ComponentInstanceRefV2) Validate() error {
	for field, value := range map[string]string{
		"component instance owner": ref.OwnerID, "component instance id": ref.InstanceID, "component capability": ref.Capability,
	} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	if ref.Revision == 0 || ref.ExpiresUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "component_instance_ref_incomplete", "component instance Ref is incomplete")
	}
	return ref.Digest.Validate()
}

// ComponentFactoryAttemptRefV2 is the stable create-once coordinate used by
// both StartOrInspect and Inspect. It is never a caller-selected retry key.
type ComponentFactoryAttemptRefV2 struct {
	AttemptID       string                `json:"attempt_id"`
	Revision        uint64                `json:"revision"`
	Digest          DigestV1              `json:"digest"`
	FactoryRef      ComponentFactoryRefV2 `json:"factory_ref"`
	RequestDigest   DigestV1              `json:"request_digest"`
	ExpiresUnixNano int64                 `json:"expires_unix_nano"`
}

func componentFactoryAttemptRefDigestV2(ref ComponentFactoryAttemptRefV2) (DigestV1, error) {
	ref.Digest = ""
	return DigestJSONV1(struct {
		Domain string                       `json:"domain"`
		Type   string                       `json:"type"`
		Body   ComponentFactoryAttemptRefV2 `json:"body"`
	}{"praxis.agent-host.component-factory-v2", "ComponentFactoryAttemptRefV2", ref})
}

func (ref ComponentFactoryAttemptRefV2) Validate() error {
	if err := ValidateIdentifierV1("component factory attempt id", ref.AttemptID); err != nil {
		return err
	}
	if !strings.HasPrefix(ref.AttemptID, ComponentFactoryStartAttemptPrefixV2) {
		return NewError(ErrorInvalidArgument, "component_factory_attempt_id_invalid", "component factory AttemptID is not a derived Start identity")
	}
	identityDigest := DigestV1(strings.TrimPrefix(ref.AttemptID, ComponentFactoryStartAttemptPrefixV2))
	if err := identityDigest.Validate(); err != nil {
		return err
	}
	if ref.Revision != 1 || ref.ExpiresUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "component_factory_attempt_ref_incomplete", "component factory attempt Ref is incomplete")
	}
	if err := ref.FactoryRef.Validate(); err != nil {
		return err
	}
	if err := ref.RequestDigest.Validate(); err != nil {
		return err
	}
	if err := ref.Digest.Validate(); err != nil {
		return err
	}
	want, err := componentFactoryAttemptRefDigestV2(ref)
	if err != nil || want != ref.Digest {
		return NewError(ErrorConflict, "component_factory_attempt_ref_digest_drift", "component factory attempt Ref digest drifted")
	}
	return nil
}

type ComponentStartRequestV2 struct {
	ContractVersion           string                                `json:"contract_version"`
	HostID                    string                                `json:"host_id"`
	StartID                   string                                `json:"start_id"`
	Attempt                   ComponentFactoryAttemptRefV2          `json:"attempt"`
	Preflight                 ComponentFactoryPreflightReceiptRefV2 `json:"preflight"`
	Descriptor                ComponentFactoryDescriptorV2          `json:"descriptor"`
	ResourceRefs              []ComponentResourceRefV2              `json:"resource_refs"`
	DependencyRefs            []ComponentInstanceRefV2              `json:"dependency_refs"`
	RequestedUnixNano         int64                                 `json:"requested_unix_nano"`
	RequestedNotAfterUnixNano int64                                 `json:"requested_not_after_unix_nano"`
	RequestDigest             DigestV1                              `json:"request_digest"`
}

func componentStartRequestDigestV2(value ComponentStartRequestV2) (DigestV1, error) {
	value.ResourceRefs = canonicalComponentResourceRefsV2(value.ResourceRefs)
	value.DependencyRefs = canonicalComponentDependencyRefsV2(value.DependencyRefs)
	value.Attempt.Digest = ""
	value.Attempt.RequestDigest = ""
	value.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string                  `json:"domain"`
		Type   string                  `json:"type"`
		Body   ComponentStartRequestV2 `json:"body"`
	}{"praxis.agent-host.component-factory-v2", "ComponentStartRequestV2", value})
}

func componentStartAttemptIdentityDigestV2(value ComponentStartRequestV2) (DigestV1, error) {
	value.ResourceRefs = canonicalComponentResourceRefsV2(value.ResourceRefs)
	value.DependencyRefs = canonicalComponentDependencyRefsV2(value.DependencyRefs)
	value.Attempt.AttemptID = ""
	value.Attempt.Digest = ""
	value.Attempt.RequestDigest = ""
	value.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string                  `json:"domain"`
		Type   string                  `json:"type"`
		Body   ComponentStartRequestV2 `json:"body"`
	}{"praxis.agent-host.component-factory-start-attempt-v2", "ComponentStartRequestV2", value})
}

// DeriveComponentFactoryStartAttemptIDV2 binds one create-once coordinate to
// the complete canonical Start payload without introducing a Host-side Store.
func DeriveComponentFactoryStartAttemptIDV2(value ComponentStartRequestV2) (string, error) {
	digest, err := componentStartAttemptIdentityDigestV2(value)
	if err != nil {
		return "", err
	}
	return ComponentFactoryStartAttemptPrefixV2 + string(digest), nil
}

func SealComponentStartRequestV2(value ComponentStartRequestV2) (ComponentStartRequestV2, error) {
	value.ContractVersion = ComponentFactoryContractVersionV2
	value.ResourceRefs = canonicalComponentResourceRefsV2(value.ResourceRefs)
	value.DependencyRefs = canonicalComponentDependencyRefsV2(value.DependencyRefs)
	providedRequest := value.RequestDigest
	providedAttemptRequest := value.Attempt.RequestDigest
	providedAttemptID := value.Attempt.AttemptID
	providedAttemptDigest := value.Attempt.Digest
	value.RequestDigest, value.Attempt.RequestDigest, value.Attempt.Digest = "", "", ""
	attemptID, err := DeriveComponentFactoryStartAttemptIDV2(value)
	if err != nil {
		return ComponentStartRequestV2{}, err
	}
	if providedAttemptID != "" && providedAttemptID != attemptID {
		return ComponentStartRequestV2{}, NewError(ErrorConflict, "component_start_attempt_payload_conflict", "component Start AttemptID belongs to another canonical request payload")
	}
	value.Attempt.AttemptID = attemptID
	digest, err := componentStartRequestDigestV2(value)
	if err != nil {
		return ComponentStartRequestV2{}, err
	}
	if (providedRequest != "" && providedRequest != digest) ||
		(providedAttemptRequest != "" && providedAttemptRequest != digest) {
		return ComponentStartRequestV2{}, NewError(ErrorConflict, "component_start_request_digest_drift", "component start request supplied a wrong digest")
	}
	value.RequestDigest, value.Attempt.RequestDigest = digest, digest
	attemptDigest, err := componentFactoryAttemptRefDigestV2(value.Attempt)
	if err != nil {
		return ComponentStartRequestV2{}, err
	}
	if providedAttemptDigest != "" && providedAttemptDigest != attemptDigest {
		return ComponentStartRequestV2{}, NewError(ErrorConflict, "component_factory_attempt_ref_digest_drift", "component factory Attempt supplied a wrong exact digest")
	}
	value.Attempt.Digest = attemptDigest
	return value, value.Validate()
}

func (value ComponentStartRequestV2) Validate() error {
	if value.ContractVersion != ComponentFactoryContractVersionV2 {
		return NewError(ErrorInvalidArgument, "component_start_contract_version_invalid", "component start request contract version is invalid")
	}
	for field, item := range map[string]string{"host id": value.HostID, "start id": value.StartID} {
		if err := ValidateIdentifierV1(field, item); err != nil {
			return err
		}
	}
	if err := value.Attempt.Validate(); err != nil {
		return err
	}
	if err := value.Preflight.Validate(); err != nil {
		return err
	}
	if err := value.Descriptor.Validate(); err != nil {
		return err
	}
	expectedAttemptID, err := DeriveComponentFactoryStartAttemptIDV2(value)
	if err != nil {
		return err
	}
	if value.Attempt.AttemptID != expectedAttemptID {
		return NewError(ErrorConflict, "component_start_attempt_payload_conflict", "component Start AttemptID belongs to another canonical request payload")
	}
	if value.Attempt.FactoryRef != value.Descriptor.Ref || value.Attempt.RequestDigest != value.RequestDigest {
		return NewError(ErrorConflict, "component_start_attempt_drift", "component start attempt does not bind the request")
	}
	if value.ResourceRefs == nil || value.DependencyRefs == nil ||
		len(value.ResourceRefs) > MaxComponentFactoryResourcesV2 ||
		len(value.DependencyRefs) > MaxComponentFactoryDependenciesV2 {
		return NewError(ErrorInvalidArgument, "component_start_reference_count_invalid", "component start exact references are incomplete")
	}
	canonicalResources := canonicalComponentResourceRefsV2(value.ResourceRefs)
	canonicalDependencies := canonicalComponentDependencyRefsV2(value.DependencyRefs)
	if !equalComponentResourceRefsV2(value.ResourceRefs, canonicalResources) ||
		!equalComponentDependencyRefsV2(value.DependencyRefs, canonicalDependencies) {
		return NewError(ErrorConflict, "component_start_references_not_canonical", "component start exact references are not canonical")
	}
	for index, ref := range value.ResourceRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
		if index > 0 && sameComponentResourceIdentityV2(value.ResourceRefs[index-1], ref) {
			return NewError(ErrorConflict, "component_start_resource_duplicate", "component start duplicates a Resource exact Ref")
		}
	}
	for index, ref := range value.DependencyRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
		if index > 0 &&
			value.DependencyRefs[index-1].OwnerID == ref.OwnerID &&
			value.DependencyRefs[index-1].InstanceID == ref.InstanceID {
			return NewError(ErrorConflict, "component_start_dependency_duplicate", "component start duplicates a Dependency exact Ref")
		}
	}
	if value.RequestedUnixNano <= 0 || value.RequestedNotAfterUnixNano <= value.RequestedUnixNano ||
		value.RequestedNotAfterUnixNano > value.Preflight.ExpiresUnixNano ||
		value.RequestedNotAfterUnixNano > value.Attempt.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "component_start_ttl_drift", "component start validity window drifted")
	}
	want, err := componentStartRequestDigestV2(value)
	if err != nil || want != value.RequestDigest {
		return NewError(ErrorConflict, "component_start_request_digest_drift", "component start request digest drifted")
	}
	return nil
}

type ComponentInstanceV2 struct {
	ContractVersion  string                       `json:"contract_version"`
	Ref              ComponentInstanceRefV2       `json:"ref"`
	FactoryRef       ComponentFactoryRefV2        `json:"factory_ref"`
	AttemptRef       ComponentFactoryAttemptRefV2 `json:"attempt_ref"`
	InspectBinding   ExactRefV1                   `json:"inspect_binding"`
	CleanupBinding   ExactRefV1                   `json:"cleanup_binding"`
	CheckedUnixNano  int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                        `json:"expires_unix_nano"`
	ProjectionDigest DigestV1                     `json:"projection_digest"`
}

func componentInstanceDigestV2(value ComponentInstanceV2) (DigestV1, error) {
	value.Ref.Digest, value.ProjectionDigest = "", ""
	return DigestJSONV1(struct {
		Domain string              `json:"domain"`
		Type   string              `json:"type"`
		Body   ComponentInstanceV2 `json:"body"`
	}{"praxis.agent-host.component-factory-v2", "ComponentInstanceV2", value})
}

func SealComponentInstanceV2(value ComponentInstanceV2) (ComponentInstanceV2, error) {
	value.ContractVersion = ComponentFactoryContractVersionV2
	providedRef, providedProjection := value.Ref.Digest, value.ProjectionDigest
	value.Ref.Digest, value.ProjectionDigest = "", ""
	digest, err := componentInstanceDigestV2(value)
	if err != nil {
		return ComponentInstanceV2{}, err
	}
	if (providedRef != "" && providedRef != digest) || (providedProjection != "" && providedProjection != digest) {
		return ComponentInstanceV2{}, NewError(ErrorConflict, "component_instance_digest_drift", "component instance supplied a wrong digest")
	}
	value.Ref.Digest, value.ProjectionDigest = digest, digest
	return value, value.Validate()
}

func (value ComponentInstanceV2) Validate() error {
	if value.ContractVersion != ComponentFactoryContractVersionV2 {
		return NewError(ErrorInvalidArgument, "component_instance_version_invalid", "component instance contract version is invalid")
	}
	if err := value.Ref.Validate(); err != nil {
		return err
	}
	if err := value.FactoryRef.Validate(); err != nil {
		return err
	}
	if err := value.AttemptRef.Validate(); err != nil {
		return err
	}
	if err := value.InspectBinding.Validate(); err != nil {
		return err
	}
	if err := value.CleanupBinding.Validate(); err != nil {
		return err
	}
	if value.AttemptRef.FactoryRef != value.FactoryRef || value.CheckedUnixNano <= 0 ||
		value.ExpiresUnixNano <= value.CheckedUnixNano || value.Ref.ExpiresUnixNano != value.ExpiresUnixNano ||
		value.ExpiresUnixNano > value.AttemptRef.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "component_instance_closure_drift", "component instance closure drifted")
	}
	want, err := componentInstanceDigestV2(value)
	if err != nil || want != value.Ref.Digest || want != value.ProjectionDigest {
		return NewError(ErrorConflict, "component_instance_digest_drift", "component instance digest drifted")
	}
	return nil
}

type ComponentDependencyCurrentV2 struct {
	ContractVersion  string                 `json:"contract_version"`
	Ref              ComponentInstanceRefV2 `json:"ref"`
	InspectBinding   ExactRefV1             `json:"inspect_binding"`
	CleanupBinding   ExactRefV1             `json:"cleanup_binding"`
	CheckedUnixNano  int64                  `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                  `json:"expires_unix_nano"`
	ProjectionDigest DigestV1               `json:"projection_digest"`
}

func componentDependencyCurrentDigestV2(value ComponentDependencyCurrentV2) (DigestV1, error) {
	value.Ref.Digest, value.ProjectionDigest = "", ""
	return DigestJSONV1(struct {
		Domain string                       `json:"domain"`
		Type   string                       `json:"type"`
		Body   ComponentDependencyCurrentV2 `json:"body"`
	}{"praxis.agent-host.component-factory-v2", "ComponentDependencyCurrentV2", value})
}

func SealComponentDependencyCurrentV2(value ComponentDependencyCurrentV2) (ComponentDependencyCurrentV2, error) {
	value.ContractVersion = ComponentFactoryContractVersionV2
	providedRef, providedProjection := value.Ref.Digest, value.ProjectionDigest
	value.Ref.Digest, value.ProjectionDigest = "", ""
	digest, err := componentDependencyCurrentDigestV2(value)
	if err != nil {
		return ComponentDependencyCurrentV2{}, err
	}
	if (providedRef != "" && providedRef != digest) || (providedProjection != "" && providedProjection != digest) {
		return ComponentDependencyCurrentV2{}, NewError(ErrorConflict, "component_dependency_digest_drift", "component dependency current supplied a wrong digest")
	}
	value.Ref.Digest, value.ProjectionDigest = digest, digest
	return value, value.Validate()
}

func (value ComponentDependencyCurrentV2) Validate() error {
	if value.ContractVersion != ComponentFactoryContractVersionV2 {
		return NewError(ErrorInvalidArgument, "component_dependency_version_invalid", "component dependency current version is invalid")
	}
	if err := value.Ref.Validate(); err != nil {
		return err
	}
	if err := value.InspectBinding.Validate(); err != nil {
		return err
	}
	if err := value.CleanupBinding.Validate(); err != nil {
		return err
	}
	if value.CheckedUnixNano <= 0 || value.ExpiresUnixNano <= value.CheckedUnixNano || value.Ref.ExpiresUnixNano != value.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "component_dependency_ttl_drift", "component dependency current validity window drifted")
	}
	want, err := componentDependencyCurrentDigestV2(value)
	if err != nil || want != value.Ref.Digest || want != value.ProjectionDigest {
		return NewError(ErrorConflict, "component_dependency_digest_drift", "component dependency current digest drifted")
	}
	return nil
}

func (value ComponentDependencyCurrentV2) ValidateCurrent(expected ComponentInstanceRefV2, now time.Time) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Ref != expected {
		return NewError(ErrorConflict, "component_dependency_ref_drift", "component dependency exact Ref drifted")
	}
	if now.IsZero() || now.UnixNano() < value.CheckedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "component dependency clock regressed")
	}
	if now.UnixNano() >= value.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "component_dependency_expired", "component dependency expired")
	}
	return nil
}

type ComponentFactoryRegistryKeyV2 struct {
	FactoryRef        ComponentFactoryRefV2                   `json:"factory_ref"`
	ConformanceRef    ComponentFactoryConformanceCurrentRefV2 `json:"conformance_ref"`
	ModuleRef         string                                  `json:"module_ref"`
	ArtifactDigest    DigestV1                                `json:"artifact_digest"`
	OutputCapability  string                                  `json:"output_capability"`
	InputSchemaDigest DigestV1                                `json:"input_schema_digest"`
	CleanupDigest     DigestV1                                `json:"cleanup_digest"`
	TrustDigest       DigestV1                                `json:"trust_digest"`
	Digest            DigestV1                                `json:"digest"`
}

func componentFactoryRegistryKeyDigestV2(value ComponentFactoryRegistryKeyV2) (DigestV1, error) {
	value.Digest = ""
	return DigestJSONV1(struct {
		Domain string                        `json:"domain"`
		Type   string                        `json:"type"`
		Body   ComponentFactoryRegistryKeyV2 `json:"body"`
	}{"praxis.agent-host.component-factory-v2", "ComponentFactoryRegistryKeyV2", value})
}

func SealComponentFactoryRegistryKeyV2(descriptor ComponentFactoryDescriptorV2, conformance ComponentFactoryConformanceCurrentV2) (ComponentFactoryRegistryKeyV2, error) {
	if err := descriptor.Validate(); err != nil {
		return ComponentFactoryRegistryKeyV2{}, err
	}
	if err := conformance.Validate(); err != nil {
		return ComponentFactoryRegistryKeyV2{}, err
	}
	if conformance.FactoryRef != descriptor.Ref || conformance.DescriptorDigest != descriptor.DescriptorDigest {
		return ComponentFactoryRegistryKeyV2{}, NewError(ErrorConflict, "component_factory_registry_conformance_drift", "component factory registry descriptor and conformance differ")
	}
	value := ComponentFactoryRegistryKeyV2{
		FactoryRef: descriptor.Ref, ConformanceRef: conformance.Ref, ModuleRef: descriptor.ModuleRef,
		ArtifactDigest: descriptor.ArtifactDigest, OutputCapability: descriptor.OutputCapability,
		InputSchemaDigest: descriptor.InputSchema.ContentDigest, CleanupDigest: descriptor.CleanupContract.Digest,
		TrustDigest: descriptor.TrustRef.Digest,
	}
	digest, err := componentFactoryRegistryKeyDigestV2(value)
	if err != nil {
		return ComponentFactoryRegistryKeyV2{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

func (value ComponentFactoryRegistryKeyV2) Validate() error {
	if err := value.FactoryRef.Validate(); err != nil {
		return err
	}
	if err := value.ConformanceRef.Validate(); err != nil {
		return err
	}
	for field, item := range map[string]string{"module ref": value.ModuleRef, "output capability": value.OutputCapability} {
		if err := ValidateIdentifierV1(field, item); err != nil {
			return err
		}
	}
	for _, digest := range []DigestV1{value.ArtifactDigest, value.InputSchemaDigest, value.CleanupDigest, value.TrustDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	want, err := componentFactoryRegistryKeyDigestV2(value)
	if err != nil || want != value.Digest {
		return NewError(ErrorConflict, "component_factory_registry_key_drift", "component factory registry key digest drifted")
	}
	return nil
}

type ComponentFactoryRegistrationV2 struct {
	ContractVersion    string                                  `json:"contract_version"`
	ObjectKind         string                                  `json:"object_kind"`
	Key                ComponentFactoryRegistryKeyV2           `json:"key"`
	Descriptor         ComponentFactoryDescriptorV2            `json:"descriptor"`
	ConformanceRef     ComponentFactoryConformanceCurrentRefV2 `json:"conformance_ref"`
	ProductionEligible bool                                    `json:"production_eligible"`
	Digest             DigestV1                                `json:"digest"`
}

func componentFactoryRegistrationDigestV2(value ComponentFactoryRegistrationV2) (DigestV1, error) {
	value.Digest = ""
	return DigestJSONV1(struct {
		Domain string                         `json:"domain"`
		Type   string                         `json:"type"`
		Body   ComponentFactoryRegistrationV2 `json:"body"`
	}{"praxis.agent-host.component-factory-v2", ComponentFactoryRegistrationObjectKindV2, value})
}

func SealComponentFactoryRegistrationV2(descriptor ComponentFactoryDescriptorV2, conformance ComponentFactoryConformanceCurrentV2) (ComponentFactoryRegistrationV2, error) {
	key, err := SealComponentFactoryRegistryKeyV2(descriptor, conformance)
	if err != nil {
		return ComponentFactoryRegistrationV2{}, err
	}
	value := ComponentFactoryRegistrationV2{
		ContractVersion: ComponentFactoryContractVersionV2, ObjectKind: ComponentFactoryRegistrationObjectKindV2,
		Key: key, Descriptor: descriptor, ConformanceRef: conformance.Ref,
	}
	digest, err := componentFactoryRegistrationDigestV2(value)
	if err != nil {
		return ComponentFactoryRegistrationV2{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

func (value ComponentFactoryRegistrationV2) Validate() error {
	if value.ContractVersion != ComponentFactoryContractVersionV2 || value.ObjectKind != ComponentFactoryRegistrationObjectKindV2 {
		return NewError(ErrorInvalidArgument, "component_factory_registration_discriminator_invalid", "component factory registration discriminator is invalid")
	}
	if err := value.Key.Validate(); err != nil {
		return err
	}
	if err := value.Descriptor.Validate(); err != nil {
		return err
	}
	if value.ProductionEligible {
		return NewError(ErrorPrecondition, "component_factory_production_provenance_unavailable", "component factory registration metadata cannot establish production eligibility")
	}
	if value.Key.FactoryRef != value.Descriptor.Ref || value.Key.ConformanceRef != value.ConformanceRef ||
		value.Key.ModuleRef != value.Descriptor.ModuleRef || value.Key.ArtifactDigest != value.Descriptor.ArtifactDigest ||
		value.Key.OutputCapability != value.Descriptor.OutputCapability || value.Key.InputSchemaDigest != value.Descriptor.InputSchema.ContentDigest ||
		value.Key.CleanupDigest != value.Descriptor.CleanupContract.Digest || value.Key.TrustDigest != value.Descriptor.TrustRef.Digest {
		return NewError(ErrorConflict, "component_factory_registration_closure_drift", "component factory registration closure drifted")
	}
	want, err := componentFactoryRegistrationDigestV2(value)
	if err != nil || want != value.Digest {
		return NewError(ErrorConflict, "component_factory_registration_digest_drift", "component factory registration digest drifted")
	}
	return nil
}

type ComponentFactoryPreflightRequestV2 struct {
	ContractVersion           string                        `json:"contract_version"`
	ObjectKind                string                        `json:"object_kind"`
	HostID                    string                        `json:"host_id"`
	StartID                   string                        `json:"start_id"`
	DeploymentRef             HostDeploymentCurrentRefV2    `json:"deployment_ref"`
	RegistryKey               ComponentFactoryRegistryKeyV2 `json:"registry_key"`
	ResourceRefs              []ComponentResourceRefV2      `json:"resource_refs"`
	DependencyRefs            []ComponentInstanceRefV2      `json:"dependency_refs"`
	AttemptID                 string                        `json:"attempt_id"`
	RequestedUnixNano         int64                         `json:"requested_unix_nano"`
	RequestedNotAfterUnixNano int64                         `json:"requested_not_after_unix_nano"`
	RequestDigest             DigestV1                      `json:"request_digest"`
}

func canonicalComponentResourceRefsV2(values []ComponentResourceRefV2) []ComponentResourceRefV2 {
	result := append([]ComponentResourceRefV2{}, values...)
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.OwnerDomain != right.OwnerDomain {
			return left.OwnerDomain < right.OwnerDomain
		}
		if left.OwnerID != right.OwnerID {
			return left.OwnerID < right.OwnerID
		}
		return left.ID < right.ID
	})
	return result
}

func canonicalComponentDependencyRefsV2(values []ComponentInstanceRefV2) []ComponentInstanceRefV2 {
	result := append([]ComponentInstanceRefV2{}, values...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].OwnerID != result[j].OwnerID {
			return result[i].OwnerID < result[j].OwnerID
		}
		return result[i].InstanceID < result[j].InstanceID
	})
	return result
}

func (value ComponentFactoryPreflightRequestV2) canonicalV2() ComponentFactoryPreflightRequestV2 {
	value.ResourceRefs = canonicalComponentResourceRefsV2(value.ResourceRefs)
	value.DependencyRefs = canonicalComponentDependencyRefsV2(value.DependencyRefs)
	return value
}

func componentFactoryPreflightRequestDigestV2(value ComponentFactoryPreflightRequestV2) (DigestV1, error) {
	identityDigest, err := componentFactoryPreflightAttemptIdentityDigestV2(value)
	if err != nil {
		return "", err
	}
	expectedAttempt := ComponentFactoryPreflightAttemptPrefixV2 + string(identityDigest)
	if value.AttemptID != expectedAttempt {
		return "", NewError(ErrorConflict, "component_factory_preflight_attempt_payload_conflict", "component factory preflight AttemptID belongs to another request payload")
	}
	return DigestJSONV1(struct {
		Domain         string   `json:"domain"`
		Type           string   `json:"type"`
		AttemptID      string   `json:"attempt_id"`
		IdentityDigest DigestV1 `json:"identity_digest"`
	}{"praxis.agent-host.component-factory-v2", ComponentFactoryPreflightRequestKindV2, value.AttemptID, identityDigest})
}

func componentFactoryPreflightAttemptIdentityDigestV2(value ComponentFactoryPreflightRequestV2) (DigestV1, error) {
	value = value.canonicalV2()
	value.AttemptID = ""
	value.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string                             `json:"domain"`
		Type   string                             `json:"type"`
		Body   ComponentFactoryPreflightRequestV2 `json:"body"`
	}{"praxis.agent-host.component-factory-preflight-attempt-v2", ComponentFactoryPreflightRequestKindV2, value})
}

// DeriveComponentFactoryPreflightAttemptIDV2 closes the pure preflight
// idempotency domain without introducing a second Store. Reusing an AttemptID
// with another resource, dependency, registry or validity window therefore
// fails before any Owner reader is called.
func DeriveComponentFactoryPreflightAttemptIDV2(value ComponentFactoryPreflightRequestV2) (string, error) {
	digest, err := componentFactoryPreflightAttemptIdentityDigestV2(value)
	if err != nil {
		return "", err
	}
	return ComponentFactoryPreflightAttemptPrefixV2 + string(digest), nil
}

func componentFactoryPreflightRequestDigestFromAttemptIDV2(attemptID string) (DigestV1, error) {
	if !strings.HasPrefix(attemptID, ComponentFactoryPreflightAttemptPrefixV2) {
		return "", NewError(ErrorInvalidArgument, "component_factory_preflight_attempt_invalid", "component factory preflight AttemptID has an invalid identity prefix")
	}
	identityDigest := DigestV1(strings.TrimPrefix(attemptID, ComponentFactoryPreflightAttemptPrefixV2))
	if err := identityDigest.Validate(); err != nil {
		return "", err
	}
	return DigestJSONV1(struct {
		Domain         string   `json:"domain"`
		Type           string   `json:"type"`
		AttemptID      string   `json:"attempt_id"`
		IdentityDigest DigestV1 `json:"identity_digest"`
	}{"praxis.agent-host.component-factory-v2", ComponentFactoryPreflightRequestKindV2, attemptID, identityDigest})
}

func SealComponentFactoryPreflightRequestV2(value ComponentFactoryPreflightRequestV2) (ComponentFactoryPreflightRequestV2, error) {
	value.ContractVersion = ComponentFactoryContractVersionV2
	value.ObjectKind = ComponentFactoryPreflightRequestKindV2
	value = value.canonicalV2()
	providedAttempt := value.AttemptID
	attemptID, err := DeriveComponentFactoryPreflightAttemptIDV2(value)
	if err != nil {
		return ComponentFactoryPreflightRequestV2{}, err
	}
	if providedAttempt != "" && providedAttempt != attemptID {
		return ComponentFactoryPreflightRequestV2{}, NewError(ErrorConflict, "component_factory_preflight_attempt_payload_conflict", "component factory preflight AttemptID belongs to another request payload")
	}
	value.AttemptID = attemptID
	provided := value.RequestDigest
	value.RequestDigest = ""
	digest, err := componentFactoryPreflightRequestDigestV2(value)
	if err != nil {
		return ComponentFactoryPreflightRequestV2{}, err
	}
	if provided != "" && provided != digest {
		return ComponentFactoryPreflightRequestV2{}, NewError(ErrorConflict, "component_factory_preflight_request_digest_drift", "component factory preflight request supplied a wrong digest")
	}
	value.RequestDigest = digest
	return value, value.Validate()
}

func (value ComponentFactoryPreflightRequestV2) Validate() error {
	if value.ContractVersion != ComponentFactoryContractVersionV2 || value.ObjectKind != ComponentFactoryPreflightRequestKindV2 {
		return NewError(ErrorInvalidArgument, "component_factory_preflight_request_discriminator_invalid", "component factory preflight request discriminator is invalid")
	}
	for field, item := range map[string]string{"host id": value.HostID, "start id": value.StartID, "attempt id": value.AttemptID} {
		if err := ValidateIdentifierV1(field, item); err != nil {
			return err
		}
	}
	if err := value.DeploymentRef.Validate(); err != nil {
		return err
	}
	if value.DeploymentRef.HostID != value.HostID {
		return NewError(ErrorConflict, "component_factory_deployment_host_drift", "component factory preflight deployment names another Host")
	}
	if err := value.RegistryKey.Validate(); err != nil {
		return err
	}
	if len(value.ResourceRefs) > MaxComponentFactoryResourcesV2 || len(value.DependencyRefs) > MaxComponentFactoryDependenciesV2 {
		return NewError(ErrorInvalidArgument, "component_factory_preflight_count_invalid", "component factory preflight resource or dependency count is invalid")
	}
	canonical := value.canonicalV2()
	if value.ResourceRefs == nil || value.DependencyRefs == nil ||
		!equalComponentResourceRefsV2(value.ResourceRefs, canonical.ResourceRefs) ||
		!equalComponentDependencyRefsV2(value.DependencyRefs, canonical.DependencyRefs) {
		return NewError(ErrorConflict, "component_factory_preflight_not_canonical", "component factory preflight refs are not canonical")
	}
	for index, ref := range value.ResourceRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
		if index > 0 && sameComponentResourceIdentityV2(value.ResourceRefs[index-1], ref) {
			return NewError(ErrorConflict, "component_factory_resource_duplicate", "component factory preflight duplicates a resource")
		}
	}
	for index, ref := range value.DependencyRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
		if index > 0 && value.DependencyRefs[index-1].OwnerID == ref.OwnerID && value.DependencyRefs[index-1].InstanceID == ref.InstanceID {
			return NewError(ErrorConflict, "component_factory_dependency_duplicate", "component factory preflight duplicates a dependency")
		}
	}
	if value.RequestedUnixNano <= 0 || value.RequestedNotAfterUnixNano <= value.RequestedUnixNano {
		return NewError(ErrorInvalidArgument, "component_factory_preflight_window_invalid", "component factory preflight request window is invalid")
	}
	attemptID, err := DeriveComponentFactoryPreflightAttemptIDV2(value)
	if err != nil {
		return err
	}
	if value.AttemptID != attemptID {
		return NewError(ErrorConflict, "component_factory_preflight_attempt_payload_conflict", "component factory preflight AttemptID belongs to another request payload")
	}
	want, err := componentFactoryPreflightRequestDigestV2(value)
	if err != nil || want != value.RequestDigest {
		return NewError(ErrorConflict, "component_factory_preflight_request_digest_drift", "component factory preflight request digest drifted")
	}
	return nil
}

func (value ComponentFactoryPreflightRequestV2) ValidateCurrent(now time.Time) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < value.RequestedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "component factory preflight clock regressed")
	}
	if now.UnixNano() >= value.RequestedNotAfterUnixNano {
		return NewError(ErrorPrecondition, "component_factory_preflight_expired", "component factory preflight request expired")
	}
	return nil
}

type ComponentFactoryPreflightReceiptRefV2 struct {
	AttemptID       string   `json:"attempt_id"`
	Revision        uint64   `json:"revision"`
	Digest          DigestV1 `json:"digest"`
	ExpiresUnixNano int64    `json:"expires_unix_nano"`
}

func (ref ComponentFactoryPreflightReceiptRefV2) Validate() error {
	if err := ValidateIdentifierV1("preflight attempt id", ref.AttemptID); err != nil {
		return err
	}
	if ref.Revision != 1 || ref.ExpiresUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "component_factory_preflight_ref_incomplete", "component factory preflight receipt Ref is incomplete")
	}
	return ref.Digest.Validate()
}

type ComponentFactoryPreflightReceiptV2 struct {
	ContractVersion   string                                         `json:"contract_version"`
	ObjectKind        string                                         `json:"object_kind"`
	Ref               ComponentFactoryPreflightReceiptRefV2          `json:"ref"`
	HostID            string                                         `json:"host_id"`
	StartID           string                                         `json:"start_id"`
	DeploymentRef     HostDeploymentCurrentRefV2                     `json:"deployment_ref"`
	Deployment        HostDeploymentCurrentV2                        `json:"deployment"`
	Request           ComponentFactoryPreflightRequestV2             `json:"request"`
	Selection         buildercontract.AgentPackageSelectionCurrentV1 `json:"selection"`
	VerifiedClosure   buildercontract.VerifiedAgentPackageClosureV1  `json:"verified_closure"`
	PackageRef        buildercontract.AgentPackageRefV1              `json:"package_ref"`
	PublicationRef    assemblycontract.AssemblyPublicationRefV2      `json:"publication_ref"`
	ClosureDigest     DigestV1                                       `json:"closure_digest"`
	PackageDescriptor assemblycontract.ModuleFactoryDescriptorV1     `json:"package_descriptor"`
	RequestDigest     DigestV1                                       `json:"request_digest"`
	Registration      ComponentFactoryRegistrationV2                 `json:"registration"`
	ConformanceRef    ComponentFactoryConformanceCurrentRefV2        `json:"conformance_ref"`
	Conformance       ComponentFactoryConformanceCurrentV2           `json:"conformance"`
	ResourceRefs      []ComponentResourceRefV2                       `json:"resource_refs"`
	ResourceCurrents  []runtimeports.ResourceHandleCurrentV1         `json:"resource_currents"`
	Dependencies      []ComponentDependencyCurrentV2                 `json:"dependencies"`
	CheckedUnixNano   int64                                          `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                                          `json:"expires_unix_nano"`
	ProjectionDigest  DigestV1                                       `json:"projection_digest"`
}

func (value ComponentFactoryPreflightReceiptV2) canonicalV2() ComponentFactoryPreflightReceiptV2 {
	value.ResourceRefs = canonicalComponentResourceRefsV2(value.ResourceRefs)
	value.ResourceCurrents = append([]runtimeports.ResourceHandleCurrentV1{}, value.ResourceCurrents...)
	sort.Slice(value.ResourceCurrents, func(i, j int) bool {
		left, right := value.ResourceCurrents[i].Ref, value.ResourceCurrents[j].Ref
		if left.Owner.Domain != right.Owner.Domain {
			return left.Owner.Domain < right.Owner.Domain
		}
		if left.Owner.ID != right.Owner.ID {
			return left.Owner.ID < right.Owner.ID
		}
		return left.ID < right.ID
	})
	value.Dependencies = append([]ComponentDependencyCurrentV2{}, value.Dependencies...)
	sort.Slice(value.Dependencies, func(i, j int) bool {
		left, right := value.Dependencies[i].Ref, value.Dependencies[j].Ref
		if left.OwnerID != right.OwnerID {
			return left.OwnerID < right.OwnerID
		}
		return left.InstanceID < right.InstanceID
	})
	return value
}

func componentFactoryPreflightReceiptDigestV2(value ComponentFactoryPreflightReceiptV2) (DigestV1, error) {
	value = value.canonicalV2()
	value.Ref.Digest, value.ProjectionDigest = "", ""
	return DigestJSONV1(struct {
		Domain string                             `json:"domain"`
		Type   string                             `json:"type"`
		Body   ComponentFactoryPreflightReceiptV2 `json:"body"`
	}{"praxis.agent-host.component-factory-v2", ComponentFactoryPreflightReceiptKindV2, value})
}

func SealComponentFactoryPreflightReceiptV2(value ComponentFactoryPreflightReceiptV2) (ComponentFactoryPreflightReceiptV2, error) {
	value.ContractVersion = ComponentFactoryContractVersionV2
	value.ObjectKind = ComponentFactoryPreflightReceiptKindV2
	value = value.canonicalV2()
	providedRef, providedProjection := value.Ref.Digest, value.ProjectionDigest
	value.Ref.Digest, value.ProjectionDigest = "", ""
	digest, err := componentFactoryPreflightReceiptDigestV2(value)
	if err != nil {
		return ComponentFactoryPreflightReceiptV2{}, err
	}
	if (providedRef != "" && providedRef != digest) || (providedProjection != "" && providedProjection != digest) {
		return ComponentFactoryPreflightReceiptV2{}, NewError(ErrorConflict, "component_factory_preflight_receipt_digest_drift", "component factory preflight receipt supplied a wrong digest")
	}
	value.Ref.Digest, value.ProjectionDigest = digest, digest
	return value, value.Validate()
}

func (value ComponentFactoryPreflightReceiptV2) Validate() error {
	if value.ContractVersion != ComponentFactoryContractVersionV2 || value.ObjectKind != ComponentFactoryPreflightReceiptKindV2 {
		return NewError(ErrorInvalidArgument, "component_factory_preflight_receipt_discriminator_invalid", "component factory preflight receipt discriminator is invalid")
	}
	if err := value.Ref.Validate(); err != nil {
		return err
	}
	for field, item := range map[string]string{"host id": value.HostID, "start id": value.StartID} {
		if err := ValidateIdentifierV1(field, item); err != nil {
			return err
		}
	}
	if err := value.DeploymentRef.Validate(); err != nil {
		return err
	}
	if err := value.Deployment.ValidateHistoricalV2(); err != nil {
		return err
	}
	if value.Deployment.Ref != value.DeploymentRef {
		return NewError(ErrorConflict, "component_factory_preflight_receipt_deployment_current_drift", "component factory preflight receipt Deployment current differs from its exact Ref")
	}
	if err := value.Request.Validate(); err != nil {
		return err
	}
	if err := value.Selection.Validate(); err != nil {
		return err
	}
	if err := value.VerifiedClosure.Validate(); err != nil {
		return err
	}
	if err := value.PackageRef.Validate(); err != nil {
		return err
	}
	if err := value.PublicationRef.Validate(); err != nil {
		return err
	}
	if err := value.ClosureDigest.Validate(); err != nil {
		return err
	}
	if err := value.PackageDescriptor.Validate(); err != nil {
		return err
	}
	if err := value.RequestDigest.Validate(); err != nil {
		return err
	}
	expectedRequestDigest, err := componentFactoryPreflightRequestDigestFromAttemptIDV2(value.Ref.AttemptID)
	if err != nil {
		return err
	}
	if value.RequestDigest != expectedRequestDigest {
		return NewError(ErrorConflict, "component_factory_preflight_receipt_request_digest_drift", "component factory preflight receipt RequestDigest does not belong to its AttemptID")
	}
	if value.Ref.AttemptID != value.Request.AttemptID ||
		value.RequestDigest != value.Request.RequestDigest ||
		value.HostID != value.Request.HostID ||
		value.StartID != value.Request.StartID ||
		value.DeploymentRef != value.Request.DeploymentRef {
		return NewError(ErrorConflict, "component_factory_preflight_receipt_request_closure_drift", "component factory preflight receipt does not close over its verified request")
	}
	if value.Selection.Ref != value.DeploymentRef.PackageSelectionRef {
		return NewError(ErrorConflict, "component_factory_preflight_receipt_selection_ref_drift", "component factory preflight receipt Selection differs from its Deployment")
	}
	if value.Selection.PackageRef != value.VerifiedClosure.Package.RefV1() ||
		value.Selection.PublicationRef != value.VerifiedClosure.PublicationRefV2() ||
		value.Selection.ClosureDigest != value.VerifiedClosure.ClosureDigest ||
		value.PackageRef != value.Selection.PackageRef ||
		value.PublicationRef != value.Selection.PublicationRef ||
		value.ClosureDigest != DigestV1(value.Selection.ClosureDigest) {
		return NewError(ErrorConflict, "component_factory_preflight_receipt_builder_closure_drift", "component factory preflight receipt Builder closure drifted")
	}
	if err := value.Registration.Validate(); err != nil {
		return err
	}
	authoritativeDescriptor, err := ReadAuthoritativeComponentFactoryPackageDescriptorV2(
		value.VerifiedClosure,
		value.Registration.Descriptor.Ref.FactoryID,
	)
	if err != nil {
		return err
	}
	if value.PackageDescriptor != authoritativeDescriptor {
		return NewError(ErrorConflict, "component_factory_preflight_receipt_package_descriptor_drift", "component factory preflight package descriptor was not read from the verified Builder closure")
	}
	if err := ValidateComponentFactoryPackageDescriptorV2(value.Registration.Descriptor, value.PackageDescriptor); err != nil {
		return err
	}
	if value.ConformanceRef != value.Registration.ConformanceRef {
		return NewError(ErrorConflict, "component_factory_preflight_conformance_drift", "component factory preflight conformance drifted")
	}
	if err := value.Conformance.Validate(); err != nil {
		return err
	}
	if value.Conformance.Ref != value.ConformanceRef ||
		value.Conformance.FactoryRef != value.Registration.Descriptor.Ref ||
		value.Conformance.DescriptorDigest != value.Registration.Descriptor.DescriptorDigest {
		return NewError(ErrorConflict, "component_factory_preflight_receipt_conformance_current_drift", "component factory preflight receipt Conformance current differs from its exact registration")
	}
	if value.Registration.Key != value.Request.RegistryKey {
		return NewError(ErrorConflict, "component_factory_preflight_receipt_registry_drift", "component factory preflight registration differs from its request")
	}
	if value.ResourceRefs == nil || value.ResourceCurrents == nil || value.Dependencies == nil ||
		len(value.ResourceRefs) != len(value.ResourceCurrents) ||
		len(value.ResourceRefs) > MaxComponentFactoryResourcesV2 || len(value.Dependencies) > MaxComponentFactoryDependenciesV2 {
		return NewError(ErrorInvalidArgument, "component_factory_preflight_receipt_count_invalid", "component factory preflight receipt refs are incomplete")
	}
	canonical := value.canonicalV2()
	if !equalComponentResourceRefsV2(value.ResourceRefs, canonical.ResourceRefs) ||
		!equalComponentResourceCurrentsV2(value.ResourceCurrents, canonical.ResourceCurrents) ||
		!equalComponentDependencyCurrentsV2(value.Dependencies, canonical.Dependencies) {
		return NewError(ErrorConflict, "component_factory_preflight_receipt_not_canonical", "component factory preflight receipt refs are not canonical")
	}
	if !equalComponentResourceRefsV2(value.ResourceRefs, value.Request.ResourceRefs) {
		return NewError(ErrorConflict, "component_factory_preflight_receipt_resource_drift", "component factory preflight resources differ from its request")
	}
	if len(value.Dependencies) != len(value.Request.DependencyRefs) {
		return NewError(ErrorConflict, "component_factory_preflight_receipt_dependency_drift", "component factory preflight dependencies differ from its request")
	}
	for index := range value.Dependencies {
		if value.Dependencies[index].Ref != value.Request.DependencyRefs[index] {
			return NewError(ErrorConflict, "component_factory_preflight_receipt_dependency_drift", "component factory preflight dependencies differ from its request")
		}
	}
	minimum := value.DeploymentRef.ExpiresUnixNano
	if value.Selection.ExpiresUnixNano < minimum {
		minimum = value.Selection.ExpiresUnixNano
	}
	for index, ref := range value.ResourceRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
		current := value.ResourceCurrents[index]
		if err := current.Validate(); err != nil {
			return err
		}
		if !componentResourceCurrentMatchesV2(ref, current) {
			return NewError(ErrorConflict, "component_factory_preflight_receipt_resource_current_drift", "component factory preflight Resource current differs from its exact Ref")
		}
		if !deploymentContainsResourceCurrentV2(value.Deployment, current) {
			return NewError(ErrorConflict, "component_factory_preflight_receipt_resource_not_deployed", "component factory preflight Resource current is absent from its Deployment")
		}
		if index > 0 && sameComponentResourceIdentityV2(value.ResourceRefs[index-1], ref) {
			return NewError(ErrorConflict, "component_factory_preflight_receipt_resource_duplicate", "component factory preflight receipt duplicates a resource")
		}
		if ref.ExpiresUnixNano < minimum {
			minimum = ref.ExpiresUnixNano
		}
	}
	for index, dependency := range value.Dependencies {
		if err := dependency.Validate(); err != nil {
			return err
		}
		if index > 0 &&
			value.Dependencies[index-1].Ref.OwnerID == dependency.Ref.OwnerID &&
			value.Dependencies[index-1].Ref.InstanceID == dependency.Ref.InstanceID {
			return NewError(ErrorConflict, "component_factory_preflight_receipt_dependency_duplicate", "component factory preflight receipt duplicates a dependency")
		}
		if dependency.ExpiresUnixNano < minimum {
			minimum = dependency.ExpiresUnixNano
		}
	}
	if value.Conformance.ExpiresUnixNano < minimum {
		minimum = value.Conformance.ExpiresUnixNano
	}
	expectedChecked := componentFactoryPreflightReceiptCheckedUnixNanoV2(value)
	if value.CheckedUnixNano != expectedChecked ||
		value.ExpiresUnixNano <= value.CheckedUnixNano ||
		value.Ref.ExpiresUnixNano != value.ExpiresUnixNano || value.ExpiresUnixNano > minimum {
		return NewError(ErrorPrecondition, "component_factory_preflight_receipt_ttl_drift", "component factory preflight receipt TTL drifted")
	}
	if value.ExpiresUnixNano > value.Request.RequestedNotAfterUnixNano {
		return NewError(ErrorPrecondition, "component_factory_preflight_receipt_request_ttl_drift", "component factory preflight receipt exceeds its request validity window")
	}
	want, err := componentFactoryPreflightReceiptDigestV2(value)
	if err != nil || want != value.Ref.Digest || want != value.ProjectionDigest {
		return NewError(ErrorConflict, "component_factory_preflight_receipt_digest_drift", "component factory preflight receipt digest drifted")
	}
	return nil
}

func CloneComponentFactoryPreflightRequestV2(value ComponentFactoryPreflightRequestV2) ComponentFactoryPreflightRequestV2 {
	value.ResourceRefs = append([]ComponentResourceRefV2{}, value.ResourceRefs...)
	value.DependencyRefs = append([]ComponentInstanceRefV2{}, value.DependencyRefs...)
	return value
}

func CloneComponentFactoryPreflightReceiptV2(value ComponentFactoryPreflightReceiptV2) ComponentFactoryPreflightReceiptV2 {
	value.Deployment = CloneHostDeploymentCurrentV2(value.Deployment)
	value.Request = CloneComponentFactoryPreflightRequestV2(value.Request)
	value.Selection = buildercontract.CloneAgentPackageSelectionCurrentV1(value.Selection)
	value.VerifiedClosure = buildercontract.CloneVerifiedAgentPackageClosureV1(value.VerifiedClosure)
	value.ResourceRefs = append([]ComponentResourceRefV2{}, value.ResourceRefs...)
	value.ResourceCurrents = append([]runtimeports.ResourceHandleCurrentV1{}, value.ResourceCurrents...)
	value.Dependencies = append([]ComponentDependencyCurrentV2{}, value.Dependencies...)
	return value
}

func equalComponentResourceRefsV2(left, right []ComponentResourceRefV2) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalComponentDependencyRefsV2(left, right []ComponentInstanceRefV2) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalComponentDependencyCurrentsV2(left, right []ComponentDependencyCurrentV2) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalComponentResourceCurrentsV2(left, right []runtimeports.ResourceHandleCurrentV1) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameComponentResourceIdentityV2(left, right ComponentResourceRefV2) bool {
	return left.OwnerDomain == right.OwnerDomain && left.OwnerID == right.OwnerID && left.ID == right.ID
}

func componentResourceCurrentMatchesV2(expected ComponentResourceRefV2, current runtimeports.ResourceHandleCurrentV1) bool {
	return string(current.Ref.Owner.Domain) == expected.OwnerDomain &&
		string(current.Ref.Owner.ID) == expected.OwnerID &&
		current.Ref.ID == expected.ID &&
		uint64(current.Ref.Revision) == expected.Revision &&
		DigestV1(current.Ref.Digest) == expected.Digest &&
		string(current.Ref.Kind) == expected.Kind &&
		DigestV1(current.Ref.ScopeDigest) == expected.ScopeDigest &&
		current.Ref.ExpiresUnixNano == expected.ExpiresUnixNano
}

func deploymentContainsResourceCurrentV2(deployment HostDeploymentCurrentV2, current runtimeports.ResourceHandleCurrentV1) bool {
	for _, ref := range deployment.ResourceHandles {
		if ref == current.Ref {
			return true
		}
	}
	return false
}

func componentFactoryPreflightReceiptCheckedUnixNanoV2(value ComponentFactoryPreflightReceiptV2) int64 {
	maximum := value.Request.RequestedUnixNano
	for _, checked := range []int64{
		value.Deployment.CheckedUnixNano,
		value.Selection.CheckedUnixNano,
		value.Conformance.CheckedUnixNano,
	} {
		if checked > maximum {
			maximum = checked
		}
	}
	for _, resource := range value.ResourceCurrents {
		if resource.CheckedUnixNano > maximum {
			maximum = resource.CheckedUnixNano
		}
	}
	for _, dependency := range value.Dependencies {
		if dependency.CheckedUnixNano > maximum {
			maximum = dependency.CheckedUnixNano
		}
	}
	return maximum
}

// ReadAuthoritativeComponentFactoryPackageDescriptorV2 returns the unique
// Builder-owned factory declaration from the verified Package/Publication
// closure. Callers cannot supply a parallel descriptor body.
func ReadAuthoritativeComponentFactoryPackageDescriptorV2(
	closure buildercontract.VerifiedAgentPackageClosureV1,
	factoryID string,
) (assemblycontract.ModuleFactoryDescriptorV1, error) {
	if err := closure.Validate(); err != nil {
		return assemblycontract.ModuleFactoryDescriptorV1{}, err
	}
	var result assemblycontract.ModuleFactoryDescriptorV1
	found := 0
	for _, value := range closure.Publication.Manifest.Factories {
		if value.FactoryID == factoryID {
			result = value
			found++
		}
	}
	if found != 1 {
		return assemblycontract.ModuleFactoryDescriptorV1{}, NewError(ErrorConflict, "component_factory_package_descriptor_missing", "verified package must contain exactly one matching factory descriptor")
	}
	return result, nil
}

// ValidateComponentFactoryPackageDescriptorV2 compares a Host declaration with
// one Builder-owned package declaration. It proves field equality, not the
// implementation source or provenance of the Host Go value. Kinds remain
// Host-owned wrappers; all historical identity fields are compared without
// minting a second Package descriptor.
func ValidateComponentFactoryPackageDescriptorV2(host ComponentFactoryDescriptorV2, packaged assemblycontract.ModuleFactoryDescriptorV1) error {
	if err := host.Validate(); err != nil {
		return err
	}
	if err := packaged.Validate(); err != nil {
		return err
	}
	cleanup := packaged.CleanupContractRef
	sameSchema := func(left ComponentSchemaRefV2, rightNamespace, rightName, rightVersion, rightMediaType string, rightDigest DigestV1) bool {
		return left.Namespace == rightNamespace && left.Name == rightName && left.Version == rightVersion &&
			left.MediaType == rightMediaType && left.ContentDigest == rightDigest
	}
	if host.Ref.FactoryID != packaged.FactoryID ||
		host.ModuleRef != packaged.ModuleRef ||
		host.ArtifactDigest != DigestV1(packaged.ArtifactDigest) ||
		host.ConstructionMode != string(packaged.ConstructionMode) ||
		host.OutputCapability != string(packaged.OutputCapability) ||
		host.Lifecycle != string(packaged.Lifecycle) ||
		!sameSchema(host.InputSchema, packaged.InputSchema.Namespace, packaged.InputSchema.Name, packaged.InputSchema.Version, packaged.InputSchema.MediaType, DigestV1(packaged.InputSchema.ContentDigest)) ||
		host.CleanupContract.Ref.ID != cleanup.Ref.ID ||
		host.CleanupContract.Ref.Revision != uint64(cleanup.Ref.Revision) ||
		host.CleanupContract.Ref.Digest != DigestV1(cleanup.Ref.Digest) ||
		host.CleanupContract.OwnerCapability != string(cleanup.OwnerCapability) ||
		!sameSchema(host.CleanupContract.RequestSchema, cleanup.RequestSchema.Namespace, cleanup.RequestSchema.Name, cleanup.RequestSchema.Version, cleanup.RequestSchema.MediaType, DigestV1(cleanup.RequestSchema.ContentDigest)) ||
		!sameSchema(host.CleanupContract.ResultSchema, cleanup.ResultSchema.Namespace, cleanup.ResultSchema.Name, cleanup.ResultSchema.Version, cleanup.ResultSchema.MediaType, DigestV1(cleanup.ResultSchema.ContentDigest)) ||
		host.TrustRef.ID != packaged.TrustRef.ID ||
		host.TrustRef.Revision != uint64(packaged.TrustRef.Revision) ||
		host.TrustRef.Digest != DigestV1(packaged.TrustRef.Digest) {
		return NewError(ErrorConflict, "component_factory_package_descriptor_drift", "registered factory descriptor differs from the verified package descriptor")
	}
	return nil
}

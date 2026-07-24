package contract

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	PackageVerificationContractVersionV1 = "1.0.0"
	packageVerificationCanonicalDomainV1 = "praxis.tool-mcp.package-verification"
	MaxPackagePolicyIdentitiesV1         = 64
	MaxPackagePredicateTypesV1           = 64
)

const (
	PackageArtifactTypeV1                                       = "application/vnd.praxis.tool-package.v1"
	PackageTrustPolicyMediaTypeV1                               = "application/vnd.praxis.sigstore-policy.v1+json"
	PackageVerifierConformanceV1  runtimeports.NamespacedNameV2 = "praxis.tool/sigstore-go-offline-v1"
)

type ToolPackageArtifactBindingV1 struct {
	ContractVersion string                                       `json:"contract_version"`
	Package         ObjectRef                                    `json:"package"`
	OCIManifest     runtimeports.SupplyChainArtifactContentRefV1 `json:"oci_manifest"`
	PackageArtifact runtimeports.SupplyChainArtifactContentRefV1 `json:"package_artifact"`
	SigstoreBundle  runtimeports.SupplyChainArtifactContentRefV1 `json:"sigstore_bundle"`
	InTotoStatement runtimeports.SupplyChainArtifactContentRefV1 `json:"in_toto_statement"`
	ArtifactType    string                                       `json:"artifact_type"`
	BindingDigest   core.Digest                                  `json:"binding_digest"`
}

func (b ToolPackageArtifactBindingV1) Validate() error {
	if b.ContractVersion != PackageVerificationContractVersionV1 || b.Package.Validate() != nil || b.OCIManifest.Validate() != nil || b.PackageArtifact.Validate() != nil || b.SigstoreBundle.Validate() != nil || b.InTotoStatement.Validate() != nil || b.ArtifactType != PackageArtifactTypeV1 {
		return invalid("package artifact binding is incomplete")
	}
	if b.BindingDigest.Validate() != nil {
		return invalid("package artifact binding digest is invalid")
	}
	digest, err := b.ComputeDigest()
	if err != nil || digest != b.BindingDigest {
		return conflict("package artifact binding digest drifted")
	}
	return nil
}

func (b ToolPackageArtifactBindingV1) ValidateAgainst(m ToolPackageManifest) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if err := m.Validate(); err != nil {
		return err
	}
	if b.Package != (ObjectRef{ID: string(m.ID), Revision: m.Revision, Digest: m.Digest}) || b.PackageArtifact.Digest != m.ArtifactDigest {
		return conflict("package artifact binding differs from package manifest")
	}
	return nil
}

func (b ToolPackageArtifactBindingV1) ComputeDigest() (core.Digest, error) {
	b.BindingDigest = ""
	return Seal(packageVerificationCanonicalDomainV1, PackageVerificationContractVersionV1, "ToolPackageArtifactBindingV1", b)
}

func SealToolPackageArtifactBindingV1(b ToolPackageArtifactBindingV1) (ToolPackageArtifactBindingV1, error) {
	b.ContractVersion = PackageVerificationContractVersionV1
	b.BindingDigest = ""
	digest, err := b.ComputeDigest()
	if err != nil {
		return ToolPackageArtifactBindingV1{}, err
	}
	b.BindingDigest = digest
	return b, b.Validate()
}

type ToolPackageRegistryCurrentRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r ToolPackageRegistryCurrentRefV1) Validate() error {
	if r.ContractVersion != PackageVerificationContractVersionV1 || ValidateStableID(r.ID) != nil || r.Revision == 0 {
		return invalid("package registry current ref is incomplete")
	}
	return r.Digest.Validate()
}

type ToolPackageRegistryRecordSourceV1 struct {
	Kind             string        `json:"kind"`
	ID               string        `json:"id"`
	ObjectRevision   core.Revision `json:"object_revision"`
	ObjectDigest     core.Digest   `json:"object_digest"`
	State            string        `json:"state"`
	RegistryRevision core.Revision `json:"registry_revision"`
	UpdatedUnixNano  int64         `json:"updated_unix_nano"`
	Digest           core.Digest   `json:"digest"`
}

func (s ToolPackageRegistryRecordSourceV1) Validate() error {
	if s.Kind != "package" || !ValidObjectID(s.ID) || s.ObjectRevision == 0 || s.RegistryRevision == 0 || s.UpdatedUnixNano <= 0 || !validPackageRegistryStateV1(s.State) || s.ObjectDigest.Validate() != nil || s.Digest.Validate() != nil {
		return invalid("package registry source record is incomplete")
	}
	digest, err := s.ComputeDigest()
	if err != nil || digest != s.Digest {
		return conflict("package registry source record digest drifted")
	}
	return nil
}

func (s ToolPackageRegistryRecordSourceV1) ComputeDigest() (core.Digest, error) {
	s.Digest = ""
	return Seal("praxis.tool-mcp.registry", PackageVerificationContractVersionV1, "ToolPackageRegistryRecordSourceV1", s)
}

func SealToolPackageRegistryRecordSourceV1(s ToolPackageRegistryRecordSourceV1) (ToolPackageRegistryRecordSourceV1, error) {
	s.Digest = ""
	digest, err := s.ComputeDigest()
	if err != nil {
		return ToolPackageRegistryRecordSourceV1{}, err
	}
	s.Digest = digest
	return s, s.Validate()
}

type ToolPackageRegistryCurrentProjectionV1 struct {
	ContractVersion  string                            `json:"contract_version"`
	Ref              ToolPackageRegistryCurrentRefV1   `json:"ref"`
	Source           ToolPackageRegistryRecordSourceV1 `json:"source"`
	Package          ObjectRef                         `json:"package"`
	Manifest         ToolPackageManifest               `json:"manifest"`
	State            string                            `json:"state"`
	RegistryRevision core.Revision                     `json:"registry_revision"`
	CheckedUnixNano  int64                             `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                             `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                       `json:"projection_digest"`
}

func (p ToolPackageRegistryCurrentProjectionV1) Clone() ToolPackageRegistryCurrentProjectionV1 {
	p.Manifest.Signatures = append([]core.Digest(nil), p.Manifest.Signatures...)
	p.Manifest.Descriptors = append([]PackageDescriptorRef(nil), p.Manifest.Descriptors...)
	p.Manifest.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), p.Manifest.EffectKinds...)
	return p
}

func (p ToolPackageRegistryCurrentProjectionV1) Validate() error {
	if p.ContractVersion != PackageVerificationContractVersionV1 || p.Ref.Validate() != nil || p.Source.Validate() != nil || p.Package.Validate() != nil || p.Manifest.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil {
		return invalid("package registry current projection is incomplete")
	}
	expectedID, err := DeriveToolPackageRegistryCurrentIDV1(p.Package)
	if err != nil || p.Ref.ID != expectedID || p.Ref.Revision != p.Source.RegistryRevision || p.Ref.Digest != p.Source.Digest || p.Package != (ObjectRef{ID: p.Source.ID, Revision: p.Source.ObjectRevision, Digest: p.Source.ObjectDigest}) || p.Package != (ObjectRef{ID: string(p.Manifest.ID), Revision: p.Manifest.Revision, Digest: p.Manifest.Digest}) || p.State != p.Source.State || p.RegistryRevision != p.Source.RegistryRevision {
		return conflict("package registry current repeated fields drifted")
	}
	digest, err := p.ComputeDigest()
	if err != nil || digest != p.ProjectionDigest {
		return conflict("package registry current projection digest drifted")
	}
	return nil
}

func (p ToolPackageRegistryCurrentProjectionV1) ValidateCurrent(expected ToolPackageRegistryCurrentRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if expected != p.Ref {
		return conflict("package registry current ref drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "package registry current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) || p.State == "deprecated" || p.State == "revoked" {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "package registry current is expired or terminal")
	}
	return nil
}

func (p ToolPackageRegistryCurrentProjectionV1) ComputeDigest() (core.Digest, error) {
	p = p.Clone()
	p.ProjectionDigest = ""
	return Seal(packageVerificationCanonicalDomainV1, PackageVerificationContractVersionV1, "ToolPackageRegistryCurrentProjectionV1", p)
}

func SealToolPackageRegistryCurrentProjectionV1(p ToolPackageRegistryCurrentProjectionV1) (ToolPackageRegistryCurrentProjectionV1, error) {
	p = p.Clone()
	p.ContractVersion = PackageVerificationContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.ComputeDigest()
	if err != nil {
		return ToolPackageRegistryCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

func DeriveToolPackageRegistryCurrentIDV1(pkg ObjectRef) (string, error) {
	if err := pkg.Validate(); err != nil {
		return "", err
	}
	return StableID("pkgreg", pkg.ID)
}

type ToolPackageSigstoreIdentityModeV1 string

const (
	ToolPackageSigstoreCertificateV1 ToolPackageSigstoreIdentityModeV1 = "certificate"
	ToolPackageSigstoreKeyV1         ToolPackageSigstoreIdentityModeV1 = "key"
)

type ToolPackageSigstoreCertificateIdentityV1 struct {
	Issuer      string `json:"issuer,omitempty"`
	IssuerRegex string `json:"issuer_regex,omitempty"`
	SANValue    string `json:"san_value,omitempty"`
	SANRegex    string `json:"san_regex,omitempty"`
}

func (i ToolPackageSigstoreCertificateIdentityV1) Validate() error {
	if !exactlyOneTextV1(i.Issuer, i.IssuerRegex) || !exactlyOneTextV1(i.SANValue, i.SANRegex) {
		return invalid("Sigstore certificate identity requires one issuer and one SAN matcher")
	}
	for _, value := range []string{i.Issuer, i.IssuerRegex, i.SANValue, i.SANRegex} {
		if len(value) > MaxStringBytes || strings.TrimSpace(value) != value {
			return invalid("Sigstore certificate identity matcher is non-canonical")
		}
	}
	for _, expression := range []string{i.IssuerRegex, i.SANRegex} {
		if expression != "" {
			if _, err := regexp.Compile(expression); err != nil {
				return invalid("Sigstore certificate identity regex is invalid")
			}
		}
	}
	return nil
}

func (i ToolPackageSigstoreCertificateIdentityV1) key() string {
	return strings.Join([]string{i.Issuer, i.IssuerRegex, i.SANValue, i.SANRegex}, "\x00")
}

type ToolPackageSigstoreTimestampModeV1 string

const (
	ToolPackageSigstoreObserverTimestampV1   ToolPackageSigstoreTimestampModeV1 = "observer"
	ToolPackageSigstoreSignedTimestampV1     ToolPackageSigstoreTimestampModeV1 = "signed"
	ToolPackageSigstoreIntegratedTimestampV1 ToolPackageSigstoreTimestampModeV1 = "integrated"
	ToolPackageSigstoreNoTimestampForKeyV1   ToolPackageSigstoreTimestampModeV1 = "none_for_key"
)

type ToolPackageTrustPolicyDocumentV1 struct {
	ContractVersion          string                                     `json:"contract_version"`
	IdentityMode             ToolPackageSigstoreIdentityModeV1          `json:"identity_mode"`
	CertificateIdentities    []ToolPackageSigstoreCertificateIdentityV1 `json:"certificate_identities"`
	RequiredPredicateTypes   []string                                   `json:"required_predicate_types"`
	TimestampMode            ToolPackageSigstoreTimestampModeV1         `json:"timestamp_mode"`
	TimestampThreshold       uint32                                     `json:"timestamp_threshold"`
	TransparencyLogThreshold uint32                                     `json:"transparency_log_threshold"`
	SCTThreshold             uint32                                     `json:"sct_threshold"`
	IdentityPolicyDigest     core.Digest                                `json:"identity_policy_digest"`
	PredicatePolicyDigest    core.Digest                                `json:"predicate_policy_digest"`
	TransparencyPolicyDigest core.Digest                                `json:"transparency_policy_digest"`
	TimestampPolicyDigest    core.Digest                                `json:"timestamp_policy_digest"`
}

func (p ToolPackageTrustPolicyDocumentV1) Clone() ToolPackageTrustPolicyDocumentV1 {
	p.CertificateIdentities = append([]ToolPackageSigstoreCertificateIdentityV1(nil), p.CertificateIdentities...)
	p.RequiredPredicateTypes = append([]string(nil), p.RequiredPredicateTypes...)
	return p
}

func (p ToolPackageTrustPolicyDocumentV1) Validate() error {
	if p.ContractVersion != PackageVerificationContractVersionV1 || len(p.RequiredPredicateTypes) == 0 || len(p.RequiredPredicateTypes) > MaxPackagePredicateTypesV1 {
		return invalid("package trust policy document is incomplete")
	}
	if p.IdentityMode == ToolPackageSigstoreCertificateV1 {
		if len(p.CertificateIdentities) == 0 || len(p.CertificateIdentities) > MaxPackagePolicyIdentitiesV1 || p.TimestampMode == ToolPackageSigstoreNoTimestampForKeyV1 || p.TimestampThreshold == 0 || p.TransparencyLogThreshold == 0 {
			return invalid("certificate trust policy requires identities and timestamp evidence")
		}
	} else if p.IdentityMode == ToolPackageSigstoreKeyV1 {
		if len(p.CertificateIdentities) != 0 || p.TimestampMode != ToolPackageSigstoreNoTimestampForKeyV1 || p.TimestampThreshold != 0 || p.TransparencyLogThreshold != 0 || p.SCTThreshold != 0 {
			return invalid("key trust policy cannot carry certificate or timestamp rules")
		}
	} else {
		return invalid("package trust policy identity mode is invalid")
	}
	switch p.TimestampMode {
	case ToolPackageSigstoreObserverTimestampV1, ToolPackageSigstoreSignedTimestampV1, ToolPackageSigstoreIntegratedTimestampV1, ToolPackageSigstoreNoTimestampForKeyV1:
	default:
		return invalid("package trust policy timestamp mode is invalid")
	}
	for index, identity := range p.CertificateIdentities {
		if err := identity.Validate(); err != nil {
			return err
		}
		if index > 0 && p.CertificateIdentities[index-1].key() >= identity.key() {
			return invalid("certificate identities must be sorted and unique")
		}
	}
	for index, predicate := range p.RequiredPredicateTypes {
		if strings.TrimSpace(predicate) != predicate || predicate == "" || len(predicate) > MaxStringBytes || (index > 0 && p.RequiredPredicateTypes[index-1] >= predicate) {
			return invalid("predicate types must be sorted, unique and bounded")
		}
	}
	identity, predicate, transparency, timestamp, err := p.ComputePolicyDigests()
	if err != nil || identity != p.IdentityPolicyDigest || predicate != p.PredicatePolicyDigest || transparency != p.TransparencyPolicyDigest || timestamp != p.TimestampPolicyDigest {
		return conflict("package trust policy component digest drifted")
	}
	return nil
}

func (p ToolPackageTrustPolicyDocumentV1) ComputePolicyDigests() (core.Digest, core.Digest, core.Digest, core.Digest, error) {
	identity, err := Seal(packageVerificationCanonicalDomainV1, PackageVerificationContractVersionV1, "ToolPackageTrustIdentityPolicyV1", struct {
		Mode       ToolPackageSigstoreIdentityModeV1          `json:"mode"`
		Identities []ToolPackageSigstoreCertificateIdentityV1 `json:"identities"`
	}{p.IdentityMode, append([]ToolPackageSigstoreCertificateIdentityV1(nil), p.CertificateIdentities...)})
	if err != nil {
		return "", "", "", "", err
	}
	predicate, err := Seal(packageVerificationCanonicalDomainV1, PackageVerificationContractVersionV1, "ToolPackagePredicatePolicyV1", append([]string(nil), p.RequiredPredicateTypes...))
	if err != nil {
		return "", "", "", "", err
	}
	transparency, err := Seal(packageVerificationCanonicalDomainV1, PackageVerificationContractVersionV1, "ToolPackageTransparencyPolicyV1", struct {
		Log uint32 `json:"log_threshold"`
		SCT uint32 `json:"sct_threshold"`
	}{p.TransparencyLogThreshold, p.SCTThreshold})
	if err != nil {
		return "", "", "", "", err
	}
	timestamp, err := Seal(packageVerificationCanonicalDomainV1, PackageVerificationContractVersionV1, "ToolPackageTimestampPolicyV1", struct {
		Mode      ToolPackageSigstoreTimestampModeV1 `json:"mode"`
		Threshold uint32                             `json:"threshold"`
	}{p.TimestampMode, p.TimestampThreshold})
	return identity, predicate, transparency, timestamp, err
}

func SealToolPackageTrustPolicyDocumentV1(p ToolPackageTrustPolicyDocumentV1) (ToolPackageTrustPolicyDocumentV1, error) {
	p = p.Clone()
	p.ContractVersion = PackageVerificationContractVersionV1
	sort.Slice(p.CertificateIdentities, func(i, j int) bool { return p.CertificateIdentities[i].key() < p.CertificateIdentities[j].key() })
	sort.Strings(p.RequiredPredicateTypes)
	identity, predicate, transparency, timestamp, err := p.ComputePolicyDigests()
	if err != nil {
		return ToolPackageTrustPolicyDocumentV1{}, err
	}
	p.IdentityPolicyDigest = identity
	p.PredicatePolicyDigest = predicate
	p.TransparencyPolicyDigest = transparency
	p.TimestampPolicyDigest = timestamp
	return p, p.Validate()
}

type ToolPackageVerificationSubjectV1 struct {
	ContractVersion string                                   `json:"contract_version"`
	PackageRegistry ToolPackageRegistryCurrentRefV1          `json:"package_registry"`
	ArtifactBinding ToolPackageArtifactBindingV1             `json:"artifact_binding"`
	TrustPolicy     runtimeports.SupplyChainTrustPolicyRefV1 `json:"trust_policy"`
	VerifierProfile runtimeports.NamespacedNameV2            `json:"verifier_profile"`
}

func (s ToolPackageVerificationSubjectV1) Validate() error {
	if s.ContractVersion != PackageVerificationContractVersionV1 || s.PackageRegistry.Validate() != nil || s.ArtifactBinding.Validate() != nil || s.TrustPolicy.Validate() != nil || runtimeports.ValidateNamespacedNameV2(s.VerifierProfile) != nil || s.PackageRegistry.ID == "" {
		return invalid("package verification subject is incomplete")
	}
	return nil
}

type ToolPackageVerifyRequestV1 struct {
	ContractVersion          string                                          `json:"contract_version"`
	Subject                  ToolPackageVerificationSubjectV1                `json:"subject"`
	TrustPolicyCurrent       runtimeports.SupplyChainTrustPolicyCurrentRefV1 `json:"trust_policy_current"`
	RequestedExpiresUnixNano int64                                           `json:"requested_expires_unix_nano"`
}

func (r ToolPackageVerifyRequestV1) ValidateCurrent(now time.Time) error {
	if r.ContractVersion != PackageVerificationContractVersionV1 || r.Subject.Validate() != nil || r.TrustPolicyCurrent.Validate() != nil || now.IsZero() || r.RequestedExpiresUnixNano <= now.UnixNano() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingExpired, "package verify request is invalid or expired")
	}
	return nil
}

type ToolPackageVerificationObservationRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r ToolPackageVerificationObservationRefV1) Validate() error {
	if r.ContractVersion != PackageVerificationContractVersionV1 || ValidateStableID(r.ID) != nil || r.Revision != 1 {
		return invalid("package verification observation ref is incomplete")
	}
	return r.Digest.Validate()
}

type ToolPackageVerificationObservationEnsureRequestV1 struct {
	ContractVersion          string                                           `json:"contract_version"`
	Subject                  ToolPackageVerificationSubjectV1                 `json:"subject"`
	TrustPolicyCurrent       runtimeports.SupplyChainTrustPolicyCurrentRefV1  `json:"trust_policy_current"`
	TrustedRoot              runtimeports.SupplyChainTrustMaterialRefV1       `json:"trusted_root"`
	PolicyDocument           runtimeports.SupplyChainTrustPolicyDocumentRefV1 `json:"policy_document"`
	IdentityPolicyDigest     core.Digest                                      `json:"identity_policy_digest"`
	PredicatePolicyDigest    core.Digest                                      `json:"predicate_policy_digest"`
	TransparencyPolicyDigest core.Digest                                      `json:"transparency_policy_digest"`
	TimestampPolicyDigest    core.Digest                                      `json:"timestamp_policy_digest"`
	SignerIdentityDigest     core.Digest                                      `json:"signer_identity_digest"`
	PredicateType            string                                           `json:"predicate_type"`
	VerifierConformance      runtimeports.NamespacedNameV2                    `json:"verifier_conformance"`
}

func (r ToolPackageVerificationObservationEnsureRequestV1) Validate() error {
	if r.ContractVersion != PackageVerificationContractVersionV1 || r.Subject.Validate() != nil || r.TrustPolicyCurrent.Validate() != nil || r.TrustedRoot.Validate() != nil || r.PolicyDocument.Validate() != nil || r.SignerIdentityDigest.Validate() != nil || strings.TrimSpace(r.PredicateType) == "" || len(r.PredicateType) > MaxStringBytes || runtimeports.ValidateNamespacedNameV2(r.VerifierConformance) != nil {
		return invalid("package verification observation request is incomplete")
	}
	for _, digest := range []core.Digest{r.IdentityPolicyDigest, r.PredicatePolicyDigest, r.TransparencyPolicyDigest, r.TimestampPolicyDigest} {
		if digest.Validate() != nil {
			return invalid("package verification observation policy digest is invalid")
		}
	}
	return nil
}

type ToolPackageVerificationObservationV1 struct {
	ContractVersion  string                                            `json:"contract_version"`
	Ref              ToolPackageVerificationObservationRefV1           `json:"ref"`
	Request          ToolPackageVerificationObservationEnsureRequestV1 `json:"request"`
	ObservedUnixNano int64                                             `json:"observed_unix_nano"`
}

func (o ToolPackageVerificationObservationV1) Validate() error {
	if o.ContractVersion != PackageVerificationContractVersionV1 || o.Ref.Validate() != nil || o.Request.Validate() != nil || o.ObservedUnixNano <= 0 {
		return invalid("package verification observation is incomplete")
	}
	id, err := DeriveToolPackageVerificationObservationIDV1(o.Request.Subject, o.Request.TrustPolicyCurrent)
	if err != nil || id != o.Ref.ID {
		return conflict("package verification observation id drifted")
	}
	digest, err := o.ComputeDigest()
	if err != nil || digest != o.Ref.Digest {
		return conflict("package verification observation digest drifted")
	}
	return nil
}

func (o ToolPackageVerificationObservationV1) ComputeDigest() (core.Digest, error) {
	o.Ref.Digest = ""
	return Seal(packageVerificationCanonicalDomainV1, PackageVerificationContractVersionV1, "ToolPackageVerificationObservationV1", o)
}

func SealToolPackageVerificationObservationV1(o ToolPackageVerificationObservationV1) (ToolPackageVerificationObservationV1, error) {
	o.ContractVersion = PackageVerificationContractVersionV1
	o.Ref.ContractVersion = PackageVerificationContractVersionV1
	o.Ref.Revision = 1
	id, err := DeriveToolPackageVerificationObservationIDV1(o.Request.Subject, o.Request.TrustPolicyCurrent)
	if err != nil {
		return ToolPackageVerificationObservationV1{}, err
	}
	if o.Ref.ID != "" && o.Ref.ID != id {
		return ToolPackageVerificationObservationV1{}, conflict("supplied package observation id drifted")
	}
	o.Ref.ID = id
	o.Ref.Digest = ""
	digest, err := o.ComputeDigest()
	if err != nil {
		return ToolPackageVerificationObservationV1{}, err
	}
	o.Ref.Digest = digest
	return o, o.Validate()
}

func DeriveToolPackageVerificationObservationIDV1(subject ToolPackageVerificationSubjectV1, policy runtimeports.SupplyChainTrustPolicyCurrentRefV1) (string, error) {
	if subject.Validate() != nil || policy.Validate() != nil {
		return "", invalid("package observation identity inputs are invalid")
	}
	digest, err := Seal(packageVerificationCanonicalDomainV1, PackageVerificationContractVersionV1, "ToolPackageVerificationObservationIdentityV1", struct {
		Subject ToolPackageVerificationSubjectV1                `json:"subject"`
		Policy  runtimeports.SupplyChainTrustPolicyCurrentRefV1 `json:"policy"`
	}{subject, policy})
	if err != nil {
		return "", err
	}
	return StableID("pkgobs", string(digest))
}

type ToolPackageVerificationFactRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r ToolPackageVerificationFactRefV1) Validate() error {
	if r.ContractVersion != PackageVerificationContractVersionV1 || ValidateStableID(r.ID) != nil || r.Revision != 1 {
		return invalid("package verification fact ref is incomplete")
	}
	return r.Digest.Validate()
}

type ToolPackageVerificationFactEnsureRequestV1 struct {
	ContractVersion string                                  `json:"contract_version"`
	Subject         ToolPackageVerificationSubjectV1        `json:"subject"`
	Observation     ToolPackageVerificationObservationRefV1 `json:"observation"`
}

func (r ToolPackageVerificationFactEnsureRequestV1) Validate() error {
	if r.ContractVersion != PackageVerificationContractVersionV1 || r.Subject.Validate() != nil || r.Observation.Validate() != nil {
		return invalid("package verification fact request is incomplete")
	}
	return nil
}

type ToolPackageVerificationFactV1 struct {
	ContractVersion       string                                   `json:"contract_version"`
	Ref                   ToolPackageVerificationFactRefV1         `json:"ref"`
	Package               ObjectRef                                `json:"package"`
	PackageRegistry       ToolPackageRegistryCurrentRefV1          `json:"package_registry"`
	ArtifactBindingDigest core.Digest                              `json:"artifact_binding_digest"`
	TrustPolicy           runtimeports.SupplyChainTrustPolicyRefV1 `json:"trust_policy"`
	Observation           ToolPackageVerificationObservationRefV1  `json:"observation"`
	SignerIdentityDigest  core.Digest                              `json:"signer_identity_digest"`
	PredicateType         string                                   `json:"predicate_type"`
	VerifierConformance   runtimeports.NamespacedNameV2            `json:"verifier_conformance"`
	VerifiedUnixNano      int64                                    `json:"verified_unix_nano"`
}

func (f ToolPackageVerificationFactV1) Validate() error {
	if f.ContractVersion != PackageVerificationContractVersionV1 || f.Ref.Validate() != nil || f.Package.Validate() != nil || f.PackageRegistry.Validate() != nil || f.ArtifactBindingDigest.Validate() != nil || f.TrustPolicy.Validate() != nil || f.Observation.Validate() != nil || f.SignerIdentityDigest.Validate() != nil || strings.TrimSpace(f.PredicateType) == "" || runtimeports.ValidateNamespacedNameV2(f.VerifierConformance) != nil || f.VerifiedUnixNano <= 0 {
		return invalid("package verification fact is incomplete")
	}
	id, err := DeriveToolPackageVerificationFactIDV1(f.Observation, f.Package)
	if err != nil || id != f.Ref.ID {
		return conflict("package verification fact id drifted")
	}
	digest, err := f.ComputeDigest()
	if err != nil || digest != f.Ref.Digest {
		return conflict("package verification fact digest drifted")
	}
	return nil
}

func (f ToolPackageVerificationFactV1) ComputeDigest() (core.Digest, error) {
	f.Ref.Digest = ""
	return Seal(packageVerificationCanonicalDomainV1, PackageVerificationContractVersionV1, "ToolPackageVerificationFactV1", f)
}

func SealToolPackageVerificationFactV1(f ToolPackageVerificationFactV1) (ToolPackageVerificationFactV1, error) {
	f.ContractVersion = PackageVerificationContractVersionV1
	f.Ref.ContractVersion = PackageVerificationContractVersionV1
	f.Ref.Revision = 1
	id, err := DeriveToolPackageVerificationFactIDV1(f.Observation, f.Package)
	if err != nil {
		return ToolPackageVerificationFactV1{}, err
	}
	if f.Ref.ID != "" && f.Ref.ID != id {
		return ToolPackageVerificationFactV1{}, conflict("supplied package fact id drifted")
	}
	f.Ref.ID = id
	f.Ref.Digest = ""
	digest, err := f.ComputeDigest()
	if err != nil {
		return ToolPackageVerificationFactV1{}, err
	}
	f.Ref.Digest = digest
	return f, f.Validate()
}

func DeriveToolPackageVerificationFactIDV1(observation ToolPackageVerificationObservationRefV1, pkg ObjectRef) (string, error) {
	if observation.Validate() != nil || pkg.Validate() != nil {
		return "", invalid("package fact identity inputs are invalid")
	}
	digest, err := Seal(packageVerificationCanonicalDomainV1, PackageVerificationContractVersionV1, "ToolPackageVerificationFactIdentityV1", struct {
		Observation ToolPackageVerificationObservationRefV1 `json:"observation"`
		Package     ObjectRef                               `json:"package"`
	}{observation, pkg})
	if err != nil {
		return "", err
	}
	return StableID("pkgfact", string(digest))
}

type ToolPackageVerificationCurrentRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r ToolPackageVerificationCurrentRefV1) Validate() error {
	if r.ContractVersion != PackageVerificationContractVersionV1 || ValidateStableID(r.ID) != nil || r.Revision != 1 {
		return invalid("package verification current ref is incomplete")
	}
	return r.Digest.Validate()
}

type ToolPackageVerificationCurrentIssuanceV1 struct {
	ContractVersion          string                                          `json:"contract_version"`
	Fact                     ToolPackageVerificationFactRefV1                `json:"fact"`
	PackageRegistry          ToolPackageRegistryCurrentRefV1                 `json:"package_registry"`
	TrustPolicyCurrent       runtimeports.SupplyChainTrustPolicyCurrentRefV1 `json:"trust_policy_current"`
	RequestedExpiresUnixNano int64                                           `json:"requested_expires_unix_nano"`
}

func (i ToolPackageVerificationCurrentIssuanceV1) ValidateCurrent(now time.Time) error {
	if i.ContractVersion != PackageVerificationContractVersionV1 || i.Fact.Validate() != nil || i.PackageRegistry.Validate() != nil || i.TrustPolicyCurrent.Validate() != nil || now.IsZero() || i.RequestedExpiresUnixNano <= now.UnixNano() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingExpired, "package verification current issuance is invalid or expired")
	}
	return nil
}

type ToolPackageVerificationCurrentProjectionV1 struct {
	ContractVersion        string                                                 `json:"contract_version"`
	Ref                    ToolPackageVerificationCurrentRefV1                    `json:"ref"`
	Issuance               ToolPackageVerificationCurrentIssuanceV1               `json:"issuance"`
	Fact                   ToolPackageVerificationFactV1                          `json:"fact"`
	CurrentPackageRegistry ToolPackageRegistryCurrentProjectionV1                 `json:"current_package_registry"`
	TrustPolicy            runtimeports.SupplyChainTrustPolicyCurrentProjectionV1 `json:"trust_policy"`
	CheckedUnixNano        int64                                                  `json:"checked_unix_nano"`
	ExpiresUnixNano        int64                                                  `json:"expires_unix_nano"`
	ProjectionDigest       core.Digest                                            `json:"projection_digest"`
}

func (p ToolPackageVerificationCurrentProjectionV1) Clone() ToolPackageVerificationCurrentProjectionV1 {
	p.CurrentPackageRegistry = p.CurrentPackageRegistry.Clone()
	return p
}

func (p ToolPackageVerificationCurrentProjectionV1) Validate() error {
	if p.ContractVersion != PackageVerificationContractVersionV1 || p.Ref.Validate() != nil || p.Fact.Validate() != nil || p.CurrentPackageRegistry.Validate() != nil || p.TrustPolicy.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil {
		return invalid("package verification current projection is incomplete")
	}
	if p.Issuance.ContractVersion != PackageVerificationContractVersionV1 || p.Issuance.Fact != p.Fact.Ref || p.Issuance.PackageRegistry != p.CurrentPackageRegistry.Ref || p.Issuance.TrustPolicyCurrent != p.TrustPolicy.Ref || p.Fact.Package != p.CurrentPackageRegistry.Package || p.Fact.TrustPolicy != p.TrustPolicy.Policy || p.ExpiresUnixNano > p.Issuance.RequestedExpiresUnixNano || p.ExpiresUnixNano > p.CurrentPackageRegistry.ExpiresUnixNano || p.ExpiresUnixNano > p.TrustPolicy.ExpiresUnixNano {
		return conflict("package verification current causal closure drifted")
	}
	id, err := DeriveToolPackageVerificationCurrentIDV1(p.Issuance)
	if err != nil || p.Ref.ID != id || p.Ref.Revision != 1 || p.Ref.Digest != p.ProjectionDigest {
		return conflict("package verification current identity drifted")
	}
	digest, err := p.ComputeDigest()
	if err != nil || digest != p.ProjectionDigest {
		return conflict("package verification current projection digest drifted")
	}
	return nil
}

func (p ToolPackageVerificationCurrentProjectionV1) ValidateCurrent(expected ToolPackageVerificationCurrentRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if expected != p.Ref {
		return conflict("package verification current ref drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "package verification current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "package verification current projection expired")
	}
	return nil
}

func (p ToolPackageVerificationCurrentProjectionV1) ComputeDigest() (core.Digest, error) {
	p = p.Clone()
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	return Seal(packageVerificationCanonicalDomainV1, PackageVerificationContractVersionV1, "ToolPackageVerificationCurrentProjectionV1", p)
}

func SealToolPackageVerificationCurrentProjectionV1(p ToolPackageVerificationCurrentProjectionV1) (ToolPackageVerificationCurrentProjectionV1, error) {
	p = p.Clone()
	p.ContractVersion = PackageVerificationContractVersionV1
	p.Ref.ContractVersion = PackageVerificationContractVersionV1
	p.Ref.Revision = 1
	id, err := DeriveToolPackageVerificationCurrentIDV1(p.Issuance)
	if err != nil {
		return ToolPackageVerificationCurrentProjectionV1{}, err
	}
	if p.Ref.ID != "" && p.Ref.ID != id {
		return ToolPackageVerificationCurrentProjectionV1{}, conflict("supplied package current id drifted")
	}
	p.Ref.ID = id
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	digest, err := p.ComputeDigest()
	if err != nil {
		return ToolPackageVerificationCurrentProjectionV1{}, err
	}
	p.Ref.Digest = digest
	p.ProjectionDigest = digest
	return p, p.Validate()
}

func DeriveToolPackageVerificationCurrentIDV1(i ToolPackageVerificationCurrentIssuanceV1) (string, error) {
	if i.ContractVersion != PackageVerificationContractVersionV1 || i.Fact.Validate() != nil || i.PackageRegistry.Validate() != nil || i.TrustPolicyCurrent.Validate() != nil || i.RequestedExpiresUnixNano <= 0 {
		return "", invalid("package verification current issuance identity is invalid")
	}
	digest, err := Seal(packageVerificationCanonicalDomainV1, PackageVerificationContractVersionV1, "ToolPackageVerificationCurrentIdentityV1", i)
	if err != nil {
		return "", err
	}
	return StableID("pkgcurrent", string(digest))
}

type ToolPackageRegistryCurrentReaderV1 interface {
	InspectCurrentToolPackageRegistryV1(context.Context, ToolPackageRegistryCurrentRefV1) (ToolPackageRegistryCurrentProjectionV1, error)
}

type ToolPackageVerificationRepositoryV1 interface {
	EnsureToolPackageVerificationObservationV1(context.Context, ToolPackageVerificationObservationEnsureRequestV1) (ToolPackageVerificationObservationV1, error)
	InspectToolPackageVerificationObservationBySubjectV1(context.Context, ToolPackageVerificationSubjectV1, runtimeports.SupplyChainTrustPolicyCurrentRefV1) (ToolPackageVerificationObservationV1, error)
	InspectExactToolPackageVerificationObservationV1(context.Context, ToolPackageVerificationObservationRefV1) (ToolPackageVerificationObservationV1, error)
	EnsureToolPackageVerificationFactV1(context.Context, ToolPackageVerificationFactEnsureRequestV1) (ToolPackageVerificationFactV1, error)
	InspectToolPackageVerificationFactByObservationV1(context.Context, ToolPackageVerificationObservationRefV1) (ToolPackageVerificationFactV1, error)
	InspectExactToolPackageVerificationFactV1(context.Context, ToolPackageVerificationFactRefV1) (ToolPackageVerificationFactV1, error)
}

type ToolPackageVerificationCurrentResolverV1 interface {
	ResolveCurrentToolPackageVerificationV1(context.Context, ToolPackageVerificationCurrentIssuanceV1) (ToolPackageVerificationCurrentProjectionV1, error)
	InspectToolPackageVerificationCurrentByIssuanceV1(context.Context, ToolPackageVerificationCurrentIssuanceV1) (ToolPackageVerificationCurrentProjectionV1, error)
	InspectCurrentToolPackageVerificationV1(context.Context, ToolPackageVerificationCurrentRefV1) (ToolPackageVerificationCurrentProjectionV1, error)
}

type ToolPackageVerifiedAdmissionRequestV1 struct {
	ContractVersion          string                                     `json:"contract_version"`
	PackageCurrent           ToolPackageRegistryCurrentProjectionV1     `json:"package_current"`
	VerificationCurrent      ToolPackageVerificationCurrentProjectionV1 `json:"verification_current"`
	ExpectedRegistryRevision core.Revision                              `json:"expected_registry_revision"`
}

type ToolPackageAdmissionCommandV1 struct {
	ContractVersion          string                              `json:"contract_version"`
	VerificationCurrent      ToolPackageVerificationCurrentRefV1 `json:"verification_current"`
	ExpectedRegistryRevision core.Revision                       `json:"expected_registry_revision"`
}

func (r ToolPackageAdmissionCommandV1) Validate() error {
	if r.ContractVersion != PackageVerificationContractVersionV1 || r.VerificationCurrent.Validate() != nil || r.ExpectedRegistryRevision == 0 {
		return invalid("Package Admission command is incomplete")
	}
	return nil
}

func (r ToolPackageVerifiedAdmissionRequestV1) ValidateCurrent(now time.Time) error {
	if r.ContractVersion != PackageVerificationContractVersionV1 || r.PackageCurrent.ValidateCurrent(r.PackageCurrent.Ref, now) != nil || r.VerificationCurrent.ValidateCurrent(r.VerificationCurrent.Ref, now) != nil || r.ExpectedRegistryRevision == 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "verified Package Admission request is incomplete or expired")
	}
	if r.PackageCurrent.Ref != r.VerificationCurrent.CurrentPackageRegistry.Ref || r.PackageCurrent.Package != r.VerificationCurrent.Fact.Package || r.PackageCurrent.Manifest.ArtifactDigest != r.VerificationCurrent.CurrentPackageRegistry.Manifest.ArtifactDigest {
		return conflict("verified Package Admission closure drifted")
	}
	return nil
}

func validPackageRegistryStateV1(value string) bool {
	switch value {
	case "submitted", "admitted", "active", "deprecated", "revoked":
		return true
	default:
		return false
	}
}

func exactlyOneTextV1(left, right string) bool {
	return (left == "") != (right == "")
}

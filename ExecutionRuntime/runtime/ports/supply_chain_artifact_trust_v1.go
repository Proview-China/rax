package ports

import (
	"context"
	"io"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	SupplyChainArtifactTrustContractVersionV1 = "1.0.0"
	supplyChainArtifactTrustCanonicalDomainV1 = "praxis.runtime.supply-chain-artifact-trust/v1"
)

// SupplyChainArtifactContentRefV1 is a Runtime-neutral content coordinate.
// Artifact owners retain the bytes and repository semantics. URL, tag, latest,
// path and provider-native keys are intentionally absent.
type SupplyChainArtifactContentRefV1 struct {
	ContractVersion string      `json:"contract_version"`
	MediaType       string      `json:"media_type"`
	Digest          core.Digest `json:"digest"`
	Size            uint64      `json:"size"`
}

func (r SupplyChainArtifactContentRefV1) Validate() error {
	if r.ContractVersion != SupplyChainArtifactTrustContractVersionV1 || !validMediaType(r.MediaType) || r.Size == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "supply-chain artifact content ref is incomplete")
	}
	return r.Digest.Validate()
}

// SupplyChainArtifactExactReaderV1 opens bytes for one exact content Ref.
// Implementations must verify exact size and digest while reading, reject
// short/extra content and close failures, and must not perform a network fetch.
type SupplyChainArtifactExactReaderV1 interface {
	OpenExactSupplyChainArtifactV1(context.Context, SupplyChainArtifactContentRefV1) (io.ReadCloser, error)
}

// SupplyChainTrustMaterialRefV1 names immutable trust material owned by the
// Trust authority. It is nominally distinct from artifact content and policy.
type SupplyChainTrustMaterialRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r SupplyChainTrustMaterialRefV1) Validate() error {
	if r.ContractVersion != SupplyChainArtifactTrustContractVersionV1 || validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "supply-chain trust material ref is incomplete")
	}
	return r.Digest.Validate()
}

// SupplyChainTrustMaterialExactReaderV1 opens one exact immutable Trust Owner
// object. It is read-only and must not refresh roots or perform remote lookup.
type SupplyChainTrustMaterialExactReaderV1 interface {
	OpenExactSupplyChainTrustMaterialV1(context.Context, SupplyChainTrustMaterialRefV1) (io.ReadCloser, error)
}

// SupplyChainTrustPolicyDocumentRefV1 names the exact, versioned policy body
// interpreted by a domain adapter. Runtime does not interpret Sigstore or any
// other provider policy schema. It only owns this neutral bounded coordinate.
type SupplyChainTrustPolicyDocumentRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	MediaType       string        `json:"media_type"`
	Digest          core.Digest   `json:"digest"`
	Size            uint64        `json:"size"`
}

func (r SupplyChainTrustPolicyDocumentRefV1) Validate() error {
	if r.ContractVersion != SupplyChainArtifactTrustContractVersionV1 || validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || !validMediaType(r.MediaType) || r.Size == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "supply-chain trust policy document ref is incomplete")
	}
	return r.Digest.Validate()
}

type SupplyChainTrustPolicyDocumentExactReaderV1 interface {
	OpenExactSupplyChainTrustPolicyDocumentV1(context.Context, SupplyChainTrustPolicyDocumentRefV1) (io.ReadCloser, error)
}

// SupplyChainTrustPolicyRefV1 is the immutable historical policy coordinate.
type SupplyChainTrustPolicyRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r SupplyChainTrustPolicyRefV1) Validate() error {
	if r.ContractVersion != SupplyChainArtifactTrustContractVersionV1 || validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "supply-chain trust policy ref is incomplete")
	}
	return r.Digest.Validate()
}

// SupplyChainTrustPolicyCurrentRefV1 identifies one immutable current lease.
// Its Digest is the digest of the complete sealed projection, not the
// historical policy digest.
type SupplyChainTrustPolicyCurrentRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r SupplyChainTrustPolicyCurrentRefV1) Validate() error {
	if r.ContractVersion != SupplyChainArtifactTrustContractVersionV1 || validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "supply-chain trust policy current ref is incomplete")
	}
	return r.Digest.Validate()
}

// SupplyChainTrustPolicyCurrentProjectionV1 is an immutable Trust/Policy
// Owner projection. Checked and Expires are sealed once; Inspect never refreshes
// the same Ref. The digest is computed with both digest fields empty.
type SupplyChainTrustPolicyCurrentProjectionV1 struct {
	ContractVersion          string                              `json:"contract_version"`
	Ref                      SupplyChainTrustPolicyCurrentRefV1  `json:"ref"`
	Policy                   SupplyChainTrustPolicyRefV1         `json:"policy"`
	PolicyDocument           SupplyChainTrustPolicyDocumentRefV1 `json:"policy_document"`
	TrustedRoot              SupplyChainTrustMaterialRefV1       `json:"trusted_root"`
	IdentityPolicyDigest     core.Digest                         `json:"identity_policy_digest"`
	PredicatePolicyDigest    core.Digest                         `json:"predicate_policy_digest"`
	TransparencyPolicyDigest core.Digest                         `json:"transparency_policy_digest"`
	TimestampPolicyDigest    core.Digest                         `json:"timestamp_policy_digest"`
	MaxPackageArtifactBytes  uint64                              `json:"max_package_artifact_bytes"`
	MaxSigstoreBundleBytes   uint64                              `json:"max_sigstore_bundle_bytes"`
	MaxInTotoStatementBytes  uint64                              `json:"max_in_toto_statement_bytes"`
	MaxTrustMaterialBytes    uint64                              `json:"max_trust_material_bytes"`
	CheckedUnixNano          int64                               `json:"checked_unix_nano"`
	ExpiresUnixNano          int64                               `json:"expires_unix_nano"`
	ProjectionDigest         core.Digest                         `json:"projection_digest"`
}

func (p SupplyChainTrustPolicyCurrentProjectionV1) Validate() error {
	if p.ContractVersion != SupplyChainArtifactTrustContractVersionV1 || p.Ref.Validate() != nil || p.Policy.Validate() != nil || p.PolicyDocument.Validate() != nil || p.TrustedRoot.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "supply-chain trust policy current projection identity is incomplete")
	}
	for _, digest := range []core.Digest{p.IdentityPolicyDigest, p.PredicatePolicyDigest, p.TransparencyPolicyDigest, p.TimestampPolicyDigest, p.ProjectionDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if p.MaxPackageArtifactBytes == 0 || p.MaxSigstoreBundleBytes == 0 || p.MaxInTotoStatementBytes == 0 || p.MaxTrustMaterialBytes == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "supply-chain trust policy size bounds are required")
	}
	if p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingExpired, "supply-chain trust policy current window is invalid")
	}
	if p.Ref.Digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "supply-chain trust policy current ref and projection digest drifted")
	}
	digest, err := DigestSupplyChainTrustPolicyCurrentProjectionV1(p)
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "supply-chain trust policy current projection digest drifted")
	}
	return nil
}

func (p SupplyChainTrustPolicyCurrentProjectionV1) ValidateCurrent(expected SupplyChainTrustPolicyCurrentRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if p.Ref != expected {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "supply-chain trust policy current ref drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "supply-chain trust policy current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "supply-chain trust policy current projection expired")
	}
	return nil
}

func DigestSupplyChainTrustPolicyCurrentProjectionV1(p SupplyChainTrustPolicyCurrentProjectionV1) (core.Digest, error) {
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(supplyChainArtifactTrustCanonicalDomainV1, SupplyChainArtifactTrustContractVersionV1, "SupplyChainTrustPolicyCurrentProjectionV1", p)
}

func SealSupplyChainTrustPolicyCurrentProjectionV1(p SupplyChainTrustPolicyCurrentProjectionV1) (SupplyChainTrustPolicyCurrentProjectionV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != SupplyChainArtifactTrustContractVersionV1 {
		return SupplyChainTrustPolicyCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidSemanticVersion, "supply-chain trust policy contract version is invalid")
	}
	if p.Ref.ContractVersion != "" && p.Ref.ContractVersion != SupplyChainArtifactTrustContractVersionV1 {
		return SupplyChainTrustPolicyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "supply-chain trust policy current ref contract version drifted")
	}
	p.ContractVersion = SupplyChainArtifactTrustContractVersionV1
	p.Ref.ContractVersion = SupplyChainArtifactTrustContractVersionV1
	providedRefDigest := p.Ref.Digest
	providedProjectionDigest := p.ProjectionDigest
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	digest, err := DigestSupplyChainTrustPolicyCurrentProjectionV1(p)
	if err != nil {
		return SupplyChainTrustPolicyCurrentProjectionV1{}, err
	}
	if providedRefDigest != "" && providedRefDigest != digest {
		return SupplyChainTrustPolicyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "supplied trust policy current ref digest drifted")
	}
	if providedProjectionDigest != "" && providedProjectionDigest != digest {
		return SupplyChainTrustPolicyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "supplied trust policy projection digest drifted")
	}
	p.Ref.Digest = digest
	p.ProjectionDigest = digest
	return p, p.Validate()
}

type SupplyChainTrustPolicyCurrentReaderV1 interface {
	InspectCurrentSupplyChainTrustPolicyV1(context.Context, SupplyChainTrustPolicyCurrentRefV1) (SupplyChainTrustPolicyCurrentProjectionV1, error)
}

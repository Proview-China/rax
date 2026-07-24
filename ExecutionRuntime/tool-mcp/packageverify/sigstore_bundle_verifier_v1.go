package packageverify

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"strings"
	"time"

	intotov1 "github.com/in-toto/attestation/go/v1"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	sigverify "github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/sigstore/sigstore/pkg/signature"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type SigstoreVerificationObservationV1 struct {
	SignerIdentityDigest core.Digest `json:"signer_identity_digest"`
	PredicateType        string      `json:"predicate_type"`
}

type SigstoreBundleVerifierV1 struct{}

func (SigstoreBundleVerifierV1) VerifyV1(ctx context.Context, binding toolcontract.ToolPackageArtifactBindingV1, manifest, artifact, bundleJSON, statementJSON, trustedRootJSON, policyJSON []byte) (SigstoreVerificationObservationV1, error) {
	if isNilV1(ctx) || binding.Validate() != nil {
		return SigstoreVerificationObservationV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Sigstore verification request is invalid")
	}
	if err := ctx.Err(); err != nil {
		return SigstoreVerificationObservationV1{}, err
	}
	policy, err := parseTrustPolicyV1(policyJSON)
	if err != nil {
		return SigstoreVerificationObservationV1{}, err
	}
	if err := validateOCIManifestV1(manifest, binding); err != nil {
		return SigstoreVerificationObservationV1{}, err
	}
	statement, err := parseStatementV1(statementJSON, binding.PackageArtifact.Digest)
	if err != nil {
		return SigstoreVerificationObservationV1{}, err
	}
	if !containsStringV1(policy.RequiredPredicateTypes, statement.GetPredicateType()) {
		return SigstoreVerificationObservationV1{}, core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "in-toto predicate type is not allowed by Trust Policy")
	}

	trusted, err := trustedMaterialV1(policy, trustedRootJSON)
	if err != nil {
		return SigstoreVerificationObservationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceTrustInvalid, "Sigstore trusted root is invalid")
	}
	entity := &bundle.Bundle{}
	if err := entity.UnmarshalJSON(bundleJSON); err != nil {
		return SigstoreVerificationObservationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceTrustInvalid, "Sigstore Bundle is invalid")
	}
	verifierOptions, err := verifierOptionsV1(policy)
	if err != nil {
		return SigstoreVerificationObservationV1{}, err
	}
	verifier, err := sigverify.NewVerifier(trusted, verifierOptions...)
	if err != nil {
		return SigstoreVerificationObservationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceTrustInvalid, "Sigstore verifier policy is invalid")
	}
	policyOptions, err := identityPolicyOptionsV1(policy)
	if err != nil {
		return SigstoreVerificationObservationV1{}, err
	}
	result, err := verifier.Verify(entity, sigverify.NewPolicy(sigverify.WithArtifact(bytes.NewReader(artifact)), policyOptions...))
	if err != nil {
		return SigstoreVerificationObservationV1{}, core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "Sigstore verification failed")
	}
	if result.Statement == nil || !proto.Equal(result.Statement, statement) || result.Statement.GetPredicateType() != statement.GetPredicateType() {
		return SigstoreVerificationObservationV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceChainConflict, "Sigstore Bundle statement differs from exact in-toto Statement")
	}
	signerDigest, err := signerIdentityDigestV1(result)
	if err != nil {
		return SigstoreVerificationObservationV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return SigstoreVerificationObservationV1{}, err
	}
	return SigstoreVerificationObservationV1{SignerIdentityDigest: signerDigest, PredicateType: statement.GetPredicateType()}, nil
}

func parseTrustPolicyV1(raw []byte) (toolcontract.ToolPackageTrustPolicyDocumentV1, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var policy toolcontract.ToolPackageTrustPolicyDocumentV1
	if err := decoder.Decode(&policy); err != nil {
		return policy, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Trust Policy document JSON is invalid")
	}
	if err := ensureJSONEOFV1(decoder); err != nil {
		return policy, err
	}
	if err := policy.Validate(); err != nil {
		return policy, err
	}
	return policy, nil
}

func ensureJSONEOFV1(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "JSON document has trailing content")
	}
	return nil
}

func validateOCIManifestV1(raw []byte, binding toolcontract.ToolPackageArtifactBindingV1) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest ociv1.Manifest
	if err := decoder.Decode(&manifest); err != nil || ensureJSONEOFV1(decoder) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "OCI Manifest JSON is invalid")
	}
	if manifest.SchemaVersion != 2 || manifest.MediaType != ociv1.MediaTypeImageManifest || manifest.ArtifactType != binding.ArtifactType {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceChainConflict, "OCI Manifest type or schema drifted")
	}
	wantDigest := string(binding.PackageArtifact.Digest)
	matches := 0
	for _, layer := range manifest.Layers {
		if layer.Digest.String() == wantDigest && uint64(layer.Size) == binding.PackageArtifact.Size && layer.MediaType == binding.PackageArtifact.MediaType {
			matches++
		}
	}
	if matches != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceChainConflict, "OCI Manifest does not exact-bind one Package Artifact")
	}
	return nil
}

func parseStatementV1(raw []byte, artifactDigest core.Digest) (*intotov1.Statement, error) {
	statement := &intotov1.Statement{}
	if err := protojson.Unmarshal(raw, statement); err != nil || statement.Validate() != nil || len(statement.GetSubject()) != 1 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "in-toto Statement V1 is invalid")
	}
	want := strings.TrimPrefix(string(artifactDigest), "sha256:")
	digests := statement.GetSubject()[0].GetDigest()
	if len(digests) != 1 || digests["sha256"] != want {
		return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceChainConflict, "in-toto Statement subject differs from Package Artifact")
	}
	return statement, nil
}

func verifierOptionsV1(policy toolcontract.ToolPackageTrustPolicyDocumentV1) ([]sigverify.VerifierOption, error) {
	options := make([]sigverify.VerifierOption, 0, 3)
	if policy.TransparencyLogThreshold > 0 {
		options = append(options, sigverify.WithTransparencyLog(int(policy.TransparencyLogThreshold)))
	}
	switch policy.TimestampMode {
	case toolcontract.ToolPackageSigstoreObserverTimestampV1:
		options = append(options, sigverify.WithObserverTimestamps(int(policy.TimestampThreshold)))
	case toolcontract.ToolPackageSigstoreSignedTimestampV1:
		options = append(options, sigverify.WithSignedTimestamps(int(policy.TimestampThreshold)))
	case toolcontract.ToolPackageSigstoreIntegratedTimestampV1:
		options = append(options, sigverify.WithIntegratedTimestamps(int(policy.TimestampThreshold)))
	case toolcontract.ToolPackageSigstoreNoTimestampForKeyV1:
		options = append(options, sigverify.WithNoObserverTimestamps())
	default:
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "unsupported Sigstore timestamp mode")
	}
	if policy.SCTThreshold > 0 {
		options = append(options, sigverify.WithSignedCertificateTimestamps(int(policy.SCTThreshold)))
	}
	return options, nil
}

func trustedMaterialV1(policy toolcontract.ToolPackageTrustPolicyDocumentV1, raw []byte) (root.TrustedMaterial, error) {
	if policy.IdentityMode == toolcontract.ToolPackageSigstoreCertificateV1 {
		return root.NewTrustedRootFromJSON(raw)
	}
	block, rest := pem.Decode(raw)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Sigstore public key PEM is invalid")
	}
	publicKey, err := cryptoutils.UnmarshalPEMToPublicKey(pem.EncodeToMemory(block))
	if err != nil {
		return nil, err
	}
	return root.NewTrustedPublicKeyMaterial(func(string) (root.TimeConstrainedVerifier, error) {
		verifier, err := signature.LoadVerifier(publicKey, crypto.SHA256)
		if err != nil {
			return nil, err
		}
		return nonExpiringPublicKeyVerifierV1{Verifier: verifier}, nil
	}), nil
}

type nonExpiringPublicKeyVerifierV1 struct{ signature.Verifier }

func (nonExpiringPublicKeyVerifierV1) ValidAtTime(time.Time) bool { return true }

func identityPolicyOptionsV1(policy toolcontract.ToolPackageTrustPolicyDocumentV1) ([]sigverify.PolicyOption, error) {
	if policy.IdentityMode == toolcontract.ToolPackageSigstoreKeyV1 {
		return []sigverify.PolicyOption{sigverify.WithKey()}, nil
	}
	options := make([]sigverify.PolicyOption, 0, len(policy.CertificateIdentities))
	for _, exact := range policy.CertificateIdentities {
		identity, err := sigverify.NewShortCertificateIdentity(exact.Issuer, exact.IssuerRegex, exact.SANValue, exact.SANRegex)
		if err != nil {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Sigstore certificate identity policy is invalid")
		}
		options = append(options, sigverify.WithCertificateIdentity(identity))
	}
	return options, nil
}

func signerIdentityDigestV1(result *sigverify.VerificationResult) (core.Digest, error) {
	if result == nil || result.Signature == nil {
		return "", core.NewError(core.ErrorConflict, core.ReasonEvidenceTrustInvalid, "Sigstore result has no verified signature identity")
	}
	if result.Signature.Certificate != nil {
		return core.CanonicalJSONDigest("praxis.tool-mcp.package-verification", toolcontract.PackageVerificationContractVersionV1, "SigstoreCertificateIdentityV1", result.Signature.Certificate)
	}
	if result.Signature.PublicKeyID != nil && len(*result.Signature.PublicKeyID) > 0 {
		return core.CanonicalJSONDigest("praxis.tool-mcp.package-verification", toolcontract.PackageVerificationContractVersionV1, "SigstorePublicKeyIdentityV1", struct {
			KeyID string `json:"key_id"`
		}{hex.EncodeToString(*result.Signature.PublicKeyID)})
	}
	return "", core.NewError(core.ErrorConflict, core.ReasonEvidenceTrustInvalid, "Sigstore result identity is empty")
}

func containsStringV1(values []string, target string) bool {
	index := sortSearchStringsV1(values, target)
	return index < len(values) && values[index] == target
}

func sortSearchStringsV1(values []string, target string) int {
	low, high := 0, len(values)
	for low < high {
		middle := int(uint(low+high) >> 1)
		if values[middle] < target {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return low
}

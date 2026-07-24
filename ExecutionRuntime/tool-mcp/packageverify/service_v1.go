package packageverify

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type CryptographicVerifierV1 interface {
	VerifyV1(context.Context, toolcontract.ToolPackageArtifactBindingV1, []byte, []byte, []byte, []byte, []byte, []byte) (SigstoreVerificationObservationV1, error)
}

type ServiceConfigV1 struct {
	Artifacts    runtimeports.SupplyChainArtifactExactReaderV1
	Trust        runtimeports.SupplyChainTrustMaterialExactReaderV1
	Policies     runtimeports.SupplyChainTrustPolicyDocumentExactReaderV1
	TrustCurrent runtimeports.SupplyChainTrustPolicyCurrentReaderV1
	Packages     toolcontract.ToolPackageRegistryCurrentReaderV1
	Repository   *RepositoryV1
	Verifier     CryptographicVerifierV1
	Clock        ClockV1
}

type ServiceV1 struct{ config ServiceConfigV1 }

func NewServiceV1(config ServiceConfigV1) (*ServiceV1, error) {
	for _, dependency := range []any{config.Artifacts, config.Trust, config.Policies, config.TrustCurrent, config.Packages, config.Repository, config.Verifier, config.Clock} {
		if isNilV1(dependency) {
			return nil, packageInvalidV1("package verification service dependency is nil")
		}
	}
	return &ServiceV1{config: config}, nil
}

func (s *ServiceV1) VerifyV1(ctx context.Context, request toolcontract.ToolPackageVerifyRequestV1) (toolcontract.ToolPackageVerificationFactV1, error) {
	if isNilV1(ctx) || s == nil || isNilV1(s.config.Clock) {
		return toolcontract.ToolPackageVerificationFactV1{}, packageInvalidV1("package Verify context or service is nil")
	}
	nowS1 := s.config.Clock.Now()
	if err := request.ValidateCurrent(nowS1); err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}
	bounded, cancel, err := boundedContextV1(ctx, request.RequestedExpiresUnixNano)
	if err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}
	defer cancel()

	packageS1, trustS1, policy, materials, err := s.readClosureV1(bounded, request, nowS1)
	if err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}
	observed, err := s.config.Verifier.VerifyV1(bounded, request.Subject.ArtifactBinding, materials.ociManifest, materials.packageArtifact, materials.sigstoreBundle, materials.inTotoStatement, materials.trustedRoot, materials.policyDocument)
	if err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}
	observationRequest := toolcontract.ToolPackageVerificationObservationEnsureRequestV1{
		ContractVersion: toolcontract.PackageVerificationContractVersionV1, Subject: request.Subject,
		TrustPolicyCurrent: request.TrustPolicyCurrent, TrustedRoot: trustS1.TrustedRoot, PolicyDocument: trustS1.PolicyDocument,
		IdentityPolicyDigest: policy.IdentityPolicyDigest, PredicatePolicyDigest: policy.PredicatePolicyDigest,
		TransparencyPolicyDigest: policy.TransparencyPolicyDigest, TimestampPolicyDigest: policy.TimestampPolicyDigest,
		SignerIdentityDigest: observed.SignerIdentityDigest, PredicateType: observed.PredicateType,
		VerifierConformance: toolcontract.PackageVerifierConformanceV1,
	}
	observation, err := s.config.Repository.EnsureToolPackageVerificationObservationV1(bounded, observationRequest)
	if err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}

	nowS2 := s.config.Clock.Now()
	if nowS2.Before(nowS1) {
		return toolcontract.ToolPackageVerificationFactV1{}, packageClockV1("package verification clock regressed before S2")
	}
	packageS2, trustS2, policyS2, materialsS2, err := s.readClosureV1(bounded, request, nowS2)
	if err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}
	if !samePackageCurrentSourceV1(packageS1, packageS2) || !reflect.DeepEqual(trustS1, trustS2) || !reflect.DeepEqual(policy, policyS2) || !reflect.DeepEqual(materials, materialsS2) {
		return toolcontract.ToolPackageVerificationFactV1{}, packageConflictV1("package verification S1/S2 closure drifted")
	}
	factRequest := toolcontract.ToolPackageVerificationFactEnsureRequestV1{
		ContractVersion: toolcontract.PackageVerificationContractVersionV1,
		Subject:         request.Subject, Observation: observation.Ref,
	}
	fact, err := s.config.Repository.EnsureToolPackageVerificationFactV1(bounded, factRequest)
	if err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}
	return fact, nil
}

func (s *ServiceV1) ResolveCurrentToolPackageVerificationV1(ctx context.Context, issuance toolcontract.ToolPackageVerificationCurrentIssuanceV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	if isNilV1(ctx) || s == nil || isNilV1(s.config.Clock) {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, packageInvalidV1("package current Resolve context or service is nil")
	}
	now := s.config.Clock.Now()
	if err := issuance.ValidateCurrent(now); err != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
	}
	if winner, err := s.config.Repository.InspectToolPackageVerificationCurrentByIssuanceV1(ctx, issuance); err == nil {
		if err := winner.ValidateCurrent(winner.Ref, now); err != nil {
			return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
		}
		return winner, nil
	} else if !core.HasCategory(err, core.ErrorNotFound) {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
	}
	bounded, cancel, err := boundedContextV1(ctx, issuance.RequestedExpiresUnixNano)
	if err != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
	}
	defer cancel()
	fact, err := s.config.Repository.InspectExactToolPackageVerificationFactV1(bounded, issuance.Fact)
	if err != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
	}
	observation, err := s.config.Repository.InspectExactToolPackageVerificationObservationV1(bounded, fact.Observation)
	if err != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
	}
	request := toolcontract.ToolPackageVerifyRequestV1{
		ContractVersion: toolcontract.PackageVerificationContractVersionV1, Subject: observation.Request.Subject,
		TrustPolicyCurrent: issuance.TrustPolicyCurrent, RequestedExpiresUnixNano: issuance.RequestedExpiresUnixNano,
	}
	if issuance.PackageRegistry != request.Subject.PackageRegistry || fact.PackageRegistry.ID == "" || fact.Package != request.Subject.ArtifactBinding.Package || fact.TrustPolicy != request.Subject.TrustPolicy {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, packageConflictV1("package current issuance differs from historical Fact closure")
	}
	packageCurrent, trustCurrent, _, _, err := s.readClosureV1(bounded, request, now)
	if err != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
	}
	checked := s.config.Clock.Now()
	if checked.Before(now) {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, packageClockV1("package current clock regressed before seal")
	}
	expires := minimumUnixNanoV1(issuance.RequestedExpiresUnixNano, packageCurrent.ExpiresUnixNano, trustCurrent.ExpiresUnixNano)
	if deadline, ok := bounded.Deadline(); ok {
		expires = minimumUnixNanoV1(expires, deadline.UnixNano())
	}
	projection, err := toolcontract.SealToolPackageVerificationCurrentProjectionV1(toolcontract.ToolPackageVerificationCurrentProjectionV1{
		Issuance: issuance, Fact: fact, CurrentPackageRegistry: packageCurrent, TrustPolicy: trustCurrent,
		CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
	}
	winner, err := s.config.Repository.ensureCurrentV1(bounded, projection)
	if err != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
	}
	return winner, winner.ValidateCurrent(winner.Ref, s.config.Clock.Now())
}

func (s *ServiceV1) InspectToolPackageVerificationCurrentByIssuanceV1(ctx context.Context, issuance toolcontract.ToolPackageVerificationCurrentIssuanceV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	return s.config.Repository.InspectToolPackageVerificationCurrentByIssuanceV1(ctx, issuance)
}

func (s *ServiceV1) InspectCurrentToolPackageVerificationV1(ctx context.Context, exact toolcontract.ToolPackageVerificationCurrentRefV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	if isNilV1(ctx) || s == nil || isNilV1(s.config.Clock) {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, packageInvalidV1("package current Inspect context or service is nil")
	}
	winner, err := s.config.Repository.InspectCurrentToolPackageVerificationV1(ctx, exact)
	if err != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
	}
	return winner, winner.ValidateCurrent(exact, s.config.Clock.Now())
}

type materialClosureV1 struct {
	ociManifest     []byte
	packageArtifact []byte
	sigstoreBundle  []byte
	inTotoStatement []byte
	trustedRoot     []byte
	policyDocument  []byte
}

func (s *ServiceV1) readClosureV1(ctx context.Context, request toolcontract.ToolPackageVerifyRequestV1, now time.Time) (toolcontract.ToolPackageRegistryCurrentProjectionV1, runtimeports.SupplyChainTrustPolicyCurrentProjectionV1, toolcontract.ToolPackageTrustPolicyDocumentV1, materialClosureV1, error) {
	packageCurrent, err := s.config.Packages.InspectCurrentToolPackageRegistryV1(ctx, request.Subject.PackageRegistry)
	if err != nil {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{}, toolcontract.ToolPackageTrustPolicyDocumentV1{}, materialClosureV1{}, err
	}
	trustCurrent, err := s.config.TrustCurrent.InspectCurrentSupplyChainTrustPolicyV1(ctx, request.TrustPolicyCurrent)
	if err != nil {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{}, toolcontract.ToolPackageTrustPolicyDocumentV1{}, materialClosureV1{}, err
	}
	if packageCurrent.ValidateCurrent(request.Subject.PackageRegistry, now) != nil || trustCurrent.ValidateCurrent(request.TrustPolicyCurrent, now) != nil || trustCurrent.Policy != request.Subject.TrustPolicy || request.Subject.ArtifactBinding.ValidateAgainst(packageCurrent.Manifest) != nil || trustCurrent.PolicyDocument.MediaType != toolcontract.PackageTrustPolicyMediaTypeV1 {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{}, toolcontract.ToolPackageTrustPolicyDocumentV1{}, materialClosureV1{}, packageConflictV1("package, Trust Policy or Artifact binding current closure drifted")
	}
	binding := request.Subject.ArtifactBinding
	materials := materialClosureV1{}
	if materials.ociManifest, err = ReadExactArtifactV1(ctx, s.config.Artifacts, binding.OCIManifest, trustCurrent.MaxPackageArtifactBytes); err != nil {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{}, toolcontract.ToolPackageTrustPolicyDocumentV1{}, materialClosureV1{}, err
	}
	if materials.packageArtifact, err = ReadExactArtifactV1(ctx, s.config.Artifacts, binding.PackageArtifact, trustCurrent.MaxPackageArtifactBytes); err != nil {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{}, toolcontract.ToolPackageTrustPolicyDocumentV1{}, materialClosureV1{}, err
	}
	if materials.sigstoreBundle, err = ReadExactArtifactV1(ctx, s.config.Artifacts, binding.SigstoreBundle, trustCurrent.MaxSigstoreBundleBytes); err != nil {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{}, toolcontract.ToolPackageTrustPolicyDocumentV1{}, materialClosureV1{}, err
	}
	if materials.inTotoStatement, err = ReadExactArtifactV1(ctx, s.config.Artifacts, binding.InTotoStatement, trustCurrent.MaxInTotoStatementBytes); err != nil {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{}, toolcontract.ToolPackageTrustPolicyDocumentV1{}, materialClosureV1{}, err
	}
	if materials.trustedRoot, err = ReadExactTrustMaterialV1(ctx, s.config.Trust, trustCurrent.TrustedRoot, trustCurrent.MaxTrustMaterialBytes); err != nil {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{}, toolcontract.ToolPackageTrustPolicyDocumentV1{}, materialClosureV1{}, err
	}
	if materials.policyDocument, err = ReadExactTrustPolicyDocumentV1(ctx, s.config.Policies, trustCurrent.PolicyDocument, trustCurrent.MaxTrustMaterialBytes); err != nil {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{}, toolcontract.ToolPackageTrustPolicyDocumentV1{}, materialClosureV1{}, err
	}
	policy, err := parseTrustPolicyV1(materials.policyDocument)
	if err != nil || policy.IdentityPolicyDigest != trustCurrent.IdentityPolicyDigest || policy.PredicatePolicyDigest != trustCurrent.PredicatePolicyDigest || policy.TransparencyPolicyDigest != trustCurrent.TransparencyPolicyDigest || policy.TimestampPolicyDigest != trustCurrent.TimestampPolicyDigest {
		return toolcontract.ToolPackageRegistryCurrentProjectionV1{}, runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{}, toolcontract.ToolPackageTrustPolicyDocumentV1{}, materialClosureV1{}, packageConflictV1("Trust Policy document differs from current projection")
	}
	return packageCurrent, trustCurrent, policy, materials, nil
}

func boundedContextV1(ctx context.Context, requested int64) (context.Context, context.CancelFunc, error) {
	if isNilV1(ctx) || requested <= 0 {
		return nil, nil, packageInvalidV1("bounded package verification context is invalid")
	}
	deadline := time.Unix(0, requested)
	if existing, ok := ctx.Deadline(); ok && existing.Before(deadline) {
		deadline = existing
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	return bounded, cancel, nil
}

func samePackageCurrentSourceV1(left, right toolcontract.ToolPackageRegistryCurrentProjectionV1) bool {
	return left.Ref == right.Ref && left.Source == right.Source && left.Package == right.Package && reflect.DeepEqual(left.Manifest, right.Manifest) && left.State == right.State && left.RegistryRevision == right.RegistryRevision
}

func minimumUnixNanoV1(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

var _ toolcontract.ToolPackageVerificationCurrentResolverV1 = (*ServiceV1)(nil)

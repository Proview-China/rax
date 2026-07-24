package testkit

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type PackageVerificationFixtureV1 struct {
	Request     toolcontract.ToolPackageVerifyRequestV1
	Observation toolcontract.ToolPackageVerificationObservationV1
	Fact        toolcontract.ToolPackageVerificationFactV1
	Current     toolcontract.ToolPackageVerificationCurrentProjectionV1
}

// PackageVerificationV1 returns sealed fake facts for SDK/API tests. It is not
// cryptographic evidence, a production Trust Policy or a production backend.
func PackageVerificationV1() PackageVerificationFixtureV1 {
	pkg := Package()
	source, err := toolcontract.SealToolPackageRegistryRecordSourceV1(toolcontract.ToolPackageRegistryRecordSourceV1{
		Kind: "package", ID: string(pkg.ID), ObjectRevision: pkg.Revision, ObjectDigest: pkg.Digest,
		State: "submitted", RegistryRevision: 7, UpdatedUnixNano: FixedTime.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	pkgRef := toolcontract.ObjectRef{ID: string(pkg.ID), Revision: pkg.Revision, Digest: pkg.Digest}
	currentID, err := toolcontract.DeriveToolPackageRegistryCurrentIDV1(pkgRef)
	if err != nil {
		panic(err)
	}
	pkgCurrent, err := toolcontract.SealToolPackageRegistryCurrentProjectionV1(toolcontract.ToolPackageRegistryCurrentProjectionV1{
		Ref: toolcontract.ToolPackageRegistryCurrentRefV1{
			ContractVersion: toolcontract.PackageVerificationContractVersionV1,
			ID:              currentID,
			Revision:        source.RegistryRevision,
			Digest:          source.Digest,
		},
		Source: source, Package: pkgRef, Manifest: pkg, State: source.State, RegistryRevision: source.RegistryRevision,
		CheckedUnixNano: FixedTime.UnixNano(), ExpiresUnixNano: FixedTime.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	policy := runtimeports.SupplyChainTrustPolicyRefV1{
		ContractVersion: runtimeports.SupplyChainArtifactTrustContractVersionV1,
		ID:              "policy-test",
		Revision:        1,
		Digest:          Digest("policy"),
	}
	policyDocument := runtimeports.SupplyChainTrustPolicyDocumentRefV1{
		ContractVersion: runtimeports.SupplyChainArtifactTrustContractVersionV1,
		ID:              "policy-document-test",
		Revision:        1,
		MediaType:       toolcontract.PackageTrustPolicyMediaTypeV1,
		Digest:          Digest("policy-document"),
		Size:            128,
	}
	root := runtimeports.SupplyChainTrustMaterialRefV1{
		ContractVersion: runtimeports.SupplyChainArtifactTrustContractVersionV1,
		ID:              "trust-material-test",
		Revision:        1,
		Digest:          Digest("trust-material"),
	}
	trustCurrent, err := runtimeports.SealSupplyChainTrustPolicyCurrentProjectionV1(runtimeports.SupplyChainTrustPolicyCurrentProjectionV1{
		Ref:                      runtimeports.SupplyChainTrustPolicyCurrentRefV1{ID: "trust-current-test", Revision: 1},
		Policy:                   policy,
		PolicyDocument:           policyDocument,
		TrustedRoot:              root,
		IdentityPolicyDigest:     Digest("identity-policy"),
		PredicatePolicyDigest:    Digest("predicate-policy"),
		TransparencyPolicyDigest: Digest("transparency-policy"),
		TimestampPolicyDigest:    Digest("timestamp-policy"),
		MaxPackageArtifactBytes:  1 << 20,
		MaxSigstoreBundleBytes:   1 << 20,
		MaxInTotoStatementBytes:  1 << 20,
		MaxTrustMaterialBytes:    1 << 20,
		CheckedUnixNano:          FixedTime.UnixNano(),
		ExpiresUnixNano:          FixedTime.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	content := func(media string, digest core.Digest) runtimeports.SupplyChainArtifactContentRefV1 {
		return runtimeports.SupplyChainArtifactContentRefV1{
			ContractVersion: runtimeports.SupplyChainArtifactTrustContractVersionV1,
			MediaType:       media,
			Digest:          digest,
			Size:            1,
		}
	}
	binding, err := toolcontract.SealToolPackageArtifactBindingV1(toolcontract.ToolPackageArtifactBindingV1{
		Package:         pkgRef,
		OCIManifest:     content("application/vnd.oci.image.manifest.v1+json", Digest("oci-manifest")),
		PackageArtifact: content("application/octet-stream", pkg.ArtifactDigest),
		SigstoreBundle:  content("application/vnd.dev.sigstore.bundle.v0.3+json", Digest("sigstore-bundle")),
		InTotoStatement: content("application/vnd.in-toto+json", Digest("in-toto-statement")),
		ArtifactType:    toolcontract.PackageArtifactTypeV1,
	})
	if err != nil {
		panic(err)
	}
	request := toolcontract.ToolPackageVerifyRequestV1{
		ContractVersion: toolcontract.PackageVerificationContractVersionV1,
		Subject: toolcontract.ToolPackageVerificationSubjectV1{
			ContractVersion: toolcontract.PackageVerificationContractVersionV1,
			PackageRegistry: pkgCurrent.Ref,
			ArtifactBinding: binding,
			TrustPolicy:     policy,
			VerifierProfile: toolcontract.PackageVerifierConformanceV1,
		},
		TrustPolicyCurrent:       trustCurrent.Ref,
		RequestedExpiresUnixNano: FixedTime.Add(30 * time.Minute).UnixNano(),
	}
	observation, err := toolcontract.SealToolPackageVerificationObservationV1(toolcontract.ToolPackageVerificationObservationV1{
		Request: toolcontract.ToolPackageVerificationObservationEnsureRequestV1{
			ContractVersion:          toolcontract.PackageVerificationContractVersionV1,
			Subject:                  request.Subject,
			TrustPolicyCurrent:       trustCurrent.Ref,
			TrustedRoot:              root,
			PolicyDocument:           policyDocument,
			IdentityPolicyDigest:     trustCurrent.IdentityPolicyDigest,
			PredicatePolicyDigest:    trustCurrent.PredicatePolicyDigest,
			TransparencyPolicyDigest: trustCurrent.TransparencyPolicyDigest,
			TimestampPolicyDigest:    trustCurrent.TimestampPolicyDigest,
			SignerIdentityDigest:     Digest("signer"),
			PredicateType:            "https://slsa.dev/provenance/v1",
			VerifierConformance:      toolcontract.PackageVerifierConformanceV1,
		},
		ObservedUnixNano: FixedTime.Add(time.Second).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	fact, err := toolcontract.SealToolPackageVerificationFactV1(toolcontract.ToolPackageVerificationFactV1{
		Package:               pkgRef,
		PackageRegistry:       pkgCurrent.Ref,
		ArtifactBindingDigest: binding.BindingDigest,
		TrustPolicy:           policy,
		Observation:           observation.Ref,
		SignerIdentityDigest:  observation.Request.SignerIdentityDigest,
		PredicateType:         observation.Request.PredicateType,
		VerifierConformance:   observation.Request.VerifierConformance,
		VerifiedUnixNano:      FixedTime.Add(2 * time.Second).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	issuance := toolcontract.ToolPackageVerificationCurrentIssuanceV1{
		ContractVersion:          toolcontract.PackageVerificationContractVersionV1,
		Fact:                     fact.Ref,
		PackageRegistry:          pkgCurrent.Ref,
		TrustPolicyCurrent:       trustCurrent.Ref,
		RequestedExpiresUnixNano: request.RequestedExpiresUnixNano,
	}
	current, err := toolcontract.SealToolPackageVerificationCurrentProjectionV1(toolcontract.ToolPackageVerificationCurrentProjectionV1{
		Issuance:               issuance,
		Fact:                   fact,
		CurrentPackageRegistry: pkgCurrent,
		TrustPolicy:            trustCurrent,
		CheckedUnixNano:        FixedTime.Add(3 * time.Second).UnixNano(),
		ExpiresUnixNano:        request.RequestedExpiresUnixNano,
	})
	if err != nil {
		panic(err)
	}
	return PackageVerificationFixtureV1{Request: request, Observation: observation, Fact: fact, Current: current}
}

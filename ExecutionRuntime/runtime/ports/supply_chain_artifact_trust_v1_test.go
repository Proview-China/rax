package ports

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const supplyChainTestDigestV1 core.Digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func validSupplyChainTrustPolicyProjectionV1(t *testing.T) SupplyChainTrustPolicyCurrentProjectionV1 {
	t.Helper()
	checked := time.Unix(1_800_000_000, 0)
	projection, err := SealSupplyChainTrustPolicyCurrentProjectionV1(SupplyChainTrustPolicyCurrentProjectionV1{
		Ref: SupplyChainTrustPolicyCurrentRefV1{
			ID:       "trust-policy-current-test",
			Revision: 3,
		},
		Policy: SupplyChainTrustPolicyRefV1{
			ContractVersion: SupplyChainArtifactTrustContractVersionV1,
			ID:              "trust-policy-test",
			Revision:        7,
			Digest:          supplyChainTestDigestV1,
		},
		PolicyDocument: SupplyChainTrustPolicyDocumentRefV1{
			ContractVersion: SupplyChainArtifactTrustContractVersionV1,
			ID:              "trust-policy-document-test",
			Revision:        7,
			MediaType:       "application/vnd.praxis.sigstore-policy.v1+json",
			Digest:          supplyChainTestDigestV1,
			Size:            2048,
		},
		TrustedRoot: SupplyChainTrustMaterialRefV1{
			ContractVersion: SupplyChainArtifactTrustContractVersionV1,
			ID:              "trust-root-test",
			Revision:        2,
			Digest:          supplyChainTestDigestV1,
		},
		IdentityPolicyDigest:     supplyChainTestDigestV1,
		PredicatePolicyDigest:    supplyChainTestDigestV1,
		TransparencyPolicyDigest: supplyChainTestDigestV1,
		TimestampPolicyDigest:    supplyChainTestDigestV1,
		MaxPackageArtifactBytes:  1 << 30,
		MaxSigstoreBundleBytes:   8 << 20,
		MaxInTotoStatementBytes:  8 << 20,
		MaxTrustMaterialBytes:    8 << 20,
		CheckedUnixNano:          checked.UnixNano(),
		ExpiresUnixNano:          checked.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatalf("seal trust policy projection: %v", err)
	}
	return projection
}

func TestSupplyChainArtifactAndTrustRefsV1(t *testing.T) {
	artifact := SupplyChainArtifactContentRefV1{
		ContractVersion: SupplyChainArtifactTrustContractVersionV1,
		MediaType:       "application/vnd.oci.image.manifest.v1+json",
		Digest:          supplyChainTestDigestV1,
		Size:            1024,
	}
	if err := artifact.Validate(); err != nil {
		t.Fatalf("valid artifact ref rejected: %v", err)
	}
	artifact.Size = 0
	if err := artifact.Validate(); err == nil {
		t.Fatal("zero-sized artifact ref accepted")
	}
	artifact.Size = 1024
	artifact.MediaType = "HTTPS://example.invalid/latest"
	if err := artifact.Validate(); err == nil {
		t.Fatal("URL/latest was accepted as media type")
	}
}

func TestSupplyChainTrustPolicyCurrentV1(t *testing.T) {
	projection := validSupplyChainTrustPolicyProjectionV1(t)
	checked := time.Unix(0, projection.CheckedUnixNano)
	if err := projection.ValidateCurrent(projection.Ref, checked); err != nil {
		t.Fatalf("current projection rejected: %v", err)
	}

	drift := projection
	drift.MaxPackageArtifactBytes++
	if err := drift.Validate(); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("projection drift error = %v, want invalid digest", err)
	}

	wrong := projection.Ref
	wrong.Revision++
	if err := projection.ValidateCurrent(wrong, checked); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("wrong exact ref error = %v, want binding drift", err)
	}
	if err := projection.ValidateCurrent(projection.Ref, checked.Add(-time.Nanosecond)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback error = %v, want clock regression", err)
	}
	if err := projection.ValidateCurrent(projection.Ref, time.Unix(0, projection.ExpiresUnixNano)); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expiry error = %v, want binding expired", err)
	}
}

func TestSealSupplyChainTrustPolicyRejectsDigestInjectionV1(t *testing.T) {
	projection := validSupplyChainTrustPolicyProjectionV1(t)
	projection.Ref.Digest = supplyChainTestDigestV1
	projection.ProjectionDigest = supplyChainTestDigestV1
	if _, err := SealSupplyChainTrustPolicyCurrentProjectionV1(projection); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("digest injection error = %v, want invalid digest", err)
	}
}

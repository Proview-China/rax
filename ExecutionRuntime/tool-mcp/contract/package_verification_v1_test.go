package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestPackageTrustPolicyCertificateAndOfflineKeyModesV1(t *testing.T) {
	certificate, err := toolcontract.SealToolPackageTrustPolicyDocumentV1(toolcontract.ToolPackageTrustPolicyDocumentV1{
		IdentityMode: toolcontract.ToolPackageSigstoreCertificateV1,
		CertificateIdentities: []toolcontract.ToolPackageSigstoreCertificateIdentityV1{{
			Issuer: "https://issuer.example", SANValue: "builder@example",
		}},
		RequiredPredicateTypes:   []string{"https://slsa.dev/provenance/v1"},
		TimestampMode:            toolcontract.ToolPackageSigstoreObserverTimestampV1,
		TimestampThreshold:       1,
		TransparencyLogThreshold: 1,
		SCTThreshold:             1,
	})
	if err != nil || certificate.Validate() != nil {
		t.Fatalf("certificate policy=%+v err=%v", certificate, err)
	}
	key, err := toolcontract.SealToolPackageTrustPolicyDocumentV1(toolcontract.ToolPackageTrustPolicyDocumentV1{
		IdentityMode:           toolcontract.ToolPackageSigstoreKeyV1,
		RequiredPredicateTypes: []string{"https://slsa.dev/provenance/v1"},
		TimestampMode:          toolcontract.ToolPackageSigstoreNoTimestampForKeyV1,
	})
	if err != nil || key.Validate() != nil {
		t.Fatalf("offline key policy=%+v err=%v", key, err)
	}
	invalid := key
	invalid.TransparencyLogThreshold = 1
	if _, err = toolcontract.SealToolPackageTrustPolicyDocumentV1(invalid); err == nil {
		t.Fatal("offline key policy accepted unbound transparency requirements")
	}
}

func TestPackageVerificationCanonicalFactsRejectDigestAndExactRefDriftV1(t *testing.T) {
	fixture := testkit.PackageVerificationV1()
	if fixture.Observation.Validate() != nil || fixture.Fact.Validate() != nil || fixture.Current.ValidateCurrent(fixture.Current.Ref, testkit.FixedTime.Add(4*time.Second)) != nil {
		t.Fatal("sealed Package Verification fixture is invalid")
	}
	observation := fixture.Observation
	observation.Ref.Digest = testkit.Digest("changed-observation")
	if observation.Validate() == nil {
		t.Fatal("Observation digest drift was accepted")
	}
	fact := fixture.Fact
	fact.Package.Digest = testkit.Digest("changed-package")
	if fact.Validate() == nil {
		t.Fatal("Fact exact Package drift was accepted")
	}
	current := fixture.Current
	current.ProjectionDigest = testkit.Digest("changed-current")
	if current.Validate() == nil {
		t.Fatal("Current projection digest drift was accepted")
	}
	if err := fixture.Current.ValidateCurrent(fixture.Current.Ref, time.Unix(0, fixture.Current.CheckedUnixNano-1)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback error=%v", err)
	}
	if err := fixture.Current.ValidateCurrent(fixture.Current.Ref, time.Unix(0, fixture.Current.ExpiresUnixNano)); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expiry error=%v", err)
	}
}

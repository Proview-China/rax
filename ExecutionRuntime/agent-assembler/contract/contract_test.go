package contract_test

import (
	"testing"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestSealedReleaseAndSnapshotsRejectTamper(t *testing.T) {
	fixture := testkit.NewFixture()
	release := fixture.Releases[0]
	release.ComponentManifest.SemanticVersion = "1.1.0"
	if err := release.Validate(); !core.HasReason(err, core.ReasonBindingNotCertified) {
		t.Fatalf("release tamper accepted: %v", err)
	}
	catalog := fixture.Catalog
	catalog.CheckedUnixNano++
	if err := catalog.Validate(); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("catalog tamper accepted: %v", err)
	}
	facts := fixture.Facts
	facts.MaximumPriority++
	if err := facts.Validate(); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("facts tamper accepted: %v", err)
	}
}

func TestCatalogRejectsMultipleCurrentRevisionsForReleaseID(t *testing.T) {
	fixture := testkit.NewFixture()
	first := fixture.Releases[0]
	first.Revision = core.Revision(0x110000)
	first.ReleaseID = "release/structural-key"
	if err := testkit.PrepareProductionRelease("structural-key", &first); err != nil {
		t.Fatal(err)
	}
	first, err := testkit.SealProductionRelease(first)
	if err != nil {
		t.Fatal(err)
	}
	second := assemblercontract.CloneComponentReleaseV1(first)
	second.Revision++
	if err = testkit.PrepareProductionRelease("structural-key", &second); err != nil {
		t.Fatal(err)
	}
	second, err = testkit.SealProductionRelease(second)
	if err != nil {
		t.Fatal(err)
	}
	catalog := fixture.Catalog
	catalog.CatalogID = "catalog/structural-key"
	catalog.Releases = []assemblercontract.ComponentReleaseV1{first, second}
	if _, err = assemblercontract.SealComponentReleaseCatalogV1(catalog); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("multiple current release revisions accepted: %v", err)
	}
}

func TestCatalogRejectsReleaseCreatedAfterCheckedTime(t *testing.T) {
	fixture := testkit.NewFixture()
	release := fixture.Releases[0]
	release.CreatedUnixNano = fixture.Catalog.CheckedUnixNano + 1
	release.ExpiresUnixNano = fixture.Catalog.ExpiresUnixNano
	release, err := testkit.SealProductionRelease(release)
	if err != nil {
		t.Fatal(err)
	}
	catalog := fixture.Catalog
	catalog.CatalogID = "catalog/future-release"
	catalog.Releases[0] = release
	if _, err = assemblercontract.SealComponentReleaseCatalogV1(catalog); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("future-created release accepted: %v", err)
	}
}

func TestProductionReleaseRejectsMissingConstructionAndCapabilitySplice(t *testing.T) {
	fixture := testkit.NewFixture()
	tests := []struct {
		name   string
		mutate func(*assemblercontract.ComponentReleaseV1)
		reason core.ReasonCode
	}{
		{name: "remote-host-adapter-factory-missing", mutate: func(release *assemblercontract.ComponentReleaseV1) {
			release.ComponentManifest.Locality = "remote_provider"
			for index := range release.ModuleDescriptors {
				release.ModuleDescriptors[index].Locality = "remote_provider"
			}
			manifestDigest, _ := release.ComponentManifest.BindingDigestV2()
			for index := range release.ModuleDescriptors {
				release.ModuleDescriptors[index].ComponentManifestRef.Digest = manifestDigest
			}
			release.FactoryDescriptors = nil
		}, reason: core.ReasonComponentMissing},
		{name: "port-missing", mutate: func(release *assemblercontract.ComponentReleaseV1) {
			release.PortSpecs = nil
		}, reason: core.ReasonComponentMissing},
		{name: "capability-descriptor-splice", mutate: func(release *assemblercontract.ComponentReleaseV1) {
			release.CapabilityDescriptors[0].TTLSeconds++
		}, reason: core.ReasonBindingDrift},
		{name: "duplicate-factory-capability-alias", mutate: func(release *assemblercontract.ComponentReleaseV1) {
			duplicate := release.FactoryDescriptors[0]
			duplicate.FactoryID += "-alias"
			release.FactoryDescriptors = append(release.FactoryDescriptors, duplicate)
		}, reason: core.ReasonDuplicateCanonicalKey},
		{name: "module-artifact-splice", mutate: func(release *assemblercontract.ComponentReleaseV1) {
			release.ModuleDescriptors[0].ArtifactDigest = testkit.Digest("spliced-module-artifact")
		}, reason: core.ReasonBindingDrift},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			release := assemblercontract.CloneComponentReleaseV1(fixture.Releases[0])
			test.mutate(&release)
			if _, err := testkit.SealProductionRelease(release); !core.HasReason(err, test.reason) {
				t.Fatalf("invalid production closure accepted or wrong reason: %v", err)
			}
		})
	}
}

func TestProductionCertificationBindsExactReleaseContent(t *testing.T) {
	fixture := testkit.NewFixture()
	release := assemblercontract.CloneComponentReleaseV1(fixture.Releases[0])
	release.CertificationRef.Digest = testkit.Digest("different-certified-content")
	if err := release.Validate(); !core.HasReason(err, core.ReasonBindingNotCertified) {
		t.Fatalf("certification drift accepted: %v", err)
	}
	release = assemblercontract.CloneComponentReleaseV1(fixture.Releases[0])
	release.ComponentManifest.Contract.Version = "1.0.1"
	if err := release.Validate(); !core.HasReason(err, core.ReasonBindingNotCertified) {
		t.Fatalf("contract drift escaped exact certification: %v", err)
	}
}

func FuzzDerivePlanIDV1IsDeterministic(f *testing.F) {
	f.Add(uint64(1))
	f.Add(uint64(0x110001))
	f.Fuzz(func(t *testing.T, revision uint64) {
		if revision == 0 {
			revision = 1
		}
		fixture := testkit.NewFixture()
		definitionRef := fixture.Definition.RefV1()
		factsRef := fixture.Facts.RefV1()
		catalogRef := fixture.Catalog.RefV1()
		definitionRef.Revision = core.Revision(revision)
		factsRef.Revision = core.Revision(revision)
		catalogRef.Revision = core.Revision(revision)
		first, err := assemblercontract.DerivePlanIDV1(definitionRef, factsRef, catalogRef)
		if err != nil {
			t.Fatal(err)
		}
		second, err := assemblercontract.DerivePlanIDV1(definitionRef, factsRef, catalogRef)
		if err != nil {
			t.Fatal(err)
		}
		if first != second {
			t.Fatalf("derived plan id changed: %q != %q", first, second)
		}
	})
}

func FuzzProductionCertificationBindsPayloadMutation(f *testing.F) {
	f.Add(uint64(1))
	f.Add(uint64(0x110001))
	f.Fuzz(func(t *testing.T, delta uint64) {
		fixture := testkit.NewFixture()
		release := assemblercontract.CloneComponentReleaseV1(fixture.Releases[0])
		release.CreatedUnixNano += int64(delta%1_000_000) + 1
		digest, err := assemblercontract.ComponentReleaseCertificationDigestV1(release)
		if err != nil {
			t.Fatal(err)
		}
		if digest == release.CertificationRef.Digest {
			t.Fatal("certification digest did not change with release payload")
		}
		if err = release.Validate(); !core.HasReason(err, core.ReasonBindingNotCertified) {
			t.Fatalf("mutated certified payload accepted: %v", err)
		}
	})
}

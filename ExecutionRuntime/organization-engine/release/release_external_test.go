package release_test

import (
	"context"
	"testing"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	assemblerrepo "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/release"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type absentLocal struct{}

func (absentLocal) InspectOrganizationLocalReadinessV1(context.Context, string, core.Revision) (release.LocalReadinessProjectionV1, error) {
	return release.LocalReadinessProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "SQLite readiness absent")
}

type absentProduction struct{}

func (absentProduction) InspectOrganizationProductionReadinessV1(context.Context, string, core.Revision) (release.ProductionReadinessProjectionV1, error) {
	return release.ProductionReadinessProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "production readiness absent")
}

func TestAgentAssemblerConsumesOrganizationReferenceCandidate(t *testing.T) {
	now := time.Date(2026, 7, 18, 3, 0, 0, 0, time.UTC)
	catalog := assemblerrepo.NewReleaseMemory()
	publisher, err := release.NewPublisherV1(absentLocal{}, absentProduction{}, catalog, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	ref := func(id string) assemblycontract.ObjectRefV1 {
		return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
	}
	result, err := publisher.Publish(context.Background(), release.PublicationRequestV1{
		ReleaseID: "praxis.organization/public-release", Revision: 1,
		SourceRef: ref("organization-source"), PublisherRef: ref("organization-publisher"), TrustRef: ref("organization-trust"),
		CertificationID: "organization-certification", ArtifactDigest: core.DigestBytes([]byte("organization-artifact")),
		CreatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Release.SupportMode != assemblercontract.SupportReferenceOnlyV1 || result.Release.ComponentManifest.ComponentID != release.ComponentIDV1 {
		t.Fatal("Assembler received a promoted or aliased Organization release")
	}
	exact, err := catalog.InspectExactComponentReleaseV1(context.Background(), result.Release.RefV1())
	if err != nil || exact.ReleaseDigest != result.Release.ReleaseDigest {
		t.Fatalf("Assembler exact Inspect failed: %v", err)
	}
}

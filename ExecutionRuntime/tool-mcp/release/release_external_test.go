package release_test

import (
	"context"
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	assemblerrepo "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/release"
	"testing"
	"time"
)

type absentLocal struct{}

func (absentLocal) InspectToolMCPLocalReadinessV1(context.Context, string, core.Revision) (release.LocalReadinessProjectionV1, error) {
	return release.LocalReadinessProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "absent")
}

type absentProduction struct{}

func (absentProduction) InspectToolMCPProductionReadinessV1(context.Context, string, core.Revision) (release.ProductionReadinessProjectionV1, error) {
	return release.ProductionReadinessProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "absent")
}
func TestPublicPublisherProducesAssemblerConsumableReferenceCandidate(t *testing.T) {
	now := time.Date(2026, 7, 18, 3, 0, 0, 0, time.UTC)
	catalog := assemblerrepo.NewReleaseMemory()
	publisher, e := release.NewPublisherV1(absentLocal{}, absentProduction{}, catalog, func() time.Time { return now })
	if e != nil {
		t.Fatal(e)
	}
	ref := func(id string) assemblycontract.ObjectRefV1 {
		return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
	}
	result, e := publisher.Publish(context.Background(), release.PublicationRequestV1{ReleaseID: "praxis.tool-mcp/public-release", Revision: 1, SourceRef: ref("source"), PublisherRef: ref("publisher"), TrustRef: ref("trust"), CertificationID: "certification", ArtifactDigest: core.DigestBytes([]byte("artifact")), CreatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if e != nil {
		t.Fatal(e)
	}
	if result.Release.SupportMode != assemblercontract.SupportReferenceOnlyV1 {
		t.Fatal("missing owner-local proof was upgraded")
	}
	inspected, e := catalog.InspectExactComponentReleaseV1(context.Background(), result.Release.RefV1())
	if e != nil || inspected.ReleaseDigest != result.Release.ReleaseDigest {
		t.Fatalf("Assembler could not consume exact release: %v", e)
	}
}

package release_test

import (
	"context"
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	assemblerrepo "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/application/release"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"testing"
	"time"
)

type absentLocal struct{}

func (absentLocal) InspectApplicationLocalReadinessV1(context.Context, string, core.Revision) (release.LocalReadinessProjectionV1, error) {
	return release.LocalReadinessProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "absent")
}

type absentProduction struct{}

func (absentProduction) InspectApplicationProductionReadinessV1(context.Context, string, core.Revision) (release.ProductionReadinessProjectionV1, error) {
	return release.ProductionReadinessProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "absent")
}
func TestAssemblerConsumesPublicApplicationReference(t *testing.T) {
	now := time.Date(2026, 7, 18, 7, 0, 0, 0, time.UTC)
	catalog := assemblerrepo.NewReleaseMemory()
	p, e := release.NewPublisherV1(absentLocal{}, absentProduction{}, catalog, func() time.Time { return now })
	if e != nil {
		t.Fatal(e)
	}
	ref := func(id string) assemblycontract.ObjectRefV1 {
		return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
	}
	got, e := p.Publish(context.Background(), release.PublicationRequestV1{ReleaseID: "praxis.application/public", Revision: 1, SourceRef: ref("source"), PublisherRef: ref("publisher"), TrustRef: ref("trust"), CertificationID: "certification", ArtifactDigest: core.DigestBytes([]byte("artifact")), CreatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if e != nil {
		t.Fatal(e)
	}
	if got.Release.SupportMode != assemblercontract.SupportReferenceOnlyV1 {
		t.Fatal("partial shared engine was upgraded")
	}
	exact, e := catalog.InspectExactComponentReleaseV1(context.Background(), got.Release.RefV1())
	if e != nil || exact.ReleaseDigest != got.Release.ReleaseDigest {
		t.Fatalf("exact Inspect failed: %v", e)
	}
}

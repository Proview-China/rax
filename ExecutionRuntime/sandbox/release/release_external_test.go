package release_test

import (
	"context"
	"testing"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	assemblerrepo "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/release"
)

type absentReadinessV1 struct{}

func (absentReadinessV1) InspectSandboxProductionReadinessV1(context.Context, string, core.Revision) (release.SandboxProductionReadinessProjectionV1, error) {
	return release.SandboxProductionReadinessProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "deployment readiness is not published")
}

func TestPublicPublisherProducesAssemblerConsumableCandidate(t *testing.T) {
	now := time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC)
	catalog := assemblerrepo.NewReleaseMemory()
	publisher, err := release.NewPublisherV1(absentReadinessV1{}, catalog, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	ref := func(id string) assemblycontract.ObjectRefV1 {
		return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
	}
	result, err := publisher.Publish(context.Background(), release.PublicationRequestV1{
		ReleaseID: "praxis.sandbox/public-release", Revision: 1,
		SourceRef: ref("sandbox-source"), PublisherRef: ref("sandbox-publisher"), TrustRef: ref("sandbox-trust"),
		CertificationID: "sandbox-certification", ArtifactDigest: core.DigestBytes([]byte("sandbox-artifact")),
		CreatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Release.SupportMode != assemblercontract.SupportStandaloneV1 {
		t.Fatal("a missing deployment proof was exposed as production")
	}
	if len(result.Release.SlotContributions) != 1 || result.Release.SlotContributions[0].SlotRef != "sandbox.execution" || len(result.Release.ProviderBindingCandidates) != 0 {
		t.Fatalf("public release did not expose the exact Sandbox slot Owner boundary: %+v", result.Release.SlotContributions)
	}
	inspected, err := catalog.InspectExactComponentReleaseV1(context.Background(), result.Release.RefV1())
	if err != nil || inspected.ReleaseDigest != result.Release.ReleaseDigest {
		t.Fatalf("Assembler catalog could not inspect the exact Sandbox candidate: %v", err)
	}
}

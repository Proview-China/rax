package conformance_test

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/conformance"
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestComponentReleaseConformanceNeverGrantsAuthority(t *testing.T) {
	fixture := testkit.NewFixture()
	store := repository.NewReleaseMemory()
	release := fixture.Releases[0]
	if _, err := store.EnsureExactComponentReleaseV1(context.Background(), release); err != nil {
		t.Fatal(err)
	}
	report, err := conformance.CheckComponentReleaseV1(context.Background(), store, release.RefV1(), testkit.Now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if !report.CertificationCandidate || report.GrantsAuthority || report.GrantsDispatch {
		t.Fatalf("unsafe report: %#v", report)
	}
}

type driftingReleaseReader struct {
	first  assemblercontract.ComponentReleaseV1
	second assemblercontract.ComponentReleaseV1
	calls  int
}

func (r *driftingReleaseReader) InspectExactComponentReleaseV1(context.Context, assemblercontract.ComponentReleaseRefV1) (assemblercontract.ComponentReleaseV1, error) {
	r.calls++
	if r.calls == 1 {
		return r.first, nil
	}
	return r.second, nil
}

func TestComponentReleaseConformanceRejectsS1S2BodyDrift(t *testing.T) {
	fixture := testkit.NewFixture()
	first := fixture.Releases[0]
	second := first
	second.CreatedUnixNano++
	reader := &driftingReleaseReader{first: first, second: second}
	if _, err := conformance.CheckComponentReleaseV1(context.Background(), reader, first.RefV1(), testkit.Now.UnixNano()); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("release body drift accepted: %v", err)
	}
}

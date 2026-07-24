package fault_test

import (
	"context"
	"testing"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/internal/testkit"
	assemblerports "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/resolver"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestResolveRejectsExpiredFactsBeforePlanWrite(t *testing.T) {
	fixture := testkit.NewFixture()
	service, err := resolver.New(fixture.Snapshots, fixture.Snapshots, fixture.Plans, func() time.Time { return testkit.Now.Add(2 * time.Hour) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Resolve(context.Background(), fixture.Request); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expired facts accepted: %v", err)
	}
	if _, err = fixture.Plans.InspectCurrentResolvedAgentPlanV1(context.Background(), fixture.Definition.DefinitionID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("plan current mutated: %v", err)
	}
}

func TestResolveRejectsNonProductionRelease(t *testing.T) {
	fixture := testkit.NewFixture()
	catalog := fixture.Catalog
	release := catalog.Releases[0]
	release.SupportMode = assemblercontract.SupportStandaloneV1
	sealedRelease, err := assemblercontract.SealComponentReleaseV1(release)
	if err != nil {
		t.Fatal(err)
	}
	catalog.Releases[0] = sealedRelease
	catalog.CatalogID = "catalog/non-production"
	catalog, err = assemblercontract.SealComponentReleaseCatalogV1(catalog)
	if err != nil {
		t.Fatal(err)
	}
	snapshots := repository.NewSnapshots()
	if err = snapshots.PutFacts(fixture.Facts); err != nil {
		t.Fatal(err)
	}
	if err = snapshots.PutCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	service, err := resolver.New(snapshots, snapshots, repository.NewMemory(), func() time.Time { return testkit.Now })
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.Request
	request.CatalogRef = catalog.RefV1()
	if _, err = service.Resolve(context.Background(), request); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("non-production release accepted: %v", err)
	}
}

type lostReplyPlans struct {
	assemblerports.ResolvedAgentPlanRepositoryV1
}

func (l lostReplyPlans) EnsureExactResolvedAgentPlanV1(ctx context.Context, value assemblercontract.ResolvedAgentPlanV1) (assemblercontract.ResolvedAgentPlanV1, error) {
	_, err := l.ResolvedAgentPlanRepositoryV1.EnsureExactResolvedAgentPlanV1(ctx, value)
	if err != nil {
		return assemblercontract.ResolvedAgentPlanV1{}, err
	}
	return assemblercontract.ResolvedAgentPlanV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost create reply")
}

type lostCurrentReplyPlans struct {
	assemblerports.ResolvedAgentPlanRepositoryV1
}

func (l lostCurrentReplyPlans) CompareAndSwapCurrentResolvedAgentPlanV1(ctx context.Context, expected *assemblercontract.CurrentResolvedPlanRefV1, next assemblercontract.CurrentResolvedPlanV1) (assemblercontract.CurrentResolvedPlanV1, error) {
	_, err := l.ResolvedAgentPlanRepositoryV1.CompareAndSwapCurrentResolvedAgentPlanV1(ctx, expected, next)
	if err != nil {
		return assemblercontract.CurrentResolvedPlanV1{}, err
	}
	return assemblercontract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost current projection reply")
}

func TestResolveRecoversLostCreateReplyByExactInspect(t *testing.T) {
	fixture := testkit.NewFixture()
	store := repository.NewMemory()
	service, err := resolver.New(fixture.Snapshots, fixture.Snapshots, lostReplyPlans{store}, func() time.Time { return testkit.Now })
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Resolve(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Plan.Digest == "" {
		t.Fatal("lost reply recovery returned empty plan")
	}
}

func TestResolveRecoversLostCurrentReplyByExactProjectionInspect(t *testing.T) {
	fixture := testkit.NewFixture()
	store := repository.NewMemory()
	service, err := resolver.New(fixture.Snapshots, fixture.Snapshots, lostCurrentReplyPlans{store}, func() time.Time { return testkit.Now })
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Resolve(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.InspectCurrentResolvedAgentPlanV1(context.Background(), fixture.Definition.DefinitionID)
	if err != nil {
		t.Fatal(err)
	}
	if current.PlanRef != result.Plan.RefV1() || current.Revision != 1 {
		t.Fatalf("wrong recovered current projection: %+v", current)
	}
}

type driftingFacts struct {
	first, second assemblercontract.ResolutionFactsSnapshotV1
	calls         int
}

func (d *driftingFacts) InspectExactResolutionFactsV1(context.Context, assemblercontract.ResolutionFactsRefV1) (assemblercontract.ResolutionFactsSnapshotV1, error) {
	d.calls++
	if d.calls == 1 {
		return d.first, nil
	}
	return d.second, nil
}

func TestResolveRejectsS1S2Drift(t *testing.T) {
	fixture := testkit.NewFixture()
	changed := fixture.Facts
	changed.MaximumPriority++
	changed.FactsID = "facts/drifted"
	changed, err := assemblercontract.SealResolutionFactsV1(changed)
	if err != nil {
		t.Fatal(err)
	}
	reader := &driftingFacts{first: fixture.Facts, second: changed}
	service, err := resolver.New(reader, fixture.Snapshots, repository.NewMemory(), func() time.Time { return testkit.Now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Resolve(context.Background(), fixture.Request); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("drift accepted: %v", err)
	}
}

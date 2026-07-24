package kernel_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/tests/testsupport"
)

func TestOperationScopeEvidenceActionRouterV3RejectsMissingDuplicateUnknownAndDrift(t *testing.T) {
	fixture := testsupport.OperationScopeEvidenceActionFixture()
	bindings, sources := actionRouterBindingsV3(t, fixture.Now, fixture.Operation.ExecutionScopeDigest)
	if _, err := kernel.NewOperationScopeEvidenceActionRouterV3(bindings[:4], func() time.Time { return fixture.Now }); err == nil {
		t.Fatal("missing route was accepted")
	}
	duplicate := append([]kernel.OperationScopeEvidenceActionRouteBindingV3{}, bindings...)
	duplicate[4] = duplicate[0]
	if _, err := kernel.NewOperationScopeEvidenceActionRouterV3(duplicate, func() time.Time { return fixture.Now }); err == nil {
		t.Fatal("duplicate route was accepted")
	}
	router, err := kernel.NewOperationScopeEvidenceActionRouterV3(bindings, func() time.Time { return fixture.Now })
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		ref, _ := ports.ProjectOperationScopeEvidenceActionApplicabilityRefV3(source)
		if _, err := router.InspectOperationScopeEvidenceActionApplicabilityCurrentV3(context.Background(), source.Route.Dimension, ref, fixture.Operation.ExecutionScopeDigest); err != nil {
			t.Fatal(err)
		}
	}
	unknown := sources[0]
	unknown.Route.Kind = "custom/run"
	unknownRef := ports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: unknown.Route.Kind, ID: unknown.ID, Revision: unknown.Revision, Digest: unknown.Digest}
	if _, err := router.InspectOperationScopeEvidenceActionApplicabilityCurrentV3(context.Background(), unknown.Route.Dimension, unknownRef, fixture.Operation.ExecutionScopeDigest); err == nil {
		t.Fatal("unknown source Kind reached an Owner Reader")
	}
	drifting := bindings[0].Reader.(*actionApplicabilityReaderV3)
	drifting.projection.Fact.Revision++
	if _, err := router.InspectOperationScopeEvidenceActionApplicabilityCurrentV3(context.Background(), sources[0].Route.Dimension, ports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: sources[0].Route.Kind, ID: sources[0].ID, Revision: sources[0].Revision, Digest: sources[0].Digest}, fixture.Operation.ExecutionScopeDigest); err == nil {
		t.Fatal("Owner projection drift was accepted")
	}
}

func TestControlledOperationProviderSeamV1EnforcesCurrentBoundaryAndCallsAtMostOnce(t *testing.T) {
	fixture := testsupport.OperationScopeEvidenceActionFixture()
	provider := &actionProviderInvokerV1{}
	boundaryReader := &actionBoundaryReaderV1{projection: fixture.Boundary}
	seam, err := kernel.NewControlledOperationProviderSeamV1(
		&actionEnforcementReaderV1{ref: fixture.Enforcement},
		&actionHandoffReaderV1{fact: fixture.Handoff},
		boundaryReader,
		provider,
		func() time.Time { return fixture.Now.Add(time.Second) },
	)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 64
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			_ = seam.CallControlledOperationProviderV1(context.Background(), fixture.Call)
		}()
	}
	wg.Wait()
	if provider.calls.Load() != 1 {
		t.Fatalf("Provider calls=%d, want one logical fixture call", provider.calls.Load())
	}
	if err := seam.CallControlledOperationProviderV1(context.Background(), fixture.Call); err == nil || provider.calls.Load() != 1 {
		t.Fatal("lost/replayed Provider reply caused another call")
	}
	changed := fixture.Call
	changed.OperationScopeDigest = core.DigestBytes([]byte("changed-operation-scope"))
	changedProjection := fixture.Boundary
	changedProjection.OperationScopeDigest = changed.OperationScopeDigest
	changedProjection, err = ports.SealOperationProviderBoundaryCurrentProjectionV1(changedProjection)
	if err != nil {
		t.Fatal(err)
	}
	boundaryReader.projection = changedProjection
	if err := seam.CallControlledOperationProviderV1(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) || provider.calls.Load() != 1 {
		t.Fatalf("same boundary id accepted changed call content: %v calls=%d", err, provider.calls.Load())
	}
}

func TestControlledOperationProviderSeamV1DriftUnavailableExpiryAndLostReplyAreFailClosed(t *testing.T) {
	for name, build := range map[string]func(t *testing.T, f testsupport.OperationScopeEvidenceActionFixtureV3, provider *actionProviderInvokerV1) *kernel.ControlledOperationProviderSeamV1{
		"enforcement_unavailable": func(t *testing.T, f testsupport.OperationScopeEvidenceActionFixtureV3, p *actionProviderInvokerV1) *kernel.ControlledOperationProviderSeamV1 {
			return mustActionSeamV1(t, &actionEnforcementReaderV1{err: unavailableActionV1()}, &actionHandoffReaderV1{fact: f.Handoff}, &actionBoundaryReaderV1{projection: f.Boundary}, p, f.Now)
		},
		"handoff_unavailable": func(t *testing.T, f testsupport.OperationScopeEvidenceActionFixtureV3, p *actionProviderInvokerV1) *kernel.ControlledOperationProviderSeamV1 {
			return mustActionSeamV1(t, &actionEnforcementReaderV1{ref: f.Enforcement}, &actionHandoffReaderV1{err: unavailableActionV1()}, &actionBoundaryReaderV1{projection: f.Boundary}, p, f.Now)
		},
		"boundary_unavailable": func(t *testing.T, f testsupport.OperationScopeEvidenceActionFixtureV3, p *actionProviderInvokerV1) *kernel.ControlledOperationProviderSeamV1 {
			return mustActionSeamV1(t, &actionEnforcementReaderV1{ref: f.Enforcement}, &actionHandoffReaderV1{fact: f.Handoff}, &actionBoundaryReaderV1{err: unavailableActionV1()}, p, f.Now)
		},
		"boundary_drift": func(t *testing.T, f testsupport.OperationScopeEvidenceActionFixtureV3, p *actionProviderInvokerV1) *kernel.ControlledOperationProviderSeamV1 {
			f.Boundary.OperationScopeDigest = core.DigestBytes([]byte("drift"))
			f.Boundary, _ = ports.SealOperationProviderBoundaryCurrentProjectionV1(f.Boundary)
			return mustActionSeamV1(t, &actionEnforcementReaderV1{ref: f.Enforcement}, &actionHandoffReaderV1{fact: f.Handoff}, &actionBoundaryReaderV1{projection: f.Boundary}, p, f.Now)
		},
		"expired": func(t *testing.T, f testsupport.OperationScopeEvidenceActionFixtureV3, p *actionProviderInvokerV1) *kernel.ControlledOperationProviderSeamV1 {
			return mustActionSeamV1(t, &actionEnforcementReaderV1{ref: f.Enforcement}, &actionHandoffReaderV1{fact: f.Handoff}, &actionBoundaryReaderV1{projection: f.Boundary}, p, time.Unix(0, f.Boundary.ExpiresUnixNano))
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := testsupport.OperationScopeEvidenceActionFixture()
			provider := &actionProviderInvokerV1{}
			seam := build(t, fixture, provider)
			if err := seam.CallControlledOperationProviderV1(context.Background(), fixture.Call); err == nil {
				t.Fatal("invalid current boundary reached Provider")
			}
			if provider.calls.Load() != 0 {
				t.Fatalf("Provider calls=%d, want zero", provider.calls.Load())
			}
		})
	}
	fixture := testsupport.OperationScopeEvidenceActionFixture()
	provider := &actionProviderInvokerV1{err: unavailableActionV1()}
	seam := mustActionSeamV1(t, &actionEnforcementReaderV1{ref: fixture.Enforcement}, &actionHandoffReaderV1{fact: fixture.Handoff}, &actionBoundaryReaderV1{projection: fixture.Boundary}, provider, fixture.Now)
	if err := seam.CallControlledOperationProviderV1(context.Background(), fixture.Call); err == nil {
		t.Fatal("lost Provider reply was not surfaced")
	}
	_ = seam.CallControlledOperationProviderV1(context.Background(), fixture.Call)
	if provider.calls.Load() != 1 {
		t.Fatal("lost Provider reply was blindly reissued")
	}
}

func TestOperationScopeEvidenceActionConformanceV3PublicOnly(t *testing.T) {
	fixture := testsupport.OperationScopeEvidenceActionFixture()
	bindings, sources := actionRouterBindingsV3(t, fixture.Now, fixture.Operation.ExecutionScopeDigest)
	router, err := kernel.NewOperationScopeEvidenceActionRouterV3(bindings, func() time.Time { return fixture.Now })
	if err != nil {
		t.Fatal(err)
	}
	provider := &actionProviderInvokerV1{}
	seam := mustActionSeamV1(t, &actionEnforcementReaderV1{ref: fixture.Enforcement}, &actionHandoffReaderV1{fact: fixture.Handoff}, &actionBoundaryReaderV1{projection: fixture.Boundary}, provider, fixture.Now)
	report, err := conformance.RunOperationScopeEvidenceActionConformanceV3(context.Background(), conformance.OperationScopeEvidenceActionConformanceCaseV3{
		Router: router, Provider: seam, Sources: sources, ScopeDigest: fixture.Operation.ExecutionScopeDigest, Call: fixture.Call, ProviderCalls: func() int { return int(provider.calls.Load()) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.ClosedMatrixExact || !report.AllFiveOwnerRoutesExact || !report.BoundaryCurrentExact || !report.ReplayDidNotRecall || report.ProductionClaimEligible {
		t.Fatalf("unexpected conformance report: %#v", report)
	}
}

type actionApplicabilityReaderV3 struct {
	projection ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3
	err        error
}

func (r *actionApplicabilityReaderV3) InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Context, ports.OperationScopeEvidenceApplicabilityFactRefV3) (ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3, error) {
	return r.projection, r.err
}

type actionEnforcementReaderV1 struct {
	ref ports.OperationDispatchEnforcementPhaseRefV4
	err error
}

func (r *actionEnforcementReaderV1) InspectCurrentOperationProviderExecuteEnforcementV1(context.Context, ports.OperationSubjectV3, ports.OperationDispatchEnforcementPhaseRefV4) (ports.OperationDispatchEnforcementPhaseRefV4, error) {
	return r.ref, r.err
}

type actionHandoffReaderV1 struct {
	fact ports.OperationScopeEvidenceProviderHandoffFactV3
	err  error
}

func (r *actionHandoffReaderV1) InspectCurrentOperationProviderEvidenceHandoffV1(context.Context, ports.OperationScopeEvidenceProviderHandoffRefV3) (ports.OperationScopeEvidenceProviderHandoffFactV3, error) {
	return r.fact, r.err
}

type actionBoundaryReaderV1 struct {
	projection ports.OperationProviderBoundaryCurrentProjectionV1
	err        error
}

func (r *actionBoundaryReaderV1) InspectCurrentOperationProviderBoundaryV1(context.Context, ports.OperationProviderBoundaryRefV1) (ports.OperationProviderBoundaryCurrentProjectionV1, error) {
	return r.projection, r.err
}

type actionProviderInvokerV1 struct {
	calls atomic.Int64
	err   error
}

func (p *actionProviderInvokerV1) InvokeOperationProviderTestV1(context.Context, ports.ControlledOperationProviderCallRequestV1) error {
	p.calls.Add(1)
	return p.err
}

func actionRouterBindingsV3(t *testing.T, now time.Time, scopeDigest core.Digest) ([]kernel.OperationScopeEvidenceActionRouteBindingV3, []ports.OperationScopeEvidenceActionApplicabilitySourceV3) {
	t.Helper()
	bindings := make([]kernel.OperationScopeEvidenceActionRouteBindingV3, 0, 5)
	sources := make([]ports.OperationScopeEvidenceActionApplicabilitySourceV3, 0, 5)
	for _, route := range ports.OperationScopeEvidenceActionRoutesV3() {
		source := ports.OperationScopeEvidenceActionApplicabilitySourceV3{Route: route, ID: "source-" + string(route.Dimension), Revision: 1, Digest: core.DigestBytes([]byte(route.Kind))}
		ref, err := ports.ProjectOperationScopeEvidenceActionApplicabilityRefV3(source)
		if err != nil {
			t.Fatal(err)
		}
		projection := ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{Fact: ref, ExecutionScopeDigest: scopeDigest, Current: true, ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}
		copy := projection
		copy.Digest = ""
		projection.Digest, err = core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", ports.OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceApplicabilityCurrentProjectionV3", copy)
		if err != nil {
			t.Fatal(err)
		}
		bindings = append(bindings, kernel.OperationScopeEvidenceActionRouteBindingV3{Route: route, Reader: &actionApplicabilityReaderV3{projection: projection}})
		sources = append(sources, source)
	}
	return bindings, sources
}

func mustActionSeamV1(t *testing.T, enforcement ports.OperationProviderExecuteEnforcementCurrentReaderV1, handoff ports.OperationProviderEvidenceHandoffCurrentReaderV1, boundary ports.OperationProviderBoundaryCurrentReaderV1, provider ports.OperationProviderTestInvokerV1, now time.Time) *kernel.ControlledOperationProviderSeamV1 {
	t.Helper()
	seam, err := kernel.NewControlledOperationProviderSeamV1(enforcement, handoff, boundary, provider, func() time.Time { return now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	return seam
}

func unavailableActionV1() error {
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "fixture unavailable")
}

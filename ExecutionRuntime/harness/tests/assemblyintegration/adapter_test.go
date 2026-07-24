package assemblyintegration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblyadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestAssociationAdapterBlackboxMapsThroughRuntimeOwnerAndReportsOnlyConformance(t *testing.T) {
	fixture := newFixtureV1(t)
	now := fixture.now
	port := newAssociationPortV1(&now)
	adapter, err := assemblyadapter.New(port, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Associate(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fact.CandidateDigest != result.Candidate.Digest || result.Fact.ID != fixture.request.AssociationID {
		t.Fatal("Runtime-owned Fact does not bind the exact mapped candidate")
	}
	if result.Conformance.Association == nil || result.Conformance.Association.Digest != result.Fact.Digest {
		t.Fatal("conformance report does not reference the exact Runtime association Fact")
	}
	if result.Conformance.Binding.ComponentID != "" || result.Conformance.CapabilityDigest != "" || len(result.Conformance.SchemaDigests) != 0 {
		t.Fatal("association conformance synthesized provider Binding or capability/schema authority")
	}
	if result.Conformance.BindingSetProjectionDigest != fixture.request.Binding.ProjectionDigest || result.Conformance.ActivationProjectionDigest != fixture.request.Activation.ProjectionDigest || !result.Conformance.Current {
		t.Fatal("conformance did not preserve exact BindingSet/Activation currentness")
	}
	if err := result.Conformance.Validate(now.UnixNano()); err != nil {
		t.Fatal(err)
	}
	report, err := conformance.CheckGenerationBindingAssociationV1(context.Background(), conformance.GenerationBindingAssociationCaseV1{Gateway: port, Candidate: result.Candidate})
	if err != nil {
		t.Fatal(err)
	}
	if !report.RuntimeFactOwnerObserved || !report.CurrentInspectObserved || report.CandidateIsBindingFact || report.ProductionClaimEligible {
		t.Fatalf("unexpected Runtime public conformance report: %+v", report)
	}
}

func TestBuildCandidateFailsClosedOnEveryAssemblySummary(t *testing.T) {
	fixture := newFixtureV1(t)
	cases := map[string]func(*assemblyadapter.AssociationRequestV1){
		"generation": func(request *assemblyadapter.AssociationRequestV1) { request.Generation.Digest = "" },
		"input": func(request *assemblyadapter.AssociationRequestV1) {
			request.Graph.InputDigest = assemblytestkit.Digest("wrong-input")
			request.Graph.Digest, _ = assemblycontract.GraphDigestV1(request.Graph)
		},
		"manifest": func(request *assemblyadapter.AssociationRequestV1) {
			request.Manifest.Digest = assemblytestkit.Digest("wrong-manifest")
		},
		"graph": func(request *assemblyadapter.AssociationRequestV1) {
			request.Graph.Digest = assemblytestkit.Digest("wrong-graph")
		},
		"catalog": func(request *assemblyadapter.AssociationRequestV1) {
			request.Handoff.CatalogDigest = assemblytestkit.Digest("wrong-catalog")
			request.Handoff.Digest, _ = assemblycontract.HandoffDigestV1(request.Handoff)
		},
		"component-manifest": func(request *assemblyadapter.AssociationRequestV1) {
			request.Manifest.ComponentManifests[0].ArtifactDigest = assemblytestkit.Digest("different-artifact")
			resealChainV1(t, request)
		},
		"governance-extension": func(request *assemblyadapter.AssociationRequestV1) {
			request.Handoff.RequiredExtension = "praxis.harness/different-extension"
			request.Handoff.Digest, _ = assemblycontract.HandoffDigestV1(request.Handoff)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := fixture.request
			mutate(&request)
			if _, err := assemblyadapter.BuildCandidateV1(request, fixture.now); err == nil {
				t.Fatal("drifted summary was accepted")
			}
		})
	}
}

func TestBuildCandidateRejectsMissingGovernanceExtension(t *testing.T) {
	fixture := newFixtureV1(t)
	fixture.request.Manifest.ComponentManifests[0].Extensions = []runtimeports.GovernanceExtensionV2{}
	manifestDigest, err := fixture.request.Manifest.ComponentManifests[0].BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	fixture.request.Manifest.Modules[0].ComponentManifestRef.Digest = manifestDigest
	resealChainV1(t, &fixture.request)
	if _, err = assemblyadapter.BuildCandidateV1(fixture.request, fixture.now); !core.HasReason(err, core.ReasonUnknownRequiredExtension) {
		t.Fatalf("expected required extension failure, got %v", err)
	}
}

func TestBuildCandidateRejectsOldGenerationWrongScopeBindingAndActivation(t *testing.T) {
	fixture := newFixtureV1(t)
	cases := map[string]func(*assemblyadapter.AssociationRequestV1){
		"old-generation": func(request *assemblyadapter.AssociationRequestV1) { request.GenerationCurrentness.Current = false },
		"wrong-binding": func(request *assemblyadapter.AssociationRequestV1) {
			request.ExpectedBindingSet.ID = "binding-set/other"
		},
		"wrong-scope": func(request *assemblyadapter.AssociationRequestV1) {
			request.ExpectedActivation = activationSubjectV1(t, assemblytestkit.Digest("other-plan"), "activation-attempt-1")
		},
		"wrong-activation": func(request *assemblyadapter.AssociationRequestV1) {
			request.ExpectedActivation = activationSubjectV1(t, request.Manifest.Plan.ResolvedAgentPlan.Digest, "activation-attempt-other")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := fixture.request
			mutate(&request)
			if _, err := assemblyadapter.BuildCandidateV1(request, fixture.now); err == nil {
				t.Fatal("stale or mismatched current projection was accepted")
			}
		})
	}
}

func TestBuildCandidateRejectsEveryCurrentnessExpiryBoundary(t *testing.T) {
	fixture := newFixtureV1(t)
	cases := map[string]func(*assemblyadapter.AssociationRequestV1){
		"generation": func(request *assemblyadapter.AssociationRequestV1) {
			request.GenerationCurrentness.ExpiresUnixNano = fixture.now.UnixNano()
		},
		"binding": func(request *assemblyadapter.AssociationRequestV1) {
			request.Binding.ExpiresUnixNano = fixture.now.UnixNano()
			request.Binding, _ = runtimeports.SealGenerationBindingSetCurrentProjectionV1(request.Binding)
		},
		"activation": func(request *assemblyadapter.AssociationRequestV1) {
			request.Activation.ExpiresUnixNano = fixture.now.UnixNano()
			request.Activation, _ = runtimeports.SealGenerationActivationCurrentProjectionV1(request.Activation)
		},
		"request": func(request *assemblyadapter.AssociationRequestV1) {
			request.RequestedExpiresUnixNano = fixture.now.UnixNano()
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := fixture.request
			mutate(&request)
			if _, err := assemblyadapter.BuildCandidateV1(request, fixture.now); err == nil {
				t.Fatal("currentness at its exclusive expiry boundary was accepted")
			}
		})
	}
}

func TestBuildCandidateRejectsResealedManifestGraphStructuralDrift(t *testing.T) {
	fixture := newFixtureV1(t)
	cases := map[string]func(*assemblyadapter.AssociationRequestV1){
		"dependency-order": func(request *assemblyadapter.AssociationRequestV1) {
			request.Graph.DependencyOrder[0], request.Graph.DependencyOrder[1] = request.Graph.DependencyOrder[1], request.Graph.DependencyOrder[0]
		},
		"port-refs": func(request *assemblyadapter.AssociationRequestV1) {
			request.Graph.PortSpecRefs = append(request.Graph.PortSpecRefs, "praxis.fixture/forged-port")
		},
		"factory-refs": func(request *assemblyadapter.AssociationRequestV1) {
			request.Graph.FactoryRefs = []string{}
		},
		"slot-contributions": func(request *assemblyadapter.AssociationRequestV1) {
			for index := range request.Graph.Slots {
				if len(request.Graph.Slots[index].Contributions) != 0 {
					request.Graph.Slots[index].Contributions = []string{}
					return
				}
			}
		},
		"phase-identity": func(request *assemblyadapter.AssociationRequestV1) {
			request.Graph.Phases[0].PhaseID = "forged.phase"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := fixture.request
			mutate(&request)
			resealChainV1(t, &request)
			if _, err := assemblyadapter.BuildCandidateV1(request, fixture.now); !core.HasReason(err, core.ReasonBindingDrift) {
				t.Fatalf("expected structural Graph drift rejection, got %v", err)
			}
		})
	}
}

func TestBuildCandidateRejectsResealedModuleComponentManifestDrift(t *testing.T) {
	fixture := newFixtureV1(t)
	fixture.request.Manifest.Modules[0].ComponentManifestRef.Digest = assemblytestkit.Digest("forged-component-manifest")
	resealChainV1(t, &fixture.request)
	if _, err := assemblyadapter.BuildCandidateV1(fixture.request, fixture.now); !core.HasReason(err, core.ReasonComponentMismatch) {
		t.Fatalf("expected module ComponentManifest drift rejection, got %v", err)
	}
}

func TestAssociationAdapterAcceptsExactLegalResidual(t *testing.T) {
	fixture := newResidualFixtureV1(t)
	if len(fixture.request.Manifest.Residuals) != 1 {
		t.Fatalf("expected one compiled legal Residual, got %d", len(fixture.request.Manifest.Residuals))
	}
	now := fixture.now
	port := newAssociationPortV1(&now)
	adapter, _ := assemblyadapter.New(port, func() time.Time { return now })
	result, err := adapter.Associate(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fact.CandidateDigest != result.Candidate.Digest || result.Conformance.Association == nil {
		t.Fatal("legal Residual association did not preserve Runtime Fact and read-only conformance")
	}
}

func TestBuildCandidateRejectsResealedResidualInspectCleanupAndOwnerDrift(t *testing.T) {
	fixture := newResidualFixtureV1(t)
	cases := map[string]func(*assemblycontract.ResidualReportV1){
		"inspect": func(residual *assemblycontract.ResidualReportV1) {
			residual.InspectContractRef.Ref = assemblytestkit.Ref("forged-residual-inspect")
		},
		"cleanup": func(residual *assemblycontract.ResidualReportV1) {
			residual.CleanupContractRef.Ref = assemblytestkit.Ref("forged-residual-cleanup")
		},
		"owner": func(residual *assemblycontract.ResidualReportV1) {
			residual.Owner = "praxis.fixture/forged-owner"
		},
		"inspect-owner-capability": func(residual *assemblycontract.ResidualReportV1) {
			residual.InspectContractRef.OwnerCapability = "praxis.fixture/forged-owner"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := fixture.request
			mutate(&request.Manifest.Residuals[0])
			resealResidualChainV1(t, &request)
			if _, err := assemblyadapter.BuildCandidateV1(request, fixture.now); !core.HasReason(err, core.ReasonBindingDrift) {
				t.Fatalf("expected exact Residual drift rejection, got %v", err)
			}
		})
	}
}

func TestAssociationAdapterSameIDDifferentContentFailsWithoutCreateReplay(t *testing.T) {
	fixture := newFixtureV1(t)
	now := fixture.now
	port := newAssociationPortV1(&now)
	adapter, _ := assemblyadapter.New(port, func() time.Time { return now })
	if _, err := adapter.Associate(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	fixture.request.RequestedExpiresUnixNano = now.Add(80 * time.Second).UnixNano()
	if _, err := adapter.Associate(context.Background(), fixture.request); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("expected same-ID payload mismatch, got %v", err)
	}
	if port.associateCalls != 1 {
		t.Fatalf("existing association was replayed through Create: calls=%d", port.associateCalls)
	}
}

func TestAssociationAdapterLostReplyRecoversOnlyByInspect(t *testing.T) {
	fixture := newFixtureV1(t)
	now := fixture.now
	port := newAssociationPortV1(&now)
	port.lostReplyOnce = true
	adapter, _ := assemblyadapter.New(port, func() time.Time { return now })
	result, err := adapter.Associate(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.RecoveredByInspect || port.associateCalls != 1 || port.inspectCalls != 2 {
		t.Fatalf("lost reply did not follow Inspect/Create/Inspect exactly: recovered=%v associate=%d inspect=%d", result.RecoveredByInspect, port.associateCalls, port.inspectCalls)
	}
	replayed, err := adapter.Associate(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.RecoveredByInspect || port.associateCalls != 1 {
		t.Fatal("replay did not recover from the initial Inspect")
	}
}

func TestAssociationAdapterConcurrentReplayReturnsOneRuntimeFact(t *testing.T) {
	fixture := newFixtureV1(t)
	now := fixture.now
	port := newAssociationPortV1(&now)
	adapter, _ := assemblyadapter.New(port, func() time.Time { return now })
	const workers = 32
	var wait sync.WaitGroup
	wait.Add(workers)
	digests := make(chan core.Digest, workers)
	errors := make(chan error, workers)
	for range workers {
		go func() {
			defer wait.Done()
			result, err := adapter.Associate(context.Background(), fixture.request)
			if err != nil {
				errors <- err
				return
			}
			digests <- result.Fact.Digest
		}()
	}
	wait.Wait()
	close(digests)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	var expected core.Digest
	for digest := range digests {
		if expected == "" {
			expected = digest
		}
		if digest != expected {
			t.Fatal("concurrent replay returned different Runtime Facts")
		}
	}
	if len(port.facts) != 1 {
		t.Fatalf("concurrent replay created %d authority records", len(port.facts))
	}
}

func TestAssociationAdapterConcurrentDifferentPayloadsLinearizeOneCandidate(t *testing.T) {
	fixture := newFixtureV1(t)
	now := fixture.now
	port := newAssociationPortV1(&now)
	adapter, _ := assemblyadapter.New(port, func() time.Time { return now })
	left := fixture.request
	right := fixture.request
	right.RequestedExpiresUnixNano = now.Add(80 * time.Second).UnixNano()

	const workers = 64
	start := make(chan struct{})
	results := make(chan assemblyadapter.AssociationResultV1, workers)
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for index := range workers {
		request := left
		if index%2 == 1 {
			request = right
		}
		go func() {
			defer wait.Done()
			<-start
			result, err := adapter.Associate(context.Background(), request)
			if err != nil {
				errors <- err
				return
			}
			results <- result
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errors)

	var winningDigest core.Digest
	successes := 0
	for result := range results {
		successes++
		if winningDigest == "" {
			winningDigest = result.Candidate.Digest
		}
		if result.Candidate.Digest != winningDigest || result.Fact.CandidateDigest != winningDigest {
			t.Fatal("concurrent contenders observed more than one winning candidate")
		}
	}
	conflicts := 0
	for err := range errors {
		if !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
			t.Fatalf("unexpected concurrent contender error: %v", err)
		}
		conflicts++
	}
	if successes == 0 || conflicts == 0 || len(port.facts) != 1 {
		t.Fatalf("expected one linearized payload with both successes and conflicts: successes=%d conflicts=%d facts=%d", successes, conflicts, len(port.facts))
	}
}

func TestAssociationAdapterConcurrentLostReplyNeverCreatesSecondFact(t *testing.T) {
	fixture := newFixtureV1(t)
	now := fixture.now
	port := newAssociationPortV1(&now)
	port.lostReplyOnce = true
	adapter, _ := assemblyadapter.New(port, func() time.Time { return now })
	const workers = 32
	start := make(chan struct{})
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			<-start
			if _, err := adapter.Associate(context.Background(), fixture.request); err != nil {
				errors <- err
			}
		}()
	}
	close(start)
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	if len(port.facts) != 1 {
		t.Fatalf("concurrent lost-reply recovery created %d Runtime Facts", len(port.facts))
	}
}

func TestAssociationAdapterRejectsFactThatBecameNonCurrentBeforeConformance(t *testing.T) {
	fixture := newFixtureV1(t)
	now := fixture.now
	port := newAssociationPortV1(&now)
	port.afterCreate = func() { now = now.Add(5 * time.Minute) }
	adapter, _ := assemblyadapter.New(port, func() time.Time { return now })
	if _, err := adapter.Associate(context.Background(), fixture.request); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expected expired association rejection, got %v", err)
	}
}

func TestAssociationAdapterRejectsConformanceClockRegression(t *testing.T) {
	fixture := newFixtureV1(t)
	now := fixture.now
	port := newAssociationPortV1(&now)
	port.afterCreate = func() { now = now.Add(-time.Second) }
	adapter, _ := assemblyadapter.New(port, func() time.Time { return now })
	if _, err := adapter.Associate(context.Background(), fixture.request); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("expected conformance clock regression, got %v", err)
	}
}

func TestAssociationAdapterRejectsDifferentPostCreateCurrentFact(t *testing.T) {
	fixture := newFixtureV1(t)
	now := fixture.now
	port := newAssociationPortV1(&now)
	port.differentCurrent = true
	adapter, _ := assemblyadapter.New(port, func() time.Time { return now })
	if _, err := adapter.Associate(context.Background(), fixture.request); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("expected post-create current Fact conflict, got %v", err)
	}
}

func TestBindingConformanceRejectsFactForDifferentHandoff(t *testing.T) {
	fixture := newFixtureV1(t)
	now := fixture.now
	port := newAssociationPortV1(&now)
	adapter, _ := assemblyadapter.New(port, func() time.Time { return now })
	result, err := adapter.Associate(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	wrong := fixture.request.Handoff
	wrong.GraphDigest = assemblytestkit.Digest("different-graph")
	wrong.Digest, _ = assemblycontract.HandoffDigestV1(wrong)
	if _, err := assemblyadapter.BuildBindingConformanceV1(wrong, result.Fact, now); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("expected exact handoff conflict, got %v", err)
	}
}

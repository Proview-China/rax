package conformance_test

import (
	"reflect"
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestInjectionConformanceStates(t *testing.T) {
	expected := expectedManifest()
	actual, observations := actualManifest()
	fact, err := kernel.CompareInjection("conformance-match", expected, actual, observations, testkit.Now+10)
	if err != nil || fact.State != contract.InjectionMatched {
		t.Fatalf("match=%#v err=%v", fact, err)
	}

	actual.ResidualPaths = []string{"provider.hidden_context"}
	fact, err = kernel.CompareInjection("conformance-residual", expected, actual, observations, testkit.Now+10)
	if err != nil || fact.State != contract.InjectionAllowedResidual {
		t.Fatalf("residual=%#v err=%v", fact, err)
	}

	actual, observations = actualManifest()
	actual.Fields[0].Digest = testkit.D("drift")
	fact, err = kernel.CompareInjection("conformance-rejected", expected, actual, observations, testkit.Now+10)
	if err != nil || fact.State != contract.InjectionRejected {
		t.Fatalf("rejected=%#v err=%v", fact, err)
	}

	actual, observations = actualManifest()
	actual.Fields[0].Opaque = true
	observations[0].Fields[0].Opaque = true
	actual.ObservationRefs[0], _ = observations[0].Ref(contract.ObservationFidelityComplete)
	fact, err = kernel.CompareInjection("conformance-unknown", expected, actual, observations, testkit.Now+10)
	if err != nil || fact.State != contract.InjectionUnknown {
		t.Fatalf("unknown=%#v err=%v", fact, err)
	}

	actual, observations = actualManifest()
	actual.Fields = append(actual.Fields, contract.InjectionField{Path: "provider.hidden", Digest: testkit.D("hidden")})
	fact, err = kernel.CompareInjection("conformance-undeclared", expected, actual, observations, testkit.Now+10)
	if err != nil || fact.State != contract.InjectionRejected {
		t.Fatalf("undeclared extra=%#v err=%v", fact, err)
	}
}

func TestObservationCausalBindingNeverMatchesOnMissingOrDrift(t *testing.T) {
	expected := expectedManifest()

	actual, _ := actualManifest()
	actual.ObservationRefs = nil
	fact, err := kernel.CompareInjection("missing-refs", expected, actual, nil, testkit.Now+10)
	if err != nil || fact.State != contract.InjectionUnknown {
		t.Fatalf("empty refs must be unknown: %#v err=%v", fact, err)
	}

	actual, _ = actualManifest()
	fact, err = kernel.CompareInjection("missing-observation", expected, actual, nil, testkit.Now+10)
	if err != nil || fact.State != contract.InjectionUnknown {
		t.Fatalf("missing provider observation must be unknown: %#v err=%v", fact, err)
	}

	tests := []struct {
		name   string
		mutate func(*contract.ProviderActualInjectionObservation)
	}{
		{"route", func(o *contract.ProviderActualInjectionObservation) { o.RouteID = "route-drift" }},
		{"attempt", func(o *contract.ProviderActualInjectionObservation) { o.AttemptID = "attempt-drift" }},
		{"frame", func(o *contract.ProviderActualInjectionObservation) {
			o.FrameRef = contract.FactRef{ID: "frame-drift", Revision: 1, Digest: testkit.D("frame-drift")}
		}},
		{"sequence", func(o *contract.ProviderActualInjectionObservation) { o.SourceSequence++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, observations := actualManifest()
			test.mutate(&observations[0])
			fact, err := kernel.CompareInjection("drift-"+test.name, expected, actual, observations, testkit.Now+10)
			if err != nil || fact.State != contract.InjectionRejected {
				t.Fatalf("drift must be rejected: %#v err=%v", fact, err)
			}
		})
	}
}

func TestIncompleteObservationFidelityIsUnknown(t *testing.T) {
	expected := expectedManifest()
	actual, observations := actualManifest()
	actual.ObservationRefs[0].Fidelity = contract.ObservationFidelityPartial
	fact, err := kernel.CompareInjection("partial-fidelity", expected, actual, observations, testkit.Now+10)
	if err != nil || fact.State != contract.InjectionUnknown {
		t.Fatalf("partial fidelity must be unknown: %#v err=%v", fact, err)
	}
}

func TestExpectedManifestCurrentnessBoundaries(t *testing.T) {
	expected := expectedManifest()
	actual, observations := actualManifest()
	actual.CreatedUnixNano = expected.CreatedUnixNano

	for _, current := range []int64{expected.CreatedUnixNano, expected.ExpiresUnixNano - 1} {
		fact, err := kernel.CompareInjection("current-boundary", expected, actual, observations, current)
		if err != nil || fact.State != contract.InjectionMatched {
			t.Fatalf("current=%d fact=%#v err=%v", current, fact, err)
		}
	}
	for _, current := range []int64{expected.CreatedUnixNano - 1, expected.ExpiresUnixNano, expected.ExpiresUnixNano + 1} {
		fact, err := kernel.CompareInjection("expired-boundary", expected, actual, observations, current)
		if err != nil || fact.State != contract.InjectionRejected || fact.Reason != "expected_manifest_not_current" {
			t.Fatalf("non-current=%d fact=%#v err=%v", current, fact, err)
		}
	}
}

func TestInjectionComparisonConcurrentDeterminism(t *testing.T) {
	expected := expectedManifest()
	first := contract.ProviderActualInjectionObservation{
		ContractVersion: contract.Version, ID: "provider-observation-1", Revision: 1, Execution: testkit.Execution(), FrameRef: frameRef(), RouteID: "route-1", AttemptID: "model-attempt-1", SourceSequence: 1,
		Fields: []contract.InjectionField{{Path: "messages.system", Digest: testkit.D("system"), Required: true}}, ObservedUnixNano: testkit.Now,
	}
	second := contract.ProviderActualInjectionObservation{
		ContractVersion: contract.Version, ID: "provider-observation-2", Revision: 1, Execution: testkit.Execution(), FrameRef: frameRef(), RouteID: "route-1", AttemptID: "model-attempt-1", SourceSequence: 2,
		Fields: []contract.InjectionField{{Path: "messages.user", Digest: testkit.D("user"), Required: true}}, ObservedUnixNano: testkit.Now + 1,
	}
	firstRef, _ := first.Ref(contract.ObservationFidelityComplete)
	secondRef, _ := second.Ref(contract.ObservationFidelityComplete)
	actual := contract.HarnessActualInjectionManifest{
		ContractVersion: contract.Version, ID: "actual-concurrent", Revision: 1, Execution: testkit.Execution(), FrameRef: frameRef(), RouteID: "route-1", AttemptID: "model-attempt-1",
		Fields: []contract.InjectionField{
			{Path: "messages.system", Digest: testkit.D("system"), Required: true},
			{Path: "messages.user", Digest: testkit.D("user"), Required: true},
		},
		ObservationRefs: []contract.ActualInjectionObservationRef{firstRef, secondRef}, CreatedUnixNano: testkit.Now + 2,
	}
	baseline, err := kernel.CompareInjection("concurrent-conformance", expected, actual, []contract.ProviderActualInjectionObservation{first, second}, testkit.Now+10)
	if err != nil || baseline.State != contract.InjectionMatched {
		t.Fatalf("baseline=%#v err=%v", baseline, err)
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 64; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			observations := []contract.ProviderActualInjectionObservation{first, second}
			if worker%2 == 1 {
				observations[0], observations[1] = observations[1], observations[0]
			}
			got, compareErr := kernel.CompareInjection("concurrent-conformance", expected, actual, observations, testkit.Now+10)
			if compareErr != nil {
				t.Errorf("worker %d: %v", worker, compareErr)
				return
			}
			if !reflect.DeepEqual(got, baseline) {
				t.Errorf("worker %d produced non-deterministic conformance", worker)
			}
		}()
	}
	wg.Wait()
}

func expectedManifest() contract.ExpectedInjectionManifest {
	return contract.ExpectedInjectionManifest{
		ContractVersion: contract.Version, ID: "expected-1", Revision: 1, Execution: testkit.Execution(), FrameRef: frameRef(),
		Fields: []contract.InjectionField{
			{Path: "messages.system", Digest: testkit.D("system"), Required: true},
			{Path: "messages.user", Digest: testkit.D("user"), Required: true},
		},
		AllowResidual: true, CapabilityRef: contract.FactRef{ID: "harness-capability", Revision: 1, Digest: testkit.D("capability")}, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 100,
	}
}

func actualManifest() (contract.HarnessActualInjectionManifest, []contract.ProviderActualInjectionObservation) {
	observation := contract.ProviderActualInjectionObservation{
		ContractVersion: contract.Version, ID: "provider-observation", Revision: 1, Execution: testkit.Execution(), FrameRef: frameRef(), RouteID: "route-1", AttemptID: "model-attempt-1", SourceSequence: 1,
		Fields: []contract.InjectionField{
			{Path: "messages.system", Digest: testkit.D("system"), Required: true},
			{Path: "messages.user", Digest: testkit.D("user"), Required: true},
		},
		ObservedUnixNano: testkit.Now,
	}
	ref, err := observation.Ref(contract.ObservationFidelityComplete)
	if err != nil {
		panic(err)
	}
	manifest := contract.HarnessActualInjectionManifest{
		ContractVersion: contract.Version, ID: "actual-1", Revision: 1, Execution: testkit.Execution(), FrameRef: frameRef(), RouteID: "route-1", AttemptID: "model-attempt-1",
		Fields: []contract.InjectionField{
			{Path: "messages.system", Digest: testkit.D("system"), Required: true},
			{Path: "messages.user", Digest: testkit.D("user"), Required: true},
		},
		ObservationRefs: []contract.ActualInjectionObservationRef{ref}, CreatedUnixNano: testkit.Now + 1,
	}
	return manifest, []contract.ProviderActualInjectionObservation{observation}
}

func frameRef() contract.FactRef {
	return contract.FactRef{ID: "frame-1", Revision: 1, Digest: testkit.D("frame")}
}

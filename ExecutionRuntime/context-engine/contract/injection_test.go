package contract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestActualObservationRefsAreNonEmptyCanonicalAndUnique(t *testing.T) {
	first := providerObservation("observation-1", 1)
	second := providerObservation("observation-2", 2)
	firstRef, _ := first.Ref(contract.ObservationFidelityComplete)
	secondRef, _ := second.Ref(contract.ObservationFidelityComplete)
	manifest := actualWithRefs([]contract.ActualInjectionObservationRef{firstRef, secondRef})
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}

	if err := actualWithRefs(nil).Validate(); err == nil {
		t.Fatal("empty observation refs were accepted")
	}
	if err := actualWithRefs([]contract.ActualInjectionObservationRef{secondRef, firstRef}).Validate(); err == nil {
		t.Fatal("non-canonical observation order was accepted")
	}
	duplicateSequence := secondRef
	duplicateSequence.SourceSequence = firstRef.SourceSequence
	if err := actualWithRefs([]contract.ActualInjectionObservationRef{firstRef, duplicateSequence}).Validate(); err == nil {
		t.Fatal("duplicate source sequence was accepted")
	}
	duplicateID := secondRef
	duplicateID.ID = firstRef.ID
	if err := actualWithRefs([]contract.ActualInjectionObservationRef{firstRef, duplicateID}).Validate(); err == nil {
		t.Fatal("duplicate observation id was accepted")
	}

	sorted := contract.SortActualInjectionObservationRefs([]contract.ActualInjectionObservationRef{secondRef, firstRef})
	if sorted[0] != firstRef || sorted[1] != secondRef {
		t.Fatalf("unexpected canonical order: %#v", sorted)
	}
}

func providerObservation(id string, sequence uint64) contract.ProviderActualInjectionObservation {
	return contract.ProviderActualInjectionObservation{
		ContractVersion: contract.Version, ID: id, Revision: 1, Execution: testkit.Execution(), FrameRef: injectionFrameRef(), RouteID: "route-1", AttemptID: "attempt-1", SourceSequence: sequence,
		Fields: []contract.InjectionField{{Path: "messages.system", Digest: testkit.D("system"), Required: true}}, ObservedUnixNano: testkit.Now + int64(sequence),
	}
}

func actualWithRefs(refs []contract.ActualInjectionObservationRef) contract.HarnessActualInjectionManifest {
	return contract.HarnessActualInjectionManifest{
		ContractVersion: contract.Version, ID: "actual-1", Revision: 1, Execution: testkit.Execution(), FrameRef: injectionFrameRef(), RouteID: "route-1", AttemptID: "attempt-1",
		Fields: []contract.InjectionField{{Path: "messages.system", Digest: testkit.D("system"), Required: true}}, ObservationRefs: refs, CreatedUnixNano: testkit.Now + 10,
	}
}

func injectionFrameRef() contract.FactRef {
	return contract.FactRef{ID: "frame-1", Revision: 1, Digest: testkit.D("frame")}
}

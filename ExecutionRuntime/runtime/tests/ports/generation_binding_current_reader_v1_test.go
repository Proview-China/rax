package ports_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGenerationBindingAssociationCurrentReaderV1IsCapabilityNarrowed(t *testing.T) {
	readerType := reflect.TypeOf((*ports.GenerationBindingAssociationCurrentReaderV1)(nil)).Elem()
	if readerType.NumMethod() != 1 {
		t.Fatalf("current Reader exposes %d methods, want exactly one", readerType.NumMethod())
	}
	method, ok := readerType.MethodByName("InspectCurrentGenerationBindingAssociationV1")
	if !ok || method.Type.NumIn() != 2 || method.Type.NumOut() != 2 {
		t.Fatalf("current Reader signature drifted: %+v", method)
	}
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()
	factType := reflect.TypeOf(ports.GenerationBindingAssociationFactV1{})
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if method.Type.In(0) != contextType || method.Type.In(1).Kind() != reflect.String || method.Type.Out(0) != factType || method.Type.Out(1) != errorType {
		t.Fatalf("current Reader parameter/result types drifted: %v", method.Type)
	}
	if _, exposed := readerType.MethodByName("AssociateGenerationBindingV1"); exposed {
		t.Fatal("capability-narrowed Reader exposes Associate authority")
	}

	governanceType := reflect.TypeOf((*ports.GenerationBindingAssociationGovernancePortV1)(nil)).Elem()
	if governanceType.NumMethod() != 2 {
		t.Fatalf("Governance method set changed: %d", governanceType.NumMethod())
	}
	if _, ok := governanceType.MethodByName("AssociateGenerationBindingV1"); !ok {
		t.Fatal("Governance Port lost Associate authority")
	}
	if _, ok := governanceType.MethodByName("InspectCurrentGenerationBindingAssociationV1"); !ok {
		t.Fatal("Governance Port no longer embeds the current Reader")
	}
}

func TestGenerationBindingAssociationCurrentReaderV1PublicConformance(t *testing.T) {
	now := time.Unix(83_000, 0)
	candidate := generationBindingCandidatePortV1(t, now)
	fact, err := ports.SealGenerationBindingAssociationFactV1(ports.GenerationBindingAssociationFactV1{
		ID: candidate.AssociationID, Revision: 1, State: ports.GenerationBindingAssociationActiveV1,
		Candidate: candidate, CandidateDigest: candidate.Digest,
		CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := generationBindingReaderOnlyV1{fact: fact}
	var _ ports.GenerationBindingAssociationCurrentReaderV1 = reader
	report, err := conformance.CheckGenerationBindingAssociationCurrentReaderV1(context.Background(), conformance.GenerationBindingAssociationCurrentReaderCaseV1{Reader: reader, AssociationID: fact.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !report.CurrentInspectObserved || report.AssociateAuthorityUsed || report.ProductionClaimEligible {
		t.Fatalf("reader-only conformance widened capability: %+v", report)
	}
}

func TestGenerationBindingAssociationCurrentReaderV1RejectsTypedNilAndWrongFact(t *testing.T) {
	var typedNil *generationBindingPointerReaderV1
	if _, err := conformance.CheckGenerationBindingAssociationCurrentReaderV1(context.Background(), conformance.GenerationBindingAssociationCurrentReaderCaseV1{Reader: typedNil, AssociationID: "association-reader"}); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Reader did not fail closed: %v", err)
	}

	now := time.Unix(84_000, 0)
	candidate := generationBindingCandidatePortV1(t, now)
	fact, err := ports.SealGenerationBindingAssociationFactV1(ports.GenerationBindingAssociationFactV1{
		ID: candidate.AssociationID, Revision: 1, State: ports.GenerationBindingAssociationActiveV1,
		Candidate: candidate, CandidateDigest: candidate.Digest,
		CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conformance.CheckGenerationBindingAssociationCurrentReaderV1(context.Background(), conformance.GenerationBindingAssociationCurrentReaderCaseV1{Reader: generationBindingReaderOnlyV1{fact: fact}, AssociationID: "association-other"}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Reader returned another association without Conflict: %v", err)
	}
}

func TestGenerationBindingAssociationCurrentReaderV1ImportBoundary(t *testing.T) {
	if err := conformance.CheckAdapterRuntimeImportsV2([]string{"github.com/Proview-China/rax/ExecutionRuntime/runtime/core", "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"}); err != nil {
		t.Fatalf("reader consumer cannot use the public Runtime boundary: %v", err)
	}
	if err := conformance.CheckAdapterRuntimeImportsV2([]string{"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"}); err == nil {
		t.Fatal("reader consumer was allowed to import Runtime Fact Owner implementation")
	}
}

type generationBindingReaderOnlyV1 struct {
	fact ports.GenerationBindingAssociationFactV1
}

func (r generationBindingReaderOnlyV1) InspectCurrentGenerationBindingAssociationV1(context.Context, string) (ports.GenerationBindingAssociationFactV1, error) {
	return r.fact, nil
}

type generationBindingPointerReaderV1 struct{}

func (*generationBindingPointerReaderV1) InspectCurrentGenerationBindingAssociationV1(context.Context, string) (ports.GenerationBindingAssociationFactV1, error) {
	return ports.GenerationBindingAssociationFactV1{}, nil
}

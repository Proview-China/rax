package toolcallobservation_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func TestProjectionPortsRemainNarrowAndDirectionallySeparated(t *testing.T) {
	publisher := reflect.TypeOf((*modelinvoker.ToolCallCandidateObservationProjectionPublisherV1)(nil)).Elem()
	reader := reflect.TypeOf((*modelinvoker.ToolCallCandidateObservationProjectionReaderV1)(nil)).Elem()
	repository := reflect.TypeOf((*modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1)(nil)).Elem()
	if publisher.NumMethod() != 1 || publisher.Method(0).Name != "PublishSealedProjectionV1" {
		t.Fatalf("publisher capability widened: %v", publisher)
	}
	if reader.NumMethod() != 1 || reader.Method(0).Name != "InspectExactProjectionV1" {
		t.Fatalf("reader capability widened: %v", reader)
	}
	if repository.NumMethod() != 1 || repository.Method(0).Name != "EnsureSealedProjectionV1" {
		t.Fatalf("repository must expose only the atomic producer capability: %v", repository)
	}
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	consumer := projectionReaderOnlyConsumerV1{reader: store}
	projection := repositoryProjection(t, modelinvoker.FunctionCall{ID: "reader-only", Name: "lookup", Arguments: json.RawMessage(`{}`)})
	if _, err := consumer.inspect(context.Background(), projection.Ref); !modelinvoker.IsToolCallCandidateObservationProjectionAuthoritativeNotFoundV1(err) {
		t.Fatalf("reader-only consumer classification = %v", err)
	}
}

func TestProjectionAuthoritativeNotFoundRequiresConsistencyProof(t *testing.T) {
	withoutProof := &modelinvoker.ToolCallCandidateObservationProjectionErrorV1{
		Kind: modelinvoker.ToolCallCandidateObservationProjectionErrorAuthoritativeAbsent, Operation: "inspect",
	}
	withProof := &modelinvoker.ToolCallCandidateObservationProjectionErrorV1{
		Kind: modelinvoker.ToolCallCandidateObservationProjectionErrorAuthoritativeAbsent, Operation: "inspect",
		Consistency: modelinvoker.ToolCallCandidateObservationProjectionConsistencyLinearizableNeverCreated,
	}
	ordinary := &modelinvoker.ToolCallCandidateObservationProjectionErrorV1{
		Kind: modelinvoker.ToolCallCandidateObservationProjectionErrorUnknownAbsent, Operation: "inspect",
	}
	if modelinvoker.IsToolCallCandidateObservationProjectionAuthoritativeNotFoundV1(withoutProof) {
		t.Fatal("authoritative kind without Repository consistency proof was accepted")
	}
	if !modelinvoker.IsToolCallCandidateObservationProjectionAuthoritativeNotFoundV1(withProof) {
		t.Fatal("linearizable never-created proof was not recognized")
	}
	if modelinvoker.IsToolCallCandidateObservationProjectionAuthoritativeNotFoundV1(ordinary) {
		t.Fatal("ordinary NotFound was upgraded to authoritative")
	}
}

type projectionReaderOnlyConsumerV1 struct {
	reader modelinvoker.ToolCallCandidateObservationProjectionReaderV1
}

func (consumer projectionReaderOnlyConsumerV1) inspect(ctx context.Context, ref modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	return consumer.reader.InspectExactProjectionV1(ctx, ref)
}

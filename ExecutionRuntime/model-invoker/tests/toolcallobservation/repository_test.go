package toolcallobservation_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestProjectionReferenceStoreCreateOnceExactReadAndClone(t *testing.T) {
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	var _ modelinvoker.ToolCallCandidateObservationProjectionPublisherV1 = store
	var _ modelinvoker.ToolCallCandidateObservationProjectionReaderV1 = store
	projection := repositoryProjection(t,
		modelinvoker.FunctionCall{ID: "call-a", Name: "lookup", Arguments: json.RawMessage(`{"b":2,"a":1}`)},
		modelinvoker.FunctionCall{ID: "call-b", Name: "lookup", Arguments: json.RawMessage(`{"path":"b"}`)},
	)

	for range 2 {
		ref, err := store.PublishSealedProjectionV1(context.Background(), projection.Clone())
		if err != nil || ref != projection.Ref {
			t.Fatalf("Publish() = %#v, %v", ref, err)
		}
	}
	first, err := store.InspectExactProjectionV1(context.Background(), projection.Ref)
	if err != nil || !reflect.DeepEqual(first, projection) {
		t.Fatalf("Inspect() = %#v, %v", first, err)
	}
	first.Observation.Calls[0].CanonicalArguments[2] = 'z'
	second, err := store.InspectExactProjectionV1(context.Background(), projection.Ref)
	if err != nil || !reflect.DeepEqual(second, projection) {
		t.Fatalf("second Inspect() aliased first result: %#v, %v", second, err)
	}
	stats := store.StatsV1()
	if stats.PublishCalls != 2 || stats.Records != 1 {
		t.Fatalf("stats = %#v", stats)
	}
}

func TestProjectionReferenceStoreZeroValueIsSafeReferenceFixture(t *testing.T) {
	var store modelinvoker.InMemoryToolCallCandidateObservationProjectionStoreV1
	projection := repositoryProjection(t, modelinvoker.FunctionCall{ID: "zero-value", Name: "lookup", Arguments: json.RawMessage(`{}`)})
	ensured, err := store.EnsureSealedProjectionV1(context.Background(), projection)
	if err != nil || !reflect.DeepEqual(ensured, projection) {
		t.Fatalf("zero-value Ensure() = %#v/%v", ensured, err)
	}
	got, err := store.InspectExactProjectionV1(context.Background(), projection.Ref)
	if err != nil || !reflect.DeepEqual(got, projection) || store.StatsV1().EnsureCalls != 1 || store.StatsV1().Records != 1 {
		t.Fatalf("zero-value Inspect() = %#v/%v/%#v", got, err, store.StatsV1())
	}
}

func TestProjectionReferenceStoreEnsureIsAtomicCreateOrReturnExisting(t *testing.T) {
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	projection := repositoryProjection(t,
		modelinvoker.FunctionCall{ID: "ensure-a", Name: "lookup", Arguments: json.RawMessage(`{"path":"a"}`)},
		modelinvoker.FunctionCall{ID: "ensure-b", Name: "lookup", Arguments: json.RawMessage(`{"path":"b"}`)},
	)
	for range 2 {
		ensured, err := store.EnsureSealedProjectionV1(context.Background(), projection.Clone())
		if err != nil || !reflect.DeepEqual(ensured, projection) {
			t.Fatalf("Ensure() = %#v/%v", ensured, err)
		}
		ensured.Observation.Calls[0].CanonicalArguments[2] = 'z'
	}
	got, err := store.InspectExactProjectionV1(context.Background(), projection.Ref)
	if err != nil || !reflect.DeepEqual(got, projection) || store.StatsV1().EnsureCalls != 2 || store.StatsV1().Records != 1 {
		t.Fatalf("atomic Ensure state = %#v/%v/%#v", got, err, store.StatsV1())
	}
}

type typedNilProjectionRepository struct{}

func (*typedNilProjectionRepository) EnsureSealedProjectionV1(context.Context, modelinvoker.ToolCallCandidateObservationProjectionV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	panic("typed-nil repository must never be invoked")
}

type fixedProjectionRepository struct {
	projection modelinvoker.ToolCallCandidateObservationProjectionV1
}

func (repository fixedProjectionRepository) EnsureSealedProjectionV1(context.Context, modelinvoker.ToolCallCandidateObservationProjectionV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	return repository.projection.Clone(), nil
}

func TestCanonicalProjectionProducerRejectsNilAndTypedNilRepository(t *testing.T) {
	sealed := repositoryProjection(t, modelinvoker.FunctionCall{ID: "typed-nil", Name: "lookup", Arguments: json.RawMessage(`{}`)})
	var nilRepository modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1
	var typedNilPointer *typedNilProjectionRepository
	var typedNilRepository modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1 = typedNilPointer
	for name, repository := range map[string]modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1{
		"nil interface":                  nilRepository,
		"typed-nil pointer in interface": typedNilRepository,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := modelinvoker.EnsureToolCallCandidateObservationProjectionV1(context.Background(), repository, sealed)
			if modelinvoker.ToolCallCandidateObservationProjectionErrorKindOfV1(err) != modelinvoker.ToolCallCandidateObservationProjectionErrorUnavailable {
				t.Fatalf("producer helper error = %v", err)
			}
		})
	}
}

func TestCanonicalProjectionProducerRejectsValidButNonExactProjection(t *testing.T) {
	sealed := repositoryProjection(t, modelinvoker.FunctionCall{ID: "sealed", Name: "lookup", Arguments: json.RawMessage(`{"path":"a"}`)})
	different := repositoryProjection(t, modelinvoker.FunctionCall{ID: "different", Name: "lookup", Arguments: json.RawMessage(`{"path":"b"}`)})
	if sealed.Ref == different.Ref || sealed.Validate() != nil || different.Validate() != nil {
		t.Fatalf("test projections are not distinct and valid: %#v/%#v", sealed.Ref, different.Ref)
	}
	_, err := modelinvoker.EnsureToolCallCandidateObservationProjectionV1(
		context.Background(), fixedProjectionRepository{projection: different}, sealed,
	)
	if modelinvoker.ToolCallCandidateObservationProjectionErrorKindOfV1(err) != modelinvoker.ToolCallCandidateObservationProjectionErrorConflict {
		t.Fatalf("valid non-exact Ensure result error = %v", err)
	}
}

func TestProjectionReferenceStoreAuthoritativeNotFoundRequiresLinearizableAbsence(t *testing.T) {
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	projection := repositoryProjection(t, modelinvoker.FunctionCall{ID: "missing", Name: "lookup", Arguments: json.RawMessage(`{}`)})
	_, err := store.InspectExactProjectionV1(context.Background(), projection.Ref)
	if !modelinvoker.IsToolCallCandidateObservationProjectionAuthoritativeNotFoundV1(err) || modelinvoker.ToolCallCandidateObservationProjectionErrorKindOfV1(err) != modelinvoker.ToolCallCandidateObservationProjectionErrorAuthoritativeAbsent {
		t.Fatalf("missing exact ref classification = %v", err)
	}

	invalid := projection.Ref
	invalid.Digest = ""
	_, err = store.InspectExactProjectionV1(context.Background(), invalid)
	if modelinvoker.IsToolCallCandidateObservationProjectionAuthoritativeNotFoundV1(err) || modelinvoker.ToolCallCandidateObservationProjectionErrorKindOfV1(err) != modelinvoker.ToolCallCandidateObservationProjectionErrorInvalid {
		t.Fatalf("invalid ref classification = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.InspectExactProjectionV1(ctx, projection.Ref)
	if modelinvoker.IsToolCallCandidateObservationProjectionAuthoritativeNotFoundV1(err) || modelinvoker.ToolCallCandidateObservationProjectionErrorKindOfV1(err) != modelinvoker.ToolCallCandidateObservationProjectionErrorIndeterminate || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read classification = %v", err)
	}
}

func TestProjectionReferenceStoreSourceConflictIsAtomic(t *testing.T) {
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	first := repositoryProjection(t, modelinvoker.FunctionCall{ID: "first", Name: "lookup", Arguments: json.RawMessage(`{"value":1}`)})
	secondObservation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(invocationDigest(), response(
		modelinvoker.FunctionCall{ID: "second", Name: "lookup", Arguments: json.RawMessage(`{"value":2}`)},
	))
	if err != nil {
		t.Fatal(err)
	}
	second, err := modelinvoker.NewToolCallCandidateObservationProjectionV1(
		first.Ref.InvocationID, first.Ref.Source.SourceSequence, first.Ref.Source.ResponseID, secondObservation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PublishSealedProjectionV1(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PublishSealedProjectionV1(context.Background(), second); modelinvoker.ToolCallCandidateObservationProjectionErrorKindOfV1(err) != modelinvoker.ToolCallCandidateObservationProjectionErrorConflict {
		t.Fatalf("source conflict = %v", err)
	}
	got, err := store.InspectExactProjectionV1(context.Background(), first.Ref)
	if err != nil || !reflect.DeepEqual(got, first) || store.StatsV1().Records != 1 {
		t.Fatalf("winner changed after conflict: %#v/%v/%#v", got, err, store.StatsV1())
	}
}

func TestProjectionReferenceStoreSameIDChangedCanonicalContentConflicts(t *testing.T) {
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	first := repositoryProjection(t, modelinvoker.FunctionCall{ID: "first-id", Name: "lookup", Arguments: json.RawMessage(`{"value":1}`)})
	secondObservation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(invocationDigest(), response(
		modelinvoker.FunctionCall{ID: "second-id", Name: "lookup", Arguments: json.RawMessage(`{"value":2}`)},
	))
	if err != nil {
		t.Fatal(err)
	}
	second, err := modelinvoker.NewToolCallCandidateObservationProjectionV1("execution-repository", 8, "response-other", secondObservation)
	if err != nil {
		t.Fatal(err)
	}
	second.Ref.ID = first.Ref.ID
	second.Ref.Digest = ""
	second.Ref.Digest, err = core.CanonicalJSONDigest(
		"praxis.model-invoker.tool-call-observation-projection", "v1", "ToolCallCandidateObservationRefV1", second.Ref,
	)
	if err != nil || second.Validate() != nil {
		t.Fatalf("forged valid same-ID projection = %#v/%v", second.Ref, err)
	}
	if _, err = store.PublishSealedProjectionV1(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PublishSealedProjectionV1(context.Background(), second); modelinvoker.ToolCallCandidateObservationProjectionErrorKindOfV1(err) != modelinvoker.ToolCallCandidateObservationProjectionErrorConflict {
		t.Fatalf("same ID changed content error = %v", err)
	}
	if store.StatsV1().Records != 1 {
		t.Fatalf("same ID conflict changed records: %#v", store.StatsV1())
	}
}

func TestProjectionReferenceStoreConcurrentEnsureCreateOnceAndConflict(t *testing.T) {
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	first := repositoryProjection(t, modelinvoker.FunctionCall{ID: "same", Name: "lookup", Arguments: json.RawMessage(`{"value":1}`)})
	secondObservation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(invocationDigest(), response(
		modelinvoker.FunctionCall{ID: "other", Name: "lookup", Arguments: json.RawMessage(`{"value":2}`)},
	))
	if err != nil {
		t.Fatal(err)
	}
	second, err := modelinvoker.NewToolCallCandidateObservationProjectionV1(first.Ref.InvocationID, first.Ref.Source.SourceSequence, first.Ref.Source.ResponseID, secondObservation)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 64
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for index := range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			candidate := first
			if index%2 == 1 {
				candidate = second
			}
			_, ensureErr := store.EnsureSealedProjectionV1(context.Background(), candidate)
			errs <- ensureErr
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	successes, conflicts := 0, 0
	for publishErr := range errs {
		switch modelinvoker.ToolCallCandidateObservationProjectionErrorKindOfV1(publishErr) {
		case "":
			successes++
		case modelinvoker.ToolCallCandidateObservationProjectionErrorConflict:
			conflicts++
		default:
			t.Fatalf("unexpected concurrent error: %v", publishErr)
		}
	}
	if successes != workers/2 || conflicts != workers/2 || store.StatsV1().EnsureCalls != workers || store.StatsV1().Records != 1 {
		t.Fatalf("success/conflict/stats = %d/%d/%#v", successes, conflicts, store.StatsV1())
	}
}

func TestProjectionReferenceStoreConcurrentReadersReturnImmutableClones(t *testing.T) {
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	projection := repositoryProjection(t, modelinvoker.FunctionCall{ID: "clone", Name: "lookup", Arguments: json.RawMessage(`{"value":"stable"}`)})
	if _, err := store.PublishSealedProjectionV1(context.Background(), projection); err != nil {
		t.Fatal(err)
	}
	const readers = 64
	start := make(chan struct{})
	errs := make(chan error, readers)
	var wait sync.WaitGroup
	for index := range readers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			clone, err := store.InspectExactProjectionV1(context.Background(), projection.Ref)
			if err == nil {
				clone.Observation.Calls[0].CanonicalArguments[2] = byte('a' + index%26)
			}
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.InspectExactProjectionV1(context.Background(), projection.Ref)
	if err != nil || !reflect.DeepEqual(got, projection) {
		t.Fatalf("concurrent reader mutated store: %#v/%v", got, err)
	}
}

func repositoryProjection(t *testing.T, calls ...modelinvoker.FunctionCall) modelinvoker.ToolCallCandidateObservationProjectionV1 {
	t.Helper()
	observation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(invocationDigest(), response(calls...))
	if err != nil {
		t.Fatal(err)
	}
	projection, err := modelinvoker.NewToolCallCandidateObservationProjectionV1("execution-repository", 7, "response-repository", observation)
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

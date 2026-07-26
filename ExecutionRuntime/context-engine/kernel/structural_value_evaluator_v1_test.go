package kernel_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestPrepareCompressionEvidenceDeterministicAndAdvisory(t *testing.T) {
	request := compressionRequestFixtureV1(t)
	first, err := kernel.PrepareCompressionEvidenceV1(context.Background(), kernel.DeterministicStructuralValueEvaluatorV1{}, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := kernel.PrepareCompressionEvidenceV1(context.Background(), kernel.DeterministicStructuralValueEvaluatorV1{}, request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.Evaluation.StructuralValuePPM == 0 || first.Evidence.InvariantGateDigest.Validate() != nil {
		t.Fatal("deterministic advisory result mismatch")
	}
}

func TestInvariantGateRejectsRequiredAndOpenEffectRemoval(t *testing.T) {
	request := compressionRequestFixtureV1(t)
	request.Invariant.RetainedAnchorRefs = []contract.FactRef{}
	if _, err := kernel.PrepareCompressionEvidenceV1(context.Background(), kernel.DeterministicStructuralValueEvaluatorV1{}, request); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("removed required ref error=%v", err)
	}
	request = compressionRequestFixtureV1(t)
	open := contract.FactRef{ID: "open-effect", Revision: 1, Digest: testkit.D("open")}
	request.Invariant.SourceGeneration.OpenEffects = []contract.FactRef{open}
	if _, err := kernel.PrepareCompressionEvidenceV1(context.Background(), kernel.DeterministicStructuralValueEvaluatorV1{}, request); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("removed open effect error=%v", err)
	}
}

func TestEvaluatorFailureAndCancelProduceZeroEvidence(t *testing.T) {
	request := compressionRequestFixtureV1(t)
	if result, err := kernel.PrepareCompressionEvidenceV1(context.Background(), failingEvaluatorV1{err: contract.ErrUnavailable}, request); !errors.Is(err, contract.ErrUnavailable) || !reflect.DeepEqual(result, kernel.CompressionPreparationResultV1{}) {
		t.Fatalf("unavailable result=%#v err=%v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := kernel.PrepareCompressionEvidenceV1(ctx, kernel.DeterministicStructuralValueEvaluatorV1{}, request); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(result, kernel.CompressionPreparationResultV1{}) {
		t.Fatalf("cancel result=%#v err=%v", result, err)
	}
}

type failingEvaluatorV1 struct{ err error }

func (f failingEvaluatorV1) EvaluateStructuralValueV1(context.Context, contract.StructuralValueEvaluationRequestV1, string) (contract.StructuralValueEvaluationV1, error) {
	return contract.StructuralValueEvaluationV1{}, f.err
}

func compressionRequestFixtureV1(t *testing.T) kernel.CompressionPreparationRequestV1 {
	t.Helper()
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	anchor := contract.FactRef{ID: "required-anchor", Revision: 1, Digest: testkit.D("anchor")}
	return kernel.CompressionPreparationRequestV1{
		EvaluationID: "structural-evaluation-1", EvidenceID: "compression-evidence-1",
		EvaluatorRef:     contract.ContextEvaluatorRefV1{ID: "deterministic-evaluator", Revision: 1, Digest: testkit.D("deterministic-evaluator")},
		EvaluatorVersion: "v1",
		Invariant: kernel.CompressionInvariantInputV1{
			SourceFrame: fixture.Result.Frame, SourceGeneration: fixture.Generation,
			CandidateOutputRef: fixture.Result.Frame.DynamicTail,
			RequiredAnchorRefs: []contract.FactRef{anchor}, RetainedAnchorRefs: []contract.FactRef{anchor},
			DeltaRefs: []contract.FactRef{}, ProtectedRefs: []contract.FactRef{},
			SourceTokens: 100, CandidateTokens: 40, CheckedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 100,
		},
	}
}

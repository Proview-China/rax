package preparedinvocation_test

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestM3PublicReaderAndNeutralAckBoundaryIsExact(t *testing.T) {
	historical := reflect.TypeOf((*modelinvoker.PreparedModelInvocationReaderV1)(nil)).Elem()
	assertExactInterfaceMethodV1(t, historical, "InspectExactPreparedModelInvocationV1", reflect.TypeOf(modelinvoker.PreparedModelInvocationRefV1{}), reflect.TypeOf(modelinvoker.PreparedModelInvocationFactV1{}))

	current := reflect.TypeOf((*modelinvoker.PreparedModelInvocationCurrentReaderV1)(nil)).Elem()
	assertExactInterfaceMethodV1(t, current, "InspectExactPreparedModelInvocationCurrentV1", reflect.TypeOf(modelinvoker.PreparedModelInvocationCurrentRefV1{}), reflect.TypeOf(modelinvoker.PreparedModelInvocationCurrentProjectionV1{}))

	assertExactStructFieldsV1(t, reflect.TypeOf(modelinvoker.PreparedModelInvocationSurfaceBindingRefV1{}), []string{
		"Owner", "ContractVersion", "ID", "Revision", "Digest",
	})
	assertExactStructFieldsV1(t, reflect.TypeOf(modelinvoker.PreparedModelInvocationCommitAckRefV1{}), []string{
		"ContractVersion", "ID", "Revision", "Digest", "PreparedRef", "CurrentRef", "SurfaceBindingRef", "CheckedUnixNano", "ExpiresUnixNano", "NotAfterUnixNano",
	})
	assertExactStructFieldsV1(t, reflect.TypeOf(modelinvoker.PreparedModelInvocationCommitAckV1{}), []string{
		"ContractVersion", "ID", "Revision", "Digest", "PreparedRef", "CurrentRef", "GateImplementationRef", "SurfaceBindingRef", "CheckedUnixNano", "ExpiresUnixNano", "NotAfterUnixNano",
	})
	assertExactStructFieldsV1(t, reflect.TypeOf(modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1{}), []string{
		"ContractVersion", "ID", "Revision", "Digest", "PreparedRef", "CurrentRef", "AckRef", "DispatchSequence", "BoundaryKind", "ProviderAttemptOrdinal", "AttemptRequestDigest", "ActualToolSurfaceDigest", "ActualProviderInjectionDigest", "CheckedUnixNano",
	})
}

func TestM3ModelOnlyAttemptFailsClosedBeforeProvider(t *testing.T) {
	fact := sealedFact()
	current := sealedCurrent(fact)
	ack := sealedAck(fact, current)

	tests := []struct {
		name   string
		gate   modelinvoker.PreparedModelInvocationCommitGateV1
		now    time.Time
		mutate func(*modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1)
	}{
		{
			name: "typed nil gate",
			gate: (*gateFake)(nil),
			now:  time.Unix(0, 4_000),
		},
		{
			name: "gate unavailable",
			gate: &gateFake{commitErr: core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "gate unavailable")},
			now:  time.Unix(0, 4_000),
		},
		{
			name: "unknown commit without stable ack ref",
			gate: &gateFake{commitErr: core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "reply lost")},
			now:  time.Unix(0, 4_000),
		},
		{
			name: "ack expires at attempt boundary",
			gate: &gateFake{ack: ack},
			now:  time.Unix(0, ack.ExpiresUnixNano),
		},
		{
			name: "tool surface digest drift",
			gate: &gateFake{ack: ack},
			now:  time.Unix(0, 4_000),
			mutate: func(receipt *modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1) {
				receipt.ActualToolSurfaceDigest = digest("drifted-tool-surface")
			},
		},
		{
			name: "provider injection digest drift",
			gate: &gateFake{ack: ack},
			now:  time.Unix(0, 4_000),
			mutate: func(receipt *modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1) {
				receipt.ActualProviderInjectionDigest = digest("drifted-provider-injection")
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var providerCalls atomic.Uint64
			if err := runModelOnlyM3AttemptV1(context.Background(), testCase.gate, fact, current, testCase.now, testCase.mutate, &providerCalls); err == nil {
				t.Fatal("invalid M3 boundary reached the provider")
			}
			if providerCalls.Load() != 0 {
				t.Fatalf("provider calls = %d", providerCalls.Load())
			}
		})
	}
}

func TestM3LostReplyExactAckDriftFailsClosedBeforeProvider(t *testing.T) {
	fact := sealedFact()
	current := sealedCurrent(fact)
	ack := sealedAck(fact, current)
	driftedDraft := ack
	driftedDraft.ID = ""
	driftedDraft.Digest = ""
	driftedDraft.SurfaceBindingRef.ID = "surface-binding-drifted"
	driftedDraft.SurfaceBindingRef.Digest = digest("surface-binding-drifted")
	drifted, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(driftedDraft)
	if err != nil || drifted.Ref() == ack.Ref() {
		t.Fatalf("drifted ACK = %#v, %v", drifted, err)
	}

	gate := &gateFake{
		ack:        ack,
		commitErr:  core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "reply lost"),
		inspectAck: drifted,
	}
	var providerCalls atomic.Uint64
	if err := runModelOnlyM3AttemptV1(context.Background(), gate, fact, current, time.Unix(0, 4_000), nil, &providerCalls); err == nil {
		t.Fatal("drifted recovered ACK reached the provider")
	}
	if gate.commitCalls.Load() != 1 || gate.inspectCalls.Load() != 1 || providerCalls.Load() != 0 {
		t.Fatalf("commit=%d inspect=%d provider=%d", gate.commitCalls.Load(), gate.inspectCalls.Load(), providerCalls.Load())
	}
}

func TestM3ModelOnlyAttemptPositiveControl(t *testing.T) {
	fact := sealedFact()
	current := sealedCurrent(fact)
	ack := sealedAck(fact, current)
	gate := &gateFake{ack: ack}
	var providerCalls atomic.Uint64
	if err := runModelOnlyM3AttemptV1(context.Background(), gate, fact, current, time.Unix(0, 4_000), nil, &providerCalls); err != nil {
		t.Fatal(err)
	}
	if gate.commitCalls.Load() != 1 || gate.inspectCalls.Load() != 0 || providerCalls.Load() != 1 {
		t.Fatalf("commit=%d inspect=%d provider=%d", gate.commitCalls.Load(), gate.inspectCalls.Load(), providerCalls.Load())
	}
}

func runModelOnlyM3AttemptV1(
	ctx context.Context,
	gate modelinvoker.PreparedModelInvocationCommitGateV1,
	fact modelinvoker.PreparedModelInvocationFactV1,
	current modelinvoker.PreparedModelInvocationCurrentProjectionV1,
	now time.Time,
	mutate func(*modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1),
	providerCalls *atomic.Uint64,
) error {
	ack, err := modelinvoker.CrossPreparedModelInvocationCommitGateV1(ctx, gate, fact.Ref(), current.Ref())
	if err != nil {
		return err
	}
	draft := modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1{
		PreparedRef:                   fact.Ref(),
		CurrentRef:                    current.Ref(),
		AckRef:                        ack.Ref(),
		DispatchSequence:              1,
		BoundaryKind:                  "provider.invoke",
		ProviderAttemptOrdinal:        1,
		AttemptRequestDigest:          digest("m3-attempt-request"),
		ActualToolSurfaceDigest:       fact.ActualToolSurfaceDigest,
		ActualProviderInjectionDigest: fact.ActualProviderInjectionDigest,
		CheckedUnixNano:               now.UnixNano(),
	}
	if mutate != nil {
		mutate(&draft)
	}
	if _, err := modelinvoker.SealPreparedModelInvocationDispatchReceiptAgainstV1(fact, current, ack, draft, now); err != nil {
		return err
	}
	providerCalls.Add(1)
	return nil
}

func assertExactInterfaceMethodV1(t *testing.T, interfaceType reflect.Type, methodName string, inputType, outputType reflect.Type) {
	t.Helper()
	if interfaceType.NumMethod() != 1 {
		t.Fatalf("%s methods = %d", interfaceType, interfaceType.NumMethod())
	}
	method, ok := interfaceType.MethodByName(methodName)
	if !ok {
		t.Fatalf("%s lacks %s", interfaceType, methodName)
	}
	wantContext := reflect.TypeOf((*context.Context)(nil)).Elem()
	if method.Type.NumIn() != 2 || method.Type.In(0) != wantContext || method.Type.In(1) != inputType ||
		method.Type.NumOut() != 2 || method.Type.Out(0) != outputType || method.Type.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Fatalf("%s signature = %s", methodName, method.Type)
	}
}

func assertExactStructFieldsV1(t *testing.T, structType reflect.Type, expected []string) {
	t.Helper()
	if structType.NumField() != len(expected) {
		t.Fatalf("%s fields = %d, want %d", structType, structType.NumField(), len(expected))
	}
	for index, name := range expected {
		if field := structType.Field(index); field.Name != name {
			t.Fatalf("%s field[%d] = %s, want %s", structType, index, field.Name, name)
		}
	}
}

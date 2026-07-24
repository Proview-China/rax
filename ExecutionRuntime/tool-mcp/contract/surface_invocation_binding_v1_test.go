package contract_test

import (
	"reflect"
	"testing"
	"time"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestToolSurfaceInvocationBindingV1CanonicalSealAndModelRef(t *testing.T) {
	request := testkit.ToolSurfaceInvocationBindingRequestV1()
	subject, err := toolcontract.SealToolSurfaceInvocationBindingSubjectV1(request)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := toolcontract.SealToolSurfaceInvocationBindingV1(toolcontract.ToolSurfaceInvocationBindingV1{
		Ref: toolcontract.ToolSurfaceInvocationBindingRefV1{Owner: testkit.Owner()}, Subject: subject,
		CreatedUnixNano:  testkit.FixedTime.Add(time.Second).UnixNano(),
		NotAfterUnixNano: request.RequestedNotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	if binding.Ref.Digest != binding.Digest || binding.Ref.ModelRefV1().Digest != binding.Ref.Digest || binding.Ref.ModelRefV1().Owner != binding.Ref.Owner {
		t.Fatalf("Binding Ref is not a lossless Model neutral Ref: %+v", binding.Ref)
	}
	second, err := toolcontract.SealToolSurfaceInvocationBindingV1(binding)
	if err != nil || !reflect.DeepEqual(second, binding) {
		t.Fatalf("Binding seal is not deterministic: %+v err=%v", second, err)
	}
}

func TestToolSurfaceInvocationBindingAckV1CanonicalDigestHasNoFeedback(t *testing.T) {
	request := testkit.ToolSurfaceInvocationBindingRequestV1()
	subject, err := toolcontract.SealToolSurfaceInvocationBindingSubjectV1(request)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := toolcontract.SealToolSurfaceInvocationBindingV1(toolcontract.ToolSurfaceInvocationBindingV1{
		Ref: toolcontract.ToolSurfaceInvocationBindingRefV1{Owner: testkit.Owner()}, Subject: subject,
		CreatedUnixNano: testkit.FixedTime.Add(time.Second).UnixNano(), NotAfterUnixNano: request.RequestedNotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	ack, err := toolcontract.SealToolSurfaceInvocationBindingAckV1(toolcontract.ToolSurfaceInvocationBindingAckV1{
		BindingRef: binding.Ref, Invocation: binding.Subject.Invocation, PreparedFactRef: binding.Subject.PreparedFactRef,
		PreparedCurrentRef: binding.Subject.PreparedCurrentRef, CheckedUnixNano: binding.CreatedUnixNano, NotAfterUnixNano: binding.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ack.Ref.Digest != ack.Digest || ack.Ref.BindingRef != binding.Ref {
		t.Fatalf("Ack canonical closure drifted: %+v", ack)
	}
	tampered := ack
	tampered.Digest = testkit.Digest("tampered-ack")
	if err := tampered.Validate(); err == nil {
		t.Fatal("Ack top-level digest tamper was accepted")
	}
	tampered = ack
	tampered.Ref.Digest = testkit.Digest("tampered-ref")
	if err := tampered.Validate(); err == nil {
		t.Fatal("Ack Ref digest tamper was accepted")
	}
}

func TestToolSurfaceInvocationBindingV1TTLUsesAllOwnerUpperBounds(t *testing.T) {
	request := testkit.ToolSurfaceInvocationBindingRequestV1()
	subject, err := toolcontract.SealToolSurfaceInvocationBindingSubjectV1(request)
	if err != nil {
		t.Fatal(err)
	}
	callerDeadline := testkit.FixedTime.Add(5 * time.Minute)
	if got := toolcontract.ToolSurfaceInvocationBindingNotAfterV1(subject, callerDeadline); got != callerDeadline.UnixNano() {
		t.Fatalf("caller deadline was not part of TTL min: %d", got)
	}
	if got := toolcontract.ToolSurfaceInvocationBindingNotAfterV1(subject, time.Time{}); got != request.RequestedNotAfterUnixNano {
		t.Fatalf("requested bound was not part of TTL min: %d", got)
	}
}

func TestToolSurfaceInvocationBindingV1RejectsCreateBeforeCurrentChecks(t *testing.T) {
	request := testkit.ToolSurfaceInvocationBindingRequestV1()
	subject, err := toolcontract.SealToolSurfaceInvocationBindingSubjectV1(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := toolcontract.SealToolSurfaceInvocationBindingV1(toolcontract.ToolSurfaceInvocationBindingV1{
		Ref: toolcontract.ToolSurfaceInvocationBindingRefV1{Owner: testkit.Owner()}, Subject: subject,
		CreatedUnixNano: testkit.FixedTime.Add(-time.Nanosecond).UnixNano(), NotAfterUnixNano: request.RequestedNotAfterUnixNano,
	}); err == nil {
		t.Fatal("Binding created before current projection checks was accepted")
	}
}

func TestToolSurfaceInvocationBindingV1RejectsCrossOwnerDriftAndMixedInjection(t *testing.T) {
	base := testkit.ToolSurfaceInvocationBindingRequestV1()
	mutations := map[string]func(*toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1){
		"prepared ref": func(request *toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1) {
			request.PreparedFactRef.Digest = testkit.Digest("wrong-prepared")
		},
		"prepared current": func(request *toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1) {
			request.PreparedCurrentRef.Digest = testkit.Digest("wrong-current")
		},
		"surface ref": func(request *toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1) {
			request.SurfaceCurrent.Ref.Digest = testkit.Digest("wrong-surface")
		},
		"assembly ref": func(request *toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1) {
			request.AssemblyCurrentRef.Digest = testkit.Digest("wrong-assembly")
		},
		"registry": func(request *toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1) {
			request.AssemblyRegistrySnapshot.Digest = testkit.Digest("wrong-registry")
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			request := base
			mutate(&request)
			if _, err := toolcontract.SealToolSurfaceInvocationBindingSubjectV1(request); err == nil {
				t.Fatal("cross-owner exact drift was accepted")
			}
		})
	}
}

func TestToolSurfaceInvocationBindingV1PublicPortMethodSet(t *testing.T) {
	writer := reflect.TypeOf((*toolcontract.ToolSurfaceInvocationBindingWriterV1)(nil)).Elem()
	reader := reflect.TypeOf((*toolcontract.ToolSurfaceInvocationBindingReaderV1)(nil)).Elem()
	repository := reflect.TypeOf((*toolcontract.ToolSurfaceInvocationBindingRepositoryV1)(nil)).Elem()
	if writer.NumMethod() != 1 || reader.NumMethod() != 2 || repository.NumMethod() != 3 {
		t.Fatalf("public Surface Binding method set drifted: writer=%d reader=%d repository=%d", writer.NumMethod(), reader.NumMethod(), repository.NumMethod())
	}
	ensure, ok := writer.MethodByName("EnsureToolSurfaceInvocationBindingV1")
	if !ok || ensure.Type.NumIn() != 2 || ensure.Type.NumOut() != 3 {
		t.Fatalf("Writer signature drifted: %+v", ensure)
	}
	if _, ok := reader.MethodByName("InspectToolSurfaceInvocationBindingByInvocationV1"); !ok {
		t.Fatal("by-invocation lost-reply Reader is absent")
	}
	if _, ok := reader.MethodByName("InspectExactToolSurfaceInvocationBindingV1"); !ok {
		t.Fatal("exact Binding Reader is absent")
	}
}

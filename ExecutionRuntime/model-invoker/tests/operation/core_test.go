package operation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
)

type fakeProvider struct {
	id       modelinvoker.ProviderID
	contract operation.CapabilityContract
	invoke   func(context.Context, operation.Request) (operation.Result, error)
}

func (p *fakeProvider) ID() modelinvoker.ProviderID { return p.id }
func (p *fakeProvider) Capabilities(context.Context, operation.Query) (operation.CapabilityContract, error) {
	return p.contract, nil
}
func (p *fakeProvider) Invoke(ctx context.Context, request operation.Request) (operation.Result, error) {
	if p.invoke != nil {
		return p.invoke(ctx, request)
	}
	return operation.Result{}, nil
}
func (*fakeProvider) Stream(context.Context, operation.Request) (operation.Stream, error) {
	return nil, errors.New("unused")
}

func TestEvaluateRequiresExplicitDegradationAndExactModel(t *testing.T) {
	request := operation.Request{Provider: "p", Kind: operation.ImageGenerate, Model: "m"}
	contract := operation.CapabilityContract{operation.ImageGenerate: {
		Level: operation.SupportPartial, Models: []string{"m"}, Limitations: []string{"preview"},
	}}
	report, err := operation.Evaluate(request, contract)
	if err == nil || report.Action != operation.MappingRejected {
		t.Fatalf("partial support must fail closed: report=%+v err=%v", report, err)
	}
	request.AllowDegradation = true
	report, err = operation.Evaluate(request, contract)
	if err != nil || report.Action != operation.MappingDegraded {
		t.Fatalf("explicit degradation should pass: report=%+v err=%v", report, err)
	}
	request.Model = "other"
	if _, err = operation.Evaluate(request, contract); err == nil {
		t.Fatal("model outside exact allowlist must fail")
	}
}

func TestRegistryRejectsTypedNilAndDuplicate(t *testing.T) {
	var nilProvider *fakeProvider
	if _, err := operation.NewRegistry(nilProvider); err == nil {
		t.Fatal("typed nil provider must be rejected")
	}
	p := &fakeProvider{id: "p"}
	registry, err := operation.NewRegistry(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(p); err == nil {
		t.Fatal("duplicate provider must be rejected")
	}
}

func TestInvokerCompletesIdentityAndNormalizesTimeout(t *testing.T) {
	p := &fakeProvider{id: "p", contract: operation.CapabilityContract{
		operation.EmbeddingCreate: {Level: operation.SupportNative, Lifecycle: operation.LifecycleRequest, Models: []string{"m"}},
	}}
	p.invoke = func(ctx context.Context, _ operation.Request) (operation.Result, error) {
		<-ctx.Done()
		return operation.Result{}, ctx.Err()
	}
	registry, _ := operation.NewRegistry(p)
	invoker, _ := operation.NewInvoker(registry)
	result, err := invoker.Invoke(context.Background(), operation.Request{
		Provider: "p", Kind: operation.EmbeddingCreate, Model: "m", Budget: operation.Budget{Timeout: time.Millisecond},
	})
	var typed *modelinvoker.Error
	if !errors.As(err, &typed) || typed.Kind != modelinvoker.ErrorTimeout {
		t.Fatalf("expected normalized timeout, got %T %v", err, err)
	}
	if result.Provider != "p" || result.Kind != operation.EmbeddingCreate || result.Model != "m" || result.MappingReport.Action != operation.MappingExact {
		t.Fatalf("identity and mapping report were not completed: %+v", result)
	}
}

func TestRequestValidationRejectsUnsafeFields(t *testing.T) {
	cases := []operation.Request{
		{},
		{Provider: "p", Kind: operation.Kind("unknown")},
		{Provider: "p", Kind: operation.FileGet, ResourceID: "x\nheader"},
		{Provider: "p", Kind: operation.ImageGenerate, ContentType: "bad type"},
		{Provider: "p", Kind: operation.ImageGenerate, ContentType: "application/json", Body: modelinvoker.NewRawPayload([]byte("{"))},
	}
	for index, request := range cases {
		if err := request.Validate(); err == nil {
			t.Fatalf("case %d should fail validation", index)
		}
	}
}

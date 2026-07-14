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

func TestInvokerRejectsProviderResultIdentityAndArtifactDrift(t *testing.T) {
	provider := &fakeProvider{id: "p", contract: operation.CapabilityContract{
		operation.ImageGenerate: {Level: operation.SupportNative, Models: []string{"m"}},
	}}
	registry, _ := operation.NewRegistry(provider)
	invoker, _ := operation.NewInvoker(registry)
	request := operation.Request{Provider: "p", Kind: operation.ImageGenerate, Model: "m"}
	results := []operation.Result{
		{Provider: "other"},
		{Kind: operation.VideoGenerate},
		{Model: "other"},
		{Artifacts: []operation.Artifact{{Kind: operation.ArtifactImage, Data: []byte("x"), URL: "https://example.com/x"}}},
		{Artifacts: []operation.Artifact{{Kind: operation.ArtifactImage, URL: "https://example.com/x"}}},
	}
	for index, candidate := range results {
		provider.invoke = func(context.Context, operation.Request) (operation.Result, error) {
			return candidate, nil
		}
		if _, err := invoker.Invoke(context.Background(), request); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping {
			t.Fatalf("provider result drift %d was not rejected: %v", index, err)
		}
	}
	provider.invoke = func(context.Context, operation.Request) (operation.Result, error) {
		return operation.Result{Artifacts: []operation.Artifact{{
			Kind: operation.ArtifactImage, URL: "https://example.com/x", ExpiryUnknown: true,
		}}}, nil
	}
	if _, err := invoker.Invoke(context.Background(), request); err != nil {
		t.Fatalf("valid provider artifact was rejected: %v", err)
	}
}

func TestInvokerIsolatesCallerRequestsAndProviderResults(t *testing.T) {
	provider := &fakeProvider{id: "p", contract: operation.CapabilityContract{
		operation.ImageGenerate: {Level: operation.SupportNative, Models: []string{"m"}},
	}}
	query := map[string][]string{"tag": {"original"}}
	metadata := modelinvoker.Metadata{"trace": "original"}
	options := modelinvoker.ProviderOptions{"p": []byte(`{"mode":"original"}`)}
	providerResult := operation.Result{
		Job:              &operation.JobRef{ID: "job-original"},
		Resource:         &operation.ResourceRef{ID: "resource-original"},
		Artifacts:        []operation.Artifact{{Kind: operation.ArtifactImage, Data: []byte("image-original"), Metadata: map[string]string{"source": "original"}}},
		Vectors:          []operation.Vector{{Values: []float32{1, 2}}},
		Rankings:         []operation.Ranking{{Text: "rank-original"}},
		ProviderMetadata: modelinvoker.ProviderMetadata{"request-id": "original"},
	}
	provider.invoke = func(_ context.Context, request operation.Request) (operation.Result, error) {
		request.Query["tag"][0] = "provider-mutated"
		request.Metadata["trace"] = "provider-mutated"
		request.ProviderOptions["p"][0] = 'X'
		return providerResult, nil
	}
	registry, _ := operation.NewRegistry(provider)
	invoker, _ := operation.NewInvoker(registry)
	result, err := invoker.Invoke(context.Background(), operation.Request{
		Provider: "p", Kind: operation.ImageGenerate, Model: "m",
		Query: query, Metadata: metadata, ProviderOptions: options,
	})
	if err != nil {
		t.Fatal(err)
	}
	if query["tag"][0] != "original" || metadata["trace"] != "original" || string(options["p"]) != `{"mode":"original"}` {
		t.Fatal("operation provider mutated caller-owned request state")
	}

	providerResult.Job.ID = "mutated"
	providerResult.Resource.ID = "mutated"
	providerResult.Artifacts[0].Data[0] = 'X'
	providerResult.Artifacts[0].Metadata["source"] = "mutated"
	providerResult.Vectors[0].Values[0] = 99
	providerResult.Rankings[0].Text = "mutated"
	providerResult.ProviderMetadata["request-id"] = "mutated"
	if result.Job.ID != "job-original" || result.Resource.ID != "resource-original" ||
		string(result.Artifacts[0].Data) != "image-original" || result.Artifacts[0].Metadata["source"] != "original" ||
		result.Vectors[0].Values[0] != 1 || result.Rankings[0].Text != "rank-original" ||
		result.ProviderMetadata["request-id"] != "original" {
		t.Fatal("operation result aliases provider-owned state")
	}
}

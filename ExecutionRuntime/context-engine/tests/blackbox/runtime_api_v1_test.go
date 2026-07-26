package blackbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/runtimeapi"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type runtimeToolBundleV1 struct {
	Result     toolcontract.ToolResultV2                  `json:"result"`
	Projection toolcontract.SettledToolResultProjectionV1 `json:"projection"`
}

type runtimeToolReaderV1 struct {
	result toolcontract.ToolResultV2
	err    error
	calls  atomic.Int64
}

func (r *runtimeToolReaderV1) InspectSettledToolResultCurrentV2(ctx context.Context, _ toolcontract.ObjectRef) (toolcontract.ToolResultV2, error) {
	r.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if r.err != nil {
		return toolcontract.ToolResultV2{}, r.err
	}
	return r.result, nil
}

func runtimeAPIFixtureV1(t *testing.T) (*testfixture.FrameConsumptionFixtureV1, runtimeToolBundleV1, *runtimeToolReaderV1, runtimeapi.AppendSettledToolResultRequestV2, runtimeapi.ContextRuntimeAPIV1) {
	t.Helper()
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile("../../kernel/testdata/settled_tool_result_inline_v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var bundle runtimeToolBundleV1
	if err = json.Unmarshal(payload, &bundle); err != nil {
		t.Fatal(err)
	}
	bundle.Projection.Inspection = bundle.Result.Inspection
	bundle.Projection.Result = toolcontract.ObjectRef{ID: bundle.Result.ID, Revision: bundle.Result.Revision, Digest: bundle.Result.Digest}
	bundle.Projection.ProjectionDigest = ""
	bundle.Projection, err = toolcontract.SealSettledToolResultProjectionV1(bundle.Projection)
	if err != nil {
		t.Fatal(err)
	}
	toolReader := &runtimeToolReaderV1{result: bundle.Result}
	frameReader := &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{fixture.Snapshot, fixture.Snapshot}}
	service, err := runtimeapi.NewServiceV1(frameReader, toolReader, fixture.AwareStore)
	if err != nil {
		t.Fatal(err)
	}
	request := runtimeapi.AppendSettledToolResultRequestV2{
		Projection:                bundle.Projection,
		ParentManifest:            fixture.Result.Manifest,
		ParentFrame:               fixture.Result.Frame,
		ParentGeneration:          fixture.Generation,
		ParentGenerationExpires:   fixture.Snapshot.GenerationExpiresUnixNano,
		Recipe:                    testkit.Recipe(),
		TenantScopeDigest:         fixture.Request.TenantScopeDigest,
		AgentInstanceRef:          fixture.Request.AgentInstanceRef,
		PromptAssetRefs:           []contract.PromptAssetRefV1{},
		DisclosureClass:           contract.DisclosureInternalV1,
		PromptExpiresUnixNano:     fixture.Snapshot.PromptExpiresUnixNano,
		DisclosureExpiresUnixNano: fixture.Snapshot.DisclosureExpiresUnixNano,
		AuthorityExpiresUnixNano:  fixture.Snapshot.AuthorityExpiresUnixNano,
		IdempotencyKey:            "runtime-api-tool-result-v1",
		CheckedUnixNano:           testkit.Now,
		NotAfterUnixNano:          testkit.Now + 600,
	}
	return fixture, bundle, toolReader, request, service
}

func TestContextRuntimeAPIConsumesAndAppendsInline(t *testing.T) {
	fixture, _, _, request, api := runtimeAPIFixtureV1(t)
	parent, err := api.ConsumeFrame(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	got, err := api.AppendSettledToolResult(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Frame.StablePrefix != fixture.Result.Frame.StablePrefix || !reflect.DeepEqual(got.Frame.SemiStable, fixture.Result.Frame.SemiStable) {
		t.Fatal("stable frame regions changed")
	}
	if got.Descriptor.CacheHint.StablePrefixFingerprint != parent.CacheHint.StablePrefixFingerprint {
		t.Fatal("stable fingerprint changed")
	}
	if got.Frame.DynamicTail == fixture.Result.Frame.DynamicTail || got.Descriptor.CacheHint.FrameFingerprint == parent.CacheHint.FrameFingerprint {
		t.Fatal("dynamic result did not invalidate the frame")
	}
	last := got.Manifest.Fragments[len(got.Manifest.Fragments)-1]
	if last.Kind != contract.FragmentToolResult || last.Region != contract.RegionDynamicTail || got.Descriptor.FrameRef != got.Generation.RootFrame {
		t.Fatalf("unexpected runtime product: fragment=%+v descriptor=%+v", last, got.Descriptor)
	}
}

func TestContextRuntimeAPIArtifactReferenceOnly(t *testing.T) {
	fixture, bundle, toolReader, request, api := runtimeAPIFixtureV1(t)
	artifact := toolcontract.ObjectRef{ID: "runtime-api-artifact-v1", Revision: 1, Digest: bundle.Result.PayloadDigest}
	result := bundle.Result
	result.Artifacts = []toolcontract.ObjectRef{artifact}
	var err error
	result, err = toolcontract.SealToolResultV2(result)
	if err != nil {
		t.Fatal(err)
	}
	projection := bundle.Projection
	projection.Result = toolcontract.ObjectRef{ID: result.ID, Revision: result.Revision, Digest: result.Digest}
	projection.Inline = nil
	projection.Artifact = &artifact
	projection.ProjectionDigest = ""
	projection, err = toolcontract.SealSettledToolResultProjectionV1(projection)
	if err != nil {
		t.Fatal(err)
	}
	toolReader.result = result
	request.Projection = projection
	got, err := api.AppendSettledToolResult(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	last := got.Manifest.Fragments[len(got.Manifest.Fragments)-1]
	materialized, err := fixture.AwareStore.GetContextV1(context.Background(), last.Content)
	if err != nil {
		t.Fatal(err)
	}
	if last.Kind != contract.FragmentArtifactReference || bytes.Contains(materialized, bundle.Projection.Inline) || len(materialized) >= contract.MaxSettledToolResultInlineBytesV2 {
		t.Fatalf("artifact result materialized body: fragment=%+v bytes=%d", last, len(materialized))
	}
}

func TestContextRuntimeAPIFailClosedAndConcurrentReplay(t *testing.T) {
	fixture, _, toolReader, request, api := runtimeAPIFixtureV1(t)
	toolReader.err = contract.ErrUnknown
	got, err := api.AppendSettledToolResult(context.Background(), request)
	if !errors.Is(err, contract.ErrUnknown) || !reflect.DeepEqual(got, runtimeapi.AppendSettledToolResultResultV2{}) {
		t.Fatalf("unknown got=%+v error=%v", got, err)
	}
	toolReader.err = nil

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = api.AppendSettledToolResult(ctx, request)
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, runtimeapi.AppendSettledToolResultResultV2{}) {
		t.Fatalf("cancel got=%+v error=%v", got, err)
	}

	const workers = 64
	values := make(chan runtimeapi.AppendSettledToolResultResultV2, workers)
	errorsCh := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, callErr := api.AppendSettledToolResult(context.Background(), request)
			values <- value
			errorsCh <- callErr
		}()
	}
	wg.Wait()
	close(values)
	close(errorsCh)
	for callErr := range errorsCh {
		if callErr != nil {
			t.Fatal(callErr)
		}
	}
	var first *runtimeapi.AppendSettledToolResultResultV2
	for value := range values {
		if first == nil {
			copy := value
			first = &copy
			continue
		}
		if value.Frame.ID != first.Frame.ID || value.Descriptor.Digest != first.Descriptor.Digest || value.Generation.RootFrame != first.Generation.RootFrame {
			t.Fatal("concurrent logical winners drifted")
		}
	}
	if first == nil || first.Frame.ParentFrame == nil || *first.Frame.ParentFrame != fixture.Request.FrameRef {
		t.Fatal("missing deterministic child frame")
	}
}

func TestContextRuntimeAPIPreservesKernelFailureBoundary(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*runtimeToolReaderV1, *runtimeapi.AppendSettledToolResultRequestV2)
	}{
		{"result_drift", func(reader *runtimeToolReaderV1, _ *runtimeapi.AppendSettledToolResultRequestV2) {
			reader.result.Revision++
		}},
		{"invalid_xor", func(_ *runtimeToolReaderV1, request *runtimeapi.AppendSettledToolResultRequestV2) {
			artifact := toolcontract.ObjectRef{ID: "invalid-xor-artifact", Revision: 1, Digest: request.Projection.PayloadDigest}
			request.Projection.Artifact = &artifact
		}},
		{"expired", func(_ *runtimeToolReaderV1, request *runtimeapi.AppendSettledToolResultRequestV2) {
			request.Projection.ExpiresUnixNano = testkit.Now + 1
			request.Projection.ProjectionDigest = ""
			request.Projection, _ = toolcontract.SealSettledToolResultProjectionV1(request.Projection)
			request.CheckedUnixNano = testkit.Now + 2
			request.NotAfterUnixNano = testkit.Now + 3
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, reader, request, api := runtimeAPIFixtureV1(t)
			tc.mutate(reader, &request)
			got, err := api.AppendSettledToolResult(context.Background(), request)
			if err == nil || !reflect.DeepEqual(got, runtimeapi.AppendSettledToolResultResultV2{}) {
				t.Fatalf("got=%+v error=%v", got, err)
			}
		})
	}
}

func TestContextRuntimeAPISequentialReplayIsExact(t *testing.T) {
	_, _, _, request, api := runtimeAPIFixtureV1(t)
	first, err := api.AppendSettledToolResult(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := api.AppendSettledToolResult(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("identical request replay changed the runtime product")
	}
}

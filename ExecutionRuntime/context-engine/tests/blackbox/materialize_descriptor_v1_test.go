package blackbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/runtimeapi"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func blackboxMaterializeRequestV1(descriptor contract.ContextFrameConsumptionDescriptorV1) runtimeapi.MaterializeDescriptorRequestV1 {
	return runtimeapi.MaterializeDescriptorRequestV1{
		Descriptor: descriptor, CheckedUnixNano: descriptor.CheckedUnixNano,
		Limits: runtimeapi.MaterializeDescriptorLimitsV1{MaxItems: 4, MaxItemBytes: runtimeapi.HardMaxMaterializedItemBytesV1, MaxTotalBytes: runtimeapi.HardMaxMaterializedTotalBytesV1},
	}
}

func TestContextRuntimeAPIMaterializesConsumedAndAppendedFrames(t *testing.T) {
	fixture, _, _, appendRequest, api := runtimeAPIFixtureV1(t)
	parentDescriptor, err := api.ConsumeFrame(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := api.MaterializeDescriptor(context.Background(), blackboxMaterializeRequestV1(parentDescriptor))
	if err != nil {
		t.Fatal(err)
	}
	childProduct, err := api.AppendSettledToolResult(context.Background(), appendRequest)
	if err != nil {
		t.Fatal(err)
	}
	child, err := api.MaterializeDescriptor(context.Background(), blackboxMaterializeRequestV1(childProduct.Descriptor))
	if err != nil {
		t.Fatal(err)
	}
	if len(parent.Items) != 4 || len(child.Items) != 4 || parent.Items[0].Ref != child.Items[0].Ref || !bytes.Equal(parent.Items[0].Bytes, child.Items[0].Bytes) || parent.Items[1].Ref != child.Items[1].Ref || !bytes.Equal(parent.Items[1].Bytes, child.Items[1].Bytes) {
		t.Fatal("stable materialized regions changed")
	}
	if parent.Items[2].Ref == child.Items[2].Ref || bytes.Equal(parent.Items[2].Bytes, child.Items[2].Bytes) || parent.Items[3].Ref == child.Items[3].Ref {
		t.Fatal("dynamic materialized regions did not change")
	}
}

func TestContextRuntimeAPIMaterializesArtifactMetadataOnly(t *testing.T) {
	_, bundle, toolReader, appendRequest, api := runtimeAPIFixtureV1(t)
	artifact := toolcontract.ObjectRef{ID: "runtime-api-materialized-artifact-v1", Revision: 1, Digest: bundle.Result.PayloadDigest}
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
	appendRequest.Projection = projection
	product, err := api.AppendSettledToolResult(context.Background(), appendRequest)
	if err != nil {
		t.Fatal(err)
	}
	materialized, err := api.MaterializeDescriptor(context.Background(), blackboxMaterializeRequestV1(product.Descriptor))
	if err != nil {
		t.Fatal(err)
	}
	dynamic := materialized.Items[len(materialized.Items)-2].Bytes
	var fragments []struct {
		Content []byte `json:"content"`
	}
	if err = json.Unmarshal(dynamic, &fragments); err != nil || len(fragments) == 0 {
		t.Fatalf("decode dynamic fragments: %v", err)
	}
	metadata := fragments[len(fragments)-1].Content
	if !bytes.Contains(metadata, []byte(artifact.ID)) || bytes.Contains(metadata, bundle.Projection.Inline) || uint64(len(dynamic)) >= runtimeapi.HardMaxMaterializedItemBytesV1 {
		t.Fatal("artifact body was followed or materialized")
	}
}

func TestContextRuntimeAPIMaterializeConcurrentResultsDoNotAlias(t *testing.T) {
	fixture, _, _, _, api := runtimeAPIFixtureV1(t)
	descriptor, err := api.ConsumeFrame(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	request := blackboxMaterializeRequestV1(descriptor)
	const workers = 64
	values := make(chan runtimeapi.MaterializeDescriptorResultV1, workers)
	errorsCh := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, callErr := api.MaterializeDescriptor(context.Background(), request)
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
	results := make([]runtimeapi.MaterializeDescriptorResultV1, 0, workers)
	for value := range values {
		results = append(results, value)
	}
	if len(results) != workers {
		t.Fatalf("results=%d", len(results))
	}
	for index := 1; index < len(results); index++ {
		if results[index].Digest != results[0].Digest || !reflect.DeepEqual(results[index].Items, results[0].Items) {
			t.Fatal("concurrent exact materialization drifted")
		}
	}
	original := results[1].Items[0].Bytes[0]
	results[0].Items[0].Bytes[0] ^= 0xff
	if results[1].Items[0].Bytes[0] != original {
		t.Fatal("concurrent results alias")
	}
}

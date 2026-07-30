package runtimeapi_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/refstore"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/runtimeapi"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type countingToolReaderV1 struct {
	calls atomic.Int64
}

func (r *countingToolReaderV1) InspectSettledToolResultCurrentV2(context.Context, toolcontract.ObjectRef) (toolcontract.ToolResultV2, error) {
	r.calls.Add(1)
	return toolcontract.ToolResultV2{}, contract.ErrUnavailable
}

type modelInputFixtureV1 struct {
	frame   *testfixture.FrameConsumptionFixtureV1
	reader  *testfixture.FrameConsumptionReaderV1
	tools   *countingToolReaderV1
	service *runtimeapi.ServiceV1
	desc    contract.ContextFrameConsumptionDescriptorV1
	binds   []contract.ContextModelInputSegmentBindingV1
}

func newModelInputFixtureV1(t *testing.T) modelInputFixtureV1 {
	t.Helper()
	store := refstore.NewMemory()
	instruction, err := store.Put([]byte("exact system instruction"))
	if err != nil {
		t.Fatal(err)
	}
	resultContent, err := store.Put([]byte("workspace read output"))
	if err != nil {
		t.Fatal(err)
	}
	recipe := testkit.Recipe()
	recipe.Rules = []contract.FragmentRule{
		{Kind: contract.FragmentInstruction, Region: contract.RegionStablePrefix, Required: true, MaxTokens: 100, Degradation: contract.DegradeReject},
		{Kind: contract.FragmentToolResult, Region: contract.RegionDynamicTail, Required: true, MaxTokens: 100, Degradation: contract.DegradeReject},
	}
	compiled, err := kernel.Compile(store, kernel.CompileRequest{
		AttemptID: "model-input-attempt", ManifestID: "model-input-manifest", FrameID: "model-input-frame", GenerationID: "model-input-generation", Generation: 1,
		Recipe: recipe, Execution: testkit.Execution(),
		Candidates: []contract.ContextCandidate{
			testkit.Candidate("model-input-instruction", contract.FragmentInstruction, instruction, 20),
			testkit.Candidate("model-input-tool-result", contract.FragmentToolResult, resultContent, 20),
		},
		CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 1_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	frameDigest, err := compiled.Frame.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest, err := compiled.Manifest.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	frameRef := contract.FactRef{ID: compiled.Frame.ID, Revision: compiled.Frame.Revision, Digest: frameDigest}
	manifestRef := contract.FactRef{ID: compiled.Manifest.ID, Revision: compiled.Manifest.Revision, Digest: manifestDigest}
	generation := contract.ContextGeneration{
		ContractVersion: contract.Version, ID: compiled.Frame.GenerationID, Revision: 1, Ordinal: 1, RootFrame: frameRef,
		RetainedAnchors: []contract.FactRef{}, OpenEffects: []contract.FactRef{}, CreatedUnixNano: testkit.Now,
	}
	generationDigest, err := contract.DigestJSON(generation)
	if err != nil {
		t.Fatal(err)
	}
	request, err := contract.SealContextFrameConsumptionRequestV1(contract.ContextFrameConsumptionRequestV1{
		DescriptorID: "model-input-descriptor", FrameRef: frameRef, ManifestRef: manifestRef,
		GenerationRef:     contract.FactRef{ID: generation.ID, Revision: generation.Revision, Digest: generationDigest},
		TenantScopeDigest: testkit.D("model-input-tenant"), AgentInstanceRef: contract.FactRef{ID: "model-input-agent", Revision: 1, Digest: testkit.D("model-input-agent")},
		RunID: testkit.Execution().RunID, RunScopeDigest: testkit.Execution().ScopeDigest,
		PromptAssetRefs: []contract.PromptAssetRefV1{}, RecipeRef: compiled.Manifest.RecipeRef, DisclosureClass: contract.DisclosureInternalV1,
		CheckedUnixNano: testkit.Now, NotAfterUnixNano: testkit.Now + 900,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := kernel.FrameConsumptionCurrentSnapshotV1{
		Manifest: compiled.Manifest, Frame: compiled.Frame, Generation: generation,
		GenerationExpiresUnixNano: testkit.Now + 850, FragmentSourceExpires: []int64{testkit.Now + 800, testkit.Now + 750},
		PromptExpiresUnixNano: testkit.Now + 900, RecipeExpiresUnixNano: testkit.Now + 850,
		DisclosureExpiresUnixNano: testkit.Now + 700, AuthorityExpiresUnixNano: testkit.Now + 650,
	}
	frame := &testfixture.FrameConsumptionFixtureV1{
		Store: store, AwareStore: testfixture.ContextAwareRefStoreV1{Store: store}, Result: compiled, Generation: generation, Snapshot: snapshot, Request: request,
	}
	reader := &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{snapshot}}
	tools := &countingToolReaderV1{}
	service, err := runtimeapi.NewServiceV1(reader, tools, frame.AwareStore)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := service.ConsumeFrame(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	bindings := make([]contract.ContextModelInputSegmentBindingV1, 2)
	bindings[0], err = contract.SealContextModelInputSegmentBindingV1(contract.ContextModelInputSegmentBindingV1{
		FragmentRef: compiled.Manifest.Fragments[0].CandidateRef, Region: compiled.Manifest.Fragments[0].Region, Position: 1,
		Kind: contract.FragmentInstruction, Trust: contract.TrustAuthoritativeInstruction, Channel: contract.ContextModelInputInstructionV1,
		Role: contract.ContextModelInputRoleSystemV1, Encoding: contract.ContextModelInputUTF8V1,
	})
	if err != nil {
		t.Fatal(err)
	}
	bindings[1], err = contract.SealContextModelInputSegmentBindingV1(contract.ContextModelInputSegmentBindingV1{
		FragmentRef: compiled.Manifest.Fragments[1].CandidateRef, Region: compiled.Manifest.Fragments[1].Region, Position: 2,
		Kind: contract.FragmentToolResult, Trust: contract.TrustObservation, Channel: contract.ContextModelInputFunctionResultV1,
		Role: contract.ContextModelInputRoleToolV1, Encoding: contract.ContextModelInputUTF8V1, CallID: "call-workspace-read-1", Name: "workspace.read",
	})
	if err != nil {
		t.Fatal(err)
	}
	return modelInputFixtureV1{frame: frame, reader: reader, tools: tools, service: service, desc: descriptor, binds: bindings}
}

func modelInputRequestV1(f modelInputFixtureV1) runtimeapi.MaterializeModelInputRequestV1 {
	return runtimeapi.MaterializeModelInputRequestV1{
		MaterialID: "model-input-material", MaterialRevision: 1, Consumption: f.frame.Request, Descriptor: f.desc,
		SegmentBindings: append([]contract.ContextModelInputSegmentBindingV1(nil), f.binds...), CheckedUnixNano: testkit.Now + 1,
		Limits: runtimeapi.MaterializeModelInputLimitsV1{
			MaxSegments: 8, MaxSegmentBytes: runtimeapi.HardMaxModelInputSegmentBytesV1, MaxTotalBytes: runtimeapi.HardMaxModelInputTotalBytesV1,
		},
	}
}

func TestMaterializeModelInputExactFunctionResultAndZeroToolCallsV1(t *testing.T) {
	fixture := newModelInputFixtureV1(t)
	material, err := fixture.service.MaterializeModelInputV1(context.Background(), modelInputRequestV1(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if err = material.Validate(); err != nil {
		t.Fatal(err)
	}
	result := material.OrderedSegments[1]
	if result.Channel != contract.ContextModelInputFunctionResultV1 || result.Role != contract.ContextModelInputRoleToolV1 || result.CallID != "call-workspace-read-1" || result.Name != "workspace.read" {
		t.Fatalf("function result semantic fields drifted: %+v", result)
	}
	if material.DescriptorRef.Digest != fixture.desc.Digest || material.FrameRef != fixture.desc.FrameRef || material.ManifestRef != fixture.desc.ManifestRef || material.GenerationRef != fixture.desc.GenerationRef || fixture.tools.calls.Load() != 0 {
		t.Fatalf("exact closure or zero-tool boundary failed: material=%+v tool_calls=%d", material, fixture.tools.calls.Load())
	}
}

func TestMaterializeModelInputSemanticAndExactDriftFailClosedV1(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*runtimeapi.MaterializeModelInputRequestV1)
		want   error
	}{
		{"missing_call_id", func(r *runtimeapi.MaterializeModelInputRequestV1) { r.SegmentBindings[1].CallID = "" }, contract.ErrConflict},
		{"missing_name", func(r *runtimeapi.MaterializeModelInputRequestV1) { r.SegmentBindings[1].Name = "" }, contract.ErrConflict},
		{"name_tamper", func(r *runtimeapi.MaterializeModelInputRequestV1) { r.SegmentBindings[1].Name = "workspace.write" }, contract.ErrConflict},
		{"role_ambiguity", func(r *runtimeapi.MaterializeModelInputRequestV1) {
			r.SegmentBindings[1].Role = contract.ContextModelInputRoleAssistantV1
		}, contract.ErrConflict},
		{"descriptor_digest", func(r *runtimeapi.MaterializeModelInputRequestV1) { r.Descriptor.Digest = testkit.D("wrong") }, contract.ErrInvalid},
		{"frame_ref", func(r *runtimeapi.MaterializeModelInputRequestV1) {
			r.Descriptor.FrameRef.Digest = testkit.D("wrong")
			sealed, err := contract.SealContextFrameConsumptionDescriptorV1(r.Descriptor)
			if err != nil {
				panic(err)
			}
			r.Descriptor = sealed
		}, contract.ErrConflict},
		{"expired_at_boundary", func(r *runtimeapi.MaterializeModelInputRequestV1) { r.CheckedUnixNano = r.Descriptor.ExpiresUnixNano }, contract.ErrExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newModelInputFixtureV1(t)
			request := modelInputRequestV1(fixture)
			tc.mutate(&request)
			got, err := fixture.service.MaterializeModelInputV1(context.Background(), request)
			if !errors.Is(err, tc.want) || got.Ref.ID != "" || fixture.tools.calls.Load() != 0 {
				t.Fatalf("got=%+v err=%v tool_calls=%d", got, err, fixture.tools.calls.Load())
			}
		})
	}
}

func TestMaterializeModelInputS2DriftFailsClosedV1(t *testing.T) {
	fixture := newModelInputFixtureV1(t)
	stable := fixture.frame.Snapshot
	drift := stable
	drift.FragmentSourceExpires = append([]int64(nil), stable.FragmentSourceExpires...)
	drift.FragmentSourceExpires[1]--
	fixture.reader.Snapshots = []kernel.FrameConsumptionCurrentSnapshotV1{stable, stable, stable, stable, stable, drift}
	fixture.reader.Calls = 2
	got, err := fixture.service.MaterializeModelInputV1(context.Background(), modelInputRequestV1(fixture))
	if !errors.Is(err, contract.ErrConflict) || got.Ref.ID != "" || fixture.tools.calls.Load() != 0 {
		t.Fatalf("got=%+v err=%v tool_calls=%d", got, err, fixture.tools.calls.Load())
	}
}

func TestMaterializeModelInputS1TTLDriftFailsClosedV1(t *testing.T) {
	fixture := newModelInputFixtureV1(t)
	stable := fixture.frame.Snapshot
	drift := stable
	drift.AuthorityExpiresUnixNano--
	fixture.reader.Snapshots = []kernel.FrameConsumptionCurrentSnapshotV1{stable, stable, stable, stable, drift, drift}
	fixture.reader.Calls = 2
	got, err := fixture.service.MaterializeModelInputV1(context.Background(), modelInputRequestV1(fixture))
	if !errors.Is(err, contract.ErrConflict) || got.Ref.ID != "" || fixture.tools.calls.Load() != 0 {
		t.Fatalf("got=%+v err=%v tool_calls=%d", got, err, fixture.tools.calls.Load())
	}
}

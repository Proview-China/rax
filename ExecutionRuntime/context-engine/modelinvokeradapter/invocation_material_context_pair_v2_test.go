package modelinvokeradapter

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestInvocationMaterialContextPairAdapterV2LowersStrictSubsetExactly(t *testing.T) {
	fixture, materials, frames, adapter, frame, material, expected := contextPairAdapterFixtureV2(t)
	instructions, input, mapped, err := lowerContextModelInputBodyV2(fixture.Material)
	if err != nil {
		t.Fatal(err)
	}
	if len(instructions) != 2 ||
		instructions[0].Role != modelinvoker.RoleSystem ||
		instructions[0].Text != "follow the exact workspace policy" ||
		instructions[1].Role != modelinvoker.RoleDeveloper ||
		instructions[1].Text != "return bounded evidence" ||
		len(input) != 2 ||
		input[0].Message == nil ||
		input[0].Message.Role != modelinvoker.RoleUser ||
		input[0].Message.Text != "inspect README.md" ||
		input[1].FunctionCall == nil ||
		input[1].FunctionCall.ID != "call-workspace-read-1" ||
		input[1].FunctionCall.Name != "workspace.read" ||
		string(input[1].FunctionCall.Arguments) != `{"path":"README.md"}` ||
		mapped != expected {
		t.Fatalf("lossless lowering drifted: instructions=%+v input=%+v mapped=%q", instructions, input, mapped)
	}
	projection, err := adapter.InspectExactInvocationContextPairV2(
		context.Background(), frame, material, expected,
	)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ContextFrame != frame || projection.ContextMaterial != material ||
		projection.ContextMappedInputDigest != expected ||
		projection.ProjectionDigest == expected ||
		projection.ProjectionDigest == core.Digest(fixture.Material.Digest) ||
		projection.ProjectionDigest == core.Digest(fixture.Frame.Digest) {
		t.Fatalf("pair projection domains or coordinates drifted: %+v", projection)
	}
	if exact, current := materials.CallsV1(); exact != 4 || current != 4 || frames.CallsV1() != 4 {
		t.Fatalf("adapter did not execute two complete source reads: exact=%d current=%d frame=%d", exact, current, frames.CallsV1())
	}
}

func TestInvocationMaterialContextPairAdapterV2RejectsUnsupportedLoweringV2(t *testing.T) {
	fixture, err := testfixture.NewModelInputSourceFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		channel  contract.ContextModelInputChannelV1
		role     contract.ContextModelInputRoleV1
		encoding contract.ContextModelInputEncodingV1
		content  string
		callID   string
		toolName string
	}{
		{"instruction_canonical_json", contract.ContextModelInputInstructionV1, contract.ContextModelInputRoleSystemV1, contract.ContextModelInputCanonicalJSONV1, `{"text":"system"}`, "", ""},
		{"instruction_artifact_ref", contract.ContextModelInputInstructionV1, contract.ContextModelInputRoleDeveloperV1, contract.ContextModelInputArtifactRefJSONV1, `{"artifact":"a"}`, "", ""},
		{"instruction_blank", contract.ContextModelInputInstructionV1, contract.ContextModelInputRoleSystemV1, contract.ContextModelInputUTF8V1, " \t\n", "", ""},
		{"message_canonical_json", contract.ContextModelInputMessageV1, contract.ContextModelInputRoleUserV1, contract.ContextModelInputCanonicalJSONV1, `{"text":"user"}`, "", ""},
		{"message_artifact_ref", contract.ContextModelInputMessageV1, contract.ContextModelInputRoleAssistantV1, contract.ContextModelInputArtifactRefJSONV1, `{"artifact":"a"}`, "", ""},
		{"message_blank", contract.ContextModelInputMessageV1, contract.ContextModelInputRoleUserV1, contract.ContextModelInputUTF8V1, " \t\n", "", ""},
		{"function_call_utf8", contract.ContextModelInputFunctionCallV1, contract.ContextModelInputRoleAssistantV1, contract.ContextModelInputUTF8V1, `{"path":"README.md"}`, "call-1", "workspace.read"},
		{"function_call_array", contract.ContextModelInputFunctionCallV1, contract.ContextModelInputRoleAssistantV1, contract.ContextModelInputCanonicalJSONV1, `["README.md"]`, "call-1", "workspace.read"},
		{"function_call_artifact", contract.ContextModelInputFunctionCallV1, contract.ContextModelInputRoleAssistantV1, contract.ContextModelInputArtifactRefJSONV1, `{"artifact":"a"}`, "call-1", "workspace.read"},
		{"function_result", contract.ContextModelInputFunctionResultV1, contract.ContextModelInputRoleToolV1, contract.ContextModelInputUTF8V1, "result", "call-1", "workspace.read"},
		{"reference_utf8", contract.ContextModelInputReferenceV1, contract.ContextModelInputRoleUserV1, contract.ContextModelInputUTF8V1, "reference", "", ""},
		{"reference_json", contract.ContextModelInputReferenceV1, contract.ContextModelInputRoleAssistantV1, contract.ContextModelInputCanonicalJSONV1, `{"ref":"a"}`, "", ""},
		{"reference_artifact", contract.ContextModelInputReferenceV1, contract.ContextModelInputRoleToolV1, contract.ContextModelInputArtifactRefJSONV1, `{"artifact":"a"}`, "", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := singleSegmentMaterialV2(
				t, fixture.Material, test.channel, test.role, test.encoding,
				test.content, test.callID, test.toolName,
			)
			if _, _, digest, lowerErr := lowerContextModelInputBodyV2(value); !errors.Is(lowerErr, contract.ErrConflict) || digest != "" {
				t.Fatalf("unsupported lowering accepted: digest=%q err=%v", digest, lowerErr)
			}
			_, _, _, adapter, frame, material, expected := contextPairAdapterForMaterialV2(t, fixture, value)
			projection, inspectErr := adapter.InspectExactInvocationContextPairV2(
				context.Background(), frame, material, expected,
			)
			if !errors.Is(inspectErr, contract.ErrConflict) ||
				projection != (modelinvoker.InvocationMaterialContextPairProjectionV2{}) {
				t.Fatalf("unsupported production material accepted: projection=%+v err=%v", projection, inspectErr)
			}
		})
	}
}

func TestInvocationMaterialContextPairAdapterV2RejectsOwnerKindRefDigestAxesV2(t *testing.T) {
	_, _, _, adapter, frame, material, expected := contextPairAdapterFixtureV2(t)
	otherOwner := frame.Owner
	otherOwner.ID = "other-context-owner"
	tests := []struct {
		name   string
		mutate func(*modelinvoker.InvocationMaterialExactSourceRefV1, *modelinvoker.InvocationMaterialExactSourceRefV1, *core.Digest)
	}{
		{"frame_owner", func(f, _ *modelinvoker.InvocationMaterialExactSourceRefV1, _ *core.Digest) { f.Owner = otherOwner }},
		{"material_owner", func(_, m *modelinvoker.InvocationMaterialExactSourceRefV1, _ *core.Digest) { m.Owner = otherOwner }},
		{"frame_kind", func(f, _ *modelinvoker.InvocationMaterialExactSourceRefV1, _ *core.Digest) {
			f.Kind = "caller/arbitrary"
		}},
		{"material_kind", func(_, m *modelinvoker.InvocationMaterialExactSourceRefV1, _ *core.Digest) {
			m.Kind = "caller/arbitrary"
		}},
		{"frame_id", func(f, _ *modelinvoker.InvocationMaterialExactSourceRefV1, _ *core.Digest) { f.ID += "-other" }},
		{"material_id", func(_, m *modelinvoker.InvocationMaterialExactSourceRefV1, _ *core.Digest) { m.ID += "-other" }},
		{"frame_revision", func(f, _ *modelinvoker.InvocationMaterialExactSourceRefV1, _ *core.Digest) { f.Revision++ }},
		{"material_revision", func(_, m *modelinvoker.InvocationMaterialExactSourceRefV1, _ *core.Digest) { m.Revision++ }},
		{"frame_digest", func(f, _ *modelinvoker.InvocationMaterialExactSourceRefV1, _ *core.Digest) {
			f.Digest = core.DigestBytes([]byte("other-frame"))
		}},
		{"material_digest", func(_, m *modelinvoker.InvocationMaterialExactSourceRefV1, _ *core.Digest) {
			m.Digest = core.DigestBytes([]byte("other-material"))
		}},
		{"mapped_digest", func(_, _ *modelinvoker.InvocationMaterialExactSourceRefV1, d *core.Digest) {
			*d = core.DigestBytes([]byte("other-mapped"))
		}},
		{"material_frame_swap", func(f, m *modelinvoker.InvocationMaterialExactSourceRefV1, _ *core.Digest) { *f, *m = *m, *f }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f, m, d := frame, material, expected
			test.mutate(&f, &m, &d)
			projection, err := adapter.InspectExactInvocationContextPairV2(context.Background(), f, m, d)
			if err == nil || projection != (modelinvoker.InvocationMaterialContextPairProjectionV2{}) {
				t.Fatalf("spliced axis accepted: projection=%+v err=%v", projection, err)
			}
		})
	}
}

func TestInvocationMaterialContextPairAdapterV2RejectsInvalidJSONRoleAndSemanticSpliceV2(t *testing.T) {
	fixture, err := testfixture.NewModelInputSourceFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*contract.ContextModelInputMaterialV1)
	}{
		{"duplicate_json", func(m *contract.ContextModelInputMaterialV1) {
			segment := &m.OrderedSegments[3]
			segment.Content = []byte(`{"path":"README.md","path":"other"}`)
			segment.ContentRef.Digest = contract.DigestBytes(segment.Content)
			segment.ContentRef.Length = uint64(len(segment.Content))
		}},
		{"noncanonical_json", func(m *contract.ContextModelInputMaterialV1) {
			segment := &m.OrderedSegments[3]
			segment.Content = []byte(`{"z":1,"a":2}`)
			segment.ContentRef.Digest = contract.DigestBytes(segment.Content)
			segment.ContentRef.Length = uint64(len(segment.Content))
		}},
		{"role_splice", func(m *contract.ContextModelInputMaterialV1) {
			m.OrderedSegments[2].Role = contract.ContextModelInputRoleSystemV1
		}},
		{"call_id_splice", func(m *contract.ContextModelInputMaterialV1) {
			m.OrderedSegments[3].CallID = "call-other"
		}},
		{"name_splice", func(m *contract.ContextModelInputMaterialV1) {
			m.OrderedSegments[3].Name = "workspace.write"
		}},
		{"semantic_digest_splice", func(m *contract.ContextModelInputMaterialV1) {
			m.OrderedSegments[3].SemanticBindingDigest = contract.DigestBytes([]byte("other-binding"))
		}},
		{"content_digest_splice", func(m *contract.ContextModelInputMaterialV1) {
			m.OrderedSegments[3].ContentRef.Digest = contract.DigestBytes([]byte("other-content"))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			material := fixture.Material.Clone()
			test.mutate(&material)
			if _, _, digest, lowerErr := lowerContextModelInputBodyV2(material); !errors.Is(lowerErr, contract.ErrInvalid) || digest != "" {
				t.Fatalf("invalid Material splice accepted: digest=%q err=%v", digest, lowerErr)
			}
		})
	}
}

func TestInvocationMaterialContextPairAdapterV2TypedNilUnavailableUnknownCancelTTLAndRollbackV2(t *testing.T) {
	fixture, err := testfixture.NewModelInputSourceFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	var typedNil *failingContextSourceV2
	if adapter, err := NewInvocationMaterialContextPairAdapterV2(
		fixture.Owner, typedNil, func() time.Time { return fixture.Now }, time.Second,
	); err == nil || adapter != nil {
		t.Fatal("typed-nil source accepted")
	}
	for _, test := range []struct {
		name string
		err  error
	}{
		{"unavailable", contract.ErrUnavailable},
		{"unknown", contract.ErrUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := &failingContextSourceV2{owner: fixture.Owner, err: test.err}
			adapter, err := NewInvocationMaterialContextPairAdapterV2(
				fixture.Owner, source, func() time.Time { return fixture.Now }, time.Second,
			)
			if err != nil {
				t.Fatal(err)
			}
			frame, material := modelContextSourcesV2(t, fixture)
			projection, inspectErr := adapter.InspectExactInvocationContextPairV2(
				context.Background(), frame, material, core.DigestBytes([]byte("expected")),
			)
			if !errors.Is(inspectErr, test.err) ||
				projection != (modelinvoker.InvocationMaterialContextPairProjectionV2{}) {
				t.Fatalf("projection=%+v err=%v", projection, inspectErr)
			}
		})
	}
	t.Run("cancel", func(t *testing.T) {
		_, _, _, adapter, frame, material, expected := contextPairAdapterFixtureV2(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		projection, err := adapter.InspectExactInvocationContextPairV2(ctx, frame, material, expected)
		if !errors.Is(err, context.Canceled) ||
			projection != (modelinvoker.InvocationMaterialContextPairProjectionV2{}) {
			t.Fatalf("projection=%+v err=%v", projection, err)
		}
	})
	for _, test := range []struct {
		name  string
		clock func(time.Time) func() time.Time
		want  error
	}{
		{
			"ttl_crossing",
			func(now time.Time) func() time.Time {
				var calls atomic.Int32
				return func() time.Time {
					if calls.Add(1) < 3 {
						return now
					}
					return now.Add(time.Second)
				}
			},
			contract.ErrExpired,
		},
		{
			"clock_rollback",
			func(now time.Time) func() time.Time {
				var calls atomic.Int32
				return func() time.Time {
					if calls.Add(1) == 1 {
						return now
					}
					return now.Add(-time.Nanosecond)
				}
			},
			contract.ErrConflict,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, _, _, _, frame, material, expected := contextPairAdapterFixtureV2(t)
			materials := testfixture.NewModelInputLineageMaterialReaderV1(fixture.Material)
			frames := testfixture.NewModelInputLineageFrameReaderV1(fixture.Owner, fixture.Frame)
			source, err := kernel.NewContextModelInputSourceCurrentReaderV1(
				fixture.Owner, materials, materials, frames,
				func() time.Time { return fixture.Now }, 30*time.Second,
			)
			if err != nil {
				t.Fatal(err)
			}
			adapter, err := NewInvocationMaterialContextPairAdapterV2(
				fixture.Owner, source, test.clock(fixture.Now), time.Second,
			)
			if err != nil {
				t.Fatal(err)
			}
			projection, inspectErr := adapter.InspectExactInvocationContextPairV2(
				context.Background(), frame, material, expected,
			)
			if !errors.Is(inspectErr, test.want) ||
				projection != (modelinvoker.InvocationMaterialContextPairProjectionV2{}) {
				t.Fatalf("projection=%+v err=%v", projection, inspectErr)
			}
		})
	}
}

func TestInvocationMaterialContextPairAdapterV264ConcurrentStableAndFlipWindowsV2(t *testing.T) {
	t.Run("stable", func(t *testing.T) {
		_, _, _, adapter, frame, material, expected := contextPairAdapterFixtureV2(t)
		var wait sync.WaitGroup
		failures := make(chan error, 64)
		digests := make(chan core.Digest, 64)
		for index := 0; index < 64; index++ {
			wait.Add(1)
			go func() {
				defer wait.Done()
				projection, err := adapter.InspectExactInvocationContextPairV2(
					context.Background(), frame, material, expected,
				)
				if err != nil {
					failures <- err
					return
				}
				digests <- projection.ProjectionDigest
			}()
		}
		wait.Wait()
		close(failures)
		close(digests)
		for err := range failures {
			t.Fatal(err)
		}
		var expectedProjection core.Digest
		for digest := range digests {
			if expectedProjection == "" {
				expectedProjection = digest
			}
			if digest != expectedProjection {
				t.Fatalf("stable projection drifted: got=%q want=%q", digest, expectedProjection)
			}
		}
	})
	t.Run("flip", func(t *testing.T) {
		fixture, err := testfixture.NewModelInputSourceFixtureV1()
		if err != nil {
			t.Fatal(err)
		}
		next := fixture.Material.Clone()
		next.Ref.Revision++
		next.Ref.Digest = ""
		next.Digest = ""
		next, err = contract.SealContextModelInputMaterialV1(next)
		if err != nil {
			t.Fatal(err)
		}
		materials := testfixture.NewModelInputLineageMaterialReaderV1(fixture.Material)
		materials.CurrentSequence = []contract.ContextModelInputMaterialV1{fixture.Material, next}
		frames := testfixture.NewModelInputLineageFrameReaderV1(fixture.Owner, fixture.Frame)
		source, err := kernel.NewContextModelInputSourceCurrentReaderV1(
			fixture.Owner, materials, materials, frames,
			func() time.Time { return fixture.Now }, 30*time.Second,
		)
		if err != nil {
			t.Fatal(err)
		}
		adapter, err := NewInvocationMaterialContextPairAdapterV2(
			fixture.Owner, source, func() time.Time { return fixture.Now }, 30*time.Second,
		)
		if err != nil {
			t.Fatal(err)
		}
		frame, material := modelContextSourcesV2(t, fixture)
		_, _, expected, err := lowerContextModelInputBodyV2(fixture.Material)
		if err != nil {
			t.Fatal(err)
		}
		var success atomic.Int32
		var wait sync.WaitGroup
		for index := 0; index < 64; index++ {
			wait.Add(1)
			go func() {
				defer wait.Done()
				projection, inspectErr := adapter.InspectExactInvocationContextPairV2(
					context.Background(), frame, material, expected,
				)
				if inspectErr == nil || projection != (modelinvoker.InvocationMaterialContextPairProjectionV2{}) {
					success.Add(1)
				}
			}()
		}
		wait.Wait()
		if success.Load() != 0 {
			t.Fatalf("flip window authorized %d projections", success.Load())
		}
	})
}

type failingContextSourceV2 struct {
	owner contract.OwnerRef
	err   error
}

var _ contextports.ContextModelInputSourceCurrentReaderV1 = (*failingContextSourceV2)(nil)

func (r *failingContextSourceV2) ContextOwnerRefV1() contract.OwnerRef {
	if r == nil {
		return contract.OwnerRef{}
	}
	return r.owner
}

func (r *failingContextSourceV2) InspectContextModelInputSourceCurrentV1(
	context.Context,
	contract.ContextModelInputSourceCurrentRequestV1,
) (contract.ContextModelInputSourceCurrentProjectionV1, error) {
	return contract.ContextModelInputSourceCurrentProjectionV1{}, r.err
}

func contextPairAdapterFixtureV2(
	t *testing.T,
) (
	testfixture.ModelInputSourceFixtureV1,
	*testfixture.ModelInputLineageMaterialReaderV1,
	*testfixture.ModelInputLineageFrameReaderV1,
	*InvocationMaterialContextPairAdapterV2,
	modelinvoker.InvocationMaterialExactSourceRefV1,
	modelinvoker.InvocationMaterialExactSourceRefV1,
	core.Digest,
) {
	t.Helper()
	fixture, err := testfixture.NewModelInputSourceFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	return contextPairAdapterForMaterialV2(t, fixture, fixture.Material)
}

func contextPairAdapterForMaterialV2(
	t *testing.T,
	fixture testfixture.ModelInputSourceFixtureV1,
	materialValue contract.ContextModelInputMaterialV1,
) (
	testfixture.ModelInputSourceFixtureV1,
	*testfixture.ModelInputLineageMaterialReaderV1,
	*testfixture.ModelInputLineageFrameReaderV1,
	*InvocationMaterialContextPairAdapterV2,
	modelinvoker.InvocationMaterialExactSourceRefV1,
	modelinvoker.InvocationMaterialExactSourceRefV1,
	core.Digest,
) {
	t.Helper()
	fixture.Material = materialValue
	materialSource, err := contract.ContextModelInputMaterialExactSourceV1(fixture.Owner, materialValue.Ref)
	if err != nil {
		t.Fatal(err)
	}
	frameSource, err := contract.ContextFrameExactSourceV1(fixture.Owner, materialValue.FrameRef)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Request, err = contract.SealContextModelInputSourceCurrentRequestV1(
		contract.ContextModelInputSourceCurrentRequestV1{
			Owner: fixture.Owner, Material: materialSource, Frame: frameSource,
			CheckedUnixNano:  fixture.Now.UnixNano(),
			NotAfterUnixNano: fixture.Now.Add(20 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	materials := testfixture.NewModelInputLineageMaterialReaderV1(materialValue)
	frames := testfixture.NewModelInputLineageFrameReaderV1(fixture.Owner, fixture.Frame)
	source, err := kernel.NewContextModelInputSourceCurrentReaderV1(
		fixture.Owner, materials, materials, frames,
		func() time.Time { return fixture.Now }, 30*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewInvocationMaterialContextPairAdapterV2(
		fixture.Owner, source, func() time.Time { return fixture.Now }, 30*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	frame, material := modelContextSourcesV2(t, fixture)
	_, _, expected, lowerErr := lowerContextModelInputBodyV2(materialValue)
	if lowerErr != nil {
		expected = core.DigestBytes([]byte("unsupported-lowering"))
	}
	return fixture, materials, frames, adapter, frame, material, expected
}

func modelContextSourcesV2(
	t *testing.T,
	fixture testfixture.ModelInputSourceFixtureV1,
) (modelinvoker.InvocationMaterialExactSourceRefV1, modelinvoker.InvocationMaterialExactSourceRefV1) {
	t.Helper()
	contextOwner := modelinvoker.ContextOwnerRef{
		ComponentID: fixture.Owner.ComponentID, BindingDigest: core.Digest(fixture.Owner.BindingDigest),
	}
	_, neutral, err := modelinvoker.MapContextOwnerRefToNeutralOwnerV1(contextOwner)
	if err != nil {
		t.Fatal(err)
	}
	frame := modelinvoker.InvocationMaterialExactSourceRefV1{
		Owner: neutral, Kind: modelinvoker.InvocationMaterialContextFrameKindV2,
		ID: fixture.Material.FrameRef.ID, Revision: core.Revision(fixture.Material.FrameRef.Revision),
		Digest: core.Digest(fixture.Material.FrameRef.Digest),
	}
	material := modelinvoker.InvocationMaterialExactSourceRefV1{
		Owner: neutral, Kind: modelinvoker.InvocationMaterialContextMaterialKindV2,
		ID: fixture.Material.Ref.ID, Revision: core.Revision(fixture.Material.Ref.Revision),
		Digest: core.Digest(fixture.Material.Ref.Digest),
	}
	return frame, material
}

func singleSegmentMaterialV2(
	t *testing.T,
	base contract.ContextModelInputMaterialV1,
	channel contract.ContextModelInputChannelV1,
	role contract.ContextModelInputRoleV1,
	encoding contract.ContextModelInputEncodingV1,
	contentValue string,
	callID string,
	name string,
) contract.ContextModelInputMaterialV1 {
	t.Helper()
	kind := contract.FragmentConversation
	trust := contract.TrustUserInput
	switch channel {
	case contract.ContextModelInputInstructionV1:
		kind, trust = contract.FragmentInstruction, contract.TrustAuthoritativeInstruction
	case contract.ContextModelInputFunctionCallV1:
		kind, trust = contract.FragmentToolCall, contract.TrustAuthoritativeInstruction
	case contract.ContextModelInputFunctionResultV1:
		kind, trust = contract.FragmentToolResult, contract.TrustObservation
	case contract.ContextModelInputReferenceV1:
		kind, trust = contract.FragmentArtifactReference, contract.TrustObservation
	}
	fragment := contract.FactRef{
		ID: "single-segment", Revision: 1, Digest: contract.DigestBytes([]byte("single-segment")),
	}
	binding, err := contract.SealContextModelInputSegmentBindingV1(
		contract.ContextModelInputSegmentBindingV1{
			FragmentRef: fragment, Region: contract.RegionDynamicTail, Position: 1,
			Kind: kind, Trust: trust, Channel: channel, Role: role, Encoding: encoding,
			CallID: callID, Name: name,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte(contentValue)
	base.Ref.ID = "single-" + string(channel) + "-" + string(encoding)
	base.Ref.Revision = 1
	base.Ref.Digest = ""
	base.Digest = ""
	base.OrderedSegments = []contract.ContextModelInputSegmentV1{{
		FragmentRef: fragment, Region: binding.Region, Position: 1, Kind: binding.Kind,
		Trust: binding.Trust, Channel: binding.Channel, Role: binding.Role,
		Encoding: binding.Encoding, CallID: binding.CallID, Name: binding.Name,
		ContentRef: contract.ContentRef{
			Ref: "single-content", Digest: contract.DigestBytes(content), Length: uint64(len(content)),
		},
		Content: content, SemanticBindingDigest: binding.Digest,
	}}
	material, err := contract.SealContextModelInputMaterialV1(base)
	if err != nil {
		t.Fatal(err)
	}
	return material
}

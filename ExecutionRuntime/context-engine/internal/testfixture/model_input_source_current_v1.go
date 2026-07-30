package testfixture

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type ModelInputSourceFixtureV1 struct {
	Now      time.Time
	Owner    contract.OwnerRef
	Material contract.ContextModelInputMaterialV1
	Frame    contract.ContextFrameExactCurrentProjectionV1
	Request  contract.ContextModelInputSourceCurrentRequestV1
}

func NewModelInputSourceFixtureV1() (ModelInputSourceFixtureV1, error) {
	now := time.Unix(1_910_000_000, 123_000_000)
	owner := contract.OwnerRef{
		ComponentID:   "context-engine/model-input-source",
		BindingDigest: contract.DigestBytes([]byte("context-model-input-source-binding")),
	}
	frameRef := contract.FactRef{
		ID: "context-frame-source", Revision: 7, Digest: contract.DigestBytes([]byte("context-frame-source")),
	}
	segments := make([]contract.ContextModelInputSegmentV1, 0, 4)
	for _, item := range []struct {
		id       string
		kind     contract.FragmentKind
		trust    contract.TrustClass
		channel  contract.ContextModelInputChannelV1
		role     contract.ContextModelInputRoleV1
		encoding contract.ContextModelInputEncodingV1
		callID   string
		name     string
		content  string
	}{
		{
			id: "system-instruction", kind: contract.FragmentInstruction, trust: contract.TrustAuthoritativeInstruction,
			channel: contract.ContextModelInputInstructionV1, role: contract.ContextModelInputRoleSystemV1,
			encoding: contract.ContextModelInputUTF8V1, content: "follow the exact workspace policy",
		},
		{
			id: "developer-instruction", kind: contract.FragmentInstruction, trust: contract.TrustAuthoritativeInstruction,
			channel: contract.ContextModelInputInstructionV1, role: contract.ContextModelInputRoleDeveloperV1,
			encoding: contract.ContextModelInputUTF8V1, content: "return bounded evidence",
		},
		{
			id: "user-message", kind: contract.FragmentConversation, trust: contract.TrustUserInput,
			channel: contract.ContextModelInputMessageV1, role: contract.ContextModelInputRoleUserV1,
			encoding: contract.ContextModelInputUTF8V1, content: "inspect README.md",
		},
		{
			id: "function-call", kind: contract.FragmentToolCall, trust: contract.TrustAuthoritativeInstruction,
			channel: contract.ContextModelInputFunctionCallV1, role: contract.ContextModelInputRoleAssistantV1,
			encoding: contract.ContextModelInputCanonicalJSONV1, callID: "call-workspace-read-1",
			name: "workspace.read", content: `{"path":"README.md"}`,
		},
	} {
		position := uint32(len(segments) + 1)
		fragmentRef := contract.FactRef{
			ID: item.id, Revision: 1, Digest: contract.DigestBytes([]byte("fragment-" + item.id)),
		}
		binding, err := contract.SealContextModelInputSegmentBindingV1(contract.ContextModelInputSegmentBindingV1{
			FragmentRef: fragmentRef, Region: contract.RegionDynamicTail, Position: position,
			Kind: item.kind, Trust: item.trust, Channel: item.channel, Role: item.role,
			Encoding: item.encoding, CallID: item.callID, Name: item.name,
		})
		if err != nil {
			return ModelInputSourceFixtureV1{}, err
		}
		content := []byte(item.content)
		segments = append(segments, contract.ContextModelInputSegmentV1{
			FragmentRef: fragmentRef, Region: binding.Region, Position: position,
			Kind: binding.Kind, Trust: binding.Trust, Channel: binding.Channel, Role: binding.Role,
			Encoding: binding.Encoding, CallID: binding.CallID, Name: binding.Name,
			ContentRef: contract.ContentRef{
				Ref: "content-" + item.id, Digest: contract.DigestBytes(content), Length: uint64(len(content)),
			},
			Content: content, SemanticBindingDigest: binding.Digest,
		})
	}
	material, err := contract.SealContextModelInputMaterialV1(contract.ContextModelInputMaterialV1{
		Ref: contract.ContextModelInputMaterialRefV1{
			ID: "context-model-input-source", Revision: 3,
		},
		DescriptorRef: contract.ContextFrameConsumptionDescriptorRefV1{
			ID: "context-model-input-descriptor", Revision: 1,
			Digest: contract.DigestBytes([]byte("context-model-input-descriptor")),
		},
		FrameRef: frameRef,
		ManifestRef: contract.FactRef{
			ID: "context-model-input-manifest", Revision: 4,
			Digest: contract.DigestBytes([]byte("context-model-input-manifest")),
		},
		GenerationRef: contract.FactRef{
			ID: "context-model-input-generation", Revision: 5,
			Digest: contract.DigestBytes([]byte("context-model-input-generation")),
		},
		MaterializedDescriptorDigest: contract.DigestBytes([]byte("context-model-input-materialized-descriptor")),
		OrderedSegments:              segments,
		CheckedUnixNano:              now.Add(-time.Minute).UnixNano(),
		ExpiresUnixNano:              now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		return ModelInputSourceFixtureV1{}, err
	}
	frame, err := contract.SealContextFrameExactCurrentProjectionV1(contract.ContextFrameExactCurrentProjectionV1{
		FrameRef: frameRef, Current: true,
		CheckedUnixNano: now.Add(-time.Second).UnixNano(),
		ExpiresUnixNano: now.Add(45 * time.Second).UnixNano(),
	}, now.UnixNano())
	if err != nil {
		return ModelInputSourceFixtureV1{}, err
	}
	materialSource, err := contract.ContextModelInputMaterialExactSourceV1(owner, material.Ref)
	if err != nil {
		return ModelInputSourceFixtureV1{}, err
	}
	frameSource, err := contract.ContextFrameExactSourceV1(owner, frameRef)
	if err != nil {
		return ModelInputSourceFixtureV1{}, err
	}
	request, err := contract.SealContextModelInputSourceCurrentRequestV1(
		contract.ContextModelInputSourceCurrentRequestV1{
			Owner: owner, Material: materialSource, Frame: frameSource,
			CheckedUnixNano: now.UnixNano(), NotAfterUnixNano: now.Add(20 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		return ModelInputSourceFixtureV1{}, err
	}
	return ModelInputSourceFixtureV1{
		Now: now, Owner: owner, Material: material, Frame: frame, Request: request,
	}, nil
}

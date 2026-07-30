package testfixture

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type ModelInputLineageFixtureV1 struct {
	Now      time.Time
	Owner    contract.OwnerRef
	Material contract.ContextModelInputMaterialV1
	Frame    contract.ContextFrameExactCurrentProjectionV1
	Request  contract.ContextModelInputLineageCurrentRequestV1
}

func NewModelInputLineageFixtureV1() (ModelInputLineageFixtureV1, error) {
	now := time.Unix(1_770_000_000, 0).UTC()
	owner := contract.OwnerRef{ComponentID: "components/context", BindingDigest: contract.DigestBytes([]byte("context-owner-binding"))}
	frameRef := contract.FactRef{ID: "context-frame-lineage", Revision: 7, Digest: contract.DigestBytes([]byte("context-frame-lineage"))}
	binding, err := contract.SealContextModelInputSegmentBindingV1(contract.ContextModelInputSegmentBindingV1{
		FragmentRef: contract.FactRef{ID: "context-fragment-lineage", Revision: 2, Digest: contract.DigestBytes([]byte("context-fragment-lineage"))},
		Region:      contract.RegionDynamicTail, Position: 1, Kind: contract.FragmentConversation, Trust: contract.TrustUserInput,
		Channel: contract.ContextModelInputMessageV1, Role: contract.ContextModelInputRoleUserV1, Encoding: contract.ContextModelInputUTF8V1,
	})
	if err != nil {
		return ModelInputLineageFixtureV1{}, err
	}
	content := []byte("lineage model input")
	material, err := contract.SealContextModelInputMaterialV1(contract.ContextModelInputMaterialV1{
		Ref:                          contract.ContextModelInputMaterialRefV1{ID: "context-model-input-lineage", Revision: 3},
		DescriptorRef:                contract.ContextFrameConsumptionDescriptorRefV1{ID: "context-descriptor-lineage", Revision: 1, Digest: contract.DigestBytes([]byte("context-descriptor-lineage"))},
		FrameRef:                     frameRef,
		ManifestRef:                  contract.FactRef{ID: "context-manifest-lineage", Revision: 4, Digest: contract.DigestBytes([]byte("context-manifest-lineage"))},
		GenerationRef:                contract.FactRef{ID: "context-generation-lineage", Revision: 5, Digest: contract.DigestBytes([]byte("context-generation-lineage"))},
		MaterializedDescriptorDigest: contract.DigestBytes([]byte("materialized-context-descriptor-lineage")),
		OrderedSegments: []contract.ContextModelInputSegmentV1{{
			FragmentRef: binding.FragmentRef, Region: binding.Region, Position: binding.Position, Kind: binding.Kind, Trust: binding.Trust,
			Channel: binding.Channel, Role: binding.Role, Encoding: binding.Encoding,
			ContentRef: contract.ContentRef{Ref: "context-content-lineage", Digest: contract.DigestBytes(content), Length: uint64(len(content))},
			Content:    content, SemanticBindingDigest: binding.Digest,
		}},
		CheckedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		return ModelInputLineageFixtureV1{}, err
	}
	frame, err := contract.SealContextFrameExactCurrentProjectionV1(contract.ContextFrameExactCurrentProjectionV1{
		FrameRef: frameRef, Current: true, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(45 * time.Second).UnixNano(),
	}, now.UnixNano())
	if err != nil {
		return ModelInputLineageFixtureV1{}, err
	}
	source, err := contract.ContextModelInputMaterialExactSourceV1(owner, material.Ref)
	if err != nil {
		return ModelInputLineageFixtureV1{}, err
	}
	request, err := contract.SealContextModelInputLineageCurrentRequestV1(contract.ContextModelInputLineageCurrentRequestV1{
		Source: source, CheckedUnixNano: now.UnixNano(), NotAfterUnixNano: now.Add(20 * time.Second).UnixNano(),
	})
	if err != nil {
		return ModelInputLineageFixtureV1{}, err
	}
	return ModelInputLineageFixtureV1{Now: now, Owner: owner, Material: material, Frame: frame, Request: request}, nil
}

type ModelInputLineageMaterialReaderV1 struct {
	mu              sync.Mutex
	ExactSequence   []contract.ContextModelInputMaterialV1
	CurrentSequence []contract.ContextModelInputMaterialV1
	ExactErr        error
	CurrentErr      error
	exactCalls      int
	currentCalls    int
}

func NewModelInputLineageMaterialReaderV1(material contract.ContextModelInputMaterialV1) *ModelInputLineageMaterialReaderV1 {
	return &ModelInputLineageMaterialReaderV1{
		ExactSequence:   []contract.ContextModelInputMaterialV1{material.Clone()},
		CurrentSequence: []contract.ContextModelInputMaterialV1{material.Clone()},
	}
}

func (r *ModelInputLineageMaterialReaderV1) ReadContextModelInputMaterialExactV1(ctx context.Context, _ contract.ContextModelInputMaterialRefV1, _ int64) (contract.ContextModelInputMaterialV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exactCalls++
	if err := ctx.Err(); err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	if r.ExactErr != nil {
		return contract.ContextModelInputMaterialV1{}, r.ExactErr
	}
	return materialAtV1(r.ExactSequence, r.exactCalls).Clone(), nil
}

func (r *ModelInputLineageMaterialReaderV1) ReadContextModelInputMaterialCurrentV1(ctx context.Context, _ string, _ int64) (contract.ContextModelInputMaterialV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentCalls++
	if err := ctx.Err(); err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	if r.CurrentErr != nil {
		return contract.ContextModelInputMaterialV1{}, r.CurrentErr
	}
	return materialAtV1(r.CurrentSequence, r.currentCalls).Clone(), nil
}

func (r *ModelInputLineageMaterialReaderV1) CallsV1() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.exactCalls, r.currentCalls
}

type ModelInputLineageFrameReaderV1 struct {
	mu         sync.Mutex
	Sequence   []contract.ContextFrameExactCurrentProjectionV1
	Err        error
	calls      int
	BeforeCall func(int)
}

func NewModelInputLineageFrameReaderV1(frame contract.ContextFrameExactCurrentProjectionV1) *ModelInputLineageFrameReaderV1 {
	return &ModelInputLineageFrameReaderV1{Sequence: []contract.ContextFrameExactCurrentProjectionV1{frame}}
}

func (r *ModelInputLineageFrameReaderV1) InspectContextFrameExactCurrentV1(ctx context.Context, _ contract.FactRef, _ int64) (contract.ContextFrameExactCurrentProjectionV1, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	hook := r.BeforeCall
	r.mu.Unlock()
	if hook != nil {
		hook(call)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextFrameExactCurrentProjectionV1{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return contract.ContextFrameExactCurrentProjectionV1{}, r.Err
	}
	return frameAtV1(r.Sequence, call), nil
}

func (r *ModelInputLineageFrameReaderV1) CallsV1() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func materialAtV1(values []contract.ContextModelInputMaterialV1, call int) contract.ContextModelInputMaterialV1 {
	if len(values) == 0 {
		return contract.ContextModelInputMaterialV1{}
	}
	index := call - 1
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func frameAtV1(values []contract.ContextFrameExactCurrentProjectionV1, call int) contract.ContextFrameExactCurrentProjectionV1 {
	if len(values) == 0 {
		return contract.ContextFrameExactCurrentProjectionV1{}
	}
	index := call - 1
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

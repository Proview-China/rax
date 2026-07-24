package kernel

import (
	"context"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

const (
	StagedStreamChunkBytesV1 = 64 * 1024
	StagedCloneChunkBytesV1  = 64 * 1024
)

type CompileWorkLimitsV1 struct {
	MaxCandidates            uint32
	MaxInputContentItems     uint32
	MaxInputContentItemBytes uint64
	MaxInputRawBytes         uint64
	MaxGeneratedContentItems uint32
	MaxGeneratedRawBytes     uint64
	MaxOutputContentItems    uint32
	MaxOutputRawBytes        uint64
	MaxTotalTokens           uint64
	StreamChunkBytes         uint32
	CloneChunkBytes          uint32
}

type InspectWorkLimitsV1 struct {
	MaxFragments        uint32
	MaxContentItems     uint32
	MaxContentItemBytes uint64
	MaxRawBytes         uint64
	StreamChunkBytes    uint32
	CloneChunkBytes     uint32
}

type ContextAwareReferenceStoreV1 interface {
	GetContextV1(context.Context, contract.ContentRef) ([]byte, error)
	PutContextV1(context.Context, []byte) (contract.ContentRef, error)
}

func (l CompileWorkLimitsV1) Validate() error {
	if l.MaxCandidates == 0 || l.MaxCandidates > 512 ||
		l.MaxInputContentItems == 0 || l.MaxInputContentItems > 1024 ||
		l.MaxInputContentItemBytes == 0 || l.MaxInputContentItemBytes > 4*1024*1024 ||
		l.MaxInputRawBytes == 0 || l.MaxInputRawBytes > 24*1024*1024 ||
		l.MaxGeneratedContentItems == 0 || l.MaxGeneratedContentItems > 4 ||
		l.MaxGeneratedRawBytes == 0 || l.MaxGeneratedRawBytes > 52*1024*1024 ||
		l.MaxOutputContentItems == 0 || l.MaxOutputContentItems > 1028 ||
		l.MaxOutputRawBytes == 0 || l.MaxOutputRawBytes > 76*1024*1024 ||
		l.MaxTotalTokens == 0 || l.MaxTotalTokens > 1024*1024 ||
		l.StreamChunkBytes != StagedStreamChunkBytesV1 || l.CloneChunkBytes != StagedCloneChunkBytesV1 {
		return fmt.Errorf("%w: staged compile limits", contract.ErrInvalid)
	}
	return nil
}

func (l InspectWorkLimitsV1) Validate() error {
	if l.MaxFragments == 0 || l.MaxFragments > 512 ||
		l.MaxContentItems == 0 || l.MaxContentItems > 1028 ||
		l.MaxContentItemBytes == 0 || l.MaxContentItemBytes > 76*1024*1024 ||
		l.MaxRawBytes == 0 || l.MaxRawBytes > 76*1024*1024 ||
		l.StreamChunkBytes != StagedStreamChunkBytesV1 || l.CloneChunkBytes != StagedCloneChunkBytesV1 {
		return fmt.Errorf("%w: staged inspect limits", contract.ErrInvalid)
	}
	return nil
}

func checkContextV1(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	return ctx.Err()
}

type legacyReferenceStoreAdapterV1 struct{ store ReferenceStore }

func (a legacyReferenceStoreAdapterV1) GetContextV1(_ context.Context, ref contract.ContentRef) ([]byte, error) {
	return a.store.Get(ref)
}

func (a legacyReferenceStoreAdapterV1) PutContextV1(_ context.Context, value []byte) (contract.ContentRef, error) {
	return a.store.Put(value)
}

func legacyCompileWorkLimitsV1() CompileWorkLimitsV1 {
	return CompileWorkLimitsV1{
		MaxCandidates: 512, MaxInputContentItems: 1024, MaxInputContentItemBytes: 4 * 1024 * 1024,
		MaxInputRawBytes: 24 * 1024 * 1024, MaxGeneratedContentItems: 4, MaxGeneratedRawBytes: 52 * 1024 * 1024,
		MaxOutputContentItems: 1028, MaxOutputRawBytes: 76 * 1024 * 1024, MaxTotalTokens: 1024 * 1024,
		StreamChunkBytes: StagedStreamChunkBytesV1, CloneChunkBytes: StagedCloneChunkBytesV1,
	}
}

func legacyInspectWorkLimitsV1() InspectWorkLimitsV1 {
	return InspectWorkLimitsV1{
		MaxFragments: 512, MaxContentItems: 1028, MaxContentItemBytes: 76 * 1024 * 1024,
		MaxRawBytes: 76 * 1024 * 1024, StreamChunkBytes: StagedStreamChunkBytesV1, CloneChunkBytes: StagedCloneChunkBytesV1,
	}
}

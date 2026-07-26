package runtimeapi

import (
	"context"
	"fmt"
	"math"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

const (
	HardMaxMaterializedItemsV1      = uint32(4)
	HardMaxMaterializedItemBytesV1  = uint64(68 * 1024 * 1024)
	HardMaxMaterializedTotalBytesV1 = uint64(100 * 1024 * 1024)
)

type MaterializeDescriptorLimitsV1 struct {
	MaxItems      uint32
	MaxItemBytes  uint64
	MaxTotalBytes uint64
}

func (l MaterializeDescriptorLimitsV1) Validate() error {
	if l.MaxItems == 0 || l.MaxItems > HardMaxMaterializedItemsV1 || l.MaxItemBytes == 0 || l.MaxItemBytes > HardMaxMaterializedItemBytesV1 || l.MaxTotalBytes == 0 || l.MaxTotalBytes > HardMaxMaterializedTotalBytesV1 {
		return fmt.Errorf("%w: descriptor materialization limits", contract.ErrInvalid)
	}
	return nil
}

type MaterializeDescriptorRequestV1 struct {
	Descriptor      contract.ContextFrameConsumptionDescriptorV1
	CheckedUnixNano int64
	Limits          MaterializeDescriptorLimitsV1
}

type MaterializedDescriptorRegionV1 string

const (
	MaterializedStablePrefixV1 MaterializedDescriptorRegionV1 = "stable_prefix"
	MaterializedSemiStableV1   MaterializedDescriptorRegionV1 = "semi_stable"
	MaterializedDynamicTailV1  MaterializedDescriptorRegionV1 = "dynamic_tail"
	MaterializedRenderedV1     MaterializedDescriptorRegionV1 = "rendered"
)

type MaterializedDescriptorItemV1 struct {
	Region MaterializedDescriptorRegionV1
	Ref    contract.ContentRef
	Bytes  []byte
}

type MaterializeDescriptorResultV1 struct {
	DescriptorRef   contract.ContextFrameConsumptionDescriptorRefV1
	Items           []MaterializedDescriptorItemV1
	CheckedUnixNano int64
	ExpiresUnixNano int64
	Digest          contract.Digest
}

func (r MaterializeDescriptorResultV1) Validate() error {
	if r.DescriptorRef.Validate() != nil || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano || r.Digest.Validate() != nil || !canonicalMaterializedItemsV1(r.Items) {
		return fmt.Errorf("%w: descriptor materialization result", contract.ErrInvalid)
	}
	for _, item := range r.Items {
		if item.Ref.Validate() != nil || len(item.Bytes) == 0 || uint64(len(item.Bytes)) != item.Ref.Length || contract.DigestBytes(item.Bytes) != item.Ref.Digest {
			return fmt.Errorf("%w: descriptor materialization item", contract.ErrConflict)
		}
	}
	want, err := materializeResultDigestV1(r)
	if err != nil || want != r.Digest {
		return fmt.Errorf("%w: descriptor materialization digest", contract.ErrConflict)
	}
	return nil
}

func (s *ServiceV1) MaterializeDescriptor(ctx context.Context, request MaterializeDescriptorRequestV1) (MaterializeDescriptorResultV1, error) {
	if s == nil || ctx == nil {
		return MaterializeDescriptorResultV1{}, fmt.Errorf("%w: context runtime api service", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return MaterializeDescriptorResultV1{}, err
	}
	if request.Descriptor.Validate() != nil || request.Limits.Validate() != nil || request.CheckedUnixNano <= 0 {
		return MaterializeDescriptorResultV1{}, fmt.Errorf("%w: descriptor materialization request", contract.ErrInvalid)
	}
	if request.CheckedUnixNano < request.Descriptor.CheckedUnixNano {
		return MaterializeDescriptorResultV1{}, fmt.Errorf("%w: descriptor materialization clock rollback", contract.ErrConflict)
	}
	if request.CheckedUnixNano >= request.Descriptor.ExpiresUnixNano {
		return MaterializeDescriptorResultV1{}, fmt.Errorf("%w: descriptor materialization lifetime", contract.ErrExpired)
	}

	refs, count := materializeRefsV1(request.Descriptor)
	if uint32(count) > request.Limits.MaxItems {
		return MaterializeDescriptorResultV1{}, fmt.Errorf("%w: descriptor materialization items", contract.ErrLimitExceeded)
	}
	var total uint64
	for index := 0; index < count; index++ {
		ref := refs[index].ref
		if ref.Validate() != nil {
			return MaterializeDescriptorResultV1{}, fmt.Errorf("%w: descriptor materialization reference", contract.ErrInvalid)
		}
		if ref.Length > request.Limits.MaxItemBytes {
			return MaterializeDescriptorResultV1{}, fmt.Errorf("%w: descriptor materialization item bytes", contract.ErrLimitExceeded)
		}
		if total > math.MaxUint64-ref.Length {
			return MaterializeDescriptorResultV1{}, fmt.Errorf("%w: descriptor materialization byte overflow", contract.ErrLimitExceeded)
		}
		total += ref.Length
		if total > request.Limits.MaxTotalBytes {
			return MaterializeDescriptorResultV1{}, fmt.Errorf("%w: descriptor materialization total bytes", contract.ErrLimitExceeded)
		}
	}

	items := make([]MaterializedDescriptorItemV1, count)
	for index := 0; index < count; index++ {
		if err := ctx.Err(); err != nil {
			return MaterializeDescriptorResultV1{}, err
		}
		payload, err := s.store.GetContextV1(ctx, refs[index].ref)
		if err != nil {
			return MaterializeDescriptorResultV1{}, err
		}
		if err = ctx.Err(); err != nil {
			return MaterializeDescriptorResultV1{}, err
		}
		if uint64(len(payload)) != refs[index].ref.Length || contract.DigestBytes(payload) != refs[index].ref.Digest {
			return MaterializeDescriptorResultV1{}, fmt.Errorf("%w: descriptor materialization exact content", contract.ErrConflict)
		}
		items[index] = MaterializedDescriptorItemV1{Region: refs[index].region, Ref: refs[index].ref, Bytes: append([]byte(nil), payload...)}
	}
	descriptorRef, err := request.Descriptor.RefV1()
	if err != nil {
		return MaterializeDescriptorResultV1{}, err
	}
	result := MaterializeDescriptorResultV1{DescriptorRef: descriptorRef, Items: items, CheckedUnixNano: request.CheckedUnixNano, ExpiresUnixNano: request.Descriptor.ExpiresUnixNano}
	result.Digest, err = materializeResultDigestV1(result)
	if err != nil {
		return MaterializeDescriptorResultV1{}, err
	}
	if err = result.Validate(); err != nil {
		return MaterializeDescriptorResultV1{}, err
	}
	return cloneMaterializeResultV1(result), nil
}

type materializeRefV1 struct {
	region MaterializedDescriptorRegionV1
	ref    contract.ContentRef
}

func materializeRefsV1(descriptor contract.ContextFrameConsumptionDescriptorV1) ([4]materializeRefV1, int) {
	refs := [4]materializeRefV1{{MaterializedStablePrefixV1, descriptor.StablePrefix}}
	count := 1
	if descriptor.SemiStable != nil {
		refs[count] = materializeRefV1{MaterializedSemiStableV1, *descriptor.SemiStable}
		count++
	}
	refs[count] = materializeRefV1{MaterializedDynamicTailV1, descriptor.DynamicTail}
	count++
	refs[count] = materializeRefV1{MaterializedRenderedV1, descriptor.Rendered}
	count++
	return refs, count
}

func canonicalMaterializedItemsV1(items []MaterializedDescriptorItemV1) bool {
	if len(items) != 3 && len(items) != 4 {
		return false
	}
	expected := []MaterializedDescriptorRegionV1{MaterializedStablePrefixV1, MaterializedDynamicTailV1, MaterializedRenderedV1}
	if len(items) == 4 {
		expected = []MaterializedDescriptorRegionV1{MaterializedStablePrefixV1, MaterializedSemiStableV1, MaterializedDynamicTailV1, MaterializedRenderedV1}
	}
	for index := range items {
		if items[index].Region != expected[index] {
			return false
		}
	}
	return true
}

func materializeResultDigestV1(result MaterializeDescriptorResultV1) (contract.Digest, error) {
	type itemRefV1 struct {
		Region MaterializedDescriptorRegionV1
		Ref    contract.ContentRef
	}
	refs := make([]itemRefV1, len(result.Items))
	for index, item := range result.Items {
		refs[index] = itemRefV1{Region: item.Region, Ref: item.Ref}
	}
	return contract.DigestJSON(struct {
		Domain          string
		DescriptorRef   contract.ContextFrameConsumptionDescriptorRefV1
		Items           []itemRefV1
		CheckedUnixNano int64
		ExpiresUnixNano int64
	}{"praxis.context/materialized-descriptor-v1", result.DescriptorRef, refs, result.CheckedUnixNano, result.ExpiresUnixNano})
}

func cloneMaterializeResultV1(result MaterializeDescriptorResultV1) MaterializeDescriptorResultV1 {
	copy := result
	copy.Items = make([]MaterializedDescriptorItemV1, len(result.Items))
	for index, item := range result.Items {
		copy.Items[index] = item
		copy.Items[index].Bytes = append([]byte(nil), item.Bytes...)
	}
	return copy
}

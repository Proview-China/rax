package runtimeapi

import (
	"context"
	"fmt"
	"math"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

const (
	HardMaxModelInputSegmentBytesV1 = uint64(68 * 1024 * 1024)
	HardMaxModelInputTotalBytesV1   = uint64(100 * 1024 * 1024)
)

type MaterializeModelInputLimitsV1 struct {
	MaxSegments     uint32
	MaxSegmentBytes uint64
	MaxTotalBytes   uint64
}

func (l MaterializeModelInputLimitsV1) Validate() error {
	if l.MaxSegments == 0 || l.MaxSegments > contract.MaxContextModelInputSegmentsV1 || l.MaxSegmentBytes == 0 || l.MaxSegmentBytes > HardMaxModelInputSegmentBytesV1 || l.MaxTotalBytes == 0 || l.MaxTotalBytes > HardMaxModelInputTotalBytesV1 {
		return fmt.Errorf("%w: model input materialization limits", contract.ErrInvalid)
	}
	return nil
}

type MaterializeModelInputRequestV1 struct {
	MaterialID       string
	MaterialRevision uint64
	Consumption      contract.ContextFrameConsumptionRequestV1
	Descriptor       contract.ContextFrameConsumptionDescriptorV1
	SegmentBindings  []contract.ContextModelInputSegmentBindingV1
	CheckedUnixNano  int64
	Limits           MaterializeModelInputLimitsV1
}

type ContextModelInputRuntimeAPIV1 interface {
	MaterializeModelInputV1(context.Context, MaterializeModelInputRequestV1) (contract.ContextModelInputMaterialV1, error)
}

// MaterializeModelInputV1 creates a Context-owned, provider-neutral model input
// fact. It never calls Tool or Provider and never parses fragment bytes to
// recover semantic fields.
func (s *ServiceV1) MaterializeModelInputV1(ctx context.Context, request MaterializeModelInputRequestV1) (contract.ContextModelInputMaterialV1, error) {
	if s == nil || ctx == nil {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: context runtime api service", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	materialRef := contract.ContextModelInputMaterialRefV1{ID: request.MaterialID, Revision: request.MaterialRevision, Digest: request.Descriptor.Digest}
	if materialRef.Validate() != nil || request.Consumption.Validate() != nil || request.Descriptor.Validate() != nil || request.Limits.Validate() != nil || request.CheckedUnixNano <= 0 {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input materialization request", contract.ErrInvalid)
	}
	if request.CheckedUnixNano < request.Descriptor.CheckedUnixNano {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input materialization clock rollback", contract.ErrConflict)
	}
	if request.CheckedUnixNano >= request.Descriptor.ExpiresUnixNano {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input materialization lifetime", contract.ErrExpired)
	}
	if request.Descriptor.FrameRef != request.Consumption.FrameRef || request.Descriptor.ManifestRef != request.Consumption.ManifestRef || request.Descriptor.GenerationRef != request.Consumption.GenerationRef || request.Descriptor.CheckedUnixNano != request.Consumption.CheckedUnixNano {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input descriptor request binding", contract.ErrConflict)
	}
	if len(request.SegmentBindings) == 0 || len(request.SegmentBindings) > int(request.Limits.MaxSegments) {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input segment count", contract.ErrLimitExceeded)
	}
	for index, binding := range request.SegmentBindings {
		if binding.Validate() != nil || binding.Position != uint32(index+1) {
			return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input explicit semantic binding", contract.ErrConflict)
		}
	}

	currentDescriptor, err := kernel.BuildFrameConsumptionDescriptorV1(ctx, s.frameReader, s.store, request.Consumption)
	if err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	if !reflect.DeepEqual(currentDescriptor, request.Descriptor) {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input descriptor current drift", contract.ErrConflict)
	}

	snapshot1, err := s.frameReader.InspectFrameConsumptionCurrentV1(ctx, request.Consumption)
	if err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	if err = validateModelInputSnapshotV1(request, snapshot1); err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	if err = preflightModelInputSegmentsV1(snapshot1.Manifest.Fragments, request.Limits); err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}

	materializedDescriptor, err := s.MaterializeDescriptor(ctx, MaterializeDescriptorRequestV1{
		Descriptor:      request.Descriptor,
		CheckedUnixNano: request.CheckedUnixNano,
		Limits: MaterializeDescriptorLimitsV1{
			MaxItems:      HardMaxMaterializedItemsV1,
			MaxItemBytes:  HardMaxMaterializedItemBytesV1,
			MaxTotalBytes: HardMaxMaterializedTotalBytesV1,
		},
	})
	if err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}

	segments := make([]contract.ContextModelInputSegmentV1, len(snapshot1.Manifest.Fragments))
	for index, fragment := range snapshot1.Manifest.Fragments {
		if err = ctx.Err(); err != nil {
			return contract.ContextModelInputMaterialV1{}, err
		}
		content, readErr := s.store.GetContextV1(ctx, fragment.Content)
		if readErr != nil {
			return contract.ContextModelInputMaterialV1{}, readErr
		}
		if uint64(len(content)) != fragment.Content.Length || contract.DigestBytes(content) != fragment.Content.Digest {
			return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input exact fragment content", contract.ErrConflict)
		}
		binding := request.SegmentBindings[index]
		segment := contract.ContextModelInputSegmentV1{
			FragmentRef:           fragment.CandidateRef,
			Region:                fragment.Region,
			Position:              fragment.Position,
			Kind:                  fragment.Kind,
			Trust:                 binding.Trust,
			Channel:               binding.Channel,
			Role:                  binding.Role,
			Encoding:              binding.Encoding,
			CallID:                binding.CallID,
			Name:                  binding.Name,
			ContentRef:            fragment.Content,
			Content:               append([]byte(nil), content...),
			SemanticBindingDigest: binding.Digest,
		}
		if err = segment.Validate(); err != nil {
			return contract.ContextModelInputMaterialV1{}, err
		}
		segments[index] = segment
	}
	if err = ctx.Err(); err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	snapshot2, err := s.frameReader.InspectFrameConsumptionCurrentV1(ctx, request.Consumption)
	if err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	if !reflect.DeepEqual(snapshot1, snapshot2) {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input materialization S2 drift", contract.ErrConflict)
	}
	if err = validateModelInputSnapshotV1(request, snapshot2); err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	if request.CheckedUnixNano >= request.Descriptor.ExpiresUnixNano {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input materialization TTL crossing", contract.ErrExpired)
	}

	material, err := contract.SealContextModelInputMaterialV1(contract.ContextModelInputMaterialV1{
		Ref:                          contract.ContextModelInputMaterialRefV1{ID: request.MaterialID, Revision: request.MaterialRevision},
		DescriptorRef:                materializedDescriptor.DescriptorRef,
		FrameRef:                     request.Descriptor.FrameRef,
		ManifestRef:                  request.Descriptor.ManifestRef,
		GenerationRef:                request.Descriptor.GenerationRef,
		MaterializedDescriptorDigest: materializedDescriptor.Digest,
		OrderedSegments:              segments,
		CheckedUnixNano:              request.CheckedUnixNano,
		ExpiresUnixNano:              request.Descriptor.ExpiresUnixNano,
	})
	if err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	return material.Clone(), nil
}

var _ ContextModelInputRuntimeAPIV1 = (*ServiceV1)(nil)

func validateModelInputSnapshotV1(request MaterializeModelInputRequestV1, snapshot kernel.FrameConsumptionCurrentSnapshotV1) error {
	if snapshot.Manifest.Validate() != nil || snapshot.Frame.Validate() != nil || snapshot.Generation.Validate() != nil {
		return fmt.Errorf("%w: model input current snapshot", contract.ErrInvalid)
	}
	manifestDigest, err := snapshot.Manifest.DigestValue()
	if err != nil {
		return err
	}
	frameDigest, err := snapshot.Frame.DigestValue()
	if err != nil {
		return err
	}
	generationDigest, err := contract.DigestJSON(snapshot.Generation)
	if err != nil {
		return err
	}
	if request.Descriptor.ManifestRef != (contract.FactRef{ID: snapshot.Manifest.ID, Revision: snapshot.Manifest.Revision, Digest: manifestDigest}) ||
		request.Descriptor.FrameRef != (contract.FactRef{ID: snapshot.Frame.ID, Revision: snapshot.Frame.Revision, Digest: frameDigest}) ||
		request.Descriptor.GenerationRef != (contract.FactRef{ID: snapshot.Generation.ID, Revision: snapshot.Generation.Revision, Digest: generationDigest}) {
		return fmt.Errorf("%w: model input exact snapshot references", contract.ErrConflict)
	}
	if len(snapshot.Manifest.Fragments) != len(request.SegmentBindings) || len(request.Descriptor.FragmentRefs) != len(snapshot.Manifest.Fragments) {
		return fmt.Errorf("%w: model input fragment cardinality", contract.ErrConflict)
	}
	for index, fragment := range snapshot.Manifest.Fragments {
		binding := request.SegmentBindings[index]
		if request.Descriptor.FragmentRefs[index] != fragment.CandidateRef || binding.FragmentRef != fragment.CandidateRef || binding.Region != fragment.Region || binding.Position != fragment.Position || binding.Kind != fragment.Kind {
			return fmt.Errorf("%w: model input fragment or semantic binding drift", contract.ErrConflict)
		}
	}
	if snapshot.Frame.ManifestRef != request.Descriptor.ManifestRef || snapshot.Generation.RootFrame != request.Descriptor.FrameRef || snapshot.Frame.GenerationID != snapshot.Generation.ID {
		return fmt.Errorf("%w: model input frame chain", contract.ErrConflict)
	}
	return nil
}

func preflightModelInputSegmentsV1(fragments []contract.ContextFragment, limits MaterializeModelInputLimitsV1) error {
	if len(fragments) == 0 || len(fragments) > int(limits.MaxSegments) {
		return fmt.Errorf("%w: model input segment count", contract.ErrLimitExceeded)
	}
	var total uint64
	for _, fragment := range fragments {
		if fragment.Content.Validate() != nil {
			return fmt.Errorf("%w: model input fragment content reference", contract.ErrInvalid)
		}
		if fragment.Content.Length > limits.MaxSegmentBytes {
			return fmt.Errorf("%w: model input segment bytes", contract.ErrLimitExceeded)
		}
		if total > math.MaxUint64-fragment.Content.Length {
			return fmt.Errorf("%w: model input total byte overflow", contract.ErrLimitExceeded)
		}
		total += fragment.Content.Length
		if total > limits.MaxTotalBytes {
			return fmt.Errorf("%w: model input total bytes", contract.ErrLimitExceeded)
		}
	}
	return nil
}

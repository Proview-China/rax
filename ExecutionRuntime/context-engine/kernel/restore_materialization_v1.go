package kernel

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

type RestoreContextMaterializationServiceV1 struct {
	frames       contextports.ContextFrameMetadataReaderV1
	generations  contextports.ContextGenerationMetadataReaderV1
	requirements contextports.RestoreContextRequirementCurrentReaderV1
	store        contextports.RestoreContextMaterializationStoreV1
	clock        func() time.Time
}

func NewRestoreContextMaterializationServiceV1(frames contextports.ContextFrameMetadataReaderV1, generations contextports.ContextGenerationMetadataReaderV1, requirements contextports.RestoreContextRequirementCurrentReaderV1, store contextports.RestoreContextMaterializationStoreV1, clock func() time.Time) (*RestoreContextMaterializationServiceV1, error) {
	if restoreContextNilV1(frames) || restoreContextNilV1(generations) || restoreContextNilV1(requirements) || restoreContextNilV1(store) || clock == nil {
		return nil, fmt.Errorf("%w: Restore Context materialization dependencies", contract.ErrInvalid)
	}
	return &RestoreContextMaterializationServiceV1{frames: frames, generations: generations, requirements: requirements, store: store, clock: clock}, nil
}

func (s *RestoreContextMaterializationServiceV1) MaterializeRestoreContextV1(ctx context.Context, request contract.RestoreContextMaterializationRequestV1) (contract.RestoreContextMaterializationFactV1, error) {
	if s == nil {
		return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: Restore Context service", contract.ErrInvalid)
	}
	now := s.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return contract.RestoreContextMaterializationFactV1{}, err
	}
	requestDigest, err := request.DigestValue()
	if err != nil {
		return contract.RestoreContextMaterializationFactV1{}, err
	}
	if existing, inspectErr := s.store.InspectRestoreContextMaterializationByTargetV1(ctx, request.Target); inspectErr == nil {
		if existing.RequestDigest != requestDigest {
			return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: Restore Context target already binds another request", contract.ErrConflict)
		}
		return existing, nil
	} else if !isRestoreContextNotFoundV1(inspectErr) {
		return contract.RestoreContextMaterializationFactV1{}, inspectErr
	}

	s1Generation, s1Frames, err := s.inspectSourceV1(ctx, request)
	if err != nil {
		return contract.RestoreContextMaterializationFactV1{}, err
	}
	requirements, err := s.requirements.InspectRestoreContextRequirementsCurrentV1(ctx, request)
	if err != nil {
		return contract.RestoreContextMaterializationFactV1{}, err
	}
	fresh := s.clock()
	if err := requirements.ValidateCurrent(fresh); err != nil || requirements.RequestDigest != requestDigest {
		return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: Restore Context requirement current drift", contract.ErrConflict)
	}
	s2Generation, s2Frames, err := s.inspectSourceV1(ctx, request)
	if err != nil {
		return contract.RestoreContextMaterializationFactV1{}, err
	}
	if !reflect.DeepEqual(s1Generation, s2Generation) || !sameSourceFramesV1(s1Frames, s2Frames) {
		return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: Restore Context S1/S2 source drift", contract.ErrConflict)
	}
	expires := minimumRestoreContextTimeV1(request.NotAfterUnixNano, requirements.ExpiresUnixNano)
	for _, source := range s1Frames {
		expires = minimumRestoreContextTimeV1(expires, source.ExpiresUnixNano)
	}
	targetFrames := make([]contract.RestoredContextFrameV1, len(s1Frames))
	targetFrameRefs := make([]contract.FactRef, len(s1Frames))
	for index, source := range s1Frames {
		idDigest, digestErr := contract.DigestJSON(struct {
			Request contract.Digest
			Source  contract.FactRef
		}{requestDigest, request.SourceFrames[index]})
		if digestErr != nil {
			return contract.RestoreContextMaterializationFactV1{}, digestErr
		}
		frame, sealErr := contract.SealRestoredContextFrameV1(contract.RestoredContextFrameV1{ID: "restore-frame:" + string(idDigest), Target: request.Target, SourceFrame: request.SourceFrames[index], StablePrefix: source.StablePrefix, SemiStable: cloneContextContentRefV1(source.SemiStable), DynamicTail: source.DynamicTail, Rendered: source.Rendered, CreatedUnixNano: fresh.UnixNano(), ExpiresUnixNano: expires})
		if sealErr != nil {
			return contract.RestoreContextMaterializationFactV1{}, sealErr
		}
		targetFrames[index], targetFrameRefs[index] = frame, frame.Ref()
	}
	sort.Slice(targetFrames, func(i, j int) bool {
		return compareRestoreContextFactRefV1(targetFrames[i].Ref(), targetFrames[j].Ref()) < 0
	})
	for index := range targetFrames {
		targetFrameRefs[index] = targetFrames[index].Ref()
	}
	generationID, err := contract.DigestJSON(struct {
		Request contract.Digest
		Source  contract.FactRef
	}{requestDigest, request.SourceGeneration})
	if err != nil {
		return contract.RestoreContextMaterializationFactV1{}, err
	}
	generation, err := contract.SealRestoredContextGenerationV1(contract.RestoredContextGenerationV1{ID: "restore-generation:" + string(generationID), Target: request.Target, SourceGeneration: request.SourceGeneration, Frames: targetFrameRefs, CreatedUnixNano: fresh.UnixNano()})
	if err != nil {
		return contract.RestoreContextMaterializationFactV1{}, err
	}
	currentDigest, err := contract.DigestJSON(struct {
		Target     contract.RestoreContextTargetBindingV1
		Generation contract.FactRef
		Frames     []contract.FactRef
	}{request.Target, generation.Ref(), targetFrameRefs})
	if err != nil {
		return contract.RestoreContextMaterializationFactV1{}, err
	}
	fact, err := contract.SealRestoreContextMaterializationFactV1(contract.RestoreContextMaterializationFactV1{ID: request.ID, RequestDigest: requestDigest, Target: request.Target, Attempt: request.Attempt, Eligibility: request.Eligibility, Stage: request.Stage, SandboxApply: request.SandboxApply, SourceGeneration: request.SourceGeneration, TargetGeneration: generation.Ref(), TargetFrames: targetFrameRefs, Requirements: requirements, CurrentDigest: currentDigest, CreatedUnixNano: fresh.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		return contract.RestoreContextMaterializationFactV1{}, err
	}
	stored, err := s.store.CommitRestoreContextMaterializationV1(ctx, request, targetFrames, generation, fact)
	if err != nil {
		var inspectErr error
		stored, inspectErr = s.store.InspectRestoreContextMaterializationByTargetV1(context.WithoutCancel(ctx), request.Target)
		if inspectErr != nil {
			return contract.RestoreContextMaterializationFactV1{}, err
		}
	}
	if stored.Ref() != fact.Ref() || stored.RequestDigest != requestDigest || stored.CurrentDigest != fact.CurrentDigest {
		return contract.RestoreContextMaterializationFactV1{}, fmt.Errorf("%w: Restore Context commit recovery drift", contract.ErrConflict)
	}
	return stored, nil
}

func (s *RestoreContextMaterializationServiceV1) inspectSourceV1(ctx context.Context, request contract.RestoreContextMaterializationRequestV1) (contract.ContextGeneration, []contract.ContextFrame, error) {
	generation, err := s.generations.GenerationByExactRef(ctx, request.SourceGeneration, request.SourceScope)
	if err != nil {
		return contract.ContextGeneration{}, nil, err
	}
	digest, err := generation.DigestValue()
	if err != nil || request.SourceGeneration != (contract.FactRef{ID: generation.ID, Revision: generation.Revision, Digest: digest}) {
		return contract.ContextGeneration{}, nil, fmt.Errorf("%w: source Context Generation exact drift", contract.ErrConflict)
	}
	frames := make([]contract.ContextFrame, len(request.SourceFrames))
	rootFound := false
	for index, expected := range request.SourceFrames {
		frame, readErr := s.frames.FrameByExactRef(ctx, expected, request.SourceScope)
		if readErr != nil {
			return contract.ContextGeneration{}, nil, readErr
		}
		frameDigest, digestErr := frame.DigestValue()
		if digestErr != nil || expected != (contract.FactRef{ID: frame.ID, Revision: frame.Revision, Digest: frameDigest}) || frame.GenerationID != generation.ID {
			return contract.ContextGeneration{}, nil, fmt.Errorf("%w: source Context Frame exact drift", contract.ErrConflict)
		}
		rootFound = rootFound || expected == generation.RootFrame
		frames[index] = frame
	}
	if !rootFound {
		return contract.ContextGeneration{}, nil, fmt.Errorf("%w: source Context Generation root Frame missing", contract.ErrConflict)
	}
	return generation, frames, nil
}

func (s *RestoreContextMaterializationServiceV1) InspectRestoreContextMaterializationV1(ctx context.Context, ref contract.FactRef) (contract.RestoreContextMaterializationFactV1, error) {
	return s.store.InspectRestoreContextMaterializationV1(ctx, ref)
}

func (s *RestoreContextMaterializationServiceV1) InspectRestoreContextMaterializationByTargetV1(ctx context.Context, target contract.RestoreContextTargetBindingV1) (contract.RestoreContextMaterializationFactV1, error) {
	return s.store.InspectRestoreContextMaterializationByTargetV1(ctx, target)
}

func sameSourceFramesV1(s1, s2 []contract.ContextFrame) bool {
	if len(s1) != len(s2) {
		return false
	}
	for index := range s1 {
		if !reflect.DeepEqual(s1[index], s2[index]) {
			return false
		}
	}
	return true
}

func cloneContextContentRefV1(value *contract.ContentRef) *contract.ContentRef {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func minimumRestoreContextTimeV1(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func compareRestoreContextFactRefV1(left, right contract.FactRef) int {
	if result := cmp.Compare(left.ID, right.ID); result != 0 {
		return result
	}
	if result := cmp.Compare(left.Revision, right.Revision); result != 0 {
		return result
	}
	return cmp.Compare(left.Digest, right.Digest)
}

func restoreContextNilV1(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func isRestoreContextNotFoundV1(err error) bool { return errors.Is(err, contract.ErrNotFound) }

var _ contextports.RestoreContextMaterializationOwnerPortV1 = (*RestoreContextMaterializationServiceV1)(nil)

package testkit

// This file contains isolated, in-memory test fixtures. They are not a
// production metadata backend, State Plane root, persistence contract or SLA.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

type MetadataOperationV1 string

const (
	MetadataResolveSourceV1  MetadataOperationV1 = "resolve_source"
	MetadataReadFrameV1      MetadataOperationV1 = "read_frame"
	MetadataReadManifestV1   MetadataOperationV1 = "read_manifest"
	MetadataReadGenerationV1 MetadataOperationV1 = "read_generation"
	MetadataReadPointerV1    MetadataOperationV1 = "read_pointer"
)

type scopedFactKeyV1 struct {
	Scope contract.Digest
	Ref   contract.FactRef
}

type pointerKeyV1 struct {
	Scope   contract.Digest
	RunID   string
	Session contract.FactRef
	Turn    uint32
}

type MetadataStoreV1 struct {
	mu          sync.RWMutex
	bindings    map[contract.ContextParentFrameApplicabilitySourceCoordinateV1][]contract.ContextParentFrameSourceBindingV1
	frames      map[scopedFactKeyV1]contract.ContextFrame
	manifests   map[scopedFactKeyV1]contract.ContextManifest
	generations map[scopedFactKeyV1]contract.ContextGeneration
	pointers    map[pointerKeyV1]contract.ContextGenerationCurrentPointerV1
	unavailable map[MetadataOperationV1]bool
}

func NewMetadataStoreV1() *MetadataStoreV1 {
	return &MetadataStoreV1{
		bindings:    make(map[contract.ContextParentFrameApplicabilitySourceCoordinateV1][]contract.ContextParentFrameSourceBindingV1),
		frames:      make(map[scopedFactKeyV1]contract.ContextFrame),
		manifests:   make(map[scopedFactKeyV1]contract.ContextManifest),
		generations: make(map[scopedFactKeyV1]contract.ContextGeneration),
		pointers:    make(map[pointerKeyV1]contract.ContextGenerationCurrentPointerV1),
		unavailable: make(map[MetadataOperationV1]bool),
	}
}

var (
	_ contextports.ContextParentFrameSourceBindingReaderV1 = (*MetadataStoreV1)(nil)
	_ contextports.ContextFrameMetadataReaderV1            = (*MetadataStoreV1)(nil)
	_ contextports.ContextManifestMetadataReaderV1         = (*MetadataStoreV1)(nil)
	_ contextports.ContextGenerationMetadataReaderV1       = (*MetadataStoreV1)(nil)
	_ contextports.ContextGenerationCurrentPointerReaderV1 = (*MetadataStoreV1)(nil)
)

func (s *MetadataStoreV1) SetUnavailable(operation MetadataOperationV1, unavailable bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unavailable[operation] = unavailable
}

func (s *MetadataStoreV1) PutSourceBinding(binding contract.ContextParentFrameSourceBindingV1) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	copy, err := cloneJSONV1(binding)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bindings[binding.Source] = []contract.ContextParentFrameSourceBindingV1{copy}
	return nil
}

// AddSourceBindingCandidate intentionally permits multiple values for one
// exact source coordinate so conflict and cross-scope ambiguity paths can be
// tested. Production stores must never choose an arbitrary candidate.
func (s *MetadataStoreV1) AddSourceBindingCandidate(source contract.ContextParentFrameApplicabilitySourceCoordinateV1, binding contract.ContextParentFrameSourceBindingV1) error {
	if source.Validate() != nil {
		return fmt.Errorf("%w: source binding candidate key", contract.ErrInvalid)
	}
	copy, err := cloneJSONV1(binding)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bindings[source] = append(s.bindings[source], copy)
	return nil
}

func (s *MetadataStoreV1) PutFrame(frame contract.ContextFrame) error {
	if err := frame.Validate(); err != nil {
		return err
	}
	digest, err := frame.DigestValue()
	if err != nil {
		return err
	}
	copy, err := cloneJSONV1(frame)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frames[scopedFactKeyV1{Scope: frame.Execution.ScopeDigest, Ref: contract.FactRef{ID: frame.ID, Revision: frame.Revision, Digest: digest}}] = copy
	return nil
}

func (s *MetadataStoreV1) PutManifest(manifest contract.ContextManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	digest, err := manifest.DigestValue()
	if err != nil {
		return err
	}
	copy, err := cloneJSONV1(manifest)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manifests[scopedFactKeyV1{Scope: manifest.Execution.ScopeDigest, Ref: contract.FactRef{ID: manifest.ID, Revision: manifest.Revision, Digest: digest}}] = copy
	return nil
}

func (s *MetadataStoreV1) PutGeneration(scope contract.Digest, generation contract.ContextGeneration) error {
	if scope.Validate() != nil || generation.Validate() != nil {
		return fmt.Errorf("%w: generation metadata", contract.ErrInvalid)
	}
	digest, err := generation.DigestValue()
	if err != nil {
		return err
	}
	copy, err := cloneJSONV1(generation)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generations[scopedFactKeyV1{Scope: scope, Ref: contract.FactRef{ID: generation.ID, Revision: generation.Revision, Digest: digest}}] = copy
	return nil
}

func (s *MetadataStoreV1) PutCurrentGenerationPointer(pointer contract.ContextGenerationCurrentPointerV1) error {
	if err := pointer.Validate(); err != nil {
		return err
	}
	copy, err := cloneJSONV1(pointer)
	if err != nil {
		return err
	}
	key := pointerKeyV1{Scope: pointer.ExecutionScopeDigest, RunID: pointer.RunID, Session: pointer.SessionRef, Turn: pointer.Turn}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pointers[key] = copy
	return nil
}

func (s *MetadataStoreV1) ResolveExactSourceBinding(ctx context.Context, source contract.ContextParentFrameApplicabilitySourceCoordinateV1) (contract.ContextParentFrameSourceBindingV1, error) {
	if err := checkContextV1(ctx); err != nil {
		return contract.ContextParentFrameSourceBindingV1{}, err
	}
	if source.Validate() != nil {
		return contract.ContextParentFrameSourceBindingV1{}, fmt.Errorf("%w: resolve exact source binding", contract.ErrInvalid)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.unavailable[MetadataResolveSourceV1] {
		return contract.ContextParentFrameSourceBindingV1{}, fmt.Errorf("%w: resolve exact source binding", contract.ErrUnavailable)
	}
	values := s.bindings[source]
	if len(values) > 1 {
		return contract.ContextParentFrameSourceBindingV1{}, fmt.Errorf("%w: ambiguous exact source binding", contract.ErrConflict)
	}
	if len(values) == 0 {
		for candidate := range s.bindings {
			if candidate.ID == source.ID {
				return contract.ContextParentFrameSourceBindingV1{}, fmt.Errorf("%w: source ID revision/digest or scope drift", contract.ErrConflict)
			}
		}
		return contract.ContextParentFrameSourceBindingV1{}, fmt.Errorf("%w: exact source binding", contract.ErrNotFound)
	}
	return cloneJSONV1(values[0])
}

func (s *MetadataStoreV1) FrameByExactRef(ctx context.Context, ref contract.FactRef, scope contract.Digest) (contract.ContextFrame, error) {
	if err := checkContextV1(ctx); err != nil {
		return contract.ContextFrame{}, err
	}
	if ref.Validate() != nil || scope.Validate() != nil {
		return contract.ContextFrame{}, fmt.Errorf("%w: frame exact read", contract.ErrInvalid)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.unavailable[MetadataReadFrameV1] {
		return contract.ContextFrame{}, fmt.Errorf("%w: frame exact read", contract.ErrUnavailable)
	}
	key := scopedFactKeyV1{Scope: scope, Ref: ref}
	value, ok := s.frames[key]
	if !ok {
		if scopedFactIDExistsV1(s.frames, ref.ID) {
			return contract.ContextFrame{}, fmt.Errorf("%w: frame revision/digest/scope drift", contract.ErrConflict)
		}
		return contract.ContextFrame{}, fmt.Errorf("%w: exact frame", contract.ErrNotFound)
	}
	return cloneJSONV1(value)
}

func (s *MetadataStoreV1) ManifestByExactRef(ctx context.Context, ref contract.FactRef, scope contract.Digest) (contract.ContextManifest, error) {
	if err := checkContextV1(ctx); err != nil {
		return contract.ContextManifest{}, err
	}
	if ref.Validate() != nil || scope.Validate() != nil {
		return contract.ContextManifest{}, fmt.Errorf("%w: manifest exact read", contract.ErrInvalid)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.unavailable[MetadataReadManifestV1] {
		return contract.ContextManifest{}, fmt.Errorf("%w: manifest exact read", contract.ErrUnavailable)
	}
	key := scopedFactKeyV1{Scope: scope, Ref: ref}
	value, ok := s.manifests[key]
	if !ok {
		if scopedFactIDExistsV1(s.manifests, ref.ID) {
			return contract.ContextManifest{}, fmt.Errorf("%w: manifest revision/digest/scope drift", contract.ErrConflict)
		}
		return contract.ContextManifest{}, fmt.Errorf("%w: exact manifest", contract.ErrNotFound)
	}
	return cloneJSONV1(value)
}

func (s *MetadataStoreV1) GenerationByExactRef(ctx context.Context, ref contract.FactRef, scope contract.Digest) (contract.ContextGeneration, error) {
	if err := checkContextV1(ctx); err != nil {
		return contract.ContextGeneration{}, err
	}
	if ref.Validate() != nil || scope.Validate() != nil {
		return contract.ContextGeneration{}, fmt.Errorf("%w: generation exact read", contract.ErrInvalid)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.unavailable[MetadataReadGenerationV1] {
		return contract.ContextGeneration{}, fmt.Errorf("%w: generation exact read", contract.ErrUnavailable)
	}
	key := scopedFactKeyV1{Scope: scope, Ref: ref}
	value, ok := s.generations[key]
	if !ok {
		if scopedFactIDExistsV1(s.generations, ref.ID) {
			return contract.ContextGeneration{}, fmt.Errorf("%w: generation revision/digest/scope drift", contract.ErrConflict)
		}
		return contract.ContextGeneration{}, fmt.Errorf("%w: exact generation", contract.ErrNotFound)
	}
	return cloneJSONV1(value)
}

func (s *MetadataStoreV1) InspectCurrentGenerationPointer(ctx context.Context, request contract.ContextGenerationCurrentPointerRequestV1) (contract.ContextGenerationCurrentPointerV1, error) {
	if err := checkContextV1(ctx); err != nil {
		return contract.ContextGenerationCurrentPointerV1{}, err
	}
	if request.Validate() != nil {
		return contract.ContextGenerationCurrentPointerV1{}, fmt.Errorf("%w: current generation pointer read", contract.ErrInvalid)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.unavailable[MetadataReadPointerV1] {
		return contract.ContextGenerationCurrentPointerV1{}, fmt.Errorf("%w: current generation pointer read", contract.ErrUnavailable)
	}
	key := pointerKeyV1{Scope: request.ExecutionScopeDigest, RunID: request.RunID, Session: request.SessionRef, Turn: request.Turn}
	value, ok := s.pointers[key]
	if !ok {
		return contract.ContextGenerationCurrentPointerV1{}, fmt.Errorf("%w: current generation pointer", contract.ErrNotFound)
	}
	return cloneJSONV1(value)
}

func scopedFactIDExistsV1[T any](values map[scopedFactKeyV1]T, id string) bool {
	for key := range values {
		if key.Ref.ID == id {
			return true
		}
	}
	return false
}

func cloneJSONV1[T any](value T) (T, error) {
	var result T
	payload, err := json.Marshal(value)
	if err != nil {
		return result, fmt.Errorf("%w: clone test metadata", contract.ErrInvalid)
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return result, fmt.Errorf("%w: clone test metadata", contract.ErrInvalid)
	}
	return result, nil
}

func checkContextV1(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	return ctx.Err()
}

type MutableReferenceStoreV1 struct {
	mu          sync.RWMutex
	content     map[string][]byte
	unavailable bool
}

func NewMutableReferenceStoreV1() *MutableReferenceStoreV1 {
	return &MutableReferenceStoreV1{content: make(map[string][]byte)}
}

func (s *MutableReferenceStoreV1) SetUnavailable(unavailable bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unavailable = unavailable
}

func (s *MutableReferenceStoreV1) Put(value []byte) (contract.ContentRef, error) {
	if len(value) == 0 {
		return contract.ContentRef{}, fmt.Errorf("%w: empty test content", contract.ErrInvalid)
	}
	digest := contract.DigestBytes(value)
	ref := contract.ContentRef{Ref: string(digest), Digest: digest, Length: uint64(len(value))}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.content[ref.Ref]; ok && !bytes.Equal(current, value) {
		return contract.ContentRef{}, fmt.Errorf("%w: test content collision", contract.ErrConflict)
	}
	s.content[ref.Ref] = append([]byte(nil), value...)
	return ref, nil
}

func (s *MutableReferenceStoreV1) Get(ref contract.ContentRef) ([]byte, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.unavailable {
		return nil, fmt.Errorf("%w: test content read", contract.ErrUnavailable)
	}
	value, ok := s.content[ref.Ref]
	if !ok {
		return nil, fmt.Errorf("%w: test content read", contract.ErrNotFound)
	}
	return append([]byte(nil), value...), nil
}

func (s *MutableReferenceStoreV1) Corrupt(ref contract.ContentRef, changed []byte) error {
	if ref.Validate() != nil || len(changed) == 0 {
		return fmt.Errorf("%w: corrupt test content", contract.ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.content[ref.Ref]; !ok {
		return fmt.Errorf("%w: corrupt test content", contract.ErrNotFound)
	}
	s.content[ref.Ref] = append([]byte(nil), changed...)
	return nil
}

package assemblypublication

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type stagedPublicationV2 struct {
	generation *assemblycontract.AssemblyGenerationV1
	manifest   *assemblycontract.AssemblyManifestV1
	graph      *assemblycontract.CompiledHarnessGraphV1
	handoff    *assemblycontract.AssemblyHandoffV1
}

// MemoryStoreV2 is a reference backend for the publication contract. Its
// mutex models the required atomic commit marker/current transaction; it is
// not a durable production State Plane.
type MemoryStoreV2 struct {
	mu        sync.RWMutex
	staged    map[string]stagedPublicationV2
	committed map[string]assemblycontract.AssemblyPublicationBundleV2
	commits   map[string]assemblycontract.AssemblyPublicationCurrentV2
	current   map[string]assemblycontract.AssemblyPublicationCurrentV2

	writeSequence int
	afterWrite    func(int) error
}

var _ OwnerStoreV2 = (*MemoryStoreV2)(nil)

func NewMemoryStoreV2() *MemoryStoreV2 {
	return &MemoryStoreV2{
		staged:    map[string]stagedPublicationV2{},
		committed: map[string]assemblycontract.AssemblyPublicationBundleV2{},
		commits:   map[string]assemblycontract.AssemblyPublicationCurrentV2{},
		current:   map[string]assemblycontract.AssemblyPublicationCurrentV2{},
	}
}

func (s *MemoryStoreV2) StageGenerationV2(ctx context.Context, publicationID string, value assemblycontract.AssemblyGenerationV1) error {
	if ctx == nil {
		return invalidStore("staging context is required")
	}
	expectedID, err := assemblycontract.DeriveAssemblyPublicationIDV2(value.InputDigest, value.GenerationID)
	if err != nil || publicationID != expectedID {
		return conflictStore("Generation does not belong to the requested PublicationID")
	}
	return s.stage(publicationID, func(candidate *stagedPublicationV2) (bool, error) {
		if candidate.generation != nil {
			if candidate.generation.Digest != value.Digest {
				return false, conflictStore("staged Generation content drifted")
			}
			return false, nil
		}
		copyValue := clone(value)
		candidate.generation = &copyValue
		return true, nil
	})
}

func (s *MemoryStoreV2) StageManifestV2(ctx context.Context, publicationID string, value assemblycontract.AssemblyManifestV1) error {
	if ctx == nil || publicationID == "" {
		return invalidStore("Manifest staging identity and context are required")
	}
	return s.stage(publicationID, func(candidate *stagedPublicationV2) (bool, error) {
		if candidate.manifest != nil {
			if candidate.manifest.Digest != value.Digest {
				return false, conflictStore("staged Manifest content drifted")
			}
			return false, nil
		}
		copyValue := clone(value)
		candidate.manifest = &copyValue
		return true, nil
	})
}

func (s *MemoryStoreV2) StageGraphV2(ctx context.Context, publicationID string, value assemblycontract.CompiledHarnessGraphV1) error {
	if ctx == nil || publicationID == "" {
		return invalidStore("Graph staging identity and context are required")
	}
	return s.stage(publicationID, func(candidate *stagedPublicationV2) (bool, error) {
		if candidate.graph != nil {
			if candidate.graph.Digest != value.Digest {
				return false, conflictStore("staged Graph content drifted")
			}
			return false, nil
		}
		copyValue := clone(value)
		candidate.graph = &copyValue
		return true, nil
	})
}

func (s *MemoryStoreV2) StageHandoffV2(ctx context.Context, publicationID string, value assemblycontract.AssemblyHandoffV1) error {
	if ctx == nil || publicationID == "" {
		return invalidStore("Handoff staging identity and context are required")
	}
	return s.stage(publicationID, func(candidate *stagedPublicationV2) (bool, error) {
		if candidate.handoff != nil {
			if candidate.handoff.Digest != value.Digest {
				return false, conflictStore("staged Handoff content drifted")
			}
			return false, nil
		}
		copyValue := clone(value)
		candidate.handoff = &copyValue
		return true, nil
	})
}

func (s *MemoryStoreV2) stage(publicationID string, update func(*stagedPublicationV2) (bool, error)) error {
	if s == nil {
		return invalidStore("Assembly publication owner store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if committed, ok := s.committed[publicationID]; ok {
		candidate := s.staged[publicationID]
		if candidate.generation == nil {
			generation := clone(committed.Generation)
			manifest := clone(committed.Manifest)
			graph := clone(committed.Graph)
			handoff := clone(committed.Handoff)
			candidate = stagedPublicationV2{generation: &generation, manifest: &manifest, graph: &graph, handoff: &handoff}
		}
		_, err := update(&candidate)
		return err
	}
	candidate := s.staged[publicationID]
	written, err := update(&candidate)
	if err != nil {
		return err
	}
	if !written {
		return nil
	}
	s.staged[publicationID] = candidate
	return s.afterWriteLocked()
}

func (s *MemoryStoreV2) InspectStagedPublicationV2(ctx context.Context, publicationID string) (StagedPublicationInspectionV2, error) {
	if s == nil || ctx == nil || publicationID == "" {
		return StagedPublicationInspectionV2{}, invalidStore("staged publication Inspect requires store, context and identity")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	candidate, ok := s.staged[publicationID]
	if !ok {
		if committed, exists := s.committed[publicationID]; exists {
			return inspectionFromBundle(committed), nil
		}
		return StagedPublicationInspectionV2{}, notFoundStore("staged publication is unavailable")
	}
	result := StagedPublicationInspectionV2{PublicationID: publicationID}
	if candidate.generation != nil {
		result.GenerationDigest = candidate.generation.Digest
	}
	if candidate.manifest != nil {
		result.ManifestDigest = candidate.manifest.Digest
	}
	if candidate.graph != nil {
		result.GraphDigest = candidate.graph.Digest
	}
	if candidate.handoff != nil {
		result.HandoffDigest = candidate.handoff.Digest
	}
	return result, nil
}

func (s *MemoryStoreV2) CommitPublicationCurrentV2(ctx context.Context, request CommitPublicationCurrentRequestV2) (assemblycontract.AssemblyPublicationCurrentV2, error) {
	if s == nil || ctx == nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, invalidStore("publication commit requires store and context")
	}
	if err := request.Expected.Validate(); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	if err := request.Bundle.Validate(); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	if err := request.Current.ValidateAt(time.Unix(0, request.Current.CheckedUnixNano)); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	if request.Current.ScopeRef != request.Bundle.Publication.ScopeRef || request.Current.Publication != (assemblycontract.AssemblyPublicationRefV2{PublicationID: request.Bundle.Publication.PublicationID, Revision: request.Bundle.Publication.Revision, Digest: request.Bundle.Publication.Digest}) || request.Current.InputDigest != request.Bundle.Publication.InputDigest || request.Current.Artifacts != request.Bundle.Publication.Artifacts {
		return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("publication current does not bind the staged bundle")
	}
	expectedRevision := core.Revision(1)
	if request.Expected.Exists {
		expectedRevision = request.Expected.Revision + 1
	}
	if request.Current.Revision != expectedRevision || expectedRevision == 0 {
		return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("publication current successor revision is invalid")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	predecessor, exists := s.current[request.Current.ScopeRef]
	if request.Expected.Exists != exists || (exists && (predecessor.Revision != request.Expected.Revision || predecessor.Digest != request.Expected.Digest)) {
		return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("publication current predecessor changed before CAS")
	}
	if committed, exists := s.committed[request.Bundle.Publication.PublicationID]; exists {
		if committed.Publication.Digest != request.Bundle.Publication.Digest {
			return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("PublicationID already carries different immutable content")
		}
		return assemblycontract.AssemblyPublicationCurrentV2{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "create-once publication was already committed")
	}
	staged, ok := s.staged[request.Bundle.Publication.PublicationID]
	if !ok || staged.generation == nil || staged.manifest == nil || staged.graph == nil || staged.handoff == nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReadyEvidenceIncomplete, "publication commit cannot expose a partial staged set")
	}
	if staged.generation.Digest != request.Bundle.Generation.Digest || staged.manifest.Digest != request.Bundle.Manifest.Digest || staged.graph.Digest != request.Bundle.Graph.Digest || staged.handoff.Digest != request.Bundle.Handoff.Digest {
		return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("staged publication content drifted before commit")
	}

	// This critical section is the single external visibility barrier: the
	// historical commit marker and scope current appear together.
	s.committed[request.Bundle.Publication.PublicationID] = clone(request.Bundle)
	s.commits[request.Bundle.Publication.PublicationID] = clone(request.Current)
	s.current[request.Current.ScopeRef] = clone(request.Current)
	delete(s.staged, request.Bundle.Publication.PublicationID)
	if err := s.afterWriteLocked(); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	return clone(request.Current), nil
}

// InspectCommittedPublicationCurrentV2 returns the exact current committed by
// one publication even after the scope current has advanced. It is an
// owner-recovery read, not the consumer-facing latest-current reader.
func (s *MemoryStoreV2) InspectCommittedPublicationCurrentV2(ctx context.Context, ref assemblycontract.AssemblyPublicationRefV2) (assemblycontract.AssemblyPublicationCurrentV2, error) {
	if s == nil || ctx == nil || ref.Validate() != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, invalidStore("committed publication current Inspect requires an exact ref")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	bundle, ok := s.committed[ref.PublicationID]
	if !ok {
		return assemblycontract.AssemblyPublicationCurrentV2{}, notFoundStore("publication commit current is unavailable")
	}
	if bundle.Publication.Digest != ref.Digest || bundle.Publication.Revision != ref.Revision {
		return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("publication commit current ref drifted")
	}
	current, ok := s.commits[ref.PublicationID]
	if !ok {
		return assemblycontract.AssemblyPublicationCurrentV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonReadyEvidenceIncomplete, "publication commit marker lacks its exact current")
	}
	return clone(current), nil
}

func (s *MemoryStoreV2) InspectHistoricalPublicationV2(ctx context.Context, ref assemblycontract.AssemblyPublicationRefV2) (assemblycontract.AssemblyPublicationBundleV2, error) {
	if s == nil || ctx == nil || ref.PublicationID == "" || ref.Revision != 1 || ref.Digest.Validate() != nil {
		return assemblycontract.AssemblyPublicationBundleV2{}, invalidStore("historical publication Inspect requires an exact ref")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	bundle, ok := s.committed[ref.PublicationID]
	if !ok {
		return assemblycontract.AssemblyPublicationBundleV2{}, notFoundStore("historical publication is not committed")
	}
	if bundle.Publication.Revision != ref.Revision || bundle.Publication.Digest != ref.Digest {
		return assemblycontract.AssemblyPublicationBundleV2{}, conflictStore("historical publication exact ref drifted")
	}
	return clone(bundle), nil
}

func (s *MemoryStoreV2) InspectCurrentPublicationV2(ctx context.Context, scopeRef string) (assemblycontract.AssemblyPublicationCurrentV2, error) {
	if s == nil || ctx == nil || scopeRef == "" {
		return assemblycontract.AssemblyPublicationCurrentV2{}, invalidStore("publication current Inspect requires store, context and scope")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, ok := s.current[scopeRef]
	if !ok {
		return assemblycontract.AssemblyPublicationCurrentV2{}, notFoundStore("publication current is unavailable")
	}
	return clone(current), nil
}

func (s *MemoryStoreV2) afterWriteLocked() error {
	s.writeSequence++
	if s.afterWrite == nil {
		return nil
	}
	return s.afterWrite(s.writeSequence)
}

func inspectionFromBundle(value assemblycontract.AssemblyPublicationBundleV2) StagedPublicationInspectionV2 {
	return StagedPublicationInspectionV2{PublicationID: value.Publication.PublicationID, GenerationDigest: value.Generation.Digest, ManifestDigest: value.Manifest.Digest, GraphDigest: value.Graph.Digest, HandoffDigest: value.Handoff.Digest}
}

func invalidStore(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}

func conflictStore(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, message)
}

func notFoundStore(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, message)
}

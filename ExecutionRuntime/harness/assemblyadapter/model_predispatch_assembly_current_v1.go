package assemblyadapter

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

const (
	ModelPreDispatchAssemblyPublisherContractVersionV1 = "praxis.harness.model-predispatch-assembly-current-publisher/v1"
	modelPreDispatchAssemblySemanticDomainV1           = "praxis.harness.model-predispatch-assembly-current-semantic/v1"
	modelPreDispatchAssemblyCurrentnessDomainV1        = "praxis.harness.model-predispatch-assembly-current-currentness/v1"
	modelPreDispatchAssemblyHandoffDomainV1            = "praxis.harness.model-predispatch-assembly-handoff-current/v1"
	modelPreDispatchAssemblyRecoveryHardCapV1          = 5 * time.Second
)

// ModelPreDispatchAssemblyHandoffCurrentProjectionV1 is the Harness-owned
// current view over one immutable Assembly Handoff. It is not a Runtime Fact.
type ModelPreDispatchAssemblyHandoffCurrentProjectionV1 struct {
	ContractVersion   string                                          `json:"contract_version"`
	Ref               runtimeports.ModelPreDispatchAssemblyExactRefV1 `json:"ref"`
	CurrentnessDigest core.Digest                                     `json:"currentness_digest"`
	ProjectionDigest  core.Digest                                     `json:"projection_digest"`
	CheckedUnixNano   int64                                           `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                                           `json:"expires_unix_nano"`
}

func (p ModelPreDispatchAssemblyHandoffCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest(modelPreDispatchAssemblyHandoffDomainV1, ModelPreDispatchAssemblyPublisherContractVersionV1, "ModelPreDispatchAssemblyHandoffCurrentProjectionV1", copy)
}

func (p ModelPreDispatchAssemblyHandoffCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ModelPreDispatchAssemblyPublisherContractVersionV1 || p.Ref.Validate() != nil || p.CurrentnessDigest.Validate() != nil || p.ProjectionDigest.Validate() != nil || p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Assembly Handoff current projection is incomplete")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Assembly Handoff current projection digest drifted")
	}
	return nil
}

func (p ModelPreDispatchAssemblyHandoffCurrentProjectionV1) ValidateCurrent(expected runtimeports.ModelPreDispatchAssemblyExactRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Assembly Handoff current validation requires time")
	}
	if p.Ref != expected {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Assembly Handoff current Ref drifted")
	}
	if now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Assembly Handoff current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Assembly Handoff current projection expired")
	}
	return nil
}

func SealModelPreDispatchAssemblyHandoffCurrentProjectionV1(p ModelPreDispatchAssemblyHandoffCurrentProjectionV1) (ModelPreDispatchAssemblyHandoffCurrentProjectionV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != ModelPreDispatchAssemblyPublisherContractVersionV1 {
		return ModelPreDispatchAssemblyHandoffCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonComponentMismatch, "Assembly Handoff current contract version drifted")
	}
	p.ContractVersion = ModelPreDispatchAssemblyPublisherContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return ModelPreDispatchAssemblyHandoffCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

// ModelPreDispatchAssemblyHandoffCurrentReaderV1 is implemented by the
// Harness Handoff owner. It exposes no publish or CAS method.
type ModelPreDispatchAssemblyHandoffCurrentReaderV1 interface {
	InspectCurrentModelPreDispatchAssemblyHandoffV1(context.Context, runtimeports.ModelPreDispatchAssemblyExactRefV1) (ModelPreDispatchAssemblyHandoffCurrentProjectionV1, error)
}

// ModelPreDispatchAssemblyPublishRequestV1 contains only exact coordinates.
// Semantic and currentness digests, clocks and revisions are owner-produced.
type ModelPreDispatchAssemblyPublishRequestV1 struct {
	ContractVersion  string                                            `json:"contract_version"`
	ExpectedCurrent  runtimeports.ModelPreDispatchAssemblyCurrentRefV1 `json:"expected_current"`
	VerifiedAssembly ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1 `json:"verified_assembly"`
	Association      runtimeports.GenerationBindingAssociationRefV1    `json:"association"`
	Generation       runtimeports.GenerationArtifactRefV1              `json:"generation"`
	Handoff          runtimeports.ModelPreDispatchAssemblyExactRefV1   `json:"handoff"`
	Manifest         runtimeports.ModelPreDispatchAssemblyExactRefV1   `json:"manifest"`
	Conformance      runtimeports.ModelPreDispatchAssemblyExactRefV1   `json:"conformance"`
	ToolSurface      runtimeports.ModelPreDispatchAssemblyExactRefV1   `json:"tool_surface"`
	ProfileDigest    core.Digest                                       `json:"profile_digest"`
	RegistrySnapshot runtimeports.RegistrySnapshotRefV1                `json:"registry_snapshot"`
}

func (r ModelPreDispatchAssemblyPublishRequestV1) Validate() error {
	if r.ContractVersion != ModelPreDispatchAssemblyPublisherContractVersionV1 || r.VerifiedAssembly.Validate() != nil || r.Association.Validate() != nil || r.Generation.Validate() != nil || r.Handoff.Validate() != nil || r.Manifest.Validate() != nil || r.Conformance.Validate() != nil || r.ToolSurface.Validate() != nil || r.ProfileDigest.Validate() != nil || r.RegistrySnapshot.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model pre-dispatch Assembly publish request is incomplete")
	}
	if r.ExpectedCurrent != (runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}) {
		return r.ExpectedCurrent.Validate()
	}
	return nil
}

// ModelPreDispatchAssemblyCurrentStoreV1 is the Harness owner's single
// linearization surface. Historical reads do not imply currentness.
type ModelPreDispatchAssemblyCurrentStoreV1 interface {
	runtimeports.ModelPreDispatchAssemblyCurrentReaderV1
	CompareAndSwapModelPreDispatchAssemblyCurrentV1(context.Context, runtimeports.ModelPreDispatchAssemblyCurrentRefV1, runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error)
	InspectHistoricalModelPreDispatchAssemblyV1(context.Context, runtimeports.ModelPreDispatchAssemblyCurrentRefV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error)
}

type InMemoryModelPreDispatchAssemblyCurrentStoreV1 struct {
	mu         sync.RWMutex
	history    map[string]map[core.Revision]runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1
	current    map[string]runtimeports.ModelPreDispatchAssemblyCurrentRefV1
	watermarks map[string]map[core.Digest]struct{}
	clock      func() time.Time
}

func NewInMemoryModelPreDispatchAssemblyCurrentStoreV1(clock func() time.Time) (*InMemoryModelPreDispatchAssemblyCurrentStoreV1, error) {
	if clock == nil {
		return nil, componentMissingModelPreDispatchAssemblyV1("Harness Assembly current Store clock is required")
	}
	return &InMemoryModelPreDispatchAssemblyCurrentStoreV1{
		history:    map[string]map[core.Revision]runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{},
		current:    map[string]runtimeports.ModelPreDispatchAssemblyCurrentRefV1{},
		watermarks: map[string]map[core.Digest]struct{}{},
		clock:      clock,
	}, nil
}

func (s *InMemoryModelPreDispatchAssemblyCurrentStoreV1) CompareAndSwapModelPreDispatchAssemblyCurrentV1(ctx context.Context, expected runtimeports.ModelPreDispatchAssemblyCurrentRefV1, next runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	if s == nil || s.clock == nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, componentMissingModelPreDispatchAssemblyV1("Harness Assembly current Store is unavailable")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if err := next.Validate(); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if expected != (runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}) {
		if err := expected.Validate(); err != nil {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	now := s.clock()
	if now.IsZero() {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Harness Assembly current Store clock is unavailable")
	}
	if err := next.ValidateCurrent(next.Ref, now); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}

	if revisions := s.history[next.Ref.ID]; revisions != nil {
		if stored, exists := revisions[next.Ref.Revision]; exists {
			if stored != next {
				return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Harness Assembly current revision already stores different content")
			}
			if s.current[next.Ref.ID] != next.Ref {
				return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "historical Harness Assembly revision is no longer current")
			}
			return stored, nil
		}
	}

	current, exists := s.current[next.Ref.ID]
	if !exists {
		if expected != (runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}) || next.Ref.Revision != 1 {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Harness Assembly initial current CAS conflicted")
		}
	} else if expected != current || next.Ref.Revision != current.Revision+1 {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Harness Assembly full current Ref CAS conflicted")
	}
	if _, seen := s.watermarks[next.Ref.ID][next.Ref.WatermarkDigest]; seen {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness Assembly current rejected ABA watermark reuse")
	}
	if s.history[next.Ref.ID] == nil {
		s.history[next.Ref.ID] = map[core.Revision]runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}
	}
	if s.watermarks[next.Ref.ID] == nil {
		s.watermarks[next.Ref.ID] = map[core.Digest]struct{}{}
	}
	s.history[next.Ref.ID][next.Ref.Revision] = next
	s.current[next.Ref.ID] = next.Ref
	s.watermarks[next.Ref.ID][next.Ref.WatermarkDigest] = struct{}{}
	return next, nil
}

func (s *InMemoryModelPreDispatchAssemblyCurrentStoreV1) InspectHistoricalModelPreDispatchAssemblyV1(ctx context.Context, ref runtimeports.ModelPreDispatchAssemblyCurrentRefV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	if s == nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, componentMissingModelPreDispatchAssemblyV1("Harness Assembly historical Store is unavailable")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	s.mu.RLock()
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		s.mu.RUnlock()
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	revisions := s.history[ref.ID]
	stored, exists := revisions[ref.Revision]
	if !exists {
		s.mu.RUnlock()
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Harness Assembly historical revision is absent")
	}
	if stored.Ref != ref {
		s.mu.RUnlock()
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness Assembly historical exact Ref drifted")
	}
	s.mu.RUnlock()
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	return stored, nil
}

func (s *InMemoryModelPreDispatchAssemblyCurrentStoreV1) InspectCurrentModelPreDispatchAssemblyV1(ctx context.Context, ref runtimeports.ModelPreDispatchAssemblyCurrentRefV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	if s == nil || s.clock == nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, componentMissingModelPreDispatchAssemblyV1("Harness Assembly current Reader is unavailable")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	s.mu.RLock()
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		s.mu.RUnlock()
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if current, exists := s.current[ref.ID]; !exists {
		s.mu.RUnlock()
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Harness Assembly current is absent")
	} else if current != ref {
		s.mu.RUnlock()
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Harness Assembly exact historical Ref is not current")
	}
	stored, exists := s.history[ref.ID][ref.Revision]
	if !exists || stored.Ref != ref {
		s.mu.RUnlock()
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness Assembly current index and history drifted")
	}
	s.mu.RUnlock()
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	now := s.clock()
	if now.IsZero() {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Harness Assembly current Reader clock is unavailable")
	}
	if err := stored.ValidateCurrent(ref, now); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	s.mu.RLock()
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		s.mu.RUnlock()
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	current := s.current[ref.ID]
	confirmed, exists := s.history[ref.ID][ref.Revision]
	s.mu.RUnlock()
	if current != ref || !exists || confirmed != stored {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness Assembly current changed during exact read")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	return stored, nil
}

type ModelPreDispatchAssemblyCurrentPublisherV1 struct {
	store        ModelPreDispatchAssemblyCurrentStoreV1
	verified     ModelPreDispatchVerifiedAssemblyOwnerCurrentReaderV1
	associations runtimeports.GenerationBindingAssociationCurrentReaderV1
	handoffs     ModelPreDispatchAssemblyHandoffCurrentReaderV1
	tools        toolcontract.ToolSurfaceManifestCurrentReaderV1
	registries   runtimeports.RegistrySnapshotExactReaderV1
	clock        func() time.Time
}

func NewModelPreDispatchAssemblyCurrentPublisherV1(store ModelPreDispatchAssemblyCurrentStoreV1, verified ModelPreDispatchVerifiedAssemblyOwnerCurrentReaderV1, associations runtimeports.GenerationBindingAssociationCurrentReaderV1, handoffs ModelPreDispatchAssemblyHandoffCurrentReaderV1, tools toolcontract.ToolSurfaceManifestCurrentReaderV1, registries runtimeports.RegistrySnapshotExactReaderV1, clock func() time.Time) (*ModelPreDispatchAssemblyCurrentPublisherV1, error) {
	for _, dependency := range []any{store, verified, associations, handoffs, tools, registries} {
		if nilLikeModelPreDispatchAssemblyV1(dependency) {
			return nil, componentMissingModelPreDispatchAssemblyV1("Harness Assembly current Publisher Owner Reader is unavailable")
		}
	}
	if clock == nil {
		return nil, componentMissingModelPreDispatchAssemblyV1("Harness Assembly current Publisher clock is required")
	}
	return &ModelPreDispatchAssemblyCurrentPublisherV1{store: store, verified: verified, associations: associations, handoffs: handoffs, tools: tools, registries: registries, clock: clock}, nil
}

func (p *ModelPreDispatchAssemblyCurrentPublisherV1) PublishModelPreDispatchAssemblyCurrentV1(ctx context.Context, request ModelPreDispatchAssemblyPublishRequestV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	if p == nil || nilLikeModelPreDispatchAssemblyV1(p.store) || nilLikeModelPreDispatchAssemblyV1(p.verified) || nilLikeModelPreDispatchAssemblyV1(p.associations) || nilLikeModelPreDispatchAssemblyV1(p.handoffs) || nilLikeModelPreDispatchAssemblyV1(p.tools) || nilLikeModelPreDispatchAssemblyV1(p.registries) || p.clock == nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, componentMissingModelPreDispatchAssemblyV1("Harness Assembly current Publisher is unavailable")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if err := request.Validate(); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}

	nowS1, err := freshModelPreDispatchAssemblyClockV1(p.clock, time.Time{})
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	first, err := p.inspectOwnerCurrentV1(ctx, request, nowS1)
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	nowS2, err := freshModelPreDispatchAssemblyClockV1(p.clock, nowS1)
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	second, err := p.inspectOwnerCurrentV1(ctx, request, nowS2)
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(first, second) {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness Assembly Owner current inputs changed between S1 and S2")
	}
	finalNow, err := freshModelPreDispatchAssemblyClockV1(p.clock, nowS2)
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	checked := maxInt64V1(second.verified.CheckedUnixNano, second.association.Candidate.Binding.IssuedUnixNano, second.handoff.CheckedUnixNano, second.tool.CheckedUnixNano)
	expires := minInt64V1(second.verified.ExpiresUnixNano, second.association.ExpiresUnixNano, second.handoff.ExpiresUnixNano, second.tool.ExpiresUnixNano)
	if checked <= 0 || checked >= expires || finalNow.UnixNano() < checked || !finalNow.Before(time.Unix(0, expires)) {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Harness Assembly composite current window is invalid or expired")
	}
	binding := runtimeports.ModelPreDispatchAssemblyBindingSetRefV1{
		ID: second.association.Candidate.Binding.BindingSetID, Revision: second.association.Candidate.Binding.BindingSetRevision,
		Digest: second.association.Candidate.Binding.BindingSetDigest, SemanticDigest: second.association.Candidate.Binding.BindingSetSemanticDigest,
		CurrentnessDigest: second.association.Candidate.Binding.CurrentnessDigest, ProjectionDigest: second.association.Candidate.Binding.ProjectionDigest,
		ExpiresUnixNano: second.association.Candidate.Binding.ExpiresUnixNano,
	}
	semantic, err := digestModelPreDispatchAssemblySemanticV1(request, binding)
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	currentness, err := digestModelPreDispatchAssemblyCurrentnessV1(request, second)
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	revision := core.Revision(1)
	if request.ExpectedCurrent != (runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}) {
		revision = request.ExpectedCurrent.Revision + 1
	}
	next, err := runtimeports.SealModelPreDispatchAssemblyCurrentProjectionV1(runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{
		Ref: runtimeports.ModelPreDispatchAssemblyCurrentRefV1{Revision: revision}, Generation: request.Generation, Handoff: request.Handoff,
		BindingSet: binding, Manifest: request.Manifest, Conformance: request.Conformance, ToolSurface: request.ToolSurface,
		ProfileDigest: request.ProfileDigest, RegistrySnapshot: request.RegistrySnapshot, SemanticDigest: semantic, CurrentnessDigest: currentness,
		CheckedUnixNano: checked, ExpiresUnixNano: expires,
	})
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if request.ExpectedCurrent != (runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}) && request.ExpectedCurrent.ID != next.Ref.ID {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Harness Assembly successor changed stable current identity")
	}
	stored, writeErr := p.store.CompareAndSwapModelPreDispatchAssemblyCurrentV1(ctx, request.ExpectedCurrent, next)
	if writeErr == nil {
		if stored != next {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness Assembly current Store returned another projection")
		}
		return stored, nil
	}
	if !core.HasCategory(writeErr, core.ErrorUnavailable) && !core.HasCategory(writeErr, core.ErrorIndeterminate) {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, writeErr
	}
	recoveryCtx, cancelRecovery := boundedModelPreDispatchAssemblyRecoveryContextV1(ctx, finalNow, next.ExpiresUnixNano)
	defer cancelRecovery()
	historical, historicalErr := p.store.InspectHistoricalModelPreDispatchAssemblyV1(recoveryCtx, next.Ref)
	if historicalErr != nil || historical != next {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, writeErr
	}
	current, currentErr := p.store.InspectCurrentModelPreDispatchAssemblyV1(recoveryCtx, next.Ref)
	if currentErr != nil || current != next {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, writeErr
	}
	return current, nil
}

func (p *ModelPreDispatchAssemblyCurrentPublisherV1) InspectCurrentModelPreDispatchAssemblyV1(ctx context.Context, ref runtimeports.ModelPreDispatchAssemblyCurrentRefV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	if p == nil || nilLikeModelPreDispatchAssemblyV1(p.store) {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, componentMissingModelPreDispatchAssemblyV1("Harness Assembly current Publisher Reader is unavailable")
	}
	return p.store.InspectCurrentModelPreDispatchAssemblyV1(ctx, ref)
}

type modelPreDispatchAssemblyOwnerSnapshotV1 struct {
	verified    ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1
	association runtimeports.GenerationBindingAssociationFactV1
	handoff     ModelPreDispatchAssemblyHandoffCurrentProjectionV1
	tool        toolcontract.ToolSurfaceManifestCurrentProjectionV1
	registry    runtimeports.RegistrySnapshotRefV1
}

func (p *ModelPreDispatchAssemblyCurrentPublisherV1) inspectOwnerCurrentV1(ctx context.Context, request ModelPreDispatchAssemblyPublishRequestV1, now time.Time) (modelPreDispatchAssemblyOwnerSnapshotV1, error) {
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, err
	}
	verified, err := p.verified.InspectCurrentModelPreDispatchVerifiedAssemblyOwnerV1(ctx, request.VerifiedAssembly)
	if err != nil {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, err
	}
	if err := verified.ValidateCurrent(request.VerifiedAssembly, now); err != nil {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, err
	}
	compile := verified.Compile
	expectedGeneration := runtimeports.GenerationArtifactRefV1{
		ID: compile.Generation.GenerationID, Revision: compile.Generation.Revision, Digest: compile.Generation.Digest,
		InputDigest: compile.Generation.InputDigest, ManifestDigest: compile.Manifest.Digest, GraphDigest: compile.Graph.Digest, CatalogDigest: compile.Handoff.CatalogDigest,
	}
	expectedHandoff := runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: verified.Conformance.HandoffRef.ID, Revision: verified.Conformance.HandoffRef.Revision, Digest: verified.Conformance.HandoffRef.Digest}
	expectedTool := runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: compile.Manifest.Plan.ToolSurface.ID, Revision: compile.Manifest.Plan.ToolSurface.Revision, Digest: compile.Manifest.Plan.ToolSurface.Digest}
	if request.Generation != expectedGeneration || request.Handoff != expectedHandoff || request.Manifest.Revision != compile.Generation.Revision || request.Manifest.Digest != compile.Manifest.Digest || request.Conformance.Revision != compile.Generation.Revision || request.Conformance.Digest != verified.Conformance.Digest || request.ToolSurface != expectedTool || request.ProfileDigest != compile.Manifest.Plan.Profile.Digest || verified.Conformance.Association == nil || request.Association != *verified.Conformance.Association {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "raw publish coordinates drifted from verified Assembly Owner current")
	}
	association, err := p.associations.InspectCurrentGenerationBindingAssociationV1(ctx, request.Association.ID)
	if err != nil {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, err
	}
	if err := association.Validate(); err != nil {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, err
	}
	if association.RefV1() != request.Association || association.State != runtimeports.GenerationBindingAssociationActiveV1 || !now.Before(time.Unix(0, association.ExpiresUnixNano)) {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Generation-Binding association is not the exact current input")
	}
	if association.Candidate.Generation.Generation != expectedGeneration || verified.Conformance.GenerationProjectionDigest != association.Candidate.Generation.ProjectionDigest ||
		verified.Conformance.BindingSetID != association.Candidate.Binding.BindingSetID || verified.Conformance.BindingSetRevision != association.Candidate.Binding.BindingSetRevision ||
		verified.Conformance.BindingSetDigest != association.Candidate.Binding.BindingSetDigest || verified.Conformance.BindingSetSemanticDigest != association.Candidate.Binding.BindingSetSemanticDigest ||
		verified.Conformance.BindingSetCurrentnessDigest != association.Candidate.Binding.CurrentnessDigest || verified.Conformance.BindingSetProjectionDigest != association.Candidate.Binding.ProjectionDigest {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Generation-Binding association generation drifted")
	}
	if err := association.Candidate.Binding.ValidateCurrent(association.Candidate.Binding.BindingSetID, association.Candidate.Binding.BindingSetRevision, now); err != nil {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, err
	}
	handoff, err := p.handoffs.InspectCurrentModelPreDispatchAssemblyHandoffV1(ctx, request.Handoff)
	if err != nil {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, err
	}
	if err := handoff.ValidateCurrent(request.Handoff, now); err != nil {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, err
	}
	toolRef := toolcontract.ToolSurfaceManifestCurrentRefV1{
		ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1,
		ID:              compile.Manifest.Plan.ToolSurface.ID, Revision: compile.Manifest.Plan.ToolSurface.Revision, Digest: compile.Manifest.Plan.ToolSurface.Digest,
	}
	tool, err := p.tools.InspectExactToolSurfaceManifestCurrentV1(ctx, toolRef)
	if err != nil {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, err
	}
	if err := tool.ValidateCurrent(toolRef, now); err != nil {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, err
	}
	if tool.Manifest.ProfileDigest != compile.Manifest.Plan.Profile.Digest || tool.Manifest.ResolvedPlanDigest != compile.Manifest.Plan.ResolvedAgentPlan.Digest || tool.Manifest.CapabilityGrantDigest != compile.Manifest.Plan.CapabilityGrant.Digest || tool.Manifest.ExpectedInjectionDigest != compile.Manifest.Plan.ExpectedInjectionManifest.Digest || tool.Manifest.RegistrySnapshotDigest != request.RegistrySnapshot.Digest {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Surface Manifest drifted from verified Assembly plan")
	}
	registry, err := p.registries.InspectExactRegistrySnapshotV1(ctx, request.RegistrySnapshot)
	if err != nil {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, err
	}
	if err := registry.Validate(); err != nil {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, err
	}
	if registry != request.RegistrySnapshot {
		return modelPreDispatchAssemblyOwnerSnapshotV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry snapshot exact Reader drifted")
	}
	return modelPreDispatchAssemblyOwnerSnapshotV1{verified: verified, association: association, handoff: handoff, tool: tool, registry: registry}, nil
}

func digestModelPreDispatchAssemblySemanticV1(request ModelPreDispatchAssemblyPublishRequestV1, binding runtimeports.ModelPreDispatchAssemblyBindingSetRefV1) (core.Digest, error) {
	return core.CanonicalJSONDigest(modelPreDispatchAssemblySemanticDomainV1, ModelPreDispatchAssemblyPublisherContractVersionV1, "ModelPreDispatchAssemblySemanticV1", struct {
		Generation       runtimeports.GenerationArtifactRefV1                 `json:"generation"`
		Handoff          runtimeports.ModelPreDispatchAssemblyExactRefV1      `json:"handoff"`
		BindingSet       runtimeports.ModelPreDispatchAssemblyBindingSetRefV1 `json:"binding_set"`
		Manifest         runtimeports.ModelPreDispatchAssemblyExactRefV1      `json:"manifest"`
		Conformance      runtimeports.ModelPreDispatchAssemblyExactRefV1      `json:"conformance"`
		ToolSurface      runtimeports.ModelPreDispatchAssemblyExactRefV1      `json:"tool_surface"`
		ProfileDigest    core.Digest                                          `json:"profile_digest"`
		RegistrySnapshot runtimeports.RegistrySnapshotRefV1                   `json:"registry_snapshot"`
	}{request.Generation, request.Handoff, binding, request.Manifest, request.Conformance, request.ToolSurface, request.ProfileDigest, request.RegistrySnapshot})
}

func digestModelPreDispatchAssemblyCurrentnessV1(request ModelPreDispatchAssemblyPublishRequestV1, snapshot modelPreDispatchAssemblyOwnerSnapshotV1) (core.Digest, error) {
	binding := snapshot.association.Candidate.Binding
	return core.CanonicalJSONDigest(modelPreDispatchAssemblyCurrentnessDomainV1, ModelPreDispatchAssemblyPublisherContractVersionV1, "ModelPreDispatchAssemblyCurrentnessV1", struct {
		VerifiedInput            ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1 `json:"verified_input"`
		VerifiedProjectionDigest core.Digest                                       `json:"verified_projection_digest"`
		VerifiedCompileDigest    core.Digest                                       `json:"verified_compile_digest"`
		VerifiedCheckedUnixNano  int64                                             `json:"verified_checked_unix_nano"`
		VerifiedExpiresUnixNano  int64                                             `json:"verified_expires_unix_nano"`
		HandoffInput             runtimeports.ModelPreDispatchAssemblyExactRefV1   `json:"handoff_input"`
		HandoffProjectionDigest  core.Digest                                       `json:"handoff_projection_digest"`
		HandoffCurrentnessDigest core.Digest                                       `json:"handoff_currentness_digest"`
		HandoffCheckedUnixNano   int64                                             `json:"handoff_checked_unix_nano"`
		HandoffExpiresUnixNano   int64                                             `json:"handoff_expires_unix_nano"`
		AssociationInput         runtimeports.GenerationBindingAssociationRefV1    `json:"association_input"`
		BindingSetID             string                                            `json:"binding_set_id"`
		BindingSetRevision       core.Revision                                     `json:"binding_set_revision"`
		BindingSetDigest         core.Digest                                       `json:"binding_set_digest"`
		BindingSetSemanticDigest core.Digest                                       `json:"binding_set_semantic_digest"`
		BindingCurrentnessDigest core.Digest                                       `json:"binding_currentness_digest"`
		BindingProjectionDigest  core.Digest                                       `json:"binding_projection_digest"`
		BindingExpiresUnixNano   int64                                             `json:"binding_expires_unix_nano"`
		ToolInput                toolcontract.ToolSurfaceManifestCurrentRefV1      `json:"tool_input"`
		ToolProjectionDigest     core.Digest                                       `json:"tool_projection_digest"`
		ToolCheckedUnixNano      int64                                             `json:"tool_checked_unix_nano"`
		ToolExpiresUnixNano      int64                                             `json:"tool_expires_unix_nano"`
		RegistryInput            runtimeports.RegistrySnapshotRefV1                `json:"registry_input"`
		RegistryOutput           runtimeports.RegistrySnapshotRefV1                `json:"registry_output"`
	}{request.VerifiedAssembly, snapshot.verified.ProjectionDigest, snapshot.verified.CompileDigest, snapshot.verified.CheckedUnixNano, snapshot.verified.ExpiresUnixNano, request.Handoff, snapshot.handoff.ProjectionDigest, snapshot.handoff.CurrentnessDigest, snapshot.handoff.CheckedUnixNano, snapshot.handoff.ExpiresUnixNano, request.Association, binding.BindingSetID, binding.BindingSetRevision, binding.BindingSetDigest, binding.BindingSetSemanticDigest, binding.CurrentnessDigest, binding.ProjectionDigest, binding.ExpiresUnixNano, snapshot.tool.Ref, snapshot.tool.ProjectionDigest, snapshot.tool.CheckedUnixNano, snapshot.tool.ExpiresUnixNano, request.RegistrySnapshot, snapshot.registry})
}

func freshModelPreDispatchAssemblyClockV1(clock func() time.Time, previous time.Time) (time.Time, error) {
	if clock == nil {
		return time.Time{}, componentMissingModelPreDispatchAssemblyV1("Harness Assembly current clock is unavailable")
	}
	now := clock()
	if now.IsZero() || (!previous.IsZero() && now.Before(previous)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Harness Assembly current clock regressed")
	}
	return now, nil
}

func modelPreDispatchAssemblyContextErrorV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Harness Assembly current context is required")
	}
	if ctx.Err() != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Harness Assembly current context is canceled")
	}
	return nil
}

func boundedModelPreDispatchAssemblyRecoveryContextV1(ctx context.Context, ownerNow time.Time, expiresUnixNano int64) (context.Context, context.CancelFunc) {
	base := context.WithoutCancel(ctx)
	budget := time.Unix(0, expiresUnixNano).Sub(ownerNow)
	if budget <= 0 {
		budget = time.Nanosecond
	}
	if budget > modelPreDispatchAssemblyRecoveryHardCapV1 {
		budget = modelPreDispatchAssemblyRecoveryHardCapV1
	}
	if callerDeadline, ok := ctx.Deadline(); ok {
		if callerBudget := time.Until(callerDeadline); callerBudget < budget {
			budget = callerBudget
		}
	}
	return context.WithTimeout(base, budget)
}

func componentMissingModelPreDispatchAssemblyV1(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, message)
}

func nilLikeModelPreDispatchAssemblyV1(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func minInt64V1(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func maxInt64V1(values ...int64) int64 {
	maximum := values[0]
	for _, value := range values[1:] {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

var (
	_ ModelPreDispatchAssemblyCurrentStoreV1               = (*InMemoryModelPreDispatchAssemblyCurrentStoreV1)(nil)
	_ runtimeports.ModelPreDispatchAssemblyCurrentReaderV1 = (*ModelPreDispatchAssemblyCurrentPublisherV1)(nil)
)

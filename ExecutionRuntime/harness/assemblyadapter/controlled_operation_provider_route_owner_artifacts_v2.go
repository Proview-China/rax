package assemblyadapter

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ControlledOperationProviderRouteOwnerArtifactPublicationV2 contains owner
// artifacts, not caller-authored currentness. Runtime association and Binding
// currentness are always reread by the conformance adapter.
type ControlledOperationProviderRouteOwnerArtifactPublicationV2 struct {
	Key         ControlledOperationProviderRouteConformanceKeyV2                   `json:"key"`
	Compile     assemblycompiler.ControlledOperationProviderRouteCompileResultV2   `json:"compile"`
	Association runtimeports.GenerationBindingAssociationRefV1                     `json:"association"`
	ActiveRoute ControlledOperationProviderActiveRouteCurrentV2                    `json:"active_route"`
	Wiring      assemblycontract.ControlledOperationProviderRouteWiringInventoryV2 `json:"wiring"`
	Bindings    [7]runtimeports.ProviderBindingRefV2                               `json:"bindings"`
}

// InMemoryControlledOperationProviderRouteOwnerArtifactStoreV2 is a
// deterministic Harness Fact Owner used by isolated composition and
// conformance tests. It does not claim a production backend or SLA.
type InMemoryControlledOperationProviderRouteOwnerArtifactStoreV2 struct {
	mu       sync.RWMutex
	byKey    map[ControlledOperationProviderRouteConformanceKeyV2]ControlledOperationProviderRouteOwnerRefsV2
	content  map[ControlledOperationProviderRouteConformanceKeyV2]core.Digest
	compiles map[ControlledOperationProviderRouteVerifiedCompileRefV2]assemblycompiler.ControlledOperationProviderRouteCompileResultV2
	active   map[ControlledOperationProviderActiveRouteCurrentRefV2]ControlledOperationProviderActiveRouteCurrentV2
	wiring   map[ControlledOperationProviderRouteWiringInventoryRefV2]assemblycontract.ControlledOperationProviderRouteWiringInventoryV2
}

func NewInMemoryControlledOperationProviderRouteOwnerArtifactStoreV2() *InMemoryControlledOperationProviderRouteOwnerArtifactStoreV2 {
	return &InMemoryControlledOperationProviderRouteOwnerArtifactStoreV2{
		byKey:    make(map[ControlledOperationProviderRouteConformanceKeyV2]ControlledOperationProviderRouteOwnerRefsV2),
		content:  make(map[ControlledOperationProviderRouteConformanceKeyV2]core.Digest),
		compiles: make(map[ControlledOperationProviderRouteVerifiedCompileRefV2]assemblycompiler.ControlledOperationProviderRouteCompileResultV2),
		active:   make(map[ControlledOperationProviderActiveRouteCurrentRefV2]ControlledOperationProviderActiveRouteCurrentV2),
		wiring:   make(map[ControlledOperationProviderRouteWiringInventoryRefV2]assemblycontract.ControlledOperationProviderRouteWiringInventoryV2),
	}
}

func (*InMemoryControlledOperationProviderRouteOwnerArtifactStoreV2) controlledOperationProviderRouteConformanceOwnerSourceV2() {
}

func (s *InMemoryControlledOperationProviderRouteOwnerArtifactStoreV2) PublishExactV2(_ context.Context, value ControlledOperationProviderRouteOwnerArtifactPublicationV2, now time.Time) (ControlledOperationProviderRouteOwnerRefsV2, error) {
	if s == nil || now.IsZero() {
		return ControlledOperationProviderRouteOwnerRefsV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "controlled Provider route Owner artifact store or clock is unavailable")
	}
	if err := value.Compile.ValidateV2(); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	if value.Key.CompileDigest != value.Compile.CompileDigest || value.Key.BindingSetID == "" || value.Key.ActiveRouteID == "" || value.Key.Revision == 0 || value.Association.Validate() != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider route Owner publication coordinates are incomplete")
	}
	if value.ActiveRoute.Record.TransportIdentity != value.Compile.ProviderTransportIdentity || value.ActiveRoute.Record.ProviderIdentity != value.Compile.ProviderIdentity || value.ActiveRoute.Ref.ActiveRouteID != value.Key.ActiveRouteID {
		return ControlledOperationProviderRouteOwnerRefsV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route Owner active identity drifted from compile")
	}
	if err := value.ActiveRoute.validateExactV2(value.ActiveRoute.Ref, now); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	if err := assemblycompiler.ValidateControlledOperationProviderWiringV2(value.Compile.Declaration, value.Compile.ProviderTransportIdentity, value.Compile.ProviderIdentity, value.Wiring, value.Bindings, value.Compile.AssemblyInputDigest, value.Compile.Graph.Digest, now.UnixNano()); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	for _, binding := range value.Bindings {
		if binding.Validate() != nil || binding.BindingSetID != value.Key.BindingSetID {
			return ControlledOperationProviderRouteOwnerRefsV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route Owner Binding set drifted")
		}
	}
	compileRef := ControlledOperationProviderRouteVerifiedCompileRefV2{CompileDigest: value.Compile.CompileDigest}
	wiringRef := ControlledOperationProviderRouteWiringInventoryRefV2{InventoryID: value.Wiring.InventoryID, Revision: value.Wiring.Revision, Digest: value.Wiring.Digest}
	refs := ControlledOperationProviderRouteOwnerRefsV2{Compile: compileRef, Association: value.Association, ActiveRoute: value.ActiveRoute.Ref, Wiring: wiringRef, Bindings: value.Bindings, Revision: value.Key.Revision}
	if err := refs.validateV2(); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	contentDigest, err := core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", assemblycontract.ControlledOperationProviderRouteContractVersionV2, "ControlledOperationProviderRouteOwnerArtifactPublicationV2", value)
	if err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.byKey[value.Key]; ok {
		if existing != refs || s.content[value.Key] != contentDigest {
			return ControlledOperationProviderRouteOwnerRefsV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "controlled Provider route Owner publication key changed content")
		}
		return existing, nil
	}
	if existing, ok := s.compiles[compileRef]; ok && existing.CompileDigest != value.Compile.CompileDigest {
		return ControlledOperationProviderRouteOwnerRefsV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "controlled Provider route verified compile ref changed content")
	}
	if existing, ok := s.active[value.ActiveRoute.Ref]; ok && existing != value.ActiveRoute {
		return ControlledOperationProviderRouteOwnerRefsV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "controlled Provider active route ref changed content")
	}
	if existing, ok := s.wiring[wiringRef]; ok && existing.Digest != value.Wiring.Digest {
		return ControlledOperationProviderRouteOwnerRefsV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "controlled Provider wiring ref changed content")
	}
	s.byKey[value.Key] = refs
	s.content[value.Key] = contentDigest
	s.compiles[compileRef] = value.Compile
	s.active[value.ActiveRoute.Ref] = value.ActiveRoute
	s.wiring[wiringRef] = value.Wiring
	return refs, nil
}

func (s *InMemoryControlledOperationProviderRouteOwnerArtifactStoreV2) InspectControlledOperationProviderRouteOwnerRefsV2(_ context.Context, key ControlledOperationProviderRouteConformanceKeyV2) (ControlledOperationProviderRouteOwnerRefsV2, error) {
	if s == nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "controlled Provider route Owner store is unavailable")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.byKey[key]
	if !ok {
		return ControlledOperationProviderRouteOwnerRefsV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "controlled Provider route Owner refs are absent")
	}
	return value, nil
}

func (s *InMemoryControlledOperationProviderRouteOwnerArtifactStoreV2) InspectVerifiedControlledOperationProviderRouteCompileV2(_ context.Context, ref ControlledOperationProviderRouteVerifiedCompileRefV2) (assemblycompiler.ControlledOperationProviderRouteCompileResultV2, error) {
	if s == nil {
		return assemblycompiler.ControlledOperationProviderRouteCompileResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "controlled Provider route compile store is unavailable")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.compiles[ref]
	if !ok {
		return assemblycompiler.ControlledOperationProviderRouteCompileResultV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "controlled Provider route compile is absent")
	}
	return value, nil
}

func (s *InMemoryControlledOperationProviderRouteOwnerArtifactStoreV2) InspectControlledOperationProviderActiveRouteCurrentV2(_ context.Context, ref ControlledOperationProviderActiveRouteCurrentRefV2) (ControlledOperationProviderActiveRouteCurrentV2, error) {
	if s == nil {
		return ControlledOperationProviderActiveRouteCurrentV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "controlled Provider active route store is unavailable")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.active[ref]
	if !ok {
		return ControlledOperationProviderActiveRouteCurrentV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "controlled Provider active route is absent")
	}
	return value, nil
}

func (s *InMemoryControlledOperationProviderRouteOwnerArtifactStoreV2) InspectControlledOperationProviderRouteWiringInventoryV2(_ context.Context, ref ControlledOperationProviderRouteWiringInventoryRefV2) (assemblycontract.ControlledOperationProviderRouteWiringInventoryV2, error) {
	if s == nil {
		return assemblycontract.ControlledOperationProviderRouteWiringInventoryV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "controlled Provider wiring store is unavailable")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.wiring[ref]
	if !ok {
		return assemblycontract.ControlledOperationProviderRouteWiringInventoryV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "controlled Provider wiring inventory is absent")
	}
	return value, nil
}

var _ ControlledOperationProviderRouteConformanceOwnerSourceV2 = (*InMemoryControlledOperationProviderRouteOwnerArtifactStoreV2)(nil)
var _ ControlledOperationProviderRouteVerifiedCompileReaderV2 = (*InMemoryControlledOperationProviderRouteOwnerArtifactStoreV2)(nil)
var _ ControlledOperationProviderActiveRouteCurrentReaderV2 = (*InMemoryControlledOperationProviderRouteOwnerArtifactStoreV2)(nil)
var _ ControlledOperationProviderRouteWiringInventoryReaderV2 = (*InMemoryControlledOperationProviderRouteOwnerArtifactStoreV2)(nil)

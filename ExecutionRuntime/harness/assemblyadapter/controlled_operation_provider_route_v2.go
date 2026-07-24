package assemblyadapter

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ControlledOperationProviderRouteConformanceRequestV2 struct {
	Compile                        assemblycompiler.ControlledOperationProviderRouteCompileResultV2   `json:"compile"`
	AssemblyInputDigest            core.Digest                                                        `json:"assembly_input_digest"`
	ManifestDigest                 core.Digest                                                        `json:"manifest_digest"`
	GraphDigest                    core.Digest                                                        `json:"graph_digest"`
	Generation                     runtimeports.GenerationArtifactRefV1                               `json:"generation"`
	HandoffID                      string                                                             `json:"handoff_id"`
	HandoffRevision                core.Revision                                                      `json:"handoff_revision"`
	HandoffDigest                  core.Digest                                                        `json:"handoff_digest"`
	BindingSetID                   string                                                             `json:"binding_set_id"`
	BindingSetRevision             core.Revision                                                      `json:"binding_set_revision"`
	BindingSetDigest               core.Digest                                                        `json:"binding_set_digest"`
	BindingSetSemanticDigest       core.Digest                                                        `json:"binding_set_semantic_digest"`
	BindingSetCurrentnessDigest    core.Digest                                                        `json:"binding_set_currentness_digest"`
	AssemblyConformance            assemblycontract.AssemblyBindingConformanceV1                      `json:"assembly_conformance"`
	AssemblyConformanceRef         assemblycontract.ObjectRefV1                                       `json:"assembly_conformance_ref"`
	ActiveRouteID                  string                                                             `json:"active_route_id"`
	ActiveRouteRevision            core.Revision                                                      `json:"active_route_revision"`
	ActiveRouteDigest              core.Digest                                                        `json:"active_route_digest"`
	Bindings                       [7]runtimeports.ProviderBindingRefV2                               `json:"bindings"`
	WiringInventory                assemblycontract.ControlledOperationProviderRouteWiringInventoryV2 `json:"wiring_inventory"`
	GenerationCurrentness          ControlledOperationProviderRouteSourceCurrentnessV2                `json:"generation_currentness"`
	HandoffCurrentness             ControlledOperationProviderRouteSourceCurrentnessV2                `json:"handoff_currentness"`
	BindingSetCurrentness          ControlledOperationProviderRouteSourceCurrentnessV2                `json:"binding_set_currentness"`
	AssemblyConformanceCurrentness ControlledOperationProviderRouteSourceCurrentnessV2                `json:"assembly_conformance_currentness"`
	ActiveRouteCurrentness         ControlledOperationProviderRouteSourceCurrentnessV2                `json:"active_route_currentness"`
	BindingCurrentness             [7]ControlledOperationProviderRouteSourceCurrentnessV2             `json:"binding_currentness"`
	CheckedUnixNano                int64                                                              `json:"checked_unix_nano"`
	ExpiresUnixNano                int64                                                              `json:"expires_unix_nano"`
	Revision                       core.Revision                                                      `json:"revision"`
}

// ControlledOperationProviderRouteConformanceKeyV2 contains only exact lookup
// coordinates. It carries no caller-authored currentness or conformance fact.
type ControlledOperationProviderRouteConformanceKeyV2 struct {
	CompileDigest core.Digest   `json:"compile_digest"`
	BindingSetID  string        `json:"binding_set_id"`
	ActiveRouteID string        `json:"active_route_id"`
	Revision      core.Revision `json:"revision"`
}

// ControlledOperationProviderRouteConformanceInputsReaderV2 is implemented by
// the Harness Assembly owner over verified compile artifacts and live exact
// readers. A production builder never accepts an inline snapshot from its
// caller.
type ControlledOperationProviderRouteConformanceInputsReaderV2 interface {
	InspectCurrentControlledOperationProviderRouteConformanceInputsV2(context.Context, ControlledOperationProviderRouteConformanceKeyV2) (ControlledOperationProviderRouteConformanceRequestV2, error)
	controlledOperationProviderRouteConformanceInputsOwnerV2()
}

type ControlledOperationProviderRouteVerifiedCompileRefV2 struct {
	CompileDigest core.Digest `json:"compile_digest"`
}

type ControlledOperationProviderActiveRouteCurrentRefV2 struct {
	ActiveRouteID string        `json:"active_route_id"`
	Revision      core.Revision `json:"revision"`
	Digest        core.Digest   `json:"digest"`
}

type ControlledOperationProviderRouteWiringInventoryRefV2 struct {
	InventoryID string        `json:"inventory_id"`
	Revision    core.Revision `json:"revision"`
	Digest      core.Digest   `json:"digest"`
}

type ControlledOperationProviderRouteOwnerRefsV2 struct {
	Compile     ControlledOperationProviderRouteVerifiedCompileRefV2 `json:"compile"`
	Association runtimeports.GenerationBindingAssociationRefV1       `json:"association"`
	ActiveRoute ControlledOperationProviderActiveRouteCurrentRefV2   `json:"active_route"`
	Wiring      ControlledOperationProviderRouteWiringInventoryRefV2 `json:"wiring"`
	Bindings    [7]runtimeports.ProviderBindingRefV2                 `json:"bindings"`
	Revision    core.Revision                                        `json:"revision"`
}

func (r ControlledOperationProviderRouteOwnerRefsV2) validateV2() error {
	if r.Compile.CompileDigest.Validate() != nil || r.Association.Validate() != nil || r.ActiveRoute.ActiveRouteID == "" || r.ActiveRoute.Revision == 0 || r.ActiveRoute.Digest.Validate() != nil || r.Wiring.InventoryID == "" || r.Wiring.Revision == 0 || r.Wiring.Digest.Validate() != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider route Owner refs are incomplete")
	}
	for _, binding := range r.Bindings {
		if binding.Validate() != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider route Owner Binding ref is incomplete")
		}
	}
	return nil
}

type ControlledOperationProviderActiveRouteCurrentV2 struct {
	Ref             ControlledOperationProviderActiveRouteCurrentRefV2              `json:"ref"`
	Record          assemblycontract.ControlledOperationProviderActiveRouteRecordV2 `json:"record"`
	CheckedUnixNano int64                                                           `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                                           `json:"expires_unix_nano"`
}

func (c ControlledOperationProviderActiveRouteCurrentV2) validateExactV2(ref ControlledOperationProviderActiveRouteCurrentRefV2, now time.Time) error {
	digest, err := core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", assemblycontract.ControlledOperationProviderRouteContractVersionV2, "ControlledOperationProviderActiveRouteRecordV2", c.Record)
	if err != nil || c.Ref != ref || c.Record.RouteID != ref.ActiveRouteID || c.Ref.Digest != digest || !c.Record.Active || c.CheckedUnixNano <= 0 || c.CheckedUnixNano > now.UnixNano() || c.ExpiresUnixNano <= now.UnixNano() {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider active route current drifted")
	}
	return nil
}

type ControlledOperationProviderRouteVerifiedCompileReaderV2 interface {
	InspectVerifiedControlledOperationProviderRouteCompileV2(context.Context, ControlledOperationProviderRouteVerifiedCompileRefV2) (assemblycompiler.ControlledOperationProviderRouteCompileResultV2, error)
}

type ControlledOperationProviderActiveRouteCurrentReaderV2 interface {
	InspectControlledOperationProviderActiveRouteCurrentV2(context.Context, ControlledOperationProviderActiveRouteCurrentRefV2) (ControlledOperationProviderActiveRouteCurrentV2, error)
}

type ControlledOperationProviderRouteWiringInventoryReaderV2 interface {
	InspectControlledOperationProviderRouteWiringInventoryV2(context.Context, ControlledOperationProviderRouteWiringInventoryRefV2) (assemblycontract.ControlledOperationProviderRouteWiringInventoryV2, error)
}

// ControlledOperationProviderRouteConformanceOwnerSourceV2 returns stable
// exact refs only. The unexported marker prevents external packages from
// routing a self-signed snapshot back through FromOwner.
type ControlledOperationProviderRouteConformanceOwnerSourceV2 interface {
	InspectControlledOperationProviderRouteOwnerRefsV2(context.Context, ControlledOperationProviderRouteConformanceKeyV2) (ControlledOperationProviderRouteOwnerRefsV2, error)
	controlledOperationProviderRouteConformanceOwnerSourceV2()
}

type controlledOperationProviderRouteConformanceOwnerReaderV2 struct {
	source       ControlledOperationProviderRouteConformanceOwnerSourceV2
	compiles     ControlledOperationProviderRouteVerifiedCompileReaderV2
	associations runtimeports.GenerationBindingAssociationGovernancePortV1
	activeRoutes ControlledOperationProviderActiveRouteCurrentReaderV2
	wiring       ControlledOperationProviderRouteWiringInventoryReaderV2
	bindings     runtimeports.ProviderBindingCurrentnessPortV2
	clock        func() time.Time
}

func (*controlledOperationProviderRouteConformanceOwnerReaderV2) controlledOperationProviderRouteConformanceInputsOwnerV2() {
}

func (r *controlledOperationProviderRouteConformanceOwnerReaderV2) InspectCurrentControlledOperationProviderRouteConformanceInputsV2(ctx context.Context, key ControlledOperationProviderRouteConformanceKeyV2) (ControlledOperationProviderRouteConformanceRequestV2, error) {
	if r == nil || isNilControlledOperationProviderRouteDependencyV2(r.source) || isNilControlledOperationProviderRouteDependencyV2(r.compiles) || isNilControlledOperationProviderRouteDependencyV2(r.associations) || isNilControlledOperationProviderRouteDependencyV2(r.activeRoutes) || isNilControlledOperationProviderRouteDependencyV2(r.wiring) || isNilControlledOperationProviderRouteDependencyV2(r.bindings) || r.clock == nil {
		return ControlledOperationProviderRouteConformanceRequestV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "controlled Provider route Owner inputs are unavailable")
	}
	now := r.clock()
	if now.IsZero() {
		return ControlledOperationProviderRouteConformanceRequestV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled Provider route Owner clock is unavailable")
	}
	refs, err := r.source.InspectControlledOperationProviderRouteOwnerRefsV2(ctx, key)
	if err != nil {
		return ControlledOperationProviderRouteConformanceRequestV2{}, err
	}
	if err := refs.validateV2(); err != nil {
		return ControlledOperationProviderRouteConformanceRequestV2{}, err
	}
	if refs.Compile.CompileDigest != key.CompileDigest || refs.ActiveRoute.ActiveRouteID != key.ActiveRouteID || refs.Revision != key.Revision {
		return ControlledOperationProviderRouteConformanceRequestV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route Owner lookup or association ref drifted")
	}
	compile, err := r.compiles.InspectVerifiedControlledOperationProviderRouteCompileV2(ctx, refs.Compile)
	if err != nil {
		return ControlledOperationProviderRouteConformanceRequestV2{}, err
	}
	if err := compile.ValidateV2(); err != nil {
		return ControlledOperationProviderRouteConformanceRequestV2{}, err
	}
	if compile.CompileDigest != refs.Compile.CompileDigest {
		return ControlledOperationProviderRouteConformanceRequestV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route verified compile ref drifted")
	}
	association, err := r.associations.InspectCurrentGenerationBindingAssociationV1(ctx, refs.Association.ID)
	if err != nil {
		return ControlledOperationProviderRouteConformanceRequestV2{}, err
	}
	if association.RefV1() != refs.Association || association.Candidate.Binding.BindingSetID != key.BindingSetID {
		return ControlledOperationProviderRouteConformanceRequestV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route association ref drifted")
	}
	activeRoute, err := r.activeRoutes.InspectControlledOperationProviderActiveRouteCurrentV2(ctx, refs.ActiveRoute)
	if err != nil {
		return ControlledOperationProviderRouteConformanceRequestV2{}, err
	}
	if err := activeRoute.validateExactV2(refs.ActiveRoute, now); err != nil {
		return ControlledOperationProviderRouteConformanceRequestV2{}, err
	}
	wiring, err := r.wiring.InspectControlledOperationProviderRouteWiringInventoryV2(ctx, refs.Wiring)
	if err != nil {
		return ControlledOperationProviderRouteConformanceRequestV2{}, err
	}
	if err := wiring.Validate(); err != nil {
		return ControlledOperationProviderRouteConformanceRequestV2{}, err
	}
	if wiring.InventoryID != refs.Wiring.InventoryID || wiring.Revision != refs.Wiring.Revision || wiring.Digest != refs.Wiring.Digest || wiring.ExpiresUnixNano <= now.UnixNano() {
		return ControlledOperationProviderRouteConformanceRequestV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route wiring inventory ref drifted")
	}
	conformance, err := BuildBindingConformanceV1(compile.Handoff, association, now)
	if err != nil {
		return ControlledOperationProviderRouteConformanceRequestV2{}, err
	}
	snapshot := ControlledOperationProviderRouteConformanceRequestV2{Compile: compile, Bindings: refs.Bindings, WiringInventory: wiring, Revision: refs.Revision}
	snapshot.AssemblyConformance = conformance
	snapshot.AssemblyConformanceRef = assemblycontract.ObjectRefV1{ID: conformance.HandoffRef.ID + "/binding-conformance", Revision: association.Revision, Digest: conformance.Digest}
	snapshot.AssemblyInputDigest = compile.AssemblyInputDigest
	snapshot.ManifestDigest = compile.Manifest.Digest
	snapshot.GraphDigest = compile.Graph.Digest
	snapshot.Generation = generationArtifactRefFromCompileV2(compile)
	snapshot.HandoffID = conformance.HandoffRef.ID
	snapshot.HandoffRevision = conformance.HandoffRef.Revision
	snapshot.HandoffDigest = conformance.HandoffRef.Digest
	snapshot.BindingSetID = association.Candidate.Binding.BindingSetID
	snapshot.BindingSetRevision = association.Candidate.Binding.BindingSetRevision
	snapshot.BindingSetDigest = association.Candidate.Binding.BindingSetDigest
	snapshot.BindingSetSemanticDigest = association.Candidate.Binding.BindingSetSemanticDigest
	snapshot.BindingSetCurrentnessDigest = association.Candidate.Binding.CurrentnessDigest
	snapshot.ActiveRouteID = activeRoute.Ref.ActiveRouteID
	snapshot.ActiveRouteRevision = activeRoute.Ref.Revision
	snapshot.ActiveRouteDigest = activeRoute.Ref.Digest
	snapshot.CheckedUnixNano = now.UnixNano()
	associationCurrentness := ControlledOperationProviderRouteSourceCurrentnessV2{CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: association.ExpiresUnixNano}
	snapshot.GenerationCurrentness = associationCurrentness
	snapshot.HandoffCurrentness = associationCurrentness
	snapshot.BindingSetCurrentness = ControlledOperationProviderRouteSourceCurrentnessV2{CheckedUnixNano: association.Candidate.Binding.IssuedUnixNano, ExpiresUnixNano: association.Candidate.Binding.ExpiresUnixNano}
	snapshot.AssemblyConformanceCurrentness = ControlledOperationProviderRouteSourceCurrentnessV2{CheckedUnixNano: conformance.ObservedUnixNano, ExpiresUnixNano: conformance.ExpiresUnixNano}
	snapshot.ActiveRouteCurrentness = ControlledOperationProviderRouteSourceCurrentnessV2{CheckedUnixNano: activeRoute.CheckedUnixNano, ExpiresUnixNano: activeRoute.ExpiresUnixNano}
	minExpires := snapshot.WiringInventory.ExpiresUnixNano
	for index, binding := range snapshot.Bindings {
		projection, inspectErr := r.bindings.InspectProviderBindingCurrentV2(ctx, binding)
		if inspectErr != nil {
			return ControlledOperationProviderRouteConformanceRequestV2{}, inspectErr
		}
		if err := projection.ValidateCurrent(binding, now); err != nil {
			return ControlledOperationProviderRouteConformanceRequestV2{}, err
		}
		if projection.BindingSetDigest != snapshot.BindingSetDigest || projection.BindingSetSemanticDigest != snapshot.BindingSetSemanticDigest {
			return ControlledOperationProviderRouteConformanceRequestV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route Binding capability is not current in the associated set")
		}
		snapshot.BindingCurrentness[index] = ControlledOperationProviderRouteSourceCurrentnessV2{CheckedUnixNano: projection.IssuedUnixNano, ExpiresUnixNano: projection.ExpiresUnixNano}
		if projection.ExpiresUnixNano < minExpires {
			minExpires = projection.ExpiresUnixNano
		}
	}
	for _, currentness := range []ControlledOperationProviderRouteSourceCurrentnessV2{snapshot.GenerationCurrentness, snapshot.HandoffCurrentness, snapshot.BindingSetCurrentness, snapshot.AssemblyConformanceCurrentness, snapshot.ActiveRouteCurrentness} {
		if currentness.ExpiresUnixNano < minExpires {
			minExpires = currentness.ExpiresUnixNano
		}
	}
	snapshot.ExpiresUnixNano = minExpires
	return snapshot, nil
}

func NewControlledOperationProviderRouteConformanceBuilderFromOwnerV2(source ControlledOperationProviderRouteConformanceOwnerSourceV2, compiles ControlledOperationProviderRouteVerifiedCompileReaderV2, associations runtimeports.GenerationBindingAssociationGovernancePortV1, activeRoutes ControlledOperationProviderActiveRouteCurrentReaderV2, wiring ControlledOperationProviderRouteWiringInventoryReaderV2, bindings runtimeports.ProviderBindingCurrentnessPortV2, clock func() time.Time) (*ControlledOperationProviderRouteConformanceBuilderV2, error) {
	if isNilControlledOperationProviderRouteDependencyV2(source) || isNilControlledOperationProviderRouteDependencyV2(compiles) || isNilControlledOperationProviderRouteDependencyV2(associations) || isNilControlledOperationProviderRouteDependencyV2(activeRoutes) || isNilControlledOperationProviderRouteDependencyV2(wiring) || isNilControlledOperationProviderRouteDependencyV2(bindings) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "controlled Provider route Owner refs, verified compile, association, active route, wiring, Binding current Readers and clock are required")
	}
	reader := &controlledOperationProviderRouteConformanceOwnerReaderV2{source: source, compiles: compiles, associations: associations, activeRoutes: activeRoutes, wiring: wiring, bindings: bindings, clock: clock}
	return NewControlledOperationProviderRouteConformanceBuilderV2(reader, clock)
}

type ControlledOperationProviderRouteConformanceBuilderV2 struct {
	inputs ControlledOperationProviderRouteConformanceInputsReaderV2
	clock  func() time.Time
}

func NewControlledOperationProviderRouteConformanceBuilderV2(inputs ControlledOperationProviderRouteConformanceInputsReaderV2, clock func() time.Time) (*ControlledOperationProviderRouteConformanceBuilderV2, error) {
	if isNilControlledOperationProviderRouteDependencyV2(inputs) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "controlled Provider route conformance inputs Reader and clock are required")
	}
	return &ControlledOperationProviderRouteConformanceBuilderV2{inputs: inputs, clock: clock}, nil
}

func (b *ControlledOperationProviderRouteConformanceBuilderV2) BuildControlledOperationProviderRouteConformanceV2(ctx context.Context, key ControlledOperationProviderRouteConformanceKeyV2) (assemblycontract.ControlledOperationProviderRouteConformanceV2, error) {
	if b == nil || isNilControlledOperationProviderRouteDependencyV2(b.inputs) || b.clock == nil || key.CompileDigest.Validate() != nil || key.BindingSetID == "" || key.ActiveRouteID == "" || key.Revision == 0 {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider route conformance key is invalid")
	}
	firstNow := b.clock()
	if firstNow.IsZero() {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled Provider route conformance clock is unavailable")
	}
	snapshot, err := b.inputs.InspectCurrentControlledOperationProviderRouteConformanceInputsV2(ctx, key)
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	secondNow := b.clock()
	if secondNow.IsZero() || secondNow.Before(firstNow) {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled Provider route conformance clock regressed")
	}
	if snapshot.Compile.CompileDigest != key.CompileDigest || snapshot.BindingSetID != key.BindingSetID || snapshot.ActiveRouteID != key.ActiveRouteID || snapshot.Revision != key.Revision {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route conformance lookup drifted")
	}
	return buildControlledOperationProviderRouteConformanceFromCurrentV2(snapshot, secondNow)
}

// ControlledOperationProviderRouteSourceCurrentnessV2 is an observation
// lease, not a domain fact TTL. The source ref/digest remains in the exact
// Conformance fields; this value only proves the min-TTL calculation.
type ControlledOperationProviderRouteSourceCurrentnessV2 struct {
	CheckedUnixNano int64 `json:"checked_unix_nano"`
	ExpiresUnixNano int64 `json:"expires_unix_nano"`
}

func (c ControlledOperationProviderRouteSourceCurrentnessV2) validateV2(now time.Time) error {
	if now.IsZero() || c.CheckedUnixNano <= 0 || c.CheckedUnixNano > now.UnixNano() || c.ExpiresUnixNano <= now.UnixNano() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled Provider route source currentness is absent, future-dated or expired")
	}
	return nil
}

func buildControlledOperationProviderRouteConformanceFromCurrentV2(request ControlledOperationProviderRouteConformanceRequestV2, now time.Time) (assemblycontract.ControlledOperationProviderRouteConformanceV2, error) {
	if err := request.Compile.ValidateV2(); err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	declaration := request.Compile.Declaration
	if err := declaration.Validate(); err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	if request.Revision == 0 || now.IsZero() || request.CheckedUnixNano <= 0 || request.CheckedUnixNano > now.UnixNano() || request.ExpiresUnixNano <= now.UnixNano() {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled Provider route conformance request is not current")
	}
	if request.AssemblyInputDigest != request.Compile.AssemblyInputDigest || request.ManifestDigest != request.Compile.Manifest.Digest || request.GraphDigest != request.Compile.Graph.Digest || request.Generation != generationArtifactRefFromCompileV2(request.Compile) || request.HandoffDigest != request.Compile.Handoff.Digest {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route current sources drifted from verified compile artifacts")
	}
	if err := request.AssemblyConformance.Validate(now.UnixNano()); err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	if request.AssemblyConformanceRef.Digest != request.AssemblyConformance.Digest || request.AssemblyConformance.HandoffRef.Digest != request.Compile.Handoff.Digest || request.AssemblyConformance.GenerationRef.ID != request.Compile.Generation.GenerationID || request.AssemblyConformance.GenerationRef.Revision != request.Compile.Generation.Revision || request.AssemblyConformance.GenerationRef.Digest != request.Compile.Generation.Digest || request.AssemblyConformance.ManifestDigest != request.ManifestDigest || request.AssemblyConformance.GraphDigest != request.GraphDigest || request.AssemblyConformance.BindingSetID != request.BindingSetID || request.AssemblyConformance.BindingSetRevision != request.BindingSetRevision || request.AssemblyConformance.BindingSetDigest != request.BindingSetDigest || request.AssemblyConformance.BindingSetSemanticDigest != request.BindingSetSemanticDigest || request.AssemblyConformance.BindingSetCurrentnessDigest != request.BindingSetCurrentnessDigest {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route Assembly Binding Conformance drifted")
	}
	currentness := []ControlledOperationProviderRouteSourceCurrentnessV2{request.GenerationCurrentness, request.HandoffCurrentness, request.BindingSetCurrentness, request.AssemblyConformanceCurrentness, request.ActiveRouteCurrentness}
	currentness = append(currentness, request.BindingCurrentness[:]...)
	minExpires := request.WiringInventory.ExpiresUnixNano
	for _, source := range currentness {
		if err := source.validateV2(now); err != nil {
			return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
		}
		if source.ExpiresUnixNano < minExpires {
			minExpires = source.ExpiresUnixNano
		}
	}
	if request.ExpiresUnixNano != minExpires {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route conformance expiry is not the exact source minimum")
	}
	if err := assemblycompiler.ValidateControlledOperationProviderWiringV2(declaration, request.Compile.ProviderTransportIdentity, request.Compile.ProviderIdentity, request.WiringInventory, request.Bindings, request.AssemblyInputDigest, request.GraphDigest, now.UnixNano()); err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	if err := validateRouteBindingsAgainstDeclarationV2(declaration, request.Bindings); err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	id, err := assemblycontract.DeriveControlledOperationProviderRouteConformanceIDV2(declaration.RouteID, request.Generation, request.BindingSetID)
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	return assemblycontract.SealControlledOperationProviderRouteConformanceV2(assemblycontract.ControlledOperationProviderRouteConformanceV2{
		ConformanceID: id, Revision: request.Revision, DeclarationRef: declaration.RefV2(),
		RequiredExtensionKey: request.Compile.Extension.Key, RequiredExtensionSchema: request.Compile.Extension.Payload.Schema,
		RequiredExtensionDigest: request.Compile.Extension.Payload.ContentDigest,
		AssemblyInputDigest:     request.AssemblyInputDigest, ManifestDigest: request.ManifestDigest, GraphDigest: request.GraphDigest,
		Generation: request.Generation, HandoffID: request.HandoffID, HandoffRevision: request.HandoffRevision, HandoffDigest: request.HandoffDigest,
		BindingSetID: request.BindingSetID, BindingSetRevision: request.BindingSetRevision, BindingSetDigest: request.BindingSetDigest,
		BindingSetSemanticDigest: request.BindingSetSemanticDigest, BindingSetCurrentnessDigest: request.BindingSetCurrentnessDigest,
		AssemblyConformanceRef: request.AssemblyConformanceRef, ActiveRouteID: request.ActiveRouteID,
		ActiveRouteRevision: request.ActiveRouteRevision, ActiveRouteDigest: request.ActiveRouteDigest,
		ToolAdapterBinding: request.Bindings[0], GatewayBinding: request.Bindings[1], ProviderTransportBinding: request.Bindings[2],
		PreparedReaderBinding: request.Bindings[3], BoundaryReaderBinding: request.Bindings[4], ProviderInspectBinding: request.Bindings[5], ProviderBinding: request.Bindings[6],
		WiringInventoryRef: assemblycontract.ObjectRefV1{ID: request.WiringInventory.InventoryID, Revision: request.WiringInventory.Revision, Digest: request.WiringInventory.Digest},
		CheckedUnixNano:    request.CheckedUnixNano, ExpiresUnixNano: request.ExpiresUnixNano, Status: assemblycontract.ControlledOperationProviderRouteConformantV2,
	})
}

func generationArtifactRefFromCompileV2(value assemblycompiler.ControlledOperationProviderRouteCompileResultV2) runtimeports.GenerationArtifactRefV1 {
	return runtimeports.GenerationArtifactRefV1{
		ID: value.Generation.GenerationID, Revision: value.Generation.Revision, Digest: value.Generation.Digest,
		InputDigest: value.Generation.InputDigest, ManifestDigest: value.Generation.ManifestDigest,
		GraphDigest: value.Generation.GraphDigest, CatalogDigest: value.Manifest.CatalogDigest,
	}
}

func validateRouteBindingsAgainstDeclarationV2(declaration assemblycontract.ControlledOperationProviderRouteDeclarationV2, bindings [7]runtimeports.ProviderBindingRefV2) error {
	targets := []struct {
		component runtimeports.ComponentIDV2
		manifest  core.Digest
		artifact  core.Digest
		cap       runtimeports.CapabilityNameV2
	}{
		{declaration.ToolAdapter.ComponentID, declaration.ToolAdapter.ManifestDigest, declaration.ToolAdapter.ArtifactDigest, runtimeports.ControlledOperationToolAdapterCapabilityV2},
		{declaration.Gateway.ComponentID, declaration.Gateway.ManifestDigest, declaration.Gateway.ArtifactDigest, runtimeports.ControlledOperationGatewayCapabilityV2},
		{declaration.ProviderTransport.ComponentID, declaration.ProviderTransport.ManifestDigest, declaration.ProviderTransport.ArtifactDigest, runtimeports.ControlledOperationProviderTransportCapabilityV2},
		{declaration.PreparedCurrentReader.ComponentID, declaration.PreparedCurrentReader.ManifestDigest, declaration.PreparedCurrentReader.ArtifactDigest, runtimeports.ControlledOperationPreparedReaderCapabilityV2},
		{declaration.BoundaryCurrentReader.ComponentID, declaration.BoundaryCurrentReader.ManifestDigest, declaration.BoundaryCurrentReader.ArtifactDigest, runtimeports.ControlledOperationBoundaryReaderCapabilityV2},
		{declaration.ProviderInspectReader.ComponentID, declaration.ProviderInspectReader.ManifestDigest, declaration.ProviderInspectReader.ArtifactDigest, runtimeports.ControlledOperationProviderInspectCapabilityV2},
		{declaration.Provider.ComponentID, declaration.Provider.ManifestDigest, declaration.Provider.ArtifactDigest, runtimeports.CapabilityNameV2(declaration.Matrix.EffectKind)},
	}
	for index, binding := range bindings {
		target := targets[index]
		if binding.ComponentID != target.component || binding.ManifestDigest != target.manifest || binding.ArtifactDigest != target.artifact || binding.Capability != target.cap {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route binding does not prove its declared role")
		}
	}
	return nil
}

type ControlledOperationProviderRouteFactStoreV2 interface {
	PublishControlledOperationProviderRouteDeclarationV2(context.Context, assemblycontract.ControlledOperationProviderRouteDeclarationV2, core.Revision) (assemblycontract.ControlledOperationProviderRouteDeclarationV2, error)
	InspectControlledOperationProviderRouteDeclarationV2(context.Context, runtimeports.ControlledOperationProviderRouteDeclarationRefV2) (assemblycontract.ControlledOperationProviderRouteDeclarationV2, error)
	PublishControlledOperationProviderRouteConformanceV2(context.Context, assemblycontract.ControlledOperationProviderRouteConformanceV2, core.Revision) (assemblycontract.ControlledOperationProviderRouteConformanceV2, error)
	InspectControlledOperationProviderRouteConformanceV2(context.Context, runtimeports.ControlledOperationProviderRouteConformanceRefV2) (assemblycontract.ControlledOperationProviderRouteConformanceV2, error)
	PublishControlledOperationProviderRouteCurrentV2(context.Context, assemblycontract.ControlledOperationProviderRouteConformanceV2, core.Revision) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error)
	InspectControlledOperationProviderRouteCurrentV2(context.Context, runtimeports.ControlledOperationProviderRouteCurrentRefV2) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error)
}

type controlledOperationProviderRouteReplyV2 string

const (
	loseDeclarationReplyV2 controlledOperationProviderRouteReplyV2 = "declaration"
	loseConformanceReplyV2 controlledOperationProviderRouteReplyV2 = "conformance"
	loseCurrentReplyV2     controlledOperationProviderRouteReplyV2 = "current"
)

// InMemoryControlledOperationProviderRouteFactStoreV2 is deterministic test
// infrastructure. It does not claim a production backend, durability or SLA.
type InMemoryControlledOperationProviderRouteFactStoreV2 struct {
	mu              sync.RWMutex
	declarations    map[string]assemblycontract.ControlledOperationProviderRouteDeclarationV2
	conformances    map[string]assemblycontract.ControlledOperationProviderRouteConformanceV2
	currents        map[string]runtimeports.ControlledOperationProviderRouteCurrentProjectionV2
	currentHistory  map[string]map[core.Digest]struct{}
	loseNextReplies map[controlledOperationProviderRouteReplyV2]int
}

func NewInMemoryControlledOperationProviderRouteFactStoreV2() *InMemoryControlledOperationProviderRouteFactStoreV2 {
	return &InMemoryControlledOperationProviderRouteFactStoreV2{declarations: map[string]assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, conformances: map[string]assemblycontract.ControlledOperationProviderRouteConformanceV2{}, currents: map[string]runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, currentHistory: map[string]map[core.Digest]struct{}{}, loseNextReplies: map[controlledOperationProviderRouteReplyV2]int{}}
}

func (s *InMemoryControlledOperationProviderRouteFactStoreV2) LoseNextDeclarationReplyV2() {
	s.lose(loseDeclarationReplyV2)
}
func (s *InMemoryControlledOperationProviderRouteFactStoreV2) LoseNextConformanceReplyV2() {
	s.lose(loseConformanceReplyV2)
}
func (s *InMemoryControlledOperationProviderRouteFactStoreV2) LoseNextCurrentReplyV2() {
	s.lose(loseCurrentReplyV2)
}

func (s *InMemoryControlledOperationProviderRouteFactStoreV2) lose(kind controlledOperationProviderRouteReplyV2) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextReplies[kind]++
}

func (s *InMemoryControlledOperationProviderRouteFactStoreV2) takeLostReplyLocked(kind controlledOperationProviderRouteReplyV2) bool {
	if s.loseNextReplies[kind] == 0 {
		return false
	}
	s.loseNextReplies[kind]--
	return true
}

func (s *InMemoryControlledOperationProviderRouteFactStoreV2) PublishControlledOperationProviderRouteDeclarationV2(_ context.Context, value assemblycontract.ControlledOperationProviderRouteDeclarationV2, expectedPrevious core.Revision) (assemblycontract.ControlledOperationProviderRouteDeclarationV2, error) {
	if err := value.Validate(); err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.declarations[value.RouteID]
	if exists && current == value {
		return current, nil
	}
	if exists || expectedPrevious != 0 || value.Revision != 1 {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route declaration CAS conflicted")
	}
	s.declarations[value.RouteID] = value
	if s.takeLostReplyLocked(loseDeclarationReplyV2) {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "controlled Provider route declaration reply was lost")
	}
	return value, nil
}

func (s *InMemoryControlledOperationProviderRouteFactStoreV2) InspectControlledOperationProviderRouteDeclarationV2(_ context.Context, ref runtimeports.ControlledOperationProviderRouteDeclarationRefV2) (assemblycontract.ControlledOperationProviderRouteDeclarationV2, error) {
	if err := ref.Validate(); err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, exists := s.declarations[ref.RouteID]
	if !exists {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "controlled Provider route declaration is absent")
	}
	if current.RefV2() != ref {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route declaration ref drifted")
	}
	return current, nil
}

func (s *InMemoryControlledOperationProviderRouteFactStoreV2) PublishControlledOperationProviderRouteConformanceV2(_ context.Context, value assemblycontract.ControlledOperationProviderRouteConformanceV2, expectedPrevious core.Revision) (assemblycontract.ControlledOperationProviderRouteConformanceV2, error) {
	if err := value.Validate(time.Unix(0, value.CheckedUnixNano)); err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	declaration, ok := s.declarations[value.DeclarationRef.RouteID]
	if !ok || declaration.RefV2() != value.DeclarationRef {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route conformance lacks its exact declaration")
	}
	current, exists := s.conformances[value.ConformanceID]
	if exists && current == value {
		return current, nil
	}
	if exists || expectedPrevious != 0 || value.Revision != 1 {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route conformance CAS conflicted")
	}
	s.conformances[value.ConformanceID] = value
	if s.takeLostReplyLocked(loseConformanceReplyV2) {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "controlled Provider route conformance reply was lost")
	}
	return value, nil
}

func (s *InMemoryControlledOperationProviderRouteFactStoreV2) InspectControlledOperationProviderRouteConformanceV2(_ context.Context, ref runtimeports.ControlledOperationProviderRouteConformanceRefV2) (assemblycontract.ControlledOperationProviderRouteConformanceV2, error) {
	if err := ref.Validate(); err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, exists := s.conformances[ref.ConformanceID]
	if !exists {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "controlled Provider route conformance is absent")
	}
	if current.RefV2() != ref {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route conformance ref drifted")
	}
	return current, nil
}

func (s *InMemoryControlledOperationProviderRouteFactStoreV2) PublishControlledOperationProviderRouteCurrentV2(_ context.Context, conformance assemblycontract.ControlledOperationProviderRouteConformanceV2, expectedPrevious core.Revision) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	if err := conformance.Validate(time.Unix(0, conformance.CheckedUnixNano)); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.conformances[conformance.ConformanceID]
	if !ok || stored != conformance {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route current lacks exact conformance")
	}
	matrixDigest, err := runtimeports.DigestOperationScopeEvidenceApplicabilityMatrixKeyV3(runtimeports.OperationScopeEvidenceActionMatrixV3())
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	currentID, err := runtimeports.DeriveControlledOperationProviderRouteCurrentIDV2(conformance.DeclarationRef.RouteID, matrixDigest)
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	current, exists := s.currents[currentID]
	revision := core.Revision(1)
	if exists {
		revision = current.Ref.Revision
	}
	projection, err := runtimeports.SealControlledOperationProviderRouteCurrentProjectionV2(runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{
		Ref: runtimeports.ControlledOperationProviderRouteCurrentRefV2{Revision: revision}, DeclarationRef: conformance.DeclarationRef, ConformanceRef: conformance.RefV2(),
		Generation: conformance.Generation, HandoffID: conformance.HandoffID, HandoffRevision: conformance.HandoffRevision, HandoffDigest: conformance.HandoffDigest,
		BindingSetID: conformance.BindingSetID, BindingSetRevision: conformance.BindingSetRevision, BindingSetDigest: conformance.BindingSetDigest,
		BindingSetSemanticDigest: conformance.BindingSetSemanticDigest, BindingSetCurrentnessDigest: conformance.BindingSetCurrentnessDigest,
		ActiveRouteID: conformance.ActiveRouteID, ActiveRouteRevision: conformance.ActiveRouteRevision, ActiveRouteDigest: conformance.ActiveRouteDigest,
		ToolAdapterBinding: conformance.ToolAdapterBinding, GatewayBinding: conformance.GatewayBinding, ProviderTransportBinding: conformance.ProviderTransportBinding,
		PreparedReaderBinding: conformance.PreparedReaderBinding, BoundaryReaderBinding: conformance.BoundaryReaderBinding, ProviderInspectBinding: conformance.ProviderInspectBinding,
		ProviderBinding: conformance.ProviderBinding, CheckedUnixNano: conformance.CheckedUnixNano, ExpiresUnixNano: conformance.ExpiresUnixNano,
	})
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	if exists {
		if projection == current {
			return current, nil
		}
		if expectedPrevious != current.Ref.Revision || conformance.CheckedUnixNano <= current.CheckedUnixNano || (conformance.Generation.ID == current.Generation.ID && conformance.Generation.Revision < current.Generation.Revision) || (conformance.BindingSetID == current.BindingSetID && conformance.BindingSetRevision <= current.BindingSetRevision) {
			return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route current CAS or monotonic advancement conflicted")
		}
		projection.Ref.Revision = current.Ref.Revision + 1
		projection.Ref.Digest = ""
		projection.ProjectionDigest = ""
		projection, err = runtimeports.SealControlledOperationProviderRouteCurrentProjectionV2(projection)
		if err != nil {
			return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
		}
		if _, seen := s.currentHistory[currentID][projection.Ref.Watermark]; seen {
			return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route current rejected ABA watermark reuse")
		}
	}
	if !exists && expectedPrevious != 0 {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route current CAS conflicted")
	}
	s.currents[currentID] = projection
	if s.currentHistory[currentID] == nil {
		s.currentHistory[currentID] = map[core.Digest]struct{}{}
	}
	s.currentHistory[currentID][projection.Ref.Watermark] = struct{}{}
	if s.takeLostReplyLocked(loseCurrentReplyV2) {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "controlled Provider route current reply was lost")
	}
	return projection, nil
}

func (s *InMemoryControlledOperationProviderRouteFactStoreV2) InspectControlledOperationProviderRouteCurrentV2(_ context.Context, ref runtimeports.ControlledOperationProviderRouteCurrentRefV2) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	if err := ref.Validate(); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, exists := s.currents[ref.CurrentID]
	if !exists {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "controlled Provider route current is absent")
	}
	if current.Ref != ref {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route current ref drifted")
	}
	return current, nil
}

type ControlledOperationProviderRouteInputsCurrentReaderV2 interface {
	InspectCurrentControlledOperationProviderRouteInputsV2(context.Context, runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error)
}

type ControlledOperationProviderRouteCurrentReaderAdapterV2 struct {
	store  ControlledOperationProviderRouteFactStoreV2
	inputs ControlledOperationProviderRouteInputsCurrentReaderV2
	clock  func() time.Time
}

func NewControlledOperationProviderRouteCurrentReaderAdapterV2(store ControlledOperationProviderRouteFactStoreV2, inputs ControlledOperationProviderRouteInputsCurrentReaderV2, clock func() time.Time) (*ControlledOperationProviderRouteCurrentReaderAdapterV2, error) {
	if isNilControlledOperationProviderRouteDependencyV2(store) || isNilControlledOperationProviderRouteDependencyV2(inputs) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "controlled Provider route Store, current inputs Reader and clock are required")
	}
	return &ControlledOperationProviderRouteCurrentReaderAdapterV2{store: store, inputs: inputs, clock: clock}, nil
}

func (a *ControlledOperationProviderRouteCurrentReaderAdapterV2) InspectCurrentControlledOperationProviderRouteV2(ctx context.Context, ref runtimeports.ControlledOperationProviderRouteCurrentRefV2, matrix runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	if a == nil || isNilControlledOperationProviderRouteDependencyV2(a.store) || isNilControlledOperationProviderRouteDependencyV2(a.inputs) || a.clock == nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "controlled Provider route Reader is unavailable")
	}
	firstNow := a.clock()
	if firstNow.IsZero() {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled Provider route clock is unavailable")
	}
	first, err := a.store.InspectControlledOperationProviderRouteCurrentV2(ctx, ref)
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	if err := first.ValidateCurrent(ref, matrix, firstNow); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	refreshed, err := a.inputs.InspectCurrentControlledOperationProviderRouteInputsV2(ctx, first)
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	secondNow := a.clock()
	if secondNow.IsZero() || secondNow.Before(firstNow) {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled Provider route clock regressed")
	}
	if !sameRouteProjectionSemanticsV2(first, refreshed) {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route current inputs drifted")
	}
	if err := refreshed.ValidateCurrent(ref, matrix, secondNow); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	second, err := a.store.InspectControlledOperationProviderRouteCurrentV2(ctx, ref)
	if err != nil || second != first {
		if err != nil {
			return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
		}
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route changed during current read")
	}
	return refreshed, nil
}

func sameRouteProjectionSemanticsV2(left, right runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) bool {
	left.CheckedUnixNano, left.ExpiresUnixNano, left.ProjectionDigest = 0, 0, ""
	right.CheckedUnixNano, right.ExpiresUnixNano, right.ProjectionDigest = 0, 0, ""
	return reflect.DeepEqual(left, right)
}

func isNilControlledOperationProviderRouteDependencyV2(value any) bool {
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

var _ ControlledOperationProviderRouteFactStoreV2 = (*InMemoryControlledOperationProviderRouteFactStoreV2)(nil)
var _ runtimeports.ControlledOperationProviderRouteCurrentReaderV2 = (*ControlledOperationProviderRouteCurrentReaderAdapterV2)(nil)

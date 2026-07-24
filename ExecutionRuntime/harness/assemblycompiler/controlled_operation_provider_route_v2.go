package assemblycompiler

import (
	"context"
	"reflect"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ControlledOperationProviderRouteCompileResultV2 struct {
	Declaration               assemblycontract.ControlledOperationProviderRouteDeclarationV2        `json:"declaration"`
	PublisherManifest         runtimeports.ComponentManifestV2                                      `json:"publisher_manifest"`
	Extension                 runtimeports.GovernanceExtensionV2                                    `json:"extension"`
	GovernanceCatalogDigest   core.Digest                                                           `json:"governance_catalog_digest"`
	AssemblyInputDigest       core.Digest                                                           `json:"assembly_input_digest"`
	Manifest                  assemblycontract.AssemblyManifestV1                                   `json:"manifest"`
	Graph                     assemblycontract.CompiledHarnessGraphV1                               `json:"graph"`
	Generation                assemblycontract.AssemblyGenerationV1                                 `json:"generation"`
	Handoff                   assemblycontract.AssemblyHandoffV1                                    `json:"handoff"`
	ProviderTransportIdentity assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2 `json:"provider_transport_identity"`
	ProviderIdentity          assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2 `json:"provider_identity"`
	CompileDigest             core.Digest                                                           `json:"compile_digest"`
}

type ControlledOperationProviderLegacyActiveRouteFactV2 struct {
	RouteBindingRef assemblycontract.ObjectRefV1                                       `json:"route_binding_ref"`
	State           ControlledOperationProviderLegacyRouteStateV2                      `json:"state"`
	Record          assemblycontract.ControlledOperationProviderActiveRouteRecordV2    `json:"record"`
	WiringInventory assemblycontract.ControlledOperationProviderRouteWiringInventoryV2 `json:"wiring_inventory"`
	CheckedUnixNano int64                                                              `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                                              `json:"expires_unix_nano"`
	Digest          core.Digest                                                        `json:"digest"`
}

// ControlledOperationProviderLegacyRouteCurrentReaderV2 is the Harness-owner
// current read boundary for a sealed V1 RouteBinding. Compile never accepts a
// caller-supplied Fact as a substitute for this owner reread.
type ControlledOperationProviderLegacyRouteCurrentReaderV2 interface {
	InspectControlledOperationProviderLegacyRouteCurrentV2(context.Context, assemblycontract.ObjectRefV1) (ControlledOperationProviderLegacyActiveRouteFactV2, error)
}

type ControlledOperationProviderLegacyRouteStateV2 string

const (
	ControlledOperationProviderLegacyRouteActiveV2   ControlledOperationProviderLegacyRouteStateV2 = "active"
	ControlledOperationProviderLegacyRouteInactiveV2 ControlledOperationProviderLegacyRouteStateV2 = "inactive"
	ControlledOperationProviderLegacyRouteRevokedV2  ControlledOperationProviderLegacyRouteStateV2 = "revoked"
)

func (f ControlledOperationProviderLegacyActiveRouteFactV2) digestV2() (core.Digest, error) {
	f.Digest = ""
	return core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", assemblycontract.ControlledOperationProviderRouteContractVersionV2, "ControlledOperationProviderLegacyActiveRouteFactV2", f)
}

func SealControlledOperationProviderLegacyActiveRouteFactV2(f ControlledOperationProviderLegacyActiveRouteFactV2) (ControlledOperationProviderLegacyActiveRouteFactV2, error) {
	provided := f.Digest
	f.Digest = ""
	digest, err := f.digestV2()
	if err != nil {
		return ControlledOperationProviderLegacyActiveRouteFactV2{}, err
	}
	if provided != "" && provided != digest {
		return ControlledOperationProviderLegacyActiveRouteFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "legacy route current Fact supplied a wrong digest")
	}
	f.Digest = digest
	expectedBindingRef, bindingErr := ControlledOperationProviderLegacyRouteBindingRefV2(f.Record)
	if bindingErr != nil || f.RouteBindingRef.Validate() != nil || f.RouteBindingRef != expectedBindingRef || !validControlledOperationProviderLegacyRouteStateV2(f.State) || f.Record.Version != "v1" || f.Record.RouteID == "" || f.Record.DeclarationRef.Validate() != nil || !runtimeports.IsOperationScopeEvidenceActionMatrixKeyV3(f.Record.Matrix) || f.Record.TransportIdentity.Validate() != nil || f.Record.ProviderIdentity.Validate() != nil || f.Record.TransportBinding.Validate() != nil || f.Record.ProviderBinding.Validate() != nil || f.WiringInventory.Validate() != nil || f.CheckedUnixNano != f.WiringInventory.CheckedUnixNano || f.ExpiresUnixNano != f.WiringInventory.ExpiresUnixNano || f.CheckedUnixNano <= 0 || f.CheckedUnixNano >= f.ExpiresUnixNano {
		return ControlledOperationProviderLegacyActiveRouteFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "legacy route current Fact is incomplete")
	}
	if (f.State == ControlledOperationProviderLegacyRouteActiveV2) != f.Record.Active {
		return ControlledOperationProviderLegacyActiveRouteFactV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "legacy route state and record Active flag drifted")
	}
	matchingRecords, matchingActive := 0, 0
	for _, route := range f.WiringInventory.ActiveRoutes {
		if route.Version == f.Record.Version && route.RouteID == f.Record.RouteID {
			if route != f.Record {
				return ControlledOperationProviderLegacyActiveRouteFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "legacy route wiring proof drifted from its record")
			}
			matchingRecords++
			if route.Active {
				matchingActive++
			}
		}
		if f.State != ControlledOperationProviderLegacyRouteActiveV2 && route.Version == "v1" && route.Active && (route.Matrix == f.Record.Matrix || sameControlledOperationProviderLegacyAliasV2(route, f.Record)) {
			return ControlledOperationProviderLegacyActiveRouteFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "legacy route absence proof contains another active V1 route with the same matrix or alias identity")
		}
	}
	if (f.State == ControlledOperationProviderLegacyRouteActiveV2 && (matchingRecords != 1 || matchingActive != 1)) || (f.State != ControlledOperationProviderLegacyRouteActiveV2 && (matchingRecords != 1 || matchingActive != 0)) {
		return ControlledOperationProviderLegacyActiveRouteFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "legacy route state is not proven by sealed wiring")
	}
	return f, nil
}

// ControlledOperationProviderLegacyRouteBindingRefV2 binds a RouteBindingRef
// to the stable V1 route identity. Active is intentionally excluded so a
// state transition cannot change the subject reference.
func ControlledOperationProviderLegacyRouteBindingRefV2(record assemblycontract.ControlledOperationProviderActiveRouteRecordV2) (assemblycontract.ObjectRefV1, error) {
	if record.Version != "v1" || record.RouteID == "" || record.DeclarationRef.Validate() != nil || !runtimeports.IsOperationScopeEvidenceActionMatrixKeyV3(record.Matrix) || record.TransportIdentity.Validate() != nil || record.ProviderIdentity.Validate() != nil || record.TransportBinding.Validate() != nil || record.ProviderBinding.Validate() != nil {
		return assemblycontract.ObjectRefV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "legacy route record cannot derive an exact binding reference")
	}
	digest, err := core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", assemblycontract.ControlledOperationProviderRouteContractVersionV2, "ControlledOperationProviderLegacyRouteBindingRefV2", struct {
		Version           string
		RouteID           string
		DeclarationRef    runtimeports.ControlledOperationProviderRouteDeclarationRefV2
		Matrix            runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3
		TransportIdentity assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2
		ProviderIdentity  assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2
		TransportBinding  runtimeports.ProviderBindingRefV2
		ProviderBinding   runtimeports.ProviderBindingRefV2
	}{record.Version, record.RouteID, record.DeclarationRef, record.Matrix, record.TransportIdentity, record.ProviderIdentity, record.TransportBinding, record.ProviderBinding})
	if err != nil {
		return assemblycontract.ObjectRefV1{}, err
	}
	return assemblycontract.ObjectRefV1{ID: record.RouteID, Revision: record.DeclarationRef.Revision, Digest: digest}, nil
}

func sameControlledOperationProviderLegacyAliasV2(left, right assemblycontract.ControlledOperationProviderActiveRouteRecordV2) bool {
	return left.TransportIdentity == right.TransportIdentity || left.ProviderIdentity == right.ProviderIdentity || left.TransportBinding == right.TransportBinding || left.ProviderBinding == right.ProviderBinding
}

func validControlledOperationProviderLegacyRouteStateV2(value ControlledOperationProviderLegacyRouteStateV2) bool {
	switch value {
	case ControlledOperationProviderLegacyRouteActiveV2, ControlledOperationProviderLegacyRouteInactiveV2, ControlledOperationProviderLegacyRouteRevokedV2:
		return true
	default:
		return false
	}
}

func (r ControlledOperationProviderRouteCompileResultV2) DigestV2() (core.Digest, error) {
	r.CompileDigest = ""
	return core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", assemblycontract.ControlledOperationProviderRouteContractVersionV2, "ControlledOperationProviderRouteCompileResultV2", r)
}

func (r ControlledOperationProviderRouteCompileResultV2) ValidateV2() error {
	if err := r.Declaration.Validate(); err != nil {
		return err
	}
	if err := r.PublisherManifest.Validate(); err != nil {
		return err
	}
	if r.PublisherManifest.ComponentID != r.Declaration.PublisherComponent || r.PublisherManifest.Locality != runtimeports.LocalityHostControlPlane {
		return core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "controlled Provider route publisher identity or locality drifted")
	}
	version, err := core.ParseSemanticVersion(r.PublisherManifest.SemanticVersion)
	if err != nil || version.Major != 2 || len(r.PublisherManifest.ProvidedCapabilities) == 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidSemanticVersion, "controlled Provider route publisher requires a registered 2.x capability manifest")
	}
	if err := r.Extension.Payload.Validate(); err != nil {
		return err
	}
	if !r.Extension.Required || r.Extension.Key != assemblycontract.ControlledOperationProviderRouteExtensionKeyV2 || r.Extension.Payload.Inline == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownRequiredExtension, "controlled Provider route required extension is not exact")
	}
	decoded, err := assemblycontract.DecodeControlledOperationProviderRouteDeclarationV2(r.Extension.Payload.Inline)
	if err != nil || decoded != r.Declaration {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider route extension does not encode the exact declaration")
	}
	registeredExtension := false
	registeredSchema := false
	for _, extension := range r.PublisherManifest.Extensions {
		registeredExtension = registeredExtension || reflect.DeepEqual(extension, r.Extension)
	}
	for _, schema := range r.PublisherManifest.Schemas {
		registeredSchema = registeredSchema || schema == r.Extension.Payload.Schema
	}
	if !registeredExtension || !registeredSchema {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownRequiredExtension, "controlled Provider route extension is absent from its publisher manifest")
	}
	for _, digest := range []core.Digest{r.GovernanceCatalogDigest, r.AssemblyInputDigest, r.Manifest.Digest, r.Graph.Digest, r.Generation.Digest, r.Handoff.Digest, r.CompileDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	manifestDigest, manifestErr := assemblycontract.ManifestDigestV1(r.Manifest)
	graphDigest, graphErr := assemblycontract.GraphDigestV1(r.Graph)
	generationDigest, generationErr := assemblycontract.GenerationDigestV1(r.Generation)
	handoffDigest, handoffErr := assemblycontract.HandoffDigestV1(r.Handoff)
	if manifestErr != nil || graphErr != nil || generationErr != nil || handoffErr != nil || manifestDigest != r.Manifest.Digest || graphDigest != r.Graph.Digest || generationDigest != r.Generation.Digest || handoffDigest != r.Handoff.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider route compile artifacts drifted")
	}
	if r.Manifest.InputDigest != r.AssemblyInputDigest || r.Graph.InputDigest != r.AssemblyInputDigest || r.Generation.InputDigest != r.AssemblyInputDigest || r.Generation.ManifestDigest != r.Manifest.Digest || r.Generation.GraphDigest != r.Graph.Digest || r.Handoff.GenerationRef.ID != r.Generation.GenerationID || r.Handoff.GenerationRef.Revision != r.Generation.Revision || r.Handoff.GenerationRef.Digest != r.Generation.Digest || r.Handoff.ManifestDigest != r.Manifest.Digest || r.Handoff.GraphDigest != r.Graph.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route compile artifact lineage drifted")
	}
	transportIdentity, providerIdentity, err := expectedControlledOperationProviderRouteIdentitiesV2(r.Manifest.ComponentManifests, r.Manifest.PortSpecs, r.Manifest.Modules, r.Manifest.ProviderBindingCandidates, r.Declaration)
	if err != nil || r.ProviderTransportIdentity != transportIdentity || r.ProviderIdentity != providerIdentity {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route expected post-binding identities drifted from real PortSpecs")
	}
	digest, err := r.DigestV2()
	if err != nil || digest != r.CompileDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider route compile result digest drifted")
	}
	return nil
}

// CompileControlledOperationProviderRouteV2 performs the pre-binding Route
// pass. It neither changes AssemblyInputV1 nor creates Runtime/Harness current
// facts.
func CompileControlledOperationProviderRouteV2(input assemblycontract.AssemblyInputV1, catalog runtimeports.GovernanceCatalogV2) (ControlledOperationProviderRouteCompileResultV2, error) {
	return compileControlledOperationProviderRouteV2(input, catalog, nil)
}

// CompileControlledOperationProviderRouteWithLegacyCurrentReaderV2 resolves
// every V1 RouteBinding through a Harness-owner current Reader. Each subject
// is reread S1/S2 under a fresh clock before Compile may use its state.
func CompileControlledOperationProviderRouteWithLegacyCurrentReaderV2(ctx context.Context, input assemblycontract.AssemblyInputV1, catalog runtimeports.GovernanceCatalogV2, reader ControlledOperationProviderLegacyRouteCurrentReaderV2, clock func() time.Time) (ControlledOperationProviderRouteCompileResultV2, error) {
	if ctx == nil || reader == nil || clock == nil {
		return ControlledOperationProviderRouteCompileResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "legacy V1 RouteBinding compile requires context, owner current Reader, and clock")
	}
	legacy, err := inspectLegacyRoutesCurrentV2(ctx, input.RouteBindings, reader, clock)
	if err != nil {
		return ControlledOperationProviderRouteCompileResultV2{}, err
	}
	return compileControlledOperationProviderRouteV2(input, catalog, legacy)
}

func compileControlledOperationProviderRouteV2(input assemblycontract.AssemblyInputV1, catalog runtimeports.GovernanceCatalogV2, legacy []ControlledOperationProviderLegacyActiveRouteFactV2) (ControlledOperationProviderRouteCompileResultV2, error) {
	if err := input.Validate(); err != nil {
		return ControlledOperationProviderRouteCompileResultV2{}, err
	}
	digest, err := assemblycontract.AssemblyInputDigestV1(input)
	if err != nil || digest != input.Digest {
		return ControlledOperationProviderRouteCompileResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "controlled Provider route AssemblyInput digest drifted")
	}
	if err := catalog.Validate(); err != nil {
		return ControlledOperationProviderRouteCompileResultV2{}, err
	}

	type declarationCandidateV2 struct {
		publisher   runtimeports.ComponentManifestV2
		extension   runtimeports.GovernanceExtensionV2
		declaration assemblycontract.ControlledOperationProviderRouteDeclarationV2
	}
	declarations := make([]declarationCandidateV2, 0, 1)
	for _, manifest := range input.ComponentManifests {
		for _, candidate := range manifest.Extensions {
			if candidate.Key != assemblycontract.ControlledOperationProviderRouteExtensionKeyV2 {
				continue
			}
			if !candidate.Required || candidate.Payload.Inline == nil {
				return ControlledOperationProviderRouteCompileResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "controlled Provider route required inline extension is missing")
			}
			if err := candidate.Payload.Validate(); err != nil {
				return ControlledOperationProviderRouteCompileResultV2{}, err
			}
			declaration, err := assemblycontract.DecodeControlledOperationProviderRouteDeclarationV2(candidate.Payload.Inline)
			if err != nil {
				return ControlledOperationProviderRouteCompileResultV2{}, err
			}
			if declaration.PublisherComponent != manifest.ComponentID {
				return ControlledOperationProviderRouteCompileResultV2{}, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "controlled Provider route publisher drifted from its manifest")
			}
			declarations = append(declarations, declarationCandidateV2{publisher: manifest, extension: candidate, declaration: declaration})
		}
	}
	if len(declarations) == 0 {
		return ControlledOperationProviderRouteCompileResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "controlled Provider route required inline extension is missing")
	}
	mergedInputs := make([]assemblycontract.ControlledOperationProviderRouteDeclarationV2, len(declarations))
	for index := range declarations {
		mergedInputs[index] = declarations[index].declaration
	}
	declaration, err := MergeControlledOperationProviderRouteDeclarationsV2(mergedInputs)
	if err != nil {
		return ControlledOperationProviderRouteCompileResultV2{}, err
	}
	sort.Slice(declarations, func(i, j int) bool {
		return declarations[i].declaration.DeclarationDigest < declarations[j].declaration.DeclarationDigest
	})
	selected := declarations[0]
	for _, candidate := range declarations {
		if candidate.declaration == declaration {
			selected = candidate
			break
		}
	}
	publisher, extension := selected.publisher, selected.extension
	schema := extension.Payload.Schema
	if schema.Namespace != assemblycontract.ControlledOperationProviderRouteSchemaNamespaceV2 || schema.Name != assemblycontract.ControlledOperationProviderRouteSchemaNameV2 || schema.Version != assemblycontract.ControlledOperationProviderRouteSchemaVersionV2 || schema.MediaType != assemblycontract.ControlledOperationProviderRouteSchemaMediaTypeV2 {
		return ControlledOperationProviderRouteCompileResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownSchema, "controlled Provider route extension schema is not registered")
	}
	if err := runtimeports.ValidateManifestAgainstCatalogV2(publisher, catalog); err != nil {
		return ControlledOperationProviderRouteCompileResultV2{}, err
	}
	version, err := core.ParseSemanticVersion(publisher.SemanticVersion)
	if err != nil || version.Major != 2 || publisher.Locality != runtimeports.LocalityHostControlPlane || len(publisher.ProvidedCapabilities) == 0 {
		return ControlledOperationProviderRouteCompileResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "controlled Provider route publisher kind, locality, 2.x version or capability is not certified")
	}
	if err := validateControlledOperationProviderRouteGraphV2(input, declaration); err != nil {
		return ControlledOperationProviderRouteCompileResultV2{}, err
	}
	transportIdentity, providerIdentity, err := expectedControlledOperationProviderRouteIdentitiesV2(input.ComponentManifests, input.PortSpecs, input.Modules, input.ProviderBindingCandidates, declaration)
	if err != nil {
		return ControlledOperationProviderRouteCompileResultV2{}, err
	}
	if len(input.RouteBindings) != 0 {
		if err := validateLegacyActiveRoutesV2(input.RouteBindings, legacy, declaration, transportIdentity, providerIdentity, input.Digest); err != nil {
			return ControlledOperationProviderRouteCompileResultV2{}, err
		}
	}
	compiled, err := New().Compile(input)
	if err != nil || compiled.Manifest == nil || compiled.Graph == nil || compiled.Generation == nil || compiled.Handoff == nil {
		if err != nil {
			return ControlledOperationProviderRouteCompileResultV2{}, err
		}
		return ControlledOperationProviderRouteCompileResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonInvalidState, "controlled Provider route assembly compile did not produce sealed artifacts")
	}
	catalogDigest, err := catalog.DigestV2()
	if err != nil {
		return ControlledOperationProviderRouteCompileResultV2{}, err
	}
	result := ControlledOperationProviderRouteCompileResultV2{
		Declaration: declaration, PublisherManifest: publisher, Extension: extension,
		GovernanceCatalogDigest: catalogDigest, AssemblyInputDigest: input.Digest,
		Manifest: *compiled.Manifest, Graph: *compiled.Graph, Generation: *compiled.Generation, Handoff: *compiled.Handoff,
		ProviderTransportIdentity: transportIdentity, ProviderIdentity: providerIdentity,
	}
	result.CompileDigest, err = result.DigestV2()
	if err != nil {
		return ControlledOperationProviderRouteCompileResultV2{}, err
	}
	return result, result.ValidateV2()
}

func validateLegacyActiveRoutesV2(routeBindings []assemblycontract.ObjectRefV1, legacy []ControlledOperationProviderLegacyActiveRouteFactV2, declaration assemblycontract.ControlledOperationProviderRouteDeclarationV2, transportIdentity, providerIdentity assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2, assemblyInputDigest core.Digest) error {
	if len(routeBindings) != len(legacy) || len(legacy) == 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "every V1 RouteBinding requires an exact Harness owner-current Fact")
	}
	facts := make(map[assemblycontract.ObjectRefV1]ControlledOperationProviderLegacyActiveRouteFactV2, len(legacy))
	for _, candidate := range legacy {
		sealed, err := SealControlledOperationProviderLegacyActiveRouteFactV2(candidate)
		if err != nil || sealed.Digest != candidate.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "legacy route owner-current Fact drifted")
		}
		if _, exists := facts[sealed.RouteBindingRef]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "legacy route owner-current Fact is duplicated")
		}
		facts[sealed.RouteBindingRef] = sealed
	}
	for _, ref := range routeBindings {
		fact, ok := facts[ref]
		if !ok {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "V1 RouteBinding has no exact active-route fact")
		}
		if fact.State != ControlledOperationProviderLegacyRouteActiveV2 {
			continue
		}
		conflict, err := assemblycontract.SealControlledOperationProviderRouteConflictV2(assemblycontract.ControlledOperationProviderRouteConflictV2{
			ConflictCode: assemblycontract.ControlledOperationProviderRouteActiveVersionConflictV2,
			Phase:        assemblycontract.ControlledOperationProviderRoutePrebindingPhaseV2,
			Matrix:       declaration.Matrix,
			Left: assemblycontract.ControlledOperationProviderRouteConflictSideV2{
				Version: "v2-prebinding", DeclarationRef: declaration.RefV2(), ProviderTransport: transportIdentity, Provider: providerIdentity,
			},
			Right: assemblycontract.ControlledOperationProviderRouteConflictSideV2{
				Version: fact.Record.Version, DeclarationRef: fact.Record.DeclarationRef, ProviderTransport: fact.Record.TransportIdentity, Provider: fact.Record.ProviderIdentity,
				TransportBinding: bindingRefPointerV2(fact.Record.TransportBinding), ProviderBinding: bindingRefPointerV2(fact.Record.ProviderBinding),
			},
			AssemblyInputDigest: assemblyInputDigest,
		})
		if err != nil {
			return err
		}
		return &assemblycontract.ControlledOperationProviderRouteConflictErrorV2{Conflict: conflict}
	}
	return nil
}

func inspectLegacyRoutesCurrentV2(ctx context.Context, routeBindings []assemblycontract.ObjectRefV1, reader ControlledOperationProviderLegacyRouteCurrentReaderV2, clock func() time.Time) ([]ControlledOperationProviderLegacyActiveRouteFactV2, error) {
	if len(routeBindings) == 0 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "legacy current read requires at least one V1 RouteBinding")
	}
	result := make([]ControlledOperationProviderLegacyActiveRouteFactV2, 0, len(routeBindings))
	for _, ref := range routeBindings {
		if err := ref.Validate(); err != nil {
			return nil, err
		}
		s1, err := reader.InspectControlledOperationProviderLegacyRouteCurrentV2(ctx, ref)
		if err != nil {
			return nil, err
		}
		nowS1 := clock()
		sealedS1, err := validateControlledOperationProviderLegacyRouteCurrentV2(ref, s1, nowS1)
		if err != nil {
			return nil, err
		}
		s2, err := reader.InspectControlledOperationProviderLegacyRouteCurrentV2(ctx, ref)
		if err != nil {
			return nil, err
		}
		nowS2 := clock()
		if nowS2.Before(nowS1) {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "legacy V1 RouteBinding current clock regressed between S1 and S2")
		}
		sealedS2, err := validateControlledOperationProviderLegacyRouteCurrentV2(ref, s2, nowS2)
		if err != nil {
			return nil, err
		}
		if sealedS1.Digest != sealedS2.Digest || !reflect.DeepEqual(sealedS1, sealedS2) {
			return nil, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "legacy V1 RouteBinding current state drifted between S1 and S2")
		}
		result = append(result, sealedS2)
	}
	return result, nil
}

func validateControlledOperationProviderLegacyRouteCurrentV2(ref assemblycontract.ObjectRefV1, fact ControlledOperationProviderLegacyActiveRouteFactV2, now time.Time) (ControlledOperationProviderLegacyActiveRouteFactV2, error) {
	sealed, err := SealControlledOperationProviderLegacyActiveRouteFactV2(fact)
	if err != nil || sealed.Digest != fact.Digest || sealed.RouteBindingRef != ref {
		return ControlledOperationProviderLegacyActiveRouteFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "legacy V1 RouteBinding current Fact is not exact")
	}
	if now.IsZero() || now.UnixNano() < sealed.CheckedUnixNano {
		return ControlledOperationProviderLegacyActiveRouteFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "legacy V1 RouteBinding current clock regressed")
	}
	if !now.Before(time.Unix(0, sealed.ExpiresUnixNano)) {
		return ControlledOperationProviderLegacyActiveRouteFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "legacy V1 RouteBinding current proof expired")
	}
	return sealed, nil
}

func expectedControlledOperationProviderRouteIdentitiesV2(manifestValues []runtimeports.ComponentManifestV2, portValues []assemblycontract.PortSpecV1, moduleValues []assemblycontract.ModuleDescriptorV1, candidateValues []assemblycontract.ProviderBindingCandidateV1, declaration assemblycontract.ControlledOperationProviderRouteDeclarationV2) (assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2, assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2, error) {
	manifests := map[string]runtimeports.ComponentManifestV2{}
	ports := map[string]assemblycontract.PortSpecV1{}
	modules := map[string]assemblycontract.ModuleDescriptorV1{}
	candidates := map[string]assemblycontract.ProviderBindingCandidateV1{}
	for _, value := range manifestValues {
		manifests[string(value.ComponentID)] = value
	}
	for _, value := range portValues {
		ports[value.PortID] = value
	}
	for _, value := range moduleValues {
		modules[value.ModuleID] = value
	}
	for _, value := range candidateValues {
		candidates[value.CandidateID] = value
	}
	transportCandidate, transportOK := candidates[declaration.ProviderTransport.CandidateID]
	providerCandidate, providerOK := candidates[declaration.Provider.CandidateID]
	transport, transportValid := normalizeRouteCandidateIdentityV2(transportCandidate, manifests, ports, modules)
	provider, providerValid := normalizeRouteCandidateIdentityV2(providerCandidate, manifests, ports, modules)
	if !transportOK || !providerOK || !transportValid || !providerValid {
		return assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{}, assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route identities cannot be derived from exact compile inputs")
	}
	return transport, provider, nil
}

func validateControlledOperationProviderRouteGraphV2(input assemblycontract.AssemblyInputV1, declaration assemblycontract.ControlledOperationProviderRouteDeclarationV2) error {
	manifests := map[string]runtimeports.ComponentManifestV2{}
	modules := map[string]assemblycontract.ModuleDescriptorV1{}
	ports := map[string]assemblycontract.PortSpecV1{}
	candidates := map[string]assemblycontract.ProviderBindingCandidateV1{}
	for _, value := range input.ComponentManifests {
		manifests[string(value.ComponentID)] = value
	}
	for _, value := range input.Modules {
		modules[value.ModuleID] = value
	}
	for _, value := range input.PortSpecs {
		ports[value.PortID] = value
	}
	for _, value := range input.ProviderBindingCandidates {
		candidates[value.CandidateID] = value
	}
	if err := validateDeclaredPortV2(declaration.ApplicationToolPort, ports); err != nil {
		return err
	}
	if err := validateDeclaredPortV2(declaration.RuntimeGovernancePort, ports); err != nil {
		return err
	}
	endpoints := []assemblycontract.ControlledOperationProviderRouteEndpointV2{declaration.ToolAdapter, declaration.Gateway, declaration.ProviderTransport, declaration.Provider}
	for _, endpoint := range endpoints {
		candidate, ok := candidates[endpoint.CandidateID]
		if !ok || candidate.Digest != endpoint.CandidateDigest || candidate.ModuleRef != endpoint.ModuleRef || candidate.PortSpecRef != endpoint.PortSpecRef || candidate.ProviderRef != endpoint.ProviderRef {
			return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "controlled Provider route endpoint candidate drifted")
		}
		module, ok := modules[endpoint.ModuleRef]
		if !ok || module.ComponentManifestRef.ID != string(endpoint.ComponentID) || module.ArtifactDigest != endpoint.ArtifactDigest || module.Locality != endpoint.Locality {
			return core.NewError(core.ErrorConflict, core.ReasonComponentMismatch, "controlled Provider route endpoint module drifted")
		}
		manifest, ok := manifests[string(endpoint.ComponentID)]
		if !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "controlled Provider route endpoint manifest is missing")
		}
		manifestDigest, err := manifest.BindingDigestV2()
		if err != nil || manifestDigest != endpoint.ManifestDigest || manifest.ArtifactDigest != endpoint.ArtifactDigest || !manifestProvidesCapabilityV2(manifest, endpoint.Capability) || !moduleProvidesCapabilityV2(module, endpoint.Capability) {
			return core.NewError(core.ErrorConflict, core.ReasonComponentMismatch, "controlled Provider route endpoint manifest or capability drifted")
		}
		port, ok := ports[endpoint.PortSpecRef]
		if !ok || port.OwnerCapability != endpoint.Capability {
			return core.NewError(core.ErrorConflict, core.ReasonUnknownCapability, "controlled Provider route endpoint PortSpec capability drifted")
		}
	}
	readers := []assemblycontract.ControlledOperationProviderRouteReaderRefV2{declaration.PreparedCurrentReader, declaration.BoundaryCurrentReader, declaration.ProviderInspectReader}
	for _, reader := range readers {
		manifest, ok := manifests[string(reader.ComponentID)]
		if !ok || !manifestProvidesCapabilityV2(manifest, reader.Capability) {
			return core.NewError(core.ErrorConflict, core.ReasonUnknownCapability, "controlled Provider route Reader capability is missing")
		}
		manifestDigest, err := manifest.BindingDigestV2()
		port, portOK := ports[reader.PortSpecID]
		if err != nil || manifestDigest != reader.ManifestDigest || manifest.ArtifactDigest != reader.ArtifactDigest || !portOK || port.OwnerCapability != reader.Capability || port.RequestSchema != reader.RequestSchema || port.ResponseSchema != reader.ProjectionSchema {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route Reader proof drifted")
		}
		portDigest, err := assemblycontract.PortSpecDigestForControlledOperationProviderRouteV2(port)
		if err != nil || portDigest != reader.PortSpecDigest {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider route Reader PortSpec digest drifted")
		}
	}
	return scanControlledOperationProviderBypassesV2(input, declaration, manifests, ports, modules, candidates)
}

// MergeControlledOperationProviderRouteDeclarationsV2 is the pure pre-binding
// merge contract. Exact duplicates are idempotent; the closed matrix never
// applies priority, first-wins or partial field merge.
func MergeControlledOperationProviderRouteDeclarationsV2(values []assemblycontract.ControlledOperationProviderRouteDeclarationV2) (assemblycontract.ControlledOperationProviderRouteDeclarationV2, error) {
	if len(values) == 0 {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "controlled Provider route declaration set is empty")
	}
	values = append([]assemblycontract.ControlledOperationProviderRouteDeclarationV2(nil), values...)
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, err
		}
	}
	sort.Slice(values, func(i, j int) bool {
		left, right := values[i].RefV2(), values[j].RefV2()
		if left.RouteID != right.RouteID {
			return left.RouteID < right.RouteID
		}
		if left.Revision != right.Revision {
			return left.Revision < right.Revision
		}
		if left.PublisherComponentID != right.PublisherComponentID {
			return left.PublisherComponentID < right.PublisherComponentID
		}
		return left.DeclarationDigest < right.DeclarationDigest
	})
	selected := values[0]
	for _, candidate := range values[1:] {
		if candidate == selected {
			continue
		}
		conflict, err := assemblycontract.SealControlledOperationProviderRouteConflictV2(assemblycontract.ControlledOperationProviderRouteConflictV2{
			ConflictCode: assemblycontract.ControlledOperationProviderRouteDeclarationConflictV2,
			Phase:        assemblycontract.ControlledOperationProviderRouteDeclarationPhaseV2,
			Matrix:       selected.Matrix,
			Left:         assemblycontract.ControlledOperationProviderRouteConflictSideV2{Version: "v2", DeclarationRef: selected.RefV2()},
			Right:        assemblycontract.ControlledOperationProviderRouteConflictSideV2{Version: "v2", DeclarationRef: candidate.RefV2()},
		})
		if err != nil {
			return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, err
		}
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, &assemblycontract.ControlledOperationProviderRouteConflictErrorV2{Conflict: conflict}
	}
	return selected, nil
}

func validateDeclaredPortV2(ref assemblycontract.ControlledOperationProviderRoutePortRefV2, ports map[string]assemblycontract.PortSpecV1) error {
	port, ok := ports[ref.PortID]
	if !ok || port.OwnerCapability != ref.OwnerCapability || port.RequestSchema != ref.RequestSchema || port.ResponseSchema != ref.ResponseSchema {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route declared PortSpec drifted")
	}
	digest, err := assemblycontract.PortSpecDigestForControlledOperationProviderRouteV2(port)
	if err != nil || digest != ref.PortDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider route declared PortSpec digest drifted")
	}
	return nil
}

func scanControlledOperationProviderBypassesV2(input assemblycontract.AssemblyInputV1, declaration assemblycontract.ControlledOperationProviderRouteDeclarationV2, manifests map[string]runtimeports.ComponentManifestV2, ports map[string]assemblycontract.PortSpecV1, modules map[string]assemblycontract.ModuleDescriptorV1, candidates map[string]assemblycontract.ProviderBindingCandidateV1) error {
	transport, provider := declaration.ProviderTransport, declaration.Provider
	expectedTransport, expectedProvider, err := expectedControlledOperationProviderRouteIdentitiesV2(input.ComponentManifests, input.PortSpecs, input.Modules, input.ProviderBindingCandidates, declaration)
	if err != nil {
		return err
	}
	protected := []assemblycontract.ControlledOperationProviderRouteEndpointV2{transport, provider}
	expectedCandidates := map[string]assemblycontract.ControlledOperationProviderRouteEndpointV2{}
	for _, endpoint := range []assemblycontract.ControlledOperationProviderRouteEndpointV2{declaration.ToolAdapter, declaration.Gateway, transport, provider} {
		expectedCandidates[endpoint.CandidateID] = endpoint
	}
	for _, candidate := range input.ProviderBindingCandidates {
		if expected, ok := expectedCandidates[candidate.CandidateID]; ok {
			if candidate.ProviderRef != expected.ProviderRef || candidate.ModuleRef != expected.ModuleRef || candidate.PortSpecRef != expected.PortSpecRef || candidate.Digest != expected.CandidateDigest {
				return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "controlled Provider route candidate identity drifted")
			}
			continue
		}
		if routeCandidateConflictsV2(candidate, transport, manifests, ports, modules, candidates) {
			alias, _ := normalizeRouteCandidateIdentityV2(candidate, manifests, ports, modules)
			return controlledOperationProviderAliasConflictV2(declaration, expectedTransport, expectedProvider, alias, true, assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasCandidateSurfaceV2, Ref: candidate.CandidateID, ModuleRef: candidate.ModuleRef, PortSpecRef: candidate.PortSpecRef}, input.Digest)
		}
		if routeCandidateConflictsV2(candidate, provider, manifests, ports, modules, candidates) {
			alias, _ := normalizeRouteCandidateIdentityV2(candidate, manifests, ports, modules)
			return controlledOperationProviderAliasConflictV2(declaration, expectedTransport, expectedProvider, alias, false, assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasCandidateSurfaceV2, Ref: candidate.CandidateID, ModuleRef: candidate.ModuleRef, PortSpecRef: candidate.PortSpecRef}, input.Digest)
		}
	}
	allowedPorts := map[string]struct{}{}
	for _, value := range []string{declaration.ApplicationToolPort.PortID, declaration.RuntimeGovernancePort.PortID, declaration.ToolAdapter.PortSpecRef, declaration.Gateway.PortSpecRef, transport.PortSpecRef, provider.PortSpecRef, declaration.PreparedCurrentReader.PortSpecID, declaration.BoundaryCurrentReader.PortSpecID, declaration.ProviderInspectReader.PortSpecID} {
		allowedPorts[value] = struct{}{}
	}
	guardedCapabilities := map[runtimeports.CapabilityNameV2]struct{}{runtimeports.ControlledOperationProviderTransportCapabilityV2: {}, runtimeports.CapabilityNameV2(declaration.Matrix.EffectKind): {}}
	for id, port := range ports {
		if _, guarded := guardedCapabilities[port.OwnerCapability]; guarded {
			if _, allowed := allowedPorts[id]; !allowed {
				return controlledOperationProviderAliasConflictV2(declaration, expectedTransport, expectedProvider, assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{}, port.OwnerCapability == expectedTransport.Capability, assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasPortSurfaceV2, Ref: id, PortSpecRef: port.PortID, Capability: string(port.OwnerCapability)}, input.Digest)
			}
		}
	}
	for _, contribution := range input.SlotContributions {
		if routeSlotContributionConflictsV2(contribution, protected, manifests, ports, modules, candidates) {
			return controlledOperationProviderAliasConflictV2(declaration, expectedTransport, expectedProvider, assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{}, true, assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasSlotSurfaceV2, Ref: routeSurfaceRefV2(contribution.ContributionID, contribution.SlotRef, contribution.ProviderCandidateRef), ModuleRef: contribution.ModuleRef, PortSpecRef: contribution.PortSpecRef, Capability: string(contribution.CapabilityRef)}, input.Digest)
		}
	}
	for _, factory := range input.Factories {
		if _, guarded := guardedCapabilities[factory.OutputCapability]; guarded || routeModuleRefConflictsV2(factory.ModuleRef, protected, manifests, modules) {
			return controlledOperationProviderAliasConflictV2(declaration, expectedTransport, expectedProvider, assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{}, factory.OutputCapability == expectedTransport.Capability, assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasFactorySurfaceV2, Ref: routeSurfaceRefV2(factory.FactoryID, factory.ModuleRef, string(factory.OutputCapability)), ModuleRef: factory.ModuleRef, Capability: string(factory.OutputCapability)}, input.Digest)
		}
	}
	for _, dependency := range input.Dependencies {
		if _, guarded := guardedCapabilities[dependency.Capability]; guarded || routeReferenceConflictsV2(dependency.FromRef, protected, manifests, ports, modules, candidates) || routeReferenceConflictsV2(dependency.ToRef, protected, manifests, ports, modules, candidates) {
			return controlledOperationProviderAliasConflictV2(declaration, expectedTransport, expectedProvider, assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{}, dependency.Capability == expectedTransport.Capability, assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasDependencySurfaceV2, Ref: routeSurfaceRefV2(dependency.FromRef, dependency.ToRef, string(dependency.Capability)), PortSpecRef: dependency.ToRef, Capability: string(dependency.Capability)}, input.Digest)
		}
	}
	for _, phase := range input.PhaseContributions {
		if routePhaseContributionConflictsV2(phase, protected, manifests, ports, modules, candidates) {
			return controlledOperationProviderAliasConflictV2(declaration, expectedTransport, expectedProvider, assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{}, true, assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasPhaseSurfaceV2, Ref: routeSurfaceRefV2(phase.ContributionID, phase.HookFaceRef, phase.HandlerDescriptorRef.ID), ModuleRef: phase.ModuleRef, PortSpecRef: phase.HookFaceRef, Capability: string(phase.Capability)}, input.Digest)
		}
	}
	for id, candidate := range candidates {
		if id != candidate.CandidateID {
			return controlledOperationProviderAliasConflictV2(declaration, expectedTransport, expectedProvider, assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{}, true, assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasCandidateSurfaceV2, Ref: routeSurfaceRefV2(id, candidate.CandidateID), ModuleRef: candidate.ModuleRef, PortSpecRef: candidate.PortSpecRef}, input.Digest)
		}
	}
	return nil
}

func controlledOperationProviderAliasConflictV2(declaration assemblycontract.ControlledOperationProviderRouteDeclarationV2, expectedTransport, expectedProvider, alias assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2, transportSide bool, surface assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2, assemblyInputDigest core.Digest) error {
	sealedSurface, err := assemblycontract.SealControlledOperationProviderRouteAliasSurfaceV2(surface)
	if err != nil {
		return err
	}
	rightTransport, rightProvider := assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{}, assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{}
	if alias.Validate() == nil {
		rightTransport, rightProvider = expectedTransport, expectedProvider
		if transportSide {
			rightTransport = alias
		} else {
			rightProvider = alias
		}
	}
	conflict, err := assemblycontract.SealControlledOperationProviderRouteConflictV2(assemblycontract.ControlledOperationProviderRouteConflictV2{
		ConflictCode: assemblycontract.ControlledOperationProviderRouteAliasConflictV2,
		Phase:        assemblycontract.ControlledOperationProviderRoutePrebindingPhaseV2,
		Matrix:       declaration.Matrix,
		Left: assemblycontract.ControlledOperationProviderRouteConflictSideV2{
			Version: "v2-protected", DeclarationRef: declaration.RefV2(), ProviderTransport: expectedTransport, Provider: expectedProvider,
		},
		Right: assemblycontract.ControlledOperationProviderRouteConflictSideV2{
			Version: "graph-alias", DeclarationRef: declaration.RefV2(), ProviderTransport: rightTransport, Provider: rightProvider,
		},
		AliasSurface:        &sealedSurface,
		AssemblyInputDigest: assemblyInputDigest,
	})
	if err != nil {
		return err
	}
	return &assemblycontract.ControlledOperationProviderRouteConflictErrorV2{Conflict: conflict}
}

func routeSurfaceRefV2(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "unknown-surface"
}

func routeCandidateConflictsV2(candidate assemblycontract.ProviderBindingCandidateV1, protected assemblycontract.ControlledOperationProviderRouteEndpointV2, manifests map[string]runtimeports.ComponentManifestV2, ports map[string]assemblycontract.PortSpecV1, modules map[string]assemblycontract.ModuleDescriptorV1, candidates map[string]assemblycontract.ProviderBindingCandidateV1) bool {
	left, leftOK := normalizeRouteCandidateIdentityV2(candidate, manifests, ports, modules)
	protectedCandidate, protectedOK := candidates[protected.CandidateID]
	right, rightOK := normalizeRouteCandidateIdentityV2(protectedCandidate, manifests, ports, modules)
	if !leftOK || !rightOK || !protectedOK {
		return true
	}
	if left.ProviderRef == right.ProviderRef || left.CandidateID == right.CandidateID || left.PortSpecRef == right.PortSpecRef || left.ModuleRef == right.ModuleRef {
		return true
	}
	if left.ComponentID == right.ComponentID && left.ComponentManifestDigest == right.ComponentManifestDigest && left.ArtifactDigest == right.ArtifactDigest && (left.Capability == right.Capability || left.PortSpecDigest == right.PortSpecDigest) {
		return true
	}
	if left.Capability == right.Capability || (left.ConflictDomain != "" && left.ConflictDomain == right.ConflictDomain) {
		return true
	}
	return candidate.SlotRef == protectedCandidate.SlotRef
}

func normalizeRouteCandidateIdentityV2(candidate assemblycontract.ProviderBindingCandidateV1, manifests map[string]runtimeports.ComponentManifestV2, ports map[string]assemblycontract.PortSpecV1, modules map[string]assemblycontract.ModuleDescriptorV1) (assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2, bool) {
	module, moduleOK := modules[candidate.ModuleRef]
	port, portOK := ports[candidate.PortSpecRef]
	manifest, manifestOK := manifests[module.ComponentManifestRef.ID]
	if !moduleOK || !portOK || !manifestOK {
		return assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{}, false
	}
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil || manifestDigest != module.ComponentManifestRef.Digest {
		return assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{}, false
	}
	portDigest, err := assemblycontract.PortSpecDigestForControlledOperationProviderRouteV2(port)
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{}, false
	}
	identity := assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{
		ProviderRef: candidate.ProviderRef, CandidateID: candidate.CandidateID, ModuleRef: candidate.ModuleRef,
		ComponentID: manifest.ComponentID, ComponentManifestDigest: manifestDigest, ArtifactDigest: module.ArtifactDigest,
		Capability: port.OwnerCapability, PortSpecRef: candidate.PortSpecRef, PortSpecDigest: portDigest, ConflictDomain: port.ConflictDomainRule,
	}
	return identity, identity.Validate() == nil
}

func routeModuleRefConflictsV2(moduleRef string, protected []assemblycontract.ControlledOperationProviderRouteEndpointV2, manifests map[string]runtimeports.ComponentManifestV2, modules map[string]assemblycontract.ModuleDescriptorV1) bool {
	module, ok := modules[moduleRef]
	if !ok {
		return false
	}
	for _, endpoint := range protected {
		other, exists := modules[endpoint.ModuleRef]
		if !exists {
			return true
		}
		if moduleRef == endpoint.ModuleRef || (module.ComponentManifestRef.ID == other.ComponentManifestRef.ID && module.ComponentManifestRef.Digest == other.ComponentManifestRef.Digest && module.ArtifactDigest == other.ArtifactDigest) {
			return true
		}
		manifest, manifestOK := manifests[module.ComponentManifestRef.ID]
		if manifestOK && manifest.ComponentID == endpoint.ComponentID && manifest.ArtifactDigest == endpoint.ArtifactDigest {
			return true
		}
	}
	return false
}

func routeReferenceConflictsV2(ref string, protected []assemblycontract.ControlledOperationProviderRouteEndpointV2, manifests map[string]runtimeports.ComponentManifestV2, ports map[string]assemblycontract.PortSpecV1, modules map[string]assemblycontract.ModuleDescriptorV1, candidates map[string]assemblycontract.ProviderBindingCandidateV1) bool {
	if routeModuleRefConflictsV2(ref, protected, manifests, modules) {
		return true
	}
	for _, endpoint := range protected {
		if ref == endpoint.CandidateID || ref == endpoint.PortSpecRef || ref == endpoint.ProviderRef.ID {
			return true
		}
		if candidate, ok := candidates[ref]; ok && routeCandidateConflictsV2(candidate, endpoint, manifests, ports, modules, candidates) {
			return true
		}
		if port, ok := ports[ref]; ok {
			protectedPort := ports[endpoint.PortSpecRef]
			if port.PortID == protectedPort.PortID || port.OwnerCapability == protectedPort.OwnerCapability || (port.ConflictDomainRule != "" && port.ConflictDomainRule == protectedPort.ConflictDomainRule) {
				return true
			}
		}
	}
	return false
}

func routeSlotContributionConflictsV2(contribution assemblycontract.SlotContributionV1, protected []assemblycontract.ControlledOperationProviderRouteEndpointV2, manifests map[string]runtimeports.ComponentManifestV2, ports map[string]assemblycontract.PortSpecV1, modules map[string]assemblycontract.ModuleDescriptorV1, candidates map[string]assemblycontract.ProviderBindingCandidateV1) bool {
	if routeReferenceConflictsV2(contribution.ModuleRef, protected, manifests, ports, modules, candidates) || routeReferenceConflictsV2(contribution.PortSpecRef, protected, manifests, ports, modules, candidates) || routeReferenceConflictsV2(contribution.ProviderCandidateRef, protected, manifests, ports, modules, candidates) {
		return true
	}
	for _, endpoint := range protected {
		if contribution.CapabilityRef == endpoint.Capability {
			return true
		}
	}
	return false
}

func routePhaseContributionConflictsV2(phase assemblycontract.PhaseContributionV1, protected []assemblycontract.ControlledOperationProviderRouteEndpointV2, manifests map[string]runtimeports.ComponentManifestV2, ports map[string]assemblycontract.PortSpecV1, modules map[string]assemblycontract.ModuleDescriptorV1, candidates map[string]assemblycontract.ProviderBindingCandidateV1) bool {
	if routeModuleRefConflictsV2(phase.ModuleRef, protected, manifests, modules) || routeReferenceConflictsV2(phase.HookFaceRef, protected, manifests, ports, modules, candidates) {
		return true
	}
	for _, endpoint := range protected {
		if phase.HandlerDescriptorRef == endpoint.ProviderRef || phase.HandlerDescriptorRef.ID == endpoint.ProviderRef.ID || (phase.HandlerDescriptorRef.Digest == endpoint.ProviderRef.Digest && phase.HandlerDescriptorRef.Digest != "") {
			return true
		}
	}
	return false
}

// ValidateControlledOperationProviderWiringV2 closes the whole post-binding
// graph from the Application Port through the actual Provider. No raw handle
// is returned.
func ValidateControlledOperationProviderWiringV2(declaration assemblycontract.ControlledOperationProviderRouteDeclarationV2, transportIdentity, providerIdentity assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2, inventory assemblycontract.ControlledOperationProviderRouteWiringInventoryV2, bindings [7]runtimeports.ProviderBindingRefV2, assemblyInputDigest, graphDigest core.Digest, nowUnixNano int64) error {
	if err := declaration.Validate(); err != nil {
		return err
	}
	if err := inventory.Validate(); err != nil {
		return err
	}
	if err := graphDigest.Validate(); err != nil {
		return err
	}
	if err := assemblyInputDigest.Validate(); err != nil {
		return err
	}
	if err := transportIdentity.Validate(); err != nil {
		return err
	}
	if err := providerIdentity.Validate(); err != nil {
		return err
	}
	if nowUnixNano < inventory.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled Provider wiring clock regressed")
	}
	if nowUnixNano >= inventory.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled Provider wiring inventory expired")
	}
	transport, provider := bindings[2], bindings[6]
	expected := []assemblycontract.ControlledOperationProviderRouteWiringEdgeV2{
		{SourceRole: assemblycontract.ControlledOperationApplicationToolPortRoleV2, SourcePortSpecRef: declaration.ApplicationToolPort.PortID, TargetRole: assemblycontract.ControlledOperationToolAdapterRoleV2, TargetComponentID: bindings[0].ComponentID, TargetPortSpecRef: declaration.ToolAdapter.PortSpecRef, ProviderRef: declaration.ToolAdapter.ProviderRef, ModuleRef: declaration.ToolAdapter.ModuleRef, CandidateID: declaration.ToolAdapter.CandidateID, Binding: bindings[0]},
		{SourceRole: assemblycontract.ControlledOperationToolAdapterRoleV2, SourceComponentID: bindings[0].ComponentID, SourcePortSpecRef: declaration.ToolAdapter.PortSpecRef, TargetRole: assemblycontract.ControlledOperationRuntimeGovernanceRoleV2, TargetPortSpecRef: declaration.RuntimeGovernancePort.PortID},
		{SourceRole: assemblycontract.ControlledOperationRuntimeGovernanceRoleV2, SourcePortSpecRef: declaration.RuntimeGovernancePort.PortID, TargetRole: assemblycontract.ControlledOperationRuntimeGatewayRoleV2, TargetComponentID: bindings[1].ComponentID, TargetPortSpecRef: declaration.Gateway.PortSpecRef, ProviderRef: declaration.Gateway.ProviderRef, ModuleRef: declaration.Gateway.ModuleRef, CandidateID: declaration.Gateway.CandidateID, Binding: bindings[1]},
		{SourceRole: assemblycontract.ControlledOperationRuntimeGatewayRoleV2, SourceComponentID: bindings[1].ComponentID, SourcePortSpecRef: declaration.Gateway.PortSpecRef, TargetRole: assemblycontract.ControlledOperationProviderTransportRoleV2, TargetComponentID: transport.ComponentID, TargetPortSpecRef: declaration.ProviderTransport.PortSpecRef, ProviderRef: declaration.ProviderTransport.ProviderRef, ModuleRef: declaration.ProviderTransport.ModuleRef, CandidateID: declaration.ProviderTransport.CandidateID, Binding: transport},
		{SourceRole: assemblycontract.ControlledOperationProviderTransportRoleV2, SourceComponentID: transport.ComponentID, SourcePortSpecRef: declaration.ProviderTransport.PortSpecRef, TargetRole: assemblycontract.ControlledOperationProviderRoleV2, TargetComponentID: provider.ComponentID, TargetPortSpecRef: declaration.Provider.PortSpecRef, ProviderRef: declaration.Provider.ProviderRef, ModuleRef: declaration.Provider.ModuleRef, CandidateID: declaration.Provider.CandidateID, Binding: provider},
	}
	sealed := assemblycontract.NormalizeControlledOperationProviderRouteWiringInventoryV2(inventory)
	want := inventory
	want.Edges = expected
	want = assemblycontract.NormalizeControlledOperationProviderRouteWiringInventoryV2(want)
	if !reflect.DeepEqual(sealed.Edges, want.Edges) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "controlled Provider wiring contains a raw or alias injection edge")
	}
	activeV2 := 0
	var expectedRecord assemblycontract.ControlledOperationProviderActiveRouteRecordV2
	for _, route := range sealed.ActiveRoutes {
		if !route.Active {
			continue
		}
		if route.Version == "v2" && route.RouteID == declaration.RouteID && route.DeclarationRef == declaration.RefV2() && route.Matrix == declaration.Matrix && route.TransportBinding == transport && route.ProviderBinding == provider && route.TransportIdentity == transportIdentity && route.ProviderIdentity == providerIdentity {
			expectedRecord = route
			activeV2++
		}
	}
	if activeV2 != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "controlled Provider route requires exactly one active V2 route")
	}
	for _, route := range sealed.ActiveRoutes {
		if !route.Active || (route.Version == "v2" && route.RouteID == declaration.RouteID && route.DeclarationRef == declaration.RefV2() && route.Matrix == declaration.Matrix && route.TransportBinding == transport && route.ProviderBinding == provider && route.TransportIdentity == transportIdentity && route.ProviderIdentity == providerIdentity) {
			continue
		}
		conflict, err := assemblycontract.SealControlledOperationProviderRouteConflictV2(assemblycontract.ControlledOperationProviderRouteConflictV2{
			ConflictCode:          assemblycontract.ControlledOperationProviderRouteActiveVersionConflictV2,
			Phase:                 assemblycontract.ControlledOperationProviderRoutePostbindingPhaseV2,
			Matrix:                declaration.Matrix,
			Left:                  assemblycontract.ControlledOperationProviderRouteConflictSideV2{Version: "v2", DeclarationRef: declaration.RefV2(), ProviderTransport: expectedRecord.TransportIdentity, Provider: expectedRecord.ProviderIdentity, TransportBinding: bindingRefPointerV2(expectedRecord.TransportBinding), ProviderBinding: bindingRefPointerV2(expectedRecord.ProviderBinding)},
			Right:                 assemblycontract.ControlledOperationProviderRouteConflictSideV2{Version: route.Version, DeclarationRef: route.DeclarationRef, ProviderTransport: route.TransportIdentity, Provider: route.ProviderIdentity, TransportBinding: bindingRefPointerV2(route.TransportBinding), ProviderBinding: bindingRefPointerV2(route.ProviderBinding)},
			AssemblyInputDigest:   assemblyInputDigest,
			GraphDigest:           graphDigest,
			WiringInventoryDigest: inventory.Digest,
		})
		if err != nil {
			return err
		}
		return &assemblycontract.ControlledOperationProviderRouteConflictErrorV2{Conflict: conflict}
	}
	return nil
}

func bindingRefPointerV2(value runtimeports.ProviderBindingRefV2) *runtimeports.ProviderBindingRefV2 {
	copyValue := value
	return &copyValue
}

func manifestProvidesCapabilityV2(manifest runtimeports.ComponentManifestV2, capability runtimeports.CapabilityNameV2) bool {
	for _, provided := range manifest.ProvidedCapabilities {
		if provided.Capability == capability {
			return true
		}
	}
	return false
}

func moduleProvidesCapabilityV2(module assemblycontract.ModuleDescriptorV1, capability runtimeports.CapabilityNameV2) bool {
	for _, provided := range module.Capabilities {
		if provided == capability {
			return true
		}
	}
	return false
}

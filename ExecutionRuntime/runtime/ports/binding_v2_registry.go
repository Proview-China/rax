package ports

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ComponentProbeObservationV2 struct {
	Manifest       ComponentManifestV2 `json:"manifest"`
	ManifestDigest core.Digest         `json:"manifest_digest"`
	ObservedAt     time.Time           `json:"observed_at"`
}

// ComponentRegistrationObservationV2 acknowledges discovery only. It has no
// observation timestamp and cannot be used as probe or certification evidence.
type ComponentRegistrationObservationV2 struct {
	Manifest       ComponentManifestV2 `json:"manifest"`
	ManifestDigest core.Digest         `json:"manifest_digest"`
}

func (o ComponentRegistrationObservationV2) Validate() error {
	if err := o.Manifest.Validate(); err != nil {
		return err
	}
	digest, err := o.Manifest.BindingDigestV2()
	if err != nil {
		return err
	}
	if digest != o.ManifestDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "registration observation manifest digest drifted")
	}
	return nil
}

func (o ComponentProbeObservationV2) Validate() error {
	if err := (ComponentRegistrationObservationV2{Manifest: o.Manifest, ManifestDigest: o.ManifestDigest}).Validate(); err != nil {
		return err
	}
	if o.ObservedAt.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceUnavailable, "probe observation requires injected observation time")
	}
	return nil
}

type registeredComponentV2 struct {
	adapter        DescriberV2
	componentID    ComponentIDV2
	kind           ComponentKindV2
	manifestDigest core.Digest
}

// ComponentRegistryV2 is a discovery/probe surface, never an authority grant.
// Certification and binding are exclusively represented by BindingFactPortV2.
type ComponentRegistryV2 struct {
	mu         sync.RWMutex
	catalog    GovernanceCatalogV2
	components map[ComponentIDV2]registeredComponentV2
}

func NewComponentRegistryV2(catalog GovernanceCatalogV2) (*ComponentRegistryV2, error) {
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	return &ComponentRegistryV2{catalog: catalog, components: make(map[ComponentIDV2]registeredComponentV2)}, nil
}

func (r *ComponentRegistryV2) Register(ctx context.Context, adapter DescriberV2) (ComponentRegistrationObservationV2, error) {
	if adapter == nil {
		return ComponentRegistrationObservationV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "v2 component adapter is required")
	}
	manifest, err := adapter.DescribeV2(ctx)
	if err != nil {
		return ComponentRegistrationObservationV2{}, err
	}
	if err := ValidateManifestAgainstCatalogV2(manifest, r.catalog); err != nil {
		return ComponentRegistrationObservationV2{}, err
	}
	digest, err := manifest.BindingDigestV2()
	if err != nil {
		return ComponentRegistrationObservationV2{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.components[manifest.ComponentID]; exists {
		return ComponentRegistrationObservationV2{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "v2 component id is already registered")
	}
	r.components[manifest.ComponentID] = registeredComponentV2{adapter: adapter, componentID: manifest.ComponentID, kind: manifest.Kind, manifestDigest: digest}
	return ComponentRegistrationObservationV2{Manifest: cloneManifestV2(manifest), ManifestDigest: digest}, nil
}

func (r *ComponentRegistryV2) Probe(ctx context.Context, componentID ComponentIDV2, now time.Time) (ComponentProbeObservationV2, error) {
	if err := ValidateNamespacedNameV2(NamespacedNameV2(componentID)); err != nil {
		return ComponentProbeObservationV2{}, err
	}
	if now.IsZero() {
		return ComponentProbeObservationV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "probe time must come from an injected clock")
	}
	r.mu.RLock()
	registered, exists := r.components[componentID]
	r.mu.RUnlock()
	if !exists {
		return ComponentProbeObservationV2{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "v2 component is not registered")
	}
	manifest, err := registered.adapter.DescribeV2(ctx)
	if err != nil {
		return ComponentProbeObservationV2{}, err
	}
	if err := ValidateManifestAgainstCatalogV2(manifest, r.catalog); err != nil {
		return ComponentProbeObservationV2{}, err
	}
	digest, err := manifest.BindingDigestV2()
	if err != nil {
		return ComponentProbeObservationV2{}, err
	}
	if manifest.ComponentID != registered.componentID || manifest.Kind != registered.kind || digest != registered.manifestDigest {
		return ComponentProbeObservationV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "registered v2 component manifest drifted after registration")
	}
	return ComponentProbeObservationV2{Manifest: cloneManifestV2(manifest), ManifestDigest: digest, ObservedAt: now}, nil
}

func (r *ComponentRegistryV2) RegisteredIDs() []ComponentIDV2 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]ComponentIDV2, 0, len(r.components))
	for id := range r.components {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// AdaptV1DescriptorToManifestV2 is a compatibility bridge only. It cannot
// certify or bind a component, and it caps conformance at restricted_controlled.
func AdaptV1DescriptorToManifestV2(descriptor ComponentDescriptor) (ComponentManifestV2, error) {
	if err := descriptor.Validate(); err != nil {
		return ComponentManifestV2{}, err
	}
	legacyID := ComponentIDV2("legacy/" + canonicalLegacySegment(descriptor.ID))
	legacyKind := ComponentKindV2("legacy/" + canonicalLegacySegment(string(descriptor.Kind)))
	capabilities := make([]ProvidedCapabilityV2, 0, len(descriptor.Capabilities))
	for _, capability := range descriptor.Capabilities {
		if capability.State == CapabilityRevoked {
			continue
		}
		capabilities = append(capabilities, ProvidedCapabilityV2{
			Capability: CapabilityNameV2("legacy/" + canonicalLegacySegment(capability.Name)),
			TTLSeconds: 60,
			Schemas:    []SchemaRefV2{},
		})
	}
	conformance := descriptor.Conformance
	if conformance == "" || conformance == ConformanceFullyControlled {
		conformance = ConformanceRestrictedControlled
	}
	manifest := ComponentManifestV2{
		ContractVersion: BindingContractVersionV2,
		ComponentID:     legacyID, Kind: legacyKind, GovernanceCategory: "legacy/adapter",
		SemanticVersion: "1.0.0", ArtifactDigest: descriptor.ArtifactDigest,
		Contract: ContractBindingV2{Name: "praxis.runtime/legacy-component", Version: "1.0.0", Compatible: VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}},
		Schemas:  []SchemaRefV2{}, Locality: LocalityHostControlPlane,
		Dependencies: []ComponentDependencyV2{}, RequiredCapabilities: []CapabilityRequirementV2{}, ProvidedCapabilities: capabilities,
		Conformance: conformance, ResidualClass: ResidualPotentiallyStale,
		Owners:      []OwnerAssignmentV2{{Role: OwnerEffect, OwnerComponentID: legacyID}, {Role: OwnerSettlement, OwnerComponentID: legacyID}, {Role: OwnerCleanup, OwnerComponentID: legacyID}},
		Credentials: []CredentialRequirementV2{}, OfflinePolicy: OfflineDenied, Extensions: []GovernanceExtensionV2{}, Annotations: []DisplayAnnotationV2{{Key: "compatibility", Value: "v1alpha1 descriptor; registration is not certification or binding"}},
	}
	if err := manifest.Validate(); err != nil {
		return ComponentManifestV2{}, err
	}
	return manifest, nil
}

func cloneManifestV2(manifest ComponentManifestV2) ComponentManifestV2 {
	payload, _ := EncodeComponentManifestV2(manifest)
	cloned, _ := DecodeComponentManifestV2(payload)
	return cloned
}

func canonicalLegacySegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, character := range []byte(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			builder.WriteByte(character)
		} else {
			builder.WriteByte('-')
		}
	}
	result := strings.Trim(builder.String(), "-_.")
	if result == "" || result[0] < 'a' || result[0] > 'z' {
		result = "component-" + result
	}
	if len(result) > 63 {
		result = result[:63]
		result = strings.TrimRight(result, "-_.")
	}
	return result
}

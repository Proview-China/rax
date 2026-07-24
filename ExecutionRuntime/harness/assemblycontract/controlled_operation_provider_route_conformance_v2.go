package assemblycontract

import (
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ControlledOperationProviderRouteConformanceStatusV2 string

const (
	ControlledOperationProviderRouteConformantV2 ControlledOperationProviderRouteConformanceStatusV2 = "conformant"
	ControlledOperationProviderRouteRejectedV2   ControlledOperationProviderRouteConformanceStatusV2 = "rejected"
	ControlledOperationProviderRouteExpiredV2    ControlledOperationProviderRouteConformanceStatusV2 = "expired"
)

type ControlledOperationProviderRouteWiringEdgeV2 struct {
	SourceRole        ControlledOperationProviderRouteRoleV2 `json:"source_role"`
	SourceComponentID runtimeports.ComponentIDV2             `json:"source_component_id,omitempty"`
	SourcePortSpecRef string                                 `json:"source_port_spec_ref"`
	TargetRole        ControlledOperationProviderRouteRoleV2 `json:"target_role"`
	TargetComponentID runtimeports.ComponentIDV2             `json:"target_component_id,omitempty"`
	TargetPortSpecRef string                                 `json:"target_port_spec_ref"`
	ProviderRef       ObjectRefV1                            `json:"provider_ref,omitempty"`
	ModuleRef         string                                 `json:"module_ref,omitempty"`
	CandidateID       string                                 `json:"candidate_id,omitempty"`
	Binding           runtimeports.ProviderBindingRefV2      `json:"binding,omitempty"`
}

type ControlledOperationProviderActiveRouteRecordV2 struct {
	Version           string                                                        `json:"version"`
	RouteID           string                                                        `json:"route_id"`
	DeclarationRef    runtimeports.ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	Matrix            runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3   `json:"matrix"`
	Active            bool                                                          `json:"active"`
	TransportIdentity ControlledOperationProviderRouteNormalizedIdentityV2          `json:"transport_identity"`
	ProviderIdentity  ControlledOperationProviderRouteNormalizedIdentityV2          `json:"provider_identity"`
	TransportBinding  runtimeports.ProviderBindingRefV2                             `json:"transport_binding"`
	ProviderBinding   runtimeports.ProviderBindingRefV2                             `json:"provider_binding"`
}

type ControlledOperationProviderRouteWiringInventoryV2 struct {
	ContractVersion string                                           `json:"contract_version"`
	InventoryID     string                                           `json:"inventory_id"`
	Revision        core.Revision                                    `json:"revision"`
	Edges           []ControlledOperationProviderRouteWiringEdgeV2   `json:"edges"`
	ActiveRoutes    []ControlledOperationProviderActiveRouteRecordV2 `json:"active_routes"`
	CheckedUnixNano int64                                            `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                            `json:"expires_unix_nano"`
	Digest          core.Digest                                      `json:"digest"`
}

func NormalizeControlledOperationProviderRouteWiringInventoryV2(value ControlledOperationProviderRouteWiringInventoryV2) ControlledOperationProviderRouteWiringInventoryV2 {
	value.Edges = append([]ControlledOperationProviderRouteWiringEdgeV2(nil), value.Edges...)
	value.ActiveRoutes = append([]ControlledOperationProviderActiveRouteRecordV2(nil), value.ActiveRoutes...)
	sort.Slice(value.Edges, func(i, j int) bool {
		left, right := value.Edges[i], value.Edges[j]
		if left.SourceRole != right.SourceRole {
			return left.SourceRole < right.SourceRole
		}
		if left.TargetRole != right.TargetRole {
			return left.TargetRole < right.TargetRole
		}
		if left.SourceComponentID != right.SourceComponentID {
			return left.SourceComponentID < right.SourceComponentID
		}
		return left.TargetPortSpecRef < right.TargetPortSpecRef
	})
	sort.Slice(value.ActiveRoutes, func(i, j int) bool {
		if value.ActiveRoutes[i].Version != value.ActiveRoutes[j].Version {
			return value.ActiveRoutes[i].Version < value.ActiveRoutes[j].Version
		}
		return value.ActiveRoutes[i].RouteID < value.ActiveRoutes[j].RouteID
	})
	if value.Edges == nil {
		value.Edges = []ControlledOperationProviderRouteWiringEdgeV2{}
	}
	if value.ActiveRoutes == nil {
		value.ActiveRoutes = []ControlledOperationProviderActiveRouteRecordV2{}
	}
	return value
}

func (value ControlledOperationProviderRouteWiringInventoryV2) DigestV2() (core.Digest, error) {
	value.Digest = ""
	value = NormalizeControlledOperationProviderRouteWiringInventoryV2(value)
	return core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", ControlledOperationProviderRouteContractVersionV2, "ControlledOperationProviderRouteWiringInventoryV2", value)
}

func SealControlledOperationProviderRouteWiringInventoryV2(value ControlledOperationProviderRouteWiringInventoryV2) (ControlledOperationProviderRouteWiringInventoryV2, error) {
	provided := value.Digest
	value.ContractVersion = ControlledOperationProviderRouteContractVersionV2
	value = NormalizeControlledOperationProviderRouteWiringInventoryV2(value)
	value.Digest = ""
	digest, err := value.DigestV2()
	if err != nil {
		return ControlledOperationProviderRouteWiringInventoryV2{}, err
	}
	if provided != "" && provided != digest {
		return ControlledOperationProviderRouteWiringInventoryV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "controlled Provider wiring inventory supplied a wrong nonzero digest")
	}
	value.Digest = digest
	return value, value.Validate()
}

func (value ControlledOperationProviderRouteWiringInventoryV2) Validate() error {
	if value.ContractVersion != ControlledOperationProviderRouteContractVersionV2 || validateRouteIDV2(value.InventoryID) != nil || value.Revision == 0 || value.CheckedUnixNano <= 0 || value.CheckedUnixNano >= value.ExpiresUnixNano || value.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "controlled Provider wiring inventory is incomplete")
	}
	normalized := NormalizeControlledOperationProviderRouteWiringInventoryV2(value)
	for index, edge := range normalized.Edges {
		if !validControlledOperationProviderRouteChainEdgeV2(edge) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider wiring edge is incomplete")
		}
		if index > 0 && normalized.Edges[index-1] == edge {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "controlled Provider wiring edge is duplicated")
		}
	}
	for index, route := range normalized.ActiveRoutes {
		if (route.Version != "v1" && route.Version != "v2") || validateRouteIDV2(route.RouteID) != nil || route.DeclarationRef.Validate() != nil || !runtimeports.IsOperationScopeEvidenceActionMatrixKeyV3(route.Matrix) || route.TransportIdentity.Validate() != nil || route.ProviderIdentity.Validate() != nil || route.TransportBinding.Validate() != nil || route.ProviderBinding.Validate() != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider active route record is invalid")
		}
		if index > 0 && normalized.ActiveRoutes[index-1].Version == route.Version && normalized.ActiveRoutes[index-1].RouteID == route.RouteID {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "controlled Provider active route record is duplicated")
		}
	}
	digest, err := value.DigestV2()
	if err != nil || digest != value.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider wiring inventory digest drifted")
	}
	return nil
}

func validControlledOperationProviderRouteChainEdgeV2(edge ControlledOperationProviderRouteWiringEdgeV2) bool {
	allowed := map[[2]ControlledOperationProviderRouteRoleV2]struct{}{
		{ControlledOperationApplicationToolPortRoleV2, ControlledOperationToolAdapterRoleV2}:  {},
		{ControlledOperationToolAdapterRoleV2, ControlledOperationRuntimeGovernanceRoleV2}:    {},
		{ControlledOperationRuntimeGovernanceRoleV2, ControlledOperationRuntimeGatewayRoleV2}: {},
		{ControlledOperationRuntimeGatewayRoleV2, ControlledOperationProviderTransportRoleV2}: {},
		{ControlledOperationProviderTransportRoleV2, ControlledOperationProviderRoleV2}:       {},
	}
	if _, ok := allowed[[2]ControlledOperationProviderRouteRoleV2{edge.SourceRole, edge.TargetRole}]; !ok || validateRouteIDV2(edge.SourcePortSpecRef) != nil || validateRouteIDV2(edge.TargetPortSpecRef) != nil {
		return false
	}
	sourceIsPort := edge.SourceRole == ControlledOperationApplicationToolPortRoleV2 || edge.SourceRole == ControlledOperationRuntimeGovernanceRoleV2
	if sourceIsPort != (edge.SourceComponentID == "") {
		return false
	}
	if !sourceIsPort && runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(edge.SourceComponentID)) != nil {
		return false
	}
	targetIsPort := edge.TargetRole == ControlledOperationRuntimeGovernanceRoleV2
	if targetIsPort {
		return edge.TargetComponentID == "" && edge.ProviderRef == (ObjectRefV1{}) && edge.ModuleRef == "" && edge.CandidateID == "" && edge.Binding == (runtimeports.ProviderBindingRefV2{})
	}
	return runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(edge.TargetComponentID)) == nil && edge.ProviderRef.Validate() == nil && validateRouteIDV2(edge.ModuleRef) == nil && validateRouteIDV2(edge.CandidateID) == nil && edge.Binding.Validate() == nil
}

type ControlledOperationProviderRouteConformanceV2 struct {
	ContractVersion             string                                                        `json:"contract_version"`
	ConformanceID               string                                                        `json:"conformance_id"`
	Revision                    core.Revision                                                 `json:"revision"`
	DeclarationRef              runtimeports.ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	RequiredExtensionKey        runtimeports.NamespacedNameV2                                 `json:"required_extension_key"`
	RequiredExtensionSchema     runtimeports.SchemaRefV2                                      `json:"required_extension_schema"`
	RequiredExtensionDigest     core.Digest                                                   `json:"required_extension_digest"`
	AssemblyInputDigest         core.Digest                                                   `json:"assembly_input_digest"`
	ManifestDigest              core.Digest                                                   `json:"manifest_digest"`
	GraphDigest                 core.Digest                                                   `json:"graph_digest"`
	Generation                  runtimeports.GenerationArtifactRefV1                          `json:"generation"`
	HandoffID                   string                                                        `json:"handoff_id"`
	HandoffRevision             core.Revision                                                 `json:"handoff_revision"`
	HandoffDigest               core.Digest                                                   `json:"handoff_digest"`
	BindingSetID                string                                                        `json:"binding_set_id"`
	BindingSetRevision          core.Revision                                                 `json:"binding_set_revision"`
	BindingSetDigest            core.Digest                                                   `json:"binding_set_digest"`
	BindingSetSemanticDigest    core.Digest                                                   `json:"binding_set_semantic_digest"`
	BindingSetCurrentnessDigest core.Digest                                                   `json:"binding_set_currentness_digest"`
	AssemblyConformanceRef      ObjectRefV1                                                   `json:"assembly_conformance_ref"`
	ActiveRouteID               string                                                        `json:"active_route_id"`
	ActiveRouteRevision         core.Revision                                                 `json:"active_route_revision"`
	ActiveRouteDigest           core.Digest                                                   `json:"active_route_digest"`
	ToolAdapterBinding          runtimeports.ProviderBindingRefV2                             `json:"tool_adapter_binding"`
	GatewayBinding              runtimeports.ProviderBindingRefV2                             `json:"gateway_binding"`
	ProviderTransportBinding    runtimeports.ProviderBindingRefV2                             `json:"provider_transport_binding"`
	PreparedReaderBinding       runtimeports.ProviderBindingRefV2                             `json:"prepared_reader_binding"`
	BoundaryReaderBinding       runtimeports.ProviderBindingRefV2                             `json:"boundary_reader_binding"`
	ProviderInspectBinding      runtimeports.ProviderBindingRefV2                             `json:"provider_inspect_binding"`
	ProviderBinding             runtimeports.ProviderBindingRefV2                             `json:"provider_binding"`
	WiringInventoryRef          ObjectRefV1                                                   `json:"wiring_inventory_ref"`
	CheckedUnixNano             int64                                                         `json:"checked_unix_nano"`
	ExpiresUnixNano             int64                                                         `json:"expires_unix_nano"`
	Status                      ControlledOperationProviderRouteConformanceStatusV2           `json:"status"`
	ConformanceDigest           core.Digest                                                   `json:"conformance_digest"`
}

func DeriveControlledOperationProviderRouteConformanceIDV2(routeID string, generation runtimeports.GenerationArtifactRefV1, bindingSetID string) (string, error) {
	if validateRouteIDV2(routeID) != nil || generation.Validate() != nil || validateRouteIDV2(bindingSetID) != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "route conformance identity inputs are invalid")
	}
	digest, err := core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", ControlledOperationProviderRouteContractVersionV2, "ControlledOperationProviderRouteConformanceIdentityV2", struct {
		RouteID      string                               `json:"route_id"`
		Generation   runtimeports.GenerationArtifactRefV1 `json:"generation"`
		BindingSetID string                               `json:"binding_set_id"`
	}{routeID, generation, bindingSetID})
	if err != nil {
		return "", err
	}
	return "controlled-provider-route-conformance-" + string(digest)[len("sha256:"):], nil
}

func (c ControlledOperationProviderRouteConformanceV2) RefV2() runtimeports.ControlledOperationProviderRouteConformanceRefV2 {
	return runtimeports.ControlledOperationProviderRouteConformanceRefV2{ConformanceID: c.ConformanceID, Revision: c.Revision, DeclarationRef: c.DeclarationRef, ConformanceDigest: c.ConformanceDigest}
}

func (c ControlledOperationProviderRouteConformanceV2) DigestV2() (core.Digest, error) {
	c.ConformanceDigest = ""
	return core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", ControlledOperationProviderRouteContractVersionV2, "ControlledOperationProviderRouteConformanceV2", c)
}

func SealControlledOperationProviderRouteConformanceV2(c ControlledOperationProviderRouteConformanceV2) (ControlledOperationProviderRouteConformanceV2, error) {
	provided := c.ConformanceDigest
	c.ContractVersion = ControlledOperationProviderRouteContractVersionV2
	c.ConformanceDigest = ""
	digest, err := c.DigestV2()
	if err != nil {
		return ControlledOperationProviderRouteConformanceV2{}, err
	}
	if provided != "" && provided != digest {
		return ControlledOperationProviderRouteConformanceV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "controlled Provider route conformance supplied a wrong nonzero digest")
	}
	c.ConformanceDigest = digest
	return c, c.Validate(time.Unix(0, c.CheckedUnixNano))
}

func (c ControlledOperationProviderRouteConformanceV2) Validate(now time.Time) error {
	if c.ContractVersion != ControlledOperationProviderRouteContractVersionV2 || validateRouteIDV2(c.ConformanceID) != nil || c.Revision == 0 || c.DeclarationRef.Validate() != nil || c.RequiredExtensionKey != ControlledOperationProviderRouteExtensionKeyV2 || c.RequiredExtensionSchema.Validate() != nil || c.RequiredExtensionDigest.Validate() != nil || c.AssemblyInputDigest.Validate() != nil || c.ManifestDigest.Validate() != nil || c.GraphDigest.Validate() != nil || c.Generation.Validate() != nil || validateRouteIDV2(c.HandoffID) != nil || c.HandoffRevision == 0 || c.HandoffDigest.Validate() != nil || validateRouteIDV2(c.BindingSetID) != nil || c.BindingSetRevision == 0 || c.BindingSetDigest.Validate() != nil || c.BindingSetSemanticDigest.Validate() != nil || c.BindingSetCurrentnessDigest.Validate() != nil || c.AssemblyConformanceRef.Validate() != nil || validateRouteIDV2(c.ActiveRouteID) != nil || c.ActiveRouteRevision == 0 || c.ActiveRouteDigest.Validate() != nil || c.WiringInventoryRef.Validate() != nil || c.CheckedUnixNano <= 0 || c.CheckedUnixNano >= c.ExpiresUnixNano || c.Status != ControlledOperationProviderRouteConformantV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingDrift, "controlled Provider route conformance is incomplete")
	}
	if now.IsZero() || now.Before(time.Unix(0, c.CheckedUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled Provider route conformance clock regressed")
	}
	if !now.Before(time.Unix(0, c.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled Provider route conformance expired")
	}
	expectedID, err := DeriveControlledOperationProviderRouteConformanceIDV2(c.DeclarationRef.RouteID, c.Generation, c.BindingSetID)
	if err != nil || c.ConformanceID != expectedID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route conformance ID drifted")
	}
	bindings := []runtimeports.ProviderBindingRefV2{c.ToolAdapterBinding, c.GatewayBinding, c.ProviderTransportBinding, c.PreparedReaderBinding, c.BoundaryReaderBinding, c.ProviderInspectBinding, c.ProviderBinding}
	expectedCapabilities := []runtimeports.CapabilityNameV2{runtimeports.ControlledOperationToolAdapterCapabilityV2, runtimeports.ControlledOperationGatewayCapabilityV2, runtimeports.ControlledOperationProviderTransportCapabilityV2, runtimeports.ControlledOperationPreparedReaderCapabilityV2, runtimeports.ControlledOperationBoundaryReaderCapabilityV2, runtimeports.ControlledOperationProviderInspectCapabilityV2, runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3)}
	for index, binding := range bindings {
		if binding.Validate() != nil || binding.BindingSetID != c.BindingSetID || binding.BindingSetRevision != c.BindingSetRevision || binding.Capability != expectedCapabilities[index] {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route conformance binding drifted")
		}
		for previous := 0; previous < index; previous++ {
			if bindings[previous] == binding {
				return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route conformance reuses a binding across roles")
			}
		}
	}
	digest, err := c.DigestV2()
	if err != nil || digest != c.ConformanceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider route conformance digest drifted")
	}
	return c.RefV2().Validate()
}

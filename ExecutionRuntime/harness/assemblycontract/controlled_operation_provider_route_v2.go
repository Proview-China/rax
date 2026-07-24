package assemblycontract

import (
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ControlledOperationProviderRouteContractVersionV2 = "praxis.harness/controlled-operation-provider-route/v2"
	ControlledOperationProviderRouteExtensionKeyV2    = runtimeports.NamespacedNameV2("praxis.harness/controlled-operation-provider-route-v2")
	ControlledOperationProviderRouteSchemaNamespaceV2 = "praxis.harness"
	ControlledOperationProviderRouteSchemaNameV2      = "controlled-operation-provider-route"
	ControlledOperationProviderRouteSchemaVersionV2   = "2.0.0"
	ControlledOperationProviderRouteSchemaMediaTypeV2 = "application/json"

	ControlledOperationProviderActiveBindingPolicyV2 = "one_active_binding"
	ControlledOperationProviderBypassPolicyV2        = "no_raw_provider_bypass"
)

type ControlledOperationProviderRouteRoleV2 string

const (
	ControlledOperationApplicationToolPortRoleV2 ControlledOperationProviderRouteRoleV2 = "application_tool_port"
	ControlledOperationToolAdapterRoleV2         ControlledOperationProviderRouteRoleV2 = "tool_adapter"
	ControlledOperationRuntimeGovernanceRoleV2   ControlledOperationProviderRouteRoleV2 = "runtime_governance"
	ControlledOperationRuntimeGatewayRoleV2      ControlledOperationProviderRouteRoleV2 = "runtime_gateway"
	ControlledOperationProviderTransportRoleV2   ControlledOperationProviderRouteRoleV2 = "provider_transport"
	ControlledOperationProviderRoleV2            ControlledOperationProviderRouteRoleV2 = "provider"
	ControlledOperationPreparedReaderRoleV2      ControlledOperationProviderRouteRoleV2 = "prepared_current_reader"
	ControlledOperationBoundaryReaderRoleV2      ControlledOperationProviderRouteRoleV2 = "boundary_current_reader"
	ControlledOperationProviderInspectRoleV2     ControlledOperationProviderRouteRoleV2 = "provider_inspect_reader"
)

// ControlledOperationProviderRoutePortRefV2 is Harness-owned declaration
// data. It is deliberately not a Runtime route ref and carries no dispatch
// authority.
type ControlledOperationProviderRoutePortRefV2 struct {
	Role            ControlledOperationProviderRouteRoleV2 `json:"role"`
	PortID          string                                 `json:"port_id"`
	PortDigest      core.Digest                            `json:"port_digest"`
	OwnerCapability runtimeports.CapabilityNameV2          `json:"owner_capability"`
	RequestSchema   runtimeports.SchemaRefV2               `json:"request_schema"`
	ResponseSchema  runtimeports.SchemaRefV2               `json:"response_schema"`
	ContractVersion string                                 `json:"contract_version"`
}

type ControlledOperationProviderRouteEndpointV2 struct {
	Role            ControlledOperationProviderRouteRoleV2 `json:"role"`
	ComponentID     runtimeports.ComponentIDV2             `json:"component_id"`
	ManifestDigest  core.Digest                            `json:"manifest_digest"`
	ArtifactDigest  core.Digest                            `json:"artifact_digest"`
	Capability      runtimeports.CapabilityNameV2          `json:"capability"`
	ContractVersion string                                 `json:"contract_version"`
	Locality        runtimeports.LocalityV2                `json:"locality"`
	CandidateID     string                                 `json:"candidate_id"`
	CandidateDigest core.Digest                            `json:"candidate_digest"`
	ModuleRef       string                                 `json:"module_ref"`
	PortSpecRef     string                                 `json:"port_spec_ref"`
	ProviderRef     ObjectRefV1                            `json:"provider_ref"`
}

type ControlledOperationProviderRouteReaderRefV2 struct {
	Role             ControlledOperationProviderRouteRoleV2 `json:"role"`
	ComponentID      runtimeports.ComponentIDV2             `json:"component_id"`
	ManifestDigest   core.Digest                            `json:"manifest_digest"`
	ArtifactDigest   core.Digest                            `json:"artifact_digest"`
	Capability       runtimeports.CapabilityNameV2          `json:"capability"`
	PortSpecID       string                                 `json:"port_spec_id"`
	PortSpecDigest   core.Digest                            `json:"port_spec_digest"`
	RequestSchema    runtimeports.SchemaRefV2               `json:"request_schema"`
	ProjectionSchema runtimeports.SchemaRefV2               `json:"projection_schema"`
	ReadOnly         bool                                   `json:"read_only"`
	NoExecute        bool                                   `json:"no_execute"`
}

type ControlledOperationProviderRouteDeclarationV2 struct {
	ContractVersion       string                                                      `json:"contract_version"`
	RouteID               string                                                      `json:"route_id"`
	Revision              core.Revision                                               `json:"revision"`
	PublisherComponent    runtimeports.ComponentIDV2                                  `json:"publisher_component"`
	Matrix                runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3 `json:"matrix"`
	ApplicationToolPort   ControlledOperationProviderRoutePortRefV2                   `json:"application_tool_port"`
	ToolAdapter           ControlledOperationProviderRouteEndpointV2                  `json:"tool_adapter"`
	RuntimeGovernancePort ControlledOperationProviderRoutePortRefV2                   `json:"runtime_governance_port"`
	Gateway               ControlledOperationProviderRouteEndpointV2                  `json:"gateway"`
	ProviderTransport     ControlledOperationProviderRouteEndpointV2                  `json:"provider_transport"`
	Provider              ControlledOperationProviderRouteEndpointV2                  `json:"provider"`
	PreparedCurrentReader ControlledOperationProviderRouteReaderRefV2                 `json:"prepared_current_reader"`
	BoundaryCurrentReader ControlledOperationProviderRouteReaderRefV2                 `json:"boundary_current_reader"`
	ProviderInspectReader ControlledOperationProviderRouteReaderRefV2                 `json:"provider_inspect_reader"`
	ActiveBindingPolicy   string                                                      `json:"active_binding_policy"`
	BypassPolicy          string                                                      `json:"bypass_policy"`
	DeclarationDigest     core.Digest                                                 `json:"declaration_digest"`
}

func (d ControlledOperationProviderRouteDeclarationV2) RefV2() runtimeports.ControlledOperationProviderRouteDeclarationRefV2 {
	return runtimeports.ControlledOperationProviderRouteDeclarationRefV2{RouteID: d.RouteID, Revision: d.Revision, PublisherComponentID: string(d.PublisherComponent), DeclarationDigest: d.DeclarationDigest}
}

func (d ControlledOperationProviderRouteDeclarationV2) DigestV2() (core.Digest, error) {
	d.DeclarationDigest = ""
	return core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", ControlledOperationProviderRouteContractVersionV2, "ControlledOperationProviderRouteDeclarationV2", d)
}

func SealControlledOperationProviderRouteDeclarationV2(d ControlledOperationProviderRouteDeclarationV2) (ControlledOperationProviderRouteDeclarationV2, error) {
	provided := d.DeclarationDigest
	d.ContractVersion = ControlledOperationProviderRouteContractVersionV2
	d.DeclarationDigest = ""
	digest, err := d.DigestV2()
	if err != nil {
		return ControlledOperationProviderRouteDeclarationV2{}, err
	}
	if provided != "" && provided != digest {
		return ControlledOperationProviderRouteDeclarationV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "controlled Provider route declaration supplied a wrong nonzero digest")
	}
	d.DeclarationDigest = digest
	return d, d.Validate()
}

func DecodeControlledOperationProviderRouteDeclarationV2(payload []byte) (ControlledOperationProviderRouteDeclarationV2, error) {
	var declaration ControlledOperationProviderRouteDeclarationV2
	if err := core.DecodeStrictJSON(payload, &declaration); err != nil {
		return ControlledOperationProviderRouteDeclarationV2{}, err
	}
	return declaration, declaration.Validate()
}

func (d ControlledOperationProviderRouteDeclarationV2) Validate() error {
	if d.ContractVersion != ControlledOperationProviderRouteContractVersionV2 || validateRouteIDV2(d.RouteID) != nil || d.Revision == 0 || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(d.PublisherComponent)) != nil || !runtimeports.IsOperationScopeEvidenceActionMatrixKeyV3(d.Matrix) || d.ActiveBindingPolicy != ControlledOperationProviderActiveBindingPolicyV2 || d.BypassPolicy != ControlledOperationProviderBypassPolicyV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "controlled Provider route declaration header is invalid")
	}
	if err := validateRoutePortV2(d.ApplicationToolPort, ControlledOperationApplicationToolPortRoleV2); err != nil {
		return err
	}
	if err := validateRoutePortV2(d.RuntimeGovernancePort, ControlledOperationRuntimeGovernanceRoleV2); err != nil {
		return err
	}
	endpoints := []struct {
		value      ControlledOperationProviderRouteEndpointV2
		role       ControlledOperationProviderRouteRoleV2
		capability runtimeports.CapabilityNameV2
	}{
		{d.ToolAdapter, ControlledOperationToolAdapterRoleV2, runtimeports.ControlledOperationToolAdapterCapabilityV2},
		{d.Gateway, ControlledOperationRuntimeGatewayRoleV2, runtimeports.ControlledOperationGatewayCapabilityV2},
		{d.ProviderTransport, ControlledOperationProviderTransportRoleV2, runtimeports.ControlledOperationProviderTransportCapabilityV2},
		{d.Provider, ControlledOperationProviderRoleV2, runtimeports.CapabilityNameV2(d.Matrix.EffectKind)},
	}
	for _, endpoint := range endpoints {
		if err := validateRouteEndpointV2(endpoint.value, endpoint.role, endpoint.capability); err != nil {
			return err
		}
	}
	readers := []struct {
		value      ControlledOperationProviderRouteReaderRefV2
		role       ControlledOperationProviderRouteRoleV2
		capability runtimeports.CapabilityNameV2
	}{
		{d.PreparedCurrentReader, ControlledOperationPreparedReaderRoleV2, runtimeports.ControlledOperationPreparedReaderCapabilityV2},
		{d.BoundaryCurrentReader, ControlledOperationBoundaryReaderRoleV2, runtimeports.ControlledOperationBoundaryReaderCapabilityV2},
		{d.ProviderInspectReader, ControlledOperationProviderInspectRoleV2, runtimeports.ControlledOperationProviderInspectCapabilityV2},
	}
	identities := map[string]struct{}{}
	candidateIDs := map[string]struct{}{}
	portIDs := map[string]struct{}{d.ApplicationToolPort.PortID: {}, d.RuntimeGovernancePort.PortID: {}}
	if d.ApplicationToolPort.PortID == d.RuntimeGovernancePort.PortID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "controlled Provider route ports must be nominally distinct")
	}
	for _, endpoint := range endpoints {
		key := string(endpoint.value.Role) + "\x00" + endpoint.value.CandidateID
		if _, duplicate := identities[key]; duplicate {
			return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "controlled Provider route endpoint is duplicated")
		}
		identities[key] = struct{}{}
		if _, duplicate := candidateIDs[endpoint.value.CandidateID]; duplicate {
			return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "controlled Provider route candidate is reused across roles")
		}
		candidateIDs[endpoint.value.CandidateID] = struct{}{}
		if _, duplicate := portIDs[endpoint.value.PortSpecRef]; duplicate {
			return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "controlled Provider route PortSpec is reused across roles")
		}
		portIDs[endpoint.value.PortSpecRef] = struct{}{}
	}
	for _, reader := range readers {
		if err := validateRouteReaderV2(reader.value, reader.role, reader.capability); err != nil {
			return err
		}
		key := string(reader.value.Role) + "\x00" + reader.value.PortSpecID
		if _, duplicate := identities[key]; duplicate {
			return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "controlled Provider route reader is duplicated")
		}
		identities[key] = struct{}{}
		if _, duplicate := portIDs[reader.value.PortSpecID]; duplicate {
			return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "controlled Provider route Reader PortSpec is reused across roles")
		}
		portIDs[reader.value.PortSpecID] = struct{}{}
	}
	if d.ProviderTransport.CandidateID == d.Provider.CandidateID || d.ProviderTransport.ProviderRef == d.Provider.ProviderRef {
		return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "Provider transport and actual Provider must be distinct nominal endpoints")
	}
	digest, err := d.DigestV2()
	if err != nil || digest != d.DeclarationDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "controlled Provider route declaration digest drifted")
	}
	return d.RefV2().Validate()
}

func validateRoutePortV2(p ControlledOperationProviderRoutePortRefV2, role ControlledOperationProviderRouteRoleV2) error {
	if p.Role != role || validateRouteIDV2(p.PortID) != nil || p.PortDigest.Validate() != nil || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(p.OwnerCapability)) != nil || p.RequestSchema.Validate() != nil || p.ResponseSchema.Validate() != nil || strings.TrimSpace(p.ContractVersion) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider route Port ref is incomplete or type-punned")
	}
	return nil
}

func validateRouteEndpointV2(e ControlledOperationProviderRouteEndpointV2, role ControlledOperationProviderRouteRoleV2, capability runtimeports.CapabilityNameV2) error {
	if e.Role != role || e.Capability != capability || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(e.ComponentID)) != nil || e.ManifestDigest.Validate() != nil || e.ArtifactDigest.Validate() != nil || strings.TrimSpace(e.ContractVersion) == "" || !validRouteLocalityV2(e.Locality) || validateRouteIDV2(e.CandidateID) != nil || e.CandidateDigest.Validate() != nil || validateRouteIDV2(e.ModuleRef) != nil || validateRouteIDV2(e.PortSpecRef) != nil || e.ProviderRef.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider route endpoint is incomplete or type-punned")
	}
	return nil
}

func validateRouteReaderV2(r ControlledOperationProviderRouteReaderRefV2, role ControlledOperationProviderRouteRoleV2, capability runtimeports.CapabilityNameV2) error {
	if r.Role != role || r.Capability != capability || !r.ReadOnly || !r.NoExecute || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(r.ComponentID)) != nil || r.ManifestDigest.Validate() != nil || r.ArtifactDigest.Validate() != nil || validateRouteIDV2(r.PortSpecID) != nil || r.PortSpecDigest.Validate() != nil || r.RequestSchema.Validate() != nil || r.ProjectionSchema.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider route Reader ref is incomplete, executable or type-punned")
	}
	return nil
}

func validRouteLocalityV2(value runtimeports.LocalityV2) bool {
	switch value {
	case runtimeports.LocalityHostControlPlane, runtimeports.LocalityInstanceDataPlane, runtimeports.LocalityExternalStatePlane, runtimeports.LocalityRemoteProvider:
		return true
	default:
		return false
	}
}

func validateRouteIDV2(value string) error {
	if strings.TrimSpace(value) != value || value == "" || len(value) > 256 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "route identifier must be non-empty, trimmed and bounded")
	}
	for _, character := range []byte(value) {
		if character < 0x21 || character > 0x7e {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "route identifier must use printable ASCII")
		}
	}
	return nil
}

// PortSpecDigestForControlledOperationProviderRouteV2 binds the complete
// existing PortSpec without changing its V1 canonical contract.
func PortSpecDigestForControlledOperationProviderRouteV2(port PortSpecV1) (core.Digest, error) {
	port.RunStartRequirementRefs = append([]RunStartRequirementRefV1(nil), port.RunStartRequirementRefs...)
	port.RunSettlementRequirementRefs = append([]RunSettlementRequirementRefV1(nil), port.RunSettlementRequirementRefs...)
	sort.Slice(port.RunStartRequirementRefs, func(i, j int) bool {
		return port.RunStartRequirementRefs[i].Ref.ID < port.RunStartRequirementRefs[j].Ref.ID
	})
	sort.Slice(port.RunSettlementRequirementRefs, func(i, j int) bool {
		return port.RunSettlementRequirementRefs[i].Ref.ID < port.RunSettlementRequirementRefs[j].Ref.ID
	})
	if port.RunStartRequirementRefs == nil {
		port.RunStartRequirementRefs = []RunStartRequirementRefV1{}
	}
	if port.RunSettlementRequirementRefs == nil {
		port.RunSettlementRequirementRefs = []RunSettlementRequirementRefV1{}
	}
	return core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", ControlledOperationProviderRouteContractVersionV2, "PortSpecV1", port)
}

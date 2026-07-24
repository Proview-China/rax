package assemblycontract

import (
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ControlledOperationProviderRouteConflictCodeV2 string
type ControlledOperationProviderRouteConflictPhaseV2 string
type ControlledOperationProviderRouteAliasSurfaceKindV2 string

const (
	ControlledOperationProviderRouteDeclarationConflictV2   ControlledOperationProviderRouteConflictCodeV2 = "declaration_merge_conflict"
	ControlledOperationProviderRouteActiveVersionConflictV2 ControlledOperationProviderRouteConflictCodeV2 = "active_route_version_conflict"
	ControlledOperationProviderRouteAliasConflictV2         ControlledOperationProviderRouteConflictCodeV2 = "provider_alias_conflict"
)

const (
	ControlledOperationProviderRouteDeclarationPhaseV2 ControlledOperationProviderRouteConflictPhaseV2 = "declaration_merge"
	ControlledOperationProviderRoutePrebindingPhaseV2  ControlledOperationProviderRouteConflictPhaseV2 = "prebinding"
	ControlledOperationProviderRoutePostbindingPhaseV2 ControlledOperationProviderRouteConflictPhaseV2 = "postbinding"
)

const (
	ControlledOperationProviderRouteAliasCandidateSurfaceV2  ControlledOperationProviderRouteAliasSurfaceKindV2 = "candidate"
	ControlledOperationProviderRouteAliasPortSurfaceV2       ControlledOperationProviderRouteAliasSurfaceKindV2 = "port"
	ControlledOperationProviderRouteAliasSlotSurfaceV2       ControlledOperationProviderRouteAliasSurfaceKindV2 = "slot"
	ControlledOperationProviderRouteAliasFactorySurfaceV2    ControlledOperationProviderRouteAliasSurfaceKindV2 = "factory"
	ControlledOperationProviderRouteAliasDependencySurfaceV2 ControlledOperationProviderRouteAliasSurfaceKindV2 = "dependency"
	ControlledOperationProviderRouteAliasPhaseSurfaceV2      ControlledOperationProviderRouteAliasSurfaceKindV2 = "phase"
)

type ControlledOperationProviderRouteAliasSurfaceV2 struct {
	Kind            ControlledOperationProviderRouteAliasSurfaceKindV2 `json:"kind"`
	Ref             string                                             `json:"ref"`
	ModuleRef       string                                             `json:"module_ref,omitempty"`
	PortSpecRef     string                                             `json:"port_spec_ref,omitempty"`
	Capability      string                                             `json:"capability,omitempty"`
	CanonicalDigest core.Digest                                        `json:"canonical_digest"`
}

func (s ControlledOperationProviderRouteAliasSurfaceV2) DigestV2() (core.Digest, error) {
	s.CanonicalDigest = ""
	return core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", ControlledOperationProviderRouteContractVersionV2, "ControlledOperationProviderRouteAliasSurfaceV2", s)
}

func SealControlledOperationProviderRouteAliasSurfaceV2(s ControlledOperationProviderRouteAliasSurfaceV2) (ControlledOperationProviderRouteAliasSurfaceV2, error) {
	provided := s.CanonicalDigest
	s.CanonicalDigest = ""
	digest, err := s.DigestV2()
	if err != nil {
		return ControlledOperationProviderRouteAliasSurfaceV2{}, err
	}
	if provided != "" && provided != digest {
		return ControlledOperationProviderRouteAliasSurfaceV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "controlled Provider alias surface supplied a wrong digest")
	}
	s.CanonicalDigest = digest
	return s, s.Validate()
}

func (s ControlledOperationProviderRouteAliasSurfaceV2) Validate() error {
	if !validControlledOperationProviderRouteAliasSurfaceKindV2(s.Kind) || validateRouteIDV2(s.Ref) != nil || s.CanonicalDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider alias surface is incomplete")
	}
	for _, optional := range []string{s.ModuleRef, s.PortSpecRef, s.Capability} {
		if optional != "" && validateRouteIDV2(optional) != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider alias surface coordinate is invalid")
		}
	}
	validCoordinates := false
	switch s.Kind {
	case ControlledOperationProviderRouteAliasCandidateSurfaceV2:
		validCoordinates = s.ModuleRef != "" && s.PortSpecRef != "" && s.Capability == ""
	case ControlledOperationProviderRouteAliasPortSurfaceV2:
		validCoordinates = s.ModuleRef == "" && s.PortSpecRef == s.Ref && s.Capability != ""
	case ControlledOperationProviderRouteAliasSlotSurfaceV2:
		validCoordinates = s.ModuleRef != "" && s.PortSpecRef != "" && s.Capability != ""
	case ControlledOperationProviderRouteAliasFactorySurfaceV2:
		validCoordinates = s.ModuleRef != "" && s.PortSpecRef == "" && s.Capability != ""
	case ControlledOperationProviderRouteAliasDependencySurfaceV2:
		validCoordinates = s.ModuleRef == "" && s.PortSpecRef != "" && s.Capability != ""
	case ControlledOperationProviderRouteAliasPhaseSurfaceV2:
		validCoordinates = s.ModuleRef != "" && s.PortSpecRef != "" && s.Capability != ""
	}
	if !validCoordinates {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider alias surface coordinates do not match its closed kind")
	}
	digest, err := s.DigestV2()
	if err != nil || digest != s.CanonicalDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider alias surface digest drifted")
	}
	return nil
}

type ControlledOperationProviderRouteNormalizedIdentityV2 struct {
	ProviderRef             ObjectRefV1                   `json:"provider_ref"`
	CandidateID             string                        `json:"candidate_id"`
	ModuleRef               string                        `json:"module_ref"`
	ComponentID             runtimeports.ComponentIDV2    `json:"component_id"`
	ComponentManifestDigest core.Digest                   `json:"component_manifest_digest"`
	ArtifactDigest          core.Digest                   `json:"artifact_digest"`
	Capability              runtimeports.CapabilityNameV2 `json:"capability"`
	PortSpecRef             string                        `json:"port_spec_ref"`
	PortSpecDigest          core.Digest                   `json:"port_spec_digest"`
	ConflictDomain          string                        `json:"conflict_domain"`
}

func (i ControlledOperationProviderRouteNormalizedIdentityV2) Validate() error {
	if i.ProviderRef.Validate() != nil || validateRouteIDV2(i.CandidateID) != nil || validateRouteIDV2(i.ModuleRef) != nil || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(i.ComponentID)) != nil || i.ComponentManifestDigest.Validate() != nil || i.ArtifactDigest.Validate() != nil || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(i.Capability)) != nil || validateRouteIDV2(i.PortSpecRef) != nil || i.PortSpecDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider normalized identity is incomplete")
	}
	return nil
}

type ControlledOperationProviderRouteConflictSideV2 struct {
	Version           string                                                        `json:"version"`
	DeclarationRef    runtimeports.ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	ProviderTransport ControlledOperationProviderRouteNormalizedIdentityV2          `json:"provider_transport"`
	Provider          ControlledOperationProviderRouteNormalizedIdentityV2          `json:"provider"`
	TransportBinding  *runtimeports.ProviderBindingRefV2                            `json:"transport_binding,omitempty"`
	ProviderBinding   *runtimeports.ProviderBindingRefV2                            `json:"provider_binding,omitempty"`
}

type ControlledOperationProviderRouteConflictV2 struct {
	ContractVersion       string                                                      `json:"contract_version"`
	ConflictCode          ControlledOperationProviderRouteConflictCodeV2              `json:"conflict_code"`
	Phase                 ControlledOperationProviderRouteConflictPhaseV2             `json:"phase"`
	Matrix                runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3 `json:"matrix"`
	Left                  ControlledOperationProviderRouteConflictSideV2              `json:"left"`
	Right                 ControlledOperationProviderRouteConflictSideV2              `json:"right"`
	AliasSurface          *ControlledOperationProviderRouteAliasSurfaceV2             `json:"alias_surface,omitempty"`
	AssemblyInputDigest   core.Digest                                                 `json:"assembly_input_digest,omitempty"`
	GraphDigest           core.Digest                                                 `json:"graph_digest,omitempty"`
	WiringInventoryDigest core.Digest                                                 `json:"wiring_inventory_digest,omitempty"`
	ConflictDigest        core.Digest                                                 `json:"conflict_digest"`
}

func (c ControlledOperationProviderRouteConflictV2) DigestV2() (core.Digest, error) {
	c.ConflictDigest = ""
	return core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", ControlledOperationProviderRouteContractVersionV2, "ControlledOperationProviderRouteConflictV2", c)
}

func SealControlledOperationProviderRouteConflictV2(c ControlledOperationProviderRouteConflictV2) (ControlledOperationProviderRouteConflictV2, error) {
	provided := c.ConflictDigest
	c.ContractVersion = ControlledOperationProviderRouteContractVersionV2
	c.ConflictDigest = ""
	digest, err := c.DigestV2()
	if err != nil {
		return ControlledOperationProviderRouteConflictV2{}, err
	}
	if provided != "" && provided != digest {
		return ControlledOperationProviderRouteConflictV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "controlled Provider route conflict supplied a wrong nonzero digest")
	}
	c.ConflictDigest = digest
	return c, c.Validate()
}

func (c ControlledOperationProviderRouteConflictV2) Validate() error {
	if c.ContractVersion != ControlledOperationProviderRouteContractVersionV2 || !validControlledOperationProviderRouteConflictCodeV2(c.ConflictCode) || !validControlledOperationProviderRouteConflictPhaseV2(c.Phase) || !runtimeports.IsOperationScopeEvidenceActionMatrixKeyV3(c.Matrix) || c.Left.DeclarationRef.Validate() != nil || c.Right.DeclarationRef.Validate() != nil || c.ConflictDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingSetConflict, "controlled Provider route conflict is incomplete")
	}
	if c.ConflictCode == ControlledOperationProviderRouteActiveVersionConflictV2 {
		if c.Left.ProviderTransport.Validate() != nil || c.Left.Provider.Validate() != nil || c.Right.ProviderTransport.Validate() != nil || c.Right.Provider.Validate() != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingSetConflict, "controlled Provider route conflict identities are incomplete")
		}
	}
	if c.ConflictCode == ControlledOperationProviderRouteAliasConflictV2 {
		if c.Left.ProviderTransport.Validate() != nil || c.Left.Provider.Validate() != nil || c.AliasSurface == nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingSetConflict, "controlled Provider alias conflict protected identities are incomplete")
		}
		rightEmpty := c.Right.ProviderTransport == (ControlledOperationProviderRouteNormalizedIdentityV2{}) && c.Right.Provider == (ControlledOperationProviderRouteNormalizedIdentityV2{})
		rightValid := c.Right.ProviderTransport.Validate() == nil && c.Right.Provider.Validate() == nil
		if c.AliasSurface.Kind == ControlledOperationProviderRouteAliasCandidateSurfaceV2 {
			if !rightValid && !rightEmpty {
				return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingSetConflict, "candidate alias conflict normalized identities are incomplete")
			}
		} else if !rightEmpty {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingSetConflict, "non-candidate alias conflict must use AliasSurface instead of identity fallback")
		}
	}
	if c.ConflictCode == ControlledOperationProviderRouteActiveVersionConflictV2 {
		leftPrebinding := c.Left.Version == "v2-prebinding" && c.Left.TransportBinding == nil && c.Left.ProviderBinding == nil
		leftBound := c.Left.TransportBinding != nil && c.Left.ProviderBinding != nil && c.Left.TransportBinding.Validate() == nil && c.Left.ProviderBinding.Validate() == nil
		rightBound := c.Right.TransportBinding != nil && c.Right.ProviderBinding != nil && c.Right.TransportBinding.Validate() == nil && c.Right.ProviderBinding.Validate() == nil
		if (!leftPrebinding && !leftBound) || !rightBound {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingSetConflict, "controlled Provider active-route conflict bindings are incomplete")
		}
	}
	if err := c.validateProvenanceV2(); err != nil {
		return err
	}
	digest, err := c.DigestV2()
	if err != nil || digest != c.ConflictDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider route conflict digest drifted")
	}
	return nil
}

func (c ControlledOperationProviderRouteConflictV2) validateProvenanceV2() error {
	switch c.ConflictCode {
	case ControlledOperationProviderRouteDeclarationConflictV2:
		if c.Phase != ControlledOperationProviderRouteDeclarationPhaseV2 || c.AliasSurface != nil || c.AssemblyInputDigest != "" || c.GraphDigest != "" || c.WiringInventoryDigest != "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "declaration conflict carries invalid provenance")
		}
	case ControlledOperationProviderRouteAliasConflictV2:
		if c.Phase != ControlledOperationProviderRoutePrebindingPhaseV2 || c.AssemblyInputDigest.Validate() != nil || c.GraphDigest != "" || c.WiringInventoryDigest != "" || c.AliasSurface == nil || c.AliasSurface.Validate() != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prebinding alias conflict provenance is incomplete")
		}
	case ControlledOperationProviderRouteActiveVersionConflictV2:
		if c.AliasSurface != nil || c.AssemblyInputDigest.Validate() != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "active-route conflict provenance is incomplete")
		}
		if c.Phase == ControlledOperationProviderRoutePrebindingPhaseV2 {
			if c.GraphDigest != "" || c.WiringInventoryDigest != "" {
				return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prebinding active-route conflict must only bind AssemblyInput")
			}
		} else if c.Phase == ControlledOperationProviderRoutePostbindingPhaseV2 {
			if c.GraphDigest.Validate() != nil || c.WiringInventoryDigest.Validate() != nil {
				return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "postbinding active-route conflict provenance is incomplete")
			}
		} else {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "active-route conflict phase is invalid")
		}
	}
	return nil
}

func validControlledOperationProviderRouteConflictCodeV2(value ControlledOperationProviderRouteConflictCodeV2) bool {
	switch value {
	case ControlledOperationProviderRouteDeclarationConflictV2, ControlledOperationProviderRouteActiveVersionConflictV2, ControlledOperationProviderRouteAliasConflictV2:
		return true
	default:
		return false
	}
}

func validControlledOperationProviderRouteConflictPhaseV2(value ControlledOperationProviderRouteConflictPhaseV2) bool {
	switch value {
	case ControlledOperationProviderRouteDeclarationPhaseV2, ControlledOperationProviderRoutePrebindingPhaseV2, ControlledOperationProviderRoutePostbindingPhaseV2:
		return true
	default:
		return false
	}
}

func validControlledOperationProviderRouteAliasSurfaceKindV2(value ControlledOperationProviderRouteAliasSurfaceKindV2) bool {
	switch value {
	case ControlledOperationProviderRouteAliasCandidateSurfaceV2, ControlledOperationProviderRouteAliasPortSurfaceV2, ControlledOperationProviderRouteAliasSlotSurfaceV2, ControlledOperationProviderRouteAliasFactorySurfaceV2, ControlledOperationProviderRouteAliasDependencySurfaceV2, ControlledOperationProviderRouteAliasPhaseSurfaceV2:
		return true
	default:
		return false
	}
}

type ControlledOperationProviderRouteConflictErrorV2 struct {
	Conflict ControlledOperationProviderRouteConflictV2
}

func (e *ControlledOperationProviderRouteConflictErrorV2) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s: %s: %s", core.ErrorConflict, core.ReasonBindingSetConflict, e.Conflict.ConflictCode)
}

func (e *ControlledOperationProviderRouteConflictErrorV2) Unwrap() error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, string(e.Conflict.ConflictCode))
}

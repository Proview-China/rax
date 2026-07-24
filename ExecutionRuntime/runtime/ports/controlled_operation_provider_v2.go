package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ControlledOperationProviderContractVersionV2             = "2.0.0"
	ControlledOperationProviderRouteCurrentContractVersionV2 = "2.0.0"

	ControlledOperationToolAdapterCapabilityV2       CapabilityNameV2 = "praxis.tool/controlled-provider-adapter-v2"
	ControlledOperationGatewayCapabilityV2           CapabilityNameV2 = "praxis.runtime/controlled-provider-gateway-v2"
	ControlledOperationProviderTransportCapabilityV2 CapabilityNameV2 = "praxis.tool/controlled-provider-transport-v2"
	ControlledOperationPreparedReaderCapabilityV2    CapabilityNameV2 = "praxis.runtime/prepared-current-reader-v2"
	ControlledOperationBoundaryReaderCapabilityV2    CapabilityNameV2 = "praxis.runtime/provider-boundary-reader-v1"
	ControlledOperationProviderInspectCapabilityV2   CapabilityNameV2 = "praxis.runtime/provider-inspect-v2"
)

type ControlledOperationProviderRouteDeclarationRefV2 struct {
	RouteID              string        `json:"route_id"`
	Revision             core.Revision `json:"revision"`
	PublisherComponentID string        `json:"publisher_component_id"`
	DeclarationDigest    core.Digest   `json:"declaration_digest"`
}

func (r ControlledOperationProviderRouteDeclarationRefV2) Validate() error {
	if validateEvidenceIDV2(r.RouteID) != nil || r.Revision == 0 || ValidateNamespacedNameV2(NamespacedNameV2(r.PublisherComponentID)) != nil || r.DeclarationDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider route declaration ref is incomplete")
	}
	return nil
}

type ControlledOperationProviderRouteConformanceRefV2 struct {
	ConformanceID     string                                           `json:"conformance_id"`
	Revision          core.Revision                                    `json:"revision"`
	DeclarationRef    ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	ConformanceDigest core.Digest                                      `json:"conformance_digest"`
}

func (r ControlledOperationProviderRouteConformanceRefV2) Validate() error {
	if validateEvidenceIDV2(r.ConformanceID) != nil || r.Revision == 0 || r.DeclarationRef.Validate() != nil || r.ConformanceDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider route conformance ref is incomplete")
	}
	return nil
}

type ControlledOperationProviderRouteCurrentRefV2 struct {
	CurrentID      string                                           `json:"current_id"`
	Revision       core.Revision                                    `json:"revision"`
	DeclarationRef ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	ConformanceRef ControlledOperationProviderRouteConformanceRefV2 `json:"conformance_ref"`
	MatrixDigest   core.Digest                                      `json:"matrix_digest"`
	Watermark      core.Digest                                      `json:"watermark"`
	Digest         core.Digest                                      `json:"digest"`
}

func (r ControlledOperationProviderRouteCurrentRefV2) Validate() error {
	if validateEvidenceIDV2(r.CurrentID) != nil || r.Revision == 0 || r.DeclarationRef.Validate() != nil || r.ConformanceRef.Validate() != nil || r.MatrixDigest.Validate() != nil || r.Watermark.Validate() != nil || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider route current ref is incomplete")
	}
	if r.ConformanceRef.DeclarationRef != r.DeclarationRef {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route refs drifted")
	}
	digest, err := r.DigestV2()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route current ref digest drifted")
	}
	return nil
}

func (r ControlledOperationProviderRouteCurrentRefV2) DigestV2() (core.Digest, error) {
	r.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-operation-provider-route-current", ControlledOperationProviderRouteCurrentContractVersionV2, "ControlledOperationProviderRouteCurrentRefV2", r)
}

func SealControlledOperationProviderRouteCurrentRefV2(r ControlledOperationProviderRouteCurrentRefV2) (ControlledOperationProviderRouteCurrentRefV2, error) {
	provided := r.Digest
	r.Digest = ""
	digest, err := r.DigestV2()
	if err != nil {
		return ControlledOperationProviderRouteCurrentRefV2{}, err
	}
	if provided != "" && provided != digest {
		return ControlledOperationProviderRouteCurrentRefV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route current ref supplied a wrong nonzero digest")
	}
	r.Digest = digest
	return r, r.Validate()
}

type ControlledOperationProviderRouteCurrentProjectionV2 struct {
	ContractVersion             string                                           `json:"contract_version"`
	Ref                         ControlledOperationProviderRouteCurrentRefV2     `json:"ref"`
	DeclarationRef              ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	ConformanceRef              ControlledOperationProviderRouteConformanceRefV2 `json:"conformance_ref"`
	Generation                  GenerationArtifactRefV1                          `json:"generation"`
	HandoffID                   string                                           `json:"handoff_id"`
	HandoffRevision             core.Revision                                    `json:"handoff_revision"`
	HandoffDigest               core.Digest                                      `json:"handoff_digest"`
	BindingSetID                string                                           `json:"binding_set_id"`
	BindingSetRevision          core.Revision                                    `json:"binding_set_revision"`
	BindingSetDigest            core.Digest                                      `json:"binding_set_digest"`
	BindingSetSemanticDigest    core.Digest                                      `json:"binding_set_semantic_digest"`
	BindingSetCurrentnessDigest core.Digest                                      `json:"binding_set_currentness_digest"`
	ActiveRouteID               string                                           `json:"active_route_id"`
	ActiveRouteRevision         core.Revision                                    `json:"active_route_revision"`
	ActiveRouteDigest           core.Digest                                      `json:"active_route_digest"`
	ToolAdapterBinding          ProviderBindingRefV2                             `json:"tool_adapter_binding"`
	GatewayBinding              ProviderBindingRefV2                             `json:"gateway_binding"`
	ProviderTransportBinding    ProviderBindingRefV2                             `json:"provider_transport_binding"`
	PreparedReaderBinding       ProviderBindingRefV2                             `json:"prepared_reader_binding"`
	BoundaryReaderBinding       ProviderBindingRefV2                             `json:"boundary_reader_binding"`
	ProviderInspectBinding      ProviderBindingRefV2                             `json:"provider_inspect_binding"`
	ProviderBinding             ProviderBindingRefV2                             `json:"provider_binding"`
	CheckedUnixNano             int64                                            `json:"checked_unix_nano"`
	ExpiresUnixNano             int64                                            `json:"expires_unix_nano"`
	ProjectionDigest            core.Digest                                      `json:"projection_digest"`
}

func (p ControlledOperationProviderRouteCurrentProjectionV2) Validate() error {
	if p.ContractVersion != ControlledOperationProviderRouteCurrentContractVersionV2 || p.Ref.Validate() != nil || p.DeclarationRef.Validate() != nil || p.ConformanceRef.Validate() != nil || p.Generation.Validate() != nil || validateEvidenceIDV2(p.HandoffID) != nil || p.HandoffRevision == 0 || p.HandoffDigest.Validate() != nil || validateEvidenceIDV2(p.BindingSetID) != nil || p.BindingSetRevision == 0 || p.BindingSetDigest.Validate() != nil || p.BindingSetSemanticDigest.Validate() != nil || p.BindingSetCurrentnessDigest.Validate() != nil || validateEvidenceIDV2(p.ActiveRouteID) != nil || p.ActiveRouteRevision == 0 || p.ActiveRouteDigest.Validate() != nil || p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingDrift, "controlled Provider route projection is incomplete")
	}
	if p.Ref.DeclarationRef != p.DeclarationRef || p.Ref.ConformanceRef != p.ConformanceRef || p.ConformanceRef.DeclarationRef != p.DeclarationRef {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route projection refs drifted")
	}
	matrix := OperationScopeEvidenceActionMatrixV3()
	matrixDigest, err := DigestOperationScopeEvidenceApplicabilityMatrixKeyV3(matrix)
	if err != nil || p.Ref.MatrixDigest != matrixDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route matrix digest drifted")
	}
	expectedCurrentID, err := DeriveControlledOperationProviderRouteCurrentIDV2(p.DeclarationRef.RouteID, matrixDigest)
	if err != nil || p.Ref.CurrentID != expectedCurrentID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route current ID is not canonical")
	}
	expectedCapabilities := []CapabilityNameV2{
		ControlledOperationToolAdapterCapabilityV2,
		ControlledOperationGatewayCapabilityV2,
		ControlledOperationProviderTransportCapabilityV2,
		ControlledOperationPreparedReaderCapabilityV2,
		ControlledOperationBoundaryReaderCapabilityV2,
		ControlledOperationProviderInspectCapabilityV2,
		CapabilityNameV2(OperationScopeEvidenceActionEffectKindV3),
	}
	bindings := []ProviderBindingRefV2{p.ToolAdapterBinding, p.GatewayBinding, p.ProviderTransportBinding, p.PreparedReaderBinding, p.BoundaryReaderBinding, p.ProviderInspectBinding, p.ProviderBinding}
	for index, binding := range bindings {
		if err := binding.Validate(); err != nil {
			return err
		}
		if binding.Capability != expectedCapabilities[index] || binding.BindingSetID != p.BindingSetID || binding.BindingSetRevision != p.BindingSetRevision {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route binding role drifted")
		}
		for previous := 0; previous < index; previous++ {
			if binding == bindings[previous] {
				return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route reuses one binding for multiple roles")
			}
		}
	}
	watermark, err := DigestControlledOperationProviderRouteWatermarkV2(p)
	if err != nil || watermark != p.Ref.Watermark {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route watermark drifted")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route projection digest drifted")
	}
	return nil
}

func (p ControlledOperationProviderRouteCurrentProjectionV2) DigestV2() (core.Digest, error) {
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-operation-provider-route-current", ControlledOperationProviderRouteCurrentContractVersionV2, "ControlledOperationProviderRouteCurrentProjectionV2", p)
}

func SealControlledOperationProviderRouteCurrentProjectionV2(p ControlledOperationProviderRouteCurrentProjectionV2) (ControlledOperationProviderRouteCurrentProjectionV2, error) {
	if p.ContractVersion != "" && p.ContractVersion != ControlledOperationProviderRouteCurrentContractVersionV2 {
		return ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider route current contract version is invalid")
	}
	p.ContractVersion = ControlledOperationProviderRouteCurrentContractVersionV2
	matrixDigest, err := DigestOperationScopeEvidenceApplicabilityMatrixKeyV3(OperationScopeEvidenceActionMatrixV3())
	if err != nil {
		return ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	if p.Ref.MatrixDigest != "" && p.Ref.MatrixDigest != matrixDigest {
		return ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route supplied a wrong nonzero matrix digest")
	}
	p.Ref.MatrixDigest = matrixDigest
	expectedCurrentID, err := DeriveControlledOperationProviderRouteCurrentIDV2(p.DeclarationRef.RouteID, matrixDigest)
	if err != nil {
		return ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	if p.Ref.CurrentID != "" && p.Ref.CurrentID != expectedCurrentID {
		return ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route supplied a wrong nonzero current ID")
	}
	p.Ref.CurrentID = expectedCurrentID
	if p.Ref.DeclarationRef != (ControlledOperationProviderRouteDeclarationRefV2{}) && p.Ref.DeclarationRef != p.DeclarationRef {
		return ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route supplied a wrong nonzero declaration ref")
	}
	if p.Ref.ConformanceRef != (ControlledOperationProviderRouteConformanceRefV2{}) && p.Ref.ConformanceRef != p.ConformanceRef {
		return ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route supplied a wrong nonzero conformance ref")
	}
	p.Ref.DeclarationRef = p.DeclarationRef
	p.Ref.ConformanceRef = p.ConformanceRef
	expectedWatermark, err := DigestControlledOperationProviderRouteWatermarkV2(p)
	if err != nil {
		return ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	if p.Ref.Watermark != "" && p.Ref.Watermark != expectedWatermark {
		return ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route supplied a wrong nonzero watermark")
	}
	p.Ref.Watermark = expectedWatermark
	p.Ref, err = SealControlledOperationProviderRouteCurrentRefV2(p.Ref)
	if err != nil {
		return ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	providedProjectionDigest := p.ProjectionDigest
	p.ProjectionDigest = ""
	digest, err := p.DigestV2()
	if err != nil {
		return ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	if providedProjectionDigest != "" && providedProjectionDigest != digest {
		return ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route supplied a wrong nonzero projection digest")
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

func (p ControlledOperationProviderRouteCurrentProjectionV2) ValidateCurrent(expected ControlledOperationProviderRouteCurrentRefV2, matrix OperationScopeEvidenceApplicabilityMatrixKeyV3, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	matrixDigest, err := DigestOperationScopeEvidenceApplicabilityMatrixKeyV3(matrix)
	if err != nil || p.Ref != expected || p.Ref.MatrixDigest != matrixDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route current key or matrix drifted")
	}
	if now.IsZero() || now.Before(time.Unix(0, p.CheckedUnixNano)) || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled Provider route is not current")
	}
	return nil
}

func DigestOperationScopeEvidenceApplicabilityMatrixKeyV3(matrix OperationScopeEvidenceApplicabilityMatrixKeyV3) (core.Digest, error) {
	if err := matrix.Validate(); err != nil || !IsOperationScopeEvidenceActionMatrixKeyV3(matrix) {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "controlled Provider route matrix is unsupported")
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceApplicabilityMatrixKeyV3", matrix)
}

func DeriveControlledOperationProviderRouteCurrentIDV2(routeID string, matrixDigest core.Digest) (string, error) {
	if validateEvidenceIDV2(routeID) != nil || matrixDigest.Validate() != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider route current identity input is invalid")
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.controlled-operation-provider-route-current", ControlledOperationProviderRouteCurrentContractVersionV2, "ControlledOperationProviderRouteCurrentIdentityV2", struct {
		RouteID      string      `json:"route_id"`
		MatrixDigest core.Digest `json:"matrix_digest"`
	}{routeID, matrixDigest})
	if err != nil {
		return "", err
	}
	return "controlled-provider-route-current-" + string(digest)[len("sha256:"):], nil
}

func DigestControlledOperationProviderRouteWatermarkV2(p ControlledOperationProviderRouteCurrentProjectionV2) (core.Digest, error) {
	if p.Generation.Validate() != nil || validateEvidenceIDV2(p.HandoffID) != nil || p.HandoffRevision == 0 || p.HandoffDigest.Validate() != nil || validateEvidenceIDV2(p.BindingSetID) != nil || p.BindingSetRevision == 0 || p.BindingSetDigest.Validate() != nil || p.BindingSetSemanticDigest.Validate() != nil || p.BindingSetCurrentnessDigest.Validate() != nil || validateEvidenceIDV2(p.ActiveRouteID) != nil || p.ActiveRouteRevision == 0 || p.ActiveRouteDigest.Validate() != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider route watermark inputs are incomplete")
	}
	return core.CanonicalJSONDigest("praxis.runtime.controlled-operation-provider-route-current", ControlledOperationProviderRouteCurrentContractVersionV2, "ControlledOperationProviderRouteWatermarkV2", struct {
		ConformanceRef              ControlledOperationProviderRouteConformanceRefV2 `json:"conformance_ref"`
		Generation                  GenerationArtifactRefV1                          `json:"generation"`
		HandoffID                   string                                           `json:"handoff_id"`
		HandoffRevision             core.Revision                                    `json:"handoff_revision"`
		HandoffDigest               core.Digest                                      `json:"handoff_digest"`
		BindingSetID                string                                           `json:"binding_set_id"`
		BindingSetRevision          core.Revision                                    `json:"binding_set_revision"`
		BindingSetDigest            core.Digest                                      `json:"binding_set_digest"`
		BindingSetSemanticDigest    core.Digest                                      `json:"binding_set_semantic_digest"`
		BindingSetCurrentnessDigest core.Digest                                      `json:"binding_set_currentness_digest"`
		ActiveRouteID               string                                           `json:"active_route_id"`
		ActiveRouteRevision         core.Revision                                    `json:"active_route_revision"`
		ActiveRouteDigest           core.Digest                                      `json:"active_route_digest"`
		ToolAdapterBinding          ProviderBindingRefV2                             `json:"tool_adapter_binding"`
		GatewayBinding              ProviderBindingRefV2                             `json:"gateway_binding"`
		ProviderTransportBinding    ProviderBindingRefV2                             `json:"provider_transport_binding"`
		PreparedReaderBinding       ProviderBindingRefV2                             `json:"prepared_reader_binding"`
		BoundaryReaderBinding       ProviderBindingRefV2                             `json:"boundary_reader_binding"`
		ProviderInspectBinding      ProviderBindingRefV2                             `json:"provider_inspect_binding"`
		ProviderBinding             ProviderBindingRefV2                             `json:"provider_binding"`
	}{p.ConformanceRef, p.Generation, p.HandoffID, p.HandoffRevision, p.HandoffDigest, p.BindingSetID, p.BindingSetRevision, p.BindingSetDigest, p.BindingSetSemanticDigest, p.BindingSetCurrentnessDigest, p.ActiveRouteID, p.ActiveRouteRevision, p.ActiveRouteDigest, p.ToolAdapterBinding, p.GatewayBinding, p.ProviderTransportBinding, p.PreparedReaderBinding, p.BoundaryReaderBinding, p.ProviderInspectBinding, p.ProviderBinding})
}

type ControlledOperationProviderRouteCurrentReaderV2 interface {
	InspectCurrentControlledOperationProviderRouteV2(context.Context, ControlledOperationProviderRouteCurrentRefV2, OperationScopeEvidenceApplicabilityMatrixKeyV3) (ControlledOperationProviderRouteCurrentProjectionV2, error)
}

type ControlledOperationPreparedSemanticSnapshotV2 struct {
	Prepared             PreparedProviderAttemptRefV2       `json:"prepared"`
	Delegation           ExecutionDelegationRefV2           `json:"delegation"`
	PersistedEnforcement PersistedOperationEnforcementRefV3 `json:"persisted_enforcement"`
	OperationDigest      core.Digest                        `json:"operation_digest"`
	EffectID             core.EffectIntentID                `json:"effect_id"`
	IntentRevision       core.Revision                      `json:"intent_revision"`
	IntentDigest         core.Digest                        `json:"intent_digest"`
	Attempt              OperationDispatchAttemptRefV3      `json:"attempt"`
	ProviderBinding      ProviderBindingRefV2               `json:"provider_binding"`
	PayloadSchema        SchemaRefV2                        `json:"payload_schema"`
	PayloadDigest        core.Digest                        `json:"payload_digest"`
	PayloadRevision      core.Revision                      `json:"payload_revision"`
	SemanticDigest       core.Digest                        `json:"semantic_digest"`
}

func (s ControlledOperationPreparedSemanticSnapshotV2) Validate() error {
	if s.Prepared.Validate() != nil || s.Delegation.Validate() != nil || s.PersistedEnforcement.Validate() != nil || s.Attempt.Validate() != nil || s.ProviderBinding.Validate() != nil || s.PayloadSchema.Validate() != nil || s.OperationDigest.Validate() != nil || s.IntentDigest.Validate() != nil || s.PayloadDigest.Validate() != nil || s.SemanticDigest.Validate() != nil || s.EffectID == "" || s.IntentRevision == 0 || s.PayloadRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "controlled Provider prepared snapshot is incomplete")
	}
	if s.Prepared.OperationDigest != s.OperationDigest || s.Prepared.IntentID != s.EffectID || s.Prepared.IntentRevision != s.IntentRevision || s.Prepared.IntentDigest != s.IntentDigest || s.Prepared.AttemptID != s.Attempt.AttemptID || s.Prepared.PermitID != s.Attempt.PermitID || s.Prepared.PermitRevision != s.Attempt.PermitRevision || s.Prepared.PermitDigest != s.Attempt.PermitDigest || s.Prepared.Provider != s.ProviderBinding || s.Prepared.PayloadSchema != s.PayloadSchema || s.Prepared.PayloadDigest != s.PayloadDigest || s.Prepared.PayloadRevision != s.PayloadRevision || s.PersistedEnforcement.OperationDigest != s.OperationDigest || s.PersistedEnforcement.AttemptID != s.Attempt.AttemptID || s.PersistedEnforcement.Provider != s.ProviderBinding || s.Delegation.ID != s.Prepared.DeclaredDelegation.ID || s.Delegation.Revision <= s.Prepared.DeclaredDelegation.Revision {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "controlled Provider prepared snapshot binds another attempt")
	}
	digest, err := s.DigestV2()
	if err != nil || digest != s.SemanticDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider prepared snapshot digest drifted")
	}
	return nil
}

func (s ControlledOperationPreparedSemanticSnapshotV2) DigestV2() (core.Digest, error) {
	s.SemanticDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-operation-provider", ControlledOperationProviderContractVersionV2, "ControlledOperationPreparedSemanticSnapshotV2", s)
}

func SealControlledOperationPreparedSemanticSnapshotV2(s ControlledOperationPreparedSemanticSnapshotV2) (ControlledOperationPreparedSemanticSnapshotV2, error) {
	s.SemanticDigest = ""
	digest, err := s.DigestV2()
	if err != nil {
		return ControlledOperationPreparedSemanticSnapshotV2{}, err
	}
	s.SemanticDigest = digest
	return s, s.Validate()
}

type ControlledOperationPreparedCurrentProjectionV2 struct {
	ContractVersion  string                                        `json:"contract_version"`
	Snapshot         ControlledOperationPreparedSemanticSnapshotV2 `json:"snapshot"`
	CheckedUnixNano  int64                                         `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                         `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                                   `json:"projection_digest"`
}

func (p ControlledOperationPreparedCurrentProjectionV2) Validate() error {
	if p.ContractVersion != ControlledOperationProviderContractVersionV2 || p.Snapshot.Validate() != nil || p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano || p.ExpiresUnixNano > p.Snapshot.Prepared.ExpiresUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "controlled Provider prepared current projection is incomplete")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider prepared current projection digest drifted")
	}
	return nil
}

func (p ControlledOperationPreparedCurrentProjectionV2) DigestV2() (core.Digest, error) {
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-operation-provider", ControlledOperationProviderContractVersionV2, "ControlledOperationPreparedCurrentProjectionV2", p)
}

func SealControlledOperationPreparedCurrentProjectionV2(p ControlledOperationPreparedCurrentProjectionV2) (ControlledOperationPreparedCurrentProjectionV2, error) {
	p.ContractVersion = ControlledOperationProviderContractVersionV2
	p.ProjectionDigest = ""
	digest, err := p.DigestV2()
	if err != nil {
		return ControlledOperationPreparedCurrentProjectionV2{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

type ControlledOperationPreparedCurrentReaderV2 interface {
	InspectCurrentControlledOperationPreparedV2(context.Context, PreparedProviderAttemptRefV2) (ControlledOperationPreparedCurrentProjectionV2, error)
}

type ControlledOperationEffectCurrentProjectionV2 struct {
	Intent          OperationEffectIntentV3 `json:"intent"`
	IntentDigest    core.Digest             `json:"intent_digest"`
	FactRevision    core.Revision           `json:"fact_revision"`
	State           string                  `json:"state"`
	CheckedUnixNano int64                   `json:"checked_unix_nano"`
	ExpiresUnixNano int64                   `json:"expires_unix_nano"`
	Digest          core.Digest             `json:"digest"`
}

func (p ControlledOperationEffectCurrentProjectionV2) DigestV2() (core.Digest, error) {
	p.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-operation-provider", ControlledOperationProviderContractVersionV2, "ControlledOperationEffectCurrentProjectionV2", p)
}

func SealControlledOperationEffectCurrentProjectionV2(p ControlledOperationEffectCurrentProjectionV2) (ControlledOperationEffectCurrentProjectionV2, error) {
	p.Digest = ""
	digest, err := p.DigestV2()
	if err != nil {
		return ControlledOperationEffectCurrentProjectionV2{}, err
	}
	p.Digest = digest
	return p, p.Validate(time.Unix(0, p.CheckedUnixNano))
}

func (p ControlledOperationEffectCurrentProjectionV2) Validate(now time.Time) error {
	if p.Intent.Validate() != nil || p.IntentDigest.Validate() != nil || p.FactRevision == 0 || p.State == "" || p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano || now.IsZero() || now.Before(time.Unix(0, p.CheckedUnixNano)) || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "controlled Provider Effect current projection is incomplete or expired")
	}
	intentDigest, err := p.Intent.DigestV3()
	if err != nil || intentDigest != p.IntentDigest || p.ExpiresUnixNano > p.Intent.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "controlled Provider Effect current projection drifted")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider Effect current projection digest drifted")
	}
	return nil
}

type ControlledOperationEffectCurrentReaderV2 interface {
	InspectCurrentControlledOperationEffectV2(context.Context, OperationSubjectV3, core.EffectIntentID) (ControlledOperationEffectCurrentProjectionV2, error)
}

type ControlledOperationEvidencePolicyCurrentReaderV2 interface {
	InspectCurrentControlledOperationEvidencePolicyV2(context.Context, OperationScopeEvidencePolicyRefV3) (OperationScopeEvidencePolicyFactV3, error)
	InspectCurrentControlledOperationApplicabilityPolicyV2(context.Context, OperationScopeEvidenceApplicabilityPolicyRefV3) (OperationScopeEvidenceApplicabilityPolicyFactV3, error)
}

type ControlledOperationProviderRequestV2 struct {
	ContractVersion        string                                           `json:"contract_version"`
	RouteDeclarationRef    ControlledOperationProviderRouteDeclarationRefV2 `json:"route_declaration_ref"`
	RouteConformanceRef    ControlledOperationProviderRouteConformanceRefV2 `json:"route_conformance_ref"`
	RouteCurrentRef        ControlledOperationProviderRouteCurrentRefV2     `json:"route_current_ref"`
	ToolAdapterBinding     ProviderBindingRefV2                             `json:"tool_adapter_binding"`
	Operation              OperationSubjectV3                               `json:"operation"`
	OperationDigest        core.Digest                                      `json:"operation_digest"`
	OperationScopeDigest   core.Digest                                      `json:"operation_scope_digest"`
	EffectID               core.EffectIntentID                              `json:"effect_id"`
	EffectRevision         core.Revision                                    `json:"effect_revision"`
	EffectKind             EffectKindV2                                     `json:"effect_kind"`
	IntentDigest           core.Digest                                      `json:"intent_digest"`
	Attempt                OperationDispatchAttemptRefV3                    `json:"attempt"`
	ProviderBinding        ProviderBindingRefV2                             `json:"provider_binding"`
	Prepared               PreparedProviderAttemptRefV2                     `json:"prepared"`
	PreparedSemantics      ControlledOperationPreparedSemanticSnapshotV2    `json:"prepared_semantics"`
	ExecuteEnforcement     OperationDispatchEnforcementPhaseRefV4           `json:"execute_enforcement"`
	ExecuteEvidenceHandoff OperationScopeEvidenceProviderHandoffRefV3       `json:"execute_evidence_handoff"`
	Boundary               OperationProviderBoundaryRefV1                   `json:"boundary"`
	EvidencePolicy         OperationScopeEvidencePolicyRefV3                `json:"evidence_policy"`
	ApplicabilityPolicy    OperationScopeEvidenceApplicabilityPolicyRefV3   `json:"applicability_policy"`
	CallerDeadlineUnixNano int64                                            `json:"caller_deadline_unix_nano"`
	RequestDigest          core.Digest                                      `json:"request_digest"`
}

func (r ControlledOperationProviderRequestV2) Validate() error {
	if r.ContractVersion != ControlledOperationProviderContractVersionV2 || r.RouteDeclarationRef.Validate() != nil || r.RouteConformanceRef.Validate() != nil || r.RouteCurrentRef.Validate() != nil || r.ToolAdapterBinding.Validate() != nil || r.Operation.Validate() != nil || r.OperationDigest.Validate() != nil || r.OperationScopeDigest.Validate() != nil || r.EffectID == "" || r.EffectRevision == 0 || ValidateNamespacedNameV2(NamespacedNameV2(r.EffectKind)) != nil || r.IntentDigest.Validate() != nil || r.Attempt.Validate() != nil || r.ProviderBinding.Validate() != nil || r.Prepared.Validate() != nil || r.PreparedSemantics.Validate() != nil || r.ExecuteEnforcement.Validate() != nil || r.ExecuteEvidenceHandoff.Validate() != nil || r.Boundary.Validate() != nil || r.EvidencePolicy.Validate() != nil || r.ApplicabilityPolicy.Validate() != nil || r.CallerDeadlineUnixNano <= 0 || r.RequestDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "controlled Provider request is incomplete")
	}
	operationDigest, err := r.Operation.DigestV3()
	if err != nil || operationDigest != r.OperationDigest || r.Operation.ExecutionScopeDigest != r.OperationScopeDigest || r.RouteCurrentRef.DeclarationRef != r.RouteDeclarationRef || r.RouteCurrentRef.ConformanceRef != r.RouteConformanceRef || r.ToolAdapterBinding.Capability != ControlledOperationToolAdapterCapabilityV2 || r.ProviderBinding.Capability != CapabilityNameV2(r.EffectKind) || r.Attempt.OperationDigest != r.OperationDigest || r.Attempt.EffectID != r.EffectID || r.Attempt.IntentRevision != r.Prepared.IntentRevision || r.Attempt.IntentDigest != r.IntentDigest || r.Prepared != r.PreparedSemantics.Prepared || r.PreparedSemantics.ProviderBinding != r.ProviderBinding || r.ExecuteEnforcement.OperationDigest != r.OperationDigest || r.ExecuteEnforcement.EffectID != r.EffectID || r.ExecuteEnforcement.AttemptID != r.Attempt.AttemptID || r.ExecuteEnforcement.Phase != OperationDispatchEnforcementExecuteV4 || r.Boundary.ID == "" {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "controlled Provider request binds another route or attempt")
	}
	digest, err := r.DigestV2()
	if err != nil || digest != r.RequestDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider request digest drifted")
	}
	return nil
}

func (r ControlledOperationProviderRequestV2) DigestV2() (core.Digest, error) {
	r.RequestDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-operation-provider", ControlledOperationProviderContractVersionV2, "ControlledOperationProviderRequestV2", r)
}

func SealControlledOperationProviderRequestV2(r ControlledOperationProviderRequestV2) (ControlledOperationProviderRequestV2, error) {
	r.ContractVersion = ControlledOperationProviderContractVersionV2
	r.RequestDigest = ""
	digest, err := r.DigestV2()
	if err != nil {
		return ControlledOperationProviderRequestV2{}, err
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

type ControlledOperationProviderEntryRefV2 struct {
	EntryID         string        `json:"entry_id"`
	Revision        core.Revision `json:"revision"`
	StableKeyDigest core.Digest   `json:"stable_key_digest"`
	Digest          core.Digest   `json:"digest"`
}

type ControlledOperationProviderInspectKeyV2 struct {
	EntryID               string      `json:"entry_id"`
	StableKeyDigest       core.Digest `json:"stable_key_digest"`
	ExpectedRequestDigest core.Digest `json:"expected_request_digest"`
}

func (k ControlledOperationProviderInspectKeyV2) Validate() error {
	if validateEvidenceIDV2(k.EntryID) != nil || k.StableKeyDigest.Validate() != nil || k.ExpectedRequestDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider Inspect key is incomplete")
	}
	expectedID, err := DeriveControlledOperationProviderEntryIDV2(k.StableKeyDigest)
	if err != nil || expectedID != k.EntryID {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "controlled Provider Inspect key ID drifted")
	}
	return nil
}

func DeriveControlledOperationProviderEntryKeyV2(request ControlledOperationProviderRequestV2) (ControlledOperationProviderInspectKeyV2, error) {
	if err := request.Validate(); err != nil {
		return ControlledOperationProviderInspectKeyV2{}, err
	}
	stable, err := core.CanonicalJSONDigest("praxis.runtime.controlled-operation-provider", ControlledOperationProviderContractVersionV2, "ControlledOperationProviderEntryStableKeyV2", struct {
		OperationDigest core.Digest                   `json:"operation_digest"`
		EffectID        core.EffectIntentID           `json:"effect_id"`
		Attempt         OperationDispatchAttemptRefV3 `json:"attempt"`
		PreparedID      string                        `json:"prepared_id"`
		PreparedDigest  core.Digest                   `json:"prepared_digest"`
	}{request.OperationDigest, request.EffectID, request.Attempt, request.Prepared.ID, request.Prepared.Digest})
	if err != nil {
		return ControlledOperationProviderInspectKeyV2{}, err
	}
	entryID, err := DeriveControlledOperationProviderEntryIDV2(stable)
	if err != nil {
		return ControlledOperationProviderInspectKeyV2{}, err
	}
	return ControlledOperationProviderInspectKeyV2{EntryID: entryID, StableKeyDigest: stable, ExpectedRequestDigest: request.RequestDigest}, nil
}

func DeriveControlledOperationProviderEntryIDV2(stableKeyDigest core.Digest) (string, error) {
	if err := stableKeyDigest.Validate(); err != nil {
		return "", err
	}
	return "controlled-provider-" + strings.TrimPrefix(string(stableKeyDigest), "sha256:"), nil
}

func (r ControlledOperationProviderEntryRefV2) Validate() error {
	if validateEvidenceIDV2(r.EntryID) != nil || r.Revision == 0 || r.StableKeyDigest.Validate() != nil || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider Entry ref is incomplete")
	}
	return nil
}

type ControlledOperationProviderResultStatusV2 string
type ControlledOperationProviderResultErrorV2 string

const (
	ControlledOperationProviderEnteredV2          ControlledOperationProviderResultStatusV2 = "entered"
	ControlledOperationProviderUnknownV2          ControlledOperationProviderResultStatusV2 = "unknown"
	ControlledOperationProviderObservedV2         ControlledOperationProviderResultStatusV2 = "observed"
	ControlledOperationProviderRejectedNoEffectV2 ControlledOperationProviderResultStatusV2 = "rejected_no_effect"

	ControlledOperationProviderErrorNoneV2             ControlledOperationProviderResultErrorV2 = "none"
	ControlledOperationProviderInspectionRequiredV2    ControlledOperationProviderResultErrorV2 = "inspection_required"
	ControlledOperationProviderOutcomeUnknownV2        ControlledOperationProviderResultErrorV2 = "provider_outcome_unknown"
	ControlledOperationProviderInspectionUnavailableV2 ControlledOperationProviderResultErrorV2 = "inspection_unavailable"
)

type ControlledOperationProviderAdmissionReceiptRefV2 struct {
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	StableKeyDigest core.Digest   `json:"stable_key_digest"`
	Admitted        bool          `json:"admitted"`
	NoEffect        bool          `json:"no_effect"`
	Digest          core.Digest   `json:"digest"`
}

func (r ControlledOperationProviderAdmissionReceiptRefV2) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || r.StableKeyDigest.Validate() != nil || r.Digest.Validate() != nil || r.Admitted && r.NoEffect || !r.Admitted && !r.NoEffect {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceConflict, "controlled Provider admission receipt is invalid")
	}
	digest, err := r.DigestV2()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider admission receipt digest drifted")
	}
	return nil
}

func (r ControlledOperationProviderAdmissionReceiptRefV2) DigestV2() (core.Digest, error) {
	r.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-operation-provider", ControlledOperationProviderContractVersionV2, "ControlledOperationProviderAdmissionReceiptRefV2", r)
}

func SealControlledOperationProviderAdmissionReceiptRefV2(r ControlledOperationProviderAdmissionReceiptRefV2) (ControlledOperationProviderAdmissionReceiptRefV2, error) {
	r.Digest = ""
	digest, err := r.DigestV2()
	if err != nil {
		return ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	r.Digest = digest
	return r, r.Validate()
}

type ControlledOperationProviderResultV2 struct {
	ContractVersion   string                                            `json:"contract_version"`
	EntryRef          ControlledOperationProviderEntryRefV2             `json:"entry_ref"`
	Status            ControlledOperationProviderResultStatusV2         `json:"status"`
	Error             ControlledOperationProviderResultErrorV2          `json:"error"`
	Prepared          PreparedProviderAttemptRefV2                      `json:"prepared"`
	Attempt           OperationDispatchAttemptRefV3                     `json:"attempt"`
	AdmissionReceipt  *ControlledOperationProviderAdmissionReceiptRefV2 `json:"admission_receipt,omitempty"`
	Observation       *ProviderAttemptObservationRefV2                  `json:"observation,omitempty"`
	InspectedUnixNano int64                                             `json:"inspected_unix_nano"`
	ResultDigest      core.Digest                                       `json:"result_digest"`
}

func (r ControlledOperationProviderResultV2) Validate() error {
	if r.ContractVersion != ControlledOperationProviderContractVersionV2 || r.EntryRef.Validate() != nil || r.Prepared.Validate() != nil || r.Attempt.Validate() != nil || r.InspectedUnixNano <= 0 || r.ResultDigest.Validate() != nil || r.Prepared.AttemptID != r.Attempt.AttemptID {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceConflict, "controlled Provider result is incomplete")
	}
	switch r.Status {
	case ControlledOperationProviderEnteredV2:
		if r.Error != ControlledOperationProviderInspectionRequiredV2 || r.AdmissionReceipt != nil || r.Observation != nil {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "entered result sidecars are invalid")
		}
	case ControlledOperationProviderUnknownV2:
		if (r.Error != ControlledOperationProviderOutcomeUnknownV2 && r.Error != ControlledOperationProviderInspectionUnavailableV2) || r.AdmissionReceipt != nil || r.Observation != nil {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "unknown result sidecars are invalid")
		}
	case ControlledOperationProviderObservedV2:
		if r.Error != ControlledOperationProviderErrorNoneV2 || r.Observation == nil || r.Observation.Validate() != nil || r.Observation.PreparedAttemptID != r.Prepared.ID {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "observed result lacks exact observation")
		}
	case ControlledOperationProviderRejectedNoEffectV2:
		if r.Error != ControlledOperationProviderErrorNoneV2 || r.AdmissionReceipt == nil || r.AdmissionReceipt.Validate() != nil || !r.AdmissionReceipt.NoEffect || r.Observation != nil {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "no-effect result lacks verifiable receipt")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "controlled Provider result status is invalid")
	}
	digest, err := r.DigestV2()
	if err != nil || digest != r.ResultDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled Provider result digest drifted")
	}
	return nil
}

func (r ControlledOperationProviderResultV2) DigestV2() (core.Digest, error) {
	r.ResultDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-operation-provider", ControlledOperationProviderContractVersionV2, "ControlledOperationProviderResultV2", r)
}

func SealControlledOperationProviderResultV2(r ControlledOperationProviderResultV2) (ControlledOperationProviderResultV2, error) {
	r.ContractVersion = ControlledOperationProviderContractVersionV2
	r.ResultDigest = ""
	digest, err := r.DigestV2()
	if err != nil {
		return ControlledOperationProviderResultV2{}, err
	}
	r.ResultDigest = digest
	return r, r.Validate()
}

type ControlledProviderInspectPortV2 interface {
	InspectOriginalControlledProviderAttemptV2(context.Context, PreparedProviderAttemptRefV2, OperationDispatchAttemptRefV3) (ProviderAttemptObservationRefV2, error)
}

type ControlledOperationProviderInspectRequestV2 struct {
	Operation OperationSubjectV3                      `json:"operation"`
	Key       ControlledOperationProviderInspectKeyV2 `json:"key"`
}

func (r ControlledOperationProviderInspectRequestV2) Validate() error {
	if r.Operation.Validate() != nil || r.Key.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider Inspect request is incomplete")
	}
	return nil
}

type ControlledOperationProviderPortV2 interface {
	EnterControlledOperationProviderV2(context.Context, ControlledOperationProviderRequestV2) (ControlledOperationProviderResultV2, error)
	InspectControlledOperationProviderV2(context.Context, ControlledOperationProviderInspectRequestV2) (ControlledOperationProviderResultV2, error)
}

package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ControlledMCPDiscoveryPageContractVersionV1             = "1.0.0"
	ControlledMCPDiscoveryPageRouteCurrentContractVersionV1 = "1.0.0"
	MCPConnectionAvailabilityNeutralContractVersionV1       = "1.0.0"

	ControlledMCPDiscoveryPageProviderTransportCapabilityV1 CapabilityNameV2 = "praxis.mcp/controlled-transport-v1"
	MCPDiscoveryPageToolsNamespaceV1                        NamespacedNameV2 = "praxis.mcp/tools"
	MCPDiscoveryPageResourcesNamespaceV1                    NamespacedNameV2 = "praxis.mcp/resources"
	MCPDiscoveryPagePromptsNamespaceV1                      NamespacedNameV2 = "praxis.mcp/prompts"
)

func IsMCPDiscoveryPageNamespaceV1(value NamespacedNameV2) bool {
	switch value {
	case MCPDiscoveryPageToolsNamespaceV1, MCPDiscoveryPageResourcesNamespaceV1, MCPDiscoveryPagePromptsNamespaceV1:
		return true
	default:
		return false
	}
}

type ControlledMCPDiscoveryPageRouteCurrentRefV1 struct {
	CurrentID      string                                           `json:"current_id"`
	Revision       core.Revision                                    `json:"revision"`
	DeclarationRef ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	ConformanceRef ControlledOperationProviderRouteConformanceRefV2 `json:"conformance_ref"`
	MatrixDigest   core.Digest                                      `json:"matrix_digest"`
	Digest         core.Digest                                      `json:"digest"`
}

func (r ControlledMCPDiscoveryPageRouteCurrentRefV1) Validate() error {
	if validateEvidenceIDV2(r.CurrentID) != nil || r.Revision == 0 || r.DeclarationRef.Validate() != nil || r.ConformanceRef.Validate() != nil || r.MatrixDigest.Validate() != nil || r.Digest.Validate() != nil || r.ConformanceRef.DeclarationRef != r.DeclarationRef {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled MCP Discovery Page route ref is incomplete")
	}
	matrixDigest, err := DigestOperationScopeEvidenceMCPDiscoveryPageMatrixV1()
	if err != nil || r.MatrixDigest != matrixDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Discovery Page route matrix drifted")
	}
	id, err := DeriveControlledMCPDiscoveryPageRouteCurrentIDV1(r.DeclarationRef.RouteID, matrixDigest)
	if err != nil || r.CurrentID != id {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Discovery Page route ID drifted")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled MCP Discovery Page route ref digest drifted")
	}
	return nil
}

func (r ControlledMCPDiscoveryPageRouteCurrentRefV1) DigestV1() (core.Digest, error) {
	r.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-mcp-discovery-page-route", ControlledMCPDiscoveryPageRouteCurrentContractVersionV1, "ControlledMCPDiscoveryPageRouteCurrentRefV1", r)
}

type ControlledMCPDiscoveryPageRouteCurrentProjectionV1 struct {
	ContractVersion             string                                      `json:"contract_version"`
	Ref                         ControlledMCPDiscoveryPageRouteCurrentRefV1 `json:"ref"`
	Generation                  GenerationArtifactRefV1                     `json:"generation"`
	Assembly                    GenerationBindingAssociationRefV1           `json:"generation_binding_association"`
	HandoffID                   string                                      `json:"handoff_id"`
	HandoffRevision             core.Revision                               `json:"handoff_revision"`
	HandoffDigest               core.Digest                                 `json:"handoff_digest"`
	BindingSetID                string                                      `json:"binding_set_id"`
	BindingSetRevision          core.Revision                               `json:"binding_set_revision"`
	BindingSetDigest            core.Digest                                 `json:"binding_set_digest"`
	BindingSetSemanticDigest    core.Digest                                 `json:"binding_set_semantic_digest"`
	BindingSetCurrentnessDigest core.Digest                                 `json:"binding_set_currentness_digest"`
	ActiveRouteID               string                                      `json:"active_route_id"`
	ActiveRouteRevision         core.Revision                               `json:"active_route_revision"`
	ActiveRouteDigest           core.Digest                                 `json:"active_route_digest"`
	ProviderTransport           ProviderBindingRefV2                        `json:"provider_transport_binding"`
	Provider                    ProviderBindingRefV2                        `json:"provider_binding"`
	CheckedUnixNano             int64                                       `json:"checked_unix_nano"`
	ExpiresUnixNano             int64                                       `json:"expires_unix_nano"`
	ProjectionDigest            core.Digest                                 `json:"projection_digest"`
}

func (p ControlledMCPDiscoveryPageRouteCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ControlledMCPDiscoveryPageRouteCurrentContractVersionV1 || p.Ref.Validate() != nil || p.Generation.Validate() != nil || p.Assembly.Validate() != nil || validateEvidenceIDV2(p.HandoffID) != nil || p.HandoffRevision == 0 || p.HandoffDigest.Validate() != nil || validateEvidenceIDV2(p.BindingSetID) != nil || p.BindingSetRevision == 0 || p.BindingSetDigest.Validate() != nil || p.BindingSetSemanticDigest.Validate() != nil || p.BindingSetCurrentnessDigest.Validate() != nil || validateEvidenceIDV2(p.ActiveRouteID) != nil || p.ActiveRouteRevision == 0 || p.ActiveRouteDigest.Validate() != nil || p.ProviderTransport.Validate() != nil || p.Provider.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingDrift, "controlled MCP Discovery Page route projection is incomplete")
	}
	if p.ProviderTransport == p.Provider || p.ProviderTransport.BindingSetID != p.BindingSetID || p.Provider.BindingSetID != p.BindingSetID || p.ProviderTransport.BindingSetRevision != p.BindingSetRevision || p.Provider.BindingSetRevision != p.BindingSetRevision || p.ProviderTransport.Capability != ControlledMCPDiscoveryPageProviderTransportCapabilityV1 || p.Provider.Capability != CapabilityNameV2(OperationScopeEvidenceMCPDiscoveryPageEffectKindV1) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Discovery Page route bindings drifted")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled MCP Discovery Page route projection digest drifted")
	}
	return nil
}

func (p ControlledMCPDiscoveryPageRouteCurrentProjectionV1) ValidateCurrent(expected ControlledMCPDiscoveryPageRouteCurrentRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Ref != expected || now.IsZero() || now.Before(time.Unix(0, p.CheckedUnixNano)) || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled MCP Discovery Page route is not current")
	}
	return nil
}

func (p ControlledMCPDiscoveryPageRouteCurrentProjectionV1) DigestV1() (core.Digest, error) {
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-mcp-discovery-page-route", ControlledMCPDiscoveryPageRouteCurrentContractVersionV1, "ControlledMCPDiscoveryPageRouteCurrentProjectionV1", p)
}

func SealControlledMCPDiscoveryPageRouteCurrentProjectionV1(p ControlledMCPDiscoveryPageRouteCurrentProjectionV1) (ControlledMCPDiscoveryPageRouteCurrentProjectionV1, error) {
	p.ContractVersion = ControlledMCPDiscoveryPageRouteCurrentContractVersionV1
	matrixDigest, err := DigestOperationScopeEvidenceMCPDiscoveryPageMatrixV1()
	if err != nil {
		return ControlledMCPDiscoveryPageRouteCurrentProjectionV1{}, err
	}
	p.Ref.MatrixDigest = matrixDigest
	p.Ref.CurrentID, err = DeriveControlledMCPDiscoveryPageRouteCurrentIDV1(p.Ref.DeclarationRef.RouteID, matrixDigest)
	if err != nil {
		return ControlledMCPDiscoveryPageRouteCurrentProjectionV1{}, err
	}
	p.Ref.Digest = ""
	p.Ref.Digest, err = p.Ref.DigestV1()
	if err != nil {
		return ControlledMCPDiscoveryPageRouteCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = ""
	p.ProjectionDigest, err = p.DigestV1()
	if err != nil {
		return ControlledMCPDiscoveryPageRouteCurrentProjectionV1{}, err
	}
	return p, p.Validate()
}

func DigestOperationScopeEvidenceMCPDiscoveryPageMatrixV1() (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceApplicabilityMatrixKeyV3", OperationScopeEvidenceMCPDiscoveryPageMatrixV1())
}

func DeriveControlledMCPDiscoveryPageRouteCurrentIDV1(routeID string, matrixDigest core.Digest) (string, error) {
	if validateEvidenceIDV2(routeID) != nil || matrixDigest.Validate() != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled MCP Discovery Page route identity input is invalid")
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.controlled-mcp-discovery-page-route", ControlledMCPDiscoveryPageRouteCurrentContractVersionV1, "ControlledMCPDiscoveryPageRouteCurrentIdentityV1", struct {
		RouteID      string      `json:"route_id"`
		MatrixDigest core.Digest `json:"matrix_digest"`
	}{routeID, matrixDigest})
	if err != nil {
		return "", err
	}
	return "controlled-mcp-discovery-page-route-" + trimDigestPrefixV3(digest), nil
}

type ControlledMCPDiscoveryPageRouteCurrentReaderV1 interface {
	InspectCurrentControlledMCPDiscoveryPageRouteV1(context.Context, ControlledMCPDiscoveryPageRouteCurrentRefV1) (ControlledMCPDiscoveryPageRouteCurrentProjectionV1, error)
}

// MCPConnectionAvailabilityNeutralRefV1 is a Runtime-neutral exact lookup
// coordinate for a Tool-owned settled Connection. It grants no Provider,
// Runtime, Review, or Evidence authority.
type MCPConnectionAvailabilityNeutralRefV1 struct {
	Owner                  EffectOwnerRefV2 `json:"owner"`
	ConnectionID           string           `json:"connection_id"`
	ConnectionRevision     core.Revision    `json:"connection_revision"`
	ConnectionDigest       core.Digest      `json:"connection_digest"`
	ApplyID                string           `json:"apply_id"`
	ApplyRevision          core.Revision    `json:"apply_revision"`
	ApplyDigest            core.Digest      `json:"apply_digest"`
	DomainResultID         string           `json:"domain_result_id"`
	DomainResultRevision   core.Revision    `json:"domain_result_revision"`
	DomainResultDigest     core.Digest      `json:"domain_result_digest"`
	SourceProjectionDigest core.Digest      `json:"source_projection_digest"`
}

func (r MCPConnectionAvailabilityNeutralRefV1) Validate() error {
	if r.Owner.Role != OwnerSettlement || ValidateNamespacedNameV2(NamespacedNameV2(r.Owner.ComponentID)) != nil || r.Owner.ManifestDigest.Validate() != nil || validateEvidenceIDV2(r.ConnectionID) != nil || r.ConnectionRevision == 0 || r.ConnectionDigest.Validate() != nil || validateEvidenceIDV2(r.ApplyID) != nil || r.ApplyRevision == 0 || r.ApplyDigest.Validate() != nil || validateEvidenceIDV2(r.DomainResultID) != nil || r.DomainResultRevision == 0 || r.DomainResultDigest.Validate() != nil || r.SourceProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Connection availability neutral ref is incomplete")
	}
	return nil
}

type MCPConnectionAvailabilityNeutralProjectionV1 struct {
	ContractVersion   string                                `json:"contract_version"`
	Ref               MCPConnectionAvailabilityNeutralRefV1 `json:"ref"`
	TenantID          core.TenantID                         `json:"tenant_id"`
	RunID             string                                `json:"run_id"`
	SessionID         string                                `json:"session_id"`
	SessionRevision   core.Revision                         `json:"session_revision"`
	SessionDigest     core.Digest                           `json:"session_digest"`
	ConnectionEpoch   core.Epoch                            `json:"connection_epoch"`
	ProviderTransport ProviderBindingRefV2                  `json:"provider_transport_binding"`
	Provider          ProviderBindingRefV2                  `json:"provider_binding"`
	CheckedUnixNano   int64                                 `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                                 `json:"expires_unix_nano"`
	ProjectionDigest  core.Digest                           `json:"projection_digest"`
}

func (p MCPConnectionAvailabilityNeutralProjectionV1) Validate() error {
	if p.ContractVersion != MCPConnectionAvailabilityNeutralContractVersionV1 || p.Ref.Validate() != nil || validateEvidenceIDV2(string(p.TenantID)) != nil || validateEvidenceIDV2(p.RunID) != nil || validateEvidenceIDV2(p.SessionID) != nil || p.SessionRevision == 0 || p.SessionDigest.Validate() != nil || p.ConnectionEpoch == 0 || p.ProviderTransport.Validate() != nil || p.Provider.Validate() != nil || p.ProviderTransport == p.Provider || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Connection availability neutral projection is incomplete")
	}
	if p.Ref.Owner.ComponentID != p.Provider.ComponentID || p.Ref.Owner.ManifestDigest != p.Provider.ManifestDigest || p.ProviderTransport.BindingSetID != p.Provider.BindingSetID || p.ProviderTransport.BindingSetRevision != p.Provider.BindingSetRevision {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connection availability neutral bindings drifted")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "MCP Connection availability neutral projection digest drifted")
	}
	return nil
}

func (p MCPConnectionAvailabilityNeutralProjectionV1) ValidateCurrent(expected MCPConnectionAvailabilityNeutralRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Ref != expected || now.IsZero() || now.Before(time.Unix(0, p.CheckedUnixNano)) || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Connection availability is not current")
	}
	return nil
}

func (p MCPConnectionAvailabilityNeutralProjectionV1) DigestV1() (core.Digest, error) {
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.mcp-connection-availability", MCPConnectionAvailabilityNeutralContractVersionV1, "MCPConnectionAvailabilityNeutralProjectionV1", p)
}

func SealMCPConnectionAvailabilityNeutralProjectionV1(p MCPConnectionAvailabilityNeutralProjectionV1) (MCPConnectionAvailabilityNeutralProjectionV1, error) {
	p.ContractVersion = MCPConnectionAvailabilityNeutralContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return MCPConnectionAvailabilityNeutralProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

type MCPConnectionAvailabilityNeutralCurrentReaderV1 interface {
	InspectCurrentMCPConnectionAvailabilityNeutralV1(context.Context, MCPConnectionAvailabilityNeutralRefV1) (MCPConnectionAvailabilityNeutralProjectionV1, error)
}

type ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1 struct {
	Route                  ControlledMCPDiscoveryPageRouteCurrentRefV1 `json:"route"`
	Execute                ExecutePreparedRequestV2                    `json:"execute"`
	Attempt                OperationDispatchAttemptRefV3               `json:"attempt"`
	ExecuteEnforcement     OperationDispatchEnforcementPhaseRefV4      `json:"execute_enforcement"`
	PrepareConsumption     OperationScopeEvidenceConsumptionRefV3      `json:"prepare_consumption"`
	ExecuteHandoff         OperationScopeEvidenceProviderHandoffRefV3  `json:"execute_handoff"`
	Association            PreparedDomainCommandAssociationRefV1       `json:"association"`
	DomainCommand          OperationDomainCommandRefV1                 `json:"domain_command"`
	ConnectionAvailability MCPConnectionAvailabilityNeutralRefV1       `json:"connection_availability"`
	Namespace              NamespacedNameV2                            `json:"namespace"`
	CursorDigest           core.Digest                                 `json:"cursor_digest"`
	PageOrdinal            uint32                                      `json:"page_ordinal"`
	CallerDeadlineUnixNano int64                                       `json:"caller_deadline_unix_nano"`
}

func (r ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1) Validate() error {
	if r.Route.Validate() != nil || r.Execute.Validate() != nil || r.Attempt.Validate() != nil || r.ExecuteEnforcement.Validate() != nil || r.PrepareConsumption.Validate() != nil || r.ExecuteHandoff.Validate() != nil || r.Association.Validate() != nil || r.DomainCommand.Validate() != nil || r.ConnectionAvailability.Validate() != nil || !IsMCPDiscoveryPageNamespaceV1(r.Namespace) || r.CursorDigest.Validate() != nil || r.CallerDeadlineUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "controlled MCP Discovery Page physical authorization request is incomplete")
	}
	intentDigest, err := r.Execute.Intent.DigestV3()
	if err != nil || r.Execute.Intent.Kind != OperationScopeEvidenceMCPDiscoveryPageEffectKindV1 || r.Execute.Intent.Operation.Kind != OperationScopeRunV3 || r.Attempt.OperationDigest != r.Execute.Prepared.OperationDigest || r.Attempt.EffectID != r.Execute.Intent.ID || r.Attempt.IntentRevision != r.Execute.Intent.Revision || r.Attempt.IntentDigest != intentDigest || r.Attempt.AttemptID != r.Execute.Prepared.AttemptID || r.ExecuteEnforcement.OperationDigest != r.Attempt.OperationDigest || r.ExecuteEnforcement.EffectID != r.Attempt.EffectID || r.ExecuteEnforcement.AttemptID != r.Attempt.AttemptID || r.ExecuteEnforcement.Phase != OperationDispatchEnforcementExecuteV4 {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Discovery Page physical request coordinates drifted")
	}
	return nil
}

type ControlledMCPDiscoveryPagePhysicalAuthorizationV1 struct {
	ContractVersion         string                                      `json:"contract_version"`
	StableKeyDigest         core.Digest                                 `json:"stable_key_digest"`
	UnifiedNotAfterUnixNano int64                                       `json:"unified_not_after_unix_nano"`
	Route                   ControlledMCPDiscoveryPageRouteCurrentRefV1 `json:"route"`
	ProviderTransport       ProviderBindingRefV2                        `json:"provider_transport_binding"`
	Provider                ProviderBindingRefV2                        `json:"provider_binding"`
	Operation               OperationSubjectV3                          `json:"operation"`
	OperationDigest         core.Digest                                 `json:"operation_digest"`
	OperationScopeDigest    core.Digest                                 `json:"operation_scope_digest"`
	EffectID                core.EffectIntentID                         `json:"effect_id"`
	EffectRevision          core.Revision                               `json:"effect_revision"`
	EffectFactRevision      core.Revision                               `json:"effect_fact_revision"`
	IntentDigest            core.Digest                                 `json:"intent_digest"`
	Prepared                PreparedProviderAttemptRefV2                `json:"prepared"`
	Attempt                 OperationDispatchAttemptRefV3               `json:"attempt"`
	ExecuteEnforcement      OperationDispatchEnforcementPhaseRefV4      `json:"execute_enforcement"`
	PrepareConsumption      OperationScopeEvidenceConsumptionRefV3      `json:"prepare_consumption"`
	ExecuteHandoff          OperationScopeEvidenceProviderHandoffRefV3  `json:"execute_handoff"`
	SandboxProjectionDigest core.Digest                                 `json:"sandbox_projection_digest"`
	CredentialFactsDigest   core.Digest                                 `json:"credential_facts_digest"`
	Association             PreparedDomainCommandAssociationRefV1       `json:"association"`
	DomainCommand           OperationDomainCommandRefV1                 `json:"domain_command"`
	ConnectionAvailability  MCPConnectionAvailabilityNeutralRefV1       `json:"connection_availability"`
	Namespace               NamespacedNameV2                            `json:"namespace"`
	CursorDigest            core.Digest                                 `json:"cursor_digest"`
	PageOrdinal             uint32                                      `json:"page_ordinal"`
	IssuedUnixNano          int64                                       `json:"issued_unix_nano"`
	Digest                  core.Digest                                 `json:"digest"`
}

func (a ControlledMCPDiscoveryPagePhysicalAuthorizationV1) Validate() error {
	if a.ContractVersion != ControlledMCPDiscoveryPageContractVersionV1 || a.StableKeyDigest.Validate() != nil || a.UnifiedNotAfterUnixNano <= 0 || a.Route.Validate() != nil || a.ProviderTransport.Validate() != nil || a.Provider.Validate() != nil || a.Operation.Validate() != nil || a.OperationDigest.Validate() != nil || a.OperationScopeDigest.Validate() != nil || validateEvidenceIDV2(string(a.EffectID)) != nil || a.EffectRevision == 0 || a.EffectFactRevision == 0 || a.IntentDigest.Validate() != nil || a.Prepared.Validate() != nil || a.Attempt.Validate() != nil || a.ExecuteEnforcement.Validate() != nil || a.PrepareConsumption.Validate() != nil || a.ExecuteHandoff.Validate() != nil || a.SandboxProjectionDigest.Validate() != nil || a.CredentialFactsDigest.Validate() != nil || a.Association.Validate() != nil || a.DomainCommand.Validate() != nil || a.ConnectionAvailability.Validate() != nil || !IsMCPDiscoveryPageNamespaceV1(a.Namespace) || a.CursorDigest.Validate() != nil || a.IssuedUnixNano <= 0 || a.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "controlled MCP Discovery Page physical authorization is incomplete")
	}
	operationDigest, err := a.Operation.DigestV3()
	if err != nil || a.Operation.Kind != OperationScopeRunV3 || operationDigest != a.OperationDigest || a.Operation.ExecutionScopeDigest != a.OperationScopeDigest || a.Attempt.OperationDigest != a.OperationDigest || a.Attempt.EffectID != a.EffectID || a.Attempt.IntentRevision != a.EffectRevision || a.Attempt.IntentDigest != a.IntentDigest || a.Prepared.AttemptID != a.Attempt.AttemptID || a.ExecuteEnforcement.AttemptID != a.Attempt.AttemptID || a.ExecuteEnforcement.Phase != OperationDispatchEnforcementExecuteV4 || a.DomainCommand.Owner.Role != OwnerSettlement || a.DomainCommand.Owner.ComponentID != a.Provider.ComponentID || a.DomainCommand.Owner.ManifestDigest != a.Provider.ManifestDigest || a.ConnectionAvailability.Owner.ComponentID != a.Provider.ComponentID || a.ConnectionAvailability.Owner.ManifestDigest != a.Provider.ManifestDigest || a.ProviderTransport == a.Provider || a.ProviderTransport.Capability != ControlledMCPDiscoveryPageProviderTransportCapabilityV1 || a.Provider.Capability != CapabilityNameV2(OperationScopeEvidenceMCPDiscoveryPageEffectKindV1) || a.IssuedUnixNano >= a.UnifiedNotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Discovery Page physical authorization bindings drifted")
	}
	stable, err := DigestControlledMCPDiscoveryPagePhysicalStableKeyV1(a)
	if err != nil || stable != a.StableKeyDigest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "controlled MCP Discovery Page physical stable key drifted")
	}
	digest, err := a.DigestV1()
	if err != nil || digest != a.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled MCP Discovery Page physical authorization digest drifted")
	}
	return nil
}

func (a ControlledMCPDiscoveryPagePhysicalAuthorizationV1) ValidateCurrent(now time.Time) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.Before(time.Unix(0, a.IssuedUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled MCP Discovery Page authorization clock regressed")
	}
	if !now.Before(time.Unix(0, a.UnifiedNotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled MCP Discovery Page authorization expired")
	}
	return nil
}

func (a ControlledMCPDiscoveryPagePhysicalAuthorizationV1) DigestV1() (core.Digest, error) {
	a.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-mcp-discovery-page", ControlledMCPDiscoveryPageContractVersionV1, "ControlledMCPDiscoveryPagePhysicalAuthorizationV1", a)
}

func DigestControlledMCPDiscoveryPagePhysicalStableKeyV1(a ControlledMCPDiscoveryPagePhysicalAuthorizationV1) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.runtime.controlled-mcp-discovery-page", ControlledMCPDiscoveryPageContractVersionV1, "ControlledMCPDiscoveryPagePhysicalStableKeyV1", struct {
		OperationDigest        core.Digest                                 `json:"operation_digest"`
		Attempt                OperationDispatchAttemptRefV3               `json:"attempt"`
		DomainCommand          OperationDomainCommandRefV1                 `json:"domain_command"`
		Route                  ControlledMCPDiscoveryPageRouteCurrentRefV1 `json:"route"`
		ConnectionAvailability MCPConnectionAvailabilityNeutralRefV1       `json:"connection_availability"`
		Namespace              NamespacedNameV2                            `json:"namespace"`
		CursorDigest           core.Digest                                 `json:"cursor_digest"`
		PageOrdinal            uint32                                      `json:"page_ordinal"`
	}{a.OperationDigest, a.Attempt, a.DomainCommand, a.Route, a.ConnectionAvailability, a.Namespace, a.CursorDigest, a.PageOrdinal})
}

func SealControlledMCPDiscoveryPagePhysicalAuthorizationV1(a ControlledMCPDiscoveryPagePhysicalAuthorizationV1) (ControlledMCPDiscoveryPagePhysicalAuthorizationV1, error) {
	a.ContractVersion = ControlledMCPDiscoveryPageContractVersionV1
	a.StableKeyDigest = ""
	stable, err := DigestControlledMCPDiscoveryPagePhysicalStableKeyV1(a)
	if err != nil {
		return ControlledMCPDiscoveryPagePhysicalAuthorizationV1{}, err
	}
	a.StableKeyDigest = stable
	a.Digest = ""
	a.Digest, err = a.DigestV1()
	if err != nil {
		return ControlledMCPDiscoveryPagePhysicalAuthorizationV1{}, err
	}
	return a, a.Validate()
}

type ControlledMCPDiscoveryPagePhysicalAuthorizationPortV1 interface {
	AuthorizeControlledMCPDiscoveryPagePhysicalV1(context.Context, ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1) (ControlledMCPDiscoveryPagePhysicalAuthorizationV1, error)
}

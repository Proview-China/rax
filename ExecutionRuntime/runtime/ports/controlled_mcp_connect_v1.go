package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ControlledMCPConnectContractVersionV1             = "1.0.0"
	ControlledMCPConnectRouteCurrentContractVersionV1 = "1.0.0"

	ControlledMCPConnectProviderTransportCapabilityV1 CapabilityNameV2 = "praxis.mcp/controlled-transport-v1"
)

type ControlledMCPConnectRouteCurrentRefV1 struct {
	CurrentID      string                                           `json:"current_id"`
	Revision       core.Revision                                    `json:"revision"`
	DeclarationRef ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	ConformanceRef ControlledOperationProviderRouteConformanceRefV2 `json:"conformance_ref"`
	MatrixDigest   core.Digest                                      `json:"matrix_digest"`
	Digest         core.Digest                                      `json:"digest"`
}

func (r ControlledMCPConnectRouteCurrentRefV1) Validate() error {
	if validateEvidenceIDV2(r.CurrentID) != nil || r.Revision == 0 || r.DeclarationRef.Validate() != nil || r.ConformanceRef.Validate() != nil || r.MatrixDigest.Validate() != nil || r.Digest.Validate() != nil || r.ConformanceRef.DeclarationRef != r.DeclarationRef {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled MCP Connect route ref is incomplete")
	}
	matrixDigest, err := DigestOperationScopeEvidenceMCPConnectMatrixV1()
	if err != nil || r.MatrixDigest != matrixDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Connect route matrix drifted")
	}
	id, err := DeriveControlledMCPConnectRouteCurrentIDV1(r.DeclarationRef.RouteID, matrixDigest)
	if err != nil || r.CurrentID != id {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Connect route ID drifted")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled MCP Connect route ref digest drifted")
	}
	return nil
}

func (r ControlledMCPConnectRouteCurrentRefV1) DigestV1() (core.Digest, error) {
	r.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-mcp-connect-route", ControlledMCPConnectRouteCurrentContractVersionV1, "ControlledMCPConnectRouteCurrentRefV1", r)
}

type ControlledMCPConnectRouteCurrentProjectionV1 struct {
	ContractVersion             string                                `json:"contract_version"`
	Ref                         ControlledMCPConnectRouteCurrentRefV1 `json:"ref"`
	Generation                  GenerationArtifactRefV1               `json:"generation"`
	Assembly                    GenerationBindingAssociationRefV1     `json:"generation_binding_association"`
	HandoffID                   string                                `json:"handoff_id"`
	HandoffRevision             core.Revision                         `json:"handoff_revision"`
	HandoffDigest               core.Digest                           `json:"handoff_digest"`
	BindingSetID                string                                `json:"binding_set_id"`
	BindingSetRevision          core.Revision                         `json:"binding_set_revision"`
	BindingSetDigest            core.Digest                           `json:"binding_set_digest"`
	BindingSetSemanticDigest    core.Digest                           `json:"binding_set_semantic_digest"`
	BindingSetCurrentnessDigest core.Digest                           `json:"binding_set_currentness_digest"`
	ActiveRouteID               string                                `json:"active_route_id"`
	ActiveRouteRevision         core.Revision                         `json:"active_route_revision"`
	ActiveRouteDigest           core.Digest                           `json:"active_route_digest"`
	ProviderTransport           ProviderBindingRefV2                  `json:"provider_transport_binding"`
	Provider                    ProviderBindingRefV2                  `json:"provider_binding"`
	CheckedUnixNano             int64                                 `json:"checked_unix_nano"`
	ExpiresUnixNano             int64                                 `json:"expires_unix_nano"`
	ProjectionDigest            core.Digest                           `json:"projection_digest"`
}

func (p ControlledMCPConnectRouteCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ControlledMCPConnectRouteCurrentContractVersionV1 || p.Ref.Validate() != nil || p.Generation.Validate() != nil || p.Assembly.Validate() != nil || validateEvidenceIDV2(p.HandoffID) != nil || p.HandoffRevision == 0 || p.HandoffDigest.Validate() != nil || validateEvidenceIDV2(p.BindingSetID) != nil || p.BindingSetRevision == 0 || p.BindingSetDigest.Validate() != nil || p.BindingSetSemanticDigest.Validate() != nil || p.BindingSetCurrentnessDigest.Validate() != nil || validateEvidenceIDV2(p.ActiveRouteID) != nil || p.ActiveRouteRevision == 0 || p.ActiveRouteDigest.Validate() != nil || p.ProviderTransport.Validate() != nil || p.Provider.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingDrift, "controlled MCP Connect route projection is incomplete")
	}
	if p.ProviderTransport == p.Provider || p.ProviderTransport.BindingSetID != p.BindingSetID || p.Provider.BindingSetID != p.BindingSetID || p.ProviderTransport.BindingSetRevision != p.BindingSetRevision || p.Provider.BindingSetRevision != p.BindingSetRevision || p.ProviderTransport.Capability != ControlledMCPConnectProviderTransportCapabilityV1 || p.Provider.Capability != CapabilityNameV2(OperationScopeEvidenceMCPConnectEffectKindV1) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Connect route bindings drifted")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled MCP Connect route projection digest drifted")
	}
	return nil
}

func (p ControlledMCPConnectRouteCurrentProjectionV1) ValidateCurrent(expected ControlledMCPConnectRouteCurrentRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Ref != expected || now.IsZero() || now.Before(time.Unix(0, p.CheckedUnixNano)) || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled MCP Connect route is not current")
	}
	return nil
}

func (p ControlledMCPConnectRouteCurrentProjectionV1) DigestV1() (core.Digest, error) {
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-mcp-connect-route", ControlledMCPConnectRouteCurrentContractVersionV1, "ControlledMCPConnectRouteCurrentProjectionV1", p)
}

func SealControlledMCPConnectRouteCurrentProjectionV1(p ControlledMCPConnectRouteCurrentProjectionV1) (ControlledMCPConnectRouteCurrentProjectionV1, error) {
	p.ContractVersion = ControlledMCPConnectRouteCurrentContractVersionV1
	matrixDigest, err := DigestOperationScopeEvidenceMCPConnectMatrixV1()
	if err != nil {
		return ControlledMCPConnectRouteCurrentProjectionV1{}, err
	}
	p.Ref.MatrixDigest = matrixDigest
	p.Ref.CurrentID, err = DeriveControlledMCPConnectRouteCurrentIDV1(p.Ref.DeclarationRef.RouteID, matrixDigest)
	if err != nil {
		return ControlledMCPConnectRouteCurrentProjectionV1{}, err
	}
	p.Ref.Digest = ""
	p.Ref.Digest, err = p.Ref.DigestV1()
	if err != nil {
		return ControlledMCPConnectRouteCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = ""
	p.ProjectionDigest, err = p.DigestV1()
	if err != nil {
		return ControlledMCPConnectRouteCurrentProjectionV1{}, err
	}
	return p, p.Validate()
}

func DigestOperationScopeEvidenceMCPConnectMatrixV1() (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceApplicabilityMatrixKeyV3", OperationScopeEvidenceMCPConnectMatrixV1())
}

func DeriveControlledMCPConnectRouteCurrentIDV1(routeID string, matrixDigest core.Digest) (string, error) {
	if validateEvidenceIDV2(routeID) != nil || matrixDigest.Validate() != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled MCP Connect route identity input is invalid")
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.controlled-mcp-connect-route", ControlledMCPConnectRouteCurrentContractVersionV1, "ControlledMCPConnectRouteCurrentIdentityV1", struct {
		RouteID      string      `json:"route_id"`
		MatrixDigest core.Digest `json:"matrix_digest"`
	}{routeID, matrixDigest})
	if err != nil {
		return "", err
	}
	return "controlled-mcp-connect-route-" + trimDigestPrefixV3(digest), nil
}

type ControlledMCPConnectRouteCurrentReaderV1 interface {
	InspectCurrentControlledMCPConnectRouteV1(context.Context, ControlledMCPConnectRouteCurrentRefV1) (ControlledMCPConnectRouteCurrentProjectionV1, error)
}

type OperationScopeEvidenceConsumptionClosureReaderV1 interface {
	InspectOperationScopeEvidenceConsumptionClosureV1(context.Context, OperationScopeEvidenceConsumptionRefV3) (OperationScopeEvidenceConsumptionFactV3, OperationScopeEvidenceQualificationFactV3, OperationScopeEvidenceProviderHandoffFactV3, error)
}

type OperationScopeEvidenceProviderHandoffClosureReaderV1 interface {
	InspectOperationScopeEvidenceProviderHandoffClosureV1(context.Context, OperationScopeEvidenceProviderHandoffRefV3) (OperationScopeEvidenceProviderHandoffFactV3, OperationScopeEvidenceQualificationFactV3, error)
}

type ControlledMCPConnectPhysicalAuthorizationRequestV1 struct {
	Route                  ControlledMCPConnectRouteCurrentRefV1      `json:"route"`
	Execute                ExecutePreparedRequestV2                   `json:"execute"`
	Attempt                OperationDispatchAttemptRefV3              `json:"attempt"`
	ExecuteEnforcement     OperationDispatchEnforcementPhaseRefV4     `json:"execute_enforcement"`
	PrepareConsumption     OperationScopeEvidenceConsumptionRefV3     `json:"prepare_consumption"`
	ExecuteHandoff         OperationScopeEvidenceProviderHandoffRefV3 `json:"execute_handoff"`
	Association            PreparedDomainCommandAssociationRefV1      `json:"association"`
	DomainCommand          OperationDomainCommandRefV1                `json:"domain_command"`
	CallerDeadlineUnixNano int64                                      `json:"caller_deadline_unix_nano"`
}

func (r ControlledMCPConnectPhysicalAuthorizationRequestV1) Validate() error {
	if r.Route.Validate() != nil || r.Execute.Validate() != nil || r.Attempt.Validate() != nil || r.ExecuteEnforcement.Validate() != nil || r.PrepareConsumption.Validate() != nil || r.ExecuteHandoff.Validate() != nil || r.Association.Validate() != nil || r.DomainCommand.Validate() != nil || r.CallerDeadlineUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "controlled MCP Connect physical authorization request is incomplete")
	}
	intentDigest, err := r.Execute.Intent.DigestV3()
	if err != nil || r.Execute.Intent.Kind != OperationScopeEvidenceMCPConnectEffectKindV1 || r.Execute.Intent.Operation.Kind != OperationScopeRunV3 || r.Attempt.OperationDigest != r.Execute.Prepared.OperationDigest || r.Attempt.EffectID != r.Execute.Intent.ID || r.Attempt.IntentRevision != r.Execute.Intent.Revision || r.Attempt.IntentDigest != intentDigest || r.Attempt.AttemptID != r.Execute.Prepared.AttemptID || r.ExecuteEnforcement.OperationDigest != r.Attempt.OperationDigest || r.ExecuteEnforcement.EffectID != r.Attempt.EffectID || r.ExecuteEnforcement.AttemptID != r.Attempt.AttemptID || r.ExecuteEnforcement.Phase != OperationDispatchEnforcementExecuteV4 {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Connect physical request coordinates drifted")
	}
	return nil
}

type ControlledMCPConnectPhysicalAuthorizationV1 struct {
	ContractVersion         string                                     `json:"contract_version"`
	StableKeyDigest         core.Digest                                `json:"stable_key_digest"`
	UnifiedNotAfterUnixNano int64                                      `json:"unified_not_after_unix_nano"`
	Route                   ControlledMCPConnectRouteCurrentRefV1      `json:"route"`
	ProviderTransport       ProviderBindingRefV2                       `json:"provider_transport_binding"`
	Provider                ProviderBindingRefV2                       `json:"provider_binding"`
	Operation               OperationSubjectV3                         `json:"operation"`
	OperationDigest         core.Digest                                `json:"operation_digest"`
	OperationScopeDigest    core.Digest                                `json:"operation_scope_digest"`
	EffectID                core.EffectIntentID                        `json:"effect_id"`
	EffectRevision          core.Revision                              `json:"effect_revision"`
	EffectFactRevision      core.Revision                              `json:"effect_fact_revision"`
	IntentDigest            core.Digest                                `json:"intent_digest"`
	Prepared                PreparedProviderAttemptRefV2               `json:"prepared"`
	Attempt                 OperationDispatchAttemptRefV3              `json:"attempt"`
	ExecuteEnforcement      OperationDispatchEnforcementPhaseRefV4     `json:"execute_enforcement"`
	PrepareConsumption      OperationScopeEvidenceConsumptionRefV3     `json:"prepare_consumption"`
	ExecuteHandoff          OperationScopeEvidenceProviderHandoffRefV3 `json:"execute_handoff"`
	SandboxProjectionDigest core.Digest                                `json:"sandbox_projection_digest"`
	CredentialFactsDigest   core.Digest                                `json:"credential_facts_digest"`
	Association             PreparedDomainCommandAssociationRefV1      `json:"association"`
	DomainCommand           OperationDomainCommandRefV1                `json:"domain_command"`
	IssuedUnixNano          int64                                      `json:"issued_unix_nano"`
	Digest                  core.Digest                                `json:"digest"`
}

func (a ControlledMCPConnectPhysicalAuthorizationV1) Validate() error {
	if a.ContractVersion != ControlledMCPConnectContractVersionV1 || a.StableKeyDigest.Validate() != nil || a.UnifiedNotAfterUnixNano <= 0 || a.Route.Validate() != nil || a.ProviderTransport.Validate() != nil || a.Provider.Validate() != nil || a.Operation.Validate() != nil || a.OperationDigest.Validate() != nil || a.OperationScopeDigest.Validate() != nil || validateEvidenceIDV2(string(a.EffectID)) != nil || a.EffectRevision == 0 || a.EffectFactRevision == 0 || a.IntentDigest.Validate() != nil || a.Prepared.Validate() != nil || a.Attempt.Validate() != nil || a.ExecuteEnforcement.Validate() != nil || a.PrepareConsumption.Validate() != nil || a.ExecuteHandoff.Validate() != nil || a.SandboxProjectionDigest.Validate() != nil || a.CredentialFactsDigest.Validate() != nil || a.Association.Validate() != nil || a.DomainCommand.Validate() != nil || a.IssuedUnixNano <= 0 || a.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "controlled MCP Connect physical authorization is incomplete")
	}
	operationDigest, err := a.Operation.DigestV3()
	if err != nil || a.Operation.Kind != OperationScopeRunV3 || operationDigest != a.OperationDigest || a.Operation.ExecutionScopeDigest != a.OperationScopeDigest || a.Attempt.OperationDigest != a.OperationDigest || a.Attempt.EffectID != a.EffectID || a.Attempt.IntentRevision != a.EffectRevision || a.Attempt.IntentDigest != a.IntentDigest || a.Prepared.AttemptID != a.Attempt.AttemptID || a.ExecuteEnforcement.AttemptID != a.Attempt.AttemptID || a.ExecuteEnforcement.Phase != OperationDispatchEnforcementExecuteV4 || a.DomainCommand.Owner.Role != OwnerSettlement || a.DomainCommand.Owner.ComponentID != a.Provider.ComponentID || a.DomainCommand.Owner.ManifestDigest != a.Provider.ManifestDigest || a.ProviderTransport == a.Provider || a.ProviderTransport.Capability != ControlledMCPConnectProviderTransportCapabilityV1 || a.Provider.Capability != CapabilityNameV2(OperationScopeEvidenceMCPConnectEffectKindV1) || a.IssuedUnixNano >= a.UnifiedNotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Connect physical authorization bindings drifted")
	}
	stable, err := DigestControlledMCPConnectPhysicalStableKeyV1(a)
	if err != nil || stable != a.StableKeyDigest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "controlled MCP Connect physical stable key drifted")
	}
	digest, err := a.DigestV1()
	if err != nil || digest != a.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "controlled MCP Connect physical authorization digest drifted")
	}
	return nil
}

func (a ControlledMCPConnectPhysicalAuthorizationV1) ValidateCurrent(now time.Time) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.Before(time.Unix(0, a.IssuedUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled MCP Connect authorization clock regressed")
	}
	if !now.Before(time.Unix(0, a.UnifiedNotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled MCP Connect authorization expired")
	}
	return nil
}

func (a ControlledMCPConnectPhysicalAuthorizationV1) DigestV1() (core.Digest, error) {
	a.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-mcp-connect", ControlledMCPConnectContractVersionV1, "ControlledMCPConnectPhysicalAuthorizationV1", a)
}

func DigestControlledMCPConnectPhysicalStableKeyV1(a ControlledMCPConnectPhysicalAuthorizationV1) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.runtime.controlled-mcp-connect", ControlledMCPConnectContractVersionV1, "ControlledMCPConnectPhysicalStableKeyV1", struct {
		OperationDigest core.Digest                           `json:"operation_digest"`
		Attempt         OperationDispatchAttemptRefV3         `json:"attempt"`
		DomainCommand   OperationDomainCommandRefV1           `json:"domain_command"`
		Route           ControlledMCPConnectRouteCurrentRefV1 `json:"route"`
	}{a.OperationDigest, a.Attempt, a.DomainCommand, a.Route})
}

func SealControlledMCPConnectPhysicalAuthorizationV1(a ControlledMCPConnectPhysicalAuthorizationV1) (ControlledMCPConnectPhysicalAuthorizationV1, error) {
	a.ContractVersion = ControlledMCPConnectContractVersionV1
	a.StableKeyDigest = ""
	stable, err := DigestControlledMCPConnectPhysicalStableKeyV1(a)
	if err != nil {
		return ControlledMCPConnectPhysicalAuthorizationV1{}, err
	}
	a.StableKeyDigest = stable
	a.Digest = ""
	a.Digest, err = a.DigestV1()
	if err != nil {
		return ControlledMCPConnectPhysicalAuthorizationV1{}, err
	}
	return a, a.Validate()
}

type ControlledMCPConnectPhysicalAuthorizationPortV1 interface {
	AuthorizeControlledMCPConnectPhysicalV1(context.Context, ControlledMCPConnectPhysicalAuthorizationRequestV1) (ControlledMCPConnectPhysicalAuthorizationV1, error)
}

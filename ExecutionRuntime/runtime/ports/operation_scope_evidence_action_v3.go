package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	OperationScopeEvidenceActionEffectKindV3    EffectKindV2     = "praxis.tool/execute"
	OperationScopeEvidenceActionPolicyProfileV3 NamespacedNameV2 = "praxis.tool/single-call-action-v1"

	OperationProviderBoundaryContractVersionV1 = "1.0.0"
	MaxOperationProviderBoundaryTTLV1          = MaxDispatchPermitTTL
)

const (
	OperationScopeEvidenceRunCurrentKindV3      NamespacedNameV2 = "praxis.runtime/run-current-v3"
	OperationScopeEvidenceSessionCurrentKindV3  NamespacedNameV2 = "praxis.harness/session"
	OperationScopeEvidenceTurnCurrentKindV3     NamespacedNameV2 = "praxis.harness/turn"
	OperationScopeEvidenceActionCandidateKindV3 NamespacedNameV2 = "praxis.tool/action-candidate-v2"
	OperationScopeEvidenceContextParentKindV3   NamespacedNameV2 = "praxis.context/parent-frame-current-v1"
	OperationScopeEvidenceRunOwnerVersionV3                      = "3.0.0"
	OperationScopeEvidenceSessionOwnerVersionV3                  = "1.0.0"
	OperationScopeEvidenceTurnOwnerVersionV3                     = "1.0.0"
	OperationScopeEvidenceActionOwnerVersionV3                   = "2.0.0"
	OperationScopeEvidenceContextOwnerVersionV3                  = "1.0.0"
)

// OperationScopeEvidenceActionMatrixV3 returns the only G6A Action row. A
// fresh value prevents callers from mutating a package-level catalog entry.
func OperationScopeEvidenceActionMatrixV3() OperationScopeEvidenceApplicabilityMatrixKeyV3 {
	return OperationScopeEvidenceApplicabilityMatrixKeyV3{
		OperationKind: OperationScopeRunV3,
		EffectKind:    OperationScopeEvidenceActionEffectKindV3,
		PolicyProfile: OperationScopeEvidenceActionPolicyProfileV3,
	}
}

func IsOperationScopeEvidenceActionMatrixKeyV3(key OperationScopeEvidenceApplicabilityMatrixKeyV3) bool {
	return key == OperationScopeEvidenceActionMatrixV3()
}

func isRegisteredOperationScopeEvidencePolicySubjectV3(operation OperationScopeKindV3, effect EffectKindV2) bool {
	if operation == OperationScopeActivationV3 {
		return effect == "praxis.sandbox/allocate" || effect == "praxis.sandbox/activate" || effect == "praxis.sandbox/open" || effect == "praxis.sandbox/inspect"
	}
	if operation == OperationScopeRunV3 {
		return effect == OperationScopeEvidenceActionEffectKindV3 || effect == OperationScopeEvidenceMCPConnectEffectKindV1 || effect == "praxis.sandbox/cancel" || effect == "praxis.sandbox/workspace-commit" || effect == "praxis.sandbox/inspect"
	}
	if operation == OperationScopeTerminationV3 {
		return effect == "praxis.sandbox/close" || effect == "praxis.sandbox/fence" || effect == "praxis.sandbox/release" || effect == "praxis.sandbox/cleanup" || effect == "praxis.sandbox/inspect"
	}
	return operation == OperationScopeAdminV3 && (effect == "praxis.sandbox/fence" || effect == "praxis.sandbox/cleanup" || effect == "praxis.sandbox/inspect")
}

type OperationScopeEvidenceActionApplicabilityRouteV3 struct {
	Dimension            OperationScopeEvidenceApplicabilityDimensionV3 `json:"dimension"`
	Kind                 NamespacedNameV2                               `json:"kind"`
	OwnerContractVersion string                                         `json:"owner_contract_version"`
}

func (r OperationScopeEvidenceActionApplicabilityRouteV3) Validate() error {
	expected, ok := operationScopeEvidenceActionRouteV3(r.Dimension)
	if !ok || r != expected {
		return core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "Action Evidence applicability route is not registered")
	}
	version, err := core.ParseSemanticVersion(r.OwnerContractVersion)
	if err != nil || version.String() != r.OwnerContractVersion {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidSemanticVersion, "Action Evidence owner contract version is not canonical SemVer")
	}
	return nil
}

func OperationScopeEvidenceActionRoutesV3() []OperationScopeEvidenceActionApplicabilityRouteV3 {
	return []OperationScopeEvidenceActionApplicabilityRouteV3{
		{Dimension: OperationScopeEvidenceRunV3, Kind: OperationScopeEvidenceRunCurrentKindV3, OwnerContractVersion: OperationScopeEvidenceRunOwnerVersionV3},
		{Dimension: OperationScopeEvidenceSessionV3, Kind: OperationScopeEvidenceSessionCurrentKindV3, OwnerContractVersion: OperationScopeEvidenceSessionOwnerVersionV3},
		{Dimension: OperationScopeEvidenceTurnV3, Kind: OperationScopeEvidenceTurnCurrentKindV3, OwnerContractVersion: OperationScopeEvidenceTurnOwnerVersionV3},
		{Dimension: OperationScopeEvidenceActionV3, Kind: OperationScopeEvidenceActionCandidateKindV3, OwnerContractVersion: OperationScopeEvidenceActionOwnerVersionV3},
		{Dimension: OperationScopeEvidenceContextV3, Kind: OperationScopeEvidenceContextParentKindV3, OwnerContractVersion: OperationScopeEvidenceContextOwnerVersionV3},
	}
}

func operationScopeEvidenceActionRouteV3(dimension OperationScopeEvidenceApplicabilityDimensionV3) (OperationScopeEvidenceActionApplicabilityRouteV3, bool) {
	for _, route := range OperationScopeEvidenceActionRoutesV3() {
		if route.Dimension == dimension {
			return route, true
		}
	}
	return OperationScopeEvidenceActionApplicabilityRouteV3{}, false
}

// OperationScopeEvidenceActionApplicabilitySourceV3 is a nominal coordinate
// supplied by an Owner adapter. Runtime projects the four fact fields without
// generating a new identity, revision or digest.
type OperationScopeEvidenceActionApplicabilitySourceV3 struct {
	Route    OperationScopeEvidenceActionApplicabilityRouteV3 `json:"route"`
	ID       string                                           `json:"id"`
	Revision core.Revision                                    `json:"revision"`
	Digest   core.Digest                                      `json:"digest"`
}

func (s OperationScopeEvidenceActionApplicabilitySourceV3) Validate() error {
	if err := s.Route.Validate(); err != nil {
		return err
	}
	return (OperationScopeEvidenceApplicabilityFactRefV3{Kind: s.Route.Kind, ID: s.ID, Revision: s.Revision, Digest: s.Digest}).Validate()
}

func ProjectOperationScopeEvidenceActionApplicabilityRefV3(source OperationScopeEvidenceActionApplicabilitySourceV3) (OperationScopeEvidenceApplicabilityFactRefV3, error) {
	if err := source.Validate(); err != nil {
		return OperationScopeEvidenceApplicabilityFactRefV3{}, err
	}
	return OperationScopeEvidenceApplicabilityFactRefV3{Kind: source.Route.Kind, ID: source.ID, Revision: source.Revision, Digest: source.Digest}, nil
}

func ValidateOperationScopeEvidenceActionApplicabilityV3(values []OperationScopeEvidenceApplicabilityV3) error {
	if err := ValidateOperationScopeEvidenceApplicabilitySetV3(values); err != nil {
		return err
	}
	normalized := NormalizeOperationScopeEvidenceApplicabilityV3(values)
	for _, value := range normalized {
		route, ok := operationScopeEvidenceActionRouteV3(value.Dimension)
		if !ok || value.Mode != OperationScopeEvidenceRequiredV3 || value.Fact == nil || value.Fact.Kind != route.Kind {
			return core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "Action Evidence requires the exact Run, Session, Turn, Action and Context Owner sources")
		}
	}
	return nil
}

// OperationScopeEvidenceActionApplicabilityCurrentRouterV3 is the closed,
// injected Router used by the Evidence gateway. It owns no Fact or cache.
type OperationScopeEvidenceActionApplicabilityCurrentRouterV3 interface {
	InspectOperationScopeEvidenceActionApplicabilityCurrentV3(context.Context, OperationScopeEvidenceApplicabilityDimensionV3, OperationScopeEvidenceApplicabilityFactRefV3, core.Digest) (OperationScopeEvidenceApplicabilityCurrentProjectionV3, error)
}

type OperationProviderBoundaryStageV1 string

const OperationProviderBoundaryCrossedV1 OperationProviderBoundaryStageV1 = "provider_boundary_crossed"

type OperationProviderBoundaryRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r OperationProviderBoundaryRefV1) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "provider boundary ref is incomplete")
	}
	return r.Digest.Validate()
}

type OperationProviderBoundaryCurrentProjectionV1 struct {
	ContractVersion        string                                     `json:"contract_version"`
	Ref                    OperationProviderBoundaryRefV1             `json:"ref"`
	Operation              OperationSubjectV3                         `json:"operation"`
	OperationDigest        core.Digest                                `json:"operation_digest"`
	OperationScopeDigest   core.Digest                                `json:"operation_scope_digest"`
	Attempt                OperationDispatchAttemptRefV3              `json:"attempt"`
	ExecuteEnforcement     OperationDispatchEnforcementPhaseRefV4     `json:"execute_enforcement"`
	ExecuteEvidenceHandoff OperationScopeEvidenceProviderHandoffRefV3 `json:"execute_evidence_handoff"`
	Stage                  OperationProviderBoundaryStageV1           `json:"stage"`
	CheckedUnixNano        int64                                      `json:"checked_unix_nano"`
	ExpiresUnixNano        int64                                      `json:"expires_unix_nano"`
	Digest                 core.Digest                                `json:"digest"`
}

func (p OperationProviderBoundaryCurrentProjectionV1) Validate() error {
	if p.ContractVersion != OperationProviderBoundaryContractVersionV1 || p.Ref.Validate() != nil || p.Operation.Validate() != nil || p.OperationDigest.Validate() != nil || p.OperationScopeDigest.Validate() != nil || p.Attempt.Validate() != nil || p.ExecuteEnforcement.Validate() != nil || p.ExecuteEvidenceHandoff.Validate() != nil || p.Stage != OperationProviderBoundaryCrossedV1 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || time.Duration(p.ExpiresUnixNano-p.CheckedUnixNano) > MaxOperationProviderBoundaryTTLV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "provider boundary projection is incomplete")
	}
	operationDigest, err := p.Operation.DigestV3()
	if err != nil || operationDigest != p.OperationDigest || p.Attempt.OperationDigest != p.OperationDigest || p.ExecuteEnforcement.OperationDigest != p.OperationDigest || p.Attempt.EffectID != p.ExecuteEnforcement.EffectID || p.Attempt.PermitID != p.ExecuteEnforcement.PermitID || p.Attempt.PermitRevision != p.ExecuteEnforcement.PermitFactRevision || p.Attempt.PermitDigest != p.ExecuteEnforcement.PermitDigest || p.Attempt.AttemptID != p.ExecuteEnforcement.AttemptID || p.ExecuteEnforcement.Phase != OperationDispatchEnforcementExecuteV4 || p.ExpiresUnixNano > p.ExecuteEnforcement.ExpiresUnixNano || p.ExpiresUnixNano > OperationScopeEvidenceFactRefV3(p.ExecuteEvidenceHandoff).ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "provider boundary projection binds another operation attempt")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "provider boundary projection digest drifted")
	}
	return nil
}

func (p OperationProviderBoundaryCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-provider-boundary-current-projection", OperationProviderBoundaryContractVersionV1, "OperationProviderBoundaryCurrentProjectionV1", copy)
}

func SealOperationProviderBoundaryCurrentProjectionV1(p OperationProviderBoundaryCurrentProjectionV1) (OperationProviderBoundaryCurrentProjectionV1, error) {
	p.ContractVersion = OperationProviderBoundaryContractVersionV1
	p.Digest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return OperationProviderBoundaryCurrentProjectionV1{}, err
	}
	p.Digest = digest
	return p, p.Validate()
}

func (p OperationProviderBoundaryCurrentProjectionV1) ValidateCurrent(expected OperationProviderBoundaryRefV1, operation OperationSubjectV3, scopeDigest core.Digest, attempt OperationDispatchAttemptRefV3, enforcement OperationDispatchEnforcementPhaseRefV4, handoff OperationScopeEvidenceProviderHandoffRefV3, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() || p.Ref != expected || !SameOperationSubjectV3(p.Operation, operation) || p.OperationScopeDigest != scopeDigest || !sameOperationProviderAttemptRefV1(p.Attempt, attempt) || p.ExecuteEnforcement != enforcement || p.ExecuteEvidenceHandoff != handoff || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "provider boundary is stale or mismatched")
	}
	return nil
}

func sameOperationProviderAttemptRefV1(left, right OperationDispatchAttemptRefV3) bool {
	if left.Validate() != nil || right.Validate() != nil {
		return false
	}
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.operation-provider-boundary-ref", OperationProviderBoundaryContractVersionV1, "OperationDispatchAttemptRefV3", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.operation-provider-boundary-ref", OperationProviderBoundaryContractVersionV1, "OperationDispatchAttemptRefV3", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

type OperationProviderBoundaryCurrentReaderV1 interface {
	InspectCurrentOperationProviderBoundaryV1(context.Context, OperationProviderBoundaryRefV1) (OperationProviderBoundaryCurrentProjectionV1, error)
}

// OperationProviderExecuteEnforcementCurrentReaderV1 and
// OperationProviderEvidenceHandoffCurrentReaderV1 are narrow read-only seams.
// Their adapters must call the existing 4.1 and Evidence V3 current gateways;
// they do not grant mutation authority.
type OperationProviderExecuteEnforcementCurrentReaderV1 interface {
	InspectCurrentOperationProviderExecuteEnforcementV1(context.Context, OperationSubjectV3, OperationDispatchEnforcementPhaseRefV4) (OperationDispatchEnforcementPhaseRefV4, error)
}

type OperationProviderEvidenceHandoffCurrentReaderV1 interface {
	InspectCurrentOperationProviderEvidenceHandoffV1(context.Context, OperationScopeEvidenceProviderHandoffRefV3) (OperationScopeEvidenceProviderHandoffFactV3, error)
}

type ControlledOperationProviderCallRequestV1 struct {
	Operation              OperationSubjectV3                         `json:"operation"`
	OperationScopeDigest   core.Digest                                `json:"operation_scope_digest"`
	Attempt                OperationDispatchAttemptRefV3              `json:"attempt"`
	ExecuteEnforcement     OperationDispatchEnforcementPhaseRefV4     `json:"execute_enforcement"`
	ExecuteEvidenceHandoff OperationScopeEvidenceProviderHandoffRefV3 `json:"execute_evidence_handoff"`
	Boundary               OperationProviderBoundaryRefV1             `json:"boundary"`
}

func (r ControlledOperationProviderCallRequestV1) Validate() error {
	if r.Operation.Validate() != nil || r.OperationScopeDigest.Validate() != nil || r.Attempt.Validate() != nil || r.ExecuteEnforcement.Validate() != nil || r.ExecuteEvidenceHandoff.Validate() != nil || r.Boundary.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "controlled provider call is incomplete")
	}
	operationDigest, err := r.Operation.DigestV3()
	if err != nil || operationDigest != r.Attempt.OperationDigest || operationDigest != r.ExecuteEnforcement.OperationDigest || r.Attempt.EffectID != r.ExecuteEnforcement.EffectID || r.Attempt.PermitID != r.ExecuteEnforcement.PermitID || r.Attempt.PermitRevision != r.ExecuteEnforcement.PermitFactRevision || r.Attempt.PermitDigest != r.ExecuteEnforcement.PermitDigest || r.Attempt.AttemptID != r.ExecuteEnforcement.AttemptID || r.ExecuteEnforcement.Phase != OperationDispatchEnforcementExecuteV4 {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "controlled provider call binds another operation attempt")
	}
	return nil
}

// OperationProviderTestInvokerV1 is intentionally fixture-only. It is not a
// production Provider contract and cannot claim physical exactly-once.
type OperationProviderTestInvokerV1 interface {
	InvokeOperationProviderTestV1(context.Context, ControlledOperationProviderCallRequestV1) error
}

type ControlledOperationProviderPortV1 interface {
	CallControlledOperationProviderV1(context.Context, ControlledOperationProviderCallRequestV1) error
}

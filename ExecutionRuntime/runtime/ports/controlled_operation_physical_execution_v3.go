package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	OperationDomainCommandContractVersionV1               = "1.0.0"
	PreparedDomainCommandAssociationContractVersionV1     = "1.0.0"
	ControlledOperationPhysicalExecutionContractVersionV3 = "3.0.0"
)

// OperationDomainCommandRefV1 is a Runtime-neutral reference to an immutable
// domain-owned command. It carries no Provider authority or execution permit.
type OperationDomainCommandRefV1 struct {
	Owner    EffectOwnerRefV2 `json:"owner"`
	Kind     NamespacedNameV2 `json:"kind"`
	ID       string           `json:"id"`
	Revision core.Revision    `json:"revision"`
	Digest   core.Digest      `json:"digest"`
}

func (r OperationDomainCommandRefV1) Validate() error {
	if r.Owner.Role != OwnerSettlement || ValidateNamespacedNameV2(NamespacedNameV2(r.Owner.ComponentID)) != nil || r.Owner.ManifestDigest.Validate() != nil || ValidateNamespacedNameV2(r.Kind) != nil || validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "operation domain command ref is incomplete")
	}
	return nil
}

type PreparedDomainCommandAssociationRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r PreparedDomainCommandAssociationRefV1) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prepared domain command association ref is incomplete")
	}
	return nil
}

// PreparedDomainCommandAssociationCurrentProjectionV1 binds one exact Runtime
// Prepared/Attempt to one domain-owned command. Runtime does not interpret the
// command's business fields and the association does not authorize execution.
type PreparedDomainCommandAssociationCurrentProjectionV1 struct {
	ContractVersion  string                                `json:"contract_version"`
	Ref              PreparedDomainCommandAssociationRefV1 `json:"ref"`
	Operation        OperationSubjectV3                    `json:"operation"`
	OperationDigest  core.Digest                           `json:"operation_digest"`
	EffectID         core.EffectIntentID                   `json:"effect_id"`
	EffectRevision   core.Revision                         `json:"effect_revision"`
	IntentDigest     core.Digest                           `json:"intent_digest"`
	Prepared         PreparedProviderAttemptRefV2          `json:"prepared"`
	Attempt          OperationDispatchAttemptRefV3         `json:"attempt"`
	Provider         ProviderBindingRefV2                  `json:"provider"`
	PayloadSchema    SchemaRefV2                           `json:"payload_schema"`
	PayloadDigest    core.Digest                           `json:"payload_digest"`
	PayloadRevision  core.Revision                         `json:"payload_revision"`
	DomainCommand    OperationDomainCommandRefV1           `json:"domain_command"`
	CheckedUnixNano  int64                                 `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                 `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                           `json:"projection_digest"`
}

func (p PreparedDomainCommandAssociationCurrentProjectionV1) Validate() error {
	if p.ContractVersion != PreparedDomainCommandAssociationContractVersionV1 || p.Ref.Validate() != nil || p.Operation.Validate() != nil || p.OperationDigest.Validate() != nil || p.EffectID == "" || p.EffectRevision == 0 || p.IntentDigest.Validate() != nil || p.Prepared.Validate() != nil || p.Attempt.Validate() != nil || p.Provider.Validate() != nil || p.PayloadSchema.Validate() != nil || p.PayloadDigest.Validate() != nil || p.PayloadRevision == 0 || p.DomainCommand.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prepared domain command association is incomplete")
	}
	operationDigest, err := p.Operation.DigestV3()
	if err != nil || operationDigest != p.OperationDigest || p.Prepared.OperationDigest != p.OperationDigest || p.Attempt.OperationDigest != p.OperationDigest || p.Prepared.IntentID != p.EffectID || p.Attempt.EffectID != p.EffectID || p.Prepared.IntentRevision != p.EffectRevision || p.Attempt.IntentRevision != p.EffectRevision || p.Prepared.IntentDigest != p.IntentDigest || p.Attempt.IntentDigest != p.IntentDigest || p.Prepared.AttemptID != p.Attempt.AttemptID || p.Prepared.Provider != p.Provider || p.Prepared.PayloadSchema != p.PayloadSchema || p.Prepared.PayloadDigest != p.PayloadDigest || p.Prepared.PayloadRevision != p.PayloadRevision {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "prepared domain command association binds another operation, attempt, provider or payload")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.Ref.Digest || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "prepared domain command association digest drifted")
	}
	id, err := DerivePreparedDomainCommandAssociationIDV1(p.Prepared, p.Attempt, p.DomainCommand)
	if err != nil || id != p.Ref.ID {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "prepared domain command association ID drifted")
	}
	return nil
}

func (p PreparedDomainCommandAssociationCurrentProjectionV1) ValidateCurrent(expected PreparedDomainCommandAssociationRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Ref != expected {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "prepared domain command association ref drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "prepared domain command association clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "prepared domain command association expired")
	}
	return nil
}

func (p PreparedDomainCommandAssociationCurrentProjectionV1) DigestV1() (core.Digest, error) {
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.prepared-domain-command-association", PreparedDomainCommandAssociationContractVersionV1, "PreparedDomainCommandAssociationCurrentProjectionV1", p)
}

func SealPreparedDomainCommandAssociationCurrentProjectionV1(p PreparedDomainCommandAssociationCurrentProjectionV1) (PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	p.ContractVersion = PreparedDomainCommandAssociationContractVersionV1
	id, err := DerivePreparedDomainCommandAssociationIDV1(p.Prepared, p.Attempt, p.DomainCommand)
	if err != nil {
		return PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	if p.Ref.ID != "" && p.Ref.ID != id {
		return PreparedDomainCommandAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "supplied prepared domain command association ID drifted")
	}
	p.Ref.ID, p.Ref.Revision = id, 1
	providedRef, providedProjection := p.Ref.Digest, p.ProjectionDigest
	p.Ref.Digest, p.ProjectionDigest = "", ""
	digest, err := p.DigestV1()
	if err != nil {
		return PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	if providedRef != "" && providedRef != digest || providedProjection != "" && providedProjection != digest {
		return PreparedDomainCommandAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "supplied prepared domain command association digest drifted")
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p, p.Validate()
}

func DerivePreparedDomainCommandAssociationIDV1(prepared PreparedProviderAttemptRefV2, attempt OperationDispatchAttemptRefV3, command OperationDomainCommandRefV1) (string, error) {
	if prepared.Validate() != nil || attempt.Validate() != nil || command.Validate() != nil || prepared.AttemptID != attempt.AttemptID {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prepared domain command association identity inputs are invalid")
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.prepared-domain-command-association", PreparedDomainCommandAssociationContractVersionV1, "PreparedDomainCommandAssociationIdentityV1", struct {
		Prepared PreparedProviderAttemptRefV2  `json:"prepared"`
		Attempt  OperationDispatchAttemptRefV3 `json:"attempt"`
		Command  OperationDomainCommandRefV1   `json:"command"`
	}{prepared, attempt, command})
	if err != nil {
		return "", err
	}
	return "prepared-domain-command-" + trimDigestPrefixV3(digest), nil
}

type PreparedDomainCommandAssociationCurrentReaderV1 interface {
	InspectCurrentPreparedDomainCommandAssociationV1(context.Context, PreparedDomainCommandAssociationRefV1) (PreparedDomainCommandAssociationCurrentProjectionV1, error)
}

// EnsurePreparedDomainCommandAssociationRequestV1 carries only exact,
// immutable coordinates. Runtime supplies Checked/Expires and the sealed Ref;
// callers cannot self-sign an association projection.
type EnsurePreparedDomainCommandAssociationRequestV1 struct {
	Operation             OperationSubjectV3            `json:"operation"`
	OperationDigest       core.Digest                   `json:"operation_digest"`
	EffectID              core.EffectIntentID           `json:"effect_id"`
	IntentRevision        core.Revision                 `json:"intent_revision"`
	IntentDigest          core.Digest                   `json:"intent_digest"`
	Prepared              PreparedProviderAttemptRefV2  `json:"prepared"`
	Attempt               OperationDispatchAttemptRefV3 `json:"attempt"`
	Provider              ProviderBindingRefV2          `json:"provider"`
	PayloadSchema         SchemaRefV2                   `json:"payload_schema"`
	PayloadDigest         core.Digest                   `json:"payload_digest"`
	PayloadRevision       core.Revision                 `json:"payload_revision"`
	DomainCommand         OperationDomainCommandRefV1   `json:"domain_command"`
	RequestedNotAfterNano int64                         `json:"requested_not_after_unix_nano"`
}

func (r EnsurePreparedDomainCommandAssociationRequestV1) Validate() error {
	if r.Operation.Validate() != nil || r.OperationDigest.Validate() != nil || r.EffectID == "" || r.IntentRevision == 0 || r.IntentDigest.Validate() != nil || r.Prepared.Validate() != nil || r.Attempt.Validate() != nil || r.Provider.Validate() != nil || r.PayloadSchema.Validate() != nil || r.PayloadDigest.Validate() != nil || r.PayloadRevision == 0 || r.DomainCommand.Validate() != nil || r.RequestedNotAfterNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prepared domain command association request is incomplete")
	}
	operationDigest, err := r.Operation.DigestV3()
	if err != nil || operationDigest != r.OperationDigest || r.Prepared.OperationDigest != r.OperationDigest || r.Attempt.OperationDigest != r.OperationDigest || r.Prepared.IntentID != r.EffectID || r.Attempt.EffectID != r.EffectID || r.Prepared.IntentRevision != r.IntentRevision || r.Attempt.IntentRevision != r.IntentRevision || r.Prepared.IntentDigest != r.IntentDigest || r.Attempt.IntentDigest != r.IntentDigest || r.Prepared.AttemptID != r.Attempt.AttemptID || r.Prepared.Provider != r.Provider || r.Prepared.PayloadSchema != r.PayloadSchema || r.Prepared.PayloadDigest != r.PayloadDigest || r.Prepared.PayloadRevision != r.PayloadRevision {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "prepared domain command association request binds another operation, attempt, Provider or payload")
	}
	if r.DomainCommand.Owner.Role != OwnerSettlement || r.DomainCommand.Owner.ComponentID != r.Provider.ComponentID || r.DomainCommand.Owner.ManifestDigest != r.Provider.ManifestDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "prepared domain command association command Owner differs from the Provider")
	}
	return nil
}

type PreparedDomainCommandAssociationStoreV1 interface {
	CreatePreparedDomainCommandAssociationV1(context.Context, PreparedDomainCommandAssociationCurrentProjectionV1) (PreparedDomainCommandAssociationCurrentProjectionV1, error)
	InspectPreparedDomainCommandAssociationByIDV1(context.Context, string) (PreparedDomainCommandAssociationCurrentProjectionV1, error)
	InspectPreparedDomainCommandAssociationV1(context.Context, PreparedDomainCommandAssociationRefV1) (PreparedDomainCommandAssociationCurrentProjectionV1, error)
}

type PreparedDomainCommandAssociationPortV1 interface {
	EnsurePreparedDomainCommandAssociationV1(context.Context, EnsurePreparedDomainCommandAssociationRequestV1) (PreparedDomainCommandAssociationCurrentProjectionV1, error)
	PreparedDomainCommandAssociationCurrentReaderV1
}

// ControlledOperationPhysicalAuthorizationRequestV3 asks Runtime to re-read
// its full V2 current closure and bind it to one exact domain command. It does
// not itself authorize or execute a physical effect.
type ControlledOperationPhysicalAuthorizationRequestV3 struct {
	Provider      ControlledOperationProviderRequestV2  `json:"provider"`
	Association   PreparedDomainCommandAssociationRefV1 `json:"association"`
	DomainCommand OperationDomainCommandRefV1           `json:"domain_command"`
}

func (r ControlledOperationPhysicalAuthorizationRequestV3) Validate() error {
	if err := r.Provider.Validate(); err != nil {
		return err
	}
	if r.Association.Validate() != nil || r.DomainCommand.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "physical execution authorization request exact refs are incomplete")
	}
	if r.DomainCommand.Owner.Role != OwnerSettlement || r.DomainCommand.Owner.ComponentID != r.Provider.ProviderBinding.ComponentID || r.DomainCommand.Owner.ManifestDigest != r.Provider.ProviderBinding.ManifestDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "physical execution domain command Owner differs from the Provider")
	}
	return nil
}

type ControlledOperationPhysicalAuthorizationPortV3 interface {
	AuthorizeControlledOperationPhysicalV3(context.Context, ControlledOperationPhysicalAuthorizationRequestV3) (ControlledOperationPhysicalExecutionAuthorizationV3, error)
}

// ControlledOperationPhysicalExecutionAuthorizationV3 is the complete,
// Runtime-issued authorization handed to the component that owns the physical
// effect entry. The receiver must still use a fresh local clock and exact
// domain-command current reader immediately before its physical write.
type ControlledOperationPhysicalExecutionAuthorizationV3 struct {
	ContractVersion         string                                     `json:"contract_version"`
	StableKeyDigest         core.Digest                                `json:"stable_key_digest"`
	UnifiedNotAfterUnixNano int64                                      `json:"unified_not_after_unix_nano"`
	ProviderTransport       ProviderBindingRefV2                       `json:"provider_transport"`
	Provider                ProviderBindingRefV2                       `json:"provider"`
	Operation               OperationSubjectV3                         `json:"operation"`
	OperationDigest         core.Digest                                `json:"operation_digest"`
	OperationScopeDigest    core.Digest                                `json:"operation_scope_digest"`
	EffectKind              EffectKindV2                               `json:"effect_kind"`
	Prepared                PreparedProviderAttemptRefV2               `json:"prepared"`
	Attempt                 OperationDispatchAttemptRefV3              `json:"attempt"`
	ExecuteEnforcement      OperationDispatchEnforcementPhaseRefV4     `json:"execute_enforcement"`
	ExecuteEvidenceHandoff  OperationScopeEvidenceProviderHandoffRefV3 `json:"execute_evidence_handoff"`
	Boundary                OperationProviderBoundaryRefV1             `json:"boundary"`
	Association             PreparedDomainCommandAssociationRefV1      `json:"association"`
	DomainCommand           OperationDomainCommandRefV1                `json:"domain_command"`
	AuthorizationDigest     core.Digest                                `json:"authorization_digest"`
}

func (a ControlledOperationPhysicalExecutionAuthorizationV3) Validate() error {
	if a.ContractVersion != ControlledOperationPhysicalExecutionContractVersionV3 || a.StableKeyDigest.Validate() != nil || a.UnifiedNotAfterUnixNano <= 0 || a.ProviderTransport.Validate() != nil || a.Provider.Validate() != nil || a.Operation.Validate() != nil || a.OperationDigest.Validate() != nil || a.OperationScopeDigest.Validate() != nil || ValidateNamespacedNameV2(NamespacedNameV2(a.EffectKind)) != nil || a.Prepared.Validate() != nil || a.Attempt.Validate() != nil || a.ExecuteEnforcement.Validate() != nil || a.ExecuteEvidenceHandoff.Validate() != nil || a.Boundary.Validate() != nil || a.Association.Validate() != nil || a.DomainCommand.Validate() != nil || a.AuthorizationDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "physical execution authorization is incomplete")
	}
	operationDigest, err := a.Operation.DigestV3()
	if err != nil || operationDigest != a.OperationDigest || a.Operation.ExecutionScopeDigest != a.OperationScopeDigest || a.EffectKind != OperationScopeEvidenceActionEffectKindV3 || a.Provider.Capability != CapabilityNameV2(a.EffectKind) || a.ProviderTransport.Capability != ControlledOperationProviderTransportCapabilityV2 || a.Provider == a.ProviderTransport || a.Prepared.OperationDigest != a.OperationDigest || a.Attempt.OperationDigest != a.OperationDigest || a.Prepared.AttemptID != a.Attempt.AttemptID || a.Prepared.Provider != a.Provider || a.ExecuteEnforcement.OperationDigest != a.OperationDigest || a.ExecuteEnforcement.EffectID != a.Attempt.EffectID || a.ExecuteEnforcement.AttemptID != a.Attempt.AttemptID || a.ExecuteEnforcement.Phase != OperationDispatchEnforcementExecuteV4 {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "physical execution authorization exact bindings drifted")
	}
	if a.DomainCommand.Owner.Role != OwnerSettlement || a.DomainCommand.Owner.ComponentID != a.Provider.ComponentID || a.DomainCommand.Owner.ManifestDigest != a.Provider.ManifestDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "physical execution domain command Owner differs from the Provider")
	}
	stable, err := a.StableKeyDigestV3()
	if err != nil || stable != a.StableKeyDigest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "physical execution stable key drifted")
	}
	digest, err := a.DigestV3()
	if err != nil || digest != a.AuthorizationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "physical execution authorization digest drifted")
	}
	return nil
}

func (a ControlledOperationPhysicalExecutionAuthorizationV3) ValidateCurrent(now time.Time) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if now.IsZero() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "physical execution clock is zero")
	}
	if !now.Before(time.Unix(0, a.UnifiedNotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "physical execution authorization expired")
	}
	return nil
}

func (a ControlledOperationPhysicalExecutionAuthorizationV3) StableKeyDigestV3() (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.runtime.controlled-operation-physical-execution", ControlledOperationPhysicalExecutionContractVersionV3, "ControlledOperationPhysicalExecutionStableKeyV3", struct {
		Operation     core.Digest                   `json:"operation"`
		Prepared      PreparedProviderAttemptRefV2  `json:"prepared"`
		Attempt       OperationDispatchAttemptRefV3 `json:"attempt"`
		Transport     ProviderBindingRefV2          `json:"transport"`
		Provider      ProviderBindingRefV2          `json:"provider"`
		DomainCommand OperationDomainCommandRefV1   `json:"domain_command"`
	}{a.OperationDigest, a.Prepared, a.Attempt, a.ProviderTransport, a.Provider, a.DomainCommand})
}

func (a ControlledOperationPhysicalExecutionAuthorizationV3) DigestV3() (core.Digest, error) {
	a.AuthorizationDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.controlled-operation-physical-execution", ControlledOperationPhysicalExecutionContractVersionV3, "ControlledOperationPhysicalExecutionAuthorizationV3", a)
}

func SealControlledOperationPhysicalExecutionAuthorizationV3(a ControlledOperationPhysicalExecutionAuthorizationV3) (ControlledOperationPhysicalExecutionAuthorizationV3, error) {
	a.ContractVersion = ControlledOperationPhysicalExecutionContractVersionV3
	stable, err := a.StableKeyDigestV3()
	if err != nil {
		return ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	if a.StableKeyDigest != "" && a.StableKeyDigest != stable {
		return ControlledOperationPhysicalExecutionAuthorizationV3{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "supplied physical execution stable key drifted")
	}
	a.StableKeyDigest = stable
	provided := a.AuthorizationDigest
	a.AuthorizationDigest = ""
	digest, err := a.DigestV3()
	if err != nil {
		return ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	if provided != "" && provided != digest {
		return ControlledOperationPhysicalExecutionAuthorizationV3{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "supplied physical execution authorization digest drifted")
	}
	a.AuthorizationDigest = digest
	return a, a.Validate()
}

type ControlledOperationPhysicalExecutionPortV3 interface {
	ExecuteControlledOperationPhysicalV3(context.Context, ControlledOperationPhysicalExecutionAuthorizationV3) (ControlledOperationProviderAdmissionReceiptRefV2, error)
}

func trimDigestPrefixV3(digest core.Digest) string {
	const prefix = "sha256:"
	value := string(digest)
	if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
		return value[len(prefix):]
	}
	return value
}

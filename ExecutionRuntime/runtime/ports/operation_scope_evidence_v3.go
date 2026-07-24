package ports

import (
	"context"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	OperationScopeEvidenceContractVersionV3 = "3.0.0"
	MaxOperationScopeEvidencePayloadBytesV3 = MaxEvidenceDeclaredBytes
	MaxOperationScopeEvidenceIngestGraceV3  = MaxDispatchPermitTTL
)

const (
	OperationScopeEvidenceActivationProfileV3           NamespacedNameV2 = "praxis.runtime/activation-evidence"
	OperationScopeEvidenceActivationInspectionProfileV3 NamespacedNameV2 = "praxis.runtime/activation-inspection-evidence"
	OperationScopeEvidenceRecoveryProfileV3             NamespacedNameV2 = "praxis.runtime/pre-run-recovery-evidence"
	OperationScopeEvidenceSandboxRunProfileV3           NamespacedNameV2 = "praxis.runtime/sandbox-run-evidence"
	OperationScopeEvidenceSandboxTerminationProfileV3   NamespacedNameV2 = "praxis.runtime/sandbox-termination-evidence"
	OperationScopeEvidenceSandboxAdminProfileV3         NamespacedNameV2 = "praxis.runtime/sandbox-admin-evidence"
	OperationScopeEvidenceSandboxInspectionProfileV3    NamespacedNameV2 = "praxis.runtime/sandbox-inspection-evidence"
)

type OperationScopeEvidenceApplicabilityMatrixKeyV3 struct {
	OperationKind OperationScopeKindV3 `json:"operation_kind"`
	EffectKind    EffectKindV2         `json:"effect_kind"`
	PolicyProfile NamespacedNameV2     `json:"policy_profile"`
}

func (k OperationScopeEvidenceApplicabilityMatrixKeyV3) Validate() error {
	if ValidateNamespacedNameV2(NamespacedNameV2(k.EffectKind)) != nil || ValidateNamespacedNameV2(k.PolicyProfile) != nil {
		return core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "Operation Evidence matrix key is unsupported")
	}
	activation := k.PolicyProfile == OperationScopeEvidenceActivationProfileV3 && (k.EffectKind == "praxis.sandbox/allocate" || k.EffectKind == "praxis.sandbox/activate" || k.EffectKind == "praxis.sandbox/open")
	inspection := k.PolicyProfile == OperationScopeEvidenceActivationInspectionProfileV3 && k.EffectKind == "praxis.sandbox/inspect"
	if k.OperationKind == OperationScopeActivationV3 && (activation || inspection) {
		return nil
	}
	sandboxRun := k.OperationKind == OperationScopeRunV3 && k.PolicyProfile == OperationScopeEvidenceSandboxRunProfileV3 && (k.EffectKind == "praxis.sandbox/cancel" || k.EffectKind == "praxis.sandbox/workspace-commit")
	sandboxTermination := k.OperationKind == OperationScopeTerminationV3 && k.PolicyProfile == OperationScopeEvidenceSandboxTerminationProfileV3 && (k.EffectKind == "praxis.sandbox/close" || k.EffectKind == "praxis.sandbox/fence" || k.EffectKind == "praxis.sandbox/release" || k.EffectKind == "praxis.sandbox/cleanup")
	sandboxAdmin := k.OperationKind == OperationScopeAdminV3 && k.PolicyProfile == OperationScopeEvidenceSandboxAdminProfileV3 && (k.EffectKind == "praxis.sandbox/fence" || k.EffectKind == "praxis.sandbox/cleanup")
	sandboxInspection := k.PolicyProfile == OperationScopeEvidenceSandboxInspectionProfileV3 && k.EffectKind == "praxis.sandbox/inspect" && (k.OperationKind == OperationScopeRunV3 || k.OperationKind == OperationScopeTerminationV3 || k.OperationKind == OperationScopeAdminV3)
	if sandboxRun || sandboxTermination || sandboxAdmin || sandboxInspection {
		return nil
	}
	if IsOperationScopeEvidenceActionMatrixKeyV3(k) {
		return nil
	}
	if IsOperationScopeEvidenceMCPConnectMatrixKeyV1(k) {
		return nil
	}
	if !activation && !inspection && !sandboxRun && !sandboxTermination && !sandboxAdmin && !sandboxInspection {
		return core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "Operation Evidence matrix key is not registered")
	}
	return core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "Operation Evidence operation kind is not registered for this profile")
}

type OperationScopeEvidenceStateV3 string

const (
	OperationScopeEvidenceIssuedV3              OperationScopeEvidenceStateV3 = "issued"
	OperationScopeEvidenceConsumedCurrentV3     OperationScopeEvidenceStateV3 = "consumed_current"
	OperationScopeEvidenceConsumedObservationV3 OperationScopeEvidenceStateV3 = "consumed_observation"
	OperationScopeEvidenceRevokedV3             OperationScopeEvidenceStateV3 = "revoked"
	OperationScopeEvidenceExpiredV3             OperationScopeEvidenceStateV3 = "expired"
)

type OperationScopeEvidencePolicyStateV3 string

const (
	OperationScopeEvidencePolicyActiveV3  OperationScopeEvidencePolicyStateV3 = "active"
	OperationScopeEvidencePolicyRevokedV3 OperationScopeEvidencePolicyStateV3 = "revoked"
	OperationScopeEvidencePolicyExpiredV3 OperationScopeEvidencePolicyStateV3 = "expired"
)

type OperationScopeEvidenceApplicabilityModeV3 string

const (
	OperationScopeEvidenceRequiredV3  OperationScopeEvidenceApplicabilityModeV3 = "required"
	OperationScopeEvidenceForbiddenV3 OperationScopeEvidenceApplicabilityModeV3 = "forbidden"
)

type OperationScopeEvidenceApplicabilityDimensionV3 string

const (
	OperationScopeEvidenceRunV3     OperationScopeEvidenceApplicabilityDimensionV3 = "run"
	OperationScopeEvidenceSessionV3 OperationScopeEvidenceApplicabilityDimensionV3 = "session"
	OperationScopeEvidenceTurnV3    OperationScopeEvidenceApplicabilityDimensionV3 = "turn"
	OperationScopeEvidenceActionV3  OperationScopeEvidenceApplicabilityDimensionV3 = "action"
	OperationScopeEvidenceContextV3 OperationScopeEvidenceApplicabilityDimensionV3 = "context"
)

type OperationScopeEvidenceFactRefV3 struct {
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r OperationScopeEvidenceFactRefV3) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || r.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Operation Evidence fact ref is incomplete")
	}
	return r.Digest.Validate()
}

type OperationScopeEvidenceLedgerScopeV3 struct {
	TenantID        core.TenantID `json:"tenant_id"`
	OperationDigest core.Digest   `json:"operation_digest"`
	ChainID         string        `json:"chain_id"`
}

func (s OperationScopeEvidenceLedgerScopeV3) Validate() error {
	if validateEvidenceIDV2(string(s.TenantID)) != nil || validateEvidenceIDV2(s.ChainID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceScopeConflict, "Operation Evidence ledger scope is incomplete")
	}
	return s.OperationDigest.Validate()
}

func (s OperationScopeEvidenceLedgerScopeV3) DigestV3() (core.Digest, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceLedgerScopeV3", s)
}

type OperationScopeEvidenceApplicabilityFactRefV3 struct {
	Kind     NamespacedNameV2 `json:"kind"`
	ID       string           `json:"id"`
	Revision core.Revision    `json:"revision"`
	Digest   core.Digest      `json:"digest"`
}

func (r OperationScopeEvidenceApplicabilityFactRefV3) Validate() error {
	if ValidateNamespacedNameV2(r.Kind) != nil || validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "applicability fact ref is incomplete")
	}
	return r.Digest.Validate()
}

type OperationScopeEvidenceApplicabilityV3 struct {
	Dimension OperationScopeEvidenceApplicabilityDimensionV3 `json:"dimension"`
	Mode      OperationScopeEvidenceApplicabilityModeV3      `json:"mode"`
	Fact      *OperationScopeEvidenceApplicabilityFactRefV3  `json:"fact,omitempty"`
}

func (a OperationScopeEvidenceApplicabilityV3) Validate() error {
	switch a.Dimension {
	case OperationScopeEvidenceRunV3, OperationScopeEvidenceSessionV3, OperationScopeEvidenceTurnV3, OperationScopeEvidenceActionV3, OperationScopeEvidenceContextV3:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "unknown applicability dimension")
	}
	switch a.Mode {
	case OperationScopeEvidenceRequiredV3:
		if a.Fact == nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "required applicability lacks exact fact")
		}
		return a.Fact.Validate()
	case OperationScopeEvidenceForbiddenV3:
		if a.Fact != nil {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "forbidden applicability carries a fact")
		}
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "unknown applicability mode")
	}
}

func ValidateOperationScopeEvidenceApplicabilitySetV3(values []OperationScopeEvidenceApplicabilityV3) error {
	if len(values) != 5 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "all five applicability dimensions are required")
	}
	previous := ""
	for index, value := range values {
		if err := value.Validate(); err != nil {
			return err
		}
		key := string(value.Dimension)
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "applicability dimensions must be sorted and unique")
		}
		previous = key
	}
	return nil
}

func NormalizeOperationScopeEvidenceApplicabilityV3(values []OperationScopeEvidenceApplicabilityV3) []OperationScopeEvidenceApplicabilityV3 {
	result := append([]OperationScopeEvidenceApplicabilityV3{}, values...)
	sort.Slice(result, func(i, j int) bool { return result[i].Dimension < result[j].Dimension })
	return result
}

type OperationScopeEvidenceApplicabilityPolicyRefV3 OperationScopeEvidenceFactRefV3
type OperationScopeEvidencePolicyRefV3 OperationScopeEvidenceFactRefV3

func (r OperationScopeEvidenceApplicabilityPolicyRefV3) Validate() error {
	return OperationScopeEvidenceFactRefV3(r).Validate()
}
func (r OperationScopeEvidencePolicyRefV3) Validate() error {
	return OperationScopeEvidenceFactRefV3(r).Validate()
}

type OperationScopeEvidenceApplicabilityPolicyFactV3 struct {
	ContractVersion      string                                  `json:"contract_version"`
	ID                   string                                  `json:"id"`
	Revision             core.Revision                           `json:"revision"`
	Digest               core.Digest                             `json:"digest"`
	State                OperationScopeEvidencePolicyStateV3     `json:"state"`
	OperationKind        OperationScopeKindV3                    `json:"operation_kind"`
	EffectKind           EffectKindV2                            `json:"effect_kind"`
	Profile              NamespacedNameV2                        `json:"profile"`
	ExecutionScopeDigest core.Digest                             `json:"execution_scope_digest"`
	Applicability        []OperationScopeEvidenceApplicabilityV3 `json:"applicability"`
	ExpiresUnixNano      int64                                   `json:"expires_unix_nano"`
}

func (f OperationScopeEvidenceApplicabilityPolicyFactV3) Validate() error {
	if f.ContractVersion != OperationScopeEvidenceContractVersionV3 || validateEvidenceIDV2(f.ID) != nil || f.Revision == 0 || f.State == "" || f.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceTrustInvalid, "applicability policy identity or TTL is incomplete")
	}
	if ValidateNamespacedNameV2(NamespacedNameV2(f.EffectKind)) != nil || ValidateNamespacedNameV2(f.Profile) != nil || f.ExecutionScopeDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "applicability policy subject is invalid")
	}
	if err := (OperationScopeEvidenceApplicabilityMatrixKeyV3{OperationKind: f.OperationKind, EffectKind: f.EffectKind, PolicyProfile: f.Profile}).Validate(); err != nil {
		return err
	}
	if f.State != OperationScopeEvidencePolicyActiveV3 && f.State != OperationScopeEvidencePolicyRevokedV3 && f.State != OperationScopeEvidencePolicyExpiredV3 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown applicability policy state")
	}
	if err := ValidateOperationScopeEvidenceApplicabilitySetV3(f.Applicability); err != nil {
		return err
	}
	digest, err := f.DigestV3()
	if err != nil || f.Digest != digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "applicability policy digest drifted")
	}
	return nil
}

func (f OperationScopeEvidenceApplicabilityPolicyFactV3) DigestV3() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	copy.Applicability = NormalizeOperationScopeEvidenceApplicabilityV3(copy.Applicability)
	return core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceApplicabilityPolicyFactV3", copy)
}

func SealOperationScopeEvidenceApplicabilityPolicyFactV3(f OperationScopeEvidenceApplicabilityPolicyFactV3) (OperationScopeEvidenceApplicabilityPolicyFactV3, error) {
	f.ContractVersion = OperationScopeEvidenceContractVersionV3
	f.Applicability = NormalizeOperationScopeEvidenceApplicabilityV3(f.Applicability)
	f.Digest = ""
	digest, err := f.DigestV3()
	if err != nil {
		return OperationScopeEvidenceApplicabilityPolicyFactV3{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

func (f OperationScopeEvidenceApplicabilityPolicyFactV3) RefV3() OperationScopeEvidenceApplicabilityPolicyRefV3 {
	return OperationScopeEvidenceApplicabilityPolicyRefV3{ID: f.ID, Revision: f.Revision, Digest: f.Digest, ExpiresUnixNano: f.ExpiresUnixNano}
}

type OperationScopeEvidencePolicyFactV3 struct {
	ContractVersion         string                                `json:"contract_version"`
	ID                      string                                `json:"id"`
	Revision                core.Revision                         `json:"revision"`
	Digest                  core.Digest                           `json:"digest"`
	State                   OperationScopeEvidencePolicyStateV3   `json:"state"`
	OperationKind           OperationScopeKindV3                  `json:"operation_kind"`
	EffectKind              EffectKindV2                          `json:"effect_kind"`
	AllowedPhases           []OperationDispatchEnforcementPhaseV4 `json:"allowed_phases"`
	ExpectedSchema          SchemaRefV2                           `json:"expected_schema"`
	MaximumPayloadBytes     uint64                                `json:"maximum_payload_bytes"`
	MaximumQualificationTTL time.Duration                         `json:"maximum_qualification_ttl"`
	MaximumIngestGrace      time.Duration                         `json:"maximum_ingest_grace"`
	ExpiresUnixNano         int64                                 `json:"expires_unix_nano"`
}

type OperationScopeEvidencePolicyCASRequestV3 struct {
	ExpectedRevision core.Revision                      `json:"expected_revision"`
	Next             OperationScopeEvidencePolicyFactV3 `json:"next"`
}

type OperationScopeEvidenceApplicabilityPolicyCASRequestV3 struct {
	ExpectedRevision core.Revision                                   `json:"expected_revision"`
	Next             OperationScopeEvidenceApplicabilityPolicyFactV3 `json:"next"`
}

func ValidateOperationScopeEvidencePolicyTransitionV3(current, next OperationScopeEvidencePolicyFactV3) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	left := current
	right := next
	left.Revision = 0
	right.Revision = 0
	left.State = ""
	right.State = ""
	left.Digest = ""
	right.Digest = ""
	ld, _ := core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidencePolicySemanticV3", left)
	rd, _ := core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidencePolicySemanticV3", right)
	if current.State != OperationScopeEvidencePolicyActiveV3 || next.Revision != current.Revision+1 || ld != rd || (next.State != OperationScopeEvidencePolicyRevokedV3 && next.State != OperationScopeEvidencePolicyExpiredV3) {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Evidence policy transition changed immutable semantics or state")
	}
	return nil
}

func ValidateOperationScopeEvidenceApplicabilityPolicyTransitionV3(current, next OperationScopeEvidenceApplicabilityPolicyFactV3) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	left := current
	right := next
	left.Revision = 0
	right.Revision = 0
	left.State = ""
	right.State = ""
	left.Digest = ""
	right.Digest = ""
	left.Applicability = NormalizeOperationScopeEvidenceApplicabilityV3(left.Applicability)
	right.Applicability = NormalizeOperationScopeEvidenceApplicabilityV3(right.Applicability)
	ld, _ := core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceApplicabilityPolicySemanticV3", left)
	rd, _ := core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceApplicabilityPolicySemanticV3", right)
	if current.State != OperationScopeEvidencePolicyActiveV3 || next.Revision != current.Revision+1 || ld != rd || (next.State != OperationScopeEvidencePolicyRevokedV3 && next.State != OperationScopeEvidencePolicyExpiredV3) {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "applicability policy transition changed immutable semantics or state")
	}
	return nil
}

func (f OperationScopeEvidencePolicyFactV3) Validate() error {
	if f.ContractVersion != OperationScopeEvidenceContractVersionV3 || validateEvidenceIDV2(f.ID) != nil || f.Revision == 0 || f.ExpiresUnixNano <= 0 || f.MaximumPayloadBytes == 0 || f.MaximumPayloadBytes > MaxOperationScopeEvidencePayloadBytesV3 || f.MaximumQualificationTTL <= 0 || f.MaximumQualificationTTL > MaxDispatchPermitTTL || f.MaximumIngestGrace < 0 || f.MaximumIngestGrace > MaxOperationScopeEvidenceIngestGraceV3 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceTrustInvalid, "Operation Evidence policy identity, limits or TTL are invalid")
	}
	if ValidateNamespacedNameV2(NamespacedNameV2(f.EffectKind)) != nil || f.ExpectedSchema.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Operation Evidence policy effect or schema is invalid")
	}
	if !isRegisteredOperationScopeEvidencePolicySubjectV3(f.OperationKind, f.EffectKind) {
		return core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "Operation Evidence policy operation and effect are not registered")
	}
	if f.State != OperationScopeEvidencePolicyActiveV3 && f.State != OperationScopeEvidencePolicyRevokedV3 && f.State != OperationScopeEvidencePolicyExpiredV3 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown Operation Evidence policy state")
	}
	if len(f.AllowedPhases) == 0 || len(f.AllowedPhases) > 2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Operation Evidence policy phases are empty or unbounded")
	}
	previous := ""
	for index, phase := range f.AllowedPhases {
		if phase != OperationDispatchEnforcementPrepareV4 && phase != OperationDispatchEnforcementExecuteV4 || index > 0 && string(phase) <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Operation Evidence policy phases must be sorted and unique")
		}
		previous = string(phase)
	}
	digest, err := f.DigestV3()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Operation Evidence policy digest drifted")
	}
	return nil
}

func (f OperationScopeEvidencePolicyFactV3) DigestV3() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	if copy.AllowedPhases == nil {
		copy.AllowedPhases = []OperationDispatchEnforcementPhaseV4{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidencePolicyFactV3", copy)
}

func SealOperationScopeEvidencePolicyFactV3(f OperationScopeEvidencePolicyFactV3) (OperationScopeEvidencePolicyFactV3, error) {
	f.ContractVersion = OperationScopeEvidenceContractVersionV3
	sort.Slice(f.AllowedPhases, func(i, j int) bool { return f.AllowedPhases[i] < f.AllowedPhases[j] })
	f.Digest = ""
	digest, err := f.DigestV3()
	if err != nil {
		return OperationScopeEvidencePolicyFactV3{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

func (f OperationScopeEvidencePolicyFactV3) RefV3() OperationScopeEvidencePolicyRefV3 {
	return OperationScopeEvidencePolicyRefV3{ID: f.ID, Revision: f.Revision, Digest: f.Digest, ExpiresUnixNano: f.ExpiresUnixNano}
}

type OperationScopeEvidenceSourceRegistrationFactV3 struct {
	ContractVersion string                              `json:"contract_version"`
	ID              string                              `json:"id"`
	Revision        core.Revision                       `json:"revision"`
	Digest          core.Digest                         `json:"digest"`
	SourceID        NamespacedNameV2                    `json:"source_id"`
	SourceEpoch     core.Epoch                          `json:"source_epoch"`
	NextSequence    uint64                              `json:"next_sequence"`
	LedgerScope     OperationScopeEvidenceLedgerScopeV3 `json:"ledger_scope"`
	Producer        EvidenceProducerBindingRefV2        `json:"producer"`
	Policy          OperationScopeEvidencePolicyRefV3   `json:"policy"`
	State           EvidenceSourceStateV2               `json:"state"`
	CreatedUnixNano int64                               `json:"created_unix_nano"`
	UpdatedUnixNano int64                               `json:"updated_unix_nano"`
	ExpiresUnixNano int64                               `json:"expires_unix_nano"`
}

func (f OperationScopeEvidenceSourceRegistrationFactV3) Validate() error {
	if f.ContractVersion != OperationScopeEvidenceContractVersionV3 || validateEvidenceIDV2(f.ID) != nil || f.Revision == 0 || f.SourceEpoch == 0 || f.NextSequence == 0 || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano || f.ExpiresUnixNano <= f.UpdatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceSourceMissing, "Operation Evidence source registration is incomplete")
	}
	if ValidateNamespacedNameV2(f.SourceID) != nil || f.LedgerScope.Validate() != nil || f.Producer.Validate() != nil || f.Policy.Validate() != nil || f.State != EvidenceSourceActive {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, "Operation Evidence source is invalid or inactive")
	}
	digest, err := f.DigestV3()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Operation Evidence source digest drifted")
	}
	return nil
}

func (f OperationScopeEvidenceSourceRegistrationFactV3) DigestV3() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceSourceRegistrationFactV3", copy)
}

func SealOperationScopeEvidenceSourceRegistrationFactV3(f OperationScopeEvidenceSourceRegistrationFactV3) (OperationScopeEvidenceSourceRegistrationFactV3, error) {
	f.ContractVersion = OperationScopeEvidenceContractVersionV3
	f.Digest = ""
	digest, err := f.DigestV3()
	if err != nil {
		return OperationScopeEvidenceSourceRegistrationFactV3{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

type OperationScopeEvidenceSourceKeyV3 struct {
	RegistrationID string     `json:"registration_id"`
	SourceEpoch    core.Epoch `json:"source_epoch"`
	SourceSequence uint64     `json:"source_sequence"`
}

func (k OperationScopeEvidenceSourceKeyV3) Validate() error {
	if validateEvidenceIDV2(k.RegistrationID) != nil || k.SourceEpoch == 0 || k.SourceSequence == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceSourceMissing, "Operation Evidence source key is incomplete")
	}
	return nil
}

type OperationScopeEvidenceSourceReservationV3 struct {
	Registration OperationScopeEvidenceFactRefV3   `json:"registration"`
	Source       OperationScopeEvidenceSourceKeyV3 `json:"source"`
	EventID      string                            `json:"event_id"`
	Schema       SchemaRefV2                       `json:"expected_schema"`
}

func (r OperationScopeEvidenceSourceReservationV3) Validate() error {
	if err := r.Source.Validate(); err != nil {
		return err
	}
	if err := r.Registration.Validate(); err != nil {
		return err
	}
	if r.Registration.ID != r.Source.RegistrationID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceSourceStale, "source reservation binds another registration")
	}
	if validateEvidenceIDV2(r.EventID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Operation Evidence event id is invalid")
	}
	return r.Schema.Validate()
}

type OperationScopeEvidenceScopeV3 struct {
	LedgerScope         OperationScopeEvidenceLedgerScopeV3            `json:"ledger_scope"`
	Operation           OperationSubjectV3                             `json:"operation"`
	OperationDigest     core.Digest                                    `json:"operation_digest"`
	EffectID            core.EffectIntentID                            `json:"effect_id"`
	EffectRevision      core.Revision                                  `json:"effect_revision"`
	EffectDigest        core.Digest                                    `json:"effect_digest"`
	EffectKind          EffectKindV2                                   `json:"effect_kind"`
	AttemptID           string                                         `json:"attempt_id"`
	Phase               OperationDispatchEnforcementPhaseV4            `json:"phase"`
	ApplicabilityPolicy OperationScopeEvidenceApplicabilityPolicyRefV3 `json:"applicability_policy"`
	Applicability       []OperationScopeEvidenceApplicabilityV3        `json:"applicability"`
	Generation          GenerationBindingAssociationRefV1              `json:"generation"`
}

func (s OperationScopeEvidenceScopeV3) Validate() error {
	if s.LedgerScope.Validate() != nil || s.Operation.Validate() != nil || validateEvidenceIDV2(string(s.EffectID)) != nil || s.EffectRevision == 0 || s.EffectDigest.Validate() != nil || ValidateNamespacedNameV2(NamespacedNameV2(s.EffectKind)) != nil || validateEvidenceIDV2(s.AttemptID) != nil || s.ApplicabilityPolicy.Validate() != nil || s.Generation.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceScopeConflict, "Operation Evidence scope is incomplete")
	}
	digest, err := s.Operation.DigestV3()
	if err != nil || digest != s.OperationDigest || digest != s.LedgerScope.OperationDigest || s.LedgerScope.TenantID != s.Operation.ExecutionScope.Identity.TenantID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceScopeConflict, "Operation Evidence scope drifted")
	}
	if s.Phase != OperationDispatchEnforcementPrepareV4 && s.Phase != OperationDispatchEnforcementExecuteV4 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "Operation Evidence phase is invalid")
	}
	return ValidateOperationScopeEvidenceApplicabilitySetV3(s.Applicability)
}

type OperationScopeEvidenceRuntimeCurrentProjectionV3 struct {
	ContractVersion    string                                 `json:"contract_version"`
	Scope              OperationScopeEvidenceScopeV3          `json:"scope"`
	PermitID           string                                 `json:"permit_id"`
	PermitFactRevision core.Revision                          `json:"permit_fact_revision"`
	PermitDigest       core.Digest                            `json:"permit_digest"`
	AdmissionDigest    core.Digest                            `json:"admission_digest"`
	Authorization      OperationReviewAuthorizationRefV4      `json:"authorization"`
	Phase              OperationDispatchEnforcementPhaseRefV4 `json:"phase_ref"`
	CheckedUnixNano    int64                                  `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                                  `json:"expires_unix_nano"`
	Digest             core.Digest                            `json:"digest"`
}

func (p OperationScopeEvidenceRuntimeCurrentProjectionV3) Validate(now time.Time) error {
	if p.ContractVersion != OperationScopeEvidenceContractVersionV3 || p.Scope.Validate() != nil || validateEvidenceIDV2(p.PermitID) != nil || p.PermitFactRevision == 0 || p.PermitDigest.Validate() != nil || p.AdmissionDigest.Validate() != nil || p.Authorization.Validate() != nil || p.Phase.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.IsZero() || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "Operation Evidence runtime projection is incomplete or expired")
	}
	if p.Phase.OperationDigest != p.Scope.OperationDigest || p.Phase.EffectID != p.Scope.EffectID || p.Phase.PermitID != p.PermitID || p.Phase.PermitFactRevision != p.PermitFactRevision || p.Phase.PermitDigest != p.PermitDigest || p.Phase.AdmissionDigest != p.AdmissionDigest || p.Phase.ReviewAuthorization != p.Authorization || p.Phase.AttemptID != p.Scope.AttemptID || p.Phase.Phase != p.Scope.Phase || p.ExpiresUnixNano > p.Phase.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Operation Evidence runtime projection binds another dispatch")
	}
	digest, err := p.DigestV3()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Operation Evidence runtime projection digest drifted")
	}
	return nil
}

func (p OperationScopeEvidenceRuntimeCurrentProjectionV3) DigestV3() (core.Digest, error) {
	copy := p
	copy.Digest = ""
	copy.Scope.Applicability = NormalizeOperationScopeEvidenceApplicabilityV3(copy.Scope.Applicability)
	return core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceRuntimeCurrentProjectionV3", copy)
}

func SealOperationScopeEvidenceRuntimeCurrentProjectionV3(p OperationScopeEvidenceRuntimeCurrentProjectionV3, now time.Time) (OperationScopeEvidenceRuntimeCurrentProjectionV3, error) {
	p.ContractVersion = OperationScopeEvidenceContractVersionV3
	p.Scope.Applicability = NormalizeOperationScopeEvidenceApplicabilityV3(p.Scope.Applicability)
	p.Digest = ""
	digest, err := p.DigestV3()
	if err != nil {
		return OperationScopeEvidenceRuntimeCurrentProjectionV3{}, err
	}
	p.Digest = digest
	return p, p.Validate(now)
}

type OperationScopeEvidenceApplicabilityCurrentProjectionV3 struct {
	Fact                 OperationScopeEvidenceApplicabilityFactRefV3 `json:"fact"`
	ExecutionScopeDigest core.Digest                                  `json:"execution_scope_digest"`
	Current              bool                                         `json:"current"`
	ExpiresUnixNano      int64                                        `json:"expires_unix_nano"`
	Digest               core.Digest                                  `json:"digest"`
}

func (p OperationScopeEvidenceApplicabilityCurrentProjectionV3) Validate(expected OperationScopeEvidenceApplicabilityFactRefV3, scopeDigest core.Digest, now time.Time) error {
	if p.Fact.Validate() != nil || p.ExecutionScopeDigest.Validate() != nil || p.ExpiresUnixNano <= 0 || p.Digest.Validate() != nil || !p.Current || now.IsZero() || !now.Before(time.Unix(0, p.ExpiresUnixNano)) || p.Fact != expected || p.ExecutionScopeDigest != scopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "applicability projection is stale or mismatched")
	}
	copy := p
	copy.Digest = ""
	digest, err := core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceApplicabilityCurrentProjectionV3", copy)
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "applicability projection digest drifted")
	}
	return nil
}

type OperationScopeEvidenceQualificationFactV3 struct {
	ContractVersion        string                                           `json:"contract_version"`
	ID                     string                                           `json:"id"`
	Revision               core.Revision                                    `json:"revision"`
	State                  OperationScopeEvidenceStateV3                    `json:"state"`
	Scope                  OperationScopeEvidenceScopeV3                    `json:"scope"`
	Runtime                OperationScopeEvidenceRuntimeCurrentProjectionV3 `json:"runtime_current"`
	EvidencePolicy         OperationScopeEvidencePolicyRefV3                `json:"evidence_policy"`
	Reservation            OperationScopeEvidenceSourceReservationV3        `json:"reservation"`
	RequestedTTL           time.Duration                                    `json:"requested_ttl"`
	CreatedUnixNano        int64                                            `json:"created_unix_nano"`
	UpdatedUnixNano        int64                                            `json:"updated_unix_nano"`
	ExpiresUnixNano        int64                                            `json:"expires_unix_nano"`
	IngestNotAfterUnixNano int64                                            `json:"ingest_not_after_unix_nano"`
	InvalidationReason     core.ReasonCode                                  `json:"invalidation_reason,omitempty"`
	Consumption            *OperationScopeEvidenceConsumptionRefV3          `json:"consumption,omitempty"`
	Digest                 core.Digest                                      `json:"digest"`
}

type OperationScopeEvidenceQualificationRefV3 OperationScopeEvidenceFactRefV3

func (r OperationScopeEvidenceQualificationRefV3) Validate() error {
	return OperationScopeEvidenceFactRefV3(r).Validate()
}

func (f OperationScopeEvidenceQualificationFactV3) RefV3() OperationScopeEvidenceQualificationRefV3 {
	return OperationScopeEvidenceQualificationRefV3{ID: f.ID, Revision: f.Revision, Digest: f.Digest, ExpiresUnixNano: f.ExpiresUnixNano}
}

func (f OperationScopeEvidenceQualificationFactV3) Validate() error {
	created := time.Unix(0, f.CreatedUnixNano)
	if f.ContractVersion != OperationScopeEvidenceContractVersionV3 || validateEvidenceIDV2(f.ID) != nil || f.Revision == 0 || f.Scope.Validate() != nil || f.Runtime.Validate(created) != nil || f.EvidencePolicy.Validate() != nil || f.Reservation.Validate() != nil || f.RequestedTTL <= 0 || f.RequestedTTL > MaxDispatchPermitTTL || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano || f.ExpiresUnixNano <= f.CreatedUnixNano || f.IngestNotAfterUnixNano < f.ExpiresUnixNano || time.Duration(f.IngestNotAfterUnixNano-f.ExpiresUnixNano) > MaxOperationScopeEvidenceIngestGraceV3 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceUnavailable, "Operation Evidence qualification is incomplete")
	}
	if f.Runtime.Scope.OperationDigest != f.Scope.OperationDigest || f.Runtime.Scope.EffectID != f.Scope.EffectID || f.Runtime.Scope.Phase != f.Scope.Phase || f.Reservation.Source.SourceSequence == 0 {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Operation Evidence qualification coordinates drifted")
	}
	switch f.State {
	case OperationScopeEvidenceIssuedV3:
		if f.Consumption != nil || f.InvalidationReason != "" || f.UpdatedUnixNano >= f.IngestNotAfterUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "issued qualification carries terminal state")
		}
	case OperationScopeEvidenceConsumedCurrentV3, OperationScopeEvidenceConsumedObservationV3:
		if f.Consumption == nil || f.Consumption.Validate() != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "consumed qualification lacks consumption")
		}
	case OperationScopeEvidenceRevokedV3, OperationScopeEvidenceExpiredV3:
		if f.InvalidationReason == "" || f.Consumption != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "invalidated qualification state is incomplete")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown Operation Evidence qualification state")
	}
	digest, err := f.DigestV3()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Operation Evidence qualification digest drifted")
	}
	return nil
}

func (f OperationScopeEvidenceQualificationFactV3) DigestV3() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	copy.Scope.Applicability = NormalizeOperationScopeEvidenceApplicabilityV3(copy.Scope.Applicability)
	return core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceQualificationFactV3", copy)
}

func SealOperationScopeEvidenceQualificationFactV3(f OperationScopeEvidenceQualificationFactV3) (OperationScopeEvidenceQualificationFactV3, error) {
	f.ContractVersion = OperationScopeEvidenceContractVersionV3
	f.Scope.Applicability = NormalizeOperationScopeEvidenceApplicabilityV3(f.Scope.Applicability)
	f.Digest = ""
	digest, err := f.DigestV3()
	if err != nil {
		return OperationScopeEvidenceQualificationFactV3{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

type OperationScopeEvidenceProviderHandoffFactV3 struct {
	ContractVersion  string                                   `json:"contract_version"`
	ID               string                                   `json:"id"`
	Revision         core.Revision                            `json:"revision"`
	Qualification    OperationScopeEvidenceQualificationRefV3 `json:"qualification"`
	Phase            OperationDispatchEnforcementPhaseRefV4   `json:"phase"`
	CheckedUnixNano  int64                                    `json:"checked_unix_nano"`
	NotAfterUnixNano int64                                    `json:"not_after_unix_nano"`
	Digest           core.Digest                              `json:"digest"`
}

type OperationScopeEvidenceProviderHandoffRefV3 OperationScopeEvidenceFactRefV3

func (r OperationScopeEvidenceProviderHandoffRefV3) Validate() error {
	return OperationScopeEvidenceFactRefV3(r).Validate()
}
func (f OperationScopeEvidenceProviderHandoffFactV3) RefV3() OperationScopeEvidenceProviderHandoffRefV3 {
	return OperationScopeEvidenceProviderHandoffRefV3{ID: f.ID, Revision: f.Revision, Digest: f.Digest, ExpiresUnixNano: f.NotAfterUnixNano}
}
func (f OperationScopeEvidenceProviderHandoffFactV3) Validate() error {
	if f.ContractVersion != OperationScopeEvidenceContractVersionV3 || validateEvidenceIDV2(f.ID) != nil || f.Revision != 1 || f.Qualification.Validate() != nil || f.Phase.Validate() != nil || f.CheckedUnixNano <= 0 || f.NotAfterUnixNano <= f.CheckedUnixNano || f.NotAfterUnixNano > f.Qualification.ExpiresUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceUnavailable, "Provider handoff is incomplete")
	}
	digest, err := f.DigestV3()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Provider handoff digest drifted")
	}
	return nil
}
func (f OperationScopeEvidenceProviderHandoffFactV3) DigestV3() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceProviderHandoffFactV3", copy)
}
func SealOperationScopeEvidenceProviderHandoffFactV3(f OperationScopeEvidenceProviderHandoffFactV3) (OperationScopeEvidenceProviderHandoffFactV3, error) {
	f.ContractVersion = OperationScopeEvidenceContractVersionV3
	f.Digest = ""
	digest, err := f.DigestV3()
	if err != nil {
		return OperationScopeEvidenceProviderHandoffFactV3{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

type OperationScopeEvidenceCandidateV3 struct {
	ContractVersion  string                                   `json:"contract_version"`
	Qualification    OperationScopeEvidenceQualificationRefV3 `json:"qualification"`
	Source           OperationScopeEvidenceSourceKeyV3        `json:"source"`
	EventID          string                                   `json:"event_id"`
	TrustClass       EvidenceTrustClassV2                     `json:"trust_class"`
	Payload          EvidencePayloadRefV2                     `json:"payload"`
	Causation        []EvidenceCausationRefV2                 `json:"causation"`
	CorrelationID    string                                   `json:"correlation_id"`
	ObservedUnixNano int64                                    `json:"observed_unix_nano"`
}

func (c OperationScopeEvidenceCandidateV3) Validate() error {
	if c.ContractVersion != OperationScopeEvidenceContractVersionV3 || c.Qualification.Validate() != nil || c.Source.Validate() != nil || validateEvidenceIDV2(c.EventID) != nil || c.TrustClass != EvidenceTrustObservation || c.Payload.Validate() != nil || validateEvidenceIDV2(c.CorrelationID) != nil || c.ObservedUnixNano <= 0 || len(c.Causation) > MaxEvidenceCausationRefs {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceUnavailable, "Operation Evidence candidate is invalid")
	}
	previous := ""
	for index, ref := range c.Causation {
		if ref.LedgerScopeDigest.Validate() != nil || validateEvidenceIDV2(ref.EventID) != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "candidate causation is invalid")
		}
		key := string(ref.LedgerScopeDigest) + "\x00" + ref.EventID
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "candidate causation must be sorted and unique")
		}
		previous = key
	}
	return nil
}

func (c OperationScopeEvidenceCandidateV3) DigestV3() (core.Digest, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	if c.Causation == nil {
		c.Causation = []EvidenceCausationRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceCandidateV3", c)
}

type OperationScopeEvidenceRecordRefV3 struct {
	LedgerScopeDigest core.Digest `json:"ledger_scope_digest"`
	Sequence          uint64      `json:"sequence"`
	RecordDigest      core.Digest `json:"record_digest"`
}

func (r OperationScopeEvidenceRecordRefV3) Validate() error {
	if r.LedgerScopeDigest.Validate() != nil || r.Sequence == 0 || r.RecordDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Operation Evidence record ref is incomplete")
	}
	return nil
}

type OperationScopeEvidenceRecordV3 struct {
	Ref                  OperationScopeEvidenceRecordRefV3 `json:"ref"`
	Candidate            OperationScopeEvidenceCandidateV3 `json:"candidate"`
	CandidateDigest      core.Digest                       `json:"candidate_digest"`
	PreviousRecordDigest core.Digest                       `json:"previous_record_digest"`
	IngestedUnixNano     int64                             `json:"ingested_unix_nano"`
	LateObservation      bool                              `json:"late_observation"`
}

func SealOperationScopeEvidenceRecordV3(record OperationScopeEvidenceRecordV3) (OperationScopeEvidenceRecordV3, error) {
	if err := record.Candidate.Validate(); err != nil {
		return OperationScopeEvidenceRecordV3{}, err
	}
	candidateDigest, err := record.Candidate.DigestV3()
	if err != nil {
		return OperationScopeEvidenceRecordV3{}, err
	}
	record.CandidateDigest = candidateDigest
	record.Ref.RecordDigest = ""
	digest, err := core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceRecordV3", record)
	if err != nil {
		return OperationScopeEvidenceRecordV3{}, err
	}
	record.Ref.RecordDigest = digest
	return record, record.Validate()
}

func (r OperationScopeEvidenceRecordV3) Validate() error {
	if r.Ref.Validate() != nil || r.Candidate.Validate() != nil || r.CandidateDigest.Validate() != nil || r.PreviousRecordDigest.Validate() != nil || r.IngestedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceChainConflict, "Operation Evidence record is invalid")
	}
	digest, err := r.Candidate.DigestV3()
	if err != nil || digest != r.CandidateDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Operation Evidence candidate digest drifted")
	}
	copy := r
	copy.Ref.RecordDigest = ""
	recordDigest, err := core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceRecordV3", copy)
	if err != nil || recordDigest != r.Ref.RecordDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Operation Evidence record digest drifted")
	}
	return nil
}

type OperationScopeEvidenceConsumptionRefV3 struct {
	ID       string                            `json:"id"`
	Revision core.Revision                     `json:"revision"`
	Digest   core.Digest                       `json:"digest"`
	Record   OperationScopeEvidenceRecordRefV3 `json:"record"`
}

func (r OperationScopeEvidenceConsumptionRefV3) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil || r.Record.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Operation Evidence consumption ref is incomplete")
	}
	return nil
}

type OperationScopeEvidenceConsumptionFactV3 struct {
	ContractVersion string                                     `json:"contract_version"`
	ID              string                                     `json:"id"`
	Revision        core.Revision                              `json:"revision"`
	Qualification   OperationScopeEvidenceQualificationRefV3   `json:"qualification"`
	Handoff         OperationScopeEvidenceProviderHandoffRefV3 `json:"handoff"`
	CandidateDigest core.Digest                                `json:"candidate_digest"`
	Record          OperationScopeEvidenceRecordRefV3          `json:"record"`
	LateObservation bool                                       `json:"late_observation"`
	CreatedUnixNano int64                                      `json:"created_unix_nano"`
	Digest          core.Digest                                `json:"digest"`
}

func (f OperationScopeEvidenceConsumptionFactV3) Validate() error {
	if f.ContractVersion != OperationScopeEvidenceContractVersionV3 || validateEvidenceIDV2(f.ID) != nil || f.Revision != 1 || f.Qualification.Validate() != nil || f.Handoff.Validate() != nil || f.CandidateDigest.Validate() != nil || f.Record.Validate() != nil || f.CreatedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceUnavailable, "Operation Evidence consumption is incomplete")
	}
	digest, err := f.DigestV3()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Operation Evidence consumption digest drifted")
	}
	return nil
}
func (f OperationScopeEvidenceConsumptionFactV3) DigestV3() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceConsumptionFactV3", copy)
}
func SealOperationScopeEvidenceConsumptionFactV3(f OperationScopeEvidenceConsumptionFactV3) (OperationScopeEvidenceConsumptionFactV3, error) {
	f.ContractVersion = OperationScopeEvidenceContractVersionV3
	f.Digest = ""
	digest, err := f.DigestV3()
	if err != nil {
		return OperationScopeEvidenceConsumptionFactV3{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}
func (f OperationScopeEvidenceConsumptionFactV3) RefV3() OperationScopeEvidenceConsumptionRefV3 {
	return OperationScopeEvidenceConsumptionRefV3{ID: f.ID, Revision: f.Revision, Digest: f.Digest, Record: f.Record}
}

type IssueOperationScopeEvidenceRequestV3 struct {
	QualificationID    string                                    `json:"qualification_id"`
	Scope              OperationScopeEvidenceScopeV3             `json:"scope"`
	PermitID           string                                    `json:"permit_id"`
	PermitFactRevision core.Revision                             `json:"permit_fact_revision"`
	PermitDigest       core.Digest                               `json:"permit_digest"`
	AdmissionDigest    core.Digest                               `json:"admission_digest"`
	Authorization      OperationReviewAuthorizationRefV4         `json:"authorization"`
	PhaseRef           OperationDispatchEnforcementPhaseRefV4    `json:"phase_ref"`
	EvidencePolicy     OperationScopeEvidencePolicyRefV3         `json:"evidence_policy"`
	Reservation        OperationScopeEvidenceSourceReservationV3 `json:"reservation"`
	RequestedTTL       time.Duration                             `json:"requested_ttl"`
}

func (r IssueOperationScopeEvidenceRequestV3) Validate() error {
	if validateEvidenceIDV2(r.QualificationID) != nil || r.Scope.Validate() != nil || validateEvidenceIDV2(r.PermitID) != nil || r.PermitFactRevision == 0 || r.PermitDigest.Validate() != nil || r.AdmissionDigest.Validate() != nil || r.Authorization.Validate() != nil || r.PhaseRef.Validate() != nil || r.EvidencePolicy.Validate() != nil || r.Reservation.Validate() != nil || r.RequestedTTL <= 0 || r.RequestedTTL > MaxDispatchPermitTTL {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceUnavailable, "Operation Evidence Issue request is incomplete")
	}
	return nil
}

type InspectOperationScopeEvidenceRequestV3 struct {
	QualificationID string `json:"qualification_id"`
}

func (r InspectOperationScopeEvidenceRequestV3) Validate() error {
	if validateEvidenceIDV2(r.QualificationID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "qualification id is invalid")
	}
	return nil
}

type InspectCurrentOperationScopeEvidenceRequestV3 struct {
	Qualification OperationScopeEvidenceQualificationRefV3 `json:"qualification"`
}

func (r InspectCurrentOperationScopeEvidenceRequestV3) Validate() error {
	return r.Qualification.Validate()
}

type HandoffOperationScopeEvidenceRequestV3 struct {
	HandoffID     string                                   `json:"handoff_id"`
	Qualification OperationScopeEvidenceQualificationRefV3 `json:"qualification"`
}

func (r HandoffOperationScopeEvidenceRequestV3) Validate() error {
	if validateEvidenceIDV2(r.HandoffID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "handoff id is invalid")
	}
	return r.Qualification.Validate()
}

type ConsumeOperationScopeEvidenceRequestV3 struct {
	ConsumptionID string                                     `json:"consumption_id"`
	Handoff       OperationScopeEvidenceProviderHandoffRefV3 `json:"handoff"`
	Candidate     OperationScopeEvidenceCandidateV3          `json:"candidate"`
}

func (r ConsumeOperationScopeEvidenceRequestV3) Validate() error {
	if validateEvidenceIDV2(r.ConsumptionID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "consumption id is invalid")
	}
	if err := r.Handoff.Validate(); err != nil {
		return err
	}
	return r.Candidate.Validate()
}

type OperationScopeEvidenceConsumeResultV3 struct {
	Qualification OperationScopeEvidenceQualificationFactV3      `json:"qualification"`
	Consumption   OperationScopeEvidenceConsumptionFactV3        `json:"consumption"`
	Record        OperationScopeEvidenceRecordV3                 `json:"record"`
	Source        OperationScopeEvidenceSourceRegistrationFactV3 `json:"source"`
}

type OperationScopeEvidenceAtomicConsumeRequestV3 struct {
	ExpectedQualificationRevision core.Revision                              `json:"expected_qualification_revision"`
	ExpectedSourceRevision        core.Revision                              `json:"expected_source_revision"`
	ConsumptionID                 string                                     `json:"consumption_id"`
	Handoff                       OperationScopeEvidenceProviderHandoffRefV3 `json:"handoff"`
	Candidate                     OperationScopeEvidenceCandidateV3          `json:"candidate"`
	LateObservation               bool                                       `json:"late_observation"`
	ConsumedUnixNano              int64                                      `json:"consumed_unix_nano"`
}

func (r OperationScopeEvidenceAtomicConsumeRequestV3) Validate() error {
	if r.ExpectedQualificationRevision == 0 || r.ExpectedSourceRevision == 0 || validateEvidenceIDV2(r.ConsumptionID) != nil || r.ConsumedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceUnavailable, "atomic Operation Evidence consume request is incomplete")
	}
	if err := r.Handoff.Validate(); err != nil {
		return err
	}
	return r.Candidate.Validate()
}

type OperationScopeEvidenceFactPortV3 interface {
	CreateOperationScopeEvidencePolicyV3(context.Context, OperationScopeEvidencePolicyFactV3) (OperationScopeEvidencePolicyFactV3, error)
	InspectOperationScopeEvidencePolicyV3(context.Context, string) (OperationScopeEvidencePolicyFactV3, error)
	CompareAndSwapOperationScopeEvidencePolicyV3(context.Context, OperationScopeEvidencePolicyCASRequestV3) (OperationScopeEvidencePolicyFactV3, error)
	CreateOperationScopeEvidenceApplicabilityPolicyV3(context.Context, OperationScopeEvidenceApplicabilityPolicyFactV3) (OperationScopeEvidenceApplicabilityPolicyFactV3, error)
	InspectOperationScopeEvidenceApplicabilityPolicyV3(context.Context, string) (OperationScopeEvidenceApplicabilityPolicyFactV3, error)
	CompareAndSwapOperationScopeEvidenceApplicabilityPolicyV3(context.Context, OperationScopeEvidenceApplicabilityPolicyCASRequestV3) (OperationScopeEvidenceApplicabilityPolicyFactV3, error)
	CreateOperationScopeEvidenceSourceV3(context.Context, OperationScopeEvidenceSourceRegistrationFactV3) (OperationScopeEvidenceSourceRegistrationFactV3, error)
	InspectOperationScopeEvidenceSourceV3(context.Context, string) (OperationScopeEvidenceSourceRegistrationFactV3, error)
	CreateOperationScopeEvidenceQualificationV3(context.Context, OperationScopeEvidenceQualificationFactV3) (OperationScopeEvidenceQualificationFactV3, error)
	InspectOperationScopeEvidenceQualificationV3(context.Context, string) (OperationScopeEvidenceQualificationFactV3, error)
	InspectOperationScopeEvidenceQualificationBySourceV3(context.Context, OperationScopeEvidenceSourceKeyV3) (OperationScopeEvidenceQualificationFactV3, error)
	CreateOperationScopeEvidenceProviderHandoffV3(context.Context, OperationScopeEvidenceProviderHandoffFactV3) (OperationScopeEvidenceProviderHandoffFactV3, error)
	InspectOperationScopeEvidenceProviderHandoffV3(context.Context, string) (OperationScopeEvidenceProviderHandoffFactV3, error)
	ConsumeOperationScopeEvidenceV3(context.Context, OperationScopeEvidenceAtomicConsumeRequestV3) (OperationScopeEvidenceConsumeResultV3, error)
	InspectOperationScopeEvidenceConsumptionV3(context.Context, string) (OperationScopeEvidenceConsumptionFactV3, error)
	InspectOperationScopeEvidenceRecordV3(context.Context, OperationScopeEvidenceRecordRefV3) (OperationScopeEvidenceRecordV3, error)
}

type OperationScopeEvidenceRuntimeCurrentReaderV3 interface {
	InspectOperationScopeEvidenceRuntimeCurrentV3(context.Context, OperationScopeEvidenceScopeV3, string) (OperationScopeEvidenceRuntimeCurrentProjectionV3, error)
}
type OperationScopeEvidenceGenerationCurrentReaderV3 interface {
	InspectOperationScopeEvidenceGenerationCurrentV3(context.Context, GenerationBindingAssociationRefV1) (OperationScopeEvidenceFactRefV3, error)
}
type OperationScopeEvidenceApplicabilityCurrentReaderV3 interface {
	InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Context, OperationScopeEvidenceApplicabilityFactRefV3) (OperationScopeEvidenceApplicabilityCurrentProjectionV3, error)
}

type OperationScopeEvidenceGovernancePortV3 interface {
	IssueOperationScopeEvidenceV3(context.Context, IssueOperationScopeEvidenceRequestV3) (OperationScopeEvidenceQualificationFactV3, error)
	InspectOperationScopeEvidenceV3(context.Context, InspectOperationScopeEvidenceRequestV3) (OperationScopeEvidenceQualificationFactV3, error)
	InspectCurrentOperationScopeEvidenceV3(context.Context, InspectCurrentOperationScopeEvidenceRequestV3) (OperationScopeEvidenceQualificationFactV3, error)
	HandoffOperationScopeEvidenceV3(context.Context, HandoffOperationScopeEvidenceRequestV3) (OperationScopeEvidenceProviderHandoffFactV3, error)
	ConsumeOperationScopeEvidenceV3(context.Context, ConsumeOperationScopeEvidenceRequestV3) (OperationScopeEvidenceConsumeResultV3, error)
}

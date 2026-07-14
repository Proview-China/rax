package ports

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	EvidenceContractVersionV2 = "2.0.0"
	MaxEvidenceCausationRefs  = 64
	MaxEvidencePageSize       = 256
	MaxEvidenceDeclaredBytes  = 64 << 20
)

const EvidenceGenesisDigestV2 core.Digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

// EvidenceTrustClassV2 is Runtime-closed. A custom class may map to one of
// these values through a current source registration, but can never invent a
// new trust level.
type EvidenceTrustClassV2 string

const (
	EvidenceTrustObservation       EvidenceTrustClassV2 = "observation"
	EvidenceTrustReceipt           EvidenceTrustClassV2 = "receipt"
	EvidenceTrustAttestation       EvidenceTrustClassV2 = "attestation"
	EvidenceTrustClaim             EvidenceTrustClassV2 = "claim"
	EvidenceTrustAuthoritativeFact EvidenceTrustClassV2 = "authoritative_fact"
	EvidenceTrustLateObservation   EvidenceTrustClassV2 = "late_observation"
)

type EvidencePartitionV2 string

const (
	EvidencePartitionTenant   EvidencePartitionV2 = "tenant"
	EvidencePartitionIdentity EvidencePartitionV2 = "identity"
	EvidencePartitionLineage  EvidencePartitionV2 = "lineage"
	EvidencePartitionInstance EvidencePartitionV2 = "instance"
	EvidencePartitionRun      EvidencePartitionV2 = "run"
	EvidencePartitionEffect   EvidencePartitionV2 = "effect"
)

type EvidenceSourceStateV2 string

const (
	EvidenceSourceActive  EvidenceSourceStateV2 = "active"
	EvidenceSourceRevoked EvidenceSourceStateV2 = "revoked"
	EvidenceSourceExpired EvidenceSourceStateV2 = "expired"
)

type EvidenceGapPolicyV2 string

const EvidenceGapStrictV2 EvidenceGapPolicyV2 = "strict"

type EvidenceSourcePolicyStateV2 string

const (
	EvidenceSourcePolicyActive  EvidenceSourcePolicyStateV2 = "active"
	EvidenceSourcePolicyRevoked EvidenceSourcePolicyStateV2 = "revoked"
	EvidenceSourcePolicyExpired EvidenceSourcePolicyStateV2 = "expired"
)

type EvidenceSourcePolicyBindingRefV2 struct {
	Ref      string        `json:"ref"`
	Digest   core.Digest   `json:"digest"`
	Revision core.Revision `json:"revision"`
}

func (r EvidenceSourcePolicyBindingRefV2) Validate() error {
	if validateEvidenceIDV2(r.Ref) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "evidence source policy ref and revision are required")
	}
	return r.Digest.Validate()
}

type EvidenceOwnerFactRuleV2 struct {
	EventKind      NamespacedNameV2 `json:"event_kind"`
	CustomClass    NamespacedNameV2 `json:"custom_class"`
	FactKind       NamespacedNameV2 `json:"fact_kind"`
	OwnerComponent ComponentIDV2    `json:"owner_component"`
}
type EvidenceClaimKindMappingV2 struct {
	EventKind   NamespacedNameV2            `json:"event_kind"`
	CustomClass NamespacedNameV2            `json:"custom_class"`
	ClaimKind   core.RunCompletionClaimKind `json:"claim_kind"`
}
type EvidenceSourcePolicyFactV2 struct {
	Ref                  string                       `json:"ref"`
	Digest               core.Digest                  `json:"digest"`
	Revision             core.Revision                `json:"revision"`
	Producer             EvidenceProducerBindingRefV2 `json:"producer"`
	PolicyOwner          EvidenceProducerBindingRefV2 `json:"policy_owner"`
	PolicyAuthority      AuthorityBindingRefV2        `json:"policy_authority"`
	PolicyScope          core.ExecutionScope          `json:"policy_scope"`
	ActionScopeDigest    core.Digest                  `json:"action_scope_digest"`
	AllowedPartitions    []EvidencePartitionV2        `json:"allowed_partitions"`
	ClassMappings        []EvidenceClassMappingV2     `json:"class_mappings"`
	AllowedKinds         []NamespacedNameV2           `json:"allowed_kinds"`
	AllowLate            bool                         `json:"allow_late"`
	OwnerFactRules       []EvidenceOwnerFactRuleV2    `json:"owner_fact_rules"`
	ClaimKinds           []EvidenceClaimKindMappingV2 `json:"claim_kinds"`
	RequireInstanceEpoch bool                         `json:"require_instance_epoch"`
	MaximumSourceTTL     time.Duration                `json:"maximum_source_ttl"`
	State                EvidenceSourcePolicyStateV2  `json:"state"`
	ExpiresUnixNano      int64                        `json:"expires_unix_nano"`
}

func (f EvidenceSourcePolicyFactV2) Validate() error {
	if validateEvidenceIDV2(f.Ref) != nil || f.Revision == 0 || f.ExpiresUnixNano <= 0 || f.MaximumSourceTTL <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceTrustInvalid, "source policy identity, revision, TTL and expiry are required")
	}
	if f.Digest != "" {
		if err := f.Digest.Validate(); err != nil {
			return err
		}
	}
	if err := f.Producer.Validate(); err != nil {
		return err
	}
	if err := f.PolicyOwner.Validate(); err != nil {
		return err
	}
	if err := f.PolicyAuthority.Validate(); err != nil {
		return err
	}
	if err := f.PolicyScope.Validate(); err != nil {
		return err
	}
	if err := f.ActionScopeDigest.Validate(); err != nil {
		return err
	}
	if f.State != EvidenceSourcePolicyActive && f.State != EvidenceSourcePolicyRevoked && f.State != EvidenceSourcePolicyExpired {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "source policy state is unknown")
	}
	if len(f.AllowedPartitions) == 0 || len(f.AllowedPartitions) > 6 || len(f.ClassMappings) == 0 || len(f.ClassMappings) > 32 || len(f.AllowedKinds) == 0 || len(f.AllowedKinds) > 128 || len(f.OwnerFactRules) > 64 || len(f.ClaimKinds) > 16 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "source policy sets are empty or exceed bounds")
	}
	var previous string
	for index, value := range f.AllowedPartitions {
		switch value {
		case EvidencePartitionTenant, EvidencePartitionIdentity, EvidencePartitionLineage, EvidencePartitionInstance, EvidencePartitionRun, EvidencePartitionEffect:
		default:
			return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceScopeConflict, "source policy partition is unknown")
		}
		if index > 0 && string(value) <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "source policy partitions must be sorted and unique")
		}
		previous = string(value)
	}
	if err := validateEvidenceMappingsV2(f.ClassMappings); err != nil {
		return err
	}
	if err := validateEvidenceKindsV2(f.AllowedKinds); err != nil {
		return err
	}
	previous = ""
	for index, rule := range f.OwnerFactRules {
		if err := ValidateNamespacedNameV2(rule.EventKind); err != nil {
			return err
		}
		if err := ValidateNamespacedNameV2(rule.CustomClass); err != nil {
			return err
		}
		if err := ValidateNamespacedNameV2(rule.FactKind); err != nil {
			return err
		}
		if err := ValidateNamespacedNameV2(NamespacedNameV2(rule.OwnerComponent)); err != nil {
			return err
		}
		key := string(rule.EventKind) + "\x00" + string(rule.CustomClass) + "\x00" + string(rule.FactKind) + "\x00" + string(rule.OwnerComponent)
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "owner fact rules must be sorted and unique")
		}
		previous = key
	}
	previous = ""
	for index, mapping := range f.ClaimKinds {
		if err := ValidateNamespacedNameV2(mapping.EventKind); err != nil {
			return err
		}
		if !validEvidenceClaimKindV2(mapping.ClaimKind) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonRunClaimUnverified, "source policy claim kind is invalid")
		}
		if err := ValidateNamespacedNameV2(mapping.CustomClass); err != nil {
			return err
		}
		key := string(mapping.EventKind) + "\x00" + string(mapping.CustomClass)
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "claim mappings must be sorted and unique")
		}
		previous = key
	}
	for _, mapping := range f.ClaimKinds {
		if !containsEvidenceKindV2(f.AllowedKinds, mapping.EventKind) || !containsEvidenceTrustMappingV2(f.ClassMappings, mapping.CustomClass, EvidenceTrustClaim) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceTrustInvalid, "claim mapping must reference allowed kind and claim class")
		}
	}
	for _, rule := range f.OwnerFactRules {
		if !containsEvidenceKindV2(f.AllowedKinds, rule.EventKind) || !containsEvidenceTrustMappingV2(f.ClassMappings, rule.CustomClass, EvidenceTrustAuthoritativeFact) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceTrustInvalid, "owner fact rule must reference allowed kind and authoritative class")
		}
	}
	return nil
}

func (f EvidenceSourcePolicyFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	copy := f
	copy.Digest = ""
	if copy.AllowedPartitions == nil {
		copy.AllowedPartitions = []EvidencePartitionV2{}
	}
	if copy.ClassMappings == nil {
		copy.ClassMappings = []EvidenceClassMappingV2{}
	}
	if copy.AllowedKinds == nil {
		copy.AllowedKinds = []NamespacedNameV2{}
	}
	if copy.OwnerFactRules == nil {
		copy.OwnerFactRules = []EvidenceOwnerFactRuleV2{}
	}
	if copy.ClaimKinds == nil {
		copy.ClaimKinds = []EvidenceClaimKindMappingV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.evidence", EvidenceContractVersionV2, "EvidenceSourcePolicyFactV2", copy)
}

type EvidenceSourcePolicyReaderV2 interface {
	InspectEvidenceSourcePolicy(context.Context, string) (EvidenceSourcePolicyFactV2, error)
}

type EvidenceOwnerFactCurrentV2 struct {
	Fact              EvidenceOwnerFactRefV2 `json:"fact"`
	Authority         AuthorityBindingRefV2  `json:"authority"`
	Scope             core.ExecutionScope    `json:"scope"`
	ActionScopeDigest core.Digest            `json:"action_scope_digest"`
	Active            bool                   `json:"active"`
	ExpiresUnixNano   int64                  `json:"expires_unix_nano"`
}
type EvidenceOwnerFactReaderV2 interface {
	InspectEvidenceOwnerFact(context.Context, string) (EvidenceOwnerFactCurrentV2, error)
}

type EvidenceLedgerScopeV2 struct {
	Partition  EvidencePartitionV2    `json:"partition"`
	TenantID   core.TenantID          `json:"tenant_id"`
	IdentityID core.AgentIdentityID   `json:"identity_id"`
	LineageID  core.InstanceLineageID `json:"lineage_id,omitempty"`
	InstanceID core.AgentInstanceID   `json:"instance_id,omitempty"`
	RunID      core.AgentRunID        `json:"run_id,omitempty"`
	EffectID   core.EffectIntentID    `json:"effect_id,omitempty"`
}

func (s EvidenceLedgerScopeV2) Validate() error {
	if strings.TrimSpace(string(s.TenantID)) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceScopeConflict, "ledger scope requires tenant")
	}
	for _, value := range []string{string(s.TenantID), string(s.IdentityID), string(s.LineageID), string(s.InstanceID), string(s.RunID), string(s.EffectID)} {
		if value != "" {
			if err := validateEvidenceIDV2(value); err != nil {
				return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceScopeConflict, "ledger scope identifiers must be bounded stable ASCII")
			}
		}
	}
	switch s.Partition {
	case EvidencePartitionTenant:
		if s.IdentityID != "" || s.LineageID != "" || s.InstanceID != "" || s.RunID != "" || s.EffectID != "" {
			return evidenceScopeShapeError()
		}
	case EvidencePartitionIdentity:
		if s.IdentityID == "" || s.LineageID != "" || s.InstanceID != "" || s.RunID != "" || s.EffectID != "" {
			return evidenceScopeShapeError()
		}
	case EvidencePartitionLineage:
		if s.IdentityID == "" || s.LineageID == "" || s.InstanceID != "" || s.RunID != "" || s.EffectID != "" {
			return evidenceScopeShapeError()
		}
	case EvidencePartitionInstance:
		if s.IdentityID == "" || s.LineageID == "" || s.InstanceID == "" || s.RunID != "" || s.EffectID != "" {
			return evidenceScopeShapeError()
		}
	case EvidencePartitionRun:
		if s.IdentityID == "" || s.LineageID == "" || s.InstanceID == "" || s.RunID == "" || s.EffectID != "" {
			return evidenceScopeShapeError()
		}
	case EvidencePartitionEffect:
		if s.IdentityID == "" || s.LineageID == "" || s.InstanceID == "" || s.RunID == "" || s.EffectID == "" {
			return evidenceScopeShapeError()
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceScopeConflict, "ledger partition is unknown")
	}
	return nil
}

func evidenceScopeShapeError() error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceScopeConflict, "ledger partition hierarchy is inconsistent")
}

func (s EvidenceLedgerScopeV2) DigestV2() (core.Digest, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.evidence", EvidenceContractVersionV2, "EvidenceLedgerScopeV2", s)
}

func (s EvidenceLedgerScopeV2) MatchesExecutionScope(scope core.ExecutionScope) bool {
	return scope.Validate() == nil && s.TenantID == scope.Identity.TenantID && (s.IdentityID == "" || s.IdentityID == scope.Identity.ID) &&
		(s.LineageID == "" || s.LineageID == scope.Lineage.ID) && (s.InstanceID == "" || s.InstanceID == scope.Instance.ID)
}

// ValidateCurrentForEvidenceV2 validates the complete read-only projection
// watermark. It is not a second Instance/Run owner.
func (f ExecutionScopeCurrentFactV2) ValidateCurrentForEvidenceV2(partition EvidencePartitionV2, expected ExecutionScopeBindingRefV2, scope core.ExecutionScope, runID core.AgentRunID, capability core.Digest, now time.Time) error {
	if strings.TrimSpace(f.Ref) == "" || f.Revision == 0 || f.ProjectionWatermark == 0 || f.ExpiresUnixNano <= 0 || f.State != ExecutionScopeFactActive {
		return core.NewError(core.ErrorForbidden, core.ReasonEffectFenceStale, "active evidence execution projection and watermark are required")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if err := f.CapabilityGrantDigest.Validate(); err != nil {
		return err
	}
	for _, source := range []GovernanceSourceFactRefV2{f.ActivationSource, f.InstanceSource, f.AuthoritySource, f.BindingSource, f.RunSource} {
		if err := source.Validate(); err != nil {
			return err
		}
	}
	if f.Scope.SandboxLease == nil {
		if f.SandboxSource != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "sandbox source exists without lease")
		}
	} else if f.SandboxSource == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "sandbox lease lacks source watermark")
	} else if err := f.SandboxSource.Validate(); err != nil {
		return err
	}
	ref, err := f.BindingRefV2()
	if err != nil {
		return err
	}
	if f.Digest != ref.Digest || ref != expected || !EvidenceTimeCurrentV2(f.ExpiresUnixNano, now) || f.CapabilityGrantDigest != capability || !SameExecutionScopeV2(f.Scope, scope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "evidence execution projection drifted")
	}
	if f.ActiveRunID == "" || (f.RunState != string(core.RunRunning) && f.RunState != string(core.RunStopping)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "evidence projection requires an active running or stopping run")
	}
	if (partition == EvidencePartitionRun || partition == EvidencePartitionEffect) && f.ActiveRunID != runID {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "evidence run partition drifted from projection")
	}
	return nil
}

type EvidenceProducerBindingRefV2 struct {
	BindingSetID       string           `json:"binding_set_id"`
	BindingSetRevision core.Revision    `json:"binding_set_revision"`
	ComponentID        ComponentIDV2    `json:"component_id"`
	ManifestDigest     core.Digest      `json:"manifest_digest"`
	ArtifactDigest     core.Digest      `json:"artifact_digest"`
	Capability         CapabilityNameV2 `json:"capability"`
}

func (r EvidenceProducerBindingRefV2) Validate() error {
	return ProviderBindingRefV2(r).Validate()
}

type EvidenceOwnerFactRefV2 struct {
	Owner           EvidenceProducerBindingRefV2 `json:"owner"`
	FactKind        NamespacedNameV2             `json:"fact_kind"`
	FactID          string                       `json:"fact_id"`
	Revision        core.Revision                `json:"revision"`
	FactDigest      core.Digest                  `json:"fact_digest"`
	PayloadSchema   SchemaRefV2                  `json:"payload_schema"`
	PayloadDigest   core.Digest                  `json:"payload_digest"`
	PayloadRevision core.Revision                `json:"payload_revision"`
}

func (r EvidenceOwnerFactRefV2) Validate() error {
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(r.FactKind); err != nil {
		return err
	}
	if strings.TrimSpace(r.FactID) == "" || len(r.FactID) > 256 || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "authoritative owner fact identity and revision are required")
	}
	if err := r.FactDigest.Validate(); err != nil {
		return err
	}
	if err := r.PayloadSchema.Validate(); err != nil {
		return err
	}
	if err := r.PayloadDigest.Validate(); err != nil {
		return err
	}
	if r.PayloadRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "owner fact payload revision is required")
	}
	return nil
}

type EvidenceClassMappingV2 struct {
	Class NamespacedNameV2     `json:"class"`
	Trust EvidenceTrustClassV2 `json:"trust"`
}

type EvidenceCausationRefV2 struct {
	LedgerScopeDigest core.Digest `json:"ledger_scope_digest"`
	EventID           string      `json:"event_id"`
}

type EvidenceHistoricalSourceV2 struct {
	RegistrationID  string              `json:"registration_id"`
	SourceID        NamespacedNameV2    `json:"source_id"`
	SourceEpoch     core.Epoch          `json:"source_epoch"`
	SourceSequence  uint64              `json:"source_sequence"`
	ContentDigest   core.Digest         `json:"content_digest"`
	Record          EvidenceRecordRefV2 `json:"record"`
	CandidateDigest core.Digest         `json:"candidate_digest"`
}

type EvidenceSourceRegistrationFactV2 struct {
	ContractVersion       string                           `json:"contract_version"`
	ID                    string                           `json:"registration_id"`
	Revision              core.Revision                    `json:"revision"`
	SourceID              NamespacedNameV2                 `json:"source_id"`
	SourceEpoch           core.Epoch                       `json:"source_epoch"`
	LedgerScope           EvidenceLedgerScopeV2            `json:"ledger_scope"`
	ExecutionScope        core.ExecutionScope              `json:"execution_scope"`
	CurrentScope          ExecutionScopeBindingRefV2       `json:"current_scope_binding"`
	CurrentScopeWatermark core.Revision                    `json:"current_scope_watermark"`
	Producer              EvidenceProducerBindingRefV2     `json:"producer_binding"`
	Authority             AuthorityBindingRefV2            `json:"authority_binding"`
	ActionScopeDigest     core.Digest                      `json:"action_scope_digest"`
	Policy                EvidenceSourcePolicyBindingRefV2 `json:"source_policy_binding"`
	ClassMappings         []EvidenceClassMappingV2         `json:"class_mappings"`
	AllowedKinds          []NamespacedNameV2               `json:"allowed_kinds"`
	GapPolicy             EvidenceGapPolicyV2              `json:"gap_policy"`
	NextSourceSequence    uint64                           `json:"next_source_sequence"`
	State                 EvidenceSourceStateV2            `json:"state"`
	CreatedUnixNano       int64                            `json:"created_unix_nano"`
	UpdatedUnixNano       int64                            `json:"updated_unix_nano"`
	ExpiresUnixNano       int64                            `json:"expires_unix_nano"`
}

func (f EvidenceSourceRegistrationFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	if f.ClassMappings == nil {
		f.ClassMappings = []EvidenceClassMappingV2{}
	}
	if f.AllowedKinds == nil {
		f.AllowedKinds = []NamespacedNameV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.evidence", EvidenceContractVersionV2, "EvidenceSourceRegistrationFactV2", f)
}

// ConfigurationDigestV2 excludes the mutable journal cursor/lifecycle fields. Every
// record binds this digest plus the exact registration revision used by the
// atomic append.
func (f EvidenceSourceRegistrationFactV2) ConfigurationDigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	f.Revision, f.NextSourceSequence = 0, 0
	f.State, f.CreatedUnixNano, f.UpdatedUnixNano = "", 0, 0
	if f.ClassMappings == nil {
		f.ClassMappings = []EvidenceClassMappingV2{}
	}
	if f.AllowedKinds == nil {
		f.AllowedKinds = []NamespacedNameV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.evidence", EvidenceContractVersionV2, "EvidenceSourceConfigurationV2", f)
}

func (f EvidenceSourceRegistrationFactV2) Validate() error {
	if f.ContractVersion != EvidenceContractVersionV2 || validateEvidenceIDV2(f.ID) != nil || f.Revision == 0 || f.SourceEpoch == 0 || f.NextSourceSequence == 0 || f.NextSourceSequence == math.MaxUint64 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceSourceMissing, "source registration identity, epoch, revision and bounded next sequence are required")
	}
	if err := ValidateNamespacedNameV2(f.SourceID); err != nil {
		return err
	}
	if err := f.LedgerScope.Validate(); err != nil {
		return err
	}
	if err := f.ExecutionScope.Validate(); err != nil {
		return err
	}
	if !f.LedgerScope.MatchesExecutionScope(f.ExecutionScope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceScopeConflict, "source ledger and execution scopes differ")
	}
	if err := f.CurrentScope.Validate(); err != nil {
		return err
	}
	if f.CurrentScopeWatermark == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceSourceStale, "source current-scope watermark is required")
	}
	if err := f.Producer.Validate(); err != nil {
		return err
	}
	if err := f.Authority.Validate(); err != nil {
		return err
	}
	if err := f.ActionScopeDigest.Validate(); err != nil {
		return err
	}
	if err := f.Policy.Validate(); err != nil {
		return err
	}
	if f.GapPolicy != EvidenceGapStrictV2 || len(f.ClassMappings) == 0 || len(f.ClassMappings) > 32 || len(f.AllowedKinds) == 0 || len(f.AllowedKinds) > 128 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "source requires bounded class/kind sets and strict gap policy")
	}
	if err := validateEvidenceMappingsV2(f.ClassMappings); err != nil {
		return err
	}
	if err := validateEvidenceKindsV2(f.AllowedKinds); err != nil {
		return err
	}
	if f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano || f.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "source registration timestamps are inconsistent")
	}
	switch f.State {
	case EvidenceSourceActive:
		if f.ExpiresUnixNano <= f.UpdatedUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, "active source must expire after its update")
		}
	case EvidenceSourceExpired:
		if f.UpdatedUnixNano < f.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, "expired source must be updated at or after expiry")
		}
	case EvidenceSourceRevoked:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "source registration state is unknown")
	}
	return nil
}

type EvidencePayloadRefV2 struct {
	Schema        SchemaRefV2   `json:"schema"`
	ContentDigest core.Digest   `json:"content_digest"`
	Revision      core.Revision `json:"revision"`
	Length        uint64        `json:"length"`
	Ref           string        `json:"ref"`
}

func (p EvidencePayloadRefV2) Validate() error {
	if err := p.Schema.Validate(); err != nil {
		return err
	}
	if err := p.ContentDigest.Validate(); err != nil {
		return err
	}
	if p.Revision == 0 || p.Length == 0 || p.Length > MaxEvidenceDeclaredBytes || strings.TrimSpace(p.Ref) == "" || len(p.Ref) > MaxOpaqueReferenceBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "evidence payload reference is incomplete or exceeds limits")
	}
	return nil
}

type EvidenceEventCandidateV2 struct {
	ContractVersion           string                           `json:"contract_version"`
	LedgerScope               EvidenceLedgerScopeV2            `json:"ledger_scope"`
	EventID                   string                           `json:"event_id"`
	RegistrationID            string                           `json:"registration_id"`
	RegistrationRevision      core.Revision                    `json:"registration_revision"`
	SourceConfigurationDigest core.Digest                      `json:"source_configuration_digest"`
	SourcePolicy              EvidenceSourcePolicyBindingRefV2 `json:"source_policy_binding"`
	SourceID                  NamespacedNameV2                 `json:"source_id"`
	SourceEpoch               core.Epoch                       `json:"source_epoch"`
	SourceSequence            uint64                           `json:"source_sequence"`
	TrustClass                EvidenceTrustClassV2             `json:"trust_class"`
	ClaimKind                 core.RunCompletionClaimKind      `json:"claim_kind,omitempty"`
	EventKind                 NamespacedNameV2                 `json:"event_kind"`
	CustomClass               NamespacedNameV2                 `json:"custom_class"`
	ExecutionScope            core.ExecutionScope              `json:"execution_scope"`
	Payload                   EvidencePayloadRefV2             `json:"payload"`
	Causation                 []EvidenceCausationRefV2         `json:"causation"`
	CorrelationID             string                           `json:"correlation_id"`
	Producer                  EvidenceProducerBindingRefV2     `json:"producer_binding"`
	Authority                 AuthorityBindingRefV2            `json:"authority_binding"`
	OwnerFact                 *EvidenceOwnerFactRefV2          `json:"owner_fact,omitempty"`
	HistoricalSource          *EvidenceHistoricalSourceV2      `json:"historical_source,omitempty"`
	ObservedUnixNano          int64                            `json:"observed_unix_nano"`
}

func (e EvidenceEventCandidateV2) DigestV2() (core.Digest, error) {
	if err := e.Validate(); err != nil {
		return "", err
	}
	if e.Causation == nil {
		e.Causation = []EvidenceCausationRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.evidence", EvidenceContractVersionV2, "EvidenceEventCandidateV2", e)
}

func (e EvidenceEventCandidateV2) Validate() error {
	if e.ContractVersion != EvidenceContractVersionV2 || validateEvidenceIDV2(e.EventID) != nil || validateEvidenceIDV2(e.RegistrationID) != nil || e.RegistrationRevision == 0 || e.SourceEpoch == 0 || e.SourceSequence == 0 || e.ObservedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceUnavailable, "evidence event identity, source sequence and observed time are required")
	}
	if err := e.SourceConfigurationDigest.Validate(); err != nil {
		return err
	}
	if err := e.SourcePolicy.Validate(); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(e.SourceID); err != nil {
		return err
	}
	if err := validateEvidenceIDV2(e.CorrelationID); err != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "bounded correlation id is required")
	}
	if err := e.LedgerScope.Validate(); err != nil {
		return err
	}
	if err := e.ExecutionScope.Validate(); err != nil {
		return err
	}
	if !e.LedgerScope.MatchesExecutionScope(e.ExecutionScope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceScopeConflict, "event ledger and execution scopes differ")
	}
	if err := ValidateNamespacedNameV2(e.EventKind); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(e.CustomClass); err != nil {
		return err
	}
	if err := e.Payload.Validate(); err != nil {
		return err
	}
	if err := e.Producer.Validate(); err != nil {
		return err
	}
	if err := e.Authority.Validate(); err != nil {
		return err
	}
	if err := validateEvidenceTrustV2(e.TrustClass); err != nil {
		return err
	}
	if e.TrustClass == EvidenceTrustClaim {
		if !validEvidenceClaimKindV2(e.ClaimKind) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonRunClaimUnverified, "claim evidence requires a closed claim kind")
		}
	} else if e.ClaimKind != "" {
		return core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "non-claim evidence cannot carry claim kind")
	}
	if len(e.Causation) > MaxEvidenceCausationRefs {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "causation set exceeds its bound")
	}
	var previous string
	for index, ref := range e.Causation {
		if ref.LedgerScopeDigest.Validate() != nil || validateEvidenceIDV2(ref.EventID) != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "causation ref is incomplete")
		}
		key := string(ref.LedgerScopeDigest) + "\x00" + ref.EventID
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "causation set must be sorted and unique")
		}
		previous = key
	}
	if e.TrustClass == EvidenceTrustAuthoritativeFact {
		if e.OwnerFact == nil {
			return core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "authoritative evidence requires an exact owner fact")
		}
		if err := e.OwnerFact.Validate(); err != nil {
			return err
		}
		if e.OwnerFact.PayloadSchema.Key() != e.Payload.Schema.Key() || e.OwnerFact.PayloadDigest != e.Payload.ContentDigest || e.OwnerFact.PayloadRevision != e.Payload.Revision {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceTrustInvalid, "authoritative payload does not match the inspected owner fact")
		}
	} else if e.OwnerFact != nil {
		return core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "non-authoritative evidence cannot carry an owner fact")
	}
	if e.TrustClass == EvidenceTrustLateObservation {
		if e.LedgerScope.Partition == EvidencePartitionRun || e.LedgerScope.Partition == EvidencePartitionEffect || e.HistoricalSource == nil {
			return core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "late observations require historical source and cannot be run/effect evidence")
		}
		if validateEvidenceIDV2(e.HistoricalSource.RegistrationID) != nil || ValidateNamespacedNameV2(e.HistoricalSource.SourceID) != nil || e.HistoricalSource.SourceEpoch == 0 || e.HistoricalSource.SourceEpoch >= e.SourceEpoch || e.HistoricalSource.SourceSequence == 0 || e.HistoricalSource.ContentDigest.Validate() != nil || e.HistoricalSource.ContentDigest != e.Payload.ContentDigest || e.HistoricalSource.Record.Validate() != nil || e.HistoricalSource.CandidateDigest.Validate() != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceSourceStale, "historical source identity is incomplete")
		}
	} else if e.HistoricalSource != nil {
		return core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "current evidence cannot carry a historical source")
	}
	return nil
}

type EvidenceRecordRefV2 struct {
	LedgerScopeDigest core.Digest `json:"ledger_scope_digest"`
	Sequence          uint64      `json:"sequence"`
	RecordDigest      core.Digest `json:"record_digest"`
}

func (r EvidenceRecordRefV2) Validate() error {
	if r.LedgerScopeDigest.Validate() != nil || r.RecordDigest.Validate() != nil || r.Sequence == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "complete evidence record ref is required")
	}
	return nil
}

type EvidenceSourceKeyV2 struct {
	RegistrationID string     `json:"registration_id"`
	SourceEpoch    core.Epoch `json:"source_epoch"`
	SourceSequence uint64     `json:"source_sequence"`
}

func (k EvidenceSourceKeyV2) Validate() error {
	if validateEvidenceIDV2(k.RegistrationID) != nil || k.SourceEpoch == 0 || k.SourceSequence == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceSourceMissing, "complete source key is required")
	}
	return nil
}

type EvidenceLedgerRecordV2 struct {
	Ref                  EvidenceRecordRefV2      `json:"ref"`
	Candidate            EvidenceEventCandidateV2 `json:"candidate"`
	CandidateDigest      core.Digest              `json:"candidate_digest"`
	PreviousRecordDigest core.Digest              `json:"previous_record_digest,omitempty"`
	IngestedUnixNano     int64                    `json:"ingested_unix_nano"`
}

func (r EvidenceLedgerRecordV2) Validate() error {
	if err := r.Ref.Validate(); err != nil {
		return err
	}
	if r.CandidateDigest.Validate() != nil || r.PreviousRecordDigest.Validate() != nil || r.IngestedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceChainConflict, "ledger record chain fields are incomplete")
	}
	return r.Candidate.Validate()
}

// EvidenceTombstoneFactV2 is a separate retention fact. It never mutates the
// chained record or its digest/source/causation identity.
type EvidenceTombstoneFactV2 struct {
	Record          EvidenceRecordRefV2      `json:"record"`
	Source          EvidenceSourceKeyV2      `json:"source"`
	Causation       []EvidenceCausationRefV2 `json:"causation"`
	Reason          NamespacedNameV2         `json:"reason"`
	Revision        core.Revision            `json:"revision"`
	CreatedUnixNano int64                    `json:"created_unix_nano"`
}

func (f EvidenceTombstoneFactV2) Validate() error {
	if err := f.Record.Validate(); err != nil {
		return err
	}
	if err := f.Source.Validate(); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(f.Reason); err != nil {
		return err
	}
	if f.Revision != 1 || f.CreatedUnixNano <= 0 || len(f.Causation) > MaxEvidenceCausationRefs {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "append-only tombstone requires revision one, time and bounded causation")
	}
	var previous string
	for index, ref := range f.Causation {
		if ref.LedgerScopeDigest.Validate() != nil || validateEvidenceIDV2(ref.EventID) != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "tombstone causation is incomplete")
		}
		key := string(ref.LedgerScopeDigest) + "\x00" + ref.EventID
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "tombstone causation must be sorted and unique")
		}
		previous = key
	}
	return nil
}

func (f EvidenceTombstoneFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	if f.Causation == nil {
		f.Causation = []EvidenceCausationRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.evidence", EvidenceContractVersionV2, "EvidenceTombstoneFactV2", f)
}

type EvidenceSourceCASRequestV2 struct {
	ExpectedRevision core.Revision                    `json:"expected_revision"`
	Next             EvidenceSourceRegistrationFactV2 `json:"next"`
}
type EvidenceAppendRequestV2 struct {
	Candidate              EvidenceEventCandidateV2 `json:"candidate"`
	ExpectedSourceRevision core.Revision            `json:"expected_source_revision"`
}
type EvidenceAppendLateRequestV2 struct {
	Candidate              EvidenceEventCandidateV2 `json:"candidate"`
	ExpectedSourceRevision core.Revision            `json:"expected_source_revision"`
}
type EvidenceWatchCursorV2 struct {
	LedgerScopeDigest core.Digest `json:"ledger_scope_digest"`
	AfterSequence     uint64      `json:"after_sequence"`
}
type EvidencePageV2 struct {
	Records []EvidenceLedgerRecordV2 `json:"records"`
	Next    EvidenceWatchCursorV2    `json:"next"`
}

// EvidenceLedgerFactPortV2 is the raw linearizable Fact Owner. Applications
// use a governance gateway; registration and append methods here are atomic
// persistence primitives, not authority grants.
type EvidenceLedgerFactPortV2 interface {
	CreateSource(context.Context, EvidenceSourceRegistrationFactV2) (EvidenceSourceRegistrationFactV2, error)
	InspectSource(context.Context, string) (EvidenceSourceRegistrationFactV2, error)
	CompareAndSwapSource(context.Context, EvidenceSourceCASRequestV2) (EvidenceSourceRegistrationFactV2, error)
	Append(context.Context, EvidenceAppendRequestV2) (EvidenceLedgerRecordV2, error)
	AppendLateObservation(context.Context, EvidenceAppendLateRequestV2) (EvidenceLedgerRecordV2, error)
	InspectBySource(context.Context, EvidenceSourceKeyV2) (EvidenceLedgerRecordV2, error)
	InspectRecord(context.Context, EvidenceRecordRefV2) (EvidenceLedgerRecordV2, error)
	Watch(context.Context, EvidenceWatchCursorV2, uint32) (EvidencePageV2, error)
	CreateTombstone(context.Context, EvidenceTombstoneFactV2) (EvidenceTombstoneFactV2, error)
	InspectTombstone(context.Context, EvidenceRecordRefV2) (EvidenceTombstoneFactV2, error)
}

// EvidenceRecordReaderV2 is the narrow immutable inspection seam used by
// settlement/review consumers. It intentionally exposes no Ledger mutation.
type EvidenceRecordReaderV2 interface {
	InspectRecord(context.Context, EvidenceRecordRefV2) (EvidenceLedgerRecordV2, error)
}

// EvidenceSourceRecordReaderV2 is the narrow immutable reader used when a
// consumer must prove that a record ref and a source key describe the same
// ledger record. It exposes no append authority.
type EvidenceSourceRecordReaderV2 interface {
	InspectRecord(context.Context, EvidenceRecordRefV2) (EvidenceLedgerRecordV2, error)
	InspectBySource(context.Context, EvidenceSourceKeyV2) (EvidenceLedgerRecordV2, error)
}

// EvidenceGovernancePortV2 is the only Application-facing append surface.
// Its distinct method names prevent the raw Fact Owner from satisfying it by
// accident.
type EvidenceGovernancePortV2 interface {
	RegisterGovernedSource(context.Context, EvidenceSourceRegistrationFactV2) (EvidenceSourceRegistrationFactV2, error)
	RenewGovernedSource(context.Context, EvidenceSourceCASRequestV2) (EvidenceSourceRegistrationFactV2, error)
	AppendGoverned(context.Context, EvidenceAppendRequestV2) (EvidenceLedgerRecordV2, error)
	AppendLateGoverned(context.Context, EvidenceAppendLateRequestV2) (EvidenceLedgerRecordV2, error)
	InspectGovernedBySource(context.Context, EvidenceSourceKeyV2) (EvidenceLedgerRecordV2, error)
	InspectGovernedRecord(context.Context, EvidenceRecordRefV2) (EvidenceLedgerRecordV2, error)
}

func validateEvidenceTrustV2(value EvidenceTrustClassV2) error {
	switch value {
	case EvidenceTrustObservation, EvidenceTrustReceipt, EvidenceTrustAttestation, EvidenceTrustClaim, EvidenceTrustAuthoritativeFact, EvidenceTrustLateObservation:
		return nil
	}
	return core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "evidence trust class is not Runtime-defined")
}

func validEvidenceClaimKindV2(kind core.RunCompletionClaimKind) bool {
	switch kind {
	case core.RunClaimCompleted, core.RunClaimCancelled, core.RunClaimFailed, core.RunClaimIndeterminate:
		return true
	}
	return false
}

func validateEvidenceMappingsV2(values []EvidenceClassMappingV2) error {
	var previous string
	for index, value := range values {
		if ValidateNamespacedNameV2(value.Class) != nil || validateEvidenceTrustV2(value.Trust) != nil || value.Trust == EvidenceTrustLateObservation {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceTrustInvalid, "source class mapping is invalid")
		}
		key := string(value.Class)
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "source class mappings must be sorted and unique")
		}
		previous = key
	}
	return nil
}

func validateEvidenceKindsV2(values []NamespacedNameV2) error {
	var previous string
	for index, value := range values {
		if err := ValidateNamespacedNameV2(value); err != nil {
			return err
		}
		key := string(value)
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "event kinds must be sorted and unique")
		}
		previous = key
	}
	return nil
}
func containsEvidenceKindV2(values []NamespacedNameV2, value NamespacedNameV2) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
func containsEvidenceTrustMappingV2(values []EvidenceClassMappingV2, class NamespacedNameV2, trust EvidenceTrustClassV2) bool {
	for _, candidate := range values {
		if candidate.Class == class && candidate.Trust == trust {
			return true
		}
	}
	return false
}

func validateEvidenceIDV2(value string) error {
	if strings.TrimSpace(value) != value || len(value) == 0 || len(value) > 256 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "identifier must be non-empty, trimmed and bounded")
	}
	for _, character := range []byte(value) {
		if character < 0x21 || character > 0x7e {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "identifier must use stable printable ASCII")
		}
	}
	return nil
}

func EvidenceTimeCurrentV2(expires int64, now time.Time) bool {
	return !now.IsZero() && now.Before(time.Unix(0, expires))
}

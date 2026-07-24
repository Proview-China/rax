package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	RestoreStageSettlementContractVersionV1                  = "1.0.0"
	RestoreStageDomainResultKindV1          NamespacedNameV2 = "praxis.sandbox/workspace-restore-stage-fact"
)

// RestoreStageDomainResultFactRefV1 is the neutral Runtime view of the
// Sandbox-owned authoritative Stage Fact. Runtime owns none of its semantics.
type RestoreStageDomainResultFactRefV1 struct {
	Owner             ProviderBindingRefV2          `json:"owner"`
	Kind              NamespacedNameV2              `json:"kind"`
	ID                string                        `json:"id"`
	Revision          core.Revision                 `json:"revision"`
	Digest            core.Digest                   `json:"digest"`
	TenantID          core.TenantID                 `json:"tenant_id"`
	Operation         OperationSubjectV3            `json:"operation"`
	OperationDigest   core.Digest                   `json:"operation_digest"`
	EffectID          core.EffectIntentID           `json:"effect_id"`
	EffectRevision    core.Revision                 `json:"effect_revision"`
	Attempt           OperationDispatchAttemptRefV3 `json:"dispatch_attempt"`
	RestoreAttempt    RestoreAttemptRefV2           `json:"restore_attempt"`
	Eligibility       RestoreEligibilityRefV2       `json:"restore_eligibility"`
	PayloadSchema     SchemaRefV2                   `json:"payload_schema"`
	PayloadDigest     core.Digest                   `json:"payload_digest"`
	PayloadRevision   core.Revision                 `json:"payload_revision"`
	AuthoritativeTime int64                         `json:"authoritative_unix_nano"`
}

func (r RestoreStageDomainResultFactRefV1) Validate() error {
	if r.Owner.Validate() != nil || r.Kind != RestoreStageDomainResultKindV1 || validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || r.Digest.Validate() != nil || validateEvidenceIDV2(string(r.TenantID)) != nil || r.Operation.Validate() != nil || r.OperationDigest.Validate() != nil || validateEvidenceIDV2(string(r.EffectID)) != nil || r.EffectRevision == 0 || r.Attempt.Validate() != nil || r.RestoreAttempt.Validate() != nil || r.Eligibility.Validate() != nil || r.PayloadSchema.Validate() != nil || r.PayloadDigest.Validate() != nil || r.PayloadRevision == 0 || r.AuthoritativeTime <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "Restore Stage DomainResult ref is incomplete")
	}
	digest, err := r.Operation.DigestV3()
	if err != nil || digest != r.OperationDigest || r.Operation.Kind != RestoreStageOperationKindV1 || r.Operation.CustomOperationID != r.RestoreAttempt.ID || r.Operation.ExecutionScope.Identity.TenantID != r.TenantID || r.RestoreAttempt.TenantID != r.TenantID || r.Eligibility.TenantID != r.TenantID || r.Attempt.OperationDigest != r.OperationDigest || r.Attempt.EffectID != r.EffectID || r.Attempt.IntentRevision != r.EffectRevision {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Restore Stage DomainResult ref mixes operation, attempt or tenant")
	}
	return nil
}

func SameRestoreStageDomainResultFactRefV1(left, right RestoreStageDomainResultFactRefV1) bool {
	ld, le := core.CanonicalJSONDigest("praxis.runtime.restore-stage-settlement", RestoreStageSettlementContractVersionV1, "RestoreStageDomainResultFactRefV1", left)
	rd, re := core.CanonicalJSONDigest("praxis.runtime.restore-stage-settlement", RestoreStageSettlementContractVersionV1, "RestoreStageDomainResultFactRefV1", right)
	return le == nil && re == nil && ld == rd
}

func (r RestoreStageDomainResultFactRefV1) EvidenceOwnerFactV2() EvidenceOwnerFactRefV2 {
	return EvidenceOwnerFactRefV2{Owner: EvidenceProducerBindingRefV2(r.Owner), FactKind: r.Kind, FactID: r.ID, Revision: r.Revision, FactDigest: r.Digest, PayloadSchema: r.PayloadSchema, PayloadDigest: r.PayloadDigest, PayloadRevision: r.PayloadRevision}
}

type RestoreStageDomainResultCurrentProjectionV1 struct {
	ContractVersion  string                            `json:"contract_version"`
	Fact             RestoreStageDomainResultFactRefV1 `json:"fact"`
	CheckedUnixNano  int64                             `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                             `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                       `json:"projection_digest"`
}

func (p RestoreStageDomainResultCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.restore-stage-settlement", RestoreStageSettlementContractVersionV1, "RestoreStageDomainResultCurrentProjectionV1", copy)
}

func SealRestoreStageDomainResultCurrentProjectionV1(p RestoreStageDomainResultCurrentProjectionV1, now time.Time) (RestoreStageDomainResultCurrentProjectionV1, error) {
	p.ContractVersion = RestoreStageSettlementContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return RestoreStageDomainResultCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

func (p RestoreStageDomainResultCurrentProjectionV1) Validate(now time.Time) error {
	if p.ContractVersion != RestoreStageSettlementContractVersionV1 || p.Fact.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || now.UnixNano() >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonSettlementOwnerMismatch, "Restore Stage DomainResult current projection is incomplete or stale")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Stage DomainResult current projection drifted")
	}
	return nil
}

type RestoreStageDomainResultCurrentReaderV1 interface {
	InspectRestoreStageDomainResultCurrentV1(context.Context, RestoreStageDomainResultFactRefV1) (RestoreStageDomainResultCurrentProjectionV1, error)
}

type RestoreStageSettlementSubmissionV1 struct {
	ContractVersion string                                    `json:"contract_version"`
	ID              string                                    `json:"id"`
	Revision        core.Revision                             `json:"revision"`
	Operation       OperationSubjectV3                        `json:"operation"`
	OperationDigest core.Digest                               `json:"operation_digest"`
	EffectID        core.EffectIntentID                       `json:"effect_id"`
	EffectRevision  core.Revision                             `json:"effect_revision"`
	RestoreAttempt  RestoreAttemptRefV2                       `json:"restore_attempt"`
	Eligibility     RestoreEligibilityRefV2                   `json:"restore_eligibility"`
	Governance      RestoreStageGovernanceCurrentProjectionV1 `json:"governance"`
	DomainResult    RestoreStageDomainResultFactRefV1         `json:"domain_result"`
	Evidence        EvidenceRecordRefV2                       `json:"evidence"`
	IdempotencyKey  string                                    `json:"idempotency_key"`
	SettledUnixNano int64                                     `json:"settled_unix_nano"`
	Digest          core.Digest                               `json:"digest"`
}

func (s RestoreStageSettlementSubmissionV1) DigestV1() (core.Digest, error) {
	copy := s
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.restore-stage-settlement", RestoreStageSettlementContractVersionV1, "RestoreStageSettlementSubmissionV1", copy)
}

func SealRestoreStageSettlementSubmissionV1(s RestoreStageSettlementSubmissionV1) (RestoreStageSettlementSubmissionV1, error) {
	s.ContractVersion = RestoreStageSettlementContractVersionV1
	s.Revision = 1
	s.Digest = ""
	digest, err := s.DigestV1()
	if err != nil {
		return RestoreStageSettlementSubmissionV1{}, err
	}
	s.Digest = digest
	return s, s.Validate()
}

func (s RestoreStageSettlementSubmissionV1) Validate() error {
	if s.ContractVersion != RestoreStageSettlementContractVersionV1 || validateEvidenceIDV2(s.ID) != nil || s.Revision != 1 || s.Operation.Validate() != nil || s.OperationDigest.Validate() != nil || validateEvidenceIDV2(string(s.EffectID)) != nil || s.EffectRevision == 0 || s.RestoreAttempt.Validate() != nil || s.Eligibility.Validate() != nil || s.Governance.Validate(time.Unix(0, s.SettledUnixNano)) != nil || s.DomainResult.Validate() != nil || s.Evidence.Validate() != nil || validateEvidenceIDV2(s.IdempotencyKey) != nil || s.SettledUnixNano <= 0 || s.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "Restore Stage settlement submission is incomplete")
	}
	opDigest, err := s.Operation.DigestV3()
	if err != nil || opDigest != s.OperationDigest || !SameOperationSubjectV3(s.Operation, s.DomainResult.Operation) || !SameOperationSubjectV3(s.Operation, s.Governance.Operation) || s.DomainResult.OperationDigest != s.OperationDigest || s.DomainResult.EffectID != s.EffectID || s.DomainResult.EffectRevision != s.EffectRevision || s.DomainResult.RestoreAttempt != s.RestoreAttempt || s.DomainResult.Eligibility != s.Eligibility || s.Governance.EffectID != s.EffectID || s.Governance.EffectRevision != s.EffectRevision || s.Governance.RestoreAttempt != s.RestoreAttempt || s.Governance.Eligibility != s.Eligibility || s.Governance.DispatchAttempt != s.DomainResult.Attempt {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Restore Stage settlement exact closure drifted")
	}
	digest, err := s.DigestV1()
	if err != nil || digest != s.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Stage settlement submission digest drifted")
	}
	return nil
}

type RestoreStageSettlementRefV1 struct {
	ID              string                            `json:"id"`
	Revision        core.Revision                     `json:"revision"`
	Digest          core.Digest                       `json:"digest"`
	OperationDigest core.Digest                       `json:"operation_digest"`
	EffectID        core.EffectIntentID               `json:"effect_id"`
	DomainResult    RestoreStageDomainResultFactRefV1 `json:"domain_result"`
}

func (r RestoreStageSettlementRefV1) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil || r.OperationDigest.Validate() != nil || validateEvidenceIDV2(string(r.EffectID)) != nil || r.DomainResult.Validate() != nil || r.DomainResult.OperationDigest != r.OperationDigest || r.DomainResult.EffectID != r.EffectID {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "Restore Stage settlement ref is incomplete")
	}
	return nil
}

// SameRestoreStageSettlementRefV1 compares semantic exact identity. It must
// not use Go struct equality because the embedded ExecutionScope contains an
// optional lease pointer whose address changes after persistence round trips.
func SameRestoreStageSettlementRefV1(left, right RestoreStageSettlementRefV1) bool {
	return left.ID == right.ID && left.Revision == right.Revision && left.Digest == right.Digest && left.OperationDigest == right.OperationDigest && left.EffectID == right.EffectID && SameRestoreStageDomainResultFactRefV1(left.DomainResult, right.DomainResult)
}

type RestoreStageSettlementFactV1 struct {
	ContractVersion string                             `json:"contract_version"`
	Submission      RestoreStageSettlementSubmissionV1 `json:"submission"`
	Revision        core.Revision                      `json:"revision"`
	Digest          core.Digest                        `json:"digest"`
}

func (f RestoreStageSettlementFactV1) DigestV1() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.restore-stage-settlement", RestoreStageSettlementContractVersionV1, "RestoreStageSettlementFactV1", copy)
}

func SealRestoreStageSettlementFactV1(f RestoreStageSettlementFactV1) (RestoreStageSettlementFactV1, error) {
	f.ContractVersion = RestoreStageSettlementContractVersionV1
	f.Revision = 1
	f.Digest = ""
	digest, err := f.DigestV1()
	if err != nil {
		return RestoreStageSettlementFactV1{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

func (f RestoreStageSettlementFactV1) Validate() error {
	if f.ContractVersion != RestoreStageSettlementContractVersionV1 || f.Submission.Validate() != nil || f.Revision != 1 || f.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "Restore Stage settlement fact is incomplete")
	}
	digest, err := f.DigestV1()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Stage settlement fact drifted")
	}
	return nil
}

func (f RestoreStageSettlementFactV1) RefV1() RestoreStageSettlementRefV1 {
	return RestoreStageSettlementRefV1{ID: f.Submission.ID, Revision: f.Revision, Digest: f.Digest, OperationDigest: f.Submission.OperationDigest, EffectID: f.Submission.EffectID, DomainResult: f.Submission.DomainResult}
}

type RestoreStageSettlementFactPortV1 interface {
	CreateRestoreStageSettlementV1(context.Context, RestoreStageSettlementFactV1) (RestoreStageSettlementFactV1, error)
	InspectRestoreStageSettlementV1(context.Context, string) (RestoreStageSettlementFactV1, error)
	InspectRestoreStageSettlementByEffectV1(context.Context, OperationSubjectV3, core.EffectIntentID) (RestoreStageSettlementFactV1, error)
}

type RestoreStageSettlementGovernancePortV1 interface {
	SettleRestoreStageV1(context.Context, RestoreStageSettlementSubmissionV1) (RestoreStageSettlementRefV1, error)
	InspectRestoreStageSettlementV1(context.Context, string) (RestoreStageSettlementFactV1, error)
	InspectCurrentRestoreStageSettlementV1(context.Context, OperationSubjectV3, core.EffectIntentID) (RestoreStageSettlementRefV1, error)
}

func ValidateRestoreStageEvidenceRecordV1(record EvidenceLedgerRecordV2, domain RestoreStageDomainResultFactRefV1) error {
	if record.Validate() != nil || domain.Validate() != nil || record.Candidate.TrustClass != EvidenceTrustAuthoritativeFact || record.Candidate.OwnerFact == nil || *record.Candidate.OwnerFact != domain.EvidenceOwnerFactV2() || record.Candidate.LedgerScope.Partition != EvidencePartitionInstance || record.Candidate.LedgerScope.TenantID != domain.TenantID || !record.Candidate.LedgerScope.MatchesExecutionScope(domain.Operation.ExecutionScope) || !SameExecutionScopeV2(record.Candidate.ExecutionScope, domain.Operation.ExecutionScope) || strings.TrimSpace(record.Candidate.Payload.Ref) == "" {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Restore Stage Evidence record does not prove the exact Sandbox DomainResult")
	}
	return nil
}

package ports

import (
	"context"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const OperationSettlementContractVersionV4 = "4.0.0"

type OperationSettlementDomainResultFactRefV4 struct {
	Owner             ProviderBindingRefV2          `json:"owner"`
	Kind              NamespacedNameV2              `json:"kind"`
	ID                string                        `json:"id"`
	Revision          core.Revision                 `json:"revision"`
	Digest            core.Digest                   `json:"digest"`
	TenantID          core.TenantID                 `json:"tenant_id"`
	EffectID          core.EffectIntentID           `json:"effect_id"`
	EffectRevision    core.Revision                 `json:"effect_revision"`
	Operation         OperationSubjectV3            `json:"operation"`
	OperationDigest   core.Digest                   `json:"operation_digest"`
	Attempt           OperationDispatchAttemptRefV3 `json:"attempt"`
	Schema            SchemaRefV2                   `json:"schema"`
	PayloadDigest     core.Digest                   `json:"payload_digest"`
	PayloadRevision   core.Revision                 `json:"payload_revision"`
	AuthoritativeTime int64                         `json:"authoritative_unix_nano"`
}

func SameOperationSettlementDomainResultFactRefV4(left, right OperationSettlementDomainResultFactRefV4) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementDomainResultFactRefV4", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementDomainResultFactRefV4", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func (r OperationSettlementDomainResultFactRefV4) Validate() error {
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	if ValidateNamespacedNameV2(r.Kind) != nil || validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || validateEvidenceIDV2(string(r.TenantID)) != nil || validateEvidenceIDV2(string(r.EffectID)) != nil || r.EffectRevision == 0 || r.PayloadRevision == 0 || r.AuthoritativeTime <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "V4 DomainResult fact ref identity is incomplete")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	if err := r.OperationDigest.Validate(); err != nil {
		return err
	}
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	operationDigest, err := r.Operation.DigestV3()
	if err != nil || operationDigest != r.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "V4 DomainResult operation identity drifted")
	}
	if err := r.Attempt.Validate(); err != nil {
		return err
	}
	if err := r.Schema.Validate(); err != nil {
		return err
	}
	if err := r.PayloadDigest.Validate(); err != nil {
		return err
	}
	if r.Attempt.OperationDigest != r.OperationDigest || r.Attempt.EffectID != r.EffectID || r.Attempt.IntentRevision != r.EffectRevision {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "V4 DomainResult fact belongs to another operation attempt")
	}
	return nil
}

type OperationSettlementDomainResultCurrentV4 struct {
	ContractVersion string                                   `json:"contract_version"`
	EffectKind      EffectKindV2                             `json:"effect_kind"`
	Fact            OperationSettlementDomainResultFactRefV4 `json:"fact"`
	CheckedUnixNano int64                                    `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                    `json:"expires_unix_nano"`
	Digest          core.Digest                              `json:"digest"`
}

func (p OperationSettlementDomainResultCurrentV4) Validate(now time.Time) error {
	if p.ContractVersion != OperationSettlementContractVersionV4 || ValidateNamespacedNameV2(NamespacedNameV2(p.EffectKind)) != nil || p.Fact.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.IsZero() || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonSettlementOwnerMismatch, "V4 DomainResult current projection is incomplete or expired")
	}
	digest, err := p.DigestV4()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V4 DomainResult current projection digest drifted")
	}
	return nil
}

func (p OperationSettlementDomainResultCurrentV4) DigestV4() (core.Digest, error) {
	copy := p
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementDomainResultCurrentV4", copy)
}

func SealOperationSettlementDomainResultCurrentV4(p OperationSettlementDomainResultCurrentV4, now time.Time) (OperationSettlementDomainResultCurrentV4, error) {
	p.ContractVersion = OperationSettlementContractVersionV4
	p.Digest = ""
	digest, err := p.DigestV4()
	if err != nil {
		return OperationSettlementDomainResultCurrentV4{}, err
	}
	p.Digest = digest
	return p, p.Validate(now)
}

type OperationSettlementDomainResultCurrentReaderV4 interface {
	InspectOperationSettlementDomainResultCurrentV4(context.Context, EffectKindV2, OperationSettlementDomainResultFactRefV4) (OperationSettlementDomainResultCurrentV4, error)
}

type OperationSettlementEvidenceBindingV4 struct {
	Phase                OperationDispatchEnforcementPhaseV4        `json:"phase"`
	Consumption          OperationScopeEvidenceConsumptionRefV3     `json:"consumption"`
	IssuedQualification  OperationScopeEvidenceQualificationRefV3   `json:"issued_qualification"`
	FinalQualification   OperationScopeEvidenceQualificationRefV3   `json:"final_qualification"`
	Record               OperationScopeEvidenceRecordRefV3          `json:"record"`
	CandidateDigest      core.Digest                                `json:"candidate_digest"`
	Handoff              OperationScopeEvidenceProviderHandoffRefV3 `json:"handoff"`
	Attempt              OperationDispatchAttemptRefV3              `json:"attempt"`
	EnforcementPhase     OperationDispatchEnforcementPhaseRefV4     `json:"enforcement_phase"`
	OperationScopeDigest core.Digest                                `json:"operation_scope_digest"`
}

func (b OperationSettlementEvidenceBindingV4) Validate() error {
	if b.Phase != OperationDispatchEnforcementPrepareV4 && b.Phase != OperationDispatchEnforcementExecuteV4 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceConflict, "V4 settlement Evidence phase is invalid")
	}
	if err := b.Consumption.Validate(); err != nil {
		return err
	}
	if err := b.IssuedQualification.Validate(); err != nil {
		return err
	}
	if err := b.FinalQualification.Validate(); err != nil {
		return err
	}
	if err := b.Record.Validate(); err != nil {
		return err
	}
	if err := b.CandidateDigest.Validate(); err != nil {
		return err
	}
	if err := b.Handoff.Validate(); err != nil {
		return err
	}
	if err := b.Attempt.Validate(); err != nil {
		return err
	}
	if err := b.EnforcementPhase.Validate(); err != nil {
		return err
	}
	if err := b.OperationScopeDigest.Validate(); err != nil {
		return err
	}
	if b.IssuedQualification.ID != b.FinalQualification.ID || b.FinalQualification.Revision != b.IssuedQualification.Revision+1 || b.Consumption.Record != b.Record || b.EnforcementPhase.Phase != b.Phase || b.EnforcementPhase.AttemptID != b.Attempt.AttemptID || b.EnforcementPhase.OperationDigest != b.Attempt.OperationDigest || b.EnforcementPhase.EffectID != b.Attempt.EffectID || b.EnforcementPhase.PermitID != b.Attempt.PermitID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 settlement Evidence binding combines different facts")
	}
	return nil
}

func (b OperationSettlementEvidenceBindingV4) DigestV4() (core.Digest, error) {
	if err := b.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementEvidenceBindingV4", b)
}

func operationSettlementPhaseRankV4(phase OperationDispatchEnforcementPhaseV4) int {
	switch phase {
	case OperationDispatchEnforcementPrepareV4:
		return 0
	case OperationDispatchEnforcementExecuteV4:
		return 1
	default:
		return 2
	}
}

func DigestOperationSettlementEvidenceScopeV4(scope OperationScopeEvidenceScopeV3) (core.Digest, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	scope.Applicability = NormalizeOperationScopeEvidenceApplicabilityV3(scope.Applicability)
	return core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceScopeV3", scope)
}

func DigestOperationSettlementScopeSetV4(evidence []OperationSettlementEvidenceBindingV4) (core.Digest, error) {
	if len(evidence) != 2 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceScopeConflict, "V4 settlement requires two phase scope digests")
	}
	values := append([]OperationSettlementEvidenceBindingV4{}, evidence...)
	sort.Slice(values, func(i, j int) bool {
		return operationSettlementPhaseRankV4(values[i].Phase) < operationSettlementPhaseRankV4(values[j].Phase)
	})
	type phaseScope struct {
		Phase  OperationDispatchEnforcementPhaseV4 `json:"phase"`
		Digest core.Digest                         `json:"digest"`
	}
	set := make([]phaseScope, len(values))
	for index := range values {
		if err := values[index].OperationScopeDigest.Validate(); err != nil {
			return "", err
		}
		set[index] = phaseScope{Phase: values[index].Phase, Digest: values[index].OperationScopeDigest}
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementScopeSetV4", set)
}

type OperationSettlementSubmissionV4 struct {
	ContractVersion               string                                   `json:"contract_version"`
	ID                            string                                   `json:"id"`
	Revision                      core.Revision                            `json:"revision"`
	TenantID                      core.TenantID                            `json:"tenant_id"`
	Operation                     OperationSubjectV3                       `json:"operation"`
	OperationDigest               core.Digest                              `json:"operation_digest"`
	OperationScopeDigest          core.Digest                              `json:"operation_scope_digest"`
	EffectID                      core.EffectIntentID                      `json:"effect_id"`
	ExpectedEffectRevision        core.Revision                            `json:"expected_effect_revision"`
	Owner                         EffectOwnerRefV2                         `json:"owner"`
	DomainResult                  OperationSettlementDomainResultFactRefV4 `json:"domain_result"`
	Evidence                      []OperationSettlementEvidenceBindingV4   `json:"evidence"`
	ExpectedTerminalGuardRevision core.Revision                            `json:"expected_terminal_guard_revision"`
	IdempotencyKey                string                                   `json:"idempotency_key"`
	ConflictDomain                core.Digest                              `json:"conflict_domain"`
	SettledUnixNano               int64                                    `json:"settled_unix_nano"`
	Digest                        core.Digest                              `json:"digest"`
}

func (s OperationSettlementSubmissionV4) Validate() error {
	if s.ContractVersion != OperationSettlementContractVersionV4 || validateEvidenceIDV2(s.ID) != nil || s.Revision != 1 || validateEvidenceIDV2(string(s.TenantID)) != nil || validateEvidenceIDV2(string(s.EffectID)) != nil || s.ExpectedEffectRevision == 0 || validateEvidenceIDV2(s.IdempotencyKey) != nil || s.SettledUnixNano <= 0 || len(s.Evidence) != 2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "V4 settlement submission identity or phase set is incomplete")
	}
	if err := s.Operation.Validate(); err != nil {
		return err
	}
	operationDigest, err := s.Operation.DigestV3()
	if err != nil || operationDigest != s.OperationDigest || s.Operation.ExecutionScope.Identity.TenantID != s.TenantID {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "V4 settlement operation identity drifted")
	}
	if err := s.OperationScopeDigest.Validate(); err != nil {
		return err
	}
	if s.Owner.Role != OwnerSettlement || ValidateNamespacedNameV2(NamespacedNameV2(s.Owner.ComponentID)) != nil || s.Owner.ManifestDigest.Validate() != nil {
		return core.NewError(core.ErrorForbidden, core.ReasonSettlementOwnerMismatch, "V4 settlement requires an exact Settlement Owner")
	}
	if err := s.DomainResult.Validate(); err != nil {
		return err
	}
	if s.DomainResult.TenantID != s.TenantID || s.DomainResult.EffectID != s.EffectID || s.DomainResult.OperationDigest != s.OperationDigest || !SameOperationSubjectV3(s.DomainResult.Operation, s.Operation) {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "V4 DomainResult belongs to another operation")
	}
	for index := range s.Evidence {
		if err := s.Evidence[index].Validate(); err != nil {
			return err
		}
		if index > 0 && operationSettlementPhaseRankV4(s.Evidence[index-1].Phase) >= operationSettlementPhaseRankV4(s.Evidence[index].Phase) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "V4 settlement Evidence phases must be sorted and unique")
		}
		if s.Evidence[index].Attempt.OperationDigest != s.OperationDigest || s.Evidence[index].Attempt.EffectID != s.EffectID || !sameOperationDispatchAttemptRefPublicV3(s.Evidence[index].Attempt, s.DomainResult.Attempt) {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 settlement Evidence belongs to another operation attempt")
		}
	}
	if s.Evidence[0].Phase != OperationDispatchEnforcementPrepareV4 || s.Evidence[1].Phase != OperationDispatchEnforcementExecuteV4 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "V4 settlement requires exactly prepare and execute Evidence")
	}
	scopeSetDigest, err := DigestOperationSettlementScopeSetV4(s.Evidence)
	if err != nil || scopeSetDigest != s.OperationScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceScopeConflict, "V4 settlement phase scope set drifted")
	}
	if err := s.ConflictDomain.Validate(); err != nil {
		return err
	}
	digest, err := s.DigestV4()
	if err != nil || digest != s.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V4 settlement submission digest drifted")
	}
	return nil
}

func (s OperationSettlementSubmissionV4) DigestV4() (core.Digest, error) {
	copy := s
	copy.Digest = ""
	if copy.Evidence == nil {
		copy.Evidence = []OperationSettlementEvidenceBindingV4{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementSubmissionV4", copy)
}

func SealOperationSettlementSubmissionV4(s OperationSettlementSubmissionV4) (OperationSettlementSubmissionV4, error) {
	s.ContractVersion = OperationSettlementContractVersionV4
	s.Revision = 1
	s.Evidence = append([]OperationSettlementEvidenceBindingV4{}, s.Evidence...)
	sort.Slice(s.Evidence, func(i, j int) bool {
		return operationSettlementPhaseRankV4(s.Evidence[i].Phase) < operationSettlementPhaseRankV4(s.Evidence[j].Phase)
	})
	s.Digest = ""
	digest, err := s.DigestV4()
	if err != nil {
		return OperationSettlementSubmissionV4{}, err
	}
	s.Digest = digest
	return s, s.Validate()
}

type OperationSettlementRefV4 struct {
	ID              string                                   `json:"id"`
	Revision        core.Revision                            `json:"revision"`
	Digest          core.Digest                              `json:"digest"`
	OperationDigest core.Digest                              `json:"operation_digest"`
	EffectID        core.EffectIntentID                      `json:"effect_id"`
	DomainResult    OperationSettlementDomainResultFactRefV4 `json:"domain_result"`
}

func SameOperationSettlementRefV4(left, right OperationSettlementRefV4) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementRefV4", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementRefV4", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func (r OperationSettlementRefV4) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision != 1 || validateEvidenceIDV2(string(r.EffectID)) != nil || r.Digest.Validate() != nil || r.OperationDigest.Validate() != nil || r.DomainResult.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "V4 settlement ref is incomplete")
	}
	if r.DomainResult.EffectID != r.EffectID || r.DomainResult.OperationDigest != r.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "V4 settlement ref combines different operations")
	}
	return nil
}

type OperationSettlementFactV4 struct {
	ContractVersion string                          `json:"contract_version"`
	Submission      OperationSettlementSubmissionV4 `json:"submission"`
	Revision        core.Revision                   `json:"revision"`
	Digest          core.Digest                     `json:"digest"`
}

func (f OperationSettlementFactV4) Validate() error {
	if f.ContractVersion != OperationSettlementContractVersionV4 || f.Submission.Validate() != nil || f.Revision != 1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "V4 settlement fact is incomplete")
	}
	digest, err := f.DigestV4()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V4 settlement fact digest drifted")
	}
	return nil
}

func (f OperationSettlementFactV4) DigestV4() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementFactV4", copy)
}

func SealOperationSettlementFactV4(f OperationSettlementFactV4) (OperationSettlementFactV4, error) {
	f.ContractVersion = OperationSettlementContractVersionV4
	f.Revision = 1
	f.Digest = ""
	digest, err := f.DigestV4()
	if err != nil {
		return OperationSettlementFactV4{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

func (f OperationSettlementFactV4) RefV4() OperationSettlementRefV4 {
	return OperationSettlementRefV4{ID: f.Submission.ID, Revision: f.Revision, Digest: f.Digest, OperationDigest: f.Submission.OperationDigest, EffectID: f.Submission.EffectID, DomainResult: f.Submission.DomainResult}
}

type OperationSettlementEvidenceAssociationV4 struct {
	ContractVersion string                               `json:"contract_version"`
	ID              string                               `json:"id"`
	Revision        core.Revision                        `json:"revision"`
	Settlement      OperationSettlementRefV4             `json:"settlement"`
	Prepare         OperationSettlementEvidenceBindingV4 `json:"prepare"`
	Execute         OperationSettlementEvidenceBindingV4 `json:"execute"`
	Digest          core.Digest                          `json:"digest"`
}

func (a OperationSettlementEvidenceAssociationV4) Validate() error {
	if a.ContractVersion != OperationSettlementContractVersionV4 || validateEvidenceIDV2(a.ID) != nil || a.Revision != 1 || a.Settlement.Validate() != nil || a.Prepare.Validate() != nil || a.Execute.Validate() != nil || a.Prepare.Phase != OperationDispatchEnforcementPrepareV4 || a.Execute.Phase != OperationDispatchEnforcementExecuteV4 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceConflict, "V4 settlement Evidence association is incomplete")
	}
	digest, err := a.DigestV4()
	if err != nil || digest != a.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V4 settlement Evidence association digest drifted")
	}
	return nil
}

func (a OperationSettlementEvidenceAssociationV4) DigestV4() (core.Digest, error) {
	copy := a
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementEvidenceAssociationV4", copy)
}

func SealOperationSettlementEvidenceAssociationV4(a OperationSettlementEvidenceAssociationV4) (OperationSettlementEvidenceAssociationV4, error) {
	a.ContractVersion = OperationSettlementContractVersionV4
	a.Revision = 1
	a.Digest = ""
	digest, err := a.DigestV4()
	if err != nil {
		return OperationSettlementEvidenceAssociationV4{}, err
	}
	a.Digest = digest
	return a, a.Validate()
}

type OperationSettlementEvidenceAssociationRefV4 struct {
	ID              string                   `json:"id"`
	Revision        core.Revision            `json:"revision"`
	Digest          core.Digest              `json:"digest"`
	Settlement      OperationSettlementRefV4 `json:"settlement"`
	OperationDigest core.Digest              `json:"operation_digest"`
	EffectID        core.EffectIntentID      `json:"effect_id"`
}

func SameOperationSettlementEvidenceAssociationRefV4(left, right OperationSettlementEvidenceAssociationRefV4) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementEvidenceAssociationRefV4", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementEvidenceAssociationRefV4", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func (r OperationSettlementEvidenceAssociationRefV4) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil || r.Settlement.Validate() != nil || r.OperationDigest.Validate() != nil || validateEvidenceIDV2(string(r.EffectID)) != nil || r.Settlement.OperationDigest != r.OperationDigest || r.Settlement.EffectID != r.EffectID {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceConflict, "V4 settlement Evidence association ref is incomplete")
	}
	return nil
}

func (a OperationSettlementEvidenceAssociationV4) RefV4() OperationSettlementEvidenceAssociationRefV4 {
	return OperationSettlementEvidenceAssociationRefV4{ID: a.ID, Revision: a.Revision, Digest: a.Digest, Settlement: a.Settlement, OperationDigest: a.Settlement.OperationDigest, EffectID: a.Settlement.EffectID}
}

type OperationSettlementTerminalGuardV4 struct {
	ContractVersion string                   `json:"contract_version"`
	ID              string                   `json:"id"`
	TenantID        core.TenantID            `json:"tenant_id"`
	OperationDigest core.Digest              `json:"operation_digest"`
	EffectID        core.EffectIntentID      `json:"effect_id"`
	Revision        core.Revision            `json:"revision"`
	Settlement      OperationSettlementRefV4 `json:"settlement"`
	Digest          core.Digest              `json:"digest"`
}

func (g OperationSettlementTerminalGuardV4) Validate() error {
	if g.ContractVersion != OperationSettlementContractVersionV4 || validateEvidenceIDV2(g.ID) != nil || validateEvidenceIDV2(string(g.TenantID)) != nil || g.Revision != 1 || g.OperationDigest.Validate() != nil || validateEvidenceIDV2(string(g.EffectID)) != nil || g.Settlement.Validate() != nil || g.Settlement.OperationDigest != g.OperationDigest || g.Settlement.EffectID != g.EffectID || g.Settlement.DomainResult.TenantID != g.TenantID {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectStateConflict, "V4 terminal guard is incomplete")
	}
	d, e := g.DigestV4()
	if e != nil || d != g.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V4 terminal guard digest drifted")
	}
	return nil
}

type OperationSettlementTerminalGuardRefV4 struct {
	ID              string                   `json:"id"`
	TenantID        core.TenantID            `json:"tenant_id"`
	EffectID        core.EffectIntentID      `json:"effect_id"`
	OperationDigest core.Digest              `json:"operation_digest"`
	Revision        core.Revision            `json:"revision"`
	Digest          core.Digest              `json:"digest"`
	Settlement      OperationSettlementRefV4 `json:"settlement"`
}

func SameOperationSettlementTerminalGuardRefV4(left, right OperationSettlementTerminalGuardRefV4) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementTerminalGuardRefV4", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementTerminalGuardRefV4", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func (r OperationSettlementTerminalGuardRefV4) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || validateEvidenceIDV2(string(r.TenantID)) != nil || validateEvidenceIDV2(string(r.EffectID)) != nil || r.OperationDigest.Validate() != nil || r.Revision != 1 || r.Digest.Validate() != nil || r.Settlement.Validate() != nil || r.Settlement.EffectID != r.EffectID || r.Settlement.OperationDigest != r.OperationDigest || r.Settlement.DomainResult.TenantID != r.TenantID {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectStateConflict, "V4 terminal guard ref is incomplete")
	}
	return nil
}

func (g OperationSettlementTerminalGuardV4) RefV4() OperationSettlementTerminalGuardRefV4 {
	return OperationSettlementTerminalGuardRefV4{ID: g.ID, TenantID: g.TenantID, EffectID: g.EffectID, OperationDigest: g.OperationDigest, Revision: g.Revision, Digest: g.Digest, Settlement: g.Settlement}
}
func (g OperationSettlementTerminalGuardV4) DigestV4() (core.Digest, error) {
	copy := g
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementTerminalGuardV4", copy)
}

func SealOperationSettlementTerminalGuardV4(g OperationSettlementTerminalGuardV4) (OperationSettlementTerminalGuardV4, error) {
	g.ContractVersion = OperationSettlementContractVersionV4
	g.Revision = 1
	g.Digest = ""
	digest, err := g.DigestV4()
	if err != nil {
		return OperationSettlementTerminalGuardV4{}, err
	}
	g.Digest = digest
	return g, g.Validate()
}

type OperationSettlementTerminalProjectionV4 struct {
	ContractVersion string                                      `json:"contract_version"`
	ID              string                                      `json:"id"`
	Revision        core.Revision                               `json:"revision"`
	TenantID        core.TenantID                               `json:"tenant_id"`
	OperationDigest core.Digest                                 `json:"operation_digest"`
	EffectID        core.EffectIntentID                         `json:"effect_id"`
	Settlement      OperationSettlementRefV4                    `json:"settlement"`
	Association     OperationSettlementEvidenceAssociationRefV4 `json:"association"`
	Guard           OperationSettlementTerminalGuardRefV4       `json:"guard"`
	DomainResult    OperationSettlementDomainResultFactRefV4    `json:"domain_result"`
	Digest          core.Digest                                 `json:"digest"`
}

func (p OperationSettlementTerminalProjectionV4) Validate() error {
	if p.ContractVersion != OperationSettlementContractVersionV4 || validateEvidenceIDV2(p.ID) != nil || p.Revision != 1 || validateEvidenceIDV2(string(p.TenantID)) != nil || p.OperationDigest.Validate() != nil || validateEvidenceIDV2(string(p.EffectID)) != nil || p.Settlement.Validate() != nil || p.Association.Validate() != nil || p.Guard.Validate() != nil || p.DomainResult.Validate() != nil || p.Settlement.OperationDigest != p.OperationDigest || p.Settlement.EffectID != p.EffectID || !SameOperationSettlementDomainResultFactRefV4(p.Settlement.DomainResult, p.DomainResult) || p.DomainResult.TenantID != p.TenantID || !SameOperationSettlementRefV4(p.Association.Settlement, p.Settlement) || !SameOperationSettlementRefV4(p.Guard.Settlement, p.Settlement) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "V4 terminal projection is incomplete")
	}
	d, e := p.DigestV4()
	if e != nil || d != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V4 terminal projection digest drifted")
	}
	return nil
}
func (p OperationSettlementTerminalProjectionV4) DigestV4() (core.Digest, error) {
	copy := p
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementTerminalProjectionV4", copy)
}

func SealOperationSettlementTerminalProjectionV4(p OperationSettlementTerminalProjectionV4) (OperationSettlementTerminalProjectionV4, error) {
	p.ContractVersion = OperationSettlementContractVersionV4
	p.Revision = 1
	p.Digest = ""
	digest, err := p.DigestV4()
	if err != nil {
		return OperationSettlementTerminalProjectionV4{}, err
	}
	p.Digest = digest
	return p, p.Validate()
}

type OperationSettlementTerminalProjectionRefV4 struct {
	ID              string                                      `json:"id"`
	Revision        core.Revision                               `json:"revision"`
	Digest          core.Digest                                 `json:"digest"`
	TenantID        core.TenantID                               `json:"tenant_id"`
	OperationDigest core.Digest                                 `json:"operation_digest"`
	EffectID        core.EffectIntentID                         `json:"effect_id"`
	Settlement      OperationSettlementRefV4                    `json:"settlement"`
	Association     OperationSettlementEvidenceAssociationRefV4 `json:"association"`
	Guard           OperationSettlementTerminalGuardRefV4       `json:"guard"`
}

func SameOperationSettlementTerminalProjectionRefV4(left, right OperationSettlementTerminalProjectionRefV4) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementTerminalProjectionRefV4", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationSettlementTerminalProjectionRefV4", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func (r OperationSettlementTerminalProjectionRefV4) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil || validateEvidenceIDV2(string(r.TenantID)) != nil || r.OperationDigest.Validate() != nil || validateEvidenceIDV2(string(r.EffectID)) != nil || r.Settlement.Validate() != nil || r.Association.Validate() != nil || r.Guard.Validate() != nil || r.Settlement.OperationDigest != r.OperationDigest || r.Settlement.EffectID != r.EffectID || r.Settlement.DomainResult.TenantID != r.TenantID || !SameOperationSettlementRefV4(r.Association.Settlement, r.Settlement) || !SameOperationSettlementRefV4(r.Guard.Settlement, r.Settlement) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "V4 terminal projection ref is incomplete")
	}
	return nil
}

func (p OperationSettlementTerminalProjectionV4) RefV4() OperationSettlementTerminalProjectionRefV4 {
	return OperationSettlementTerminalProjectionRefV4{ID: p.ID, Revision: p.Revision, Digest: p.Digest, TenantID: p.TenantID, OperationDigest: p.OperationDigest, EffectID: p.EffectID, Settlement: p.Settlement, Association: p.Association, Guard: p.Guard}
}

type OperationSettlementCommitBundleV4 struct {
	Settlement  OperationSettlementFactV4                `json:"settlement"`
	Association OperationSettlementEvidenceAssociationV4 `json:"association"`
	Guard       OperationSettlementTerminalGuardV4       `json:"guard"`
	Projection  OperationSettlementTerminalProjectionV4  `json:"projection"`
}

func (b OperationSettlementCommitBundleV4) Validate() error {
	if b.Settlement.Validate() != nil || b.Association.Validate() != nil || b.Guard.Validate() != nil || b.Projection.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "V4 settlement commit bundle is invalid")
	}
	ref := b.Settlement.RefV4()
	if !SameOperationSettlementRefV4(b.Association.Settlement, ref) || !SameOperationSettlementRefV4(b.Guard.Settlement, ref) || !SameOperationSettlementRefV4(b.Projection.Settlement, ref) || !SameOperationSettlementEvidenceAssociationRefV4(b.Projection.Association, b.Association.RefV4()) || !SameOperationSettlementTerminalGuardRefV4(b.Projection.Guard, b.Guard.RefV4()) {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "V4 settlement commit bundle is not exact")
	}
	prepareDigest, prepareErr := b.Association.Prepare.DigestV4()
	executeDigest, executeErr := b.Association.Execute.DigestV4()
	submissionPrepareDigest, submissionPrepareErr := b.Settlement.Submission.Evidence[0].DigestV4()
	submissionExecuteDigest, submissionExecuteErr := b.Settlement.Submission.Evidence[1].DigestV4()
	if prepareErr != nil || executeErr != nil || submissionPrepareErr != nil || submissionExecuteErr != nil || prepareDigest != submissionPrepareDigest || executeDigest != submissionExecuteDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 settlement association does not bind the submitted phase Evidence")
	}
	return nil
}

type OperationInspectionSettlementRefV4 struct {
	Settlement         OperationSettlementRefV4                    `json:"settlement"`
	Association        OperationSettlementEvidenceAssociationRefV4 `json:"association"`
	Guard              OperationSettlementTerminalGuardRefV4       `json:"guard"`
	Projection         OperationSettlementTerminalProjectionRefV4  `json:"projection"`
	DomainResult       OperationSettlementDomainResultFactRefV4    `json:"domain_result"`
	EffectFactRevision core.Revision                               `json:"effect_fact_revision"`
	Owner              EffectOwnerRefV2                            `json:"owner"`
	CheckedUnixNano    int64                                       `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                                       `json:"expires_unix_nano"`
	Digest             core.Digest                                 `json:"digest"`
}

func (r OperationInspectionSettlementRefV4) Validate(now time.Time) error {
	if r.Settlement.Validate() != nil || r.Association.Validate() != nil || r.Guard.Validate() != nil || r.Projection.Validate() != nil || r.DomainResult.Validate() != nil || r.EffectFactRevision == 0 || r.Owner.Role != OwnerSettlement || r.Owner.ManifestDigest.Validate() != nil || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano || now.IsZero() || !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonSettlementOwnerMismatch, "V4 current settlement inspection is incomplete or expired")
	}
	if !SameOperationSettlementRefV4(r.Association.Settlement, r.Settlement) || !SameOperationSettlementRefV4(r.Guard.Settlement, r.Settlement) || !SameOperationSettlementRefV4(r.Projection.Settlement, r.Settlement) || !SameOperationSettlementEvidenceAssociationRefV4(r.Projection.Association, r.Association) || !SameOperationSettlementTerminalGuardRefV4(r.Projection.Guard, r.Guard) || !SameOperationSettlementDomainResultFactRefV4(r.Settlement.DomainResult, r.DomainResult) {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "V4 current settlement inspection combines different terminal facts")
	}
	d, e := r.DigestV4()
	if e != nil || d != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "V4 current settlement inspection digest drifted")
	}
	return nil
}
func (r OperationInspectionSettlementRefV4) DigestV4() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-settlement", OperationSettlementContractVersionV4, "OperationInspectionSettlementRefV4", copy)
}

func SealOperationInspectionSettlementRefV4(r OperationInspectionSettlementRefV4, now time.Time) (OperationInspectionSettlementRefV4, error) {
	r.Digest = ""
	digest, err := r.DigestV4()
	if err != nil {
		return OperationInspectionSettlementRefV4{}, err
	}
	r.Digest = digest
	return r, r.Validate(now)
}

type OperationSettlementCommitRequestV4 struct {
	ExpectedEffectRevision core.Revision                     `json:"expected_effect_revision"`
	Bundle                 OperationSettlementCommitBundleV4 `json:"bundle"`
}

func (r OperationSettlementCommitRequestV4) Validate() error {
	if r.ExpectedEffectRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "V4 settlement commit expected Effect revision is required")
	}
	return r.Bundle.Validate()
}

type OperationSettlementFactPortV4 interface {
	CommitOperationSettlementV4(context.Context, OperationSettlementCommitRequestV4) (OperationSettlementCommitBundleV4, error)
	InspectOperationSettlementV4(context.Context, OperationSubjectV3, string) (OperationSettlementCommitBundleV4, error)
	InspectOperationSettlementByEffectV4(context.Context, OperationSubjectV3, core.EffectIntentID) (OperationSettlementCommitBundleV4, error)
	InspectOperationSettlementEvidenceAssociationV4(context.Context, OperationSubjectV3, OperationSettlementEvidenceAssociationRefV4) (OperationSettlementEvidenceAssociationV4, error)
	InspectOperationSettlementTerminalGuardV4(context.Context, OperationSubjectV3, OperationSettlementTerminalGuardRefV4) (OperationSettlementTerminalGuardV4, error)
	InspectOperationSettlementTerminalProjectionV4(context.Context, OperationSubjectV3, OperationSettlementTerminalProjectionRefV4) (OperationSettlementTerminalProjectionV4, error)
}

type OperationSettlementEvidenceReaderV4 interface {
	InspectOperationScopeEvidenceQualificationV3(context.Context, string) (OperationScopeEvidenceQualificationFactV3, error)
	InspectOperationScopeEvidenceProviderHandoffV3(context.Context, string) (OperationScopeEvidenceProviderHandoffFactV3, error)
	InspectOperationScopeEvidenceConsumptionV3(context.Context, string) (OperationScopeEvidenceConsumptionFactV3, error)
	InspectOperationScopeEvidenceRecordV3(context.Context, OperationScopeEvidenceRecordRefV3) (OperationScopeEvidenceRecordV3, error)
}

type OperationSettlementEnforcementReaderV4 interface {
	InspectOperationDispatchEnforcementV4(context.Context, OperationSubjectV3, core.EffectIntentID, string) (OperationDispatchEnforcementJournalV4, error)
}

type InspectOperationSettlementRequestV4 struct {
	Operation    OperationSubjectV3 `json:"operation"`
	SettlementID string             `json:"settlement_id"`
}

func (r InspectOperationSettlementRequestV4) Validate() error {
	if r.Operation.Validate() != nil || validateEvidenceIDV2(r.SettlementID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "V4 settlement Inspect key is invalid")
	}
	return nil
}

type InspectCurrentOperationSettlementRequestV4 struct {
	Operation OperationSubjectV3  `json:"operation"`
	EffectID  core.EffectIntentID `json:"effect_id"`
}

func (r InspectCurrentOperationSettlementRequestV4) Validate() error {
	if r.Operation.Validate() != nil || validateEvidenceIDV2(string(r.EffectID)) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "V4 current settlement Inspect key is invalid")
	}
	return nil
}

type OperationSettlementGovernancePortV4 interface {
	SettleOperationV4(context.Context, OperationSettlementSubmissionV4) (OperationSettlementRefV4, error)
	InspectOperationSettlementV4(context.Context, InspectOperationSettlementRequestV4) (OperationSettlementFactV4, error)
	InspectOperationSettlementClosureV4(context.Context, InspectOperationSettlementRequestV4) (OperationSettlementCommitBundleV4, error)
	InspectCurrentOperationSettlementV4(context.Context, InspectCurrentOperationSettlementRequestV4) (OperationInspectionSettlementRefV4, error)
	InspectOperationSettlementEvidenceAssociationV4(context.Context, OperationSubjectV3, OperationSettlementEvidenceAssociationRefV4) (OperationSettlementEvidenceAssociationV4, error)
	InspectOperationSettlementTerminalGuardV4(context.Context, OperationSubjectV3, OperationSettlementTerminalGuardRefV4) (OperationSettlementTerminalGuardV4, error)
	InspectOperationSettlementTerminalProjectionV4(context.Context, OperationSubjectV3, OperationSettlementTerminalProjectionRefV4) (OperationSettlementTerminalProjectionV4, error)
}

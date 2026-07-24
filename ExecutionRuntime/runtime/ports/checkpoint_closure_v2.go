package ports

import (
	"context"
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type EffectCutEntryDispositionV2 string

const (
	EffectCutSettledV2             EffectCutEntryDispositionV2 = "settled"
	EffectCutConfirmedNotAppliedV2 EffectCutEntryDispositionV2 = "confirmed_not_applied"
	EffectCutUnknownV2             EffectCutEntryDispositionV2 = "unknown"
	EffectCutExcludedByPolicyV2    EffectCutEntryDispositionV2 = "excluded_by_policy"
)

type RuntimeOperationTerminalKindV2 string

const (
	RuntimeTerminalOperationSettlementV3V2  RuntimeOperationTerminalKindV2 = "operation_settlement_v3"
	RuntimeTerminalOperationSettlementV4V2  RuntimeOperationTerminalKindV2 = "operation_settlement_v4"
	RuntimeTerminalCheckpointSettlementV5V2 RuntimeOperationTerminalKindV2 = "checkpoint_settlement_v5"
	RuntimeTerminalDispatchUnknownV3V2      RuntimeOperationTerminalKindV2 = "dispatch_unknown_v3"
	RuntimeTerminalPolicyExclusionV2        RuntimeOperationTerminalKindV2 = "checkpoint_policy_exclusion_v2"
)

type CheckpointEffectPolicyExclusionRefV2 struct {
	ID       string                        `json:"exclusion_id"`
	Revision core.Revision                 `json:"revision"`
	EffectID core.EffectIntentID           `json:"effect_id"`
	Attempt  OperationDispatchAttemptRefV3 `json:"attempt"`
	Policy   CheckpointBarrierPolicyRefV2  `json:"policy"`
	Digest   core.Digest                   `json:"digest"`
}

func (r CheckpointEffectPolicyExclusionRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision != 1 || r.EffectID == "" || r.Attempt.Validate() != nil || r.Policy.Validate() != nil || r.Attempt.EffectID != r.EffectID || r.Digest.Validate() != nil {
		return checkpointInvalidV2("checkpoint Effect policy exclusion ref is incomplete")
	}
	digest, err := r.DigestV2()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Effect policy exclusion digest drifted")
	}
	return nil
}

func (r CheckpointEffectPolicyExclusionRefV2) DigestV2() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return checkpointDigestV2("CheckpointEffectPolicyExclusionRefV2", copy)
}

// RuntimeOperationTerminalRefV2 is a closed tagged union. A free-form kind or
// a structurally valid ref in the wrong slot cannot be elevated into an Effect
// Cut terminal.
type RuntimeOperationTerminalRefV2 struct {
	Kind                   RuntimeOperationTerminalKindV2             `json:"kind"`
	OperationSettlementV3  *OperationSettlementRefV3                  `json:"operation_settlement_v3,omitempty"`
	OperationSettlementV4  *OperationSettlementRefV4                  `json:"operation_settlement_v4,omitempty"`
	CheckpointSettlementV5 *OperationCheckpointRestoreSettlementRefV5 `json:"checkpoint_settlement_v5,omitempty"`
	DispatchUnknownV3      *OperationDispatchAttemptRefV3             `json:"dispatch_unknown_v3,omitempty"`
	PolicyExclusion        *CheckpointEffectPolicyExclusionRefV2      `json:"policy_exclusion,omitempty"`
}

func (r RuntimeOperationTerminalRefV2) Validate() error {
	count := 0
	for _, present := range []bool{r.OperationSettlementV3 != nil, r.OperationSettlementV4 != nil, r.CheckpointSettlementV5 != nil, r.DispatchUnknownV3 != nil, r.PolicyExclusion != nil} {
		if present {
			count++
		}
	}
	if count != 1 {
		return checkpointInvalidV2("Runtime operation terminal requires exactly one closed tagged ref")
	}
	switch r.Kind {
	case RuntimeTerminalOperationSettlementV3V2:
		if r.OperationSettlementV3 == nil {
			return checkpointInvalidV2("Runtime terminal kind does not match its V3 settlement")
		}
		return r.OperationSettlementV3.Validate()
	case RuntimeTerminalOperationSettlementV4V2:
		if r.OperationSettlementV4 == nil {
			return checkpointInvalidV2("Runtime terminal kind does not match its V4 settlement")
		}
		return r.OperationSettlementV4.Validate()
	case RuntimeTerminalCheckpointSettlementV5V2:
		if r.CheckpointSettlementV5 == nil {
			return checkpointInvalidV2("Runtime terminal kind does not match its V5 settlement")
		}
		return r.CheckpointSettlementV5.Validate()
	case RuntimeTerminalDispatchUnknownV3V2:
		if r.DispatchUnknownV3 == nil {
			return checkpointInvalidV2("Runtime terminal kind does not match its unknown dispatch")
		}
		return r.DispatchUnknownV3.Validate()
	case RuntimeTerminalPolicyExclusionV2:
		if r.PolicyExclusion == nil {
			return checkpointInvalidV2("Runtime terminal kind does not match its policy exclusion")
		}
		return r.PolicyExclusion.Validate()
	default:
		return checkpointInvalidV2("unknown Runtime operation terminal kind")
	}
}

type EffectCutEntryV2 struct {
	EffectID       core.EffectIntentID           `json:"effect_id"`
	IntentRevision core.Revision                 `json:"intent_revision"`
	IntentDigest   core.Digest                   `json:"intent_digest"`
	Attempt        OperationDispatchAttemptRefV3 `json:"attempt"`
	Phase          string                        `json:"phase"`
	Disposition    EffectCutEntryDispositionV2   `json:"disposition"`
	Terminal       RuntimeOperationTerminalRefV2 `json:"terminal"`
}

func (e EffectCutEntryV2) Validate() error {
	if e.EffectID == "" || e.IntentRevision == 0 || e.IntentDigest.Validate() != nil || e.Attempt.Validate() != nil || !validCheckpointIDV2(e.Phase) || e.Terminal.Validate() != nil {
		return checkpointInvalidV2("checkpoint Effect Cut entry is incomplete")
	}
	if e.EffectID != e.Attempt.EffectID || e.IntentRevision != e.Attempt.IntentRevision || e.IntentDigest != e.Attempt.IntentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Effect Cut outer Effect identity differs from its operation attempt")
	}
	switch e.Terminal.Kind {
	case RuntimeTerminalOperationSettlementV3V2:
		settlement := e.Terminal.OperationSettlementV3
		if settlement == nil || !sameCheckpointCanonicalV2("OperationDispatchAttemptRefV3", settlement.Attempt, e.Attempt) {
			return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Effect Cut V3 settlement belongs to another attempt")
		}
		expected := EffectCutSettledV2
		if settlement.Disposition == OperationSettlementNotAppliedV3 {
			expected = EffectCutConfirmedNotAppliedV2
		} else if settlement.Disposition == OperationSettlementFailedV3 {
			return core.NewError(core.ErrorForbidden, core.ReasonCheckpointInconsistent, "failed Operation settlement is outside the frozen Effect Cut terminal set")
		}
		if e.Disposition != expected {
			return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Effect Cut disposition mismatches V3 settlement")
		}
	case RuntimeTerminalOperationSettlementV4V2:
		if e.Disposition != EffectCutSettledV2 || e.Terminal.OperationSettlementV4.EffectID != e.EffectID || e.Terminal.OperationSettlementV4.OperationDigest != e.Attempt.OperationDigest || !sameCheckpointCanonicalV2("OperationDispatchAttemptRefV3", e.Terminal.OperationSettlementV4.DomainResult.Attempt, e.Attempt) {
			return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Effect Cut disposition mismatches V4 settlement")
		}
	case RuntimeTerminalCheckpointSettlementV5V2:
		return core.NewError(core.ErrorForbidden, core.ReasonCheckpointInconsistent, "Checkpoint Settlement V5 cannot prove the exact dispatch attempt required by Effect Cut V2")
	case RuntimeTerminalDispatchUnknownV3V2:
		if e.Disposition != EffectCutUnknownV2 || !sameCheckpointCanonicalV2("OperationDispatchAttemptRefV3", *e.Terminal.DispatchUnknownV3, e.Attempt) {
			return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Effect Cut unknown terminal mismatches its attempt")
		}
	case RuntimeTerminalPolicyExclusionV2:
		if e.Disposition != EffectCutExcludedByPolicyV2 || e.Terminal.PolicyExclusion.EffectID != e.EffectID || !sameCheckpointCanonicalV2("OperationDispatchAttemptRefV3", e.Terminal.PolicyExclusion.Attempt, e.Attempt) {
			return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Effect Cut policy exclusion mismatches its attempt")
		}
	default:
		return checkpointInvalidV2("unknown checkpoint Effect Cut terminal kind")
	}
	return nil
}

type EffectCutRefV2 struct {
	ID         string                 `json:"effect_cut_id"`
	Revision   core.Revision          `json:"revision"`
	Attempt    CheckpointAttemptRefV2 `json:"attempt"`
	RootDigest core.Digest            `json:"root_digest"`
	Watermark  core.Revision          `json:"watermark"`
	Count      uint64                 `json:"count"`
	Digest     core.Digest            `json:"digest"`
}

func (r EffectCutRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision == 0 || r.Attempt.Validate() != nil || r.RootDigest.Validate() != nil || r.Watermark == 0 || r.Digest.Validate() != nil {
		return checkpointInvalidV2("checkpoint Effect Cut ref is incomplete")
	}
	return nil
}

type EffectCutFactV2 struct {
	ContractVersion string                      `json:"contract_version"`
	Ref             EffectCutRefV2              `json:"ref"`
	Barrier         CheckpointBarrierLeaseRefV2 `json:"barrier"`
	Entries         []EffectCutEntryV2          `json:"entries"`
	CreatedUnixNano int64                       `json:"created_unix_nano"`
}

func (f EffectCutFactV2) Validate() error {
	if f.ContractVersion != CheckpointGovernanceContractVersionV2 || f.Ref.Validate() != nil || f.Barrier.Validate() != nil || f.Ref.Attempt.ID != f.Barrier.AttemptID || f.Ref.Attempt.TenantID != f.Barrier.TenantID || f.CreatedUnixNano <= 0 || len(f.Entries) > MaxCheckpointEffectCutEntriesV2 || uint64(len(f.Entries)) != f.Ref.Count {
		return checkpointInvalidV2("checkpoint Effect Cut fact is incomplete")
	}
	var previous core.EffectIntentID
	for index, entry := range f.Entries {
		if err := entry.Validate(); err != nil {
			return err
		}
		if index > 0 && entry.EffectID <= previous {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "checkpoint Effect Cut entries must be sorted and unique")
		}
		previous = entry.EffectID
	}
	digest, err := f.DigestV2()
	if err != nil || digest != f.Ref.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Effect Cut digest drifted")
	}
	return nil
}

func (f EffectCutFactV2) DigestV2() (core.Digest, error) {
	copy := f
	copy.Ref.Digest = ""
	return checkpointDigestV2("EffectCutFactV2", copy)
}

func SealEffectCutFactV2(f EffectCutFactV2) (EffectCutFactV2, error) {
	f.ContractVersion = CheckpointGovernanceContractVersionV2
	f.Entries = append([]EffectCutEntryV2{}, f.Entries...)
	sort.Slice(f.Entries, func(i, j int) bool { return f.Entries[i].EffectID < f.Entries[j].EffectID })
	f.Ref.Count = uint64(len(f.Entries))
	f.Ref.Digest = ""
	digest, err := f.DigestV2()
	if err != nil {
		return EffectCutFactV2{}, err
	}
	f.Ref.Digest = digest
	return f, f.Validate()
}

type FreezeCheckpointEffectCutRequestV2 struct {
	Attempt                  CheckpointAttemptRefV2      `json:"attempt"`
	Barrier                  CheckpointBarrierLeaseRefV2 `json:"barrier"`
	ExpectedAttemptRevision  core.Revision               `json:"expected_attempt_revision"`
	ExpectedBarrierRevision  core.Revision               `json:"expected_barrier_revision"`
	EffectInventoryRoot      core.Digest                 `json:"effect_inventory_root"`
	EffectInventoryWatermark core.Revision               `json:"effect_inventory_watermark"`
	ExpectedEffectCount      uint64                      `json:"expected_effect_count"`
	IdempotencyKey           string                      `json:"idempotency_key"`
}

func (r FreezeCheckpointEffectCutRequestV2) Validate() error {
	if r.Attempt.Validate() != nil || r.Barrier.Validate() != nil || r.ExpectedAttemptRevision == 0 || r.ExpectedBarrierRevision == 0 || r.EffectInventoryRoot.Validate() != nil || r.EffectInventoryWatermark == 0 || r.ExpectedEffectCount > MaxCheckpointEffectCutEntriesV2 || !validCheckpointIDV2(r.IdempotencyKey) {
		return checkpointInvalidV2("freeze checkpoint Effect Cut request is incomplete")
	}
	return nil
}

type CheckpointEffectCutBundleV2 struct {
	Attempt CheckpointAttemptFactV2 `json:"attempt"`
	Cut     EffectCutFactV2         `json:"effect_cut"`
}

func (b CheckpointEffectCutBundleV2) Validate() error {
	if b.Attempt.Validate() != nil || b.Cut.Validate() != nil || b.Attempt.EffectCut == nil || *b.Attempt.EffectCut != b.Cut.Ref || b.Attempt.State != CheckpointAttemptCutFrozenV2 {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Effect Cut bundle is not exact")
	}
	return nil
}

type CheckpointEffectCutCommitRequestV2 struct {
	ExpectedAttemptRevision core.Revision           `json:"expected_attempt_revision"`
	ExpectedBarrierRevision core.Revision           `json:"expected_barrier_revision"`
	NextAttempt             CheckpointAttemptFactV2 `json:"next_attempt"`
	Cut                     EffectCutFactV2         `json:"effect_cut"`
}

type CheckpointDiagnosticSetRefV2 struct {
	AttemptID string        `json:"attempt_id"`
	Revision  core.Revision `json:"revision"`
	Count     uint32        `json:"count"`
	SetDigest core.Digest   `json:"set_digest"`
}

func (r CheckpointDiagnosticSetRefV2) Validate() error {
	if !validCheckpointIDV2(r.AttemptID) || r.Revision == 0 {
		return checkpointInvalidV2("checkpoint diagnostic set ref is incomplete")
	}
	return r.SetDigest.Validate()
}

type CheckpointResidualSetRefV2 struct {
	AttemptID string        `json:"attempt_id"`
	Revision  core.Revision `json:"revision"`
	Count     uint32        `json:"count"`
	SetDigest core.Digest   `json:"set_digest"`
}

type CheckpointFinalizationClassificationV2 string

const (
	CheckpointClassificationConfirmedNotAppliedV2 CheckpointFinalizationClassificationV2 = "confirmed_not_applied"
	CheckpointClassificationIncompleteV2          CheckpointFinalizationClassificationV2 = "incomplete"
	CheckpointClassificationUnknownV2             CheckpointFinalizationClassificationV2 = "unknown"
)

type CheckpointFinalizationClassificationEntryV2 struct {
	ID             string                                 `json:"id"`
	Kind           NamespacedNameV2                       `json:"kind"`
	Classification CheckpointFinalizationClassificationV2 `json:"classification"`
	SourceRevision core.Revision                          `json:"source_revision"`
	SourceDigest   core.Digest                            `json:"source_digest"`
}

func (e CheckpointFinalizationClassificationEntryV2) Validate() error {
	if !validCheckpointIDV2(e.ID) || ValidateNamespacedNameV2(e.Kind) != nil || e.SourceRevision == 0 || e.SourceDigest.Validate() != nil {
		return checkpointInvalidV2("checkpoint finalization classification is incomplete")
	}
	switch e.Classification {
	case CheckpointClassificationConfirmedNotAppliedV2, CheckpointClassificationIncompleteV2, CheckpointClassificationUnknownV2:
		return nil
	default:
		return checkpointInvalidV2("checkpoint finalization classification is unknown")
	}
}

type CheckpointFinalizationClassificationSetV2 struct {
	Entries []CheckpointFinalizationClassificationEntryV2 `json:"entries"`
	Digest  core.Digest                                   `json:"digest"`
}

func (s CheckpointFinalizationClassificationSetV2) Validate() error {
	if s.Digest.Validate() != nil || len(s.Entries) > MaxCheckpointEffectCutEntriesV2 {
		return checkpointInvalidV2("checkpoint finalization classification set is incomplete")
	}
	for index, entry := range s.Entries {
		if err := entry.Validate(); err != nil {
			return err
		}
		if index > 0 && entry.ID <= s.Entries[index-1].ID {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "checkpoint finalization classifications must be sorted and unique")
		}
	}
	digest, err := s.DigestV2()
	if err != nil || digest != s.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint finalization classification set drifted")
	}
	return nil
}

func (s CheckpointFinalizationClassificationSetV2) DigestV2() (core.Digest, error) {
	copy := s
	copy.Digest = ""
	return checkpointDigestV2("CheckpointFinalizationClassificationSetV2", copy)
}

func SealCheckpointFinalizationClassificationSetV2(s CheckpointFinalizationClassificationSetV2) (CheckpointFinalizationClassificationSetV2, error) {
	s.Entries = append([]CheckpointFinalizationClassificationEntryV2{}, s.Entries...)
	sort.Slice(s.Entries, func(i, j int) bool { return s.Entries[i].ID < s.Entries[j].ID })
	s.Digest = ""
	digest, err := s.DigestV2()
	if err != nil {
		return CheckpointFinalizationClassificationSetV2{}, err
	}
	s.Digest = digest
	return s, s.Validate()
}

func (r CheckpointResidualSetRefV2) Validate() error {
	if !validCheckpointIDV2(r.AttemptID) || r.Revision == 0 {
		return checkpointInvalidV2("checkpoint residual set ref is incomplete")
	}
	return r.SetDigest.Validate()
}

type CheckpointFinalizationCutRefV2 struct {
	ID          string                 `json:"cut_id"`
	Revision    core.Revision          `json:"revision"`
	Attempt     CheckpointAttemptRefV2 `json:"attempt"`
	EffectCut   EffectCutRefV2         `json:"effect_cut"`
	CutUnixNano int64                  `json:"cut_unix_nano"`
	Digest      core.Digest            `json:"digest"`
}

type CheckpointFinalizationCutFactV2 struct {
	ContractVersion string                         `json:"contract_version"`
	Ref             CheckpointFinalizationCutRefV2 `json:"ref"`
	CreatedUnixNano int64                          `json:"created_unix_nano"`
}

func (f CheckpointFinalizationCutFactV2) Validate() error {
	if f.ContractVersion != CheckpointGovernanceContractVersionV2 || f.Ref.Validate() != nil || f.CreatedUnixNano != f.Ref.CutUnixNano {
		return checkpointInvalidV2("checkpoint Finalization Cut fact is incomplete")
	}
	return nil
}

func (r CheckpointFinalizationCutRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision == 0 || r.Attempt.Validate() != nil || r.EffectCut.Validate() != nil || r.CutUnixNano <= 0 || r.Digest.Validate() != nil || r.Attempt.ID != r.EffectCut.Attempt.ID {
		return checkpointInvalidV2("checkpoint Finalization Cut is incomplete")
	}
	digest, err := r.DigestV2()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Finalization Cut digest drifted")
	}
	return nil
}

func (r CheckpointFinalizationCutRefV2) DigestV2() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return checkpointDigestV2("CheckpointFinalizationCutRefV2", copy)
}

type CheckpointDiagnosticsFinalizationSealRefV2 struct {
	ID                string                                    `json:"seal_id"`
	Revision          core.Revision                             `json:"revision"`
	Attempt           CheckpointAttemptRefV2                    `json:"attempt"`
	FinalizationCut   CheckpointFinalizationCutRefV2            `json:"finalization_cut"`
	Owner             ProviderBindingRefV2                      `json:"owner"`
	SourceEpoch       uint64                                    `json:"source_epoch"`
	SourceSequence    uint64                                    `json:"source_sequence"`
	LedgerRootDigest  core.Digest                               `json:"ledger_root_digest"`
	CompleteSet       CheckpointDiagnosticSetRefV2              `json:"complete_set"`
	CompleteSetDigest core.Digest                               `json:"complete_set_digest"`
	Classifications   CheckpointFinalizationClassificationSetV2 `json:"classifications"`
	Digest            core.Digest                               `json:"digest"`
}

func (r CheckpointDiagnosticsFinalizationSealRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision != 1 || r.Attempt.Validate() != nil || r.FinalizationCut.Validate() != nil || r.Owner.Validate() != nil || r.SourceEpoch == 0 || r.SourceSequence == 0 || r.LedgerRootDigest.Validate() != nil || r.CompleteSet.Validate() != nil || r.CompleteSetDigest.Validate() != nil || r.Classifications.Validate() != nil || r.Digest.Validate() != nil || r.CompleteSet.SetDigest != r.CompleteSetDigest || r.CompleteSet.Count != uint32(len(r.Classifications.Entries)) || r.Attempt.ID != r.FinalizationCut.Attempt.ID {
		return checkpointInvalidV2("checkpoint Diagnostics Finalization Seal is incomplete")
	}
	return nil
}

type CheckpointResidualsFinalizationSealRefV2 struct {
	ID                string                                    `json:"seal_id"`
	Revision          core.Revision                             `json:"revision"`
	Attempt           CheckpointAttemptRefV2                    `json:"attempt"`
	FinalizationCut   CheckpointFinalizationCutRefV2            `json:"finalization_cut"`
	Owner             ProviderBindingRefV2                      `json:"owner"`
	SourceEpoch       uint64                                    `json:"source_epoch"`
	SourceSequence    uint64                                    `json:"source_sequence"`
	LedgerRootDigest  core.Digest                               `json:"ledger_root_digest"`
	CompleteSet       CheckpointResidualSetRefV2                `json:"complete_set"`
	CompleteSetDigest core.Digest                               `json:"complete_set_digest"`
	Classifications   CheckpointFinalizationClassificationSetV2 `json:"classifications"`
	Digest            core.Digest                               `json:"digest"`
}

func (r CheckpointResidualsFinalizationSealRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision != 1 || r.Attempt.Validate() != nil || r.FinalizationCut.Validate() != nil || r.Owner.Validate() != nil || r.SourceEpoch == 0 || r.SourceSequence == 0 || r.LedgerRootDigest.Validate() != nil || r.CompleteSet.Validate() != nil || r.CompleteSetDigest.Validate() != nil || r.Classifications.Validate() != nil || r.Digest.Validate() != nil || r.CompleteSet.SetDigest != r.CompleteSetDigest || r.CompleteSet.Count != uint32(len(r.Classifications.Entries)) || r.Attempt.ID != r.FinalizationCut.Attempt.ID {
		return checkpointInvalidV2("checkpoint Residuals Finalization Seal is incomplete")
	}
	return nil
}

type CheckpointDiagnosticsFinalizationSealProjectionV2 struct {
	Ref              CheckpointDiagnosticsFinalizationSealRefV2 `json:"ref"`
	Current          bool                                       `json:"current"`
	CheckedUnixNano  int64                                      `json:"checked_unix_nano"`
	ProjectionDigest core.Digest                                `json:"projection_digest"`
}

func (p CheckpointDiagnosticsFinalizationSealProjectionV2) Validate() error {
	if p.Ref.Validate() != nil || !p.Current || p.CheckedUnixNano <= 0 || p.ProjectionDigest.Validate() != nil {
		return checkpointInvalidV2("checkpoint Diagnostics Seal current projection is incomplete")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Diagnostics Seal projection drifted")
	}
	return nil
}

func (p CheckpointDiagnosticsFinalizationSealProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return checkpointDigestV2("CheckpointDiagnosticsFinalizationSealProjectionV2", copy)
}

type CheckpointResidualsFinalizationSealProjectionV2 struct {
	Ref              CheckpointResidualsFinalizationSealRefV2 `json:"ref"`
	Current          bool                                     `json:"current"`
	CheckedUnixNano  int64                                    `json:"checked_unix_nano"`
	ProjectionDigest core.Digest                              `json:"projection_digest"`
}

func (p CheckpointResidualsFinalizationSealProjectionV2) Validate() error {
	if p.Ref.Validate() != nil || !p.Current || p.CheckedUnixNano <= 0 || p.ProjectionDigest.Validate() != nil {
		return checkpointInvalidV2("checkpoint Residuals Seal current projection is incomplete")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Residuals Seal projection drifted")
	}
	return nil
}

func (p CheckpointResidualsFinalizationSealProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return checkpointDigestV2("CheckpointResidualsFinalizationSealProjectionV2", copy)
}

type CheckpointFinalizationInputClosureRefV2 struct {
	ID              string                                     `json:"closure_id"`
	Revision        core.Revision                              `json:"revision"`
	Attempt         CheckpointAttemptRefV2                     `json:"attempt"`
	Barrier         CheckpointBarrierLeaseRefV2                `json:"barrier"`
	EffectCut       EffectCutRefV2                             `json:"effect_cut"`
	FinalizationCut CheckpointFinalizationCutRefV2             `json:"finalization_cut"`
	DiagnosticsSeal CheckpointDiagnosticsFinalizationSealRefV2 `json:"diagnostics_seal"`
	ResidualsSeal   CheckpointResidualsFinalizationSealRefV2   `json:"residuals_seal"`
	Digest          core.Digest                                `json:"digest"`
}

func (r CheckpointFinalizationInputClosureRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision != 1 || r.Attempt.Validate() != nil || r.Barrier.Validate() != nil || r.EffectCut.Validate() != nil || r.FinalizationCut.Validate() != nil || r.DiagnosticsSeal.Validate() != nil || r.ResidualsSeal.Validate() != nil || r.Digest.Validate() != nil {
		return checkpointInvalidV2("checkpoint Finalization Input Closure is incomplete")
	}
	if r.Attempt.ID != r.Barrier.AttemptID || r.Attempt.ID != r.EffectCut.Attempt.ID || r.Attempt.ID != r.FinalizationCut.Attempt.ID || r.DiagnosticsSeal.FinalizationCut.Digest != r.FinalizationCut.Digest || r.ResidualsSeal.FinalizationCut.Digest != r.FinalizationCut.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Finalization Closure mixes attempts or Cuts")
	}
	digest, err := r.DigestV2()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Finalization Closure digest drifted")
	}
	return nil
}

func (r CheckpointFinalizationInputClosureRefV2) DigestV2() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return checkpointDigestV2("CheckpointFinalizationInputClosureRefV2", copy)
}

type CheckpointFinalizationInputClosureFactV2 struct {
	ContractVersion string                                  `json:"contract_version"`
	Ref             CheckpointFinalizationInputClosureRefV2 `json:"ref"`
	CreatedUnixNano int64                                   `json:"created_unix_nano"`
}

func (f CheckpointFinalizationInputClosureFactV2) Validate() error {
	if f.ContractVersion != CheckpointGovernanceContractVersionV2 || f.Ref.Validate() != nil || f.CreatedUnixNano < f.Ref.FinalizationCut.CutUnixNano {
		return checkpointInvalidV2("checkpoint Finalization Closure fact is incomplete")
	}
	return nil
}

type PrepareCheckpointFinalizationInputsRequestV2 struct {
	Attempt                 CheckpointAttemptRefV2        `json:"attempt"`
	Barrier                 CheckpointBarrierLeaseRefV2   `json:"barrier"`
	EffectCut               EffectCutRefV2                `json:"effect_cut"`
	ExpectedAttemptRevision core.Revision                 `json:"expected_attempt_revision"`
	ExpectedBarrierRevision core.Revision                 `json:"expected_barrier_revision"`
	ExpectedDiagnostics     *CheckpointDiagnosticSetRefV2 `json:"expected_diagnostics,omitempty"`
	ExpectedResiduals       *CheckpointResidualSetRefV2   `json:"expected_residuals,omitempty"`
	IdempotencyKey          string                        `json:"idempotency_key"`
}

func (r PrepareCheckpointFinalizationInputsRequestV2) Validate() error {
	if r.Attempt.Validate() != nil || r.Barrier.Validate() != nil || r.EffectCut.Validate() != nil || r.ExpectedAttemptRevision == 0 || r.ExpectedBarrierRevision == 0 || !validCheckpointIDV2(r.IdempotencyKey) {
		return checkpointInvalidV2("prepare checkpoint Finalization Inputs request is incomplete")
	}
	if r.ExpectedDiagnostics != nil && r.ExpectedDiagnostics.Validate() != nil {
		return checkpointInvalidV2("expected checkpoint diagnostics are invalid")
	}
	if r.ExpectedResiduals != nil && r.ExpectedResiduals.Validate() != nil {
		return checkpointInvalidV2("expected checkpoint residuals are invalid")
	}
	return nil
}

type CheckpointFinalizationInputsCommitRequestV2 struct {
	ExpectedAttemptRevision core.Revision                            `json:"expected_attempt_revision"`
	NextAttempt             CheckpointAttemptFactV2                  `json:"next_attempt"`
	Closure                 CheckpointFinalizationInputClosureFactV2 `json:"closure"`
}

type CheckpointFinalizationCutCommitRequestV2 struct {
	ExpectedAttemptRevision core.Revision                   `json:"expected_attempt_revision"`
	NextAttempt             CheckpointAttemptFactV2         `json:"next_attempt"`
	Cut                     CheckpointFinalizationCutFactV2 `json:"finalization_cut"`
}

type CheckpointConsistencyRefV2 struct {
	ID       string                 `json:"consistency_id"`
	Revision core.Revision          `json:"revision"`
	Attempt  CheckpointAttemptRefV2 `json:"attempt"`
	Digest   core.Digest            `json:"digest"`
}

func (r CheckpointConsistencyRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision != 1 || r.Attempt.Validate() != nil || r.Digest.Validate() != nil {
		return checkpointInvalidV2("checkpoint Consistency ref is incomplete")
	}
	return nil
}

type CheckpointConsistencyFactV2 struct {
	ContractVersion       string                              `json:"contract_version"`
	Ref                   CheckpointConsistencyRefV2          `json:"ref"`
	Barrier               CheckpointBarrierLeaseRefV2         `json:"barrier"`
	EffectCut             EffectCutRefV2                      `json:"effect_cut"`
	ManifestSeal          CheckpointManifestSealRefV2         `json:"manifest_seal"`
	ParticipantClosures   []CheckpointParticipantClosureRefV2 `json:"participant_closures"`
	ParticipantSetDigest  core.Digest                         `json:"participant_set_digest"`
	ParticipantRootDigest core.Digest                         `json:"participant_root_digest"`
	ParticipantWatermark  core.Revision                       `json:"participant_watermark"`
	ParticipantCount      uint64                              `json:"participant_count"`
	FrozenRefSetDigest    core.Digest                         `json:"frozen_ref_set_digest"`
	CreatedUnixNano       int64                               `json:"created_unix_nano"`
}

func (f CheckpointConsistencyFactV2) Validate() error {
	if f.ContractVersion != CheckpointGovernanceContractVersionV2 || f.Ref.Validate() != nil || f.Barrier.Validate() != nil || f.EffectCut.Validate() != nil || f.ManifestSeal.Validate() != nil || f.ParticipantSetDigest.Validate() != nil || f.ParticipantRootDigest.Validate() != nil || f.ParticipantWatermark == 0 || f.ParticipantCount == 0 || f.ParticipantCount != uint64(len(f.ParticipantClosures)) || f.FrozenRefSetDigest.Validate() != nil || f.CreatedUnixNano <= 0 || len(f.ParticipantClosures) == 0 || len(f.ParticipantClosures) > MaxCheckpointParticipantClosuresV2 {
		return checkpointInvalidV2("checkpoint Consistency fact is incomplete")
	}
	for index, closure := range f.ParticipantClosures {
		if err := closure.Validate(); err != nil {
			return err
		}
		if index > 0 && closure.ID <= f.ParticipantClosures[index-1].ID {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "checkpoint Participant closures must be sorted and unique")
		}
	}
	digest, err := f.DigestV2()
	if err != nil || digest != f.Ref.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Consistency digest drifted")
	}
	return nil
}

func (f CheckpointConsistencyFactV2) DigestV2() (core.Digest, error) {
	copy := f
	copy.Ref.Digest = ""
	copy.ParticipantClosures = normalizeCheckpointClosureRefsV2(copy.ParticipantClosures)
	return checkpointDigestV2("CheckpointConsistencyFactV2", copy)
}

func SealCheckpointConsistencyFactV2(f CheckpointConsistencyFactV2) (CheckpointConsistencyFactV2, error) {
	f.ContractVersion = CheckpointGovernanceContractVersionV2
	f.ParticipantClosures = normalizeCheckpointClosureRefsV2(f.ParticipantClosures)
	f.Ref.Digest = ""
	digest, err := f.DigestV2()
	if err != nil {
		return CheckpointConsistencyFactV2{}, err
	}
	f.Ref.Digest = digest
	return f, f.Validate()
}

type CommitCheckpointConsistencyRequestV2 struct {
	Attempt                      CheckpointAttemptRefV2      `json:"attempt"`
	Barrier                      CheckpointBarrierLeaseRefV2 `json:"barrier"`
	ExpectedAttemptRevision      core.Revision               `json:"expected_attempt_revision"`
	ExpectedBarrierRevision      core.Revision               `json:"expected_barrier_revision"`
	EffectCut                    EffectCutRefV2              `json:"effect_cut"`
	ManifestSeal                 CheckpointManifestSealRefV2 `json:"manifest_seal"`
	ExpectedParticipantRoot      core.Digest                 `json:"expected_participant_root"`
	ExpectedParticipantWatermark core.Revision               `json:"expected_participant_watermark"`
	ExpectedParticipantCount     uint64                      `json:"expected_participant_count"`
	IdempotencyKey               string                      `json:"idempotency_key"`
}

func (r CommitCheckpointConsistencyRequestV2) Validate() error {
	if r.Attempt.Validate() != nil || r.Barrier.Validate() != nil || r.ExpectedAttemptRevision == 0 || r.ExpectedBarrierRevision == 0 || r.EffectCut.Validate() != nil || r.ManifestSeal.Validate() != nil || r.ExpectedParticipantRoot.Validate() != nil || r.ExpectedParticipantWatermark == 0 || r.ExpectedParticipantCount == 0 || r.ExpectedParticipantCount > MaxCheckpointParticipantClosuresV2 || !validCheckpointIDV2(r.IdempotencyKey) {
		return checkpointInvalidV2("commit checkpoint Consistency request is incomplete")
	}
	return nil
}

type CheckpointConsistencyCommitBundleV2 struct {
	Attempt     CheckpointAttemptFactV2      `json:"attempt"`
	Barrier     CheckpointBarrierLeaseFactV2 `json:"barrier"`
	Consistency CheckpointConsistencyFactV2  `json:"consistency"`
}

func (b CheckpointConsistencyCommitBundleV2) Validate() error {
	if b.Attempt.Validate() != nil || b.Barrier.Validate() != nil || b.Consistency.Validate() != nil || b.Attempt.State != CheckpointAttemptConsistentV2 || b.Barrier.State != CheckpointBarrierClosedV2 || b.Attempt.Consistency == nil || *b.Attempt.Consistency != b.Consistency.Ref || b.Consistency.Barrier != b.Barrier.RefV2() {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Consistency commit bundle is not exact")
	}
	return nil
}

type CheckpointConsistencyOwnerCommitRequestV2 struct {
	ExpectedAttemptRevision core.Revision                       `json:"expected_attempt_revision"`
	ExpectedBarrierRevision core.Revision                       `json:"expected_barrier_revision"`
	Bundle                  CheckpointConsistencyCommitBundleV2 `json:"bundle"`
}

type FinalizeCheckpointAttemptRequestV2 struct {
	Attempt                 CheckpointAttemptRefV2                  `json:"attempt"`
	Barrier                 CheckpointBarrierLeaseRefV2             `json:"barrier"`
	ExpectedAttemptRevision core.Revision                           `json:"expected_attempt_revision"`
	ExpectedBarrierRevision core.Revision                           `json:"expected_barrier_revision"`
	Inputs                  CheckpointFinalizationInputClosureRefV2 `json:"inputs"`
	IdempotencyKey          string                                  `json:"idempotency_key"`
}

func (r FinalizeCheckpointAttemptRequestV2) Validate() error {
	if r.Attempt.Validate() != nil || r.Barrier.Validate() != nil || r.ExpectedAttemptRevision == 0 || r.ExpectedBarrierRevision == 0 || r.Inputs.Validate() != nil || !validCheckpointIDV2(r.IdempotencyKey) {
		return checkpointInvalidV2("finalize checkpoint Attempt request is incomplete")
	}
	return nil
}

type CheckpointAttemptFinalizationBundleV2 struct {
	Attempt CheckpointAttemptFactV2                  `json:"attempt"`
	Barrier CheckpointBarrierLeaseFactV2             `json:"barrier"`
	Inputs  CheckpointFinalizationInputClosureFactV2 `json:"inputs"`
}

func (b CheckpointAttemptFinalizationBundleV2) Validate() error {
	if b.Attempt.Validate() != nil || b.Barrier.Validate() != nil || b.Inputs.Validate() != nil || !terminalCheckpointAttemptStateV2(b.Attempt.State) || b.Attempt.State == CheckpointAttemptConsistentV2 || b.Barrier.State != CheckpointBarrierClosedV2 || b.Attempt.FinalizationInputs == nil || !SameCheckpointFinalizationInputClosureRefV2(*b.Attempt.FinalizationInputs, b.Inputs.Ref) {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Finalization bundle is not exact")
	}
	return nil
}

type CheckpointFinalizationOwnerCommitRequestV2 struct {
	ExpectedAttemptRevision core.Revision                         `json:"expected_attempt_revision"`
	ExpectedBarrierRevision core.Revision                         `json:"expected_barrier_revision"`
	Bundle                  CheckpointAttemptFinalizationBundleV2 `json:"bundle"`
}

type CheckpointAttemptTerminalCurrentProjectionV2 struct {
	ContractVersion  string                                      `json:"contract_version"`
	Attempt          CheckpointAttemptRefV2                      `json:"attempt"`
	Barrier          CheckpointBarrierLeaseRefV2                 `json:"barrier"`
	TerminalState    CheckpointAttemptStateV2                    `json:"terminal_state"`
	Consistency      *CheckpointConsistencyRefV2                 `json:"consistency,omitempty"`
	Inputs           *CheckpointFinalizationInputClosureRefV2    `json:"inputs,omitempty"`
	DiagnosticsSeal  *CheckpointDiagnosticsFinalizationSealRefV2 `json:"diagnostics_seal,omitempty"`
	ResidualsSeal    *CheckpointResidualsFinalizationSealRefV2   `json:"residuals_seal,omitempty"`
	CheckedUnixNano  int64                                       `json:"checked_unix_nano"`
	ProjectionDigest core.Digest                                 `json:"projection_digest"`
}

func (p CheckpointAttemptTerminalCurrentProjectionV2) Validate() error {
	if p.ContractVersion != CheckpointGovernanceContractVersionV2 || p.Attempt.Validate() != nil || p.Barrier.Validate() != nil || !terminalCheckpointAttemptStateV2(p.TerminalState) || p.CheckedUnixNano <= 0 || p.ProjectionDigest.Validate() != nil || p.Attempt.ID != p.Barrier.AttemptID || p.Attempt.TenantID != p.Barrier.TenantID {
		return checkpointInvalidV2("checkpoint terminal current projection is incomplete")
	}
	if p.TerminalState == CheckpointAttemptConsistentV2 {
		if p.Consistency == nil || p.Consistency.Validate() != nil || p.Inputs != nil || p.DiagnosticsSeal != nil || p.ResidualsSeal != nil {
			return checkpointInvalidV2("consistent checkpoint projection has invalid sidecars")
		}
	} else if p.Inputs == nil || p.DiagnosticsSeal == nil || p.ResidualsSeal == nil || p.Inputs.Validate() != nil || p.DiagnosticsSeal.Validate() != nil || p.ResidualsSeal.Validate() != nil || p.Consistency != nil || !SameCheckpointDiagnosticsFinalizationSealRefV2(p.Inputs.DiagnosticsSeal, *p.DiagnosticsSeal) || !SameCheckpointResidualsFinalizationSealRefV2(p.Inputs.ResidualsSeal, *p.ResidualsSeal) {
		return checkpointInvalidV2("non-success checkpoint projection requires exact Finalization Closure")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint terminal current projection digest drifted")
	}
	return nil
}

func SameCheckpointFinalizationInputClosureRefV2(left, right CheckpointFinalizationInputClosureRefV2) bool {
	return left.ID == right.ID && left.Revision == right.Revision && left.Attempt == right.Attempt && left.Barrier == right.Barrier && left.EffectCut == right.EffectCut && left.FinalizationCut == right.FinalizationCut && SameCheckpointDiagnosticsFinalizationSealRefV2(left.DiagnosticsSeal, right.DiagnosticsSeal) && SameCheckpointResidualsFinalizationSealRefV2(left.ResidualsSeal, right.ResidualsSeal) && left.Digest == right.Digest
}

func SameCheckpointDiagnosticsFinalizationSealRefV2(left, right CheckpointDiagnosticsFinalizationSealRefV2) bool {
	return left.ID == right.ID && left.Revision == right.Revision && left.Attempt == right.Attempt && left.FinalizationCut == right.FinalizationCut && left.Owner == right.Owner && left.SourceEpoch == right.SourceEpoch && left.SourceSequence == right.SourceSequence && left.LedgerRootDigest == right.LedgerRootDigest && left.CompleteSet == right.CompleteSet && left.CompleteSetDigest == right.CompleteSetDigest && sameCheckpointFinalizationClassificationSetV2(left.Classifications, right.Classifications) && left.Digest == right.Digest
}

func SameCheckpointResidualsFinalizationSealRefV2(left, right CheckpointResidualsFinalizationSealRefV2) bool {
	return left.ID == right.ID && left.Revision == right.Revision && left.Attempt == right.Attempt && left.FinalizationCut == right.FinalizationCut && left.Owner == right.Owner && left.SourceEpoch == right.SourceEpoch && left.SourceSequence == right.SourceSequence && left.LedgerRootDigest == right.LedgerRootDigest && left.CompleteSet == right.CompleteSet && left.CompleteSetDigest == right.CompleteSetDigest && sameCheckpointFinalizationClassificationSetV2(left.Classifications, right.Classifications) && left.Digest == right.Digest
}

func sameCheckpointFinalizationClassificationSetV2(left, right CheckpointFinalizationClassificationSetV2) bool {
	if left.Digest != right.Digest || len(left.Entries) != len(right.Entries) {
		return false
	}
	for index := range left.Entries {
		if left.Entries[index] != right.Entries[index] {
			return false
		}
	}
	return true
}

func (p CheckpointAttemptTerminalCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return checkpointDigestV2("CheckpointAttemptTerminalCurrentProjectionV2", copy)
}

func SealCheckpointAttemptTerminalCurrentProjectionV2(p CheckpointAttemptTerminalCurrentProjectionV2) (CheckpointAttemptTerminalCurrentProjectionV2, error) {
	p.ContractVersion = CheckpointGovernanceContractVersionV2
	p.ProjectionDigest = ""
	digest, err := p.DigestV2()
	if err != nil {
		return CheckpointAttemptTerminalCurrentProjectionV2{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

type CheckpointAttemptDiagnosticsFinalizationOwnerPortV2 interface {
	SealCheckpointDiagnosticsForFinalizationV2(context.Context, CheckpointAttemptRefV2, EffectCutRefV2, CheckpointFinalizationCutRefV2) (CheckpointDiagnosticsFinalizationSealRefV2, error)
	InspectCheckpointDiagnosticsFinalizationSealCurrentV2(context.Context, CheckpointDiagnosticsFinalizationSealRefV2) (CheckpointDiagnosticsFinalizationSealProjectionV2, error)
}

type CheckpointAttemptResidualsFinalizationOwnerPortV2 interface {
	SealCheckpointResidualsForFinalizationV2(context.Context, CheckpointAttemptRefV2, EffectCutRefV2, CheckpointFinalizationCutRefV2) (CheckpointResidualsFinalizationSealRefV2, error)
	InspectCheckpointResidualsFinalizationSealCurrentV2(context.Context, CheckpointResidualsFinalizationSealRefV2) (CheckpointResidualsFinalizationSealProjectionV2, error)
}

func validateCheckpointAttemptSidecarsV2(f CheckpointAttemptFactV2) error {
	switch f.State {
	case CheckpointAttemptBarrierAcquiredV2:
		if f.EffectCut != nil || f.FinalizationCut != nil || f.FinalizationInputs != nil || f.Consistency != nil {
			return checkpointInvalidV2("new checkpoint Attempt cannot carry later facts")
		}
	case CheckpointAttemptCutFrozenV2, CheckpointAttemptCollectingV2:
		if f.EffectCut == nil || f.EffectCut.Validate() != nil || f.FinalizationCut != nil || f.FinalizationInputs != nil || f.Consistency != nil {
			return checkpointInvalidV2("collecting checkpoint Attempt requires only Effect Cut")
		}
	case CheckpointAttemptFinalizingInputsV2:
		if f.EffectCut == nil || f.EffectCut.Validate() != nil || f.FinalizationCut == nil || f.FinalizationCut.Validate() != nil || f.Consistency != nil {
			return checkpointInvalidV2("finalizing checkpoint Attempt requires Effect and Finalization Cuts")
		}
	case CheckpointAttemptConsistentV2:
		if f.EffectCut == nil || f.Consistency == nil || f.Consistency.Validate() != nil || f.FinalizationCut != nil || f.FinalizationInputs != nil {
			return checkpointInvalidV2("consistent checkpoint Attempt requires exact Consistency only")
		}
	case CheckpointAttemptIncompleteV2, CheckpointAttemptAbortedV2, CheckpointAttemptIndeterminateV2:
		if f.EffectCut == nil || f.FinalizationCut == nil || f.FinalizationInputs == nil || f.FinalizationInputs.Validate() != nil || f.Consistency != nil {
			return checkpointInvalidV2("non-success checkpoint Attempt requires exact Finalization Closure")
		}
	}
	return nil
}

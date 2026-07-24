package ports

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	CheckpointGovernanceContractVersionV2 = "2.0.0"
	MaxCheckpointEffectCutEntriesV2       = 4096
	MaxCheckpointParticipantClosuresV2    = 256
)

type CheckpointAttemptStateV2 string

const (
	CheckpointAttemptBarrierAcquiredV2  CheckpointAttemptStateV2 = "barrier_acquired"
	CheckpointAttemptCutFrozenV2        CheckpointAttemptStateV2 = "cut_frozen"
	CheckpointAttemptCollectingV2       CheckpointAttemptStateV2 = "collecting"
	CheckpointAttemptFinalizingInputsV2 CheckpointAttemptStateV2 = "finalizing_inputs"
	CheckpointAttemptConsistentV2       CheckpointAttemptStateV2 = "consistent"
	CheckpointAttemptIncompleteV2       CheckpointAttemptStateV2 = "incomplete"
	CheckpointAttemptAbortedV2          CheckpointAttemptStateV2 = "aborted"
	CheckpointAttemptIndeterminateV2    CheckpointAttemptStateV2 = "indeterminate"
)

type CheckpointBarrierStateV2 string

const (
	CheckpointBarrierActiveV2 CheckpointBarrierStateV2 = "active"
	CheckpointBarrierClosedV2 CheckpointBarrierStateV2 = "closed"
)

type CheckpointUnknownAtDeadlineModeV2 string

const CheckpointUnknownAtDeadlineIndeterminateV2 CheckpointUnknownAtDeadlineModeV2 = "terminalize_indeterminate"

type CheckpointBarrierPolicyRefV2 struct {
	ID             string        `json:"policy_id"`
	Revision       core.Revision `json:"revision"`
	Digest         core.Digest   `json:"digest"`
	SemanticDigest core.Digest   `json:"semantic_digest"`
}

func (r CheckpointBarrierPolicyRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision == 0 {
		return checkpointInvalidV2("checkpoint Barrier Policy ref is incomplete")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	return r.SemanticDigest.Validate()
}

type CheckpointBarrierPolicyCurrentProjectionV2 struct {
	ContractVersion               string                            `json:"contract_version"`
	Ref                           CheckpointBarrierPolicyRefV2      `json:"ref"`
	MaxBarrierTTLUnixNano         int64                             `json:"max_barrier_ttl_unix_nano"`
	MaxReconciliationTTLUnixNano  int64                             `json:"max_reconciliation_ttl_unix_nano"`
	UnknownAtDeadlineMode         CheckpointUnknownAtDeadlineModeV2 `json:"unknown_at_deadline_mode"`
	AllowConfirmedNotAppliedAbort bool                              `json:"allow_confirmed_not_applied_abort"`
	AbsoluteNotAfterUnixNano      int64                             `json:"absolute_not_after_unix_nano"`
	CheckedUnixNano               int64                             `json:"checked_unix_nano"`
	ExpiresUnixNano               int64                             `json:"expires_unix_nano"`
	ProjectionDigest              core.Digest                       `json:"projection_digest"`
}

func (p CheckpointBarrierPolicyCurrentProjectionV2) Validate(now time.Time) error {
	if p.ContractVersion != CheckpointGovernanceContractVersionV2 || p.Ref.Validate() != nil || p.MaxBarrierTTLUnixNano <= 0 || p.MaxReconciliationTTLUnixNano <= 0 || p.UnknownAtDeadlineMode != CheckpointUnknownAtDeadlineIndeterminateV2 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.AbsoluteNotAfterUnixNano <= p.CheckedUnixNano || now.IsZero() {
		return checkpointInvalidV2("checkpoint Barrier Policy projection is incomplete")
	}
	if now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "checkpoint policy current clock regressed")
	}
	if !now.Before(time.Unix(0, minInt64V2(p.ExpiresUnixNano, p.AbsoluteNotAfterUnixNano))) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "checkpoint Barrier Policy is expired")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "checkpoint Barrier Policy projection drifted")
	}
	return nil
}

func (p CheckpointBarrierPolicyCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return checkpointDigestV2("CheckpointBarrierPolicyCurrentProjectionV2", copy)
}

func SealCheckpointBarrierPolicyCurrentProjectionV2(p CheckpointBarrierPolicyCurrentProjectionV2, now time.Time) (CheckpointBarrierPolicyCurrentProjectionV2, error) {
	p.ContractVersion = CheckpointGovernanceContractVersionV2
	p.ProjectionDigest = ""
	digest, err := p.DigestV2()
	if err != nil {
		return CheckpointBarrierPolicyCurrentProjectionV2{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type CheckpointParticipantSetCertificationRefV2 struct {
	ID       string        `json:"certification_id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r CheckpointParticipantSetCertificationRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision == 0 {
		return checkpointInvalidV2("checkpoint Participant Set certification ref is incomplete")
	}
	return r.Digest.Validate()
}

type CheckpointWorkflowRefV2 struct {
	ID       string        `json:"workflow_id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
	NotAfter int64         `json:"not_after_unix_nano"`
}

func (r CheckpointWorkflowRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision == 0 || r.NotAfter <= 0 {
		return checkpointInvalidV2("checkpoint Workflow ref is incomplete")
	}
	return r.Digest.Validate()
}

type CheckpointAttemptRefV2 struct {
	TenantID core.TenantID `json:"tenant_id"`
	ID       string        `json:"attempt_id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r CheckpointAttemptRefV2) Validate() error {
	if strings.TrimSpace(string(r.TenantID)) == "" || !validCheckpointIDV2(r.ID) || r.Revision == 0 {
		return checkpointInvalidV2("checkpoint Attempt ref is incomplete")
	}
	return r.Digest.Validate()
}

type CheckpointBarrierLeaseRefV2 struct {
	TenantID        core.TenantID `json:"tenant_id"`
	ID              string        `json:"barrier_id"`
	AttemptID       string        `json:"attempt_id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r CheckpointBarrierLeaseRefV2) Validate() error {
	if strings.TrimSpace(string(r.TenantID)) == "" || !validCheckpointIDV2(r.ID) || !validCheckpointIDV2(r.AttemptID) || r.Revision == 0 || r.ExpiresUnixNano <= 0 {
		return checkpointInvalidV2("checkpoint Barrier ref is incomplete")
	}
	return r.Digest.Validate()
}

type CheckpointAttemptFactV2 struct {
	ContractVersion                string                                     `json:"contract_version"`
	TenantID                       core.TenantID                              `json:"tenant_id"`
	ID                             string                                     `json:"attempt_id"`
	Revision                       core.Revision                              `json:"revision"`
	State                          CheckpointAttemptStateV2                   `json:"state"`
	Scope                          core.ExecutionScope                        `json:"scope"`
	ScopeDigest                    core.Digest                                `json:"scope_digest"`
	RunID                          core.AgentRunID                            `json:"run_id"`
	RunStableIdentityDigest        core.Digest                                `json:"run_stable_identity_digest"`
	Generation                     GenerationArtifactRefV1                    `json:"generation"`
	GenerationBinding              GenerationBindingAssociationRefV1          `json:"generation_binding"`
	BindingSet                     RunBindingSetRefV2                         `json:"binding_set"`
	ParticipantSetCertification    CheckpointParticipantSetCertificationRefV2 `json:"participant_set_certification"`
	Workflow                       CheckpointWorkflowRefV2                    `json:"workflow"`
	BarrierPolicy                  CheckpointBarrierPolicyRefV2               `json:"barrier_policy"`
	BarrierPolicySemanticDigest    core.Digest                                `json:"barrier_policy_semantic_digest"`
	FrozenUnknownAtDeadlineMode    CheckpointUnknownAtDeadlineModeV2          `json:"frozen_unknown_at_deadline_mode"`
	FrozenAllowNotAppliedAbort     bool                                       `json:"frozen_allow_confirmed_not_applied_abort"`
	Barrier                        CheckpointBarrierLeaseRefV2                `json:"barrier"`
	EffectCut                      *EffectCutRefV2                            `json:"effect_cut,omitempty"`
	FinalizationCut                *CheckpointFinalizationCutRefV2            `json:"finalization_cut,omitempty"`
	FinalizationInputs             *CheckpointFinalizationInputClosureRefV2   `json:"finalization_inputs,omitempty"`
	Consistency                    *CheckpointConsistencyRefV2                `json:"consistency,omitempty"`
	ReconciliationDeadlineUnixNano int64                                      `json:"reconciliation_deadline_unix_nano"`
	CreatedUnixNano                int64                                      `json:"created_unix_nano"`
	UpdatedUnixNano                int64                                      `json:"updated_unix_nano"`
	Digest                         core.Digest                                `json:"digest"`
}

func (f CheckpointAttemptFactV2) RefV2() CheckpointAttemptRefV2 {
	return CheckpointAttemptRefV2{TenantID: f.TenantID, ID: f.ID, Revision: f.Revision, Digest: f.Digest}
}

func (f CheckpointAttemptFactV2) Validate() error {
	if f.ContractVersion != CheckpointGovernanceContractVersionV2 || strings.TrimSpace(string(f.TenantID)) == "" || !validCheckpointIDV2(f.ID) || f.Revision == 0 || !validCheckpointAttemptStateV2(f.State) || f.Scope.Validate() != nil || f.RunID == "" || f.Generation.Validate() != nil || f.GenerationBinding.Validate() != nil || f.BindingSet.Validate() != nil || f.ParticipantSetCertification.Validate() != nil || f.Workflow.Validate() != nil || f.BarrierPolicy.Validate() != nil || f.Barrier.Validate() != nil || f.ReconciliationDeadlineUnixNano <= f.CreatedUnixNano || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano {
		return checkpointInvalidV2("checkpoint Attempt fact is incomplete")
	}
	for _, digest := range []core.Digest{f.ScopeDigest, f.RunStableIdentityDigest, f.BarrierPolicySemanticDigest, f.Digest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	scopeDigest, err := ExecutionScopeDigestV2(f.Scope)
	if err != nil || scopeDigest != f.ScopeDigest || f.Scope.Identity.TenantID != f.TenantID || f.Barrier.TenantID != f.TenantID || f.Barrier.AttemptID != f.ID || f.BarrierPolicy.SemanticDigest != f.BarrierPolicySemanticDigest || f.FrozenUnknownAtDeadlineMode != CheckpointUnknownAtDeadlineIndeterminateV2 || f.ReconciliationDeadlineUnixNano > f.Barrier.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Attempt identity, Policy or Barrier relation drifted")
	}
	if err := validateCheckpointAttemptSidecarsV2(f); err != nil {
		return err
	}
	digest, err := f.DigestV2()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Attempt digest drifted")
	}
	return nil
}

func (f CheckpointAttemptFactV2) DigestV2() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	return checkpointDigestV2("CheckpointAttemptFactV2", copy)
}

func SealCheckpointAttemptFactV2(f CheckpointAttemptFactV2) (CheckpointAttemptFactV2, error) {
	f.ContractVersion = CheckpointGovernanceContractVersionV2
	f.Digest = ""
	digest, err := f.DigestV2()
	if err != nil {
		return CheckpointAttemptFactV2{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

type CheckpointBarrierLeaseFactV2 struct {
	ContractVersion           string                       `json:"contract_version"`
	TenantID                  core.TenantID                `json:"tenant_id"`
	ID                        string                       `json:"barrier_id"`
	AttemptID                 string                       `json:"attempt_id"`
	Revision                  core.Revision                `json:"revision"`
	State                     CheckpointBarrierStateV2     `json:"state"`
	ScopeDigest               core.Digest                  `json:"scope_digest"`
	RunID                     core.AgentRunID              `json:"run_id"`
	RunStableIdentityDigest   core.Digest                  `json:"run_stable_identity_digest"`
	Policy                    CheckpointBarrierPolicyRefV2 `json:"policy"`
	AcquiredDispatchWatermark core.Revision                `json:"acquired_dispatch_watermark"`
	AcquiredUnixNano          int64                        `json:"acquired_unix_nano"`
	ExpiresUnixNano           int64                        `json:"expires_unix_nano"`
	ClosedUnixNano            int64                        `json:"closed_unix_nano,omitempty"`
	CloseReason               core.ReasonCode              `json:"close_reason,omitempty"`
	Digest                    core.Digest                  `json:"digest"`
}

func (f CheckpointBarrierLeaseFactV2) RefV2() CheckpointBarrierLeaseRefV2 {
	return CheckpointBarrierLeaseRefV2{TenantID: f.TenantID, ID: f.ID, AttemptID: f.AttemptID, Revision: f.Revision, Digest: f.Digest, ExpiresUnixNano: f.ExpiresUnixNano}
}

func (f CheckpointBarrierLeaseFactV2) Validate() error {
	if f.ContractVersion != CheckpointGovernanceContractVersionV2 || strings.TrimSpace(string(f.TenantID)) == "" || !validCheckpointIDV2(f.ID) || !validCheckpointIDV2(f.AttemptID) || f.Revision == 0 || f.ScopeDigest.Validate() != nil || f.RunID == "" || f.RunStableIdentityDigest.Validate() != nil || f.Policy.Validate() != nil || f.AcquiredDispatchWatermark == 0 || f.AcquiredUnixNano <= 0 || f.ExpiresUnixNano <= f.AcquiredUnixNano || f.Digest.Validate() != nil {
		return checkpointInvalidV2("checkpoint Barrier fact is incomplete")
	}
	switch f.State {
	case CheckpointBarrierActiveV2:
		if f.ClosedUnixNano != 0 || f.CloseReason != "" {
			return checkpointInvalidV2("active checkpoint Barrier cannot carry close provenance")
		}
	case CheckpointBarrierClosedV2:
		if f.ClosedUnixNano < f.AcquiredUnixNano || f.CloseReason == "" {
			return checkpointInvalidV2("closed checkpoint Barrier requires close provenance")
		}
	default:
		return checkpointInvalidV2("unknown checkpoint Barrier state")
	}
	digest, err := f.DigestV2()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Barrier digest drifted")
	}
	return nil
}

func (f CheckpointBarrierLeaseFactV2) DigestV2() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	return checkpointDigestV2("CheckpointBarrierLeaseFactV2", copy)
}

func SealCheckpointBarrierLeaseFactV2(f CheckpointBarrierLeaseFactV2) (CheckpointBarrierLeaseFactV2, error) {
	f.ContractVersion = CheckpointGovernanceContractVersionV2
	f.Digest = ""
	digest, err := f.DigestV2()
	if err != nil {
		return CheckpointBarrierLeaseFactV2{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

type CheckpointAttemptBarrierBundleV2 struct {
	Attempt CheckpointAttemptFactV2      `json:"attempt"`
	Barrier CheckpointBarrierLeaseFactV2 `json:"barrier"`
}

func (b CheckpointAttemptBarrierBundleV2) Validate() error {
	if err := b.Attempt.Validate(); err != nil {
		return err
	}
	if err := b.Barrier.Validate(); err != nil {
		return err
	}
	if b.Attempt.Barrier != b.Barrier.RefV2() || b.Attempt.ID != b.Barrier.AttemptID || b.Attempt.TenantID != b.Barrier.TenantID || b.Attempt.ScopeDigest != b.Barrier.ScopeDigest || b.Attempt.RunID != b.Barrier.RunID || b.Attempt.RunStableIdentityDigest != b.Barrier.RunStableIdentityDigest || b.Attempt.BarrierPolicy != b.Barrier.Policy {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Attempt and Barrier are not one exact bundle")
	}
	return nil
}

type CreateCheckpointAttemptRequestV2 struct {
	AttemptID                      string                                     `json:"attempt_id"`
	BarrierID                      string                                     `json:"barrier_id"`
	IdempotencyKey                 string                                     `json:"idempotency_key"`
	Scope                          core.ExecutionScope                        `json:"scope"`
	ScopeDigest                    core.Digest                                `json:"scope_digest"`
	RunID                          core.AgentRunID                            `json:"run_id"`
	RunStableIdentityDigest        core.Digest                                `json:"run_stable_identity_digest"`
	Generation                     GenerationArtifactRefV1                    `json:"generation"`
	GenerationBinding              GenerationBindingAssociationRefV1          `json:"generation_binding"`
	BindingSet                     RunBindingSetRefV2                         `json:"binding_set"`
	ParticipantSetCertification    CheckpointParticipantSetCertificationRefV2 `json:"participant_set_certification"`
	Workflow                       CheckpointWorkflowRefV2                    `json:"workflow"`
	BarrierPolicy                  CheckpointBarrierPolicyRefV2               `json:"barrier_policy"`
	ExpectedRunRevision            core.Revision                              `json:"expected_run_revision"`
	ExpectedBarrierExpiresUnixNano int64                                      `json:"expected_barrier_expires_unix_nano,omitempty"`
	AcquiredDispatchWatermark      core.Revision                              `json:"acquired_dispatch_watermark"`
}

func (r CreateCheckpointAttemptRequestV2) Validate() error {
	if !validCheckpointIDV2(r.AttemptID) || !validCheckpointIDV2(r.BarrierID) || !validCheckpointIDV2(r.IdempotencyKey) || r.Scope.Validate() != nil || r.RunID == "" || r.ExpectedRunRevision == 0 || r.AcquiredDispatchWatermark == 0 || r.Generation.Validate() != nil || r.GenerationBinding.Validate() != nil || r.BindingSet.Validate() != nil || r.ParticipantSetCertification.Validate() != nil || r.Workflow.Validate() != nil || r.BarrierPolicy.Validate() != nil {
		return checkpointInvalidV2("create checkpoint Attempt request is incomplete")
	}
	for _, digest := range []core.Digest{r.ScopeDigest, r.RunStableIdentityDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	scopeDigest, err := ExecutionScopeDigestV2(r.Scope)
	if err != nil || scopeDigest != r.ScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint request Scope digest drifted")
	}
	return nil
}

type InspectCheckpointAttemptRequestV2 struct {
	TenantID  core.TenantID `json:"tenant_id"`
	AttemptID string        `json:"attempt_id"`
}

type InspectCheckpointAttemptLineageRequestV2 struct {
	TenantID     core.TenantID `json:"tenant_id"`
	AttemptID    string        `json:"attempt_id"`
	FromRevision core.Revision `json:"from_revision"`
	ToRevision   core.Revision `json:"to_revision"`
}

func (r InspectCheckpointAttemptLineageRequestV2) Validate() error {
	if strings.TrimSpace(string(r.TenantID)) == "" || !validCheckpointIDV2(r.AttemptID) || r.FromRevision == 0 || r.ToRevision < r.FromRevision {
		return checkpointInvalidV2("checkpoint Attempt lineage request is incomplete")
	}
	return nil
}

type CheckpointAttemptLineageV2 struct {
	Attempts []CheckpointAttemptFactV2      `json:"attempts"`
	Barriers []CheckpointBarrierLeaseFactV2 `json:"barriers"`
}

func (r InspectCheckpointAttemptRequestV2) Validate() error {
	if strings.TrimSpace(string(r.TenantID)) == "" || !validCheckpointIDV2(r.AttemptID) {
		return checkpointInvalidV2("checkpoint Attempt inspect identity is incomplete")
	}
	return nil
}

type CheckpointBarrierCurrentProjectionV2 struct {
	ContractVersion  string                      `json:"contract_version"`
	Ref              CheckpointBarrierLeaseRefV2 `json:"ref"`
	State            CheckpointBarrierStateV2    `json:"state"`
	Current          bool                        `json:"current"`
	CheckedUnixNano  int64                       `json:"checked_unix_nano"`
	ProjectionDigest core.Digest                 `json:"projection_digest"`
}

func (p CheckpointBarrierCurrentProjectionV2) Validate(now time.Time) error {
	if p.ContractVersion != CheckpointGovernanceContractVersionV2 || p.Ref.Validate() != nil || p.State != CheckpointBarrierActiveV2 || !p.Current || p.CheckedUnixNano <= 0 || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.Ref.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint Barrier is not current")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Barrier current projection drifted")
	}
	return nil
}

func (p CheckpointBarrierCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return checkpointDigestV2("CheckpointBarrierCurrentProjectionV2", copy)
}

func SealCheckpointBarrierCurrentProjectionV2(p CheckpointBarrierCurrentProjectionV2, now time.Time) (CheckpointBarrierCurrentProjectionV2, error) {
	p.ContractVersion = CheckpointGovernanceContractVersionV2
	p.ProjectionDigest = ""
	digest, err := p.DigestV2()
	if err != nil {
		return CheckpointBarrierCurrentProjectionV2{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type CheckpointBarrierPolicyCurrentReaderV2 interface {
	InspectCheckpointBarrierPolicyCurrentV2(context.Context, CheckpointBarrierPolicyRefV2) (CheckpointBarrierPolicyCurrentProjectionV2, error)
}

type CheckpointFactPortV2 interface {
	CreateCheckpointAttemptBundleV2(context.Context, CheckpointAttemptBarrierBundleV2) (CheckpointAttemptBarrierBundleV2, error)
	InspectCheckpointAttemptBundleV2(context.Context, InspectCheckpointAttemptRequestV2) (CheckpointAttemptBarrierBundleV2, error)
	InspectCheckpointAttemptHistoricalV2(context.Context, CheckpointAttemptRefV2) (CheckpointAttemptFactV2, error)
	InspectCheckpointBarrierHistoricalV2(context.Context, CheckpointBarrierLeaseRefV2) (CheckpointBarrierLeaseFactV2, error)
	InspectCheckpointAttemptLineageV2(context.Context, InspectCheckpointAttemptLineageRequestV2) (CheckpointAttemptLineageV2, error)
	CommitCheckpointEffectCutV2(context.Context, CheckpointEffectCutCommitRequestV2) (CheckpointEffectCutBundleV2, error)
	InspectCheckpointEffectCutV2(context.Context, EffectCutRefV2) (EffectCutFactV2, error)
	CommitCheckpointFinalizationCutV2(context.Context, CheckpointFinalizationCutCommitRequestV2) (CheckpointFinalizationCutFactV2, error)
	InspectCheckpointFinalizationCutV2(context.Context, CheckpointFinalizationCutRefV2) (CheckpointFinalizationCutFactV2, error)
	CommitCheckpointFinalizationInputsV2(context.Context, CheckpointFinalizationInputsCommitRequestV2) (CheckpointFinalizationInputClosureFactV2, error)
	InspectCheckpointFinalizationInputsV2(context.Context, CheckpointFinalizationInputClosureRefV2) (CheckpointFinalizationInputClosureFactV2, error)
	CommitCheckpointConsistencyV2(context.Context, CheckpointConsistencyOwnerCommitRequestV2) (CheckpointConsistencyCommitBundleV2, error)
	CommitCheckpointFinalizationV2(context.Context, CheckpointFinalizationOwnerCommitRequestV2) (CheckpointAttemptFinalizationBundleV2, error)
	InspectCheckpointConsistencyV2(context.Context, CheckpointConsistencyRefV2) (CheckpointConsistencyFactV2, error)
}

type CheckpointGovernancePortV2 interface {
	CreateCheckpointAttemptV2(context.Context, CreateCheckpointAttemptRequestV2) (CheckpointAttemptBarrierBundleV2, error)
	InspectCheckpointAttemptV2(context.Context, InspectCheckpointAttemptRequestV2) (CheckpointAttemptBarrierBundleV2, error)
	InspectCheckpointAttemptHistoricalV2(context.Context, CheckpointAttemptRefV2) (CheckpointAttemptFactV2, error)
	InspectCheckpointBarrierHistoricalV2(context.Context, CheckpointBarrierLeaseRefV2) (CheckpointBarrierLeaseFactV2, error)
	InspectCheckpointBarrierCurrentV2(context.Context, CheckpointBarrierLeaseRefV2) (CheckpointBarrierCurrentProjectionV2, error)
	FreezeCheckpointEffectCutV2(context.Context, FreezeCheckpointEffectCutRequestV2) (CheckpointEffectCutBundleV2, error)
	InspectCheckpointEffectCutV2(context.Context, EffectCutRefV2) (EffectCutFactV2, error)
	PrepareCheckpointFinalizationInputsV2(context.Context, PrepareCheckpointFinalizationInputsRequestV2) (CheckpointFinalizationInputClosureRefV2, error)
	InspectCheckpointFinalizationInputsV2(context.Context, CheckpointFinalizationInputClosureRefV2) (CheckpointFinalizationInputClosureFactV2, error)
	InspectCheckpointAttemptTerminalCurrentV2(context.Context, CheckpointAttemptRefV2) (CheckpointAttemptTerminalCurrentProjectionV2, error)
	CommitCheckpointConsistencyAndCloseBarrierV2(context.Context, CommitCheckpointConsistencyRequestV2) (CheckpointConsistencyCommitBundleV2, error)
	FinalizeCheckpointAttemptAndCloseBarrierV2(context.Context, FinalizeCheckpointAttemptRequestV2) (CheckpointAttemptFinalizationBundleV2, error)
	InspectCheckpointConsistencyV2(context.Context, CheckpointConsistencyRefV2) (CheckpointConsistencyFactV2, error)
}

func validCheckpointIDV2(value string) bool {
	if len(value) == 0 || len(value) > 192 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r <= 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func checkpointInvalidV2(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}

func checkpointDigestV2(discriminator string, value any) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.runtime.checkpoint-governance", CheckpointGovernanceContractVersionV2, discriminator, value)
}

func minInt64V2(values ...int64) int64 {
	result := int64(^uint64(0) >> 1)
	for _, value := range values {
		if value < result {
			result = value
		}
	}
	return result
}

func validCheckpointAttemptStateV2(state CheckpointAttemptStateV2) bool {
	switch state {
	case CheckpointAttemptBarrierAcquiredV2, CheckpointAttemptCutFrozenV2, CheckpointAttemptCollectingV2, CheckpointAttemptFinalizingInputsV2, CheckpointAttemptConsistentV2, CheckpointAttemptIncompleteV2, CheckpointAttemptAbortedV2, CheckpointAttemptIndeterminateV2:
		return true
	default:
		return false
	}
}

func terminalCheckpointAttemptStateV2(state CheckpointAttemptStateV2) bool {
	return state == CheckpointAttemptConsistentV2 || state == CheckpointAttemptIncompleteV2 || state == CheckpointAttemptAbortedV2 || state == CheckpointAttemptIndeterminateV2
}

func normalizeCheckpointClosureRefsV2(input []CheckpointParticipantClosureRefV2) []CheckpointParticipantClosureRefV2 {
	output := append([]CheckpointParticipantClosureRefV2{}, input...)
	sort.Slice(output, func(i, j int) bool { return output[i].ID < output[j].ID })
	if output == nil {
		output = []CheckpointParticipantClosureRefV2{}
	}
	return output
}

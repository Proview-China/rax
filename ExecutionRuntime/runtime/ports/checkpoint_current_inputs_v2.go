package ports

import (
	"context"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// CheckpointCurrentInputRefV2 is a neutral exact watermark owned by the
// semantic source named by Kind. Runtime consumes it and never creates the
// referenced source fact.
type CheckpointCurrentInputRefV2 struct {
	Kind            NamespacedNameV2 `json:"kind"`
	ID              string           `json:"id"`
	Revision        core.Revision    `json:"revision"`
	Digest          core.Digest      `json:"digest"`
	CheckedUnixNano int64            `json:"checked_unix_nano"`
	ExpiresUnixNano int64            `json:"expires_unix_nano"`
}

func (r CheckpointCurrentInputRefV2) Validate(now time.Time) error {
	if ValidateNamespacedNameV2(r.Kind) != nil || !validCheckpointIDV2(r.ID) || r.Revision == 0 || r.Digest.Validate() != nil || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano || now.IsZero() {
		return checkpointInvalidV2("checkpoint current input ref is incomplete")
	}
	if now.UnixNano() < r.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "checkpoint current input clock regressed")
	}
	if !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "checkpoint current input expired")
	}
	return nil
}

// CheckpointAttemptInputsCurrentProjectionV2 closes the immutable attempt
// inputs against their current semantic Owners. It is not an authorization.
type CheckpointAttemptInputsCurrentProjectionV2 struct {
	ContractVersion             string                                     `json:"contract_version"`
	AttemptID                   string                                     `json:"attempt_id"`
	TenantID                    core.TenantID                              `json:"tenant_id"`
	Run                         CheckpointCurrentInputRefV2                `json:"run"`
	RunID                       core.AgentRunID                            `json:"run_id"`
	RunStableIdentityDigest     core.Digest                                `json:"run_stable_identity_digest"`
	Generation                  CheckpointCurrentInputRefV2                `json:"generation"`
	GenerationArtifact          GenerationArtifactRefV1                    `json:"generation_artifact"`
	GenerationBinding           GenerationBindingAssociationRefV1          `json:"generation_binding"`
	Binding                     CheckpointCurrentInputRefV2                `json:"binding"`
	BindingSet                  RunBindingSetRefV2                         `json:"binding_set"`
	ParticipantCertification    CheckpointCurrentInputRefV2                `json:"participant_certification"`
	ParticipantSetCertification CheckpointParticipantSetCertificationRefV2 `json:"participant_set_certification"`
	WorkflowCurrent             CheckpointCurrentInputRefV2                `json:"workflow_current"`
	Workflow                    CheckpointWorkflowRefV2                    `json:"workflow"`
	Authority                   CheckpointCurrentInputRefV2                `json:"authority"`
	AuthorityRef                AuthorityBindingRefV2                      `json:"authority_ref"`
	CheckedUnixNano             int64                                      `json:"checked_unix_nano"`
	ExpiresUnixNano             int64                                      `json:"expires_unix_nano"`
	ProjectionDigest            core.Digest                                `json:"projection_digest"`
}

func (p CheckpointAttemptInputsCurrentProjectionV2) Validate(now time.Time) error {
	if p.ContractVersion != CheckpointGovernanceContractVersionV2 || !validCheckpointIDV2(p.AttemptID) || p.TenantID == "" || p.RunID == "" || p.RunStableIdentityDigest.Validate() != nil || p.GenerationArtifact.Validate() != nil || p.GenerationBinding.Validate() != nil || p.BindingSet.Validate() != nil || p.ParticipantSetCertification.Validate() != nil || p.Workflow.Validate() != nil || p.AuthorityRef.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() {
		return checkpointInvalidV2("checkpoint attempt inputs current projection is incomplete")
	}
	refs := []CheckpointCurrentInputRefV2{p.Run, p.Generation, p.Binding, p.ParticipantCertification, p.WorkflowCurrent, p.Authority}
	for _, ref := range refs {
		if err := ref.Validate(now); err != nil {
			return err
		}
		if ref.ExpiresUnixNano < p.ExpiresUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "checkpoint inputs expiry exceeds a source Owner watermark")
		}
	}
	if now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "checkpoint attempt inputs are stale")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "checkpoint attempt inputs projection drifted")
	}
	return nil
}

func (p CheckpointAttemptInputsCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return checkpointDigestV2("CheckpointAttemptInputsCurrentProjectionV2", copy)
}

func SealCheckpointAttemptInputsCurrentProjectionV2(p CheckpointAttemptInputsCurrentProjectionV2, now time.Time) (CheckpointAttemptInputsCurrentProjectionV2, error) {
	p.ContractVersion = CheckpointGovernanceContractVersionV2
	p.ProjectionDigest = ""
	digest, err := p.DigestV2()
	if err != nil {
		return CheckpointAttemptInputsCurrentProjectionV2{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type CheckpointEffectInventoryCurrentProjectionV2 struct {
	ContractVersion  string                      `json:"contract_version"`
	Attempt          CheckpointAttemptRefV2      `json:"attempt"`
	Barrier          CheckpointBarrierLeaseRefV2 `json:"barrier"`
	RootDigest       core.Digest                 `json:"root_digest"`
	Watermark        core.Revision               `json:"watermark"`
	Entries          []EffectCutEntryV2          `json:"entries"`
	CheckedUnixNano  int64                       `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                       `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                 `json:"projection_digest"`
}

// CheckpointRunCurrentProjectionV2 is the narrow Run Owner projection used
// when a Checkpoint Attempt is first created. It makes ExpectedRunRevision an
// enforced governance watermark instead of caller metadata.
type CheckpointRunCurrentProjectionV2 struct {
	ContractVersion         string          `json:"contract_version"`
	RunID                   core.AgentRunID `json:"run_id"`
	Revision                core.Revision   `json:"revision"`
	Status                  core.RunStatus  `json:"status"`
	RunStableIdentityDigest core.Digest     `json:"run_stable_identity_digest"`
	ExecutionScopeDigest    core.Digest     `json:"execution_scope_digest"`
	CheckedUnixNano         int64           `json:"checked_unix_nano"`
	ExpiresUnixNano         int64           `json:"expires_unix_nano"`
	ProjectionDigest        core.Digest     `json:"projection_digest"`
}

func (p CheckpointRunCurrentProjectionV2) Validate(now time.Time) error {
	if p.ContractVersion != CheckpointGovernanceContractVersionV2 || p.RunID == "" || p.Revision == 0 || p.Status != core.RunRunning || p.RunStableIdentityDigest.Validate() != nil || p.ExecutionScopeDigest.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "checkpoint Run is not exact running current")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Run current projection drifted")
	}
	return nil
}

func (p CheckpointRunCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return checkpointDigestV2("CheckpointRunCurrentProjectionV2", copy)
}

func SealCheckpointRunCurrentProjectionV2(p CheckpointRunCurrentProjectionV2, now time.Time) (CheckpointRunCurrentProjectionV2, error) {
	p.ContractVersion = CheckpointGovernanceContractVersionV2
	p.ProjectionDigest = ""
	digest, err := p.DigestV2()
	if err != nil {
		return CheckpointRunCurrentProjectionV2{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type CheckpointRunCurrentReaderV2 interface {
	InspectCheckpointRunCurrentV2(context.Context, core.ExecutionScope, core.AgentRunID) (CheckpointRunCurrentProjectionV2, error)
}

func (p CheckpointEffectInventoryCurrentProjectionV2) Validate(now time.Time) error {
	if p.ContractVersion != CheckpointGovernanceContractVersionV2 || p.Attempt.Validate() != nil || p.Barrier.Validate() != nil || p.RootDigest.Validate() != nil || p.Watermark == 0 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || len(p.Entries) > MaxCheckpointEffectCutEntriesV2 || now.IsZero() || p.Attempt.ID != p.Barrier.AttemptID || p.Attempt.TenantID != p.Barrier.TenantID {
		return checkpointInvalidV2("checkpoint Effect inventory projection is incomplete")
	}
	if now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "checkpoint Effect inventory is stale")
	}
	for index, entry := range p.Entries {
		if err := entry.Validate(); err != nil {
			return err
		}
		if index > 0 && entry.EffectID <= p.Entries[index-1].EffectID {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "checkpoint Effect inventory is not canonical")
		}
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "checkpoint Effect inventory projection drifted")
	}
	return nil
}

func (p CheckpointEffectInventoryCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	copy.Entries = append([]EffectCutEntryV2{}, copy.Entries...)
	sort.Slice(copy.Entries, func(i, j int) bool { return copy.Entries[i].EffectID < copy.Entries[j].EffectID })
	return checkpointDigestV2("CheckpointEffectInventoryCurrentProjectionV2", copy)
}

func SealCheckpointEffectInventoryCurrentProjectionV2(p CheckpointEffectInventoryCurrentProjectionV2, now time.Time) (CheckpointEffectInventoryCurrentProjectionV2, error) {
	p.ContractVersion = CheckpointGovernanceContractVersionV2
	p.Entries = append([]EffectCutEntryV2{}, p.Entries...)
	sort.Slice(p.Entries, func(i, j int) bool { return p.Entries[i].EffectID < p.Entries[j].EffectID })
	p.ProjectionDigest = ""
	digest, err := p.DigestV2()
	if err != nil {
		return CheckpointEffectInventoryCurrentProjectionV2{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type CheckpointParticipantSetCurrentProjectionV2 struct {
	ContractVersion  string                                     `json:"contract_version"`
	Attempt          CheckpointAttemptRefV2                     `json:"attempt"`
	Certification    CheckpointParticipantSetCertificationRefV2 `json:"certification"`
	RootDigest       core.Digest                                `json:"root_digest"`
	Watermark        core.Revision                              `json:"watermark"`
	Participants     []CheckpointParticipantRefV2               `json:"participants"`
	CheckedUnixNano  int64                                      `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                      `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                                `json:"projection_digest"`
}

func (p CheckpointParticipantSetCurrentProjectionV2) Validate(now time.Time) error {
	if p.ContractVersion != CheckpointGovernanceContractVersionV2 || p.Attempt.Validate() != nil || p.Certification.Validate() != nil || p.RootDigest.Validate() != nil || p.Watermark == 0 || len(p.Participants) == 0 || len(p.Participants) > MaxCheckpointParticipantClosuresV2 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() {
		return checkpointInvalidV2("checkpoint Participant set current projection is incomplete")
	}
	if now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "checkpoint Participant set is stale")
	}
	for index, participant := range p.Participants {
		if err := participant.Validate(); err != nil {
			return err
		}
		if index > 0 && participant.ID <= p.Participants[index-1].ID {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "checkpoint Participant set is not canonical")
		}
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "checkpoint Participant set projection drifted")
	}
	return nil
}

func (p CheckpointParticipantSetCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return checkpointDigestV2("CheckpointParticipantSetCurrentProjectionV2", copy)
}

type CheckpointParticipantClosureCurrentProjectionV2 struct {
	ContractVersion  string                                `json:"contract_version"`
	Attempt          CheckpointAttemptRefV2                `json:"attempt"`
	Participant      CheckpointParticipantRefV2            `json:"participant"`
	Closure          CheckpointParticipantClosureRefV2     `json:"closure"`
	BranchGuard      CheckpointParticipantBranchGuardRefV2 `json:"branch_guard"`
	CheckedUnixNano  int64                                 `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                 `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                           `json:"projection_digest"`
}

func (p CheckpointParticipantClosureCurrentProjectionV2) Validate(now time.Time) error {
	if p.ContractVersion != CheckpointGovernanceContractVersionV2 || p.Attempt.Validate() != nil || p.Participant.Validate() != nil || p.Closure.Validate() != nil || p.BranchGuard.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() || p.Closure.Participant != p.Participant || p.Closure.Prepare.DomainResult.Attempt != p.Attempt || p.BranchGuard.TenantID != p.Attempt.TenantID || p.BranchGuard.AttemptID != p.Attempt.ID || p.BranchGuard.ParticipantID != p.Participant.ID || p.Closure.Terminal == nil || p.BranchGuard.SelectedPhase != p.Closure.Terminal.Phase {
		return checkpointInvalidV2("checkpoint Participant closure current projection is incomplete")
	}
	if now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "checkpoint Participant closure is stale")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "checkpoint Participant closure projection drifted")
	}
	return nil
}

func (p CheckpointParticipantClosureCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return checkpointDigestV2("CheckpointParticipantClosureCurrentProjectionV2", copy)
}

type CheckpointAttemptInputsCurrentReaderV2 interface {
	InspectCheckpointAttemptInputsCurrentV2(context.Context, CheckpointAttemptRefV2) (CheckpointAttemptInputsCurrentProjectionV2, error)
}

type CheckpointEffectInventoryCurrentReaderV2 interface {
	InspectCheckpointEffectInventoryCurrentV2(context.Context, CheckpointAttemptRefV2, CheckpointBarrierLeaseRefV2) (CheckpointEffectInventoryCurrentProjectionV2, error)
}

type CheckpointParticipantSetCurrentReaderV2 interface {
	InspectCheckpointParticipantSetCurrentV2(context.Context, CheckpointAttemptRefV2, CheckpointParticipantSetCertificationRefV2) (CheckpointParticipantSetCurrentProjectionV2, error)
}

type CheckpointParticipantClosureCurrentReaderV2 interface {
	InspectCheckpointParticipantClosureCurrentV2(context.Context, CheckpointAttemptRefV2, CheckpointParticipantRefV2) (CheckpointParticipantClosureCurrentProjectionV2, error)
}

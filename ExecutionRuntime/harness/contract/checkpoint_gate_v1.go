package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	CheckpointGateContractVersionV1  = "praxis.harness/checkpoint-gate/v1"
	checkpointGateDigestDomainV1     = "praxis.harness.checkpoint-gate"
	checkpointSnapshotDigestDomainV1 = "praxis.harness.checkpoint-snapshot"
)

type CheckpointGateStateV1 string

const (
	CheckpointGateAcquiredV1    CheckpointGateStateV1 = "acquired"
	CheckpointGateBoundV1       CheckpointGateStateV1 = "runtime_bound"
	CheckpointGateInvalidatedV1 CheckpointGateStateV1 = "invalidated"
	CheckpointGateReleasedV1    CheckpointGateStateV1 = "released"
)

type CheckpointGateRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r CheckpointGateRefV1) Validate() error {
	if strings.TrimSpace(r.ID) == "" || len(r.ID) > MaxReferenceBytes || r.Revision == 0 || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "checkpoint gate ref is invalid")
	}
	return nil
}

type HarnessCheckpointSnapshotRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r HarnessCheckpointSnapshotRefV1) Validate() error {
	if strings.TrimSpace(r.ID) == "" || len(r.ID) > MaxReferenceBytes || r.Revision != 1 || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Harness checkpoint snapshot ref is invalid")
	}
	return nil
}

// AcquireCheckpointGateRequestV1 is the G7 pre-Runtime gate. The caller may
// identify an Application Intent and an expected Session, but cannot submit a
// Snapshot or any Runtime checkpoint Fact.
type AcquireCheckpointGateRequestV1 struct {
	StableID                string        `json:"stable_id"`
	IntentDigest            core.Digest   `json:"intent_digest"`
	Run                     RunRef        `json:"run"`
	SessionID               string        `json:"session_id"`
	ExpectedSessionRevision core.Revision `json:"expected_session_revision"`
	ExpectedSessionDigest   core.Digest   `json:"expected_session_digest"`
	RequestedNotAfter       int64         `json:"requested_not_after_unix_nano"`
}

func (r AcquireCheckpointGateRequestV1) Validate() error {
	if strings.TrimSpace(r.StableID) == "" || len(r.StableID) > MaxReferenceBytes || r.IntentDigest.Validate() != nil || r.Run.Validate() != nil || strings.TrimSpace(r.SessionID) == "" || len(r.SessionID) > MaxReferenceBytes || r.ExpectedSessionRevision == 0 || r.ExpectedSessionDigest.Validate() != nil || r.RequestedNotAfter <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "checkpoint gate request is incomplete")
	}
	return nil
}

type CheckpointRuntimeBindingV1 struct {
	Attempt   runtimeports.CheckpointAttemptRefV2      `json:"attempt"`
	Barrier   runtimeports.CheckpointBarrierLeaseRefV2 `json:"barrier"`
	EffectCut runtimeports.EffectCutRefV2              `json:"effect_cut"`
}

func (b CheckpointRuntimeBindingV1) Validate(run RunRef) error {
	if b.Attempt.Validate() != nil || b.Barrier.Validate() != nil || b.EffectCut.Validate() != nil || run.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "checkpoint Runtime binding is incomplete")
	}
	// Runtime freezes an EffectCut against the pre-transition Attempt ref and
	// atomically advances the current Attempt by one revision. Harness binds
	// that public pair without interpreting Runtime state or copying lineage.
	if b.Attempt.TenantID != run.Scope.Identity.TenantID || b.Attempt.ID != b.Barrier.AttemptID || b.Attempt.ID != b.EffectCut.Attempt.ID || b.Barrier.TenantID != b.Attempt.TenantID || b.EffectCut.Attempt.TenantID != b.Attempt.TenantID || b.Attempt.Revision != b.EffectCut.Attempt.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Runtime binding mixes Run, Attempt, Barrier, or EffectCut")
	}
	return nil
}

type BindCheckpointGateRuntimeRequestV1 struct {
	Expected CheckpointGateRefV1        `json:"expected"`
	Runtime  CheckpointRuntimeBindingV1 `json:"runtime"`
}

func (r BindCheckpointGateRuntimeRequestV1) Validate() error {
	if r.Expected.Validate() != nil || r.Expected.Revision != 1 || r.Runtime.Attempt.Validate() != nil || r.Runtime.Barrier.Validate() != nil || r.Runtime.EffectCut.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "bind checkpoint gate Runtime request is incomplete")
	}
	return nil
}

type HarnessCheckpointSnapshotFactV1 struct {
	ContractVersion  string                         `json:"contract_version"`
	Ref              HarnessCheckpointSnapshotRefV1 `json:"ref"`
	IntentDigest     core.Digest                    `json:"intent_digest"`
	Run              RunRef                         `json:"run"`
	Session          GovernedSessionV4              `json:"session"`
	CapturedUnixNano int64                          `json:"captured_unix_nano"`
}

func (f HarnessCheckpointSnapshotFactV1) Clone() HarnessCheckpointSnapshotFactV1 {
	f.Run.Scope = cloneExecutionScopeV3(f.Run.Scope)
	f.Session = f.Session.Clone()
	return f
}

func (f HarnessCheckpointSnapshotFactV1) DigestV1() (core.Digest, error) {
	copy := f.Clone()
	copy.Ref.Digest = ""
	return core.CanonicalJSONDigest(checkpointSnapshotDigestDomainV1, CheckpointGateContractVersionV1, "HarnessCheckpointSnapshotFactV1", copy)
}

func (f HarnessCheckpointSnapshotFactV1) Validate() error {
	if f.ContractVersion != CheckpointGateContractVersionV1 || f.Ref.Validate() != nil || f.IntentDigest.Validate() != nil || f.Run.Validate() != nil || f.Session.Validate() != nil || f.CapturedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Harness checkpoint snapshot is incomplete")
	}
	if f.Session.Run.RunID != f.Run.RunID || !runtimeports.SameExecutionScopeV2(f.Session.Run.Scope, f.Run.Scope) || !CheckpointSafeSessionPhaseV1(f.Session.Phase) {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Harness checkpoint snapshot coordinates or phase drifted")
	}
	digest, err := f.DigestV1()
	if err != nil || digest != f.Ref.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Harness checkpoint snapshot digest drifted")
	}
	return nil
}

func SealHarnessCheckpointSnapshotFactV1(f HarnessCheckpointSnapshotFactV1) (HarnessCheckpointSnapshotFactV1, error) {
	f = f.Clone()
	f.ContractVersion = CheckpointGateContractVersionV1
	f.Ref.Revision = 1
	f.Ref.Digest = ""
	digest, err := f.DigestV1()
	if err != nil {
		return HarnessCheckpointSnapshotFactV1{}, err
	}
	f.Ref.Digest = digest
	return f.Clone(), f.Validate()
}

type CheckpointGateFactV1 struct {
	ContractVersion  string                         `json:"contract_version"`
	Ref              CheckpointGateRefV1            `json:"ref"`
	State            CheckpointGateStateV1          `json:"state"`
	Request          AcquireCheckpointGateRequestV1 `json:"request"`
	Snapshot         HarnessCheckpointSnapshotRefV1 `json:"snapshot"`
	Runtime          *CheckpointRuntimeBindingV1    `json:"runtime,omitempty"`
	AcquiredUnixNano int64                          `json:"acquired_unix_nano"`
	BoundUnixNano    int64                          `json:"bound_unix_nano,omitempty"`
	ReleasedUnixNano int64                          `json:"released_unix_nano,omitempty"`
	ExpiresUnixNano  int64                          `json:"expires_unix_nano"`
}

func (f CheckpointGateFactV1) Clone() CheckpointGateFactV1 {
	f.Request.Run.Scope = cloneExecutionScopeV3(f.Request.Run.Scope)
	if f.Runtime != nil {
		value := *f.Runtime
		f.Runtime = &value
	}
	return f
}

func (f CheckpointGateFactV1) DigestV1() (core.Digest, error) {
	copy := f.Clone()
	copy.Ref.Digest = ""
	return core.CanonicalJSONDigest(checkpointGateDigestDomainV1, CheckpointGateContractVersionV1, "CheckpointGateFactV1", copy)
}

func (f CheckpointGateFactV1) Validate() error {
	if f.ContractVersion != CheckpointGateContractVersionV1 || f.Ref.Validate() != nil || f.Request.Validate() != nil || f.Snapshot.Validate() != nil || f.AcquiredUnixNano <= 0 || f.ExpiresUnixNano <= f.AcquiredUnixNano || f.ExpiresUnixNano > f.Request.RequestedNotAfter {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "checkpoint gate fact is incomplete")
	}
	switch f.State {
	case CheckpointGateAcquiredV1:
		if f.Ref.Revision != 1 || f.Runtime != nil || f.BoundUnixNano != 0 || f.ReleasedUnixNano != 0 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "acquired checkpoint gate has invalid fields")
		}
	case CheckpointGateBoundV1:
		if f.Ref.Revision != 2 || f.Runtime == nil || f.Runtime.Validate(f.Request.Run) != nil || f.BoundUnixNano < f.AcquiredUnixNano || f.ReleasedUnixNano != 0 || f.ExpiresUnixNano > f.Runtime.Barrier.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Runtime-bound checkpoint gate has invalid fields")
		}
	case CheckpointGateInvalidatedV1:
		if f.Ref.Revision != 2 || f.Runtime != nil || f.BoundUnixNano != 0 || f.ReleasedUnixNano < f.AcquiredUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "invalidated checkpoint gate has invalid fields")
		}
	case CheckpointGateReleasedV1:
		if f.Ref.Revision != 3 || f.Runtime == nil || f.Runtime.Validate(f.Request.Run) != nil || f.BoundUnixNano < f.AcquiredUnixNano || f.ReleasedUnixNano < f.BoundUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "released checkpoint gate has invalid fields")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "checkpoint gate state is invalid")
	}
	digest, err := f.DigestV1()
	if err != nil || digest != f.Ref.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint gate digest drifted")
	}
	return nil
}

func (f CheckpointGateFactV1) ValidateCurrent(now time.Time) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if (f.State != CheckpointGateAcquiredV1 && f.State != CheckpointGateBoundV1) || now.IsZero() || now.UnixNano() < f.AcquiredUnixNano || !now.Before(time.Unix(0, f.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "checkpoint gate is not current")
	}
	return nil
}

func SealCheckpointGateFactV1(f CheckpointGateFactV1) (CheckpointGateFactV1, error) {
	f = f.Clone()
	f.ContractVersion = CheckpointGateContractVersionV1
	f.Ref.Digest = ""
	digest, err := f.DigestV1()
	if err != nil {
		return CheckpointGateFactV1{}, err
	}
	f.Ref.Digest = digest
	return f.Clone(), f.Validate()
}

type ReleaseCheckpointGateRequestV1 struct {
	Expected        CheckpointGateRefV1                 `json:"expected"`
	TerminalAttempt runtimeports.CheckpointAttemptRefV2 `json:"terminal_attempt"`
}

type InvalidateCheckpointGateRequestV1 struct {
	Expected CheckpointGateRefV1 `json:"expected"`
}

func (r InvalidateCheckpointGateRequestV1) Validate() error {
	if r.Expected.Validate() != nil || r.Expected.Revision != 1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "invalidate checkpoint gate request is incomplete")
	}
	return nil
}

func (r ReleaseCheckpointGateRequestV1) Validate() error {
	if r.Expected.Validate() != nil || r.Expected.Revision != 2 || r.TerminalAttempt.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "release checkpoint gate request is incomplete")
	}
	return nil
}

func CheckpointSafeSessionPhaseV1(phase SessionPhaseV2) bool {
	return phase == SessionWaitingActionV2 || phase == SessionWaitingInputV2 || phase == SessionTerminalV2
}

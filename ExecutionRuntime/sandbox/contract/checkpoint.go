package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const CheckpointEffectKind = "praxis.sandbox/checkpoint"

type CheckpointPhase string

const (
	CheckpointPhasePrepare CheckpointPhase = "prepare"
	CheckpointPhaseCommit  CheckpointPhase = "commit"
	CheckpointPhaseAbort   CheckpointPhase = "abort"
)

func (p CheckpointPhase) Validate() error {
	switch p {
	case CheckpointPhasePrepare, CheckpointPhaseCommit, CheckpointPhaseAbort:
		return nil
	default:
		return fmt.Errorf("unsupported checkpoint phase %q", p)
	}
}

type CheckpointPhaseState string

const (
	CheckpointPhasePrepared      CheckpointPhaseState = "prepared"
	CheckpointPhaseFailed        CheckpointPhaseState = "failed"
	CheckpointPhaseNotApplied    CheckpointPhaseState = "not_applied"
	CheckpointPhaseUnknown       CheckpointPhaseState = "unknown"
	CheckpointPhaseCommitted     CheckpointPhaseState = "committed"
	CheckpointPhaseAborted       CheckpointPhaseState = "aborted"
	CheckpointPhaseIndeterminate CheckpointPhaseState = "indeterminate"
)

func (s CheckpointPhaseState) ValidateFor(phase CheckpointPhase) error {
	if err := phase.Validate(); err != nil {
		return err
	}
	switch s {
	case CheckpointPhaseFailed, CheckpointPhaseNotApplied, CheckpointPhaseUnknown, CheckpointPhaseIndeterminate:
		return nil
	case CheckpointPhasePrepared:
		if phase == CheckpointPhasePrepare {
			return nil
		}
	case CheckpointPhaseCommitted:
		if phase == CheckpointPhaseCommit {
			return nil
		}
	case CheckpointPhaseAborted:
		if phase == CheckpointPhaseAbort {
			return nil
		}
	}
	return fmt.Errorf("checkpoint state %q is invalid for phase %q", s, phase)
}

type CheckpointParticipantState string

const (
	CheckpointParticipantUnprepared          CheckpointParticipantState = "unprepared"
	CheckpointParticipantPrepared            CheckpointParticipantState = "prepared"
	CheckpointParticipantCommitted           CheckpointParticipantState = "committed"
	CheckpointParticipantAborted             CheckpointParticipantState = "aborted"
	CheckpointParticipantIncomplete          CheckpointParticipantState = "incomplete"
	CheckpointParticipantConfirmedNotApplied CheckpointParticipantState = "confirmed_not_applied"
	CheckpointParticipantIndeterminate       CheckpointParticipantState = "indeterminate"
)

func (s CheckpointParticipantState) Validate() error {
	switch s {
	case CheckpointParticipantUnprepared, CheckpointParticipantPrepared, CheckpointParticipantCommitted,
		CheckpointParticipantAborted, CheckpointParticipantIncomplete,
		CheckpointParticipantConfirmedNotApplied, CheckpointParticipantIndeterminate:
		return nil
	default:
		return errors.New("checkpoint participant state is invalid")
	}
}

type CheckpointPhaseClosureRef struct {
	Ref             Ref                  `json:"ref"`
	Phase           CheckpointPhase      `json:"phase"`
	State           CheckpointPhaseState `json:"state"`
	ExpiresUnixNano int64                `json:"expires_unix_nano"`
}

func (r CheckpointPhaseClosureRef) ValidateShape() error {
	if err := r.Ref.ValidateShape("checkpoint phase closure ref"); err != nil {
		return err
	}
	if err := r.State.ValidateFor(r.Phase); err != nil {
		return err
	}
	if r.ExpiresUnixNano <= 0 {
		return errors.New("checkpoint phase closure expiry is required")
	}
	return nil
}

func (r CheckpointPhaseClosureRef) ValidateCurrent(now time.Time) error {
	if err := r.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() >= r.ExpiresUnixNano {
		return errors.New("checkpoint phase closure is expired")
	}
	return nil
}

func SameCheckpointPhaseClosure(a, b CheckpointPhaseClosureRef) bool {
	return SameRef(a.Ref, b.Ref) && a.Phase == b.Phase && a.State == b.State && a.ExpiresUnixNano == b.ExpiresUnixNano
}

type CheckpointPresence string

const (
	CheckpointAbsent  CheckpointPresence = "absent"
	CheckpointPresent CheckpointPresence = "present"
)

func (p CheckpointPresence) Validate() error {
	switch p {
	case CheckpointAbsent, CheckpointPresent:
		return nil
	default:
		return errors.New("checkpoint presence must be explicitly absent or present")
	}
}

type CheckpointOptionalRef struct {
	Presence CheckpointPresence `json:"presence"`
	Ref      *Ref               `json:"ref,omitempty"`
}

func (r CheckpointOptionalRef) ValidateShape(name string) error {
	if err := r.Presence.Validate(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if r.Presence == CheckpointAbsent {
		if r.Ref != nil {
			return fmt.Errorf("%s is absent but carries a ref", name)
		}
		return nil
	}
	if r.Ref == nil {
		return fmt.Errorf("%s is present but carries a nil ref", name)
	}
	return r.Ref.ValidateShape(name + " ref")
}

type CheckpointRuntimeBinding struct {
	InstanceID    string `json:"instance_id"`
	InstanceEpoch uint64 `json:"instance_epoch"`
	LeaseID       string `json:"lease_id"`
	LeaseEpoch    uint64 `json:"lease_epoch"`
	FenceEpoch    uint64 `json:"fence_epoch"`
}

func (b CheckpointRuntimeBinding) ValidateShape() error {
	if strings.TrimSpace(b.InstanceID) == "" || b.InstanceEpoch == 0 || strings.TrimSpace(b.LeaseID) == "" || b.LeaseEpoch == 0 || b.FenceEpoch == 0 {
		return errors.New("checkpoint instance, lease, and fence coordinates are required")
	}
	return nil
}

type CheckpointWatermark struct {
	SourceID    string `json:"source_id"`
	SourceEpoch uint64 `json:"source_epoch"`
	Sequence    uint64 `json:"sequence"`
}

func validateCheckpointWatermarks(values []CheckpointWatermark) error {
	if len(values) == 0 {
		return errors.New("checkpoint source watermarks are required")
	}
	if !slices.IsSortedFunc(values, func(a, b CheckpointWatermark) int { return strings.Compare(a.SourceID, b.SourceID) }) {
		return errors.New("checkpoint source watermarks must be sorted")
	}
	for index, value := range values {
		if strings.TrimSpace(value.SourceID) == "" || value.SourceEpoch == 0 || value.Sequence == 0 {
			return errors.New("checkpoint source watermark is incomplete")
		}
		if index > 0 && values[index-1].SourceID == value.SourceID {
			return errors.New("checkpoint source watermarks contain a duplicate source")
		}
	}
	return nil
}

type CheckpointBaseCurrentRefs struct {
	CheckpointAttempt   Ref `json:"checkpoint_attempt"`
	Barrier             Ref `json:"barrier"`
	EffectCut           Ref `json:"effect_cut"`
	RuntimeLeaseBinding Ref `json:"runtime_lease_binding"`
	Requirement         Ref `json:"requirement"`
	Policy              Ref `json:"policy"`
	Workspace           Ref `json:"workspace"`
	Placement           Ref `json:"placement"`
	Backend             Ref `json:"backend"`
	Slot                Ref `json:"slot"`
	Generation          Ref `json:"generation"`
}

func (r CheckpointBaseCurrentRefs) ValidateShape() error {
	for name, ref := range map[string]Ref{
		"checkpoint attempt":    r.CheckpointAttempt,
		"barrier":               r.Barrier,
		"effect cut":            r.EffectCut,
		"runtime lease binding": r.RuntimeLeaseBinding,
		"requirement":           r.Requirement,
		"policy":                r.Policy,
		"workspace":             r.Workspace,
		"placement":             r.Placement,
		"backend":               r.Backend,
		"slot":                  r.Slot,
		"generation":            r.Generation,
	} {
		if err := ref.ValidateShape(name + " ref"); err != nil {
			return err
		}
	}
	return nil
}

func validateCheckpointAttemptIdentity(attemptID string, authoritative Ref) error {
	if attemptID != authoritative.ID {
		return errors.New("checkpoint attempt ID does not match authoritative checkpoint attempt ref")
	}
	return nil
}

type CheckpointPhaseReservation struct {
	Meta                        Meta                       `json:"meta"`
	TenantID                    string                     `json:"tenant_id"`
	ParticipantRef              Ref                        `json:"participant_ref"`
	ExpectedParticipantRevision uint64                     `json:"expected_participant_revision"`
	Phase                       CheckpointPhase            `json:"phase"`
	PreviousPresence            CheckpointPresence         `json:"previous_presence"`
	PreviousPhase               *CheckpointPhaseClosureRef `json:"previous_phase,omitempty"`
	OperationID                 string                     `json:"operation_id"`
	EffectID                    string                     `json:"effect_id"`
	AttemptID                   string                     `json:"attempt_id"`
	ExpectedRuntimeAttemptRef   Ref                        `json:"expected_runtime_attempt_ref"`
	Runtime                     CheckpointRuntimeBinding   `json:"runtime"`
	ChangeSet                   CheckpointOptionalRef      `json:"change_set"`
	Watermarks                  []CheckpointWatermark      `json:"watermarks"`
	Base                        CheckpointBaseCurrentRefs  `json:"base"`
}

func SealCheckpointPhaseReservation(r CheckpointPhaseReservation) (CheckpointPhaseReservation, error) {
	var err error
	r, err = cloneCheckpoint(r)
	if err != nil {
		return CheckpointPhaseReservation{}, err
	}
	r.Meta.Digest = ""
	digest, err := Digest("checkpoint-phase-reservation-v2", r)
	if err != nil {
		return CheckpointPhaseReservation{}, err
	}
	r.Meta.Digest = digest
	if err := r.ValidateShape(); err != nil {
		return CheckpointPhaseReservation{}, err
	}
	return r, nil
}

func (r CheckpointPhaseReservation) ValidateShape() error {
	if err := r.Meta.ValidateShape(); err != nil {
		return err
	}
	if strings.TrimSpace(r.TenantID) == "" || strings.TrimSpace(r.OperationID) == "" || strings.TrimSpace(r.EffectID) == "" || strings.TrimSpace(r.AttemptID) == "" {
		return errors.New("checkpoint tenant and operation coordinates are required")
	}
	if err := r.ParticipantRef.ValidateShape("checkpoint participant ref"); err != nil {
		return err
	}
	if r.ExpectedParticipantRevision == 0 {
		return errors.New("expected participant revision is required")
	}
	if r.ExpectedParticipantRevision != r.ParticipantRef.Revision {
		return errors.New("expected participant revision does not match participant exact ref")
	}
	if err := r.Phase.Validate(); err != nil {
		return err
	}
	if err := r.Base.ValidateShape(); err != nil {
		return err
	}
	if err := validateCheckpointAttemptIdentity(r.AttemptID, r.Base.CheckpointAttempt); err != nil {
		return err
	}
	if err := r.ExpectedRuntimeAttemptRef.ValidateShape("checkpoint expected runtime attempt ref"); err != nil {
		return err
	}
	if err := r.Runtime.ValidateShape(); err != nil {
		return err
	}
	if err := r.ChangeSet.ValidateShape("checkpoint change set"); err != nil {
		return err
	}
	if err := validateCheckpointWatermarks(r.Watermarks); err != nil {
		return err
	}
	if err := r.PreviousPresence.Validate(); err != nil {
		return err
	}
	switch r.Phase {
	case CheckpointPhasePrepare:
		if r.PreviousPresence != CheckpointAbsent || r.PreviousPhase != nil {
			return errors.New("prepare reservation must not have a previous phase")
		}
	case CheckpointPhaseCommit, CheckpointPhaseAbort:
		if r.PreviousPresence != CheckpointPresent || r.PreviousPhase == nil {
			return errors.New("commit and abort require an exact previous phase closure")
		}
		if err := r.PreviousPhase.ValidateShape(); err != nil {
			return err
		}
		if r.PreviousPhase.Phase != CheckpointPhasePrepare || r.PreviousPhase.State != CheckpointPhasePrepared {
			return errors.New("only a prepared prepare closure can create a checkpoint successor")
		}
		if r.Meta.ExpiresUnixNano > r.PreviousPhase.ExpiresUnixNano {
			return errors.New("checkpoint successor expiry exceeds previous phase closure")
		}
	}
	expected, err := checkpointReservationDigest(r)
	if err != nil {
		return err
	}
	if expected != r.Meta.Digest {
		return errors.New("checkpoint reservation digest mismatch")
	}
	return nil
}

func (r CheckpointPhaseReservation) ValidateCurrent(now time.Time) error {
	if err := r.ValidateShape(); err != nil {
		return err
	}
	if err := r.Meta.ValidateCurrent(now); err != nil {
		return err
	}
	if r.PreviousPhase != nil {
		return r.PreviousPhase.ValidateCurrent(now)
	}
	return nil
}

func checkpointReservationDigest(r CheckpointPhaseReservation) (string, error) {
	r.Meta.Digest = ""
	return Digest("checkpoint-phase-reservation-v2", r)
}

func CheckpointPhaseKey(r CheckpointPhaseReservation) (string, error) {
	return Digest("checkpoint-phase-key-v2", struct {
		TenantID            string
		CheckpointAttemptID string
		ParticipantID       string
		Phase               CheckpointPhase
	}{r.TenantID, r.Base.CheckpointAttempt.ID, r.ParticipantRef.ID, r.Phase})
}

func CheckpointBranchKey(r CheckpointPhaseReservation) (string, error) {
	if r.PreviousPhase == nil {
		return "", errors.New("checkpoint branch key requires a previous phase closure")
	}
	return Digest("checkpoint-branch-key-v2", struct {
		TenantID            string
		CheckpointAttemptID string
		ParticipantID       string
	}{r.TenantID, r.Base.CheckpointAttempt.ID, r.ParticipantRef.ID})
}

type CheckpointParticipantFact struct {
	Meta                 Meta                       `json:"meta"`
	TenantID             string                     `json:"tenant_id"`
	CheckpointAttemptRef Ref                        `json:"checkpoint_attempt_ref"`
	State                CheckpointParticipantState `json:"state"`
	ActivePhase          CheckpointPhase            `json:"active_phase"`
	ActiveReservation    CheckpointOptionalRef      `json:"active_reservation"`
	Closure              *CheckpointPhaseClosureRef `json:"closure,omitempty"`
}

func SealCheckpointParticipantFact(f CheckpointParticipantFact) (CheckpointParticipantFact, error) {
	var err error
	f, err = cloneCheckpoint(f)
	if err != nil {
		return CheckpointParticipantFact{}, err
	}
	f.Meta.Digest = ""
	digest, err := Digest("checkpoint-participant-fact-v2", f)
	if err != nil {
		return CheckpointParticipantFact{}, err
	}
	f.Meta.Digest = digest
	if err := f.ValidateShape(); err != nil {
		return CheckpointParticipantFact{}, err
	}
	return f, nil
}

func (f CheckpointParticipantFact) ValidateShape() error {
	if err := f.Meta.ValidateShape(); err != nil {
		return err
	}
	if strings.TrimSpace(f.TenantID) == "" {
		return errors.New("checkpoint participant tenant is required")
	}
	if err := f.CheckpointAttemptRef.ValidateShape("checkpoint participant attempt ref"); err != nil {
		return err
	}
	if err := f.State.Validate(); err != nil {
		return err
	}
	if err := f.ActiveReservation.ValidateShape("checkpoint active reservation"); err != nil {
		return err
	}
	if f.ActiveReservation.Presence == CheckpointAbsent {
		if f.State != CheckpointParticipantUnprepared || f.Closure != nil || f.ActivePhase != "" {
			return errors.New("unreserved checkpoint participant contains active phase state")
		}
	} else {
		if err := f.ActivePhase.Validate(); err != nil {
			return err
		}
		if f.State != CheckpointParticipantUnprepared && f.Closure == nil {
			return errors.New("settled checkpoint participant is missing its closure")
		}
		if f.Closure != nil {
			if err := f.Closure.ValidateShape(); err != nil {
				return err
			}
			carriesPreparedBranch := (f.ActivePhase == CheckpointPhaseCommit || f.ActivePhase == CheckpointPhaseAbort) && f.Closure.Phase == CheckpointPhasePrepare && f.Closure.State == CheckpointPhasePrepared
			if f.Closure.Phase != f.ActivePhase && !carriesPreparedBranch {
				return errors.New("checkpoint participant closure phase drifted")
			}
		}
	}
	expected := f
	expected.Meta.Digest = ""
	digest, err := Digest("checkpoint-participant-fact-v2", expected)
	if err != nil {
		return err
	}
	if digest != f.Meta.Digest {
		return errors.New("checkpoint participant fact digest mismatch")
	}
	return nil
}

func (f CheckpointParticipantFact) ValidateCurrent(now time.Time) error {
	if err := f.ValidateShape(); err != nil {
		return err
	}
	return f.Meta.ValidateCurrent(now)
}

type CheckpointPhaseFact struct {
	Meta                 Meta                       `json:"meta"`
	ReservationRef       Ref                        `json:"reservation_ref"`
	TenantID             string                     `json:"tenant_id"`
	ParticipantRef       Ref                        `json:"participant_ref"`
	CheckpointAttemptRef Ref                        `json:"checkpoint_attempt_ref"`
	Phase                CheckpointPhase            `json:"phase"`
	PreviousPresence     CheckpointPresence         `json:"previous_presence"`
	PreviousPhase        *CheckpointPhaseClosureRef `json:"previous_phase,omitempty"`
	OperationID          string                     `json:"operation_id"`
	EffectID             string                     `json:"effect_id"`
	AttemptID            string                     `json:"attempt_id"`
	State                CheckpointPhaseState       `json:"state"`
	EvidenceRefs         []Ref                      `json:"evidence_refs"`
	DomainResultRef      Ref                        `json:"domain_result_ref"`
	RuntimeSettlementRef Ref                        `json:"runtime_settlement_ref"`
	ApplySettlementRef   Ref                        `json:"apply_settlement_ref"`
}

func SealCheckpointPhaseFact(f CheckpointPhaseFact) (CheckpointPhaseFact, error) {
	var err error
	f, err = cloneCheckpoint(f)
	if err != nil {
		return CheckpointPhaseFact{}, err
	}
	f.Meta.Digest = ""
	digest, err := Digest("checkpoint-phase-fact-v2", f)
	if err != nil {
		return CheckpointPhaseFact{}, err
	}
	f.Meta.Digest = digest
	if err := f.ValidateShape(); err != nil {
		return CheckpointPhaseFact{}, err
	}
	return f, nil
}

func (f CheckpointPhaseFact) ValidateShape() error {
	if err := f.Meta.ValidateShape(); err != nil {
		return err
	}
	if strings.TrimSpace(f.TenantID) == "" || strings.TrimSpace(f.OperationID) == "" || strings.TrimSpace(f.EffectID) == "" || strings.TrimSpace(f.AttemptID) == "" {
		return errors.New("checkpoint phase fact coordinates are required")
	}
	for name, ref := range map[string]Ref{
		"reservation":        f.ReservationRef,
		"participant":        f.ParticipantRef,
		"checkpoint attempt": f.CheckpointAttemptRef,
		"domain result":      f.DomainResultRef,
		"runtime settlement": f.RuntimeSettlementRef,
		"apply settlement":   f.ApplySettlementRef,
	} {
		if err := ref.ValidateShape(name + " ref"); err != nil {
			return err
		}
	}
	if err := validateCheckpointAttemptIdentity(f.AttemptID, f.CheckpointAttemptRef); err != nil {
		return err
	}
	if err := f.State.ValidateFor(f.Phase); err != nil {
		return err
	}
	if err := f.PreviousPresence.Validate(); err != nil {
		return err
	}
	if f.Phase == CheckpointPhasePrepare {
		if f.PreviousPresence != CheckpointAbsent || f.PreviousPhase != nil {
			return errors.New("prepare fact must not have a previous phase")
		}
	} else {
		if f.PreviousPresence != CheckpointPresent || f.PreviousPhase == nil {
			return errors.New("checkpoint successor fact requires previous phase closure")
		}
		if err := f.PreviousPhase.ValidateShape(); err != nil {
			return err
		}
		if f.PreviousPhase.Phase != CheckpointPhasePrepare || f.PreviousPhase.State != CheckpointPhasePrepared {
			return errors.New("checkpoint successor fact does not bind prepared closure")
		}
		if f.Meta.ExpiresUnixNano > f.PreviousPhase.ExpiresUnixNano {
			return errors.New("checkpoint phase fact expiry exceeds previous closure")
		}
	}
	if len(f.EvidenceRefs) == 0 {
		return errors.New("checkpoint phase fact evidence refs are required")
	}
	for _, ref := range f.EvidenceRefs {
		if err := ref.ValidateShape("checkpoint phase evidence ref"); err != nil {
			return err
		}
	}
	expected, err := checkpointPhaseFactDigest(f)
	if err != nil {
		return err
	}
	if expected != f.Meta.Digest {
		return errors.New("checkpoint phase fact digest mismatch")
	}
	return nil
}

func (f CheckpointPhaseFact) ValidateCurrent(now time.Time) error {
	if err := f.ValidateShape(); err != nil {
		return err
	}
	return f.Meta.ValidateCurrent(now)
}

func (f CheckpointPhaseFact) ClosureRef() CheckpointPhaseClosureRef {
	return CheckpointPhaseClosureRef{Ref: f.Meta.Ref(), Phase: f.Phase, State: f.State, ExpiresUnixNano: f.Meta.ExpiresUnixNano}
}

func (f CheckpointPhaseFact) ParticipantState() CheckpointParticipantState {
	switch f.State {
	case CheckpointPhasePrepared:
		return CheckpointParticipantPrepared
	case CheckpointPhaseCommitted:
		return CheckpointParticipantCommitted
	case CheckpointPhaseAborted:
		return CheckpointParticipantAborted
	case CheckpointPhaseFailed:
		return CheckpointParticipantIncomplete
	case CheckpointPhaseNotApplied:
		return CheckpointParticipantConfirmedNotApplied
	case CheckpointPhaseUnknown, CheckpointPhaseIndeterminate:
		return CheckpointParticipantIndeterminate
	default:
		return CheckpointParticipantUnprepared
	}
}

func checkpointPhaseFactDigest(f CheckpointPhaseFact) (string, error) {
	f.Meta.Digest = ""
	return Digest("checkpoint-phase-fact-v2", f)
}

func ValidateCheckpointUnknownReconcile(current, next CheckpointPhaseFact) error {
	if err := current.ValidateShape(); err != nil {
		return fmt.Errorf("current checkpoint phase fact: %w", err)
	}
	if err := next.ValidateShape(); err != nil {
		return fmt.Errorf("next checkpoint phase fact: %w", err)
	}
	if current.State != CheckpointPhaseUnknown {
		return errors.New("only an unknown checkpoint phase may be reconciled")
	}
	if next.State != CheckpointPhaseIndeterminate {
		return errors.New("checkpoint unknown reconciliation must become indeterminate")
	}
	if current.Meta.ID != next.Meta.ID || next.Meta.Revision != current.Meta.Revision+1 || next.Meta.CreatedUnixNano != current.Meta.CreatedUnixNano || next.Meta.ExpiresUnixNano > current.Meta.ExpiresUnixNano {
		return errors.New("checkpoint reconciliation violates monotonic revision or TTL")
	}
	if !SameRef(current.ReservationRef, next.ReservationRef) || current.TenantID != next.TenantID || !SameRef(current.ParticipantRef, next.ParticipantRef) || !SameRef(current.CheckpointAttemptRef, next.CheckpointAttemptRef) || current.Phase != next.Phase || current.OperationID != next.OperationID || current.EffectID != next.EffectID || current.AttemptID != next.AttemptID {
		return errors.New("checkpoint reconciliation changed immutable coordinates")
	}
	if current.PreviousPresence != next.PreviousPresence || (current.PreviousPhase == nil) != (next.PreviousPhase == nil) {
		return errors.New("checkpoint reconciliation changed previous phase presence")
	}
	if current.PreviousPhase != nil && !SameCheckpointPhaseClosure(*current.PreviousPhase, *next.PreviousPhase) {
		return errors.New("checkpoint reconciliation changed previous phase closure")
	}
	return nil
}

type CheckpointReadStage string

const (
	CheckpointReadPreAdmission CheckpointReadStage = "pre_admission"
	CheckpointReadPrePrepare   CheckpointReadStage = "pre_prepare"
	CheckpointReadPreExecute   CheckpointReadStage = "pre_execute"
)

func (s CheckpointReadStage) Validate() error {
	switch s {
	case CheckpointReadPreAdmission, CheckpointReadPrePrepare, CheckpointReadPreExecute:
		return nil
	default:
		return fmt.Errorf("unsupported checkpoint read stage %q", s)
	}
}

type CheckpointCurrentKind string

const (
	CheckpointCurrentCheckpointAttempt  CheckpointCurrentKind = "checkpoint_attempt"
	CheckpointCurrentBarrier            CheckpointCurrentKind = "barrier"
	CheckpointCurrentEffectCut          CheckpointCurrentKind = "effect_cut"
	CheckpointCurrentRuntimeLease       CheckpointCurrentKind = "runtime_lease_binding"
	CheckpointCurrentRequirement        CheckpointCurrentKind = "requirement"
	CheckpointCurrentPolicy             CheckpointCurrentKind = "policy"
	CheckpointCurrentWorkspace          CheckpointCurrentKind = "workspace"
	CheckpointCurrentChangeSet          CheckpointCurrentKind = "change_set"
	CheckpointCurrentPlacement          CheckpointCurrentKind = "placement"
	CheckpointCurrentBackend            CheckpointCurrentKind = "backend"
	CheckpointCurrentSlot               CheckpointCurrentKind = "slot"
	CheckpointCurrentGeneration         CheckpointCurrentKind = "generation"
	CheckpointCurrentOperation          CheckpointCurrentKind = "operation"
	CheckpointCurrentAttempt            CheckpointCurrentKind = "attempt"
	CheckpointCurrentAdmission          CheckpointCurrentKind = "admission"
	CheckpointCurrentReview             CheckpointCurrentKind = "review"
	CheckpointCurrentAuthority          CheckpointCurrentKind = "authority"
	CheckpointCurrentBudget             CheckpointCurrentKind = "budget"
	CheckpointCurrentScope              CheckpointCurrentKind = "scope"
	CheckpointCurrentPermit             CheckpointCurrentKind = "permit"
	CheckpointCurrentBegin              CheckpointCurrentKind = "begin"
	CheckpointCurrentPrepareEnforcement CheckpointCurrentKind = "prepare_enforcement"
	CheckpointCurrentPreparedAttempt    CheckpointCurrentKind = "prepared_attempt"
)

func (k CheckpointCurrentKind) Validate() error {
	if slices.Contains(allCheckpointKinds(), k) {
		return nil
	}
	return fmt.Errorf("unsupported checkpoint current kind %q", k)
}

type CheckpointExpectedCurrentRef struct {
	Kind     CheckpointCurrentKind `json:"kind"`
	Presence CheckpointPresence    `json:"presence"`
	Ref      *Ref                  `json:"ref,omitempty"`
}

func (r CheckpointExpectedCurrentRef) ValidateShape() error {
	if err := r.Kind.Validate(); err != nil {
		return err
	}
	return CheckpointOptionalRef{Presence: r.Presence, Ref: r.Ref}.ValidateShape("checkpoint expected current")
}

type CheckpointCurrentReadRequest struct {
	TenantID               string                         `json:"tenant_id"`
	ParticipantRef         Ref                            `json:"participant_ref"`
	CheckpointAttemptRef   Ref                            `json:"checkpoint_attempt_ref"`
	Phase                  CheckpointPhase                `json:"phase"`
	PreviousPresence       CheckpointPresence             `json:"previous_presence"`
	Stage                  CheckpointReadStage            `json:"stage"`
	ExpectedReservationRef Ref                            `json:"expected_reservation_ref"`
	ExpectedPreviousPhase  *CheckpointPhaseClosureRef     `json:"expected_previous_phase,omitempty"`
	OperationID            string                         `json:"operation_id"`
	EffectID               string                         `json:"effect_id"`
	AttemptID              string                         `json:"attempt_id"`
	ExpectedRuntimeAttempt Ref                            `json:"expected_runtime_attempt"`
	Runtime                CheckpointRuntimeBinding       `json:"runtime"`
	ChangeSet              CheckpointOptionalRef          `json:"change_set"`
	Watermarks             []CheckpointWatermark          `json:"watermarks"`
	ExpectedCurrentRefs    []CheckpointExpectedCurrentRef `json:"expected_current_refs"`
}

func (r CheckpointCurrentReadRequest) ValidateShape() error {
	if strings.TrimSpace(r.TenantID) == "" {
		return errors.New("checkpoint current request tenant is required")
	}
	if err := r.ParticipantRef.ValidateShape("checkpoint current participant ref"); err != nil {
		return err
	}
	if err := r.CheckpointAttemptRef.ValidateShape("checkpoint current attempt ref"); err != nil {
		return err
	}
	if err := r.ExpectedReservationRef.ValidateShape("checkpoint current reservation ref"); err != nil {
		return err
	}
	if err := r.Phase.Validate(); err != nil {
		return err
	}
	if err := r.Stage.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.OperationID) == "" || strings.TrimSpace(r.EffectID) == "" || strings.TrimSpace(r.AttemptID) == "" {
		return errors.New("checkpoint current request operation coordinates are required")
	}
	if err := validateCheckpointAttemptIdentity(r.AttemptID, r.CheckpointAttemptRef); err != nil {
		return err
	}
	if err := r.ExpectedRuntimeAttempt.ValidateShape("checkpoint current expected runtime attempt"); err != nil {
		return err
	}
	if err := r.Runtime.ValidateShape(); err != nil {
		return err
	}
	if err := r.ChangeSet.ValidateShape("checkpoint current change set"); err != nil {
		return err
	}
	if err := validateCheckpointWatermarks(r.Watermarks); err != nil {
		return err
	}
	if err := r.PreviousPresence.Validate(); err != nil {
		return err
	}
	if r.Phase == CheckpointPhasePrepare {
		if r.PreviousPresence != CheckpointAbsent || r.ExpectedPreviousPhase != nil {
			return errors.New("prepare current request must not carry previous phase")
		}
	} else {
		if r.PreviousPresence != CheckpointPresent || r.ExpectedPreviousPhase == nil {
			return errors.New("checkpoint successor current request requires previous phase")
		}
		if err := r.ExpectedPreviousPhase.ValidateShape(); err != nil {
			return err
		}
		if r.ExpectedPreviousPhase.Phase != CheckpointPhasePrepare || r.ExpectedPreviousPhase.State != CheckpointPhasePrepared {
			return errors.New("checkpoint successor current request requires prepared closure")
		}
	}
	if !slices.IsSortedFunc(r.ExpectedCurrentRefs, func(a, b CheckpointExpectedCurrentRef) int { return strings.Compare(string(a.Kind), string(b.Kind)) }) {
		return errors.New("checkpoint expected current refs must be sorted by kind")
	}
	all := allCheckpointKinds()
	if len(r.ExpectedCurrentRefs) != len(all) {
		return errors.New("checkpoint current request must explicitly type every current gate")
	}
	for index, expected := range r.ExpectedCurrentRefs {
		if err := expected.ValidateShape(); err != nil {
			return err
		}
		if index > 0 && r.ExpectedCurrentRefs[index-1].Kind == expected.Kind {
			return errors.New("checkpoint current request contains duplicate kind")
		}
		if expected.Kind != all[index] {
			return errors.New("checkpoint current request kinds do not match the closed gate set")
		}
		want := checkpointExpectedPresence(r.Stage, expected.Kind, r.ChangeSet.Presence)
		if expected.Presence != want {
			return fmt.Errorf("checkpoint current %s must be explicitly %s at %s", expected.Kind, want, r.Stage)
		}
	}
	return nil
}

func allCheckpointKinds() []CheckpointCurrentKind {
	values := []CheckpointCurrentKind{
		CheckpointCurrentAdmission,
		CheckpointCurrentAttempt,
		CheckpointCurrentAuthority,
		CheckpointCurrentBackend,
		CheckpointCurrentBarrier,
		CheckpointCurrentBegin,
		CheckpointCurrentBudget,
		CheckpointCurrentChangeSet,
		CheckpointCurrentCheckpointAttempt,
		CheckpointCurrentEffectCut,
		CheckpointCurrentGeneration,
		CheckpointCurrentOperation,
		CheckpointCurrentPlacement,
		CheckpointCurrentPolicy,
		CheckpointCurrentPrepareEnforcement,
		CheckpointCurrentPreparedAttempt,
		CheckpointCurrentPermit,
		CheckpointCurrentRequirement,
		CheckpointCurrentReview,
		CheckpointCurrentRuntimeLease,
		CheckpointCurrentScope,
		CheckpointCurrentSlot,
		CheckpointCurrentWorkspace,
	}
	slices.Sort(values)
	return values
}

func RequiredCheckpointCurrentKinds(stage CheckpointReadStage) []CheckpointCurrentKind {
	values := make([]CheckpointCurrentKind, 0, len(allCheckpointKinds()))
	for _, kind := range allCheckpointKinds() {
		if checkpointExpectedPresence(stage, kind, CheckpointPresent) == CheckpointPresent {
			values = append(values, kind)
		}
	}
	return values
}

func AllCheckpointCurrentKinds() []CheckpointCurrentKind {
	return slices.Clone(allCheckpointKinds())
}

func CheckpointExpectedPresenceFor(stage CheckpointReadStage, kind CheckpointCurrentKind, changeSet CheckpointPresence) CheckpointPresence {
	return checkpointExpectedPresence(stage, kind, changeSet)
}

func checkpointExpectedPresence(stage CheckpointReadStage, kind CheckpointCurrentKind, changeSet CheckpointPresence) CheckpointPresence {
	if kind == CheckpointCurrentChangeSet {
		return changeSet
	}
	switch kind {
	case CheckpointCurrentBackend, CheckpointCurrentBarrier, CheckpointCurrentCheckpointAttempt,
		CheckpointCurrentEffectCut, CheckpointCurrentGeneration, CheckpointCurrentPlacement,
		CheckpointCurrentPolicy, CheckpointCurrentRequirement, CheckpointCurrentRuntimeLease,
		CheckpointCurrentSlot, CheckpointCurrentWorkspace:
		return CheckpointPresent
	case CheckpointCurrentPrepareEnforcement, CheckpointCurrentPreparedAttempt:
		if stage == CheckpointReadPreExecute {
			return CheckpointPresent
		}
		return CheckpointAbsent
	default:
		if stage == CheckpointReadPreAdmission {
			return CheckpointAbsent
		}
		return CheckpointPresent
	}
}

type CheckpointCurrentCoordinate struct {
	Meta                      Meta                     `json:"meta"`
	State                     CurrentFactState         `json:"state"`
	Kind                      CheckpointCurrentKind    `json:"kind"`
	TenantID                  string                   `json:"tenant_id"`
	ParticipantID             string                   `json:"participant_id"`
	CheckpointAttemptRef      Ref                      `json:"checkpoint_attempt_ref"`
	Phase                     CheckpointPhase          `json:"phase"`
	OperationID               string                   `json:"operation_id"`
	EffectID                  string                   `json:"effect_id"`
	AttemptID                 string                   `json:"attempt_id"`
	ExpectedRuntimeAttemptRef Ref                      `json:"expected_runtime_attempt_ref"`
	Runtime                   CheckpointRuntimeBinding `json:"runtime"`
	ChangeSet                 CheckpointOptionalRef    `json:"change_set"`
	Watermarks                []CheckpointWatermark    `json:"watermarks"`
}

func (c CheckpointCurrentCoordinate) ValidateShape() error {
	if err := c.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := c.State.Validate(); err != nil {
		return err
	}
	if err := c.Kind.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.TenantID) == "" || strings.TrimSpace(c.ParticipantID) == "" || strings.TrimSpace(c.OperationID) == "" || strings.TrimSpace(c.EffectID) == "" || strings.TrimSpace(c.AttemptID) == "" {
		return errors.New("checkpoint current coordinate identity is incomplete")
	}
	if err := c.CheckpointAttemptRef.ValidateShape("checkpoint current coordinate attempt ref"); err != nil {
		return err
	}
	if err := validateCheckpointAttemptIdentity(c.AttemptID, c.CheckpointAttemptRef); err != nil {
		return err
	}
	if err := c.ExpectedRuntimeAttemptRef.ValidateShape("checkpoint current coordinate expected runtime attempt ref"); err != nil {
		return err
	}
	if err := c.Runtime.ValidateShape(); err != nil {
		return err
	}
	if err := c.ChangeSet.ValidateShape("checkpoint current coordinate change set"); err != nil {
		return err
	}
	if err := validateCheckpointWatermarks(c.Watermarks); err != nil {
		return err
	}
	return c.Phase.Validate()
}

type CheckpointCurrentQuery struct {
	Kind                      CheckpointCurrentKind
	TenantID                  string
	ParticipantID             string
	CheckpointAttemptRef      Ref
	Phase                     CheckpointPhase
	OperationID               string
	EffectID                  string
	AttemptID                 string
	ExpectedRuntimeAttemptRef Ref
}

func (c CheckpointCurrentCoordinate) ValidateCurrent(now time.Time) error {
	if err := c.ValidateShape(); err != nil {
		return err
	}
	if c.State != CurrentFactActive {
		return errors.New("checkpoint current coordinate is not active")
	}
	return c.Meta.ValidateCurrent(now)
}

type CheckpointParticipantCurrentProjection struct {
	ContractVersion           string                        `json:"contract_version"`
	TenantID                  string                        `json:"tenant_id"`
	ReservationRef            Ref                           `json:"reservation_ref"`
	ParticipantRef            Ref                           `json:"participant_ref"`
	CheckpointAttemptRef      Ref                           `json:"checkpoint_attempt_ref"`
	Phase                     CheckpointPhase               `json:"phase"`
	Stage                     CheckpointReadStage           `json:"stage"`
	PreviousPhase             *CheckpointPhaseClosureRef    `json:"previous_phase,omitempty"`
	PreviousPresence          CheckpointPresence            `json:"previous_presence"`
	OperationID               string                        `json:"operation_id"`
	EffectID                  string                        `json:"effect_id"`
	AttemptID                 string                        `json:"attempt_id"`
	ExpectedRuntimeAttemptRef Ref                           `json:"expected_runtime_attempt_ref"`
	Runtime                   CheckpointRuntimeBinding      `json:"runtime"`
	ChangeSet                 CheckpointOptionalRef         `json:"change_set"`
	Watermarks                []CheckpointWatermark         `json:"watermarks"`
	Current                   []CheckpointCurrentCoordinate `json:"current"`
	Absent                    []CheckpointCurrentKind       `json:"absent"`
	ProjectionRevision        uint64                        `json:"projection_revision"`
	ProjectionDigest          string                        `json:"projection_digest"`
	OwnerComputedCurrent      bool                          `json:"owner_computed_current"`
	CheckedUnixNano           int64                         `json:"checked_unix_nano"`
	ExpiresUnixNano           int64                         `json:"expires_unix_nano"`
}

func SealCheckpointParticipantCurrentProjection(p CheckpointParticipantCurrentProjection) (CheckpointParticipantCurrentProjection, error) {
	var err error
	p, err = cloneCheckpoint(p)
	if err != nil {
		return CheckpointParticipantCurrentProjection{}, err
	}
	p.ContractVersion = ContractFamily
	p.ProjectionDigest = ""
	digest, err := Digest("checkpoint-participant-current-projection-v2", p)
	if err != nil {
		return CheckpointParticipantCurrentProjection{}, err
	}
	p.ProjectionDigest = digest
	return p, nil
}

func (p CheckpointParticipantCurrentProjection) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != ContractFamily || p.ProjectionRevision == 0 || !ValidDigest(p.ProjectionDigest) || !p.OwnerComputedCurrent || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return errors.New("checkpoint current projection identity is incomplete")
	}
	if now.IsZero() || now.UnixNano() >= p.ExpiresUnixNano {
		return errors.New("checkpoint current projection is expired")
	}
	for name, ref := range map[string]Ref{"reservation": p.ReservationRef, "participant": p.ParticipantRef, "checkpoint attempt": p.CheckpointAttemptRef} {
		if err := ref.ValidateShape(name + " ref"); err != nil {
			return err
		}
	}
	if err := p.Phase.Validate(); err != nil {
		return err
	}
	if err := p.Stage.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(p.TenantID) == "" || strings.TrimSpace(p.OperationID) == "" || strings.TrimSpace(p.EffectID) == "" || strings.TrimSpace(p.AttemptID) == "" {
		return errors.New("checkpoint projection operation coordinates are required")
	}
	if err := validateCheckpointAttemptIdentity(p.AttemptID, p.CheckpointAttemptRef); err != nil {
		return err
	}
	if err := p.ExpectedRuntimeAttemptRef.ValidateShape("checkpoint projection expected runtime attempt ref"); err != nil {
		return err
	}
	if err := p.Runtime.ValidateShape(); err != nil {
		return err
	}
	if err := p.ChangeSet.ValidateShape("checkpoint projection change set"); err != nil {
		return err
	}
	if err := validateCheckpointWatermarks(p.Watermarks); err != nil {
		return err
	}
	if err := p.PreviousPresence.Validate(); err != nil {
		return err
	}
	if p.PreviousPresence == CheckpointAbsent && p.PreviousPhase != nil {
		return errors.New("checkpoint projection marks previous phase absent but carries a closure")
	}
	if p.PreviousPresence == CheckpointPresent {
		if p.PreviousPhase == nil {
			return errors.New("checkpoint projection marks previous phase present without a closure")
		}
		if err := p.PreviousPhase.ValidateShape(); err != nil {
			return err
		}
	}
	if !slices.IsSortedFunc(p.Current, func(a, b CheckpointCurrentCoordinate) int { return strings.Compare(string(a.Kind), string(b.Kind)) }) {
		return errors.New("checkpoint current projection facts are not sorted")
	}
	if !slices.IsSorted(p.Absent) {
		return errors.New("checkpoint current projection absent gates are not sorted")
	}
	seen := make(map[CheckpointCurrentKind]CheckpointPresence, len(allCheckpointKinds()))
	for _, current := range p.Current {
		if seen[current.Kind] != "" {
			return errors.New("checkpoint current projection contains a duplicate gate")
		}
		seen[current.Kind] = CheckpointPresent
		if err := current.ValidateCurrent(time.Unix(0, p.CheckedUnixNano)); err != nil {
			return err
		}
		if current.TenantID != p.TenantID || current.ParticipantID != p.ParticipantRef.ID ||
			!SameRef(current.CheckpointAttemptRef, p.CheckpointAttemptRef) || current.Phase != p.Phase ||
			current.OperationID != p.OperationID || current.EffectID != p.EffectID || current.AttemptID != p.AttemptID ||
			!SameRef(current.ExpectedRuntimeAttemptRef, p.ExpectedRuntimeAttemptRef) || current.Runtime != p.Runtime ||
			current.ChangeSet.Presence != p.ChangeSet.Presence || (current.ChangeSet.Ref == nil) != (p.ChangeSet.Ref == nil) ||
			(current.ChangeSet.Ref != nil && !SameRef(*current.ChangeSet.Ref, *p.ChangeSet.Ref)) || !slices.Equal(current.Watermarks, p.Watermarks) {
			return errors.New("checkpoint current projection coordinate binding drifted")
		}
		if p.ExpiresUnixNano > current.Meta.ExpiresUnixNano {
			return errors.New("checkpoint current projection extends source TTL")
		}
	}
	for _, kind := range p.Absent {
		if err := kind.Validate(); err != nil || seen[kind] != "" {
			return errors.New("checkpoint current projection contains invalid absent gate")
		}
		seen[kind] = CheckpointAbsent
	}
	for _, kind := range allCheckpointKinds() {
		want := checkpointExpectedPresence(p.Stage, kind, p.ChangeSet.Presence)
		if seen[kind] != want {
			return fmt.Errorf("checkpoint projection %s presence drifted", kind)
		}
	}
	if p.PreviousPhase != nil && p.ExpiresUnixNano > p.PreviousPhase.ExpiresUnixNano {
		return errors.New("checkpoint current projection extends previous phase TTL")
	}
	expected := p
	expected.ProjectionDigest = ""
	digest, err := Digest("checkpoint-participant-current-projection-v2", expected)
	if err != nil {
		return err
	}
	if digest != p.ProjectionDigest {
		return errors.New("checkpoint current projection digest mismatch")
	}
	return nil
}

func cloneCheckpoint[T any](value T) (T, error) {
	var zero T
	payload, err := json.Marshal(value)
	if err != nil {
		return zero, err
	}
	var result T
	if err := json.Unmarshal(payload, &result); err != nil {
		return zero, err
	}
	return result, nil
}

type CheckpointConformanceCapability string

const (
	CheckpointConformanceCreateOnce       CheckpointConformanceCapability = "reservation_create_once"
	CheckpointConformanceExactCurrent     CheckpointConformanceCapability = "exact_current_reader"
	CheckpointConformancePreparedXOR      CheckpointConformanceCapability = "prepared_commit_xor_abort"
	CheckpointConformanceFailureTerminal  CheckpointConformanceCapability = "failure_no_successor"
	CheckpointConformanceUnknownInspect   CheckpointConformanceCapability = "unknown_inspect_reconcile_only"
	CheckpointConformanceNoABA            CheckpointConformanceCapability = "lost_reply_no_aba"
	CheckpointConformanceTypedNilRejected CheckpointConformanceCapability = "typed_nil_rejected"
	CheckpointConformanceTTLCapped        CheckpointConformanceCapability = "ttl_capped"
)

func RequiredCheckpointConformanceCapabilities() []CheckpointConformanceCapability {
	values := []CheckpointConformanceCapability{
		CheckpointConformanceCreateOnce,
		CheckpointConformanceExactCurrent,
		CheckpointConformancePreparedXOR,
		CheckpointConformanceFailureTerminal,
		CheckpointConformanceUnknownInspect,
		CheckpointConformanceNoABA,
		CheckpointConformanceTypedNilRejected,
		CheckpointConformanceTTLCapped,
	}
	slices.Sort(values)
	return values
}

type CheckpointConformanceReport struct {
	Meta            Meta                              `json:"meta"`
	ReservationRef  Ref                               `json:"reservation_ref"`
	Capabilities    []CheckpointConformanceCapability `json:"capabilities"`
	EvidenceRefs    []Ref                             `json:"evidence_refs"`
	ProviderCalls   uint64                            `json:"provider_calls"`
	ProductionProof bool                              `json:"production_proof"`
}

func SealCheckpointConformanceReport(r CheckpointConformanceReport) (CheckpointConformanceReport, error) {
	var err error
	r, err = cloneCheckpoint(r)
	if err != nil {
		return CheckpointConformanceReport{}, err
	}
	r.Meta.Digest = ""
	digest, err := Digest("checkpoint-conformance-report-v2", r)
	if err != nil {
		return CheckpointConformanceReport{}, err
	}
	r.Meta.Digest = digest
	if err := r.ValidateShape(); err != nil {
		return CheckpointConformanceReport{}, err
	}
	return r, nil
}

func (r CheckpointConformanceReport) ValidateShape() error {
	if err := r.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := r.ReservationRef.ValidateShape("checkpoint conformance reservation ref"); err != nil {
		return err
	}
	if r.ProductionProof || r.ProviderCalls != 0 {
		return errors.New("checkpoint local conformance cannot claim production proof or provider calls")
	}
	required := RequiredCheckpointConformanceCapabilities()
	if !slices.Equal(r.Capabilities, required) {
		return errors.New("checkpoint conformance capabilities are incomplete or unsorted")
	}
	if len(r.EvidenceRefs) == 0 {
		return errors.New("checkpoint conformance evidence is required")
	}
	for _, ref := range r.EvidenceRefs {
		if err := ref.ValidateShape("checkpoint conformance evidence ref"); err != nil {
			return err
		}
	}
	expected := r
	expected.Meta.Digest = ""
	digest, err := Digest("checkpoint-conformance-report-v2", expected)
	if err != nil {
		return err
	}
	if digest != r.Meta.Digest {
		return errors.New("checkpoint conformance digest mismatch")
	}
	return nil
}

func (r CheckpointConformanceReport) ValidateCurrent(now time.Time) error {
	if err := r.ValidateShape(); err != nil {
		return err
	}
	return r.Meta.ValidateCurrent(now)
}

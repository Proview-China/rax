package contract

import (
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type CheckpointTimelineCutV1 struct {
	LedgerScopeDigest core.Digest                  `json:"ledger_scope_digest"`
	LedgerSequence    core.Revision                `json:"ledger_sequence"`
	EvidenceRecord    CheckpointExternalExactRefV1 `json:"evidence_record"`
}

func (c CheckpointTimelineCutV1) Validate() error {
	if c.LedgerScopeDigest.Validate() != nil || c.LedgerSequence == 0 || c.EvidenceRecord.Validate() != nil {
		return checkpointCoordinationInvalidV1("checkpoint Timeline cut is incomplete")
	}
	return nil
}

type CheckpointAttemptSettlementClosureV1 struct {
	Attempt    CheckpointExternalExactRefV1   `json:"attempt"`
	Begun      bool                           `json:"begun"`
	Settlement *CheckpointExternalExactRefV1  `json:"settlement,omitempty"`
	Inspection *CheckpointExternalExactRefV1  `json:"inspection,omitempty"`
	Residuals  []CheckpointExternalExactRefV1 `json:"residuals"`
}

func (c CheckpointAttemptSettlementClosureV1) Clone() CheckpointAttemptSettlementClosureV1 {
	if c.Settlement != nil {
		value := *c.Settlement
		c.Settlement = &value
	}
	if c.Inspection != nil {
		value := *c.Inspection
		c.Inspection = &value
	}
	c.Residuals = append([]CheckpointExternalExactRefV1(nil), c.Residuals...)
	return c
}

func (c CheckpointAttemptSettlementClosureV1) Validate() error {
	if c.Attempt.Validate() != nil || (c.Settlement != nil && c.Settlement.Validate() != nil) || (c.Inspection != nil && c.Inspection.Validate() != nil) {
		return checkpointCoordinationInvalidV1("checkpoint Attempt settlement closure is incomplete")
	}
	if c.Settlement != nil && !c.Begun {
		return checkpointCoordinationInvalidV1("checkpoint settlement cannot precede Begin")
	}
	if c.Begun && c.Settlement == nil && (c.Inspection == nil || len(c.Residuals) == 0) {
		return core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "begun checkpoint Attempt requires Settlement or Inspection plus Residual")
	}
	return validateCheckpointExactSetV1(c.Attempt, c.Residuals)
}

// CheckpointManifestInputCurrentProjectionV1 is supplied by current Readers
// owned by Timeline, Context, Runtime, Memory and Knowledge. Application only
// sequences the exact refs; it cannot mint or upgrade them.
type CheckpointManifestInputCurrentProjectionV1 struct {
	Attempt            runtimeports.CheckpointAttemptRefV2    `json:"attempt"`
	Timeline           CheckpointTimelineCutV1                `json:"timeline"`
	ContextGeneration  CheckpointExternalExactRefV1           `json:"context_generation"`
	ContextFrames      []CheckpointExternalExactRefV1         `json:"context_frames"`
	AttemptSettlements []CheckpointAttemptSettlementClosureV1 `json:"attempt_settlements"`
	Memory             []CheckpointExternalExactRefV1         `json:"memory"`
	Knowledge          []CheckpointExternalExactRefV1         `json:"knowledge"`
	CheckedUnixNano    int64                                  `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                                  `json:"expires_unix_nano"`
	ProjectionDigest   core.Digest                            `json:"projection_digest"`
}

func (p CheckpointManifestInputCurrentProjectionV1) Clone() CheckpointManifestInputCurrentProjectionV1 {
	p.ContextFrames = append([]CheckpointExternalExactRefV1(nil), p.ContextFrames...)
	p.Memory = append([]CheckpointExternalExactRefV1(nil), p.Memory...)
	p.Knowledge = append([]CheckpointExternalExactRefV1(nil), p.Knowledge...)
	p.AttemptSettlements = make([]CheckpointAttemptSettlementClosureV1, len(p.AttemptSettlements))
	for i := range p.AttemptSettlements {
		p.AttemptSettlements[i] = p.AttemptSettlements[i].Clone()
	}
	return p
}

func (p CheckpointManifestInputCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p.Clone()
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.application.checkpoint-manifest-input", CheckpointCoordinationContractVersionV1, "CheckpointManifestInputCurrentProjectionV1", copy)
}

func (p CheckpointManifestInputCurrentProjectionV1) Validate(now time.Time) error {
	if p.Attempt.Validate() != nil || p.Timeline.Validate() != nil || p.ContextGeneration.Validate() != nil || len(p.ContextFrames) == 0 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) || p.ProjectionDigest.Validate() != nil {
		return checkpointCoordinationInvalidV1("checkpoint Manifest input projection is incomplete or stale")
	}
	anchor := p.ContextGeneration
	sets := [][]CheckpointExternalExactRefV1{p.ContextFrames, p.Memory, p.Knowledge}
	for _, set := range sets {
		if err := validateCheckpointExactSetV1(anchor, set); err != nil {
			return err
		}
	}
	if err := validateCheckpointExactSetV1(anchor, []CheckpointExternalExactRefV1{p.Timeline.EvidenceRecord}); err != nil {
		return err
	}
	for _, closure := range p.AttemptSettlements {
		if err := closure.Validate(); err != nil {
			return err
		}
		refs := []CheckpointExternalExactRefV1{closure.Attempt}
		if closure.Settlement != nil {
			refs = append(refs, *closure.Settlement)
		}
		if closure.Inspection != nil {
			refs = append(refs, *closure.Inspection)
		}
		refs = append(refs, closure.Residuals...)
		if err := validateCheckpointExactSetV1(anchor, refs); err != nil {
			return err
		}
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Manifest input projection drifted")
	}
	return nil
}

func SealCheckpointManifestInputCurrentProjectionV1(p CheckpointManifestInputCurrentProjectionV1, now time.Time) (CheckpointManifestInputCurrentProjectionV1, error) {
	p = p.Clone()
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return CheckpointManifestInputCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type CheckpointParticipantCommitV1 struct {
	RuntimeClosure  runtimeports.CheckpointParticipantClosureRefV2 `json:"runtime_closure"`
	ParticipantFact CheckpointExternalExactRefV1                   `json:"participant_fact"`
	Snapshot        CheckpointExternalExactRefV1                   `json:"snapshot"`
	Coverage        CheckpointExternalExactRefV1                   `json:"coverage"`
	Evidence        []CheckpointExternalExactRefV1                 `json:"evidence"`
	Residuals       []CheckpointExternalExactRefV1                 `json:"residuals"`
}

// CheckpointParticipantOwnerCandidateV1 is an Owner-sealed local result. It
// deliberately excludes Runtime closure and Evidence, which are produced only
// by the governed phase chain.
type CheckpointParticipantOwnerCandidateV1 struct {
	Participant      runtimeports.CheckpointParticipantRefV2 `json:"participant"`
	ParticipantFact  CheckpointExternalExactRefV1            `json:"participant_fact"`
	Snapshot         CheckpointExternalExactRefV1            `json:"snapshot"`
	Coverage         CheckpointExternalExactRefV1            `json:"coverage"`
	CheckedUnixNano  int64                                   `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                   `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                             `json:"projection_digest"`
}

func (c CheckpointParticipantOwnerCandidateV1) DigestV1() (core.Digest, error) {
	copy := c
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.application.checkpoint-participant-owner", CheckpointCoordinationContractVersionV1, "CheckpointParticipantOwnerCandidateV1", copy)
}

func (c CheckpointParticipantOwnerCandidateV1) Validate(work CheckpointParticipantWorkRequestV1, now time.Time) error {
	if c.Participant.Validate() != nil || c.Participant != work.Participant || c.ParticipantFact.Validate() != nil || c.Snapshot.Validate() != nil || c.Coverage.Validate() != nil || c.CheckedUnixNano <= 0 || c.ExpiresUnixNano <= c.CheckedUnixNano || now.IsZero() || now.UnixNano() < c.CheckedUnixNano || !now.Before(time.Unix(0, c.ExpiresUnixNano)) || c.ExpiresUnixNano > work.NotAfter || c.ProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint Participant Owner candidate is incomplete or stale")
	}
	refs := []CheckpointExternalExactRefV1{c.Coverage, c.Snapshot}
	slices.SortFunc(refs, compareCheckpointExternalRefV1)
	if err := validateCheckpointExactSetV1(c.ParticipantFact, refs); err != nil {
		return err
	}
	digest, err := c.DigestV1()
	if err != nil || digest != c.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Participant Owner candidate drifted")
	}
	return nil
}

func SealCheckpointParticipantOwnerCandidateV1(c CheckpointParticipantOwnerCandidateV1, work CheckpointParticipantWorkRequestV1, now time.Time) (CheckpointParticipantOwnerCandidateV1, error) {
	c.ProjectionDigest = ""
	digest, err := c.DigestV1()
	if err != nil {
		return CheckpointParticipantOwnerCandidateV1{}, err
	}
	c.ProjectionDigest = digest
	return c, c.Validate(work, now)
}

func (c CheckpointParticipantCommitV1) Clone() CheckpointParticipantCommitV1 {
	c.Evidence = append([]CheckpointExternalExactRefV1(nil), c.Evidence...)
	c.Residuals = append([]CheckpointExternalExactRefV1(nil), c.Residuals...)
	return c
}

func (c CheckpointParticipantCommitV1) Validate(expected runtimeports.CheckpointParticipantRefV2) error {
	if c.RuntimeClosure.Validate() != nil || c.RuntimeClosure.Participant != expected || c.RuntimeClosure.Terminal == nil || c.RuntimeClosure.Terminal.Phase != runtimeports.CheckpointPhaseCommitV2 || c.ParticipantFact.Validate() != nil || c.Snapshot.Validate() != nil || c.Coverage.Validate() != nil || len(c.Evidence) == 0 || len(c.Residuals) != 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint Participant is not exact committed and residual-free")
	}
	refs := append([]CheckpointExternalExactRefV1{c.Snapshot, c.Coverage}, c.Evidence...)
	slices.SortFunc(refs, compareCheckpointExternalRefV1)
	return validateCheckpointExactSetV1(c.ParticipantFact, refs)
}

func (c CheckpointParticipantCommitV1) ValidateForAttemptV1(expected runtimeports.CheckpointParticipantRefV2, attempt runtimeports.CheckpointAttemptRefV2) error {
	if err := c.Validate(expected); err != nil {
		return err
	}
	if attempt.Validate() != nil || c.RuntimeClosure.Prepare.DomainResult.Attempt != attempt || c.RuntimeClosure.Terminal == nil || c.RuntimeClosure.Terminal.DomainResult.Attempt != attempt {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Participant closure belongs to another Attempt")
	}
	return nil
}

type StartCheckpointCoordinationRequestV1 struct {
	StableID       string                                        `json:"stable_id"`
	Gate           AcquireCheckpointGateRequestV1                `json:"gate"`
	RuntimeCreate  runtimeports.CreateCheckpointAttemptRequestV2 `json:"runtime_create"`
	CutID          string                                        `json:"cut_id"`
	ManifestID     string                                        `json:"manifest_id"`
	ManifestSealID string                                        `json:"manifest_seal_id"`
	NotAfter       int64                                         `json:"not_after_unix_nano"`
}

func (r StartCheckpointCoordinationRequestV1) Validate(now time.Time) error {
	if !validCheckpointCoordinationIDV1(r.StableID) || !validCheckpointCoordinationIDV1(r.CutID) || !validCheckpointCoordinationIDV1(r.ManifestID) || !validCheckpointCoordinationIDV1(r.ManifestSealID) || r.Gate.Validate(now) != nil || r.RuntimeCreate.Validate() != nil || r.NotAfter <= 0 || !now.Before(time.Unix(0, r.NotAfter)) || r.Gate.Scope != r.RuntimeCreate.Scope || r.Gate.RunID != r.RuntimeCreate.RunID || r.Gate.Scope.Identity.TenantID != r.RuntimeCreate.Scope.Identity.TenantID {
		return checkpointCoordinationInvalidV1("checkpoint coordination request is invalid or cross-scoped")
	}
	return nil
}

func validateCheckpointExactSetV1(anchor CheckpointExternalExactRefV1, values []CheckpointExternalExactRefV1) error {
	if anchor.Validate() != nil {
		return checkpointCoordinationInvalidV1("checkpoint exact-ref anchor is invalid")
	}
	if !slices.IsSortedFunc(values, compareCheckpointExternalRefV1) {
		return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "checkpoint exact refs are not canonical")
	}
	for i, value := range values {
		if value.Validate() != nil || value.TenantID != anchor.TenantID || value.ScopeDigest != anchor.ScopeDigest || value.RunID != anchor.RunID || (i > 0 && compareCheckpointExternalRefV1(values[i-1], value) == 0) {
			return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint exact refs contain a scope splice or duplicate")
		}
	}
	return nil
}

func compareCheckpointExternalRefV1(a, b CheckpointExternalExactRefV1) int {
	for _, pair := range [][2]string{{string(a.Owner.ComponentID), string(b.Owner.ComponentID)}, {a.FactKind, b.FactKind}, {a.ID, b.ID}} {
		if c := strings.Compare(pair[0], pair[1]); c != 0 {
			return c
		}
	}
	if a.Revision < b.Revision {
		return -1
	}
	if a.Revision > b.Revision {
		return 1
	}
	return strings.Compare(string(a.Digest), string(b.Digest))
}

func validCheckpointCoordinationIDV1(value string) bool {
	return strings.TrimSpace(value) == value && value != "" && len(value) <= 192
}
func checkpointCoordinationInvalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}

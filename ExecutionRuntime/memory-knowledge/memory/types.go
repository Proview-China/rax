// Package memory implements the Wave 1 Memory domain owner. It deliberately
// contains no Runtime, Context, Review, Assembly, remote-store, or connector
// adapter. External orchestration settles only by opaque references.
package memory

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

type CandidateKind string

const (
	CandidateCreate     CandidateKind = "create"
	CandidateCorrection CandidateKind = "correction"
	CandidateTombstone  CandidateKind = "tombstone"
	CandidatePin        CandidateKind = "pin"
	CandidateArchive    CandidateKind = "archive"
	CandidateForget     CandidateKind = "forget"
	CandidateMerge      CandidateKind = "merge"
)

type Candidate struct {
	Envelope             contract.Envelope    `json:"envelope"`
	Kind                 CandidateKind        `json:"kind"`
	ProducerRef          contract.Ref         `json:"producer_ref"`
	SourceEpoch          uint64               `json:"source_epoch"`
	SourceSequence       uint64               `json:"source_sequence"`
	Scope                string               `json:"scope"`
	Subject              string               `json:"subject"`
	ContentRef           *contract.ContentRef `json:"content_ref,omitempty"`
	SourceRefs           []contract.Ref       `json:"source_refs"`
	EvidenceRefs         []contract.Ref       `json:"evidence_refs"`
	Sensitivity          string               `json:"sensitivity"`
	RetentionRef         contract.Ref         `json:"retention_ref,omitempty"`
	LegalHoldRef         contract.Ref         `json:"legal_hold_ref,omitempty"`
	DecayPolicyRef       contract.Ref         `json:"decay_policy_ref,omitempty"`
	DecayHalfLifeSeconds uint64               `json:"decay_half_life_seconds,omitempty"`
	MergeSourceRefs      []contract.Ref       `json:"merge_source_refs"`
	RiskFlags            []string             `json:"risk_flags"`
	TargetRecordRef      contract.Ref         `json:"target_record_ref,omitempty"`
}

func SealCandidate(c Candidate) Candidate {
	sealed, err := sealCandidate(c)
	if err != nil {
		// SealCandidate is also used at public construction boundaries. A value
		// that cannot be represented canonically stays unsealed and is rejected
		// by Validate instead of turning untrusted input into a panic.
		c.Envelope.Digest = ""
		return c
	}
	return sealed
}

func sealCandidate(c Candidate) (Candidate, error) {
	c.SourceRefs = contract.NormalizeRefs(c.SourceRefs)
	c.EvidenceRefs = contract.NormalizeRefs(c.EvidenceRefs)
	c.MergeSourceRefs = normalizeMergeRefs(c.MergeSourceRefs)
	c.RiskFlags = sortedUnique(c.RiskFlags)
	c.Envelope.Causation = contract.NormalizeRefs(c.Envelope.Causation)
	c.Envelope.Digest = ""
	digest, err := contract.Digest(c)
	if err != nil {
		return Candidate{}, fmt.Errorf("canonical memory candidate: %w", err)
	}
	c.Envelope.Digest = digest
	return c, nil
}

func (c Candidate) Ref() contract.Ref {
	return contract.Ref{ID: c.Envelope.ID, Revision: c.Envelope.Revision, Digest: c.Envelope.Digest}
}

func (c Candidate) Validate(now time.Time) error {
	if err := c.Envelope.ValidateCurrent(now); err != nil {
		return err
	}
	if strings.TrimSpace(c.Scope) == "" || strings.TrimSpace(c.Subject) == "" || strings.TrimSpace(c.Sensitivity) == "" || len(c.SourceRefs) == 0 || c.SourceEpoch == 0 || c.SourceSequence == 0 {
		return fmt.Errorf("%w: incomplete memory candidate", contract.ErrInvalidArgument)
	}
	if err := c.ProducerRef.Validate(); err != nil {
		return err
	}
	if !slices.Equal(c.SourceRefs, contract.NormalizeRefs(c.SourceRefs)) || !slices.Equal(c.EvidenceRefs, contract.NormalizeRefs(c.EvidenceRefs)) {
		return fmt.Errorf("%w: refs must be canonical", contract.ErrInvalidArgument)
	}
	for _, ref := range append(slices.Clone(c.SourceRefs), c.EvidenceRefs...) {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	switch c.Kind {
	case CandidateCreate:
		if c.ContentRef == nil || c.TargetRecordRef.ID != "" {
			return fmt.Errorf("%w: create candidate shape", contract.ErrInvalidArgument)
		}
	case CandidateCorrection:
		if c.ContentRef == nil || c.TargetRecordRef.Validate() != nil {
			return fmt.Errorf("%w: correction candidate shape", contract.ErrInvalidArgument)
		}
	case CandidateTombstone:
		if c.ContentRef != nil || c.TargetRecordRef.Validate() != nil {
			return fmt.Errorf("%w: tombstone candidate must not carry content", contract.ErrInvalidArgument)
		}
	case CandidatePin:
		if c.ContentRef != nil || c.TargetRecordRef.Validate() != nil || c.RetentionRef.Validate() != nil {
			return fmt.Errorf("%w: pin candidate shape", contract.ErrInvalidArgument)
		}
	case CandidateArchive, CandidateForget:
		if c.ContentRef != nil || c.TargetRecordRef.Validate() != nil {
			return fmt.Errorf("%w: lifecycle candidate shape", contract.ErrInvalidArgument)
		}
	case CandidateMerge:
		if c.ContentRef == nil || c.TargetRecordRef.Validate() != nil || len(c.MergeSourceRefs) < 2 || !containsExactRef(c.MergeSourceRefs, c.TargetRecordRef) {
			return fmt.Errorf("%w: merge candidate shape", contract.ErrInvalidArgument)
		}
	default:
		return fmt.Errorf("%w: unknown candidate kind", contract.ErrInvalidArgument)
	}
	if c.ContentRef != nil {
		if err := c.ContentRef.Validate(); err != nil {
			return err
		}
	}
	for _, optional := range []contract.Ref{c.RetentionRef, c.LegalHoldRef, c.DecayPolicyRef} {
		if optional != (contract.Ref{}) {
			if err := optional.Validate(); err != nil {
				return err
			}
		}
	}
	if (c.DecayPolicyRef == (contract.Ref{})) != (c.DecayHalfLifeSeconds == 0) {
		return fmt.Errorf("%w: decay policy", contract.ErrInvalidArgument)
	}
	if !slices.Equal(c.MergeSourceRefs, normalizeMergeRefs(c.MergeSourceRefs)) {
		return fmt.Errorf("%w: merge sources not canonical", contract.ErrInvalidArgument)
	}
	if !slices.Equal(c.RiskFlags, sortedUnique(c.RiskFlags)) {
		return fmt.Errorf("%w: risk flags", contract.ErrInvalidArgument)
	}
	seenMerge := make(map[string]struct{}, len(c.MergeSourceRefs))
	for _, ref := range c.MergeSourceRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
		if _, ok := seenMerge[ref.ID]; ok {
			return fmt.Errorf("%w: duplicate merge record", contract.ErrEvidenceConflict)
		}
		seenMerge[ref.ID] = struct{}{}
	}
	unsigned := c
	unsigned.Envelope.Digest = ""
	digest, err := contract.Digest(unsigned)
	if err != nil {
		return fmt.Errorf("canonical memory candidate: %w", err)
	}
	if digest != c.Envelope.Digest {
		return fmt.Errorf("%w: candidate digest", contract.ErrEvidenceConflict)
	}
	return nil
}

type AdmissionDecision string

const (
	AdmissionRejected       AdmissionDecision = "rejected"
	AdmissionMerged         AdmissionDecision = "merged"
	AdmissionReviewRequired AdmissionDecision = "review_required"
	AdmissionCommitReady    AdmissionDecision = "commit_ready"
)

type AdmissionRequest struct {
	ID               string
	CandidateRef     contract.Ref
	Decision         AdmissionDecision
	MergeTarget      contract.Ref
	Reason           string
	ExpiresAt        time.Time
	ExpectedRevision contract.ExpectedRevision
}

type AdmissionFact struct {
	Ref          contract.Ref         `json:"ref"`
	Owner        contract.OwnerDomain `json:"owner"`
	TenantID     string               `json:"tenant_id"`
	CandidateRef contract.Ref         `json:"candidate_ref"`
	Decision     AdmissionDecision    `json:"decision"`
	MergeTarget  contract.Ref         `json:"merge_target,omitempty"`
	Reason       string               `json:"reason"`
	CreatedAt    time.Time            `json:"created_at"`
	ExpiresAt    time.Time            `json:"expires_at"`
}

func (a AdmissionFact) Current(now time.Time) bool { return a.ExpiresAt.After(now) }

type RecordStatus string

const (
	RecordActive     RecordStatus = "active"
	RecordSuperseded RecordStatus = "superseded"
	RecordExpired    RecordStatus = "expired"
	RecordArchived   RecordStatus = "archived"
	RecordTombstoned RecordStatus = "tombstoned"
)

type Record struct {
	Ref                  contract.Ref         `json:"ref"`
	Owner                contract.OwnerDomain `json:"owner"`
	TenantID             string               `json:"tenant_id"`
	IdentityID           string               `json:"identity_id"`
	AuthorityRef         contract.Ref         `json:"authority_ref"`
	AuthorityEpoch       uint64               `json:"authority_epoch"`
	PolicyRef            contract.Ref         `json:"policy_ref"`
	Purpose              string               `json:"purpose"`
	ActionScopeDigest    string               `json:"action_scope_digest"`
	Kind                 string               `json:"kind"`
	Scope                string               `json:"scope"`
	Subject              string               `json:"subject"`
	ContentRef           *contract.ContentRef `json:"content_ref,omitempty"`
	SourceRefs           []contract.Ref       `json:"source_refs"`
	EvidenceRefs         []contract.Ref       `json:"evidence_refs"`
	Sensitivity          string               `json:"sensitivity"`
	Pinned               bool                 `json:"pinned"`
	RetentionRef         contract.Ref         `json:"retention_ref,omitempty"`
	LegalHoldRef         contract.Ref         `json:"legal_hold_ref,omitempty"`
	DecayPolicyRef       contract.Ref         `json:"decay_policy_ref,omitempty"`
	DecayHalfLifeSeconds uint64               `json:"decay_half_life_seconds,omitempty"`
	MergeSourceRefs      []contract.Ref       `json:"merge_source_refs"`
	Status               RecordStatus         `json:"status"`
	Corrects             contract.Ref         `json:"corrects,omitempty"`
	Watermark            uint64               `json:"watermark"`
	CreatedAt            time.Time            `json:"created_at"`
	ExpiresAt            time.Time            `json:"expires_at"`
}

type MergeFact struct {
	Ref          contract.Ref         `json:"ref"`
	Owner        contract.OwnerDomain `json:"owner"`
	TenantID     string               `json:"tenant_id"`
	TargetRef    contract.Ref         `json:"target_ref"`
	SourceRefs   []contract.Ref       `json:"source_refs"`
	CandidateRef contract.Ref         `json:"candidate_ref"`
	CreatedAt    time.Time            `json:"created_at"`
}

func (f MergeFact) Validate() error {
	if f.Owner != contract.OwnerMemory || strings.TrimSpace(f.TenantID) == "" || f.Ref.Validate() != nil || f.TargetRef.Validate() != nil || f.CandidateRef.Validate() != nil || len(f.SourceRefs) < 2 || f.CreatedAt.IsZero() || !slices.Equal(f.SourceRefs, contract.NormalizeRefs(f.SourceRefs)) {
		return fmt.Errorf("%w: merge fact", contract.ErrInvalidArgument)
	}
	for _, ref := range f.SourceRefs {
		if ref.Validate() != nil {
			return contract.ErrInvalidArgument
		}
	}
	copy := f
	copy.Ref.Digest = ""
	digest, err := contract.Digest(copy)
	if err != nil {
		return err
	}
	if digest != f.Ref.Digest {
		return fmt.Errorf("%w: merge fact digest", contract.ErrEvidenceConflict)
	}
	return nil
}

func containsExactRef(refs []contract.Ref, target contract.Ref) bool {
	for _, ref := range refs {
		if contract.SameRef(ref, target) {
			return true
		}
	}
	return false
}
func normalizeMergeRefs(in []contract.Ref) []contract.Ref {
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b contract.Ref) int {
		if c := strings.Compare(a.ID, b.ID); c != 0 {
			return c
		}
		if a.Revision < b.Revision {
			return -1
		}
		if a.Revision > b.Revision {
			return 1
		}
		return strings.Compare(a.Digest, b.Digest)
	})
	if out == nil {
		return []contract.Ref{}
	}
	return out
}

func (r Record) Current(now time.Time) bool {
	return r.Status == RecordActive && r.ExpiresAt.After(now)
}

type Watermark struct {
	Ref      contract.Ref `json:"ref"`
	TenantID string       `json:"tenant_id"`
	Sequence uint64       `json:"sequence"`
}

type ProjectionState string

const (
	ProjectionReady   ProjectionState = "ready"
	ProjectionPartial ProjectionState = "partial"
	ProjectionStale   ProjectionState = "stale"
)

type Projection struct {
	Ref            contract.Ref         `json:"ref"`
	Owner          contract.OwnerDomain `json:"owner"`
	TenantID       string               `json:"tenant_id"`
	RecordRef      contract.Ref         `json:"record_ref"`
	Kind           string               `json:"kind"`
	BuilderVersion string               `json:"builder_version"`
	State          ProjectionState      `json:"state"`
	Coverage       contract.Coverage    `json:"coverage"`
	CreatedAt      time.Time            `json:"created_at"`
	ExpiresAt      time.Time            `json:"expires_at"`
}

func SealProjection(p Projection) Projection {
	sealed, err := sealProjection(p)
	if err != nil {
		p.Ref.Digest = ""
		return p
	}
	return sealed
}

func sealProjection(p Projection) (Projection, error) {
	p.Owner = contract.OwnerMemory
	p.Coverage.ProjectionRefs = contract.NormalizeRefs(p.Coverage.ProjectionRefs)
	p.Ref.Digest = ""
	digest, err := contract.Digest(p)
	if err != nil {
		return Projection{}, fmt.Errorf("canonical memory projection: %w", err)
	}
	p.Ref.Digest = digest
	return p, nil
}

type View struct {
	Ref            contract.Ref         `json:"ref"`
	Owner          contract.OwnerDomain `json:"owner"`
	TenantID       string               `json:"tenant_id"`
	PrincipalID    string               `json:"principal_id"`
	AuthorityRef   contract.Ref         `json:"authority_ref"`
	AuthorityEpoch uint64               `json:"authority_epoch"`
	PolicyRef      contract.Ref         `json:"policy_ref"`
	Purpose        string               `json:"purpose"`
	Scopes         []string             `json:"scopes"`
	SensitivityMax string               `json:"sensitivity_max"`
	WatermarkRef   contract.Ref         `json:"watermark_ref"`
	ProjectionRefs []contract.Ref       `json:"projection_refs"`
	CreatedAt      time.Time            `json:"created_at"`
	ExpiresAt      time.Time            `json:"expires_at"`
}

func SealView(v View) View {
	sealed, err := sealView(v)
	if err != nil {
		v.Ref.Digest = ""
		return v
	}
	return sealed
}

func sealView(v View) (View, error) {
	v.Owner = contract.OwnerMemory
	v.Scopes = sortedUnique(v.Scopes)
	v.ProjectionRefs = contract.NormalizeRefs(v.ProjectionRefs)
	v.Ref.Digest = ""
	digest, err := contract.Digest(v)
	if err != nil {
		return View{}, fmt.Errorf("canonical memory view: %w", err)
	}
	v.Ref.Digest = digest
	return v, nil
}

type Access struct {
	TenantID       string
	IdentityID     string
	AuthorityRef   contract.Ref
	AuthorityEpoch uint64
	PolicyRef      contract.Ref
}

func (a Access) validate() error {
	if strings.TrimSpace(a.TenantID) == "" || strings.TrimSpace(a.IdentityID) == "" || a.AuthorityEpoch == 0 || a.AuthorityRef.Validate() != nil || a.PolicyRef.Validate() != nil {
		return fmt.Errorf("%w: incomplete access", contract.ErrInvalidArgument)
	}
	return nil
}

type CommitRequest struct {
	TenantID         string
	AttemptID        string
	ResultID         string
	RecordID         string
	CandidateRef     contract.Ref
	AdmissionRef     contract.Ref
	OperationRef     contract.Ref
	ExpectedRevision contract.ExpectedRevision
}

type InspectionState string

const (
	InspectionApplied    InspectionState = "applied"
	InspectionNotApplied InspectionState = "confirmed_not_applied"
	InspectionIncomplete InspectionState = "incomplete"
)

type CommitInspection struct {
	Ref          contract.Ref         `json:"ref"`
	Owner        contract.OwnerDomain `json:"owner"`
	TenantID     string               `json:"tenant_id"`
	IdentityID   string               `json:"identity_id"`
	AttemptID    string               `json:"attempt_id"`
	OperationRef contract.Ref         `json:"operation_ref"`
	RecordRef    contract.Ref         `json:"record_ref"`
	State        InspectionState      `json:"state"`
	ObservedAt   time.Time            `json:"observed_at"`
}

type SettlementRequest struct {
	TenantID         string
	Association      contract.DomainResultAssociation
	Settlement       contract.RuntimeSettlementRef
	ExpectedRevision contract.ExpectedRevision
}

func sortedUnique(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}

func hasBlockingRisk(flags []string) bool {
	for _, flag := range flags {
		switch flag {
		case "secret_plaintext", "pii_unapproved", "prompt_injection", "poisoning_suspected", "unsettled_effect", "unknown_outcome", "chain_of_thought":
			return true
		}
	}
	return false
}

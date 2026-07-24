package knowledge

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

type SourceState string

const (
	SourceRegistered SourceState = "registered"
	SourceAvailable  SourceState = "available"
	SourceStale      SourceState = "stale"
	SourceWithdrawn  SourceState = "withdrawn"
	SourceDeprecated SourceState = "deprecated"
)

type PackageState string

const (
	PackageReady     PackageState = "ready"
	PackageWithdrawn PackageState = "withdrawn"
)

type CandidateKind string

const (
	CandidateRecord     CandidateKind = "record"
	CandidateCorrection CandidateKind = "correction"
	CandidateWithdraw   CandidateKind = "withdraw"
)

type AdmissionDecision string

const (
	AdmissionRejected        AdmissionDecision = "rejected"
	AdmissionMerged          AdmissionDecision = "merged"
	AdmissionConflictPending AdmissionDecision = "conflict_pending"
	AdmissionReviewRequired  AdmissionDecision = "review_required"
	AdmissionCommitReady     AdmissionDecision = "commit_ready"
)

type RecordStatus string

const (
	RecordActive    RecordStatus = "active"
	RecordWithdrawn RecordStatus = "withdrawn"
)

type TrustState string

const (
	TrustUnverified      TrustState = "unverified"
	TrustSourceSupported TrustState = "source_supported"
	TrustConflicted      TrustState = "conflicted"
	TrustWithdrawn       TrustState = "withdrawn"
)

type SnapshotState string

const (
	SnapshotReady     SnapshotState = "ready"
	SnapshotPublished SnapshotState = "published"
)

type ProjectionState string

const (
	ProjectionReady   ProjectionState = "ready"
	ProjectionPartial ProjectionState = "partial"
	ProjectionStale   ProjectionState = "stale"
)

type AttemptState string

const (
	AttemptBegun   AttemptState = "begun"
	AttemptApplied AttemptState = "applied"
	AttemptFailed  AttemptState = "failed"
)

type InspectionOutcome string

const (
	InspectionApplied    InspectionOutcome = "applied"
	InspectionNotApplied InspectionOutcome = "not_applied"
)

// Access is an already-governed domain view. Wave 1 does not create or
// interpret Authority/Policy facts; it requires their exact refs on every
// authoritative read or write.
type Access struct {
	TenantID     string       `json:"tenant_id"`
	AuthorityRef contract.Ref `json:"authority_ref"`
	PolicyRef    contract.Ref `json:"policy_ref"`
}

func (a Access) Validate() error {
	if strings.TrimSpace(a.TenantID) == "" {
		return fmt.Errorf("%w: tenant id required", contract.ErrInvalidArgument)
	}
	if err := a.AuthorityRef.Validate(); err != nil {
		return err
	}
	return a.PolicyRef.Validate()
}

type Source struct {
	Ref           contract.Ref         `json:"ref"`
	TenantID      string               `json:"tenant_id"`
	Owner         contract.OwnerDomain `json:"owner"`
	Version       string               `json:"version"`
	AssetRef      contract.Ref         `json:"asset_ref"`
	ContentDigest string               `json:"content_digest"`
	AuthorityRef  contract.Ref         `json:"authority_ref"`
	PolicyRef     contract.Ref         `json:"policy_ref"`
	License       string               `json:"license"`
	Scope         string               `json:"scope"`
	Sensitivity   string               `json:"sensitivity"`
	State         SourceState          `json:"state"`
	Provenance    []contract.Ref       `json:"provenance"`
	AcquiredAt    time.Time            `json:"acquired_at"`
	ValidFrom     time.Time            `json:"valid_from"`
	ValidTo       time.Time            `json:"valid_to"`
	UpdatedAt     time.Time            `json:"updated_at"`
}

type SourceInput struct {
	TenantID      string
	ID            string
	Version       string
	AssetRef      contract.Ref
	ContentDigest string
	AuthorityRef  contract.Ref
	PolicyRef     contract.Ref
	License       string
	Scope         string
	Sensitivity   string
	State         SourceState
	Provenance    []contract.Ref
	AcquiredAt    time.Time
	ValidFrom     time.Time
	ValidTo       time.Time
}

type Package struct {
	Ref          contract.Ref         `json:"ref"`
	TenantID     string               `json:"tenant_id"`
	Owner        contract.OwnerDomain `json:"owner"`
	Version      string               `json:"version"`
	SourceRefs   []contract.Ref       `json:"source_refs"`
	AuthorityRef contract.Ref         `json:"authority_ref"`
	PolicyRef    contract.Ref         `json:"policy_ref"`
	License      string               `json:"license"`
	Coverage     contract.Coverage    `json:"coverage"`
	State        PackageState         `json:"state"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
}

type PackageInput struct {
	TenantID     string
	ID           string
	Version      string
	SourceRefs   []contract.Ref
	AuthorityRef contract.Ref
	PolicyRef    contract.Ref
	License      string
	Coverage     contract.Coverage
	State        PackageState
}

type RecordDraft struct {
	ID            string              `json:"id"`
	PackageRef    contract.Ref        `json:"package_ref"`
	ContentRef    contract.ContentRef `json:"content_ref"`
	SourceRefs    []contract.Ref      `json:"source_refs"`
	EvidenceRefs  []contract.Ref      `json:"evidence_refs"`
	Scope         string              `json:"scope"`
	Subject       string              `json:"subject"`
	Sensitivity   string              `json:"sensitivity"`
	License       string              `json:"license"`
	TrustState    TrustState          `json:"trust_state"`
	ConflictGroup string              `json:"conflict_group,omitempty"`
	ValidFrom     time.Time           `json:"valid_from"`
	ValidTo       time.Time           `json:"valid_to"`
}

type Candidate struct {
	Ref            contract.Ref         `json:"ref"`
	TenantID       string               `json:"tenant_id"`
	Owner          contract.OwnerDomain `json:"owner"`
	ProducerID     string               `json:"producer_id"`
	SourceEpoch    uint64               `json:"source_epoch"`
	SourceSequence uint64               `json:"source_sequence"`
	Kind           CandidateKind        `json:"kind"`
	TargetRef      contract.Ref         `json:"target_ref,omitempty"`
	Draft          RecordDraft          `json:"draft"`
	PayloadDigest  string               `json:"payload_digest"`
	EvidenceRefs   []contract.Ref       `json:"evidence_refs"`
	RiskFlags      []string             `json:"risk_flags"`
	CreatedAt      time.Time            `json:"created_at"`
	ExpiresAt      time.Time            `json:"expires_at"`
}

type CandidateInput struct {
	TenantID       string
	ID             string
	ProducerID     string
	SourceEpoch    uint64
	SourceSequence uint64
	Kind           CandidateKind
	TargetRef      contract.Ref
	Draft          RecordDraft
	PayloadDigest  string
	EvidenceRefs   []contract.Ref
	RiskFlags      []string
	TTL            time.Duration
}

type Admission struct {
	Ref          contract.Ref         `json:"ref"`
	TenantID     string               `json:"tenant_id"`
	Owner        contract.OwnerDomain `json:"owner"`
	CandidateRef contract.Ref         `json:"candidate_ref"`
	Decision     AdmissionDecision    `json:"decision"`
	Reason       string               `json:"reason"`
	CreatedAt    time.Time            `json:"created_at"`
	ExpiresAt    time.Time            `json:"expires_at"`
}

type Record struct {
	Ref           contract.Ref         `json:"ref"`
	TenantID      string               `json:"tenant_id"`
	Owner         contract.OwnerDomain `json:"owner"`
	PackageRef    contract.Ref         `json:"package_ref"`
	AuthorityRef  contract.Ref         `json:"authority_ref"`
	PolicyRef     contract.Ref         `json:"policy_ref"`
	ContentRef    contract.ContentRef  `json:"content_ref"`
	SourceRefs    []contract.Ref       `json:"source_refs"`
	EvidenceRefs  []contract.Ref       `json:"evidence_refs"`
	Scope         string               `json:"scope"`
	Subject       string               `json:"subject"`
	Sensitivity   string               `json:"sensitivity"`
	License       string               `json:"license"`
	TrustState    TrustState           `json:"trust_state"`
	ConflictGroup string               `json:"conflict_group,omitempty"`
	Status        RecordStatus         `json:"status"`
	Corrects      contract.Ref         `json:"corrects,omitempty"`
	WithdrawnBy   contract.Ref         `json:"withdrawn_by,omitempty"`
	ValidFrom     time.Time            `json:"valid_from"`
	ValidTo       time.Time            `json:"valid_to"`
	TransactionAt time.Time            `json:"transaction_at"`
}

type Tombstone struct {
	Ref        contract.Ref         `json:"ref"`
	TenantID   string               `json:"tenant_id"`
	Owner      contract.OwnerDomain `json:"owner"`
	TargetKind string               `json:"target_kind"`
	TargetRef  contract.Ref         `json:"target_ref"`
	Reason     string               `json:"reason"`
	CreatedAt  time.Time            `json:"created_at"`
}

type Projection struct {
	Ref            contract.Ref         `json:"ref"`
	TenantID       string               `json:"tenant_id"`
	Owner          contract.OwnerDomain `json:"owner"`
	Kind           string               `json:"kind"`
	SnapshotRef    contract.Ref         `json:"snapshot_ref,omitempty"`
	RecordRefs     []contract.Ref       `json:"record_refs"`
	BuilderVersion string               `json:"builder_version"`
	Coverage       contract.Coverage    `json:"coverage"`
	State          ProjectionState      `json:"state"`
	CreatedAt      time.Time            `json:"created_at"`
	ExpiresAt      time.Time            `json:"expires_at"`
}

type ProjectionInput struct {
	TenantID       string
	ID             string
	Kind           string
	SnapshotRef    contract.Ref
	RecordRefs     []contract.Ref
	BuilderVersion string
	Coverage       contract.Coverage
	State          ProjectionState
	TTL            time.Duration
}

type Snapshot struct {
	Ref            contract.Ref         `json:"ref"`
	TenantID       string               `json:"tenant_id"`
	Owner          contract.OwnerDomain `json:"owner"`
	Version        string               `json:"version"`
	SourceRefs     []contract.Ref       `json:"source_refs"`
	PackageRefs    []contract.Ref       `json:"package_refs"`
	RecordRefs     []contract.Ref       `json:"record_refs"`
	ProjectionRefs []contract.Ref       `json:"projection_refs"`
	AuthorityRefs  []contract.Ref       `json:"authority_refs"`
	PolicyRefs     []contract.Ref       `json:"policy_refs"`
	Coverage       contract.Coverage    `json:"coverage"`
	ManifestDigest string               `json:"manifest_digest"`
	State          SnapshotState        `json:"state"`
	BuiltFrom      contract.Ref         `json:"built_from,omitempty"`
	Previous       contract.Ref         `json:"previous,omitempty"`
	BuiltAt        time.Time            `json:"built_at"`
	PublishedAt    time.Time            `json:"published_at,omitempty"`
}

type SnapshotInput struct {
	TenantID       string
	ID             string
	Version        string
	SourceRefs     []contract.Ref
	PackageRefs    []contract.Ref
	RecordRefs     []contract.Ref
	ProjectionRefs []contract.Ref
	Coverage       contract.Coverage
}

type SnapshotPointer struct {
	Ref       contract.Ref         `json:"ref"`
	TenantID  string               `json:"tenant_id"`
	Owner     contract.OwnerDomain `json:"owner"`
	TargetRef contract.Ref         `json:"target_ref"`
	Previous  contract.Ref         `json:"previous,omitempty"`
	UpdatedAt time.Time            `json:"updated_at"`
}

type View struct {
	Ref             contract.Ref         `json:"ref"`
	TenantID        string               `json:"tenant_id"`
	Owner           contract.OwnerDomain `json:"owner"`
	SnapshotRef     contract.Ref         `json:"snapshot_ref"`
	AuthorityRef    contract.Ref         `json:"authority_ref"`
	PolicyRef       contract.Ref         `json:"policy_ref"`
	ProjectionRefs  []contract.Ref       `json:"projection_refs"`
	Scopes          []string             `json:"scopes"`
	AllowedLicenses []string             `json:"allowed_licenses"`
	SensitivityMax  string               `json:"sensitivity_max"`
	Purpose         string               `json:"purpose"`
	CurrentOnly     bool                 `json:"current_only"`
	CreatedAt       time.Time            `json:"created_at"`
	ExpiresAt       time.Time            `json:"expires_at"`
}

type ViewInput struct {
	TenantID        string
	ID              string
	SnapshotRef     contract.Ref
	AuthorityRef    contract.Ref
	PolicyRef       contract.Ref
	ProjectionRefs  []contract.Ref
	Scopes          []string
	AllowedLicenses []string
	SensitivityMax  string
	Purpose         string
	CurrentOnly     bool
	TTL             time.Duration
}

type CommitRequest struct {
	TenantID       string
	AttemptID      string
	OperationRef   contract.Ref
	CandidateRef   contract.Ref
	AdmissionRef   contract.Ref
	ExpectedRecord contract.ExpectedRevision
}

type CommitAttempt struct {
	Ref             contract.Ref              `json:"ref"`
	TenantID        string                    `json:"tenant_id"`
	Owner           contract.OwnerDomain      `json:"owner"`
	OperationRef    contract.Ref              `json:"operation_ref"`
	CandidateRef    contract.Ref              `json:"candidate_ref"`
	AdmissionRef    contract.Ref              `json:"admission_ref"`
	ExpectedRecord  contract.ExpectedRevision `json:"expected_record"`
	State           AttemptState              `json:"state"`
	SubjectRef      contract.Ref              `json:"subject_ref,omitempty"`
	InspectionRef   contract.Ref              `json:"inspection_ref,omitempty"`
	DomainResultRef contract.Ref              `json:"domain_result_ref,omitempty"`
	Failure         string                    `json:"failure,omitempty"`
	BegunAt         time.Time                 `json:"begun_at"`
	UpdatedAt       time.Time                 `json:"updated_at"`
}

type Inspection struct {
	Ref          contract.Ref         `json:"ref"`
	TenantID     string               `json:"tenant_id"`
	Owner        contract.OwnerDomain `json:"owner"`
	AttemptRef   contract.Ref         `json:"attempt_ref"`
	OperationRef contract.Ref         `json:"operation_ref"`
	Outcome      InspectionOutcome    `json:"outcome"`
	SubjectRef   contract.Ref         `json:"subject_ref,omitempty"`
	ResultRef    contract.Ref         `json:"result_ref,omitempty"`
	InspectedAt  time.Time            `json:"inspected_at"`
}

func validID(parts ...string) bool {
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return false
		}
	}
	return true
}

func validateRefs(refs []contract.Ref) error {
	for _, ref := range refs {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func normalizeStrings(values []string) []string {
	out := slices.Clone(values)
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if len(out) > 256 {
		return out[:256]
	}
	return out
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

func validateTTL(ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("%w: ttl must be positive", contract.ErrInvalidArgument)
	}
	return nil
}

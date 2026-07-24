package contract

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const VersionV1 = "praxis.memory-knowledge/v1"

type OwnerDomain string

const (
	OwnerMemory    OwnerDomain = "praxis.memory"
	OwnerKnowledge OwnerDomain = "praxis.knowledge"
)

type Ref struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Digest   string `json:"digest"`
}

// ExpectedRevision keeps expect_absent distinct from revision zero. Callers
// must choose one mode explicitly for every authoritative CAS.
type ExpectedRevision struct {
	Absent   bool   `json:"absent"`
	Revision uint64 `json:"revision"`
}

func ExpectAbsent() ExpectedRevision { return ExpectedRevision{Absent: true} }

func ExpectRevision(revision uint64) ExpectedRevision {
	return ExpectedRevision{Revision: revision}
}

func (e ExpectedRevision) Validate() error {
	if e.Absent == (e.Revision != 0) {
		return fmt.Errorf("%w: expected revision must select absent or a positive revision", ErrInvalidArgument)
	}
	return nil
}

func (e ExpectedRevision) Matches(exists bool, revision uint64) bool {
	if e.Absent {
		return !exists
	}
	return exists && revision == e.Revision
}

func (r Ref) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision == 0 || strings.TrimSpace(r.Digest) == "" {
		return fmt.Errorf("%w: incomplete ref", ErrInvalidArgument)
	}
	return nil
}

func SameRef(a, b Ref) bool {
	return a.ID == b.ID && a.Revision == b.Revision && a.Digest == b.Digest
}

type ContentRef struct {
	ID        string `json:"id"`
	Digest    string `json:"digest"`
	Length    int64  `json:"length"`
	MediaType string `json:"media_type"`
}

func (r ContentRef) Validate() error {
	if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.Digest) == "" || r.Length < 0 || strings.TrimSpace(r.MediaType) == "" {
		return fmt.Errorf("%w: incomplete content ref", ErrInvalidArgument)
	}
	return nil
}

type Envelope struct {
	ContractVersion   string    `json:"contract_version"`
	SchemaRef         string    `json:"schema_ref"`
	ID                string    `json:"id"`
	Revision          uint64    `json:"revision"`
	Digest            string    `json:"digest"`
	TenantID          string    `json:"tenant_id"`
	IdentityID        string    `json:"identity_id"`
	IdentityEpoch     uint64    `json:"identity_epoch"`
	LineageID         string    `json:"lineage_id,omitempty"`
	PlanDigest        string    `json:"plan_digest,omitempty"`
	InstanceID        string    `json:"instance_id,omitempty"`
	InstanceEpoch     uint64    `json:"instance_epoch,omitempty"`
	SandboxLeaseID    string    `json:"sandbox_lease_id,omitempty"`
	SandboxLeaseEpoch uint64    `json:"sandbox_lease_epoch,omitempty"`
	RunID             string    `json:"run_id,omitempty"`
	AuthorityRef      Ref       `json:"authority_ref"`
	AuthorityEpoch    uint64    `json:"authority_epoch"`
	PolicyRef         Ref       `json:"policy_ref"`
	Purpose           string    `json:"purpose"`
	ActionScopeDigest string    `json:"action_scope_digest"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	Causation         []Ref     `json:"causation"`
	CorrelationID     string    `json:"correlation_id"`
}

func (e Envelope) ValidateCurrent(now time.Time) error {
	if e.ContractVersion != VersionV1 || strings.TrimSpace(e.SchemaRef) == "" || strings.TrimSpace(e.ID) == "" || e.Revision == 0 || strings.TrimSpace(e.TenantID) == "" || strings.TrimSpace(e.IdentityID) == "" || e.IdentityEpoch == 0 || strings.TrimSpace(e.Purpose) == "" || strings.TrimSpace(e.ActionScopeDigest) == "" {
		return fmt.Errorf("%w: incomplete envelope", ErrInvalidArgument)
	}
	if err := e.AuthorityRef.Validate(); err != nil {
		return fmt.Errorf("%w: authority: %v", ErrInvalidArgument, err)
	}
	if err := e.PolicyRef.Validate(); err != nil {
		return fmt.Errorf("%w: policy: %v", ErrInvalidArgument, err)
	}
	if e.CreatedAt.IsZero() || e.UpdatedAt.IsZero() || e.ExpiresAt.IsZero() || !e.ExpiresAt.After(now) {
		return ErrNotCurrent
	}
	if len(e.Causation) > 64 {
		return fmt.Errorf("%w: causation exceeds 64 refs", ErrInvalidArgument)
	}
	for _, ref := range e.Causation {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func NormalizeRefs(refs []Ref) []Ref {
	if len(refs) == 0 {
		return []Ref{}
	}
	out := slices.Clone(refs)
	slices.SortFunc(out, func(a, b Ref) int {
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
	return slices.CompactFunc(out, SameRef)
}

type Clock interface {
	Now() time.Time
}

type ClockFunc func() time.Time

func (f ClockFunc) Now() time.Time { return f() }

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

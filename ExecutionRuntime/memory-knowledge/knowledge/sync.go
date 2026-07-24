package knowledge

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

const (
	SyncContractVersionV1   = "praxis.knowledge/sync/v1"
	SyncAttemptObjectKindV1 = "knowledge_sync_attempt"
)

type SyncState string

const (
	SyncReserved       SyncState = "reserved"
	SyncAcquired       SyncState = "acquired"
	SyncParsed         SyncState = "parsed"
	SyncNormalized     SyncState = "normalized"
	SyncValidated      SyncState = "validated"
	SyncIndexed        SyncState = "indexed"
	SyncSnapshotReady  SyncState = "snapshot_ready"
	SyncPublished      SyncState = "published"
	SyncUnknownOutcome SyncState = "unknown_outcome"
	SyncResidual       SyncState = "residual"
)

type SyncAttemptV1 struct {
	ContractVersion        string               `json:"contract_version"`
	ObjectKind             string               `json:"object_kind"`
	Ref                    contract.Ref         `json:"ref"`
	Owner                  contract.OwnerDomain `json:"owner"`
	TenantID               string               `json:"tenant_id"`
	JobAttemptRef          contract.Ref         `json:"job_attempt_ref"`
	SourceSubjectRef       contract.Ref         `json:"source_subject_ref"`
	AuthorityRef           contract.Ref         `json:"authority_ref"`
	PolicyRef              contract.Ref         `json:"policy_ref"`
	ScopeRef               contract.Ref         `json:"scope_ref"`
	InputDigest            string               `json:"input_digest"`
	State                  SyncState            `json:"state"`
	AcquireObservationRef  contract.Ref         `json:"acquire_observation_ref,omitempty"`
	ParsedPackageRef       contract.Ref         `json:"parsed_package_ref,omitempty"`
	NormalizedPackageRef   contract.Ref         `json:"normalized_package_ref,omitempty"`
	ValidationEvidenceRefs []contract.Ref       `json:"validation_evidence_refs"`
	RecordRefs             []contract.Ref       `json:"record_refs"`
	ProjectionRefs         []contract.Ref       `json:"projection_refs"`
	SnapshotRef            contract.Ref         `json:"snapshot_ref,omitempty"`
	PointerRef             contract.Ref         `json:"pointer_ref,omitempty"`
	Residuals              []string             `json:"residuals"`
	CreatedAt              time.Time            `json:"created_at"`
	UpdatedAt              time.Time            `json:"updated_at"`
	ExpiresAt              time.Time            `json:"expires_at"`
	Digest                 string               `json:"digest"`
}

func SealSyncAttemptV1(in SyncAttemptV1) (SyncAttemptV1, error) {
	in.ContractVersion, in.ObjectKind = SyncContractVersionV1, SyncAttemptObjectKindV1
	in.Owner = contract.OwnerKnowledge
	in.ValidationEvidenceRefs = contract.NormalizeRefs(in.ValidationEvidenceRefs)
	in.RecordRefs = contract.NormalizeRefs(in.RecordRefs)
	in.ProjectionRefs = contract.NormalizeRefs(in.ProjectionRefs)
	in.Residuals = sortedUnique(in.Residuals)
	in.CreatedAt, in.UpdatedAt, in.ExpiresAt = in.CreatedAt.UTC(), in.UpdatedAt.UTC(), in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	d, e := contract.Digest(in)
	if e != nil {
		return SyncAttemptV1{}, e
	}
	in.Ref.Digest, in.Digest = d, d
	if e = in.Validate(); e != nil {
		return SyncAttemptV1{}, e
	}
	return in, nil
}
func (in SyncAttemptV1) Validate() error {
	if in.ContractVersion != SyncContractVersionV1 || in.ObjectKind != SyncAttemptObjectKindV1 || in.Owner != contract.OwnerKnowledge || in.Ref.Validate() != nil || strings.TrimSpace(in.TenantID) == "" || in.JobAttemptRef.Validate() != nil || in.SourceSubjectRef.Validate() != nil || in.AuthorityRef.Validate() != nil || in.PolicyRef.Validate() != nil || in.ScopeRef.Validate() != nil || strings.TrimSpace(in.InputDigest) == "" || !validSyncState(in.State) || in.CreatedAt.IsZero() || in.UpdatedAt.Before(in.CreatedAt) || !in.ExpiresAt.After(in.UpdatedAt) {
		return fmt.Errorf("%w: sync attempt", contract.ErrInvalidArgument)
	}
	if stateAtLeast(in.State, SyncAcquired) && in.AcquireObservationRef.Validate() != nil {
		return fmt.Errorf("%w: acquire observation", contract.ErrInvalidArgument)
	}
	if stateAtLeast(in.State, SyncParsed) && in.ParsedPackageRef.Validate() != nil {
		return fmt.Errorf("%w: parsed package", contract.ErrInvalidArgument)
	}
	if stateAtLeast(in.State, SyncNormalized) && in.NormalizedPackageRef.Validate() != nil {
		return fmt.Errorf("%w: normalized package", contract.ErrInvalidArgument)
	}
	if stateAtLeast(in.State, SyncValidated) && len(in.ValidationEvidenceRefs) == 0 {
		return fmt.Errorf("%w: validation evidence", contract.ErrInvalidArgument)
	}
	if stateAtLeast(in.State, SyncIndexed) && (len(in.RecordRefs) == 0 || len(in.ProjectionRefs) == 0) {
		return fmt.Errorf("%w: index refs", contract.ErrInvalidArgument)
	}
	if stateAtLeast(in.State, SyncSnapshotReady) && in.SnapshotRef.Validate() != nil {
		return fmt.Errorf("%w: snapshot ref", contract.ErrInvalidArgument)
	}
	if in.State == SyncPublished && in.PointerRef.Validate() != nil {
		return fmt.Errorf("%w: pointer ref", contract.ErrInvalidArgument)
	}
	if in.State == SyncResidual && len(in.Residuals) == 0 {
		return fmt.Errorf("%w: sync residual", contract.ErrInvalidArgument)
	}
	c := in
	c.Ref.Digest, c.Digest = "", ""
	d, e := contract.Digest(c)
	if e != nil {
		return e
	}
	if d != in.Digest || in.Ref.Digest != d {
		return fmt.Errorf("%w: sync digest", contract.ErrEvidenceConflict)
	}
	return nil
}

func ValidateSyncTransitionV1(current, next SyncAttemptV1) error {
	if e := current.Validate(); e != nil {
		return e
	}
	if current.State == SyncUnknownOutcome && next.State != SyncAcquired && next.State != SyncResidual {
		return contract.ErrUnknownOutcome
	}
	if e := next.Validate(); e != nil {
		return e
	}
	if current.Ref.ID != next.Ref.ID || next.Ref.Revision != current.Ref.Revision+1 || current.TenantID != next.TenantID || !contract.SameRef(current.JobAttemptRef, next.JobAttemptRef) || !contract.SameRef(current.SourceSubjectRef, next.SourceSubjectRef) || !contract.SameRef(current.AuthorityRef, next.AuthorityRef) || !contract.SameRef(current.PolicyRef, next.PolicyRef) || !contract.SameRef(current.ScopeRef, next.ScopeRef) || current.InputDigest != next.InputDigest || !current.CreatedAt.Equal(next.CreatedAt) || !current.ExpiresAt.Equal(next.ExpiresAt) || next.UpdatedAt.Before(current.UpdatedAt) {
		return fmt.Errorf("%w: sync identity drift", contract.ErrEvidenceConflict)
	}
	if !allowedSyncTransition(current.State, next.State) {
		return fmt.Errorf("%w: sync transition", contract.ErrRevisionConflict)
	}
	return nil
}

func (s *Store) ReserveSync(access Access, in SyncAttemptV1) (SyncAttemptV1, error) {
	if e := access.Validate(); e != nil {
		return SyncAttemptV1{}, e
	}
	if in.TenantID != access.TenantID || !contract.SameRef(in.AuthorityRef, access.AuthorityRef) || !contract.SameRef(in.PolicyRef, access.PolicyRef) || in.State != SyncReserved || in.Ref.Revision != 1 {
		return SyncAttemptV1{}, contract.ErrScopeDenied
	}
	sealed, e := SealSyncAttemptV1(in)
	if e != nil {
		return SyncAttemptV1{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, e := s.tenantLocked(access.TenantID, true)
	if e != nil {
		return SyncAttemptV1{}, e
	}
	if h := t.syncs[sealed.Ref.ID]; len(h) > 0 {
		cur := h[len(h)-1]
		if contract.SameRef(cur.Ref, sealed.Ref) {
			return cloneSync(cur), nil
		}
		return SyncAttemptV1{}, contract.ErrEvidenceConflict
	}
	t.syncs[sealed.Ref.ID] = []SyncAttemptV1{cloneSync(sealed)}
	return cloneSync(sealed), nil
}
func (s *Store) AdvanceSync(access Access, expected contract.Ref, next SyncAttemptV1) (SyncAttemptV1, error) {
	if e := access.Validate(); e != nil {
		return SyncAttemptV1{}, e
	}
	if e := expected.Validate(); e != nil {
		return SyncAttemptV1{}, e
	}
	if next.TenantID != access.TenantID || !contract.SameRef(next.AuthorityRef, access.AuthorityRef) || !contract.SameRef(next.PolicyRef, access.PolicyRef) {
		return SyncAttemptV1{}, contract.ErrScopeDenied
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, e := s.tenantLocked(access.TenantID, false)
	if e != nil || len(t.syncs[expected.ID]) == 0 {
		return SyncAttemptV1{}, contract.ErrNotFound
	}
	h := t.syncs[expected.ID]
	cur := h[len(h)-1]
	if cur.State == SyncUnknownOutcome && next.State != SyncAcquired && next.State != SyncResidual {
		return SyncAttemptV1{}, contract.ErrUnknownOutcome
	}
	sealed, e := SealSyncAttemptV1(next)
	if e != nil {
		return SyncAttemptV1{}, e
	}
	if contract.SameRef(cur.Ref, sealed.Ref) {
		return cloneSync(cur), nil
	}
	if !contract.SameRef(cur.Ref, expected) {
		return SyncAttemptV1{}, contract.ErrRevisionConflict
	}
	if e = ValidateSyncTransitionV1(cur, sealed); e != nil {
		return SyncAttemptV1{}, e
	}
	t.syncs[expected.ID] = append(h, cloneSync(sealed))
	return cloneSync(sealed), nil
}
func (s *Store) InspectSync(access Access, exact contract.Ref) (SyncAttemptV1, error) {
	if e := access.Validate(); e != nil {
		return SyncAttemptV1{}, e
	}
	if e := exact.Validate(); e != nil {
		return SyncAttemptV1{}, e
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, e := s.tenantLocked(access.TenantID, false)
	if e != nil || len(t.syncs[exact.ID]) == 0 {
		return SyncAttemptV1{}, contract.ErrNotFound
	}
	for _, v := range t.syncs[exact.ID] {
		if contract.SameRef(v.Ref, exact) {
			if !contract.SameRef(v.AuthorityRef, access.AuthorityRef) || !contract.SameRef(v.PolicyRef, access.PolicyRef) {
				return SyncAttemptV1{}, contract.ErrScopeDenied
			}
			if e = v.Validate(); e != nil {
				return SyncAttemptV1{}, e
			}
			return cloneSync(v), nil
		}
	}
	return SyncAttemptV1{}, contract.ErrEvidenceConflict
}
func cloneSync(in SyncAttemptV1) SyncAttemptV1 {
	in.ValidationEvidenceRefs = append([]contract.Ref{}, in.ValidationEvidenceRefs...)
	in.RecordRefs = append([]contract.Ref{}, in.RecordRefs...)
	in.ProjectionRefs = append([]contract.Ref{}, in.ProjectionRefs...)
	in.Residuals = append([]string{}, in.Residuals...)
	return in
}
func validSyncState(s SyncState) bool {
	return s == SyncReserved || s == SyncAcquired || s == SyncParsed || s == SyncNormalized || s == SyncValidated || s == SyncIndexed || s == SyncSnapshotReady || s == SyncPublished || s == SyncUnknownOutcome || s == SyncResidual
}
func allowedSyncTransition(a, b SyncState) bool {
	switch a {
	case SyncReserved:
		return b == SyncAcquired || b == SyncUnknownOutcome || b == SyncResidual
	case SyncUnknownOutcome:
		return b == SyncAcquired || b == SyncResidual
	case SyncAcquired:
		return b == SyncParsed || b == SyncResidual
	case SyncParsed:
		return b == SyncNormalized || b == SyncResidual
	case SyncNormalized:
		return b == SyncValidated || b == SyncResidual
	case SyncValidated:
		return b == SyncIndexed || b == SyncResidual
	case SyncIndexed:
		return b == SyncSnapshotReady || b == SyncResidual
	case SyncSnapshotReady:
		return b == SyncPublished || b == SyncResidual
	default:
		return false
	}
}
func stateAtLeast(current, target SyncState) bool {
	order := map[SyncState]int{SyncReserved: 0, SyncUnknownOutcome: 0, SyncAcquired: 1, SyncParsed: 2, SyncNormalized: 3, SyncValidated: 4, SyncIndexed: 5, SyncSnapshotReady: 6, SyncPublished: 7}
	return order[current] >= order[target] && current != SyncResidual
}
func sortedUnique(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	out = slices.Compact(out)
	if out == nil {
		return []string{}
	}
	return out
}

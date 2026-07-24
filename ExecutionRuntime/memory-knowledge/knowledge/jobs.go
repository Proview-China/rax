package knowledge

import (
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func (s *Store) ReserveJob(access Access, in contract.OwnerJobAttemptV1) (contract.OwnerJobAttemptV1, error) {
	if err := access.Validate(); err != nil {
		return contract.OwnerJobAttemptV1{}, err
	}
	if in.Owner != contract.OwnerKnowledge || in.TenantID != access.TenantID || !contract.SameRef(in.AuthorityRef, access.AuthorityRef) || !contract.SameRef(in.PolicyRef, access.PolicyRef) || in.State != contract.JobReserved || in.Ref.Revision != 1 {
		return contract.OwnerJobAttemptV1{}, contract.ErrScopeDenied
	}
	sealed, err := contract.SealOwnerJobAttemptV1(in)
	if err != nil {
		return contract.OwnerJobAttemptV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(access.TenantID, true)
	if err != nil {
		return contract.OwnerJobAttemptV1{}, err
	}
	if history := t.jobs[sealed.Ref.ID]; len(history) != 0 {
		current := history[len(history)-1]
		if contract.SameRef(current.Ref, sealed.Ref) {
			return cloneOwnerJob(current), nil
		}
		return contract.OwnerJobAttemptV1{}, contract.ErrEvidenceConflict
	}
	t.jobs[sealed.Ref.ID] = []contract.OwnerJobAttemptV1{cloneOwnerJob(sealed)}
	return cloneOwnerJob(sealed), nil
}

func (s *Store) AdvanceJob(access Access, expected contract.Ref, next contract.OwnerJobAttemptV1) (contract.OwnerJobAttemptV1, error) {
	if err := access.Validate(); err != nil {
		return contract.OwnerJobAttemptV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.OwnerJobAttemptV1{}, err
	}
	if next.Owner != contract.OwnerKnowledge || next.TenantID != access.TenantID || !contract.SameRef(next.AuthorityRef, access.AuthorityRef) || !contract.SameRef(next.PolicyRef, access.PolicyRef) {
		return contract.OwnerJobAttemptV1{}, contract.ErrScopeDenied
	}
	sealed, err := contract.SealOwnerJobAttemptV1(next)
	if err != nil {
		return contract.OwnerJobAttemptV1{}, fmt.Errorf("seal knowledge job transition: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil || len(t.jobs[expected.ID]) == 0 {
		return contract.OwnerJobAttemptV1{}, contract.ErrNotFound
	}
	history := t.jobs[expected.ID]
	current := history[len(history)-1]
	if contract.SameRef(current.Ref, sealed.Ref) {
		return cloneOwnerJob(current), nil
	}
	if !contract.SameRef(current.Ref, expected) {
		return contract.OwnerJobAttemptV1{}, contract.ErrRevisionConflict
	}
	if err := contract.ValidateOwnerJobTransitionV1(current, sealed); err != nil {
		return contract.OwnerJobAttemptV1{}, fmt.Errorf("validate knowledge job transition: %w", err)
	}
	t.jobs[expected.ID] = append(history, cloneOwnerJob(sealed))
	return cloneOwnerJob(sealed), nil
}

func (s *Store) InspectJob(access Access, exact contract.Ref) (contract.OwnerJobAttemptV1, error) {
	if err := access.Validate(); err != nil {
		return contract.OwnerJobAttemptV1{}, err
	}
	if err := exact.Validate(); err != nil {
		return contract.OwnerJobAttemptV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil || len(t.jobs[exact.ID]) == 0 {
		return contract.OwnerJobAttemptV1{}, contract.ErrNotFound
	}
	for _, job := range t.jobs[exact.ID] {
		if contract.SameRef(job.Ref, exact) {
			if job.Owner != contract.OwnerKnowledge || job.TenantID != access.TenantID || !contract.SameRef(job.AuthorityRef, access.AuthorityRef) || !contract.SameRef(job.PolicyRef, access.PolicyRef) {
				return contract.OwnerJobAttemptV1{}, contract.ErrScopeDenied
			}
			if err := job.Validate(); err != nil {
				return contract.OwnerJobAttemptV1{}, fmt.Errorf("validate stored knowledge job: %w", err)
			}
			return cloneOwnerJob(job), nil
		}
	}
	return contract.OwnerJobAttemptV1{}, contract.ErrEvidenceConflict
}

func cloneOwnerJob(in contract.OwnerJobAttemptV1) contract.OwnerJobAttemptV1 {
	in.Residuals = append([]string{}, in.Residuals...)
	return in
}

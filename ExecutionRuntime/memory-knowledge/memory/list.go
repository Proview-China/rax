package memory

import (
	"slices"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func (s *Store) ListProjections(access Access) ([]Projection, error) {
	if err := access.validate(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t := s.tenants[access.TenantID]
	if t == nil {
		return nil, contract.ErrNotFound
	}
	out := make([]Projection, 0, len(t.projections))
	for _, history := range t.projections {
		if len(history) == 0 {
			continue
		}
		p := history[len(history)-1]
		record := findRecordByRef(t, p.RecordRef)
		if record.IdentityID == access.IdentityID && record.AuthorityEpoch == access.AuthorityEpoch && contract.SameRef(record.AuthorityRef, access.AuthorityRef) && contract.SameRef(record.PolicyRef, access.PolicyRef) {
			out = append(out, cloneProjection(p))
		}
	}
	slices.SortFunc(out, func(a, b Projection) int { return strings.Compare(a.Ref.ID, b.Ref.ID) })
	return out, nil
}

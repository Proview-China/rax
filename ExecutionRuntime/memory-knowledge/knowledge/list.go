package knowledge

import (
	"slices"
	"strings"
)

func (s *Store) ListSources(access Access) ([]Source, error) {
	if err := access.Validate(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return nil, err
	}
	out := make([]Source, 0, len(t.sources))
	for _, history := range t.sources {
		if len(history) == 0 {
			continue
		}
		source := history[len(history)-1]
		if accessAllows(access, source.AuthorityRef, source.PolicyRef) {
			out = append(out, cloneSource(source))
		}
	}
	slices.SortFunc(out, func(a, b Source) int { return strings.Compare(a.Ref.ID, b.Ref.ID) })
	return out, nil
}
func (s *Store) ListProjections(access Access) ([]Projection, error) {
	if err := access.Validate(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return nil, err
	}
	out := make([]Projection, 0, len(t.projections))
	for _, history := range t.projections {
		if len(history) == 0 {
			continue
		}
		projection := history[len(history)-1]
		allowed := false
		for _, ref := range projection.RecordRefs {
			if record, ok := recordByRef(t, ref); ok && accessAllows(access, record.AuthorityRef, record.PolicyRef) {
				allowed = true
			} else {
				allowed = false
				break
			}
		}
		if allowed {
			out = append(out, cloneProjection(projection))
		}
	}
	slices.SortFunc(out, func(a, b Projection) int { return strings.Compare(a.Ref.ID, b.Ref.ID) })
	return out, nil
}

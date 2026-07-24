package memory

import (
	"fmt"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	sharedretrieval "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/retrieval"
)

func (s *Store) PublishView(access Access, view View, expected contract.ExpectedRevision) (View, error) {
	if err := access.validate(); err != nil {
		return View{}, err
	}
	if err := expected.Validate(); err != nil {
		return View{}, err
	}
	var err error
	view, err = sealView(view)
	if err != nil {
		return View{}, err
	}
	if view.TenantID != access.TenantID || view.PrincipalID != access.IdentityID || view.Owner != contract.OwnerMemory || view.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(view.AuthorityRef, access.AuthorityRef) || !contract.SameRef(view.PolicyRef, access.PolicyRef) || strings.TrimSpace(view.Ref.ID) == "" || view.Ref.Revision == 0 || strings.TrimSpace(view.Purpose) == "" || len(view.Scopes) == 0 || strings.TrimSpace(view.SensitivityMax) == "" || !view.ExpiresAt.After(s.clock.Now()) {
		return View{}, contract.ErrScopeDenied
	}
	if err := view.WatermarkRef.Validate(); err != nil {
		return View{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tenant(access.TenantID)
	if view.WatermarkRef.ID != "memory/watermark/"+access.TenantID || view.WatermarkRef.Revision > t.watermark || !contract.SameRef(view.WatermarkRef, makeWatermark(access.TenantID, view.WatermarkRef.Revision).Ref) {
		return View{}, contract.ErrNotCurrent
	}
	for _, wanted := range view.ProjectionRefs {
		found := false
		for _, versions := range t.projections {
			for _, p := range versions {
				if contract.SameRef(p.Ref, wanted) && p.TenantID == access.TenantID {
					found = true
					break
				}
			}
		}
		if !found {
			return View{}, contract.ErrNotFound
		}
	}
	versions := t.views[view.Ref.ID]
	exists := len(versions) > 0
	current := uint64(0)
	if exists {
		current = versions[len(versions)-1].Ref.Revision
	}
	if !expected.Matches(exists, current) || view.Ref.Revision != current+1 {
		return View{}, contract.ErrRevisionConflict
	}
	t.views[view.Ref.ID] = append(versions, cloneView(view))
	return cloneView(view), nil
}

func (s *Store) PutProjection(access Access, projection Projection, expected contract.ExpectedRevision) (Projection, error) {
	if err := access.validate(); err != nil {
		return Projection{}, err
	}
	if err := expected.Validate(); err != nil {
		return Projection{}, err
	}
	var err error
	projection, err = sealProjection(projection)
	if err != nil {
		return Projection{}, err
	}
	if projection.TenantID != access.TenantID || strings.TrimSpace(projection.Ref.ID) == "" || projection.Ref.Revision == 0 || strings.TrimSpace(projection.Kind) == "" || strings.TrimSpace(projection.BuilderVersion) == "" || !projection.ExpiresAt.After(s.clock.Now()) {
		return Projection{}, contract.ErrScopeDenied
	}
	if projection.State != ProjectionReady && projection.State != ProjectionPartial && projection.State != ProjectionStale {
		return Projection{}, fmt.Errorf("%w: projection state", contract.ErrInvalidArgument)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tenant(access.TenantID)
	record := findRecordByRef(t, projection.RecordRef)
	if record.Ref.ID == "" || record.IdentityID != access.IdentityID || record.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(record.AuthorityRef, access.AuthorityRef) || !contract.SameRef(record.PolicyRef, access.PolicyRef) {
		return Projection{}, contract.ErrNotFound
	}
	versions := t.projections[projection.Ref.ID]
	exists := len(versions) > 0
	current := uint64(0)
	if exists {
		current = versions[len(versions)-1].Ref.Revision
		priorRecord := findRecordByRef(t, versions[len(versions)-1].RecordRef)
		if priorRecord.IdentityID != access.IdentityID {
			return Projection{}, contract.ErrNotFound
		}
	}
	if !expected.Matches(exists, current) || projection.Ref.Revision != current+1 {
		return Projection{}, contract.ErrRevisionConflict
	}
	t.projections[projection.Ref.ID] = append(versions, cloneProjection(projection))
	return cloneProjection(projection), nil
}

func (s *Store) Query(access Access, query contract.RetrievalQuery) (contract.RetrievalResult, error) {
	if err := access.validate(); err != nil {
		return contract.RetrievalResult{}, err
	}
	if query.Domain != contract.OwnerMemory {
		return contract.RetrievalResult{}, contract.ErrScopeDenied
	}
	s.mu.RLock()
	t := s.tenants[access.TenantID]
	if t == nil {
		s.mu.RUnlock()
		return contract.RetrievalResult{}, contract.ErrNotFound
	}
	view, ok := currentViewByRef(t, query.ViewRef)
	if !ok || view.PrincipalID != access.IdentityID || view.TenantID != access.TenantID {
		s.mu.RUnlock()
		return contract.RetrievalResult{}, contract.ErrNotFound
	}
	now := s.clock.Now()
	if !view.ExpiresAt.After(now) || view.Purpose != query.Purpose || view.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(view.AuthorityRef, access.AuthorityRef) || !contract.SameRef(view.PolicyRef, access.PolicyRef) || !scopesSubset(query.Scopes, view.Scopes) || !sensitivityAllowed(view.SensitivityMax, query.SensitivityMax) {
		s.mu.RUnlock()
		return contract.RetrievalResult{}, contract.ErrNotCurrent
	}
	var records []Record
	for _, versions := range t.records {
		for i := len(versions) - 1; i >= 0; i-- {
			r := versions[i]
			if _, merged := t.mergedInto[r.Ref.ID]; merged {
				break
			}
			if r.Watermark <= view.WatermarkRef.Revision {
				if r.Current(now) && r.IdentityID == access.IdentityID && r.AuthorityEpoch == access.AuthorityEpoch && contract.SameRef(r.AuthorityRef, access.AuthorityRef) && contract.SameRef(r.PolicyRef, access.PolicyRef) && r.Purpose == query.Purpose && contains(query.Scopes, r.Scope) && sensitivityAllowed(query.SensitivityMax, r.Sensitivity) {
					records = append(records, cloneRecord(r))
				}
				break
			}
		}
	}
	allowedProjection := make(map[string]struct{}, len(view.ProjectionRefs))
	for _, ref := range view.ProjectionRefs {
		allowedProjection[candidateKey(ref)+":"+ref.Digest] = struct{}{}
	}
	projections := make(map[string][]Projection)
	for _, versions := range t.projections {
		for _, p := range versions {
			_, bound := allowedProjection[candidateKey(p.Ref)+":"+p.Ref.Digest]
			if bound && p.ExpiresAt.After(now) && p.State != ProjectionStale {
				key := candidateKey(p.RecordRef)
				projections[key] = append(projections[key], cloneProjection(p))
			}
		}
	}
	s.mu.RUnlock()

	docs := make([]sharedretrieval.Document, 0, len(records))
	coverage := contract.Coverage{Status: contract.CoverageComplete, Expected: len(records)}
	for _, record := range records {
		if record.ContentRef == nil {
			continue
		}
		if s.content == nil {
			coverage.DroppedReasons = append(coverage.DroppedReasons, "content_reader_unavailable:"+record.Ref.ID)
			continue
		}
		content, err := s.content.Get(*record.ContentRef)
		if err != nil {
			coverage.DroppedReasons = append(coverage.DroppedReasons, "content_unavailable:"+record.Ref.ID)
			continue
		}
		var projectionRefs []contract.Ref
		for _, p := range projections[candidateKey(record.Ref)] {
			projectionRefs = append(projectionRefs, p.Ref)
			coverage.ProjectionRefs = append(coverage.ProjectionRefs, p.Ref)
			if p.State == ProjectionPartial {
				coverage.DroppedReasons = append(coverage.DroppedReasons, "projection_partial:"+p.Ref.ID)
			}
		}
		coverage.Available++
		docs = append(docs, sharedretrieval.Document{Domain: contract.OwnerMemory, RecordRef: record.Ref, ContentRef: *record.ContentRef, Text: string(content), Scope: record.Scope, Subject: record.Subject, Sensitivity: record.Sensitivity, Current: true, SourceRefs: record.SourceRefs, EvidenceRefs: record.EvidenceRefs, ProjectionRefs: projectionRefs, RelevanceBPS: recordRelevanceBPS(record, now)})
	}
	if coverage.Available < coverage.Expected || len(coverage.DroppedReasons) > 0 {
		coverage.Status = contract.CoveragePartial
	}
	if coverage.Expected > 0 && coverage.Available == 0 {
		coverage.Status = contract.CoverageUnavailable
	}
	return sharedretrieval.Search(now, query, view.WatermarkRef, docs, coverage)
}

func recordRelevanceBPS(record Record, now time.Time) int {
	if record.Pinned || record.DecayHalfLifeSeconds == 0 || !now.After(record.CreatedAt) {
		return 10_000
	}
	periods := uint64(now.Sub(record.CreatedAt) / (time.Duration(record.DecayHalfLifeSeconds) * time.Second))
	if periods >= 14 {
		return 1
	}
	return max(1, 10_000>>periods)
}

func currentViewByRef(t *tenantState, ref contract.Ref) (View, bool) {
	versions := t.views[ref.ID]
	for _, v := range versions {
		if contract.SameRef(v.Ref, ref) {
			return v, true
		}
	}
	return View{}, false
}

func scopesSubset(requested, allowed []string) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		set[value] = struct{}{}
	}
	if len(requested) == 0 {
		return false
	}
	for _, value := range requested {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func sensitivityAllowed(maximum, requested string) bool {
	rank := map[string]int{"public": 0, "internal": 1, "confidential": 2, "restricted": 3}
	maxRank, maxOK := rank[maximum]
	requestedRank, requestedOK := rank[requested]
	return maxOK && requestedOK && requestedRank <= maxRank
}

func cloneView(v View) View {
	v.Scopes = append([]string(nil), v.Scopes...)
	v.ProjectionRefs = append([]contract.Ref(nil), v.ProjectionRefs...)
	return v
}

func cloneProjection(p Projection) Projection {
	p.Coverage.ProjectionRefs = append([]contract.Ref(nil), p.Coverage.ProjectionRefs...)
	p.Coverage.DroppedReasons = append([]string(nil), p.Coverage.DroppedReasons...)
	return p
}

package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/retrieval"
)

// ContentReader is the local, read-only materialization seam used by the
// deterministic Wave 1 retriever. Implementations must not perform an
// ungoverned remote effect.
type ContentReader interface {
	Get(contract.ContentRef) ([]byte, error)
}

func (s *Store) Query(access Access, query contract.RetrievalQuery, content ContentReader) (contract.RetrievalResult, error) {
	if err := access.Validate(); err != nil {
		return contract.RetrievalResult{}, err
	}
	if content == nil {
		return contract.RetrievalResult{}, contract.ErrContextUnmaterialized
	}
	if query.Domain != contract.OwnerKnowledge {
		return contract.RetrievalResult{}, contract.ErrScopeDenied
	}
	now := s.clock.Now().UTC()
	if !now.Before(query.ExpiresAt) {
		return contract.RetrievalResult{}, contract.ErrNotCurrent
	}
	s.mu.RLock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		s.mu.RUnlock()
		return contract.RetrievalResult{}, err
	}
	view, ok := viewByRef(t, query.ViewRef)
	if !ok {
		s.mu.RUnlock()
		return contract.RetrievalResult{}, contract.ErrNotFound
	}
	if view.Owner != contract.OwnerKnowledge || !accessAllows(access, view.AuthorityRef, view.PolicyRef) || query.Purpose != view.Purpose || !now.Before(view.ExpiresAt) {
		s.mu.RUnlock()
		return contract.RetrievalResult{}, contract.ErrNotCurrent
	}
	if !subsetStrings(query.Scopes, view.Scopes) || !sensitivityAtMost(query.SensitivityMax, view.SensitivityMax) {
		s.mu.RUnlock()
		return contract.RetrievalResult{}, contract.ErrScopeDenied
	}
	snapshot, ok := snapshotByRef(t, view.SnapshotRef)
	if !ok || snapshot.State != SnapshotPublished || !containsRef(snapshot.AuthorityRefs, access.AuthorityRef) || !containsRef(snapshot.PolicyRefs, access.PolicyRef) {
		s.mu.RUnlock()
		return contract.RetrievalResult{}, contract.ErrNotCurrent
	}
	if view.CurrentOnly && (t.current == nil || !contract.SameRef(t.current.TargetRef, snapshot.Ref)) {
		s.mu.RUnlock()
		return contract.RetrievalResult{}, contract.ErrNotCurrent
	}
	projectionsCurrent := true
	dropped := slices.Clone(snapshot.Coverage.DroppedReasons)
	for _, ref := range view.ProjectionRefs {
		if !containsRef(snapshot.ProjectionRefs, ref) {
			s.mu.RUnlock()
			return contract.RetrievalResult{}, contract.ErrNotCurrent
		}
		projection, found := projectionByRef(t, ref)
		if !found || !now.Before(projection.ExpiresAt) || projection.State == ProjectionStale {
			projectionsCurrent = false
			dropped = append(dropped, "projection_stale:"+ref.ID)
		} else if projection.State == ProjectionPartial || projection.Coverage.Status == contract.CoveragePartial {
			projectionsCurrent = false
			dropped = append(dropped, "projection_partial:"+ref.ID)
		}
	}
	records := make([]Record, 0, len(snapshot.RecordRefs))
	for _, ref := range snapshot.RecordRefs {
		record, found := recordByRef(t, ref)
		if !found {
			dropped = append(dropped, "record_missing:"+ref.ID)
			continue
		}
		if !accessAllows(access, record.AuthorityRef, record.PolicyRef) || record.Status != RecordActive || now.Before(record.ValidFrom) || !now.Before(record.ValidTo) {
			dropped = append(dropped, "record_not_current:"+ref.ID)
			continue
		}
		if view.CurrentOnly {
			history := t.records[record.Ref.ID]
			if len(history) == 0 || !contract.SameRef(history[len(history)-1].Ref, record.Ref) {
				dropped = append(dropped, "record_watermark_stale:"+ref.ID)
				continue
			}
		}
		sourcesCurrent := true
		for _, sourceRef := range record.SourceRefs {
			source, found := sourceByRef(t, sourceRef)
			if !found || !accessAllows(access, source.AuthorityRef, source.PolicyRef) || source.State == SourceWithdrawn || source.State == SourceDeprecated || now.Before(source.ValidFrom) || !now.Before(source.ValidTo) {
				sourcesCurrent = false
				break
			}
			if view.CurrentOnly {
				history := t.sources[source.Ref.ID]
				if len(history) == 0 || !contract.SameRef(history[len(history)-1].Ref, source.Ref) {
					sourcesCurrent = false
					break
				}
			}
		}
		if !sourcesCurrent {
			dropped = append(dropped, "source_not_current:"+ref.ID)
			continue
		}
		if !slices.Contains(view.AllowedLicenses, record.License) {
			dropped = append(dropped, "license_denied:"+ref.ID)
			continue
		}
		records = append(records, cloneRecord(record))
	}
	view = cloneView(view)
	snapshot = cloneSnapshot(snapshot)
	s.mu.RUnlock()

	docs := make([]retrieval.Document, 0, len(records))
	for _, record := range records {
		body, readErr := content.Get(record.ContentRef)
		if readErr != nil || !contentDigestMatches(record.ContentRef, body) {
			dropped = append(dropped, "content_unavailable:"+record.Ref.ID)
			continue
		}
		docs = append(docs, retrieval.Document{
			Domain: contract.OwnerKnowledge, RecordRef: record.Ref, ContentRef: record.ContentRef,
			Text: string(body), Scope: record.Scope, Subject: record.Subject, Sensitivity: record.Sensitivity,
			Current: true, SourceRefs: contract.NormalizeRefs(record.SourceRefs), EvidenceRefs: contract.NormalizeRefs(record.EvidenceRefs),
			ProjectionRefs: slices.Clone(view.ProjectionRefs), ConflictGroup: record.ConflictGroup,
			TrustState: string(record.TrustState), License: record.License, SnapshotRef: snapshot.Ref, PackageRef: record.PackageRef,
		})
	}
	coverage := contract.Coverage{
		Status: contract.CoverageComplete, Expected: len(snapshot.RecordRefs), Available: len(docs),
		ProjectionRefs: contract.NormalizeRefs(view.ProjectionRefs), DroppedReasons: normalizeStrings(dropped),
	}
	if len(docs) == 0 && coverage.Expected > 0 {
		coverage.Status = contract.CoverageNone
	} else if len(docs) < coverage.Expected || !projectionsCurrent || snapshot.Coverage.Status == contract.CoveragePartial {
		coverage.Status = contract.CoveragePartial
	}
	return retrieval.Search(now, query, snapshot.Ref, docs, coverage)
}

func subsetStrings(values, allowed []string) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		set[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return len(values) != 0
}

func sensitivityAtMost(value, maximum string) bool {
	rank := map[string]int{"public": 0, "internal": 1, "confidential": 2, "restricted": 3}
	v, vok := rank[value]
	m, mok := rank[maximum]
	return vok && mok && v <= m
}

func contentDigestMatches(ref contract.ContentRef, body []byte) bool {
	if int64(len(body)) != ref.Length {
		return false
	}
	sum := sha256.Sum256(body)
	return strings.EqualFold(ref.Digest, "sha256:"+hex.EncodeToString(sum[:]))
}

package knowledge

import (
	"slices"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func (s *Store) ExportView(access Access, viewRef contract.Ref, id string, ttl time.Duration) (contract.ExportManifestV1, error) {
	if err := access.Validate(); err != nil {
		return contract.ExportManifestV1{}, err
	}
	if viewRef.Validate() != nil || id == "" || ttl <= 0 {
		return contract.ExportManifestV1{}, contract.ErrInvalidArgument
	}
	now := s.clock.Now().UTC()
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, err := s.tenantLocked(access.TenantID, false)
	if err != nil {
		return contract.ExportManifestV1{}, err
	}
	view, ok := viewByRef(t, viewRef)
	if !ok || !accessAllows(access, view.AuthorityRef, view.PolicyRef) || !view.ExpiresAt.After(now) {
		return contract.ExportManifestV1{}, contract.ErrNotCurrent
	}
	snapshot, ok := snapshotByRef(t, view.SnapshotRef)
	if !ok || snapshot.State != SnapshotPublished {
		return contract.ExportManifestV1{}, contract.ErrNotCurrent
	}
	entries := make([]contract.ExportEntryV1, 0)
	for _, ref := range snapshot.RecordRefs {
		record, ok := recordByRef(t, ref)
		if !ok || !accessAllows(access, record.AuthorityRef, record.PolicyRef) || record.Status != RecordActive || now.Before(record.ValidFrom) || !record.ValidTo.After(now) || !slices.Contains(view.AllowedLicenses, record.License) {
			continue
		}
		if view.CurrentOnly {
			history := t.records[record.Ref.ID]
			if len(history) == 0 || !contract.SameRef(history[len(history)-1].Ref, record.Ref) {
				continue
			}
		}
		sourcesCurrent := true
		for _, sourceRef := range record.SourceRefs {
			source, found := sourceByRef(t, sourceRef)
			if !found || !accessAllows(access, source.AuthorityRef, source.PolicyRef) || source.State == SourceWithdrawn || source.State == SourceDeprecated || now.Before(source.ValidFrom) || !source.ValidTo.After(now) {
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
			continue
		}
		entries = append(entries, contract.ExportEntryV1{RecordRef: record.Ref, ContentRef: &record.ContentRef, SourceRefs: append([]contract.Ref{}, record.SourceRefs...), EvidenceRefs: append([]contract.Ref{}, record.EvidenceRefs...), Scope: record.Scope, Sensitivity: record.Sensitivity, License: record.License})
	}
	return contract.SealExportManifestV1(contract.ExportManifestV1{Ref: contract.Ref{ID: id, Revision: 1}, Owner: contract.OwnerKnowledge, TenantID: access.TenantID, ViewRef: viewRef, Entries: entries, CreatedAt: now, ExpiresAt: now.Add(ttl)})
}

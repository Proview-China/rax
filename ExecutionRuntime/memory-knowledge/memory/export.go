package memory

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func (s *Store) ExportView(access Access, viewRef contract.Ref, id string, ttl time.Duration) (contract.ExportManifestV1, error) {
	if err := access.validate(); err != nil {
		return contract.ExportManifestV1{}, err
	}
	if viewRef.Validate() != nil || id == "" || ttl <= 0 {
		return contract.ExportManifestV1{}, contract.ErrInvalidArgument
	}
	now := s.clock.Now().UTC()
	s.mu.RLock()
	t := s.tenants[access.TenantID]
	if t == nil {
		s.mu.RUnlock()
		return contract.ExportManifestV1{}, contract.ErrNotFound
	}
	view, ok := currentViewByRef(t, viewRef)
	if !ok || view.PrincipalID != access.IdentityID || !view.ExpiresAt.After(now) || view.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(view.AuthorityRef, access.AuthorityRef) || !contract.SameRef(view.PolicyRef, access.PolicyRef) {
		s.mu.RUnlock()
		return contract.ExportManifestV1{}, contract.ErrNotCurrent
	}
	entries := make([]contract.ExportEntryV1, 0)
	for _, history := range t.records {
		for i := len(history) - 1; i >= 0; i-- {
			record := history[i]
			if record.Watermark <= view.WatermarkRef.Revision {
				if _, merged := t.mergedInto[record.Ref.ID]; !merged && record.Current(now) && record.IdentityID == access.IdentityID && record.AuthorityEpoch == access.AuthorityEpoch && contract.SameRef(record.AuthorityRef, access.AuthorityRef) && contract.SameRef(record.PolicyRef, access.PolicyRef) && contains(view.Scopes, record.Scope) && sensitivityAllowed(view.SensitivityMax, record.Sensitivity) {
					entries = append(entries, contract.ExportEntryV1{RecordRef: record.Ref, ContentRef: cloneContentRef(record.ContentRef), SourceRefs: append([]contract.Ref{}, record.SourceRefs...), EvidenceRefs: append([]contract.Ref{}, record.EvidenceRefs...), Scope: record.Scope, Sensitivity: record.Sensitivity})
				}
				break
			}
		}
	}
	s.mu.RUnlock()
	return contract.SealExportManifestV1(contract.ExportManifestV1{Ref: contract.Ref{ID: id, Revision: 1}, Owner: contract.OwnerMemory, TenantID: access.TenantID, ViewRef: viewRef, Entries: entries, CreatedAt: now, ExpiresAt: now.Add(ttl)})
}

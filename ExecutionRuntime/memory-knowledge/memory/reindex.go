package memory

import (
	"fmt"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func (s *Store) ReindexLocal(access Access, projection Projection, expectedProjection contract.ExpectedRevision, descriptor contract.IndexDescriptorV1, expectedDescriptor contract.ExpectedRevision) (Projection, contract.IndexDescriptorV1, error) {
	if err := access.validate(); err != nil {
		return Projection{}, contract.IndexDescriptorV1{}, err
	}
	if descriptor.Kind != contract.IndexKind(projection.Kind) {
		return Projection{}, contract.IndexDescriptorV1{}, contract.ErrEvidenceConflict
	}
	if err := expectedProjection.Validate(); err != nil {
		return Projection{}, contract.IndexDescriptorV1{}, err
	}
	if err := expectedDescriptor.Validate(); err != nil {
		return Projection{}, contract.IndexDescriptorV1{}, err
	}
	published, err := sealProjection(projection)
	if err != nil {
		return Projection{}, contract.IndexDescriptorV1{}, err
	}
	now := s.clock.Now().UTC()
	if published.TenantID != access.TenantID || strings.TrimSpace(published.Ref.ID) == "" || published.Ref.Revision == 0 || strings.TrimSpace(published.Kind) == "" || strings.TrimSpace(published.BuilderVersion) == "" || !published.ExpiresAt.After(now) {
		return Projection{}, contract.IndexDescriptorV1{}, contract.ErrScopeDenied
	}
	if published.State != ProjectionReady && published.State != ProjectionPartial && published.State != ProjectionStale {
		return Projection{}, contract.IndexDescriptorV1{}, fmt.Errorf("%w: projection state", contract.ErrInvalidArgument)
	}
	descriptor.Owner = contract.OwnerMemory
	descriptor.RecordRefs = []contract.Ref{published.RecordRef}
	descriptor.Coverage.ProjectionRefs = contract.NormalizeRefs(append(descriptor.Coverage.ProjectionRefs, published.Ref))
	descriptor, err = contract.SealIndexDescriptorV1(descriptor)
	if err != nil {
		return Projection{}, contract.IndexDescriptorV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tenant(access.TenantID)
	record := findRecordByRef(t, published.RecordRef)
	if record.Ref.ID == "" || record.IdentityID != access.IdentityID || record.AuthorityEpoch != access.AuthorityEpoch || !contract.SameRef(record.AuthorityRef, access.AuthorityRef) || !contract.SameRef(record.PolicyRef, access.PolicyRef) {
		return Projection{}, contract.IndexDescriptorV1{}, contract.ErrNotFound
	}
	versions := t.projections[published.Ref.ID]
	exists := len(versions) > 0
	current := uint64(0)
	if exists {
		current = versions[len(versions)-1].Ref.Revision
		priorRecord := findRecordByRef(t, versions[len(versions)-1].RecordRef)
		if priorRecord.IdentityID != access.IdentityID {
			return Projection{}, contract.IndexDescriptorV1{}, contract.ErrNotFound
		}
	}
	if !expectedProjection.Matches(exists, current) || published.Ref.Revision != current+1 {
		return Projection{}, contract.IndexDescriptorV1{}, contract.ErrRevisionConflict
	}
	index, err := s.indexes.PublishAtomic(now, descriptor, expectedDescriptor, func() error {
		t.projections[published.Ref.ID] = append(versions, cloneProjection(published))
		return nil
	})
	if err != nil {
		return Projection{}, contract.IndexDescriptorV1{}, err
	}
	return published, index, nil
}
func (s *Store) ListIndexDescriptors(access Access) ([]contract.IndexDescriptorV1, error) {
	if err := access.validate(); err != nil {
		return nil, err
	}
	return s.indexes.List(s.clock.Now(), contract.OwnerMemory)
}

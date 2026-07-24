package knowledge

import "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"

func (s *Store) ReindexLocal(access Access, input ProjectionInput, expectedProjection contract.ExpectedRevision, descriptor contract.IndexDescriptorV1, expectedDescriptor contract.ExpectedRevision) (Projection, contract.IndexDescriptorV1, error) {
	if err := access.Validate(); err != nil {
		return Projection{}, contract.IndexDescriptorV1{}, err
	}
	if descriptor.Kind != contract.IndexKind(input.Kind) {
		return Projection{}, contract.IndexDescriptorV1{}, contract.ErrEvidenceConflict
	}
	if err := expectedProjection.Validate(); err != nil {
		return Projection{}, contract.IndexDescriptorV1{}, err
	}
	if err := expectedDescriptor.Validate(); err != nil {
		return Projection{}, contract.IndexDescriptorV1{}, err
	}
	if access.TenantID != input.TenantID {
		return Projection{}, contract.IndexDescriptorV1{}, contract.ErrScopeDenied
	}
	if !validID(input.TenantID, input.ID, input.Kind, input.BuilderVersion) || len(input.RecordRefs) == 0 || validateTTL(input.TTL) != nil || validateRefs(input.RecordRefs) != nil || input.State != ProjectionReady && input.State != ProjectionPartial && input.State != ProjectionStale {
		return Projection{}, contract.IndexDescriptorV1{}, contract.ErrInvalidArgument
	}
	now := s.clock.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tenantLocked(input.TenantID, true)
	if err != nil {
		return Projection{}, contract.IndexDescriptorV1{}, err
	}
	for _, ref := range input.RecordRefs {
		record, ok := recordByRef(t, ref)
		if !ok {
			return Projection{}, contract.IndexDescriptorV1{}, contract.ErrNotFound
		}
		if !accessAllows(access, record.AuthorityRef, record.PolicyRef) {
			return Projection{}, contract.IndexDescriptorV1{}, contract.ErrScopeDenied
		}
	}
	history := t.projections[input.ID]
	exists := len(history) != 0
	revision := uint64(0)
	if exists {
		revision = history[len(history)-1].Ref.Revision
	}
	if !expectedProjection.Matches(exists, revision) {
		return Projection{}, contract.IndexDescriptorV1{}, contract.ErrRevisionConflict
	}
	projection := Projection{Ref: contract.Ref{ID: input.ID, Revision: revision + 1}, TenantID: input.TenantID, Owner: contract.OwnerKnowledge, Kind: input.Kind, SnapshotRef: input.SnapshotRef, RecordRefs: contract.NormalizeRefs(input.RecordRefs), BuilderVersion: input.BuilderVersion, Coverage: cloneCoverage(input.Coverage), State: input.State, CreatedAt: now, ExpiresAt: now.Add(input.TTL)}
	if err := setCanonicalDigest(&projection.Ref, projection); err != nil {
		return Projection{}, contract.IndexDescriptorV1{}, err
	}
	descriptor.Owner = contract.OwnerKnowledge
	descriptor.RecordRefs = contract.NormalizeRefs(input.RecordRefs)
	descriptor.Coverage.ProjectionRefs = contract.NormalizeRefs(append(descriptor.Coverage.ProjectionRefs, projection.Ref))
	descriptor, err = contract.SealIndexDescriptorV1(descriptor)
	if err != nil {
		return Projection{}, contract.IndexDescriptorV1{}, err
	}
	index, err := s.indexes.PublishAtomic(now, descriptor, expectedDescriptor, func() error {
		t.projections[input.ID] = append(history, projection)
		return nil
	})
	if err != nil {
		return Projection{}, contract.IndexDescriptorV1{}, err
	}
	return projection, index, nil
}
func (s *Store) ListIndexDescriptors(access Access) ([]contract.IndexDescriptorV1, error) {
	if err := access.Validate(); err != nil {
		return nil, err
	}
	return s.indexes.List(s.clock.Now(), contract.OwnerKnowledge)
}

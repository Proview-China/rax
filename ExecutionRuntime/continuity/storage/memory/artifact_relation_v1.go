package memory

import (
	"context"
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type artifactRelationKeyV1 struct {
	tenantID    string
	scopeDigest string
	relationID  string
}

type artifactRelationRequestKeyV1 struct {
	tenantID       string
	scopeDigest    string
	idempotencyKey string
}

func artifactRelationKeyFromFactV1(fact contract.ArtifactRelationFactV1) artifactRelationKeyV1 {
	return artifactRelationKeyV1{tenantID: fact.Scope.TenantID, scopeDigest: fact.Scope.ExecutionScopeDigest, relationID: fact.RelationID}
}

func (b *Backend) CreateArtifactRelationFactV1(_ context.Context, fact contract.ArtifactRelationFactV1) (contract.ArtifactRelationFactV1, bool, error) {
	if err := fact.Validate(); err != nil {
		return contract.ArtifactRelationFactV1{}, false, err
	}
	key := artifactRelationKeyFromFactV1(fact)
	requestKey := artifactRelationRequestKeyV1{tenantID: key.tenantID, scopeDigest: key.scopeDigest, idempotencyKey: fact.IdempotencyKey}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existingKey, ok := b.artifactRelationByRequestV1[requestKey]; ok && existingKey != key {
		return contract.ArtifactRelationFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another Artifact Relation")
	}
	if existing, ok := b.artifactRelationsV1[key]; ok {
		if existing.Ref().Exact().Equal(fact.Ref().Exact()) && existing.IdempotencyKey == fact.IdempotencyKey {
			return existing.Clone(), true, nil
		}
		return contract.ArtifactRelationFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "relation_id", "create-once Artifact Relation changed content")
	}
	b.artifactRelationsV1[key] = fact.Clone()
	b.artifactRelationByRequestV1[requestKey] = key
	artifactKey := fact.SourceProjection.Artifact.ArtifactFactRef.IdentityKey()
	relatedKey := fact.SourceProjection.RelatedFactRef.IdentityKey()
	indexArtifactRelationV1(b.artifactRelationsByArtifactV1, artifactKey, key)
	indexArtifactRelationV1(b.artifactRelationsByRelatedV1, relatedKey, key)
	return fact.Clone(), false, nil
}

func indexArtifactRelationV1(index map[contract.ExactFactIdentityKeyV2]map[artifactRelationKeyV1]struct{}, identity contract.ExactFactIdentityKeyV2, key artifactRelationKeyV1) {
	entries := index[identity]
	if entries == nil {
		entries = make(map[artifactRelationKeyV1]struct{})
		index[identity] = entries
	}
	entries[key] = struct{}{}
}

func (b *Backend) InspectArtifactRelationV1(_ context.Context, request ports.InspectArtifactRelationRequestV1) (contract.ArtifactRelationFactV1, error) {
	if err := request.Ref.Validate(); err != nil {
		return contract.ArtifactRelationFactV1{}, err
	}
	ref := request.Ref.Exact()
	key := artifactRelationKeyV1{tenantID: ref.TenantID, scopeDigest: ref.ScopeDigest, relationID: ref.ID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	fact, ok := b.artifactRelationsV1[key]
	if !ok {
		return contract.ArtifactRelationFactV1{}, contract.NewError(contract.ErrNotFound, "artifact_relation_ref", "Artifact Relation not found")
	}
	if !fact.Ref().Exact().Equal(ref) {
		return contract.ArtifactRelationFactV1{}, contract.NewError(contract.ErrRevisionConflict, "artifact_relation_ref", "exact Artifact Relation ref mismatch")
	}
	return fact.Clone(), nil
}

func (b *Backend) InspectArtifactRelationByIDV1(_ context.Context, request ports.InspectArtifactRelationByIDRequestV1) (contract.ArtifactRelationFactV1, error) {
	if err := validateArtifactRelationByIDRequestV1(request); err != nil {
		return contract.ArtifactRelationFactV1{}, err
	}
	key := artifactRelationKeyV1{tenantID: request.TenantID, scopeDigest: request.ScopeDigest, relationID: request.RelationID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	fact, ok := b.artifactRelationsV1[key]
	if !ok {
		return contract.ArtifactRelationFactV1{}, contract.NewError(contract.ErrNotFound, "artifact_relation_id", "Artifact Relation not found")
	}
	if fact.Owner != request.Owner {
		return contract.ArtifactRelationFactV1{}, contract.NewError(contract.ErrRevisionConflict, "owner_binding", "Artifact Relation Owner mismatch")
	}
	return fact.Clone(), nil
}

func (b *Backend) ListArtifactRelationsV1(_ context.Context, request ports.ListArtifactRelationsRequestV1) ([]contract.ArtifactRelationFactV1, error) {
	if err := request.ArtifactFactRef.Validate(); err != nil {
		return nil, err
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.listArtifactRelationIndexV1(b.artifactRelationsByArtifactV1[request.ArtifactFactRef.IdentityKey()]), nil
}

func (b *Backend) ListRelatedArtifactRelationsV1(_ context.Context, request ports.ListRelatedArtifactRelationsRequestV1) ([]contract.ArtifactRelationFactV1, error) {
	if err := request.RelatedFactRef.Validate(); err != nil {
		return nil, err
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.listArtifactRelationIndexV1(b.artifactRelationsByRelatedV1[request.RelatedFactRef.IdentityKey()]), nil
}

func (b *Backend) listArtifactRelationIndexV1(keys map[artifactRelationKeyV1]struct{}) []contract.ArtifactRelationFactV1 {
	result := make([]contract.ArtifactRelationFactV1, 0, len(keys))
	for key := range keys {
		result = append(result, b.artifactRelationsV1[key].Clone())
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedUnixNano == result[j].CreatedUnixNano {
			return result[i].RelationID < result[j].RelationID
		}
		return result[i].CreatedUnixNano < result[j].CreatedUnixNano
	})
	return result
}

func validateArtifactRelationByIDRequestV1(request ports.InspectArtifactRelationByIDRequestV1) error {
	for field, value := range map[string]string{"tenant_id": request.TenantID, "scope_digest": request.ScopeDigest, "relation_id": request.RelationID} {
		if err := contract.ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := request.Owner.Validate(); err != nil {
		return err
	}
	if request.Owner.ComponentID != contract.ContinuityComponentID || request.Owner.Capability != contract.ArtifactRelationCapabilityV1 || request.Owner.FactKind != "artifact_relation_fact_v1" {
		return contract.NewError(contract.ErrInvalidArgument, "owner_binding", "wrong Continuity Artifact Relation owner")
	}
	return nil
}

var _ ports.ArtifactRelationReaderV1 = (*Backend)(nil)

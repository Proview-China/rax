package memory

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type checkpointObjectKeyV2 struct {
	tenantID    string
	scopeDigest string
	id          string
}

type checkpointRequestKeyV2 struct {
	tenantID       string
	scopeDigest    string
	idempotencyKey string
}

func manifestKeyFromFactV2(fact contract.CheckpointManifestFactV2) checkpointObjectKeyV2 {
	return checkpointObjectKeyV2{tenantID: fact.Scope.TenantID, scopeDigest: fact.Scope.ExecutionScopeDigest, id: fact.ManifestID}
}

func objectKeyFromExactRefV2(ref contract.ExactFactRefV2) checkpointObjectKeyV2 {
	return checkpointObjectKeyV2{tenantID: ref.TenantID, scopeDigest: ref.ScopeDigest, id: ref.ID}
}

func sealKeyFromFactV2(seal contract.CheckpointManifestSealFactV2) checkpointObjectKeyV2 {
	return checkpointObjectKeyV2{tenantID: seal.TenantID, scopeDigest: seal.ScopeDigest, id: seal.SealID}
}

func (b *Backend) CreateCheckpointManifestFactV2(
	_ context.Context,
	fact contract.CheckpointManifestFactV2,
) (contract.CheckpointManifestFactV2, bool, error) {
	if err := fact.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	if fact.Revision != 1 || fact.State != contract.ManifestCollecting {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrInvalidArgument, "manifest_create", "revision 1 collecting fact is required")
	}
	key := manifestKeyFromFactV2(fact)
	requestKey := checkpointRequestKeyV2{tenantID: key.tenantID, scopeDigest: key.scopeDigest, idempotencyKey: fact.IdempotencyKey}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existingKey, exists := b.checkpointManifestByRequestV2[requestKey]; exists && existingKey != key {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another manifest in this tenant scope")
	}
	history := b.checkpointManifestsV2[key]
	if history != nil {
		initial, exists := history[1]
		if exists && initial.Ref().Exact().Equal(fact.Ref().Exact()) {
			return initial.Clone(), true, nil
		}
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_id", "create-once manifest identity changed")
	}
	b.checkpointManifestsV2[key] = map[uint64]contract.CheckpointManifestFactV2{1: fact.Clone()}
	b.checkpointManifestCurrentV2[key] = 1
	b.checkpointManifestByRequestV2[requestKey] = key
	return fact.Clone(), false, nil
}

func (b *Backend) CompareAndSwapCheckpointManifestFactV2(
	_ context.Context,
	expected contract.CheckpointManifestRefV2,
	next contract.CheckpointManifestFactV2,
) (contract.CheckpointManifestFactV2, bool, error) {
	if err := expected.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	if err := next.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	key := objectKeyFromExactRefV2(expected.Exact())
	if key != manifestKeyFromFactV2(next) {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_key", "tenant, scope, or manifest ID changed")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	history := b.checkpointManifestsV2[key]
	if history == nil {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrNotFound, "manifest_key", "manifest not found in tenant scope")
	}
	currentRevision := b.checkpointManifestCurrentV2[key]
	current := history[currentRevision]
	if currentRevision == expected.Exact().Revision+1 && current.Ref().Exact().Equal(next.Ref().Exact()) {
		return current.Clone(), true, nil
	}
	if !current.Ref().Exact().Equal(expected.Exact()) {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_ref", "CAS expected ref is not current")
	}
	if next.Revision != current.Revision+1 {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_revision", "CAS must advance exactly one revision")
	}
	if err := validateManifestMutationIdentityV2(current, next); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	if err := contract.AdvanceCheckpointManifestStateV2(current.State, next.State); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	if _, exists := history[next.Revision]; exists {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_revision", "history revision already exists")
	}
	history[next.Revision] = next.Clone()
	b.checkpointManifestCurrentV2[key] = next.Revision
	return next.Clone(), false, nil
}

func (b *Backend) InspectCheckpointManifestV2(
	_ context.Context,
	request ports.InspectCheckpointManifestRequestV2,
) (contract.CheckpointManifestFactV2, error) {
	if err := request.Ref.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	ref := request.Ref.Exact()
	key := objectKeyFromExactRefV2(ref)
	b.mu.RLock()
	defer b.mu.RUnlock()
	history := b.checkpointManifestsV2[key]
	if history == nil {
		return contract.CheckpointManifestFactV2{}, contract.NewError(contract.ErrNotFound, "manifest_key", "manifest not found in tenant scope")
	}
	fact, exists := history[ref.Revision]
	if !exists {
		return contract.CheckpointManifestFactV2{}, contract.NewError(contract.ErrNotFound, "manifest_revision", "manifest revision not found")
	}
	if !fact.Ref().Exact().Equal(ref) {
		return contract.CheckpointManifestFactV2{}, contract.NewError(contract.ErrRevisionConflict, "manifest_ref", "exact manifest ref or owner mismatch")
	}
	return fact.Clone(), nil
}

func (b *Backend) InspectCurrentCheckpointManifestV2(
	_ context.Context,
	request ports.InspectCurrentCheckpointManifestRequestV2,
) (contract.CheckpointManifestFactV2, error) {
	if err := request.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	key := checkpointObjectKeyV2{tenantID: request.TenantID, scopeDigest: request.ScopeDigest, id: request.ManifestID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	history := b.checkpointManifestsV2[key]
	if history == nil {
		return contract.CheckpointManifestFactV2{}, contract.NewError(contract.ErrNotFound, "manifest_key", "manifest not found in tenant scope")
	}
	fact := history[b.checkpointManifestCurrentV2[key]]
	if fact.Owner != request.Owner {
		return contract.CheckpointManifestFactV2{}, contract.NewError(contract.ErrRevisionConflict, "owner_binding", "current manifest owner mismatch")
	}
	return fact.Clone(), nil
}

func (b *Backend) CreateCheckpointManifestSealFactV2(
	_ context.Context,
	seal contract.CheckpointManifestSealFactV2,
) (contract.CheckpointManifestSealFactV2, bool, error) {
	if err := seal.Validate(); err != nil {
		return contract.CheckpointManifestSealFactV2{}, false, err
	}
	sealKey := sealKeyFromFactV2(seal)
	requestKey := checkpointRequestKeyV2{tenantID: seal.TenantID, scopeDigest: seal.ScopeDigest, idempotencyKey: seal.IdempotencyKey}
	manifestKey := objectKeyFromExactRefV2(seal.ManifestRef.Exact())
	b.mu.Lock()
	defer b.mu.Unlock()
	history := b.checkpointManifestsV2[manifestKey]
	if history == nil {
		return contract.CheckpointManifestSealFactV2{}, false, contract.NewError(contract.ErrNotFound, "manifest_key", "seal manifest not found in tenant scope")
	}
	current := history[b.checkpointManifestCurrentV2[manifestKey]]
	if !current.Ref().Exact().Equal(seal.ManifestRef.Exact()) {
		return contract.CheckpointManifestSealFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_ref", "seal manifest ref is not current")
	}
	if err := contract.ValidateCheckpointManifestSealBindingV2(current, seal); err != nil {
		return contract.CheckpointManifestSealFactV2{}, false, err
	}
	if existing, exists := b.checkpointManifestSealsV2[sealKey]; exists {
		if existing.Ref().Exact().Equal(seal.Ref().Exact()) {
			return existing.Clone(), true, nil
		}
		return contract.CheckpointManifestSealFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "seal_id", "immutable seal content changed")
	}
	if existingKey, exists := b.checkpointSealByRequestV2[requestKey]; exists && existingKey != sealKey {
		return contract.CheckpointManifestSealFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another seal in this tenant scope")
	}
	manifestIdentity := seal.ManifestRef.Exact().IdentityKey()
	if existingKey, exists := b.checkpointSealByManifestV2[manifestIdentity]; exists && existingKey != sealKey {
		return contract.CheckpointManifestSealFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_ref", "exact manifest revision already has a seal")
	}
	b.checkpointManifestSealsV2[sealKey] = seal.Clone()
	b.checkpointSealByRequestV2[requestKey] = sealKey
	b.checkpointSealByManifestV2[manifestIdentity] = sealKey
	return seal.Clone(), false, nil
}

func (b *Backend) InspectCheckpointManifestSealV2(
	_ context.Context,
	request ports.InspectCheckpointManifestSealRequestV2,
) (contract.CheckpointManifestSealFactV2, error) {
	if err := request.Ref.Validate(); err != nil {
		return contract.CheckpointManifestSealFactV2{}, err
	}
	ref := request.Ref.Exact()
	key := objectKeyFromExactRefV2(ref)
	b.mu.RLock()
	defer b.mu.RUnlock()
	seal, exists := b.checkpointManifestSealsV2[key]
	if !exists {
		return contract.CheckpointManifestSealFactV2{}, contract.NewError(contract.ErrNotFound, "seal_key", "manifest seal not found in tenant scope")
	}
	if !seal.Ref().Exact().Equal(ref) {
		return contract.CheckpointManifestSealFactV2{}, contract.NewError(contract.ErrRevisionConflict, "manifest_seal_ref", "exact seal ref or owner mismatch")
	}
	return seal.Clone(), nil
}

func validateManifestMutationIdentityV2(current, next contract.CheckpointManifestFactV2) error {
	currentFrames, err := contract.ExactRefSetDigestV2(current.ContextFrameRefs)
	if err != nil {
		return err
	}
	nextFrames, err := contract.ExactRefSetDigestV2(next.ContextFrameRefs)
	if err != nil {
		return err
	}
	if current.Owner != next.Owner || current.Scope != next.Scope || current.IdempotencyKey != next.IdempotencyKey ||
		!current.CheckpointAttemptRef.Equal(next.CheckpointAttemptRef) ||
		!current.BarrierRef.Equal(next.BarrierRef) ||
		!current.EffectCutRef.Equal(next.EffectCutRef) ||
		current.TimelineCut != next.TimelineCut ||
		!current.ContextGenerationRef.Equal(next.ContextGenerationRef) ||
		currentFrames != nextFrames ||
		current.RequiredParticipantSetDigest != next.RequiredParticipantSetDigest ||
		current.CreatedUnixNano != next.CreatedUnixNano {
		return contract.NewError(contract.ErrRevisionConflict, "manifest_identity", "immutable manifest identity changed")
	}
	return nil
}

var _ ports.CheckpointManifestRepositoryV2 = (*Backend)(nil)

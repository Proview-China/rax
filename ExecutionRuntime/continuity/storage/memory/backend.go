// Package memory provides an in-process reference backend for tests and local
// semantic validation. It is not a production persistence backend or SLA.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

type objectEntry struct {
	manifest  contract.ObjectManifest
	committed bool
	visible   bool
}

type Backend struct {
	mu    sync.RWMutex
	clock func() time.Time

	recordsByEvidence  map[string]contract.TimelineEventRecord
	sourceToEvidence   map[string]string
	sequenceToEvidence map[string]map[uint64]string
	tombstonesByID     map[string]contract.TimelineProjectionTombstoneFactV1
	visibilityOverlay  map[string]string

	timelineAttemptsV1       map[timelineAttemptKeyV1]map[uint64]contract.TimelineProjectionAttemptFactV1
	timelineAttemptCurrentV1 map[timelineAttemptKeyV1]uint64
	timelineIdempotencyV1    map[timelineIdempotencyKeyV1]timelineAttemptKeyV1
	timelineCurrentV1        map[timelineEventKeyV1]contract.TimelineProjectionCurrentV1
	timelinePoliciesV1       map[timelinePolicyKeyV1]map[uint64]contract.TimelineProjectionPolicyCurrentV1
	timelinePolicyCurrentV1  map[timelinePolicyKeyV1]uint64

	objects    map[string]objectEntry
	chunks     map[string][]byte
	journals   map[string]contract.WriteJournal
	retentions map[string]contract.RetentionFact

	checkpointManifestsV2         map[checkpointObjectKeyV2]map[uint64]contract.CheckpointManifestFactV2
	checkpointManifestCurrentV2   map[checkpointObjectKeyV2]uint64
	checkpointManifestByRequestV2 map[checkpointRequestKeyV2]checkpointObjectKeyV2
	checkpointManifestSealsV2     map[checkpointObjectKeyV2]contract.CheckpointManifestSealFactV2
	checkpointSealByRequestV2     map[checkpointRequestKeyV2]checkpointObjectKeyV2
	checkpointSealByManifestV2    map[contract.ExactFactIdentityKeyV2]checkpointObjectKeyV2

	restorePlansV2         map[checkpointObjectKeyV2]map[uint64]contract.RestorePlanFactV2
	restorePlanCurrentV2   map[checkpointObjectKeyV2]uint64
	restorePlanByRequestV2 map[checkpointRequestKeyV2]checkpointObjectKeyV2
	rewindPlansV2          map[checkpointObjectKeyV2]map[uint64]contract.RewindPlanFactV2
	rewindPlanCurrentV2    map[checkpointObjectKeyV2]uint64
	rewindPlanByRequestV2  map[checkpointRequestKeyV2]checkpointObjectKeyV2

	artifactRelationsV1           map[artifactRelationKeyV1]contract.ArtifactRelationFactV1
	artifactRelationByRequestV1   map[artifactRelationRequestKeyV1]artifactRelationKeyV1
	artifactRelationsByArtifactV1 map[contract.ExactFactIdentityKeyV2]map[artifactRelationKeyV1]struct{}
	artifactRelationsByRelatedV1  map[contract.ExactFactIdentityKeyV2]map[artifactRelationKeyV1]struct{}

	contentIntegrityAuditsV1         map[contentIntegrityAuditKeyV1]contract.ContentIntegrityAuditFactV1
	contentIntegrityAuditByRequestV1 map[contentIntegrityAuditRequestKeyV1]contentIntegrityAuditKeyV1

	contentDeltasV1         map[contentDeltaKeyV1]contract.ContentDeltaFactV1
	contentDeltaByRequestV1 map[contentDeltaRequestKeyV1]contentDeltaKeyV1

	historyDerivationCandidatesV1 map[historyDerivationCandidateKeyV1]contract.HistoryDerivationCandidateFactV1
	historyDerivationByRequestV1  map[historyDerivationRequestKeyV1]historyDerivationCandidateKeyV1
}

func New() *Backend {
	return NewWithClock(time.Now)
}

func NewWithClock(clock func() time.Time) *Backend {
	if clock == nil {
		clock = time.Now
	}
	return &Backend{
		clock:                    clock,
		recordsByEvidence:        make(map[string]contract.TimelineEventRecord),
		sourceToEvidence:         make(map[string]string),
		sequenceToEvidence:       make(map[string]map[uint64]string),
		tombstonesByID:           make(map[string]contract.TimelineProjectionTombstoneFactV1),
		visibilityOverlay:        make(map[string]string),
		timelineAttemptsV1:       make(map[timelineAttemptKeyV1]map[uint64]contract.TimelineProjectionAttemptFactV1),
		timelineAttemptCurrentV1: make(map[timelineAttemptKeyV1]uint64),
		timelineIdempotencyV1:    make(map[timelineIdempotencyKeyV1]timelineAttemptKeyV1),
		timelineCurrentV1:        make(map[timelineEventKeyV1]contract.TimelineProjectionCurrentV1),
		timelinePoliciesV1:       make(map[timelinePolicyKeyV1]map[uint64]contract.TimelineProjectionPolicyCurrentV1),
		timelinePolicyCurrentV1:  make(map[timelinePolicyKeyV1]uint64),
		objects:                  make(map[string]objectEntry), chunks: make(map[string][]byte),
		journals: make(map[string]contract.WriteJournal), retentions: make(map[string]contract.RetentionFact),
		checkpointManifestsV2:            make(map[checkpointObjectKeyV2]map[uint64]contract.CheckpointManifestFactV2),
		checkpointManifestCurrentV2:      make(map[checkpointObjectKeyV2]uint64),
		checkpointManifestByRequestV2:    make(map[checkpointRequestKeyV2]checkpointObjectKeyV2),
		checkpointManifestSealsV2:        make(map[checkpointObjectKeyV2]contract.CheckpointManifestSealFactV2),
		checkpointSealByRequestV2:        make(map[checkpointRequestKeyV2]checkpointObjectKeyV2),
		checkpointSealByManifestV2:       make(map[contract.ExactFactIdentityKeyV2]checkpointObjectKeyV2),
		restorePlansV2:                   make(map[checkpointObjectKeyV2]map[uint64]contract.RestorePlanFactV2),
		restorePlanCurrentV2:             make(map[checkpointObjectKeyV2]uint64),
		restorePlanByRequestV2:           make(map[checkpointRequestKeyV2]checkpointObjectKeyV2),
		rewindPlansV2:                    make(map[checkpointObjectKeyV2]map[uint64]contract.RewindPlanFactV2),
		rewindPlanCurrentV2:              make(map[checkpointObjectKeyV2]uint64),
		rewindPlanByRequestV2:            make(map[checkpointRequestKeyV2]checkpointObjectKeyV2),
		artifactRelationsV1:              make(map[artifactRelationKeyV1]contract.ArtifactRelationFactV1),
		artifactRelationByRequestV1:      make(map[artifactRelationRequestKeyV1]artifactRelationKeyV1),
		artifactRelationsByArtifactV1:    make(map[contract.ExactFactIdentityKeyV2]map[artifactRelationKeyV1]struct{}),
		artifactRelationsByRelatedV1:     make(map[contract.ExactFactIdentityKeyV2]map[artifactRelationKeyV1]struct{}),
		contentIntegrityAuditsV1:         make(map[contentIntegrityAuditKeyV1]contract.ContentIntegrityAuditFactV1),
		contentIntegrityAuditByRequestV1: make(map[contentIntegrityAuditRequestKeyV1]contentIntegrityAuditKeyV1),
		contentDeltasV1:                  make(map[contentDeltaKeyV1]contract.ContentDeltaFactV1),
		contentDeltaByRequestV1:          make(map[contentDeltaRequestKeyV1]contentDeltaKeyV1),
		historyDerivationCandidatesV1:    make(map[historyDerivationCandidateKeyV1]contract.HistoryDerivationCandidateFactV1),
		historyDerivationByRequestV1:     make(map[historyDerivationRequestKeyV1]historyDerivationCandidateKeyV1),
	}
}

func (b *Backend) PutProjection(_ context.Context, record contract.TimelineEventRecord) (contract.TimelineEventRecord, bool, error) {
	if err := record.Validate(); err != nil {
		return contract.TimelineEventRecord{}, false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existing, ok := b.recordsByEvidence[record.EvidenceRecordRef]; ok {
		if existing.Candidate.Digest != record.Candidate.Digest {
			return contract.TimelineEventRecord{}, false, contract.NewError(contract.ErrProjectionConflict, "evidence_ref", "same evidence changed projection")
		}
		return existing.Clone(), true, nil
	}
	sourceKey := scopedSource(record)
	if evidenceRef, ok := b.sourceToEvidence[sourceKey]; ok {
		existing := b.recordsByEvidence[evidenceRef]
		if existing.EvidenceRecordDigest != record.EvidenceRecordDigest {
			return contract.TimelineEventRecord{}, false, contract.NewError(contract.ErrEvidenceConflict, "source_key", "same source sequence changed evidence")
		}
		if existing.Candidate.Digest != record.Candidate.Digest {
			return contract.TimelineEventRecord{}, false, contract.NewError(contract.ErrProjectionConflict, "source_key", "same evidence source changed projection semantics")
		}
		return existing.Clone(), true, nil
	}
	sequences := b.sequenceToEvidence[record.LedgerScopeDigest]
	if sequences == nil {
		sequences = make(map[uint64]string)
		b.sequenceToEvidence[record.LedgerScopeDigest] = sequences
	}
	if evidenceRef, ok := sequences[record.LedgerSequence]; ok {
		existing := b.recordsByEvidence[evidenceRef]
		if existing.EvidenceRecordDigest != record.EvidenceRecordDigest {
			return contract.TimelineEventRecord{}, false, contract.NewError(contract.ErrEvidenceConflict, "ledger_sequence", "sequence belongs to different evidence")
		}
		if existing.Candidate.Digest != record.Candidate.Digest {
			return contract.TimelineEventRecord{}, false, contract.NewError(contract.ErrProjectionConflict, "ledger_sequence", "same evidence sequence changed projection semantics")
		}
		return existing.Clone(), true, nil
	}
	if projectionCycle(b.recordsByEvidence, record) {
		return contract.TimelineEventRecord{}, false, contract.NewError(contract.ErrProjectionConflict, "parent_refs", "cycle detected")
	}
	b.recordsByEvidence[record.EvidenceRecordRef] = record.Clone()
	b.sourceToEvidence[sourceKey] = record.EvidenceRecordRef
	sequences[record.LedgerSequence] = record.EvidenceRecordRef
	return record.Clone(), false, nil
}

func projectionCycle(existing map[string]contract.TimelineEventRecord, incoming contract.TimelineEventRecord) bool {
	graph := make(map[string][]string, len(existing)+1)
	for _, record := range existing {
		if record.LedgerScopeDigest == incoming.LedgerScopeDigest {
			graph[record.Candidate.CandidateID] = append([]string{}, record.Candidate.ParentRefs...)
		}
	}
	graph[incoming.Candidate.CandidateID] = append([]string{}, incoming.Candidate.ParentRefs...)
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, parent := range graph[id] {
			if _, exists := graph[parent]; exists && visit(parent) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	for id := range graph {
		if visit(id) {
			return true
		}
	}
	return false
}

func (b *Backend) InspectByEvidence(_ context.Context, evidenceRef string) (contract.TimelineEventRecord, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	record, ok := b.recordsByEvidence[evidenceRef]
	if !ok {
		return contract.TimelineEventRecord{}, contract.NewError(contract.ErrNotFound, "evidence_ref", "projection not found")
	}
	return record.Clone(), nil
}

func (b *Backend) ListLedgerScope(_ context.Context, scope string) ([]contract.TimelineEventRecord, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]contract.TimelineEventRecord, 0)
	for _, record := range b.recordsByEvidence {
		if record.LedgerScopeDigest == scope {
			view := record.Clone()
			if tombstoneID, ok := b.visibilityOverlay[record.EvidenceRecordRef]; ok {
				view.Visibility = "tombstoned"
				view.TombstoneRef = tombstoneID
			}
			result = append(result, view)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LedgerSequence < result[j].LedgerSequence })
	return result, nil
}

func (b *Backend) CreateTombstoneOverlay(_ context.Context, fact contract.TimelineProjectionTombstoneFactV1) (contract.TimelineProjectionTombstoneFactV1, bool, error) {
	if err := fact.Validate(); err != nil {
		return contract.TimelineProjectionTombstoneFactV1{}, false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existing, ok := b.tombstonesByID[fact.TombstoneID]; ok {
		if existing.Digest == fact.Digest {
			return existing, true, nil
		}
		return contract.TimelineProjectionTombstoneFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "tombstone_id", "create-once tombstone changed content")
	}
	record, ok := b.recordsByEvidence[fact.EvidenceRecordRef]
	if !ok {
		return contract.TimelineProjectionTombstoneFactV1{}, false, contract.NewError(contract.ErrNotFound, "evidence_ref", "projection not found")
	}
	if record.Candidate.Scope.ExecutionScopeDigest != fact.ScopeDigest {
		return contract.TimelineProjectionTombstoneFactV1{}, false, contract.NewError(contract.ErrProjectionConflict, "scope_digest", "tombstone belongs to another execution scope")
	}
	if _, ok := b.visibilityOverlay[fact.EvidenceRecordRef]; ok {
		return contract.TimelineProjectionTombstoneFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "tombstone_ref", "projection already has another immutable tombstone")
	}
	b.tombstonesByID[fact.TombstoneID] = fact
	b.visibilityOverlay[fact.EvidenceRecordRef] = fact.TombstoneID
	return fact, false, nil
}

func (b *Backend) InspectTombstone(_ context.Context, tombstoneID string) (contract.TimelineProjectionTombstoneFactV1, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	fact, ok := b.tombstonesByID[tombstoneID]
	if !ok {
		return contract.TimelineProjectionTombstoneFactV1{}, contract.NewError(contract.ErrNotFound, "tombstone_id", "tombstone not found")
	}
	return fact, nil
}

func scopedSource(record contract.TimelineEventRecord) string {
	return record.LedgerScopeDigest + "|" + record.Candidate.Evidence.SourceKey.String()
}

func (b *Backend) CreateJournal(_ context.Context, journal contract.WriteJournal) error {
	if err := journal.Validate(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existing, ok := b.journals[journal.JournalID]; ok {
		if existing.ObjectID == journal.ObjectID && existing.ManifestDigest == journal.ManifestDigest {
			return nil
		}
		return contract.NewError(contract.ErrRevisionConflict, "journal_id", "create-once journal conflict")
	}
	b.journals[journal.JournalID] = cloneJournal(journal)
	return nil
}

func (b *Backend) CASJournal(_ context.Context, expected uint64, next contract.WriteJournal) error {
	if err := next.Validate(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	current, ok := b.journals[next.JournalID]
	if !ok {
		return contract.NewError(contract.ErrNotFound, "journal_id", "journal not found")
	}
	if current.Revision != expected || next.Revision != expected+1 {
		return contract.NewError(contract.ErrRevisionConflict, "journal_revision", "CAS mismatch")
	}
	if current.ObjectID != next.ObjectID || current.ManifestDigest != next.ManifestDigest || current.ObjectDigest != next.ObjectDigest {
		return contract.NewError(contract.ErrRevisionConflict, "journal_identity", "immutable identity changed")
	}
	if err := contract.AdvanceJournal(current.State, next.State); err != nil {
		return err
	}
	b.journals[next.JournalID] = cloneJournal(next)
	return nil
}

func (b *Backend) InspectJournal(_ context.Context, id string) (contract.WriteJournal, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	j, ok := b.journals[id]
	if !ok {
		return contract.WriteJournal{}, contract.NewError(contract.ErrNotFound, "journal_id", "journal not found")
	}
	return cloneJournal(j), nil
}

func cloneJournal(j contract.WriteJournal) contract.WriteJournal {
	j.ResidualRefs = append([]contract.ResidualRef{}, j.ResidualRefs...)
	return j
}

func (b *Backend) StageManifest(_ context.Context, manifest contract.ObjectManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existing, ok := b.objects[manifest.ObjectID]; ok {
		if existing.manifest.Digest == manifest.Digest {
			return nil
		}
		return contract.NewError(contract.ErrRevisionConflict, "object_id", "manifest changed")
	}
	b.objects[manifest.ObjectID] = objectEntry{manifest: cloneManifest(manifest)}
	return nil
}

func (b *Backend) CommitObjectReference(_ context.Context, objectID, digest string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	entry, ok := b.objects[objectID]
	if !ok {
		return contract.NewError(contract.ErrNotFound, "object_id", "manifest not staged")
	}
	if entry.manifest.ContentDigest != digest {
		return contract.NewError(contract.ErrContentDigestMismatch, "content_digest", "reference digest mismatch")
	}
	entry.committed = true
	b.objects[objectID] = entry
	return nil
}

func (b *Backend) SetObjectVisible(_ context.Context, objectID string, visible bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	entry, ok := b.objects[objectID]
	if !ok || !entry.committed {
		return contract.NewError(contract.ErrCrossStoreIndeterminate, "object_id", "reference is not committed")
	}
	entry.visible = visible
	b.objects[objectID] = entry
	return nil
}

func (b *Backend) InspectObject(_ context.Context, objectID string) (contract.ObjectManifest, bool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	entry, ok := b.objects[objectID]
	if !ok {
		return contract.ObjectManifest{}, false, contract.NewError(contract.ErrNotFound, "object_id", "object not found")
	}
	return cloneManifest(entry.manifest), entry.visible, nil
}

func cloneManifest(m contract.ObjectManifest) contract.ObjectManifest {
	m.Chunks = append([]contract.ChunkRef{}, m.Chunks...)
	return m
}

func (b *Backend) PutChunk(_ context.Context, ref contract.ChunkRef, data []byte) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if int64(len(data)) != ref.Length || contract.DigestBytes(data) != ref.Digest {
		return contract.NewError(contract.ErrContentDigestMismatch, "chunk", "length or digest mismatch")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existing, ok := b.chunks[ref.Digest]; ok {
		if contract.DigestBytes(existing) != ref.Digest {
			return contract.NewError(contract.ErrContentDigestMismatch, "chunk", "existing content is corrupt")
		}
		return nil
	}
	b.chunks[ref.Digest] = append([]byte{}, data...)
	return nil
}

func (b *Backend) GetChunk(_ context.Context, ref contract.ChunkRef) ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	data, ok := b.chunks[ref.Digest]
	if !ok {
		return nil, contract.NewError(contract.ErrNotFound, "chunk", "chunk not found")
	}
	if int64(len(data)) != ref.Length || contract.DigestBytes(data) != ref.Digest {
		return nil, contract.NewError(contract.ErrContentDigestMismatch, "chunk", "stored content is corrupt")
	}
	return append([]byte{}, data...), nil
}

func (b *Backend) HasChunk(_ context.Context, ref contract.ChunkRef) (bool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	data, ok := b.chunks[ref.Digest]
	if !ok {
		return false, nil
	}
	if int64(len(data)) != ref.Length || contract.DigestBytes(data) != ref.Digest {
		return false, contract.NewError(contract.ErrContentDigestMismatch, "chunk", "stored content is corrupt")
	}
	return true, nil
}

// CorruptChunkForTest is intentionally explicit and only exists on the
// reference backend so fault-injection tests can verify fail-closed reads.
func (b *Backend) CorruptChunkForTest(digest string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if data, ok := b.chunks[digest]; ok && len(data) > 0 {
		copy := append([]byte{}, data...)
		copy[0] ^= 0xff
		b.chunks[digest] = copy
	}
}

func (b *Backend) CreateRetention(_ context.Context, fact contract.RetentionFact) error {
	if err := fact.Validate(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.retentions[fact.ObjectID]; ok {
		return contract.NewError(contract.ErrRevisionConflict, "object_id", "retention fact already exists")
	}
	b.retentions[fact.ObjectID] = fact
	return nil
}

func (b *Backend) CASRetention(_ context.Context, expected uint64, next contract.RetentionFact) error {
	if err := next.Validate(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	current, ok := b.retentions[next.ObjectID]
	if !ok {
		return contract.NewError(contract.ErrNotFound, "object_id", "retention fact not found")
	}
	if current.Revision != expected || next.Revision != expected+1 {
		return contract.NewError(contract.ErrRevisionConflict, "retention_revision", "CAS mismatch")
	}
	b.retentions[next.ObjectID] = next
	return nil
}

func (b *Backend) InspectRetention(_ context.Context, objectID string) (contract.RetentionFact, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	fact, ok := b.retentions[objectID]
	if !ok {
		return contract.RetentionFact{}, contract.NewError(contract.ErrNotFound, "object_id", "retention fact not found")
	}
	return fact, nil
}

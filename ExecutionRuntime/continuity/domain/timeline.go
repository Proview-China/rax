package domain

import (
	"context"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

// ReferenceTimeline is the caller-candidate Wave 1 fixture. Production
// projection uses runtimeadapter.TimelineProjectionAdapterV1; this type must
// never be assembled as a production current writer.
type ReferenceTimeline struct {
	store     ports.TimelineProjectionStore
	clock     Clock
	cursorTTL time.Duration
}

func NewReferenceTimeline(store ports.TimelineProjectionStore, clock Clock, cursorTTL time.Duration) (*ReferenceTimeline, error) {
	if store == nil || clock == nil {
		return nil, contract.NewError(contract.ErrInvalidArgument, "timeline", "store and clock are required")
	}
	if cursorTTL <= 0 {
		return nil, contract.NewError(contract.ErrInvalidArgument, "cursor_ttl", "must be positive")
	}
	return &ReferenceTimeline{store: store, clock: clock, cursorTTL: cursorTTL}, nil
}

func (t *ReferenceTimeline) Project(ctx context.Context, candidate contract.TimelineProjectionCandidate) (contract.TimelineEventRecord, bool, error) {
	if err := candidate.Validate(); err != nil {
		return contract.TimelineEventRecord{}, false, err
	}
	record := recordFromCandidate(candidate)
	if err := record.Validate(); err != nil {
		return contract.TimelineEventRecord{}, false, err
	}
	return t.store.PutProjection(ctx, record)
}

func recordFromCandidate(candidate contract.TimelineProjectionCandidate) contract.TimelineEventRecord {
	return contract.TimelineEventRecord{
		Candidate:            candidate.Clone(),
		EvidenceRecordRef:    candidate.Evidence.RecordRef,
		LedgerScopeDigest:    candidate.Evidence.LedgerScopeDigest,
		LedgerSequence:       candidate.Evidence.LedgerSequence,
		EvidenceRecordDigest: candidate.Evidence.RecordDigest,
		TrustClass:           candidate.Evidence.TrustClass,
		ProjectionRevision:   candidate.Revision,
		Visibility:           "visible",
	}
}

func (t *ReferenceTimeline) Inspect(ctx context.Context, evidenceRef string) (contract.TimelineEventRecord, error) {
	if err := contract.ValidateToken("evidence_ref", evidenceRef); err != nil {
		return contract.TimelineEventRecord{}, err
	}
	return t.store.InspectByEvidence(ctx, evidenceRef)
}

func (t *ReferenceTimeline) Query(ctx context.Context, query contract.TimelineQuery) (contract.TimelinePage, error) {
	if err := query.Validate(); err != nil {
		return contract.TimelinePage{}, err
	}
	var after uint64
	if query.Cursor != "" {
		cursor, err := contract.DecodeTimelineCursor(query.Cursor)
		if err != nil {
			return contract.TimelinePage{}, err
		}
		if err := cursor.ValidateFor(query, t.clock.Now()); err != nil {
			return contract.TimelinePage{}, err
		}
		after = cursor.AfterSequence
	}
	records, err := t.store.ListLedgerScope(ctx, query.LedgerScopeDigest)
	if err != nil {
		return contract.TimelinePage{}, err
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].LedgerSequence == records[j].LedgerSequence {
			return records[i].EvidenceRecordRef < records[j].EvidenceRecordRef
		}
		return records[i].LedgerSequence < records[j].LedgerSequence
	})
	filtered := make([]contract.TimelineEventRecord, 0, query.PageLimit)
	remaining := false
	for _, record := range records {
		if record.LedgerSequence <= after || !contract.TimelineEventMatchesQuery(record, query) {
			continue
		}
		if len(filtered) == query.PageLimit {
			remaining = true
			break
		}
		filtered = append(filtered, record.Clone())
	}
	nextAfter := after
	if len(filtered) > 0 {
		nextAfter = filtered[len(filtered)-1].LedgerSequence
	}
	next, err := t.makeCursor(query, nextAfter)
	if err != nil {
		return contract.TimelinePage{}, err
	}
	return contract.TimelinePage{Records: filtered, NextCursor: next, Exhausted: !remaining}, nil
}

// Watch is a bounded polling watch. It never hides a missing ledger sequence:
// a consumer must rebuild/requery after an explicit watch_gap error.
func (t *ReferenceTimeline) Watch(ctx context.Context, query contract.TimelineQuery) (contract.TimelinePage, error) {
	if query.Cursor == "" {
		return t.Query(ctx, query)
	}
	cursor, err := contract.DecodeTimelineCursor(query.Cursor)
	if err != nil {
		return contract.TimelinePage{}, err
	}
	if err := cursor.ValidateFor(query, t.clock.Now()); err != nil {
		return contract.TimelinePage{}, err
	}
	all, err := t.store.ListLedgerScope(ctx, query.LedgerScopeDigest)
	if err != nil {
		return contract.TimelinePage{}, err
	}
	var first uint64
	for _, record := range all {
		if record.LedgerSequence > cursor.AfterSequence && (first == 0 || record.LedgerSequence < first) {
			first = record.LedgerSequence
		}
	}
	if cursor.AfterSequence > 0 && first > cursor.AfterSequence+1 {
		return contract.TimelinePage{}, contract.NewError(contract.ErrWatchGap, "ledger_sequence", "missing projected ledger sequence")
	}
	return t.Query(ctx, query)
}

func (t *ReferenceTimeline) Rebuild(ctx context.Context, ledgerScope string, candidates []contract.TimelineProjectionCandidate) error {
	if err := contract.ValidateToken("ledger_scope_digest", ledgerScope); err != nil {
		return err
	}
	records := make([]contract.TimelineEventRecord, 0, len(candidates))
	bySource := map[string]string{}
	byEvidence := map[string]string{}
	bySequence := map[uint64]string{}
	for _, candidate := range candidates {
		if err := candidate.Validate(); err != nil {
			return err
		}
		if candidate.Evidence.LedgerScopeDigest != ledgerScope {
			return contract.NewError(contract.ErrProjectionConflict, "ledger_scope_digest", "rebuild candidate belongs to another scope")
		}
		source := candidate.Evidence.SourceKey.String()
		if digest, ok := bySource[source]; ok && digest != candidate.Evidence.RecordDigest {
			return contract.NewError(contract.ErrEvidenceConflict, "source_key", "same source identity changed content")
		}
		bySource[source] = candidate.Evidence.RecordDigest
		if digest, ok := byEvidence[candidate.Evidence.RecordRef]; ok && digest != candidate.Digest {
			return contract.NewError(contract.ErrProjectionConflict, "evidence_ref", "same evidence changed projection")
		}
		byEvidence[candidate.Evidence.RecordRef] = candidate.Digest
		if digest, ok := bySequence[candidate.Evidence.LedgerSequence]; ok && digest != candidate.Evidence.RecordDigest {
			return contract.NewError(contract.ErrEvidenceConflict, "ledger_sequence", "sequence belongs to different evidence")
		}
		bySequence[candidate.Evidence.LedgerSequence] = candidate.Evidence.RecordDigest
		records = append(records, recordFromCandidate(candidate))
	}
	if err := validateAcyclic(records); err != nil {
		return err
	}
	for _, record := range records {
		if _, _, err := t.store.PutProjection(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (t *ReferenceTimeline) CreateTombstone(ctx context.Context, request contract.TimelineProjectionTombstoneRequestV1) (contract.TimelineProjectionTombstoneFactV1, bool, error) {
	if err := request.Validate(); err != nil {
		return contract.TimelineProjectionTombstoneFactV1{}, false, err
	}
	record, err := t.store.InspectByEvidence(ctx, request.EvidenceRecordRef)
	if err != nil {
		return contract.TimelineProjectionTombstoneFactV1{}, false, err
	}
	fact := contract.TimelineProjectionTombstoneFactV1{
		ContractVersion: contract.ContractVersion, TombstoneID: request.TombstoneID,
		EvidenceRecordRef: request.EvidenceRecordRef, SourceTombstoneRef: request.SourceTombstoneRef,
		PolicyBasisRef: request.PolicyBasisRef, IdempotencyKey: request.IdempotencyKey,
		ScopeDigest: record.Candidate.Scope.ExecutionScopeDigest, Revision: 1,
		CreatedUnixNano: t.clock.Now().UnixNano(),
	}
	fact.Digest, err = fact.CanonicalDigest()
	if err != nil {
		return contract.TimelineProjectionTombstoneFactV1{}, false, err
	}
	return t.store.CreateTombstoneOverlay(ctx, fact)
}

func (t *ReferenceTimeline) makeCursor(query contract.TimelineQuery, after uint64) (string, error) {
	digest, err := query.Digest()
	if err != nil {
		return "", err
	}
	now := t.clock.Now()
	return (contract.TimelineCursor{
		LedgerScopeDigest: query.LedgerScopeDigest, AfterSequence: after,
		QueryDigest: digest, AuthorityWatermark: query.AuthorityWatermark,
		PolicyWatermark: query.PolicyWatermark, ProjectionSchema: contract.ProjectionSchema,
		PageLimit: query.PageLimit, IssuedUnixNano: now.UnixNano(),
		ExpiresUnixNano: now.Add(t.cursorTTL).UnixNano(), State: "active",
	}).Encode()
}

func validateAcyclic(records []contract.TimelineEventRecord) error {
	graph := make(map[string][]string, len(records))
	for _, record := range records {
		graph[record.Candidate.CandidateID] = append([]string{}, record.Candidate.ParentRefs...)
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return false
		}
		if visited[id] {
			return true
		}
		visiting[id] = true
		for _, parent := range graph[id] {
			if _, exists := graph[parent]; exists && !visit(parent) {
				return false
			}
		}
		visiting[id] = false
		visited[id] = true
		return true
	}
	for id := range graph {
		if !visit(id) {
			return contract.NewError(contract.ErrProjectionConflict, "parent_refs", "cycle detected")
		}
	}
	return nil
}

package memory

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type timelineAttemptKeyV1 struct {
	ScopeDigest string
	AttemptID   string
}

type timelineIdempotencyKeyV1 struct {
	ScopeDigest    string
	IdempotencyKey string
}

type timelineEventKeyV1 struct {
	LedgerScopeDigest string
	EvidenceRecordRef string
}

func (b *Backend) CreateTimelineProjectionAttemptV1(_ context.Context, candidate contract.TimelineProjectionAttemptFactV1) (contract.TimelineProjectionAttemptFactV1, bool, error) {
	if err := candidate.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, false, err
	}
	if candidate.Ref.Revision != 1 || candidate.State != contract.TimelineAttemptProposedV1 {
		return contract.TimelineProjectionAttemptFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "attempt_create", "create requires proposed revision one")
	}
	key := timelineAttemptKeyV1{ScopeDigest: candidate.Ref.ScopeDigest, AttemptID: candidate.Ref.AttemptID}
	idempotency := timelineIdempotencyKeyV1{ScopeDigest: candidate.Ref.ScopeDigest, IdempotencyKey: candidate.Request.IdempotencyKey}
	b.mu.Lock()
	defer b.mu.Unlock()
	if bound, ok := b.timelineIdempotencyV1[idempotency]; ok && bound != key {
		return contract.TimelineProjectionAttemptFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "idempotency key belongs to another attempt")
	}
	if revision, ok := b.timelineAttemptCurrentV1[key]; ok {
		existing := b.timelineAttemptsV1[key][revision]
		if existing.Request.Digest == candidate.Request.Digest {
			return existing.Clone(), true, nil
		}
		return contract.TimelineProjectionAttemptFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "attempt_id", "create-once attempt changed request")
	}
	b.timelineAttemptsV1[key] = map[uint64]contract.TimelineProjectionAttemptFactV1{1: candidate.Clone()}
	b.timelineAttemptCurrentV1[key] = 1
	b.timelineIdempotencyV1[idempotency] = key
	return candidate.Clone(), false, nil
}

func (b *Backend) InspectTimelineProjectionAttemptV1(_ context.Context, ref contract.TimelineProjectionAttemptRefV1) (contract.TimelineProjectionAttemptFactV1, error) {
	if err := ref.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	key := timelineAttemptKeyV1{ScopeDigest: ref.ScopeDigest, AttemptID: ref.AttemptID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	fact, ok := b.timelineAttemptsV1[key][ref.Revision]
	if !ok {
		return contract.TimelineProjectionAttemptFactV1{}, contract.NewError(contract.ErrNotFound, "attempt_ref", "attempt revision not found")
	}
	if fact.Ref != ref {
		return contract.TimelineProjectionAttemptFactV1{}, contract.NewError(contract.ErrRevisionConflict, "attempt_ref", "attempt digest drifted")
	}
	return fact.Clone(), nil
}

func (b *Backend) InspectCurrentTimelineProjectionAttemptV1(_ context.Context, scopeDigest, attemptID string) (contract.TimelineProjectionAttemptFactV1, error) {
	if err := contract.ValidateToken("scope_digest", scopeDigest); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	if err := contract.ValidateToken("attempt_id", attemptID); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	key := timelineAttemptKeyV1{ScopeDigest: scopeDigest, AttemptID: attemptID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	revision, ok := b.timelineAttemptCurrentV1[key]
	if !ok {
		return contract.TimelineProjectionAttemptFactV1{}, contract.NewError(contract.ErrNotFound, "attempt_id", "attempt not found")
	}
	return b.timelineAttemptsV1[key][revision].Clone(), nil
}

func (b *Backend) CompareAndSwapTimelineProjectionAttemptV1(_ context.Context, expected contract.TimelineProjectionAttemptRefV1, next contract.TimelineProjectionAttemptFactV1) (contract.TimelineProjectionAttemptFactV1, error) {
	if err := expected.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	if err := next.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	key := timelineAttemptKeyV1{ScopeDigest: expected.ScopeDigest, AttemptID: expected.AttemptID}
	b.mu.Lock()
	defer b.mu.Unlock()
	current, err := b.currentTimelineAttemptLockedV1(key, expected)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	if err := validateTimelineAttemptSuccessorV1(current, next); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	if next.State == contract.TimelineAttemptVisibleV1 {
		return contract.TimelineProjectionAttemptFactV1{}, contract.NewError(contract.ErrUnsupported, "attempt_state", "visible transition requires atomic publish")
	}
	b.timelineAttemptsV1[key][next.Ref.Revision] = next.Clone()
	b.timelineAttemptCurrentV1[key] = next.Ref.Revision
	return next.Clone(), nil
}

func (b *Backend) PublishTimelineProjectionV1(_ context.Context, request ports.PublishTimelineProjectionV1Request) (contract.TimelineProjectionAttemptFactV1, contract.TimelineProjectionCurrentV1, error) {
	if err := request.ExpectedAttempt.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	if err := request.VisibleAttempt.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	if err := request.Event.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	if err := request.Current.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	key := timelineAttemptKeyV1{ScopeDigest: request.ExpectedAttempt.ScopeDigest, AttemptID: request.ExpectedAttempt.AttemptID}
	b.mu.Lock()
	defer b.mu.Unlock()
	currentAttempt, err := b.currentTimelineAttemptLockedV1(key, request.ExpectedAttempt)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	if err := validateTimelineAttemptSuccessorV1(currentAttempt, request.VisibleAttempt); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	if request.VisibleAttempt.State != contract.TimelineAttemptVisibleV1 || request.VisibleAttempt.Event == nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrRevisionConflict, "attempt_state", "atomic publish requires visible attempt")
	}
	eventRef := timelineEventRefV1(request.Event)
	if *request.VisibleAttempt.Event != eventRef || request.Current.Event != eventRef || request.Current.Attempt != request.VisibleAttempt.Ref {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrProjectionConflict, "atomic_publish", "event, attempt and current bindings differ")
	}
	if request.Current.EvidenceProjectionRef != request.VisibleAttempt.EvidenceProjectionRef ||
		request.Current.EvidenceProjectionDigest != request.VisibleAttempt.EvidenceProjectionDigest ||
		request.Current.EvidenceCurrentIndexRef != request.VisibleAttempt.EvidenceCurrentIndexRef ||
		request.Current.EvidenceCurrentIndexDigest != request.VisibleAttempt.EvidenceCurrentIndexDigest ||
		request.Current.OwnerProjectionDigest != request.VisibleAttempt.OwnerProjectionDigest ||
		request.Current.PolicyProjectionDigest != request.VisibleAttempt.PolicyProjectionDigest ||
		request.Current.CheckedUnixNano != request.VisibleAttempt.CheckedUnixNano ||
		request.Current.NotAfterUnixNano != request.VisibleAttempt.NotAfterUnixNano {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrProjectionConflict, "atomic_publish", "current projection differs from admitted attempt")
	}
	if err := b.validateProjectionPutLockedV1(request.Event); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	eventKey := timelineEventKeyV1{LedgerScopeDigest: eventRef.LedgerScopeDigest, EvidenceRecordRef: eventRef.EvidenceRecordRef}
	if existing, ok := b.timelineCurrentV1[eventKey]; ok && !reflect.DeepEqual(existing, request.Current) {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrRevisionConflict, "projection_current", "event already has another current projection")
	}
	b.putProjectionLockedV1(request.Event)
	b.timelineAttemptsV1[key][request.VisibleAttempt.Ref.Revision] = request.VisibleAttempt.Clone()
	b.timelineAttemptCurrentV1[key] = request.VisibleAttempt.Ref.Revision
	b.timelineCurrentV1[eventKey] = request.Current
	return request.VisibleAttempt.Clone(), request.Current, nil
}

func (b *Backend) InspectTimelineProjectionCurrentV1(_ context.Context, event contract.TimelineEventRefV1) (contract.TimelineProjectionCurrentV1, error) {
	if err := event.Validate(); err != nil {
		return contract.TimelineProjectionCurrentV1{}, err
	}
	key := timelineEventKeyV1{LedgerScopeDigest: event.LedgerScopeDigest, EvidenceRecordRef: event.EvidenceRecordRef}
	b.mu.RLock()
	defer b.mu.RUnlock()
	current, ok := b.timelineCurrentV1[key]
	if !ok {
		return contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrNotFound, "event_ref", "current projection not found")
	}
	if current.Event != event {
		return contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrRevisionConflict, "event_ref", "current projection belongs to another event revision")
	}
	return current, nil
}

func (b *Backend) currentTimelineAttemptLockedV1(key timelineAttemptKeyV1, expected contract.TimelineProjectionAttemptRefV1) (contract.TimelineProjectionAttemptFactV1, error) {
	revision, ok := b.timelineAttemptCurrentV1[key]
	if !ok {
		return contract.TimelineProjectionAttemptFactV1{}, contract.NewError(contract.ErrNotFound, "attempt_id", "attempt not found")
	}
	current := b.timelineAttemptsV1[key][revision]
	if current.Ref != expected {
		return contract.TimelineProjectionAttemptFactV1{}, contract.NewError(contract.ErrRevisionConflict, "attempt_ref", "CAS expected ref is stale")
	}
	return current, nil
}

func validateTimelineAttemptSuccessorV1(current, next contract.TimelineProjectionAttemptFactV1) error {
	if next.Ref.AttemptID != current.Ref.AttemptID || next.Ref.ScopeDigest != current.Ref.ScopeDigest || next.Ref.Revision != current.Ref.Revision+1 || next.Request.Digest != current.Request.Digest {
		return contract.NewError(contract.ErrRevisionConflict, "attempt_ref", "attempt identity, request or revision drifted")
	}
	return contract.AdvanceTimelineProjectionAttemptV1(current.State, next.State)
}

func timelineEventRefV1(event contract.TimelineEventRecord) contract.TimelineEventRefV1 {
	return contract.TimelineEventRefV1{
		EventID: event.Candidate.CandidateID, EvidenceRecordRef: event.EvidenceRecordRef,
		LedgerScopeDigest: event.LedgerScopeDigest, LedgerSequence: event.LedgerSequence,
		Digest: event.Candidate.Digest,
	}
}

func (b *Backend) validateProjectionPutLockedV1(record contract.TimelineEventRecord) error {
	if existing, ok := b.recordsByEvidence[record.EvidenceRecordRef]; ok {
		if !reflect.DeepEqual(existing, record) {
			return contract.NewError(contract.ErrProjectionConflict, "evidence_ref", "same evidence changed event")
		}
		return nil
	}
	sourceKey := scopedSource(record)
	if evidenceRef, ok := b.sourceToEvidence[sourceKey]; ok {
		existing := b.recordsByEvidence[evidenceRef]
		if !reflect.DeepEqual(existing, record) {
			return contract.NewError(contract.ErrEvidenceConflict, "source_key", "same source changed event")
		}
	}
	if evidenceRef, ok := b.sequenceToEvidence[record.LedgerScopeDigest][record.LedgerSequence]; ok {
		existing := b.recordsByEvidence[evidenceRef]
		if !reflect.DeepEqual(existing, record) {
			return contract.NewError(contract.ErrEvidenceConflict, "ledger_sequence", "sequence changed event")
		}
	}
	if projectionCycle(b.recordsByEvidence, record) {
		return contract.NewError(contract.ErrProjectionConflict, "parent_refs", "cycle detected")
	}
	return nil
}

func (b *Backend) putProjectionLockedV1(record contract.TimelineEventRecord) {
	if _, ok := b.recordsByEvidence[record.EvidenceRecordRef]; ok {
		return
	}
	b.recordsByEvidence[record.EvidenceRecordRef] = record.Clone()
	b.sourceToEvidence[scopedSource(record)] = record.EvidenceRecordRef
	sequences := b.sequenceToEvidence[record.LedgerScopeDigest]
	if sequences == nil {
		sequences = make(map[uint64]string)
		b.sequenceToEvidence[record.LedgerScopeDigest] = sequences
	}
	sequences[record.LedgerSequence] = record.EvidenceRecordRef
}

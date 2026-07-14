package fakes

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// EvidenceLedgerStoreV2 is one deterministic in-memory Fact Owner for tests.
// Its single lock models the required atomic source-cursor + ledger-record
// commit; it does not claim a production backend, durability or SLA.
type EvidenceLedgerStoreV2 struct {
	mu                     sync.Mutex
	clock                  func() time.Time
	sources                map[string]ports.EvidenceSourceRegistrationFactV2
	sourceEpochs           map[string]string
	records                map[core.Digest]map[uint64]ports.EvidenceLedgerRecordV2
	bySource               map[string]ports.EvidenceLedgerRecordV2
	byEvent                map[string]ports.EvidenceLedgerRecordV2
	lastSequence           map[core.Digest]uint64
	lastDigest             map[core.Digest]core.Digest
	tombstones             map[string]ports.EvidenceTombstoneFactV2
	loseNextCreateReply    bool
	loseNextCASReply       bool
	loseNextAppendReply    bool
	loseNextTombstoneReply bool
}

func NewEvidenceLedgerStoreV2(clock func() time.Time) *EvidenceLedgerStoreV2 {
	if clock == nil {
		clock = time.Now
	}
	return &EvidenceLedgerStoreV2{clock: clock, sources: map[string]ports.EvidenceSourceRegistrationFactV2{}, sourceEpochs: map[string]string{}, records: map[core.Digest]map[uint64]ports.EvidenceLedgerRecordV2{}, bySource: map[string]ports.EvidenceLedgerRecordV2{}, byEvent: map[string]ports.EvidenceLedgerRecordV2{}, lastSequence: map[core.Digest]uint64{}, lastDigest: map[core.Digest]core.Digest{}, tombstones: map[string]ports.EvidenceTombstoneFactV2{}}
}

func (s *EvidenceLedgerStoreV2) LoseNextCreateReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextCreateReply = true
}
func (s *EvidenceLedgerStoreV2) LoseNextCASReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextCASReply = true
}
func (s *EvidenceLedgerStoreV2) LoseNextAppendReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextAppendReply = true
}
func (s *EvidenceLedgerStoreV2) LoseNextTombstoneReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextTombstoneReply = true
}

func (s *EvidenceLedgerStoreV2) CreateSource(ctx context.Context, fact ports.EvidenceSourceRegistrationFactV2) (ports.EvidenceSourceRegistrationFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.EvidenceSourceRegistrationFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sources[fact.ID]; ok {
		left, _ := existing.DigestV2()
		right, _ := fact.DigestV2()
		if left == right {
			return cloneEvidenceV2(existing), nil
		}
		return ports.EvidenceSourceRegistrationFactV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "source id already binds different registration")
	}
	if err := control.ValidateNewEvidenceSourceV2(fact, s.clock()); err != nil {
		return ports.EvidenceSourceRegistrationFactV2{}, err
	}
	scopeDigest, _ := fact.LedgerScope.DigestV2()
	epochKey := string(scopeDigest) + "\x00" + string(fact.SourceID) + "\x00" + strconv.FormatUint(uint64(fact.SourceEpoch), 10)
	if existingID, exists := s.sourceEpochs[epochKey]; exists && existingID != fact.ID {
		return ports.EvidenceSourceRegistrationFactV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "ledger scope, source and epoch already have a registration")
	}
	s.sources[fact.ID] = cloneEvidenceV2(fact)
	s.sourceEpochs[epochKey] = fact.ID
	if s.loseNextCreateReply {
		s.loseNextCreateReply = false
		return ports.EvidenceSourceRegistrationFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected source create reply loss")
	}
	return cloneEvidenceV2(fact), nil
}

func (s *EvidenceLedgerStoreV2) InspectSource(ctx context.Context, id string) (ports.EvidenceSourceRegistrationFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.EvidenceSourceRegistrationFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.sources[id]
	if !ok {
		return ports.EvidenceSourceRegistrationFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "source registration not found")
	}
	return cloneEvidenceV2(fact), nil
}

func (s *EvidenceLedgerStoreV2) CompareAndSwapSource(ctx context.Context, request ports.EvidenceSourceCASRequestV2) (ports.EvidenceSourceRegistrationFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.EvidenceSourceRegistrationFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sources[request.Next.ID]
	if !ok {
		return ports.EvidenceSourceRegistrationFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "source registration not found")
	}
	if current.Revision != request.ExpectedRevision {
		left, _ := current.DigestV2()
		right, _ := request.Next.DigestV2()
		if left == right {
			return cloneEvidenceV2(current), nil
		}
		return ports.EvidenceSourceRegistrationFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "source CAS revision conflict")
	}
	if err := control.ValidateEvidenceSourceTransitionV2(current, request.Next, s.clock()); err != nil {
		return ports.EvidenceSourceRegistrationFactV2{}, err
	}
	s.sources[current.ID] = cloneEvidenceV2(request.Next)
	if s.loseNextCASReply {
		s.loseNextCASReply = false
		return ports.EvidenceSourceRegistrationFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected source CAS reply loss")
	}
	return cloneEvidenceV2(request.Next), nil
}

func (s *EvidenceLedgerStoreV2) Append(ctx context.Context, request ports.EvidenceAppendRequestV2) (ports.EvidenceLedgerRecordV2, error) {
	return s.append(ctx, request.Candidate, request.ExpectedSourceRevision, false)
}
func (s *EvidenceLedgerStoreV2) AppendLateObservation(ctx context.Context, request ports.EvidenceAppendLateRequestV2) (ports.EvidenceLedgerRecordV2, error) {
	return s.append(ctx, request.Candidate, request.ExpectedSourceRevision, true)
}

func (s *EvidenceLedgerStoreV2) append(ctx context.Context, candidate ports.EvidenceEventCandidateV2, expected core.Revision, late bool) (ports.EvidenceLedgerRecordV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	if err := candidate.Validate(); err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := evidenceSourceKeyV2(ports.EvidenceSourceKeyV2{RegistrationID: candidate.RegistrationID, SourceEpoch: candidate.SourceEpoch, SourceSequence: candidate.SourceSequence})
	if existing, ok := s.bySource[key]; ok {
		digest, _ := candidate.DigestV2()
		if digest == existing.CandidateDigest {
			return cloneEvidenceV2(existing), nil
		}
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "same source sequence changed content")
	}
	source, ok := s.sources[candidate.RegistrationID]
	if !ok {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "source registration not found")
	}
	now := s.clock()
	var err error
	if late {
		err = control.ValidateEvidenceLateAppendV2(source, ports.EvidenceAppendLateRequestV2{Candidate: candidate, ExpectedSourceRevision: expected}, now)
	} else {
		err = control.ValidateEvidenceAppendV2(source, ports.EvidenceAppendRequestV2{Candidate: candidate, ExpectedSourceRevision: expected}, now)
	}
	if err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	scopeDigest, _ := candidate.LedgerScope.DigestV2()
	eventKey := string(scopeDigest) + "\x00" + candidate.EventID
	if existing, ok := s.byEvent[eventKey]; ok {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "event id already binds another source record: "+string(existing.Ref.RecordDigest))
	}
	sequence := s.lastSequence[scopeDigest] + 1
	previous := s.lastDigest[scopeDigest]
	if sequence == 1 {
		previous = ports.EvidenceGenesisDigestV2
	}
	record, err := control.NewEvidenceLedgerRecordV2(candidate, sequence, previous, now)
	if err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	// One critical section is the fake's transaction boundary: record, source
	// key, chain head, and source cursor become visible together.
	if s.records[scopeDigest] == nil {
		s.records[scopeDigest] = map[uint64]ports.EvidenceLedgerRecordV2{}
	}
	s.records[scopeDigest][sequence] = cloneEvidenceV2(record)
	s.bySource[key] = cloneEvidenceV2(record)
	s.byEvent[eventKey] = cloneEvidenceV2(record)
	s.lastSequence[scopeDigest] = sequence
	s.lastDigest[scopeDigest] = record.Ref.RecordDigest
	source.Revision++
	source.NextSourceSequence++
	source.UpdatedUnixNano = now.UnixNano()
	s.sources[source.ID] = cloneEvidenceV2(source)
	if s.loseNextAppendReply {
		s.loseNextAppendReply = false
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected evidence append reply loss")
	}
	return cloneEvidenceV2(record), nil
}

func (s *EvidenceLedgerStoreV2) InspectBySource(ctx context.Context, key ports.EvidenceSourceKeyV2) (ports.EvidenceLedgerRecordV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	if err := key.Validate(); err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.bySource[evidenceSourceKeyV2(key)]
	if !ok {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "source record not found")
	}
	return cloneEvidenceV2(record), nil
}
func (s *EvidenceLedgerStoreV2) InspectRecord(ctx context.Context, ref ports.EvidenceRecordRefV2) (ports.EvidenceLedgerRecordV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[ref.LedgerScopeDigest][ref.Sequence]
	if !ok {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "ledger record not found")
	}
	if record.Ref.RecordDigest != ref.RecordDigest {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "record ref digest conflicts with ledger")
	}
	return cloneEvidenceV2(record), nil
}

func (s *EvidenceLedgerStoreV2) Watch(ctx context.Context, cursor ports.EvidenceWatchCursorV2, limit uint32) (ports.EvidencePageV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.EvidencePageV2{}, err
	}
	if cursor.LedgerScopeDigest.Validate() != nil || limit == 0 || limit > ports.MaxEvidencePageSize {
		return ports.EvidencePageV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceCursorInvalid, "watch cursor and bounded page limit are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.records[cursor.LedgerScopeDigest]
	sequences := make([]uint64, 0, len(items))
	for sequence := range items {
		if sequence > cursor.AfterSequence {
			sequences = append(sequences, sequence)
		}
	}
	sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })
	if len(sequences) > int(limit) {
		sequences = sequences[:limit]
	}
	page := ports.EvidencePageV2{Records: []ports.EvidenceLedgerRecordV2{}, Next: cursor}
	for _, sequence := range sequences {
		page.Records = append(page.Records, cloneEvidenceV2(items[sequence]))
		page.Next.AfterSequence = sequence
	}
	return page, nil
}

func (s *EvidenceLedgerStoreV2) CreateTombstone(ctx context.Context, fact ports.EvidenceTombstoneFactV2) (ports.EvidenceTombstoneFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.EvidenceTombstoneFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(fact.Record.LedgerScopeDigest) + "\x00" + strconv.FormatUint(fact.Record.Sequence, 10)
	if existing, exists := s.tombstones[key]; exists {
		left, _ := existing.DigestV2()
		right, _ := fact.DigestV2()
		if left == right {
			return cloneEvidenceV2(existing), nil
		}
		return ports.EvidenceTombstoneFactV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "record already has a different tombstone")
	}
	if err := fact.Validate(); err != nil {
		return ports.EvidenceTombstoneFactV2{}, err
	}
	if fact.CreatedUnixNano > s.clock().UnixNano() {
		return ports.EvidenceTombstoneFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "tombstone cannot be created in the future")
	}
	record, ok := s.records[fact.Record.LedgerScopeDigest][fact.Record.Sequence]
	if !ok || record.Ref.RecordDigest != fact.Record.RecordDigest {
		return ports.EvidenceTombstoneFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "tombstone target record not found")
	}
	recordSource := ports.EvidenceSourceKeyV2{RegistrationID: record.Candidate.RegistrationID, SourceEpoch: record.Candidate.SourceEpoch, SourceSequence: record.Candidate.SourceSequence}
	if evidenceSourceKeyV2(fact.Source) != evidenceSourceKeyV2(recordSource) {
		return ports.EvidenceTombstoneFactV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "tombstone source key does not match immutable record")
	}
	s.tombstones[key] = cloneEvidenceV2(fact)
	if s.loseNextTombstoneReply {
		s.loseNextTombstoneReply = false
		return ports.EvidenceTombstoneFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected tombstone reply loss")
	}
	return cloneEvidenceV2(fact), nil
}

func (s *EvidenceLedgerStoreV2) InspectTombstone(ctx context.Context, ref ports.EvidenceRecordRefV2) (ports.EvidenceTombstoneFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.EvidenceTombstoneFactV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.EvidenceTombstoneFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(ref.LedgerScopeDigest) + "\x00" + strconv.FormatUint(ref.Sequence, 10)
	fact, ok := s.tombstones[key]
	if !ok {
		return ports.EvidenceTombstoneFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "tombstone not found")
	}
	if fact.Record.RecordDigest != ref.RecordDigest {
		return ports.EvidenceTombstoneFactV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "tombstone record digest conflicts")
	}
	return cloneEvidenceV2(fact), nil
}

func evidenceSourceKeyV2(key ports.EvidenceSourceKeyV2) string {
	return key.RegistrationID + "\x00" + strconv.FormatUint(uint64(key.SourceEpoch), 10) + "\x00" + strconv.FormatUint(key.SourceSequence, 10)
}
func cloneEvidenceV2[T any](value T) T {
	payload, _ := json.Marshal(value)
	var cloned T
	_ = json.Unmarshal(payload, &cloned)
	return cloned
}

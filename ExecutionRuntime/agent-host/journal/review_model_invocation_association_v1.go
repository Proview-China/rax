package journal

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

// MemoryReviewModelInvocationAssociationStoreV1 is reference/test-only.
type MemoryReviewModelInvocationAssociationStoreV1 struct {
	mu                  sync.RWMutex
	clock               func() time.Time
	history             map[string][]byte
	current             map[string]contract.ReviewModelInvocationAssociationRefV1
	highest             map[string]uint64
	loseCreate, loseCAS bool
	failCreate, failCAS contract.ErrorCode
}

func NewMemoryReviewModelInvocationAssociationStoreV1(clock func() time.Time) *MemoryReviewModelInvocationAssociationStoreV1 {
	if clock == nil {
		clock = time.Now
	}
	return &MemoryReviewModelInvocationAssociationStoreV1{clock: clock, history: map[string][]byte{}, current: map[string]contract.ReviewModelInvocationAssociationRefV1{}, highest: map[string]uint64{}}
}
func (s *MemoryReviewModelInvocationAssociationStoreV1) LoseNextCreateReplyV1() {
	s.mu.Lock()
	s.loseCreate = true
	s.mu.Unlock()
}
func (s *MemoryReviewModelInvocationAssociationStoreV1) LoseNextCASReplyV1() {
	s.mu.Lock()
	s.loseCAS = true
	s.mu.Unlock()
}
func (s *MemoryReviewModelInvocationAssociationStoreV1) FailNextCreateBeforeCommitV1(code contract.ErrorCode) {
	s.mu.Lock()
	s.failCreate = code
	s.mu.Unlock()
}
func (s *MemoryReviewModelInvocationAssociationStoreV1) FailNextCASBeforeCommitV1(code contract.ErrorCode) {
	s.mu.Lock()
	s.failCAS = code
	s.mu.Unlock()
}

func (s *MemoryReviewModelInvocationAssociationStoreV1) CreateReviewModelInvocationAssociationV1(ctx context.Context, value contract.ReviewModelInvocationAssociationFactV1) (hostports.ReviewModelInvocationAssociationCreateReceiptV1, error) {
	if err := associationMutationContext(ctx); err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, err
	}
	baseline := s.clock()
	if baseline.IsZero() {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "association Create baseline clock is unavailable")
	}
	if err := value.ValidateCurrentV1(baseline); err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, err
	}
	if value.Revision != 1 || value.State != contract.ReviewModelInvocationAssociationActiveV1 {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, associationConflict("association create requires revision-one active")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, associationConflict("association encode failed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	actual := s.clock()
	if actual.IsZero() || actual.Before(baseline) {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "association Create clock regressed")
	}
	if err = value.ValidateCurrentV1(actual); err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, err
	}
	if s.failCreate != "" {
		code := s.failCreate
		s.failCreate = ""
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, associationFault(code, "association create failed before commit")
	}
	if ref, ok := s.current[value.ID]; ok {
		if ref.Subject != value.Subject {
			return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, associationConflict("association current subject drifted")
		}
		current, decodeErr := decodeAssociation(s.history[associationHistoryKey(ref)])
		if decodeErr != nil {
			return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, decodeErr
		}
		if current.RefV1() != ref {
			return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, associationConflict("association current row and payload drifted")
		}
		existing, decodeErr := decodeAssociation(s.history[associationHistoryKey(value.RefV1())])
		if decodeErr != nil {
			return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, decodeErr
		}
		if reflect.DeepEqual(existing, value) {
			return hostports.ReviewModelInvocationAssociationCreateReceiptV1{Fact: existing, Created: false}, nil
		}
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, associationConflict("association subject already binds different initial content")
	}
	if s.highest[value.ID] != 0 {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, associationConflict("association history exists without current")
	}
	s.history[associationHistoryKey(value.RefV1())] = append([]byte(nil), payload...)
	s.current[value.ID] = value.RefV1()
	s.highest[value.ID] = uint64(value.Revision)
	if s.loseCreate {
		s.loseCreate = false
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, contract.NewError(contract.ErrorUnknownOutcome, "reply_lost", "association create committed but reply was lost")
	}
	clone, _ := decodeAssociation(payload)
	return hostports.ReviewModelInvocationAssociationCreateReceiptV1{Fact: clone, Created: true}, nil
}

func (s *MemoryReviewModelInvocationAssociationStoreV1) ResolveCurrentReviewModelInvocationAssociationV1(ctx context.Context, subject contract.ReviewModelInvocationAssociationSubjectV1) (contract.ReviewModelInvocationAssociationRefV1, error) {
	if err := associationReadContext(ctx); err != nil {
		return contract.ReviewModelInvocationAssociationRefV1{}, err
	}
	id, err := subject.StableIDV1()
	if err != nil {
		return contract.ReviewModelInvocationAssociationRefV1{}, err
	}
	baseline := s.clock()
	s.mu.RLock()
	ref, ok := s.current[id]
	if !ok {
		s.mu.RUnlock()
		return contract.ReviewModelInvocationAssociationRefV1{}, contract.NewError(contract.ErrorNotFound, "association_missing", "association current is absent")
	}
	if ref.Subject != subject || s.highest[id] != uint64(ref.Revision) {
		s.mu.RUnlock()
		return contract.ReviewModelInvocationAssociationRefV1{}, associationConflict("association current index drifted")
	}
	value, err := decodeAssociation(s.history[associationHistoryKey(ref)])
	s.mu.RUnlock()
	if err != nil {
		return contract.ReviewModelInvocationAssociationRefV1{}, err
	}
	if value.RefV1() != ref {
		return contract.ReviewModelInvocationAssociationRefV1{}, associationConflict("association current row and payload drifted")
	}
	now := s.clock()
	if baseline.IsZero() || now.IsZero() || now.Before(baseline) {
		return contract.ReviewModelInvocationAssociationRefV1{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "association Resolve clock regressed")
	}
	if err = value.ValidateCurrentV1(now); err != nil {
		return contract.ReviewModelInvocationAssociationRefV1{}, err
	}
	return ref, nil
}

func (s *MemoryReviewModelInvocationAssociationStoreV1) InspectCurrentReviewModelInvocationAssociationV1(ctx context.Context, subject contract.ReviewModelInvocationAssociationSubjectV1, expected contract.ReviewModelInvocationAssociationRefV1) (contract.ReviewModelInvocationAssociationFactV1, error) {
	if err := associationReadContext(ctx); err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, err
	}
	baseline := s.clock()
	s.mu.RLock()
	current, ok := s.current[expected.ID]
	if !ok {
		s.mu.RUnlock()
		return contract.ReviewModelInvocationAssociationFactV1{}, contract.NewError(contract.ErrorNotFound, "association_missing", "association current is absent")
	}
	if current != expected || current.Subject != subject || s.highest[current.ID] != uint64(current.Revision) {
		s.mu.RUnlock()
		return contract.ReviewModelInvocationAssociationFactV1{}, associationConflict("association current full Ref changed")
	}
	value, err := decodeAssociation(s.history[associationHistoryKey(current)])
	s.mu.RUnlock()
	if err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, err
	}
	if value.RefV1() != current {
		return contract.ReviewModelInvocationAssociationFactV1{}, associationConflict("association current row and payload drifted")
	}
	now := s.clock()
	if baseline.IsZero() || now.IsZero() || now.Before(baseline) {
		return contract.ReviewModelInvocationAssociationFactV1{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "association Inspect clock regressed")
	}
	if err = value.ValidateCurrentV1(now); err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, err
	}
	return value, nil
}

func (s *MemoryReviewModelInvocationAssociationStoreV1) InspectHistoricalReviewModelInvocationAssociationV1(ctx context.Context, ref contract.ReviewModelInvocationAssociationRefV1) (contract.ReviewModelInvocationAssociationFactV1, error) {
	if err := associationReadContext(ctx); err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, err
	}
	s.mu.RLock()
	payload, ok := s.history[associationHistoryKey(ref)]
	s.mu.RUnlock()
	if !ok {
		return contract.ReviewModelInvocationAssociationFactV1{}, contract.NewError(contract.ErrorNotFound, "association_history_missing", "association historical Fact is absent")
	}
	value, err := decodeAssociation(payload)
	if err != nil {
		return value, err
	}
	if value.RefV1() != ref {
		return contract.ReviewModelInvocationAssociationFactV1{}, associationConflict("association historical exact Ref drifted")
	}
	return value, nil
}

func (s *MemoryReviewModelInvocationAssociationStoreV1) CompareAndSwapReviewModelInvocationAssociationV1(ctx context.Context, request hostports.ReviewModelInvocationAssociationCASRequestV1) (hostports.ReviewModelInvocationAssociationCASReceiptV1, error) {
	if err := associationMutationContext(ctx); err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, err
	}
	payload, err := json.Marshal(request.Next)
	if err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, associationConflict("association encode failed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failCAS != "" {
		code := s.failCAS
		s.failCAS = ""
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, associationFault(code, "association CAS failed before commit")
	}
	current, ok := s.current[request.Expected.ID]
	if !ok {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, contract.NewError(contract.ErrorNotFound, "association_missing", "association current is absent")
	}
	existing, err := decodeAssociation(s.history[associationHistoryKey(current)])
	if err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, err
	}
	if existing.RefV1() != current {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, associationConflict("association current row and payload drifted")
	}
	if current == request.Next.RefV1() && reflect.DeepEqual(existing, request.Next) {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{Fact: existing, Applied: false}, nil
	}
	if current != request.Expected || s.highest[current.ID] != uint64(current.Revision) {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, associationConflict("association CAS predecessor changed")
	}
	if err = contract.ValidateReviewModelInvocationAssociationTransitionV1(existing, request.Next); err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, err
	}
	key := associationHistoryKey(request.Next.RefV1())
	if old, exists := s.history[key]; exists && string(old) != string(payload) {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, associationConflict("association successor changed content")
	}
	s.history[key] = append([]byte(nil), payload...)
	s.current[current.ID] = request.Next.RefV1()
	s.highest[current.ID] = uint64(request.Next.Revision)
	if s.loseCAS {
		s.loseCAS = false
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, contract.NewError(contract.ErrorUnknownOutcome, "reply_lost", "association CAS committed but reply was lost")
	}
	clone, _ := decodeAssociation(payload)
	return hostports.ReviewModelInvocationAssociationCASReceiptV1{Fact: clone, Applied: true}, nil
}

func decodeAssociation(payload []byte) (contract.ReviewModelInvocationAssociationFactV1, error) {
	var value contract.ReviewModelInvocationAssociationFactV1
	if len(payload) == 0 || json.Unmarshal(payload, &value) != nil || value.ValidateHistoricalV1() != nil {
		return value, associationConflict("association stored payload drifted")
	}
	return value, nil
}
func associationHistoryKey(ref contract.ReviewModelInvocationAssociationRefV1) string {
	return ref.ID + "\x00" + strconv.FormatUint(uint64(ref.Revision), 10) + "\x00" + string(ref.Digest)
}
func associationReadContext(ctx context.Context) error {
	if ctx == nil {
		return contract.NewError(contract.ErrorInvalidArgument, "context_missing", "association read context is required")
	}
	if ctx.Err() != nil {
		return contract.NewError(contract.ErrorUnavailable, "context_ended", "association read context ended")
	}
	return nil
}
func associationMutationContext(ctx context.Context) error {
	if ctx == nil {
		return contract.NewError(contract.ErrorInvalidArgument, "context_missing", "association mutation context is required")
	}
	if ctx.Err() != nil {
		return contract.NewError(contract.ErrorUnknownOutcome, "context_ended", "association mutation outcome is unknown")
	}
	return nil
}
func associationConflict(message string) error {
	return contract.NewError(contract.ErrorConflict, "association_conflict", message)
}
func associationFault(code contract.ErrorCode, message string) error {
	if code == contract.ErrorUnavailable || code == contract.ErrorConflict {
		return contract.NewError(code, "injected_failure", message)
	}
	return contract.NewError(contract.ErrorUnknownOutcome, "injected_failure", message)
}

var _ hostports.ReviewModelInvocationAssociationPortV1 = (*MemoryReviewModelInvocationAssociationStoreV1)(nil)

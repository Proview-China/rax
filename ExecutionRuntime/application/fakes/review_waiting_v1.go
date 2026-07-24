// Package fakes provides deterministic Application test fixtures. They are not
// production persistence, Review adapters, composition roots or SLA claims.
package fakes

import (
	"context"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ReviewWaitingStoreV1 struct {
	mu               sync.RWMutex
	clock            func() time.Time
	history          map[string]contract.ReviewWaitingCoordinationFactV1
	current          map[string]contract.ReviewWaitingCoordinationRefV1
	highest          map[string]core.Revision
	loseCreateReply  bool
	loseCASReply     bool
	failCreateBefore core.ErrorCategory
	failCASBefore    core.ErrorCategory
}

func NewReviewWaitingStoreV1(clock func() time.Time) *ReviewWaitingStoreV1 {
	if clock == nil {
		clock = time.Now
	}
	return &ReviewWaitingStoreV1{clock: clock, history: make(map[string]contract.ReviewWaitingCoordinationFactV1), current: make(map[string]contract.ReviewWaitingCoordinationRefV1), highest: make(map[string]core.Revision)}
}

func (s *ReviewWaitingStoreV1) LoseNextCreateReplyV1() {
	s.mu.Lock()
	s.loseCreateReply = true
	s.mu.Unlock()
}
func (s *ReviewWaitingStoreV1) LoseNextCASReplyV1() {
	s.mu.Lock()
	s.loseCASReply = true
	s.mu.Unlock()
}
func (s *ReviewWaitingStoreV1) FailNextCreateBeforeCommitV1(category core.ErrorCategory) {
	s.mu.Lock()
	s.failCreateBefore = category
	s.mu.Unlock()
}
func (s *ReviewWaitingStoreV1) FailNextCASBeforeCommitV1(category core.ErrorCategory) {
	s.mu.Lock()
	s.failCASBefore = category
	s.mu.Unlock()
}

func (s *ReviewWaitingStoreV1) CreateReviewWaitingCoordinationV1(ctx context.Context, value contract.ReviewWaitingCoordinationFactV1) (applicationports.ReviewWaitingCoordinationCreateReceiptV1, error) {
	if err := reviewWaitingMutationContextV1(ctx); err != nil {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, err
	}
	if err := value.Validate(); err != nil {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, err
	}
	if value.Revision != 1 || value.State != contract.ReviewWaitingStateV1 {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, reviewWaitingFakeConflictV1("Review waiting create requires revision-one waiting_review")
	}
	if err := value.Request.ValidateCurrent(s.clock()); err != nil {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, err
	}
	value = value.Clone()
	currentKey, err := reviewWaitingCurrentKeyV1(value.Request.ExecutionScope, value.ID)
	if err != nil {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if category := s.failCreateBefore; category != "" {
		s.failCreateBefore = ""
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, reviewWaitingFakeFaultV1(category, "Review waiting create failed before commit")
	}
	if current, ok := s.current[currentKey]; ok {
		existing, exists := s.history[reviewWaitingHistoryKeyV1(currentKey, current)]
		if !exists || s.highest[currentKey] != current.Revision {
			return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, reviewWaitingFakeConflictV1("Review waiting current/history drifted")
		}
		if reflect.DeepEqual(existing, value) {
			return applicationports.ReviewWaitingCoordinationCreateReceiptV1{Fact: existing.Clone(), Created: false}, nil
		}
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, reviewWaitingFakeConflictV1("Review waiting ID already binds different content")
	}
	ref := value.RefV1()
	s.history[reviewWaitingHistoryKeyV1(currentKey, ref)] = value.Clone()
	s.current[currentKey] = ref
	s.highest[currentKey] = ref.Revision
	lose := s.loseCreateReply
	s.loseCreateReply = false
	if lose {
		return applicationports.ReviewWaitingCoordinationCreateReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Review waiting create committed but reply was lost")
	}
	return applicationports.ReviewWaitingCoordinationCreateReceiptV1{Fact: value.Clone(), Created: true}, nil
}

func (s *ReviewWaitingStoreV1) InspectCurrentReviewWaitingCoordinationV1(ctx context.Context, scope core.ExecutionScope, id string) (contract.ReviewWaitingCoordinationFactV1, error) {
	if err := reviewWaitingReadContextV1(ctx); err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, err
	}
	currentKey, err := reviewWaitingCurrentKeyV1(scope, id)
	if err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ref, ok := s.current[currentKey]
	if !ok {
		return contract.ReviewWaitingCoordinationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Review waiting coordination is absent")
	}
	if s.highest[currentKey] != ref.Revision {
		return contract.ReviewWaitingCoordinationFactV1{}, reviewWaitingFakeConflictV1("Review waiting current/highest drifted")
	}
	value, ok := s.history[reviewWaitingHistoryKeyV1(currentKey, ref)]
	if !ok || value.RefV1() != ref {
		return contract.ReviewWaitingCoordinationFactV1{}, reviewWaitingFakeConflictV1("Review waiting current history is incomplete")
	}
	return value.Clone(), nil
}

func (s *ReviewWaitingStoreV1) InspectHistoricalReviewWaitingCoordinationV1(ctx context.Context, scope core.ExecutionScope, ref contract.ReviewWaitingCoordinationRefV1) (contract.ReviewWaitingCoordinationFactV1, error) {
	if err := reviewWaitingReadContextV1(ctx); err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, err
	}
	currentKey, err := reviewWaitingCurrentKeyV1(scope, ref.ID)
	if err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.history[reviewWaitingHistoryKeyV1(currentKey, ref)]
	if !ok || value.RefV1() != ref {
		return contract.ReviewWaitingCoordinationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Review waiting historical coordination is absent")
	}
	return value.Clone(), nil
}

func (s *ReviewWaitingStoreV1) CompareAndSwapReviewWaitingCoordinationV1(ctx context.Context, request applicationports.ReviewWaitingCoordinationCASRequestV1) (applicationports.ReviewWaitingCoordinationCASReceiptV1, error) {
	if err := reviewWaitingMutationContextV1(ctx); err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, err
	}
	if err := request.Next.Request.ValidateCurrent(s.clock()); err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, err
	}
	next := request.Next.Clone()
	currentKey, err := reviewWaitingCurrentKeyV1(request.Scope, next.ID)
	if err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if category := s.failCASBefore; category != "" {
		s.failCASBefore = ""
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, reviewWaitingFakeFaultV1(category, "Review waiting CAS failed before commit")
	}
	currentRef, ok := s.current[currentKey]
	if !ok {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Review waiting coordination is absent")
	}
	if s.highest[currentKey] != currentRef.Revision {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, reviewWaitingFakeConflictV1("Review waiting CAS highest/current drifted")
	}
	current, ok := s.history[reviewWaitingHistoryKeyV1(currentKey, currentRef)]
	if !ok {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, reviewWaitingFakeConflictV1("Review waiting CAS predecessor history is absent")
	}
	if currentRef == next.RefV1() && reflect.DeepEqual(current, next) {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{Fact: current.Clone(), Applied: false}, nil
	}
	if currentRef != request.Expected {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, reviewWaitingFakeConflictV1("Review waiting CAS predecessor changed")
	}
	if err := contract.ValidateReviewWaitingCoordinationTransitionV1(current, next); err != nil {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, err
	}
	key := reviewWaitingHistoryKeyV1(currentKey, next.RefV1())
	if existing, exists := s.history[key]; exists && !reflect.DeepEqual(existing, next) {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, reviewWaitingFakeConflictV1("Review waiting exact successor changed content")
	}
	s.history[key] = next.Clone()
	s.current[currentKey] = next.RefV1()
	s.highest[currentKey] = next.Revision
	lose := s.loseCASReply
	s.loseCASReply = false
	if lose {
		return applicationports.ReviewWaitingCoordinationCASReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Review waiting CAS committed but reply was lost")
	}
	return applicationports.ReviewWaitingCoordinationCASReceiptV1{Fact: next.Clone(), Applied: true}, nil
}

func reviewWaitingCurrentKeyV1(scope core.ExecutionScope, id string) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	if id == "" {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review waiting ID is required")
	}
	digest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		return "", err
	}
	return string(scope.Identity.TenantID) + "\x00" + string(digest) + "\x00" + id, nil
}

func reviewWaitingHistoryKeyV1(currentKey string, ref contract.ReviewWaitingCoordinationRefV1) string {
	return currentKey + "\x00" + strconv.FormatUint(uint64(ref.Revision), 10) + "\x00" + string(ref.Digest)
}

func reviewWaitingReadContextV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review waiting read context is required")
	}
	if ctx.Err() != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Review waiting read context ended")
	}
	return nil
}

func reviewWaitingMutationContextV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review waiting mutation context is required")
	}
	if ctx.Err() != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Review waiting mutation outcome is unknown")
	}
	return nil
}

func reviewWaitingFakeConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, message)
}

func reviewWaitingFakeFaultV1(category core.ErrorCategory, message string) error {
	switch category {
	case core.ErrorConflict:
		return core.NewError(category, core.ReasonRevisionConflict, message)
	case core.ErrorUnavailable:
		return core.NewError(category, core.ReasonEvidenceUnavailable, message)
	default:
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, message)
	}
}

var _ applicationports.ReviewWaitingCoordinationFactPortV1 = (*ReviewWaitingStoreV1)(nil)

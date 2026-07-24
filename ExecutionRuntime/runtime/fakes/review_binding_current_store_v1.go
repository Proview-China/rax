package fakes

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ReviewBindingCurrentStoreV1 is an in-memory reference transaction owner. It
// serializes Binding facts, Consumer associations, immutable projection
// history, highest revisions, current full Refs and publish receipts under one
// lock. It is a test/conformance fake, not a production persistence backend.
type ReviewBindingCurrentStoreV1 struct {
	mu    sync.Mutex
	clock func() time.Time
	inner *BindingStoreV2

	associationHistory    map[string]map[core.Revision]ports.ReviewBindingConsumerAssociationCurrentProjectionV1
	associationCurrent    map[string]ports.ReviewBindingConsumerAssociationRefV1
	projectionHistory     map[string]map[core.Revision]ports.ReviewBindingCurrentProjectionV1
	projectionCurrent     map[string]ports.ReviewBindingProjectionRefV1
	highestRevision       map[string]core.Revision
	receipts              map[string]ports.ReviewBindingProjectionPublishReceiptV1
	loseNextReply         bool
	failNextCompoundStage bool
}

func NewReviewBindingCurrentStoreV1(clock func() time.Time) *ReviewBindingCurrentStoreV1 {
	if clock == nil {
		clock = time.Now
	}
	return &ReviewBindingCurrentStoreV1{
		clock: clock, inner: NewBindingStoreV2(clock),
		associationHistory: make(map[string]map[core.Revision]ports.ReviewBindingConsumerAssociationCurrentProjectionV1),
		associationCurrent: make(map[string]ports.ReviewBindingConsumerAssociationRefV1),
		projectionHistory:  make(map[string]map[core.Revision]ports.ReviewBindingCurrentProjectionV1),
		projectionCurrent:  make(map[string]ports.ReviewBindingProjectionRefV1),
		highestRevision:    make(map[string]core.Revision), receipts: make(map[string]ports.ReviewBindingProjectionPublishReceiptV1),
	}
}

func (s *ReviewBindingCurrentStoreV1) LoseNextReviewBindingPublishReplyV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextReply = true
}

func (s *ReviewBindingCurrentStoreV1) FailNextReviewBindingCompoundStageV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNextCompoundStage = true
}

// CreateReviewBindingConsumerAssociationV1 is an Owner-only fixture mutation.
// Production composition never receives this method.
func (s *ReviewBindingCurrentStoreV1) CreateReviewBindingConsumerAssociationV1(ctx context.Context, projection ports.ReviewBindingConsumerAssociationCurrentProjectionV1) (ports.ReviewBindingConsumerAssociationCurrentProjectionV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if err := projection.Validate(); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.associationHistory[projection.Ref.ID]; exists {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "Review Binding Consumer association already exists")
	}
	if projection.Ref.Revision != 1 || !projection.Current {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "Review Binding Consumer association must start current at revision one")
	}
	now := s.clock()
	if now.IsZero() {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Binding Consumer association clock is unavailable")
	}
	if err := projection.ValidateCurrent(projection.Ref, projection.Consumer, projection.Source, now); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if _, err := s.readAuthoritativeClosureLockedV1(ctx, projection.Source, projection, now); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	s.associationHistory[projection.Ref.ID] = map[core.Revision]ports.ReviewBindingConsumerAssociationCurrentProjectionV1{projection.Ref.Revision: projection}
	s.associationCurrent[projection.Ref.ID] = projection.Ref
	return projection, nil
}

// CompareAndSwapReviewBindingConsumerAssociationV1 advances the immutable
// association history. The previous projection is never rewritten.
func (s *ReviewBindingCurrentStoreV1) CompareAndSwapReviewBindingConsumerAssociationV1(ctx context.Context, expected ports.ReviewBindingConsumerAssociationRefV1, next ports.ReviewBindingConsumerAssociationCurrentProjectionV1) (ports.ReviewBindingConsumerAssociationCurrentProjectionV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if err := next.Validate(); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.associationCurrent[expected.ID]
	if !exists {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review Binding Consumer association current index is absent")
	}
	if current != expected || next.Ref.ID != expected.ID || next.Ref.Revision != expected.Revision+1 || !next.Current {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding Consumer association CAS precondition drifted")
	}
	for _, projectionRef := range s.projectionCurrent {
		projection := s.projectionHistory[projectionRef.ID][projectionRef.Revision]
		if projection.ConsumerAssociation.Ref == expected {
			return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "association with an active Review projection requires compound CAS")
		}
	}
	previous := s.associationHistory[expected.ID][expected.Revision]
	if previous.Consumer != next.Consumer || previous.Source != next.Source || previous.CheckedUnixNano > next.CheckedUnixNano {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding Consumer association changed stable coordinates")
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() < next.CheckedUnixNano {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Binding Consumer association CAS clock regressed")
	}
	if err := next.ValidateCurrent(next.Ref, next.Consumer, next.Source, now); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if _, err := s.readAuthoritativeClosureLockedV1(ctx, next.Source, next, now); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	s.associationHistory[next.Ref.ID][next.Ref.Revision] = next
	s.associationCurrent[next.Ref.ID] = next.Ref
	return next, nil
}

func (s *ReviewBindingCurrentStoreV1) ResolveCurrentReviewBindingV1(ctx context.Context, request ports.ResolveReviewBindingCurrentRequestV1) (ports.ReviewBindingProjectionRefV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewBindingProjectionRefV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingProjectionRefV1{}, err
	}
	id, err := ports.DeriveReviewBindingProjectionIDV1(ports.ReviewBindingProjectionIdentityInputV1{Source: request.Source, Subject: request.Subject})
	if err != nil {
		return ports.ReviewBindingProjectionRefV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ref, exists := s.projectionCurrent[id]
	if !exists {
		return ports.ReviewBindingProjectionRefV1{}, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review Binding current index is absent")
	}
	if _, err := s.inspectCurrentLockedV1(ctx, ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: ref, ExpectedSource: request.Source, ExpectedSubject: request.Subject}); err != nil {
		return ports.ReviewBindingProjectionRefV1{}, err
	}
	return ref, nil
}

func (s *ReviewBindingCurrentStoreV1) InspectReviewBindingProjectionV1(ctx context.Context, request ports.InspectReviewBindingProjectionRequestV1) (ports.ReviewBindingCurrentProjectionV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	history, exists := s.projectionHistory[request.Ref.ID]
	if !exists {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review Binding projection history is absent")
	}
	projection, exists := history[request.Ref.Revision]
	if !exists {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review Binding projection revision is absent")
	}
	if projection.Ref != request.Ref || projection.Source != request.ExpectedSource || projection.Subject != request.ExpectedSubject || projection.Validate() != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding historical projection drifted")
	}
	return projection.CloneV1(), nil
}

func (s *ReviewBindingCurrentStoreV1) InspectCurrentReviewBindingV1(ctx context.Context, request ports.InspectCurrentReviewBindingRequestV1) (ports.ReviewBindingCurrentProjectionV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inspectCurrentLockedV1(ctx, request)
}

func (s *ReviewBindingCurrentStoreV1) inspectCurrentLockedV1(ctx context.Context, request ports.InspectCurrentReviewBindingRequestV1) (ports.ReviewBindingCurrentProjectionV1, error) {
	current, exists := s.projectionCurrent[request.ExpectedRef.ID]
	if !exists {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review Binding current index is absent")
	}
	if current != request.ExpectedRef || s.highestRevision[current.ID] != current.Revision {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding current index or highest revision drifted")
	}
	projection, exists := s.projectionHistory[current.ID][current.Revision]
	if !exists || projection.Ref != current || projection.Source != request.ExpectedSource || projection.Subject != request.ExpectedSubject {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding current history closure drifted")
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() < projection.CheckedUnixNano {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Binding current clock regressed")
	}
	association, err := s.currentAssociationLockedV1(projection.ConsumerAssociation.Ref)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	closure, err := s.readAuthoritativeClosureLockedV1(ctx, projection.Source, association, now)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	closureDigest, err := closure.DigestV1()
	if err != nil || closureDigest != projection.ClosureDigest || !reflect.DeepEqual(closure.Members, projection.Members) || closure.BindingSet.ID != projection.BindingSetID || closure.BindingSet.Revision != projection.BindingSetRevision || closure.BindingSet.Digest != projection.BindingSetDigest || closure.BindingSet.SemanticDigest != projection.BindingSetSemanticDigest || closure.BindingSet.ExpiresUnixNano != projection.BindingSetExpiresUnixNano || closure.SelectedGrant != projection.SelectedGrant || closure.ConsumerAssociation != projection.ConsumerAssociation || closure.ConsumerBinding != projection.ConsumerBinding || closure.ExpiresUnixNano != projection.ExpiresUnixNano {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding authoritative closure drifted without a projection advance")
	}
	if err := projection.ValidateCurrent(current, request.ExpectedSource, request.ExpectedSubject, now); err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	return projection.CloneV1(), nil
}

func (s *ReviewBindingCurrentStoreV1) InspectCurrentReviewBindingConsumerAssociationV1(ctx context.Context, expected ports.ReviewBindingConsumerAssociationRefV1) (ports.ReviewBindingConsumerAssociationCurrentProjectionV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	projection, err := s.currentAssociationLockedV1(expected)
	if err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	now := s.clock()
	if err := projection.ValidateCurrent(expected, projection.Consumer, projection.Source, now); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	consumer, err := s.providerCurrentLockedV1(ctx, projection.Consumer, now)
	if err != nil || consumer.Ref != projection.Consumer {
		if err != nil {
			return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
		}
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding Consumer current proof drifted")
	}
	return projection, nil
}

func (s *ReviewBindingCurrentStoreV1) currentAssociationLockedV1(expected ports.ReviewBindingConsumerAssociationRefV1) (ports.ReviewBindingConsumerAssociationCurrentProjectionV1, error) {
	current, exists := s.associationCurrent[expected.ID]
	if !exists {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review Binding Consumer association current index is absent")
	}
	if current != expected {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding Consumer association current index drifted")
	}
	projection, exists := s.associationHistory[expected.ID][expected.Revision]
	if !exists || projection.Ref != expected {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding Consumer association history drifted")
	}
	return projection, nil
}

func (s *ReviewBindingCurrentStoreV1) CreateReviewBindingProjectionV1(ctx context.Context, request ports.CreateReviewBindingProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if receipt, exists := s.receipts[request.PublishRef.ID]; exists {
		return receipt, nil
	}
	id, err := ports.DeriveReviewBindingProjectionIDV1(ports.ReviewBindingProjectionIdentityInputV1{Source: request.Input.Source, Subject: request.Input.Subject})
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if _, exists := s.projectionHistory[id]; exists || s.highestRevision[id] != 0 {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding projection identity already exists")
	}
	association, err := s.currentAssociationLockedV1(request.Input.Association)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	projection, err := s.buildProjectionLockedV1(ctx, request.Input.Source, request.Input.Subject, association, 1)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	receipt, err := ports.SealReviewBindingProjectionPublishReceiptV1(ports.ReviewBindingProjectionPublishReceiptV1{PublishRef: request.PublishRef, Projection: projection.Ref, CurrentIndex: projection.Ref, HighestRevision: projection.Ref.Revision})
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	// One lock is the reference transaction boundary: receipt, history,
	// highest revision and current full Ref become visible together.
	s.projectionHistory[id] = map[core.Revision]ports.ReviewBindingCurrentProjectionV1{projection.Ref.Revision: projection.CloneV1()}
	s.highestRevision[id] = projection.Ref.Revision
	s.projectionCurrent[id] = projection.Ref
	s.receipts[request.PublishRef.ID] = receipt
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Review Binding Create reply loss")
	}
	return receipt, nil
}

func (s *ReviewBindingCurrentStoreV1) CompareAndSwapReviewBindingProjectionV1(ctx context.Context, request ports.CompareAndSwapReviewBindingProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if receipt, exists := s.receipts[request.PublishRef.ID]; exists {
		return receipt, nil
	}
	current, exists := s.projectionCurrent[request.Input.ExpectedCurrent.ID]
	if !exists {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review Binding projection current index is absent")
	}
	if current != request.Input.ExpectedCurrent || s.highestRevision[current.ID] != current.Revision {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding projection CAS precondition drifted")
	}
	association, err := s.currentAssociationLockedV1(request.Input.Association)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	previous := s.projectionHistory[current.ID][current.Revision]
	if request.Input.Association != previous.ConsumerAssociation.Ref {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "association advance requires Owner compound CAS")
	}
	next, err := s.buildProjectionLockedV1(ctx, request.Input.Source, request.Input.Subject, association, current.Revision+1)
	if err != nil {
		if request.Input.Association != previous.ConsumerAssociation.Ref {
			return ports.ReviewBindingProjectionPublishReceiptV1{}, err
		}
		next, err = s.buildTerminalProjectionLockedV1(ctx, previous, request.Input.Source, request.Input.Subject, current.Revision+1)
		if err != nil {
			return ports.ReviewBindingProjectionPublishReceiptV1{}, err
		}
	}
	if next.ClosureDigest == previous.ClosureDigest && next.State == previous.State {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Review Binding CAS has no authoritative state change")
	}
	receipt, err := ports.SealReviewBindingProjectionPublishReceiptV1(ports.ReviewBindingProjectionPublishReceiptV1{PublishRef: request.PublishRef, Projection: next.Ref, CurrentIndex: next.Ref, HighestRevision: next.Ref.Revision})
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	// Stage completed: commit the four projection sidecars atomically.
	s.projectionHistory[current.ID][next.Ref.Revision] = next.CloneV1()
	s.highestRevision[current.ID] = next.Ref.Revision
	s.projectionCurrent[current.ID] = next.Ref
	s.receipts[request.PublishRef.ID] = receipt
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Review Binding CAS reply loss")
	}
	return receipt, nil
}

func (s *ReviewBindingCurrentStoreV1) CompareAndSwapReviewBindingAssociationProjectionV1(ctx context.Context, request control.CompareAndSwapReviewBindingAssociationProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if receipt, exists := s.receipts[request.Projection.PublishRef.ID]; exists {
		return receipt, nil
	}
	associationCurrent, exists := s.associationCurrent[request.ExpectedAssociation.ID]
	if !exists {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review Binding Consumer association current index is absent")
	}
	if associationCurrent != request.ExpectedAssociation {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding Consumer association compound CAS precondition drifted")
	}
	previousAssociation := s.associationHistory[request.ExpectedAssociation.ID][request.ExpectedAssociation.Revision]
	if previousAssociation.Ref != request.ExpectedAssociation || previousAssociation.Consumer != request.NextAssociation.Consumer || previousAssociation.Source != request.NextAssociation.Source || previousAssociation.CheckedUnixNano > request.NextAssociation.CheckedUnixNano {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding Consumer association compound CAS changed stable coordinates")
	}
	current := s.projectionCurrent[request.Projection.Input.ExpectedCurrent.ID]
	if current != request.Projection.Input.ExpectedCurrent || s.highestRevision[current.ID] != current.Revision {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding projection compound CAS precondition drifted")
	}
	previousProjection, exists := s.projectionHistory[current.ID][current.Revision]
	if !exists || previousProjection.ConsumerAssociation.Ref != request.ExpectedAssociation || previousProjection.Source != request.Projection.Input.Source || previousProjection.Subject != request.Projection.Input.Subject {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding compound CAS previous closure drifted")
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() < request.NextAssociation.CheckedUnixNano {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Binding compound CAS clock regressed")
	}
	if err := request.NextAssociation.ValidateCurrent(request.NextAssociation.Ref, request.NextAssociation.Consumer, request.NextAssociation.Source, now); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	nextProjection, err := s.buildProjectionLockedV1(ctx, request.Projection.Input.Source, request.Projection.Input.Subject, request.NextAssociation, current.Revision+1)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if nextProjection.ClosureDigest == previousProjection.ClosureDigest {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Review Binding compound CAS has no authoritative state change")
	}
	receipt, err := ports.SealReviewBindingProjectionPublishReceiptV1(ports.ReviewBindingProjectionPublishReceiptV1{PublishRef: request.Projection.PublishRef, Projection: nextProjection.Ref, CurrentIndex: nextProjection.Ref, HighestRevision: nextProjection.Ref.Revision})
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if s.failNextCompoundStage {
		s.failNextCompoundStage = false
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review Binding compound stage failure")
	}
	// One transaction boundary commits association history/current plus all
	// projection sidecars. No intermediate association-only state is visible.
	s.associationHistory[request.NextAssociation.Ref.ID][request.NextAssociation.Ref.Revision] = request.NextAssociation
	s.associationCurrent[request.NextAssociation.Ref.ID] = request.NextAssociation.Ref
	s.projectionHistory[current.ID][nextProjection.Ref.Revision] = nextProjection.CloneV1()
	s.highestRevision[current.ID] = nextProjection.Ref.Revision
	s.projectionCurrent[current.ID] = nextProjection.Ref
	s.receipts[request.Projection.PublishRef.ID] = receipt
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Review Binding compound CAS reply loss")
	}
	return receipt, nil
}

func (s *ReviewBindingCurrentStoreV1) buildTerminalProjectionLockedV1(ctx context.Context, previous ports.ReviewBindingCurrentProjectionV1, source ports.ReviewComponentBindingRefV2, subject ports.ReviewBindingSubjectV1, revision core.Revision) (ports.ReviewBindingCurrentProjectionV1, error) {
	set, err := s.inner.InspectBindingSet(ctx, source.BindingSetID)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	state := ports.ReviewBindingCurrentStateV1("")
	switch {
	case set.State == control.BindingSetRevoked:
		state = ports.ReviewBindingCurrentRevokedV1
	case set.State == control.BindingSetExpired:
		state = ports.ReviewBindingCurrentExpiredV1
	case set.Revision != source.BindingSetRevision:
		state = ports.ReviewBindingCurrentSupersededV1
	default:
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Review Binding authoritative closure changed without a terminal source transition")
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() < previous.CheckedUnixNano || !now.Before(time.Unix(0, previous.ExpiresUnixNano)) {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Review Binding terminal transition crossed its immutable TTL")
	}
	next := previous.CloneV1()
	next.Ref.Revision = revision
	next.Ref.Digest = ""
	next.State, next.Current = state, false
	next.CheckedUnixNano = now.UnixNano()
	next.ProjectionDigest = ""
	return ports.SealReviewBindingCurrentProjectionV1(next)
}

func (s *ReviewBindingCurrentStoreV1) InspectReviewBindingProjectionPublishV1(ctx context.Context, expected ports.ReviewBindingProjectionPublishRefV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	receipt, exists := s.receipts[expected.ID]
	if !exists {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review Binding Publish receipt is absent")
	}
	if receipt.PublishRef != expected || receipt.Validate() != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding Publish receipt drifted")
	}
	return receipt, nil
}

func (s *ReviewBindingCurrentStoreV1) buildProjectionLockedV1(ctx context.Context, source ports.ReviewComponentBindingRefV2, subject ports.ReviewBindingSubjectV1, association ports.ReviewBindingConsumerAssociationCurrentProjectionV1, revision core.Revision) (ports.ReviewBindingCurrentProjectionV1, error) {
	now := s.clock()
	if now.IsZero() {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Binding projection clock is unavailable")
	}
	closure, err := s.readAuthoritativeClosureLockedV1(ctx, source, association, now)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	return ports.SealReviewBindingCurrentProjectionV1(ports.ReviewBindingCurrentProjectionV1{
		Ref: ports.ReviewBindingProjectionRefV1{Revision: revision}, Source: source, Subject: subject,
		State: ports.ReviewBindingCurrentActiveV1, Current: true,
		BindingSetID: closure.BindingSet.ID, BindingSetRevision: closure.BindingSet.Revision,
		BindingSetDigest: closure.BindingSet.Digest, BindingSetSemanticDigest: closure.BindingSet.SemanticDigest,
		BindingSetExpiresUnixNano: closure.BindingSet.ExpiresUnixNano,
		Members:                   closure.Members, SelectedGrant: closure.SelectedGrant,
		ConsumerAssociation: closure.ConsumerAssociation, ConsumerBinding: closure.ConsumerBinding,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: closure.ExpiresUnixNano,
	})
}

func (s *ReviewBindingCurrentStoreV1) readAuthoritativeClosureLockedV1(ctx context.Context, source ports.ReviewComponentBindingRefV2, association ports.ReviewBindingConsumerAssociationCurrentProjectionV1, now time.Time) (ports.ReviewBindingAuthoritativeClosureInputV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	if err := source.Validate(); err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	set, err := s.inner.InspectBindingSet(ctx, source.BindingSetID)
	if err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	if set.Revision != source.BindingSetRevision || set.State != control.BindingSetActive || !now.Before(time.Unix(0, set.ExpiresUnixNano)) {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Review Binding source BindingSet is not current")
	}
	setDigest, err := control.BindingSetDigestV2(set)
	if err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	semanticDigest, err := control.BindingSetSemanticDigestV2(set)
	if err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	members := make([]ports.ReviewBindingMemberCurrentRefV1, 0, len(set.Members))
	facts := make([]control.BindingFactV2, 0, len(set.Members))
	probes := make([]control.BindingCurrentProbeV2, 0, len(set.Members))
	minimum := set.ExpiresUnixNano
	var selected ports.ReviewBindingSelectedGrantRefV1
	selectedCount := 0
	for _, member := range set.Members {
		fact, inspectErr := s.inner.InspectBinding(ctx, member.BindingID)
		if inspectErr != nil {
			return ports.ReviewBindingAuthoritativeClosureInputV1{}, inspectErr
		}
		if fact.Validate() != nil || fact.State != control.BindingBound || fact.BindingSetID != set.ID || fact.Revision != member.BindingRevision || fact.ComponentID != member.ComponentID || fact.ManifestDigest != member.ManifestDigest || fact.Manifest.ArtifactDigest != member.ArtifactDigest {
			return ports.ReviewBindingAuthoritativeClosureInputV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding member Fact drifted from its Set")
		}
		setGrantDigest, setGrantErr := control.BindingGrantSetDigestV2(member.Grants)
		factGrantDigest, factGrantErr := control.BindingGrantSetDigestV2(fact.Grants)
		if setGrantErr != nil || factGrantErr != nil || setGrantDigest != factGrantDigest || !reflect.DeepEqual(member.Grants, fact.Grants) {
			return ports.ReviewBindingAuthoritativeClosureInputV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding Set and Fact Grant closures drifted")
		}
		factDigest, digestErr := digestReviewBindingFactV1(fact)
		if digestErr != nil {
			return ports.ReviewBindingAuthoritativeClosureInputV1{}, digestErr
		}
		grantMin := fact.ExpiresUnixNano
		for _, grant := range fact.Grants {
			if grant.ExpiresUnixNano < grantMin {
				grantMin = grant.ExpiresUnixNano
			}
			if member.ComponentID == source.ComponentID && member.ManifestDigest == source.ManifestDigest && member.ArtifactDigest == source.ArtifactDigest && grant.Capability == source.Capability {
				grantDigest, digestErr := digestReviewBindingGrantV1(grant)
				if digestErr != nil {
					return ports.ReviewBindingAuthoritativeClosureInputV1{}, digestErr
				}
				selected = ports.ReviewBindingSelectedGrantRefV1{ComponentID: member.ComponentID, BindingID: member.BindingID, BindingRevision: member.BindingRevision, Capability: grant.Capability, SetGrantDigest: grantDigest, FactGrantDigest: grantDigest, ExpiresUnixNano: grant.ExpiresUnixNano}
				selectedCount++
			}
		}
		minimum = minReviewBindingStoreExpiryV1(minimum, fact.ExpiresUnixNano, grantMin)
		members = append(members, ports.ReviewBindingMemberCurrentRefV1{ComponentID: member.ComponentID, BindingID: member.BindingID, BindingRevision: member.BindingRevision, BindingFactDigest: factDigest, ManifestDigest: member.ManifestDigest, ArtifactDigest: member.ArtifactDigest, SetGrantSetDigest: setGrantDigest, FactGrantSetDigest: factGrantDigest, BindingFactExpiresUnixNano: fact.ExpiresUnixNano, SetGrantMinExpiresUnixNano: grantMin, FactGrantMinExpiresUnixNano: grantMin})
		facts = append(facts, fact)
		probes = append(probes, control.BindingCurrentProbeV2{ComponentID: member.ComponentID, ManifestDigest: member.ManifestDigest})
	}
	if selectedCount != 1 {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Review Binding Source capability is missing or ambiguous")
	}
	if err := control.ValidateBindingSetCurrentV2(set, facts, probes, now); err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ComponentID < members[j].ComponentID })
	consumer, err := s.providerCurrentLockedV1(ctx, association.Consumer, now)
	if err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	if err := association.ValidateCurrent(association.Ref, association.Consumer, source, now); err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	minimum = minReviewBindingStoreExpiryV1(minimum, selected.ExpiresUnixNano, association.ExpiresUnixNano, consumer.ExpiresUnixNano)
	closure := ports.ReviewBindingAuthoritativeClosureInputV1{
		Source:     source,
		BindingSet: ports.ReviewBindingSetExactRefV1{ID: set.ID, Revision: set.Revision, Digest: setDigest, SemanticDigest: semanticDigest, ExpiresUnixNano: set.ExpiresUnixNano},
		Members:    members, SelectedGrant: selected, ConsumerAssociation: association, ConsumerBinding: consumer, ExpiresUnixNano: minimum,
	}
	return closure, closure.Validate()
}

func (s *ReviewBindingCurrentStoreV1) providerCurrentLockedV1(ctx context.Context, expected ports.ProviderBindingRefV2, now time.Time) (ports.ProviderBindingCurrentProjectionV2, error) {
	adapter := control.ProviderBindingCurrentnessAdapterV2{Bindings: s.inner, Clock: func() time.Time { return now }}
	return adapter.InspectProviderBindingCurrentV2(ctx, expected)
}

func digestReviewBindingFactV1(fact control.BindingFactV2) (core.Digest, error) {
	if err := fact.Validate(); err != nil {
		return "", err
	}
	copy := fact
	copy.Grants = append([]ports.CapabilityGrantV2(nil), fact.Grants...)
	copy.RenewalEvidence = append([]ports.EvidenceRecordRefV2(nil), fact.RenewalEvidence...)
	return core.CanonicalJSONDigest("praxis.runtime.binding", ports.BindingContractVersionV2, "BindingFactV2", copy)
}

func digestReviewBindingGrantV1(grant ports.CapabilityGrantV2) (core.Digest, error) {
	if err := ports.ValidateCapabilityGrantStructureV2([]ports.CapabilityGrantV2{grant}); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.binding", ports.BindingContractVersionV2, "CapabilityGrantV2", grant)
}

func minReviewBindingStoreExpiryV1(first int64, rest ...int64) int64 {
	minimum := first
	for _, value := range rest {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

// BindingFactPortV2 is forwarded under the same outer lock so no binding
// mutation can interleave with a Review Binding snapshot or publication.
func (s *ReviewBindingCurrentStoreV1) CreateBinding(ctx context.Context, fact control.BindingFactV2) (control.BindingFactV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.CreateBinding(ctx, fact)
}

func (s *ReviewBindingCurrentStoreV1) InspectBinding(ctx context.Context, id string) (control.BindingFactV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.InspectBinding(ctx, id)
}

func (s *ReviewBindingCurrentStoreV1) CompareAndSwapBinding(ctx context.Context, request control.BindingFactCASRequestV2) (control.BindingFactV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.CompareAndSwapBinding(ctx, request)
}

func (s *ReviewBindingCurrentStoreV1) CommitBindingSet(ctx context.Context, request control.CommitBindingSetRequestV2) (control.BindingSetFactV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.CommitBindingSet(ctx, request)
}

func (s *ReviewBindingCurrentStoreV1) InspectBindingSet(ctx context.Context, id string) (control.BindingSetFactV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.InspectBindingSet(ctx, id)
}

func (s *ReviewBindingCurrentStoreV1) CompareAndSwapBindingSet(ctx context.Context, request control.BindingSetCASRequestV2) (control.BindingSetFactV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.CompareAndSwapBindingSet(ctx, request)
}

func (s *ReviewBindingCurrentStoreV1) RenewBindingSetV2(ctx context.Context, request control.RenewBindingSetRequestV2) (control.BindingSetFactV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.RenewBindingSetV2(ctx, request)
}

func (s *ReviewBindingCurrentStoreV1) SetRenewalAttestationsV2(reader control.BindingRenewalAttestationReaderV2) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inner.SetRenewalAttestations(reader)
}

var _ control.BindingFactPortV2 = (*ReviewBindingCurrentStoreV1)(nil)
var _ control.BindingRenewalPortV2 = (*ReviewBindingCurrentStoreV1)(nil)
var _ control.ReviewBindingCurrentRepositoryV1 = (*ReviewBindingCurrentStoreV1)(nil)
var _ control.ReviewBindingAssociationProjectionPublisherV1 = (*ReviewBindingCurrentStoreV1)(nil)

package fakes

import (
	"context"
	"reflect"
	"strings"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ReviewEvidenceApplicabilityStoreV1 is an in-memory reference store. It has
// no production durability, topology or SLA claim.
type ReviewEvidenceApplicabilityStoreV1 struct {
	mu sync.Mutex

	history  map[string]map[core.Revision]ports.ReviewEvidenceApplicabilityProjectionV1
	current  map[core.Digest]ports.ReviewEvidenceApplicabilityCurrentIndexRefV1
	receipts map[string]ports.ReviewEvidenceApplicabilityPublishReceiptV1

	failNextCommit bool
	loseNextReply  bool
}

func NewReviewEvidenceApplicabilityStoreV1() *ReviewEvidenceApplicabilityStoreV1 {
	return &ReviewEvidenceApplicabilityStoreV1{
		history:  make(map[string]map[core.Revision]ports.ReviewEvidenceApplicabilityProjectionV1),
		current:  make(map[core.Digest]ports.ReviewEvidenceApplicabilityCurrentIndexRefV1),
		receipts: make(map[string]ports.ReviewEvidenceApplicabilityPublishReceiptV1),
	}
}

func (s *ReviewEvidenceApplicabilityStoreV1) FailNextReviewEvidenceApplicabilityCommitV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNextCommit = true
}

func (s *ReviewEvidenceApplicabilityStoreV1) LoseNextReviewEvidenceApplicabilityReplyV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextReply = true
}

func (s *ReviewEvidenceApplicabilityStoreV1) InspectReviewEvidenceApplicabilityCurrentFactV1(ctx context.Context, subjectDigest core.Digest) (ports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	if err := subjectDigest.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index, ok := s.current[subjectDigest]
	if !ok {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "Review evidence applicability current index is absent")
	}
	projection, ok := s.history[index.CurrentProjection.ProjectionID][index.CurrentProjection.Revision]
	if !ok || projection.Ref != index.CurrentProjection {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability current history closure drifted")
	}
	snapshot := ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{ContractVersion: ports.ReviewEvidenceCurrentContractVersionV1, Projection: projection, CurrentIndex: index}
	if err := snapshot.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	return ports.CloneReviewEvidenceApplicabilityCurrentSnapshotV1(snapshot), nil
}

func (s *ReviewEvidenceApplicabilityStoreV1) InspectReviewEvidenceApplicabilityProjectionFactV1(ctx context.Context, ref ports.ReviewEvidenceApplicabilityRefV1) (ports.ReviewEvidenceApplicabilityProjectionV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	revisions, ok := s.history[ref.ProjectionID]
	if !ok {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "Review evidence applicability history is absent")
	}
	projection, ok := revisions[ref.Revision]
	if !ok {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "Review evidence applicability revision is absent")
	}
	if projection.Ref != ref || projection.Validate() != nil {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability historical ref drifted")
	}
	return ports.CloneReviewEvidenceApplicabilityProjectionV1(projection), nil
}

func (s *ReviewEvidenceApplicabilityStoreV1) PublishReviewEvidenceApplicabilityFactV1(ctx context.Context, request ports.PublishReviewEvidenceApplicabilityRequestV1, receipt ports.ReviewEvidenceApplicabilityPublishReceiptV1) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if err := receipt.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if receipt.PublishID != string(request.RequestDigest) || receipt.RequestDigest != request.RequestDigest || receipt.Projection != request.Projection.Ref || !reflect.DeepEqual(receipt.CurrentIndex, request.NextCurrentIndex) {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability receipt drifted from publish request")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.receipts[receipt.PublishID]; ok {
		if existing.Validate() != nil || existing.RequestDigest != request.RequestDigest || existing.Projection != request.Projection.Ref || !reflect.DeepEqual(existing.CurrentIndex, request.NextCurrentIndex) {
			return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review evidence applicability PublishID changed content")
		}
		return ports.CloneReviewEvidenceApplicabilityPublishReceiptV1(existing), nil
	}

	current, exists := s.current[request.Projection.SubjectDigest]
	if request.ExpectedCurrentIndex == nil {
		if exists || request.Projection.Ref.Revision != 1 || s.history[request.Projection.Ref.ProjectionID] != nil {
			return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review evidence applicability first publish identity already exists")
		}
	} else if !exists || !reflect.DeepEqual(current, *request.ExpectedCurrentIndex) || current.CurrentProjection != *request.Projection.Previous || current.HighestRevision != current.Revision || request.Projection.Ref.Revision != current.Revision+1 {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review evidence applicability current CAS precondition drifted")
	}
	if request.NextCurrentIndex.CurrentProjection != request.Projection.Ref || request.NextCurrentIndex.SubjectDigest != request.Projection.SubjectDigest || request.NextCurrentIndex.Revision != request.Projection.Ref.Revision || request.NextCurrentIndex.HighestRevision != request.Projection.Ref.Revision {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review evidence applicability next current index drifted")
	}
	if revisions := s.history[request.Projection.Ref.ProjectionID]; revisions != nil {
		if existing, ok := revisions[request.Projection.Ref.Revision]; ok && !reflect.DeepEqual(existing, request.Projection) {
			return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review evidence applicability immutable history would be overwritten")
		}
	}
	if s.failNextCommit {
		s.failNextCommit = false
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review evidence applicability staged failure")
	}
	if s.history[request.Projection.Ref.ProjectionID] == nil {
		s.history[request.Projection.Ref.ProjectionID] = make(map[core.Revision]ports.ReviewEvidenceApplicabilityProjectionV1)
	}
	s.history[request.Projection.Ref.ProjectionID][request.Projection.Ref.Revision] = ports.CloneReviewEvidenceApplicabilityProjectionV1(request.Projection)
	s.current[request.Projection.SubjectDigest] = request.NextCurrentIndex
	s.receipts[receipt.PublishID] = ports.CloneReviewEvidenceApplicabilityPublishReceiptV1(receipt)
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Review evidence applicability publish reply loss")
	}
	return ports.CloneReviewEvidenceApplicabilityPublishReceiptV1(receipt), nil
}

func (s *ReviewEvidenceApplicabilityStoreV1) InspectReviewEvidenceApplicabilityPublishFactV1(ctx context.Context, publishID string) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if strings.TrimSpace(publishID) == "" {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review evidence applicability PublishID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	receipt, ok := s.receipts[publishID]
	if !ok {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "Review evidence applicability publish receipt is absent")
	}
	if err := receipt.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	return ports.CloneReviewEvidenceApplicabilityPublishReceiptV1(receipt), nil
}

var _ control.ReviewEvidenceApplicabilityFactPortV1 = (*ReviewEvidenceApplicabilityStoreV1)(nil)

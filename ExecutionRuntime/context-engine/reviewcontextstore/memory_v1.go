// Package reviewcontextstore provides Context Owner storage for immutable
// Reviewer Context envelopes. MemoryV1 is a process-local reference store; it
// is not a durable State Plane backend, production root or SLA.
package reviewcontextstore

import (
	"context"
	"reflect"
	"sync"

	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// RepositoryV1 is the narrow Context-owned persistence seam used by the
// Review public adapter. Its method names intentionally differ from the public
// Review ports so callers cannot bypass adapter currentness and recovery.
type RepositoryV1 interface {
	CommitV1(context.Context, reviewport.ReviewerContextPublishRequestV1) (reviewport.ReviewerContextPublishReceiptV1, error)
	ResolveV1(context.Context, reviewcontract.ReviewerContextSubjectV1) (reviewcontract.ReviewerContextEnvelopeRefV1, error)
	InspectCurrentV1(context.Context, reviewcontract.ReviewerContextSubjectV1, reviewcontract.ReviewerContextEnvelopeRefV1) (reviewcontract.ReviewerContextEnvelopeV1, error)
	InspectHistoricalV1(context.Context, reviewcontract.ReviewerContextEnvelopeRefV1) (reviewcontract.ReviewerContextEnvelopeV1, error)
}

type identityKeyV1 struct {
	tenant core.TenantID
	id     string
}

type historyKeyV1 struct {
	identity identityKeyV1
	revision core.Revision
}

type MemoryV1 struct {
	mu      sync.RWMutex
	history map[historyKeyV1]reviewcontract.ReviewerContextEnvelopeV1
	highest map[identityKeyV1]core.Revision
	current map[identityKeyV1]reviewcontract.ReviewerContextEnvelopeRefV1
}

func NewMemoryV1() *MemoryV1 {
	return &MemoryV1{
		history: make(map[historyKeyV1]reviewcontract.ReviewerContextEnvelopeV1),
		highest: make(map[identityKeyV1]core.Revision),
		current: make(map[identityKeyV1]reviewcontract.ReviewerContextEnvelopeRefV1),
	}
}

var _ RepositoryV1 = (*MemoryV1)(nil)

func (m *MemoryV1) CommitV1(ctx context.Context, request reviewport.ReviewerContextPublishRequestV1) (reviewport.ReviewerContextPublishReceiptV1, error) {
	if m == nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, invalidV1("Reviewer Context repository is unavailable")
	}
	if err := checkContextV1(ctx); err != nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, err
	}
	value := request.Value.Clone()
	identity := identityKeyV1{tenant: value.Ref.TenantID, id: value.Ref.ID}
	targetKey := historyKeyV1{identity: identity, revision: value.Ref.Revision}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Existing revisions are immutable. Exact canonical replays are read-only
	// idempotent observations; changed payload or causal predecessor conflicts.
	if prior, exists := m.history[targetKey]; exists {
		if !reflect.DeepEqual(prior, value) || !m.replayPredecessorExactLockedV1(request, identity) {
			return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context revision already exists with different canonical content or predecessor")
		}
		if highest, ok := m.highest[identity]; !ok || highest < value.Ref.Revision {
			return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context revision history watermark is inconsistent")
		}
		return reviewport.ReviewerContextPublishReceiptV1{Ref: value.Ref, Created: false}, nil
	}

	if request.Previous == nil {
		if _, exists := m.highest[identity]; exists {
			return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context stable identity already exists")
		}
		if _, exists := m.current[identity]; exists {
			return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context current index already exists")
		}
	} else {
		current, exists := m.current[identity]
		if !exists || current != *request.Previous {
			return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context current full-ref CAS failed")
		}
		highest, exists := m.highest[identity]
		if !exists || highest != request.Previous.Revision || value.Ref.Revision != highest+1 {
			return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context highest revision CAS failed")
		}
		previous, exists := m.history[historyKeyV1{identity: identity, revision: request.Previous.Revision}]
		if !exists || previous.Ref != *request.Previous || previous.Subject != value.Subject {
			return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context predecessor or exact subject drifted")
		}
	}

	// The immutable history append, highest watermark and current full-ref CAS
	// are the only mutation point and share this single lock boundary.
	m.history[targetKey] = value.Clone()
	m.highest[identity] = value.Ref.Revision
	m.current[identity] = value.Ref
	return reviewport.ReviewerContextPublishReceiptV1{Ref: value.Ref, Created: true}, nil
}

func (m *MemoryV1) replayPredecessorExactLockedV1(request reviewport.ReviewerContextPublishRequestV1, identity identityKeyV1) bool {
	if request.Value.Ref.Revision == 1 {
		return request.Previous == nil
	}
	if request.Previous == nil || request.Previous.Revision+1 != request.Value.Ref.Revision {
		return false
	}
	previous, exists := m.history[historyKeyV1{identity: identity, revision: request.Previous.Revision}]
	return exists && previous.Ref == *request.Previous && previous.Subject == request.Value.Subject
}

func (m *MemoryV1) ResolveV1(ctx context.Context, subject reviewcontract.ReviewerContextSubjectV1) (reviewcontract.ReviewerContextEnvelopeRefV1, error) {
	if m == nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, invalidV1("Reviewer Context repository is unavailable")
	}
	if err := checkContextV1(ctx); err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, err
	}
	if err := subject.Validate(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, err
	}
	id, err := reviewcontract.DeriveReviewerContextEnvelopeIDV1(subject)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, err
	}
	identity := identityKeyV1{tenant: subject.TenantID, id: id}
	m.mu.RLock()
	defer m.mu.RUnlock()
	ref, exists := m.current[identity]
	if !exists {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, notFoundV1("Reviewer Context subject has no current envelope")
	}
	value, exists := m.history[historyKeyV1{identity: identity, revision: ref.Revision}]
	if !exists || value.Ref != ref || value.Subject != subject || m.highest[identity] != ref.Revision {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, conflictV1("Reviewer Context subject current index is inconsistent")
	}
	return ref, nil
}

func (m *MemoryV1) InspectCurrentV1(ctx context.Context, subject reviewcontract.ReviewerContextSubjectV1, expected reviewcontract.ReviewerContextEnvelopeRefV1) (reviewcontract.ReviewerContextEnvelopeV1, error) {
	if m == nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, invalidV1("Reviewer Context repository is unavailable")
	}
	if err := checkContextV1(ctx); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	if err := subject.Validate(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	id, err := reviewcontract.DeriveReviewerContextEnvelopeIDV1(subject)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	identity := identityKeyV1{tenant: subject.TenantID, id: id}
	if expected.TenantID != subject.TenantID || expected.ID != id {
		return reviewcontract.ReviewerContextEnvelopeV1{}, conflictV1("Reviewer Context expected ref does not belong to the exact subject")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	current, exists := m.current[identity]
	if !exists {
		return reviewcontract.ReviewerContextEnvelopeV1{}, notFoundV1("Reviewer Context subject has no current envelope")
	}
	if current != expected || m.highest[identity] != expected.Revision {
		return reviewcontract.ReviewerContextEnvelopeV1{}, conflictV1("Reviewer Context current full ref drifted")
	}
	value, exists := m.history[historyKeyV1{identity: identity, revision: expected.Revision}]
	if !exists || value.Ref != expected || value.Subject != subject {
		return reviewcontract.ReviewerContextEnvelopeV1{}, conflictV1("Reviewer Context current history closure drifted")
	}
	return value.Clone(), nil
}

func (m *MemoryV1) InspectHistoricalV1(ctx context.Context, exact reviewcontract.ReviewerContextEnvelopeRefV1) (reviewcontract.ReviewerContextEnvelopeV1, error) {
	if m == nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, invalidV1("Reviewer Context repository is unavailable")
	}
	if err := checkContextV1(ctx); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	if err := exact.Validate(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	identity := identityKeyV1{tenant: exact.TenantID, id: exact.ID}
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, exists := m.history[historyKeyV1{identity: identity, revision: exact.Revision}]
	if !exists {
		if _, identityExists := m.highest[identity]; identityExists {
			return reviewcontract.ReviewerContextEnvelopeV1{}, conflictV1("Reviewer Context historical revision is not part of this identity")
		}
		return reviewcontract.ReviewerContextEnvelopeV1{}, notFoundV1("Reviewer Context historical envelope was not found")
	}
	if value.Ref != exact {
		return reviewcontract.ReviewerContextEnvelopeV1{}, conflictV1("Reviewer Context historical digest drifted")
	}
	return value.Clone(), nil
}

func checkContextV1(ctx context.Context) error {
	if ctx == nil {
		return invalidV1("Reviewer Context context is required")
	}
	if err := ctx.Err(); err != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "Reviewer Context operation was canceled before a linearized result")
	}
	return nil
}

func invalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}

func notFoundV1(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, message)
}

func conflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, message)
}

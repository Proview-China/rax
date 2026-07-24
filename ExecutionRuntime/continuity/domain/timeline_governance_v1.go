package domain

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type TimelineAdmissionBindingsV1 struct {
	EvidenceProjectionRef      string
	EvidenceProjectionDigest   string
	EvidenceCurrentIndexRef    string
	EvidenceCurrentIndexDigest string
	OwnerProjectionDigest      string
	PolicyProjectionDigest     string
	CheckedUnixNano            int64
	NaturalNotAfterUnixNano    int64
}

func (b TimelineAdmissionBindingsV1) validate() error {
	for field, value := range map[string]string{
		"evidence_projection_ref":       b.EvidenceProjectionRef,
		"evidence_projection_digest":    b.EvidenceProjectionDigest,
		"evidence_current_index_ref":    b.EvidenceCurrentIndexRef,
		"evidence_current_index_digest": b.EvidenceCurrentIndexDigest,
		"policy_projection_digest":      b.PolicyProjectionDigest,
	} {
		if err := contract.ValidateToken(field, value); err != nil {
			return err
		}
	}
	if b.CheckedUnixNano <= 0 || b.NaturalNotAfterUnixNano <= b.CheckedUnixNano {
		return contract.NewError(contract.ErrInvalidArgument, "admission_ttl", "natural checked and expiry are inconsistent")
	}
	return nil
}

type TimelineProjectionControllerV1 struct {
	repository ports.TimelineGovernanceRepositoryV1
	clock      Clock
}

func NewTimelineProjectionControllerV1(repository ports.TimelineGovernanceRepositoryV1, clock Clock) (*TimelineProjectionControllerV1, error) {
	if nilOrTypedNilTimelineGovernanceV1(repository) || nilOrTypedNilTimelineGovernanceV1(clock) {
		return nil, contract.NewError(contract.ErrUnavailable, "timeline_governance", "repository and clock are required")
	}
	return &TimelineProjectionControllerV1{repository: repository, clock: clock}, nil
}

func (c *TimelineProjectionControllerV1) CreateAttempt(ctx context.Context, request contract.TimelineProjectionRequestV1) (contract.TimelineProjectionAttemptFactV1, bool, error) {
	if err := request.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, false, err
	}
	fact, err := contract.SealTimelineProjectionAttemptV1(contract.TimelineProjectionAttemptFactV1{
		ContractVersion: contract.TimelineGovernanceContractVersionV1,
		Ref:             contract.TimelineProjectionAttemptRefV1{AttemptID: request.AttemptID, Revision: 1, ScopeDigest: request.ScopeDigest},
		Request:         request.Clone(), State: contract.TimelineAttemptProposedV1,
	})
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, false, err
	}
	return c.repository.CreateTimelineProjectionAttemptV1(ctx, fact)
}

func (c *TimelineProjectionControllerV1) InspectAttempt(ctx context.Context, ref contract.TimelineProjectionAttemptRefV1) (contract.TimelineProjectionAttemptFactV1, error) {
	if err := ref.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	return c.repository.InspectTimelineProjectionAttemptV1(ctx, ref)
}

func (c *TimelineProjectionControllerV1) InspectCurrent(ctx context.Context, event contract.TimelineEventRefV1) (contract.TimelineProjectionCurrentV1, error) {
	if err := event.Validate(); err != nil {
		return contract.TimelineProjectionCurrentV1{}, err
	}
	return c.repository.InspectTimelineProjectionCurrentV1(ctx, event)
}

func (c *TimelineProjectionControllerV1) BeginInspection(ctx context.Context, expected contract.TimelineProjectionAttemptRefV1) (contract.TimelineProjectionAttemptFactV1, error) {
	return c.transition(ctx, expected, contract.TimelineAttemptInspectingV1, TimelineAdmissionBindingsV1{})
}

func (c *TimelineProjectionControllerV1) Admit(ctx context.Context, expected contract.TimelineProjectionAttemptRefV1, bindings TimelineAdmissionBindingsV1) (contract.TimelineProjectionAttemptFactV1, error) {
	if err := bindings.validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	return c.transition(ctx, expected, contract.TimelineAttemptAdmittedV1, bindings)
}

func (c *TimelineProjectionControllerV1) RequireReconcile(ctx context.Context, expected contract.TimelineProjectionAttemptRefV1) (contract.TimelineProjectionAttemptFactV1, error) {
	return c.transition(ctx, expected, contract.TimelineAttemptReconcileRequiredV1, TimelineAdmissionBindingsV1{})
}

func (c *TimelineProjectionControllerV1) transition(ctx context.Context, expected contract.TimelineProjectionAttemptRefV1, state contract.TimelineProjectionAttemptStateV1, bindings TimelineAdmissionBindingsV1) (contract.TimelineProjectionAttemptFactV1, error) {
	current, err := c.repository.InspectTimelineProjectionAttemptV1(ctx, expected)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	next := current.Clone()
	next.Ref.Revision++
	next.State = state
	next.Ref.Digest = ""
	if state == contract.TimelineAttemptAdmittedV1 {
		now := c.clock.Now()
		if now.IsZero() || now.UnixNano() < bindings.CheckedUnixNano {
			return contract.TimelineProjectionAttemptFactV1{}, contract.NewError(contract.ErrPreconditionFailed, "clock", "fresh time is missing or before sealed check time")
		}
		notAfter := bindings.NaturalNotAfterUnixNano
		if current.Request.RequestedNotAfter > 0 && current.Request.RequestedNotAfter < notAfter {
			notAfter = current.Request.RequestedNotAfter
		}
		if now.UnixNano() >= notAfter {
			return contract.TimelineProjectionAttemptFactV1{}, contract.NewError(contract.ErrPreconditionFailed, "not_after", "projection current window has expired")
		}
		next.EvidenceProjectionRef = bindings.EvidenceProjectionRef
		next.EvidenceProjectionDigest = bindings.EvidenceProjectionDigest
		next.EvidenceCurrentIndexRef = bindings.EvidenceCurrentIndexRef
		next.EvidenceCurrentIndexDigest = bindings.EvidenceCurrentIndexDigest
		next.OwnerProjectionDigest = bindings.OwnerProjectionDigest
		next.PolicyProjectionDigest = bindings.PolicyProjectionDigest
		next.CheckedUnixNano = bindings.CheckedUnixNano
		next.NotAfterUnixNano = notAfter
	}
	sealed, err := contract.SealTimelineProjectionAttemptV1(next)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	return c.repository.CompareAndSwapTimelineProjectionAttemptV1(ctx, expected, sealed)
}

func (c *TimelineProjectionControllerV1) Publish(ctx context.Context, expected contract.TimelineProjectionAttemptRefV1, event contract.TimelineEventRecord) (contract.TimelineProjectionAttemptFactV1, contract.TimelineProjectionCurrentV1, error) {
	admitted, err := c.repository.InspectTimelineProjectionAttemptV1(ctx, expected)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	if admitted.State != contract.TimelineAttemptAdmittedV1 && admitted.State != contract.TimelineAttemptReconcileRequiredV1 {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrRevisionConflict, "attempt_state", "publish requires admitted or reconciled attempt")
	}
	if err := validateEventForTimelineRequestV1(admitted.Request, event); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	now := c.clock.Now()
	if now.IsZero() || now.UnixNano() >= admitted.NotAfterUnixNano {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrPreconditionFailed, "not_after", "projection expired before publish")
	}
	eventRef := contract.TimelineEventRefV1{
		EventID: event.Candidate.CandidateID, EvidenceRecordRef: event.EvidenceRecordRef,
		LedgerScopeDigest: event.LedgerScopeDigest, LedgerSequence: event.LedgerSequence,
		Digest: event.Candidate.Digest,
	}
	visible := admitted.Clone()
	visible.Ref.Revision++
	visible.Ref.Digest = ""
	visible.State = contract.TimelineAttemptVisibleV1
	visible.Event = &eventRef
	visible, err = contract.SealTimelineProjectionAttemptV1(visible)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	current, err := contract.SealTimelineProjectionCurrentV1(contract.TimelineProjectionCurrentV1{
		ContractVersion: contract.TimelineGovernanceContractVersionV1,
		Event:           eventRef, Attempt: visible.Ref,
		EvidenceProjectionRef:      admitted.EvidenceProjectionRef,
		EvidenceProjectionDigest:   admitted.EvidenceProjectionDigest,
		EvidenceCurrentIndexRef:    admitted.EvidenceCurrentIndexRef,
		EvidenceCurrentIndexDigest: admitted.EvidenceCurrentIndexDigest,
		OwnerProjectionDigest:      admitted.OwnerProjectionDigest,
		PolicyProjectionDigest:     admitted.PolicyProjectionDigest,
		CheckedUnixNano:            admitted.CheckedUnixNano, NotAfterUnixNano: admitted.NotAfterUnixNano,
	})
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	return c.repository.PublishTimelineProjectionV1(ctx, ports.PublishTimelineProjectionV1Request{
		ExpectedAttempt: expected, VisibleAttempt: visible, Event: event.Clone(), Current: current,
	})
}

func validateEventForTimelineRequestV1(request contract.TimelineProjectionRequestV1, event contract.TimelineEventRecord) error {
	if err := event.Validate(); err != nil {
		return err
	}
	if event.Candidate.Scope.ExecutionScopeDigest != request.ScopeDigest || event.Candidate.ProjectionPolicyRef != request.ProjectionPolicy || event.Candidate.Evidence.SourceKey != request.EvidenceSource {
		return contract.NewError(contract.ErrProjectionConflict, "event", "reader-derived event differs from request coordinates")
	}
	if request.ExpectedRecord != nil {
		actual := contract.TimelineEvidenceRecordRefV1{LedgerScopeDigest: event.LedgerScopeDigest, Sequence: event.LedgerSequence, RecordDigest: event.EvidenceRecordDigest}
		if actual != *request.ExpectedRecord {
			return contract.NewError(contract.ErrProjectionConflict, "expected_record", "reader returned another record")
		}
	}
	if event.TrustClass == contract.TrustAuthoritativeFact {
		if request.OwnerFact == nil || event.Candidate.OwnerFactExactRef == nil || *request.OwnerFact != *event.Candidate.OwnerFactExactRef {
			return contract.NewError(contract.ErrUnsupported, "owner_fact", "authoritative projection requires typed owner current routing")
		}
	} else if request.OwnerFact != nil || event.Candidate.OwnerFactRef != nil || event.Candidate.OwnerFactExactRef != nil {
		return contract.NewError(contract.ErrProjectionConflict, "owner_fact", "non-authoritative projection cannot carry owner fact")
	}
	return nil
}

func nilOrTypedNilTimelineGovernanceV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

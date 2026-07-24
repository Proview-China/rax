package runtimeadapter

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	continuityports "github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type TimelineProjectionPolicyCurrentV1 = contract.TimelineProjectionPolicyCurrentV1
type TimelineProjectionPolicyCurrentReaderV1 = continuityports.TimelineProjectionPolicyCurrentReaderV1

// TimelineOwnerCurrentProjectionV1 is produced by a domain-specific typed
// Owner reader. It does not reinterpret Runtime evidence trust.
type TimelineOwnerCurrentProjectionV1 = continuityports.TimelineOwnerCurrentProjectionV1
type TimelineTypedOwnerCurrentReaderV1 = continuityports.TimelineTypedOwnerCurrentReaderV1
type TimelineTypedOwnerRouterV1 = continuityports.TimelineTypedOwnerRouterV1
type TimelineOwnerCurrentInspectRequestV2 = continuityports.TimelineOwnerCurrentInspectRequestV2
type TimelineTypedOwnerCurrentReaderV2 = continuityports.TimelineTypedOwnerCurrentReaderV2
type TimelineTypedOwnerRouterV2 = continuityports.TimelineTypedOwnerRouterV2

type TimelineProjectionAdapterV1 struct {
	Controller  *domain.TimelineProjectionControllerV1
	Records     runtimeports.EvidenceSourceRecordReaderV2
	Current     runtimeports.EvidenceSubjectCurrentReaderV1
	Consumer    runtimeports.ProviderBindingRefV2
	Policy      TimelineProjectionPolicyCurrentReaderV1
	OwnerRouter TimelineTypedOwnerRouterV1
	// OwnerRouterV2 is the only production-capable typed Owner route because
	// its exact request includes TenantID. OwnerRouter is compatibility-only.
	OwnerRouterV2 *continuityports.ClosedTimelineTypedOwnerRouterV2
	Clock         func() time.Time
}

// InspectAttempt is the only recovery action after an indeterminate/lost
// publish reply. It never starts a second attempt or republishes an Event.
func (a *TimelineProjectionAdapterV1) InspectAttempt(ctx context.Context, ref contract.TimelineProjectionAttemptRefV1) (contract.TimelineProjectionAttemptFactV1, error) {
	if a == nil || a.Controller == nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.NewError(contract.ErrUnavailable, "timeline_adapter", "controller is unavailable")
	}
	return a.Controller.InspectAttempt(ctx, ref)
}

func (a *TimelineProjectionAdapterV1) Project(ctx context.Context, request contract.TimelineProjectionRequestV1) (contract.TimelineProjectionAttemptFactV1, contract.TimelineProjectionCurrentV1, error) {
	if a == nil || a.Controller == nil || nilOrTypedNil(a.Records) || nilOrTypedNil(a.Current) || nilOrTypedNil(a.Policy) || a.Clock == nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrUnavailable, "timeline_adapter", "required public readers are unavailable")
	}
	if err := request.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	attempt, duplicate, err := a.Controller.CreateAttempt(ctx, request)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	if duplicate {
		if attempt.State == contract.TimelineAttemptVisibleV1 && attempt.Event != nil {
			current, err := a.Controller.InspectCurrent(ctx, *attempt.Event)
			return attempt, current, err
		}
		return attempt, contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrRevisionConflict, "attempt", "existing attempt is not a completed visible result; Inspect it")
	}
	attempt, err = a.Controller.BeginInspection(ctx, attempt.Ref)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}

	s1, err := a.inspectStable(ctx, request)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, mapRuntimeError(err, "s1")
	}
	s2, err := a.inspectStable(ctx, request)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, mapRuntimeError(err, "s2")
	}
	if !reflect.DeepEqual(s1, s2) {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrProjectionConflict, "s1_s2", "current projections drifted during admission")
	}
	if err := a.validateFresh(ctx, s2); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, mapRuntimeError(err, "fresh")
	}
	event, err := timelineEventFromRuntimeV1(request, s2.record, s2.current.Projection)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	bindings := domain.TimelineAdmissionBindingsV1{
		EvidenceProjectionRef:      digestRuntimeRef("EvidenceSubjectProjectionRefV1", s2.current.Projection.Ref),
		EvidenceProjectionDigest:   string(s2.current.Projection.ProjectionDigest),
		EvidenceCurrentIndexRef:    digestRuntimeRef("EvidenceSubjectCurrentIndexRefV1", s2.current.CurrentIndex),
		EvidenceCurrentIndexDigest: string(s2.current.CurrentIndex.Digest),
		PolicyProjectionDigest:     s2.policy.Ref.Digest,
		CheckedUnixNano:            maxInt64(s2.current.Projection.CheckedUnixNano, s2.policy.CheckedUnixNano),
		NaturalNotAfterUnixNano:    minInt64(s2.current.Projection.ExpiresUnixNano, s2.policy.ExpiresUnixNano),
	}
	if s2.owner != nil {
		bindings.OwnerProjectionDigest = s2.owner.Digest
		bindings.CheckedUnixNano = maxInt64(bindings.CheckedUnixNano, s2.owner.CheckedUnixNano)
		bindings.NaturalNotAfterUnixNano = minInt64(bindings.NaturalNotAfterUnixNano, s2.owner.ExpiresUnixNano)
	}
	attempt, err = a.Controller.Admit(ctx, attempt.Ref, bindings)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	visible, current, err := a.Controller.Publish(ctx, attempt.Ref, event)
	if err != nil && contract.HasCode(mapRuntimeError(err, "publish"), contract.ErrIndeterminate) {
		_, _ = a.Controller.RequireReconcile(ctx, attempt.Ref)
	}
	return visible, current, err
}

// Rebuild replays only coordinate-only requests. Every item uses the same
// Attempt/S1/S2/fresh/atomic path as Project; there is no bulk Event import or
// scope replacement operation.
func (a *TimelineProjectionAdapterV1) Rebuild(ctx context.Context, requests []contract.TimelineProjectionRequestV1) ([]contract.TimelineProjectionAttemptFactV1, error) {
	results := make([]contract.TimelineProjectionAttemptFactV1, 0, len(requests))
	for _, request := range requests {
		attempt, _, err := a.Project(ctx, request)
		if err != nil {
			return results, err
		}
		results = append(results, attempt)
	}
	return results, nil
}

type stableTimelineInspectionV1 struct {
	record  runtimeports.EvidenceLedgerRecordV2
	current runtimeports.EvidenceSubjectCurrentSnapshotV1
	policy  TimelineProjectionPolicyCurrentV1
	owner   *TimelineOwnerCurrentProjectionV1
	reader  TimelineTypedOwnerCurrentReaderV2
	request TimelineOwnerCurrentInspectRequestV2
}

func (a *TimelineProjectionAdapterV1) inspectStable(ctx context.Context, request contract.TimelineProjectionRequestV1) (stableTimelineInspectionV1, error) {
	source := runtimeports.EvidenceSourceKeyV2{RegistrationID: request.EvidenceSource.RegistrationID, SourceEpoch: runtimecore.Epoch(request.EvidenceSource.SourceEpoch), SourceSequence: request.EvidenceSource.SourceSequence}
	bySource, err := a.Records.InspectBySource(ctx, source)
	if err != nil {
		return stableTimelineInspectionV1{}, err
	}
	if err := bySource.Validate(); err != nil {
		return stableTimelineInspectionV1{}, err
	}
	byRef, err := a.Records.InspectRecord(ctx, bySource.Ref)
	if err != nil {
		return stableTimelineInspectionV1{}, err
	}
	if err := byRef.Validate(); err != nil || !reflect.DeepEqual(bySource, byRef) {
		if err != nil {
			return stableTimelineInspectionV1{}, err
		}
		return stableTimelineInspectionV1{}, contract.NewError(contract.ErrEvidenceConflict, "record", "source and exact record reads disagree")
	}
	if request.ExpectedRecord != nil && (string(byRef.Ref.LedgerScopeDigest) != request.ExpectedRecord.LedgerScopeDigest || byRef.Ref.Sequence != request.ExpectedRecord.Sequence || string(byRef.Ref.RecordDigest) != request.ExpectedRecord.RecordDigest) {
		return stableTimelineInspectionV1{}, contract.NewError(contract.ErrEvidenceConflict, "expected_record", "exact record expectation drifted")
	}
	subject := runtimeports.EvidenceSubjectKeyV1{Record: byRef.Ref, Source: source}
	lookup := runtimeports.EvidenceSubjectCurrentLookupRequestV1{ContractVersion: runtimeports.EvidenceSubjectCurrentContractVersionV1, Subject: subject, ExpectedConsumer: a.Consumer, ExpectedExecutionScopeDigest: runtimecore.Digest(request.ScopeDigest), ExpectedSourcePolicy: byRef.Candidate.SourcePolicy}
	if err := lookup.Validate(); err != nil {
		return stableTimelineInspectionV1{}, err
	}
	current, err := a.Current.InspectEvidenceSubjectCurrentV1(ctx, lookup)
	if err != nil {
		return stableTimelineInspectionV1{}, err
	}
	if err := current.Validate(); err != nil || current.Projection.Record != byRef.Ref || current.Projection.Source != source || current.Projection.CandidateDigest != byRef.CandidateDigest {
		if err != nil {
			return stableTimelineInspectionV1{}, err
		}
		return stableTimelineInspectionV1{}, contract.NewError(contract.ErrEvidenceConflict, "current", "subject current does not bind the exact record")
	}
	policy, err := a.Policy.InspectTimelineProjectionPolicyCurrentV1(ctx, request.ProjectionPolicy, request.ScopeDigest)
	if err != nil {
		return stableTimelineInspectionV1{}, err
	}
	if err := validatePolicyProjection(policy, request); err != nil {
		return stableTimelineInspectionV1{}, err
	}
	result := stableTimelineInspectionV1{record: byRef, current: current, policy: policy}
	if byRef.Candidate.TrustClass == runtimeports.EvidenceTrustAuthoritativeFact {
		if request.OwnerFact == nil || byRef.Candidate.OwnerFact == nil || nilOrTypedNil(a.OwnerRouterV2) {
			return stableTimelineInspectionV1{}, contract.NewError(contract.ErrUnsupported, "owner_router", "authoritative evidence requires a typed Owner current reader")
		}
		if !sameRuntimeOwnerFact(request.OwnerFact, byRef.Candidate.OwnerFact, request.ScopeDigest) {
			return stableTimelineInspectionV1{}, contract.NewError(contract.ErrEvidenceConflict, "owner_fact", "Runtime owner fact differs from request")
		}
		ownerRequest := TimelineOwnerCurrentInspectRequestV2{
			TenantID: byRef.Candidate.ExecutionScope.Identity.TenantID,
			Fact:     *request.OwnerFact,
		}
		if err := ownerRequest.Validate(); err != nil {
			return stableTimelineInspectionV1{}, err
		}
		reader, err := a.OwnerRouterV2.ReaderForTimelineOwnerV2(ownerRequest)
		if err != nil || nilOrTypedNil(reader) {
			if err != nil {
				return stableTimelineInspectionV1{}, err
			}
			return stableTimelineInspectionV1{}, contract.NewError(contract.ErrUnsupported, "owner_reader", "typed Owner current reader is unavailable")
		}
		owner, err := reader.InspectTimelineOwnerCurrentV2(ctx, ownerRequest)
		if err != nil {
			return stableTimelineInspectionV1{}, err
		}
		if owner.Fact != *request.OwnerFact || owner.Digest == "" || owner.CheckedUnixNano <= 0 || owner.ExpiresUnixNano <= owner.CheckedUnixNano {
			return stableTimelineInspectionV1{}, contract.NewError(contract.ErrEvidenceConflict, "owner_current", "typed Owner current projection drifted")
		}
		result.owner, result.reader, result.request = &owner, reader, ownerRequest
	} else if request.OwnerFact != nil || byRef.Candidate.OwnerFact != nil {
		return stableTimelineInspectionV1{}, contract.NewError(contract.ErrEvidenceConflict, "owner_fact", "non-authoritative evidence cannot carry an Owner fact")
	}
	return result, nil
}

func (a *TimelineProjectionAdapterV1) validateFresh(ctx context.Context, s stableTimelineInspectionV1) error {
	now := a.Clock()
	if now.IsZero() {
		return contract.NewError(contract.ErrUnavailable, "clock", "fresh time is unavailable")
	}
	p := s.current.Projection
	request := runtimeports.EvidenceSubjectCurrentValidationRequestV1{ContractVersion: runtimeports.EvidenceSubjectCurrentContractVersionV1, Subject: p.Subject, ExpectedProjection: p.Ref, ExpectedCurrentIndex: s.current.CurrentIndex, ExpectedRegistration: p.Registration, ExpectedReaderBinding: p.ReaderBinding, ExpectedReaderCapability: p.ReaderCapability, ExpectedConsumer: a.Consumer, ExpectedExecutionScopeDigest: p.ExecutionScopeDigest, ExpectedSourcePolicy: p.SourcePolicy}
	if err := request.Validate(); err != nil {
		return err
	}
	validated, err := a.Current.ValidateEvidenceSubjectCurrentV1(ctx, request)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(validated, s.current) {
		return contract.NewError(contract.ErrProjectionConflict, "current", "fresh validation returned another projection")
	}
	if err := a.Policy.ValidateTimelineProjectionPolicyCurrentV1(ctx, s.policy); err != nil {
		return err
	}
	if s.owner != nil {
		if nilOrTypedNil(s.reader) {
			return contract.NewError(contract.ErrUnsupported, "owner_reader", "typed Owner current reader disappeared")
		}
		if err := s.reader.ValidateTimelineOwnerCurrentV2(ctx, s.request, *s.owner); err != nil {
			return err
		}
	}
	if now.UnixNano() < maxInt64(p.CheckedUnixNano, s.policy.CheckedUnixNano) || now.UnixNano() >= minInt64(p.ExpiresUnixNano, s.policy.ExpiresUnixNano) {
		return contract.NewError(contract.ErrPreconditionFailed, "current_ttl", "current projection is outside its natural window")
	}
	return nil
}

func timelineEventFromRuntimeV1(request contract.TimelineProjectionRequestV1, record runtimeports.EvidenceLedgerRecordV2, current runtimeports.EvidenceSubjectCurrentProjectionV1) (contract.TimelineEventRecord, error) {
	trust, err := mapTrustClass(record.Candidate.TrustClass)
	if err != nil {
		return contract.TimelineEventRecord{}, err
	}
	scope := record.Candidate.ExecutionScope
	projectedScope := contract.Scope{TenantID: string(scope.Identity.TenantID), IdentityID: string(scope.Identity.ID), IdentityEpoch: uint64(scope.Identity.Epoch), LineageID: string(scope.Lineage.ID), PlanDigest: string(scope.Lineage.PlanDigest), InstanceID: string(scope.Instance.ID), InstanceEpoch: uint64(scope.Instance.Epoch), AuthorityEpoch: uint64(scope.AuthorityEpoch), ExecutionScopeDigest: string(current.ExecutionScopeDigest)}
	if scope.SandboxLease != nil {
		projectedScope.SandboxLeaseID, projectedScope.SandboxLeaseEpoch = string(scope.SandboxLease.ID), uint64(scope.SandboxLease.Epoch)
	}
	if current.ExecutionScopeCurrent.ActiveRunID != "" {
		projectedScope.RunID, projectedScope.RunIdentityDigest = string(current.ExecutionScopeCurrent.ActiveRunID), string(current.ExecutionScopeCurrent.RunSource.Digest)
	}
	owner := contract.OwnerBinding{BindingSetID: record.Candidate.Producer.BindingSetID, BindingRevision: uint64(record.Candidate.Producer.BindingSetRevision), ComponentID: string(record.Candidate.Producer.ComponentID), ManifestDigest: string(record.Candidate.Producer.ManifestDigest), ArtifactDigest: string(record.Candidate.Producer.ArtifactDigest), Capability: string(record.Candidate.Producer.Capability), FactKind: string(record.Candidate.EventKind)}
	evidence := contract.EvidenceAdmission{RecordRef: string(record.Ref.RecordDigest), LedgerScopeDigest: string(record.Ref.LedgerScopeDigest), LedgerSequence: record.Ref.Sequence, RecordDigest: string(record.Ref.RecordDigest), SourceKey: request.EvidenceSource, TrustClass: trust, ObservedUnixNano: record.Candidate.ObservedUnixNano, RecordedUnixNano: record.IngestedUnixNano, PayloadRef: record.Candidate.Payload.Ref, PayloadSchema: record.Candidate.Payload.Schema.Key(), PayloadDigest: string(record.Candidate.Payload.ContentDigest), PayloadRevision: uint64(record.Candidate.Payload.Revision), AdmittedByLedger: true, InspectedByOwner: true}
	candidate := contract.TimelineProjectionCandidate{ContractVersion: contract.ContractVersion, CandidateID: record.Candidate.EventID, Revision: 1, Scope: projectedScope, Owner: owner, Evidence: evidence, SemanticKind: string(record.Candidate.EventKind), CustomClass: string(record.Candidate.CustomClass), ParentRefs: []string{}, CausationRefs: []string{}, CorrelationID: record.Candidate.CorrelationID, ObjectRefs: []string{record.Candidate.Payload.Ref}, ProjectionPolicyRef: request.ProjectionPolicy}
	for _, ref := range record.Candidate.Causation {
		candidate.CausationRefs = append(candidate.CausationRefs, digestRuntimeRef("EvidenceCausationRefV2", ref))
	}
	if trust == contract.TrustAuthoritativeFact {
		candidate.OwnerFactExactRef = request.OwnerFact
	}
	digest, err := candidate.CanonicalDigest()
	if err != nil {
		return contract.TimelineEventRecord{}, err
	}
	candidate.Digest = digest
	event := contract.TimelineEventRecord{Candidate: candidate, EvidenceRecordRef: evidence.RecordRef, LedgerScopeDigest: evidence.LedgerScopeDigest, LedgerSequence: evidence.LedgerSequence, EvidenceRecordDigest: evidence.RecordDigest, TrustClass: trust, ProjectionRevision: 1, Visibility: "visible"}
	return event, event.Validate()
}

func mapTrustClass(value runtimeports.EvidenceTrustClassV2) (contract.TrustClass, error) {
	switch value {
	case runtimeports.EvidenceTrustObservation:
		return contract.TrustObservation, nil
	case runtimeports.EvidenceTrustLateObservation:
		return contract.TrustLateObservation, nil
	case runtimeports.EvidenceTrustReceipt:
		return contract.TrustReceipt, nil
	case runtimeports.EvidenceTrustAttestation:
		return contract.TrustAttestation, nil
	case runtimeports.EvidenceTrustClaim:
		return contract.TrustClaim, nil
	case runtimeports.EvidenceTrustAuthoritativeFact:
		return contract.TrustAuthoritativeFact, nil
	default:
		return "", contract.NewError(contract.ErrInvalidArgument, "trust_class", "unknown Runtime trust class")
	}
}

func sameRuntimeOwnerFact(expected *contract.TimelineOwnerFactRefV1, actual *runtimeports.EvidenceOwnerFactRefV2, scope string) bool {
	return expected != nil && actual != nil && expected.ScopeDigest == scope && expected.Owner.BindingSetID == actual.Owner.BindingSetID && expected.Owner.BindingRevision == uint64(actual.Owner.BindingSetRevision) && expected.Owner.ComponentID == string(actual.Owner.ComponentID) && expected.Owner.ManifestDigest == string(actual.Owner.ManifestDigest) && expected.Owner.ArtifactDigest == string(actual.Owner.ArtifactDigest) && expected.Owner.Capability == string(actual.Owner.Capability) && expected.FactKind == string(actual.FactKind) && expected.FactID == actual.FactID && expected.Revision == uint64(actual.Revision) && expected.FactDigest == string(actual.FactDigest) && expected.PayloadSchema == actual.PayloadSchema.Key() && expected.PayloadDigest == string(actual.PayloadDigest) && expected.PayloadRevision == uint64(actual.PayloadRevision)
}

func validatePolicyProjection(p TimelineProjectionPolicyCurrentV1, request contract.TimelineProjectionRequestV1) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Ref.PolicyID != request.ProjectionPolicy || p.Ref.ScopeDigest != request.ScopeDigest || p.State != contract.TimelineProjectionPolicyActiveV1 {
		return contract.NewError(contract.ErrProjectionConflict, "policy_current", "policy projection is incomplete or scope-spliced")
	}
	return nil
}

func digestRuntimeRef(kind string, value any) string {
	digest, err := runtimecore.CanonicalJSONDigest("praxis.continuity.runtime-adapter", contract.TimelineGovernanceContractVersionV1, kind, value)
	if err != nil {
		return ""
	}
	return string(digest)
}

func mapRuntimeError(err error, field string) error {
	if err == nil {
		return nil
	}
	for category, code := range map[runtimecore.ErrorCategory]contract.ErrorCode{runtimecore.ErrorInvalidArgument: contract.ErrInvalidArgument, runtimecore.ErrorNotFound: contract.ErrNotFound, runtimecore.ErrorConflict: contract.ErrProjectionConflict, runtimecore.ErrorPreconditionFailed: contract.ErrPreconditionFailed, runtimecore.ErrorForbidden: contract.ErrPreconditionFailed, runtimecore.ErrorUnauthenticated: contract.ErrPreconditionFailed, runtimecore.ErrorCapabilityUnavailable: contract.ErrUnsupported, runtimecore.ErrorUnavailable: contract.ErrUnavailable, runtimecore.ErrorRateLimited: contract.ErrUnavailable, runtimecore.ErrorIndeterminate: contract.ErrIndeterminate, runtimecore.ErrorInternal: contract.ErrIndeterminate} {
		if runtimecore.HasCategory(err, category) {
			return contract.NewError(code, field, "Runtime public reader rejected the operation")
		}
	}
	return err
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func nilOrTypedNil(value any) bool {
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

package action

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type OwnerCurrentReaderV1 interface {
	InspectOwnerCurrentV1(context.Context, contract.OwnerCurrentRefV1) (contract.OwnerCurrentRefV1, error)
}

type ProviderObservationExactReaderV1 interface {
	InspectProviderObservationExactV1(context.Context, runtimeports.ProviderAttemptObservationRefV2) (runtimeports.ProviderAttemptObservationRefV2, error)
}

type EnforcementPhaseExactReaderV1 interface {
	InspectEnforcementPhaseExactV1(context.Context, runtimeports.OperationDispatchEnforcementPhaseRefV4) (runtimeports.OperationDispatchEnforcementPhaseRefV4, error)
}

type EvidenceConsumptionExactReaderV1 interface {
	InspectEvidenceConsumptionExactV1(context.Context, runtimeports.OperationScopeEvidenceConsumptionRefV3) (runtimeports.OperationScopeEvidenceConsumptionRefV3, error)
}

type CausalReadersV1 struct {
	OwnerCurrent OwnerCurrentReaderV1
	Observation  ProviderObservationExactReaderV1
	Enforcement  EnforcementPhaseExactReaderV1
	Consumption  EvidenceConsumptionExactReaderV1
}

type RecordV2 struct {
	Candidate    contract.ActionCandidateV2          `json:"candidate"`
	Reservation  *contract.ActionReservationFactV2   `json:"reservation,omitempty"`
	DomainResult *contract.ToolDomainResultFactV2    `json:"domain_result,omitempty"`
	Apply        *contract.ToolApplySettlementFactV2 `json:"apply,omitempty"`
	Result       *contract.ToolResultV2              `json:"result,omitempty"`
	Revision     core.Revision                       `json:"revision"`
}

type StoreV2 struct {
	mu          sync.RWMutex
	records     map[string]RecordV2
	domainIndex map[string]string
	readers     CausalReadersV1
}

func NewStoreV2(readers CausalReadersV1) *StoreV2 {
	return &StoreV2{records: make(map[string]RecordV2), domainIndex: make(map[string]string), readers: readers}
}

func (s *StoreV2) PutCandidateV2(candidate contract.ActionCandidateV2) (RecordV2, error) {
	if err := candidate.Validate(); err != nil {
		return RecordV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.records[candidate.ID]; ok {
		if existing.Candidate.Digest == candidate.Digest {
			return cloneRecordV2(existing), nil
		}
		return RecordV2{}, conflictV2("ActionCandidate ID already binds different content")
	}
	record := RecordV2{Candidate: cloneCandidateV2(candidate), Revision: 1}
	s.records[candidate.ID] = cloneRecordV2(record)
	return record, nil
}

func (s *StoreV2) InspectCandidateCurrentV2(ctx context.Context, exact contract.ObjectRef, now time.Time) (contract.ActionCandidateV2, error) {
	if exact.Validate() != nil || now.IsZero() {
		return contract.ActionCandidateV2{}, invalidV2("exact candidate ref and current time are required")
	}
	s.mu.RLock()
	record, ok := s.records[exact.ID]
	s.mu.RUnlock()
	if !ok {
		return contract.ActionCandidateV2{}, notFoundV2("ActionCandidate not found")
	}
	if record.Candidate.Revision != exact.Revision || record.Candidate.Digest != exact.Digest {
		return contract.ActionCandidateV2{}, conflictV2("ActionCandidate exact ref drifted")
	}
	if now.UnixNano() < record.Candidate.CreatedUnixNano || now.UnixNano() >= record.Candidate.CurrentExpiresUnixNano() {
		return contract.ActionCandidateV2{}, expiredV2("ActionCandidate is not current")
	}
	if s.readers.OwnerCurrent == nil {
		return contract.ActionCandidateV2{}, unavailableV2("Owner current reader is unavailable")
	}
	refs := []contract.OwnerCurrentRefV1{record.Candidate.PendingActionCurrent, record.Candidate.SurfaceCurrent, record.Candidate.CapabilityCurrent, record.Candidate.ToolCurrent, record.Candidate.InputSchemaCurrent, record.Candidate.SourceCandidateCurrent}
	for _, expected := range refs {
		actual, err := s.readers.OwnerCurrent.InspectOwnerCurrentV1(ctx, expected)
		if err != nil {
			return contract.ActionCandidateV2{}, err
		}
		if !reflect.DeepEqual(actual, expected) || actual.Validate(now) != nil {
			return contract.ActionCandidateV2{}, conflictV2("owner current projection drifted")
		}
	}
	return cloneCandidateV2(record.Candidate), nil
}

func (s *StoreV2) ReserveV2(ctx context.Context, action contract.ObjectRef, appAttempt contract.ApplicationAttemptRefV1, intentDigest core.Digest, sessionRef string, subjectDigest core.Digest, now, expires time.Time) (contract.ActionReservationFactV2, error) {
	candidate, err := s.InspectCandidateCurrentV2(ctx, action, now)
	if err != nil {
		return contract.ActionReservationFactV2{}, err
	}
	if appAttempt.Validate() != nil || intentDigest.Validate() != nil || subjectDigest.Validate() != nil || expires.IsZero() || !expires.After(now) || expires.UnixNano() > candidate.CurrentExpiresUnixNano() || sessionRef != candidate.SessionID {
		return contract.ActionReservationFactV2{}, invalidV2("reservation exact bindings or lifetime are invalid")
	}
	id, err := contract.StableID("reservation-v2", candidate.ID, appAttempt.ID, string(appAttempt.Digest), string(intentDigest))
	if err != nil {
		return contract.ActionReservationFactV2{}, err
	}
	fact, err := contract.SealActionReservationFactV2(contract.ActionReservationFactV2{ID: id, TenantID: candidate.TenantID, Action: action, ApplicationAttempt: appAttempt, IntentDigest: intentDigest, SessionRef: sessionRef, DomainSubjectDigest: subjectDigest, ReservedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		return contract.ActionReservationFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[action.ID]
	if !ok || record.Candidate.Revision != action.Revision || record.Candidate.Digest != action.Digest {
		return contract.ActionReservationFactV2{}, conflictV2("candidate changed before reservation CAS")
	}
	if record.Reservation != nil {
		if record.Reservation.Digest == fact.Digest {
			return *record.Reservation, nil
		}
		return contract.ActionReservationFactV2{}, conflictV2("reservation create-once content mismatch")
	}
	if record.DomainResult != nil || record.Apply != nil || record.Result != nil {
		return contract.ActionReservationFactV2{}, conflictV2("reservation cannot be inserted after later facts")
	}
	record.Reservation = &fact
	record.Revision++
	s.records[action.ID] = cloneRecordV2(record)
	return fact, nil
}

func (s *StoreV2) InspectReservationV2(actionID string, exact contract.ObjectRef) (contract.ActionReservationFactV2, error) {
	if exact.Validate() != nil {
		return contract.ActionReservationFactV2{}, invalidV2("exact reservation ref is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[actionID]
	if !ok || record.Reservation == nil {
		return contract.ActionReservationFactV2{}, notFoundV2("reservation not found")
	}
	if record.Reservation.ID != exact.ID || record.Reservation.Revision != exact.Revision || record.Reservation.Digest != exact.Digest {
		return contract.ActionReservationFactV2{}, conflictV2("reservation exact ref drifted")
	}
	return *record.Reservation, nil
}

func (s *StoreV2) PutDomainResultV2(ctx context.Context, fact contract.ToolDomainResultFactV2) (contract.ToolDomainResultFactV2, error) {
	if err := fact.Validate(); err != nil {
		return contract.ToolDomainResultFactV2{}, err
	}
	if err := s.rereadDomainCausality(ctx, fact); err != nil {
		return contract.ToolDomainResultFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[fact.Action.ID]
	if !ok || record.Reservation == nil {
		return contract.ToolDomainResultFactV2{}, notFoundV2("candidate or reservation not found")
	}
	if record.Candidate.Revision != fact.Action.Revision || record.Candidate.Digest != fact.Action.Digest || record.Reservation.ID != fact.Reservation.ID || record.Reservation.Revision != fact.Reservation.Revision || record.Reservation.Digest != fact.Reservation.Digest || record.Reservation.ApplicationAttempt != fact.ApplicationAttempt || record.Candidate.TenantID != fact.TenantID || record.Candidate.OperationScopeDigest != fact.OperationScopeDigest || record.Candidate.ExpectedOwner != fact.Owner {
		return contract.ToolDomainResultFactV2{}, conflictV2("DomainResult causal chain drifted")
	}
	if record.DomainResult != nil {
		if record.DomainResult.Digest == fact.Digest {
			return cloneDomainResultV2(*record.DomainResult), nil
		}
		return contract.ToolDomainResultFactV2{}, conflictV2("DomainResult create-once content mismatch")
	}
	copy := cloneDomainResultV2(fact)
	record.DomainResult = &copy
	record.Revision++
	s.records[fact.Action.ID] = cloneRecordV2(record)
	s.domainIndex[fact.ID] = fact.Action.ID
	return copy, nil
}

// InspectDomainResultByExactV2 resolves an exact Tool DomainResult without
// requiring callers to duplicate the Tool-owned action index.
func (s *StoreV2) InspectDomainResultByExactV2(exact contract.ObjectRef) (contract.ToolDomainResultFactV2, error) {
	if exact.Validate() != nil {
		return contract.ToolDomainResultFactV2{}, invalidV2("exact DomainResult ref is required")
	}
	s.mu.RLock()
	actionID, ok := s.domainIndex[exact.ID]
	s.mu.RUnlock()
	if !ok {
		return contract.ToolDomainResultFactV2{}, notFoundV2("DomainResult not found")
	}
	return s.InspectDomainResultV2(actionID, exact)
}

func (s *StoreV2) InspectDomainResultCurrentByExactV1(ctx context.Context, exact contract.ObjectRef, now time.Time, ttl time.Duration) (contract.ToolDomainResultCurrentProjectionV1, error) {
	if exact.Validate() != nil {
		return contract.ToolDomainResultCurrentProjectionV1{}, invalidV2("exact DomainResult ref is required")
	}
	s.mu.RLock()
	actionID, ok := s.domainIndex[exact.ID]
	s.mu.RUnlock()
	if !ok {
		return contract.ToolDomainResultCurrentProjectionV1{}, notFoundV2("DomainResult not found")
	}
	return s.InspectDomainResultCurrentV1(ctx, actionID, exact, now, ttl)
}

func (s *StoreV2) InspectDomainResultV2(actionID string, exact contract.ObjectRef) (contract.ToolDomainResultFactV2, error) {
	if exact.Validate() != nil {
		return contract.ToolDomainResultFactV2{}, invalidV2("exact DomainResult ref is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[actionID]
	if !ok || record.DomainResult == nil {
		return contract.ToolDomainResultFactV2{}, notFoundV2("DomainResult not found")
	}
	if record.DomainResult.ID != exact.ID || record.DomainResult.Revision != exact.Revision || record.DomainResult.Digest != exact.Digest {
		return contract.ToolDomainResultFactV2{}, conflictV2("DomainResult exact ref drifted")
	}
	return cloneDomainResultV2(*record.DomainResult), nil
}

func (s *StoreV2) InspectDomainResultCurrentV1(ctx context.Context, actionID string, exact contract.ObjectRef, now time.Time, ttl time.Duration) (contract.ToolDomainResultCurrentProjectionV1, error) {
	if ttl <= 0 || ttl > contract.MaxDomainResultCurrentTTLV1 || now.IsZero() {
		return contract.ToolDomainResultCurrentProjectionV1{}, invalidV2("DomainResult lease must be positive and at most 30 seconds")
	}
	fact, err := s.InspectDomainResultV2(actionID, exact)
	if err != nil {
		return contract.ToolDomainResultCurrentProjectionV1{}, err
	}
	if err = s.rereadDomainCausality(ctx, fact); err != nil {
		return contract.ToolDomainResultCurrentProjectionV1{}, err
	}
	p := contract.ToolDomainResultCurrentProjectionV1{ContractVersion: contract.ResultContractVersionV2, Fact: exact, CausalityDigest: fact.Causality.Digest, Observation: fact.Observation, PrepareEnforcement: fact.PrepareEnforcement, ExecuteEnforcement: fact.ExecuteEnforcement, PrepareConsumption: fact.PrepareConsumption, ExecuteConsumption: fact.ExecuteConsumption, Owner: fact.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(ttl).UnixNano()}
	p.Digest, err = p.ComputeDigest()
	if err != nil {
		return contract.ToolDomainResultCurrentProjectionV1{}, err
	}
	return p, p.Validate(now)
}

func (s *StoreV2) rereadDomainCausality(ctx context.Context, f contract.ToolDomainResultFactV2) error {
	if s.readers.Observation == nil || s.readers.Enforcement == nil || s.readers.Consumption == nil {
		return unavailableV2("DomainResult causal readers are unavailable")
	}
	ob, e := s.readers.Observation.InspectProviderObservationExactV1(ctx, f.Observation)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(ob, f.Observation) {
		return conflictV2("Provider Observation exact ref drifted")
	}
	for _, expected := range []runtimeports.OperationDispatchEnforcementPhaseRefV4{f.PrepareEnforcement, f.ExecuteEnforcement} {
		actual, e := s.readers.Enforcement.InspectEnforcementPhaseExactV1(ctx, expected)
		if e != nil {
			return e
		}
		if !reflect.DeepEqual(actual, expected) {
			return conflictV2("Enforcement phase exact ref drifted")
		}
	}
	for _, expected := range []runtimeports.OperationScopeEvidenceConsumptionRefV3{f.PrepareConsumption, f.ExecuteConsumption} {
		actual, e := s.readers.Consumption.InspectEvidenceConsumptionExactV1(ctx, expected)
		if e != nil {
			return e
		}
		if !reflect.DeepEqual(actual, expected) {
			return conflictV2("Evidence Consumption exact ref drifted")
		}
	}
	return nil
}

func (s *StoreV2) ApplySettlementV2(actionID string, domainResult contract.ObjectRef, inspection runtimeports.OperationInspectionSettlementRefV4, outcome contract.ToolOutcomeV2, disposition contract.ToolDispositionV2, now time.Time) (contract.ToolResultV2, error) {
	if now.IsZero() || inspection.Validate(now) != nil || contract.ValidateToolOutcomeDispositionV2(outcome, disposition) != nil {
		return contract.ToolResultV2{}, invalidV2("fresh Runtime V4 inspection and legal Tool outcome are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[actionID]
	if !ok || record.Reservation == nil || record.DomainResult == nil {
		return contract.ToolResultV2{}, notFoundV2("Tool DomainResult not found")
	}
	f := record.DomainResult
	if f.ID != domainResult.ID || f.Revision != domainResult.Revision || f.Digest != domainResult.Digest {
		return contract.ToolResultV2{}, conflictV2("Tool DomainResult exact ref drifted")
	}
	rdr := inspection.DomainResult
	if rdr.ID != f.ID || rdr.Revision != f.Revision || rdr.Digest != f.Digest || rdr.TenantID != f.TenantID || rdr.OperationDigest != f.Causality.OperationDigest || !reflect.DeepEqual(rdr.Attempt, f.Causality.Attempt) || rdr.Schema != f.Schema || rdr.PayloadDigest != f.PayloadDigest || rdr.PayloadRevision != f.PayloadRevision || inspection.Owner != f.Owner {
		return contract.ToolResultV2{}, conflictV2("Runtime V4 inspection does not close the exact Tool DomainResult")
	}
	if record.Result != nil {
		if record.Result.Inspection.Digest == inspection.Digest && record.Result.Outcome == outcome && record.Result.Disposition == disposition {
			return cloneToolResultV2(*record.Result), nil
		}
		return contract.ToolResultV2{}, conflictV2("Tool settlement already binds different content")
	}
	applyID, e := contract.StableID("tool-apply-v2", actionID, f.ID, string(inspection.Digest))
	if e != nil {
		return contract.ToolResultV2{}, e
	}
	apply, e := contract.SealToolApplySettlementFactV2(contract.ToolApplySettlementFactV2{ID: applyID, TenantID: f.TenantID, OperationScopeDigest: f.OperationScopeDigest, Action: f.Action, Reservation: f.Reservation, DomainResult: domainResult, Inspection: inspection, Outcome: outcome, Disposition: disposition, Owner: f.Owner, AppliedUnixNano: now.UnixNano()})
	if e != nil {
		return contract.ToolResultV2{}, e
	}
	resultID, e := contract.StableID("tool-result-v2", actionID, f.ID, apply.ID, string(apply.Digest))
	if e != nil {
		return contract.ToolResultV2{}, e
	}
	result, e := contract.SealToolResultV2(contract.ToolResultV2{ID: resultID, Action: f.Action, Reservation: f.Reservation, DomainResult: domainResult, Apply: contract.ObjectRef{ID: apply.ID, Revision: apply.Revision, Digest: apply.Digest}, Inspection: inspection, Outcome: outcome, Disposition: disposition, Schema: f.Schema, PayloadDigest: f.PayloadDigest, PayloadRevision: f.PayloadRevision, Residuals: append([]contract.Residual(nil), f.Residuals...), FinalizedUnixNano: now.UnixNano()})
	if e != nil {
		return contract.ToolResultV2{}, e
	}
	record.Apply = &apply
	record.Result = &result
	record.Revision++
	s.records[actionID] = cloneRecordV2(record)
	return cloneToolResultV2(result), nil
}

func (s *StoreV2) InspectResultV2(actionID string, exact contract.ObjectRef) (contract.ToolResultV2, error) {
	if exact.Validate() != nil {
		return contract.ToolResultV2{}, invalidV2("exact ToolResult ref is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.records[actionID]
	if !ok || r.Result == nil {
		return contract.ToolResultV2{}, notFoundV2("ToolResult not found")
	}
	if r.Result.ID != exact.ID || r.Result.Revision != exact.Revision || r.Result.Digest != exact.Digest {
		return contract.ToolResultV2{}, conflictV2("ToolResult exact ref drifted")
	}
	return cloneToolResultV2(*r.Result), nil
}

// InspectSettledResultForApplyV2 resolves the immutable result associated with
// an exact ApplySettlement fact. It is the crash-recovery read after Tool Apply.
func (s *StoreV2) InspectSettledResultForApplyV2(actionID string, apply contract.ObjectRef) (contract.ToolResultV2, error) {
	if apply.Validate() != nil {
		return contract.ToolResultV2{}, invalidV2("exact ApplySettlement ref is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[actionID]
	if !ok || record.Apply == nil || record.Result == nil {
		return contract.ToolResultV2{}, notFoundV2("settled ToolResult not found")
	}
	if record.Apply.ID != apply.ID || record.Apply.Revision != apply.Revision || record.Apply.Digest != apply.Digest || record.Result.Apply != apply {
		return contract.ToolResultV2{}, conflictV2("ApplySettlement exact ref drifted")
	}
	return cloneToolResultV2(*record.Result), nil
}

func cloneCandidateV2(c contract.ActionCandidateV2) contract.ActionCandidateV2 {
	c.Payload.Inline = append([]byte(nil), c.Payload.Inline...)
	return c
}
func cloneDomainResultV2(f contract.ToolDomainResultFactV2) contract.ToolDomainResultFactV2 {
	f.Residuals = append([]contract.Residual(nil), f.Residuals...)
	return f
}
func cloneToolResultV2(r contract.ToolResultV2) contract.ToolResultV2 {
	r.Artifacts = append([]contract.ObjectRef(nil), r.Artifacts...)
	r.Residuals = append([]contract.Residual(nil), r.Residuals...)
	return r
}
func cloneRecordV2(r RecordV2) RecordV2 {
	r.Candidate = cloneCandidateV2(r.Candidate)
	if r.Reservation != nil {
		x := *r.Reservation
		r.Reservation = &x
	}
	if r.DomainResult != nil {
		x := cloneDomainResultV2(*r.DomainResult)
		r.DomainResult = &x
	}
	if r.Apply != nil {
		x := *r.Apply
		r.Apply = &x
	}
	if r.Result != nil {
		x := cloneToolResultV2(*r.Result)
		r.Result = &x
	}
	return r
}
func invalidV2(m string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, m)
}
func conflictV2(m string) error {
	return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, m)
}
func notFoundV2(m string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, m)
}
func expiredV2(m string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, m)
}
func unavailableV2(m string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, m)
}

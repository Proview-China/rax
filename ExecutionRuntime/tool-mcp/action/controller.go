package action

import (
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type State string

const (
	StateCandidate    State = "candidate"
	StateReserved     State = "reserved"
	StateDomainResult State = "domain_result_ready"
	StateSettled      State = "settled"
)

type Record struct {
	Candidate    contract.ActionCandidate        `json:"candidate"`
	Reservation  *contract.ActionReservationFact `json:"reservation,omitempty"`
	DomainResult *contract.DomainResultFact      `json:"domain_result,omitempty"`
	Result       *contract.ToolResult            `json:"result,omitempty"`
	State        State                           `json:"state"`
	Revision     core.Revision                   `json:"revision"`
}

type Controller struct {
	mu      sync.RWMutex
	records map[string]Record
	pending map[string]pendingBinding
}

type pendingBinding struct {
	RequestDigest    core.Digest
	ProjectionDigest core.Digest
	ActionID         string
	CandidateDigest  core.Digest
}

func NewController() *Controller {
	return &Controller{records: make(map[string]Record), pending: make(map[string]pendingBinding)}
}

func (c *Controller) PutCandidate(candidate contract.ActionCandidate, expectedPendingActionDigest core.Digest) (Record, error) {
	if err := candidate.Validate(); err != nil {
		return Record{}, err
	}
	if err := expectedPendingActionDigest.Validate(); err != nil {
		return Record{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidDigest, "expected Harness PendingAction RequestDigest is required")
	}
	if candidate.PendingActionDigest != expectedPendingActionDigest {
		return Record{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "action candidate binds a different Harness PendingAction RequestDigest")
	}
	projectionDigest, err := candidate.PendingActionProjectionDigest()
	if err != nil {
		return Record{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.records[candidate.ID]; ok {
		if existing.Candidate.Digest == candidate.Digest {
			return cloneRecord(existing), nil
		}
		return Record{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "action id already binds another candidate")
	}
	if existing, ok := c.pending[candidate.PendingActionRef]; ok {
		if existing.RequestDigest != candidate.PendingActionDigest || existing.ProjectionDigest != projectionDigest || existing.ActionID != candidate.ID || existing.CandidateDigest != candidate.Digest {
			return Record{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Harness PendingAction ref already binds different digest, payload, capability or source candidate content")
		}
	}
	record := Record{Candidate: cloneCandidate(candidate), State: StateCandidate, Revision: 1}
	c.records[candidate.ID] = cloneRecord(record)
	c.pending[candidate.PendingActionRef] = pendingBinding{RequestDigest: candidate.PendingActionDigest, ProjectionDigest: projectionDigest, ActionID: candidate.ID, CandidateDigest: candidate.Digest}
	return cloneRecord(record), nil
}

func (c *Controller) Reserve(actionID string, applicationAttemptDigest, intentDigest, domainSubjectDigest core.Digest, sessionRef string, now, expires time.Time) (Record, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	record, ok := c.records[actionID]
	if !ok {
		return Record{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "action candidate not found")
	}
	if err := validateCandidateCurrent(record.Candidate, now); err != nil {
		return Record{}, err
	}
	if expires.IsZero() || !expires.After(now) {
		return Record{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Reservation expiry must be after current time")
	}
	if expires.UTC().UnixNano() > record.Candidate.ExpiresUnixNano {
		return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Reservation cannot outlive its ActionCandidate")
	}
	id, err := contract.StableID("reservation", record.Candidate.ID, string(applicationAttemptDigest), string(intentDigest))
	if err != nil {
		return Record{}, err
	}
	fact, err := contract.SealReservation(contract.ActionReservationFact{
		ID: id, Action: contract.ObjectRef{ID: record.Candidate.ID, Revision: record.Candidate.Revision, Digest: record.Candidate.Digest}, ApplicationAttemptDigest: applicationAttemptDigest,
		IntentDigest: intentDigest, SessionRef: sessionRef, DomainSubjectDigest: domainSubjectDigest, ReservedUnixNano: now.UTC().UnixNano(), ExpiresUnixNano: expires.UTC().UnixNano(),
	})
	if err != nil {
		return Record{}, err
	}
	if record.Reservation != nil {
		if record.Reservation.Digest == fact.Digest {
			return cloneRecord(record), nil
		}
		return Record{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "action reservation differs from create-once fact")
	}
	if record.State != StateCandidate {
		return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "action cannot be reserved from current state")
	}
	record.Reservation = &fact
	record.State = StateReserved
	record.Revision++
	c.records[actionID] = cloneRecord(record)
	return cloneRecord(record), nil
}

func validateCandidateCurrent(candidate contract.ActionCandidate, now time.Time) error {
	if now.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "ActionCandidate currentness requires current time")
	}
	currentUnixNano := now.UTC().UnixNano()
	if currentUnixNano < candidate.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "ActionCandidate is not yet current")
	}
	if currentUnixNano >= candidate.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "expired ActionCandidate cannot be reserved or retried")
	}
	return nil
}

func (c *Controller) RecordDomainResult(actionID, attemptID string, observationDigest core.Digest, payload runtimeports.OpaquePayloadV2, residuals []contract.Residual, now time.Time) (Record, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	record, ok := c.records[actionID]
	if !ok {
		return Record{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "action candidate not found")
	}
	id, err := contract.StableID("domain-result", actionID, attemptID, string(observationDigest), string(payload.ContentDigest))
	if err != nil {
		return Record{}, err
	}
	fact, err := contract.SealDomainResult(contract.DomainResultFact{
		ID: id, Action: contract.ObjectRef{ID: record.Candidate.ID, Revision: record.Candidate.Revision, Digest: record.Candidate.Digest}, AttemptID: attemptID,
		ObservationDigest: observationDigest, Payload: payload, Residuals: append([]contract.Residual(nil), residuals...), CreatedUnixNano: now.UTC().UnixNano(),
	})
	if err != nil {
		return Record{}, err
	}
	if record.DomainResult != nil {
		if record.DomainResult.Digest == fact.Digest {
			return cloneRecord(record), nil
		}
		return Record{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "attempt already binds a different domain result")
	}
	if record.State != StateReserved {
		return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "domain result requires a reservation")
	}
	record.DomainResult = &fact
	record.State = StateDomainResult
	record.Revision++
	c.records[actionID] = cloneRecord(record)
	return cloneRecord(record), nil
}

func (c *Controller) ApplySettlement(actionID string, settlement runtimeports.OperationSettlementRefV3, now time.Time) (Record, error) {
	if err := settlement.Validate(); err != nil {
		return Record{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	record, ok := c.records[actionID]
	if !ok {
		return Record{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "action candidate not found")
	}
	if record.Result != nil {
		if record.Result.Settlement.Digest == settlement.Digest {
			return cloneRecord(record), nil
		}
		return Record{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "action already settled with another runtime settlement")
	}
	if record.State != StateDomainResult || record.DomainResult == nil {
		return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "domain result must exist before ApplySettlement")
	}
	if settlement.Attempt.AttemptID != record.DomainResult.AttemptID || settlement.Owner != record.Candidate.ExpectedOwner {
		return Record{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "runtime settlement belongs to another attempt or owner")
	}
	if settlement.DomainResultSchema == nil || *settlement.DomainResultSchema != record.DomainResult.Payload.Schema || settlement.DomainResultDigest != record.DomainResult.Payload.ContentDigest {
		return Record{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "runtime settlement does not reference the exact pre-existing domain result")
	}
	id, err := contract.StableID("tool-result", actionID, settlement.ID, string(settlement.Digest), string(record.DomainResult.Digest))
	if err != nil {
		return Record{}, err
	}
	result, err := contract.SealToolResult(contract.ToolResult{
		ID: id, Action: contract.ObjectRef{ID: record.Candidate.ID, Revision: record.Candidate.Revision, Digest: record.Candidate.Digest},
		DomainResult: contract.ObjectRef{ID: record.DomainResult.ID, Revision: record.DomainResult.Revision, Digest: record.DomainResult.Digest}, Settlement: settlement, FinalizedUnixNano: now.UTC().UnixNano(),
	})
	if err != nil {
		return Record{}, err
	}
	record.Result = &result
	record.State = StateSettled
	record.Revision++
	c.records[actionID] = cloneRecord(record)
	return cloneRecord(record), nil
}

func (c *Controller) Inspect(actionID string) (Record, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	record, ok := c.records[actionID]
	return cloneRecord(record), ok
}

// InspectReservation is the mandatory exact recovery path when Reserve may
// have committed but its reply was lost. It never creates or advances state.
func (c *Controller) InspectReservation(actionID, reservationID string, reservationDigest core.Digest) (contract.ActionReservationFact, error) {
	if contract.ValidateStableID(reservationID) != nil || reservationDigest.Validate() != nil {
		return contract.ActionReservationFact{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "exact reservation id and digest are required")
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	record, ok := c.records[actionID]
	if !ok || record.Reservation == nil {
		return contract.ActionReservationFact{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "action reservation not found")
	}
	if record.Reservation.ID != reservationID || record.Reservation.Digest != reservationDigest {
		return contract.ActionReservationFact{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "action reservation differs from exact inspect key")
	}
	return *record.Reservation, nil
}

func cloneRecord(record Record) Record {
	record.Candidate = cloneCandidate(record.Candidate)
	if record.Reservation != nil {
		value := *record.Reservation
		record.Reservation = &value
	}
	if record.DomainResult != nil {
		value := *record.DomainResult
		value.Residuals = append([]contract.Residual(nil), value.Residuals...)
		record.DomainResult = &value
	}
	if record.Result != nil {
		value := *record.Result
		value.Settlement.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), value.Settlement.Evidence...)
		if value.Settlement.DomainResultSchema != nil {
			schema := *value.Settlement.DomainResultSchema
			value.Settlement.DomainResultSchema = &schema
		}
		record.Result = &value
	}
	return record
}

func cloneCandidate(value contract.ActionCandidate) contract.ActionCandidate {
	value.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), value.EffectKinds...)
	value.Payload.Inline = append([]byte(nil), value.Payload.Inline...)
	return value
}

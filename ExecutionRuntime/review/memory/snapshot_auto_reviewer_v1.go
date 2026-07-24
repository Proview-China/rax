package memory

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// AutoReviewerSnapshotV1 is optional so snapshots created before the Auto
// Reviewer implementation remain readable without changing their digest.
type AutoReviewerSnapshotV1 struct {
	Current      map[string]contract.AutoReviewerAttemptV1                   `json:"current"`
	History      map[string]map[core.Revision]contract.AutoReviewerAttemptV1 `json:"history"`
	Idempotency  map[string]string                                           `json:"idempotency"`
	Observations map[string]contract.AutoReviewerInvocationObservationV1     `json:"observations"`
}

func emptyAutoReviewerSnapshotV1() *AutoReviewerSnapshotV1 {
	return &AutoReviewerSnapshotV1{Current: map[string]contract.AutoReviewerAttemptV1{}, History: map[string]map[core.Revision]contract.AutoReviewerAttemptV1{}, Idempotency: map[string]string{}, Observations: map[string]contract.AutoReviewerInvocationObservationV1{}}
}

func (snapshot *AutoReviewerSnapshotV1) validate(tenant core.TenantID, results map[string]contract.ReviewerInvocationResultFactV1) error {
	if snapshot == nil {
		return nil
	}
	if snapshot.Current == nil || snapshot.History == nil || snapshot.Idempotency == nil || snapshot.Observations == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "auto reviewer snapshot maps must be present")
	}
	if err := validateCurrentHistory(tenant, snapshot.Current, snapshot.History, func(v contract.AutoReviewerAttemptV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, func(v contract.AutoReviewerAttemptV1) error { return v.Validate() }, "auto reviewer attempt"); err != nil {
		return err
	}
	for idempotency, attemptID := range snapshot.Idempotency {
		attempt, ok := snapshot.Current[attemptID]
		if !ok || attempt.IdempotencyKey != idempotency {
			return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "auto reviewer snapshot idempotency index drifted")
		}
	}
	if len(snapshot.Idempotency) != len(snapshot.Current) {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "auto reviewer snapshot idempotency index is incomplete")
	}
	seenObservations := make(map[string]struct{})
	for id, revisions := range snapshot.History {
		for _, attempt := range revisions {
			if attempt.Observation == nil {
				continue
			}
			observation, ok := snapshot.Observations[attempt.Observation.ID]
			if !ok || observation.Revision != attempt.Observation.Revision || observation.Digest != attempt.Observation.Digest || observation.AttemptID != id || attempt.InvocationAttempt == nil || observation.AttemptRevision != attempt.InvocationAttempt.Revision || observation.AttemptDigest != attempt.InvocationAttempt.Digest {
				return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto reviewer snapshot Observation provenance drifted")
			}
			source, ok := revisions[attempt.InvocationAttempt.Revision]
			if !ok || source.Digest != attempt.InvocationAttempt.Digest || source.State != contract.AutoReviewerAttemptPreparedV1 {
				return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto reviewer snapshot original invocation Attempt is missing")
			}
			seenObservations[observation.ID] = struct{}{}
			if attempt.DomainResult == nil {
				return core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "observed auto reviewer attempt has no DomainResult")
			}
			result, ok := results[attempt.DomainResult.ID]
			if !ok || result.Revision != attempt.DomainResult.Revision || result.Digest != attempt.DomainResult.Digest || result.AttemptID != observation.RuntimeAttempt.AttemptID || result.ResultDigest != observation.Output.Digest {
				return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto reviewer snapshot DomainResult provenance drifted")
			}
		}
	}
	if len(seenObservations) != len(snapshot.Observations) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto reviewer snapshot contains an unreferenced Observation")
	}
	for id, observation := range snapshot.Observations {
		if err := validateSnapshotFact(tenant, id, observation.FactIdentityV1, observation.Validate(), "auto reviewer Observation"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) exportAutoReviewerSnapshotV1(tenant core.TenantID) *AutoReviewerSnapshotV1 {
	prefix := string(tenant) + "\x00"
	copyID := func(value string) string { return strings.TrimPrefix(value, prefix) }
	out := emptyAutoReviewerSnapshotV1()
	for itemKey, value := range s.autoReviewerAttempts {
		if strings.HasPrefix(itemKey, prefix) {
			out.Current[copyID(itemKey)], _ = clone(value)
		}
	}
	for itemKey, value := range s.autoReviewerHistory {
		if strings.HasPrefix(itemKey, prefix) {
			out.History[copyID(itemKey)], _ = clone(value)
		}
	}
	for itemKey, value := range s.autoReviewerKeys {
		if strings.HasPrefix(itemKey, prefix) {
			out.Idempotency[copyID(itemKey)] = value
		}
	}
	for itemKey, value := range s.autoReviewerObservations {
		if strings.HasPrefix(itemKey, prefix) {
			out.Observations[copyID(itemKey)], _ = clone(value)
		}
	}
	if len(out.Current) == 0 && len(out.History) == 0 && len(out.Idempotency) == 0 && len(out.Observations) == 0 {
		return nil
	}
	return out
}

func (s *Store) importAutoReviewerSnapshotV1(tenant core.TenantID, snapshot *AutoReviewerSnapshotV1) {
	if snapshot == nil {
		return
	}
	for id, value := range snapshot.Current {
		s.autoReviewerAttempts[key(tenant, id)], _ = clone(value)
	}
	for id, value := range snapshot.History {
		s.autoReviewerHistory[key(tenant, id)], _ = clone(value)
	}
	for idempotency, attemptID := range snapshot.Idempotency {
		s.autoReviewerKeys[key(tenant, idempotency)] = attemptID
	}
	for id, value := range snapshot.Observations {
		s.autoReviewerObservations[key(tenant, id)], _ = clone(value)
	}
}

package contract

import "sort"

const HostJournalContractVersionV2 = "praxis.agent-host/journal/v2"

type HostPhaseV2 string

const (
	HostAcceptedV2              HostPhaseV2 = "accepted"
	HostValidatingV2            HostPhaseV2 = "validating"
	HostResolvingV2             HostPhaseV2 = "resolving"
	HostCompilingV2             HostPhaseV2 = "compiling"
	HostBindingV2               HostPhaseV2 = "binding"
	HostConstructingControlV2   HostPhaseV2 = "constructing_control"
	HostActivatingV2            HostPhaseV2 = "activating"
	HostAssociatingGenerationV2 HostPhaseV2 = "associating_generation"
	HostVerifyingV2             HostPhaseV2 = "verifying"
	HostReadyV2                 HostPhaseV2 = "ready"
	HostDrainingV2              HostPhaseV2 = "draining"
	HostReconcilingV2           HostPhaseV2 = "reconciling"
	HostClosedV2                HostPhaseV2 = "closed"
	HostIndeterminateV2         HostPhaseV2 = "indeterminate"
)

type HostOperationStateV2 string

const (
	HostOperationIntentRecordedV2         HostOperationStateV2 = "intent_recorded"
	HostOperationResultRecordedV2         HostOperationStateV2 = "result_recorded"
	HostOperationOutcomeUnknownV2         HostOperationStateV2 = "outcome_unknown"
	HostOperationReconciliationRequiredV2 HostOperationStateV2 = "reconciliation_required"
)

// HostOperationCoordinateV2 is a journal-only exact coordinate. ContractKind
// is the nominal public contract discriminator; this record never upgrades the
// referenced object to Host authority.
type HostOperationCoordinateV2 struct {
	ContractKind    string   `json:"contract_kind"`
	OwnerID         string   `json:"owner_id"`
	ID              string   `json:"id"`
	Revision        uint64   `json:"revision"`
	Digest          DigestV1 `json:"digest"`
	Current         bool     `json:"current"`
	ExpiresUnixNano int64    `json:"expires_unix_nano,omitempty"`
}

func (r HostOperationCoordinateV2) Validate() error {
	for field, value := range map[string]string{"operation coordinate contract kind": r.ContractKind, "operation coordinate owner": r.OwnerID, "operation coordinate id": r.ID} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	if r.Revision == 0 {
		return NewError(ErrorInvalidArgument, "operation_coordinate_revision_invalid", "operation coordinate revision must be positive")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	if r.Current && r.ExpiresUnixNano <= 0 {
		return NewError(ErrorInvalidArgument, "operation_coordinate_expiry_missing", "current operation coordinate requires expiry")
	}
	if !r.Current && r.ExpiresUnixNano != 0 {
		return NewError(ErrorInvalidArgument, "operation_coordinate_expiry_forbidden", "immutable operation coordinate cannot carry current expiry")
	}
	return nil
}

func hostOperationCoordinateKeyV2(r HostOperationCoordinateV2) string {
	return r.ContractKind + "\x00" + r.OwnerID + "\x00" + r.ID
}

type HostOperationAttemptV2 struct {
	ContractVersion string                      `json:"contract_version"`
	AttemptID       string                      `json:"attempt_id"`
	Revision        uint64                      `json:"revision"`
	OperationKind   string                      `json:"operation_kind"`
	Phase           HostPhaseV2                 `json:"phase"`
	RequestDigest   DigestV1                    `json:"request_digest"`
	Inputs          []HostOperationCoordinateV2 `json:"inputs"`
	State           HostOperationStateV2        `json:"state"`
	Result          *HostOperationCoordinateV2  `json:"result,omitempty"`
	CreatedUnixNano int64                       `json:"created_unix_nano"`
	UpdatedUnixNano int64                       `json:"updated_unix_nano"`
	Digest          DigestV1                    `json:"digest"`
}

func (a HostOperationAttemptV2) canonicalV2() HostOperationAttemptV2 {
	clone := a
	clone.Inputs = append([]HostOperationCoordinateV2{}, a.Inputs...)
	sort.Slice(clone.Inputs, func(i, j int) bool {
		return hostOperationCoordinateKeyV2(clone.Inputs[i]) < hostOperationCoordinateKeyV2(clone.Inputs[j])
	})
	if a.Result != nil {
		value := *a.Result
		clone.Result = &value
	}
	return clone
}

func (a HostOperationAttemptV2) digestV2() (DigestV1, error) {
	clone := a.canonicalV2()
	clone.Digest = ""
	return DigestJSONV1(struct {
		Domain string                 `json:"domain"`
		Type   string                 `json:"type"`
		Body   HostOperationAttemptV2 `json:"body"`
	}{Domain: "praxis.agent-host.journal-v2", Type: "HostOperationAttemptV2", Body: clone})
}

func SealHostOperationAttemptV2(a HostOperationAttemptV2) (HostOperationAttemptV2, error) {
	a = a.canonicalV2()
	provided := a.Digest
	digest, err := a.digestV2()
	if err != nil {
		return HostOperationAttemptV2{}, err
	}
	if provided != "" && provided != digest {
		return HostOperationAttemptV2{}, NewError(ErrorConflict, "host_operation_attempt_drift", "Host operation attempt supplied a wrong non-zero digest")
	}
	a.Digest = digest
	if err := a.Validate(); err != nil {
		return HostOperationAttemptV2{}, err
	}
	return a, nil
}

func (a HostOperationAttemptV2) Validate() error {
	if a.ContractVersion != HostJournalContractVersionV2 || a.Revision == 0 || a.CreatedUnixNano <= 0 || a.UpdatedUnixNano < a.CreatedUnixNano || len(a.Inputs) == 0 {
		return NewError(ErrorInvalidArgument, "host_operation_attempt_incomplete", "Host operation attempt is incomplete")
	}
	if err := ValidateIdentifierV1("host operation attempt id", a.AttemptID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("host operation kind", a.OperationKind); err != nil {
		return err
	}
	if !validHostPhaseV2(a.Phase) {
		return NewError(ErrorInvalidArgument, "host_phase_invalid", "HostV2 phase is unsupported")
	}
	if err := a.RequestDigest.Validate(); err != nil {
		return err
	}
	for index, input := range a.Inputs {
		if err := input.Validate(); err != nil {
			return err
		}
		if index > 0 && hostOperationCoordinateKeyV2(a.Inputs[index-1]) >= hostOperationCoordinateKeyV2(input) {
			return NewError(ErrorConflict, "host_operation_input_duplicate", "Host operation inputs must be sorted and unique")
		}
	}
	switch a.State {
	case HostOperationIntentRecordedV2, HostOperationOutcomeUnknownV2, HostOperationReconciliationRequiredV2:
		if a.Result != nil {
			return NewError(ErrorPrecondition, "host_operation_result_too_early", "non-terminal Host operation cannot carry a result")
		}
	case HostOperationResultRecordedV2:
		if a.Result == nil {
			return NewError(ErrorPrecondition, "host_operation_result_missing", "settled Host operation requires an exact result")
		}
		if err := a.Result.Validate(); err != nil {
			return err
		}
	default:
		return NewError(ErrorInvalidArgument, "host_operation_state_invalid", "Host operation state is unsupported")
	}
	expected, err := a.digestV2()
	if err != nil {
		return err
	}
	if expected != a.Digest {
		return NewError(ErrorConflict, "host_operation_attempt_drift", "Host operation attempt digest drifted")
	}
	return nil
}

func ValidateHostOperationAttemptSuccessorV2(current, next HostOperationAttemptV2) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.AttemptID != next.AttemptID || current.OperationKind != next.OperationKind || current.Phase != next.Phase || current.RequestDigest != next.RequestDigest || current.CreatedUnixNano != next.CreatedUnixNano || len(current.Inputs) != len(next.Inputs) {
		return NewError(ErrorConflict, "host_operation_attempt_identity_drift", "Host operation attempt identity drifted")
	}
	for i := range current.Inputs {
		if current.Inputs[i] != next.Inputs[i] {
			return NewError(ErrorConflict, "host_operation_input_drift", "Host operation exact inputs drifted")
		}
	}
	if next.Revision != current.Revision+1 || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return NewError(ErrorConflict, "host_operation_revision_drift", "Host operation successor must advance one revision")
	}
	allowed := current.State == HostOperationIntentRecordedV2 && (next.State == HostOperationResultRecordedV2 || next.State == HostOperationOutcomeUnknownV2) || current.State == HostOperationOutcomeUnknownV2 && (next.State == HostOperationResultRecordedV2 || next.State == HostOperationReconciliationRequiredV2) || current.State == HostOperationReconciliationRequiredV2 && (next.State == HostOperationResultRecordedV2 || next.State == HostOperationReconciliationRequiredV2)
	if !allowed {
		return NewError(ErrorPrecondition, "host_operation_transition_invalid", "Host operation transition is not allowed")
	}
	return nil
}

type HostJournalV2 struct {
	ContractVersion string                   `json:"contract_version"`
	HostID          string                   `json:"host_id"`
	StartID         string                   `json:"start_id"`
	Revision        uint64                   `json:"revision"`
	Phase           HostPhaseV2              `json:"phase"`
	StartClaimRef   ExactRefV1               `json:"start_claim_ref"`
	ConfigDigest    DigestV1                 `json:"config_digest"`
	Operations      []HostOperationAttemptV2 `json:"operations"`
	CreatedUnixNano int64                    `json:"created_unix_nano"`
	UpdatedUnixNano int64                    `json:"updated_unix_nano"`
	Digest          DigestV1                 `json:"digest"`
}

func (j HostJournalV2) digestV2() (DigestV1, error) {
	clone := j
	clone.Operations = append([]HostOperationAttemptV2{}, j.Operations...)
	clone.Digest = ""
	return DigestJSONV1(struct {
		Domain string        `json:"domain"`
		Type   string        `json:"type"`
		Body   HostJournalV2 `json:"body"`
	}{Domain: "praxis.agent-host.journal-v2", Type: "HostJournalV2", Body: clone})
}
func SealHostJournalV2(j HostJournalV2) (HostJournalV2, error) {
	j.Operations = append([]HostOperationAttemptV2{}, j.Operations...)
	provided := j.Digest
	digest, err := j.digestV2()
	if err != nil {
		return HostJournalV2{}, err
	}
	if provided != "" && provided != digest {
		return HostJournalV2{}, NewError(ErrorConflict, "host_journal_v2_drift", "HostV2 Journal supplied a wrong non-zero digest")
	}
	j.Digest = digest
	if err := j.Validate(); err != nil {
		return HostJournalV2{}, err
	}
	return j, nil
}
func (j HostJournalV2) Validate() error {
	if j.ContractVersion != HostJournalContractVersionV2 || j.Revision == 0 || j.CreatedUnixNano <= 0 || j.UpdatedUnixNano < j.CreatedUnixNano {
		return NewError(ErrorInvalidArgument, "host_journal_v2_incomplete", "HostV2 Journal is incomplete")
	}
	if err := ValidateIdentifierV1("host id", j.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("start id", j.StartID); err != nil {
		return err
	}
	if !validHostPhaseV2(j.Phase) {
		return NewError(ErrorInvalidArgument, "host_phase_invalid", "HostV2 phase is unsupported")
	}
	if err := j.StartClaimRef.Validate(); err != nil {
		return err
	}
	if err := j.ConfigDigest.Validate(); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, attempt := range j.Operations {
		if err := attempt.Validate(); err != nil {
			return err
		}
		if _, ok := seen[attempt.AttemptID]; ok {
			return NewError(ErrorConflict, "host_operation_attempt_duplicate", "HostV2 Journal duplicates an operation attempt")
		}
		seen[attempt.AttemptID] = struct{}{}
	}
	expected, err := j.digestV2()
	if err != nil {
		return err
	}
	if expected != j.Digest {
		return NewError(ErrorConflict, "host_journal_v2_drift", "HostV2 Journal digest drifted")
	}
	return nil
}
func (j HostJournalV2) RefV2() (ExactRefV1, error) {
	if err := j.Validate(); err != nil {
		return ExactRefV1{}, err
	}
	return ExactRefV1{Kind: "praxis.agent-host/journal-v2", ID: j.HostID + "/" + j.StartID, Revision: j.Revision, Digest: j.Digest}, nil
}

func ValidateHostJournalSuccessorV2(current, next HostJournalV2) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.HostID != next.HostID || current.StartID != next.StartID || current.StartClaimRef != next.StartClaimRef || current.ConfigDigest != next.ConfigDigest || current.CreatedUnixNano != next.CreatedUnixNano {
		return NewError(ErrorConflict, "host_journal_v2_identity_drift", "HostV2 Journal immutable identity drifted")
	}
	if next.Revision != current.Revision+1 || next.UpdatedUnixNano < current.UpdatedUnixNano || !allowedHostPhaseTransitionV2(current.Phase, next.Phase) {
		return NewError(ErrorPrecondition, "host_journal_v2_successor_invalid", "HostV2 Journal revision, clock or phase transition is invalid")
	}
	if len(next.Operations) < len(current.Operations) || len(next.Operations) > len(current.Operations)+1 {
		return NewError(ErrorConflict, "host_operation_history_drift", "HostV2 operation history must append at most one attempt")
	}
	for index := range current.Operations {
		if index == len(current.Operations)-1 && current.Operations[index].Digest != next.Operations[index].Digest {
			if err := ValidateHostOperationAttemptSuccessorV2(current.Operations[index], next.Operations[index]); err != nil {
				return err
			}
		} else if current.Operations[index].Digest != next.Operations[index].Digest {
			return NewError(ErrorConflict, "host_operation_history_drift", "settled HostV2 operation history is immutable")
		}
	}
	if len(next.Operations) == len(current.Operations)+1 && next.Operations[len(current.Operations)].State != HostOperationIntentRecordedV2 {
		return NewError(ErrorPrecondition, "host_operation_first_state_invalid", "new HostV2 operation must begin at intent_recorded")
	}
	if len(next.Operations) == len(current.Operations)+1 {
		if len(current.Operations) > 0 && current.Operations[len(current.Operations)-1].State != HostOperationResultRecordedV2 {
			return NewError(ErrorPrecondition, "host_operation_previous_unsettled", "HostV2 cannot append another operation before exact reconciliation")
		}
		if next.Operations[len(current.Operations)].Phase != next.Phase {
			return NewError(ErrorConflict, "host_operation_phase_drift", "new HostV2 operation phase must match the Journal phase")
		}
	} else if current.Phase != next.Phase && len(next.Operations) > 0 && next.Operations[len(next.Operations)-1].State != HostOperationResultRecordedV2 {
		return NewError(ErrorPrecondition, "host_phase_advanced_with_unsettled_operation", "HostV2 phase cannot advance with an unsettled operation")
	}
	return nil
}

func validHostPhaseV2(value HostPhaseV2) bool {
	switch value {
	case HostAcceptedV2, HostValidatingV2, HostResolvingV2, HostCompilingV2, HostBindingV2, HostConstructingControlV2, HostActivatingV2, HostAssociatingGenerationV2, HostVerifyingV2, HostReadyV2, HostDrainingV2, HostReconcilingV2, HostClosedV2, HostIndeterminateV2:
		return true
	}
	return false
}
func allowedHostPhaseTransitionV2(current, next HostPhaseV2) bool {
	if current == next && current != HostClosedV2 && current != HostIndeterminateV2 {
		return true
	}
	allowed := map[HostPhaseV2]HostPhaseV2{HostAcceptedV2: HostValidatingV2, HostValidatingV2: HostResolvingV2, HostResolvingV2: HostCompilingV2, HostCompilingV2: HostBindingV2, HostBindingV2: HostConstructingControlV2, HostConstructingControlV2: HostActivatingV2, HostActivatingV2: HostAssociatingGenerationV2, HostAssociatingGenerationV2: HostVerifyingV2, HostVerifyingV2: HostReadyV2, HostReadyV2: HostDrainingV2, HostDrainingV2: HostReconcilingV2, HostReconcilingV2: HostClosedV2}
	if allowed[current] == next {
		return true
	}
	// Stop may begin after any accepted Start phase because even a partially
	// completed activation can own resources that require the typed cleanup
	// DAG. The existing Journal remains the single phase linearization point.
	if next == HostDrainingV2 && hostActivePhaseV2(current) {
		return true
	}
	return next == HostIndeterminateV2 && current != HostClosedV2 || current == HostIndeterminateV2 && next == HostReconcilingV2
}

func hostActivePhaseV2(phase HostPhaseV2) bool {
	switch phase {
	case HostAcceptedV2, HostValidatingV2, HostResolvingV2, HostCompilingV2, HostBindingV2, HostConstructingControlV2, HostActivatingV2, HostAssociatingGenerationV2, HostVerifyingV2, HostReadyV2:
		return true
	default:
		return false
	}
}

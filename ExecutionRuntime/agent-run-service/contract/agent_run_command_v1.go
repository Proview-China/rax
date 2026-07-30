package contract

import "strings"

const (
	AgentRunCommandRefKindV1     = "praxis.agent-run-service/command"
	AgentRunHostStartClaimKindV1 = "praxis.agent-host/host-start-claim"
	AgentRunExecutionScopeKindV1 = "praxis.runtime/execution-scope"
	AgentRunCurrentKindV1        = "praxis.runtime/agent-run-current"
)

type AgentRunTargetV1 struct {
	HostID         string         `json:"host_id"`
	StartID        string         `json:"start_id"`
	HostStartClaim ExactRefWireV1 `json:"host_start_claim"`
	ExecutionScope ExactRefWireV1 `json:"execution_scope"`
	RunCurrent     ExactRefWireV1 `json:"run_current"`
	AuthorityEpoch WireUint64V1   `json:"authority_epoch"`
	TargetDigest   DigestV1       `json:"target_digest"`
}

func SealAgentRunTargetV1(target AgentRunTargetV1) (AgentRunTargetV1, error) {
	provided := target.TargetDigest
	target.TargetDigest = ""
	digest, err := target.digestV1()
	if err != nil {
		return AgentRunTargetV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentRunTargetV1{}, NewError(FaultRevisionConflictV1, "agent_run_target_digest_drift", "Agent Run target supplied a wrong digest")
	}
	target.TargetDigest = digest
	return target, target.Validate()
}

func (t AgentRunTargetV1) digestV1() (DigestV1, error) {
	clone := t
	clone.TargetDigest = ""
	return DigestJSONV1(struct {
		Domain string           `json:"domain"`
		Type   string           `json:"type"`
		Body   AgentRunTargetV1 `json:"body"`
	}{"praxis.agent-run-service.target", "AgentRunTargetV1", clone})
}

func (t AgentRunTargetV1) Validate() error {
	for field, value := range map[string]string{"host id": t.HostID, "start id": t.StartID} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	for _, ref := range []ExactRefWireV1{t.HostStartClaim, t.ExecutionScope, t.RunCurrent} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if t.HostStartClaim.Kind != AgentRunHostStartClaimKindV1 || t.ExecutionScope.Kind != AgentRunExecutionScopeKindV1 || t.RunCurrent.Kind != AgentRunCurrentKindV1 {
		return NewError(FaultPreconditionFailedV1, "agent_run_target_kind_drift", "Agent Run target exact ref kinds drifted")
	}
	if err := t.AuthorityEpoch.ValidatePositiveV1("Agent Run authority epoch"); err != nil {
		return err
	}
	digest, err := t.digestV1()
	if err != nil || digest != t.TargetDigest {
		return NewError(FaultRevisionConflictV1, "agent_run_target_digest_drift", "Agent Run target digest drifted")
	}
	return nil
}

type ExpectedCurrentV1 struct {
	Ref   ExactRefWireV1 `json:"ref"`
	Epoch WireUint64V1   `json:"epoch"`
}

func (e ExpectedCurrentV1) Validate() error {
	if err := e.Ref.Validate(); err != nil {
		return err
	}
	return e.Epoch.ValidatePositiveV1("expected current epoch")
}

type AgentRunCommandKindV1 string

const (
	AgentRunCommandCancelRunV1 AgentRunCommandKindV1 = "CANCEL_RUN"
	AgentRunCommandStopHostV1  AgentRunCommandKindV1 = "STOP_HOST"
)

type AgentRunCommandPayloadV1 struct {
	Reason         string          `json:"reason"`
	HostJournal    *ExactRefWireV1 `json:"host_journal,omitempty"`
	CleanupClosure *ExactRefWireV1 `json:"cleanup_closure,omitempty"`
}

func (p AgentRunCommandPayloadV1) validateForV1(kind AgentRunCommandKindV1) error {
	if strings.TrimSpace(p.Reason) == "" || p.Reason != strings.TrimSpace(p.Reason) {
		return NewError(FaultInvalidArgumentV1, "command_reason_invalid", "command reason is required")
	}
	for _, ref := range []*ExactRefWireV1{p.HostJournal, p.CleanupClosure} {
		if ref != nil {
			if err := ref.Validate(); err != nil {
				return err
			}
		}
	}
	switch kind {
	case AgentRunCommandCancelRunV1:
		if p.HostJournal != nil || p.CleanupClosure != nil {
			return NewError(FaultPreconditionFailedV1, "cancel_payload_host_fact_present", "Cancel cannot carry Host Stop facts")
		}
	case AgentRunCommandStopHostV1:
		if p.HostJournal == nil || p.CleanupClosure == nil {
			return NewError(FaultPreconditionFailedV1, "stop_payload_host_fact_missing", "Host Stop requires exact Journal and Cleanup Closure refs")
		}
	default:
		return NewError(FaultInvalidArgumentV1, "command_kind_invalid", "Agent Run command kind is invalid")
	}
	return nil
}

type AgentRunCommandEnvelopeV1 struct {
	ContractVersion        string                   `json:"contract_version"`
	CommandID              string                   `json:"command_id"`
	TraceID                string                   `json:"trace_id"`
	Kind                   AgentRunCommandKindV1    `json:"kind"`
	Target                 AgentRunTargetV1         `json:"target"`
	Actor                  string                   `json:"actor"`
	AuthorityRef           ExactRefWireV1           `json:"authority_ref"`
	Payload                AgentRunCommandPayloadV1 `json:"payload"`
	ExpectedCurrent        ExpectedCurrentV1        `json:"expected_current"`
	IdempotencyKey         string                   `json:"idempotency_key"`
	CanonicalPayloadDigest DigestV1                 `json:"canonical_payload_digest"`
	SubmittedAtUnixNano    WireUnixNanoV1           `json:"submitted_at_unix_nano"`
	NotAfterUnixNano       WireUnixNanoV1           `json:"not_after_unix_nano"`
	RequestDigest          DigestV1                 `json:"request_digest"`
}

func SealAgentRunCommandEnvelopeV1(envelope AgentRunCommandEnvelopeV1) (AgentRunCommandEnvelopeV1, error) {
	if envelope.ContractVersion != "" && envelope.ContractVersion != AgentRunServiceContractVersionV1 {
		return AgentRunCommandEnvelopeV1{}, NewError(FaultRevisionConflictV1, "agent_run_service_version_mismatch", "Agent Run command version drifted")
	}
	envelope.ContractVersion = AgentRunServiceContractVersionV1
	providedPayload := envelope.CanonicalPayloadDigest
	envelope.CanonicalPayloadDigest = ""
	payloadDigest, err := envelope.canonicalPayloadDigestV1()
	if err != nil {
		return AgentRunCommandEnvelopeV1{}, err
	}
	if providedPayload != "" && providedPayload != payloadDigest {
		return AgentRunCommandEnvelopeV1{}, NewError(FaultIdempotencyConflictV1, "canonical_payload_digest_drift", "command supplied a wrong canonical payload digest")
	}
	envelope.CanonicalPayloadDigest = payloadDigest
	providedRequest := envelope.RequestDigest
	envelope.RequestDigest = ""
	requestDigest, err := envelope.requestDigestV1()
	if err != nil {
		return AgentRunCommandEnvelopeV1{}, err
	}
	if providedRequest != "" && providedRequest != requestDigest {
		return AgentRunCommandEnvelopeV1{}, NewError(FaultRevisionConflictV1, "command_request_digest_drift", "command supplied a wrong request digest")
	}
	envelope.RequestDigest = requestDigest
	return envelope, envelope.Validate()
}

func (e AgentRunCommandEnvelopeV1) canonicalPayloadDigestV1() (DigestV1, error) {
	return DigestJSONV1(struct {
		Domain          string                   `json:"domain"`
		ContractVersion string                   `json:"contract_version"`
		Kind            AgentRunCommandKindV1    `json:"kind"`
		Target          AgentRunTargetV1         `json:"target"`
		Actor           string                   `json:"actor"`
		AuthorityRef    ExactRefWireV1           `json:"authority_ref"`
		Payload         AgentRunCommandPayloadV1 `json:"payload"`
		ExpectedCurrent ExpectedCurrentV1        `json:"expected_current"`
		NotAfter        WireUnixNanoV1           `json:"not_after_unix_nano"`
	}{"praxis.agent-run-service.command-payload", e.ContractVersion, e.Kind, e.Target, e.Actor, e.AuthorityRef, e.Payload, e.ExpectedCurrent, e.NotAfterUnixNano})
}

func (e AgentRunCommandEnvelopeV1) requestDigestV1() (DigestV1, error) {
	clone := e
	clone.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string                    `json:"domain"`
		Type   string                    `json:"type"`
		Body   AgentRunCommandEnvelopeV1 `json:"body"`
	}{"praxis.agent-run-service.command", "AgentRunCommandEnvelopeV1", clone})
}

func (e AgentRunCommandEnvelopeV1) Validate() error {
	if e.ContractVersion != AgentRunServiceContractVersionV1 {
		return NewError(FaultInvalidArgumentV1, "command_contract_version_invalid", "Agent Run command contract version is invalid")
	}
	for field, value := range map[string]string{"command id": e.CommandID, "trace id": e.TraceID, "actor": e.Actor, "idempotency key": e.IdempotencyKey} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	if err := e.Target.Validate(); err != nil {
		return err
	}
	if err := e.AuthorityRef.Validate(); err != nil {
		return err
	}
	if err := e.Payload.validateForV1(e.Kind); err != nil {
		return err
	}
	if err := e.ExpectedCurrent.Validate(); err != nil {
		return err
	}
	if e.ExpectedCurrent.Epoch != e.Target.AuthorityEpoch {
		return NewError(FaultRevisionConflictV1, "expected_epoch_drift", "expected current epoch differs from target authority epoch")
	}
	switch e.Kind {
	case AgentRunCommandCancelRunV1:
		if e.ExpectedCurrent.Ref != e.Target.RunCurrent {
			return NewError(FaultRevisionConflictV1, "cancel_expected_current_drift", "Cancel expected current differs from the exact Run current")
		}
	case AgentRunCommandStopHostV1:
		if e.Payload.HostJournal == nil || e.ExpectedCurrent.Ref != *e.Payload.HostJournal {
			return NewError(FaultRevisionConflictV1, "stop_expected_current_drift", "Stop expected current differs from the exact Host Journal")
		}
	default:
		return NewError(FaultInvalidArgumentV1, "command_kind_invalid", "Agent Run command kind is invalid")
	}
	if err := e.SubmittedAtUnixNano.ValidatePositiveV1("command submitted UnixNano"); err != nil {
		return err
	}
	if err := e.NotAfterUnixNano.ValidatePositiveV1("command not-after UnixNano"); err != nil {
		return err
	}
	submitted, _ := e.SubmittedAtUnixNano.Int64V1()
	notAfter, _ := e.NotAfterUnixNano.Int64V1()
	if notAfter <= submitted {
		return NewError(FaultInvalidArgumentV1, "command_window_invalid", "command not-after must be after submission")
	}
	payloadDigest, err := e.canonicalPayloadDigestV1()
	if err != nil || payloadDigest != e.CanonicalPayloadDigest {
		return NewError(FaultIdempotencyConflictV1, "canonical_payload_digest_drift", "command canonical payload digest drifted")
	}
	requestDigest, err := e.requestDigestV1()
	if err != nil || requestDigest != e.RequestDigest {
		return NewError(FaultRevisionConflictV1, "command_request_digest_drift", "command request digest drifted")
	}
	return nil
}

func (e AgentRunCommandEnvelopeV1) ValidateCurrentV1(now WireUnixNanoV1) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if err := now.ValidatePositiveV1("current UnixNano"); err != nil {
		return err
	}
	submitted, _ := e.SubmittedAtUnixNano.Int64V1()
	notAfter, _ := e.NotAfterUnixNano.Int64V1()
	current, _ := now.Int64V1()
	if current < submitted {
		return NewError(FaultPreconditionFailedV1, "clock_regression", "command was checked before submission")
	}
	if current >= notAfter {
		return NewError(FaultPreconditionFailedV1, "command_expired", "command reached or exceeded not-after")
	}
	return nil
}

func (e AgentRunCommandEnvelopeV1) CommandRefV1() (ExactRefWireV1, error) {
	if err := e.Validate(); err != nil {
		return ExactRefWireV1{}, err
	}
	return ExactRefWireV1{Kind: AgentRunCommandRefKindV1, ID: e.CommandID, Revision: NewWireUint64V1(1), Digest: e.RequestDigest}, nil
}

type IdempotencyClassificationV1 string

const (
	IdempotencyNewCommandV1     IdempotencyClassificationV1 = "NEW_COMMAND"
	IdempotencyReplayOriginalV1 IdempotencyClassificationV1 = "REPLAY_ORIGINAL_RECEIPT"
)

func ClassifyAgentRunCommandReplayV1(original, retry AgentRunCommandEnvelopeV1) (IdempotencyClassificationV1, error) {
	if err := original.Validate(); err != nil {
		return "", err
	}
	if err := retry.Validate(); err != nil {
		return "", err
	}
	if original.CommandID == retry.CommandID && original.IdempotencyKey != retry.IdempotencyKey {
		return "", NewError(FaultRevisionConflictV1, "command_identity_conflict", "same command ID was rebound to another idempotency key")
	}
	if original.IdempotencyKey != retry.IdempotencyKey {
		return IdempotencyNewCommandV1, nil
	}
	if original.CanonicalPayloadDigest != retry.CanonicalPayloadDigest {
		return "", NewError(FaultIdempotencyConflictV1, "idempotency_payload_conflict", "same idempotency key was reused with a different canonical payload")
	}
	return IdempotencyReplayOriginalV1, nil
}

type AgentRunCommandStatusV1 string

const (
	AgentRunCommandAcceptedV1      AgentRunCommandStatusV1 = "ACCEPTED"
	AgentRunCommandRejectedV1      AgentRunCommandStatusV1 = "REJECTED"
	AgentRunCommandExecutingV1     AgentRunCommandStatusV1 = "EXECUTING"
	AgentRunCommandCompletedV1     AgentRunCommandStatusV1 = "COMPLETED"
	AgentRunCommandIndeterminateV1 AgentRunCommandStatusV1 = "INDETERMINATE"
)

// AgentRunCommandReceiptV1 is only an Application/Host command receipt. Even
// COMPLETED does not assert a Runtime Run outcome or termination report.
type AgentRunCommandReceiptV1 struct {
	ContractVersion        string                  `json:"contract_version"`
	CommandRef             ExactRefWireV1          `json:"command_ref"`
	AttemptRef             *ExactRefWireV1         `json:"attempt_ref,omitempty"`
	CurrentRef             *ExactRefWireV1         `json:"current_ref,omitempty"`
	IdempotencyKey         string                  `json:"idempotency_key"`
	CanonicalPayloadDigest DigestV1                `json:"canonical_payload_digest"`
	OriginalRequestDigest  DigestV1                `json:"original_request_digest"`
	Status                 AgentRunCommandStatusV1 `json:"status"`
	Fault                  *FaultV1                `json:"fault,omitempty"`
	TraceID                string                  `json:"trace_id"`
	RecordedUnixNano       WireUnixNanoV1          `json:"recorded_unix_nano"`
	ReceiptDigest          DigestV1                `json:"receipt_digest"`
}

func SealAgentRunCommandReceiptV1(receipt AgentRunCommandReceiptV1) (AgentRunCommandReceiptV1, error) {
	if err := validateOptionalContractVersionV1(receipt.ContractVersion, AgentRunServiceContractVersionV1); err != nil {
		return AgentRunCommandReceiptV1{}, err
	}
	receipt.ContractVersion = AgentRunServiceContractVersionV1
	provided := receipt.ReceiptDigest
	receipt.ReceiptDigest = ""
	digest, err := receipt.digestV1()
	if err != nil {
		return AgentRunCommandReceiptV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentRunCommandReceiptV1{}, NewError(FaultRevisionConflictV1, "command_receipt_digest_drift", "command receipt supplied a wrong digest")
	}
	receipt.ReceiptDigest = digest
	return receipt, receipt.Validate()
}

func (r AgentRunCommandReceiptV1) digestV1() (DigestV1, error) {
	clone := r
	clone.ReceiptDigest = ""
	return DigestJSONV1(struct {
		Domain string                   `json:"domain"`
		Type   string                   `json:"type"`
		Body   AgentRunCommandReceiptV1 `json:"body"`
	}{"praxis.agent-run-service.command-receipt", "AgentRunCommandReceiptV1", clone})
}

func (r AgentRunCommandReceiptV1) Validate() error {
	if r.ContractVersion != AgentRunServiceContractVersionV1 {
		return NewError(FaultInvalidArgumentV1, "command_receipt_version_invalid", "command receipt contract version is invalid")
	}
	if err := r.CommandRef.Validate(); err != nil {
		return err
	}
	if r.CommandRef.Kind != AgentRunCommandRefKindV1 {
		return NewError(FaultPreconditionFailedV1, "command_receipt_ref_kind_drift", "receipt command ref kind drifted")
	}
	for _, ref := range []*ExactRefWireV1{r.AttemptRef, r.CurrentRef} {
		if ref != nil {
			if err := ref.Validate(); err != nil {
				return err
			}
		}
	}
	for field, value := range map[string]string{"idempotency key": r.IdempotencyKey, "trace id": r.TraceID} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	for _, digest := range []DigestV1{r.CanonicalPayloadDigest, r.OriginalRequestDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := r.RecordedUnixNano.ValidatePositiveV1("receipt recorded UnixNano"); err != nil {
		return err
	}
	switch r.Status {
	case AgentRunCommandAcceptedV1, AgentRunCommandExecutingV1, AgentRunCommandCompletedV1:
		if r.Fault != nil {
			return NewError(FaultPreconditionFailedV1, "successful_command_fault_present", "non-failed command receipt cannot carry a fault")
		}
	case AgentRunCommandRejectedV1:
		if r.Fault == nil {
			return NewError(FaultPreconditionFailedV1, "rejected_command_fault_missing", "rejected command receipt requires a public fault")
		}
	case AgentRunCommandIndeterminateV1:
		if r.Fault == nil || (r.Fault.Code != FaultUnknownOutcomeV1 && r.Fault.Code != FaultIndeterminateV1) {
			return NewError(FaultPreconditionFailedV1, "indeterminate_command_fault_missing", "indeterminate receipt requires UNKNOWN_OUTCOME or INDETERMINATE")
		}
	default:
		return NewError(FaultInvalidArgumentV1, "command_receipt_status_invalid", "command receipt status is invalid")
	}
	if r.Fault != nil {
		if err := r.Fault.Validate(); err != nil {
			return err
		}
		if r.Fault.TraceID != r.TraceID {
			return NewError(FaultRevisionConflictV1, "command_fault_trace_drift", "command fault trace differs from receipt")
		}
		if r.Fault.CommandRef != nil && *r.Fault.CommandRef != r.CommandRef {
			return NewError(FaultRevisionConflictV1, "command_fault_ref_drift", "command fault names another command")
		}
		if r.Fault.AttemptRef != nil && (r.AttemptRef == nil || *r.Fault.AttemptRef != *r.AttemptRef) {
			return NewError(FaultRevisionConflictV1, "attempt_fault_ref_drift", "command fault names another attempt")
		}
	}
	digest, err := r.digestV1()
	if err != nil || digest != r.ReceiptDigest {
		return NewError(FaultRevisionConflictV1, "command_receipt_digest_drift", "command receipt digest drifted")
	}
	return nil
}

func (r AgentRunCommandReceiptV1) ValidateFor(envelope AgentRunCommandEnvelopeV1) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	commandRef, _ := envelope.CommandRefV1()
	if r.CommandRef != commandRef || r.IdempotencyKey != envelope.IdempotencyKey || r.CanonicalPayloadDigest != envelope.CanonicalPayloadDigest || r.OriginalRequestDigest != envelope.RequestDigest || r.TraceID != envelope.TraceID {
		return NewError(FaultRevisionConflictV1, "command_receipt_request_drift", "command receipt does not bind the exact original command")
	}
	recorded, _ := r.RecordedUnixNano.Int64V1()
	submitted, _ := envelope.SubmittedAtUnixNano.Int64V1()
	notAfter, _ := envelope.NotAfterUnixNano.Int64V1()
	if recorded < submitted || recorded >= notAfter {
		return NewError(FaultPreconditionFailedV1, "command_receipt_time_outside_request", "command receipt was recorded outside the exact request interval")
	}
	return nil
}

func (r AgentRunCommandReceiptV1) ValidateForReplay(original, retry AgentRunCommandEnvelopeV1) error {
	classification, err := ClassifyAgentRunCommandReplayV1(original, retry)
	if err != nil {
		return err
	}
	if classification != IdempotencyReplayOriginalV1 {
		return NewError(FaultInvalidArgumentV1, "not_an_idempotent_replay", "retry does not reuse the original idempotency key")
	}
	return r.ValidateFor(original)
}

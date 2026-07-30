package contract

type InspectOriginalRequestV1 struct {
	ContractVersion        string          `json:"contract_version"`
	RequestID              string          `json:"request_id"`
	TraceID                string          `json:"trace_id"`
	OriginalCommandRef     *ExactRefWireV1 `json:"original_command_ref,omitempty"`
	OriginalAttemptRef     *ExactRefWireV1 `json:"original_attempt_ref,omitempty"`
	OriginalIdempotencyKey string          `json:"original_idempotency_key,omitempty"`
	OriginalRequestDigest  DigestV1        `json:"original_request_digest,omitempty"`
	RequestedUnixNano      WireUnixNanoV1  `json:"requested_unix_nano"`
	NotAfterUnixNano       WireUnixNanoV1  `json:"not_after_unix_nano"`
	RequestDigest          DigestV1        `json:"request_digest"`
}

func SealInspectOriginalRequestV1(request InspectOriginalRequestV1) (InspectOriginalRequestV1, error) {
	if request.ContractVersion != "" && request.ContractVersion != AgentRunServiceContractVersionV1 {
		return InspectOriginalRequestV1{}, NewError(FaultRevisionConflictV1, "agent_run_service_version_mismatch", "Inspect Original version drifted")
	}
	request.ContractVersion = AgentRunServiceContractVersionV1
	provided := request.RequestDigest
	request.RequestDigest = ""
	digest, err := request.digestV1()
	if err != nil {
		return InspectOriginalRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return InspectOriginalRequestV1{}, NewError(FaultRevisionConflictV1, "inspect_original_request_digest_drift", "Inspect Original request supplied a wrong digest")
	}
	request.RequestDigest = digest
	return request, request.Validate()
}

func (r InspectOriginalRequestV1) digestV1() (DigestV1, error) {
	clone := r
	clone.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string                   `json:"domain"`
		Type   string                   `json:"type"`
		Body   InspectOriginalRequestV1 `json:"body"`
	}{"praxis.agent-run-service.inspect-original", "InspectOriginalRequestV1", clone})
}

func (r InspectOriginalRequestV1) Validate() error {
	if r.ContractVersion != AgentRunServiceContractVersionV1 {
		return NewError(FaultInvalidArgumentV1, "inspect_original_version_invalid", "Inspect Original contract version is invalid")
	}
	for field, value := range map[string]string{"request id": r.RequestID, "trace id": r.TraceID} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	hasCommand, hasAttempt := r.OriginalCommandRef != nil, r.OriginalAttemptRef != nil
	if hasCommand == hasAttempt {
		return NewError(FaultInvalidArgumentV1, "inspect_original_subject_invalid", "Inspect Original requires exactly one command or attempt ref")
	}
	if hasCommand {
		if err := r.OriginalCommandRef.Validate(); err != nil {
			return err
		}
		if r.OriginalCommandRef.Kind != AgentRunCommandRefKindV1 || r.OriginalCommandRef.Digest != r.OriginalRequestDigest {
			return NewError(FaultRevisionConflictV1, "inspect_original_command_drift", "Inspect Original command ref and request digest differ")
		}
		if err := ValidateIdentifierV1("original idempotency key", r.OriginalIdempotencyKey); err != nil {
			return err
		}
		if err := r.OriginalRequestDigest.Validate(); err != nil {
			return err
		}
	} else {
		if err := r.OriginalAttemptRef.Validate(); err != nil {
			return err
		}
		if r.OriginalIdempotencyKey != "" || r.OriginalRequestDigest != "" {
			return NewError(FaultPreconditionFailedV1, "inspect_original_attempt_command_fields_present", "attempt Inspect cannot invent command idempotency fields")
		}
	}
	if err := r.RequestedUnixNano.ValidatePositiveV1("Inspect Original requested UnixNano"); err != nil {
		return err
	}
	if err := r.NotAfterUnixNano.ValidatePositiveV1("Inspect Original not-after UnixNano"); err != nil {
		return err
	}
	requested, _ := r.RequestedUnixNano.Int64V1()
	notAfter, _ := r.NotAfterUnixNano.Int64V1()
	if notAfter <= requested {
		return NewError(FaultInvalidArgumentV1, "inspect_original_window_invalid", "Inspect Original not-after must be after request")
	}
	digest, err := r.digestV1()
	if err != nil || digest != r.RequestDigest {
		return NewError(FaultRevisionConflictV1, "inspect_original_request_digest_drift", "Inspect Original request digest drifted")
	}
	return nil
}

func (r InspectOriginalRequestV1) ValidateCurrentV1(now WireUnixNanoV1) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := now.ValidatePositiveV1("current UnixNano"); err != nil {
		return err
	}
	requested, _ := r.RequestedUnixNano.Int64V1()
	notAfter, _ := r.NotAfterUnixNano.Int64V1()
	current, _ := now.Int64V1()
	if current < requested {
		return NewError(FaultPreconditionFailedV1, "clock_regression", "Inspect Original was checked before request")
	}
	if current >= notAfter {
		return NewError(FaultPreconditionFailedV1, "inspect_original_request_expired", "Inspect Original reached or exceeded not-after")
	}
	return nil
}

type InspectOriginalDispositionV1 string

const (
	InspectOriginalObservedV1      InspectOriginalDispositionV1 = "OBSERVED"
	InspectOriginalNotFoundV1      InspectOriginalDispositionV1 = "NOT_FOUND"
	InspectOriginalIndeterminateV1 InspectOriginalDispositionV1 = "INDETERMINATE"
)

type InspectOriginalResultV1 struct {
	ContractVersion    string                       `json:"contract_version"`
	RequestDigest      DigestV1                     `json:"request_digest"`
	TraceID            string                       `json:"trace_id"`
	Disposition        InspectOriginalDispositionV1 `json:"disposition"`
	CommandReceipt     *AgentRunCommandReceiptV1    `json:"command_receipt,omitempty"`
	ObservedAttemptRef *ExactRefWireV1              `json:"observed_attempt_ref,omitempty"`
	CurrentRef         *ExactRefWireV1              `json:"current_ref,omitempty"`
	Fault              *FaultV1                     `json:"fault,omitempty"`
	Window             WireValidityWindowV1         `json:"window"`
	ResultDigest       DigestV1                     `json:"result_digest"`
}

func SealInspectOriginalResultV1(result InspectOriginalResultV1) (InspectOriginalResultV1, error) {
	if err := validateOptionalContractVersionV1(result.ContractVersion, AgentRunServiceContractVersionV1); err != nil {
		return InspectOriginalResultV1{}, err
	}
	result.ContractVersion = AgentRunServiceContractVersionV1
	provided := result.ResultDigest
	result.ResultDigest = ""
	digest, err := result.digestV1()
	if err != nil {
		return InspectOriginalResultV1{}, err
	}
	if provided != "" && provided != digest {
		return InspectOriginalResultV1{}, NewError(FaultRevisionConflictV1, "inspect_original_result_digest_drift", "Inspect Original result supplied a wrong digest")
	}
	result.ResultDigest = digest
	return result, result.Validate()
}

func (r InspectOriginalResultV1) digestV1() (DigestV1, error) {
	clone := r
	clone.ResultDigest = ""
	return DigestJSONV1(struct {
		Domain string                  `json:"domain"`
		Type   string                  `json:"type"`
		Body   InspectOriginalResultV1 `json:"body"`
	}{"praxis.agent-run-service.inspect-original", "InspectOriginalResultV1", clone})
}

func (r InspectOriginalResultV1) Validate() error {
	if r.ContractVersion != AgentRunServiceContractVersionV1 {
		return NewError(FaultInvalidArgumentV1, "inspect_original_result_version_invalid", "Inspect Original result contract version is invalid")
	}
	if err := r.RequestDigest.Validate(); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("trace id", r.TraceID); err != nil {
		return err
	}
	if err := r.Window.Validate(); err != nil {
		return err
	}
	for _, ref := range []*ExactRefWireV1{r.ObservedAttemptRef, r.CurrentRef} {
		if ref != nil {
			if err := ref.Validate(); err != nil {
				return err
			}
		}
	}
	switch r.Disposition {
	case InspectOriginalObservedV1:
		if (r.CommandReceipt == nil) == (r.ObservedAttemptRef == nil) || r.Fault != nil {
			return NewError(FaultPreconditionFailedV1, "inspect_original_observed_subject_invalid", "observed result requires exactly one command receipt or attempt ref and no fault")
		}
		if r.CommandReceipt != nil {
			if err := r.CommandReceipt.Validate(); err != nil {
				return err
			}
			recorded, _ := r.CommandReceipt.RecordedUnixNano.Int64V1()
			checked, _ := r.Window.CheckedUnixNano.Int64V1()
			if recorded > checked {
				return NewError(FaultPreconditionFailedV1, "inspect_original_receipt_after_check", "observed command receipt was recorded after the result checked watermark")
			}
		}
	case InspectOriginalNotFoundV1:
		if r.CommandReceipt != nil || r.ObservedAttemptRef != nil || r.Fault == nil || r.Fault.Code != FaultNotFoundV1 {
			return NewError(FaultPreconditionFailedV1, "inspect_original_not_found_fault_missing", "not-found result requires a typed NOT_FOUND fault")
		}
	case InspectOriginalIndeterminateV1:
		if r.CommandReceipt != nil || r.ObservedAttemptRef != nil || r.Fault == nil || (r.Fault.Code != FaultUnknownOutcomeV1 && r.Fault.Code != FaultIndeterminateV1) {
			return NewError(FaultPreconditionFailedV1, "inspect_original_indeterminate_fault_missing", "indeterminate result requires UNKNOWN_OUTCOME or INDETERMINATE")
		}
	default:
		return NewError(FaultInvalidArgumentV1, "inspect_original_disposition_invalid", "Inspect Original disposition is invalid")
	}
	if r.Fault != nil {
		if err := r.Fault.Validate(); err != nil {
			return err
		}
		if r.Fault.TraceID != r.TraceID {
			return NewError(FaultRevisionConflictV1, "inspect_original_fault_trace_drift", "Inspect Original fault trace drifted")
		}
	}
	digest, err := r.digestV1()
	if err != nil || digest != r.ResultDigest {
		return NewError(FaultRevisionConflictV1, "inspect_original_result_digest_drift", "Inspect Original result digest drifted")
	}
	return nil
}

func (r InspectOriginalResultV1) ValidateFor(request InspectOriginalRequestV1) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if r.RequestDigest != request.RequestDigest || r.TraceID != request.TraceID {
		return NewError(FaultRevisionConflictV1, "inspect_original_result_request_drift", "Inspect Original result belongs to another request")
	}
	if err := r.Window.ValidateWithinRequestV1(request.RequestedUnixNano, request.NotAfterUnixNano); err != nil {
		return err
	}
	if request.OriginalCommandRef != nil {
		if r.Fault != nil && r.Fault.AttemptRef != nil {
			return NewError(FaultRevisionConflictV1, "inspect_original_fault_subject_drift", "command Inspect fault cannot name an attempt")
		}
		if r.CommandReceipt != nil && (r.CommandReceipt.CommandRef != *request.OriginalCommandRef || r.CommandReceipt.IdempotencyKey != request.OriginalIdempotencyKey || r.CommandReceipt.OriginalRequestDigest != request.OriginalRequestDigest) {
			return NewError(FaultRevisionConflictV1, "inspect_original_receipt_drift", "Inspect Original returned another command receipt")
		}
		if r.ObservedAttemptRef != nil {
			return NewError(FaultRevisionConflictV1, "inspect_original_subject_drift", "command Inspect returned an attempt observation")
		}
		if r.Disposition == InspectOriginalIndeterminateV1 && (r.Fault.CommandRef == nil || r.Fault.AttemptRef != nil || *r.Fault.CommandRef != *request.OriginalCommandRef) {
			return NewError(FaultRevisionConflictV1, "inspect_original_fault_subject_drift", "indeterminate command Inspect must name the exact original command")
		}
		if r.Fault != nil && r.Fault.CommandRef != nil && *r.Fault.CommandRef != *request.OriginalCommandRef {
			return NewError(FaultRevisionConflictV1, "inspect_original_fault_subject_drift", "Inspect Original fault names another command")
		}
	} else {
		if r.Fault != nil && r.Fault.CommandRef != nil {
			return NewError(FaultRevisionConflictV1, "inspect_original_fault_subject_drift", "attempt Inspect fault cannot name a command")
		}
		if r.CommandReceipt != nil || (r.ObservedAttemptRef != nil && *r.ObservedAttemptRef != *request.OriginalAttemptRef) {
			return NewError(FaultRevisionConflictV1, "inspect_original_subject_drift", "attempt Inspect returned another subject")
		}
		if r.Disposition == InspectOriginalIndeterminateV1 && (r.Fault.AttemptRef == nil || r.Fault.CommandRef != nil || *r.Fault.AttemptRef != *request.OriginalAttemptRef) {
			return NewError(FaultRevisionConflictV1, "inspect_original_fault_subject_drift", "indeterminate attempt Inspect must name the exact original attempt")
		}
		if r.Fault != nil && r.Fault.AttemptRef != nil && *r.Fault.AttemptRef != *request.OriginalAttemptRef {
			return NewError(FaultRevisionConflictV1, "inspect_original_fault_subject_drift", "Inspect Original fault names another attempt")
		}
	}
	return nil
}

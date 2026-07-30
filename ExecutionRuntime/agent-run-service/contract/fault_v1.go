package contract

import "strings"

type RetryDirectiveV1 string

const (
	RetryNoneV1    RetryDirectiveV1 = "NONE"
	RetryCommandV1 RetryDirectiveV1 = "RETRY"
	RetryInspectV1 RetryDirectiveV1 = "INSPECT"
)

// FaultV1 is a transport-safe, typed failure. Transports must map it without
// reducing distinct public codes to a generic conflict or HTTP 500.
type FaultV1 struct {
	Code               FaultCodeV1      `json:"code"`
	Reason             string           `json:"reason"`
	Message            string           `json:"message"`
	CommandRef         *ExactRefWireV1  `json:"command_ref,omitempty"`
	AttemptRef         *ExactRefWireV1  `json:"attempt_ref,omitempty"`
	CurrentRef         *ExactRefWireV1  `json:"current_ref,omitempty"`
	TraceID            string           `json:"trace_id"`
	RetryDirective     RetryDirectiveV1 `json:"retry_directive"`
	RetryAfterUnixNano *WireUnixNanoV1  `json:"retry_after_unix_nano,omitempty"`
}

func (f FaultV1) Validate() error {
	if !validFaultCodeV1(f.Code) {
		return NewError(FaultInvalidArgumentV1, "fault_code_invalid", "public fault code is invalid")
	}
	if err := ValidateIdentifierV1("fault reason", f.Reason); err != nil {
		return err
	}
	if strings.TrimSpace(f.Message) == "" || f.Message != strings.TrimSpace(f.Message) {
		return NewError(FaultInvalidArgumentV1, "fault_message_invalid", "fault message is required")
	}
	if err := ValidateIdentifierV1("trace id", f.TraceID); err != nil {
		return err
	}
	for _, ref := range []*ExactRefWireV1{f.CommandRef, f.AttemptRef, f.CurrentRef} {
		if ref != nil {
			if err := ref.Validate(); err != nil {
				return err
			}
		}
	}
	switch f.RetryDirective {
	case RetryNoneV1, RetryCommandV1, RetryInspectV1:
	default:
		return NewError(FaultInvalidArgumentV1, "retry_directive_invalid", "retry directive is invalid")
	}
	if f.RetryAfterUnixNano != nil {
		if f.RetryDirective != RetryCommandV1 {
			return NewError(FaultPreconditionFailedV1, "retry_after_without_retry", "retry-after requires RETRY")
		}
		if err := f.RetryAfterUnixNano.ValidatePositiveV1("retry-after UnixNano"); err != nil {
			return err
		}
	}
	if f.Code == FaultUnknownOutcomeV1 || f.Code == FaultIndeterminateV1 {
		hasCommand, hasAttempt := f.CommandRef != nil, f.AttemptRef != nil
		if f.RetryDirective != RetryInspectV1 || hasCommand == hasAttempt {
			return NewError(FaultPreconditionFailedV1, "unknown_outcome_inspect_subject_invalid", "unknown or indeterminate outcome requires INSPECT and exactly one original command or attempt ref")
		}
	}
	if f.Code == FaultIdempotencyConflictV1 && f.CommandRef == nil {
		return NewError(FaultPreconditionFailedV1, "idempotency_conflict_command_missing", "idempotency conflict requires the original command ref")
	}
	return nil
}

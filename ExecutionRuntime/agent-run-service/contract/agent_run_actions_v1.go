package contract

type CancelAgentRunRequestV1 struct {
	Command AgentRunCommandEnvelopeV1 `json:"command"`
}

func (r CancelAgentRunRequestV1) Validate() error {
	if err := r.Command.Validate(); err != nil {
		return err
	}
	if r.Command.Kind != AgentRunCommandCancelRunV1 {
		return NewError(FaultInvalidArgumentV1, "cancel_command_kind_invalid", "Cancel request requires CANCEL_RUN")
	}
	return nil
}

type StopAgentHostRequestV1 struct {
	Command AgentRunCommandEnvelopeV1 `json:"command"`
}

func (r StopAgentHostRequestV1) Validate() error {
	if err := r.Command.Validate(); err != nil {
		return err
	}
	if r.Command.Kind != AgentRunCommandStopHostV1 {
		return NewError(FaultInvalidArgumentV1, "stop_command_kind_invalid", "Host Stop request requires STOP_HOST")
	}
	return nil
}

// CommandResultV1 carries a typed receipt even when the owner outcome is
// UNKNOWN_OUTCOME or INDETERMINATE. Transport implementations must not replace
// this result with a generic 500 response.
type CommandResultV1 struct {
	ContractVersion string                   `json:"contract_version"`
	RequestDigest   DigestV1                 `json:"request_digest"`
	TraceID         string                   `json:"trace_id"`
	Receipt         AgentRunCommandReceiptV1 `json:"receipt"`
	Window          WireValidityWindowV1     `json:"window"`
	ResultDigest    DigestV1                 `json:"result_digest"`
}

func SealCommandResultV1(result CommandResultV1) (CommandResultV1, error) {
	if err := validateOptionalContractVersionV1(result.ContractVersion, AgentRunServiceContractVersionV1); err != nil {
		return CommandResultV1{}, err
	}
	result.ContractVersion = AgentRunServiceContractVersionV1
	provided := result.ResultDigest
	result.ResultDigest = ""
	digest, err := result.digestV1()
	if err != nil {
		return CommandResultV1{}, err
	}
	if provided != "" && provided != digest {
		return CommandResultV1{}, NewError(FaultRevisionConflictV1, "command_result_digest_drift", "command result supplied a wrong digest")
	}
	result.ResultDigest = digest
	return result, result.Validate()
}

func (r CommandResultV1) digestV1() (DigestV1, error) {
	clone := r
	clone.ResultDigest = ""
	return DigestJSONV1(struct {
		Domain string          `json:"domain"`
		Type   string          `json:"type"`
		Body   CommandResultV1 `json:"body"`
	}{"praxis.agent-run-service.command-result", "CommandResultV1", clone})
}

func (r CommandResultV1) Validate() error {
	if r.ContractVersion != AgentRunServiceContractVersionV1 {
		return NewError(FaultInvalidArgumentV1, "command_result_version_invalid", "command result contract version is invalid")
	}
	if err := r.RequestDigest.Validate(); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("trace id", r.TraceID); err != nil {
		return err
	}
	if err := r.Receipt.Validate(); err != nil {
		return err
	}
	if err := r.Window.Validate(); err != nil {
		return err
	}
	if r.RequestDigest != r.Receipt.OriginalRequestDigest || r.TraceID != r.Receipt.TraceID {
		return NewError(FaultRevisionConflictV1, "command_result_request_drift", "command result and receipt bind different request traces")
	}
	recorded, _ := r.Receipt.RecordedUnixNano.Int64V1()
	checked, _ := r.Window.CheckedUnixNano.Int64V1()
	if recorded > checked {
		return NewError(FaultPreconditionFailedV1, "command_receipt_after_result_check", "command receipt was recorded after the result checked watermark")
	}
	digest, err := r.digestV1()
	if err != nil || digest != r.ResultDigest {
		return NewError(FaultRevisionConflictV1, "command_result_digest_drift", "command result digest drifted")
	}
	return nil
}

func (r CommandResultV1) ValidateFor(command AgentRunCommandEnvelopeV1) error {
	if err := command.Validate(); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if r.RequestDigest != command.RequestDigest || r.TraceID != command.TraceID {
		return NewError(FaultRevisionConflictV1, "command_result_request_drift", "command result belongs to another request")
	}
	if err := r.Window.ValidateWithinRequestV1(command.SubmittedAtUnixNano, command.NotAfterUnixNano); err != nil {
		return err
	}
	return r.Receipt.ValidateFor(command)
}

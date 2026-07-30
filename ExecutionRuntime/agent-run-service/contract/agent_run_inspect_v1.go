package contract

import "sort"

type AgentRunInspectRequestV1 struct {
	ContractVersion   string           `json:"contract_version"`
	RequestID         string           `json:"request_id"`
	TraceID           string           `json:"trace_id"`
	Target            AgentRunTargetV1 `json:"target"`
	RequestedUnixNano WireUnixNanoV1   `json:"requested_unix_nano"`
	NotAfterUnixNano  WireUnixNanoV1   `json:"not_after_unix_nano"`
	RequestDigest     DigestV1         `json:"request_digest"`
}

func SealAgentRunInspectRequestV1(request AgentRunInspectRequestV1) (AgentRunInspectRequestV1, error) {
	if request.ContractVersion != "" && request.ContractVersion != AgentRunServiceContractVersionV1 {
		return AgentRunInspectRequestV1{}, NewError(FaultRevisionConflictV1, "agent_run_service_version_mismatch", "Agent Run Inspect version drifted")
	}
	request.ContractVersion = AgentRunServiceContractVersionV1
	provided := request.RequestDigest
	request.RequestDigest = ""
	digest, err := request.digestV1()
	if err != nil {
		return AgentRunInspectRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentRunInspectRequestV1{}, NewError(FaultRevisionConflictV1, "inspect_request_digest_drift", "Inspect request supplied a wrong digest")
	}
	request.RequestDigest = digest
	return request, request.Validate()
}

func (r AgentRunInspectRequestV1) digestV1() (DigestV1, error) {
	clone := r
	clone.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string                   `json:"domain"`
		Type   string                   `json:"type"`
		Body   AgentRunInspectRequestV1 `json:"body"`
	}{"praxis.agent-run-service.inspect", "AgentRunInspectRequestV1", clone})
}

func (r AgentRunInspectRequestV1) Validate() error {
	if r.ContractVersion != AgentRunServiceContractVersionV1 {
		return NewError(FaultInvalidArgumentV1, "inspect_contract_version_invalid", "Agent Run Inspect contract version is invalid")
	}
	for field, value := range map[string]string{"request id": r.RequestID, "trace id": r.TraceID} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	if err := r.Target.Validate(); err != nil {
		return err
	}
	if err := r.RequestedUnixNano.ValidatePositiveV1("Inspect requested UnixNano"); err != nil {
		return err
	}
	if err := r.NotAfterUnixNano.ValidatePositiveV1("Inspect not-after UnixNano"); err != nil {
		return err
	}
	requested, _ := r.RequestedUnixNano.Int64V1()
	notAfter, _ := r.NotAfterUnixNano.Int64V1()
	if notAfter <= requested {
		return NewError(FaultInvalidArgumentV1, "inspect_window_invalid", "Inspect not-after must be after request")
	}
	digest, err := r.digestV1()
	if err != nil || digest != r.RequestDigest {
		return NewError(FaultRevisionConflictV1, "inspect_request_digest_drift", "Inspect request digest drifted")
	}
	return nil
}

func (r AgentRunInspectRequestV1) ValidateCurrentV1(now WireUnixNanoV1) error {
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
		return NewError(FaultPreconditionFailedV1, "clock_regression", "Inspect was checked before request")
	}
	if current >= notAfter {
		return NewError(FaultPreconditionFailedV1, "inspect_request_expired", "Inspect reached or exceeded not-after")
	}
	return nil
}

// OwnerProjectionV1 remains an observation of an exact owner current. State
// and outcome strings are never aggregated into a new Runtime outcome.
type OwnerProjectionV1 struct {
	OwnerDomain      string               `json:"owner_domain"`
	OwnerContract    string               `json:"owner_contract"`
	CurrentRef       ExactRefWireV1       `json:"current_ref"`
	State            string               `json:"state"`
	Outcome          string               `json:"outcome,omitempty"`
	Window           WireValidityWindowV1 `json:"window"`
	ProjectionDigest DigestV1             `json:"projection_digest"`
}

func SealOwnerProjectionV1(projection OwnerProjectionV1) (OwnerProjectionV1, error) {
	provided := projection.ProjectionDigest
	projection.ProjectionDigest = ""
	digest, err := projection.digestV1()
	if err != nil {
		return OwnerProjectionV1{}, err
	}
	if provided != "" && provided != digest {
		return OwnerProjectionV1{}, NewError(FaultRevisionConflictV1, "owner_projection_digest_drift", "owner projection supplied a wrong digest")
	}
	projection.ProjectionDigest = digest
	return projection, projection.Validate()
}

func (p OwnerProjectionV1) digestV1() (DigestV1, error) {
	clone := p
	clone.ProjectionDigest = ""
	return DigestJSONV1(struct {
		Domain string            `json:"domain"`
		Type   string            `json:"type"`
		Body   OwnerProjectionV1 `json:"body"`
	}{"praxis.agent-run-service.owner-projection", "OwnerProjectionV1", clone})
}

func (p OwnerProjectionV1) Validate() error {
	for field, value := range map[string]string{"owner domain": p.OwnerDomain, "owner contract": p.OwnerContract, "owner state": p.State} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	if p.Outcome != "" {
		if err := ValidateIdentifierV1("owner outcome", p.Outcome); err != nil {
			return err
		}
	}
	if err := p.CurrentRef.Validate(); err != nil {
		return err
	}
	if err := p.Window.Validate(); err != nil {
		return err
	}
	digest, err := p.digestV1()
	if err != nil || digest != p.ProjectionDigest {
		return NewError(FaultRevisionConflictV1, "owner_projection_digest_drift", "owner projection digest drifted")
	}
	return nil
}

type AgentRunInspectDispositionV1 string

const (
	AgentRunInspectObservedV1      AgentRunInspectDispositionV1 = "OBSERVED"
	AgentRunInspectNotFoundV1      AgentRunInspectDispositionV1 = "NOT_FOUND"
	AgentRunInspectIndeterminateV1 AgentRunInspectDispositionV1 = "INDETERMINATE"
)

type AgentRunInspectResultV1 struct {
	ContractVersion string                       `json:"contract_version"`
	RequestDigest   DigestV1                     `json:"request_digest"`
	TraceID         string                       `json:"trace_id"`
	Target          AgentRunTargetV1             `json:"target"`
	Disposition     AgentRunInspectDispositionV1 `json:"disposition"`
	Projections     []OwnerProjectionV1          `json:"projections"`
	Fault           *FaultV1                     `json:"fault,omitempty"`
	Window          WireValidityWindowV1         `json:"window"`
	ResultDigest    DigestV1                     `json:"result_digest"`
}

func SealAgentRunInspectResultV1(result AgentRunInspectResultV1) (AgentRunInspectResultV1, error) {
	if err := validateOptionalContractVersionV1(result.ContractVersion, AgentRunServiceContractVersionV1); err != nil {
		return AgentRunInspectResultV1{}, err
	}
	result.ContractVersion = AgentRunServiceContractVersionV1
	result.Projections = append([]OwnerProjectionV1(nil), result.Projections...)
	sort.Slice(result.Projections, func(i, j int) bool { return result.Projections[i].OwnerDomain < result.Projections[j].OwnerDomain })
	provided := result.ResultDigest
	result.ResultDigest = ""
	digest, err := result.digestV1()
	if err != nil {
		return AgentRunInspectResultV1{}, err
	}
	if provided != "" && provided != digest {
		return AgentRunInspectResultV1{}, NewError(FaultRevisionConflictV1, "inspect_result_digest_drift", "Inspect result supplied a wrong digest")
	}
	result.ResultDigest = digest
	return result, result.Validate()
}

func (r AgentRunInspectResultV1) digestV1() (DigestV1, error) {
	clone := r
	clone.ResultDigest = ""
	return DigestJSONV1(struct {
		Domain string                  `json:"domain"`
		Type   string                  `json:"type"`
		Body   AgentRunInspectResultV1 `json:"body"`
	}{"praxis.agent-run-service.inspect", "AgentRunInspectResultV1", clone})
}

func (r AgentRunInspectResultV1) Validate() error {
	if r.ContractVersion != AgentRunServiceContractVersionV1 {
		return NewError(FaultInvalidArgumentV1, "inspect_result_version_invalid", "Inspect result contract version is invalid")
	}
	if err := r.RequestDigest.Validate(); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("trace id", r.TraceID); err != nil {
		return err
	}
	if err := r.Target.Validate(); err != nil {
		return err
	}
	if err := r.Window.Validate(); err != nil {
		return err
	}
	resultChecked, _ := r.Window.CheckedUnixNano.Int64V1()
	resultExpires, _ := r.Window.ExpiresUnixNano.Int64V1()
	for index, projection := range r.Projections {
		if err := projection.Validate(); err != nil {
			return err
		}
		if index > 0 && r.Projections[index-1].OwnerDomain >= projection.OwnerDomain {
			return NewError(FaultInvalidArgumentV1, "owner_projection_duplicate", "owner projections must be sorted and unique")
		}
		projectionChecked, _ := projection.Window.CheckedUnixNano.Int64V1()
		projectionExpires, _ := projection.Window.ExpiresUnixNano.Int64V1()
		if resultChecked < projectionChecked || resultExpires > projectionExpires {
			return NewError(FaultPreconditionFailedV1, "inspect_result_window_drift", "Inspect result exceeds an owner projection window")
		}
	}
	switch r.Disposition {
	case AgentRunInspectObservedV1:
		if len(r.Projections) == 0 || r.Fault != nil {
			return NewError(FaultPreconditionFailedV1, "observed_inspect_incomplete", "observed Inspect requires projections and no fault")
		}
	case AgentRunInspectNotFoundV1:
		if len(r.Projections) != 0 || r.Fault == nil || r.Fault.Code != FaultNotFoundV1 {
			return NewError(FaultPreconditionFailedV1, "not_found_inspect_fault_missing", "not-found Inspect requires a typed NOT_FOUND fault")
		}
	case AgentRunInspectIndeterminateV1:
		if r.Fault == nil || (r.Fault.Code != FaultUnavailableV1 && r.Fault.Code != FaultInternalV1) {
			return NewError(FaultPreconditionFailedV1, "indeterminate_inspect_fault_missing", "indeterminate aggregate Inspect requires a typed availability fault")
		}
	default:
		return NewError(FaultInvalidArgumentV1, "inspect_disposition_invalid", "Inspect disposition is invalid")
	}
	if r.Fault != nil {
		if err := r.Fault.Validate(); err != nil {
			return err
		}
		if r.Fault.TraceID != r.TraceID {
			return NewError(FaultRevisionConflictV1, "inspect_fault_trace_drift", "Inspect fault trace differs from result")
		}
	}
	digest, err := r.digestV1()
	if err != nil || digest != r.ResultDigest {
		return NewError(FaultRevisionConflictV1, "inspect_result_digest_drift", "Inspect result digest drifted")
	}
	return nil
}

func (r AgentRunInspectResultV1) ValidateFor(request AgentRunInspectRequestV1) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if r.RequestDigest != request.RequestDigest || r.TraceID != request.TraceID || r.Target != request.Target {
		return NewError(FaultRevisionConflictV1, "inspect_result_request_drift", "Inspect result belongs to another exact target or request")
	}
	if err := r.Window.ValidateWithinRequestV1(request.RequestedUnixNano, request.NotAfterUnixNano); err != nil {
		return err
	}
	return nil
}

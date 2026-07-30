package contract

import "sort"

const AgentRunServiceContractVersionV1 = "praxis.agent-run-service/v1"

type CapabilityV1 string

const (
	CapabilityAgentRunInspectV1        CapabilityV1 = "agent_run.inspect"
	CapabilityAgentRunWatchV1          CapabilityV1 = "agent_run.watch"
	CapabilityAgentRunCancelV1         CapabilityV1 = "agent_run.cancel"
	CapabilityAgentHostStopV1          CapabilityV1 = "agent_host.stop"
	CapabilityCommandInspectOriginalV1 CapabilityV1 = "command.inspect_original"
)

func validCapabilityV1(capability CapabilityV1) bool {
	switch capability {
	case CapabilityAgentRunInspectV1, CapabilityAgentRunWatchV1, CapabilityAgentRunCancelV1,
		CapabilityAgentHostStopV1, CapabilityCommandInspectOriginalV1:
		return true
	default:
		return false
	}
}

type NegotiationRequestV1 struct {
	WireContractVersion  string         `json:"wire_contract_version"`
	RequestID            string         `json:"request_id"`
	TraceID              string         `json:"trace_id"`
	SupportedVersions    []string       `json:"supported_versions"`
	RequiredCapabilities []CapabilityV1 `json:"required_capabilities"`
	OptionalCapabilities []CapabilityV1 `json:"optional_capabilities"`
	RequestedUnixNano    WireUnixNanoV1 `json:"requested_unix_nano"`
	NotAfterUnixNano     WireUnixNanoV1 `json:"not_after_unix_nano"`
	RequestDigest        DigestV1       `json:"request_digest"`
}

func SealNegotiationRequestV1(request NegotiationRequestV1) (NegotiationRequestV1, error) {
	if request.WireContractVersion != "" && request.WireContractVersion != CrossLanguageWireContractVersionV1 {
		return NegotiationRequestV1{}, NewError(FaultRevisionConflictV1, "wire_contract_version_mismatch", "wire negotiation version drifted")
	}
	request.WireContractVersion = CrossLanguageWireContractVersionV1
	var err error
	request.RequiredCapabilities, err = canonicalCapabilitiesV1(request.RequiredCapabilities)
	if err != nil {
		return NegotiationRequestV1{}, err
	}
	request.OptionalCapabilities, err = canonicalCapabilitiesV1(request.OptionalCapabilities)
	if err != nil {
		return NegotiationRequestV1{}, err
	}
	if err := validateCapabilityOverlapV1(request.RequiredCapabilities, request.OptionalCapabilities); err != nil {
		return NegotiationRequestV1{}, err
	}
	provided := request.RequestDigest
	request.RequestDigest = ""
	digest, err := request.digestV1()
	if err != nil {
		return NegotiationRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return NegotiationRequestV1{}, NewError(FaultRevisionConflictV1, "negotiation_request_digest_drift", "negotiation request supplied a wrong digest")
	}
	request.RequestDigest = digest
	return request, request.Validate()
}

func (r NegotiationRequestV1) digestV1() (DigestV1, error) {
	clone := r
	clone.RequestDigest = ""
	return DigestJSONV1(struct {
		Domain string               `json:"domain"`
		Type   string               `json:"type"`
		Body   NegotiationRequestV1 `json:"body"`
	}{"praxis.agent-run-service.negotiation", "NegotiationRequestV1", clone})
}

func (r NegotiationRequestV1) Validate() error {
	if r.WireContractVersion != CrossLanguageWireContractVersionV1 || len(r.SupportedVersions) == 0 {
		return NewError(FaultInvalidArgumentV1, "negotiation_request_incomplete", "wire negotiation requires a version preference")
	}
	for field, value := range map[string]string{"request id": r.RequestID, "trace id": r.TraceID} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	seenVersions := map[string]struct{}{}
	for _, version := range r.SupportedVersions {
		if err := ValidateIdentifierV1("supported contract version", version); err != nil {
			return err
		}
		if _, exists := seenVersions[version]; exists {
			return NewError(FaultInvalidArgumentV1, "supported_version_duplicate", "wire negotiation duplicates a supported version")
		}
		seenVersions[version] = struct{}{}
	}
	if !capabilitiesCanonicalV1(r.RequiredCapabilities) || !capabilitiesCanonicalV1(r.OptionalCapabilities) {
		return NewError(FaultInvalidArgumentV1, "capabilities_not_canonical", "capabilities must be sorted and unique")
	}
	if err := validateCapabilityOverlapV1(r.RequiredCapabilities, r.OptionalCapabilities); err != nil {
		return err
	}
	if err := r.RequestedUnixNano.ValidatePositiveV1("negotiation requested UnixNano"); err != nil {
		return err
	}
	if err := r.NotAfterUnixNano.ValidatePositiveV1("negotiation not-after UnixNano"); err != nil {
		return err
	}
	requested, _ := r.RequestedUnixNano.Int64V1()
	notAfter, _ := r.NotAfterUnixNano.Int64V1()
	if notAfter <= requested {
		return NewError(FaultInvalidArgumentV1, "negotiation_window_invalid", "negotiation not-after must be after request")
	}
	digest, err := r.digestV1()
	if err != nil || digest != r.RequestDigest {
		return NewError(FaultRevisionConflictV1, "negotiation_request_digest_drift", "negotiation request digest drifted")
	}
	return nil
}

func (r NegotiationRequestV1) ValidateCurrentV1(now WireUnixNanoV1) error {
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
		return NewError(FaultPreconditionFailedV1, "clock_regression", "negotiation was checked before request")
	}
	if current >= notAfter {
		return NewError(FaultPreconditionFailedV1, "negotiation_request_expired", "negotiation reached or exceeded not-after")
	}
	return nil
}

type NegotiationDispositionV1 string

const (
	NegotiationSelectedV1              NegotiationDispositionV1 = "SELECTED"
	NegotiationCapabilityUnavailableV1 NegotiationDispositionV1 = "CAPABILITY_UNAVAILABLE"
	NegotiationIndeterminateV1         NegotiationDispositionV1 = "INDETERMINATE"
)

type NegotiationResultV1 struct {
	WireContractVersion string                   `json:"wire_contract_version"`
	RequestDigest       DigestV1                 `json:"request_digest"`
	TraceID             string                   `json:"trace_id"`
	Disposition         NegotiationDispositionV1 `json:"disposition"`
	SelectedVersion     string                   `json:"selected_version,omitempty"`
	GrantedCapabilities []CapabilityV1           `json:"granted_capabilities"`
	Fault               *FaultV1                 `json:"fault,omitempty"`
	Window              WireValidityWindowV1     `json:"window"`
	ResultDigest        DigestV1                 `json:"result_digest"`
}

func SealNegotiationResultV1(result NegotiationResultV1) (NegotiationResultV1, error) {
	if err := validateOptionalContractVersionV1(result.WireContractVersion, CrossLanguageWireContractVersionV1); err != nil {
		return NegotiationResultV1{}, err
	}
	result.WireContractVersion = CrossLanguageWireContractVersionV1
	capabilities, err := canonicalCapabilitiesV1(result.GrantedCapabilities)
	if err != nil {
		return NegotiationResultV1{}, err
	}
	result.GrantedCapabilities = capabilities
	provided := result.ResultDigest
	result.ResultDigest = ""
	digest, err := result.digestV1()
	if err != nil {
		return NegotiationResultV1{}, err
	}
	if provided != "" && provided != digest {
		return NegotiationResultV1{}, NewError(FaultRevisionConflictV1, "negotiation_result_digest_drift", "negotiation result supplied a wrong digest")
	}
	result.ResultDigest = digest
	return result, result.Validate()
}

func (r NegotiationResultV1) digestV1() (DigestV1, error) {
	clone := r
	clone.ResultDigest = ""
	return DigestJSONV1(struct {
		Domain string              `json:"domain"`
		Type   string              `json:"type"`
		Body   NegotiationResultV1 `json:"body"`
	}{"praxis.agent-run-service.negotiation", "NegotiationResultV1", clone})
}

func (r NegotiationResultV1) Validate() error {
	if r.WireContractVersion != CrossLanguageWireContractVersionV1 {
		return NewError(FaultInvalidArgumentV1, "negotiation_result_incomplete", "wire negotiation result version is required")
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
	if !capabilitiesCanonicalV1(r.GrantedCapabilities) {
		return NewError(FaultInvalidArgumentV1, "granted_capabilities_not_canonical", "granted capabilities must be sorted and unique")
	}
	switch r.Disposition {
	case NegotiationSelectedV1:
		if err := ValidateIdentifierV1("selected contract version", r.SelectedVersion); err != nil {
			return err
		}
		if r.Fault != nil {
			return NewError(FaultPreconditionFailedV1, "selected_negotiation_fault_present", "selected negotiation cannot carry a fault")
		}
	case NegotiationCapabilityUnavailableV1:
		if r.SelectedVersion != "" || r.Fault == nil || r.Fault.Code != FaultCapabilityUnavailableV1 {
			return NewError(FaultPreconditionFailedV1, "capability_unavailable_fault_missing", "unavailable negotiation requires a typed capability fault")
		}
		if err := r.Fault.Validate(); err != nil {
			return err
		}
	case NegotiationIndeterminateV1:
		if r.SelectedVersion != "" || r.Fault == nil || (r.Fault.Code != FaultUnavailableV1 && r.Fault.Code != FaultInternalV1) {
			return NewError(FaultPreconditionFailedV1, "indeterminate_negotiation_fault_missing", "indeterminate negotiation requires a typed availability fault")
		}
		if err := r.Fault.Validate(); err != nil {
			return err
		}
	default:
		return NewError(FaultInvalidArgumentV1, "negotiation_disposition_invalid", "wire negotiation disposition is invalid")
	}
	if r.Fault != nil && r.Fault.TraceID != r.TraceID {
		return NewError(FaultRevisionConflictV1, "negotiation_fault_trace_drift", "negotiation fault trace differs from result")
	}
	digest, err := r.digestV1()
	if err != nil || digest != r.ResultDigest {
		return NewError(FaultRevisionConflictV1, "negotiation_result_digest_drift", "negotiation result digest drifted")
	}
	return nil
}

func (r NegotiationResultV1) ValidateFor(request NegotiationRequestV1) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if r.RequestDigest != request.RequestDigest || r.TraceID != request.TraceID {
		return NewError(FaultRevisionConflictV1, "negotiation_request_mismatch", "negotiation result belongs to another request")
	}
	if err := r.Window.ValidateWithinRequestV1(request.RequestedUnixNano, request.NotAfterUnixNano); err != nil {
		return err
	}
	if r.Disposition != NegotiationSelectedV1 {
		return nil
	}
	versions := map[string]struct{}{}
	for _, version := range request.SupportedVersions {
		versions[version] = struct{}{}
	}
	if _, exists := versions[r.SelectedVersion]; !exists {
		return NewError(FaultCapabilityUnavailableV1, "selected_version_unoffered", "negotiation selected an unoffered version")
	}
	offered := map[CapabilityV1]struct{}{}
	for _, capability := range append(append([]CapabilityV1{}, request.RequiredCapabilities...), request.OptionalCapabilities...) {
		offered[capability] = struct{}{}
	}
	granted := map[CapabilityV1]struct{}{}
	for _, capability := range r.GrantedCapabilities {
		if _, exists := offered[capability]; !exists {
			return NewError(FaultCapabilityUnavailableV1, "granted_capability_unoffered", "negotiation granted an unoffered capability")
		}
		granted[capability] = struct{}{}
	}
	for _, required := range request.RequiredCapabilities {
		if _, exists := granted[required]; !exists {
			return NewError(FaultCapabilityUnavailableV1, "required_capability_missing", "negotiation silently dropped a required capability")
		}
	}
	return nil
}

func canonicalCapabilitiesV1(input []CapabilityV1) ([]CapabilityV1, error) {
	result := append([]CapabilityV1(nil), input...)
	for _, capability := range result {
		if !validCapabilityV1(capability) {
			return nil, NewError(FaultInvalidArgumentV1, "capability_invalid", "capability is not frozen by AgentRunServiceV1")
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	for index := 1; index < len(result); index++ {
		if result[index-1] == result[index] {
			return nil, NewError(FaultInvalidArgumentV1, "capability_duplicate", "capability is duplicated")
		}
	}
	return result, nil
}

func capabilitiesCanonicalV1(input []CapabilityV1) bool {
	for index, capability := range input {
		if !validCapabilityV1(capability) || (index > 0 && input[index-1] >= capability) {
			return false
		}
	}
	return true
}

func validateCapabilityOverlapV1(required, optional []CapabilityV1) error {
	seen := map[CapabilityV1]struct{}{}
	for _, capability := range required {
		seen[capability] = struct{}{}
	}
	for _, capability := range optional {
		if _, exists := seen[capability]; exists {
			return NewError(FaultInvalidArgumentV1, "capability_overlap", "required and optional capabilities overlap")
		}
	}
	return nil
}

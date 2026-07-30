package contract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
)

func negotiationRequestV1(t *testing.T) contract.NegotiationRequestV1 {
	t.Helper()
	request, err := contract.SealNegotiationRequestV1(contract.NegotiationRequestV1{
		RequestID:            "negotiation-1",
		TraceID:              "trace-1",
		SupportedVersions:    []string{contract.AgentRunServiceContractVersionV1},
		RequiredCapabilities: []contract.CapabilityV1{contract.CapabilityAgentRunInspectV1},
		OptionalCapabilities: []contract.CapabilityV1{contract.CapabilityAgentRunWatchV1},
		RequestedUnixNano:    contract.NewWireUnixNanoV1(100),
		NotAfterUnixNano:     contract.NewWireUnixNanoV1(200),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestNegotiationV1SelectsOnlyOfferedVersionAndCapabilities(t *testing.T) {
	request := negotiationRequestV1(t)
	result, err := contract.SealNegotiationResultV1(contract.NegotiationResultV1{
		RequestDigest:       request.RequestDigest,
		TraceID:             request.TraceID,
		Disposition:         contract.NegotiationSelectedV1,
		SelectedVersion:     contract.AgentRunServiceContractVersionV1,
		GrantedCapabilities: []contract.CapabilityV1{contract.CapabilityAgentRunWatchV1, contract.CapabilityAgentRunInspectV1},
		Window: contract.WireValidityWindowV1{
			CheckedUnixNano: contract.NewWireUnixNanoV1(100),
			ExpiresUnixNano: contract.NewWireUnixNanoV1(200),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(request); err != nil {
		t.Fatal(err)
	}
	result.Window = contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(99), ExpiresUnixNano: contract.NewWireUnixNanoV1(190)}
	result.ResultDigest = ""
	old, err := contract.SealNegotiationResultV1(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.ValidateFor(request); err == nil || !contract.HasCode(err, contract.FaultPreconditionFailedV1) {
		t.Fatalf("old negotiation snapshot err=%v", err)
	}
}

func TestNegotiationSealRejectsRequiredOptionalOverlap(t *testing.T) {
	_, err := contract.SealNegotiationRequestV1(contract.NegotiationRequestV1{
		RequestID:            "negotiation-overlap",
		TraceID:              "trace-1",
		SupportedVersions:    []string{contract.AgentRunServiceContractVersionV1},
		RequiredCapabilities: []contract.CapabilityV1{contract.CapabilityAgentRunInspectV1},
		OptionalCapabilities: []contract.CapabilityV1{contract.CapabilityAgentRunInspectV1},
		RequestedUnixNano:    contract.NewWireUnixNanoV1(100),
		NotAfterUnixNano:     contract.NewWireUnixNanoV1(200),
	})
	if err == nil || !contract.HasCode(err, contract.FaultInvalidArgumentV1) {
		t.Fatalf("overlap err=%v", err)
	}
}

func TestNegotiationV1RejectsMissingRequiredAndTraceSplice(t *testing.T) {
	request := negotiationRequestV1(t)
	result, err := contract.SealNegotiationResultV1(contract.NegotiationResultV1{
		RequestDigest:       request.RequestDigest,
		TraceID:             request.TraceID,
		Disposition:         contract.NegotiationSelectedV1,
		SelectedVersion:     contract.AgentRunServiceContractVersionV1,
		GrantedCapabilities: []contract.CapabilityV1{contract.CapabilityAgentRunWatchV1},
		Window:              contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(100), ExpiresUnixNano: contract.NewWireUnixNanoV1(200)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(request); err == nil || !contract.HasCode(err, contract.FaultCapabilityUnavailableV1) {
		t.Fatalf("missing required capability err=%v", err)
	}
	result.TraceID = "trace-splice"
	result.ResultDigest = ""
	spliced, err := contract.SealNegotiationResultV1(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := spliced.ValidateFor(request); err == nil || !contract.HasCode(err, contract.FaultRevisionConflictV1) {
		t.Fatalf("trace splice err=%v", err)
	}
}

func TestNegotiationV1RejectsUnknownEnum(t *testing.T) {
	request := negotiationRequestV1(t)
	_, err := contract.SealNegotiationResultV1(contract.NegotiationResultV1{
		RequestDigest: request.RequestDigest,
		TraceID:       request.TraceID,
		Disposition:   contract.NegotiationDispositionV1("FUTURE_VALUE"),
		Window:        contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(100), ExpiresUnixNano: contract.NewWireUnixNanoV1(200)},
	})
	if err == nil || !contract.HasCode(err, contract.FaultInvalidArgumentV1) {
		t.Fatalf("unknown enum err=%v", err)
	}
}

func TestNegotiationV1RejectsUnknownCapabilityAndExpiredRequest(t *testing.T) {
	_, err := contract.SealNegotiationRequestV1(contract.NegotiationRequestV1{
		RequestID:            "negotiation-future-capability",
		TraceID:              "trace-1",
		SupportedVersions:    []string{contract.AgentRunServiceContractVersionV1},
		RequiredCapabilities: []contract.CapabilityV1{"future.capability"},
		RequestedUnixNano:    contract.NewWireUnixNanoV1(100),
		NotAfterUnixNano:     contract.NewWireUnixNanoV1(200),
	})
	if err == nil || !contract.HasCode(err, contract.FaultInvalidArgumentV1) {
		t.Fatalf("unknown capability err=%v", err)
	}
	request := negotiationRequestV1(t)
	if err := request.ValidateCurrentV1(contract.NewWireUnixNanoV1(200)); err == nil || !contract.HasCode(err, contract.FaultPreconditionFailedV1) {
		t.Fatalf("expired negotiation err=%v", err)
	}
}

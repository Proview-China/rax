package contract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
)

func inspectRequestV1(t *testing.T) contract.AgentRunInspectRequestV1 {
	t.Helper()
	request, err := contract.SealAgentRunInspectRequestV1(contract.AgentRunInspectRequestV1{
		RequestID:         "inspect-1",
		TraceID:           "trace-1",
		Target:            targetV1(t),
		RequestedUnixNano: contract.NewWireUnixNanoV1(100),
		NotAfterUnixNano:  contract.NewWireUnixNanoV1(200),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestAgentRunInspectV1BindsExactOwnerProjectionAndTrace(t *testing.T) {
	request := inspectRequestV1(t)
	projection, err := contract.SealOwnerProjectionV1(contract.OwnerProjectionV1{
		OwnerDomain:   "praxis.runtime",
		OwnerContract: "praxis.runtime/run-lifecycle-v3",
		CurrentRef:    request.Target.RunCurrent,
		State:         "running",
		Window: contract.WireValidityWindowV1{
			CheckedUnixNano: contract.NewWireUnixNanoV1(110),
			ExpiresUnixNano: contract.NewWireUnixNanoV1(190),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := contract.SealAgentRunInspectResultV1(contract.AgentRunInspectResultV1{
		RequestDigest: request.RequestDigest,
		TraceID:       request.TraceID,
		Target:        request.Target,
		Disposition:   contract.AgentRunInspectObservedV1,
		Projections:   []contract.OwnerProjectionV1{projection},
		Window: contract.WireValidityWindowV1{
			CheckedUnixNano: contract.NewWireUnixNanoV1(110),
			ExpiresUnixNano: contract.NewWireUnixNanoV1(190),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(request); err != nil {
		t.Fatal(err)
	}
	old := result
	oldProjection := old.Projections[0]
	oldProjection.Window = contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(99), ExpiresUnixNano: contract.NewWireUnixNanoV1(190)}
	oldProjection.ProjectionDigest = ""
	oldProjection, err = contract.SealOwnerProjectionV1(oldProjection)
	if err != nil {
		t.Fatal(err)
	}
	old.Projections = []contract.OwnerProjectionV1{oldProjection}
	old.Window = contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(99), ExpiresUnixNano: contract.NewWireUnixNanoV1(190)}
	old.ResultDigest = ""
	old, err = contract.SealAgentRunInspectResultV1(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.ValidateFor(request); err == nil || !contract.HasCode(err, contract.FaultPreconditionFailedV1) {
		t.Fatalf("old Inspect snapshot err=%v", err)
	}
	result.TraceID = "trace-splice"
	result.ResultDigest = ""
	spliced, err := contract.SealAgentRunInspectResultV1(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := spliced.ValidateFor(request); err == nil || !contract.HasCode(err, contract.FaultRevisionConflictV1) {
		t.Fatalf("trace splice err=%v", err)
	}
}

func TestAgentRunInspectV1UnknownDispositionRejected(t *testing.T) {
	request := inspectRequestV1(t)
	_, err := contract.SealAgentRunInspectResultV1(contract.AgentRunInspectResultV1{
		RequestDigest: request.RequestDigest,
		TraceID:       request.TraceID,
		Target:        request.Target,
		Disposition:   contract.AgentRunInspectDispositionV1("FUTURE_VALUE"),
		Window:        contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(110), ExpiresUnixNano: contract.NewWireUnixNanoV1(190)},
	})
	if err == nil || !contract.HasCode(err, contract.FaultInvalidArgumentV1) {
		t.Fatalf("unknown disposition err=%v", err)
	}
}

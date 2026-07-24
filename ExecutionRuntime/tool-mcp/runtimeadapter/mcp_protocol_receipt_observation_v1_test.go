package runtimeadapter_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/runtimeadapter"
)

type receiptExactReaderV1 struct {
	value toolcontract.MCPProtocolReceiptV1
}

func (r *receiptExactReaderV1) InspectMCPProtocolReceiptV1(_ context.Context, exact toolcontract.MCPProtocolReceiptRefV1) (toolcontract.MCPProtocolReceiptV1, error) {
	if r == nil || r.value.Ref != exact {
		return toolcontract.MCPProtocolReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "receipt not found")
	}
	return toolcontract.CloneMCPProtocolReceiptV1(r.value), nil
}

type commandExactReaderV1 struct {
	value toolcontract.MCPExecutionCommandFactV1
}

type commandAttemptReaderV1 struct {
	value toolcontract.MCPExecutionCommandFactV1
}

func (r *commandAttemptReaderV1) InspectMCPExecutionCommandByAttemptV1(_ context.Context, exact runtimeports.OperationDispatchAttemptRefV3) (toolcontract.MCPExecutionCommandFactV1, error) {
	if r == nil || r.value.Attempt != exact {
		return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "command Attempt not found")
	}
	return toolcontract.CloneMCPExecutionCommandFactV1(r.value), nil
}

type receiptIDReaderV1 struct {
	value toolcontract.MCPProtocolReceiptV1
}

func (r *receiptIDReaderV1) InspectMCPProtocolReceiptByIDV1(_ context.Context, id string) (toolcontract.MCPProtocolReceiptV1, error) {
	if r == nil || r.value.Ref.ID != id {
		return toolcontract.MCPProtocolReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "receipt ID not found")
	}
	return toolcontract.CloneMCPProtocolReceiptV1(r.value), nil
}

type formalObservationReaderV1 struct {
	value runtimeports.ProviderAttemptObservationRefV2
}

func (r *formalObservationReaderV1) InspectOperationProviderReceiptObservationV1(_ context.Context, delegation runtimeports.ExecutionDelegationRefV2, preparedID string) (runtimeports.ProviderAttemptObservationRefV2, error) {
	if r == nil || r.value.Delegation != delegation || r.value.PreparedAttemptID != preparedID {
		return runtimeports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "formal observation not found")
	}
	return r.value, nil
}

func (r *commandExactReaderV1) InspectMCPExecutionCommandV1(_ context.Context, exact toolcontract.MCPExecutionCommandRefV1) (toolcontract.MCPExecutionCommandFactV1, error) {
	if r == nil || r.value.Ref != exact {
		return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "command not found")
	}
	return toolcontract.CloneMCPExecutionCommandFactV1(r.value), nil
}

func TestMCPProtocolReceiptObservationReaderExactProjection(t *testing.T) {
	now := testkit.FixedTime
	fixture := testkit.MCPExecutionV1(now)
	receipt := protocolReceiptFixtureV1(t, fixture, now.Add(time.Second))
	reader, err := runtimeadapter.NewMCPProtocolReceiptObservationReaderV1(&receiptExactReaderV1{receipt}, &commandExactReaderV1{fixture.Command}, testkit.SettlementOwner(), func() time.Time { return now.Add(2 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	exact, err := runtimeadapter.MCPProtocolReceiptRuntimeRefV1(testkit.SettlementOwner(), receipt.Ref)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := reader.InspectOperationProviderReceiptV1(context.Background(), exact)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ValidateExact(exact, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if projection.Prepared != fixture.Command.Prepared || projection.Attempt.AttemptID != fixture.Command.Attempt.AttemptID || projection.Provider != fixture.Provider || projection.Payload.ContentDigest != receipt.ResponseDigest || projection.Payload.Ref == "" || projection.ProviderOperationRef != receipt.Ref.ID {
		t.Fatal("Runtime projection lost exact MCP receipt coordinates")
	}
}

func TestMCPProtocolReceiptObservationReaderFailClosed(t *testing.T) {
	now := testkit.FixedTime
	fixture := testkit.MCPExecutionV1(now)
	receipt := protocolReceiptFixtureV1(t, fixture, now.Add(time.Second))
	exact, _ := runtimeadapter.MCPProtocolReceiptRuntimeRefV1(testkit.SettlementOwner(), receipt.Ref)

	var typedNil *receiptExactReaderV1
	if _, err := runtimeadapter.NewMCPProtocolReceiptObservationReaderV1(typedNil, &commandExactReaderV1{fixture.Command}, testkit.SettlementOwner(), time.Now); err == nil {
		t.Fatal("typed-nil receipt reader passed constructor")
	}
	reader, err := runtimeadapter.NewMCPProtocolReceiptObservationReaderV1(&receiptExactReaderV1{receipt}, &commandExactReaderV1{fixture.Command}, testkit.SettlementOwner(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.InspectOperationProviderReceiptV1(nil, exact); err == nil {
		t.Fatal("nil context passed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.InspectOperationProviderReceiptV1(ctx, exact); err != context.Canceled {
		t.Fatalf("canceled context changed: %v", err)
	}
	if _, err := reader.InspectOperationProviderReceiptV1(context.Background(), exact); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock regression passed: %v", err)
	}
	drift := exact
	drift.Digest = testkit.Digest("another-receipt")
	if _, err := reader.InspectOperationProviderReceiptV1(context.Background(), drift); err == nil {
		t.Fatal("same receipt ID with another digest passed")
	}
}

func TestMCPProviderObservationReaderV1JoinsExactFormalObservation(t *testing.T) {
	now := testkit.FixedTime
	fixture := testkit.MCPExecutionV1(now)
	receipt := protocolReceiptFixtureV1(t, fixture, now.Add(time.Second))
	observation := formalObservationRefV1(t, fixture, receipt)
	reader, err := runtimeadapter.NewMCPProviderObservationReaderV1(&commandAttemptReaderV1{fixture.Command}, &receiptIDReaderV1{receipt}, &formalObservationReaderV1{observation})
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := reader.InspectSingleCallToolProviderObservationV1(context.Background(), fixture.Command.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Observation != observation || inspection.Prepared != fixture.Command.Prepared || inspection.PayloadDigest != receipt.ResponseDigest || inspection.Outcome != toolcontract.ToolOutcomeSucceededV2 || inspection.Disposition != toolcontract.ToolDispositionConfirmedAppliedV2 || inspection.Residuals == nil {
		t.Fatal("MCP formal Observation join lost exact Tool semantics")
	}

	receipt.ToolError = true
	receipt.Ref = toolcontract.MCPProtocolReceiptRefV1{}
	receipt, err = toolcontract.SealMCPProtocolReceiptV1(receipt)
	if err != nil {
		t.Fatal(err)
	}
	observation = formalObservationRefV1(t, fixture, receipt)
	reader, _ = runtimeadapter.NewMCPProviderObservationReaderV1(&commandAttemptReaderV1{fixture.Command}, &receiptIDReaderV1{receipt}, &formalObservationReaderV1{observation})
	inspection, err = reader.InspectSingleCallToolProviderObservationV1(context.Background(), fixture.Command.Attempt)
	if err != nil || inspection.Outcome != toolcontract.ToolOutcomeFailedV2 || inspection.Disposition != toolcontract.ToolDispositionConfirmedAppliedV2 {
		t.Fatalf("ToolError mapping=%s/%s err=%v", inspection.Outcome, inspection.Disposition, err)
	}
}

func TestMCPProviderObservationReaderV1FailsClosed(t *testing.T) {
	now := testkit.FixedTime
	fixture := testkit.MCPExecutionV1(now)
	receipt := protocolReceiptFixtureV1(t, fixture, now.Add(time.Second))
	observation := formalObservationRefV1(t, fixture, receipt)
	var typedNil *commandAttemptReaderV1
	if _, err := runtimeadapter.NewMCPProviderObservationReaderV1(typedNil, &receiptIDReaderV1{receipt}, &formalObservationReaderV1{observation}); err == nil {
		t.Fatal("typed-nil command reader passed")
	}
	reader, err := runtimeadapter.NewMCPProviderObservationReaderV1(&commandAttemptReaderV1{fixture.Command}, &receiptIDReaderV1{receipt}, &formalObservationReaderV1{observation})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reader.InspectSingleCallToolProviderObservationV1(nil, fixture.Command.Attempt); err == nil {
		t.Fatal("nil context passed")
	}
	drift := observation
	drift.PayloadDigest = testkit.Digest("other-payload")
	reader, _ = runtimeadapter.NewMCPProviderObservationReaderV1(&commandAttemptReaderV1{fixture.Command}, &receiptIDReaderV1{receipt}, &formalObservationReaderV1{drift})
	if _, err = reader.InspectSingleCallToolProviderObservationV1(context.Background(), fixture.Command.Attempt); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("payload splice error=%v", err)
	}
}

func protocolReceiptFixtureV1(t *testing.T, fixture testkit.MCPExecutionFixtureV1, observed time.Time) toolcontract.MCPProtocolReceiptV1 {
	t.Helper()
	admission, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{ID: "admission-receipt-reader", Revision: 1, StableKeyDigest: fixture.Authorization.StableKeyDigest, Admitted: true})
	if err != nil {
		t.Fatal(err)
	}
	response := []byte(`{"content":[{"type":"text","text":"ok"}]}`)
	receipt, err := toolcontract.SealMCPProtocolReceiptV1(toolcontract.MCPProtocolReceiptV1{Command: fixture.Command.Ref, StableKeyDigest: fixture.Authorization.StableKeyDigest, AdmissionReceipt: admission, JSONRPCRequestID: fixture.Command.JSONRPCRequestID, CanonicalResponse: response, ResponseDigest: core.DigestBytes(response), ObservedUnixNano: observed.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func formalObservationRefV1(t *testing.T, fixture testkit.MCPExecutionFixtureV1, receipt toolcontract.MCPProtocolReceiptV1) runtimeports.ProviderAttemptObservationRefV2 {
	t.Helper()
	if fixture.Command.Attempt.Delegation == nil {
		t.Fatal("fixture Attempt lacks Delegation")
	}
	value := runtimeports.ProviderAttemptObservationRefV2{
		Delegation: *fixture.Command.Attempt.Delegation, PreparedAttemptID: fixture.Command.Prepared.ID,
		ProviderOperationRef: receipt.Ref.ID, Revision: 1, State: runtimeports.ProviderAttemptObservedV2,
		Digest: testkit.Digest("formal-observation-" + receipt.Ref.ID), PayloadDigest: receipt.ResponseDigest, PayloadRevision: 1,
		SourceRegistrationID: "receipt-source", SourceEpoch: 1, SourceSequence: 1,
		Evidence:         runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: testkit.Digest("receipt-ledger"), Sequence: 1, RecordDigest: testkit.Digest("receipt-record-" + receipt.Ref.ID)},
		ObservedUnixNano: receipt.ObservedUnixNano,
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	return value
}

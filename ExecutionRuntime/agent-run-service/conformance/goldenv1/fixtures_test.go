package goldenv1_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/conformance/goldenv1"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/transport/jsonv1"
)

func TestCommittedFixturesEqualFreshGenerationV1(t *testing.T) {
	fresh := goldenv1.BuildV1()
	if len(fresh) != 19 {
		t.Fatalf("fixtures=%d want=19", len(fresh))
	}
	for _, name := range goldenv1.NamesV1() {
		committed := readFixtureV1(t, name)
		if !bytes.Equal(committed, fresh[name]) {
			t.Fatalf("fixture %s differs from fresh generation", name)
		}
	}
}

func TestSixServiceMethodsStrictRoundTripAndValidateForV1(t *testing.T) {
	var negotiateRequest contract.NegotiationRequestV1
	strictFixtureV1(t, "negotiate-request.json", &negotiateRequest)
	if err := negotiateRequest.Validate(); err != nil {
		t.Fatal(err)
	}
	var negotiateResult contract.NegotiationResultV1
	strictFixtureV1(t, "negotiate-result.json", &negotiateResult)
	if err := negotiateResult.ValidateFor(negotiateRequest); err != nil {
		t.Fatal(err)
	}

	var inspectRequest contract.AgentRunInspectRequestV1
	strictFixtureV1(t, "inspect-agent-run-request.json", &inspectRequest)
	if err := inspectRequest.Validate(); err != nil {
		t.Fatal(err)
	}
	var inspectResult contract.AgentRunInspectResultV1
	strictFixtureV1(t, "inspect-agent-run-result.json", &inspectResult)
	if err := inspectResult.ValidateFor(inspectRequest); err != nil {
		t.Fatal(err)
	}

	var originalRequest contract.InspectOriginalRequestV1
	strictFixtureV1(t, "inspect-original-request.json", &originalRequest)
	if err := originalRequest.Validate(); err != nil {
		t.Fatal(err)
	}
	var originalResult contract.InspectOriginalResultV1
	strictFixtureV1(t, "inspect-original-result.json", &originalResult)
	if err := originalResult.ValidateFor(originalRequest); err != nil {
		t.Fatal(err)
	}

	var watchRequest contract.AgentRunWatchRequestV1
	strictFixtureV1(t, "watch-agent-run-request.json", &watchRequest)
	if err := watchRequest.Validate(); err != nil {
		t.Fatal(err)
	}
	var watchResult contract.AgentRunWatchResultV1
	strictFixtureV1(t, "watch-agent-run-result.json", &watchResult)
	if err := watchResult.ValidateFor(watchRequest); err != nil {
		t.Fatal(err)
	}

	var cancelRequest contract.CancelAgentRunRequestV1
	strictFixtureV1(t, "cancel-agent-run-request.json", &cancelRequest)
	if err := cancelRequest.Validate(); err != nil {
		t.Fatal(err)
	}
	var cancelResult contract.CommandResultV1
	strictFixtureV1(t, "cancel-agent-run-result.json", &cancelResult)
	if err := cancelResult.ValidateFor(cancelRequest.Command); err != nil {
		t.Fatal(err)
	}

	var stopRequest contract.StopAgentHostRequestV1
	strictFixtureV1(t, "stop-agent-host-request.json", &stopRequest)
	if err := stopRequest.Validate(); err != nil {
		t.Fatal(err)
	}
	var stopResult contract.CommandResultV1
	strictFixtureV1(t, "stop-agent-host-result.json", &stopResult)
	if err := stopResult.ValidateFor(stopRequest.Command); err != nil {
		t.Fatal(err)
	}
}

func TestWireScalarAndExactRefCoverageV1(t *testing.T) {
	var fixture struct {
		Descriptor contract.CrossLanguageWirePrimitivesV1 `json:"descriptor"`
		Uint64Min  contract.WireUint64V1                  `json:"uint64_min"`
		Uint64Max  contract.WireUint64V1                  `json:"uint64_max"`
		Int64Min   contract.WireInt64V1                   `json:"int64_min"`
		Int64Max   contract.WireInt64V1                   `json:"int64_max"`
		UnixNano   contract.WireUnixNanoV1                `json:"unix_nano"`
		ExactRef   contract.ExactRefWireV1                `json:"exact_ref"`
	}
	strictFixtureV1(t, "wire-primitives-v1.json", &fixture)
	if err := fixture.Descriptor.Validate(); err != nil {
		t.Fatal(err)
	}
	if value, err := fixture.Uint64Min.Uint64V1(); err != nil || value != 0 {
		t.Fatalf("uint64 min=%d err=%v", value, err)
	}
	if value, err := fixture.Uint64Max.Uint64V1(); err != nil || value != ^uint64(0) {
		t.Fatalf("uint64 max=%d err=%v", value, err)
	}
	if value, err := fixture.Int64Min.Int64V1(); err != nil || value != -1<<63 {
		t.Fatalf("int64 min=%d err=%v", value, err)
	}
	if value, err := fixture.Int64Max.Int64V1(); err != nil || value != 1<<63-1 {
		t.Fatalf("int64 max=%d err=%v", value, err)
	}
	if err := fixture.UnixNano.ValidatePositiveV1("fixture UnixNano"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.ExactRef.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestIdempotencyReplayAndConflictGoldenV1(t *testing.T) {
	var original, replay, conflict contract.AgentRunCommandEnvelopeV1
	strictFixtureV1(t, "idempotency-original.json", &original)
	strictFixtureV1(t, "idempotency-replay.json", &replay)
	strictFixtureV1(t, "idempotency-conflict.json", &conflict)
	classification, err := contract.ClassifyAgentRunCommandReplayV1(original, replay)
	if err != nil || classification != contract.IdempotencyReplayOriginalV1 {
		t.Fatalf("classification=%s err=%v", classification, err)
	}
	if _, err := contract.ClassifyAgentRunCommandReplayV1(original, conflict); err == nil || !contract.HasCode(err, contract.FaultIdempotencyConflictV1) {
		t.Fatalf("conflict err=%v", err)
	}
}

func TestCursorResumeRetentionGapAndResyncGoldenV1(t *testing.T) {
	var request contract.AgentRunWatchRequestV1
	strictFixtureV1(t, "watch-agent-run-resync-request.json", &request)
	if request.Cursor == nil || request.AfterSequence != request.Cursor.Sequence {
		t.Fatal("resume fixture lost exact cursor")
	}
	var result contract.AgentRunWatchResultV1
	strictFixtureV1(t, "watch-agent-run-resync-result.json", &result)
	if result.Disposition != contract.AgentRunWatchResyncRequiredV1 || result.Fault == nil || result.Fault.CurrentRef == nil || *result.Fault.CurrentRef != request.RetentionCurrentRef {
		t.Fatal("retention gap fixture lost exact RESYNC_REQUIRED current")
	}
	if err := result.ValidateFor(request); err != nil {
		t.Fatal(err)
	}
}

func TestFaultMapAndUnknownEnumsFailClosedV1(t *testing.T) {
	var faults []contract.FaultV1
	strictFixtureV1(t, "fault-map-v1.json", &faults)
	if len(faults) != 13 {
		t.Fatalf("faults=%d want=13", len(faults))
	}
	for _, fault := range faults {
		if err := fault.Validate(); err != nil {
			t.Fatalf("code=%s err=%v", fault.Code, err)
		}
	}

	payload := readFixtureV1(t, "negotiate-result.json")
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatal(err)
	}
	raw["disposition"] = "FUTURE_VALUE"
	mutated, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var result contract.NegotiationResultV1
	if err := jsonv1.NewDecoderV1(1<<20).DecodeStrictV1(mutated, &result); err != nil {
		t.Fatalf("strict structural decode err=%v", err)
	}
	if err := result.Validate(); err == nil || !contract.HasCode(err, contract.FaultInvalidArgumentV1) {
		t.Fatalf("unknown enum err=%v", err)
	}
}

func TestOptionalAbsentAndExplicitNullRejectedV1(t *testing.T) {
	payload := readFixtureV1(t, "inspect-original-request.json")
	if bytes.Contains(payload, []byte("original_attempt_ref")) {
		t.Fatal("absent optional attempt was serialized")
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatal(err)
	}
	raw["original_attempt_ref"] = nil
	mutated, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var request contract.InspectOriginalRequestV1
	err = jsonv1.NewDecoderV1(1<<20).DecodeStrictV1(mutated, &request)
	if err == nil || !strings.Contains(err.Error(), "json_optional_null_forbidden") {
		t.Fatalf("explicit null err=%v", err)
	}
}

func strictFixtureV1(t *testing.T, name string, target any) {
	t.Helper()
	payload := readFixtureV1(t, name)
	if err := jsonv1.NewDecoderV1(1<<20).DecodeStrictV1(payload, target); err != nil {
		t.Fatalf("strict decode %s: %v", name, err)
	}
	roundTrip, err := json.MarshalIndent(target, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	roundTrip = append(roundTrip, '\n')
	if !bytes.Equal(payload, roundTrip) {
		t.Fatalf("Go round-trip changed %s", name)
	}
}

func readFixtureV1(t *testing.T, name string) []byte {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate fixture test")
	}
	payload, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "wire-v1", name))
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

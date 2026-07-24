package preparedinvocation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestFactSealIdentityAndRegistryExactReader(t *testing.T) {
	reader := &exactRegistryReader{ref: registryRef()}
	fact, err := modelinvoker.PrepareModelInvocationFactV1(context.Background(), reader, draftFact())
	if err != nil {
		t.Fatal(err)
	}
	if reader.calls.Load() != 1 || fact.InvocationDigest != fact.UnifiedRequestDigest || fact.Ref().Digest != fact.Digest {
		t.Fatalf("sealed fact = %#v, registry reads=%d", fact, reader.calls.Load())
	}
	second, err := modelinvoker.SealPreparedModelInvocationFactV1(draftFact())
	if err != nil || second.ID != fact.ID || second.Digest != fact.Digest {
		t.Fatalf("deterministic seal = %#v, %v", second, err)
	}

	drifted := registryRef()
	drifted.Revision++
	reader.ref = drifted
	if _, err := modelinvoker.PrepareModelInvocationFactV1(context.Background(), reader, draftFact()); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("registry drift error = %v", err)
	}
}

func TestRegistryReaderTypedNilAndUnavailableFailClosed(t *testing.T) {
	var typedNil *exactRegistryReader
	if _, err := modelinvoker.PrepareModelInvocationFactV1(context.Background(), typedNil, draftFact()); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Registry Reader = %v", err)
	}
	reader := &exactRegistryReader{ref: registryRef(), err: core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "registry unavailable")}
	if _, err := modelinvoker.PrepareModelInvocationFactV1(context.Background(), reader, draftFact()); !core.HasCategory(err, core.ErrorUnavailable) || reader.calls.Load() != 1 {
		t.Fatalf("Registry unavailable = %v, calls=%d", err, reader.calls.Load())
	}
}

func TestFactStableIDIncludesContractVersion(t *testing.T) {
	fact := sealedFact()
	wire, err := modelinvoker.EncodePreparedModelInvocationFactV1(fact)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire, []byte(modelinvoker.PreparedModelInvocationContractVersionV1)) {
		t.Fatalf("wire omits contract version: %s", wire)
	}
	expectedDigest, err := core.CanonicalJSONDigest(
		"praxis.model-invoker.prepared-model-invocation",
		"v1",
		"PreparedModelInvocationIdentityV1",
		struct {
			ContractVersion  string      `json:"contract_version"`
			InvocationID     string      `json:"invocation_id"`
			InvocationDigest core.Digest `json:"invocation_digest"`
		}{
			ContractVersion:  modelinvoker.PreparedModelInvocationContractVersionV1,
			InvocationID:     fact.InvocationID,
			InvocationDigest: fact.InvocationDigest,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := "prepared-model-invocation/" + strings.TrimPrefix(string(expectedDigest), "sha256:"); fact.ID != want {
		t.Fatalf("stable ID = %s, want %s", fact.ID, want)
	}
	ref := fact.Ref()
	ref.ContractVersion = "praxis.model-invoker.prepared-model-invocation/v2"
	if err := ref.Validate(); err == nil {
		t.Fatal("changed contract version was accepted")
	}
}

func TestProgrammaticInvalidUTF8FailsBeforeSeal(t *testing.T) {
	draft := draftFact()
	draft.InvocationID = string([]byte{'i', 0xff})
	if _, err := modelinvoker.SealPreparedModelInvocationFactV1(draft); err == nil {
		t.Fatal("invalid UTF-8 InvocationID was accepted")
	}
	draft = draftFact()
	draft.RegistrySnapshotRef.Owner.Domain = string([]byte{'r', 0xff})
	if _, err := modelinvoker.SealPreparedModelInvocationFactV1(draft); err == nil {
		t.Fatal("invalid UTF-8 Registry owner was accepted")
	}
}

func TestRuntimeRegistryNominalIdentity(t *testing.T) {
	factType := reflect.TypeOf(modelinvoker.PreparedModelInvocationFactV1{})
	currentType := reflect.TypeOf(modelinvoker.PreparedModelInvocationCurrentProjectionV1{})
	want := reflect.TypeOf(runtimeports.RegistrySnapshotRefV1{})
	for _, candidate := range []reflect.Type{factType, currentType} {
		field, ok := candidate.FieldByName("RegistrySnapshotRef")
		if !ok || field.Type != want {
			t.Fatalf("%s RegistrySnapshotRef type = %v", candidate, field.Type)
		}
	}
}

func TestCurrentAckAndReceiptTimeBoundaries(t *testing.T) {
	fact := sealedFact()
	current := sealedCurrent(fact)
	ack := sealedAck(fact, current)
	if err := current.ValidateAgainstFact(fact); err != nil {
		t.Fatal(err)
	}
	if err := ack.ValidateCurrent(current, time.Unix(0, 4_000)); err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*modelinvoker.PreparedModelInvocationCommitAckV1){
		"checked before current": func(value *modelinvoker.PreparedModelInvocationCommitAckV1) { value.CheckedUnixNano = 1_999 },
		"checked equals expires": func(value *modelinvoker.PreparedModelInvocationCommitAckV1) {
			value.CheckedUnixNano = value.ExpiresUnixNano
		},
		"expires after current": func(value *modelinvoker.PreparedModelInvocationCommitAckV1) { value.ExpiresUnixNano = 8_001 },
		"not-after drift":       func(value *modelinvoker.PreparedModelInvocationCommitAckV1) { value.NotAfterUnixNano++ },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := ack
			candidate.ID, candidate.Digest = "", ""
			mutate(&candidate)
			if _, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(candidate); err == nil {
				t.Fatal("invalid ACK was sealed")
			}
		})
	}

	draft := modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1{
		PreparedRef: fact.Ref(), CurrentRef: current.Ref(), AckRef: ack.Ref(),
		DispatchSequence: 1, BoundaryKind: "provider.invoke", ProviderAttemptOrdinal: 1,
		AttemptRequestDigest:          digest("attempt"),
		ActualToolSurfaceDigest:       fact.ActualToolSurfaceDigest,
		ActualProviderInjectionDigest: fact.ActualProviderInjectionDigest,
		CheckedUnixNano:               4_000,
	}
	receipt, err := modelinvoker.SealPreparedModelInvocationDispatchReceiptAgainstV1(fact, current, ack, draft, time.Unix(0, 4_000))
	if err != nil || receipt.Validate() != nil {
		t.Fatalf("valid receipt = %#v, %v", receipt, err)
	}

	for _, checked := range []int64{1_999, 2_999, 7_000, 8_000} {
		candidate := draft
		candidate.CheckedUnixNano = checked
		if _, err := modelinvoker.SealPreparedModelInvocationDispatchReceiptV1(candidate); err == nil {
			t.Fatalf("receipt checked=%d was accepted", checked)
		}
	}
	if err := current.ValidateCurrent(current.Ref(), time.Unix(0, 1_999)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback error = %v", err)
	}
	if err := current.ValidateCurrent(current.Ref(), time.Unix(0, 8_000)); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expiry boundary error = %v", err)
	}
}

func TestStrictCodecsRejectDuplicateUnknownTrailingAndNonCanonical(t *testing.T) {
	fact := sealedFact()
	wire, err := modelinvoker.EncodePreparedModelInvocationFactV1(fact)
	if err != nil {
		t.Fatal(err)
	}
	duplicateTop := bytes.Replace(wire, []byte(`{"contract_version":`), []byte(`{"contract_version":"duplicate","contract_version":`), 1)
	duplicateNested := bytes.Replace(wire, []byte(`"owner":{"domain":`), []byte(`"owner":{"domain":"duplicate","domain":`), 1)
	unknown := append(append(json.RawMessage(nil), wire[:len(wire)-1]...), []byte(`,"unknown":true}`)...)
	trailing := append(append(json.RawMessage(nil), wire...), []byte(` {}`)...)
	nonCanonical := append(json.RawMessage{' '}, wire...)
	invalidUTF8 := append(append(json.RawMessage(nil), wire[:len(wire)-1]...), 0xff, '}')
	for name, payload := range map[string]json.RawMessage{
		"duplicate top":    duplicateTop,
		"duplicate nested": duplicateNested,
		"unknown":          unknown,
		"trailing":         trailing,
		"noncanonical":     nonCanonical,
		"invalid utf8":     invalidUTF8,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := modelinvoker.DecodePreparedModelInvocationFactV1(payload); err == nil {
				t.Fatalf("payload accepted: %q", payload)
			}
		})
	}
}

func TestAllStrictCodecsRoundTripExact(t *testing.T) {
	fact := sealedFact()
	current := sealedCurrent(fact)
	ack := sealedAck(fact, current)
	receipt, err := modelinvoker.SealPreparedModelInvocationDispatchReceiptAgainstV1(
		fact,
		current,
		ack,
		modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1{
			PreparedRef: fact.Ref(), CurrentRef: current.Ref(), AckRef: ack.Ref(),
			DispatchSequence: 1, BoundaryKind: "provider.stream", ProviderAttemptOrdinal: 1,
			AttemptRequestDigest:          digest("attempt-roundtrip"),
			ActualToolSurfaceDigest:       fact.ActualToolSurfaceDigest,
			ActualProviderInjectionDigest: fact.ActualProviderInjectionDigest,
			CheckedUnixNano:               4_000,
		},
		time.Unix(0, 4_000),
	)
	if err != nil {
		t.Fatal(err)
	}

	factWire, err := modelinvoker.EncodePreparedModelInvocationFactV1(fact)
	if err != nil {
		t.Fatal(err)
	}
	decodedFact, err := modelinvoker.DecodePreparedModelInvocationFactV1(factWire)
	if err != nil || decodedFact != fact {
		t.Fatalf("Fact round trip = %#v, %v", decodedFact, err)
	}
	currentWire, err := modelinvoker.EncodePreparedModelInvocationCurrentV1(current)
	if err != nil {
		t.Fatal(err)
	}
	decodedCurrent, err := modelinvoker.DecodePreparedModelInvocationCurrentV1(currentWire)
	if err != nil || decodedCurrent != current {
		t.Fatalf("Current round trip = %#v, %v", decodedCurrent, err)
	}
	ackWire, err := modelinvoker.EncodePreparedModelInvocationCommitAckV1(ack)
	if err != nil {
		t.Fatal(err)
	}
	decodedAck, err := modelinvoker.DecodePreparedModelInvocationCommitAckV1(ackWire)
	if err != nil || decodedAck != ack {
		t.Fatalf("ACK round trip = %#v, %v", decodedAck, err)
	}
	receiptWire, err := modelinvoker.EncodePreparedModelInvocationDispatchReceiptV1(receipt)
	if err != nil {
		t.Fatal(err)
	}
	decodedReceipt, err := modelinvoker.DecodePreparedModelInvocationDispatchReceiptV1(receiptWire)
	if err != nil || decodedReceipt != receipt {
		t.Fatalf("Receipt round trip = %#v, %v", decodedReceipt, err)
	}
}

type gateFake struct {
	ack          modelinvoker.PreparedModelInvocationCommitAckV1
	commitErr    error
	inspectErr   error
	inspectAck   modelinvoker.PreparedModelInvocationCommitAckV1
	commitCalls  atomic.Uint64
	inspectCalls atomic.Uint64
}

func (g *gateFake) Commit(context.Context, modelinvoker.PreparedModelInvocationRefV1, modelinvoker.PreparedModelInvocationCurrentRefV1) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	g.commitCalls.Add(1)
	return g.ack, g.commitErr
}

func (g *gateFake) InspectExactAck(context.Context, modelinvoker.PreparedModelInvocationCommitAckRefV1) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	g.inspectCalls.Add(1)
	return g.inspectAck, g.inspectErr
}

func TestGateLostReplyUsesExactInspectNeverSecondCommit(t *testing.T) {
	fact := sealedFact()
	current := sealedCurrent(fact)
	ack := sealedAck(fact, current)
	indeterminate := core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "reply lost")
	gate := &gateFake{ack: ack, commitErr: indeterminate, inspectAck: ack}
	got, err := modelinvoker.CrossPreparedModelInvocationCommitGateV1(context.Background(), gate, fact.Ref(), current.Ref())
	if err != nil || got.Ref() != ack.Ref() || gate.commitCalls.Load() != 1 || gate.inspectCalls.Load() != 1 {
		t.Fatalf("lost reply = %#v/%v commit=%d inspect=%d", got, err, gate.commitCalls.Load(), gate.inspectCalls.Load())
	}

	gate = &gateFake{commitErr: indeterminate}
	if _, err := modelinvoker.CrossPreparedModelInvocationCommitGateV1(context.Background(), gate, fact.Ref(), current.Ref()); err == nil || gate.commitCalls.Load() != 1 || gate.inspectCalls.Load() != 0 {
		t.Fatalf("unknown without stable ref = %v commit=%d inspect=%d", err, gate.commitCalls.Load(), gate.inspectCalls.Load())
	}

	gate = &gateFake{ack: ack, commitErr: indeterminate, inspectErr: errors.New("unavailable")}
	if _, err := modelinvoker.CrossPreparedModelInvocationCommitGateV1(context.Background(), gate, fact.Ref(), current.Ref()); err == nil || gate.commitCalls.Load() != 1 || gate.inspectCalls.Load() != 1 {
		t.Fatalf("inspect unavailable = %v commit=%d inspect=%d", err, gate.commitCalls.Load(), gate.inspectCalls.Load())
	}
}

func TestGateSuccessAndExactReader(t *testing.T) {
	fact := sealedFact()
	current := sealedCurrent(fact)
	ack := sealedAck(fact, current)
	gate := &gateFake{ack: ack, inspectAck: ack}
	got, err := modelinvoker.CrossPreparedModelInvocationCommitGateV1(context.Background(), gate, fact.Ref(), current.Ref())
	if err != nil || got != ack || gate.commitCalls.Load() != 1 || gate.inspectCalls.Load() != 0 {
		t.Fatalf("Commit = %#v/%v commit=%d inspect=%d", got, err, gate.commitCalls.Load(), gate.inspectCalls.Load())
	}
	got, err = modelinvoker.InspectPreparedModelInvocationCommitAckV1(context.Background(), gate, ack.Ref())
	if err != nil || got != ack || gate.inspectCalls.Load() != 1 {
		t.Fatalf("Inspect = %#v/%v inspect=%d", got, err, gate.inspectCalls.Load())
	}
}

func TestGateMethodSetAndAckShape(t *testing.T) {
	gateType := reflect.TypeOf((*modelinvoker.PreparedModelInvocationCommitGateV1)(nil)).Elem()
	if gateType.NumMethod() != 2 {
		t.Fatalf("Gate methods = %d", gateType.NumMethod())
	}
	for _, name := range []string{"Commit", "InspectExactAck"} {
		if _, ok := gateType.MethodByName(name); !ok {
			t.Fatalf("Gate lacks %s", name)
		}
	}
	ackType := reflect.TypeOf(modelinvoker.PreparedModelInvocationCommitAckV1{})
	for index := 0; index < ackType.NumField(); index++ {
		if strings.Contains(strings.ToLower(ackType.Field(index).Name), "toolack") {
			t.Fatalf("ACK contains forbidden Tool ACK field %s", ackType.Field(index).Name)
		}
	}
}

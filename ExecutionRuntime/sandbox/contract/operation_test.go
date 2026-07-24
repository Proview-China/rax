package contract_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
)

func TestDomainResultRejectsCombinedEffects(t *testing.T) {
	t.Parallel()
	reservation := testkit.Reservation(contract.EffectClose, 1, "combined")
	observation := testkit.Observation(reservation, 1, "combined")
	inspection := testkit.Inspection(reservation, observation, contract.DispositionConfirmedApplied, "combined")
	result := testkit.Result(reservation, inspection, contract.DomainResultPayload{
		EnvironmentClosed: true,
		ExecutionQuiesced: true,
	}, "combined")
	if err := result.ValidateCurrent(testkit.FixedNow); err == nil {
		t.Fatal("close result combined environment-closed with execution-quiesced")
	}
}

func TestNoGoRuntimeSettlementRefIsOpaqueAndCarriesNoRuntimeSemantics(t *testing.T) {
	t.Parallel()
	typeOfRef := reflect.TypeOf(contract.RuntimeOperationSettlementRef{})
	wantFields := map[string]bool{
		"OpaqueRef":       true,
		"OperationID":     true,
		"AttemptID":       true,
		"DomainResultRef": true,
	}
	if typeOfRef.NumField() != len(wantFields) {
		t.Fatalf("RuntimeOperationSettlementRef has %d fields, want only opaque exact-binding coordinates", typeOfRef.NumField())
	}
	for index := 0; index < typeOfRef.NumField(); index++ {
		field := typeOfRef.Field(index)
		if !wantFields[field.Name] {
			t.Fatalf("RuntimeOperationSettlementRef contains forbidden semantic field %q", field.Name)
		}
	}

	valid := testkit.Settlement(testkit.Result(
		testkit.Reservation(contract.EffectAllocate, 1, "opaque"),
		testkit.Inspection(
			testkit.Reservation(contract.EffectAllocate, 1, "opaque"),
			testkit.Observation(testkit.Reservation(contract.EffectAllocate, 1, "opaque"), 1, "opaque"),
			contract.DispositionConfirmedApplied,
			"opaque",
		),
		contract.DomainResultPayload{AllocationConfirmed: true},
		"opaque",
	), "opaque")
	payload := `{"opaque_ref":{"id":"runtime","revision":1,"digest":"` + valid.OpaqueRef.Digest + `"},"operation_id":"op","attempt_id":"attempt","domain_result_ref":{"id":"result","revision":1,"digest":"` + valid.DomainResultRef.Digest + `"},"disposition":"confirmed_applied"}`
	if _, err := contract.DecodeStrict[contract.RuntimeOperationSettlementRef]([]byte(payload)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("legacy/runtime semantic copy was not rejected: %v", err)
	}
}

func TestUnknownResultCannotClaimAppliedPayload(t *testing.T) {
	t.Parallel()
	reservation := testkit.Reservation(contract.EffectAllocate, 1, "unknown-payload")
	observation := testkit.Observation(reservation, 1, "unknown-payload")
	inspection := testkit.Inspection(reservation, observation, contract.DispositionUnknown, "unknown-payload")
	result := testkit.Result(reservation, inspection, contract.DomainResultPayload{AllocationConfirmed: true}, "unknown-payload")
	if err := result.ValidateCurrent(testkit.FixedNow); err == nil {
		t.Fatal("unknown result claimed applied allocation")
	}
}

package conformance_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

func TestNoGoOperationSettlementRefContainsOnlyOpaqueIdentityAndDigests(t *testing.T) {
	typeOf := reflect.TypeOf(contract.OperationSettlementRef{})
	want := map[string]bool{
		"SettlementID": true, "SettlementDigest": true,
		"OperationID": true, "OperationDigest": true,
		"DomainResultFactID": true, "DomainResultDigest": true,
	}
	if typeOf.NumField() != len(want) {
		t.Fatalf("opaque ref has %d fields, want exactly %d", typeOf.NumField(), len(want))
	}
	for i := 0; i < typeOf.NumField(); i++ {
		field := typeOf.Field(i)
		if !want[field.Name] {
			t.Fatalf("NO-GO: Runtime semantic copy %q appeared in OperationSettlementRef", field.Name)
		}
	}
	for _, forbidden := range []string{"Outcome", "Disposition", "Status", "Result", "Success"} {
		if _, ok := typeOf.FieldByName(forbidden); ok {
			t.Fatalf("NO-GO: forbidden Runtime semantic field %s exists", forbidden)
		}
	}
}

func TestNoGoOperationSettlementRefRejectsSemanticWireFields(t *testing.T) {
	base := `{
		"settlement_id":"settlement-1",
		"settlement_digest":"settlement-digest-1",
		"operation_id":"operation-1",
		"operation_digest":"operation-digest-1",
		"domain_result_fact_id":"fact-1",
		"domain_result_digest":"fact-digest-1"`
	for _, semantic := range []string{
		`,"outcome":"unknown"}`,
		`,"disposition":"settled"}`,
		`,"status":"complete"}`,
		`,"result":"success"}`,
	} {
		var ref contract.OperationSettlementRef
		err := json.Unmarshal([]byte(base+semantic), &ref)
		if !contract.HasCode(err, contract.ErrInvalidArgument) {
			t.Fatalf("NO-GO semantic field was accepted: %s err=%v", semantic, err)
		}
	}
}

func TestNoGoAppliedSettlementDoesNotInterpretOutcome(t *testing.T) {
	typeOf := reflect.TypeOf(contract.AppliedSettlement{})
	for _, forbidden := range []string{"State", "Outcome", "Disposition", "Status", "Result"} {
		if _, ok := typeOf.FieldByName(forbidden); ok {
			t.Fatalf("NO-GO: ApplySettlement output interprets semantics through %s", forbidden)
		}
	}
}

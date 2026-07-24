package ports_test

import (
	"encoding/json"
	"testing"
	"time"

	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRestoreStageSettlementRefV1SemanticExactSurvivesPersistenceRoundTrip(t *testing.T) {
	now := time.Unix(1_990_000_000, 0)
	_, _, _, submission, err := runtimefakes.BuildRestoreStageSettlementFixtureV1("persistence", now)
	if err != nil {
		t.Fatal(err)
	}
	fact, err := runtimeports.SealRestoreStageSettlementFactV1(runtimeports.RestoreStageSettlementFactV1{Submission: submission})
	if err != nil {
		t.Fatal(err)
	}
	original := fact.RefV1()
	payload, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored runtimeports.RestoreStageSettlementRefV1
	if err := json.Unmarshal(payload, &restored); err != nil {
		t.Fatal(err)
	}
	if original.Validate() != nil || restored.Validate() != nil || !runtimeports.SameRestoreStageSettlementRefV1(original, restored) {
		t.Fatalf("persisted Restore Stage Settlement ref lost semantic exact identity")
	}
	restored.DomainResult.ID += "-drift"
	if runtimeports.SameRestoreStageSettlementRefV1(original, restored) {
		t.Fatal("changed DomainResult was accepted as the same Settlement ref")
	}
}

func TestRestoreStageApplySettlementRefV1SemanticExactSurvivesPersistenceRoundTrip(t *testing.T) {
	now := time.Unix(1_990_000_000, 0)
	_, _, _, submission, err := runtimefakes.BuildRestoreStageSettlementFixtureV1("apply-persistence", now)
	if err != nil {
		t.Fatal(err)
	}
	fact, err := runtimeports.SealRestoreStageSettlementFactV1(runtimeports.RestoreStageSettlementFactV1{Submission: submission})
	if err != nil {
		t.Fatal(err)
	}
	settlement := fact.RefV1()
	original := runtimeports.RestoreStageApplySettlementRefV1{
		Owner:             settlement.DomainResult.Owner,
		ID:                "restore-stage-apply-persistence",
		Revision:          1,
		Digest:            settlement.Digest,
		TenantID:          settlement.DomainResult.TenantID,
		DomainResult:      settlement.DomainResult,
		RuntimeSettlement: settlement,
	}
	if err := original.Validate(); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored runtimeports.RestoreStageApplySettlementRefV1
	if err := json.Unmarshal(payload, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Validate() != nil || !runtimeports.SameRestoreStageApplySettlementRefV1(original, restored) {
		t.Fatal("persisted Restore Stage ApplySettlement ref lost semantic exact identity")
	}
	restored.RuntimeSettlement.ID += "-drift"
	if runtimeports.SameRestoreStageApplySettlementRefV1(original, restored) {
		t.Fatal("changed Runtime Settlement was accepted as the same ApplySettlement ref")
	}
}

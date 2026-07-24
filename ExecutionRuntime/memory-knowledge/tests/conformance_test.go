package tests

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestWave1ConformanceRuntimeSettlementIsOpaqueRefOnly(t *testing.T) {
	t.Parallel()
	typeOf := reflect.TypeOf(contract.RuntimeSettlementRef{})
	if typeOf.NumField() != 1 || typeOf.Field(0).Name != "Ref" || typeOf.Field(0).Type != reflect.TypeOf(contract.Ref{}) {
		t.Fatalf("Runtime settlement leaked non-ref semantics: %v", typeOf)
	}
	for _, forbidden := range []string{"Outcome", "Disposition", "Binding", "Policy", "Trust"} {
		if _, ok := typeOf.FieldByName(forbidden); ok {
			t.Fatalf("Runtime settlement leaked %s", forbidden)
		}
	}
}

func TestWave1ConformanceAssociationBindsExactDomainResult(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	ref := contract.Ref{ID: "fact", Revision: 1, Digest: "sha256:x"}
	result, err := contract.NewDomainResultFact(contract.OwnerMemory, "result", "attempt", ref, ref, ref, 0, 1, nil, contract.Coverage{Status: contract.CoverageComplete}, "complete", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	association := contract.DomainResultAssociation{DomainResultRef: result.Ref}
	if err := association.Verify(result); err != nil {
		t.Fatalf("exact association rejected: %v", err)
	}
	wrong := association
	wrong.DomainResultRef.Digest = "sha256:wrong"
	if err := wrong.Verify(result); !errors.Is(err, contract.ErrSettlementMismatch) {
		t.Fatalf("wrong association accepted: %v", err)
	}
}

func TestWave1ConformanceOwnerDomainsAreDistinct(t *testing.T) {
	t.Parallel()
	if contract.OwnerMemory == contract.OwnerKnowledge || contract.OwnerMemory == "" || contract.OwnerKnowledge == "" {
		t.Fatalf("owner domains are not isolated: memory=%q knowledge=%q", contract.OwnerMemory, contract.OwnerKnowledge)
	}
}

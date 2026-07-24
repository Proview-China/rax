package service

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type claimCommandsStubV2 struct{}

func (claimCommandsStubV2) ClaimAssignmentV2(context.Context, reviewport.ClaimHumanAssignmentMutationV2, reviewport.HumanOrganizationCurrentRequestV2) (reviewport.ClaimHumanAssignmentResultV2, error) {
	return reviewport.ClaimHumanAssignmentResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "stub")
}

type multiCommandsStubV2 struct{}

func (multiCommandsStubV2) OpenPanelV2(context.Context, reviewport.CreateHumanPanelMutationV2) (reviewport.CreateHumanPanelResultV2, error) {
	return reviewport.CreateHumanPanelResultV2{}, nil
}
func (multiCommandsStubV2) SubmitAttestationV2(context.Context, reviewport.RecordHumanAttestationMutationV2) (reviewport.RecordHumanAttestationResultV2, error) {
	return reviewport.RecordHumanAttestationResultV2{}, nil
}

func TestHumanMultiSignProductionV2RequiresClaimCapability(t *testing.T) {
	store := memory.NewStore()
	if _, err := NewHumanMultiSignProductionV2(multiCommandsStubV2{}, claimCommandsStubV2{}, store); err != nil {
		t.Fatal(err)
	}
	var nilClaims *claimCommandsStubV2
	if _, err := NewHumanMultiSignProductionV2(multiCommandsStubV2{}, nilClaims, store); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed nil Claim capability accepted: %v", err)
	}
	legacy, err := NewHumanMultiSignV2(multiCommandsStubV2{}, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = legacy.ClaimAssignmentV2(context.Background(), reviewport.ClaimHumanAssignmentMutationV2{}, reviewport.HumanOrganizationCurrentRequestV2{}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("legacy service exposed a production Claim path: %v", err)
	}
}

package assemblyadapter

import (
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
)

func TestReviewGateAssemblyContributionV1ReusesLiveSlotAndPhase(t *testing.T) {
	port, slot, phase, err := ReviewGateAssemblyContributionV1()
	if err != nil {
		t.Fatal(err)
	}
	if err := port.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := slot.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := phase.Validate(); err != nil {
		t.Fatal(err)
	}
	if slot.SlotRef != "review.gate" || slot.Kind != assemblycontract.SlotContributionOwnerV1 || port.EffectKind != "" || port.CancelSupported || phase.Capability != assemblycontract.PhaseGateV1 || phase.Async || len(phase.WriteSet) != 0 {
		t.Fatalf("Review Gate contribution escaped its read-only Gate boundary: port=%+v slot=%+v phase=%+v", port, slot, phase)
	}
	hook, found := reviewGateHookFaceV1()
	if !found || hook.PhaseID != contract.ReviewGatePhaseIDV1 || hook.Kind != assemblycontract.PhaseGateV1 || phase.HookFaceRef != hook.HookFaceID {
		t.Fatalf("contribution did not reuse live action.review HookFace: hook=%+v phase=%+v", hook, phase)
	}
	foundSlot := false
	for _, live := range assemblycontract.SlotCatalogV1() {
		if live.SlotID == slot.SlotRef {
			foundSlot = true
			if live.OwnerCapability != slot.CapabilityRef {
				t.Fatalf("slot owner capability drifted: live=%q contribution=%q", live.OwnerCapability, slot.CapabilityRef)
			}
		}
	}
	if !foundSlot {
		t.Fatal("live review.gate slot is absent")
	}
}

func TestReviewGateAssemblyContributionV1IsDeterministic(t *testing.T) {
	firstPort, firstSlot, firstPhase, err := ReviewGateAssemblyContributionV1()
	if err != nil {
		t.Fatal(err)
	}
	secondPort, secondSlot, secondPhase, err := ReviewGateAssemblyContributionV1()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstPort, secondPort) || firstSlot.Digest != secondSlot.Digest || firstPhase.Digest != secondPhase.Digest {
		t.Fatal("Review Gate assembly declaration is not deterministic")
	}
}

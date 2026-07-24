package applicationadapter

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

func inspectionDigestV4(t *testing.T, value string) string {
	t.Helper()
	digest, err := contract.Digest("governed-inspection-test", value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestGovernedInspectionV4ProviderStateMappingFailsClosed(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		kind        contract.EffectKind
		provider    string
		state       string
		disposition contract.Disposition
	}{
		{name: "container allocation", kind: contract.EffectAllocate, provider: "containerd_oci", state: "container_prepared", disposition: contract.DispositionConfirmedApplied},
		{name: "wasm allocation", kind: contract.EffectAllocate, provider: "wasmtime_component", state: "allocated", disposition: contract.DispositionConfirmedApplied},
		{name: "container running", kind: contract.EffectActivate, provider: "containerd_oci", state: "task:running:pid:42", disposition: contract.DispositionConfirmedApplied},
		{name: "container stopped after start", kind: contract.EffectOpen, provider: "containerd_oci", state: "task:stopped:pid:42", disposition: contract.DispositionConfirmedApplied},
		{name: "wasm executing", kind: contract.EffectActivate, provider: "wasmtime_component", state: "executing", disposition: contract.DispositionConfirmedApplied},
		{name: "wasm exited", kind: contract.EffectOpen, provider: "wasmtime_component", state: "exited:7", disposition: contract.DispositionConfirmedApplied},
		{name: "not found is not not-applied", kind: contract.EffectAllocate, provider: "containerd_oci", state: "not_found", disposition: contract.DispositionUnknown},
		{name: "prepared activation", kind: contract.EffectActivate, provider: "wasmtime_component", state: "prepared", disposition: contract.DispositionUnknown},
		{name: "created task", kind: contract.EffectOpen, provider: "containerd_oci", state: "task:created:pid:42", disposition: contract.DispositionUnknown},
		{name: "unknown provider string", kind: contract.EffectOpen, provider: "wasmtime_component", state: "provider-says-ok", disposition: contract.DispositionUnknown},
		{name: "cancel quiesced", kind: contract.EffectCancel, provider: "host_workspace", state: "fenced", disposition: contract.DispositionConfirmedApplied},
		{name: "close stopped task", kind: contract.EffectClose, provider: "containerd_oci", state: "task:stopped:pid:42", disposition: contract.DispositionConfirmedApplied},
		{name: "fence pending remains unknown", kind: contract.EffectFence, provider: "qemu_microvm", state: "fence_pending", disposition: contract.DispositionUnknown},
		{name: "release absent", kind: contract.EffectRelease, provider: "host_workspace", state: "not_found", disposition: contract.DispositionConfirmedApplied},
		{name: "cleanup all absent", kind: contract.EffectCleanup, provider: "containerd_oci", state: "cleanup_absent", disposition: contract.DispositionConfirmedApplied},
		{name: "cleanup residual", kind: contract.EffectCleanup, provider: "remote_sandbox", state: "residual_present", disposition: contract.DispositionUnknown},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := dispositionForProviderState(testCase.kind, testCase.provider, testCase.state); got != testCase.disposition {
				t.Fatalf("disposition=%s want=%s", got, testCase.disposition)
			}
		})
	}
}

func TestCleanupReportRequiresExplicitCleanupAbsentObservation(t *testing.T) {
	evidence := []contract.Ref{{ID: "evidence-1", Revision: 1, Digest: inspectionDigestV4(t, "evidence-1")}}
	report := cleanupReportForProviderState(contract.EffectCleanup, "cleanup_absent", evidence)
	if report == nil || report.ValidateShape() != nil || !report.Complete() {
		t.Fatalf("explicit seven-dimensional cleanup proof was not produced: %#v", report)
	}
	evidence[0].ID = "mutated"
	if report.EvidenceRefs[0].ID != "evidence-1" {
		t.Fatal("cleanup report aliases caller evidence")
	}
	for _, state := range []string{"not_found", "released", "residual_present", ""} {
		if cleanupReportForProviderState(contract.EffectCleanup, state, evidence) != nil {
			t.Fatalf("state %q incorrectly produced a complete cleanup report", state)
		}
	}
	if cleanupReportForProviderState(contract.EffectRelease, "cleanup_absent", evidence) != nil {
		t.Fatal("release observation incorrectly produced a cleanup report")
	}
}

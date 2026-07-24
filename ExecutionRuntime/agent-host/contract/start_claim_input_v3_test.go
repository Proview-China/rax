package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

func TestHostStartInputV3ClaimAndSidecarExactClosure(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	input := startInputContractFixtureV3(t, now)
	claim, err := input.ClaimV1()
	if err != nil {
		t.Fatal(err)
	}
	if claim.HostContractVersion != contract.HostLifecycleContractVersionV3 || claim.ConfigDigest != input.ContentDigest || claim.DefinitionSourceRef != input.DefinitionSourceRef {
		t.Fatalf("claim=%+v", claim)
	}
	binding, err := contract.NewHostStartClaimInputBindingV3(claim, input)
	if err != nil {
		t.Fatal(err)
	}
	if err = binding.ValidateV3(); err != nil {
		t.Fatal(err)
	}
	changed := input
	changed.DefinitionSourceRef = exactContractV3(t, "praxis.agent-definition/definition", "other")
	changed.ContentDigest = ""
	changed, _ = contract.SealHostStartClaimInputV3(changed)
	if _, err = contract.NewHostStartClaimInputBindingV3(claim, changed); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("splice=%v", err)
	}
	wrong := binding
	wrong.BindingDigest = digestContractV3(t, "wrong")
	if _, err = contract.SealHostStartClaimInputBindingV3(wrong); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("wrong binding digest=%v", err)
	}
}

func startInputContractFixtureV3(t *testing.T, now time.Time) contract.HostStartClaimInputV3 {
	t.Helper()
	deploymentDigest := digestContractV3(t, "deployment")
	input, err := contract.SealHostStartClaimInputV3(contract.HostStartClaimInputV3{HostID: "host-1", StartID: "start-1", DeploymentCurrentRef: contract.HostDeploymentCurrentRefV1{HostID: "host-1", DeploymentID: "deployment-1", Revision: 1, BootstrapDigest: digestContractV3(t, "bootstrap"), ExpiresUnixNano: now.Add(2 * time.Hour).UnixNano(), Digest: deploymentDigest}, HostConfigDigest: digestContractV3(t, "config"), DefinitionSourceRef: exactContractV3(t, "praxis.agent-definition/definition", "definition-1"), RequestedOperation: contract.HostStartOperationStartV1, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return input
}

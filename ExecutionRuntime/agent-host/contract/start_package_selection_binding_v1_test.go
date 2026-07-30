package contract_test

import (
	"testing"
	"time"

	buildercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	assemblycontract "github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestHostStartPackageSelectionBindingV1ExactClosureAndExpiry(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	claim, input, deployment, selection, binding := packageSelectionBindingFixtureV1(t, now, "a")
	if err := binding.ValidateAgainstClaimInputV1(claim, input); err != nil {
		t.Fatal(err)
	}
	if binding.Ref.Revision != 1 ||
		binding.Ref.Digest != binding.BindingDigest ||
		binding.ExpiresUnixNano != selection.ExpiresUnixNano ||
		binding.PackageSelectionRef != deployment.Ref.PackageSelectionRef ||
		binding.VerifiedPackageClosureDigest != contract.DigestV1(selection.ClosureDigest) {
		t.Fatalf("binding=%+v", binding)
	}
	if err := binding.ValidateCurrentV1(binding.Ref, now); err != nil {
		t.Fatal(err)
	}
	if err := binding.ValidateCurrentV1(binding.Ref, time.Unix(0, binding.ExpiresUnixNano)); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("now==expiry=%v", err)
	}
}

func TestHostStartPackageSelectionBindingV1RejectsEverySpliceAxis(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	claim, input, deployment, selection, binding := packageSelectionBindingFixtureV1(t, now, "a")
	tests := []struct {
		name   string
		mutate func() contract.HostStartPackageSelectionBindingV1
	}{
		{"claim input", func() contract.HostStartPackageSelectionBindingV1 {
			value := binding
			value.ClaimInputBindingDigest = contract.DigestV1(core.DigestBytes([]byte("other-input")))
			value.Ref.Digest, value.BindingDigest = "", ""
			value, _ = contract.SealHostStartPackageSelectionBindingV1(value)
			return value
		}},
		{"deployment selection", func() contract.HostStartPackageSelectionBindingV1 {
			value := binding
			value.PackageSelectionRef.Revision++
			value.Ref.Digest, value.BindingDigest = "", ""
			value, _ = contract.SealHostStartPackageSelectionBindingV1(value)
			return value
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := test.mutate()
			if err := value.ValidateAgainstClaimInputV1(claim, input); err == nil {
				t.Fatal("spliced binding validated")
			}
		})
	}
	changedSelection := selection
	changedSelection.Ref.SelectionID = "selection/other"
	changedSelection.Ref.Digest, changedSelection.ProjectionDigest = "", ""
	changedSelection, _ = buildercontract.SealAgentPackageSelectionCurrentV1(changedSelection)
	if _, err := contract.NewHostStartPackageSelectionBindingV1(claim, input, deployment, changedSelection, now.UnixNano()); err == nil {
		t.Fatal("deployment and selection splice sealed")
	}
}

func packageSelectionBindingFixtureV1(
	t *testing.T,
	now time.Time,
	label string,
) (
	contract.HostStartClaimV1,
	contract.HostStartClaimInputBindingV3,
	contract.HostDeploymentCurrentV2,
	buildercontract.AgentPackageSelectionCurrentV1,
	contract.HostStartPackageSelectionBindingV1,
) {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(label + "/" + value)) }
	selectionExpires := now.Add(30 * time.Minute).UnixNano()
	selection, err := buildercontract.SealAgentPackageSelectionCurrentV1(buildercontract.AgentPackageSelectionCurrentV1{
		Ref: buildercontract.AgentPackageSelectionCurrentRefV1{
			SelectionID: "selection/" + label, Revision: 1, ExpiresUnixNano: selectionExpires,
		},
		PackageRef: buildercontract.AgentPackageRefV1{
			PackageID: "agent-package-" + label, Revision: 1, Digest: digest("package"),
			ContractVersion: buildercontract.ContractVersionV1, SchemaVersion: buildercontract.SchemaVersionV1,
		},
		PublicationRef: assemblycontract.AssemblyPublicationRefV2{
			PublicationID: "publication-" + label, Revision: 1, Digest: digest("publication"),
		},
		ClosureDigest: digest("closure"), CheckedUnixNano: now.Add(-time.Minute).UnixNano(),
		ExpiresUnixNano: selectionExpires,
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := contract.SealHostDeploymentCurrentV2(contract.HostDeploymentCurrentV2{
		Ref: contract.HostDeploymentCurrentRefV2{
			HostID: "host-" + label, DeploymentID: "deployment-" + label, Revision: 1,
			BootstrapDigest: contract.DigestV1(digest("bootstrap")), PackageSelectionRef: selection.Ref,
			ExpiresUnixNano: selectionExpires,
		},
		ResourceHandles: []runtimeports.ResourceHandleRefV1{},
		ServiceBindings: []contract.HostServiceBindingRefV1{},
		CheckedUnixNano: now.Add(-time.Minute).UnixNano(),
		ExpiresUnixNano: selectionExpires,
	})
	if err != nil {
		t.Fatal(err)
	}
	inputValue, err := contract.SealHostStartClaimInputV3(contract.HostStartClaimInputV3{
		HostID: "host-" + label, StartID: "start-" + label,
		DeploymentCurrentRef: contract.HostDeploymentCurrentRefV1{
			HostID: deployment.Ref.HostID, DeploymentID: deployment.Ref.DeploymentID, Revision: deployment.Ref.Revision,
			BootstrapDigest: deployment.Ref.BootstrapDigest,
			ExpiresUnixNano: now.Add(time.Hour).UnixNano(), Digest: contract.DigestV1(digest("deployment-v1")),
		},
		HostConfigDigest: contract.DigestV1(digest("config")),
		DefinitionSourceRef: contract.ExactRefV1{
			Kind: "praxis.agent-definition/source-current", ID: "source-" + label, Revision: 1, Digest: contract.DigestV1(digest("source")),
		},
		RequestedOperation: contract.HostStartOperationStartV1,
		CreatedUnixNano:    now.Add(-2 * time.Minute).UnixNano(),
		ExpiresUnixNano:    now.Add(50 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := inputValue.ClaimV1()
	if err != nil {
		t.Fatal(err)
	}
	input, err := contract.NewHostStartClaimInputBindingV3(claim, inputValue)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := contract.NewHostStartPackageSelectionBindingV1(claim, input, deployment, selection, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	return claim, input, deployment, selection, binding
}

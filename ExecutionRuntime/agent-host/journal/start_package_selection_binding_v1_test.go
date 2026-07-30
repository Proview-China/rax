package journal

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	buildercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	assemblycontract "github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestMemoryHostStartPackageSelectionAdmissionUnknownInspectsExactOnceV1(t *testing.T) {
	now := time.Unix(2_400_100_000, 0)
	claim, input, binding := journalPackageSelectionFixtureV1(t, now, "lost")
	inner := NewMemoryHostStartPackageSelectionStoreV1()
	fault := &lostReplyStartPackageSelectionPortV1{inner: inner}
	admission, err := NewHostStartPackageSelectionAdmissionV1(fault, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	actual, err := admission.ClaimV1(context.Background(), claim, input.Input, binding)
	if err != nil || actual.Ref != binding.Ref {
		t.Fatalf("recovered=%+v err=%v", actual, err)
	}
	if fault.writes.Load() != 1 || fault.inspects.Load() != 1 {
		t.Fatalf("writes=%d exact-inspects=%d", fault.writes.Load(), fault.inspects.Load())
	}
}

func TestMemoryHostStartPackageSelectionSixtyFourSameClaimAndConflictV1(t *testing.T) {
	now := time.Unix(2_400_100_000, 0)
	claim, input, binding := journalPackageSelectionFixtureV1(t, now, "concurrent")
	store := NewMemoryHostStartPackageSelectionStoreV1()
	var successes atomic.Int64
	var wait sync.WaitGroup
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			actual, err := store.ClaimOrInspectHostStartPackageSelectionV1(context.Background(), claim, input.Input, binding)
			if err != nil {
				t.Errorf("Claim=%v", err)
				return
			}
			if actual.Ref != binding.Ref {
				t.Errorf("Ref=%+v", actual.Ref)
				return
			}
			successes.Add(1)
		}()
	}
	wait.Wait()
	if successes.Load() != 64 {
		t.Fatalf("successes=%d", successes.Load())
	}
	changed := binding
	changed.VerifiedPackageClosureDigest = contract.DigestV1(core.DigestBytes([]byte("other-closure")))
	changed.Ref.Digest, changed.BindingDigest = "", ""
	changed, err := contract.SealHostStartPackageSelectionBindingV1(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ClaimOrInspectHostStartPackageSelectionV1(context.Background(), claim, input.Input, changed); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("association replacement=%v", err)
	}
}

func TestMemoryHostStartPackageSelectionDoesNotUpgradeOldV3ClaimV1(t *testing.T) {
	now := time.Unix(2_400_100_000, 0)
	claim, input, _ := journalPackageSelectionFixtureV1(t, now, "old-v3")
	store := NewMemoryHostStartPackageSelectionStoreV1()
	if _, err := store.claims.ClaimOrInspectHostStartV3(context.Background(), claim, input.Input); err != nil {
		t.Fatal(err)
	}
	ref, _ := claim.CurrentRefV1()
	if _, err := store.InspectHostStartPackageSelectionBindingForClaimV1(context.Background(), ref); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("old V3 Claim gained association=%v", err)
	}
}

type lostReplyStartPackageSelectionPortV1 struct {
	inner    hostports.HostStartClaimPackageSelectionPortV1
	writes   atomic.Int64
	inspects atomic.Int64
}

func (port *lostReplyStartPackageSelectionPortV1) ClaimOrInspectHostStartPackageSelectionV1(
	ctx context.Context,
	claim contract.HostStartClaimV1,
	input contract.HostStartClaimInputV3,
	binding contract.HostStartPackageSelectionBindingV1,
) (contract.HostStartPackageSelectionBindingV1, error) {
	port.writes.Add(1)
	actual, err := port.inner.ClaimOrInspectHostStartPackageSelectionV1(ctx, claim, input, binding)
	if err != nil {
		return actual, err
	}
	return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorUnknownOutcome, "lost_reply", "committed reply was lost")
}

func (port *lostReplyStartPackageSelectionPortV1) InspectHostStartPackageSelectionBindingV1(
	ctx context.Context,
	expected contract.HostStartPackageSelectionBindingRefV1,
) (contract.HostStartPackageSelectionBindingV1, error) {
	port.inspects.Add(1)
	return port.inner.InspectHostStartPackageSelectionBindingV1(ctx, expected)
}

func (port *lostReplyStartPackageSelectionPortV1) InspectHostStartPackageSelectionBindingForClaimV1(
	ctx context.Context,
	expected contract.HostStartClaimRefV1,
) (contract.HostStartPackageSelectionBindingV1, error) {
	return port.inner.InspectHostStartPackageSelectionBindingForClaimV1(ctx, expected)
}

func journalPackageSelectionFixtureV1(
	t *testing.T,
	now time.Time,
	label string,
) (contract.HostStartClaimV1, contract.HostStartClaimInputBindingV3, contract.HostStartPackageSelectionBindingV1) {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(label + "/" + value)) }
	expires := now.Add(time.Hour).UnixNano()
	selection, err := buildercontract.SealAgentPackageSelectionCurrentV1(buildercontract.AgentPackageSelectionCurrentV1{
		Ref: buildercontract.AgentPackageSelectionCurrentRefV1{
			SelectionID: "selection/" + label, Revision: 1, ExpiresUnixNano: expires,
		},
		PackageRef: buildercontract.AgentPackageRefV1{
			PackageID: "agent-package-" + label, Revision: 1, Digest: digest("package"),
			ContractVersion: buildercontract.ContractVersionV1, SchemaVersion: buildercontract.SchemaVersionV1,
		},
		PublicationRef: assemblycontract.AssemblyPublicationRefV2{
			PublicationID: "publication-" + label, Revision: 1, Digest: digest("publication"),
		},
		ClosureDigest: digest("closure"), CheckedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := contract.SealHostDeploymentCurrentV2(contract.HostDeploymentCurrentV2{
		Ref: contract.HostDeploymentCurrentRefV2{
			HostID: "host-" + label, DeploymentID: "deployment-" + label, Revision: 1,
			BootstrapDigest: contract.DigestV1(digest("bootstrap")), PackageSelectionRef: selection.Ref, ExpiresUnixNano: expires,
		},
		ResourceHandles: []runtimeports.ResourceHandleRefV1{},
		ServiceBindings: []contract.HostServiceBindingRefV1{},
		CheckedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	inputValue, err := contract.SealHostStartClaimInputV3(contract.HostStartClaimInputV3{
		HostID: deployment.Ref.HostID, StartID: "start-" + label,
		DeploymentCurrentRef: contract.HostDeploymentCurrentRefV1{
			HostID: deployment.Ref.HostID, DeploymentID: deployment.Ref.DeploymentID, Revision: deployment.Ref.Revision,
			BootstrapDigest: deployment.Ref.BootstrapDigest, ExpiresUnixNano: expires,
			Digest: contract.DigestV1(digest("deployment-v1")),
		},
		HostConfigDigest: contract.DigestV1(digest("config")),
		DefinitionSourceRef: contract.ExactRefV1{
			Kind: "praxis.agent-definition/source-current", ID: "source-" + label, Revision: 1, Digest: contract.DigestV1(digest("source")),
		},
		RequestedOperation: contract.HostStartOperationStartV1,
		CreatedUnixNano:    now.Add(-time.Minute).UnixNano(),
		ExpiresUnixNano:    expires,
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
	return claim, input, binding
}

var _ hostports.HostStartClaimPackageSelectionPortV1 = (*lostReplyStartPackageSelectionPortV1)(nil)

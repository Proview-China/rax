package kernel

import (
	"strings"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

func TestWorkspaceReadRuntimeLeaseBindingRejectsEveryAxis(t *testing.T) {
	expires := time.Unix(1_900_000_000, 0).Add(time.Hour).UnixNano()
	scope := runtimecore.DigestBytes([]byte("workspace-read-scope"))
	rawScope := strings.TrimPrefix(string(scope), "sha256:")
	current := runtimeports.CurrentOperationDispatchEnforcementV4{
		Sandbox: runtimeports.OperationDispatchSandboxCurrentProjectionV4{
			Operation: runtimeports.OperationSubjectV3{
				ExecutionScope: runtimecore.ExecutionScope{
					Identity: runtimecore.AgentIdentityRef{TenantID: "tenant", ID: "identity", Epoch: 1},
				},
			},
			RuntimeLease: runtimeports.OperationDispatchRuntimeLeaseBindingV4{
				Ref:              runtimeports.OperationDispatchSandboxFactRefV4{ID: "runtime-lease", Revision: 1, Digest: runtimecore.DigestBytes([]byte("runtime-lease")), ExpiresUnixNano: expires},
				Lease:            runtimecore.SandboxLeaseRef{ID: "lease", Epoch: 2},
				Instance:         runtimecore.InstanceRef{ID: "instance", Epoch: 3},
				FenceEpoch:       4,
				ScopeDigest:      scope,
				ObservedRevision: 5,
			},
		},
	}
	binding := contract.RuntimeLeaseBinding{
		TenantID: "tenant", InstanceID: "instance", InstanceEpoch: 3,
		LeaseID: "lease", LeaseEpoch: 2, FenceEpoch: 4,
		ScopeDigest: rawScope, ObservedRevision: 5, ExpiresUnixNano: expires,
	}
	if err := validateWorkspaceReadRuntimeLeaseV1(binding, current); err != nil {
		t.Fatalf("valid exact lease: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*contract.RuntimeLeaseBinding)
	}{
		{"tenant", func(v *contract.RuntimeLeaseBinding) { v.TenantID = "other" }},
		{"instance-id", func(v *contract.RuntimeLeaseBinding) { v.InstanceID = "other" }},
		{"instance-epoch", func(v *contract.RuntimeLeaseBinding) { v.InstanceEpoch++ }},
		{"lease-id", func(v *contract.RuntimeLeaseBinding) { v.LeaseID = "other" }},
		{"lease-epoch", func(v *contract.RuntimeLeaseBinding) { v.LeaseEpoch++ }},
		{"fence", func(v *contract.RuntimeLeaseBinding) { v.FenceEpoch++ }},
		{"scope", func(v *contract.RuntimeLeaseBinding) {
			v.ScopeDigest = strings.TrimPrefix(string(runtimecore.DigestBytes([]byte("other"))), "sha256:")
		}},
		{"scope-prefixed-splice", func(v *contract.RuntimeLeaseBinding) { v.ScopeDigest = string(scope) }},
		{"observed-revision", func(v *contract.RuntimeLeaseBinding) { v.ObservedRevision++ }},
		{"expiry", func(v *contract.RuntimeLeaseBinding) { v.ExpiresUnixNano++ }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			drift := binding
			testCase.mutate(&drift)
			if err := validateWorkspaceReadRuntimeLeaseV1(drift, current); err == nil {
				t.Fatal("drifted WorkspaceView lease reached the physical boundary")
			}
		})
	}
}

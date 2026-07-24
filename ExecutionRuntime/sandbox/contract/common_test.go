package contract_test

import (
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
)

func TestDecodeStrict(t *testing.T) {
	t.Parallel()
	type document struct {
		Name string `json:"name"`
	}
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "valid", input: `{"name":"sandbox"}`},
		{name: "unknown", input: `{"name":"sandbox","extra":true}`, wantErr: "unknown field"},
		{name: "duplicate", input: `{"name":"one","name":"two"}`, wantErr: "duplicate json key"},
		{name: "trailing", input: `{"name":"sandbox"} {}`, wantErr: "trailing json document"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := contract.DecodeStrict[document]([]byte(test.input))
			if test.wantErr == "" {
				if err != nil || got.Name != "sandbox" {
					t.Fatalf("DecodeStrict() = %#v, %v", got, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("DecodeStrict() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestRequirementRejectsUnboundedAndPlainValueShape(t *testing.T) {
	t.Parallel()
	requirement := testkit.Requirement()
	requirement.Resources.MemoryBytes = 0
	if err := requirement.ValidateCurrent(testkit.FixedNow); err == nil {
		t.Fatal("Requirement.ValidateCurrent() accepted zero memory bound")
	}

	requirement = testkit.Requirement()
	requirement.Secrets = []contract.SecretRequirement{{
		SecretRef: testkit.Ref("secret"), Class: "api", InjectionMode: "ref", MaxTTLSeconds: 0,
	}}
	if err := requirement.ValidateCurrent(testkit.FixedNow); err == nil {
		t.Fatal("Requirement.ValidateCurrent() accepted secret without finite ttl")
	}
}

func TestRuntimeLeaseBindingUsesValueSemantics(t *testing.T) {
	t.Parallel()
	one := testkit.Lease()
	two := one
	if !contract.SameRuntimeLeaseBinding(one, two) {
		t.Fatal("equal copied lease bindings must compare equal")
	}
	tests := []struct {
		name   string
		mutate func(*contract.RuntimeLeaseBinding)
	}{
		{name: "tenant", mutate: func(v *contract.RuntimeLeaseBinding) { v.TenantID += "-drift" }},
		{name: "instance", mutate: func(v *contract.RuntimeLeaseBinding) { v.InstanceID += "-drift" }},
		{name: "instance epoch", mutate: func(v *contract.RuntimeLeaseBinding) { v.InstanceEpoch++ }},
		{name: "lease", mutate: func(v *contract.RuntimeLeaseBinding) { v.LeaseID += "-drift" }},
		{name: "lease epoch", mutate: func(v *contract.RuntimeLeaseBinding) { v.LeaseEpoch++ }},
		{name: "fence epoch", mutate: func(v *contract.RuntimeLeaseBinding) { v.FenceEpoch++ }},
		{name: "scope", mutate: func(v *contract.RuntimeLeaseBinding) { v.ScopeDigest = testkit.Ref("scope-drift").Digest }},
		{name: "observed revision", mutate: func(v *contract.RuntimeLeaseBinding) { v.ObservedRevision++ }},
		{name: "ttl", mutate: func(v *contract.RuntimeLeaseBinding) { v.ExpiresUnixNano++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			drifted := one
			test.mutate(&drifted)
			if contract.SameRuntimeLeaseBinding(one, drifted) {
				t.Fatalf("%s drift compared equal", test.name)
			}
		})
	}
}

func TestCleanupRequiresSevenDimensions(t *testing.T) {
	t.Parallel()
	report := testkit.CompleteCleanup()
	if err := report.ValidateShape(); err != nil || !report.Complete() {
		t.Fatalf("complete cleanup = %v, %v", report.Complete(), err)
	}
	report.RemoteContinuation = contract.CleanupIndeterminate
	if report.Complete() {
		t.Fatal("indeterminate remote continuation cannot be complete")
	}
}

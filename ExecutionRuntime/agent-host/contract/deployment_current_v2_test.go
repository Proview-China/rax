package contract_test

import (
	"reflect"
	"testing"
	"time"

	buildercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestHostDeploymentCurrentV2SelectionAxesAreDigestBound(t *testing.T) {
	now := time.Unix(2_300_000_000, 0)
	current := deploymentCurrentFixtureV2(t, now)
	mutations := []struct {
		name   string
		mutate func(*contract.HostDeploymentCurrentV2)
	}{
		{"selection id", func(value *contract.HostDeploymentCurrentV2) { value.Ref.PackageSelectionRef.SelectionID += "-other" }},
		{"selection revision", func(value *contract.HostDeploymentCurrentV2) { value.Ref.PackageSelectionRef.Revision++ }},
		{"selection digest", func(value *contract.HostDeploymentCurrentV2) {
			value.Ref.PackageSelectionRef.Digest = core.DigestBytes([]byte("other-selection"))
		}},
		{"selection expiry", func(value *contract.HostDeploymentCurrentV2) { value.Ref.PackageSelectionRef.ExpiresUnixNano++ }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			spliced := contract.CloneHostDeploymentCurrentV2(current)
			mutation.mutate(&spliced)
			if err := spliced.ValidateHistoricalV2(); !contract.HasCode(err, contract.ErrorConflict) {
				t.Fatalf("selection splice accepted: %v", err)
			}
		})
	}
}

func TestHostDeploymentCurrentV2ExpiryEqualityFailsClosed(t *testing.T) {
	now := time.Unix(2_300_000_000, 0)
	current := deploymentCurrentFixtureV2(t, now)
	if err := current.ValidateCurrentV2(current.Ref, time.Unix(0, current.ExpiresUnixNano)); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("now==expiry accepted: %v", err)
	}
}

func TestHostDeploymentCurrentV2CanonicalEmptyAndNoMirroredPackageLineage(t *testing.T) {
	current := deploymentCurrentFixtureV2(t, time.Unix(2_300_000_000, 0))
	if current.ResourceHandles == nil || current.ServiceBindings == nil {
		t.Fatal("empty collections were not sealed as canonical non-nil arrays")
	}
	for _, typ := range []reflect.Type{
		reflect.TypeOf(contract.HostDeploymentCurrentRefV2{}),
		reflect.TypeOf(contract.HostDeploymentCurrentV2{}),
	} {
		for _, forbidden := range []string{"PackageRef", "PublicationRef", "ClosureDigest"} {
			if _, exists := typ.FieldByName(forbidden); exists {
				t.Fatalf("%s mirrors forbidden Builder field %s", typ.Name(), forbidden)
			}
		}
	}
}

func TestHostDeploymentCurrentV2ResourceAndServiceAxesAreDigestBound(t *testing.T) {
	now := time.Unix(2_300_000_000, 0)
	expires := now.Add(time.Hour).UnixNano()
	base := deploymentCurrentFixtureV2(t, now)
	base.ResourceHandles = []runtimeports.ResourceHandleRefV1{{
		Owner: core.OwnerRef{Domain: "praxis.host-test", ID: "resource-owner"},
		ID:    "resource-a", Revision: 1, Digest: core.DigestBytes([]byte("resource-a")),
		Kind: "praxis/sqlite", ScopeDigest: core.DigestBytes([]byte("scope-a")), ExpiresUnixNano: expires,
	}}
	base.ServiceBindings = []contract.HostServiceBindingRefV1{{
		Role: contract.HostServiceRuntimeV1, ConfiguredID: "runtime-a",
		BindingRef: contract.ExactRefV1{
			Kind: "praxis.agent-host/service-binding", ID: "runtime-a", Revision: 1,
			Digest: contract.DigestV1(core.DigestBytes([]byte("runtime-a"))),
		},
		Capability: "praxis.host/runtime_service", ExpiresUnixNano: expires,
	}}
	base.Ref.Digest, base.ProjectionDigest = "", ""
	base, err := contract.SealHostDeploymentCurrentV2(base)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name   string
		mutate func(*contract.HostDeploymentCurrentV2)
	}{
		{"resource owner", func(value *contract.HostDeploymentCurrentV2) { value.ResourceHandles[0].Owner.ID += "-other" }},
		{"resource id", func(value *contract.HostDeploymentCurrentV2) { value.ResourceHandles[0].ID += "-other" }},
		{"resource revision", func(value *contract.HostDeploymentCurrentV2) { value.ResourceHandles[0].Revision++ }},
		{"resource digest", func(value *contract.HostDeploymentCurrentV2) {
			value.ResourceHandles[0].Digest = core.DigestBytes([]byte("other-resource"))
		}},
		{"resource kind", func(value *contract.HostDeploymentCurrentV2) { value.ResourceHandles[0].Kind = "praxis/other" }},
		{"resource scope", func(value *contract.HostDeploymentCurrentV2) {
			value.ResourceHandles[0].ScopeDigest = core.DigestBytes([]byte("other-scope"))
		}},
		{"resource expiry", func(value *contract.HostDeploymentCurrentV2) { value.ResourceHandles[0].ExpiresUnixNano-- }},
		{"service role", func(value *contract.HostDeploymentCurrentV2) {
			value.ServiceBindings[0].Role = contract.HostServiceHarnessV1
		}},
		{"service id", func(value *contract.HostDeploymentCurrentV2) { value.ServiceBindings[0].ConfiguredID += "-other" }},
		{"service ref", func(value *contract.HostDeploymentCurrentV2) {
			value.ServiceBindings[0].BindingRef.Digest = contract.DigestV1(core.DigestBytes([]byte("other-service")))
		}},
		{"service capability", func(value *contract.HostDeploymentCurrentV2) { value.ServiceBindings[0].Capability += "-other" }},
		{"service expiry", func(value *contract.HostDeploymentCurrentV2) { value.ServiceBindings[0].ExpiresUnixNano-- }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			spliced := contract.CloneHostDeploymentCurrentV2(base)
			mutation.mutate(&spliced)
			if err := spliced.ValidateHistoricalV2(); err == nil {
				t.Fatal("resource/service splice accepted")
			}
		})
	}
}

func deploymentCurrentFixtureV2(t *testing.T, now time.Time) contract.HostDeploymentCurrentV2 {
	t.Helper()
	expires := now.Add(time.Hour).UnixNano()
	value, err := contract.SealHostDeploymentCurrentV2(contract.HostDeploymentCurrentV2{
		Ref: contract.HostDeploymentCurrentRefV2{
			HostID:          "host-contract-v2",
			DeploymentID:    "deployment-contract-v2",
			Revision:        1,
			BootstrapDigest: contract.DigestV1(core.DigestBytes([]byte("bootstrap"))),
			PackageSelectionRef: buildercontract.AgentPackageSelectionCurrentRefV1{
				SelectionID:     "selection/contract-v2",
				Revision:        1,
				Digest:          core.DigestBytes([]byte("selection")),
				ExpiresUnixNano: expires,
			},
			ExpiresUnixNano: expires,
		},
		ResourceHandles: []runtimeports.ResourceHandleRefV1{},
		ServiceBindings: []contract.HostServiceBindingRefV1{},
		CheckedUnixNano: now.UnixNano(),
		ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

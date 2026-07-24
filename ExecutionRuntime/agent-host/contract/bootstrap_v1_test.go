package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestHostBootstrapCanonicalStrictAndCurrentV1(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	a := bootstrapFixtureV1(t, now)
	b := a
	b.ContentDigest = ""
	b.StatePlaneBindingIDs = []string{"state-b", "state-a"}
	b.EnabledControlAPISurfaces = []string{"stop", "run", "inspect"}
	b, err := contract.SealHostBootstrapConfigV1(b)
	if err != nil || a.ContentDigest != b.ContentDigest {
		t.Fatalf("canonical digest=%s/%s err=%v", a.ContentDigest, b.ContentDigest, err)
	}
	if err := a.ValidateCurrentV1(now); err != nil {
		t.Fatal(err)
	}
	bad := a
	bad.ProviderEndpointRegistryBindingID = "https://raw-provider"
	bad.ContentDigest = ""
	if _, err := contract.SealHostBootstrapConfigV1(bad); !contract.HasCode(err, contract.ErrorInvalidArgument) {
		t.Fatalf("raw endpoint=%v", err)
	}
	bad = a
	bad.ContentDigest = digestContractV3(t, "wrong")
	if _, err := contract.SealHostBootstrapConfigV1(bad); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("wrong digest=%v", err)
	}
	if err := a.ValidateCurrentV1(now.Add(3 * time.Hour)); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("expired=%v", err)
	}
}

func TestHostDeploymentCurrentBindsCompleteBootstrapV1(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	bootstrap := bootstrapFixtureV1(t, now)
	expires := now.Add(time.Hour).UnixNano()
	owner := core.OwnerRef{Domain: "praxis.deployment", ID: "host-owner"}
	handles := []runtimeports.ResourceHandleRefV1{}
	for _, id := range bootstrap.StatePlaneBindingIDs {
		handles = append(handles, runtimeports.ResourceHandleRefV1{Owner: owner, ID: id, Revision: 1, Digest: core.DigestBytes([]byte("resource-" + id)), Kind: "praxis/sqlite", ScopeDigest: core.DigestBytes([]byte("scope-" + id)), ExpiresUnixNano: expires})
	}
	services := []contract.HostServiceBindingRefV1{}
	for key := range bootstrapServiceKeysFixtureV1(bootstrap) {
		role, id := splitServiceKeyFixtureV1(key)
		services = append(services, contract.HostServiceBindingRefV1{Role: contract.HostServiceBindingRoleV1(role), ConfiguredID: id, BindingRef: exactContractV3(t, "praxis.agent-host/service-binding", role+"-"+id), Capability: "praxis.host/" + role, ExpiresUnixNano: expires})
	}
	p, err := contract.SealHostDeploymentCurrentV1(contract.HostDeploymentCurrentV1{Ref: contract.HostDeploymentCurrentRefV1{HostID: bootstrap.HostID, DeploymentID: "deployment-1", Revision: 1, BootstrapDigest: bootstrap.ContentDigest, ExpiresUnixNano: expires}, ResourceHandles: handles, ServiceBindings: services, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.ValidateForBootstrapV1(bootstrap, now); err != nil {
		t.Fatal(err)
	}
	drift := bootstrap
	drift.CatalogBindingID = "catalog-other"
	drift.ContentDigest = ""
	drift, _ = contract.SealHostBootstrapConfigV1(drift)
	if err = p.ValidateForBootstrapV1(drift, now); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("bootstrap splice=%v", err)
	}
	tampered := p
	tampered.ServiceBindings[0].ConfiguredID = "other"
	if _, err = contract.SealHostDeploymentCurrentV1(tampered); err == nil {
		t.Fatal("service drift resealed")
	}
}

func bootstrapFixtureV1(t *testing.T, now time.Time) contract.HostBootstrapConfigV1 {
	t.Helper()
	v, err := contract.SealHostBootstrapConfigV1(contract.HostBootstrapConfigV1{HostID: "host-1", StatePlaneBindingIDs: []string{"state-a", "state-b"}, DefinitionSourceBindingID: "definition-source", CatalogBindingID: "catalog", ResolutionFactsBindingID: "resolution", SecretBrokerBindingID: "secret-broker", CredentialRegistryBindingID: "credentials", ProviderEndpointRegistryBindingID: "providers", RuntimeServiceBindingIDs: []string{"runtime"}, ApplicationServiceBindingIDs: []string{"application"}, HarnessServiceBindingIDs: []string{"harness"}, ListenBindingID: "listen", DiagnosticsPolicyBindingID: "diagnostics", ShutdownPolicyBindingID: "shutdown", EnabledControlAPISurfaces: []string{"inspect", "run", "stop"}, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), NotAfterUnixNano: now.Add(2 * time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func digestContractV3(t *testing.T, value string) contract.DigestV1 {
	t.Helper()
	d, e := contract.DigestJSONV1(value)
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func exactContractV3(t *testing.T, kind, id string) contract.ExactRefV1 {
	t.Helper()
	return contract.ExactRefV1{Kind: kind, ID: id, Revision: 1, Digest: digestContractV3(t, id)}
}

func bootstrapServiceKeysFixtureV1(c contract.HostBootstrapConfigV1) map[string]struct{} {
	m := map[string]struct{}{}
	add := func(role string, ids ...string) {
		for _, id := range ids {
			m[role+"\x00"+id] = struct{}{}
		}
	}
	add(string(contract.HostServiceDefinitionSourceV1), c.DefinitionSourceBindingID)
	add(string(contract.HostServiceCatalogV1), c.CatalogBindingID)
	add(string(contract.HostServiceResolutionFactsV1), c.ResolutionFactsBindingID)
	add(string(contract.HostServiceSecretBrokerV1), c.SecretBrokerBindingID)
	add(string(contract.HostServiceCredentialRegistryV1), c.CredentialRegistryBindingID)
	add(string(contract.HostServiceProviderRegistryV1), c.ProviderEndpointRegistryBindingID)
	add(string(contract.HostServiceRuntimeV1), c.RuntimeServiceBindingIDs...)
	add(string(contract.HostServiceApplicationV1), c.ApplicationServiceBindingIDs...)
	add(string(contract.HostServiceHarnessV1), c.HarnessServiceBindingIDs...)
	add(string(contract.HostServiceListenV1), c.ListenBindingID)
	add(string(contract.HostServiceDiagnosticsV1), c.DiagnosticsPolicyBindingID)
	add(string(contract.HostServiceShutdownV1), c.ShutdownPolicyBindingID)
	return m
}
func splitServiceKeyFixtureV1(v string) (string, string) {
	for i := range v {
		if v[i] == 0 {
			return v[:i], v[i+1:]
		}
	}
	return "", v
}

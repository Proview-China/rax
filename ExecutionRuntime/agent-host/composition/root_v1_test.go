package composition_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	definitionconformance "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/conformance"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	definitionports "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/composition"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestDeclarativeRootValidateIsPureAndRunUsesHostV3V1(t *testing.T) {
	now := time.Unix(2_410_000_000, 0)
	catalog := definitionconformance.CatalogV1()
	source := definitionconformance.SourceV1(now)
	payload, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := rootBootstrapFixtureV1(t, now)
	deployment := rootDeploymentFixtureV1(t, bootstrap, now)
	definitions := &definitionPublisherV1{catalog: catalog, now: now}
	sources := &sourcePublisherV1{now: now}
	host := &hostV3Stub{}
	root, err := composition.NewDeclarativeRootV1(composition.DeclarativeRootConfigV1{
		DefinitionCatalog: catalog, Definitions: definitions, DefinitionSources: sources,
		Assembler: declarativeAssemblerStub{}, Deployments: deploymentReaderV1{value: deployment}, Host: host,
		Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	validated, err := root.ValidateDefinitionV1(context.Background(), composition.ValidateDefinitionRequestV1{Format: composition.DefinitionFormatJSONV1, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if definitions.calls.Load() != 0 || sources.calls.Load() != 0 || host.starts.Load() != 0 || validated.Source.DefinitionID != source.DefinitionID {
		t.Fatalf("validate caused writes: definitions=%d sources=%d starts=%d", definitions.calls.Load(), sources.calls.Load(), host.starts.Load())
	}
	result, err := root.RunDefinitionV1(context.Background(), composition.RunDefinitionRequestV1{
		Bootstrap: bootstrap, DeploymentCurrent: deployment.Ref, StartID: "start-1",
		DefinitionFormat: composition.DefinitionFormatJSONV1, DefinitionPayload: payload,
		RequestedNotAfterUnixNano: now.Add(30 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if definitions.calls.Load() != 1 || sources.calls.Load() != 1 || host.starts.Load() != 1 {
		t.Fatalf("run call counts: definitions=%d sources=%d starts=%d", definitions.calls.Load(), sources.calls.Load(), host.starts.Load())
	}
	if host.request.ContractVersion != contract.HostLifecycleContractVersionV3 || host.request.DeploymentCurrentRef != deployment.Ref || host.request.DefinitionSourceCurrent.Kind != contract.DefinitionSourceCurrentKindV1 || result.RequestDigest != host.request.RequestDigest {
		t.Fatalf("HostV3 request/result drift: request=%+v result=%+v", host.request, result)
	}
}

func TestDeclarativeRootRejectsDeploymentDriftBeforeHostStartV1(t *testing.T) {
	now := time.Unix(2_410_000_100, 0)
	catalog := definitionconformance.CatalogV1()
	source := definitionconformance.SourceV1(now)
	payload, _ := json.Marshal(source)
	bootstrap := rootBootstrapFixtureV1(t, now)
	deployment := rootDeploymentFixtureV1(t, bootstrap, now)
	host := &hostV3Stub{}
	reader := &driftingDeploymentReaderV1{value: deployment}
	root, err := composition.NewDeclarativeRootV1(composition.DeclarativeRootConfigV1{
		DefinitionCatalog: catalog, Definitions: &definitionPublisherV1{catalog: catalog, now: now},
		DefinitionSources: &sourcePublisherV1{now: now}, Assembler: declarativeAssemblerStub{},
		Deployments: reader, Host: host, Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = root.RunDefinitionV1(context.Background(), composition.RunDefinitionRequestV1{Bootstrap: bootstrap, DeploymentCurrent: deployment.Ref, StartID: "start-1", DefinitionFormat: composition.DefinitionFormatJSONV1, DefinitionPayload: payload, RequestedNotAfterUnixNano: now.Add(30 * time.Minute).UnixNano()})
	if err == nil || host.starts.Load() != 0 {
		t.Fatalf("deployment drift err=%v host starts=%d", err, host.starts.Load())
	}
}

type definitionPublisherV1 struct {
	catalog definitioncontract.ValidationCatalogV1
	now     time.Time
	calls   atomic.Int64
}

func (p *definitionPublisherV1) CreateSourceV1(_ context.Context, source definitioncontract.AgentDefinitionSourceV1) (definitionports.CreateDefinitionResultV1, error) {
	p.calls.Add(1)
	definition, err := definitioncontract.SealDefinitionV1(source, p.catalog, p.now.UnixNano())
	if err != nil {
		return definitionports.CreateDefinitionResultV1{}, err
	}
	current, err := definitioncontract.SealDefinitionCurrentV1(definitioncontract.DefinitionCurrentV1{Definition: definition.RefV1(), State: definitioncontract.DefinitionCurrentActiveV1, Revision: 1, UpdatedUnixNano: p.now.UnixNano(), CheckedUnixNano: p.now.UnixNano()})
	return definitionports.CreateDefinitionResultV1{Definition: definition, Current: current}, err
}

type sourcePublisherV1 struct {
	now   time.Time
	calls atomic.Int64
}

func (p *sourcePublisherV1) EnsureDefinitionSourceCurrentV1(_ context.Context, stableID string, definition definitioncontract.AgentDefinitionRefV1, notAfter int64) (contract.DefinitionSourceCurrentV1, error) {
	p.calls.Add(1)
	return contract.SealDefinitionSourceCurrentV1(contract.DefinitionSourceCurrentV1{
		ContractVersion: contract.ContractVersionV1, ObjectKind: contract.DefinitionSourceCurrentKindV1,
		SourceStableID:     stableID,
		DefinitionExactRef: contract.ExactRefV1{Kind: "praxis.agent-definition/definition", ID: definition.DefinitionID, Revision: uint64(definition.Revision), Digest: contract.DigestV1(definition.Digest)},
		Revision:           1, CheckedUnixNano: p.now.UnixNano(), ExpiresUnixNano: notAfter,
	})
}

type declarativeAssemblerStub struct{}

func (declarativeAssemblerStub) StartOrInspectDeclarativeAssemblyV1(context.Context, contract.HostConfigV1, contract.DefinitionSourceCurrentV1) (assemblercontract.ResolvedAgentPlanV1, error) {
	return assemblercontract.ResolvedAgentPlanV1{}, contract.NewError(contract.ErrorNotFound, "fixture_not_used", "fixture assembler is not used")
}
func (declarativeAssemblerStub) InspectDeclarativeAssemblyV1(context.Context, contract.HostConfigV1, contract.DefinitionSourceCurrentV1) (assemblercontract.ResolvedAgentPlanV1, error) {
	return assemblercontract.ResolvedAgentPlanV1{}, contract.NewError(contract.ErrorNotFound, "fixture_not_used", "fixture assembler is not used")
}

type deploymentReaderV1 struct {
	value contract.HostDeploymentCurrentV1
}

func (r deploymentReaderV1) InspectHostDeploymentCurrentV1(context.Context, contract.HostDeploymentCurrentRefV1) (contract.HostDeploymentCurrentV1, error) {
	return r.value, nil
}

type driftingDeploymentReaderV1 struct {
	value contract.HostDeploymentCurrentV1
	calls atomic.Int64
}

func (r *driftingDeploymentReaderV1) InspectHostDeploymentCurrentV1(context.Context, contract.HostDeploymentCurrentRefV1) (contract.HostDeploymentCurrentV1, error) {
	if r.calls.Add(1) > 1 {
		return contract.HostDeploymentCurrentV1{}, contract.NewError(contract.ErrorUnavailable, "deployment_changed", "deployment changed between reads")
	}
	return r.value, nil
}

type hostV3Stub struct {
	starts  atomic.Int64
	request contract.StartRequestV3
}

func (h *hostV3Stub) StartV3(_ context.Context, request contract.StartRequestV3) (contract.StartResultV3, error) {
	h.starts.Add(1)
	h.request = request
	claimInput, _ := request.ClaimInputV3()
	claim, _ := claimInput.ClaimV1()
	claimRef, _ := claim.CurrentRefV1()
	expires := request.RequestedNotAfterUnixNano
	digest := core.DigestBytes([]byte("root-result"))
	ready := contract.SystemReadyCurrentRefV2{ID: "ready-current", Revision: 1, Epoch: 1, Digest: digest, ExpiresUnixNano: expires}
	availability := runtimeports.AgentExecutionAvailabilityRefV1{Owner: core.OwnerRef{Domain: "praxis.agent-host", ID: "availability-owner"}, ID: "availability", Revision: 1, Epoch: 1, Digest: digest, ExpiresUnixNano: expires}
	return contract.SealStartResultV3(contract.StartResultV3{HostID: request.Config.HostID, StartID: request.StartID, RequestDigest: request.RequestDigest, RequestNotAfterUnixNano: request.RequestedNotAfterUnixNano, StartClaim: claimRef, Journal: rootExactRefV1("journal", contract.DigestV1(digest)), CleanupClosure: rootExactRefV1("cleanup", contract.DigestV1(digest)), Ready: ready, Availability: availability, CheckedUnixNano: request.RequestedAtUnixNano, ExpiresUnixNano: expires})
}
func (*hostV3Stub) InspectV3(context.Context, contract.InspectRequestV3) (contract.InspectResultV3, error) {
	return contract.InspectResultV3{}, contract.NewError(contract.ErrorNotFound, "fixture_not_used", "fixture inspect is not used")
}
func (*hostV3Stub) StopV3(context.Context, contract.StopRequestV3) (contract.StopResultV3, error) {
	return contract.StopResultV3{}, contract.NewError(contract.ErrorNotFound, "fixture_not_used", "fixture stop is not used")
}

func rootBootstrapFixtureV1(t *testing.T, now time.Time) contract.HostBootstrapConfigV1 {
	t.Helper()
	value, err := contract.SealHostBootstrapConfigV1(contract.HostBootstrapConfigV1{
		HostID: "host-1", StatePlaneBindingIDs: []string{"state-runtime"}, DefinitionSourceBindingID: "definition-source",
		CatalogBindingID: "catalog", ResolutionFactsBindingID: "resolution-facts", SecretBrokerBindingID: "secret-broker",
		CredentialRegistryBindingID: "credentials", ProviderEndpointRegistryBindingID: "provider-registry",
		RuntimeServiceBindingIDs: []string{"runtime"}, ApplicationServiceBindingIDs: []string{"application"}, HarnessServiceBindingIDs: []string{"harness"},
		ListenBindingID: "listen", DiagnosticsPolicyBindingID: "diagnostics", ShutdownPolicyBindingID: "shutdown",
		EnabledControlAPISurfaces: []string{"validate", "assemble", "run", "inspect", "stop"},
		CreatedUnixNano:           now.Add(-time.Minute).UnixNano(), NotAfterUnixNano: now.Add(2 * time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func rootDeploymentFixtureV1(t *testing.T, bootstrap contract.HostBootstrapConfigV1, now time.Time) contract.HostDeploymentCurrentV1 {
	t.Helper()
	expires := now.Add(time.Hour).UnixNano()
	owner := core.OwnerRef{Domain: "praxis.deployment", ID: "host-owner"}
	handles := []runtimeports.ResourceHandleRefV1{{Owner: owner, ID: "state-runtime", Revision: 1, Digest: core.DigestBytes([]byte("state-runtime")), Kind: "praxis/sqlite", ScopeDigest: core.DigestBytes([]byte("state-scope")), ExpiresUnixNano: expires}}
	roles := []struct {
		role contract.HostServiceBindingRoleV1
		ids  []string
	}{
		{contract.HostServiceDefinitionSourceV1, []string{bootstrap.DefinitionSourceBindingID}}, {contract.HostServiceCatalogV1, []string{bootstrap.CatalogBindingID}},
		{contract.HostServiceResolutionFactsV1, []string{bootstrap.ResolutionFactsBindingID}}, {contract.HostServiceSecretBrokerV1, []string{bootstrap.SecretBrokerBindingID}},
		{contract.HostServiceCredentialRegistryV1, []string{bootstrap.CredentialRegistryBindingID}}, {contract.HostServiceProviderRegistryV1, []string{bootstrap.ProviderEndpointRegistryBindingID}},
		{contract.HostServiceRuntimeV1, bootstrap.RuntimeServiceBindingIDs}, {contract.HostServiceApplicationV1, bootstrap.ApplicationServiceBindingIDs},
		{contract.HostServiceHarnessV1, bootstrap.HarnessServiceBindingIDs}, {contract.HostServiceListenV1, []string{bootstrap.ListenBindingID}},
		{contract.HostServiceDiagnosticsV1, []string{bootstrap.DiagnosticsPolicyBindingID}}, {contract.HostServiceShutdownV1, []string{bootstrap.ShutdownPolicyBindingID}},
	}
	services := []contract.HostServiceBindingRefV1{}
	for _, group := range roles {
		for _, id := range group.ids {
			services = append(services, contract.HostServiceBindingRefV1{Role: group.role, ConfiguredID: id, BindingRef: rootExactRefV1(string(group.role)+"-"+id, contract.DigestV1(core.DigestBytes([]byte(string(group.role)+id)))), Capability: "praxis.host/" + string(group.role), ExpiresUnixNano: expires})
		}
	}
	value, err := contract.SealHostDeploymentCurrentV1(contract.HostDeploymentCurrentV1{Ref: contract.HostDeploymentCurrentRefV1{HostID: bootstrap.HostID, DeploymentID: "deployment-1", Revision: 1, BootstrapDigest: bootstrap.ContentDigest, ExpiresUnixNano: expires}, ResourceHandles: handles, ServiceBindings: services, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func rootExactRefV1(id string, digest contract.DigestV1) contract.ExactRefV1 {
	return contract.ExactRefV1{Kind: "praxis.agent-host/fixture", ID: id, Revision: 1, Digest: digest}
}

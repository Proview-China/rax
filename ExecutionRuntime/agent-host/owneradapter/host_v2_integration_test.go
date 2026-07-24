package owneradapter

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	hostcontract "github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/lifecycle"
	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationfakes "github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblypublication"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestHostV2FullDeclarativeStartAndReadyReentryInspectOnly(t *testing.T) {
	fixture := newHostV2IntegrationFixture(t)
	result, err := fixture.host.StartV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if err = result.ValidateFor(fixture.request, fixture.now); err != nil {
		t.Fatal(err)
	}
	if len(result.Outputs.Controls) != 2 || result.Outputs.Ready.AttemptID == "" {
		t.Fatalf("incomplete HostV2 output: %+v", result.Outputs)
	}
	starts := fixture.startCounts()
	if _, err = fixture.host.StartV2(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	if got := fixture.startCounts(); got != starts {
		t.Fatalf("Ready reentry dispatched Owner work: before=%+v after=%+v", starts, got)
	}
	if fixture.pipeline.inspectDefinition.Load() == 0 || fixture.pipeline.inspectCompile.Load() == 0 || fixture.publicationInspect.Load() == 0 || fixture.ready.inspect.Load() < 2 {
		t.Fatalf("Ready reentry did not reconstruct through Inspect: pipeline=%+v publication=%d ready=%d", fixture.pipeline, fixture.publicationInspect.Load(), fixture.ready.inspect.Load())
	}
	value, err := fixture.journal.InspectHostJournalV2(context.Background(), fixture.request.Config.HostID, fixture.request.StartID)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"generation", "manifest", "graph", "handoff", "commit"} {
		found := false
		for _, operation := range value.Operations {
			if operation.AttemptID == fixture.publicationAttempt+"-publication-"+suffix && operation.State == hostcontract.HostOperationResultRecordedV2 {
				found = true
			}
		}
		if !found {
			t.Fatalf("publication %s result missing from HostJournal", suffix)
		}
	}
}

func TestHostV2SixtyFourIndependentCoordinatorsShareOneDispatchChain(t *testing.T) {
	fixture := newHostV2IntegrationFixture(t)
	start := make(chan struct{})
	var wait sync.WaitGroup
	var successes atomic.Int64
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			host, err := lifecycle.NewHostV2(fixture.config())
			if err != nil {
				t.Error(err)
				return
			}
			<-start
			if _, err = host.StartV2(context.Background(), fixture.request); err == nil {
				successes.Add(1)
			}
		}()
	}
	close(start)
	wait.Wait()
	if successes.Load() == 0 {
		t.Fatal("no HostV2 coordinator completed")
	}
	counts := fixture.startCounts()
	if counts.definition != 1 || counts.plan != 1 || counts.compile != 1 || counts.binding != 1 || counts.control != 2 || counts.activation != 1 || counts.generation != 1 || counts.ready != 1 {
		t.Fatalf("Owner dispatch did not linearize once per exact operation: %+v", counts)
	}
}

type hostV2StartCounts struct{ definition, plan, compile, publication, binding, control, activation, generation, ready int64 }

type hostV2IntegrationFixture struct {
	t                  *testing.T
	now                time.Time
	request            hostcontract.StartRequestV2
	journal            *journal.MemoryHostJournalStoreV2
	claims             *journal.MemoryHostStartClaimStoreV1
	coordinator        *journal.CoordinatorV2
	admission          *journal.HostStartAdmissionV1
	pipeline           *hostV2Pipeline
	publication        *countingAssemblyPublisherV2
	publicationInspect atomic.Int64
	publicationAttempt string
	inputs             *hostV2Inputs
	binding            *countingBindingAdmissionV1
	control            *hostV2ControlGateway
	activation         *countingActivationV1
	generation         *hostV2GenerationGateway
	ready              *hostV2ReadyGateway
	host               *lifecycle.HostV2
}

func newHostV2IntegrationFixture(t *testing.T) *hostV2IntegrationFixture {
	t.Helper()
	now := time.Unix(1_960_000_000, 0)
	request, err := hostcontract.SealStartRequestV2(hostcontract.StartRequestV2{StartID: "start-v2-full", Config: hostcontract.HostConfigV1{ContractVersion: hostcontract.ContractVersionV1, HostID: "host-v2-full", DefinitionSourceRef: "definition-source", StatePlaneBindings: []string{"state-plane"}, ProviderEndpointRefs: []string{"provider"}, SecretBrokerRef: "secret-broker", CatalogRef: "catalog", ResolutionFactsRef: "resolution", RuntimeServiceRefs: []string{"runtime"}, ListenRef: "listen", DiagnosticsPolicyRef: "diagnostics"}, DefinitionSourceCurrent: hostExactRefV2(t, "praxis.agent-definition/source", "definition-source"), RequestedAtUnixNano: now.Add(-time.Second).UnixNano(), RequestedNotAfterUnixNano: now.Add(30 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	publicationBase := publicationRequestFixtureV2(t, now)
	definition := hostcontract.DecodedDefinitionV1{Ref: hostExactRefV2(t, "praxis.agent-definition/definition", "definition-v2-full")}
	resolved := hostcontract.ResolvedAssemblyV1{PlanRef: hostExactRefV2(t, "praxis.agent-assembler/plan", "plan-v2-full"), InputRef: publicationBase.Artifacts.InputRef}
	pipeline := &hostV2Pipeline{definition: definition, resolved: resolved, compiled: publicationBase.Artifacts}
	journalStore := journal.NewMemoryHostJournalStoreV2()
	claimStore := journal.NewMemoryHostStartClaimStoreV1()
	coordinator, _ := journal.NewCoordinatorV2(journalStore, func() time.Time { return now })
	admission, _ := journal.NewHostStartAdmissionV1(claimStore, func() time.Time { return now })
	owner := core.OwnerRef{Domain: "praxis.harness", ID: "assembly-publication-owner"}
	publicationAdapter, err := NewAssemblyPublicationAdapterV2(assemblypublication.NewMemoryStoreV2(), journalStore, owner, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	publication := &countingAssemblyPublisherV2{inner: publicationAdapter}
	resources := hostV2Resources(t, now, []runtimeports.ComponentIDV2{"fixture/component-a", "fixture/component-b"})
	inputs := &hostV2Inputs{t: t, now: now, publicationBase: publicationBase, resources: resources}
	bindingStore := runtimefakes.NewBindingAdmissionStoreV1(func() time.Time { return now })
	binding := &countingBindingAdmissionV1{inner: bindingStore}
	control := &hostV2ControlGateway{values: map[string]hostcontract.ControlAdapterInstanceV2{}, now: now}
	activationInner, err := applicationfakes.NewAgentLifecycleV1(func(r applicationcontract.AgentActivationStartRequestV1) (applicationcontract.AgentActivationResultV1, error) {
		lease := core.SandboxLeaseRef{ID: "sandbox-lease-v2", Epoch: 1}
		scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-v2", ID: "agent-v2", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-v2", PlanDigest: r.PlanCurrent.Digest}, Instance: core.InstanceRef{ID: "instance-v2", Epoch: 1}, SandboxLease: &lease, AuthorityEpoch: 1}
		scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(scope)
		expires := now.Add(20 * time.Minute).UnixNano()
		return applicationcontract.SealAgentActivationResultV1(applicationcontract.AgentActivationResultV1{ActivationID: r.ActivationID, AttemptID: r.AttemptID, RequestDigest: r.RequestDigest, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, ActivationCurrent: hostOwnerCurrentV2("runtime", "activation-current", expires), SandboxLease: lease, SandboxLeaseCurrent: hostOwnerCurrentV2("sandbox", "sandbox-lease-current", expires), SandboxActiveCurrent: hostOwnerCurrentV2("sandbox", "sandbox-active-current", expires), ExecutionReadyCurrent: hostOwnerCurrentV2("harness", "execution-ready-current", expires), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	}, func(applicationcontract.AgentTerminationRequestV1) (applicationcontract.AgentTerminationResultV1, error) {
		return applicationcontract.AgentTerminationResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "termination not used")
	})
	if err != nil {
		t.Fatal(err)
	}
	activation := &countingActivationV1{inner: activationInner}
	generation := &hostV2GenerationGateway{facts: map[string]runtimeports.GenerationBindingAssociationFactV1{}, now: now}
	ready := &hostV2ReadyGateway{values: map[string]hostcontract.SystemReadyGatewayResultV2{}, now: now}
	fixture := &hostV2IntegrationFixture{t: t, now: now, request: request, journal: journalStore, claims: claimStore, coordinator: coordinator, admission: admission, pipeline: pipeline, publication: publication, publicationAttempt: "publication-v2-full", inputs: inputs, binding: binding, control: control, activation: activation, generation: generation, ready: ready}
	publication.inspectHook = func() { fixture.publicationInspect.Add(1) }
	host, err := lifecycle.NewHostV2(fixture.config())
	if err != nil {
		t.Fatal(err)
	}
	fixture.host = host
	return fixture
}

func (f *hostV2IntegrationFixture) config() lifecycle.ConfigV2 {
	return lifecycle.ConfigV2{StartAdmission: f.admission, Journal: f.coordinator, JournalFacts: f.journal, Definition: f.pipeline, Assembler: f.pipeline, Compiler: f.pipeline, Publisher: f.publication, PublicationRead: f.publication, Inputs: f.inputs, Binding: f.binding, Control: f.control, Activation: f.activation, Generation: f.generation, Ready: f.ready, Clock: func() time.Time { return f.now }}
}
func (f *hostV2IntegrationFixture) startCounts() hostV2StartCounts {
	return hostV2StartCounts{f.pipeline.startDefinition.Load(), f.pipeline.startPlan.Load(), f.pipeline.startCompile.Load(), f.publication.starts.Load(), f.binding.starts.Load(), f.control.starts.Load(), f.activation.starts.Load(), f.generation.starts.Load(), f.ready.starts.Load()}
}

type hostV2Pipeline struct {
	definition                                                                               hostcontract.DecodedDefinitionV1
	resolved                                                                                 hostcontract.ResolvedAssemblyV1
	compiled                                                                                 hostcontract.CompiledAssemblyArtifactsV2
	startDefinition, inspectDefinition, startPlan, inspectPlan, startCompile, inspectCompile atomic.Int64
}

func (p *hostV2Pipeline) StartOrInspectDefinitionV2(context.Context, hostcontract.StartRequestV2) (hostcontract.DecodedDefinitionV1, error) {
	p.startDefinition.Add(1)
	return p.definition, nil
}
func (p *hostV2Pipeline) InspectDefinitionV2(context.Context, hostcontract.StartRequestV2) (hostcontract.DecodedDefinitionV1, error) {
	p.inspectDefinition.Add(1)
	return p.definition, nil
}
func (p *hostV2Pipeline) StartOrInspectAssemblyPlanV2(context.Context, hostcontract.StartRequestV2, hostcontract.DecodedDefinitionV1) (hostcontract.ResolvedAssemblyV1, error) {
	p.startPlan.Add(1)
	return p.resolved, nil
}
func (p *hostV2Pipeline) InspectAssemblyPlanV2(context.Context, hostcontract.StartRequestV2, hostcontract.DecodedDefinitionV1) (hostcontract.ResolvedAssemblyV1, error) {
	p.inspectPlan.Add(1)
	return p.resolved, nil
}
func (p *hostV2Pipeline) StartOrInspectHarnessCompileV2(context.Context, hostcontract.StartRequestV2, hostcontract.ResolvedAssemblyV1) (hostcontract.CompiledAssemblyArtifactsV2, error) {
	p.startCompile.Add(1)
	return p.compiled, nil
}
func (p *hostV2Pipeline) InspectHarnessCompileV2(context.Context, hostcontract.StartRequestV2, hostcontract.ResolvedAssemblyV1) (hostcontract.CompiledAssemblyArtifactsV2, error) {
	p.inspectCompile.Add(1)
	return p.compiled, nil
}

type countingAssemblyPublisherV2 struct {
	inner       *AssemblyPublicationAdapterV2
	starts      atomic.Int64
	inspectHook func()
}

func (p *countingAssemblyPublisherV2) PublishAssemblyV2(ctx context.Context, request hostcontract.AssemblyPublicationRequestV2) (hostcontract.AssemblyPublicationResultV2, error) {
	p.starts.Add(1)
	return p.inner.PublishAssemblyV2(ctx, request)
}
func (p *countingAssemblyPublisherV2) InspectAssemblyPublicationV2(ctx context.Context, request hostcontract.AssemblyPublicationRequestV2) (hostcontract.AssemblyPublicationResultV2, error) {
	if p.inspectHook != nil {
		p.inspectHook()
	}
	return p.inner.InspectAssemblyPublicationV2(ctx, request)
}

type hostV2Inputs struct {
	t               *testing.T
	now             time.Time
	publicationBase hostcontract.AssemblyPublicationRequestV2
	resources       runtimeports.ResourceBindingSetV1
}

func (i *hostV2Inputs) BuildAssemblyPublicationRequestV2(_ context.Context, start hostcontract.StartRequestV2, compiled hostcontract.CompiledAssemblyArtifactsV2) (hostcontract.AssemblyPublicationRequestV2, error) {
	value := i.publicationBase
	value.HostID, value.StartID, value.AttemptID, value.Artifacts, value.RequestedExpiresUnixNano = start.Config.HostID, start.StartID, "publication-v2-full", compiled, start.RequestedNotAfterUnixNano
	return value, value.ValidateAt(i.now)
}
func (i *hostV2Inputs) BuildBindingAdmissionRequestV2(_ context.Context, start hostcontract.StartRequestV2, _ hostcontract.DecodedDefinitionV1, _ hostcontract.ResolvedAssemblyV1, assembly hostcontract.AssemblyPublicationResultV2) (runtimeports.BindingAdmissionRequestV1, error) {
	current := func(id string) runtimeports.OwnerCurrentRefV1 {
		return hostOwnerCurrentV2("fixture", id, start.RequestedNotAfterUnixNano)
	}
	releases := []runtimeports.PreBindingComponentReleaseV1{}
	for _, component := range []runtimeports.ComponentIDV2{"fixture/component-a", "fixture/component-b"} {
		releases = append(releases, runtimeports.PreBindingComponentReleaseV1{ComponentID: component, Release: current("release-" + string(component)), Certification: current("cert-" + string(component)), DeploymentReadiness: current("deploy-" + string(component))})
	}
	return runtimeports.SealBindingAdmissionRequestV1(runtimeports.BindingAdmissionRequestV1{AttemptID: "binding-v2-full", DefinitionCurrent: current("definition"), PlanCurrent: current("plan"), AssemblyCurrent: assembly.OwnerCurrent, CatalogCurrent: current("catalog"), ResolutionCurrent: current("resolution"), Releases: releases, ResourceBindingSet: i.resources.Ref, AuthorityCurrent: current("authority"), PolicyCurrent: current("policy"), ExpectedBindingSetID: "binding-set-v2-full", RequestedNotAfterUnixNano: start.RequestedNotAfterUnixNano})
}
func (i *hostV2Inputs) BuildControlAdapterRequestsV2(_ context.Context, start hostcontract.StartRequestV2, assembly hostcontract.AssemblyPublicationResultV2, binding runtimeports.BindingAdmissionResultV1) ([]hostcontract.ControlAdapterConstructRequestV2, error) {
	result := make([]hostcontract.ControlAdapterConstructRequestV2, 0, len(binding.Bindings))
	for index, bound := range binding.Bindings {
		request, err := hostControlRequestV2(i.t, i.now, start, assembly, bound, i.resources, index)
		if err != nil {
			return nil, err
		}
		result = append(result, request)
	}
	return result, nil
}
func (i *hostV2Inputs) BuildAgentActivationRequestV2(_ context.Context, start hostcontract.StartRequestV2, assembly hostcontract.AssemblyPublicationResultV2, binding runtimeports.BindingAdmissionResultV1, _ []hostcontract.ControlAdapterInstanceV2) (applicationcontract.AgentActivationStartRequestV1, error) {
	current := func(domain, id string) runtimeports.OwnerCurrentRefV1 {
		return hostOwnerCurrentV2(domain, id, start.RequestedNotAfterUnixNano)
	}
	return applicationcontract.SealAgentActivationStartRequestV1(applicationcontract.AgentActivationStartRequestV1{ActivationID: "activation-v2-full", AttemptID: "activation-attempt-v2-full", IdempotencyKey: "activation-key-v2-full", DefinitionCurrent: current("definition", "definition"), PlanCurrent: current("assembler", "plan"), AssemblyCurrent: assembly.OwnerCurrent, BindingSetCurrent: runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: "praxis.runtime", ID: "binding-owner"}, ContractVersion: runtimeports.BindingAdmissionContractVersionV1, ID: binding.BindingSet.ID, Revision: binding.BindingSet.Revision, Digest: binding.BindingSet.Digest, ExpiresUnixNano: binding.BindingSet.ExpiresUnixNano}, AuthorityCurrent: current("runtime", "authority"), PolicyCurrent: current("policy", "policy"), BudgetCurrent: current("budget", "budget"), CredentialCurrent: current("credential", "credential"), SandboxAdapterBinding: current("sandbox", "sandbox-adapter"), ExecutionAdapterBinding: current("harness", "execution-adapter"), RequestedNotAfterUnixNano: start.RequestedNotAfterUnixNano})
}
func (i *hostV2Inputs) BuildGenerationAssociationCandidateV2(_ context.Context, _ hostcontract.StartRequestV2, assembly hostcontract.AssemblyPublicationResultV2, binding runtimeports.BindingAdmissionResultV1, activation applicationcontract.AgentActivationResultV1) (runtimeports.GenerationBindingAssociationCandidateV1, error) {
	return hostGenerationCandidateV2(i.t, i.now, assembly, binding, activation)
}
func (i *hostV2Inputs) BuildSystemReadyRequestV2(_ context.Context, start hostcontract.StartRequestV2, claim hostcontract.HostStartClaimV1, _ hostcontract.DecodedDefinitionV1, _ hostcontract.ResolvedAssemblyV1, assembly hostcontract.AssemblyPublicationResultV2, binding runtimeports.BindingAdmissionResultV1, controls []hostcontract.ControlAdapterInstanceV2, activation applicationcontract.AgentActivationResultV1, generation runtimeports.GenerationBindingAssociationFactV1) (hostcontract.SystemReadyEnsureRequestV2, error) {
	claimRef, _ := claim.CurrentRefV1()
	current := func(domain, id string) runtimeports.OwnerCurrentRefV1 {
		return hostOwnerCurrentV2(domain, id, start.RequestedNotAfterUnixNano)
	}
	components := make([]hostcontract.ComponentProductionCurrentV2, len(binding.Bindings))
	for index, bound := range binding.Bindings {
		components[index] = hostcontract.ComponentProductionCurrentV2{Domain: runtimeports.NamespacedNameV2(bound.ComponentID), ReleaseCurrent: current("release", "release-"+fmt.Sprint(index)), ConstructedComponent: controls[index].InstanceRef, Binding: bound, GenerationCurrent: current("generation", "generation-"+fmt.Sprint(index)), ActivationCurrent: current("activation", "activation-"+fmt.Sprint(index)), ProductionCurrent: current("production", "production-"+fmt.Sprint(index))}
	}
	return hostcontract.SealSystemReadyEnsureRequestV2(hostcontract.SystemReadyEnsureRequestV2{AttemptID: "ready-v2-full", HostID: start.Config.HostID, StartID: start.StartID, Claim: claimRef, Definition: current("definition", "definition"), Plan: current("assembler", "plan"), Assembly: assembly.OwnerCurrent, BindingSet: runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: "praxis.runtime", ID: "binding-owner"}, ContractVersion: runtimeports.BindingAdmissionContractVersionV1, ID: binding.BindingSet.ID, Revision: binding.BindingSet.Revision, Digest: binding.BindingSet.Digest, ExpiresUnixNano: binding.BindingSet.ExpiresUnixNano}, Activation: activation.ActivationCurrent, GenerationBinding: hostOwnerCurrentV2("runtime", generation.ID, generation.ExpiresUnixNano), ApplicationStart: activation.Ref, SandboxLease: activation.SandboxLeaseCurrent, SandboxActive: activation.SandboxActiveCurrent, ExecutionReady: activation.ExecutionReadyCurrent, SupervisionPolicy: current("runtime", "supervision"), Components: components, MinimumReadyWindowNanos: int64(time.Minute), AvailabilityEpoch: 1})
}

type countingBindingAdmissionV1 struct {
	inner  *runtimefakes.BindingAdmissionStoreV1
	starts atomic.Int64
}

func (b *countingBindingAdmissionV1) StartOrInspectBindingAdmissionV1(ctx context.Context, request runtimeports.BindingAdmissionRequestV1) (runtimeports.BindingAdmissionResultV1, error) {
	b.starts.Add(1)
	return b.inner.StartOrInspectBindingAdmissionV1(ctx, request)
}
func (b *countingBindingAdmissionV1) InspectBindingAdmissionV1(ctx context.Context, request runtimeports.BindingAdmissionInspectRequestV1) (runtimeports.BindingAdmissionResultV1, error) {
	return b.inner.InspectBindingAdmissionV1(ctx, request)
}

type hostV2ControlGateway struct {
	mu     sync.Mutex
	values map[string]hostcontract.ControlAdapterInstanceV2
	now    time.Time
	starts atomic.Int64
}

func (g *hostV2ControlGateway) StartOrInspectControlAdapterConstructionV2(_ context.Context, request hostcontract.ControlAdapterConstructRequestV2) (hostcontract.ControlAdapterInstanceV2, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if value, ok := g.values[request.AttemptID]; ok {
		return value, nil
	}
	g.starts.Add(1)
	value, err := hostcontract.SealControlAdapterInstanceV2(hostcontract.ControlAdapterInstanceV2{InstanceRef: hostExactRefV2NoT("praxis.agent-host/control-adapter-instance", "instance-"+request.AttemptID), AttemptID: request.AttemptID, RequestDigest: request.RequestDigest, DescriptorRef: request.Descriptor.Ref, CheckedUnixNano: g.now.UnixNano(), ExpiresUnixNano: request.RequestedNotAfterUnixNano})
	if err == nil {
		g.values[request.AttemptID] = value
	}
	return value, err
}
func (g *hostV2ControlGateway) InspectControlAdapterConstructionV2(_ context.Context, request hostcontract.ControlAdapterConstructRequestV2) (hostcontract.ControlAdapterInstanceV2, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	value, ok := g.values[request.AttemptID]
	if !ok {
		return hostcontract.ControlAdapterInstanceV2{}, hostcontract.NewError(hostcontract.ErrorNotFound, "control_missing", "control missing")
	}
	return value, nil
}

type countingActivationV1 struct {
	inner  *applicationfakes.AgentLifecycleV1
	starts atomic.Int64
}

func (a *countingActivationV1) StartOrInspectAgentActivationV1(ctx context.Context, request applicationcontract.AgentActivationStartRequestV1) (applicationcontract.AgentActivationResultV1, error) {
	a.starts.Add(1)
	return a.inner.StartOrInspectAgentActivationV1(ctx, request)
}
func (a *countingActivationV1) InspectAgentActivationV1(ctx context.Context, request applicationcontract.AgentActivationStartRequestV1) (applicationcontract.AgentActivationResultV1, error) {
	return a.inner.InspectAgentActivationV1(ctx, request)
}

type hostV2GenerationGateway struct {
	mu     sync.Mutex
	facts  map[string]runtimeports.GenerationBindingAssociationFactV1
	now    time.Time
	starts atomic.Int64
}

func (g *hostV2GenerationGateway) AssociateGenerationBindingV1(_ context.Context, candidate runtimeports.GenerationBindingAssociationCandidateV1) (runtimeports.GenerationBindingAssociationFactV1, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if value, ok := g.facts[candidate.AssociationID]; ok {
		return value, nil
	}
	g.starts.Add(1)
	expires := candidate.RequestedExpiresUnixNano
	for _, value := range []int64{candidate.Generation.ExpiresUnixNano, candidate.Binding.ExpiresUnixNano, candidate.Activation.ExpiresUnixNano} {
		if value < expires {
			expires = value
		}
	}
	value, err := runtimeports.SealGenerationBindingAssociationFactV1(runtimeports.GenerationBindingAssociationFactV1{ID: candidate.AssociationID, Revision: 1, State: runtimeports.GenerationBindingAssociationActiveV1, Candidate: candidate, CandidateDigest: candidate.Digest, CreatedUnixNano: g.now.UnixNano(), UpdatedUnixNano: g.now.UnixNano(), ExpiresUnixNano: expires})
	if err == nil {
		g.facts[candidate.AssociationID] = value
	}
	return value, err
}
func (g *hostV2GenerationGateway) InspectCurrentGenerationBindingAssociationV1(_ context.Context, id string) (runtimeports.GenerationBindingAssociationFactV1, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	value, ok := g.facts[id]
	if !ok {
		return runtimeports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "generation missing")
	}
	return value, nil
}

type hostV2ReadyGateway struct {
	mu              sync.Mutex
	values          map[string]hostcontract.SystemReadyGatewayResultV2
	now             time.Time
	starts, inspect atomic.Int64
}

func (g *hostV2ReadyGateway) StartOrInspectSystemReadyV2(_ context.Context, request hostcontract.SystemReadyEnsureRequestV2) (hostcontract.SystemReadyGatewayResultV2, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if value, ok := g.values[request.AttemptID]; ok {
		return value, nil
	}
	g.starts.Add(1)
	expires := g.now.Add(10 * time.Minute).UnixNano()
	value, err := hostcontract.SealSystemReadyGatewayResultV2(hostcontract.SystemReadyGatewayResultV2{AttemptID: request.AttemptID, RequestDigest: request.RequestDigest, Fact: hostcontract.SystemReadyFactRefV2{ID: "ready-fact", Revision: 1, Digest: hostCoreDigestV2("ready-fact"), ExpiresUnixNano: expires}, Current: hostcontract.SystemReadyCurrentRefV2{ID: "ready-current", Revision: 1, Epoch: 1, Digest: hostCoreDigestV2("ready-current"), ExpiresUnixNano: expires}})
	if err == nil {
		g.values[request.AttemptID] = value
	}
	return value, err
}
func (g *hostV2ReadyGateway) InspectSystemReadyV2(_ context.Context, request hostcontract.SystemReadyInspectRequestV2) (hostcontract.SystemReadyGatewayResultV2, error) {
	g.inspect.Add(1)
	g.mu.Lock()
	defer g.mu.Unlock()
	value, ok := g.values[request.AttemptID]
	if !ok {
		return hostcontract.SystemReadyGatewayResultV2{}, hostcontract.NewError(hostcontract.ErrorNotFound, "ready_missing", "ready missing")
	}
	return value, nil
}

func hostV2Resources(t *testing.T, now time.Time, components []runtimeports.ComponentIDV2) runtimeports.ResourceBindingSetV1 {
	t.Helper()
	owner := core.OwnerRef{Domain: "fixture.resources", ID: "resource-owner"}
	expires := now.Add(time.Hour).UnixNano()
	bindings := make([]runtimeports.ResourceBindingV1, 0, len(components))
	for index, component := range components {
		cleanup := hostOwnerCurrentV2("resource", fmt.Sprintf("cleanup-%d", index), expires)
		deploy := hostOwnerCurrentV2("resource", fmt.Sprintf("deploy-%d", index), expires)
		handle, err := runtimeports.SealResourceHandleCurrentV1(runtimeports.ResourceHandleCurrentV1{Ref: runtimeports.ResourceHandleRefV1{Owner: owner, ID: fmt.Sprintf("resource-%d", index), Revision: 1, Kind: "fixture/sqlite", ScopeDigest: hostCoreDigestV2(fmt.Sprintf("scope-%d", index))}, CleanupContract: cleanup, DeploymentAttestation: deploy, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
		if err != nil {
			t.Fatal(err)
		}
		bindings = append(bindings, runtimeports.ResourceBindingV1{ComponentID: component, Handle: handle.Ref, CleanupContract: cleanup, DeploymentAttestation: deploy})
	}
	value, err := runtimeports.SealResourceBindingSetV1(runtimeports.ResourceBindingSetV1{Ref: runtimeports.ResourceBindingSetRefV1{ID: "resource-set-v2-full", Revision: 1}, Bindings: bindings, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func hostControlRequestV2(t *testing.T, now time.Time, start hostcontract.StartRequestV2, assembly hostcontract.AssemblyPublicationResultV2, bound runtimeports.BindingAdmissionBindingRefV1, resources runtimeports.ResourceBindingSetV1, index int) (hostcontract.ControlAdapterConstructRequestV2, error) {
	handles := make([]runtimeports.ResourceHandleRefV1, len(resources.Bindings))
	for i := range resources.Bindings {
		handles[i] = resources.Bindings[i].Handle
	}
	descriptor, err := hostcontract.SealControlAdapterFactoryDescriptorV2(hostcontract.ControlAdapterFactoryDescriptorV2{Ref: hostcontract.ControlAdapterFactoryRefV2{FactoryID: fmt.Sprintf("factory/control-%02d", index), Revision: 1}, ComponentID: bound.ComponentID, ArtifactDigest: hostCoreDigestV2(fmt.Sprintf("factory-artifact-%d", index)), ComponentContract: "1.0.0", Capability: runtimeports.CapabilityNameV2(fmt.Sprintf("fixture/control-%d", index)), Binding: bound, Generation: assembly.OwnerCurrent, ResourceBindingSet: resources.Ref, ResourceHandles: handles, OutputPortCapabilities: []runtimeports.CapabilityNameV2{runtimeports.CapabilityNameV2(fmt.Sprintf("fixture/control-current-%d", index))}, EffectClass: hostcontract.ControlAdapterEffectNoneV2})
	if err != nil {
		return hostcontract.ControlAdapterConstructRequestV2{}, err
	}
	evidence := func(id string) runtimeports.OwnerCurrentRefV1 {
		return hostOwnerCurrentV2("certification", id, start.RequestedNotAfterUnixNano)
	}
	conformance, err := hostcontract.SealControlAdapterConformanceV2(hostcontract.ControlAdapterConformanceV2{ConformanceID: fmt.Sprintf("conformance-%02d", index), Revision: 1, DescriptorRef: descriptor.Ref, CertificationCurrent: evidence(fmt.Sprintf("cert-%d", index)), StaticImportEvidence: evidence(fmt.Sprintf("imports-%d", index)), NoRawProviderEvidence: evidence(fmt.Sprintf("provider-%d", index)), ZeroEffectEvidence: evidence(fmt.Sprintf("effect-%d", index)), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: start.RequestedNotAfterUnixNano})
	if err != nil {
		return hostcontract.ControlAdapterConstructRequestV2{}, err
	}
	return hostcontract.SealControlAdapterConstructRequestV2(hostcontract.ControlAdapterConstructRequestV2{HostID: start.Config.HostID, StartID: start.StartID, AttemptID: fmt.Sprintf("control-attempt-%02d", index), Descriptor: descriptor, Conformance: conformance, ResourceBindings: resources, RequestedNotAfterUnixNano: start.RequestedNotAfterUnixNano})
}
func hostGenerationCandidateV2(t *testing.T, now time.Time, assembly hostcontract.AssemblyPublicationResultV2, binding runtimeports.BindingAdmissionResultV1, activation applicationcontract.AgentActivationResultV1) (runtimeports.GenerationBindingAssociationCandidateV1, error) {
	components := make([]runtimeports.GenerationComponentManifestRefV1, len(binding.Bindings))
	for index, bound := range binding.Bindings {
		components[index] = runtimeports.GenerationComponentManifestRefV1{ComponentID: bound.ComponentID, ManifestDigest: hostCoreDigestV2("manifest-" + string(bound.ComponentID)), ArtifactDigest: hostCoreDigestV2("artifact-" + string(bound.ComponentID))}
	}
	generation, err := runtimeports.SealGenerationCurrentProjectionV1(runtimeports.GenerationCurrentProjectionV1{Generation: runtimeports.GenerationArtifactRefV1{ID: assembly.Generation.ID, Revision: core.Revision(assembly.Generation.Revision), Digest: core.Digest(assembly.Generation.Digest), InputDigest: hostCoreDigestV2("input"), ManifestDigest: core.Digest(assembly.Manifest.Digest), GraphDigest: core.Digest(assembly.Graph.Digest), CatalogDigest: hostCoreDigestV2("catalog")}, ComponentManifests: components, Extension: runtimeports.GenerationGovernanceExtensionRefV1{Kind: "praxis.harness/assembly-generation", Contract: runtimeports.SchemaRefV2{Namespace: "praxis.harness", Name: "assembly-generation", Version: "1.0.0", MediaType: "application/json", ContentDigest: hostCoreDigestV2("schema")}, Digest: hostCoreDigestV2("extension")}, State: runtimeports.GenerationCurrentSealedV1, Current: true, Watermark: 1, ExpiresUnixNano: assembly.OwnerCurrent.ExpiresUnixNano})
	if err != nil {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, err
	}
	bindingProjection, err := runtimeports.SealGenerationBindingSetCurrentProjectionV1(runtimeports.GenerationBindingSetCurrentProjectionV1{BindingSetID: binding.BindingSet.ID, BindingSetRevision: binding.BindingSet.Revision, BindingSetDigest: binding.BindingSet.Digest, BindingSetSemanticDigest: hostCoreDigestV2("binding-semantic"), PlanDigest: hostCoreDigestV2("plan"), GovernanceDigest: hostCoreDigestV2("governance"), ComponentManifestSetDigest: runtimeports.GenerationComponentManifestSetDigestV1(components), CurrentnessDigest: hostCoreDigestV2("binding-current"), IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: binding.ExpiresUnixNano})
	if err != nil {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, err
	}
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeActivationV3, ExecutionScope: activation.ExecutionScope, ExecutionScopeDigest: activation.ExecutionScopeDigest, ActivationAttemptID: activation.AttemptID, SubjectRevision: 1, CurrentProjectionRef: "activation-projection-v2", CurrentProjectionDigest: hostCoreDigestV2("activation-projection"), CurrentProjectionRevision: 1}
	operationDigest, _ := operation.DigestV3()
	activationProjection, err := runtimeports.SealGenerationActivationCurrentProjectionV1(runtimeports.GenerationActivationCurrentProjectionV1{Operation: operation, OperationDigest: operationDigest, Active: true, Watermark: 1, CurrentnessDigest: hostCoreDigestV2("activation-current"), ExpiresUnixNano: activation.ExpiresUnixNano})
	if err != nil {
		return runtimeports.GenerationBindingAssociationCandidateV1{}, err
	}
	return runtimeports.SealGenerationBindingAssociationCandidateV1(runtimeports.GenerationBindingAssociationCandidateV1{AssociationID: "generation-association-v2-full", Generation: generation, Binding: bindingProjection, Activation: activationProjection, RequestedExpiresUnixNano: activation.ExpiresUnixNano})
}
func hostOwnerCurrentV2(domain, id string, expires int64) runtimeports.OwnerCurrentRefV1 {
	return runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: "praxis." + domain, ID: core.OwnerID("owner-" + domain)}, ContractVersion: "1.0.0", ID: id, Revision: 1, Digest: hostCoreDigestV2(domain + "\x00" + id), ExpiresUnixNano: expires}
}
func hostExactRefV2(t *testing.T, kind, id string) hostcontract.ExactRefV1 {
	t.Helper()
	return hostExactRefV2NoT(kind, id)
}
func hostExactRefV2NoT(kind, id string) hostcontract.ExactRefV1 {
	return hostcontract.ExactRefV1{Kind: kind, ID: id, Revision: 1, Digest: hostcontract.DigestV1(hostCoreDigestV2(kind + "\x00" + id))}
}
func hostCoreDigestV2(value string) core.Digest { return core.DigestBytes([]byte(value)) }

var _ = sort.Strings

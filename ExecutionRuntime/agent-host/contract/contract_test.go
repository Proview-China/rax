package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

func digest(t *testing.T, value string) contract.DigestV1 {
	t.Helper()
	result, err := contract.DigestJSONV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func ref(t *testing.T, kind, id string) contract.ExactRefV1 {
	return contract.ExactRefV1{Kind: kind, ID: id, Revision: 1, Digest: digest(t, id)}
}
func key(t *testing.T, id string) contract.FactoryKeyV1 {
	return contract.FactoryKeyV1{ComponentID: id, ArtifactDigest: digest(t, "artifact-"+id), Contract: "praxis.fixture/contract-v1", Capability: "praxis.fixture/capability"}
}

func TestConstructionGraphDeterministicOrderAndFailures(t *testing.T) {
	graph := contract.ConstructionGraphV1{GraphRef: ref(t, "praxis.harness/graph", "graph-1"), Nodes: []contract.ComponentNodeV1{{NodeID: "b", Factory: key(t, "b"), Dependencies: []string{"a"}}, {NodeID: "c", Factory: key(t, "c"), Dependencies: []string{"a"}}, {NodeID: "a", Factory: key(t, "a")}}}
	if err := graph.Validate(); err != nil {
		t.Fatal(err)
	}
	order, err := graph.DependencyOrderV1()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := join(order), "a,b,c"; got != want {
		t.Fatalf("order=%s want=%s", got, want)
	}
	missing := graph
	missing.Nodes = append([]contract.ComponentNodeV1(nil), graph.Nodes...)
	missing.Nodes[0].Dependencies = []string{"absent"}
	if !contract.HasCode(missing.Validate(), contract.ErrorPrecondition) {
		t.Fatal("missing dependency accepted")
	}
	cycle := graph
	cycle.Nodes = append([]contract.ComponentNodeV1(nil), graph.Nodes...)
	cycle.Nodes[2].Dependencies = []string{"b"}
	if !contract.HasCode(cycle.Validate(), contract.ErrorPrecondition) {
		t.Fatal("cycle accepted")
	}
	alias := graph
	alias.Nodes = append([]contract.ComponentNodeV1(nil), graph.Nodes...)
	alias.Nodes[1].Factory = alias.Nodes[0].Factory
	if !contract.HasCode(alias.Validate(), contract.ErrorConflict) {
		t.Fatal("same exact factory bound to multiple nodes")
	}
}

func TestHostConfigCanonicalDigestAndSecretRefOnlyShape(t *testing.T) {
	config := contract.HostConfigV1{ContractVersion: contract.ContractVersionV1, HostID: "host-1", DefinitionSourceRef: "asset:/definition/1", StatePlaneBindings: []string{"state:/b", "state:/a"}, ProviderEndpointRefs: []string{"provider:/b", "provider:/a"}, SecretBrokerRef: "secret:/broker/1", CatalogRef: "catalog:/1", ResolutionFactsRef: "facts:/1", RuntimeServiceRefs: []string{"runtime:/b", "runtime:/a"}, ListenRef: "listen:/local", DiagnosticsPolicyRef: "policy:/diag"}
	first, err := config.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	config.StatePlaneBindings[0], config.StatePlaneBindings[1] = config.StatePlaneBindings[1], config.StatePlaneBindings[0]
	second, err := config.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("config digest depends on input order")
	}
	config.SecretBrokerRef = " plaintext secret "
	if !contract.HasCode(config.Validate(), contract.ErrorInvalidArgument) {
		t.Fatal("non-ref secret value accepted")
	}
}

func TestJournalSuccessorIsExactAppendOnly(t *testing.T) {
	now := time.Now()
	base, _ := contract.SealHostJournalV1(contract.HostJournalV1{ContractVersion: contract.ContractVersionV1, HostID: "host-1", StartID: "start-1", Revision: 1, Phase: contract.HostAcceptedV1, ConfigDigest: digest(t, "config"), CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	next := base
	next.Revision = 2
	next.Phase = contract.HostValidatingV1
	definition := ref(t, "praxis.definition", "definition")
	next.DefinitionRef = &definition
	next.UpdatedUnixNano++
	next, _ = contract.SealHostJournalV1(next)
	if err := contract.ValidateJournalSuccessorV1(base, next); err != nil {
		t.Fatal(err)
	}
	bad := next
	bad.Revision = 4
	bad, _ = contract.SealHostJournalV1(bad)
	if !contract.HasCode(contract.ValidateJournalSuccessorV1(base, bad), contract.ErrorConflict) {
		t.Fatal("revision jump accepted")
	}
	regress := next
	regress.Revision = 3
	regress.Phase = contract.HostAcceptedV1
	regress.UpdatedUnixNano++
	regress, _ = contract.SealHostJournalV1(regress)
	if !contract.HasCode(contract.ValidateJournalSuccessorV1(next, regress), contract.ErrorPrecondition) {
		t.Fatal("phase regression accepted")
	}
}

func TestJournalSuccessorRejectsDirectAttemptOutcomeAndBatchAppend(t *testing.T) {
	now := time.Now()
	configDigest := digest(t, "config")
	base, _ := contract.SealHostJournalV1(contract.HostJournalV1{ContractVersion: contract.ContractVersionV1, HostID: "host-1", StartID: "start-1", Revision: 1, Phase: contract.HostAcceptedV1, ConfigDigest: configDigest, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	definition := contract.DecodedDefinitionV1{Ref: ref(t, "praxis.definition", "definition")}
	resolved := contract.ResolvedAssemblyV1{PlanRef: ref(t, "praxis.plan", "plan"), InputRef: ref(t, "praxis.input", "input")}
	graph := contract.ConstructionGraphV1{GraphRef: ref(t, "praxis.graph", "graph"), Nodes: []contract.ComponentNodeV1{{NodeID: "node", Factory: key(t, "node")}}}
	compiled := contract.CompiledAssemblyV1{GenerationRef: ref(t, "praxis.generation", "generation"), ManifestRef: ref(t, "praxis.manifest", "manifest"), Graph: graph, HandoffRef: ref(t, "praxis.handoff", "handoff")}
	binding, _ := contract.NewBindingAttemptV1("host-1", "start-1", configDigest, definition, resolved, compiled)
	binding.State = contract.AttemptBoundV1
	bindingRef := ref(t, "praxis.binding", "binding")
	binding.BindingRef = &bindingRef
	binding, _ = contract.SealBindingAttemptV1(binding)
	directBinding := base
	directBinding.Revision++
	directBinding.Phase = contract.HostValidatingV1
	directBinding.UpdatedUnixNano++
	directBinding.DefinitionRef = &definition.Ref
	directBinding.BindingAttempt = &binding
	directBinding.BindingRef = &bindingRef
	directBinding, _ = contract.SealHostJournalV1(directBinding)
	if !contract.HasCode(contract.ValidateJournalSuccessorV1(base, directBinding), contract.ErrorPrecondition) {
		t.Fatal("binding first append bypassed planned")
	}
	attempt, _ := contract.NewConstructionAttemptV1("host-1", "start-1", graph.GraphRef, graph.Nodes[0], nil)
	unknown := attempt
	unknown.State = contract.AttemptUnknownV1
	unknown.Reason = "unknown"
	unknown, _ = contract.SealConstructionAttemptV1(unknown)
	directConstruction := base
	directConstruction.Revision++
	directConstruction.Phase = contract.HostValidatingV1
	directConstruction.UpdatedUnixNano++
	directConstruction.DefinitionRef = &definition.Ref
	directConstruction.ConstructionAttempts = []contract.ConstructionAttemptV1{unknown}
	directConstruction, _ = contract.SealHostJournalV1(directConstruction)
	if !contract.HasCode(contract.ValidateJournalSuccessorV1(base, directConstruction), contract.ErrorPrecondition) {
		t.Fatal("construction first append bypassed planned")
	}
	secondGraph := graph
	secondGraph.GraphRef = ref(t, "praxis.graph", "graph-2")
	secondGraph.Nodes[0].NodeID = "node-2"
	secondGraph.Nodes[0].Factory = key(t, "node-2")
	second, _ := contract.NewConstructionAttemptV1("host-1", "start-1", secondGraph.GraphRef, secondGraph.Nodes[0], nil)
	batch := base
	batch.Revision++
	batch.Phase = contract.HostValidatingV1
	batch.UpdatedUnixNano++
	batch.DefinitionRef = &definition.Ref
	batch.ConstructionAttempts = []contract.ConstructionAttemptV1{attempt, second}
	batch, _ = contract.SealHostJournalV1(batch)
	if !contract.HasCode(contract.ValidateJournalSuccessorV1(base, batch), contract.ErrorConflict) {
		t.Fatal("multiple construction attempts appended in one successor")
	}
}

func TestSystemReadyRequiresAllProductionReleases(t *testing.T) {
	now := time.Now()
	components := []contract.ConstructedComponentV1{{NodeID: "node", Factory: key(t, "component"), ComponentRef: ref(t, "praxis.component/instance", "component-1")}}
	ready := contract.SystemReadyV1{ContractVersion: contract.ContractVersionV1, HostID: "host-1", StartID: "start-1", DefinitionRef: ref(t, "praxis.definition", "definition"), PlanRef: ref(t, "praxis.plan", "plan"), GenerationRef: ref(t, "praxis.generation", "generation"), HandoffRef: ref(t, "praxis.handoff", "handoff"), BindingRef: ref(t, "praxis.binding", "binding"), Components: components, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	for _, domain := range contract.RequiredReleaseDomainsV1() {
		ready.Releases = append(ready.Releases, contract.ReleaseCurrentV1{Domain: domain, ReleaseRef: ref(t, "praxis.release", domain), Production: true, ExpiresUnixNano: now.Add(time.Minute).UnixNano()})
	}
	ready, _ = contract.SealSystemReadyV1(ready)
	if err := ready.Validate(now); err != nil {
		t.Fatal(err)
	}
	ready.Releases = ready.Releases[:len(ready.Releases)-1]
	ready, _ = contract.SealSystemReadyV1(ready)
	if !contract.HasCode(ready.Validate(now), contract.ErrorPrecondition) {
		t.Fatal("missing release accepted")
	}
	ready.Releases = append(ready.Releases, contract.ReleaseCurrentV1{Domain: contract.RequiredReleaseDomainsV1()[len(contract.RequiredReleaseDomainsV1())-1], ReleaseRef: ref(t, "praxis.release", "restored"), Production: true, ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	ready.ExpiresUnixNano = now.Add(time.Minute).UnixNano()
	ready, _ = contract.SealSystemReadyV1(ready)
	if !contract.HasCode(ready.Validate(now), contract.ErrorPrecondition) {
		t.Fatal("ready lifetime exceeded release lifetime")
	}
}

func join(values []string) string {
	result := ""
	for i, value := range values {
		if i > 0 {
			result += ","
		}
		result += value
	}
	return result
}

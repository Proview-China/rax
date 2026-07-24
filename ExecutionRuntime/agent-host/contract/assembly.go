package contract

import (
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
)

type FactoryKeyV1 struct {
	ComponentID    string   `json:"component_id"`
	ArtifactDigest DigestV1 `json:"artifact_digest"`
	Contract       string   `json:"contract"`
	Capability     string   `json:"capability"`
}

func (k FactoryKeyV1) Validate() error {
	if err := ValidateIdentifierV1("component id", k.ComponentID); err != nil {
		return err
	}
	if err := k.ArtifactDigest.Validate(); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("contract", k.Contract); err != nil {
		return err
	}
	return ValidateIdentifierV1("capability", k.Capability)
}

func (k FactoryKeyV1) CanonicalIDV1() (string, error) {
	if err := k.Validate(); err != nil {
		return "", err
	}
	digest, err := DigestJSONV1(k)
	if err != nil {
		return "", err
	}
	return k.ComponentID + "@" + string(digest), nil
}

type ComponentNodeV1 struct {
	NodeID       string       `json:"node_id"`
	Factory      FactoryKeyV1 `json:"factory"`
	Dependencies []string     `json:"dependencies"`
}

func (n ComponentNodeV1) Validate() error {
	if err := ValidateIdentifierV1("node id", n.NodeID); err != nil {
		return err
	}
	if err := n.Factory.Validate(); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, dependency := range n.Dependencies {
		if err := ValidateIdentifierV1("dependency", dependency); err != nil {
			return err
		}
		if dependency == n.NodeID {
			return NewError(ErrorPrecondition, "dependency_cycle", "component cannot depend on itself")
		}
		if _, ok := seen[dependency]; ok {
			return NewError(ErrorConflict, "duplicate_dependency", "component dependency is duplicated")
		}
		seen[dependency] = struct{}{}
	}
	return nil
}

type ConstructionGraphV1 struct {
	GraphRef ExactRefV1        `json:"graph_ref"`
	Nodes    []ComponentNodeV1 `json:"nodes"`
}

func (g ConstructionGraphV1) Validate() error {
	if err := g.GraphRef.Validate(); err != nil {
		return err
	}
	if len(g.Nodes) == 0 {
		return NewError(ErrorInvalidArgument, "empty_graph", "construction graph has no nodes")
	}
	nodes := make(map[string]ComponentNodeV1, len(g.Nodes))
	factories := make(map[string]string, len(g.Nodes))
	for _, node := range g.Nodes {
		if err := node.Validate(); err != nil {
			return err
		}
		if _, ok := nodes[node.NodeID]; ok {
			return NewError(ErrorConflict, "duplicate_node", "construction graph contains duplicate node id")
		}
		nodes[node.NodeID] = node
		factoryID, err := node.Factory.CanonicalIDV1()
		if err != nil {
			return err
		}
		if previous, ok := factories[factoryID]; ok {
			return NewError(ErrorConflict, "factory_alias", "construction graph binds one exact factory to multiple nodes: "+previous)
		}
		factories[factoryID] = node.NodeID
	}
	for _, node := range g.Nodes {
		for _, dependency := range node.Dependencies {
			if _, ok := nodes[dependency]; !ok {
				return NewError(ErrorPrecondition, "dependency_missing", "construction graph dependency is missing")
			}
		}
	}
	_, err := g.DependencyOrderV1()
	return err
}

func (g ConstructionGraphV1) DependencyOrderV1() ([]string, error) {
	nodes := make(map[string]ComponentNodeV1, len(g.Nodes))
	indegree := make(map[string]int, len(g.Nodes))
	children := make(map[string][]string, len(g.Nodes))
	for _, node := range g.Nodes {
		nodes[node.NodeID] = node
		indegree[node.NodeID] = len(node.Dependencies)
	}
	for _, node := range g.Nodes {
		for _, dependency := range node.Dependencies {
			children[dependency] = append(children[dependency], node.NodeID)
		}
	}
	ready := make([]string, 0, len(g.Nodes))
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	order := make([]string, 0, len(g.Nodes))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		sort.Strings(children[id])
		for _, child := range children[id] {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
				sort.Strings(ready)
			}
		}
	}
	if len(order) != len(nodes) {
		return nil, NewError(ErrorPrecondition, "dependency_cycle", "construction graph contains a dependency cycle")
	}
	return order, nil
}

func (g ConstructionGraphV1) NodeV1(id string) (ComponentNodeV1, bool) {
	for _, node := range g.Nodes {
		if node.NodeID == id {
			node.Dependencies = append([]string(nil), node.Dependencies...)
			return node, true
		}
	}
	return ComponentNodeV1{}, false
}

type DecodedDefinitionV1 struct {
	Ref ExactRefV1 `json:"ref"`
}
type ResolvedAssemblyV1 struct {
	PlanRef  ExactRefV1 `json:"plan_ref"`
	InputRef ExactRefV1 `json:"input_ref"`
}
type CompiledAssemblyV1 struct {
	GenerationRef ExactRefV1          `json:"generation_ref"`
	ManifestRef   ExactRefV1          `json:"manifest_ref"`
	Graph         ConstructionGraphV1 `json:"graph"`
	HandoffRef    ExactRefV1          `json:"handoff_ref"`
}

const CompiledAssemblyArtifactsContractVersionV2 = "praxis.agent-host/compiled-assembly-artifacts/v2"

// CompiledAssemblyArtifactsV2 is the additive H3 output consumed by H4. It
// retains the four Harness-owned public sealed objects from the same compile;
// it never reconstructs them from Host refs.
type CompiledAssemblyArtifactsV2 struct {
	ContractVersion string                           `json:"contract_version"`
	ScopeRef        string                           `json:"scope_ref"`
	InputRef        ExactRefV1                       `json:"input_ref"`
	Compiled        CompiledAssemblyV1               `json:"compiled"`
	Harness         assemblycontract.CompileResultV1 `json:"harness"`
	CheckedUnixNano int64                            `json:"checked_unix_nano"`
	ExpiresUnixNano int64                            `json:"expires_unix_nano"`
	Digest          DigestV1                         `json:"digest"`
}

func (v CompiledAssemblyArtifactsV2) digestV2() (DigestV1, error) {
	clone := v
	clone.Digest = ""
	return DigestJSONV1(struct {
		Domain string                      `json:"domain"`
		Type   string                      `json:"type"`
		Body   CompiledAssemblyArtifactsV2 `json:"body"`
	}{Domain: "praxis.agent-host.compiled-assembly-artifacts", Type: "CompiledAssemblyArtifactsV2", Body: clone})
}

func SealCompiledAssemblyArtifactsV2(v CompiledAssemblyArtifactsV2) (CompiledAssemblyArtifactsV2, error) {
	v.ContractVersion = CompiledAssemblyArtifactsContractVersionV2
	provided := v.Digest
	v.Digest = ""
	digest, err := v.digestV2()
	if err != nil {
		return CompiledAssemblyArtifactsV2{}, err
	}
	if provided != "" && provided != digest {
		return CompiledAssemblyArtifactsV2{}, NewError(ErrorConflict, "compiled_artifacts_digest_drift", "compiled Assembly artifacts supplied a wrong digest")
	}
	v.Digest = digest
	return v, v.ValidateAt(time.Unix(0, v.CheckedUnixNano))
}

func (v CompiledAssemblyArtifactsV2) ValidateAt(now time.Time) error {
	if v.ContractVersion != CompiledAssemblyArtifactsContractVersionV2 || now.IsZero() || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "compiled_artifacts_incomplete", "compiled Assembly artifacts and current window are required")
	}
	if err := ValidateIdentifierV1("compiled Assembly scope", v.ScopeRef); err != nil {
		return err
	}
	if err := v.InputRef.Validate(); err != nil {
		return err
	}
	if now.UnixNano() < v.CheckedUnixNano {
		return NewError(ErrorPrecondition, "compiled_artifacts_clock_regression", "compiled Assembly artifacts clock regressed")
	}
	if now.UnixNano() >= v.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "compiled_artifacts_expired", "compiled Assembly artifacts expired")
	}
	if err := v.Compiled.Validate(); err != nil {
		return err
	}
	if v.Harness.Generation == nil || v.Harness.Manifest == nil || v.Harness.Graph == nil || v.Harness.Handoff == nil {
		return NewError(ErrorPrecondition, "compiled_artifacts_partial", "compiled Assembly artifacts require all four sealed Harness objects")
	}
	generation, manifest, graph, handoff := *v.Harness.Generation, *v.Harness.Manifest, *v.Harness.Graph, *v.Harness.Handoff
	if v.InputRef.Kind != "praxis.harness/assembly-input" || v.InputRef.Digest != DigestV1(generation.InputDigest) {
		return NewError(ErrorConflict, "compiled_input_splice", "compiled Harness objects drifted from the exact Assembly input")
	}
	if digest, err := assemblycontract.GenerationDigestV1(generation); err != nil || digest != generation.Digest {
		return NewError(ErrorConflict, "compiled_generation_drift", "compiled Generation digest drifted")
	}
	if digest, err := assemblycontract.ManifestDigestV1(manifest); err != nil || digest != manifest.Digest {
		return NewError(ErrorConflict, "compiled_manifest_drift", "compiled Manifest digest drifted")
	}
	if digest, err := assemblycontract.GraphDigestV1(graph); err != nil || digest != graph.Digest {
		return NewError(ErrorConflict, "compiled_graph_drift", "compiled Graph digest drifted")
	}
	if err := handoff.Validate(); err != nil {
		return err
	}
	if generation.State != assemblycontract.AssemblyStateSealedV1 || generation.InputDigest != manifest.InputDigest || generation.InputDigest != graph.InputDigest || generation.ManifestDigest != manifest.Digest || generation.GraphDigest != graph.Digest || handoff.GenerationRef != (assemblycontract.ObjectRefV1{ID: generation.GenerationID, Revision: generation.Revision, Digest: generation.Digest}) || handoff.ManifestDigest != manifest.Digest || handoff.GraphDigest != graph.Digest {
		return NewError(ErrorConflict, "compiled_artifact_splice", "compiled Harness artifact chain drifted")
	}
	wantGeneration := ExactRefV1{Kind: "praxis.harness/assembly-generation", ID: generation.GenerationID, Revision: uint64(generation.Revision), Digest: DigestV1(generation.Digest)}
	wantManifest := ExactRefV1{Kind: "praxis.harness/assembly-manifest", ID: generation.GenerationID + "/manifest", Revision: uint64(generation.Revision), Digest: DigestV1(manifest.Digest)}
	wantGraph := ExactRefV1{Kind: "praxis.harness/compiled-graph", ID: generation.GenerationID + "/graph", Revision: uint64(generation.Revision), Digest: DigestV1(graph.Digest)}
	wantHandoff := ExactRefV1{Kind: "praxis.harness/assembly-handoff", ID: generation.GenerationID + "/handoff", Revision: uint64(generation.Revision), Digest: DigestV1(handoff.Digest)}
	if v.Compiled.GenerationRef != wantGeneration || v.Compiled.ManifestRef != wantManifest || v.Compiled.Graph.GraphRef != wantGraph || v.Compiled.HandoffRef != wantHandoff {
		return NewError(ErrorConflict, "compiled_host_ref_splice", "Host compiled refs drifted from the same Harness compile")
	}
	expected, err := v.digestV2()
	if err != nil || expected != v.Digest {
		return NewError(ErrorConflict, "compiled_artifacts_digest_drift", "compiled Assembly artifacts digest drifted")
	}
	return nil
}

func (v DecodedDefinitionV1) Validate() error { return v.Ref.Validate() }
func (v ResolvedAssemblyV1) Validate() error {
	if err := v.PlanRef.Validate(); err != nil {
		return err
	}
	return v.InputRef.Validate()
}
func (v CompiledAssemblyV1) Validate() error {
	for _, ref := range []ExactRefV1{v.GenerationRef, v.ManifestRef, v.HandoffRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return v.Graph.Validate()
}

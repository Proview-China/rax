package assembly_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblysdk"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestPreBindingArtifactsContainNoRuntimeBindingOrExecutableFactory(t *testing.T) {
	t.Parallel()
	result, err := assemblysdk.New().Compile(assemblytestkit.ValidInput())
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{"manifest": result.Manifest, "graph": result.Graph} {
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		text := string(payload)
		for _, forbidden := range []string{"binding_set_id", "actual_injection", "settlement_disposition", "domain_result_payload", "function", "handler_pointer"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s leaked forbidden post-binding/runtime field %q", name, forbidden)
			}
		}
	}
	if result.Handoff.RequiredExtension != "praxis.harness/assembly-generation" {
		t.Fatalf("unexpected required extension %q", result.Handoff.RequiredExtension)
	}
}

func TestBindingConformanceDoesNotMutatePreBindingDigests(t *testing.T) {
	t.Parallel()
	result, err := assemblysdk.New().Compile(assemblytestkit.ValidInput())
	if err != nil {
		t.Fatal(err)
	}
	now := assemblytestkit.Now.UnixNano()
	conformance, err := assemblycontract.SealBindingConformanceV1(assemblycontract.AssemblyBindingConformanceV1{HandoffRef: assemblycontract.ObjectRefV1{ID: "handoff", Revision: 1, Digest: result.Handoff.Digest}, GenerationRef: result.Handoff.GenerationRef, ManifestDigest: result.Manifest.Digest, GraphDigest: result.Graph.Digest, Binding: assemblytestkit.RuntimeBindingRef(), BindingSetDigest: assemblytestkit.Digest("binding-set"), BindingSetSemanticDigest: assemblytestkit.Digest("binding-set-semantic"), CapabilityDigest: assemblytestkit.Digest("capability"), SchemaDigests: []core.Digest{assemblytestkit.Digest("schema")}, ObservedUnixNano: now, ExpiresUnixNano: now + 1_000_000, Current: true}, now+1)
	if err != nil {
		t.Fatal(err)
	}
	if conformance.ManifestDigest != result.Manifest.Digest || conformance.GraphDigest != result.Graph.Digest {
		t.Fatal("post-binding conformance rewrote pre-binding digests")
	}
}

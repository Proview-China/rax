package ports_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRuntimeSharedEngineComponentIDV1IsPublicCanonicalAndAliasSensitive(t *testing.T) {
	canonical := runtimeports.RuntimeSharedEngineComponentIDV1
	if canonical != runtimeports.ComponentIDV2("components/runtime") {
		t.Fatalf("Runtime shared-engine component identity drifted: %q", canonical)
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(canonical)); err != nil {
		t.Fatalf("Runtime shared-engine component identity is invalid: %v", err)
	}
	type payload struct {
		ComponentID runtimeports.ComponentIDV2 `json:"component_id"`
	}
	want, err := core.CanonicalJSONDigest("praxis.runtime.component-identity", "v1", "RuntimeSharedEngineComponentIdentityV1", payload{ComponentID: canonical})
	if err != nil {
		t.Fatal(err)
	}
	alias, err := core.CanonicalJSONDigest("praxis.runtime.component-identity", "v1", "RuntimeSharedEngineComponentIdentityV1", payload{ComponentID: "praxis/runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if want == alias {
		t.Fatal("Runtime component alias preserved the canonical identity digest")
	}
}

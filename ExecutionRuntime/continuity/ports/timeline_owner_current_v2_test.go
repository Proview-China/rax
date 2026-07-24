package ports_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func TestTimelineOwnerCurrentInspectRequestV2RequiresTenantAndExactFact(t *testing.T) {
	request := ports.TimelineOwnerCurrentInspectRequestV2{
		TenantID: "tenant-a",
		Fact: contract.TimelineOwnerFactRefV1{
			Owner: contract.OwnerBinding{
				BindingSetID: "binding-a", BindingRevision: 1, ComponentID: "component-a",
				ManifestDigest: "manifest-a", ArtifactDigest: "artifact-a",
				Capability: "custom/current", FactKind: "custom/fact",
			},
			FactKind: "custom/fact", FactID: "fact-a", Revision: 1,
			FactDigest: "fact-digest", PayloadSchema: "custom/fact@1.0.0",
			PayloadDigest: "payload-digest", PayloadRevision: 1, ScopeDigest: "scope-digest",
		},
	}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.TenantID = ""
	if err := request.Validate(); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("tenantless exact request must fail closed: %v", err)
	}
	request.TenantID = "tenant-a"
	request.Fact.Owner.FactKind = "custom/other-fact"
	if err := request.Validate(); !contract.HasCode(err, contract.ErrProjectionConflict) {
		t.Fatalf("Owner/fact kind splice must fail closed: %v", err)
	}
}

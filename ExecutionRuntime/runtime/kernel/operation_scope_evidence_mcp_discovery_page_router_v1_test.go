package kernel

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type mcpDiscoveryPageApplicabilityReaderV1 struct {
	value ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3
}

func (r *mcpDiscoveryPageApplicabilityReaderV1) InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Context, ports.OperationScopeEvidenceApplicabilityFactRefV3) (ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3, error) {
	return r.value, nil
}

func TestOperationScopeEvidenceMCPDiscoveryPageRouterV1ExactTwoReaders(t *testing.T) {
	now := time.Unix(1_750_000_000, 0).UTC()
	scope := core.DigestBytes([]byte("scope"))
	bindings := make([]OperationScopeEvidenceMCPDiscoveryPageRouteBindingV1, 0, 2)
	for _, route := range ports.OperationScopeEvidenceMCPDiscoveryPageRoutesV1() {
		fact := ports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: route.Kind, ID: "mcp-discovery-page-" + string(route.Dimension), Revision: 1, Digest: core.DigestBytes([]byte(route.Kind))}
		projection := ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{Fact: fact, ExecutionScopeDigest: scope, Current: true, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
		copy := projection
		copy.Digest = ""
		projection.Digest, _ = core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", ports.OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceApplicabilityCurrentProjectionV3", copy)
		bindings = append(bindings, OperationScopeEvidenceMCPDiscoveryPageRouteBindingV1{Route: route, Reader: &mcpDiscoveryPageApplicabilityReaderV1{value: projection}})
	}
	router, err := NewOperationScopeEvidenceMCPDiscoveryPageRouterV1(bindings, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	for index, binding := range bindings {
		reader := binding.Reader.(*mcpDiscoveryPageApplicabilityReaderV1)
		if _, err := router.InspectOperationScopeEvidenceMCPDiscoveryPageApplicabilityCurrentV1(context.Background(), binding.Route.Dimension, reader.value.Fact, scope); err != nil {
			t.Fatalf("reader %d failed: %v", index, err)
		}
	}
	wrong := bindings[0].Reader.(*mcpDiscoveryPageApplicabilityReaderV1).value.Fact
	wrong.Kind = ports.OperationScopeEvidenceSessionCurrentKindV3
	if _, err := router.InspectOperationScopeEvidenceMCPDiscoveryPageApplicabilityCurrentV1(context.Background(), ports.OperationScopeEvidenceRunV3, wrong, scope); err == nil {
		t.Fatal("type-punned MCP Discovery Page applicability fact was admitted")
	}
}

func TestOperationScopeEvidenceMCPDiscoveryPageRouterV1TypedNilAndClock(t *testing.T) {
	routes := ports.OperationScopeEvidenceMCPDiscoveryPageRoutesV1()
	var typedNil *mcpDiscoveryPageApplicabilityReaderV1
	if _, err := NewOperationScopeEvidenceMCPDiscoveryPageRouterV1([]OperationScopeEvidenceMCPDiscoveryPageRouteBindingV1{{Route: routes[0], Reader: typedNil}, {Route: routes[1], Reader: &mcpDiscoveryPageApplicabilityReaderV1{}}}, time.Now); err == nil {
		t.Fatal("typed-nil MCP Discovery Page applicability Reader was admitted")
	}
	if _, err := NewOperationScopeEvidenceMCPDiscoveryPageRouterV1(nil, nil); err == nil {
		t.Fatal("nil MCP Discovery Page Router clock was admitted")
	}
}

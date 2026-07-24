package blackbox_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestBlackboxP42StableIssuanceAndN1HistoricalLineage(t *testing.T) {
	model := testkit.ModelProjection(1)
	historical, err := toolcontract.SealModelSourceCandidateHistoricalRefV1(model)
	if err != nil {
		t.Fatal(err)
	}
	if historical.CallOrdinal != 0 || historical.CanonicalArgumentsDigest != core.DigestBytes(model.Observation.Calls[0].CanonicalArguments) {
		t.Fatal("historical Model lineage did not bind the unique canonical call")
	}
	if _, err = toolcontract.SealModelSourceCandidateHistoricalRefV1(testkit.ModelProjection(2)); err == nil {
		t.Fatal("N>1 Model observation passed the single-call contract")
	}

	capability, tool := testkit.Capability(), testkit.Tool()
	surface := testkit.ToolSurfaceManifestV1(1)
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-v2", BindingSetRevision: 1, ComponentID: testkit.SettlementOwner().ComponentID, ManifestDigest: testkit.SettlementOwner().ManifestDigest, ArtifactDigest: tool.ArtifactDigest, Capability: runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3)}
	request := toolcontract.ToolInputContractResolveRequestV1{
		ApplicationRequestID: "application-request-v2", ApplicationRequestRevision: 1, ApplicationRequestDigest: testkit.Digest("application-request-v2"),
		PendingAction:        toolcontract.PendingActionExactRefV2{ID: "pending-action-v2", Revision: 1, RequestDigest: testkit.Digest("pending-action-v2")},
		OperationScopeDigest: testkit.Digest("scope-v2"), ProviderBinding: provider, ExpectedOwner: testkit.SettlementOwner(),
		Surface: toolcontract.ObjectRef{ID: surface.ID, Revision: surface.Revision, Digest: surface.Digest}, CallName: surface.Entries[0].ModelName,
		Capability: toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest},
		Tool:       toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}, InputSchema: tool.InputSchema,
		RequestedExpiresUnixNano: testkit.FixedTime.Add(5).UnixNano(),
	}
	first, err := toolcontract.ToolInputContractIssuanceFromResolveRequestV1(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := toolcontract.ToolInputContractIssuanceFromResolveRequestV1(request)
	if err != nil || first != second {
		t.Fatalf("same stable request did not derive the same issuance: %v", err)
	}
	firstID, err := toolcontract.DeriveToolInputContractCurrentIDV1(first)
	if err != nil {
		t.Fatal(err)
	}
	request.RequestedExpiresUnixNano++
	changed, err := toolcontract.ToolInputContractIssuanceFromResolveRequestV1(request)
	if err != nil {
		t.Fatal(err)
	}
	changedID, err := toolcontract.DeriveToolInputContractCurrentIDV1(changed)
	if err != nil || changedID == firstID {
		t.Fatalf("requested bound did not change issuance identity: %v", err)
	}
}

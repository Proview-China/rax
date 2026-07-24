package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCommittedPendingActionOwnerInputsSealExactKindMatrixAndClone(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	prepare, _, _, _ := testkit.GovernedProviderFixtureV2(now)
	matrix := runtimeports.OperationScopeEvidenceActionMatrixV3()
	matrixDigest, err := runtimeports.DigestOperationScopeEvidenceApplicabilityMatrixKeyV3(matrix)
	if err != nil {
		t.Fatal(err)
	}
	declaration := runtimeports.ControlledOperationProviderRouteDeclarationRefV2{RouteID: "route-owner-inputs", Revision: 1, PublisherComponentID: "praxis.harness/assembly", DeclarationDigest: testkit.Digest("route-declaration")}
	conformance := runtimeports.ControlledOperationProviderRouteConformanceRefV2{ConformanceID: "route-conformance", Revision: 1, DeclarationRef: declaration, ConformanceDigest: testkit.Digest("route-conformance")}
	currentID, err := runtimeports.DeriveControlledOperationProviderRouteCurrentIDV2(declaration.RouteID, matrixDigest)
	if err != nil {
		t.Fatal(err)
	}
	route, err := runtimeports.SealControlledOperationProviderRouteCurrentRefV2(runtimeports.ControlledOperationProviderRouteCurrentRefV2{CurrentID: currentID, Revision: 1, DeclarationRef: declaration, ConformanceRef: conformance, MatrixDigest: matrixDigest, Watermark: testkit.Digest("route-watermark")})
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := contract.SealCommittedPendingActionOwnerCurrentInputsV1(contract.CommittedPendingActionOwnerCurrentInputsV1{ModelTurnOperation: prepare.Intent.Operation, GenerationBindingAssociation: runtimeports.GenerationBindingAssociationRefV1{ID: "association-owner-inputs", Revision: 1, Digest: testkit.Digest("association")}, RouteCurrent: route, RouteMatrix: matrix, ContextApplicability: runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: runtimeports.OperationScopeEvidenceContextParentKindV3, ID: "context-parent", Revision: 1, Digest: testkit.Digest("context-parent")}})
	if err != nil {
		t.Fatal(err)
	}
	clone := inputs.Clone()
	clone.ModelTurnOperation.ExecutionScope.SandboxLease.ID = "mutated"
	if inputs.ModelTurnOperation.ExecutionScope.SandboxLease.ID == "mutated" {
		t.Fatal("owner inputs clone aliased SandboxLease")
	}
	wrong := inputs.Clone()
	wrong.ContextApplicability.Kind = runtimeports.NamespacedNameV2("praxis.harness/session-current-v1")
	wrong.Digest = ""
	if _, err := contract.SealCommittedPendingActionOwnerCurrentInputsV1(wrong); err == nil {
		t.Fatal("wrong Context applicability Kind was accepted")
	}
	wrong = inputs.Clone()
	wrong.RouteMatrix.PolicyProfile = "praxis.custom/wrong"
	wrong.Digest = ""
	if _, err := contract.SealCommittedPendingActionOwnerCurrentInputsV1(wrong); err == nil {
		t.Fatal("wrong route matrix was accepted")
	}

	steps := governedSessionV3Steps(t, now)
	base := *steps[len(steps)-1].ApplicationBinding
	operationDigest, err := inputs.ModelTurnOperation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	base.ModelTurnSettlementRef.Attempt.OperationDigest = operationDigest
	binding, err := contract.SealPendingActionApplicationBindingV2(contract.PendingActionApplicationBindingV2{Base: base, OwnerCurrentInputs: inputs})
	if err != nil {
		t.Fatal(err)
	}
	copy := binding.Clone()
	copy.Base.PendingAction.Payload.Inline[0] ^= 1
	copy.OwnerCurrentInputs.ModelTurnOperation.ExecutionScope.SandboxLease.ID = "changed"
	if string(binding.Base.PendingAction.Payload.Inline) != `{"input":"governed"}` || binding.OwnerCurrentInputs.ModelTurnOperation.ExecutionScope.SandboxLease.ID == "changed" {
		t.Fatal("Binding V2 clone retained aliases")
	}
	spliced := binding.Clone()
	spliced.OwnerCurrentInputs.ModelTurnOperation.CurrentProjectionDigest = testkit.Digest("other-operation-current")
	spliced.OwnerCurrentInputs, err = contract.SealCommittedPendingActionOwnerCurrentInputsV1(spliced.OwnerCurrentInputs)
	if err != nil {
		t.Fatal(err)
	}
	spliced.Digest = ""
	if _, err := contract.SealPendingActionApplicationBindingV2(spliced); err == nil {
		t.Fatal("valid operation and Settlement attempt splice was accepted")
	}
}

func TestCommittedPendingActionRequestV3TimeRules(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	var request contract.CommittedPendingActionCurrentRequestV3
	if err := request.Validate(time.Time{}); err == nil {
		t.Fatal("zero validation time accepted")
	}
	request.RequestedNotAfterUnixNano = -1
	if err := request.Validate(now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("negative bound error=%v", err)
	}
}

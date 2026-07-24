package assemblycontract

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestControlledOperationProviderRouteSealsRejectWrongNonzeroDigestsV2(t *testing.T) {
	t.Parallel()
	wrong := core.DigestBytes([]byte("wrong"))
	declaration := ControlledOperationProviderRouteDeclarationV2{DeclarationDigest: wrong}
	if _, err := SealControlledOperationProviderRouteDeclarationV2(declaration); !core.HasCategory(err, core.ErrorPreconditionFailed) || !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("declaration got %v", err)
	}
	inventory := ControlledOperationProviderRouteWiringInventoryV2{Digest: wrong}
	if _, err := SealControlledOperationProviderRouteWiringInventoryV2(inventory); !core.HasCategory(err, core.ErrorPreconditionFailed) || !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("inventory got %v", err)
	}
	conformance := ControlledOperationProviderRouteConformanceV2{CheckedUnixNano: time.Now().UnixNano(), ConformanceDigest: wrong}
	if _, err := SealControlledOperationProviderRouteConformanceV2(conformance); !core.HasCategory(err, core.ErrorPreconditionFailed) || !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("conformance got %v", err)
	}
}

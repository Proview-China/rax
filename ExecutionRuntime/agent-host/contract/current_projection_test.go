package contract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

func TestResolutionInputsCurrentSealValidateAndTTL(t *testing.T) {
	now := int64(100)
	value, err := contract.SealResolutionInputsCurrentV1(contract.ResolutionInputsCurrentV1{
		ContractVersion: contract.ContractVersionV1, ObjectKind: contract.ResolutionInputsCurrentKindV1,
		CatalogStableID: "catalog-current", ResolutionFactsStableID: "facts-current", Revision: 1,
		CatalogExactRef:         exactRef("praxis.agent-assembler/component-release-catalog", "catalog-exact"),
		ResolutionFactsExactRef: exactRef("praxis.agent-assembler/resolution-facts", "facts-exact"),
		CheckedUnixNano:         now, ExpiresUnixNano: now + 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Validate(now + 9); err != nil {
		t.Fatal(err)
	}
	if err := value.Validate(now + 10); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("expiry error=%v", err)
	}
	drift := value
	drift.Revision = 2
	if err := drift.Validate(now + 1); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("digest drift error=%v", err)
	}
}

func TestCurrentProjectionSealersRejectShapeSplice(t *testing.T) {
	_, err := contract.SealResolutionInputsCurrentV1(contract.ResolutionInputsCurrentV1{ContractVersion: contract.ContractVersionV1, ObjectKind: contract.DefinitionSourceCurrentKindV1})
	if !contract.HasCode(err, contract.ErrorInvalidArgument) {
		t.Fatalf("resolution shape error=%v", err)
	}
	_, err = contract.SealDefinitionSourceCurrentV1(contract.DefinitionSourceCurrentV1{ContractVersion: contract.ContractVersionV1, ObjectKind: contract.ResolutionInputsCurrentKindV1})
	if !contract.HasCode(err, contract.ErrorInvalidArgument) {
		t.Fatalf("definition shape error=%v", err)
	}
}

func exactRef(kind, id string) contract.ExactRefV1 {
	return contract.ExactRefV1{Kind: kind, ID: id, Revision: 1, Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
}

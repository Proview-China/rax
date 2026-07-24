package ports_test

import (
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationDispatchV5IsNominalAndDoesNotEmbedLegacyReview(t *testing.T) {
	if ports.OperationDispatchGovernanceContractVersionV5 == ports.OperationDispatchGovernanceContractVersionV4 {
		t.Fatal("V5 governance reused the V4 contract identity")
	}
	permit := reflect.TypeOf(ports.OperationDispatchPermitV5{})
	for _, forbidden := range []string{"LegacyPermit", "LegacyReview", "Verdict", "ReviewRequired"} {
		if _, exists := permit.FieldByName(forbidden); exists {
			t.Fatalf("V5 Permit exposes legacy/type-punned field %q", forbidden)
		}
	}
	for _, required := range []string{"Authorization", "AuthorizationBasis", "Admission", "FenceDigest", "Budget", "Authority", "Review", "Policy"} {
		if _, exists := permit.FieldByName(required); !exists {
			t.Fatalf("V5 Permit omitted exact governance field %q", required)
		}
	}
}

func TestOperationDispatchV5ThreeBasisConstantsAreDistinct(t *testing.T) {
	values := map[ports.OperationReviewAuthorizationBasisV5]struct{}{
		ports.OperationReviewBasisAcceptedQuorumV5:             {},
		ports.OperationReviewBasisConditionalQuorumSatisfiedV5: {},
		ports.OperationReviewBasisPolicyNotRequiredV5:          {},
	}
	if len(values) != 3 {
		t.Fatalf("V5 authorization bases are not nominally distinct: %#v", values)
	}
	for basis := range values {
		if basis == "" {
			t.Fatal("V5 authorization basis is empty")
		}
	}
}

func TestOperationDispatchEnforcementV5CarriesExactV5AuthorizationBranch(t *testing.T) {
	request := reflect.TypeOf(ports.EnforceCurrentOperationDispatchRequestV5{})
	authorization, ok := request.FieldByName("ReviewAuthorization")
	if !ok || authorization.Type != reflect.TypeOf(ports.OperationReviewAuthorizationRefV5{}) {
		t.Fatalf("V5 enforcement does not carry the exact V5 Authorization ref: %#v", authorization)
	}
	basis, ok := request.FieldByName("AuthorizationBasis")
	if !ok || basis.Type != reflect.TypeOf(ports.OperationReviewAuthorizationBasisV5("")) {
		t.Fatalf("V5 enforcement does not carry the exact V5 basis: %#v", basis)
	}
	if _, exists := request.FieldByName("ReviewAuthorizationV4"); exists {
		t.Fatal("V5 enforcement exposes a V4 Review authorization escape hatch")
	}
}

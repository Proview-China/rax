package contract

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestNextModelTurnDispatchPublicSurfaceIsEligibilityOnlyV1(t *testing.T) {
	requestType := reflect.TypeOf(NextModelTurnDispatchRequestV1{})
	expected := []string{
		"contract_version",
		"eligibility_request",
		"eligibility_projection",
		"requested_not_after_unix_nano",
		"request_digest",
	}
	if requestType.NumField() != len(expected) {
		t.Fatalf("dispatch request fields=%d want=%d", requestType.NumField(), len(expected))
	}
	for index, want := range expected {
		field := requestType.Field(index)
		if got := strings.Split(field.Tag.Get("json"), ",")[0]; got != want {
			t.Fatalf("dispatch request field %d=%q want=%q", index, got, want)
		}
	}
	for _, typ := range []reflect.Type{
		requestType,
		reflect.TypeOf(NextModelTurnDispatchCurrentV1{}),
		reflect.TypeOf(NextModelTurnDispatchInspectRequestV1{}),
	} {
		for index := 0; index < typ.NumField(); index++ {
			surface := strings.ToLower(
				typ.Field(index).Name + " " +
					typ.Field(index).Type.String() + " " +
					typ.Field(index).Tag.Get("json"),
			)
			for _, forbidden := range []string{
				"prepared", "currentref", "invocationmaterial", "sourcelineage",
				"routecall", "tooldigest", "providerordinal", "harnessenvelope",
				"modelattempt", "harnessdispatch", "outcome",
			} {
				if strings.Contains(surface, forbidden) {
					t.Fatalf("%s exposes forbidden %s surface at %s", typ.Name(), forbidden, typ.Field(index).Name)
				}
			}
		}
	}
}

func TestNextModelTurnDispatchCurrentStrictCanonicalJSONV1(t *testing.T) {
	digest := func(seed string) core.Digest { return core.DigestBytes([]byte(seed)) }
	current := NextModelTurnDispatchCurrentV1{
		ContractVersion: NextModelTurnDispatchContractVersionV1,
		DerivedDispatch: NextModelTurnDerivedDispatchRefV1{
			ContractVersion: NextModelTurnEligibilityContractVersionV1,
			ID:              "derived-dispatch", Revision: 1, RequestDigest: digest("eligibility"),
			ContinuationAttempt: TurnContinuationAttemptRefV1{
				ID: "attempt", Revision: 1, Digest: digest("attempt"),
				ExecutionScopeDigest: digest("scope"), RunID: "run", SessionID: "session",
				SourceTurn: 1, TargetTurn: 2,
			},
			ContinuationCurrentDigest:       digest("continuation"),
			ActiveContextDigest:             digest("context"),
			RuntimeActualPointRequestDigest: digest("actual-point"),
			Digest:                          digest("derived"),
		},
		Revision: 1, State: NextModelTurnDispatchAttemptBoundV1,
		RequestDigest:   digest("request"),
		CheckedUnixNano: 10, NotAfterUnixNano: 20,
	}
	var err error
	current.Digest, err = current.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := EncodeNextModelTurnDispatchCurrentV1(current)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeNextModelTurnDispatchCurrentV1(payload)
	if err != nil || decoded != current {
		t.Fatalf("canonical round trip failed: decoded=%+v err=%v", decoded, err)
	}
	withUnknown := append(payload[:len(payload)-1], []byte(`,"unknown":true}`)...)
	if _, err = DecodeNextModelTurnDispatchCurrentV1(withUnknown); !core.HasReason(err, core.ReasonInvalidCanonicalForm) {
		t.Fatalf("unknown field was accepted: %v", err)
	}
	reduced := []byte(`{"revision":1,"contract_version":"praxis.application/next-model-turn-dispatch/v1"}`)
	if _, err = DecodeNextModelTurnDispatchCurrentV1(reduced); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("reduced/noncanonical JSON was accepted: %v", err)
	}
}

func TestNextModelTurnDispatchAttemptBoundRemainsNonProductionV1(t *testing.T) {
	if NextModelTurnDispatchAttemptBoundV1 != "attempt_bound" {
		t.Fatalf("unexpected front-slice state: %q", NextModelTurnDispatchAttemptBoundV1)
	}
}

func TestNextModelTurnDispatchInspectRejectsStructurallyForgedDerivedRefV1(t *testing.T) {
	digest := func(seed string) core.Digest { return core.DigestBytes([]byte(seed)) }
	requestDigest := digest("eligibility-request")
	id, err := DeriveNextModelTurnDispatchIDFromRequestDigestV1(requestDigest)
	if err != nil {
		t.Fatal(err)
	}
	ref := NextModelTurnDerivedDispatchRefV1{
		ContractVersion: NextModelTurnEligibilityContractVersionV1,
		ID:              id,
		Revision:        1,
		RequestDigest:   requestDigest,
		ContinuationAttempt: TurnContinuationAttemptRefV1{
			ID: "attempt", Revision: 1, Digest: digest("attempt"),
			ExecutionScopeDigest: digest("scope"), RunID: "run", SessionID: "session",
			SourceTurn: 1, TargetTurn: 2,
		},
		ContinuationCurrentDigest:       digest("continuation-current"),
		ActiveContextDigest:             digest("active-context"),
		RuntimeActualPointRequestDigest: digest("actual-point"),
	}
	ref.Digest, err = ref.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	sealInspect := func(ref NextModelTurnDerivedDispatchRefV1) NextModelTurnDispatchInspectRequestV1 {
		t.Helper()
		inspect := NextModelTurnDispatchInspectRequestV1{
			ContractVersion: NextModelTurnDispatchContractVersionV1,
			DerivedDispatch: ref,
			RequestDigest:   digest("dispatch-request"),
		}
		inspect.Digest, err = inspect.DigestV1()
		if err != nil {
			t.Fatal(err)
		}
		return inspect
	}
	if err = sealInspect(ref).Validate(); err != nil {
		t.Fatalf("valid structural derived ref was rejected: %v", err)
	}
	for _, mutate := range []func(*NextModelTurnDerivedDispatchRefV1){
		func(value *NextModelTurnDerivedDispatchRefV1) {
			value.ID = "attacker-selected-derived-id"
		},
		func(value *NextModelTurnDerivedDispatchRefV1) {
			value.ContinuationAttempt.SourceTurn = 0
		},
	} {
		forged := ref
		mutate(&forged)
		forged.Digest = ""
		forged.Digest, err = forged.DigestV1()
		if err != nil {
			t.Fatal(err)
		}
		if err = sealInspect(forged).Validate(); err == nil {
			t.Fatal("structurally forged derived ref was accepted after all digests were recomputed")
		}
	}
}

package contract_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestComponentSchemaRefV2RejectsMediaTypeAliases(t *testing.T) {
	base := contract.ComponentSchemaRefV2{
		Namespace: "praxis.component",
		Name:      "input",
		Version:   "v1",
		MediaType: "application/json",
		ContentDigest: contract.DigestV1(
			core.DigestBytes([]byte("component-schema")),
		),
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		mediaType string
	}{
		{"invalid_utf8", "application/" + string([]byte{0xff})},
		{"uppercase", "Application/JSON"},
		{"leading_space", " application/json"},
		{"trailing_space", "application/json "},
		{"control", "application/\x00json"},
		{"double_slash_alias", "application//json"},
		{"parameter_alias", "application/json;charset=utf-8"},
		{"non_ascii_alias", "application/jsön"},
		{"empty_type", "/json"},
		{"empty_subtype", "application/"},
		{"empty_type_and_subtype", "/"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			value.MediaType = test.mediaType
			if err := value.Validate(); !contract.HasCode(err, contract.ErrorInvalidArgument) {
				t.Fatalf("non-canonical media type %q accepted: %v", test.mediaType, err)
			}
		})
	}
}

func TestComponentStartRequestV2DerivesOneStableExactAttempt(t *testing.T) {
	start := componentStartFixtureV2(t)
	replayed, err := contract.SealComponentStartRequestV2(start)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(start, replayed) {
		t.Fatalf("restart replay changed exact Start Attempt: first=%+v replayed=%+v", start.Attempt, replayed.Attempt)
	}
	if err = start.Attempt.Validate(); err != nil {
		t.Fatalf("lost-reply Inspect rejected original exact Attempt: %v", err)
	}

	wrongDigest := start
	wrongDigest.Attempt.Digest = contract.DigestV1(core.DigestBytes([]byte("replacement-attempt-digest")))
	if err = wrongDigest.Validate(); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("Start accepted replacement exact Attempt digest: %v", err)
	}
	if _, err = contract.SealComponentStartRequestV2(wrongDigest); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("Seal accepted replacement exact Attempt digest: %v", err)
	}

	reusedID := start
	reusedID.RequestDigest = ""
	reusedID.Attempt.RequestDigest = ""
	reusedID.Attempt.Digest = ""
	reusedID.StartID = "start-component-v2-other"
	if _, err = contract.SealComponentStartRequestV2(reusedID); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("different payload reused the original AttemptID: %v", err)
	}
	reusedID.Attempt.AttemptID = ""
	other, err := contract.SealComponentStartRequestV2(reusedID)
	if err != nil {
		t.Fatal(err)
	}
	if other.Attempt.AttemptID == start.Attempt.AttemptID ||
		other.Attempt.Digest == start.Attempt.Digest ||
		other.RequestDigest == start.RequestDigest {
		t.Fatal("different Start payload reused an exact Attempt identity")
	}
	changedAttemptCoordinate := start
	changedAttemptCoordinate.RequestDigest = ""
	changedAttemptCoordinate.Attempt.AttemptID = ""
	changedAttemptCoordinate.Attempt.RequestDigest = ""
	changedAttemptCoordinate.Attempt.Digest = ""
	changedAttemptCoordinate.Attempt.ExpiresUnixNano++
	changedAttemptCoordinate, err = contract.SealComponentStartRequestV2(changedAttemptCoordinate)
	if err != nil {
		t.Fatal(err)
	}
	if changedAttemptCoordinate.RequestDigest == start.RequestDigest ||
		changedAttemptCoordinate.Attempt.AttemptID == start.Attempt.AttemptID ||
		changedAttemptCoordinate.Attempt.Digest == start.Attempt.Digest {
		t.Fatal("parent RequestDigest did not seal the complete Attempt identity")
	}

	callerSelected := start
	callerSelected.RequestDigest = ""
	callerSelected.Attempt.RequestDigest = ""
	callerSelected.Attempt.Digest = ""
	callerSelected.Attempt.AttemptID = "caller-selected-attempt"
	if _, err = contract.SealComponentStartRequestV2(callerSelected); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("caller-selected AttemptID was accepted: %v", err)
	}
}

func TestComponentFactoryAttemptRefV2LostReplyRequiresOriginalExactRef(t *testing.T) {
	start := componentStartFixtureV2(t)
	tests := []struct {
		name   string
		mutate func(*contract.ComponentFactoryAttemptRefV2)
	}{
		{"attempt_id", func(value *contract.ComponentFactoryAttemptRefV2) {
			value.AttemptID = "component-start/sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}},
		{"factory", func(value *contract.ComponentFactoryAttemptRefV2) {
			value.FactoryRef.FactoryID = "praxis.component/other-factory"
		}},
		{"request_digest", func(value *contract.ComponentFactoryAttemptRefV2) {
			value.RequestDigest = contract.DigestV1(core.DigestBytes([]byte("other-start-request")))
		}},
		{"expiry", func(value *contract.ComponentFactoryAttemptRefV2) {
			value.ExpiresUnixNano--
		}},
		{"digest", func(value *contract.ComponentFactoryAttemptRefV2) {
			value.Digest = contract.DigestV1(core.DigestBytes([]byte("other-attempt-ref")))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref := start.Attempt
			test.mutate(&ref)
			if err := ref.Validate(); !contract.HasCode(err, contract.ErrorConflict) {
				t.Fatalf("lost-reply Inspect accepted non-original %s coordinate: %v", test.name, err)
			}
		})
	}
}

func componentStartFixtureV2(t *testing.T) contract.ComponentStartRequestV2 {
	t.Helper()
	digest := func(value string) contract.DigestV1 {
		return contract.DigestV1(core.DigestBytes([]byte(value)))
	}
	schema := func(name string) contract.ComponentSchemaRefV2 {
		return contract.ComponentSchemaRefV2{
			Namespace: "praxis.component",
			Name:      name,
			Version:   "v1",
			MediaType: "application/json",
			ContentDigest: digest(
				"schema-" + name,
			),
		}
	}
	cleanup, err := contract.SealComponentCleanupContractV2(contract.ComponentCleanupContractV2{
		Ref: contract.ExactRefV1{
			Kind: "praxis.component/cleanup-contract", ID: "cleanup-component-v2",
			Revision: 1, Digest: digest("cleanup-ref"),
		},
		OwnerCapability: "praxis.component/cleanup",
		RequestSchema:   schema("cleanup-request"),
		ResultSchema:    schema("cleanup-result"),
	})
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := contract.SealComponentFactoryDescriptorV2(contract.ComponentFactoryDescriptorV2{
		Ref:              contract.ComponentFactoryRefV2{FactoryID: "praxis.component/factory", Revision: 1},
		ModuleRef:        "praxis.component/module",
		ArtifactDigest:   digest("artifact"),
		ConstructionMode: contract.ComponentFactoryConstructionTrustedGoV2,
		InputSchema:      schema("input"),
		OutputCapability: "praxis.component/output",
		Lifecycle:        "host",
		CleanupContract:  cleanup,
		TrustRef: contract.ExactRefV1{
			Kind: "praxis.component/trust", ID: "trust-component-v2",
			Revision: 1, Digest: digest("trust"),
		},
		Implementation: contract.ComponentFactoryImplementationOwnerV2,
		ProviderAccess: contract.ComponentFactoryProviderAccessNoneV2,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2_401_000_000, 0)
	expires := now.Add(time.Hour).UnixNano()
	value, err := contract.SealComponentStartRequestV2(contract.ComponentStartRequestV2{
		HostID:  "host-component-v2",
		StartID: "start-component-v2",
		Attempt: contract.ComponentFactoryAttemptRefV2{
			Revision: 1, FactoryRef: descriptor.Ref, ExpiresUnixNano: expires,
		},
		Preflight: contract.ComponentFactoryPreflightReceiptRefV2{
			AttemptID:       "preflight-component-v2",
			Revision:        1,
			Digest:          digest("preflight"),
			ExpiresUnixNano: expires,
		},
		Descriptor:                descriptor,
		ResourceRefs:              []contract.ComponentResourceRefV2{},
		DependencyRefs:            []contract.ComponentInstanceRefV2{},
		RequestedUnixNano:         now.UnixNano(),
		RequestedNotAfterUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

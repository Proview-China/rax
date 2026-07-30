package contract_test

import (
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func modelInputRefV1(id string) contract.FactRef {
	return contract.FactRef{ID: id, Revision: 1, Digest: contract.DigestBytes([]byte(id))}
}

func TestModelInputSemanticBindingRequiresExplicitToolIdentityV1(t *testing.T) {
	base := contract.ContextModelInputSegmentBindingV1{
		FragmentRef: modelInputRefV1("tool-result"), Region: contract.RegionDynamicTail, Position: 1,
		Kind: contract.FragmentToolResult, Trust: contract.TrustObservation, Channel: contract.ContextModelInputFunctionResultV1,
		Role: contract.ContextModelInputRoleToolV1, Encoding: contract.ContextModelInputUTF8V1,
		CallID: "call-1", Name: "workspace.read",
	}
	sealed, err := contract.SealContextModelInputSegmentBindingV1(base)
	if err != nil || sealed.Name != "workspace.read" {
		t.Fatalf("real tool name was rejected: %+v err=%v", sealed, err)
	}
	for _, mutate := range []func(*contract.ContextModelInputSegmentBindingV1){
		func(v *contract.ContextModelInputSegmentBindingV1) { v.CallID = "" },
		func(v *contract.ContextModelInputSegmentBindingV1) { v.Name = "" },
		func(v *contract.ContextModelInputSegmentBindingV1) { v.Name = "workspace.write" },
		func(v *contract.ContextModelInputSegmentBindingV1) {
			v.Role = contract.ContextModelInputRoleAssistantV1
		},
		func(v *contract.ContextModelInputSegmentBindingV1) { v.Channel = contract.ContextModelInputMessageV1 },
	} {
		value := sealed
		mutate(&value)
		if err := value.Validate(); !errors.Is(err, contract.ErrConflict) {
			t.Fatalf("semantic drift was accepted: %+v err=%v", value, err)
		}
	}
}

func TestModelInputClosedEnumsAndCanonicalEncodingV1(t *testing.T) {
	for _, tc := range []struct {
		name     string
		channel  contract.ContextModelInputChannelV1
		role     contract.ContextModelInputRoleV1
		encoding contract.ContextModelInputEncodingV1
	}{
		{"channel", "other", contract.ContextModelInputRoleUserV1, contract.ContextModelInputUTF8V1},
		{"role", contract.ContextModelInputMessageV1, "other", contract.ContextModelInputUTF8V1},
		{"encoding", contract.ContextModelInputMessageV1, contract.ContextModelInputRoleUserV1, "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := contract.SealContextModelInputSegmentBindingV1(contract.ContextModelInputSegmentBindingV1{
				FragmentRef: modelInputRefV1("conversation"), Region: contract.RegionDynamicTail, Position: 1,
				Kind: contract.FragmentConversation, Trust: contract.TrustUserInput, Channel: tc.channel, Role: tc.role, Encoding: tc.encoding,
			})
			if !errors.Is(err, contract.ErrInvalid) {
				t.Fatalf("open enum was accepted: %v", err)
			}
		})
	}

	content := []byte(`{"a":1,"b":2}`)
	binding, err := contract.SealContextModelInputSegmentBindingV1(contract.ContextModelInputSegmentBindingV1{
		FragmentRef: modelInputRefV1("json-reference"), Region: contract.RegionDynamicTail, Position: 1,
		Kind: contract.FragmentArtifactReference, Trust: contract.TrustObservation, Channel: contract.ContextModelInputReferenceV1,
		Role: contract.ContextModelInputRoleUserV1, Encoding: contract.ContextModelInputCanonicalJSONV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	segment := contract.ContextModelInputSegmentV1{
		FragmentRef: binding.FragmentRef, Region: binding.Region, Position: binding.Position, Kind: binding.Kind, Trust: binding.Trust,
		Channel: binding.Channel, Role: binding.Role, Encoding: binding.Encoding,
		ContentRef: contract.ContentRef{Ref: "json-content", Digest: contract.DigestBytes(content), Length: uint64(len(content))},
		Content:    content, SemanticBindingDigest: binding.Digest,
	}
	if err := segment.Validate(); err != nil {
		t.Fatal(err)
	}
	segment.Content = []byte(`{"b":2,"a":1}`)
	segment.ContentRef = contract.ContentRef{Ref: "noncanonical-json", Digest: contract.DigestBytes(segment.Content), Length: uint64(len(segment.Content))}
	if err := segment.Validate(); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("non-canonical JSON was accepted: %v", err)
	}
}

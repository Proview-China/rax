package contract_test

import (
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
)

func TestModelInputSourceCurrentProjectionBindsFullMaterialFrameAndOwnerV1(t *testing.T) {
	fixture, err := testfixture.NewModelInputSourceFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	projection, err := contract.SealContextModelInputSourceCurrentProjectionV1(
		contract.ContextModelInputSourceCurrentProjectionV1{
			Owner: fixture.Owner, MaterialSource: fixture.Request.Material,
			Material: fixture.Material, FrameSource: fixture.Request.Frame, Frame: fixture.Frame,
			CheckedUnixNano: fixture.Now.UnixNano(),
			ExpiresUnixNano: fixture.Request.NotAfterUnixNano,
		},
		fixture.Now.UnixNano(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ValidateAgainst(fixture.Request, fixture.Now.UnixNano()); err != nil {
		t.Fatal(err)
	}
	if projection.Digest == fixture.Material.Digest ||
		projection.Digest == fixture.Frame.Digest ||
		projection.Digest == fixture.Owner.BindingDigest {
		t.Fatal("Context projection domains collapsed")
	}
	projection.Material.OrderedSegments[0].Content[0] = 'X'
	if fixture.Material.OrderedSegments[0].Content[0] == 'X' {
		t.Fatal("projection retained caller-owned Material bytes")
	}
}

func TestModelInputSourceCurrentRequestRejectsOwnerKindRefAndDigestSpliceV1(t *testing.T) {
	fixture, err := testfixture.NewModelInputSourceFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*contract.ContextModelInputSourceCurrentRequestV1)
	}{
		{"owner", func(v *contract.ContextModelInputSourceCurrentRequestV1) {
			v.Owner.ComponentID = "context-engine/other"
		}},
		{"material_owner", func(v *contract.ContextModelInputSourceCurrentRequestV1) {
			v.Material.Owner.BindingDigest = contract.DigestBytes([]byte("other-owner"))
		}},
		{"material_kind", func(v *contract.ContextModelInputSourceCurrentRequestV1) {
			v.Material.Kind = contract.ContextInvocationSourceFrameV1
		}},
		{"frame_kind", func(v *contract.ContextModelInputSourceCurrentRequestV1) {
			v.Frame.Kind = contract.ContextInvocationSourceModelInputMaterialV1
		}},
		{"material_id", func(v *contract.ContextModelInputSourceCurrentRequestV1) { v.Material.ID += "-other" }},
		{"frame_revision", func(v *contract.ContextModelInputSourceCurrentRequestV1) { v.Frame.Revision++ }},
		{"frame_digest", func(v *contract.ContextModelInputSourceCurrentRequestV1) {
			v.Frame.Digest = contract.DigestBytes([]byte("other-frame"))
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := fixture.Request
			test.mutate(&request)
			if err := request.Validate(); !errors.Is(err, contract.ErrInvalid) && !errors.Is(err, contract.ErrConflict) {
				t.Fatalf("spliced request accepted: %v", err)
			}
		})
	}
}

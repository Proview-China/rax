package contract_test

import (
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
)

func TestContextModelInputLineageExactSourcesRemainNominallyDistinctV1(t *testing.T) {
	fixture, err := testfixture.NewModelInputLineageFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	material, err := contract.ContextModelInputMaterialExactSourceV1(fixture.Owner, fixture.Material.Ref)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := contract.ContextFrameExactSourceV1(fixture.Owner, fixture.Material.FrameRef)
	if err != nil {
		t.Fatal(err)
	}
	if material.Owner != frame.Owner || material.Kind == frame.Kind || material.Digest == frame.Digest {
		t.Fatalf("material/frame nominal separation lost: material=%+v frame=%+v", material, frame)
	}
	if got, err := material.MaterialRefV1(); err != nil || got != fixture.Material.Ref {
		t.Fatalf("material round trip: got=%+v err=%v", got, err)
	}
	if got, err := frame.FrameRefV1(); err != nil || got != fixture.Material.FrameRef {
		t.Fatalf("frame round trip: got=%+v err=%v", got, err)
	}
	if _, err := material.FrameRefV1(); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("material source type-punned as frame: %v", err)
	}
	if _, err := frame.MaterialRefV1(); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("frame source type-punned as material: %v", err)
	}
}

func TestContextModelInputLineageRequestAndProjectionSealEveryFieldV1(t *testing.T) {
	fixture, err := testfixture.NewModelInputLineageFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	frame, err := contract.ContextFrameExactSourceV1(fixture.Owner, fixture.Material.FrameRef)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := contract.SealContextModelInputLineageCurrentProjectionV1(contract.ContextModelInputLineageCurrentProjectionV1{
		Material: fixture.Request.Source, Frame: frame,
		CheckedUnixNano: fixture.Now.UnixNano(), ExpiresUnixNano: fixture.Now.Add(10).UnixNano(),
	}, fixture.Now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ValidateAgainst(fixture.Request, fixture.Now.UnixNano()); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*contract.ContextModelInputLineageCurrentProjectionV1){
		func(v *contract.ContextModelInputLineageCurrentProjectionV1) {
			v.Material.Owner.BindingDigest = contract.DigestBytes([]byte("owner-drift"))
		},
		func(v *contract.ContextModelInputLineageCurrentProjectionV1) {
			v.Material.Kind = contract.ContextInvocationSourceFrameV1
		},
		func(v *contract.ContextModelInputLineageCurrentProjectionV1) { v.Material.ID += "-drift" },
		func(v *contract.ContextModelInputLineageCurrentProjectionV1) { v.Material.Revision++ },
		func(v *contract.ContextModelInputLineageCurrentProjectionV1) {
			v.Material.Digest = contract.DigestBytes([]byte("material-drift"))
		},
		func(v *contract.ContextModelInputLineageCurrentProjectionV1) {
			v.Frame.Kind = contract.ContextInvocationSourceModelInputMaterialV1
		},
		func(v *contract.ContextModelInputLineageCurrentProjectionV1) { v.Frame.Digest = v.Material.Digest },
	}
	for index, mutate := range mutations {
		changed := projection
		mutate(&changed)
		if err := changed.ValidateAt(fixture.Now.UnixNano()); err == nil {
			t.Fatalf("mutation %d retained a valid projection", index)
		}
	}
}

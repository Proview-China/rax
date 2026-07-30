package blackbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestModelInputLineagePublicReaderMapsMaterialAndFrameExactSourcesV1(t *testing.T) {
	fixture, materials, frames, reader := blackboxLineageReaderV1(t)
	projection, err := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Material.ID != fixture.Material.Ref.ID || projection.Material.Revision != fixture.Material.Ref.Revision || projection.Material.Digest != fixture.Material.Ref.Digest ||
		projection.Frame.ID != fixture.Material.FrameRef.ID || projection.Frame.Revision != fixture.Material.FrameRef.Revision || projection.Frame.Digest != fixture.Material.FrameRef.Digest {
		t.Fatalf("public exact-source mapping drifted: %+v", projection)
	}
	if projection.Material.Owner != fixture.Owner || projection.Frame.Owner != fixture.Owner ||
		projection.Material.Kind != contract.ContextInvocationSourceModelInputMaterialV1 || projection.Frame.Kind != contract.ContextInvocationSourceFrameV1 ||
		projection.Material.Digest == projection.Frame.Digest {
		t.Fatalf("public nominal lineage drifted: %+v", projection)
	}
	if exact, current := materials.CallsV1(); exact != 2 || current != 2 || frames.CallsV1() != 2 {
		t.Fatalf("owner S1/S2 reads: exact=%d current=%d frame=%d", exact, current, frames.CallsV1())
	}
}

func TestModelInputLineageRejectsEveryMaterialCoordinateAxisBeforeFrameUseV1(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*contract.ContextInvocationExactSourceRefV1)
	}{
		{"owner", func(v *contract.ContextInvocationExactSourceRefV1) {
			v.Owner.BindingDigest = contract.DigestBytes([]byte("wrong-owner"))
		}},
		{"kind", func(v *contract.ContextInvocationExactSourceRefV1) { v.Kind = contract.ContextInvocationSourceFrameV1 }},
		{"id", func(v *contract.ContextInvocationExactSourceRefV1) { v.ID += "-wrong" }},
		{"revision", func(v *contract.ContextInvocationExactSourceRefV1) { v.Revision++ }},
		{"digest", func(v *contract.ContextInvocationExactSourceRefV1) {
			v.Digest = contract.DigestBytes([]byte("wrong-material"))
		}},
	}
	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) {
			fixture, _, frames, reader := blackboxLineageReaderV1(t)
			request := fixture.Request
			tt.mutate(&request.Source)
			var err error
			request, err = contract.SealContextModelInputLineageCurrentRequestV1(request)
			if err != nil {
				if frames.CallsV1() != 0 {
					t.Fatalf("invalid %s reached frame reader", tt.name)
				}
				return
			}
			projection, inspectErr := reader.InspectContextModelInputLineageCurrentV1(context.Background(), request)
			if inspectErr == nil || projection != (contract.ContextModelInputLineageCurrentProjectionV1{}) {
				t.Fatalf("%s drift accepted: projection=%+v err=%v", tt.name, projection, inspectErr)
			}
			if frames.CallsV1() != 0 {
				t.Fatalf("%s drift reached downstream frame reader: %d", tt.name, frames.CallsV1())
			}
		})
	}
}

func TestModelInputLineageRejectsEveryFrameExactCoordinateAxisV1(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*contract.FactRef)
	}{
		{"id", func(v *contract.FactRef) { v.ID += "-wrong" }},
		{"revision", func(v *contract.FactRef) { v.Revision++ }},
		{"digest", func(v *contract.FactRef) { v.Digest = contract.DigestBytes([]byte("wrong-frame")) }},
	}
	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) {
			fixture, materials, frames, reader := blackboxLineageReaderV1(t)
			frame := fixture.Frame
			tt.mutate(&frame.FrameRef)
			frame.Digest = ""
			var err error
			frame, err = contract.SealContextFrameExactCurrentProjectionV1(frame, fixture.Now.UnixNano())
			if err != nil {
				t.Fatal(err)
			}
			frames.Sequence = []contract.ContextFrameExactCurrentProjectionV1{frame}
			projection, inspectErr := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request)
			if !errors.Is(inspectErr, contract.ErrConflict) || projection != (contract.ContextModelInputLineageCurrentProjectionV1{}) {
				t.Fatalf("frame %s drift accepted: projection=%+v err=%v", tt.name, projection, inspectErr)
			}
			if exact, current := materials.CallsV1(); exact != 1 || current != 1 || frames.CallsV1() != 1 {
				t.Fatalf("frame %s failure read counts: exact=%d current=%d frame=%d", tt.name, exact, current, frames.CallsV1())
			}
		})
	}
}

func TestModelInputLineageRejectsOldRefResealV1(t *testing.T) {
	fixture, materials, frames, reader := blackboxLineageReaderV1(t)
	tampered := fixture.Material.Clone()
	tampered.FrameRef.Digest = contract.DigestBytes([]byte("spliced-frame"))
	resealed, err := contract.SealContextModelInputMaterialV1(tampered)
	if err != nil || resealed.Ref.Digest == fixture.Material.Ref.Digest {
		t.Fatalf("fixture reseal did not create a distinct exact material: resealed=%+v err=%v", resealed, err)
	}
	materials.ExactSequence = []contract.ContextModelInputMaterialV1{resealed}
	materials.CurrentSequence = []contract.ContextModelInputMaterialV1{resealed}
	projection, inspectErr := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request)
	if !errors.Is(inspectErr, contract.ErrConflict) || projection != (contract.ContextModelInputLineageCurrentProjectionV1{}) {
		t.Fatalf("old exact Ref accepted resealed material: projection=%+v err=%v", projection, inspectErr)
	}
	if frames.CallsV1() != 0 {
		t.Fatalf("old exact Ref reached downstream frame reader: %d", frames.CallsV1())
	}
}

func blackboxLineageReaderV1(t *testing.T) (testfixture.ModelInputLineageFixtureV1, *testfixture.ModelInputLineageMaterialReaderV1, *testfixture.ModelInputLineageFrameReaderV1, *kernel.ContextModelInputLineageCurrentReaderV1) {
	t.Helper()
	fixture, err := testfixture.NewModelInputLineageFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	materials := testfixture.NewModelInputLineageMaterialReaderV1(fixture.Material)
	frames := testfixture.NewModelInputLineageFrameReaderV1(fixture.Frame)
	reader, err := kernel.NewContextModelInputLineageCurrentReaderV1(fixture.Owner, materials, materials, frames, func() time.Time { return fixture.Now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, materials, frames, reader
}

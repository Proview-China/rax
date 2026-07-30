package failure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestModelInputLineageOwnerFailuresReturnNoProjectionV1(t *testing.T) {
	tests := []struct {
		name   string
		target string
		err    error
	}{
		{"exact_not_found", "exact", contract.ErrNotFound},
		{"exact_unavailable", "exact", contract.ErrUnavailable},
		{"exact_unknown", "exact", contract.ErrUnknown},
		{"current_unavailable", "current", contract.ErrUnavailable},
		{"frame_unavailable", "frame", contract.ErrUnavailable},
		{"frame_unknown", "frame", contract.ErrUnknown},
		{"deadline", "exact", context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture, err := testfixture.NewModelInputLineageFixtureV1()
			if err != nil {
				t.Fatal(err)
			}
			materials := testfixture.NewModelInputLineageMaterialReaderV1(fixture.Material)
			frames := testfixture.NewModelInputLineageFrameReaderV1(fixture.Frame)
			switch tt.target {
			case "exact":
				materials.ExactErr = tt.err
			case "current":
				materials.CurrentErr = tt.err
			case "frame":
				frames.Err = tt.err
			}
			reader, err := kernel.NewContextModelInputLineageCurrentReaderV1(fixture.Owner, materials, materials, frames, func() time.Time { return fixture.Now }, 30*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			projection, inspectErr := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request)
			if !errors.Is(inspectErr, tt.err) || projection != (contract.ContextModelInputLineageCurrentProjectionV1{}) {
				t.Fatalf("failure classification drifted: projection=%+v err=%v", projection, inspectErr)
			}
			if tt.target != "frame" && frames.CallsV1() != 0 {
				t.Fatalf("%s reached downstream frame reader: %d", tt.name, frames.CallsV1())
			}
		})
	}
}

func TestModelInputLineageFrameDigestSwapFailsClosedV1(t *testing.T) {
	fixture, err := testfixture.NewModelInputLineageFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	materials := testfixture.NewModelInputLineageMaterialReaderV1(fixture.Material)
	frame := fixture.Frame
	frame.FrameRef.Digest = fixture.Material.Ref.Digest
	frame.Digest = ""
	frame, err = contract.SealContextFrameExactCurrentProjectionV1(frame, fixture.Now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	frames := testfixture.NewModelInputLineageFrameReaderV1(frame)
	reader, err := kernel.NewContextModelInputLineageCurrentReaderV1(fixture.Owner, materials, materials, frames, func() time.Time { return fixture.Now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	projection, inspectErr := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request)
	if !errors.Is(inspectErr, contract.ErrConflict) || projection != (contract.ContextModelInputLineageCurrentProjectionV1{}) {
		t.Fatalf("material digest accepted as frame digest: projection=%+v err=%v", projection, inspectErr)
	}
}

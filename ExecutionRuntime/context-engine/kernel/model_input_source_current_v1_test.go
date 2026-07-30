package kernel_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestModelInputSourceCurrentReaderReturnsFullExactCurrentSourceV1(t *testing.T) {
	fixture, materials, frames, reader := modelInputSourceReaderFixtureV1(t)
	projection, err := reader.InspectContextModelInputSourceCurrentV1(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Owner != fixture.Owner ||
		projection.MaterialSource != fixture.Request.Material ||
		projection.FrameSource != fixture.Request.Frame ||
		projection.Material.Ref != fixture.Material.Ref ||
		len(projection.Material.OrderedSegments) != len(fixture.Material.OrderedSegments) ||
		projection.Frame.FrameRef != fixture.Material.FrameRef ||
		projection.ExpiresUnixNano != fixture.Request.NotAfterUnixNano {
		t.Fatalf("source projection drifted: %+v", projection)
	}
	if exact, current := materials.CallsV1(); exact != 2 || current != 2 || frames.CallsV1() != 2 {
		t.Fatalf("reader did not execute full S1/S2: exact=%d current=%d frame=%d", exact, current, frames.CallsV1())
	}
	projection.Material.OrderedSegments[0].Content[0] = 'X'
	if fixture.Material.OrderedSegments[0].Content[0] == 'X' {
		t.Fatal("reader leaked mutable Material bytes")
	}
}

func TestModelInputSourceCurrentReaderRejectsHistoryOwnerFrameAndClockDriftV1(t *testing.T) {
	t.Run("history_only", func(t *testing.T) {
		fixture, materials, frames, reader := modelInputSourceReaderFixtureV1(t)
		next := fixture.Material.Clone()
		next.Ref.Revision++
		next.Ref.Digest = ""
		next.Digest = ""
		var err error
		next, err = contract.SealContextModelInputMaterialV1(next)
		if err != nil {
			t.Fatal(err)
		}
		materials.CurrentSequence = []contract.ContextModelInputMaterialV1{next}
		got, inspectErr := reader.InspectContextModelInputSourceCurrentV1(context.Background(), fixture.Request)
		if !errors.Is(inspectErr, contract.ErrConflict) ||
			got.Digest != "" || frames.CallsV1() != 0 {
			t.Fatalf("history-only accepted: got=%+v frame_calls=%d err=%v", got, frames.CallsV1(), inspectErr)
		}
	})
	t.Run("owner_drift", func(t *testing.T) {
		fixture, _, frames, reader := modelInputSourceReaderFixtureV1(t)
		frames.Owner.ComponentID = "context-engine/drifted"
		got, inspectErr := reader.InspectContextModelInputSourceCurrentV1(context.Background(), fixture.Request)
		if !errors.Is(inspectErr, contract.ErrConflict) || got.Digest != "" {
			t.Fatalf("Owner drift accepted: got=%+v err=%v", got, inspectErr)
		}
	})
	t.Run("frame_current_false", func(t *testing.T) {
		fixture, _, frames, reader := modelInputSourceReaderFixtureV1(t)
		frame := fixture.Frame
		frame.Current = false
		frame.Digest = ""
		frames.Sequence = []contract.ContextFrameExactCurrentProjectionV1{frame}
		got, inspectErr := reader.InspectContextModelInputSourceCurrentV1(context.Background(), fixture.Request)
		if !errors.Is(inspectErr, contract.ErrConflict) || got.Digest != "" {
			t.Fatalf("non-current Frame accepted: got=%+v err=%v", got, inspectErr)
		}
	})
	t.Run("ttl_crossing", func(t *testing.T) {
		fixture, materials, frames, _ := modelInputSourceReaderFixtureV1(t)
		var calls atomic.Int32
		reader, err := kernel.NewContextModelInputSourceCurrentReaderV1(
			fixture.Owner, materials, materials, frames,
			func() time.Time {
				if calls.Add(1) == 1 {
					return fixture.Now
				}
				return time.Unix(0, fixture.Request.NotAfterUnixNano)
			},
			30*time.Second,
		)
		if err != nil {
			t.Fatal(err)
		}
		got, inspectErr := reader.InspectContextModelInputSourceCurrentV1(context.Background(), fixture.Request)
		if !errors.Is(inspectErr, contract.ErrExpired) || got.Digest != "" {
			t.Fatalf("TTL crossing accepted: got=%+v err=%v", got, inspectErr)
		}
	})
	t.Run("clock_rollback", func(t *testing.T) {
		fixture, materials, frames, _ := modelInputSourceReaderFixtureV1(t)
		var calls atomic.Int32
		reader, err := kernel.NewContextModelInputSourceCurrentReaderV1(
			fixture.Owner, materials, materials, frames,
			func() time.Time {
				if calls.Add(1) == 1 {
					return fixture.Now
				}
				return fixture.Now.Add(-time.Nanosecond)
			},
			30*time.Second,
		)
		if err != nil {
			t.Fatal(err)
		}
		got, inspectErr := reader.InspectContextModelInputSourceCurrentV1(context.Background(), fixture.Request)
		if !errors.Is(inspectErr, contract.ErrConflict) || got.Digest != "" {
			t.Fatalf("clock rollback accepted: got=%+v err=%v", got, inspectErr)
		}
	})
}

func TestModelInputSourceCurrentReaderUnavailableTypedNilCancelAndUnknownV1(t *testing.T) {
	fixture, materials, frames, _ := modelInputSourceReaderFixtureV1(t)
	var typedNilMaterials *testfixture.ModelInputLineageMaterialReaderV1
	if reader, err := kernel.NewContextModelInputSourceCurrentReaderV1(
		fixture.Owner, typedNilMaterials, typedNilMaterials, frames,
		func() time.Time { return fixture.Now }, time.Second,
	); err == nil || reader != nil {
		t.Fatal("typed-nil Material reader accepted")
	}
	var typedNilFrames *testfixture.ModelInputLineageFrameReaderV1
	if reader, err := kernel.NewContextModelInputSourceCurrentReaderV1(
		fixture.Owner, materials, materials, typedNilFrames,
		func() time.Time { return fixture.Now }, time.Second,
	); err == nil || reader != nil {
		t.Fatal("typed-nil Frame reader accepted")
	}
	for _, test := range []struct {
		name string
		err  error
	}{
		{"unavailable", contract.ErrUnavailable},
		{"unknown", contract.ErrUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, materials, _, reader := modelInputSourceReaderFixtureV1(t)
			materials.ExactErr = test.err
			got, err := reader.InspectContextModelInputSourceCurrentV1(context.Background(), fixture.Request)
			if !errors.Is(err, test.err) || got.Digest != "" {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := modelInputSourceReaderForV1(t, fixture, materials, frames).
		InspectContextModelInputSourceCurrentV1(ctx, fixture.Request)
	if !errors.Is(err, context.Canceled) || got.Digest != "" {
		t.Fatalf("cancel got=%+v err=%v", got, err)
	}
}

func TestModelInputSourceCurrentReader64ConcurrentStableWindowV1(t *testing.T) {
	fixture, _, _, reader := modelInputSourceReaderFixtureV1(t)
	var wait sync.WaitGroup
	failures := make(chan error, 64)
	digests := make(chan contract.Digest, 64)
	for index := 0; index < 64; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			projection, err := reader.InspectContextModelInputSourceCurrentV1(context.Background(), fixture.Request)
			if err != nil {
				failures <- err
				return
			}
			digests <- projection.Digest
		}()
	}
	wait.Wait()
	close(failures)
	close(digests)
	for err := range failures {
		t.Fatal(err)
	}
	var expected contract.Digest
	for digest := range digests {
		if expected == "" {
			expected = digest
		}
		if digest != expected {
			t.Fatalf("stable window digest drifted: got=%q want=%q", digest, expected)
		}
	}
}

func modelInputSourceReaderFixtureV1(
	t *testing.T,
) (
	testfixture.ModelInputSourceFixtureV1,
	*testfixture.ModelInputLineageMaterialReaderV1,
	*testfixture.ModelInputLineageFrameReaderV1,
	*kernel.ContextModelInputSourceCurrentReaderV1,
) {
	t.Helper()
	fixture, err := testfixture.NewModelInputSourceFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	materials := testfixture.NewModelInputLineageMaterialReaderV1(fixture.Material)
	frames := testfixture.NewModelInputLineageFrameReaderV1(fixture.Owner, fixture.Frame)
	reader := modelInputSourceReaderForV1(t, fixture, materials, frames)
	return fixture, materials, frames, reader
}

func modelInputSourceReaderForV1(
	t *testing.T,
	fixture testfixture.ModelInputSourceFixtureV1,
	materials *testfixture.ModelInputLineageMaterialReaderV1,
	frames *testfixture.ModelInputLineageFrameReaderV1,
) *kernel.ContextModelInputSourceCurrentReaderV1 {
	t.Helper()
	reader, err := kernel.NewContextModelInputSourceCurrentReaderV1(
		fixture.Owner, materials, materials, frames,
		func() time.Time { return fixture.Now }, 30*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

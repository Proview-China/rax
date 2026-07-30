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

func TestContextModelInputLineageCurrentReaderExactS1S2AndTTLMinimumV1(t *testing.T) {
	fixture, materials, frames, reader := lineageReaderFixtureV1(t, nil)
	projection, err := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Material != fixture.Request.Source || projection.Frame.ID != fixture.Material.FrameRef.ID || projection.Frame.Revision != fixture.Material.FrameRef.Revision || projection.Frame.Digest != fixture.Material.FrameRef.Digest {
		t.Fatalf("wrong exact lineage projection: %+v", projection)
	}
	if projection.ExpiresUnixNano != fixture.Request.NotAfterUnixNano {
		t.Fatalf("TTL minimum drifted: got=%d want=%d", projection.ExpiresUnixNano, fixture.Request.NotAfterUnixNano)
	}
	if exact, current := materials.CallsV1(); exact != 2 || current != 2 || frames.CallsV1() != 2 {
		t.Fatalf("S1/S2 reads: exact=%d current=%d frame=%d", exact, current, frames.CallsV1())
	}
}

func TestContextModelInputLineageRejectsHistoryOnlyAndS1S2DriftV1(t *testing.T) {
	t.Run("history_only", func(t *testing.T) {
		fixture, materials, frames, reader := lineageReaderFixtureV1(t, nil)
		next := resealLineageMaterialV1(t, fixture.Material, fixture.Material.Ref.Revision+1, fixture.Material.FrameRef)
		materials.CurrentSequence = []contract.ContextModelInputMaterialV1{next}
		if got, err := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request); !errors.Is(err, contract.ErrConflict) || got != (contract.ContextModelInputLineageCurrentProjectionV1{}) {
			t.Fatalf("history-only accepted: got=%+v err=%v", got, err)
		}
		if frames.CallsV1() != 0 {
			t.Fatalf("history-only reached frame reader: %d", frames.CallsV1())
		}
	})
	t.Run("material_s2", func(t *testing.T) {
		fixture, materials, _, reader := lineageReaderFixtureV1(t, nil)
		next := resealLineageMaterialV1(t, fixture.Material, fixture.Material.Ref.Revision+1, fixture.Material.FrameRef)
		materials.ExactSequence = []contract.ContextModelInputMaterialV1{fixture.Material, next}
		materials.CurrentSequence = []contract.ContextModelInputMaterialV1{fixture.Material, next}
		if got, err := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request); !errors.Is(err, contract.ErrConflict) || got != (contract.ContextModelInputLineageCurrentProjectionV1{}) {
			t.Fatalf("material S2 drift accepted: got=%+v err=%v", got, err)
		}
	})
	t.Run("frame_s2", func(t *testing.T) {
		fixture, _, frames, reader := lineageReaderFixtureV1(t, nil)
		nextRef := fixture.Material.FrameRef
		nextRef.Revision++
		nextRef.Digest = contract.DigestBytes([]byte("frame-s2-drift"))
		next, err := contract.SealContextFrameExactCurrentProjectionV1(contract.ContextFrameExactCurrentProjectionV1{
			FrameRef: nextRef, Current: true, CheckedUnixNano: fixture.Now.Add(-time.Second).UnixNano(), ExpiresUnixNano: fixture.Now.Add(30 * time.Second).UnixNano(),
		}, fixture.Now.UnixNano())
		if err != nil {
			t.Fatal(err)
		}
		frames.Sequence = []contract.ContextFrameExactCurrentProjectionV1{fixture.Frame, next}
		if got, err := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request); !errors.Is(err, contract.ErrConflict) || got != (contract.ContextModelInputLineageCurrentProjectionV1{}) {
			t.Fatalf("frame S2 drift accepted: got=%+v err=%v", got, err)
		}
	})
}

func TestContextModelInputLineageFailClosedErrorsAndTypedNilV1(t *testing.T) {
	fixture, err := testfixture.NewModelInputLineageFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	var typedNil *testfixture.ModelInputLineageMaterialReaderV1
	if reader, err := kernel.NewContextModelInputLineageCurrentReaderV1(fixture.Owner, typedNil, typedNil, testfixture.NewModelInputLineageFrameReaderV1(fixture.Owner, fixture.Frame), func() time.Time { return fixture.Now }, time.Second); err == nil || reader != nil {
		t.Fatalf("typed nil dependencies accepted: reader=%v err=%v", reader, err)
	}
	var typedNilFrame *testfixture.ModelInputLineageFrameReaderV1
	materials := testfixture.NewModelInputLineageMaterialReaderV1(fixture.Material)
	if reader, err := kernel.NewContextModelInputLineageCurrentReaderV1(fixture.Owner, materials, materials, typedNilFrame, func() time.Time { return fixture.Now }, time.Second); err == nil || reader != nil {
		t.Fatalf("typed nil frame accepted: reader=%v err=%v", reader, err)
	}

	t.Run("unavailable", func(t *testing.T) {
		for _, target := range []string{"exact", "current", "frame"} {
			fixture, materials, frames, reader := lineageReaderFixtureV1(t, nil)
			switch target {
			case "exact":
				materials.ExactErr = contract.ErrUnavailable
			case "current":
				materials.CurrentErr = contract.ErrUnavailable
			case "frame":
				frames.Err = contract.ErrUnavailable
			}
			if got, err := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request); !errors.Is(err, contract.ErrUnavailable) || got != (contract.ContextModelInputLineageCurrentProjectionV1{}) {
				t.Fatalf("%s unavailable: got=%+v err=%v", target, got, err)
			}
		}
	})
	t.Run("cancel", func(t *testing.T) {
		fixture, materials, frames, reader := lineageReaderFixtureV1(t, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if got, err := reader.InspectContextModelInputLineageCurrentV1(ctx, fixture.Request); !errors.Is(err, context.Canceled) || got != (contract.ContextModelInputLineageCurrentProjectionV1{}) {
			t.Fatalf("cancel: got=%+v err=%v", got, err)
		}
		if exact, current := materials.CallsV1(); exact != 0 || current != 0 || frames.CallsV1() != 0 {
			t.Fatalf("cancel reached dependencies: exact=%d current=%d frame=%d", exact, current, frames.CallsV1())
		}
	})
}

func TestContextModelInputLineageExpiryUsesEveryMinimumV1(t *testing.T) {
	t.Run("max_ttl", func(t *testing.T) {
		fixture, materials, frames, _ := lineageReaderFixtureV1(t, nil)
		reader, err := kernel.NewContextModelInputLineageCurrentReaderV1(fixture.Owner, materials, materials, frames, func() time.Time { return fixture.Now }, 3*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		projection, err := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request)
		if err != nil {
			t.Fatal(err)
		}
		if want := fixture.Now.Add(3 * time.Second).UnixNano(); projection.ExpiresUnixNano != want {
			t.Fatalf("max TTL minimum: got=%d want=%d", projection.ExpiresUnixNano, want)
		}
	})
	t.Run("material", func(t *testing.T) {
		fixture, _, frames, _ := lineageReaderFixtureV1(t, nil)
		material := fixture.Material.Clone()
		material.ExpiresUnixNano = fixture.Now.Add(4 * time.Second).UnixNano()
		material.Ref.Digest = ""
		material.Digest = ""
		material, err := contract.SealContextModelInputMaterialV1(material)
		if err != nil {
			t.Fatal(err)
		}
		source, err := contract.ContextModelInputMaterialExactSourceV1(fixture.Owner, material.Ref)
		if err != nil {
			t.Fatal(err)
		}
		request, err := contract.SealContextModelInputLineageCurrentRequestV1(contract.ContextModelInputLineageCurrentRequestV1{
			Source: source, CheckedUnixNano: fixture.Now.UnixNano(), NotAfterUnixNano: fixture.Now.Add(20 * time.Second).UnixNano(),
		})
		if err != nil {
			t.Fatal(err)
		}
		materials := testfixture.NewModelInputLineageMaterialReaderV1(material)
		reader, err := kernel.NewContextModelInputLineageCurrentReaderV1(fixture.Owner, materials, materials, frames, func() time.Time { return fixture.Now }, 30*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		projection, err := reader.InspectContextModelInputLineageCurrentV1(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if projection.ExpiresUnixNano != material.ExpiresUnixNano {
			t.Fatalf("material TTL minimum: got=%d want=%d", projection.ExpiresUnixNano, material.ExpiresUnixNano)
		}
	})
	t.Run("frame", func(t *testing.T) {
		fixture, materials, _, _ := lineageReaderFixtureV1(t, nil)
		frame, err := contract.SealContextFrameExactCurrentProjectionV1(contract.ContextFrameExactCurrentProjectionV1{
			FrameRef: fixture.Material.FrameRef, Current: true, CheckedUnixNano: fixture.Now.Add(-time.Second).UnixNano(), ExpiresUnixNano: fixture.Now.Add(5 * time.Second).UnixNano(),
		}, fixture.Now.UnixNano())
		if err != nil {
			t.Fatal(err)
		}
		frames := testfixture.NewModelInputLineageFrameReaderV1(fixture.Owner, frame)
		reader, err := kernel.NewContextModelInputLineageCurrentReaderV1(fixture.Owner, materials, materials, frames, func() time.Time { return fixture.Now }, 30*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		projection, err := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request)
		if err != nil {
			t.Fatal(err)
		}
		if projection.ExpiresUnixNano != frame.ExpiresUnixNano {
			t.Fatalf("frame TTL minimum: got=%d want=%d", projection.ExpiresUnixNano, frame.ExpiresUnixNano)
		}
	})
}

func TestContextModelInputLineageClockAndTTLCrossingV1(t *testing.T) {
	t.Run("rollback", func(t *testing.T) {
		fixture, materials, frames, reader := lineageReaderFixtureV1(t, []time.Time{time.Unix(0, fixtureTimeV1(t).UnixNano()-1)})
		if got, err := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request); !errors.Is(err, contract.ErrConflict) || got != (contract.ContextModelInputLineageCurrentProjectionV1{}) {
			t.Fatalf("clock rollback: got=%+v err=%v", got, err)
		}
		if exact, current := materials.CallsV1(); exact != 0 || current != 0 || frames.CallsV1() != 0 {
			t.Fatalf("clock rollback reached dependencies")
		}
	})
	t.Run("crossing", func(t *testing.T) {
		fixture, _, _, reader := lineageReaderFixtureV1(t, []time.Time{fixtureTimeV1(t), fixtureTimeV1(t).Add(21 * time.Second)})
		if got, err := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request); !errors.Is(err, contract.ErrExpired) || got != (contract.ContextModelInputLineageCurrentProjectionV1{}) {
			t.Fatalf("TTL crossing: got=%+v err=%v", got, err)
		}
	})
	t.Run("now_equals_expiry_fails_closed", func(t *testing.T) {
		now := fixtureTimeV1(t)
		fixture, _, _, reader := lineageReaderFixtureV1(t, []time.Time{now, now.Add(20 * time.Second)})
		if got, err := reader.InspectContextModelInputLineageCurrentV1(context.Background(), fixture.Request); !errors.Is(err, contract.ErrExpired) || got != (contract.ContextModelInputLineageCurrentProjectionV1{}) {
			t.Fatalf("now==expiry accepted: got=%+v err=%v", got, err)
		}
	})
}

func TestContextModelInputLineage64ConcurrentStableBeforeFlipAfterWindowsV1(t *testing.T) {
	before, beforeMaterials, _, beforeReader := lineageReaderFixtureV1(t, nil)
	beforeSuccess, beforeConflict := runConcurrentLineageWindowV1(t, beforeReader, before.Request, before.Request.Source, before.Material.FrameRef.Digest)
	if beforeSuccess != 64 || beforeConflict != 0 {
		t.Fatalf("stable before window: success=%d conflict=%d", beforeSuccess, beforeConflict)
	}
	if exact, current := beforeMaterials.CallsV1(); exact == 0 || current == 0 {
		t.Fatalf("stable before material readers were not exercised")
	}

	fixture, flipMaterials, frames, flipReader := lineageReaderFixtureV1(t, nil)
	nextRef := fixture.Material.FrameRef
	nextRef.Revision++
	nextRef.Digest = contract.DigestBytes([]byte("concurrent-frame-flip"))
	nextFrame, err := contract.SealContextFrameExactCurrentProjectionV1(contract.ContextFrameExactCurrentProjectionV1{
		FrameRef: nextRef, Current: true, CheckedUnixNano: fixture.Now.Add(-time.Second).UnixNano(), ExpiresUnixNano: fixture.Now.Add(30 * time.Second).UnixNano(),
	}, fixture.Now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	frames.Sequence = []contract.ContextFrameExactCurrentProjectionV1{fixture.Frame, nextFrame}
	flipSuccess, flipConflict := runConcurrentLineageWindowV1(t, flipReader, fixture.Request, fixture.Request.Source, fixture.Material.FrameRef.Digest)
	if flipSuccess != 0 || flipConflict != 64 {
		t.Fatalf("flip window was not fail-closed: success=%d conflict=%d", flipSuccess, flipConflict)
	}
	if exact, current := flipMaterials.CallsV1(); exact == 0 || current == 0 {
		t.Fatalf("flip material readers were not exercised")
	}

	afterMaterial := resealLineageMaterialV1(t, fixture.Material, fixture.Material.Ref.Revision+1, nextRef)
	afterSource, err := contract.ContextModelInputMaterialExactSourceV1(fixture.Owner, afterMaterial.Ref)
	if err != nil {
		t.Fatal(err)
	}
	afterRequest, err := contract.SealContextModelInputLineageCurrentRequestV1(contract.ContextModelInputLineageCurrentRequestV1{
		Source: afterSource, CheckedUnixNano: fixture.Now.UnixNano(), NotAfterUnixNano: fixture.Request.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	afterMaterials := testfixture.NewModelInputLineageMaterialReaderV1(afterMaterial)
	afterFrames := testfixture.NewModelInputLineageFrameReaderV1(fixture.Owner, nextFrame)
	afterReader, err := kernel.NewContextModelInputLineageCurrentReaderV1(
		fixture.Owner, afterMaterials, afterMaterials, afterFrames, func() time.Time { return fixture.Now }, 30*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	afterSuccess, afterConflict := runConcurrentLineageWindowV1(t, afterReader, afterRequest, afterSource, nextRef.Digest)
	if afterSuccess != 64 || afterConflict != 0 {
		t.Fatalf("stable after window: success=%d conflict=%d", afterSuccess, afterConflict)
	}
}

func runConcurrentLineageWindowV1(
	t *testing.T,
	reader *kernel.ContextModelInputLineageCurrentReaderV1,
	request contract.ContextModelInputLineageCurrentRequestV1,
	wantMaterial contract.ContextInvocationExactSourceRefV1,
	wantFrameDigest contract.Digest,
) (int64, int64) {
	t.Helper()
	const workers = 64
	var wg sync.WaitGroup
	var success atomic.Int64
	var conflict atomic.Int64
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := reader.InspectContextModelInputLineageCurrentV1(context.Background(), request)
			if err == nil {
				if got.Material != wantMaterial || got.Frame.Digest != wantFrameDigest {
					t.Errorf("spliced projection: %+v", got)
					return
				}
				success.Add(1)
				return
			}
			if errors.Is(err, contract.ErrConflict) {
				conflict.Add(1)
				return
			}
			t.Errorf("unexpected concurrent error: %v", err)
		}()
	}
	wg.Wait()
	if success.Load()+conflict.Load() != workers {
		t.Fatalf("concurrent accounting: success=%d conflict=%d", success.Load(), conflict.Load())
	}
	return success.Load(), conflict.Load()
}

type clockSequenceV1 struct {
	mu     sync.Mutex
	values []time.Time
	index  int
}

func (c *clockSequenceV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.values) == 0 {
		return time.Time{}
	}
	index := c.index
	if index >= len(c.values) {
		index = len(c.values) - 1
	}
	c.index++
	return c.values[index]
}

func lineageReaderFixtureV1(t *testing.T, times []time.Time) (testfixture.ModelInputLineageFixtureV1, *testfixture.ModelInputLineageMaterialReaderV1, *testfixture.ModelInputLineageFrameReaderV1, *kernel.ContextModelInputLineageCurrentReaderV1) {
	t.Helper()
	fixture, err := testfixture.NewModelInputLineageFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	if times == nil {
		times = []time.Time{fixture.Now, fixture.Now}
	}
	clock := &clockSequenceV1{values: times}
	materials := testfixture.NewModelInputLineageMaterialReaderV1(fixture.Material)
	frames := testfixture.NewModelInputLineageFrameReaderV1(fixture.Owner, fixture.Frame)
	reader, err := kernel.NewContextModelInputLineageCurrentReaderV1(fixture.Owner, materials, materials, frames, clock.Now, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, materials, frames, reader
}

func fixtureTimeV1(t *testing.T) time.Time {
	t.Helper()
	fixture, err := testfixture.NewModelInputLineageFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	return fixture.Now
}

func resealLineageMaterialV1(t *testing.T, material contract.ContextModelInputMaterialV1, revision uint64, frame contract.FactRef) contract.ContextModelInputMaterialV1 {
	t.Helper()
	next := material.Clone()
	next.Ref.Revision = revision
	next.Ref.Digest = ""
	next.Digest = ""
	next.FrameRef = frame
	sealed, err := contract.SealContextModelInputMaterialV1(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

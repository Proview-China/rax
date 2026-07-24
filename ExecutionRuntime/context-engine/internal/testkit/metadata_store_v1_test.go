package testkit_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestMetadataStoreV1ExactReadsAndDrift(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	fixture, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	binding, err := fixture.Metadata.ResolveExactSourceBinding(ctx, fixture.Source)
	if err != nil || binding.Source != fixture.Source {
		t.Fatalf("exact source binding: %v", err)
	}
	frame, err := fixture.Metadata.FrameByExactRef(ctx, fixture.Binding.Subject.FrameRef, fixture.Binding.Subject.ExecutionScopeDigest)
	if err != nil || frame.ID != fixture.Frame.ID {
		t.Fatalf("exact frame: %v", err)
	}

	driftedSource := fixture.Source
	driftedSource.Digest = testkit.D("same-id-other-digest")
	if _, err := fixture.Metadata.ResolveExactSourceBinding(ctx, driftedSource); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("same ID changed digest must conflict, got %v", err)
	}
	driftedFrame := fixture.Binding.Subject.FrameRef
	driftedFrame.Digest = testkit.D("same-id-other-frame-digest")
	if _, err := fixture.Metadata.FrameByExactRef(ctx, driftedFrame, fixture.Binding.Subject.ExecutionScopeDigest); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("same Frame ID changed digest must conflict, got %v", err)
	}
	driftedManifest := fixture.Binding.Subject.ManifestRef
	driftedManifest.Revision++
	if _, err := fixture.Metadata.ManifestByExactRef(ctx, driftedManifest, fixture.Binding.Subject.ExecutionScopeDigest); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("same Manifest ID changed revision must conflict, got %v", err)
	}
	driftedGeneration := fixture.Binding.Subject.GenerationRef
	driftedGeneration.Digest = testkit.D("same-id-other-generation-digest")
	if _, err := fixture.Metadata.GenerationByExactRef(ctx, driftedGeneration, fixture.Binding.Subject.ExecutionScopeDigest); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("same Generation ID changed digest must conflict, got %v", err)
	}
	if _, err := fixture.Metadata.FrameByExactRef(ctx, fixture.Binding.Subject.FrameRef, testkit.D("tenant-b-scope")); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("same Frame ID cross-scope read must conflict, got %v", err)
	}
}

func TestMetadataStoreV1AmbiguousScopeAndUnavailable(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	fixture, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	other := fixture.Binding
	other.Subject.ExecutionScopeDigest = testkit.D("tenant-b-scope")
	if err := fixture.Metadata.AddSourceBindingCandidate(fixture.Source, other); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Metadata.ResolveExactSourceBinding(context.Background(), fixture.Source); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("cross-scope ambiguity must conflict, got %v", err)
	}

	clean, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	clean.Metadata.SetUnavailable(testkit.MetadataReadManifestV1, true)
	if _, err := clean.Metadata.ManifestByExactRef(context.Background(), clean.Binding.Subject.ManifestRef, clean.Binding.Subject.ExecutionScopeDigest); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("unavailable manifest read: %v", err)
	}
	empty := testkit.NewMetadataStoreV1()
	if _, err := empty.ResolveExactSourceBinding(context.Background(), clean.Source); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("missing source binding must be NotFound: %v", err)
	}
}

func TestMetadataStoreV1PointerSwitchAndConcurrentReads(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	fixture, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	request := contract.ContextGenerationCurrentPointerRequestV1{
		ExecutionScopeDigest: fixture.Binding.Subject.ExecutionScopeDigest,
		RunID:                fixture.Binding.Subject.RunID,
		SessionRef:           fixture.Binding.Subject.SessionRef,
		Turn:                 fixture.Binding.Subject.Turn,
	}
	next := fixture.Pointer
	next.Revision++
	next.ExpiresUnixNano++
	next, err = contract.SealContextGenerationCurrentPointerV1(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.Metadata.PutCurrentGenerationPointer(next); err != nil {
		t.Fatal(err)
	}
	current, err := fixture.Metadata.InspectCurrentGenerationPointer(context.Background(), request)
	if err != nil || current.Revision != next.Revision {
		t.Fatalf("pointer switch was not visible: %+v %v", current, err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			binding, readErr := fixture.Metadata.ResolveExactSourceBinding(context.Background(), fixture.Source)
			if readErr != nil {
				errCh <- readErr
				return
			}
			if binding.Source != fixture.Source {
				errCh <- errors.New("concurrent exact source read drifted")
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestMutableReferenceStoreV1CanInjectChangedContent(t *testing.T) {
	store := testkit.NewMutableReferenceStoreV1()
	ref, err := store.Put([]byte("original"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Corrupt(ref, []byte("changed")); err != nil {
		t.Fatal(err)
	}
	value, err := store.Get(ref)
	if err != nil {
		t.Fatal(err)
	}
	if contract.DigestBytes(value) == ref.Digest {
		t.Fatal("corruption fixture did not create digest drift")
	}
}

func TestMetadataStoreV1PreservesCanceledAndDeadline(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	fixture, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	pointerRequest := contract.ContextGenerationCurrentPointerRequestV1{
		ExecutionScopeDigest: fixture.Binding.Subject.ExecutionScopeDigest,
		RunID:                fixture.Binding.Subject.RunID,
		SessionRef:           fixture.Binding.Subject.SessionRef,
		Turn:                 fixture.Binding.Subject.Turn,
	}
	readers := []struct {
		name string
		call func(context.Context) error
	}{
		{"source", func(ctx context.Context) error {
			_, err := fixture.Metadata.ResolveExactSourceBinding(ctx, fixture.Source)
			return err
		}},
		{"frame", func(ctx context.Context) error {
			_, err := fixture.Metadata.FrameByExactRef(ctx, fixture.Binding.Subject.FrameRef, fixture.Binding.Subject.ExecutionScopeDigest)
			return err
		}},
		{"manifest", func(ctx context.Context) error {
			_, err := fixture.Metadata.ManifestByExactRef(ctx, fixture.Binding.Subject.ManifestRef, fixture.Binding.Subject.ExecutionScopeDigest)
			return err
		}},
		{"generation", func(ctx context.Context) error {
			_, err := fixture.Metadata.GenerationByExactRef(ctx, fixture.Binding.Subject.GenerationRef, fixture.Binding.Subject.ExecutionScopeDigest)
			return err
		}},
		{"pointer", func(ctx context.Context) error {
			_, err := fixture.Metadata.InspectCurrentGenerationPointer(ctx, pointerRequest)
			return err
		}},
	}
	contexts := []struct {
		name string
		make func() (context.Context, context.CancelFunc)
		want error
	}{
		{"canceled", func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, func() {}
		}, context.Canceled},
		{"deadline", func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), time.Unix(0, 1))
		}, context.DeadlineExceeded},
	}
	for _, reader := range readers {
		for _, contextCase := range contexts {
			t.Run(reader.name+"_"+contextCase.name, func(t *testing.T) {
				ctx, cancel := contextCase.make()
				defer cancel()
				if err := reader.call(ctx); !errors.Is(err, contextCase.want) {
					t.Fatalf("got %v want %v", err, contextCase.want)
				}
			})
		}
	}
}

package kernel_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestParentFrameCurrentReaderExactS1S2AndTTLMinimum(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	fixture, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := fixture.Reader.InspectContextParentFrameCurrentV1(context.Background(), fixture.Source)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Source != fixture.Source || projection.FrameRef != fixture.Binding.Subject.FrameRef || projection.ManifestRef != fixture.Binding.Subject.ManifestRef || projection.GenerationRef != fixture.Binding.Subject.GenerationRef {
		t.Fatalf("projection lost exact refs: %+v", projection)
	}
	if projection.ExecutionScopeDigest != fixture.Frame.Execution.ScopeDigest {
		t.Fatalf("scope must come from the exact Owner-read Frame: %s", projection.ExecutionScopeDigest)
	}
	if want := fixture.Pointer.ExpiresUnixNano; projection.ExpiresUnixNano != want {
		t.Fatalf("TTL must take the strict owner minimum: got %d want %d", projection.ExpiresUnixNano, want)
	}
}

func TestParentFrameCurrentReaderRejectsTTLOverThirtySeconds(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	if _, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second+time.Nanosecond); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("expected invalid TTL cap, got %v", err)
	}
}

func TestParentFrameCurrentReaderTTLUsesEveryAvailableUpperBound(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	tests := []struct {
		name   string
		maxTTL time.Duration
		mutate func(*contract.ContextParentFrameSourceBindingV1, *contract.ContextGenerationCurrentPointerV1)
		want   time.Duration
	}{
		{"request_cap", 5 * time.Second, func(*contract.ContextParentFrameSourceBindingV1, *contract.ContextGenerationCurrentPointerV1) {}, 5 * time.Second},
		{"binding", 30 * time.Second, func(b *contract.ContextParentFrameSourceBindingV1, _ *contract.ContextGenerationCurrentPointerV1) {
			b.BindingExpiresUnixNano = now.Add(6 * time.Second).UnixNano()
		}, 6 * time.Second},
		{"recipe", 30 * time.Second, func(b *contract.ContextParentFrameSourceBindingV1, _ *contract.ContextGenerationCurrentPointerV1) {
			b.RecipeExpiresUnixNano = now.Add(7 * time.Second).UnixNano()
		}, 7 * time.Second},
		{"authority", 30 * time.Second, func(b *contract.ContextParentFrameSourceBindingV1, _ *contract.ContextGenerationCurrentPointerV1) {
			b.AuthorityExpiresUnixNano = now.Add(8 * time.Second).UnixNano()
		}, 8 * time.Second},
		{"generation_pointer", 30 * time.Second, func(_ *contract.ContextParentFrameSourceBindingV1, p *contract.ContextGenerationCurrentPointerV1) {
			p.ExpiresUnixNano = now.Add(9 * time.Second).UnixNano()
		}, 9 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			binding := fixture.Binding
			pointer := fixture.Pointer
			tt.mutate(&binding, &pointer)
			pointer, err = contract.SealContextGenerationCurrentPointerV1(pointer)
			if err != nil {
				t.Fatal(err)
			}
			if err := fixture.Metadata.PutSourceBinding(binding); err != nil {
				t.Fatal(err)
			}
			if err := fixture.Metadata.PutCurrentGenerationPointer(pointer); err != nil {
				t.Fatal(err)
			}
			reader, err := kernel.NewParentFrameCurrentReaderV1(fixture.Metadata, fixture.Metadata, fixture.Metadata, fixture.Metadata, fixture.Metadata, fixture.Content, func() time.Time { return now }, tt.maxTTL)
			if err != nil {
				t.Fatal(err)
			}
			projection, err := reader.InspectContextParentFrameCurrentV1(context.Background(), fixture.Source)
			if err != nil {
				t.Fatal(err)
			}
			if want := now.Add(tt.want).UnixNano(); projection.ExpiresUnixNano != want {
				t.Fatalf("expiry=%d want=%d", projection.ExpiresUnixNano, want)
			}
		})
	}
}

func TestParentFrameCurrentReaderRejectsPointerSwitchBetweenS1S2(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	fixture, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	next := fixture.Pointer
	next.Revision++
	next.GenerationRef = contract.FactRef{ID: "generation-parent-current-2", Revision: 1, Digest: testkit.D("generation-parent-current-2")}
	next.GenerationOrdinal++
	next, err = contract.SealContextGenerationCurrentPointerV1(next)
	if err != nil {
		t.Fatal(err)
	}
	pointers := &switchingPointerReaderV1{base: fixture.Metadata, next: next}
	reader, err := kernel.NewParentFrameCurrentReaderV1(fixture.Metadata, fixture.Metadata, fixture.Metadata, fixture.Metadata, pointers, fixture.Content, func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.InspectContextParentFrameCurrentV1(context.Background(), fixture.Source); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("pointer switch must fail closed, got %v", err)
	}
}

func TestParentFrameCurrentReaderRejectsReferenceStoreChange(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	fixture, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	content := &corruptingReferenceStoreV1{base: fixture.Content, target: fixture.Frame.StablePrefix}
	reader, err := kernel.NewParentFrameCurrentReaderV1(fixture.Metadata, fixture.Metadata, fixture.Metadata, fixture.Metadata, fixture.Metadata, content, func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.InspectContextParentFrameCurrentV1(context.Background(), fixture.Source); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("changed content must fail closed, got %v", err)
	}
}

func TestParentFrameCurrentReaderRejectsTTLCrossing(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	var calls atomic.Int32
	clock := func() time.Time {
		if calls.Add(1) == 1 {
			return now
		}
		return now.Add(12 * time.Second)
	}
	fixture, err := testfixture.NewParentFrameFixtureV1(clock, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Reader.InspectContextParentFrameCurrentV1(context.Background(), fixture.Source); !errors.Is(err, contract.ErrExpired) {
		t.Fatalf("TTL boundary crossing must fail closed, got %v", err)
	}
}

type switchingPointerReaderV1 struct {
	base  *testkit.MetadataStoreV1
	next  contract.ContextGenerationCurrentPointerV1
	calls atomic.Int32
}

func (r *switchingPointerReaderV1) InspectCurrentGenerationPointer(ctx context.Context, request contract.ContextGenerationCurrentPointerRequestV1) (contract.ContextGenerationCurrentPointerV1, error) {
	pointer, err := r.base.InspectCurrentGenerationPointer(ctx, request)
	if err == nil && r.calls.Add(1) == 1 {
		if putErr := r.base.PutCurrentGenerationPointer(r.next); putErr != nil {
			return contract.ContextGenerationCurrentPointerV1{}, putErr
		}
	}
	return pointer, err
}

type corruptingReferenceStoreV1 struct {
	base   *testkit.MutableReferenceStoreV1
	target contract.ContentRef
	reads  atomic.Int32
}

func (s *corruptingReferenceStoreV1) Put(value []byte) (contract.ContentRef, error) {
	return s.base.Put(value)
}

func (s *corruptingReferenceStoreV1) Get(ref contract.ContentRef) ([]byte, error) {
	if s.reads.Add(1) == 5 {
		if err := s.base.Corrupt(s.target, []byte("changed stable prefix")); err != nil {
			return nil, err
		}
	}
	return s.base.Get(ref)
}

package modelinvokeradapter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/modelinvokeradapter"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestContextOwnerBindingAdapterBlackBoxUsesAuthoritativeContextLineageV1(t *testing.T) {
	fixture, materials, frames, adapter := authoritativeAdapterFixtureV1(t)
	request := modelRequestFromContextV1(t, fixture.Request)
	projection, err := adapter.InspectCurrentInvocationContextOwnerBindingV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Material.ID != fixture.Material.Ref.ID ||
		projection.Material.Revision != core.Revision(fixture.Material.Ref.Revision) ||
		projection.Material.Digest != core.Digest(fixture.Material.Ref.Digest) ||
		projection.Frame.ID != fixture.Material.FrameRef.ID ||
		projection.Frame.Revision != core.Revision(fixture.Material.FrameRef.Revision) ||
		projection.Frame.Digest != core.Digest(fixture.Material.FrameRef.Digest) ||
		projection.ContextOwner.ComponentID != fixture.Owner.ComponentID ||
		projection.ContextOwner.BindingDigest != core.Digest(fixture.Owner.BindingDigest) {
		t.Fatalf("authoritative Context chain was not preserved: %+v", projection)
	}
	if exact, current := materials.CallsV1(); exact != 4 || current != 4 || frames.CallsV1() != 4 {
		t.Fatalf("adapter did not execute two complete Context lineage reads: exact=%d current=%d frame=%d", exact, current, frames.CallsV1())
	}
}

func TestContextOwnerBindingAdapterBlackBoxRejectsHistoryOnlyAndCurrentFalseV1(t *testing.T) {
	t.Run("history only", func(t *testing.T) {
		fixture, materials, frames, adapter := authoritativeAdapterFixtureV1(t)
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
		projection, inspectErr := adapter.InspectCurrentInvocationContextOwnerBindingV1(
			context.Background(),
			modelRequestFromContextV1(t, fixture.Request),
		)
		if !errors.Is(inspectErr, contract.ErrConflict) ||
			projection != (modelinvoker.InvocationContextOwnerBindingProjectionV1{}) ||
			frames.CallsV1() != 0 {
			t.Fatalf("history-only material accepted: projection=%+v frame_calls=%d err=%v", projection, frames.CallsV1(), inspectErr)
		}
	})
	t.Run("frame current false", func(t *testing.T) {
		fixture, _, frames, adapter := authoritativeAdapterFixtureV1(t)
		notCurrent := fixture.Frame
		notCurrent.Current = false
		notCurrent.Digest = ""
		frames.Sequence = []contract.ContextFrameExactCurrentProjectionV1{notCurrent}
		projection, inspectErr := adapter.InspectCurrentInvocationContextOwnerBindingV1(
			context.Background(),
			modelRequestFromContextV1(t, fixture.Request),
		)
		if !errors.Is(inspectErr, contract.ErrConflict) ||
			projection != (modelinvoker.InvocationContextOwnerBindingProjectionV1{}) ||
			frames.CallsV1() != 1 {
			t.Fatalf("non-current frame accepted: projection=%+v frame_calls=%d err=%v", projection, frames.CallsV1(), inspectErr)
		}
	})
}

func authoritativeAdapterFixtureV1(
	t *testing.T,
) (
	testfixture.ModelInputLineageFixtureV1,
	*testfixture.ModelInputLineageMaterialReaderV1,
	*testfixture.ModelInputLineageFrameReaderV1,
	*modelinvokeradapter.InvocationContextOwnerBindingAdapterV1,
) {
	t.Helper()
	fixture, err := testfixture.NewModelInputLineageFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	materials := testfixture.NewModelInputLineageMaterialReaderV1(fixture.Material)
	frames := testfixture.NewModelInputLineageFrameReaderV1(fixture.Owner, fixture.Frame)
	lineage, err := kernel.NewContextModelInputLineageCurrentReaderV1(
		fixture.Owner,
		materials,
		materials,
		frames,
		func() time.Time { return fixture.Now },
		30*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := modelinvokeradapter.NewInvocationContextOwnerBindingAdapterV1(
		fixture.Owner,
		lineage,
		func() time.Time { return fixture.Now },
	)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, materials, frames, adapter
}

func modelRequestFromContextV1(
	t *testing.T,
	request contract.ContextModelInputLineageCurrentRequestV1,
) modelinvoker.InvocationContextOwnerBindingRequestV1 {
	t.Helper()
	sealed, err := modelinvoker.SealInvocationContextOwnerBindingRequestV1(
		modelinvoker.InvocationContextOwnerBindingRequestV1{
			MaterialLookup: modelinvoker.ContextMaterialLookupV1{
				Kind:     string(request.Source.Kind),
				ID:       request.Source.ID,
				Revision: core.Revision(request.Source.Revision),
				Digest:   core.Digest(request.Source.Digest),
			},
			CheckedUnixNano:  request.CheckedUnixNano,
			NotAfterUnixNano: request.NotAfterUnixNano,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

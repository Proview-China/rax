package failure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestParentFrameCurrentMetadataUnavailableFailsClosedAtEveryStage(t *testing.T) {
	operations := []testkit.MetadataOperationV1{
		testkit.MetadataResolveSourceV1,
		testkit.MetadataReadFrameV1,
		testkit.MetadataReadManifestV1,
		testkit.MetadataReadGenerationV1,
		testkit.MetadataReadPointerV1,
	}
	for _, operation := range operations {
		t.Run(string(operation), func(t *testing.T) {
			now := time.Unix(0, testkit.Now)
			fixture, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			fixture.Metadata.SetUnavailable(operation, true)
			if _, err := fixture.Reader.InspectContextParentFrameCurrentV1(context.Background(), fixture.Source); !errors.Is(err, contract.ErrUnavailable) {
				t.Fatalf("stage %s did not fail unavailable: %v", operation, err)
			}
		})
	}
}

func TestParentFrameCurrentReferenceStoreUnavailableFailsClosed(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	fixture, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Content.SetUnavailable(true)
	if _, err := fixture.Reader.InspectContextParentFrameCurrentV1(context.Background(), fixture.Source); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("content unavailable did not fail closed: %v", err)
	}
}

func TestRuntimeAdapterReturnsNoProjectionOnOwnerFailure(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	source := contract.ContextParentFrameApplicabilitySourceCoordinateV1{
		Kind: contract.ContextParentFrameApplicabilityKindV1, ID: "frame-1", Revision: 1, Digest: testkit.D("source"),
	}
	fact := runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{
		Kind: runtimeports.NamespacedNameV2(source.Kind), ID: source.ID, Revision: core.Revision(source.Revision), Digest: core.Digest(source.Digest),
	}
	tests := []struct {
		name     string
		err      error
		category core.ErrorCategory
	}{
		{"not_found", contract.ErrNotFound, core.ErrorNotFound},
		{"unavailable", contract.ErrUnavailable, core.ErrorUnavailable},
		{"unknown", contract.ErrUnknown, core.ErrorIndeterminate},
		{"canceled", context.Canceled, core.ErrorIndeterminate},
		{"deadline", context.DeadlineExceeded, core.ErrorIndeterminate},
		{"conflict", contract.ErrConflict, core.ErrorConflict},
		{"expired", contract.ErrExpired, core.ErrorPreconditionFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := runtimeadapter.ParentFrameApplicabilityCurrentAdapterV3{Reader: failingCurrentReaderV1{err: tt.err}, Clock: func() time.Time { return now }}
			projection, err := adapter.InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Background(), fact)
			if !core.HasCategory(err, tt.category) {
				t.Fatalf("category=%v err=%v", tt.category, err)
			}
			if projection != (runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}) {
				t.Fatalf("owner failure returned a projection: %+v", projection)
			}
		})
	}
}

func TestParentFrameReaderPreservesCanceledAndDeadline(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	fixture, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	deadline, deadlineCancel := context.WithDeadline(context.Background(), time.Unix(0, 1))
	defer deadlineCancel()
	for _, tt := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"canceled", canceled, context.Canceled},
		{"deadline", deadline, context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			projection, err := fixture.Reader.InspectContextParentFrameCurrentV1(tt.ctx, fixture.Source)
			if !errors.Is(err, tt.want) {
				t.Fatalf("got %v want %v", err, tt.want)
			}
			if projection != (contract.ContextParentFrameCurrentProjectionV1{}) {
				t.Fatalf("canceled reader returned stale projection: %+v", projection)
			}
		})
	}
}

type failingCurrentReaderV1 struct{ err error }

func (r failingCurrentReaderV1) InspectContextParentFrameCurrentV1(context.Context, contract.ContextParentFrameApplicabilitySourceCoordinateV1) (contract.ContextParentFrameCurrentProjectionV1, error) {
	return contract.ContextParentFrameCurrentProjectionV1{}, r.err
}

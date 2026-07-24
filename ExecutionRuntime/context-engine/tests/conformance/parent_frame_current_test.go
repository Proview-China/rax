package conformance_test

import (
	"context"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestParentFrameRuntimeAdapterHasNoMetadataSnapshotSurface(t *testing.T) {
	typeOf := reflect.TypeOf(runtimeadapter.ParentFrameApplicabilityCurrentAdapterV3{})
	if typeOf.NumField() != 2 || typeOf.Field(0).Name != "Reader" || typeOf.Field(1).Name != "Clock" {
		t.Fatalf("adapter surface may retain metadata snapshots: %v", typeOf)
	}
	var _ runtimeports.OperationScopeEvidenceApplicabilityCurrentReaderV3 = runtimeadapter.ParentFrameApplicabilityCurrentAdapterV3{}
}

func TestPublicApplicabilityRefDoesNotBypassContextExactCurrentness(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	fixture, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	drifted := fixture.Source
	drifted.Digest = testkit.D("same-frame-id-different-sealed-subject")
	fact := runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{
		Kind: runtimeports.NamespacedNameV2(drifted.Kind), ID: drifted.ID,
		Revision: core.Revision(drifted.Revision), Digest: core.Digest(drifted.Digest),
	}
	adapter := runtimeadapter.ParentFrameApplicabilityCurrentAdapterV3{Reader: fixture.Reader, Clock: func() time.Time { return now }}
	projection, err := adapter.InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Background(), fact)
	if !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("public ref type-pun must fail owner-current inspection: %v", err)
	}
	if projection.Current || projection.Digest != "" || projection.ExpiresUnixNano != 0 {
		t.Fatalf("failed inspection emitted current authority: %+v", projection)
	}
}

func TestContextCurrentProjectionCarriesNoEvidenceOrExecutionAuthority(t *testing.T) {
	typeOf := reflect.TypeOf(contract.ContextParentFrameCurrentProjectionV1{})
	for index := 0; index < typeOf.NumField(); index++ {
		field := strings.ToLower(typeOf.Field(index).Name)
		for _, forbidden := range []string{"evidence", "permit", "watermark", "provider", "domainresult", "settlement", "continuation"} {
			if strings.Contains(field, forbidden) {
				t.Fatalf("Context current projection contains forbidden authority field %s", typeOf.Field(index).Name)
			}
		}
	}
}

func TestCanceledDeadlineAndUnknownProduceNoCurrentProjectionOrOwnerReadLeak(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	source := contract.ContextParentFrameApplicabilitySourceCoordinateV1{
		Kind: contract.ContextParentFrameApplicabilityKindV1, ID: "frame-1", Revision: 1, Digest: testkit.D("source"),
	}
	fact := runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{
		Kind: runtimeports.NamespacedNameV2(source.Kind), ID: source.ID,
		Revision: core.Revision(source.Revision), Digest: core.Digest(source.Digest),
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	deadline, deadlineCancel := context.WithDeadline(context.Background(), time.Unix(0, 1))
	defer deadlineCancel()
	tests := []struct {
		name      string
		ctx       context.Context
		readerErr error
		wantCalls int32
	}{
		{"canceled", canceled, nil, 0},
		{"deadline", deadline, nil, 0},
		{"owner_unknown", context.Background(), contract.ErrUnknown, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &countingFailureReaderV1{err: tt.readerErr}
			adapter := runtimeadapter.ParentFrameApplicabilityCurrentAdapterV3{Reader: reader, Clock: func() time.Time { return now }}
			projection, err := adapter.InspectOperationScopeEvidenceApplicabilityCurrentV3(tt.ctx, fact)
			if !core.HasCategory(err, core.ErrorIndeterminate) {
				t.Fatalf("unknown outcome lost indeterminate classification: %v", err)
			}
			if reader.calls.Load() != tt.wantCalls {
				t.Fatalf("owner reads=%d want=%d", reader.calls.Load(), tt.wantCalls)
			}
			if projection != (runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}) {
				t.Fatalf("unknown outcome returned stale projection: %+v", projection)
			}
		})
	}
}

type countingFailureReaderV1 struct {
	err   error
	calls atomic.Int32
}

func (r *countingFailureReaderV1) InspectContextParentFrameCurrentV1(context.Context, contract.ContextParentFrameApplicabilitySourceCoordinateV1) (contract.ContextParentFrameCurrentProjectionV1, error) {
	r.calls.Add(1)
	return contract.ContextParentFrameCurrentProjectionV1{}, r.err
}

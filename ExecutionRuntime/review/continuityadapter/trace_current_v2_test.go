package continuityadapter

import (
	"context"
	"testing"
	"time"

	continuitycontract "github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	continuityports "github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	reviewsqlite "github.com/Proview-China/rax/ExecutionRuntime/review/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var (
	_ TraceCurrentSourceV2 = (*memory.Store)(nil)
	_ TraceCurrentSourceV2 = (*reviewsqlite.Store)(nil)
)

func TestTraceCurrentReaderV2ExactTenantScopeAndFreshReread(t *testing.T) {
	source, request := traceCurrentFixtureV2(t)
	reader, err := NewTraceCurrentReaderV2(source)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := reader.InspectTimelineOwnerCurrentV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Fact != request.Fact || projection.CheckedUnixNano != source.trace.UpdatedUnixNano || projection.ExpiresUnixNano != source.target.ExpiresUnixNano || projection.Digest == "" {
		t.Fatalf("projection lost exact Review current coordinates: %+v", projection)
	}
	if err := reader.ValidateTimelineOwnerCurrentV2(context.Background(), request, projection); err != nil {
		t.Fatal(err)
	}
	if source.traceReads != 2 || source.targetReads != 2 || source.targetCurrentReads != 2 {
		t.Fatalf("S1/fresh validation must re-read both exact and current facts: trace=%d target=%d current=%d", source.traceReads, source.targetReads, source.targetCurrentReads)
	}
}

func TestTraceCurrentReaderV2HardNegatives(t *testing.T) {
	tests := []struct {
		name string
		edit func(*traceCurrentSourceV2, *continuityports.TimelineOwnerCurrentInspectRequestV2)
		code continuitycontract.ErrorCode
	}{
		{
			name: "cross tenant",
			edit: func(_ *traceCurrentSourceV2, request *continuityports.TimelineOwnerCurrentInspectRequestV2) {
				request.TenantID = "tenant-b"
			},
			code: continuitycontract.ErrNotFound,
		},
		{
			name: "scope splice",
			edit: func(_ *traceCurrentSourceV2, request *continuityports.TimelineOwnerCurrentInspectRequestV2) {
				request.Fact.ScopeDigest = string(testkit.Digest("another-scope"))
			},
			code: continuitycontract.ErrProjectionConflict,
		},
		{
			name: "owner component type pun",
			edit: func(_ *traceCurrentSourceV2, request *continuityports.TimelineOwnerCurrentInspectRequestV2) {
				request.Fact.Owner.ComponentID = "components/other"
			},
			code: continuitycontract.ErrUnsupported,
		},
		{
			name: "owner capability type pun",
			edit: func(_ *traceCurrentSourceV2, request *continuityports.TimelineOwnerCurrentInspectRequestV2) {
				request.Fact.Owner.Capability = "praxis.review/verdict-current"
			},
			code: continuitycontract.ErrUnsupported,
		},
		{
			name: "payload type pun",
			edit: func(_ *traceCurrentSourceV2, request *continuityports.TimelineOwnerCurrentInspectRequestV2) {
				request.Fact.PayloadSchema = "praxis.review/verdict@1.0.0;application/json;" + string(testkit.Digest("schema"))
			},
			code: continuitycontract.ErrProjectionConflict,
		},
		{
			name: "payload schema digest drift",
			edit: func(_ *traceCurrentSourceV2, request *continuityports.TimelineOwnerCurrentInspectRequestV2) {
				request.Fact.PayloadSchema = (runtimeports.SchemaRefV2{
					Namespace: "praxis.review", Name: "trace-fact", Version: "1.0.0",
					MediaType: "application/json", ContentDigest: testkit.Digest("different-trace-schema"),
				}).Key()
			},
			code: continuitycontract.ErrProjectionConflict,
		},
		{
			name: "fact kind type pun",
			edit: func(_ *traceCurrentSourceV2, request *continuityports.TimelineOwnerCurrentInspectRequestV2) {
				request.Fact.FactKind = "praxis.review/verdict-v1"
				request.Fact.Owner.FactKind = request.Fact.FactKind
			},
			code: continuitycontract.ErrUnsupported,
		},
		{
			name: "payload digest drift",
			edit: func(_ *traceCurrentSourceV2, request *continuityports.TimelineOwnerCurrentInspectRequestV2) {
				request.Fact.PayloadDigest = string(testkit.Digest("another-payload"))
			},
			code: continuitycontract.ErrProjectionConflict,
		},
		{
			name: "target exact drift",
			edit: func(source *traceCurrentSourceV2, _ *continuityports.TimelineOwnerCurrentInspectRequestV2) {
				source.target = testkit.Target(time.Unix(1_760_000_100, 0))
			},
			code: continuitycontract.ErrProjectionConflict,
		},
		{
			name: "target current superseded",
			edit: func(source *traceCurrentSourceV2, _ *continuityports.TimelineOwnerCurrentInspectRequestV2) {
				next := source.target
				next.Revision++
				next.UpdatedUnixNano++
				next.Digest = ""
				var err error
				source.currentTarget, err = reviewcontract.SealTargetSnapshotV1(next)
				if err != nil {
					panic(err)
				}
			},
			code: continuitycontract.ErrProjectionConflict,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, request := traceCurrentFixtureV2(t)
			tt.edit(source, &request)
			reader, err := NewTraceCurrentReaderV2(source)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = reader.InspectTimelineOwnerCurrentV2(context.Background(), request); !continuitycontract.HasCode(err, tt.code) {
				t.Fatalf("want %s, got %v", tt.code, err)
			}
		})
	}
}

func TestTraceCurrentReaderV2ValidationDetectsDriftAndPreservesCancellation(t *testing.T) {
	source, request := traceCurrentFixtureV2(t)
	reader, _ := NewTraceCurrentReaderV2(source)
	projection, err := reader.InspectTimelineOwnerCurrentV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	source.target.ExpiresUnixNano--
	source.target, err = reviewcontract.SealTargetSnapshotV1(source.target)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.ValidateTimelineOwnerCurrentV2(context.Background(), request, projection); !continuitycontract.HasCode(err, continuitycontract.ErrProjectionConflict) {
		t.Fatalf("fresh target drift must fail closed: %v", err)
	}

	source, request = traceCurrentFixtureV2(t)
	reader, _ = NewTraceCurrentReaderV2(source)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.InspectTimelineOwnerCurrentV2(ctx, request); !continuitycontract.HasCode(err, continuitycontract.ErrIndeterminate) {
		t.Fatalf("cancellation must remain indeterminate: %v", err)
	}
}

func TestTraceCurrentReaderV2RejectsTypedNil(t *testing.T) {
	var source *traceCurrentSourceV2
	if _, err := NewTraceCurrentReaderV2(source); !continuitycontract.HasCode(err, continuitycontract.ErrInvalidArgument) {
		t.Fatalf("typed nil source must fail closed: %v", err)
	}
}

func TestTraceCurrentReaderV2WorksOnlyThroughExactClosedRoute(t *testing.T) {
	source, request := traceCurrentFixtureV2(t)
	reader, err := NewTraceCurrentReaderV2(source)
	if err != nil {
		t.Fatal(err)
	}
	router, err := continuityports.NewClosedTimelineTypedOwnerRouterV2([]continuityports.TimelineTypedOwnerRouteV2{{
		OwnerComponentID: TraceOwnerComponentIDV1,
		Capability:       TraceOwnerCapabilityV1,
		FactKind:         TraceOwnerFactKindV1,
		PayloadSchema:    TracePayloadSchemaIdentityV1(),
		Reader:           reader,
	}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := router.ReaderForTimelineOwnerV2(request)
	if err != nil || got != reader {
		t.Fatalf("exact Review route missing: got=%T err=%v", got, err)
	}
	request.Fact.PayloadSchema = (runtimeports.SchemaRefV2{
		Namespace: "praxis.review", Name: "trace-fact", Version: "1.0.0",
		MediaType: "application/json", ContentDigest: testkit.Digest("route-schema-drift"),
	}).Key()
	if _, err := router.ReaderForTimelineOwnerV2(request); !continuitycontract.HasCode(err, continuitycontract.ErrUnsupported) {
		t.Fatalf("schema-drift route must fail closed: %v", err)
	}
}

type traceCurrentSourceV2 struct {
	trace              reviewcontract.TraceFactV1
	target             reviewcontract.TargetSnapshotV1
	currentTarget      reviewcontract.TargetSnapshotV1
	traceReads         int
	targetReads        int
	targetCurrentReads int
}

func (s *traceCurrentSourceV2) InspectTraceExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (reviewcontract.TraceFactV1, error) {
	if err := ctx.Err(); err != nil {
		return reviewcontract.TraceFactV1{}, err
	}
	s.traceReads++
	if tenant != s.trace.TenantID || ref.ID != s.trace.ID || ref.Revision != s.trace.Revision || ref.Digest != s.trace.Digest {
		return reviewcontract.TraceFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "trace not found")
	}
	return s.trace, nil
}

func (s *traceCurrentSourceV2) ListTracePageV2(context.Context, reviewport.ListTracePageRequestV2) (reviewport.ListTracePageResultV2, error) {
	return reviewport.ListTracePageResultV2{}, nil
}

func (s *traceCurrentSourceV2) InspectTargetExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (reviewcontract.TargetSnapshotV1, error) {
	if err := ctx.Err(); err != nil {
		return reviewcontract.TargetSnapshotV1{}, err
	}
	s.targetReads++
	if tenant != s.target.TenantID {
		return reviewcontract.TargetSnapshotV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "target not found")
	}
	// Returning a valid but different target lets the adapter prove that it
	// checks exact target coordinates rather than trusting this test source.
	return s.target, nil
}

func (s *traceCurrentSourceV2) InspectTargetV1(ctx context.Context, tenant core.TenantID, id string) (reviewcontract.TargetSnapshotV1, error) {
	if err := ctx.Err(); err != nil {
		return reviewcontract.TargetSnapshotV1{}, err
	}
	s.targetCurrentReads++
	if tenant != s.currentTarget.TenantID || id != s.currentTarget.ID {
		return reviewcontract.TargetSnapshotV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "current target not found")
	}
	return s.currentTarget, nil
}

func traceCurrentFixtureV2(t *testing.T) (*traceCurrentSourceV2, continuityports.TimelineOwnerCurrentInspectRequestV2) {
	t.Helper()
	now := time.Unix(1_760_000_000, 0)
	target := testkit.Target(now)
	trace := testkit.TraceForTarget(now.Add(time.Second), "case-timeline", target, reviewcontract.TraceFindingV1, 44, "finding-a")
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(target.Scope)
	if err != nil {
		t.Fatal(err)
	}
	fact := continuitycontract.TimelineOwnerFactRefV1{
		Owner: continuitycontract.OwnerBinding{
			BindingSetID: "binding-review", BindingRevision: 1,
			ComponentID: TraceOwnerComponentIDV1, ManifestDigest: string(testkit.Digest("manifest-review")),
			ArtifactDigest: string(testkit.Digest("artifact-review")), Capability: TraceOwnerCapabilityV1,
			FactKind: TraceOwnerFactKindV1,
		},
		FactKind: TraceOwnerFactKindV1, FactID: trace.ID, Revision: uint64(trace.Revision),
		FactDigest: string(trace.Digest), PayloadSchema: TracePayloadSchemaIdentityV1(), PayloadDigest: string(trace.Digest),
		PayloadRevision: uint64(trace.Revision), ScopeDigest: string(scopeDigest),
	}
	request := continuityports.TimelineOwnerCurrentInspectRequestV2{TenantID: target.TenantID, Fact: fact}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	return &traceCurrentSourceV2{trace: trace, target: target, currentTarget: target}, request
}

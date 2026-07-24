// Package continuityadapter exposes Review-owned facts through Continuity's
// narrow typed-owner current Reader. It never publishes Timeline/Evidence,
// allocates a sequence, or turns a Review Trace into a Continuity fact.
package continuityadapter

import (
	"context"
	"errors"
	"reflect"

	continuitycontract "github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	continuityports "github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	TraceOwnerComponentIDV1  = "components/review"
	TraceOwnerCapabilityV1   = "praxis.review/trace-current"
	TraceOwnerFactKindV1     = "praxis.review/trace-fact-v1"
	traceProjectionVersionV2 = "praxis.review/continuity-trace-current-v2"
)

// TracePayloadSchemaIdentityV1 returns the complete immutable schema identity,
// including its schema document digest. It returns a value rather than a
// mutable package global.
func TracePayloadSchemaIdentityV1() string {
	return reviewcontract.TraceFactSchemaRefV1().Key()
}

// TraceCurrentSourceV2 is deliberately read-only and narrower than StoreV1.
// Target is re-read to prove the Trace's tenant and execution scope; neither
// method exposes a Review mutation.
type TraceCurrentSourceV2 interface {
	reviewport.TraceEventReaderV2
	InspectTargetV1(context.Context, core.TenantID, string) (reviewcontract.TargetSnapshotV1, error)
	InspectTargetExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (reviewcontract.TargetSnapshotV1, error)
}

type TraceCurrentReaderV2 struct {
	source TraceCurrentSourceV2
}

func NewTraceCurrentReaderV2(source TraceCurrentSourceV2) (*TraceCurrentReaderV2, error) {
	if nilOrTypedNil(source) {
		return nil, continuitycontract.NewError(continuitycontract.ErrInvalidArgument, "review_trace_source", "exact Review source is required")
	}
	return &TraceCurrentReaderV2{source: source}, nil
}

func (r *TraceCurrentReaderV2) InspectTimelineOwnerCurrentV2(ctx context.Context, request continuityports.TimelineOwnerCurrentInspectRequestV2) (continuityports.TimelineOwnerCurrentProjectionV1, error) {
	if r == nil || nilOrTypedNil(r.source) {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, continuitycontract.NewError(continuitycontract.ErrUnavailable, "review_trace_source", "exact Review source is unavailable")
	}
	if err := request.Validate(); err != nil {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, err
	}
	if request.Fact.Owner.ComponentID != TraceOwnerComponentIDV1 ||
		request.Fact.Owner.Capability != TraceOwnerCapabilityV1 ||
		request.Fact.FactKind != TraceOwnerFactKindV1 ||
		request.Fact.Owner.FactKind != TraceOwnerFactKindV1 {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, continuitycontract.NewError(continuitycontract.ErrUnsupported, "fact_kind", "Review adapter only supports exact Trace facts")
	}
	if request.Fact.PayloadSchema != TracePayloadSchemaIdentityV1() {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, continuitycontract.NewError(continuitycontract.ErrProjectionConflict, "payload_schema", "Trace payload schema identity drifted")
	}

	trace, err := r.source.InspectTraceExactV1(ctx, request.TenantID, reviewport.ExactV1(request.Fact.FactID, core.Revision(request.Fact.Revision), core.Digest(request.Fact.FactDigest)))
	if err != nil {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, mapReviewError(err, "trace")
	}
	if err := trace.Validate(); err != nil {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, mapReviewError(err, "trace")
	}
	if trace.TenantID != request.TenantID || trace.ID != request.Fact.FactID || uint64(trace.Revision) != request.Fact.Revision || string(trace.Digest) != request.Fact.FactDigest || request.Fact.PayloadDigest != string(trace.Digest) || request.Fact.PayloadRevision != uint64(trace.Revision) {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, continuitycontract.NewError(continuitycontract.ErrProjectionConflict, "trace", "Owner ref does not bind the exact Review Trace")
	}

	target, err := r.source.InspectTargetExactV1(ctx, request.TenantID, reviewport.ExactV1(trace.TargetID, trace.TargetRevision, trace.TargetDigest))
	if err != nil {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, mapReviewError(err, "target")
	}
	if err := target.Validate(); err != nil {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, mapReviewError(err, "target")
	}
	if target.TenantID != request.TenantID || target.ID != trace.TargetID || target.Revision != trace.TargetRevision || target.Digest != trace.TargetDigest {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, continuitycontract.NewError(continuitycontract.ErrProjectionConflict, "target", "Trace target exact binding drifted")
	}
	currentTarget, err := r.source.InspectTargetV1(ctx, request.TenantID, trace.TargetID)
	if err != nil {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, mapReviewError(err, "target_current")
	}
	if err := currentTarget.Validate(); err != nil {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, mapReviewError(err, "target_current")
	}
	if currentTarget.ID != target.ID || currentTarget.Revision != target.Revision || currentTarget.Digest != target.Digest {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, continuitycontract.NewError(continuitycontract.ErrProjectionConflict, "target_current", "Review Trace target is no longer current")
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(target.Scope)
	if err != nil {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, mapReviewError(err, "scope")
	}
	if string(scopeDigest) != request.Fact.ScopeDigest {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, continuitycontract.NewError(continuitycontract.ErrProjectionConflict, "scope_digest", "Trace target belongs to another execution scope")
	}

	checked := trace.UpdatedUnixNano
	if target.UpdatedUnixNano > checked {
		checked = target.UpdatedUnixNano
	}
	expires := target.ExpiresUnixNano
	if expires <= checked {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, continuitycontract.NewError(continuitycontract.ErrPreconditionFailed, "trace_current", "Review Trace current window is invalid")
	}
	projection := continuityports.TimelineOwnerCurrentProjectionV1{
		Fact: request.Fact, CheckedUnixNano: checked, ExpiresUnixNano: expires,
	}
	digest, err := core.CanonicalJSONDigest("praxis.review.continuity-adapter", traceProjectionVersionV2, "TimelineOwnerCurrentProjectionV1", projection)
	if err != nil {
		return continuityports.TimelineOwnerCurrentProjectionV1{}, mapReviewError(err, "projection_digest")
	}
	projection.Digest = string(digest)
	return projection, nil
}

func (r *TraceCurrentReaderV2) ValidateTimelineOwnerCurrentV2(ctx context.Context, request continuityports.TimelineOwnerCurrentInspectRequestV2, expected continuityports.TimelineOwnerCurrentProjectionV1) error {
	actual, err := r.InspectTimelineOwnerCurrentV2(ctx, request)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) {
		return continuitycontract.NewError(continuitycontract.ErrProjectionConflict, "trace_current", "fresh exact Review Trace projection drifted")
	}
	return nil
}

func nilOrTypedNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func mapReviewError(err error, field string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return continuitycontract.NewError(continuitycontract.ErrIndeterminate, field, "Review exact Inspect was interrupted")
	}
	switch {
	case core.HasCategory(err, core.ErrorInvalidArgument):
		return continuitycontract.NewError(continuitycontract.ErrInvalidArgument, field, "Review exact fact is invalid")
	case core.HasCategory(err, core.ErrorNotFound):
		return continuitycontract.NewError(continuitycontract.ErrNotFound, field, "Review exact fact was not found")
	case core.HasCategory(err, core.ErrorConflict):
		return continuitycontract.NewError(continuitycontract.ErrProjectionConflict, field, "Review exact fact conflicts")
	case core.HasCategory(err, core.ErrorPreconditionFailed), core.HasCategory(err, core.ErrorForbidden), core.HasCategory(err, core.ErrorUnauthenticated):
		return continuitycontract.NewError(continuitycontract.ErrPreconditionFailed, field, "Review exact fact is not current")
	case core.HasCategory(err, core.ErrorCapabilityUnavailable):
		return continuitycontract.NewError(continuitycontract.ErrUnsupported, field, "Review exact Reader capability is unavailable")
	case core.HasCategory(err, core.ErrorUnavailable), core.HasCategory(err, core.ErrorRateLimited):
		return continuitycontract.NewError(continuitycontract.ErrUnavailable, field, "Review exact Reader is unavailable")
	default:
		return continuitycontract.NewError(continuitycontract.ErrIndeterminate, field, "Review exact Inspect outcome is indeterminate")
	}
}

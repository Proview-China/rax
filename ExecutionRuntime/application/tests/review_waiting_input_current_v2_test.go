package application_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestReviewWaitingInputExactCurrentReaderV2CompileShape(t *testing.T) {
	reader := reflect.TypeOf((*applicationports.ReviewWaitingInputExactCurrentReaderV2)(nil)).Elem()
	if reader.NumMethod() != 1 {
		t.Fatalf("V2 Reader exposes %d methods, want one exact read", reader.NumMethod())
	}
	method, ok := reader.MethodByName("InspectReviewWaitingInputExactCurrentV2")
	if !ok || method.Type.NumIn() != 2 || method.Type.NumOut() != 2 || method.Type.In(1) != reflect.TypeOf(contract.ReviewWaitingInputCurrentRequestV2{}) || method.Type.Out(0) != reflect.TypeOf(contract.ReviewWaitingInputCurrentProjectionV2{}) || method.Type.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Fatalf("V2 Reader exact signature drifted: %v", method.Type)
	}
	for _, mutation := range []string{"Create", "Publish", "CompareAndSwap", "Register", "Delete"} {
		if _, found := reader.MethodByName(mutation + "ReviewWaitingInputCurrentV2"); found {
			t.Fatalf("V2 Reader exposes mutation %s", mutation)
		}
	}
}

func TestReviewWaitingInputExactCurrentReaderV2ClosedErrors(t *testing.T) {
	for _, category := range []core.ErrorCategory{core.ErrorInvalidArgument, core.ErrorNotFound, core.ErrorConflict, core.ErrorPreconditionFailed, core.ErrorCapabilityUnavailable, core.ErrorUnavailable, core.ErrorIndeterminate} {
		err := core.NewError(category, core.ReasonInvalidReference, "closed error")
		if !applicationports.IsReviewWaitingInputExactCurrentClosedErrorV2(err) {
			t.Fatalf("closed category %s was rejected", category)
		}
	}
	for _, category := range []core.ErrorCategory{core.ErrorUnauthenticated, core.ErrorForbidden, core.ErrorRateLimited, core.ErrorInternal} {
		err := core.NewError(category, core.ReasonInvalidReference, "open error")
		if applicationports.IsReviewWaitingInputExactCurrentClosedErrorV2(err) {
			t.Fatalf("open category %s was accepted", category)
		}
	}
	if applicationports.IsReviewWaitingInputExactCurrentClosedErrorV2(nil) {
		t.Fatal("nil error was accepted as a closed failure")
	}
}

func TestReviewWaitingInputExactCurrentReaderV2Conformance(t *testing.T) {
	now, current, expected := reviewWaitingInputConformanceFixtureV2(t)
	missing := reviewWaitingInputRequestVariantV2(t, current, "missing-source", "missing", now.Add(20*time.Minute))
	drift := reviewWaitingInputRequestVariantV2(t, current, "drift-source", "drift", now.Add(20*time.Minute))
	expired := reviewWaitingInputRequestVariantV2(t, current, "expired-source", "expired", now.Add(-time.Second))
	typePun := current.Clone()
	typePun.Source.Kind = contract.ReviewPhaseRunV1
	typePun, _ = contract.SealReviewWaitingInputCurrentRequestV2(typePun)
	reader := &reviewWaitingInputReaderFixtureV2{now: now, current: current, projection: expected, missingID: missing.Source.ID, driftID: drift.Source.ID}
	report, err := conformance.CheckReviewWaitingInputExactCurrentReaderV2(context.Background(), conformance.ReviewWaitingInputCurrentCaseV2{Reader: reader, Current: current, Expected: expected, Missing: missing, TypePun: typePun, Drift: drift, Expired: expired, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !report.ExactRead || !report.S1S2Stable || !report.DeepClone || !report.MissingFailClosed || !report.TypePunFailClosed || !report.DriftFailClosed || !report.TTLFailClosed || !report.ClosedErrorSet || report.ProductionEligible {
		t.Fatalf("incomplete or overstated V2 conformance report: %+v", report)
	}
}

func TestReviewWaitingInputExactCurrentReaderV2ConformanceRejectsTypedNilAndOpenError(t *testing.T) {
	now, current, expected := reviewWaitingInputConformanceFixtureV2(t)
	var typedNil *reviewWaitingInputReaderFixtureV2
	if _, err := conformance.CheckReviewWaitingInputExactCurrentReaderV2(context.Background(), conformance.ReviewWaitingInputCurrentCaseV2{Reader: typedNil, Now: now}); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil Reader accepted: %v", err)
	}
	if _, err := conformance.CheckReviewWaitingInputExactCurrentReaderV2(nil, conformance.ReviewWaitingInputCurrentCaseV2{}); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context accepted: %v", err)
	}
	missing := reviewWaitingInputRequestVariantV2(t, current, "open-error-source", "missing", now.Add(20*time.Minute))
	typePun := current.Clone()
	typePun.Source.Kind = contract.ReviewPhaseRunV1
	typePun, _ = contract.SealReviewWaitingInputCurrentRequestV2(typePun)
	drift := reviewWaitingInputRequestVariantV2(t, current, "drift-open-source", "drift", now.Add(20*time.Minute))
	expired := reviewWaitingInputRequestVariantV2(t, current, "expired-open-source", "expired", now.Add(-time.Second))
	reader := &reviewWaitingInputReaderFixtureV2{now: now, current: current, projection: expected, missingID: missing.Source.ID, driftID: drift.Source.ID, openMissing: true}
	_, err := conformance.CheckReviewWaitingInputExactCurrentReaderV2(context.Background(), conformance.ReviewWaitingInputCurrentCaseV2{Reader: reader, Current: current, Expected: expected, Missing: missing, TypePun: typePun, Drift: drift, Expired: expired, Now: now})
	if err == nil {
		t.Fatal("conformance accepted an open Internal error")
	}
}

type reviewWaitingInputReaderFixtureV2 struct {
	now         time.Time
	current     contract.ReviewWaitingInputCurrentRequestV2
	projection  contract.ReviewWaitingInputCurrentProjectionV2
	missingID   string
	driftID     string
	openMissing bool
}

func (r *reviewWaitingInputReaderFixtureV2) InspectReviewWaitingInputExactCurrentV2(ctx context.Context, request contract.ReviewWaitingInputCurrentRequestV2) (contract.ReviewWaitingInputCurrentProjectionV2, error) {
	if ctx == nil {
		return contract.ReviewWaitingInputCurrentProjectionV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context is required")
	}
	if err := request.ValidateCurrent(r.now); err != nil {
		return contract.ReviewWaitingInputCurrentProjectionV2{}, err
	}
	if request.Source.ID == r.missingID {
		if r.openMissing {
			return contract.ReviewWaitingInputCurrentProjectionV2{}, core.NewError(core.ErrorInternal, core.ReasonInvalidState, "open test error")
		}
		return contract.ReviewWaitingInputCurrentProjectionV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "exact source is absent")
	}
	if request.Source.ID == r.driftID {
		return contract.ReviewWaitingInputCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "source drifted")
	}
	if request != r.current {
		return contract.ReviewWaitingInputCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "request drifted")
	}
	return r.projection.Clone(), nil
}

func reviewWaitingInputConformanceFixtureV2(t testing.TB) (time.Time, contract.ReviewWaitingInputCurrentRequestV2, contract.ReviewWaitingInputCurrentProjectionV2) {
	t.Helper()
	now := time.Unix(2_100_100_000, 0)
	tenant, run := core.TenantID("tenant-review-input-conformance"), core.AgentRunID("run-review-input-conformance")
	phase := contract.ReviewPhasePointCoordinateV1{Kind: contract.ReviewPhaseActionV1, ID: "phase-review-input-conformance", Revision: 2, Digest: core.DigestBytes([]byte("phase-review-input-conformance")), CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	target := contract.ReviewWaitingTargetCoordinateV1{TenantID: tenant, ID: "target-review-input-conformance", Revision: 3, Digest: core.DigestBytes([]byte("target-review-input-conformance")), RunID: run, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(50 * time.Minute).UnixNano()}
	request, err := contract.SealReviewWaitingInputCurrentRequestV2(contract.ReviewWaitingInputCurrentRequestV2{Subject: contract.ReviewWaitingInputSubjectV1{TenantID: tenant, RunID: run, PhaseKind: phase.Kind, PhaseID: phase.ID}, Source: contract.ReviewWaitingInputSourceRefV2{ContractVersion: contract.ReviewWaitingInputSourceContractVersionV1, Kind: phase.Kind, ID: "source-review-input-conformance", Revision: 5, Digest: core.DigestBytes([]byte("source-review-input-conformance")), NotAfterUnixNano: now.Add(40 * time.Minute).UnixNano()}, ExpectedSourceClosureDigest: core.DigestBytes([]byte("source-closure-review-input-conformance")), ExpectedPhase: phase, ExpectedTarget: target, ExecutionScopeDigest: core.DigestBytes([]byte("scope-review-input-conformance"))})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := contract.SealReviewWaitingInputCurrentProjectionV2(contract.ReviewWaitingInputCurrentProjectionV2{CheckedUnixNano: now.UnixNano()}, request, now)
	if err != nil {
		t.Fatal(err)
	}
	return now, request, projection
}

func reviewWaitingInputRequestVariantV2(t testing.TB, base contract.ReviewWaitingInputCurrentRequestV2, id, digest string, notAfter time.Time) contract.ReviewWaitingInputCurrentRequestV2 {
	t.Helper()
	request := base.Clone()
	request.Source.ID = id
	request.Source.Digest = core.DigestBytes([]byte(digest))
	request.Source.NotAfterUnixNano = notAfter.UnixNano()
	request, err := contract.SealReviewWaitingInputCurrentRequestV2(request)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

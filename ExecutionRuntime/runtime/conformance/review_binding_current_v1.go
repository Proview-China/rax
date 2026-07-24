package conformance

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ReviewBindingAuthoritativeCurrentCaseV1 exercises the Owner-local contract.
// PrepareCAS is a fixture hook that prepares the Binding Owner's compound
// association/projection mutation; it is never part of the public Reader or
// production composition surface.
type ReviewBindingAuthoritativeCurrentCaseV1 struct {
	Reader            ports.ReviewBindingAuthoritativeCurrentReaderV1
	Publisher         ports.ReviewBindingProjectionPublisherV1
	CompoundPublisher control.ReviewBindingAssociationProjectionPublisherV1
	Create            ports.CreateReviewBindingProjectionRequestV1
	PrepareCAS        func(context.Context, ports.ReviewBindingProjectionRefV1) (control.CompareAndSwapReviewBindingAssociationProjectionRequestV1, error)
	BeforeCreate      func()
	Now               func() time.Time
}

type ReviewBindingAuthoritativeCurrentReportV1 struct {
	CreateRecoveredExactly  bool `json:"create_recovered_exactly"`
	ResolvedCurrent         bool `json:"resolved_current"`
	HistoricalImmutable     bool `json:"historical_immutable"`
	ConcurrentCASCalls      int  `json:"concurrent_cas_calls"`
	LogicalCASRevisions     int  `json:"logical_cas_revisions"`
	CurrentClosureObserved  bool `json:"current_closure_observed"`
	MutationRetryObserved   bool `json:"mutation_retry_observed"`
	ProductionClaimEligible bool `json:"production_claim_eligible"`
}

// CheckReviewBindingAuthoritativeCurrentV1 proves canonical read recovery,
// immutable history and one logical CAS revision under 64 concurrent canonical
// replays. Passing never proves production persistence, topology or SLA.
func CheckReviewBindingAuthoritativeCurrentV1(ctx context.Context, testCase ReviewBindingAuthoritativeCurrentCaseV1) (ReviewBindingAuthoritativeCurrentReportV1, error) {
	if isNilReviewBindingConformanceV1(testCase.Reader) || isNilReviewBindingConformanceV1(testCase.Publisher) || isNilReviewBindingConformanceV1(testCase.CompoundPublisher) || testCase.PrepareCAS == nil || testCase.Now == nil {
		return ReviewBindingAuthoritativeCurrentReportV1{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "Review Binding conformance dependencies are incomplete")
	}
	if err := testCase.Create.Validate(); err != nil {
		return ReviewBindingAuthoritativeCurrentReportV1{}, err
	}
	if testCase.BeforeCreate != nil {
		testCase.BeforeCreate()
	}
	report := ReviewBindingAuthoritativeCurrentReportV1{ConcurrentCASCalls: 64, ProductionClaimEligible: false}
	created, err := testCase.Publisher.CreateReviewBindingProjectionV1(ctx, testCase.Create)
	if err != nil {
		if !core.HasCategory(err, core.ErrorIndeterminate) {
			return ReviewBindingAuthoritativeCurrentReportV1{}, err
		}
		created, err = testCase.Publisher.InspectReviewBindingProjectionPublishV1(context.WithoutCancel(ctx), testCase.Create.PublishRef)
		if err != nil {
			return ReviewBindingAuthoritativeCurrentReportV1{}, err
		}
		report.CreateRecoveredExactly = true
	}
	if err := created.Validate(); err != nil || created.PublishRef != testCase.Create.PublishRef {
		return ReviewBindingAuthoritativeCurrentReportV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding Create receipt is not exact")
	}
	resolved, err := testCase.Reader.ResolveCurrentReviewBindingV1(ctx, ports.ResolveReviewBindingCurrentRequestV1{Source: testCase.Create.Input.Source, Subject: testCase.Create.Input.Subject})
	if err != nil || resolved != created.Projection {
		if err != nil {
			return ReviewBindingAuthoritativeCurrentReportV1{}, err
		}
		return ReviewBindingAuthoritativeCurrentReportV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding Resolve returned another current Ref")
	}
	report.ResolvedCurrent = true
	historicalRequest := ports.InspectReviewBindingProjectionRequestV1{Ref: created.Projection, ExpectedSource: testCase.Create.Input.Source, ExpectedSubject: testCase.Create.Input.Subject}
	historical, err := testCase.Reader.InspectReviewBindingProjectionV1(ctx, historicalRequest)
	if err != nil {
		return ReviewBindingAuthoritativeCurrentReportV1{}, err
	}
	if len(historical.Members) == 0 {
		return ReviewBindingAuthoritativeCurrentReportV1{}, core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "Review Binding conformance projection has no member closure")
	}
	historical.Members[0].BindingID = "conformance-mutated-return"
	again, err := testCase.Reader.InspectReviewBindingProjectionV1(ctx, historicalRequest)
	if err != nil || len(again.Members) == 0 || again.Members[0].BindingID == "conformance-mutated-return" {
		if err != nil {
			return ReviewBindingAuthoritativeCurrentReportV1{}, err
		}
		return ReviewBindingAuthoritativeCurrentReportV1{}, core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "Review Binding historical Inspect leaked a mutable alias")
	}
	report.HistoricalImmutable = true

	cas, err := testCase.PrepareCAS(ctx, created.Projection)
	if err != nil {
		return ReviewBindingAuthoritativeCurrentReportV1{}, err
	}
	if err := cas.Validate(); err != nil {
		return ReviewBindingAuthoritativeCurrentReportV1{}, err
	}
	type result struct {
		receipt ports.ReviewBindingProjectionPublishReceiptV1
		err     error
	}
	results := make(chan result, report.ConcurrentCASCalls)
	var wait sync.WaitGroup
	for range report.ConcurrentCASCalls {
		wait.Add(1)
		go func() {
			defer wait.Done()
			receipt, callErr := testCase.CompoundPublisher.CompareAndSwapReviewBindingAssociationProjectionV1(ctx, cas)
			results <- result{receipt: receipt, err: callErr}
		}()
	}
	wait.Wait()
	close(results)
	var canonical ports.ReviewBindingProjectionPublishReceiptV1
	for item := range results {
		if item.err != nil {
			return ReviewBindingAuthoritativeCurrentReportV1{}, item.err
		}
		if canonical.PublishRef.ID == "" {
			canonical = item.receipt
		}
		if item.receipt != canonical {
			return ReviewBindingAuthoritativeCurrentReportV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "concurrent Review Binding CAS returned different receipts")
		}
	}
	if canonical.Projection.Revision != created.Projection.Revision+1 || canonical.HighestRevision != canonical.Projection.Revision {
		return ReviewBindingAuthoritativeCurrentReportV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "concurrent Review Binding CAS advanced more than one logical revision")
	}
	report.LogicalCASRevisions = 1
	current, err := testCase.Reader.InspectCurrentReviewBindingV1(ctx, ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: canonical.Projection, ExpectedSource: cas.Projection.Input.Source, ExpectedSubject: cas.Projection.Input.Subject})
	if err != nil {
		return ReviewBindingAuthoritativeCurrentReportV1{}, err
	}
	if err := current.ValidateCurrent(canonical.Projection, cas.Projection.Input.Source, cas.Projection.Input.Subject, testCase.Now()); err != nil {
		return ReviewBindingAuthoritativeCurrentReportV1{}, err
	}
	report.CurrentClosureObserved = true
	return report, nil
}

func isNilReviewBindingConformanceV1(value any) bool {
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

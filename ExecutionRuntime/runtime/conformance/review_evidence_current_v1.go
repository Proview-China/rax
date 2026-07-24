package conformance

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ReviewEvidenceApplicabilityCurrentCaseV1 struct {
	Reader    ports.ReviewEvidenceApplicabilityCurrentReaderV1
	Publisher ports.ReviewEvidenceApplicabilityOwnerPublisherV1
	Create    ports.PublishReviewEvidenceApplicabilityRequestV1
	Now       func() time.Time
}

type ReviewEvidenceApplicabilityCurrentReportV1 struct {
	ConcurrentPublishCalls int  `json:"concurrent_publish_calls"`
	OneLogicalRevision     bool `json:"one_logical_revision"`
	ResolvedExact          bool `json:"resolved_exact"`
	CurrentExact           bool `json:"current_exact"`
	HistoricalExact        bool `json:"historical_exact"`
	DeepClone              bool `json:"deep_clone"`
	ProductionClaim        bool `json:"production_claim"`
}

// VerifyReviewEvidenceApplicabilityCurrentV1 checks the owner-local reference
// contract. Passing is not evidence of production persistence, composition,
// availability or SLA.
func VerifyReviewEvidenceApplicabilityCurrentV1(ctx context.Context, testCase ReviewEvidenceApplicabilityCurrentCaseV1) (ReviewEvidenceApplicabilityCurrentReportV1, error) {
	if nilOrTypedNilReviewEvidenceConformanceV1(testCase.Reader) || nilOrTypedNilReviewEvidenceConformanceV1(testCase.Publisher) || testCase.Now == nil {
		return ReviewEvidenceApplicabilityCurrentReportV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Review evidence applicability conformance dependencies are incomplete")
	}
	if err := testCase.Create.Validate(); err != nil {
		return ReviewEvidenceApplicabilityCurrentReportV1{}, err
	}
	report := ReviewEvidenceApplicabilityCurrentReportV1{ConcurrentPublishCalls: 64, ProductionClaim: false}
	type result struct {
		receipt ports.ReviewEvidenceApplicabilityPublishReceiptV1
		err     error
	}
	results := make(chan result, report.ConcurrentPublishCalls)
	var wait sync.WaitGroup
	for range report.ConcurrentPublishCalls {
		wait.Add(1)
		go func() {
			defer wait.Done()
			receipt, err := testCase.Publisher.PublishReviewEvidenceApplicabilityV1(ctx, testCase.Create)
			results <- result{receipt: receipt, err: err}
		}()
	}
	wait.Wait()
	close(results)
	var canonical ports.ReviewEvidenceApplicabilityPublishReceiptV1
	for item := range results {
		if item.err != nil {
			return report, item.err
		}
		if canonical.PublishID == "" {
			canonical = item.receipt
		}
		if !reflect.DeepEqual(canonical, item.receipt) {
			return report, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "concurrent Review evidence publishes returned different receipts")
		}
	}
	report.OneLogicalRevision = canonical.Projection.Revision == 1 && canonical.CurrentIndex.HighestRevision == 1
	resolved, err := testCase.Reader.ResolveReviewEvidenceApplicabilityCurrentV1(ctx, ports.ResolveReviewEvidenceApplicabilityCurrentRequestV1{ContractVersion: ports.ReviewEvidenceCurrentContractVersionV1, Subject: testCase.Create.Projection.Subject})
	if err != nil {
		return report, err
	}
	report.ResolvedExact = resolved.Projection.Ref == canonical.Projection && resolved.CurrentIndex == canonical.CurrentIndex
	current, err := testCase.Reader.InspectCurrentReviewEvidenceApplicabilityV1(ctx, canonical.Projection)
	if err != nil {
		return report, err
	}
	report.CurrentExact = reflect.DeepEqual(current, resolved) && current.ValidateCurrent(canonical.Projection, testCase.Now()) == nil
	historical, err := testCase.Reader.InspectHistoricalReviewEvidenceApplicabilityV1(ctx, canonical.Projection)
	if err != nil {
		return report, err
	}
	report.HistoricalExact = reflect.DeepEqual(historical, resolved.Projection)
	if len(historical.EvidenceSubjectSnapshot.Projection.Causation) == 0 {
		historical.EvidenceSubjectSnapshot.Projection.Causation = append(historical.EvidenceSubjectSnapshot.Projection.Causation, ports.EvidenceCausationRefV2{})
	} else {
		historical.EvidenceSubjectSnapshot.Projection.Causation[0].EventID = "conformance-mutated"
	}
	again, err := testCase.Reader.InspectHistoricalReviewEvidenceApplicabilityV1(ctx, canonical.Projection)
	if err != nil {
		return report, err
	}
	report.DeepClone = !reflect.DeepEqual(historical, again)
	if !report.OneLogicalRevision || !report.ResolvedExact || !report.CurrentExact || !report.HistoricalExact || !report.DeepClone {
		return report, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability reference implementation failed conformance")
	}
	return report, nil
}

func nilOrTypedNilReviewEvidenceConformanceV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

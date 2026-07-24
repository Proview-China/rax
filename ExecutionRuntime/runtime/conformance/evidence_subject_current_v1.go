package conformance

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type EvidenceSubjectCurrentConformanceReportV1 struct {
	PublicReaderOnly      bool `json:"public_reader_only"`
	HistoricalExact       bool `json:"historical_exact"`
	CurrentClosureExact   bool `json:"current_closure_exact"`
	StaleExpectedRejected bool `json:"stale_expected_rejected"`
	ProductionClaim       bool `json:"production_claim"`
}

// VerifyEvidenceSubjectCurrentReaderV1 exercises only the least-authority
// public Reader. It cannot publish, CAS, append, tombstone or obtain the Fact
// Owner. Production durability and SLA are deliberately outside this report.
func VerifyEvidenceSubjectCurrentReaderV1(ctx context.Context, reader ports.EvidenceSubjectCurrentReaderV1, lookup ports.EvidenceSubjectCurrentLookupRequestV1) (EvidenceSubjectCurrentConformanceReportV1, error) {
	if nilOrTypedNilEvidenceSubjectConformanceV1(reader) {
		return EvidenceSubjectCurrentConformanceReportV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Evidence subject current conformance Reader is unavailable")
	}
	if err := lookup.Validate(); err != nil {
		return EvidenceSubjectCurrentConformanceReportV1{}, err
	}
	current, err := reader.InspectEvidenceSubjectCurrentV1(ctx, lookup)
	if err != nil {
		return EvidenceSubjectCurrentConformanceReportV1{}, err
	}
	if err := current.Validate(); err != nil {
		return EvidenceSubjectCurrentConformanceReportV1{}, err
	}
	historical, err := reader.InspectEvidenceSubjectProjectionV1(ctx, current.Projection.Ref)
	if err != nil {
		return EvidenceSubjectCurrentConformanceReportV1{}, err
	}
	validation := ports.EvidenceSubjectCurrentValidationRequestV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Subject: lookup.Subject, ExpectedProjection: current.Projection.Ref, ExpectedCurrentIndex: current.CurrentIndex, ExpectedRegistration: current.Projection.Registration, ExpectedReaderBinding: current.Projection.ReaderBinding, ExpectedReaderCapability: current.Projection.ReaderCapability, ExpectedConsumer: lookup.ExpectedConsumer, ExpectedExecutionScopeDigest: lookup.ExpectedExecutionScopeDigest, ExpectedSourcePolicy: lookup.ExpectedSourcePolicy}
	validated, err := reader.ValidateEvidenceSubjectCurrentV1(ctx, validation)
	if err != nil {
		return EvidenceSubjectCurrentConformanceReportV1{}, err
	}
	report := EvidenceSubjectCurrentConformanceReportV1{PublicReaderOnly: true, HistoricalExact: reflect.DeepEqual(historical, current.Projection), CurrentClosureExact: reflect.DeepEqual(validated, current), ProductionClaim: false}
	stale := validation
	stale.ExpectedProjection.Revision++
	_, staleErr := reader.ValidateEvidenceSubjectCurrentV1(ctx, stale)
	report.StaleExpectedRejected = staleErr != nil
	if !report.HistoricalExact || !report.CurrentClosureExact || !report.StaleExpectedRejected {
		return report, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence subject current public Reader failed conformance")
	}
	return report, nil
}

func nilOrTypedNilEvidenceSubjectConformanceV1(value any) bool {
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

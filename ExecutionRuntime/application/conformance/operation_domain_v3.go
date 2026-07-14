package conformance

import (
	"context"
	"sync"

	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type OperationDomainStateCaseV3 struct {
	NewPort         func() applicationports.OperationDomainStatePortV3
	Prepared        applicationports.BindPreparedOperationRequestV3
	Observed        applicationports.BindObservedOperationRequestV3
	Unknown         applicationports.MarkUnknownOperationRequestV3
	SettledObserved applicationports.ApplyOperationSettlementRequestV3
	SettledUnknown  applicationports.ApplyOperationSettlementRequestV3
}

// OperationDomainStateReportV3 is only a reusable certification candidate.
// Passing it never grants Binding, production, dispatch or domain commit
// authority to an adapter or custom component.
type OperationDomainStateReportV3 struct {
	IntentReservation       bool `json:"intent_reservation"`
	PreparedObservedSettled bool `json:"prepared_observed_settled"`
	PreparedUnknownSettled  bool `json:"prepared_unknown_settled"`
	ExactInspect            bool `json:"exact_inspect"`
	IdempotentReplay        bool `json:"idempotent_replay"`
	ConcurrentLinearization bool `json:"concurrent_linearization"`
	ForgedSwapRejected      bool `json:"forged_swap_rejected"`
	CertificationCandidate  bool `json:"certification_candidate"`
	BindingEligible         bool `json:"binding_eligible"`
	ProductionEligible      bool `json:"production_eligible"`
	DispatchEligible        bool `json:"dispatch_eligible"`
	CommitEligible          bool `json:"commit_eligible"`
}

func CheckOperationDomainStatePortV3(ctx context.Context, testCase OperationDomainStateCaseV3) (OperationDomainStateReportV3, error) {
	if testCase.NewPort == nil {
		return OperationDomainStateReportV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "operation domain conformance requires a fresh-port factory")
	}
	for _, validate := range []func() error{testCase.Prepared.Validate, testCase.Observed.Validate, testCase.Unknown.Validate, testCase.SettledObserved.Validate, testCase.SettledUnknown.Validate} {
		if err := validate(); err != nil {
			return OperationDomainStateReportV3{}, err
		}
	}
	if testCase.Prepared.Attempt.DomainReservation == nil {
		return OperationDomainStateReportV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "prepared conformance request lacks its Domain Reservation")
	}
	initial := testCase.Prepared.Attempt
	initial.Revision, initial.State, initial.Digest = 1, "intent_recorded", testCase.Prepared.Attempt.DomainReservation.AttemptDigest
	initial.DomainReservation = nil
	initial.AuthorizationDigest = core.DigestBytes([]byte("authorization-not-yet-created"))
	reserve := applicationports.ReserveOperationIntentRequestV3{StepKind: testCase.Prepared.StepKind, Descriptor: initial.Descriptor, DomainAdapter: initial.DomainAdapter, Attempt: initial, Intent: testCase.Prepared.Intent}
	if err := reserve.Validate(); err != nil {
		return OperationDomainStateReportV3{}, err
	}
	reservationPort := testCase.NewPort()
	if reservationPort == nil {
		return OperationDomainStateReportV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "operation domain factory returned nil")
	}
	reservation, err := reservationPort.ReserveOperationIntentV3(ctx, reserve)
	if err != nil {
		return OperationDomainStateReportV3{}, err
	}
	inspect := applicationports.InspectOperationIntentReservationRequestV3{Scope: reserve.Intent.Operation.ExecutionScope, StepKind: reserve.StepKind, DomainAdapter: reserve.DomainAdapter, AttemptID: reserve.Attempt.ID}
	inspected, err := reservationPort.InspectOperationIntentReservationV3(ctx, inspect)
	if err != nil || reservation.Digest != inspected.Digest {
		return OperationDomainStateReportV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "domain reservation is not create-once and inspectable")
	}
	if _, err := reservationPort.InspectOperationDomainStateV3(ctx, inspectForPreparedV3(testCase.Prepared)); !core.HasCategory(err, core.ErrorNotFound) {
		return OperationDomainStateReportV3{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "domain reservation prematurely persisted a prepared domain fact")
	}
	if err := checkObservedDomainBranchV3(ctx, testCase); err != nil {
		return OperationDomainStateReportV3{}, err
	}
	if err := checkUnknownDomainBranchV3(ctx, testCase); err != nil {
		return OperationDomainStateReportV3{}, err
	}
	if err := checkConcurrentDomainCreateV3(ctx, testCase); err != nil {
		return OperationDomainStateReportV3{}, err
	}
	if err := checkDomainForgeryRejectionV3(ctx, testCase); err != nil {
		return OperationDomainStateReportV3{}, err
	}
	return OperationDomainStateReportV3{IntentReservation: true, PreparedObservedSettled: true, PreparedUnknownSettled: true, ExactInspect: true, IdempotentReplay: true, ConcurrentLinearization: true, ForgedSwapRejected: true, CertificationCandidate: true}, nil
}

func checkObservedDomainBranchV3(ctx context.Context, tc OperationDomainStateCaseV3) error {
	port := tc.NewPort()
	if port == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "operation domain factory returned nil")
	}
	if err := reservePreparedDomainV3(ctx, port, tc.Prepared); err != nil {
		return err
	}
	prepared, err := port.BindPrepared(ctx, tc.Prepared)
	if err != nil {
		return err
	}
	replayed, err := port.BindPrepared(ctx, tc.Prepared)
	if err != nil || !sameDomainStateRefV3(prepared, replayed) {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "BindPrepared replay was not exact and idempotent")
	}
	inspected, err := port.InspectOperationDomainStateV3(ctx, inspectForPreparedV3(tc.Prepared))
	if err != nil || !sameDomainStateRefV3(prepared, inspected) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "domain Inspect did not return exact prepared state")
	}
	observed, err := port.BindObserved(ctx, tc.Observed)
	if err != nil || observed.State != applicationports.OperationDomainObservedV3 {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "domain did not advance prepared to observed")
	}
	settled, err := port.ApplySettlement(ctx, tc.SettledObserved)
	if err != nil || settled.State != applicationports.OperationDomainSettledV3 {
		return core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "domain did not advance observed to settled")
	}
	return nil
}

func checkUnknownDomainBranchV3(ctx context.Context, tc OperationDomainStateCaseV3) error {
	port := tc.NewPort()
	if err := reservePreparedDomainV3(ctx, port, tc.Prepared); err != nil {
		return err
	}
	if _, err := port.BindPrepared(ctx, tc.Prepared); err != nil {
		return err
	}
	unknown, err := port.MarkUnknown(ctx, tc.Unknown)
	if err != nil || unknown.State != applicationports.OperationDomainUnknownV3 {
		return core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "domain did not advance prepared to unknown")
	}
	settled, err := port.ApplySettlement(ctx, tc.SettledUnknown)
	if err != nil || settled.State != applicationports.OperationDomainSettledV3 {
		return core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "domain did not advance unknown to settled")
	}
	return nil
}

func checkConcurrentDomainCreateV3(ctx context.Context, tc OperationDomainStateCaseV3) error {
	port := tc.NewPort()
	if err := reservePreparedDomainV3(ctx, port, tc.Prepared); err != nil {
		return err
	}
	values := make(chan applicationports.OperationDomainStateRefV3, 64)
	errs := make(chan error, 64)
	var group sync.WaitGroup
	for range 64 {
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := port.BindPrepared(ctx, tc.Prepared)
			if err != nil {
				errs <- err
				return
			}
			values <- value
		}()
	}
	group.Wait()
	close(values)
	close(errs)
	for err := range errs {
		return err
	}
	var expected *applicationports.OperationDomainStateRefV3
	for value := range values {
		if expected == nil {
			copy := value
			expected = &copy
		} else if !sameDomainStateRefV3(*expected, value) {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "concurrent BindPrepared did not linearize to one fact")
		}
	}
	return nil
}

func checkDomainForgeryRejectionV3(ctx context.Context, tc OperationDomainStateCaseV3) error {
	mutations := []func(applicationports.BindPreparedOperationRequestV3) applicationports.BindPreparedOperationRequestV3{
		func(value applicationports.BindPreparedOperationRequestV3) applicationports.BindPreparedOperationRequestV3 {
			value.StepKind = "conformance.invalid/swap"
			return value
		},
		func(value applicationports.BindPreparedOperationRequestV3) applicationports.BindPreparedOperationRequestV3 {
			value.RuntimeAttempt.PermitID = "forged-permit"
			return value
		},
		func(value applicationports.BindPreparedOperationRequestV3) applicationports.BindPreparedOperationRequestV3 {
			value.Prepared.Enforcement.ReceiptDigest = core.DigestBytes([]byte("forged-basis"))
			return value
		},
	}
	for _, mutate := range mutations {
		port := tc.NewPort()
		if err := reservePreparedDomainV3(ctx, port, tc.Prepared); err != nil {
			return err
		}
		if _, err := port.BindPrepared(ctx, mutate(tc.Prepared)); err == nil {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "operation domain accepted a forged Application attempt, Runtime attempt, StepKind or Basis")
		}
	}
	port := tc.NewPort()
	if err := reservePreparedDomainV3(ctx, port, tc.Prepared); err != nil {
		return err
	}
	if _, err := port.BindPrepared(ctx, tc.Prepared); err != nil {
		return err
	}
	forgedAttempt := tc.Prepared
	forgedAttempt.Attempt.Digest = core.DigestBytes([]byte("forged-attempt"))
	if _, err := port.BindPrepared(ctx, forgedAttempt); err == nil {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "operation domain replaced an existing Application attempt ref")
	}
	return nil
}

func reservePreparedDomainV3(ctx context.Context, port applicationports.OperationDomainStatePortV3, prepared applicationports.BindPreparedOperationRequestV3) error {
	if prepared.Attempt.DomainReservation == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "prepared request lacks Domain Reservation")
	}
	initial := prepared.Attempt
	initial.Revision, initial.State, initial.Digest = 1, "intent_recorded", prepared.Attempt.DomainReservation.AttemptDigest
	initial.DomainReservation = nil
	initial.AuthorizationDigest = core.DigestBytes([]byte("authorization-not-yet-created"))
	request := applicationports.ReserveOperationIntentRequestV3{StepKind: prepared.StepKind, Descriptor: initial.Descriptor, DomainAdapter: initial.DomainAdapter, Attempt: initial, Intent: prepared.Intent}
	reservation, err := port.ReserveOperationIntentV3(ctx, request)
	if err != nil {
		return err
	}
	if err := applicationports.ValidateOperationDomainReservationForV3(reservation, request); err != nil {
		return err
	}
	if reservation.Digest != prepared.Attempt.DomainReservation.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "prepared request carries another Domain Reservation than the owner")
	}
	return nil
}

func inspectForPreparedV3(request applicationports.BindPreparedOperationRequestV3) applicationports.OperationDomainInspectRequestV3 {
	return applicationports.OperationDomainInspectRequestV3{Scope: request.Intent.Operation.ExecutionScope, StepKind: request.StepKind, AttemptID: request.Attempt.ID}
}

func sameDomainStateRefV3(left, right applicationports.OperationDomainStateRefV3) bool {
	ld, le := core.CanonicalJSONDigest("praxis.application.conformance", applicationports.OperationDomainContractVersionV3, "OperationDomainStateRefV3", left)
	rd, re := core.CanonicalJSONDigest("praxis.application.conformance", applicationports.OperationDomainContractVersionV3, "OperationDomainStateRefV3", right)
	return le == nil && re == nil && ld == rd
}

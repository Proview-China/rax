package conformance

import (
	"context"
	"reflect"
	"strings"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var singleCallToolActionAdapterAllowedImportsV2 = [...]string{
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core",
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports",
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract",
	"github.com/Proview-China/rax/ExecutionRuntime/application/ports",
}

func SingleCallToolActionAdapterAllowedImportsV2() []string {
	result := make([]string, len(singleCallToolActionAdapterAllowedImportsV2))
	copy(result, singleCallToolActionAdapterAllowedImportsV2[:])
	return result
}

// CheckSingleCallToolActionAdapterImportsV2 is build hygiene only. It grants
// no Binding, dispatch, Provider access, production or settlement authority.
func CheckSingleCallToolActionAdapterImportsV2(imports []string) error {
	allowed := SingleCallToolActionAdapterAllowedImportsV2()
	for _, candidate := range imports {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call V2 adapter import is empty")
		}
		if !strings.HasPrefix(candidate, "github.com/Proview-China/rax/ExecutionRuntime/") {
			continue
		}
		permitted := false
		for _, prefix := range allowed {
			if candidate == prefix || strings.HasPrefix(candidate, prefix+"/") {
				permitted = true
				break
			}
		}
		if !permitted {
			return core.NewError(core.ErrorForbidden, core.ReasonComponentMismatch, "single-call V2 adapter imports an Owner implementation package")
		}
	}
	return nil
}

type SingleCallToolActionCoordinationCaseV2 struct {
	NewPort     func() applicationports.SingleCallToolActionCoordinationFactPortV2
	Prepared    contract.SingleCallToolActionCoordinationFactV2
	Dispatch    applicationports.SingleCallToolActionCoordinationCASRequestV2
	StartClaim  applicationports.SingleCallToolActionCoordinationCASRequestV2
	Completed   applicationports.SingleCallToolActionCoordinationCASRequestV2
	Conflicting contract.SingleCallToolActionCoordinationFactV2
}

// SingleCallToolActionCoordinationReportV2 is a reusable certification
// candidate. Even a complete report cannot claim production durability,
// dispatch eligibility or permission to expose a Tool/Provider route.
type SingleCallToolActionCoordinationReportV2 struct {
	AtomicInitialClaim      bool `json:"atomic_initial_claim"`
	ExactInspect            bool `json:"exact_inspect"`
	IdempotentReplay        bool `json:"idempotent_replay"`
	ConcurrentLinearization bool `json:"concurrent_linearization"`
	StateMachine            bool `json:"state_machine"`
	ChangedContentRejected  bool `json:"changed_content_rejected"`
	CertificationCandidate  bool `json:"certification_candidate"`
	BindingEligible         bool `json:"binding_eligible"`
	ProductionEligible      bool `json:"production_eligible"`
	DispatchEligible        bool `json:"dispatch_eligible"`
	SystemG6AEligible       bool `json:"system_g6a_eligible"`
}

type SingleCallToolActionAtomicInitialClaimInspectorV2 interface {
	InspectSingleCallToolActionAtomicInitialClaimForTestV2(core.ExecutionScope, string) (contract.SingleCallToolActionCoordinationFactV2, contract.SingleCallToolActionVersionClaimV1, error)
}

func CheckSingleCallToolActionCoordinationPortV2(ctx context.Context, tc SingleCallToolActionCoordinationCaseV2) (SingleCallToolActionCoordinationReportV2, error) {
	if isNilSingleCallValueV2(ctx) {
		return SingleCallToolActionCoordinationReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call V2 conformance context is nil")
	}
	if tc.NewPort == nil {
		return SingleCallToolActionCoordinationReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "single-call V2 conformance requires a fresh-port factory")
	}
	for _, validate := range []func() error{tc.Prepared.Validate, tc.Dispatch.Validate, tc.StartClaim.Validate, tc.Completed.Validate} {
		if err := validate(); err != nil {
			return SingleCallToolActionCoordinationReportV2{}, err
		}
	}
	if tc.Prepared.State != contract.SingleCallToolActionPreparedV2 || tc.Prepared.Revision != 1 {
		return SingleCallToolActionCoordinationReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "single-call V2 conformance must begin with prepared revision 1")
	}
	port := tc.NewPort()
	if isNilSingleCallPortV2(port) {
		return SingleCallToolActionCoordinationReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "single-call V2 conformance factory returned nil")
	}
	created, err := port.CreateSingleCallToolActionCoordinationV2(ctx, tc.Prepared)
	if err != nil {
		return SingleCallToolActionCoordinationReportV2{}, err
	}
	replayed, err := port.CreateSingleCallToolActionCoordinationV2(ctx, tc.Prepared)
	if err != nil || !sameSingleCallFactV2(created, replayed) {
		return SingleCallToolActionCoordinationReportV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call V2 create replay was not exact")
	}
	inspected, err := port.InspectSingleCallToolActionCoordinationV2(ctx, tc.Prepared.Request.Action.ExecutionScope, tc.Prepared.ID)
	if err != nil || !sameSingleCallFactV2(created, inspected) {
		return SingleCallToolActionCoordinationReportV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call V2 Inspect did not return the exact fact")
	}
	atomicInitialClaim := false
	if inspector, ok := port.(SingleCallToolActionAtomicInitialClaimInspectorV2); ok {
		initial, claim, inspectErr := inspector.InspectSingleCallToolActionAtomicInitialClaimForTestV2(tc.Prepared.Request.Action.ExecutionScope, tc.Prepared.ID)
		if inspectErr != nil || !sameSingleCallFactV2(initial, tc.Prepared) || claim.ValidateFor(initial) != nil {
			return SingleCallToolActionCoordinationReportV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call V2 initial VersionClaim is not exact")
		}
		atomicInitialClaim = true
	}
	if err := checkConcurrentSingleCallCreateV2(ctx, tc); err != nil {
		return SingleCallToolActionCoordinationReportV2{}, err
	}
	if tc.Conflicting.ID != "" {
		if err := tc.Conflicting.Validate(); err != nil {
			return SingleCallToolActionCoordinationReportV2{}, err
		}
		preparedKey, err := contract.DeriveSingleCallToolActionCrossVersionConflictKeyV1(tc.Prepared.Request)
		if err != nil {
			return SingleCallToolActionCoordinationReportV2{}, err
		}
		conflictingKey, err := contract.DeriveSingleCallToolActionCrossVersionConflictKeyV1(tc.Conflicting.Request)
		if err != nil {
			return SingleCallToolActionCoordinationReportV2{}, err
		}
		if preparedKey != conflictingKey || tc.Prepared.Digest == tc.Conflicting.Digest {
			return SingleCallToolActionCoordinationReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "single-call V2 conflicting fixture must carry different content for one stable semantic key")
		}
		if _, err := port.CreateSingleCallToolActionCoordinationV2(ctx, tc.Conflicting); !core.HasCategory(err, core.ErrorConflict) {
			return SingleCallToolActionCoordinationReportV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call V2 store accepted changed content for an occupied semantic key")
		}
		if err := checkConcurrentSingleCallConflictV2(ctx, tc); err != nil {
			return SingleCallToolActionCoordinationReportV2{}, err
		}
	}
	current := created
	for _, request := range []applicationports.SingleCallToolActionCoordinationCASRequestV2{tc.Dispatch, tc.StartClaim, tc.Completed} {
		if request.ExpectedRevision != current.Revision || request.ExpectedDigest != current.Digest {
			return SingleCallToolActionCoordinationReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "single-call V2 conformance CAS chain is not exact")
		}
		current, err = port.CompareAndSwapSingleCallToolActionCoordinationV2(ctx, request)
		if err != nil || !sameSingleCallFactV2(current, request.Next) {
			return SingleCallToolActionCoordinationReportV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "single-call V2 CAS did not return the exact requested successor")
		}
	}
	changedContentRejected := tc.Conflicting.ID != ""
	return SingleCallToolActionCoordinationReportV2{
		AtomicInitialClaim: atomicInitialClaim, ExactInspect: true, IdempotentReplay: true,
		ConcurrentLinearization: true, StateMachine: true,
		ChangedContentRejected: changedContentRejected, CertificationCandidate: atomicInitialClaim && changedContentRejected,
	}, nil
}

func checkConcurrentSingleCallConflictV2(ctx context.Context, tc SingleCallToolActionCoordinationCaseV2) error {
	port := tc.NewPort()
	if isNilSingleCallPortV2(port) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "single-call V2 conformance factory returned nil")
	}
	values := make(chan contract.SingleCallToolActionCoordinationFactV2, 64)
	errors := make(chan error, 64)
	var group sync.WaitGroup
	for index := range 64 {
		candidate := tc.Prepared
		if index%2 == 1 {
			candidate = tc.Conflicting
		}
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := port.CreateSingleCallToolActionCoordinationV2(ctx, candidate)
			if err != nil {
				errors <- err
				return
			}
			values <- value
		}()
	}
	group.Wait()
	close(values)
	close(errors)
	for err := range errors {
		if !core.HasCategory(err, core.ErrorConflict) {
			return err
		}
	}
	var winner *contract.SingleCallToolActionCoordinationFactV2
	for value := range values {
		if winner == nil {
			copy := value
			winner = &copy
			continue
		}
		if !sameSingleCallFactV2(*winner, value) {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "different single-call V2 contents both linearized for one semantic key")
		}
	}
	if winner == nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "single-call V2 semantic conflict produced no linearized winner")
	}
	inspected, err := port.InspectSingleCallToolActionCoordinationV2(ctx, winner.Request.Action.ExecutionScope, winner.ID)
	if err != nil || !sameSingleCallFactV2(*winner, inspected) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call V2 semantic winner is not exactly inspectable")
	}
	loser := tc.Prepared
	if winner.Digest == tc.Prepared.Digest {
		loser = tc.Conflicting
	}
	if loser.ID != winner.ID {
		if _, err := port.InspectSingleCallToolActionCoordinationV2(ctx, loser.Request.Action.ExecutionScope, loser.ID); !core.HasCategory(err, core.ErrorNotFound) {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call V2 semantic loser left an orphan coordination fact")
		}
	}
	return nil
}

func checkConcurrentSingleCallCreateV2(ctx context.Context, tc SingleCallToolActionCoordinationCaseV2) error {
	port := tc.NewPort()
	if isNilSingleCallPortV2(port) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "single-call V2 conformance factory returned nil")
	}
	values := make(chan contract.SingleCallToolActionCoordinationFactV2, 64)
	errors := make(chan error, 64)
	var group sync.WaitGroup
	for range 64 {
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := port.CreateSingleCallToolActionCoordinationV2(ctx, tc.Prepared)
			if err != nil {
				errors <- err
				return
			}
			values <- value
		}()
	}
	group.Wait()
	close(values)
	close(errors)
	for err := range errors {
		return err
	}
	for value := range values {
		if !sameSingleCallFactV2(value, tc.Prepared) {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "concurrent single-call V2 create did not linearize to one exact fact")
		}
	}
	return nil
}

func sameSingleCallFactV2(left, right contract.SingleCallToolActionCoordinationFactV2) bool {
	if left.Validate() != nil || right.Validate() != nil {
		return false
	}
	ld, le := core.CanonicalJSONDigest("praxis.application.single-call-conformance-v2", "2.0.0", "SingleCallToolActionCoordinationFactV2", left)
	rd, re := core.CanonicalJSONDigest("praxis.application.single-call-conformance-v2", "2.0.0", "SingleCallToolActionCoordinationFactV2", right)
	return le == nil && re == nil && ld == rd
}

func isNilSingleCallPortV2(port applicationports.SingleCallToolActionCoordinationFactPortV2) bool {
	return isNilSingleCallValueV2(port)
}

func isNilSingleCallValueV2(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

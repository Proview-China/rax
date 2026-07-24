package applicationadapter

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type CheckpointProviderResultBindingV2 struct {
	Reservation contract.Ref                                           `json:"reservation_ref"`
	Phase       contract.CheckpointPhase                               `json:"phase"`
	Execute     dataplaneadapter.DispatchRequestV1                     `json:"execute_request"`
	Evidence    runtimeports.CheckpointRestoreEvidenceConsumptionRefV1 `json:"evidence_consumption"`
}

func (v CheckpointProviderResultBindingV2) Validate() error {
	if v.Reservation.ValidateShape("checkpoint Provider result reservation") != nil || v.Phase.Validate() != nil || v.Execute.ContractVersion != dataplaneadapter.ContractVersionV1 || v.Execute.RequestID == "" || v.Execute.Digest == "" || v.Execute.Phase != dataplaneadapter.PhaseExecute || v.Evidence.Validate() != nil || string(v.Evidence.Phase) != "checkpoint_"+string(v.Phase) {
		return errors.New("checkpoint Provider result binding is incomplete")
	}
	return nil
}

type CheckpointProviderResultBindingStoreV2 interface {
	CreateCheckpointProviderResultBindingV2(context.Context, CheckpointProviderResultBindingV2) (CheckpointProviderResultBindingV2, error)
	InspectCheckpointProviderResultBindingV2(context.Context, contract.Ref) (CheckpointProviderResultBindingV2, error)
}

// CheckpointActualPointOwnerV2 persists only the exact bridge needed to
// re-inspect Provider and Runtime Evidence current. It never mints a
// DomainResult, Settlement, or PhaseFact.
type CheckpointActualPointOwnerV2 struct {
	boundary *CheckpointProviderBoundaryV1
	bindings CheckpointProviderResultBindingStoreV2
}

func NewCheckpointActualPointOwnerV2(boundary *CheckpointProviderBoundaryV1, bindings CheckpointProviderResultBindingStoreV2) (*CheckpointActualPointOwnerV2, error) {
	if boundary == nil || nilLike(bindings) {
		return nil, errors.New("checkpoint actual-point boundary and durable binding store are required")
	}
	return &CheckpointActualPointOwnerV2{boundary: boundary, bindings: bindings}, nil
}

func (o *CheckpointActualPointOwnerV2) ExecuteAndBindCheckpointPhaseV2(ctx context.Context, reservation contract.Ref, phase contract.CheckpointPhase, plan CheckpointProviderPlanV1) (contract.CheckpointPhaseResultCurrentProjectionV2, CheckpointProviderResultV1, error) {
	if o == nil || nilLike(ctx) || reservation.ValidateShape("checkpoint actual-point reservation") != nil || phase.Validate() != nil || string(plan.Phase) != "checkpoint_"+string(phase) {
		return contract.CheckpointPhaseResultCurrentProjectionV2{}, CheckpointProviderResultV1{}, errors.New("checkpoint actual-point request is invalid")
	}
	result, err := o.boundary.ExecuteCheckpointPhaseV1(ctx, plan)
	if err != nil {
		return contract.CheckpointPhaseResultCurrentProjectionV2{}, CheckpointProviderResultV1{}, err
	}
	binding := CheckpointProviderResultBindingV2{Reservation: reservation, Phase: phase, Execute: result.ExecuteRequest, Evidence: result.Consumption}
	stored, createErr := o.bindings.CreateCheckpointProviderResultBindingV2(ctx, binding)
	if createErr != nil {
		stored, err = o.bindings.InspectCheckpointProviderResultBindingV2(context.WithoutCancel(ctx), reservation)
		if err != nil {
			return contract.CheckpointPhaseResultCurrentProjectionV2{}, CheckpointProviderResultV1{}, createErr
		}
	}
	if !reflect.DeepEqual(stored, binding) {
		return contract.CheckpointPhaseResultCurrentProjectionV2{}, CheckpointProviderResultV1{}, fmt.Errorf("%w: checkpoint actual-point binding winner differs", ports.ErrConflict)
	}
	projection, err := o.InspectCheckpointPhaseResultCurrentV2(ctx, reservation)
	return projection, result, err
}

func (o *CheckpointActualPointOwnerV2) InspectCheckpointPhaseResultCurrentV2(ctx context.Context, reservation contract.Ref) (contract.CheckpointPhaseResultCurrentProjectionV2, error) {
	if o == nil || nilLike(ctx) || reservation.ValidateShape("checkpoint result current reservation") != nil {
		return contract.CheckpointPhaseResultCurrentProjectionV2{}, errors.New("checkpoint result current request is invalid")
	}
	binding, err := o.bindings.InspectCheckpointProviderResultBindingV2(ctx, reservation)
	if err != nil {
		return contract.CheckpointPhaseResultCurrentProjectionV2{}, err
	}
	if binding.Validate() != nil || !contract.SameRef(binding.Reservation, reservation) {
		return contract.CheckpointPhaseResultCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint result binding drifted", ports.ErrConflict)
	}
	response, err := o.boundary.dataplane.Inspect(ctx, binding.Execute)
	if err != nil || validateCheckpointProviderInspectionV1(binding.Execute, response, o.boundary.now()) != nil {
		return contract.CheckpointPhaseResultCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint Provider exact Inspect is unavailable", ports.ErrUnknownOutcome)
	}
	consumption, err := o.boundary.checkpointEvidence.InspectCheckpointPhaseEvidenceConsumptionCurrentV1(ctx, binding.Evidence)
	if err != nil || consumption.Validate() != nil || consumption.Ref != binding.Evidence {
		return contract.CheckpointPhaseResultCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint Evidence consumption is not exact current", ports.ErrConflict)
	}
	qualification, err := o.boundary.checkpointEvidence.InspectCheckpointPhaseQualificationCurrentV1(ctx, binding.Evidence.Qualification)
	if err != nil || qualification.Validate(o.boundary.now()) != nil || qualification.Ref != binding.Evidence.Qualification {
		return contract.CheckpointPhaseResultCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint Evidence qualification is not exact current", ports.ErrConflict)
	}
	return checkpointResultProjectionFromInspectionV2(binding, response, qualification.Ref.ExpiresUnixNano)
}

func checkpointResultProjectionFromInspectionV2(binding CheckpointProviderResultBindingV2, response dataplaneadapter.DispatchResponseV1, evidenceExpires int64) (contract.CheckpointPhaseResultCurrentProjectionV2, error) {
	if binding.Validate() != nil || response.ProviderAttempt == nil || response.ProviderObservation == nil || response.ProviderReceipt == nil || response.ObservationDigest == nil || response.ReceiptDigest == nil || response.ProviderObservation.CheckpointArtifact == nil {
		return contract.CheckpointPhaseResultCurrentProjectionV2{}, errors.New("checkpoint result inspection lacks exact Provider closure")
	}
	artifact := response.ProviderObservation.CheckpointArtifact
	state := contract.CheckpointPhaseState(artifact.State)
	if state.ValidateFor(binding.Phase) != nil || artifact.CheckpointPhase != "checkpoint_"+string(binding.Phase) {
		return contract.CheckpointPhaseResultCurrentProjectionV2{}, errors.New("checkpoint Provider state does not match semantic phase")
	}
	strip := func(value string) string { return strings.TrimPrefix(value, "sha256:") }
	attempt := contract.Ref{ID: response.ProviderAttempt.ID, Revision: response.ProviderAttempt.Revision, Digest: strip(response.ProviderAttempt.Digest)}
	observation := contract.Ref{ID: response.ProviderAttempt.ID + "-observation", Revision: response.ProviderAttempt.Revision, Digest: strip(*response.ObservationDigest)}
	receipt := contract.Ref{ID: response.ProviderAttempt.ID + "-receipt", Revision: response.ProviderAttempt.Revision, Digest: strip(*response.ReceiptDigest)}
	evidence := contract.Ref{ID: binding.Evidence.ID, Revision: uint64(binding.Evidence.Revision), Digest: strip(string(binding.Evidence.Digest))}
	expires := min(response.ExpiresUnixNano, artifact.ExpiresUnixNano, binding.Evidence.Qualification.ExpiresUnixNano, evidenceExpires)
	return contract.SealCheckpointPhaseResultCurrentProjectionV2(contract.CheckpointPhaseResultCurrentProjectionV2{ReservationRef: binding.Reservation, State: state, ProviderAttemptRef: attempt, ProviderObservation: observation, ProviderReceipt: receipt, EvidenceConsumption: evidence, CheckedUnixNano: response.ProviderObservation.ObservedUnixNano, ExpiresUnixNano: expires})
}

var _ ports.CheckpointPhaseResultCurrentReaderV2 = (*CheckpointActualPointOwnerV2)(nil)

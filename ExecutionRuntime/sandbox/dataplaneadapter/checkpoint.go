package dataplaneadapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	CheckpointEffectKindV1    = "praxis.sandbox/checkpoint"
	checkpointQueryContractV1 = "praxis.sandbox/checkpoint-current-query/v1"
	checkpointPhasePrepareV1  = "checkpoint_prepare"
	checkpointPhaseCommitV1   = "checkpoint_commit"
	checkpointPhaseAbortV1    = "checkpoint_abort"
)

type CheckpointExactRefV1 struct {
	ID              string `json:"id"`
	Revision        uint64 `json:"revision"`
	Digest          string `json:"digest"`
	ExpiresUnixNano int64  `json:"expires_unix_nano"`
}

type CheckpointPreviousPhaseV1 struct {
	Reservation     CheckpointExactRefV1 `json:"reservation"`
	ClosureID       string               `json:"closure_id"`
	ClosureDigest   string               `json:"closure_digest"`
	State           string               `json:"state"`
	ExpiresUnixNano int64                `json:"expires_unix_nano"`
}

type CheckpointRuntimeCurrentQueryV1 struct {
	ContractVersion   string                                                        `json:"contract_version"`
	RuntimeInspect    runtimeports.InspectCurrentCheckpointRestoreDispatchRequestV1 `json:"runtime_inspect"`
	Phase             string                                                        `json:"phase"`
	CheckpointAttempt CheckpointExactRefV1                                          `json:"checkpoint_attempt"`
	Barrier           CheckpointExactRefV1                                          `json:"barrier"`
	EffectCut         CheckpointExactRefV1                                          `json:"effect_cut"`
	Reservation       CheckpointExactRefV1                                          `json:"reservation"`
	Participant       CheckpointExactRefV1                                          `json:"participant"`
	PreviousPhase     *CheckpointPreviousPhaseV1                                    `json:"previous_phase,omitempty"`
	ProjectionDigest  string                                                        `json:"projection_digest"`
	ExpiresUnixNano   int64                                                         `json:"expires_unix_nano"`
}

type CheckpointDispatchInputV1 struct {
	RequestID         string
	Current           runtimeports.CurrentCheckpointRestoreDispatchEnforcementV1
	PayloadSchema     string
	PayloadRevision   uint64
	Payload           ProviderPayloadV1
	RequestedNotAfter time.Time
}

func NewCheckpointDispatchRequestV1(input CheckpointDispatchInputV1) (DispatchRequestV1, error) {
	now := time.Now()
	if err := input.Current.Validate(now); err != nil {
		return DispatchRequestV1{}, fmt.Errorf("checkpoint runtime enforcement current: %w", err)
	}
	phase, err := phaseFromRuntime(input.Current.Phase.Phase)
	if err != nil {
		return DispatchRequestV1{}, err
	}
	query, err := checkpointQueryFromCurrentV1(input.Current, now)
	if err != nil {
		return DispatchRequestV1{}, err
	}
	queryJSON, err := json.Marshal(query)
	if err != nil {
		return DispatchRequestV1{}, err
	}
	queryJSON, err = canonicalJSON(queryJSON)
	if err != nil {
		return DispatchRequestV1{}, err
	}
	provider, err := providerFromRuntime(input.Current.Sandbox.Verifier)
	if err != nil {
		return DispatchRequestV1{}, err
	}
	request := DispatchRequestV1{
		ContractVersion:           ContractVersionV1,
		RequestID:                 input.RequestID,
		Phase:                     phase,
		EffectKind:                CheckpointEffectKindV1,
		OperationDigest:           string(input.Current.Sandbox.OperationDigest),
		EffectID:                  string(input.Current.Sandbox.EffectID),
		IntentRevision:            uint64(input.Current.Sandbox.IntentRevision),
		IntentDigest:              string(input.Current.Sandbox.IntentDigest),
		AttemptID:                 input.Current.Sandbox.DispatchAttempt.ID,
		TenantID:                  string(input.Current.Sandbox.Operation.ExecutionScope.Identity.TenantID),
		ProviderBinding:           provider,
		SandboxAttempt:            factRef(input.Current.Sandbox.DispatchAttempt),
		ExecutionBinding:          checkpointExecutionBindingV1(input.Current),
		RuntimeEnforcement:        enforcementRef(input.Current.Phase, phase),
		RuntimeCurrentQuery:       queryJSON,
		RequestedNotAfterUnixNano: input.RequestedNotAfter.UnixNano(),
		PayloadSchema:             input.PayloadSchema,
		PayloadRevision:           input.PayloadRevision,
		Payload:                   input.Payload,
	}
	request.RuntimeCurrentQueryDigest, err = canonicalDigest("RuntimeCurrentQueryV1", json.RawMessage(queryJSON))
	if err != nil {
		return DispatchRequestV1{}, err
	}
	request.PayloadDigest, err = canonicalDigest("ProviderPayloadV1", request.Payload)
	if err != nil {
		return DispatchRequestV1{}, err
	}
	legacy := input.Current.Dispatch.Record.Permit.LegacyPermit
	if err := validateProviderPayloadBinding(legacy.PayloadSchema, legacy.PayloadDigest, legacy.PayloadRevision, request.PayloadSchema, request.PayloadDigest, request.PayloadRevision); err != nil {
		return DispatchRequestV1{}, errors.New("checkpoint provider payload differs from the exact Runtime Permit")
	}
	request.Digest, err = request.digestV1()
	if err != nil {
		return DispatchRequestV1{}, err
	}
	return request, request.ValidateCurrent(now)
}

func checkpointQueryFromCurrentV1(current runtimeports.CurrentCheckpointRestoreDispatchEnforcementV1, now time.Time) (CheckpointRuntimeCurrentQueryV1, error) {
	phase := string(current.Sandbox.Reservation.Phase)
	if phase != checkpointPhasePrepareV1 && phase != checkpointPhaseCommitV1 && phase != checkpointPhaseAbortV1 {
		return CheckpointRuntimeCurrentQueryV1{}, errors.New("checkpoint participant phase is unsupported")
	}
	inspect := runtimeports.InspectCurrentCheckpointRestoreDispatchRequestV1{
		Operation:               current.Sandbox.Operation,
		EffectID:                current.Sandbox.EffectID,
		PermitID:                current.Phase.PermitID,
		Phase:                   current.Phase.Phase,
		PermitDigest:            current.Phase.PermitDigest,
		AdmissionDigest:         current.Phase.AdmissionDigest,
		ReviewAuthorization:     current.Phase.ReviewAuthorization,
		Reservation:             current.Sandbox.Reservation.Ref,
		SandboxProjectionDigest: current.Sandbox.ProjectionDigest,
	}
	if err := inspect.Validate(); err != nil {
		return CheckpointRuntimeCurrentQueryV1{}, err
	}
	expires := current.ExpiresUnixNano
	query := CheckpointRuntimeCurrentQueryV1{
		ContractVersion: checkpointQueryContractV1,
		RuntimeInspect:  inspect,
		Phase:           phase,
		CheckpointAttempt: CheckpointExactRefV1{
			ID: current.Sandbox.Reservation.Attempt.ID, Revision: uint64(current.Sandbox.Reservation.Attempt.Revision), Digest: string(current.Sandbox.Reservation.Attempt.Digest), ExpiresUnixNano: expires,
		},
		Barrier: CheckpointExactRefV1{
			ID: current.Sandbox.Reservation.Barrier.ID, Revision: uint64(current.Sandbox.Reservation.Barrier.Revision), Digest: string(current.Sandbox.Reservation.Barrier.Digest), ExpiresUnixNano: current.Sandbox.Reservation.Barrier.ExpiresUnixNano,
		},
		EffectCut: CheckpointExactRefV1{
			ID: current.Sandbox.Reservation.EffectCut.ID, Revision: uint64(current.Sandbox.Reservation.EffectCut.Revision), Digest: string(current.Sandbox.Reservation.EffectCut.Digest), ExpiresUnixNano: expires,
		},
		Reservation:      factRefAsCheckpointV1(current.Sandbox.SandboxReservation),
		Participant:      factRefAsCheckpointV1(current.Sandbox.Participant),
		ProjectionDigest: string(current.Sandbox.ProjectionDigest),
		ExpiresUnixNano:  expires,
	}
	if previous := current.Sandbox.Reservation.PreviousPhase; previous != nil {
		previousExpires := minimum(expires, previous.Reservation.ExpiresUnixNano, previous.Evidence.Qualification.ExpiresUnixNano)
		query.PreviousPhase = &CheckpointPreviousPhaseV1{
			Reservation: CheckpointExactRefV1{
				ID: previous.Reservation.ID, Revision: uint64(previous.Reservation.Revision), Digest: string(previous.Reservation.Digest), ExpiresUnixNano: previous.Reservation.ExpiresUnixNano,
			},
			ClosureID: previous.ID, ClosureDigest: string(previous.Digest), State: string(previous.PhaseFact.State), ExpiresUnixNano: previousExpires,
		}
	}
	return query, validateCheckpointQueryV1(query, now)
}

func validateCheckpointQueryV1(query CheckpointRuntimeCurrentQueryV1, now time.Time) error {
	if query.ContractVersion != checkpointQueryContractV1 || query.RuntimeInspect.Validate() != nil || query.CheckpointAttempt.ID == "" || query.Barrier.ID == "" || query.EffectCut.ID == "" || query.Reservation.ID == "" || query.Participant.ID == "" || !validDigest(query.ProjectionDigest) || query.ExpiresUnixNano <= 0 || !now.Before(time.Unix(0, query.ExpiresUnixNano)) {
		return errors.New("checkpoint current query is incomplete or expired")
	}
	for _, ref := range []CheckpointExactRefV1{query.CheckpointAttempt, query.Barrier, query.EffectCut, query.Reservation, query.Participant} {
		if ref.Revision == 0 || !validDigest(ref.Digest) || !now.Before(time.Unix(0, ref.ExpiresUnixNano)) {
			return errors.New("checkpoint current query exact ref is incomplete or expired")
		}
	}
	switch query.Phase {
	case checkpointPhasePrepareV1:
		if query.PreviousPhase != nil {
			return errors.New("checkpoint prepare query carries previous phase")
		}
	case checkpointPhaseCommitV1, checkpointPhaseAbortV1:
		if query.PreviousPhase == nil || query.PreviousPhase.State != "prepared" || query.PreviousPhase.ClosureID == "" || !validDigest(query.PreviousPhase.ClosureDigest) || !now.Before(time.Unix(0, query.PreviousPhase.ExpiresUnixNano)) {
			return errors.New("checkpoint successor query lacks exact prepared closure")
		}
	default:
		return errors.New("checkpoint current query phase is invalid")
	}
	return nil
}

func factRefAsCheckpointV1(value runtimeports.OperationDispatchSandboxFactRefV4) CheckpointExactRefV1 {
	return CheckpointExactRefV1{ID: value.ID, Revision: uint64(value.Revision), Digest: string(value.Digest), ExpiresUnixNano: value.ExpiresUnixNano}
}

func checkpointExecutionBindingV1(value runtimeports.CurrentCheckpointRestoreDispatchEnforcementV1) ExecutionBindingV1 {
	binding := value.Sandbox.RuntimeLease
	return ExecutionBindingV1{
		TenantID:   string(value.Sandbox.Operation.ExecutionScope.Identity.TenantID),
		InstanceID: string(binding.Instance.ID), InstanceEpoch: uint64(binding.Instance.Epoch),
		LeaseID: string(binding.Lease.ID), LeaseEpoch: uint64(binding.Lease.Epoch), FenceEpoch: uint64(binding.FenceEpoch),
		ScopeDigest: string(binding.ScopeDigest), ObservedRevision: uint64(binding.ObservedRevision), ExpiresUnixNano: binding.Ref.ExpiresUnixNano,
	}
}

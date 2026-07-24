package testkit

import (
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

var FixedNow = time.Unix(1_800_000_000, 0).UTC()

func Ref(id string) contract.Ref {
	digest, err := contract.Digest("test-ref", id)
	if err != nil {
		panic(err)
	}
	return contract.Ref{ID: id, Revision: 1, Digest: digest}
}

func Meta(id string, revision uint64) contract.Meta {
	meta, err := contract.NewMeta(id, revision, FixedNow, FixedNow.Add(24*time.Hour), "test-meta", struct {
		ID       string
		Revision uint64
	}{id, revision})
	if err != nil {
		panic(err)
	}
	return meta
}

func Lease() contract.RuntimeLeaseBinding {
	return contract.RuntimeLeaseBinding{
		TenantID:         "tenant-1",
		InstanceID:       "instance-1",
		InstanceEpoch:    7,
		LeaseID:          "lease-1",
		LeaseEpoch:       11,
		FenceEpoch:       3,
		ScopeDigest:      Ref("scope").Digest,
		ObservedRevision: 5,
		ExpiresUnixNano:  FixedNow.Add(12 * time.Hour).UnixNano(),
	}
}

func Projection() contract.EnvironmentProjection {
	return contract.EnvironmentProjection{
		Meta:  Meta("projection-lease-1", 1),
		Lease: Lease(),
	}
}

func CompleteCleanup() contract.CleanupReport {
	return contract.CleanupReport{
		Processes:          contract.CleanupConfirmedClean,
		FileMounts:         contract.CleanupConfirmedClean,
		Network:            contract.CleanupConfirmedClean,
		Secrets:            contract.CleanupConfirmedClean,
		BackgroundTasks:    contract.CleanupConfirmedClean,
		RemoteContinuation: contract.CleanupConfirmedClean,
		ProviderRetention:  contract.CleanupConfirmedClean,
		EvidenceRefs:       []contract.Ref{Ref("cleanup-evidence")},
	}
}

func Reservation(kind contract.EffectKind, projectionRevision uint64, suffix string) contract.DomainReservation {
	requirement, policy, backend, placement, slot := Requirement(), Policy(), Backend(), Candidate(), Slot()
	return contract.DomainReservation{
		Meta:                       Meta("reservation-"+suffix, 1),
		State:                      contract.CurrentFactActive,
		OperationID:                "operation-" + suffix,
		EffectID:                   "effect-" + suffix,
		IntentRevision:             1,
		IntentDigest:               Ref("intent-" + suffix).Digest,
		AttemptID:                  "attempt-" + suffix,
		AttemptRef:                 Meta("attempt-"+suffix, 1).Ref(),
		Kind:                       kind,
		OperationSubjectDigest:     Ref("subject-" + suffix).Digest,
		ConflictDomain:             kind.ConflictDomain(),
		ConflictScopeDigest:        Ref("tenant-stable-conflict-scope").Digest,
		Lease:                      Lease(),
		RuntimeLeaseBindingRef:     LeaseFact().Meta.Ref(),
		GenerationBindingRef:       Ref("generation-association"),
		RequirementRef:             requirement.Meta.Ref(),
		PolicyRef:                  policy.Meta.Ref(),
		PlacementRef:               placement.Meta.Ref(),
		BackendRef:                 backend.Meta.Ref(),
		SlotRef:                    slot.Meta.Ref(),
		ProviderBinding:            ProviderBinding(),
		ExpectedProjectionRevision: projectionRevision,
		RunID:                      runIDFor(kind),
	}
}

func Attempt(reservation contract.DomainReservation) contract.DomainAttemptFact {
	return contract.DomainAttemptFact{
		Meta:                   Meta(reservation.AttemptID, reservation.AttemptRef.Revision),
		State:                  contract.CurrentFactActive,
		OperationID:            reservation.OperationID,
		EffectID:               reservation.EffectID,
		IntentRevision:         reservation.IntentRevision,
		IntentDigest:           reservation.IntentDigest,
		AttemptID:              reservation.AttemptID,
		ReservationRef:         reservation.Meta.Ref(),
		RuntimeLeaseBindingRef: reservation.RuntimeLeaseBindingRef,
	}
}

func LeaseFact() contract.RuntimeLeaseBindingFact {
	return contract.RuntimeLeaseBindingFact{Meta: Meta("runtime-lease-binding", 1), State: contract.CurrentFactActive, Binding: Lease()}
}

func Observation(reservation contract.DomainReservation, sequence uint64, suffix string) contract.Observation {
	return contract.Observation{
		Meta:                 Meta("observation-"+suffix, 1),
		ReservationRef:       reservation.Meta.Ref(),
		OperationID:          reservation.OperationID,
		AttemptID:            reservation.AttemptID,
		SourceRegistrationID: "source-1",
		SourceEpoch:          1,
		SourceSequence:       sequence,
		PayloadDigest:        Ref(fmt.Sprintf("payload-%d-%s", sequence, suffix)).Digest,
		ReceiptRef:           Ref("receipt-" + suffix),
		EvidenceRefs:         []contract.Ref{Ref("evidence-" + suffix)},
		ObservedState:        "observed",
	}
}

func Inspection(reservation contract.DomainReservation, observation contract.Observation, disposition contract.Disposition, suffix string) contract.InspectionFact {
	return contract.InspectionFact{
		Meta:           Meta("inspection-"+suffix, 1),
		ReservationRef: reservation.Meta.Ref(),
		ObservationRef: observation.Meta.Ref(),
		OperationID:    reservation.OperationID,
		AttemptID:      reservation.AttemptID,
		Disposition:    disposition,
		Coverage:       []string{"attempt", "lease", "provider"},
		EvidenceRefs:   []contract.Ref{Ref("inspect-evidence-" + suffix)},
	}
}

func Result(reservation contract.DomainReservation, inspection contract.InspectionFact, payload contract.DomainResultPayload, suffix string) contract.SandboxDomainResultFact {
	return contract.SandboxDomainResultFact{
		Meta:           Meta("result-"+suffix, 1),
		ReservationRef: reservation.Meta.Ref(),
		InspectionRef:  inspection.Meta.Ref(),
		OperationID:    reservation.OperationID,
		AttemptID:      reservation.AttemptID,
		Kind:           reservation.Kind,
		Disposition:    inspection.Disposition,
		Lease:          reservation.Lease,
		Payload:        payload,
		EvidenceRefs:   []contract.Ref{Ref("result-evidence-" + suffix)},
	}
}

func Settlement(result contract.SandboxDomainResultFact, suffix string) contract.RuntimeOperationSettlementRef {
	return contract.RuntimeOperationSettlementRef{
		OpaqueRef:       Ref("runtime-settlement-" + suffix),
		OperationID:     result.OperationID,
		AttemptID:       result.AttemptID,
		DomainResultRef: result.Meta.Ref(),
	}
}

func runIDFor(kind contract.EffectKind) string {
	if kind == contract.EffectCancel || kind == contract.EffectWorkspaceCommit {
		return "run-1"
	}
	return ""
}

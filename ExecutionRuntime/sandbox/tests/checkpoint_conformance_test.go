package sandbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestCheckpointConformanceIsLocalCompleteAndProviderZero(t *testing.T) {
	t.Parallel()
	participant := testkit.CheckpointParticipant("conformance")
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "conformance", participant, nil)
	report := testkit.CheckpointConformance(reservation)
	port := testkit.NewLocalCheckpointConformance(report)
	var _ ports.CheckpointParticipantConformancePort = port

	got, err := port.AssessCheckpointParticipant(context.Background(), ports.CheckpointConformanceRequest{ReservationRef: reservation.Meta.Ref()})
	if err != nil {
		t.Fatal(err)
	}
	if err := got.ValidateCurrent(testkit.FixedNow); err != nil {
		t.Fatalf("validate report: %v", err)
	}
	if got.ProviderCalls != 0 || got.ProductionProof {
		t.Fatalf("provider_calls=%d production_proof=%v", got.ProviderCalls, got.ProductionProof)
	}
	report.EvidenceRefs[0].ID = "mutated"
	again, err := port.AssessCheckpointParticipant(context.Background(), ports.CheckpointConformanceRequest{ReservationRef: reservation.Meta.Ref()})
	if err != nil || again.EvidenceRefs[0].ID == "mutated" {
		t.Fatalf("conformance retained caller evidence alias: %v", err)
	}

	unsafe := report
	unsafe.ProviderCalls = 1
	unsafePort := testkit.NewLocalCheckpointConformance(unsafe)
	if _, err := unsafePort.AssessCheckpointParticipant(context.Background(), ports.CheckpointConformanceRequest{ReservationRef: reservation.Meta.Ref()}); !errors.Is(err, ports.ErrUnsupported) {
		t.Fatalf("provider-backed report = %v", err)
	}
}

func TestCheckpointFeatureBoundary(t *testing.T) {
	t.Parallel()
	for _, feature := range []ports.Feature{ports.FeatureCheckpointParticipant, ports.FeatureCheckpointRestore, ports.FeatureExternalLifecycle, ports.FeatureRuntimeAdapter, ports.FeatureApplicationAdapter} {
		if !ports.Supported(feature) {
			t.Fatalf("implemented feature %q reported unsupported", feature)
		}
	}
}

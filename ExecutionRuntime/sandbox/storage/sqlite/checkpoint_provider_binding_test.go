package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/applicationadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

func TestCheckpointProviderResultBindingPersistsExactRequestWithoutAliasV2(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := sqlite.OpenWithClock(ctx, path, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	binding := checkpointProviderBindingFixtureV2(t, "persist")
	created, err := store.CreateCheckpointProviderResultBindingV2(ctx, binding)
	if err != nil {
		t.Fatal(err)
	}
	created.Execute.RuntimeCurrentQuery[0] = '{'
	inspected, err := store.InspectCheckpointProviderResultBindingV2(ctx, binding.Reservation)
	if err != nil {
		t.Fatal(err)
	}
	if string(inspected.Execute.RuntimeCurrentQuery) != "{}" {
		t.Fatalf("stored checkpoint binding aliased caller bytes: %q", inspected.Execute.RuntimeCurrentQuery)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.OpenWithClock(ctx, path, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.InspectCheckpointProviderResultBindingV2(ctx, binding.Reservation); err != nil {
		t.Fatalf("checkpoint Provider result binding did not survive restart: %v", err)
	}
	drift := binding
	drift.Execute.RequestID += "-other"
	if _, err := reopened.CreateCheckpointProviderResultBindingV2(ctx, drift); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("same Reservation accepted another Provider request: %v", err)
	}
}

func checkpointProviderBindingFixtureV2(t *testing.T, suffix string) applicationadapter.CheckpointProviderResultBindingV2 {
	t.Helper()
	now := testkit.FixedNow
	digest := func(value string) runtimecore.Digest { return runtimecore.DigestBytes([]byte(value)) }
	attempt := runtimeports.CheckpointAttemptRefV2{TenantID: "tenant-1", ID: "checkpoint-attempt-" + suffix, Revision: 1, Digest: digest("attempt-" + suffix)}
	barrier := runtimeports.CheckpointBarrierLeaseRefV2{TenantID: attempt.TenantID, ID: "barrier-" + suffix, AttemptID: attempt.ID, Revision: 1, Digest: digest("barrier-" + suffix), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	cut := runtimeports.EffectCutRefV2{ID: "cut-" + suffix, Revision: 1, Attempt: attempt, RootDigest: digest("cut-root-" + suffix), Watermark: 1, Digest: digest("cut-" + suffix)}
	reservation := runtimeports.CheckpointParticipantPhaseReservationRefV2{ID: "reservation-" + suffix, Revision: 1, Digest: digest("reservation-" + suffix), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	scopeDigest := digest("scope-" + suffix)
	qualification := runtimeports.CheckpointRestoreEvidenceQualificationRefV1{ID: "qualification-" + suffix, Revision: 1, Attempt: attempt, Barrier: barrier, EffectCut: cut, Reservation: reservation, Phase: runtimeports.CheckpointPhasePrepareV2, ScopeDigest: scopeDigest, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	qualification.Digest, _ = qualification.DigestV1()
	dispatch := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: digest("operation-" + suffix), EffectID: runtimecore.EffectIntentID("effect-" + suffix), IntentRevision: 1, IntentDigest: digest("intent-" + suffix), PermitID: "permit-" + suffix, PermitRevision: 1, PermitDigest: digest("permit-" + suffix), AttemptID: "dispatch-" + suffix}
	handoff := runtimeports.CheckpointRestoreEvidenceProviderHandoffRefV1{ID: "handoff-" + suffix, Revision: 1, Qualification: qualification, Attempt: dispatch, Phase: runtimeports.CheckpointPhasePrepareV2, ScopeDigest: scopeDigest}
	handoff.Digest, _ = handoff.DigestV1()
	source := runtimeports.EvidenceSourceKeyV2{RegistrationID: "source-" + suffix, SourceEpoch: 1, SourceSequence: 1}
	evidence := runtimeports.CheckpointRestoreEvidenceConsumptionRefV1{ID: "consumption-" + suffix, Revision: 1, Qualification: qualification, Handoff: handoff, Record: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: scopeDigest, Sequence: 1, RecordDigest: digest("record-" + suffix)}, Attempt: attempt, Phase: runtimeports.CheckpointPhasePrepareV2, State: runtimeports.CheckpointEvidenceConsumedCurrentV1, ScopeDigest: scopeDigest, Source: source}
	evidence.Digest, _ = evidence.DigestV1()
	if err := evidence.Validate(); err != nil {
		t.Fatal(err)
	}
	return applicationadapter.CheckpointProviderResultBindingV2{Reservation: testkit.Ref("local-reservation-" + suffix), Phase: "prepare", Execute: dataplaneadapter.DispatchRequestV1{ContractVersion: dataplaneadapter.ContractVersionV1, RequestID: "request-" + suffix, Phase: dataplaneadapter.PhaseExecute, RuntimeCurrentQuery: []byte("{}"), Digest: string(digest("dispatch-request-" + suffix))}, Evidence: evidence}
}

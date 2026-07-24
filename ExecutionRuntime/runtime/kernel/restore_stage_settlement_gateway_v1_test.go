package kernel_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRestoreStageSettlementGatewayV1HappyLostReplyCurrentAndConcurrent(t *testing.T) {
	fixture := newRestoreStageSettlementGatewayFixtureV1(t, "happy")
	fixture.store.LoseNextCreateReplyV1()
	ref, err := fixture.gateway.SettleRestoreStageV1(context.Background(), fixture.submission)
	if err != nil || ref.Validate() != nil || ref.ID != fixture.submission.ID {
		t.Fatalf("lost create reply recovery: ref=%+v err=%v", ref, err)
	}
	current, err := fixture.gateway.InspectCurrentRestoreStageSettlementV1(context.Background(), fixture.submission.Operation, fixture.submission.EffectID)
	if err != nil || current != ref {
		t.Fatalf("current Inspect: ref=%+v err=%v", current, err)
	}

	var wg sync.WaitGroup
	errors := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, settleErr := fixture.gateway.SettleRestoreStageV1(context.Background(), fixture.submission)
			if settleErr != nil || got != ref {
				errors <- settleErr
			}
		}()
	}
	wg.Wait()
	close(errors)
	for settleErr := range errors {
		t.Fatalf("concurrent exact retry drifted: %v", settleErr)
	}
	if commits := fixture.store.CreateCommitCountV1(); commits != 1 {
		t.Fatalf("create-once commits=%d want=1", commits)
	}
}

func TestRestoreStageSettlementGatewayV1S1S2DriftWritesNothing(t *testing.T) {
	for _, test := range []struct {
		name  string
		drift func(*restoreStageSettlementGatewayFixtureV1)
	}{
		{name: "governance", drift: func(f *restoreStageSettlementGatewayFixtureV1) { f.governance.driftOnSecond.Store(true) }},
		{name: "domain", drift: func(f *restoreStageSettlementGatewayFixtureV1) { f.domain.driftOnSecond.Store(true) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRestoreStageSettlementGatewayFixtureV1(t, "drift-"+test.name)
			test.drift(&fixture)
			if _, err := fixture.gateway.SettleRestoreStageV1(context.Background(), fixture.submission); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("S1/S2 drift was accepted: %v", err)
			}
			if commits := fixture.store.CreateCommitCountV1(); commits != 0 {
				t.Fatalf("S1/S2 drift wrote %d facts", commits)
			}
		})
	}
}

func TestRestoreStageSettlementGatewayV1RejectsObservationAndDifferentContent(t *testing.T) {
	fixture := newRestoreStageSettlementGatewayFixtureV1(t, "negative")
	fixture.evidence.record.Candidate.TrustClass = ports.EvidenceTrustObservation
	fixture.evidence.record.Candidate.OwnerFact = nil
	digest, err := fixture.evidence.record.Candidate.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	fixture.evidence.record.CandidateDigest = digest
	if _, err := fixture.gateway.SettleRestoreStageV1(context.Background(), fixture.submission); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Observation settled Restore Stage: %v", err)
	}
	if fixture.store.CreateCommitCountV1() != 0 {
		t.Fatal("rejected Observation wrote Settlement")
	}

	fixture = newRestoreStageSettlementGatewayFixtureV1(t, "idempotency")
	if _, err := fixture.gateway.SettleRestoreStageV1(context.Background(), fixture.submission); err != nil {
		t.Fatal(err)
	}
	drift := fixture.submission
	drift.IdempotencyKey = "different-content-same-id"
	drift, err = ports.SealRestoreStageSettlementSubmissionV1(drift)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.gateway.SettleRestoreStageV1(context.Background(), drift); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same Settlement ID changed content was accepted: %v", err)
	}
}

type restoreStageSettlementGatewayFixtureV1 struct {
	now        time.Time
	store      *fakes.RestoreStageSettlementStoreV1
	governance *restoreStageGovernanceReaderV1
	domain     *restoreStageDomainReaderV1
	evidence   *restoreStageEvidenceReaderV1
	gateway    kernel.RestoreStageSettlementGatewayV1
	submission ports.RestoreStageSettlementSubmissionV1
}

func newRestoreStageSettlementGatewayFixtureV1(t *testing.T, suffix string) restoreStageSettlementGatewayFixtureV1 {
	t.Helper()
	now := time.Unix(1_950_300_000, 0).UTC()
	governance, domain, record, submission, err := fakes.BuildRestoreStageSettlementFixtureV1(suffix, now)
	if err != nil {
		t.Fatal(err)
	}
	store := fakes.NewRestoreStageSettlementStoreV1()
	governanceReader := &restoreStageGovernanceReaderV1{projection: governance, now: now}
	domainReader := &restoreStageDomainReaderV1{projection: domain, now: now}
	evidenceReader := &restoreStageEvidenceReaderV1{record: record}
	gateway := kernel.RestoreStageSettlementGatewayV1{Facts: store, Governance: governanceReader, Domains: domainReader, Evidence: evidenceReader, Clock: func() time.Time { return now }}
	return restoreStageSettlementGatewayFixtureV1{now: now, store: store, governance: governanceReader, domain: domainReader, evidence: evidenceReader, gateway: gateway, submission: submission}
}

type restoreStageGovernanceReaderV1 struct {
	projection    ports.RestoreStageGovernanceCurrentProjectionV1
	now           time.Time
	calls         atomic.Int32
	driftOnSecond atomic.Bool
}

func (r *restoreStageGovernanceReaderV1) InspectRestoreStageGovernanceCurrentV1(_ context.Context, request ports.InspectRestoreStageGovernanceCurrentRequestV1) (ports.RestoreStageGovernanceCurrentProjectionV1, error) {
	if err := request.Validate(); err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	projection := r.projection
	call := r.calls.Add(1)
	if r.driftOnSecond.Load() && call == 2 {
		projection.ExpiresUnixNano--
		projection.ProjectionDigest = ""
		return ports.SealRestoreStageGovernanceCurrentProjectionV1(projection, r.now)
	}
	return projection, nil
}

type restoreStageDomainReaderV1 struct {
	projection    ports.RestoreStageDomainResultCurrentProjectionV1
	now           time.Time
	calls         atomic.Int32
	driftOnSecond atomic.Bool
}

func (r *restoreStageDomainReaderV1) InspectRestoreStageDomainResultCurrentV1(_ context.Context, expected ports.RestoreStageDomainResultFactRefV1) (ports.RestoreStageDomainResultCurrentProjectionV1, error) {
	if !ports.SameRestoreStageDomainResultFactRefV1(expected, r.projection.Fact) {
		return ports.RestoreStageDomainResultCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "DomainResult expected ref drifted")
	}
	projection := r.projection
	call := r.calls.Add(1)
	if r.driftOnSecond.Load() && call == 2 {
		projection.ExpiresUnixNano--
		projection.ProjectionDigest = ""
		return ports.SealRestoreStageDomainResultCurrentProjectionV1(projection, r.now)
	}
	return projection, nil
}

type restoreStageEvidenceReaderV1 struct{ record ports.EvidenceLedgerRecordV2 }

func (r *restoreStageEvidenceReaderV1) InspectRecord(_ context.Context, expected ports.EvidenceRecordRefV2) (ports.EvidenceLedgerRecordV2, error) {
	if expected != r.record.Ref {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence expected ref drifted")
	}
	return r.record, nil
}

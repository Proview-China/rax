package kernel_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRunClaimGatewayV2RecoversAppendAndAssociationLostReplies(t *testing.T) {
	now := time.Unix(110_000, 0)
	run, runs := claimRunV2(t, now)
	evidence := newGovernedEvidenceStubV2(now)
	evidence.loseNext = true
	associations := fakes.NewRunClaimAssociationStoreV2()
	associations.LoseNextReply()
	gateway, err := kernel.NewRunClaimGatewayV2(evidence, associations, runs, func() time.Time { return now.Add(time.Hour) })
	if err != nil {
		t.Fatal(err)
	}
	candidate := claimCandidateV2(t, run, 1, "event-claim-lost", core.RunClaimCompleted)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	type ingestResult struct {
		result kernel.RunClaimIngestResultV2
		err    error
	}
	done := make(chan ingestResult, 1)
	go func() {
		result, err := gateway.Ingest(ctx, kernel.RunClaimIngestRequestV2{ExpectedRunRevision: run.Revision, Candidate: candidate})
		done <- ingestResult{result: result, err: err}
	}()
	var result kernel.RunClaimIngestResultV2
	select {
	case completed := <-done:
		if completed.err != nil {
			t.Fatal(completed.err)
		}
		result = completed.result
	case <-ctx.Done():
		t.Fatal("association recovery deadlocked while inspecting exact evidence record")
	}
	if evidence.inspectRecordCount() != 1 {
		t.Fatalf("persisted association recovery must inspect exact evidence record, got %d calls", evidence.inspectRecordCount())
	}
	if result.Association.CreatedUnixNano != result.Evidence.IngestedUnixNano || result.Association.ClaimKind != candidate.ClaimKind {
		t.Fatalf("association must derive immutable ledger time and claim kind: %+v", result.Association)
	}
	replayed, err := gateway.Ingest(context.Background(), kernel.RunClaimIngestRequestV2{ExpectedRunRevision: run.Revision, Candidate: candidate})
	if err != nil || replayed.Association.ID != result.Association.ID {
		t.Fatalf("restart replay must recover exact association: %v %+v", err, replayed)
	}
}

func TestRunClaimGatewayV2AllowsStoppingButRejectsTerminalRace(t *testing.T) {
	now := time.Unix(111_000, 0)
	run, base := claimRunV2(t, now)
	candidate := claimCandidateV2(t, run, 1, "event-stopping", core.RunClaimCancelled)
	stopping := run
	stopping.Status = core.RunStopping
	stopping.Revision++
	runs := &secondReadRunPortV2{RunFactPort: base, first: run, second: stopping}
	gateway, _ := kernel.NewRunClaimGatewayV2(newGovernedEvidenceStubV2(now), fakes.NewRunClaimAssociationStoreV2(), runs, func() time.Time { return now })
	result, err := gateway.Ingest(context.Background(), kernel.RunClaimIngestRequestV2{ExpectedRunRevision: 1, Candidate: candidate})
	if err != nil || result.Association.RunRevisionAtAssociation != 2 {
		t.Fatalf("running to stopping transition must remain associable: %v %+v", err, result)
	}
	terminal := run
	terminal.Status = core.RunTerminal
	terminal.Revision++
	terminal.EndedAt = now
	terminal.Outcome = core.OutcomeCompleted
	runs = &secondReadRunPortV2{RunFactPort: base, first: run, second: terminal}
	gateway, _ = kernel.NewRunClaimGatewayV2(newGovernedEvidenceStubV2(now), fakes.NewRunClaimAssociationStoreV2(), runs, func() time.Time { return now })
	if _, err = gateway.Ingest(context.Background(), kernel.RunClaimIngestRequestV2{ExpectedRunRevision: 1, Candidate: claimCandidateV2(t, run, 1, "event-terminal", core.RunClaimCompleted)}); !core.HasReason(err, core.ReasonRunClaimUnverified) {
		t.Fatalf("terminal race must reject association after preserving evidence: %v", err)
	}
}

func TestRunClaimGatewayV2ClaimKindIsEvidenceBoundAndConcurrentClaimsLinearizeOnce(t *testing.T) {
	now := time.Unix(112_000, 0)
	run, runs := claimRunV2(t, now)
	evidence := newGovernedEvidenceStubV2(now)
	associations := fakes.NewRunClaimAssociationStoreV2()
	gateway, _ := kernel.NewRunClaimGatewayV2(evidence, associations, runs, func() time.Time { return now })
	first := claimCandidateV2(t, run, 1, "event-completed", core.RunClaimCompleted)
	changed := first
	changed.ClaimKind = core.RunClaimFailed
	if _, err := gateway.Ingest(context.Background(), kernel.RunClaimIngestRequestV2{ExpectedRunRevision: 1, Candidate: first}); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.Ingest(context.Background(), kernel.RunClaimIngestRequestV2{ExpectedRunRevision: 1, Candidate: changed}); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("same source evidence cannot be reinterpreted with another claim kind: %v", err)
	}
	run2, runs2 := claimRunV2WithID(t, now, "run-concurrent")
	evidence2 := newGovernedEvidenceStubV2(now)
	associations2 := fakes.NewRunClaimAssociationStoreV2()
	gateway2, _ := kernel.NewRunClaimGatewayV2(evidence2, associations2, runs2, func() time.Time { return now })
	candidates := []ports.EvidenceEventCandidateV2{claimCandidateV2(t, run2, 1, "event-a", core.RunClaimCompleted), claimCandidateV2(t, run2, 2, "event-b", core.RunClaimFailed)}
	var successes int
	var mu sync.Mutex
	var wait sync.WaitGroup
	for _, candidate := range candidates {
		candidate := candidate
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := gateway2.Ingest(context.Background(), kernel.RunClaimIngestRequestV2{ExpectedRunRevision: 1, Candidate: candidate}); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			} else if !core.HasReason(err, core.ReasonRunClaimConflict) {
				t.Errorf("unexpected concurrent claim error: %v", err)
			}
		}()
	}
	wait.Wait()
	if successes != 1 {
		t.Fatalf("one run must linearize one V2 claim association, got %d", successes)
	}
	if evidence2.count() != 2 {
		t.Fatalf("conflicting claim evidence remains in ledger, got %d records", evidence2.count())
	}
}

func TestRunClaimGatewayV2RejectsNonClaimNonRunAndRunIdentityDrift(t *testing.T) {
	now := time.Unix(113_000, 0)
	run, runs := claimRunV2(t, now)
	gateway, _ := kernel.NewRunClaimGatewayV2(newGovernedEvidenceStubV2(now), fakes.NewRunClaimAssociationStoreV2(), runs, func() time.Time { return now })
	nonclaim := claimCandidateV2(t, run, 1, "event-nonclaim", core.RunClaimCompleted)
	nonclaim.TrustClass = ports.EvidenceTrustObservation
	nonclaim.ClaimKind = ""
	if _, err := gateway.Ingest(context.Background(), kernel.RunClaimIngestRequestV2{ExpectedRunRevision: 1, Candidate: nonclaim}); !core.HasReason(err, core.ReasonRunClaimUnverified) {
		t.Fatalf("nonclaim evidence must reject: %v", err)
	}
	nonrun := claimCandidateV2(t, run, 1, "event-nonrun", core.RunClaimCompleted)
	nonrun.LedgerScope.Partition = ports.EvidencePartitionInstance
	nonrun.LedgerScope.RunID = ""
	if _, err := gateway.Ingest(context.Background(), kernel.RunClaimIngestRequestV2{ExpectedRunRevision: 1, Candidate: nonrun}); !core.HasReason(err, core.ReasonRunClaimUnverified) {
		t.Fatalf("non-run ledger evidence must reject: %v", err)
	}
	drift := run
	drift.SessionRef = "changed-session"
	transition := &secondReadRunPortV2{RunFactPort: runs, first: run, second: drift}
	gateway, _ = kernel.NewRunClaimGatewayV2(newGovernedEvidenceStubV2(now), fakes.NewRunClaimAssociationStoreV2(), transition, func() time.Time { return now })
	if _, err := gateway.Ingest(context.Background(), kernel.RunClaimIngestRequestV2{ExpectedRunRevision: 1, Candidate: claimCandidateV2(t, run, 1, "event-identity-drift", core.RunClaimCompleted)}); !core.HasReason(err, core.ReasonRunClaimUnverified) {
		t.Fatalf("run identity drift before association must reject: %v", err)
	}
}

func TestRunClaimGatewayV2HighSourceCoordinatesAndAssociationIdentityAreCanonical(t *testing.T) {
	now := time.Unix(114_000, 0)
	run, runs := claimRunV2(t, now)
	evidence := newGovernedEvidenceStubV2(now)
	evidence.loseNext = true
	gateway, _ := kernel.NewRunClaimGatewayV2(evidence, fakes.NewRunClaimAssociationStoreV2(), runs, func() time.Time { return now })
	candidate := claimCandidateV2(t, run, ^uint64(0)-2, "event-high-coordinate", core.RunClaimCompleted)
	candidate.SourceEpoch = core.Epoch(^uint64(0) - 3)
	result, err := gateway.Ingest(context.Background(), kernel.RunClaimIngestRequestV2{ExpectedRunRevision: 1, Candidate: candidate})
	if err != nil {
		t.Fatal(err)
	}
	expected, err := ports.RunClaimAssociationIDV2(run.ID, result.Evidence.Ref)
	if err != nil || result.Association.ID != expected {
		t.Fatalf("association id must be canonical digest identity: %v %+v", err, result.Association)
	}
	forged := result.Association
	forged.Evidence.RecordDigest = claimDigestV2(t, "forged-record")
	if err := forged.Validate(); !core.HasReason(err, core.ReasonRunClaimConflict) {
		t.Fatalf("forged evidence ref must invalidate association identity: %v", err)
	}
}

func TestRunClaimGatewayV2RecoveryRejectsForgedAssociationFields(t *testing.T) {
	now := time.Unix(115_000, 0)
	run, runs := claimRunV2WithID(t, now, "run-forged-association")
	evidence := newGovernedEvidenceStubV2(now)
	baseStore := fakes.NewRunClaimAssociationStoreV2()
	baseGateway, _ := kernel.NewRunClaimGatewayV2(evidence, baseStore, runs, func() time.Time { return now })
	candidate := claimCandidateV2(t, run, 1, "event-forged-association", core.RunClaimCompleted)
	base, err := baseGateway.Ingest(context.Background(), kernel.RunClaimIngestRequestV2{ExpectedRunRevision: 1, Candidate: candidate})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*ports.RunClaimAssociationFactV2)
	}{
		{"registration", func(f *ports.RunClaimAssociationFactV2) { f.RegistrationID = "other-registration" }},
		{"source", func(f *ports.RunClaimAssociationFactV2) { f.SourceID = "other/source" }},
		{"source epoch", func(f *ports.RunClaimAssociationFactV2) { f.SourceEpoch++ }},
		{"source sequence", func(f *ports.RunClaimAssociationFactV2) { f.SourceSequence++ }},
		{"event", func(f *ports.RunClaimAssociationFactV2) { f.EventID = "other-event" }},
		{"claim kind", func(f *ports.RunClaimAssociationFactV2) { f.ClaimKind = core.RunClaimFailed }},
		{"observed time", func(f *ports.RunClaimAssociationFactV2) { f.ObservedUnixNano-- }},
		{"ingested time", func(f *ports.RunClaimAssociationFactV2) { f.EvidenceIngestedUnixNano++; f.CreatedUnixNano++ }},
		{"candidate digest", func(f *ports.RunClaimAssociationFactV2) { f.CandidateDigest = claimDigestV2(t, "other-candidate") }},
		{"payload digest", func(f *ports.RunClaimAssociationFactV2) { f.PayloadDigest = claimDigestV2(t, "other-payload") }},
		{"execution scope", func(f *ports.RunClaimAssociationFactV2) {
			f.ExecutionScope.Instance.ID = "other-instance"
			f.ExecutionScopeDigest, _ = ports.ExecutionScopeDigestV2(f.ExecutionScope)
		}},
		{"run id", func(f *ports.RunClaimAssociationFactV2) {
			f.RunID = "other-run"
			f.ID, _ = ports.RunClaimAssociationIDV2(f.RunID, f.Evidence)
		}},
		{"ledger record ref", func(f *ports.RunClaimAssociationFactV2) {
			f.Evidence.RecordDigest = claimDigestV2(t, "other-record")
			f.ID, _ = ports.RunClaimAssociationIDV2(f.RunID, f.Evidence)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			forged := base.Association
			test.mutate(&forged)
			associations := forgedAssociationPortV2{fact: forged}
			gateway, constructErr := kernel.NewRunClaimGatewayV2(evidence, associations, runs, func() time.Time { return now })
			if constructErr != nil {
				t.Fatal(constructErr)
			}
			if _, ingestErr := gateway.Ingest(context.Background(), kernel.RunClaimIngestRequestV2{ExpectedRunRevision: 1, Candidate: candidate}); ingestErr == nil {
				t.Fatal("forged persisted association must fail closed")
			}
		})
	}
}

type governedEvidenceStubV2 struct {
	mu                 sync.Mutex
	now                time.Time
	records            map[string]ports.EvidenceLedgerRecordV2
	loseNext           bool
	nextLedger         uint64
	last               core.Digest
	inspectRecordCalls int
}

type forgedAssociationPortV2 struct {
	fact ports.RunClaimAssociationFactV2
}

func (forgedAssociationPortV2) CreateRunClaimAssociation(context.Context, ports.RunClaimAssociationFactV2) (ports.RunClaimAssociationFactV2, error) {
	return ports.RunClaimAssociationFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected association uncertainty")
}
func (p forgedAssociationPortV2) InspectRunClaimAssociation(context.Context, core.Digest, core.AgentRunID) (ports.RunClaimAssociationFactV2, error) {
	return p.fact, nil
}

func newGovernedEvidenceStubV2(now time.Time) *governedEvidenceStubV2 {
	return &governedEvidenceStubV2{now: now, records: map[string]ports.EvidenceLedgerRecordV2{}}
}
func (s *governedEvidenceStubV2) RegisterGovernedSource(context.Context, ports.EvidenceSourceRegistrationFactV2) (ports.EvidenceSourceRegistrationFactV2, error) {
	panic("unused")
}
func (s *governedEvidenceStubV2) RenewGovernedSource(context.Context, ports.EvidenceSourceCASRequestV2) (ports.EvidenceSourceRegistrationFactV2, error) {
	panic("unused")
}
func (s *governedEvidenceStubV2) AppendLateGoverned(context.Context, ports.EvidenceAppendLateRequestV2) (ports.EvidenceLedgerRecordV2, error) {
	panic("unused")
}
func (s *governedEvidenceStubV2) AppendGoverned(_ context.Context, request ports.EvidenceAppendRequestV2) (ports.EvidenceLedgerRecordV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := claimSourceKeyV2(request.Candidate)
	if existing, ok := s.records[key]; ok {
		digest, _ := request.Candidate.DigestV2()
		if digest == existing.CandidateDigest {
			return existing, nil
		}
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "changed source content")
	}
	s.nextLedger++
	previous := s.last
	if s.nextLedger == 1 {
		previous = ports.EvidenceGenesisDigestV2
	}
	record, err := control.NewEvidenceLedgerRecordV2(request.Candidate, s.nextLedger, previous, s.now)
	if err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	s.records[key] = record
	s.last = record.Ref.RecordDigest
	if s.loseNext {
		s.loseNext = false
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost append reply")
	}
	return record, nil
}
func (s *governedEvidenceStubV2) InspectGovernedBySource(_ context.Context, key ports.EvidenceSourceKeyV2) (ports.EvidenceLedgerRecordV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[key.RegistrationID+"/"+strconv.FormatUint(uint64(key.SourceEpoch), 10)+"/"+strconv.FormatUint(key.SourceSequence, 10)]
	if !ok {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "not found")
	}
	return record, nil
}
func (s *governedEvidenceStubV2) InspectGovernedRecord(_ context.Context, ref ports.EvidenceRecordRefV2) (ports.EvidenceLedgerRecordV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inspectRecordCalls++
	for _, record := range s.records {
		if record.Ref == ref {
			return record, nil
		}
	}
	return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "not found")
}
func (s *governedEvidenceStubV2) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.records) }
func (s *governedEvidenceStubV2) inspectRecordCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inspectRecordCalls
}
func claimSourceKeyV2(c ports.EvidenceEventCandidateV2) string {
	return c.RegistrationID + "/" + strconv.FormatUint(uint64(c.SourceEpoch), 10) + "/" + strconv.FormatUint(c.SourceSequence, 10)
}

type secondReadRunPortV2 struct {
	control.RunFactPort
	mu            sync.Mutex
	calls         int
	first, second core.AgentRunRecord
}

func (s *secondReadRunPortV2) InspectRun(context.Context, core.ExecutionScope, core.AgentRunID) (core.AgentRunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.calls == 1 {
		return s.first, nil
	}
	return s.second, nil
}

func claimRunV2(t *testing.T, now time.Time) (core.AgentRunRecord, *fakes.FactStore) {
	return claimRunV2WithID(t, now, "run-claim-v2")
}
func claimRunV2WithID(t *testing.T, now time.Time, id core.AgentRunID) (core.AgentRunRecord, *fakes.FactStore) {
	t.Helper()
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-claim", ID: "identity-claim", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-claim", PlanDigest: claimDigestV2(t, "claim-plan")}, Instance: core.InstanceRef{ID: "instance-claim", Epoch: 1}, AuthorityEpoch: 1}
	run := core.AgentRunRecord{ID: id, Scope: scope, Status: core.RunRunning, Revision: 1, SessionRef: "session-claim", StartedAt: now.Add(-time.Second)}
	store := fakes.NewFactStore(func() time.Time { return now })
	if _, err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	return run, store
}
func claimCandidateV2(t *testing.T, run core.AgentRunRecord, sequence uint64, event string, kind core.RunCompletionClaimKind) ports.EvidenceEventCandidateV2 {
	t.Helper()
	schema := ports.SchemaRefV2{Namespace: "runtime", Name: "run-claim", Version: "2.0.0", MediaType: "application/octet-stream", ContentDigest: claimDigestV2(t, "claim-schema")}
	return ports.EvidenceEventCandidateV2{ContractVersion: ports.EvidenceContractVersionV2, LedgerScope: ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionRun, TenantID: run.Scope.Identity.TenantID, IdentityID: run.Scope.Identity.ID, LineageID: run.Scope.Lineage.ID, InstanceID: run.Scope.Instance.ID, RunID: run.ID}, EventID: event, RegistrationID: "claim-registration", RegistrationRevision: 1, SourceConfigurationDigest: claimDigestV2(t, "claim-config"), SourcePolicy: ports.EvidenceSourcePolicyBindingRefV2{Ref: "claim-policy", Digest: claimDigestV2(t, "claim-policy"), Revision: 1}, SourceID: "harness/run-claim", SourceEpoch: 9, SourceSequence: sequence, TrustClass: ports.EvidenceTrustClaim, ClaimKind: kind, EventKind: "runtime/run-completion", CustomClass: "runtime/claim", ExecutionScope: run.Scope, Payload: ports.EvidencePayloadRefV2{Schema: schema, ContentDigest: claimDigestV2(t, event), Revision: 1, Length: 1, Ref: "memory://" + event}, Causation: []ports.EvidenceCausationRefV2{}, CorrelationID: string(run.ID), Producer: ports.EvidenceProducerBindingRefV2{BindingSetID: "claim-binding", BindingSetRevision: 1, ComponentID: "runtime/harness", ManifestDigest: claimDigestV2(t, "claim-manifest"), ArtifactDigest: claimDigestV2(t, "claim-artifact"), Capability: "runtime/claim"}, Authority: ports.AuthorityBindingRefV2{Ref: "claim-authority", Digest: claimDigestV2(t, "claim-authority"), Revision: 1, Epoch: run.Scope.AuthorityEpoch}, ObservedUnixNano: run.StartedAt.Add(time.Second).UnixNano()}
}
func claimDigestV2(t *testing.T, value string) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

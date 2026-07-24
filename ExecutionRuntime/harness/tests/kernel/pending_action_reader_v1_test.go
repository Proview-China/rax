package kernel_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCommittedPendingActionReaderV1ReturnsCanonicalShortLivedProjection(t *testing.T) {
	now, session, request := committedPendingActionFixtureV1(t)
	store := &pendingActionSessionPortV1{sessions: []contract.GovernedSessionV2{session}}
	reader, err := kernel.NewCommittedPendingActionReaderV1(store, func() time.Time { return now }, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	projection, err := reader.InspectCommittedPendingActionCurrentV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projection.PendingAction, *session.PendingAction) || projection.SessionDigest == "" || projection.Phase != contract.SessionWaitingActionV2 || projection.CheckedUnixNano != now.UnixNano() || projection.ExpiresUnixNano != now.Add(10*time.Second).UnixNano() {
		t.Fatalf("projection omitted exact committed action fields: %#v", projection)
	}
	if projection.SessionApplicability.Kind != contract.CommittedPendingActionSessionKindV1 || projection.TurnApplicability.Kind != contract.CommittedPendingActionTurnKindV1 || projection.SessionApplicability.Digest == projection.TurnApplicability.Digest {
		t.Fatalf("Session/Turn source coordinates were not domain-separated: %#v %#v", projection.SessionApplicability, projection.TurnApplicability)
	}
	expectedSessionDigest, err := core.CanonicalJSONDigest("praxis.harness.committed-pending-action-session-coordinate", contract.CommittedPendingActionReaderContractVersionV1, "CommittedPendingActionSessionApplicabilityCoordinateV1", struct {
		Run                  contract.RunRef         `json:"run"`
		ExecutionScopeDigest core.Digest             `json:"execution_scope_digest"`
		SessionID            string                  `json:"session_id"`
		SessionRevision      core.Revision           `json:"session_revision"`
		SessionDigest        core.Digest             `json:"session_digest"`
		Phase                contract.SessionPhaseV2 `json:"phase"`
		PendingActionRef     string                  `json:"pending_action_ref"`
		PendingActionDigest  core.Digest             `json:"pending_action_digest"`
	}{projection.Run, projection.ExecutionScopeDigest, projection.SessionID, projection.SessionRevision, projection.SessionDigest, projection.Phase, projection.PendingAction.Ref, projection.PendingAction.RequestDigest})
	if err != nil || projection.SessionApplicability.Digest != expectedSessionDigest {
		t.Fatalf("Session source coordinate did not use its exact canonical domain: digest=%q err=%v", projection.SessionApplicability.Digest, err)
	}
	expectedTurnDigest, err := core.CanonicalJSONDigest("praxis.harness.committed-pending-action-turn-coordinate", contract.CommittedPendingActionReaderContractVersionV1, "CommittedPendingActionTurnApplicabilityCoordinateV1", struct {
		Session             contract.CommittedPendingActionSessionApplicabilityCoordinateV1 `json:"session"`
		Turn                uint32                                                          `json:"turn"`
		PendingActionRef    string                                                          `json:"pending_action_ref"`
		PendingActionDigest core.Digest                                                     `json:"pending_action_digest"`
	}{projection.SessionApplicability, projection.Turn, projection.PendingAction.Ref, projection.PendingAction.RequestDigest})
	if err != nil || projection.TurnApplicability.Digest != expectedTurnDigest {
		t.Fatalf("Turn source coordinate did not use its exact canonical domain: digest=%q err=%v", projection.TurnApplicability.Digest, err)
	}
	if reflect.TypeOf(projection.SessionApplicability) == reflect.TypeOf(runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{}) || reflect.TypeOf(projection.TurnApplicability) == reflect.TypeOf(runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{}) {
		t.Fatal("Harness Reader leaked a Runtime applicability reference")
	}
	if err := projection.Validate(request, now); err != nil {
		t.Fatalf("sealed projection did not validate: %v", err)
	}
	if err := projection.Validate(request, time.Unix(0, projection.ExpiresUnixNano)); !core.HasReason(err, core.ReasonEffectFenceStale) {
		t.Fatalf("expired projection remained current: %v", err)
	}
	if store.inspectCalls() != 2 || store.writeCalls() != 0 {
		t.Fatalf("reader must perform exactly S1/S2 and no writes: inspect=%d write=%d", store.inspectCalls(), store.writeCalls())
	}

	replayed, err := reader.InspectCommittedPendingActionCurrentV1(context.Background(), request)
	if err != nil || replayed.Digest != projection.Digest {
		t.Fatalf("same exact read was not canonical: digest=%q err=%v", replayed.Digest, err)
	}

	typePunned := projection
	typePunned.SessionApplicability = contract.CommittedPendingActionSessionApplicabilityCoordinateV1{
		Kind: projection.TurnApplicability.Kind, ID: projection.TurnApplicability.ID,
		Revision: projection.TurnApplicability.Revision, Digest: projection.TurnApplicability.Digest,
	}
	if err := typePunned.Validate(request, now); !core.HasReason(err, core.ReasonInvalidReference) {
		t.Fatalf("Session/Turn applicability type-pun was accepted: %v", err)
	}
}

func TestCommittedPendingActionReaderV1RejectsRequestDriftBeforeOrDuringInspect(t *testing.T) {
	now, session, request := committedPendingActionFixtureV1(t)
	tests := []struct {
		name   string
		mutate func(*contract.InspectCommittedPendingActionCurrentRequestV1)
	}{
		{"revision", func(r *contract.InspectCommittedPendingActionCurrentRequestV1) { r.ExpectedSessionRevision++ }},
		{"turn", func(r *contract.InspectCommittedPendingActionCurrentRequestV1) { r.ExpectedTurn++ }},
		{"pending-ref", func(r *contract.InspectCommittedPendingActionCurrentRequestV1) {
			r.ExpectedPendingActionRef = "action-other"
		}},
		{"pending-digest-type-pun", func(r *contract.InspectCommittedPendingActionCurrentRequestV1) {
			r.ExpectedPendingActionDigest, _ = session.DigestV2()
		}},
		{"checked-at", func(r *contract.InspectCommittedPendingActionCurrentRequestV1) { r.CheckedAtUnixNano++ }},
		{"forged-scope", func(r *contract.InspectCommittedPendingActionCurrentRequestV1) {
			r.Run.Scope.AuthorityEpoch++
			r.ExecutionScopeDigest, _ = runtimeports.ExecutionScopeDigestV2(r.Run.Scope)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := request
			test.mutate(&changed)
			store := &pendingActionSessionPortV1{sessions: []contract.GovernedSessionV2{session}}
			reader, _ := kernel.NewCommittedPendingActionReaderV1(store, func() time.Time { return now }, time.Second)
			if _, err := reader.InspectCommittedPendingActionCurrentV1(context.Background(), changed); err == nil {
				t.Fatal("drifted request was accepted")
			}
			if store.writeCalls() != 0 {
				t.Fatal("rejected read touched a write API")
			}
		})
	}
}

func TestCommittedPendingActionReaderV1FailsClosedOnS1S2Drift(t *testing.T) {
	now, session, request := committedPendingActionFixtureV1(t)
	alternate, err := contract.NewPendingActionV2("action-other", session.PendingAction.Capability, session.PendingAction.Payload, session.PendingAction.SourceCandidate)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*contract.GovernedSessionV2)
	}{
		{"revision", func(s *contract.GovernedSessionV2) { s.Revision++ }},
		{"session-digest", func(s *contract.GovernedSessionV2) { s.UpdatedUnixNano++ }},
		{"phase", func(s *contract.GovernedSessionV2) { s.Phase = contract.SessionWaitingSettlementV2 }},
		{"turn", func(s *contract.GovernedSessionV2) { s.Turn++ }},
		{"pending-action", func(s *contract.GovernedSessionV2) { s.PendingAction = &alternate }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s2 := session
			test.mutate(&s2)
			store := &pendingActionSessionPortV1{sessions: []contract.GovernedSessionV2{session, s2}}
			reader, _ := kernel.NewCommittedPendingActionReaderV1(store, func() time.Time { return now }, time.Second)
			if _, err := reader.InspectCommittedPendingActionCurrentV1(context.Background(), request); err == nil {
				t.Fatal("S1/S2 drift was accepted")
			}
			if store.inspectCalls() != 2 || store.writeCalls() != 0 {
				t.Fatalf("drift path did not remain read-only S1/S2: inspect=%d write=%d", store.inspectCalls(), store.writeCalls())
			}
		})
	}
}

func TestCommittedPendingActionReaderV1UnavailableAndConfigurationFailClosed(t *testing.T) {
	now, _, request := committedPendingActionFixtureV1(t)
	unavailable := &pendingActionSessionPortV1{inspectErr: core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "injected reader unavailable")}
	reader, err := kernel.NewCommittedPendingActionReaderV1(unavailable, func() time.Time { return now }, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.InspectCommittedPendingActionCurrentV1(context.Background(), request); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("unavailable Session reader was not preserved: %v", err)
	}
	if unavailable.writeCalls() != 0 {
		t.Fatal("unavailable read attempted a write")
	}
	if _, err := kernel.NewCommittedPendingActionReaderV1(nil, func() time.Time { return now }, time.Second); err == nil {
		t.Fatal("nil Session reader was accepted")
	}
	if _, err := kernel.NewCommittedPendingActionReaderV1(unavailable, func() time.Time { return now }, kernel.MaxCommittedPendingActionProjectionTTLV1+time.Nanosecond); err == nil {
		t.Fatal("unbounded projection TTL was accepted")
	}
}

func TestCommittedPendingActionReaderV1ConcurrentReadsAreStable(t *testing.T) {
	now, session, request := committedPendingActionFixtureV1(t)
	store := &pendingActionSessionPortV1{sessions: []contract.GovernedSessionV2{session}}
	reader, _ := kernel.NewCommittedPendingActionReaderV1(store, func() time.Time { return now }, time.Second)
	const workers = 64
	start := make(chan struct{})
	results := make(chan contract.CommittedPendingActionCurrentV1, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			<-start
			projection, err := reader.InspectCommittedPendingActionCurrentV1(context.Background(), request)
			results <- projection
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errs)
	var digest core.Digest
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for result := range results {
		if digest == "" {
			digest = result.Digest
		}
		if result.Digest != digest {
			t.Fatal("concurrent current reads returned different canonical projections")
		}
	}
	if store.inspectCalls() != workers*2 || store.writeCalls() != 0 {
		t.Fatalf("concurrent reader did not remain two-read/zero-write: inspect=%d write=%d", store.inspectCalls(), store.writeCalls())
	}
}

func committedPendingActionFixtureV1(t *testing.T) (time.Time, contract.GovernedSessionV2, contract.InspectCommittedPendingActionCurrentRequestV1) {
	t.Helper()
	now := time.Unix(2_000_000_100, 0)
	session, candidate := testkit.GovernedFactsV2(now.Add(-time.Minute))
	candidateRef, err := candidate.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := contract.NewPendingActionV2("action-reader", "custom.tool/execute", candidate.Input, candidateRef)
	if err != nil {
		t.Fatal(err)
	}
	turn := contract.SettledTurnResultV2{ContractVersion: contract.SettledTurnResultContractV2, Candidate: candidateRef, State: contract.SettledTurnActionRequiredV2, Action: &pending}
	settled, _ := testkit.GovernedSettledAttemptRefsV2(now.Add(-time.Minute), candidate, turn, runtimeports.OperationSettlementAppliedV3)
	session.Revision = 5
	session.Phase = contract.SessionWaitingActionV2
	session.Turn = 1
	session.Candidate = nil
	session.DomainReservation = nil
	session.Execution = &settled
	session.PendingAction = &pending
	session.UpdatedUnixNano = now.Add(-time.Second).UnixNano()
	if err := session.Validate(); err != nil {
		t.Fatalf("waiting_action fixture is invalid: %v", err)
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(session.Run.Scope)
	if err != nil {
		t.Fatal(err)
	}
	request := contract.InspectCommittedPendingActionCurrentRequestV1{
		ContractVersion: contract.CommittedPendingActionReaderContractVersionV1,
		Run:             session.Run, ExecutionScopeDigest: scopeDigest, SessionID: session.ID,
		ExpectedSessionRevision: session.Revision, ExpectedTurn: session.Turn,
		ExpectedPendingActionRef: pending.Ref, ExpectedPendingActionDigest: pending.RequestDigest,
		CheckedAtUnixNano: now.UnixNano(),
	}
	return now, session, request
}

type pendingActionSessionPortV1 struct {
	mu         sync.Mutex
	sessions   []contract.GovernedSessionV2
	inspectErr error
	reads      int
	writes     int
}

func (p *pendingActionSessionPortV1) CreateSessionV2(context.Context, contract.GovernedSessionV2) (contract.GovernedSessionV2, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes++
	return contract.GovernedSessionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "unexpected Session write")
}

func (p *pendingActionSessionPortV1) InspectSessionV2(context.Context, contract.RunRef, string) (contract.GovernedSessionV2, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reads++
	if p.inspectErr != nil {
		return contract.GovernedSessionV2{}, p.inspectErr
	}
	if len(p.sessions) == 0 {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "test Session missing")
	}
	index := p.reads - 1
	if index >= len(p.sessions) {
		index = len(p.sessions) - 1
	}
	return p.sessions[index], nil
}

func (p *pendingActionSessionPortV1) CompareAndSwapSessionV2(context.Context, harnessports.SessionCASRequestV2) (contract.GovernedSessionV2, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes++
	return contract.GovernedSessionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "unexpected Session CAS")
}

func (p *pendingActionSessionPortV1) inspectCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reads
}

func (p *pendingActionSessionPortV1) writeCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.writes
}

package runtimeintegration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestUnitOperationReviewCurrentReaderV4ExactAcceptedProjection(t *testing.T) {
	fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
	source := newAtomicSourceV4(t, fixture)
	reader := mustReaderV4(t, source, func() time.Time { return fixture.now })

	projection, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent)
	if err != nil {
		t.Fatal(err)
	}
	intentDigest, _ := fixture.intent.DigestV3()
	if projection.Basis != runtimeports.OperationReviewBasisAcceptedV4 || projection.IntentDigest != intentDigest || projection.Target.Ref != fixture.snapshot.Target.ID || projection.Case.Ref != fixture.snapshot.Case.ID || projection.Verdict.Ref != fixture.snapshot.Verdict.ID || projection.CurrentnessDigest != fixture.snapshot.Digest || projection.ExpiresUnixNano != fixture.snapshot.ExpiresUnixNano || source.callCount() != 1 {
		t.Fatalf("unexpected exact projection: %+v calls=%d", projection, source.callCount())
	}
	if err := projection.Validate(fixture.now); err != nil {
		t.Fatal(err)
	}
	var _ runtimeports.OperationReviewCurrentReaderV4 = reader
}

func TestBlackboxOperationReviewCurrentReaderV4AllowList(t *testing.T) {
	tests := []struct {
		name        string
		state       contract.VerdictStateV1
		notRequired bool
		basis       runtimeports.OperationReviewAuthorizationBasisV4
		satisfied   bool
	}{
		{name: "accepted", state: contract.VerdictAcceptedV1, basis: runtimeports.OperationReviewBasisAcceptedV4},
		{name: "conditional_satisfied", state: contract.VerdictConditionalV1, basis: runtimeports.OperationReviewBasisConditionalSatisfiedV4, satisfied: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixtureV4(t, test.state, test.notRequired)
			reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
			projection, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent)
			if err != nil {
				t.Fatal(err)
			}
			if projection.Basis != test.basis || (projection.Satisfaction != nil) != test.satisfied {
				t.Fatalf("basis/satisfaction mismatch: %+v", projection)
			}
		})
	}
	t.Run("operation_not_required_without_independent_fact", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictAcceptedV1, true)
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("accepted Verdict forged operation_not_required: %v", err)
		}
	})
}

func TestBlackboxOperationReviewCurrentReaderV4RejectsHistoricalRuntimeV3AutoAttestation(t *testing.T) {
	fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
	legacy := fixture.snapshot.Attestations[0]
	legacy.Route = contract.RouteAutoV1
	legacy.DomainApplySettlement = &contract.DomainApplySettlementRefV1{
		ID:                        "legacy-auto-apply",
		Revision:                  1,
		Digest:                    digest("legacy-auto-apply"),
		DomainResultID:            "legacy-auto-result",
		DomainResultDigest:        digest("legacy-auto-result"),
		RuntimeSettlementID:       "legacy-runtime-v3-settlement",
		RuntimeSettlementRevision: 1,
		RuntimeSettlementDigest:   digest("legacy-runtime-v3-settlement"),
		State:                     contract.DomainApplyAppliedV1,
	}
	legacy.ReviewerAttemptID = "legacy-runtime-v3-attempt"
	legacy.ReviewerResultDigest = digest("legacy-auto-result-payload")
	legacy.AutoProvenance = nil
	legacy.Digest = ""
	var err error
	legacy, err = contract.SealAttestationV1(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Validate(); err != nil {
		t.Fatalf("historical Runtime V3 Auto Attestation is no longer readable: %v", err)
	}
	if err := legacy.ValidateProductionAutoProvenanceV4(); err == nil {
		t.Fatal("historical Runtime V3 Auto Attestation passed production admission")
	}
	fixture.snapshot.Attestations[0] = legacy
	fixture.snapshot = resealSnapshot(t, fixture.snapshot)
	reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
	projection, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent)
	if err == nil || projection.Current {
		t.Fatalf("historical Runtime V3 Auto Attestation was upgraded to current authorization: projection=%+v err=%v", projection, err)
	}
}

func TestWhiteboxOperationReviewCurrentReaderV4StatesFailClosed(t *testing.T) {
	states := []contract.VerdictStateV1{contract.VerdictRejectedV1, contract.VerdictExpiredV1, contract.VerdictRevokedV1, contract.VerdictSupersededV1}
	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			fixture := newFixtureV4(t, state, false)
			reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
			if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonReviewVerdictStale) {
				t.Fatalf("state %s did not fail closed: %v", state, err)
			}
		})
	}

	t.Run("pending_case", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
		pending := fixture.snapshot.Case
		pending.State = contract.CaseWaitingEvidenceV1
		pending.VerdictID, pending.VerdictRevision, pending.VerdictDigest = "", 0, ""
		pending.Digest = ""
		var err error
		pending, err = contract.SealReviewCaseV1(pending)
		if err != nil {
			t.Fatal(err)
		}
		fixture.snapshot.Case = pending
		fixture.snapshot = resealSnapshot(t, fixture.snapshot)
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("pending case did not fail closed: %v", err)
		}
	})

	t.Run("unknown_verdict", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
		fixture.snapshot.Verdict.State = contract.VerdictStateV1("unknown")
		fixture.snapshot = resealSnapshot(t, fixture.snapshot)
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); err == nil {
			t.Fatal("unknown Verdict state authorized")
		}
	})

	t.Run("resealed_case_revision_drift", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
		resealed := fixture.snapshot.Case
		resealed.Revision++
		resealed.Digest = ""
		var err error
		resealed, err = contract.SealReviewCaseV1(resealed)
		if err != nil {
			t.Fatal(err)
		}
		if resealed.VerdictDigest != fixture.snapshot.Verdict.Digest || resealed.Revision == fixture.snapshot.Verdict.CaseRevision+1 {
			t.Fatal("counterexample did not preserve Verdict while drifting the resolved Case revision")
		}
		fixture.snapshot.Case = resealed
		fixture.snapshot = resealSnapshot(t, fixture.snapshot)
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("re-sealed Case revision drift did not fail closed: %v", err)
		}
	})
}

func TestWhiteboxOperationReviewCurrentReaderV4DriftMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*runtimeadapter.CurrentFactSnapshotV4)
	}{
		{name: "target", mutate: func(s *runtimeadapter.CurrentFactSnapshotV4) { s.Target.PayloadRevision++ }},
		{name: "evidence", mutate: func(s *runtimeadapter.CurrentFactSnapshotV4) {
			s.DecisionEvidence[0].Review.Digest = digest("other-review-evidence")
		}},
		{name: "policy", mutate: func(s *runtimeadapter.CurrentFactSnapshotV4) { s.Policy.Active = false }},
		{name: "reviewer_authority", mutate: func(s *runtimeadapter.CurrentFactSnapshotV4) { s.ReviewerAuthority.Digest = digest("other-authority") }},
		{name: "scope", mutate: func(s *runtimeadapter.CurrentFactSnapshotV4) { s.Scope.Digest = digest("other-scope") }},
		{name: "binding", mutate: func(s *runtimeadapter.CurrentFactSnapshotV4) { s.Binding.Ref = "other-binding" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
			test.mutate(&fixture.snapshot)
			fixture.snapshot = resealSnapshot(t, fixture.snapshot)
			reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
			if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); err == nil {
				t.Fatalf("%s drift authorized", test.name)
			}
		})
	}
}

func TestWhiteboxOperationReviewCurrentReaderV4ConditionalFailures(t *testing.T) {
	t.Run("pending_satisfaction", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictConditionalV1, false)
		fact := *fixture.snapshot.Satisfaction
		fact.State = runtimeports.ConditionSatisfactionPending
		fact.Proofs = nil
		fact.ProofsDigest = ""
		fact.SatisfiedUnixNano = 0
		fixture.snapshot.Satisfaction = &fact
		fixture.snapshot.SatisfactionEvidence = nil
		fixture.snapshot = resealSnapshot(t, fixture.snapshot)
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonReviewConditionUnsatisfied) {
			t.Fatalf("pending Satisfaction did not fail closed: %v", err)
		}
	})

	t.Run("missing_satisfaction_evidence", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictConditionalV1, false)
		fixture.snapshot.SatisfactionEvidence = nil
		fixture.snapshot = resealSnapshot(t, fixture.snapshot)
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonEvidenceUnavailable) {
			t.Fatalf("missing Satisfaction evidence did not fail closed: %v", err)
		}
	})

	t.Run("not_required_policy_with_conditional_verdict", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictConditionalV1, true)
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("not-required Policy combined with conditional Verdict: %v", err)
		}
	})
}

func TestConditionV2RuntimeReaderRejectsLegacyAndExactSetDrift(t *testing.T) {
	t.Run("legacy digest-only attestation", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictConditionalV1, false)
		legacy := fixture.snapshot.Attestations[0]
		legacy.Conditions = nil
		legacy.Digest = ""
		digest, err := core.CanonicalJSONDigest("praxis.review", contract.ContractVersionV1, "AttestationV1", legacy)
		if err != nil {
			t.Fatal(err)
		}
		legacy.Digest = digest
		if err := legacy.Validate(); err != nil {
			t.Fatalf("historical legacy fact is not readable: %v", err)
		}
		if err := legacy.ValidateProductionConditionsV2(); !core.HasReason(err, core.ReasonReviewConditionUnsatisfied) {
			t.Fatalf("legacy fact did not fail production validation: %v", err)
		}
		fixture.snapshot.Attestations[0] = legacy
		fixture.snapshot = resealSnapshot(t, fixture.snapshot)
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonReviewConditionUnsatisfied) {
			t.Fatalf("Runtime reader accepted legacy digest-only conditional: %v", err)
		}
	})

	t.Run("attestation exact field drift", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictConditionalV1, false)
		attestation := fixture.snapshot.Attestations[0]
		attestation.Conditions[0].ConstraintDigest = digest("different-constraint")
		attestation.ConditionsDigest, _ = runtimeports.DigestReviewConditionsV2(attestation.Conditions)
		attestation.Digest = ""
		var err error
		attestation, err = contract.SealAttestationV1(attestation)
		if err != nil {
			t.Fatal(err)
		}
		fixture.snapshot.Attestations[0] = attestation
		fixture.snapshot = resealSnapshot(t, fixture.snapshot)
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("Runtime reader accepted exact condition drift: %v", err)
		}
	})

	t.Run("proof owner drift", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictConditionalV1, false)
		fixture.snapshot.Satisfaction.Proofs[0].Owner.ComponentID = "review.test/different-owner"
		fixture.snapshot.Satisfaction.ProofsDigest, _ = runtimeports.DigestConditionProofsV2(fixture.snapshot.Satisfaction.Proofs)
		fixture.snapshot = resealSnapshot(t, fixture.snapshot)
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonReviewConditionUnsatisfied) {
			t.Fatalf("Runtime reader accepted proof/condition owner drift: %v", err)
		}
	})
}

func TestFaultOperationReviewCurrentReaderV4LostReplyRecoveryIgnoresOriginalCancellation(t *testing.T) {
	fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKeyV4("key"), "value"))
	source := &cancelOnLostReplySourceV4{snapshot: fixture.snapshot, cancel: cancel}
	reader := mustReaderV4(t, source, func() time.Time { return fixture.now })

	projection, err := reader.InspectOperationReviewCurrentV4(ctx, fixture.intent)
	if err != nil {
		t.Fatal(err)
	}
	if projection.CurrentnessDigest != fixture.snapshot.Digest || source.callCount() != 2 || source.recoveryContextError() != nil || source.recoveryValue() != "value" || ctx.Err() != context.Canceled {
		t.Fatalf("lost reply recovery was not exact/detached: calls=%d recoveryErr=%v value=%v original=%v", source.callCount(), source.recoveryContextError(), source.recoveryValue(), ctx.Err())
	}
}

func TestFaultOperationReviewCurrentReaderV4PersistentUnknownFailsClosed(t *testing.T) {
	fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
	source := newAtomicSourceV4(t, fixture)
	source.alwaysUnknown = true
	reader := mustReaderV4(t, source, func() time.Time { return fixture.now })
	if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasCategory(err, core.ErrorUnavailable) || source.callCount() != 2 {
		t.Fatalf("persistent unknown did not fail after exact recovery Inspect: calls=%d err=%v", source.callCount(), err)
	}
}

func TestFaultOperationReviewCurrentReaderV4SnapshotDigestAndShortestTTL(t *testing.T) {
	t.Run("digest", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
		fixture.snapshot.DecisionEvidence[0].Ledger.Sequence++
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonInvalidDigest) {
			t.Fatalf("mutated snapshot digest was accepted: %v", err)
		}
	})

	t.Run("shortest_ttl", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictConditionalV1, false)
		proofExpiry := fixture.snapshot.Satisfaction.Proofs[0].ExpiresUnixNano
		if fixture.snapshot.ExpiresUnixNano != proofExpiry {
			t.Fatalf("fixture does not expose proof as shortest TTL: got=%d proof=%d", fixture.snapshot.ExpiresUnixNano, proofExpiry)
		}
		fixture.snapshot.ExpiresUnixNano++
		fixture.snapshot.Digest = ""
		var err error
		fixture.snapshot, err = runtimeadapter.SealCurrentFactSnapshotV4(fixture.snapshot)
		if err != nil {
			t.Fatal(err)
		}
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("TTL longer than shortest input was accepted: %v", err)
		}
	})

	t.Run("lease_boundary", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return time.Unix(0, fixture.snapshot.Assignments[0].LeaseExpiresUnixNano) })
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("Assignment lease boundary was accepted: %v", err)
		}
	})
}

func TestFaultOperationReviewCurrentReaderV4FreshClockBoundaries(t *testing.T) {
	t.Run("inspect_crosses_ttl", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
		clock := sequenceClockV4(fixture.now, time.Unix(0, fixture.snapshot.ExpiresUnixNano))
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), clock)
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("Inspect TTL crossing authorized: %v", err)
		}
	})
	t.Run("retry_crosses_ttl", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
		source := newAtomicSourceV4(t, fixture)
		source.loseReplies = 1
		clock := sequenceClockV4(fixture.now, fixture.now.Add(time.Nanosecond), time.Unix(0, fixture.snapshot.ExpiresUnixNano))
		reader := mustReaderV4(t, source, clock)
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonReviewVerdictStale) || source.callCount() != 2 {
			t.Fatalf("lost-reply retry TTL crossing authorized: calls=%d err=%v", source.callCount(), err)
		}
	})
	t.Run("inspect_clock_rollback", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
		reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), sequenceClockV4(fixture.now, fixture.now.Add(-time.Nanosecond)))
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("Inspect clock rollback did not fail closed: %v", err)
		}
	})
	t.Run("retry_clock_rollback", func(t *testing.T) {
		fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
		source := newAtomicSourceV4(t, fixture)
		source.loseReplies = 1
		reader := mustReaderV4(t, source, sequenceClockV4(fixture.now, fixture.now.Add(time.Nanosecond), fixture.now))
		if _, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent); !core.HasReason(err, core.ReasonClockRegression) || source.callCount() != 2 {
			t.Fatalf("retry clock rollback did not fail closed: calls=%d err=%v", source.callCount(), err)
		}
	})
}

func sequenceClockV4(values ...time.Time) func() time.Time {
	var mu sync.Mutex
	index := 0
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}

func TestIntegrationOperationReviewCurrentReaderV4PublicRuntimeContract(t *testing.T) {
	fixture := newFixtureV4(t, contract.VerdictConditionalV1, false)
	reader := mustReaderV4(t, newAtomicSourceV4(t, fixture), func() time.Time { return fixture.now })
	projection, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent)
	if err != nil {
		t.Fatal(err)
	}
	current := runtimeports.OperationGovernanceSnapshotV3{Operation: fixture.intent.Operation, Active: true, ProjectionWatermark: 1, Binding: projection.Binding, CurrentScope: projection.Scope, Review: runtimeports.OperationReviewAuthorizationV3{Case: projection.Case, CandidateDigest: projection.Target.Digest, CandidateRevision: projection.Target.Revision, Verdict: projection.Verdict, Satisfaction: &projection.Satisfaction.Fact, ReviewerAuthority: projection.ReviewerAuthority, PolicyDigest: projection.Policy.Digest, ExpiresUnixNano: projection.ExpiresUnixNano}, ExpiresUnixNano: projection.ExpiresUnixNano}
	if err := projection.ValidateAgainstIntent(fixture.intent, current, fixture.now); err != nil {
		t.Fatal(err)
	}
	// The adapter output remains the read-only projection type. Runtime alone
	// owns creation of OperationReviewAuthorizationFactV4 and Permit references.
	if projection.ContractVersion != runtimeports.OperationReviewAuthorizationContractVersionV4 {
		t.Fatalf("unexpected public contract version: %s", projection.ContractVersion)
	}
}

func TestIntegrationOperationReviewCurrentReaderV4ConcurrentAtomicSnapshots(t *testing.T) {
	fixture := newFixtureV4(t, contract.VerdictAcceptedV1, false)
	first := fixture.snapshot
	second := cloneTestSnapshot(first)
	second.DecisionEvidence[0].Ledger.Sequence = 2
	second = resealSnapshot(t, second)
	source := newAtomicSourceV4(t, fixture)
	reader := mustReaderV4(t, source, func() time.Time { return fixture.now })
	expected := map[core.Digest]uint64{first.Digest: first.DecisionEvidence[0].Ledger.Sequence, second.Digest: second.DecisionEvidence[0].Ledger.Sequence}

	var wg sync.WaitGroup
	errors := make(chan error, 64)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for index := 0; index < 500; index++ {
			if index%2 == 0 {
				source.replace(first)
			} else {
				source.replace(second)
			}
		}
	}()
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := 0; index < 50; index++ {
				projection, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent)
				if err != nil {
					errors <- err
					return
				}
				sequence, ok := expected[projection.CurrentnessDigest]
				if !ok || len(projection.DecisionEvidence) != 1 || projection.DecisionEvidence[0].Sequence != sequence {
					errors <- core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "mixed concurrent Review snapshot")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
}

func mustReaderV4(t *testing.T, source runtimeadapter.CurrentFactSourceV4, clock runtimeadapter.Clock) *runtimeadapter.ReaderV4 {
	t.Helper()
	reader, err := runtimeadapter.NewReaderV4(source, clock)
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

type contextKeyV4 string

type cancelOnLostReplySourceV4 struct {
	mu          sync.Mutex
	snapshot    runtimeadapter.CurrentFactSnapshotV4
	cancel      context.CancelFunc
	calls       int
	recoveryErr error
	value       any
}

func (s *cancelOnLostReplySourceV4) InspectReviewCurrentFactsV4(ctx context.Context, _ runtimeadapter.ExactCurrentRequestV4) (runtimeadapter.CurrentFactSnapshotV4, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.calls == 1 {
		s.cancel()
		return runtimeadapter.CurrentFactSnapshotV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost read reply")
	}
	s.recoveryErr = ctx.Err()
	s.value = ctx.Value(contextKeyV4("key"))
	return cloneTestSnapshot(s.snapshot), nil
}

func (s *cancelOnLostReplySourceV4) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *cancelOnLostReplySourceV4) recoveryContextError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recoveryErr
}

func (s *cancelOnLostReplySourceV4) recoveryValue() any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value
}

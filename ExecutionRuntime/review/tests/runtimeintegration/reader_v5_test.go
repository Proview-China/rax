package runtimeintegration_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestUnitOperationReviewCurrentReaderV5NominalBranches(t *testing.T) {
	tests := []struct {
		name    string
		fixture func(*testing.T) fixtureV5
		basis   runtimeports.OperationReviewAuthorizationBasisV5
	}{
		{"accepted_quorum", newQuorumFixtureV5, runtimeports.OperationReviewBasisAcceptedQuorumV5},
		{"conditional_quorum", newConditionalQuorumFixtureV5, runtimeports.OperationReviewBasisConditionalQuorumSatisfiedV5},
		{"operation_not_required", newBypassFixtureV5, runtimeports.OperationReviewBasisPolicyNotRequiredV5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.fixture(t)
			source := &atomicSourceV5{snapshot: f.snapshot}
			reader, err := runtimeadapter.NewReaderV5(source, func() time.Time { return f.now.Add(6 * time.Second) })
			if err != nil {
				t.Fatal(err)
			}
			p, err := reader.InspectOperationReviewCurrentV5(context.Background(), f.request)
			if err != nil {
				t.Fatal(err)
			}
			if p.Basis != tc.basis || source.count() != 1 {
				t.Fatalf("unexpected projection: %+v calls=%d", p, source.count())
			}
			if tc.basis == runtimeports.OperationReviewBasisPolicyNotRequiredV5 && (p.PolicyNotRequired == nil || p.Quorum != nil) {
				t.Fatal("not-required branch was type-punned")
			}
			if (tc.basis == runtimeports.OperationReviewBasisAcceptedQuorumV5 || tc.basis == runtimeports.OperationReviewBasisConditionalQuorumSatisfiedV5) && (p.Quorum == nil || p.PolicyNotRequired != nil || p.Quorum.AcceptCount != 2) {
				t.Fatal("quorum branch is incomplete")
			}
			if tc.basis == runtimeports.OperationReviewBasisConditionalQuorumSatisfiedV5 && p.Quorum.Satisfaction == nil {
				t.Fatal("conditional quorum omitted Satisfaction")
			}
			var _ runtimeports.OperationReviewCurrentReaderV5 = reader
		})
	}
}

func TestBlackboxOperationReviewCurrentReaderV5FailClosedMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*runtimeadapter.CurrentFactSnapshotV5)
	}{
		{"case_resealed_revision", func(s *runtimeadapter.CurrentFactSnapshotV5) { s.Quorum.CurrentCase.Revision++ }},
		{"panel_resealed_revision", func(s *runtimeadapter.CurrentFactSnapshotV5) { s.Quorum.CurrentPanel.Revision++ }},
		{"veto", func(s *runtimeadapter.CurrentFactSnapshotV5) { s.Quorum.Quorum.Vetoed = true }},
		{"threshold", func(s *runtimeadapter.CurrentFactSnapshotV5) { s.Quorum.Quorum.AcceptCount = 1 }},
		{"assignment_target", func(s *runtimeadapter.CurrentFactSnapshotV5) { s.Quorum.Assignments[0].Target.Digest = digest("drift") }},
		{"attestation_binding", func(s *runtimeadapter.CurrentFactSnapshotV5) {
			s.Quorum.Attestations[0].ReviewerBinding.BindingSetID = "drift"
		}},
		{"organization", func(s *runtimeadapter.CurrentFactSnapshotV5) {
			s.Quorum.OrganizationCut.Items[0].ReviewerIdentity.Digest = digest("drift")
		}},
		{"authority", func(s *runtimeadapter.CurrentFactSnapshotV5) {
			s.Quorum.ReviewerAuthorities[0].SourceDigest = digest("drift")
		}},
		{"binding", func(s *runtimeadapter.CurrentFactSnapshotV5) { s.Quorum.Bindings[0].Current = false }},
		{"evidence", func(s *runtimeadapter.CurrentFactSnapshotV5) { s.Quorum.Evidence[0].Ledger.Sequence++ }},
		{"policy", func(s *runtimeadapter.CurrentFactSnapshotV5) { s.Quorum.Policy.SourceDigest = digest("drift") }},
		{"scope", func(s *runtimeadapter.CurrentFactSnapshotV5) { s.Quorum.Scope.SourceDigest = digest("drift") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newQuorumFixtureV5(t)
			tc.mutate(&f.snapshot)
			f.snapshot.Digest = ""
			f.snapshot, _ = runtimeadapter.SealCurrentFactSnapshotV5(f.snapshot)
			reader, _ := runtimeadapter.NewReaderV5(&atomicSourceV5{snapshot: f.snapshot}, func() time.Time { return f.now.Add(6 * time.Second) })
			if _, err := reader.InspectOperationReviewCurrentV5(context.Background(), f.request); err == nil {
				t.Fatalf("%s drift authorized", tc.name)
			}
		})
	}
}

func TestBlackboxOperationReviewCurrentReaderV5ConditionSatisfactionExactBinding(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*runtimeports.ConditionSatisfactionFactV2)
	}{
		{"subject", func(f *runtimeports.ConditionSatisfactionFactV2) { f.SubjectDigest = digest("other-subject") }},
		{"policy", func(f *runtimeports.ConditionSatisfactionFactV2) { f.Policy.Revision++ }},
		{"scope", func(f *runtimeports.ConditionSatisfactionFactV2) { f.Scope.AuthorityEpoch++ }},
		{"run", func(f *runtimeports.ConditionSatisfactionFactV2) { f.RunID = core.AgentRunID("other-run") }},
		{"action_scope", func(f *runtimeports.ConditionSatisfactionFactV2) { f.ActionScopeDigest = digest("other-action-scope") }},
		{"current_scope", func(f *runtimeports.ConditionSatisfactionFactV2) { f.CurrentScope.Revision++ }},
		{"missing_proof", func(f *runtimeports.ConditionSatisfactionFactV2) { f.Proofs = nil }},
		{"extra_proof", func(f *runtimeports.ConditionSatisfactionFactV2) { f.Proofs = append(f.Proofs, f.Proofs[0]) }},
		{"proof_condition", func(f *runtimeports.ConditionSatisfactionFactV2) {
			f.Proofs[0].ConditionID = "review.test/other-condition"
		}},
		{"proof_revision", func(f *runtimeports.ConditionSatisfactionFactV2) { f.Proofs[0].ConditionRevision++ }},
		{"proof_constraint", func(f *runtimeports.ConditionSatisfactionFactV2) {
			f.Proofs[0].ConstraintDigest = digest("other-constraint")
		}},
		{"proof_owner", func(f *runtimeports.ConditionSatisfactionFactV2) {
			f.Proofs[0].Owner.ComponentID = "review.test/other-owner"
		}},
		{"proof_scope", func(f *runtimeports.ConditionSatisfactionFactV2) {
			f.Proofs[0].ScopeDigest = digest("other-proof-scope")
		}},
		{"proof_authority", func(f *runtimeports.ConditionSatisfactionFactV2) { f.Proofs[0].Authority.Ref = "other-authority" }},
		{"proof_expiry", func(f *runtimeports.ConditionSatisfactionFactV2) { f.Proofs[0].ExpiresUnixNano++ }},
		{"proof_evidence", func(f *runtimeports.ConditionSatisfactionFactV2) {
			f.Proofs[0].Evidence.Ref = "other-condition-evidence"
			f.Proofs[0].Evidence.Digest = digest("other-condition-evidence")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newConditionalQuorumFixtureV5(t)
			tc.mutate(f.snapshot.Quorum.Satisfaction)
			f.snapshot.Quorum.Satisfaction.ProofsDigest, _ = runtimeports.DigestConditionProofsV2(f.snapshot.Quorum.Satisfaction.Proofs)
			f.snapshot.Digest = ""
			f.snapshot, _ = runtimeadapter.SealCurrentFactSnapshotV5(f.snapshot)
			reader, _ := runtimeadapter.NewReaderV5(&atomicSourceV5{snapshot: f.snapshot}, func() time.Time { return f.now.Add(6 * time.Second) })
			projection, err := reader.InspectOperationReviewCurrentV5(context.Background(), f.request)
			if err == nil || projection.Quorum != nil {
				t.Fatalf("%s drift authorized: projection=%+v err=%v", tc.name, projection, err)
			}
		})
	}
}

func TestBlackboxOperationReviewCurrentReaderV5BypassDriftAndNoSyntheticVerdict(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*runtimeadapter.CurrentFactSnapshotV5)
	}{{"case_state", func(s *runtimeadapter.CurrentFactSnapshotV5) {
		s.PolicyNotRequired.CurrentCase.State = contract.CaseResolvedV1
	}}, {"policy", func(s *runtimeadapter.CurrentFactSnapshotV5) { s.PolicyNotRequired.Policy.Current = false }}, {"authority", func(s *runtimeadapter.CurrentFactSnapshotV5) {
		s.PolicyNotRequired.Authority.SourceDigest = digest("drift")
	}}, {"scope", func(s *runtimeadapter.CurrentFactSnapshotV5) {
		s.PolicyNotRequired.Scope.SourceDigest = digest("drift")
	}}, {"binding", func(s *runtimeadapter.CurrentFactSnapshotV5) { s.PolicyNotRequired.Binding.SourceRevision++ }}, {"revoked", func(s *runtimeadapter.CurrentFactSnapshotV5) {
		s.PolicyNotRequired.Decision.State = contract.BypassDecisionRevokedV1
	}}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newBypassFixtureV5(t)
			tc.mutate(&f.snapshot)
			f.snapshot.Digest = ""
			f.snapshot, _ = runtimeadapter.SealCurrentFactSnapshotV5(f.snapshot)
			reader, _ := runtimeadapter.NewReaderV5(&atomicSourceV5{snapshot: f.snapshot}, func() time.Time { return f.now.Add(time.Second) })
			if p, err := reader.InspectOperationReviewCurrentV5(context.Background(), f.request); err == nil || p.Quorum != nil {
				t.Fatalf("bypass drift authorized or synthetic Verdict emitted: p=%+v err=%v", p, err)
			}
		})
	}
}

func TestWhiteboxOperationReviewCurrentReaderV5ShortestCutAndTypedNil(t *testing.T) {
	var typedNil *atomicSourceV5
	if _, err := runtimeadapter.NewReaderV5(typedNil, time.Now); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil source accepted: %v", err)
	}
	f := newQuorumFixtureV5(t)
	f.snapshot.Quorum.OrganizationCut.ExpiresUnixNano--
	f.snapshot.ExpiresUnixNano = f.snapshot.Quorum.OrganizationCut.ExpiresUnixNano
	f.snapshot.Digest = ""
	f.snapshot, _ = runtimeadapter.SealCurrentFactSnapshotV5(f.snapshot)
	reader, _ := runtimeadapter.NewReaderV5(&atomicSourceV5{snapshot: f.snapshot}, func() time.Time { return f.now.Add(6 * time.Second) })
	if _, err := reader.InspectOperationReviewCurrentV5(context.Background(), f.request); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("hidden shorter Organization TTL was not fail closed: %v", err)
	}
}

func TestFaultOperationReviewCurrentReaderV5LostReplyDetachedExactAndClock(t *testing.T) {
	t.Run("lost_reply", func(t *testing.T) {
		f := newQuorumFixtureV5(t)
		ctx, cancel := context.WithCancel(context.Background())
		source := &atomicSourceV5{snapshot: f.snapshot, lose: 1, cancel: cancel}
		reader, _ := runtimeadapter.NewReaderV5(source, sequenceClockV4(f.now.Add(6*time.Second), f.now.Add(6*time.Second+time.Nanosecond), f.now.Add(6*time.Second+2*time.Nanosecond)))
		if _, err := reader.InspectOperationReviewCurrentV5(ctx, f.request); err != nil {
			t.Fatal(err)
		}
		if source.count() != 2 || source.recoveryErr != nil || ctx.Err() != context.Canceled {
			t.Fatalf("recovery not detached/exact: calls=%d recovery=%v original=%v", source.count(), source.recoveryErr, ctx.Err())
		}
		requests := source.exactRequests()
		if len(requests) != 2 || !reflect.DeepEqual(requests[0], requests[1]) {
			t.Fatalf("lost reply changed exact Inspect request: %+v", requests)
		}
	})
	t.Run("persistent_unknown", func(t *testing.T) {
		f := newQuorumFixtureV5(t)
		source := &atomicSourceV5{snapshot: f.snapshot, always: true}
		reader, _ := runtimeadapter.NewReaderV5(source, func() time.Time { return f.now })
		if _, err := reader.InspectOperationReviewCurrentV5(context.Background(), f.request); !core.HasCategory(err, core.ErrorIndeterminate) || source.count() != 2 {
			t.Fatalf("persistent unknown: calls=%d err=%v", source.count(), err)
		}
	})
	t.Run("rollback", func(t *testing.T) {
		f := newQuorumFixtureV5(t)
		reader, _ := runtimeadapter.NewReaderV5(&atomicSourceV5{snapshot: f.snapshot}, sequenceClockV4(f.now, f.now.Add(-time.Nanosecond)))
		if _, err := reader.InspectOperationReviewCurrentV5(context.Background(), f.request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("rollback accepted: %v", err)
		}
	})
	t.Run("retry_ttl", func(t *testing.T) {
		f := newQuorumFixtureV5(t)
		source := &atomicSourceV5{snapshot: f.snapshot, lose: 1}
		reader, _ := runtimeadapter.NewReaderV5(source, sequenceClockV4(f.now, f.now.Add(time.Nanosecond), time.Unix(0, f.snapshot.ExpiresUnixNano)))
		if _, err := reader.InspectOperationReviewCurrentV5(context.Background(), f.request); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("retry TTL crossing accepted: %v", err)
		}
	})
	t.Run("inspect_ttl", func(t *testing.T) {
		f := newQuorumFixtureV5(t)
		reader, _ := runtimeadapter.NewReaderV5(&atomicSourceV5{snapshot: f.snapshot}, sequenceClockV4(f.now, time.Unix(0, f.snapshot.ExpiresUnixNano)))
		if _, err := reader.InspectOperationReviewCurrentV5(context.Background(), f.request); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("Inspect TTL crossing accepted: %v", err)
		}
	})
}

func TestIntegrationOperationReviewCurrentReaderV5ConcurrentConsistentDeepClones(t *testing.T) {
	f := newQuorumFixtureV5(t)
	source := &atomicSourceV5{snapshot: f.snapshot}
	reader, _ := runtimeadapter.NewReaderV5(source, func() time.Time { return f.now.Add(6 * time.Second) })
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				p, err := reader.InspectOperationReviewCurrentV5(context.Background(), f.request)
				if err != nil {
					errs <- err
					return
				}
				if p.Quorum == nil || p.Quorum.ReviewerSetDigest != f.snapshot.Quorum.Verdict.ReviewerSetDigest || len(p.Quorum.DecisionEvidence) != 2 {
					errs <- core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "torn quorum snapshot")
					return
				}
				p.Quorum.DecisionEvidence[0].Sequence = 999
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if f.snapshot.Quorum.Evidence[0].Ledger.Sequence == 999 {
		t.Fatal("projection mutation aliased source snapshot")
	}
}

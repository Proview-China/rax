package fakes_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationScopeEvidenceV3IssueDoesNotAdvanceCursorAndAtomicConsume(t *testing.T) {
	f := newOperationScopeEvidenceFixtureV3(t)
	qualification := f.issue(t)
	source, err := f.store.InspectOperationScopeEvidenceSourceV3(context.Background(), f.source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if source.Revision != 1 || source.NextSequence != 1 {
		t.Fatalf("Issue advanced source cursor: %#v", source)
	}
	handoff := f.handoff(t, qualification)
	result := f.consume(t, qualification, handoff, "consume-1")
	if result.Source.Revision != 2 || result.Source.NextSequence != 2 || result.Record.Ref.Sequence != 1 || result.Qualification.State != ports.OperationScopeEvidenceConsumedCurrentV3 {
		t.Fatalf("atomic consume did not close all facts: %#v", result)
	}
	if result.Consumption.Record != result.Record.Ref || result.Qualification.Consumption == nil || *result.Qualification.Consumption != result.Consumption.RefV3() {
		t.Fatal("consume association is not exact")
	}
}

func TestOperationScopeEvidenceV3LostRepliesRecoverByExactInspect(t *testing.T) {
	f := newOperationScopeEvidenceFixtureV3(t)
	f.store.LoseNextQualificationReply()
	qualification := f.issue(t)
	f.store.LoseNextHandoffReply()
	handoff := f.handoff(t, qualification)
	f.store.LoseNextConsumeReply()
	result := f.consume(t, qualification, handoff, "consume-lost")
	if result.Record.Ref.Sequence != 1 {
		t.Fatalf("lost reply duplicated record: %#v", result.Record.Ref)
	}
	replayed := f.consume(t, qualification, handoff, "consume-lost")
	if replayed.Consumption.Digest != result.Consumption.Digest || replayed.Record.Ref != result.Record.Ref {
		t.Fatal("lost reply recovery changed canonical facts")
	}
}

func TestOperationScopeEvidenceV3ConsumeBindsIndependentExactHandoff(t *testing.T) {
	f := newOperationScopeEvidenceFixtureV3(t)
	qualification := f.issue(t)
	first := f.handoff(t, qualification)
	second, err := f.gateway.HandoffOperationScopeEvidenceV3(context.Background(), ports.HandoffOperationScopeEvidenceRequestV3{HandoffID: "handoff-2", Qualification: qualification.RefV3()})
	if err != nil {
		t.Fatal(err)
	}
	candidate := f.candidate(qualification, first)
	stored, err := f.gateway.ConsumeOperationScopeEvidenceV3(context.Background(), ports.ConsumeOperationScopeEvidenceRequestV3{ConsumptionID: "consume-handoff-exact", Handoff: first.RefV3(), Candidate: candidate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.gateway.ConsumeOperationScopeEvidenceV3(context.Background(), ports.ConsumeOperationScopeEvidenceRequestV3{ConsumptionID: "consume-handoff-exact", Handoff: second.RefV3(), Candidate: candidate}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same consumption id accepted another handoff: %v", err)
	}
	inspected, err := f.store.InspectOperationScopeEvidenceConsumptionV3(context.Background(), stored.Consumption.ID)
	if err != nil || inspected.Handoff != first.RefV3() || inspected.Digest != stored.Consumption.Digest {
		t.Fatalf("handoff conflict changed consumption: %v %#v", err, inspected)
	}
}

func TestOperationScopeEvidenceV3PolicyCASLostReplyAndConcurrentContent(t *testing.T) {
	t.Run("lost_reply", func(t *testing.T) {
		f := newOperationScopeEvidenceFixtureV3(t)
		next := sealEvidencePolicyStateV3(t, f.policy, ports.OperationScopeEvidencePolicyRevokedV3)
		request := ports.OperationScopeEvidencePolicyCASRequestV3{ExpectedRevision: f.policy.Revision, Next: next}
		f.store.LoseNextPolicyCASReply()
		if _, err := f.store.CompareAndSwapOperationScopeEvidencePolicyV3(context.Background(), request); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("lost policy CAS reply was not unknown: %v", err)
		}
		inspected, err := f.store.InspectOperationScopeEvidencePolicyV3(context.Background(), next.ID)
		if err != nil || inspected.Digest != next.Digest {
			t.Fatalf("lost policy CAS reply was not inspectable: %v %#v", err, inspected)
		}
		replayed, err := f.store.CompareAndSwapOperationScopeEvidencePolicyV3(context.Background(), request)
		if err != nil || replayed.Digest != next.Digest {
			t.Fatalf("policy CAS replay was not idempotent: %v %#v", err, replayed)
		}
		forged := next
		forged.MaximumPayloadBytes++
		if _, err := f.store.CompareAndSwapOperationScopeEvidencePolicyV3(context.Background(), ports.OperationScopeEvidencePolicyCASRequestV3{ExpectedRevision: f.policy.Revision, Next: forged}); !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("stale policy revision accepted changed content under copied digest: %v", err)
		}
	})
	t.Run("conflicting_content", func(t *testing.T) {
		f := newOperationScopeEvidenceFixtureV3(t)
		revoked := sealEvidencePolicyStateV3(t, f.policy, ports.OperationScopeEvidencePolicyRevokedV3)
		expired := sealEvidencePolicyStateV3(t, f.policy, ports.OperationScopeEvidencePolicyExpiredV3)
		var wg sync.WaitGroup
		results := make(chan core.Digest, 64)
		errors := make(chan error, 64)
		for index := range 64 {
			next := revoked
			if index%2 == 1 {
				next = expired
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				fact, err := f.store.CompareAndSwapOperationScopeEvidencePolicyV3(context.Background(), ports.OperationScopeEvidencePolicyCASRequestV3{ExpectedRevision: f.policy.Revision, Next: next})
				if err != nil {
					errors <- err
					return
				}
				results <- fact.Digest
			}()
		}
		wg.Wait()
		close(results)
		close(errors)
		assertOnePolicyContentV3(t, results, errors, 32)
	})
}

func TestOperationScopeEvidenceV3ApplicabilityPolicyCASLostReplyAndConcurrentContent(t *testing.T) {
	t.Run("lost_reply", func(t *testing.T) {
		f := newOperationScopeEvidenceFixtureV3(t)
		next := sealApplicabilityPolicyStateV3(t, f.app, ports.OperationScopeEvidencePolicyRevokedV3)
		request := ports.OperationScopeEvidenceApplicabilityPolicyCASRequestV3{ExpectedRevision: f.app.Revision, Next: next}
		f.store.LoseNextApplicabilityPolicyCASReply()
		if _, err := f.store.CompareAndSwapOperationScopeEvidenceApplicabilityPolicyV3(context.Background(), request); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("lost applicability CAS reply was not unknown: %v", err)
		}
		inspected, err := f.store.InspectOperationScopeEvidenceApplicabilityPolicyV3(context.Background(), next.ID)
		if err != nil || inspected.Digest != next.Digest {
			t.Fatalf("lost applicability CAS reply was not inspectable: %v %#v", err, inspected)
		}
		replayed, err := f.store.CompareAndSwapOperationScopeEvidenceApplicabilityPolicyV3(context.Background(), request)
		if err != nil || replayed.Digest != next.Digest {
			t.Fatalf("applicability CAS replay was not idempotent: %v %#v", err, replayed)
		}
		forged := next
		forged.ExecutionScopeDigest = digestV3("forged-scope")
		if _, err := f.store.CompareAndSwapOperationScopeEvidenceApplicabilityPolicyV3(context.Background(), ports.OperationScopeEvidenceApplicabilityPolicyCASRequestV3{ExpectedRevision: f.app.Revision, Next: forged}); !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("stale applicability revision accepted changed content under copied digest: %v", err)
		}
	})
	t.Run("conflicting_content", func(t *testing.T) {
		f := newOperationScopeEvidenceFixtureV3(t)
		revoked := sealApplicabilityPolicyStateV3(t, f.app, ports.OperationScopeEvidencePolicyRevokedV3)
		expired := sealApplicabilityPolicyStateV3(t, f.app, ports.OperationScopeEvidencePolicyExpiredV3)
		var wg sync.WaitGroup
		results := make(chan core.Digest, 64)
		errors := make(chan error, 64)
		for index := range 64 {
			next := revoked
			if index%2 == 1 {
				next = expired
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				fact, err := f.store.CompareAndSwapOperationScopeEvidenceApplicabilityPolicyV3(context.Background(), ports.OperationScopeEvidenceApplicabilityPolicyCASRequestV3{ExpectedRevision: f.app.Revision, Next: next})
				if err != nil {
					errors <- err
					return
				}
				results <- fact.Digest
			}()
		}
		wg.Wait()
		close(results)
		close(errors)
		assertOnePolicyContentV3(t, results, errors, 32)
	})
}

func TestOperationScopeEvidenceV3ConcurrentConsumeLinearizesOnce(t *testing.T) {
	f := newOperationScopeEvidenceFixtureV3(t)
	qualification := f.issue(t)
	handoff := f.handoff(t, qualification)
	const workers = 64
	results := make(chan ports.OperationScopeEvidenceConsumeResultV3, workers)
	errors := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := f.gateway.ConsumeOperationScopeEvidenceV3(context.Background(), ports.ConsumeOperationScopeEvidenceRequestV3{ConsumptionID: "consume-concurrent", Handoff: handoff.RefV3(), Candidate: f.candidate(qualification, handoff)})
			if err != nil {
				errors <- err
				return
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	var expected ports.OperationScopeEvidenceRecordRefV3
	count := 0
	for result := range results {
		count++
		if count == 1 {
			expected = result.Record.Ref
		} else if result.Record.Ref != expected {
			t.Fatalf("concurrent consume produced another record: %#v != %#v", result.Record.Ref, expected)
		}
	}
	if count != workers {
		t.Fatalf("got %d results", count)
	}
	source, _ := f.store.InspectOperationScopeEvidenceSourceV3(context.Background(), f.source.ID)
	if source.NextSequence != 2 {
		t.Fatalf("cursor advanced %d times", source.NextSequence-1)
	}
}

func TestOperationScopeEvidenceV3DriftAndPhaseSwapFailBeforeWrites(t *testing.T) {
	f := newOperationScopeEvidenceFixtureV3(t)
	f.runtime.mu.Lock()
	f.runtime.value.PermitDigest = core.DigestBytes([]byte("drift"))
	f.runtime.value, _ = ports.SealOperationScopeEvidenceRuntimeCurrentProjectionV3(f.runtime.value, f.now)
	f.runtime.mu.Unlock()
	if _, err := f.gateway.IssueOperationScopeEvidenceV3(context.Background(), f.issueRequest()); err == nil {
		t.Fatal("runtime drift authorized qualification")
	}
	if _, err := f.store.InspectOperationScopeEvidenceQualificationV3(context.Background(), "qualification-1"); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("drift wrote qualification: %v", err)
	}
	f = newOperationScopeEvidenceFixtureV3(t)
	request := f.issueRequest()
	request.Scope.Phase = ports.OperationDispatchEnforcementExecuteV4
	if _, err := f.gateway.IssueOperationScopeEvidenceV3(context.Background(), request); err == nil {
		t.Fatal("phase swap authorized qualification")
	}
}

func TestOperationScopeEvidenceV3IssueDriftMatrixHasZeroWrites(t *testing.T) {
	tests := map[string]func(*operationScopeEvidenceFixtureV3, *ports.IssueOperationScopeEvidenceRequestV3){
		"attempt": func(_ *operationScopeEvidenceFixtureV3, r *ports.IssueOperationScopeEvidenceRequestV3) {
			r.Scope.AttemptID = "sandbox-attempt-drift"
		},
		"permit": func(_ *operationScopeEvidenceFixtureV3, r *ports.IssueOperationScopeEvidenceRequestV3) {
			r.PermitDigest = digestV3("permit-drift")
		},
		"authorization": func(_ *operationScopeEvidenceFixtureV3, r *ports.IssueOperationScopeEvidenceRequestV3) {
			r.Authorization.Digest = digestV3("authorization-drift")
		},
		"generation": func(_ *operationScopeEvidenceFixtureV3, r *ports.IssueOperationScopeEvidenceRequestV3) {
			r.Scope.Generation.Digest = digestV3("generation-drift")
		},
		"evidence_policy": func(_ *operationScopeEvidenceFixtureV3, r *ports.IssueOperationScopeEvidenceRequestV3) {
			ref := ports.OperationScopeEvidenceFactRefV3(r.EvidencePolicy)
			ref.Digest = digestV3("policy-drift")
			r.EvidencePolicy = ports.OperationScopeEvidencePolicyRefV3(ref)
		},
		"source_registration": func(_ *operationScopeEvidenceFixtureV3, r *ports.IssueOperationScopeEvidenceRequestV3) {
			r.Reservation.Registration.Digest = digestV3("source-drift")
		},
		"custom_effect_kind": func(_ *operationScopeEvidenceFixtureV3, r *ports.IssueOperationScopeEvidenceRequestV3) {
			r.Scope.EffectKind = "custom/activation"
		},
		"custom_policy_profile": func(_ *operationScopeEvidenceFixtureV3, r *ports.IssueOperationScopeEvidenceRequestV3) {
			r.Scope.ApplicabilityPolicy = ports.OperationScopeEvidenceApplicabilityPolicyRefV3{ID: "custom-profile-policy", Revision: 1, Digest: digestV3("custom-profile"), ExpiresUnixNano: r.Scope.ApplicabilityPolicy.ExpiresUnixNano}
		},
		"ttl": func(_ *operationScopeEvidenceFixtureV3, r *ports.IssueOperationScopeEvidenceRequestV3) {
			r.RequestedTTL = ports.MaxDispatchPermitTTL + time.Nanosecond
		},
		"run_scope_forbidden": func(_ *operationScopeEvidenceFixtureV3, r *ports.IssueOperationScopeEvidenceRequestV3) {
			r.Scope.Operation.Kind = ports.OperationScopeRunV3
			r.Scope.Operation.ActivationAttemptID = ""
			r.Scope.Operation.RunID = "run-forbidden"
			r.Scope.OperationDigest, _ = r.Scope.Operation.DigestV3()
			r.Scope.LedgerScope.OperationDigest = r.Scope.OperationDigest
		},
		"termination_attempt_forbidden": func(_ *operationScopeEvidenceFixtureV3, r *ports.IssueOperationScopeEvidenceRequestV3) {
			r.Scope.Operation.Kind = ports.OperationScopeTerminationV3
			r.Scope.Operation.ActivationAttemptID = ""
			r.Scope.Operation.TerminationAttemptID = "termination-attempt-forbidden"
			r.Scope.OperationDigest, _ = r.Scope.Operation.DigestV3()
			r.Scope.LedgerScope.OperationDigest = r.Scope.OperationDigest
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			f := newOperationScopeEvidenceFixtureV3(t)
			request := f.issueRequest()
			mutate(f, &request)
			if _, err := f.gateway.IssueOperationScopeEvidenceV3(context.Background(), request); err == nil {
				t.Fatal("drift authorized qualification")
			}
			if _, err := f.store.InspectOperationScopeEvidenceQualificationV3(context.Background(), request.QualificationID); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("drift wrote qualification: %v", err)
			}
			source, _ := f.store.InspectOperationScopeEvidenceSourceV3(context.Background(), f.source.ID)
			if source.Revision != 1 || source.NextSequence != 1 {
				t.Fatalf("drift advanced source: %#v", source)
			}
		})
	}
}

func TestOperationScopeEvidenceV3ClosedMatrixZeroWritesAndSafeKindsIssue(t *testing.T) {
	for _, kind := range []ports.EffectKindV2{"praxis.sandbox/allocate", "praxis.sandbox/activate", "praxis.sandbox/open", "praxis.sandbox/inspect"} {
		t.Run("safe_"+string(kind), func(t *testing.T) {
			f := newOperationScopeEvidenceFixtureForKindV3(t, kind)
			if _, err := f.gateway.IssueOperationScopeEvidenceV3(context.Background(), f.issueRequest()); err != nil {
				t.Fatal(err)
			}
		})
	}
	unsupported := []ports.EffectKindV2{"praxis.sandbox/backend-discovery", "discover", "praxis.sandbox/cancel", "rollback", "praxis.sandbox/close", "praxis.sandbox/release", "custom/activation"}
	for _, kind := range unsupported {
		t.Run("reject_"+string(kind), func(t *testing.T) {
			f := newOperationScopeEvidenceFixtureV3(t)
			request := f.issueRequest()
			request.Scope.EffectKind = kind
			if _, err := f.gateway.IssueOperationScopeEvidenceV3(context.Background(), request); err == nil {
				t.Fatal("unsupported EffectKind wrote qualification")
			}
			if _, err := f.store.InspectOperationScopeEvidenceQualificationV3(context.Background(), request.QualificationID); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("unsupported EffectKind changed state: %v", err)
			}
			if _, err := f.store.InspectOperationScopeEvidenceProviderHandoffV3(context.Background(), "handoff-forbidden"); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("unsupported EffectKind reached Provider handoff: %v", err)
			}
		})
	}
	for _, kind := range []ports.EffectKindV2{"praxis.sandbox/allocate", "praxis.sandbox/activate", "praxis.sandbox/open"} {
		t.Run("reject_recovery_profile_"+string(kind), func(t *testing.T) {
			f := newOperationScopeEvidenceFixtureV3(t)
			forged := f.app
			forged.ID = "recovery-profile-" + string(kind)[len("praxis.sandbox/"):]
			forged.EffectKind = kind
			forged.Profile = ports.OperationScopeEvidenceRecoveryProfileV3
			forged.Digest = ""
			forged.Digest, _ = forged.DigestV3()
			if _, err := f.store.CreateOperationScopeEvidenceApplicabilityPolicyV3(context.Background(), forged); err == nil {
				t.Fatal("recovery profile was registered")
			}
			if _, err := f.store.InspectOperationScopeEvidenceApplicabilityPolicyV3(context.Background(), forged.ID); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("recovery profile changed policy state: %v", err)
			}
		})
	}
}

func TestOperationScopeEvidenceV3ConcurrentIssueSameContentAndChangedContent(t *testing.T) {
	f := newOperationScopeEvidenceFixtureV3(t)
	request := f.issueRequest()
	const workers = 64
	var wg sync.WaitGroup
	results := make(chan ports.OperationScopeEvidenceQualificationFactV3, workers)
	errors := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q, err := f.gateway.IssueOperationScopeEvidenceV3(context.Background(), request)
			if err != nil {
				errors <- err
				return
			}
			results <- q
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	var digest core.Digest
	count := 0
	for q := range results {
		count++
		if digest == "" {
			digest = q.Digest
		} else if q.Digest != digest {
			t.Fatal("same Issue produced different facts")
		}
	}
	if count != workers {
		t.Fatalf("got %d qualifications", count)
	}
	changed := request
	changed.RequestedTTL++
	if _, err := f.gateway.IssueOperationScopeEvidenceV3(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same id changed content did not conflict: %v", err)
	}
	source, _ := f.store.InspectOperationScopeEvidenceSourceV3(context.Background(), f.source.ID)
	if source.Revision != 1 || source.NextSequence != 1 {
		t.Fatal("concurrent Issue advanced cursor")
	}
}

func TestOperationScopeEvidenceV3IssueSameIDRequiresExactCanonicalScope(t *testing.T) {
	tests := map[string]func(*ports.IssueOperationScopeEvidenceRequestV3){
		"ledger_scope": func(request *ports.IssueOperationScopeEvidenceRequestV3) {
			request.Scope.LedgerScope.ChainID = "another-activation-chain"
		},
		"effect_kind": func(request *ports.IssueOperationScopeEvidenceRequestV3) {
			request.Scope.EffectKind = "custom/another-effect"
		},
		"applicability_policy": func(request *ports.IssueOperationScopeEvidenceRequestV3) {
			request.Scope.ApplicabilityPolicy.ID = "another-applicability-policy"
		},
		"applicability": func(request *ports.IssueOperationScopeEvidenceRequestV3) {
			fact := ports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: "custom/run-fact", ID: "run-fact-1", Revision: 1, Digest: digestV3("run-fact")}
			for index := range request.Scope.Applicability {
				if request.Scope.Applicability[index].Dimension == ports.OperationScopeEvidenceRunV3 {
					request.Scope.Applicability[index] = ports.OperationScopeEvidenceApplicabilityV3{Dimension: ports.OperationScopeEvidenceRunV3, Mode: ports.OperationScopeEvidenceRequiredV3, Fact: &fact}
				}
			}
		},
		"generation": func(request *ports.IssueOperationScopeEvidenceRequestV3) {
			request.Scope.Generation.ID = "another-generation-association"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			f := newOperationScopeEvidenceFixtureV3(t)
			original := f.issue(t)
			readsBefore := f.runtime.inspectCount()
			changed := f.issueRequest()
			mutate(&changed)
			if _, err := f.gateway.IssueOperationScopeEvidenceV3(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("same Qualification ID with changed Scope did not conflict: %v", err)
			}
			if f.runtime.inspectCount() != readsBefore {
				t.Fatal("historical idempotency conflict reread current Runtime facts")
			}
			stored, err := f.store.InspectOperationScopeEvidenceQualificationV3(context.Background(), original.ID)
			if err != nil || stored.Digest != original.Digest {
				t.Fatalf("Scope conflict changed stored qualification: %v %#v", err, stored)
			}
			source, _ := f.store.InspectOperationScopeEvidenceSourceV3(context.Background(), f.source.ID)
			if source.Revision != 1 || source.NextSequence != 1 {
				t.Fatalf("Scope conflict advanced source: %#v", source)
			}
		})
	}
}

func TestOperationScopeEvidenceV3CrossTTLOnlyCreatesLateObservation(t *testing.T) {
	f := newOperationScopeEvidenceFixtureV3(t)
	f.requestedTTL = time.Second
	qualification := f.issue(t)
	handoff := f.handoff(t, qualification)
	f.now = f.now.Add(time.Second)
	result := f.consume(t, qualification, handoff, "consume-late")
	if !result.Record.LateObservation || result.Qualification.State != ports.OperationScopeEvidenceConsumedObservationV3 || result.Record.Candidate.TrustClass != ports.EvidenceTrustObservation {
		t.Fatalf("cross-TTL consume upgraded evidence: %#v", result)
	}
	f2 := newOperationScopeEvidenceFixtureV3(t)
	f2.requestedTTL = time.Second
	q2 := f2.issue(t)
	h2 := f2.handoff(t, q2)
	f2.now = f2.now.Add(2 * time.Second)
	if _, err := f2.gateway.ConsumeOperationScopeEvidenceV3(context.Background(), ports.ConsumeOperationScopeEvidenceRequestV3{ConsumptionID: "too-late", Handoff: h2.RefV3(), Candidate: f2.candidate(q2, h2)}); err == nil {
		t.Fatal("consume crossed bounded late TTL")
	}
}

func TestOperationScopeEvidenceV3HandoffAndConsumeRereadCurrentPolicy(t *testing.T) {
	t.Run("handoff", func(t *testing.T) {
		f := newOperationScopeEvidenceFixtureV3(t)
		q := f.issue(t)
		revokeEvidencePolicyV3(t, f)
		if _, err := f.gateway.HandoffOperationScopeEvidenceV3(context.Background(), ports.HandoffOperationScopeEvidenceRequestV3{HandoffID: "handoff-revoked", Qualification: q.RefV3()}); err == nil {
			t.Fatal("revoked policy permitted handoff")
		}
		if _, err := f.store.InspectOperationScopeEvidenceProviderHandoffV3(context.Background(), "handoff-revoked"); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("revoked handoff wrote fact: %v", err)
		}
	})
	t.Run("consume", func(t *testing.T) {
		f := newOperationScopeEvidenceFixtureV3(t)
		q := f.issue(t)
		h := f.handoff(t, q)
		revokeEvidencePolicyV3(t, f)
		if _, err := f.gateway.ConsumeOperationScopeEvidenceV3(context.Background(), ports.ConsumeOperationScopeEvidenceRequestV3{ConsumptionID: "consume-revoked", Handoff: h.RefV3(), Candidate: f.candidate(q, h)}); err == nil {
			t.Fatal("revoked policy permitted consume")
		}
		source, _ := f.store.InspectOperationScopeEvidenceSourceV3(context.Background(), f.source.ID)
		if source.Revision != 1 || source.NextSequence != 1 {
			t.Fatal("rejected consume advanced cursor")
		}
		if _, err := f.store.InspectOperationScopeEvidenceConsumptionV3(context.Background(), "consume-revoked"); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("rejected consume wrote association: %v", err)
		}
	})
}

func TestOperationScopeEvidenceV3PublicConformance(t *testing.T) {
	f := newOperationScopeEvidenceFixtureV3(t)
	report, err := conformance.CheckOperationScopeEvidenceV3(context.Background(), conformance.OperationScopeEvidenceConformanceCaseV3{
		Governance:              f.gateway,
		Facts:                   f.store,
		Issue:                   f.issueRequest(),
		HandoffID:               "handoff-conformance",
		ConsumptionID:           "consume-conformance",
		Candidate:               f.candidate,
		ForbiddenMutationCounts: func() (int, int, int) { return 0, 0, 0 },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.IssueDidNotAdvanceCursor || !report.HandoffIsProofOnly || !report.ConsumeAdvancedAtomically || !report.SameContentReplayIdempotent || !report.ChangedContentConflicts || !report.NoProviderDomainOrSettlementMutation || report.ProductionClaimEligible {
		t.Fatalf("unexpected conformance report: %#v", report)
	}
}

type operationScopeEvidenceRuntimeStubV3 struct {
	mu    sync.Mutex
	value ports.OperationScopeEvidenceRuntimeCurrentProjectionV3
	calls int
}

func (s *operationScopeEvidenceRuntimeStubV3) InspectOperationScopeEvidenceRuntimeCurrentV3(context.Context, ports.OperationScopeEvidenceScopeV3, string) (ports.OperationScopeEvidenceRuntimeCurrentProjectionV3, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.value, nil
}

func (s *operationScopeEvidenceRuntimeStubV3) inspectCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type operationScopeEvidenceGenerationStubV3 struct {
	value ports.OperationScopeEvidenceFactRefV3
}

func (s operationScopeEvidenceGenerationStubV3) InspectOperationScopeEvidenceGenerationCurrentV3(context.Context, ports.GenerationBindingAssociationRefV1) (ports.OperationScopeEvidenceFactRefV3, error) {
	return s.value, nil
}

type operationScopeEvidenceFixtureV3 struct {
	t            *testing.T
	now          time.Time
	store        *fakes.OperationScopeEvidenceStoreV3
	gateway      kernel.OperationScopeEvidenceGatewayV3
	runtime      *operationScopeEvidenceRuntimeStubV3
	scope        ports.OperationScopeEvidenceScopeV3
	policy       ports.OperationScopeEvidencePolicyFactV3
	app          ports.OperationScopeEvidenceApplicabilityPolicyFactV3
	source       ports.OperationScopeEvidenceSourceRegistrationFactV3
	phase        ports.OperationDispatchEnforcementPhaseRefV4
	auth         ports.OperationReviewAuthorizationRefV4
	schema       ports.SchemaRefV2
	requestedTTL time.Duration
}

func newOperationScopeEvidenceFixtureV3(t *testing.T) *operationScopeEvidenceFixtureV3 {
	return newOperationScopeEvidenceFixtureForKindV3(t, "praxis.sandbox/allocate")
}

func newOperationScopeEvidenceFixtureForKindV3(t *testing.T, effectKind ports.EffectKindV2) *operationScopeEvidenceFixtureV3 {
	t.Helper()
	f := &operationScopeEvidenceFixtureV3{t: t, now: time.Unix(900000, 0), requestedTTL: 2 * time.Second}
	f.store = fakes.NewOperationScopeEvidenceStoreV3(func() time.Time { return f.now })
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-evidence", ID: "identity-evidence", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-evidence", PlanDigest: digestV3("lineage")}, Instance: core.InstanceRef{ID: "instance-evidence", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
	operation := ports.OperationSubjectV3{Kind: ports.OperationScopeActivationV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, ActivationAttemptID: "activation-attempt-1", SubjectRevision: 1, CurrentProjectionRef: "activation-current-1", CurrentProjectionDigest: digestV3("activation-current"), CurrentProjectionRevision: 1}
	operationDigest, _ := operation.DigestV3()
	applicability := ports.NormalizeOperationScopeEvidenceApplicabilityV3([]ports.OperationScopeEvidenceApplicabilityV3{{Dimension: ports.OperationScopeEvidenceRunV3, Mode: ports.OperationScopeEvidenceForbiddenV3}, {Dimension: ports.OperationScopeEvidenceSessionV3, Mode: ports.OperationScopeEvidenceForbiddenV3}, {Dimension: ports.OperationScopeEvidenceTurnV3, Mode: ports.OperationScopeEvidenceForbiddenV3}, {Dimension: ports.OperationScopeEvidenceActionV3, Mode: ports.OperationScopeEvidenceForbiddenV3}, {Dimension: ports.OperationScopeEvidenceContextV3, Mode: ports.OperationScopeEvidenceForbiddenV3}})
	f.schema = ports.SchemaRefV2{Namespace: "custom.evidence", Name: "activation-observation", Version: "1.0.0", MediaType: "application/json", ContentDigest: digestV3("schema")}
	profile := ports.OperationScopeEvidenceActivationProfileV3
	if effectKind == "praxis.sandbox/inspect" {
		profile = ports.OperationScopeEvidenceActivationInspectionProfileV3
	}
	var err error
	f.app, err = ports.SealOperationScopeEvidenceApplicabilityPolicyFactV3(ports.OperationScopeEvidenceApplicabilityPolicyFactV3{ID: "applicability-policy-1", Revision: 1, State: ports.OperationScopeEvidencePolicyActiveV3, OperationKind: ports.OperationScopeActivationV3, EffectKind: effectKind, Profile: profile, ExecutionScopeDigest: scopeDigest, Applicability: applicability, ExpiresUnixNano: f.now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	f.policy, err = ports.SealOperationScopeEvidencePolicyFactV3(ports.OperationScopeEvidencePolicyFactV3{ID: "evidence-policy-1", Revision: 1, State: ports.OperationScopeEvidencePolicyActiveV3, OperationKind: ports.OperationScopeActivationV3, EffectKind: effectKind, AllowedPhases: []ports.OperationDispatchEnforcementPhaseV4{ports.OperationDispatchEnforcementPrepareV4}, ExpectedSchema: f.schema, MaximumPayloadBytes: 1024, MaximumQualificationTTL: 10 * time.Second, MaximumIngestGrace: time.Second, ExpiresUnixNano: f.now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	generation := ports.GenerationBindingAssociationRefV1{ID: "generation-association-1", Revision: 1, Digest: digestV3("generation")}
	f.scope = ports.OperationScopeEvidenceScopeV3{LedgerScope: ports.OperationScopeEvidenceLedgerScopeV3{TenantID: scope.Identity.TenantID, OperationDigest: operationDigest, ChainID: "activation-chain-1"}, Operation: operation, OperationDigest: operationDigest, EffectID: "operation-effect-1", EffectRevision: 1, EffectDigest: digestV3("effect"), EffectKind: effectKind, AttemptID: "sandbox-attempt-1", Phase: ports.OperationDispatchEnforcementPrepareV4, ApplicabilityPolicy: f.app.RefV3(), Applicability: applicability, Generation: generation}
	f.auth = ports.OperationReviewAuthorizationRefV4{ID: "review-authorization-1", Revision: 1, Digest: digestV3("authorization")}
	sandboxAttempt := ports.OperationDispatchSandboxFactRefV4{ID: f.scope.AttemptID, Revision: 1, Digest: digestV3("sandbox-attempt"), ExpiresUnixNano: f.now.Add(30 * time.Second).UnixNano()}
	f.phase = ports.OperationDispatchEnforcementPhaseRefV4{OperationDigest: operationDigest, EffectID: f.scope.EffectID, PermitID: "permit-1", PermitFactRevision: 2, PermitDigest: digestV3("permit"), AdmissionDigest: digestV3("admission"), ReviewAuthorization: f.auth, AttemptID: f.scope.AttemptID, SandboxAttempt: sandboxAttempt, Phase: ports.OperationDispatchEnforcementPrepareV4, ReceiptDigest: digestV3("receipt"), JournalRevision: 1, ValidatedUnixNano: f.now.Add(-time.Second).UnixNano(), ExpiresUnixNano: f.now.Add(20 * time.Second).UnixNano()}
	runtime, err := ports.SealOperationScopeEvidenceRuntimeCurrentProjectionV3(ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{Scope: f.scope, PermitID: "permit-1", PermitFactRevision: 2, PermitDigest: f.phase.PermitDigest, AdmissionDigest: f.phase.AdmissionDigest, Authorization: f.auth, Phase: f.phase, CheckedUnixNano: f.now.UnixNano(), ExpiresUnixNano: f.now.Add(20 * time.Second).UnixNano()}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	f.runtime = &operationScopeEvidenceRuntimeStubV3{value: runtime}
	producer := ports.EvidenceProducerBindingRefV2{BindingSetID: "binding-set-1", BindingSetRevision: 1, ComponentID: "custom/evidence-owner", ManifestDigest: digestV3("manifest"), ArtifactDigest: digestV3("artifact"), Capability: "custom/evidence-append"}
	f.source, err = ports.SealOperationScopeEvidenceSourceRegistrationFactV3(ports.OperationScopeEvidenceSourceRegistrationFactV3{ID: "source-registration-1", Revision: 1, SourceID: "custom/evidence-source", SourceEpoch: 1, NextSequence: 1, LedgerScope: f.scope.LedgerScope, Producer: producer, Policy: f.policy.RefV3(), State: ports.EvidenceSourceActive, CreatedUnixNano: f.now.Add(-time.Second).UnixNano(), UpdatedUnixNano: f.now.Add(-time.Second).UnixNano(), ExpiresUnixNano: f.now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.CreateOperationScopeEvidenceApplicabilityPolicyV3(context.Background(), f.app); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.CreateOperationScopeEvidencePolicyV3(context.Background(), f.policy); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.CreateOperationScopeEvidenceSourceV3(context.Background(), f.source); err != nil {
		t.Fatal(err)
	}
	f.gateway = kernel.OperationScopeEvidenceGatewayV3{Facts: f.store, Runtime: f.runtime, Generation: operationScopeEvidenceGenerationStubV3{value: ports.OperationScopeEvidenceFactRefV3{ID: generation.ID, Revision: generation.Revision, Digest: generation.Digest, ExpiresUnixNano: f.now.Add(20 * time.Second).UnixNano()}}, Clock: func() time.Time { return f.now }}
	return f
}
func (f *operationScopeEvidenceFixtureV3) issueRequest() ports.IssueOperationScopeEvidenceRequestV3 {
	return ports.IssueOperationScopeEvidenceRequestV3{QualificationID: "qualification-1", Scope: f.scope, PermitID: "permit-1", PermitFactRevision: 2, PermitDigest: f.phase.PermitDigest, AdmissionDigest: f.phase.AdmissionDigest, Authorization: f.auth, PhaseRef: f.phase, EvidencePolicy: f.policy.RefV3(), Reservation: ports.OperationScopeEvidenceSourceReservationV3{Registration: ports.OperationScopeEvidenceFactRefV3{ID: f.source.ID, Revision: f.source.Revision, Digest: f.source.Digest, ExpiresUnixNano: f.source.ExpiresUnixNano}, Source: ports.OperationScopeEvidenceSourceKeyV3{RegistrationID: f.source.ID, SourceEpoch: f.source.SourceEpoch, SourceSequence: f.source.NextSequence}, EventID: "event-1", Schema: f.schema}, RequestedTTL: f.requestedTTL}
}
func (f *operationScopeEvidenceFixtureV3) issue(t *testing.T) ports.OperationScopeEvidenceQualificationFactV3 {
	q, err := f.gateway.IssueOperationScopeEvidenceV3(context.Background(), f.issueRequest())
	if err != nil {
		t.Fatal(err)
	}
	return q
}
func (f *operationScopeEvidenceFixtureV3) handoff(t *testing.T, q ports.OperationScopeEvidenceQualificationFactV3) ports.OperationScopeEvidenceProviderHandoffFactV3 {
	h, err := f.gateway.HandoffOperationScopeEvidenceV3(context.Background(), ports.HandoffOperationScopeEvidenceRequestV3{HandoffID: "handoff-1", Qualification: q.RefV3()})
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func (f *operationScopeEvidenceFixtureV3) candidate(q ports.OperationScopeEvidenceQualificationFactV3, h ports.OperationScopeEvidenceProviderHandoffFactV3) ports.OperationScopeEvidenceCandidateV3 {
	return ports.OperationScopeEvidenceCandidateV3{ContractVersion: ports.OperationScopeEvidenceContractVersionV3, Qualification: q.RefV3(), Source: q.Reservation.Source, EventID: q.Reservation.EventID, TrustClass: ports.EvidenceTrustObservation, Payload: ports.EvidencePayloadRefV2{Schema: f.schema, ContentDigest: digestV3("payload"), Revision: 1, Length: 7, Ref: "evidence://payload/1"}, Causation: []ports.EvidenceCausationRefV2{}, CorrelationID: "correlation-1", ObservedUnixNano: f.now.UnixNano()}
}
func (f *operationScopeEvidenceFixtureV3) consume(t *testing.T, q ports.OperationScopeEvidenceQualificationFactV3, h ports.OperationScopeEvidenceProviderHandoffFactV3, id string) ports.OperationScopeEvidenceConsumeResultV3 {
	r, err := f.gateway.ConsumeOperationScopeEvidenceV3(context.Background(), ports.ConsumeOperationScopeEvidenceRequestV3{ConsumptionID: id, Handoff: h.RefV3(), Candidate: f.candidate(q, h)})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func revokeEvidencePolicyV3(t *testing.T, f *operationScopeEvidenceFixtureV3) {
	t.Helper()
	next := f.policy
	next.Revision++
	next.State = ports.OperationScopeEvidencePolicyRevokedV3
	var err error
	next, err = ports.SealOperationScopeEvidencePolicyFactV3(next)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.CompareAndSwapOperationScopeEvidencePolicyV3(context.Background(), ports.OperationScopeEvidencePolicyCASRequestV3{ExpectedRevision: f.policy.Revision, Next: next}); err != nil {
		t.Fatal(err)
	}
}

func sealEvidencePolicyStateV3(t *testing.T, current ports.OperationScopeEvidencePolicyFactV3, state ports.OperationScopeEvidencePolicyStateV3) ports.OperationScopeEvidencePolicyFactV3 {
	t.Helper()
	next := current
	next.Revision++
	next.State = state
	sealed, err := ports.SealOperationScopeEvidencePolicyFactV3(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func sealApplicabilityPolicyStateV3(t *testing.T, current ports.OperationScopeEvidenceApplicabilityPolicyFactV3, state ports.OperationScopeEvidencePolicyStateV3) ports.OperationScopeEvidenceApplicabilityPolicyFactV3 {
	t.Helper()
	next := current
	next.Revision++
	next.State = state
	sealed, err := ports.SealOperationScopeEvidenceApplicabilityPolicyFactV3(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func assertOnePolicyContentV3(t *testing.T, results <-chan core.Digest, errors <-chan error, expectedEach int) {
	t.Helper()
	var winning core.Digest
	successes := 0
	for digest := range results {
		successes++
		if winning == "" {
			winning = digest
		} else if winning != digest {
			t.Fatalf("two policy contents linearized: %s and %s", winning, digest)
		}
	}
	conflicts := 0
	for err := range errors {
		if !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("concurrent policy CAS returned non-conflict: %v", err)
		}
		conflicts++
	}
	if successes != expectedEach || conflicts != expectedEach {
		t.Fatalf("concurrent policy CAS got successes=%d conflicts=%d", successes, conflicts)
	}
}

func digestV3(value string) core.Digest { return core.DigestBytes([]byte(value)) }

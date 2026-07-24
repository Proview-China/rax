package fakes_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationDispatchEnforcementV4PrepareExecuteLostReplyAndCurrentInspect(t *testing.T) {
	fixture := newOperationEnforcementFixtureV4(t, "enforcement-flow")
	fixture.effect.store.LoseNextEnforcementV4Reply()
	preparedCurrent, err := fixture.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), fixture.prepare)
	if err != nil {
		t.Fatal(err)
	}
	if preparedCurrent.Phase.Phase != ports.OperationDispatchEnforcementPrepareV4 || fixture.effect.store.EnforcementV4CommitCount() != 1 {
		t.Fatalf("prepare did not recover one linearized receipt: %#v", preparedCurrent)
	}
	historical, err := fixture.enforcement.InspectOperationDispatchEnforcementV4(context.Background(), ports.InspectOperationDispatchEnforcementRequestV4{
		Operation: fixture.prepare.Operation, EffectID: fixture.prepare.EffectID, PermitID: fixture.prepare.PermitID, Phase: ports.OperationDispatchEnforcementPrepareV4,
	})
	if err != nil || historical.Digest != preparedCurrent.Journal.Digest {
		t.Fatalf("historical enforcement Inspect failed: %#v err=%v", historical, err)
	}
	historicalOnly := control.OperationDispatchEnforcementGatewayV4{Facts: fixture.effect.store}
	if recovered, err := historicalOnly.InspectOperationDispatchEnforcementV4(context.Background(), ports.InspectOperationDispatchEnforcementRequestV4{
		Operation: fixture.prepare.Operation, EffectID: fixture.prepare.EffectID, PermitID: fixture.prepare.PermitID, Phase: ports.OperationDispatchEnforcementPrepareV4,
	}); err != nil || recovered.Digest != historical.Digest {
		t.Fatalf("historical Inspect incorrectly depended on current readers: %#v err=%v", recovered, err)
	}
	current, err := fixture.enforcement.InspectCurrentOperationDispatchEnforcementV4(context.Background(), ports.InspectCurrentOperationDispatchEnforcementRequestV4{
		Inspect:      ports.InspectOperationDispatchEnforcementRequestV4{Operation: fixture.prepare.Operation, EffectID: fixture.prepare.EffectID, PermitID: fixture.prepare.PermitID, Phase: ports.OperationDispatchEnforcementPrepareV4},
		PermitDigest: fixture.prepare.PermitDigest, AdmissionDigest: fixture.prepare.AdmissionDigest,
		ReviewAuthorization: fixture.prepare.ReviewAuthorization, SandboxAttempt: fixture.prepare.SandboxAttempt, SandboxProjectionDigest: fixture.prepare.SandboxProjectionDigest,
	})
	if err != nil || current.Phase.ReceiptDigest != preparedCurrent.Phase.ReceiptDigest {
		t.Fatalf("current enforcement Inspect failed: %#v err=%v", current, err)
	}

	execute := fixture.prepare
	execute.Phase = ports.OperationDispatchEnforcementExecuteV4
	execute.ExpectedJournalRevision = 1
	execute.Prepare = &preparedCurrent.Phase
	execute.PreparedAttempt = preparedAttemptForEnforcementV4(t, fixture, preparedCurrent)
	fixture.effect.store.LoseNextEnforcementV4Reply()
	executed, err := fixture.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), execute)
	if err != nil {
		t.Fatal(err)
	}
	if executed.Phase.Phase != ports.OperationDispatchEnforcementExecuteV4 || executed.Journal.Revision != 2 || fixture.effect.store.EnforcementV4CommitCount() != 2 {
		t.Fatalf("execute did not append the second immutable slot: %#v", executed)
	}
	if executed.Dispatch.Record.Enforcement != nil {
		t.Fatal("4.1 sidecar rewrote the frozen V4.0 Permit enforcement field")
	}
}

func TestOperationDispatchEnforcementV4DriftFailsBeforeJournalWrite(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*operationEnforcementFixtureV4)
	}{
		{name: "review", mutate: func(f *operationEnforcementFixtureV4) {
			f.dispatch.review.mutateAndSeal(t, f.effect.now, func(value *ports.OperationReviewCurrentProjectionV4) {
				value.CurrentnessDigest = core.DigestBytes([]byte("drifted-review"))
			})
		}},
		{name: "authority", mutate: func(f *operationEnforcementFixtureV4) {
			f.effect.current.mutate(func(value *ports.OperationGovernanceSnapshotV3) {
				value.Authority.Digest = core.DigestBytes([]byte("drifted-authority"))
			})
		}},
		{name: "sandbox_reservation", mutate: func(f *operationEnforcementFixtureV4) {
			f.sandbox.mutate(t, func(value *ports.OperationDispatchSandboxCurrentProjectionV4) {
				value.Reservation.Revision++
				value.Reservation.Digest = core.DigestBytes([]byte("replacement-reservation"))
			})
		}},
		{name: "attempt_revision", mutate: func(f *operationEnforcementFixtureV4) {
			f.sandbox.mutate(t, func(value *ports.OperationDispatchSandboxCurrentProjectionV4) {
				value.Attempt.Revision++
			})
		}},
		{name: "attempt_digest", mutate: func(f *operationEnforcementFixtureV4) {
			f.sandbox.mutate(t, func(value *ports.OperationDispatchSandboxCurrentProjectionV4) {
				value.Attempt.Digest = core.DigestBytes([]byte("replacement-attempt"))
			})
		}},
		{name: "attempt_exact_ttl", mutate: func(f *operationEnforcementFixtureV4) {
			f.sandbox.mutate(t, func(value *ports.OperationDispatchSandboxCurrentProjectionV4) {
				value.Attempt.ExpiresUnixNano = f.effect.now.UnixNano()
				value.ExpiresUnixNano = f.effect.now.UnixNano()
			})
		}},
		{name: "sandbox_lease", mutate: func(f *operationEnforcementFixtureV4) {
			f.sandbox.mutate(t, func(value *ports.OperationDispatchSandboxCurrentProjectionV4) {
				value.SandboxLease.Epoch++
				value.RuntimeLease.Lease = value.SandboxLease
				lease := value.SandboxLease
				value.Operation.ExecutionScope.SandboxLease = &lease
				value.Operation.ExecutionScopeDigest, _ = ports.ExecutionScopeDigestV2(value.Operation.ExecutionScope)
				value.OperationDigest, _ = value.Operation.DigestV3()
				value.RuntimeLease.ScopeDigest = value.Operation.ExecutionScopeDigest
			})
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newOperationEnforcementFixtureV4(t, "enforcement-drift-"+testCase.name)
			testCase.mutate(fixture)
			if _, err := fixture.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), fixture.prepare); err == nil {
				t.Fatal("drifted current fact produced an enforcement receipt")
			}
			if fixture.effect.store.EnforcementV4CommitCount() != 0 {
				t.Fatal("failed current validation changed the enforcement journal")
			}
		})
	}
}

func TestOperationDispatchEnforcementV4VerifierMustEqualPermitEnforcementPoint(t *testing.T) {
	fixture := newOperationEnforcementFixtureV4(t, "enforcement-verifier")
	request := fixture.prepare
	request.Verifier.Capability = "custom/other-verifier"
	if _, err := fixture.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), request); !core.HasReason(err, core.ReasonDispatchPermitInvalid) {
		t.Fatalf("unbound verifier reached the Effect Owner: %v", err)
	}
	if fixture.effect.store.EnforcementV4CommitCount() != 0 {
		t.Fatal("unbound verifier changed the enforcement journal")
	}
}

func TestOperationDispatchEnforcementV4CurrentPhaseRefMustExactlyDeriveFromJournalReceipt(t *testing.T) {
	fixture := newOperationEnforcementFixtureV4(t, "enforcement-ref-exact")
	prepared, err := fixture.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), fixture.prepare)
	if err != nil {
		t.Fatal(err)
	}
	execute := fixture.prepare
	execute.Phase = ports.OperationDispatchEnforcementExecuteV4
	execute.ExpectedJournalRevision = 1
	execute.Prepare = &prepared.Phase
	execute.PreparedAttempt = preparedAttemptForEnforcementV4(t, fixture, prepared)
	executed, err := fixture.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), execute)
	if err != nil {
		t.Fatal(err)
	}

	for _, phase := range []struct {
		name    string
		current ports.CurrentOperationDispatchEnforcementV4
	}{
		{name: "prepare", current: prepared},
		{name: "execute", current: executed},
	} {
		t.Run(phase.name, func(t *testing.T) {
			mutations := []struct {
				name   string
				mutate func(*ports.OperationDispatchEnforcementPhaseRefV4)
			}{
				{name: "receipt_digest", mutate: func(ref *ports.OperationDispatchEnforcementPhaseRefV4) {
					ref.ReceiptDigest = core.DigestBytes([]byte("forged-receipt"))
				}},
				{name: "permit_revision", mutate: func(ref *ports.OperationDispatchEnforcementPhaseRefV4) { ref.PermitFactRevision++ }},
				{name: "permit_digest", mutate: func(ref *ports.OperationDispatchEnforcementPhaseRefV4) {
					ref.PermitDigest = core.DigestBytes([]byte("forged-permit"))
				}},
				{name: "admission", mutate: func(ref *ports.OperationDispatchEnforcementPhaseRefV4) {
					ref.AdmissionDigest = core.DigestBytes([]byte("forged-admission"))
				}},
				{name: "authorization", mutate: func(ref *ports.OperationDispatchEnforcementPhaseRefV4) { ref.ReviewAuthorization.Revision++ }},
				{name: "attempt", mutate: func(ref *ports.OperationDispatchEnforcementPhaseRefV4) { ref.SandboxAttempt.Revision++ }},
				{name: "validated_time", mutate: func(ref *ports.OperationDispatchEnforcementPhaseRefV4) { ref.ValidatedUnixNano++ }},
				{name: "expiry", mutate: func(ref *ports.OperationDispatchEnforcementPhaseRefV4) { ref.ExpiresUnixNano-- }},
			}
			for _, mutation := range mutations {
				t.Run(mutation.name, func(t *testing.T) {
					forged := phase.current
					mutation.mutate(&forged.Phase)
					if _, err := ports.SealCurrentOperationDispatchEnforcementV4(forged); err == nil {
						t.Fatal("re-sealed current envelope accepted a phase ref not derived from its journal receipt")
					}
				})
			}

			forgedJournal := phase.current.Journal
			if phase.name == "prepare" {
				receipt := *forgedJournal.Prepare
				receipt.Digest = core.DigestBytes([]byte("forged-historical-prepare-receipt"))
				forgedJournal.Prepare = &receipt
			} else {
				receipt := *forgedJournal.Execute
				receipt.Digest = core.DigestBytes([]byte("forged-historical-execute-receipt"))
				forgedJournal.Execute = &receipt
			}
			if _, err := ports.SealOperationDispatchEnforcementJournalV4(forgedJournal); err == nil {
				t.Fatal("re-sealed historical journal accepted a changed receipt digest")
			}

			forgedCurrent := phase.current
			forgedJournal = phase.current.Journal
			if phase.name == "prepare" {
				receipt := *forgedJournal.Prepare
				receipt.ValidatedUnixNano++
				receipt, err = ports.SealOperationDispatchEnforcementPhaseReceiptV4(receipt)
				if err != nil {
					t.Fatal(err)
				}
				forgedJournal.Prepare = &receipt
				forgedJournal.UpdatedUnixNano = receipt.ValidatedUnixNano
			} else {
				receipt := *forgedJournal.Execute
				receipt.ValidatedUnixNano++
				receipt, err = ports.SealOperationDispatchEnforcementPhaseReceiptV4(receipt)
				if err != nil {
					t.Fatal(err)
				}
				forgedJournal.Execute = &receipt
				forgedJournal.UpdatedUnixNano = receipt.ValidatedUnixNano
			}
			forgedJournal, err = ports.SealOperationDispatchEnforcementJournalV4(forgedJournal)
			if err != nil {
				t.Fatal(err)
			}
			forgedCurrent.Journal = forgedJournal
			if _, err := ports.SealCurrentOperationDispatchEnforcementV4(forgedCurrent); err == nil {
				t.Fatal("current envelope accepted a re-sealed changed journal receipt with the old phase ref")
			}
		})
	}
}

func TestOperationDispatchEnforcementV4RawOwnerIdempotencyConflictAndConcurrent64(t *testing.T) {
	fixture := newOperationEnforcementFixtureV4(t, "enforcement-owner")
	current, err := fixture.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), fixture.prepare)
	if err != nil {
		t.Fatal(err)
	}
	raw := control.AppendOperationDispatchEnforcementRequestV4{
		Operation: fixture.prepare.Operation, EffectID: fixture.prepare.EffectID, PermitID: fixture.prepare.PermitID,
		Receipt: *current.Journal.Prepare,
	}
	const workers = 64
	var wait sync.WaitGroup
	errors := make(chan error, workers)
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			journal, err := fixture.effect.store.AppendOperationDispatchEnforcementV4(context.Background(), raw)
			if err == nil && journal.Digest != current.Journal.Digest {
				err = core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "concurrent replay returned another journal")
			}
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent exact phase replay failed: %v", err)
		}
	}
	if fixture.effect.store.EnforcementV4CommitCount() != 1 {
		t.Fatalf("exact phase committed %d times", fixture.effect.store.EnforcementV4CommitCount())
	}
	changed := raw
	changed.Receipt.Sandbox.Attempt.Revision++
	changed.Receipt.Sandbox.Attempt.Digest = core.DigestBytes([]byte("forged-attempt"))
	changed.Receipt.SandboxAttempt = changed.Receipt.Sandbox.Attempt
	changed.Receipt.Sandbox, err = ports.SealOperationDispatchSandboxCurrentProjectionV4(changed.Receipt.Sandbox)
	if err != nil {
		t.Fatal(err)
	}
	changed.Receipt, err = ports.SealOperationDispatchEnforcementPhaseReceiptV4(changed.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.effect.store.AppendOperationDispatchEnforcementV4(context.Background(), changed); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same phase changed Attempt Fact without conflict: %v", err)
	}
}

func TestOperationDispatchEnforcementV4PrepareDoesNotAuthorizeExecuteAfterCurrentDrift(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*operationEnforcementFixtureV4)
	}{
		{name: "review", mutate: func(f *operationEnforcementFixtureV4) {
			f.dispatch.review.mutateAndSeal(t, f.effect.now, func(value *ports.OperationReviewCurrentProjectionV4) {
				value.Verdict.Digest = core.DigestBytes([]byte("post-prepare-verdict"))
				value.CurrentnessDigest = core.DigestBytes([]byte("post-prepare-review"))
			})
		}},
		{name: "binding", mutate: func(f *operationEnforcementFixtureV4) {
			f.effect.current.mutate(func(value *ports.OperationGovernanceSnapshotV3) {
				value.Binding.Digest = core.DigestBytes([]byte("post-prepare-binding"))
			})
		}},
		{name: "identity", mutate: func(f *operationEnforcementFixtureV4) {
			f.effect.current.mutate(func(value *ports.OperationGovernanceSnapshotV3) {
				value.Identity.Digest = core.DigestBytes([]byte("post-prepare-identity"))
			})
		}},
		{name: "current_scope", mutate: func(f *operationEnforcementFixtureV4) {
			f.effect.current.mutate(func(value *ports.OperationGovernanceSnapshotV3) {
				value.CurrentScope.Digest = core.DigestBytes([]byte("post-prepare-scope"))
			})
		}},
		{name: "authority", mutate: func(f *operationEnforcementFixtureV4) {
			f.effect.current.mutate(func(value *ports.OperationGovernanceSnapshotV3) {
				value.Authority.Digest = core.DigestBytes([]byte("post-prepare-authority"))
			})
		}},
		{name: "budget", mutate: func(f *operationEnforcementFixtureV4) {
			f.effect.current.mutate(func(value *ports.OperationGovernanceSnapshotV3) {
				value.Budget.Digest = core.DigestBytes([]byte("post-prepare-budget"))
			})
		}},
		{name: "policy", mutate: func(f *operationEnforcementFixtureV4) {
			f.effect.current.mutate(func(value *ports.OperationGovernanceSnapshotV3) {
				value.Policy.Digest = core.DigestBytes([]byte("post-prepare-policy"))
			})
		}},
		{name: "capability", mutate: func(f *operationEnforcementFixtureV4) {
			f.effect.current.mutate(func(value *ports.OperationGovernanceSnapshotV3) {
				value.CapabilityGrantDigest = core.DigestBytes([]byte("post-prepare-capability"))
			})
		}},
		{name: "credential", mutate: func(f *operationEnforcementFixtureV4) {
			f.effect.current.mutate(func(value *ports.OperationGovernanceSnapshotV3) {
				value.Credentials = append(value.Credentials, ports.OperationCredentialCurrentFactV3{})
			})
		}},
		{name: "sandbox", mutate: func(f *operationEnforcementFixtureV4) {
			f.sandbox.mutate(t, func(value *ports.OperationDispatchSandboxCurrentProjectionV4) {
				value.Slot.Revision++
				value.Slot.Digest = core.DigestBytes([]byte("post-prepare-slot"))
			})
		}},
		{name: "attempt_revision", mutate: func(f *operationEnforcementFixtureV4) {
			f.sandbox.mutate(t, func(value *ports.OperationDispatchSandboxCurrentProjectionV4) {
				value.Attempt.Revision++
			})
		}},
		{name: "fence_epoch", mutate: func(f *operationEnforcementFixtureV4) {
			f.sandbox.mutate(t, func(value *ports.OperationDispatchSandboxCurrentProjectionV4) {
				value.RuntimeLease.FenceEpoch++
			})
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newOperationEnforcementFixtureV4(t, "post-prepare-"+testCase.name)
			prepared, err := fixture.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), fixture.prepare)
			if err != nil {
				t.Fatal(err)
			}
			execute := fixture.prepare
			execute.Phase = ports.OperationDispatchEnforcementExecuteV4
			execute.ExpectedJournalRevision = 1
			execute.Prepare = &prepared.Phase
			execute.PreparedAttempt = preparedAttemptForEnforcementV4(t, fixture, prepared)
			testCase.mutate(fixture)
			if _, err := fixture.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), execute); err == nil {
				t.Fatal("prepare receipt was treated as execute authority after current drift")
			}
			if fixture.effect.store.EnforcementV4CommitCount() != 1 {
				t.Fatal("failed execute changed the append-only journal")
			}
		})
	}
}

func TestOperationDispatchEnforcementV4ConformanceNeverClaimsProviderOrProduction(t *testing.T) {
	fixture := newOperationEnforcementFixtureV4(t, "enforcement-conformance")
	prepared, err := fixture.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), fixture.prepare)
	if err != nil {
		t.Fatal(err)
	}
	report, err := conformance.CheckOperationDispatchEnforcementV4(context.Background(), conformance.OperationDispatchEnforcementCaseV4{
		Gateway: fixture.enforcement, Prepare: fixture.prepare,
		PreparedAttempt: *preparedAttemptForEnforcementV4(t, fixture, prepared),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.ProviderCalled || report.BeginIsExecution || report.ProductionClaimEligible {
		t.Fatalf("enforcement conformance exceeded Runtime authority: %#v", report)
	}
}

type operationSandboxReaderV4 struct {
	mu    sync.Mutex
	value ports.OperationDispatchSandboxCurrentProjectionV4
}

func (r *operationSandboxReaderV4) InspectOperationDispatchSandboxCurrentV4(_ context.Context, _ ports.OperationSubjectV3, _ core.EffectIntentID, _ ports.OperationDispatchSandboxFactRefV4) (ports.OperationDispatchSandboxCurrentProjectionV4, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.value, nil
}

func (r *operationSandboxReaderV4) mutate(t *testing.T, change func(*ports.OperationDispatchSandboxCurrentProjectionV4)) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	change(&r.value)
	sealed, err := ports.SealOperationDispatchSandboxCurrentProjectionV4(r.value)
	if err != nil {
		t.Fatal(err)
	}
	r.value = sealed
}

type operationEnforcementFixtureV4 struct {
	effect      *operationFixtureV3
	dispatch    *operationDispatchFixtureV4
	sandbox     *operationSandboxReaderV4
	enforcement control.OperationDispatchEnforcementGatewayV4
	prepare     ports.EnforceCurrentOperationDispatchRequestV4
}

func newOperationEnforcementFixtureV4(t *testing.T, suffix string) *operationEnforcementFixtureV4 {
	return newOperationEnforcementFixtureForScopeV4(t, suffix, "run-enforcement", "tenant-enforcement", "")
}

func newActivationOperationEnforcementFixtureV4(t *testing.T, suffix string) *operationEnforcementFixtureV4 {
	return newOperationEnforcementFixtureForScopeV4(t, suffix, "", "tenant-enforcement", "")
}

func newActivationOperationEnforcementFixtureForTenantV4(t *testing.T, suffix string, tenantID core.TenantID) *operationEnforcementFixtureV4 {
	return newOperationEnforcementFixtureForScopeV4(t, suffix, "", tenantID, "")
}

func newActivationOperationEnforcementFixtureForEffectKindV4(t *testing.T, suffix string, effectKind ports.EffectKindV2) *operationEnforcementFixtureV4 {
	return newOperationEnforcementFixtureForScopeV4(t, suffix, "", "tenant-enforcement", effectKind)
}

func newOperationEnforcementFixtureForScopeV4(t *testing.T, suffix string, runID core.AgentRunID, tenantID core.TenantID, effectKind ports.EffectKindV2) *operationEnforcementFixtureV4 {
	t.Helper()
	scope := core.ExecutionScope{
		Identity:     core.AgentIdentityRef{TenantID: tenantID, ID: "identity-enforcement", Epoch: 1},
		Lineage:      core.LineageRef{ID: "lineage-enforcement", PlanDigest: core.DigestBytes([]byte("lineage-enforcement"))},
		Instance:     core.InstanceRef{ID: "instance-enforcement", Epoch: 1},
		SandboxLease: &core.SandboxLeaseRef{ID: "lease-enforcement", Epoch: 1}, AuthorityEpoch: 1,
	}
	effect := newOperationFixtureForRunAndKindV3(t, runID, &scope, effectKind)
	reviewGateway, authorizationStore, review := operationReviewAuthorizationForEffectV4(t, effect)
	effect.store.BindOperationReviewAuthorizationFactsV4(authorizationStore)
	authorization, err := reviewGateway.CreateOperationReviewAuthorizationV4(context.Background(), ports.CreateOperationReviewAuthorizationRequestV4{
		AuthorizationID: "authorization-" + suffix, Operation: effect.intent.Operation, EffectID: effect.intent.ID,
		ExpectedEffectRevision: effect.accepted.Revision, RequestedTTL: 20 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	admissions := control.OperationEffectAdmissionGatewayV3{Effects: effect.store}
	admission, err := admissions.InspectAcceptedOperationEffectV3(context.Background(), effect.intent.Operation, effect.intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	dispatchGateway := control.OperationGovernanceGatewayV4{Effects: effect.store, Admissions: admissions, Reviews: reviewGateway, Current: effect.current, Clock: func() time.Time { return effect.now }}
	issue := ports.IssueGovernedOperationDispatchRequestV4{
		Operation: effect.intent.Operation, EffectID: effect.intent.ID, ExpectedEffectRevision: effect.accepted.Revision,
		Admission: admission, ReviewAuthorization: authorization.RefV4(), PermitID: "permit-" + suffix,
		AttemptID: "attempt-" + suffix, PermitTTL: 10 * time.Second,
	}
	issued, err := dispatchGateway.IssueOperationDispatchV4(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}
	begun, err := dispatchGateway.BeginOperationDispatchV4(context.Background(), beginOperationDispatchRequestV4(&operationDispatchFixtureV4{effect: effect, authorization: authorization, issue: issue}, issued))
	if err != nil {
		t.Fatal(err)
	}
	sandboxProjection := sandboxProjectionForEnforcementV4(t, effect, begun)
	sandbox := &operationSandboxReaderV4{value: sandboxProjection}
	gateway := control.OperationDispatchEnforcementGatewayV4{Dispatch: dispatchGateway, Sandbox: sandbox, Facts: effect.store, Clock: func() time.Time { return effect.now }}
	prepare := ports.EnforceCurrentOperationDispatchRequestV4{
		Operation: effect.intent.Operation, EffectID: effect.intent.ID, PermitID: issue.PermitID,
		ExpectedPermitFactRevision: begun.Record.Revision, PermitDigest: begun.Record.PermitDigest,
		AdmissionDigest: begun.Record.Permit.Admission.Digest, ReviewAuthorization: authorization.RefV4(), AttemptID: issue.AttemptID,
		Phase: ports.OperationDispatchEnforcementPrepareV4, SandboxAttempt: sandboxProjection.Attempt, SandboxReservation: sandboxProjection.Reservation,
		SandboxProjectionDigest: sandboxProjection.ProjectionDigest, Verifier: begun.Record.Permit.LegacyPermit.EnforcementPoint,
	}
	dispatch := &operationDispatchFixtureV4{effect: effect, review: review, authorizationStore: authorizationStore, authorization: authorization, gateway: dispatchGateway, issue: issue}
	return &operationEnforcementFixtureV4{effect: effect, dispatch: dispatch, sandbox: sandbox, enforcement: gateway, prepare: prepare}
}

func operationReviewAuthorizationForEffectV4(t *testing.T, fixture *operationFixtureV3) (kernel.OperationReviewAuthorizationGatewayV4, *fakes.OperationReviewAuthorizationStoreV4, *operationReviewReaderV4) {
	t.Helper()
	snapshot := fixture.current.snapshot
	intentDigest, err := fixture.intent.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	review, err := ports.SealOperationReviewCurrentProjectionV4(ports.OperationReviewCurrentProjectionV4{
		Operation: fixture.intent.Operation, IntentID: fixture.intent.ID, IntentRevision: fixture.intent.Revision, IntentDigest: intentDigest,
		PayloadSchema: fixture.intent.Payload.Schema, PayloadDigest: fixture.intent.Payload.ContentDigest, PayloadRevision: fixture.intent.PayloadRevision,
		Target: ports.OperationReviewTargetRefV4{Ref: fixture.intent.Target, Revision: fixture.intent.Review.CandidateRevision, Digest: fixture.intent.Review.CandidateDigest},
		Case:   snapshot.Review.Case, Verdict: snapshot.Review.Verdict, Basis: ports.OperationReviewBasisAcceptedV4,
		Policy:            ports.OperationGovernanceFactRefV3{Ref: "review-policy-enforcement", Revision: 1, Digest: fixture.intent.Review.PolicyDigest, ExpiresUnixNano: fixture.now.Add(30 * time.Second).UnixNano()},
		ReviewerAuthority: snapshot.Review.ReviewerAuthority, Scope: snapshot.CurrentScope, Binding: snapshot.Binding,
		DecisionEvidence: []ports.EvidenceRecordRefV2{{LedgerScopeDigest: core.DigestBytes([]byte("review-ledger-enforcement")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("review-evidence-enforcement"))}},
		Current:          true, CurrentnessDigest: core.DigestBytes([]byte("review-currentness-enforcement")), ExpiresUnixNano: fixture.now.Add(30 * time.Second).UnixNano(),
	}, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	reader := &operationReviewReaderV4{value: review}
	store := fakes.NewOperationReviewAuthorizationStoreV4(func() time.Time { return fixture.now })
	gateway := kernel.OperationReviewAuthorizationGatewayV4{Facts: store, Effects: fixture.store, Governance: fixture.current, Reviews: reader, Clock: func() time.Time { return fixture.now }}
	return gateway, store, reader
}

func sandboxProjectionForEnforcementV4(t *testing.T, fixture *operationFixtureV3, begun ports.CurrentOperationDispatchAuthorizationV4) ports.OperationDispatchSandboxCurrentProjectionV4 {
	t.Helper()
	expires := fixture.now.Add(8 * time.Second).UnixNano()
	ref := func(id string) ports.OperationDispatchSandboxFactRefV4 {
		return ports.OperationDispatchSandboxFactRefV4{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id)), ExpiresUnixNano: expires}
	}
	lease := *fixture.intent.Operation.ExecutionScope.SandboxLease
	projection, err := ports.SealOperationDispatchSandboxCurrentProjectionV4(ports.OperationDispatchSandboxCurrentProjectionV4{
		Operation: fixture.intent.Operation, OperationDigest: mustOperationDigestForEnforcementV4(t, fixture.intent.Operation),
		EffectID: fixture.intent.ID, IntentRevision: fixture.intent.Revision, IntentDigest: begun.Record.Permit.LegacyPermit.IntentDigest,
		AttemptID: begun.Record.Permit.LegacyPermit.AttemptID, Attempt: ref(begun.Record.Permit.LegacyPermit.AttemptID), Reservation: ref("sandbox-reservation"), SandboxLease: lease,
		RuntimeLease: ports.OperationDispatchRuntimeLeaseBindingV4{Ref: ref("runtime-lease-binding"), Lease: lease, Instance: fixture.intent.Operation.ExecutionScope.Instance, FenceEpoch: 1, ScopeDigest: fixture.intent.Operation.ExecutionScopeDigest, ObservedRevision: 1},
		Generation:   ports.GenerationBindingAssociationRefV1{ID: "generation-association", Revision: 1, Digest: core.DigestBytes([]byte("generation-association"))},
		Placement:    ref("sandbox-placement"), Backend: ref("sandbox-backend"), Slot: ref("sandbox-slot"),
		ProviderBinding: begun.Record.Permit.LegacyPermit.EnforcementPoint, Current: true, ProjectionRevision: 1, ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func preparedAttemptForEnforcementV4(t *testing.T, fixture *operationEnforcementFixtureV4, prepared ports.CurrentOperationDispatchEnforcementV4) *ports.PreparedProviderAttemptRefV2 {
	t.Helper()
	legacy := prepared.Dispatch.Record.Permit.LegacyPermit
	legacyDigest, err := legacy.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	delegation := ports.ExecutionDelegationRefV2{ID: "delegation-enforcement", Revision: 1, Digest: core.DigestBytes([]byte("delegation-enforcement"))}
	id, err := ports.DerivePreparedProviderAttemptIDV2(delegation.ID, legacy.ID, legacy.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	value, err := ports.SealPreparedProviderAttemptRefV2(ports.PreparedProviderAttemptRefV2{
		ID: id, Revision: 1, DeclaredDelegation: delegation, OperationDigest: mustOperationDigestForEnforcementV4(t, legacy.Operation),
		IntentID: legacy.IntentID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest,
		PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest, AttemptID: legacy.AttemptID,
		Provider: legacy.EnforcementPoint, PayloadSchema: legacy.PayloadSchema, PayloadDigest: legacy.PayloadDigest, PayloadRevision: legacy.PayloadRevision,
		PreparedUnixNano: fixture.effect.now.UnixNano(), ExpiresUnixNano: fixture.effect.now.Add(7 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &value
}

func mustOperationDigestForEnforcementV4(t *testing.T, operation ports.OperationSubjectV3) core.Digest {
	t.Helper()
	digest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

var _ ports.OperationDispatchSandboxCurrentReaderV4 = (*operationSandboxReaderV4)(nil)
var _ ports.OperationDispatchEnforcementGovernancePortV4 = control.OperationDispatchEnforcementGatewayV4{}
var _ control.OperationDispatchEnforcementFactPortV4 = (*fakes.OperationEffectStoreV3)(nil)

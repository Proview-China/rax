package contract

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestModelTurnCandidateV2AllowsCustomCombinedComponentWithDistinctCapabilities(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	candidate := governedCandidateFixtureV2(t, now)
	if err := candidate.Validate(now); err != nil {
		t.Fatalf("valid custom component candidate rejected: %v", err)
	}
	first, err := candidate.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	second, err := candidate.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("candidate canonical digest changed between reads")
	}
	ref, err := candidate.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	if ref.ID != candidate.ID || ref.Revision != 1 || ref.Digest != first {
		t.Fatalf("candidate ref drifted: %#v", ref)
	}
}

func TestModelTurnCandidateV2RejectsCrossBoundaryAndRevisionDrift(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tests := []struct {
		name   string
		mutate func(*ModelTurnCandidateV2)
	}{
		{"session revision missing", func(c *ModelTurnCandidateV2) { c.ExpectedSessionRevision = 0 }},
		{"endpoint scope drift", func(c *ModelTurnCandidateV2) { c.Endpoint.Scope.Instance.Epoch++ }},
		{"context digest missing", func(c *ModelTurnCandidateV2) { c.ContextDigest = "" }},
		{"harness capability reused", func(c *ModelTurnCandidateV2) { c.Provider.Capability = c.Endpoint.Binding.Capability }},
		{"candidate expired", func(c *ModelTurnCandidateV2) { c.ExpiresUnixNano = now.UnixNano() }},
		{"initial has continuation", func(c *ModelTurnCandidateV2) {
			c.Continuation = governedContinuationFixtureV2(t, CandidateActionTurnV2)
		}},
		{"continuation absent", func(c *ModelTurnCandidateV2) { c.Kind = CandidateActionTurnV2 }},
		{"continuation kind swap", func(c *ModelTurnCandidateV2) {
			c.Kind = CandidateActionTurnV2
			c.Continuation = governedContinuationFixtureV2(t, CandidateInputTurnV2)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := governedCandidateFixtureV2(t, now)
			test.mutate(&candidate)
			if err := candidate.Validate(now); err == nil {
				t.Fatal("drifted candidate was accepted")
			}
		})
	}
}

func TestEndpointRefV2DigestBindsExactScopeAndProvider(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	candidate := governedCandidateFixtureV2(t, now)
	endpoint := candidate.Endpoint
	if err := endpoint.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*EndpointRefV2){
		func(e *EndpointRefV2) { e.Scope.AuthorityEpoch++ },
		func(e *EndpointRefV2) { e.Binding.BindingSetRevision++ },
		func(e *EndpointRefV2) { e.Binding.Capability = "custom/other" },
		func(e *EndpointRefV2) { e.Revision++ },
	} {
		changed := endpoint
		mutate(&changed)
		if err := changed.Validate(); err == nil {
			t.Fatal("endpoint identity drift was accepted")
		}
	}
}

func TestPendingActionV2BindsPayloadCandidateAndCapability(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	candidate := governedCandidateFixtureV2(t, now)
	ref, err := candidate.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	pending := PendingActionV2{Ref: "action-1", Capability: "custom/tool", Payload: candidate.Input, SourceCandidate: ref}
	pending.RequestDigest, err = pending.digestSubjectV2()
	if err != nil {
		t.Fatal(err)
	}
	if err := pending.Validate(); err != nil {
		t.Fatal(err)
	}
	changed := pending
	changed.Capability = "custom/other-tool"
	if err := changed.Validate(); err == nil {
		t.Fatal("capability swap preserved action authorization")
	}
	changed = pending
	changed.SourceCandidate.Revision++
	if err := changed.Validate(); err == nil {
		t.Fatal("candidate revision swap preserved action authorization")
	}
}

func TestGovernedSessionV2PhaseMatrixFailsClosed(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	candidate := governedCandidateFixtureV2(t, now)
	ref, err := candidate.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	base := GovernedSessionV2{ContractVersion: GovernedContractVersionV2, ID: "session-1", Revision: 1, Run: candidate.Run, Endpoint: candidate.Endpoint, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}
	creating := base
	creating.Phase = SessionCreatingV2
	if err := creating.Validate(); err != nil {
		t.Fatalf("creating rejected: %v", err)
	}
	waiting := base
	waiting.Phase, waiting.Turn, waiting.Candidate = SessionWaitingModelDispatchV2, 1, &ref
	if err := waiting.Validate(); err != nil {
		t.Fatalf("waiting dispatch rejected: %v", err)
	}
	reservation := governedReservationFixtureV2(ref, now)
	reserved := waiting
	reserved.Phase, reserved.DomainReservation = SessionModelDispatchReservedV2, &reservation
	if err := reserved.Validate(); err != nil {
		t.Fatalf("reserved dispatch rejected: %v", err)
	}
	reconciling := reserved
	reconciling.Phase = SessionReconcilingV2
	preparedExecution := governedExecutionFixtureV2(t, now, candidate, "")
	reconciling.Execution = &preparedExecution
	if err := reconciling.Validate(); err != nil {
		t.Fatalf("reconciling rejected: %v", err)
	}
	waitingSettlement := reserved
	waitingSettlement.Phase = SessionWaitingSettlementV2
	observedExecution := governedExecutionFixtureV2(t, now, candidate, runtimeports.ProviderAttemptObservedV2)
	waitingSettlement.Execution = &observedExecution
	if err := waitingSettlement.Validate(); err != nil {
		t.Fatalf("waiting settlement rejected: %v", err)
	}

	invalid := []GovernedSessionV2{creating, waiting, waitingSettlement}
	invalid[0].Turn = 1
	invalid[1].Candidate = nil
	invalid[2].Candidate = nil
	for index := range invalid {
		if err := invalid[index].Validate(); err == nil {
			t.Fatalf("invalid phase fields accepted at %d", index)
		}
	}
}

func TestGovernedSessionTransitionV2UsesSandboxLeaseValueSemantics(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	candidate := governedCandidateFixtureV2(t, now)
	current := GovernedSessionV2{ContractVersion: GovernedContractVersionV2, ID: "session-1", Revision: 1, Run: candidate.Run, Endpoint: candidate.Endpoint, Phase: SessionCreatingV2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}
	ref, err := candidate.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	next := current
	next.Run.Scope.SandboxLease = &core.SandboxLeaseRef{ID: current.Run.Scope.SandboxLease.ID, Epoch: current.Run.Scope.SandboxLease.Epoch}
	next.Revision, next.Phase, next.Turn, next.Candidate = 2, SessionWaitingModelDispatchV2, 1, &ref
	if err := ValidateSessionTransitionV2(current, next); err != nil {
		t.Fatalf("equal sandbox value with a different pointer was rejected: %v", err)
	}
	next.Run.Scope.SandboxLease.Epoch++
	if err := ValidateSessionTransitionV2(current, next); err == nil {
		t.Fatal("sandbox lease epoch drift was accepted")
	}
}

func TestGovernedSessionV2RequiresExactRuntimeAttemptBeforeModelStateCanAdvance(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	candidate := governedCandidateFixtureV2(t, now)
	ref, err := candidate.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	waiting := GovernedSessionV2{ContractVersion: GovernedContractVersionV2, ID: "session-1", Revision: 2, Run: candidate.Run, Endpoint: candidate.Endpoint, Phase: SessionWaitingModelDispatchV2, Turn: 1, Candidate: &ref, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}
	forgedInflight := waiting
	forgedInflight.Revision++
	forgedInflight.Phase = SessionModelInFlightV2
	forgedInflight.UpdatedUnixNano++
	if err := ValidateSessionTransitionV2(waiting, forgedInflight); err == nil {
		t.Fatal("session entered model_in_flight without persisted Runtime attempt refs")
	}
	reservation := governedReservationFixtureV2(ref, now)
	reserved := waiting
	reserved.Revision++
	reserved.Phase = SessionModelDispatchReservedV2
	reserved.DomainReservation = &reservation
	reserved.UpdatedUnixNano++
	if err := ValidateSessionTransitionV2(waiting, reserved); err != nil {
		t.Fatalf("exact reservation was rejected: %v", err)
	}
	prepared := governedExecutionFixtureV2(t, now, candidate, "")
	inflight := reserved
	inflight.Revision++
	inflight.Phase = SessionModelInFlightV2
	inflight.Execution = &prepared
	inflight.UpdatedUnixNano++
	if err := ValidateSessionTransitionV2(reserved, inflight); err != nil {
		t.Fatalf("exact prepared Runtime attempt was rejected: %v", err)
	}
	forgedTerminal := inflight
	forgedTerminal.Revision++
	forgedTerminal.Phase = SessionTerminalV2
	forgedTerminal.Candidate = nil
	forgedTerminal.DomainReservation = nil
	forgedTerminal.CompletionClaim = ClaimCompleted
	forgedTerminal.UpdatedUnixNano++
	if err := ValidateSessionTransitionV2(inflight, forgedTerminal); err == nil {
		t.Fatal("session claimed completion without an exact provider Settlement")
	}
	observed := governedExecutionFixtureV2(t, now, candidate, runtimeports.ProviderAttemptObservedV2)
	waitingSettlement := inflight
	waitingSettlement.Revision++
	waitingSettlement.Phase = SessionWaitingSettlementV2
	waitingSettlement.Execution = &observed
	waitingSettlement.UpdatedUnixNano++
	if err := ValidateSessionTransitionV2(inflight, waitingSettlement); err != nil {
		t.Fatalf("exact observation could not enter waiting settlement: %v", err)
	}
	settled := governedSettledExecutionFixtureV2(t, now, candidate, SettledTurnCompletedV2)
	terminal := waitingSettlement
	terminal.Revision++
	terminal.Phase = SessionTerminalV2
	terminal.Candidate = nil
	terminal.DomainReservation = nil
	terminal.Execution = &settled
	terminal.CompletionClaim = ClaimCompleted
	terminal.UpdatedUnixNano++
	if err := ValidateSessionTransitionV2(waitingSettlement, terminal); err != nil {
		t.Fatalf("terminal settlement path was rejected: %v", err)
	}
}

func TestGovernedSessionV2CanCancelPreTerminalWaitsButCannotClaimCompletion(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	candidate := governedCandidateFixtureV2(t, now)
	ref, err := candidate.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	creating := GovernedSessionV2{ContractVersion: GovernedContractVersionV2, ID: "session-1", Revision: 1, Run: candidate.Run, Endpoint: candidate.Endpoint, Phase: SessionCreatingV2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}
	waiting := creating
	waiting.Revision, waiting.Phase, waiting.Turn, waiting.Candidate = 2, SessionWaitingModelDispatchV2, 1, &ref
	pending, err := NewPendingActionV2("action-1", "custom/tool", candidate.Input, ref)
	if err != nil {
		t.Fatal(err)
	}
	action := waiting
	action.Revision, action.Phase, action.Candidate, action.PendingAction = 3, SessionWaitingActionV2, nil, &pending
	settledExecution := governedSettledExecutionFixtureV2(t, now, candidate, SettledTurnActionRequiredV2)
	action.Execution = &settledExecution

	for _, current := range []GovernedSessionV2{creating, waiting, action} {
		next := current
		next.Revision++
		next.Phase = SessionTerminalV2
		next.Candidate, next.PendingAction, next.PendingInput = nil, nil, nil
		next.Execution = nil
		next.CompletionClaim = ClaimCancelled
		next.UpdatedUnixNano = now.Add(time.Second).UnixNano()
		if err := ValidateSessionTransitionV2(current, next); err != nil {
			t.Fatalf("phase %s cannot be cancelled: %v", current.Phase, err)
		}
		forged := next
		forged.CompletionClaim = ClaimCompleted
		if err := ValidateSessionTransitionV2(current, forged); err == nil {
			t.Fatalf("phase %s claimed completion without a model terminal observation", current.Phase)
		}
	}
}

func governedReservationFixtureV2(candidate CandidateRefV2, now time.Time) ModelDispatchReservationRefV2 {
	return ModelDispatchReservationRefV2{ID: "reservation-1", Digest: core.DigestBytes([]byte("reservation")), AttemptID: "attempt-1", IntentDigest: core.DigestBytes([]byte("intent")), CandidateDigest: candidate.Digest, ReservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
}

func FuzzModelTurnCandidateV2Canonical(f *testing.F) {
	f.Add("candidate-1", uint32(1))
	f.Add("custom-candidate", uint32(42))
	f.Fuzz(func(t *testing.T, id string, turn uint32) {
		if id == "" || len(id) > MaxReferenceBytes || turn == 0 {
			return
		}
		now := time.Unix(1_800_000_000, 0)
		candidate := governedCandidateFixtureV2(t, now)
		candidate.ID, candidate.Turn = id, turn
		if err := candidate.Validate(now); err != nil {
			return
		}
		first, err := candidate.DigestV2()
		if err != nil {
			t.Fatal(err)
		}
		second, err := candidate.DigestV2()
		if err != nil {
			t.Fatal(err)
		}
		if first != second {
			t.Fatal("canonical digest is nondeterministic")
		}
	})
}

func governedCandidateFixtureV2(t *testing.T, now time.Time) ModelTurnCandidateV2 {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: digest("plan")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox-1", Epoch: 1}, AuthorityEpoch: 1}
	harnessBinding := runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-1", BindingSetRevision: 1, ComponentID: "custom/combined", ManifestDigest: digest("manifest"), ArtifactDigest: digest("artifact"), Capability: "praxis/harness-execution"}
	endpoint, err := NewEndpointRefV2("endpoint-1", scope, harnessBinding)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"prompt":"hello"}`)
	schema := runtimeports.SchemaRefV2{Namespace: "custom", Name: "model-input", Version: "1.0.0", MediaType: "application/json", ContentDigest: digest("schema")}
	input := runtimeports.OpaquePayloadV2{Schema: schema, ContentDigest: core.DigestBytes(payload), Length: uint64(len(payload)), Inline: payload, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "custom/default-limit", Digest: digest("limit")}}
	modelBinding := harnessBinding
	modelBinding.Capability = "praxis/model-turn"
	return ModelTurnCandidateV2{ContractVersion: GovernedContractVersionV2, ID: "candidate-1", Revision: 1, Run: RunRef{Scope: scope, RunID: "run-1"}, Endpoint: endpoint, SessionRef: "session-1", ExpectedSessionRevision: 1, Turn: 1, Kind: CandidateInitialTurnV2, Input: input, ContextRef: "context-1", ContextDigest: digest("context"), Provider: modelBinding, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
}

func governedExecutionFixtureV2(t *testing.T, now time.Time, candidate ModelTurnCandidateV2, state runtimeports.ProviderAttemptStateV2) runtimeports.GovernedExecutionAttemptRefsV2 {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	operationDigest, intentDigest, permitDigest := digest("operation"), digest("intent"), digest("permit")
	declared := runtimeports.ExecutionDelegationRefV2{ID: "delegation-1", Revision: 1, Digest: digest("delegation-declared")}
	preparedID, err := runtimeports.DerivePreparedProviderAttemptIDV2(declared.ID, "permit-1", "attempt-1")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{ID: preparedID, Revision: 1, DeclaredDelegation: declared, OperationDigest: operationDigest, IntentID: "effect-1", IntentRevision: 1, IntentDigest: intentDigest, PermitID: "permit-1", PermitRevision: 1, PermitDigest: permitDigest, AttemptID: "attempt-1", Provider: candidate.Provider, PayloadSchema: candidate.Input.Schema, PayloadDigest: candidate.Input.ContentDigest, PayloadRevision: 1, PreparedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	delegation := runtimeports.ExecutionDelegationRefV2{ID: declared.ID, Revision: 2, Digest: digest("delegation-prepared")}
	result := runtimeports.GovernedExecutionAttemptRefsV2{Admission: runtimeports.OperationEffectAdmissionReceiptV3{OperationDigest: operationDigest, EffectID: "effect-1", IntentRevision: 1, IntentDigest: intentDigest, FactRevision: 2, State: "accepted"}, PermitID: "permit-1", PermitRevision: 1, PermitDigest: permitDigest, AttemptID: "attempt-1", Delegation: delegation, Prepared: prepared, Enforcement: runtimeports.PersistedOperationEnforcementRefV3{PermitID: "permit-1", PermitRevision: 1, PermitDigest: permitDigest, AttemptID: "attempt-1", OperationDigest: operationDigest, Provider: candidate.Provider, ReceiptDigest: digest("enforcement"), RecordedRevision: 3}}
	if state != "" {
		result.Observation = &runtimeports.ProviderAttemptObservationRefV2{Delegation: delegation, PreparedAttemptID: prepared.ID, ProviderOperationRef: "provider-operation-1", Revision: 1, State: state, Digest: digest("observation"), PayloadDigest: candidate.Input.ContentDigest, PayloadRevision: 1, SourceRegistrationID: "provider-source-1", SourceEpoch: 1, SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: digest("ledger-scope"), Sequence: 1, RecordDigest: digest("record")}, ObservedUnixNano: now.Add(time.Second).UnixNano()}
	}
	if err := result.ValidatePrepared(); err != nil {
		t.Fatal(err)
	}
	return result
}

func governedSettledExecutionFixtureV2(t *testing.T, now time.Time, candidate ModelTurnCandidateV2, state SettledTurnResultStateV2) runtimeports.GovernedExecutionAttemptRefsV2 {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	result := governedExecutionFixtureV2(t, now, candidate, runtimeports.ProviderAttemptObservedV2)
	delegation := result.Delegation
	observation := *result.Observation
	disposition := runtimeports.OperationSettlementAppliedV3
	if state == SettledTurnFailedV2 {
		disposition = runtimeports.OperationSettlementFailedV3
	}
	schema := SettledTurnResultSchemaV2()
	result.Settlement = &runtimeports.OperationSettlementRefV3{
		ID: "settlement-1", Revision: 1, Digest: digest("settlement"),
		Attempt: runtimeports.OperationDispatchAttemptRefV3{
			OperationDigest: result.Admission.OperationDigest, EffectID: result.Admission.EffectID,
			IntentRevision: result.Admission.IntentRevision, IntentDigest: result.Admission.IntentDigest,
			PermitID: result.PermitID, PermitRevision: result.PermitRevision, PermitDigest: result.PermitDigest,
			AttemptID: result.AttemptID, Delegation: &delegation,
		},
		Disposition: disposition,
		Owner:       runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: "custom.model/settlement-owner", ManifestDigest: digest("settlement-owner")},
		Observation: &observation, Evidence: []runtimeports.EvidenceRecordRefV2{observation.Evidence},
		DomainResultSchema: &schema, DomainResultDigest: digest("settled-domain-result"),
	}
	if err := result.ValidatePrepared(); err != nil {
		t.Fatal(err)
	}
	return result
}

func governedContinuationFixtureV2(t *testing.T, kind CandidateKindV2) *ContinuationRefV2 {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	return &ContinuationRefV2{Kind: kind, PendingRef: "pending-1", PendingDigest: digest("pending"), SettlementRef: "settlement-1", SettlementDigest: digest("settlement"), EvidenceRef: "evidence-1", EvidenceDigest: digest("evidence")}
}

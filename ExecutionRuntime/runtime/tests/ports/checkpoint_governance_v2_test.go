package ports_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCheckpointGovernanceV2PolicyCanonicalAndTTL(t *testing.T) {
	now := time.Unix(1_780_100_000, 0).UTC()
	projection := ports.CheckpointBarrierPolicyCurrentProjectionV2{
		Ref:                   ports.CheckpointBarrierPolicyRefV2{ID: "policy-checkpoint", Revision: 1, Digest: checkpointPortsDigest("policy"), SemanticDigest: checkpointPortsDigest("semantic")},
		MaxBarrierTTLUnixNano: int64(time.Minute), MaxReconciliationTTLUnixNano: int64(30 * time.Second),
		UnknownAtDeadlineMode:    ports.CheckpointUnknownAtDeadlineIndeterminateV2,
		AbsoluteNotAfterUnixNano: now.Add(2 * time.Minute).UnixNano(), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(90 * time.Second).UnixNano(),
	}
	sealed, err := ports.SealCheckpointBarrierPolicyCurrentProjectionV2(projection, now)
	if err != nil {
		t.Fatal(err)
	}
	resealed, err := ports.SealCheckpointBarrierPolicyCurrentProjectionV2(sealed, now)
	if err != nil || resealed.ProjectionDigest != sealed.ProjectionDigest {
		t.Fatalf("checkpoint Policy Seal is not deterministic: %v", err)
	}
	stale := sealed
	stale.MaxBarrierTTLUnixNano++
	if err := stale.Validate(now); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("re-sealed Policy field drift must be rejected: %v", err)
	}
	if err := sealed.Validate(time.Unix(0, sealed.ExpiresUnixNano)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL equality must be expired: %v", err)
	}
}

func TestCheckpointGovernanceV2TerminalProjectionRejectsSidecarTypePun(t *testing.T) {
	projection := ports.CheckpointAttemptTerminalCurrentProjectionV2{ContractVersion: ports.CheckpointGovernanceContractVersionV2, TerminalState: ports.CheckpointAttemptConsistentV2, CheckedUnixNano: 1, ProjectionDigest: checkpointPortsDigest("placeholder")}
	if err := projection.Validate(); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("incomplete terminal projection must fail closed: %v", err)
	}
}

func TestCheckpointParticipantReservationV2P14RejectsNonPreparedSuccessor(t *testing.T) {
	request := ports.ReserveCheckpointParticipantPhaseRequestV2{Phase: ports.CheckpointPhaseCommitV2}
	if err := request.Validate(); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("commit without exact prepared PreviousPhase must be rejected: %v", err)
	}
}

func TestCheckpointClosureV2ExactEqualityIncludesOrderedClassificationSets(t *testing.T) {
	classification := ports.CheckpointFinalizationClassificationSetV2{
		Entries: []ports.CheckpointFinalizationClassificationEntryV2{{ID: "diagnostic-a", Kind: "praxis.runtime/diagnostic", Classification: ports.CheckpointClassificationUnknownV2, SourceRevision: 1, SourceDigest: checkpointPortsDigest("diagnostic-a")}},
		Digest:  checkpointPortsDigest("classification-set"),
	}
	diagnostics := ports.CheckpointDiagnosticsFinalizationSealRefV2{
		ID: "diagnostics-seal", Revision: 1, Attempt: ports.CheckpointAttemptRefV2{TenantID: "tenant-a", ID: "attempt-a", Revision: 1, Digest: checkpointPortsDigest("attempt")},
		FinalizationCut: ports.CheckpointFinalizationCutRefV2{ID: "cut-a", Revision: 1, Attempt: ports.CheckpointAttemptRefV2{TenantID: "tenant-a", ID: "attempt-a", Revision: 1, Digest: checkpointPortsDigest("attempt")}, Digest: checkpointPortsDigest("cut")},
		Owner:           ports.ProviderBindingRefV2{ComponentID: "diagnostics-owner"},
		SourceEpoch:     1, SourceSequence: 1, LedgerRootDigest: checkpointPortsDigest("ledger"), CompleteSet: ports.CheckpointDiagnosticSetRefV2{AttemptID: "attempt-a", Revision: 1, Count: 1, SetDigest: checkpointPortsDigest("complete-digest")}, CompleteSetDigest: checkpointPortsDigest("complete-digest"), Classifications: classification, Digest: checkpointPortsDigest("seal"),
	}
	if !ports.SameCheckpointDiagnosticsFinalizationSealRefV2(diagnostics, diagnostics) {
		t.Fatal("identical diagnostics seals must compare exact")
	}
	drifted := diagnostics
	drifted.Classifications.Entries = append([]ports.CheckpointFinalizationClassificationEntryV2{}, diagnostics.Classifications.Entries...)
	drifted.Classifications.Entries[0].Classification = ports.CheckpointClassificationIncompleteV2
	if ports.SameCheckpointDiagnosticsFinalizationSealRefV2(diagnostics, drifted) {
		t.Fatal("single classification field drift must be rejected")
	}
	residuals := ports.CheckpointResidualsFinalizationSealRefV2{
		ID: "residuals-seal", Revision: 1, Attempt: diagnostics.Attempt, FinalizationCut: diagnostics.FinalizationCut,
		Owner:       ports.ProviderBindingRefV2{ComponentID: "residuals-owner"},
		SourceEpoch: 1, SourceSequence: 1, LedgerRootDigest: checkpointPortsDigest("residual-ledger"), CompleteSet: ports.CheckpointResidualSetRefV2{AttemptID: "attempt-a", Revision: 1, Count: 1, SetDigest: checkpointPortsDigest("residual-complete-digest")}, CompleteSetDigest: checkpointPortsDigest("residual-complete-digest"), Classifications: classification, Digest: checkpointPortsDigest("residual-seal"),
	}
	if !ports.SameCheckpointResidualsFinalizationSealRefV2(residuals, residuals) {
		t.Fatal("identical residual seals must compare exact")
	}
	residualDrift := residuals
	residualDrift.SourceSequence++
	if ports.SameCheckpointResidualsFinalizationSealRefV2(residuals, residualDrift) {
		t.Fatal("single residual seal field drift must be rejected")
	}
	closure := ports.CheckpointFinalizationInputClosureRefV2{ID: "closure-a", Revision: 1, Attempt: diagnostics.Attempt, FinalizationCut: diagnostics.FinalizationCut, DiagnosticsSeal: diagnostics, ResidualsSeal: residuals, Digest: checkpointPortsDigest("closure")}
	if !ports.SameCheckpointFinalizationInputClosureRefV2(closure, closure) {
		t.Fatal("identical finalization closures must compare exact")
	}
	closureDrift := closure
	closureDrift.DiagnosticsSeal = drifted
	if ports.SameCheckpointFinalizationInputClosureRefV2(closure, closureDrift) {
		t.Fatal("nested seal drift must be rejected by closure equality")
	}
}

func TestCheckpointGovernanceV2MutationRequestsCannotCarryCallerSignedOwnerFacts(t *testing.T) {
	freeze := reflect.TypeOf(ports.FreezeCheckpointEffectCutRequestV2{})
	if _, exists := freeze.FieldByName("Entries"); exists {
		t.Fatal("Effect Cut request must not accept caller-signed Effect entries")
	}
	consistency := reflect.TypeOf(ports.CommitCheckpointConsistencyRequestV2{})
	if _, exists := consistency.FieldByName("ParticipantClosures"); exists {
		t.Fatal("Consistency request must not accept caller-signed Participant closures")
	}
	if _, exists := consistency.FieldByName("Closures"); exists {
		t.Fatal("Consistency request must not accept caller-signed Participant closures")
	}
}

func TestCheckpointEffectCutTerminalV2ClosedTaggedUnionAndDisposition(t *testing.T) {
	attempt := ports.OperationDispatchAttemptRefV3{
		OperationDigest: checkpointPortsDigest("operation-terminal"),
		EffectID:        "effect-terminal",
		IntentRevision:  1,
		IntentDigest:    checkpointPortsDigest("intent-terminal"),
		PermitID:        "permit-terminal",
		PermitRevision:  1,
		PermitDigest:    checkpointPortsDigest("permit-terminal"),
		AttemptID:       "attempt-terminal",
	}
	unknown := ports.EffectCutEntryV2{
		EffectID:       attempt.EffectID,
		IntentRevision: attempt.IntentRevision,
		IntentDigest:   attempt.IntentDigest,
		Attempt:        attempt,
		Phase:          "checkpoint-prepare",
		Disposition:    ports.EffectCutUnknownV2,
		Terminal:       ports.RuntimeOperationTerminalRefV2{Kind: ports.RuntimeTerminalDispatchUnknownV3V2, DispatchUnknownV3: &attempt},
	}
	if err := unknown.Validate(); err != nil {
		t.Fatalf("closed unknown terminal must validate: %v", err)
	}
	unknownKind := unknown
	unknownKind.Terminal.Kind = ports.RuntimeOperationTerminalKindV2("custom/dispatch-unknown")
	if err := unknownKind.Validate(); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("free-form terminal kind must fail closed: %v", err)
	}
	mixed := unknown
	policy := ports.CheckpointBarrierPolicyRefV2{ID: "policy-terminal", Revision: 1, Digest: checkpointPortsDigest("policy-terminal"), SemanticDigest: checkpointPortsDigest("policy-terminal-semantic")}
	exclusion := ports.CheckpointEffectPolicyExclusionRefV2{ID: "exclusion-terminal", Revision: 1, EffectID: attempt.EffectID, Attempt: attempt, Policy: policy}
	exclusion.Digest, _ = exclusion.DigestV2()
	mixed.Terminal.PolicyExclusion = &exclusion
	if err := mixed.Validate(); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("mixed tagged terminal must fail closed: %v", err)
	}
	dispositionDrift := unknown
	dispositionDrift.Disposition = ports.EffectCutSettledV2
	if err := dispositionDrift.Validate(); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("terminal/disposition mismatch must fail closed: %v", err)
	}
	excluded := unknown
	excluded.Disposition = ports.EffectCutExcludedByPolicyV2
	excluded.Terminal = ports.RuntimeOperationTerminalRefV2{Kind: ports.RuntimeTerminalPolicyExclusionV2, PolicyExclusion: &exclusion}
	if err := excluded.Validate(); err != nil {
		t.Fatalf("exact policy exclusion terminal must validate: %v", err)
	}
	for name, mutate := range map[string]func(*ports.EffectCutEntryV2){
		"effect-id":       func(value *ports.EffectCutEntryV2) { value.EffectID = "effect-spliced" },
		"intent-revision": func(value *ports.EffectCutEntryV2) { value.IntentRevision++ },
		"intent-digest":   func(value *ports.EffectCutEntryV2) { value.IntentDigest = checkpointPortsDigest("intent-spliced") },
	} {
		t.Run("attempt-splice-"+name, func(t *testing.T) {
			spliced := unknown
			mutate(&spliced)
			if err := spliced.Validate(); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("outer Effect identity detached from exact Attempt: %v", err)
			}
		})
	}
	failed := unknown
	failed.Disposition = ports.EffectCutEntryDispositionV2("confirmed_failed")
	if err := failed.Validate(); err == nil {
		t.Fatal("removed failed disposition must remain outside the frozen Effect Cut union")
	}
	v5 := unknown
	v5.Disposition = ports.EffectCutSettledV2
	v5.Terminal = ports.RuntimeOperationTerminalRefV2{Kind: ports.RuntimeTerminalCheckpointSettlementV5V2, CheckpointSettlementV5: &ports.OperationCheckpointRestoreSettlementRefV5{ID: "checkpoint-settlement-terminal", Revision: 1, TenantID: "tenant-terminal", EffectID: attempt.EffectID, Attempt: ports.CheckpointAttemptRefV2{ID: "checkpoint-attempt-terminal", Revision: 1, TenantID: "tenant-terminal", Digest: checkpointPortsDigest("checkpoint-attempt-terminal")}, Phase: ports.CheckpointPhasePrepareV2, OperationDigest: attempt.OperationDigest, Digest: checkpointPortsDigest("checkpoint-settlement-terminal")}}
	if err := v5.Validate(); !core.HasCategory(err, core.ErrorForbidden) {
		t.Fatalf("V5 terminal without an Owner-derived exact dispatch Attempt proof must remain unsupported: %v", err)
	}
}

func FuzzCheckpointBarrierPolicyCanonicalV2(f *testing.F) {
	f.Add(int64(time.Minute), int64(30*time.Second), int64(90*time.Second))
	f.Add(int64(0), int64(time.Second), int64(time.Minute))
	f.Add(int64(-1), int64(-1), int64(-1))
	f.Fuzz(func(t *testing.T, barrierTTL, reconciliationTTL, projectionTTL int64) {
		now := time.Unix(1_780_400_000, 0).UTC()
		projection := ports.CheckpointBarrierPolicyCurrentProjectionV2{
			Ref:                   ports.CheckpointBarrierPolicyRefV2{ID: "policy-fuzz", Revision: 1, Digest: checkpointPortsDigest("policy-fuzz"), SemanticDigest: checkpointPortsDigest("semantic-fuzz")},
			MaxBarrierTTLUnixNano: barrierTTL, MaxReconciliationTTLUnixNano: reconciliationTTL,
			UnknownAtDeadlineMode:    ports.CheckpointUnknownAtDeadlineIndeterminateV2,
			AbsoluteNotAfterUnixNano: now.Add(2 * time.Minute).UnixNano(), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.UnixNano() + projectionTTL,
		}
		sealed, err := ports.SealCheckpointBarrierPolicyCurrentProjectionV2(projection, now)
		if err != nil {
			return
		}
		if err := sealed.Validate(now); err != nil {
			t.Fatalf("Seal returned an invalid projection: %v", err)
		}
		resealed, err := ports.SealCheckpointBarrierPolicyCurrentProjectionV2(sealed, now)
		if err != nil || resealed.ProjectionDigest != sealed.ProjectionDigest {
			t.Fatalf("canonical Seal changed digest: %v", err)
		}
	})
}

func checkpointPortsDigest(value string) core.Digest { return core.DigestBytes([]byte(value)) }

package runtimeadapter

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	continuityports "github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	runtimetestsupport "github.com/Proview-China/rax/ExecutionRuntime/runtime/tests/testsupport"
)

func TestCheckpointManifestSealReaderV2ProjectsExactRuntimeClosure(t *testing.T) {
	fixture := newCheckpointManifestAdapterFixtureV2(t)
	projection, err := fixture.adapter.InspectCheckpointManifestSealV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Ref != fixture.request.Ref || projection.ParticipantSetDigest != fixture.request.ExpectedParticipantSetDigest || len(projection.ParticipantClosures) != 1 || projection.ParticipantClosures[0].Digest != fixture.closure.Digest {
		t.Fatalf("Runtime projection lost exact bindings: %+v", projection)
	}
	if fixture.reader.calls != 1 || fixture.reader.last.Ref.Exact() != fixture.seal.Ref().Exact() {
		t.Fatalf("adapter did not use one exact Continuity lookup: reader=%+v", fixture.reader)
	}
}

func TestCheckpointManifestSealReaderV2RejectsBindingDrift(t *testing.T) {
	tests := map[string]func(*checkpointManifestAdapterFixtureV2){
		"owner": func(f *checkpointManifestAdapterFixtureV2) {
			f.request.Ref.ExactLookup.Owner.ArtifactDigest += "-drift"
		},
		"scope": func(f *checkpointManifestAdapterFixtureV2) {
			f.request.Ref.ExactLookup.ScopeDigest = string(runtimeDigestCheckpointV2("other-scope"))
		},
		"manifest": func(f *checkpointManifestAdapterFixtureV2) {
			f.request.Ref.ManifestDigest = runtimeDigestCheckpointV2("other-manifest")
		},
		"participant_set": func(f *checkpointManifestAdapterFixtureV2) {
			f.request.ExpectedParticipantSetDigest = runtimeDigestCheckpointV2("other-set")
		},
		"participant_owner": func(f *checkpointManifestAdapterFixtureV2) {
			f.request.ExpectedParticipantClosures[0].Participant.Owner.ArtifactDigest = runtimeDigestCheckpointV2("other-artifact")
			refreshRuntimeClosureV2(t, &f.request.ExpectedParticipantClosures[0])
		},
		"participant_digest": func(f *checkpointManifestAdapterFixtureV2) {
			f.request.ExpectedParticipantClosures[0].Digest = runtimeDigestCheckpointV2("other-closure")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newCheckpointManifestAdapterFixtureV2(t)
			mutate(&fixture)
			if _, err := fixture.adapter.InspectCheckpointManifestSealV2(context.Background(), fixture.request); err == nil {
				t.Fatal("drifted cross-owner binding was accepted")
			}
		})
	}
}

func TestCheckpointManifestSealReaderV2TypedNilAndLostInspectReply(t *testing.T) {
	fixture := newCheckpointManifestAdapterFixtureV2(t)
	var reader *checkpointManifestExactReaderV2
	fixture.adapter.Manifests = reader
	if _, err := fixture.adapter.InspectCheckpointManifestSealV2(context.Background(), fixture.request); !runtimecore.HasCategory(err, runtimecore.ErrorUnavailable) {
		t.Fatalf("typed-nil Reader must fail closed: %v", err)
	}

	fixture = newCheckpointManifestAdapterFixtureV2(t)
	fixture.reader.err = contract.NewError(contract.ErrIndeterminate, "inspect", "injected lost reply")
	if _, err := fixture.adapter.InspectCheckpointManifestSealV2(context.Background(), fixture.request); !runtimecore.HasCategory(err, runtimecore.ErrorIndeterminate) {
		t.Fatalf("lost Inspect reply must remain indeterminate: %v", err)
	}
	fixture.reader.err = nil
	if projection, err := fixture.adapter.InspectCheckpointManifestSealV2(context.Background(), fixture.request); err != nil || projection.Ref != fixture.request.Ref {
		t.Fatalf("recovery must repeat the same exact Inspect: projection=%+v err=%v", projection, err)
	}
}

type checkpointManifestAdapterFixtureV2 struct {
	adapter CheckpointManifestSealReaderV2
	reader  *checkpointManifestExactReaderV2
	request runtimeports.InspectCheckpointManifestSealRequestV2
	seal    contract.CheckpointManifestSealFactV2
	closure runtimeports.CheckpointParticipantClosureRefV2
}

func newCheckpointManifestAdapterFixtureV2(t *testing.T) checkpointManifestAdapterFixtureV2 {
	t.Helper()
	tenant := runtimecore.TenantID("tenant-checkpoint-adapter")
	scopeDigest := runtimeDigestCheckpointV2("scope")
	attempt := runtimeports.CheckpointAttemptRefV2{TenantID: tenant, ID: "attempt-checkpoint-adapter", Revision: 2, Digest: runtimeDigestCheckpointV2("attempt")}
	barrier := runtimeports.CheckpointBarrierLeaseRefV2{TenantID: tenant, ID: "barrier-checkpoint-adapter", AttemptID: attempt.ID, Revision: 1, Digest: runtimeDigestCheckpointV2("barrier"), ExpiresUnixNano: time.Unix(1_800_000_000, 0).UnixNano()}
	cut := runtimeports.EffectCutRefV2{ID: "cut-checkpoint-adapter", Revision: 1, Attempt: attempt, RootDigest: runtimeDigestCheckpointV2("cut-root"), Watermark: 1, Digest: runtimeDigestCheckpointV2("cut")}
	closure := runtimeParticipantClosureV2(t, attempt, barrier, cut)
	setDigest := runtimeDigestCheckpointV2("participant-set")
	externalClosure, err := runtimeports.DeriveCheckpointParticipantClosureExactRefV2(tenant, string(scopeDigest), closure)
	if err != nil {
		t.Fatal(err)
	}

	manifest := testkit.RetargetManifestScopeV2(testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2), string(tenant), string(scopeDigest))
	manifest.ContextGenerationRef.Owner.FactKind = "context_generation_fact_v2"
	manifest.ContextGenerationRef.Owner.Capability = "context-generation-current-v2"
	manifest.ContextGenerationRef.SchemaRef = "praxis.context/context-generation-fact/v2"
	for index := range manifest.ContextFrameRefs {
		manifest.ContextFrameRefs[index].Owner.FactKind = "context_frame_fact_v2"
		manifest.ContextFrameRefs[index].Owner.Capability = "context-frame-reader-v2"
		manifest.ContextFrameRefs[index].SchemaRef = "praxis.context/context-frame-fact/v2"
	}
	manifest.CheckpointAttemptRef = runtimeExactRefV2(manifest.CheckpointAttemptRef, attempt.ID, uint64(attempt.Revision), string(attempt.Digest), string(tenant), string(scopeDigest), "checkpoint_attempt_fact_v2")
	manifest.BarrierRef = runtimeExactRefV2(manifest.BarrierRef, barrier.ID, uint64(barrier.Revision), string(barrier.Digest), string(tenant), string(scopeDigest), "checkpoint_barrier_fact_v2")
	manifest.EffectCutRef = runtimeExactRefV2(manifest.EffectCutRef, cut.ID, uint64(cut.Revision), string(cut.Digest), string(tenant), string(scopeDigest), "checkpoint_effect_cut_fact_v2")
	manifest.ParticipantClosures[0].ParticipantID = closure.Participant.ID
	manifest.ParticipantClosures[0].RuntimeClosureRef = externalExactRefToContinuityV2(externalClosure)
	manifest.RuntimeParticipantSetDigest = string(setDigest)
	testkit.RefreshManifestV2(&manifest)
	seal := testkit.SealV2(manifest)

	sealDigest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(seal.Digest)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(manifest.Digest)
	if err != nil {
		t.Fatal(err)
	}
	frozenDigest, err := runtimeports.NormalizeCheckpointExternalSHA256DigestV2(manifest.FrozenRefSetDigest)
	if err != nil {
		t.Fatal(err)
	}
	sealExact := seal.Ref().Exact()
	ref := runtimeports.CheckpointManifestSealRefV2{
		ExactLookup: runtimeports.CheckpointExternalExactFactRefV2{
			ContractVersion: sealExact.ContractVersion, SchemaRef: sealExact.SchemaRef,
			Owner: runtimeports.CheckpointManifestSealOwnerBindingV2{
				BindingSetID: sealExact.Owner.BindingSetID, BindingRevision: runtimecore.Revision(sealExact.Owner.BindingRevision),
				ComponentID: sealExact.Owner.ComponentID, ManifestDigest: sealExact.Owner.ManifestDigest,
				ArtifactDigest: sealExact.Owner.ArtifactDigest, Capability: sealExact.Owner.Capability, FactKind: sealExact.Owner.FactKind,
			},
			TenantID: sealExact.TenantID, ID: sealExact.ID, Revision: runtimecore.Revision(sealExact.Revision), Digest: sealExact.Digest, ScopeDigest: sealExact.ScopeDigest,
		},
		ID: seal.SealID, Revision: 1, Digest: sealDigest,
		ManifestID: manifest.ManifestID, ManifestRevision: runtimecore.Revision(manifest.Revision), ManifestDigest: manifestDigest,
		Attempt: attempt, Barrier: barrier, EffectCut: cut, FrozenRefSetDigest: frozenDigest,
	}
	request := runtimeports.InspectCheckpointManifestSealRequestV2{Ref: ref, ExpectedParticipantSetDigest: setDigest, ExpectedParticipantClosures: []runtimeports.CheckpointParticipantClosureRefV2{closure}}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	reader := &checkpointManifestExactReaderV2{manifest: manifest, seal: seal}
	return checkpointManifestAdapterFixtureV2{adapter: CheckpointManifestSealReaderV2{Manifests: reader}, reader: reader, request: request, seal: seal, closure: closure}
}

type checkpointManifestExactReaderV2 struct {
	manifest contract.CheckpointManifestFactV2
	seal     contract.CheckpointManifestSealFactV2
	err      error
	calls    int
	last     continuityports.InspectCheckpointManifestSealRequestV2
}

func (r *checkpointManifestExactReaderV2) InspectCheckpointManifestSealV2(_ context.Context, request continuityports.InspectCheckpointManifestSealRequestV2) (contract.CheckpointManifestSealFactV2, error) {
	r.calls++
	r.last = request
	if r.err != nil {
		return contract.CheckpointManifestSealFactV2{}, r.err
	}
	return r.seal.Clone(), nil
}

func (r *checkpointManifestExactReaderV2) InspectCheckpointManifestV2(_ context.Context, request continuityports.InspectCheckpointManifestRequestV2) (contract.CheckpointManifestFactV2, error) {
	if request.Ref != r.manifest.Ref() {
		return contract.CheckpointManifestFactV2{}, contract.NewError(contract.ErrRevisionConflict, "manifest_ref", "test exact Manifest ref drift")
	}
	return r.manifest.Clone(), nil
}

func (r *checkpointManifestExactReaderV2) InspectCurrentCheckpointManifestV2(context.Context, continuityports.InspectCurrentCheckpointManifestRequestV2) (contract.CheckpointManifestFactV2, error) {
	return contract.CheckpointManifestFactV2{}, contract.NewError(contract.ErrUnsupported, "test", "unused")
}

func runtimeExactRefV2(ref contract.ExactFactRefV2, id string, revision uint64, digest, tenant, scope, factKind string) contract.ExactFactRefV2 {
	ref.ContractVersion = runtimeports.CheckpointGovernanceContractVersionV2
	ref.SchemaRef = "praxis.runtime/" + factKind
	ref.Owner.ComponentID = "praxis/runtime"
	ref.Owner.FactKind = factKind
	ref.TenantID, ref.ScopeDigest, ref.ID, ref.Revision, ref.Digest = tenant, scope, id, revision, digest
	return ref
}

func runtimeParticipantClosureV2(t *testing.T, attempt runtimeports.CheckpointAttemptRefV2, barrier runtimeports.CheckpointBarrierLeaseRefV2, cut runtimeports.EffectCutRefV2) runtimeports.CheckpointParticipantClosureRefV2 {
	t.Helper()
	settlement := runtimetestsupport.OperationSettlementSubmissionV4()
	operation := settlement.Operation
	operationDigest := settlement.OperationDigest
	participant := runtimeports.CheckpointParticipantRefV2{ID: "participant-checkpoint-adapter", Owner: settlement.DomainResult.Owner, Digest: runtimeDigestCheckpointV2("participant")}
	reservation := runtimeports.CheckpointParticipantPhaseReservationRefV2{ID: "reservation-checkpoint-adapter", Revision: 1, Digest: runtimeDigestCheckpointV2("reservation"), ExpiresUnixNano: time.Unix(1_800_000_000, 0).UnixNano()}
	phaseFact := runtimeports.CheckpointParticipantPhaseRefV2{ID: "phase-checkpoint-adapter", Revision: 1, Phase: runtimeports.CheckpointPhasePrepareV2, State: runtimeports.CheckpointParticipantPreparedV2, Digest: runtimeDigestCheckpointV2("phase")}
	domain := runtimeports.CheckpointParticipantDomainResultRefV2{ID: "domain-checkpoint-adapter", Revision: 1, Kind: "praxis.sandbox/checkpoint-domain-result", Attempt: attempt, Participant: participant, Phase: runtimeports.CheckpointPhasePrepareV2, Operation: operation, OperationDigest: operationDigest, Digest: runtimeDigestCheckpointV2("domain")}
	scopeDigest := runtimeDigestCheckpointV2("checkpoint-evidence-scope")
	qualification := runtimeports.CheckpointRestoreEvidenceQualificationRefV1{ID: "qualification-checkpoint-adapter", Revision: 1, Attempt: attempt, Barrier: barrier, EffectCut: cut, Reservation: reservation, Phase: runtimeports.CheckpointPhasePrepareV2, ScopeDigest: scopeDigest, ExpiresUnixNano: time.Unix(1_800_000_000, 0).UnixNano()}
	qualification.Digest, _ = qualification.DigestV1()
	handoff := runtimeports.CheckpointRestoreEvidenceProviderHandoffRefV1{ID: "handoff-checkpoint-adapter", Revision: 1, Qualification: qualification, Attempt: settlement.DomainResult.Attempt, Phase: runtimeports.CheckpointPhasePrepareV2, ScopeDigest: scopeDigest}
	handoff.Digest, _ = handoff.DigestV1()
	source := runtimeports.EvidenceSourceKeyV2{RegistrationID: "source-checkpoint-adapter", SourceEpoch: 1, SourceSequence: 1}
	evidence := runtimeports.CheckpointRestoreEvidenceConsumptionRefV1{ID: "evidence-checkpoint-adapter", Revision: 1, Qualification: qualification, Handoff: handoff, Record: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: runtimeDigestCheckpointV2("ledger"), Sequence: 1, RecordDigest: runtimeDigestCheckpointV2("record")}, Attempt: attempt, Phase: runtimeports.CheckpointPhasePrepareV2, State: runtimeports.CheckpointEvidenceConsumedCurrentV1, ScopeDigest: scopeDigest, Source: source}
	evidence.Digest, _ = evidence.DigestV1()
	settlementRef := runtimeports.OperationCheckpointRestoreSettlementRefV5{ID: "settlement-checkpoint-adapter", Revision: 1, TenantID: attempt.TenantID, EffectID: handoff.Attempt.EffectID, Attempt: attempt, Phase: runtimeports.CheckpointPhasePrepareV2, OperationDigest: operationDigest, Digest: runtimeDigestCheckpointV2("settlement")}
	apply := runtimeports.CheckpointParticipantApplySettlementRefV2{ID: "apply-checkpoint-adapter", Revision: 1, Participant: participant, Phase: runtimeports.CheckpointPhasePrepareV2, SettlementID: settlementRef.ID, Digest: runtimeDigestCheckpointV2("apply")}
	prepare := runtimeports.CheckpointParticipantPhaseClosureRefV2{ID: "prepare-closure-checkpoint-adapter", Phase: runtimeports.CheckpointPhasePrepareV2, Reservation: reservation, PhaseFact: phaseFact, DomainResult: domain, Evidence: evidence, Settlement: settlementRef, ApplySettlement: apply}
	prepare.Digest, _ = prepare.DigestV2()
	closure := runtimeports.CheckpointParticipantClosureRefV2{ID: "closure-checkpoint-adapter", Participant: participant, Prepare: prepare}
	refreshRuntimeClosureV2(t, &closure)
	if err := closure.Validate(); err != nil {
		t.Fatal(err)
	}
	return closure
}

func refreshRuntimeClosureV2(t *testing.T, closure *runtimeports.CheckpointParticipantClosureRefV2) {
	t.Helper()
	closure.Digest = ""
	digest, err := closure.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	closure.Digest = digest
}

func runtimeDigestCheckpointV2(value string) runtimecore.Digest {
	return runtimecore.DigestBytes([]byte(value))
}

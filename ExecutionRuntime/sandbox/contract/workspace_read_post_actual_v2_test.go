package contract_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

func TestWorkspaceReadExecutionQualificationV2ExactIdentityAndHistoricalShape(t *testing.T) {
	qualification := postActualQualificationV2(t)
	if err := qualification.Validate(); err != nil {
		t.Fatal(err)
	}
	other := qualification
	other.CurrentQueryDigest = postActualDigestV2(t, "other-query")
	other, err := contract.SealWorkspaceReadExecutionQualificationV2(other)
	if err != nil {
		t.Fatal(err)
	}
	if other.Ref.ID != qualification.Ref.ID || other.Ref.Digest == qualification.Ref.Digest {
		t.Fatal("same origin must keep one stable identity while a different closure changes the exact digest")
	}

	// Historical validation deliberately does not require a wall clock and
	// remains valid after the former execution authority expired.
	historicalNow := time.Unix(0, qualification.ExpiresUnixNano).Add(24 * time.Hour)
	if !historicalNow.After(time.Unix(0, qualification.ExpiresUnixNano)) {
		t.Fatal("fixture did not cross the qualification expiry")
	}
	if err = qualification.Validate(); err != nil {
		t.Fatalf("expired historical qualification became unreadable: %v", err)
	}
}

func TestWorkspaceReadExecutionQualificationV2RejectsAdmissionAndAttemptSplice(t *testing.T) {
	base := postActualQualificationV2(t)
	tests := map[string]func(contract.WorkspaceReadExecutionQualificationV2) contract.WorkspaceReadExecutionQualificationV2{
		"binding digest": func(value contract.WorkspaceReadExecutionQualificationV2) contract.WorkspaceReadExecutionQualificationV2 {
			value.AdmissionAttemptBindingDigest = ""
			return value
		},
		"admission id": func(value contract.WorkspaceReadExecutionQualificationV2) contract.WorkspaceReadExecutionQualificationV2 {
			value.AdmissionReceipt.ID = "another-admission"
			return value
		},
		"runtime attempt digest": func(value contract.WorkspaceReadExecutionQualificationV2) contract.WorkspaceReadExecutionQualificationV2 {
			value.RuntimeAttemptDigest = runtimecore.DigestBytes([]byte("another-attempt"))
			return value
		},
		"owner current": func(value contract.WorkspaceReadExecutionQualificationV2) contract.WorkspaceReadExecutionQualificationV2 {
			value.CommandOwnerCurrent.Digest = postActualDigestV2(t, "other-owner-current")
			return value
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := mutate(base)
			if err := value.Validate(); err == nil {
				t.Fatal("spliced qualification was accepted")
			}
		})
	}
}

func TestWorkspaceReadPhysicalJournalLookupV2DerivesOnlyHistoricalCoordinates(t *testing.T) {
	qualification := postActualQualificationV2(t)
	lookup, err := contract.BuildWorkspaceReadPhysicalJournalLookupV2(qualification)
	if err != nil {
		t.Fatal(err)
	}
	if lookup.RuntimeAttemptID != qualification.RuntimeAttempt.AttemptID ||
		lookup.RequestDigest != qualification.ActualRequestDigest ||
		lookup.PayloadDigest != qualification.PayloadDigest ||
		lookup.Phase != contract.WorkspaceReadPhysicalJournalExecuteV2 {
		t.Fatal("physical journal lookup drifted from its exact Qualification")
	}
	journal := postActualIndeterminateTerminalV2(t, qualification, contract.WorkspaceReadPhysicalJournalStartedV2).Journal
	if err = journal.ValidateLookupV2(lookup); err != nil {
		t.Fatal(err)
	}
	lookup.RequestDigest = postActualDigestV2(t, "another-request")
	if err = journal.ValidateLookupV2(lookup); err == nil {
		t.Fatal("journal accepted a spliced lookup")
	}
}

func TestWorkspaceReadTerminalV2ObservedAndIndeterminateShareOneIdentity(t *testing.T) {
	qualification := postActualQualificationV2(t)
	observed := postActualObservedTerminalV2(t, qualification)
	unknown := postActualIndeterminateTerminalV2(t, qualification, contract.WorkspaceReadPhysicalJournalCompletedV2)
	if observed.Ref.ID != unknown.Ref.ID || observed.Ref.Digest == unknown.Ref.Digest {
		t.Fatal("terminal outcomes must compete on one origin identity with different exact bodies")
	}
	if observed.RecordedUnixNano <= qualification.ExpiresUnixNano {
		t.Fatal("fixture did not prove post-expiry terminal persistence")
	}
	if err := observed.Validate(); err != nil {
		t.Fatalf("post-expiry observed history was rejected: %v", err)
	}
	if err := unknown.Validate(); err != nil {
		t.Fatalf("completed-before-S2 indeterminate history was rejected: %v", err)
	}
}

func TestWorkspaceReadTerminalV2OutcomeClosureRejectsSplice(t *testing.T) {
	qualification := postActualQualificationV2(t)
	base := postActualObservedTerminalV2(t, qualification)
	tests := map[string]func(contract.WorkspaceReadTerminalFactV2) contract.WorkspaceReadTerminalFactV2{
		"started observed": func(value contract.WorkspaceReadTerminalFactV2) contract.WorkspaceReadTerminalFactV2 {
			value.Journal.State = contract.WorkspaceReadPhysicalJournalStartedV2
			value.Journal.Revision = 1
			return value
		},
		"journal phase": func(value contract.WorkspaceReadTerminalFactV2) contract.WorkspaceReadTerminalFactV2 {
			value.Journal.Phase = "prepare"
			return value
		},
		"journal revision": func(value contract.WorkspaceReadTerminalFactV2) contract.WorkspaceReadTerminalFactV2 {
			value.Journal.Revision = 3
			return value
		},
		"provider observation": func(value contract.WorkspaceReadTerminalFactV2) contract.WorkspaceReadTerminalFactV2 {
			copy := *value.Observed
			copy.S2Proof.ProviderReceipt.ObservationDigest = postActualDigestV2(t, "other-provider-observation")
			value.Observed = &copy
			return value
		},
		"qualification origin": func(value contract.WorkspaceReadTerminalFactV2) contract.WorkspaceReadTerminalFactV2 {
			value.OriginAttempt.ID = "another-origin"
			return value
		},
		"runtime attempt": func(value contract.WorkspaceReadTerminalFactV2) contract.WorkspaceReadTerminalFactV2 {
			value.Journal.AttemptID = "another-runtime-attempt"
			return value
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := mutate(base)
			if _, err := contract.SealWorkspaceReadTerminalFactV2(value); err == nil {
				t.Fatal("spliced terminal was accepted")
			}
		})
	}
}

func TestWorkspaceReadOutcomeS2ProofV2RejectsEveryQualificationAxisDrift(t *testing.T) {
	qualification := postActualQualificationV2(t)
	base := postActualObservedTerminalV2(t, qualification).Observed.S2Proof
	tests := map[string]func(contract.WorkspaceReadOutcomeS2ProofV2) contract.WorkspaceReadOutcomeS2ProofV2{
		"qualification": func(value contract.WorkspaceReadOutcomeS2ProofV2) contract.WorkspaceReadOutcomeS2ProofV2 {
			value.Qualification.Digest = postActualDigestV2(t, "other-qualification")
			return value
		},
		"origin": func(value contract.WorkspaceReadOutcomeS2ProofV2) contract.WorkspaceReadOutcomeS2ProofV2 {
			value.OriginAttempt.Digest = postActualDigestV2(t, "other-origin")
			return value
		},
		"association": func(value contract.WorkspaceReadOutcomeS2ProofV2) contract.WorkspaceReadOutcomeS2ProofV2 {
			value.Association.Digest = runtimecore.DigestBytes([]byte("other-association"))
			return value
		},
		"publication": func(value contract.WorkspaceReadOutcomeS2ProofV2) contract.WorkspaceReadOutcomeS2ProofV2 {
			value.CommandPublication.Digest = postActualDigestV2(t, "other-publication")
			return value
		},
		"owner current": func(value contract.WorkspaceReadOutcomeS2ProofV2) contract.WorkspaceReadOutcomeS2ProofV2 {
			value.CommandOwnerCurrent.Digest = postActualDigestV2(t, "other-owner-current")
			return value
		},
		"workspace view": func(value contract.WorkspaceReadOutcomeS2ProofV2) contract.WorkspaceReadOutcomeS2ProofV2 {
			value.WorkspaceView.Digest = postActualDigestV2(t, "other-workspace")
			return value
		},
		"workspace lease": func(value contract.WorkspaceReadOutcomeS2ProofV2) contract.WorkspaceReadOutcomeS2ProofV2 {
			value.WorkspaceLeaseDigest = postActualDigestV2(t, "other-workspace-lease")
			return value
		},
		"runtime current": func(value contract.WorkspaceReadOutcomeS2ProofV2) contract.WorkspaceReadOutcomeS2ProofV2 {
			value.RuntimeCurrentDigest = runtimecore.Digest("sha256:" + postActualDigestV2(t, "other-runtime-current"))
			return value
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := mutate(base)
			sealed, err := contract.SealWorkspaceReadOutcomeS2ProofV2(value)
			if err != nil {
				t.Fatal(err)
			}
			if err = sealed.ValidateQualificationV2(qualification); err == nil {
				t.Fatal("S2 proof drifted from Qualification S1")
			}
		})
	}
}

func TestWorkspaceReadIndeterminateEvidenceV2DerivesStageFromJournalAndErrorClass(t *testing.T) {
	qualification := postActualQualificationV2(t)
	started := postActualIndeterminateTerminalV2(t, qualification, contract.WorkspaceReadPhysicalJournalStartedV2)
	journal := started.Journal
	if _, err := contract.SealWorkspaceReadIndeterminateEvidenceV2(
		qualification.Ref,
		qualification.OriginAttempt,
		journal,
		contract.WorkspaceReadIndeterminateErrorS2UnavailableV2,
		postActualDigestV2(t, "error"),
		journal.RecordedUnixNano,
	); err == nil {
		t.Fatal("started journal accepted an S2-only error class")
	}

	completed := postActualObservedTerminalV2(t, qualification).Journal
	evidence, err := contract.SealWorkspaceReadIndeterminateEvidenceV2(
		qualification.Ref,
		qualification.OriginAttempt,
		completed,
		contract.WorkspaceReadIndeterminateErrorS2ExpiredV2,
		postActualDigestV2(t, "expired-error"),
		completed.RecordedUnixNano,
	)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Stage != contract.WorkspaceReadIndeterminateOutcomeS2ExpiredV2 {
		t.Fatalf("completed S2 expiry derived %q", evidence.Stage)
	}
	tampered := evidence
	tampered.Stage = contract.WorkspaceReadIndeterminateRecoveryIndeterminateV2
	if err = tampered.Validate(); err == nil {
		t.Fatal("caller-selected indeterminate stage was accepted")
	}
}

func TestWorkspaceReadIndeterminateEvidenceV2RejectsQualificationOriginAndClockSplice(t *testing.T) {
	qualification := postActualQualificationV2(t)
	base := postActualIndeterminateTerminalV2(t, qualification, contract.WorkspaceReadPhysicalJournalStartedV2).Indeterminate.Evidence
	tests := map[string]func(contract.WorkspaceReadIndeterminateEvidenceV2) contract.WorkspaceReadIndeterminateEvidenceV2{
		"qualification": func(value contract.WorkspaceReadIndeterminateEvidenceV2) contract.WorkspaceReadIndeterminateEvidenceV2 {
			value.Qualification.Digest = postActualDigestV2(t, "other-qualification")
			return value
		},
		"origin": func(value contract.WorkspaceReadIndeterminateEvidenceV2) contract.WorkspaceReadIndeterminateEvidenceV2 {
			value.OriginAttempt.Digest = postActualDigestV2(t, "other-origin")
			return value
		},
		"checked before journal": func(value contract.WorkspaceReadIndeterminateEvidenceV2) contract.WorkspaceReadIndeterminateEvidenceV2 {
			value.CheckedUnixNano = value.Journal.RecordedUnixNano - 1
			return value
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			if err := mutate(base).Validate(); err == nil {
				t.Fatal("spliced indeterminate evidence was accepted")
			}
		})
	}
}

func TestWorkspaceReadIndeterminateTerminalV2CarriesNoContent(t *testing.T) {
	qualification := postActualQualificationV2(t)
	unknown := postActualIndeterminateTerminalV2(t, qualification, contract.WorkspaceReadPhysicalJournalStartedV2)
	payload, err := json.Marshal(unknown)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"content", "artifact", "raw_response", "secret"} {
		if strings.Contains(strings.ToLower(string(payload)), forbidden) {
			t.Fatalf("indeterminate terminal leaked forbidden field %q: %s", forbidden, payload)
		}
	}
	if unknown.Observed != nil || unknown.Indeterminate == nil {
		t.Fatal("indeterminate terminal sidecars drifted")
	}
}

func postActualQualificationV2(t *testing.T) contract.WorkspaceReadExecutionQualificationV2 {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	expires := now.Add(time.Minute)
	stable := runtimecore.DigestBytes([]byte("post-actual-stable"))
	runtimeAdmission, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(
		runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
			ID: "admission-post-actual", Revision: 1, StableKeyDigest: stable, Admitted: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	attempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: runtimecore.DigestBytes([]byte("operation")),
		EffectID:        runtimecore.EffectIntentID("effect-post-actual"), IntentRevision: 1,
		IntentDigest: runtimecore.DigestBytes([]byte("intent")), PermitID: "permit-post-actual",
		PermitRevision: 1, PermitDigest: runtimecore.DigestBytes([]byte("permit")),
		AttemptID: "runtime-attempt-post-actual",
	}
	if err = attempt.Validate(); err != nil {
		t.Fatal(err)
	}
	attemptDigest, err := contract.WorkspaceReadSourceRuntimeAttemptDigestV2(attempt)
	if err != nil {
		t.Fatal(err)
	}
	digest := func(value string) string { return postActualDigestV2(t, value) }
	value := contract.WorkspaceReadExecutionQualificationV2{
		OriginAttempt: contract.WorkspaceReadAttemptRefV1{ID: "workspace-read-attempt-origin", Revision: 1, Digest: digest("origin")},
		Reservation:   contract.Ref{ID: "reservation", Revision: 1, Digest: digest("reservation")},
		AdmissionReceipt: contract.WorkspaceReadReceiptBindingV1{
			ID: runtimeAdmission.ID, Revision: uint64(runtimeAdmission.Revision), Digest: string(runtimeAdmission.Digest),
			StableKeyDigest: string(stable), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
		},
		RuntimeAdmissionReceipt:       runtimeAdmission,
		AdmissionAttemptBindingDigest: digest("admission-attempt-binding"),
		RuntimeAttempt:                attempt,
		RuntimeAttemptDigest:          attemptDigest,
		AuthorizationDigest:           runtimecore.DigestBytes([]byte("authorization")),
		Association:                   runtimeports.PreparedDomainCommandAssociationRefV1{ID: "association-post-actual", Revision: 1, Digest: runtimecore.DigestBytes([]byte("association"))},
		Command:                       contract.Ref{ID: "command", Revision: 1, Digest: digest("command")},
		CommandPublication:            contract.Ref{ID: "publication", Revision: 1, Digest: digest("publication")},
		CommandOwnerCurrent:           contract.Ref{ID: "owner-current", Revision: 1, Digest: digest("owner-current")},
		WorkspaceView:                 contract.Ref{ID: "workspace", Revision: 1, Digest: digest("workspace")},
		WorkspaceLeaseDigest:          digest("workspace-lease"),
		CurrentQueryDigest:            digest("current-query"),
		ExpectedRuntimeCurrentDigest:  runtimecore.DigestBytes([]byte("runtime-current")),
		ActualRequestDigest:           digest("actual-request"),
		PayloadDigest:                 digest("payload"),
		S1CheckedUnixNano:             now.UnixNano(),
		ExpiresUnixNano:               expires.UnixNano(),
	}
	sealed, err := contract.SealWorkspaceReadExecutionQualificationV2(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func postActualObservedTerminalV2(t *testing.T, qualification contract.WorkspaceReadExecutionQualificationV2) contract.WorkspaceReadTerminalFactV2 {
	t.Helper()
	journalTime := time.Unix(0, qualification.ExpiresUnixNano).Add(time.Second)
	observationDigest := postActualDigestV2(t, "sandbox-observation")
	providerObservationDigest := postActualDigestV2(t, "provider-observation")
	journal := contract.WorkspaceReadPhysicalJournalRefV2{
		AttemptID: qualification.RuntimeAttempt.AttemptID, RequestDigest: qualification.ActualRequestDigest,
		PayloadDigest: qualification.PayloadDigest, Phase: contract.WorkspaceReadPhysicalJournalExecuteV2,
		State: contract.WorkspaceReadPhysicalJournalCompletedV2, Revision: 2,
		RecordedUnixNano: journalTime.UnixNano(), RecordDigest: postActualDigestV2(t, "journal-completed"),
	}
	checked := journalTime.Add(time.Second)
	recorded := checked.Add(time.Second)
	proof, err := contract.SealWorkspaceReadOutcomeS2ProofV2(contract.WorkspaceReadOutcomeS2ProofV2{
		Qualification: qualification.Ref, OriginAttempt: qualification.OriginAttempt,
		Association: qualification.Association, CommandPublication: qualification.CommandPublication,
		CommandOwnerCurrent: qualification.CommandOwnerCurrent, WorkspaceView: qualification.WorkspaceView,
		WorkspaceLeaseDigest: qualification.WorkspaceLeaseDigest,
		RuntimeCurrentDigest: qualification.ExpectedRuntimeCurrentDigest, Journal: journal,
		Observation: contract.Ref{ID: "provider-observation", Revision: 1, Digest: observationDigest},
		ProviderReceipt: contract.WorkspaceReadReceiptBindingV1{
			ID: "provider-receipt", Revision: 1, Digest: postActualDigestV2(t, "provider-receipt"),
			ObservationDigest: providerObservationDigest, StableKeyDigest: qualification.AdmissionReceipt.StableKeyDigest,
			CheckedUnixNano: journalTime.UnixNano(), ExpiresUnixNano: recorded.Add(time.Minute).UnixNano(),
		},
		S2CheckedUnixNano: checked.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	value := contract.WorkspaceReadTerminalFactV2{
		Qualification: qualification.Ref, OriginAttempt: qualification.OriginAttempt,
		RuntimeAttempt: qualification.RuntimeAttempt, RuntimeAttemptDigest: qualification.RuntimeAttemptDigest,
		ActualRequestDigest: qualification.ActualRequestDigest, Journal: journal,
		Outcome:                contract.WorkspaceReadTerminalObservedV2,
		Observed:               &contract.WorkspaceReadObservedTerminalV2{S2Proof: proof},
		OutcomeCheckedUnixNano: checked.UnixNano(), RecordedUnixNano: recorded.UnixNano(),
		QualificationExpiresUnixNano: qualification.ExpiresUnixNano,
	}
	sealed, err := contract.SealWorkspaceReadTerminalFactV2(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func postActualIndeterminateTerminalV2(t *testing.T, qualification contract.WorkspaceReadExecutionQualificationV2, state contract.WorkspaceReadPhysicalJournalStateV2) contract.WorkspaceReadTerminalFactV2 {
	t.Helper()
	journalTime := time.Unix(0, qualification.ExpiresUnixNano).Add(time.Second)
	revision := uint64(1)
	stage := contract.WorkspaceReadIndeterminatePhysicalStartedV2
	if state == contract.WorkspaceReadPhysicalJournalCompletedV2 {
		revision = 2
		stage = contract.WorkspaceReadIndeterminateCompletedBeforeS2V2
	}
	errorClass := contract.WorkspaceReadIndeterminateErrorActualPointUnknownV2
	if state == contract.WorkspaceReadPhysicalJournalCompletedV2 {
		errorClass = contract.WorkspaceReadIndeterminateErrorActualPointUnknownV2
	}
	journal := contract.WorkspaceReadPhysicalJournalRefV2{
		AttemptID: qualification.RuntimeAttempt.AttemptID, RequestDigest: qualification.ActualRequestDigest,
		PayloadDigest: qualification.PayloadDigest, Phase: contract.WorkspaceReadPhysicalJournalExecuteV2,
		State: state, Revision: revision, RecordedUnixNano: journalTime.UnixNano(),
		RecordDigest: postActualDigestV2(t, "journal-"+string(state)),
	}
	evidence, err := contract.SealWorkspaceReadIndeterminateEvidenceV2(
		qualification.Ref,
		qualification.OriginAttempt,
		journal,
		errorClass,
		postActualDigestV2(t, "unknown-error"),
		journalTime.Add(time.Second).UnixNano(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Stage != stage {
		t.Fatalf("indeterminate stage drifted: got %s want %s", evidence.Stage, stage)
	}
	value := contract.WorkspaceReadTerminalFactV2{
		Qualification: qualification.Ref, OriginAttempt: qualification.OriginAttempt,
		RuntimeAttempt: qualification.RuntimeAttempt, RuntimeAttemptDigest: qualification.RuntimeAttemptDigest,
		ActualRequestDigest:          qualification.ActualRequestDigest,
		Journal:                      journal,
		Outcome:                      contract.WorkspaceReadTerminalIndeterminateV2,
		Indeterminate:                &contract.WorkspaceReadIndeterminateTerminalV2{Evidence: evidence},
		OutcomeCheckedUnixNano:       journalTime.Add(time.Second).UnixNano(),
		RecordedUnixNano:             journalTime.Add(2 * time.Second).UnixNano(),
		QualificationExpiresUnixNano: qualification.ExpiresUnixNano,
	}
	sealed, err := contract.SealWorkspaceReadTerminalFactV2(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func postActualDigestV2(t *testing.T, value string) string {
	t.Helper()
	digest, err := contract.Digest("workspace-read-post-actual-v2-test", value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

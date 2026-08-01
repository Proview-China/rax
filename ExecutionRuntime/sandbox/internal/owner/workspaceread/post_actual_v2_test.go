package workspaceread

import (
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestQualificationS1CheckedClosesEveryAuthoritativeSourceV2(t *testing.T) {
	const checked = int64(100)
	base := struct {
		fact           contract.WorkspaceReadExecutionQualificationV2
		binding        sandboxports.WorkspaceReadAdmissionAttemptBindingV2
		reservation    contract.WorkspaceReadReservationV1
		attempt        contract.WorkspaceReadAttemptV1
		publication    contract.WorkspaceReadCommandPublicationV2
		ownerCurrent   contract.WorkspaceReadCommandOwnerCurrentV2
		workspace      contract.WorkspaceView
		query          sandboxports.WorkspaceReadCurrentQueryV2
		runtimeCurrent runtimeports.CurrentOperationDispatchEnforcementV4
	}{
		fact: contract.WorkspaceReadExecutionQualificationV2{
			S1CheckedUnixNano: checked,
			AdmissionReceipt:  contract.WorkspaceReadReceiptBindingV1{CheckedUnixNano: checked},
		},
		binding: sandboxports.WorkspaceReadAdmissionAttemptBindingV2{
			AdmissionBinding:     sandboxports.WorkspaceReadAdmissionAttemptBindingV1{CreatedUnixNano: checked},
			WorkspaceReadCommand: contract.WorkspaceReadCommandV1{Meta: contract.Meta{UpdatedUnixNano: checked}},
		},
		reservation:  contract.WorkspaceReadReservationV1{Meta: contract.Meta{UpdatedUnixNano: checked}},
		attempt:      contract.WorkspaceReadAttemptV1{Meta: contract.Meta{UpdatedUnixNano: checked}},
		publication:  contract.WorkspaceReadCommandPublicationV2{Meta: contract.Meta{UpdatedUnixNano: checked}},
		ownerCurrent: contract.WorkspaceReadCommandOwnerCurrentV2{Meta: contract.Meta{UpdatedUnixNano: checked}, CheckedUnixNano: checked},
		workspace:    contract.WorkspaceView{Meta: contract.Meta{UpdatedUnixNano: checked}},
		query: sandboxports.WorkspaceReadCurrentQueryV2{
			Base: sandboxports.WorkspaceReadCurrentQueryV1{CheckedUnixNano: checked},
		},
		runtimeCurrent: runtimeports.CurrentOperationDispatchEnforcementV4{CheckedUnixNano: checked},
	}
	if !qualificationS1CheckedClosesSourcesV2(
		base.fact, base.binding, base.reservation, base.attempt, base.publication,
		base.ownerCurrent, base.workspace, base.query, base.runtimeCurrent,
	) {
		t.Fatal("equal authoritative source timestamps did not close")
	}
	tests := map[string]func(){
		"admission receipt": func() { base.fact.AdmissionReceipt.CheckedUnixNano = checked + 1 },
		"admission binding": func() { base.binding.AdmissionBinding.CreatedUnixNano = checked + 1 },
		"command":           func() { base.binding.WorkspaceReadCommand.Meta.UpdatedUnixNano = checked + 1 },
		"publication":       func() { base.publication.Meta.UpdatedUnixNano = checked + 1 },
		"owner current":     func() { base.ownerCurrent.CheckedUnixNano = checked + 1 },
		"owner current meta": func() {
			base.ownerCurrent.Meta.UpdatedUnixNano = checked + 1
		},
		"workspace":   func() { base.workspace.Meta.UpdatedUnixNano = checked + 1 },
		"reservation": func() { base.reservation.Meta.UpdatedUnixNano = checked + 1 },
		"attempt":     func() { base.attempt.Meta.UpdatedUnixNano = checked + 1 },
		"query":       func() { base.query.Base.CheckedUnixNano = checked + 1 },
		"runtime":     func() { base.runtimeCurrent.CheckedUnixNano = checked + 1 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			copy := base
			mutate()
			if qualificationS1CheckedClosesSourcesV2(
				base.fact, base.binding, base.reservation, base.attempt, base.publication,
				base.ownerCurrent, base.workspace, base.query, base.runtimeCurrent,
			) {
				t.Fatalf("%s newer than S1Checked was accepted", name)
			}
			base = copy
		})
	}
}

func postActualOwnerQualificationV2(t *testing.T) contract.WorkspaceReadExecutionQualificationV2 {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	expires := now.Add(time.Minute)
	stable := runtimecore.DigestBytes([]byte("owner-stable"))
	admission, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(
		runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
			ID: "owner-admission", Revision: 1, StableKeyDigest: stable, Admitted: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	attempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: runtimecore.DigestBytes([]byte("owner-operation")),
		EffectID:        runtimecore.EffectIntentID("owner-effect"),
		IntentRevision:  1,
		IntentDigest:    runtimecore.DigestBytes([]byte("owner-intent")),
		PermitID:        "owner-permit",
		PermitRevision:  1,
		PermitDigest:    runtimecore.DigestBytes([]byte("owner-permit")),
		AttemptID:       "owner-runtime-attempt",
	}
	if err = attempt.Validate(); err != nil {
		t.Fatal(err)
	}
	attemptDigest, err := contract.WorkspaceReadSourceRuntimeAttemptDigestV2(attempt)
	if err != nil {
		t.Fatal(err)
	}
	value := contract.WorkspaceReadExecutionQualificationV2{
		OriginAttempt: contract.WorkspaceReadAttemptRefV1{ID: "owner-origin", Revision: 1, Digest: postActualOwnerDigestV2(t, "origin")},
		Reservation:   contract.Ref{ID: "owner-reservation", Revision: 1, Digest: postActualOwnerDigestV2(t, "reservation")},
		AdmissionReceipt: contract.WorkspaceReadReceiptBindingV1{
			ID: admission.ID, Revision: uint64(admission.Revision), Digest: string(admission.Digest),
			StableKeyDigest: string(admission.StableKeyDigest), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
		},
		RuntimeAdmissionReceipt:       admission,
		AdmissionAttemptBindingDigest: postActualOwnerDigestV2(t, "admission-binding"),
		RuntimeAttempt:                attempt,
		RuntimeAttemptDigest:          attemptDigest,
		AuthorizationDigest:           runtimecore.DigestBytes([]byte("owner-authorization")),
		Association: runtimeports.PreparedDomainCommandAssociationRefV1{
			ID: "owner-association", Revision: 1, Digest: runtimecore.DigestBytes([]byte("owner-association")),
		},
		Command:                      contract.Ref{ID: "owner-command", Revision: 1, Digest: postActualOwnerDigestV2(t, "command")},
		CommandPublication:           contract.Ref{ID: "owner-publication", Revision: 1, Digest: postActualOwnerDigestV2(t, "publication")},
		CommandOwnerCurrent:          contract.Ref{ID: "owner-current", Revision: 1, Digest: postActualOwnerDigestV2(t, "current")},
		WorkspaceView:                contract.Ref{ID: "owner-workspace", Revision: 1, Digest: postActualOwnerDigestV2(t, "workspace")},
		WorkspaceLeaseDigest:         postActualOwnerDigestV2(t, "lease"),
		CurrentQueryDigest:           postActualOwnerDigestV2(t, "query"),
		ExpectedRuntimeCurrentDigest: runtimecore.DigestBytes([]byte("runtime-current")),
		ActualRequestDigest:          postActualOwnerDigestV2(t, "request"),
		PayloadDigest:                postActualOwnerDigestV2(t, "payload"),
		S1CheckedUnixNano:            now.UnixNano(),
		ExpiresUnixNano:              expires.UnixNano(),
	}
	sealed, err := contract.SealWorkspaceReadExecutionQualificationV2(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func postActualOwnerJournalV2(
	t *testing.T,
	qualification contract.WorkspaceReadExecutionQualificationV2,
	state contract.WorkspaceReadPhysicalJournalStateV2,
) contract.WorkspaceReadPhysicalJournalRefV2 {
	t.Helper()
	revision := uint64(1)
	if state == contract.WorkspaceReadPhysicalJournalCompletedV2 {
		revision = 2
	}
	return contract.WorkspaceReadPhysicalJournalRefV2{
		AttemptID: qualification.RuntimeAttempt.AttemptID, RequestDigest: qualification.ActualRequestDigest,
		PayloadDigest: qualification.PayloadDigest, Phase: contract.WorkspaceReadPhysicalJournalExecuteV2,
		State: state, Revision: revision, RecordedUnixNano: qualification.ExpiresUnixNano + int64(time.Second),
		RecordDigest: postActualOwnerDigestV2(t, "journal-"+string(state)),
	}
}

func postActualOwnerObservedTerminalV2(t *testing.T, qualification contract.WorkspaceReadExecutionQualificationV2) contract.WorkspaceReadTerminalFactV2 {
	t.Helper()
	journal := postActualOwnerJournalV2(t, qualification, contract.WorkspaceReadPhysicalJournalCompletedV2)
	observationDigest := postActualOwnerDigestV2(t, "sandbox-observation")
	providerObservationDigest := postActualOwnerDigestV2(t, "provider-observation")
	checked := journal.RecordedUnixNano + int64(time.Second)
	proof, err := contract.SealWorkspaceReadOutcomeS2ProofV2(contract.WorkspaceReadOutcomeS2ProofV2{
		Qualification: qualification.Ref, OriginAttempt: qualification.OriginAttempt,
		Association: qualification.Association, CommandPublication: qualification.CommandPublication,
		CommandOwnerCurrent: qualification.CommandOwnerCurrent, WorkspaceView: qualification.WorkspaceView,
		WorkspaceLeaseDigest: qualification.WorkspaceLeaseDigest,
		RuntimeCurrentDigest: qualification.ExpectedRuntimeCurrentDigest, Journal: journal,
		Observation: contract.Ref{ID: "owner-observation", Revision: 1, Digest: observationDigest},
		ProviderReceipt: contract.WorkspaceReadReceiptBindingV1{
			ID: "owner-receipt", Revision: 1, Digest: postActualOwnerDigestV2(t, "receipt"),
			ObservationDigest: providerObservationDigest, StableKeyDigest: qualification.AdmissionReceipt.StableKeyDigest,
			CheckedUnixNano: journal.RecordedUnixNano, ExpiresUnixNano: checked + int64(time.Minute),
		},
		S2CheckedUnixNano: checked,
	})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := contract.SealWorkspaceReadTerminalFactV2(contract.WorkspaceReadTerminalFactV2{
		Qualification: qualification.Ref, OriginAttempt: qualification.OriginAttempt,
		RuntimeAttempt: qualification.RuntimeAttempt, RuntimeAttemptDigest: qualification.RuntimeAttemptDigest,
		ActualRequestDigest: qualification.ActualRequestDigest, Journal: journal,
		Outcome:                contract.WorkspaceReadTerminalObservedV2,
		Observed:               &contract.WorkspaceReadObservedTerminalV2{S2Proof: proof},
		OutcomeCheckedUnixNano: checked, RecordedUnixNano: checked + int64(time.Second),
		QualificationExpiresUnixNano: qualification.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func postActualOwnerIndeterminateTerminalV2(
	t *testing.T,
	qualification contract.WorkspaceReadExecutionQualificationV2,
	journal contract.WorkspaceReadPhysicalJournalRefV2,
	evidence contract.WorkspaceReadIndeterminateEvidenceV2,
) contract.WorkspaceReadTerminalFactV2 {
	t.Helper()
	checked := journal.RecordedUnixNano + int64(time.Second)
	fact, err := contract.SealWorkspaceReadTerminalFactV2(contract.WorkspaceReadTerminalFactV2{
		Qualification: qualification.Ref, OriginAttempt: qualification.OriginAttempt,
		RuntimeAttempt: qualification.RuntimeAttempt, RuntimeAttemptDigest: qualification.RuntimeAttemptDigest,
		ActualRequestDigest: qualification.ActualRequestDigest, Journal: journal,
		Outcome:                contract.WorkspaceReadTerminalIndeterminateV2,
		Indeterminate:          &contract.WorkspaceReadIndeterminateTerminalV2{Evidence: evidence},
		OutcomeCheckedUnixNano: checked, RecordedUnixNano: checked + int64(time.Second),
		QualificationExpiresUnixNano: qualification.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func postActualOwnerDigestV2(t *testing.T, value string) string {
	t.Helper()
	digest, err := contract.Digest("workspace-read-post-actual-owner-test", value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

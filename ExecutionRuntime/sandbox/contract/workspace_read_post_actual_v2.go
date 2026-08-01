package contract

import (
	"errors"
	"strings"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	WorkspaceReadPostActualContractVersionV2 = "praxis.sandbox/workspace-read-post-actual/v2"

	WorkspaceReadTerminalObservedV2      WorkspaceReadTerminalOutcomeV2 = "observed"
	WorkspaceReadTerminalIndeterminateV2 WorkspaceReadTerminalOutcomeV2 = "indeterminate"

	WorkspaceReadPhysicalJournalStartedV2   WorkspaceReadPhysicalJournalStateV2 = "started"
	WorkspaceReadPhysicalJournalCompletedV2 WorkspaceReadPhysicalJournalStateV2 = "completed"
	WorkspaceReadPhysicalJournalExecuteV2                                       = "execute"

	WorkspaceReadIndeterminateBoundaryV2 = "post_actual"

	WorkspaceReadIndeterminateErrorActualPointUnknownV2 WorkspaceReadIndeterminateErrorClassV2 = "actual_point_unknown"
	WorkspaceReadIndeterminateErrorS2UnavailableV2      WorkspaceReadIndeterminateErrorClassV2 = "s2_unavailable"
	WorkspaceReadIndeterminateErrorS2ExpiredV2          WorkspaceReadIndeterminateErrorClassV2 = "s2_expired"
	WorkspaceReadIndeterminateErrorS2DriftedV2          WorkspaceReadIndeterminateErrorClassV2 = "s2_drifted"
	WorkspaceReadIndeterminateErrorRecoveryUnknownV2    WorkspaceReadIndeterminateErrorClassV2 = "recovery_unknown"

	WorkspaceReadIndeterminatePhysicalStartedV2       WorkspaceReadIndeterminateStageV2 = "physical_started"
	WorkspaceReadIndeterminateCompletedBeforeS2V2     WorkspaceReadIndeterminateStageV2 = "physical_completed_before_s2"
	WorkspaceReadIndeterminateOutcomeS2UnavailableV2  WorkspaceReadIndeterminateStageV2 = "outcome_s2_unavailable"
	WorkspaceReadIndeterminateOutcomeS2ExpiredV2      WorkspaceReadIndeterminateStageV2 = "outcome_s2_expired"
	WorkspaceReadIndeterminateOutcomeS2DriftedV2      WorkspaceReadIndeterminateStageV2 = "outcome_s2_drifted"
	WorkspaceReadIndeterminateRecoveryIndeterminateV2 WorkspaceReadIndeterminateStageV2 = "recovery_indeterminate"
)

type WorkspaceReadTerminalOutcomeV2 string
type WorkspaceReadPhysicalJournalStateV2 string
type WorkspaceReadIndeterminateStageV2 string
type WorkspaceReadIndeterminateErrorClassV2 string

// WorkspaceReadExecutionQualificationRefV2 is an exact historical coordinate.
// ExpiresUnixNano records the former execution upper bound; it does not make an
// expired qualification current again.
type WorkspaceReadExecutionQualificationRefV2 struct {
	ID              string `json:"id"`
	Revision        uint64 `json:"revision"`
	Digest          string `json:"digest"`
	ExpiresUnixNano int64  `json:"expires_unix_nano"`
}

func (r WorkspaceReadExecutionQualificationRefV2) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision != 1 || !ValidDigest(r.Digest) || r.ExpiresUnixNano <= 0 {
		return errors.New("workspace read execution qualification Ref is incomplete")
	}
	return nil
}

// WorkspaceReadExecutionQualificationV2 is written before entering the Data
// Plane actual point. It records a verified S1 closure but is neither a current
// projection nor renewable execution authority.
type WorkspaceReadExecutionQualificationV2 struct {
	ContractVersion               string                                                        `json:"contract_version"`
	Ref                           WorkspaceReadExecutionQualificationRefV2                      `json:"ref"`
	OriginAttempt                 WorkspaceReadAttemptRefV1                                     `json:"origin_attempt"`
	Reservation                   Ref                                                           `json:"reservation"`
	AdmissionReceipt              WorkspaceReadReceiptBindingV1                                 `json:"admission_receipt"`
	RuntimeAdmissionReceipt       runtimeports.ControlledOperationProviderAdmissionReceiptRefV2 `json:"runtime_admission_receipt"`
	AdmissionAttemptBindingDigest string                                                        `json:"admission_attempt_binding_digest"`
	RuntimeAttempt                runtimeports.OperationDispatchAttemptRefV3                    `json:"runtime_attempt"`
	RuntimeAttemptDigest          runtimecore.Digest                                            `json:"runtime_attempt_digest"`
	AuthorizationDigest           runtimecore.Digest                                            `json:"authorization_digest"`
	Association                   runtimeports.PreparedDomainCommandAssociationRefV1            `json:"association"`
	Command                       Ref                                                           `json:"command"`
	CommandPublication            Ref                                                           `json:"command_publication"`
	CommandOwnerCurrent           Ref                                                           `json:"command_owner_current"`
	WorkspaceView                 Ref                                                           `json:"workspace_view"`
	WorkspaceLeaseDigest          string                                                        `json:"workspace_lease_digest"`
	CurrentQueryDigest            string                                                        `json:"current_query_digest"`
	ExpectedRuntimeCurrentDigest  runtimecore.Digest                                            `json:"expected_runtime_current_digest"`
	ActualRequestDigest           string                                                        `json:"actual_request_digest"`
	PayloadDigest                 string                                                        `json:"payload_digest"`
	S1CheckedUnixNano             int64                                                         `json:"s1_checked_unix_nano"`
	ExpiresUnixNano               int64                                                         `json:"expires_unix_nano"`
}

func (q WorkspaceReadExecutionQualificationV2) Validate() error {
	if q.ContractVersion != WorkspaceReadPostActualContractVersionV2 ||
		q.Ref.Validate() != nil ||
		q.OriginAttempt.Validate() != nil ||
		q.Reservation.ValidateShape("workspace read reservation") != nil ||
		q.AdmissionReceipt.Validate() != nil ||
		q.RuntimeAdmissionReceipt.Validate() != nil ||
		!q.RuntimeAdmissionReceipt.Admitted ||
		q.RuntimeAdmissionReceipt.NoEffect ||
		!ValidDigest(q.AdmissionAttemptBindingDigest) ||
		q.RuntimeAttempt.Validate() != nil ||
		q.RuntimeAttemptDigest.Validate() != nil ||
		q.AuthorizationDigest.Validate() != nil ||
		q.Association.Validate() != nil ||
		q.Command.ValidateShape("workspace read command") != nil ||
		q.CommandPublication.ValidateShape("workspace read command publication") != nil ||
		q.CommandOwnerCurrent.ValidateShape("workspace read command owner current") != nil ||
		q.WorkspaceView.ValidateShape("workspace view") != nil ||
		!ValidDigest(q.WorkspaceLeaseDigest) ||
		!ValidDigest(q.CurrentQueryDigest) ||
		q.ExpectedRuntimeCurrentDigest.Validate() != nil ||
		!ValidDigest(q.ActualRequestDigest) ||
		!ValidDigest(q.PayloadDigest) ||
		q.S1CheckedUnixNano <= 0 ||
		q.ExpiresUnixNano <= q.S1CheckedUnixNano ||
		q.Ref.ExpiresUnixNano != q.ExpiresUnixNano {
		return errors.New("workspace read execution qualification is incomplete")
	}
	if q.RuntimeAdmissionReceipt.ID != q.AdmissionReceipt.ID ||
		uint64(q.RuntimeAdmissionReceipt.Revision) != q.AdmissionReceipt.Revision ||
		string(q.RuntimeAdmissionReceipt.Digest) != q.AdmissionReceipt.Digest ||
		string(q.RuntimeAdmissionReceipt.StableKeyDigest) != q.AdmissionReceipt.StableKeyDigest {
		return errors.New("workspace read execution qualification admission drifted")
	}
	attemptDigest, err := WorkspaceReadSourceRuntimeAttemptDigestV2(q.RuntimeAttempt)
	if err != nil || attemptDigest != q.RuntimeAttemptDigest {
		return errors.New("workspace read execution qualification Runtime Attempt digest drifted")
	}
	expectedID, err := DeriveWorkspaceReadExecutionQualificationIDV2(q.OriginAttempt)
	if err != nil || q.Ref.ID != expectedID {
		return errors.New("workspace read execution qualification identity drifted")
	}
	copy := cloneWorkspaceReadExecutionQualificationV2(q)
	copy.Ref.Digest = ""
	digest, err := Digest("workspace-read-execution-qualification-v2", copy)
	if err != nil || digest != q.Ref.Digest {
		return errors.New("workspace read execution qualification digest drifted")
	}
	return nil
}

func SealWorkspaceReadExecutionQualificationV2(q WorkspaceReadExecutionQualificationV2) (WorkspaceReadExecutionQualificationV2, error) {
	q = cloneWorkspaceReadExecutionQualificationV2(q)
	q.ContractVersion = WorkspaceReadPostActualContractVersionV2
	id, err := DeriveWorkspaceReadExecutionQualificationIDV2(q.OriginAttempt)
	if err != nil {
		return WorkspaceReadExecutionQualificationV2{}, err
	}
	q.Ref = WorkspaceReadExecutionQualificationRefV2{ID: id, Revision: 1, ExpiresUnixNano: q.ExpiresUnixNano}
	digest, err := Digest("workspace-read-execution-qualification-v2", q)
	if err != nil {
		return WorkspaceReadExecutionQualificationV2{}, err
	}
	q.Ref.Digest = digest
	return q, q.Validate()
}

func DeriveWorkspaceReadExecutionQualificationIDV2(origin WorkspaceReadAttemptRefV1) (string, error) {
	if err := origin.Validate(); err != nil {
		return "", err
	}
	digest, err := Digest("workspace-read-execution-qualification-v2-id", origin)
	if err != nil {
		return "", err
	}
	return "workspace-read-qualification-" + digest, nil
}

// WorkspaceReadPhysicalJournalLookupV2 is the complete historical recovery
// key. It is derived from a sealed Qualification and intentionally excludes
// expired Dispatch/current bodies.
type WorkspaceReadPhysicalJournalLookupV2 struct {
	RuntimeAttemptID string `json:"runtime_attempt_id"`
	RequestDigest    string `json:"request_digest"`
	PayloadDigest    string `json:"payload_digest"`
	Phase            string `json:"phase"`
	Digest           string `json:"digest"`
}

func (l WorkspaceReadPhysicalJournalLookupV2) Validate() error {
	if strings.TrimSpace(l.RuntimeAttemptID) == "" ||
		!ValidDigest(l.RequestDigest) ||
		!ValidDigest(l.PayloadDigest) ||
		l.Phase != WorkspaceReadPhysicalJournalExecuteV2 ||
		!ValidDigest(l.Digest) {
		return errors.New("workspace read physical journal lookup is incomplete")
	}
	copy := l
	copy.Digest = ""
	digest, err := Digest("workspace-read-physical-journal-lookup-v2", copy)
	if err != nil || digest != l.Digest {
		return errors.New("workspace read physical journal lookup digest drifted")
	}
	return nil
}

func BuildWorkspaceReadPhysicalJournalLookupV2(
	qualification WorkspaceReadExecutionQualificationV2,
) (WorkspaceReadPhysicalJournalLookupV2, error) {
	if err := qualification.Validate(); err != nil {
		return WorkspaceReadPhysicalJournalLookupV2{}, err
	}
	lookup := WorkspaceReadPhysicalJournalLookupV2{
		RuntimeAttemptID: qualification.RuntimeAttempt.AttemptID,
		RequestDigest:    qualification.ActualRequestDigest,
		PayloadDigest:    qualification.PayloadDigest,
		Phase:            WorkspaceReadPhysicalJournalExecuteV2,
	}
	digest, err := Digest("workspace-read-physical-journal-lookup-v2", lookup)
	if err != nil {
		return WorkspaceReadPhysicalJournalLookupV2{}, err
	}
	lookup.Digest = digest
	return lookup, lookup.Validate()
}

// WorkspaceReadPhysicalJournalRefV2 is a neutral exact projection of the
// Sandbox Data Plane journal. Completed result bytes are deliberately absent.
type WorkspaceReadPhysicalJournalRefV2 struct {
	AttemptID        string                              `json:"attempt_id"`
	RequestDigest    string                              `json:"request_digest"`
	PayloadDigest    string                              `json:"payload_digest"`
	Phase            string                              `json:"phase"`
	State            WorkspaceReadPhysicalJournalStateV2 `json:"state"`
	Revision         uint64                              `json:"revision"`
	RecordedUnixNano int64                               `json:"recorded_unix_nano"`
	RecordDigest     string                              `json:"record_digest"`
}

func (r WorkspaceReadPhysicalJournalRefV2) Validate() error {
	if strings.TrimSpace(r.AttemptID) == "" ||
		!ValidDigest(r.RequestDigest) ||
		!ValidDigest(r.PayloadDigest) ||
		strings.TrimSpace(r.Phase) == "" ||
		r.Phase != WorkspaceReadPhysicalJournalExecuteV2 ||
		(r.State != WorkspaceReadPhysicalJournalStartedV2 && r.State != WorkspaceReadPhysicalJournalCompletedV2) ||
		r.Revision == 0 ||
		r.RecordedUnixNano <= 0 ||
		!ValidDigest(r.RecordDigest) {
		return errors.New("workspace read physical journal Ref is incomplete")
	}
	if (r.State == WorkspaceReadPhysicalJournalStartedV2 && r.Revision != 1) ||
		(r.State == WorkspaceReadPhysicalJournalCompletedV2 && r.Revision != 2) {
		return errors.New("workspace read physical journal revision does not match its state")
	}
	return nil
}

func (r WorkspaceReadPhysicalJournalRefV2) ValidateLookupV2(lookup WorkspaceReadPhysicalJournalLookupV2) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := lookup.Validate(); err != nil {
		return err
	}
	if r.AttemptID != lookup.RuntimeAttemptID ||
		r.RequestDigest != lookup.RequestDigest ||
		r.PayloadDigest != lookup.PayloadDigest ||
		r.Phase != lookup.Phase {
		return errors.New("workspace read physical journal belongs to another exact lookup")
	}
	return nil
}

type WorkspaceReadTerminalRefV2 struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Digest   string `json:"digest"`
}

func (r WorkspaceReadTerminalRefV2) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision != 1 || !ValidDigest(r.Digest) {
		return errors.New("workspace read terminal Ref is incomplete")
	}
	return nil
}

// WorkspaceReadOutcomeS2ProofV2 is the complete, recomputable post-actual
// closure. Every stable axis must equal the Qualification's S1 closure.
type WorkspaceReadOutcomeS2ProofV2 struct {
	Qualification        WorkspaceReadExecutionQualificationRefV2           `json:"qualification"`
	OriginAttempt        WorkspaceReadAttemptRefV1                          `json:"origin_attempt"`
	Association          runtimeports.PreparedDomainCommandAssociationRefV1 `json:"association"`
	CommandPublication   Ref                                                `json:"command_publication"`
	CommandOwnerCurrent  Ref                                                `json:"command_owner_current"`
	WorkspaceView        Ref                                                `json:"workspace_view"`
	WorkspaceLeaseDigest string                                             `json:"workspace_lease_digest"`
	RuntimeCurrentDigest runtimecore.Digest                                 `json:"runtime_current_digest"`
	Journal              WorkspaceReadPhysicalJournalRefV2                  `json:"journal"`
	// Observation is the Sandbox-owned Observation Ref. The provider receipt
	// separately carries the Rust Provider Observation digest; equating those
	// digest domains would create a circular Sandbox Observation hash.
	Observation       Ref                           `json:"observation"`
	ProviderReceipt   WorkspaceReadReceiptBindingV1 `json:"provider_receipt"`
	S2CheckedUnixNano int64                         `json:"s2_checked_unix_nano"`
	Digest            string                        `json:"digest"`
}

func (p WorkspaceReadOutcomeS2ProofV2) Validate() error {
	if p.Qualification.Validate() != nil ||
		p.OriginAttempt.Validate() != nil ||
		p.Association.Validate() != nil ||
		p.CommandPublication.ValidateShape("workspace read command publication") != nil ||
		p.CommandOwnerCurrent.ValidateShape("workspace read command owner current") != nil ||
		p.WorkspaceView.ValidateShape("workspace view") != nil ||
		!ValidDigest(p.WorkspaceLeaseDigest) ||
		p.RuntimeCurrentDigest.Validate() != nil ||
		p.Journal.Validate() != nil ||
		p.Journal.State != WorkspaceReadPhysicalJournalCompletedV2 ||
		p.Observation.ValidateShape("workspace read provider observation") != nil ||
		p.ProviderReceipt.Validate() != nil ||
		p.S2CheckedUnixNano < p.Journal.RecordedUnixNano ||
		!ValidDigest(p.Digest) {
		return errors.New("workspace read outcome S2 proof is incomplete")
	}
	copy := p
	copy.Digest = ""
	digest, err := Digest("workspace-read-outcome-s2-proof-v2", copy)
	if err != nil || digest != p.Digest {
		return errors.New("workspace read outcome S2 proof digest drifted")
	}
	return nil
}

func SealWorkspaceReadOutcomeS2ProofV2(p WorkspaceReadOutcomeS2ProofV2) (WorkspaceReadOutcomeS2ProofV2, error) {
	p.Digest = ""
	digest, err := Digest("workspace-read-outcome-s2-proof-v2", p)
	if err != nil {
		return WorkspaceReadOutcomeS2ProofV2{}, err
	}
	p.Digest = digest
	return p, p.Validate()
}

func (p WorkspaceReadOutcomeS2ProofV2) ValidateQualificationV2(q WorkspaceReadExecutionQualificationV2) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := q.Validate(); err != nil {
		return err
	}
	if p.Qualification != q.Ref ||
		p.OriginAttempt != q.OriginAttempt ||
		p.Association != q.Association ||
		p.CommandPublication != q.CommandPublication ||
		p.CommandOwnerCurrent != q.CommandOwnerCurrent ||
		p.WorkspaceView != q.WorkspaceView ||
		p.WorkspaceLeaseDigest != q.WorkspaceLeaseDigest ||
		p.RuntimeCurrentDigest != q.ExpectedRuntimeCurrentDigest {
		return errors.New("workspace read outcome S2 proof drifted from Qualification S1")
	}
	return nil
}

type WorkspaceReadObservedTerminalV2 struct {
	S2Proof WorkspaceReadOutcomeS2ProofV2 `json:"s2_proof"`
}

func (o WorkspaceReadObservedTerminalV2) Validate(journal WorkspaceReadPhysicalJournalRefV2, outcomeChecked int64) error {
	if o.S2Proof.Validate() != nil ||
		o.S2Proof.Journal != journal ||
		outcomeChecked < o.S2Proof.S2CheckedUnixNano {
		return errors.New("workspace read observed terminal proof is incomplete")
	}
	return nil
}

type WorkspaceReadIndeterminateEvidenceV2 struct {
	Qualification   WorkspaceReadExecutionQualificationRefV2 `json:"qualification"`
	OriginAttempt   WorkspaceReadAttemptRefV1                `json:"origin_attempt"`
	Boundary        string                                   `json:"boundary"`
	Stage           WorkspaceReadIndeterminateStageV2        `json:"stage"`
	ErrorClass      WorkspaceReadIndeterminateErrorClassV2   `json:"error_class"`
	Journal         WorkspaceReadPhysicalJournalRefV2        `json:"journal"`
	CheckedUnixNano int64                                    `json:"checked_unix_nano"`
	ErrorDigest     string                                   `json:"error_digest"`
	Digest          string                                   `json:"digest"`
}

func (u WorkspaceReadIndeterminateEvidenceV2) Validate() error {
	if u.Qualification.Validate() != nil ||
		u.OriginAttempt.Validate() != nil ||
		u.Boundary != WorkspaceReadIndeterminateBoundaryV2 ||
		!validWorkspaceReadIndeterminateStageV2(u.Stage) ||
		u.Journal.Validate() != nil ||
		u.CheckedUnixNano < u.Journal.RecordedUnixNano ||
		!ValidDigest(u.ErrorDigest) ||
		!ValidDigest(u.Digest) {
		return errors.New("workspace read indeterminate terminal proof is incomplete")
	}
	expectedQualificationID, err := DeriveWorkspaceReadExecutionQualificationIDV2(u.OriginAttempt)
	if err != nil || u.Qualification.ID != expectedQualificationID {
		return errors.New("workspace read indeterminate evidence belongs to another origin")
	}
	expectedStage, err := workspaceReadIndeterminateStageV2(u.Journal, u.ErrorClass)
	if err != nil || expectedStage != u.Stage {
		return errors.New("workspace read indeterminate stage drifted from journal or error class")
	}
	copy := u
	copy.Digest = ""
	digest, err := Digest("workspace-read-indeterminate-evidence-v2", copy)
	if err != nil || digest != u.Digest {
		return errors.New("workspace read indeterminate evidence digest drifted")
	}
	return nil
}

func SealWorkspaceReadIndeterminateEvidenceV2(
	qualification WorkspaceReadExecutionQualificationRefV2,
	origin WorkspaceReadAttemptRefV1,
	journal WorkspaceReadPhysicalJournalRefV2,
	errorClass WorkspaceReadIndeterminateErrorClassV2,
	errorDigest string,
	checkedUnixNano int64,
) (WorkspaceReadIndeterminateEvidenceV2, error) {
	stage, err := workspaceReadIndeterminateStageV2(journal, errorClass)
	if err != nil {
		return WorkspaceReadIndeterminateEvidenceV2{}, err
	}
	value := WorkspaceReadIndeterminateEvidenceV2{
		Qualification: qualification, OriginAttempt: origin,
		Boundary: WorkspaceReadIndeterminateBoundaryV2, Stage: stage, ErrorClass: errorClass,
		Journal: journal, CheckedUnixNano: checkedUnixNano, ErrorDigest: errorDigest,
	}
	digest, err := Digest("workspace-read-indeterminate-evidence-v2", value)
	if err != nil {
		return WorkspaceReadIndeterminateEvidenceV2{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

type WorkspaceReadIndeterminateTerminalV2 struct {
	Evidence WorkspaceReadIndeterminateEvidenceV2 `json:"evidence"`
}

func (u WorkspaceReadIndeterminateTerminalV2) Validate(journal WorkspaceReadPhysicalJournalRefV2) error {
	if err := u.Evidence.Validate(); err != nil {
		return err
	}
	if u.Evidence.Journal != journal {
		return errors.New("workspace read indeterminate evidence belongs to another journal")
	}
	return nil
}

// WorkspaceReadTerminalFactV2 is append-only historical evidence. Its
// timestamps may be after Qualification expiry because expiry removes authority
// but cannot erase an actual point that already occurred.
type WorkspaceReadTerminalFactV2 struct {
	ContractVersion              string                                     `json:"contract_version"`
	Ref                          WorkspaceReadTerminalRefV2                 `json:"ref"`
	Qualification                WorkspaceReadExecutionQualificationRefV2   `json:"qualification"`
	OriginAttempt                WorkspaceReadAttemptRefV1                  `json:"origin_attempt"`
	RuntimeAttempt               runtimeports.OperationDispatchAttemptRefV3 `json:"runtime_attempt"`
	RuntimeAttemptDigest         runtimecore.Digest                         `json:"runtime_attempt_digest"`
	ActualRequestDigest          string                                     `json:"actual_request_digest"`
	Journal                      WorkspaceReadPhysicalJournalRefV2          `json:"journal"`
	Outcome                      WorkspaceReadTerminalOutcomeV2             `json:"outcome"`
	Observed                     *WorkspaceReadObservedTerminalV2           `json:"observed,omitempty"`
	Indeterminate                *WorkspaceReadIndeterminateTerminalV2      `json:"indeterminate,omitempty"`
	OutcomeCheckedUnixNano       int64                                      `json:"outcome_checked_unix_nano"`
	RecordedUnixNano             int64                                      `json:"recorded_unix_nano"`
	QualificationExpiresUnixNano int64                                      `json:"qualification_expires_unix_nano"`
}

func (f WorkspaceReadTerminalFactV2) Validate() error {
	if f.ContractVersion != WorkspaceReadPostActualContractVersionV2 ||
		f.Ref.Validate() != nil ||
		f.Qualification.Validate() != nil ||
		f.OriginAttempt.Validate() != nil ||
		f.RuntimeAttempt.Validate() != nil ||
		f.RuntimeAttemptDigest.Validate() != nil ||
		!ValidDigest(f.ActualRequestDigest) ||
		f.Journal.Validate() != nil ||
		f.OutcomeCheckedUnixNano < f.Journal.RecordedUnixNano ||
		f.RecordedUnixNano < f.OutcomeCheckedUnixNano ||
		f.QualificationExpiresUnixNano != f.Qualification.ExpiresUnixNano ||
		f.Journal.AttemptID != f.RuntimeAttempt.AttemptID ||
		f.Journal.RequestDigest != f.ActualRequestDigest {
		return errors.New("workspace read terminal fact is incomplete")
	}
	attemptDigest, err := WorkspaceReadSourceRuntimeAttemptDigestV2(f.RuntimeAttempt)
	if err != nil || attemptDigest != f.RuntimeAttemptDigest {
		return errors.New("workspace read terminal Runtime Attempt digest drifted")
	}
	expectedQualificationID, err := DeriveWorkspaceReadExecutionQualificationIDV2(f.OriginAttempt)
	if err != nil || f.Qualification.ID != expectedQualificationID {
		return errors.New("workspace read terminal Qualification belongs to another origin Attempt")
	}
	switch f.Outcome {
	case WorkspaceReadTerminalObservedV2:
		if f.Observed == nil || f.Indeterminate != nil ||
			f.Observed.Validate(f.Journal, f.OutcomeCheckedUnixNano) != nil ||
			f.Observed.S2Proof.Qualification != f.Qualification ||
			f.Observed.S2Proof.OriginAttempt != f.OriginAttempt {
			return errors.New("workspace read observed terminal sidecar is invalid")
		}
	case WorkspaceReadTerminalIndeterminateV2:
		if f.Observed != nil || f.Indeterminate == nil ||
			f.Indeterminate.Validate(f.Journal) != nil ||
			f.Indeterminate.Evidence.Qualification != f.Qualification ||
			f.Indeterminate.Evidence.OriginAttempt != f.OriginAttempt ||
			f.OutcomeCheckedUnixNano < f.Indeterminate.Evidence.CheckedUnixNano {
			return errors.New("workspace read indeterminate terminal sidecar is invalid")
		}
	default:
		return errors.New("workspace read terminal outcome is invalid")
	}
	expectedID, err := DeriveWorkspaceReadTerminalIDV2(f.OriginAttempt)
	if err != nil || f.Ref.ID != expectedID {
		return errors.New("workspace read terminal identity drifted")
	}
	copy := cloneWorkspaceReadTerminalFactV2(f)
	copy.Ref.Digest = ""
	digest, err := Digest("workspace-read-terminal-fact-v2", copy)
	if err != nil || digest != f.Ref.Digest {
		return errors.New("workspace read terminal fact digest drifted")
	}
	return nil
}

func SealWorkspaceReadTerminalFactV2(f WorkspaceReadTerminalFactV2) (WorkspaceReadTerminalFactV2, error) {
	f = cloneWorkspaceReadTerminalFactV2(f)
	f.ContractVersion = WorkspaceReadPostActualContractVersionV2
	id, err := DeriveWorkspaceReadTerminalIDV2(f.OriginAttempt)
	if err != nil {
		return WorkspaceReadTerminalFactV2{}, err
	}
	f.Ref = WorkspaceReadTerminalRefV2{ID: id, Revision: 1}
	digest, err := Digest("workspace-read-terminal-fact-v2", f)
	if err != nil {
		return WorkspaceReadTerminalFactV2{}, err
	}
	f.Ref.Digest = digest
	return f, f.Validate()
}

func DeriveWorkspaceReadTerminalIDV2(origin WorkspaceReadAttemptRefV1) (string, error) {
	if err := origin.Validate(); err != nil {
		return "", err
	}
	digest, err := Digest("workspace-read-terminal-fact-v2-id", origin)
	if err != nil {
		return "", err
	}
	return "workspace-read-terminal-" + digest, nil
}

func validWorkspaceReadIndeterminateStageV2(stage WorkspaceReadIndeterminateStageV2) bool {
	switch stage {
	case WorkspaceReadIndeterminatePhysicalStartedV2,
		WorkspaceReadIndeterminateCompletedBeforeS2V2,
		WorkspaceReadIndeterminateOutcomeS2UnavailableV2,
		WorkspaceReadIndeterminateOutcomeS2ExpiredV2,
		WorkspaceReadIndeterminateOutcomeS2DriftedV2,
		WorkspaceReadIndeterminateRecoveryIndeterminateV2:
		return true
	default:
		return false
	}
}

func workspaceReadIndeterminateStageV2(
	journal WorkspaceReadPhysicalJournalRefV2,
	errorClass WorkspaceReadIndeterminateErrorClassV2,
) (WorkspaceReadIndeterminateStageV2, error) {
	if err := journal.Validate(); err != nil {
		return "", err
	}
	if journal.State == WorkspaceReadPhysicalJournalStartedV2 {
		if errorClass != WorkspaceReadIndeterminateErrorActualPointUnknownV2 &&
			errorClass != WorkspaceReadIndeterminateErrorRecoveryUnknownV2 {
			return "", errors.New("workspace read started journal has incompatible error class")
		}
		return WorkspaceReadIndeterminatePhysicalStartedV2, nil
	}
	switch errorClass {
	case WorkspaceReadIndeterminateErrorActualPointUnknownV2:
		return WorkspaceReadIndeterminateCompletedBeforeS2V2, nil
	case WorkspaceReadIndeterminateErrorS2UnavailableV2:
		return WorkspaceReadIndeterminateOutcomeS2UnavailableV2, nil
	case WorkspaceReadIndeterminateErrorS2ExpiredV2:
		return WorkspaceReadIndeterminateOutcomeS2ExpiredV2, nil
	case WorkspaceReadIndeterminateErrorS2DriftedV2:
		return WorkspaceReadIndeterminateOutcomeS2DriftedV2, nil
	case WorkspaceReadIndeterminateErrorRecoveryUnknownV2:
		return WorkspaceReadIndeterminateRecoveryIndeterminateV2, nil
	default:
		return "", errors.New("workspace read indeterminate error class is invalid")
	}
}

func WorkspaceReadRuntimeLeaseDigestV2(lease RuntimeLeaseBinding) (string, error) {
	if err := lease.ValidateShape(); err != nil {
		return "", err
	}
	return Digest("workspace-read-runtime-lease-v2", lease)
}

func cloneWorkspaceReadExecutionQualificationV2(q WorkspaceReadExecutionQualificationV2) WorkspaceReadExecutionQualificationV2 {
	if q.RuntimeAttempt.Delegation != nil {
		delegation := *q.RuntimeAttempt.Delegation
		q.RuntimeAttempt.Delegation = &delegation
	}
	return q
}

func cloneWorkspaceReadTerminalFactV2(f WorkspaceReadTerminalFactV2) WorkspaceReadTerminalFactV2 {
	if f.RuntimeAttempt.Delegation != nil {
		delegation := *f.RuntimeAttempt.Delegation
		f.RuntimeAttempt.Delegation = &delegation
	}
	if f.Observed != nil {
		observed := *f.Observed
		f.Observed = &observed
	}
	if f.Indeterminate != nil {
		indeterminate := *f.Indeterminate
		f.Indeterminate = &indeterminate
	}
	return f
}

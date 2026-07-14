package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const RunClaimAssociationContractVersionV2 = "2.0.0"

type RunClaimAssociationStateV2 string

const RunClaimAssociatedV2 RunClaimAssociationStateV2 = "associated"

type RunClaimAssociationFactV2 struct {
	ContractVersion          string                      `json:"contract_version"`
	ID                       string                      `json:"association_id"`
	Revision                 core.Revision               `json:"revision"`
	State                    RunClaimAssociationStateV2  `json:"state"`
	RunID                    core.AgentRunID             `json:"run_id"`
	RunRevisionAtAssociation core.Revision               `json:"run_revision_at_association"`
	RunIdentityDigest        core.Digest                 `json:"run_identity_digest"`
	ExecutionScope           core.ExecutionScope         `json:"execution_scope"`
	ExecutionScopeDigest     core.Digest                 `json:"execution_scope_digest"`
	LineagePlanDigest        core.Digest                 `json:"lineage_plan_digest"`
	ClaimKind                core.RunCompletionClaimKind `json:"claim_kind"`
	RegistrationID           string                      `json:"registration_id"`
	SourceID                 NamespacedNameV2            `json:"source_id"`
	SourceEpoch              core.Epoch                  `json:"source_epoch"`
	SourceSequence           uint64                      `json:"source_sequence"`
	EventID                  string                      `json:"event_id"`
	Evidence                 EvidenceRecordRefV2         `json:"evidence"`
	CandidateDigest          core.Digest                 `json:"candidate_digest"`
	PayloadDigest            core.Digest                 `json:"payload_digest"`
	ObservedUnixNano         int64                       `json:"observed_unix_nano"`
	EvidenceIngestedUnixNano int64                       `json:"evidence_ingested_unix_nano"`
	CreatedUnixNano          int64                       `json:"created_unix_nano"`
}

func ExecutionScopeDigestV2(scope core.ExecutionScope) (core.Digest, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-claim", RunClaimAssociationContractVersionV2, "ExecutionScopeV2", scope)
}
func RunIdentityDigestV2(run core.AgentRunRecord) (core.Digest, error) {
	if err := run.Validate(); err != nil {
		return "", err
	}
	identity := struct {
		ID      core.AgentRunID     `json:"run_id"`
		Scope   core.ExecutionScope `json:"scope"`
		Session string              `json:"session_ref"`
	}{run.ID, run.Scope, run.SessionRef}
	return core.CanonicalJSONDigest("praxis.runtime.run-claim", RunClaimAssociationContractVersionV2, "RunIdentityV2", identity)
}
func RunClaimAssociationIDV2(runID core.AgentRunID, record EvidenceRecordRefV2) (string, error) {
	if validateEvidenceIDV2(string(runID)) != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonRunClaimUnverified, "run id is invalid")
	}
	if err := record.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.run-claim", RunClaimAssociationContractVersionV2, "RunClaimAssociationIdentityV2", struct {
		RunID  core.AgentRunID     `json:"run_id"`
		Record EvidenceRecordRefV2 `json:"record"`
	}{runID, record})
	if err != nil {
		return "", err
	}
	return "association:" + string(digest), nil
}
func (f RunClaimAssociationFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.run-claim", RunClaimAssociationContractVersionV2, "RunClaimAssociationFactV2", f)
}
func (f RunClaimAssociationFactV2) Validate() error {
	if f.ContractVersion != RunClaimAssociationContractVersionV2 || validateEvidenceIDV2(f.ID) != nil || validateEvidenceIDV2(string(f.RunID)) != nil || f.Revision != 1 || f.State != RunClaimAssociatedV2 || f.RunRevisionAtAssociation == 0 || f.SourceEpoch == 0 || f.SourceSequence == 0 || f.ObservedUnixNano <= 0 || f.EvidenceIngestedUnixNano < f.ObservedUnixNano || f.CreatedUnixNano != f.EvidenceIngestedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunClaimUnverified, "complete create-once run claim association is required")
	}
	if err := f.ExecutionScope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := ExecutionScopeDigestV2(f.ExecutionScope)
	if err != nil {
		return err
	}
	if f.ExecutionScopeDigest != scopeDigest || f.LineagePlanDigest != f.ExecutionScope.Lineage.PlanDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "association scope or lineage digest drifted")
	}
	if err := ValidateNamespacedNameV2(f.SourceID); err != nil {
		return err
	}
	if validateEvidenceIDV2(f.RegistrationID) != nil || validateEvidenceIDV2(f.EventID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunClaimUnverified, "association source and event identity are required")
	}
	if err := f.Evidence.Validate(); err != nil {
		return err
	}
	expectedID, err := RunClaimAssociationIDV2(f.RunID, f.Evidence)
	if err != nil {
		return err
	}
	if f.ID != expectedID {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimConflict, "association id is not the canonical run+record identity")
	}
	for _, digest := range []core.Digest{f.RunIdentityDigest, f.CandidateDigest, f.PayloadDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	switch f.ClaimKind {
	case core.RunClaimCompleted, core.RunClaimCancelled, core.RunClaimFailed, core.RunClaimIndeterminate:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunClaimUnverified, "association claim kind is invalid")
	}
	return nil
}

type RunClaimAssociationPortV2 interface {
	CreateRunClaimAssociation(context.Context, RunClaimAssociationFactV2) (RunClaimAssociationFactV2, error)
	InspectRunClaimAssociation(context.Context, core.Digest, core.AgentRunID) (RunClaimAssociationFactV2, error)
}

// RunClaimIngestRequestV2 is the Application-facing claim command. ClaimKind
// is derived from the governed Evidence candidate/policy; callers cannot
// submit an Outcome or write the association Fact directly.
type RunClaimIngestRequestV2 struct {
	ExpectedRunRevision core.Revision            `json:"expected_run_revision"`
	Candidate           EvidenceEventCandidateV2 `json:"candidate"`
}

func (r RunClaimIngestRequestV2) Validate() error {
	if r.ExpectedRunRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunClaimUnverified, "claim ingest requires the expected active Run revision")
	}
	if err := r.Candidate.Validate(); err != nil {
		return err
	}
	if r.Candidate.TrustClass != EvidenceTrustClaim || r.Candidate.LedgerScope.Partition != EvidencePartitionRun || validateEvidenceIDV2(string(r.Candidate.LedgerScope.RunID)) != nil || r.Candidate.OwnerFact != nil || r.Candidate.HistoricalSource != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRunClaimUnverified, "claim ingest requires current governed run-partition Claim evidence")
	}
	return nil
}

type RunClaimIngestResultV2 struct {
	Run         core.AgentRunRecord       `json:"run"`
	Evidence    EvidenceLedgerRecordV2    `json:"evidence"`
	Association RunClaimAssociationFactV2 `json:"association"`
}

func (r RunClaimIngestResultV2) Validate() error {
	if err := r.Run.Validate(); err != nil {
		return err
	}
	if err := r.Evidence.Validate(); err != nil {
		return err
	}
	if err := r.Association.Validate(); err != nil {
		return err
	}
	candidateDigest, err := r.Evidence.Candidate.DigestV2()
	if err != nil {
		return err
	}
	runIdentity, err := RunIdentityDigestV2(r.Run)
	if err != nil {
		return err
	}
	if r.Run.ID != r.Association.RunID || runIdentity != r.Association.RunIdentityDigest || !SameExecutionScopeV2(r.Run.Scope, r.Association.ExecutionScope) || r.Association.Evidence != r.Evidence.Ref || r.Association.CandidateDigest != r.Evidence.CandidateDigest || r.Evidence.CandidateDigest != candidateDigest || r.Association.PayloadDigest != r.Evidence.Candidate.Payload.ContentDigest || r.Association.RegistrationID != r.Evidence.Candidate.RegistrationID || r.Association.SourceID != r.Evidence.Candidate.SourceID || r.Association.SourceEpoch != r.Evidence.Candidate.SourceEpoch || r.Association.SourceSequence != r.Evidence.Candidate.SourceSequence || r.Association.EventID != r.Evidence.Candidate.EventID || r.Association.ClaimKind != r.Evidence.Candidate.ClaimKind {
		return core.NewError(core.ErrorConflict, core.ReasonRunClaimConflict, "claim ingest result does not bind one exact Run, Evidence record and Association")
	}
	return nil
}

type RunClaimIngestGovernancePortV2 interface {
	IngestRunClaimV2(context.Context, RunClaimIngestRequestV2) (RunClaimIngestResultV2, error)
	InspectRunClaimV2(context.Context, core.ExecutionScope, core.AgentRunID) (RunClaimIngestResultV2, error)
}

type RunClaimIngestResultV3 struct {
	Certification RunSettlementPlanCertificationAssociationV3 `json:"plan_certification"`
	Plan          RunSettlementPlanLifecycleRefV3             `json:"plan"`
	Run           core.AgentRunRecord                         `json:"run"`
	Evidence      EvidenceLedgerRecordV2                      `json:"evidence"`
	Association   RunClaimAssociationFactV2                   `json:"association"`
}

func (r RunClaimIngestResultV3) Validate() error {
	if err := r.Certification.Validate(); err != nil {
		return err
	}
	if err := r.Plan.Validate(); err != nil {
		return err
	}
	legacy := RunClaimIngestResultV2{Run: r.Run, Evidence: r.Evidence, Association: r.Association}
	if err := legacy.Validate(); err != nil {
		return err
	}
	identity, _ := RunIdentityDigestV2(r.Run)
	scopeDigest, scopeErr := ExecutionScopeDigestV2(r.Run.Scope)
	if scopeErr != nil || r.Certification.RunID != r.Run.ID || r.Certification.RunIdentityDigest != identity || r.Certification.Plan != r.Plan.RunSettlementPlanRefV2 || r.Plan.RunID != r.Run.ID || r.Plan.RunIdentityDigest != identity || r.Certification.ExecutionScopeDigest != scopeDigest || r.Plan.ExecutionScopeDigest != scopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonRunClaimUnverified, "claim result belongs to another certified Run")
	}
	return nil
}

// RunClaimIngestGovernancePortV3 is the certified Application-facing entry.
// V2 remains restricted legacy compatibility and never grants Run completion.
type RunClaimIngestGovernancePortV3 interface {
	IngestRunClaimV3(context.Context, RunClaimIngestRequestV2) (RunClaimIngestResultV3, error)
	InspectRunClaimV3(context.Context, core.ExecutionScope, core.AgentRunID) (RunClaimIngestResultV3, error)
}

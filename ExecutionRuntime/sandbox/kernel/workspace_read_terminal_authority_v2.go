package kernel

import (
	"context"
	"errors"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// WorkspaceReadAuthorizedTerminalV2 is minted only by the Kernel after it has
// consumed private IPC journal evidence. SQLite can inspect the sealed Fact,
// but no other production package can construct this capability.
type WorkspaceReadAuthorizedTerminalV2 struct {
	fact contract.WorkspaceReadTerminalFactV2
}

func (a WorkspaceReadAuthorizedTerminalV2) FactV2() (contract.WorkspaceReadTerminalFactV2, error) {
	if err := a.fact.Validate(); err != nil {
		return contract.WorkspaceReadTerminalFactV2{}, err
	}
	return a.fact, nil
}

type workspaceReadPostActualRepositoryV2 interface {
	ownerworkspaceread.PostActualRepositoryV2
	CreateOrInspectKernelTerminalV2(
		context.Context,
		WorkspaceReadAuthorizedTerminalV2,
	) (contract.WorkspaceReadTerminalFactV2, bool, error)
}

type workspaceReadOutcomeS2AuthorityV2 struct {
	proof contract.WorkspaceReadOutcomeS2ProofV2
}

func authorizeWorkspaceReadOutcomeS2V2(
	qualification contract.WorkspaceReadExecutionQualificationV2,
	association runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1,
	publication contract.WorkspaceReadCommandPublicationV2,
	ownerCurrent contract.WorkspaceReadCommandOwnerCurrentV2,
	workspace contract.WorkspaceView,
	runtimeCurrent runtimeports.CurrentOperationDispatchEnforcementV4,
	journalEvidence workspaceReadPhysicalJournalEvidenceV2,
	observation contract.Ref,
	providerReceipt contract.WorkspaceReadReceiptBindingV1,
	s2CheckedUnixNano int64,
) (workspaceReadOutcomeS2AuthorityV2, error) {
	if err := qualification.Validate(); err != nil {
		return workspaceReadOutcomeS2AuthorityV2{}, err
	}
	now := time.Unix(0, s2CheckedUnixNano)
	if association.ValidateCurrent(qualification.Association, now) != nil ||
		publication.ValidateShape() != nil || publication.Meta.ValidateCurrent(now) != nil ||
		ownerCurrent.ValidateCurrent(now) != nil ||
		workspace.ValidateCurrent(now) != nil || workspace.Lease.ValidateCurrent(now) != nil ||
		runtimeCurrent.Validate() != nil ||
		s2CheckedUnixNano < runtimeCurrent.CheckedUnixNano ||
		!now.Before(time.Unix(0, runtimeCurrent.ExpiresUnixNano)) ||
		!now.Before(time.Unix(0, qualification.ExpiresUnixNano)) ||
		providerReceipt.Validate() != nil ||
		providerReceipt.CheckedUnixNano > s2CheckedUnixNano ||
		!now.Before(time.Unix(0, providerReceipt.ExpiresUnixNano)) {
		return workspaceReadOutcomeS2AuthorityV2{}, errors.New("workspace read outcome S2 authority is incomplete")
	}
	journal, err := journalEvidence.JournalV2()
	if err != nil {
		return workspaceReadOutcomeS2AuthorityV2{}, err
	}
	leaseDigest, err := contract.WorkspaceReadRuntimeLeaseDigestV2(workspace.Lease)
	if err != nil {
		return workspaceReadOutcomeS2AuthorityV2{}, err
	}
	proof, err := contract.SealWorkspaceReadOutcomeS2ProofV2(contract.WorkspaceReadOutcomeS2ProofV2{
		Qualification: qualification.Ref, OriginAttempt: qualification.OriginAttempt,
		Association: association.Ref, CommandPublication: publication.Meta.Ref(),
		CommandOwnerCurrent: ownerCurrent.Meta.Ref(), WorkspaceView: workspace.Meta.Ref(),
		WorkspaceLeaseDigest: leaseDigest, RuntimeCurrentDigest: runtimeCurrent.Digest,
		Journal: journal, Observation: observation, ProviderReceipt: providerReceipt,
		S2CheckedUnixNano: s2CheckedUnixNano,
	})
	if err != nil {
		return workspaceReadOutcomeS2AuthorityV2{}, err
	}
	if err = proof.ValidateQualificationV2(qualification); err != nil {
		return workspaceReadOutcomeS2AuthorityV2{}, err
	}
	if publication.Command != qualification.Command ||
		publication.Semantic.Workspace.Meta.Ref() != qualification.WorkspaceView ||
		ownerCurrent.Command != qualification.Command ||
		ownerCurrent.Publication != qualification.CommandPublication ||
		ownerCurrent.WorkspaceView != qualification.WorkspaceView ||
		workspace.Meta.Ref() != qualification.WorkspaceView ||
		runtimeCurrent.Digest != qualification.ExpectedRuntimeCurrentDigest {
		return workspaceReadOutcomeS2AuthorityV2{}, sandboxports.ErrConflict
	}
	return workspaceReadOutcomeS2AuthorityV2{proof: proof}, nil
}

func buildWorkspaceReadObservedTerminalV2(
	qualification contract.WorkspaceReadExecutionQualificationV2,
	journalEvidence workspaceReadPhysicalJournalEvidenceV2,
	s2 workspaceReadOutcomeS2AuthorityV2,
	outcomeCheckedUnixNano int64,
	recordedUnixNano int64,
) (WorkspaceReadAuthorizedTerminalV2, error) {
	journal, err := journalEvidence.JournalV2()
	if err != nil {
		return WorkspaceReadAuthorizedTerminalV2{}, err
	}
	fact, err := contract.SealWorkspaceReadTerminalFactV2(contract.WorkspaceReadTerminalFactV2{
		Qualification: qualification.Ref, OriginAttempt: qualification.OriginAttempt,
		RuntimeAttempt: qualification.RuntimeAttempt, RuntimeAttemptDigest: qualification.RuntimeAttemptDigest,
		ActualRequestDigest: qualification.ActualRequestDigest, Journal: journal,
		Outcome:                contract.WorkspaceReadTerminalObservedV2,
		Observed:               &contract.WorkspaceReadObservedTerminalV2{S2Proof: s2.proof},
		OutcomeCheckedUnixNano: outcomeCheckedUnixNano, RecordedUnixNano: recordedUnixNano,
		QualificationExpiresUnixNano: qualification.ExpiresUnixNano,
	})
	if err != nil {
		return WorkspaceReadAuthorizedTerminalV2{}, err
	}
	if err = s2.proof.ValidateQualificationV2(qualification); err != nil ||
		fact.Observed == nil || fact.Observed.S2Proof != s2.proof ||
		fact.Journal != journal {
		return WorkspaceReadAuthorizedTerminalV2{}, sandboxports.ErrConflict
	}
	return WorkspaceReadAuthorizedTerminalV2{fact: fact}, nil
}

type workspaceReadIndeterminateAuthorityV2 struct {
	evidence contract.WorkspaceReadIndeterminateEvidenceV2
}

func authorizeWorkspaceReadIndeterminateV2(
	qualification contract.WorkspaceReadExecutionQualificationV2,
	journalEvidence workspaceReadPhysicalJournalEvidenceV2,
	errorClass contract.WorkspaceReadIndeterminateErrorClassV2,
	errorDigest string,
	checkedUnixNano int64,
) (workspaceReadIndeterminateAuthorityV2, error) {
	if err := qualification.Validate(); err != nil {
		return workspaceReadIndeterminateAuthorityV2{}, err
	}
	journal, err := journalEvidence.JournalV2()
	if err != nil {
		return workspaceReadIndeterminateAuthorityV2{}, err
	}
	evidence, err := contract.SealWorkspaceReadIndeterminateEvidenceV2(
		qualification.Ref,
		qualification.OriginAttempt,
		journal,
		errorClass,
		errorDigest,
		checkedUnixNano,
	)
	if err != nil {
		return workspaceReadIndeterminateAuthorityV2{}, err
	}
	return workspaceReadIndeterminateAuthorityV2{evidence: evidence}, nil
}

func buildWorkspaceReadIndeterminateTerminalV2(
	qualification contract.WorkspaceReadExecutionQualificationV2,
	journalEvidence workspaceReadPhysicalJournalEvidenceV2,
	unknown workspaceReadIndeterminateAuthorityV2,
	outcomeCheckedUnixNano int64,
	recordedUnixNano int64,
) (WorkspaceReadAuthorizedTerminalV2, error) {
	journal, err := journalEvidence.JournalV2()
	if err != nil {
		return WorkspaceReadAuthorizedTerminalV2{}, err
	}
	fact, err := contract.SealWorkspaceReadTerminalFactV2(contract.WorkspaceReadTerminalFactV2{
		Qualification: qualification.Ref, OriginAttempt: qualification.OriginAttempt,
		RuntimeAttempt: qualification.RuntimeAttempt, RuntimeAttemptDigest: qualification.RuntimeAttemptDigest,
		ActualRequestDigest: qualification.ActualRequestDigest, Journal: journal,
		Outcome:                contract.WorkspaceReadTerminalIndeterminateV2,
		Indeterminate:          &contract.WorkspaceReadIndeterminateTerminalV2{Evidence: unknown.evidence},
		OutcomeCheckedUnixNano: outcomeCheckedUnixNano, RecordedUnixNano: recordedUnixNano,
		QualificationExpiresUnixNano: qualification.ExpiresUnixNano,
	})
	if err != nil {
		return WorkspaceReadAuthorizedTerminalV2{}, err
	}
	if fact.Indeterminate == nil || fact.Indeterminate.Evidence != unknown.evidence ||
		fact.Journal != journal {
		return WorkspaceReadAuthorizedTerminalV2{}, sandboxports.ErrConflict
	}
	return WorkspaceReadAuthorizedTerminalV2{fact: fact}, nil
}

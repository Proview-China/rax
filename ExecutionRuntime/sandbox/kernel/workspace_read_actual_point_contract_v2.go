package kernel

import (
	"context"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// workspaceReadPhysicalJournalEvidenceV2 is minted only inside this package
// after strict Data Plane IPC response validation. Its source journal cannot be
// supplied or replaced by a caller in Kernel, SQLite, or another Sandbox
// package.
type workspaceReadPhysicalJournalEvidenceV2 struct {
	journal contract.WorkspaceReadPhysicalJournalRefV2
}

func authorizeWorkspaceReadPhysicalJournalEvidenceV2(
	journal contract.WorkspaceReadPhysicalJournalRefV2,
) (workspaceReadPhysicalJournalEvidenceV2, error) {
	if err := journal.Validate(); err != nil {
		return workspaceReadPhysicalJournalEvidenceV2{}, err
	}
	return workspaceReadPhysicalJournalEvidenceV2{journal: journal}, nil
}

func (e workspaceReadPhysicalJournalEvidenceV2) JournalV2() (contract.WorkspaceReadPhysicalJournalRefV2, error) {
	if err := e.journal.Validate(); err != nil {
		return contract.WorkspaceReadPhysicalJournalRefV2{}, err
	}
	return e.journal, nil
}

// workspaceReadActualPointPreparationV2 exposes only the exact identity of a
// sealed Data Plane execute request. The concrete handle remains private to
// the adapter that created it and cannot itself dispatch.
type WorkspaceReadActualPointPreparationV2 interface {
	ActualRequestDigestV2() string
	ActualPayloadDigestV2() string
	ActualAttemptIDV2() string
	ActualExpiresUnixNanoV2() int64
}

type WorkspaceReadActualPointResultV1 struct {
	File              contract.Ref
	Content           string
	ContentDigest     string
	StartByte         uint64
	ReturnedBytes     uint64
	TotalBytes        uint64
	Complete          bool
	ProviderS1Checked bool
	ProviderS2Checked bool
	PhysicalReadCount uint64
	ProviderReceipt   contract.WorkspaceReadReceiptBindingV1
	Journal           contract.WorkspaceReadPhysicalJournalRefV2
	JournalEvidence   workspaceReadPhysicalJournalEvidenceV2
}

// workspaceReadActualPointInspectionV2 is historical evidence. A nil Result
// means either Started-without-Complete or a legacy Completed record without
// the sealed request body needed to validate a Provider result.
type WorkspaceReadActualPointInspectionV2 struct {
	Journal         contract.WorkspaceReadPhysicalJournalRefV2
	JournalEvidence workspaceReadPhysicalJournalEvidenceV2
	Result          *WorkspaceReadActualPointResultV1
}

// workspaceReadActualPointV1 remains only as a compatibility shape. Production
// workspace-read composition must require V2 before a physical boundary.
type WorkspaceReadActualPointV1 interface {
	ReadWorkspaceFileV1(context.Context, sandboxports.WorkspaceReadActualPointRequestV1) (WorkspaceReadActualPointResultV1, error)
}

type WorkspaceReadActualPointV2 interface {
	WorkspaceReadActualPointV1
	PrepareWorkspaceReadV2(context.Context, sandboxports.WorkspaceReadActualPointRequestV1) (WorkspaceReadActualPointPreparationV2, error)
	DispatchPreparedWorkspaceReadV2(context.Context, WorkspaceReadActualPointPreparationV2) (WorkspaceReadActualPointResultV1, error)
	InspectWorkspaceReadJournalV2(context.Context, contract.WorkspaceReadExecutionQualificationV2) (WorkspaceReadActualPointInspectionV2, error)
}

// errWorkspaceReadPhysicalJournalNotFoundV2 is returned only by the dedicated
// historical journal lookup. It never authorizes another dispatch.
var ErrWorkspaceReadPhysicalJournalNotFoundV2 = errors.New("workspace read physical journal is absent for the exact Qualification")

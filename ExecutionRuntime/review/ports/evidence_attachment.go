package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// EvidenceAttachmentStoreV1 is kept separate from StoreV1 so Verdict Owner
// implementations cannot acquire an Evidence attachment mutation implicitly.
type EvidenceAttachmentStoreV1 interface {
	CreateEvidenceAttachmentV1(context.Context, CreateEvidenceAttachmentMutationV1) (contract.EvidenceAttachmentV1, error)
	InspectEvidenceAttachmentExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.EvidenceAttachmentV1, error)
	InspectEvidenceAttachmentByIdempotencyV1(context.Context, core.TenantID, string) (contract.EvidenceAttachmentV1, error)
}

type CreateEvidenceAttachmentMutationV1 struct {
	Attachment      contract.EvidenceAttachmentV1 `json:"attachment"`
	CheckedUnixNano int64                         `json:"checked_unix_nano"`
}

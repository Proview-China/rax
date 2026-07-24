package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// AutoReviewerOutputSchemaReaderV1 is an exact immutable content reader. It
// exposes no publisher: this V1 schema is a Review release asset, while Rubric
// publication/revocation remains the separate currentness boundary.
type AutoReviewerOutputSchemaReaderV1 interface {
	InspectAutoReviewerOutputSchemaV1(context.Context, runtimeports.SchemaRefV2) (contract.AutoReviewerOutputSchemaDocumentV1, error)
}

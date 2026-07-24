package reviewer

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// BuiltinOutputSchemaReaderV1 publishes only the immutable schema compiled
// into the Review release. It has no runtime registration or mutable map.
type BuiltinOutputSchemaReaderV1 struct {
	document contract.AutoReviewerOutputSchemaDocumentV1
}

func NewBuiltinOutputSchemaReaderV1() (*BuiltinOutputSchemaReaderV1, error) {
	document, err := contract.BuiltinAutoReviewerOutputSchemaDocumentV1()
	if err != nil {
		return nil, err
	}
	return &BuiltinOutputSchemaReaderV1{document: document}, nil
}

func (r *BuiltinOutputSchemaReaderV1) InspectAutoReviewerOutputSchemaV1(ctx context.Context, ref runtimeports.SchemaRefV2) (contract.AutoReviewerOutputSchemaDocumentV1, error) {
	if r == nil {
		return contract.AutoReviewerOutputSchemaDocumentV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "auto reviewer output schema reader is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return contract.AutoReviewerOutputSchemaDocumentV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "auto reviewer output schema read was cancelled")
	}
	if err := ref.Validate(); err != nil {
		return contract.AutoReviewerOutputSchemaDocumentV1{}, err
	}
	if ref != r.document.Schema {
		return contract.AutoReviewerOutputSchemaDocumentV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "auto reviewer output schema exact ref is not present")
	}
	return r.document.Clone(), nil
}

var _ reviewport.AutoReviewerOutputSchemaReaderV1 = (*BuiltinOutputSchemaReaderV1)(nil)

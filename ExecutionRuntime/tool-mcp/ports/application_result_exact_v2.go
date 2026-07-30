package ports

import (
	"context"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// ApplicationResultExactRecordV2 is the immutable Application request/result
// pair retained by the Tool-owned exact reference store. It is not a new
// Application fact and grants no Application mutation capability.
type ApplicationResultExactRecordV2 struct {
	Request applicationcontract.SingleCallToolActionRequestV2 `json:"request"`
	Result  applicationcontract.SingleCallToolActionResultV2  `json:"result"`
}

func (r ApplicationResultExactRecordV2) ValidateForKey(key applicationcontract.SingleCallToolActionInspectKeyV2) error {
	if err := key.Validate(); err != nil {
		return err
	}
	if err := r.Request.Validate(); err != nil {
		return err
	}
	expected, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(r.Request)
	if err != nil {
		return err
	}
	if expected != key {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Application V2 result record belongs to another request")
	}
	checked := time.Unix(0, r.Result.Coordinate.AssociationCheckedUnixNano)
	if err = r.Result.ValidateCurrentFor(r.Request, checked); err != nil {
		return err
	}
	return nil
}

type ApplicationResultExactReaderV2 interface {
	InspectApplicationResultExactV2(context.Context, applicationcontract.SingleCallToolActionResultRefV2) (ApplicationResultExactRecordV2, error)
}

package control

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationDispatchPermitFactV4 is the Runtime Operation Effect Owner's
// persisted record. The public alias is read-only to Application code; only
// this raw owner Port can create or advance it.
type OperationDispatchPermitFactV4 = ports.OperationDispatchRecordV4

type IssueOperationPermitRequestV4 struct {
	Operation              ports.OperationSubjectV3                 `json:"operation"`
	EffectID               core.EffectIntentID                      `json:"effect_id"`
	ExpectedEffectRevision core.Revision                            `json:"expected_effect_revision"`
	Permit                 ports.OperationDispatchPermitV4          `json:"permit"`
	Fence                  core.ExecutionFence                      `json:"fence"`
	ReviewAuthorization    ports.OperationReviewAuthorizationFactV4 `json:"review_authorization"`
}

type IssueOperationPermitResultV4 struct {
	Effect OperationEffectFactV3         `json:"effect"`
	Permit OperationDispatchPermitFactV4 `json:"permit"`
}

type BeginOperationDispatchRequestV4 struct {
	Operation                  ports.OperationSubjectV3                `json:"operation"`
	EffectID                   core.EffectIntentID                     `json:"effect_id"`
	ExpectedEffectRevision     core.Revision                           `json:"expected_effect_revision"`
	PermitID                   string                                  `json:"permit_id"`
	ExpectedPermitFactRevision core.Revision                           `json:"expected_permit_fact_revision"`
	AdmissionDigest            core.Digest                             `json:"admission_digest"`
	ReviewAuthorization        ports.OperationReviewAuthorizationRefV4 `json:"review_authorization"`
}

// OperationEffectDispatchFactPortV4 is a raw single-Owner primitive. It is not
// an Application governance entry point.
type OperationEffectDispatchFactPortV4 interface {
	InspectOperationEffectV3(context.Context, ports.OperationSubjectV3, core.EffectIntentID) (OperationEffectFactV3, error)
	IssueOperationDispatchPermitV4(context.Context, IssueOperationPermitRequestV4) (IssueOperationPermitResultV4, error)
	InspectOperationDispatchPermitV4(context.Context, ports.OperationSubjectV3, string) (OperationDispatchPermitFactV4, error)
	BeginOperationDispatchV4(context.Context, BeginOperationDispatchRequestV4) (OperationDispatchPermitFactV4, error)
}

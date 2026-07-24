package control

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationDispatchPermitFactV5 = ports.OperationDispatchRecordV5

type IssueOperationPermitRequestV5 struct {
	Operation              ports.OperationSubjectV3                 `json:"operation"`
	EffectID               core.EffectIntentID                      `json:"effect_id"`
	ExpectedEffectRevision core.Revision                            `json:"expected_effect_revision"`
	Permit                 ports.OperationDispatchPermitV5          `json:"permit"`
	Fence                  core.ExecutionFence                      `json:"fence"`
	ReviewAuthorization    ports.OperationReviewAuthorizationFactV5 `json:"review_authorization"`
}

type IssueOperationPermitResultV5 struct {
	Effect OperationEffectFactV3         `json:"effect"`
	Permit OperationDispatchPermitFactV5 `json:"permit"`
}

type BeginOperationDispatchRequestV5 struct {
	Operation                  ports.OperationSubjectV3                  `json:"operation"`
	EffectID                   core.EffectIntentID                       `json:"effect_id"`
	ExpectedEffectRevision     core.Revision                             `json:"expected_effect_revision"`
	PermitID                   string                                    `json:"permit_id"`
	ExpectedPermitFactRevision core.Revision                             `json:"expected_permit_fact_revision"`
	AdmissionDigest            core.Digest                               `json:"admission_digest"`
	ReviewAuthorization        ports.OperationReviewAuthorizationRefV5   `json:"review_authorization"`
	AuthorizationBasis         ports.OperationReviewAuthorizationBasisV5 `json:"review_authorization_basis"`
}

type OperationEffectDispatchFactPortV5 interface {
	InspectOperationEffectV3(context.Context, ports.OperationSubjectV3, core.EffectIntentID) (OperationEffectFactV3, error)
	IssueOperationDispatchPermitV5(context.Context, IssueOperationPermitRequestV5) (IssueOperationPermitResultV5, error)
	InspectOperationDispatchPermitV5(context.Context, ports.OperationSubjectV3, string) (OperationDispatchPermitFactV5, error)
	BeginOperationDispatchV5(context.Context, BeginOperationDispatchRequestV5) (OperationDispatchPermitFactV5, error)
}

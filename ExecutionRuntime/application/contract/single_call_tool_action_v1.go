package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	SingleCallToolActionContractVersionV1 = "praxis.application.single-call-tool-action/v1"
	SingleCallToolActionStepKindV1        = runtimeports.NamespacedNameV2("praxis.application/single-call-tool-action")
	SingleCallSessionSourceKindV1         = runtimeports.NamespacedNameV2("praxis.harness/session")
	SingleCallTurnSourceKindV1            = runtimeports.NamespacedNameV2("praxis.harness/turn")
	SingleCallParentFrameSourceKindV1     = runtimeports.NamespacedNameV2("praxis.context/parent-frame-current-v1")
	SingleCallSessionWaitingActionV1      = "waiting_action"
	MaxSingleCallCoordinateIDBytesV1      = 512
)

type SingleCallWorkflowCoordinateV1 struct {
	WorkflowContractVersion string                        `json:"workflow_contract_version"`
	PlanID                  string                        `json:"plan_id"`
	PlanRevision            core.Revision                 `json:"plan_revision"`
	PlanDigest              core.Digest                   `json:"plan_digest"`
	JournalID               string                        `json:"journal_id"`
	JournalRevision         core.Revision                 `json:"journal_revision"`
	JournalDigest           core.Digest                   `json:"journal_digest"`
	StepID                  string                        `json:"step_id"`
	StepKind                runtimeports.NamespacedNameV2 `json:"step_kind"`
	StepDescriptor          StepDescriptorRefV2           `json:"step_descriptor"`
	WorkflowAttempt         uint32                        `json:"workflow_attempt"`
}

func (c SingleCallWorkflowCoordinateV1) Validate() error {
	if c.WorkflowContractVersion != WorkflowContractVersionV2 || !validSingleCallIDV1(c.PlanID) || c.PlanRevision == 0 || !validSingleCallIDV1(c.JournalID) || c.JournalRevision == 0 || !validSingleCallIDV1(c.StepID) || c.StepKind != SingleCallToolActionStepKindV1 || c.WorkflowAttempt == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call workflow coordinate is incomplete")
	}
	for _, digest := range []core.Digest{c.PlanDigest, c.JournalDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return c.StepDescriptor.Validate(c.StepKind)
}

type SingleCallRunCoordinateV1 struct {
	RunID    core.AgentRunID `json:"run_id"`
	Revision core.Revision   `json:"run_revision"`
	Digest   core.Digest     `json:"run_digest"`
}

func (c SingleCallRunCoordinateV1) Validate() error {
	if !validSingleCallIDV1(string(c.RunID)) || c.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call Run coordinate is incomplete")
	}
	return c.Digest.Validate()
}

type SingleCallSessionCoordinateV1 struct {
	ID              string        `json:"session_id"`
	Revision        core.Revision `json:"session_revision"`
	Digest          core.Digest   `json:"session_digest"`
	Phase           string        `json:"phase"`
	CheckedUnixNano int64         `json:"checked_unix_nano"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (c SingleCallSessionCoordinateV1) Validate() error {
	if !validSingleCallIDV1(c.ID) || c.Revision == 0 || c.Phase != SingleCallSessionWaitingActionV1 || c.CheckedUnixNano <= 0 || c.ExpiresUnixNano <= c.CheckedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call Session coordinate is incomplete")
	}
	return c.Digest.Validate()
}

type SingleCallSessionApplicabilitySourceCoordinateV1 struct {
	Kind     runtimeports.NamespacedNameV2 `json:"kind"`
	ID       string                        `json:"id"`
	Revision core.Revision                 `json:"revision"`
	Digest   core.Digest                   `json:"digest"`
}

func (c SingleCallSessionApplicabilitySourceCoordinateV1) Validate() error {
	if c.Kind != SingleCallSessionSourceKindV1 || !validSingleCallIDV1(c.ID) || c.Revision == 0 || c.Digest.Validate() != nil || c.ID != "session:"+string(c.Digest) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call Session applicability source is invalid")
	}
	_, err := c.CanonicalDigestV1()
	return err
}

func (c SingleCallSessionApplicabilitySourceCoordinateV1) CanonicalDigestV1() (core.Digest, error) {
	if c.Kind != SingleCallSessionSourceKindV1 || !validSingleCallIDV1(c.ID) || c.Revision == 0 || c.Digest.Validate() != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call Session applicability source is incomplete")
	}
	return core.CanonicalJSONDigest("praxis.application.single-call-session-source", SingleCallToolActionContractVersionV1, "SingleCallSessionApplicabilitySourceCoordinateV1", c)
}

type SingleCallTurnCoordinateV1 struct {
	ID       string        `json:"turn_id"`
	Ordinal  uint32        `json:"ordinal"`
	Revision core.Revision `json:"turn_revision"`
	Digest   core.Digest   `json:"turn_digest"`
}

func (c SingleCallTurnCoordinateV1) Validate() error {
	if !validSingleCallIDV1(c.ID) || c.Ordinal == 0 || c.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call Turn coordinate is incomplete")
	}
	return c.Digest.Validate()
}

type SingleCallTurnApplicabilitySourceCoordinateV1 struct {
	Kind     runtimeports.NamespacedNameV2 `json:"kind"`
	ID       string                        `json:"id"`
	Revision core.Revision                 `json:"revision"`
	Digest   core.Digest                   `json:"digest"`
}

func (c SingleCallTurnApplicabilitySourceCoordinateV1) Validate() error {
	if c.Kind != SingleCallTurnSourceKindV1 || !validSingleCallIDV1(c.ID) || c.Revision == 0 || c.Digest.Validate() != nil || c.ID != "turn:"+string(c.Digest) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call Turn applicability source is invalid")
	}
	_, err := c.CanonicalDigestV1()
	return err
}

func (c SingleCallTurnApplicabilitySourceCoordinateV1) CanonicalDigestV1() (core.Digest, error) {
	if c.Kind != SingleCallTurnSourceKindV1 || !validSingleCallIDV1(c.ID) || c.Revision == 0 || c.Digest.Validate() != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call Turn applicability source is incomplete")
	}
	return core.CanonicalJSONDigest("praxis.application.single-call-turn-source", SingleCallToolActionContractVersionV1, "SingleCallTurnApplicabilitySourceCoordinateV1", c)
}

type SingleCallPendingActionCoordinateV1 struct {
	ActionRef               string                        `json:"action_ref"`
	RequestDigest           core.Digest                   `json:"request_digest"`
	Capability              runtimeports.CapabilityNameV2 `json:"capability"`
	PayloadSchema           runtimeports.SchemaRefV2      `json:"payload_schema"`
	PayloadDigest           core.Digest                   `json:"payload_digest"`
	SourceCandidateID       string                        `json:"source_candidate_id"`
	SourceCandidateRevision core.Revision                 `json:"source_candidate_revision"`
	SourceCandidateDigest   core.Digest                   `json:"source_candidate_digest"`
	ProjectionDigest        core.Digest                   `json:"projection_digest"`
}

func (c SingleCallPendingActionCoordinateV1) Validate() error {
	if !validSingleCallIDV1(c.ActionRef) || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(c.Capability)) != nil || !validSingleCallIDV1(c.SourceCandidateID) || c.SourceCandidateRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call PendingAction coordinate is incomplete")
	}
	if err := c.PayloadSchema.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{c.RequestDigest, c.PayloadDigest, c.SourceCandidateDigest, c.ProjectionDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type SingleCallObservationCoordinateV1 struct {
	ProjectionContractVersion string                           `json:"projection_contract_version"`
	ProjectionID              string                           `json:"projection_id"`
	ProjectionRevision        core.Revision                    `json:"projection_revision"`
	ProjectionDigest          core.Digest                      `json:"projection_digest"`
	InvocationID              string                           `json:"invocation_id"`
	InvocationDigest          core.Digest                      `json:"invocation_digest"`
	ObservationDigest         core.Digest                      `json:"observation_digest"`
	SourceResponseID          string                           `json:"source_response_id,omitempty"`
	SourceSequence            uint64                           `json:"source_sequence"`
	Evidence                  runtimeports.EvidenceRecordRefV2 `json:"evidence"`
	CallCount                 uint32                           `json:"call_count"`
}

func (c SingleCallObservationCoordinateV1) Validate() error {
	if !validSingleCallIDV1(c.ProjectionContractVersion) || !validSingleCallIDV1(c.ProjectionID) || c.ProjectionRevision != 1 || !validSingleCallIDV1(c.InvocationID) || c.SourceSequence == 0 || c.CallCount != 1 || len(c.SourceResponseID) > MaxSingleCallCoordinateIDBytesV1 || strings.TrimSpace(c.SourceResponseID) != c.SourceResponseID {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call Observation coordinate is incomplete or not N=1")
	}
	for _, digest := range []core.Digest{c.ProjectionDigest, c.InvocationDigest, c.ObservationDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return c.Evidence.Validate()
}

type SingleCallAssemblyCoordinateV1 struct {
	GenerationID       string                                         `json:"generation_id"`
	GenerationRevision core.Revision                                  `json:"generation_revision"`
	GenerationDigest   core.Digest                                    `json:"generation_digest"`
	BindingAssociation runtimeports.GenerationBindingAssociationRefV1 `json:"binding_association"`
	ToolProvider       runtimeports.ProviderBindingRefV2              `json:"tool_provider"`
}

func (c SingleCallAssemblyCoordinateV1) Validate() error {
	if !validSingleCallIDV1(c.GenerationID) || c.GenerationRevision == 0 || c.GenerationDigest.Validate() != nil || c.BindingAssociation.Validate() != nil || c.ToolProvider.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call Assembly coordinate is incomplete")
	}
	return nil
}

type SingleCallParentFrameCoordinateV1 struct {
	FrameID            string        `json:"frame_id"`
	FrameRevision      core.Revision `json:"frame_revision"`
	FrameDigest        core.Digest   `json:"frame_digest"`
	GenerationID       string        `json:"generation_id"`
	GenerationRevision core.Revision `json:"generation_revision"`
	GenerationDigest   core.Digest   `json:"generation_digest"`
	ExpiresUnixNano    int64         `json:"expires_unix_nano"`
}

func (c SingleCallParentFrameCoordinateV1) Validate() error {
	if !validSingleCallIDV1(c.FrameID) || c.FrameRevision == 0 || !validSingleCallIDV1(c.GenerationID) || c.GenerationRevision == 0 || c.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call ParentFrame metadata coordinate is incomplete")
	}
	if err := c.FrameDigest.Validate(); err != nil {
		return err
	}
	return c.GenerationDigest.Validate()
}

type SingleCallParentFrameApplicabilitySourceCoordinateV1 struct {
	Kind     runtimeports.NamespacedNameV2 `json:"kind"`
	ID       string                        `json:"id"`
	Revision core.Revision                 `json:"revision"`
	Digest   core.Digest                   `json:"digest"`
}

func (c SingleCallParentFrameApplicabilitySourceCoordinateV1) Validate() error {
	if c.Kind != SingleCallParentFrameSourceKindV1 || !validSingleCallIDV1(c.ID) || c.Revision == 0 || c.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call ParentFrame applicability source is invalid")
	}
	_, err := c.CanonicalDigestV1()
	return err
}

func (c SingleCallParentFrameApplicabilitySourceCoordinateV1) CanonicalDigestV1() (core.Digest, error) {
	if c.Kind != SingleCallParentFrameSourceKindV1 || !validSingleCallIDV1(c.ID) || c.Revision == 0 || c.Digest.Validate() != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call ParentFrame applicability source is incomplete")
	}
	return core.CanonicalJSONDigest("praxis.application.single-call-parent-frame-source", SingleCallToolActionContractVersionV1, "SingleCallParentFrameApplicabilitySourceCoordinateV1", c)
}

type SingleCallToolActionRequestV1 struct {
	ContractVersion                string                                               `json:"contract_version"`
	ID                             string                                               `json:"id"`
	Revision                       core.Revision                                        `json:"revision"`
	Workflow                       SingleCallWorkflowCoordinateV1                       `json:"workflow"`
	ExecutionScope                 core.ExecutionScope                                  `json:"execution_scope"`
	ExecutionScopeDigest           core.Digest                                          `json:"execution_scope_digest"`
	Run                            SingleCallRunCoordinateV1                            `json:"run"`
	Session                        SingleCallSessionCoordinateV1                        `json:"session"`
	SessionApplicabilitySource     SingleCallSessionApplicabilitySourceCoordinateV1     `json:"session_applicability_source"`
	Turn                           SingleCallTurnCoordinateV1                           `json:"turn"`
	TurnApplicabilitySource        SingleCallTurnApplicabilitySourceCoordinateV1        `json:"turn_applicability_source"`
	PendingAction                  SingleCallPendingActionCoordinateV1                  `json:"pending_action"`
	Observation                    SingleCallObservationCoordinateV1                    `json:"observation"`
	Assembly                       SingleCallAssemblyCoordinateV1                       `json:"assembly"`
	Authority                      runtimeports.AuthorityBindingRefV2                   `json:"authority"`
	ParentFrame                    SingleCallParentFrameCoordinateV1                    `json:"parent_frame"`
	ParentFrameApplicabilitySource SingleCallParentFrameApplicabilitySourceCoordinateV1 `json:"parent_frame_applicability_source"`
	CreatedUnixNano                int64                                                `json:"created_unix_nano"`
	ExpiresUnixNano                int64                                                `json:"expires_unix_nano"`
	Digest                         core.Digest                                          `json:"digest"`
}

func SealSingleCallToolActionRequestV1(request SingleCallToolActionRequestV1) (SingleCallToolActionRequestV1, error) {
	request.ContractVersion = SingleCallToolActionContractVersionV1
	request.Revision = 1
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(request.ExecutionScope)
	if err != nil {
		return SingleCallToolActionRequestV1{}, err
	}
	request.ExecutionScopeDigest = scopeDigest
	request.ID = ""
	request.Digest = ""
	idDigest, err := request.subjectDigestV1()
	if err != nil {
		return SingleCallToolActionRequestV1{}, err
	}
	request.ID = "single-call/" + strings.TrimPrefix(string(idDigest), "sha256:")
	request.Digest, err = request.DigestV1()
	if err != nil {
		return SingleCallToolActionRequestV1{}, err
	}
	return request, request.Validate()
}

func (r SingleCallToolActionRequestV1) Validate() error {
	if err := r.validateShapeV1(); err != nil {
		return err
	}
	idDigest, err := r.subjectDigestV1()
	if err != nil {
		return err
	}
	if r.ID != "single-call/"+strings.TrimPrefix(string(idDigest), "sha256:") {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call request ID does not match its canonical subject")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call request digest drifted")
	}
	return nil
}

func (r SingleCallToolActionRequestV1) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.CreatedUnixNano || !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "single-call request is not current")
	}
	return nil
}

func (r SingleCallToolActionRequestV1) validateShapeV1() error {
	if r.ContractVersion != SingleCallToolActionContractVersionV1 || !validSingleCallIDV1(r.ID) || r.Revision != 1 || r.CreatedUnixNano <= 0 || r.ExpiresUnixNano <= r.CreatedUnixNano || time.Duration(r.ExpiresUnixNano-r.CreatedUnixNano) > runtimeports.MaxDispatchPermitTTL {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call request identity or bounded TTL is invalid")
	}
	if err := r.Workflow.Validate(); err != nil {
		return err
	}
	if err := r.ExecutionScope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(r.ExecutionScope)
	if err != nil || scopeDigest != r.ExecutionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "single-call request ExecutionScope digest drifted")
	}
	validators := []func() error{r.Run.Validate, r.Session.Validate, r.SessionApplicabilitySource.Validate, r.Turn.Validate, r.TurnApplicabilitySource.Validate, r.PendingAction.Validate, r.Observation.Validate, r.Assembly.Validate, r.Authority.Validate, r.ParentFrame.Validate, r.ParentFrameApplicabilitySource.Validate}
	for _, validate := range validators {
		if err := validate(); err != nil {
			return err
		}
	}
	if r.Run.RunID == "" || r.Assembly.GenerationID != r.ParentFrame.GenerationID || r.Assembly.GenerationRevision != r.ParentFrame.GenerationRevision || r.Assembly.GenerationDigest != r.ParentFrame.GenerationDigest || r.ParentFrameApplicabilitySource.ID != r.ParentFrame.FrameID || r.ExecutionScope.AuthorityEpoch != r.Authority.Epoch {
		return core.NewError(core.ErrorConflict, core.ReasonComponentMismatch, "single-call request owner coordinates belong to different subjects")
	}
	for _, upper := range []int64{r.Session.ExpiresUnixNano, r.ParentFrame.ExpiresUnixNano, r.Workflow.StepDescriptor.ExpiresUnixNano} {
		if r.ExpiresUnixNano > upper {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "single-call request exceeds an owner currentness boundary")
		}
	}
	return r.Digest.Validate()
}

func (r SingleCallToolActionRequestV1) subjectDigestV1() (core.Digest, error) {
	copy := r
	copy.ID = ""
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.single-call-tool-action", SingleCallToolActionContractVersionV1, "SingleCallToolActionRequestSubjectV1", copy)
}

func (r SingleCallToolActionRequestV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.single-call-tool-action", SingleCallToolActionContractVersionV1, "SingleCallToolActionRequestV1", copy)
}

type SingleCallToolResultCoordinateV1 struct {
	ID                      string                                `json:"result_id"`
	Revision                core.Revision                         `json:"result_revision"`
	Digest                  core.Digest                           `json:"result_digest"`
	ActionCoordinateDigest  core.Digest                           `json:"action_coordinate_digest"`
	ApplySettlementID       string                                `json:"apply_settlement_id"`
	ApplySettlementRevision core.Revision                         `json:"apply_settlement_revision"`
	ApplySettlementDigest   core.Digest                           `json:"apply_settlement_digest"`
	Settlement              runtimeports.OperationSettlementRefV4 `json:"settlement"`
	ResultSchema            runtimeports.SchemaRefV2              `json:"result_schema"`
	PayloadDigest           core.Digest                           `json:"payload_digest"`
	FinalizedUnixNano       int64                                 `json:"finalized_unix_nano"`
	ExpiresUnixNano         int64                                 `json:"expires_unix_nano"`
}

func (c SingleCallToolResultCoordinateV1) Validate() error {
	if !validSingleCallIDV1(c.ID) || c.Revision == 0 || !validSingleCallIDV1(c.ApplySettlementID) || c.ApplySettlementRevision == 0 || c.FinalizedUnixNano <= 0 || c.ExpiresUnixNano <= c.FinalizedUnixNano || c.Settlement.Validate() != nil || c.ResultSchema.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call ToolResult coordinate is incomplete")
	}
	for _, digest := range []core.Digest{c.Digest, c.ActionCoordinateDigest, c.ApplySettlementDigest, c.PayloadDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func SingleCallActionCoordinateDigestV1(request SingleCallToolActionRequestV1) (core.Digest, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.application.single-call-tool-action", SingleCallToolActionContractVersionV1, "SingleCallActionCoordinateV1", struct {
		Run           SingleCallRunCoordinateV1           `json:"run"`
		Session       SingleCallSessionCoordinateV1       `json:"session"`
		Turn          SingleCallTurnCoordinateV1          `json:"turn"`
		PendingAction SingleCallPendingActionCoordinateV1 `json:"pending_action"`
		Observation   SingleCallObservationCoordinateV1   `json:"observation"`
	}{request.Run, request.Session, request.Turn, request.PendingAction, request.Observation})
}

type SingleCallToolActionResultV1 struct {
	ContractVersion            string                                                   `json:"contract_version"`
	ID                         string                                                   `json:"id"`
	Revision                   core.Revision                                            `json:"revision"`
	RequestID                  string                                                   `json:"request_id"`
	RequestRevision            core.Revision                                            `json:"request_revision"`
	RequestDigest              core.Digest                                              `json:"request_digest"`
	ToolResult                 SingleCallToolResultCoordinateV1                         `json:"tool_result"`
	Inspection                 runtimeports.OperationInspectionSettlementRefV4          `json:"inspection"`
	Association                runtimeports.OperationSettlementEvidenceAssociationRefV4 `json:"association"`
	AssociationCheckedUnixNano int64                                                    `json:"association_checked_unix_nano"`
	ExpiresUnixNano            int64                                                    `json:"expires_unix_nano"`
	Digest                     core.Digest                                              `json:"digest"`
}

func SealSingleCallToolActionResultV1(result SingleCallToolActionResultV1, request SingleCallToolActionRequestV1, now time.Time) (SingleCallToolActionResultV1, error) {
	result.ContractVersion = SingleCallToolActionContractVersionV1
	result.Revision = 1
	result.RequestID = request.ID
	result.RequestRevision = request.Revision
	result.RequestDigest = request.Digest
	result.ID = ""
	result.Digest = ""
	idDigest, err := result.subjectDigestV1()
	if err != nil {
		return SingleCallToolActionResultV1{}, err
	}
	result.ID = "single-call-result/" + strings.TrimPrefix(string(idDigest), "sha256:")
	result.Digest, err = result.DigestV1()
	if err != nil {
		return SingleCallToolActionResultV1{}, err
	}
	return result, result.ValidateCurrentFor(request, now)
}

func (r SingleCallToolActionResultV1) ValidateCurrentFor(request SingleCallToolActionRequestV1, now time.Time) error {
	if err := request.ValidateCurrent(now); err != nil {
		return err
	}
	if r.ContractVersion != SingleCallToolActionContractVersionV1 || !validSingleCallIDV1(r.ID) || r.Revision != 1 || r.RequestID != request.ID || r.RequestRevision != request.Revision || r.RequestDigest != request.Digest || r.AssociationCheckedUnixNano < r.Inspection.CheckedUnixNano || r.AssociationCheckedUnixNano >= r.Inspection.ExpiresUnixNano || r.ExpiresUnixNano <= r.AssociationCheckedUnixNano || r.ExpiresUnixNano > request.ExpiresUnixNano || r.ExpiresUnixNano > r.Inspection.ExpiresUnixNano || r.ExpiresUnixNano > r.ToolResult.ExpiresUnixNano || now.IsZero() || now.UnixNano() < r.AssociationCheckedUnixNano || !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "single-call Result identity or currentness is incomplete")
	}
	if err := r.ToolResult.Validate(); err != nil {
		return err
	}
	if err := r.Inspection.Validate(now); err != nil {
		return err
	}
	if !runtimeports.SameExecutionScopeV2(r.Inspection.DomainResult.Operation.ExecutionScope, request.ExecutionScope) || r.Inspection.DomainResult.Operation.ExecutionScopeDigest != request.ExecutionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "single-call settlement belongs to another ExecutionScope")
	}
	if err := r.Association.Validate(); err != nil {
		return err
	}
	actionDigest, err := SingleCallActionCoordinateDigestV1(request)
	if err != nil || actionDigest != r.ToolResult.ActionCoordinateDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call ToolResult action coordinate drifted")
	}
	if !runtimeports.SameOperationSettlementRefV4(r.ToolResult.Settlement, r.Inspection.Settlement) || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(r.Association, r.Inspection.Association) {
		return core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "single-call Result does not bind the exact V4 closure")
	}
	idDigest, err := r.subjectDigestV1()
	if err != nil || r.ID != "single-call-result/"+strings.TrimPrefix(string(idDigest), "sha256:") {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call Result ID does not match its canonical subject")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call Result digest drifted")
	}
	return nil
}

func (r SingleCallToolActionResultV1) subjectDigestV1() (core.Digest, error) {
	copy := r
	copy.ID = ""
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.single-call-tool-action", SingleCallToolActionContractVersionV1, "SingleCallToolActionResultSubjectV1", copy)
}

func (r SingleCallToolActionResultV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.single-call-tool-action", SingleCallToolActionContractVersionV1, "SingleCallToolActionResultV1", copy)
}

type SingleCallToolActionResultRefV1 struct {
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	RequestID       string        `json:"request_id"`
	RequestRevision core.Revision `json:"request_revision"`
	RequestDigest   core.Digest   `json:"request_digest"`
}

func (r SingleCallToolActionResultV1) RefV1() SingleCallToolActionResultRefV1 {
	return SingleCallToolActionResultRefV1{ID: r.ID, Revision: r.Revision, Digest: r.Digest, RequestID: r.RequestID, RequestRevision: r.RequestRevision, RequestDigest: r.RequestDigest}
}

func (r SingleCallToolActionResultRefV1) Validate() error {
	if !validSingleCallIDV1(r.ID) || r.Revision != 1 || !validSingleCallIDV1(r.RequestID) || r.RequestRevision != 1 || r.Digest.Validate() != nil || r.RequestDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call Result ref is incomplete")
	}
	return nil
}

type SingleCallToolActionInputCurrentProjectionV1 struct {
	ContractVersion                string                                               `json:"contract_version"`
	RequestID                      string                                               `json:"request_id"`
	RequestRevision                core.Revision                                        `json:"request_revision"`
	RequestDigest                  core.Digest                                          `json:"request_digest"`
	ScopeDigest                    core.Digest                                          `json:"scope_digest"`
	Run                            SingleCallRunCoordinateV1                            `json:"run"`
	Session                        SingleCallSessionCoordinateV1                        `json:"session"`
	SessionApplicabilitySource     SingleCallSessionApplicabilitySourceCoordinateV1     `json:"session_applicability_source"`
	Turn                           SingleCallTurnCoordinateV1                           `json:"turn"`
	TurnApplicabilitySource        SingleCallTurnApplicabilitySourceCoordinateV1        `json:"turn_applicability_source"`
	PendingAction                  SingleCallPendingActionCoordinateV1                  `json:"pending_action"`
	Observation                    SingleCallObservationCoordinateV1                    `json:"observation"`
	Assembly                       SingleCallAssemblyCoordinateV1                       `json:"assembly"`
	Authority                      runtimeports.AuthorityBindingRefV2                   `json:"authority"`
	ParentFrame                    SingleCallParentFrameCoordinateV1                    `json:"parent_frame"`
	ParentFrameApplicabilitySource SingleCallParentFrameApplicabilitySourceCoordinateV1 `json:"parent_frame_applicability_source"`
	CheckedUnixNano                int64                                                `json:"checked_unix_nano"`
	ExpiresUnixNano                int64                                                `json:"expires_unix_nano"`
	Digest                         core.Digest                                          `json:"digest"`
}

func SealSingleCallToolActionInputCurrentProjectionV1(projection SingleCallToolActionInputCurrentProjectionV1, request SingleCallToolActionRequestV1, now time.Time) (SingleCallToolActionInputCurrentProjectionV1, error) {
	projection.ContractVersion = SingleCallToolActionContractVersionV1
	projection.RequestID = request.ID
	projection.RequestRevision = request.Revision
	projection.RequestDigest = request.Digest
	projection.ScopeDigest = request.ExecutionScopeDigest
	projection.Digest = ""
	digest, err := projection.DigestV1()
	if err != nil {
		return SingleCallToolActionInputCurrentProjectionV1{}, err
	}
	projection.Digest = digest
	return projection, projection.ValidateFor(request, now)
}

func (p SingleCallToolActionInputCurrentProjectionV1) ValidateFor(request SingleCallToolActionRequestV1, now time.Time) error {
	if err := request.ValidateCurrent(now); err != nil {
		return err
	}
	if p.ContractVersion != SingleCallToolActionContractVersionV1 || p.RequestID != request.ID || p.RequestRevision != request.Revision || p.RequestDigest != request.Digest || p.ScopeDigest != request.ExecutionScopeDigest || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ExpiresUnixNano > request.ExpiresUnixNano || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "single-call input current projection is stale or mismatched")
	}
	if p.Run != request.Run || p.Session != request.Session || p.SessionApplicabilitySource != request.SessionApplicabilitySource || p.Turn != request.Turn || p.TurnApplicabilitySource != request.TurnApplicabilitySource || p.PendingAction != request.PendingAction || p.Observation != request.Observation || p.Assembly != request.Assembly || p.Authority != request.Authority || p.ParentFrame != request.ParentFrame || p.ParentFrameApplicabilitySource != request.ParentFrameApplicabilitySource {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call input current projection contains owner drift")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call input current projection digest drifted")
	}
	return nil
}

func (p SingleCallToolActionInputCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.single-call-tool-action", SingleCallToolActionContractVersionV1, "SingleCallToolActionInputCurrentProjectionV1", copy)
}

type SingleCallToolActionCoordinationStateV1 string

const (
	SingleCallToolActionPreparedV1       SingleCallToolActionCoordinationStateV1 = "prepared"
	SingleCallToolActionDispatchIntentV1 SingleCallToolActionCoordinationStateV1 = "dispatch_intent"
	SingleCallToolActionWaitingInspectV1 SingleCallToolActionCoordinationStateV1 = "waiting_inspect"
	SingleCallToolActionCompletedV1      SingleCallToolActionCoordinationStateV1 = "completed"
)

type SingleCallToolActionCoordinationFactV1 struct {
	ContractVersion string                                  `json:"contract_version"`
	ID              string                                  `json:"id"`
	Revision        core.Revision                           `json:"revision"`
	State           SingleCallToolActionCoordinationStateV1 `json:"state"`
	StartClaimID    string                                  `json:"start_claim_id,omitempty"`
	Request         SingleCallToolActionRequestV1           `json:"request"`
	Result          *SingleCallToolActionResultRefV1        `json:"result,omitempty"`
	CreatedUnixNano int64                                   `json:"created_unix_nano"`
	UpdatedUnixNano int64                                   `json:"updated_unix_nano"`
	Digest          core.Digest                             `json:"digest"`
}

func NewSingleCallToolActionCoordinationFactV1(request SingleCallToolActionRequestV1, now time.Time) (SingleCallToolActionCoordinationFactV1, error) {
	if err := request.ValidateCurrent(now); err != nil {
		return SingleCallToolActionCoordinationFactV1{}, err
	}
	fact := SingleCallToolActionCoordinationFactV1{ContractVersion: SingleCallToolActionContractVersionV1, ID: request.ID, Revision: 1, State: SingleCallToolActionPreparedV1, Request: request, CreatedUnixNano: request.CreatedUnixNano, UpdatedUnixNano: request.CreatedUnixNano}
	return SealSingleCallToolActionCoordinationFactV1(fact)
}

func SealSingleCallToolActionCoordinationFactV1(fact SingleCallToolActionCoordinationFactV1) (SingleCallToolActionCoordinationFactV1, error) {
	fact.Digest = ""
	digest, err := fact.DigestV1()
	if err != nil {
		return SingleCallToolActionCoordinationFactV1{}, err
	}
	fact.Digest = digest
	return fact, fact.Validate()
}

func (f SingleCallToolActionCoordinationFactV1) Validate() error {
	if f.ContractVersion != SingleCallToolActionContractVersionV1 || !validSingleCallIDV1(f.ID) || f.Revision == 0 || f.ID != f.Request.ID || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano || f.Request.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call coordination fact is incomplete")
	}
	switch f.State {
	case SingleCallToolActionPreparedV1, SingleCallToolActionDispatchIntentV1:
		if f.StartClaimID != "" || f.Result != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "pre-dispatch single-call coordination cannot contain a start claim or Result")
		}
	case SingleCallToolActionWaitingInspectV1:
		if !validSingleCallIDV1(f.StartClaimID) || f.Result != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "waiting single-call coordination requires one exact start claim and no Result")
		}
	case SingleCallToolActionCompletedV1:
		if !validSingleCallIDV1(f.StartClaimID) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "completed single-call coordination lacks its start claim")
		}
		if f.Result != nil {
			if f.Result.Validate() != nil || f.Result.RequestID != f.Request.ID || f.Result.RequestDigest != f.Request.Digest {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "completed single-call coordination contains another Result")
			}
		} else {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "completed single-call coordination lacks the exact Result")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "single-call coordination state is invalid")
	}
	digest, err := f.DigestV1()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call coordination fact digest drifted")
	}
	return nil
}

func (f SingleCallToolActionCoordinationFactV1) DigestV1() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.single-call-tool-action", SingleCallToolActionContractVersionV1, "SingleCallToolActionCoordinationFactV1", copy)
}

func NextSingleCallToolActionCoordinationFactV1(current SingleCallToolActionCoordinationFactV1, state SingleCallToolActionCoordinationStateV1, result *SingleCallToolActionResultRefV1, now time.Time) (SingleCallToolActionCoordinationFactV1, error) {
	if current.State == SingleCallToolActionDispatchIntentV1 && state == SingleCallToolActionWaitingInspectV1 {
		return SingleCallToolActionCoordinationFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "waiting_inspect requires an explicit unique start claim")
	}
	next := current
	next.Revision++
	next.State = state
	next.Result = result
	next.UpdatedUnixNano = now.UnixNano()
	next.Digest = ""
	next, err := SealSingleCallToolActionCoordinationFactV1(next)
	if err != nil {
		return SingleCallToolActionCoordinationFactV1{}, err
	}
	return next, ValidateSingleCallToolActionCoordinationTransitionV1(current, next)
}

func ClaimSingleCallToolActionStartV1(current SingleCallToolActionCoordinationFactV1, claimID string, now time.Time) (SingleCallToolActionCoordinationFactV1, error) {
	if current.State != SingleCallToolActionDispatchIntentV1 || !validSingleCallIDV1(claimID) {
		return SingleCallToolActionCoordinationFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call start claim requires dispatch_intent and a unique claim ID")
	}
	next := current
	next.Revision++
	next.State = SingleCallToolActionWaitingInspectV1
	next.StartClaimID = claimID
	next.Result = nil
	next.UpdatedUnixNano = now.UnixNano()
	next.Digest = ""
	next, err := SealSingleCallToolActionCoordinationFactV1(next)
	if err != nil {
		return SingleCallToolActionCoordinationFactV1{}, err
	}
	return next, ValidateSingleCallToolActionCoordinationTransitionV1(current, next)
}

func ValidateSingleCallToolActionCoordinationTransitionV1(current, next SingleCallToolActionCoordinationFactV1) error {
	if current.Validate() != nil || next.Validate() != nil || current.ID != next.ID || current.Request.Digest != next.Request.Digest || next.Revision != current.Revision+1 || next.CreatedUnixNano != current.CreatedUnixNano || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "single-call coordination immutable content or revision drifted")
	}
	if current.State == SingleCallToolActionDispatchIntentV1 && next.State == SingleCallToolActionWaitingInspectV1 {
		if current.StartClaimID != "" || next.StartClaimID == "" {
			return core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "single-call start claim was not created exactly once")
		}
	} else if current.StartClaimID != next.StartClaimID {
		return core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "single-call start claim is immutable")
	}
	allowed := current.State == SingleCallToolActionPreparedV1 && next.State == SingleCallToolActionDispatchIntentV1 || current.State == SingleCallToolActionDispatchIntentV1 && (next.State == SingleCallToolActionWaitingInspectV1 || next.State == SingleCallToolActionCompletedV1) || current.State == SingleCallToolActionWaitingInspectV1 && next.State == SingleCallToolActionCompletedV1
	if !allowed {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "single-call coordination transition is not authorized")
	}
	return nil
}

func SameSingleCallToolActionResultRefV1(left, right SingleCallToolActionResultRefV1) bool {
	return left == right
}

func validSingleCallIDV1(value string) bool {
	return value != "" && len(value) <= MaxSingleCallCoordinateIDBytesV1 && strings.TrimSpace(value) == value
}

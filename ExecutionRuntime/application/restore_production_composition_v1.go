package application

import (
	"time"

	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RestoreProductionConfigV1 contains only versioned public Owner ports. It
// does not expose Runtime/Sandbox/Context stores or implementation packages.
type RestoreProductionConfigV1 struct {
	ExecutionIntents    applicationports.RestoreExecutionIntentFactPortV1
	StageResults        applicationports.RestoreStageActionResultFactPortV1
	ExecutionResults    applicationports.RestoreExecutionResultFactPortV1
	Restore             runtimeports.RestoreGovernancePortV2
	Materialization     runtimeports.RestoreMaterializationCurrentReaderV1
	AuthorizationInputs applicationports.RestoreStageAuthorizationInputCurrentReaderV1
	Admission           runtimeports.OperationEffectAdmissionPortV3
	Reviews             runtimeports.OperationReviewAuthorizationGovernancePortV4
	Dispatch            runtimeports.OperationGovernancePortV4
	Participant         applicationports.RestoreStageParticipantPortV1
	Enforcement         runtimeports.RestoreStageEnforcementGovernancePortV1
	StageGovernance     runtimeports.RestoreStageGovernanceCurrentPortV1
	Evidence            runtimeports.RestoreStageEvidenceGovernancePortV1
	StageSettlements    runtimeports.RestoreStageSettlementGovernancePortV1
	Context             applicationports.RestoreContextMaterializationPortV1
	Activation          runtimeports.RestoreActivationGovernancePortV1
	Clock               func() time.Time
}

// RestoreProductionCompositionV1 is the host-level vertical Restore route:
// Intent current -> Admission -> Review -> Permit/Begin -> Sandbox Stage ->
// authoritative Evidence -> Runtime Settlement -> Sandbox ApplySettlement ->
// Context materialization -> Runtime Activation. It never claims rollback of
// the external world and contains no Provider implementation.
type RestoreProductionCompositionV1 struct {
	Authorization *RestoreStageAuthorizationGatewayV1
	Evidence      *RestoreStageEvidenceAdapterV1
	Stage         *RestoreStageActionGatewayV1
	Restore       *RestoreExecutionCoordinatorV1
}

func NewRestoreProductionCompositionV1(config RestoreProductionConfigV1) (*RestoreProductionCompositionV1, error) {
	if config.Clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Restore production composition clock is required")
	}
	authorization, err := NewRestoreStageAuthorizationGatewayV1(RestoreStageAuthorizationGatewayConfigV1{Inputs: config.AuthorizationInputs, Admission: config.Admission, Reviews: config.Reviews, Dispatch: config.Dispatch, Clock: config.Clock})
	if err != nil {
		return nil, err
	}
	evidence, err := NewRestoreStageEvidenceAdapterV1(config.Evidence, config.Clock)
	if err != nil {
		return nil, err
	}
	stage, err := NewRestoreStageActionGatewayV1(RestoreStageActionGatewayConfigV1{Results: config.StageResults, Authorization: authorization, Participant: config.Participant, Enforcement: config.Enforcement, Governance: config.StageGovernance, Evidence: evidence, Settlements: config.StageSettlements, Clock: config.Clock})
	if err != nil {
		return nil, err
	}
	restore, err := NewRestoreExecutionCoordinatorV1(RestoreExecutionCoordinatorConfigV1{Intents: config.ExecutionIntents, Results: config.ExecutionResults, Restore: config.Restore, Materialization: config.Materialization, Stage: stage, Context: config.Context, Activation: config.Activation, Clock: config.Clock})
	if err != nil {
		return nil, err
	}
	return &RestoreProductionCompositionV1{Authorization: authorization, Evidence: evidence, Stage: stage, Restore: restore}, nil
}

var _ applicationports.RestoreExecutionPortV1 = (*RestoreExecutionCoordinatorV1)(nil)

package control

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationScopeEvidenceRuntimeCurrentAdapterV3 composes the public V4/V4.1
// read-only gateways with the immutable Effect fact. It owns no governance
// facts and never calls a Provider.
type OperationScopeEvidenceRuntimeCurrentAdapterV3 struct {
	Effects     OperationEffectFactPortV3
	Enforcement ports.OperationDispatchEnforcementGovernancePortV4
	Clock       func() time.Time
}

func (a OperationScopeEvidenceRuntimeCurrentAdapterV3) InspectOperationScopeEvidenceRuntimeCurrentV3(ctx context.Context, scope ports.OperationScopeEvidenceScopeV3, permitID string) (ports.OperationScopeEvidenceRuntimeCurrentProjectionV3, error) {
	if a.Effects == nil || a.Enforcement == nil || a.Clock == nil {
		return ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Effect and enforcement current readers are required")
	}
	if err := scope.Validate(); err != nil {
		return ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{}, err
	}
	if permitID == "" {
		return ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "permit id is required")
	}
	now := a.Clock()
	if now.IsZero() {
		return ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "runtime current clock returned zero")
	}
	effect, err := a.Effects.InspectOperationEffectV3(ctx, scope.Operation, scope.EffectID)
	if err != nil {
		return ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{}, err
	}
	intentDigest, err := effect.Intent.DigestV3()
	if err != nil {
		return ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{}, err
	}
	if effect.Intent.Revision != scope.EffectRevision || intentDigest != scope.EffectDigest || effect.Intent.Kind != scope.EffectKind {
		return ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectIntentMissing, "Operation Evidence Effect currentness drifted")
	}
	historical, err := a.Enforcement.InspectOperationDispatchEnforcementV4(ctx, ports.InspectOperationDispatchEnforcementRequestV4{Operation: scope.Operation, EffectID: scope.EffectID, PermitID: permitID, Phase: scope.Phase})
	if err != nil {
		return ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{}, err
	}
	var receipt *ports.OperationDispatchEnforcementPhaseReceiptV4
	if scope.Phase == ports.OperationDispatchEnforcementPrepareV4 {
		receipt = historical.Prepare
	} else {
		receipt = historical.Execute
	}
	if receipt == nil {
		return ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "enforcement phase is absent")
	}
	current, err := a.Enforcement.InspectCurrentOperationDispatchEnforcementV4(ctx, ports.InspectCurrentOperationDispatchEnforcementRequestV4{
		Inspect:      ports.InspectOperationDispatchEnforcementRequestV4{Operation: scope.Operation, EffectID: scope.EffectID, PermitID: permitID, Phase: scope.Phase},
		PermitDigest: receipt.PermitDigest, AdmissionDigest: receipt.AdmissionDigest, ReviewAuthorization: receipt.ReviewAuthorization,
		SandboxAttempt: receipt.SandboxAttempt, SandboxProjectionDigest: receipt.Sandbox.ProjectionDigest,
	})
	if err != nil {
		return ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{}, err
	}
	if err := current.Validate(); err != nil {
		return ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{}, err
	}
	return ports.SealOperationScopeEvidenceRuntimeCurrentProjectionV3(ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{
		Scope: scope, PermitID: permitID, PermitFactRevision: current.Dispatch.Record.Revision, PermitDigest: current.Dispatch.Record.PermitDigest,
		AdmissionDigest: current.Dispatch.Record.Permit.Admission.Digest, Authorization: current.Dispatch.ReviewAuthorization,
		Phase: current.Phase, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: current.ExpiresUnixNano,
	}, now)
}

type OperationScopeEvidenceGenerationCurrentAdapterV3 struct {
	Associations ports.GenerationBindingAssociationGovernancePortV1
	Clock        func() time.Time
}

func (a OperationScopeEvidenceGenerationCurrentAdapterV3) InspectOperationScopeEvidenceGenerationCurrentV3(ctx context.Context, expected ports.GenerationBindingAssociationRefV1) (ports.OperationScopeEvidenceFactRefV3, error) {
	if a.Associations == nil || a.Clock == nil {
		return ports.OperationScopeEvidenceFactRefV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "generation association current reader is required")
	}
	if err := expected.Validate(); err != nil {
		return ports.OperationScopeEvidenceFactRefV3{}, err
	}
	now := a.Clock()
	if now.IsZero() {
		return ports.OperationScopeEvidenceFactRefV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "generation current clock returned zero")
	}
	fact, err := a.Associations.InspectCurrentGenerationBindingAssociationV1(ctx, expected.ID)
	if err != nil {
		return ports.OperationScopeEvidenceFactRefV3{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.OperationScopeEvidenceFactRefV3{}, err
	}
	if fact.RefV1() != expected || fact.State != ports.GenerationBindingAssociationActiveV1 || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return ports.OperationScopeEvidenceFactRefV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "generation association is stale")
	}
	return ports.OperationScopeEvidenceFactRefV3{ID: fact.ID, Revision: fact.Revision, Digest: fact.Digest, ExpiresUnixNano: fact.ExpiresUnixNano}, nil
}

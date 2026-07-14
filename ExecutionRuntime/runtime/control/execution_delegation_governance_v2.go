package control

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ExecutionDelegationGovernanceGatewayV2 is the Application-facing owner of
// declaration and prepared-commit orchestration. The enforcement Fact and the
// delegation Fact have different owners, so recovery always inspects both
// exact watermarks and never claims cross-store atomicity.
type ExecutionDelegationGovernanceGatewayV2 struct {
	Effects     OperationEffectFactPortV3
	Delegations ports.ExecutionDelegationFactPortV2
	Current     ports.OperationGovernanceCurrentReaderV3
	Clock       func() time.Time
}

func (g ExecutionDelegationGovernanceGatewayV2) DeclareExecutionDelegationV2(ctx context.Context, request ports.DeclareExecutionDelegationRequestV2) (ports.ExecutionDelegationRefV2, error) {
	if err := g.validate(); err != nil {
		return ports.ExecutionDelegationRefV2{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.ExecutionDelegationRefV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "delegation gateway clock returned zero")
	}
	fact := request.Delegation
	if err := fact.Validate(); err != nil {
		return ports.ExecutionDelegationRefV2{}, err
	}
	if fact.State != ports.ExecutionDelegationDeclaredV2 || fact.Revision != 1 || fact.Preparation != nil {
		return ports.ExecutionDelegationRefV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "new governed delegation must be declared revision one")
	}
	if err := request.Intent.Validate(); err != nil {
		return ports.ExecutionDelegationRefV2{}, err
	}
	if err := request.Permit.Validate(); err != nil {
		return ports.ExecutionDelegationRefV2{}, err
	}
	permit, err := g.Effects.InspectOperationDispatchPermitV3(ctx, request.Intent.Operation, request.Permit.ID)
	if err != nil {
		return ports.ExecutionDelegationRefV2{}, err
	}
	current, err := g.Current.InspectOperationGovernance(ctx, request.Intent.Operation)
	if err != nil {
		return ports.ExecutionDelegationRefV2{}, err
	}
	if err := ports.ValidateOperationAtExecutionPointV3(request.Permit, request.Intent, request.Fence, current, now); err != nil {
		return ports.ExecutionDelegationRefV2{}, err
	}
	permitDigest, _ := request.Permit.DigestV3()
	operationDigest, _ := request.Intent.Operation.DigestV3()
	if permit.State != OperationPermitBegunV3 || permit.PermitDigest != permitDigest || permit.Permit.ID != fact.ProviderPermitID || permit.Permit.Revision != fact.ProviderPermitRevision || permit.Permit.AttemptID != fact.ProviderAttemptID || permit.Enforcement != nil || operationDigest != mustDelegationOperationDigestV2(fact.Operation) || fact.IntentID != request.Intent.ID || fact.IntentRevision != request.Intent.Revision || fact.IntentDigest != permit.Permit.IntentDigest || fact.PayloadSchema != request.Intent.Payload.Schema || fact.PayloadDigest != request.Intent.Payload.ContentDigest || fact.PayloadRevision != request.Intent.PayloadRevision || fact.DataProvider != request.Permit.Provider || fact.ProviderPermitDigest != permitDigest || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return ports.ExecutionDelegationRefV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "delegation does not bind the exact begun operation attempt")
	}
	created, err := g.Delegations.CreateExecutionDelegationV2(ctx, fact)
	if err != nil {
		if !recoverableOperationWriteErrorV3(err) {
			return ports.ExecutionDelegationRefV2{}, err
		}
		inspected, inspectErr := g.Delegations.InspectExecutionDelegationV2(context.WithoutCancel(ctx), fact.ID)
		if inspectErr != nil || !sameExecutionDelegationV2(inspected, fact) {
			return ports.ExecutionDelegationRefV2{}, err
		}
		created = inspected
	}
	return created.RefV2()
}

func (g ExecutionDelegationGovernanceGatewayV2) CommitPreparedExecutionV2(ctx context.Context, request ports.CommitPreparedExecutionRequestV2) (ports.PreparedExecutionGovernanceResultV2, error) {
	if err := g.validate(); err != nil {
		return ports.PreparedExecutionGovernanceResultV2{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.PreparedExecutionGovernanceResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "delegation gateway clock returned zero")
	}
	declared, err := g.Delegations.InspectExecutionDelegationV2(ctx, request.Declared.ID)
	if err != nil {
		return ports.PreparedExecutionGovernanceResultV2{}, err
	}
	declaredRef, refErr := declared.RefV2()
	if refErr != nil || declaredRef != request.Declared || declared.State != ports.ExecutionDelegationDeclaredV2 {
		// A lost CAS reply may already have advanced this exact declaration.
		if declared.State == ports.ExecutionDelegationPreparedV2 && declared.Preparation != nil {
			return g.inspectPrepared(ctx, request.Intent.Operation, declared, request.Permit.ID)
		}
		return ports.PreparedExecutionGovernanceResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "declared delegation watermark drifted")
	}
	prepare := ports.PrepareGovernedExecutionRequestV2{Delegation: request.Declared, Intent: request.Intent, Permit: request.Permit, Fence: request.Fence}
	if err := request.Preparation.ValidateAgainstPrepare(prepare, now); err != nil {
		return ports.PreparedExecutionGovernanceResultV2{}, err
	}
	if err := prepare.ValidateAgainstDelegation(declared, now); err != nil {
		return ports.PreparedExecutionGovernanceResultV2{}, err
	}
	current, err := g.Current.InspectOperationGovernance(ctx, request.Intent.Operation)
	if err != nil {
		return ports.PreparedExecutionGovernanceResultV2{}, err
	}
	if err := ports.ValidateOperationAtExecutionPointV3(request.Permit, request.Intent, request.Fence, current, now); err != nil {
		return ports.PreparedExecutionGovernanceResultV2{}, err
	}
	permit, err := g.Effects.InspectOperationDispatchPermitV3(ctx, request.Intent.Operation, request.Permit.ID)
	if err != nil {
		return ports.PreparedExecutionGovernanceResultV2{}, err
	}
	permitDigest, _ := request.Permit.DigestV3()
	if permit.State != OperationPermitBegunV3 || permit.PermitDigest != permitDigest {
		return ports.PreparedExecutionGovernanceResultV2{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "prepared commit requires the exact begun Permit")
	}
	storedPermit := permit
	if permit.Enforcement != nil {
		if *permit.Enforcement != request.Preparation.Enforcement {
			return ports.PreparedExecutionGovernanceResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "persisted enforcement belongs to another preparation")
		}
	} else {
		storedPermit, err = g.Effects.RecordOperationEnforcementV3(ctx, RecordOperationEnforcementRequestV3{Operation: request.Intent.Operation, PermitID: request.Permit.ID, ExpectedPermitRevision: permit.Revision, Receipt: request.Preparation.Enforcement})
		if err != nil {
			if !recoverableOperationWriteErrorV3(err) {
				return ports.PreparedExecutionGovernanceResultV2{}, err
			}
			storedPermit, err = g.Effects.InspectOperationDispatchPermitV3(context.WithoutCancel(ctx), request.Intent.Operation, request.Permit.ID)
			if err != nil || storedPermit.Enforcement == nil || *storedPermit.Enforcement != request.Preparation.Enforcement {
				return ports.PreparedExecutionGovernanceResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "cannot prove persisted operation enforcement")
			}
		}
	}
	enforcement, err := storedPermit.PersistedEnforcementRefV3()
	if err != nil {
		return ports.PreparedExecutionGovernanceResultV2{}, err
	}
	next := declared
	next.State = ports.ExecutionDelegationPreparedV2
	next.Revision++
	preparation := request.Preparation
	next.Preparation = &preparation
	storedDelegation, err := g.Delegations.CompareAndSwapExecutionDelegationV2(ctx, ports.ExecutionDelegationCASRequestV2{ExpectedRevision: declared.Revision, Next: next})
	if err != nil {
		if !recoverableOperationWriteErrorV3(err) {
			return ports.PreparedExecutionGovernanceResultV2{}, err
		}
		storedDelegation, err = g.Delegations.InspectExecutionDelegationV2(context.WithoutCancel(ctx), declared.ID)
		if err != nil || !sameExecutionDelegationV2(storedDelegation, next) {
			return ports.PreparedExecutionGovernanceResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "cannot prove prepared delegation CAS")
		}
	}
	ref, _ := storedDelegation.RefV2()
	result := ports.PreparedExecutionGovernanceResultV2{Delegation: ref, Prepared: request.Preparation.Prepared, Enforcement: enforcement}
	return result, result.Validate()
}

func (g ExecutionDelegationGovernanceGatewayV2) InspectDeclaredExecutionV2(ctx context.Context, operation ports.OperationSubjectV3, delegationID string) (ports.ExecutionDelegationRefV2, error) {
	if g.Delegations == nil {
		return ports.ExecutionDelegationRefV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "delegation inspection requires the delegation Fact Owner")
	}
	if err := operation.Validate(); err != nil {
		return ports.ExecutionDelegationRefV2{}, err
	}
	fact, err := g.Delegations.InspectExecutionDelegationV2(ctx, delegationID)
	if err != nil {
		return ports.ExecutionDelegationRefV2{}, err
	}
	if fact.State != ports.ExecutionDelegationDeclaredV2 || !ports.SameOperationSubjectV3(fact.Operation, operation) || fact.ID != delegationID {
		return ports.ExecutionDelegationRefV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "exact declared execution delegation is not current")
	}
	return fact.RefV2()
}

func (g ExecutionDelegationGovernanceGatewayV2) InspectPreparedExecutionV2(ctx context.Context, operation ports.OperationSubjectV3, delegationID, permitID string) (ports.PreparedExecutionGovernanceResultV2, error) {
	if err := g.validate(); err != nil {
		return ports.PreparedExecutionGovernanceResultV2{}, err
	}
	if err := operation.Validate(); err != nil {
		return ports.PreparedExecutionGovernanceResultV2{}, err
	}
	if strings.TrimSpace(delegationID) == "" || strings.TrimSpace(permitID) == "" {
		return ports.PreparedExecutionGovernanceResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prepared execution inspection requires delegation and Permit identities")
	}
	fact, err := g.Delegations.InspectExecutionDelegationV2(ctx, delegationID)
	if err != nil {
		return ports.PreparedExecutionGovernanceResultV2{}, err
	}
	return g.inspectPrepared(ctx, operation, fact, permitID)
}

func (g ExecutionDelegationGovernanceGatewayV2) inspectPrepared(ctx context.Context, operation ports.OperationSubjectV3, fact ports.ExecutionDelegationFactV2, permitID string) (ports.PreparedExecutionGovernanceResultV2, error) {
	if err := fact.Validate(); err != nil {
		return ports.PreparedExecutionGovernanceResultV2{}, err
	}
	if fact.ProviderPermitID != permitID || !ports.SameOperationSubjectV3(fact.Operation, operation) {
		return ports.PreparedExecutionGovernanceResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "prepared delegation not found at exact operation/Permit")
	}
	if fact.State == ports.ExecutionDelegationDeclaredV2 && fact.Preparation == nil {
		return ports.PreparedExecutionGovernanceResultV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "prepared execution fact does not exist for the exact declared delegation")
	}
	if fact.State != ports.ExecutionDelegationPreparedV2 || fact.Preparation == nil {
		return ports.PreparedExecutionGovernanceResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "delegation is not an exact prepared execution fact")
	}
	permit, err := g.Effects.InspectOperationDispatchPermitV3(ctx, operation, permitID)
	if err != nil {
		return ports.PreparedExecutionGovernanceResultV2{}, err
	}
	enforcement, err := permit.PersistedEnforcementRefV3()
	if err != nil {
		return ports.PreparedExecutionGovernanceResultV2{}, err
	}
	ref, _ := fact.RefV2()
	result := ports.PreparedExecutionGovernanceResultV2{Delegation: ref, Prepared: fact.Preparation.Prepared, Enforcement: enforcement}
	return result, result.Validate()
}

func (g ExecutionDelegationGovernanceGatewayV2) validate() error {
	if g.Effects == nil || g.Delegations == nil || g.Current == nil || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "delegation gateway requires Effect, delegation, current governance and clock")
	}
	return nil
}

func sameExecutionDelegationV2(left, right ports.ExecutionDelegationFactV2) bool {
	ld, le := left.DigestV2()
	rd, re := right.DigestV2()
	return le == nil && re == nil && ld == rd
}

func mustDelegationOperationDigestV2(subject ports.OperationSubjectV3) core.Digest {
	d, _ := subject.DigestV3()
	return d
}

func recoverableOperationWriteErrorV3(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate) || core.HasCategory(err, core.ErrorConflict)
}

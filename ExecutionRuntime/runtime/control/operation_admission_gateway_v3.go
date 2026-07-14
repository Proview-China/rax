package control

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationEffectAdmissionGatewayV3 is the only Application-facing path from
// immutable Operation Intent to accepted Effect Fact. The raw Fact Port stays
// private to the Fact Owner/gateway composition.
type OperationEffectAdmissionGatewayV3 struct {
	Effects OperationEffectFactPortV3
	Current ports.OperationIntentAdmissionReaderV3
	Clock   func() time.Time
}

func (g OperationEffectAdmissionGatewayV3) AdmitOperationEffectV3(ctx context.Context, intent ports.OperationEffectIntentV3) (ports.OperationEffectAdmissionReceiptV3, error) {
	if g.Effects == nil || g.Current == nil || g.Clock == nil {
		return ports.OperationEffectAdmissionReceiptV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "operation admission requires Fact Owner, admission reader and injected clock")
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.OperationEffectAdmissionReceiptV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "operation admission clock returned zero")
	}
	if err := intent.Validate(); err != nil {
		return ports.OperationEffectAdmissionReceiptV3{}, err
	}
	current, err := g.Current.InspectOperationIntentAdmission(ctx, intent)
	if err != nil {
		return ports.OperationEffectAdmissionReceiptV3{}, err
	}
	if err := current.ValidateCurrent(intent, now); err != nil {
		return ports.OperationEffectAdmissionReceiptV3{}, err
	}
	proposed, err := NewProposedOperationEffectFactV3(intent, now)
	if err != nil {
		return ports.OperationEffectAdmissionReceiptV3{}, err
	}
	created, err := g.Effects.CreateOperationEffectV3(ctx, proposed)
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) {
			return ports.OperationEffectAdmissionReceiptV3{}, err
		}
		inspected, inspectErr := g.Effects.InspectOperationEffectV3(context.WithoutCancel(ctx), intent.Operation, intent.ID)
		if inspectErr != nil {
			return ports.OperationEffectAdmissionReceiptV3{}, err
		}
		if inspected.IntentDigest != proposed.IntentDigest || inspected.Intent.ID != intent.ID || inspected.Intent.Revision != intent.Revision {
			return ports.OperationEffectAdmissionReceiptV3{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "inspected operation Effect differs after unknown create result")
		}
		created = inspected
	}
	if created.State == OperationEffectAcceptedV3 {
		return operationAdmissionReceiptV3(created)
	}
	if created.State != OperationEffectProposedV3 {
		return ports.OperationEffectAdmissionReceiptV3{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "operation Effect is neither proposed nor accepted during admission")
	}
	// Re-read immediately before acceptance so an Owner/Binding revocation after
	// write-ahead cannot be hidden by the proposed Fact.
	current, err = g.Current.InspectOperationIntentAdmission(ctx, intent)
	if err != nil {
		return ports.OperationEffectAdmissionReceiptV3{}, err
	}
	if err := current.ValidateCurrent(intent, now); err != nil {
		return ports.OperationEffectAdmissionReceiptV3{}, err
	}
	accepted := created
	accepted.State = OperationEffectAcceptedV3
	accepted.Revision++
	accepted.UpdatedUnixNano = now.UnixNano()
	stored, err := g.Effects.CompareAndSwapOperationEffectV3(ctx, intent.Operation, OperationEffectCASRequestV3{ExpectedRevision: created.Revision, Next: accepted})
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) {
			return ports.OperationEffectAdmissionReceiptV3{}, err
		}
		inspected, inspectErr := g.Effects.InspectOperationEffectV3(context.WithoutCancel(ctx), intent.Operation, intent.ID)
		if inspectErr != nil {
			return ports.OperationEffectAdmissionReceiptV3{}, err
		}
		if inspected.State != OperationEffectAcceptedV3 || inspected.IntentDigest != accepted.IntentDigest || inspected.Revision != accepted.Revision {
			return ports.OperationEffectAdmissionReceiptV3{}, err
		}
		stored = inspected
	}
	return operationAdmissionReceiptV3(stored)
}

func (g OperationEffectAdmissionGatewayV3) InspectAcceptedOperationEffectV3(ctx context.Context, operation ports.OperationSubjectV3, effectID core.EffectIntentID) (ports.OperationEffectAdmissionReceiptV3, error) {
	if g.Effects == nil {
		return ports.OperationEffectAdmissionReceiptV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "operation admission inspection requires the Effect Fact Owner")
	}
	if err := operation.Validate(); err != nil {
		return ports.OperationEffectAdmissionReceiptV3{}, err
	}
	if strings.TrimSpace(string(effectID)) == "" {
		return ports.OperationEffectAdmissionReceiptV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "accepted operation Effect inspection requires an Effect ID")
	}
	fact, err := g.Effects.InspectOperationEffectV3(ctx, operation, effectID)
	if err != nil {
		return ports.OperationEffectAdmissionReceiptV3{}, err
	}
	if fact.State != OperationEffectAcceptedV3 || !ports.SameOperationSubjectV3(fact.Intent.Operation, operation) || fact.Intent.ID != effectID {
		return ports.OperationEffectAdmissionReceiptV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "exact accepted operation Effect is not current")
	}
	return operationAdmissionReceiptV3(fact)
}

func operationAdmissionReceiptV3(fact OperationEffectFactV3) (ports.OperationEffectAdmissionReceiptV3, error) {
	ref, err := fact.RefV3()
	if err != nil {
		return ports.OperationEffectAdmissionReceiptV3{}, err
	}
	receipt := ports.OperationEffectAdmissionReceiptV3{
		OperationDigest: ref.OperationDigest,
		EffectID:        ref.EffectID,
		IntentRevision:  ref.IntentRevision,
		IntentDigest:    ref.IntentDigest,
		FactRevision:    ref.FactRevision,
		State:           string(fact.State),
	}
	return receipt, receipt.Validate()
}

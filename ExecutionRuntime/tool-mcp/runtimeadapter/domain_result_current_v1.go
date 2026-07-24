package runtimeadapter

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type ToolDomainResultCurrentSourceV1 interface {
	InspectDomainResultByExactV2(contract.ObjectRef) (contract.ToolDomainResultFactV2, error)
	InspectDomainResultCurrentByExactV1(context.Context, contract.ObjectRef, time.Time, time.Duration) (contract.ToolDomainResultCurrentProjectionV1, error)
}

type DomainResultCurrentAdapterConfigV1 struct {
	Source      ToolDomainResultCurrentSourceV1
	Clock       ClockV1
	LeaseTTL    time.Duration
	EffectKind  runtimeports.EffectKindV2
	DomainOwner runtimeports.ProviderBindingRefV2
	DomainKind  runtimeports.NamespacedNameV2
}

// DomainResultCurrentAdapterV1 exposes a Runtime-neutral public current reader
// over Tool-owned authoritative facts. It grants no settlement authority.
type DomainResultCurrentAdapterV1 struct {
	config DomainResultCurrentAdapterConfigV1
}

func NewDomainResultCurrentAdapterV1(config DomainResultCurrentAdapterConfigV1) (*DomainResultCurrentAdapterV1, error) {
	if config.Source == nil || config.Clock == nil || config.LeaseTTL <= 0 || config.LeaseTTL > contract.MaxDomainResultCurrentTTLV1 || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(config.EffectKind)) != nil || config.DomainOwner.Validate() != nil || runtimeports.ValidateNamespacedNameV2(config.DomainKind) != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool DomainResult current adapter configuration is incomplete")
	}
	return &DomainResultCurrentAdapterV1{config: config}, nil
}

func (a *DomainResultCurrentAdapterV1) InspectOperationSettlementDomainResultCurrentV4(ctx context.Context, effectKind runtimeports.EffectKindV2, exact runtimeports.OperationSettlementDomainResultFactRefV4) (runtimeports.OperationSettlementDomainResultCurrentV4, error) {
	if a == nil || effectKind != a.config.EffectKind || exact.Validate() != nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool DomainResult exact Runtime ref is invalid")
	}
	now := a.config.Clock.Now()
	if now.IsZero() {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "Tool DomainResult current read requires current time")
	}
	toolRef := contract.ObjectRef{ID: exact.ID, Revision: exact.Revision, Digest: exact.Digest}
	fact, err := a.config.Source.InspectDomainResultByExactV2(toolRef)
	if err != nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, err
	}
	mapped := runtimeports.OperationSettlementDomainResultFactRefV4{Owner: a.config.DomainOwner, Kind: a.config.DomainKind, ID: fact.ID, Revision: fact.Revision, Digest: fact.Digest, TenantID: fact.TenantID, EffectID: fact.Causality.EffectID, EffectRevision: fact.Causality.EffectRevision, Operation: fact.Causality.Operation, OperationDigest: fact.Causality.OperationDigest, Attempt: fact.Causality.Attempt, Schema: fact.Schema, PayloadDigest: fact.PayloadDigest, PayloadRevision: fact.PayloadRevision, AuthoritativeTime: fact.CreatedUnixNano}
	if !runtimeports.SameOperationSettlementDomainResultFactRefV4(mapped, exact) {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Runtime DomainResult ref cannot be losslessly mapped to Tool fact")
	}
	current, err := a.config.Source.InspectDomainResultCurrentByExactV1(ctx, toolRef, now, a.config.LeaseTTL)
	if err != nil {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, err
	}
	if current.Fact != toolRef || current.CausalityDigest != fact.Causality.Digest {
		return runtimeports.OperationSettlementDomainResultCurrentV4{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Tool DomainResult current projection drifted")
	}
	return runtimeports.SealOperationSettlementDomainResultCurrentV4(runtimeports.OperationSettlementDomainResultCurrentV4{EffectKind: effectKind, Fact: exact, CheckedUnixNano: current.CheckedUnixNano, ExpiresUnixNano: current.ExpiresUnixNano}, now)
}

var _ runtimeports.OperationSettlementDomainResultCurrentReaderV4 = (*DomainResultCurrentAdapterV1)(nil)

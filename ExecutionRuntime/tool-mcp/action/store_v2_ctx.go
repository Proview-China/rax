package action

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func checkStoreContextV2(ctx context.Context) error {
	if ctx == nil {
		return invalidV2("Tool fact store context is required")
	}
	return ctx.Err()
}

func checkStorePostMutationContextV2(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "in-memory Tool fact mutation committed while caller outcome became unknown; only exact Inspect may continue")
	}
	return nil
}

// ContextFactStoreV2 is the context-aware view over the reference in-memory
// StoreV2. StoreV2 keeps its original source-compatible methods.
type ContextFactStoreV2 struct {
	store *StoreV2
}

func NewContextFactStoreV2(store *StoreV2) (*ContextFactStoreV2, error) {
	if store == nil {
		return nil, invalidV2("in-memory Tool fact store is required")
	}
	return &ContextFactStoreV2{store: store}, nil
}

func (s *ContextFactStoreV2) CreateCandidateFactV2(ctx context.Context, candidate contract.ActionCandidateV2) (RecordV2, error) {
	if err := checkStoreContextV2(ctx); err != nil {
		return RecordV2{}, err
	}
	record, err := s.store.PutCandidateV2(candidate)
	if err != nil {
		return RecordV2{}, err
	}
	if err = checkStorePostMutationContextV2(ctx); err != nil {
		return RecordV2{}, err
	}
	return record, nil
}

func (s *ContextFactStoreV2) InspectCandidateCurrentV2(ctx context.Context, exact contract.ObjectRef, now time.Time) (contract.ActionCandidateV2, error) {
	if err := checkStoreContextV2(ctx); err != nil {
		return contract.ActionCandidateV2{}, err
	}
	return s.store.InspectCandidateCurrentV2(ctx, exact, now)
}

func (s *ContextFactStoreV2) CreateReservationFactV2(ctx context.Context, action contract.ObjectRef, attempt contract.ApplicationAttemptRefV1, intent core.Digest, session string, subject core.Digest, now, expires time.Time) (contract.ActionReservationFactV2, error) {
	if err := checkStoreContextV2(ctx); err != nil {
		return contract.ActionReservationFactV2{}, err
	}
	fact, err := s.store.ReserveV2(ctx, action, attempt, intent, session, subject, now, expires)
	if err != nil {
		return contract.ActionReservationFactV2{}, err
	}
	if err = checkStorePostMutationContextV2(ctx); err != nil {
		return contract.ActionReservationFactV2{}, err
	}
	return fact, nil
}

func (s *ContextFactStoreV2) InspectReservationExactV2(ctx context.Context, actionID string, exact contract.ObjectRef) (contract.ActionReservationFactV2, error) {
	if err := checkStoreContextV2(ctx); err != nil {
		return contract.ActionReservationFactV2{}, err
	}
	return s.store.InspectReservationV2(actionID, exact)
}

func (s *ContextFactStoreV2) CreateDomainResultFactV2(ctx context.Context, fact contract.ToolDomainResultFactV2) (contract.ToolDomainResultFactV2, error) {
	if err := checkStoreContextV2(ctx); err != nil {
		return contract.ToolDomainResultFactV2{}, err
	}
	out, err := s.store.PutDomainResultV2(ctx, fact)
	if err != nil {
		return contract.ToolDomainResultFactV2{}, err
	}
	if err = checkStorePostMutationContextV2(ctx); err != nil {
		return contract.ToolDomainResultFactV2{}, err
	}
	return out, nil
}

func (s *ContextFactStoreV2) InspectDomainResultExactV2(ctx context.Context, actionID string, exact contract.ObjectRef) (contract.ToolDomainResultFactV2, error) {
	if err := checkStoreContextV2(ctx); err != nil {
		return contract.ToolDomainResultFactV2{}, err
	}
	return s.store.InspectDomainResultV2(actionID, exact)
}

func (s *ContextFactStoreV2) InspectDomainResultCurrentByExactV1(ctx context.Context, exact contract.ObjectRef, now time.Time, ttl time.Duration) (contract.ToolDomainResultCurrentProjectionV1, error) {
	if err := checkStoreContextV2(ctx); err != nil {
		return contract.ToolDomainResultCurrentProjectionV1{}, err
	}
	return s.store.InspectDomainResultCurrentByExactV1(ctx, exact, now, ttl)
}

func (s *ContextFactStoreV2) ApplySettlementAndCreateResultV2(ctx context.Context, actionID string, domain contract.ObjectRef, inspection runtimeports.OperationInspectionSettlementRefV4, outcome contract.ToolOutcomeV2, disposition contract.ToolDispositionV2, now time.Time) (contract.ToolResultV2, error) {
	if err := checkStoreContextV2(ctx); err != nil {
		return contract.ToolResultV2{}, err
	}
	result, err := s.store.ApplySettlementWithContextV2(ctx, actionID, domain, inspection, outcome, disposition, now)
	if err != nil {
		return contract.ToolResultV2{}, err
	}
	if err = checkStorePostMutationContextV2(ctx); err != nil {
		return contract.ToolResultV2{}, err
	}
	return result, nil
}

func (s *ContextFactStoreV2) InspectResultExactV2(ctx context.Context, actionID string, exact contract.ObjectRef) (contract.ToolResultV2, error) {
	if err := checkStoreContextV2(ctx); err != nil {
		return contract.ToolResultV2{}, err
	}
	return s.store.InspectResultV2(actionID, exact)
}

func (s *ContextFactStoreV2) InspectSettledResultForApplyV2(ctx context.Context, actionID string, apply contract.ObjectRef) (contract.ToolResultV2, error) {
	if err := checkStoreContextV2(ctx); err != nil {
		return contract.ToolResultV2{}, err
	}
	return s.store.InspectSettledResultForApplyV2(actionID, apply)
}

var _ FactStoreV2 = (*ContextFactStoreV2)(nil)

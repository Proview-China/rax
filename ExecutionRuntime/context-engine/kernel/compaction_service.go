package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

type ContextCompactionServiceV1 struct {
	backend contextports.ContextCompactionOwnerBackendV1
	content ReferenceStore
	clock   func() time.Time
}

func NewContextCompactionServiceV1(backend contextports.ContextCompactionOwnerBackendV1, content ReferenceStore, clock func() time.Time) (*ContextCompactionServiceV1, error) {
	if backend == nil || content == nil || clock == nil {
		return nil, fmt.Errorf("%w: compaction service dependencies", contract.ErrInvalid)
	}
	return &ContextCompactionServiceV1{backend: backend, content: content, clock: clock}, nil
}

func (s *ContextCompactionServiceV1) Prepare(ctx context.Context, plan contract.ContextCompactionPlanV1, summary contract.ContextCompactionSummaryV1, manifest contract.ContextManifest, frame contract.ContextFrame) (contract.ContextCompactionPreparedV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	if err := plan.Validate(); err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	planRef := contract.FactRef{ID: plan.AttemptID, Revision: plan.Revision, Digest: plan.Digest}
	if _, inspectErr := s.backend.InspectContextCompactionV1(ctx, contract.InspectContextCompactionRequestV1{PlanRef: planRef}); inspectErr == nil {
		return contract.ContextCompactionPreparedV1{}, fmt.Errorf("%w: %w: compaction attempt already exists", contract.ErrInspectOnly, contract.ErrConflict)
	} else if !errors.Is(inspectErr, contract.ErrNotFound) {
		return contract.ContextCompactionPreparedV1{}, inspectErr
	}
	current, err := s.backend.InspectCurrentGenerationPointer(ctx, currentPointerRequestV1(plan.ExpectedCurrent))
	if err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	if current != plan.ExpectedCurrent {
		return contract.ContextCompactionPreparedV1{}, fmt.Errorf("%w: compaction S1 current drift", contract.ErrConflict)
	}
	prepared, err := PrepareContextCompactionV1(ctx, plan, summary)
	if err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	if err := InspectFrame(s.content, manifest, frame); err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	frameDigest, err := frame.DigestValue()
	if err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	if plan.TargetRootFrameRef != (contract.FactRef{ID: frame.ID, Revision: frame.Revision, Digest: frameDigest}) || frame.GenerationID != prepared.Generation.ID || frame.Generation != prepared.Generation.Ordinal || frame.Execution.ScopeDigest != current.ExecutionScopeDigest || frame.Execution.RunID != current.RunID || frame.Execution.Turn != current.Turn {
		return contract.ContextCompactionPreparedV1{}, fmt.Errorf("%w: compaction candidate frame binding", contract.ErrConflict)
	}
	now := s.clock().UnixNano()
	if now < plan.CheckedUnixNano || now >= plan.ExpiresUnixNano || now >= frame.ExpiresUnixNano || now >= summary.ExpiresUnixNano {
		return contract.ContextCompactionPreparedV1{}, fmt.Errorf("%w: compaction prepare currentness", contract.ErrExpired)
	}
	expires := minInt64V1(plan.ExpiresUnixNano, frame.ExpiresUnixNano, summary.ExpiresUnixNano, current.ExpiresUnixNano)
	bindingDigest, err := contract.DigestJSON(struct {
		Previous   contract.Digest  `json:"previous_binding_digest"`
		Summary    contract.FactRef `json:"summary_ref"`
		Frame      contract.FactRef `json:"frame_ref"`
		Generation contract.FactRef `json:"generation_ref"`
	}{current.ParentFrameGenerationBindingDigest, plan.SummaryRef, plan.TargetRootFrameRef, prepared.GenerationRef})
	if err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	next, err := contract.SealContextGenerationCurrentPointerV1(contract.ContextGenerationCurrentPointerV1{
		ID: current.ID, Revision: current.Revision + 1, ExecutionScopeDigest: current.ExecutionScopeDigest, RunID: current.RunID, SessionRef: current.SessionRef, Turn: current.Turn,
		GenerationRef: prepared.GenerationRef, GenerationOrdinal: prepared.Generation.Ordinal, ParentFrameGenerationBindingDigest: bindingDigest, ExpiresUnixNano: expires,
	})
	if err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	record := contract.ContextCompactionPendingRecordV1{Plan: plan, Summary: summary, Manifest: manifest, Frame: frame, Prepared: prepared, NextCurrent: next}
	if err := record.Validate(); err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	return s.backend.ReserveContextCompactionV1(ctx, record)
}

func (s *ContextCompactionServiceV1) Apply(ctx context.Context, request contract.ApplyContextCompactionRequestV1) (contract.ContextCompactionResultV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ContextCompactionResultV1{}, err
	}
	if err := request.Validate(); err != nil {
		return contract.ContextCompactionResultV1{}, err
	}
	prior, inspectErr := s.backend.InspectContextCompactionV1(ctx, contract.InspectContextCompactionRequestV1{PlanRef: request.PlanRef})
	if inspectErr == nil {
		if prior.Status == contract.ContextCompactionAppliedV1 {
			return contract.ContextCompactionResultV1{}, fmt.Errorf("%w: %w: compaction already applied", contract.ErrInspectOnly, contract.ErrConflict)
		}
		if prior.Status != contract.ContextCompactionPendingV1 {
			return contract.ContextCompactionResultV1{}, fmt.Errorf("%w: compaction inspect status", contract.ErrConflict)
		}
	} else if !errors.Is(inspectErr, contract.ErrNotFound) {
		return contract.ContextCompactionResultV1{}, inspectErr
	} else {
		return contract.ContextCompactionResultV1{}, inspectErr
	}
	record, err := s.backend.LoadContextCompactionPendingV1(ctx, request.PlanRef)
	if err != nil {
		return contract.ContextCompactionResultV1{}, err
	}
	if request.ExpectedCurrent != record.Plan.ExpectedCurrent || request.PreparedDigest != record.Prepared.Digest {
		return contract.ContextCompactionResultV1{}, fmt.Errorf("%w: compaction apply request binding", contract.ErrConflict)
	}
	now := s.clock().UnixNano()
	if now < request.CheckedUnixNano || now >= record.Plan.ExpiresUnixNano || now >= record.Summary.ExpiresUnixNano || now >= record.Frame.ExpiresUnixNano {
		return contract.ContextCompactionResultV1{}, fmt.Errorf("%w: compaction S2 currentness", contract.ErrExpired)
	}
	current, err := s.backend.InspectCurrentGenerationPointer(ctx, currentPointerRequestV1(request.ExpectedCurrent))
	if err != nil {
		return contract.ContextCompactionResultV1{}, err
	}
	if current != request.ExpectedCurrent {
		return contract.ContextCompactionResultV1{}, fmt.Errorf("%w: compaction S2 current drift", contract.ErrConflict)
	}
	commit := request
	commit.CheckedUnixNano = now
	commit, err = contract.SealApplyContextCompactionRequestV1(commit)
	if err != nil {
		return contract.ContextCompactionResultV1{}, err
	}
	return s.backend.ApplyContextCompactionCurrentCASV1(ctx, commit)
}

func (s *ContextCompactionServiceV1) Inspect(ctx context.Context, request contract.InspectContextCompactionRequestV1) (contract.ContextCompactionResultV1, error) {
	return s.backend.InspectContextCompactionV1(ctx, request)
}

func currentPointerRequestV1(pointer contract.ContextGenerationCurrentPointerV1) contract.ContextGenerationCurrentPointerRequestV1 {
	return contract.ContextGenerationCurrentPointerRequestV1{ExecutionScopeDigest: pointer.ExecutionScopeDigest, RunID: pointer.RunID, SessionRef: pointer.SessionRef, Turn: pointer.Turn}
}

func minInt64V1(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

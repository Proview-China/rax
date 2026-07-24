package runtimeadapter

import (
	"context"
	"errors"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ParentFrameApplicabilityCurrentAdapterV3 is a stateless projection adapter.
// It keeps no Frame, Manifest, Generation, metadata or sealed-binding snapshot.
type ParentFrameApplicabilityCurrentAdapterV3 struct {
	Reader contextports.ContextParentFrameCurrentReaderV1
	Clock  func() time.Time
}

var _ runtimeports.OperationScopeEvidenceApplicabilityCurrentReaderV3 = ParentFrameApplicabilityCurrentAdapterV3{}

func (a ParentFrameApplicabilityCurrentAdapterV3) InspectOperationScopeEvidenceApplicabilityCurrentV3(ctx context.Context, fact runtimeports.OperationScopeEvidenceApplicabilityFactRefV3) (runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3, error) {
	if ctx == nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Context ParentFrame current inspection requires a context")
	}
	if err := ctx.Err(); err != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, mapContextCurrentErrorV3(err)
	}
	if a.Reader == nil || a.Clock == nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Context ParentFrame current reader and clock are required")
	}
	if err := fact.Validate(); err != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	if string(fact.Kind) != contract.ContextParentFrameApplicabilityKindV1 {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "applicability kind is not owned by Context ParentFrame reader")
	}
	source := contract.ContextParentFrameApplicabilitySourceCoordinateV1{
		Kind:     string(fact.Kind),
		ID:       fact.ID,
		Revision: uint64(fact.Revision),
		Digest:   contract.Digest(fact.Digest),
	}
	if err := source.Validate(); err != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Context ParentFrame source coordinate is invalid")
	}

	current, err := a.Reader.InspectContextParentFrameCurrentV1(ctx, source)
	if err != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, mapContextCurrentErrorV3(err)
	}
	if err := ctx.Err(); err != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, mapContextCurrentErrorV3(err)
	}
	now := a.Clock()
	if now.IsZero() {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Context applicability adapter clock returned zero")
	}
	if current.Source != source || current.ValidateAt(now.UnixNano()) != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "Context ParentFrame current projection is stale or mismatched")
	}

	projection := runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{
		Fact:                 fact,
		ExecutionScopeDigest: core.Digest(current.ExecutionScopeDigest),
		Current:              true,
		ExpiresUnixNano:      current.ExpiresUnixNano,
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", runtimeports.OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceApplicabilityCurrentProjectionV3", projection)
	if err != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	projection.Digest = digest
	if err := projection.Validate(fact, projection.ExecutionScopeDigest, now); err != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	return projection, nil
}

func mapContextCurrentErrorV3(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), errors.Is(err, contract.ErrUnknown):
		return core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "Context ParentFrame current inspection outcome is unknown")
	case errors.Is(err, contract.ErrNotFound):
		return core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "Context ParentFrame current metadata was not found")
	case errors.Is(err, contract.ErrUnavailable):
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Context ParentFrame current metadata is unavailable")
	case errors.Is(err, contract.ErrExpired):
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "Context ParentFrame current metadata expired")
	case errors.Is(err, contract.ErrConflict):
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceScopeConflict, "Context ParentFrame current metadata drifted")
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Context ParentFrame current inspection failed")
	}
}

package applicationadapter

import (
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// SettledToolResultExactChainV1 is an owner-local validation result. It is not
// a public wire DTO and deliberately contains no Context ContentRef, token
// estimate, sensitivity, recipe, cache identity or current pointer.
type SettledToolResultExactChainV1 struct {
	ApplicationRequest applicationcontract.SingleCallToolActionRequestV2
	ApplicationResult  applicationcontract.SingleCallToolActionResultV2
	ToolResult         toolcontract.ToolResultV2
	DomainResult       toolcontract.DomainResultFact
	Projection         toolcontract.SettledToolResultProjectionV1
	CheckedUnixNano    int64
	ExpiresUnixNano    int64
}

// BuildSettledToolResultExactChainV1 proves that Application's neutral result,
// Tool's settled result/current projection and Runtime's settlement closure all
// name the same exact facts. It stops before creating any Context-owned fact.
func BuildSettledToolResultExactChainV1(
	request applicationcontract.SingleCallToolActionRequestV2,
	result applicationcontract.SingleCallToolActionResultV2,
	toolResult toolcontract.ToolResultV2,
	domainResult toolcontract.DomainResultFact,
	projection toolcontract.SettledToolResultProjectionV1,
	now time.Time,
) (SettledToolResultExactChainV1, error) {
	chain := SettledToolResultExactChainV1{
		ApplicationRequest: request,
		ApplicationResult:  result,
		ToolResult:         toolResult,
		DomainResult:       domainResult,
		Projection:         projection.Clone(),
		CheckedUnixNano:    maximumUnixNanoV1(result.Coordinate.AssociationCheckedUnixNano, toolResult.Inspection.CheckedUnixNano, projection.CheckedUnixNano),
		ExpiresUnixNano:    minimumUnixNanoV1(request.ExpiresUnixNano, result.Coordinate.ExpiresUnixNano, toolResult.Inspection.ExpiresUnixNano, projection.ExpiresUnixNano),
	}
	if err := chain.ValidateCurrent(now); err != nil {
		return SettledToolResultExactChainV1{}, err
	}
	return chain, nil
}

func (c SettledToolResultExactChainV1) ValidateCurrent(now time.Time) error {
	if now.IsZero() || c.CheckedUnixNano <= 0 || c.ExpiresUnixNano <= c.CheckedUnixNano || now.UnixNano() < c.CheckedUnixNano || !now.Before(time.Unix(0, c.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "settled Tool result exact chain is not current")
	}
	if err := c.ApplicationRequest.ValidateCurrent(now); err != nil {
		return err
	}
	if err := c.ApplicationResult.ValidateCurrentFor(c.ApplicationRequest, now); err != nil {
		return err
	}
	if err := c.ToolResult.Validate(); err != nil {
		return err
	}
	if err := c.DomainResult.Validate(); err != nil {
		return err
	}
	if err := c.Projection.ValidateCurrent(c.ToolResult, now); err != nil {
		return err
	}

	coordinate := c.ApplicationResult.Coordinate
	owner := coordinate.ToolResult
	resultRef := toolcontract.ObjectRef{ID: c.ToolResult.ID, Revision: c.ToolResult.Revision, Digest: c.ToolResult.Digest}
	domainRef := toolcontract.ObjectRef{ID: c.DomainResult.ID, Revision: c.DomainResult.Revision, Digest: c.DomainResult.Digest}
	if owner.OwnerContractVersion != applicationcontract.SingleCallToolOwnerResultContractVersionV2 ||
		owner.ID != c.ToolResult.ID || owner.Revision != c.ToolResult.Revision || owner.Digest != c.ToolResult.Digest ||
		owner.ActionID != c.ApplicationRequest.Action.PendingSubject.PendingActionRef ||
		owner.ActionRevision != 1 ||
		owner.ActionDigest != c.ApplicationRequest.Action.Digest ||
		owner.ApplyID != c.ToolResult.Apply.ID || owner.ApplyRevision != c.ToolResult.Apply.Revision || owner.ApplyDigest != c.ToolResult.Apply.Digest ||
		owner.Schema != c.ToolResult.Schema || owner.PayloadDigest != c.ToolResult.PayloadDigest || owner.PayloadRevision != c.ToolResult.PayloadRevision || owner.FinalizedUnixNano != c.ToolResult.FinalizedUnixNano ||
		c.ToolResult.DomainResult != domainRef || c.DomainResult.Action != c.ToolResult.Action ||
		c.Projection.Result != resultRef {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Application, Tool result, DomainResult or projection exact refs differ")
	}
	if coordinate.Inspection.Digest != c.ToolResult.Inspection.Digest || owner.Inspection.Digest != c.ToolResult.Inspection.Digest || c.Projection.Inspection.Digest != c.ToolResult.Inspection.Digest ||
		!runtimeports.SameOperationSettlementRefV4(coordinate.Inspection.Settlement, c.ToolResult.Inspection.Settlement) ||
		!runtimeports.SameOperationSettlementEvidenceAssociationRefV4(coordinate.Association, c.ToolResult.Inspection.Association) ||
		!runtimeports.SameOperationSettlementEvidenceAssociationRefV4(coordinate.Inspection.Association, c.ToolResult.Inspection.Association) ||
		!runtimeports.SameOperationSettlementDomainResultFactRefV4(coordinate.Inspection.DomainResult, c.ToolResult.Inspection.DomainResult) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Runtime Inspection or Association exact chain differs")
	}
	if err := validateDomainResultRuntimeExactV1(c.DomainResult, c.ToolResult.Inspection.DomainResult); err != nil {
		return err
	}
	return nil
}

func validateDomainResultRuntimeExactV1(domain toolcontract.DomainResultFact, runtime runtimeports.OperationSettlementDomainResultFactRefV4) error {
	if domain.ID != runtime.ID || core.Revision(domain.Revision) != runtime.Revision || domain.Digest != runtime.Digest ||
		domain.AttemptID != runtime.Attempt.AttemptID || domain.Payload.Schema != runtime.Schema ||
		domain.Payload.ContentDigest != runtime.PayloadDigest || runtime.PayloadRevision != 1 ||
		domain.CreatedUnixNano != runtime.AuthoritativeTime {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Tool DomainResult and Runtime DomainResult semantics differ")
	}
	return nil
}

func maximumUnixNanoV1(value int64, values ...int64) int64 {
	for _, candidate := range values {
		if candidate > value {
			value = candidate
		}
	}
	return value
}

func minimumUnixNanoV1(value int64, values ...int64) int64 {
	for _, candidate := range values {
		if candidate < value {
			value = candidate
		}
	}
	return value
}

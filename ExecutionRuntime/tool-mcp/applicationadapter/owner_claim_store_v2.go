package applicationadapter

import (
	"context"
	"encoding/json"
	"sync"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

const toolOwnerSingleCallClaimVersionV2 = "praxis.tool-mcp.single-call-owner-claim/v2"

type ToolOwnerSingleCallClaimV2 struct {
	ContractVersion      string                                               `json:"contract_version"`
	ID                   string                                               `json:"id"`
	Revision             core.Revision                                        `json:"revision"`
	RequestID            string                                               `json:"request_id"`
	RequestDigest        core.Digest                                          `json:"request_digest"`
	ActionDigest         core.Digest                                          `json:"action_digest"`
	ExecutionScopeDigest core.Digest                                          `json:"execution_scope_digest"`
	BindingRef           toolcontract.SingleCallToolActionBindingCurrentRefV2 `json:"binding_ref"`
	CreatedUnixNano      int64                                                `json:"created_unix_nano"`
	Digest               core.Digest                                          `json:"digest"`
}

func (c ToolOwnerSingleCallClaimV2) ValidateFor(input ToolOwnerSingleCallExecutionV2) error {
	if err := input.Validate(); err != nil {
		return err
	}
	if c.ContractVersion != toolOwnerSingleCallClaimVersionV2 || c.Revision != 1 || c.RequestID != input.Request.ID || c.RequestDigest != input.Request.Digest || c.ActionDigest != input.Request.Action.Digest || c.ExecutionScopeDigest != input.Request.Action.ExecutionScopeDigest || c.BindingRef != input.Binding.Ref || c.CreatedUnixNano <= 0 {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner V2 claim immutable identity drifted")
	}
	id, err := deriveToolOwnerSingleCallClaimIDV2(input)
	if err != nil || id != c.ID {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner V2 claim ID drifted")
	}
	digest, err := c.DigestV2()
	if err != nil || digest != c.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Tool Owner V2 claim digest drifted")
	}
	return nil
}

func (c ToolOwnerSingleCallClaimV2) DigestV2() (core.Digest, error) {
	c.Digest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.single-call-owner-claim", "2.0.0", "ToolOwnerSingleCallClaimV2", c)
}

func newToolOwnerSingleCallClaimV2(input ToolOwnerSingleCallExecutionV2, createdUnixNano int64) (ToolOwnerSingleCallClaimV2, error) {
	id, err := deriveToolOwnerSingleCallClaimIDV2(input)
	if err != nil {
		return ToolOwnerSingleCallClaimV2{}, err
	}
	claim := ToolOwnerSingleCallClaimV2{ContractVersion: toolOwnerSingleCallClaimVersionV2, ID: id, Revision: 1, RequestID: input.Request.ID, RequestDigest: input.Request.Digest, ActionDigest: input.Request.Action.Digest, ExecutionScopeDigest: input.Request.Action.ExecutionScopeDigest, BindingRef: input.Binding.Ref, CreatedUnixNano: createdUnixNano}
	claim.Digest, err = claim.DigestV2()
	if err != nil {
		return ToolOwnerSingleCallClaimV2{}, err
	}
	return claim, claim.ValidateFor(input)
}

func deriveToolOwnerSingleCallClaimIDV2(input ToolOwnerSingleCallExecutionV2) (string, error) {
	if err := input.Validate(); err != nil {
		return "", err
	}
	return toolcontract.StableID("tool-owner-single-call-claim-v2", input.Request.ID, string(input.Request.Digest), string(input.Request.Action.ExecutionScopeDigest))
}

type ToolOwnerSingleCallClaimRecordV2 struct {
	Claim ToolOwnerSingleCallClaimV2     `json:"claim"`
	Input ToolOwnerSingleCallExecutionV2 `json:"input"`
}

func (r ToolOwnerSingleCallClaimRecordV2) Validate() error {
	return r.Claim.ValidateFor(r.Input)
}

func sameToolOwnerSingleCallClaimPayloadV2(left, right ToolOwnerSingleCallClaimRecordV2) (bool, error) {
	if err := left.Validate(); err != nil {
		return false, err
	}
	if err := right.Validate(); err != nil {
		return false, err
	}
	if left.Claim.ID != right.Claim.ID || left.Claim.RequestID != right.Claim.RequestID || left.Claim.RequestDigest != right.Claim.RequestDigest || left.Claim.ActionDigest != right.Claim.ActionDigest || left.Claim.ExecutionScopeDigest != right.Claim.ExecutionScopeDigest || left.Claim.BindingRef != right.Claim.BindingRef {
		return false, nil
	}
	leftInput, err := core.CanonicalJSONDigest("praxis.tool-mcp.single-call-owner-claim-input", "2.0.0", "ToolOwnerSingleCallExecutionV2", left.Input)
	if err != nil {
		return false, err
	}
	rightInput, err := core.CanonicalJSONDigest("praxis.tool-mcp.single-call-owner-claim-input", "2.0.0", "ToolOwnerSingleCallExecutionV2", right.Input)
	if err != nil {
		return false, err
	}
	return leftInput == rightInput, nil
}

type ToolOwnerSingleCallClaimStoreV2 interface {
	CreateToolOwnerSingleCallClaimV2(context.Context, ToolOwnerSingleCallClaimRecordV2) (ToolOwnerSingleCallClaimRecordV2, bool, error)
	InspectToolOwnerSingleCallClaimV2(context.Context, applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallClaimRecordV2, error)
}

// InMemoryToolOwnerSingleCallClaimStoreV2 is a reference/test owner store. A
// production composition must supply durable create-once semantics; this type
// makes no durability, backend or SLA claim.
type InMemoryToolOwnerSingleCallClaimStoreV2 struct {
	mu      sync.RWMutex
	records map[string]ToolOwnerSingleCallClaimRecordV2
}

func NewInMemoryToolOwnerSingleCallClaimStoreV2() *InMemoryToolOwnerSingleCallClaimStoreV2 {
	return &InMemoryToolOwnerSingleCallClaimStoreV2{records: make(map[string]ToolOwnerSingleCallClaimRecordV2)}
}

func (s *InMemoryToolOwnerSingleCallClaimStoreV2) CreateToolOwnerSingleCallClaimV2(ctx context.Context, record ToolOwnerSingleCallClaimRecordV2) (ToolOwnerSingleCallClaimRecordV2, bool, error) {
	if s == nil || isNilFlowDependencyV1(ctx) {
		return ToolOwnerSingleCallClaimRecordV2{}, false, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Tool Owner V2 claim store is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, false, err
	}
	if err := record.Validate(); err != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, false, err
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(record.Input.Request)
	if err != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, false, err
	}
	clone, err := cloneToolOwnerSingleCallClaimRecordV2(record)
	if err != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	mapKey := applicationResultKeyV2(key)
	if existing, ok := s.records[mapKey]; ok {
		same, compareErr := sameToolOwnerSingleCallClaimPayloadV2(existing, clone)
		if compareErr != nil {
			return ToolOwnerSingleCallClaimRecordV2{}, false, compareErr
		}
		if !same {
			return ToolOwnerSingleCallClaimRecordV2{}, false, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner V2 claim key binds different content")
		}
		if err = existing.Validate(); err != nil {
			return ToolOwnerSingleCallClaimRecordV2{}, false, err
		}
		winner, cloneErr := cloneToolOwnerSingleCallClaimRecordV2(existing)
		return winner, false, cloneErr
	}
	s.records[mapKey] = clone
	winner, cloneErr := cloneToolOwnerSingleCallClaimRecordV2(clone)
	return winner, true, cloneErr
}

func (s *InMemoryToolOwnerSingleCallClaimStoreV2) InspectToolOwnerSingleCallClaimV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallClaimRecordV2, error) {
	if s == nil || isNilFlowDependencyV1(ctx) {
		return ToolOwnerSingleCallClaimRecordV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Tool Owner V2 claim store is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, err
	}
	if err := key.Validate(); err != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, err
	}
	s.mu.RLock()
	record, ok := s.records[applicationResultKeyV2(key)]
	s.mu.RUnlock()
	if !ok {
		return ToolOwnerSingleCallClaimRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Tool Owner V2 claim not found")
	}
	if err := record.Validate(); err != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, err
	}
	expected, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(record.Input.Request)
	if err != nil || expected != key {
		return ToolOwnerSingleCallClaimRecordV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner V2 claim belongs to another request")
	}
	return cloneToolOwnerSingleCallClaimRecordV2(record)
}

func cloneToolOwnerSingleCallClaimRecordV2(value ToolOwnerSingleCallClaimRecordV2) (ToolOwnerSingleCallClaimRecordV2, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, err
	}
	var clone ToolOwnerSingleCallClaimRecordV2
	if err = json.Unmarshal(payload, &clone); err != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, err
	}
	return clone, nil
}

var _ ToolOwnerSingleCallClaimStoreV2 = (*InMemoryToolOwnerSingleCallClaimStoreV2)(nil)

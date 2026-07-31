package applicationadapter

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	toolsqlite "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/storage/sqlite"
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
	_ = createdUnixNano
	id, err := deriveToolOwnerSingleCallClaimIDV2(input)
	if err != nil {
		return ToolOwnerSingleCallClaimV2{}, err
	}
	claim := ToolOwnerSingleCallClaimV2{ContractVersion: toolOwnerSingleCallClaimVersionV2, ID: id, Revision: 1, RequestID: input.Request.ID, RequestDigest: input.Request.Digest, ActionDigest: input.Request.Action.Digest, ExecutionScopeDigest: input.Request.Action.ExecutionScopeDigest, BindingRef: input.Binding.Ref, CreatedUnixNano: input.Request.CreatedUnixNano}
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
	return reflect.DeepEqual(left, right), nil
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

// SQLiteToolOwnerSingleCallClaimStoreV2 is the typed Tool Owner adapter over
// the SQLite package's cycle-free exact row store.
type SQLiteToolOwnerSingleCallClaimStoreV2 struct {
	raw *toolsqlite.OwnerClaimExecutionStoreV2
}

func NewSQLiteToolOwnerSingleCallClaimStoreV2(raw *toolsqlite.OwnerClaimExecutionStoreV2) (*SQLiteToolOwnerSingleCallClaimStoreV2, error) {
	if raw == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool Owner SQLite claim row store is required")
	}
	return &SQLiteToolOwnerSingleCallClaimStoreV2{raw: raw}, nil
}

func (s *SQLiteToolOwnerSingleCallClaimStoreV2) CreateToolOwnerSingleCallClaimV2(ctx context.Context, record ToolOwnerSingleCallClaimRecordV2) (ToolOwnerSingleCallClaimRecordV2, bool, error) {
	key, row, err := encodeSQLiteClaimRowV2(record)
	if err != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, false, err
	}
	winner, created, err := s.raw.CreateClaimRowV2(ctx, row)
	if err != nil {
		if core.HasCategory(err, core.ErrorIndeterminate) {
			recoveryCtx, cancel := boundedOwnerLocalRecoveryContextV2(ctx, defaultToolOwnerRecoveryTimeoutV2)
			_, _ = s.InspectToolOwnerSingleCallClaimV2(recoveryCtx, key)
			cancel()
		}
		return ToolOwnerSingleCallClaimRecordV2{}, false, err
	}
	decoded, err := decodeSQLiteClaimRowV2(winner)
	if err != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, false, err
	}
	same, err := sameToolOwnerSingleCallClaimPayloadV2(decoded, record)
	if err != nil || !same {
		if err != nil {
			return ToolOwnerSingleCallClaimRecordV2{}, false, err
		}
		return ToolOwnerSingleCallClaimRecordV2{}, false, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner SQLite claim winner binds different content")
	}
	return decoded, created, nil
}

func (s *SQLiteToolOwnerSingleCallClaimStoreV2) InspectToolOwnerSingleCallClaimV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallClaimRecordV2, error) {
	if s == nil || s.raw == nil || key.Validate() != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Owner SQLite claim Inspect is invalid")
	}
	row, err := s.raw.InspectClaimRowV2(ctx, sqliteRequestKeyRowV2(key))
	if err != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, err
	}
	record, err := decodeSQLiteClaimRowV2(row)
	if err != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, err
	}
	expected, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(record.Input.Request)
	if err != nil || expected != key {
		return ToolOwnerSingleCallClaimRecordV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner SQLite claim request key drifted")
	}
	return record, nil
}

func encodeSQLiteClaimRowV2(record ToolOwnerSingleCallClaimRecordV2) (applicationcontract.SingleCallToolActionInspectKeyV2, toolsqlite.OwnerClaimRowV2, error) {
	if err := record.Validate(); err != nil {
		return applicationcontract.SingleCallToolActionInspectKeyV2{}, toolsqlite.OwnerClaimRowV2{}, err
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(record.Input.Request)
	if err != nil {
		return applicationcontract.SingleCallToolActionInspectKeyV2{}, toolsqlite.OwnerClaimRowV2{}, err
	}
	inputDigest, err := ComputeToolOwnerSingleCallExecutionInputDigestV2(record.Input)
	if err != nil {
		return applicationcontract.SingleCallToolActionInspectKeyV2{}, toolsqlite.OwnerClaimRowV2{}, err
	}
	claimJSON, err := json.Marshal(record.Claim)
	if err != nil {
		return applicationcontract.SingleCallToolActionInspectKeyV2{}, toolsqlite.OwnerClaimRowV2{}, err
	}
	inputJSON, err := json.Marshal(record.Input)
	if err != nil {
		return applicationcontract.SingleCallToolActionInspectKeyV2{}, toolsqlite.OwnerClaimRowV2{}, err
	}
	rowDigest, err := core.CanonicalJSONDigest("praxis.tool-mcp.sqlite-row", "v1", "ToolOwnerSingleCallClaimRecordV2", struct {
		Claim       ToolOwnerSingleCallClaimV2     `json:"claim"`
		Input       ToolOwnerSingleCallExecutionV2 `json:"input"`
		InputDigest core.Digest                    `json:"input_digest"`
	}{record.Claim, record.Input, inputDigest})
	if err != nil {
		return applicationcontract.SingleCallToolActionInspectKeyV2{}, toolsqlite.OwnerClaimRowV2{}, err
	}
	return key, toolsqlite.OwnerClaimRowV2{ClaimID: record.Claim.ID, ClaimRevision: int64(record.Claim.Revision), ClaimDigest: string(record.Claim.Digest), Request: sqliteRequestKeyRowV2(key), BindingID: record.Claim.BindingRef.ID, BindingRevision: int64(record.Claim.BindingRef.Revision), BindingDigest: string(record.Claim.BindingRef.Digest), InputDigest: string(inputDigest), ClaimJSON: claimJSON, InputJSON: inputJSON, RowDigest: string(rowDigest)}, nil
}

func decodeSQLiteClaimRowV2(row toolsqlite.OwnerClaimRowV2) (ToolOwnerSingleCallClaimRecordV2, error) {
	var record ToolOwnerSingleCallClaimRecordV2
	if core.DecodeStrictJSON(row.ClaimJSON, &record.Claim) != nil || core.DecodeStrictJSON(row.InputJSON, &record.Input) != nil {
		return ToolOwnerSingleCallClaimRecordV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "Tool Owner SQLite claim JSON is invalid")
	}
	_, expected, err := encodeSQLiteClaimRowV2(record)
	if err != nil || !reflectSQLiteClaimRowV2(row, expected) {
		return ToolOwnerSingleCallClaimRecordV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Tool Owner SQLite claim exact columns or row digest drifted")
	}
	return cloneToolOwnerSingleCallClaimRecordV2(record)
}

func reflectSQLiteClaimRowV2(left, right toolsqlite.OwnerClaimRowV2) bool {
	return left.ClaimID == right.ClaimID && left.ClaimRevision == right.ClaimRevision && left.ClaimDigest == right.ClaimDigest &&
		left.Request.RequestID == right.Request.RequestID && left.Request.RequestRevision == right.Request.RequestRevision &&
		left.Request.RequestDigest == right.Request.RequestDigest && left.Request.ActionCoordinateDigest == right.Request.ActionCoordinateDigest &&
		left.Request.ExecutionScopeDigest == right.Request.ExecutionScopeDigest &&
		left.BindingID == right.BindingID && left.BindingRevision == right.BindingRevision && left.BindingDigest == right.BindingDigest &&
		left.InputDigest == right.InputDigest && string(left.ClaimJSON) == string(right.ClaimJSON) &&
		string(left.InputJSON) == string(right.InputJSON) && left.RowDigest == right.RowDigest
}

func sqliteRequestKeyRowV2(key applicationcontract.SingleCallToolActionInspectKeyV2) toolsqlite.OwnerRequestKeyRowV2 {
	return toolsqlite.OwnerRequestKeyRowV2{RequestKeyDigest: string(key.Digest), RequestID: key.RequestID, RequestRevision: int64(key.RequestRevision), RequestDigest: string(key.RequestDigest), ActionCoordinateDigest: string(key.ActionCoordinateDigest), ExecutionScopeDigest: string(key.ScopeDigest)}
}

var _ ToolOwnerSingleCallClaimStoreV2 = (*SQLiteToolOwnerSingleCallClaimStoreV2)(nil)

package applicationadapter

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"sync"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	toolsqlite "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/storage/sqlite"
)

const ToolOwnerSingleCallExecutionStateContractVersionV2 = "praxis.tool-mcp.single-call-execution-state/v2"
const ToolOwnerSingleCallEntryLeaseContractVersionV2 = "praxis.tool-mcp.single-call-entry-lease/v2"

type ToolOwnerSingleCallExecutionStateKindV2 string

const (
	ToolOwnerExecutionStartCommittedV2 ToolOwnerSingleCallExecutionStateKindV2 = "start_committed"
	ToolOwnerExecutionInspectOnlyV2    ToolOwnerSingleCallExecutionStateKindV2 = "inspect_only"
	ToolOwnerExecutionSettledV2        ToolOwnerSingleCallExecutionStateKindV2 = "settled"
)

type ToolOwnerSingleCallUnknownClassV2 string

const (
	ToolOwnerEntryOutcomeUnknownV2     ToolOwnerSingleCallUnknownClassV2 = "entry_outcome_unknown"
	ToolOwnerInspectionIndeterminateV2 ToolOwnerSingleCallUnknownClassV2 = "inspection_indeterminate"
)

type ToolOwnerSingleCallExecutionUnknownV2 struct {
	Class          ToolOwnerSingleCallUnknownClassV2 `json:"class"`
	ErrorDigest    core.Digest                       `json:"error_digest"`
	MarkedUnixNano int64                             `json:"marked_unix_nano"`
}

func (v ToolOwnerSingleCallExecutionUnknownV2) Validate() error {
	if (v.Class != ToolOwnerEntryOutcomeUnknownV2 && v.Class != ToolOwnerInspectionIndeterminateV2) || v.ErrorDigest.Validate() != nil || v.MarkedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Tool Owner execution unknown marker is invalid")
	}
	return nil
}

type ToolOwnerSingleCallExecutionStateV2 struct {
	ContractVersion      string                                               `json:"contract_version"`
	ID                   string                                               `json:"id"`
	Revision             core.Revision                                        `json:"revision"`
	Digest               core.Digest                                          `json:"digest"`
	ClaimRef             toolcontract.ObjectRef                               `json:"claim_ref"`
	RequestKey           applicationcontract.SingleCallToolActionInspectKeyV2 `json:"request_key"`
	RequestDigest        core.Digest                                          `json:"request_digest"`
	ActionDigest         core.Digest                                          `json:"action_digest"`
	ExecutionScopeDigest core.Digest                                          `json:"execution_scope_digest"`
	BindingRef           toolcontract.SingleCallToolActionBindingCurrentRefV2 `json:"binding_ref"`
	ExecutionInputDigest core.Digest                                          `json:"execution_input_digest"`
	ExecutionAttemptID   string                                               `json:"execution_attempt_id"`
	State                ToolOwnerSingleCallExecutionStateKindV2              `json:"state"`
	Result               *toolcontract.ObjectRef                              `json:"result,omitempty"`
	Unknown              *ToolOwnerSingleCallExecutionUnknownV2               `json:"unknown,omitempty"`
	CreatedUnixNano      int64                                                `json:"created_unix_nano"`
	UpdatedUnixNano      int64                                                `json:"updated_unix_nano"`
	ExpiresUnixNano      int64                                                `json:"expires_unix_nano"`
}

func (v ToolOwnerSingleCallExecutionStateV2) RefV2() toolcontract.ObjectRef {
	return toolcontract.ObjectRef{ID: v.ID, Revision: v.Revision, Digest: v.Digest}
}

func (v ToolOwnerSingleCallExecutionStateV2) DigestV2() (core.Digest, error) {
	v.Digest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.single-call-execution-state", "2.0.0", "ToolOwnerSingleCallExecutionStateV2", v)
}

func (v ToolOwnerSingleCallExecutionStateV2) Validate() error {
	if v.ContractVersion != ToolOwnerSingleCallExecutionStateContractVersionV2 || toolcontract.ValidateStableID(v.ID) != nil || v.Revision == 0 || v.ClaimRef.Validate() != nil || v.RequestKey.Validate() != nil || v.RequestDigest.Validate() != nil || v.ActionDigest.Validate() != nil || v.ExecutionScopeDigest.Validate() != nil || v.BindingRef.Validate() != nil || v.ExecutionInputDigest.Validate() != nil || toolcontract.ValidateStableID(v.ExecutionAttemptID) != nil || v.CreatedUnixNano <= 0 || v.UpdatedUnixNano < v.CreatedUnixNano || v.ExpiresUnixNano <= v.UpdatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Tool Owner execution state is invalid")
	}
	if v.RequestKey.RequestDigest != v.RequestDigest || v.RequestKey.ActionCoordinateDigest != v.ActionDigest || v.RequestKey.ScopeDigest != v.ExecutionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Owner execution state repeated request coordinates drifted")
	}
	id, err := toolcontract.StableID("tool-owner-execution-state-v2", v.ClaimRef.ID, string(v.ExecutionInputDigest))
	if err != nil || id != v.ID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Owner execution state ID drifted")
	}
	attemptID, err := toolcontract.StableID("tool-owner-execution-attempt-v2", v.ClaimRef.ID, string(v.ExecutionInputDigest))
	if err != nil || attemptID != v.ExecutionAttemptID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Owner execution attempt ID drifted")
	}
	switch v.State {
	case ToolOwnerExecutionStartCommittedV2:
		if v.Revision != 1 || v.Result != nil || v.Unknown != nil {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "start_committed execution state cannot carry result or unknown")
		}
	case ToolOwnerExecutionInspectOnlyV2:
		if v.Revision != 2 || v.Result != nil || v.Unknown == nil || v.Unknown.Validate() != nil || v.Unknown.MarkedUnixNano != v.UpdatedUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "inspect_only execution state requires one exact unknown marker")
		}
	case ToolOwnerExecutionSettledV2:
		if (v.Revision != 2 && v.Revision != 3) || v.Result == nil || v.Result.Validate() != nil || v.Unknown != nil {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "settled execution state requires one exact result")
		}
	default:
		return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner execution state kind is unsupported")
	}
	digest, err := v.DigestV2()
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Tool Owner execution state digest drifted")
	}
	return nil
}

func ComputeToolOwnerSingleCallExecutionInputDigestV2(input ToolOwnerSingleCallExecutionV2) (core.Digest, error) {
	if err := input.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.tool-mcp.single-call-owner-claim-input", "2.0.0", "ToolOwnerSingleCallExecutionV2", input)
}

func NewToolOwnerSingleCallExecutionStartV2(record ToolOwnerSingleCallClaimRecordV2, nowUnixNano int64) (ToolOwnerSingleCallExecutionStateV2, error) {
	if err := record.Validate(); err != nil || nowUnixNano <= 0 {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Tool Owner execution start input is invalid")
	}
	nowUnixNano = record.Claim.CreatedUnixNano
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(record.Input.Request)
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	inputDigest, err := ComputeToolOwnerSingleCallExecutionInputDigestV2(record.Input)
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	id, err := toolcontract.StableID("tool-owner-execution-state-v2", record.Claim.ID, string(inputDigest))
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	attemptID, err := toolcontract.StableID("tool-owner-execution-attempt-v2", record.Claim.ID, string(inputDigest))
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	state := ToolOwnerSingleCallExecutionStateV2{ContractVersion: ToolOwnerSingleCallExecutionStateContractVersionV2, ID: id, Revision: 1, ClaimRef: toolcontract.ObjectRef{ID: record.Claim.ID, Revision: record.Claim.Revision, Digest: record.Claim.Digest}, RequestKey: key, RequestDigest: record.Claim.RequestDigest, ActionDigest: record.Claim.ActionDigest, ExecutionScopeDigest: record.Claim.ExecutionScopeDigest, BindingRef: record.Claim.BindingRef, ExecutionInputDigest: inputDigest, ExecutionAttemptID: attemptID, State: ToolOwnerExecutionStartCommittedV2, CreatedUnixNano: nowUnixNano, UpdatedUnixNano: nowUnixNano, ExpiresUnixNano: record.Input.Binding.ExpiresUnixNano}
	state.Digest, err = state.DigestV2()
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	return state, state.Validate()
}

type ToolOwnerSingleCallExecutionStateStoreV2 interface {
	CreateExecutionStartV2(context.Context, ToolOwnerSingleCallExecutionStateV2) (ToolOwnerSingleCallExecutionStateV2, bool, error)
	InspectExecutionStateV2(context.Context, applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallExecutionStateV2, error)
	AdvanceExecutionInspectOnlyV2(context.Context, toolcontract.ObjectRef, ToolOwnerSingleCallExecutionUnknownV2) (ToolOwnerSingleCallExecutionStateV2, error)
	AdvanceExecutionSettledV2(context.Context, toolcontract.ObjectRef, toolcontract.ObjectRef, int64) (ToolOwnerSingleCallExecutionStateV2, error)
}

type ToolOwnerSingleCallEntryLeasePhaseV2 string

const (
	ToolOwnerEntryStartOrInspectV2        ToolOwnerSingleCallEntryLeasePhaseV2 = "start_or_inspect"
	ToolOwnerEntryInspectV2               ToolOwnerSingleCallEntryLeasePhaseV2 = "inspect"
	ToolOwnerEntryHandoffStartOrInspectV2 ToolOwnerSingleCallEntryLeasePhaseV2 = "handoff_start_or_inspect"
	ToolOwnerEntryHandoffInspectV2        ToolOwnerSingleCallEntryLeasePhaseV2 = "handoff_inspect"
)

type ToolOwnerSingleCallEntryLeaseV2 struct {
	ContractVersion      string                                               `json:"contract_version"`
	ID                   string                                               `json:"id"`
	Revision             core.Revision                                        `json:"revision"`
	Digest               core.Digest                                          `json:"digest"`
	RequestKey           applicationcontract.SingleCallToolActionInspectKeyV2 `json:"request_key"`
	RequestDigest        core.Digest                                          `json:"request_digest"`
	ExecutionInputDigest core.Digest                                          `json:"execution_input_digest"`
	ExecutionAttemptID   string                                               `json:"execution_attempt_id"`
	HolderIncarnationID  string                                               `json:"holder_incarnation_id"`
	Phase                ToolOwnerSingleCallEntryLeasePhaseV2                 `json:"phase"`
	AcquiredUnixNano     int64                                                `json:"acquired_unix_nano"`
	ExpiresUnixNano      int64                                                `json:"expires_unix_nano"`
}

func (v ToolOwnerSingleCallEntryLeaseV2) RefV2() toolcontract.ObjectRef {
	return toolcontract.ObjectRef{ID: v.ID, Revision: v.Revision, Digest: v.Digest}
}

func (v ToolOwnerSingleCallEntryLeaseV2) DigestV2() (core.Digest, error) {
	v.Digest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.single-call-entry-lease", "2.0.0", "ToolOwnerSingleCallEntryLeaseV2", v)
}

func (v ToolOwnerSingleCallEntryLeaseV2) Validate() error {
	if v.ContractVersion != ToolOwnerSingleCallEntryLeaseContractVersionV2 || toolcontract.ValidateStableID(v.ID) != nil ||
		v.Revision == 0 || v.RequestKey.Validate() != nil || v.RequestDigest.Validate() != nil ||
		v.ExecutionInputDigest.Validate() != nil || toolcontract.ValidateStableID(v.ExecutionAttemptID) != nil ||
		toolcontract.ValidateStableID(v.HolderIncarnationID) != nil ||
		!validEntryLeasePhaseV2(v.Phase) ||
		v.AcquiredUnixNano <= 0 || v.ExpiresUnixNano <= v.AcquiredUnixNano ||
		v.RequestKey.RequestDigest != v.RequestDigest {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Tool Owner entry lease is invalid")
	}
	id, err := toolcontract.StableID("tool-owner-entry-lease-v2", v.ExecutionAttemptID)
	if err != nil || id != v.ID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Owner entry lease ID drifted")
	}
	digest, err := v.DigestV2()
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Tool Owner entry lease digest drifted")
	}
	return nil
}

func (v ToolOwnerSingleCallEntryLeaseV2) ValidateCurrent(now time.Time) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if isEntryLeaseHandoffPhaseV2(v.Phase) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "Tool Owner entry lease has been handed off")
	}
	if now.IsZero() || now.UnixNano() < v.AcquiredUnixNano {
		return core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "Tool Owner entry lease clock regressed")
	}
	if now.UnixNano() >= v.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Tool Owner entry lease expired")
	}
	return nil
}

type ToolOwnerSingleCallEntryLeaseAcquireV2 struct {
	RequestKey           applicationcontract.SingleCallToolActionInspectKeyV2
	RequestDigest        core.Digest
	ExecutionInputDigest core.Digest
	ExecutionAttemptID   string
	HolderIncarnationID  string
	Phase                ToolOwnerSingleCallEntryLeasePhaseV2
	AcquiredUnixNano     int64
	ExpiresUnixNano      int64
}

func (v ToolOwnerSingleCallEntryLeaseAcquireV2) Validate() error {
	if v.RequestKey.Validate() != nil || v.RequestDigest.Validate() != nil || v.ExecutionInputDigest.Validate() != nil ||
		toolcontract.ValidateStableID(v.ExecutionAttemptID) != nil || toolcontract.ValidateStableID(v.HolderIncarnationID) != nil ||
		(v.Phase != ToolOwnerEntryStartOrInspectV2 && v.Phase != ToolOwnerEntryInspectV2) ||
		v.AcquiredUnixNano <= 0 || v.ExpiresUnixNano <= v.AcquiredUnixNano || v.RequestKey.RequestDigest != v.RequestDigest {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Tool Owner entry lease acquisition is invalid")
	}
	return nil
}

type ToolOwnerSingleCallEntryLeaseStoreV2 interface {
	TryAcquireExecutionEntryLeaseV2(context.Context, ToolOwnerSingleCallEntryLeaseAcquireV2) (ToolOwnerSingleCallEntryLeaseV2, bool, error)
	InspectExecutionEntryLeaseV2(context.Context, string) (ToolOwnerSingleCallEntryLeaseV2, error)
	AdvanceExecutionEntryLeaseHandoffV2(context.Context, ToolOwnerSingleCallEntryLeaseV2, ToolOwnerSingleCallEntryLeasePhaseV2, int64) (ToolOwnerSingleCallEntryLeaseV2, error)
}

type InMemoryToolOwnerSingleCallExecutionStateStoreV2 struct {
	mu           sync.RWMutex
	heads        map[string]ToolOwnerSingleCallExecutionStateV2
	history      map[string]ToolOwnerSingleCallExecutionStateV2
	leaseHeads   map[string]ToolOwnerSingleCallEntryLeaseV2
	leaseHistory map[string]ToolOwnerSingleCallEntryLeaseV2
}

func NewInMemoryToolOwnerSingleCallExecutionStateStoreV2() *InMemoryToolOwnerSingleCallExecutionStateStoreV2 {
	return &InMemoryToolOwnerSingleCallExecutionStateStoreV2{
		heads: make(map[string]ToolOwnerSingleCallExecutionStateV2), history: make(map[string]ToolOwnerSingleCallExecutionStateV2),
		leaseHeads: make(map[string]ToolOwnerSingleCallEntryLeaseV2), leaseHistory: make(map[string]ToolOwnerSingleCallEntryLeaseV2),
	}
}

func (s *InMemoryToolOwnerSingleCallExecutionStateStoreV2) CreateExecutionStartV2(ctx context.Context, state ToolOwnerSingleCallExecutionStateV2) (ToolOwnerSingleCallExecutionStateV2, bool, error) {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, false, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Tool Owner execution state store is unavailable")
	}
	if err := state.Validate(); err != nil || state.Revision != 1 || state.State != ToolOwnerExecutionStartCommittedV2 {
		return ToolOwnerSingleCallExecutionStateV2{}, false, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "Tool Owner execution start state is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(state.RequestKey.Digest)
	if existing, ok := s.heads[key]; ok {
		if initial, found := s.history[executionHistoryKeyV2(state)]; found && reflect.DeepEqual(initial, state) {
			return cloneExecutionStateV2(existing), false, nil
		}
		return ToolOwnerSingleCallExecutionStateV2{}, false, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner execution request key binds different content")
	}
	s.heads[key], s.history[executionHistoryKeyV2(state)] = cloneExecutionStateV2(state), cloneExecutionStateV2(state)
	return cloneExecutionStateV2(state), true, nil
}

func (s *InMemoryToolOwnerSingleCallExecutionStateStoreV2) InspectExecutionStateV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	if s == nil || ctx == nil || ctx.Err() != nil || key.Validate() != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Owner execution state Inspect is invalid")
	}
	s.mu.RLock()
	state, ok := s.heads[string(key.Digest)]
	s.mu.RUnlock()
	if !ok {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Tool Owner execution state not found")
	}
	if state.RequestKey != key || state.Validate() != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Owner execution state row drifted")
	}
	return cloneExecutionStateV2(state), nil
}

func (s *InMemoryToolOwnerSingleCallExecutionStateStoreV2) AdvanceExecutionInspectOnlyV2(ctx context.Context, expected toolcontract.ObjectRef, unknown ToolOwnerSingleCallExecutionUnknownV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	if s == nil || ctx == nil || ctx.Err() != nil || expected.Validate() != nil || unknown.Validate() != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "Tool Owner inspect-only transition is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, key, err := s.currentByExpectedV2(expected)
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	if current.State == ToolOwnerExecutionInspectOnlyV2 && reflect.DeepEqual(current.Unknown, &unknown) {
		return cloneExecutionStateV2(current), nil
	}
	if current.State != ToolOwnerExecutionStartCommittedV2 || unknown.MarkedUnixNano < current.UpdatedUnixNano || unknown.MarkedUnixNano >= current.ExpiresUnixNano {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner inspect-only transition source drifted")
	}
	next := current
	next.Revision++
	next.State, next.Unknown, next.Result, next.UpdatedUnixNano = ToolOwnerExecutionInspectOnlyV2, &unknown, nil, unknown.MarkedUnixNano
	next.Digest, err = next.DigestV2()
	if err != nil || next.Validate() != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Tool Owner inspect-only state could not be sealed")
	}
	s.heads[key], s.history[executionHistoryKeyV2(next)] = cloneExecutionStateV2(next), cloneExecutionStateV2(next)
	return cloneExecutionStateV2(next), nil
}

func (s *InMemoryToolOwnerSingleCallExecutionStateStoreV2) AdvanceExecutionSettledV2(ctx context.Context, expected toolcontract.ObjectRef, result toolcontract.ObjectRef, nowUnixNano int64) (ToolOwnerSingleCallExecutionStateV2, error) {
	if s == nil || ctx == nil || ctx.Err() != nil || expected.Validate() != nil || result.Validate() != nil || nowUnixNano <= 0 {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "Tool Owner settled transition is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, key, err := s.currentByExpectedV2(expected)
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	if current.State == ToolOwnerExecutionSettledV2 {
		if current.Result != nil && *current.Result == result {
			return cloneExecutionStateV2(current), nil
		}
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner settled replay binds different result")
	}
	if (current.State != ToolOwnerExecutionStartCommittedV2 && current.State != ToolOwnerExecutionInspectOnlyV2) || nowUnixNano < current.UpdatedUnixNano || nowUnixNano >= current.ExpiresUnixNano {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner settled transition source drifted")
	}
	next := current
	next.Revision++
	next.State, next.Result, next.Unknown, next.UpdatedUnixNano = ToolOwnerExecutionSettledV2, &result, nil, nowUnixNano
	next.Digest, err = next.DigestV2()
	if err != nil || next.Validate() != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Tool Owner settled state could not be sealed")
	}
	s.heads[key], s.history[executionHistoryKeyV2(next)] = cloneExecutionStateV2(next), cloneExecutionStateV2(next)
	return cloneExecutionStateV2(next), nil
}

func (s *InMemoryToolOwnerSingleCallExecutionStateStoreV2) currentByExpectedV2(expected toolcontract.ObjectRef) (ToolOwnerSingleCallExecutionStateV2, string, error) {
	for key, current := range s.heads {
		if current.ID == expected.ID {
			if current.RefV2() != expected {
				return ToolOwnerSingleCallExecutionStateV2{}, "", core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner execution state CAS source drifted")
			}
			return current, key, nil
		}
	}
	return ToolOwnerSingleCallExecutionStateV2{}, "", core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Tool Owner execution state not found")
}

func sameExecutionIdentityV2(left, right ToolOwnerSingleCallExecutionStateV2) bool {
	return left.ID == right.ID && left.ClaimRef == right.ClaimRef && left.RequestKey == right.RequestKey && left.RequestDigest == right.RequestDigest && left.ActionDigest == right.ActionDigest && left.ExecutionScopeDigest == right.ExecutionScopeDigest && left.BindingRef == right.BindingRef && left.ExecutionInputDigest == right.ExecutionInputDigest && left.ExecutionAttemptID == right.ExecutionAttemptID && left.CreatedUnixNano == right.CreatedUnixNano && left.ExpiresUnixNano == right.ExpiresUnixNano
}

func executionHistoryKeyV2(state ToolOwnerSingleCallExecutionStateV2) string {
	return state.ID + "\x00" + strconv.FormatUint(uint64(state.Revision), 10) + "\x00" + string(state.Digest)
}

func cloneExecutionStateV2(state ToolOwnerSingleCallExecutionStateV2) ToolOwnerSingleCallExecutionStateV2 {
	body, _ := json.Marshal(state)
	var clone ToolOwnerSingleCallExecutionStateV2
	_ = core.DecodeStrictJSON(body, &clone)
	return clone
}

func (s *InMemoryToolOwnerSingleCallExecutionStateStoreV2) TryAcquireExecutionEntryLeaseV2(ctx context.Context, request ToolOwnerSingleCallEntryLeaseAcquireV2) (ToolOwnerSingleCallEntryLeaseV2, bool, error) {
	if s == nil || ctx == nil || ctx.Err() != nil || request.Validate() != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, false, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Owner entry lease acquisition is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, found := s.leaseHeads[request.ExecutionAttemptID]
	if found {
		if !sameEntryLeaseIdentityV2(current, request) {
			return ToolOwnerSingleCallEntryLeaseV2{}, false, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner entry lease binds different execution identity")
		}
		if request.AcquiredUnixNano < current.AcquiredUnixNano {
			return ToolOwnerSingleCallEntryLeaseV2{}, false, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "Tool Owner entry lease clock regressed")
		}
		if !isEntryLeaseHandoffPhaseV2(current.Phase) && request.AcquiredUnixNano < current.ExpiresUnixNano {
			return cloneEntryLeaseV2(current), false, nil
		}
		if isEntryLeaseHandoffPhaseV2(current.Phase) && request.Phase != activeEntryLeasePhaseV2(current.Phase) {
			return ToolOwnerSingleCallEntryLeaseV2{}, false, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner entry lease handoff only permits its exact next phase")
		}
	}
	next, err := sealEntryLeaseV2(request, 1)
	if found {
		next, err = sealEntryLeaseV2(request, current.Revision+1)
	}
	if err != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, false, err
	}
	s.leaseHeads[request.ExecutionAttemptID] = cloneEntryLeaseV2(next)
	s.leaseHistory[entryLeaseHistoryKeyV2(next)] = cloneEntryLeaseV2(next)
	return cloneEntryLeaseV2(next), true, nil
}

func (s *InMemoryToolOwnerSingleCallExecutionStateStoreV2) InspectExecutionEntryLeaseV2(ctx context.Context, executionAttemptID string) (ToolOwnerSingleCallEntryLeaseV2, error) {
	if s == nil || ctx == nil || ctx.Err() != nil || toolcontract.ValidateStableID(executionAttemptID) != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Owner entry lease Inspect is invalid")
	}
	s.mu.RLock()
	lease, ok := s.leaseHeads[executionAttemptID]
	s.mu.RUnlock()
	if !ok {
		return ToolOwnerSingleCallEntryLeaseV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Tool Owner entry lease not found")
	}
	if lease.ExecutionAttemptID != executionAttemptID || lease.Validate() != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Owner entry lease row drifted")
	}
	return cloneEntryLeaseV2(lease), nil
}

func (s *InMemoryToolOwnerSingleCallExecutionStateStoreV2) AdvanceExecutionEntryLeaseHandoffV2(ctx context.Context, expected ToolOwnerSingleCallEntryLeaseV2, nextPhase ToolOwnerSingleCallEntryLeasePhaseV2, nowUnixNano int64) (ToolOwnerSingleCallEntryLeaseV2, error) {
	if s == nil || ctx == nil || ctx.Err() != nil || expected.Validate() != nil || !isActiveEntryLeasePhaseV2(nextPhase) || nowUnixNano <= 0 {
		return ToolOwnerSingleCallEntryLeaseV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "Tool Owner entry lease handoff is invalid")
	}
	next, err := sealEntryLeaseHandoffV2(expected, nextPhase, nowUnixNano)
	if err != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var current ToolOwnerSingleCallEntryLeaseV2
	found := false
	current, found = s.leaseHeads[expected.ExecutionAttemptID]
	if !found {
		return ToolOwnerSingleCallEntryLeaseV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Tool Owner entry lease handoff source is absent")
	}
	if current.RefV2() != expected.RefV2() {
		if current.RefV2() == next.RefV2() && reflect.DeepEqual(current, next) {
			return cloneEntryLeaseV2(current), nil
		}
		return ToolOwnerSingleCallEntryLeaseV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner entry lease handoff CAS source drifted")
	}
	s.leaseHeads[current.ExecutionAttemptID] = cloneEntryLeaseV2(next)
	s.leaseHistory[entryLeaseHistoryKeyV2(next)] = cloneEntryLeaseV2(next)
	return cloneEntryLeaseV2(next), nil
}

func sealEntryLeaseV2(request ToolOwnerSingleCallEntryLeaseAcquireV2, revision core.Revision) (ToolOwnerSingleCallEntryLeaseV2, error) {
	if err := request.Validate(); err != nil || revision == 0 {
		return ToolOwnerSingleCallEntryLeaseV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Tool Owner entry lease seal input is invalid")
	}
	id, err := toolcontract.StableID("tool-owner-entry-lease-v2", request.ExecutionAttemptID)
	if err != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, err
	}
	lease := ToolOwnerSingleCallEntryLeaseV2{
		ContractVersion: ToolOwnerSingleCallEntryLeaseContractVersionV2, ID: id, Revision: revision,
		RequestKey: request.RequestKey, RequestDigest: request.RequestDigest, ExecutionInputDigest: request.ExecutionInputDigest,
		ExecutionAttemptID: request.ExecutionAttemptID, HolderIncarnationID: request.HolderIncarnationID, Phase: request.Phase,
		AcquiredUnixNano: request.AcquiredUnixNano, ExpiresUnixNano: request.ExpiresUnixNano,
	}
	lease.Digest, err = lease.DigestV2()
	if err != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, err
	}
	return lease, lease.Validate()
}

func sameEntryLeaseIdentityV2(lease ToolOwnerSingleCallEntryLeaseV2, request ToolOwnerSingleCallEntryLeaseAcquireV2) bool {
	return lease.RequestKey == request.RequestKey && lease.RequestDigest == request.RequestDigest &&
		lease.ExecutionInputDigest == request.ExecutionInputDigest && lease.ExecutionAttemptID == request.ExecutionAttemptID
}

func validEntryLeasePhaseV2(phase ToolOwnerSingleCallEntryLeasePhaseV2) bool {
	return isActiveEntryLeasePhaseV2(phase) || isEntryLeaseHandoffPhaseV2(phase)
}

func isActiveEntryLeasePhaseV2(phase ToolOwnerSingleCallEntryLeasePhaseV2) bool {
	return phase == ToolOwnerEntryStartOrInspectV2 || phase == ToolOwnerEntryInspectV2
}

func isEntryLeaseHandoffPhaseV2(phase ToolOwnerSingleCallEntryLeasePhaseV2) bool {
	return phase == ToolOwnerEntryHandoffStartOrInspectV2 || phase == ToolOwnerEntryHandoffInspectV2
}

func activeEntryLeasePhaseV2(phase ToolOwnerSingleCallEntryLeasePhaseV2) ToolOwnerSingleCallEntryLeasePhaseV2 {
	switch phase {
	case ToolOwnerEntryHandoffStartOrInspectV2:
		return ToolOwnerEntryStartOrInspectV2
	case ToolOwnerEntryHandoffInspectV2:
		return ToolOwnerEntryInspectV2
	default:
		return phase
	}
}

func handoffEntryLeasePhaseV2(phase ToolOwnerSingleCallEntryLeasePhaseV2) ToolOwnerSingleCallEntryLeasePhaseV2 {
	switch phase {
	case ToolOwnerEntryStartOrInspectV2:
		return ToolOwnerEntryHandoffStartOrInspectV2
	case ToolOwnerEntryInspectV2:
		return ToolOwnerEntryHandoffInspectV2
	default:
		return ""
	}
}

func sealEntryLeaseHandoffV2(current ToolOwnerSingleCallEntryLeaseV2, nextPhase ToolOwnerSingleCallEntryLeasePhaseV2, nowUnixNano int64) (ToolOwnerSingleCallEntryLeaseV2, error) {
	if err := current.Validate(); err != nil || !isActiveEntryLeasePhaseV2(current.Phase) || !isActiveEntryLeasePhaseV2(nextPhase) ||
		nowUnixNano < current.AcquiredUnixNano || nowUnixNano >= current.ExpiresUnixNano {
		return ToolOwnerSingleCallEntryLeaseV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner entry lease cannot be handed off")
	}
	next := current
	next.Revision++
	next.Phase = handoffEntryLeasePhaseV2(nextPhase)
	next.AcquiredUnixNano = nowUnixNano
	var err error
	next.Digest, err = next.DigestV2()
	if err != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, err
	}
	return next, next.Validate()
}

func entryLeaseHistoryKeyV2(lease ToolOwnerSingleCallEntryLeaseV2) string {
	return lease.ID + "\x00" + strconv.FormatUint(uint64(lease.Revision), 10) + "\x00" + string(lease.Digest)
}

func cloneEntryLeaseV2(lease ToolOwnerSingleCallEntryLeaseV2) ToolOwnerSingleCallEntryLeaseV2 {
	body, _ := json.Marshal(lease)
	var clone ToolOwnerSingleCallEntryLeaseV2
	_ = core.DecodeStrictJSON(body, &clone)
	return clone
}

var _ ToolOwnerSingleCallExecutionStateStoreV2 = (*InMemoryToolOwnerSingleCallExecutionStateStoreV2)(nil)
var _ ToolOwnerSingleCallEntryLeaseStoreV2 = (*InMemoryToolOwnerSingleCallExecutionStateStoreV2)(nil)

type SQLiteToolOwnerSingleCallExecutionStateStoreV2 struct {
	raw *toolsqlite.OwnerClaimExecutionStoreV2
}

func NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(raw *toolsqlite.OwnerClaimExecutionStoreV2) (*SQLiteToolOwnerSingleCallExecutionStateStoreV2, error) {
	if raw == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool Owner SQLite execution row store is required")
	}
	return &SQLiteToolOwnerSingleCallExecutionStateStoreV2{raw: raw}, nil
}

func (s *SQLiteToolOwnerSingleCallExecutionStateStoreV2) CreateExecutionStartV2(ctx context.Context, state ToolOwnerSingleCallExecutionStateV2) (ToolOwnerSingleCallExecutionStateV2, bool, error) {
	if s == nil || s.raw == nil || state.Validate() != nil || state.Revision != 1 || state.State != ToolOwnerExecutionStartCommittedV2 {
		return ToolOwnerSingleCallExecutionStateV2{}, false, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "Tool Owner SQLite execution start is invalid")
	}
	row, err := encodeSQLiteExecutionRowV2(state)
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, false, err
	}
	winner, created, err := s.raw.CreateExecutionRowV2(ctx, row)
	if err != nil {
		if core.HasCategory(err, core.ErrorIndeterminate) {
			recoveryCtx, cancel := boundedOwnerLocalRecoveryContextV2(ctx, defaultToolOwnerRecoveryTimeoutV2)
			_, _ = s.raw.InspectExecutionHistoryExactRowV2(recoveryCtx, row)
			cancel()
		}
		return ToolOwnerSingleCallExecutionStateV2{}, false, err
	}
	decoded, err := decodeSQLiteExecutionRowV2(winner)
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, false, err
	}
	if !sameExecutionIdentityV2(decoded, state) {
		return ToolOwnerSingleCallExecutionStateV2{}, false, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner SQLite execution winner binds different identity")
	}
	return decoded, created, nil
}

func (s *SQLiteToolOwnerSingleCallExecutionStateStoreV2) InspectExecutionStateV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	if s == nil || s.raw == nil || key.Validate() != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Owner SQLite execution Inspect is invalid")
	}
	row, err := s.raw.InspectExecutionRowV2(ctx, string(key.Digest))
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	state, err := decodeSQLiteExecutionRowV2(row)
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	if state.RequestKey != key {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Owner SQLite execution request key drifted")
	}
	return state, nil
}

func (s *SQLiteToolOwnerSingleCallExecutionStateStoreV2) AdvanceExecutionInspectOnlyV2(ctx context.Context, expected toolcontract.ObjectRef, unknown ToolOwnerSingleCallExecutionUnknownV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	if expected.Validate() != nil || unknown.Validate() != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "Tool Owner SQLite inspect-only transition is invalid")
	}
	return s.advanceSQLiteExecutionV2(ctx, expected, func(current ToolOwnerSingleCallExecutionStateV2) (ToolOwnerSingleCallExecutionStateV2, error) {
		if current.State == ToolOwnerExecutionInspectOnlyV2 && reflect.DeepEqual(current.Unknown, &unknown) {
			return current, nil
		}
		if current.State != ToolOwnerExecutionStartCommittedV2 || unknown.MarkedUnixNano < current.UpdatedUnixNano || unknown.MarkedUnixNano >= current.ExpiresUnixNano {
			return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner SQLite inspect-only source drifted")
		}
		next := current
		next.Revision++
		next.State, next.Result, next.Unknown, next.UpdatedUnixNano = ToolOwnerExecutionInspectOnlyV2, nil, &unknown, unknown.MarkedUnixNano
		next.Digest = ""
		digest, err := next.DigestV2()
		next.Digest = digest
		return next, err
	})
}

func (s *SQLiteToolOwnerSingleCallExecutionStateStoreV2) AdvanceExecutionSettledV2(ctx context.Context, expected toolcontract.ObjectRef, result toolcontract.ObjectRef, nowUnixNano int64) (ToolOwnerSingleCallExecutionStateV2, error) {
	if expected.Validate() != nil || result.Validate() != nil || nowUnixNano <= 0 {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "Tool Owner SQLite settled transition is invalid")
	}
	return s.advanceSQLiteExecutionV2(ctx, expected, func(current ToolOwnerSingleCallExecutionStateV2) (ToolOwnerSingleCallExecutionStateV2, error) {
		if current.State == ToolOwnerExecutionSettledV2 {
			if current.Result != nil && *current.Result == result {
				return current, nil
			}
			return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner SQLite settled replay binds different result")
		}
		if (current.State != ToolOwnerExecutionStartCommittedV2 && current.State != ToolOwnerExecutionInspectOnlyV2) || nowUnixNano < current.UpdatedUnixNano || nowUnixNano >= current.ExpiresUnixNano {
			return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner SQLite settled source drifted")
		}
		next := current
		next.Revision++
		next.State, next.Result, next.Unknown, next.UpdatedUnixNano = ToolOwnerExecutionSettledV2, &result, nil, nowUnixNano
		next.Digest = ""
		digest, err := next.DigestV2()
		next.Digest = digest
		return next, err
	})
}

func (s *SQLiteToolOwnerSingleCallExecutionStateStoreV2) advanceSQLiteExecutionV2(ctx context.Context, expected toolcontract.ObjectRef, build func(ToolOwnerSingleCallExecutionStateV2) (ToolOwnerSingleCallExecutionStateV2, error)) (ToolOwnerSingleCallExecutionStateV2, error) {
	row, err := s.raw.InspectExecutionRowByStateIDV2(ctx, expected.ID)
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	current, err := decodeSQLiteExecutionRowV2(row)
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	if current.RefV2() != expected {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner SQLite execution CAS source drifted")
	}
	next, err := build(current)
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	if next.RefV2() == current.RefV2() {
		return current, nil
	}
	if err = next.Validate(); err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	nextRow, err := encodeSQLiteExecutionRowV2(next)
	if err != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	if err = s.raw.AdvanceExecutionRowV2(ctx, expected.ID, int64(expected.Revision), string(expected.Digest), nextRow); err != nil {
		if core.HasCategory(err, core.ErrorIndeterminate) {
			recoveryCtx, cancel := boundedOwnerLocalRecoveryContextV2(ctx, defaultToolOwnerRecoveryTimeoutV2)
			_, _ = s.raw.InspectExecutionHistoryExactRowV2(recoveryCtx, nextRow)
			cancel()
		}
		return ToolOwnerSingleCallExecutionStateV2{}, err
	}
	return next, nil
}

func encodeSQLiteExecutionRowV2(state ToolOwnerSingleCallExecutionStateV2) (toolsqlite.OwnerExecutionRowV2, error) {
	if err := state.Validate(); err != nil {
		return toolsqlite.OwnerExecutionRowV2{}, err
	}
	body, err := json.Marshal(state)
	if err != nil {
		return toolsqlite.OwnerExecutionRowV2{}, err
	}
	rowDigest, err := core.CanonicalJSONDigest("praxis.tool-mcp.sqlite-row", "v1", "ToolOwnerSingleCallExecutionStateV2", state)
	if err != nil {
		return toolsqlite.OwnerExecutionRowV2{}, err
	}
	return toolsqlite.OwnerExecutionRowV2{RequestKeyDigest: string(state.RequestKey.Digest), RequestID: state.RequestKey.RequestID, RequestDigest: string(state.RequestDigest), ActionCoordinateDigest: string(state.ActionDigest), ExecutionScopeDigest: string(state.ExecutionScopeDigest), BindingID: state.BindingRef.ID, BindingRevision: int64(state.BindingRef.Revision), BindingDigest: string(state.BindingRef.Digest), InputDigest: string(state.ExecutionInputDigest), StateID: state.ID, StateRevision: int64(state.Revision), StateDigest: string(state.Digest), StateJSON: body, RowDigest: string(rowDigest)}, nil
}

func decodeSQLiteExecutionRowV2(row toolsqlite.OwnerExecutionRowV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	var state ToolOwnerSingleCallExecutionStateV2
	if core.DecodeStrictJSON(row.StateJSON, &state) != nil || state.Validate() != nil {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "Tool Owner SQLite execution JSON is invalid")
	}
	expected, err := encodeSQLiteExecutionRowV2(state)
	if err != nil || !reflectSQLiteExecutionRowV2(row, expected) {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Tool Owner SQLite execution exact columns or row digest drifted")
	}
	return cloneExecutionStateV2(state), nil
}

func reflectSQLiteExecutionRowV2(left, right toolsqlite.OwnerExecutionRowV2) bool {
	return left.RequestKeyDigest == right.RequestKeyDigest && left.RequestID == right.RequestID && left.RequestDigest == right.RequestDigest && left.ActionCoordinateDigest == right.ActionCoordinateDigest && left.ExecutionScopeDigest == right.ExecutionScopeDigest && left.BindingID == right.BindingID && left.BindingRevision == right.BindingRevision && left.BindingDigest == right.BindingDigest && left.InputDigest == right.InputDigest && left.StateID == right.StateID && left.StateRevision == right.StateRevision && left.StateDigest == right.StateDigest && string(left.StateJSON) == string(right.StateJSON) && left.RowDigest == right.RowDigest
}

func (s *SQLiteToolOwnerSingleCallExecutionStateStoreV2) TryAcquireExecutionEntryLeaseV2(ctx context.Context, request ToolOwnerSingleCallEntryLeaseAcquireV2) (ToolOwnerSingleCallEntryLeaseV2, bool, error) {
	if s == nil || s.raw == nil || request.Validate() != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, false, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Owner SQLite entry lease acquisition is invalid")
	}
	for attempt := 0; attempt < 8; attempt++ {
		var current ToolOwnerSingleCallEntryLeaseV2
		var currentRow *toolsqlite.OwnerEntryLeaseRowV2
		row, inspectErr := s.raw.InspectEntryLeaseRowV2(ctx, request.ExecutionAttemptID)
		switch {
		case inspectErr == nil:
			decoded, decodeErr := decodeSQLiteEntryLeaseRowV2(row)
			if decodeErr != nil {
				return ToolOwnerSingleCallEntryLeaseV2{}, false, decodeErr
			}
			current = decoded
			currentRow = &row
			if !sameEntryLeaseIdentityV2(current, request) {
				return ToolOwnerSingleCallEntryLeaseV2{}, false, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Owner SQLite entry lease binds different execution identity")
			}
			if request.AcquiredUnixNano < current.AcquiredUnixNano {
				return ToolOwnerSingleCallEntryLeaseV2{}, false, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "Tool Owner SQLite entry lease clock regressed")
			}
			if !isEntryLeaseHandoffPhaseV2(current.Phase) && request.AcquiredUnixNano < current.ExpiresUnixNano {
				return current, false, nil
			}
			if isEntryLeaseHandoffPhaseV2(current.Phase) && request.Phase != activeEntryLeasePhaseV2(current.Phase) {
				return ToolOwnerSingleCallEntryLeaseV2{}, false, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner SQLite entry lease handoff only permits its exact next phase")
			}
		case core.HasCategory(inspectErr, core.ErrorNotFound):
		default:
			return ToolOwnerSingleCallEntryLeaseV2{}, false, inspectErr
		}
		revision := core.Revision(1)
		if currentRow != nil {
			revision = current.Revision + 1
		}
		next, sealErr := sealEntryLeaseV2(request, revision)
		if sealErr != nil {
			return ToolOwnerSingleCallEntryLeaseV2{}, false, sealErr
		}
		nextRow, encodeErr := encodeSQLiteEntryLeaseRowV2(next)
		if encodeErr != nil {
			return ToolOwnerSingleCallEntryLeaseV2{}, false, encodeErr
		}
		casErr := s.raw.CompareAndSwapEntryLeaseRowV2(ctx, currentRow, nextRow)
		if casErr == nil {
			return next, true, nil
		}
		if core.HasCategory(casErr, core.ErrorIndeterminate) {
			winner, winnerErr := s.raw.InspectEntryLeaseHistoryExactRowV2(ctx, nextRow)
			if winnerErr != nil {
				return ToolOwnerSingleCallEntryLeaseV2{}, false, casErr
			}
			decoded, decodeErr := decodeSQLiteEntryLeaseRowV2(winner)
			if decodeErr != nil {
				return ToolOwnerSingleCallEntryLeaseV2{}, false, decodeErr
			}
			if decoded.RefV2() == next.RefV2() {
				return decoded, true, nil
			}
			return ToolOwnerSingleCallEntryLeaseV2{}, false, casErr
		}
		if !core.HasCategory(casErr, core.ErrorConflict) {
			return ToolOwnerSingleCallEntryLeaseV2{}, false, casErr
		}
	}
	return ToolOwnerSingleCallEntryLeaseV2{}, false, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Tool Owner SQLite entry lease contention did not converge")
}

func (s *SQLiteToolOwnerSingleCallExecutionStateStoreV2) InspectExecutionEntryLeaseV2(ctx context.Context, executionAttemptID string) (ToolOwnerSingleCallEntryLeaseV2, error) {
	if s == nil || s.raw == nil || toolcontract.ValidateStableID(executionAttemptID) != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Owner SQLite entry lease Inspect is invalid")
	}
	row, err := s.raw.InspectEntryLeaseRowV2(ctx, executionAttemptID)
	if err != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, err
	}
	return decodeSQLiteEntryLeaseRowV2(row)
}

func (s *SQLiteToolOwnerSingleCallExecutionStateStoreV2) AdvanceExecutionEntryLeaseHandoffV2(ctx context.Context, expected ToolOwnerSingleCallEntryLeaseV2, nextPhase ToolOwnerSingleCallEntryLeasePhaseV2, nowUnixNano int64) (ToolOwnerSingleCallEntryLeaseV2, error) {
	if s == nil || s.raw == nil || expected.Validate() != nil || !isActiveEntryLeasePhaseV2(nextPhase) || nowUnixNano <= 0 {
		return ToolOwnerSingleCallEntryLeaseV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "Tool Owner SQLite entry lease handoff is invalid")
	}
	next, err := sealEntryLeaseHandoffV2(expected, nextPhase, nowUnixNano)
	if err != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, err
	}
	row, err := s.raw.InspectEntryLeaseRowV2(ctx, expected.ExecutionAttemptID)
	if err != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, err
	}
	current, err := decodeSQLiteEntryLeaseRowV2(row)
	if err != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, err
	}
	if current.RefV2() != expected.RefV2() {
		if current.RefV2() == next.RefV2() && reflect.DeepEqual(current, next) {
			return current, nil
		}
		return ToolOwnerSingleCallEntryLeaseV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Tool Owner SQLite entry lease handoff CAS source drifted")
	}
	nextRow, err := encodeSQLiteEntryLeaseRowV2(next)
	if err != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, err
	}
	if err = s.raw.CompareAndSwapEntryLeaseRowV2(ctx, &row, nextRow); err == nil {
		return next, nil
	}
	recoveryCtx, cancel := boundedOwnerLocalRecoveryContextV2(ctx, defaultToolOwnerRecoveryTimeoutV2)
	defer cancel()
	winnerRow, inspectErr := s.raw.InspectEntryLeaseHistoryExactRowV2(recoveryCtx, nextRow)
	if inspectErr != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, err
	}
	winner, inspectErr := decodeSQLiteEntryLeaseRowV2(winnerRow)
	if inspectErr != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, inspectErr
	}
	if winner.RefV2() == next.RefV2() && reflect.DeepEqual(winner, next) {
		return winner, nil
	}
	return ToolOwnerSingleCallEntryLeaseV2{}, err
}

func encodeSQLiteEntryLeaseRowV2(lease ToolOwnerSingleCallEntryLeaseV2) (toolsqlite.OwnerEntryLeaseRowV2, error) {
	if err := lease.Validate(); err != nil {
		return toolsqlite.OwnerEntryLeaseRowV2{}, err
	}
	body, err := json.Marshal(lease)
	if err != nil {
		return toolsqlite.OwnerEntryLeaseRowV2{}, err
	}
	rowDigest, err := core.CanonicalJSONDigest("praxis.tool-mcp.sqlite-row", "v1", "ToolOwnerSingleCallEntryLeaseV2", lease)
	if err != nil {
		return toolsqlite.OwnerEntryLeaseRowV2{}, err
	}
	return toolsqlite.OwnerEntryLeaseRowV2{
		LeaseID: lease.ID, LeaseRevision: int64(lease.Revision), LeaseDigest: string(lease.Digest),
		ExecutionAttemptID: lease.ExecutionAttemptID, RequestKeyDigest: string(lease.RequestKey.Digest),
		RequestDigest: string(lease.RequestDigest), InputDigest: string(lease.ExecutionInputDigest),
		HolderIncarnationID: lease.HolderIncarnationID, Phase: string(lease.Phase),
		AcquiredUnixNano: lease.AcquiredUnixNano, ExpiresUnixNano: lease.ExpiresUnixNano,
		LeaseJSON: body, RowDigest: string(rowDigest),
	}, nil
}

func decodeSQLiteEntryLeaseRowV2(row toolsqlite.OwnerEntryLeaseRowV2) (ToolOwnerSingleCallEntryLeaseV2, error) {
	var lease ToolOwnerSingleCallEntryLeaseV2
	if core.DecodeStrictJSON(row.LeaseJSON, &lease) != nil || lease.Validate() != nil {
		return ToolOwnerSingleCallEntryLeaseV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "Tool Owner SQLite entry lease JSON is invalid")
	}
	expected, err := encodeSQLiteEntryLeaseRowV2(lease)
	if err != nil || !reflect.DeepEqual(row, expected) {
		return ToolOwnerSingleCallEntryLeaseV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Tool Owner SQLite entry lease exact columns or row digest drifted")
	}
	return cloneEntryLeaseV2(lease), nil
}

var _ ToolOwnerSingleCallExecutionStateStoreV2 = (*SQLiteToolOwnerSingleCallExecutionStateStoreV2)(nil)
var _ ToolOwnerSingleCallEntryLeaseStoreV2 = (*SQLiteToolOwnerSingleCallExecutionStateStoreV2)(nil)

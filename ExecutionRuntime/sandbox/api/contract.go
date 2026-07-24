package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

const ContractVersionV1 = "praxis.sandbox.api/v1"

type ActionV1 string

const (
	ActionDescribeBackendsV1 ActionV1 = "praxis.sandbox.api/describe-backends"
	ActionMatchRequirementV1 ActionV1 = "praxis.sandbox.api/match-requirement"
	ActionInspectV1          ActionV1 = "praxis.sandbox.api/inspect"
	ActionWorkspaceDiffV1    ActionV1 = "praxis.sandbox.api/workspace-diff"
	ActionLifecycleV1        ActionV1 = "praxis.sandbox.api/lifecycle"
	ActionFenceV1            ActionV1 = "praxis.sandbox.api/fence"
	ActionWorkspaceCommitV1  ActionV1 = "praxis.sandbox.api/workspace-commit"
	ActionCheckpointV1       ActionV1 = "praxis.sandbox.api/checkpoint"
	ActionWorkspaceRestoreV1 ActionV1 = "praxis.sandbox.api/workspace-restore"
	ActionWorkspaceRewindV1  ActionV1 = "praxis.sandbox.api/workspace-rewind-compose"
	ActionReleaseV1          ActionV1 = "praxis.sandbox.api/release"
	ActionCleanupV1          ActionV1 = "praxis.sandbox.api/cleanup"
)

func (a ActionV1) Validate() error {
	switch a {
	case ActionDescribeBackendsV1, ActionMatchRequirementV1, ActionInspectV1, ActionWorkspaceDiffV1,
		ActionLifecycleV1, ActionFenceV1, ActionWorkspaceCommitV1, ActionCheckpointV1,
		ActionWorkspaceRestoreV1, ActionWorkspaceRewindV1, ActionReleaseV1, ActionCleanupV1:
		return nil
	default:
		return fmt.Errorf("unsupported sandbox API action %q", a)
	}
}

func (a ActionV1) Effectful() bool {
	switch a {
	case ActionLifecycleV1, ActionFenceV1, ActionWorkspaceCommitV1, ActionCheckpointV1,
		ActionWorkspaceRestoreV1, ActionReleaseV1, ActionCleanupV1:
		return true
	default:
		return false
	}
}

type OperationRequestV1 struct {
	ContractVersion           string          `json:"contract_version"`
	RequestID                 string          `json:"request_id"`
	IdempotencyKey            string          `json:"idempotency_key"`
	TenantID                  string          `json:"tenant_id"`
	Action                    ActionV1        `json:"action"`
	PayloadSchema             string          `json:"payload_schema"`
	PayloadRevision           uint64          `json:"payload_revision"`
	Payload                   json.RawMessage `json:"payload"`
	PayloadDigest             string          `json:"payload_digest"`
	ExpectedOperationRevision uint64          `json:"expected_operation_revision"`
	RequestedUnixNano         int64           `json:"requested_unix_nano"`
	RequestedNotAfterUnixNano int64           `json:"requested_not_after_unix_nano"`
	Digest                    string          `json:"digest"`
}

func SealOperationRequestV1(value OperationRequestV1) (OperationRequestV1, error) {
	value.ContractVersion = ContractVersionV1
	payload, err := canonicalJSON(value.Payload)
	if err != nil {
		return OperationRequestV1{}, err
	}
	value.Payload = payload
	value.PayloadDigest, err = contract.Digest("sandbox-api-operation-payload-v1", []byte(payload))
	if err != nil {
		return OperationRequestV1{}, err
	}
	value.Digest = ""
	value.Digest, err = contract.Digest("sandbox-api-operation-request-v1", value)
	if err != nil {
		return OperationRequestV1{}, err
	}
	return value, value.ValidateShape()
}

func (r OperationRequestV1) ValidateShape() error {
	if r.ContractVersion != ContractVersionV1 || strings.TrimSpace(r.RequestID) == "" || strings.TrimSpace(r.IdempotencyKey) == "" || strings.TrimSpace(r.TenantID) == "" || strings.TrimSpace(r.PayloadSchema) == "" || r.PayloadRevision == 0 || r.ExpectedOperationRevision != 0 || r.RequestedUnixNano <= 0 || r.RequestedNotAfterUnixNano <= r.RequestedUnixNano {
		return errors.New("sandbox API operation request coordinates are incomplete")
	}
	if err := r.Action.Validate(); err != nil {
		return err
	}
	payload, err := canonicalJSON(r.Payload)
	if err != nil || !bytes.Equal(payload, r.Payload) {
		return errors.New("sandbox API payload is not canonical JSON")
	}
	payloadDigest, err := contract.Digest("sandbox-api-operation-payload-v1", []byte(payload))
	if err != nil || payloadDigest != r.PayloadDigest {
		return errors.New("sandbox API payload digest drifted")
	}
	copy := r
	copy.Digest = ""
	digest, err := contract.Digest("sandbox-api-operation-request-v1", copy)
	if err != nil || digest != r.Digest {
		return errors.New("sandbox API request digest drifted")
	}
	return nil
}

func (r OperationRequestV1) ValidateCurrent(now time.Time) error {
	if err := r.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.RequestedUnixNano || now.UnixNano() >= r.RequestedNotAfterUnixNano {
		return errors.New("sandbox API request is not current")
	}
	return nil
}

type OperationStateV1 string

const (
	OperationQueuedV1        OperationStateV1 = "queued"
	OperationRunningV1       OperationStateV1 = "running"
	OperationSucceededV1     OperationStateV1 = "succeeded"
	OperationFailedV1        OperationStateV1 = "failed"
	OperationCancelledV1     OperationStateV1 = "cancelled"
	OperationIndeterminateV1 OperationStateV1 = "indeterminate"
)

func (s OperationStateV1) Terminal() bool {
	return s == OperationSucceededV1 || s == OperationFailedV1 || s == OperationCancelledV1 || s == OperationIndeterminateV1
}

type ResultV1 struct {
	Schema   string          `json:"schema"`
	Revision uint64          `json:"revision"`
	Payload  json.RawMessage `json:"payload"`
	Digest   string          `json:"digest"`
}

func SealResultV1(value ResultV1) (ResultV1, error) {
	if strings.TrimSpace(value.Schema) == "" || value.Revision == 0 {
		return ResultV1{}, errors.New("sandbox API result schema is incomplete")
	}
	payload, err := canonicalJSON(value.Payload)
	if err != nil {
		return ResultV1{}, err
	}
	value.Payload = payload
	value.Digest, err = contract.Digest("sandbox-api-result-v1", struct {
		Schema   string
		Revision uint64
		Payload  []byte
	}{value.Schema, value.Revision, payload})
	if err != nil {
		return ResultV1{}, err
	}
	return value, value.Validate()
}

func (r ResultV1) Validate() error {
	if strings.TrimSpace(r.Schema) == "" || r.Revision == 0 {
		return errors.New("sandbox API result schema is incomplete")
	}
	payload, err := canonicalJSON(r.Payload)
	if err != nil || !bytes.Equal(payload, r.Payload) {
		return errors.New("sandbox API result payload is not canonical JSON")
	}
	digest, err := contract.Digest("sandbox-api-result-v1", struct {
		Schema   string
		Revision uint64
		Payload  []byte
	}{r.Schema, r.Revision, payload})
	if err != nil || digest != r.Digest {
		return errors.New("sandbox API result is invalid or drifted")
	}
	return nil
}

type ClosedErrorV1 struct {
	Category string `json:"category"`
	Reason   string `json:"reason"`
	Message  string `json:"message"`
}

func (e ClosedErrorV1) Validate() error {
	if strings.TrimSpace(e.Category) == "" || strings.TrimSpace(e.Reason) == "" || strings.TrimSpace(e.Message) == "" {
		return errors.New("sandbox API closed error is incomplete")
	}
	return nil
}

type OperationFactV1 struct {
	ContractVersion       string             `json:"contract_version"`
	ID                    string             `json:"id"`
	Revision              uint64             `json:"revision"`
	Request               OperationRequestV1 `json:"request"`
	State                 OperationStateV1   `json:"state"`
	CancellationRequested bool               `json:"cancellation_requested"`
	Result                *ResultV1          `json:"result,omitempty"`
	Error                 *ClosedErrorV1     `json:"error,omitempty"`
	CreatedUnixNano       int64              `json:"created_unix_nano"`
	UpdatedUnixNano       int64              `json:"updated_unix_nano"`
	ExpiresUnixNano       int64              `json:"expires_unix_nano"`
	Digest                string             `json:"digest"`
}

func SealOperationFactV1(value OperationFactV1) (OperationFactV1, error) {
	value.ContractVersion = ContractVersionV1
	value.Digest = ""
	digest, err := contract.Digest("sandbox-api-operation-fact-v1", value)
	if err != nil {
		return OperationFactV1{}, err
	}
	value.Digest = digest
	return value, value.ValidateShape()
}

func (f OperationFactV1) ValidateShape() error {
	if f.ContractVersion != ContractVersionV1 || strings.TrimSpace(f.ID) == "" || f.ID != f.Request.RequestID || f.Revision == 0 || f.Request.ValidateShape() != nil || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano || f.ExpiresUnixNano <= f.CreatedUnixNano || f.ExpiresUnixNano > f.Request.RequestedNotAfterUnixNano {
		return errors.New("sandbox API operation fact coordinates are incomplete")
	}
	switch f.State {
	case OperationQueuedV1, OperationRunningV1:
		if f.Result != nil || f.Error != nil || f.ExpiresUnixNano <= f.UpdatedUnixNano {
			return errors.New("nonterminal sandbox API operation carries terminal output")
		}
	case OperationSucceededV1:
		if f.Result == nil || f.Result.Validate() != nil || f.Error != nil {
			return errors.New("successful sandbox API operation output is invalid")
		}
	case OperationFailedV1, OperationCancelledV1, OperationIndeterminateV1:
		if f.Result != nil || f.Error == nil || f.Error.Validate() != nil {
			return errors.New("terminal sandbox API error output is invalid")
		}
	default:
		return errors.New("sandbox API operation state is invalid")
	}
	copy := f
	copy.Digest = ""
	digest, err := contract.Digest("sandbox-api-operation-fact-v1", copy)
	if err != nil || digest != f.Digest {
		return errors.New("sandbox API operation fact digest drifted")
	}
	return nil
}

func canonicalJSON(raw json.RawMessage) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("sandbox API JSON payload is required")
	}
	value, err := contract.DecodeStrict[any](raw)
	if err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}

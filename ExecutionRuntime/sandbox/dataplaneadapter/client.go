package dataplaneadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

type Client struct {
	SocketPath string
	AllowedUID uint32
}

type DataPlaneOperationV1 string

const (
	DataPlaneDispatchV1 DataPlaneOperationV1 = "dispatch"
	DataPlaneInspectV1  DataPlaneOperationV1 = "inspect"
)

type DataPlaneRequestV1 struct {
	ContractVersion string               `json:"contract_version"`
	Operation       DataPlaneOperationV1 `json:"operation"`
	Request         DispatchRequestV1    `json:"request"`
}

func (c Client) Dispatch(ctx context.Context, request DispatchRequestV1) (DispatchResponseV1, error) {
	return c.call(ctx, DataPlaneDispatchV1, request)
}

// Inspect recovers only the exact original request result from the Data Plane
// durable journal. The Data Plane does not call a Provider for this method.
func (c Client) Inspect(ctx context.Context, request DispatchRequestV1) (DispatchResponseV1, error) {
	return c.call(ctx, DataPlaneInspectV1, request)
}

func (c Client) call(ctx context.Context, operation DataPlaneOperationV1, request DispatchRequestV1) (DispatchResponseV1, error) {
	if err := request.ValidateCurrent(time.Now()); err != nil {
		return DispatchResponseV1{}, err
	}
	if operation != DataPlaneDispatchV1 && operation != DataPlaneInspectV1 {
		return DispatchResponseV1{}, errors.New("data plane operation is invalid")
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return DispatchResponseV1{}, err
	}
	defer connection.Close()
	unix, ok := connection.(*net.UnixConn)
	if !ok {
		return DispatchResponseV1{}, errors.New("data plane connection is not Unix domain IPC")
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := unix.SetDeadline(deadline); err != nil {
			return DispatchResponseV1{}, err
		}
	}
	if err := validatePeerUID(unix, c.AllowedUID); err != nil {
		return DispatchResponseV1{}, err
	}
	envelope := DataPlaneRequestV1{ContractVersion: ContractVersionV1, Operation: operation, Request: request}
	if err := writeFrame(unix, envelope); err != nil {
		return DispatchResponseV1{}, err
	}
	var response DispatchResponseV1
	if err := readFrame(unix, &response); err != nil {
		return DispatchResponseV1{}, err
	}
	if err := response.Validate(request); err != nil {
		return DispatchResponseV1{}, err
	}
	if !response.Accepted {
		return response, response.Error
	}
	return response, nil
}

func (r DispatchResponseV1) Validate(request DispatchRequestV1) error {
	if r.ContractVersion != ContractVersionV1 || r.RequestID != request.RequestID || r.RequestDigest != request.Digest || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano < r.CheckedUnixNano {
		return errors.New("data plane response coordinates are invalid")
	}
	if r.Accepted {
		if r.ProviderAttempt == nil || r.ProviderObservation == nil || r.ProviderReceipt == nil || r.ProviderAttempt.ID == "" || r.ProviderAttempt.Revision == 0 || !validDigest(r.ProviderAttempt.Digest) || r.ProviderAttempt.ExpiresUnixNano != r.ExpiresUnixNano || r.ObservationDigest == nil || r.ReceiptDigest == nil || r.Error != nil || !validDigest(*r.ObservationDigest) || !validDigest(*r.ReceiptDigest) || r.ProviderObservation.Provider == "" || r.ProviderObservation.Provider != r.ProviderReceipt.Provider || r.ProviderObservation.Attempt != *r.ProviderAttempt || r.ProviderReceipt.Attempt != *r.ProviderAttempt || r.ProviderObservation.PayloadDigest != request.PayloadDigest || r.ProviderObservation.ObservedUnixNano <= 0 || r.ProviderObservation.ObservedUnixNano > r.CheckedUnixNano || r.ProviderReceipt.Phase != string(request.Phase) || r.ProviderReceipt.RecordedUnixNano <= 0 || r.ProviderReceipt.RecordedUnixNano > r.CheckedUnixNano || r.ProviderReceipt.ExpiresUnixNano != r.ExpiresUnixNano || r.ProviderObservation.Digest != *r.ObservationDigest || r.ProviderReceipt.Digest != *r.ReceiptDigest || r.ProviderReceipt.ObservationDigest != *r.ObservationDigest {
			return errors.New("accepted data plane response presence is invalid")
		}
		if request.EffectKind == CheckpointEffectKindV1 && request.Phase == PhaseExecute {
			if err := validateCheckpointArtifactObservation(r.ProviderObservation.CheckpointArtifact, request, r.CheckedUnixNano); err != nil {
				return err
			}
		} else if r.ProviderObservation.CheckpointArtifact != nil {
			return errors.New("non-checkpoint Provider observation carries a checkpoint artifact")
		}
		if request.Payload.ProviderKind == "workspace_commit" {
			if err := validateWorkspaceCommitObservation(r.ProviderObservation.WorkspaceCommit, request, r.CheckedUnixNano); err != nil {
				return err
			}
		} else if r.ProviderObservation.WorkspaceCommit != nil {
			return errors.New("non-workspace Provider observation carries a workspace commit result")
		}
		if request.Payload.ProviderKind == "workspace_read" && request.Phase == PhaseExecute {
			if err := validateWorkspaceReadObservation(r.ProviderObservation.WorkspaceRead, request, r.CheckedUnixNano); err != nil {
				return err
			}
		} else if r.ProviderObservation.WorkspaceRead != nil {
			return errors.New("non-workspace-read observation carries a workspace read result")
		}
		providerName, providerKind, err := expectedProviderIdentity(request.Payload.ProviderKind)
		if err != nil || r.ProviderObservation.Provider != providerKind || r.ProviderAttempt.ID != providerName+"/"+request.TenantID+"/"+request.AttemptID {
			return errors.New("provider response identity drifted from the sealed request")
		}
		expectedExpiry := min(request.RequestedNotAfterUnixNano, request.SandboxAttempt.ExpiresUnixNano, request.ExecutionBinding.ExpiresUnixNano, request.RuntimeEnforcement.ExpiresUnixNano)
		if r.ExpiresUnixNano != expectedExpiry {
			return errors.New("provider response TTL drifted from the sealed request")
		}
		attempt := *r.ProviderAttempt
		attempt.Digest = ""
		attemptDigest, err := canonicalDigest("ProviderAttemptRefV1", attempt)
		if err != nil || attemptDigest != r.ProviderAttempt.Digest {
			return errors.New("provider attempt digest drifted")
		}
		observation := *r.ProviderObservation
		observation.Digest = ""
		observationDigest, err := canonicalDigest("ProviderObservationV1", observation)
		if err != nil || observationDigest != *r.ObservationDigest {
			return errors.New("provider observation digest drifted")
		}
		receipt := *r.ProviderReceipt
		receipt.Digest = ""
		receiptDigest, err := canonicalDigest("ProviderReceiptV1", receipt)
		if err != nil || receiptDigest != *r.ReceiptDigest {
			return errors.New("provider receipt digest drifted")
		}
	} else if r.ProviderAttempt != nil || r.ProviderObservation != nil || r.ProviderReceipt != nil || r.ObservationDigest != nil || r.ReceiptDigest != nil || r.Error == nil {
		return errors.New("rejected data plane response presence is invalid")
	}
	copy := r
	copy.Digest = ""
	digest, err := canonicalDigest("DispatchResponseV1", copy)
	if err != nil || digest != r.Digest {
		return errors.New("data plane response digest drifted")
	}
	return nil
}

func validateCheckpointArtifactObservation(value *CheckpointArtifactObservationV1, request DispatchRequestV1, now int64) error {
	if value == nil || value.ContractVersion != "praxis.sandbox/checkpoint-artifact-observation/v1" || value.ArtifactID == "" || !validDigest(value.SubjectDigest) || !validDigest(value.ContentDigest) || value.ContentLength == 0 || value.RecordedUnixNano <= 0 || value.RecordedUnixNano > now || value.ExpiresUnixNano <= now || value.ExpiresUnixNano > request.RequestedNotAfterUnixNano {
		return errors.New("checkpoint Provider observation lacks a current opaque artifact")
	}
	expected := map[string]string{"checkpoint_prepare": "prepared", "checkpoint_commit": "committed", "checkpoint_abort": "aborted"}[value.CheckpointPhase]
	if expected == "" || value.State != expected || !strings.HasSuffix(value.ArtifactID, strings.TrimPrefix(value.SubjectDigest, "sha256:")) {
		return errors.New("checkpoint Provider artifact identity drifted")
	}
	return nil
}

func validateWorkspaceCommitObservation(value *WorkspaceCommitObservationV1, request DispatchRequestV1, now int64) error {
	if value == nil {
		return errors.New("workspace Provider observation lacks exact commit result")
	}
	var payload WorkspaceCommitPayloadV1
	if err := json.Unmarshal(request.Payload.ProviderPayload, &payload); err != nil {
		return errors.New("workspace Provider payload is invalid")
	}
	expires := min(request.RequestedNotAfterUnixNano, request.SandboxAttempt.ExpiresUnixNano, request.ExecutionBinding.ExpiresUnixNano, request.RuntimeEnforcement.ExpiresUnixNano, payload.ChangeSet.ExpiresUnixNano, payload.View.ExpiresUnixNano)
	if value.ContractVersion != "praxis.sandbox/workspace-commit-observation/v1" || value.ChangeSet != payload.ChangeSet || value.View != payload.View || value.BaseRevision != payload.BaseRevision || !validDigest(value.CommittedRevision) || value.RecordedUnixNano <= 0 || value.RecordedUnixNano > now || value.ExpiresUnixNano != expires || now >= value.ExpiresUnixNano {
		return errors.New("workspace Provider observation is incomplete or stale")
	}
	validState := false
	switch {
	case request.EffectKind == "praxis.sandbox/workspace-commit" && request.Phase == PhasePrepare:
		validState = value.State == "prepared"
	case request.EffectKind == "praxis.sandbox/workspace-commit" && request.Phase == PhaseExecute:
		validState = value.State == "committed"
	case request.EffectKind == "praxis.sandbox/inspect" && payload.InspectionTarget != nil && payload.InspectionTarget.OriginalEffectKind == "praxis.sandbox/workspace-commit":
		validState = value.State == "committed" || value.State == "not_applied" || value.State == "indeterminate"
	}
	if !validState {
		return errors.New("workspace Provider observation state drifted from its phase")
	}
	return nil
}

func expectedProviderIdentity(kind string) (string, string, error) {
	switch kind {
	case "host_workspace":
		return "host", kind, nil
	case "qemu_microvm":
		return "microvm", kind, nil
	case "containerd_oci":
		return "containerd", kind, nil
	case "wasmtime_component":
		return "wasmtime", kind, nil
	case "remote_sandbox":
		return "remote", kind, nil
	case "workspace_commit":
		return "workspace-commit", kind, nil
	case "workspace_read":
		return "workspace-read", kind, nil
	default:
		return "", "", errors.New("provider kind is unsupported")
	}
}

func validateWorkspaceReadObservation(value *WorkspaceReadObservationV1, request DispatchRequestV1, now int64) error {
	if value == nil {
		return errors.New("workspace read Provider observation is missing")
	}
	var payload WorkspaceReadPayloadV1
	if err := json.Unmarshal(request.Payload.ProviderPayload, &payload); err != nil {
		return errors.New("workspace read Provider payload is invalid")
	}
	expires := min(request.RequestedNotAfterUnixNano, request.SandboxAttempt.ExpiresUnixNano, request.ExecutionBinding.ExpiresUnixNano, request.RuntimeEnforcement.ExpiresUnixNano, payload.Workspace.ExpiresUnixNano)
	if value.ContractVersion != "praxis.sandbox/workspace-read-observation/v1" || value.Workspace != payload.Workspace || value.RelativePath != payload.RelativePath || value.StartByte != payload.StartByte || value.ReturnedBytes != uint64(len([]byte(value.Content))) || value.ReturnedBytes > payload.MaxBytes || value.TotalBytes > 1<<20 || value.StartByte > value.TotalBytes || value.ReturnedBytes > value.TotalBytes-value.StartByte || value.Complete != (value.StartByte+value.ReturnedBytes == value.TotalBytes) || !value.S1Checked || !value.S2Checked || value.PhysicalReadCount != 1 || value.RecordedUnixNano <= 0 || value.RecordedUnixNano > now || value.ExpiresUnixNano != expires || now >= expires {
		return errors.New("workspace read Provider observation is incomplete or stale")
	}
	if value.File.ID == "" || value.File.Revision != payload.Workspace.Revision || !validDigest(value.File.Digest) || value.File.ExpiresUnixNano != payload.Workspace.ExpiresUnixNano || payload.ExpectedFileRef != nil && value.File != *payload.ExpectedFileRef {
		return errors.New("workspace read exact file ref drifted")
	}
	if !validDigest(value.ContentDigest) || value.ContentDigest != workspaceReadContentDigest([]byte(value.Content), value.StartByte, value.TotalBytes, value.Complete) {
		return errors.New("workspace read bounded content digest drifted")
	}
	return nil
}

func workspaceReadContentDigest(value []byte, start, total uint64, complete bool) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "praxis.sandbox/workspace-read-range/v1%c%d%c%d%c%t%c", 0, start, 0, total, 0, complete, 0)
	_, _ = h.Write(value)
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

package dataplaneadapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ContractVersionV1 = "praxis.sandbox/data-plane-ipc/v1"
	maxFrameBytes     = 4 * 1024 * 1024
)

type EnforcementPhaseV1 string

const (
	PhasePrepare EnforcementPhaseV1 = "prepare"
	PhaseExecute EnforcementPhaseV1 = "execute"
)

type ExactRefV1 struct {
	ID              string `json:"id"`
	Revision        uint64 `json:"revision"`
	Digest          string `json:"digest"`
	ExpiresUnixNano int64  `json:"expires_unix_nano"`
}

type ProviderBindingV1 struct {
	BindingSetID       string `json:"binding_set_id"`
	BindingSetRevision uint64 `json:"binding_set_revision"`
	ComponentID        string `json:"component_id"`
	ManifestDigest     string `json:"manifest_digest"`
	ArtifactDigest     string `json:"artifact_digest"`
	Capability         string `json:"capability"`
	Digest             string `json:"digest"`
}

type SandboxProjectionRefV1 struct {
	Revision        uint64 `json:"revision"`
	Digest          string `json:"digest"`
	ExpiresUnixNano int64  `json:"expires_unix_nano"`
}

type RuntimeEnforcementRefV1 struct {
	OperationDigest string             `json:"operation_digest"`
	EffectID        string             `json:"effect_id"`
	PermitID        string             `json:"permit_id"`
	AttemptID       string             `json:"attempt_id"`
	Phase           EnforcementPhaseV1 `json:"phase"`
	ReceiptDigest   string             `json:"receipt_digest"`
	JournalRevision uint64             `json:"journal_revision"`
	ExpiresUnixNano int64              `json:"expires_unix_nano"`
}

type ExecutionBindingV1 struct {
	TenantID         string `json:"tenant_id"`
	InstanceID       string `json:"instance_id"`
	InstanceEpoch    uint64 `json:"instance_epoch"`
	LeaseID          string `json:"lease_id"`
	LeaseEpoch       uint64 `json:"lease_epoch"`
	FenceEpoch       uint64 `json:"fence_epoch"`
	ScopeDigest      string `json:"scope_digest"`
	ObservedRevision uint64 `json:"observed_revision"`
	ExpiresUnixNano  int64  `json:"expires_unix_nano"`
}

type ProviderPayloadV1 struct {
	ProviderKind    string          `json:"provider_kind"`
	ProviderPayload json.RawMessage `json:"provider_payload"`
}

type ProviderInspectionTargetV1 struct {
	OriginalEffectKind    string     `json:"original_effect_kind"`
	OriginalAttemptID     string     `json:"original_attempt_id"`
	ProviderAttempt       ExactRefV1 `json:"provider_attempt"`
	OriginalRequestDigest string     `json:"original_request_digest"`
	OriginalPayloadDigest string     `json:"original_payload_digest"`
}

type ContainerPayloadV1 struct {
	RootfsBindingID  string                      `json:"rootfs_binding_id"`
	ImageDigest      string                      `json:"image_digest"`
	Argv             []string                    `json:"argv"`
	Environment      map[string]string           `json:"environment"`
	WorkingDirectory string                      `json:"working_directory"`
	ReadOnlyRootfs   bool                        `json:"read_only_rootfs"`
	CPUQuotaMicros   uint64                      `json:"cpu_quota_micros"`
	MemoryLimitBytes uint64                      `json:"memory_limit_bytes"`
	PidsLimit        uint64                      `json:"pids_limit"`
	NetworkDenyAll   bool                        `json:"network_deny_all"`
	InspectionTarget *ProviderInspectionTargetV1 `json:"inspection_target,omitempty"`
}

type WasmPayloadV1 struct {
	ComponentPathBindingID string                      `json:"component_path_binding_id"`
	ComponentDigest        string                      `json:"component_digest"`
	World                  string                      `json:"world"`
	Export                 string                      `json:"export"`
	Fuel                   uint64                      `json:"fuel"`
	EpochDeadlineTicks     uint64                      `json:"epoch_deadline_ticks"`
	MemoryLimitBytes       uint64                      `json:"memory_limit_bytes"`
	TableElementsLimit     uint64                      `json:"table_elements_limit"`
	InstanceLimit          uint64                      `json:"instance_limit"`
	CapabilityBindings     []WasmCapabilityBindingV1   `json:"capability_bindings,omitempty"`
	InspectionTarget       *ProviderInspectionTargetV1 `json:"inspection_target,omitempty"`
}

type WasmCapabilityBindingV1 struct {
	Name             string     `json:"name"`
	Grant            ExactRefV1 `json:"grant"`
	RequestSchema    string     `json:"request_schema"`
	ResponseSchema   string     `json:"response_schema"`
	MaxRequestBytes  uint64     `json:"max_request_bytes"`
	MaxResponseBytes uint64     `json:"max_response_bytes"`
}

type HostWorkspacePayloadV1 struct {
	WorkspaceBindingID    string                      `json:"workspace_binding_id"`
	WorkspaceDigest       string                      `json:"workspace_digest"`
	ToolBindingID         string                      `json:"tool_binding_id"`
	ToolDigest            string                      `json:"tool_digest"`
	Argv                  []string                    `json:"argv"`
	Environment           map[string]string           `json:"environment"`
	WorkingDirectory      string                      `json:"working_directory"`
	NetworkDenyAll        bool                        `json:"network_deny_all"`
	WallClockTimeoutMilli uint64                      `json:"wall_clock_timeout_millis"`
	InspectionTarget      *ProviderInspectionTargetV1 `json:"inspection_target,omitempty"`
}

type MicroVMPayloadV1 struct {
	KernelBindingID       string                      `json:"kernel_binding_id"`
	KernelDigest          string                      `json:"kernel_digest"`
	InitramfsBindingID    string                      `json:"initramfs_binding_id"`
	InitramfsDigest       string                      `json:"initramfs_digest"`
	VCPUs                 uint16                      `json:"vcpus"`
	MemoryMiB             uint32                      `json:"memory_mib"`
	NetworkDenyAll        bool                        `json:"network_deny_all"`
	WallClockTimeoutMilli uint64                      `json:"wall_clock_timeout_millis"`
	InspectionTarget      *ProviderInspectionTargetV1 `json:"inspection_target,omitempty"`
}

// RemotePayloadV1 carries only opaque, exact bindings. Endpoint addresses and
// credential material remain in the trusted Remote transport implementation
// and never cross the Runtime/Sandbox dispatch contract.
type RemotePayloadV1 struct {
	EndpointBindingID string                      `json:"endpoint_binding_id"`
	EndpointDigest    string                      `json:"endpoint_digest"`
	WorkloadID        string                      `json:"workload_id"`
	WorkloadDigest    string                      `json:"workload_digest"`
	Credential        ExactRefV1                  `json:"credential"`
	IsolationProfile  string                      `json:"isolation_profile"`
	InspectionTarget  *ProviderInspectionTargetV1 `json:"inspection_target,omitempty"`
}

type WorkspaceMutationV1 struct {
	Kind       string `json:"kind"`
	Path       string `json:"path"`
	TargetPath string `json:"target_path,omitempty"`
	BlobID     string `json:"blob_id,omitempty"`
	BlobDigest string `json:"blob_digest,omitempty"`
	Mode       uint32 `json:"mode,omitempty"`
}

type WorkspaceCommitPayloadV1 struct {
	WorkspaceBindingID string                      `json:"workspace_binding_id"`
	WorkspaceDigest    string                      `json:"workspace_digest"`
	ChangeSet          ExactRefV1                  `json:"change_set"`
	View               ExactRefV1                  `json:"view"`
	BaseRevision       string                      `json:"base_revision"`
	FileScopeDigest    string                      `json:"file_scope_digest"`
	WriteScopes        []string                    `json:"write_scopes"`
	Changes            []WorkspaceMutationV1       `json:"changes"`
	InspectionTarget   *ProviderInspectionTargetV1 `json:"inspection_target,omitempty"`
}

type DispatchRequestV1 struct {
	ContractVersion           string                  `json:"contract_version"`
	RequestID                 string                  `json:"request_id"`
	Phase                     EnforcementPhaseV1      `json:"phase"`
	EffectKind                string                  `json:"effect_kind"`
	OperationDigest           string                  `json:"operation_digest"`
	EffectID                  string                  `json:"effect_id"`
	IntentRevision            uint64                  `json:"intent_revision"`
	IntentDigest              string                  `json:"intent_digest"`
	AttemptID                 string                  `json:"attempt_id"`
	TenantID                  string                  `json:"tenant_id"`
	ProviderBinding           ProviderBindingV1       `json:"provider_binding"`
	SandboxAttempt            ExactRefV1              `json:"sandbox_attempt"`
	ExecutionBinding          ExecutionBindingV1      `json:"execution_binding"`
	RuntimeEnforcement        RuntimeEnforcementRefV1 `json:"runtime_enforcement"`
	RuntimeCurrentQuery       json.RawMessage         `json:"runtime_current_query"`
	RuntimeCurrentQueryDigest string                  `json:"runtime_current_query_digest"`
	RequestedNotAfterUnixNano int64                   `json:"requested_not_after_unix_nano"`
	PayloadSchema             string                  `json:"payload_schema"`
	PayloadDigest             string                  `json:"payload_digest"`
	PayloadRevision           uint64                  `json:"payload_revision"`
	Payload                   ProviderPayloadV1       `json:"payload"`
	Digest                    string                  `json:"digest"`
}

type CurrentAuthorizationV1 struct {
	ContractVersion    string                  `json:"contract_version"`
	RequestDigest      string                  `json:"request_digest"`
	OperationDigest    string                  `json:"operation_digest"`
	EffectID           string                  `json:"effect_id"`
	AttemptID          string                  `json:"attempt_id"`
	Phase              EnforcementPhaseV1      `json:"phase"`
	ProviderBinding    ProviderBindingV1       `json:"provider_binding"`
	SandboxProjection  SandboxProjectionRefV1  `json:"sandbox_projection"`
	ExecutionBinding   ExecutionBindingV1      `json:"execution_binding"`
	RuntimeEnforcement RuntimeEnforcementRefV1 `json:"runtime_enforcement"`
	CheckedUnixNano    int64                   `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                   `json:"expires_unix_nano"`
	Digest             string                  `json:"digest"`
}

type ClosedError struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func (e ClosedError) Error() string { return e.Reason + ": " + e.Message }

type DispatchResponseV1 struct {
	ContractVersion     string                 `json:"contract_version"`
	RequestID           string                 `json:"request_id"`
	RequestDigest       string                 `json:"request_digest"`
	Accepted            bool                   `json:"accepted"`
	ProviderAttempt     *ExactRefV1            `json:"provider_attempt"`
	ProviderObservation *ProviderObservationV1 `json:"provider_observation"`
	ProviderReceipt     *ProviderReceiptV1     `json:"provider_receipt"`
	ObservationDigest   *string                `json:"observation_digest"`
	ReceiptDigest       *string                `json:"receipt_digest"`
	CheckedUnixNano     int64                  `json:"checked_unix_nano"`
	ExpiresUnixNano     int64                  `json:"expires_unix_nano"`
	Error               *ClosedError           `json:"error"`
	Digest              string                 `json:"digest"`
}

type ProviderObservationV1 struct {
	Provider           string                           `json:"provider"`
	Attempt            ExactRefV1                       `json:"attempt"`
	State              string                           `json:"state"`
	PayloadDigest      string                           `json:"payload_digest"`
	CheckpointArtifact *CheckpointArtifactObservationV1 `json:"checkpoint_artifact,omitempty"`
	WorkspaceCommit    *WorkspaceCommitObservationV1    `json:"workspace_commit,omitempty"`
	ObservedUnixNano   int64                            `json:"observed_unix_nano"`
	Digest             string                           `json:"digest"`
}

type WorkspaceCommitObservationV1 struct {
	ContractVersion   string     `json:"contract_version"`
	ChangeSet         ExactRefV1 `json:"change_set"`
	View              ExactRefV1 `json:"view"`
	BaseRevision      string     `json:"base_revision"`
	CommittedRevision string     `json:"committed_revision"`
	State             string     `json:"state"`
	RecordedUnixNano  int64      `json:"recorded_unix_nano"`
	ExpiresUnixNano   int64      `json:"expires_unix_nano"`
}

type CheckpointArtifactObservationV1 struct {
	ContractVersion  string `json:"contract_version"`
	ArtifactID       string `json:"artifact_id"`
	SubjectDigest    string `json:"subject_digest"`
	ContentDigest    string `json:"content_digest"`
	ContentLength    uint64 `json:"content_length"`
	State            string `json:"state"`
	CheckpointPhase  string `json:"checkpoint_phase"`
	RecordedUnixNano int64  `json:"recorded_unix_nano"`
	ExpiresUnixNano  int64  `json:"expires_unix_nano"`
}

type ProviderReceiptV1 struct {
	Provider          string     `json:"provider"`
	Attempt           ExactRefV1 `json:"attempt"`
	Phase             string     `json:"phase"`
	ObservationDigest string     `json:"observation_digest"`
	RecordedUnixNano  int64      `json:"recorded_unix_nano"`
	ExpiresUnixNano   int64      `json:"expires_unix_nano"`
	Digest            string     `json:"digest"`
}

type DispatchInput struct {
	RequestID         string
	Current           runtimeports.CurrentOperationDispatchEnforcementV4
	EffectKind        string
	PayloadSchema     string
	PayloadRevision   uint64
	Payload           ProviderPayloadV1
	RequestedNotAfter time.Time
}

func NewContainerPayload(value ContainerPayloadV1) (ProviderPayloadV1, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return ProviderPayloadV1{}, err
	}
	return ProviderPayloadV1{ProviderKind: "containerd_oci", ProviderPayload: raw}, nil
}

func NewWasmPayload(value WasmPayloadV1) (ProviderPayloadV1, error) {
	seen := make(map[string]struct{}, len(value.CapabilityBindings))
	if len(value.CapabilityBindings) > 64 {
		return ProviderPayloadV1{}, errors.New("WASM capability binding limit exceeded")
	}
	for _, binding := range value.CapabilityBindings {
		if strings.TrimSpace(binding.Name) == "" || strings.TrimSpace(binding.Grant.ID) == "" || binding.Grant.Revision == 0 || !validDigest(binding.Grant.Digest) || binding.Grant.ExpiresUnixNano <= 0 || strings.TrimSpace(binding.RequestSchema) == "" || strings.TrimSpace(binding.ResponseSchema) == "" || binding.MaxRequestBytes == 0 || binding.MaxResponseBytes == 0 || binding.MaxRequestBytes > 4*1024*1024 || binding.MaxResponseBytes > 4*1024*1024 {
			return ProviderPayloadV1{}, errors.New("WASM capability binding violates the closed policy")
		}
		if _, exists := seen[binding.Name]; exists {
			return ProviderPayloadV1{}, errors.New("WASM capability binding is duplicated")
		}
		seen[binding.Name] = struct{}{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ProviderPayloadV1{}, err
	}
	return ProviderPayloadV1{ProviderKind: "wasmtime_component", ProviderPayload: raw}, nil
}

func NewHostWorkspacePayload(value HostWorkspacePayloadV1) (ProviderPayloadV1, error) {
	if strings.TrimSpace(value.WorkspaceBindingID) == "" || !validDigest(value.WorkspaceDigest) || strings.TrimSpace(value.ToolBindingID) == "" || !validDigest(value.ToolDigest) || len(value.Argv) > 256 || value.WorkingDirectory == "" || path.IsAbs(value.WorkingDirectory) || path.Clean(value.WorkingDirectory) != value.WorkingDirectory || value.WorkingDirectory == "." || !value.NetworkDenyAll || value.WallClockTimeoutMilli == 0 || value.WallClockTimeoutMilli > 86_400_000 || len(value.Environment) > 128 {
		return ProviderPayloadV1{}, errors.New("host workspace payload violates the closed policy")
	}
	for _, segment := range strings.Split(value.WorkingDirectory, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return ProviderPayloadV1{}, errors.New("host workspace working directory is invalid")
		}
	}
	for _, argument := range value.Argv {
		if argument == "" || strings.ContainsRune(argument, 0) {
			return ProviderPayloadV1{}, errors.New("host workspace argument is invalid")
		}
	}
	for key, value := range value.Environment {
		if key == "" || len(key) > 128 || strings.ContainsAny(key, "=\x00") || len(value) > 16*1024 || strings.ContainsRune(value, 0) {
			return ProviderPayloadV1{}, errors.New("host workspace environment is invalid")
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ProviderPayloadV1{}, err
	}
	return ProviderPayloadV1{ProviderKind: "host_workspace", ProviderPayload: raw}, nil
}

func NewMicroVMPayload(value MicroVMPayloadV1) (ProviderPayloadV1, error) {
	if strings.TrimSpace(value.KernelBindingID) == "" || !validDigest(value.KernelDigest) || strings.TrimSpace(value.InitramfsBindingID) == "" || !validDigest(value.InitramfsDigest) || value.VCPUs == 0 || value.VCPUs > 64 || value.MemoryMiB < 64 || value.MemoryMiB > 1_048_576 || !value.NetworkDenyAll || value.WallClockTimeoutMilli == 0 || value.WallClockTimeoutMilli > 604_800_000 {
		return ProviderPayloadV1{}, errors.New("microVM payload violates the closed policy")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ProviderPayloadV1{}, err
	}
	return ProviderPayloadV1{ProviderKind: "qemu_microvm", ProviderPayload: raw}, nil
}

func NewRemotePayload(value RemotePayloadV1) (ProviderPayloadV1, error) {
	if strings.TrimSpace(value.EndpointBindingID) == "" || !validDigest(value.EndpointDigest) ||
		strings.TrimSpace(value.WorkloadID) == "" || !validDigest(value.WorkloadDigest) ||
		strings.TrimSpace(value.Credential.ID) == "" || value.Credential.Revision == 0 ||
		!validDigest(value.Credential.Digest) || value.Credential.ExpiresUnixNano <= 0 ||
		strings.TrimSpace(value.IsolationProfile) == "" {
		return ProviderPayloadV1{}, errors.New("remote payload violates the closed policy")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ProviderPayloadV1{}, err
	}
	return ProviderPayloadV1{ProviderKind: "remote_sandbox", ProviderPayload: raw}, nil
}

func NewWorkspaceCommitPayload(value WorkspaceCommitPayloadV1) (ProviderPayloadV1, error) {
	if strings.TrimSpace(value.WorkspaceBindingID) == "" || !validDigest(value.WorkspaceDigest) ||
		value.ChangeSet.ID == "" || value.ChangeSet.Revision == 0 || !validDigest(value.ChangeSet.Digest) || value.ChangeSet.ExpiresUnixNano <= 0 ||
		value.View.ID == "" || value.View.Revision == 0 || !validDigest(value.View.Digest) || value.View.ExpiresUnixNano <= 0 ||
		!validDigest(value.BaseRevision) || !validDigest(value.FileScopeDigest) || len(value.WriteScopes) == 0 || len(value.WriteScopes) > 256 || len(value.Changes) == 0 || len(value.Changes) > 4096 {
		return ProviderPayloadV1{}, errors.New("workspace commit payload violates the closed policy")
	}
	previousScope := ""
	for _, scope := range value.WriteScopes {
		if !validLogicalWirePath(scope) || scope <= previousScope {
			return ProviderPayloadV1{}, errors.New("workspace commit scopes are not canonical, sorted, and unique")
		}
		previousScope = scope
	}
	previousChange := ""
	touched := make(map[string]struct{}, len(value.Changes)*2)
	for _, change := range value.Changes {
		key := change.Path + "\x00" + change.Kind + "\x00" + change.TargetPath
		if !validLogicalWirePath(change.Path) || key <= previousChange || !withinWireScope(change.Path, value.WriteScopes) {
			return ProviderPayloadV1{}, errors.New("workspace commit changes are not canonical, sorted, or scoped")
		}
		if _, exists := touched[change.Path]; exists {
			return ProviderPayloadV1{}, errors.New("workspace commit path is touched more than once")
		}
		touched[change.Path] = struct{}{}
		switch change.Kind {
		case "add", "modify":
			if change.TargetPath != "" || !strings.HasPrefix(change.BlobID, "workspace-blob-") || len(change.BlobID) != len("workspace-blob-")+64 || !validDigest(change.BlobDigest) || change.Mode == 0 || change.Mode&^0o777 != 0 {
				return ProviderPayloadV1{}, errors.New("workspace add/modify mutation is incomplete")
			}
		case "delete":
			if change.TargetPath != "" || change.BlobID != "" || change.BlobDigest != "" || change.Mode != 0 {
				return ProviderPayloadV1{}, errors.New("workspace delete mutation carries forbidden fields")
			}
		case "rename":
			if !validLogicalWirePath(change.TargetPath) || !withinWireScope(change.TargetPath, value.WriteScopes) || change.BlobID != "" || change.BlobDigest != "" || change.Mode != 0 {
				return ProviderPayloadV1{}, errors.New("workspace rename mutation is incomplete")
			}
			if _, exists := touched[change.TargetPath]; exists {
				return ProviderPayloadV1{}, errors.New("workspace commit target path is touched more than once")
			}
			touched[change.TargetPath] = struct{}{}
		default:
			return ProviderPayloadV1{}, errors.New("workspace mutation kind is unsupported")
		}
		previousChange = key
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ProviderPayloadV1{}, err
	}
	return ProviderPayloadV1{ProviderKind: "workspace_commit", ProviderPayload: raw}, nil
}

func validLogicalWirePath(value string) bool {
	return value != "" && !path.IsAbs(value) && !strings.Contains(value, "\\") && path.Clean(value) == value && value != "." && value != ".." && !strings.HasPrefix(value, "../")
}

func withinWireScope(value string, scopes []string) bool {
	for _, scope := range scopes {
		if value == scope || strings.HasPrefix(value, scope+"/") {
			return true
		}
	}
	return false
}

func (p ProviderPayloadV1) InspectionTarget() (*ProviderInspectionTargetV1, error) {
	var envelope struct {
		InspectionTarget *ProviderInspectionTargetV1 `json:"inspection_target"`
	}
	if p.ProviderKind != "host_workspace" && p.ProviderKind != "qemu_microvm" && p.ProviderKind != "containerd_oci" && p.ProviderKind != "wasmtime_component" && p.ProviderKind != "remote_sandbox" && p.ProviderKind != "workspace_commit" {
		return nil, errors.New("provider payload kind is unsupported")
	}
	if err := json.Unmarshal(p.ProviderPayload, &envelope); err != nil {
		return nil, errors.New("provider payload is malformed")
	}
	return envelope.InspectionTarget, nil
}

func NewDispatchRequestV1(input DispatchInput) (DispatchRequestV1, error) {
	if err := input.Current.Validate(); err != nil {
		return DispatchRequestV1{}, fmt.Errorf("runtime enforcement current: %w", err)
	}
	phase, err := phaseFromRuntime(input.Current.Phase.Phase)
	if err != nil {
		return DispatchRequestV1{}, err
	}
	query := runtimeports.InspectCurrentOperationDispatchEnforcementRequestV4{
		Inspect: runtimeports.InspectOperationDispatchEnforcementRequestV4{
			Operation: input.Current.Sandbox.Operation,
			EffectID:  input.Current.Sandbox.EffectID,
			PermitID:  input.Current.Phase.PermitID,
			Phase:     input.Current.Phase.Phase,
		},
		PermitDigest:            input.Current.Phase.PermitDigest,
		AdmissionDigest:         input.Current.Phase.AdmissionDigest,
		ReviewAuthorization:     input.Current.Phase.ReviewAuthorization,
		SandboxAttempt:          input.Current.Phase.SandboxAttempt,
		SandboxProjectionDigest: input.Current.Sandbox.ProjectionDigest,
	}
	if err := query.Validate(); err != nil {
		return DispatchRequestV1{}, fmt.Errorf("runtime current query: %w", err)
	}
	queryJSON, err := json.Marshal(query)
	if err != nil {
		return DispatchRequestV1{}, err
	}
	queryJSON, err = canonicalJSON(queryJSON)
	if err != nil {
		return DispatchRequestV1{}, err
	}
	provider, err := providerFromRuntime(input.Current.Sandbox.ProviderBinding)
	if err != nil {
		return DispatchRequestV1{}, err
	}
	request := DispatchRequestV1{
		ContractVersion:           ContractVersionV1,
		RequestID:                 input.RequestID,
		Phase:                     phase,
		EffectKind:                input.EffectKind,
		OperationDigest:           string(input.Current.Sandbox.OperationDigest),
		EffectID:                  string(input.Current.Sandbox.EffectID),
		IntentRevision:            uint64(input.Current.Sandbox.IntentRevision),
		IntentDigest:              string(input.Current.Sandbox.IntentDigest),
		AttemptID:                 input.Current.Sandbox.AttemptID,
		TenantID:                  string(input.Current.Sandbox.Operation.ExecutionScope.Identity.TenantID),
		ProviderBinding:           provider,
		SandboxAttempt:            factRef(input.Current.Sandbox.Attempt),
		ExecutionBinding:          executionBinding(input.Current),
		RuntimeEnforcement:        enforcementRef(input.Current.Phase, phase),
		RuntimeCurrentQuery:       queryJSON,
		RequestedNotAfterUnixNano: input.RequestedNotAfter.UnixNano(),
		PayloadSchema:             input.PayloadSchema,
		PayloadRevision:           input.PayloadRevision,
		Payload:                   input.Payload,
	}
	request.RuntimeCurrentQueryDigest, err = canonicalDigest("RuntimeCurrentQueryV1", json.RawMessage(queryJSON))
	if err != nil {
		return DispatchRequestV1{}, err
	}
	request.PayloadDigest, err = canonicalDigest("ProviderPayloadV1", request.Payload)
	if err != nil {
		return DispatchRequestV1{}, err
	}
	legacy := input.Current.Dispatch.Record.Permit.LegacyPermit
	if err := validateProviderPayloadBinding(legacy.PayloadSchema, legacy.PayloadDigest, legacy.PayloadRevision, request.PayloadSchema, request.PayloadDigest, request.PayloadRevision); err != nil {
		return DispatchRequestV1{}, errors.New("provider payload differs from the exact Runtime Permit")
	}
	request.Digest, err = request.digestV1()
	if err != nil {
		return DispatchRequestV1{}, err
	}
	return request, request.ValidateCurrent(time.Now())
}

func validateProviderPayloadBinding(expectedSchema runtimeports.SchemaRefV2, expectedDigest runtimecore.Digest, expectedRevision runtimecore.Revision, schema, digest string, revision uint64) error {
	if expectedSchema.Validate() != nil || expectedDigest.Validate() != nil || expectedRevision == 0 || schema != expectedSchema.Key() || digest != string(expectedDigest) || revision != uint64(expectedRevision) {
		return errors.New("provider payload binding drifted")
	}
	return nil
}

func (r DispatchRequestV1) ValidateCurrent(now time.Time) error {
	if r.ContractVersion != ContractVersionV1 || strings.TrimSpace(r.RequestID) == "" || !validEffectKind(r.EffectKind) || strings.TrimSpace(r.EffectID) == "" || strings.TrimSpace(r.AttemptID) == "" || strings.TrimSpace(r.TenantID) == "" || r.IntentRevision == 0 || r.PayloadRevision == 0 || strings.TrimSpace(r.PayloadSchema) == "" {
		return errors.New("data plane dispatch identity is incomplete")
	}
	for _, digest := range []string{r.OperationDigest, r.IntentDigest, r.RuntimeCurrentQueryDigest, r.PayloadDigest, r.Digest} {
		if !validDigest(digest) {
			return errors.New("data plane dispatch digest is invalid")
		}
	}
	if now.IsZero() || !now.Before(time.Unix(0, r.RequestedNotAfterUnixNano)) || !now.Before(time.Unix(0, r.SandboxAttempt.ExpiresUnixNano)) || !now.Before(time.Unix(0, r.RuntimeEnforcement.ExpiresUnixNano)) {
		return errors.New("data plane dispatch is expired")
	}
	if r.RuntimeEnforcement.OperationDigest != r.OperationDigest || r.RuntimeEnforcement.EffectID != r.EffectID || r.RuntimeEnforcement.AttemptID != r.AttemptID || r.RuntimeEnforcement.Phase != r.Phase {
		return errors.New("runtime enforcement ref drifted from dispatch")
	}
	if r.SandboxAttempt.ID != r.AttemptID || r.SandboxAttempt.Revision == 0 || !validDigest(r.SandboxAttempt.Digest) {
		return errors.New("sandbox attempt exact ref drifted")
	}
	if r.ExecutionBinding.TenantID != r.TenantID || r.ExecutionBinding.InstanceID == "" || r.ExecutionBinding.LeaseID == "" || r.ExecutionBinding.InstanceEpoch == 0 || r.ExecutionBinding.LeaseEpoch == 0 || r.ExecutionBinding.FenceEpoch == 0 || r.ExecutionBinding.ObservedRevision == 0 || !validDigest(r.ExecutionBinding.ScopeDigest) || !now.Before(time.Unix(0, r.ExecutionBinding.ExpiresUnixNano)) {
		return errors.New("execution binding is incomplete, expired, or drifted")
	}
	if err := validateInspectionTarget(r.EffectKind, r.TenantID, r.Payload, now); err != nil {
		return err
	}
	queryDigest, err := canonicalDigest("RuntimeCurrentQueryV1", json.RawMessage(r.RuntimeCurrentQuery))
	if err != nil || queryDigest != r.RuntimeCurrentQueryDigest {
		return errors.New("runtime current query digest drifted")
	}
	payloadDigest, err := canonicalDigest("ProviderPayloadV1", r.Payload)
	if err != nil || payloadDigest != r.PayloadDigest {
		return errors.New("provider payload digest drifted")
	}
	digest, err := r.digestV1()
	if err != nil || digest != r.Digest {
		return errors.New("dispatch digest drifted")
	}
	return nil
}

func validateInspectionTarget(effectKind, tenantID string, payload ProviderPayloadV1, now time.Time) error {
	target, err := payload.InspectionTarget()
	if err != nil {
		return err
	}
	allowsTarget := effectKind == "praxis.sandbox/inspect" || (effectKind == "praxis.sandbox/cleanup" && payload.ProviderKind == "workspace_commit")
	if !allowsTarget {
		if target != nil {
			return errors.New("dispatch payload carries a forbidden inspection target")
		}
		return nil
	}
	if target == nil {
		return errors.New("inspection payload lacks an exact original target")
	}
	if !inspectableOriginalEffectKind(target.OriginalEffectKind) {
		return errors.New("inspection target effect is unsupported")
	}
	if strings.TrimSpace(target.OriginalAttemptID) == "" || target.ProviderAttempt.Revision != 2 || !validDigest(target.ProviderAttempt.Digest) || !now.Before(time.Unix(0, target.ProviderAttempt.ExpiresUnixNano)) || !validDigest(target.OriginalRequestDigest) || !validDigest(target.OriginalPayloadDigest) {
		return errors.New("inspection target exact coordinates are incomplete or expired")
	}
	providerName := "containerd"
	if payload.ProviderKind == "host_workspace" {
		providerName = "host"
	} else if payload.ProviderKind == "qemu_microvm" {
		providerName = "microvm"
	} else if payload.ProviderKind == "wasmtime_component" {
		providerName = "wasmtime"
	} else if payload.ProviderKind == "remote_sandbox" {
		providerName = "remote"
	} else if payload.ProviderKind == "workspace_commit" {
		providerName = "workspace-commit"
	}
	if target.ProviderAttempt.ID != providerName+"/"+tenantID+"/"+target.OriginalAttemptID {
		return errors.New("inspection target provider attempt identity drifted")
	}
	return nil
}

func inspectableOriginalEffectKind(value string) bool {
	switch value {
	case "praxis.sandbox/backend-discovery", "praxis.sandbox/allocate", "praxis.sandbox/activate",
		"praxis.sandbox/open", "praxis.sandbox/cancel", "praxis.sandbox/close",
		"praxis.sandbox/fence", "praxis.sandbox/release", "praxis.sandbox/cleanup",
		"praxis.sandbox/workspace-commit":
		return true
	default:
		return false
	}
}

func (r DispatchRequestV1) digestV1() (string, error) {
	copy := r
	copy.Digest = ""
	return canonicalDigest("DispatchRequestV1", copy)
}

func providerFromRuntime(value runtimeports.ProviderBindingRefV2) (ProviderBindingV1, error) {
	result := ProviderBindingV1{
		BindingSetID:       value.BindingSetID,
		BindingSetRevision: uint64(value.BindingSetRevision),
		ComponentID:        string(value.ComponentID),
		ManifestDigest:     string(value.ManifestDigest),
		ArtifactDigest:     string(value.ArtifactDigest),
		Capability:         string(value.Capability),
	}
	var err error
	result.Digest, err = canonicalDigest("ProviderBindingV1", result)
	return result, err
}

func factRef(value runtimeports.OperationDispatchSandboxFactRefV4) ExactRefV1 {
	return ExactRefV1{ID: value.ID, Revision: uint64(value.Revision), Digest: string(value.Digest), ExpiresUnixNano: value.ExpiresUnixNano}
}

func enforcementRef(value runtimeports.OperationDispatchEnforcementPhaseRefV4, phase EnforcementPhaseV1) RuntimeEnforcementRefV1 {
	return RuntimeEnforcementRefV1{
		OperationDigest: string(value.OperationDigest), EffectID: string(value.EffectID), PermitID: value.PermitID,
		AttemptID: value.AttemptID, Phase: phase, ReceiptDigest: string(value.ReceiptDigest),
		JournalRevision: uint64(value.JournalRevision), ExpiresUnixNano: value.ExpiresUnixNano,
	}
}

func executionBinding(value runtimeports.CurrentOperationDispatchEnforcementV4) ExecutionBindingV1 {
	binding := value.Sandbox.RuntimeLease
	return ExecutionBindingV1{
		TenantID:   string(value.Sandbox.Operation.ExecutionScope.Identity.TenantID),
		InstanceID: string(binding.Instance.ID), InstanceEpoch: uint64(binding.Instance.Epoch),
		LeaseID: string(binding.Lease.ID), LeaseEpoch: uint64(binding.Lease.Epoch), FenceEpoch: uint64(binding.FenceEpoch),
		ScopeDigest: string(binding.ScopeDigest), ObservedRevision: uint64(binding.ObservedRevision), ExpiresUnixNano: binding.Ref.ExpiresUnixNano,
	}
}

func phaseFromRuntime(value runtimeports.OperationDispatchEnforcementPhaseV4) (EnforcementPhaseV1, error) {
	switch value {
	case runtimeports.OperationDispatchEnforcementPrepareV4:
		return PhasePrepare, nil
	case runtimeports.OperationDispatchEnforcementExecuteV4:
		return PhaseExecute, nil
	default:
		return "", errors.New("runtime enforcement phase is unsupported")
	}
}

func runtimePhase(value EnforcementPhaseV1) (runtimeports.OperationDispatchEnforcementPhaseV4, error) {
	switch value {
	case PhasePrepare:
		return runtimeports.OperationDispatchEnforcementPrepareV4, nil
	case PhaseExecute:
		return runtimeports.OperationDispatchEnforcementExecuteV4, nil
	default:
		return "", errors.New("only prepare and execute have Runtime V4 current enforcement")
	}
}

func runtimeDigest(value string) runtimecore.Digest { return runtimecore.Digest(value) }

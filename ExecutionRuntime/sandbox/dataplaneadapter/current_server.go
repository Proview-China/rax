package dataplaneadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
)

type CurrentReadResponseV1 struct {
	Authorization *CurrentAuthorizationV1 `json:"authorization"`
	Error         *ClosedError            `json:"error"`
}

type CurrentServer struct {
	SocketPath             string
	SocketMode             os.FileMode
	AllowedUID             uint32
	Governance             runtimeports.OperationDispatchEnforcementGovernancePortV4
	CheckpointGovernance   runtimeports.CheckpointRestoreDispatchEnforcementGovernancePortV1
	Sandbox                sandboxports.ExactCurrentStore
	WorkspaceReadCurrent   sandboxports.WorkspaceReadCurrentProjectionReaderV1
	WorkspaceReadCurrentV2 *runtimeadapter.WorkspaceReadCurrentAdapterV2
	Now                    func() time.Time
}

func (s CurrentServer) Listen() (*net.UnixListener, error) {
	if s.Governance == nil || s.Sandbox == nil || s.Now == nil || s.SocketPath == "" || s.SocketMode&0o007 != 0 {
		return nil, errors.New("current server configuration is incomplete")
	}
	if err := os.MkdirAll(filepath.Dir(s.SocketPath), 0o750); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(s.SocketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("current socket path is occupied by a non-socket")
		}
		if err := os.Remove(s.SocketPath); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: s.SocketPath, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(s.SocketPath, s.SocketMode); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

func (s CurrentServer) Serve(ctx context.Context, listener *net.UnixListener) error {
	if listener == nil {
		return errors.New("current listener is required")
	}
	go func() {
		<-ctx.Done()
		listener.Close()
	}()
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		go s.serveConnection(ctx, connection)
	}
}

func (s CurrentServer) serveConnection(ctx context.Context, connection *net.UnixConn) {
	defer connection.Close()
	if err := validatePeerUID(connection, s.AllowedUID); err != nil {
		return
	}
	var request DispatchRequestV1
	if err := readFrame(connection, &request); err != nil {
		return
	}
	authorization, err := s.inspect(ctx, request)
	response := CurrentReadResponseV1{}
	if err != nil {
		response.Error = &ClosedError{Reason: "current_unavailable", Message: err.Error()}
	} else {
		response.Authorization = &authorization
	}
	_ = writeFrame(connection, response)
}

func (s CurrentServer) inspect(ctx context.Context, request DispatchRequestV1) (CurrentAuthorizationV1, error) {
	now := s.Now()
	if err := request.ValidateCurrent(now); err != nil {
		return CurrentAuthorizationV1{}, err
	}
	if request.EffectKind == CheckpointEffectKindV1 {
		return s.inspectCheckpointV1(ctx, request, now)
	}
	if request.Payload.ProviderKind == "workspace_read" && request.Phase == PhaseExecute {
		if workspaceReadCurrentQueryV2(request.RuntimeCurrentQuery) {
			return s.inspectWorkspaceReadV2(ctx, request, now)
		}
		return CurrentAuthorizationV1{}, errors.New("workspace read physical execution requires exact current v2")
	}
	var query runtimeports.InspectCurrentOperationDispatchEnforcementRequestV4
	decoder := json.NewDecoder(bytesReader(request.RuntimeCurrentQuery))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&query); err != nil {
		return CurrentAuthorizationV1{}, fmt.Errorf("runtime current query decode: %w", err)
	}
	if err := query.Validate(); err != nil {
		return CurrentAuthorizationV1{}, err
	}
	phase, err := runtimePhase(request.Phase)
	if err != nil || query.Inspect.Phase != phase || string(query.Inspect.EffectID) != request.EffectID {
		return CurrentAuthorizationV1{}, errors.New("runtime current query phase or effect drifted")
	}
	current, err := s.Governance.InspectCurrentOperationDispatchEnforcementV4(ctx, query)
	if err != nil {
		return CurrentAuthorizationV1{}, err
	}
	if err := current.Validate(); err != nil {
		return CurrentAuthorizationV1{}, err
	}
	if now.IsZero() || !current.Sandbox.Current || !now.Before(time.Unix(0, current.ExpiresUnixNano)) || !now.Before(time.Unix(0, current.Sandbox.ExpiresUnixNano)) {
		return CurrentAuthorizationV1{}, errors.New("runtime or sandbox enforcement is not current")
	}
	provider, err := providerFromRuntime(current.Sandbox.ProviderBinding)
	if err != nil || provider != request.ProviderBinding {
		return CurrentAuthorizationV1{}, errors.New("provider binding drifted at actual execution point")
	}
	actualRef := enforcementRef(current.Phase, request.Phase)
	actualBinding := executionBinding(current)
	operationID := exactOperationID(current.Sandbox.Operation)
	reservation, err := s.Sandbox.InspectReservationByAttempt(ctx, operationID, request.EffectID, request.AttemptID)
	if err != nil {
		return CurrentAuthorizationV1{}, err
	}
	if err := reservation.ValidateCurrent(now); err != nil || string(reservation.Kind) != request.EffectKind || reservation.Meta.ID != current.Sandbox.Reservation.ID || reservation.Meta.Revision != uint64(current.Sandbox.Reservation.Revision) || reservation.Meta.Digest != string(current.Sandbox.Reservation.Digest) {
		return CurrentAuthorizationV1{}, errors.New("sandbox effect kind or reservation current drifted")
	}
	if actualRef != request.RuntimeEnforcement || actualBinding != request.ExecutionBinding || current.Sandbox.Attempt != query.SandboxAttempt || factRef(current.Sandbox.Attempt) != request.SandboxAttempt || string(current.Sandbox.OperationDigest) != request.OperationDigest || string(current.Sandbox.EffectID) != request.EffectID || current.Sandbox.AttemptID != request.AttemptID {
		return CurrentAuthorizationV1{}, errors.New("actual-point enforcement coordinates drifted")
	}
	if request.SandboxProjection != nil && !sameSandboxProjectionV1(*request.SandboxProjection, current.Sandbox.ProjectionRevision, current.Sandbox.ProjectionDigest, current.Sandbox.ExpiresUnixNano) {
		return CurrentAuthorizationV1{}, errors.New("request sandbox projection drifted from current")
	}
	expires := minimum(request.RequestedNotAfterUnixNano, current.ExpiresUnixNano, current.Sandbox.ExpiresUnixNano, current.Phase.ExpiresUnixNano)
	authorization := CurrentAuthorizationV1{
		ContractVersion:    ContractVersionV1,
		RequestDigest:      request.Digest,
		OperationDigest:    request.OperationDigest,
		EffectID:           request.EffectID,
		AttemptID:          request.AttemptID,
		Phase:              request.Phase,
		ProviderBinding:    provider,
		SandboxProjection:  SandboxProjectionRefV1{Revision: uint64(current.Sandbox.ProjectionRevision), Digest: string(current.Sandbox.ProjectionDigest), ExpiresUnixNano: current.Sandbox.ExpiresUnixNano},
		ExecutionBinding:   actualBinding,
		RuntimeEnforcement: actualRef,
		CheckedUnixNano:    now.UnixNano(),
		ExpiresUnixNano:    expires,
	}
	authorization.Digest, err = canonicalDigest("CurrentAuthorizationV1", authorization)
	if err != nil {
		return CurrentAuthorizationV1{}, err
	}
	return authorization, nil
}

func workspaceReadCurrentQueryV1(raw json.RawMessage) bool {
	var header struct {
		ContractVersion string `json:"contract_version"`
	}
	return json.Unmarshal(raw, &header) == nil && header.ContractVersion == sandboxports.WorkspaceReadCurrentContractVersionV1
}

func workspaceReadCurrentQueryV2(raw json.RawMessage) bool {
	var header struct {
		ContractVersion string `json:"contract_version"`
	}
	return json.Unmarshal(raw, &header) == nil && header.ContractVersion == sandboxports.WorkspaceReadCurrentContractVersionV2
}

func (s CurrentServer) inspectWorkspaceReadV2(ctx context.Context, request DispatchRequestV1, now time.Time) (CurrentAuthorizationV1, error) {
	if s.WorkspaceReadCurrentV2 == nil || !s.WorkspaceReadCurrentV2.PhysicalQualifiedV2() {
		return CurrentAuthorizationV1{}, errors.New("workspace read exact current v2 reader is unavailable")
	}
	var query sandboxports.WorkspaceReadCurrentQueryV2
	decoder := json.NewDecoder(bytesReader(request.RuntimeCurrentQuery))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&query); err != nil {
		return CurrentAuthorizationV1{}, fmt.Errorf("workspace read current v2 query decode: %w", err)
	}
	if err := query.ValidateCurrent(now); err != nil {
		return CurrentAuthorizationV1{}, err
	}
	var payload WorkspaceReadPayloadV1
	decoder = json.NewDecoder(bytesReader(request.Payload.ProviderPayload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return CurrentAuthorizationV1{}, fmt.Errorf("workspace read payload decode: %w", err)
	}
	current, err := s.WorkspaceReadCurrentV2.InspectWorkspaceReadCurrentV2(ctx, query)
	if err != nil {
		return CurrentAuthorizationV1{}, err
	}
	checked := s.Now()
	if checked.IsZero() || checked.Before(now) {
		return CurrentAuthorizationV1{}, errors.New("workspace read current server clock regressed")
	}
	if err := current.ValidateCurrent(checked); err != nil {
		return CurrentAuthorizationV1{}, err
	}
	base := current.Base
	provider, err := providerFromRuntime(base.ProviderBinding)
	if err != nil {
		return CurrentAuthorizationV1{}, err
	}
	phase, err := runtimePhase(request.Phase)
	if err != nil ||
		base.QueryDigest != query.Base.Digest ||
		current.Reservation.Meta.Ref() != query.Reservation ||
		current.Attempt.Meta.Ref() != query.Attempt.OwnerRef() ||
		current.Attempt.State != contract.WorkspaceReadStartedV1 ||
		current.AdmissionReceipt != query.AdmissionReceipt ||
		base.ProviderBinding != query.Base.Authorization.Provider ||
		base.ReviewAuthorization != query.Base.RuntimeInspect.ReviewAuthorization ||
		base.ExecuteEnforcement.Phase != phase ||
		base.Command != query.Base.Command ||
		base.WorkspaceView != query.Base.WorkspaceView ||
		payload.Workspace.ID != base.WorkspaceView.ID ||
		payload.Workspace.Revision != base.WorkspaceView.Revision ||
		payload.Workspace.Digest != prefixedCurrentDigestV1(base.WorkspaceView.Digest) ||
		base.FileScopeDigest != payload.FileScopeDigest ||
		base.RelativePath != payload.RelativePath ||
		provider != request.ProviderBinding {
		return CurrentAuthorizationV1{}, errors.New("workspace read exact current v2 projection drifted from the dispatch request")
	}
	actualBinding := executionBindingFromWorkspaceReadCurrentV1(base)
	actualRef := enforcementRef(base.ExecuteEnforcement, request.Phase)
	if actualBinding != request.ExecutionBinding ||
		actualRef != request.RuntimeEnforcement ||
		factRef(base.SandboxAttempt) != request.SandboxAttempt ||
		request.SandboxProjection == nil ||
		!sameSandboxProjectionV1(*request.SandboxProjection, base.SandboxProjectionRevision, base.SandboxProjectionDigest, base.SandboxProjectionExpiresUnixNano) ||
		string(base.ExecuteEnforcement.OperationDigest) != request.OperationDigest ||
		string(base.ExecuteEnforcement.EffectID) != request.EffectID ||
		base.ExecuteEnforcement.AttemptID != request.AttemptID {
		return CurrentAuthorizationV1{}, errors.New("workspace read v2 actual-point coordinates drifted")
	}
	expires := minimum(request.RequestedNotAfterUnixNano, current.ExpiresUnixNano, base.ExpiresUnixNano, base.ExecuteEnforcement.ExpiresUnixNano)
	authorization := CurrentAuthorizationV1{
		ContractVersion: ContractVersionV1, RequestDigest: request.Digest,
		OperationDigest: request.OperationDigest, EffectID: request.EffectID, AttemptID: request.AttemptID,
		Phase: request.Phase, ProviderBinding: provider,
		SandboxProjection: SandboxProjectionRefV1{
			Revision:        uint64(base.SandboxProjectionRevision),
			Digest:          string(base.SandboxProjectionDigest),
			ExpiresUnixNano: base.SandboxProjectionExpiresUnixNano,
		},
		ExecutionBinding: actualBinding, RuntimeEnforcement: actualRef,
		CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires,
	}
	authorization.Digest, err = canonicalDigest("CurrentAuthorizationV1", authorization)
	if err != nil {
		return CurrentAuthorizationV1{}, err
	}
	return authorization, nil
}

func (s CurrentServer) inspectWorkspaceReadV1(ctx context.Context, request DispatchRequestV1, now time.Time) (CurrentAuthorizationV1, error) {
	if s.WorkspaceReadCurrent == nil {
		return CurrentAuthorizationV1{}, errors.New("workspace read exact current reader is unavailable")
	}
	var query sandboxports.WorkspaceReadCurrentQueryV1
	decoder := json.NewDecoder(bytesReader(request.RuntimeCurrentQuery))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&query); err != nil {
		return CurrentAuthorizationV1{}, fmt.Errorf("workspace read current query decode: %w", err)
	}
	if err := query.ValidateCurrent(now); err != nil {
		return CurrentAuthorizationV1{}, err
	}
	var payload WorkspaceReadPayloadV1
	decoder = json.NewDecoder(bytesReader(request.Payload.ProviderPayload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return CurrentAuthorizationV1{}, fmt.Errorf("workspace read payload decode: %w", err)
	}
	current, err := s.WorkspaceReadCurrent.InspectWorkspaceReadCurrentV1(ctx, query)
	if err != nil {
		return CurrentAuthorizationV1{}, err
	}
	if err := current.ValidateCurrent(now); err != nil {
		return CurrentAuthorizationV1{}, err
	}
	provider, err := providerFromRuntime(current.ProviderBinding)
	if err != nil {
		return CurrentAuthorizationV1{}, err
	}
	phase, err := runtimePhase(request.Phase)
	if err != nil ||
		current.QueryDigest != query.Digest ||
		current.ProviderBinding != query.Authorization.Provider ||
		current.ReviewAuthorization != query.RuntimeInspect.ReviewAuthorization ||
		current.ExecuteEnforcement.Phase != phase ||
		current.Command != query.Command ||
		current.WorkspaceView != query.WorkspaceView ||
		payload.Workspace.ID != current.WorkspaceView.ID ||
		payload.Workspace.Revision != current.WorkspaceView.Revision ||
		payload.Workspace.Digest != prefixedCurrentDigestV1(current.WorkspaceView.Digest) ||
		current.FileScopeDigest != payload.FileScopeDigest ||
		current.RelativePath != payload.RelativePath ||
		provider != request.ProviderBinding {
		return CurrentAuthorizationV1{}, errors.New("workspace read exact current projection drifted from the dispatch request")
	}
	actualBinding := executionBindingFromWorkspaceReadCurrentV1(current)
	actualRef := enforcementRef(current.ExecuteEnforcement, request.Phase)
	if actualBinding != request.ExecutionBinding ||
		actualRef != request.RuntimeEnforcement ||
		factRef(current.SandboxAttempt) != request.SandboxAttempt ||
		request.SandboxProjection == nil ||
		!sameSandboxProjectionV1(*request.SandboxProjection, current.SandboxProjectionRevision, current.SandboxProjectionDigest, current.ExpiresUnixNano) ||
		string(current.ExecuteEnforcement.OperationDigest) != request.OperationDigest ||
		string(current.ExecuteEnforcement.EffectID) != request.EffectID ||
		current.ExecuteEnforcement.AttemptID != request.AttemptID {
		return CurrentAuthorizationV1{}, errors.New("workspace read actual-point coordinates drifted")
	}
	expires := minimum(request.RequestedNotAfterUnixNano, current.ExpiresUnixNano, current.ExecuteEnforcement.ExpiresUnixNano)
	authorization := CurrentAuthorizationV1{
		ContractVersion: ContractVersionV1, RequestDigest: request.Digest,
		OperationDigest: request.OperationDigest, EffectID: request.EffectID, AttemptID: request.AttemptID,
		Phase: request.Phase, ProviderBinding: provider,
		SandboxProjection: SandboxProjectionRefV1{
			Revision: uint64(current.SandboxProjectionRevision), Digest: string(current.SandboxProjectionDigest),
			ExpiresUnixNano: current.ExpiresUnixNano,
		},
		ExecutionBinding: actualBinding, RuntimeEnforcement: actualRef,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	}
	authorization.Digest, err = canonicalDigest("CurrentAuthorizationV1", authorization)
	if err != nil {
		return CurrentAuthorizationV1{}, err
	}
	return authorization, nil
}

func sameSandboxProjectionV1(actual SandboxProjectionRefV1, revision runtimecore.Revision, digest runtimecore.Digest, expires int64) bool {
	return actual.Revision == uint64(revision) &&
		actual.Digest == string(digest) &&
		actual.ExpiresUnixNano == expires
}

func prefixedCurrentDigestV1(value string) string {
	if len(value) >= len("sha256:") && value[:len("sha256:")] == "sha256:" {
		return value
	}
	return "sha256:" + value
}

func executionBindingFromWorkspaceReadCurrentV1(current sandboxports.WorkspaceReadCurrentProjectionV1) ExecutionBindingV1 {
	lease := current.RuntimeLease
	return ExecutionBindingV1{
		TenantID:   current.TenantID,
		InstanceID: string(lease.Instance.ID), InstanceEpoch: uint64(lease.Instance.Epoch),
		LeaseID: string(lease.Lease.ID), LeaseEpoch: uint64(lease.Lease.Epoch),
		FenceEpoch: uint64(lease.FenceEpoch), ScopeDigest: string(lease.ScopeDigest),
		ObservedRevision: uint64(lease.ObservedRevision), ExpiresUnixNano: lease.Ref.ExpiresUnixNano,
	}
}

func (s CurrentServer) inspectCheckpointV1(ctx context.Context, request DispatchRequestV1, now time.Time) (CurrentAuthorizationV1, error) {
	if s.CheckpointGovernance == nil {
		return CurrentAuthorizationV1{}, errors.New("checkpoint current governance is unavailable")
	}
	var query CheckpointRuntimeCurrentQueryV1
	decoder := json.NewDecoder(bytesReader(request.RuntimeCurrentQuery))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&query); err != nil {
		return CurrentAuthorizationV1{}, fmt.Errorf("checkpoint runtime current query decode: %w", err)
	}
	if err := validateCheckpointQueryV1(query, now); err != nil {
		return CurrentAuthorizationV1{}, err
	}
	current, err := s.CheckpointGovernance.InspectCurrentCheckpointRestoreDispatchV1(ctx, query.RuntimeInspect)
	if err != nil {
		return CurrentAuthorizationV1{}, err
	}
	if err := current.Validate(now); err != nil {
		return CurrentAuthorizationV1{}, err
	}
	expectedQuery, err := checkpointQueryFromCurrentV1(current, now)
	if err != nil || !reflect.DeepEqual(query, expectedQuery) {
		if err != nil {
			return CurrentAuthorizationV1{}, err
		}
		return CurrentAuthorizationV1{}, errors.New("checkpoint current query drifted from the independent reread")
	}
	provider, err := providerFromRuntime(current.Sandbox.Verifier)
	if err != nil || provider != request.ProviderBinding {
		return CurrentAuthorizationV1{}, errors.New("checkpoint provider binding drifted at actual execution point")
	}
	actualRef := enforcementRef(current.Phase, request.Phase)
	actualBinding := checkpointExecutionBindingV1(current)
	if actualRef != request.RuntimeEnforcement || actualBinding != request.ExecutionBinding || factRef(current.Sandbox.DispatchAttempt) != request.SandboxAttempt || current.Sandbox.DispatchAttempt.ID != request.AttemptID || string(current.Sandbox.OperationDigest) != request.OperationDigest || string(current.Sandbox.EffectID) != request.EffectID || string(current.Sandbox.IntentDigest) != request.IntentDigest || uint64(current.Sandbox.IntentRevision) != request.IntentRevision {
		return CurrentAuthorizationV1{}, errors.New("checkpoint actual-point enforcement coordinates drifted")
	}
	expires := minimum(request.RequestedNotAfterUnixNano, current.ExpiresUnixNano, current.Sandbox.ExpiresUnixNano, current.Phase.ExpiresUnixNano, query.ExpiresUnixNano)
	authorization := CurrentAuthorizationV1{
		ContractVersion:    ContractVersionV1,
		RequestDigest:      request.Digest,
		OperationDigest:    request.OperationDigest,
		EffectID:           request.EffectID,
		AttemptID:          request.AttemptID,
		Phase:              request.Phase,
		ProviderBinding:    provider,
		SandboxProjection:  SandboxProjectionRefV1{Revision: uint64(current.Sandbox.ProjectionRevision), Digest: string(current.Sandbox.ProjectionDigest), ExpiresUnixNano: current.Sandbox.ExpiresUnixNano},
		ExecutionBinding:   actualBinding,
		RuntimeEnforcement: actualRef,
		CheckedUnixNano:    now.UnixNano(),
		ExpiresUnixNano:    expires,
	}
	authorization.Digest, err = canonicalDigest("CurrentAuthorizationV1", authorization)
	if err != nil {
		return CurrentAuthorizationV1{}, err
	}
	return authorization, nil
}

func exactOperationID(operation runtimeports.OperationSubjectV3) string {
	switch operation.Kind {
	case runtimeports.OperationScopeActivationV3:
		return operation.ActivationAttemptID
	case runtimeports.OperationScopeRunV3:
		return string(operation.RunID)
	case runtimeports.OperationScopeTerminationV3:
		return operation.TerminationAttemptID
	case runtimeports.OperationScopeAdminV3:
		return operation.AdminOperationID
	default:
		return operation.CustomOperationID
	}
}

func minimum(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

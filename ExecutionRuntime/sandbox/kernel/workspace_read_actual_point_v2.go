package kernel

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// workspaceReadActualPointAdapterV2 is the concrete Sandbox-internal bridge
// from the Go owner kernel to the Rust Data Plane Unix IPC. It is not a public
// Runtime or product API.
type workspaceReadActualPointAdapterV2 struct {
	client dataplaneadapter.Client
}

type workspaceReadPreparedActualPointV2 struct {
	owner           *workspaceReadActualPointAdapterV2
	prepareRequest  dataplaneadapter.DispatchRequestV1
	executeRequest  dataplaneadapter.DispatchRequestV1
	stableKeyDigest string
}

func (p *workspaceReadPreparedActualPointV2) ActualRequestDigestV2() string {
	if p == nil {
		return ""
	}
	return strings.TrimPrefix(p.executeRequest.Digest, "sha256:")
}

func (p *workspaceReadPreparedActualPointV2) ActualPayloadDigestV2() string {
	if p == nil {
		return ""
	}
	return strings.TrimPrefix(p.executeRequest.PayloadDigest, "sha256:")
}

func (p *workspaceReadPreparedActualPointV2) ActualAttemptIDV2() string {
	if p == nil {
		return ""
	}
	return p.executeRequest.AttemptID
}

func (p *workspaceReadPreparedActualPointV2) ActualExpiresUnixNanoV2() int64 {
	if p == nil {
		return 0
	}
	return p.executeRequest.RequestedNotAfterUnixNano
}

func (*workspaceReadPreparedActualPointV2) workspaceReadActualPointPreparationV2() {}

func newWorkspaceReadActualPointAdapterV2(client dataplaneadapter.Client) (*workspaceReadActualPointAdapterV2, error) {
	if strings.TrimSpace(client.SocketPath) == "" {
		return nil, errors.New("workspace read Data Plane socket is required")
	}
	return &workspaceReadActualPointAdapterV2{client: client}, nil
}

func (*workspaceReadActualPointAdapterV2) workspaceReadActualPointSealV2() {}

func (a *workspaceReadActualPointAdapterV2) ReadWorkspaceFileV1(ctx context.Context, input sandboxports.WorkspaceReadActualPointRequestV1) (WorkspaceReadActualPointResultV1, error) {
	prepared, err := a.PrepareWorkspaceReadV2(ctx, input)
	if err != nil {
		return WorkspaceReadActualPointResultV1{}, err
	}
	return a.DispatchPreparedWorkspaceReadV2(ctx, prepared)
}

func (a *workspaceReadActualPointAdapterV2) PrepareWorkspaceReadV2(ctx context.Context, input sandboxports.WorkspaceReadActualPointRequestV1) (WorkspaceReadActualPointPreparationV2, error) {
	if a == nil {
		return nil, errors.New("workspace read Data Plane adapter is unavailable")
	}
	if input.Reservation.ValidateShape() != nil || input.Command.ValidateShape() != nil || input.Workspace.ValidateShape() != nil || input.RuntimeCurrent.Validate() != nil || input.CurrentQuery.Validate() != nil || input.S1CheckedUnixNano <= 0 || input.ExpiresUnixNano <= input.S1CheckedUnixNano {
		return nil, sandboxports.NewWorkspaceReadActualPointErrorV1(sandboxports.WorkspaceReadEffectNotStartedV1, errors.New("workspace read actual-point input is incomplete"))
	}
	expectedAttempt := contract.Ref{
		ID:       "workspace-read-attempt-" + trimWorkspaceReadDigestV1(input.Reservation.StableKeyDigest),
		Revision: 1,
		Digest:   input.CurrentQuery.Attempt.Digest,
	}
	if input.CurrentQuery.Base.Command != input.Command.Meta.Ref() ||
		input.CurrentQuery.Base.WorkspaceView != input.Workspace.Meta.Ref() ||
		input.CurrentQuery.Reservation != input.Reservation.Meta.Ref() ||
		input.CurrentQuery.Attempt.OwnerRef() != expectedAttempt ||
		input.CurrentQuery.Base.FileScopeDigest != input.Command.FileScopeDigest ||
		input.CurrentQuery.Base.RelativePath != input.Command.RelativePath ||
		input.CurrentQuery.Base.CheckedUnixNano != input.S1CheckedUnixNano ||
		input.CurrentQuery.Base.ExpiresUnixNano != input.ExpiresUnixNano {
		return nil, sandboxports.NewWorkspaceReadActualPointErrorV1(sandboxports.WorkspaceReadEffectNotStartedV1, errors.New("workspace read exact current query drifted from actual-point input"))
	}
	workspace := dataplaneadapter.ExactRefV1{
		ID: input.Workspace.Meta.ID, Revision: input.Workspace.Meta.Revision,
		Digest: input.Workspace.Meta.Digest, ExpiresUnixNano: input.Workspace.Meta.ExpiresUnixNano,
	}
	var expected *dataplaneadapter.ExactRefV1
	if input.Command.ExpectedFileRef != nil {
		expected = &dataplaneadapter.ExactRefV1{
			ID: input.Command.ExpectedFileRef.ID, Revision: input.Command.ExpectedFileRef.Revision,
			Digest: input.Command.ExpectedFileRef.Digest, ExpiresUnixNano: input.Workspace.Meta.ExpiresUnixNano,
		}
	}
	payload, err := dataplaneadapter.NewWorkspaceReadPayloadV1(dataplaneadapter.WorkspaceReadPayloadV1{
		WorkspaceBindingID: input.Workspace.Meta.ID,
		WorkspaceDigest:    input.Workspace.Meta.Digest,
		Workspace:          workspace,
		FileScopeDigest:    input.Command.FileScopeDigest,
		RelativePath:       input.Command.RelativePath,
		StartByte:          input.Command.StartByte,
		MaxBytes:           input.Command.MaxBytes,
		ExpectedFileRef:    expected,
		S1Checked:          true,
	})
	if err != nil {
		return nil, sandboxports.NewWorkspaceReadActualPointErrorV1(sandboxports.WorkspaceReadEffectNotStartedV1, err)
	}
	prepareCurrent, err := workspaceReadPrepareCurrentV1(input.RuntimeCurrent)
	if err != nil {
		return nil, sandboxports.NewWorkspaceReadActualPointErrorV1(sandboxports.WorkspaceReadEffectNotStartedV1, err)
	}
	prepareRequest, err := dataplaneadapter.NewDispatchRequestV1(dataplaneadapter.DispatchInput{
		RequestID:         input.Reservation.Meta.ID,
		Current:           prepareCurrent,
		EffectKind:        "praxis.sandbox/workspace-read",
		PayloadSchema:     input.Command.SourceToolPayloadSchema,
		PayloadRevision:   input.Command.SourceToolPayloadRevision,
		Payload:           payload,
		RequestedNotAfter: time.Unix(0, input.ExpiresUnixNano),
	})
	if err != nil {
		return nil, sandboxports.NewWorkspaceReadActualPointErrorV1(sandboxports.WorkspaceReadEffectNotStartedV1, err)
	}
	executeRequest, err := dataplaneadapter.NewDispatchRequestV1(dataplaneadapter.DispatchInput{
		RequestID:              input.Reservation.Meta.ID,
		Current:                input.RuntimeCurrent,
		WorkspaceReadCurrentV2: &input.CurrentQuery,
		EffectKind:             "praxis.sandbox/workspace-read",
		PayloadSchema:          input.Command.SourceToolPayloadSchema,
		PayloadRevision:        input.Command.SourceToolPayloadRevision,
		Payload:                payload,
		RequestedNotAfter:      time.Unix(0, input.ExpiresUnixNano),
	})
	if err != nil {
		return nil, sandboxports.NewWorkspaceReadActualPointErrorV1(sandboxports.WorkspaceReadEffectNotStartedV1, err)
	}
	return &workspaceReadPreparedActualPointV2{
		owner: a, prepareRequest: prepareRequest, executeRequest: executeRequest,
		stableKeyDigest: input.Reservation.StableKeyDigest,
	}, nil
}

func (a *workspaceReadActualPointAdapterV2) DispatchPreparedWorkspaceReadV2(ctx context.Context, value WorkspaceReadActualPointPreparationV2) (WorkspaceReadActualPointResultV1, error) {
	prepared, ok := value.(*workspaceReadPreparedActualPointV2)
	if !ok || prepared == nil || prepared.owner != a || a == nil {
		return WorkspaceReadActualPointResultV1{}, sandboxports.NewWorkspaceReadActualPointErrorV1(sandboxports.WorkspaceReadEffectNotStartedV1, errors.New("workspace read prepared actual-point handle is foreign"))
	}
	if err := prepared.prepareRequest.ValidateCurrent(time.Now()); err != nil {
		return WorkspaceReadActualPointResultV1{}, sandboxports.NewWorkspaceReadActualPointErrorV1(sandboxports.WorkspaceReadEffectNotStartedV1, err)
	}
	if err := prepared.executeRequest.ValidateCurrent(time.Now()); err != nil {
		return WorkspaceReadActualPointResultV1{}, sandboxports.NewWorkspaceReadActualPointErrorV1(sandboxports.WorkspaceReadEffectNotStartedV1, err)
	}
	if err := a.ensureWorkspaceReadPrepareV1(ctx, prepared.prepareRequest); err != nil {
		return WorkspaceReadActualPointResultV1{}, sandboxports.NewWorkspaceReadActualPointErrorV1(sandboxports.WorkspaceReadEffectNotStartedV1, err)
	}
	return a.dispatchSealedWorkspaceReadV1(ctx, prepared.executeRequest, prepared.stableKeyDigest)
}

func (a *workspaceReadActualPointAdapterV2) InspectWorkspaceReadJournalV2(ctx context.Context, qualification contract.WorkspaceReadExecutionQualificationV2) (WorkspaceReadActualPointInspectionV2, error) {
	if a == nil {
		return WorkspaceReadActualPointInspectionV2{}, errors.New("workspace read Data Plane adapter is unavailable")
	}
	lookup, err := contract.BuildWorkspaceReadPhysicalJournalLookupV2(qualification)
	if err != nil {
		return WorkspaceReadActualPointInspectionV2{}, err
	}
	wireLookup := dataplaneadapter.WorkspaceReadPhysicalJournalLookupV2{
		AttemptID: lookup.RuntimeAttemptID, RequestDigest: "sha256:" + lookup.RequestDigest,
		PayloadDigest: "sha256:" + lookup.PayloadDigest, Phase: lookup.Phase,
	}
	wireLookup.Digest, err = workspaceReadWireDigestV1("WorkspaceReadPhysicalJournalLookupV2", wireLookup)
	if err != nil {
		return WorkspaceReadActualPointInspectionV2{}, err
	}
	response, err := a.client.InspectWorkspaceReadJournalV2(ctx, wireLookup)
	if err != nil {
		var closed *dataplaneadapter.ClosedError
		if errors.As(err, &closed) && closed.Reason == "provider_unknown" {
			return WorkspaceReadActualPointInspectionV2{}, ErrWorkspaceReadPhysicalJournalNotFoundV2
		}
		return WorkspaceReadActualPointInspectionV2{}, err
	}
	if response.Journal == nil {
		return WorkspaceReadActualPointInspectionV2{}, errors.New("workspace read journal inspect lacks exact evidence")
	}
	journal, err := workspaceReadPhysicalJournalRefV2(*response.Journal)
	if err != nil || journal.ValidateLookupV2(lookup) != nil {
		return WorkspaceReadActualPointInspectionV2{}, errors.New("workspace read journal inspect drifted from Qualification")
	}
	evidence, err := authorizeWorkspaceReadPhysicalJournalEvidenceV2(journal)
	if err != nil {
		return WorkspaceReadActualPointInspectionV2{}, err
	}
	inspection := WorkspaceReadActualPointInspectionV2{Journal: journal, JournalEvidence: evidence}
	if response.ProviderResult == nil {
		return inspection, nil
	}
	result, err := workspaceReadActualPointResultV1(
		response.ProviderResult.Observation.WorkspaceRead,
		&response.ProviderResult.Receipt,
		journal,
		qualification.AdmissionReceipt.StableKeyDigest,
	)
	if err != nil {
		return WorkspaceReadActualPointInspectionV2{}, err
	}
	inspection.Result = &result
	return inspection, nil
}

func workspaceReadPrepareCurrentV1(current runtimeports.CurrentOperationDispatchEnforcementV4) (runtimeports.CurrentOperationDispatchEnforcementV4, error) {
	prepare, err := current.Journal.PhaseRefV4(runtimeports.OperationDispatchEnforcementPrepareV4)
	if err != nil {
		return runtimeports.CurrentOperationDispatchEnforcementV4{}, err
	}
	current.Phase = prepare
	return runtimeports.SealCurrentOperationDispatchEnforcementV4(current)
}

func (a *workspaceReadActualPointAdapterV2) ensureWorkspaceReadPrepareV1(ctx context.Context, request dataplaneadapter.DispatchRequestV1) error {
	if _, err := a.dispatchWorkspaceReadIPC(ctx, request); err == nil {
		return nil
	} else {
		recovered, inspectErr := a.client.Inspect(ctx, request)
		if inspectErr == nil && recovered.Accepted {
			return nil
		}
		return err
	}
}

func (a *workspaceReadActualPointAdapterV2) dispatchSealedWorkspaceReadV1(ctx context.Context, request dataplaneadapter.DispatchRequestV1, stableKeyDigest string) (WorkspaceReadActualPointResultV1, error) {
	response, err := a.dispatchWorkspaceReadIPC(ctx, request)
	if err != nil {
		recovered, inspectErr := a.client.Inspect(ctx, request)
		if inspectErr == nil && recovered.Accepted {
			response = recovered
		} else {
			if recovered.WorkspaceReadJournal != nil {
				journal, journalErr := workspaceReadPhysicalJournalRefV2(*recovered.WorkspaceReadJournal)
				if journalErr == nil {
					return WorkspaceReadActualPointResultV1{}, sandboxports.NewWorkspaceReadActualPointErrorWithJournalV2(sandboxports.WorkspaceReadEffectStartedUnknownV1, err, journal)
				}
			}
			var closed *dataplaneadapter.ClosedError
			if errors.As(err, &closed) && workspaceReadNoEffectEvidenceV1(closed) {
				return WorkspaceReadActualPointResultV1{}, sandboxports.NewWorkspaceReadActualPointErrorV1(sandboxports.WorkspaceReadEffectNotStartedV1, err)
			}
			return WorkspaceReadActualPointResultV1{}, sandboxports.NewWorkspaceReadActualPointErrorV1(sandboxports.WorkspaceReadEffectStartedUnknownV1, err)
		}
	}
	if response.ProviderObservation == nil || response.ProviderObservation.WorkspaceRead == nil || response.ProviderReceipt == nil || response.WorkspaceReadJournal == nil {
		return WorkspaceReadActualPointResultV1{}, errors.New("workspace read Data Plane response lacks exact observation and receipt")
	}
	journal, err := workspaceReadPhysicalJournalRefV2(*response.WorkspaceReadJournal)
	if err != nil {
		return WorkspaceReadActualPointResultV1{}, sandboxports.NewWorkspaceReadActualPointErrorV1(sandboxports.WorkspaceReadEffectStartedUnknownV1, err)
	}
	return workspaceReadActualPointResultV1(response.ProviderObservation.WorkspaceRead, response.ProviderReceipt, journal, stableKeyDigest)
}

func workspaceReadActualPointResultV1(
	observation *dataplaneadapter.WorkspaceReadObservationV1,
	receipt *dataplaneadapter.ProviderReceiptV1,
	journal contract.WorkspaceReadPhysicalJournalRefV2,
	stableKeyDigest string,
) (WorkspaceReadActualPointResultV1, error) {
	if observation == nil || receipt == nil || journal.State != contract.WorkspaceReadPhysicalJournalCompletedV2 || !contract.ValidDigest(strings.TrimPrefix(receipt.Digest, "sha256:")) || !contract.ValidDigest(strings.TrimPrefix(receipt.ObservationDigest, "sha256:")) {
		return WorkspaceReadActualPointResultV1{}, errors.New("workspace read recovered result is incomplete")
	}
	evidence, err := authorizeWorkspaceReadPhysicalJournalEvidenceV2(journal)
	if err != nil {
		return WorkspaceReadActualPointResultV1{}, err
	}
	return WorkspaceReadActualPointResultV1{
		File:              contract.Ref{ID: observation.File.ID, Revision: observation.File.Revision, Digest: observation.File.Digest},
		Content:           observation.Content,
		ContentDigest:     observation.ContentDigest,
		StartByte:         observation.StartByte,
		ReturnedBytes:     observation.ReturnedBytes,
		TotalBytes:        observation.TotalBytes,
		Complete:          observation.Complete,
		ProviderS1Checked: observation.S1Checked,
		ProviderS2Checked: observation.S2Checked,
		PhysicalReadCount: observation.PhysicalReadCount,
		ProviderReceipt: contract.WorkspaceReadReceiptBindingV1{
			ID:                "workspace-read-provider-receipt-" + trimWorkspaceReadDigestV1(receipt.Digest),
			Revision:          1,
			Digest:            strings.TrimPrefix(receipt.Digest, "sha256:"),
			ObservationDigest: strings.TrimPrefix(receipt.ObservationDigest, "sha256:"),
			StableKeyDigest:   stableKeyDigest,
			CheckedUnixNano:   receipt.RecordedUnixNano,
			ExpiresUnixNano:   receipt.ExpiresUnixNano,
		},
		Journal: journal, JournalEvidence: evidence,
	}, nil
}

func workspaceReadPhysicalJournalRefV2(value dataplaneadapter.WorkspaceReadPhysicalJournalRefV2) (contract.WorkspaceReadPhysicalJournalRefV2, error) {
	result := contract.WorkspaceReadPhysicalJournalRefV2{
		AttemptID:        value.AttemptID,
		RequestDigest:    strings.TrimPrefix(value.RequestDigest, "sha256:"),
		PayloadDigest:    strings.TrimPrefix(value.PayloadDigest, "sha256:"),
		Phase:            value.Phase,
		State:            contract.WorkspaceReadPhysicalJournalStateV2(value.State),
		Revision:         value.Revision,
		RecordedUnixNano: value.RecordedUnixNano,
		RecordDigest:     strings.TrimPrefix(value.RecordDigest, "sha256:"),
	}
	if err := result.Validate(); err != nil {
		return contract.WorkspaceReadPhysicalJournalRefV2{}, err
	}
	return result, nil
}

func workspaceReadNoEffectEvidenceV1(closed *dataplaneadapter.ClosedError) bool {
	return closed != nil &&
		closed.EffectBoundary == "effect_not_started" &&
		closed.CrossedActualPoint != nil &&
		!*closed.CrossedActualPoint &&
		closed.PhysicalReadCount != nil &&
		*closed.PhysicalReadCount == 0
}

// dispatchWorkspaceReadIPC is deliberately package-private. Public
// dataplaneadapter.Client.Dispatch rejects workspace_read, so the only Go
// callsite able to cross this physical boundary is the qualified Kernel
// executor holding a private workspaceReadActualPointAdapterV2.
func (a *workspaceReadActualPointAdapterV2) dispatchWorkspaceReadIPC(
	ctx context.Context,
	request dataplaneadapter.DispatchRequestV1,
) (dataplaneadapter.DispatchResponseV1, error) {
	if a == nil || request.Payload.ProviderKind != "workspace_read" || request.EffectKind != "praxis.sandbox/workspace-read" {
		return dataplaneadapter.DispatchResponseV1{}, errors.New("workspace read private dispatch rejected a foreign request")
	}
	if err := request.ValidateCurrent(time.Now()); err != nil {
		return dataplaneadapter.DispatchResponseV1{}, err
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", a.client.SocketPath)
	if err != nil {
		return dataplaneadapter.DispatchResponseV1{}, err
	}
	defer connection.Close()
	unix, ok := connection.(*net.UnixConn)
	if !ok {
		return dataplaneadapter.DispatchResponseV1{}, errors.New("workspace read Data Plane connection is not Unix IPC")
	}
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
		if err = unix.SetDeadline(deadline); err != nil {
			return dataplaneadapter.DispatchResponseV1{}, err
		}
	}
	if err = validateWorkspaceReadPeerUIDV2(unix, a.client.AllowedUID); err != nil {
		return dataplaneadapter.DispatchResponseV1{}, err
	}
	envelope := dataplaneadapter.DataPlaneRequestV1{
		ContractVersion: dataplaneadapter.ContractVersionV1,
		Operation:       dataplaneadapter.DataPlaneDispatchV1,
		Request:         request,
	}
	if err = writeWorkspaceReadFrameV2(unix, envelope); err != nil {
		return dataplaneadapter.DispatchResponseV1{}, err
	}
	var response dataplaneadapter.DispatchResponseV1
	if err = readWorkspaceReadFrameV2(unix, &response); err != nil {
		return dataplaneadapter.DispatchResponseV1{}, err
	}
	if err = response.Validate(request); err != nil {
		return dataplaneadapter.DispatchResponseV1{}, err
	}
	if !response.Accepted {
		return response, response.Error
	}
	return response, nil
}

const maxWorkspaceReadIPCFrameBytesV2 = 4 << 20

func writeWorkspaceReadFrameV2(writer io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(payload) == 0 || len(payload) > maxWorkspaceReadIPCFrameBytesV2 {
		return errors.New("workspace read IPC frame is outside the closed bounds")
	}
	if err = binary.Write(writer, binary.BigEndian, uint32(len(payload))); err != nil {
		return err
	}
	_, err = writer.Write(payload)
	return err
}

func readWorkspaceReadFrameV2(reader io.Reader, value any) error {
	var length uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return err
	}
	if length == 0 || length > maxWorkspaceReadIPCFrameBytesV2 {
		return errors.New("workspace read IPC frame is outside the closed bounds")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("workspace read IPC frame contains trailing JSON")
	}
	return nil
}

func validateWorkspaceReadPeerUIDV2(connection *net.UnixConn, expected uint32) error {
	raw, err := connection.SyscallConn()
	if err != nil {
		return err
	}
	var credentials *syscall.Ucred
	var socketErr error
	if err = raw.Control(func(fd uintptr) {
		credentials, socketErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return err
	}
	if socketErr != nil {
		return socketErr
	}
	if credentials == nil || credentials.Uid != expected {
		return fmt.Errorf("workspace read IPC peer uid is unauthorized")
	}
	return nil
}

func workspaceReadWireDigestV1(kind string, value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(dataplaneadapter.ContractVersionV1))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(kind))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(payload)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func trimWorkspaceReadDigestV1(value string) string {
	return strings.TrimPrefix(value, "sha256:")
}

var _ WorkspaceReadActualPointV1 = (*workspaceReadActualPointAdapterV2)(nil)
var _ WorkspaceReadActualPointV2 = (*workspaceReadActualPointAdapterV2)(nil)

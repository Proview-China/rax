package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/ports"
)

type ClockV1 func() time.Time

type ConfigV1 struct {
	Clock          ClockV1
	ResultTTL      time.Duration
	Journal        ports.CommandJournalV1
	Inspect        ports.AgentRunInspectOwnerAdapterV1
	InspectAttempt ports.OriginalAttemptOwnerAdapterV1
	Events         ports.AgentRunEventStreamV1
	Cancel         ports.CommandOwnerAdapterV1
	Stop           ports.CommandOwnerAdapterV1
}

type ServiceV1 struct {
	clock          ClockV1
	resultTTL      time.Duration
	journal        ports.CommandJournalV1
	inspect        ports.AgentRunInspectOwnerAdapterV1
	inspectAttempt ports.OriginalAttemptOwnerAdapterV1
	events         ports.AgentRunEventStreamV1
	cancel         ports.CommandOwnerAdapterV1
	stop           ports.CommandOwnerAdapterV1
	commandMu      sync.Mutex
}

var _ ports.AgentRunServiceV1 = (*ServiceV1)(nil)

func NewV1(config ConfigV1) (*ServiceV1, error) {
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.ResultTTL <= 0 {
		config.ResultTTL = time.Second
	}
	if config.Journal == nil {
		return nil, contract.NewError(contract.FaultInvalidArgumentV1, "command_journal_missing", "AgentRunService requires a command journal")
	}
	return &ServiceV1{
		clock: config.Clock, resultTTL: config.ResultTTL, journal: config.Journal,
		inspect: config.Inspect, inspectAttempt: config.InspectAttempt, events: config.Events,
		cancel: config.Cancel, stop: config.Stop,
	}, nil
}

func (s *ServiceV1) nowV1() contract.WireUnixNanoV1 {
	return contract.NewWireUnixNanoV1(s.clock().UTC().UnixNano())
}

func (s *ServiceV1) windowV1(notAfter contract.WireUnixNanoV1) contract.WireValidityWindowV1 {
	now := s.clock().UTC().UnixNano()
	expiry := now + s.resultTTL.Nanoseconds()
	limit, _ := notAfter.Int64V1()
	if expiry > limit {
		expiry = limit
	}
	return contract.WireValidityWindowV1{
		CheckedUnixNano: contract.NewWireUnixNanoV1(now),
		ExpiresUnixNano: contract.NewWireUnixNanoV1(expiry),
	}
}

func capabilityErrorV1(reason, message string) error {
	return contract.NewError(contract.FaultCapabilityUnavailableV1, reason, message)
}

func (s *ServiceV1) capabilitiesV1() []contract.CapabilityV1 {
	result := make([]contract.CapabilityV1, 0, 5)
	if s.inspect != nil {
		result = append(result, contract.CapabilityAgentRunInspectV1)
	}
	if s.events != nil {
		result = append(result, contract.CapabilityAgentRunWatchV1)
	}
	if s.cancel != nil {
		result = append(result, contract.CapabilityAgentRunCancelV1)
	}
	if s.stop != nil {
		result = append(result, contract.CapabilityAgentHostStopV1)
	}
	if s.journal != nil || s.inspectAttempt != nil {
		result = append(result, contract.CapabilityCommandInspectOriginalV1)
	}
	return result
}

func (s *ServiceV1) NegotiateV1(_ context.Context, request contract.NegotiationRequestV1) (contract.NegotiationResultV1, error) {
	now := s.nowV1()
	if err := request.ValidateCurrentV1(now); err != nil {
		return contract.NegotiationResultV1{}, err
	}
	granted := make([]contract.CapabilityV1, 0, len(request.RequiredCapabilities)+len(request.OptionalCapabilities))
	available := map[contract.CapabilityV1]bool{}
	for _, capability := range s.capabilitiesV1() {
		available[capability] = true
	}
	versionSupported := false
	for _, version := range request.SupportedVersions {
		if version == contract.AgentRunServiceContractVersionV1 {
			versionSupported = true
			break
		}
	}
	missing := false
	for _, capability := range request.RequiredCapabilities {
		if !available[capability] {
			missing = true
		} else {
			granted = append(granted, capability)
		}
	}
	for _, capability := range request.OptionalCapabilities {
		if available[capability] {
			granted = append(granted, capability)
		}
	}
	result := contract.NegotiationResultV1{
		RequestDigest: request.RequestDigest, TraceID: request.TraceID,
		GrantedCapabilities: granted, Window: s.windowV1(request.NotAfterUnixNano),
	}
	if !versionSupported || missing {
		result.Disposition = contract.NegotiationCapabilityUnavailableV1
		result.GrantedCapabilities = nil
		result.Fault = &contract.FaultV1{
			Code: contract.FaultCapabilityUnavailableV1, Reason: "required_capability_unavailable",
			Message: "requested protocol version or required capability is unavailable",
			TraceID: request.TraceID, RetryDirective: contract.RetryNoneV1,
		}
	} else {
		result.Disposition = contract.NegotiationSelectedV1
		result.SelectedVersion = contract.AgentRunServiceContractVersionV1
	}
	sealed, err := contract.SealNegotiationResultV1(result)
	if err != nil {
		return contract.NegotiationResultV1{}, err
	}
	return sealed, sealed.ValidateFor(request)
}

func (s *ServiceV1) InspectAgentRunV1(ctx context.Context, request contract.AgentRunInspectRequestV1) (contract.AgentRunInspectResultV1, error) {
	if s.inspect == nil {
		return contract.AgentRunInspectResultV1{}, capabilityErrorV1("agent_run_inspect_unavailable", "Agent Run Inspect owner adapter is unavailable")
	}
	if err := request.ValidateCurrentV1(s.nowV1()); err != nil {
		return contract.AgentRunInspectResultV1{}, err
	}
	result, err := s.inspect.InspectAgentRunV1(ctx, request)
	if err != nil {
		return contract.AgentRunInspectResultV1{}, err
	}
	return result, result.ValidateFor(request)
}

func (s *ServiceV1) WatchAgentRunV1(ctx context.Context, request contract.AgentRunWatchRequestV1) (contract.AgentRunWatchResultV1, error) {
	if s.events == nil {
		return contract.AgentRunWatchResultV1{}, capabilityErrorV1("agent_run_watch_unavailable", "Agent Run event stream is unavailable")
	}
	if err := request.ValidateCurrentV1(s.nowV1()); err != nil {
		return contract.AgentRunWatchResultV1{}, err
	}
	result, err := s.events.WatchAgentRunV1(ctx, request)
	if err != nil {
		return contract.AgentRunWatchResultV1{}, err
	}
	return result, result.ValidateFor(request)
}

func (s *ServiceV1) InspectOriginalV1(ctx context.Context, request contract.InspectOriginalRequestV1) (contract.InspectOriginalResultV1, error) {
	if err := request.ValidateCurrentV1(s.nowV1()); err != nil {
		return contract.InspectOriginalResultV1{}, err
	}
	if request.OriginalAttemptRef != nil {
		if s.inspectAttempt == nil {
			return contract.InspectOriginalResultV1{}, capabilityErrorV1("attempt_inspect_unavailable", "original attempt owner adapter is unavailable")
		}
		result, err := s.inspectAttempt.InspectOriginalAttemptV1(ctx, request)
		if err != nil {
			return contract.InspectOriginalResultV1{}, err
		}
		return result, result.ValidateFor(request)
	}
	entry, err := s.journal.InspectCommandV1(ctx, request)
	if err != nil {
		var fault *contract.Error
		if errors.As(err, &fault) && fault.Code == contract.FaultNotFoundV1 {
			result, sealErr := contract.SealInspectOriginalResultV1(contract.InspectOriginalResultV1{
				RequestDigest: request.RequestDigest, TraceID: request.TraceID,
				Disposition: contract.InspectOriginalNotFoundV1,
				Fault: &contract.FaultV1{
					Code: contract.FaultNotFoundV1, Reason: "original_command_not_found",
					Message: "original command was not found", TraceID: request.TraceID,
					RetryDirective: contract.RetryNoneV1,
				},
				Window: s.windowV1(request.NotAfterUnixNano),
			})
			if sealErr != nil {
				return contract.InspectOriginalResultV1{}, sealErr
			}
			return result, result.ValidateFor(request)
		}
		return contract.InspectOriginalResultV1{}, err
	}
	if entry.Receipt == nil {
		commandRef, _ := entry.Command.CommandRefV1()
		result, sealErr := contract.SealInspectOriginalResultV1(contract.InspectOriginalResultV1{
			RequestDigest: request.RequestDigest, TraceID: request.TraceID,
			Disposition: contract.InspectOriginalIndeterminateV1,
			Fault: &contract.FaultV1{
				Code: contract.FaultUnknownOutcomeV1, Reason: "command_receipt_pending",
				Message:    "original command has no durable receipt; inspect the same original command",
				CommandRef: &commandRef, TraceID: request.TraceID, RetryDirective: contract.RetryInspectV1,
			},
			Window: s.windowV1(request.NotAfterUnixNano),
		})
		if sealErr != nil {
			return contract.InspectOriginalResultV1{}, sealErr
		}
		return result, result.ValidateFor(request)
	}
	result, err := contract.SealInspectOriginalResultV1(contract.InspectOriginalResultV1{
		RequestDigest: request.RequestDigest, TraceID: request.TraceID,
		Disposition: contract.InspectOriginalObservedV1, CommandReceipt: entry.Receipt,
		CurrentRef: entry.Receipt.CurrentRef, Window: s.windowV1(request.NotAfterUnixNano),
	})
	if err != nil {
		return contract.InspectOriginalResultV1{}, err
	}
	return result, result.ValidateFor(request)
}

func (s *ServiceV1) CancelAgentRunV1(ctx context.Context, request contract.CancelAgentRunRequestV1) (contract.CommandResultV1, error) {
	if s.cancel == nil {
		return contract.CommandResultV1{}, capabilityErrorV1("agent_run_cancel_unavailable", "Agent Run Cancel owner adapter is unavailable")
	}
	return s.executeCommandV1(ctx, request.Command, s.cancel)
}

func (s *ServiceV1) StopAgentHostV1(ctx context.Context, request contract.StopAgentHostRequestV1) (contract.CommandResultV1, error) {
	if s.stop == nil {
		return contract.CommandResultV1{}, capabilityErrorV1("agent_host_stop_unavailable", "Agent Host Stop owner adapter is unavailable")
	}
	return s.executeCommandV1(ctx, request.Command, s.stop)
}

func (s *ServiceV1) executeCommandV1(ctx context.Context, command contract.AgentRunCommandEnvelopeV1, adapter ports.CommandOwnerAdapterV1) (contract.CommandResultV1, error) {
	if err := command.ValidateCurrentV1(s.nowV1()); err != nil {
		return contract.CommandResultV1{}, err
	}
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	disposition, entry, err := s.journal.ReserveCommandV1(ctx, command)
	if err != nil {
		return contract.CommandResultV1{}, err
	}
	if disposition == ports.CommandJournalReplayV1 {
		if entry.Receipt == nil {
			return s.indeterminateCommandResultV1(entry.Command, "command_receipt_pending", "original command is reserved without a durable receipt")
		}
		return s.commandResultV1(entry.Command, *entry.Receipt)
	}
	receipt, invokeErr := invokeCommandAdapterV1(ctx, adapter, command)
	if invokeErr != nil {
		receipt, err = s.indeterminateReceiptV1(command, "owner_receipt_unavailable", "owner invocation produced no trustworthy receipt")
		if err != nil {
			return contract.CommandResultV1{}, err
		}
	}
	if err := receipt.ValidateFor(command); err != nil {
		receipt, err = s.indeterminateReceiptV1(command, "owner_receipt_invalid", "owner invocation returned an invalid receipt")
		if err != nil {
			return contract.CommandResultV1{}, err
		}
	}
	if err := s.journal.RecordReceiptV1(ctx, command, receipt); err != nil {
		recovered, inspectErr := s.journal.InspectReservedCommandV1(context.Background(), command)
		if inspectErr == nil && recovered.Receipt != nil {
			return s.commandResultV1(recovered.Command, *recovered.Receipt)
		}
		return s.indeterminateCommandResultV1(command, "receipt_record_unknown", "command receipt persistence outcome is unknown")
	}
	return s.commandResultV1(command, receipt)
}

func invokeCommandAdapterV1(ctx context.Context, adapter ports.CommandOwnerAdapterV1, command contract.AgentRunCommandEnvelopeV1) (receipt contract.AgentRunCommandReceiptV1, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("owner adapter panic: %v", recovered)
		}
	}()
	return adapter.ExecuteCommandV1(ctx, command)
}

func (s *ServiceV1) indeterminateReceiptV1(command contract.AgentRunCommandEnvelopeV1, reason, message string) (contract.AgentRunCommandReceiptV1, error) {
	commandRef, err := command.CommandRefV1()
	if err != nil {
		return contract.AgentRunCommandReceiptV1{}, err
	}
	fault := &contract.FaultV1{
		Code: contract.FaultUnknownOutcomeV1, Reason: reason, Message: message,
		CommandRef: &commandRef, TraceID: command.TraceID, RetryDirective: contract.RetryInspectV1,
	}
	return contract.SealAgentRunCommandReceiptV1(contract.AgentRunCommandReceiptV1{
		CommandRef: commandRef, IdempotencyKey: command.IdempotencyKey,
		CanonicalPayloadDigest: command.CanonicalPayloadDigest,
		OriginalRequestDigest:  command.RequestDigest,
		Status:                 contract.AgentRunCommandIndeterminateV1, Fault: fault, TraceID: command.TraceID,
		RecordedUnixNano: s.nowV1(),
	})
}

func (s *ServiceV1) indeterminateCommandResultV1(command contract.AgentRunCommandEnvelopeV1, reason, message string) (contract.CommandResultV1, error) {
	receipt, err := s.indeterminateReceiptV1(command, reason, message)
	if err != nil {
		return contract.CommandResultV1{}, err
	}
	return s.commandResultV1(command, receipt)
}

func (s *ServiceV1) commandResultV1(original contract.AgentRunCommandEnvelopeV1, receipt contract.AgentRunCommandReceiptV1) (contract.CommandResultV1, error) {
	result, err := contract.SealCommandResultV1(contract.CommandResultV1{
		RequestDigest: original.RequestDigest, TraceID: original.TraceID, Receipt: receipt,
		Window: s.windowV1(original.NotAfterUnixNano),
	})
	if err != nil {
		return contract.CommandResultV1{}, err
	}
	return result, result.ValidateFor(original)
}

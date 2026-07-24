// Package agentdefinition exposes the owner service used by CLI/API/Host glue.
// It validates approval currentness before sealing and performs create-once
// recovery by inspecting the original immutable revision.
package agentdefinition

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/decoder"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ServiceV1 struct {
	repository ports.DefinitionRepositoryV1
	approvals  ports.ApprovalCurrentReaderV1
	catalog    contract.ValidationCatalogV1
	clock      func() time.Time
}

func NewServiceV1(repository ports.DefinitionRepositoryV1, approvals ports.ApprovalCurrentReaderV1, catalog contract.ValidationCatalogV1, clock func() time.Time) (*ServiceV1, error) {
	if isNilV1(repository) || isNilV1(approvals) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "definition repository, approval current reader, and clock are required")
	}
	return &ServiceV1{repository: repository, approvals: approvals, catalog: contract.CloneValidationCatalogV1(catalog), clock: clock}, nil
}

func (s *ServiceV1) CreateYAMLV1(ctx context.Context, payload []byte) (ports.CreateDefinitionResultV1, error) {
	if err := s.validate(); err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	source, err := decoder.DecodeYAMLV1(payload, s.catalog)
	if err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	return s.CreateSourceV1(ctx, source)
}

// CreateJSONV1 is the strict JSON twin of CreateYAMLV1. Both paths converge on
// the same immutable source, approval checks, and create-once repository flow.
func (s *ServiceV1) CreateJSONV1(ctx context.Context, payload []byte) (ports.CreateDefinitionResultV1, error) {
	if err := s.validate(); err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	source, err := decoder.DecodeJSONV1(payload, s.catalog)
	if err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	return s.CreateSourceV1(ctx, source)
}

func (s *ServiceV1) CreateSourceV1(ctx context.Context, source contract.AgentDefinitionSourceV1) (ports.CreateDefinitionResultV1, error) {
	if err := s.validate(); err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	if ctx == nil || ctx.Err() != nil {
		return ports.CreateDefinitionResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "definition create context is nil or canceled")
	}
	source = contract.NormalizeSourceV1(source)
	sourceDigest, err := contract.SourceDigestV1(source, s.catalog)
	if err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	clock := requestClockCursorV1{read: s.clock}
	if existing, inspectErr := s.repository.InspectDefinitionRevisionV1(ctx, source.DefinitionID, source.Revision); inspectErr == nil {
		if existing.SourceDigest != sourceDigest {
			return ports.CreateDefinitionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "definition revision already binds different source content")
		}
		observed, clockErr := clock.observe()
		if clockErr != nil {
			return ports.CreateDefinitionResultV1{}, clockErr
		}
		current, currentErr := s.repository.InspectCurrentDefinitionV1(ctx, source.DefinitionID, observed.UnixNano())
		if currentErr != nil {
			return ports.CreateDefinitionResultV1{}, currentErr
		}
		if err := validateObservedCurrentV1(current, observed); err != nil {
			return ports.CreateDefinitionResultV1{}, err
		}
		return ports.CreateDefinitionResultV1{Definition: existing, Current: current}, nil
	} else if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return ports.CreateDefinitionResultV1{}, inspectErr
	}

	now1, err := clock.observe()
	if err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	approvalS1, err := s.approvals.InspectApprovalCurrentV1(ctx, source.ApprovalRef)
	if err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	if err := approvalS1.Validate(source.ApprovalRef, now1.UnixNano()); err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	approvalS2, err := s.approvals.InspectApprovalCurrentV1(ctx, source.ApprovalRef)
	if err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	if approvalS2 != approvalS1 {
		return ports.CreateDefinitionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "definition approval changed between S1 and S2")
	}
	now2, err := clock.observe()
	if err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	if err := approvalS2.Validate(source.ApprovalRef, now2.UnixNano()); err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	definition, err := contract.SealDefinitionV1(source, s.catalog, now2.UnixNano())
	if err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	beforeWrite, err := clock.observe()
	if err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	if err := approvalS2.Validate(source.ApprovalRef, beforeWrite.UnixNano()); err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	if !beforeWrite.Before(time.Unix(0, source.EffectiveWindow.NotAfterUnixNano)) {
		return ports.CreateDefinitionResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "definition effective window expired before publication")
	}
	result, createErr := s.repository.CreateDefinitionV1(ctx, ports.CreateDefinitionRequestV1{Definition: definition, ExpectedCurrentRevision: core.Revision(source.Revision - 1)})
	if createErr == nil {
		if err := validateCreateResultV1(result, definition, s.catalog); err != nil {
			return ports.CreateDefinitionResultV1{}, err
		}
		return result, nil
	}
	if !recoverableCreateErrorV1(createErr) {
		return ports.CreateDefinitionResultV1{}, createErr
	}
	recovered, inspectErr := s.repository.InspectDefinitionRevisionV1(context.WithoutCancel(ctx), source.DefinitionID, source.Revision)
	if inspectErr != nil {
		return ports.CreateDefinitionResultV1{}, createErr
	}
	if recovered.SourceDigest != sourceDigest {
		return ports.CreateDefinitionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "concurrent definition revision bound different source content")
	}
	recoveryNow, clockErr := clock.observe()
	if clockErr != nil {
		return ports.CreateDefinitionResultV1{}, clockErr
	}
	current, currentErr := s.repository.InspectCurrentDefinitionV1(context.WithoutCancel(ctx), source.DefinitionID, recoveryNow.UnixNano())
	if currentErr != nil {
		return ports.CreateDefinitionResultV1{}, currentErr
	}
	if err := validateObservedCurrentV1(current, recoveryNow); err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	return ports.CreateDefinitionResultV1{Definition: recovered, Current: current}, nil
}

type requestClockCursorV1 struct {
	read func() time.Time
	last time.Time
}

func (c *requestClockCursorV1) observe() (time.Time, error) {
	now := c.read()
	if now.IsZero() || (!c.last.IsZero() && now.Before(c.last)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "definition owner clock regressed during publication")
	}
	c.last = now
	return now, nil
}

func validateObservedCurrentV1(current contract.DefinitionCurrentV1, observed time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if current.CheckedUnixNano != observed.UnixNano() {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "definition repository returned a current projection for another observation time")
	}
	return nil
}

func validateCreateResultV1(result ports.CreateDefinitionResultV1, expected contract.AgentDefinitionV1, catalog contract.ValidationCatalogV1) error {
	if err := result.Definition.Validate(catalog); err != nil {
		return err
	}
	if result.Definition.RefV1() != expected.RefV1() || result.Definition.SourceDigest != expected.SourceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "definition repository returned different immutable content")
	}
	if err := result.Current.Validate(); err != nil {
		return err
	}
	return nil
}

func (s *ServiceV1) validate() error {
	if s == nil || isNilV1(s.repository) || isNilV1(s.approvals) || s.clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "definition service is not initialized")
	}
	return nil
}

func isNilV1(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}

func recoverableCreateErrorV1(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate) || core.HasCategory(err, core.ErrorConflict)
}

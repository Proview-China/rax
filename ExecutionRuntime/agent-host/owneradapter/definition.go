package owneradapter

import (
	"context"
	"time"

	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	definitionports "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports"
	hostcontract "github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

type DefinitionAdapterV1 struct {
	current definitionports.DefinitionCurrentReaderV1
	sources hostports.DefinitionSourceCurrentReaderV1
	catalog definitioncontract.ValidationCatalogV1
	clock   func() time.Time
}

func NewDefinitionAdapterV1(current definitionports.DefinitionCurrentReaderV1, sources hostports.DefinitionSourceCurrentReaderV1, catalog definitioncontract.ValidationCatalogV1, clock func() time.Time) *DefinitionAdapterV1 {
	return &DefinitionAdapterV1{current: current, sources: sources, catalog: cloneValidationCatalogV1(catalog), clock: clock}
}

func (a *DefinitionAdapterV1) DecodeDefinitionV1(ctx context.Context, config hostcontract.HostConfigV1) (hostcontract.DecodedDefinitionV1, error) {
	if a == nil {
		return hostcontract.DecodedDefinitionV1{}, hostcontract.NewError(hostcontract.ErrorUnavailable, "definition_adapter_unavailable", "definition adapter is unavailable")
	}
	if ctx == nil {
		return hostcontract.DecodedDefinitionV1{}, hostcontract.NewError(hostcontract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := config.Validate(); err != nil {
		return hostcontract.DecodedDefinitionV1{}, err
	}
	if err := unavailableV1(a.current, "definition_reader_unavailable"); err != nil {
		return hostcontract.DecodedDefinitionV1{}, err
	}
	if err := unavailableV1(a.sources, "definition_source_reader_unavailable"); err != nil {
		return hostcontract.DecodedDefinitionV1{}, err
	}
	now1, err := nowUnixNanoV1(a.clock)
	if err != nil {
		return hostcontract.DecodedDefinitionV1{}, err
	}
	s1, err := a.sources.InspectDefinitionSourceCurrentV1(ctx, config.DefinitionSourceRef)
	if err != nil {
		return hostcontract.DecodedDefinitionV1{}, ownerErrorV1(err, "definition_current_s1_failed")
	}
	if err := validateDefinitionSourceV1(s1, config.DefinitionSourceRef, now1); err != nil {
		return hostcontract.DecodedDefinitionV1{}, err
	}
	definitionRef, err := ownerDefinitionRefV1(s1.DefinitionExactRef)
	if err != nil {
		return hostcontract.DecodedDefinitionV1{}, err
	}
	ownerCurrent1, err := a.current.InspectCurrentDefinitionV1(ctx, definitionRef.DefinitionID, now1)
	if err != nil {
		return hostcontract.DecodedDefinitionV1{}, ownerErrorV1(err, "definition_owner_current_s1_failed")
	}
	if err := validateDefinitionCurrentV1(ownerCurrent1); err != nil {
		return hostcontract.DecodedDefinitionV1{}, err
	}
	if ownerCurrent1.Definition != definitionRef {
		return hostcontract.DecodedDefinitionV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "definition_owner_current_mismatch", "definition source does not map to the active owner current revision")
	}
	definition, err := a.current.InspectExactDefinitionV1(ctx, definitionRef)
	if err != nil {
		return hostcontract.DecodedDefinitionV1{}, ownerErrorV1(err, "definition_exact_failed")
	}
	if definition.RefV1() != definitionRef {
		return hostcontract.DecodedDefinitionV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "definition_exact_splice", "definition current and exact result differ")
	}
	if err := definition.Validate(cloneValidationCatalogV1(a.catalog)); err != nil {
		return hostcontract.DecodedDefinitionV1{}, ownerErrorV1(err, "definition_invalid")
	}
	now2, err := nowUnixNanoV1(a.clock)
	if err != nil {
		return hostcontract.DecodedDefinitionV1{}, err
	}
	if now2 < now1 || now2 < definition.EffectiveWindow.NotBeforeUnixNano || now2 >= definition.EffectiveWindow.NotAfterUnixNano {
		return hostcontract.DecodedDefinitionV1{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "definition_not_effective", "definition is not effective at the second observation")
	}
	s2, err := a.sources.InspectDefinitionSourceCurrentV1(ctx, config.DefinitionSourceRef)
	if err != nil {
		return hostcontract.DecodedDefinitionV1{}, ownerErrorV1(err, "definition_current_s2_failed")
	}
	if err := validateDefinitionSourceV1(s2, config.DefinitionSourceRef, now2); err != nil {
		return hostcontract.DecodedDefinitionV1{}, err
	}
	if s1 != s2 {
		return hostcontract.DecodedDefinitionV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "definition_current_drift", "definition current projection changed during exact inspection")
	}
	ownerCurrent2, err := a.current.InspectCurrentDefinitionV1(ctx, definitionRef.DefinitionID, now2)
	if err != nil {
		return hostcontract.DecodedDefinitionV1{}, ownerErrorV1(err, "definition_owner_current_s2_failed")
	}
	if err := validateDefinitionCurrentV1(ownerCurrent2); err != nil {
		return hostcontract.DecodedDefinitionV1{}, err
	}
	if ownerCurrent1 != ownerCurrent2 || ownerCurrent2.Definition != definitionRef {
		return hostcontract.DecodedDefinitionV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "definition_owner_current_drift", "definition owner current projection changed during exact inspection")
	}
	finalNow, err := nowUnixNanoV1(a.clock)
	if err != nil {
		return hostcontract.DecodedDefinitionV1{}, err
	}
	if finalNow < now2 || finalNow < definition.EffectiveWindow.NotBeforeUnixNano || finalNow >= definition.EffectiveWindow.NotAfterUnixNano {
		return hostcontract.DecodedDefinitionV1{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "definition_not_effective", "definition is not effective after the second observation")
	}
	if err := validateDefinitionSourceV1(s2, config.DefinitionSourceRef, finalNow); err != nil {
		return hostcontract.DecodedDefinitionV1{}, err
	}
	return hostcontract.DecodedDefinitionV1{Ref: definitionRefV1(definition.RefV1())}, nil
}

func validateDefinitionSourceV1(value hostcontract.DefinitionSourceCurrentV1, stableID string, now int64) error {
	if err := value.Validate(now); err != nil {
		return err
	}
	if value.SourceStableID != stableID || value.DefinitionExactRef.Kind != DefinitionKindV1 {
		return hostcontract.NewError(hostcontract.ErrorConflict, "definition_source_alias_drift", "definition source current projection differs from configured identity")
	}
	return nil
}

func validateDefinitionCurrentV1(value definitioncontract.DefinitionCurrentV1) error {
	if err := value.Validate(); err != nil {
		return ownerErrorV1(err, "definition_current_invalid")
	}
	if value.State != definitioncontract.DefinitionCurrentActiveV1 {
		return hostcontract.NewError(hostcontract.ErrorPrecondition, "definition_not_active", "definition current projection is not active")
	}
	return nil
}

func cloneValidationCatalogV1(value definitioncontract.ValidationCatalogV1) definitioncontract.ValidationCatalogV1 {
	value.Kinds = append([]string(nil), value.Kinds...)
	value.Capabilities = append([]string(nil), value.Capabilities...)
	value.RegisteredExtensionKeys = append([]string(nil), value.RegisteredExtensionKeys...)
	return value
}

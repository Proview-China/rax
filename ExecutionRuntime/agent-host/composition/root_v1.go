package composition

import (
	"context"
	"reflect"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	definitiondecoder "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/decoder"
	definitionports "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type DefinitionFormatV1 string

const (
	DefinitionFormatJSONV1 DefinitionFormatV1 = "json"
	DefinitionFormatYAMLV1 DefinitionFormatV1 = "yaml"
)

type DeclarativeRootConfigV1 struct {
	DefinitionCatalog definitioncontract.ValidationCatalogV1
	Definitions       hostports.AgentDefinitionPublicationOwnerV1
	DefinitionSources hostports.DefinitionSourcePublicationOwnerV1
	Assembler         hostports.DeclarativeAssemblyOperationV1
	Deployments       hostports.HostDeploymentCurrentReaderV1
	Host              hostports.HostV3
	Clock             func() time.Time
}

type DeclarativeRootV1 struct{ config DeclarativeRootConfigV1 }

func NewDeclarativeRootV1(config DeclarativeRootConfigV1) (*DeclarativeRootV1, error) {
	for _, dependency := range []any{config.Definitions, config.DefinitionSources, config.Assembler, config.Deployments, config.Host} {
		if isNilRootV1(dependency) {
			return nil, contract.NewError(contract.ErrorInvalidArgument, "declarative_root_dependency_missing", "declarative root requires every typed Owner dependency")
		}
	}
	if config.Clock == nil {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "declarative_root_clock_missing", "declarative root requires an injected Owner clock")
	}
	config.DefinitionCatalog = definitioncontract.CloneValidationCatalogV1(config.DefinitionCatalog)
	return &DeclarativeRootV1{config: config}, nil
}

type ValidateDefinitionRequestV1 struct {
	Format  DefinitionFormatV1
	Payload []byte
}

type ValidateDefinitionResultV1 struct {
	Source       definitioncontract.AgentDefinitionSourceV1
	SourceDigest core.Digest
}

func (r *DeclarativeRootV1) ValidateDefinitionV1(ctx context.Context, request ValidateDefinitionRequestV1) (ValidateDefinitionResultV1, error) {
	if r == nil || ctx == nil || ctx.Err() != nil {
		return ValidateDefinitionResultV1{}, contract.NewError(contract.ErrorInvalidArgument, "declarative_validate_context_invalid", "declarative validation requires a live root and context")
	}
	source, err := decodeDefinitionV1(request.Format, request.Payload, r.config.DefinitionCatalog)
	if err != nil {
		return ValidateDefinitionResultV1{}, err
	}
	digest, err := definitioncontract.SourceDigestV1(source, r.config.DefinitionCatalog)
	if err != nil {
		return ValidateDefinitionResultV1{}, err
	}
	return ValidateDefinitionResultV1{Source: source, SourceDigest: digest}, nil
}

type AssembleDefinitionRequestV1 struct {
	Bootstrap         contract.HostBootstrapConfigV1
	DeploymentCurrent contract.HostDeploymentCurrentRefV1
	DefinitionSource  contract.DefinitionSourceCurrentV1
}

type AssembleDefinitionResultV1 struct {
	Plan assemblercontract.ResolvedAgentPlanV1
}

func (r *DeclarativeRootV1) AssembleDefinitionV1(ctx context.Context, request AssembleDefinitionRequestV1) (AssembleDefinitionResultV1, error) {
	if r == nil || ctx == nil || ctx.Err() != nil {
		return AssembleDefinitionResultV1{}, contract.NewError(contract.ErrorInvalidArgument, "declarative_assemble_context_invalid", "declarative assembly requires a live root and context")
	}
	cursor := rootClockCursorV1{read: r.config.Clock}
	now1, err := cursor.observe()
	if err != nil {
		return AssembleDefinitionResultV1{}, err
	}
	deployment1, err := r.inspectDeploymentV1(ctx, request.Bootstrap, request.DeploymentCurrent, now1)
	if err != nil {
		return AssembleDefinitionResultV1{}, err
	}
	if err = request.DefinitionSource.Validate(now1.UnixNano()); err != nil {
		return AssembleDefinitionResultV1{}, err
	}
	if request.DefinitionSource.SourceStableID != request.Bootstrap.DefinitionSourceBindingID {
		return AssembleDefinitionResultV1{}, contract.NewError(contract.ErrorConflict, "declarative_assemble_source_drift", "Definition source does not match Bootstrap")
	}
	hostConfig := hostConfigFromBootstrapV1(request.Bootstrap)
	plan, err := r.config.Assembler.StartOrInspectDeclarativeAssemblyV1(ctx, hostConfig, request.DefinitionSource)
	if err != nil {
		return AssembleDefinitionResultV1{}, err
	}
	if err = plan.Validate(); err != nil {
		return AssembleDefinitionResultV1{}, err
	}
	if plan.DefinitionRef.DefinitionID != request.DefinitionSource.DefinitionExactRef.ID || uint64(plan.DefinitionRef.Revision) != request.DefinitionSource.DefinitionExactRef.Revision || plan.DefinitionRef.Digest != core.Digest(request.DefinitionSource.DefinitionExactRef.Digest) {
		return AssembleDefinitionResultV1{}, contract.NewError(contract.ErrorConflict, "declarative_assemble_plan_splice", "Resolved Plan does not bind the exact Definition source")
	}
	now2, err := cursor.observe()
	if err != nil {
		return AssembleDefinitionResultV1{}, err
	}
	deployment2, err := r.inspectDeploymentV1(ctx, request.Bootstrap, request.DeploymentCurrent, now2)
	if err != nil {
		return AssembleDefinitionResultV1{}, err
	}
	if !reflect.DeepEqual(deployment1, deployment2) {
		return AssembleDefinitionResultV1{}, contract.NewError(contract.ErrorConflict, "declarative_assemble_deployment_drift", "Deployment current changed during explicit assembly")
	}
	if err = request.DefinitionSource.Validate(now2.UnixNano()); err != nil {
		return AssembleDefinitionResultV1{}, err
	}
	return AssembleDefinitionResultV1{Plan: plan}, nil
}

type RunDefinitionRequestV1 struct {
	Bootstrap                 contract.HostBootstrapConfigV1
	DeploymentCurrent         contract.HostDeploymentCurrentRefV1
	StartID                   string
	DefinitionFormat          DefinitionFormatV1
	DefinitionPayload         []byte
	RequestedNotAfterUnixNano int64
}

func (r *DeclarativeRootV1) RunDefinitionV1(ctx context.Context, request RunDefinitionRequestV1) (contract.StartResultV3, error) {
	if r == nil || ctx == nil || ctx.Err() != nil {
		return contract.StartResultV3{}, contract.NewError(contract.ErrorInvalidArgument, "declarative_run_context_invalid", "declarative run requires a live root and context")
	}
	validated, err := r.ValidateDefinitionV1(ctx, ValidateDefinitionRequestV1{Format: request.DefinitionFormat, Payload: request.DefinitionPayload})
	if err != nil {
		return contract.StartResultV3{}, err
	}
	if err = contract.ValidateIdentifierV1("start id", request.StartID); err != nil {
		return contract.StartResultV3{}, err
	}
	cursor := rootClockCursorV1{read: r.config.Clock}
	now1, err := cursor.observe()
	if err != nil {
		return contract.StartResultV3{}, err
	}
	deployment1, err := r.inspectDeploymentV1(ctx, request.Bootstrap, request.DeploymentCurrent, now1)
	if err != nil {
		return contract.StartResultV3{}, err
	}
	if request.RequestedNotAfterUnixNano <= now1.UnixNano() {
		return contract.StartResultV3{}, contract.NewError(contract.ErrorInvalidArgument, "declarative_run_window_invalid", "declarative run deadline must be in the future")
	}
	published, err := r.config.Definitions.CreateSourceV1(ctx, validated.Source)
	if err != nil {
		return contract.StartResultV3{}, err
	}
	if err = validateDefinitionPublicationV1(published, validated, r.config.DefinitionCatalog); err != nil {
		return contract.StartResultV3{}, err
	}
	deadline := minimumUnixNanoV1(request.RequestedNotAfterUnixNano, request.Bootstrap.NotAfterUnixNano, deployment1.ExpiresUnixNano, published.Definition.EffectiveWindow.NotAfterUnixNano)
	sourceCurrent, err := r.config.DefinitionSources.EnsureDefinitionSourceCurrentV1(ctx, request.Bootstrap.DefinitionSourceBindingID, published.Definition.RefV1(), deadline)
	if err != nil {
		return contract.StartResultV3{}, err
	}
	now2, err := cursor.observe()
	if err != nil {
		return contract.StartResultV3{}, err
	}
	if err = validateSourcePublicationV1(sourceCurrent, request.Bootstrap.DefinitionSourceBindingID, published.Definition.RefV1(), deadline, now2); err != nil {
		return contract.StartResultV3{}, err
	}
	deployment2, err := r.inspectDeploymentV1(ctx, request.Bootstrap, request.DeploymentCurrent, now2)
	if err != nil {
		return contract.StartResultV3{}, err
	}
	if !reflect.DeepEqual(deployment1, deployment2) {
		return contract.StartResultV3{}, contract.NewError(contract.ErrorConflict, "declarative_run_deployment_drift", "Deployment current changed before Host Start")
	}
	start, err := contract.SealStartRequestV3(contract.StartRequestV3{
		StartID:                   request.StartID,
		DeploymentCurrentRef:      request.DeploymentCurrent,
		Config:                    hostConfigFromBootstrapV1(request.Bootstrap),
		DefinitionSourceCurrent:   definitionSourceRefV1(sourceCurrent),
		RequestedAtUnixNano:       now2.UnixNano(),
		RequestedNotAfterUnixNano: deadline,
	})
	if err != nil {
		return contract.StartResultV3{}, err
	}
	result, err := r.config.Host.StartV3(ctx, start)
	if err != nil {
		return contract.StartResultV3{}, err
	}
	checked, err := cursor.observe()
	if err != nil {
		return contract.StartResultV3{}, err
	}
	if err = result.ValidateFor(start, checked); err != nil {
		return contract.StartResultV3{}, err
	}
	return result, nil
}

func (r *DeclarativeRootV1) InspectV1(ctx context.Context, request contract.InspectRequestV3) (contract.InspectResultV3, error) {
	if r == nil || isNilRootV1(r.config.Host) {
		return contract.InspectResultV3{}, contract.NewError(contract.ErrorUnavailable, "declarative_root_unavailable", "declarative root is unavailable")
	}
	return r.config.Host.InspectV3(ctx, request)
}

func (r *DeclarativeRootV1) StopV1(ctx context.Context, request contract.StopRequestV3) (contract.StopResultV3, error) {
	if r == nil || isNilRootV1(r.config.Host) {
		return contract.StopResultV3{}, contract.NewError(contract.ErrorUnavailable, "declarative_root_unavailable", "declarative root is unavailable")
	}
	return r.config.Host.StopV3(ctx, request)
}

func (r *DeclarativeRootV1) inspectDeploymentV1(ctx context.Context, bootstrap contract.HostBootstrapConfigV1, ref contract.HostDeploymentCurrentRefV1, now time.Time) (contract.HostDeploymentCurrentV1, error) {
	if err := bootstrap.ValidateCurrentV1(now); err != nil {
		return contract.HostDeploymentCurrentV1{}, err
	}
	if ref.BootstrapDigest != bootstrap.ContentDigest || ref.HostID != bootstrap.HostID {
		return contract.HostDeploymentCurrentV1{}, contract.NewError(contract.ErrorConflict, "declarative_deployment_bootstrap_drift", "Deployment current Ref does not bind Bootstrap")
	}
	value, err := r.config.Deployments.InspectHostDeploymentCurrentV1(ctx, ref)
	if err != nil {
		return contract.HostDeploymentCurrentV1{}, err
	}
	if err = value.ValidateForBootstrapV1(bootstrap, now); err != nil {
		return contract.HostDeploymentCurrentV1{}, err
	}
	return value, nil
}

func decodeDefinitionV1(format DefinitionFormatV1, payload []byte, catalog definitioncontract.ValidationCatalogV1) (definitioncontract.AgentDefinitionSourceV1, error) {
	switch format {
	case DefinitionFormatJSONV1:
		return definitiondecoder.DecodeJSONV1(payload, catalog)
	case DefinitionFormatYAMLV1:
		return definitiondecoder.DecodeYAMLV1(payload, catalog)
	default:
		return definitioncontract.AgentDefinitionSourceV1{}, contract.NewError(contract.ErrorInvalidArgument, "definition_format_unsupported", "definition format must be explicit json or yaml")
	}
}

func validateDefinitionPublicationV1(actual definitionports.CreateDefinitionResultV1, expected ValidateDefinitionResultV1, catalog definitioncontract.ValidationCatalogV1) error {
	if err := actual.Definition.Validate(catalog); err != nil {
		return err
	}
	if actual.Definition.SourceDigest != expected.SourceDigest || actual.Definition.DefinitionID != expected.Source.DefinitionID || actual.Definition.Revision != expected.Source.Revision {
		return contract.NewError(contract.ErrorConflict, "definition_publication_drift", "Definition Owner published different content")
	}
	if err := actual.Current.Validate(); err != nil {
		return err
	}
	if actual.Current.Definition != actual.Definition.RefV1() || actual.Current.State != definitioncontract.DefinitionCurrentActiveV1 {
		return contract.NewError(contract.ErrorConflict, "definition_publication_current_drift", "Definition Owner current does not bind the published revision")
	}
	return nil
}

func validateSourcePublicationV1(actual contract.DefinitionSourceCurrentV1, sourceID string, definition definitioncontract.AgentDefinitionRefV1, deadline int64, now time.Time) error {
	if err := actual.Validate(now.UnixNano()); err != nil {
		return err
	}
	expected := contract.ExactRefV1{Kind: "praxis.agent-definition/definition", ID: definition.DefinitionID, Revision: uint64(definition.Revision), Digest: contract.DigestV1(definition.Digest)}
	if actual.SourceStableID != sourceID || actual.DefinitionExactRef != expected || actual.ExpiresUnixNano > deadline {
		return contract.NewError(contract.ErrorConflict, "definition_source_publication_drift", "Definition source Owner published different coordinates or exceeded the requested window")
	}
	return nil
}

func definitionSourceRefV1(value contract.DefinitionSourceCurrentV1) contract.ExactRefV1 {
	return contract.ExactRefV1{Kind: contract.DefinitionSourceCurrentKindV1, ID: value.SourceStableID, Revision: value.Revision, Digest: value.ProjectionDigest}
}

func hostConfigFromBootstrapV1(value contract.HostBootstrapConfigV1) contract.HostConfigV1 {
	return contract.HostConfigV1{
		ContractVersion: contract.ContractVersionV1, HostID: value.HostID,
		DefinitionSourceRef:  value.DefinitionSourceBindingID,
		StatePlaneBindings:   append([]string(nil), value.StatePlaneBindingIDs...),
		ProviderEndpointRefs: []string{value.ProviderEndpointRegistryBindingID},
		SecretBrokerRef:      value.SecretBrokerBindingID, CatalogRef: value.CatalogBindingID,
		ResolutionFactsRef: value.ResolutionFactsBindingID,
		RuntimeServiceRefs: append([]string(nil), value.RuntimeServiceBindingIDs...),
		ListenRef:          value.ListenBindingID, DiagnosticsPolicyRef: value.DiagnosticsPolicyBindingID,
	}.CanonicalV1()
}

type rootClockCursorV1 struct {
	read func() time.Time
	last time.Time
}

func (c *rootClockCursorV1) observe() (time.Time, error) {
	now := c.read()
	if now.IsZero() || (!c.last.IsZero() && now.Before(c.last)) {
		return time.Time{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "declarative root clock regressed")
	}
	c.last = now
	return now, nil
}

func minimumUnixNanoV1(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func isNilRootV1(value any) bool {
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

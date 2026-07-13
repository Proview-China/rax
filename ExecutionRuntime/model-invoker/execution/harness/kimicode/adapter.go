package kimicode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/acp"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

var ErrInvalidConfig = errors.New("Kimi Code current ACP adapter configuration is invalid")

var forbiddenFeatures = map[string]struct{}{
	"legacy_wire":      {},
	"agent_file":       {},
	"str_replace_file": {},
}

type Config struct {
	ACP                acp.AdapterConfig
	ProtocolGeneration string
}

type Adapter struct {
	config Config
	base   *acp.Adapter
}

func New(config Config) (*Adapter, error) {
	if config.ProtocolGeneration != "current_acp" {
		return nil, fmt.Errorf("%w: protocol generation must be current_acp", ErrInvalidConfig)
	}
	if !containsArgument(config.ACP.Client.Process.Arguments, "acp") {
		return nil, fmt.Errorf("%w: pinned Kimi executable must use the acp subcommand", ErrInvalidConfig)
	}
	for _, argument := range config.ACP.Client.Process.Arguments {
		if normalizedFeature(argument) == "legacy_wire" || argument == "--wire" || argument == "wire" {
			return nil, fmt.Errorf("%w: legacy Wire launch is forbidden", ErrInvalidConfig)
		}
	}
	if feature, found := forbiddenJSONFeature(config.ACP.SessionOptions); found {
		return nil, fmt.Errorf("%w: session options request forbidden feature %q", ErrInvalidConfig, feature)
	}
	base, err := acp.NewAdapter(config.ACP)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	return &Adapter{config: config, base: base}, nil
}

func NewAdapter(config Config) (*Adapter, error) { return New(config) }

func (adapter *Adapter) Describe(ctx context.Context) (execution.AdapterDescriptor, error) {
	if adapter == nil {
		return execution.AdapterDescriptor{}, ErrInvalidConfig
	}
	return adapter.base.Describe(ctx)
}

func (adapter *Adapter) Preflight(ctx context.Context, invocation execution.Invocation) (execution.PreflightReport, error) {
	if adapter == nil {
		return execution.PreflightReport{}, ErrInvalidConfig
	}
	if feature := invocationForbiddenFeature(invocation); feature != "" {
		return execution.PreflightReport{Accepted: false, RejectionCode: "kimi_forbidden_legacy_surface_" + feature}, nil
	}
	report, err := adapter.base.Preflight(ctx, invocation)
	if err != nil || !report.Accepted {
		return report, err
	}
	manifest, err := report.ActualManifest.Clone()
	if err != nil {
		_ = adapter.base.ClosePrepared(invocation.Request.ExecutionID)
		return execution.PreflightReport{}, err
	}
	manifest.ID = invocation.Plan.ExpectedManifest.ID + ".kimi-current-acp.actual"
	manifest.Digest = ""
	digest, err := union.StableDigest(map[string]any{
		"route": invocation.Plan.Route, "surface": "kimi_code_acp", "protocol_generation": adapter.config.ProtocolGeneration,
		"forbidden": []string{"legacy_wire", "agent_file", "str_replace_file"},
	})
	if err != nil {
		_ = adapter.base.ClosePrepared(invocation.Request.ExecutionID)
		return execution.PreflightReport{}, err
	}
	manifest.Components = append(manifest.Components, union.ManifestComponent{
		Kind: "native_surface", Name: "kimi_current_acp", State: "reported", Version: adapter.config.ProtocolGeneration,
		Digest: digest, Owner: union.ExecutionOwnerHarness, ModelVisible: true, Opaque: true,
	})
	manifest.OpaqueFields = appendUnique(manifest.OpaqueFields, "instructions.kimi_current_acp_internal_loop")
	if err := manifest.Validate(); err != nil {
		_ = adapter.base.ClosePrepared(invocation.Request.ExecutionID)
		return execution.PreflightReport{}, fmt.Errorf("%w: actual manifest: %v", ErrInvalidConfig, err)
	}
	manifest.Digest, err = manifest.ComputeDigest()
	if err != nil {
		_ = adapter.base.ClosePrepared(invocation.Request.ExecutionID)
		return execution.PreflightReport{}, err
	}
	report.ActualManifest = manifest
	report.Residuals = append(report.Residuals, union.Residual{
		Path: "context.harness.kimi.current_acp", Kind: "current_acp_harness_context", Severity: "P2",
		Impact:     "Kimi Code ACP retains an internal agent loop whose context is not fully represented by prompt blocks.",
		Mitigation: "Legacy compatibility surfaces are rejected and final Effects remain observer-owned.",
	})
	return report, nil
}

func (adapter *Adapter) Open(ctx context.Context, invocation execution.Invocation) (execution.Session, error) {
	if adapter == nil {
		return nil, ErrInvalidConfig
	}
	if feature := invocationForbiddenFeature(invocation); feature != "" {
		_ = adapter.base.ClosePrepared(invocation.Request.ExecutionID)
		return nil, fmt.Errorf("%w: forbidden legacy feature %q", ErrInvalidConfig, feature)
	}
	return adapter.base.Open(ctx, invocation)
}

func (adapter *Adapter) ClosePrepared(executionID union.ExecutionID) error {
	if adapter == nil {
		return nil
	}
	return adapter.base.ClosePrepared(executionID)
}

func invocationForbiddenFeature(invocation execution.Invocation) string {
	for name, raw := range invocation.Request.Extensions {
		if forbidden(name) {
			return normalizedFeature(name)
		}
		if feature, found := forbiddenJSONFeature(raw); found {
			return feature
		}
	}
	for key, value := range invocation.Request.Metadata {
		if forbidden(key) || forbidden(value) {
			if forbidden(key) {
				return normalizedFeature(key)
			}
			return normalizedFeature(value)
		}
	}
	for _, tool := range invocation.Request.Tools {
		for _, value := range []string{tool.ID, tool.Name, tool.Kind} {
			if forbidden(value) {
				return normalizedFeature(value)
			}
		}
		if feature, found := forbiddenJSONFeature(tool.Extension); found {
			return feature
		}
	}
	for _, plan := range invocation.Plan.Mechanisms {
		for _, value := range append([]string{plan.Kind, plan.CapabilityRef}, plan.HardConstraints...) {
			if forbidden(value) {
				return normalizedFeature(value)
			}
		}
	}
	for key, value := range invocation.Plan.Metadata {
		if forbidden(key) {
			return normalizedFeature(key)
		}
		for _, token := range strings.FieldsFunc(value, func(character rune) bool {
			return character == ',' || character == ';' || character == ' ' || character == '|'
		}) {
			if forbidden(token) {
				return normalizedFeature(token)
			}
		}
	}
	return ""
}

func forbiddenJSONFeature(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || !json.Valid(raw) {
		return "", false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return walkForbidden(value)
}

func walkForbidden(value any) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if forbidden(key) {
				return normalizedFeature(key), true
			}
			if feature, found := walkForbidden(nested); found {
				return feature, true
			}
		}
	case []any:
		for _, nested := range typed {
			if feature, found := walkForbidden(nested); found {
				return feature, true
			}
		}
	case string:
		if forbidden(typed) {
			return normalizedFeature(typed), true
		}
	}
	return "", false
}

func forbidden(value string) bool {
	_, found := forbiddenFeatures[normalizedFeature(value)]
	return found
}

func normalizedFeature(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimLeft(normalized, "-")
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return normalized
}

func containsArgument(arguments []string, wanted string) bool {
	for _, argument := range arguments {
		if argument == wanted {
			return true
		}
	}
	return false
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}

var (
	_ execution.Adapter          = (*Adapter)(nil)
	_ execution.PreflightCleaner = (*Adapter)(nil)
)

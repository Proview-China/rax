package gemini

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/acp"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

var ErrInvalidConfig = errors.New("Gemini CLI ACP adapter configuration is invalid")

type Config struct {
	ACP                     acp.AdapterConfig
	FirstUserSessionContext bool
}

type Adapter struct {
	config Config
	base   *acp.Adapter
}

func New(config Config) (*Adapter, error) {
	if !config.FirstUserSessionContext {
		return nil, fmt.Errorf("%w: first-user session context acknowledgement is required", ErrInvalidConfig)
	}
	if !containsArgument(config.ACP.Client.Process.Arguments, "--acp") {
		return nil, fmt.Errorf("%w: pinned Gemini executable must be launched with --acp", ErrInvalidConfig)
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
	report, err := adapter.base.Preflight(ctx, invocation)
	if err != nil || !report.Accepted {
		return report, err
	}
	manifest, err := report.ActualManifest.Clone()
	if err != nil {
		_ = adapter.base.ClosePrepared(invocation.Request.ExecutionID)
		return execution.PreflightReport{}, err
	}
	manifest.ID = invocation.Plan.ExpectedManifest.ID + ".gemini-acp.actual"
	manifest.Digest = ""
	digest, err := union.StableDigest(map[string]any{
		"route": invocation.Plan.Route, "surface": "gemini_cli_acp", "context_tier": "first_user_session_context",
	})
	if err != nil {
		_ = adapter.base.ClosePrepared(invocation.Request.ExecutionID)
		return execution.PreflightReport{}, err
	}
	manifest.Components = append(manifest.Components, union.ManifestComponent{
		Kind: "native_surface", Name: "gemini_first_user_session_context", State: "reported", Version: "tiered-context-v1",
		Digest: digest, Owner: union.ExecutionOwnerHarness, ModelVisible: true, Opaque: true,
	})
	manifest.OpaqueFields = appendUnique(manifest.OpaqueFields, "instructions.gemini_first_user_session_context")
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
		Path: "context.harness.gemini.first_user", Kind: "first_user_session_context", Severity: "P2",
		Impact:     "Gemini CLI places extension and project session context in the first user turn outside the ACP prompt blocks.",
		Mitigation: "The context is declared in ActualManifest and final Effects remain observer-owned.",
	})
	return report, nil
}

func (adapter *Adapter) Open(ctx context.Context, invocation execution.Invocation) (execution.Session, error) {
	if adapter == nil {
		return nil, ErrInvalidConfig
	}
	return adapter.base.Open(ctx, invocation)
}

func (adapter *Adapter) ClosePrepared(executionID union.ExecutionID) error {
	if adapter == nil {
		return nil
	}
	return adapter.base.ClosePrepared(executionID)
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

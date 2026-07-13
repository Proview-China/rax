package acp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	harnessprocess "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/process"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type AdapterConfig struct {
	Identity          union.VersionedIdentity
	RouteID           string
	Client            Config
	ExpectedAgentName string
	SessionOptions    json.RawMessage
	ApprovalTTL       time.Duration
	Clock             func() time.Time
}

type Adapter struct {
	config   AdapterConfig
	mu       sync.Mutex
	prepared map[union.ExecutionID]*preparedExecution
}

type preparedExecution struct {
	client        *Client
	manifest      union.ContextManifestSummary
	planDigest    string
	requestDigest string
}

func NewAdapter(config AdapterConfig) (*Adapter, error) {
	config = cloneAdapterConfig(config)
	if len(config.SessionOptions) == 0 {
		config.SessionOptions = json.RawMessage(`{}`)
	}
	if config.ApprovalTTL == 0 {
		config.ApprovalTTL = 2 * time.Minute
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if err := validateAdapterConfig(config); err != nil {
		return nil, err
	}
	return &Adapter{config: config, prepared: make(map[union.ExecutionID]*preparedExecution)}, nil
}

func validateAdapterConfig(config AdapterConfig) error {
	if err := config.Identity.Validate("acp.identity"); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if strings.TrimSpace(config.RouteID) == "" || strings.TrimSpace(config.ExpectedAgentName) == "" || config.ApprovalTTL <= 0 || !validJSONObject(config.SessionOptions) || !validJSONObject(config.Client.InitializeParams) {
		return fmt.Errorf("%w: RouteID, expected Agent identity, object initialize/session options and positive approval TTL are required", ErrInvalidConfig)
	}
	process := config.Client.Process
	if !filepath.IsAbs(process.Executable) || !filepath.IsAbs(process.WorkingDirectory) || len(process.AllowedWorkingDirectories) == 0 || process.Protocol != harnessprocess.ProtocolJSONRPCNDJSON {
		return fmt.Errorf("%w: explicit executable, cwd allowlist and jsonrpc_ndjson protocol are required", ErrInvalidConfig)
	}
	return nil
}

func (adapter *Adapter) Describe(_ context.Context) (execution.AdapterDescriptor, error) {
	if adapter == nil {
		return execution.AdapterDescriptor{}, ErrInvalidConfig
	}
	return execution.AdapterDescriptor{
		Identity: adapter.config.Identity, Origin: union.EventOriginHarness,
		ExecutionKinds: []union.ExecutionKind{union.ExecutionKindAgent},
	}, nil
}

func (adapter *Adapter) Preflight(ctx context.Context, invocation execution.Invocation) (execution.PreflightReport, error) {
	if adapter == nil || ctx == nil {
		return execution.PreflightReport{}, ErrInvalidConfig
	}
	if err := invocation.Validate(); err != nil {
		return execution.PreflightReport{}, err
	}
	if invocation.Plan.ExecutionKind != union.ExecutionKindAgent || invocation.Plan.Route.ID != adapter.config.RouteID {
		return execution.PreflightReport{Accepted: false, RejectionCode: "acp_route_mismatch"}, nil
	}
	if _, _, err := mapACPInvocation(invocation, adapter.config); err != nil {
		return execution.PreflightReport{Accepted: false, RejectionCode: "acp_mapping_rejected"}, nil
	}
	adapter.mu.Lock()
	_, exists := adapter.prepared[invocation.Request.ExecutionID]
	adapter.mu.Unlock()
	if exists {
		return execution.PreflightReport{}, ErrAlreadyPrepared
	}
	evidence, err := collectLaunchEvidence(adapter.config.Client.Process)
	if err != nil {
		return execution.PreflightReport{}, err
	}
	probe, err := Start(ctx, adapter.config.Client)
	if err != nil {
		return execution.PreflightReport{}, fmt.Errorf("probe ACP initialize: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = probe.Close()
		}
	}()
	initialize := probe.InitializeResult()
	agentName := nestedString(initialize.Raw, "agentInfo", "name")
	agentVersion := nestedString(initialize.Raw, "agentInfo", "version")
	if agentName != adapter.config.ExpectedAgentName {
		return execution.PreflightReport{Accepted: false, RejectionCode: "acp_agent_identity_drift"}, nil
	}
	actual, err := acpActualManifest(invocation.Plan.ExpectedManifest, initialize.Raw, agentName, agentVersion, evidence)
	if err != nil {
		return execution.PreflightReport{}, err
	}
	planDigest, err := invocation.Plan.ComputeDigest()
	if err != nil {
		return execution.PreflightReport{}, err
	}
	requestDigest, err := invocation.Request.Digest()
	if err != nil {
		return execution.PreflightReport{}, err
	}
	adapter.mu.Lock()
	if _, duplicate := adapter.prepared[invocation.Request.ExecutionID]; duplicate {
		adapter.mu.Unlock()
		return execution.PreflightReport{}, ErrAlreadyPrepared
	}
	adapter.prepared[invocation.Request.ExecutionID] = &preparedExecution{
		client: probe, manifest: actual, planDigest: planDigest, requestDigest: requestDigest,
	}
	adapter.mu.Unlock()
	keep = true
	return execution.PreflightReport{
		Accepted:       true,
		ActualManifest: actual,
		Residuals: []union.Residual{{
			Path: "context.harness.acp", Kind: "opaque_harness_context", Severity: "P2",
			Impact:     "The ACP Agent may add native instructions and tools outside the standard prompt surface.",
			Mitigation: "The route mapper retains native updates and Effect reconciliation observes host state independently.",
		}},
	}, nil
}

func (adapter *Adapter) Open(ctx context.Context, invocation execution.Invocation) (execution.Session, error) {
	if adapter == nil || ctx == nil {
		return nil, ErrInvalidConfig
	}
	if err := invocation.Validate(); err != nil {
		return nil, err
	}
	if invocation.Plan.ExecutionKind != union.ExecutionKindAgent || invocation.Plan.Route.ID != adapter.config.RouteID {
		return nil, fmt.Errorf("%w: route mismatch", ErrMapping)
	}
	adapter.mu.Lock()
	prepared := adapter.prepared[invocation.Request.ExecutionID]
	delete(adapter.prepared, invocation.Request.ExecutionID)
	adapter.mu.Unlock()
	if prepared == nil {
		return nil, ErrPreparedNotFound
	}
	planDigest, planErr := invocation.Plan.ComputeDigest()
	requestDigest, requestErr := invocation.Request.Digest()
	if planErr != nil || requestErr != nil || planDigest != prepared.planDigest || requestDigest != prepared.requestDigest {
		_ = prepared.client.Close()
		return nil, ErrRouteMismatch
	}
	sessionParams, prompt, err := mapACPInvocation(invocation, adapter.config)
	if err != nil {
		_ = prepared.client.Close()
		return nil, err
	}
	bindings, err := selectAttemptBindings(invocation)
	if err != nil {
		_ = prepared.client.Close()
		return nil, err
	}
	client := prepared.client
	native, _, err := client.NewSession(ctx, sessionParams)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	primary := bindings[0]
	mapper, err := NewMapper(MappingContext{
		ExecutionID: invocation.Request.ExecutionID, Profile: invocation.Plan.Profile, Route: invocation.Plan.Route,
		IntentID: primary.intentID, MechanismPlanID: primary.planID, MechanismAttemptID: primary.attemptID,
		ApprovalTTL: adapter.config.ApprovalTTL, Clock: adapter.config.Clock,
	})
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	session := newExecutionSession(native, mapper, invocation, bindings, adapter.config.Clock)
	session.startPrompt(ctx, prompt)
	return session, nil
}

func (adapter *Adapter) ClosePrepared(executionID union.ExecutionID) error {
	if adapter == nil {
		return nil
	}
	adapter.mu.Lock()
	prepared := adapter.prepared[executionID]
	delete(adapter.prepared, executionID)
	adapter.mu.Unlock()
	if prepared == nil {
		return nil
	}
	return prepared.client.Close()
}

type launchEvidence struct {
	actualExecutable  string
	actualDigest      string
	expectedDigest    string
	arguments         []string
	argumentsDigest   string
	environmentNames  []string
	environmentDigest string
	workingDirectory  string
	evidenceDigest    string
}

func collectLaunchEvidence(config harnessprocess.Config) (launchEvidence, error) {
	actualExecutable, err := filepath.EvalSymlinks(filepath.Clean(config.Executable))
	if err != nil {
		return launchEvidence{}, fmt.Errorf("%w: resolve executable: %v", ErrInvalidConfig, err)
	}
	file, err := os.Open(actualExecutable)
	if err != nil {
		return launchEvidence{}, fmt.Errorf("%w: open executable: %v", ErrInvalidConfig, err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil {
		return launchEvidence{}, fmt.Errorf("%w: hash executable: %v", ErrInvalidConfig, copyErr)
	}
	if closeErr != nil {
		return launchEvidence{}, fmt.Errorf("%w: close executable: %v", ErrInvalidConfig, closeErr)
	}
	workingDirectory, err := filepath.EvalSymlinks(filepath.Clean(config.WorkingDirectory))
	if err != nil {
		return launchEvidence{}, fmt.Errorf("%w: resolve cwd: %v", ErrInvalidConfig, err)
	}
	environmentNames := make([]string, 0, len(config.Environment))
	environmentEntries := make([]string, 0, len(config.Environment))
	for name, value := range config.Environment {
		environmentNames = append(environmentNames, name)
		environmentEntries = append(environmentEntries, name+"="+value)
	}
	sort.Strings(environmentNames)
	sort.Strings(environmentEntries)
	arguments := append([]string(nil), config.Arguments...)
	argumentsDigest, err := union.StableDigest(arguments)
	if err != nil {
		return launchEvidence{}, err
	}
	environmentDigest, err := union.StableDigest(environmentEntries)
	if err != nil {
		return launchEvidence{}, err
	}
	evidenceDigest, err := union.StableDigest(map[string]any{
		"executable": actualExecutable, "arguments_digest": argumentsDigest, "environment_digest": environmentDigest,
		"cwd": workingDirectory, "protocol": config.Protocol,
	})
	if err != nil {
		return launchEvidence{}, err
	}
	return launchEvidence{
		actualExecutable: actualExecutable, actualDigest: fmt.Sprintf("sha256:%x", hash.Sum(nil)), expectedDigest: config.ExpectedExecutableDigest,
		arguments: arguments, argumentsDigest: argumentsDigest, environmentNames: environmentNames, environmentDigest: environmentDigest,
		workingDirectory: workingDirectory, evidenceDigest: evidenceDigest,
	}, nil
}

func acpActualManifest(expected union.ContextManifestSummary, initialize []byte, agentName, agentVersion string, evidence launchEvidence) (union.ContextManifestSummary, error) {
	actual, err := expected.Clone()
	if err != nil {
		return union.ContextManifestSummary{}, err
	}
	actual.ID = expected.ID + ".acp.actual"
	actual.Digest = ""
	initializeDigest, err := union.StableDigest(json.RawMessage(initialize))
	if err != nil {
		return union.ContextManifestSummary{}, err
	}
	argumentsJSON, _ := json.Marshal(evidence.arguments)
	environmentJSON, _ := json.Marshal(evidence.environmentNames)
	actual.Components = append(actual.Components,
		union.ManifestComponent{Kind: "launch_probe", Name: "actual_executable", State: "observed", Version: evidence.actualExecutable, Digest: evidence.actualDigest, Owner: union.ExecutionOwnerHarness, Executable: true},
		union.ManifestComponent{Kind: "launch_probe", Name: "executable_pin", State: "observed", Version: evidence.expectedDigest, Digest: evidence.actualDigest, Owner: union.ExecutionOwnerHarness},
		union.ManifestComponent{Kind: "launch_probe", Name: "actual_argv", State: "observed", Version: string(argumentsJSON), Digest: evidence.argumentsDigest, Owner: union.ExecutionOwnerHarness},
		union.ManifestComponent{Kind: "launch_probe", Name: "sanitized_environment", State: "observed", Version: string(environmentJSON), Digest: evidence.environmentDigest, Owner: union.ExecutionOwnerHarness},
		union.ManifestComponent{Kind: "launch_probe", Name: "actual_cwd", State: "observed", Version: evidence.workingDirectory, Digest: evidence.evidenceDigest, Owner: union.ExecutionOwnerHarness},
		union.ManifestComponent{Kind: "native_surface", Name: "initialize", State: "reported", Digest: initializeDigest, Owner: union.ExecutionOwnerHarness, ModelVisible: true, Opaque: true},
		union.ManifestComponent{Kind: "native_surface", Name: "agent_identity", State: "reported", Version: agentName + "@" + agentVersion, Digest: initializeDigest, Owner: union.ExecutionOwnerHarness, ModelVisible: true},
	)
	actual.OpaqueFields = appendUnique(actual.OpaqueFields, "instructions.acp_agent_internal_loop")
	if err := actual.Validate(); err != nil {
		return union.ContextManifestSummary{}, fmt.Errorf("%w: actual manifest: %v", ErrProtocol, err)
	}
	actual.Digest, err = actual.ComputeDigest()
	if err != nil {
		return union.ContextManifestSummary{}, err
	}
	return actual, nil
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func cloneAdapterConfig(config AdapterConfig) AdapterConfig {
	clone := config
	clone.Client.InitializeParams = cloneRaw(config.Client.InitializeParams)
	clone.SessionOptions = cloneRaw(config.SessionOptions)
	clone.Client.Process.Arguments = append([]string(nil), config.Client.Process.Arguments...)
	clone.Client.Process.AllowedWorkingDirectories = append([]string(nil), config.Client.Process.AllowedWorkingDirectories...)
	clone.Client.Process.AllowedEnvironment = append([]string(nil), config.Client.Process.AllowedEnvironment...)
	if config.Client.Process.Environment != nil {
		clone.Client.Process.Environment = make(map[string]string, len(config.Client.Process.Environment))
		for name, value := range config.Client.Process.Environment {
			clone.Client.Process.Environment[name] = value
		}
	}
	return clone
}

var _ execution.Adapter = (*Adapter)(nil)

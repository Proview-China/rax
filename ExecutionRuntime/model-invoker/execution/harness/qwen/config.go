package qwen

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	harnessprocess "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/process"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/streamjson"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type SurfaceMode string

const (
	SurfaceBareFixed         SurfaceMode = "bare_fixed"
	SurfaceControlledNonBare SurfaceMode = "controlled_nonbare"
)

var bareFixedTools = []string{"edit", "notebook_edit", "read_file", "run_shell_command"}

type Config struct {
	Identity          union.VersionedIdentity
	Route             union.VersionedIdentity
	Process           harnessprocess.Config
	InitializeRequest json.RawMessage
	ExpectedInit      ExpectedInit
	SurfaceMode       SurfaceMode
	CoreTools         []string
	ExcludeTools      []string
	FallbackModel     string
	ApprovalTTL       time.Duration
	Clock             func() time.Time
}

func (config Config) clone() Config {
	clone := config
	clone.Process = streamjson.CloneProcessConfig(config.Process)
	clone.InitializeRequest = append(json.RawMessage(nil), config.InitializeRequest...)
	clone.ExpectedInit.Tools = append([]string(nil), config.ExpectedInit.Tools...)
	clone.ExpectedInit.MCPServers = append([]string(nil), config.ExpectedInit.MCPServers...)
	clone.ExpectedInit.Agents = append([]string(nil), config.ExpectedInit.Agents...)
	clone.ExpectedInit.Skills = append([]string(nil), config.ExpectedInit.Skills...)
	clone.CoreTools = append([]string(nil), config.CoreTools...)
	clone.ExcludeTools = append([]string(nil), config.ExcludeTools...)
	return clone
}

func (config *Config) normalize() error {
	if err := config.Identity.Validate("qwen.identity"); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if err := config.Route.Validate("qwen.route"); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if config.Process.Protocol != harnessprocess.ProtocolJSONL || strings.TrimSpace(config.Process.ExpectedExecutableDigest) == "" {
		return fmt.Errorf("%w: jsonl and a pinned executable digest are required", ErrInvalidConfig)
	}
	bare := hasArgument(config.Process.Arguments, "--bare")
	coreToolsOnArgv := hasArgument(config.Process.Arguments, "--core-tools") || hasArgumentPrefix(config.Process.Arguments, "--core-tools=")
	if bare && (len(config.CoreTools) != 0 || coreToolsOnArgv) {
		return ErrBareCoreTools
	}
	if strings.TrimSpace(config.FallbackModel) != "" || hasArgument(config.Process.Arguments, "--fallback-model") || hasArgumentPrefix(config.Process.Arguments, "--fallback-model=") {
		return fmt.Errorf("%w: model fallback must be disabled", ErrInvalidConfig)
	}
	config.CoreTools = sortedStrings(config.CoreTools)
	config.ExcludeTools = sortedStrings(config.ExcludeTools)
	switch config.SurfaceMode {
	case SurfaceBareFixed:
		if !bare {
			return fmt.Errorf("%w: bare_fixed requires --bare", ErrInvalidConfig)
		}
		want, err := subtractTools(bareFixedTools, config.ExcludeTools)
		if err != nil {
			return err
		}
		if !sameStrings(config.ExpectedInit.Tools, want) {
			return fmt.Errorf("%w: bare_fixed expected tools must equal fixed tools minus excludeTools", ErrInvalidConfig)
		}
	case SurfaceControlledNonBare:
		if bare || len(config.CoreTools) == 0 {
			return fmt.Errorf("%w: controlled_nonbare requires coreTools and forbids --bare", ErrInvalidConfig)
		}
		want, err := subtractTools(config.CoreTools, config.ExcludeTools)
		if err != nil {
			return err
		}
		if !sameStrings(config.ExpectedInit.Tools, want) {
			return fmt.Errorf("%w: controlled_nonbare expected tools must equal coreTools minus excludeTools", ErrInvalidConfig)
		}
	default:
		return fmt.Errorf("%w: unknown surface mode %q", ErrInvalidConfig, config.SurfaceMode)
	}
	if strings.TrimSpace(config.ExpectedInit.Model) == "" || strings.TrimSpace(config.ExpectedInit.CWD) == "" ||
		strings.TrimSpace(config.ExpectedInit.QwenVersion) == "" || len(config.ExpectedInit.Tools) == 0 {
		return fmt.Errorf("%w: exact model, cwd, Qwen version, and tools are required", ErrInvalidConfig)
	}
	config.ExpectedInit.Tools = sortedStrings(config.ExpectedInit.Tools)
	config.ExpectedInit.MCPServers = sortedStrings(config.ExpectedInit.MCPServers)
	config.ExpectedInit.Agents = sortedStrings(config.ExpectedInit.Agents)
	config.ExpectedInit.Skills = sortedStrings(config.ExpectedInit.Skills)
	if config.ApprovalTTL == 0 {
		config.ApprovalTTL = 5 * time.Minute
	}
	if config.ApprovalTTL < time.Second {
		return fmt.Errorf("%w: approval TTL must be at least one second", ErrInvalidConfig)
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if len(config.InitializeRequest) == 0 {
		config.InitializeRequest = json.RawMessage(`{"subtype":"initialize","hooks":null,"mcpServers":{},"agents":[]}`)
	}
	var initialize map[string]json.RawMessage
	if json.Unmarshal(config.InitializeRequest, &initialize) != nil || objectString(config.InitializeRequest, "subtype") != "initialize" {
		return fmt.Errorf("%w: initialize request must be an object with subtype=initialize", ErrInvalidConfig)
	}
	return nil
}

func subtractTools(base, excludes []string) ([]string, error) {
	available := make(map[string]bool, len(base))
	for _, tool := range base {
		available[tool] = true
	}
	for _, tool := range excludes {
		if !available[tool] {
			return nil, fmt.Errorf("%w: excludeTools contains unavailable tool %q", ErrInvalidConfig, tool)
		}
		delete(available, tool)
	}
	result := make([]string, 0, len(available))
	for tool := range available {
		result = append(result, tool)
	}
	return sortedStrings(result), nil
}

func hasArgument(arguments []string, target string) bool {
	for _, argument := range arguments {
		if argument == target {
			return true
		}
	}
	return false
}

func hasArgumentPrefix(arguments []string, prefix string) bool {
	for _, argument := range arguments {
		if strings.HasPrefix(argument, prefix) {
			return true
		}
	}
	return false
}

func rejected(code string) execution.PreflightReport {
	return execution.PreflightReport{Accepted: false, RejectionCode: code}
}

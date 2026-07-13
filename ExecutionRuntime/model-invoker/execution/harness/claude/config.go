package claude

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

type Config struct {
	Identity          union.VersionedIdentity
	Route             union.VersionedIdentity
	Process           harnessprocess.Config
	InitializeRequest json.RawMessage
	ExpectedInit      ExpectedInit
	ApprovalTTL       time.Duration
	Clock             func() time.Time
}

func (config Config) clone() Config {
	clone := config
	clone.Process = streamjson.CloneProcessConfig(config.Process)
	clone.InitializeRequest = append(json.RawMessage(nil), config.InitializeRequest...)
	clone.ExpectedInit.Tools = append([]string(nil), config.ExpectedInit.Tools...)
	clone.ExpectedInit.MCPServers = append([]string(nil), config.ExpectedInit.MCPServers...)
	return clone
}

func (config *Config) normalize() error {
	if err := config.Identity.Validate("claude.identity"); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if err := config.Route.Validate("claude.route"); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if config.Process.Protocol != harnessprocess.ProtocolJSONL || strings.TrimSpace(config.Process.ExpectedExecutableDigest) == "" {
		return fmt.Errorf("%w: jsonl and a pinned executable digest are required", ErrInvalidConfig)
	}
	for _, argument := range config.Process.Arguments {
		if argument == "--bare" {
			return fmt.Errorf("%w: Claude subscription/SDK Route cannot use --bare", ErrInvalidConfig)
		}
	}
	for name := range config.Process.Environment {
		upper := strings.ToUpper(name)
		if forbiddenClaudeEnvironment(upper) {
			return fmt.Errorf("%w: conflicting environment variable %s", ErrInvalidConfig, name)
		}
	}
	if strings.TrimSpace(config.ExpectedInit.Model) == "" || strings.TrimSpace(config.ExpectedInit.CWD) == "" ||
		strings.TrimSpace(config.ExpectedInit.CLIVersion) == "" || len(config.ExpectedInit.Tools) == 0 {
		return fmt.Errorf("%w: exact model, cwd, CLI version, and tools are required", ErrInvalidConfig)
	}
	config.ExpectedInit.Tools = sortedStrings(config.ExpectedInit.Tools)
	config.ExpectedInit.MCPServers = sortedStrings(config.ExpectedInit.MCPServers)
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
		config.InitializeRequest = json.RawMessage(`{"subtype":"initialize","hooks":null,"agents":{}}`)
	}
	var initialize map[string]json.RawMessage
	if json.Unmarshal(config.InitializeRequest, &initialize) != nil || objectString(config.InitializeRequest, "subtype") != "initialize" {
		return fmt.Errorf("%w: initialize request must be an object with subtype=initialize", ErrInvalidConfig)
	}
	return nil
}

func forbiddenClaudeEnvironment(name string) bool {
	if strings.Contains(name, "PROXY") || strings.HasPrefix(name, "AWS_") || strings.HasPrefix(name, "GOOGLE_") || strings.HasPrefix(name, "VERTEX_") {
		return true
	}
	switch name {
	case "ANTHROPIC_BASE_URL", "CLAUDE_CONFIG_DIR", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY":
		return true
	default:
		return false
	}
}

func rejected(code string) execution.PreflightReport {
	return execution.PreflightReport{Accepted: false, RejectionCode: code}
}

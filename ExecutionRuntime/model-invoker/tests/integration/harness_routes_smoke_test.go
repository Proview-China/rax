//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/acp"
	claudeharness "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/claude"
	codexharness "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/codexappserver"
	geminiharness "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/gemini"
	kimiharness "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/kimicode"
	harnessprocess "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/process"
	qwenharness "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/qwen"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

const harnessSmokeMarker = "praxis-harness-ok"

type liveHarnessCase struct {
	name      string
	prefix    string
	enableEnv string
	profileID profile.ProfileID
}

// TestOfficialHarnessRoutesLiveSmoke runs real official processes only after
// three exact confirmations. It never reads an API key or OAuth token. The
// child gets a complete, non-secret environment containing only an explicitly
// selected HOME plus fixed runtime variables, so the official component owns
// and reuses its own local login.
func TestOfficialHarnessRoutesLiveSmoke(t *testing.T) {
	if !officialHarnessGlobalGate(os.Getenv) {
		t.Skip("official Harness smoke requires exact global live and Harness confirmations")
	}
	now := time.Now().UTC()
	profiles, err := profile.RepresentativeProfiles(now)
	if err != nil {
		t.Fatalf("construct representative Profiles: %v", err)
	}
	profilesByID := make(map[profile.ProfileID]profile.SemanticRouteProfile, len(profiles))
	for _, candidate := range profiles {
		profilesByID[candidate.ID] = candidate
	}

	cases := []liveHarnessCase{
		{name: "codex_app_server", prefix: "PRAXIS_CODEX_HARNESS_", enableEnv: "PRAXIS_CODEX_HARNESS_LIVE", profileID: profile.ProfileCodex},
		{name: "claude_sdk_cli", prefix: "PRAXIS_CLAUDE_HARNESS_", enableEnv: "PRAXIS_CLAUDE_HARNESS_LIVE", profileID: profile.ProfileClaudeSDK},
		{name: "gemini_acp", prefix: "PRAXIS_GEMINI_HARNESS_", enableEnv: "PRAXIS_GEMINI_HARNESS_LIVE", profileID: profile.ProfileGeminiCLI},
		{name: "kimi_current_acp", prefix: "PRAXIS_KIMI_HARNESS_", enableEnv: "PRAXIS_KIMI_HARNESS_LIVE", profileID: profile.ProfileKimiCLI},
		{name: "qwen_sdk_cli", prefix: "PRAXIS_QWEN_HARNESS_", enableEnv: "PRAXIS_QWEN_HARNESS_LIVE", profileID: profile.ProfileQwenSDK},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if os.Getenv(test.enableEnv) != "confirmed" {
				t.Skip("route-specific official Harness confirmation is not exact")
			}
			selected, found := profilesByID[test.profileID]
			if !found {
				t.Fatal("selected representative Profile is unavailable")
			}
			input, err := loadCommonHarnessInput(test.prefix)
			if err != nil {
				t.Fatalf("load explicit Harness pins: %v", err)
			}
			if input.Model != selected.Selection.ModelID {
				t.Fatal("explicit model does not equal the exact model pinned by the selected Profile")
			}
			adapter, nativeExpected, err := buildOfficialHarnessAdapter(test, input, selected)
			if err != nil {
				t.Fatalf("construct real Harness Adapter: %v", err)
			}
			invocation, err := buildHarnessSmokeInvocation(now, selected, input)
			if err != nil {
				t.Fatalf("compile exact live invocation: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			report, err := adapter.Preflight(ctx, invocation)
			if err != nil {
				t.Fatalf("real Harness Preflight failed (%s); native error details are intentionally not printed", safeHarnessErrorCode(err))
			}
			prepared := true
			defer func() {
				if prepared {
					closePrepared(adapter, invocation.Request.ExecutionID)
				}
			}()
			if err := validateLivePreflight(invocation, report, input, nativeExpected); err != nil {
				t.Fatalf("real Harness manifest: %v", err)
			}
			session, err := adapter.Open(ctx, invocation)
			prepared = false
			if err != nil {
				t.Fatal("real Harness Open failed; native error details are intentionally not printed")
			}
			defer func() {
				if closeErr := session.Close(); closeErr != nil {
					t.Error("closing the real Harness session failed; native error details are intentionally not printed")
				}
			}()
			if err := consumeSideEffectFreeHarnessSmoke(ctx, session); err != nil {
				t.Fatalf("real Harness marker handshake: %v", err)
			}
		})
	}
}

func safeHarnessErrorCode(err error) string {
	known := []struct {
		code string
		err  error
	}{
		{"invalid_config", harnessprocess.ErrInvalidConfig},
		{"executable_not_runnable", harnessprocess.ErrExecutableNotRunnable},
		{"executable_digest_mismatch", harnessprocess.ErrExecutableDigestMismatch},
		{"working_directory_not_allowed", harnessprocess.ErrWorkingDirectoryNotAllowed},
		{"environment_not_allowed", harnessprocess.ErrEnvironmentNotAllowed},
		{"sensitive_environment", harnessprocess.ErrSensitiveEnvironment},
		{"unsafe_environment", harnessprocess.ErrUnsafeEnvironment},
		{"process_exit", harnessprocess.ErrProcessExit},
		{"stdout_limit", harnessprocess.ErrStdoutLimit},
		{"stderr_limit", harnessprocess.ErrStderrLimit},
		{"frame_too_large", harnessprocess.ErrFrameTooLarge},
		{"invalid_jsonrpc", harnessprocess.ErrInvalidJSONRPC},
		{"codex_invalid_config", codexharness.ErrInvalidConfig},
		{"codex_mapping", codexharness.ErrMapping},
		{"codex_protocol", codexharness.ErrProtocol},
		{"codex_rpc", codexharness.ErrRPC},
	}
	for _, candidate := range known {
		if errors.Is(err, candidate.err) {
			return candidate.code
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	return "unclassified"
}

type commonHarnessInput struct {
	Executable         string
	ResolvedExecutable string
	ExecutableSHA256   string
	CWD                string
	ResolvedCWD        string
	Home               string
	Model              string
	Version            string
	Arguments          []string
	ProxyEnvironment   map[string]string
}

var commonHarnessSuffixes = []string{"EXECUTABLE", "SHA256", "CWD", "HOME", "MODEL", "VERSION", "ARGS_JSON"}

func officialHarnessGlobalGate(getenv func(string) string) bool {
	return getenv("PRAXIS_LIVE_TESTS") == "1" && getenv("PRAXIS_HARNESS_PROBE") == "confirmed"
}

func missingCommonHarnessPins(getenv func(string) string, prefix string) []string {
	missing := make([]string, 0)
	for _, suffix := range commonHarnessSuffixes {
		name := prefix + suffix
		if strings.TrimSpace(getenv(name)) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

func loadCommonHarnessInput(prefix string) (commonHarnessInput, error) {
	if missing := missingCommonHarnessPins(os.Getenv, prefix); len(missing) != 0 {
		return commonHarnessInput{}, fmt.Errorf("required non-secret pin %s is missing", missing[0])
	}
	input := commonHarnessInput{
		Executable:       strings.TrimSpace(os.Getenv(prefix + "EXECUTABLE")),
		ExecutableSHA256: strings.TrimSpace(os.Getenv(prefix + "SHA256")),
		CWD:              strings.TrimSpace(os.Getenv(prefix + "CWD")),
		Home:             strings.TrimSpace(os.Getenv(prefix + "HOME")),
		Model:            strings.TrimSpace(os.Getenv(prefix + "MODEL")),
		Version:          strings.TrimSpace(os.Getenv(prefix + "VERSION")),
	}
	if !filepath.IsAbs(input.Executable) || !filepath.IsAbs(input.CWD) || !filepath.IsAbs(input.Home) {
		return commonHarnessInput{}, fmt.Errorf("executable, cwd and HOME must all be absolute")
	}
	if !canonicalSHA256(input.ExecutableSHA256) {
		return commonHarnessInput{}, fmt.Errorf("SHA256 must be canonical sha256:<lowercase hex>")
	}
	var err error
	input.ResolvedExecutable, err = filepath.EvalSymlinks(filepath.Clean(input.Executable))
	if err != nil {
		return commonHarnessInput{}, fmt.Errorf("resolve executable: %w", err)
	}
	info, err := os.Stat(input.ResolvedExecutable)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return commonHarnessInput{}, fmt.Errorf("executable is not a runnable regular file")
	}
	input.ResolvedCWD, err = filepath.EvalSymlinks(filepath.Clean(input.CWD))
	if err != nil {
		return commonHarnessInput{}, fmt.Errorf("resolve cwd: %w", err)
	}
	for _, directory := range []string{input.ResolvedCWD, input.Home} {
		info, statErr := os.Stat(directory)
		if statErr != nil || !info.IsDir() {
			return commonHarnessInput{}, fmt.Errorf("cwd and HOME must be existing directories")
		}
	}
	if err := decodeStrictJSON([]byte(os.Getenv(prefix+"ARGS_JSON")), &input.Arguments); err != nil || len(input.Arguments) == 0 {
		return commonHarnessInput{}, fmt.Errorf("ARGS_JSON must be a non-empty JSON string array")
	}
	for _, argument := range input.Arguments {
		if strings.IndexByte(argument, 0) >= 0 {
			return commonHarnessInput{}, fmt.Errorf("ARGS_JSON contains NUL")
		}
		if credentialLikeArgument(argument) {
			return commonHarnessInput{}, fmt.Errorf("ARGS_JSON must not contain credential or authentication arguments")
		}
	}
	input.ProxyEnvironment, err = loadExplicitProxyEnvironment(prefix + "PROXY_ENV_NAMES_JSON")
	if err != nil {
		return commonHarnessInput{}, err
	}
	return input, nil
}

func loadExplicitProxyEnvironment(name string) (map[string]string, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil, nil
	}
	var names []string
	if decodeStrictJSON([]byte(raw), &names) != nil || len(names) == 0 {
		return nil, fmt.Errorf("%s must be a non-empty JSON string array", name)
	}
	allowed := map[string]struct{}{
		"HTTP_PROXY": {}, "HTTPS_PROXY": {}, "ALL_PROXY": {}, "NO_PROXY": {},
		"http_proxy": {}, "https_proxy": {}, "all_proxy": {}, "no_proxy": {},
	}
	result := make(map[string]string, len(names))
	for _, proxyName := range names {
		if _, ok := allowed[proxyName]; !ok {
			return nil, fmt.Errorf("%s contains an unsupported proxy variable name", name)
		}
		value, ok := os.LookupEnv(proxyName)
		if !ok || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("%s requires every named proxy variable to be present", name)
		}
		result[proxyName] = value
	}
	return result, nil
}

func canonicalSHA256(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

func credentialLikeArgument(argument string) bool {
	name := argument
	if separator := strings.IndexAny(name, "=:"); separator >= 0 {
		name = name[:separator]
	}
	return credentialLikeName(name) || containsSecretLiteral(argument)
}

func credentialLikeName(name string) bool {
	var normalized strings.Builder
	for _, character := range strings.ToLower(name) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			normalized.WriteRune(character)
		}
	}
	value := normalized.String()
	if value == "oauth" || strings.Contains(value, "oauth") {
		return true
	}
	for _, suffix := range []string{
		"apikey", "accesstoken", "authtoken", "refreshtoken", "idtoken", "sessiontoken",
		"clientsecret", "secretkey", "password", "passwd", "cookie", "authorization", "credential", "credentials",
	} {
		if value == suffix || strings.HasSuffix(value, suffix) {
			return true
		}
	}
	return value == "secret" || value == "bearer"
}

func containsSecretLiteral(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"sk-", "ghp_", "github_pat_", "xoxb-", "xoxp-", "ya29."} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	fields := strings.FieldsFunc(value, func(character rune) bool {
		switch character {
		case ' ', '\t', '\r', '\n', ':', '=', ',', ';', '\'', '"':
			return true
		default:
			return false
		}
	})
	for index, field := range fields {
		if (strings.EqualFold(field, "bearer") || strings.EqualFold(field, "basic")) && index+1 < len(fields) && len(fields[index+1]) >= 8 {
			return true
		}
		trimmed := strings.Trim(field, "()[]{}<>")
		segments := strings.Split(trimmed, ".")
		if len(segments) == 3 && strings.HasPrefix(segments[0], "eyJ") && len(segments[0]) >= 8 && len(segments[1]) >= 8 && len(segments[2]) >= 8 {
			return true
		}
		if len(trimmed) == 20 && (strings.HasPrefix(trimmed, "AKIA") || strings.HasPrefix(trimmed, "ASIA")) {
			return true
		}
	}
	return false
}

func commonProcess(input commonHarnessInput, protocol harnessprocess.Protocol) harnessprocess.Config {
	environment := map[string]string{
		"HOME": input.Home, "PATH": "/usr/local/bin:/usr/bin:/bin", "LANG": "C.UTF-8",
		"LC_ALL": "C.UTF-8", "NO_COLOR": "1", "TERM": "dumb",
	}
	for name, value := range input.ProxyEnvironment {
		environment[name] = value
	}
	allowedEnvironment := make([]string, 0, len(environment))
	for name := range environment {
		allowedEnvironment = append(allowedEnvironment, name)
	}
	sort.Strings(allowedEnvironment)
	return harnessprocess.Config{
		Executable: input.Executable, ExpectedExecutableDigest: input.ExecutableSHA256,
		Arguments: input.Arguments, WorkingDirectory: input.CWD, AllowedWorkingDirectories: []string{input.CWD},
		Environment: environment, AllowedEnvironment: allowedEnvironment, Protocol: protocol,
		TerminationGrace: 2 * time.Second, KillWait: 2 * time.Second,
	}
}

func buildOfficialHarnessAdapter(test liveHarnessCase, input commonHarnessInput, selected profile.SemanticRouteProfile) (execution.Adapter, map[string]string, error) {
	route := union.VersionedIdentity{ID: string(selected.Selection.BaseRouteID), Version: selected.Selection.ModelRevision}
	switch test.profileID {
	case profile.ProfileCodex:
		if !containsArgument(input.Arguments, "app-server") {
			return nil, nil, fmt.Errorf("Codex ARGS_JSON must explicitly contain app-server")
		}
		adapter, err := codexharness.NewAdapter(codexharness.AdapterConfig{
			Identity: union.VersionedIdentity{ID: "live.codex.app-server", Version: "v1"},
			RouteID:  string(selected.Selection.BaseRouteID), Model: input.Model,
			Client: codexharness.Config{
				Process:      commonProcess(input, harnessprocess.ProtocolCodexAppServer),
				ClientInfo:   codexharness.ClientInfo{Name: "praxis-harness-smoke", Title: "Praxis Harness Smoke", Version: "v1"},
				Capabilities: json.RawMessage(`{}`),
			},
			ApprovalPolicy: "never", Sandbox: "read-only", ServiceName: "praxis-harness-smoke", Ephemeral: true,
		})
		return adapter, map[string]string{"native_surface/app_server_user_agent": input.Version}, err
	case profile.ProfileClaudeSDK:
		initialize, err := requiredJSONObject(test.prefix + "INITIALIZE_JSON")
		if err != nil {
			return nil, nil, err
		}
		var expected claudeExpectedInput
		if err := requiredStrictObject(test.prefix+"EXPECTED_INIT_JSON", &expected); err != nil {
			return nil, nil, err
		}
		if len(expected.Tools) == 0 || expected.PermissionMode == "" || expected.APIKeySource == "" {
			return nil, nil, fmt.Errorf("Claude EXPECTED_INIT_JSON requires tools, permission_mode and api_key_source")
		}
		adapter, err := claudeharness.New(claudeharness.Config{
			Identity: union.VersionedIdentity{ID: "live.claude.agent-sdk", Version: "v1"}, Route: route,
			Process: commonProcess(input, harnessprocess.ProtocolJSONL), InitializeRequest: initialize,
			ExpectedInit: claudeharness.ExpectedInit{
				Model: input.Model, CWD: input.ResolvedCWD, Tools: expected.Tools, MCPServers: expected.MCPServers,
				PermissionMode: expected.PermissionMode, CLIVersion: input.Version, APIKeySource: expected.APIKeySource,
			},
		})
		return adapter, map[string]string{
			"native_surface/system_init": input.Version, "native_surface/actual_model": input.Model,
			"native_surface/permission_mode": expected.PermissionMode, "native_surface/api_key_source": expected.APIKeySource,
		}, err
	case profile.ProfileGeminiCLI:
		base, expected, err := buildACPConfig(test, input, selected)
		if err != nil {
			return nil, nil, err
		}
		adapter, err := geminiharness.New(geminiharness.Config{ACP: base, FirstUserSessionContext: true})
		expected["native_surface/gemini_first_user_session_context"] = "tiered-context-v1"
		return adapter, expected, err
	case profile.ProfileKimiCLI:
		base, expected, err := buildACPConfig(test, input, selected)
		if err != nil {
			return nil, nil, err
		}
		adapter, err := kimiharness.New(kimiharness.Config{ACP: base, ProtocolGeneration: "current_acp"})
		expected["native_surface/kimi_current_acp"] = "current_acp"
		return adapter, expected, err
	case profile.ProfileQwenSDK:
		initialize, err := requiredJSONObject(test.prefix + "INITIALIZE_JSON")
		if err != nil {
			return nil, nil, err
		}
		var expected qwenExpectedInput
		if err := requiredStrictObject(test.prefix+"EXPECTED_INIT_JSON", &expected); err != nil {
			return nil, nil, err
		}
		if len(expected.Tools) == 0 || expected.PermissionMode == "" || expected.SurfaceMode == "" {
			return nil, nil, fmt.Errorf("Qwen EXPECTED_INIT_JSON requires tools, permission_mode and surface_mode")
		}
		adapter, err := qwenharness.New(qwenharness.Config{
			Identity: union.VersionedIdentity{ID: "live.qwen.code-sdk", Version: "v1"}, Route: route,
			Process: commonProcess(input, harnessprocess.ProtocolJSONL), InitializeRequest: initialize,
			ExpectedInit: qwenharness.ExpectedInit{
				Model: input.Model, CWD: input.ResolvedCWD, Tools: expected.Tools, MCPServers: expected.MCPServers,
				PermissionMode: expected.PermissionMode, QwenVersion: input.Version, Agents: expected.Agents, Skills: expected.Skills,
			},
			SurfaceMode: qwenharness.SurfaceMode(expected.SurfaceMode), CoreTools: expected.CoreTools, ExcludeTools: expected.ExcludeTools,
		})
		return adapter, map[string]string{
			"native_surface/sdk_system_init": input.Version, "native_surface/actual_model": input.Model,
			"native_surface/permission_mode": expected.PermissionMode,
		}, err
	default:
		return nil, nil, fmt.Errorf("unsupported live Harness Profile")
	}
}

type claudeExpectedInput struct {
	Tools          []string `json:"tools"`
	MCPServers     []string `json:"mcp_servers"`
	PermissionMode string   `json:"permission_mode"`
	APIKeySource   string   `json:"api_key_source"`
}

type qwenExpectedInput struct {
	Tools          []string `json:"tools"`
	MCPServers     []string `json:"mcp_servers"`
	PermissionMode string   `json:"permission_mode"`
	Agents         []string `json:"agents"`
	Skills         []string `json:"skills"`
	SurfaceMode    string   `json:"surface_mode"`
	CoreTools      []string `json:"core_tools"`
	ExcludeTools   []string `json:"exclude_tools"`
}

func buildACPConfig(test liveHarnessCase, input commonHarnessInput, selected profile.SemanticRouteProfile) (acp.AdapterConfig, map[string]string, error) {
	initialize, err := requiredJSONObject(test.prefix + "INITIALIZE_JSON")
	if err != nil {
		return acp.AdapterConfig{}, nil, err
	}
	sessionOptions, err := requiredJSONObject(test.prefix + "SESSION_JSON")
	if err != nil {
		return acp.AdapterConfig{}, nil, err
	}
	var session map[string]json.RawMessage
	if json.Unmarshal(sessionOptions, &session) != nil {
		return acp.AdapterConfig{}, nil, fmt.Errorf("SESSION_JSON is not an object")
	}
	var sessionModel string
	if json.Unmarshal(session["model"], &sessionModel) != nil || sessionModel != input.Model {
		return acp.AdapterConfig{}, nil, fmt.Errorf("SESSION_JSON must pin model to the selected Profile model")
	}
	agentName := strings.TrimSpace(os.Getenv(test.prefix + "AGENT_NAME"))
	if agentName == "" {
		return acp.AdapterConfig{}, nil, fmt.Errorf("required non-secret AGENT_NAME is missing")
	}
	config := acp.AdapterConfig{
		Identity: union.VersionedIdentity{ID: "live." + strings.ReplaceAll(test.name, "_", "."), Version: "v1"},
		RouteID:  string(selected.Selection.BaseRouteID), ExpectedAgentName: agentName,
		Client:         acp.Config{Process: commonProcess(input, harnessprocess.ProtocolJSONRPCNDJSON), InitializeParams: initialize},
		SessionOptions: sessionOptions,
	}
	return config, map[string]string{"native_surface/agent_identity": agentName + "@" + input.Version}, nil
}

func requiredJSONObject(name string) (json.RawMessage, error) {
	raw := []byte(strings.TrimSpace(os.Getenv(name)))
	if len(raw) == 0 {
		return nil, fmt.Errorf("required non-secret %s is missing", name)
	}
	var object map[string]json.RawMessage
	if err := decodeStrictJSON(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be exactly one JSON object", name)
	}
	if containsCredentialMaterial(object) {
		return nil, fmt.Errorf("%s must not contain credential material", name)
	}
	return append(json.RawMessage(nil), raw...), nil
}

func containsCredentialMaterial(value any) bool {
	switch typed := value.(type) {
	case map[string]json.RawMessage:
		for key, raw := range typed {
			if credentialLikeName(key) {
				return true
			}
			var nested any
			if json.Unmarshal(raw, &nested) == nil && containsCredentialMaterial(nested) {
				return true
			}
		}
	case map[string]any:
		for key, nested := range typed {
			raw, _ := json.Marshal(map[string]any{key: nested})
			var object map[string]json.RawMessage
			_ = json.Unmarshal(raw, &object)
			if containsCredentialMaterial(object) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if containsCredentialMaterial(nested) {
				return true
			}
		}
	case string:
		return containsSecretLiteral(strings.TrimSpace(typed))
	}
	return false
}

func requiredStrictObject(name string, target any) error {
	raw := []byte(strings.TrimSpace(os.Getenv(name)))
	if len(raw) == 0 {
		return fmt.Errorf("required non-secret %s is missing", name)
	}
	if err := decodeStrictJSON(raw, target); err != nil {
		return fmt.Errorf("%s is invalid", name)
	}
	return nil
}

func decodeStrictJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("multiple JSON values")
	}
	return nil
}

func containsArgument(arguments []string, wanted string) bool {
	for _, argument := range arguments {
		if argument == wanted {
			return true
		}
	}
	return false
}

func buildHarnessSmokeInvocation(now time.Time, selected profile.SemanticRouteProfile, input commonHarnessInput) (execution.Invocation, error) {
	registry, err := profile.NewRegistry(now, selected)
	if err != nil {
		return execution.Invocation{}, err
	}
	compiler, err := profile.NewCompiler(registry, now)
	if err != nil {
		return execution.Invocation{}, err
	}
	request := union.UnifiedExecutionRequest{
		SemanticVersion: union.SemanticVersionV1,
		ExecutionID:     union.ExecutionID("live.harness." + string(selected.ID)),
		ProfileSelector: union.ProfileSelector{Exact: &union.VersionedIdentity{ID: string(selected.ID), Version: string(selected.Version)}},
		ExecutionKind:   union.ExecutionKindAuto,
		Input: []union.InputItem{{
			ID: "live.harness.input", Kind: "message", Role: "user",
			Content: []union.ContentPart{{Kind: "text", Text: `Return only this exact JSON object and do not use any tool: {"marker":"praxis-harness-ok"}`}},
		}},
		ToolPolicy: union.ToolPolicy{DefaultApproval: "never", Parallelism: 1},
		OutputContract: union.OutputContract{
			AcceptedContentKinds: []string{"json"}, CompletionMode: "final",
		},
		SessionIntent: union.SessionIntent{Mode: "new"},
		ExecutionPolicy: union.ExecutionPolicy{
			Sandbox: "read_only", CWDReference: input.CWD, NetworkPolicy: "denied",
			UserPresence: "present", Foreground: "required", InteractionMode: "interactive", MaxConcurrency: 1,
		},
		Budget:            union.Budget{MaxOutputTokens: 64, MaxWallTime: 90 * time.Second},
		DegradationPolicy: union.DegradationPolicy{Default: union.DegradationDefaultReject, RequirePreflightAck: true},
		IntentGraph: union.IntentGraph{Nodes: []union.IntentNode{{
			ID: "live-harness-structured", Kind: union.IntentProduceStructured, Target: "live_harness_marker", Required: true,
			Postconditions:   []union.Condition{{Kind: "json_schema_valid"}},
			AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact, union.SemanticFidelityTransformed},
		}}},
		Metadata: map[string]string{"exact_model": input.Model, "harness_version": input.Version},
	}
	compiled, err := compiler.Compile(profile.CompileInput{
		Request: request, PaperOnly: true,
		ActualManifest: profile.InjectionManifest{SchemaVersion: string(selected.Version), ProbeStatus: profile.ManifestProbeNotRun},
	})
	if err != nil {
		return execution.Invocation{}, err
	}
	return execution.NewInvocation(request, compiled.Plan)
}

func validateLivePreflight(invocation execution.Invocation, report execution.PreflightReport, input commonHarnessInput, nativeExpected map[string]string) error {
	if err := report.Validate(); err != nil {
		return err
	}
	if !report.Accepted {
		return fmt.Errorf("Adapter rejected the exact live route before Open")
	}
	if err := execution.CompareContextManifests(invocation.Plan.ExpectedManifest, report.ActualManifest); err != nil {
		return err
	}
	expectedArguments, _ := json.Marshal(input.Arguments)
	environmentNames := []string{"HOME", "LANG", "LC_ALL", "NO_COLOR", "PATH", "TERM"}
	for name := range input.ProxyEnvironment {
		environmentNames = append(environmentNames, name)
	}
	sort.Strings(environmentNames)
	expectedEnvironment, _ := json.Marshal(environmentNames)
	expected := map[string]string{
		"launch_probe/actual_executable":     input.ResolvedExecutable,
		"launch_probe/executable_pin":        input.ExecutableSHA256,
		"launch_probe/actual_argv":           string(expectedArguments),
		"launch_probe/sanitized_environment": string(expectedEnvironment),
		"launch_probe/actual_cwd":            input.ResolvedCWD,
	}
	for key, value := range nativeExpected {
		expected[key] = value
	}
	components := make(map[string]union.ManifestComponent, len(report.ActualManifest.Components))
	for _, component := range report.ActualManifest.Components {
		components[component.Kind+"/"+component.Name] = component
	}
	for key, wanted := range expected {
		component, found := components[key]
		if !found || component.Version != wanted {
			return fmt.Errorf("required exact manifest component %s is missing or drifted", key)
		}
	}
	if components["launch_probe/actual_executable"].Digest != input.ExecutableSHA256 ||
		components["launch_probe/executable_pin"].Digest != input.ExecutableSHA256 {
		return fmt.Errorf("live executable digest differs from the explicit SHA-256 pin")
	}
	return nil
}

func closePrepared(adapter execution.Adapter, executionID union.ExecutionID) {
	if cleaner, ok := adapter.(execution.PreflightCleaner); ok {
		_ = cleaner.ClosePrepared(executionID)
	}
}

func consumeSideEffectFreeHarnessSmoke(ctx context.Context, session execution.Session) error {
	var deltas, completed strings.Builder
	terminal := false
	for eventCount := 0; eventCount < 10_000; eventCount++ {
		event, err := session.Receive(ctx)
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("Harness ended before a terminal marker")
			}
			return fmt.Errorf("Harness receive failed; native error details are intentionally not retained")
		}
		if event.Control != nil && event.Control.Kind == execution.ControlApprovalRequested {
			return fmt.Errorf("Harness requested a tool approval during the side-effect-free smoke")
		}
		if event.Item != nil && event.Item.Item.SideEffectState != union.SideEffectNone {
			return fmt.Errorf("Harness attempted a tool or side effect during the marker smoke")
		}
		if event.Mechanism != nil && event.Mechanism.Attempt != nil {
			state := event.Mechanism.Attempt.SideEffectState
			if state != union.SideEffectNone {
				return fmt.Errorf("Harness mechanism reported a non-empty side-effect state")
			}
		}
		if event.Diagnostic != nil && event.Diagnostic.Kind == "native_error" {
			if codexNativeErrorWillRetry(event.Diagnostic.Payload) {
				continue
			}
			return fmt.Errorf("Harness emitted a native error (%s)", safeCodexNativeErrorCode(event.Diagnostic.Payload))
		}
		if event.Model != nil {
			target := &completed
			switch event.Model.Kind {
			case "content_delta", "agent_message_chunk", "agent_message_delta":
				target = &deltas
			case "content_completed":
			default:
				target = nil
			}
			if target != nil {
				for _, part := range event.Model.Content {
					if part.Kind == "text" {
						target.WriteString(part.Text)
					}
				}
			}
		}
		if event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate {
			var candidate execution.RouteTerminalCandidate
			if json.Unmarshal(event.Diagnostic.Payload, &candidate) != nil || candidate.Status != union.ExecutionStatusSucceeded || candidate.SideEffectState != union.SideEffectNone {
				return fmt.Errorf("Harness terminal is not a side-effect-free success")
			}
			terminal = true
			break
		}
		if event.Lifecycle != nil && event.Lifecycle.Status != "" {
			if event.Lifecycle.Status != union.ExecutionStatusSucceeded {
				return fmt.Errorf("Harness lifecycle terminal is not successful")
			}
			terminal = true
			break
		}
	}
	if !terminal {
		return fmt.Errorf("Harness exceeded the bounded event count without a terminal")
	}
	output := deltas.String()
	if output == "" {
		output = completed.String()
	}
	if !hasExactHarnessSmokeMarker(output) {
		return fmt.Errorf("Harness output did not contain the exact reviewed JSON marker")
	}
	return nil
}

func safeCodexNativeErrorCode(raw json.RawMessage) string {
	var notification struct {
		Error struct {
			CodexErrorInfo json.RawMessage `json:"codexErrorInfo"`
		} `json:"error"`
		WillRetry bool `json:"willRetry"`
	}
	if json.Unmarshal(raw, &notification) != nil || len(notification.Error.CodexErrorInfo) == 0 || string(notification.Error.CodexErrorInfo) == "null" {
		return "unclassified"
	}
	var scalar string
	if json.Unmarshal(notification.Error.CodexErrorInfo, &scalar) == nil {
		switch scalar {
		case "contextWindowExceeded", "sessionBudgetExceeded", "usageLimitExceeded", "serverOverloaded", "cyberPolicy", "internalServerError", "unauthorized", "badRequest", "threadRollbackFailed", "sandboxError", "other":
			return scalar
		default:
			return "unclassified"
		}
	}
	var variant map[string]json.RawMessage
	if json.Unmarshal(notification.Error.CodexErrorInfo, &variant) != nil || len(variant) != 1 {
		return "unclassified"
	}
	for key := range variant {
		switch key {
		case "httpConnectionFailed", "responseStreamConnectionFailed", "responseStreamDisconnected", "responseTooManyFailedAttempts", "activeTurnNotSteerable":
			return key
		}
	}
	return "unclassified"
}

func codexNativeErrorWillRetry(raw json.RawMessage) bool {
	var notification struct {
		WillRetry bool `json:"willRetry"`
	}
	return json.Unmarshal(raw, &notification) == nil && notification.WillRetry
}

func hasExactHarnessSmokeMarker(text string) bool {
	var object map[string]json.RawMessage
	if decodeStrictJSON([]byte(strings.TrimSpace(text)), &object) != nil || len(object) != 1 {
		return false
	}
	var marker string
	return json.Unmarshal(object["marker"], &marker) == nil && marker == harnessSmokeMarker
}

func TestOfficialHarnessSmokeGlobalGateIsExact(t *testing.T) {
	for _, values := range []map[string]string{
		{},
		{"PRAXIS_LIVE_TESTS": "1"},
		{"PRAXIS_HARNESS_PROBE": "confirmed"},
		{"PRAXIS_LIVE_TESTS": "true", "PRAXIS_HARNESS_PROBE": "confirmed"},
		{"PRAXIS_LIVE_TESTS": "1", "PRAXIS_HARNESS_PROBE": "1"},
	} {
		if officialHarnessGlobalGate(func(name string) string { return values[name] }) {
			t.Fatalf("incomplete or non-exact live gate was accepted")
		}
	}
	valid := map[string]string{"PRAXIS_LIVE_TESTS": "1", "PRAXIS_HARNESS_PROBE": "confirmed"}
	if !officialHarnessGlobalGate(func(name string) string { return valid[name] }) {
		t.Fatal("exact live Harness gate was rejected")
	}
}

func TestSafeCodexNativeErrorCodeDoesNotExposeMessage(t *testing.T) {
	secret := "sensitive-upstream-message"
	raw := json.RawMessage(`{"error":{"message":"` + secret + `","codexErrorInfo":{"httpConnectionFailed":{"httpStatusCode":null}}},"willRetry":false}`)
	code := safeCodexNativeErrorCode(raw)
	if code != "httpConnectionFailed" {
		t.Fatalf("unexpected safe error code: %q", code)
	}
	if strings.Contains(code, secret) {
		t.Fatal("safe error code leaked the native message")
	}
	retryRaw := json.RawMessage(`{"error":{"message":"secret","codexErrorInfo":{"responseStreamDisconnected":{"httpStatusCode":null}}},"willRetry":true}`)
	if !codexNativeErrorWillRetry(retryRaw) || codexNativeErrorWillRetry(json.RawMessage(`not-json`)) {
		t.Fatal("Codex native retry signal was not parsed conservatively")
	}
	for _, candidate := range []json.RawMessage{
		json.RawMessage(`{"error":{"message":"secret","codexErrorInfo":"usageLimitExceeded"},"willRetry":false}`),
		json.RawMessage(`{"error":{"message":"secret","codexErrorInfo":"futureError"},"willRetry":false}`),
		json.RawMessage(`{"error":{"message":"secret","codexErrorInfo":null},"willRetry":false}`),
		json.RawMessage(`not-json`),
	} {
		result := safeCodexNativeErrorCode(candidate)
		if strings.Contains(result, "secret") {
			t.Fatal("safe error classification leaked a native message")
		}
	}
}

func TestExplicitProxyEnvironmentRequiresNamedSafeVariables(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://proxy-user:proxy-secret@example.invalid:8080")
	t.Setenv("PROXY_NAMES", `["HTTPS_PROXY"]`)
	values, err := loadExplicitProxyEnvironment("PROXY_NAMES")
	if err != nil || len(values) != 1 || values["HTTPS_PROXY"] == "" {
		t.Fatalf("explicit proxy environment was not loaded: size=%d err=%v", len(values), err)
	}
	t.Setenv("PROXY_NAMES", `["OPENAI_API_KEY"]`)
	if _, err := loadExplicitProxyEnvironment("PROXY_NAMES"); err == nil || strings.Contains(err.Error(), "proxy-secret") {
		t.Fatal("unsupported proxy name was accepted or a proxy value leaked")
	}
	t.Setenv("ALL_PROXY", "")
	t.Setenv("PROXY_NAMES", `["ALL_PROXY"]`)
	if _, err := loadExplicitProxyEnvironment("PROXY_NAMES"); err == nil {
		t.Fatal("missing explicitly named proxy variable was accepted")
	}
}

func TestOfficialHarnessSmokeRequiresEveryCommonPin(t *testing.T) {
	values := make(map[string]string)
	for _, suffix := range commonHarnessSuffixes {
		values["P_"+suffix] = "set"
	}
	if missing := missingCommonHarnessPins(func(name string) string { return values[name] }, "P_"); len(missing) != 0 {
		t.Fatalf("complete pins reported missing values")
	}
	for _, suffix := range commonHarnessSuffixes {
		delete(values, "P_"+suffix)
		if missing := missingCommonHarnessPins(func(name string) string { return values[name] }, "P_"); len(missing) == 0 {
			t.Fatalf("missing %s was accepted", suffix)
		}
		values["P_"+suffix] = "set"
	}
}

func TestOfficialHarnessSmokeMarkerIsExact(t *testing.T) {
	for _, value := range []string{
		"", harnessSmokeMarker, `{"marker":"wrong"}`, `{"marker":"praxis-harness-ok","extra":true}`,
		"prefix " + `{"marker":"praxis-harness-ok"}`,
	} {
		if hasExactHarnessSmokeMarker(value) {
			t.Fatalf("non-exact Harness marker was accepted")
		}
	}
	if !hasExactHarnessSmokeMarker(" \n" + `{"marker":"praxis-harness-ok"}` + "\t") {
		t.Fatal("exact Harness JSON marker was rejected")
	}
}

func TestOfficialHarnessSmokeRejectsCredentialArgumentsAndJSON(t *testing.T) {
	for _, argument := range []string{
		"--api-key=value", "--oauth-token", "Authorization: Bearer long-secret-value", "--password=value",
		"--header=x-praxis: sk-proj-example-secret", "--header=Authorization: Basic dXNlcjpwYXNz",
		"--header=x-session: eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJwcmF4aXMifQ.signature123",
		"--header=x-aws: AKIAIOSFODNN7EXAMPLE",
	} {
		if !credentialLikeArgument(argument) {
			t.Fatal("credential-like Harness argument was not rejected")
		}
	}
	for _, argument := range []string{"--max-output-tokens=2048", "--model=token-budget-v1", "--header=x-mode: basic compact"} {
		if credentialLikeArgument(argument) {
			t.Fatal("non-secret token-budget or mode argument was rejected")
		}
	}
	for _, raw := range []string{
		`{"api_key":"value"}`, `{"headers":{"Authorization":"Bearer long-secret-value"}}`, `{"nested":[{"oauth_token":"value"}]}`,
		`{"headers":{"x-praxis":"sk-proj-example-secret"}}`, `{"headers":{"x-session":"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJwcmF4aXMifQ.signature123"}}`,
	} {
		var object map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &object) != nil || !containsCredentialMaterial(object) {
			t.Fatal("credential-like Harness JSON was not rejected")
		}
	}
	var safe map[string]json.RawMessage
	_ = json.Unmarshal([]byte(`{"model":"token-budget-v1","limits":{"token_budget":4096},"clientInfo":{"name":"praxis","version":"v1"}}`), &safe)
	if containsCredentialMaterial(safe) {
		t.Fatal("non-secret Harness JSON was rejected")
	}
}

// This guard reaches every real Adapter constructor with non-secret pinned
// fixtures. It does not start a process; route-local black-box suites cover the
// protocol processes, while TestOfficialHarnessRoutesLiveSmoke owns real Open.
func TestOfficialHarnessSmokeFactoriesUseRealAdapters(t *testing.T) {
	now := time.Date(2026, 7, 13, 4, 0, 0, 0, time.UTC)
	profiles, err := profile.RepresentativeProfiles(now)
	if err != nil {
		t.Fatal(err)
	}
	profilesByID := make(map[profile.ProfileID]profile.SemanticRouteProfile, len(profiles))
	for _, selected := range profiles {
		profilesByID[selected.ID] = selected
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	base := commonHarnessInput{
		Executable: executable, ResolvedExecutable: executable,
		ExecutableSHA256: "sha256:" + strings.Repeat("0", 64),
		CWD:              directory, ResolvedCWD: directory, Home: directory, Version: "test-version",
	}

	t.Setenv("PRAXIS_CLAUDE_HARNESS_INITIALIZE_JSON", `{"subtype":"initialize","hooks":null,"agents":{}}`)
	t.Setenv("PRAXIS_CLAUDE_HARNESS_EXPECTED_INIT_JSON", `{"tools":["Bash","Edit","Read","Write"],"mcp_servers":[],"permission_mode":"default","api_key_source":"none"}`)
	t.Setenv("PRAXIS_GEMINI_HARNESS_INITIALIZE_JSON", `{"protocolVersion":1,"clientCapabilities":{},"clientInfo":{"name":"praxis","version":"v1"}}`)
	t.Setenv("PRAXIS_GEMINI_HARNESS_SESSION_JSON", `{"model":"gemini-3.5-flash","mcpServers":[]}`)
	t.Setenv("PRAXIS_GEMINI_HARNESS_AGENT_NAME", "gemini-cli")
	t.Setenv("PRAXIS_KIMI_HARNESS_INITIALIZE_JSON", `{"protocolVersion":1,"clientCapabilities":{},"clientInfo":{"name":"praxis","version":"v1"}}`)
	t.Setenv("PRAXIS_KIMI_HARNESS_SESSION_JSON", `{"model":"kimi-for-coding","mcpServers":[]}`)
	t.Setenv("PRAXIS_KIMI_HARNESS_AGENT_NAME", "kimi-code")
	t.Setenv("PRAXIS_QWEN_HARNESS_INITIALIZE_JSON", `{"subtype":"initialize","hooks":null,"mcpServers":{},"agents":[]}`)
	t.Setenv("PRAXIS_QWEN_HARNESS_EXPECTED_INIT_JSON", `{"tools":["edit","notebook_edit","read_file","run_shell_command"],"mcp_servers":[],"permission_mode":"default","agents":[],"skills":[],"surface_mode":"bare_fixed","core_tools":[],"exclude_tools":[]}`)

	cases := []struct {
		test liveHarnessCase
		args []string
	}{
		{liveHarnessCase{name: "codex_app_server", prefix: "PRAXIS_CODEX_HARNESS_", profileID: profile.ProfileCodex}, []string{"app-server"}},
		{liveHarnessCase{name: "claude_sdk_cli", prefix: "PRAXIS_CLAUDE_HARNESS_", profileID: profile.ProfileClaudeSDK}, []string{"--output-format", "stream-json"}},
		{liveHarnessCase{name: "gemini_acp", prefix: "PRAXIS_GEMINI_HARNESS_", profileID: profile.ProfileGeminiCLI}, []string{"--acp"}},
		{liveHarnessCase{name: "kimi_current_acp", prefix: "PRAXIS_KIMI_HARNESS_", profileID: profile.ProfileKimiCLI}, []string{"acp"}},
		{liveHarnessCase{name: "qwen_sdk_cli", prefix: "PRAXIS_QWEN_HARNESS_", profileID: profile.ProfileQwenSDK}, []string{"--bare"}},
	}
	for _, test := range cases {
		selected := profilesByID[test.test.profileID]
		input := base
		input.Model = selected.Selection.ModelID
		input.Arguments = test.args
		adapter, _, err := buildOfficialHarnessAdapter(test.test, input, selected)
		if err != nil {
			t.Fatalf("%s real Adapter constructor: %v", test.test.name, err)
		}
		descriptor, err := adapter.Describe(context.Background())
		if err != nil || !descriptor.Supports(union.ExecutionKindAgent) || descriptor.Origin != union.EventOriginHarness {
			t.Fatalf("%s descriptor is not an Agent Harness", test.test.name)
		}
	}
}

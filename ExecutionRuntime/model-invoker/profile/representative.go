package profile

import (
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

const (
	ProfileOpenAIDirect ProfileID = "openai.responses.direct.semantic-stable"
	ProfileCodex        ProfileID = "openai.codex.app-server.semantic-stable"
	ProfileClaudeSDK    ProfileID = "anthropic.claude.agent-sdk.semantic-stable"
	ProfileGeminiCLI    ProfileID = "google.gemini.cli-acp.semantic-stable"
	ProfileKimiCLI      ProfileID = "kimi.code.cli-acp.current.semantic-stable"
	ProfileQwenSDK      ProfileID = "alibaba.qwen.code-sdk.semantic-stable"
)

func RepresentativeProfiles(now time.Time) ([]SemanticRouteProfile, error) {
	if now.IsZero() {
		return nil, fmt.Errorf("representative Profile time is required")
	}
	profiles := []SemanticRouteProfile{
		representativeOpenAIDirect(now),
		representativeCodex(now),
		representativeClaude(now),
		representativeGemini(now),
		representativeKimi(now),
		representativeQwen(now),
	}
	for index := range profiles {
		if err := profiles[index].Validate(now); err != nil {
			return nil, fmt.Errorf("representative Profile %q: %w", profiles[index].ID, err)
		}
	}
	return profiles, nil
}

func representativeOpenAIDirect(now time.Time) SemanticRouteProfile {
	selection := ProfileSelectionKey{
		BaseRouteID: "openai.direct.payg.responses", Provider: "openai", ModelID: "gpt-5.6",
		ModelRevision: "gpt-5.6-2026-07-12", Deployment: "openai.platform.global", Region: "global",
		EndpointIdentity: "openai.platform.responses", Protocol: upstream.ProtocolResponses,
		ProtocolSchemaVersion: "responses-2026-07-12", Offering: "openai.platform.payg",
		AuthRoute: "platform_api_key_reference", ExecutionSurface: ExecutionSurfaceDirectAPI,
	}
	mechanisms := []MechanismCapability{
		mechanism("openai.caller.apply_patch", "filesystem_patch", []union.IntentKind{union.IntentModifyFile, union.IntentCreateFile}, union.CapabilityOriginCallerHosted, union.ExecutionOwnerPraxis, union.SemanticFidelityExact, 98, true, "file_changed", "file_created"),
		mechanism("openai.caller.write_file", "filesystem_write", []union.IntentKind{union.IntentCreateFile, union.IntentRewriteFile}, union.CapabilityOriginCallerHosted, union.ExecutionOwnerPraxis, union.SemanticFidelityExact, 82, true, "file_created", "file_rewritten"),
		mechanism("openai.caller.delete_path", "filesystem_delete", []union.IntentKind{union.IntentDeleteFile, union.IntentDeleteDirectory}, union.CapabilityOriginCallerHosted, union.ExecutionOwnerPraxis, union.SemanticFidelityExact, 90, false, "file_deleted", "directory_deleted"),
		mechanism("openai.caller.move_path", "filesystem_move", []union.IntentKind{union.IntentMoveFile}, union.CapabilityOriginCallerHosted, union.ExecutionOwnerPraxis, union.SemanticFidelityExact, 90, false, "file_moved"),
		mechanism("openai.caller.directory", "filesystem_directory", []union.IntentKind{union.IntentCreateDirectory}, union.CapabilityOriginCallerHosted, union.ExecutionOwnerPraxis, union.SemanticFidelityExact, 90, true, "directory_created"),
		mechanism("openai.caller.function", "tool_call", []union.IntentKind{union.IntentCallTool}, union.CapabilityOriginCallerHosted, union.ExecutionOwnerPraxis, union.SemanticFidelityExact, 96, true, "tool_call_completed"),
		mechanism("openai.caller.process", "process_exec", []union.IntentKind{union.IntentExecuteCode}, union.CapabilityOriginCallerHosted, union.ExecutionOwnerPraxis, union.SemanticFidelityExact, 95, false, "code_execution_completed"),
		mechanism("openai.responses.json_schema", "structured_output", []union.IntentKind{union.IntentProduceStructured}, union.CapabilityOriginNative, union.ExecutionOwnerProvider, union.SemanticFidelityExact, 100, true, "structured_output_produced"),
		mechanism("praxis.schema.repair", "structured_output_repair", []union.IntentKind{union.IntentProduceStructured}, union.CapabilityOriginEmulated, union.ExecutionOwnerPraxis, union.SemanticFidelityTransformed, 70, true, "structured_output_produced"),
	}
	result := buildRepresentative(ProfileOpenAIDirect, selection, "gpt", "direct", mechanisms, now, nil, nil, nil)
	result.ModelBehavior.Evidence[0].Attribution = AttributionProviderProtocol
	return result
}

func representativeCodex(now time.Time) SemanticRouteProfile {
	stack := []HarnessComponent{harnessComponent("codex-app-server", "v2", "/usr/bin/codex")}
	selection := ProfileSelectionKey{
		BaseRouteID: "openai.chatgpt.codex.app-server", Provider: "openai", ModelID: "gpt-5.6-sol",
		ModelRevision: "gpt-5.6-sol-codex-2026-07-13", Deployment: "codex_managed", Region: "local",
		EndpointIdentity: "codex.app-server.local", Protocol: "app_server_v2",
		ProtocolSchemaVersion: "app-server-v2-2026-07-13", Offering: "openai.chatgpt.codex.subscription",
		AuthRoute: "codex_official_managed_login", ExecutionSurface: ExecutionSurfaceAppServer, HarnessStack: stack,
	}
	mechanisms := []MechanismCapability{
		mechanism("codex.apply_patch", "filesystem_patch", []union.IntentKind{union.IntentModifyFile, union.IntentCreateFile}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 100, true, "file_changed", "file_created"),
		mechanism("codex.write_file", "filesystem_write", []union.IntentKind{union.IntentCreateFile, union.IntentRewriteFile}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 78, true, "file_created", "file_rewritten"),
		mechanism("codex.shell", "process_exec", []union.IntentKind{union.IntentExecuteCode}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 96, false, "code_execution_completed"),
		mechanism("codex.shell.fs_admin", "filesystem_shell", []union.IntentKind{union.IntentDeleteFile, union.IntentMoveFile, union.IntentCreateDirectory, union.IntentDeleteDirectory}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 88, false, "file_deleted", "file_moved", "directory_created", "directory_deleted"),
		mechanism("codex.dynamic_tool", "tool_call", []union.IntentKind{union.IntentCallTool}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 96, false, "tool_call_completed"),
		mechanism("codex.output_schema", "structured_output", []union.IntentKind{union.IntentProduceStructured}, union.CapabilityOriginNative, union.ExecutionOwnerProvider, union.SemanticFidelityExact, 100, false, "structured_output_produced"),
	}
	return buildRepresentative(ProfileCodex, selection, "gpt", "codex", mechanisms, now, nil, nil, nil)
}

func representativeClaude(now time.Time) SemanticRouteProfile {
	stack := []HarnessComponent{
		harnessComponent("claude-agent-sdk", "v1", "/opt/praxis/claude-agent-sdk"),
		harnessComponent("claude-code-cli", "2.1.181", "/usr/bin/claude"),
	}
	selection := ProfileSelectionKey{
		BaseRouteID: "anthropic.consumer.agent-sdk", Provider: "anthropic", ModelID: "claude-fable-5",
		ModelRevision: "claude-fable-5-2026-07-12", Deployment: "claude_code_managed", Region: "local",
		EndpointIdentity: "claude.code.local", Protocol: "agent_sdk_stream_json",
		ProtocolSchemaVersion: "agent-sdk-v1-2026-07-12", Offering: "anthropic.claude.subscription",
		AuthRoute: "official_local_claude_login", ExecutionSurface: ExecutionSurfaceAgentSDK, HarnessStack: stack,
	}
	mechanisms := []MechanismCapability{
		mechanism("claude.edit", "filesystem_edit", []union.IntentKind{union.IntentModifyFile}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 100, false, "file_changed"),
		mechanism("claude.write", "filesystem_write", []union.IntentKind{union.IntentCreateFile, union.IntentRewriteFile}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 100, false, "file_created", "file_rewritten"),
		mechanism("claude.bash", "process_exec", []union.IntentKind{union.IntentExecuteCode}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 94, false, "code_execution_completed"),
		mechanism("claude.bash.fs_admin", "filesystem_shell", []union.IntentKind{union.IntentDeleteFile, union.IntentMoveFile, union.IntentCreateDirectory, union.IntentDeleteDirectory}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 88, false, "file_deleted", "file_moved", "directory_created", "directory_deleted"),
		mechanism("claude.tool_use", "tool_call", []union.IntentKind{union.IntentCallTool}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 96, false, "tool_call_completed"),
		mechanism("claude.output_format", "structured_output", []union.IntentKind{union.IntentProduceStructured}, union.CapabilityOriginNative, union.ExecutionOwnerProvider, union.SemanticFidelityExact, 98, true, "structured_output_produced"),
		mechanism("praxis.schema.repair", "structured_output_repair", []union.IntentKind{union.IntentProduceStructured}, union.CapabilityOriginEmulated, union.ExecutionOwnerPraxis, union.SemanticFidelityTransformed, 70, true, "structured_output_produced"),
	}
	return buildRepresentative(ProfileClaudeSDK, selection, "claude", "claude-code", mechanisms, now, nil, nil, nil)
}

func representativeGemini(now time.Time) SemanticRouteProfile {
	stack := []HarnessComponent{
		harnessComponent("gemini-cli", "v1", "/usr/bin/gemini"),
		harnessComponent("acp-schema", "v1", "/opt/praxis/schemas/acp"),
	}
	selection := ProfileSelectionKey{
		BaseRouteID: "google.gemini-code-assist.cli-acp", Provider: "google.gemini-code-assist", ModelID: "gemini-3.5-flash",
		ModelRevision: "gemini-3.5-flash-2026-07-12", Deployment: "google.code-assist.managed", Region: "global",
		EndpointIdentity: "gemini.cli.local", Protocol: "acp_jsonrpc",
		ProtocolSchemaVersion: "acp-v1-2026-07-12", Offering: "google.code-assist.subscription",
		AuthRoute: "official_google_login", ExecutionSurface: ExecutionSurfaceOfficialCLI, HarnessStack: stack,
	}
	mechanisms := []MechanismCapability{
		mechanism("gemini.replace", "filesystem_replace", []union.IntentKind{union.IntentModifyFile}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 100, true, "file_changed"),
		mechanism("gemini.write_file", "filesystem_write", []union.IntentKind{union.IntentCreateFile, union.IntentRewriteFile}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 94, true, "file_created", "file_rewritten"),
		mechanism("gemini.run_shell_command", "process_exec", []union.IntentKind{union.IntentExecuteCode}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 94, false, "code_execution_completed"),
		mechanism("gemini.run_shell_command.fs_admin", "filesystem_shell", []union.IntentKind{union.IntentDeleteFile, union.IntentMoveFile, union.IntentCreateDirectory, union.IntentDeleteDirectory}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 86, false, "file_deleted", "file_moved", "directory_created", "directory_deleted"),
		mechanism("gemini.tool_call", "tool_call", []union.IntentKind{union.IntentCallTool}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 94, false, "tool_call_completed"),
		mechanism("praxis.gemini.schema_repair", "structured_output_repair", []union.IntentKind{union.IntentProduceStructured}, union.CapabilityOriginEmulated, union.ExecutionOwnerPraxis, union.SemanticFidelityTransformed, 86, false, "structured_output_produced"),
	}
	return buildRepresentative(ProfileGeminiCLI, selection, "gemini", "gemini-cli", mechanisms, now, nil, nil, nil)
}

func representativeKimi(now time.Time) SemanticRouteProfile {
	stack := []HarnessComponent{
		harnessComponent("kimi-code-cli", "current", "/usr/bin/kimi"),
		harnessComponent("kimi-acp-adapter", "0.23", "/opt/praxis/kimi-acp"),
		harnessComponent("kimi-embedded-sdk", "current", "/opt/praxis/kimi-sdk"),
	}
	selection := ProfileSelectionKey{
		BaseRouteID: "kimi.code-membership.acp", Provider: "kimi", ModelID: "kimi-for-coding",
		ModelRevision: "kimi-code-current-2026-07-12", Deployment: "kimi_code_managed", Region: "global",
		EndpointIdentity: "kimi.code.local", Protocol: "acp_0.23",
		ProtocolSchemaVersion: "acp-0.23", Offering: "kimi.code-membership",
		AuthRoute: "official_device_oauth", ExecutionSurface: ExecutionSurfaceOfficialCLI, HarnessStack: stack,
	}
	mechanisms := []MechanismCapability{
		mechanism("kimi.edit", "filesystem_edit", []union.IntentKind{union.IntentModifyFile}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 100, true, "file_changed"),
		mechanism("kimi.write", "filesystem_write", []union.IntentKind{union.IntentCreateFile, union.IntentRewriteFile}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 96, true, "file_created", "file_rewritten"),
		mechanism("kimi.shell", "process_exec", []union.IntentKind{union.IntentExecuteCode}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 90, false, "code_execution_completed"),
		mechanism("kimi.shell.fs_admin", "filesystem_shell", []union.IntentKind{union.IntentDeleteFile, union.IntentMoveFile, union.IntentCreateDirectory, union.IntentDeleteDirectory}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 84, false, "file_deleted", "file_moved", "directory_created", "directory_deleted"),
		mechanism("kimi.tool_call", "tool_call", []union.IntentKind{union.IntentCallTool}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 92, false, "tool_call_completed"),
		mechanism("praxis.kimi.schema_repair", "structured_output_repair", []union.IntentKind{union.IntentProduceStructured}, union.CapabilityOriginEmulated, union.ExecutionOwnerPraxis, union.SemanticFidelityTransformed, 84, false, "structured_output_produced"),
	}
	return buildRepresentative(
		ProfileKimiCLI, selection, "kimi", "kimi-code", mechanisms, now,
		[]string{"current_acp", "edit", "write"},
		[]string{"legacy_wire", "agent_file", "str_replace_file"},
		nil,
	)
}

func representativeQwen(now time.Time) SemanticRouteProfile {
	stack := []HarnessComponent{
		harnessComponent("qwen-code-sdk", "v1", "/opt/praxis/qwen-code-sdk"),
		harnessComponent("qwen-code-cli", "v1", "/usr/bin/qwen"),
	}
	selection := ProfileSelectionKey{
		BaseRouteID: "alibaba.coding-plan.qwen-code.sdk", Provider: "alibaba.model-studio", ModelID: "qwen3.7-max",
		ModelRevision: "qwen3.7-max-2026-07-12", Deployment: "alibaba.coding-plan.cn", Region: "cn",
		EndpointIdentity: "qwen.code.local", Protocol: "qwen_sdk_stream_json",
		ProtocolSchemaVersion: "qwen-sdk-v1-2026-07-12", Offering: "alibaba.coding-plan",
		AuthRoute: "qwen_actual_auth_type", ExecutionSurface: ExecutionSurfaceOfficialSDK, HarnessStack: stack,
	}
	mechanisms := []MechanismCapability{
		mechanism("qwen.edit", "filesystem_edit", []union.IntentKind{union.IntentModifyFile}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 100, false, "file_changed"),
		mechanism("qwen.write", "filesystem_write", []union.IntentKind{union.IntentCreateFile, union.IntentRewriteFile}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 92, false, "file_created", "file_rewritten"),
		mechanism("qwen.bash", "process_exec", []union.IntentKind{union.IntentExecuteCode}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 90, false, "code_execution_completed"),
		mechanism("qwen.bash.fs_admin", "filesystem_shell", []union.IntentKind{union.IntentDeleteFile, union.IntentMoveFile, union.IntentCreateDirectory, union.IntentDeleteDirectory}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 84, false, "file_deleted", "file_moved", "directory_created", "directory_deleted"),
		mechanism("qwen.tool_call", "tool_call", []union.IntentKind{union.IntentCallTool}, union.CapabilityOriginHarnessHosted, union.ExecutionOwnerHarness, union.SemanticFidelityExact, 92, false, "tool_call_completed"),
		mechanism("praxis.qwen.schema_repair", "structured_output_repair", []union.IntentKind{union.IntentProduceStructured}, union.CapabilityOriginEmulated, union.ExecutionOwnerPraxis, union.SemanticFidelityTransformed, 82, false, "structured_output_produced"),
	}
	return buildRepresentative(
		ProfileQwenSDK, selection, "qwen", "qwen-code", mechanisms, now,
		[]string{"bare", "core_tools", "controlled_nonbare"}, nil,
		[]FeatureConflict{{Left: "bare", Right: "core_tools"}},
	)
}

func buildRepresentative(
	id ProfileID,
	selection ProfileSelectionKey,
	modelFamily, harnessName string,
	mechanisms []MechanismCapability,
	now time.Time,
	supportedFeatures, forbiddenFeatures []string,
	conflicts []FeatureConflict,
) SemanticRouteProfile {
	evidenceID := string(id) + ".official-behavior"
	preferences := make([]MechanismPreference, 0, len(mechanisms))
	for _, candidate := range mechanisms {
		for _, intentKind := range candidate.IntentKinds {
			preferences = append(preferences, MechanismPreference{
				IntentKind: intentKind, MechanismID: candidate.ID,
				Rank: maxPreferenceRank(100 - candidate.Score.ModelAffinity), EvidenceID: evidenceID,
			})
		}
	}
	manifest := representativeExpectedManifest(selection, mechanisms)
	policy := representativeRuntimePolicy(id, selection, mechanisms)
	return SemanticRouteProfile{
		ID: id, Version: "v1candidate", Selection: selection,
		ModelBehavior: ModelBehaviorProfile{
			ID: ProfileID(string(id) + ".model"), Version: "v1candidate", ModelFamily: modelFamily,
			ExactModelIDs: []string{selection.ModelID},
			Evidence: []BehaviorEvidence{{
				ID: evidenceID, Reference: "official:" + string(id), Attribution: AttributionOfficialHarness,
				CheckedAt: now.Add(-time.Hour), ValidUntil: now.Add(30 * 24 * time.Hour),
				Digest: DigestString("evidence:" + string(id)),
			}},
			MechanismPreferences: preferences,
			KnownFailurePatterns: []string{"harness_or_provider_behavior_must_be_verified"},
			BenchmarkDigest:      DigestString("benchmark:" + string(id)),
		},
		HarnessCapability: HarnessCapabilityProfile{
			ID: ProfileID(string(id) + ".harness"), Version: "v1candidate",
			ExecutionSurface: selection.ExecutionSurface, HarnessName: harnessName,
			HarnessStack: selection.HarnessStack, ContextMode: ContextSemanticStable,
			Transparency: "manifest_bounded", InstructionControl: "semantic_bridge",
			ContextControl: "fresh_workspace", EventFidelity: "profile_declared",
			ExpectedManifest: manifest, AvailableMechanisms: mechanisms,
			SupportedNativeFeatures: supportedFeatures, ForbiddenNativeFeatures: forbiddenFeatures,
			FeatureConflicts:           conflicts,
			OpaqueFields:               []string{"provider.hidden_policy"},
			Controls:                   ControlCapabilities{Approval: true, Cancel: true, ProvideToolResult: true},
			ProbeDigest:                DigestString("probe:" + string(id)),
			SanitizedEnvironmentDigest: DigestString("environment:" + string(id)),
		},
		DefaultPolicy: policy, SemanticCodecVersion: "v1candidate", ContextMode: ContextSemanticStable,
	}
}

func representativeExpectedManifest(selection ProfileSelectionKey, mechanisms []MechanismCapability) InjectionManifest {
	fields := []ManifestField{
		{Path: "identity.provider", State: ManifestFieldPresent, Value: string(selection.Provider)},
		{Path: "identity.model", State: ManifestFieldPresent, Value: selection.ModelID},
		{Path: "auth.route", State: ManifestFieldPresent, Value: selection.AuthRoute},
		{Path: "sandbox.mode", State: ManifestFieldPresent, Value: "workspace_scoped"},
		{Path: "instructions.semantic_profile", State: ManifestFieldPresent, Value: "minimal_bridge"},
		{Path: "context.sources.workspace", State: ManifestFieldPresent, Value: "/workspace"},
		{Path: "event.fidelity", State: ManifestFieldPresent, Value: "profile_declared"},
	}
	for _, candidate := range mechanisms {
		fields = append(fields,
			ManifestField{Path: "tools." + candidate.ID + ".registered", State: ManifestFieldPresent, Value: "true"},
			ManifestField{Path: "tools." + candidate.ID + ".executable", State: ManifestFieldPresent, Value: "true"},
			ManifestField{Path: "tools." + candidate.ID + ".execution_owner", State: ManifestFieldPresent, Value: string(candidate.Owner)},
			ManifestField{Path: "tools." + candidate.ID + ".schema_digest", State: ManifestFieldPresent, Value: DigestString("schema:" + candidate.ID)},
		)
	}
	return InjectionManifest{SchemaVersion: "v1candidate", ProbeStatus: ManifestProbeNotRun, Fields: fields}
}

func representativeRuntimePolicy(id ProfileID, selection ProfileSelectionKey, mechanisms []MechanismCapability) RuntimePolicy {
	mechanismIDs := make([]string, 0, len(mechanisms))
	fallbackIDs := make([]string, 0, len(mechanisms))
	for _, candidate := range mechanisms {
		mechanismIDs = append(mechanismIDs, candidate.ID)
		if candidate.FallbackSafe {
			fallbackIDs = append(fallbackIDs, candidate.ID)
		}
	}
	return RuntimePolicy{
		ID: ProfileID(string(id) + ".policy"), Version: "v1candidate",
		Identity: IdentityLock{
			BaseRouteID: selection.BaseRouteID, Provider: selection.Provider,
			Offering: selection.Offering, ModelID: selection.ModelID,
		},
		AllowedIntentKinds: IntentSetConstraint{Specified: true, Values: []union.IntentKind{
			union.IntentCreateFile, union.IntentModifyFile, union.IntentRewriteFile, union.IntentDeleteFile,
			union.IntentMoveFile, union.IntentCreateDirectory, union.IntentDeleteDirectory,
			union.IntentProduceStructured, union.IntentCallTool, union.IntentExecuteCode,
		}},
		AllowedMechanismIDs:    StringSetConstraint{Specified: true, Values: mechanismIDs},
		AllowedP2ManifestPaths: StringSetConstraint{Specified: true},
		MaxWallTime:            120 * time.Second, MaxActions: 8, MaxFallbacks: 1, MaxConcurrency: 1,
		Verification: VerificationStrong,
		Filesystem: FilesystemPolicy{
			ReadableRoots: PathSetConstraint{Specified: true, Values: []string{"/workspace"}},
			WritablePaths: PathSetConstraint{Specified: true, Values: []string{
				"/workspace/internal/config/config.go",
				"/workspace/internal/config/config_test.go",
			}},
			DeniedPaths: []string{"/workspace/.git"}, FollowSymlinks: false,
			MaxFileSize: 128 * 1024, MaxTotalDelta: 64 * 1024,
			AllowDelete: false, AllowMove: false, RequireAtomicWrite: true,
		},
		Process: ProcessPolicy{
			AllowedArgv:   ArgvSetConstraint{Specified: true, Values: [][]string{{"go", "test", "./internal/config"}}},
			AllowedCWDs:   PathSetConstraint{Specified: true, Values: []string{"/workspace"}},
			NetworkAccess: NetworkDenied, MaxTimeout: 120 * time.Second, AllowShellMeta: false,
		},
		Network: NetworkPolicy{
			Mode: NetworkDenied, AllowedHosts: StringSetConstraint{Specified: true}, AllowDNS: false,
		},
		Computer: ComputerPolicy{
			Enabled: false, AllowedOrigins: StringSetConstraint{Specified: true},
			RequireExternalStateEvidence: true,
		},
		Approval: ApprovalPolicy{
			Strength:                        ApprovalOnSideEffect,
			PreauthorizedExactActionIDs:     []string{"i1", "i2", "i3", "i4"},
			ChangedRevisionRequiresApproval: true, UnknownDecisionIsDeny: true,
		},
		Secret: SecretPolicy{
			AllowedReferenceStores: StringSetConstraint{Specified: true, Values: []string{"credential_store"}},
			DeniedEnvironmentNames: []string{"PLAINTEXT_SECRET"}, ForbidPlaintext: true, RequireRedaction: true,
		},
		RetryFallback: RetryFallbackPolicy{
			MaxAttempts: 1, MaxFallbacks: 1,
			AllowedFallbackMechanismIDs: StringSetConstraint{Specified: true, Values: fallbackIDs},
			RequireSideEffectStateNone:  true, RequireReconcileBeforeFallback: true,
			AllowModelSwitch: false, AllowRouteSwitch: false,
		},
		AllowLegacyCompatibility:  false,
		MechanismPreferenceWeight: map[string]int{},
	}
}

func mechanism(
	id, kind string,
	intentKinds []union.IntentKind,
	origin union.CapabilityOrigin,
	owner union.ExecutionOwner,
	fidelity union.SemanticFidelity,
	affinity int,
	fallbackSafe bool,
	effects ...string,
) MechanismCapability {
	return MechanismCapability{
		ID: id, Kind: kind, IntentKinds: intentKinds, Origin: origin, Owner: owner,
		SelectionAuthority: union.SelectionAuthorityRuntime, Fidelity: fidelity,
		ModelAddressable: true, EffectObservable: true, VerifierAvailable: true,
		ApprovalSupported: true, Cancellable: true, Idempotent: false, FallbackSafe: fallbackSafe,
		Capabilities: []string{kind}, HardConstraints: []string{"runtime_policy", "effect_verification"},
		ExpectedEffects: effects,
		Score: MechanismScore{
			ModelAffinity: affinity, SemanticFidelity: 100, EffectObservability: 100,
			VerificationStrength: 100, Determinism: 90, Efficiency: 80,
			TransformationCost: 10, OperationalRisk: 10, FallbackRisk: 10,
		},
	}
}

func harnessComponent(name, version, executable string) HarnessComponent {
	return HarnessComponent{
		Component: name, Version: version, ExecutablePath: executable,
		BinaryDigest:         DigestString("binary:" + name + ":" + version),
		ProtocolSchemaDigest: DigestString("protocol:" + name + ":" + version),
	}
}

func maxPreferenceRank(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

package qwen

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/streamjson"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func buildActualManifest(expected union.ContextManifestSummary, init InitMessage, evidence streamjson.LaunchEvidence) (union.ContextManifestSummary, error) {
	initDigest, err := union.StableDigest(init.Raw)
	if err != nil {
		return union.ContextManifestSummary{}, err
	}
	evidenceDigest, err := evidence.Digest()
	if err != nil {
		return union.ContextManifestSummary{}, err
	}
	manifest, err := expected.Clone()
	if err != nil {
		return union.ContextManifestSummary{}, err
	}
	manifest.ID = expected.ID + ".actual.qwen"
	manifest.Digest = ""
	argumentsJSON, _ := json.Marshal(evidence.Arguments)
	environmentNamesJSON, _ := json.Marshal(evidence.EnvironmentNames)
	manifest.Components = append(manifest.Components,
		union.ManifestComponent{Kind: "launch_probe", Name: "actual_executable", State: "observed", Version: evidence.ActualExecutablePath, Digest: evidence.ActualExecutableDigest, Owner: union.ExecutionOwnerHarness, Executable: true},
		union.ManifestComponent{Kind: "launch_probe", Name: "executable_pin", State: "observed", Version: evidence.ExpectedExecutableDigest, Digest: evidence.ActualExecutableDigest, Owner: union.ExecutionOwnerHarness},
		union.ManifestComponent{Kind: "launch_probe", Name: "actual_argv", State: "observed", Version: string(argumentsJSON), Digest: evidence.ArgumentsDigest, Owner: union.ExecutionOwnerHarness},
		union.ManifestComponent{Kind: "launch_probe", Name: "sanitized_environment", State: "observed", Version: string(environmentNamesJSON), Digest: evidence.EnvironmentDigest, Owner: union.ExecutionOwnerHarness},
		union.ManifestComponent{Kind: "launch_probe", Name: "actual_cwd", State: "observed", Digest: evidenceDigest, Version: evidence.WorkingDirectory, Owner: union.ExecutionOwnerHarness},
		union.ManifestComponent{Kind: "native_surface", Name: "sdk_system_init", State: "reported", Digest: initDigest, Version: init.EffectiveVersion(), Owner: union.ExecutionOwnerHarness, ModelVisible: true},
		union.ManifestComponent{Kind: "native_surface", Name: "actual_model", State: "reported", Digest: initDigest, Version: init.Model, Owner: union.ExecutionOwnerProvider, ModelVisible: true},
		union.ManifestComponent{Kind: "native_surface", Name: "permission_mode", State: "reported", Digest: initDigest, Version: init.EffectivePermissionMode(), Owner: union.ExecutionOwnerHarness},
		union.ManifestComponent{Kind: "native_surface", Name: "agents", State: "reported", Digest: initDigest, Version: fmt.Sprint(sortedStrings(init.Agents)), Owner: union.ExecutionOwnerHarness},
		union.ManifestComponent{Kind: "native_surface", Name: "skills", State: "reported", Digest: initDigest, Version: fmt.Sprint(sortedStrings(init.Skills)), Owner: union.ExecutionOwnerHarness},
	)
	for _, opaque := range []string{
		"instructions.harness_internal_loop", "instructions.startup_date", "instructions.os",
		"instructions.directory_tree", "instructions.git_status",
	} {
		manifest.OpaqueFields = appendUnique(manifest.OpaqueFields, opaque)
	}
	tools := sortedStrings(init.Tools)
	for _, name := range tools {
		observed := union.ToolSurfaceEntry{
			ID: "native_surface:" + name, NativeName: name, Discovered: true, Registered: true, ModelVisible: true, Executable: true,
			PermissionMode: init.EffectivePermissionMode(), Owner: union.ExecutionOwnerHarness,
			Probe: union.ToolSurfaceProbe{Status: union.ToolProbeReported, EvidenceDigest: initDigest},
		}
		manifest.Tools.Entries = append(manifest.Tools.Entries, observed)
	}
	sort.Slice(manifest.Tools.Entries, func(i, j int) bool { return manifest.Tools.Entries[i].ID < manifest.Tools.Entries[j].ID })
	if err := manifest.Validate(); err != nil {
		return union.ContextManifestSummary{}, fmt.Errorf("%w: actual manifest: %v", ErrProtocol, err)
	}
	digest, err := manifest.ComputeDigest()
	if err != nil {
		return union.ContextManifestSummary{}, err
	}
	manifest.Digest = digest
	return manifest, nil
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

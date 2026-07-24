package dataplaneadapter

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"strings"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestDispatchRequestCanonicalSealAndDrift(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	request := fixtureRequest(t, now)
	if err := request.ValidateCurrent(now); err != nil {
		t.Fatal(err)
	}
	request.ExecutionBinding.FenceEpoch++
	if err := request.ValidateCurrent(now); err == nil {
		t.Fatal("fence drift retained the old request digest")
	}
}

func TestProviderPayloadMustExactlyEqualRuntimePermitBinding(t *testing.T) {
	digest := runtimecore.Digest(digestForTest(t, "permitted-payload"))
	schema := runtimeports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "provider-payload", Version: "1.0.0", MediaType: "application/json", ContentDigest: runtimecore.Digest(digestForTest(t, "provider-schema"))}
	if err := validateProviderPayloadBinding(schema, digest, 3, schema.Key(), string(digest), 3); err != nil {
		t.Fatal(err)
	}
	for name, values := range map[string]struct {
		schema   string
		digest   string
		revision uint64
	}{
		"schema":   {"praxis.sandbox/other", string(digest), 3},
		"digest":   {schema.Key(), digestForTest(t, "changed-payload"), 3},
		"revision": {schema.Key(), string(digest), 4},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateProviderPayloadBinding(schema, digest, 3, values.schema, values.digest, values.revision); err == nil {
				t.Fatal("post-Permit provider payload drift was accepted")
			}
		})
	}
}

func TestWorkspaceCommitPayloadIsCanonicalScopedAndExact(t *testing.T) {
	expires := time.Unix(1_800_000_000, 0).Add(time.Hour).UnixNano()
	value := WorkspaceCommitPayloadV1{
		WorkspaceBindingID: "workspace-1", WorkspaceDigest: digestForTest(t, "workspace-binding"),
		ChangeSet:    ExactRefV1{ID: "changes-1", Revision: 1, Digest: digestForTest(t, "changes"), ExpiresUnixNano: expires},
		View:         ExactRefV1{ID: "view-1", Revision: 1, Digest: digestForTest(t, "view"), ExpiresUnixNano: expires},
		BaseRevision: digestForTest(t, "base"), FileScopeDigest: digestForTest(t, "scope"),
		WriteScopes: []string{"src/generated"},
		Changes:     []WorkspaceMutationV1{{Kind: "add", Path: "src/generated/a.go", BlobID: "workspace-blob-" + strings.Repeat("a", 64), BlobDigest: digestForTest(t, "blob"), Mode: 0o600}},
	}
	payload, err := NewWorkspaceCommitPayload(value)
	if err != nil || payload.ProviderKind != "workspace_commit" {
		t.Fatalf("workspace payload: %#v %v", payload, err)
	}
	for name, mutate := range map[string]func(*WorkspaceCommitPayloadV1){
		"escape":          func(v *WorkspaceCommitPayloadV1) { v.Changes[0].Path = "../escape" },
		"scope":           func(v *WorkspaceCommitPayloadV1) { v.Changes[0].Path = "src/other" },
		"duplicate scope": func(v *WorkspaceCommitPayloadV1) { v.WriteScopes = append(v.WriteScopes, v.WriteScopes[0]) },
		"digest":          func(v *WorkspaceCommitPayloadV1) { v.Changes[0].BlobDigest = "sha256:bad" },
	} {
		t.Run(name, func(t *testing.T) {
			drift := value
			drift.WriteScopes = append([]string(nil), value.WriteScopes...)
			drift.Changes = append([]WorkspaceMutationV1(nil), value.Changes...)
			mutate(&drift)
			if _, err := NewWorkspaceCommitPayload(drift); err == nil {
				t.Fatal("invalid workspace payload was accepted")
			}
		})
	}
}

func TestWorkspaceCleanupAloneMayBindExactOriginalCommitTarget(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	expires := now.Add(time.Hour).UnixNano()
	value := WorkspaceCommitPayloadV1{
		WorkspaceBindingID: "workspace-1", WorkspaceDigest: digestForTest(t, "workspace-binding-cleanup"),
		ChangeSet:    ExactRefV1{ID: "changes-cleanup", Revision: 1, Digest: digestForTest(t, "changes-cleanup"), ExpiresUnixNano: expires},
		View:         ExactRefV1{ID: "view-cleanup", Revision: 1, Digest: digestForTest(t, "view-cleanup"), ExpiresUnixNano: expires},
		BaseRevision: digestForTest(t, "base-cleanup"), FileScopeDigest: digestForTest(t, "scope-cleanup"), WriteScopes: []string{"src"},
		Changes: []WorkspaceMutationV1{{Kind: "delete", Path: "src/old.go"}},
		InspectionTarget: &ProviderInspectionTargetV1{
			OriginalEffectKind: "praxis.sandbox/workspace-commit", OriginalAttemptID: "commit-attempt-1",
			ProviderAttempt:       ExactRefV1{ID: "workspace-commit/tenant-1/commit-attempt-1", Revision: 2, Digest: digestForTest(t, "provider-attempt-cleanup"), ExpiresUnixNano: expires},
			OriginalRequestDigest: digestForTest(t, "request-cleanup"), OriginalPayloadDigest: digestForTest(t, "payload-cleanup"),
		},
	}
	payload, err := NewWorkspaceCommitPayload(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateInspectionTarget("praxis.sandbox/cleanup", "tenant-1", payload, now); err != nil {
		t.Fatalf("governed workspace cleanup target rejected: %v", err)
	}
	if err := validateInspectionTarget("praxis.sandbox/open", "tenant-1", payload, now); err == nil {
		t.Fatal("workspace inspection target escaped into another effect")
	}
}

func TestRuntimeQueryCanonicalizationDoesNotDependOnObjectKeyOrder(t *testing.T) {
	first, err := canonicalJSON([]byte(`{"z":1,"a":{"y":2,"b":3}}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := canonicalJSON([]byte(`{"a":{"b":3,"y":2},"z":1}`))
	if err != nil {
		t.Fatal(err)
	}
	firstDigest, err := canonicalDigest("RuntimeCurrentQueryV1", json.RawMessage(first))
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := canonicalDigest("RuntimeCurrentQueryV1", json.RawMessage(second))
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest != secondDigest {
		t.Fatal("semantically equal current queries produced different digests")
	}
}

func TestCurrentServerRejectsMalformedRequestBeforeOwnerReads(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	request := fixtureRequest(t, now)
	request.Digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	server := CurrentServer{Now: func() time.Time { return now }}
	if _, err := server.inspect(context.Background(), request); err == nil {
		t.Fatal("malformed request reached owner current readers")
	}
}

func TestProviderBindingDigestMatchesRustWireGolden(t *testing.T) {
	data, err := os.ReadFile("../protocol/v1/golden/provider-binding-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Kind           string            `json:"kind"`
		Canonical      ProviderBindingV1 `json:"canonical"`
		ExpectedDigest string            `json:"expected_digest"`
	}
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	digest, err := canonicalDigest(golden.Kind, golden.Canonical)
	if err != nil {
		t.Fatal(err)
	}
	if digest != golden.ExpectedDigest {
		t.Fatalf("Go digest %s differs from shared golden %s", digest, golden.ExpectedDigest)
	}
}

func TestResponseRejectsSelfConsistentForgedProviderAttempt(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	request := fixtureRequest(t, now)
	attempt := ExactRefV1{ID: "wasmtime/tenant-1/attempt-1", Revision: 1, ExpiresUnixNano: request.RequestedNotAfterUnixNano}
	var err error
	attempt.Digest, err = canonicalDigest("ProviderAttemptRefV1", attempt)
	if err != nil {
		t.Fatal(err)
	}
	observation := ProviderObservationV1{Provider: "wasmtime_component", Attempt: attempt, State: "prepared", PayloadDigest: request.PayloadDigest, ObservedUnixNano: now.UnixNano()}
	observation.Digest, err = canonicalDigest("ProviderObservationV1", observation)
	if err != nil {
		t.Fatal(err)
	}
	receipt := ProviderReceiptV1{Provider: "wasmtime_component", Attempt: attempt, Phase: "prepare", ObservationDigest: observation.Digest, RecordedUnixNano: now.UnixNano(), ExpiresUnixNano: attempt.ExpiresUnixNano}
	receipt.Digest, err = canonicalDigest("ProviderReceiptV1", receipt)
	if err != nil {
		t.Fatal(err)
	}
	response := DispatchResponseV1{
		ContractVersion: ContractVersionV1, RequestID: request.RequestID, RequestDigest: request.Digest,
		Accepted: true, ProviderAttempt: &attempt, ProviderObservation: &observation, ProviderReceipt: &receipt,
		ObservationDigest: &observation.Digest, ReceiptDigest: &receipt.Digest,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: attempt.ExpiresUnixNano,
	}
	response.Digest, err = canonicalDigest("DispatchResponseV1", response)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Validate(request); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}

	forged := digestForTest(t, "forged-attempt")
	response.ProviderAttempt.Digest = forged
	response.ProviderObservation.Attempt = *response.ProviderAttempt
	response.ProviderObservation.Digest = ""
	response.ProviderObservation.Digest, err = canonicalDigest("ProviderObservationV1", *response.ProviderObservation)
	if err != nil {
		t.Fatal(err)
	}
	response.ProviderReceipt.Attempt = *response.ProviderAttempt
	response.ProviderReceipt.ObservationDigest = response.ProviderObservation.Digest
	response.ProviderReceipt.Digest = ""
	response.ProviderReceipt.Digest, err = canonicalDigest("ProviderReceiptV1", *response.ProviderReceipt)
	if err != nil {
		t.Fatal(err)
	}
	*response.ObservationDigest = response.ProviderObservation.Digest
	*response.ReceiptDigest = response.ProviderReceipt.Digest
	response.Digest = ""
	response.Digest, err = canonicalDigest("DispatchResponseV1", response)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Validate(request); err == nil {
		t.Fatal("self-consistent forged provider attempt digest was accepted")
	}
}

func TestInspectionPayloadBindsExactOriginalProviderAttempt(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	request := inspectionRequestFixture(t, now)
	if err := request.ValidateCurrent(now); err != nil {
		t.Fatal(err)
	}

	var payload WasmPayloadV1
	if err := json.Unmarshal(request.Payload.ProviderPayload, &payload); err != nil {
		t.Fatal(err)
	}
	payload.InspectionTarget.OriginalPayloadDigest = digestForTest(t, "another-payload")
	providerPayload, err := NewWasmPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	request.Payload = providerPayload
	request.PayloadDigest, err = canonicalDigest("ProviderPayloadV1", request.Payload)
	if err != nil {
		t.Fatal(err)
	}
	request.Digest, err = request.digestV1()
	if err != nil {
		t.Fatal(err)
	}
	if err := request.ValidateCurrent(now); err != nil {
		t.Fatalf("a newly governed exact target is structurally valid before Provider comparison: %v", err)
	}

	request.EffectKind = "praxis.sandbox/open"
	request.Digest, err = request.digestV1()
	if err != nil {
		t.Fatal(err)
	}
	if err := request.ValidateCurrent(now); err == nil {
		t.Fatal("non-inspection dispatch accepted an inspection target")
	}
}

func TestInspectionPayloadRejectsAttemptIdentityOrTTLDrift(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for name, mutate := range map[string]func(*ProviderInspectionTargetV1){
		"provider attempt": func(target *ProviderInspectionTargetV1) {
			target.ProviderAttempt.ID = "wasmtime/tenant-1/another-attempt"
		},
		"attempt revision": func(target *ProviderInspectionTargetV1) { target.ProviderAttempt.Revision = 1 },
		"expired":          func(target *ProviderInspectionTargetV1) { target.ProviderAttempt.ExpiresUnixNano = now.UnixNano() },
		"effect":           func(target *ProviderInspectionTargetV1) { target.OriginalEffectKind = CheckpointEffectKindV1 },
	} {
		t.Run(name, func(t *testing.T) {
			request := inspectionRequestFixture(t, now)
			var payload WasmPayloadV1
			if err := json.Unmarshal(request.Payload.ProviderPayload, &payload); err != nil {
				t.Fatal(err)
			}
			mutate(payload.InspectionTarget)
			providerPayload, err := NewWasmPayload(payload)
			if err != nil {
				t.Fatal(err)
			}
			request.Payload = providerPayload
			request.PayloadDigest, err = canonicalDigest("ProviderPayloadV1", request.Payload)
			if err != nil {
				t.Fatal(err)
			}
			request.Digest, err = request.digestV1()
			if err != nil {
				t.Fatal(err)
			}
			if err := request.ValidateCurrent(now); err == nil {
				t.Fatal("drifted inspection target was accepted")
			}
		})
	}
}

func TestHostWorkspacePayloadIsStrictAndPathFree(t *testing.T) {
	valid := HostWorkspacePayloadV1{
		WorkspaceBindingID: "workspace-1", WorkspaceDigest: digestForTest(t, "workspace"),
		ToolBindingID: "tool-1", ToolDigest: digestForTest(t, "tool"),
		Argv: []string{"-c", "true"}, Environment: map[string]string{"LANG": "C"},
		WorkingDirectory: "work", NetworkDenyAll: true, WallClockTimeoutMilli: 1000,
	}
	payload, err := NewHostWorkspacePayload(valid)
	if err != nil {
		t.Fatal(err)
	}
	if payload.ProviderKind != "host_workspace" || strings.Contains(string(payload.ProviderPayload), "/home/") {
		t.Fatal("host payload did not remain an opaque binding-only request")
	}
	for name, mutate := range map[string]func(*HostWorkspacePayloadV1){
		"absolute path": func(value *HostWorkspacePayloadV1) { value.WorkingDirectory = "/tmp" },
		"parent escape": func(value *HostWorkspacePayloadV1) { value.WorkingDirectory = "work/../secret" },
		"network":       func(value *HostWorkspacePayloadV1) { value.NetworkDenyAll = false },
		"ttl":           func(value *HostWorkspacePayloadV1) { value.WallClockTimeoutMilli = 0 },
		"nul":           func(value *HostWorkspacePayloadV1) { value.Argv = []string{"bad\x00arg"} },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.Argv = append([]string(nil), valid.Argv...)
			candidate.Environment = maps.Clone(valid.Environment)
			mutate(&candidate)
			if _, err := NewHostWorkspacePayload(candidate); err == nil {
				t.Fatal("invalid host payload was accepted")
			}
		})
	}
}

func TestMicroVMPayloadIsStrictAndBindingOnly(t *testing.T) {
	valid := MicroVMPayloadV1{
		KernelBindingID: "kernel-1", KernelDigest: digestForTest(t, "kernel"),
		InitramfsBindingID: "initramfs-1", InitramfsDigest: digestForTest(t, "initramfs"),
		VCPUs: 2, MemoryMiB: 256, NetworkDenyAll: true, WallClockTimeoutMilli: 30_000,
	}
	payload, err := NewMicroVMPayload(valid)
	if err != nil {
		t.Fatal(err)
	}
	if payload.ProviderKind != "qemu_microvm" || strings.Contains(string(payload.ProviderPayload), "/boot/") {
		t.Fatal("microVM payload did not remain an opaque binding-only request")
	}
	for name, mutate := range map[string]func(*MicroVMPayloadV1){
		"kernel binding": func(value *MicroVMPayloadV1) { value.KernelBindingID = "" },
		"kernel digest":  func(value *MicroVMPayloadV1) { value.KernelDigest = "sha256:bad" },
		"initramfs":      func(value *MicroVMPayloadV1) { value.InitramfsBindingID = "" },
		"vcpus":          func(value *MicroVMPayloadV1) { value.VCPUs = 0 },
		"memory":         func(value *MicroVMPayloadV1) { value.MemoryMiB = 63 },
		"network":        func(value *MicroVMPayloadV1) { value.NetworkDenyAll = false },
		"ttl":            func(value *MicroVMPayloadV1) { value.WallClockTimeoutMilli = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if _, err := NewMicroVMPayload(candidate); err == nil {
				t.Fatal("invalid microVM payload was accepted")
			}
		})
	}
}

func digestForTest(t *testing.T, value string) string {
	t.Helper()
	digest, err := canonicalDigest("fixture", value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func fixtureRequest(t *testing.T, now time.Time) DispatchRequestV1 {
	t.Helper()
	expires := now.Add(time.Minute).UnixNano()
	digest := func(value string) string {
		result, err := canonicalDigest("fixture", value)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	payload, err := NewWasmPayload(WasmPayloadV1{
		ComponentPathBindingID: "component-1", ComponentDigest: digest("component"),
		World: "praxis:sandbox/capability@1.0.0", Export: "run", Fuel: 1000,
		EpochDeadlineTicks: 10, MemoryLimitBytes: 16 << 20, TableElementsLimit: 128, InstanceLimit: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := ProviderBindingV1{
		BindingSetID: "bindings", BindingSetRevision: 1, ComponentID: "provider",
		ManifestDigest: digest("manifest"), ArtifactDigest: digest("artifact"), Capability: "sandbox.execute",
	}
	provider.Digest, err = canonicalDigest("ProviderBindingV1", provider)
	if err != nil {
		t.Fatal(err)
	}
	query, err := canonicalJSON([]byte(`{"fixture":"query"}`))
	if err != nil {
		t.Fatal(err)
	}
	request := DispatchRequestV1{
		ContractVersion: ContractVersionV1, RequestID: "request-1", Phase: PhasePrepare,
		EffectKind: "praxis.sandbox/open", OperationDigest: digest("operation"), EffectID: "effect-1",
		IntentRevision: 1, IntentDigest: digest("intent"), AttemptID: "attempt-1", TenantID: "tenant-1",
		ProviderBinding:     provider,
		SandboxAttempt:      ExactRefV1{ID: "attempt-1", Revision: 1, Digest: digest("attempt"), ExpiresUnixNano: expires},
		ExecutionBinding:    ExecutionBindingV1{TenantID: "tenant-1", InstanceID: "instance-1", InstanceEpoch: 1, LeaseID: "lease-1", LeaseEpoch: 1, FenceEpoch: 1, ScopeDigest: digest("scope"), ObservedRevision: 1, ExpiresUnixNano: expires},
		RuntimeEnforcement:  RuntimeEnforcementRefV1{OperationDigest: digest("operation"), EffectID: "effect-1", PermitID: "permit-1", AttemptID: "attempt-1", Phase: PhasePrepare, ReceiptDigest: digest("receipt"), JournalRevision: 1, ExpiresUnixNano: expires},
		RuntimeCurrentQuery: query, RequestedNotAfterUnixNano: expires,
		PayloadSchema: "praxis.sandbox/provider-payload/v1", PayloadRevision: 1, Payload: payload,
	}
	request.RuntimeCurrentQueryDigest, err = canonicalDigest("RuntimeCurrentQueryV1", json.RawMessage(query))
	if err != nil {
		t.Fatal(err)
	}
	request.PayloadDigest, err = canonicalDigest("ProviderPayloadV1", payload)
	if err != nil {
		t.Fatal(err)
	}
	request.Digest, err = request.digestV1()
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func inspectionRequestFixture(t *testing.T, now time.Time) DispatchRequestV1 {
	t.Helper()
	request := fixtureRequest(t, now)
	target := ProviderInspectionTargetV1{
		OriginalEffectKind: "praxis.sandbox/open", OriginalAttemptID: "attempt-1",
		ProviderAttempt:       ExactRefV1{ID: "wasmtime/tenant-1/attempt-1", Revision: 2, Digest: digestForTest(t, "provider-attempt"), ExpiresUnixNano: now.Add(time.Minute).UnixNano()},
		OriginalRequestDigest: digestForTest(t, "original-request"), OriginalPayloadDigest: request.PayloadDigest,
	}
	var payload WasmPayloadV1
	if err := json.Unmarshal(request.Payload.ProviderPayload, &payload); err != nil {
		t.Fatal(err)
	}
	payload.InspectionTarget = &target
	providerPayload, err := NewWasmPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	request.RequestID = "inspect-request-1"
	request.EffectKind = "praxis.sandbox/inspect"
	request.EffectID = "inspect-effect-1"
	request.AttemptID = "inspect-attempt-1"
	request.SandboxAttempt.ID = request.AttemptID
	request.SandboxAttempt.Digest = digestForTest(t, "inspect-attempt")
	request.RuntimeEnforcement.EffectID = request.EffectID
	request.RuntimeEnforcement.AttemptID = request.AttemptID
	request.RuntimeEnforcement.ReceiptDigest = digestForTest(t, "inspect-enforcement")
	request.Payload = providerPayload
	request.PayloadDigest, err = canonicalDigest("ProviderPayloadV1", request.Payload)
	if err != nil {
		t.Fatal(err)
	}
	request.Digest, err = request.digestV1()
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestInspectableOriginalEffectKindsCoverIndependentLifecycleEffects(t *testing.T) {
	for _, effect := range []string{
		"praxis.sandbox/backend-discovery", "praxis.sandbox/allocate", "praxis.sandbox/activate",
		"praxis.sandbox/open", "praxis.sandbox/cancel", "praxis.sandbox/close",
		"praxis.sandbox/fence", "praxis.sandbox/release", "praxis.sandbox/cleanup",
	} {
		if !inspectableOriginalEffectKind(effect) {
			t.Fatalf("independent lifecycle effect %q cannot be inspected", effect)
		}
	}
	for _, effect := range []string{"", "praxis.sandbox/inspect", CheckpointEffectKindV1, "custom/effect"} {
		if inspectableOriginalEffectKind(effect) {
			t.Fatalf("effect %q incorrectly gained ordinary lifecycle Inspect semantics", effect)
		}
	}
}

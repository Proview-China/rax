package contract

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var contractTestNow = time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)

func TestBootstrapPlanValidationMatrix(t *testing.T) {
	t.Parallel()
	valid := validTestManifest().Bootstrap
	cases := []struct {
		name   string
		mutate func(*BootstrapPlan)
		now    time.Time
	}{
		{"missing_identity", func(p *BootstrapPlan) { p.ID = "" }, contractTestNow},
		{"oversized_identity", func(p *BootstrapPlan) { p.ID = strings.Repeat("p", MaxReferenceBytes+1) }, contractTestNow},
		{"invalid_digest", func(p *BootstrapPlan) { p.ToolSurfaceDigest = "bad" }, contractTestNow},
		{"ungoverned", func(p *BootstrapPlan) { p.MinimumConformance = runtimeports.ConformanceContainedObserveOnly }, contractTestNow},
		{"zero_validation_time", func(*BootstrapPlan) {}, time.Time{}},
		{"expired", func(p *BootstrapPlan) { p.EvidenceExpiresAt = contractTestNow }, contractTestNow},
		{"blank_residual", func(p *BootstrapPlan) { p.AllowedResiduals = []string{""} }, contractTestNow},
		{"duplicate_residual", func(p *BootstrapPlan) { p.AllowedResiduals = []string{"x", "x"} }, contractTestNow},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			test.mutate(&candidate)
			if err := candidate.Validate(test.now); err == nil {
				t.Fatal("invalid bootstrap plan was accepted")
			}
		})
	}
	if err := valid.Validate(contractTestNow); err != nil {
		t.Fatal(err)
	}
}

func TestManifestValidationMatrix(t *testing.T) {
	t.Parallel()
	valid := validTestManifest()
	cases := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"contract_version", func(m *Manifest) { m.ContractVersion = "old" }},
		{"missing_id", func(m *Manifest) { m.ID = "" }},
		{"oversized_id", func(m *Manifest) { m.ID = strings.Repeat("h", MaxReferenceBytes+1) }},
		{"ungoverned", func(m *Manifest) { m.Conformance = runtimeports.ConformanceRejected }},
		{"below_minimum", func(m *Manifest) { m.Conformance = runtimeports.ConformanceRestrictedControlled }},
		{"invalid_bootstrap", func(m *Manifest) { m.Bootstrap.ID = "" }},
		{"artifact_digest", func(m *Manifest) { m.ArtifactDigest = "bad" }},
		{"evidence_digest", func(m *Manifest) { m.EvidenceDigest = "bad" }},
		{"evidence_expired", func(m *Manifest) { m.EvidenceExpiresAt = contractTestNow }},
		{"blank_capability", func(m *Manifest) { m.Capabilities = []string{""} }},
		{"duplicate_capability", func(m *Manifest) { m.Capabilities = []string{"x", "x"} }},
		{"blank_boundary", func(m *Manifest) { m.OpaqueBoundaries = []string{""} }},
		{"duplicate_boundary", func(m *Manifest) { m.OpaqueBoundaries = []string{"x", "x"} }},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := CloneManifest(valid)
			test.mutate(&candidate)
			if err := candidate.Validate(contractTestNow); err == nil {
				t.Fatal("invalid manifest was accepted")
			}
		})
	}
	restricted := valid
	restricted.Conformance = runtimeports.ConformanceRestrictedControlled
	restricted.Bootstrap.MinimumConformance = runtimeports.ConformanceRestrictedControlled
	if err := restricted.Validate(contractTestNow); err != nil {
		t.Fatal(err)
	}
}

func TestRunAndActionValueValidation(t *testing.T) {
	t.Parallel()
	run := RunRef{Scope: validTestScope(), RunID: "run-1"}
	if err := run.Validate(); err != nil {
		t.Fatal(err)
	}
	badRun := run
	badRun.RunID = ""
	if err := badRun.Validate(); err == nil {
		t.Fatal("blank run id accepted")
	}
	badRun = run
	badRun.Scope.AuthorityEpoch = 0
	if err := badRun.Validate(); err == nil {
		t.Fatal("invalid scope accepted")
	}

	action := ActionRequest{Ref: "action-1", Capability: "tool.search", Payload: validTestPayload("action")}
	if err := action.Validate(); err != nil {
		t.Fatal(err)
	}
	badAction := action
	badAction.Ref = ""
	if err := badAction.Validate(); err == nil {
		t.Fatal("blank action ref accepted")
	}
	result := ActionResult{Ref: "action-1", Payload: validTestPayload("result")}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	result.Ref = ""
	if err := result.Validate(); err == nil {
		t.Fatal("blank result ref accepted")
	}
}

func TestRunStateValidationMatrix(t *testing.T) {
	t.Parallel()
	base := RunState{
		Ref: RunRef{Scope: validTestScope(), RunID: "run-1"}, Phase: RunRunning,
		Revision: 1, SessionRef: "session-1", StartedAt: contractTestNow,
	}
	validStates := []RunState{
		base,
		func() RunState { state := base; state.Phase = RunStarting; return state }(),
		func() RunState { state := base; state.Phase = RunWaitingInput; return state }(),
		func() RunState { state := base; state.Phase = RunReconciling; return state }(),
		func() RunState { state := base; state.Phase = RunCancelling; return state }(),
		func() RunState {
			state := base
			state.Phase = RunWaitingAction
			state.PendingAction = &ActionRequest{Ref: "a", Capability: "tool", Payload: validTestPayload("a")}
			return state
		}(),
	}
	for _, claim := range []CompletionClaim{ClaimCompleted, ClaimCancelled, ClaimFailed, ClaimIndeterminate} {
		state := base
		state.Phase, state.CompletionClaim, state.EndedAt = RunTerminal, claim, contractTestNow.Add(time.Second)
		validStates = append(validStates, state)
	}
	for index, state := range validStates {
		if err := state.Validate(); err != nil {
			t.Fatalf("valid state %d rejected: %v", index, err)
		}
	}
	cases := []struct {
		name   string
		mutate func(*RunState)
	}{
		{"invalid_ref", func(s *RunState) { s.Ref.RunID = "" }},
		{"missing_revision", func(s *RunState) { s.Revision = 0 }},
		{"active_with_terminal", func(s *RunState) { s.EndedAt = contractTestNow }},
		{"waiting_action_missing", func(s *RunState) { s.Phase = RunWaitingAction }},
		{"waiting_action_invalid", func(s *RunState) { s.Phase = RunWaitingAction; s.PendingAction = &ActionRequest{} }},
		{"terminal_missing_end", func(s *RunState) { s.Phase = RunTerminal; s.CompletionClaim = ClaimCompleted }},
		{"terminal_end_before_start", func(s *RunState) {
			s.Phase = RunTerminal
			s.CompletionClaim = ClaimCompleted
			s.EndedAt = contractTestNow.Add(-time.Second)
		}},
		{"terminal_bad_claim", func(s *RunState) {
			s.Phase = RunTerminal
			s.CompletionClaim = "bad"
			s.EndedAt = contractTestNow.Add(time.Second)
		}},
		{"unknown_phase", func(s *RunState) { s.Phase = "unknown" }},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid run state accepted")
			}
		})
	}
}

func TestEventSnapshotAndOpaqueValidationMatrix(t *testing.T) {
	t.Parallel()
	event := Event{
		SourceComponentID: "harness", SourceEpoch: 1, SourceSequence: 1, RunID: "run-1",
		Kind: EventRunStarted, Payload: validTestPayload("event"), ObservedAt: contractTestNow,
	}
	for _, kind := range []EventKind{
		EventRunStarted, EventModelTurnStarted, EventModelTurnObserved, EventModelTurnUncertain, EventModelOutput, EventActionRequested,
		EventActionResultReceived, EventInputRequested, EventInputReceived, EventCancelRequested,
		EventRunCompleted, EventRunCancelled, EventRunFailed,
	} {
		candidate := event
		candidate.Kind = kind
		if err := candidate.Validate(); err != nil {
			t.Fatalf("valid event kind %s rejected: %v", kind, err)
		}
	}
	badEvent := event
	badEvent.SourceSequence = 0
	if err := badEvent.Validate(); err == nil {
		t.Fatal("invalid event identity accepted")
	}
	badEvent = event
	badEvent.Kind = "unknown"
	if err := badEvent.Validate(); err == nil {
		t.Fatal("unknown event kind accepted")
	}

	state := RunState{Ref: RunRef{Scope: validTestScope(), RunID: "run-1"}, Phase: RunRunning, Revision: 1, SessionRef: "session", StartedAt: contractTestNow}
	snapshot := Snapshot{State: state, EventsDigest: testDigest("events"), CapturedAt: contractTestNow}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	snapshot.EventsDigest = "bad"
	if err := snapshot.Validate(); err == nil {
		t.Fatal("invalid snapshot digest accepted")
	}
	snapshot.EventsDigest = testDigest("events")
	snapshot.CapturedAt = time.Time{}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("missing capture time accepted")
	}

	valid := validTestPayload("opaque")
	if err := ValidateOpaque(valid); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []runtimeports.OpaquePayload{
		{Digest: valid.Digest, Payload: valid.Payload},
		{Schema: "x", Digest: "bad", Payload: valid.Payload},
		{Schema: "x", Digest: valid.Digest},
		{Schema: "x", Digest: valid.Digest, Payload: json.RawMessage(`{`)},
		{Schema: "x", Digest: valid.Digest, Payload: json.RawMessage(`"changed"`)},
	} {
		if err := ValidateOpaque(invalid); err == nil {
			t.Fatalf("invalid opaque payload accepted: %+v", invalid)
		}
	}
	oversized := validTestPayload(string(make([]byte, MaxOpaquePayloadBytes+1)))
	if err := ValidateOpaque(oversized); !core.HasReason(err, core.ReasonInvalidReference) {
		t.Fatalf("oversized opaque payload was not rejected: %v", err)
	}
	longSession := state
	longSession.SessionRef = string(make([]byte, MaxReferenceBytes+1))
	if err := longSession.Validate(); err == nil {
		t.Fatal("oversized native session reference was accepted")
	}
}

func FuzzValidateOpaqueNeverPanics(f *testing.F) {
	f.Add([]byte("seed"), "custom/schema")
	f.Add([]byte{}, "x")
	f.Fuzz(func(t *testing.T, raw []byte, schema string) {
		encoded, err := json.Marshal(string(raw))
		if err != nil {
			t.Skip()
		}
		digest, err := core.DigestJSON(json.RawMessage(encoded))
		if err != nil {
			t.Skip()
		}
		payload := runtimeports.OpaquePayload{Schema: schema, Digest: digest, Payload: encoded}
		validationErr := ValidateOpaque(payload)
		if validationErr == nil {
			mutated := CloneOpaque(payload)
			mutated.Payload = json.RawMessage(`"drift"`)
			if err := ValidateOpaque(mutated); err == nil {
				t.Fatal("content drift retained the original digest")
			}
		}
	})
}

func TestRunAndEventReferencesAreBounded(t *testing.T) {
	t.Parallel()
	run := RunRef{Scope: validTestScope(), RunID: core.AgentRunID(strings.Repeat("r", MaxReferenceBytes+1))}
	if err := run.Validate(); !core.HasReason(err, core.ReasonInvalidReference) {
		t.Fatalf("oversized run id was accepted: %v", err)
	}
	event := Event{SourceComponentID: strings.Repeat("s", MaxReferenceBytes+1), SourceEpoch: 1, SourceSequence: 1, RunID: "run", Kind: EventRunStarted, Payload: validTestPayload("x"), ObservedAt: contractTestNow}
	if err := event.Validate(); !core.HasReason(err, core.ReasonInvalidReference) {
		t.Fatalf("oversized evidence source id was accepted: %v", err)
	}
}

func validTestManifest() Manifest {
	return Manifest{
		ContractVersion: Version, ID: "harness", Version: "1", ArtifactDigest: testDigest("artifact"),
		Conformance: runtimeports.ConformanceFullyControlled,
		Bootstrap: BootstrapPlan{
			ID: "bootstrap", Version: "1", ResolvedPlanDigest: testDigest("plan"), ProfileDigest: testDigest("profile"),
			RuntimePolicyDigest: testDigest("policy"), HarnessStackDigest: testDigest("stack"), SemanticRouteDigest: testDigest("route"),
			ExpectedInjectionManifestDigest: testDigest("injection"), ContextPlanDigest: testDigest("context"),
			ToolSurfaceDigest: testDigest("tools"), CapabilityGrantDigest: testDigest("grant"),
			MinimumConformance: runtimeports.ConformanceFullyControlled, EvidenceExpiresAt: contractTestNow.Add(time.Hour),
		},
		Capabilities: []string{"run"}, OpaqueBoundaries: []string{"native"},
		EvidenceDigest: testDigest("evidence"), EvidenceExpiresAt: contractTestNow.Add(time.Hour),
	}
}

func validTestScope() core.ExecutionScope {
	return core.ExecutionScope{
		Identity:     core.AgentIdentityRef{TenantID: "tenant", ID: "agent", Epoch: 1},
		Lineage:      core.LineageRef{ID: "lineage", PlanDigest: testDigest("plan")},
		Instance:     core.InstanceRef{ID: "instance", Epoch: 1},
		SandboxLease: &core.SandboxLeaseRef{ID: "sandbox", Epoch: 1}, AuthorityEpoch: 1,
	}
}

func validTestPayload(value any) runtimeports.OpaquePayload {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return runtimeports.OpaquePayload{Schema: "test/v1", Digest: testDigest(value), Payload: payload}
}

func testDigest(value any) core.Digest {
	digest, err := core.DigestJSON(value)
	if err != nil {
		panic(err)
	}
	return digest
}

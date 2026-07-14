package ports

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var portsTestNow = time.Date(2026, 7, 14, 10, 30, 0, 0, time.UTC)

func TestContextValuesValidation(t *testing.T) {
	t.Parallel()
	run := contract.RunRef{Scope: portsTestScope(), RunID: "run-1"}
	request := ContextRequest{Run: run, ContextPlanDigest: portsDigest("context-plan"), Input: portsPayload("input")}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	invalidRequest := request
	invalidRequest.Run.RunID = ""
	if err := invalidRequest.Validate(); err == nil {
		t.Fatal("invalid run accepted")
	}
	invalidRequest = request
	invalidRequest.ContextPlanDigest = "bad"
	if err := invalidRequest.Validate(); err == nil {
		t.Fatal("invalid context digest accepted")
	}
	invalidRequest = request
	invalidRequest.Input.Payload = json.RawMessage(`"changed"`)
	if err := invalidRequest.Validate(); err == nil {
		t.Fatal("tampered input accepted")
	}

	snapshot := portsContextSnapshot()
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ContextSnapshot){
		func(s *ContextSnapshot) { s.Ref = "" },
		func(s *ContextSnapshot) { s.ObservedAt = time.Time{} },
		func(s *ContextSnapshot) { s.Payload.Payload = json.RawMessage(`null`) },
		func(s *ContextSnapshot) { s.EvidenceDigest = "bad" },
	} {
		candidate := snapshot
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatal("invalid context snapshot accepted")
		}
	}
}

func TestModelTurnRequestValidationMatrix(t *testing.T) {
	t.Parallel()
	scope := portsTestScope()
	intent, fence := portsIntentFence(scope, "turn")
	request := ModelTurnRequest{
		Run: contract.RunRef{Scope: scope, RunID: "run-1"}, Input: portsPayload("input"),
		Context: portsContextSnapshot(), Intent: intent, Fence: fence,
	}
	if err := request.Validate(portsTestNow); err != nil {
		t.Fatal(err)
	}
	action := contract.ActionResult{Ref: "action", Payload: portsPayload("result")}
	request.ActionResult = &action
	if err := request.Validate(portsTestNow); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*ModelTurnRequest)
	}{
		{"run", func(r *ModelTurnRequest) { r.Run.RunID = "" }},
		{"input", func(r *ModelTurnRequest) { r.Input.Payload = json.RawMessage(`"changed"`) }},
		{"context", func(r *ModelTurnRequest) { r.Context.Ref = "" }},
		{"action", func(r *ModelTurnRequest) { r.ActionResult.Ref = "" }},
		{"intent", func(r *ModelTurnRequest) { r.Intent.PersistedAt = time.Time{} }},
		{"fence", func(r *ModelTurnRequest) { r.Fence.ExpiresAt = portsTestNow }},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate := request
			copyAction := *request.ActionResult
			candidate.ActionResult = &copyAction
			test.mutate(&candidate)
			if err := candidate.Validate(portsTestNow); err == nil {
				t.Fatal("invalid model turn request accepted")
			}
		})
	}
}

func TestModelTurnResultValidationMatrix(t *testing.T) {
	t.Parallel()
	output := portsPayload("output")
	action := contract.ActionRequest{Ref: "action", Capability: "tool.search", Payload: portsPayload("action")}
	valid := []ModelTurnResult{
		{State: TurnCompleted, Output: &output, NativeSessionRef: "session", EvidenceDigest: portsDigest("completed")},
		{State: TurnActionRequired, Action: &action, NativeSessionRef: "session", EvidenceDigest: portsDigest("action")},
		{State: TurnInputRequired, NativeSessionRef: "session", EvidenceDigest: portsDigest("input")},
	}
	for _, result := range valid {
		if err := result.Validate(); err != nil {
			t.Fatalf("valid result rejected: %v", err)
		}
	}
	cases := []ModelTurnResult{
		{State: TurnCompleted, Output: &output, EvidenceDigest: portsDigest("x")},
		{State: TurnCompleted, Output: &output, NativeSessionRef: "s", EvidenceDigest: "bad"},
		{State: TurnCompleted, NativeSessionRef: "s", EvidenceDigest: portsDigest("x")},
		{State: TurnCompleted, Output: &output, Action: &action, NativeSessionRef: "s", EvidenceDigest: portsDigest("x")},
		{State: TurnActionRequired, NativeSessionRef: "s", EvidenceDigest: portsDigest("x")},
		{State: TurnActionRequired, Action: &action, Output: &output, NativeSessionRef: "s", EvidenceDigest: portsDigest("x")},
		{State: TurnInputRequired, Output: &output, NativeSessionRef: "s", EvidenceDigest: portsDigest("x")},
		{State: "unknown", NativeSessionRef: "s", EvidenceDigest: portsDigest("x")},
	}
	for index, result := range cases {
		if err := result.Validate(); err == nil {
			t.Fatalf("invalid result %d accepted", index)
		}
	}
	tampered := valid[0]
	tampered.Output = &runtimeports.OpaquePayload{Schema: "x", Digest: portsDigest("x"), Payload: json.RawMessage(`"y"`)}
	if err := tampered.Validate(); err == nil {
		t.Fatal("tampered output accepted")
	}
}

func portsContextSnapshot() ContextSnapshot {
	return ContextSnapshot{Ref: "context-1", Payload: portsPayload("context"), EvidenceDigest: portsDigest("context-evidence"), ObservedAt: portsTestNow}
}

func portsTestScope() core.ExecutionScope {
	return core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant", ID: "agent", Epoch: 1},
		Lineage:  core.LineageRef{ID: "lineage", PlanDigest: portsDigest("plan")},
		Instance: core.InstanceRef{ID: "instance", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox", Epoch: 1}, AuthorityEpoch: 1,
	}
}

func portsIntentFence(scope core.ExecutionScope, id string) (core.EffectIntent, core.ExecutionFence) {
	payload := portsDigest(id)
	intent := core.EffectIntent{
		ID: core.EffectIntentID(id), Revision: 1, Kind: core.EffectKindHostedExecution, RiskClass: "test",
		CanonicalPayloadDigest: payload, Target: id, ConflictEffectDomain: "test", Ownership: core.EffectOwnership{
			IntentOwner: core.OwnerRef{Domain: "test", ID: "owner"}, SettlementOwner: core.OwnerRef{Domain: "test", ID: "owner"},
		}, AuthorizationRef: "auth", IdempotencyClass: core.IdempotencyQueryable, PersistedAt: portsTestNow.Add(-time.Minute),
	}
	fence := core.ExecutionFence{
		BoundaryScope: core.FenceBoundaryInstance, Scope: scope, CapabilityGrantDigest: portsDigest("grant"),
		EffectIntentID: intent.ID, EffectIntentRevision: 1, CanonicalPayloadDigest: payload, ExpiresAt: portsTestNow.Add(time.Hour),
	}
	return intent, fence
}

func portsPayload(value any) runtimeports.OpaquePayload {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return runtimeports.OpaquePayload{Schema: "test/v1", Digest: portsDigest(value), Payload: data}
}

func portsDigest(value any) core.Digest {
	digest, err := core.DigestJSON(value)
	if err != nil {
		panic(err)
	}
	return digest
}

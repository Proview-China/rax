package performance_test

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

var performanceTime = time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)

func profileCompileFixture(tb interface {
	Helper()
	Fatalf(string, ...any)
}) (*profile.Compiler, profile.CompileInput) {
	tb.Helper()
	profiles, err := profile.RepresentativeProfiles(performanceTime)
	if err != nil {
		tb.Fatalf("RepresentativeProfiles: %v", err)
	}
	registry, err := profile.NewRegistry(performanceTime, profiles...)
	if err != nil {
		tb.Fatalf("NewRegistry: %v", err)
	}
	compiler, err := profile.NewCompiler(registry, performanceTime)
	if err != nil {
		tb.Fatalf("NewCompiler: %v", err)
	}
	selected := profiles[0]
	actual := observedManifest(selected.HarnessCapability.ExpectedManifest)
	return compiler, profile.CompileInput{
		Request:        compileRequest(selected.ID),
		ActualManifest: actual,
	}
}

func compileRequest(id profile.ProfileID) union.UnifiedExecutionRequest {
	return union.UnifiedExecutionRequest{
		SemanticVersion: union.SemanticVersionV1,
		ExecutionID:     "exec-performance-profile",
		ProfileSelector: union.ProfileSelector{Exact: &union.VersionedIdentity{
			ID: string(id), Version: "v1candidate",
		}},
		ExecutionKind: union.ExecutionKindAuto,
		ToolPolicy: union.ToolPolicy{
			DefaultApproval: "on_side_effect", Parallelism: 1, MaxActions: 8,
		},
		OutputContract: union.OutputContract{
			AcceptedContentKinds: []string{"json"}, CompletionMode: "final",
			JSONSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
		},
		SessionIntent: union.SessionIntent{Mode: "new"},
		ExecutionPolicy: union.ExecutionPolicy{
			Sandbox: "workspace_write", CWDReference: "/workspace", NetworkPolicy: "denied",
			UserPresence: "present", Foreground: "required", InteractionMode: "interactive", MaxConcurrency: 1,
		},
		Budget:            union.Budget{MaxWallTime: 120 * time.Second, MaxToolActions: 8},
		DegradationPolicy: union.DegradationPolicy{Default: union.DegradationDefaultReject},
		IntentGraph: union.IntentGraph{Nodes: []union.IntentNode{
			{
				ID: "intent-structured", Kind: union.IntentProduceStructured, Target: "summary", Required: true,
				AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact, union.SemanticFidelityTransformed},
			},
		}},
	}
}

func observedManifest(expected profile.InjectionManifest) profile.InjectionManifest {
	actual := expected.Clone()
	actual.ProbeStatus = profile.ManifestProbeObserved
	for index := range actual.Fields {
		actual.Fields[index].Evidence = profile.ManifestEvidence{
			Source: profile.ManifestEvidenceObserved, Confidence: 100,
			Reference: "fixture://performance/" + actual.Fields[index].Path,
		}
	}
	return actual
}

func replayEvents(count int) []union.UnifiedExecutionEvent {
	if count < 1 {
		count = 1
	}
	events := make([]union.UnifiedExecutionEvent, 0, count)
	for index := 0; index < count; index++ {
		events = append(events, diagnosticEvent(index+1, byte(index)))
	}
	return events
}

func diagnosticEvent(sequence int, value byte) union.UnifiedExecutionEvent {
	payload, _ := json.Marshal(struct {
		Value byte `json:"value"`
	}{Value: value})
	return union.UnifiedExecutionEvent{
		Header: performanceHeader(uint64(sequence), union.EventFamilyDiagnostic),
		Diagnostic: &union.DiagnosticEvent{
			Kind: "performance_trace", Payload: payload,
		},
	}
}

func lifecycleEvent(sequence int, kind string, status union.ExecutionStatus) union.UnifiedExecutionEvent {
	return union.UnifiedExecutionEvent{
		Header: performanceHeader(uint64(sequence), union.EventFamilyLifecycle),
		Lifecycle: &union.LifecycleEvent{
			Kind: kind, Status: status,
		},
	}
}

func modelEvent(sequence int, value byte) union.UnifiedExecutionEvent {
	payload, _ := json.Marshal(struct {
		Value byte `json:"value"`
	}{Value: value})
	return union.UnifiedExecutionEvent{
		Header: performanceHeader(uint64(sequence), union.EventFamilyModel),
		Model:  &union.ModelEvent{Kind: "performance_delta", Payload: payload},
	}
}

func performanceHeader(sequence uint64, family union.EventFamily) union.EventHeader {
	return union.EventHeader{
		EventID:                union.EventID(fmt.Sprintf("performance-event-%d", sequence)),
		SemanticVersion:        union.SemanticVersionV1,
		ExecutionID:            "exec-performance-replay",
		Sequence:               sequence,
		Timestamp:              performanceTime.Add(time.Duration(sequence) * time.Microsecond),
		Origin:                 union.EventOriginPraxis,
		Family:                 family,
		Visibility:             union.VisibilityAuditOnly,
		SecurityClassification: union.SecurityInternal,
		ExecutionKind:          union.ExecutionKindModel,
		Profile:                union.VersionedIdentity{ID: "profile-performance", Version: "v1"},
		Route:                  union.VersionedIdentity{ID: "route-performance", Version: "v1"},
	}
}

func replayDeterministically(events []union.UnifiedExecutionEvent) (execution.LedgerState, error) {
	ledger, err := execution.Replay("exec-performance-replay", events)
	if err != nil {
		return execution.LedgerState{}, err
	}
	return ledger.State(), nil
}

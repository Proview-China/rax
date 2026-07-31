package routegateway_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	modelsqlite "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGovernedModelTurnV3ActualBoundaryFailsClosedWithoutExactCredentialCurrent(
	t *testing.T,
) {
	fixture := newGovernedActualBoundaryFixtureV3(t)
	defer fixture.close(t)

	result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
		context.Background(),
		fixture.command,
	)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorUnavailable {
		t.Fatalf("pre-invoke result=%+v err=%v", result, err)
	}
	if result.Turn.RefV3() != fixture.command.TurnRef ||
		result.Boundary.Fact.ID != "" ||
		result.Invoke.Response.ID != "" ||
		fixture.state.invoke.Load() != 0 {
		t.Fatalf(
			"pre-invoke result=%+v provider calls=%d want exact turn/zero boundary/0",
			result,
			fixture.state.invoke.Load(),
		)
	}
	if _, inspectErr :=
		fixture.store.InspectGovernedModelTurnProviderBoundaryTurnAttemptV3(
			context.Background(),
			result.Turn.AttemptRefV3(),
		); modelinvoker.GovernedModelInvocationErrorKindOfV1(inspectErr) !=
		modelinvoker.GovernedModelInvocationErrorNotFound {
		t.Fatalf("pre-invoke boundary write error=%v want=not_found", inspectErr)
	}
}

func TestGovernedModelTurnV3ActualBoundaryRejectsProviderInjectionShapeDrift(
	t *testing.T,
) {
	falseValue := false
	trueValue := true
	tests := []struct {
		name           string
		mutateCall     func(*modelinvoker.RouteCall)
		mutatePrepared func(*modelinvoker.Request)
	}{
		{
			name: "provider",
			mutatePrepared: func(request *modelinvoker.Request) {
				request.Provider = "anthropic"
			},
		},
		{
			name: "protocol",
			mutatePrepared: func(request *modelinvoker.Request) {
				request.Protocol = modelinvoker.ProtocolChatCompletions
			},
		},
		{
			name: "tool_order",
			mutateCall: func(call *modelinvoker.RouteCall) {
				second := call.Request.Tools[0]
				second.Name = "workspace_search"
				second.Description = "search one workspace"
				second.Parameters = []byte(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":false}`)
				call.Request.Tools = append(call.Request.Tools, second)
			},
			mutatePrepared: func(request *modelinvoker.Request) {
				request.Tools[0], request.Tools[1] =
					request.Tools[1], request.Tools[0]
			},
		},
		{
			name: "tool_name",
			mutatePrepared: func(request *modelinvoker.Request) {
				request.Tools[0].Name = "workspace_inspect"
			},
		},
		{
			name: "tool_description",
			mutatePrepared: func(request *modelinvoker.Request) {
				request.Tools[0].Description = "inspect one file"
			},
		},
		{
			name: "tool_parameters",
			mutatePrepared: func(request *modelinvoker.Request) {
				request.Tools[0].Parameters =
					[]byte(`{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer"}},"required":["path"],"additionalProperties":false}`)
			},
		},
		{
			name: "tool_strict",
			mutatePrepared: func(request *modelinvoker.Request) {
				request.Tools[0].Strict = &falseValue
			},
		},
		{
			name: "tool_choice",
			mutatePrepared: func(request *modelinvoker.Request) {
				request.ToolChoice = modelinvoker.ToolChoice{
					Mode: modelinvoker.ToolChoiceAuto,
				}
			},
		},
		{
			name: "parallel_unspecified",
			mutatePrepared: func(request *modelinvoker.Request) {
				request.ParallelToolCalls = nil
			},
		},
		{
			name: "parallel_true",
			mutatePrepared: func(request *modelinvoker.Request) {
				request.ParallelToolCalls = &trueValue
			},
		},
		{
			name: "provider_options_present_empty",
			mutatePrepared: func(request *modelinvoker.Request) {
				request.ProviderOptions = modelinvoker.ProviderOptions{
					request.Provider: []byte(`{}`),
				}
			},
		},
		{
			name: "provider_options_body",
			mutatePrepared: func(request *modelinvoker.Request) {
				request.ProviderOptions = modelinvoker.ProviderOptions{
					request.Provider: []byte(
						`{"hosted":{"mode":"search"}}`,
					),
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture :=
				newGovernedActualBoundaryFixtureV3WithProviderInjection(
					t,
					nil,
					test.mutateCall,
					test.mutatePrepared,
				)
			defer fixture.close(t)

			assertProviderInjectionShapeMismatchV3(t, fixture, nil)
		})
	}
}

func TestGovernedModelTurnV3ActualBoundaryAcceptsCanonicalEquivalentProviderInjectionShape(
	t *testing.T,
) {
	tests := []struct {
		name           string
		mutateCall     func(*modelinvoker.RouteCall)
		mutatePrepared func(*modelinvoker.Request)
	}{
		{
			name: "parameters_key_order_and_whitespace",
			mutatePrepared: func(request *modelinvoker.Request) {
				request.Tools[0].Parameters = []byte(`{
					"additionalProperties": false,
					"properties": {"path": {"type": "string"}},
					"required": ["path"],
					"type": "object"
				}`)
			},
		},
		{
			name: "parameters_paired_surrogate_and_literal_scalar",
			mutateCall: func(call *modelinvoker.RouteCall) {
				call.Request.Tools[0].Parameters = []byte(
					`{"type":"object","marker":"😀"}`,
				)
			},
			mutatePrepared: func(request *modelinvoker.Request) {
				request.Tools[0].Parameters = []byte(
					`{"marker":"\ud83d\ude00","type":"object"}`,
				)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture :=
				newGovernedActualBoundaryFixtureV3WithProviderInjection(
					t,
					nil,
					test.mutateCall,
					test.mutatePrepared,
				)
			defer fixture.close(t)

			result, err :=
				fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
					context.Background(),
					fixture.command,
				)
			if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
				modelinvoker.GovernedModelInvocationErrorUnavailable ||
				result.Boundary.Fact.ID != "" ||
				result.Invoke.Response.ID != "" ||
				fixture.state.invoke.Load() != 0 {
				t.Fatalf(
					"canonical-equivalent result=%+v provider=%d err=%v",
					result,
					fixture.state.invoke.Load(),
					err,
				)
			}
			if !strings.Contains(
				err.Error(),
				"exact credential current proof is unavailable",
			) {
				t.Fatalf(
					"canonical-equivalent did not reach credential hard NO-GO: %v",
					err,
				)
			}
			assertProviderIrreversibleCountsZeroV3(t, fixture)
			assertNoProviderBoundaryFactV3(t, fixture)
		})
	}
}

func TestGovernedModelTurnV3ActualBoundaryRejectsInvalidActualProviderInjectionShape(
	t *testing.T,
) {
	const rawSecret = "A2-RAW-PROVIDER-INJECTION-MUST-NOT-LEAK"
	fixture := newGovernedActualBoundaryFixtureV3WithProviderInjection(
		t,
		nil,
		func(call *modelinvoker.RouteCall) {
			call.Request.Tools[0].Parameters = []byte(
				`{"type":"object","marker":"` +
					rawSecret +
					`","x":"\ud800"}`,
			)
		},
		func(request *modelinvoker.Request) {
			request.Tools[0].Parameters = []byte(
				`{"type":"object","properties":{},"additionalProperties":false}`,
			)
		},
	)
	defer fixture.close(t)

	result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
		context.Background(),
		fixture.command,
	)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorConflict ||
		result.Boundary.Fact.ID != "" ||
		result.Invoke.Response.ID != "" ||
		strings.Contains(err.Error(), rawSecret) {
		t.Fatalf(
			"invalid actual provider injection result=%+v err=%v",
			result,
			err,
		)
	}
	var governedErr *modelinvoker.GovernedModelInvocationErrorV1
	if !errors.As(err, &governedErr) ||
		governedErr.Operation != "turn_v3_provider_injection_shape" {
		t.Fatalf("invalid actual provider injection operation drifted: %v", err)
	}
	assertProviderIrreversibleCountsZeroV3(t, fixture)
	if fixture.state.closed.Load() != 1 {
		t.Fatalf(
			"invalid actual provider injection closes=%d want=1",
			fixture.state.closed.Load(),
		)
	}
	assertNoProviderBoundaryFactV3(t, fixture)
}

func TestGovernedModelTurnV3ActualBoundaryProviderInjectionMismatchPreservesReleaseFailure(
	t *testing.T,
) {
	const sentinel = "V3-A2-RELEASE-FAILURE-MUST-NOT-LEAK"
	releaseFailure := errors.New(sentinel)
	var releaseFactory *actualBoundaryReleaseErrorFactoryV3
	fixture := newGovernedActualBoundaryFixtureV3WithProviderInjection(
		t,
		func(
			t *testing.T,
			routeCatalog *catalog.Catalog,
			state *callState,
			dependencies routegateway.GovernedModelTurnActualBoundaryDependenciesV3,
		) *routegateway.Gateway {
			t.Helper()
			releaseFactory = &actualBoundaryReleaseErrorFactoryV3{
				fakeFactory: fakeFactory{id: "openai", state: state},
				closeErr:    releaseFailure,
			}
			return actualBoundaryGatewayWithOverrideFactoryV3(
				t,
				routeCatalog,
				releaseFactory,
				state,
				dependencies,
			)
		},
		nil,
		func(request *modelinvoker.Request) {
			request.ToolChoice = modelinvoker.ToolChoice{
				Mode: modelinvoker.ToolChoiceAuto,
			}
		},
	)
	defer fixture.close(t)

	assertProviderInjectionShapeMismatchV3(
		t,
		fixture,
		releaseFailure,
	)
	if releaseFactory.closeCalls.Load() != 1 {
		t.Fatalf(
			"provider injection mismatch closes=%d want=1",
			releaseFactory.closeCalls.Load(),
		)
	}
}

func assertProviderInjectionShapeMismatchV3(
	t *testing.T,
	fixture governedActualBoundaryFixtureV3,
	releaseFailure error,
) {
	t.Helper()
	result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
		context.Background(),
		fixture.command,
	)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorConflict ||
		result.Boundary.Fact.ID != "" ||
		result.Invoke.Response.ID != "" ||
		fixture.state.invoke.Load() != 0 {
		t.Fatalf(
			"provider injection mismatch result=%+v provider=%d err=%v",
			result,
			fixture.state.invoke.Load(),
			err,
		)
	}
	if !strings.Contains(err.Error(), "prepared Provider injection shape") {
		t.Fatalf("provider injection mismatch reason drifted: %v", err)
	}
	if releaseFailure != nil {
		if !errors.Is(err, releaseFailure) ||
			strings.Contains(err.Error(), releaseFailure.Error()) {
			t.Fatalf("release failure evidence lost or leaked: %v", err)
		}
	}
	assertProviderIrreversibleCountsZeroV3(t, fixture)
	assertNoProviderBoundaryFactV3(t, fixture)
}

func assertProviderIrreversibleCountsZeroV3(
	t *testing.T,
	fixture governedActualBoundaryFixtureV3,
) {
	t.Helper()
	for name, calls := range map[string]int64{
		"authorization": fixture.state.authorization.Load(),
		"capabilities":  fixture.state.capabilities.Load(),
		"invoke":        fixture.state.invoke.Load(),
		"stream":        fixture.state.stream.Load(),
	} {
		if calls != 0 {
			t.Fatalf(
				"provider injection path touched irreversible %s=%d want=0",
				name,
				calls,
			)
		}
	}
}

func assertNoProviderBoundaryFactV3(
	t *testing.T,
	fixture governedActualBoundaryFixtureV3,
) {
	t.Helper()
	turn, err := fixture.store.InspectExactGovernedModelTurnV3(
		context.Background(),
		fixture.command.TurnRef,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, inspectErr :=
		fixture.store.InspectGovernedModelTurnProviderBoundaryTurnAttemptV3(
			context.Background(),
			turn.AttemptRefV3(),
		); modelinvoker.GovernedModelInvocationErrorKindOfV1(inspectErr) !=
		modelinvoker.GovernedModelInvocationErrorNotFound {
		t.Fatalf(
			"provider injection mismatch boundary write error=%v want=not_found",
			inspectErr,
		)
	}
}

func TestGovernedModelTurnV3ActualBoundaryPreservesAdapterReleaseFailure(
	t *testing.T,
) {
	const sentinel = "V3-RELEASE-FAILURE-MUST-NOT-LEAK"
	releaseFailure := errors.New(sentinel)
	var releaseFactory *actualBoundaryReleaseErrorFactoryV3
	fixture := newGovernedActualBoundaryFixtureV3WithGateway(
		t,
		func(
			t *testing.T,
			routeCatalog *catalog.Catalog,
			state *callState,
			dependencies routegateway.GovernedModelTurnActualBoundaryDependenciesV3,
		) *routegateway.Gateway {
			t.Helper()
			releaseFactory = &actualBoundaryReleaseErrorFactoryV3{
				fakeFactory: fakeFactory{id: "openai", state: state},
				closeErr:    releaseFailure,
			}
			return actualBoundaryGatewayWithOverrideFactoryV3(
				t,
				routeCatalog,
				releaseFactory,
				state,
				dependencies,
			)
		},
	)
	defer fixture.close(t)

	result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
		context.Background(),
		fixture.command,
	)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorUnavailable ||
		!errors.Is(err, releaseFailure) ||
		strings.Contains(err.Error(), sentinel) {
		t.Fatalf("release failure lost or leaked evidence: %v", err)
	}
	if result.Boundary.Fact.ID != "" ||
		result.Invoke.Response.ID != "" ||
		fixture.state.invoke.Load() != 0 ||
		releaseFactory.closeCalls.Load() != 1 {
		t.Fatalf(
			"release failure result=%+v invoke=%d closes=%d want zero/zero/1",
			result,
			fixture.state.invoke.Load(),
			releaseFactory.closeCalls.Load(),
		)
	}
}

func TestGovernedModelTurnV3ActualBoundarySameCoordinateMaterialDriftStaysClosed(
	t *testing.T,
) {
	var resolver *sameCoordinateMaterialDriftSecretV3
	fixture := newGovernedActualBoundaryFixtureV3WithGateway(
		t,
		func(
			t *testing.T,
			routeCatalog *catalog.Catalog,
			state *callState,
			dependencies routegateway.GovernedModelTurnActualBoundaryDependenciesV3,
		) *routegateway.Gateway {
			t.Helper()
			resolver = &sameCoordinateMaterialDriftSecretV3{state: state}
			return fakeGateway(
				t,
				routeCatalog,
				countingBinding{state: state},
				resolver,
				state,
				routegateway.WithGovernedModelTurnActualBoundaryV3(dependencies),
			)
		},
	)
	defer fixture.close(t)

	for attempt := 0; attempt < 2; attempt++ {
		result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
			context.Background(),
			fixture.command,
		)
		if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
			modelinvoker.GovernedModelInvocationErrorUnavailable ||
			result.Boundary.Fact.ID != "" {
			t.Fatalf("material drift attempt=%d result=%+v err=%v", attempt, result, err)
		}
	}
	if resolver.calls.Load() != 2 || fixture.state.invoke.Load() != 0 {
		t.Fatalf(
			"same-coordinate material generations=%d provider calls=%d want=2/0",
			resolver.calls.Load(),
			fixture.state.invoke.Load(),
		)
	}
	turn, err := fixture.store.InspectExactGovernedModelTurnV3(
		context.Background(),
		fixture.command.TurnRef,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, inspectErr :=
		fixture.store.InspectGovernedModelTurnProviderBoundaryTurnAttemptV3(
			context.Background(),
			turn.AttemptRefV3(),
		); modelinvoker.GovernedModelInvocationErrorKindOfV1(inspectErr) !=
		modelinvoker.GovernedModelInvocationErrorNotFound {
		t.Fatalf("material drift boundary write error=%v want=not_found", inspectErr)
	}
}

func TestGovernedModelTurnV3ActualBoundaryConcurrentCallsWriteNothing(
	t *testing.T,
) {
	fixture := newGovernedActualBoundaryFixtureV3(t)
	defer fixture.close(t)
	const workers = 64
	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	done.Add(workers)
	var unavailable atomic.Int64
	for range workers {
		go func() {
			defer done.Done()
			start.Wait()
			result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
				context.Background(),
				fixture.command,
			)
			if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) ==
				modelinvoker.GovernedModelInvocationErrorUnavailable &&
				result.Boundary.Fact.ID == "" {
				unavailable.Add(1)
			}
		}()
	}
	start.Done()
	done.Wait()
	if unavailable.Load() != workers || fixture.state.invoke.Load() != 0 {
		t.Fatalf(
			"fail-closed calls=%d provider calls=%d want=%d/0",
			unavailable.Load(),
			fixture.state.invoke.Load(),
			workers,
		)
	}
	turn, err := fixture.store.InspectExactGovernedModelTurnV3(
		context.Background(),
		fixture.command.TurnRef,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, inspectErr :=
		fixture.store.InspectGovernedModelTurnProviderBoundaryTurnAttemptV3(
			context.Background(),
			turn.AttemptRefV3(),
		); modelinvoker.GovernedModelInvocationErrorKindOfV1(inspectErr) !=
		modelinvoker.GovernedModelInvocationErrorNotFound {
		t.Fatalf("concurrent boundary write error=%v want=not_found", inspectErr)
	}
}

func TestGovernedModelTurnV3ActualBoundaryRejectsSourceTTLDriftAcrossS1S2(
	t *testing.T,
) {
	for _, test := range []struct {
		name      string
		source    string
		alternate time.Time
	}{
		{
			name:      "context_non_min_shrink",
			source:    "context",
			alternate: gatewayNow.Add(17 * time.Minute / 2),
		},
		{
			name:      "context_non_min_extend",
			source:    "context",
			alternate: gatewayNow.Add(19 * time.Minute / 2),
		},
		{
			name:      "tool_non_min_shrink",
			source:    "tool",
			alternate: gatewayNow.Add(17 * time.Minute / 2),
		},
		{
			name:      "tool_non_min_extend",
			source:    "tool",
			alternate: gatewayNow.Add(19 * time.Minute / 2),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGovernedActualBoundaryFixtureV3(t)
			defer fixture.close(t)
			fixture.readers.calls.Store(0)
			fixture.readers.contextCalls.Store(0)
			fixture.readers.toolCalls.Store(0)
			fixture.readers.alternateAt = 3
			switch test.source {
			case "context":
				fixture.readers.contextAlternateExpires =
					test.alternate.UnixNano()
			case "tool":
				fixture.readers.toolAlternateExpires =
					test.alternate.UnixNano()
			default:
				t.Fatalf("unknown source %q", test.source)
			}

			result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
				context.Background(),
				fixture.command,
			)
			if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
				modelinvoker.GovernedModelInvocationErrorConflict ||
				result.Boundary.Fact.ID != "" ||
				fixture.state.invoke.Load() != 0 {
				t.Fatalf(
					"TTL %s result=%+v provider=%d err=%v",
					test.name,
					result,
					fixture.state.invoke.Load(),
					err,
				)
			}
			for key, calls := range fixture.state.snapshot() {
				if calls != 0 {
					t.Fatalf("TTL %s touched %s=%d want=0", test.name, key, calls)
				}
			}
			if fixture.readers.contextCalls.Load() != 2 ||
				fixture.readers.toolCalls.Load() != 2 {
				t.Fatalf(
					"TTL %s pair reads context/tool=%d/%d want=2/2",
					test.name,
					fixture.readers.contextCalls.Load(),
					fixture.readers.toolCalls.Load(),
				)
			}
		})
	}
}

func TestGovernedModelTurnV3ActualBoundaryRejectsSourceCheckedDriftAcrossS1S2(
	t *testing.T,
) {
	for _, source := range []string{"context", "tool"} {
		t.Run(source, func(t *testing.T) {
			fixture := newGovernedActualBoundaryFixtureV3(t)
			defer fixture.close(t)
			fixture.readers.calls.Store(0)
			fixture.readers.contextCalls.Store(0)
			fixture.readers.toolCalls.Store(0)
			fixture.readers.alternateAt = 3
			switch source {
			case "context":
				fixture.readers.contextAlternateChecked =
					gatewayNow.Add(-30 * time.Second).UnixNano()
			case "tool":
				fixture.readers.toolAlternateChecked =
					gatewayNow.Add(-30 * time.Second).UnixNano()
			}

			result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
				context.Background(),
				fixture.command,
			)
			assertActualBoundaryRejectedBeforePrepareV3(
				t,
				fixture,
				result,
				err,
			)
			if fixture.readers.contextCalls.Load() != 2 ||
				fixture.readers.toolCalls.Load() != 2 {
				t.Fatalf(
					"checked %s pair reads context/tool=%d/%d want=2/2",
					source,
					fixture.readers.contextCalls.Load(),
					fixture.readers.toolCalls.Load(),
				)
			}
		})
	}
}

func TestGovernedModelTurnV3ActualBoundaryRejectsPairProjectionDigestSplice(
	t *testing.T,
) {
	for _, source := range []string{"context", "tool"} {
		t.Run(source, func(t *testing.T) {
			fixture := newGovernedActualBoundaryFixtureV3(t)
			defer fixture.close(t)
			fixture.readers.calls.Store(0)
			fixture.readers.contextCalls.Store(0)
			fixture.readers.toolCalls.Store(0)
			switch source {
			case "context":
				fixture.readers.contextCorruptDigestAt = 1
			case "tool":
				fixture.readers.toolCorruptDigestAt = 2
			}

			result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
				context.Background(),
				fixture.command,
			)
			assertActualBoundaryRejectedBeforePrepareV3(
				t,
				fixture,
				result,
				err,
			)
		})
	}
}

func TestGovernedModelTurnV3ActualBoundaryRejectsAckExpiryEquality(
	t *testing.T,
) {
	fixture := newGovernedActualBoundaryFixtureV3(t)
	defer fixture.close(t)
	ack := fixture.gate.ack
	ack.ExpiresUnixNano = gatewayNow.UnixNano()
	ack.Digest = ""
	var err error
	ack, err = modelinvoker.SealPreparedModelInvocationCommitAckV1(ack)
	if err != nil {
		t.Fatal(err)
	}
	fixture.gate.ack = ack

	result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
		context.Background(),
		fixture.command,
	)
	assertActualBoundaryRejectedBeforePrepareV3(
		t,
		fixture,
		result,
		err,
	)
}

func TestGovernedModelTurnV3ActualBoundaryRejectsAckExpiryEqualityAtS2(
	t *testing.T,
) {
	clock := &actualBoundaryClockV3{
		values: []time.Time{
			gatewayNow,
			gatewayNow.Add(time.Second),
		},
	}
	fixture := newGovernedActualBoundaryFixtureV3WithGateway(
		t,
		func(
			t *testing.T,
			routeCatalog *catalog.Catalog,
			state *callState,
			dependencies routegateway.GovernedModelTurnActualBoundaryDependenciesV3,
		) *routegateway.Gateway {
			t.Helper()
			return fakeGateway(
				t,
				routeCatalog,
				countingBinding{state: state},
				countingSecret{state: state, version: "v1"},
				state,
				routegateway.WithGovernedModelTurnActualBoundaryV3(dependencies),
				routegateway.WithClock(clock.Now),
			)
		},
	)
	defer fixture.close(t)
	ack := fixture.gate.ack
	ack.ExpiresUnixNano = gatewayNow.Add(time.Second).UnixNano()
	ack.Digest = ""
	var err error
	ack, err = modelinvoker.SealPreparedModelInvocationCommitAckV1(ack)
	if err != nil {
		t.Fatal(err)
	}
	fixture.gate.ack = ack

	result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
		context.Background(),
		fixture.command,
	)
	assertActualBoundaryRejectedBeforePrepareV3(
		t,
		fixture,
		result,
		err,
	)
}

func TestGovernedModelTurnV3ActualBoundaryRejectsAckBodySpliceAtS2(
	t *testing.T,
) {
	fixture := newGovernedActualBoundaryFixtureV3(t)
	defer fixture.close(t)
	spliced := fixture.gate.ack
	spliced.GateImplementationRef.ID += "-spliced"
	fixture.gate.inspectOverride = &spliced

	result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
		context.Background(),
		fixture.command,
	)
	assertActualBoundaryRejectedBeforePrepareV3(
		t,
		fixture,
		result,
		err,
	)
}

func TestGovernedModelTurnV3ActualBoundaryRejectsValidAckDriftAtS2(
	t *testing.T,
) {
	fixture := newGovernedActualBoundaryFixtureV3(t)
	defer fixture.close(t)
	drifted := fixture.gate.ack
	drifted.GateImplementationRef.ID += "-drifted"
	drifted.Digest = ""
	var err error
	drifted, err = modelinvoker.SealPreparedModelInvocationCommitAckV1(drifted)
	if err != nil {
		t.Fatal(err)
	}
	fixture.gate.inspectOverride = &drifted

	result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
		context.Background(),
		fixture.command,
	)
	assertActualBoundaryRejectedBeforePrepareV3(
		t,
		fixture,
		result,
		err,
	)
}

func TestGovernedModelTurnV3ActualBoundaryRejectsExactTurnBodySplice(
	t *testing.T,
) {
	for _, test := range []struct {
		name   string
		splice func(*modelinvoker.GovernedModelTurnOutcomeV3)
	}{
		{
			name: "state",
			splice: func(turn *modelinvoker.GovernedModelTurnOutcomeV3) {
				turn.State = "spliced"
			},
		},
		{
			name: "created",
			splice: func(turn *modelinvoker.GovernedModelTurnOutcomeV3) {
				turn.CreatedUnixNano++
			},
		},
		{
			name: "updated",
			splice: func(turn *modelinvoker.GovernedModelTurnOutcomeV3) {
				turn.UpdatedUnixNano++
			},
		},
		{
			name: "expires",
			splice: func(turn *modelinvoker.GovernedModelTurnOutcomeV3) {
				turn.ExpiresUnixNano--
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGovernedActualBoundaryFixtureV3(t)
			defer fixture.close(t)
			turn, err := fixture.store.InspectExactGovernedModelTurnV3(
				context.Background(),
				fixture.command.TurnRef,
			)
			if err != nil {
				t.Fatal(err)
			}
			test.splice(&turn)
			if turn.RefV3() != fixture.command.TurnRef {
				t.Fatal("test splice unexpectedly changed exact Turn Ref")
			}
			reader := &actualBoundaryTurnReaderV3{
				inner:    fixture.store,
				override: turn,
			}
			dependencies, err :=
				routegateway.NewGovernedModelTurnActualBoundaryDependenciesV3(
					fixture.store,
					fixture.store,
					fixture.gate,
					fixture.store,
					fixture.authorizer,
					reader,
					fixture.store,
				)
			if err != nil {
				t.Fatal(err)
			}
			gateway := fakeGateway(
				t,
				fixture.routeCatalog,
				countingBinding{state: fixture.state},
				countingSecret{state: fixture.state, version: "v1"},
				fixture.state,
				routegateway.WithGovernedModelTurnActualBoundaryV3(dependencies),
			)
			defer gateway.Close()

			result, err :=
				gateway.InvokeGovernedModelTurnActualBoundaryV3(
					context.Background(),
					fixture.command,
				)
			assertActualBoundaryRejectedBeforePrepareV3(
				t,
				fixture,
				result,
				err,
			)
		})
	}
}

func assertActualBoundaryRejectedBeforePrepareV3(
	t *testing.T,
	fixture governedActualBoundaryFixtureV3,
	result routegateway.GovernedModelTurnActualBoundaryResultV3,
	err error,
) {
	t.Helper()
	if err == nil ||
		result.Boundary.Fact.ID != "" ||
		result.Invoke.Response.ID != "" {
		t.Fatalf("pre-prepare rejection result=%+v err=%v", result, err)
	}
	for key, calls := range fixture.state.snapshot() {
		if calls != 0 {
			t.Fatalf("pre-prepare rejection touched %s=%d want=0", key, calls)
		}
	}
}

func TestGovernedModelTurnV3ActualBoundaryRejectsS2ClockRegression(
	t *testing.T,
) {
	clock := &actualBoundaryClockV3{
		values: []time.Time{
			gatewayNow,
			gatewayNow.Add(-time.Nanosecond),
		},
	}
	fixture := newGovernedActualBoundaryFixtureV3WithGateway(
		t,
		func(
			t *testing.T,
			routeCatalog *catalog.Catalog,
			state *callState,
			dependencies routegateway.GovernedModelTurnActualBoundaryDependenciesV3,
		) *routegateway.Gateway {
			t.Helper()
			return fakeGateway(
				t,
				routeCatalog,
				countingBinding{state: state},
				countingSecret{state: state, version: "v1"},
				state,
				routegateway.WithGovernedModelTurnActualBoundaryV3(dependencies),
				routegateway.WithClock(clock.Now),
			)
		},
	)
	defer fixture.close(t)

	result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
		context.Background(),
		fixture.command,
	)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorConflict ||
		result.Boundary.Fact.ID != "" ||
		fixture.state.invoke.Load() != 0 {
		t.Fatalf(
			"S2 regression result=%+v provider=%d err=%v",
			result,
			fixture.state.invoke.Load(),
			err,
		)
	}
	for key, calls := range fixture.state.snapshot() {
		if calls != 0 {
			t.Fatalf("S2 regression touched %s=%d want=0", key, calls)
		}
	}
}

func TestGovernedModelTurnV3ActualBoundaryRejectsRuntimeDraftBeforeProviderPrepare(
	t *testing.T,
) {
	fixture := newGovernedActualBoundaryFixtureV3(t)
	defer fixture.close(t)
	fixture.command.RuntimeRequest.PermitID += "-other"

	before := fixture.state.snapshot()
	result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
		context.Background(),
		fixture.command,
	)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorConflict ||
		result.Boundary.Fact.ID != "" {
		t.Fatalf("invalid Runtime draft result=%+v err=%v", result, err)
	}
	after := fixture.state.snapshot()
	for _, key := range []string{"binding", "secret", "factory", "invoke"} {
		if after[key] != before[key] {
			t.Fatalf(
				"invalid Runtime draft touched %s: before=%d after=%d",
				key,
				before[key],
				after[key],
			)
		}
	}
}

func TestGovernedModelTurnV3ActualBoundaryExistingWinnerRequiresExactCallerBoundary(
	t *testing.T,
) {
	fixture := newGovernedActualBoundaryFixtureV3(t)
	defer fixture.close(t)
	fact := buildGovernedActualBoundaryFactV3(t, fixture)
	persisted, err :=
		modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
			context.Background(),
			fixture.store,
			fact,
			func() time.Time { return gatewayNow },
		)
	if err != nil ||
		persisted.Disposition !=
			modelinvoker.GovernedModelTurnProviderBoundaryPersistenceCreatedV3 {
		t.Fatalf("seed boundary result=%+v err=%v", persisted, err)
	}

	exact := fixture.command
	exact.RuntimeRequest.ModelBoundary = persisted.Fact.RuntimeRequest.ModelBoundary
	result, err := fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
		context.Background(),
		exact,
	)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorIndeterminate ||
		result.Boundary.Fact.RefV3() != persisted.Fact.RefV3() ||
		result.Boundary.Disposition !=
			modelinvoker.GovernedModelTurnProviderBoundaryPersistenceExistingV3 {
		t.Fatalf("exact caller boundary result=%+v err=%v", result, err)
	}

	mismatched := fixture.command
	mismatchedBoundary := persisted.Fact.RuntimeRequest.ModelBoundary
	mismatchedBoundary.ID += "-other"
	mismatchedBoundary, err =
		runtimeports.SealModelProviderBoundaryCurrentRefV1(mismatchedBoundary)
	if err != nil {
		t.Fatal(err)
	}
	mismatched.RuntimeRequest.ModelBoundary = mismatchedBoundary
	result, err = fixture.gateway.InvokeGovernedModelTurnActualBoundaryV3(
		context.Background(),
		mismatched,
	)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorConflict ||
		result.Boundary.Fact.ID != "" ||
		fixture.state.invoke.Load() != 0 {
		t.Fatalf(
			"mismatched caller boundary result=%+v provider=%d err=%v",
			result,
			fixture.state.invoke.Load(),
			err,
		)
	}
	for key, calls := range fixture.state.snapshot() {
		if calls != 0 {
			t.Fatalf("caller boundary replay touched %s=%d want=0", key, calls)
		}
	}
}

func TestGovernedModelTurnV3ActualBoundaryConstructorRejectsSplitTypedNilStore(
	t *testing.T,
) {
	fixture := newGovernedActualBoundaryFixtureV3(t)
	defer fixture.close(t)
	var typedNil *boundaryStoreSpyV3
	_, err := routegateway.NewGovernedModelTurnActualBoundaryDependenciesV3(
		fixture.store,
		fixture.store,
		fixture.gate,
		fixture.store,
		fixture.authorizer,
		fixture.store,
		typedNil,
	)
	if err == nil {
		t.Fatal("typed-nil composite Boundary store error=nil")
	}
}

func TestGovernedModelTurnV3ActualBoundaryHistoricalWinnerUsesOneStoreBeforeProvider(
	t *testing.T,
) {
	fixture := newGovernedActualBoundaryFixtureV3(t)
	defer fixture.close(t)
	spy := &boundaryStoreSpyV3{inner: fixture.store}
	fact := buildGovernedActualBoundaryFactV3(t, fixture)
	persisted, err :=
		modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
			context.Background(),
			spy,
			fact,
			func() time.Time { return gatewayNow },
		)
	if err != nil ||
		persisted.Disposition !=
			modelinvoker.GovernedModelTurnProviderBoundaryPersistenceCreatedV3 {
		t.Fatalf("seed boundary result=%+v err=%v", persisted, err)
	}
	dependencies, err :=
		routegateway.NewGovernedModelTurnActualBoundaryDependenciesV3(
			fixture.store,
			fixture.store,
			fixture.gate,
			fixture.store,
			fixture.authorizer,
			fixture.store,
			spy,
		)
	if err != nil {
		t.Fatal(err)
	}

	restartedState := &callState{}
	restarted := fakeGateway(
		t,
		fixture.routeCatalog,
		countingBinding{state: restartedState},
		&sameCoordinateMaterialDriftSecretV3{state: restartedState},
		restartedState,
		routegateway.WithGovernedModelTurnActualBoundaryV3(dependencies),
	)
	defer func() {
		if err := restarted.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	result, err := restarted.InvokeGovernedModelTurnActualBoundaryV3(
		context.Background(),
		fixture.command,
	)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorIndeterminate ||
		result.Boundary.Disposition !=
			modelinvoker.GovernedModelTurnProviderBoundaryPersistenceExistingV3 ||
		result.Boundary.Fact.RefV3() != persisted.Fact.RefV3() {
		t.Fatalf("historical winner result=%+v err=%v", result, err)
	}
	if spy.turnAttempt.Load() != 1 || spy.ensure.Load() != 1 ||
		spy.exact.Load() < 2 {
		t.Fatalf(
			"single Boundary store calls turn_attempt=%d ensure=%d exact=%d",
			spy.turnAttempt.Load(),
			spy.ensure.Load(),
			spy.exact.Load(),
		)
	}
	for key, calls := range restartedState.snapshot() {
		if calls != 0 {
			t.Fatalf("historical winner touched %s=%d want=0", key, calls)
		}
	}
}

type governedActualBoundaryFixtureV3 struct {
	gateway      *routegateway.Gateway
	store        *modelsqlite.Store
	state        *callState
	readers      *actualBoundaryReadersV3
	gate         *actualBoundaryGateV3
	authorizer   *modelinvoker.InvocationMaterialAuthorizerV2
	routeCatalog *catalog.Catalog
	dependencies routegateway.GovernedModelTurnActualBoundaryDependenciesV3
	command      routegateway.GovernedModelTurnActualBoundaryCommandV3
}

func buildGovernedActualBoundaryFactV3(
	t *testing.T,
	fixture governedActualBoundaryFixtureV3,
) modelinvoker.GovernedModelTurnProviderBoundaryFactV3 {
	t.Helper()
	ctx := context.Background()
	turn, err := fixture.store.InspectExactGovernedModelTurnV3(
		ctx,
		fixture.command.TurnRef,
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := fixture.store.InspectExactPreparedModelInvocationV1(
		ctx,
		turn.PreparedRef,
	)
	if err != nil {
		t.Fatal(err)
	}
	current, err := fixture.store.InspectExactPreparedModelInvocationCurrentV1(
		ctx,
		turn.CurrentRef,
	)
	if err != nil {
		t.Fatal(err)
	}
	ack, err := fixture.gate.InspectExactAck(ctx, fixture.gate.ack.Ref())
	if err != nil {
		t.Fatal(err)
	}
	checked := gatewayNow
	receipt, err := modelinvoker.SealPreparedModelInvocationDispatchReceiptAgainstV1(
		prepared,
		current,
		ack,
		modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1{
			PreparedRef:                   turn.PreparedRef,
			CurrentRef:                    turn.CurrentRef,
			AckRef:                        ack.Ref(),
			DispatchSequence:              turn.DispatchSequence,
			BoundaryKind:                  modelinvoker.GovernedModelTurnProviderBoundaryKindV2,
			ProviderAttemptOrdinal:        turn.ProviderAttemptOrdinal,
			AttemptRequestDigest:          turn.AttemptRequestDigest,
			ActualToolSurfaceDigest:       prepared.ActualToolSurfaceDigest,
			ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest,
			CheckedUnixNano:               checked.UnixNano(),
		},
		checked,
	)
	if err != nil {
		t.Fatal(err)
	}
	fact, err := modelinvoker.BuildGovernedModelTurnProviderBoundaryFactV3(
		ctx,
		modelinvoker.GovernedModelTurnProviderBoundaryVerificationReadersV3{
			TurnHistory:     fixture.store,
			PreparedHistory: fixture.store,
			PreparedCurrent: fixture.store,
			AckHistory:      fixture.gate,
		},
		modelinvoker.GovernedModelTurnProviderBoundaryDraftV3{
			TurnRef:         turn.RefV3(),
			DispatchReceipt: receipt,
			RuntimeRequest:  fixture.command.RuntimeRequest,
			Provider:        fixture.command.RuntimeRequest.Verifier,
			CheckedUnixNano: checked.UnixNano(),
		},
		checked,
	)
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func (f governedActualBoundaryFixtureV3) close(t *testing.T) {
	t.Helper()
	if err := f.gateway.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
}

func newGovernedActualBoundaryFixtureV3(
	t *testing.T,
) governedActualBoundaryFixtureV3 {
	return newGovernedActualBoundaryFixtureV3WithGateway(t, nil)
}

type actualBoundaryGatewayBuilderV3 func(
	*testing.T,
	*catalog.Catalog,
	*callState,
	routegateway.GovernedModelTurnActualBoundaryDependenciesV3,
) *routegateway.Gateway

func actualBoundaryGatewayWithOverrideFactoryV3(
	t *testing.T,
	routeCatalog *catalog.Catalog,
	override routegateway.AdapterFactory,
	state *callState,
	dependencies routegateway.GovernedModelTurnActualBoundaryDependenciesV3,
) *routegateway.Gateway {
	t.Helper()
	builtins, err := routegateway.NewBuiltinFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	factories := make([]routegateway.AdapterFactory, 0, len(builtins.IDs()))
	for _, id := range builtins.IDs() {
		if id == override.AdapterID() {
			factories = append(factories, override)
		} else {
			factories = append(factories, fakeFactory{id: id, state: state})
		}
	}
	registry, err := routegateway.NewFactoryRegistry(factories...)
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := routegateway.New(
		routeCatalog,
		countingBinding{state: state},
		countingSecret{state: state, version: "v1"},
		registry,
		routegateway.WithClock(func() time.Time { return gatewayNow }),
		routegateway.WithGovernedModelTurnActualBoundaryV3(dependencies),
	)
	if err != nil {
		t.Fatal(err)
	}
	return gateway
}

func newGovernedActualBoundaryFixtureV3WithGateway(
	t *testing.T,
	gatewayBuilder actualBoundaryGatewayBuilderV3,
) governedActualBoundaryFixtureV3 {
	return newGovernedActualBoundaryFixtureV3WithProviderInjection(
		t,
		gatewayBuilder,
		nil,
		nil,
	)
}

func newGovernedActualBoundaryFixtureV3WithProviderInjection(
	t *testing.T,
	gatewayBuilder actualBoundaryGatewayBuilderV3,
	mutateCall func(*modelinvoker.RouteCall),
	mutatePreparedProviderRequest func(*modelinvoker.Request),
) governedActualBoundaryFixtureV3 {
	t.Helper()
	call := governedCallV2()
	call.Request.Tools[0].Name = "workspace_read"
	if mutateCall != nil {
		mutateCall(&call)
	}
	routeCatalog := defaultCatalog(t)
	tempState := &callState{}
	temp := fakeGateway(
		t,
		routeCatalog,
		countingBinding{state: tempState},
		countingSecret{state: tempState, version: "v1"},
		tempState,
	)
	resolution, err := temp.Resolve(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if err := temp.Close(); err != nil {
		t.Fatal(err)
	}
	routeDigest, err := modelinvoker.DigestGovernedRouteSelectionV1(resolution.Route)
	if err != nil {
		t.Fatal(err)
	}
	concreteProviderRequest := call.Request
	concreteProviderRequest.Provider = resolution.Route.AdapterID
	concreteProviderRequest.Protocol = resolution.Route.Protocol
	concreteProviderRequest.Endpoint = resolution.Route.Endpoint
	concreteProviderRequest = cloneProviderInjectionRequestV3(
		concreteProviderRequest,
	)
	if mutatePreparedProviderRequest != nil {
		mutatePreparedProviderRequest(&concreteProviderRequest)
	}
	actualProviderInjectionDigest, err :=
		modelinvoker.ComputeActualProviderInjectionDigestV1(
			concreteProviderRequest,
		)
	if err != nil {
		t.Fatal(err)
	}
	requestTools, err := modelinvoker.DigestGovernedModelTurnRequestToolsV2(call)
	if err != nil {
		t.Fatal(err)
	}
	contextMapped, err := modelinvoker.DigestGovernedModelTurnContextV2(call)
	if err != nil {
		t.Fatal(err)
	}
	requestDigest := digestV2(t, "v3-unified-request", call)
	prepared, err := modelinvoker.SealPreparedModelInvocationFactV1(
		modelinvoker.PreparedModelInvocationFactV1{
			InvocationID:                  "routegateway-v3",
			InvocationDigest:              requestDigest,
			UnifiedRequestDigest:          requestDigest,
			RequestToolsDigest:            requestTools,
			PreparedPlanDigest:            digestV2(t, "v3-plan", call.Request.Input),
			RouteDigest:                   routeDigest,
			ProfileDigest:                 digestV2(t, "v3-profile", call.Request.Model),
			ActualToolSurfaceDigest:       digestV2(t, "v3-surface", call.Request.Tools),
			ActualProviderInjectionDigest: actualProviderInjectionDigest,
			CapabilitySnapshotRef: modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{
				ContractVersion: "1.0.0",
				ID:              "v3-capability",
				Revision:        1,
				Digest:          digestV2(t, "v3-capability", call.RouteID),
			},
			RegistrySnapshotRef: runtimeports.RegistrySnapshotRefV1{
				Owner:           ownerV2("registry", "v3"),
				ContractVersion: "1.0.0",
				ID:              "v3-registry",
				Revision:        1,
				Digest:          digestV2(t, "v3-registry", call.RouteID),
			},
			CreatedUnixNano:  gatewayNow.Add(-2 * time.Minute).UnixNano(),
			NotAfterUnixNano: gatewayNow.Add(10 * time.Minute).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(
		modelinvoker.PreparedModelInvocationCurrentProjectionV1{
			Prepared:                      prepared.Ref(),
			CapabilitySnapshotRef:         prepared.CapabilitySnapshotRef,
			RegistrySnapshotRef:           prepared.RegistrySnapshotRef,
			ActualToolSurfaceDigest:       prepared.ActualToolSurfaceDigest,
			ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest,
			CheckedUnixNano:               gatewayNow.Add(-time.Minute).UnixNano(),
			ExpiresUnixNano:               gatewayNow.Add(8 * time.Minute).UnixNano(),
			NotAfterUnixNano:              prepared.NotAfterUnixNano,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	contextOwner := ownerV2("praxis.context", "v3-context")
	toolOwner := ownerV2("praxis.tool", "v3-tool")
	lineage, err := modelinvoker.SealInvocationMaterialSourceLineageV2(
		modelinvoker.InvocationMaterialSourceLineageV2{
			ContextFrame: actualBoundarySourceRefV3(
				contextOwner,
				modelinvoker.InvocationMaterialContextFrameKindV2,
				"frame",
				digestV2(t, "v3-frame", call.Request.Input),
			),
			ContextMaterial: actualBoundarySourceRefV3(
				contextOwner,
				modelinvoker.InvocationMaterialContextMaterialKindV2,
				"material",
				digestV2(t, "v3-material", call.Request.Instructions),
			),
			ToolInjectionMaterial: actualBoundarySourceRefV3(
				toolOwner,
				modelinvoker.InvocationMaterialToolInjectionMaterialKindV2,
				"injection",
				digestV2(t, "v3-injection", call.Request.Tools),
			),
			ToolSurface: actualBoundarySourceRefV3(
				toolOwner,
				modelinvoker.InvocationMaterialToolSurfaceKindV2,
				"surface",
				digestV2(t, "v3-surface-ref", call.Request.Tools),
			),
			ContextMappedInputDigest: contextMapped,
			ExpectedInjectionDigest:  prepared.ActualToolSurfaceDigest,
			CompiledToolsDigest:      digestV2(t, "v3-compiled", call.Request.Tools),
			RequestToolsDigest:       requestTools,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	readers := &actualBoundaryReadersV3{
		expectedInjection: lineage.ExpectedInjectionDigest,
		compiledTools:     lineage.CompiledToolsDigest,
		checked:           gatewayNow.Add(-time.Minute).UnixNano(),
		expires:           gatewayNow.Add(9 * time.Minute).UnixNano(),
	}
	closure := modelinvoker.InvocationMaterialExactClosureV2{
		SourceLineage: lineage,
		ProviderInjection: actualBoundarySourceRefV3(
			ownerV2("praxis.model", "v3-model"),
			"model-source",
			"provider",
			prepared.ActualProviderInjectionDigest,
		),
		Route: actualBoundarySourceRefV3(
			ownerV2("praxis.model", "v3-model"),
			"model-source",
			"route",
			prepared.RouteDigest,
		),
		Profile: actualBoundarySourceRefV3(
			ownerV2("praxis.model", "v3-model"),
			"model-source",
			"profile",
			prepared.ProfileDigest,
		),
	}
	authorizer, err := modelinvoker.NewInvocationMaterialAuthorizerV2(
		modelinvoker.InvocationMaterialAuthorizerConfigV2{
			ContextPair:       readers,
			ToolPair:          readers,
			ProviderInjection: readers,
			Route:             readers,
			Profile:           readers,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	store, err := modelsqlite.Open(
		context.Background(),
		modelsqlite.Config{Path: t.TempDir() + "/routegateway-v3.db"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsurePreparedModelInvocationV1(
		context.Background(),
		prepared,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsurePreparedModelInvocationCurrentV1(
		context.Background(),
		current,
	); err != nil {
		t.Fatal(err)
	}
	material, err := modelinvoker.AuthorizeAndEnsureInvocationMaterialV2(
		context.Background(),
		authorizer,
		store,
		prepared,
		current,
		call,
		closure,
		func() time.Time { return gatewayNow },
	)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := modelinvoker.NewPreparedGovernedModelTurnV3(
		modelinvoker.GovernedModelTurnCommandV3{
			PreparedRef:            prepared.Ref(),
			CurrentRef:             current.Ref(),
			MaterialRef:            material.RefV2(),
			AttemptRequestDigest:   prepared.UnifiedRequestDigest,
			RouteCallDigest:        material.RouteCallDigest,
			DispatchSequence:       1,
			ProviderAttemptOrdinal: 1,
		},
		gatewayNow.Add(-time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsurePreparedGovernedModelTurnV3(
		context.Background(),
		turn,
	); err != nil {
		t.Fatal(err)
	}
	ack, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(
		modelinvoker.PreparedModelInvocationCommitAckV1{
			PreparedRef: prepared.Ref(),
			CurrentRef:  current.Ref(),
			GateImplementationRef: modelinvoker.PreparedModelInvocationGateImplementationRefV1{
				Owner:           ownerV2("praxis.harness", "v3-gate"),
				ContractVersion: "1.0.0",
				ID:              "v3-gate",
				Revision:        1,
				Digest:          digestV2(t, "v3-gate", prepared.Ref()),
			},
			SurfaceBindingRef: modelinvoker.PreparedModelInvocationSurfaceBindingRefV1{
				Owner:           toolOwner,
				ContractVersion: "1.0.0",
				ID:              "v3-surface",
				Revision:        1,
				Digest:          prepared.ActualToolSurfaceDigest,
			},
			CheckedUnixNano:  gatewayNow.Add(-time.Minute).UnixNano(),
			ExpiresUnixNano:  gatewayNow.Add(6 * time.Minute).UnixNano(),
			NotAfterUnixNano: current.NotAfterUnixNano,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	gate := &actualBoundaryGateV3{ack: ack}
	provider := actualBoundaryProviderBindingV3(t)
	runtimeRequest := actualBoundaryRuntimeRequestV3(t, provider)
	state := &callState{}
	state.governedV2.Store(true)
	dependencies, err :=
		routegateway.NewGovernedModelTurnActualBoundaryDependenciesV3(
			store,
			store,
			gate,
			store,
			authorizer,
			store,
			store,
		)
	if err != nil {
		t.Fatal(err)
	}
	var gateway *routegateway.Gateway
	if gatewayBuilder == nil {
		gateway = fakeGateway(
			t,
			routeCatalog,
			countingBinding{state: state},
			countingSecret{state: state, version: "v1"},
			state,
			routegateway.WithGovernedModelTurnActualBoundaryV3(dependencies),
		)
	} else {
		gateway = gatewayBuilder(t, routeCatalog, state, dependencies)
	}
	return governedActualBoundaryFixtureV3{
		gateway:      gateway,
		store:        store,
		state:        state,
		readers:      readers,
		gate:         gate,
		authorizer:   authorizer,
		routeCatalog: routeCatalog,
		dependencies: dependencies,
		command: routegateway.GovernedModelTurnActualBoundaryCommandV3{
			TurnRef:        turn.RefV3(),
			RuntimeRequest: runtimeRequest,
		},
	}
}

func cloneProviderInjectionRequestV3(
	request modelinvoker.Request,
) modelinvoker.Request {
	cloned := request
	cloned.Tools = make([]modelinvoker.Tool, len(request.Tools))
	for index, tool := range request.Tools {
		cloned.Tools[index] = tool
		cloned.Tools[index].Parameters = append([]byte(nil), tool.Parameters...)
		if tool.Strict != nil {
			strict := *tool.Strict
			cloned.Tools[index].Strict = &strict
		}
	}
	if request.ParallelToolCalls != nil {
		parallel := *request.ParallelToolCalls
		cloned.ParallelToolCalls = &parallel
	}
	if request.ProviderOptions != nil {
		cloned.ProviderOptions = make(
			modelinvoker.ProviderOptions,
			len(request.ProviderOptions),
		)
		for provider, options := range request.ProviderOptions {
			cloned.ProviderOptions[provider] = append([]byte(nil), options...)
		}
	}
	return cloned
}

type sameCoordinateMaterialDriftSecretV3 struct {
	state *callState
	calls atomic.Int64
}

type actualBoundaryReleaseErrorFactoryV3 struct {
	fakeFactory
	closeErr   error
	closeCalls atomic.Int64
}

func (f *actualBoundaryReleaseErrorFactoryV3) Build(
	_ context.Context,
	input routegateway.FactoryInput,
) (routegateway.FactoryResult, error) {
	f.state.factory.Add(1)
	return routegateway.FactoryResult{
		Provider: &fakeProvider{id: f.id, state: f.state},
		Closer: actualBoundaryReleaseErrorCloserV3{
			err:   f.closeErr,
			calls: &f.closeCalls,
		},
		Endpoint: input.Endpoint,
	}, nil
}

type actualBoundaryReleaseErrorCloserV3 struct {
	err   error
	calls *atomic.Int64
}

func (c actualBoundaryReleaseErrorCloserV3) Close() error {
	c.calls.Add(1)
	return c.err
}

type actualBoundaryGateV3 struct {
	ack             modelinvoker.PreparedModelInvocationCommitAckV1
	inspectOverride *modelinvoker.PreparedModelInvocationCommitAckV1
	commit          atomic.Uint64
	inspect         atomic.Uint64
}

type actualBoundaryTurnReaderV3 struct {
	inner    modelinvoker.GovernedModelTurnRepositoryV3
	override modelinvoker.GovernedModelTurnOutcomeV3
}

func (r *actualBoundaryTurnReaderV3) EnsurePreparedGovernedModelTurnV3(
	ctx context.Context,
	outcome modelinvoker.GovernedModelTurnOutcomeV3,
) (modelinvoker.GovernedModelTurnMutationV3, error) {
	return r.inner.EnsurePreparedGovernedModelTurnV3(ctx, outcome)
}

func (r *actualBoundaryTurnReaderV3) InspectGovernedModelTurnAttemptV3(
	ctx context.Context,
	ref modelinvoker.GovernedModelTurnAttemptRefV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	return r.inner.InspectGovernedModelTurnAttemptV3(ctx, ref)
}

func (r *actualBoundaryTurnReaderV3) InspectExactGovernedModelTurnV3(
	context.Context,
	modelinvoker.GovernedModelTurnRefV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	return r.override, nil
}

func (g *actualBoundaryGateV3) Commit(
	_ context.Context,
	prepared modelinvoker.PreparedModelInvocationRefV1,
	current modelinvoker.PreparedModelInvocationCurrentRefV1,
) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	g.commit.Add(1)
	if g.ack.PreparedRef != prepared || g.ack.CurrentRef != current {
		return modelinvoker.PreparedModelInvocationCommitAckV1{},
			errors.New("actual boundary gate lineage drift")
	}
	return g.ack, nil
}

func (g *actualBoundaryGateV3) InspectExactAck(
	_ context.Context,
	ref modelinvoker.PreparedModelInvocationCommitAckRefV1,
) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	g.inspect.Add(1)
	if g.inspectOverride != nil {
		return *g.inspectOverride, nil
	}
	if g.ack.Ref() != ref {
		return modelinvoker.PreparedModelInvocationCommitAckV1{},
			errors.New("actual boundary ACK absent")
	}
	return g.ack, nil
}

func (r *sameCoordinateMaterialDriftSecretV3) ResolveSecret(
	_ context.Context,
	request routegateway.SecretRequest,
) (routegateway.SecretMaterial, error) {
	generation := r.calls.Add(1)
	r.state.secret.Add(1)
	value := "offline-api-key-generation-" + string(rune('0'+generation))
	if len(request.Profile.KeyPrefixes) > 0 {
		value = request.Profile.KeyPrefixes[0] + "generation-" +
			string(rune('0'+generation))
	}
	return routegateway.NewSecretMaterial(
		request.Profile.ID,
		request.Profile.Type,
		"same-version",
		gatewayNow.Add(time.Hour),
		map[upstream.CredentialPurpose][]byte{
			upstream.CredentialPurposeAPIKey: []byte(value),
		},
	)
}

type boundaryStoreSpyV3 struct {
	inner       *modelsqlite.Store
	turnAttempt atomic.Int64
	ensure      atomic.Int64
	attempt     atomic.Int64
	exact       atomic.Int64
}

func (r *boundaryStoreSpyV3) InspectGovernedModelTurnProviderBoundaryTurnAttemptV3(
	ctx context.Context,
	ref modelinvoker.GovernedModelTurnAttemptRefV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	r.turnAttempt.Add(1)
	return r.inner.InspectGovernedModelTurnProviderBoundaryTurnAttemptV3(ctx, ref)
}

func (r *boundaryStoreSpyV3) EnsureGovernedModelTurnProviderBoundaryFactV3(
	ctx context.Context,
	fact modelinvoker.GovernedModelTurnProviderBoundaryFactV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryMutationV3, error) {
	r.ensure.Add(1)
	return r.inner.EnsureGovernedModelTurnProviderBoundaryFactV3(ctx, fact)
}

func (r *boundaryStoreSpyV3) InspectGovernedModelTurnProviderBoundaryAttemptV3(
	ctx context.Context,
	ref runtimeports.ModelProviderBoundaryCurrentRefV1,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	r.attempt.Add(1)
	return r.inner.InspectGovernedModelTurnProviderBoundaryAttemptV3(ctx, ref)
}

func (r *boundaryStoreSpyV3) InspectExactGovernedModelTurnProviderBoundaryV3(
	ctx context.Context,
	ref modelinvoker.GovernedModelTurnProviderBoundaryRefV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	r.exact.Add(1)
	return r.inner.InspectExactGovernedModelTurnProviderBoundaryV3(ctx, ref)
}

type actualBoundaryReadersV3 struct {
	expectedInjection       core.Digest
	compiledTools           core.Digest
	checked                 int64
	expires                 int64
	contextAlternateChecked int64
	contextAlternateExpires int64
	contextCorruptDigestAt  int64
	toolAlternateChecked    int64
	toolAlternateExpires    int64
	toolCorruptDigestAt     int64
	alternateAt             int64
	calls                   atomic.Int64
	contextCalls            atomic.Int64
	toolCalls               atomic.Int64
	failAt                  atomic.Int64
}

func (r *actualBoundaryReadersV3) contextExpires(call int64) int64 {
	if r.alternateAt > 0 && call >= r.alternateAt &&
		r.contextAlternateExpires > 0 {
		return r.contextAlternateExpires
	}
	return r.expires
}

func (r *actualBoundaryReadersV3) contextChecked(call int64) int64 {
	if r.alternateAt > 0 && call >= r.alternateAt &&
		r.contextAlternateChecked > 0 {
		return r.contextAlternateChecked
	}
	return r.checked
}

func (r *actualBoundaryReadersV3) toolExpires(call int64) int64 {
	if r.alternateAt > 0 && call >= r.alternateAt &&
		r.toolAlternateExpires > 0 {
		return r.toolAlternateExpires
	}
	return r.expires
}

func (r *actualBoundaryReadersV3) toolChecked(call int64) int64 {
	if r.alternateAt > 0 && call >= r.alternateAt &&
		r.toolAlternateChecked > 0 {
		return r.toolAlternateChecked
	}
	return r.checked
}

func (r *actualBoundaryReadersV3) InspectExactInvocationContextPairV2(
	_ context.Context,
	frame modelinvoker.InvocationMaterialExactSourceRefV1,
	material modelinvoker.InvocationMaterialExactSourceRefV1,
	mapped core.Digest,
) (modelinvoker.InvocationMaterialContextPairProjectionV2, error) {
	call := r.calls.Add(1)
	r.contextCalls.Add(1)
	if r.failAt.Load() > 0 && call >= r.failAt.Load() {
		return modelinvoker.InvocationMaterialContextPairProjectionV2{},
			core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "context pair drifted")
	}
	projection, err := modelinvoker.SealInvocationMaterialContextPairProjectionV2(
		modelinvoker.InvocationMaterialContextPairProjectionV2{
			ContextFrame:             frame,
			ContextMaterial:          material,
			ContextMappedInputDigest: mapped,
			CheckedUnixNano:          r.contextChecked(call),
			ExpiresUnixNano:          r.contextExpires(call),
		},
	)
	if err == nil && call == r.contextCorruptDigestAt {
		projection.ProjectionDigest = ""
	}
	return projection, err
}

func (r *actualBoundaryReadersV3) InspectExactInvocationToolPairV2(
	_ context.Context,
	injection modelinvoker.InvocationMaterialExactSourceRefV1,
	surface modelinvoker.InvocationMaterialExactSourceRefV1,
	requestTools core.Digest,
) (modelinvoker.InvocationMaterialToolPairProjectionV2, error) {
	call := r.calls.Add(1)
	r.toolCalls.Add(1)
	if r.failAt.Load() > 0 && call >= r.failAt.Load() {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{},
			core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "tool pair drifted")
	}
	projection, err := modelinvoker.SealInvocationMaterialToolPairProjectionV2(
		modelinvoker.InvocationMaterialToolPairProjectionV2{
			ToolInjectionMaterial:   injection,
			ToolSurface:             surface,
			ExpectedInjectionDigest: r.expectedInjection,
			CompiledToolsDigest:     r.compiledTools,
			RequestToolsDigest:      requestTools,
			CheckedUnixNano:         r.toolChecked(call),
			ExpiresUnixNano:         r.toolExpires(call),
		},
	)
	if err == nil && call == r.toolCorruptDigestAt {
		projection.ProjectionDigest = ""
	}
	return projection, err
}

func (r *actualBoundaryReadersV3) projection(
	ref modelinvoker.InvocationMaterialExactSourceRefV1,
) modelinvoker.InvocationMaterialExactSourceProjectionV1 {
	return modelinvoker.InvocationMaterialExactSourceProjectionV1{
		Ref:             ref,
		CheckedUnixNano: r.checked,
		ExpiresUnixNano: r.expires,
	}
}

func (r *actualBoundaryReadersV3) InspectExactInvocationProviderInjectionV1(
	_ context.Context,
	ref modelinvoker.InvocationMaterialExactSourceRefV1,
) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	return r.projection(ref), nil
}

func (r *actualBoundaryReadersV3) InspectExactInvocationRouteV1(
	_ context.Context,
	ref modelinvoker.InvocationMaterialExactSourceRefV1,
) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	return r.projection(ref), nil
}

func (r *actualBoundaryReadersV3) InspectExactInvocationProfileV1(
	_ context.Context,
	ref modelinvoker.InvocationMaterialExactSourceRefV1,
) (modelinvoker.InvocationMaterialExactSourceProjectionV1, error) {
	return r.projection(ref), nil
}

type actualBoundaryClockV3 struct {
	values []time.Time
	calls  atomic.Int64
}

func (c *actualBoundaryClockV3) Now() time.Time {
	index := c.calls.Add(1) - 1
	if index < int64(len(c.values)) {
		return c.values[index]
	}
	return c.values[len(c.values)-1]
}

func actualBoundaryRuntimeRequestV3(
	t *testing.T,
	provider runtimeports.ProviderBindingRefV2,
) runtimeports.InspectCurrentModelProviderActualPointRequestV1 {
	t.Helper()
	lease := &core.SandboxLeaseRef{ID: "v3-lease", Epoch: 1}
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant", ID: "agent", Epoch: 1},
		Lineage: core.LineageRef{
			ID:         "v3-lineage",
			PlanDigest: digestV2(t, "v3-lineage", "plan"),
		},
		Instance:       core.InstanceRef{ID: "v3-instance", Epoch: 1},
		SandboxLease:   lease,
		AuthorityEpoch: 1,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{
		Kind:                      runtimeports.OperationScopeRunV3,
		ExecutionScope:            scope,
		ExecutionScopeDigest:      scopeDigest,
		RunID:                     "v3-run",
		SubjectRevision:           1,
		CurrentProjectionRef:      "v3-run-current",
		CurrentProjectionRevision: 1,
		CurrentProjectionDigest:   digestV2(t, "v3-run-current", "current"),
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	attempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest,
		EffectID:        "v3-effect",
		IntentRevision:  1,
		IntentDigest:    digestV2(t, "v3-intent", "intent"),
		PermitID:        "v3-permit",
		PermitRevision:  1,
		PermitDigest:    digestV2(t, "v3-permit", "permit"),
		AttemptID:       "v3-runtime-attempt",
	}
	return runtimeports.InspectCurrentModelProviderActualPointRequestV1{
		Operation:                  operation,
		EffectID:                   attempt.EffectID,
		ExpectedEffectRevision:     3,
		PermitID:                   attempt.PermitID,
		ExpectedPermitFactRevision: 2,
		PermitDigest:               attempt.PermitDigest,
		AdmissionDigest:            digestV2(t, "v3-admission", "admission"),
		ReviewAuthorization: runtimeports.OperationReviewAuthorizationRefV4{
			ID:       "v3-review",
			Revision: 1,
			Digest:   digestV2(t, "v3-review", "review"),
		},
		Attempt:                   attempt,
		Verifier:                  provider,
		FenceDigest:               digestV2(t, "v3-fence", "fence"),
		RequestedNotAfterUnixNano: gatewayNow.Add(5 * time.Minute).UnixNano(),
	}
}

func actualBoundaryProviderBindingV3(
	t *testing.T,
) runtimeports.ProviderBindingRefV2 {
	t.Helper()
	return runtimeports.ProviderBindingRefV2{
		BindingSetID:       "v3-binding-set",
		BindingSetRevision: 1,
		ComponentID:        "praxis.model/provider",
		ManifestDigest:     digestV2(t, "v3-manifest", "manifest"),
		ArtifactDigest:     digestV2(t, "v3-artifact", "artifact"),
		Capability:         runtimeports.ModelInvokeCapabilityV1,
	}
}

func actualBoundarySourceRefV3(
	owner core.OwnerRef,
	kind string,
	id string,
	digest core.Digest,
) modelinvoker.InvocationMaterialExactSourceRefV1 {
	return modelinvoker.InvocationMaterialExactSourceRefV1{
		Owner:    owner,
		Kind:     kind,
		ID:       id,
		Revision: 1,
		Digest:   digest,
	}
}

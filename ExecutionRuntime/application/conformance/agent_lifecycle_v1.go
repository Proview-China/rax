package conformance

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var agentLifecycleAdapterAllowedImportsV1 = [...]string{
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core",
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports",
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract",
	"github.com/Proview-China/rax/ExecutionRuntime/application/ports",
}

func AgentLifecycleAdapterAllowedImportsV1() []string {
	result := make([]string, len(agentLifecycleAdapterAllowedImportsV1))
	copy(result, agentLifecycleAdapterAllowedImportsV1[:])
	return result
}

func CheckAgentLifecycleAdapterImportsV1(imports []string) error {
	allowed := AgentLifecycleAdapterAllowedImportsV1()
	for _, candidate := range imports {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Agent lifecycle adapter import is empty")
		}
		if !strings.HasPrefix(candidate, "github.com/Proview-China/rax/ExecutionRuntime/") {
			continue
		}
		permitted := false
		for _, prefix := range allowed {
			if candidate == prefix || strings.HasPrefix(candidate, prefix+"/") {
				permitted = true
				break
			}
		}
		if !permitted {
			return core.NewError(core.ErrorForbidden, core.ReasonComponentMismatch, "Agent lifecycle adapter imports an Owner implementation package")
		}
	}
	return nil
}

func CheckAgentActivationClosureV1(request contract.AgentActivationStartRequestV1, result contract.AgentActivationResultV1, now time.Time) error {
	return result.ValidateFor(request, now)
}

func CheckAgentTerminationClosureV1(request contract.AgentTerminationRequestV1, result contract.AgentTerminationResultV1, now time.Time) error {
	return result.ValidateFor(request, now)
}

func CheckAgentActivationCoordinationV1(request contract.AgentActivationStartRequestV1, fact contract.AgentActivationCoordinationFactV1, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := fact.Validate(); err != nil {
		return err
	}
	if fact.Request.RequestDigest != request.RequestDigest || fact.Result == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReadyEvidenceIncomplete, "Agent activation coordination is not an exact completed closure")
	}
	return fact.Result.ValidateFor(request, now)
}

type AgentLifecycleTestingT interface {
	Helper()
	Fatalf(string, ...any)
}

// RunAgentLifecyclePortConformanceV1 checks strict start-or-inspect and the
// independent stop attempt. It does not certify durability or production I/O.
func RunAgentLifecyclePortConformanceV1(t AgentLifecycleTestingT, port applicationports.AgentLifecyclePortV1, start contract.AgentActivationStartRequestV1, stop contract.AgentTerminationRequestV1, now time.Time) {
	t.Helper()
	ctx := context.Background()
	started, err := port.StartOrInspectAgentActivationV1(ctx, start)
	if err != nil || CheckAgentActivationClosureV1(start, started, now) != nil {
		t.Fatalf("Agent activation Start conformance failed: %#v %v", started, err)
		return
	}
	inspected, err := port.InspectAgentActivationV1(ctx, start)
	if err != nil || inspected.ResultDigest != started.ResultDigest {
		t.Fatalf("Agent activation exact Inspect conformance failed: %#v %v", inspected, err)
		return
	}
	replayed, err := port.StartOrInspectAgentActivationV1(ctx, start)
	if err != nil || replayed.ResultDigest != started.ResultDigest {
		t.Fatalf("Agent activation replay conformance failed: %#v %v", replayed, err)
		return
	}
	drift := start
	drift.IdempotencyKey += "-drift"
	drift.RequestDigest = ""
	drift, err = contract.SealAgentActivationStartRequestV1(drift)
	if err != nil {
		t.Fatalf("seal Agent activation drift fixture: %v", err)
		return
	}
	if _, err = port.StartOrInspectAgentActivationV1(ctx, drift); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Agent activation changed-content replay was accepted: %v", err)
		return
	}
	stopped, err := port.StopOrInspectAgentV1(ctx, stop)
	if err != nil || CheckAgentTerminationClosureV1(stop, stopped, now) != nil {
		t.Fatalf("Agent termination conformance failed: %#v %v", stopped, err)
		return
	}
	stopReplay, err := port.StopOrInspectAgentV1(ctx, stop)
	if err != nil || stopReplay.ResultDigest != stopped.ResultDigest {
		t.Fatalf("Agent termination replay conformance failed: %#v %v", stopReplay, err)
	}
}

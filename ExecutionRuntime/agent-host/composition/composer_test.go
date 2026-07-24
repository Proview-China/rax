package composition_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/composition"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/registry"
)

type recorder struct {
	mu                sync.Mutex
	constructed       []string
	cleaned           []string
	fail              string
	requireWriteAhead bool
	planned           map[string]string
}
type componentFactory struct {
	key    contract.FactoryKeyV1
	record *recorder
}
type componentHandle struct {
	node   string
	ref    contract.ExactRefV1
	record *recorder
}
type panicFactory struct{ cleanup bool }
type panicHandle struct{ ref contract.ExactRefV1 }

func (p panicFactory) StartOrInspectConstructionV1(context.Context, ports.ConstructRequestV1) (ports.ComponentHandleV1, error) {
	if !p.cleanup {
		panic("construct")
	}
	return &panicHandle{ref: exactRef("praxis.component/instance", "panic")}, nil
}
func (p panicFactory) InspectConstructionV1(context.Context, ports.ConstructRequestV1) (ports.ComponentHandleV1, error) {
	panic("reattach")
}
func (h *panicHandle) RefV1() contract.ExactRefV1                                { return h.ref }
func (h *panicHandle) CleanupV1(context.Context) (contract.CleanupItemV1, error) { panic("cleanup") }

func (f componentFactory) StartOrInspectConstructionV1(_ context.Context, request ports.ConstructRequestV1) (ports.ComponentHandleV1, error) {
	f.record.mu.Lock()
	defer f.record.mu.Unlock()
	if f.record.requireWriteAhead && (f.record.planned == nil || f.record.planned[request.Node.NodeID] != request.Attempt.AttemptID) {
		return nil, contract.NewError(contract.ErrorPrecondition, "attempt_not_written", "factory called before attempt journal")
	}
	if request.Node.NodeID == f.record.fail {
		return nil, contract.NewError(contract.ErrorUnavailable, "injected_construction_failure", "injected")
	}
	f.record.constructed = append(f.record.constructed, request.Node.NodeID)
	return &componentHandle{node: request.Node.NodeID, ref: exactRef("praxis.component/instance", request.Node.NodeID), record: f.record}, nil
}
func (f componentFactory) InspectConstructionV1(_ context.Context, request ports.ConstructRequestV1) (ports.ComponentHandleV1, error) {
	if request.Attempt.ComponentRef == nil {
		return nil, contract.NewError(contract.ErrorNotFound, "missing", "missing")
	}
	return &componentHandle{node: request.Node.NodeID, ref: *request.Attempt.ComponentRef, record: f.record}, nil
}
func (h *componentHandle) RefV1() contract.ExactRefV1 { return h.ref }
func (h *componentHandle) CleanupV1(context.Context) (contract.CleanupItemV1, error) {
	h.record.mu.Lock()
	defer h.record.mu.Unlock()
	h.record.cleaned = append(h.record.cleaned, h.node)
	return contract.CleanupItemV1{NodeID: h.node, ComponentRef: h.ref, State: contract.CleanupClosedV1}, nil
}

func exactRef(kind, id string) contract.ExactRefV1 {
	digest, _ := contract.DigestJSONV1(id)
	return contract.ExactRefV1{Kind: kind, ID: id, Revision: 1, Digest: digest}
}
func factoryKey(id string) contract.FactoryKeyV1 {
	digest, _ := contract.DigestJSONV1("artifact-" + id)
	return contract.FactoryKeyV1{ComponentID: id, ArtifactDigest: digest, Contract: "praxis.fixture/v1", Capability: "praxis.fixture/start"}
}
func graph() contract.ConstructionGraphV1 {
	return contract.ConstructionGraphV1{GraphRef: exactRef("praxis.harness/graph", "graph"), Nodes: []contract.ComponentNodeV1{{NodeID: "sandbox", Factory: factoryKey("sandbox")}, {NodeID: "harness", Factory: factoryKey("harness"), Dependencies: []string{"sandbox"}}, {NodeID: "tool", Factory: factoryKey("tool"), Dependencies: []string{"harness"}}}}
}
func composerFixture(t *testing.T, record *recorder) *composition.ComposerV1 {
	t.Helper()
	reg := registry.NewV1()
	for _, node := range graph().Nodes {
		if err := reg.RegisterV1(node.Factory, componentFactory{key: node.Factory, record: record}); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.SealV1(); err != nil {
		t.Fatal(err)
	}
	value, err := composition.NewComposerV1(reg)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestComposerConstructsDAGAndCleansReverseOrder(t *testing.T) {
	record := &recorder{}
	composer := composerFixture(t, record)
	var snapshots [][]contract.ConstructedComponentV1
	value, _, err := composer.ConstructV1(context.Background(), "host-1", "start-1", graph(), nil, nil, func(_ []contract.ConstructionAttemptV1, items []contract.ConstructedComponentV1) error {
		snapshots = append(snapshots, items)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := join(record.constructed); got != "sandbox,harness,tool" {
		t.Fatalf("construction order %s", got)
	}
	if len(snapshots) != 6 || len(snapshots[len(snapshots)-1]) != 3 {
		t.Fatal("progress did not preserve each append-only construction")
	}
	summary, err := value.CleanupV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := join(record.cleaned); got != "tool,harness,sandbox" {
		t.Fatalf("cleanup order %s", got)
	}
	if summary.State != contract.CleanupClosedV1 {
		t.Fatalf("cleanup state %s", summary.State)
	}
}

func TestComposerFailureCleansConstructedPrefixReverse(t *testing.T) {
	record := &recorder{fail: "tool"}
	composer := composerFixture(t, record)
	value, summary, err := composer.ConstructV1(context.Background(), "host-1", "start-1", graph(), nil, nil, nil)
	if err == nil || value != nil {
		t.Fatal("construction failure accepted")
	}
	if got := join(record.cleaned); got != "harness,sandbox" {
		t.Fatalf("partial cleanup order %s", got)
	}
	if summary.State != contract.CleanupIndeterminateV1 {
		t.Fatalf("cleanup state %s", summary.State)
	}
}

func TestComposerWritesDeterministicAttemptBeforeFactoryAndPreservesUnknown(t *testing.T) {
	record := &recorder{fail: "tool", requireWriteAhead: true, planned: map[string]string{}}
	composer := composerFixture(t, record)
	var latest []contract.ConstructionAttemptV1
	_, summary, err := composer.ConstructV1(context.Background(), "host-1", "attempt-start", graph(), nil, nil, func(attempts []contract.ConstructionAttemptV1, _ []contract.ConstructedComponentV1) error {
		record.mu.Lock()
		for _, attempt := range attempts {
			record.planned[attempt.NodeID] = attempt.AttemptID
		}
		record.mu.Unlock()
		latest = append([]contract.ConstructionAttemptV1(nil), attempts...)
		return nil
	})
	if err == nil || summary.State != contract.CleanupIndeterminateV1 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	if len(latest) != 3 || latest[2].State != contract.AttemptUnknownV1 {
		t.Fatalf("attempts=%+v", latest)
	}
	again, makeErr := contract.NewConstructionAttemptV1("host-1", "attempt-start", graph().GraphRef, graph().Nodes[2], nil)
	if makeErr != nil {
		t.Fatal(makeErr)
	}
	if again.AttemptID == latest[2].AttemptID {
		t.Fatal("different dependency set reused attempt identity")
	}
}

func TestConstructedProgressFailureCleansReturnedHandle(t *testing.T) {
	record := &recorder{}
	composer := composerFixture(t, record)
	_, summary, err := composer.ConstructV1(context.Background(), "host-1", "progress-fail", graph(), nil, nil, func(_ []contract.ConstructionAttemptV1, components []contract.ConstructedComponentV1) error {
		if len(components) > 0 {
			return errors.New("persist failed")
		}
		return nil
	})
	if err == nil || summary.State != contract.CleanupIndeterminateV1 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	if got := join(record.cleaned); got != "sandbox" {
		t.Fatalf("returned handle leaked, cleaned=%s", got)
	}
}

func TestUnknownProgressFailureIsReturnedAndNotClaimedDurable(t *testing.T) {
	record := &recorder{fail: "sandbox"}
	composer := composerFixture(t, record)
	_, summary, err := composer.ConstructV1(context.Background(), "host-1", "unknown-persist-fail", graph(), nil, nil, func(attempts []contract.ConstructionAttemptV1, _ []contract.ConstructedComponentV1) error {
		if len(attempts) > 0 && attempts[0].State == contract.AttemptUnknownV1 {
			return errors.New("journal unavailable")
		}
		return nil
	})
	if err == nil || summary.State != contract.CleanupIndeterminateV1 || !strings.Contains(err.Error(), "unknown_progress_not_persisted") {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
}

func TestComposerRecoveryRequiresExactGraphPrefix(t *testing.T) {
	record := &recorder{}
	composer := composerFixture(t, record)
	bad := []contract.ConstructedComponentV1{{NodeID: "harness", Factory: factoryKey("harness"), ComponentRef: exactRef("praxis.component/instance", "harness")}}
	if _, _, err := composer.ConstructV1(context.Background(), "host-1", "start-1", graph(), nil, bad, nil); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatal("non-prefix recovery accepted")
	}
}

func TestComposerIsolatesFactoryAndCleanupPanics(t *testing.T) {
	one := contract.ConstructionGraphV1{GraphRef: exactRef("praxis.harness/graph", "panic-graph"), Nodes: []contract.ComponentNodeV1{{NodeID: "panic-node", Factory: factoryKey("panic-component")}}}
	reg := registry.NewV1()
	if err := reg.RegisterV1(one.Nodes[0].Factory, panicFactory{}); err != nil {
		t.Fatal(err)
	}
	if err := reg.SealV1(); err != nil {
		t.Fatal(err)
	}
	composer, _ := composition.NewComposerV1(reg)
	if _, _, err := composer.ConstructV1(context.Background(), "host-1", "start-1", one, nil, nil, nil); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("constructor panic error=%v", err)
	}
	reg2 := registry.NewV1()
	if err := reg2.RegisterV1(one.Nodes[0].Factory, panicFactory{cleanup: true}); err != nil {
		t.Fatal(err)
	}
	_ = reg2.SealV1()
	composer2, _ := composition.NewComposerV1(reg2)
	value, _, err := composer2.ConstructV1(context.Background(), "host-1", "start-1", one, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := value.CleanupV1(context.Background())
	if !contract.HasCode(err, contract.ErrorUnknownOutcome) || summary.State != contract.CleanupIndeterminateV1 {
		t.Fatalf("cleanup panic summary=%+v err=%v", summary, err)
	}
}

func join(values []string) string {
	result := ""
	for i, value := range values {
		if i > 0 {
			result += ","
		}
		result += value
	}
	return result
}

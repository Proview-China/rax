package lifecycle_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/composition"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/lifecycle"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/registry"
)

type journalStore struct {
	mu                    sync.Mutex
	values                map[string]contract.HostJournalV1
	loseConstructionCAS   bool
	panicCreate           bool
	panicCAS              bool
	panicInspect          bool
	failInspectOnce       bool
	failBindingUnknownCAS bool
}

func (s *journalStore) CreateHostJournalV1(_ context.Context, value contract.HostJournalV1) (contract.HostJournalV1, error) {
	if s.panicCreate {
		panic("create")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := value.HostID + "/" + value.StartID
	if _, ok := s.values[key]; ok {
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorConflict, "exists", "exists")
	}
	s.values[key] = value
	return value, nil
}
func (s *journalStore) CompareAndSwapHostJournalV1(_ context.Context, expected contract.ExactRefV1, next contract.HostJournalV1) (contract.HostJournalV1, error) {
	if s.panicCAS {
		panic("cas")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := next.HostID + "/" + next.StartID
	current, ok := s.values[key]
	if !ok {
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorNotFound, "missing", "missing")
	}
	ref, _ := current.RefV1()
	if ref != expected {
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorConflict, "cas", "cas")
	}
	if s.failBindingUnknownCAS && next.BindingAttempt != nil && next.BindingAttempt.State == contract.AttemptUnknownV1 {
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorUnavailable, "binding_unknown_persist", "down")
	}
	s.values[key] = next
	if s.loseConstructionCAS && next.Phase == contract.HostConstructingV1 && len(next.Constructed) > 0 {
		s.loseConstructionCAS = false
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorUnknownOutcome, "lost_reply", "lost")
	}
	return next, nil
}
func (s *journalStore) InspectHostJournalV1(_ context.Context, hostID, startID string) (contract.HostJournalV1, error) {
	if s.panicInspect {
		panic("inspect")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failInspectOnce {
		s.failInspectOnce = false
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorUnavailable, "inspect_once", "down")
	}
	value, ok := s.values[hostID+"/"+startID]
	if !ok {
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorNotFound, "missing", "missing")
	}
	return value, nil
}

type pipeline struct {
	definition          contract.DecodedDefinitionV1
	resolved            contract.ResolvedAssemblyV1
	compiled            contract.CompiledAssemblyV1
	binding             contract.ExactRefV1
	decodeCalls         atomic.Int64
	resolveCalls        atomic.Int64
	compileCalls        atomic.Int64
	bindCalls           atomic.Int64
	inspectBindingCalls atomic.Int64
	bindingStartErr     error
	bindingInspectErr   error
	bindingEntered      chan string
	bindingRelease      <-chan struct{}
	bindingMu           sync.Mutex
	bindingRequests     []ports.BindingRequestV1
	panicDecode         bool
	panicResolve        bool
	panicCompile        bool
	panicBindingStart   bool
	panicBindingInspect bool
}

func (p *pipeline) DecodeDefinitionV1(context.Context, contract.HostConfigV1) (contract.DecodedDefinitionV1, error) {
	if p.panicDecode {
		panic("decode")
	}
	p.decodeCalls.Add(1)
	return p.definition, nil
}
func (p *pipeline) ResolveAgentV1(context.Context, contract.HostConfigV1, contract.DecodedDefinitionV1) (contract.ResolvedAssemblyV1, error) {
	if p.panicResolve {
		panic("resolve")
	}
	p.resolveCalls.Add(1)
	return p.resolved, nil
}
func (p *pipeline) CompileHarnessV1(context.Context, contract.HostConfigV1, contract.ResolvedAssemblyV1) (contract.CompiledAssemblyV1, error) {
	if p.panicCompile {
		panic("compile")
	}
	p.compileCalls.Add(1)
	return p.compiled, nil
}
func (p *pipeline) StartOrInspectBindingV1(_ context.Context, request ports.BindingRequestV1) (contract.ExactRefV1, error) {
	p.bindCalls.Add(1)
	p.bindingMu.Lock()
	p.bindingRequests = append(p.bindingRequests, request)
	p.bindingMu.Unlock()
	if p.panicBindingStart {
		panic("binding start")
	}
	if p.bindingEntered != nil {
		p.bindingEntered <- request.StartID
		<-p.bindingRelease
	}
	if p.bindingStartErr != nil {
		return contract.ExactRefV1{}, p.bindingStartErr
	}
	return p.binding, nil
}
func (p *pipeline) InspectBindingV1(_ context.Context, request ports.BindingRequestV1) (contract.ExactRefV1, error) {
	p.inspectBindingCalls.Add(1)
	p.bindingMu.Lock()
	p.bindingRequests = append(p.bindingRequests, request)
	p.bindingMu.Unlock()
	if p.panicBindingInspect {
		panic("binding inspect")
	}
	if p.bindingInspectErr != nil {
		return contract.ExactRefV1{}, p.bindingInspectErr
	}
	return p.binding, nil
}

type readiness struct {
	mu           sync.Mutex
	now          time.Time
	values       map[string]contract.SystemReadyV1
	omitLast     bool
	drift        bool
	verifyCalls  atomic.Int64
	panicVerify  bool
	panicInspect bool
}

func (r *readiness) VerifySystemReadyV1(_ context.Context, request ports.ReadinessRequestV1) (contract.SystemReadyV1, error) {
	r.verifyCalls.Add(1)
	if r.panicVerify {
		panic("verify")
	}
	value := contract.SystemReadyV1{ContractVersion: contract.ContractVersionV1, HostID: request.HostID, StartID: request.StartID, DefinitionRef: request.Definition.Ref, PlanRef: request.Resolved.PlanRef, GenerationRef: request.Compiled.GenerationRef, HandoffRef: request.Compiled.HandoffRef, BindingRef: request.BindingRef, Components: append([]contract.ConstructedComponentV1(nil), request.Components...), CheckedUnixNano: r.now.Add(-time.Second).UnixNano(), ExpiresUnixNano: r.now.Add(time.Hour).UnixNano()}
	for _, domain := range contract.RequiredReleaseDomainsV1() {
		value.Releases = append(value.Releases, contract.ReleaseCurrentV1{Domain: domain, ReleaseRef: exactRef("praxis.release", domain), Production: true, ExpiresUnixNano: r.now.Add(time.Hour).UnixNano()})
	}
	if r.omitLast {
		value.Releases = value.Releases[:len(value.Releases)-1]
	}
	if r.drift {
		value.PlanRef = exactRef("praxis.plan", "drift")
	}
	value, _ = contract.SealSystemReadyV1(value)
	r.mu.Lock()
	r.values[request.HostID+"/"+request.StartID] = value
	r.mu.Unlock()
	return value, nil
}
func (r *readiness) InspectSystemReadyV1(_ context.Context, ref contract.ExactRefV1) (contract.SystemReadyV1, error) {
	if r.panicInspect {
		panic("inspect ready")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[ref.ID]
	if !ok {
		return contract.SystemReadyV1{}, contract.NewError(contract.ErrorNotFound, "ready_missing", "missing")
	}
	return value, nil
}

type factoryLog struct {
	mu          sync.Mutex
	constructed []string
	reattached  []string
	cleaned     []string
	startErr    error
	inspectErr  error
}
type factory struct{ log *factoryLog }
type handle struct {
	node string
	ref  contract.ExactRefV1
	log  *factoryLog
}

func (f factory) StartOrInspectConstructionV1(_ context.Context, request ports.ConstructRequestV1) (ports.ComponentHandleV1, error) {
	f.log.mu.Lock()
	f.log.constructed = append(f.log.constructed, request.Node.NodeID)
	f.log.mu.Unlock()
	if f.log.startErr != nil {
		return nil, f.log.startErr
	}
	return &handle{node: request.Node.NodeID, ref: exactRef("praxis.component/instance", request.Node.NodeID+"-instance"), log: f.log}, nil
}
func (f factory) InspectConstructionV1(_ context.Context, request ports.ConstructRequestV1) (ports.ComponentHandleV1, error) {
	f.log.mu.Lock()
	f.log.reattached = append(f.log.reattached, request.Node.NodeID)
	f.log.mu.Unlock()
	if f.log.inspectErr != nil {
		return nil, f.log.inspectErr
	}
	if request.Attempt.ComponentRef == nil {
		return nil, contract.NewError(contract.ErrorNotFound, "missing", "missing")
	}
	return &handle{node: request.Node.NodeID, ref: *request.Attempt.ComponentRef, log: f.log}, nil
}
func (h *handle) RefV1() contract.ExactRefV1 { return h.ref }
func (h *handle) CleanupV1(context.Context) (contract.CleanupItemV1, error) {
	h.log.mu.Lock()
	h.log.cleaned = append(h.log.cleaned, h.node)
	h.log.mu.Unlock()
	return contract.CleanupItemV1{NodeID: h.node, ComponentRef: h.ref, State: contract.CleanupClosedV1}, nil
}

type fixture struct {
	now         time.Time
	config      contract.HostConfigV1
	pipeline    *pipeline
	ready       *readiness
	journals    *journalStore
	log         *factoryLog
	composer    *composition.ComposerV1
	host        *lifecycle.HostV1
	coordinator *journal.CoordinatorV1
	clock       func() time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	now := time.Now()
	graph := constructionGraph()
	p := &pipeline{definition: contract.DecodedDefinitionV1{Ref: exactRef("praxis.definition", "definition")}, resolved: contract.ResolvedAssemblyV1{PlanRef: exactRef("praxis.plan", "plan"), InputRef: exactRef("praxis.assembly/input", "input")}, compiled: contract.CompiledAssemblyV1{GenerationRef: exactRef("praxis.assembly/generation", "generation"), ManifestRef: exactRef("praxis.assembly/manifest", "manifest"), Graph: graph, HandoffRef: exactRef("praxis.assembly/handoff", "handoff")}, binding: exactRef("praxis.runtime/binding", "binding")}
	facts := &journalStore{values: map[string]contract.HostJournalV1{}}
	coordinator, err := journal.NewCoordinatorV1(facts, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	log := &factoryLog{}
	reg := registry.NewV1()
	for _, node := range graph.Nodes {
		if err := reg.RegisterV1(node.Factory, factory{log: log}); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.SealV1(); err != nil {
		t.Fatal(err)
	}
	composer, err := composition.NewComposerV1(reg)
	if err != nil {
		t.Fatal(err)
	}
	ready := &readiness{now: now, values: map[string]contract.SystemReadyV1{}}
	var clockNanos atomic.Int64
	clockNanos.Store(now.UnixNano())
	clock := func() time.Time { return time.Unix(0, clockNanos.Add(int64(time.Millisecond))) }
	host, err := lifecycle.NewHostV1(lifecycle.ConfigV1{Decoder: p, Assembler: p, Compiler: p, Bindings: p, Readiness: ready, Journal: coordinator, Composer: composer, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{now: now, config: hostConfig(), pipeline: p, ready: ready, journals: facts, log: log, composer: composer, host: host, coordinator: coordinator, clock: clock}
}

func TestValidateAndAssembleHaveNoBindingFactoryOrReadinessEffects(t *testing.T) {
	f := newFixture(t)
	if _, err := f.host.Validate(context.Background(), contract.ValidateRequestV1{Config: f.config}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.host.Assemble(context.Background(), contract.AssembleRequestV1{Config: f.config}); err != nil {
		t.Fatal(err)
	}
	if f.pipeline.bindCalls.Load() != 0 || f.ready.verifyCalls.Load() != 0 || len(f.log.constructed) != 0 {
		t.Fatal("validate/assemble crossed an effectful host boundary")
	}
}

func TestStartReadyInspectAndReverseDAGStop(t *testing.T) {
	f := newFixture(t)
	started, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "start-1", Config: f.config})
	if err != nil {
		t.Fatal(err)
	}
	if started.Ready.StartID != "start-1" {
		t.Fatal("wrong ready projection")
	}
	inspected, err := f.host.Inspect(context.Background(), contract.InspectRequestV1{HostID: f.config.HostID, StartID: "start-1"})
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Journal.Phase != contract.HostReadyV1 || inspected.Ready == nil {
		t.Fatal("host did not inspect ready")
	}
	stopped, err := f.host.Stop(context.Background(), contract.StopRequestV1{HostID: f.config.HostID, StartID: "start-1"})
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Cleanup.State != contract.CleanupClosedV1 {
		t.Fatalf("cleanup=%s", stopped.Cleanup.State)
	}
	if got := join(f.log.cleaned); got != "harness,sandbox" {
		t.Fatalf("cleanup order %s", got)
	}
	final, _ := f.journals.InspectHostJournalV1(context.Background(), f.config.HostID, "start-1")
	if final.Phase != contract.HostClosedV1 {
		t.Fatalf("phase=%s", final.Phase)
	}
}

func TestConcurrentSameStartConstructsAndBindsOnce(t *testing.T) {
	f := newFixture(t)
	var wg sync.WaitGroup
	errors := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "start-1", Config: f.config})
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.pipeline.bindCalls.Load() != 1 {
		t.Fatalf("bind calls=%d", f.pipeline.bindCalls.Load())
	}
	if got := join(f.log.constructed); got != "sandbox,harness" {
		t.Fatalf("constructed=%s", got)
	}
}

func TestDifferentStartIDsDoNotShareLifecycleLock(t *testing.T) {
	f := newFixture(t)
	entered := make(chan string, 2)
	release := make(chan struct{})
	f.pipeline.bindingEntered = entered
	f.pipeline.bindingRelease = release
	errs := make(chan error, 2)
	for _, id := range []string{"start-a", "start-b"} {
		go func(startID string) {
			_, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: startID, Config: f.config})
			errs <- err
		}(id)
	}
	seen := map[string]bool{}
	for range 2 {
		select {
		case id := <-entered:
			seen[id] = true
		case <-time.After(2 * time.Second):
			t.Fatal("different starts were serialized by one global lock")
		}
	}
	close(release)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if !seen["start-a"] || !seen["start-b"] {
		t.Fatalf("entered=%v", seen)
	}
}

func TestBindingUnknownIsWriteAheadAndRetryInspectsOriginalAttemptOnly(t *testing.T) {
	f := newFixture(t)
	f.pipeline.bindingStartErr = contract.NewError(contract.ErrorUnknownOutcome, "lost_reply", "lost")
	f.pipeline.bindingInspectErr = contract.NewError(contract.ErrorUnavailable, "inspect_unavailable", "down")
	request := contract.StartRequestV1{StartID: "binding-unknown", Config: f.config}
	if _, err := f.host.Start(context.Background(), request); err == nil {
		t.Fatal("unknown binding accepted")
	}
	first, _ := f.journals.InspectHostJournalV1(context.Background(), f.config.HostID, request.StartID)
	if first.Phase != contract.HostBindingV1 || first.BindingAttempt == nil || first.BindingAttempt.State != contract.AttemptUnknownV1 {
		t.Fatalf("journal=%+v", first)
	}
	if first.BindingAttempt.AttemptID == "" || first.BindingAttempt.RequestDigest == "" {
		t.Fatal("binding attempt identity missing")
	}
	if _, err := f.host.Start(context.Background(), request); err == nil {
		t.Fatal("unknown binding retry accepted")
	}
	if f.pipeline.bindCalls.Load() != 1 || f.pipeline.inspectBindingCalls.Load() != 2 {
		t.Fatalf("start=%d inspect=%d", f.pipeline.bindCalls.Load(), f.pipeline.inspectBindingCalls.Load())
	}
	if _, err := f.host.Stop(context.Background(), contract.StopRequestV1{HostID: f.config.HostID, StartID: request.StartID}); err != nil {
		t.Fatal(err)
	}
	final, _ := f.journals.InspectHostJournalV1(context.Background(), f.config.HostID, request.StartID)
	if final.Phase != contract.HostIndeterminateV1 {
		t.Fatalf("unknown binding closed as %s", final.Phase)
	}
}

func TestBindingUnknownPersistenceFailureIsReturnedAndNotClaimedDurable(t *testing.T) {
	f := newFixture(t)
	f.pipeline.bindingStartErr = contract.NewError(contract.ErrorUnknownOutcome, "lost_reply", "lost")
	f.pipeline.bindingInspectErr = contract.NewError(contract.ErrorUnavailable, "inspect_unavailable", "down")
	f.journals.failBindingUnknownCAS = true
	_, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "binding-unknown-persist-fail", Config: f.config})
	if err == nil || !strings.Contains(err.Error(), "unknown_progress_not_persisted") {
		t.Fatalf("error=%v", err)
	}
	current, _ := f.journals.InspectHostJournalV1(context.Background(), f.config.HostID, "binding-unknown-persist-fail")
	if current.BindingAttempt == nil || current.BindingAttempt.State != contract.AttemptPlannedV1 {
		t.Fatalf("unknown was falsely claimed durable: %+v", current)
	}
}

func TestConstructionProgressLostCASReplyRecoversWithoutSecondFactoryStart(t *testing.T) {
	f := newFixture(t)
	f.journals.loseConstructionCAS = true
	if _, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "lost-progress", Config: f.config}); err != nil {
		t.Fatal(err)
	}
	if got := join(f.log.constructed); got != "sandbox,harness" {
		t.Fatalf("factory starts=%s", got)
	}
	current, _ := f.journals.InspectHostJournalV1(context.Background(), f.config.HostID, "lost-progress")
	if len(current.ConstructionAttempts) != 2 || current.Phase != contract.HostReadyV1 {
		t.Fatalf("journal=%+v", current)
	}
}

func TestCommittedConstructedProgressRecoveryFailureStillCleansAndEndsIndeterminate(t *testing.T) {
	f := newFixture(t)
	f.journals.loseConstructionCAS = true
	f.journals.failInspectOnce = true
	_, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "committed-progress-fail", Config: f.config})
	if err == nil {
		t.Fatal("lost constructed progress inspection unexpectedly succeeded")
	}
	if got := join(f.log.cleaned); got != "sandbox" {
		t.Fatalf("valid returned handle leaked, cleaned=%s", got)
	}
	current, _ := f.journals.InspectHostJournalV1(context.Background(), f.config.HostID, "committed-progress-fail")
	if current.Phase != contract.HostIndeterminateV1 || current.ReadyRef != nil {
		t.Fatalf("journal=%+v", current)
	}
}

func TestExternalPortPanicsAreClosedErrorsAndBindingPanicInspectsOriginalAttempt(t *testing.T) {
	f := newFixture(t)
	f.pipeline.panicDecode = true
	if _, err := f.host.Validate(context.Background(), contract.ValidateRequestV1{Config: f.config}); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("decoder panic=%v", err)
	}
	f = newFixture(t)
	f.pipeline.panicResolve = true
	if _, err := f.host.Assemble(context.Background(), contract.AssembleRequestV1{Config: f.config}); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("assembler panic=%v", err)
	}
	f = newFixture(t)
	f.pipeline.panicCompile = true
	if _, err := f.host.Assemble(context.Background(), contract.AssembleRequestV1{Config: f.config}); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("compiler panic=%v", err)
	}
	f = newFixture(t)
	f.pipeline.panicBindingStart = true
	if _, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "binding-panic-recovered", Config: f.config}); err != nil {
		t.Fatal(err)
	}
	if f.pipeline.bindCalls.Load() != 1 || f.pipeline.inspectBindingCalls.Load() != 1 {
		t.Fatalf("start=%d inspect=%d", f.pipeline.bindCalls.Load(), f.pipeline.inspectBindingCalls.Load())
	}
	if len(f.pipeline.bindingRequests) != 2 || f.pipeline.bindingRequests[0].Attempt.AttemptID != f.pipeline.bindingRequests[1].Attempt.AttemptID || f.pipeline.bindingRequests[0].Attempt.RequestDigest != f.pipeline.bindingRequests[1].Attempt.RequestDigest {
		t.Fatal("binding panic recovery did not inspect the original attempt")
	}
	f = newFixture(t)
	f.pipeline.panicBindingStart = true
	f.pipeline.panicBindingInspect = true
	if _, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "binding-panic-unknown", Config: f.config}); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("binding panic=%v", err)
	}
	current, _ := f.journals.InspectHostJournalV1(context.Background(), f.config.HostID, "binding-panic-unknown")
	if current.BindingAttempt == nil || current.BindingAttempt.State != contract.AttemptUnknownV1 {
		t.Fatalf("journal=%+v", current)
	}
	f = newFixture(t)
	f.ready.panicVerify = true
	if _, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "ready-verify-panic", Config: f.config}); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("verify panic=%v", err)
	}
	f = newFixture(t)
	if _, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "ready-inspect-panic", Config: f.config}); err != nil {
		t.Fatal(err)
	}
	f.ready.panicInspect = true
	if _, err := f.host.Inspect(context.Background(), contract.InspectRequestV1{HostID: f.config.HostID, StartID: "ready-inspect-panic"}); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("inspect panic=%v", err)
	}
	f = newFixture(t)
	f.journals.panicCreate = true
	if _, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "journal-create-panic", Config: f.config}); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("journal create panic=%v", err)
	}
	f = newFixture(t)
	f.journals.panicCAS = true
	if _, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "journal-cas-panic", Config: f.config}); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("journal cas panic=%v", err)
	}
	f = newFixture(t)
	f.journals.panicInspect = true
	if _, err := f.host.Inspect(context.Background(), contract.InspectRequestV1{HostID: f.config.HostID, StartID: "journal-inspect-panic"}); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("journal inspect panic=%v", err)
	}
	f = newFixture(t)
	panicClock, err := lifecycle.NewHostV1(lifecycle.ConfigV1{Decoder: f.pipeline, Assembler: f.pipeline, Compiler: f.pipeline, Bindings: f.pipeline, Readiness: f.ready, Journal: f.coordinator, Composer: f.composer, Clock: func() time.Time { panic("clock") }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := panicClock.Start(context.Background(), contract.StartRequestV1{StartID: "clock-panic", Config: f.config}); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("clock panic=%v", err)
	}
}

func TestReadyInspectClockCannotTrailJournalWatermark(t *testing.T) {
	f := newFixture(t)
	if _, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "clock-watermark", Config: f.config}); err != nil {
		t.Fatal(err)
	}
	restarted, err := lifecycle.NewHostV1(lifecycle.ConfigV1{Decoder: f.pipeline, Assembler: f.pipeline, Compiler: f.pipeline, Bindings: f.pipeline, Readiness: f.ready, Journal: f.coordinator, Composer: f.composer, Clock: func() time.Time { return f.now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Inspect(context.Background(), contract.InspectRequestV1{HostID: f.config.HostID, StartID: "clock-watermark"}); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("clock rollback accepted: %v", err)
	}
}

func TestConstructionLostOutcomePersistsUnknownAndStopCannotClose(t *testing.T) {
	f := newFixture(t)
	f.log.startErr = contract.NewError(contract.ErrorUnknownOutcome, "lost_reply", "lost")
	f.log.inspectErr = contract.NewError(contract.ErrorUnavailable, "inspect_unavailable", "down")
	request := contract.StartRequestV1{StartID: "construction-unknown", Config: f.config}
	if _, err := f.host.Start(context.Background(), request); err == nil {
		t.Fatal("unknown construction accepted")
	}
	current, _ := f.journals.InspectHostJournalV1(context.Background(), f.config.HostID, request.StartID)
	if current.Phase != contract.HostIndeterminateV1 || len(current.ConstructionAttempts) != 1 || current.ConstructionAttempts[0].State != contract.AttemptUnknownV1 {
		t.Fatalf("journal=%+v", current)
	}
	if _, err := f.host.Stop(context.Background(), contract.StopRequestV1{HostID: f.config.HostID, StartID: request.StartID}); err != nil {
		t.Fatal(err)
	}
	final, _ := f.journals.InspectHostJournalV1(context.Background(), f.config.HostID, request.StartID)
	if final.Phase != contract.HostIndeterminateV1 {
		t.Fatalf("unknown construction closed as %s", final.Phase)
	}
}

func TestInspectReturnsReadyOnlyForExactFreshReadyPhase(t *testing.T) {
	f := newFixture(t)
	if _, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "start-1", Config: f.config}); err != nil {
		t.Fatal(err)
	}
	raw := f.ready.values[f.config.HostID+"/start-1"]
	raw.CheckedUnixNano++
	raw, _ = contract.SealSystemReadyV1(raw)
	f.ready.values[f.config.HostID+"/start-1"] = raw
	if _, err := f.host.Inspect(context.Background(), contract.InspectRequestV1{HostID: f.config.HostID, StartID: "start-1"}); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("drifted ready accepted: %v", err)
	}
	if _, err := f.host.Stop(context.Background(), contract.StopRequestV1{HostID: f.config.HostID, StartID: "start-1"}); err != nil {
		t.Fatal(err)
	}
	inspected, err := f.host.Inspect(context.Background(), contract.InspectRequestV1{HostID: f.config.HostID, StartID: "start-1"})
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Journal.Phase != contract.HostClosedV1 || inspected.Ready != nil {
		t.Fatalf("closed phase projected ready: %+v", inspected)
	}
}

func TestInspectNeverProjectsReadyOutsideReadyPhase(t *testing.T) {
	for _, phase := range []contract.HostPhaseV1{contract.HostDrainingV1, contract.HostReconcilingV1, contract.HostIndeterminateV1, contract.HostClosedV1} {
		t.Run(string(phase), func(t *testing.T) {
			f := newFixture(t)
			if _, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "phase-inspect", Config: f.config}); err != nil {
				t.Fatal(err)
			}
			current, _ := f.journals.InspectHostJournalV1(context.Background(), f.config.HostID, "phase-inspect")
			current.Phase = phase
			current.Revision++
			current.UpdatedUnixNano++
			current, _ = contract.SealHostJournalV1(current)
			f.journals.mu.Lock()
			f.journals.values[f.config.HostID+"/phase-inspect"] = current
			f.journals.mu.Unlock()
			result, err := f.host.Inspect(context.Background(), contract.InspectRequestV1{HostID: f.config.HostID, StartID: "phase-inspect"})
			if err != nil {
				t.Fatal(err)
			}
			if result.Ready != nil {
				t.Fatalf("phase %s projected ready", phase)
			}
		})
	}
}

func TestReadinessMissingReleaseFailsClosedThenStopCleans(t *testing.T) {
	f := newFixture(t)
	f.ready.omitLast = true
	if _, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "start-1", Config: f.config}); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("error=%v", err)
	}
	current, _ := f.journals.InspectHostJournalV1(context.Background(), f.config.HostID, "start-1")
	if current.Phase != contract.HostVerifyingV1 {
		t.Fatalf("phase=%s", current.Phase)
	}
	if _, err := f.host.Stop(context.Background(), contract.StopRequestV1{HostID: f.config.HostID, StartID: "start-1"}); err != nil {
		t.Fatal(err)
	}
}

func TestReadinessExactChainDriftRejected(t *testing.T) {
	f := newFixture(t)
	f.ready.drift = true
	if _, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "start-1", Config: f.config}); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("error=%v", err)
	}
}

func TestRestartStopReattachesExactComponents(t *testing.T) {
	f := newFixture(t)
	if _, err := f.host.Start(context.Background(), contract.StartRequestV1{StartID: "start-1", Config: f.config}); err != nil {
		t.Fatal(err)
	}
	coordinator, _ := journal.NewCoordinatorV1(f.journals, func() time.Time { return f.now })
	restarted, err := lifecycle.NewHostV1(lifecycle.ConfigV1{Decoder: f.pipeline, Assembler: f.pipeline, Compiler: f.pipeline, Bindings: f.pipeline, Readiness: f.ready, Journal: coordinator, Composer: f.composer, Clock: f.clock})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Stop(context.Background(), contract.StopRequestV1{HostID: f.config.HostID, StartID: "start-1"}); err != nil {
		t.Fatal(err)
	}
	if got := join(f.log.reattached); got != "sandbox,harness" {
		t.Fatalf("reattached=%s", got)
	}
}

func TestNewHostRejectsTypedNilDependency(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*lifecycle.ConfigV1)
	}{
		{"decoder", func(c *lifecycle.ConfigV1) { var v *pipeline; c.Decoder = v }},
		{"assembler", func(c *lifecycle.ConfigV1) { var v *pipeline; c.Assembler = v }},
		{"compiler", func(c *lifecycle.ConfigV1) { var v *pipeline; c.Compiler = v }},
		{"binding", func(c *lifecycle.ConfigV1) { var v *pipeline; c.Bindings = v }},
		{"readiness", func(c *lifecycle.ConfigV1) { var v *readiness; c.Readiness = v }},
		{"journal", func(c *lifecycle.ConfigV1) { var v *journal.CoordinatorV1; c.Journal = v }},
		{"composer", func(c *lifecycle.ConfigV1) { var v *composition.ComposerV1; c.Composer = v }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			config := lifecycle.ConfigV1{Decoder: f.pipeline, Assembler: f.pipeline, Compiler: f.pipeline, Bindings: f.pipeline, Readiness: f.ready, Journal: f.coordinator, Composer: f.composer, Clock: f.clock}
			test.mutate(&config)
			if _, err := lifecycle.NewHostV1(config); !contract.HasCode(err, contract.ErrorInvalidArgument) {
				t.Fatalf("typed nil %s accepted: %v", test.name, err)
			}
		})
	}
}

func TestStartFailsClosedOnClockRollbackOrEqualTick(t *testing.T) {
	for _, test := range []struct {
		name   string
		offset time.Duration
	}{{"rollback", -time.Second}, {"equal", 0}} {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			host, err := lifecycle.NewHostV1(lifecycle.ConfigV1{Decoder: f.pipeline, Assembler: f.pipeline, Compiler: f.pipeline, Bindings: f.pipeline, Readiness: f.ready, Journal: f.coordinator, Composer: f.composer, Clock: func() time.Time { return f.now.Add(test.offset) }})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := host.Start(context.Background(), contract.StartRequestV1{StartID: "clock-start", Config: f.config}); !contract.HasCode(err, contract.ErrorPrecondition) {
				t.Fatalf("error=%v", err)
			} else {
				var typed *contract.Error
				if !errors.As(err, &typed) || typed.Reason != "clock_regression" {
					t.Fatalf("error=%v", err)
				}
			}
			current, inspectErr := f.journals.InspectHostJournalV1(context.Background(), f.config.HostID, "clock-start")
			if inspectErr != nil {
				t.Fatal(inspectErr)
			}
			if current.Phase != contract.HostAcceptedV1 || current.Revision != 1 {
				t.Fatalf("rollback mutated journal: %+v", current)
			}
		})
	}
}

func hostConfig() contract.HostConfigV1 {
	return contract.HostConfigV1{ContractVersion: contract.ContractVersionV1, HostID: "host-1", DefinitionSourceRef: "asset:/definition/1", StatePlaneBindings: []string{"state:/primary"}, ProviderEndpointRefs: []string{"provider:/model"}, SecretBrokerRef: "secret:/broker", CatalogRef: "catalog:/1", ResolutionFactsRef: "facts:/1", RuntimeServiceRefs: []string{"runtime:/control"}, ListenRef: "listen:/local", DiagnosticsPolicyRef: "policy:/diagnostics"}
}
func exactRef(kind, id string) contract.ExactRefV1 {
	digest, _ := contract.DigestJSONV1(id)
	return contract.ExactRefV1{Kind: kind, ID: id, Revision: 1, Digest: digest}
}
func factoryKey(id string) contract.FactoryKeyV1 {
	digest, _ := contract.DigestJSONV1("artifact-" + id)
	return contract.FactoryKeyV1{ComponentID: id, ArtifactDigest: digest, Contract: "praxis.fixture/v1", Capability: "praxis.fixture/start"}
}
func constructionGraph() contract.ConstructionGraphV1 {
	return contract.ConstructionGraphV1{GraphRef: exactRef("praxis.harness/graph", "graph"), Nodes: []contract.ComponentNodeV1{{NodeID: "harness", Factory: factoryKey("harness"), Dependencies: []string{"sandbox"}}, {NodeID: "sandbox", Factory: factoryKey("sandbox")}}}
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

package journal_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type currentReaders struct {
	drift                  bool
	policyWindowNanos      int64
	policyWindowAfterFirst int64
	policyCalls            atomic.Int64
}

func (r *currentReaders) get(v runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	if r.drift {
		v.Revision++
	}
	return v, nil
}
func (r *currentReaders) InspectDefinitionCurrentV2(c context.Context, v runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return r.get(v)
}
func (r *currentReaders) InspectPlanCurrentV2(c context.Context, v runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return r.get(v)
}
func (r *currentReaders) InspectAssemblyCurrentV2(c context.Context, v runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return r.get(v)
}
func (r *currentReaders) InspectBindingSetCurrentV2(c context.Context, v runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return r.get(v)
}
func (r *currentReaders) InspectActivationCurrentV2(c context.Context, v runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return r.get(v)
}
func (r *currentReaders) InspectGenerationBindingCurrentV2(c context.Context, v runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return r.get(v)
}
func (r *currentReaders) InspectApplicationStartCurrentV2(c context.Context, v runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return r.get(v)
}
func (r *currentReaders) InspectSandboxLeaseCurrentV2(c context.Context, v runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return r.get(v)
}
func (r *currentReaders) InspectSandboxActiveCurrentV2(c context.Context, v runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return r.get(v)
}
func (r *currentReaders) InspectExecutionReadyCurrentV2(c context.Context, v runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return r.get(v)
}
func (r *currentReaders) InspectSupervisionPolicyCurrentV2(_ context.Context, v runtimeports.OwnerCurrentRefV1) (contract.SystemReadySupervisionPolicyCurrentV2, error) {
	actual, err := r.get(v)
	if err != nil {
		return contract.SystemReadySupervisionPolicyCurrentV2{}, err
	}
	window := r.policyWindowNanos
	if window == 0 {
		window = int64(time.Minute)
	}
	if r.policyCalls.Add(1) > 1 && r.policyWindowAfterFirst != 0 {
		window = r.policyWindowAfterFirst
	}
	return contract.SealSystemReadySupervisionPolicyCurrentV2(contract.SystemReadySupervisionPolicyCurrentV2{
		Ref:                     actual,
		MinimumReadyWindowNanos: window,
		CheckedUnixNano:         actual.ExpiresUnixNano - int64(time.Hour),
		ExpiresUnixNano:         actual.ExpiresUnixNano,
	})
}

type componentReader struct{}

func (componentReader) InspectComponentProductionCurrentV2(_ context.Context, v contract.ComponentProductionCurrentV2) (contract.ComponentProductionCurrentV2, error) {
	return v, nil
}

type componentRegistry struct{}

func (componentRegistry) ReaderForComponentProductionCurrentV2(runtimeports.NamespacedNameV2) (ports.ComponentProductionCurrentReaderV2, error) {
	return componentReader{}, nil
}

type lostReadyStore struct {
	inner                 *journal.MemorySystemReadyStoreV2
	loseFact, loseCurrent atomic.Bool
	facts, currents       atomic.Int64
}

type lostAttemptStore struct {
	inner                  *journal.MemorySystemReadyAttemptStoreV2
	loseCreate, loseCAS    atomic.Bool
	creates, createCommits atomic.Int64
	cas, casCommits        atomic.Int64
}

func (s *lostAttemptStore) CreateSystemReadyAttemptV2(c context.Context, v contract.SystemReadyAttemptFactV2) (contract.SystemReadyAttemptFactV2, error) {
	s.creates.Add(1)
	got, err := s.inner.CreateSystemReadyAttemptV2(c, v)
	if err == nil {
		s.createCommits.Add(1)
		if s.loseCreate.CompareAndSwap(true, false) {
			return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorUnknownOutcome, "lost", "lost attempt create reply")
		}
	}
	return got, err
}

func (s *lostAttemptStore) CompareAndSwapSystemReadyAttemptV2(c context.Context, expected contract.ExactRefV1, next contract.SystemReadyAttemptFactV2) (contract.SystemReadyAttemptFactV2, error) {
	s.cas.Add(1)
	got, err := s.inner.CompareAndSwapSystemReadyAttemptV2(c, expected, next)
	if err == nil {
		s.casCommits.Add(1)
		if s.loseCAS.CompareAndSwap(true, false) {
			return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorUnknownOutcome, "lost", "lost attempt CAS reply")
		}
	}
	return got, err
}

func (s *lostAttemptStore) InspectSystemReadyAttemptV2(c context.Context, key contract.SystemReadyAttemptStepKeyV2) (contract.SystemReadyAttemptFactV2, error) {
	return s.inner.InspectSystemReadyAttemptV2(c, key)
}

func (s *lostReadyStore) CreateSystemReadyFactV2(c context.Context, v contract.SystemReadyFactV2) (contract.SystemReadyFactV2, error) {
	s.facts.Add(1)
	got, e := s.inner.CreateSystemReadyFactV2(c, v)
	if e == nil && s.loseFact.CompareAndSwap(true, false) {
		return contract.SystemReadyFactV2{}, contract.NewError(contract.ErrorUnknownOutcome, "lost", "lost fact reply")
	}
	return got, e
}
func (s *lostReadyStore) InspectSystemReadyFactV2(c context.Context, v contract.SystemReadyFactRefV2) (contract.SystemReadyFactV2, error) {
	return s.inner.InspectSystemReadyFactV2(c, v)
}
func (s *lostReadyStore) CreateSystemReadyCurrentV2(c context.Context, v contract.SystemReadyCurrentV2) (contract.SystemReadyCurrentV2, error) {
	s.currents.Add(1)
	got, e := s.inner.CreateSystemReadyCurrentV2(c, v)
	if e == nil && s.loseCurrent.CompareAndSwap(true, false) {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorUnknownOutcome, "lost", "lost current reply")
	}
	return got, e
}
func (s *lostReadyStore) CompareAndSwapSystemReadyCurrentV2(c context.Context, e contract.SystemReadyCurrentRefV2, v contract.SystemReadyCurrentV2) (contract.SystemReadyCurrentV2, error) {
	s.currents.Add(1)
	got, err := s.inner.CompareAndSwapSystemReadyCurrentV2(c, e, v)
	if err == nil && s.loseCurrent.CompareAndSwap(true, false) {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorUnknownOutcome, "lost", "lost current CAS reply")
	}
	return got, err
}
func (s *lostReadyStore) InspectSystemReadyCurrentV2(c context.Context, v contract.SystemReadyCurrentRefV2) (contract.SystemReadyCurrentV2, error) {
	return s.inner.InspectSystemReadyCurrentV2(c, v)
}

func TestSystemReadyGatewayV2LostRepliesAnd64Workers(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	g, store, request := gatewayFixture(t, func() time.Time { return now })
	store.loseFact.Store(true)
	store.loseCurrent.Store(true)
	var wg sync.WaitGroup
	var ok atomic.Int64
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := g.StartOrInspectSystemReadyV2(context.Background(), request); e == nil {
				ok.Add(1)
			} else if !contract.HasCode(e, contract.ErrorUnknownOutcome) && !contract.HasCode(e, contract.ErrorConflict) {
				t.Errorf("start: %v", e)
			}
		}()
	}
	wg.Wait()
	if ok.Load() == 0 || store.facts.Load() != 1 || store.currents.Load() != 1 {
		t.Fatalf("ok=%d facts=%d currents=%d", ok.Load(), store.facts.Load(), store.currents.Load())
	}
	if _, e := g.StartOrInspectSystemReadyV2(context.Background(), request); e != nil {
		t.Fatalf("settled retry: %v", e)
	}
	got, e := g.InspectSystemReadyV2(context.Background(), contract.SystemReadyInspectRequestV2{HostID: request.HostID, StartID: request.StartID, AttemptID: request.AttemptID, RequestDigest: request.RequestDigest})
	if e != nil || got.Current.Epoch != request.AvailabilityEpoch {
		t.Fatalf("inspect=%+v err=%v", got, e)
	}
}

func TestSystemReadyGatewayV2Across64GatewaysLinearizesOneEffectWriter(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	g, store, request := gatewayFixture(t, func() time.Time { return now })
	attempts := gatewayAttempts(g)
	claims := gatewayClaims(g)
	gateways := make([]*journal.SystemReadyGatewayV2, 64)
	for i := range gateways {
		var err error
		observed := now.Add(time.Duration(i) * time.Nanosecond)
		gateways[i], err = journal.NewSystemReadyGatewayV2(store, attempts, claims, &currentReaders{}, componentRegistry{}, func() time.Time { return observed })
		if err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	var successes atomic.Int64
	var errorMu sync.Mutex
	errorsSeen := map[string]int{}
	for _, gateway := range gateways {
		wg.Add(1)
		go func(gateway *journal.SystemReadyGatewayV2) {
			defer wg.Done()
			if _, err := gateway.StartOrInspectSystemReadyV2(context.Background(), request); err == nil {
				successes.Add(1)
			} else {
				errorMu.Lock()
				errorsSeen[err.Error()]++
				errorMu.Unlock()
			}
		}(gateway)
	}
	wg.Wait()
	if successes.Load() == 0 {
		t.Fatalf("no gateway observed the settled result: %v", errorsSeen)
	}
	if store.facts.Load() != 1 || store.currents.Load() != 1 {
		t.Fatalf("effect writes facts=%d currents=%d", store.facts.Load(), store.currents.Load())
	}
	newGateway, err := journal.NewSystemReadyGatewayV2(store, attempts, claims, &currentReaders{}, componentRegistry{}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = newGateway.StartOrInspectSystemReadyV2(context.Background(), request); err != nil {
		t.Fatalf("restart recovery: %v", err)
	}
	if a := attempts.(*lostAttemptStore); a.createCommits.Load() != 1 {
		t.Fatalf("attempt create commits=%d", a.createCommits.Load())
	}
}

func TestSystemReadyGatewayV2LostAttemptRepliesRecoverOrRequireInspect(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	t.Run("lost intent create never dispatches", func(t *testing.T) {
		g, store, request := gatewayFixture(t, func() time.Time { return now })
		attempts := gatewayAttempts(g).(*lostAttemptStore)
		attempts.loseCreate.Store(true)
		if _, err := g.StartOrInspectSystemReadyV2(context.Background(), request); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
			t.Fatalf("err=%v", err)
		}
		if store.facts.Load() != 0 || store.currents.Load() != 0 {
			t.Fatalf("effects facts=%d currents=%d", store.facts.Load(), store.currents.Load())
		}
		restarted, err := journal.NewSystemReadyGatewayV2(store, attempts, gatewayClaims(g), &currentReaders{}, componentRegistry{}, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		if _, err = restarted.StartOrInspectSystemReadyV2(context.Background(), request); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
			t.Fatalf("restart err=%v", err)
		}
		if store.facts.Load() != 0 || store.currents.Load() != 0 {
			t.Fatalf("restart dispatched effects facts=%d currents=%d", store.facts.Load(), store.currents.Load())
		}
		attempt, err := attempts.InspectSystemReadyAttemptV2(context.Background(), contract.NewSystemReadyAttemptStepKeyV2(request.HostID, request.StartID, request.AttemptID))
		if err != nil || attempt.State != contract.SystemReadyAttemptReconciliationRequiredV2 {
			t.Fatalf("attempt=%+v err=%v", attempt, err)
		}
	})
	t.Run("lost result CAS is exact inspected", func(t *testing.T) {
		g, store, request := gatewayFixture(t, func() time.Time { return now })
		attempts := gatewayAttempts(g).(*lostAttemptStore)
		attempts.loseCAS.Store(true)
		if _, err := g.StartOrInspectSystemReadyV2(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if attempts.casCommits.Load() != 1 || store.facts.Load() != 1 || store.currents.Load() != 1 {
			t.Fatalf("attemptCAS=%d facts=%d currents=%d", attempts.casCommits.Load(), store.facts.Load(), store.currents.Load())
		}
	})
}

func TestSystemReadyGatewayV2RenewsSameEpoch(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	g, _, r := gatewayFixture(t, func() time.Time { return now })
	first, e := g.StartOrInspectSystemReadyV2(context.Background(), r)
	if e != nil {
		t.Fatal(e)
	}
	r.AttemptID = "ready-attempt-2"
	r.ExpectedCurrent = &first.Current
	r.RequestDigest = ""
	r, e = contract.SealSystemReadyEnsureRequestV2(r)
	if e != nil {
		t.Fatal(e)
	}
	// The renewal CAS commits but its reply is lost. The gateway must recover by
	// exact Inspect and must not allocate another epoch.
	store := gatewayStore(g).(*lostReadyStore)
	store.loseCurrent.Store(true)
	second, e := g.StartOrInspectSystemReadyV2(context.Background(), r)
	if e != nil {
		t.Fatal(e)
	}
	if second.Current.Revision != first.Current.Revision+1 || second.Current.Epoch != first.Current.Epoch {
		t.Fatalf("first=%+v second=%+v", first.Current, second.Current)
	}
	if store.currents.Load() != 2 {
		t.Fatalf("current writes=%d", store.currents.Load())
	}
}

func TestSystemReadyGatewayV2FailsClosedOnDriftExpiryAndClockRollback(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	t.Run("drift", func(t *testing.T) {
		g, _, r := gatewayFixture(t, func() time.Time { return now })
		g2, e := journal.NewSystemReadyGatewayV2(gatewayStore(g), gatewayAttempts(g), gatewayClaims(g), &currentReaders{drift: true}, componentRegistry{}, func() time.Time { return now })
		if e != nil {
			t.Fatal(e)
		}
		if _, e = g2.StartOrInspectSystemReadyV2(context.Background(), r); !contract.HasCode(e, contract.ErrorConflict) {
			t.Fatalf("err=%v", e)
		}
	})
	t.Run("expired", func(t *testing.T) {
		g, _, r := gatewayFixture(t, func() time.Time { return now })
		r.Definition.ExpiresUnixNano = now.UnixNano()
		r.RequestDigest = ""
		r, _ = contract.SealSystemReadyEnsureRequestV2(r)
		if _, e := g.StartOrInspectSystemReadyV2(context.Background(), r); !contract.HasCode(e, contract.ErrorPrecondition) {
			t.Fatalf("err=%v", e)
		}
	})
	t.Run("rollback", func(t *testing.T) {
		times := []time.Time{now, now.Add(time.Second), now.Add(-time.Second)}
		var i int
		g, _, r := gatewayFixture(t, func() time.Time {
			v := times[i]
			if i < len(times)-1 {
				i++
			}
			return v
		})
		if _, e := g.StartOrInspectSystemReadyV2(context.Background(), r); !contract.HasCode(e, contract.ErrorPrecondition) {
			t.Fatalf("err=%v", e)
		}
	})
}

func TestSystemReadyGatewayV2PolicyValueIsOwnerCurrentAndRereadAtS2(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	t.Run("initial value drift", func(t *testing.T) {
		g, store, r := gatewayFixture(t, func() time.Time { return now })
		g2, err := journal.NewSystemReadyGatewayV2(gatewayStore(g), gatewayAttempts(g), gatewayClaims(g), &currentReaders{policyWindowNanos: int64(30 * time.Second)}, componentRegistry{}, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		if _, err = g2.StartOrInspectSystemReadyV2(context.Background(), r); !contract.HasCode(err, contract.ErrorConflict) {
			t.Fatalf("err=%v", err)
		}
		if store.facts.Load() != 0 || store.currents.Load() != 0 {
			t.Fatalf("writes facts=%d currents=%d", store.facts.Load(), store.currents.Load())
		}
	})
	t.Run("window shortens between S1 and S2", func(t *testing.T) {
		g, store, r := gatewayFixture(t, func() time.Time { return now })
		readers := &currentReaders{policyWindowNanos: int64(time.Minute), policyWindowAfterFirst: int64(30 * time.Second)}
		g2, err := journal.NewSystemReadyGatewayV2(gatewayStore(g), gatewayAttempts(g), gatewayClaims(g), readers, componentRegistry{}, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		if _, err = g2.StartOrInspectSystemReadyV2(context.Background(), r); !contract.HasCode(err, contract.ErrorConflict) {
			t.Fatalf("err=%v", err)
		}
		if readers.policyCalls.Load() != 2 || store.facts.Load() != 0 || store.currents.Load() != 0 {
			t.Fatalf("reads=%d writes facts=%d currents=%d", readers.policyCalls.Load(), store.facts.Load(), store.currents.Load())
		}
	})
}

// fixture accessors deliberately rebuild public dependencies; no production handle is exposed.
func gatewayStore(g *journal.SystemReadyGatewayV2) ports.SystemReadyFactPortV2 {
	return fixtureLastStore
}
func gatewayClaims(g *journal.SystemReadyGatewayV2) ports.HostStartClaimCurrentReaderV1 {
	return fixtureLastClaims
}
func gatewayAttempts(g *journal.SystemReadyGatewayV2) ports.SystemReadyAttemptFactPortV2 {
	return fixtureLastAttempts
}

var fixtureLastStore ports.SystemReadyFactPortV2
var fixtureLastClaims ports.HostStartClaimCurrentReaderV1
var fixtureLastAttempts ports.SystemReadyAttemptFactPortV2

func gatewayFixture(t *testing.T, clock func() time.Time) (*journal.SystemReadyGatewayV2, *lostReadyStore, contract.SystemReadyEnsureRequestV2) {
	t.Helper()
	now := time.Unix(1_900_000_000, 0)
	owner := core.OwnerRef{Domain: "praxis.agent-host", ID: "host-owner"}
	base, _ := journal.NewMemorySystemReadyStoreV2(owner)
	store := &lostReadyStore{inner: base}
	attempts := &lostAttemptStore{inner: journal.NewMemorySystemReadyAttemptStoreV2()}
	claims := journal.NewMemoryHostStartClaimStoreV1()
	claim, _ := contract.SealHostStartClaimV1(contract.HostStartClaimV1{ContractVersion: contract.HostStartClaimContractVersionV1, HostContractVersion: contract.ContractVersionV2, HostID: "host-1", StartID: "start-1", ConfigDigest: contract.DigestV1(digest(t, "config")), DefinitionSourceRef: contract.ExactRefV1{Kind: "definition", ID: "definition", Revision: 1, Digest: contract.DigestV1(digest(t, "definition"))}, RequestedOperation: contract.HostStartOperationStartV1, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if _, e := claims.ClaimOrInspectHostStartV1(context.Background(), claim); e != nil {
		t.Fatal(e)
	}
	claimRef, _ := claim.CurrentRefV1()
	ref := func(id string) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: "fixture.owner", ID: core.OwnerID("owner-" + id)}, ContractVersion: "1.0.0", ID: id, Revision: 1, Digest: digest(t, id), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	}
	binding := runtimeports.BindingAdmissionBindingRefV1{ComponentID: "fixture/component", ID: "binding", Revision: 1, Digest: digest(t, "binding"), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	component := contract.ComponentProductionCurrentV2{Domain: "fixture/component", ReleaseCurrent: ref("release"), ConstructedComponent: contract.ExactRefV1{Kind: "component", ID: "component", Revision: 1, Digest: contract.DigestV1(digest(t, "component"))}, Binding: binding, GenerationCurrent: ref("generation"), ActivationCurrent: ref("activation-component"), ProductionCurrent: ref("production")}
	r, _ := contract.SealSystemReadyEnsureRequestV2(contract.SystemReadyEnsureRequestV2{AttemptID: "ready-attempt-1", HostID: "host-1", StartID: "start-1", Claim: claimRef, Definition: ref("definition"), Plan: ref("plan"), Assembly: ref("assembly"), BindingSet: ref("binding-set"), Activation: ref("activation"), GenerationBinding: ref("generation-binding"), ApplicationStart: ref("application"), SandboxLease: ref("lease"), SandboxActive: ref("active"), ExecutionReady: ref("execution"), SupervisionPolicy: ref("policy"), Components: []contract.ComponentProductionCurrentV2{component}, MinimumReadyWindowNanos: int64(time.Minute), AvailabilityEpoch: 1})
	g, e := journal.NewSystemReadyGatewayV2(store, attempts, claims, &currentReaders{}, componentRegistry{}, clock)
	if e != nil {
		t.Fatal(e)
	}
	fixtureLastStore, fixtureLastClaims, fixtureLastAttempts = store, claims, attempts
	return g, store, r
}
func digest(t *testing.T, s string) core.Digest {
	t.Helper()
	d, e := core.CanonicalJSONDigest("fixture", "1.0.0", "Fixture", s)
	if e != nil {
		t.Fatal(e)
	}
	return d
}

func TestSystemReadyGatewayV2RejectsTypedNil(t *testing.T) {
	var store *journal.MemorySystemReadyStoreV2
	if _, e := journal.NewSystemReadyGatewayV2(store, nil, nil, nil, nil, nil); !contract.HasCode(e, contract.ErrorInvalidArgument) {
		t.Fatalf("err=%v", e)
	}
}

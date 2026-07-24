package surfacebinding

import (
	"context"
	"sync"
	"testing"
	"time"

	modelcontract "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestToolSurfaceInvocationBindingRepositoryV1CreateInspectIdempotentLostReply(t *testing.T) {
	clock := testkit.NewManualClock(testkit.FixedTime.Add(time.Second))
	repository := mustRepositoryV1(t, clock.Now)
	request := testkit.ToolSurfaceInvocationBindingRequestV1()
	binding, ack, err := repository.EnsureToolSurfaceInvocationBindingV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, secondAck, err := repository.EnsureToolSurfaceInvocationBindingV1(context.Background(), request)
	if err != nil || second.Ref != binding.Ref || secondAck.Ref != ack.Ref {
		t.Fatalf("same canonical replay did not return winner: binding=%+v ack=%+v err=%v", second.Ref, secondAck.Ref, err)
	}
	byInvocation, recoveredAck, err := repository.InspectToolSurfaceInvocationBindingByInvocationV1(context.Background(), request.Invocation)
	if err != nil || byInvocation.Ref != binding.Ref || recoveredAck.Ref != ack.Ref {
		t.Fatalf("lost reply recovery failed: binding=%+v ack=%+v err=%v", byInvocation.Ref, recoveredAck.Ref, err)
	}
	byExact, exactAck, err := repository.InspectExactToolSurfaceInvocationBindingV1(context.Background(), binding.Ref)
	if err != nil || byExact.Ref != binding.Ref || exactAck.Ref != ack.Ref {
		t.Fatalf("exact inspection failed: binding=%+v ack=%+v err=%v", byExact.Ref, exactAck.Ref, err)
	}
}

func TestToolSurfaceInvocationBindingRepositoryV1SameInvocationDriftConflicts(t *testing.T) {
	repository := mustRepositoryV1(t, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	request := testkit.ToolSurfaceInvocationBindingRequestV1()
	if _, _, err := repository.EnsureToolSurfaceInvocationBindingV1(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.RequestedNotAfterUnixNano--
	if _, _, err := repository.EnsureToolSurfaceInvocationBindingV1(context.Background(), request); err == nil {
		t.Fatal("same invocation with changed canonical request was accepted")
	}
	winner, _, err := repository.InspectToolSurfaceInvocationBindingByInvocationV1(context.Background(), request.Invocation)
	if err != nil || winner.Subject.RequestedNotAfterUnixNano == request.RequestedNotAfterUnixNano {
		t.Fatalf("canonical winner changed: %+v err=%v", winner.Subject, err)
	}
}

func TestToolSurfaceInvocationBindingRepositoryV1DeepClone(t *testing.T) {
	repository := mustRepositoryV1(t, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	request := testkit.ToolSurfaceInvocationBindingRequestV1()
	binding, _, err := repository.EnsureToolSurfaceInvocationBindingV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	binding.Subject.SurfaceCurrent.Manifest.Entries[0].EffectKinds[0] = "praxis.tool/tampered"
	request.SurfaceCurrent.Manifest.Entries[0].EffectKinds[0] = "praxis.tool/request-tampered"
	read, _, err := repository.InspectToolSurfaceInvocationBindingByInvocationV1(context.Background(), testkit.ToolSurfaceInvocationBindingRequestV1().Invocation)
	if err != nil {
		t.Fatal(err)
	}
	if got := read.Subject.SurfaceCurrent.Manifest.Entries[0].EffectKinds[0]; got != "praxis.tool/execute" {
		t.Fatalf("stored projection aliased caller memory: %s", got)
	}
}

func TestToolSurfaceInvocationBindingRepositoryV1ClockAndContextFailClosed(t *testing.T) {
	request := testkit.ToolSurfaceInvocationBindingRequestV1()
	t.Run("nil clock", func(t *testing.T) {
		var clock func() time.Time
		if repository, err := NewInMemoryRepositoryV1(testkit.Owner(), clock); err == nil || repository != nil {
			t.Fatal("nil clock passed constructor")
		}
	})
	t.Run("nil receiver", func(t *testing.T) {
		var repository *InMemoryRepositoryV1
		if _, _, err := repository.EnsureToolSurfaceInvocationBindingV1(context.Background(), request); err == nil {
			t.Fatal("typed-nil repository was accepted")
		}
	})
	t.Run("nil context", func(t *testing.T) {
		repository := mustRepositoryV1(t, func() time.Time { return testkit.FixedTime.Add(time.Second) })
		if _, _, err := repository.EnsureToolSurfaceInvocationBindingV1(nil, request); err == nil {
			t.Fatal("nil context was accepted")
		}
	})
	t.Run("canceled context", func(t *testing.T) {
		repository := mustRepositoryV1(t, func() time.Time { return testkit.FixedTime.Add(time.Second) })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, _, err := repository.EnsureToolSurfaceInvocationBindingV1(ctx, request); err != context.Canceled {
			t.Fatalf("cancellation sentinel changed: %v", err)
		}
	})
	t.Run("clock rollback", func(t *testing.T) {
		clock := testkit.NewSequenceClock(testkit.FixedTime.Add(2*time.Second), testkit.FixedTime.Add(time.Second))
		repository := mustRepositoryV1(t, clock.Now)
		if _, _, err := repository.EnsureToolSurfaceInvocationBindingV1(context.Background(), request); err == nil {
			t.Fatal("clock rollback was accepted")
		}
		if _, _, err := repository.InspectToolSurfaceInvocationBindingByInvocationV1(context.Background(), request.Invocation); err == nil {
			t.Fatal("clock rollback wrote a binding")
		}
	})
	t.Run("ttl crossing", func(t *testing.T) {
		clock := testkit.NewSequenceClock(testkit.FixedTime.Add(time.Second), testkit.FixedTime.Add(11*time.Minute))
		repository := mustRepositoryV1(t, clock.Now)
		if _, _, err := repository.EnsureToolSurfaceInvocationBindingV1(context.Background(), request); err == nil {
			t.Fatal("TTL crossing was accepted")
		}
		if _, _, err := repository.InspectToolSurfaceInvocationBindingByInvocationV1(context.Background(), request.Invocation); err == nil {
			t.Fatal("TTL crossing wrote a binding")
		}
	})
}

func TestToolSurfaceInvocationBindingRepositoryV1ConcurrentSameInvocationSingleWinner(t *testing.T) {
	repository := mustRepositoryV1(t, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	request := testkit.ToolSurfaceInvocationBindingRequestV1()
	const workers = 64
	refs := make(chan toolcontract.ToolSurfaceInvocationBindingRefV1, workers)
	errs := make(chan error, workers)
	var start sync.WaitGroup
	start.Add(1)
	var workersWG sync.WaitGroup
	for range workers {
		workersWG.Add(1)
		go func() {
			defer workersWG.Done()
			start.Wait()
			binding, _, err := repository.EnsureToolSurfaceInvocationBindingV1(context.Background(), request)
			if err != nil {
				errs <- err
				return
			}
			refs <- binding.Ref
		}()
	}
	start.Done()
	workersWG.Wait()
	close(refs)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var winner toolcontract.ToolSurfaceInvocationBindingRefV1
	for ref := range refs {
		if winner == (toolcontract.ToolSurfaceInvocationBindingRefV1{}) {
			winner = ref
		}
		if ref != winner {
			t.Fatalf("same invocation produced multiple winners: %+v %+v", winner, ref)
		}
	}
	read, _, err := repository.InspectToolSurfaceInvocationBindingByInvocationV1(context.Background(), request.Invocation)
	if err != nil || read.Ref != winner {
		t.Fatalf("winner is not inspectable: %+v err=%v", read.Ref, err)
	}
}

func TestToolSurfaceInvocationBindingRepositoryV1ConcurrentChangedCanonicalSingleWinner(t *testing.T) {
	repository := mustRepositoryV1(t, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	requestA := testkit.ToolSurfaceInvocationBindingRequestV1()
	requestB := requestA
	requestB.RequestedNotAfterUnixNano--
	requests := []toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1{requestA, requestB}
	const workers = 64
	var successes, conflicts int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for index := range workers {
		wg.Add(1)
		go func(request toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1) {
			defer wg.Done()
			_, _, err := repository.EnsureToolSurfaceInvocationBindingV1(context.Background(), request)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else {
				conflicts++
			}
		}(requests[index%len(requests)])
	}
	wg.Wait()
	if successes == 0 || conflicts == 0 || successes+conflicts != workers {
		t.Fatalf("changed canonical race did not converge: successes=%d conflicts=%d", successes, conflicts)
	}
}

func mustRepositoryV1(t *testing.T, clock func() time.Time) *InMemoryRepositoryV1 {
	t.Helper()
	repository, err := NewInMemoryRepositoryV1(testkit.Owner(), clock)
	if err != nil {
		t.Fatal(err)
	}
	return repository
}

func resealPreparedV1(t *testing.T, fact modelcontract.PreparedModelInvocationFactV1) modelcontract.PreparedModelInvocationFactV1 {
	t.Helper()
	fact.Digest = ""
	sealed, err := modelcontract.SealPreparedModelInvocationFactV1(fact)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

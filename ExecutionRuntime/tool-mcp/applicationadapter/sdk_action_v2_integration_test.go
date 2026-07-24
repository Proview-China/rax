package applicationadapter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

type sdkActionPortV2 struct {
	result       applicationcontract.SingleCallToolActionResultV2
	executeErr   error
	executeCalls atomic.Int32
	inspectCalls atomic.Int32
}

func (p *sdkActionPortV2) ExecuteSingleCallToolActionV2(context.Context, applicationcontract.SingleCallToolActionRequestV2) (applicationcontract.SingleCallToolActionResultV2, error) {
	p.executeCalls.Add(1)
	if p.executeErr != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, p.executeErr
	}
	return p.result, nil
}

func (p *sdkActionPortV2) InspectSingleCallToolActionV2(context.Context, applicationcontract.SingleCallToolActionInspectKeyV2) (applicationcontract.SingleCallToolActionResultV2, error) {
	p.inspectCalls.Add(1)
	return p.result, nil
}

func TestSingleCallToolActionSDKV2ExecuteInspectAndConcurrentStartOrInspect(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	now := fixture.binding.now
	client, err := sdk.NewSingleCallToolActionClientV2(fixture.adapter, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.binding.request.ApplicationRequest
	got, err := client.ExecuteSingleCallToolActionV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := client.InspectSingleCallToolActionV2(context.Background(), request)
	if err != nil || inspected.Digest != got.Digest {
		t.Fatalf("inspect=%#v err=%v", inspected, err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, callErr := client.ExecuteSingleCallToolActionV2(context.Background(), request)
			errs <- callErr
		}()
	}
	wg.Wait()
	close(errs)
	for callErr := range errs {
		if callErr != nil {
			t.Fatal(callErr)
		}
	}
	if fixture.execution.executeCalls.Load() != 1 {
		t.Fatalf("same canonical SDK calls reached Tool effect %d times", fixture.execution.executeCalls.Load())
	}
}

func TestSingleCallToolActionSDKV2UnknownDoesNotRetryOrInspect(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	port := &sdkActionPortV2{result: fixture.adapterResultV2(), executeErr: core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "lost Application reply")}
	client, err := sdk.NewSingleCallToolActionClientV2(port, func() time.Time { return fixture.binding.now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.ExecuteSingleCallToolActionV2(context.Background(), fixture.binding.request.ApplicationRequest); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("unknown error=%v", err)
	}
	if port.executeCalls.Load() != 1 || port.inspectCalls.Load() != 0 {
		t.Fatalf("execute=%d inspect=%d", port.executeCalls.Load(), port.inspectCalls.Load())
	}
}

func TestSingleCallToolActionSDKV2FailClosedContextClockAndTypedNil(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	request := fixture.binding.request.ApplicationRequest
	port := &sdkActionPortV2{result: fixture.adapterResultV2()}
	var typedNil *sdkActionPortV2
	if _, err := sdk.NewSingleCallToolActionClientV2(typedNil, time.Now); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil error=%v", err)
	}
	client, err := sdk.NewSingleCallToolActionClientV2(port, func() time.Time { return fixture.binding.now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.ExecuteSingleCallToolActionV2(nil, request); !core.HasCategory(err, core.ErrorInvalidArgument) || port.executeCalls.Load() != 0 {
		t.Fatalf("nil context error=%v calls=%d", err, port.executeCalls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = client.ExecuteSingleCallToolActionV2(ctx, request); err != context.Canceled || port.executeCalls.Load() != 0 {
		t.Fatalf("canceled context error=%v calls=%d", err, port.executeCalls.Load())
	}

	times := []time.Time{fixture.binding.now, fixture.binding.now.Add(-time.Nanosecond)}
	var index atomic.Int32
	rollback, err := sdk.NewSingleCallToolActionClientV2(port, func() time.Time {
		i := int(index.Add(1) - 1)
		if i >= len(times) {
			return times[len(times)-1]
		}
		return times[i]
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rollback.ExecuteSingleCallToolActionV2(context.Background(), request); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback error=%v", err)
	}

	crossingTimes := []time.Time{fixture.binding.now, time.Unix(0, request.ExpiresUnixNano)}
	index.Store(0)
	crossing, err := sdk.NewSingleCallToolActionClientV2(port, func() time.Time {
		i := int(index.Add(1) - 1)
		if i >= len(crossingTimes) {
			return crossingTimes[len(crossingTimes)-1]
		}
		return crossingTimes[i]
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = crossing.ExecuteSingleCallToolActionV2(context.Background(), request); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL crossing error=%v", err)
	}

	driftedResult := port.result
	driftedResult.Digest = core.DigestBytes([]byte("drifted-sdk-result"))
	driftedPort := &sdkActionPortV2{result: driftedResult}
	drifted, err := sdk.NewSingleCallToolActionClientV2(driftedPort, func() time.Time { return fixture.binding.now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = drifted.ExecuteSingleCallToolActionV2(context.Background(), request); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("drifted result error=%v", err)
	}

	expired, err := sdk.NewSingleCallToolActionClientV2(port, func() time.Time { return time.Unix(0, request.ExpiresUnixNano) })
	if err != nil {
		t.Fatal(err)
	}
	before := port.executeCalls.Load()
	if _, err = expired.ExecuteSingleCallToolActionV2(context.Background(), request); !core.HasCategory(err, core.ErrorPreconditionFailed) || port.executeCalls.Load() != before {
		t.Fatalf("expired request error=%v calls=%d/%d", err, before, port.executeCalls.Load())
	}
}

func (f adapterV2Fixture) adapterResultV2() applicationcontract.SingleCallToolActionResultV2 {
	request := f.binding.request.ApplicationRequest
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(request)
	if err != nil {
		panic(err)
	}
	result, err := f.adapter.InspectSingleCallToolActionV2(context.Background(), key)
	if err == nil {
		return result
	}
	result, err = f.adapter.ExecuteSingleCallToolActionV2(context.Background(), request)
	if err != nil {
		panic(err)
	}
	return result
}

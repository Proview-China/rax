package sdk

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestOfflineSDKMaxSizeConcurrencyEvidenceV1(t *testing.T) {
	if os.Getenv("PRAXIS_CONTEXT_MAX_SIZE") != "1" {
		t.Skip("set PRAXIS_CONTEXT_MAX_SIZE=1 for the bounded max-size evidence run")
	}
	request := maxSizeCompileRequestV1(t)
	const mib = uint64(1024 * 1024)
	var inputBytes uint64
	for _, item := range request.InputBundle.items {
		inputBytes += item.Ref.Length
	}
	if inputBytes != 24*mib {
		t.Fatalf("max fixture input=%d, want 24 MiB", inputBytes)
	}
	requestWire, err := EncodeCompileFrameRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(len(requestWire)) > hardWire48MiBV1 {
		t.Fatalf("max fixture request wire=%d exceeds 48 MiB", len(requestWire))
	}
	decodedRequest, err := DecodeCompileFrameRequestV1(context.Background(), requestWire)
	if err != nil || decodedRequest.Meta.RequestDigest != request.Meta.RequestDigest || decodedRequest.InputBundle.ContentSetDigest() != request.InputBundle.ContentSetDigest() {
		t.Fatalf("max fixture request round-trip drift: %v", err)
	}
	t.Logf("max-fixture request-wire=%d", len(requestWire))
	requestWire = nil
	decodedRequest = CompileFrameRequestV1{}
	runtime.GC()

	baseline, err := CompileFrameV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var outputBytes uint64
	for _, item := range baseline.Compiled.ContentBundle.items {
		outputBytes += item.Ref.Length
	}
	generatedBytes := outputBytes - inputBytes
	if generatedBytes > 52*mib || generatedBytes < 50*mib || outputBytes > 76*mib || outputBytes < 74*mib {
		t.Fatalf("unexpected max fixture accounting input=%d generated=%d output=%d", inputBytes, generatedBytes, outputBytes)
	}
	wire, err := EncodeCompileFrameResponseV1(context.Background(), baseline)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(len(wire)) > hardWire144MiBV1 {
		t.Fatalf("max fixture wire=%d exceeds 144 MiB", len(wire))
	}
	t.Logf("max-fixture input=%d generated=%d output=%d wire=%d", inputBytes, generatedBytes, outputBytes, len(wire))
	wire = nil
	baseline = CompileFrameResponseV1{}
	runtime.GC()

	for _, concurrency := range []int{1, 2, 4, 8} {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		started := time.Now()
		var wg sync.WaitGroup
		errs := make(chan error, concurrency)
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				response, compileErr := CompileFrameV1(context.Background(), request)
				if compileErr == nil && response.Compiled.CompileDigest.Validate() != nil {
					compileErr = contract.ErrConflict
				}
				errs <- compileErr
			}()
		}
		wg.Wait()
		close(errs)
		for compileErr := range errs {
			if compileErr != nil {
				t.Fatalf("concurrency %d: %v", concurrency, compileErr)
			}
		}
		elapsed := time.Since(started)
		runtime.ReadMemStats(&after)
		t.Logf("concurrency=%d ns/op=%d B/op=%d allocs/op=%d heap_alloc=%d heap_sys=%d vm_hwm_kib=%d",
			concurrency, elapsed.Nanoseconds()/int64(concurrency), (after.TotalAlloc-before.TotalAlloc)/uint64(concurrency), (after.Mallocs-before.Mallocs)/uint64(concurrency), after.HeapAlloc, after.HeapSys, processHighWaterKiBV1())
	}

	cancelCtx := &countdownContextV1{Context: context.Background(), remaining: 100}
	started := time.Now()
	response, err := CompileFrameV1(cancelCtx, request)
	if !errors.Is(err, context.Canceled) || response.Meta.RequestID != "" {
		t.Fatalf("max fixture mid-cancel returned partial response: %#v %v", response, err)
	}
	t.Logf("max-fixture cancel-to-return=%s", time.Since(started))
}

func maxSizeCompileRequestV1(t *testing.T) CompileFrameRequestV1 {
	t.Helper()
	limits := testLimitsV1(OfflineCompileFrameV1)
	sizes := []int{4 << 20, 4 << 20, 4 << 20, 4 << 20, 3 << 20, 4 << 20, 1 << 20}
	items := make([]OfflineContentItemV1, len(sizes))
	for index, size := range sizes {
		value := make([]byte, size)
		for offset := range value {
			value[offset] = byte(index + 1)
		}
		items[index] = itemV1(value)
	}
	bundle, err := newOfflineContentBundleContextV1(context.Background(), items, limits)
	if err != nil {
		t.Fatal(err)
	}
	kinds := []contract.FragmentKind{
		contract.FragmentInstruction, contract.FragmentArtifactInline, contract.FragmentConversation,
		contract.FragmentArtifactInline, contract.FragmentConversation, contract.FragmentToolResult, contract.FragmentToolCall,
	}
	candidates := make([]contract.ContextCandidate, len(items))
	for index := range items {
		candidates[index] = testkit.Candidate("max-candidate-"+strconv.Itoa(index), kinds[index], items[index].Ref, 1)
	}
	request := CompileFrameRequestV1{
		Meta: requestMetaV1(OfflineCompileFrameV1, "compile-max-size"), AttemptID: "attempt-max-size", ManifestID: "manifest-max-size",
		FrameID: "frame-max-size", GenerationID: "generation-max-size", GenerationOrdinal: 1, Recipe: testkit.Recipe(), Execution: testkit.Execution(),
		Candidates: candidates, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 1_000, InputBundle: bundle,
	}
	request.Meta.RequestDigest, err = compileRequestDigestV1(request)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func processHighWaterKiBV1() uint64 {
	payload, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(payload), "\n") {
		if !strings.HasPrefix(line, "VmHWM:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			value, _ := strconv.ParseUint(fields[1], 10, 64)
			return value
		}
	}
	return 0
}

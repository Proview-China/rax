package main

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
)

func quickConfigV1(now time.Time) api.CorePackPreviewConfigV1 {
	return api.CorePackPreviewConfigV1{ContractVersion: api.CorePackPreviewContractVersionV1, Owner: core.OwnerRef{Domain: "praxis.tool-mcp", ID: "quickstart"}, ArtifactDigest: core.DigestBytes([]byte("artifact")), SignatureDigest: core.DigestBytes([]byte("signature")), ProvenanceDigest: core.DigestBytes([]byte("provenance")), SurfaceID: "quickstart-surface-v1", ResolvedPlanDigest: core.DigestBytes([]byte("plan")), ProfileDigest: core.DigestBytes([]byte("profile")), CapabilityGrantDigest: core.DigestBytes([]byte("grant")), CreatedUnixNano: now.UnixNano(), SurfaceExpiresUnixNano: now.Add(time.Hour).UnixNano(), RequestedExpiresUnixNano: now.Add(2 * time.Hour).UnixNano()}
}

func TestRunV1PreviewAndCheck(t *testing.T) {
	now := time.Unix(40_000, 0).UTC()
	payload, _ := json.Marshal(quickConfigV1(now))
	for _, args := range [][]string{{"--config-json", string(payload)}, {"--check", "--config-json", string(payload)}} {
		var stdout, stderr bytes.Buffer
		if code := runV1(context.Background(), args, &stdout, &stderr, func() time.Time { return now }); code != 0 || stderr.Len() != 0 || bytes.Count(stdout.Bytes(), []byte{'\n'}) != 1 {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if args[0] == "--check" {
			if envelope["ok"] != true || envelope["declaration_count"] != float64(5) || envelope["reference_only"] != true || envelope["executable"] != false {
				t.Fatalf("check=%v", envelope)
			}
		} else if len(envelope["declarations"].([]any)) != 5 {
			t.Fatalf("preview=%v", envelope)
		}
	}
}

func TestRunV1FailuresWriteZeroStdout(t *testing.T) {
	now := time.Unix(41_000, 0).UTC()
	valid, _ := json.Marshal(quickConfigV1(now))
	expired := quickConfigV1(now)
	expired.SurfaceExpiresUnixNano = now.UnixNano()
	expiredJSON, _ := json.Marshal(expired)
	tests := [][]string{nil, {"--unknown"}, {"--config-json", "{}"}, {"--config-json", string(valid) + " {}"}, {"--config-json", string(expiredJSON)}}
	for _, args := range tests {
		var stdout, stderr bytes.Buffer
		if code := runV1(context.Background(), args, &stdout, &stderr, func() time.Time { return now }); code == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	if code := runV1(ctx, []string{"--config-json", string(valid)}, &stdout, &stderr, func() time.Time { return now }); code == 0 || stdout.Len() != 0 {
		t.Fatalf("cancel code=%d stdout=%q", code, stdout.String())
	}
}

func TestRunV1ConcurrentCheckDeterministic(t *testing.T) {
	now := time.Unix(42_000, 0).UTC()
	payload, _ := json.Marshal(quickConfigV1(now))
	const workers = 64
	var wg sync.WaitGroup
	outputs := make(chan string, workers)
	codes := make(chan int, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out, errout bytes.Buffer
			codes <- runV1(context.Background(), []string{"--check", "--config-json", string(payload)}, &out, &errout, func() time.Time { return now })
			outputs <- out.String()
		}()
	}
	wg.Wait()
	close(codes)
	close(outputs)
	for code := range codes {
		if code != 0 {
			t.Fatalf("code=%d", code)
		}
	}
	var want string
	for out := range outputs {
		if want == "" {
			want = out
		}
		if out != want {
			t.Fatal("concurrent check output drifted")
		}
	}
}

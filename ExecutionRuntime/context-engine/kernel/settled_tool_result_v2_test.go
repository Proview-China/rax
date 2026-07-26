package kernel_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type toolBundleV2 struct {
	Result     toolcontract.ToolResultV2                  `json:"result"`
	Projection toolcontract.SettledToolResultProjectionV1 `json:"projection"`
}
type exactToolReaderV2 struct {
	result toolcontract.ToolResultV2
	err    error
	calls  atomic.Int64
}

func (r *exactToolReaderV2) InspectSettledToolResultCurrentV2(ctx context.Context, ref toolcontract.ObjectRef) (toolcontract.ToolResultV2, error) {
	r.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if r.err != nil {
		return toolcontract.ToolResultV2{}, r.err
	}
	return r.result, nil
}

func loadToolBundleV2(t *testing.T) toolBundleV2 {
	t.Helper()
	b, e := os.ReadFile("testdata/settled_tool_result_inline_v2.json")
	if e != nil {
		t.Fatal(e)
	}
	var v toolBundleV2
	if e = json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func appendFixtureV2(t *testing.T) (*testfixture.FrameConsumptionFixtureV1, toolBundleV2, *exactToolReaderV2, kernel.AppendSettledToolResultRequestV2) {
	t.Helper()
	f, e := testfixture.NewFrameConsumptionFixtureV1()
	if e != nil {
		t.Fatal(e)
	}
	b := loadToolBundleV2(t)
	b.Projection.Inspection = b.Result.Inspection
	b.Projection.Result = toolcontract.ObjectRef{ID: b.Result.ID, Revision: b.Result.Revision, Digest: b.Result.Digest}
	b.Projection.ProjectionDigest = ""
	var sealErr error
	b.Projection, sealErr = toolcontract.SealSettledToolResultProjectionV1(b.Projection)
	if sealErr != nil {
		t.Fatal(sealErr)
	}
	r := &exactToolReaderV2{result: b.Result}
	q := kernel.AppendSettledToolResultRequestV2{Projection: b.Projection, ParentManifest: f.Result.Manifest, ParentFrame: f.Result.Frame, ParentGeneration: f.Generation, ParentGenerationExpires: f.Snapshot.GenerationExpiresUnixNano, Recipe: testkit.Recipe(), TenantScopeDigest: f.Request.TenantScopeDigest, AgentInstanceRef: f.Request.AgentInstanceRef, PromptAssetRefs: []contract.PromptAssetRefV1{}, DisclosureClass: contract.DisclosureInternalV1, PromptExpiresUnixNano: f.Snapshot.PromptExpiresUnixNano, DisclosureExpiresUnixNano: f.Snapshot.DisclosureExpiresUnixNano, AuthorityExpiresUnixNano: f.Snapshot.AuthorityExpiresUnixNano, IdempotencyKey: "tool-result-refresh-v2", CheckedUnixNano: testkit.Now, NotAfterUnixNano: testkit.Now + 600}
	return f, b, r, q
}

func TestAppendSettledToolResultV2InlineIsIncrementalAndNeutral(t *testing.T) {
	f, _, r, q := appendFixtureV2(t)
	got, e := kernel.AppendSettledToolResultV2(context.Background(), r, f.AwareStore, q)
	if e != nil {
		t.Fatal(e)
	}
	if r.calls.Load() != 1 {
		t.Fatalf("reader calls=%d", r.calls.Load())
	}
	if got.Frame.StablePrefix != f.Result.Frame.StablePrefix || !reflect.DeepEqual(got.Frame.SemiStable, f.Result.Frame.SemiStable) {
		t.Fatal("stable regions changed")
	}
	if got.Frame.DynamicTail == f.Result.Frame.DynamicTail || got.Frame.Rendered == f.Result.Frame.Rendered {
		t.Fatal("dynamic frame did not change")
	}
	last := got.Manifest.Fragments[len(got.Manifest.Fragments)-1]
	if last.Kind != contract.FragmentToolResult || last.Region != contract.RegionDynamicTail {
		t.Fatalf("unexpected fragment: %+v", last)
	}
	if got.Descriptor.FrameRef != got.Generation.RootFrame || got.Descriptor.StablePrefix != f.Result.Frame.StablePrefix {
		t.Fatal("neutral descriptor drift")
	}
	if !reflect.DeepEqual(got.Descriptor.CacheHint.InvalidationReasons, []contract.ContextCacheInvalidationReasonV1{contract.CacheInvalidationFragmentChangedV1, contract.CacheInvalidationFrameChangedV1}) {
		t.Fatalf("invalidation=%v", got.Descriptor.CacheHint.InvalidationReasons)
	}
	if got.Frame.ExpiresUnixNano != testkit.Now+600 {
		t.Fatalf("expiry=%d", got.Frame.ExpiresUnixNano)
	}
}

func TestAppendSettledToolResultV2ArtifactReferenceOnly(t *testing.T) {
	f, b, r, q := appendFixtureV2(t)
	artifact := toolcontract.ObjectRef{ID: "artifact-context-v2", Revision: 1, Digest: b.Result.PayloadDigest}
	res := b.Result
	res.Artifacts = []toolcontract.ObjectRef{artifact}
	var e error
	res, e = toolcontract.SealToolResultV2(res)
	if e != nil {
		t.Fatal(e)
	}
	p := b.Projection
	p.Result = toolcontract.ObjectRef{ID: res.ID, Revision: res.Revision, Digest: res.Digest}
	p.Inline = nil
	p.Artifact = &artifact
	p.ProjectionDigest = ""
	p, e = toolcontract.SealSettledToolResultProjectionV1(p)
	if e != nil {
		t.Fatal(e)
	}
	r.result = res
	q.Projection = p
	got, e := kernel.AppendSettledToolResultV2(context.Background(), r, f.AwareStore, q)
	if e != nil {
		t.Fatal(e)
	}
	last := got.Manifest.Fragments[len(got.Manifest.Fragments)-1]
	if last.Kind != contract.FragmentArtifactReference || last.Region != contract.RegionDynamicTail {
		t.Fatalf("artifact fragment=%+v", last)
	}
	payload, e := f.AwareStore.GetContextV1(context.Background(), last.Content)
	if e != nil {
		t.Fatal(e)
	}
	if len(payload) >= contract.MaxSettledToolResultInlineBytesV2 || string(payload) == string(b.Projection.Inline) {
		t.Fatal("artifact body was materialized")
	}
}

func TestAppendSettledToolResultV2DeterministicConcurrentWinner(t *testing.T) {
	f, _, r, q := appendFixtureV2(t)
	const n = 64
	out := make(chan kernel.AppendSettledToolResultResultV2, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := kernel.AppendSettledToolResultV2(context.Background(), r, f.AwareStore, q)
			out <- v
			errs <- e
		}()
	}
	wg.Wait()
	close(out)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first *kernel.AppendSettledToolResultResultV2
	for v := range out {
		if first == nil {
			x := v
			first = &x
		} else if v.Frame.ID != first.Frame.ID || v.Descriptor.Digest != first.Descriptor.Digest || v.Generation.RootFrame != first.Generation.RootFrame {
			t.Fatal("concurrent logical winners drifted")
		}
	}
}

func TestAppendSettledToolResultV2FailsClosed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*toolBundleV2, *exactToolReaderV2, *kernel.AppendSettledToolResultRequestV2)
		want   error
	}{{"reader_unknown", func(_ *toolBundleV2, r *exactToolReaderV2, _ *kernel.AppendSettledToolResultRequestV2) {
		r.err = contract.ErrUnknown
	}, contract.ErrUnknown}, {"result_drift", func(_ *toolBundleV2, r *exactToolReaderV2, _ *kernel.AppendSettledToolResultRequestV2) {
		r.result.Revision++
	}, contract.ErrConflict}, {"projection_drift", func(b *toolBundleV2, _ *exactToolReaderV2, q *kernel.AppendSettledToolResultRequestV2) {
		q.Projection = b.Projection
		q.Projection.PayloadRevision++
	}, contract.ErrConflict}, {"expired", func(_ *toolBundleV2, _ *exactToolReaderV2, q *kernel.AppendSettledToolResultRequestV2) {
		q.Projection.ExpiresUnixNano = testkit.Now + 500
		q.Projection.ProjectionDigest = ""
		q.Projection, _ = toolcontract.SealSettledToolResultProjectionV1(q.Projection)
		q.CheckedUnixNano = testkit.Now + 600
		q.NotAfterUnixNano = testkit.Now + 700
	}, contract.ErrConflict}, {"xor", func(_ *toolBundleV2, _ *exactToolReaderV2, q *kernel.AppendSettledToolResultRequestV2) {
		a := toolcontract.ObjectRef{ID: "bad-artifact", Revision: 1, Digest: q.Projection.PayloadDigest}
		q.Projection.Artifact = &a
	}, contract.ErrConflict}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, b, r, q := appendFixtureV2(t)
			tc.mutate(&b, r, &q)
			got, e := kernel.AppendSettledToolResultV2(context.Background(), r, f.AwareStore, q)
			if e == nil || !errors.Is(e, tc.want) {
				t.Fatalf("error=%v want=%v", e, tc.want)
			}
			if !reflect.DeepEqual(got, kernel.AppendSettledToolResultResultV2{}) {
				t.Fatal("failure exposed a frame")
			}
		})
	}
}

func TestAppendSettledToolResultV2CancellationBeforeReader(t *testing.T) {
	f, _, r, q := appendFixtureV2(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, e := kernel.AppendSettledToolResultV2(ctx, r, f.AwareStore, q)
	if !errors.Is(e, context.Canceled) || r.calls.Load() != 0 || !reflect.DeepEqual(got, kernel.AppendSettledToolResultResultV2{}) {
		t.Fatalf("got=%+v err=%v calls=%d", got, e, r.calls.Load())
	}
}

type cancelAfterFirstPutV2 struct {
	base   testfixture.ContextAwareRefStoreV1
	cancel context.CancelFunc
	once   sync.Once
	first  contract.ContentRef
}

func (s *cancelAfterFirstPutV2) GetContextV1(ctx context.Context, ref contract.ContentRef) ([]byte, error) {
	return s.base.GetContextV1(ctx, ref)
}
func (s *cancelAfterFirstPutV2) PutContextV1(ctx context.Context, value []byte) (contract.ContentRef, error) {
	ref, err := s.base.PutContextV1(ctx, value)
	if err == nil {
		s.once.Do(func() { s.first = ref; s.cancel() })
	}
	return ref, err
}

func TestAppendSettledToolResultV2StableFingerprintAndPayloadRevisionInvalidation(t *testing.T) {
	f, b, r, q := appendFixtureV2(t)
	parentReader := &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{f.Snapshot}}
	parentDescriptor, err := kernel.BuildFrameConsumptionDescriptorV1(context.Background(), parentReader, f.AwareStore, f.Request)
	if err != nil {
		t.Fatal(err)
	}
	first, err := kernel.AppendSettledToolResultV2(context.Background(), r, f.AwareStore, q)
	if err != nil {
		t.Fatal(err)
	}
	if first.Descriptor.CacheHint.StablePrefixFingerprint != parentDescriptor.CacheHint.StablePrefixFingerprint {
		t.Fatal("stable fingerprint changed")
	}
	payload := []byte("settled tool result revision two")
	changed := b.Result
	changed.PayloadRevision++
	changed.PayloadDigest = core.Digest(contract.DigestBytes(payload))
	changed, err = toolcontract.SealToolResultV2(changed)
	if err != nil {
		t.Fatal(err)
	}
	projection := b.Projection
	projection.Result = toolcontract.ObjectRef{ID: changed.ID, Revision: changed.Revision, Digest: changed.Digest}
	projection.Inspection = changed.Inspection
	projection.PayloadRevision = changed.PayloadRevision
	projection.PayloadDigest = changed.PayloadDigest
	projection.Inline = payload
	projection.ProjectionDigest = ""
	projection, err = toolcontract.SealSettledToolResultProjectionV1(projection)
	if err != nil {
		t.Fatal(err)
	}
	r.result = changed
	q.Projection = projection
	q.IdempotencyKey = "tool-result-refresh-v2-revision-2"
	second, err := kernel.AppendSettledToolResultV2(context.Background(), r, f.AwareStore, q)
	if err != nil {
		t.Fatal(err)
	}
	if second.Frame.ID == first.Frame.ID || second.Frame.DynamicTail == first.Frame.DynamicTail || second.Descriptor.CacheHint.FrameFingerprint == first.Descriptor.CacheHint.FrameFingerprint {
		t.Fatal("payload revision did not locally invalidate frame")
	}
	if second.Frame.StablePrefix != first.Frame.StablePrefix || second.Descriptor.CacheHint.StablePrefixFingerprint != first.Descriptor.CacheHint.StablePrefixFingerprint {
		t.Fatal("payload revision invalidated stable prefix")
	}
}

func TestAppendSettledToolResultV2CanceledContentResidualIsAddressedAndReusable(t *testing.T) {
	f, _, r, q := appendFixtureV2(t)
	ctx, cancel := context.WithCancel(context.Background())
	store := &cancelAfterFirstPutV2{base: f.AwareStore, cancel: cancel}
	got, err := kernel.AppendSettledToolResultV2(ctx, r, store, q)
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, kernel.AppendSettledToolResultResultV2{}) || store.first.Validate() != nil {
		t.Fatalf("got=%+v err=%v residual=%+v", got, err, store.first)
	}
	retry, err := kernel.AppendSettledToolResultV2(context.Background(), r, f.AwareStore, q)
	if err != nil {
		t.Fatal(err)
	}
	last := retry.Manifest.Fragments[len(retry.Manifest.Fragments)-1]
	if last.Content != store.first {
		t.Fatalf("content-addressed residual not reused: residual=%+v current=%+v", store.first, last.Content)
	}
}

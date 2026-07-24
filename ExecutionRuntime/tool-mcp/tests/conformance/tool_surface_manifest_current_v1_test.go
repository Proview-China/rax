package conformance_test

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

type countingC2Repository struct {
	inner        contract.ToolSurfaceManifestCurrentRepositoryV1
	inspectCalls atomic.Int64
	ensureCalls  atomic.Int64
}

func (r *countingC2Repository) InspectExactToolSurfaceManifestCurrentV1(ctx context.Context, ref contract.ToolSurfaceManifestCurrentRefV1) (contract.ToolSurfaceManifestCurrentProjectionV1, error) {
	r.inspectCalls.Add(1)
	return r.inner.InspectExactToolSurfaceManifestCurrentV1(ctx, ref)
}

func (r *countingC2Repository) EnsureExactToolSurfaceManifestCurrentV1(ctx context.Context, request contract.ToolSurfaceManifestCurrentEnsureRequestV1) (contract.ToolSurfaceManifestCurrentProjectionV1, error) {
	r.ensureCalls.Add(1)
	return r.inner.EnsureExactToolSurfaceManifestCurrentV1(ctx, request)
}

type c2ReaderConsumer struct {
	reader contract.ToolSurfaceManifestCurrentReaderV1
}

func newC2ReaderConsumer(reader contract.ToolSurfaceManifestCurrentReaderV1) (*c2ReaderConsumer, error) {
	if reader == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "C2 reader is required")
	}
	value := reflect.ValueOf(reader)
	if (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface || value.Kind() == reflect.Func || value.Kind() == reflect.Map || value.Kind() == reflect.Slice) && value.IsNil() {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "C2 reader is typed nil")
	}
	return &c2ReaderConsumer{reader: reader}, nil
}

func (c *c2ReaderConsumer) inspect(ctx context.Context, ref contract.ToolSurfaceManifestCurrentRefV1) (contract.ToolSurfaceManifestCurrentProjectionV1, error) {
	return c.reader.InspectExactToolSurfaceManifestCurrentV1(ctx, ref)
}

func TestToolSurfaceManifestCurrentConformanceV1(t *testing.T) {
	newCounting := func(t *testing.T) (*countingC2Repository, contract.ToolSurfaceManifestCurrentProjectionV1) {
		t.Helper()
		repo, err := surface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(func() time.Time { return testkit.FixedTime })
		if err != nil {
			t.Fatal(err)
		}
		counter := &countingC2Repository{inner: repo}
		winner, err := counter.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), testkit.ToolSurfaceManifestCurrentRequestV1(1))
		if err != nil {
			t.Fatal(err)
		}
		counter.ensureCalls.Store(0)
		return counter, winner
	}

	t.Run("C2-018 typed nil Reader is rejected", func(t *testing.T) {
		var repo *surface.InMemoryToolSurfaceManifestCurrentRepositoryV1
		if _, err := newC2ReaderConsumer(repo); err == nil {
			t.Fatal("typed-nil Reader passed constructor boundary")
		}
	})

	t.Run("C2-019 normal M2-shaped read has zero Ensure", func(t *testing.T) {
		counter, winner := newCounting(t)
		consumer, err := newC2ReaderConsumer(counter)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = consumer.inspect(context.Background(), winner.Ref); err != nil || counter.inspectCalls.Load() != 1 || counter.ensureCalls.Load() != 0 {
			t.Fatalf("narrow read drifted: err=%v inspect=%d ensure=%d", err, counter.inspectCalls.Load(), counter.ensureCalls.Load())
		}
	})

	t.Run("C2-021 Repository passed through Reader remains read only", func(t *testing.T) {
		counter, winner := newCounting(t)
		var reader contract.ToolSurfaceManifestCurrentReaderV1 = counter
		consumer, err := newC2ReaderConsumer(reader)
		if err != nil {
			t.Fatal(err)
		}
		_, err = consumer.inspect(context.Background(), winner.Ref)
		if err != nil || counter.ensureCalls.Load() != 0 {
			t.Fatal("Reader consumer reached Ensure")
		}
	})

	t.Run("C2-027 Projection has no Registry lookup echo", func(t *testing.T) {
		projectionType := reflect.TypeOf(contract.ToolSurfaceManifestCurrentProjectionV1{})
		for _, name := range []string{"Registry", "RegistrySnapshot", "RegistryCurrent", "RegistryReader"} {
			if _, exists := projectionType.FieldByName(name); exists {
				t.Fatalf("Projection exposes forbidden Registry echo %s", name)
			}
		}
	})

	t.Run("C2-028 Projection excludes Assembly and Prepared", func(t *testing.T) {
		projectionType := reflect.TypeOf(contract.ToolSurfaceManifestCurrentProjectionV1{})
		for _, name := range []string{"Assembly", "AssemblyCurrent", "Prepared", "PreparedCurrent"} {
			if _, exists := projectionType.FieldByName(name); exists {
				t.Fatalf("Projection exposes forbidden cross-owner echo %s", name)
			}
		}
	})

	t.Run("C2-029 Inspect path cannot call Ensure", func(t *testing.T) {
		counter, winner := newCounting(t)
		for i := 0; i < 3; i++ {
			if _, err := counter.InspectExactToolSurfaceManifestCurrentV1(context.Background(), winner.Ref); err != nil {
				t.Fatal(err)
			}
		}
		if counter.inspectCalls.Load() != 3 || counter.ensureCalls.Load() != 0 {
			t.Fatal("Inspect path reached Ensure")
		}
	})

	t.Run("C2-030 Reader method set is exact", func(t *testing.T) {
		reader := reflect.TypeOf((*contract.ToolSurfaceManifestCurrentReaderV1)(nil)).Elem()
		if reader.NumMethod() != 1 || reader.Method(0).Name != "InspectExactToolSurfaceManifestCurrentV1" {
			t.Fatalf("Reader method set drifted: %v", reader)
		}
	})

	t.Run("C2-031 Repository embeds Reader plus Ensure", func(t *testing.T) {
		repository := reflect.TypeOf((*contract.ToolSurfaceManifestCurrentRepositoryV1)(nil)).Elem()
		if repository.NumMethod() != 2 {
			t.Fatalf("Repository method set drifted: %v", repository)
		}
		if _, ok := repository.MethodByName("InspectExactToolSurfaceManifestCurrentV1"); !ok {
			t.Fatal("Repository does not include Reader")
		}
		if _, ok := repository.MethodByName("EnsureExactToolSurfaceManifestCurrentV1"); !ok {
			t.Fatal("Repository does not expose Ensure")
		}
	})

	t.Run("C2-045 Harness implementation import is outside C2", func(t *testing.T) {
		assertC2ImportBoundary(t)
	})

	t.Run("C2-046 consumer constructor accepts Reader not Repository", func(t *testing.T) {
		constructor := reflect.TypeOf(newC2ReaderConsumer)
		reader := reflect.TypeOf((*contract.ToolSurfaceManifestCurrentReaderV1)(nil)).Elem()
		if constructor.In(0) != reader {
			t.Fatalf("consumer constructor input drifted to %v", constructor.In(0))
		}
	})

	t.Run("C2-047 C2 production files are zero network", func(t *testing.T) {
		assertC2ImportBoundary(t)
	})
}

var (
	_ contract.ToolSurfaceManifestCurrentReaderV1     = (*surface.InMemoryToolSurfaceManifestCurrentRepositoryV1)(nil)
	_ contract.ToolSurfaceManifestCurrentRepositoryV1 = (*surface.InMemoryToolSurfaceManifestCurrentRepositoryV1)(nil)
)

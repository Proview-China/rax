// Package hostroot composes the Sandbox public SDK, asynchronous API and
// actual-point current Reader into one host-owned process lifecycle.
package hostroot

import (
	"context"
	"errors"
	"net"
	"net/http"
	"reflect"
	"sync/atomic"
	"time"

	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/api"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/apihandler"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/sdk"
)

const (
	ProductionHostContractVersionV1 = "praxis.sandbox/production-host/v1"
	LifecycleFactoryIDV1            = "praxis.sandbox/factory/lifecycle-v4"
	ExecutionFactoryIDV1            = "praxis.sandbox/factory/execution-v1"
)

// CurrentPlaneServerV1 is the narrow actual-point reverse-current server seam.
// The bundled implementation is dataplaneadapter.CurrentServer. It is not a
// Provider and cannot grant authority by itself.
type CurrentPlaneServerV1 interface {
	Listen() (*net.UnixListener, error)
	Serve(context.Context, *net.UnixListener) error
}

type ProductionHostConfigV1 struct {
	Backends          []contract.BackendDescriptor
	Facts             sdk.FactReader
	Lifecycle         applicationports.SandboxLifecyclePortV4
	WorkspaceCapture  ports.WorkspaceChangeSetCapturePortV1
	Checkpoint        applicationports.CheckpointParticipantDriverV1
	SnapshotArtifact  ports.SnapshotArtifactOwnerPortV2
	WorkspaceRestore  ports.WorkspaceRestoreOwnerPortV1
	WorkspaceRewind   ports.WorkspaceRewindCompositionPortV1
	Operations        api.OperationStoreV1
	Authorization     api.TransportAuthorizationV1
	CurrentPlane      CurrentPlaneServerV1
	Clock             func() time.Time
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
}

// ProductionHostV1 is the real Sandbox in-process factory product referenced
// by the component release descriptors. Runtime/Application public Owner ports
// are injected by the trusted host; this root neither constructs nor copies
// their facts.
type ProductionHostV1 struct {
	SDK          *sdk.Client
	Handler      *apihandler.SDKHandlerV1
	Service      *api.ServiceV1
	HTTP         *api.HTTPServerV1
	CurrentPlane CurrentPlaneServerV1
	httpServer   *http.Server
	ready        atomic.Bool
}

func NewProductionHostV1(config ProductionHostConfigV1) (*ProductionHostV1, error) {
	if len(config.Backends) == 0 || nilLikeV1(config.Facts) || nilLikeV1(config.Lifecycle) || nilLikeV1(config.WorkspaceCapture) || nilLikeV1(config.Checkpoint) || nilLikeV1(config.SnapshotArtifact) || nilLikeV1(config.WorkspaceRestore) || nilLikeV1(config.WorkspaceRewind) || nilLikeV1(config.Operations) || nilLikeV1(config.Authorization) || nilLikeV1(config.CurrentPlane) || config.Clock == nil || config.ReadHeaderTimeout <= 0 || config.IdleTimeout <= 0 {
		return nil, errors.New("production Sandbox host requires complete governed SDK, durable API, current-plane, authorization, and clock dependencies")
	}
	client, err := sdk.New(sdk.Config{Backends: config.Backends, Facts: config.Facts, Lifecycle: config.Lifecycle, WorkspaceCapture: config.WorkspaceCapture, Checkpoint: config.Checkpoint, SnapshotArtifact: config.SnapshotArtifact, WorkspaceRestore: config.WorkspaceRestore, WorkspaceRewind: config.WorkspaceRewind, Clock: config.Clock})
	if err != nil {
		return nil, err
	}
	handler, err := apihandler.NewSDKHandlerV1(client)
	if err != nil {
		return nil, err
	}
	service, err := api.NewServiceV1(config.Operations, handler, config.Clock)
	if err != nil {
		return nil, err
	}
	httpHandler, err := api.NewHTTPServerV1(service, config.Authorization, config.Clock)
	if err != nil {
		return nil, err
	}
	root := &ProductionHostV1{SDK: client, Handler: handler, Service: service, HTTP: httpHandler, CurrentPlane: config.CurrentPlane}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", root.serveLiveV1)
	mux.HandleFunc("GET /readyz", root.serveReadyV1)
	mux.Handle("/", httpHandler)
	root.httpServer = &http.Server{Handler: mux, ReadHeaderTimeout: config.ReadHeaderTimeout, IdleTimeout: config.IdleTimeout, MaxHeaderBytes: 64 << 10}
	return root, nil
}

// Serve starts the HTTP API and actual-point current Reader as one failure
// domain. Either listener failing cancels and closes the other. The Rust Data
// Plane remains a separately supervised process and connects only after this
// current Reader is listening.
func (r *ProductionHostV1) Serve(ctx context.Context, apiListener net.Listener) error {
	if r == nil || apiListener == nil || r.httpServer == nil || nilLikeV1(r.CurrentPlane) {
		return errors.New("production Sandbox host or API listener is nil")
	}
	currentListener, err := r.CurrentPlane.Listen()
	if err != nil {
		return err
	}
	r.ready.Store(true)
	defer r.ready.Store(false)
	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, 2)
	go func() { results <- r.CurrentPlane.Serve(serveCtx, currentListener) }()
	go func() { results <- r.httpServer.Serve(apiListener) }()

	first := <-results
	cancel()
	_ = currentListener.Close()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	_ = r.httpServer.Shutdown(shutdownCtx)
	shutdownCancel()
	_ = apiListener.Close()
	second := <-results
	return normalizeServeErrorsV1(ctx, first, second)
}

func (r *ProductionHostV1) Ready() bool {
	return r != nil && r.ready.Load()
}

func (r *ProductionHostV1) serveLiveV1(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte("ok\n"))
}

func (r *ProductionHostV1) serveReadyV1(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if !r.Ready() {
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte("not ready\n"))
		return
	}
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte("ready\n"))
}

func normalizeServeErrorsV1(ctx context.Context, values ...error) error {
	var result error
	for _, err := range values {
		if err == nil || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) {
			continue
		}
		result = errors.Join(result, err)
	}
	if result != nil {
		return result
	}
	return ctx.Err()
}

func nilLikeV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

var (
	_ CurrentPlaneServerV1 = dataplaneadapter.CurrentServer{}
)

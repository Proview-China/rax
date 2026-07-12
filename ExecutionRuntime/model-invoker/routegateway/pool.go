package routegateway

import (
	"context"
	"errors"
	"io"
	"sync"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

type poolKey struct {
	routeDigest, evidence, credentialVersion, bindingVersion, factoryID, factoryVersion, clientIdentity string
}

type poolEntry struct {
	key       poolKey
	routeID   upstream.RouteID
	provider  modelinvoker.Provider
	closer    io.Closer
	endpoint  string
	ready     chan struct{}
	createErr error
	refs      int
	stale     bool
	closed    bool
}

type adapterPool struct {
	mu            sync.Mutex
	entries       map[poolKey]*poolEntry
	closed        bool
	lifecycleErrs []error
	lifecycleWG   sync.WaitGroup
}

type adapterLease struct {
	pool     *adapterPool
	entry    *poolEntry
	provider modelinvoker.Provider
	endpoint string
	once     sync.Once
	err      error
}

func newAdapterPool() *adapterPool { return &adapterPool{entries: make(map[poolKey]*poolEntry)} }

func (p *adapterPool) acquire(ctx context.Context, key poolKey, routeID upstream.RouteID, create func(context.Context) (FactoryResult, error)) (*adapterLease, error) {
	if ctx == nil {
		return nil, gatewayError(modelinvoker.ErrorInvalidRequest, "context_nil", "context is required", nil)
	}
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, gatewayError(modelinvoker.ErrorProviderUnavailable, "gateway_closed", "route gateway is closed", nil)
		}
		if existing := p.entries[key]; existing != nil {
			ready := existing.ready
			if ready != nil {
				p.mu.Unlock()
				select {
				case <-ctx.Done():
					return nil, gatewayError(contextKind(ctx.Err()), "factory_wait_cancelled", "adapter creation wait was cancelled", ctx.Err())
				case <-ready:
				}
				continue
			}
			if existing.createErr != nil {
				err := existing.createErr
				p.mu.Unlock()
				return nil, err
			}
			if !existing.stale && !existing.closed {
				existing.refs++
				provider := existing.provider
				p.mu.Unlock()
				return &adapterLease{pool: p, entry: existing, provider: provider, endpoint: existing.endpoint}, nil
			}
		}

		var idleClosers []io.Closer
		for oldKey, old := range p.entries {
			if old.routeID != routeID || oldKey == key {
				continue
			}
			old.stale = true
			delete(p.entries, oldKey)
			if old.refs == 0 && !old.closed && old.ready == nil {
				old.closed = true
				idleClosers = append(idleClosers, old.closer)
			}
		}
		entry := &poolEntry{key: key, routeID: routeID, ready: make(chan struct{})}
		p.entries[key] = entry
		// The build is part of the pool lifecycle. Close marks the pool closed
		// under the same mutex before waiting, so no Add can race a zero-count
		// Wait. The deferred Done also covers the pre-build closed check.
		p.lifecycleWG.Add(1)
		defer p.lifecycleWG.Done()
		if len(idleClosers) > 0 {
			p.lifecycleWG.Add(1)
		}
		p.mu.Unlock()
		if len(idleClosers) > 0 {
			p.recordLifecycleError(closeAll(idleClosers))
			p.lifecycleWG.Done()
		}

		p.mu.Lock()
		if p.closed {
			err := gatewayError(modelinvoker.ErrorProviderUnavailable, "gateway_closed", "route gateway is closed", nil)
			entry.createErr = err
			if entry.ready != nil {
				close(entry.ready)
				entry.ready = nil
			}
			delete(p.entries, key)
			p.mu.Unlock()
			return nil, err
		}
		p.mu.Unlock()

		result, err := create(ctx)
		if err == nil && nilInterface(result.Provider) {
			err = gatewayError(modelinvoker.ErrorProviderUnavailable, "factory_provider_nil", "adapter factory returned a nil provider", nil)
		}
		p.mu.Lock()
		if contextErr := ctx.Err(); contextErr != nil {
			err = errors.Join(
				gatewayError(contextKind(contextErr), "factory_context_done", "adapter construction completed after its caller context ended", contextErr),
				err,
			)
		}
		if p.closed || entry.stale || p.entries[key] != entry {
			closedErr := gatewayError(modelinvoker.ErrorProviderUnavailable, "gateway_closed", "route gateway is closed", nil)
			combinedErr := errors.Join(closedErr, err)
			entry.createErr = combinedErr
			if entry.ready != nil {
				close(entry.ready)
				entry.ready = nil
			}
			delete(p.entries, key)
			p.mu.Unlock()
			closeErr := closeAll([]io.Closer{result.Closer})
			p.recordLifecycleError(closeErr)
			return nil, errors.Join(combinedErr, closeErr)
		}
		if err != nil {
			combinedErr := errors.Join(err, closeAll([]io.Closer{result.Closer}))
			entry.createErr = combinedErr
			delete(p.entries, key)
			close(entry.ready)
			entry.ready = nil
			p.mu.Unlock()
			return nil, combinedErr
		}
		entry.provider = result.Provider
		entry.closer = result.Closer
		entry.endpoint = result.Endpoint
		entry.refs = 1
		close(entry.ready)
		entry.ready = nil
		p.mu.Unlock()
		return &adapterLease{pool: p, entry: entry, provider: result.Provider, endpoint: result.Endpoint}, nil
	}
}

func (l *adapterLease) release() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		p, entry := l.pool, l.entry
		if p == nil || entry == nil {
			return
		}
		var closer io.Closer
		p.mu.Lock()
		if entry.refs > 0 {
			entry.refs--
		}
		if entry.refs == 0 && (entry.stale || p.closed) && !entry.closed {
			entry.closed = true
			closer = entry.closer
		}
		p.mu.Unlock()
		if closer != nil {
			l.err = closeAll([]io.Closer{closer})
		}
	})
	return l.err
}

func (p *adapterPool) close() error {
	if p == nil {
		return nil
	}
	var closers []io.Closer
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	for key, entry := range p.entries {
		entry.stale = true
		delete(p.entries, key)
		if entry.refs == 0 && entry.ready == nil && !entry.closed {
			entry.closed = true
			closers = append(closers, entry.closer)
		}
	}
	p.mu.Unlock()
	p.lifecycleWG.Wait()
	p.mu.Lock()
	recorded := append([]error(nil), p.lifecycleErrs...)
	p.mu.Unlock()
	return errors.Join(append(recorded, closeAll(closers))...)
}

func (p *adapterPool) recordLifecycleError(err error) {
	if p == nil || err == nil {
		return
	}
	p.mu.Lock()
	p.lifecycleErrs = append(p.lifecycleErrs, err)
	p.mu.Unlock()
}

func closeAll(closers []io.Closer) error {
	var errs []error
	for _, closer := range closers {
		if closer != nil {
			if err := closer.Close(); err != nil {
				errs = append(errs, gatewayError(modelinvoker.ErrorProviderUnavailable, "adapter_close_failed", "adapter close failed", &lifecycleCause{raw: err}))
			}
		}
	}
	return errors.Join(errs...)
}

// lifecycleCause preserves errors.Is observability for Gateway-owned Close
// failures while ensuring an accidentally formatted intermediate cause cannot
// reveal the closer's untrusted text.
type lifecycleCause struct {
	raw    error
	public error
}

func (*lifecycleCause) Error() string { return "adapter lifecycle cause" }
func (cause *lifecycleCause) Unwrap() error {
	if cause == nil {
		return nil
	}
	return cause.public
}
func (cause *lifecycleCause) Is(target error) bool {
	return cause != nil && errors.Is(cause.raw, target)
}

func contextKind(err error) modelinvoker.ErrorKind {
	if errors.Is(err, context.DeadlineExceeded) {
		return modelinvoker.ErrorTimeout
	}
	return modelinvoker.ErrorCancelled
}

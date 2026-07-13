package codexappserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"

	harnessprocess "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/process"
)

type wireEnvelope struct {
	message harnessprocess.JSONRPCMessage
	raw     json.RawMessage
}

type wireResponse struct {
	result json.RawMessage
	fault  json.RawMessage
}

type rpcWire struct {
	process *harnessprocess.Session
	nextID  atomic.Uint64

	mu      sync.Mutex
	pending map[string]chan wireResponse
	err     error
	inbound chan wireEnvelope
	done    chan struct{}
	once    sync.Once
}

func startRPCWire(ctx context.Context, config harnessprocess.Config) (*rpcWire, error) {
	if config.Protocol != harnessprocess.ProtocolCodexAppServer {
		return nil, fmt.Errorf("%w: process protocol must be codex_app_server_ndjson", ErrInvalidConfig)
	}
	processSession, err := harnessprocess.Start(ctx, config)
	if err != nil {
		return nil, err
	}
	wire := &rpcWire{
		process: processSession, pending: make(map[string]chan wireResponse),
		inbound: make(chan wireEnvelope, 128), done: make(chan struct{}),
	}
	go wire.readLoop()
	return wire, nil
}

func (w *rpcWire) readLoop() {
	for {
		frame, err := w.process.ReadFrame()
		if err != nil {
			w.fail(err)
			return
		}
		if frame.RPC == nil {
			w.fail(fmt.Errorf("%w: missing JSON-RPC projection", ErrProtocol))
			return
		}
		message := *frame.RPC
		if message.Kind == harnessprocess.JSONRPCResponse {
			key := string(message.ID)
			w.mu.Lock()
			response := w.pending[key]
			if response != nil {
				delete(w.pending, key)
			}
			w.mu.Unlock()
			if response == nil {
				w.fail(fmt.Errorf("%w: %s", ErrUnexpectedRPCID, key))
				return
			}
			response <- wireResponse{result: append(json.RawMessage(nil), message.Result...), fault: append(json.RawMessage(nil), message.Error...)}
			continue
		}
		select {
		case w.inbound <- wireEnvelope{message: message, raw: append(json.RawMessage(nil), frame.Raw...)}:
		case <-w.done:
			return
		}
	}
}

func (w *rpcWire) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if method == "" || !validJSONObject(params) {
		return nil, fmt.Errorf("%w: method and object params are required", ErrInvalidConfig)
	}
	id := w.nextID.Add(1)
	key := strconv.FormatUint(id, 10)
	response := make(chan wireResponse, 1)
	w.mu.Lock()
	if w.err != nil {
		err := w.err
		w.mu.Unlock()
		return nil, err
	}
	w.pending[key] = response
	w.mu.Unlock()
	raw, err := json.Marshal(struct {
		ID     uint64          `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}{ID: id, Method: method, Params: params})
	if err != nil {
		w.removePending(key)
		return nil, err
	}
	if err := w.process.WriteFrame(raw); err != nil {
		w.removePending(key)
		w.fail(err)
		return nil, err
	}
	select {
	case reply := <-response:
		if len(reply.fault) != 0 {
			return nil, fmt.Errorf("%w: %s", ErrRPC, reply.fault)
		}
		return reply.result, nil
	case <-ctx.Done():
		w.removePending(key)
		w.fail(ctx.Err())
		return nil, ctx.Err()
	case <-w.done:
		return nil, w.failure()
	}
}

func (w *rpcWire) notify(method string, params json.RawMessage) error {
	if method == "" || !validJSONObject(params) {
		return fmt.Errorf("%w: method and object params are required", ErrInvalidConfig)
	}
	raw, err := json.Marshal(struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}{Method: method, Params: params})
	if err != nil {
		return err
	}
	if err := w.process.WriteFrame(raw); err != nil {
		w.fail(err)
		return err
	}
	return nil
}

func (w *rpcWire) respond(id, result, fault json.RawMessage) error {
	if len(id) == 0 || (len(result) == 0) == (len(fault) == 0) {
		return fmt.Errorf("%w: response requires an id and exactly one result or error", ErrInvalidConfig)
	}
	payload := map[string]json.RawMessage{"id": append(json.RawMessage(nil), id...)}
	if len(result) != 0 {
		payload["result"] = result
	} else {
		payload["error"] = fault
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := w.process.WriteFrame(raw); err != nil {
		w.fail(err)
		return err
	}
	return nil
}

func (w *rpcWire) receive(ctx context.Context) (wireEnvelope, error) {
	select {
	case envelope := <-w.inbound:
		return envelope, nil
	default:
	}
	select {
	case envelope := <-w.inbound:
		return envelope, nil
	case <-ctx.Done():
		return wireEnvelope{}, ctx.Err()
	case <-w.done:
		select {
		case envelope := <-w.inbound:
			return envelope, nil
		default:
			return wireEnvelope{}, w.failure()
		}
	}
}

func (w *rpcWire) close() error {
	err := w.process.Close()
	w.fail(ErrClosed)
	if err != nil && !errors.Is(err, harnessprocess.ErrClosed) {
		return err
	}
	return nil
}

func (w *rpcWire) fail(err error) {
	if err == nil {
		err = io.EOF
	}
	w.once.Do(func() {
		w.mu.Lock()
		w.err = err
		w.mu.Unlock()
		close(w.done)
		if !errors.Is(err, io.EOF) && !errors.Is(err, ErrClosed) {
			go func() { _ = w.process.Close() }()
		}
	})
}

func (w *rpcWire) failure() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err == nil {
		return ErrClosed
	}
	return w.err
}

func (w *rpcWire) removePending(key string) {
	w.mu.Lock()
	delete(w.pending, key)
	w.mu.Unlock()
}

func validJSONObject(raw json.RawMessage) bool {
	if len(raw) == 0 || !json.Valid(raw) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}

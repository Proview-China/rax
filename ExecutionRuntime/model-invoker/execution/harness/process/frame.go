package process

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

// Frame is one validated native protocol frame. RPC is populated only for
// an RPC protocol.
type Frame struct {
	Raw json.RawMessage
	RPC *JSONRPCMessage
}

type frameQueue struct {
	mu     sync.Mutex
	ready  *sync.Cond
	frames []Frame
	closed bool
	err    error
}

func newFrameQueue() *frameQueue {
	queue := &frameQueue{}
	queue.ready = sync.NewCond(&queue.mu)
	return queue
}

func (q *frameQueue) push(frame Frame) {
	q.mu.Lock()
	if !q.closed {
		q.frames = append(q.frames, frame)
		q.ready.Signal()
	}
	q.mu.Unlock()
}

func (q *frameQueue) finish(err error) {
	q.mu.Lock()
	if !q.closed {
		q.closed = true
		q.err = err
		q.ready.Broadcast()
	}
	q.mu.Unlock()
}

func (q *frameQueue) pop() (Frame, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.frames) == 0 && !q.closed {
		q.ready.Wait()
	}
	if len(q.frames) > 0 {
		frame := q.frames[0]
		q.frames[0] = Frame{}
		q.frames = q.frames[1:]
		return frame, nil
	}
	if q.err != nil {
		return Frame{}, q.err
	}
	return Frame{}, io.EOF
}

type budgetReader struct {
	reader   io.Reader
	limit    int64
	read     atomic.Int64
	exceeded atomic.Bool
	onLimit  func()
}

func (r *budgetReader) Read(buffer []byte) (int, error) {
	if r.exceeded.Load() {
		return 0, ErrStdoutLimit
	}
	remaining := r.limit - r.read.Load()
	if remaining < 0 {
		r.exceed()
		return 0, ErrStdoutLimit
	}
	requested := len(buffer)
	if int64(requested) > remaining+1 {
		requested = int(remaining + 1)
	}
	count, err := r.reader.Read(buffer[:requested])
	if int64(count) <= remaining {
		r.read.Add(int64(count))
		return count, err
	}
	allowed := int(remaining)
	r.read.Add(int64(count))
	r.exceed()
	if allowed > 0 {
		return allowed, nil
	}
	return 0, ErrStdoutLimit
}

func (r *budgetReader) exceed() {
	if r.exceeded.CompareAndSwap(false, true) && r.onLimit != nil {
		r.onLimit()
	}
}

func (r *budgetReader) bytesRead() int64 { return r.read.Load() }

type cappedCapture struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	limit     int64
	total     atomic.Int64
	truncated atomic.Bool
	onLimit   func()
}

func (w *cappedCapture) Write(data []byte) (int, error) {
	w.total.Add(int64(len(data)))
	w.mu.Lock()
	remaining := w.limit - int64(w.buffer.Len())
	if remaining > 0 {
		keep := len(data)
		if int64(keep) > remaining {
			keep = int(remaining)
		}
		_, _ = w.buffer.Write(data[:keep])
	}
	w.mu.Unlock()
	if w.total.Load() > w.limit && w.truncated.CompareAndSwap(false, true) && w.onLimit != nil {
		w.onLimit()
	}
	return len(data), nil
}

func (w *cappedCapture) snapshot() ([]byte, int64, bool) {
	w.mu.Lock()
	data := append([]byte(nil), w.buffer.Bytes()...)
	w.mu.Unlock()
	return data, w.total.Load(), w.truncated.Load()
}

func decodeFrames(reader io.Reader, protocol Protocol, maxFrameBytes int, tracker *rpcTracker, emit func(Frame)) error {
	buffered := bufio.NewReaderSize(reader, 32*1024)
	for {
		payload, err := readBoundedLine(buffered, maxFrameBytes)
		if err != nil {
			return err
		}
		frame := Frame{Raw: append(json.RawMessage(nil), payload...)}
		if isRPCProtocol(protocol) {
			message, parseErr := parseRPC(payload, protocol == ProtocolCodexAppServer)
			if parseErr != nil {
				return parseErr
			}
			if trackErr := tracker.acceptIncoming(message); trackErr != nil {
				return trackErr
			}
			frame.RPC = &message
		}
		emit(frame)
	}
}

func readBoundedLine(reader *bufio.Reader, max int) ([]byte, error) {
	line := make([]byte, 0, min(max+1, 32*1024))
	for {
		fragment, err := reader.ReadSlice('\n')
		line = append(line, fragment...)
		if len(line) > max+2 {
			drainLine(reader, err)
			return nil, ErrFrameTooLarge
		}
		switch {
		case err == nil:
			payload := line[:len(line)-1]
			if len(payload) > 0 && payload[len(payload)-1] == '\r' {
				payload = payload[:len(payload)-1]
			}
			if len(payload) > max {
				return nil, ErrFrameTooLarge
			}
			if !utf8.Valid(payload) {
				return nil, ErrInvalidUTF8
			}
			if !json.Valid(payload) {
				return nil, ErrInvalidJSON
			}
			return append([]byte(nil), payload...), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, ErrStdoutLimit):
			return nil, ErrStdoutLimit
		case errors.Is(err, io.EOF):
			if len(line) == 0 {
				return nil, io.EOF
			}
			return nil, ErrPartialFrame
		default:
			return nil, fmt.Errorf("read harness frame: %w", err)
		}
	}
}

func drainLine(reader *bufio.Reader, prior error) {
	if prior == nil || errors.Is(prior, io.EOF) || errors.Is(prior, ErrStdoutLimit) {
		return
	}
	for {
		_, err := reader.ReadSlice('\n')
		if err == nil || (!errors.Is(err, bufio.ErrBufferFull) && err != nil) {
			return
		}
	}
}

func validateOutboundFrame(raw []byte, protocol Protocol, max int) (JSONRPCMessage, error) {
	if len(raw) > max {
		return JSONRPCMessage{}, ErrFrameTooLarge
	}
	if !utf8.Valid(raw) {
		return JSONRPCMessage{}, ErrInvalidUTF8
	}
	if !json.Valid(raw) {
		return JSONRPCMessage{}, ErrInvalidJSON
	}
	if isRPCProtocol(protocol) {
		return parseRPC(raw, protocol == ProtocolCodexAppServer)
	}
	return JSONRPCMessage{}, nil
}

func isRPCProtocol(protocol Protocol) bool {
	return protocol == ProtocolJSONRPCNDJSON || protocol == ProtocolCodexAppServer
}

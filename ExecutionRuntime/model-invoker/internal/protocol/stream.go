package protocol

import (
	"context"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

// BindStream places identity enforcement outside a protocol stream. Provider
// adapters must apply this wrapper after their redacting stream so redaction
// cannot alter the authoritative Binding identity.
func (b Binding) BindStream(ctx context.Context, request modelinvoker.Request, stream modelinvoker.Stream) modelinvoker.Stream {
	if IsNil(stream) {
		return nil
	}
	return &identityBoundStream{binding: b.Clone(), ctx: ctx, request: request, inner: stream}
}

type identityBoundStream struct {
	binding Binding
	ctx     context.Context
	request modelinvoker.Request
	inner   modelinvoker.Stream

	current   modelinvoker.StreamEvent
	err       error
	done      bool
	closed    bool
	innerDone bool
}

func (s *identityBoundStream) Next() bool {
	if s == nil || s.closed || s.done {
		return false
	}
	if !s.inner.Next() {
		s.done = true
		if err := s.inner.Err(); err != nil {
			s.err = s.binding.StampError(s.ctx, s.request, err, "stream")
		}
		return false
	}
	s.current = s.binding.StampEvent(s.ctx, s.request, s.inner.Event())
	return true
}

func (s *identityBoundStream) Event() modelinvoker.StreamEvent {
	if s == nil {
		return modelinvoker.StreamEvent{}
	}
	return s.current
}

func (s *identityBoundStream) Err() error {
	if s == nil {
		return nil
	}
	if s.err != nil || s.done {
		return s.err
	}
	if err := s.inner.Err(); err != nil {
		return s.binding.StampError(s.ctx, s.request, err, "stream")
	}
	return nil
}

func (s *identityBoundStream) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	s.done = true
	if s.innerDone {
		return nil
	}
	s.innerDone = true
	if err := s.inner.Close(); err != nil {
		return s.binding.StampError(s.ctx, s.request, err, "stream_close")
	}
	return nil
}

var _ modelinvoker.Stream = (*identityBoundStream)(nil)

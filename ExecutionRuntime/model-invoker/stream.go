package modelinvoker

type StreamEventType string

const (
	StreamEventResponseStarted        StreamEventType = "response_started"
	StreamEventTextDelta              StreamEventType = "text_delta"
	StreamEventFunctionCallStarted    StreamEventType = "function_call_started"
	StreamEventFunctionArgumentsDelta StreamEventType = "function_arguments_delta"
	StreamEventFunctionCallCompleted  StreamEventType = "function_call_completed"
	StreamEventReasoningDelta         StreamEventType = "reasoning_delta"
	StreamEventUsage                  StreamEventType = "usage"
	StreamEventResponseCompleted      StreamEventType = "response_completed"
	StreamEventError                  StreamEventType = "error"
	StreamEventNative                 StreamEventType = "native"
)

type StreamEvent struct {
	Type           StreamEventType
	Sequence       int64
	ResponseID     string
	TextDelta      string
	ReasoningDelta string
	ArgumentsDelta string
	FunctionCall   *FunctionCall
	Usage          *Usage
	Response       *Response
	Error          *Error
	Raw            RawPayload
}

// Stream is a synchronous iterator. Next preserves provider event order and
// does not create a background goroutine.
type Stream interface {
	Next() bool
	Event() StreamEvent
	Err() error
	Close() error
}

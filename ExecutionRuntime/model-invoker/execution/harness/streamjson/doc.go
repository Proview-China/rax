// Package streamjson implements the bounded bidirectional JSONL control
// transport shared by official Agent SDK sidecars.
//
// It deliberately owns framing and control-request correlation only. Native
// Claude and Qwen message semantics remain in their respective adapters.
package streamjson

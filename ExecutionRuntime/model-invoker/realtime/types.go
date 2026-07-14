// Package realtime defines the provider-neutral lifecycle for bidirectional
// model sessions. It is intentionally separate from SSE response streaming.
package realtime

import (
	"context"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

type Modality string

const (
	Text  Modality = "text"
	Audio Modality = "audio"
	Video Modality = "video"
)

type Request struct {
	Provider        modelinvoker.ProviderID
	Model           string
	Modalities      []Modality
	Configuration   modelinvoker.RawPayload
	Metadata        modelinvoker.Metadata
	ProviderOptions modelinvoker.ProviderOptions
	Timeout         time.Duration
}

type ClientEvent struct {
	Type   string
	Text   string
	Binary []byte
	Raw    modelinvoker.RawPayload
}

type ServerEvent struct {
	Type     string
	Sequence int64
	Text     string
	Binary   []byte
	Usage    *modelinvoker.Usage
	Error    *modelinvoker.Error
	Raw      modelinvoker.RawPayload
}

type Session interface {
	Send(context.Context, ClientEvent) error
	Next() bool
	Event() ServerEvent
	Err() error
	CloseWrite() error
	Close() error
}

type Provider interface {
	ID() modelinvoker.ProviderID
	Open(context.Context, Request) (Session, error)
}

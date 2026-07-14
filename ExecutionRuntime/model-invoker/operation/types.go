// Package operation defines the provider-neutral union for non-LLM upstream
// operations such as embeddings, media generation, speech, and asynchronous
// jobs. Provider SDK types never cross this package boundary.
package operation

import (
	"context"
	"net/http"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

type Kind string

const (
	EmbeddingCreate  Kind = "embedding.create"
	RerankCreate     Kind = "rerank.create"
	ModerationCreate Kind = "moderation.create"
	ImageGenerate    Kind = "image.generate"
	ImageEdit        Kind = "image.edit"
	ImageVariation   Kind = "image.variation"
	VideoGenerate    Kind = "video.generate"
	VideoEdit        Kind = "video.edit"
	VideoExtend      Kind = "video.extend"
	VideoRemix       Kind = "video.remix"
	VideoGet         Kind = "video.get"
	VideoDelete      Kind = "video.delete"
	VideoContent     Kind = "video.content"
	AudioTranscribe  Kind = "audio.transcribe"
	AudioTranslate   Kind = "audio.translate"
	SpeechGenerate   Kind = "speech.generate"
	MusicGenerate    Kind = "music.generate"
	TokenCount       Kind = "token.count"
	BatchCreate      Kind = "batch.create"
	BatchList        Kind = "batch.list"
	BatchGet         Kind = "batch.get"
	BatchCancel      Kind = "batch.cancel"
	BatchDelete      Kind = "batch.delete"
	BatchResults     Kind = "batch.results"
	FileCreate       Kind = "file.create"
	FileList         Kind = "file.list"
	FileGet          Kind = "file.get"
	FileDelete       Kind = "file.delete"
	FileContent      Kind = "file.content"
	StoreCreate      Kind = "store.create"
	StoreList        Kind = "store.list"
	StoreGet         Kind = "store.get"
	StoreDelete      Kind = "store.delete"
	StoreSearch      Kind = "store.search"
)

type Lifecycle string

const (
	LifecycleRequest  Lifecycle = "request"
	LifecycleJob      Lifecycle = "job"
	LifecycleResource Lifecycle = "resource"
	LifecycleRealtime Lifecycle = "realtime"
)

type Status string

const (
	StatusUnknown    Status = "unknown"
	StatusQueued     Status = "queued"
	StatusValidating Status = "validating"
	StatusRunning    Status = "running"
	StatusFinalizing Status = "finalizing"
	StatusSucceeded  Status = "succeeded"
	StatusFailed     Status = "failed"
	StatusCancelling Status = "cancelling"
	StatusCancelled  Status = "cancelled"
	StatusExpired    Status = "expired"
)

type ArtifactKind string

const (
	ArtifactImage ArtifactKind = "image"
	ArtifactVideo ArtifactKind = "video"
	ArtifactAudio ArtifactKind = "audio"
	ArtifactText  ArtifactKind = "text"
	ArtifactFile  ArtifactKind = "file"
	ArtifactJSON  ArtifactKind = "json"
)

type Artifact struct {
	Kind          ArtifactKind
	MIMEType      string
	Filename      string
	Data          []byte
	URL           string
	ResourceID    string
	SizeBytes     int64
	SHA256        string
	ExpiresAt     time.Time
	ExpiryUnknown bool
	Metadata      map[string]string
}

type JobRef struct {
	ID           string
	Status       Status
	NativeStatus string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ExpiresAt    time.Time
}

type ResourceRef struct {
	ID           string
	ParentID     string
	Status       Status
	NativeStatus string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

type Vector struct {
	Index  int
	Values []float32
}

type Ranking struct {
	Index int
	Score float64
	Text  string
}

type Result struct {
	Provider         modelinvoker.ProviderID
	Kind             Kind
	Model            string
	Status           Status
	Job              *JobRef
	Resource         *ResourceRef
	Artifacts        []Artifact
	Vectors          []Vector
	Rankings         []Ranking
	Transcript       string
	Usage            modelinvoker.Usage
	RequestID        string
	ProviderMetadata modelinvoker.ProviderMetadata
	MappingReport    MappingReport
	RawRequest       modelinvoker.RawPayload
	RawResponse      modelinvoker.RawPayload
}

type Budget struct {
	Timeout          time.Duration
	MaxResponseBytes int64
}

// Request carries a stable operation intent plus a provider-native body. Body
// is deliberately opaque and redacted: provider-specific builders own its
// schema until a field has a genuine cross-provider semantic.
type Request struct {
	Provider         modelinvoker.ProviderID
	Kind             Kind
	Model            string
	ResourceID       string
	ParentID         string
	Body             modelinvoker.RawPayload
	ContentType      string
	Query            map[string][]string
	Metadata         modelinvoker.Metadata
	ProviderOptions  modelinvoker.ProviderOptions
	IdempotencyKey   string
	Budget           Budget
	AllowDegradation bool
}

type Query struct {
	Kind  Kind
	Model string
}

type SupportLevel string

const (
	SupportNative      SupportLevel = "native"
	SupportCompatible  SupportLevel = "compatible"
	SupportPartial     SupportLevel = "partial"
	SupportUnsupported SupportLevel = "unsupported"
)

type Capability struct {
	Level       SupportLevel
	Lifecycle   Lifecycle
	Models      []string
	Limitations []string
}

type CapabilityContract map[Kind]Capability

type MappingAction string

const (
	MappingExact       MappingAction = "exact"
	MappingTransformed MappingAction = "transformed"
	MappingDegraded    MappingAction = "degraded"
	MappingRejected    MappingAction = "rejected"
)

type MappingReport struct {
	Provider modelinvoker.ProviderID
	Kind     Kind
	Model    string
	Action   MappingAction
	Detail   string
}

type StreamEventType string

const (
	StreamStarted       StreamEventType = "started"
	StreamTextDelta     StreamEventType = "text_delta"
	StreamArtifactChunk StreamEventType = "artifact_chunk"
	StreamProgress      StreamEventType = "progress"
	StreamUsage         StreamEventType = "usage"
	StreamCompleted     StreamEventType = "completed"
	StreamError         StreamEventType = "error"
	StreamNative        StreamEventType = "native"
)

type StreamEvent struct {
	Type     StreamEventType
	Sequence int64
	Text     string
	Chunk    []byte
	Progress float64
	Usage    *modelinvoker.Usage
	Result   *Result
	Error    *modelinvoker.Error
	Raw      modelinvoker.RawPayload
}

type Stream interface {
	Next() bool
	Event() StreamEvent
	Err() error
	Close() error
}

type Provider interface {
	ID() modelinvoker.ProviderID
	Capabilities(context.Context, Query) (CapabilityContract, error)
	Invoke(context.Context, Request) (Result, error)
	Stream(context.Context, Request) (Stream, error)
}

// HTTPDoer is the narrow transport seam used by native HTTP providers.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

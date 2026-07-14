// Package specs contains audited provider-native operation descriptors. The
// descriptors expose paths and lifecycle only; account/model entitlement is
// still supplied as an exact model allowlist by the host.
package specs

import (
	"net/http"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation/nativehttp"
)

func OpenAI(models map[operation.Kind][]string) []nativehttp.Spec {
	json := []string{"application/json"}
	multipart := []string{"multipart/form-data"}
	return withModels([]nativehttp.Spec{
		{Kind: operation.EmbeddingCreate, Method: http.MethodPost, Path: "/embeddings", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.ModerationCreate, Method: http.MethodPost, Path: "/moderations", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/images/generations", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactImage, ArtifactMIME: "image/png", URLKeys: []string{"url"}, Base64Keys: []string{"b64_json"}},
		{Kind: operation.ImageEdit, Method: http.MethodPost, Path: "/images/edits", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: multipart, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactImage, ArtifactMIME: "image/png", URLKeys: []string{"url"}, Base64Keys: []string{"b64_json"}},
		{Kind: operation.ImageVariation, Method: http.MethodPost, Path: "/images/variations", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: multipart, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactImage, ArtifactMIME: "image/png", URLKeys: []string{"url"}, Base64Keys: []string{"b64_json"}},
		{Kind: operation.AudioTranscribe, Method: http.MethodPost, Path: "/audio/transcriptions", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: multipart, ResponseMode: nativehttp.ResponseJSON, TranscriptKeys: []string{"text"}},
		{Kind: operation.AudioTranslate, Method: http.MethodPost, Path: "/audio/translations", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: multipart, ResponseMode: nativehttp.ResponseJSON, TranscriptKeys: []string{"text"}},
		{Kind: operation.SpeechGenerate, Method: http.MethodPost, Path: "/audio/speech", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseBinary, ArtifactKind: operation.ArtifactAudio, ArtifactMIME: "audio/mpeg"},
		{Kind: operation.VideoGenerate, Method: http.MethodPost, Path: "/videos", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, ContentTypes: multipart, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactVideo, IDKeys: []string{"id"}, StatusKeys: []string{"status"}},
		{Kind: operation.VideoEdit, Method: http.MethodPost, Path: "/videos/edits", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, ContentTypes: multipart, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactVideo, IDKeys: []string{"id"}, StatusKeys: []string{"status"}},
		{Kind: operation.VideoExtend, Method: http.MethodPost, Path: "/videos/extensions", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, ContentTypes: multipart, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactVideo, IDKeys: []string{"id"}, StatusKeys: []string{"status"}},
		{Kind: operation.VideoRemix, Method: http.MethodPost, Path: "/videos/{id}/remix", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, RequiresResourceID: true, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactVideo, IDKeys: []string{"id"}, StatusKeys: []string{"status"}},
		{Kind: operation.VideoGet, Method: http.MethodGet, Path: "/videos/{id}", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactVideo, IDKeys: []string{"id"}, StatusKeys: []string{"status"}},
		{Kind: operation.VideoDelete, Method: http.MethodDelete, Path: "/videos/{id}", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactVideo, IDKeys: []string{"id"}, StatusKeys: []string{"status"}},
		{Kind: operation.VideoContent, Method: http.MethodGet, Path: "/videos/{id}/content", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, RequiresResourceID: true, AllowedQuery: []string{"variant"}, ResponseMode: nativehttp.ResponseBinary, ArtifactKind: operation.ArtifactVideo, ArtifactMIME: "video/mp4"},
		{Kind: operation.FileCreate, Method: http.MethodPost, Path: "/files", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, ContentTypes: multipart, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}},
		{Kind: operation.FileList, Method: http.MethodGet, Path: "/files", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, AllowedQuery: []string{"after", "limit", "order", "purpose"}, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.FileGet, Method: http.MethodGet, Path: "/files/{id}", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}},
		{Kind: operation.FileDelete, Method: http.MethodDelete, Path: "/files/{id}", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}},
		{Kind: operation.FileContent, Method: http.MethodGet, Path: "/files/{id}/content", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseBinary, ArtifactKind: operation.ArtifactFile, ArtifactMIME: "application/octet-stream"},
		{Kind: operation.StoreCreate, Method: http.MethodPost, Path: "/vector_stores", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}, StatusKeys: []string{"status"}},
		{Kind: operation.StoreList, Method: http.MethodGet, Path: "/vector_stores", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, AllowedQuery: []string{"after", "before", "limit", "order"}, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.StoreGet, Method: http.MethodGet, Path: "/vector_stores/{id}", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}, StatusKeys: []string{"status"}},
		{Kind: operation.StoreDelete, Method: http.MethodDelete, Path: "/vector_stores/{id}", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}},
		{Kind: operation.StoreSearch, Method: http.MethodPost, Path: "/vector_stores/{id}/search", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, RequiresResourceID: true, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.BatchCreate, Method: http.MethodPost, Path: "/batches", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}, StatusKeys: []string{"status"}},
		{Kind: operation.BatchList, Method: http.MethodGet, Path: "/batches", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, AllowedQuery: []string{"after", "limit"}, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.BatchGet, Method: http.MethodGet, Path: "/batches/{id}", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}, StatusKeys: []string{"status"}},
		{Kind: operation.BatchCancel, Method: http.MethodPost, Path: "/batches/{id}/cancel", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}, StatusKeys: []string{"status"}},
	}, models)
}

func Anthropic(models map[operation.Kind][]string) []nativehttp.Spec {
	json := []string{"application/json"}
	multipart := []string{"multipart/form-data"}
	filesBeta := http.Header{"Anthropic-Beta": {"files-api-2025-04-14"}}
	return withModels([]nativehttp.Spec{
		{Kind: operation.TokenCount, Method: http.MethodPost, Path: "/messages/count_tokens", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.BatchCreate, Method: http.MethodPost, Path: "/messages/batches", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}, StatusKeys: []string{"processing_status"}},
		{Kind: operation.BatchList, Method: http.MethodGet, Path: "/messages/batches", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, AllowedQuery: []string{"after_id", "before_id", "limit"}, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.BatchGet, Method: http.MethodGet, Path: "/messages/batches/{id}", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}, StatusKeys: []string{"processing_status"}},
		{Kind: operation.BatchCancel, Method: http.MethodPost, Path: "/messages/batches/{id}/cancel", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}, StatusKeys: []string{"processing_status"}},
		{Kind: operation.BatchResults, Method: http.MethodGet, Path: "/messages/batches/{id}/results", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseNDJSON},
		{Kind: operation.FileCreate, Method: http.MethodPost, Path: "/files", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, ContentTypes: multipart, Headers: filesBeta, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}},
		{Kind: operation.FileList, Method: http.MethodGet, Path: "/files", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, AllowedQuery: []string{"after_id", "before_id", "limit", "scope_id"}, Headers: filesBeta, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.FileGet, Method: http.MethodGet, Path: "/files/{id}", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, RequiresResourceID: true, Headers: filesBeta, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}},
		{Kind: operation.FileDelete, Method: http.MethodDelete, Path: "/files/{id}", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, RequiresResourceID: true, Headers: filesBeta, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}},
		{Kind: operation.FileContent, Method: http.MethodGet, Path: "/files/{id}/content", Lifecycle: operation.LifecycleResource, Support: operation.SupportPartial, RequiresResourceID: true, Headers: filesBeta, ResponseMode: nativehttp.ResponseBinary, ArtifactKind: operation.ArtifactFile, ArtifactMIME: "application/octet-stream", Limitations: []string{"only provider-generated downloadable files can be downloaded"}},
	}, models)
}

func Gemini(models map[operation.Kind][]string) []nativehttp.Spec {
	json := []string{"application/json"}
	return withModels([]nativehttp.Spec{
		{Kind: operation.EmbeddingCreate, Method: http.MethodPost, Path: "/models/{model}:embedContent", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, RequiresModel: true, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/models/{model}:generateContent", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, RequiresModel: true, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactImage, ArtifactMIME: "image/png", Base64Keys: []string{"inlineData.data"}},
		{Kind: operation.SpeechGenerate, Method: http.MethodPost, Path: "/models/{model}:generateContent", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, RequiresModel: true, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactAudio, ArtifactMIME: "audio/pcm", Base64Keys: []string{"inlineData.data"}},
		{Kind: operation.VideoGenerate, Method: http.MethodPost, Path: "/models/{model}:predictLongRunning", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, RequiresModel: true, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactVideo, IDKeys: []string{"name"}, IDPrefix: "operations/", StatusKeys: []string{"state", "status"}, DoneKeys: []string{"done"}, FailureKeys: []string{"error"}},
		{Kind: operation.VideoGet, Method: http.MethodGet, Path: "/operations/{id}", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactVideo, IDKeys: []string{"name"}, IDPrefix: "operations/", StatusKeys: []string{"state", "status"}, DoneKeys: []string{"done"}, FailureKeys: []string{"error"}, URLKeys: []string{"uri"}},
		{Kind: operation.FileList, Method: http.MethodGet, Path: "/files", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, AllowedQuery: []string{"pageSize", "pageToken"}, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.FileGet, Method: http.MethodGet, Path: "/files/{id}", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"name"}, IDPrefix: "files/", StatusKeys: []string{"state"}},
		{Kind: operation.FileDelete, Method: http.MethodDelete, Path: "/files/{id}", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.BatchCreate, Method: http.MethodPost, Path: "/models/{model}:batchGenerateContent", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, RequiresModel: true, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"name"}, IDPrefix: "batches/", StatusKeys: []string{"state"}},
		{Kind: operation.BatchList, Method: http.MethodGet, Path: "/batches", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, AllowedQuery: []string{"pageSize", "pageToken"}, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.BatchGet, Method: http.MethodGet, Path: "/batches/{id}", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"name"}, IDPrefix: "batches/", StatusKeys: []string{"state"}, FailureKeys: []string{"error"}},
		{Kind: operation.BatchCancel, Method: http.MethodPost, Path: "/batches/{id}:cancel", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"name"}, IDPrefix: "batches/", StatusKeys: []string{"state"}},
		{Kind: operation.BatchDelete, Method: http.MethodDelete, Path: "/batches/{id}", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON},
	}, models)
}

func XAI(models map[operation.Kind][]string) []nativehttp.Spec {
	specs := OpenAI(models)
	confirmed := map[operation.Kind]bool{
		operation.ImageGenerate: true, operation.ImageEdit: true,
		operation.VideoGenerate: true, operation.VideoEdit: true, operation.VideoExtend: true,
		operation.VideoGet: true, operation.VideoDelete: true, operation.VideoContent: true,
		operation.FileCreate: true, operation.FileList: true, operation.FileGet: true,
		operation.FileDelete: true, operation.FileContent: true,
		operation.BatchCreate: true, operation.BatchList: true, operation.BatchGet: true, operation.BatchCancel: true,
	}
	out := make([]nativehttp.Spec, 0, len(confirmed))
	for _, spec := range specs {
		if confirmed[spec.Kind] {
			spec.Support = operation.SupportCompatible
			out = append(out, spec)
		}
	}
	out = append(out, nativehttp.Spec{Kind: operation.StoreSearch, Method: http.MethodPost, Path: "/documents/search", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: []string{"application/json"}, ResponseMode: nativehttp.ResponseJSON, Limitations: []string{"collection IDs are supplied in the provider-native request body"}})
	return out
}

// XAIManagement is a separate operation surface because collection lifecycle
// uses management-api.x.ai and a Management API key, while file upload and
// document search use api.x.ai and the inference API key.
func XAIManagement() []nativehttp.Spec {
	json := []string{"application/json"}
	return []nativehttp.Spec{
		{Kind: operation.StoreCreate, Method: http.MethodPost, Path: "/collections", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id", "collection_id"}},
		{Kind: operation.StoreList, Method: http.MethodGet, Path: "/collections", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.StoreGet, Method: http.MethodGet, Path: "/collections/{id}", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id", "collection_id"}},
		{Kind: operation.StoreDelete, Method: http.MethodDelete, Path: "/collections/{id}", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON},
	}
}

func Kimi(models map[operation.Kind][]string) []nativehttp.Spec {
	specs := OpenAI(models)
	keep := map[operation.Kind]bool{operation.FileCreate: true, operation.FileList: true, operation.FileGet: true, operation.FileDelete: true, operation.FileContent: true, operation.BatchCreate: true, operation.BatchList: true, operation.BatchGet: true, operation.BatchCancel: true}
	out := specs[:0]
	for _, spec := range specs {
		if keep[spec.Kind] {
			spec.Support = operation.SupportCompatible
			out = append(out, spec)
		}
	}
	return out
}

func ZAI(models map[operation.Kind][]string) []nativehttp.Spec {
	json := []string{"application/json"}
	multipart := []string{"multipart/form-data"}
	return withModels([]nativehttp.Spec{
		{Kind: operation.AudioTranscribe, Method: http.MethodPost, Path: "/audio/transcriptions", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: multipart, ResponseMode: nativehttp.ResponseJSON, TranscriptKeys: []string{"text"}},
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/images/generations", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactImage, ArtifactMIME: "image/png", URLKeys: []string{"url"}},
		{Kind: operation.VideoGenerate, Method: http.MethodPost, Path: "/videos/generations", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactVideo, IDKeys: []string{"id", "task_id"}, StatusKeys: []string{"status", "task_status"}},
		{Kind: operation.VideoGet, Method: http.MethodGet, Path: "/async-result/{id}", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactVideo, IDKeys: []string{"id", "task_id"}, StatusKeys: []string{"status", "task_status"}, URLKeys: []string{"video_url", "url"}},
	}, models)
}

func MiMo(models map[operation.Kind][]string) []nativehttp.Spec {
	json := []string{"application/json"}
	return withModels([]nativehttp.Spec{
		{Kind: operation.AudioTranscribe, Method: http.MethodPost, Path: "/chat/completions", Lifecycle: operation.LifecycleRequest, Support: operation.SupportCompatible, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, TranscriptKeys: []string{"content"}, Limitations: []string{"MiMo ASR uses a Chat Completions-shaped provider dialect"}},
		{Kind: operation.SpeechGenerate, Method: http.MethodPost, Path: "/chat/completions", Lifecycle: operation.LifecycleRequest, Support: operation.SupportCompatible, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactAudio, ArtifactMIME: "audio/pcm", Base64Keys: []string{"data", "audio"}, Limitations: []string{"MiMo TTS uses provider-specific assistant message and audio fields"}},
	}, models)
}

func MiniMax(models map[operation.Kind][]string) []nativehttp.Spec {
	json := []string{"application/json"}
	return withModels([]nativehttp.Spec{
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/image_generation", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactImage, ArtifactMIME: "image/png", URLKeys: []string{"image_url", "url"}, Base64Keys: []string{"base64"}},
		{Kind: operation.VideoGenerate, Method: http.MethodPost, Path: "/video_generation", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactVideo, IDKeys: []string{"task_id"}, StatusKeys: []string{"status"}},
		{Kind: operation.SpeechGenerate, Method: http.MethodPost, Path: "/t2a_v2", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactAudio, ArtifactMIME: "audio/mpeg", Base64Keys: []string{"audio"}},
		{Kind: operation.MusicGenerate, Method: http.MethodPost, Path: "/music_generation", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactAudio, ArtifactMIME: "audio/mpeg", URLKeys: []string{"audio_url", "url"}, Base64Keys: []string{"audio"}},
		{Kind: operation.FileCreate, Method: http.MethodPost, Path: "/files/upload", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, ContentTypes: []string{"multipart/form-data"}, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"file_id", "id"}},
		{Kind: operation.FileList, Method: http.MethodGet, Path: "/files/list", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, AllowedQuery: []string{"purpose"}, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.FileGet, Method: http.MethodGet, Path: "/files/retrieve/{id}", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"file_id", "id"}},
		{Kind: operation.FileDelete, Method: http.MethodPost, Path: "/files/delete/{id}", Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON},
	}, models)
}

func Qwen(models map[operation.Kind][]string) []nativehttp.Spec {
	json := []string{"application/json"}
	return withModels([]nativehttp.Spec{
		{Kind: operation.EmbeddingCreate, Method: http.MethodPost, Path: "/services/embeddings/text-embedding/text-embedding", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.RerankCreate, Method: http.MethodPost, Path: "/services/rerank/text-rerank/text-rerank", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/services/aigc/text2image/image-synthesis", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, ContentTypes: json, Headers: http.Header{"X-DashScope-Async": {"enable"}}, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactImage, ArtifactMIME: "image/png", IDKeys: []string{"task_id"}, StatusKeys: []string{"task_status"}, URLKeys: []string{"url"}},
		{Kind: operation.VideoGenerate, Method: http.MethodPost, Path: "/services/aigc/video-generation/video-synthesis", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, ContentTypes: json, Headers: http.Header{"X-DashScope-Async": {"enable"}}, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactVideo, IDKeys: []string{"task_id"}, StatusKeys: []string{"task_status"}, URLKeys: []string{"video_url", "url"}},
		{Kind: operation.VideoGet, Method: http.MethodGet, Path: "/tasks/{id}", Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, RequiresResourceID: true, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactVideo, IDKeys: []string{"task_id"}, StatusKeys: []string{"task_status"}, URLKeys: []string{"video_url", "url"}},
		{Kind: operation.SpeechGenerate, Method: http.MethodPost, Path: "/services/aigc/multimodal-generation/generation", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactAudio, ArtifactMIME: "audio/pcm", URLKeys: []string{"audio_url"}, Base64Keys: []string{"audio"}},
	}, models)
}

// QwenOpenAICompatibleBatch exposes only the officially documented
// OpenAI-compatible Files and Batch surface. It is intentionally separate from
// DashScope native media endpoints and must use the compatible-mode base URL.
func QwenOpenAICompatibleBatch() []nativehttp.Spec {
	all := OpenAI(nil)
	keep := map[operation.Kind]bool{
		operation.FileCreate: true, operation.FileList: true, operation.FileGet: true, operation.FileDelete: true, operation.FileContent: true,
		operation.BatchCreate: true, operation.BatchList: true, operation.BatchGet: true, operation.BatchCancel: true,
	}
	out := make([]nativehttp.Spec, 0, len(keep))
	for _, spec := range all {
		if keep[spec.Kind] {
			spec.Support = operation.SupportCompatible
			spec.Limitations = append(spec.Limitations, "requires the Qwen OpenAI-compatible Batch API base URL and a batch-supported model")
			out = append(out, spec)
		}
	}
	return out
}

func Ollama(models map[operation.Kind][]string) []nativehttp.Spec {
	json := []string{"application/json"}
	return withModels([]nativehttp.Spec{
		{Kind: operation.EmbeddingCreate, Method: http.MethodPost, Path: "/api/embed", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/v1/images/generations", Lifecycle: operation.LifecycleRequest, Support: operation.SupportPartial, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, ArtifactKind: operation.ArtifactImage, ArtifactMIME: "image/png", Base64Keys: []string{"b64_json"}, Limitations: []string{"Ollama OpenAI image endpoint is experimental"}},
	}, models)
}

func LlamaCPP(models map[operation.Kind][]string) []nativehttp.Spec {
	json := []string{"application/json"}
	return withModels([]nativehttp.Spec{
		{Kind: operation.EmbeddingCreate, Method: http.MethodPost, Path: "/v1/embeddings", Lifecycle: operation.LifecycleRequest, Support: operation.SupportCompatible, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, Limitations: []string{"requires an embedding model and non-none pooling"}},
		{Kind: operation.RerankCreate, Method: http.MethodPost, Path: "/v1/rerank", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON, Limitations: []string{"requires reranking server mode"}},
		{Kind: operation.TokenCount, Method: http.MethodPost, Path: "/v1/responses/input_tokens", Lifecycle: operation.LifecycleRequest, Support: operation.SupportCompatible, ContentTypes: json, ResponseMode: nativehttp.ResponseJSON},
	}, models)
}

func GenericOpenAICompatible(models map[operation.Kind][]string) []nativehttp.Spec {
	// A generic compatible server is allowed to expose only explicitly attested
	// model-bound operations. Resource/job APIs cannot be inferred from a /v1
	// prefix and therefore are intentionally omitted here.
	specs := OpenAI(models)
	out := make([]nativehttp.Spec, 0, len(specs))
	for index := range specs {
		if !modelBoundKinds[specs[index].Kind] {
			continue
		}
		specs[index].Support = operation.SupportCompatible
		specs[index].Limitations = append(specs[index].Limitations, "capability must be explicitly enabled by the self-hosted endpoint administrator")
		out = append(out, specs[index])
	}
	return out
}

func withModels(specs []nativehttp.Spec, models map[operation.Kind][]string) []nativehttp.Spec {
	out := make([]nativehttp.Spec, 0, len(specs))
	for _, spec := range specs {
		if modelBoundKinds[spec.Kind] || spec.RequiresModel {
			allowed := models[spec.Kind]
			if len(allowed) == 0 {
				continue
			}
			spec.Models = append([]string(nil), allowed...)
			if !spec.RequiresModel {
				spec.BodyModelField = "model"
			}
		}
		out = append(out, spec)
	}
	return out
}

var modelBoundKinds = map[operation.Kind]bool{
	operation.EmbeddingCreate:  true,
	operation.RerankCreate:     true,
	operation.ModerationCreate: true,
	operation.ImageGenerate:    true,
	operation.ImageEdit:        true,
	operation.ImageVariation:   true,
	operation.VideoGenerate:    true,
	operation.AudioTranscribe:  true,
	operation.AudioTranslate:   true,
	operation.SpeechGenerate:   true,
	operation.MusicGenerate:    true,
	operation.TokenCount:       true,
}

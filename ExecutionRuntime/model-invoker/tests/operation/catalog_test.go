package operation_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation/nativehttp"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation/specs"
)

func TestCatalogEnablesModelOperationsOnlyWithExactAllowlist(t *testing.T) {
	openAI := specs.OpenAI(map[operation.Kind][]string{operation.ImageGenerate: {"image-model"}})
	image, ok := findSpec(openAI, operation.ImageGenerate)
	if !ok || len(image.Models) != 1 || image.Models[0] != "image-model" || image.BodyModelField != "model" {
		t.Fatalf("image model binding missing: %+v", image)
	}
	if _, ok := findSpec(openAI, operation.SpeechGenerate); ok {
		t.Fatal("unattested model operation was enabled")
	}
	if _, ok := findSpec(openAI, operation.FileCreate); !ok {
		t.Fatal("audited official resource operation was unexpectedly removed")
	}
}

func TestGenericCompatibleDoesNotInferResourceOrBatchAPIs(t *testing.T) {
	generic := specs.GenericOpenAICompatible(map[operation.Kind][]string{operation.EmbeddingCreate: {"embed"}})
	if len(generic) != 1 || generic[0].Kind != operation.EmbeddingCreate || generic[0].Support != operation.SupportCompatible {
		t.Fatalf("generic compatible surface overclaimed capabilities: %+v", generic)
	}
}

func TestXAICatalogDoesNotInheritUnconfirmedOpenAIPeripherals(t *testing.T) {
	xai := specs.XAI(map[operation.Kind][]string{operation.ImageGenerate: {"image"}, operation.VideoGenerate: {"video"}})
	for _, forbidden := range []operation.Kind{operation.EmbeddingCreate, operation.ModerationCreate, operation.SpeechGenerate, operation.AudioTranscribe, operation.StoreCreate} {
		if _, ok := findSpec(xai, forbidden); ok {
			t.Fatalf("xAI catalog inherited unconfirmed operation %s", forbidden)
		}
	}
	for _, expected := range []operation.Kind{operation.ImageGenerate, operation.VideoGenerate, operation.FileCreate, operation.BatchCreate, operation.VideoGet, operation.VideoContent, operation.StoreSearch} {
		if _, ok := findSpec(xai, expected); !ok {
			t.Fatalf("xAI confirmed operation %s missing", expected)
		}
	}
}

func TestXAIManagementAndQwenCompatibleBatchStayOnSeparateSurfaces(t *testing.T) {
	management := specs.XAIManagement()
	for _, expected := range []operation.Kind{operation.StoreCreate, operation.StoreList, operation.StoreGet, operation.StoreDelete} {
		if _, ok := findSpec(management, expected); !ok {
			t.Fatalf("xAI management operation %s missing", expected)
		}
	}
	if _, ok := findSpec(management, operation.StoreSearch); ok {
		t.Fatal("xAI inference-key document search leaked into management-key catalog")
	}
	qwen := specs.QwenOpenAICompatibleBatch()
	if _, ok := findSpec(qwen, operation.BatchCreate); !ok {
		t.Fatal("Qwen compatible Batch create missing")
	}
	if _, ok := findSpec(qwen, operation.ImageGenerate); ok {
		t.Fatal("Qwen native media leaked into compatible Batch catalog")
	}
}

func TestGeminiCatalogBindsBatchAndLongRunningResourceNames(t *testing.T) {
	items := specs.Gemini(map[operation.Kind][]string{
		operation.BatchCreate:   {"gemini-model"},
		operation.VideoGenerate: {"veo-model"},
	})
	batch, ok := findSpec(items, operation.BatchCreate)
	if !ok || !batch.RequiresModel || len(batch.Models) != 1 || batch.Path != "/models/{model}:batchGenerateContent" {
		t.Fatalf("Gemini batch create spec is incomplete: %+v", batch)
	}
	get, ok := findSpec(items, operation.BatchGet)
	if !ok || get.IDPrefix != "batches/" || get.Path != "/batches/{id}" {
		t.Fatalf("Gemini batch resource normalization missing: %+v", get)
	}
	video, ok := findSpec(items, operation.VideoGet)
	if !ok || video.IDPrefix != "operations/" || len(video.DoneKeys) == 0 || len(video.FailureKeys) == 0 {
		t.Fatalf("Gemini long-running operation normalization missing: %+v", video)
	}
}

func findSpec(items []nativehttp.Spec, kind operation.Kind) (nativehttp.Spec, bool) {
	for _, item := range items {
		if item.Kind == kind {
			return item, true
		}
	}
	return nativehttp.Spec{}, false
}

package operation_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation/nativehttp"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation/specs"
)

func allModelAllowlist() map[operation.Kind][]string {
	return map[operation.Kind][]string{
		operation.EmbeddingCreate: {"model"}, operation.RerankCreate: {"model"}, operation.ModerationCreate: {"model"},
		operation.ImageGenerate: {"model"}, operation.ImageEdit: {"model"}, operation.ImageVariation: {"model"},
		operation.VideoGenerate: {"model"}, operation.AudioTranscribe: {"model"}, operation.AudioTranslate: {"model"},
		operation.SpeechGenerate: {"model"}, operation.MusicGenerate: {"model"}, operation.TokenCount: {"model"},
		operation.BatchCreate: {"model"},
	}
}

func TestEveryOfficialAndLocalCatalogBuildsAsAClosedNativeProvider(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	models := allModelAllowlist()
	catalogs := []struct {
		name  string
		specs []nativehttp.Spec
	}{
		{"openai", specs.OpenAI(models)},
		{"anthropic", specs.Anthropic(models)},
		{"gemini", specs.Gemini(models)},
		{"xai", specs.XAI(models)},
		{"xai-management", specs.XAIManagement()},
		{"kimi", specs.Kimi(models)},
		{"zai", specs.ZAI(models)},
		{"mimo", specs.MiMo(models)},
		{"minimax", specs.MiniMax(models)},
		{"qwen", specs.Qwen(models)},
		{"qwen-compatible-batch", specs.QwenOpenAICompatibleBatch()},
		{"ollama", specs.Ollama(models)},
		{"llamacpp", specs.LlamaCPP(models)},
		{"generic", specs.GenericOpenAICompatible(models)},
	}
	for _, catalog := range catalogs {
		t.Run(catalog.name, func(t *testing.T) {
			if len(catalog.specs) == 0 {
				t.Fatal("catalog is empty")
			}
			seen := map[operation.Kind]bool{}
			for _, spec := range catalog.specs {
				if seen[spec.Kind] {
					t.Fatalf("duplicate operation %s", spec.Kind)
				}
				seen[spec.Kind] = true
				if spec.Support == operation.SupportUnsupported || spec.Support == "" {
					t.Fatalf("non-callable spec leaked into runtime catalog: %+v", spec)
				}
			}
			provider, err := nativehttp.New(nativehttp.Config{
				Provider: modelinvoker.ProviderID("catalog-" + catalog.name), BaseURL: server.URL,
				Trust: nativehttp.TrustLocal, Auth: nativehttp.AuthAnonymous, Specs: catalog.specs,
			})
			if err != nil {
				t.Fatalf("catalog failed provider validation: %v", err)
			}
			if len(provider.Kinds()) != len(catalog.specs) {
				t.Fatalf("provider kind count drifted: %d != %d", len(provider.Kinds()), len(catalog.specs))
			}
		})
	}
}

func TestArtifactValidationMatrix(t *testing.T) {
	valid := []operation.Artifact{
		{Kind: operation.ArtifactImage},
		{Kind: operation.ArtifactAudio, Data: []byte("audio")},
		{Kind: operation.ArtifactFile, URL: "https://example.com/file"},
		{Kind: operation.ArtifactVideo, ResourceID: "video-1"},
	}
	for index, artifact := range valid {
		if err := operation.ValidateArtifact(artifact); err != nil {
			t.Fatalf("valid artifact %d rejected: %v", index, err)
		}
	}
	invalid := []operation.Artifact{
		{},
		{Kind: operation.ArtifactFile, Data: []byte("x"), URL: "https://example.com/x"},
		{Kind: operation.ArtifactFile, URL: "ftp://example.com/x"},
		{Kind: operation.ArtifactFile, URL: "https://user:pass@example.com/x"},
		{Kind: operation.ArtifactFile, URL: "://bad"},
		{Kind: operation.ArtifactFile, SizeBytes: -1},
	}
	for index, artifact := range invalid {
		if err := operation.ValidateArtifact(artifact); err == nil {
			t.Fatalf("invalid artifact %d accepted: %+v", index, artifact)
		}
	}
}

func TestRequestValidationExtendedNegativeMatrix(t *testing.T) {
	base := operation.Request{Provider: "p", Kind: operation.ImageGenerate}
	cases := []operation.Request{
		func() operation.Request { r := base; r.Budget.Timeout = -1; return r }(),
		func() operation.Request { r := base; r.Budget.MaxResponseBytes = -1; return r }(),
		func() operation.Request { r := base; r.ParentID = "bad\x00id"; return r }(),
		func() operation.Request { r := base; r.IdempotencyKey = "bad\nkey"; return r }(),
		func() operation.Request { r := base; r.Query = map[string][]string{"": {"x"}}; return r }(),
		func() operation.Request { r := base; r.Query = map[string][]string{"ok": {"bad\rvalue"}}; return r }(),
		func() operation.Request { r := base; r.Metadata = modelinvoker.Metadata{" ": "x"}; return r }(),
		func() operation.Request {
			r := base
			r.ProviderOptions = modelinvoker.ProviderOptions{"": []byte(`{}`)}
			return r
		}(),
		func() operation.Request {
			r := base
			r.ProviderOptions = modelinvoker.ProviderOptions{"p": []byte(`{`)}
			return r
		}(),
	}
	for index, request := range cases {
		if err := request.Validate(); err == nil {
			t.Fatalf("invalid request %d accepted: %+v", index, request)
		}
	}
	if err := (operation.Request{Provider: "p", Kind: operation.ImageGenerate, ContentType: "application/json", Body: modelinvoker.NewRawPayload([]byte(`{}`)), Query: map[string][]string{"n": {"1"}}, Metadata: modelinvoker.Metadata{"trace": "x"}, ProviderOptions: modelinvoker.ProviderOptions{"p": []byte(`{}`)}}).Validate(); err != nil {
		t.Fatalf("valid extended request rejected: %v", err)
	}
}

func TestNativeHTTPConfigurationAndSpecNegativeMatrix(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	validSpec := nativehttp.Spec{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/image", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON}
	valid := nativehttp.Config{Provider: "p", BaseURL: server.URL, Trust: nativehttp.TrustLocal, Auth: nativehttp.AuthAnonymous, Specs: []nativehttp.Spec{validSpec}}
	mutate := func(change func(*nativehttp.Config)) nativehttp.Config {
		copy := valid
		copy.Specs = append([]nativehttp.Spec(nil), valid.Specs...)
		change(&copy)
		return copy
	}
	cases := []nativehttp.Config{
		mutate(func(c *nativehttp.Config) { c.Provider = "" }),
		mutate(func(c *nativehttp.Config) { c.BaseURL = "" }),
		mutate(func(c *nativehttp.Config) { c.UserAgent = "bad\nagent" }),
		mutate(func(c *nativehttp.Config) { c.StaticHeaders = http.Header{"Cookie": {"x"}} }),
		mutate(func(c *nativehttp.Config) { c.StaticHeaders = http.Header{"X-Test": {"bad\nvalue"}} }),
		mutate(func(c *nativehttp.Config) { c.Trust = nativehttp.TrustMode("bad") }),
		mutate(func(c *nativehttp.Config) { c.Auth = nativehttp.AuthBearer }),
		mutate(func(c *nativehttp.Config) { c.Auth = nativehttp.AuthHeader; c.APIKey = "key" }),
		mutate(func(c *nativehttp.Config) { c.Auth = nativehttp.AuthMode("bad") }),
		mutate(func(c *nativehttp.Config) { c.Specs = nil }),
		mutate(func(c *nativehttp.Config) { c.Specs = append(c.Specs, c.Specs[0]) }),
	}
	badSpecs := []nativehttp.Spec{
		{},
		{Kind: operation.ImageGenerate, Method: "TRACE", Path: "/x", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "relative", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/{unknown}", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/{id}", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/x", RequiresResourceID: true, Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/x", BodyModelField: "model", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/x", Lifecycle: operation.LifecycleRealtime, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/x", Lifecycle: operation.LifecycleRequest, Support: operation.SupportUnsupported, ResponseMode: nativehttp.ResponseJSON},
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/x", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseMode("bad")},
		{Kind: operation.ImageGenerate, Method: http.MethodPost, Path: "/x", Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON, Headers: http.Header{"Authorization": {"x"}}},
	}
	for _, spec := range badSpecs {
		cases = append(cases, mutate(func(c *nativehttp.Config) { c.Specs = []nativehttp.Spec{spec} }))
	}
	for index, config := range cases {
		if _, err := nativehttp.New(config); err == nil {
			t.Fatalf("invalid native config %d accepted: %s", index, fmt.Sprintf("%#v", config))
		}
	}
	const secret = "configuration-secret"
	formatted := fmt.Sprintf("%v %#v", nativehttp.Config{APIKey: secret}, nativehttp.Config{APIKey: secret})
	if strings.Contains(formatted, secret) {
		t.Fatal("native configuration formatting leaked credential")
	}
}

package operation_test

import (
	"net/http"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation/nativehttp"
)

func FuzzOperationRequestValidationNeverPanics(f *testing.F) {
	f.Add("provider", "image.generate", "model", "application/json", `{"model":"model"}`)
	f.Add("p\n", "unknown", "../m", "bad type", "{")
	f.Fuzz(func(t *testing.T, provider, kind, model, contentType, body string) {
		request := operation.Request{
			Provider: modelinvoker.ProviderID(provider), Kind: operation.Kind(kind), Model: model,
			ContentType: contentType, Body: modelinvoker.NewRawPayload([]byte(body)),
		}
		_ = request.Validate()
	})
}

func FuzzNativeOperationSpecValidationNeverLeaksCredential(f *testing.F) {
	f.Add("/images", "POST", "secret")
	f.Add("/../escape", "TRACE", "secret\nheader")
	f.Fuzz(func(t *testing.T, path, method, credential string) {
		config := nativehttp.Config{
			Provider: "p", BaseURL: "http://127.0.0.1:1", Trust: nativehttp.TrustLocal,
			Auth: nativehttp.AuthBearer, APIKey: credential,
			Specs:      []nativehttp.Spec{{Kind: operation.ImageGenerate, Path: path, Method: method, Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON}},
			HTTPClient: &http.Client{},
		}
		_, _ = nativehttp.New(config)
	})
}

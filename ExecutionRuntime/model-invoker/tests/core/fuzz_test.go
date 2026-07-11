package core_test

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	. "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

func FuzzRequestValidateSchemaInputs(f *testing.F) {
	f.Add([]byte(`{"type":"object","properties":{},"required":[],"additionalProperties":false}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`not-json`))
	f.Fuzz(func(t *testing.T, schema []byte) {
		request := validRequest()
		strict := false
		request.Tools = []Tool{{Name: "tool", Parameters: append(json.RawMessage(nil), schema...), Strict: &strict}}
		_ = request.Validate()
	})
}

func FuzzRawPayloadDefensiveCopyAndRedaction(f *testing.F) {
	f.Add([]byte("secret"))
	f.Add([]byte{})
	f.Add([]byte{0, 1, 2, 255})
	f.Fuzz(func(t *testing.T, input []byte) {
		payload := NewRawPayload(input)
		if payload.String() != testRedactedPayload {
			t.Fatal("String() exposed payload")
		}
		got := payload.Bytes()
		if !bytes.Equal(got, input) {
			t.Fatal("Bytes() did not preserve payload")
		}
		if len(got) > 0 {
			got[0] ^= 0xff
			if bytes.Equal(got, payload.Bytes()) {
				t.Fatal("Bytes() did not return a defensive copy")
			}
		}
	})
}

func FuzzAdapterCoreRedactorEscapedForms(f *testing.F) {
	const secret = `sk-fuzz/" a&<`
	encoded, _ := json.Marshal(secret)
	forms := []string{secret, string(encoded[1 : len(encoded)-1]), url.QueryEscape(secret), url.PathEscape(secret)}
	for _, form := range forms {
		f.Add("prefix:" + form + ":suffix")
	}
	redactor := adaptercore.NewRedactor(secret)
	f.Fuzz(func(t *testing.T, input string) {
		output := redactor.String(input)
		for _, form := range forms {
			if strings.Contains(output, form) {
				t.Fatalf("redacted output retained secret form %q", form)
			}
		}
	})
}

func BenchmarkEvaluateCapabilities(b *testing.B) {
	request := validRequest()
	request.Stream = true
	request.Tools = []Tool{{
		Name: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{},"required":[],"additionalProperties":false}`),
	}}
	request.Output.Type = OutputJSONObject
	contract := CapabilityContract{}
	for _, capability := range RequiredCapabilities(request) {
		contract[capability] = CapabilitySupport{Level: SupportNative}
	}
	b.ResetTimer()
	for range b.N {
		if _, err := EvaluateCapabilities(request, contract); err != nil {
			b.Fatal(err)
		}
	}
}

package preparedproviderinjectionv1_test

import (
	"encoding/json"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func TestPreparedProviderInjectionShapeV1RejectsInvalidIdentityAndToolChoice(t *testing.T) {
	invalidUTF8 := string([]byte{'x', 0xff})
	tests := map[string]func(*modelinvoker.Request){
		"blank provider": func(request *modelinvoker.Request) {
			request.Provider = " "
		},
		"invalid provider UTF-8": func(request *modelinvoker.Request) {
			request.Provider = modelinvoker.ProviderID(invalidUTF8)
		},
		"auto protocol": func(request *modelinvoker.Request) {
			request.Protocol = modelinvoker.ProtocolAuto
		},
		"unknown protocol": func(request *modelinvoker.Request) {
			request.Protocol = modelinvoker.Protocol("future")
		},
		"blank tool name": func(request *modelinvoker.Request) {
			request.Tools[0].Name = " "
		},
		"invalid tool name UTF-8": func(request *modelinvoker.Request) {
			request.Tools[0].Name = invalidUTF8
		},
		"invalid description UTF-8": func(request *modelinvoker.Request) {
			request.Tools[0].Description = invalidUTF8
		},
		"duplicate tool name": func(request *modelinvoker.Request) {
			request.Tools[1].Name = request.Tools[0].Name
		},
		"auto with name": func(request *modelinvoker.Request) {
			request.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceAuto, Name: "workspace.read"}
		},
		"none with name": func(request *modelinvoker.Request) {
			request.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceNone, Name: "workspace.read"}
		},
		"required with name": func(request *modelinvoker.Request) {
			request.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceRequired, Name: "workspace.read"}
		},
		"required without tools": func(request *modelinvoker.Request) {
			request.Tools = nil
			request.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceRequired}
		},
		"function blank name": func(request *modelinvoker.Request) {
			request.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceFunction, Name: " "}
		},
		"function invalid UTF-8": func(request *modelinvoker.Request) {
			request.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceFunction, Name: invalidUTF8}
		},
		"function unknown name": func(request *modelinvoker.Request) {
			request.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceFunction, Name: "workspace.unknown"}
		},
		"unknown choice": func(request *modelinvoker.Request) {
			request.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceMode("future")}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			request := goldenRequestV1()
			mutate(&request)
			if shape, err := modelinvoker.BuildPreparedProviderInjectionShapeV1(request); err == nil {
				t.Fatalf("invalid request built shape %#v", shape)
			}
			if digest, err := modelinvoker.ComputeActualProviderInjectionDigestV1(request); err == nil || digest != "" {
				t.Fatalf("invalid request digest=%q err=%v", digest, err)
			}
		})
	}
}

func TestPreparedProviderInjectionShapeV1RejectsInvalidStrictJSONObjectAxes(t *testing.T) {
	oversized := json.RawMessage(`{"x":"` + strings.Repeat("a", 1<<20) + `"}`)
	invalidUTF8 := json.RawMessage([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})
	documents := map[string]json.RawMessage{
		"empty":               nil,
		"recursive duplicate": json.RawMessage(`{"outer":{"x":1,"x":2}}`),
		"trailing":            json.RawMessage(`{} {}`),
		"invalid UTF-8":       invalidUTF8,
		"array":               json.RawMessage(`[]`),
		"null":                json.RawMessage(`null`),
		"scalar":              json.RawMessage(`1`),
		"oversized":           oversized,
		"unpaired high d800":  json.RawMessage(`{"value":"\ud800"}`),
		"nested high d801":    json.RawMessage(`{"outer":[{"value":"\ud801"}]}`),
		"low dc00 in key":     json.RawMessage(`{"\udc00":"value"}`),
		"high then non-low":   json.RawMessage(`{"value":"\ud800\u0041"}`),
		"high then literal":   json.RawMessage(`{"value":"\ud800x"}`),
	}
	for name, raw := range documents {
		t.Run("parameters "+name, func(t *testing.T) {
			request := goldenRequestV1()
			request.Tools[0].Parameters = raw
			if _, err := modelinvoker.BuildPreparedProviderInjectionShapeV1(request); err == nil {
				t.Fatal("invalid tool parameters were accepted")
			}
			if digest, err := modelinvoker.ComputeActualProviderInjectionDigestV1(request); err == nil || digest != "" {
				t.Fatalf("invalid tool parameters digest=%q err=%v", digest, err)
			}
		})
		t.Run("options "+name, func(t *testing.T) {
			request := goldenRequestV1()
			request.ProviderOptions = modelinvoker.ProviderOptions{"openai": raw}
			if _, err := modelinvoker.BuildPreparedProviderInjectionShapeV1(request); err == nil {
				t.Fatal("invalid provider options were accepted")
			}
			if digest, err := modelinvoker.ComputeActualProviderInjectionDigestV1(request); err == nil || digest != "" {
				t.Fatalf("invalid provider options digest=%q err=%v", digest, err)
			}
		})
	}
}

func TestPreparedProviderInjectionShapeV1RejectsWrongOrMultipleProviderOptionsNamespaces(t *testing.T) {
	tests := []modelinvoker.ProviderOptions{
		{"anthropic": json.RawMessage(`{}`)},
		{"openai": json.RawMessage(`{}`), "anthropic": json.RawMessage(`{}`)},
		{modelinvoker.ProviderID(string([]byte{'x', 0xff})): json.RawMessage(`{}`)},
	}
	for index, options := range tests {
		request := goldenRequestV1()
		request.ProviderOptions = options
		if _, err := modelinvoker.BuildPreparedProviderInjectionShapeV1(request); err == nil {
			t.Fatalf("case %d invalid namespace was accepted", index)
		}
	}
}

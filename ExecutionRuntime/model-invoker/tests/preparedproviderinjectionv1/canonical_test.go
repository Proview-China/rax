package preparedproviderinjectionv1_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestPreparedProviderInjectionShapeV1Golden(t *testing.T) {
	request := goldenRequestV1()
	shape, err := modelinvoker.BuildPreparedProviderInjectionShapeV1(request)
	if err != nil {
		t.Fatal(err)
	}
	if shape.ContractVersion != modelinvoker.PreparedProviderInjectionShapeContractVersionV1 ||
		shape.Provider != request.Provider ||
		shape.Protocol != request.Protocol ||
		len(shape.OrderedTools) != 2 ||
		shape.ToolChoice != (modelinvoker.PreparedProviderToolChoiceV1{Mode: "function", Name: "workspace.read"}) ||
		shape.ParallelToolCalls != modelinvoker.PreparedProviderTriStateFalseV1 ||
		shape.ProviderOptions.Provider != request.Provider ||
		!shape.ProviderOptions.Present {
		t.Fatalf("unexpected shape: %#v", shape)
	}
	if got := string(shape.OrderedTools[0].Parameters); got != `{"properties":{"path":{"type":"string"}},"required":["path"],"type":"object"}` {
		t.Fatalf("canonical parameters = %s", got)
	}
	if got := string(shape.ProviderOptions.CanonicalJSON); got != `{"hosted":{"mode":"search"},"limit":1.0}` {
		t.Fatalf("canonical provider options = %s", got)
	}

	digest, err := modelinvoker.ComputeActualProviderInjectionDigestV1(request)
	if err != nil {
		t.Fatal(err)
	}
	const want = core.Digest("sha256:48a493182a43572797ac61ef555d85610bd0e7bd83ee74c11f59ee22b498fba8")
	if digest != want {
		t.Fatalf("digest = %q, want %q", digest, want)
	}
	direct, err := core.CanonicalJSONDigest(
		"praxis.model-invoker.prepared-provider-injection",
		"v1",
		"PreparedProviderInjectionShapeV1",
		shape,
	)
	if err != nil || direct != digest {
		t.Fatalf("direct digest = %q, err=%v; computed=%q", direct, err, digest)
	}
}

func TestPreparedProviderInjectionShapeV1CanonicalAxes(t *testing.T) {
	base := goldenRequestV1()
	baseDigest := mustDigestV1(t, base)

	axes := map[string]func(*modelinvoker.Request){
		"provider": func(request *modelinvoker.Request) {
			request.Provider = "anthropic"
			request.Protocol = modelinvoker.ProtocolMessages
			request.ProviderOptions = modelinvoker.ProviderOptions{"anthropic": json.RawMessage(`{"limit":1.0,"hosted":{"mode":"search"}}`)}
		},
		"protocol": func(request *modelinvoker.Request) {
			request.Protocol = modelinvoker.ProtocolChatCompletions
		},
		"tool order": func(request *modelinvoker.Request) {
			request.Tools[0], request.Tools[1] = request.Tools[1], request.Tools[0]
		},
		"tool count": func(request *modelinvoker.Request) {
			request.Tools = request.Tools[:1]
		},
		"tool name": func(request *modelinvoker.Request) {
			request.Tools[1].Name = "workspace.stat"
		},
		"tool description": func(request *modelinvoker.Request) {
			request.Tools[0].Description = "read one exact bounded file"
		},
		"tool parameters": func(request *modelinvoker.Request) {
			request.Tools[0].Parameters = json.RawMessage(`{"type":"object","required":["path","line"],"properties":{"path":{"type":"string"},"line":{"type":"integer"}}}`)
		},
		"tool strict": func(request *modelinvoker.Request) {
			request.Tools[0].Strict = nil
		},
		"tool choice": func(request *modelinvoker.Request) {
			request.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceRequired}
		},
		"parallel": func(request *modelinvoker.Request) {
			request.ParallelToolCalls = boolV1(true)
		},
		"options presence": func(request *modelinvoker.Request) {
			request.ProviderOptions = nil
		},
		"options body": func(request *modelinvoker.Request) {
			request.ProviderOptions = modelinvoker.ProviderOptions{"openai": json.RawMessage(`{"limit":2,"hosted":{"mode":"search"}}`)}
		},
	}
	seen := map[core.Digest]string{baseDigest: "base"}
	for name, mutate := range axes {
		t.Run(name, func(t *testing.T) {
			request := goldenRequestV1()
			mutate(&request)
			digest := mustDigestV1(t, request)
			if digest == baseDigest {
				t.Fatalf("%s did not change digest %q", name, digest)
			}
			if prior, exists := seen[digest]; exists {
				t.Fatalf("%s collided with %s at %q", name, prior, digest)
			}
			seen[digest] = name
		})
	}
}

func TestPreparedProviderInjectionShapeV1NormalizesJSONButPreservesNumberLexemes(t *testing.T) {
	first := goldenRequestV1()
	second := goldenRequestV1()
	second.Tools[0].Parameters = json.RawMessage("{\n  \"required\" : [\"path\"], \"type\":\"object\", \"properties\":{\"path\":{\"type\":\"string\"}}\n}")
	second.ProviderOptions = modelinvoker.ProviderOptions{
		"openai": json.RawMessage(`{ "hosted" : {"mode":"search"}, "limit" : 1.0 }`),
	}
	if left, right := mustDigestV1(t, first), mustDigestV1(t, second); left != right {
		t.Fatalf("key order/whitespace changed digest: %q != %q", left, right)
	}

	integer := goldenRequestV1()
	integer.ProviderOptions = modelinvoker.ProviderOptions{"openai": json.RawMessage(`{"hosted":{"mode":"search"},"limit":1}`)}
	if left, right := mustDigestV1(t, first), mustDigestV1(t, integer); left == right {
		t.Fatalf("1 and 1.0 collapsed at %q", left)
	}
}

func TestPreparedProviderInjectionShapeV1CanonicalizesPairedSurrogatesAsUnicodeScalars(t *testing.T) {
	for name, mutate := range map[string]func(*modelinvoker.Request, json.RawMessage){
		"parameters": func(request *modelinvoker.Request, raw json.RawMessage) {
			request.Tools[0].Parameters = raw
		},
		"provider options": func(request *modelinvoker.Request, raw json.RawMessage) {
			request.ProviderOptions = modelinvoker.ProviderOptions{"openai": raw}
		},
	} {
		t.Run(name, func(t *testing.T) {
			escaped := goldenRequestV1()
			literal := goldenRequestV1()
			mutate(&escaped, json.RawMessage(`{"value":"\ud83d\ude00"}`))
			mutate(&literal, json.RawMessage(`{"value":"😀"}`))

			escapedShape, err := modelinvoker.BuildPreparedProviderInjectionShapeV1(escaped)
			if err != nil {
				t.Fatal(err)
			}
			literalShape, err := modelinvoker.BuildPreparedProviderInjectionShapeV1(literal)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(escapedShape, literalShape) {
				t.Fatalf("paired surrogate and literal scalar differ:\nescaped=%#v\nliteral=%#v", escapedShape, literalShape)
			}
			if left, right := mustDigestV1(t, escaped), mustDigestV1(t, literal); left != right {
				t.Fatalf("paired surrogate and literal scalar digests differ: %q != %q", left, right)
			}
		})
	}
}

func TestPreparedProviderInjectionShapeV1PreservesAllChoiceAndTriStates(t *testing.T) {
	choices := []modelinvoker.ToolChoice{
		{Mode: modelinvoker.ToolChoiceAuto},
		{Mode: modelinvoker.ToolChoiceNone},
		{Mode: modelinvoker.ToolChoiceRequired},
		{Mode: modelinvoker.ToolChoiceFunction, Name: "workspace.read"},
	}
	choiceDigests := make(map[core.Digest]struct{}, len(choices))
	for _, choice := range choices {
		request := goldenRequestV1()
		request.ToolChoice = choice
		choiceDigests[mustDigestV1(t, request)] = struct{}{}
	}
	if len(choiceDigests) != len(choices) {
		t.Fatalf("tool choice states collapsed: %d digests", len(choiceDigests))
	}

	for name, apply := range map[string]func(*modelinvoker.Request){
		"strict": func(request *modelinvoker.Request) {
			request.Tools[0].Strict = nil
		},
		"parallel": func(request *modelinvoker.Request) {
			request.ParallelToolCalls = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			digests := make(map[core.Digest]struct{}, 3)
			for _, value := range []*bool{nil, boolV1(false), boolV1(true)} {
				request := goldenRequestV1()
				apply(&request)
				if name == "strict" {
					request.Tools[0].Strict = value
				} else {
					request.ParallelToolCalls = value
				}
				digests[mustDigestV1(t, request)] = struct{}{}
			}
			if len(digests) != 3 {
				t.Fatalf("%s tri-state collapsed: %d digests", name, len(digests))
			}
		})
	}
}

func TestPreparedProviderInjectionShapeV1UnifiesNilAndEmptyToolsAndOptions(t *testing.T) {
	first := modelinvoker.Request{
		Provider:   "openai",
		Protocol:   modelinvoker.ProtocolResponses,
		Tools:      nil,
		ToolChoice: modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceAuto},
	}
	second := first
	second.Tools = []modelinvoker.Tool{}
	second.ProviderOptions = modelinvoker.ProviderOptions{}
	if left, right := mustDigestV1(t, first), mustDigestV1(t, second); left != right {
		t.Fatalf("nil and empty collections differ: %q != %q", left, right)
	}
	shape, err := modelinvoker.BuildPreparedProviderInjectionShapeV1(first)
	if err != nil {
		t.Fatal(err)
	}
	if shape.OrderedTools == nil || len(shape.OrderedTools) != 0 || shape.ProviderOptions.Present || shape.ProviderOptions.CanonicalJSON != nil {
		t.Fatalf("nil collections were not normalized: %#v", shape)
	}

	present := first
	present.ProviderOptions = modelinvoker.ProviderOptions{"openai": json.RawMessage(`{}`)}
	if mustDigestV1(t, first) == mustDigestV1(t, present) {
		t.Fatal("absent provider options collapsed with present empty object")
	}
}

func TestPreparedProviderInjectionShapeV1ExcludesNonInjectionFields(t *testing.T) {
	base := goldenRequestV1()
	changed := goldenRequestV1()
	changed.Endpoint = "https://secret.invalid"
	changed.Model = "other-model"
	changed.Input = []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "different input")}
	changed.Instructions = []modelinvoker.Instruction{{Role: modelinvoker.RoleDeveloper, Text: "different instructions"}}
	changed.Output = modelinvoker.OutputConstraint{Type: modelinvoker.OutputJSONObject}
	changed.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
	changed.State = &modelinvoker.State{Kind: modelinvoker.StateServerContinuation, Provider: "another-provider", Protocol: modelinvoker.ProtocolMessages, ID: "state"}
	changed.Stream = true
	changed.Budget = modelinvoker.Budget{MaxOutputTokens: 999, Timeout: time.Hour}
	changed.Metadata = modelinvoker.Metadata{"secret": "excluded"}
	changed.AllowDegradation = true
	if left, right := mustDigestV1(t, base), mustDigestV1(t, changed); left != right {
		t.Fatalf("excluded fields changed injection digest: %q != %q", left, right)
	}
}

func TestPreparedProviderInjectionShapeV1DetachesRawAliases(t *testing.T) {
	request := goldenRequestV1()
	shape, err := modelinvoker.BuildPreparedProviderInjectionShapeV1(request)
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(shape)
	if err != nil {
		t.Fatal(err)
	}

	request.Tools[0].Name = "changed"
	request.Tools[0].Description = "changed"
	request.Tools[0].Parameters[0] = '['
	*request.Tools[0].Strict = true
	request.ProviderOptions["openai"][0] = '['

	after, err := json.Marshal(shape)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("built shape retained caller alias:\nbefore=%s\nafter=%s", before, after)
	}
}

func goldenRequestV1() modelinvoker.Request {
	return modelinvoker.Request{
		Provider: "openai",
		Protocol: modelinvoker.ProtocolResponses,
		Tools: []modelinvoker.Tool{
			{
				Name:        "workspace.read",
				Description: "read one bounded file",
				Parameters:  json.RawMessage(`{"type":"object","required":["path"],"properties":{"path":{"type":"string"}}}`),
				Strict:      boolV1(true),
			},
			{
				Name:        "workspace.inspect",
				Description: "inspect one bounded path",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
				Strict:      nil,
			},
		},
		ToolChoice:        modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceFunction, Name: "workspace.read"},
		ParallelToolCalls: boolV1(false),
		ProviderOptions: modelinvoker.ProviderOptions{
			"openai": json.RawMessage(`{"limit":1.0,"hosted":{"mode":"search"}}`),
		},
	}
}

func mustDigestV1(t *testing.T, request modelinvoker.Request) core.Digest {
	t.Helper()
	digest, err := modelinvoker.ComputeActualProviderInjectionDigestV1(request)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func boolV1(value bool) *bool { return &value }

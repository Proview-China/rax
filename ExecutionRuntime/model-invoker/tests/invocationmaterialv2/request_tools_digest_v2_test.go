package invocationmaterialv2_test

import (
	"encoding/json"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestDigestGovernedModelTurnRequestToolSetV2GoldenAxes(t *testing.T) {
	tests := []struct {
		name  string
		tools []modelinvoker.Tool
		want  core.Digest
	}{
		{
			name:  "base",
			tools: requestToolSetV2(),
			want:  "sha256:62f760cd871f0d3e16f166405a4a4b762ff6f9c45fd500a9590cb225beff9cba",
		},
		{
			name: "add",
			tools: append(requestToolSetV2(), strictToolV2(
				"workspace.inspect",
				"inspect one bounded path",
				`{"type":"object","required":["path"],"properties":{"path":{"type":"string"}},"additionalProperties":false}`,
			)),
			want: "sha256:a67248ca65b3c04c0cf9d813933f2e85ad754861003528d7bdbf7d72d1af24f8",
		},
		{
			name:  "remove",
			tools: requestToolSetV2()[:1],
			want:  "sha256:0b432e05ba5fcad122e592d3aa248662b98d8944570ceec5c13b67473cf73d46",
		},
		{
			name: "reorder",
			tools: func() []modelinvoker.Tool {
				tools := requestToolSetV2()
				tools[0], tools[1] = tools[1], tools[0]
				return tools
			}(),
			want: "sha256:293a97db7efe7de47feb9ca2c403bdc3fbb64caa3c95df883c9a1a8a745bc0be",
		},
		{
			name: "name",
			tools: func() []modelinvoker.Tool {
				tools := requestToolSetV2()
				tools[0].Name = "workspace.fetch"
				return tools
			}(),
			want: "sha256:bb511993c95603094186eae11a709ac20fe6e6ca04d476ca44beb2a6d01cdaa1",
		},
		{
			name: "description",
			tools: func() []modelinvoker.Tool {
				tools := requestToolSetV2()
				tools[0].Description = "read exactly one bounded file"
				return tools
			}(),
			want: "sha256:ca9279ac550f25485c7c780d489f85c90b2adb26bf1c4337c93212d1e26ae70b",
		},
		{
			name: "schema",
			tools: func() []modelinvoker.Tool {
				tools := requestToolSetV2()
				tools[0].Parameters = json.RawMessage(
					`{"type":"object","required":["path","line"],"properties":{"path":{"type":"string"},"line":{"type":"integer"}},"additionalProperties":false}`,
				)
				return tools
			}(),
			want: "sha256:c5beaf14848987ee4f48ed90aa6ae6ba908112e6a524d03fd3e52b521b7ee6d6",
		},
		{
			name: "unknown schema member remains bound",
			tools: func() []modelinvoker.Tool {
				tools := requestToolSetV2()
				tools[0].Parameters = json.RawMessage(
					`{"type":"object","required":["path"],"properties":{"path":{"type":"string"}},"additionalProperties":false,"x-owner-unknown":true}`,
				)
				return tools
			}(),
			want: "sha256:fc909f10db3207177d4ddff3702aee385991623fb5a4cc7650ca57cbc3adf8b5",
		},
		{
			name: "schema alias spelling remains bound",
			tools: func() []modelinvoker.Tool {
				tools := requestToolSetV2()
				tools[0].Parameters = json.RawMessage(
					`{"type":"object","required":["path"],"properties":{"path":{"type":"string"}},"additionalProperties":false,"definitions":{}}`,
				)
				return tools
			}(),
			want: "sha256:d00ae15343571487f02aaba4fc75314893be4fb287e71d406c4c2d6699b4799b",
		},
	}
	seen := make(map[core.Digest]string, len(tests))
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := modelinvoker.DigestGovernedModelTurnRequestToolSetV2(test.tools)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("digest = %q, want %q", got, test.want)
			}
			if prior, exists := seen[got]; exists {
				t.Fatalf("axis collided with %q", prior)
			}
			seen[got] = test.name
		})
	}
}

func TestDigestGovernedModelTurnRequestToolSetV2MatchesValidatedRouteCallEntry(t *testing.T) {
	fixture := newFixtureV2(t)
	fromRoute, err := modelinvoker.DigestGovernedModelTurnRequestToolsV2(fixture.call)
	if err != nil {
		t.Fatal(err)
	}
	fromToolSet, err := modelinvoker.DigestGovernedModelTurnRequestToolSetV2(fixture.call.Request.Tools)
	if err != nil {
		t.Fatal(err)
	}
	if fromToolSet != fromRoute {
		t.Fatalf("tool-set digest %q differs from RouteCall digest %q", fromToolSet, fromRoute)
	}
}

func TestDigestGovernedModelTurnRequestToolSetV2RejectsInvalidAxes(t *testing.T) {
	falseValue := false
	tests := []struct {
		name  string
		tools []modelinvoker.Tool
	}{
		{name: "nil", tools: nil},
		{name: "empty", tools: []modelinvoker.Tool{}},
		{
			name: "blank name",
			tools: []modelinvoker.Tool{
				strictToolV2(" \t", "blank", `{"type":"object"}`),
			},
		},
		{
			name: "strict nil",
			tools: []modelinvoker.Tool{{
				Name: "workspace.read", Parameters: json.RawMessage(`{"type":"object"}`),
			}},
		},
		{
			name: "strict false",
			tools: []modelinvoker.Tool{{
				Name: "workspace.read", Parameters: json.RawMessage(`{"type":"object"}`), Strict: &falseValue,
			}},
		},
		{
			name: "duplicate name",
			tools: []modelinvoker.Tool{
				strictToolV2("workspace.read", "first", `{"type":"object"}`),
				strictToolV2("workspace.read", "second", `{"type":"object"}`),
			},
		},
		{
			name: "schema is not object",
			tools: []modelinvoker.Tool{
				strictToolV2("workspace.read", "read", `[]`),
			},
		},
		{
			name: "schema duplicate member",
			tools: []modelinvoker.Tool{
				strictToolV2("workspace.read", "read", `{"type":"object","type":"array"}`),
			},
		},
		{
			name: "schema trailing document",
			tools: []modelinvoker.Tool{
				strictToolV2("workspace.read", "read", `{"type":"object"} {}`),
			},
		},
		{
			name: "schema unknown token",
			tools: []modelinvoker.Tool{
				strictToolV2("workspace.read", "read", `{"type":unknown}`),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if digest, err := modelinvoker.DigestGovernedModelTurnRequestToolSetV2(test.tools); err == nil {
				t.Fatalf("invalid tool set sealed as %q", digest)
			}
		})
	}
}

func TestDigestGovernedModelTurnRequestToolSetV2SealsWithoutInputAliasesOrEffects(t *testing.T) {
	strict := true
	parameters := json.RawMessage(
		`{"type":"object","required":["path"],"properties":{"path":{"type":"string"}},"additionalProperties":false}`,
	)
	tools := []modelinvoker.Tool{
		{Name: "workspace.read", Description: "read", Parameters: parameters, Strict: &strict},
		{Name: "workspace.inspect", Description: "inspect", Parameters: parameters, Strict: &strict},
	}
	providerCalls := 0
	toolExecutions := 0
	sealed, err := modelinvoker.DigestGovernedModelTurnRequestToolSetV2(tools)
	if err != nil {
		t.Fatal(err)
	}

	tools[0].Name = "workspace.changed"
	tools[0].Description = "changed"
	tools[0].Parameters[0] = '['
	*tools[0].Strict = false
	if sealed == "" {
		t.Fatal("sealed digest was cleared by caller mutation")
	}
	if providerCalls != 0 || toolExecutions != 0 {
		t.Fatalf("pure digest path caused provider=%d tool=%d effects", providerCalls, toolExecutions)
	}

	recomputed, recomputeErr := modelinvoker.DigestGovernedModelTurnRequestToolSetV2(tools)
	if recomputeErr == nil && recomputed == sealed {
		t.Fatal("mutated input unexpectedly reproduced the already-sealed digest")
	}
}

func requestToolSetV2() []modelinvoker.Tool {
	return []modelinvoker.Tool{
		strictToolV2(
			"workspace.read",
			"read one bounded file",
			`{"type":"object","required":["path"],"properties":{"path":{"type":"string"}},"additionalProperties":false}`,
		),
		strictToolV2(
			"workspace.search",
			"search one bounded workspace",
			`{"type":"object","required":["query"],"properties":{"query":{"type":"string"}},"additionalProperties":false}`,
		),
	}
}

func strictToolV2(name, description, parameters string) modelinvoker.Tool {
	strict := true
	return modelinvoker.Tool{
		Name:        name,
		Description: description,
		Parameters:  json.RawMessage(parameters),
		Strict:      &strict,
	}
}

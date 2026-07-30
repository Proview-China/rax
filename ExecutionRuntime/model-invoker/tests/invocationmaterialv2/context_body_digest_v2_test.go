package invocationmaterialv2_test

import (
	"encoding/json"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestDigestGovernedModelTurnContextBodyV2GoldenAxes(t *testing.T) {
	tests := []struct {
		name         string
		instructions []modelinvoker.Instruction
		input        []modelinvoker.InputItem
		want         core.Digest
	}{
		{
			name:         "base",
			instructions: contextInstructionsV2(),
			input:        contextInputV2(),
			want:         "sha256:010db899735f65468842d1a1a4fa295fc1927958a61b19274e633f0eea3b8b43",
		},
		{
			name: "instruction reorder",
			instructions: func() []modelinvoker.Instruction {
				instructions := contextInstructionsV2()
				instructions[0], instructions[1] = instructions[1], instructions[0]
				return instructions
			}(),
			input: contextInputV2(),
			want:  "sha256:c6fb25b6cd3d9fd94c6f1c71793e0853032ccb555e81806206647086382c9b6c",
		},
		{
			name:         "input reorder",
			instructions: contextInstructionsV2(),
			input: func() []modelinvoker.InputItem {
				input := contextInputV2()
				input[0], input[2] = input[2], input[0]
				return input
			}(),
			want: "sha256:1d1b09ce6f04eed3440ea4e399a98bf802e9b8e6dec24bcc95a8f45cbd901761",
		},
		{
			name: "instruction role",
			instructions: func() []modelinvoker.Instruction {
				instructions := contextInstructionsV2()
				instructions[0].Role = modelinvoker.RoleDeveloper
				return instructions
			}(),
			input: contextInputV2(),
			want:  "sha256:aeb00c8beebab4cb7529956300adea8723db9e75aa817c862b782f24c1cd32fc",
		},
		{
			name: "instruction text",
			instructions: func() []modelinvoker.Instruction {
				instructions := contextInstructionsV2()
				instructions[0].Text = "follow the exact governed contract"
				return instructions
			}(),
			input: contextInputV2(),
			want:  "sha256:5eb77e504c475f9fcd3d7456cb0d79514facc8d68a64147b87606827286011f1",
		},
		{
			name:         "message role",
			instructions: contextInstructionsV2(),
			input: func() []modelinvoker.InputItem {
				input := contextInputV2()
				input[0].Message.Role = modelinvoker.RoleAssistant
				return input
			}(),
			want: "sha256:d2d5beb0d63ee0c45439be34646b748b6649128a7834e1f1af422a9186f438af",
		},
		{
			name:         "message text",
			instructions: contextInstructionsV2(),
			input: func() []modelinvoker.InputItem {
				input := contextInputV2()
				input[0].Message.Text = "inspect README"
				return input
			}(),
			want: "sha256:46376e7962fcae0bb53718c26d7d7afd44c32d0ba4b1e6e7e6a4e67f2183f059",
		},
		{
			name:         "function call id",
			instructions: contextInstructionsV2(),
			input: func() []modelinvoker.InputItem {
				input := contextInputV2()
				input[1].FunctionCall.ID = "call-2"
				return input
			}(),
			want: "sha256:898d1c9e82ac9f40444e9a103a5705e5f31771b4b00fdb6f9754568f74feb27d",
		},
		{
			name:         "function call name",
			instructions: contextInstructionsV2(),
			input: func() []modelinvoker.InputItem {
				input := contextInputV2()
				input[1].FunctionCall.Name = "workspace.inspect"
				return input
			}(),
			want: "sha256:9a4446507b4cc76c983b6903ff2955d079c98ecd84bbd630a8824ab66229ea29",
		},
		{
			name:         "function call arguments",
			instructions: contextInstructionsV2(),
			input: func() []modelinvoker.InputItem {
				input := contextInputV2()
				input[1].FunctionCall.Arguments = json.RawMessage(`{"path":"AGENTS.md"}`)
				return input
			}(),
			want: "sha256:ed7cd96113ce6c68ee22a1900761b6b45e705292335b7d462bca67d9ca5fed20",
		},
		{
			name:         "function result call id",
			instructions: contextInstructionsV2(),
			input: func() []modelinvoker.InputItem {
				input := contextInputV2()
				input[2].FunctionResult.CallID = "call-2"
				return input
			}(),
			want: "sha256:9ef2dbb9b491383645fee41ae9e8c701fe5f577ec6c549aef00c3ae0e79a8588",
		},
		{
			name:         "function result name",
			instructions: contextInstructionsV2(),
			input: func() []modelinvoker.InputItem {
				input := contextInputV2()
				input[2].FunctionResult.Name = "workspace.inspect"
				return input
			}(),
			want: "sha256:7bc21eb68296a474c279f43765b1f7e32b677c49f1730a890346fc312f22a942",
		},
		{
			name:         "function result output",
			instructions: contextInstructionsV2(),
			input: func() []modelinvoker.InputItem {
				input := contextInputV2()
				input[2].FunctionResult.Output = `{"content":"changed"}`
				return input
			}(),
			want: "sha256:96d44eb304ca0fefba09240e2a7370f01cc24a3d6338f8bea694e4f36159a8ce",
		},
		{
			name:         "function result is error",
			instructions: contextInstructionsV2(),
			input: func() []modelinvoker.InputItem {
				input := contextInputV2()
				input[2].FunctionResult.IsError = true
				return input
			}(),
			want: "sha256:0705c8eb7ebb25c0925f562718c1e40ed8cb6850484c20ad3f96dc11e9e4b100",
		},
		{
			name:         "nil instructions",
			instructions: nil,
			input:        contextInputV2(),
			want:         "sha256:3dc276094cc021e26d7b5255776ce08513d5f1aa1946aec91a5541ad99dcb7d6",
		},
		{
			name:         "empty instructions",
			instructions: []modelinvoker.Instruction{},
			input:        contextInputV2(),
			want:         "sha256:b4b93bd94cb1c0ff40f29953420e9ca815d6baaa35f2f22316061101cfb7d464",
		},
		{
			name:         "duplicate input remains ordered and bound",
			instructions: contextInstructionsV2(),
			input: append(
				contextInputV2(),
				modelinvoker.MessageInput(modelinvoker.RoleUser, "read README"),
			),
			want: "sha256:ec5b58a8519320c276bb4bff8ea4950dc33fbebbe8f7ccfa90ed94437fd7ba55",
		},
		{
			name:         "unknown argument member remains bound",
			instructions: contextInstructionsV2(),
			input: func() []modelinvoker.InputItem {
				input := contextInputV2()
				input[1].FunctionCall.Arguments = json.RawMessage(
					`{"path":"README.md","x-owner-unknown":true}`,
				)
				return input
			}(),
			want: "sha256:9f6583ba5b11f7de739e115dfd10e3cc9e3f79745fc1eb886f057cc13bcbaf35",
		},
	}
	seen := make(map[core.Digest]string, len(tests))
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := modelinvoker.DigestGovernedModelTurnContextBodyV2(
				test.instructions,
				test.input,
			)
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

func TestDigestGovernedModelTurnContextBodyV2MatchesValidatedRouteCallEntry(t *testing.T) {
	fixture := newFixtureV2(t)
	fromRoute, err := modelinvoker.DigestGovernedModelTurnContextV2(fixture.call)
	if err != nil {
		t.Fatal(err)
	}
	fromBody, err := modelinvoker.DigestGovernedModelTurnContextBodyV2(
		fixture.call.Request.Instructions,
		fixture.call.Request.Input,
	)
	if err != nil {
		t.Fatal(err)
	}
	if fromBody != fromRoute {
		t.Fatalf("context-body digest %q differs from RouteCall digest %q", fromBody, fromRoute)
	}
}

func TestDigestGovernedModelTurnContextBodyV2RejectsInvalidAxes(t *testing.T) {
	tests := []struct {
		name         string
		instructions []modelinvoker.Instruction
		input        []modelinvoker.InputItem
	}{
		{name: "nil input", input: nil},
		{name: "empty input", input: []modelinvoker.InputItem{}},
		{
			name: "instruction role",
			instructions: []modelinvoker.Instruction{{
				Role: modelinvoker.RoleUser, Text: "not an instruction role",
			}},
			input: contextInputV2(),
		},
		{
			name: "instruction text",
			instructions: []modelinvoker.Instruction{{
				Role: modelinvoker.RoleSystem, Text: " \t",
			}},
			input: contextInputV2(),
		},
		{
			name:  "unknown input type",
			input: []modelinvoker.InputItem{{Type: modelinvoker.InputType("unknown")}},
		},
		{
			name: "nil input union",
			input: []modelinvoker.InputItem{{
				Type: modelinvoker.InputTypeMessage,
			}},
		},
		{
			name: "aliased input union members",
			input: []modelinvoker.InputItem{{
				Type:         modelinvoker.InputTypeMessage,
				Message:      &modelinvoker.Message{Role: modelinvoker.RoleUser, Text: "read"},
				FunctionCall: &modelinvoker.FunctionCall{Name: "workspace.read", Arguments: json.RawMessage(`{}`)},
			}},
		},
		{
			name: "message role",
			input: []modelinvoker.InputItem{
				modelinvoker.MessageInput(modelinvoker.Role("unknown"), "read"),
			},
		},
		{
			name: "message text",
			input: []modelinvoker.InputItem{
				modelinvoker.MessageInput(modelinvoker.RoleUser, " \t"),
			},
		},
		{
			name: "function call name",
			input: []modelinvoker.InputItem{
				modelinvoker.FunctionCallInput("call-1", " \t", json.RawMessage(`{}`)),
			},
		},
		{
			name: "function call arguments are not object",
			input: []modelinvoker.InputItem{
				modelinvoker.FunctionCallInput("call-1", "workspace.read", json.RawMessage(`[]`)),
			},
		},
		{
			name: "function call arguments duplicate member",
			input: []modelinvoker.InputItem{
				modelinvoker.FunctionCallInput(
					"call-1",
					"workspace.read",
					json.RawMessage(`{"path":"README.md","path":"AGENTS.md"}`),
				),
			},
		},
		{
			name: "function call arguments trailing document",
			input: []modelinvoker.InputItem{
				modelinvoker.FunctionCallInput(
					"call-1",
					"workspace.read",
					json.RawMessage(`{"path":"README.md"} {}`),
				),
			},
		},
		{
			name: "function call arguments unknown token",
			input: []modelinvoker.InputItem{
				modelinvoker.FunctionCallInput(
					"call-1",
					"workspace.read",
					json.RawMessage(`{"path":unknown}`),
				),
			},
		},
		{
			name: "function result identity",
			input: []modelinvoker.InputItem{
				modelinvoker.NamedFunctionResultInput("", "", "missing identity", false),
			},
		},
		{
			name: "function result blank optional name",
			input: []modelinvoker.InputItem{
				modelinvoker.NamedFunctionResultInput("call-1", " \t", "blank name", false),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if digest, err := modelinvoker.DigestGovernedModelTurnContextBodyV2(
				test.instructions,
				test.input,
			); err == nil {
				t.Fatalf("invalid context body sealed as %q", digest)
			}
		})
	}
}

func TestDigestGovernedModelTurnContextBodyV2SealsWithoutInputAliasesOrEffects(t *testing.T) {
	message := &modelinvoker.Message{Role: modelinvoker.RoleUser, Text: "read README"}
	arguments := json.RawMessage(`{"path":"README.md"}`)
	functionCall := &modelinvoker.FunctionCall{
		ID: "call-1", Name: "workspace.read", Arguments: arguments,
	}
	instructions := []modelinvoker.Instruction{{
		Role: modelinvoker.RoleSystem, Text: "follow the contract",
	}}
	input := []modelinvoker.InputItem{
		{Type: modelinvoker.InputTypeMessage, Message: message},
		{Type: modelinvoker.InputTypeFunctionCall, FunctionCall: functionCall},
	}
	providerCalls := 0
	contextReads := 0
	sealed, err := modelinvoker.DigestGovernedModelTurnContextBodyV2(instructions, input)
	if err != nil {
		t.Fatal(err)
	}

	instructions[0].Text = "changed"
	message.Text = "changed"
	functionCall.Name = "workspace.changed"
	arguments[0] = '['
	if sealed == "" {
		t.Fatal("sealed digest was cleared by caller mutation")
	}
	if providerCalls != 0 || contextReads != 0 {
		t.Fatalf("pure digest path caused provider=%d context=%d effects", providerCalls, contextReads)
	}

	recomputed, recomputeErr := modelinvoker.DigestGovernedModelTurnContextBodyV2(instructions, input)
	if recomputeErr == nil && recomputed == sealed {
		t.Fatal("mutated input unexpectedly reproduced the already-sealed digest")
	}
}

func contextInstructionsV2() []modelinvoker.Instruction {
	return []modelinvoker.Instruction{
		{Role: modelinvoker.RoleSystem, Text: "follow the governed contract"},
		{Role: modelinvoker.RoleDeveloper, Text: "preserve exact source bytes"},
	}
}

func contextInputV2() []modelinvoker.InputItem {
	return []modelinvoker.InputItem{
		modelinvoker.MessageInput(modelinvoker.RoleUser, "read README"),
		modelinvoker.FunctionCallInput(
			"call-1",
			"workspace.read",
			json.RawMessage(`{"path":"README.md"}`),
		),
		modelinvoker.NamedFunctionResultInput(
			"call-1",
			"workspace.read",
			`{"content":"ok"}`,
			false,
		),
	}
}

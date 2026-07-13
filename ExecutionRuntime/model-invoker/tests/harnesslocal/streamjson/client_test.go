package streamjson_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	harnessprocess "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/process"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/streamjson"
)

func TestClientCorrelatesControlAndPreservesPinnedLaunchEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	config := helperConfig(t, "normal")
	client, err := streamjson.Start(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()
	response, err := client.Call(ctx, map[string]any{"subtype": "initialize"})
	if err != nil || !json.Valid(response) || !containsJSON(response, "ready") {
		t.Fatalf("control response=%s err=%v", response, err)
	}
	message, err := client.Receive(ctx)
	if err != nil || message.Type != "future_event" {
		t.Fatalf("message=%#v err=%v", message, err)
	}
	evidence := client.Evidence()
	if !evidence.Pinned() || evidence.ActualExecutableDigest != config.ExpectedExecutableDigest {
		t.Fatalf("launch evidence is not pinned: %#v", evidence)
	}
	if evidence.WorkingDirectory != config.WorkingDirectory || len(evidence.Arguments) == 0 {
		t.Fatalf("launch evidence lost cwd/argv: %#v", evidence)
	}
}

func TestClientFailsClosedOnUnknownControlResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := streamjson.Start(ctx, helperConfig(t, "rogue-response"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = client.Receive(ctx)
	if !errors.Is(err, streamjson.ErrUnexpectedResponse) {
		t.Fatalf("receive error=%v, want ErrUnexpectedResponse", err)
	}
}

func TestProbeLaunchCommitsButDoesNotExposeEnvironmentValues(t *testing.T) {
	config := helperConfig(t, "normal")
	config.Environment = map[string]string{"LANG": "C.UTF-8"}
	config.AllowedEnvironment = []string{"LANG"}
	evidence, err := streamjson.ProbeLaunch(config)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(evidence)
	if string(encoded) == "" || containsJSON(encoded, "C.UTF-8") {
		t.Fatalf("evidence exposed environment value: %s", encoded)
	}
	if len(evidence.EnvironmentNames) != 1 || evidence.EnvironmentNames[0] != "LANG" || evidence.EnvironmentDigest == "" {
		t.Fatalf("environment evidence=%#v", evidence)
	}
}

func helperConfig(t *testing.T, mode string) harnessprocess.Config {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	directory := t.TempDir()
	return harnessprocess.Config{
		Executable: executable, ExpectedExecutableDigest: digest,
		Arguments:        []string{"-test.run=^TestStreamJSONFakeProcess$", "--", mode},
		WorkingDirectory: directory, AllowedWorkingDirectories: []string{directory},
		Protocol: harnessprocess.ProtocolJSONL, TerminationGrace: 100 * time.Millisecond, KillWait: time.Second,
	}
}

func TestStreamJSONFakeProcess(t *testing.T) {
	mode := helperMode(os.Args)
	if mode == "" {
		return
	}
	os.Exit(runFake(mode, os.Stdin, os.Stdout))
}

func runFake(mode string, input io.Reader, output io.Writer) int {
	encoder := json.NewEncoder(output)
	if mode == "rogue-response" {
		_ = encoder.Encode(map[string]any{
			"type":     "control_response",
			"response": map[string]any{"subtype": "success", "request_id": "rogue", "response": map[string]any{}},
		})
		return 0
	}
	scanner := bufio.NewScanner(input)
	if !scanner.Scan() {
		return 2
	}
	var request struct {
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
	}
	if json.Unmarshal(scanner.Bytes(), &request) != nil || request.Type != "control_request" || request.RequestID == "" {
		return 3
	}
	if err := encoder.Encode(map[string]any{
		"type":     "control_response",
		"response": map[string]any{"subtype": "success", "request_id": request.RequestID, "response": map[string]any{"ready": true}},
	}); err != nil {
		return 4
	}
	if err := encoder.Encode(map[string]any{"type": "future_event", "subtype": "v2", "payload": map[string]any{"x": 1}}); err != nil {
		return 5
	}
	for scanner.Scan() {
	}
	return 0
}

func helperMode(arguments []string) string {
	for index, argument := range arguments {
		if argument == "--" && index+1 < len(arguments) {
			return arguments[index+1]
		}
	}
	return ""
}

func containsJSON(raw []byte, value string) bool {
	return len(raw) != 0 && string(raw) != "" && json.Valid(raw) && contains(string(raw), value)
}

func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

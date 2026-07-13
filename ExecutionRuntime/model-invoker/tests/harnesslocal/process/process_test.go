package process_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	harnessprocess "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/process"
)

func TestExplicitExecutableCWDEnvironmentAndNoShell(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, "must-not-exist")
	literal := "; touch " + marker
	config := helperConfig(t, directory, "inspect", literal)
	config.Environment = map[string]string{"SAFE_VALUE": "visible"}
	config.AllowedEnvironment = []string{"SAFE_VALUE"}
	session := startSession(t, context.Background(), config)

	frame, err := session.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		CWD   string   `json:"cwd"`
		Env   string   `json:"env"`
		Home  string   `json:"home"`
		Extra []string `json:"extra"`
	}
	if err := json.Unmarshal(frame.Raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.CWD != directory || report.Env != "visible" || report.Home != "" || len(report.Extra) != 1 || report.Extra[0] != literal {
		t.Fatalf("helper report = %#v", report)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("argument was interpreted by a shell: stat error = %v", err)
	}
	result, err := session.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if result.PID <= 0 || result.ActualExecutablePath == "" || !strings.HasPrefix(result.ActualExecutableDigest, "sha256:") || result.ExitCode != 0 || result.Signal != "" || !result.Quiesced {
		t.Fatalf("process evidence = %#v", result)
	}
}

func TestConfigRejectsUncontrolledInputs(t *testing.T) {
	executable := helperExecutable(t)
	directory := t.TempDir()
	otherDirectory := t.TempDir()
	base := harnessprocess.Config{
		Executable: executable, WorkingDirectory: directory, AllowedWorkingDirectories: []string{directory},
		Protocol: harnessprocess.ProtocolJSONL,
	}
	tests := []struct {
		name   string
		mutate func(*harnessprocess.Config)
		want   error
	}{
		{name: "PATH lookup", mutate: func(config *harnessprocess.Config) { config.Executable = filepath.Base(executable) }, want: harnessprocess.ErrExecutableNotAbsolute},
		{name: "cwd outside roots", mutate: func(config *harnessprocess.Config) { config.AllowedWorkingDirectories = []string{otherDirectory} }, want: harnessprocess.ErrWorkingDirectoryNotAllowed},
		{name: "environment not allowed", mutate: func(config *harnessprocess.Config) { config.Environment = map[string]string{"SAFE_VALUE": "x"} }, want: harnessprocess.ErrEnvironmentNotAllowed},
		{name: "sensitive environment", mutate: func(config *harnessprocess.Config) {
			config.Environment = map[string]string{"OPENAI_API_KEY": "not-a-real-key"}
			config.AllowedEnvironment = []string{"OPENAI_API_KEY"}
		}, want: harnessprocess.ErrSensitiveEnvironment},
		{name: "loader environment", mutate: func(config *harnessprocess.Config) {
			config.Environment = map[string]string{"LD_PRELOAD": "/tmp/not-real"}
			config.AllowedEnvironment = []string{"LD_PRELOAD"}
		}, want: harnessprocess.ErrUnsafeEnvironment},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := base
			test.mutate(&config)
			session, err := harnessprocess.Start(context.Background(), config)
			if session != nil {
				_ = session.Close()
				t.Fatal("invalid config started a process")
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("Start() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestExecutableDigestPinAndEvidence(t *testing.T) {
	executable := helperExecutable(t)
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	directory := t.TempDir()
	config := helperConfig(t, directory, "inspect")
	config.ExpectedExecutableDigest = digest
	session := startSession(t, context.Background(), config)
	if _, err := session.ReadFrame(); err != nil {
		t.Fatal(err)
	}
	result, err := session.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if result.ActualExecutableDigest != digest {
		t.Fatalf("actual digest = %q, want %q", result.ActualExecutableDigest, digest)
	}

	config.ExpectedExecutableDigest = "sha256:" + strings.Repeat("0", 64)
	if session, err := harnessprocess.Start(context.Background(), config); session != nil || !errors.Is(err, harnessprocess.ErrExecutableDigestMismatch) {
		if session != nil {
			_ = session.Close()
		}
		t.Fatalf("digest mismatch Start() = %v, %v", session, err)
	}
}

func TestJSONLRoundTripAndInputClose(t *testing.T) {
	directory := t.TempDir()
	session := startSession(t, context.Background(), helperConfig(t, directory, "echo-jsonl"))
	want := []byte(`{"kind":"ping","value":1}`)
	if err := session.WriteFrame(want); err != nil {
		t.Fatal(err)
	}
	if err := session.CloseInput(); err != nil {
		t.Fatal(err)
	}
	if err := session.CloseInput(); err != nil {
		t.Fatalf("second CloseInput() = %v", err)
	}
	frame, err := session.ReadFrame()
	if err != nil || string(frame.Raw) != string(want) || frame.RPC != nil {
		t.Fatalf("ReadFrame() = %s, %#v, %v", frame.Raw, frame.RPC, err)
	}
	if _, err := session.ReadFrame(); !errors.Is(err, io.EOF) {
		t.Fatalf("terminal ReadFrame() error = %v", err)
	}
	result, err := session.Wait()
	if err != nil || result.StdoutBytes == 0 || result.StdoutTruncated || result.StderrTruncated {
		t.Fatalf("Wait() = %#v, %v", result, err)
	}
}

func TestJSONRPCNDJSONCorrelationAndDuplicateResponse(t *testing.T) {
	t.Run("correlated response", func(t *testing.T) {
		directory := t.TempDir()
		config := helperConfig(t, directory, "rpc-echo")
		config.Protocol = harnessprocess.ProtocolJSONRPCNDJSON
		session := startSession(t, context.Background(), config)
		request := []byte(`{"jsonrpc":"2.0","id":"r1","method":"initialize","params":{}}`)
		if err := session.WriteFrame(request); err != nil {
			t.Fatal(err)
		}
		frame, err := session.ReadFrame()
		if err != nil || frame.RPC == nil || frame.RPC.Kind != harnessprocess.JSONRPCResponse || string(frame.RPC.ID) != `"r1"` {
			t.Fatalf("response = %#v, %v", frame, err)
		}
		if _, err := session.Wait(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("duplicate response", func(t *testing.T) {
		directory := t.TempDir()
		config := helperConfig(t, directory, "rpc-duplicate")
		config.Protocol = harnessprocess.ProtocolJSONRPCNDJSON
		session := startSession(t, context.Background(), config)
		request := []byte(`{"jsonrpc":"2.0","id":7,"method":"run"}`)
		if err := session.WriteFrame(request); err != nil {
			t.Fatal(err)
		}
		if _, err := session.ReadFrame(); err != nil {
			t.Fatal(err)
		}
		if _, err := session.ReadFrame(); !errors.Is(err, harnessprocess.ErrDuplicateResponseID) {
			t.Fatalf("second response error = %v", err)
		}
		if _, err := session.Wait(); !errors.Is(err, harnessprocess.ErrDuplicateResponseID) {
			t.Fatalf("Wait() error = %v", err)
		}
	})

	t.Run("unknown response", func(t *testing.T) {
		directory := t.TempDir()
		config := helperConfig(t, directory, "rpc-unknown")
		config.Protocol = harnessprocess.ProtocolJSONRPCNDJSON
		session := startSession(t, context.Background(), config)
		if _, err := session.ReadFrame(); !errors.Is(err, harnessprocess.ErrUnknownResponseID) {
			t.Fatalf("ReadFrame() error = %v", err)
		}
	})

	t.Run("reverse request and response", func(t *testing.T) {
		directory := t.TempDir()
		config := helperConfig(t, directory, "rpc-reverse")
		config.Protocol = harnessprocess.ProtocolJSONRPCNDJSON
		session := startSession(t, context.Background(), config)
		frame, err := session.ReadFrame()
		if err != nil || frame.RPC == nil || frame.RPC.Kind != harnessprocess.JSONRPCRequest || frame.RPC.Method != "fs/read_text_file" {
			t.Fatalf("reverse request = %#v, %v", frame, err)
		}
		response := []byte(`{"jsonrpc":"2.0","id":"server-1","result":{"content":"ok"}}`)
		if err := session.WriteFrame(response); err != nil {
			t.Fatal(err)
		}
		if err := session.WriteFrame(response); !errors.Is(err, harnessprocess.ErrDuplicateResponseID) {
			t.Fatalf("duplicate reverse response error = %v", err)
		}
		if err := session.CloseInput(); err != nil {
			t.Fatal(err)
		}
		if _, err := session.Wait(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCodexAppServerRPCDialectIsVersionlessWithoutWeakeningJSONRPC(t *testing.T) {
	directory := t.TempDir()
	config := helperConfig(t, directory, "codex-rpc-echo")
	config.Protocol = harnessprocess.ProtocolCodexAppServer
	session := startSession(t, context.Background(), config)
	if err := session.WriteFrame([]byte(`{"jsonrpc":"1.0","id":2,"method":"invalid"}`)); !errors.Is(err, harnessprocess.ErrInvalidJSONRPC) {
		t.Fatalf("Codex dialect accepted an invalid explicit version: %v", err)
	}
	request := []byte(`{"id":1,"method":"initialize","params":{}}`)
	if err := session.WriteFrame(request); err != nil {
		t.Fatal(err)
	}
	frame, err := session.ReadFrame()
	if err != nil || frame.RPC == nil || frame.RPC.Kind != harnessprocess.JSONRPCResponse || string(frame.RPC.ID) != "1" {
		t.Fatalf("Codex response = %#v, %v", frame, err)
	}
	if bytes.Contains(frame.Raw, []byte(`"jsonrpc"`)) {
		t.Fatalf("Codex response unexpectedly declares JSON-RPC version: %s", frame.Raw)
	}
	if _, err := session.Wait(); err != nil {
		t.Fatal(err)
	}

	strict := helperConfig(t, t.TempDir(), "codex-rpc-echo")
	strict.Protocol = harnessprocess.ProtocolJSONRPCNDJSON
	strictSession := startSession(t, context.Background(), strict)
	if err := strictSession.WriteFrame([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := strictSession.ReadFrame(); !errors.Is(err, harnessprocess.ErrInvalidJSONRPC) {
		t.Fatalf("strict JSON-RPC accepted versionless Codex response: %v", err)
	}
}

func TestFramingRejectsPartialOversizedInvalidUTF8AndInvalidOutbound(t *testing.T) {
	tests := []struct {
		name string
		mode string
		max  int
		want error
	}{
		{name: "partial", mode: "partial", max: 64, want: harnessprocess.ErrPartialFrame},
		{name: "oversized", mode: "oversized", max: 32, want: harnessprocess.ErrFrameTooLarge},
		{name: "invalid UTF-8", mode: "invalid-utf8", max: 64, want: harnessprocess.ErrInvalidUTF8},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			config := helperConfig(t, directory, test.mode)
			config.MaxFrameBytes = test.max
			session := startSession(t, context.Background(), config)
			if _, err := session.ReadFrame(); !errors.Is(err, test.want) {
				t.Fatalf("ReadFrame() error = %v, want %v", err, test.want)
			}
			if _, err := session.Wait(); !errors.Is(err, test.want) {
				t.Fatalf("Wait() error = %v, want %v", err, test.want)
			}
		})
	}

	directory := t.TempDir()
	session := startSession(t, context.Background(), helperConfig(t, directory, "echo-jsonl"))
	for _, invalid := range [][]byte{{0xff}, []byte("{}\n{}"), []byte("not-json")} {
		if err := session.WriteFrame(invalid); err == nil {
			t.Fatalf("WriteFrame(%q) succeeded", invalid)
		}
	}
	_ = session.Close()

	directory = t.TempDir()
	config := helperConfig(t, directory, "echo-jsonl")
	config.MaxFrameBytes = 16
	session = startSession(t, context.Background(), config)
	if err := session.WriteFrame([]byte(`{"payload":"` + strings.Repeat("x", 32) + `"}`)); !errors.Is(err, harnessprocess.ErrFrameTooLarge) {
		t.Fatalf("oversized outbound error = %v", err)
	}
	_ = session.Close()
}

func TestOutputLimitsAndExitEvidence(t *testing.T) {
	t.Run("stdout", func(t *testing.T) {
		directory := t.TempDir()
		config := helperConfig(t, directory, "stdout-flood")
		config.MaxFrameBytes = 64
		config.MaxStdoutBytes = 128
		session := startSession(t, context.Background(), config)
		for {
			if _, err := session.ReadFrame(); err != nil {
				if !errors.Is(err, harnessprocess.ErrStdoutLimit) {
					t.Fatalf("ReadFrame() error = %v", err)
				}
				break
			}
		}
		result, err := session.Wait()
		if !errors.Is(err, harnessprocess.ErrStdoutLimit) || !result.StdoutTruncated || result.StdoutBytes != 129 || !result.Quiesced {
			t.Fatalf("Wait() = %#v, %v", result, err)
		}
	})

	t.Run("stderr", func(t *testing.T) {
		directory := t.TempDir()
		config := helperConfig(t, directory, "stderr-flood")
		config.MaxStderrBytes = 32
		session := startSession(t, context.Background(), config)
		result, err := session.Wait()
		if !errors.Is(err, harnessprocess.ErrStderrLimit) || !result.StderrTruncated || len(result.Stderr) != 32 || result.StderrBytes <= 32 || !result.Quiesced {
			t.Fatalf("Wait() = %#v, %v", result, err)
		}
	})

	t.Run("nonzero exit", func(t *testing.T) {
		directory := t.TempDir()
		session := startSession(t, context.Background(), helperConfig(t, directory, "exit", "7"))
		result, err := session.Wait()
		if !errors.Is(err, harnessprocess.ErrProcessExit) || result.ExitCode != 7 || result.Signal != "" || !result.Quiesced {
			t.Fatalf("Wait() = %#v, %v", result, err)
		}
	})
}

func TestCancellationUsesSIGTERMThenBoundedSIGKILL(t *testing.T) {
	t.Run("cooperative SIGTERM", func(t *testing.T) {
		directory := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		config := helperConfig(t, directory, "term-exit")
		config.TerminationGrace = 2 * time.Second
		session := startSession(t, ctx, config)
		waitReady(t, session)
		cancel()
		result, err := session.Wait()
		if !errors.Is(err, context.Canceled) || result.Killed || !result.TerminationRequested || !result.Quiesced {
			t.Fatalf("Wait() = %#v, %v", result, err)
		}
	})

	t.Run("SIGKILL escalation", func(t *testing.T) {
		directory := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		config := helperConfig(t, directory, "ignore-term")
		config.TerminationGrace = 60 * time.Millisecond
		config.KillWait = time.Second
		session := startSession(t, ctx, config)
		waitReady(t, session)
		cancel()
		result, err := session.Wait()
		if !errors.Is(err, context.Canceled) || !result.Killed || result.Signal != "killed" || !result.Quiesced {
			t.Fatalf("Wait() = %#v, %v", result, err)
		}
	})
}

func TestProcessGroupQuiescenceKillsDescendant(t *testing.T) {
	directory := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	config := helperConfig(t, directory, "spawn-descendant")
	config.TerminationGrace = 80 * time.Millisecond
	config.KillWait = 2 * time.Second
	session := startSession(t, ctx, config)
	frame, err := session.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	var ready struct {
		PID int `json:"child_pid"`
	}
	if err := json.Unmarshal(frame.Raw, &ready); err != nil || ready.PID <= 0 {
		t.Fatalf("descendant frame = %s, %v", frame.Raw, err)
	}
	cancel()
	result, err := session.Wait()
	if !errors.Is(err, context.Canceled) || !result.Killed || !result.Quiesced {
		t.Fatalf("Wait() = %#v, %v", result, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := syscall.Kill(ready.PID, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("descendant %d still exists: %v", ready.PID, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNaturalLeaderExitReportsAndCleansProcessGroupLeak(t *testing.T) {
	directory := t.TempDir()
	config := helperConfig(t, directory, "orphan-descendant")
	config.TerminationGrace = 60 * time.Millisecond
	config.KillWait = 2 * time.Second
	session := startSession(t, context.Background(), config)
	frame, err := session.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	var ready struct {
		PID int `json:"child_pid"`
	}
	if err := json.Unmarshal(frame.Raw, &ready); err != nil || ready.PID <= 0 {
		t.Fatalf("descendant frame = %s, %v", frame.Raw, err)
	}
	result, err := session.Wait()
	if !errors.Is(err, harnessprocess.ErrProcessGroupLeak) || !result.Killed || !result.Quiesced {
		t.Fatalf("Wait() = %#v, %v", result, err)
	}
	waitProcessGone(t, ready.PID)
}

func TestCloseIsIdempotent(t *testing.T) {
	directory := t.TempDir()
	config := helperConfig(t, directory, "ignore-term")
	config.TerminationGrace = 40 * time.Millisecond
	session := startSession(t, context.Background(), config)
	waitReady(t, session)
	if err := session.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
	result, err := session.Wait()
	if !errors.Is(err, harnessprocess.ErrClosed) || !result.Killed || !result.Quiesced {
		t.Fatalf("Wait() = %#v, %v", result, err)
	}
}

func helperConfig(t *testing.T, directory, mode string, arguments ...string) harnessprocess.Config {
	t.Helper()
	return harnessprocess.Config{
		Executable:       helperExecutable(t),
		Arguments:        append([]string{"-test.run=^TestHarnessProcessHelper$", "--", mode}, arguments...),
		WorkingDirectory: directory, AllowedWorkingDirectories: []string{directory},
		Protocol:         harnessprocess.ProtocolJSONL,
		TerminationGrace: 100 * time.Millisecond, KillWait: time.Second,
	}
}

func helperExecutable(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		t.Fatal(err)
	}
	return executable
}

func startSession(t *testing.T, ctx context.Context, config harnessprocess.Config) *harnessprocess.Session {
	t.Helper()
	session, err := harnessprocess.Start(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func waitReady(t *testing.T, session *harnessprocess.Session) {
	t.Helper()
	frame, err := session.ReadFrame()
	if err != nil || string(frame.Raw) != `{"ready":true}` {
		t.Fatalf("ready frame = %s, %v", frame.Raw, err)
	}
}

func waitProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d still exists: %v", pid, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestHarnessProcessHelper is re-executed as the only fake executable used by
// this package. In the parent test process there is no "--" marker, so it is a
// no-op and cannot inspect real CLI state or credentials.
func TestHarnessProcessHelper(t *testing.T) {
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		return
	}
	os.Exit(runHelper(os.Args[separator+1:]))
}

func runHelper(arguments []string) int {
	mode := arguments[0]
	extra := arguments[1:]
	switch mode {
	case "inspect":
		cwd, _ := os.Getwd()
		writeHelperJSON(map[string]any{"cwd": cwd, "env": os.Getenv("SAFE_VALUE"), "home": os.Getenv("HOME"), "extra": extra})
	case "echo-jsonl":
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return 2
		}
		_, _ = io.WriteString(os.Stdout, line)
	case "rpc-echo", "rpc-duplicate", "codex-rpc-echo":
		line, err := bufio.NewReader(os.Stdin).ReadBytes('\n')
		if err != nil {
			return 2
		}
		var request map[string]json.RawMessage
		if json.Unmarshal(line, &request) != nil {
			return 2
		}
		responseObject := map[string]any{"id": request["id"], "result": map[string]any{"ok": true}}
		if mode != "codex-rpc-echo" {
			responseObject["jsonrpc"] = "2.0"
		}
		response, _ := json.Marshal(responseObject)
		_, _ = os.Stdout.Write(append(response, '\n'))
		if mode == "rpc-duplicate" {
			_, _ = os.Stdout.Write(append(response, '\n'))
		}
	case "rpc-unknown":
		_, _ = io.WriteString(os.Stdout, `{"jsonrpc":"2.0","id":"unknown","result":true}`+"\n")
	case "rpc-reverse":
		_, _ = io.WriteString(os.Stdout, `{"jsonrpc":"2.0","id":"server-1","method":"fs/read_text_file","params":{"path":"a.txt"}}`+"\n")
		line, err := bufio.NewReader(os.Stdin).ReadBytes('\n')
		if err != nil {
			return 2
		}
		var response map[string]json.RawMessage
		if json.Unmarshal(line, &response) != nil || string(response["id"]) != `"server-1"` {
			return 2
		}
		_, _ = io.Copy(io.Discard, os.Stdin)
	case "partial":
		_, _ = io.WriteString(os.Stdout, `{"partial":true}`)
	case "oversized":
		_, _ = io.WriteString(os.Stdout, `{"payload":"`+strings.Repeat("x", 256)+`"}`+"\n")
	case "invalid-utf8":
		_, _ = os.Stdout.Write([]byte{0xff, '\n'})
	case "stdout-flood":
		for index := 0; index < 100; index++ {
			_, _ = fmt.Fprintf(os.Stdout, "{\"index\":%d}\n", index)
		}
	case "stderr-flood":
		_, _ = io.WriteString(os.Stderr, strings.Repeat("e", 4096))
	case "exit":
		code, _ := strconv.Atoi(extra[0])
		return code
	case "term-exit":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM)
		writeHelperJSON(map[string]any{"ready": true})
		<-signals
		return 0
	case "ignore-term":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM)
		writeHelperJSON(map[string]any{"ready": true})
		for {
			<-signals
		}
	case "spawn-descendant", "orphan-descendant":
		command := exec.Command(os.Args[0], "-test.run=^TestHarnessProcessHelper$", "--", "ignore-term")
		stdout, err := command.StdoutPipe()
		if err != nil || command.Start() != nil {
			return 2
		}
		if _, err := bufio.NewReader(stdout).ReadString('\n'); err != nil {
			return 2
		}
		writeHelperJSON(map[string]any{"child_pid": command.Process.Pid})
		if mode == "orphan-descendant" {
			return 0
		}
		select {}
	default:
		return 2
	}
	return 0
}

func writeHelperJSON(value any) {
	data, _ := json.Marshal(value)
	_, _ = os.Stdout.Write(append(data, '\n'))
}

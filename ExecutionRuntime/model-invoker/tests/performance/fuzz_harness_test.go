package performance_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	harnessprocess "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/process"
)

func FuzzHarnessFrame(f *testing.F) {
	f.Add([]byte(`{"kind":"seed","value":1}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"nested":[true,false,null]}`))
	f.Add([]byte{0xff})

	executable, err := os.Executable()
	if err != nil {
		f.Fatal(err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		f.Fatal(err)
	}
	directory := f.TempDir()
	session, err := harnessprocess.Start(context.Background(), harnessprocess.Config{
		Executable:       executable,
		Arguments:        []string{"-test.run=^TestHarnessFrameHelper$", "--", "echo-loop"},
		WorkingDirectory: directory, AllowedWorkingDirectories: []string{directory},
		Protocol: harnessprocess.ProtocolJSONL, MaxFrameBytes: 4096,
		// The session is intentionally shared for the lifetime of one fuzz
		// worker and every accepted frame is read synchronously. Output-limit
		// behavior has dedicated process tests, so use a practically unreachable
		// cumulative budget here; otherwise fuzz throughput eventually exhausts
		// the budget and looks like a framing context-deadline failure.
		MaxStdoutBytes: 1 << 62, MaxStderrBytes: 1 << 20,
		TerminationGrace: 100 * time.Millisecond, KillWait: time.Second,
	})
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() {
		_ = session.CloseInput()
		_, _ = session.Wait()
	})

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 8192 {
			raw = raw[:8192]
		}
		if err := session.WriteFrame(raw); err != nil {
			return
		}
		frame, err := session.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame after accepted WriteFrame: %v", err)
		}
		if frame.RPC != nil || !bytes.Equal(frame.Raw, raw) {
			t.Fatalf("framing changed accepted JSON: got %q, want %q", frame.Raw, raw)
		}
	})
}

// TestHarnessFrameHelper is the only child executable used by the framing
// fuzz target. Without the explicit "--" marker it is a no-op. The child
// inherits no environment through process.Config and never discovers a CLI or
// credential.
func TestHarnessFrameHelper(t *testing.T) {
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
	if os.Args[separator+1] != "echo-loop" {
		os.Exit(2)
	}
	os.Exit(runFrameEchoLoop(os.Stdin, os.Stdout))
}

func runFrameEchoLoop(reader io.Reader, writer io.Writer) int {
	buffered := bufio.NewReader(reader)
	for {
		line, err := buffered.ReadBytes('\n')
		if len(line) != 0 {
			if _, writeErr := writer.Write(line); writeErr != nil {
				return 2
			}
		}
		if errors.Is(err, io.EOF) {
			return 0
		}
		if err != nil {
			return 2
		}
	}
}

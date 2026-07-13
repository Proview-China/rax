package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Result is the bounded, secret-agnostic process evidence available after
// Wait. Stderr contains only the retained prefix and must still be redacted by
// the caller before publication.
type Result struct {
	ActualExecutablePath   string
	ActualExecutableDigest string
	PID                    int
	ExitCode               int
	Signal                 string
	StdoutBytes            int64
	StdoutTruncated        bool
	Stderr                 []byte
	StderrBytes            int64
	StderrTruncated        bool
	TerminationRequested   bool
	Killed                 bool
	Quiesced               bool
}

// Session owns one child process and its stdin/stdout protocol state.
type Session struct {
	config     normalizedConfig
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdoutPipe io.ReadCloser
	stderrPipe io.ReadCloser

	frames  *frameQueue
	tracker *rpcTracker
	stdout  *budgetReader
	stderr  *cappedCapture

	writeMu sync.Mutex
	readMu  sync.Mutex

	inputOnce sync.Once
	inputErr  error
	stopMu    sync.Mutex
	stopCause error
	stopCh    chan struct{}
	done      chan struct{}

	resultMu sync.RWMutex
	result   Result
	waitErr  error

	closeOnce sync.Once
	closeErr  error
}

// Start validates all process inputs before starting an explicitly addressed
// executable. It never invokes a shell or searches PATH.
func Start(ctx context.Context, config Config) (*Session, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is nil", ErrInvalidConfig)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}

	command := exec.Command(normalized.executable, normalized.arguments...)
	command.Dir = normalized.directory
	command.Env = make([]string, len(normalized.environment))
	copy(command.Env, normalized.environment)
	command.WaitDelay = normalized.terminationGrace
	configureProcessGroup(command)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create harness stdin: %w", err)
	}
	stdoutPipe, stdoutWriter := io.Pipe()
	stderrPipe, stderrWriter := io.Pipe()
	command.Stdout = stdoutWriter
	command.Stderr = stderrWriter

	session := &Session{
		config: normalized, cmd: command, stdin: stdin, stdoutPipe: stdoutPipe, stderrPipe: stderrPipe,
		frames: newFrameQueue(), tracker: newRPCTracker(),
		stopCh: make(chan struct{}, 1), done: make(chan struct{}),
	}
	session.stdout = &budgetReader{reader: stdoutPipe, limit: normalized.maxStdoutBytes, onLimit: func() { session.requestStop(ErrStdoutLimit) }}
	session.stderr = &cappedCapture{limit: normalized.maxStderrBytes, onLimit: func() { session.requestStop(ErrStderrLimit) }}

	if err := command.Start(); err != nil {
		_ = stdin.Close()
		_ = stdoutPipe.Close()
		_ = stdoutWriter.Close()
		_ = stderrPipe.Close()
		_ = stderrWriter.Close()
		return nil, fmt.Errorf("start harness executable: %w", err)
	}
	session.result = Result{
		ActualExecutablePath: normalized.executable, ActualExecutableDigest: normalized.executableDigest,
		PID: command.Process.Pid, ExitCode: -1,
	}

	stdoutDone := make(chan error, 1)
	go func() {
		err := decodeFrames(session.stdout, normalized.protocol, normalized.maxFrameBytes, session.tracker, session.frames.push)
		if errors.Is(err, io.EOF) {
			err = nil
		}
		session.frames.finish(err)
		if err != nil {
			session.requestStop(err)
			_, _ = io.Copy(io.Discard, stdoutPipe)
		}
		stdoutDone <- err
	}()
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(session.stderr, stderrPipe)
		close(stderrDone)
	}()
	waitDone := make(chan error, 1)
	go func() {
		waitErr := command.Wait()
		_ = stdoutWriter.Close()
		_ = stderrWriter.Close()
		waitDone <- waitErr
	}()
	go session.supervise(ctx, waitDone, stdoutDone, stderrDone)
	return session, nil
}

// WriteFrame validates and writes one JSONL or JSON-RPC NDJSON frame.
func (s *Session) WriteFrame(raw []byte) error {
	if s == nil {
		return ErrClosed
	}
	select {
	case <-s.done:
		return ErrClosed
	default:
	}
	if bytes.ContainsAny(raw, "\r\n") {
		return ErrInvalidJSON
	}
	message, err := validateOutboundFrame(raw, s.config.protocol, s.config.maxFrameBytes)
	if err != nil {
		return err
	}
	rollback := func() {}
	if isRPCProtocol(s.config.protocol) {
		rollback, err = s.tracker.prepareOutgoing(message)
		if err != nil {
			return err
		}
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := writeAll(s.stdin, append(append([]byte(nil), raw...), '\n')); err != nil {
		rollback()
		s.requestStop(fmt.Errorf("write harness frame: %w", err))
		return fmt.Errorf("write harness frame: %w", err)
	}
	return nil
}

// ReadFrame returns the next validated frame in native order.
func (s *Session) ReadFrame() (Frame, error) {
	if s == nil {
		return Frame{}, ErrClosed
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	return s.frames.pop()
}

// CloseInput closes stdin exactly once. It is useful for one-shot headless
// commands that terminate after consuming all input.
func (s *Session) CloseInput() error {
	if s == nil {
		return ErrClosed
	}
	s.inputOnce.Do(func() { s.inputErr = s.stdin.Close() })
	return s.inputErr
}

// Wait waits for the child and every member of its process group to become
// quiescent. Repeated calls return independent copies of the same evidence.
func (s *Session) Wait() (Result, error) {
	if s == nil {
		return Result{}, ErrClosed
	}
	<-s.done
	s.resultMu.RLock()
	result := s.result
	result.Stderr = append([]byte(nil), result.Stderr...)
	err := s.waitErr
	s.resultMu.RUnlock()
	return result, err
}

// Close is idempotent. It requests bounded termination when the child is still
// running and suppresses only the expected ErrClosed terminal cause.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.requestStop(ErrClosed)
		_, err := s.Wait()
		if err != nil && (!errors.Is(err, ErrClosed) || errors.Is(err, ErrProcessNotQuiescent) || errors.Is(err, ErrProcessGroupLeak) || errors.Is(err, ErrStdoutLimit) || errors.Is(err, ErrStderrLimit)) {
			s.closeErr = err
		}
	})
	return s.closeErr
}

func (s *Session) requestStop(cause error) {
	if cause == nil {
		return
	}
	select {
	case <-s.done:
		return
	default:
	}
	s.stopMu.Lock()
	if s.stopCause == nil {
		s.stopCause = cause
		select {
		case s.stopCh <- struct{}{}:
		default:
		}
	}
	s.stopMu.Unlock()
}

func (s *Session) firstStopCause() error {
	s.stopMu.Lock()
	defer s.stopMu.Unlock()
	return s.stopCause
}

func (s *Session) supervise(ctx context.Context, waitDone <-chan error, stdoutDone <-chan error, stderrDone <-chan struct{}) {
	var waitErr error
	waitReceived := false
	select {
	case waitErr = <-waitDone:
		waitReceived = true
	case <-ctx.Done():
		s.requestStop(ctx.Err())
	case <-s.stopCh:
	}

	cause := s.firstStopCause()
	terminationRequested := cause != nil
	killed := false
	quiesced := false
	var quiesceErr error
	if terminationRequested {
		_ = s.CloseInput()
		killed, quiesced, quiesceErr = terminateAndQuiesce(s.cmd.Process, s.cmd.Process.Pid, s.config.terminationGrace, s.config.killWait)
		if !waitReceived {
			select {
			case waitErr = <-waitDone:
				waitReceived = true
			case <-time.After(s.config.killWait):
				quiesceErr = errors.Join(quiesceErr, ErrProcessNotQuiescent)
			}
		}
	} else {
		alive, aliveErr := processGroupAlive(s.cmd.Process, s.cmd.Process.Pid)
		if aliveErr != nil {
			quiesceErr = aliveErr
		} else if alive {
			cause = ErrProcessGroupLeak
			terminationRequested = true
			killed, quiesced, quiesceErr = terminateAndQuiesce(s.cmd.Process, s.cmd.Process.Pid, s.config.terminationGrace, s.config.killWait)
		} else {
			quiesced = true
		}
	}
	if !quiesced || !waitReceived {
		_ = s.stdoutPipe.Close()
		_ = s.stderrPipe.Close()
	}

	stdoutErr := <-stdoutDone
	<-stderrDone
	if cause == nil {
		cause = s.firstStopCause()
		terminationRequested = cause != nil
	}
	stderr, stderrBytes, stderrTruncated := s.stderr.snapshot()
	result := Result{
		ActualExecutablePath: s.config.executable, ActualExecutableDigest: s.config.executableDigest,
		PID: s.cmd.Process.Pid, ExitCode: -1, Signal: processStateSignal(s.cmd.ProcessState),
		StdoutBytes: s.stdout.bytesRead(), StdoutTruncated: s.stdout.exceeded.Load(),
		Stderr: stderr, StderrBytes: stderrBytes, StderrTruncated: stderrTruncated,
		TerminationRequested: terminationRequested, Killed: killed, Quiesced: quiesced,
	}
	if s.cmd.ProcessState != nil {
		result.ExitCode = s.cmd.ProcessState.ExitCode()
	}

	finalErr := cause
	if finalErr == nil && waitErr != nil {
		finalErr = fmt.Errorf("%w: %v", ErrProcessExit, waitErr)
	}
	if stdoutErr != nil {
		finalErr = errors.Join(finalErr, stdoutErr)
	}
	if quiesceErr != nil {
		finalErr = errors.Join(finalErr, quiesceErr)
	}
	if !quiesced {
		finalErr = errors.Join(finalErr, ErrProcessNotQuiescent)
	}

	s.resultMu.Lock()
	s.result = result
	s.waitErr = finalErr
	s.resultMu.Unlock()
	close(s.done)
}

func terminateAndQuiesce(process *os.Process, pid int, grace, killWait time.Duration) (bool, bool, error) {
	if err := signalProcessGroup(process, pid, groupSignalTerminate); err != nil {
		return false, false, fmt.Errorf("send SIGTERM to harness process group: %w", err)
	}
	if waitForGroupQuiescence(process, pid, grace) {
		return false, true, nil
	}
	if err := signalProcessGroup(process, pid, groupSignalKill); err != nil {
		return true, false, fmt.Errorf("send SIGKILL to harness process group: %w", err)
	}
	if waitForGroupQuiescence(process, pid, killWait) {
		return true, true, nil
	}
	return true, false, ErrProcessNotQuiescent
}

func waitForGroupQuiescence(process *os.Process, pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		alive, err := processGroupAlive(process, pid)
		if err == nil && !alive {
			return true
		}
		if timeout == 0 || !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(min(10*time.Millisecond, time.Until(deadline)))
	}
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		count, err := writer.Write(data)
		if err != nil {
			return err
		}
		if count == 0 {
			return io.ErrShortWrite
		}
		data = data[count:]
	}
	return nil
}

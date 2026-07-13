package process

import "errors"

var (
	ErrInvalidConfig              = errors.New("harness process configuration is invalid")
	ErrExecutableNotAbsolute      = errors.New("harness executable must be an absolute path")
	ErrExecutableNotRunnable      = errors.New("harness executable is not a runnable regular file")
	ErrExecutableDigestMismatch   = errors.New("harness executable digest does not match the pinned digest")
	ErrWorkingDirectoryNotAllowed = errors.New("harness working directory is not allowed")
	ErrEnvironmentNotAllowed      = errors.New("harness environment variable is not allowed")
	ErrSensitiveEnvironment       = errors.New("harness environment contains a sensitive variable")
	ErrUnsafeEnvironment          = errors.New("harness environment contains an unsafe loader variable")
	ErrClosed                     = errors.New("harness process is closed")
	ErrProcessExit                = errors.New("harness process exited unsuccessfully")
	ErrProcessNotQuiescent        = errors.New("harness process group did not become quiescent")
	ErrProcessGroupLeak           = errors.New("harness process left a descendant running")
	ErrStdoutLimit                = errors.New("harness stdout limit exceeded")
	ErrStderrLimit                = errors.New("harness stderr limit exceeded")
	ErrFrameTooLarge              = errors.New("harness frame size limit exceeded")
	ErrInvalidUTF8                = errors.New("harness frame is not valid UTF-8")
	ErrInvalidJSON                = errors.New("harness frame is not valid JSON")
	ErrPartialFrame               = errors.New("harness stream ended with a partial frame")
	ErrInvalidJSONRPC             = errors.New("harness frame is not a valid JSON-RPC 2.0 message")
	ErrDuplicateRequestID         = errors.New("harness JSON-RPC request ID was reused")
	ErrDuplicateResponseID        = errors.New("harness JSON-RPC response ID was duplicated")
	ErrUnknownResponseID          = errors.New("harness JSON-RPC response ID is unknown")
)

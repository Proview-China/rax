package codexappserver

import "errors"

var (
	ErrInvalidConfig      = errors.New("codex app-server client configuration is invalid")
	ErrProtocol           = errors.New("codex app-server protocol violation")
	ErrRPC                = errors.New("codex app-server JSON-RPC request failed")
	ErrClosed             = errors.New("codex app-server client is closed")
	ErrNotInitialized     = errors.New("codex app-server client is not initialized")
	ErrNoActiveTurn       = errors.New("codex app-server client has no active turn")
	ErrMissingTerminal    = errors.New("codex app-server stream ended without turn/completed")
	ErrUnexpectedRPCID    = errors.New("codex app-server received an unexpected response ID")
	ErrReverseRequest     = errors.New("codex app-server reverse request is not pending")
	ErrMapping            = errors.New("codex app-server invocation cannot be mapped without semantic loss")
	ErrUnsupportedCommand = errors.New("codex app-server execution command is unsupported")
	ErrAlreadyPrepared    = errors.New("codex app-server execution is already prepared")
	ErrPreparedNotFound   = errors.New("codex app-server execution was not preflighted")
	ErrRouteMismatch      = errors.New("codex app-server invocation changed after preflight")
)

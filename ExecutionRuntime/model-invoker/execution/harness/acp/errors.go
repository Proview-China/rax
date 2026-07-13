package acp

import "errors"

var (
	ErrInvalidConfig      = errors.New("ACP client configuration is invalid")
	ErrProtocol           = errors.New("ACP protocol violation")
	ErrRPC                = errors.New("ACP JSON-RPC request failed")
	ErrClosed             = errors.New("ACP client is closed")
	ErrMissingTerminal    = errors.New("ACP stream ended without prompt stopReason")
	ErrUnexpectedRPCID    = errors.New("ACP received an unexpected response ID")
	ErrReverseRequest     = errors.New("ACP reverse request is not pending")
	ErrMapping            = errors.New("ACP invocation cannot be mapped without semantic loss")
	ErrUnsupportedCommand = errors.New("ACP execution command is unsupported")
	ErrAlreadyPrepared    = errors.New("ACP execution is already prepared")
	ErrPreparedNotFound   = errors.New("ACP execution was not preflighted")
	ErrRouteMismatch      = errors.New("ACP invocation changed after preflight")
)

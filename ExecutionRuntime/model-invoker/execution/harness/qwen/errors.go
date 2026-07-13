package qwen

import "errors"

var (
	ErrInvalidConfig      = errors.New("Qwen harness configuration is invalid")
	ErrBareCoreTools      = errors.New("Qwen --bare cannot be combined with coreTools")
	ErrRouteMismatch      = errors.New("Qwen harness Route does not match the prepared plan")
	ErrProtocol           = errors.New("Qwen stream-json protocol violation")
	ErrManifestDrift      = errors.New("Qwen SDKSystemMessage differs from the expected manifest")
	ErrPreparedNotFound   = errors.New("Qwen preflight process was not found")
	ErrAlreadyPrepared    = errors.New("Qwen execution is already preflighted")
	ErrMissingResult      = errors.New("Qwen stream ended without SDKResult")
	ErrUnsupportedCommand = errors.New("Qwen harness command is unsupported")
)

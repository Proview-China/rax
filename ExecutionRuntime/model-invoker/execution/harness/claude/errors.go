package claude

import "errors"

var (
	ErrInvalidConfig      = errors.New("Claude harness configuration is invalid")
	ErrRouteMismatch      = errors.New("Claude harness Route does not match the prepared plan")
	ErrProtocol           = errors.New("Claude stream-json protocol violation")
	ErrManifestDrift      = errors.New("Claude init manifest differs from the expected manifest")
	ErrPreparedNotFound   = errors.New("Claude preflight process was not found")
	ErrAlreadyPrepared    = errors.New("Claude execution is already preflighted")
	ErrMissingResult      = errors.New("Claude stream ended without ResultMessage")
	ErrUnsupportedCommand = errors.New("Claude harness command is unsupported")
)

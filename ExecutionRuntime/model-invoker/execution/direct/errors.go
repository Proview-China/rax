package direct

import "errors"

var (
	ErrInvalidConfig      = errors.New("direct execution adapter configuration is invalid")
	ErrMapping            = errors.New("direct execution request cannot be mapped without semantic loss")
	ErrUnsupportedInput   = errors.New("direct execution input is unsupported")
	ErrUnsupportedTool    = errors.New("direct execution tool is unsupported")
	ErrUnsupportedSession = errors.New("direct execution session mode is unsupported")
	ErrUnsupportedCommand = errors.New("direct execution command is unsupported")
	ErrProtocolTerminal   = errors.New("direct model stream ended without a terminal event")
)

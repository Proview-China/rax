package streamjson

import "errors"

var (
	ErrInvalidConfig      = errors.New("stream-json configuration is invalid")
	ErrProtocol           = errors.New("stream-json protocol violation")
	ErrClosed             = errors.New("stream-json client is closed")
	ErrUnexpectedResponse = errors.New("stream-json control response has no pending request")
	ErrControl            = errors.New("stream-json control request failed")
)

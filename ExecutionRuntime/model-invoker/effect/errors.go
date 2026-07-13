package effect

import "errors"

var (
	ErrInvalidPolicy       = errors.New("effect observer policy is invalid")
	ErrPathOutsideRoots    = errors.New("effect path is outside allowed roots")
	ErrSymlinkNotAllowed   = errors.New("effect path traverses or names a disallowed symlink")
	ErrUnsupportedFileType = errors.New("effect path has an unsupported file type")
	ErrFileTooLarge        = errors.New("effect file exceeds the configured size limit")
	ErrSnapshotMismatch    = errors.New("effect snapshots do not describe the same target")
	ErrNoObservableChange  = errors.New("no observable effect occurred")
	ErrIntentMismatch      = errors.New("observed effect does not satisfy the intent kind")
	ErrInvalidJSON         = errors.New("structured output is not one strict JSON value")
	ErrInvalidSchema       = errors.New("structured output schema is invalid")
	ErrSchemaViolation     = errors.New("structured output violates its schema")
	ErrExternalSchemaRef   = errors.New("external JSON Schema references are disabled")
	ErrRepairExhausted     = errors.New("structured output repair attempts were exhausted")
)

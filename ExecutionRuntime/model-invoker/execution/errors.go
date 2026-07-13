package execution

import "errors"

var (
	ErrInvalidAdapter           = errors.New("execution adapter is invalid")
	ErrAdapterAlreadyRegistered = errors.New("execution adapter is already registered")
	ErrAdapterNotFound          = errors.New("execution adapter was not found")
	ErrInvalidInvocation        = errors.New("execution invocation is invalid")
	ErrPreflightRejected        = errors.New("execution preflight was rejected")
	ErrPreflightManifestDrift   = errors.New("execution preflight manifest drifted")
	ErrSessionClosed            = errors.New("execution session is closed")
	ErrLedgerInvariant          = errors.New("execution ledger invariant violated")
	ErrSequence                 = errors.New("execution event sequence is invalid")
	ErrTerminal                 = errors.New("execution is already terminal")
	ErrAdapterAuthority         = errors.New("adapter attempted a runtime-owned transition")
	ErrApprovalNotPending       = errors.New("execution approval is not pending")
	ErrApprovalExpired          = errors.New("execution approval expired")
	ErrApprovalRevision         = errors.New("execution approval revision does not match")
	ErrIdempotencyConflict      = errors.New("execution idempotency key was reused for a different command")
	ErrOptimisticConcurrency    = errors.New("execution command expected state does not match")
	ErrCancelState              = errors.New("execution cancellation transition is invalid")
	ErrProjectionInvariant      = errors.New("execution result projection invariant violated")
)

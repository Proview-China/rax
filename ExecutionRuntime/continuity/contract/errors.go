package contract

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	ErrInvalidArgument         ErrorCode = "continuity/invalid_argument"
	ErrEvidenceNotInspectable  ErrorCode = "continuity/evidence_not_inspectable"
	ErrEvidenceConflict        ErrorCode = "continuity/evidence_conflict"
	ErrProjectionConflict      ErrorCode = "continuity/timeline_projection_conflict"
	ErrCursorInvalidated       ErrorCode = "continuity/cursor_invalidated"
	ErrCursorExpired           ErrorCode = "continuity/cursor_expired"
	ErrWatchGap                ErrorCode = "continuity/watch_gap"
	ErrContentDigestMismatch   ErrorCode = "continuity/content_digest_mismatch"
	ErrCrossStoreIndeterminate ErrorCode = "continuity/cross_store_indeterminate"
	ErrRevisionConflict        ErrorCode = "continuity/revision_conflict"
	ErrNotFound                ErrorCode = "continuity/not_found"
	ErrCheckpointPartial       ErrorCode = "continuity/checkpoint_partial"
	ErrCheckpointIndeterminate ErrorCode = "continuity/checkpoint_indeterminate"
	ErrRestoreIncompatible     ErrorCode = "continuity/restore_incompatible"
	ErrRewindConflict          ErrorCode = "continuity/rewind_conflict"
	ErrRetentionBlocked        ErrorCode = "continuity/retention_blocked"
	ErrEffectUnknown           ErrorCode = "continuity/effect_unknown"
	ErrUnsupported             ErrorCode = "continuity/unsupported"
	ErrPreconditionFailed      ErrorCode = "continuity/precondition_failed"
	ErrUnavailable             ErrorCode = "continuity/unavailable"
	ErrIndeterminate           ErrorCode = "continuity/indeterminate"
)

type Error struct {
	Code    ErrorCode
	Field   string
	Message string
}

func (e *Error) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %s", e.Code, e.Field, e.Message)
}

func NewError(code ErrorCode, field, message string) error {
	return &Error{Code: code, Field: field, Message: message}
}

func HasCode(err error, code ErrorCode) bool {
	var target *Error
	return errors.As(err, &target) && target.Code == code
}

package contract

import "errors"

var (
	ErrInvalidArgument       = errors.New("memory-knowledge: invalid argument")
	ErrNotFound              = errors.New("memory-knowledge: not found")
	ErrAlreadyExists         = errors.New("memory-knowledge: already exists")
	ErrRevisionConflict      = errors.New("memory-knowledge: revision conflict")
	ErrEvidenceConflict      = errors.New("memory-knowledge: evidence conflict")
	ErrCandidateRejected     = errors.New("memory-knowledge: candidate rejected")
	ErrNotCurrent            = errors.New("memory-knowledge: fact is not current")
	ErrScopeDenied           = errors.New("memory-knowledge: scope denied")
	ErrSensitivityDenied     = errors.New("memory-knowledge: sensitivity denied")
	ErrUnknownOutcome        = errors.New("memory-knowledge: unknown outcome")
	ErrInspectionIncomplete  = errors.New("memory-knowledge: inspection incomplete")
	ErrSettlementMismatch    = errors.New("memory-knowledge: settlement reference mismatch")
	ErrContextUnmaterialized = errors.New("memory-knowledge: context reference cannot be materialized")
	ErrUnsupported           = errors.New("memory-knowledge: unsupported in wave 1")
)

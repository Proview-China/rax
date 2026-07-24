package fakes

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

// ContentIntegrityAuditGovernanceV1 is a test-only lost-reply seam. It does
// not synthesize findings, content, cleanup authority, or provider behavior.
type ContentIntegrityAuditGovernanceV1 struct {
	Delegate ports.ContentIntegrityAuditGovernancePortV1

	mu            sync.Mutex
	nextCreateErr error
}

func NewContentIntegrityAuditGovernanceV1(delegate ports.ContentIntegrityAuditGovernancePortV1) (*ContentIntegrityAuditGovernanceV1, error) {
	if nilContentIntegrityAuditPortV1(delegate) {
		return nil, contract.NewError(contract.ErrInvalidArgument, "content_integrity_audit_governance_port", "delegate is required")
	}
	return &ContentIntegrityAuditGovernanceV1{Delegate: delegate}, nil
}

func (f *ContentIntegrityAuditGovernanceV1) LoseNextSuccessfulCreateReply(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextCreateErr = err
}

func (f *ContentIntegrityAuditGovernanceV1) CreateContentIntegrityAuditV1(ctx context.Context, request ports.CreateContentIntegrityAuditRequestV1) (contract.ContentIntegrityAuditFactV1, bool, error) {
	value, replay, err := f.Delegate.CreateContentIntegrityAuditV1(ctx, request)
	if err != nil {
		return value, replay, err
	}
	f.mu.Lock()
	lost := f.nextCreateErr
	f.nextCreateErr = nil
	f.mu.Unlock()
	if lost != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, lost
	}
	return value, replay, nil
}

func (f *ContentIntegrityAuditGovernanceV1) InspectContentIntegrityAuditV1(ctx context.Context, request ports.InspectContentIntegrityAuditRequestV1) (contract.ContentIntegrityAuditFactV1, error) {
	return f.Delegate.InspectContentIntegrityAuditV1(ctx, request)
}

func (f *ContentIntegrityAuditGovernanceV1) InspectContentIntegrityAuditByIDV1(ctx context.Context, request ports.InspectContentIntegrityAuditByIDRequestV1) (contract.ContentIntegrityAuditFactV1, error) {
	return f.Delegate.InspectContentIntegrityAuditByIDV1(ctx, request)
}

func nilContentIntegrityAuditPortV1(value ports.ContentIntegrityAuditGovernancePortV1) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}

var _ ports.ContentIntegrityAuditGovernancePortV1 = (*ContentIntegrityAuditGovernanceV1)(nil)

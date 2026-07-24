package fakes

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

// HistoryDerivationCandidateGovernanceV1 is a test-only lost-reply seam. It
// cannot publish current state, mutate Events, execute compaction, or purge.
type HistoryDerivationCandidateGovernanceV1 struct {
	Delegate      ports.HistoryDerivationCandidateGovernancePortV1
	mu            sync.Mutex
	nextCreateErr error
}

func NewHistoryDerivationCandidateGovernanceV1(delegate ports.HistoryDerivationCandidateGovernancePortV1) (*HistoryDerivationCandidateGovernanceV1, error) {
	if delegate == nil || (reflect.ValueOf(delegate).Kind() == reflect.Pointer && reflect.ValueOf(delegate).IsNil()) {
		return nil, contract.NewError(contract.ErrInvalidArgument, "history_derivation_governance_port", "delegate is required")
	}
	return &HistoryDerivationCandidateGovernanceV1{Delegate: delegate}, nil
}

func (f *HistoryDerivationCandidateGovernanceV1) LoseNextSuccessfulCreateReply(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextCreateErr = err
}

func (f *HistoryDerivationCandidateGovernanceV1) CreateHistoryDerivationCandidateV1(ctx context.Context, request ports.CreateHistoryDerivationCandidateRequestV1) (contract.HistoryDerivationCandidateFactV1, bool, error) {
	value, replay, err := f.Delegate.CreateHistoryDerivationCandidateV1(ctx, request)
	if err != nil {
		return value, replay, err
	}
	f.mu.Lock()
	lost := f.nextCreateErr
	f.nextCreateErr = nil
	f.mu.Unlock()
	if lost != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, lost
	}
	return value, replay, nil
}

func (f *HistoryDerivationCandidateGovernanceV1) InspectHistoryDerivationCandidateV1(ctx context.Context, request ports.InspectHistoryDerivationCandidateRequestV1) (contract.HistoryDerivationCandidateFactV1, error) {
	return f.Delegate.InspectHistoryDerivationCandidateV1(ctx, request)
}

func (f *HistoryDerivationCandidateGovernanceV1) InspectHistoryDerivationCandidateByIDV1(ctx context.Context, request ports.InspectHistoryDerivationCandidateByIDRequestV1) (contract.HistoryDerivationCandidateFactV1, error) {
	return f.Delegate.InspectHistoryDerivationCandidateByIDV1(ctx, request)
}

var _ ports.HistoryDerivationCandidateGovernancePortV1 = (*HistoryDerivationCandidateGovernanceV1)(nil)

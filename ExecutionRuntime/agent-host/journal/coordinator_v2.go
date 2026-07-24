package journal

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

type CoordinatorV2 struct {
	facts ports.JournalFactPortV2
	now   func() time.Time
}

func NewCoordinatorV2(facts ports.JournalFactPortV2, now func() time.Time) (*CoordinatorV2, error) {
	if contract.IsTypedNilV1(facts) { return nil, contract.NewError(contract.ErrorInvalidArgument, "journal_port_missing", "HostV2 Journal Fact Port is required") }
	if now == nil { now = time.Now }
	return &CoordinatorV2{facts: facts, now: now}, nil
}

func (c *CoordinatorV2) EnsureAcceptedV2(ctx context.Context, claim contract.HostStartClaimV1) (contract.HostJournalV2, error) {
	if c == nil || contract.IsTypedNilV1(c.facts) { return contract.HostJournalV2{}, contract.NewError(contract.ErrorUnavailable, "journal_coordinator_missing", "HostV2 Journal coordinator is unavailable") }
	if ctx == nil { return contract.HostJournalV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required") }
	if claim.HostContractVersion != contract.ContractVersionV2 { return contract.HostJournalV2{}, contract.NewError(contract.ErrorConflict, "host_start_claim_version_drift", "HostV2 Journal requires a V2 shared start claim") }
	now, err := safeNowV1(c.now); if err != nil { return contract.HostJournalV2{}, err }; if err := claim.ValidateCurrentV1(now); err != nil { return contract.HostJournalV2{}, err }
	claimRef, err := claim.RefV1(); if err != nil { return contract.HostJournalV2{}, err }
	desired, err := contract.SealHostJournalV2(contract.HostJournalV2{ContractVersion: contract.HostJournalContractVersionV2, HostID: claim.HostID, StartID: claim.StartID, Revision: 1, Phase: contract.HostAcceptedV2, StartClaimRef: claimRef, ConfigDigest: claim.ConfigDigest, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}); if err != nil { return contract.HostJournalV2{}, err }
	actual, writeErr := safeCreateJournalV2(ctx, c.facts, desired)
	if writeErr == nil { return exactJournalOriginV2(actual, desired) }
	if !contract.HasCode(writeErr, contract.ErrorConflict) && !contract.HasCode(writeErr, contract.ErrorUnavailable) && !contract.HasCode(writeErr, contract.ErrorUnknownOutcome) { return contract.HostJournalV2{}, writeErr }
	inspected, inspectErr := safeInspectJournalV2(context.WithoutCancel(ctx), c.facts, claim.HostID, claim.StartID); if inspectErr != nil { return contract.HostJournalV2{}, writeErr }
	return exactJournalOriginV2(inspected, desired)
}

func (c *CoordinatorV2) AdvanceV2(ctx context.Context, current, next contract.HostJournalV2) (contract.HostJournalV2, error) {
	if c == nil || contract.IsTypedNilV1(c.facts) { return contract.HostJournalV2{}, contract.NewError(contract.ErrorUnavailable, "journal_coordinator_missing", "HostV2 Journal coordinator is unavailable") }
	if ctx == nil { return contract.HostJournalV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required") }
	if err := contract.ValidateHostJournalSuccessorV2(current, next); err != nil { return contract.HostJournalV2{}, err }
	expected, _ := current.RefV2(); actual, writeErr := safeCASJournalV2(ctx, c.facts, expected, next)
	if writeErr == nil { return exactJournalSuccessorV2(actual, next) }
	if !contract.HasCode(writeErr, contract.ErrorConflict) && !contract.HasCode(writeErr, contract.ErrorUnavailable) && !contract.HasCode(writeErr, contract.ErrorUnknownOutcome) { return contract.HostJournalV2{}, writeErr }
	inspected, inspectErr := safeInspectJournalV2(context.WithoutCancel(ctx), c.facts, current.HostID, current.StartID); if inspectErr != nil { return contract.HostJournalV2{}, writeErr }
	if inspected.Digest == current.Digest { return contract.HostJournalV2{}, writeErr }
	return exactJournalSuccessorV2(inspected, next)
}

func (c *CoordinatorV2) InspectV2(ctx context.Context, hostID, startID string) (contract.HostJournalV2, error) {
	if c == nil || contract.IsTypedNilV1(c.facts) { return contract.HostJournalV2{}, contract.NewError(contract.ErrorUnavailable, "journal_coordinator_missing", "HostV2 Journal coordinator is unavailable") }
	if ctx == nil { return contract.HostJournalV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required") }
	value, err := safeInspectJournalV2(ctx, c.facts, hostID, startID); if err != nil { return contract.HostJournalV2{}, err }; if err := value.Validate(); err != nil { return contract.HostJournalV2{}, err }; return value, nil
}

func safeCreateJournalV2(ctx context.Context, facts ports.JournalFactPortV2, value contract.HostJournalV2) (result contract.HostJournalV2, err error) { defer func(){ if recover()!=nil { result=contract.HostJournalV2{}; err=contract.NewError(contract.ErrorUnknownOutcome,"host_journal_create_panic","HostV2 Journal create outcome is unknown after panic") } }(); return facts.CreateHostJournalV2(ctx,value) }
func safeCASJournalV2(ctx context.Context, facts ports.JournalFactPortV2, expected contract.ExactRefV1, next contract.HostJournalV2) (result contract.HostJournalV2, err error) { defer func(){ if recover()!=nil { result=contract.HostJournalV2{}; err=contract.NewError(contract.ErrorUnknownOutcome,"host_journal_cas_panic","HostV2 Journal CAS outcome is unknown after panic") } }(); return facts.CompareAndSwapHostJournalV2(ctx,expected,next) }
func safeInspectJournalV2(ctx context.Context, facts ports.JournalFactPortV2, hostID,startID string) (result contract.HostJournalV2, err error) { defer func(){ if recover()!=nil { result=contract.HostJournalV2{}; err=contract.NewError(contract.ErrorUnavailable,"host_journal_inspect_panic","HostV2 Journal inspect panicked") } }(); return facts.InspectHostJournalV2(ctx,hostID,startID) }
func exactJournalOriginV2(actual, desired contract.HostJournalV2) (contract.HostJournalV2,error) { if err:=actual.Validate(); err!=nil{return contract.HostJournalV2{},err}; if actual.HostID!=desired.HostID || actual.StartID!=desired.StartID || actual.StartClaimRef!=desired.StartClaimRef || actual.ConfigDigest!=desired.ConfigDigest { return contract.HostJournalV2{},contract.NewError(contract.ErrorConflict,"host_journal_origin_conflict","HostV2 Journal origin drifted") }; return actual,nil }
func exactJournalSuccessorV2(actual, desired contract.HostJournalV2) (contract.HostJournalV2,error) { if err:=actual.Validate(); err!=nil{return contract.HostJournalV2{},err}; if actual.Digest!=desired.Digest{return contract.HostJournalV2{},contract.NewError(contract.ErrorConflict,"host_journal_successor_conflict","HostV2 Journal recovery returned another successor")}; return actual,nil }

package journal

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

type HostStartAdmissionV1 struct {
	facts ports.HostStartClaimPortV1
	now   func() time.Time
}

func NewHostStartAdmissionV1(facts ports.HostStartClaimPortV1, now func() time.Time) (*HostStartAdmissionV1, error) {
	if contract.IsTypedNilV1(facts) {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "host_start_claim_port_missing", "host start claim port is required")
	}
	if now == nil {
		now = time.Now
	}
	return &HostStartAdmissionV1{facts: facts, now: now}, nil
}

// ClaimV1 returns only the exact desired claim. Any uncertain write is followed
// by Inspect; NotFound/Unavailable after an uncertain write remains unknown and
// never authorizes a second identity or different payload.
func (a *HostStartAdmissionV1) ClaimV1(ctx context.Context, desired contract.HostStartClaimV1) (contract.HostStartClaimV1, error) {
	if a == nil || contract.IsTypedNilV1(a.facts) {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorUnavailable, "host_start_admission_missing", "host start admission is unavailable")
	}
	if ctx == nil {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	// HostV3 couples the permanent Claim and its Input sidecar at one atomic
	// owner boundary. Reject it here, before touching even a permissive V1
	// implementation; relying on the concrete Store to reject this would leave
	// the public V1 admission path capable of creating a partial V3 fact.
	if desired.HostContractVersion == contract.HostLifecycleContractVersionV3 {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorPrecondition, "host_start_v3_atomic_port_required", "HostStart V3 must use the atomic V3 Claim/Input admission")
	}
	now, err := safeNowV1(a.now)
	if err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if err := desired.ValidateCurrentV1(now); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	actual, writeErr := safeClaimHostStartV1(ctx, a.facts, desired)
	if writeErr == nil {
		return exactHostStartClaimV1(actual, desired)
	}
	if !contract.HasCode(writeErr, contract.ErrorConflict) && !contract.HasCode(writeErr, contract.ErrorUnavailable) && !contract.HasCode(writeErr, contract.ErrorUnknownOutcome) {
		return contract.HostStartClaimV1{}, writeErr
	}
	inspected, inspectErr := safeInspectHostStartV1(context.WithoutCancel(ctx), a.facts, desired.HostID, desired.StartID)
	if inspectErr != nil {
		return contract.HostStartClaimV1{}, writeErr
	}
	return exactHostStartClaimV1(inspected, desired)
}

func (a *HostStartAdmissionV1) InspectV1(ctx context.Context, hostID, startID string) (contract.HostStartClaimV1, error) {
	if a == nil || contract.IsTypedNilV1(a.facts) {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorUnavailable, "host_start_admission_missing", "host start admission is unavailable")
	}
	if ctx == nil {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	value, err := safeInspectHostStartV1(ctx, a.facts, hostID, startID)
	if err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if err := value.ValidateHistoricalV1(); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	return value, nil
}

func safeClaimHostStartV1(ctx context.Context, facts ports.HostStartClaimPortV1, desired contract.HostStartClaimV1) (result contract.HostStartClaimV1, err error) {
	defer func() {
		if recover() != nil {
			result = contract.HostStartClaimV1{}
			err = contract.NewError(contract.ErrorUnknownOutcome, "host_start_claim_panic", "host start claim outcome is unknown after panic")
		}
	}()
	return facts.ClaimOrInspectHostStartV1(ctx, desired)
}

func safeInspectHostStartV1(ctx context.Context, facts ports.HostStartClaimPortV1, hostID, startID string) (result contract.HostStartClaimV1, err error) {
	defer func() {
		if recover() != nil {
			result = contract.HostStartClaimV1{}
			err = contract.NewError(contract.ErrorUnavailable, "host_start_claim_inspect_panic", "host start claim inspection panicked")
		}
	}()
	return facts.InspectHostStartClaimV1(ctx, hostID, startID)
}

func exactHostStartClaimV1(actual, desired contract.HostStartClaimV1) (contract.HostStartClaimV1, error) {
	if err := actual.ValidateHistoricalV1(); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if !contract.SameHostStartClaimV1(actual, desired) {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorConflict, "host_start_claim_conflict", "HostID and StartID are bound to another exact claim")
	}
	return actual, nil
}

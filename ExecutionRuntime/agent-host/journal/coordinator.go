package journal

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

type CoordinatorV1 struct {
	facts ports.JournalFactPortV1
	now   func() time.Time
}

func NewCoordinatorV1(facts ports.JournalFactPortV1, now func() time.Time) (*CoordinatorV1, error) {
	if contract.IsTypedNilV1(facts) {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "journal_port_missing", "journal fact port is required")
	}
	if now == nil {
		now = time.Now
	}
	return &CoordinatorV1{facts: facts, now: now}, nil
}

func (c *CoordinatorV1) EnsureAcceptedV1(ctx context.Context, hostID, startID string, configDigest contract.DigestV1) (contract.HostJournalV1, error) {
	if err := contract.ValidateIdentifierV1("host id", hostID); err != nil {
		return contract.HostJournalV1{}, err
	}
	if err := contract.ValidateIdentifierV1("start id", startID); err != nil {
		return contract.HostJournalV1{}, err
	}
	if err := configDigest.Validate(); err != nil {
		return contract.HostJournalV1{}, err
	}
	now, clockErr := safeNowV1(c.now)
	if clockErr != nil {
		return contract.HostJournalV1{}, clockErr
	}
	if now.IsZero() {
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorUnavailable, "clock_unavailable", "journal clock returned zero")
	}
	desired, err := contract.SealHostJournalV1(contract.HostJournalV1{ContractVersion: contract.ContractVersionV1, HostID: hostID, StartID: startID, Revision: 1, Phase: contract.HostAcceptedV1, ConfigDigest: configDigest, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	if err != nil {
		return contract.HostJournalV1{}, err
	}
	created, err := safeCreateV1(ctx, c.facts, desired)
	if err == nil {
		return c.exactOriginV1(created, desired)
	}
	if !contract.HasCode(err, contract.ErrorConflict) && !contract.HasCode(err, contract.ErrorUnavailable) && !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		return contract.HostJournalV1{}, err
	}
	inspected, inspectErr := safeInspectV1(context.WithoutCancel(ctx), c.facts, hostID, startID)
	if inspectErr != nil {
		return contract.HostJournalV1{}, err
	}
	return c.exactOriginV1(inspected, desired)
}

func (c *CoordinatorV1) AdvanceV1(ctx context.Context, current, next contract.HostJournalV1) (contract.HostJournalV1, error) {
	if err := contract.ValidateJournalSuccessorV1(current, next); err != nil {
		return contract.HostJournalV1{}, err
	}
	expected, err := current.RefV1()
	if err != nil {
		return contract.HostJournalV1{}, err
	}
	committed, err := safeCASV1(ctx, c.facts, expected, next)
	if err == nil {
		return c.exactSuccessorV1(committed, next)
	}
	if !contract.HasCode(err, contract.ErrorConflict) && !contract.HasCode(err, contract.ErrorUnavailable) && !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		return contract.HostJournalV1{}, err
	}
	inspected, inspectErr := safeInspectV1(context.WithoutCancel(ctx), c.facts, current.HostID, current.StartID)
	if inspectErr != nil {
		return contract.HostJournalV1{}, err
	}
	if inspected.Digest == current.Digest {
		return contract.HostJournalV1{}, err
	}
	return c.exactSuccessorV1(inspected, next)
}

func (c *CoordinatorV1) InspectV1(ctx context.Context, hostID, startID string) (contract.HostJournalV1, error) {
	value, err := safeInspectV1(ctx, c.facts, hostID, startID)
	if err != nil {
		return contract.HostJournalV1{}, err
	}
	if err := value.Validate(); err != nil {
		return contract.HostJournalV1{}, err
	}
	return value, nil
}

func safeNowV1(now func() time.Time) (result time.Time, err error) {
	defer func() {
		if recover() != nil {
			result = time.Time{}
			err = contract.NewError(contract.ErrorUnavailable, "journal_clock_panic", "journal clock panicked")
		}
	}()
	result = now()
	return result, nil
}
func safeCreateV1(ctx context.Context, facts ports.JournalFactPortV1, value contract.HostJournalV1) (result contract.HostJournalV1, err error) {
	defer func() {
		if recover() != nil {
			result = contract.HostJournalV1{}
			err = contract.NewError(contract.ErrorUnknownOutcome, "journal_create_panic", "journal create outcome is unknown after panic")
		}
	}()
	return facts.CreateHostJournalV1(ctx, value)
}
func safeCASV1(ctx context.Context, facts ports.JournalFactPortV1, expected contract.ExactRefV1, next contract.HostJournalV1) (result contract.HostJournalV1, err error) {
	defer func() {
		if recover() != nil {
			result = contract.HostJournalV1{}
			err = contract.NewError(contract.ErrorUnknownOutcome, "journal_cas_panic", "journal compare-and-swap outcome is unknown after panic")
		}
	}()
	return facts.CompareAndSwapHostJournalV1(ctx, expected, next)
}
func safeInspectV1(ctx context.Context, facts ports.JournalFactPortV1, hostID, startID string) (result contract.HostJournalV1, err error) {
	defer func() {
		if recover() != nil {
			result = contract.HostJournalV1{}
			err = contract.NewError(contract.ErrorUnavailable, "journal_inspect_panic", "journal inspection panicked")
		}
	}()
	return facts.InspectHostJournalV1(ctx, hostID, startID)
}

func (*CoordinatorV1) exactOriginV1(actual, desired contract.HostJournalV1) (contract.HostJournalV1, error) {
	if err := actual.Validate(); err != nil {
		return contract.HostJournalV1{}, err
	}
	if actual.HostID != desired.HostID || actual.StartID != desired.StartID || actual.ConfigDigest != desired.ConfigDigest {
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorConflict, "journal_origin_conflict", "start id already binds another host config")
	}
	return actual, nil
}
func (*CoordinatorV1) exactSuccessorV1(actual, desired contract.HostJournalV1) (contract.HostJournalV1, error) {
	if err := actual.Validate(); err != nil {
		return contract.HostJournalV1{}, err
	}
	if actual.Digest != desired.Digest {
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorConflict, "journal_successor_conflict", "journal recovery returned another successor")
	}
	return actual, nil
}

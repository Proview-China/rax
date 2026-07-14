// Package fakes provides deterministic, in-memory Runtime fact owners for
// contract tests. They are not production persistence backends.
package fakes

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type FactStore struct {
	mu                sync.Mutex
	clock             func() time.Time
	evidenceAvailable bool
	instances         map[string]*instanceFacts
	leases            map[string]*leaseFacts
	leaseIDs          map[string]*leaseFacts
	activations       map[string]*activationFacts
	runs              map[string]*runFacts
	activeRuns        map[string]core.AgentRunID
	loseRunWriteReply bool
}

type instanceFacts struct {
	desired     control.DesiredStateSnapshot
	commands    []control.CommandRecord
	outbox      []control.OutboxRecord
	idempotency map[string]control.CommandAcceptance
}

type leaseFacts struct {
	lease control.IdentityExecutionLease
}

type runFacts struct {
	record core.AgentRunRecord
}

func NewFactStore(clock func() time.Time) *FactStore {
	if clock == nil {
		clock = time.Now
	}
	return &FactStore{
		clock: clock, evidenceAvailable: true,
		instances: make(map[string]*instanceFacts), leases: make(map[string]*leaseFacts), leaseIDs: make(map[string]*leaseFacts), activations: make(map[string]*activationFacts),
		runs: make(map[string]*runFacts), activeRuns: make(map[string]core.AgentRunID),
	}
}

// SetEvidenceAvailable is a deterministic fault-injection control for tests.
func (s *FactStore) SetEvidenceAvailable(available bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evidenceAvailable = available
}

// LoseNextRunWriteReply injects a durable-write/successful-commit response
// loss. A restarted coordinator must Inspect instead of blindly creating or
// completing the Run again.
func (s *FactStore) LoseNextRunWriteReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseRunWriteReply = true
}

func (s *FactStore) CreateRun(ctx context.Context, initial core.AgentRunRecord) (core.AgentRunRecord, error) {
	if err := contextError(ctx); err != nil {
		return core.AgentRunRecord{}, err
	}
	if err := initial.Validate(); err != nil {
		return core.AgentRunRecord{}, err
	}
	if initial.Revision != 1 || (initial.Status != core.RunPending && initial.Status != core.RunRunning) {
		return core.AgentRunRecord{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "new run fact must be pending or running at revision one")
	}
	key := runKey(initial.Scope, initial.ID)
	instanceKey := executionKey(initial.Scope)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[key]; exists {
		return core.AgentRunRecord{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "run fact already exists")
	}
	if _, exists := s.activeRuns[instanceKey]; exists {
		return core.AgentRunRecord{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "instance already has an active run fact")
	}
	initial.Scope = cloneScope(initial.Scope)
	s.runs[key] = &runFacts{record: initial}
	s.activeRuns[instanceKey] = initial.ID
	if s.consumeLostRunWriteReply() {
		return core.AgentRunRecord{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected lost run fact write reply")
	}
	return cloneRun(initial), nil
}

func (s *FactStore) InspectRun(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (core.AgentRunRecord, error) {
	if err := contextError(ctx); err != nil {
		return core.AgentRunRecord{}, err
	}
	if err := scope.Validate(); err != nil {
		return core.AgentRunRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, exists := s.runs[runKey(scope, runID)]
	if !exists || !sameScope(facts.record.Scope, scope) {
		return core.AgentRunRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "run fact does not exist for the execution scope")
	}
	return cloneRun(facts.record), nil
}

func (s *FactStore) InspectActiveRun(ctx context.Context, scope core.ExecutionScope) (core.AgentRunRecord, error) {
	if err := contextError(ctx); err != nil {
		return core.AgentRunRecord{}, err
	}
	if err := scope.Validate(); err != nil {
		return core.AgentRunRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	runID, exists := s.activeRuns[executionKey(scope)]
	if !exists {
		return core.AgentRunRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "execution scope has no active run fact")
	}
	facts, exists := s.runs[runKey(scope, runID)]
	if !exists || !sameScope(facts.record.Scope, scope) {
		return core.AgentRunRecord{}, core.NewError(core.ErrorInternal, core.ReasonRunConflict, "active run index does not match a run fact")
	}
	return cloneRun(facts.record), nil
}

func (s *FactStore) CompareAndSwapRun(ctx context.Context, request control.RunFactCASRequest) (core.AgentRunRecord, error) {
	if err := contextError(ctx); err != nil {
		return core.AgentRunRecord{}, err
	}
	if err := request.Validate(); err != nil {
		return core.AgentRunRecord{}, err
	}
	key := runKey(request.Next.Scope, request.Next.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, exists := s.runs[key]
	if !exists || !sameScope(facts.record.Scope, request.Next.Scope) {
		return core.AgentRunRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "run fact does not exist for the execution scope")
	}
	if facts.record.Revision != request.ExpectedRevision {
		return core.AgentRunRecord{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "run fact revision does not match CAS precondition")
	}
	if err := control.ValidateRunFactTransition(facts.record, request.Next); err != nil {
		return core.AgentRunRecord{}, err
	}
	next := request.Next
	next.Scope = cloneScope(next.Scope)
	facts.record = next
	instanceKey := executionKey(next.Scope)
	if next.Status == core.RunTerminal {
		delete(s.activeRuns, instanceKey)
	} else {
		s.activeRuns[instanceKey] = next.ID
	}
	if s.consumeLostRunWriteReply() {
		return core.AgentRunRecord{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected lost run fact write reply")
	}
	return cloneRun(next), nil
}

func (s *FactStore) consumeLostRunWriteReply() bool {
	if !s.loseRunWriteReply {
		return false
	}
	s.loseRunWriteReply = false
	return true
}

func (s *FactStore) CreateDesiredState(ctx context.Context, initial control.DesiredStateSnapshot) (control.DesiredStateSnapshot, error) {
	if err := contextError(ctx); err != nil {
		return control.DesiredStateSnapshot{}, err
	}
	if err := initial.Scope.Validate(); err != nil {
		return control.DesiredStateSnapshot{}, err
	}
	if !control.ValidDesiredState(initial.Desired) || initial.Revision != 1 || initial.LastCommandID != "" {
		return control.DesiredStateSnapshot{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "initial desired state requires a valid state, revision one and no command")
	}
	key := executionKey(initial.Scope)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.instances[key]; exists {
		return control.DesiredStateSnapshot{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "desired state already exists")
	}
	initial.Scope = cloneScope(initial.Scope)
	s.instances[key] = &instanceFacts{desired: initial, idempotency: make(map[string]control.CommandAcceptance)}
	return cloneDesired(initial), nil
}

func (s *FactStore) AcceptCommand(ctx context.Context, intent control.CommandIntent) (control.CommandAcceptance, error) {
	if err := contextError(ctx); err != nil {
		return control.CommandAcceptance{}, err
	}
	if err := intent.Envelope.Validate(); err != nil {
		return control.CommandAcceptance{}, err
	}
	if err := intent.Mutation.ValidateFor(intent.Envelope.Kind); err != nil {
		return control.CommandAcceptance{}, err
	}
	now := s.clock()
	if !now.Before(intent.Envelope.ExpiresAt) {
		return control.CommandAcceptance{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "command has expired")
	}

	key := executionKey(intent.Envelope.Target)
	idempotencyKey := commandIdempotencyKey(intent.Envelope)
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, exists := s.instances[key]
	if !exists {
		return control.CommandAcceptance{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "desired state does not exist")
	}
	if prior, exists := facts.idempotency[idempotencyKey]; exists {
		if prior.Record.Envelope.CanonicalPayloadDigest != intent.Envelope.CanonicalPayloadDigest {
			return control.CommandAcceptance{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "idempotency key was reused with a different payload")
		}
		return cloneAcceptance(prior), nil
	}
	if !sameScope(facts.desired.Scope, intent.Envelope.Target) {
		return control.CommandAcceptance{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleInstanceEpoch, "command target does not match registered execution scope")
	}
	if err := core.CheckExecutionPreconditions(intent.Envelope.Preconditions, core.CurrentExecutionFacts{Scope: facts.desired.Scope, Revision: facts.desired.Revision}); err != nil {
		return control.CommandAcceptance{}, err
	}
	if isNonLifecycleCommand(intent.Envelope.Kind) && intent.Mutation.Desired != facts.desired.Desired {
		return control.CommandAcceptance{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "non-lifecycle command cannot change desired lifecycle state")
	}
	if !s.evidenceAvailable {
		return control.CommandAcceptance{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "command intent and outbox cannot be persisted")
	}
	for _, existing := range facts.commands {
		if (existing.Status == control.CommandAccepted || existing.Status == control.CommandExecuting) && control.Supersedes(existing.Envelope, intent.Envelope) {
			return control.CommandAcceptance{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCommandDominated, "an accepted safety command dominates this command")
		}
		if existing.Envelope.ID == intent.Envelope.ID {
			return control.CommandAcceptance{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "command id already exists")
		}
	}

	nextRevision := facts.desired.Revision + 1
	for index := range facts.commands {
		if (facts.commands[index].Status == control.CommandAccepted || facts.commands[index].Status == control.CommandExecuting) && control.Supersedes(intent.Envelope, facts.commands[index].Envelope) {
			facts.commands[index].Status = control.CommandSuperseded
		}
	}
	record := control.CommandRecord{Envelope: intent.Envelope, Revision: nextRevision, Status: control.CommandAccepted, RecordedAt: now}
	desired := control.DesiredStateSnapshot{
		Scope: cloneScope(facts.desired.Scope), Desired: intent.Mutation.Desired,
		Revision: nextRevision, LastCommandID: intent.Envelope.ID,
	}
	outbox := control.OutboxRecord{
		CommandID: intent.Envelope.ID, Revision: nextRevision,
		PayloadDigest: intent.Envelope.CanonicalPayloadDigest, RecordedAt: now,
	}
	acceptance := control.CommandAcceptance{Record: record, DesiredState: desired, Outbox: outbox}
	facts.commands = append(facts.commands, record)
	facts.outbox = append(facts.outbox, outbox)
	facts.desired = desired
	facts.idempotency[idempotencyKey] = acceptance
	return cloneAcceptance(acceptance), nil
}

func (s *FactStore) ReadDesiredState(ctx context.Context, scope core.ExecutionScope) (control.DesiredStateSnapshot, error) {
	if err := contextError(ctx); err != nil {
		return control.DesiredStateSnapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, exists := s.instances[executionKey(scope)]
	if !exists || !sameScope(facts.desired.Scope, scope) {
		return control.DesiredStateSnapshot{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "desired state does not exist")
	}
	return cloneDesired(facts.desired), nil
}

func (s *FactStore) ListCommands(ctx context.Context, scope core.ExecutionScope) ([]control.CommandRecord, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, exists := s.instances[executionKey(scope)]
	if !exists || !sameScope(facts.desired.Scope, scope) {
		return nil, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "desired state does not exist")
	}
	return append([]control.CommandRecord(nil), facts.commands...), nil
}

func (s *FactStore) ListOutbox(ctx context.Context, scope core.ExecutionScope) ([]control.OutboxRecord, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, exists := s.instances[executionKey(scope)]
	if !exists || !sameScope(facts.desired.Scope, scope) {
		return nil, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "desired state does not exist")
	}
	return append([]control.OutboxRecord(nil), facts.outbox...), nil
}

func (s *FactStore) MarkOutboxDispatched(ctx context.Context, scope core.ExecutionScope, commandID string, revision core.Revision) (control.OutboxRecord, error) {
	if err := contextError(ctx); err != nil {
		return control.OutboxRecord{}, err
	}
	if strings.TrimSpace(commandID) == "" || revision == 0 {
		return control.OutboxRecord{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "command id and revision are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, exists := s.instances[executionKey(scope)]
	if !exists || !sameScope(facts.desired.Scope, scope) {
		return control.OutboxRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "desired state does not exist")
	}
	for index := range facts.outbox {
		if facts.outbox[index].CommandID == commandID {
			if facts.outbox[index].Revision != revision {
				return control.OutboxRecord{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "outbox revision does not match command")
			}
			facts.outbox[index].Dispatched = true
			return facts.outbox[index], nil
		}
	}
	return control.OutboxRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "outbox record does not exist")
}

func (s *FactStore) ReserveIdentityLease(ctx context.Context, request control.ReserveIdentityLeaseRequest) (control.IdentityExecutionLease, error) {
	if err := contextError(ctx); err != nil {
		return control.IdentityExecutionLease{}, err
	}
	now := s.clock()
	if err := request.Validate(now); err != nil {
		return control.IdentityExecutionLease{}, err
	}
	key := identityKey(request.TenantID, request.IdentityID)
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, exists := s.leases[key]
	if exists {
		expireLeaseIfNeeded(facts, now)
		current := facts.lease
		if current.State == control.IdentityLeaseReserved && current.ActivationAttemptID == request.ActivationAttemptID &&
			current.Lineage == request.Lineage && current.AuthorityEpoch == request.AuthorityEpoch &&
			request.ExpectedIdentityEpoch+1 == current.Identity.Epoch {
			return current, nil
		}
		if current.State == control.IdentityLeaseReserved || current.State == control.IdentityLeaseActive {
			return control.IdentityExecutionLease{}, core.NewError(core.ErrorConflict, core.ReasonIdentityLeaseConflict, "identity already has a reserved or active holder")
		}
		if current.Identity.Epoch != request.ExpectedIdentityEpoch {
			return control.IdentityExecutionLease{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleIdentityEpoch, "identity epoch does not match current lease fact")
		}
	} else if request.ExpectedIdentityEpoch != 0 {
		return control.IdentityExecutionLease{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleIdentityEpoch, "new identity must start from epoch zero")
	}

	newEpoch := request.ExpectedIdentityEpoch + 1
	lease := control.IdentityExecutionLease{
		ID:       fmt.Sprintf("%s/%s/lease/%d", request.TenantID, request.IdentityID, newEpoch),
		Identity: core.AgentIdentityRef{TenantID: request.TenantID, ID: request.IdentityID, Epoch: newEpoch},
		Lineage:  request.Lineage, ActivationAttemptID: request.ActivationAttemptID,
		State: control.IdentityLeaseReserved, AuthorityEpoch: request.AuthorityEpoch,
		ExpiresAt: request.ExpiresAt, Revision: 1,
	}
	facts = &leaseFacts{lease: lease}
	s.leases[key] = facts
	s.leaseIDs[lease.ID] = facts
	return lease, nil
}

func (s *FactStore) RenewIdentityLease(ctx context.Context, request control.RenewIdentityLeaseRequest) (control.IdentityExecutionLease, error) {
	return s.mutateLease(ctx, request.LeaseID, request.ExpectedRevision, func(lease *control.IdentityExecutionLease, now time.Time) error {
		if request.AuthorityEpoch == 0 || lease.AuthorityEpoch != request.AuthorityEpoch {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleAuthorityEpoch, "authority epoch does not match active lease")
		}
		if lease.State != control.IdentityLeaseActive || !request.ExpiresAt.After(now) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonIdentityLeaseStateInvalid, "renew requires an active lease and future expiry")
		}
		lease.ExpiresAt = request.ExpiresAt
		return nil
	})
}

func (s *FactStore) RevokeIdentityLease(ctx context.Context, request control.EndIdentityLeaseRequest) (control.IdentityExecutionLease, error) {
	return s.mutateLease(ctx, request.LeaseID, request.ExpectedRevision, func(lease *control.IdentityExecutionLease, _ time.Time) error {
		if strings.TrimSpace(request.Reason) == "" || (lease.State != control.IdentityLeaseReserved && lease.State != control.IdentityLeaseActive) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonIdentityLeaseStateInvalid, "revoke requires a reason and reserved or active lease")
		}
		lease.State = control.IdentityLeaseRevoked
		return nil
	})
}

func (s *FactStore) ReleaseIdentityLease(ctx context.Context, request control.EndIdentityLeaseRequest) (control.IdentityExecutionLease, error) {
	return s.mutateLease(ctx, request.LeaseID, request.ExpectedRevision, func(lease *control.IdentityExecutionLease, _ time.Time) error {
		if strings.TrimSpace(request.Reason) == "" || lease.State == control.IdentityLeaseReleased || lease.State == control.IdentityLeaseActive {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonIdentityLeaseStateInvalid, "release requires a reason and a reserved, revoked or expired lease")
		}
		lease.State = control.IdentityLeaseReleased
		return nil
	})
}

func (s *FactStore) InspectIdentityLease(ctx context.Context, tenantID core.TenantID, identityID core.AgentIdentityID) (control.IdentityExecutionLease, error) {
	if err := contextError(ctx); err != nil {
		return control.IdentityExecutionLease{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, exists := s.leases[identityKey(tenantID, identityID)]
	if !exists {
		return control.IdentityExecutionLease{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "identity lease does not exist")
	}
	expireLeaseIfNeeded(facts, s.clock())
	return facts.lease, nil
}

func (s *FactStore) mutateLease(ctx context.Context, leaseID string, expectedRevision core.Revision, mutate func(*control.IdentityExecutionLease, time.Time) error) (control.IdentityExecutionLease, error) {
	if err := contextError(ctx); err != nil {
		return control.IdentityExecutionLease{}, err
	}
	if strings.TrimSpace(leaseID) == "" || expectedRevision == 0 {
		return control.IdentityExecutionLease{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "lease id and expected revision are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, exists := s.leaseIDs[leaseID]
	if !exists {
		return control.IdentityExecutionLease{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "identity lease does not exist")
	}
	now := s.clock()
	expireLeaseIfNeeded(facts, now)
	if facts.lease.Revision != expectedRevision {
		return control.IdentityExecutionLease{}, core.NewError(core.ErrorConflict, core.ReasonStaleLeaseRevision, "lease revision does not match current fact")
	}
	next := facts.lease
	if err := mutate(&next, now); err != nil {
		return control.IdentityExecutionLease{}, err
	}
	next.Revision++
	facts.lease = next
	return next, nil
}

func expireLeaseIfNeeded(facts *leaseFacts, now time.Time) {
	if (facts.lease.State == control.IdentityLeaseReserved || facts.lease.State == control.IdentityLeaseActive) && !now.Before(facts.lease.ExpiresAt) {
		facts.lease.State = control.IdentityLeaseExpired
		facts.lease.Revision++
	}
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context is required")
	}
	return ctx.Err()
}

func executionKey(scope core.ExecutionScope) string {
	return fmt.Sprintf("%s/%s/%s/%d", scope.Identity.TenantID, scope.Identity.ID, scope.Instance.ID, scope.Instance.Epoch)
}

func runKey(scope core.ExecutionScope, runID core.AgentRunID) string {
	return fmt.Sprintf("%s/%s", executionKey(scope), runID)
}

func identityKey(tenantID core.TenantID, identityID core.AgentIdentityID) string {
	return fmt.Sprintf("%s/%s", tenantID, identityID)
}

func commandIdempotencyKey(envelope control.CommandEnvelope) string {
	return fmt.Sprintf("%s/%s/%s/%d/%s/%s", envelope.Target.Identity.TenantID, envelope.Actor, envelope.Target.Instance.ID, envelope.Target.Instance.Epoch, envelope.Kind, envelope.IdempotencyKey)
}

func isNonLifecycleCommand(kind control.CommandKind) bool {
	switch kind {
	case control.CommandProvideInput, control.CommandCancelRun, control.CommandApproveEffect, control.CommandDenyEffect:
		return true
	default:
		return false
	}
}

func sameScope(left, right core.ExecutionScope) bool {
	if left.Identity != right.Identity || left.Lineage != right.Lineage || left.Instance != right.Instance || left.AuthorityEpoch != right.AuthorityEpoch {
		return false
	}
	if left.SandboxLease == nil || right.SandboxLease == nil {
		return left.SandboxLease == nil && right.SandboxLease == nil
	}
	return *left.SandboxLease == *right.SandboxLease
}

func cloneScope(scope core.ExecutionScope) core.ExecutionScope {
	clone := scope
	if scope.SandboxLease != nil {
		lease := *scope.SandboxLease
		clone.SandboxLease = &lease
	}
	return clone
}

func cloneDesired(snapshot control.DesiredStateSnapshot) control.DesiredStateSnapshot {
	snapshot.Scope = cloneScope(snapshot.Scope)
	return snapshot
}

func cloneAcceptance(acceptance control.CommandAcceptance) control.CommandAcceptance {
	acceptance.DesiredState = cloneDesired(acceptance.DesiredState)
	acceptance.Record.Envelope.Target = cloneScope(acceptance.Record.Envelope.Target)
	return acceptance
}

func cloneRun(record core.AgentRunRecord) core.AgentRunRecord {
	record.Scope = cloneScope(record.Scope)
	if record.CompletionClaim != nil {
		claim := *record.CompletionClaim
		record.CompletionClaim = &claim
	}
	return record
}

var _ control.CommandFactPort = (*FactStore)(nil)
var _ control.IdentityLeaseFactPort = (*FactStore)(nil)
var _ control.RunFactPort = (*FactStore)(nil)

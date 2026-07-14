// Package fakes provides deterministic Application test fixtures. They make no
// production durability, topology or SLA claim.
package fakes

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type FactStoreV2 struct {
	mu                         sync.Mutex
	Clock                      func() time.Time
	submissions                map[string]contract.SubmissionBundleV2
	journals                   map[string]contract.WorkflowJournalV2
	descriptors                map[runtimeports.NamespacedNameV2]applicationports.StepKindDescriptorV2
	claims                     map[string]applicationports.WorkflowJournalClaimV2
	LoseNextSubmissionReply    bool
	LoseNextJournalCreateReply bool
	LoseNextJournalCASReply    bool
	LoseNextClaimReply         bool
	LoseNextClaimReleaseReply  bool
}

func NewFactStoreV2() *FactStoreV2 {
	return &FactStoreV2{Clock: time.Now, submissions: make(map[string]contract.SubmissionBundleV2), journals: make(map[string]contract.WorkflowJournalV2), descriptors: make(map[runtimeports.NamespacedNameV2]applicationports.StepKindDescriptorV2), claims: make(map[string]applicationports.WorkflowJournalClaimV2)}
}

var _ applicationports.SubmissionFactPortV2 = (*FactStoreV2)(nil)
var _ applicationports.WorkflowJournalFactPortV2 = (*FactStoreV2)(nil)
var _ applicationports.WorkflowJournalRecoveryPortV2 = (*FactStoreV2)(nil)
var _ applicationports.StepCatalogV2 = (*FactStoreV2)(nil)

func (s *FactStoreV2) CreateSubmissionBundleV2(_ context.Context, bundle contract.SubmissionBundleV2) (contract.SubmissionBundleV2, error) {
	if s.Clock == nil {
		return contract.SubmissionBundleV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "application fake clock is required")
	}
	if err := bundle.Validate(s.Clock()); err != nil {
		return contract.SubmissionBundleV2{}, err
	}
	digest, err := submissionDigestV2(bundle)
	if err != nil {
		return contract.SubmissionBundleV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := scopedApplicationKeyV2(bundle.Command.Target, bundle.Command.ID)
	if current, ok := s.submissions[key]; ok {
		currentDigest, _ := submissionDigestV2(current)
		if currentDigest == digest {
			return cloneSubmissionV2(current), nil
		}
		return contract.SubmissionBundleV2{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "submission bundle already binds different content")
	}
	s.submissions[key] = cloneSubmissionV2(bundle)
	if s.LoseNextSubmissionReply {
		s.LoseNextSubmissionReply = false
		return contract.SubmissionBundleV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected submission reply loss")
	}
	return cloneSubmissionV2(bundle), nil
}

func (s *FactStoreV2) InspectSubmissionBundleV2(_ context.Context, scope core.ExecutionScope, commandID string) (contract.SubmissionBundleV2, error) {
	if err := scope.Validate(); err != nil {
		return contract.SubmissionBundleV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.submissions[scopedApplicationKeyV2(scope, commandID)]
	if !ok {
		return contract.SubmissionBundleV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "submission bundle not found")
	}
	return cloneSubmissionV2(current), nil
}

func (s *FactStoreV2) CreateWorkflowJournalV2(_ context.Context, plan contract.WorkflowPlanV2, journal contract.WorkflowJournalV2) (contract.WorkflowJournalV2, error) {
	if err := journal.ValidateFor(plan); err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	digest, err := journal.DigestV2(plan)
	if err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := scopedApplicationKeyV2(plan.Target, journal.ID)
	if current, ok := s.journals[key]; ok {
		currentDigest, _ := current.DigestV2(plan)
		if currentDigest == digest {
			return cloneJournalV2(current), nil
		}
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "workflow journal already binds different content")
	}
	for existingKey, current := range s.journals {
		if existingKey != key && current.CommandID == journal.CommandID && strings.HasPrefix(existingKey, scopedApplicationKeyV2(plan.Target, "")) {
			return contract.WorkflowJournalV2{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "command already has a workflow journal")
		}
	}
	s.journals[key] = cloneJournalV2(journal)
	if s.LoseNextJournalCreateReply {
		s.LoseNextJournalCreateReply = false
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected journal create reply loss")
	}
	return cloneJournalV2(journal), nil
}

func (s *FactStoreV2) InspectWorkflowJournalV2(_ context.Context, scope core.ExecutionScope, id string) (contract.WorkflowJournalV2, error) {
	if err := scope.Validate(); err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.journals[scopedApplicationKeyV2(scope, id)]
	if !ok {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "workflow journal not found")
	}
	return cloneJournalV2(current), nil
}

func (s *FactStoreV2) CompareAndSwapWorkflowJournalV2(_ context.Context, plan contract.WorkflowPlanV2, request applicationports.WorkflowJournalCASRequestV2) (contract.WorkflowJournalV2, error) {
	if request.ExpectedRevision == 0 || request.Next.Revision != request.ExpectedRevision+1 {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "workflow journal CAS revisions must be consecutive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := scopedApplicationKeyV2(plan.Target, request.Next.ID)
	current, ok := s.journals[key]
	if !ok {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "workflow journal not found")
	}
	if current.Revision != request.ExpectedRevision {
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "workflow journal revision changed")
	}
	if err := contract.ValidateWorkflowJournalTransitionV2(plan, current, request.Next); err != nil {
		return contract.WorkflowJournalV2{}, err
	}
	s.journals[key] = cloneJournalV2(request.Next)
	if s.LoseNextJournalCASReply {
		s.LoseNextJournalCASReply = false
		return contract.WorkflowJournalV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected journal CAS reply loss")
	}
	return cloneJournalV2(request.Next), nil
}

func (s *FactStoreV2) ListWorkflowJournalsV2(_ context.Context, scope core.ExecutionScope, afterID string, limit uint16) ([]contract.WorkflowJournalV2, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if limit == 0 || limit > 512 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "workflow journal list limit must be between 1 and 512")
	}
	prefix := scopedApplicationKeyV2(scope, "")
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0)
	byID := make(map[string]contract.WorkflowJournalV2)
	for key, journal := range s.journals {
		if !strings.HasPrefix(key, prefix) || journal.ID <= afterID || journal.Status == contract.WorkflowCompletedV2 {
			continue
		}
		ids = append(ids, journal.ID)
		byID[journal.ID] = journal
	}
	sort.Strings(ids)
	if len(ids) > int(limit) {
		ids = ids[:limit]
	}
	result := make([]contract.WorkflowJournalV2, len(ids))
	for index, id := range ids {
		result[index] = cloneJournalV2(byID[id])
	}
	return result, nil
}

func (s *FactStoreV2) ClaimWorkflowJournalV2(_ context.Context, request applicationports.WorkflowJournalClaimRequestV2) (applicationports.WorkflowJournalClaimV2, error) {
	if s.Clock == nil || strings.TrimSpace(request.JournalID) == "" || strings.TrimSpace(request.OwnerID) == "" || len(request.OwnerID) > 256 || request.LeaseNanos <= 0 {
		return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "workflow journal claim request is incomplete")
	}
	if err := request.Scope.Validate(); err != nil {
		return applicationports.WorkflowJournalClaimV2{}, err
	}
	if err := request.PolicyDigest.Validate(); err != nil {
		return applicationports.WorkflowJournalClaimV2{}, err
	}
	now := s.Clock().UnixNano()
	if request.LeaseNanos > int64(^uint64(0)>>1)-now {
		return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "workflow journal claim lifetime overflows")
	}
	key := scopedApplicationKeyV2(request.Scope, request.JournalID)
	s.mu.Lock()
	defer s.mu.Unlock()
	journal, ok := s.journals[key]
	if !ok {
		return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "workflow journal not found")
	}
	if journal.Status == contract.WorkflowCompletedV2 {
		return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "completed workflow journal cannot be claimed")
	}
	current, exists := s.claims[key]
	if !exists {
		if request.ExpectedRevision != 0 {
			return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "workflow journal claim does not yet exist")
		}
		current = applicationports.WorkflowJournalClaimV2{Scope: cloneScopeV2(request.Scope), JournalID: request.JournalID, PlanDigest: journal.PlanDigest, OwnerID: request.OwnerID, PolicyDigest: request.PolicyDigest, Epoch: 1, Revision: 1, State: applicationports.WorkflowJournalClaimActiveV2, AcquiredUnixNano: now, UpdatedUnixNano: now, ExpiresUnixNano: now + request.LeaseNanos}
	} else {
		if current.Revision != request.ExpectedRevision {
			return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "workflow journal claim revision changed")
		}
		active := current.State == applicationports.WorkflowJournalClaimActiveV2 && now < current.ExpiresUnixNano
		if active && current.OwnerID != request.OwnerID {
			return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "workflow journal is actively claimed by another worker")
		}
		if active && current.PolicyDigest != request.PolicyDigest {
			return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonSupervisionPolicyDrift, "active workflow claim policy drifted")
		}
		epoch := current.Epoch
		acquired := current.AcquiredUnixNano
		if !active {
			epoch++
			acquired = now
		}
		current = applicationports.WorkflowJournalClaimV2{Scope: cloneScopeV2(request.Scope), JournalID: request.JournalID, PlanDigest: journal.PlanDigest, OwnerID: request.OwnerID, PolicyDigest: request.PolicyDigest, Epoch: epoch, Revision: current.Revision + 1, State: applicationports.WorkflowJournalClaimActiveV2, AcquiredUnixNano: acquired, UpdatedUnixNano: now, ExpiresUnixNano: now + request.LeaseNanos}
	}
	if err := current.ValidateFor(journal); err != nil {
		return applicationports.WorkflowJournalClaimV2{}, err
	}
	s.claims[key] = current
	if s.LoseNextClaimReply {
		s.LoseNextClaimReply = false
		return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected workflow claim reply loss")
	}
	return cloneWorkflowClaimV2(current), nil
}

func (s *FactStoreV2) InspectWorkflowJournalClaimV2(_ context.Context, scope core.ExecutionScope, journalID string) (applicationports.WorkflowJournalClaimV2, error) {
	if err := scope.Validate(); err != nil {
		return applicationports.WorkflowJournalClaimV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.claims[scopedApplicationKeyV2(scope, journalID)]
	if !ok {
		return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "workflow journal claim not found")
	}
	return cloneWorkflowClaimV2(current), nil
}

func (s *FactStoreV2) ReleaseWorkflowJournalClaimV2(_ context.Context, request applicationports.WorkflowJournalReleaseRequestV2) (applicationports.WorkflowJournalClaimV2, error) {
	if s.Clock == nil || strings.TrimSpace(request.JournalID) == "" || strings.TrimSpace(request.OwnerID) == "" || request.Epoch == 0 || request.ExpectedRevision == 0 {
		return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "workflow journal claim release request is incomplete")
	}
	if err := request.Scope.Validate(); err != nil {
		return applicationports.WorkflowJournalClaimV2{}, err
	}
	key := scopedApplicationKeyV2(request.Scope, request.JournalID)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.claims[key]
	if !ok {
		return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "workflow journal claim not found")
	}
	if current.Revision != request.ExpectedRevision || current.OwnerID != request.OwnerID || current.Epoch != request.Epoch {
		return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "only the exact current workflow claim can be released")
	}
	if current.State == applicationports.WorkflowJournalClaimReleasedV2 {
		return cloneWorkflowClaimV2(current), nil
	}
	current.State = applicationports.WorkflowJournalClaimReleasedV2
	current.Revision++
	current.UpdatedUnixNano = s.Clock().UnixNano()
	s.claims[key] = current
	if s.LoseNextClaimReleaseReply {
		s.LoseNextClaimReleaseReply = false
		return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected workflow claim release reply loss")
	}
	return cloneWorkflowClaimV2(current), nil
}

func (s *FactStoreV2) RegisterStepDescriptorV2(descriptor applicationports.StepKindDescriptorV2) error {
	if err := descriptor.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.descriptors[descriptor.Kind]; ok {
		currentDigest, _ := current.DigestV2()
		nextDigest, _ := descriptor.DigestV2()
		if currentDigest == nextDigest {
			return nil
		}
		return core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "step kind already binds a different descriptor")
	}
	s.descriptors[descriptor.Kind] = cloneDescriptorV2(descriptor)
	return nil
}

func (s *FactStoreV2) ResolveStepKindV2(_ context.Context, kind runtimeports.NamespacedNameV2) (applicationports.StepKindDescriptorV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	descriptor, ok := s.descriptors[kind]
	if !ok {
		return applicationports.StepKindDescriptorV2{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownCapability, "workflow step kind is not registered")
	}
	return cloneDescriptorV2(descriptor), nil
}

func submissionDigestV2(bundle contract.SubmissionBundleV2) (core.Digest, error) {
	commandDigest, err := core.CanonicalJSONDigest("praxis.application.workflow", contract.WorkflowContractVersionV2, "CommandEnvelope", bundle.Command)
	if err != nil {
		return "", err
	}
	payloadDigest, err := bundle.Payload.DigestV2()
	if err != nil {
		return "", err
	}
	planDigest, err := bundle.Plan.DigestV2()
	if err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.application.workflow", contract.WorkflowContractVersionV2, "SubmissionBundleV2", struct {
		Command core.Digest `json:"command_digest"`
		Payload core.Digest `json:"payload_digest"`
		Plan    core.Digest `json:"plan_digest"`
	}{commandDigest, payloadDigest, planDigest})
}

func cloneSubmissionV2(value contract.SubmissionBundleV2) contract.SubmissionBundleV2 {
	clone := value
	clone.Command.Target = cloneScopeV2(value.Command.Target)
	if value.Command.Preconditions.LeaseEpoch != nil {
		epoch := *value.Command.Preconditions.LeaseEpoch
		clone.Command.Preconditions.LeaseEpoch = &epoch
	}
	clone.Payload.Payload.Inline = append([]byte(nil), value.Payload.Payload.Inline...)
	clone.Plan = clonePlanV2(value.Plan)
	return clone
}

func clonePlanV2(value contract.WorkflowPlanV2) contract.WorkflowPlanV2 {
	clone := value
	clone.Target = cloneScopeV2(value.Target)
	clone.Steps = make([]contract.WorkflowStepV2, len(value.Steps))
	for index, step := range value.Steps {
		clone.Steps[index] = step
		clone.Steps[index].Dependencies = append([]string(nil), step.Dependencies...)
		clone.Steps[index].Payload.Inline = append([]byte(nil), step.Payload.Inline...)
		if step.Provider != nil {
			provider := *step.Provider
			clone.Steps[index].Provider = &provider
		}
		if step.DomainAdapter != nil {
			domain := *step.DomainAdapter
			clone.Steps[index].DomainAdapter = &domain
		}
	}
	return clone
}

func cloneJournalV2(value contract.WorkflowJournalV2) contract.WorkflowJournalV2 {
	clone := value
	clone.Steps = make([]contract.WorkflowStepProgressV2, len(value.Steps))
	for index, step := range value.Steps {
		clone.Steps[index] = step
		if step.Effect != nil {
			fact := *step.Effect
			clone.Steps[index].Effect = &fact
		}
		if step.Settlement != nil {
			fact := *step.Settlement
			clone.Steps[index].Settlement = &fact
		}
	}
	return clone
}

func cloneDescriptorV2(value applicationports.StepKindDescriptorV2) applicationports.StepKindDescriptorV2 {
	clone := value
	clone.Schemas = append([]runtimeports.SchemaRefV2(nil), value.Schemas...)
	return clone
}

func cloneScopeV2(value core.ExecutionScope) core.ExecutionScope {
	clone := value
	if value.SandboxLease != nil {
		lease := *value.SandboxLease
		clone.SandboxLease = &lease
	}
	return clone
}

func cloneWorkflowClaimV2(value applicationports.WorkflowJournalClaimV2) applicationports.WorkflowJournalClaimV2 {
	clone := value
	clone.Scope = cloneScopeV2(value.Scope)
	return clone
}

func scopedApplicationKeyV2(scope core.ExecutionScope, id string) string {
	digest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	return string(digest) + "\x00" + id
}

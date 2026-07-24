package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ControlledOperationProviderEntryStoreV2 is a deterministic reference Fact
// Owner. It is a test/conformance backend, not a production durability or SLA
// claim.
type ControlledOperationProviderEntryStoreV2 struct {
	mu                  sync.Mutex
	clock               func() time.Time
	entries             map[string]map[string]control.ControlledOperationProviderEntryFactV2
	loseNextCreateReply bool
	loseNextCASReply    bool
}

func NewControlledOperationProviderEntryStoreV2(clock func() time.Time) *ControlledOperationProviderEntryStoreV2 {
	if clock == nil {
		clock = time.Now
	}
	return &ControlledOperationProviderEntryStoreV2{clock: clock, entries: map[string]map[string]control.ControlledOperationProviderEntryFactV2{}}
}

func (s *ControlledOperationProviderEntryStoreV2) LoseNextControlledOperationProviderCreateReplyV2() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextCreateReply = true
}

func (s *ControlledOperationProviderEntryStoreV2) LoseNextControlledOperationProviderCASReplyV2() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextCASReply = true
}

func (s *ControlledOperationProviderEntryStoreV2) CreateControlledOperationProviderEntryV2(ctx context.Context, fact control.ControlledOperationProviderEntryFactV2) (control.CreateControlledOperationProviderEntryResultV2, error) {
	if err := contextError(ctx); err != nil {
		return control.CreateControlledOperationProviderEntryResultV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return control.CreateControlledOperationProviderEntryResultV2{}, err
	}
	key, err := controlledOperationProviderPartitionV2(fact.Request.Operation)
	if err != nil {
		return control.CreateControlledOperationProviderEntryResultV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.entries[key][fact.EntryID]; ok {
		if !control.SameControlledOperationProviderEntryImmutableV2(existing, fact) {
			return control.CreateControlledOperationProviderEntryResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "controlled Provider Entry ID binds different canonical content")
		}
		return control.NewCreateControlledOperationProviderEntryResultV2(existing, false, "")
	}
	if s.entries[key] == nil {
		s.entries[key] = map[string]control.ControlledOperationProviderEntryFactV2{}
	}
	s.entries[key][fact.EntryID] = fact
	nonce, err := core.CanonicalJSONDigest("praxis.runtime.controlled-operation-provider", ports.ControlledOperationProviderContractVersionV2, "ControlledOperationProviderOpaqueClaimV2", struct {
		EntryID     string `json:"entry_id"`
		EnteredTime int64  `json:"entered_unix_nano"`
	}{fact.EntryID, fact.EnteredUnixNano})
	if err != nil {
		delete(s.entries[key], fact.EntryID)
		return control.CreateControlledOperationProviderEntryResultV2{}, err
	}
	if s.loseNextCreateReply {
		s.loseNextCreateReply = false
		return control.CreateControlledOperationProviderEntryResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected controlled Provider Entry create reply loss")
	}
	return control.NewCreateControlledOperationProviderEntryResultV2(fact, true, nonce)
}

func (s *ControlledOperationProviderEntryStoreV2) InspectControlledOperationProviderEntryV2(ctx context.Context, operation ports.OperationSubjectV3, entryID string) (control.ControlledOperationProviderEntryFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.ControlledOperationProviderEntryFactV2{}, err
	}
	if entryID == "" {
		return control.ControlledOperationProviderEntryFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider Entry ID is empty")
	}
	key, err := controlledOperationProviderPartitionV2(operation)
	if err != nil {
		return control.ControlledOperationProviderEntryFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.entries[key][entryID]
	if !ok {
		return control.ControlledOperationProviderEntryFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "controlled Provider Entry not found")
	}
	if err := fact.Validate(); err != nil {
		return control.ControlledOperationProviderEntryFactV2{}, err
	}
	return fact, nil
}

func (s *ControlledOperationProviderEntryStoreV2) CompareAndSwapControlledOperationProviderEntryV2(ctx context.Context, operation ports.OperationSubjectV3, request control.ControlledOperationProviderEntryCASRequestV2) (control.ControlledOperationProviderEntryFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.ControlledOperationProviderEntryFactV2{}, err
	}
	key, err := controlledOperationProviderPartitionV2(operation)
	if err != nil {
		return control.ControlledOperationProviderEntryFactV2{}, err
	}
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.entries[key][request.Next.EntryID]
	if !ok {
		return control.ControlledOperationProviderEntryFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "controlled Provider Entry not found")
	}
	if current.Revision != request.ExpectedRevision {
		if current.Digest == request.Next.Digest {
			return current, nil
		}
		return control.ControlledOperationProviderEntryFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "controlled Provider Entry CAS conflicts")
	}
	if err := control.ValidateControlledOperationProviderEntryTransitionV2(current, request.Next, now); err != nil {
		return control.ControlledOperationProviderEntryFactV2{}, err
	}
	s.entries[key][request.Next.EntryID] = request.Next
	if s.loseNextCASReply {
		s.loseNextCASReply = false
		return control.ControlledOperationProviderEntryFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected controlled Provider Entry CAS reply loss")
	}
	return request.Next, nil
}

func controlledOperationProviderPartitionV2(operation ports.OperationSubjectV3) (string, error) {
	if err := operation.Validate(); err != nil {
		return "", err
	}
	digest, err := operation.DigestV3()
	if err != nil {
		return "", err
	}
	return string(operation.ExecutionScope.Identity.TenantID) + "\x00" + string(digest), nil
}

var _ control.ControlledOperationProviderEntryFactPortV2 = (*ControlledOperationProviderEntryStoreV2)(nil)

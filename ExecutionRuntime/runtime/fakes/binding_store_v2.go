package fakes

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// BindingStoreV2 is a deterministic in-memory conformance fixture. It is not
// a production persistence backend and advertises no production conformance.
type BindingStoreV2 struct {
	mu                  sync.Mutex
	clock               func() time.Time
	bindings            map[string]control.BindingFactV2
	sets                map[string]control.BindingSetFactV2
	renewalEvidence     control.BindingRenewalAttestationReaderV2
	loseNextCommitReply bool
}

func (s *BindingStoreV2) SetRenewalAttestations(reader control.BindingRenewalAttestationReaderV2) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renewalEvidence = reader
}

func NewBindingStoreV2(clock func() time.Time) *BindingStoreV2 {
	if clock == nil {
		clock = time.Now
	}
	return &BindingStoreV2{clock: clock, bindings: make(map[string]control.BindingFactV2), sets: make(map[string]control.BindingSetFactV2)}
}

func (s *BindingStoreV2) LoseNextCommitReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextCommitReply = true
}

func (s *BindingStoreV2) CreateBinding(ctx context.Context, fact control.BindingFactV2) (control.BindingFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.BindingFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return control.BindingFactV2{}, err
	}
	if fact.Revision != 1 || fact.State != control.BindingDeclared {
		return control.BindingFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "new binding fact must be declared at revision one")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.bindings[fact.ID]; exists {
		return control.BindingFactV2{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "binding fact already exists")
	}
	s.bindings[fact.ID] = cloneBindingFactV2(fact)
	return cloneBindingFactV2(fact), nil
}

func (s *BindingStoreV2) InspectBinding(ctx context.Context, id string) (control.BindingFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.BindingFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, exists := s.bindings[id]
	if !exists {
		return control.BindingFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "binding fact does not exist")
	}
	return cloneBindingFactV2(fact), nil
}

func (s *BindingStoreV2) CompareAndSwapBinding(ctx context.Context, request control.BindingFactCASRequestV2) (control.BindingFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.BindingFactV2{}, err
	}
	now := s.clock()
	if err := request.Validate(now); err != nil {
		return control.BindingFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.bindings[request.Next.ID]
	if !exists {
		return control.BindingFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "binding fact does not exist")
	}
	if current.Revision != request.ExpectedRevision {
		return control.BindingFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "binding fact revision does not match CAS precondition")
	}
	if err := control.ValidateBindingFactTransitionV2(current, request.Next, now); err != nil {
		return control.BindingFactV2{}, err
	}
	next := cloneBindingFactV2(request.Next)
	s.bindings[next.ID] = next
	return cloneBindingFactV2(next), nil
}

func (s *BindingStoreV2) CommitBindingSet(ctx context.Context, request control.CommitBindingSetRequestV2) (control.BindingSetFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.BindingSetFactV2{}, err
	}
	if err := request.Set.Validate(); err != nil {
		return control.BindingSetFactV2{}, err
	}
	if request.Set.State != control.BindingSetActive || request.Set.Revision != 1 || len(request.Expected) != len(request.Set.Members) {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonBindingSetConflict, "binding set commit requires an active revision-one set and one expected revision per member")
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() < request.Set.CreatedUnixNano || !now.Before(time.Unix(0, request.Set.ExpiresUnixNano)) {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "binding set commit clock regressed or reached expiry")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, exists := s.sets[request.Set.ID]; exists {
		if bindingSetCommitReplayV2(existing, request) {
			return cloneBindingSetFactV2(existing), nil
		}
		return control.BindingSetFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "binding set id already exists with different content")
	}
	expected := make(map[string]core.Revision, len(request.Expected))
	for _, item := range request.Expected {
		if item.BindingID == "" || item.ExpectedRevision == 0 {
			return control.BindingSetFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "expected binding revision is incomplete")
		}
		if _, exists := expected[item.BindingID]; exists {
			return control.BindingSetFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "expected binding revision set contains a duplicate")
		}
		expected[item.BindingID] = item.ExpectedRevision
	}
	nextFacts := make(map[string]control.BindingFactV2, len(request.Set.Members))
	for _, member := range request.Set.Members {
		fact, exists := s.bindings[member.BindingID]
		expectedRevision, expectedExists := expected[member.BindingID]
		if !exists || !expectedExists || fact.Revision != expectedRevision || member.BindingRevision != expectedRevision || fact.State != control.BindingCertified || fact.ComponentID != member.ComponentID || fact.ManifestDigest != member.ManifestDigest || fact.Manifest.ArtifactDigest != member.ArtifactDigest || fact.GovernanceDigest != request.Set.GovernanceDigest {
			return control.BindingSetFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "binding member is stale, uncertified or drifted")
		}
		if !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
			return control.BindingSetFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "binding member expired before atomic commit")
		}
		next := fact
		next.State = control.BindingBound
		next.Revision++
		next.BindingSetID = request.Set.ID
		if err := control.ValidateBindingFactTransitionV2(fact, next, now); err != nil {
			return control.BindingSetFactV2{}, err
		}
		nextFacts[member.BindingID] = next
	}
	set := cloneBindingSetFactV2(request.Set)
	for index, member := range request.Set.Members {
		s.bindings[member.BindingID] = cloneBindingFactV2(nextFacts[member.BindingID])
		set.Members[index].BindingRevision = nextFacts[member.BindingID].Revision
	}
	// Binding member watermark always means the current post-commit Binding
	// revision, including the initial Certified->Bound transition.
	if err := set.Validate(); err != nil {
		return control.BindingSetFactV2{}, err
	}
	s.sets[set.ID] = set
	if s.loseNextCommitReply {
		s.loseNextCommitReply = false
		return control.BindingSetFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected binding set commit reply loss")
	}
	return cloneBindingSetFactV2(set), nil
}

func (s *BindingStoreV2) InspectBindingSet(ctx context.Context, id string) (control.BindingSetFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.BindingSetFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	set, exists := s.sets[id]
	if !exists {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "binding set does not exist")
	}
	return cloneBindingSetFactV2(set), nil
}

func (s *BindingStoreV2) CompareAndSwapBindingSet(ctx context.Context, request control.BindingSetCASRequestV2) (control.BindingSetFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.BindingSetFactV2{}, err
	}
	if request.ExpectedRevision == 0 || request.Next.Revision != request.ExpectedRevision+1 {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "binding set CAS requires the next consecutive revision")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.sets[request.Next.ID]
	if !exists {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "binding set does not exist")
	}
	if current.Revision != request.ExpectedRevision {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "binding set revision does not match CAS precondition")
	}
	if err := control.ValidateBindingSetTransitionV2(current, request.Next); err != nil {
		return control.BindingSetFactV2{}, err
	}
	next := cloneBindingSetFactV2(request.Next)
	s.sets[next.ID] = next
	return cloneBindingSetFactV2(next), nil
}

// RenewBindingSetV2 atomically advances every governed Bound member and the
// BindingSet lease. The in-memory transaction is a conformance fake, not a
// production storage claim.
func (s *BindingStoreV2) RenewBindingSetV2(ctx context.Context, request control.RenewBindingSetRequestV2) (control.BindingSetFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.BindingSetFactV2{}, err
	}
	if request.ExpectedSetRevision == 0 || request.NextSet.Revision != request.ExpectedSetRevision+1 || len(request.NextBindings) != len(request.NextSet.Members) {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "governed Binding renewal requires consecutive Set and member revisions")
	}
	if err := request.NextSet.Validate(); err != nil {
		return control.BindingSetFactV2{}, err
	}
	now := s.clock()
	s.mu.Lock()
	attestations := s.renewalEvidence
	s.mu.Unlock()
	if attestations == nil {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Binding renewal requires an independent Attestation reader")
	}
	verified := make(map[string]control.BindingRenewalAttestationV2, len(request.NextBindings))
	for _, nextFact := range request.NextBindings {
		if len(nextFact.RenewalEvidence) == 0 {
			return control.BindingSetFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonBindingNotCertified, "Binding renewal lacks appended certification Evidence")
		}
		ref := nextFact.RenewalEvidence[len(nextFact.RenewalEvidence)-1]
		attestation, err := attestations.InspectBindingRenewalAttestationV2(ctx, ref)
		if err != nil {
			return control.BindingSetFactV2{}, err
		}
		grantDigest, err := control.BindingGrantSetDigestV2(nextFact.Grants)
		if err != nil || attestation.Validate() != nil || attestation.Evidence != ref || attestation.BindingID != nextFact.ID || attestation.ComponentID != nextFact.ComponentID || attestation.ManifestDigest != nextFact.ManifestDigest || attestation.GrantSetDigest != grantDigest || attestation.SourceSequence != ref.Sequence || now.Before(time.Unix(0, attestation.ObservedUnixNano)) || !now.Before(time.Unix(0, attestation.ExpiresUnixNano)) {
			return control.BindingSetFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonBindingNotCertified, "Binding renewal Attestation is stale, mismatched or untrusted")
		}
		verified[nextFact.ID] = attestation
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	currentSet, exists := s.sets[request.NextSet.ID]
	if !exists {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "BindingSet does not exist")
	}
	if currentSet.Revision == request.NextSet.Revision {
		if bindingSetEqualV2(currentSet, request.NextSet) {
			return cloneBindingSetFactV2(currentSet), nil
		}
		return control.BindingSetFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "same renewal revision has different content")
	}
	if currentSet.Revision != request.ExpectedSetRevision || currentSet.State != control.BindingSetActive || request.NextSet.State != control.BindingSetActive {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "BindingSet renewal precondition is stale or inactive")
	}
	currentSemantic, currentErr := control.BindingSetSemanticDigestV2(currentSet)
	nextSemantic, nextErr := control.BindingSetSemanticDigestV2(request.NextSet)
	if currentErr != nil || nextErr != nil || currentSemantic != nextSemantic || request.NextSet.ExpiresUnixNano <= currentSet.ExpiresUnixNano {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "BindingSet renewal changed stable semantics or failed to extend TTL")
	}
	nextByID := make(map[string]control.BindingFactV2, len(request.NextBindings))
	for _, nextFact := range request.NextBindings {
		if _, duplicate := nextByID[nextFact.ID]; duplicate {
			return control.BindingSetFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "Binding renewal contains a duplicate member")
		}
		currentFact, exists := s.bindings[nextFact.ID]
		if !exists {
			return control.BindingSetFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Binding renewal member does not exist")
		}
		if err := control.ValidateBindingFactRenewalV2(currentFact, nextFact, now); err != nil {
			return control.BindingSetFactV2{}, err
		}
		nextByID[nextFact.ID] = cloneBindingFactV2(nextFact)
	}
	for _, member := range request.NextSet.Members {
		nextFact, exists := nextByID[member.BindingID]
		attestation := verified[member.BindingID]
		certifierSet, certifierExists := s.sets[attestation.Certifier.BindingSetID]
		certifierCurrent := certifierExists && certifierSet.State == control.BindingSetActive && certifierSet.Revision == attestation.Certifier.BindingSetRevision && now.Before(time.Unix(0, certifierSet.ExpiresUnixNano)) && bindingSetContainsExactProducerV2(certifierSet, attestation.Certifier)
		if !exists || !certifierCurrent || member.BindingRevision != nextFact.Revision || member.ComponentID != nextFact.ComponentID || member.ManifestDigest != nextFact.ManifestDigest || member.ArtifactDigest != nextFact.Manifest.ArtifactDigest || !reflect.DeepEqual(member.Contract, nextFact.Manifest.Contract) || !reflect.DeepEqual(member.Owners, nextFact.Manifest.Owners) || !reflect.DeepEqual(member.Grants, nextFact.Grants) {
			return control.BindingSetFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "renewed BindingSet member does not exactly match its current Binding Fact")
		}
	}
	for id, fact := range nextByID {
		s.bindings[id] = cloneBindingFactV2(fact)
	}
	s.sets[request.NextSet.ID] = cloneBindingSetFactV2(request.NextSet)
	return cloneBindingSetFactV2(request.NextSet), nil
}

func bindingSetContainsExactProducerV2(set control.BindingSetFactV2, producer ports.EvidenceProducerBindingRefV2) bool {
	for _, member := range set.Members {
		if member.ComponentID != producer.ComponentID || member.ManifestDigest != producer.ManifestDigest || member.ArtifactDigest != producer.ArtifactDigest {
			continue
		}
		for _, grant := range member.Grants {
			if grant.Capability == producer.Capability {
				return true
			}
		}
	}
	return false
}

func bindingSetCommitReplayV2(existing control.BindingSetFactV2, request control.CommitBindingSetRequestV2) bool {
	if existing.ID != request.Set.ID || existing.PlanID != request.Set.PlanID || existing.PlanDigest != request.Set.PlanDigest || existing.GovernanceDigest != request.Set.GovernanceDigest || existing.State != request.Set.State || existing.CreatedUnixNano != request.Set.CreatedUnixNano || existing.ExpiresUnixNano != request.Set.ExpiresUnixNano || len(existing.Members) != len(request.Set.Members) {
		return false
	}
	for index := range existing.Members {
		left, right := existing.Members[index], request.Set.Members[index]
		if left.BindingRevision != right.BindingRevision+1 {
			return false
		}
		left.BindingRevision = right.BindingRevision
		if !reflect.DeepEqual(left, right) {
			return false
		}
	}
	return reflect.DeepEqual(existing.TopologicalOrder, request.Set.TopologicalOrder) && reflect.DeepEqual(existing.Residuals, request.Set.Residuals)
}

func cloneBindingFactV2(fact control.BindingFactV2) control.BindingFactV2 {
	fact.Manifest = cloneManifestForBindingStore(fact.Manifest)
	fact.Grants = append([]ports.CapabilityGrantV2(nil), fact.Grants...)
	fact.RenewalEvidence = append([]ports.EvidenceRecordRefV2(nil), fact.RenewalEvidence...)
	return fact
}

func cloneManifestForBindingStore(manifest ports.ComponentManifestV2) ports.ComponentManifestV2 {
	payload, _ := ports.EncodeComponentManifestV2(manifest)
	cloned, _ := ports.DecodeComponentManifestV2(payload)
	return cloned
}

func cloneBindingSetFactV2(set control.BindingSetFactV2) control.BindingSetFactV2 {
	set.Members = append([]control.BindingMemberV2(nil), set.Members...)
	for index := range set.Members {
		set.Members[index].Owners = append([]ports.OwnerAssignmentV2(nil), set.Members[index].Owners...)
		set.Members[index].Grants = append([]ports.CapabilityGrantV2(nil), set.Members[index].Grants...)
	}
	set.TopologicalOrder = append([]ports.ComponentIDV2(nil), set.TopologicalOrder...)
	set.Residuals = append([]control.BindingResidualV2(nil), set.Residuals...)
	return set
}

func bindingSetEqualV2(left, right control.BindingSetFactV2) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.binding", ports.BindingContractVersionV2, "BindingSetFactV2", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.binding", ports.BindingContractVersionV2, "BindingSetFactV2", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

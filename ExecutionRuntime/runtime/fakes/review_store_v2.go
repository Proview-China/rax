package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ReviewStoreV2 is a deterministic in-memory Review fact owner for tests. It
// is not a production backend and its raw methods are not Application-facing
// governance entry points.
type ReviewStoreV2 struct {
	mu                    sync.Mutex
	clock                 func() time.Time
	cases                 map[string]ports.ReviewCaseFactV2
	verdicts              map[string]ports.ReviewVerdictFactV2
	satisfactions         map[string]ports.ConditionSatisfactionFactV2
	satisfactionByVerdict map[string]string
	loseNextCreateReply   bool
	loseNextDecisionReply bool
	loseNextCASReply      bool
}

func NewReviewStoreV2(clock func() time.Time) *ReviewStoreV2 {
	if clock == nil {
		clock = time.Now
	}
	return &ReviewStoreV2{clock: clock, cases: map[string]ports.ReviewCaseFactV2{}, verdicts: map[string]ports.ReviewVerdictFactV2{}, satisfactions: map[string]ports.ConditionSatisfactionFactV2{}, satisfactionByVerdict: map[string]string{}}
}
func (s *ReviewStoreV2) SetClock(clock func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if clock == nil {
		clock = time.Now
	}
	s.clock = clock
}
func (s *ReviewStoreV2) LoseNextCreateReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextCreateReply = true
}
func (s *ReviewStoreV2) LoseNextDecisionReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextDecisionReply = true
}
func (s *ReviewStoreV2) LoseNextCASReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextCASReply = true
}

func (s *ReviewStoreV2) CreateReviewCase(ctx context.Context, fact ports.ReviewCaseFactV2) (ports.ReviewCaseFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	if fact.State != ports.ReviewCasePending || fact.Revision != 1 {
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonReviewCandidateConflict, "new review case must be pending at revision one")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.cases[fact.Candidate.ID]; ok {
		if existing.CandidateDigest == fact.CandidateDigest {
			return cloneReviewCaseV2(existing), nil
		}
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "review case id already binds different candidate")
	}
	s.cases[fact.Candidate.ID] = cloneReviewCaseV2(fact)
	if s.loseNextCreateReply {
		s.loseNextCreateReply = false
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected review create reply loss")
	}
	return cloneReviewCaseV2(fact), nil
}

func (s *ReviewStoreV2) InspectReviewCase(ctx context.Context, id string) (ports.ReviewCaseFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.cases[id]
	if !ok {
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "review case not found")
	}
	return cloneReviewCaseV2(fact), nil
}

func (s *ReviewStoreV2) CompareAndSwapReviewCase(ctx context.Context, request ports.ReviewCaseCASRequestV2) (ports.ReviewCaseFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.cases[request.CaseID]
	if !ok {
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "review case not found")
	}
	if current.Revision != request.ExpectedRevision {
		if sameReviewCaseFactV2(current, request.Next) {
			return cloneReviewCaseV2(current), nil
		}
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review case CAS revision conflict")
	}
	if err := control.ValidateReviewCaseTransitionV2(current, request.Next, now); err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	s.cases[request.CaseID] = cloneReviewCaseV2(request.Next)
	if s.loseNextCASReply {
		s.loseNextCASReply = false
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected review case CAS reply loss")
	}
	return cloneReviewCaseV2(request.Next), nil
}

func (s *ReviewStoreV2) DecideReview(ctx context.Context, request ports.DecideReviewRequestV2) (ports.DecideReviewResultV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.DecideReviewResultV2{}, err
	}
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.cases[request.CaseID]
	if !ok {
		return ports.DecideReviewResultV2{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "review case not found")
	}
	if existing, ok := s.verdicts[request.Verdict.ID]; ok {
		digest, _ := request.Verdict.DigestV2()
		existingDigest, _ := existing.DigestV2()
		if digest == existingDigest {
			return ports.DecideReviewResultV2{Case: cloneReviewCaseV2(s.cases[request.CaseID]), Verdict: cloneReviewVerdictV2(existing)}, nil
		}
		return ports.DecideReviewResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "review verdict id already binds another decision")
	}
	if current.Revision != request.ExpectedCaseRevision {
		return ports.DecideReviewResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review decision lost case CAS")
	}
	// Governance facts are re-read by ReviewGovernanceGatewayV2. The raw store
	// performs only the atomic candidate/decision linkage.
	nextCase, err := control.ValidateReviewDecisionTransitionV2(current, request.Verdict, now)
	if err != nil {
		return ports.DecideReviewResultV2{}, err
	}
	s.cases[request.CaseID], s.verdicts[request.Verdict.ID] = cloneReviewCaseV2(nextCase), cloneReviewVerdictV2(request.Verdict)
	result := ports.DecideReviewResultV2{Case: cloneReviewCaseV2(nextCase), Verdict: cloneReviewVerdictV2(request.Verdict)}
	if s.loseNextDecisionReply {
		s.loseNextDecisionReply = false
		return ports.DecideReviewResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected review decision reply loss")
	}
	return result, nil
}

func (s *ReviewStoreV2) InspectReviewVerdict(ctx context.Context, id string) (ports.ReviewVerdictFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewVerdictFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.verdicts[id]
	if !ok {
		return ports.ReviewVerdictFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "review verdict not found")
	}
	return cloneReviewVerdictV2(fact), nil
}

func (s *ReviewStoreV2) CompareAndSwapReviewVerdict(ctx context.Context, request ports.ReviewVerdictCASRequestV2) (ports.ReviewVerdictFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ReviewVerdictFactV2{}, err
	}
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.verdicts[request.VerdictID]
	if !ok {
		return ports.ReviewVerdictFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "review verdict not found")
	}
	if current.Revision != request.ExpectedRevision {
		if sameReviewVerdictFactV2(current, request.Next) {
			return cloneReviewVerdictV2(current), nil
		}
		return ports.ReviewVerdictFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review verdict CAS revision conflict")
	}
	if err := control.ValidateReviewVerdictTransitionV2(current, request.Next, now); err != nil {
		return ports.ReviewVerdictFactV2{}, err
	}
	s.verdicts[request.VerdictID] = cloneReviewVerdictV2(request.Next)
	if s.loseNextCASReply {
		s.loseNextCASReply = false
		return ports.ReviewVerdictFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected review verdict CAS reply loss")
	}
	return cloneReviewVerdictV2(request.Next), nil
}

func (s *ReviewStoreV2) CreateConditionSatisfaction(ctx context.Context, fact ports.ConditionSatisfactionFactV2) (ports.ConditionSatisfactionFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	if fact.State != ports.ConditionSatisfactionPending || fact.Revision != 1 {
		return ports.ConditionSatisfactionFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonReviewConditionUnsatisfied, "new satisfaction must be pending revision one")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.satisfactions[fact.ID]; ok {
		if sameConditionSatisfactionFactV2(existing, fact) {
			return cloneSatisfactionV2(existing), nil
		}
		return ports.ConditionSatisfactionFactV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "satisfaction id binds different subject")
	}
	if other, ok := s.satisfactionByVerdict[fact.VerdictID]; ok && other != fact.ID {
		return ports.ConditionSatisfactionFactV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "conditional verdict already has a satisfaction journal")
	}
	s.satisfactions[fact.ID], s.satisfactionByVerdict[fact.VerdictID] = cloneSatisfactionV2(fact), fact.ID
	return cloneSatisfactionV2(fact), nil
}
func (s *ReviewStoreV2) InspectConditionSatisfaction(ctx context.Context, id string) (ports.ConditionSatisfactionFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.satisfactions[id]
	if !ok {
		return ports.ConditionSatisfactionFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonReviewConditionUnsatisfied, "satisfaction not found")
	}
	return cloneSatisfactionV2(fact), nil
}
func (s *ReviewStoreV2) InspectConditionSatisfactionByVerdict(ctx context.Context, verdictID string) (ports.ConditionSatisfactionFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.satisfactionByVerdict[verdictID]
	if !ok {
		return ports.ConditionSatisfactionFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonReviewConditionUnsatisfied, "verdict satisfaction not found")
	}
	return cloneSatisfactionV2(s.satisfactions[id]), nil
}
func (s *ReviewStoreV2) CompareAndSwapConditionSatisfaction(ctx context.Context, request ports.ConditionSatisfactionCASRequestV2) (ports.ConditionSatisfactionFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.satisfactions[request.SatisfactionID]
	if !ok {
		return ports.ConditionSatisfactionFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonReviewConditionUnsatisfied, "satisfaction not found")
	}
	verdict, ok := s.verdicts[current.VerdictID]
	if !ok {
		return ports.ConditionSatisfactionFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "conditional verdict not found")
	}
	if current.Revision != request.ExpectedRevision {
		if sameConditionSatisfactionFactV2(current, request.Next) {
			return cloneSatisfactionV2(current), nil
		}
		return ports.ConditionSatisfactionFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "satisfaction CAS revision conflict")
	}
	if err := control.ValidateConditionSatisfactionTransitionV2(current, request.Next, verdict, now); err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	s.satisfactions[request.SatisfactionID] = cloneSatisfactionV2(request.Next)
	if s.loseNextCASReply {
		s.loseNextCASReply = false
		return ports.ConditionSatisfactionFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected satisfaction CAS reply loss")
	}
	return cloneSatisfactionV2(request.Next), nil
}

func cloneReviewCaseV2(v ports.ReviewCaseFactV2) ports.ReviewCaseFactV2 {
	v.Candidate.Evidence = append([]ports.ReviewEvidenceRefV2(nil), v.Candidate.Evidence...)
	return v
}
func cloneReviewVerdictV2(v ports.ReviewVerdictFactV2) ports.ReviewVerdictFactV2 {
	v.DecisionEvidence = append([]ports.ReviewEvidenceRefV2(nil), v.DecisionEvidence...)
	v.Conditions = append([]ports.ReviewConditionV2(nil), v.Conditions...)
	if v.InvocationEffect != nil {
		copy := *v.InvocationEffect
		v.InvocationEffect = &copy
	}
	return v
}
func cloneSatisfactionV2(v ports.ConditionSatisfactionFactV2) ports.ConditionSatisfactionFactV2 {
	v.Proofs = append([]ports.ReviewConditionProofV2(nil), v.Proofs...)
	return v
}

func sameReviewCaseFactV2(left, right ports.ReviewCaseFactV2) bool {
	leftDigest, leftErr := left.DigestV2()
	rightDigest, rightErr := right.DigestV2()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func sameReviewVerdictFactV2(left, right ports.ReviewVerdictFactV2) bool {
	leftDigest, leftErr := left.DigestV2()
	rightDigest, rightErr := right.DigestV2()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func sameConditionSatisfactionFactV2(left, right ports.ConditionSatisfactionFactV2) bool {
	leftDigest, leftErr := left.DigestV2()
	rightDigest, rightErr := right.DigestV2()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

var _ ports.ReviewVerdictFactPortV2 = (*ReviewStoreV2)(nil)

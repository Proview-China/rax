package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	GovernedContractVersionV2 = "praxis.harness.governed/v2"
	MaxContinuationItemsV2    = 64
)

// EndpointRefV2 is the immutable Harness execution-surface identity. It does
// not prove that an endpoint is open or dispatchable.
type EndpointRefV2 struct {
	ID             string                            `json:"endpoint_id"`
	Scope          core.ExecutionScope               `json:"scope"`
	Binding        runtimeports.ProviderBindingRefV2 `json:"binding"`
	Revision       core.Revision                     `json:"revision"`
	IdentityDigest core.Digest                       `json:"identity_digest"`
}

func (r EndpointRefV2) Validate() error {
	if strings.TrimSpace(r.ID) == "" || len(r.ID) > MaxReferenceBytes || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "governed endpoint identity and revision are required")
	}
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	expected, err := endpointIdentityDigestV2(r.ID, r.Scope, r.Binding, r.Revision)
	if err != nil {
		return err
	}
	if r.IdentityDigest != expected {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "governed endpoint identity digest drifted")
	}
	return nil
}

func NewEndpointRefV2(id string, scope core.ExecutionScope, binding runtimeports.ProviderBindingRefV2) (EndpointRefV2, error) {
	digest, err := endpointIdentityDigestV2(id, scope, binding, 1)
	if err != nil {
		return EndpointRefV2{}, err
	}
	result := EndpointRefV2{ID: id, Scope: scope, Binding: binding, Revision: 1, IdentityDigest: digest}
	return result, result.Validate()
}

func endpointIdentityDigestV2(id string, scope core.ExecutionScope, binding runtimeports.ProviderBindingRefV2, revision core.Revision) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.harness.governed", GovernedContractVersionV2, "EndpointIdentityV2", struct {
		ID       string                            `json:"endpoint_id"`
		Scope    core.ExecutionScope               `json:"scope"`
		Binding  runtimeports.ProviderBindingRefV2 `json:"binding"`
		Revision core.Revision                     `json:"revision"`
	}{id, scope, binding, revision})
}

type SessionPhaseV2 string

const (
	SessionCreatingV2              SessionPhaseV2 = "creating"
	SessionWaitingModelDispatchV2  SessionPhaseV2 = "waiting_model_dispatch"
	SessionModelDispatchReservedV2 SessionPhaseV2 = "model_dispatch_reserved"
	SessionModelInFlightV2         SessionPhaseV2 = "model_in_flight"
	SessionWaitingSettlementV2     SessionPhaseV2 = "waiting_settlement"
	SessionWaitingActionV2         SessionPhaseV2 = "waiting_action"
	SessionWaitingInputV2          SessionPhaseV2 = "waiting_input"
	SessionReconcilingV2           SessionPhaseV2 = "reconciling"
	SessionTerminalV2              SessionPhaseV2 = "terminal"
)

type CandidateKindV2 string

const (
	CandidateInitialTurnV2 CandidateKindV2 = "initial_turn"
	CandidateActionTurnV2  CandidateKindV2 = "action_continuation"
	CandidateInputTurnV2   CandidateKindV2 = "input_continuation"
)

type CandidateRefV2 struct {
	ID       string        `json:"candidate_id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

// ModelDispatchReservationRefV2 is the Harness-local projection needed to
// exclude competing dispatches. Application-specific routing stays in the
// bridge-owned reservation Fact.
type ModelDispatchReservationRefV2 struct {
	ID               string      `json:"id"`
	Digest           core.Digest `json:"digest"`
	AttemptID        string      `json:"attempt_id"`
	IntentDigest     core.Digest `json:"intent_digest"`
	CandidateDigest  core.Digest `json:"candidate_digest"`
	ReservedUnixNano int64       `json:"reserved_unix_nano"`
	ExpiresUnixNano  int64       `json:"expires_unix_nano"`
}

func (r ModelDispatchReservationRefV2) Validate() error {
	if strings.TrimSpace(r.ID) == "" || len(r.ID) > MaxReferenceBytes || strings.TrimSpace(r.AttemptID) == "" || len(r.AttemptID) > MaxReferenceBytes || r.ReservedUnixNano <= 0 || r.ExpiresUnixNano <= r.ReservedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "model dispatch reservation reference is incomplete")
	}
	for _, digest := range []core.Digest{r.Digest, r.IntentDigest, r.CandidateDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (r CandidateRefV2) Validate() error {
	if strings.TrimSpace(r.ID) == "" || len(r.ID) > MaxReferenceBytes || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "model candidate reference and revision are required")
	}
	return r.Digest.Validate()
}

type ContinuationRefV2 struct {
	Kind             CandidateKindV2 `json:"kind"`
	PendingRef       string          `json:"pending_ref"`
	PendingDigest    core.Digest     `json:"pending_digest"`
	SettlementRef    string          `json:"settlement_ref"`
	SettlementDigest core.Digest     `json:"settlement_digest"`
	EvidenceRef      string          `json:"evidence_ref"`
	EvidenceDigest   core.Digest     `json:"evidence_digest"`
}

func (r ContinuationRefV2) Validate() error {
	if r.Kind != CandidateActionTurnV2 && r.Kind != CandidateInputTurnV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "continuation kind must be action or input")
	}
	for _, value := range []string{r.PendingRef, r.SettlementRef, r.EvidenceRef} {
		if strings.TrimSpace(value) == "" || len(value) > MaxReferenceBytes {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "continuation references are required and bounded")
		}
	}
	for _, digest := range []core.Digest{r.PendingDigest, r.SettlementDigest, r.EvidenceDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ModelTurnCandidateV2 is immutable input to governance. Creating it never
// authorizes or performs a model call.
type ModelTurnCandidateV2 struct {
	ContractVersion         string                            `json:"contract_version"`
	ID                      string                            `json:"candidate_id"`
	Revision                core.Revision                     `json:"revision"`
	Run                     RunRef                            `json:"run"`
	Endpoint                EndpointRefV2                     `json:"endpoint"`
	SessionRef              string                            `json:"session_ref"`
	ExpectedSessionRevision core.Revision                     `json:"expected_session_revision"`
	Turn                    uint32                            `json:"turn"`
	Kind                    CandidateKindV2                   `json:"kind"`
	Input                   runtimeports.OpaquePayloadV2      `json:"input"`
	ContextRef              string                            `json:"context_ref"`
	ContextDigest           core.Digest                       `json:"context_digest"`
	Continuation            *ContinuationRefV2                `json:"continuation,omitempty"`
	Provider                runtimeports.ProviderBindingRefV2 `json:"provider"`
	CreatedUnixNano         int64                             `json:"created_unix_nano"`
	ExpiresUnixNano         int64                             `json:"expires_unix_nano"`
}

func (c ModelTurnCandidateV2) Validate(now time.Time) error {
	if c.ContractVersion != GovernedContractVersionV2 || strings.TrimSpace(c.ID) == "" || len(c.ID) > MaxReferenceBytes || c.Revision != 1 || c.ExpectedSessionRevision == 0 || c.Turn == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "immutable model candidate identity, session revision and turn are required")
	}
	if err := c.Run.Validate(); err != nil {
		return err
	}
	if err := c.Endpoint.Validate(); err != nil {
		return err
	}
	if !sameExecutionScopeV2(c.Run.Scope, c.Endpoint.Scope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "model candidate endpoint and run scope differ")
	}
	if strings.TrimSpace(c.SessionRef) == "" || len(c.SessionRef) > MaxReferenceBytes || strings.TrimSpace(c.ContextRef) == "" || len(c.ContextRef) > MaxReferenceBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "model candidate session and context references are required")
	}
	if err := c.Input.Validate(); err != nil {
		return err
	}
	// The current model-turn bridge has no governed resolver for referenced
	// input bodies. Accepting a Ref here would move an undeclared fetch Effect
	// into the provider and make its digest/credential/cleanup owner ambiguous.
	// A future reference-capable contract must add that resolver explicitly.
	if c.Input.Inline == nil {
		return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownSchema, "model candidate referenced input is unsupported without a governed resolver")
	}
	if err := c.ContextDigest.Validate(); err != nil {
		return err
	}
	if err := c.Provider.Validate(); err != nil {
		return err
	}
	// A single custom component may provide both capabilities, but the exact
	// Harness execution capability must never be reused as the model dispatch
	// authority.
	if c.Provider.ComponentID == c.Endpoint.Binding.ComponentID && c.Provider.Capability == c.Endpoint.Binding.Capability {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "model execution requires an explicitly separate provider capability")
	}
	if c.CreatedUnixNano <= 0 || c.ExpiresUnixNano <= c.CreatedUnixNano || (!now.IsZero() && !now.Before(time.Unix(0, c.ExpiresUnixNano))) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "model candidate lifetime is invalid or expired")
	}
	switch c.Kind {
	case CandidateInitialTurnV2:
		if c.Continuation != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "initial candidate cannot bind a continuation")
		}
	case CandidateActionTurnV2, CandidateInputTurnV2:
		if c.Continuation == nil || c.Continuation.Kind != c.Kind {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "continuation candidate requires the exact continuation kind")
		}
		if err := c.Continuation.Validate(); err != nil {
			return err
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown model candidate kind")
	}
	return validateModelTurnEffectEncodingBudgetV2(c)
}

func (c ModelTurnCandidateV2) DigestV2() (core.Digest, error) {
	if err := c.Validate(time.Time{}); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.harness.governed", GovernedContractVersionV2, "ModelTurnCandidateV2", c)
}

func (c ModelTurnCandidateV2) RefV2() (CandidateRefV2, error) {
	digest, err := c.DigestV2()
	if err != nil {
		return CandidateRefV2{}, err
	}
	return CandidateRefV2{ID: c.ID, Revision: c.Revision, Digest: digest}, nil
}

type PendingActionV2 struct {
	Ref             string                        `json:"action_ref"`
	Capability      runtimeports.CapabilityNameV2 `json:"capability"`
	Payload         runtimeports.OpaquePayloadV2  `json:"payload"`
	SourceCandidate CandidateRefV2                `json:"source_candidate"`
	RequestDigest   core.Digest                   `json:"request_digest"`
}

func NewPendingActionV2(ref string, capability runtimeports.CapabilityNameV2, payload runtimeports.OpaquePayloadV2, source CandidateRefV2) (PendingActionV2, error) {
	result := PendingActionV2{Ref: ref, Capability: capability, Payload: payload, SourceCandidate: source}
	digest, err := result.digestSubjectV2()
	if err != nil {
		return PendingActionV2{}, err
	}
	result.RequestDigest = digest
	return result, result.Validate()
}

func (p PendingActionV2) Validate() error {
	if strings.TrimSpace(p.Ref) == "" || len(p.Ref) > MaxReferenceBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "pending action ref is required")
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(p.Capability)); err != nil {
		return err
	}
	if err := p.Payload.Validate(); err != nil {
		return err
	}
	if err := p.SourceCandidate.Validate(); err != nil {
		return err
	}
	expected, err := p.digestSubjectV2()
	if err != nil {
		return err
	}
	if p.RequestDigest != expected {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "pending action digest drifted")
	}
	return nil
}

func (p PendingActionV2) digestSubjectV2() (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.harness.governed", GovernedContractVersionV2, "PendingActionSubjectV2", struct {
		Ref        string                        `json:"action_ref"`
		Capability runtimeports.CapabilityNameV2 `json:"capability"`
		Payload    runtimeports.OpaquePayloadV2  `json:"payload"`
		Source     CandidateRefV2                `json:"source_candidate"`
	}{p.Ref, p.Capability, p.Payload, p.SourceCandidate})
}

type PendingInputV2 struct {
	Ref             string                   `json:"input_ref"`
	Schema          runtimeports.SchemaRefV2 `json:"schema"`
	SourceCandidate CandidateRefV2           `json:"source_candidate"`
	RequestDigest   core.Digest              `json:"request_digest"`
}

func NewPendingInputV2(ref string, schema runtimeports.SchemaRefV2, source CandidateRefV2) (PendingInputV2, error) {
	result := PendingInputV2{Ref: ref, Schema: schema, SourceCandidate: source}
	digest, err := result.digestSubjectV2()
	if err != nil {
		return PendingInputV2{}, err
	}
	result.RequestDigest = digest
	return result, result.Validate()
}

func (p PendingInputV2) Validate() error {
	if strings.TrimSpace(p.Ref) == "" || len(p.Ref) > MaxReferenceBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "pending input ref is required")
	}
	if err := p.Schema.Validate(); err != nil {
		return err
	}
	if err := p.SourceCandidate.Validate(); err != nil {
		return err
	}
	expected, err := p.digestSubjectV2()
	if err != nil {
		return err
	}
	if p.RequestDigest != expected {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "pending input digest drifted")
	}
	return nil
}

func (p PendingInputV2) digestSubjectV2() (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.harness.governed", GovernedContractVersionV2, "PendingInputSubjectV2", struct {
		Ref    string                   `json:"input_ref"`
		Schema runtimeports.SchemaRefV2 `json:"schema"`
		Source CandidateRefV2           `json:"source_candidate"`
	}{p.Ref, p.Schema, p.SourceCandidate})
}

type GovernedSessionV2 struct {
	ContractVersion        string                                       `json:"contract_version"`
	ID                     string                                       `json:"session_id"`
	Revision               core.Revision                                `json:"revision"`
	Run                    RunRef                                       `json:"run"`
	Endpoint               EndpointRefV2                                `json:"endpoint"`
	Phase                  SessionPhaseV2                               `json:"phase"`
	Turn                   uint32                                       `json:"turn"`
	Candidate              *CandidateRefV2                              `json:"candidate,omitempty"`
	DomainReservation      *ModelDispatchReservationRefV2               `json:"domain_reservation,omitempty"`
	Execution              *runtimeports.GovernedExecutionAttemptRefsV2 `json:"execution,omitempty"`
	PendingAction          *PendingActionV2                             `json:"pending_action,omitempty"`
	PendingInput           *PendingInputV2                              `json:"pending_input,omitempty"`
	UndispatchedSettlement *UndispatchedSettlementBindingV2             `json:"undispatched_settlement,omitempty"`
	CompletionClaim        CompletionClaim                              `json:"completion_claim,omitempty"`
	CreatedUnixNano        int64                                        `json:"created_unix_nano"`
	UpdatedUnixNano        int64                                        `json:"updated_unix_nano"`
}

func (s GovernedSessionV2) Validate() error {
	if s.ContractVersion != GovernedContractVersionV2 || strings.TrimSpace(s.ID) == "" || len(s.ID) > MaxReferenceBytes || s.Revision == 0 || s.CreatedUnixNano <= 0 || s.UpdatedUnixNano < s.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "governed session identity, revision and timestamps are required")
	}
	if err := s.Run.Validate(); err != nil {
		return err
	}
	if err := s.Endpoint.Validate(); err != nil {
		return err
	}
	if !sameExecutionScopeV2(s.Run.Scope, s.Endpoint.Scope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "session endpoint and run scope differ")
	}
	if s.Candidate != nil {
		if err := s.Candidate.Validate(); err != nil {
			return err
		}
	}
	if s.Execution != nil {
		if err := s.Execution.ValidatePrepared(); err != nil {
			return err
		}
	}
	if s.DomainReservation != nil {
		if err := s.DomainReservation.Validate(); err != nil {
			return err
		}
		if s.Candidate == nil || s.DomainReservation.CandidateDigest != s.Candidate.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "session reservation differs from its Candidate or endpoint")
		}
	}
	if s.PendingAction != nil {
		if err := s.PendingAction.Validate(); err != nil {
			return err
		}
	}
	if s.PendingInput != nil {
		if err := s.PendingInput.Validate(); err != nil {
			return err
		}
	}
	if s.UndispatchedSettlement != nil {
		if err := s.UndispatchedSettlement.Validate(); err != nil {
			return err
		}
	}
	switch s.Phase {
	case SessionCreatingV2:
		if s.Turn != 0 || s.Candidate != nil || s.DomainReservation != nil || s.Execution != nil || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV2()
		}
	case SessionWaitingModelDispatchV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation != nil || s.Execution != nil || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV2()
		}
	case SessionModelDispatchReservedV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation == nil || s.Execution != nil || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV2()
		}
	case SessionModelInFlightV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation == nil || s.Execution == nil || s.Execution.Observation != nil || s.Execution.Settlement != nil || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV2()
		}
	case SessionReconcilingV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation == nil || s.Execution == nil || s.Execution.Settlement != nil || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV2()
		}
	case SessionWaitingSettlementV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation == nil || !hasObservedUnsettledExecutionV2(s.Execution) || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV2()
		}
	case SessionWaitingActionV2:
		if s.Turn == 0 || s.Candidate != nil || s.DomainReservation != nil || !hasSettledExecutionV2(s.Execution) || s.PendingAction == nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV2()
		}
	case SessionWaitingInputV2:
		if s.Turn == 0 || s.Candidate != nil || s.DomainReservation != nil || !hasSettledExecutionV2(s.Execution) || s.PendingAction != nil || s.PendingInput == nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV2()
		}
	case SessionTerminalV2:
		if s.Candidate != nil || s.DomainReservation != nil || s.PendingAction != nil || s.PendingInput != nil || !validClaim(s.CompletionClaim) || !validTerminalExecutionOrUndispatchedV2(s) {
			return invalidSessionFieldsV2()
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown governed session phase")
	}
	return nil
}

func hasObservedUnsettledExecutionV2(execution *runtimeports.GovernedExecutionAttemptRefsV2) bool {
	return execution != nil && execution.Observation != nil && execution.Observation.State == runtimeports.ProviderAttemptObservedV2 && execution.Settlement == nil
}

func hasSettledExecutionV2(execution *runtimeports.GovernedExecutionAttemptRefsV2) bool {
	return execution != nil && execution.Settlement != nil && execution.Settlement.DomainResultSchema != nil && *execution.Settlement.DomainResultSchema == SettledTurnResultSchemaV2() && execution.Settlement.DomainResultDigest.Validate() == nil
}

func hasTerminalExecutionV2(execution *runtimeports.GovernedExecutionAttemptRefsV2, claim CompletionClaim) bool {
	if !hasSettledExecutionV2(execution) {
		return false
	}
	switch claim {
	case ClaimCompleted:
		return execution.Settlement.Disposition == runtimeports.OperationSettlementAppliedV3
	case ClaimFailed:
		return execution.Settlement.Disposition == runtimeports.OperationSettlementAppliedV3 || execution.Settlement.Disposition == runtimeports.OperationSettlementFailedV3 || execution.Settlement.Disposition == runtimeports.OperationSettlementNotAppliedV3
	default:
		return false
	}
}

func validTerminalExecutionOrUndispatchedV2(session GovernedSessionV2) bool {
	if session.CompletionClaim == ClaimCancelled {
		return session.UndispatchedSettlement == nil
	}
	if session.UndispatchedSettlement != nil {
		return session.CompletionClaim == ClaimFailed && session.Execution == nil
	}
	return hasTerminalExecutionV2(session.Execution, session.CompletionClaim)
}

func invalidSessionFieldsV2() error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "governed session phase fields are inconsistent")
}

// ValidateSessionTransitionV2 centralizes the Harness-owned CAS state
// machine. Provider adapters and custom components cannot invent transitions.
func ValidateSessionTransitionV2(current, next GovernedSessionV2) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.ID != next.ID || current.ContractVersion != next.ContractVersion || current.Run.RunID != next.Run.RunID || !sameExecutionScopeV2(current.Run.Scope, next.Run.Scope) || current.Endpoint.ID != next.Endpoint.ID || current.Endpoint.IdentityDigest != next.Endpoint.IdentityDigest || current.CreatedUnixNano != next.CreatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "governed session immutable identity changed")
	}
	if next.Revision != current.Revision+1 || next.UpdatedUnixNano < current.UpdatedUnixNano || next.Turn < current.Turn || next.Turn > current.Turn+1 {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "governed session revision, time or turn is not monotonic")
	}
	allowed := false
	switch current.Phase {
	case SessionCreatingV2:
		allowed = next.Phase == SessionWaitingModelDispatchV2 && next.Turn == 1 || isCancellationTerminalV2(next) && next.Turn == current.Turn
	case SessionWaitingModelDispatchV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionModelDispatchReservedV2 || isCancellationTerminalV2(next))
	case SessionModelDispatchReservedV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionModelInFlightV2 || isCancellationTerminalV2(next) || isUndispatchedFailureTerminalV2(next))
	case SessionModelInFlightV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionWaitingSettlementV2 || next.Phase == SessionReconcilingV2 || isCancellationTerminalV2(next))
	case SessionWaitingSettlementV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionWaitingActionV2 || next.Phase == SessionWaitingInputV2 || next.Phase == SessionTerminalV2)
	case SessionWaitingActionV2, SessionWaitingInputV2:
		allowed = next.Phase == SessionWaitingModelDispatchV2 && next.Turn == current.Turn+1 || isCancellationTerminalV2(next) && next.Turn == current.Turn
	case SessionReconcilingV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionWaitingSettlementV2 || next.Phase == SessionWaitingActionV2 || next.Phase == SessionWaitingInputV2 || next.Phase == SessionTerminalV2)
	case SessionTerminalV2:
		allowed = false
	}
	if !allowed {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "governed session transition is not allowed")
	}
	return nil
}

func isCancellationTerminalV2(session GovernedSessionV2) bool {
	return session.Phase == SessionTerminalV2 && session.CompletionClaim == ClaimCancelled
}

func isUndispatchedFailureTerminalV2(session GovernedSessionV2) bool {
	return session.Phase == SessionTerminalV2 && session.CompletionClaim == ClaimFailed && session.Execution == nil && session.UndispatchedSettlement != nil
}

func sameExecutionScopeV2(a, b core.ExecutionScope) bool {
	ad, err := core.CanonicalJSONDigest("praxis.harness.governed", GovernedContractVersionV2, "ExecutionScopeV2", a)
	if err != nil {
		return false
	}
	bd, err := core.CanonicalJSONDigest("praxis.harness.governed", GovernedContractVersionV2, "ExecutionScopeV2", b)
	return err == nil && ad == bd
}

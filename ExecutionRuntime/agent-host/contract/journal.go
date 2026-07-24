package contract

type HostPhaseV1 string

const (
	HostAcceptedV1      HostPhaseV1 = "accepted"
	HostValidatingV1    HostPhaseV1 = "validating"
	HostResolvingV1     HostPhaseV1 = "resolving"
	HostCompilingV1     HostPhaseV1 = "compiling"
	HostBindingV1       HostPhaseV1 = "binding"
	HostConstructingV1  HostPhaseV1 = "constructing"
	HostVerifyingV1     HostPhaseV1 = "verifying"
	HostReadyV1         HostPhaseV1 = "ready"
	HostDrainingV1      HostPhaseV1 = "draining"
	HostReconcilingV1   HostPhaseV1 = "reconciling"
	HostClosedV1        HostPhaseV1 = "closed"
	HostIndeterminateV1 HostPhaseV1 = "indeterminate"
)

type ConstructedComponentV1 struct {
	NodeID       string       `json:"node_id"`
	Factory      FactoryKeyV1 `json:"factory"`
	ComponentRef ExactRefV1   `json:"component_ref"`
}

func (c ConstructedComponentV1) Validate() error {
	if err := ValidateIdentifierV1("node id", c.NodeID); err != nil {
		return err
	}
	if err := c.Factory.Validate(); err != nil {
		return err
	}
	return c.ComponentRef.Validate()
}

type HostJournalV1 struct {
	ContractVersion      string                   `json:"contract_version"`
	HostID               string                   `json:"host_id"`
	StartID              string                   `json:"start_id"`
	Revision             uint64                   `json:"revision"`
	Phase                HostPhaseV1              `json:"phase"`
	ConfigDigest         DigestV1                 `json:"config_digest"`
	DefinitionRef        *ExactRefV1              `json:"definition_ref,omitempty"`
	PlanRef              *ExactRefV1              `json:"plan_ref,omitempty"`
	GenerationRef        *ExactRefV1              `json:"generation_ref,omitempty"`
	HandoffRef           *ExactRefV1              `json:"handoff_ref,omitempty"`
	BindingAttempt       *BindingAttemptV1        `json:"binding_attempt,omitempty"`
	BindingRef           *ExactRefV1              `json:"binding_ref,omitempty"`
	GraphRef             *ExactRefV1              `json:"graph_ref,omitempty"`
	ConstructionAttempts []ConstructionAttemptV1  `json:"construction_attempts"`
	Constructed          []ConstructedComponentV1 `json:"constructed"`
	ReadyRef             *ExactRefV1              `json:"ready_ref,omitempty"`
	CreatedUnixNano      int64                    `json:"created_unix_nano"`
	UpdatedUnixNano      int64                    `json:"updated_unix_nano"`
	Digest               DigestV1                 `json:"digest"`
}

func (j HostJournalV1) Validate() error {
	if j.ContractVersion != ContractVersionV1 {
		return NewError(ErrorInvalidArgument, "contract_version_mismatch", "journal contract version is unsupported")
	}
	if err := ValidateIdentifierV1("host id", j.HostID); err != nil {
		return err
	}
	if err := ValidateIdentifierV1("start id", j.StartID); err != nil {
		return err
	}
	if j.Revision == 0 || j.CreatedUnixNano <= 0 || j.UpdatedUnixNano < j.CreatedUnixNano {
		return NewError(ErrorInvalidArgument, "invalid_journal_watermark", "journal revision or time watermark is invalid")
	}
	if !validHostPhaseV1(j.Phase) {
		return NewError(ErrorInvalidArgument, "invalid_phase", "journal phase is unsupported")
	}
	if err := j.ConfigDigest.Validate(); err != nil {
		return err
	}
	for _, ref := range []*ExactRefV1{j.DefinitionRef, j.PlanRef, j.GenerationRef, j.HandoffRef, j.BindingRef, j.GraphRef, j.ReadyRef} {
		if ref != nil {
			if err := ref.Validate(); err != nil {
				return err
			}
		}
	}
	if j.BindingAttempt != nil {
		if err := j.BindingAttempt.Validate(); err != nil {
			return err
		}
		if j.BindingAttempt.State == AttemptBoundV1 && (j.BindingRef == nil || !SameExactRefV1(*j.BindingAttempt.BindingRef, *j.BindingRef)) {
			return NewError(ErrorConflict, "binding_attempt_ref_drift", "bound attempt does not match journal binding ref")
		}
	}
	if err := validatePhaseClosureV1(j); err != nil {
		return err
	}
	attempts := map[string]ConstructionAttemptV1{}
	ids := map[string]struct{}{}
	for _, a := range j.ConstructionAttempts {
		if err := a.Validate(); err != nil {
			return err
		}
		if _, ok := attempts[a.NodeID]; ok {
			return NewError(ErrorConflict, "duplicate_construction_attempt", "journal duplicates a construction node attempt")
		}
		if _, ok := ids[a.AttemptID]; ok {
			return NewError(ErrorConflict, "duplicate_construction_attempt", "journal duplicates a construction attempt id")
		}
		attempts[a.NodeID] = a
		ids[a.AttemptID] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, component := range j.Constructed {
		if err := component.Validate(); err != nil {
			return err
		}
		if _, ok := seen[component.NodeID]; ok {
			return NewError(ErrorConflict, "duplicate_component", "journal duplicates a constructed node")
		}
		seen[component.NodeID] = struct{}{}
		a, ok := attempts[component.NodeID]
		if !ok || a.State != AttemptConstructedV1 || a.ComponentRef == nil || a.Factory != component.Factory || !SameExactRefV1(*a.ComponentRef, component.ComponentRef) {
			return NewError(ErrorConflict, "construction_attempt_component_drift", "constructed component lacks its exact successful attempt")
		}
	}
	for _, a := range j.ConstructionAttempts {
		if a.State == AttemptConstructedV1 {
			if _, ok := seen[a.NodeID]; !ok {
				return NewError(ErrorConflict, "construction_attempt_component_missing", "successful attempt lacks its constructed component")
			}
		}
	}
	expected, err := j.digestV1()
	if err != nil {
		return err
	}
	if expected != j.Digest {
		return NewError(ErrorPrecondition, "journal_digest_drift", "journal digest drifted")
	}
	return nil
}
func (j HostJournalV1) digestV1() (DigestV1, error) {
	clone := j
	clone.Digest = ""
	return DigestJSONV1(clone)
}
func SealHostJournalV1(j HostJournalV1) (HostJournalV1, error) {
	d, err := j.digestV1()
	if err != nil {
		return HostJournalV1{}, err
	}
	j.Digest = d
	return j, nil
}
func (j HostJournalV1) RefV1() (ExactRefV1, error) {
	if err := j.Validate(); err != nil {
		return ExactRefV1{}, err
	}
	return ExactRefV1{Kind: "praxis.agent-host/journal", ID: j.HostID + "/" + j.StartID, Revision: j.Revision, Digest: j.Digest}, nil
}
func (j HostJournalV1) HasUnknownAttemptsV1() bool {
	if j.BindingAttempt != nil && (j.BindingAttempt.State == AttemptUnknownV1 || j.BindingAttempt.State == AttemptPlannedV1) {
		return true
	}
	for _, a := range j.ConstructionAttempts {
		if a.State == AttemptUnknownV1 || a.State == AttemptPlannedV1 {
			return true
		}
	}
	return false
}

func validHostPhaseV1(v HostPhaseV1) bool {
	switch v {
	case HostAcceptedV1, HostValidatingV1, HostResolvingV1, HostCompilingV1, HostBindingV1, HostConstructingV1, HostVerifyingV1, HostReadyV1, HostDrainingV1, HostReconcilingV1, HostClosedV1, HostIndeterminateV1:
		return true
	}
	return false
}

var allowedTransitionsV1 = map[HostPhaseV1]map[HostPhaseV1]bool{
	HostAcceptedV1: {HostValidatingV1: true, HostDrainingV1: true, HostIndeterminateV1: true}, HostValidatingV1: {HostResolvingV1: true, HostDrainingV1: true, HostIndeterminateV1: true}, HostResolvingV1: {HostCompilingV1: true, HostDrainingV1: true, HostIndeterminateV1: true}, HostCompilingV1: {HostBindingV1: true, HostDrainingV1: true, HostIndeterminateV1: true}, HostBindingV1: {HostBindingV1: true, HostConstructingV1: true, HostDrainingV1: true, HostIndeterminateV1: true}, HostConstructingV1: {HostConstructingV1: true, HostVerifyingV1: true, HostDrainingV1: true, HostIndeterminateV1: true}, HostVerifyingV1: {HostReadyV1: true, HostDrainingV1: true, HostIndeterminateV1: true}, HostReadyV1: {HostDrainingV1: true, HostIndeterminateV1: true}, HostDrainingV1: {HostReconcilingV1: true, HostIndeterminateV1: true}, HostReconcilingV1: {HostClosedV1: true, HostIndeterminateV1: true}, HostIndeterminateV1: {HostReconcilingV1: true}}

func ValidateJournalSuccessorV1(current, next HostJournalV1) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.HostID != next.HostID || current.StartID != next.StartID || current.ConfigDigest != next.ConfigDigest || current.CreatedUnixNano != next.CreatedUnixNano {
		return NewError(ErrorConflict, "journal_identity_drift", "journal immutable identity drifted")
	}
	if next.Revision != current.Revision+1 || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return NewError(ErrorConflict, "journal_revision_drift", "journal successor is not exact revision plus one")
	}
	if !allowedTransitionsV1[current.Phase][next.Phase] {
		return NewError(ErrorPrecondition, "invalid_phase_transition", "journal phase transition is not allowed")
	}
	if len(next.Constructed) < len(current.Constructed) {
		return NewError(ErrorConflict, "construction_history_regressed", "constructed history cannot shrink")
	}
	if len(next.ConstructionAttempts) < len(current.ConstructionAttempts) {
		return NewError(ErrorConflict, "construction_attempt_history_regressed", "construction attempt history cannot shrink")
	}
	if len(next.ConstructionAttempts) > len(current.ConstructionAttempts)+1 {
		return NewError(ErrorConflict, "construction_attempt_batch_append", "only one planned construction attempt may be appended per successor")
	}
	if len(next.ConstructionAttempts) == len(current.ConstructionAttempts)+1 && next.ConstructionAttempts[len(current.ConstructionAttempts)].State != AttemptPlannedV1 {
		return NewError(ErrorPrecondition, "construction_attempt_first_state_invalid", "a construction attempt must first be appended as planned")
	}
	if len(next.Constructed) > len(current.Constructed)+1 {
		return NewError(ErrorConflict, "construction_batch_append", "only one constructed component may be appended per successor")
	}
	if err := validateBindingAttemptSuccessorV1(current.BindingAttempt, next.BindingAttempt); err != nil {
		return err
	}
	if current.BindingAttempt == nil && next.BindingAttempt != nil && next.BindingRef != nil {
		return NewError(ErrorPrecondition, "binding_attempt_first_state_invalid", "a newly planned binding attempt cannot carry a journal binding ref")
	}
	for _, pair := range [][2]*ExactRefV1{{current.DefinitionRef, next.DefinitionRef}, {current.PlanRef, next.PlanRef}, {current.GenerationRef, next.GenerationRef}, {current.HandoffRef, next.HandoffRef}, {current.BindingRef, next.BindingRef}, {current.GraphRef, next.GraphRef}, {current.ReadyRef, next.ReadyRef}} {
		if pair[0] != nil && (pair[1] == nil || !SameExactRefV1(*pair[0], *pair[1])) {
			return NewError(ErrorConflict, "journal_ref_drift", "journal exact refs are immutable once recorded")
		}
	}
	for i := range current.Constructed {
		if current.Constructed[i] != next.Constructed[i] {
			return NewError(ErrorConflict, "construction_history_drift", "constructed history must be append-only")
		}
	}
	for i := range current.ConstructionAttempts {
		if err := validateConstructionAttemptSuccessorV1(current.ConstructionAttempts[i], next.ConstructionAttempts[i]); err != nil {
			return err
		}
	}
	return nil
}

func validatePhaseClosureV1(j HostJournalV1) error {
	require := func(ref *ExactRefV1, reason string) error {
		if ref == nil {
			return NewError(ErrorPrecondition, reason, "journal phase is missing its exact predecessor ref")
		}
		return nil
	}
	switch j.Phase {
	case HostValidatingV1:
		return require(j.DefinitionRef, "definition_ref_missing")
	case HostResolvingV1:
		if err := require(j.DefinitionRef, "definition_ref_missing"); err != nil {
			return err
		}
		return require(j.PlanRef, "plan_ref_missing")
	case HostCompilingV1:
		for _, x := range []struct {
			r *ExactRefV1
			s string
		}{{j.DefinitionRef, "definition_ref_missing"}, {j.PlanRef, "plan_ref_missing"}, {j.GenerationRef, "generation_ref_missing"}, {j.HandoffRef, "handoff_ref_missing"}, {j.GraphRef, "graph_ref_missing"}} {
			if err := require(x.r, x.s); err != nil {
				return err
			}
		}
	case HostBindingV1, HostConstructingV1, HostVerifyingV1, HostReadyV1:
		for _, x := range []struct {
			r *ExactRefV1
			s string
		}{{j.DefinitionRef, "definition_ref_missing"}, {j.PlanRef, "plan_ref_missing"}, {j.GenerationRef, "generation_ref_missing"}, {j.HandoffRef, "handoff_ref_missing"}, {j.GraphRef, "graph_ref_missing"}} {
			if err := require(x.r, x.s); err != nil {
				return err
			}
		}
		if j.BindingAttempt == nil {
			return NewError(ErrorPrecondition, "binding_attempt_missing", "journal phase is missing its binding attempt")
		}
		if j.Phase != HostBindingV1 {
			if j.BindingAttempt.State != AttemptBoundV1 {
				return NewError(ErrorPrecondition, "binding_attempt_not_bound", "journal phase requires a bound attempt")
			}
			if err := require(j.BindingRef, "binding_ref_missing"); err != nil {
				return err
			}
		}
		if j.Phase == HostReadyV1 {
			return require(j.ReadyRef, "ready_ref_missing")
		}
	}
	return nil
}

func validateBindingAttemptSuccessorV1(current, next *BindingAttemptV1) error {
	if current == nil {
		if next != nil && next.State != AttemptPlannedV1 {
			return NewError(ErrorPrecondition, "binding_attempt_first_state_invalid", "a binding attempt must first be appended as planned")
		}
		return nil
	}
	if next == nil || current.AttemptID != next.AttemptID || current.RequestDigest != next.RequestDigest {
		return NewError(ErrorConflict, "binding_attempt_drift", "binding attempt identity is immutable")
	}
	if current.State == next.State {
		if current.Digest != next.Digest {
			return NewError(ErrorConflict, "binding_attempt_drift", "binding attempt changed without state transition")
		}
		return nil
	}
	if current.State == AttemptPlannedV1 && (next.State == AttemptUnknownV1 || next.State == AttemptBoundV1) {
		return nil
	}
	if current.State == AttemptUnknownV1 && next.State == AttemptBoundV1 {
		return nil
	}
	return NewError(ErrorPrecondition, "binding_attempt_transition_invalid", "binding attempt transition is not monotonic")
}
func validateConstructionAttemptSuccessorV1(current, next ConstructionAttemptV1) error {
	if current.AttemptID != next.AttemptID || current.RequestDigest != next.RequestDigest || current.NodeID != next.NodeID || current.Factory != next.Factory {
		return NewError(ErrorConflict, "construction_attempt_drift", "construction attempt identity is immutable")
	}
	if current.State == next.State {
		if current.Digest != next.Digest {
			return NewError(ErrorConflict, "construction_attempt_drift", "construction attempt changed without state transition")
		}
		return nil
	}
	if current.State == AttemptPlannedV1 && (next.State == AttemptUnknownV1 || next.State == AttemptConstructedV1) {
		return nil
	}
	if current.State == AttemptUnknownV1 && next.State == AttemptConstructedV1 {
		return nil
	}
	return NewError(ErrorPrecondition, "construction_attempt_transition_invalid", "construction attempt transition is not monotonic")
}

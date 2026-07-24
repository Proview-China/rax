package contract

const (
	SystemReadyAttemptContractVersionV2 = "praxis.agent-host/system-ready-attempt/v2"
	SystemReadyAttemptOperationKindV2   = "praxis.agent-host/system-ready"
	SystemReadyAttemptRefKindV2         = "praxis.agent-host/system-ready-attempt"
)

type SystemReadyAttemptStateV2 string

const (
	SystemReadyAttemptIntentRecordedV2         SystemReadyAttemptStateV2 = "intent_recorded"
	SystemReadyAttemptResultRecordedV2         SystemReadyAttemptStateV2 = "result_recorded"
	SystemReadyAttemptOutcomeUnknownV2         SystemReadyAttemptStateV2 = "outcome_unknown"
	SystemReadyAttemptReconciliationRequiredV2 SystemReadyAttemptStateV2 = "reconciliation_required"
)

type SystemReadyAttemptStepKeyV2 struct {
	HostID              string      `json:"host_id"`
	StartID             string      `json:"start_id"`
	HostContractVersion string      `json:"host_contract_version"`
	Phase               HostPhaseV2 `json:"phase"`
	OperationKind       string      `json:"operation_kind"`
	AttemptID           string      `json:"attempt_id"`
}

func NewSystemReadyAttemptStepKeyV2(hostID, startID, attemptID string) SystemReadyAttemptStepKeyV2 {
	return SystemReadyAttemptStepKeyV2{HostID: hostID, StartID: startID, HostContractVersion: ContractVersionV2, Phase: HostReadyV2, OperationKind: SystemReadyAttemptOperationKindV2, AttemptID: attemptID}
}

func (k SystemReadyAttemptStepKeyV2) Validate() error {
	for name, value := range map[string]string{"host": k.HostID, "start": k.StartID, "attempt": k.AttemptID} {
		if err := ValidateIdentifierV1(name, value); err != nil {
			return err
		}
	}
	if k.HostContractVersion != ContractVersionV2 || k.Phase != HostReadyV2 || k.OperationKind != SystemReadyAttemptOperationKindV2 {
		return NewError(ErrorInvalidArgument, "system_ready_attempt_step_key_invalid", "SystemReady attempt step key is not the closed H4 ready operation")
	}
	return nil
}

func DeriveSystemReadyAttemptIDV2(k SystemReadyAttemptStepKeyV2) string {
	d, err := DigestJSONV1(struct {
		Domain string                      `json:"domain"`
		Key    SystemReadyAttemptStepKeyV2 `json:"key"`
	}{Domain: "praxis.agent-host.system-ready-attempt-id-v2", Key: k})
	if err != nil {
		return ""
	}
	return "system-ready-attempt/" + string(d)
}

type SystemReadyAttemptFactV2 struct {
	ContractVersion  string                      `json:"contract_version"`
	StepKey          SystemReadyAttemptStepKeyV2 `json:"step_key"`
	Revision         uint64                      `json:"revision"`
	Request          SystemReadyEnsureRequestV2  `json:"request"`
	FactCandidate    SystemReadyFactV2           `json:"fact_candidate"`
	CurrentCandidate SystemReadyCurrentV2        `json:"current_candidate"`
	State            SystemReadyAttemptStateV2   `json:"state"`
	Result           *SystemReadyGatewayResultV2 `json:"result,omitempty"`
	CreatedUnixNano  int64                       `json:"created_unix_nano"`
	UpdatedUnixNano  int64                       `json:"updated_unix_nano"`
	Digest           DigestV1                    `json:"digest"`
}

func cloneSystemReadyRequestV2(v SystemReadyEnsureRequestV2) SystemReadyEnsureRequestV2 {
	c := v
	c.Components = append([]ComponentProductionCurrentV2{}, v.Components...)
	if v.ExpectedCurrent != nil {
		x := *v.ExpectedCurrent
		c.ExpectedCurrent = &x
	}
	return c
}

func cloneSystemReadyFactV2(v SystemReadyFactV2) SystemReadyFactV2 {
	c := v
	c.Components = append([]ComponentProductionCurrentV2{}, v.Components...)
	return c
}

func (a SystemReadyAttemptFactV2) CloneV2() SystemReadyAttemptFactV2 {
	c := a
	c.Request = cloneSystemReadyRequestV2(a.Request)
	c.FactCandidate = cloneSystemReadyFactV2(a.FactCandidate)
	if a.Result != nil {
		x := *a.Result
		c.Result = &x
	}
	return c
}

func (a SystemReadyAttemptFactV2) digestV2() (DigestV1, error) {
	c := a.CloneV2()
	c.Digest = ""
	return DigestJSONV1(struct {
		Domain string                   `json:"domain"`
		Type   string                   `json:"type"`
		Body   SystemReadyAttemptFactV2 `json:"body"`
	}{Domain: "praxis.agent-host.system-ready-attempt-v2", Type: "SystemReadyAttemptFactV2", Body: c})
}

func SealSystemReadyAttemptFactV2(a SystemReadyAttemptFactV2) (SystemReadyAttemptFactV2, error) {
	a = a.CloneV2()
	if a.ContractVersion != "" && a.ContractVersion != SystemReadyAttemptContractVersionV2 {
		return SystemReadyAttemptFactV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "SystemReady attempt contract version drifted")
	}
	a.ContractVersion = SystemReadyAttemptContractVersionV2
	provided := a.Digest
	a.Digest = ""
	d, err := a.digestV2()
	if err != nil {
		return SystemReadyAttemptFactV2{}, err
	}
	if provided != "" && provided != d {
		return SystemReadyAttemptFactV2{}, NewError(ErrorConflict, "system_ready_attempt_digest_drift", "SystemReady attempt supplied a wrong digest")
	}
	a.Digest = d
	return a, a.Validate()
}

func (a SystemReadyAttemptFactV2) RefV2() ExactRefV1 {
	return ExactRefV1{Kind: SystemReadyAttemptRefKindV2, ID: DeriveSystemReadyAttemptIDV2(a.StepKey), Revision: a.Revision, Digest: a.Digest}
}

func (a SystemReadyAttemptFactV2) Validate() error {
	if a.ContractVersion != SystemReadyAttemptContractVersionV2 || a.Revision == 0 || a.CreatedUnixNano <= 0 || a.UpdatedUnixNano < a.CreatedUnixNano {
		return NewError(ErrorInvalidArgument, "system_ready_attempt_incomplete", "SystemReady attempt is incomplete")
	}
	if err := a.StepKey.Validate(); err != nil {
		return err
	}
	if err := a.Request.Validate(); err != nil {
		return err
	}
	if a.StepKey.HostID != a.Request.HostID || a.StepKey.StartID != a.Request.StartID || a.StepKey.AttemptID != a.Request.AttemptID {
		return NewError(ErrorConflict, "system_ready_attempt_request_subject_drift", "SystemReady attempt step key drifted from its request")
	}
	if err := a.FactCandidate.Validate(); err != nil {
		return err
	}
	if err := a.CurrentCandidate.Validate(); err != nil {
		return err
	}
	if err := validateSystemReadyAttemptCandidatesV2(a); err != nil {
		return err
	}
	switch a.State {
	case SystemReadyAttemptIntentRecordedV2, SystemReadyAttemptOutcomeUnknownV2, SystemReadyAttemptReconciliationRequiredV2:
		if a.Result != nil {
			return NewError(ErrorPrecondition, "system_ready_attempt_result_too_early", "non-terminal SystemReady attempt cannot carry a result")
		}
	case SystemReadyAttemptResultRecordedV2:
		if a.Result == nil {
			return NewError(ErrorPrecondition, "system_ready_attempt_result_missing", "terminal SystemReady attempt requires a result")
		}
		if err := a.Result.Validate(); err != nil {
			return err
		}
		if a.Result.AttemptID != a.Request.AttemptID || a.Result.RequestDigest != a.Request.RequestDigest || a.Result.Fact != a.FactCandidate.Ref || a.Result.Current != a.CurrentCandidate.Ref {
			return NewError(ErrorConflict, "system_ready_attempt_result_drift", "SystemReady attempt result drifted from its candidates")
		}
	default:
		return NewError(ErrorInvalidArgument, "system_ready_attempt_state_invalid", "SystemReady attempt state is unsupported")
	}
	d, err := a.digestV2()
	if err != nil {
		return err
	}
	if d != a.Digest {
		return NewError(ErrorConflict, "system_ready_attempt_digest_drift", "SystemReady attempt digest drifted")
	}
	return nil
}

func validateSystemReadyAttemptCandidatesV2(a SystemReadyAttemptFactV2) error {
	r := a.Request
	f := a.FactCandidate
	c := a.CurrentCandidate
	if f.HostID != r.HostID || f.StartID != r.StartID || f.HostStartClaim != r.Claim || f.DefinitionCurrent != r.Definition || f.PlanCurrent != r.Plan || f.AssemblyCurrent != r.Assembly || f.BindingSetCurrent != r.BindingSet || f.ActivationCurrent != r.Activation || f.GenerationBindingCurrent != r.GenerationBinding || f.ApplicationStartCurrent != r.ApplicationStart || f.SandboxLeaseCurrent != r.SandboxLease || f.SandboxActiveCurrent != r.SandboxActive || f.ExecutionReadyCurrent != r.ExecutionReady || f.SupervisionPolicyCurrent != r.SupervisionPolicy || f.MinimumReadyWindowNanos != r.MinimumReadyWindowNanos {
		return NewError(ErrorConflict, "system_ready_attempt_fact_candidate_drift", "SystemReady Fact candidate drifted from the request")
	}
	if len(f.Components) != len(r.Components) {
		return NewError(ErrorConflict, "system_ready_attempt_component_drift", "SystemReady candidate components drifted")
	}
	for i := range f.Components {
		if f.Components[i] != r.Components[i] {
			return NewError(ErrorConflict, "system_ready_attempt_component_drift", "SystemReady candidate components drifted")
		}
	}
	if c.HostID != r.HostID || c.StartID != r.StartID || c.FactRef != f.Ref || c.State != SystemReadyCurrentReadyV2 || c.Ref.ID != DeriveSystemReadyCurrentIDV2(r.HostID, r.StartID) || c.Ref.Epoch != r.AvailabilityEpoch || c.ExpiresUnixNano != f.ExpiresUnixNano {
		return NewError(ErrorConflict, "system_ready_attempt_current_candidate_drift", "SystemReady Current candidate drifted from the request and Fact")
	}
	if r.ExpectedCurrent == nil {
		if c.Ref.Revision != 1 {
			return NewError(ErrorConflict, "system_ready_attempt_current_revision_drift", "initial SystemReady Current must use revision one")
		}
	} else if c.Ref.Revision != r.ExpectedCurrent.Revision+1 || c.Ref.Epoch != r.ExpectedCurrent.Epoch {
		return NewError(ErrorConflict, "system_ready_attempt_current_revision_drift", "renewed SystemReady Current did not advance the expected revision in the same epoch")
	}
	return nil
}

func ValidateSystemReadyAttemptSuccessorV2(current, next SystemReadyAttemptFactV2) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.StepKey != next.StepKey || current.Request.RequestDigest != next.Request.RequestDigest || current.FactCandidate.Digest != next.FactCandidate.Digest || current.CurrentCandidate.ProjectionDigest != next.CurrentCandidate.ProjectionDigest || current.CreatedUnixNano != next.CreatedUnixNano {
		return NewError(ErrorConflict, "system_ready_attempt_identity_drift", "SystemReady attempt immutable content drifted")
	}
	if next.Revision != current.Revision+1 || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return NewError(ErrorConflict, "system_ready_attempt_revision_drift", "SystemReady attempt successor must advance exactly one revision")
	}
	allowed := current.State == SystemReadyAttemptIntentRecordedV2 && (next.State == SystemReadyAttemptResultRecordedV2 || next.State == SystemReadyAttemptOutcomeUnknownV2) || current.State == SystemReadyAttemptOutcomeUnknownV2 && (next.State == SystemReadyAttemptResultRecordedV2 || next.State == SystemReadyAttemptReconciliationRequiredV2) || current.State == SystemReadyAttemptReconciliationRequiredV2 && (next.State == SystemReadyAttemptResultRecordedV2 || next.State == SystemReadyAttemptReconciliationRequiredV2)
	if !allowed {
		return NewError(ErrorPrecondition, "system_ready_attempt_transition_invalid", "SystemReady attempt transition is not allowed")
	}
	return nil
}

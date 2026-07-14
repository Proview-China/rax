package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const GovernedOperationAttemptContractVersionV3 = "praxis.application.governed-operation-attempt/v3"

type GovernedOperationAttemptStateV3 string

const (
	OperationIntentRecordedV3     GovernedOperationAttemptStateV3 = "intent_recorded"
	OperationDomainReservedV3     GovernedOperationAttemptStateV3 = "domain_reserved"
	OperationEffectAdmittedV3     GovernedOperationAttemptStateV3 = "effect_admitted"
	OperationPermitIssuedV3       GovernedOperationAttemptStateV3 = "permit_issued"
	OperationPermitBegunV3        GovernedOperationAttemptStateV3 = "permit_begun"
	OperationDelegationDeclaredV3 GovernedOperationAttemptStateV3 = "delegation_declared"
	OperationExecutionPreparedV3  GovernedOperationAttemptStateV3 = "execution_prepared"
	OperationProviderObservedV3   GovernedOperationAttemptStateV3 = "provider_observed"
	OperationDispatchUnknownV3    GovernedOperationAttemptStateV3 = "dispatch_unknown"
	OperationSettledV3            GovernedOperationAttemptStateV3 = "settled"
)

// OperationDomainReservationRefV3 is the immutable create-once proof that the
// unique domain owner reserved its subject/session/candidate for one exact
// Application attempt before any Runtime Effect admission.
type OperationDomainReservationRefV3 struct {
	ContractVersion     string                            `json:"contract_version"`
	ID                  string                            `json:"id"`
	Revision            core.Revision                     `json:"revision"`
	Digest              core.Digest                       `json:"digest"`
	StepKind            runtimeports.NamespacedNameV2     `json:"step_kind"`
	Descriptor          StepDescriptorRefV2               `json:"descriptor"`
	DomainAdapter       runtimeports.ProviderBindingRefV2 `json:"domain_adapter"`
	AttemptID           string                            `json:"attempt_id"`
	AttemptRevision     core.Revision                     `json:"attempt_revision"`
	AttemptDigest       core.Digest                       `json:"attempt_digest"`
	IntentDigest        core.Digest                       `json:"intent_digest"`
	DomainSubjectDigest core.Digest                       `json:"domain_subject_digest"`
	SessionRef          string                            `json:"session_ref"`
	CandidateDigest     core.Digest                       `json:"candidate_digest"`
	ReservedUnixNano    int64                             `json:"reserved_unix_nano"`
	ExpiresUnixNano     int64                             `json:"expires_unix_nano"`
}

func (r OperationDomainReservationRefV3) validateShapeV3() error {
	if r.ContractVersion != GovernedOperationAttemptContractVersionV3 || strings.TrimSpace(r.ID) == "" || len(r.ID) > 512 || r.Revision != 1 || strings.TrimSpace(r.AttemptID) == "" || r.AttemptRevision == 0 || strings.TrimSpace(r.SessionRef) == "" || len(r.SessionRef) > 512 || r.ReservedUnixNano <= 0 || r.ExpiresUnixNano <= r.ReservedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "operation domain reservation identity and immutable subject are incomplete")
	}
	if runtimeports.ValidateNamespacedNameV2(r.StepKind) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidNamespace, "operation domain reservation StepKind must be namespaced")
	}
	if err := r.Descriptor.Validate(r.StepKind); err != nil {
		return err
	}
	if err := r.DomainAdapter.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{r.AttemptDigest, r.IntentDigest, r.DomainSubjectDigest, r.CandidateDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (r OperationDomainReservationRefV3) ComputeDigestV3() (core.Digest, error) {
	if err := r.validateShapeV3(); err != nil {
		return "", err
	}
	r.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.operation-domain-reservation", GovernedOperationAttemptContractVersionV3, "OperationDomainReservationRefV3", r)
}

func SealOperationDomainReservationRefV3(r OperationDomainReservationRefV3) (OperationDomainReservationRefV3, error) {
	r.Digest = ""
	digest, err := r.ComputeDigestV3()
	if err != nil {
		return OperationDomainReservationRefV3{}, err
	}
	r.Digest = digest
	return r, r.Validate()
}

func (r OperationDomainReservationRefV3) Validate() error {
	if err := r.validateShapeV3(); err != nil {
		return err
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	digest, err := r.ComputeDigestV3()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "operation domain reservation digest drifted")
	}
	return nil
}

type OperationIntentRefV3 struct {
	EffectID        core.EffectIntentID `json:"effect_id"`
	IntentRevision  core.Revision       `json:"intent_revision"`
	IntentDigest    core.Digest         `json:"intent_digest"`
	OperationDigest core.Digest         `json:"operation_digest"`
}

func NewOperationIntentRefV3(intent runtimeports.OperationEffectIntentV3) (OperationIntentRefV3, error) {
	intentDigest, err := intent.DigestV3()
	if err != nil {
		return OperationIntentRefV3{}, err
	}
	operationDigest, err := intent.Operation.DigestV3()
	if err != nil {
		return OperationIntentRefV3{}, err
	}
	return OperationIntentRefV3{EffectID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, OperationDigest: operationDigest}, nil
}

func (r OperationIntentRefV3) Validate() error {
	if strings.TrimSpace(string(r.EffectID)) == "" || r.IntentRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectIntentMissing, "operation Intent ref is incomplete")
	}
	if err := r.IntentDigest.Validate(); err != nil {
		return err
	}
	return r.OperationDigest.Validate()
}

// OperationDispatchPlanV3 freezes the only values which must exist before
// Issue. It is not a Permit and grants no dispatch authority.
type OperationDispatchPlanV3 struct {
	PermitID       string `json:"permit_id"`
	AttemptID      string `json:"attempt_id"`
	PermitTTLNanos int64  `json:"permit_ttl_nanos"`
}

func (p OperationDispatchPlanV3) Validate() error {
	if strings.TrimSpace(p.PermitID) == "" || len(p.PermitID) > 512 || strings.TrimSpace(p.AttemptID) == "" || len(p.AttemptID) > 512 || p.PermitTTLNanos <= 0 || time.Duration(p.PermitTTLNanos) > runtimeports.MaxDispatchPermitTTL {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "preallocated Permit, attempt and bounded TTL are required")
	}
	return nil
}

// ExecutionDelegationPlanV3 freezes caller-selected relay identity and
// topology before any provider contact. Runtime-owned Operation/Permit values
// are deliberately derived later from IntentValue and Authorization.
type ExecutionDelegationPlanV3 struct {
	ContractVersion                string                             `json:"contract_version"`
	DelegationID                   string                             `json:"delegation_id"`
	HostAdapter                    runtimeports.ProviderBindingRefV2  `json:"host_adapter"`
	RelayHops                      []runtimeports.ExecutionRelayHopV2 `json:"relay_hops"`
	EndpointID                     string                             `json:"endpoint_id"`
	RuntimeSessionRef              string                             `json:"runtime_session_ref"`
	HostBindingExpiresUnixNano     int64                              `json:"host_binding_expires_unix_nano"`
	ProviderBindingExpiresUnixNano int64                              `json:"provider_binding_expires_unix_nano"`
	DelegationTTLNanos             int64                              `json:"delegation_ttl_nanos"`
}

func (p ExecutionDelegationPlanV3) ValidateFor(provider runtimeports.ProviderBindingRefV2) error {
	if p.ContractVersion != GovernedOperationAttemptContractVersionV3 || strings.TrimSpace(p.DelegationID) == "" || len(p.DelegationID) > 512 || strings.TrimSpace(p.EndpointID) == "" || len(p.EndpointID) > 512 || strings.TrimSpace(p.RuntimeSessionRef) == "" || len(p.RuntimeSessionRef) > 512 || p.HostBindingExpiresUnixNano <= 0 || p.ProviderBindingExpiresUnixNano <= 0 || p.DelegationTTLNanos <= 0 || time.Duration(p.DelegationTTLNanos) > runtimeports.MaxExecutionDelegationTTLV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "execution Delegation plan identity, binding lifetimes and bounded TTL are required")
	}
	if err := provider.Validate(); err != nil {
		return err
	}
	if err := p.HostAdapter.Validate(); err != nil {
		return err
	}
	if p.HostAdapter == provider || p.HostAdapter.BindingSetID != provider.BindingSetID || p.HostAdapter.BindingSetRevision != provider.BindingSetRevision {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "Delegation plan host and provider must be distinct bindings in one BindingSet")
	}
	if len(p.RelayHops) == 0 || len(p.RelayHops) > runtimeports.MaxExecutionDelegationHopsV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Delegation plan relay chain is empty or exceeds its bound")
	}
	seen := map[runtimeports.ProviderBindingRefV2]struct{}{}
	for i, hop := range p.RelayHops {
		if err := hop.Validate(); err != nil {
			return err
		}
		if hop.Sequence != uint32(i+1) || i == 0 && hop.Relay != p.HostAdapter || hop.Relay == provider || hop.Relay.BindingSetID != provider.BindingSetID || hop.Relay.BindingSetRevision != provider.BindingSetRevision {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "Delegation plan relay path is noncanonical or crosses its BindingSet")
		}
		if _, ok := seen[hop.Relay]; ok {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonDependencyCycle, "Delegation plan repeats a relay binding")
		}
		seen[hop.Relay] = struct{}{}
	}
	return nil
}

type GovernedOperationAttemptFactV3 struct {
	ContractVersion        string                                           `json:"contract_version"`
	ID                     string                                           `json:"id"`
	Revision               core.Revision                                    `json:"revision"`
	State                  GovernedOperationAttemptStateV3                  `json:"state"`
	Scope                  core.ExecutionScope                              `json:"scope"`
	ScopeDigest            core.Digest                                      `json:"scope_digest"`
	JournalID              string                                           `json:"journal_id"`
	JournalRevision        core.Revision                                    `json:"journal_revision"`
	JournalDigest          core.Digest                                      `json:"journal_digest"`
	PlanID                 string                                           `json:"plan_id"`
	PlanRevision           core.Revision                                    `json:"plan_revision"`
	PlanDigest             core.Digest                                      `json:"plan_digest"`
	StepID                 string                                           `json:"step_id"`
	StepKind               runtimeports.NamespacedNameV2                    `json:"step_kind"`
	Descriptor             StepDescriptorRefV2                              `json:"descriptor"`
	PlannedProvider        runtimeports.ProviderBindingRefV2                `json:"planned_provider"`
	DomainAdapter          runtimeports.ProviderBindingRefV2                `json:"domain_adapter"`
	PlanAuthority          runtimeports.AuthorityBindingRefV2               `json:"plan_authority"`
	RoutingDigest          core.Digest                                      `json:"routing_digest"`
	WorkflowAttempt        uint32                                           `json:"workflow_attempt"`
	Operation              runtimeports.OperationSubjectV3                  `json:"operation"`
	Intent                 OperationIntentRefV3                             `json:"intent"`
	IntentValue            runtimeports.OperationEffectIntentV3             `json:"intent_value"`
	DispatchPlan           OperationDispatchPlanV3                          `json:"dispatch_plan"`
	DelegationPlan         ExecutionDelegationPlanV3                        `json:"delegation_plan"`
	DomainReservation      *OperationDomainReservationRefV3                 `json:"domain_reservation,omitempty"`
	Admission              *runtimeports.OperationEffectAdmissionReceiptV3  `json:"admission,omitempty"`
	IssuedAuthorization    *runtimeports.OperationDispatchAuthorizationV3   `json:"issued_authorization,omitempty"`
	BegunAuthorization     *runtimeports.OperationDispatchAuthorizationV3   `json:"begun_authorization,omitempty"`
	DelegationFact         *runtimeports.ExecutionDelegationFactV2          `json:"declared_delegation_fact,omitempty"`
	DeclaredDelegation     *runtimeports.ExecutionDelegationRefV2           `json:"declared_delegation,omitempty"`
	PreparedDelegation     *runtimeports.ExecutionDelegationRefV2           `json:"prepared_delegation,omitempty"`
	Prepared               *runtimeports.PreparedProviderAttemptRefV2       `json:"prepared,omitempty"`
	Enforcement            *runtimeports.PersistedOperationEnforcementRefV3 `json:"enforcement,omitempty"`
	Observation            *runtimeports.ProviderAttemptObservationRefV2    `json:"observation,omitempty"`
	UnknownAuthorization   *runtimeports.OperationDispatchAuthorizationV3   `json:"unknown_authorization,omitempty"`
	Settlement             *runtimeports.OperationSettlementRefV3           `json:"settlement,omitempty"`
	SettlementDomainResult *runtimeports.OpaquePayloadV2                    `json:"settlement_domain_result,omitempty"`
	CreatedUnixNano        int64                                            `json:"created_unix_nano"`
	UpdatedUnixNano        int64                                            `json:"updated_unix_nano"`
}

func NewGovernedOperationAttemptFactV3(id string, plan WorkflowPlanV2, journal WorkflowJournalV2, stepID string, workflowAttempt uint32, operation runtimeports.OperationSubjectV3, intent runtimeports.OperationEffectIntentV3, dispatchPlan OperationDispatchPlanV3, delegationPlan ExecutionDelegationPlanV3, nowUnixNano int64) (GovernedOperationAttemptFactV3, error) {
	if err := journal.ValidateFor(plan); err != nil {
		return GovernedOperationAttemptFactV3{}, err
	}
	if err := dispatchPlan.Validate(); err != nil {
		return GovernedOperationAttemptFactV3{}, err
	}
	if err := delegationPlan.ValidateFor(intent.Provider); err != nil {
		return GovernedOperationAttemptFactV3{}, err
	}
	planDigest, err := plan.DigestV2()
	if err != nil {
		return GovernedOperationAttemptFactV3{}, err
	}
	journalDigest, err := journal.DigestV2(plan)
	if err != nil {
		return GovernedOperationAttemptFactV3{}, err
	}
	intentRef, err := NewOperationIntentRefV3(intent)
	if err != nil {
		return GovernedOperationAttemptFactV3{}, err
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(plan.Target)
	if err != nil {
		return GovernedOperationAttemptFactV3{}, err
	}
	var step *WorkflowStepV2
	for i := range plan.Steps {
		if plan.Steps[i].ID == stepID {
			step = &plan.Steps[i]
			break
		}
	}
	if step == nil || step.ExecutionClass != StepGovernedEffectV2 || step.Provider == nil || step.DomainAdapter == nil {
		return GovernedOperationAttemptFactV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "operation attempt requires one governed workflow step")
	}
	if *step.Provider != intent.Provider || plan.Authority != intent.Authority {
		return GovernedOperationAttemptFactV3{}, core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "workflow Plan provider/authority differs from operation Intent")
	}
	routingDigest, err := operationRoutingDigestV3(step.Kind, step.Descriptor, *step.Provider, *step.DomainAdapter, plan.Authority)
	if err != nil {
		return GovernedOperationAttemptFactV3{}, err
	}
	f := GovernedOperationAttemptFactV3{ContractVersion: GovernedOperationAttemptContractVersionV3, ID: id, Revision: 1, State: OperationIntentRecordedV3, Scope: plan.Target, ScopeDigest: scopeDigest, JournalID: journal.ID, JournalRevision: journal.Revision, JournalDigest: journalDigest, PlanID: plan.ID, PlanRevision: plan.Revision, PlanDigest: planDigest, StepID: step.ID, StepKind: step.Kind, Descriptor: step.Descriptor, PlannedProvider: *step.Provider, DomainAdapter: *step.DomainAdapter, PlanAuthority: plan.Authority, RoutingDigest: routingDigest, WorkflowAttempt: workflowAttempt, Operation: operation, Intent: intentRef, IntentValue: intent, DispatchPlan: dispatchPlan, DelegationPlan: delegationPlan, CreatedUnixNano: nowUnixNano, UpdatedUnixNano: nowUnixNano}
	return f, f.Validate()
}

func (f GovernedOperationAttemptFactV3) Validate() error {
	if f.ContractVersion != GovernedOperationAttemptContractVersionV3 || strings.TrimSpace(f.ID) == "" || len(f.ID) > 512 || f.Revision == 0 || strings.TrimSpace(f.JournalID) == "" || f.JournalRevision == 0 || strings.TrimSpace(f.PlanID) == "" || f.PlanRevision == 0 || strings.TrimSpace(f.StepID) == "" || f.WorkflowAttempt == 0 || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "governed operation attempt identity is incomplete")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(f.Scope)
	if err != nil || scopeDigest != f.ScopeDigest || !runtimeports.SameExecutionScopeV2(f.Scope, f.Operation.ExecutionScope) {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "operation attempt scope drifted")
	}
	for _, d := range []core.Digest{f.JournalDigest, f.PlanDigest, f.RoutingDigest} {
		if err := d.Validate(); err != nil {
			return err
		}
	}
	if runtimeports.ValidateNamespacedNameV2(f.StepKind) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "operation step kind must remain namespaced")
	}
	if err := f.Descriptor.Validate(f.StepKind); err != nil {
		return err
	}
	if err := f.PlannedProvider.Validate(); err != nil {
		return err
	}
	if err := f.DomainAdapter.Validate(); err != nil {
		return err
	}
	if err := f.PlanAuthority.Validate(); err != nil {
		return err
	}
	routingDigest, err := operationRoutingDigestV3(f.StepKind, f.Descriptor, f.PlannedProvider, f.DomainAdapter, f.PlanAuthority)
	if err != nil {
		return err
	}
	if routingDigest != f.RoutingDigest {
		return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "persisted operation routing bindings drifted from their frozen Plan identity")
	}
	if err := f.Operation.Validate(); err != nil {
		return err
	}
	if err := f.Intent.Validate(); err != nil {
		return err
	}
	if err := f.IntentValue.Validate(); err != nil {
		return err
	}
	actualIntent, err := NewOperationIntentRefV3(f.IntentValue)
	if err != nil || actualIntent != f.Intent || !runtimeports.SameOperationSubjectV3(f.Operation, f.IntentValue.Operation) {
		return stageConflictV3("persisted Intent")
	}
	if f.PlannedProvider != f.IntentValue.Provider || f.PlanAuthority != f.IntentValue.Authority {
		return core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "persisted Plan provider/authority drifted from Intent")
	}
	if err := f.DispatchPlan.Validate(); err != nil {
		return err
	}
	if err := f.DelegationPlan.ValidateFor(f.IntentValue.Provider); err != nil {
		return err
	}
	if f.DelegationPlan.HostBindingExpiresUnixNano <= f.CreatedUnixNano || f.DelegationPlan.ProviderBindingExpiresUnixNano <= f.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "persisted Delegation plan bindings already expired at attempt creation")
	}
	return f.validateStageV3()
}

func (f GovernedOperationAttemptFactV3) validateStageV3() error {
	reserved := f.DomainReservation != nil
	declared := f.DelegationFact != nil || f.DeclaredDelegation != nil
	prepared := f.PreparedDelegation != nil || f.Prepared != nil || f.Enforcement != nil
	unknown := f.UnknownAuthorization != nil
	require := func(want, got bool, name string) error {
		if want != got {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "operation attempt "+name+" presence differs from state")
		}
		return nil
	}
	var needReservation, admission, issued, begun, needDeclared, needPrepared, observation, settlement bool
	switch f.State {
	case OperationIntentRecordedV3:
	case OperationDomainReservedV3:
		needReservation = true
	case OperationEffectAdmittedV3:
		needReservation, admission = true, true
	case OperationPermitIssuedV3:
		needReservation, admission, issued = true, true, true
	case OperationPermitBegunV3:
		needReservation, admission, issued, begun = true, true, true, true
	case OperationDelegationDeclaredV3:
		needReservation, admission, issued, begun, needDeclared = true, true, true, true, true
	case OperationExecutionPreparedV3:
		needReservation, admission, issued, begun, needDeclared, needPrepared = true, true, true, true, true, true
	case OperationProviderObservedV3:
		needReservation, admission, issued, begun, needDeclared, needPrepared, observation = true, true, true, true, true, true, true
	case OperationDispatchUnknownV3:
		needReservation, admission, issued, begun = true, true, true, f.BegunAuthorization != nil
		if !unknown {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "dispatch_unknown requires exact Runtime unknown authorization")
		}
	case OperationSettledV3:
		needReservation, admission, issued, begun, settlement = true, true, true, f.BegunAuthorization != nil, true
		if unknown {
		} else {
			needDeclared, needPrepared, observation = true, true, true
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown governed operation attempt state")
	}
	if err := require(needReservation, reserved, "Domain Reservation"); err != nil {
		return err
	}
	if f.DomainReservation != nil {
		if err := f.DomainReservation.Validate(); err != nil {
			return err
		}
		initial := f
		initial.Revision, initial.State, initial.UpdatedUnixNano = 1, OperationIntentRecordedV3, initial.CreatedUnixNano
		initial.DomainReservation = nil
		initial.Admission, initial.IssuedAuthorization, initial.BegunAuthorization = nil, nil, nil
		initial.DelegationFact, initial.DeclaredDelegation, initial.PreparedDelegation = nil, nil, nil
		initial.Prepared, initial.Enforcement, initial.Observation, initial.UnknownAuthorization = nil, nil, nil, nil
		initial.Settlement, initial.SettlementDomainResult = nil, nil
		initialRef, err := initial.RefV3()
		if err != nil || f.DomainReservation.StepKind != f.StepKind || f.DomainReservation.Descriptor != f.Descriptor || f.DomainReservation.DomainAdapter != f.DomainAdapter || f.DomainReservation.AttemptID != initialRef.ID || f.DomainReservation.AttemptRevision != initialRef.Revision || f.DomainReservation.AttemptDigest != initialRef.Digest || f.DomainReservation.IntentDigest != f.Intent.IntentDigest {
			return stageConflictV3("Domain Reservation")
		}
	}
	for _, check := range []struct {
		want, got bool
		name      string
	}{{admission, f.Admission != nil, "Admission"}, {issued, f.IssuedAuthorization != nil, "issued Authorization"}, {begun, f.BegunAuthorization != nil, "begun Authorization"}, {observation, f.Observation != nil, "Observation"}, {settlement, f.Settlement != nil, "Settlement"}} {
		if err := require(check.want, check.got, check.name); err != nil {
			return err
		}
	}
	resultRequired := settlement && f.Settlement != nil && f.Settlement.DomainResultSchema != nil
	if err := require(resultRequired, f.SettlementDomainResult != nil, "Settlement DomainResult"); err != nil {
		return err
	}
	if f.State != OperationDispatchUnknownV3 && f.State != OperationSettledV3 && unknown {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "unknown authorization is premature")
	}
	if f.State != OperationDispatchUnknownV3 && !(f.State == OperationSettledV3 && unknown) {
		if err := require(needDeclared, declared, "declared Delegation"); err != nil {
			return err
		}
		if err := require(needPrepared, prepared, "Prepared execution"); err != nil {
			return err
		}
	} else {
		if (f.DelegationFact == nil) != (f.DeclaredDelegation == nil) {
			return stageConflictV3("declared Delegation")
		}
		if prepared && (!declared || f.PreparedDelegation == nil || f.Prepared == nil || f.Enforcement == nil) {
			return stageConflictV3("partial Prepared execution")
		}
		if f.BegunAuthorization == nil && (declared || prepared) {
			return stageConflictV3("unknown dispatch without persisted Begin")
		}
	}
	if f.Admission != nil {
		if err := f.Admission.Validate(); err != nil {
			return err
		}
		if f.Admission.OperationDigest != f.Intent.OperationDigest || f.Admission.EffectID != f.Intent.EffectID || f.Admission.IntentRevision != f.Intent.IntentRevision || f.Admission.IntentDigest != f.Intent.IntentDigest {
			return stageConflictV3("Effect Admission")
		}
	}
	if f.IssuedAuthorization != nil {
		if err := f.validateAuthorizationV3(*f.IssuedAuthorization, runtimeports.OperationDispatchAuthorizationIssuedV3); err != nil {
			return err
		}
	}
	if f.BegunAuthorization != nil {
		if err := f.validateAuthorizationV3(*f.BegunAuthorization, runtimeports.OperationDispatchAuthorizationBegunV3); err != nil {
			return err
		}
		if !sameAuthorizationPayloadV3(*f.IssuedAuthorization, *f.BegunAuthorization) || f.BegunAuthorization.EffectFactRevision != f.IssuedAuthorization.EffectFactRevision || f.BegunAuthorization.PermitFactRevision != f.IssuedAuthorization.PermitFactRevision+1 {
			return stageConflictV3("begun Authorization")
		}
	}
	if f.DelegationFact != nil {
		if err := f.validateDelegationV3(); err != nil {
			return err
		}
	}
	if f.Prepared != nil {
		if err := f.validatePreparedV3(); err != nil {
			return err
		}
	}
	if f.Observation != nil {
		if err := f.Observation.Validate(); err != nil {
			return err
		}
		if f.Observation.Delegation != *f.PreparedDelegation || f.Observation.PreparedAttemptID != f.Prepared.ID {
			return stageConflictV3("provider Observation")
		}
	}
	if f.UnknownAuthorization != nil {
		if err := f.validateAuthorizationV3(*f.UnknownAuthorization, runtimeports.OperationDispatchAuthorizationUnknownV3); err != nil {
			return err
		}
		if f.BegunAuthorization == nil {
			if !sameAuthorizationPayloadV3(*f.IssuedAuthorization, *f.UnknownAuthorization) || f.UnknownAuthorization.EffectFactRevision != f.IssuedAuthorization.EffectFactRevision+2 || f.UnknownAuthorization.PermitFactRevision != f.IssuedAuthorization.PermitFactRevision+1 {
				return stageConflictV3("issued-to-unknown Authorization")
			}
		} else if !sameAuthorizationPayloadV3(*f.BegunAuthorization, *f.UnknownAuthorization) || f.UnknownAuthorization.EffectFactRevision != f.BegunAuthorization.EffectFactRevision+1 || !f.validUnknownPermitFactRevisionV3() {
			return stageConflictV3("unknown Authorization")
		}
		if f.Observation != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "unknown dispatch cannot claim provider Observation")
		}
	}
	if f.Settlement != nil {
		return f.validateSettlementV3(unknown)
	}
	return nil
}

func (f GovernedOperationAttemptFactV3) validUnknownPermitFactRevisionV3() bool {
	if f.BegunAuthorization == nil || f.UnknownAuthorization == nil {
		return false
	}
	if f.Prepared == nil {
		return f.Enforcement == nil && f.UnknownAuthorization.PermitFactRevision == f.BegunAuthorization.PermitFactRevision
	}
	if f.Enforcement == nil {
		return false
	}

	// Preparing execution persists the exact provider Enforcement into the
	// Runtime Permit fact. That write is the only admissible source of a
	// post-Begin Permit fact revision advance on the unknown path.
	a := f.BegunAuthorization.Attempt
	prepared := f.Prepared
	enforcement := f.Enforcement
	if prepared.OperationDigest != a.OperationDigest || prepared.IntentID != a.EffectID || prepared.IntentRevision != a.IntentRevision || prepared.IntentDigest != a.IntentDigest || prepared.PermitID != a.PermitID || prepared.PermitRevision != a.PermitRevision || prepared.PermitDigest != a.PermitDigest || prepared.AttemptID != a.AttemptID || enforcement.PermitID != prepared.PermitID || enforcement.PermitRevision != prepared.PermitRevision || enforcement.PermitDigest != prepared.PermitDigest || enforcement.AttemptID != prepared.AttemptID || enforcement.OperationDigest != prepared.OperationDigest || enforcement.Provider != prepared.Provider {
		return false
	}
	if enforcement.RecordedRevision != f.UnknownAuthorization.PermitFactRevision || enforcement.RecordedRevision <= f.BegunAuthorization.PermitFactRevision {
		return false
	}
	return enforcement.RecordedRevision-f.BegunAuthorization.PermitFactRevision == 1
}

func (f GovernedOperationAttemptFactV3) validateAuthorizationV3(a runtimeports.OperationDispatchAuthorizationV3, state runtimeports.OperationDispatchAuthorizationStateV3) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if a.State != state || a.Attempt.EffectID != f.Intent.EffectID || a.Attempt.IntentRevision != f.Intent.IntentRevision || a.Attempt.IntentDigest != f.Intent.IntentDigest || a.Attempt.OperationDigest != f.Intent.OperationDigest || a.Attempt.PermitID != f.DispatchPlan.PermitID || a.Attempt.AttemptID != f.DispatchPlan.AttemptID || !runtimeports.SameOperationSubjectV3(a.Permit.Operation, f.Operation) || a.Permit.ExpiresUnixNano-a.Permit.IssuedUnixNano <= 0 || a.Permit.ExpiresUnixNano-a.Permit.IssuedUnixNano > f.DispatchPlan.PermitTTLNanos {
		return stageConflictV3("dispatch Authorization")
	}
	if a.Attempt.Delegation != nil {
		return stageConflictV3("dispatch Authorization delegation")
	}
	return nil
}

func (f GovernedOperationAttemptFactV3) validateDelegationV3() error {
	if err := f.DelegationFact.Validate(); err != nil {
		return err
	}
	if f.DelegationFact.State != runtimeports.ExecutionDelegationDeclaredV2 {
		return stageConflictV3("declared Delegation state")
	}
	ref, err := f.DelegationFact.RefV2()
	if err != nil || ref != *f.DeclaredDelegation {
		return stageConflictV3("declared Delegation ref")
	}
	a := f.BegunAuthorization.Attempt
	d := f.DelegationFact
	p := f.DelegationPlan
	if d.ID != p.DelegationID || d.HostAdapter != p.HostAdapter || !sameRelayHopsV3(d.RelayHops, p.RelayHops) || d.EndpointID != p.EndpointID || d.RuntimeSessionRef != p.RuntimeSessionRef || d.HostBindingExpiresUnixNano != p.HostBindingExpiresUnixNano || d.ProviderBindingExpiresUnixNano != p.ProviderBindingExpiresUnixNano || d.ExpiresUnixNano-d.CreatedUnixNano > p.DelegationTTLNanos || d.OperationExpiresUnixNano != f.IntentValue.ExpiresUnixNano || d.PermitExpiresUnixNano != f.BegunAuthorization.Permit.ExpiresUnixNano || !runtimeports.SameOperationSubjectV3(d.Operation, f.Operation) || d.IntentID != a.EffectID || d.IntentRevision != a.IntentRevision || d.IntentDigest != a.IntentDigest || d.ProviderPermitID != a.PermitID || d.ProviderPermitRevision != a.PermitRevision || d.ProviderPermitDigest != a.PermitDigest || d.ProviderAttemptID != a.AttemptID || d.DataProvider != f.BegunAuthorization.Permit.Provider {
		return stageConflictV3("declared Delegation")
	}
	return nil
}

func (f GovernedOperationAttemptFactV3) validatePreparedV3() error {
	if err := f.PreparedDelegation.Validate(); err != nil {
		return err
	}
	if f.PreparedDelegation.ID != f.DeclaredDelegation.ID || f.PreparedDelegation.Revision <= f.DeclaredDelegation.Revision {
		return stageConflictV3("prepared Delegation")
	}
	if err := f.Prepared.Validate(); err != nil {
		return err
	}
	if err := f.Enforcement.Validate(); err != nil {
		return err
	}
	a := f.BegunAuthorization.Attempt
	if f.Prepared.DeclaredDelegation != *f.DeclaredDelegation || f.Prepared.OperationDigest != a.OperationDigest || f.Prepared.IntentID != a.EffectID || f.Prepared.IntentRevision != a.IntentRevision || f.Prepared.IntentDigest != a.IntentDigest || f.Prepared.PermitID != a.PermitID || f.Prepared.PermitRevision != a.PermitRevision || f.Prepared.PermitDigest != a.PermitDigest || f.Prepared.AttemptID != a.AttemptID || f.Enforcement.PermitID != f.Prepared.PermitID || f.Enforcement.PermitRevision != f.Prepared.PermitRevision || f.Enforcement.PermitDigest != f.Prepared.PermitDigest || f.Enforcement.AttemptID != f.Prepared.AttemptID || f.Enforcement.OperationDigest != f.Prepared.OperationDigest || f.Enforcement.Provider != f.Prepared.Provider {
		return stageConflictV3("Prepared execution")
	}
	return nil
}

func (f GovernedOperationAttemptFactV3) validateSettlementV3(unknown bool) error {
	if err := f.Settlement.Validate(); err != nil {
		return err
	}
	if f.Settlement.DomainResultSchema == nil {
		if f.SettlementDomainResult != nil {
			return stageConflictV3("unexpected Settlement DomainResult")
		}
	} else {
		if f.SettlementDomainResult == nil {
			return stageConflictV3("missing Settlement DomainResult")
		}
		if err := f.SettlementDomainResult.Validate(); err != nil {
			return err
		}
		if f.SettlementDomainResult.Schema != *f.Settlement.DomainResultSchema || f.SettlementDomainResult.ContentDigest != f.Settlement.DomainResultDigest {
			return stageConflictV3("Settlement DomainResult")
		}
	}
	a := f.Settlement.Attempt
	authorization := f.BegunAuthorization
	if unknown {
		authorization = f.UnknownAuthorization
	}
	expected := authorization.Attempt
	if a.OperationDigest != expected.OperationDigest || a.EffectID != expected.EffectID || a.IntentRevision != expected.IntentRevision || a.IntentDigest != expected.IntentDigest || a.PermitID != expected.PermitID || a.PermitRevision != expected.PermitRevision || a.PermitDigest != expected.PermitDigest || a.AttemptID != expected.AttemptID {
		return stageConflictV3("operation Settlement")
	}
	if unknown {
		if f.Settlement.Observation != nil {
			return stageConflictV3("unknown Settlement Observation")
		}
		if f.PreparedDelegation == nil {
			if a.Delegation != nil {
				return stageConflictV3("pre-prepared unknown Settlement Delegation")
			}
		} else if a.Delegation == nil || *a.Delegation != *f.PreparedDelegation {
			return stageConflictV3("prepared unknown Settlement Delegation")
		}
		return nil
	}
	if a.Delegation == nil || *a.Delegation != *f.PreparedDelegation || f.Settlement.Observation == nil || *f.Settlement.Observation != *f.Observation {
		return stageConflictV3("observed Settlement")
	}
	return nil
}

func sameAuthorizationPayloadV3(a, b runtimeports.OperationDispatchAuthorizationV3) bool {
	a.State = ""
	b.State = ""
	a.EffectFactRevision = 0
	b.EffectFactRevision = 0
	a.PermitFactRevision = 0
	b.PermitFactRevision = 0
	ad, _ := core.CanonicalJSONDigest("praxis.application.governed-operation-attempt", GovernedOperationAttemptContractVersionV3, "authorization-payload", a)
	bd, _ := core.CanonicalJSONDigest("praxis.application.governed-operation-attempt", GovernedOperationAttemptContractVersionV3, "authorization-payload", b)
	return ad == bd
}
func sameRelayHopsV3(a, b []runtimeports.ExecutionRelayHopV2) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func stageConflictV3(name string) error {
	return core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, name+" belongs to another operation attempt")
}

func (f GovernedOperationAttemptFactV3) DigestV3() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.application.governed-operation-attempt", GovernedOperationAttemptContractVersionV3, "GovernedOperationAttemptFactV3", f)
}

type GovernedOperationAttemptRefV3 struct {
	ID                  string                                 `json:"id"`
	Revision            core.Revision                          `json:"revision"`
	State               GovernedOperationAttemptStateV3        `json:"state"`
	Digest              core.Digest                            `json:"digest"`
	ScopeDigest         core.Digest                            `json:"scope_digest"`
	JournalID           string                                 `json:"journal_id"`
	StepID              string                                 `json:"step_id"`
	StepKind            runtimeports.NamespacedNameV2          `json:"step_kind"`
	Descriptor          StepDescriptorRefV2                    `json:"descriptor"`
	PlannedProvider     runtimeports.ProviderBindingRefV2      `json:"planned_provider"`
	DomainAdapter       runtimeports.ProviderBindingRefV2      `json:"domain_adapter"`
	PlanAuthority       runtimeports.AuthorityBindingRefV2     `json:"plan_authority"`
	RoutingDigest       core.Digest                            `json:"routing_digest"`
	WorkflowAttempt     uint32                                 `json:"workflow_attempt"`
	OperationDigest     core.Digest                            `json:"operation_digest"`
	EffectID            core.EffectIntentID                    `json:"effect_id"`
	DispatchUnknown     bool                                   `json:"dispatch_unknown"`
	AuthorizationDigest core.Digest                            `json:"authorization_digest"`
	DomainReservation   *OperationDomainReservationRefV3       `json:"domain_reservation,omitempty"`
	Settlement          *runtimeports.OperationSettlementRefV3 `json:"settlement,omitempty"`
}

func (f GovernedOperationAttemptFactV3) RefV3() (GovernedOperationAttemptRefV3, error) {
	digest, err := f.DigestV3()
	if err != nil {
		return GovernedOperationAttemptRefV3{}, err
	}
	var auth *runtimeports.OperationDispatchAuthorizationV3
	if f.UnknownAuthorization != nil {
		auth = f.UnknownAuthorization
	} else if f.BegunAuthorization != nil {
		auth = f.BegunAuthorization
	} else if f.IssuedAuthorization != nil {
		auth = f.IssuedAuthorization
	}
	authDigest := core.DigestBytes([]byte("authorization-not-yet-created"))
	if auth != nil {
		authDigest, err = core.CanonicalJSONDigest("praxis.application.governed-operation-attempt", GovernedOperationAttemptContractVersionV3, "OperationDispatchAuthorizationV3", auth)
		if err != nil {
			return GovernedOperationAttemptRefV3{}, err
		}
	}
	routingDigest, err := operationRoutingDigestV3(f.StepKind, f.Descriptor, f.PlannedProvider, f.DomainAdapter, f.PlanAuthority)
	if err != nil {
		return GovernedOperationAttemptRefV3{}, err
	}
	if routingDigest != f.RoutingDigest {
		return GovernedOperationAttemptRefV3{}, core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "attempt Fact routing digest drifted")
	}
	r := GovernedOperationAttemptRefV3{ID: f.ID, Revision: f.Revision, State: f.State, Digest: digest, ScopeDigest: f.ScopeDigest, JournalID: f.JournalID, StepID: f.StepID, StepKind: f.StepKind, Descriptor: f.Descriptor, PlannedProvider: f.PlannedProvider, DomainAdapter: f.DomainAdapter, PlanAuthority: f.PlanAuthority, RoutingDigest: routingDigest, WorkflowAttempt: f.WorkflowAttempt, OperationDigest: f.Intent.OperationDigest, EffectID: f.Intent.EffectID, DispatchUnknown: f.UnknownAuthorization != nil, AuthorizationDigest: authDigest}
	if f.DomainReservation != nil {
		reservation := *f.DomainReservation
		r.DomainReservation = &reservation
	}
	if f.Settlement != nil {
		r.Settlement = cloneSettlementRefV3(*f.Settlement)
	}
	return r, r.Validate()
}

func (r GovernedOperationAttemptRefV3) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision == 0 || !validOperationAttemptStateV3(r.State) || strings.TrimSpace(r.JournalID) == "" || strings.TrimSpace(r.StepID) == "" || r.WorkflowAttempt == 0 || strings.TrimSpace(string(r.EffectID)) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "governed operation attempt ref is incomplete")
	}
	for _, d := range []core.Digest{r.Digest, r.ScopeDigest, r.OperationDigest, r.AuthorizationDigest, r.RoutingDigest} {
		if err := d.Validate(); err != nil {
			return err
		}
	}
	if runtimeports.ValidateNamespacedNameV2(r.StepKind) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "attempt ref StepKind must remain namespaced")
	}
	if err := r.Descriptor.Validate(r.StepKind); err != nil {
		return err
	}
	if err := r.PlannedProvider.Validate(); err != nil {
		return err
	}
	if err := r.DomainAdapter.Validate(); err != nil {
		return err
	}
	if err := r.PlanAuthority.Validate(); err != nil {
		return err
	}
	routing, err := operationRoutingDigestV3(r.StepKind, r.Descriptor, r.PlannedProvider, r.DomainAdapter, r.PlanAuthority)
	if err != nil {
		return err
	}
	if routing != r.RoutingDigest {
		return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "attempt ref Domain routing bindings drifted")
	}
	if (r.State == OperationSettledV3) != (r.Settlement != nil) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "only a settled attempt ref carries Settlement")
	}
	if r.DispatchUnknown && r.State != OperationDispatchUnknownV3 && r.State != OperationSettledV3 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "attempt ref unknown marker is premature")
	}
	if (r.State != OperationIntentRecordedV3) != (r.DomainReservation != nil) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "attempt ref Domain Reservation presence differs from state")
	}
	if r.DomainReservation != nil {
		if err := r.DomainReservation.Validate(); err != nil {
			return err
		}
		if r.DomainReservation.StepKind != r.StepKind || r.DomainReservation.Descriptor != r.Descriptor || r.DomainReservation.DomainAdapter != r.DomainAdapter || r.DomainReservation.AttemptID != r.ID || r.DomainReservation.IntentDigest == "" {
			return stageConflictV3("attempt ref Domain Reservation")
		}
	}
	if r.Settlement != nil {
		if err := r.Settlement.Validate(); err != nil {
			return err
		}
		if r.Settlement.Attempt.OperationDigest != r.OperationDigest || r.Settlement.Attempt.EffectID != r.EffectID {
			return stageConflictV3("attempt ref Settlement")
		}
		if r.DispatchUnknown != (r.Settlement.Observation == nil) {
			return stageConflictV3("attempt ref unknown marker")
		}
	}
	return nil
}

func (r GovernedOperationAttemptRefV3) ValidateSettledForV3(settlement runtimeports.OperationSettlementRefV3) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.State != OperationSettledV3 || r.Settlement == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "operation attempt is not settled")
	}
	if err := settlement.Validate(); err != nil {
		return err
	}
	left, _ := core.CanonicalJSONDigest("praxis.application.governed-operation-attempt", GovernedOperationAttemptContractVersionV3, "OperationSettlementRefV3", r.Settlement)
	right, _ := core.CanonicalJSONDigest("praxis.application.governed-operation-attempt", GovernedOperationAttemptContractVersionV3, "OperationSettlementRefV3", settlement)
	if left != right {
		return stageConflictV3("workflow completion Settlement")
	}
	return nil
}

func operationRoutingDigestV3(kind runtimeports.NamespacedNameV2, descriptor StepDescriptorRefV2, provider, domainAdapter runtimeports.ProviderBindingRefV2, authority runtimeports.AuthorityBindingRefV2) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.application.governed-operation-attempt", GovernedOperationAttemptContractVersionV3, "OperationRoutingBindingsV3", struct {
		StepKind        runtimeports.NamespacedNameV2      `json:"step_kind"`
		Descriptor      StepDescriptorRefV2                `json:"descriptor"`
		PlannedProvider runtimeports.ProviderBindingRefV2  `json:"planned_provider"`
		DomainAdapter   runtimeports.ProviderBindingRefV2  `json:"domain_adapter"`
		PlanAuthority   runtimeports.AuthorityBindingRefV2 `json:"plan_authority"`
	}{kind, descriptor, provider, domainAdapter, authority})
}

func ValidateGovernedOperationAttemptTransitionV3(current, next GovernedOperationAttemptFactV3) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if next.DomainAdapter != current.DomainAdapter {
		return stageConflictV3("frozen DomainAdapter")
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if next.Revision != current.Revision+1 || next.UpdatedUnixNano < current.UpdatedUnixNano || !allowedOperationAttemptTransitionV3(current.State, next.State) {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "operation attempt CAS must advance exactly one authorized stage and revision")
	}
	a, b := current, next
	a.Revision, b.Revision = 0, 0
	a.State, b.State = "", ""
	a.UpdatedUnixNano, b.UpdatedUnixNano = 0, 0
	switch next.State {
	case OperationDomainReservedV3:
		b.DomainReservation = nil
	case OperationEffectAdmittedV3:
		b.Admission = nil
	case OperationPermitIssuedV3:
		b.IssuedAuthorization = nil
	case OperationPermitBegunV3:
		b.BegunAuthorization = nil
	case OperationDelegationDeclaredV3:
		b.DelegationFact = nil
		b.DeclaredDelegation = nil
	case OperationExecutionPreparedV3:
		b.PreparedDelegation = nil
		b.Prepared = nil
		b.Enforcement = nil
	case OperationProviderObservedV3:
		b.Observation = nil
	case OperationDispatchUnknownV3:
		b.UnknownAuthorization = nil
	case OperationSettledV3:
		b.Settlement = nil
		b.SettlementDomainResult = nil
	}
	ad, _ := core.CanonicalJSONDigest("praxis.application.governed-operation-attempt", GovernedOperationAttemptContractVersionV3, "predecessor", a)
	bd, _ := core.CanonicalJSONDigest("praxis.application.governed-operation-attempt", GovernedOperationAttemptContractVersionV3, "predecessor", b)
	if ad != bd {
		return stageConflictV3("operation CAS predecessor")
	}
	return nil
}

func allowedOperationAttemptTransitionV3(a, b GovernedOperationAttemptStateV3) bool {
	switch a {
	case OperationIntentRecordedV3:
		return b == OperationDomainReservedV3
	case OperationDomainReservedV3:
		return b == OperationEffectAdmittedV3
	case OperationEffectAdmittedV3:
		return b == OperationPermitIssuedV3
	case OperationPermitIssuedV3:
		return b == OperationPermitBegunV3 || b == OperationDispatchUnknownV3
	case OperationPermitBegunV3:
		return b == OperationDelegationDeclaredV3 || b == OperationDispatchUnknownV3
	case OperationDelegationDeclaredV3:
		return b == OperationExecutionPreparedV3 || b == OperationDispatchUnknownV3
	case OperationExecutionPreparedV3:
		return b == OperationProviderObservedV3 || b == OperationDispatchUnknownV3
	case OperationProviderObservedV3, OperationDispatchUnknownV3:
		return b == OperationSettledV3
	}
	return false
}
func validOperationAttemptStateV3(s GovernedOperationAttemptStateV3) bool {
	return s == OperationIntentRecordedV3 || s == OperationDomainReservedV3 || s == OperationEffectAdmittedV3 || s == OperationPermitIssuedV3 || s == OperationPermitBegunV3 || s == OperationDelegationDeclaredV3 || s == OperationExecutionPreparedV3 || s == OperationProviderObservedV3 || s == OperationDispatchUnknownV3 || s == OperationSettledV3
}

func cloneSettlementRefV3(value runtimeports.OperationSettlementRefV3) *runtimeports.OperationSettlementRefV3 {
	v := value
	v.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), value.Evidence...)
	if value.Attempt.Delegation != nil {
		d := *value.Attempt.Delegation
		v.Attempt.Delegation = &d
	}
	if value.Observation != nil {
		o := *value.Observation
		v.Observation = &o
	}
	if value.DomainResultSchema != nil {
		s := *value.DomainResultSchema
		v.DomainResultSchema = &s
	}
	return &v
}

package runtimeadapter_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type fixedProviderV2 struct {
	prepare      runtimeports.ProviderPreparationAttestationV2
	observation  runtimeports.ProviderAttemptObservationV2
	prepareErr   error
	executeErr   error
	prepareCalls int
	executeCalls int
	inspectCalls int
}

func (p *fixedProviderV2) Prepare(context.Context, runtimeports.PrepareGovernedExecutionRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	p.prepareCalls++
	if p.prepareErr != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, p.prepareErr
	}
	return p.prepare, nil
}

func (p *fixedProviderV2) InspectPrepared(context.Context, runtimeports.InspectPreparedProviderRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	p.inspectCalls++
	return p.prepare, nil
}

func (p *fixedProviderV2) ExecutePrepared(context.Context, runtimeports.ExecutePreparedRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	p.executeCalls++
	if p.executeErr != nil {
		return runtimeports.ProviderAttemptObservationV2{}, p.executeErr
	}
	return p.observation, nil
}

func (p *fixedProviderV2) InspectLocalAttempt(context.Context, runtimeports.InspectLocalProviderAttemptRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	p.inspectCalls++
	return p.observation, nil
}

type replyLossProviderV2 struct {
	mu           sync.Mutex
	prepared     runtimeports.ProviderPreparationAttestationV2
	observation  runtimeports.ProviderAttemptObservationV2
	prepareCalls int
	executeCalls int
	dropPrepare  bool
	dropExecute  bool
}

func (p *replyLossProviderV2) Prepare(_ context.Context, request runtimeports.PrepareGovernedExecutionRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prepareCalls++
	preparedID, _ := runtimeports.DerivePreparedProviderAttemptIDV2(request.Delegation.ID, request.Permit.ID, request.Permit.AttemptID)
	prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{ID: preparedID, Revision: 1, DeclaredDelegation: request.Delegation, OperationDigest: mustOperationDigestV2(request.Intent.Operation), IntentID: request.Intent.ID, IntentRevision: request.Intent.Revision, IntentDigest: request.Permit.IntentDigest, PermitID: request.Permit.ID, PermitRevision: request.Permit.Revision, PermitDigest: mustPermitDigestV2(request.Permit), AttemptID: request.Permit.AttemptID, Provider: request.Permit.Provider, PayloadSchema: request.Intent.Payload.Schema, PayloadDigest: request.Intent.Payload.ContentDigest, PayloadRevision: request.Intent.PayloadRevision, PreparedUnixNano: request.Permit.IssuedUnixNano, ExpiresUnixNano: request.Permit.ExpiresUnixNano})
	if err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	attestation := runtimeports.ProviderPreparationAttestationV2{ContractVersion: runtimeports.ExecutionGovernanceContractVersionV2, Delegation: request.Delegation, Prepared: prepared, Enforcement: runtimeports.OperationEnforcementReceiptV3{ContractVersion: runtimeports.OperationEffectContractVersionV3, PermitID: request.Permit.ID, PermitRevision: request.Permit.Revision, AttemptID: request.Permit.AttemptID, PermitDigest: mustPermitDigestV2(request.Permit), Operation: request.Intent.Operation, Verifier: request.Permit.EnforcementPoint, ValidatedUnixNano: request.Permit.IssuedUnixNano}, ObservedUnixNano: request.Permit.IssuedUnixNano}
	if !p.dropPrepare {
		p.prepared = attestation
	}
	return runtimeports.ProviderPreparationAttestationV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Prepare reply loss")
}

func (p *replyLossProviderV2) InspectPrepared(_ context.Context, request runtimeports.InspectPreparedProviderRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.prepared.Prepared.ID != request.PreparedAttemptID {
		return runtimeports.ProviderPreparationAttestationV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "prepared attempt absent")
	}
	return p.prepared, nil
}

func (p *replyLossProviderV2) ExecutePrepared(_ context.Context, request runtimeports.ExecutePreparedRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.executeCalls++
	payload := relayOpaqueV2("provider-result")
	observation := runtimeports.ProviderAttemptObservationV2{ContractVersion: runtimeports.ExecutionGovernanceContractVersionV2, Delegation: request.Delegation, Prepared: request.Prepared, Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Payload: payload, PayloadRevision: 1, ProviderOperationRef: "provider-operation-relay", SourceRegistrationID: "provider-source-relay", SourceEpoch: 1, SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: relayDigestV2("ledger-scope"), Sequence: 1, RecordDigest: relayDigestV2("record")}, ObservedUnixNano: request.Prepared.PreparedUnixNano + 1}
	if !p.dropExecute {
		p.observation = observation
	}
	return runtimeports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Execute reply loss")
}

func (p *replyLossProviderV2) InspectLocalAttempt(_ context.Context, request runtimeports.InspectLocalProviderAttemptRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.observation.Prepared.ID != request.Prepared.ID {
		return runtimeports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "provider observation absent")
	}
	return p.observation, nil
}

func TestGovernedRelayV2RecoversLostRepliesOnlyByLocalInspect(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	prepare := relayPrepareFixtureV2(t, now)
	provider := &replyLossProviderV2{}
	relay, err := runtimeadapter.NewGovernedRelayV2(provider, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	attestation, err := relay.RelayPrepare(context.Background(), prepare)
	if err != nil {
		t.Fatal(err)
	}
	if provider.prepareCalls != 1 || attestation.Prepared.ID == "" {
		t.Fatalf("Prepare was redispatched or not recovered: calls=%d attestation=%#v", provider.prepareCalls, attestation)
	}
	delegation := runtimeports.ExecutionDelegationRefV2{ID: prepare.Delegation.ID, Revision: prepare.Delegation.Revision + 1, Digest: relayDigestV2("prepared-delegation")}
	enforcement := runtimeports.PersistedOperationEnforcementRefV3{PermitID: prepare.Permit.ID, PermitRevision: prepare.Permit.Revision, PermitDigest: mustPermitDigestV2(prepare.Permit), AttemptID: prepare.Permit.AttemptID, OperationDigest: mustOperationDigestV2(prepare.Intent.Operation), Provider: prepare.Permit.Provider, ReceiptDigest: relayDigestV2("enforcement"), RecordedRevision: 3}
	execute := runtimeports.ExecutePreparedRequestV2{Delegation: delegation, Prepared: attestation.Prepared, Enforcement: enforcement, Intent: prepare.Intent, Permit: prepare.Permit, Fence: prepare.Fence}
	observation, err := relay.RelayExecutePrepared(context.Background(), execute)
	if err != nil {
		t.Fatal(err)
	}
	if provider.executeCalls != 1 || observation.Prepared.ID != attestation.Prepared.ID {
		t.Fatalf("Execute was redispatched or not recovered: calls=%d observation=%#v", provider.executeCalls, observation)
	}
}

func TestGovernedRelayV2NeverRedispatchesWhenLocalInspectCannotProveCall(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	prepare := relayPrepareFixtureV2(t, now)
	droppedPrepare := &replyLossProviderV2{dropPrepare: true}
	relay, _ := runtimeadapter.NewGovernedRelayV2(droppedPrepare, func() time.Time { return now })
	if _, err := relay.RelayPrepare(context.Background(), prepare); !core.HasCategory(err, core.ErrorIndeterminate) || droppedPrepare.prepareCalls != 1 {
		t.Fatalf("unknown Prepare was retried or misclassified: calls=%d err=%v", droppedPrepare.prepareCalls, err)
	}

	provider := &replyLossProviderV2{}
	relay, _ = runtimeadapter.NewGovernedRelayV2(provider, func() time.Time { return now })
	attestation, err := relay.RelayPrepare(context.Background(), prepare)
	if err != nil {
		t.Fatal(err)
	}
	droppedExecute := &replyLossProviderV2{dropExecute: true, prepared: attestation}
	relay, _ = runtimeadapter.NewGovernedRelayV2(droppedExecute, func() time.Time { return now })
	delegation := runtimeports.ExecutionDelegationRefV2{ID: prepare.Delegation.ID, Revision: prepare.Delegation.Revision + 1, Digest: relayDigestV2("prepared-delegation")}
	execute := runtimeports.ExecutePreparedRequestV2{Delegation: delegation, Prepared: attestation.Prepared, Enforcement: runtimeports.PersistedOperationEnforcementRefV3{PermitID: prepare.Permit.ID, PermitRevision: prepare.Permit.Revision, PermitDigest: mustPermitDigestV2(prepare.Permit), AttemptID: prepare.Permit.AttemptID, OperationDigest: mustOperationDigestV2(prepare.Intent.Operation), Provider: prepare.Permit.Provider, ReceiptDigest: relayDigestV2("enforcement"), RecordedRevision: 3}, Intent: prepare.Intent, Permit: prepare.Permit, Fence: prepare.Fence}
	if _, err := relay.RelayExecutePrepared(context.Background(), execute); !core.HasCategory(err, core.ErrorIndeterminate) || droppedExecute.executeCalls != 1 {
		t.Fatalf("unknown Execute was retried or misclassified: calls=%d err=%v", droppedExecute.executeCalls, err)
	}
}

func TestGovernedRelayV2TreatsContextLossAsUnknownAndOnlyInspects(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	prepare, prepared, execute, observation := testkit.GovernedProviderFixtureV2(now)
	provider := &fixedProviderV2{prepare: prepared, observation: observation, prepareErr: context.DeadlineExceeded}
	relay, _ := runtimeadapter.NewGovernedRelayV2(provider, func() time.Time { return now })
	if got, err := relay.RelayPrepare(context.Background(), prepare); err != nil || got.Prepared != prepared.Prepared {
		t.Fatalf("deadline-lost Prepare was not recovered by exact local Inspect: got=%#v err=%v", got, err)
	}
	if provider.prepareCalls != 1 || provider.inspectCalls != 1 {
		t.Fatalf("deadline-lost Prepare was redispatched: prepare=%d inspect=%d", provider.prepareCalls, provider.inspectCalls)
	}
	provider.prepareErr = nil
	provider.executeErr = context.Canceled
	provider.inspectCalls = 0
	if got, err := relay.RelayExecutePrepared(context.Background(), execute); err != nil || got.Prepared != observation.Prepared {
		t.Fatalf("cancel-lost Execute was not recovered by exact local Inspect: got=%#v err=%v", got, err)
	}
	if provider.executeCalls != 1 || provider.inspectCalls != 1 {
		t.Fatalf("cancel-lost Execute was redispatched: execute=%d inspect=%d", provider.executeCalls, provider.inspectCalls)
	}
}

func TestGovernedRelayV2RejectsMaliciousPreparedSubstitution(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	prepare, valid, _, _ := testkit.GovernedProviderFixtureV2(now)
	tests := map[string]func(runtimeports.ProviderPreparationAttestationV2) runtimeports.ProviderPreparationAttestationV2{
		"permit": func(value runtimeports.ProviderPreparationAttestationV2) runtimeports.ProviderPreparationAttestationV2 {
			value.Prepared.PermitDigest = relayDigestV2("forged-permit")
			value.Enforcement.PermitDigest = value.Prepared.PermitDigest
			value.Prepared = resealPreparedV2(t, value.Prepared)
			return value
		},
		"payload": func(value runtimeports.ProviderPreparationAttestationV2) runtimeports.ProviderPreparationAttestationV2 {
			value.Prepared.PayloadDigest = relayDigestV2("forged-payload")
			value.Prepared = resealPreparedV2(t, value.Prepared)
			return value
		},
		"attempt": func(value runtimeports.ProviderPreparationAttestationV2) runtimeports.ProviderPreparationAttestationV2 {
			value.Prepared.AttemptID = "attempt-forged"
			value.Enforcement.AttemptID = value.Prepared.AttemptID
			value.Prepared.ID, _ = runtimeports.DerivePreparedProviderAttemptIDV2(value.Prepared.DeclaredDelegation.ID, value.Prepared.PermitID, value.Prepared.AttemptID)
			value.Prepared = resealPreparedV2(t, value.Prepared)
			return value
		},
		"provider": func(value runtimeports.ProviderPreparationAttestationV2) runtimeports.ProviderPreparationAttestationV2 {
			forged := value.Prepared.Provider
			forged.ComponentID = "custom.provider/forged"
			forged.ManifestDigest = relayDigestV2("forged-manifest")
			value.Prepared.Provider = forged
			value.Enforcement.Verifier = forged
			value.Prepared = resealPreparedV2(t, value.Prepared)
			return value
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			forged := mutate(valid)
			if err := forged.Validate(); err != nil {
				t.Fatalf("test forgery must remain structurally valid: %v", err)
			}
			provider := &fixedProviderV2{prepare: forged}
			relay, _ := runtimeadapter.NewGovernedRelayV2(provider, func() time.Time { return now })
			if _, err := relay.RelayPrepare(context.Background(), prepare); err == nil {
				t.Fatal("malicious Prepare substitution crossed the host relay")
			}
			if provider.prepareCalls != 1 || provider.inspectCalls != 0 {
				t.Fatalf("structural forgery changed relay call cardinality: prepare=%d inspect=%d", provider.prepareCalls, provider.inspectCalls)
			}
		})
	}
}

func TestGovernedRelayV2RejectsMaliciousExecuteAttemptOrDelegation(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	_, prepared, execute, valid := testkit.GovernedProviderFixtureV2(now)
	tests := map[string]func(runtimeports.ProviderAttemptObservationV2) runtimeports.ProviderAttemptObservationV2{
		"prepared-attempt": func(value runtimeports.ProviderAttemptObservationV2) runtimeports.ProviderAttemptObservationV2 {
			value.Prepared.AttemptID = "attempt-forged"
			value.Prepared.ID, _ = runtimeports.DerivePreparedProviderAttemptIDV2(value.Prepared.DeclaredDelegation.ID, value.Prepared.PermitID, value.Prepared.AttemptID)
			value.Prepared = resealPreparedV2(t, value.Prepared)
			return value
		},
		"prepared-payload": func(value runtimeports.ProviderAttemptObservationV2) runtimeports.ProviderAttemptObservationV2 {
			value.Prepared.PayloadDigest = relayDigestV2("forged-payload")
			value.Prepared = resealPreparedV2(t, value.Prepared)
			return value
		},
		"prepared-provider": func(value runtimeports.ProviderAttemptObservationV2) runtimeports.ProviderAttemptObservationV2 {
			value.Prepared.Provider.ComponentID = "custom.provider/forged"
			value.Prepared.Provider.ManifestDigest = relayDigestV2("forged-manifest")
			value.Prepared = resealPreparedV2(t, value.Prepared)
			return value
		},
		"delegation": func(value runtimeports.ProviderAttemptObservationV2) runtimeports.ProviderAttemptObservationV2 {
			value.Delegation.Digest = relayDigestV2("forged-delegation")
			return value
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			forged := mutate(valid)
			if err := forged.Validate(); err != nil {
				t.Fatalf("test forgery must remain structurally valid: %v", err)
			}
			provider := &fixedProviderV2{prepare: prepared, observation: forged}
			relay, _ := runtimeadapter.NewGovernedRelayV2(provider, func() time.Time { return now })
			if _, err := relay.RelayExecutePrepared(context.Background(), execute); err == nil {
				t.Fatal("malicious Execute observation crossed the host relay")
			}
			if provider.executeCalls != 1 || provider.inspectCalls != 0 {
				t.Fatalf("structural forgery changed relay call cardinality: execute=%d inspect=%d", provider.executeCalls, provider.inspectCalls)
			}
		})
	}
}

func TestGovernedRelayV2DirectLocalInspectMethodsFailClosed(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	prepare, prepared, execute, observation := testkit.GovernedProviderFixtureV2(now)
	provider := &fixedProviderV2{prepare: prepared, observation: observation}
	relay, err := runtimeadapter.NewGovernedRelayV2(provider, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	preparedRequest := runtimeports.InspectPreparedProviderRequestV2{
		DeclaredDelegation: prepare.Delegation, PreparedAttemptID: prepared.Prepared.ID,
		PermitID: prepare.Permit.ID, AttemptID: prepare.Permit.AttemptID,
	}
	if got, err := relay.RelayInspectPrepared(context.Background(), preparedRequest); err != nil || got.Prepared != prepared.Prepared {
		t.Fatalf("exact prepared local Inspect failed: got=%#v err=%v", got, err)
	}
	localRequest := runtimeports.InspectLocalProviderAttemptRequestV2{Delegation: execute.Delegation, Prepared: execute.Prepared}
	if got, err := relay.RelayInspectLocalAttempt(context.Background(), localRequest); err != nil || got.Prepared != observation.Prepared {
		t.Fatalf("exact attempt local Inspect failed: got=%#v err=%v", got, err)
	}

	forgedPrepared := prepared
	forgedPrepared.Prepared.AttemptID = "attempt-forged"
	forgedPrepared.Enforcement.AttemptID = forgedPrepared.Prepared.AttemptID
	forgedPrepared.Prepared.ID, _ = runtimeports.DerivePreparedProviderAttemptIDV2(forgedPrepared.Prepared.DeclaredDelegation.ID, forgedPrepared.Prepared.PermitID, forgedPrepared.Prepared.AttemptID)
	forgedPrepared.Prepared = resealPreparedV2(t, forgedPrepared.Prepared)
	provider.prepare = forgedPrepared
	if _, err := relay.RelayInspectPrepared(context.Background(), preparedRequest); err == nil {
		t.Fatal("direct prepared Inspect accepted another provider attempt")
	}

	forgedObservation := observation
	forgedObservation.Delegation.Digest = relayDigestV2("forged-delegation")
	provider.observation = forgedObservation
	if _, err := relay.RelayInspectLocalAttempt(context.Background(), localRequest); err == nil {
		t.Fatal("direct attempt Inspect accepted another delegation")
	}
	if provider.inspectCalls != 4 {
		t.Fatalf("direct Inspect was retried: calls=%d", provider.inspectCalls)
	}
}

func TestNewGovernedRelayV2RequiresProvider(t *testing.T) {
	if relay, err := runtimeadapter.NewGovernedRelayV2(nil, nil); err == nil || relay != nil {
		t.Fatalf("nil custom provider produced a relay: relay=%#v err=%v", relay, err)
	}
}

func resealPreparedV2(t *testing.T, value runtimeports.PreparedProviderAttemptRefV2) runtimeports.PreparedProviderAttemptRefV2 {
	t.Helper()
	value.Digest = ""
	sealed, err := runtimeports.SealPreparedProviderAttemptRefV2(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func relayPrepareFixtureV2(t *testing.T, now time.Time) runtimeports.PrepareGovernedExecutionRequestV2 {
	t.Helper()
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-relay", ID: "identity-relay", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-relay", PlanDigest: relayDigestV2("lineage")}, Instance: core.InstanceRef{ID: "instance-relay", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox-relay", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, RunID: "run-relay", SubjectRevision: 1, CurrentProjectionRef: "current-relay", CurrentProjectionDigest: relayDigestV2("current"), CurrentProjectionRevision: 1}
	operationDigest, _ := operation.DigestV3()
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "binding-relay", BindingSetRevision: 1, ComponentID: "custom.relay/provider", ManifestDigest: relayDigestV2("manifest"), ArtifactDigest: relayDigestV2("artifact"), Capability: "custom.relay/execute"}
	authority := runtimeports.AuthorityBindingRefV2{Ref: "authority-relay", Digest: relayDigestV2("authority"), Revision: 1, Epoch: 1}
	review := runtimeports.OperationReviewBindingRefV3{CaseRef: "review-case-relay", CandidateDigest: relayDigestV2("candidate"), CandidateRevision: 1, PolicyDigest: relayDigestV2("review-policy")}
	budget := runtimeports.OperationBudgetBindingRefV3{Ref: "budget-relay", Digest: relayDigestV2("budget"), Revision: 1, PolicyDigest: relayDigestV2("budget-policy"), SubjectDigest: operationDigest}
	policy := runtimeports.OperationPolicyBindingRefV3{Ref: "policy-relay", Digest: relayDigestV2("policy"), Revision: 1, SubjectDigest: operationDigest}
	intent := runtimeports.OperationEffectIntentV3{ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: "effect-relay", Revision: 1, Operation: operation, Kind: "custom.relay/model", RiskClass: "custom.relay/controlled", ActionScopeDigest: relayDigestV2("action-scope"), Payload: relayOpaqueV2("model-input"), PayloadRevision: 1, Target: "provider/model", ConflictDomain: runtimeports.ConflictDomainBindingV2{Domain: "custom.relay/model", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(scope.Identity.TenantID)}, Owners: []runtimeports.EffectOwnerRefV2{{Role: runtimeports.OwnerCleanup, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest}, {Role: runtimeports.OwnerEffect, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest}, {Role: runtimeports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest}}, Provider: provider, Authority: authority, Review: review, Budget: budget, Policy: policy, Idempotency: runtimeports.IdempotencyBindingV2{Key: "relay-idempotency", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(scope.Identity.TenantID), Class: core.IdempotencyQueryable}, CredentialLeases: []runtimeports.CredentialLeaseRefV2{}, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	intentDigest, err := intent.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	fact := func(ref string) runtimeports.OperationGovernanceFactRefV3 {
		return runtimeports.OperationGovernanceFactRefV3{Ref: ref, Revision: 1, Digest: relayDigestV2(ref), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	}
	reviewAuth := runtimeports.OperationReviewAuthorizationV3{Case: fact(review.CaseRef), CandidateDigest: review.CandidateDigest, CandidateRevision: 1, Verdict: fact("verdict-relay"), ReviewerAuthority: fact("reviewer-relay"), PolicyDigest: review.PolicyDigest, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	permit := runtimeports.OperationDispatchPermitV3{ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: "permit-relay", Revision: 1, AttemptID: "attempt-relay", IntentID: intent.ID, IntentRevision: 1, IntentDigest: intentDigest, Operation: operation, PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: 1, ConflictDomain: intent.ConflictDomain, Provider: provider, EnforcementPoint: provider, Authority: authority, Review: review, ReviewAuthorization: reviewAuth, Budget: budget, Policy: policy, CapabilityGrantDigest: relayDigestV2("capability"), CredentialGrantDigest: relayDigestV2("credentials"), GovernanceSnapshotDigest: relayDigestV2("snapshot"), Idempotency: intent.Idempotency, IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()}
	fence := core.ExecutionFence{BoundaryScope: core.FenceBoundaryInstance, Scope: scope, CapabilityGrantDigest: permit.CapabilityGrantDigest, EffectIntentID: intent.ID, EffectIntentRevision: intent.Revision, CanonicalPayloadDigest: intent.Payload.ContentDigest, ExpiresAt: time.Unix(0, permit.ExpiresUnixNano)}
	fenceDigest, err := runtimeports.DigestOperationExecutionFenceV3(fence, operation)
	if err != nil {
		t.Fatal(err)
	}
	permit.FenceDigest = fenceDigest
	if err := permit.Validate(); err != nil {
		t.Fatal(err)
	}
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "delegation-relay", Revision: 1, Digest: relayDigestV2("delegation")}
	return runtimeports.PrepareGovernedExecutionRequestV2{Delegation: delegation, Intent: intent, Permit: permit, Fence: fence}
}

func relayOpaqueV2(value string) runtimeports.OpaquePayloadV2 {
	bytes := []byte(value)
	return runtimeports.OpaquePayloadV2{Schema: runtimeports.SchemaRefV2{Namespace: "custom.relay", Name: "payload", Version: "1.0.0", MediaType: "application/octet-stream", ContentDigest: relayDigestV2("schema")}, ContentDigest: core.DigestBytes(bytes), Length: uint64(len(bytes)), Inline: bytes, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "custom.relay/limit", Digest: relayDigestV2("limit")}}
}

func relayDigestV2(value string) core.Digest { return core.DigestBytes([]byte(value)) }
func mustOperationDigestV2(value runtimeports.OperationSubjectV3) core.Digest {
	digest, _ := value.DigestV3()
	return digest
}
func mustPermitDigestV2(value runtimeports.OperationDispatchPermitV3) core.Digest {
	digest, _ := value.DigestV3()
	return digest
}

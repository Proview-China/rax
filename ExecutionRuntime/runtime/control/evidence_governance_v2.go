package control

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// EvidenceGovernanceGatewayV2 re-reads every current authority before a new
// append. The raw ledger remains a linearizable persistence primitive and is
// never an Application authority surface.
type EvidenceGovernanceGatewayV2 struct {
	Ledger        ports.EvidenceLedgerFactPortV2
	Bindings      BindingFactPortV2
	Authority     ports.AuthorityFactReaderV2
	CurrentScopes ports.ExecutionScopeFactReaderV2
	Policies      ports.EvidenceSourcePolicyReaderV2
	Runs          RunFactPort
	Effects       EffectFactPortV2
	OwnerFacts    map[ports.NamespacedNameV2]ports.EvidenceOwnerFactReaderV2
	Clock         func() time.Time
}

func (g EvidenceGovernanceGatewayV2) RegisterGovernedSource(ctx context.Context, fact ports.EvidenceSourceRegistrationFactV2) (ports.EvidenceSourceRegistrationFactV2, error) {
	if err := g.validate(); err != nil {
		return ports.EvidenceSourceRegistrationFactV2{}, err
	}
	if existing, err := g.Ledger.InspectSource(ctx, fact.ID); err == nil {
		left, _ := existing.DigestV2()
		right, _ := fact.DigestV2()
		if left == right {
			return existing, nil
		}
		return ports.EvidenceSourceRegistrationFactV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "registration id already binds another source")
	} else if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.EvidenceSourceRegistrationFactV2{}, err
	}
	now := g.Clock()
	if err := ValidateNewEvidenceSourceV2(fact, now); err != nil {
		return ports.EvidenceSourceRegistrationFactV2{}, err
	}
	if _, err := g.inspectSourceGovernance(ctx, fact, now); err != nil {
		return ports.EvidenceSourceRegistrationFactV2{}, err
	}
	return g.Ledger.CreateSource(ctx, fact)
}

func (g EvidenceGovernanceGatewayV2) RenewGovernedSource(ctx context.Context, request ports.EvidenceSourceCASRequestV2) (ports.EvidenceSourceRegistrationFactV2, error) {
	if err := g.validate(); err != nil {
		return ports.EvidenceSourceRegistrationFactV2{}, err
	}
	current, err := g.Ledger.InspectSource(ctx, request.Next.ID)
	if err != nil {
		return ports.EvidenceSourceRegistrationFactV2{}, err
	}
	left, right := current.DigestV2()
	nextDigest, nextErr := request.Next.DigestV2()
	if right == nil && nextErr == nil && left == nextDigest {
		return current, nil
	}
	now := g.Clock()
	if err := ValidateEvidenceSourceTransitionV2(current, request.Next, now); err != nil {
		return ports.EvidenceSourceRegistrationFactV2{}, err
	}
	if _, err := g.inspectSourceGovernance(ctx, request.Next, now); err != nil {
		return ports.EvidenceSourceRegistrationFactV2{}, err
	}
	return g.Ledger.CompareAndSwapSource(ctx, request)
}

func (g EvidenceGovernanceGatewayV2) AppendGoverned(ctx context.Context, request ports.EvidenceAppendRequestV2) (ports.EvidenceLedgerRecordV2, error) {
	return g.append(ctx, request.Candidate, request.ExpectedSourceRevision, false)
}
func (g EvidenceGovernanceGatewayV2) AppendLateGoverned(ctx context.Context, request ports.EvidenceAppendLateRequestV2) (ports.EvidenceLedgerRecordV2, error) {
	return g.append(ctx, request.Candidate, request.ExpectedSourceRevision, true)
}
func (g EvidenceGovernanceGatewayV2) InspectGovernedBySource(ctx context.Context, key ports.EvidenceSourceKeyV2) (ports.EvidenceLedgerRecordV2, error) {
	if err := g.validate(); err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	return g.Ledger.InspectBySource(ctx, key)
}
func (g EvidenceGovernanceGatewayV2) InspectGovernedRecord(ctx context.Context, ref ports.EvidenceRecordRefV2) (ports.EvidenceLedgerRecordV2, error) {
	if err := g.validate(); err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	return g.Ledger.InspectRecord(ctx, ref)
}

func (g EvidenceGovernanceGatewayV2) append(ctx context.Context, candidate ports.EvidenceEventCandidateV2, expected core.Revision, late bool) (ports.EvidenceLedgerRecordV2, error) {
	if err := g.validate(); err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	if err := candidate.Validate(); err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	key := ports.EvidenceSourceKeyV2{RegistrationID: candidate.RegistrationID, SourceEpoch: candidate.SourceEpoch, SourceSequence: candidate.SourceSequence}
	if existing, err := g.Ledger.InspectBySource(ctx, key); err == nil {
		digest, _ := candidate.DigestV2()
		if digest == existing.CandidateDigest {
			return existing, nil
		}
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "same source key changed governed event content")
	} else if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	source, err := g.Ledger.InspectSource(ctx, candidate.RegistrationID)
	if err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	now := g.Clock()
	policy, err := g.inspectSourceGovernance(ctx, source, now)
	if err != nil {
		return ports.EvidenceLedgerRecordV2{}, err
	}
	if late {
		if !policy.AllowLate {
			return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "source policy does not permit late observations")
		}
		if err := ValidateEvidenceLateAppendV2(source, ports.EvidenceAppendLateRequestV2{Candidate: candidate, ExpectedSourceRevision: expected}, now); err != nil {
			return ports.EvidenceLedgerRecordV2{}, err
		}
	} else {
		if err := ValidateEvidenceAppendV2(source, ports.EvidenceAppendRequestV2{Candidate: candidate, ExpectedSourceRevision: expected}, now); err != nil {
			return ports.EvidenceLedgerRecordV2{}, err
		}
	}
	if candidate.TrustClass == ports.EvidenceTrustAuthoritativeFact {
		if err := g.inspectAuthoritativeFact(ctx, candidate, source, policy, now); err != nil {
			return ports.EvidenceLedgerRecordV2{}, err
		}
	}
	if candidate.TrustClass == ports.EvidenceTrustClaim {
		allowed := false
		for _, mapping := range policy.ClaimKinds {
			if mapping.EventKind == candidate.EventKind && mapping.CustomClass == candidate.CustomClass && mapping.ClaimKind == candidate.ClaimKind {
				allowed = true
				break
			}
		}
		if !allowed {
			return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorForbidden, core.ReasonRunClaimUnverified, "source policy does not authorize this event-to-claim mapping")
		}
	}
	if late {
		historical := candidate.HistoricalSource
		record, err := g.Ledger.InspectBySource(ctx, ports.EvidenceSourceKeyV2{RegistrationID: historical.RegistrationID, SourceEpoch: historical.SourceEpoch, SourceSequence: historical.SourceSequence})
		if err != nil {
			return ports.EvidenceLedgerRecordV2{}, err
		}
		if record.Ref != historical.Record || record.CandidateDigest != historical.CandidateDigest || record.Candidate.SourceID != historical.SourceID || record.Candidate.Payload.ContentDigest != historical.ContentDigest {
			return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, "historical source does not match an exact V2 ledger record")
		}
	}
	if late {
		return g.Ledger.AppendLateObservation(ctx, ports.EvidenceAppendLateRequestV2{Candidate: candidate, ExpectedSourceRevision: expected})
	}
	return g.Ledger.Append(ctx, ports.EvidenceAppendRequestV2{Candidate: candidate, ExpectedSourceRevision: expected})
}

func (g EvidenceGovernanceGatewayV2) inspectSourceGovernance(ctx context.Context, source ports.EvidenceSourceRegistrationFactV2, now time.Time) (ports.EvidenceSourcePolicyFactV2, error) {
	set, err := g.Bindings.InspectBindingSet(ctx, source.Producer.BindingSetID)
	if err != nil {
		return ports.EvidenceSourcePolicyFactV2{}, err
	}
	if err := validateEvidenceProducerBindingV2(set, source.Producer, now); err != nil {
		return ports.EvidenceSourcePolicyFactV2{}, err
	}
	capabilityDigest, err := set.CapabilityGrantDigestV2()
	if err != nil {
		return ports.EvidenceSourcePolicyFactV2{}, err
	}
	authority, err := g.Authority.InspectDispatchAuthority(ctx, source.Authority.Ref)
	if err != nil {
		return ports.EvidenceSourcePolicyFactV2{}, err
	}
	if err := authority.ValidateCurrent(source.Authority, source.ExecutionScope, source.ActionScopeDigest, now); err != nil {
		return ports.EvidenceSourcePolicyFactV2{}, err
	}
	current, err := g.CurrentScopes.InspectCurrentExecutionScope(ctx, source.CurrentScope.Ref)
	if err != nil {
		return ports.EvidenceSourcePolicyFactV2{}, err
	}
	if err := current.ValidateCurrentForEvidenceV2(source.LedgerScope.Partition, source.CurrentScope, source.ExecutionScope, source.LedgerScope.RunID, capabilityDigest, now); err != nil {
		return ports.EvidenceSourcePolicyFactV2{}, err
	}
	if current.ProjectionWatermark != source.CurrentScopeWatermark {
		return ports.EvidenceSourcePolicyFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "source current-scope projection watermark drifted")
	}
	policy, err := g.Policies.InspectEvidenceSourcePolicy(ctx, source.Policy.Ref)
	if err != nil {
		return ports.EvidenceSourcePolicyFactV2{}, err
	}
	if err := validateEvidenceSourcePolicyV2(policy, source, now); err != nil {
		return ports.EvidenceSourcePolicyFactV2{}, err
	}
	policyOwnerSet, err := g.Bindings.InspectBindingSet(ctx, policy.PolicyOwner.BindingSetID)
	if err != nil {
		return ports.EvidenceSourcePolicyFactV2{}, err
	}
	if err := validateEvidenceProducerBindingV2(policyOwnerSet, policy.PolicyOwner, now); err != nil {
		return ports.EvidenceSourcePolicyFactV2{}, err
	}
	policyAuthority, err := g.Authority.InspectDispatchAuthority(ctx, policy.PolicyAuthority.Ref)
	if err != nil {
		return ports.EvidenceSourcePolicyFactV2{}, err
	}
	if err := policyAuthority.ValidateCurrent(policy.PolicyAuthority, policy.PolicyScope, policy.ActionScopeDigest, now); err != nil {
		return ports.EvidenceSourcePolicyFactV2{}, err
	}
	if source.ExpiresUnixNano > policy.ExpiresUnixNano || source.ExpiresUnixNano > policyAuthority.ExpiresUnixNano || source.ExpiresUnixNano > policyOwnerSet.ExpiresUnixNano || source.ExpiresUnixNano > authority.ExpiresUnixNano || source.ExpiresUnixNano > current.ExpiresUnixNano || source.ExpiresUnixNano > set.ExpiresUnixNano {
		return ports.EvidenceSourcePolicyFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, "source TTL exceeds a current governance fact")
	}
	activeRunID := current.ActiveRunID
	if source.LedgerScope.Partition == ports.EvidencePartitionRun || source.LedgerScope.Partition == ports.EvidencePartitionEffect {
		activeRunID = source.LedgerScope.RunID
	}
	{
		run, err := g.Runs.InspectRun(ctx, source.ExecutionScope, activeRunID)
		if err != nil {
			return ports.EvidenceSourcePolicyFactV2{}, err
		}
		if (run.Status != core.RunRunning && run.Status != core.RunStopping) || current.ActiveRunID != run.ID || current.RunState != string(run.Status) {
			return ports.EvidenceSourcePolicyFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "evidence run partition is not current")
		}
	}
	if source.LedgerScope.Partition == ports.EvidencePartitionEffect {
		effect, err := g.Effects.InspectEffect(ctx, source.LedgerScope.EffectID)
		if err != nil {
			return ports.EvidenceSourcePolicyFactV2{}, err
		}
		if effect.Intent.ID != source.LedgerScope.EffectID || effect.Intent.RunID != source.LedgerScope.RunID || !ports.SameExecutionScopeV2(effect.Intent.Scope, source.ExecutionScope) {
			return ports.EvidenceSourcePolicyFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "evidence effect partition drifted")
		}
	}
	return policy, nil
}

func (g EvidenceGovernanceGatewayV2) inspectAuthoritativeFact(ctx context.Context, candidate ports.EvidenceEventCandidateV2, source ports.EvidenceSourceRegistrationFactV2, policy ports.EvidenceSourcePolicyFactV2, now time.Time) error {
	if candidate.OwnerFact == nil {
		return core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "authoritative evidence requires owner fact")
	}
	allowed := false
	for _, rule := range policy.OwnerFactRules {
		if rule.EventKind == candidate.EventKind && rule.CustomClass == candidate.CustomClass && rule.FactKind == candidate.OwnerFact.FactKind && rule.OwnerComponent == candidate.OwnerFact.Owner.ComponentID {
			allowed = true
			break
		}
	}
	if !allowed {
		return core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "source policy does not allow this owner fact")
	}
	reader := g.OwnerFacts[candidate.OwnerFact.FactKind]
	if reader == nil {
		return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownGovernanceCategory, "owner fact kind has no registered inspector")
	}
	current, err := reader.InspectEvidenceOwnerFact(ctx, candidate.OwnerFact.FactID)
	if err != nil {
		return err
	}
	if current.Fact != *candidate.OwnerFact || !current.Active || !now.Before(time.Unix(0, current.ExpiresUnixNano)) || !ports.SameExecutionScopeV2(current.Scope, candidate.ExecutionScope) || current.ActionScopeDigest != source.ActionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceTrustInvalid, "owner fact is stale or does not bind candidate payload and scope")
	}
	authority, err := g.Authority.InspectDispatchAuthority(ctx, current.Authority.Ref)
	if err != nil {
		return err
	}
	return authority.ValidateCurrent(current.Authority, current.Scope, current.ActionScopeDigest, now)
}

func (g EvidenceGovernanceGatewayV2) validate() error {
	if g.Ledger == nil || g.Bindings == nil || g.Authority == nil || g.CurrentScopes == nil || g.Policies == nil || g.Runs == nil || g.Effects == nil || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "evidence gateway requires ledger, binding, authority, current scope, policy, run, effect and clock ports")
	}
	return nil
}

func validateEvidenceProducerBindingV2(set BindingSetFactV2, ref ports.EvidenceProducerBindingRefV2, now time.Time) error {
	if err := set.Validate(); err != nil {
		return err
	}
	if set.ID != ref.BindingSetID || set.Revision != ref.BindingSetRevision || set.State != BindingSetActive || !now.Before(time.Unix(0, set.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "evidence producer binding set is stale")
	}
	for _, member := range set.Members {
		if member.ComponentID == ref.ComponentID && member.ManifestDigest == ref.ManifestDigest && member.ArtifactDigest == ref.ArtifactDigest {
			for _, grant := range member.Grants {
				if grant.Capability == ref.Capability && now.Before(time.Unix(0, grant.ExpiresUnixNano)) {
					return nil
				}
			}
		}
	}
	return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownCapability, "evidence producer capability is not current")
}

func validateEvidenceSourcePolicyV2(f ports.EvidenceSourcePolicyFactV2, source ports.EvidenceSourceRegistrationFactV2, now time.Time) error {
	digest, err := f.DigestV2()
	if err != nil {
		return err
	}
	if f.Ref != source.Policy.Ref || f.Revision != source.Policy.Revision || f.Digest != digest || f.Digest != source.Policy.Digest || f.State != ports.EvidenceSourcePolicyActive || f.Producer != source.Producer || f.MaximumSourceTTL <= 0 || !now.Before(time.Unix(0, f.ExpiresUnixNano)) || time.Duration(source.ExpiresUnixNano-source.UpdatedUnixNano) > f.MaximumSourceTTL || !ports.SameExecutionScopeV2(f.PolicyScope, source.ExecutionScope) || f.ActionScopeDigest != source.ActionScopeDigest || f.RequireInstanceEpoch && source.SourceEpoch != source.ExecutionScope.Instance.Epoch {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceTrustInvalid, "evidence source policy is stale or mismatched")
	}
	if !containsEvidencePartitionV2(f.AllowedPartitions, source.LedgerScope.Partition) || !sameEvidenceMappingsV2(f.ClassMappings, source.ClassMappings) || !sameEvidenceKindsV2(f.AllowedKinds, source.AllowedKinds) {
		return core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "source configuration is not granted by policy")
	}
	return nil
}
func containsEvidencePartitionV2(values []ports.EvidencePartitionV2, value ports.EvidencePartitionV2) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

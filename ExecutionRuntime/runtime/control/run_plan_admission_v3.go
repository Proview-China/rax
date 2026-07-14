package control

import (
	"context"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const RunSettlementPlanCertifyCapabilityV3 ports.CapabilityNameV2 = "runtime/certify-run-plan"

// RunSettlementPlanAdmissionGatewayV3 is the sole certification owner. A
// component may publish a declaration, but only this host-control-plane
// gateway can aggregate current Binding, baseline policy and all declarations
// into a Plan certification.
type RunSettlementPlanAdmissionGatewayV3 struct {
	Bindings       BindingFactPortV2
	Declarations   ports.RunSettlementDeclarationReaderV3
	Baselines      ports.RunSettlementBaselinePolicyReaderV3
	Certifications ports.RunSettlementPlanCertificationFactPortV3
	Clock          func() time.Time
}

func (g RunSettlementPlanAdmissionGatewayV3) CertifyRunSettlementPlanV3(ctx context.Context, request ports.CertifyRunSettlementPlanRequestV3) (ports.RunSettlementPlanCertificationFactV3, error) {
	if err := g.validateDependencies(); err != nil {
		return ports.RunSettlementPlanCertificationFactV3{}, err
	}
	if request.TTL <= 0 || request.CertificationID == "" || request.Owner.Validate() != nil || request.BaselinePolicy.Validate() != nil {
		return ports.RunSettlementPlanCertificationFactV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRunSettlementPlanConflict, "Plan certification request identity, owner, baseline and TTL are required")
	}
	if err := request.Run.Validate(); err != nil {
		return ports.RunSettlementPlanCertificationFactV3{}, err
	}
	if request.Run.Status != core.RunPending || request.Run.Revision != 1 || !request.Run.StartedAt.IsZero() {
		return ports.RunSettlementPlanCertificationFactV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunConflict, "Plan certification requires the exact pending Run")
	}
	if err := request.Plan.Validate(); err != nil {
		return ports.RunSettlementPlanCertificationFactV3{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.RunSettlementPlanCertificationFactV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Plan certification clock returned zero")
	}
	inputs, err := g.inspectCurrentInputs(ctx, request.Run, request.Plan, request.BaselinePolicy, request.Owner, now)
	if err != nil {
		return ports.RunSettlementPlanCertificationFactV3{}, err
	}
	requestedExpiry := now.Add(request.TTL).UnixNano()
	expires := minRunPlanAdmissionExpiryV3(requestedExpiry, inputs.minimumExpiry)
	refs := make([]ports.RunSettlementDeclarationRefV3, 0, len(inputs.declarations))
	for _, declaration := range inputs.declarations {
		ref, _ := declaration.RefV3()
		refs = append(refs, ref)
	}
	planRef, _ := request.Plan.RefV2()
	fact, err := ports.SealRunSettlementPlanCertificationFactV3(ports.RunSettlementPlanCertificationFactV3{
		ContractVersion: ports.RunSettlementPlanAdmissionContractVersionV3,
		ID:              request.CertificationID, Revision: 1, RunID: request.Run.ID,
		RunIdentityDigest: request.Plan.RunIdentityDigest, ExecutionScope: request.Run.Scope, ExecutionScopeDigest: request.Plan.ExecutionScopeDigest,
		Plan: planRef, BindingSet: inputs.bindingRef, BaselinePolicy: request.BaselinePolicy, Declarations: refs, CertificationOwner: request.Owner,
		CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		return ports.RunSettlementPlanCertificationFactV3{}, err
	}
	stored, err := g.Certifications.CreateRunSettlementPlanCertificationV3(ctx, fact)
	if err == nil {
		if validateErr := stored.Validate(); validateErr != nil || stored.Digest != fact.Digest {
			return ports.RunSettlementPlanCertificationFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Plan certification owner returned a mismatched fact")
		}
		return stored, nil
	}
	if !recoverableOperationWriteErrorV3(err) {
		return ports.RunSettlementPlanCertificationFactV3{}, err
	}
	inspected, inspectErr := g.Certifications.InspectRunSettlementPlanCertificationV3(context.WithoutCancel(ctx), request.Run.Scope, request.Run.ID)
	if inspectErr != nil || inspected.Validate() != nil {
		return ports.RunSettlementPlanCertificationFactV3{}, core.NewError(core.ErrorIndeterminate, core.ReasonRunSettlementPlanConflict, "cannot prove Plan certification create")
	}
	if inspected.Digest != fact.Digest {
		return ports.RunSettlementPlanCertificationFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Run already has a different authoritative Plan certification")
	}
	return inspected, nil
}

func (g RunSettlementPlanAdmissionGatewayV3) InspectCertifiedRunSettlementPlanV3(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (ports.RunSettlementPlanCertificationFactV3, error) {
	if err := (ports.RunTerminationRequestV3{ExecutionScope: scope, RunID: runID}).Validate(); err != nil {
		return ports.RunSettlementPlanCertificationFactV3{}, err
	}
	if g.Certifications == nil {
		return ports.RunSettlementPlanCertificationFactV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Plan certification owner is required")
	}
	fact, err := g.Certifications.InspectRunSettlementPlanCertificationV3(ctx, scope, runID)
	if err != nil {
		return ports.RunSettlementPlanCertificationFactV3{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.RunSettlementPlanCertificationFactV3{}, err
	}
	return fact, nil
}

func (g RunSettlementPlanAdmissionGatewayV3) ValidateRunSettlementPlanCertificationV3(ctx context.Context, expected ports.RunSettlementPlanCertificationRefV3, run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2) error {
	if err := run.Validate(); err != nil {
		return err
	}
	if err := plan.Validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if err := g.validateDependencies(); err != nil {
		return err
	}
	current, err := g.Certifications.InspectRunSettlementPlanCertificationV3(ctx, run.Scope, run.ID)
	if err != nil {
		return err
	}
	ref, err := current.RefV3()
	if err != nil || ref != expected {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Plan certification ref drifted")
	}
	now := g.Clock()
	if now.IsZero() || !now.Before(time.Unix(0, current.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementPlanConflict, "Plan certification expired")
	}
	planRef, _ := plan.RefV2()
	identity, _ := ports.RunIdentityDigestV2(run)
	if current.Plan != planRef || current.RunID != run.ID || current.RunIdentityDigest != identity || current.ExecutionScopeDigest != plan.ExecutionScopeDigest || !ports.SameExecutionScopeV2(current.ExecutionScope, run.Scope) {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Plan certification belongs to another Run or Plan")
	}
	inputs, err := g.inspectCurrentInputs(ctx, run, plan, current.BaselinePolicy, current.CertificationOwner, now)
	if err != nil {
		return err
	}
	if inputs.bindingRef != current.BindingSet || !sameRunSettlementDeclarationRefsV3(current.Declarations, inputs.declarations) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Plan certification inputs drifted")
	}
	return nil
}

type runPlanAdmissionInputsV3 struct {
	binding       BindingSetFactV2
	bindingRef    ports.RunBindingSetRefV2
	baseline      ports.RunSettlementBaselinePolicyFactV3
	declarations  []ports.RunSettlementDeclarationFactV3
	minimumExpiry int64
}

func (g RunSettlementPlanAdmissionGatewayV3) inspectCurrentInputs(ctx context.Context, run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2, baselineRef ports.RunSettlementBaselinePolicyRefV3, owner ports.EvidenceProducerBindingRefV2, now time.Time) (runPlanAdmissionInputsV3, error) {
	binding, err := g.Bindings.InspectBindingSet(ctx, plan.BindingSet.ID)
	if err != nil {
		return runPlanAdmissionInputsV3{}, err
	}
	bindingDigest, digestErr := BindingSetDigestV2(binding)
	bindingSemantic, semanticErr := BindingSetSemanticDigestV2(binding)
	bindingRef := ports.RunBindingSetRefV2{ID: binding.ID, Revision: binding.Revision, Digest: bindingDigest, SemanticDigest: bindingSemantic}
	if digestErr != nil || semanticErr != nil || binding.State != BindingSetActive || !now.Before(time.Unix(0, binding.ExpiresUnixNano)) || bindingRef != plan.BindingSet {
		return runPlanAdmissionInputsV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Plan certification requires the exact current BindingSet")
	}
	ownerMember, ownerOK := findRunPlanMemberV3(binding, owner)
	if !ownerOK || owner.Capability != RunSettlementPlanCertifyCapabilityV3 || !memberHasCurrentCapabilityV3(ownerMember, RunSettlementPlanCertifyCapabilityV3, now) || countRunPlanCertifiersV3(binding, now) != 1 {
		return runPlanAdmissionInputsV3{}, core.NewError(core.ErrorForbidden, core.ReasonOwnerMissing, "Plan certification owner lacks the current host certification grant")
	}
	ownerFact, err := g.Bindings.InspectBinding(ctx, ownerMember.BindingID)
	if err != nil || ownerFact.Validate() != nil || ownerFact.State != BindingBound || ownerFact.Revision != ownerMember.BindingRevision || ownerFact.BindingSetID != binding.ID || ownerFact.Manifest.Locality != ports.LocalityHostControlPlane || !sameBindingMemberGrantsV3(ownerMember, ownerFact) {
		return runPlanAdmissionInputsV3{}, core.NewError(core.ErrorForbidden, core.ReasonOwnerMissing, "Plan certification owner is not the unique current host-control-plane Binding")
	}
	baseline, err := g.Baselines.InspectRunSettlementBaselinePolicyV3(ctx, baselineRef.ID)
	if err != nil {
		return runPlanAdmissionInputsV3{}, err
	}
	currentBaselineRef, baselineErr := baseline.RefV3()
	if baselineErr != nil || currentBaselineRef != baselineRef || baseline.PolicyOwner != owner || baseline.RunID != run.ID || baseline.RunIdentityDigest != plan.RunIdentityDigest || baseline.ExecutionScopeDigest != plan.ExecutionScopeDigest || !now.Before(time.Unix(0, baseline.ExpiresUnixNano)) || !baselineHasReservedRuntimeRequirementsV3(baseline) {
		return runPlanAdmissionInputsV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementPlanConflict, "Runtime baseline settlement policy drifted")
	}
	minimumExpiry := minRunPlanAdmissionExpiryV3(binding.ExpiresUnixNano, baseline.ExpiresUnixNano, ownerFact.ExpiresUnixNano, capabilityExpiryV3(ownerMember, owner.Capability))
	declarations := make([]ports.RunSettlementDeclarationFactV3, 0, len(binding.Members))
	for _, member := range binding.Members {
		memberFact, inspectFactErr := g.Bindings.InspectBinding(ctx, member.BindingID)
		if inspectFactErr != nil || memberFact.Validate() != nil || memberFact.State != BindingBound || memberFact.Revision != member.BindingRevision || memberFact.BindingSetID != binding.ID || memberFact.ManifestDigest != member.ManifestDigest || memberFact.Manifest.ArtifactDigest != member.ArtifactDigest || !sameBindingMemberGrantsV3(member, memberFact) || !now.Before(time.Unix(0, memberFact.ExpiresUnixNano)) {
			return runPlanAdmissionInputsV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "component Binding Fact drifted from the current set")
		}
		minimumExpiry = minRunPlanAdmissionExpiryV3(minimumExpiry, memberFact.ExpiresUnixNano)
		declaration, inspectErr := g.Declarations.InspectRunSettlementDeclarationV3(ctx, binding.ID, member.ComponentID)
		if inspectErr != nil {
			return runPlanAdmissionInputsV3{}, inspectErr
		}
		if declaration.Validate() != nil || declaration.BindingSetID != binding.ID || declaration.BindingSetRevision != binding.Revision || declaration.BindingRevision != member.BindingRevision || declaration.BindingID != member.BindingID || declaration.ComponentID != member.ComponentID || declaration.ManifestDigest != member.ManifestDigest || declaration.ArtifactDigest != member.ArtifactDigest || !now.Before(time.Unix(0, declaration.ExpiresUnixNano)) {
			return runPlanAdmissionInputsV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "component Run settlement declaration drifted from the current Binding member")
		}
		declarations = append(declarations, declaration)
		minimumExpiry = minRunPlanAdmissionExpiryV3(minimumExpiry, declaration.ExpiresUnixNano)
	}
	if err := validateRunPlanRequirementAggregationV3(plan, baseline, declarations, binding, now); err != nil {
		return runPlanAdmissionInputsV3{}, err
	}
	for _, requirement := range plan.Requirements {
		member, ok := findRunPlanMemberV3(binding, requirement.Owner)
		if !ok {
			return runPlanAdmissionInputsV3{}, core.NewError(core.ErrorForbidden, core.ReasonOwnerMissing, "Plan requirement owner is absent from current BindingSet")
		}
		expiry := capabilityExpiryV3(member, requirement.Owner.Capability)
		if expiry == 0 || !now.Before(time.Unix(0, expiry)) {
			return runPlanAdmissionInputsV3{}, core.NewError(core.ErrorForbidden, core.ReasonOwnerMissing, "Plan requirement owner grant is missing or expired")
		}
		minimumExpiry = minRunPlanAdmissionExpiryV3(minimumExpiry, expiry)
	}
	return runPlanAdmissionInputsV3{binding: binding, bindingRef: bindingRef, baseline: baseline, declarations: declarations, minimumExpiry: minimumExpiry}, nil
}

func sameBindingMemberGrantsV3(member BindingMemberV2, fact BindingFactV2) bool {
	memberDigest, memberErr := BindingGrantSetDigestV2(member.Grants)
	factDigest, factErr := BindingGrantSetDigestV2(fact.Grants)
	return memberErr == nil && factErr == nil && memberDigest == factDigest
}

func countRunPlanCertifiersV3(set BindingSetFactV2, now time.Time) int {
	count := 0
	for _, member := range set.Members {
		if memberHasCurrentCapabilityV3(member, RunSettlementPlanCertifyCapabilityV3, now) {
			count++
		}
	}
	return count
}

func capabilityExpiryV3(member BindingMemberV2, capability ports.CapabilityNameV2) int64 {
	for _, grant := range member.Grants {
		if grant.Capability == capability {
			return grant.ExpiresUnixNano
		}
	}
	return 0
}

func baselineHasReservedRuntimeRequirementsV3(baseline ports.RunSettlementBaselinePolicyFactV3) bool {
	required := map[ports.NamespacedNameV2]ports.RunSettlementRequirementPhaseV2{
		ports.RunRequirementExecutionTruth:      ports.RunSettlementPhaseCompletion,
		ports.RunRequirementEffects:             ports.RunSettlementPhaseCompletion,
		ports.RunRequirementRemoteContinuations: ports.RunSettlementPhaseCompletion,
		ports.RunRequirementDomainCommits:       ports.RunSettlementPhaseCompletion,
		ports.RunRequirementBudget:              ports.RunSettlementPhaseCompletion,
		ports.RunRequirementCleanup:             ports.RunSettlementPhaseTerminationReport,
		ports.RunRequirementResidual:            ports.RunSettlementPhaseTerminationReport,
		ports.RunRequirementProviderRetention:   ports.RunSettlementPhaseTerminationReport,
	}
	seen := map[ports.NamespacedNameV2]int{}
	for _, requirement := range baseline.Requirements {
		if phase, ok := required[requirement.Kind]; ok && phase == requirement.Phase {
			seen[requirement.Kind]++
		}
	}
	for kind := range required {
		if seen[kind] != 1 {
			return false
		}
	}
	return true
}

func validateRunPlanRequirementAggregationV3(plan ports.RunSettlementPlanFactV2, baseline ports.RunSettlementBaselinePolicyFactV3, declarations []ports.RunSettlementDeclarationFactV3, binding BindingSetFactV2, now time.Time) error {
	aggregated := map[ports.NamespacedNameV2]core.Digest{}
	add := func(requirement ports.RunSettlementRequirementV2) error {
		digest, err := requirement.DigestV2()
		if err != nil {
			return err
		}
		if _, exists := aggregated[requirement.ID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "baseline and component declarations contain duplicate requirement IDs")
		}
		member, ok := findRunPlanMemberV3(binding, requirement.Owner)
		if !ok || !memberHasCurrentCapabilityV3(member, requirement.Owner.Capability, now) {
			return core.NewError(core.ErrorForbidden, core.ReasonOwnerMissing, "Run settlement requirement owner/capability is not current in the BindingSet")
		}
		aggregated[requirement.ID] = digest
		return nil
	}
	for _, requirement := range baseline.Requirements {
		if err := add(requirement); err != nil {
			return err
		}
	}
	for _, declaration := range declarations {
		for _, requirement := range declaration.Requirements {
			if requirement.Owner.ComponentID != declaration.ComponentID || requirement.Owner.BindingSetID != declaration.BindingSetID || requirement.Owner.BindingSetRevision != declaration.BindingSetRevision || requirement.Owner.ManifestDigest != declaration.ManifestDigest || requirement.Owner.ArtifactDigest != declaration.ArtifactDigest {
				return core.NewError(core.ErrorForbidden, core.ReasonOwnerMissing, "component declaration attempted to own another Binding member's requirement")
			}
			if err := add(requirement); err != nil {
				return err
			}
		}
	}
	if len(aggregated) != len(plan.Requirements) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementPlanConflict, "certified requirement aggregate does not equal the Plan requirement set")
	}
	for _, requirement := range plan.Requirements {
		digest, _ := requirement.DigestV2()
		if aggregated[requirement.ID] != digest {
			return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Plan requirement, owner, policy or cleanup semantics differ from certified declarations")
		}
	}
	return nil
}

func findRunPlanMemberV3(set BindingSetFactV2, owner ports.EvidenceProducerBindingRefV2) (BindingMemberV2, bool) {
	for _, member := range set.Members {
		if member.ComponentID == owner.ComponentID && member.ManifestDigest == owner.ManifestDigest && member.ArtifactDigest == owner.ArtifactDigest && set.ID == owner.BindingSetID && set.Revision == owner.BindingSetRevision {
			return member, true
		}
	}
	return BindingMemberV2{}, false
}

func memberHasCurrentCapabilityV3(member BindingMemberV2, capability ports.CapabilityNameV2, now time.Time) bool {
	for _, grant := range member.Grants {
		if grant.Capability == capability && now.Before(time.Unix(0, grant.ExpiresUnixNano)) {
			return true
		}
	}
	return false
}

func sameRunSettlementDeclarationRefsV3(expected []ports.RunSettlementDeclarationRefV3, facts []ports.RunSettlementDeclarationFactV3) bool {
	refs := make([]ports.RunSettlementDeclarationRefV3, 0, len(facts))
	for _, fact := range facts {
		ref, err := fact.RefV3()
		if err != nil {
			return false
		}
		refs = append(refs, ref)
	}
	left := append([]ports.RunSettlementDeclarationRefV3{}, expected...)
	sort.Slice(left, func(i, j int) bool { return left[i].ComponentID < left[j].ComponentID })
	sort.Slice(refs, func(i, j int) bool { return refs[i].ComponentID < refs[j].ComponentID })
	if len(left) != len(refs) {
		return false
	}
	for index := range left {
		if left[index] != refs[index] {
			return false
		}
	}
	return true
}

func (g RunSettlementPlanAdmissionGatewayV3) validateDependencies() error {
	if g.Bindings == nil || g.Declarations == nil || g.Baselines == nil || g.Certifications == nil || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Plan certification requires Binding, declaration, baseline, certification and clock owners")
	}
	return nil
}

func minRunPlanAdmissionExpiryV3(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

var _ ports.RunSettlementPlanAdmissionPortV3 = RunSettlementPlanAdmissionGatewayV3{}

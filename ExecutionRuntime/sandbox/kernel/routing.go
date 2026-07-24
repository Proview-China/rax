package kernel

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

type AdmissionDecision struct {
	Admitted bool
	Reasons  []string
}

func EvaluatePlacement(now time.Time, requirement contract.ExecutionRequirement, policy contract.PolicyProjection, backend contract.BackendDescriptor, candidate contract.PlacementCandidate) (AdmissionDecision, error) {
	if err := requirement.ValidateCurrent(now); err != nil {
		return AdmissionDecision{}, fmt.Errorf("requirement: %w", err)
	}
	if err := policy.ValidateCurrent(now); err != nil {
		return AdmissionDecision{}, fmt.Errorf("policy: %w", err)
	}
	if err := backend.ValidateCurrent(now); err != nil {
		return AdmissionDecision{}, fmt.Errorf("backend: %w", err)
	}
	if err := candidate.ValidateCurrent(now); err != nil {
		return AdmissionDecision{}, fmt.Errorf("candidate: %w", err)
	}
	if !contract.SameRef(requirement.Meta.Ref(), policy.RequirementRef) ||
		!contract.SameRef(requirement.Meta.Ref(), candidate.RequirementRef) ||
		!contract.SameRef(policy.Meta.Ref(), candidate.PolicyRef) ||
		!contract.SameRef(backend.Meta.Ref(), candidate.BackendRef) {
		return AdmissionDecision{}, errors.New("placement bindings do not exactly match")
	}
	if err := ValidatePolicyProjection(requirement, policy); err != nil {
		return AdmissionDecision{}, fmt.Errorf("policy projection: %w", err)
	}
	if !policy.ExternalEffectsDisabled {
		return AdmissionDecision{Reasons: []string{"wave 1 requires external_effects_disabled"}}, nil
	}
	reasons := make([]string, 0)
	allowed := slices.Contains(requirement.AllowedSurfaces, backend.Surface)
	if candidate.RequestedDowngrade != "" && candidate.RequestedDowngrade != string(backend.Surface) {
		reasons = append(reasons, "requested downgrade does not match backend surface")
	}
	if !allowed {
		allowed = slices.Contains(requirement.AllowedDowngrades, backend.Surface) && candidate.RequestedDowngrade == string(backend.Surface)
		if !allowed {
			reasons = append(reasons, "surface is neither directly allowed nor explicitly downgraded")
		}
	}
	if backend.Conformance.Rank() < policy.MinimumConformance.Rank() {
		reasons = append(reasons, "backend conformance is below policy minimum")
	}
	for _, capability := range requirement.RequiredCapabilities {
		if backend.Capabilities[capability] != contract.CapabilityEnforced {
			reasons = append(reasons, fmt.Sprintf("required capability %s is not enforced", capability))
		}
	}
	if backend.RawBypass && (requirement.Risk == contract.RiskHigh || requirement.Risk == contract.RiskUntrusted) {
		reasons = append(reasons, "raw bypass is forbidden for high/untrusted risk")
	}
	for _, residual := range backend.ResidualClasses {
		if slices.Contains(requirement.ProhibitedResiduals, residual) {
			reasons = append(reasons, "backend has prohibited residual "+residual)
			continue
		}
		if !slices.Contains(policy.AllowedResiduals, residual) {
			reasons = append(reasons, "backend residual is not explicitly allowed: "+residual)
		}
	}
	if backend.Locality == contract.LocalityRemoteProvider {
		for _, capability := range []contract.BackendCapability{contract.CapabilityInspectAttempt, contract.CapabilityCleanupCoverage, contract.CapabilityProcessFence} {
			if backend.Capabilities[capability] != contract.CapabilityEnforced {
				reasons = append(reasons, fmt.Sprintf("remote backend must enforce %s", capability))
			}
		}
	}
	return AdmissionDecision{Admitted: len(reasons) == 0, Reasons: reasons}, nil
}

func ValidatePolicyProjection(requirement contract.ExecutionRequirement, policy contract.PolicyProjection) error {
	if !contract.SameRef(requirement.Meta.Ref(), policy.RequirementRef) {
		return errors.New("policy does not bind exact requirement")
	}
	for _, scope := range policy.ReadScopes {
		if !logicalScopeWithin(scope, requirement.ReadScopes) {
			return fmt.Errorf("read scope %q expands requirement", scope)
		}
	}
	for _, scope := range policy.WriteScopes {
		if !logicalScopeWithin(scope, requirement.WriteScopes) {
			return fmt.Errorf("write scope %q expands requirement", scope)
		}
	}
	for _, process := range policy.ProcessScopes {
		if !slices.Contains(requirement.ProcessScopes, process) {
			return fmt.Errorf("process scope %q expands requirement", process)
		}
	}
	for _, policySecret := range policy.SecretRefs {
		matched := false
		for _, requirementSecret := range requirement.Secrets {
			if contract.SameRef(policySecret, requirementSecret.SecretRef) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("secret ref %q expands requirement", policySecret.ID)
		}
	}
	if policy.Resources.CPUUnits > requirement.Resources.CPUUnits ||
		policy.Resources.MemoryBytes > requirement.Resources.MemoryBytes ||
		policy.Resources.StorageBytes > requirement.Resources.StorageBytes ||
		policy.Resources.PIDLimit > requirement.Resources.PIDLimit ||
		policy.Resources.WallTimeSeconds > requirement.Resources.WallTimeSeconds {
		return errors.New("policy resource bounds expand requirement")
	}
	if requirement.Network.Mode == contract.NetworkDenyAll && policy.Network.Mode != contract.NetworkDenyAll {
		return errors.New("policy network expands deny_all requirement")
	}
	if requirement.Network.Mode == contract.NetworkAllowList {
		if policy.Network.Mode == contract.NetworkAllowList {
			for _, target := range policy.Network.Targets {
				if !slices.Contains(requirement.Network.Targets, target) {
					return fmt.Errorf("network target %q expands requirement", target)
				}
			}
		}
	}
	for _, residual := range policy.AllowedResiduals {
		if slices.Contains(requirement.ProhibitedResiduals, residual) {
			return fmt.Errorf("policy allows prohibited residual %q", residual)
		}
	}
	return nil
}

func logicalScopeWithin(value string, parents []string) bool {
	for _, parent := range parents {
		if value == parent || strings.HasPrefix(value, parent+"/") {
			return true
		}
	}
	return false
}

package contract

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type ExecutionSurface string

const (
	SurfaceHostWorkspace ExecutionSurface = "host_workspace"
	SurfaceContainer     ExecutionSurface = "container"
	SurfaceMicroVM       ExecutionSurface = "microvm"
	SurfaceWASM          ExecutionSurface = "wasm_capability"
	SurfaceRemoteSandbox ExecutionSurface = "remote_sandbox"
)

func (s ExecutionSurface) Validate() error {
	switch s {
	case SurfaceHostWorkspace, SurfaceContainer, SurfaceMicroVM, SurfaceWASM, SurfaceRemoteSandbox:
		return nil
	default:
		return fmt.Errorf("unsupported execution surface %q", s)
	}
}

func validateSurfaces(values []ExecutionSurface, name string) error {
	seen := make(map[ExecutionSurface]struct{}, len(values))
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return err
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("%s contains duplicate %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

type Locality string

const (
	LocalityHost              Locality = "host"
	LocalityInstanceDataPlane Locality = "instance_data_plane"
	LocalityRemoteProvider    Locality = "remote_provider"
)

func (l Locality) Validate() error {
	switch l {
	case LocalityHost, LocalityInstanceDataPlane, LocalityRemoteProvider:
		return nil
	default:
		return fmt.Errorf("unsupported locality %q", l)
	}
}

type BackendCapability string

const (
	CapabilityExecutionControlled   BackendCapability = "sandbox.execution.controlled"
	CapabilityFilesView             BackendCapability = "sandbox.files.view"
	CapabilityFilesOverlay          BackendCapability = "sandbox.files.overlay"
	CapabilityNetworkDenyAll        BackendCapability = "sandbox.network.deny_all"
	CapabilityNetworkAllowList      BackendCapability = "sandbox.network.allow_list"
	CapabilityProcessFence          BackendCapability = "sandbox.process.fence"
	CapabilitySecretEphemeral       BackendCapability = "sandbox.secret.ephemeral"
	CapabilityResourceLimit         BackendCapability = "sandbox.resource.limit"
	CapabilityInspectPrepared       BackendCapability = "sandbox.inspect.prepared-local"
	CapabilityInspectAttempt        BackendCapability = "sandbox.inspect.attempt-local"
	CapabilityCleanupCoverage       BackendCapability = "sandbox.cleanup.coverage"
	CapabilityCheckpointWorkspace   BackendCapability = "sandbox.checkpoint.workspace"
	CapabilityCheckpointEnvironment BackendCapability = "sandbox.checkpoint.environment"
)

var knownCapabilities = map[BackendCapability]struct{}{
	CapabilityExecutionControlled:   {},
	CapabilityFilesView:             {},
	CapabilityFilesOverlay:          {},
	CapabilityNetworkDenyAll:        {},
	CapabilityNetworkAllowList:      {},
	CapabilityProcessFence:          {},
	CapabilitySecretEphemeral:       {},
	CapabilityResourceLimit:         {},
	CapabilityInspectPrepared:       {},
	CapabilityInspectAttempt:        {},
	CapabilityCleanupCoverage:       {},
	CapabilityCheckpointWorkspace:   {},
	CapabilityCheckpointEnvironment: {},
}

func (c BackendCapability) Validate() error {
	if _, ok := knownCapabilities[c]; !ok {
		return fmt.Errorf("unsupported backend capability %q", c)
	}
	return nil
}

func validateCapabilities(values []BackendCapability) error {
	seen := make(map[BackendCapability]struct{}, len(values))
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return err
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("duplicate backend capability %q", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

type CapabilityLevel string

const (
	CapabilityEnforced     CapabilityLevel = "enforced"
	CapabilityObservedOnly CapabilityLevel = "observed_only"
	CapabilityUnsupported  CapabilityLevel = "unsupported"
)

func (l CapabilityLevel) Validate() error {
	switch l {
	case CapabilityEnforced, CapabilityObservedOnly, CapabilityUnsupported:
		return nil
	default:
		return fmt.Errorf("unsupported capability level %q", l)
	}
}

type ConformanceLevel string

const (
	ConformanceFullyControlled      ConformanceLevel = "fully_controlled"
	ConformanceRestrictedControlled ConformanceLevel = "restricted_controlled"
	ConformanceContainedObserveOnly ConformanceLevel = "contained_observe_only"
	ConformanceRejected             ConformanceLevel = "rejected"
)

func (l ConformanceLevel) Validate() error {
	switch l {
	case ConformanceFullyControlled, ConformanceRestrictedControlled, ConformanceContainedObserveOnly, ConformanceRejected:
		return nil
	default:
		return fmt.Errorf("unsupported conformance level %q", l)
	}
}

func (l ConformanceLevel) Rank() int {
	switch l {
	case ConformanceFullyControlled:
		return 3
	case ConformanceRestrictedControlled:
		return 2
	case ConformanceContainedObserveOnly:
		return 1
	default:
		return 0
	}
}

type BackendDescriptor struct {
	Meta               Meta                                  `json:"meta"`
	Surface            ExecutionSurface                      `json:"surface"`
	Locality           Locality                              `json:"locality"`
	ArtifactRef        Ref                                   `json:"artifact_ref"`
	BackendContractRef Ref                                   `json:"backend_contract_ref"`
	Capabilities       map[BackendCapability]CapabilityLevel `json:"capabilities"`
	RawBypass          bool                                  `json:"raw_bypass"`
	ResidualClasses    []string                              `json:"residual_classes,omitempty"`
	Conformance        ConformanceLevel                      `json:"conformance"`
	ConformanceRef     Ref                                   `json:"conformance_ref"`
}

func (d BackendDescriptor) ValidateShape() error {
	if err := d.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := d.Surface.Validate(); err != nil {
		return err
	}
	if err := d.Locality.Validate(); err != nil {
		return err
	}
	if err := d.ArtifactRef.ValidateShape("backend artifact ref"); err != nil {
		return err
	}
	if err := d.BackendContractRef.ValidateShape("backend contract ref"); err != nil {
		return err
	}
	if err := d.Conformance.Validate(); err != nil {
		return err
	}
	if err := d.ConformanceRef.ValidateShape("conformance ref"); err != nil {
		return err
	}
	if len(d.Capabilities) == 0 {
		return errors.New("backend capabilities are required")
	}
	for capability, level := range d.Capabilities {
		if err := capability.Validate(); err != nil {
			return err
		}
		if err := level.Validate(); err != nil {
			return err
		}
	}
	return ValidateSortedUnique(d.ResidualClasses, "backend residual classes")
}

func (d BackendDescriptor) ValidateCurrent(now time.Time) error {
	if err := d.ValidateShape(); err != nil {
		return err
	}
	return d.Meta.ValidateCurrent(now)
}

type PlacementCandidate struct {
	Meta                  Meta   `json:"meta"`
	RequirementRef        Ref    `json:"requirement_ref"`
	PolicyRef             Ref    `json:"policy_ref"`
	BackendRef            Ref    `json:"backend_ref"`
	SlotCandidateRef      Ref    `json:"slot_candidate_ref"`
	MatchEvidenceRefs     []Ref  `json:"match_evidence_refs"`
	RequestedDowngrade    string `json:"requested_downgrade,omitempty"`
	CostObservationDigest string `json:"cost_observation_digest,omitempty"`
}

func (c PlacementCandidate) ValidateShape() error {
	if err := c.Meta.ValidateShape(); err != nil {
		return err
	}
	for name, ref := range map[string]Ref{
		"requirement":    c.RequirementRef,
		"policy":         c.PolicyRef,
		"backend":        c.BackendRef,
		"slot candidate": c.SlotCandidateRef,
	} {
		if err := ref.ValidateShape(name + " ref"); err != nil {
			return err
		}
	}
	if len(c.MatchEvidenceRefs) == 0 {
		return errors.New("placement candidate needs match evidence refs")
	}
	for _, ref := range c.MatchEvidenceRefs {
		if err := ref.ValidateShape("match evidence ref"); err != nil {
			return err
		}
	}
	if c.CostObservationDigest != "" && !ValidDigest(c.CostObservationDigest) {
		return errors.New("cost observation digest is invalid")
	}
	if strings.Contains(strings.ToLower(c.RequestedDowngrade), "automatic") {
		return errors.New("automatic placement downgrade is forbidden")
	}
	return nil
}

func (c PlacementCandidate) ValidateCurrent(now time.Time) error {
	if err := c.ValidateShape(); err != nil {
		return err
	}
	return c.Meta.ValidateCurrent(now)
}

type ConformanceDisposition string

const (
	ConformanceAdmitted                 ConformanceDisposition = "admitted"
	ConformanceAdmittedExplicitResidual ConformanceDisposition = "admitted_with_explicit_residual"
	ConformanceObserveOnly              ConformanceDisposition = "observe_only"
	ConformanceDenied                   ConformanceDisposition = "rejected"
)

type BackendConformanceReport struct {
	Meta              Meta                                  `json:"meta"`
	BackendRef        Ref                                   `json:"backend_ref"`
	RequirementRef    Ref                                   `json:"requirement_ref"`
	Disposition       ConformanceDisposition                `json:"disposition"`
	CapabilityResults map[BackendCapability]CapabilityLevel `json:"capability_results"`
	Residuals         []string                              `json:"residuals,omitempty"`
	EvidenceRefs      []Ref                                 `json:"evidence_refs"`
	ProductionProof   bool                                  `json:"production_proof"`
}

func (r BackendConformanceReport) ValidateShape() error {
	if err := r.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := r.BackendRef.ValidateShape("backend ref"); err != nil {
		return err
	}
	if err := r.RequirementRef.ValidateShape("requirement ref"); err != nil {
		return err
	}
	switch r.Disposition {
	case ConformanceAdmitted, ConformanceAdmittedExplicitResidual, ConformanceObserveOnly, ConformanceDenied:
	default:
		return fmt.Errorf("unsupported conformance disposition %q", r.Disposition)
	}
	if len(r.CapabilityResults) == 0 || len(r.EvidenceRefs) == 0 {
		return errors.New("capability results and evidence refs are required")
	}
	for capability, level := range r.CapabilityResults {
		if err := capability.Validate(); err != nil {
			return err
		}
		if err := level.Validate(); err != nil {
			return err
		}
	}
	for _, ref := range r.EvidenceRefs {
		if err := ref.ValidateShape("conformance evidence ref"); err != nil {
			return err
		}
	}
	if err := ValidateSortedUnique(r.Residuals, "conformance residuals"); err != nil {
		return err
	}
	return nil
}

func (r BackendConformanceReport) ValidateCurrent(now time.Time) error {
	if err := r.ValidateShape(); err != nil {
		return err
	}
	return r.Meta.ValidateCurrent(now)
}

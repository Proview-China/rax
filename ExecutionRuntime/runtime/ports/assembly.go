package ports

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ComponentRequirement struct {
	ID             string        `json:"component_id"`
	Kind           ComponentKind `json:"component_kind"`
	Version        string        `json:"version"`
	ArtifactDigest core.Digest   `json:"artifact_digest"`
	Required       bool          `json:"required"`
	AllowResidual  bool          `json:"allow_residual"`
	Capabilities   []string      `json:"capabilities"`
	DependsOn      []string      `json:"depends_on,omitempty"`
}

func (r ComponentRequirement) Validate() error {
	if strings.TrimSpace(r.ID) == "" || !validComponentKind(r.Kind) || strings.TrimSpace(r.Version) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "component requirement identity, kind and version are required")
	}
	if err := r.ArtifactDigest.Validate(); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(r.Capabilities))
	for _, capability := range r.Capabilities {
		if strings.TrimSpace(capability) == "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "capability name cannot be empty")
		}
		if _, exists := seen[capability]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "duplicate required capability")
		}
		seen[capability] = struct{}{}
	}
	dependencies := make(map[string]struct{}, len(r.DependsOn))
	for _, dependency := range r.DependsOn {
		if strings.TrimSpace(dependency) == "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "component dependency cannot be empty")
		}
		if _, exists := dependencies[dependency]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "duplicate component dependency")
		}
		dependencies[dependency] = struct{}{}
	}
	return nil
}

type ResolvedAgentPlan struct {
	ID              string                 `json:"resolved_plan_id"`
	Digest          core.Digest            `json:"resolved_plan_digest"`
	ProfileDigest   core.Digest            `json:"profile_digest"`
	ContextDigest   core.Digest            `json:"context_digest"`
	AuthorityDigest core.Digest            `json:"authority_digest"`
	Requirements    []ComponentRequirement `json:"component_requirements"`
}

func (p ResolvedAgentPlan) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "resolved plan id is required")
	}
	for _, digest := range []core.Digest{p.Digest, p.ProfileDigest, p.ContextDigest, p.AuthorityDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	ids := make(map[string]struct{}, len(p.Requirements))
	dependencies := make(map[string][]string, len(p.Requirements))
	for _, requirement := range p.Requirements {
		if err := requirement.Validate(); err != nil {
			return err
		}
		if _, exists := ids[requirement.ID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "duplicate component requirement")
		}
		ids[requirement.ID] = struct{}{}
		dependencies[requirement.ID] = append([]string(nil), requirement.DependsOn...)
	}
	for id, refs := range dependencies {
		for _, ref := range refs {
			if ref == id {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonDependencyCycle, "component cannot depend on itself")
			}
			if _, exists := ids[ref]; !exists {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "component dependency is absent from the resolved plan")
			}
		}
	}
	if hasDependencyCycle(dependencies) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDependencyCycle, "component dependency graph contains a cycle")
	}
	return nil
}

type Describer interface {
	Describe(context.Context) (ComponentDescriptor, error)
}

type RegisteredComponent struct {
	Descriptor ComponentDescriptor
	Adapter    Describer
}

type BindingResidual struct {
	ComponentID string `json:"component_id"`
	Reason      string `json:"reason"`
}

type BindingSet struct {
	Descriptors []ComponentDescriptor `json:"descriptors"`
	Residuals   []BindingResidual     `json:"residuals,omitempty"`
}

type ComponentRegistry struct {
	mu         sync.RWMutex
	components map[string]RegisteredComponent
}

func NewComponentRegistry() *ComponentRegistry {
	return &ComponentRegistry{components: make(map[string]RegisteredComponent)}
}

func (r *ComponentRegistry) Register(ctx context.Context, adapter Describer) error {
	if adapter == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "component adapter is required")
	}
	descriptor, err := adapter.Describe(ctx)
	if err != nil {
		return err
	}
	if err := descriptor.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.components[descriptor.ID]; exists {
		return core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "component id is already registered")
	}
	r.components[descriptor.ID] = RegisteredComponent{Descriptor: descriptor, Adapter: adapter}
	return nil
}

func (r *ComponentRegistry) Resolve(ctx context.Context, plan ResolvedAgentPlan, now time.Time) (BindingSet, error) {
	if err := plan.Validate(); err != nil {
		return BindingSet{}, err
	}
	if now.IsZero() {
		return BindingSet{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "binding validation time is required")
	}
	r.mu.RLock()
	registered := make(map[string]RegisteredComponent, len(r.components))
	for id, component := range r.components {
		registered[id] = component
	}
	r.mu.RUnlock()

	bindings := BindingSet{}
	bound := make(map[string]ComponentDescriptor, len(plan.Requirements))
	requirements := make(map[string]ComponentRequirement, len(plan.Requirements))
	for _, requirement := range plan.Requirements {
		requirements[requirement.ID] = requirement
		component, exists := registered[requirement.ID]
		if !exists {
			if requirement.Required || !requirement.AllowResidual {
				return BindingSet{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "required component is not registered")
			}
			bindings.Residuals = append(bindings.Residuals, BindingResidual{ComponentID: requirement.ID, Reason: "component_missing"})
			continue
		}
		descriptor, err := component.Adapter.Describe(ctx)
		if err != nil {
			return BindingSet{}, err
		}
		if err := descriptor.Validate(); err != nil {
			return BindingSet{}, err
		}
		if descriptor.ID != requirement.ID || descriptor.Kind != requirement.Kind || descriptor.Version != requirement.Version || descriptor.ArtifactDigest != requirement.ArtifactDigest {
			return BindingSet{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "registered component does not match the resolved plan")
		}
		if err := validateCapabilities(descriptor, requirement, now); err != nil {
			if requirement.Required || !requirement.AllowResidual {
				return BindingSet{}, err
			}
			bindings.Residuals = append(bindings.Residuals, BindingResidual{ComponentID: requirement.ID, Reason: "capability_unavailable"})
			continue
		}
		bound[requirement.ID] = descriptor
	}
	// A component is usable only when every dependency in the resolved plan is
	// also bound. Repeat because pruning one optional component can invalidate
	// another optional dependency chain.
	for changed := true; changed; {
		changed = false
		for id := range bound {
			requirement := requirements[id]
			for _, dependency := range requirement.DependsOn {
				if _, exists := bound[dependency]; exists {
					continue
				}
				if requirement.Required || !requirement.AllowResidual {
					return BindingSet{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "bound component dependency is unavailable")
				}
				delete(bound, id)
				bindings.Residuals = append(bindings.Residuals, BindingResidual{ComponentID: id, Reason: "dependency_unavailable"})
				changed = true
				break
			}
		}
	}
	for _, descriptor := range bound {
		bindings.Descriptors = append(bindings.Descriptors, descriptor)
	}
	sort.Slice(bindings.Descriptors, func(i, j int) bool { return bindings.Descriptors[i].ID < bindings.Descriptors[j].ID })
	sort.Slice(bindings.Residuals, func(i, j int) bool { return bindings.Residuals[i].ComponentID < bindings.Residuals[j].ComponentID })
	return bindings, nil
}

func validateCapabilities(descriptor ComponentDescriptor, requirement ComponentRequirement, now time.Time) error {
	available := make(map[string]Capability, len(descriptor.Capabilities))
	for _, capability := range descriptor.Capabilities {
		available[capability.Name] = capability
	}
	for _, name := range requirement.Capabilities {
		capability, exists := available[name]
		if !exists || capability.State != CapabilityBound {
			return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "required bound capability is unavailable")
		}
		if !now.Before(capability.EvidenceExpiry) {
			return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonCapabilityExpired, "required capability evidence has expired")
		}
	}
	return nil
}

func hasDependencyCycle(graph map[string][]string) bool {
	const (
		unseen = iota
		visiting
		done
	)
	state := make(map[string]int, len(graph))
	var visit func(string) bool
	visit = func(node string) bool {
		if state[node] == visiting {
			return true
		}
		if state[node] == done {
			return false
		}
		state[node] = visiting
		for _, dependency := range graph[node] {
			if visit(dependency) {
				return true
			}
		}
		state[node] = done
		return false
	}
	for node := range graph {
		if visit(node) {
			return true
		}
	}
	return false
}

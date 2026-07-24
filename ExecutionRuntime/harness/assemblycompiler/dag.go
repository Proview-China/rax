package assemblycompiler

import (
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func dependencyOrder(input assemblycontract.AssemblyInputV1, index indexes) ([]string, error) {
	nodes := map[string]struct{}{}
	for id := range index.modules {
		nodes[id] = struct{}{}
	}
	for id := range index.slotContributions {
		nodes[id] = struct{}{}
	}
	for id := range index.phaseContributions {
		nodes[id] = struct{}{}
	}
	for id := range index.ports {
		nodes[id] = struct{}{}
	}
	for id := range index.factories {
		nodes[id] = struct{}{}
	}
	for id := range index.candidates {
		nodes[id] = struct{}{}
	}
	edges := map[string]map[string]struct{}{}
	indegree := map[string]int{}
	for node := range nodes {
		edges[node] = map[string]struct{}{}
		indegree[node] = 0
	}
	add := func(dependent, dependency string, required bool) error {
		if _, ok := nodes[dependent]; !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "dependency dependent ref is unknown")
		}
		if _, ok := nodes[dependency]; !ok {
			if required {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "required dependency ref is unknown")
			}
			return nil
		}
		if _, exists := edges[dependency][dependent]; !exists {
			edges[dependency][dependent] = struct{}{}
			indegree[dependent]++
		}
		return nil
	}
	for _, dep := range input.Dependencies {
		if err := add(dep.FromRef, dep.ToRef, dep.Required); err != nil {
			return nil, err
		}
	}
	for _, value := range input.SlotContributions {
		for _, dep := range value.Dependencies {
			if err := add(value.ContributionID, dep, true); err != nil {
				return nil, err
			}
		}
	}
	for _, value := range input.PhaseContributions {
		for _, dep := range value.Dependencies {
			if err := add(value.ContributionID, dep, true); err != nil {
				return nil, err
			}
		}
	}
	ready := []string{}
	for node, degree := range indegree {
		if degree == 0 {
			ready = append(ready, node)
		}
	}
	sort.Strings(ready)
	order := make([]string, 0, len(nodes))
	for len(ready) > 0 {
		node := ready[0]
		ready = ready[1:]
		order = append(order, node)
		children := make([]string, 0, len(edges[node]))
		for child := range edges[node] {
			children = append(children, child)
		}
		sort.Strings(children)
		for _, child := range children {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
				sort.Strings(ready)
			}
		}
	}
	if len(order) != len(nodes) {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonDependencyCycle, "assembly dependency graph contains a cycle")
	}
	return order, nil
}

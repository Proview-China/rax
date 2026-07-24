package composition

import (
	"context"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/registry"
)

type ProgressV1 func([]contract.ConstructionAttemptV1, []contract.ConstructedComponentV1) error
type CompositionV1 struct {
	components []contract.ConstructedComponentV1
	handles    []ports.ComponentHandleV1
}

func (c *CompositionV1) ComponentsV1() []contract.ConstructedComponentV1 {
	return cloneComponentsV1(c.components)
}

type ComposerV1 struct{ registry *registry.RegistryV1 }

func NewComposerV1(reg *registry.RegistryV1) (*ComposerV1, error) {
	if reg == nil {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "registry_missing", "factory registry is required")
	}
	return &ComposerV1{registry: reg}, nil
}

func (c *ComposerV1) ConstructV1(ctx context.Context, hostID, startID string, graph contract.ConstructionGraphV1, existingAttempts []contract.ConstructionAttemptV1, existing []contract.ConstructedComponentV1, progress ProgressV1) (*CompositionV1, contract.CleanupSummaryV1, error) {
	if err := contract.ValidateIdentifierV1("host id", hostID); err != nil {
		return nil, contract.CleanupSummaryV1{}, err
	}
	if err := contract.ValidateIdentifierV1("start id", startID); err != nil {
		return nil, contract.CleanupSummaryV1{}, err
	}
	if err := graph.Validate(); err != nil {
		return nil, contract.CleanupSummaryV1{}, err
	}
	order, err := graph.DependencyOrderV1()
	if err != nil {
		return nil, contract.CleanupSummaryV1{}, err
	}
	if len(existingAttempts) > len(order) || len(existing) > len(order) {
		return nil, contract.CleanupSummaryV1{}, contract.NewError(contract.ErrorConflict, "construction_history_drift", "construction history exceeds graph")
	}
	if len(existing) > len(existingAttempts) {
		return nil, contract.CleanupSummaryV1{}, contract.NewError(contract.ErrorConflict, "construction_history_drift", "constructed history lacks write-ahead attempts")
	}
	for i, attempt := range existingAttempts {
		node, _ := graph.NodeV1(order[i])
		if attempt.NodeID != node.NodeID || attempt.Factory != node.Factory {
			return nil, contract.CleanupSummaryV1{}, contract.NewError(contract.ErrorConflict, "construction_attempt_history_drift", "construction attempts are not an exact graph prefix")
		}
	}
	for i, item := range existing {
		node, _ := graph.NodeV1(order[i])
		if item.NodeID != node.NodeID || item.Factory != node.Factory || existingAttempts[i].NodeID != node.NodeID || existingAttempts[i].State != contract.AttemptConstructedV1 {
			return nil, contract.CleanupSummaryV1{}, contract.NewError(contract.ErrorConflict, "construction_history_drift", "construction history is not an exact graph prefix")
		}
	}
	attempts := append([]contract.ConstructionAttemptV1(nil), existingAttempts...)
	components := cloneComponentsV1(existing)
	byNode := map[string]contract.ConstructedComponentV1{}
	attemptByNode := map[string]int{}
	for i, a := range attempts {
		attemptByNode[a.NodeID] = i
	}
	for _, item := range components {
		byNode[item.NodeID] = item
	}
	result := &CompositionV1{}
	for _, nodeID := range order {
		node, _ := graph.NodeV1(nodeID)
		dependencies := make([]contract.ConstructedComponentV1, 0, len(node.Dependencies))
		for _, id := range node.Dependencies {
			item, ok := byNode[id]
			if !ok {
				return c.failV1(ctx, result, contract.CleanupSummaryV1{}, contract.NewError(contract.ErrorPrecondition, "dependency_not_constructed", "dependency is not constructed"), false)
			}
			dependencies = append(dependencies, item)
		}
		desired, makeErr := contract.NewConstructionAttemptV1(hostID, startID, graph.GraphRef, node, dependencies)
		if makeErr != nil {
			return nil, contract.CleanupSummaryV1{}, makeErr
		}
		idx, exists := attemptByNode[nodeID]
		if !exists {
			attempts = append(attempts, desired)
			idx = len(attempts) - 1
			attemptByNode[nodeID] = idx
			if progress != nil {
				if e := progress(cloneAttemptsV1(attempts), cloneComponentsV1(components)); e != nil {
					return c.failV1(ctx, result, contract.CleanupSummaryV1{}, e, true)
				}
			}
		} else if attempts[idx].AttemptID != desired.AttemptID || attempts[idx].RequestDigest != desired.RequestDigest {
			return nil, contract.CleanupSummaryV1{}, contract.NewError(contract.ErrorConflict, "construction_attempt_drift", "recovered attempt does not match exact construction request")
		}
		attempt := attempts[idx]
		factory, resolveErr := c.registry.ResolveV1(node.Factory)
		if resolveErr != nil {
			return c.failV1(ctx, result, contract.CleanupSummaryV1{}, resolveErr, false)
		}
		request := ports.ConstructRequestV1{HostID: hostID, StartID: startID, Node: node, Attempt: attempt, Dependencies: cloneComponentsV1(dependencies)}
		var handle ports.ComponentHandleV1
		var effectErr error
		switch attempt.State {
		case contract.AttemptConstructedV1:
			handle, effectErr = safeInspectV1(ctx, factory, request)
		case contract.AttemptUnknownV1:
			handle, effectErr = safeInspectV1(ctx, factory, request)
		case contract.AttemptPlannedV1:
			handle, effectErr = safeStartV1(ctx, factory, request)
			if effectErr != nil {
				var inspectErr error
				handle, inspectErr = safeInspectV1(context.WithoutCancel(ctx), factory, request)
				if inspectErr != nil {
					if knownNoEffectV1(effectErr) {
						return c.failV1(ctx, result, contract.CleanupSummaryV1{}, errors.Join(effectErr, inspectErr), false)
					}
					attempt.State = contract.AttemptUnknownV1
					attempt.Reason = "construction outcome unavailable"
					attempt.ComponentRef = nil
					attempt, _ = contract.SealConstructionAttemptV1(attempt)
					attempts[idx] = attempt
					var persistErr error
					if progress != nil {
						persistErr = progress(cloneAttemptsV1(attempts), cloneComponentsV1(components))
					}
					if persistErr != nil {
						persistErr = errors.Join(persistErr, contract.NewError(contract.ErrorUnknownOutcome, "unknown_progress_not_persisted", "construction unknown state was not proven durable"))
					}
					return c.failV1(ctx, result, contract.CleanupSummaryV1{}, errors.Join(effectErr, inspectErr, contract.NewError(contract.ErrorUnknownOutcome, "construction_outcome_unknown", "construction effect cannot be proven absent or recovered"), persistErr), true)
				}
				effectErr = nil
			}
		}
		if effectErr != nil {
			return c.failV1(ctx, result, contract.CleanupSummaryV1{}, effectErr, attempt.State == contract.AttemptUnknownV1)
		}
		if contract.IsTypedNilV1(handle) {
			return c.unknownV1(ctx, result, attempts, components, idx, progress, contract.NewError(contract.ErrorUnknownOutcome, "component_handle_missing", "factory outcome has no inspectable handle"))
		}
		ref, refErr := safeRefV1(handle)
		if refErr != nil {
			return c.unknownV1(ctx, result, attempts, components, idx, progress, refErr)
		}
		if err := ref.Validate(); err != nil {
			return c.unknownV1(ctx, result, attempts, components, idx, progress, err)
		}
		if attempt.State == contract.AttemptConstructedV1 && attempt.ComponentRef != nil && !contract.SameExactRefV1(*attempt.ComponentRef, ref) {
			return c.failV1(ctx, result, contract.CleanupSummaryV1{}, contract.NewError(contract.ErrorConflict, "component_ref_drift", "inspected component ref drifted"), true)
		}
		item := contract.ConstructedComponentV1{NodeID: node.NodeID, Factory: node.Factory, ComponentRef: ref}
		result.components = append(result.components, item)
		result.handles = append(result.handles, handle)
		if _, ok := byNode[nodeID]; !ok {
			attempt.State = contract.AttemptConstructedV1
			attempt.ComponentRef = refPtrV1(ref)
			attempt.Reason = ""
			attempt, _ = contract.SealConstructionAttemptV1(attempt)
			attempts[idx] = attempt
			components = append(components, item)
			byNode[nodeID] = item
			if progress != nil {
				if e := progress(cloneAttemptsV1(attempts), cloneComponentsV1(components)); e != nil {
					return c.failV1(ctx, result, contract.CleanupSummaryV1{}, e, true)
				}
			}
		} else if !contract.SameExactRefV1(byNode[nodeID].ComponentRef, ref) {
			return c.failV1(ctx, result, contract.CleanupSummaryV1{}, contract.NewError(contract.ErrorConflict, "component_ref_drift", "recovered component ref drifted"), true)
		}
	}
	return result, contract.CleanupSummaryV1{}, nil
}

func (c *ComposerV1) unknownV1(ctx context.Context, result *CompositionV1, attempts []contract.ConstructionAttemptV1, components []contract.ConstructedComponentV1, idx int, progress ProgressV1, cause error) (*CompositionV1, contract.CleanupSummaryV1, error) {
	a := attempts[idx]
	a.State = contract.AttemptUnknownV1
	a.ComponentRef = nil
	a.Reason = "construction outcome unavailable"
	a, _ = contract.SealConstructionAttemptV1(a)
	attempts[idx] = a
	var persistErr error
	if progress != nil {
		persistErr = progress(cloneAttemptsV1(attempts), cloneComponentsV1(components))
	}
	if persistErr != nil {
		persistErr = errors.Join(persistErr, contract.NewError(contract.ErrorUnknownOutcome, "unknown_progress_not_persisted", "construction unknown state was not proven durable"))
	}
	return c.failV1(ctx, result, contract.CleanupSummaryV1{}, errors.Join(cause, contract.NewError(contract.ErrorUnknownOutcome, "construction_outcome_unknown", "construction outcome is indeterminate"), persistErr), true)
}
func (c *ComposerV1) failV1(ctx context.Context, result *CompositionV1, _ contract.CleanupSummaryV1, cause error, unknown bool) (*CompositionV1, contract.CleanupSummaryV1, error) {
	summary, cleanupErr := result.CleanupV1(context.WithoutCancel(ctx))
	if unknown {
		summary.State = contract.CleanupIndeterminateV1
	}
	return nil, summary, errors.Join(cause, cleanupErr)
}

func (c *ComposerV1) ReattachV1(ctx context.Context, attempts []contract.ConstructionAttemptV1, components []contract.ConstructedComponentV1) (*CompositionV1, error) {
	byNode := map[string]contract.ConstructedComponentV1{}
	for _, item := range components {
		byNode[item.NodeID] = item
	}
	result := &CompositionV1{}
	for _, a := range attempts {
		factory, err := c.registry.ResolveV1(a.Factory)
		if err != nil {
			if a.State == contract.AttemptConstructedV1 {
				return nil, err
			}
			continue
		}
		request := ports.ConstructRequestV1{HostID: a.HostID, StartID: a.StartID, Node: a.Node, Attempt: a, Dependencies: cloneComponentsV1(a.Dependencies)}
		handle, err := safeInspectV1(ctx, factory, request)
		if err != nil {
			if a.State == contract.AttemptConstructedV1 {
				return nil, err
			}
			continue
		}
		if contract.IsTypedNilV1(handle) {
			if a.State == contract.AttemptConstructedV1 {
				return nil, contract.NewError(contract.ErrorUnavailable, "component_handle_missing", "factory inspection returned nil")
			}
			continue
		}
		if a.State != contract.AttemptConstructedV1 {
			ref, refErr := safeRefV1(handle)
			if refErr != nil || ref.Validate() != nil {
				continue
			}
			result.components = append(result.components, contract.ConstructedComponentV1{NodeID: a.NodeID, Factory: a.Factory, ComponentRef: ref})
			result.handles = append(result.handles, handle)
			continue
		}
		item, ok := byNode[a.NodeID]
		if !ok {
			return nil, contract.NewError(contract.ErrorConflict, "construction_attempt_component_missing", "successful attempt lacks component")
		}
		if err := validateHandleV1(handle, item.ComponentRef); err != nil {
			return nil, err
		}
		result.components = append(result.components, item)
		result.handles = append(result.handles, handle)
	}
	return result, nil
}

func (c *CompositionV1) CleanupV1(ctx context.Context) (contract.CleanupSummaryV1, error) {
	summary := contract.CleanupSummaryV1{State: contract.CleanupClosedV1}
	var joined error
	for i := len(c.handles) - 1; i >= 0; i-- {
		item, err := safeCleanupV1(ctx, c.handles[i])
		expected := c.components[i]
		if item.NodeID == "" {
			item.NodeID = expected.NodeID
		}
		if item.ComponentRef == (contract.ExactRefV1{}) {
			item.ComponentRef = expected.ComponentRef
		}
		if item.NodeID != expected.NodeID || !contract.SameExactRefV1(item.ComponentRef, expected.ComponentRef) {
			err = errors.Join(err, contract.NewError(contract.ErrorConflict, "cleanup_identity_drift", "cleanup result binds another component"))
			item.State = contract.CleanupIndeterminateV1
		}
		if item.State != contract.CleanupClosedV1 && item.State != contract.CleanupResidualV1 && item.State != contract.CleanupIndeterminateV1 {
			err = errors.Join(err, contract.NewError(contract.ErrorInvalidArgument, "cleanup_state_invalid", "cleanup state is unsupported"))
			item.State = contract.CleanupIndeterminateV1
		}
		if err != nil {
			joined = errors.Join(joined, err)
			item.State = contract.CleanupIndeterminateV1
		}
		if item.State == contract.CleanupIndeterminateV1 {
			summary.State = contract.CleanupIndeterminateV1
		} else if item.State == contract.CleanupResidualV1 && summary.State == contract.CleanupClosedV1 {
			summary.State = contract.CleanupResidualV1
		}
		summary.Items = append(summary.Items, item)
	}
	return summary, joined
}
func validateHandleV1(h ports.ComponentHandleV1, expected contract.ExactRefV1) error {
	if contract.IsTypedNilV1(h) {
		return contract.NewError(contract.ErrorUnavailable, "component_handle_missing", "factory returned nil handle")
	}
	actual, err := safeRefV1(h)
	if err != nil {
		return err
	}
	if err := actual.Validate(); err != nil {
		return err
	}
	if !contract.SameExactRefV1(actual, expected) {
		return contract.NewError(contract.ErrorConflict, "component_ref_drift", "inspected component ref drifted")
	}
	return nil
}
func safeStartV1(ctx context.Context, f ports.ComponentFactoryV1, r ports.ConstructRequestV1) (h ports.ComponentHandleV1, err error) {
	defer func() {
		if recover() != nil {
			h = nil
			err = contract.NewError(contract.ErrorUnknownOutcome, "factory_panic", "factory start-or-inspect panicked")
		}
	}()
	return f.StartOrInspectConstructionV1(ctx, r)
}
func safeInspectV1(ctx context.Context, f ports.ComponentFactoryV1, r ports.ConstructRequestV1) (h ports.ComponentHandleV1, err error) {
	defer func() {
		if recover() != nil {
			h = nil
			err = contract.NewError(contract.ErrorUnavailable, "factory_panic", "factory inspection panicked")
		}
	}()
	return f.InspectConstructionV1(ctx, r)
}
func safeRefV1(h ports.ComponentHandleV1) (r contract.ExactRefV1, err error) {
	defer func() {
		if recover() != nil {
			r = contract.ExactRefV1{}
			err = contract.NewError(contract.ErrorUnavailable, "component_handle_panic", "component ref panicked")
		}
	}()
	return h.RefV1(), nil
}
func safeCleanupV1(ctx context.Context, h ports.ComponentHandleV1) (item contract.CleanupItemV1, err error) {
	defer func() {
		if recover() != nil {
			item.State = contract.CleanupIndeterminateV1
			err = contract.NewError(contract.ErrorUnknownOutcome, "cleanup_panic", "component cleanup panicked")
		}
	}()
	return h.CleanupV1(ctx)
}
func knownNoEffectV1(err error) bool {
	return contract.HasCode(err, contract.ErrorInvalidArgument) || contract.HasCode(err, contract.ErrorPrecondition) || contract.HasCode(err, contract.ErrorNotFound)
}
func cloneComponentsV1(v []contract.ConstructedComponentV1) []contract.ConstructedComponentV1 {
	return append([]contract.ConstructedComponentV1(nil), v...)
}
func cloneAttemptsV1(v []contract.ConstructionAttemptV1) []contract.ConstructionAttemptV1 {
	return append([]contract.ConstructionAttemptV1(nil), v...)
}
func refPtrV1(v contract.ExactRefV1) *contract.ExactRefV1 { x := v; return &x }

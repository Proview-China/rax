package contract

import "sort"

const CleanupContractVersionV2 = "praxis.agent-host/cleanup/v2"

const (
	CleanupBarrierHarnessCloseV2            = "harness_close"
	CleanupBarrierSandboxFenceV2            = "sandbox_fence"
	CleanupBarrierSandboxReleaseV2          = "sandbox_release"
	CleanupBarrierRuntimeCleanupAggregateV2 = "runtime_cleanup_aggregate"
)

const (
	CleanupBarrierOwnerHarnessV2 = "praxis.harness"
	CleanupBarrierOwnerSandboxV2 = "praxis.sandbox"
	CleanupBarrierOwnerRuntimeV2 = "praxis.runtime"
)

type CleanupResourceClassV2 string

const (
	CleanupLiveExecutionV2      CleanupResourceClassV2 = "live_execution"
	CleanupFencedSandboxLeaseV2 CleanupResourceClassV2 = "fenced_sandbox_lease"
	CleanupSandboxIndependentV2 CleanupResourceClassV2 = "sandbox_independent"
	CleanupHostControlHandleV2  CleanupResourceClassV2 = "host_control_handle"
)

type CleanupNodeKindV2 string

const (
	CleanupOwnerNodeV2   CleanupNodeKindV2 = "owner_cleanup"
	CleanupBarrierNodeV2 CleanupNodeKindV2 = "barrier"
)

type CleanupNodeV2 struct {
	NodeID              string                 `json:"node_id"`
	Kind                CleanupNodeKindV2      `json:"kind"`
	OwnerComponentID    string                 `json:"owner_component_id"`
	CleanupContractRef  ExactRefV1             `json:"cleanup_contract_ref"`
	ResourceClass       CleanupResourceClassV2 `json:"resource_class"`
	RequiredBarrierIDs  []string               `json:"required_barrier_ids"`
	InspectPortBinding  ExactRefV1             `json:"inspect_port_binding"`
	RequestSchemaDigest DigestV1               `json:"request_schema_digest"`
	ResultSchemaDigest  DigestV1               `json:"result_schema_digest"`
	Digest              DigestV1               `json:"digest"`
}

func (n CleanupNodeV2) canonicalV2() CleanupNodeV2 {
	clone := n
	clone.RequiredBarrierIDs = append([]string(nil), n.RequiredBarrierIDs...)
	if clone.RequiredBarrierIDs == nil {
		clone.RequiredBarrierIDs = []string{}
	}
	sort.Strings(clone.RequiredBarrierIDs)
	return clone
}

func (n CleanupNodeV2) digestV2() (DigestV1, error) {
	clone := n.canonicalV2()
	clone.Digest = ""
	return DigestJSONV1(struct {
		Domain string        `json:"domain"`
		Type   string        `json:"type"`
		Body   CleanupNodeV2 `json:"body"`
	}{Domain: "praxis.agent-host.cleanup-v2", Type: "CleanupNodeV2", Body: clone})
}

func SealCleanupNodeV2(n CleanupNodeV2) (CleanupNodeV2, error) {
	n = n.canonicalV2()
	provided := n.Digest
	digest, err := n.digestV2()
	if err != nil {
		return CleanupNodeV2{}, err
	}
	if provided != "" && provided != digest {
		return CleanupNodeV2{}, NewError(ErrorConflict, "cleanup_node_digest_drift", "cleanup node supplied a wrong non-zero digest")
	}
	n.Digest = digest
	if err := n.Validate(); err != nil {
		return CleanupNodeV2{}, err
	}
	return n, nil
}

func (n CleanupNodeV2) Validate() error {
	if err := ValidateIdentifierV1("cleanup node id", n.NodeID); err != nil {
		return err
	}
	if n.Kind != CleanupOwnerNodeV2 && n.Kind != CleanupBarrierNodeV2 {
		return NewError(ErrorInvalidArgument, "cleanup_node_kind_invalid", "cleanup node kind is unsupported")
	}
	if err := ValidateIdentifierV1("cleanup owner component id", n.OwnerComponentID); err != nil {
		return err
	}
	if err := n.CleanupContractRef.Validate(); err != nil {
		return err
	}
	if err := n.InspectPortBinding.Validate(); err != nil {
		return err
	}
	if err := n.RequestSchemaDigest.Validate(); err != nil {
		return err
	}
	if err := n.ResultSchemaDigest.Validate(); err != nil {
		return err
	}
	if !validCleanupResourceClassV2(n.ResourceClass) {
		return NewError(ErrorInvalidArgument, "cleanup_resource_class_invalid", "cleanup resource class is unsupported")
	}
	seen := map[string]struct{}{}
	for i, dependency := range n.RequiredBarrierIDs {
		if err := ValidateIdentifierV1("cleanup dependency id", dependency); err != nil {
			return err
		}
		if dependency == n.NodeID {
			return NewError(ErrorConflict, "cleanup_self_dependency", "cleanup node cannot depend on itself")
		}
		if _, ok := seen[dependency]; ok {
			return NewError(ErrorConflict, "cleanup_dependency_duplicate", "cleanup dependency is duplicated")
		}
		if i > 0 && n.RequiredBarrierIDs[i-1] > dependency {
			return NewError(ErrorInvalidArgument, "cleanup_dependencies_not_canonical", "cleanup dependencies must be sorted")
		}
		seen[dependency] = struct{}{}
	}
	expected, err := n.digestV2()
	if err != nil {
		return err
	}
	if expected != n.Digest {
		return NewError(ErrorPrecondition, "cleanup_node_digest_drift", "cleanup node digest drifted")
	}
	return nil
}

type CleanupPlanV2 struct {
	ContractVersion string          `json:"contract_version"`
	PlanID          string          `json:"plan_id"`
	Revision        uint64          `json:"revision"`
	HostID          string          `json:"host_id"`
	StartID         string          `json:"start_id"`
	Nodes           []CleanupNodeV2 `json:"nodes"`
	Digest          DigestV1        `json:"digest"`
}

func (p CleanupPlanV2) canonicalV2() CleanupPlanV2 {
	clone := p
	clone.Nodes = append([]CleanupNodeV2(nil), p.Nodes...)
	if clone.Nodes == nil {
		clone.Nodes = []CleanupNodeV2{}
	}
	sort.Slice(clone.Nodes, func(i, j int) bool { return clone.Nodes[i].NodeID < clone.Nodes[j].NodeID })
	return clone
}

func (p CleanupPlanV2) digestV2() (DigestV1, error) {
	clone := p.canonicalV2()
	clone.Digest = ""
	return DigestJSONV1(struct {
		Domain string        `json:"domain"`
		Type   string        `json:"type"`
		Body   CleanupPlanV2 `json:"body"`
	}{Domain: "praxis.agent-host.cleanup-v2", Type: "CleanupPlanV2", Body: clone})
}

func SealCleanupPlanV2(p CleanupPlanV2) (CleanupPlanV2, error) {
	p = p.canonicalV2()
	provided := p.Digest
	digest, err := p.digestV2()
	if err != nil {
		return CleanupPlanV2{}, err
	}
	if provided != "" && provided != digest {
		return CleanupPlanV2{}, NewError(ErrorConflict, "cleanup_plan_digest_drift", "cleanup plan supplied a wrong non-zero digest")
	}
	p.Digest = digest
	if err := p.Validate(); err != nil {
		return CleanupPlanV2{}, err
	}
	return p, nil
}

func (p CleanupPlanV2) Validate() error {
	if p.ContractVersion != CleanupContractVersionV2 {
		return NewError(ErrorInvalidArgument, "contract_version_mismatch", "cleanup plan contract version is unsupported")
	}
	for field, value := range map[string]string{"cleanup plan id": p.PlanID, "host id": p.HostID, "start id": p.StartID} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	if p.Revision == 0 || len(p.Nodes) < 4 {
		return NewError(ErrorInvalidArgument, "cleanup_plan_incomplete", "cleanup plan revision and fixed barriers are required")
	}
	nodes := make(map[string]CleanupNodeV2, len(p.Nodes))
	for i, node := range p.Nodes {
		if err := node.Validate(); err != nil {
			return err
		}
		if i > 0 && p.Nodes[i-1].NodeID > node.NodeID {
			return NewError(ErrorInvalidArgument, "cleanup_nodes_not_canonical", "cleanup nodes must be sorted")
		}
		if _, exists := nodes[node.NodeID]; exists {
			return NewError(ErrorConflict, "cleanup_node_duplicate", "cleanup plan duplicates a node")
		}
		nodes[node.NodeID] = node
	}
	for _, barrier := range []string{CleanupBarrierHarnessCloseV2, CleanupBarrierSandboxFenceV2, CleanupBarrierSandboxReleaseV2, CleanupBarrierRuntimeCleanupAggregateV2} {
		node, ok := nodes[barrier]
		if !ok || node.Kind != CleanupBarrierNodeV2 {
			return NewError(ErrorPrecondition, "cleanup_barrier_missing", "cleanup plan lacks a fixed typed barrier")
		}
	}
	barrierRequirements := map[string]struct {
		owner string
		class CleanupResourceClassV2
	}{
		CleanupBarrierHarnessCloseV2:            {CleanupBarrierOwnerHarnessV2, CleanupLiveExecutionV2},
		CleanupBarrierSandboxFenceV2:            {CleanupBarrierOwnerSandboxV2, CleanupFencedSandboxLeaseV2},
		CleanupBarrierSandboxReleaseV2:          {CleanupBarrierOwnerSandboxV2, CleanupFencedSandboxLeaseV2},
		CleanupBarrierRuntimeCleanupAggregateV2: {CleanupBarrierOwnerRuntimeV2, CleanupHostControlHandleV2},
	}
	for id, expected := range barrierRequirements {
		node := nodes[id]
		if node.OwnerComponentID != expected.owner || node.ResourceClass != expected.class {
			return NewError(ErrorConflict, "cleanup_barrier_identity_drift", "fixed cleanup barrier Owner or resource class drifted")
		}
	}
	for _, node := range p.Nodes {
		for _, dependency := range node.RequiredBarrierIDs {
			if _, ok := nodes[dependency]; !ok {
				return NewError(ErrorPrecondition, "cleanup_dependency_missing", "cleanup dependency is absent from the plan")
			}
		}
	}
	if hasCleanupCycleV2(nodes) {
		return NewError(ErrorConflict, "cleanup_dependency_cycle", "cleanup plan contains a dependency cycle")
	}
	reaches := func(from, to string) bool { return cleanupReachableV2(nodes, from, to, map[string]bool{}) }
	if !reaches(CleanupBarrierHarnessCloseV2, CleanupBarrierSandboxFenceV2) || !reaches(CleanupBarrierSandboxFenceV2, CleanupBarrierSandboxReleaseV2) || !reaches(CleanupBarrierSandboxReleaseV2, CleanupBarrierRuntimeCleanupAggregateV2) {
		return NewError(ErrorPrecondition, "cleanup_barrier_order_invalid", "fixed cleanup barriers are not ordered")
	}
	for _, node := range p.Nodes {
		switch node.ResourceClass {
		case CleanupLiveExecutionV2:
			if node.NodeID != CleanupBarrierHarnessCloseV2 && !reaches(node.NodeID, CleanupBarrierHarnessCloseV2) {
				return NewError(ErrorPrecondition, "live_execution_cleanup_misordered", "live-execution cleanup must complete before Harness close")
			}
		case CleanupFencedSandboxLeaseV2:
			if node.NodeID != CleanupBarrierSandboxFenceV2 && node.NodeID != CleanupBarrierSandboxReleaseV2 && (!reaches(CleanupBarrierSandboxFenceV2, node.NodeID) || !reaches(node.NodeID, CleanupBarrierSandboxReleaseV2)) {
				return NewError(ErrorPrecondition, "lease_cleanup_misordered", "lease-dependent cleanup must be between Sandbox fence and release")
			}
		case CleanupHostControlHandleV2:
			if node.NodeID != CleanupBarrierRuntimeCleanupAggregateV2 && !reaches(CleanupBarrierRuntimeCleanupAggregateV2, node.NodeID) {
				return NewError(ErrorPrecondition, "host_handle_cleanup_misordered", "host control handle cleanup must follow Runtime aggregation")
			}
		}
	}
	expected, err := p.digestV2()
	if err != nil {
		return err
	}
	if expected != p.Digest {
		return NewError(ErrorPrecondition, "cleanup_plan_digest_drift", "cleanup plan digest drifted")
	}
	return nil
}

func (p CleanupPlanV2) RefV2() (ExactRefV1, error) {
	if err := p.Validate(); err != nil {
		return ExactRefV1{}, err
	}
	return ExactRefV1{Kind: "praxis.agent-host/cleanup-plan-v2", ID: p.PlanID, Revision: p.Revision, Digest: p.Digest}, nil
}

func hasCleanupCycleV2(nodes map[string]CleanupNodeV2) bool {
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, dependency := range nodes[id].RequiredBarrierIDs {
			if visit(dependency) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	for id := range nodes {
		if visit(id) {
			return true
		}
	}
	return false
}

// cleanupReachableV2 reports whether from must complete before to.
func cleanupReachableV2(nodes map[string]CleanupNodeV2, from, to string, seen map[string]bool) bool {
	if from == to {
		return true
	}
	if seen[to] {
		return false
	}
	seen[to] = true
	for _, dependency := range nodes[to].RequiredBarrierIDs {
		if dependency == from || cleanupReachableV2(nodes, from, dependency, seen) {
			return true
		}
	}
	return false
}

func validCleanupResourceClassV2(value CleanupResourceClassV2) bool {
	switch value {
	case CleanupLiveExecutionV2, CleanupFencedSandboxLeaseV2, CleanupSandboxIndependentV2, CleanupHostControlHandleV2:
		return true
	default:
		return false
	}
}

type CleanupAttemptStateV2 string

const (
	CleanupIntentRecordedV2         CleanupAttemptStateV2 = "intent_recorded"
	CleanupResultRecordedV2         CleanupAttemptStateV2 = "result_recorded"
	CleanupOutcomeUnknownV2         CleanupAttemptStateV2 = "outcome_unknown"
	CleanupReconciliationRequiredV2 CleanupAttemptStateV2 = "reconciliation_required"
)

type CleanupDispositionV2 string

const (
	CleanupDispositionSettledV2  CleanupDispositionV2 = "settled"
	CleanupDispositionResidualV2 CleanupDispositionV2 = "residual"
)

type CleanupAttemptV2 struct {
	ContractVersion     string                `json:"contract_version"`
	AttemptID           string                `json:"attempt_id"`
	Revision            uint64                `json:"revision"`
	HostID              string                `json:"host_id"`
	StartID             string                `json:"start_id"`
	PlanRef             ExactRefV1            `json:"plan_ref"`
	NodeID              string                `json:"node_id"`
	RequestDigest       DigestV1              `json:"request_digest"`
	PredecessorRevision uint64                `json:"predecessor_revision"`
	BarrierCurrentRefs  []ExactRefV1          `json:"barrier_current_refs"`
	State               CleanupAttemptStateV2 `json:"state"`
	ResultRef           *ExactRefV1           `json:"result_ref,omitempty"`
	ResultDisposition   CleanupDispositionV2  `json:"result_disposition,omitempty"`
	CreatedUnixNano     int64                 `json:"created_unix_nano"`
	UpdatedUnixNano     int64                 `json:"updated_unix_nano"`
	Digest              DigestV1              `json:"digest"`
}

func (a CleanupAttemptV2) digestV2() (DigestV1, error) {
	clone := a
	clone.BarrierCurrentRefs = append([]ExactRefV1(nil), a.BarrierCurrentRefs...)
	if clone.BarrierCurrentRefs == nil {
		clone.BarrierCurrentRefs = []ExactRefV1{}
	}
	sort.Slice(clone.BarrierCurrentRefs, func(i, j int) bool {
		if clone.BarrierCurrentRefs[i].Kind != clone.BarrierCurrentRefs[j].Kind {
			return clone.BarrierCurrentRefs[i].Kind < clone.BarrierCurrentRefs[j].Kind
		}
		return clone.BarrierCurrentRefs[i].ID < clone.BarrierCurrentRefs[j].ID
	})
	clone.Digest = ""
	return DigestJSONV1(struct {
		Domain string           `json:"domain"`
		Type   string           `json:"type"`
		Body   CleanupAttemptV2 `json:"body"`
	}{Domain: "praxis.agent-host.cleanup-v2", Type: "CleanupAttemptV2", Body: clone})
}

func SealCleanupAttemptV2(a CleanupAttemptV2) (CleanupAttemptV2, error) {
	a.BarrierCurrentRefs = append([]ExactRefV1(nil), a.BarrierCurrentRefs...)
	if a.BarrierCurrentRefs == nil {
		a.BarrierCurrentRefs = []ExactRefV1{}
	}
	sort.Slice(a.BarrierCurrentRefs, func(i, j int) bool {
		if a.BarrierCurrentRefs[i].Kind != a.BarrierCurrentRefs[j].Kind {
			return a.BarrierCurrentRefs[i].Kind < a.BarrierCurrentRefs[j].Kind
		}
		return a.BarrierCurrentRefs[i].ID < a.BarrierCurrentRefs[j].ID
	})
	provided := a.Digest
	digest, err := a.digestV2()
	if err != nil {
		return CleanupAttemptV2{}, err
	}
	if provided != "" && provided != digest {
		return CleanupAttemptV2{}, NewError(ErrorConflict, "cleanup_attempt_digest_drift", "cleanup attempt supplied a wrong non-zero digest")
	}
	a.Digest = digest
	if err := a.Validate(); err != nil {
		return CleanupAttemptV2{}, err
	}
	return a, nil
}

func (a CleanupAttemptV2) Validate() error {
	if a.ContractVersion != CleanupContractVersionV2 {
		return NewError(ErrorInvalidArgument, "contract_version_mismatch", "cleanup attempt contract version is unsupported")
	}
	for field, value := range map[string]string{"attempt id": a.AttemptID, "host id": a.HostID, "start id": a.StartID, "node id": a.NodeID} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	if a.Revision == 0 || a.CreatedUnixNano <= 0 || a.UpdatedUnixNano < a.CreatedUnixNano {
		return NewError(ErrorInvalidArgument, "cleanup_attempt_watermark_invalid", "cleanup attempt revision or time watermark is invalid")
	}
	if err := a.PlanRef.Validate(); err != nil {
		return err
	}
	if err := a.RequestDigest.Validate(); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for i, ref := range a.BarrierCurrentRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
		key := ref.Kind + "\x00" + ref.ID
		if _, ok := seen[key]; ok {
			return NewError(ErrorConflict, "cleanup_barrier_ref_duplicate", "cleanup attempt duplicates a barrier current ref")
		}
		if i > 0 {
			prev := a.BarrierCurrentRefs[i-1]
			if prev.Kind > ref.Kind || (prev.Kind == ref.Kind && prev.ID > ref.ID) {
				return NewError(ErrorInvalidArgument, "cleanup_barrier_refs_not_canonical", "cleanup barrier refs must be sorted")
			}
		}
		seen[key] = struct{}{}
	}
	switch a.State {
	case CleanupIntentRecordedV2, CleanupOutcomeUnknownV2, CleanupReconciliationRequiredV2:
		if a.ResultRef != nil || a.ResultDisposition != "" {
			return NewError(ErrorPrecondition, "cleanup_result_too_early", "non-terminal cleanup attempt cannot carry a result")
		}
	case CleanupResultRecordedV2:
		if a.ResultRef == nil {
			return NewError(ErrorPrecondition, "cleanup_result_missing", "settled cleanup attempt requires a result")
		}
		if err := a.ResultRef.Validate(); err != nil {
			return err
		}
		if a.ResultDisposition != CleanupDispositionSettledV2 && a.ResultDisposition != CleanupDispositionResidualV2 {
			return NewError(ErrorInvalidArgument, "cleanup_result_disposition_invalid", "cleanup result requires a closed settled or residual disposition")
		}
	default:
		return NewError(ErrorInvalidArgument, "cleanup_attempt_state_invalid", "cleanup attempt state is unsupported")
	}
	expected, err := a.digestV2()
	if err != nil {
		return err
	}
	if expected != a.Digest {
		return NewError(ErrorPrecondition, "cleanup_attempt_digest_drift", "cleanup attempt digest drifted")
	}
	return nil
}

func ValidateCleanupAttemptSuccessorV2(current, next CleanupAttemptV2) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.AttemptID != next.AttemptID || current.HostID != next.HostID || current.StartID != next.StartID || current.PlanRef != next.PlanRef || current.NodeID != next.NodeID || current.RequestDigest != next.RequestDigest || current.PredecessorRevision != next.PredecessorRevision || current.CreatedUnixNano != next.CreatedUnixNano {
		return NewError(ErrorConflict, "cleanup_attempt_identity_drift", "cleanup attempt immutable identity drifted")
	}
	if len(current.BarrierCurrentRefs) != len(next.BarrierCurrentRefs) {
		return NewError(ErrorConflict, "cleanup_attempt_barrier_drift", "cleanup attempt barrier current refs drifted")
	}
	for i := range current.BarrierCurrentRefs {
		if current.BarrierCurrentRefs[i] != next.BarrierCurrentRefs[i] {
			return NewError(ErrorConflict, "cleanup_attempt_barrier_drift", "cleanup attempt barrier current refs drifted")
		}
	}
	if next.Revision != current.Revision+1 || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return NewError(ErrorConflict, "cleanup_attempt_revision_drift", "cleanup attempt successor must advance exactly one revision")
	}
	allowed := current.State == CleanupIntentRecordedV2 && (next.State == CleanupResultRecordedV2 || next.State == CleanupOutcomeUnknownV2) || current.State == CleanupOutcomeUnknownV2 && (next.State == CleanupResultRecordedV2 || next.State == CleanupReconciliationRequiredV2) || current.State == CleanupReconciliationRequiredV2 && (next.State == CleanupResultRecordedV2 || next.State == CleanupReconciliationRequiredV2)
	if !allowed {
		return NewError(ErrorPrecondition, "cleanup_attempt_transition_invalid", "cleanup attempt transition is not allowed")
	}
	return nil
}

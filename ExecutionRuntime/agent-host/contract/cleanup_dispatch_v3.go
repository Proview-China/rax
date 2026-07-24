package contract

import (
	"sort"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const CleanupNodeDispatchContractVersionV3 = "praxis.agent-host/cleanup-node-dispatch/v3"

type CleanupNodeTargetRoleV3 string

const (
	CleanupTargetConstructedV3    CleanupNodeTargetRoleV3 = "constructed_target"
	CleanupTargetAttemptInspectV3 CleanupNodeTargetRoleV3 = "attempt_inspect"
	CleanupTargetNotConstructedV3 CleanupNodeTargetRoleV3 = "not_constructed"
)

type CleanupNodeTargetClassV3 string

const (
	CleanupTargetControlV3          CleanupNodeTargetClassV3 = "control_cleanup"
	CleanupTargetHarnessCloseV3     CleanupNodeTargetClassV3 = "harness_close"
	CleanupTargetSandboxFenceV3     CleanupNodeTargetClassV3 = "sandbox_fence"
	CleanupTargetSandboxReleaseV3   CleanupNodeTargetClassV3 = "sandbox_release"
	CleanupTargetRuntimeAggregateV3 CleanupNodeTargetClassV3 = "runtime_cleanup_aggregate"
)

type CleanupNodeTargetV3 struct {
	Role               CleanupNodeTargetRoleV3            `json:"role"`
	Class              CleanupNodeTargetClassV3           `json:"class"`
	ControlInstance    *ExactRefV1                        `json:"control_instance,omitempty"`
	ResourceHandles    []runtimeports.ResourceHandleRefV1 `json:"resource_handles"`
	ExecutionOpen      *ExactRefV1                        `json:"execution_open,omitempty"`
	Endpoint           *ExactRefV1                        `json:"endpoint,omitempty"`
	SandboxReservation *ExactRefV1                        `json:"sandbox_reservation,omitempty"`
	SandboxLease       *ExactRefV1                        `json:"sandbox_lease,omitempty"`
	Activation         *ExactRefV1                        `json:"activation,omitempty"`
	ActivationAttempt  *ExactRefV1                        `json:"activation_attempt,omitempty"`
	IdentityLease      *ExactRefV1                        `json:"identity_lease,omitempty"`
	Commit             *ExactRefV1                        `json:"commit,omitempty"`
	Run                *ExactRefV1                        `json:"run,omitempty"`
	AttemptRef         *ExactRefV1                        `json:"attempt_ref,omitempty"`
	IntentRef          *ExactRefV1                        `json:"intent_ref,omitempty"`
	OperationRef       *ExactRefV1                        `json:"operation_ref,omitempty"`
	JournalProof       *ExactRefV1                        `json:"journal_proof,omitempty"`
	Digest             DigestV1                           `json:"digest"`
}

func (t CleanupNodeTargetV3) canonicalV3() CleanupNodeTargetV3 {
	t.ResourceHandles = append([]runtimeports.ResourceHandleRefV1{}, t.ResourceHandles...)
	sort.Slice(t.ResourceHandles, func(i, j int) bool {
		return ownerHandleKeyV2(t.ResourceHandles[i]) < ownerHandleKeyV2(t.ResourceHandles[j])
	})
	return t
}

func (t CleanupNodeTargetV3) digestV3() (DigestV1, error) {
	t = t.canonicalV3()
	t.Digest = ""
	return DigestJSONV1(struct {
		Domain string              `json:"domain"`
		Type   string              `json:"type"`
		Body   CleanupNodeTargetV3 `json:"body"`
	}{"praxis.agent-host.cleanup-node-target-v3", "CleanupNodeTargetV3", t})
}

func SealCleanupNodeTargetV3(t CleanupNodeTargetV3) (CleanupNodeTargetV3, error) {
	t = t.canonicalV3()
	provided := t.Digest
	t.Digest = ""
	digest, err := t.digestV3()
	if err != nil {
		return CleanupNodeTargetV3{}, err
	}
	if provided != "" && provided != digest {
		return CleanupNodeTargetV3{}, NewError(ErrorConflict, "cleanup_target_digest_drift", "cleanup target supplied a wrong digest")
	}
	t.Digest = digest
	return t, t.Validate()
}

func (t CleanupNodeTargetV3) Validate() error {
	validRef := func(ref *ExactRefV1) error {
		if ref == nil {
			return nil
		}
		return ref.Validate()
	}
	for _, ref := range []*ExactRefV1{t.ControlInstance, t.ExecutionOpen, t.Endpoint, t.SandboxReservation, t.SandboxLease, t.Activation, t.ActivationAttempt, t.IdentityLease, t.Commit, t.Run, t.AttemptRef, t.IntentRef, t.OperationRef, t.JournalProof} {
		if err := validRef(ref); err != nil {
			return err
		}
	}
	for i, h := range t.ResourceHandles {
		if err := h.Validate(); err != nil {
			return err
		}
		if i > 0 && ownerHandleKeyV2(t.ResourceHandles[i-1]) >= ownerHandleKeyV2(h) {
			return NewError(ErrorConflict, "cleanup_target_resource_duplicate", "cleanup target resources must be sorted and unique")
		}
	}
	constructedFields := t.ControlInstance != nil || len(t.ResourceHandles) > 0 || t.ExecutionOpen != nil || t.Endpoint != nil || t.SandboxReservation != nil || t.SandboxLease != nil || t.Activation != nil || t.ActivationAttempt != nil || t.IdentityLease != nil || t.Commit != nil || t.Run != nil
	attemptFields := t.AttemptRef != nil || t.IntentRef != nil || t.OperationRef != nil
	switch t.Role {
	case CleanupTargetConstructedV3:
		if !constructedFields || attemptFields || t.JournalProof != nil {
			return NewError(ErrorPrecondition, "cleanup_constructed_target_invalid", "constructed target fields are incomplete or mixed")
		}
		switch t.Class {
		case CleanupTargetControlV3:
			if t.ControlInstance == nil || len(t.ResourceHandles) == 0 || t.ExecutionOpen != nil || t.Endpoint != nil || t.SandboxReservation != nil || t.SandboxLease != nil || t.Activation != nil || t.ActivationAttempt != nil || t.IdentityLease != nil || t.Commit != nil || t.Run != nil {
				return NewError(ErrorPrecondition, "cleanup_control_target_invalid", "control target fields drifted")
			}
		case CleanupTargetHarnessCloseV3:
			if t.ExecutionOpen == nil || t.Endpoint == nil || t.ControlInstance != nil || len(t.ResourceHandles) > 0 || t.SandboxReservation != nil || t.SandboxLease != nil || t.Activation != nil || t.ActivationAttempt != nil || t.IdentityLease != nil || t.Commit != nil || t.Run != nil {
				return NewError(ErrorPrecondition, "cleanup_harness_target_invalid", "Harness target fields drifted")
			}
		case CleanupTargetSandboxFenceV3, CleanupTargetSandboxReleaseV3:
			if t.SandboxReservation == nil || t.SandboxLease == nil || t.ControlInstance != nil || len(t.ResourceHandles) > 0 || t.ExecutionOpen != nil || t.Endpoint != nil || t.ActivationAttempt != nil || t.IdentityLease != nil || t.Commit != nil || t.Run != nil {
				return NewError(ErrorPrecondition, "cleanup_sandbox_target_invalid", "Sandbox target fields drifted")
			}
		case CleanupTargetRuntimeAggregateV3:
			if t.ControlInstance != nil || len(t.ResourceHandles) > 0 || t.ExecutionOpen != nil || t.Endpoint != nil || t.SandboxReservation != nil || t.SandboxLease != nil || t.ActivationAttempt == nil || t.IdentityLease == nil || t.Commit == nil {
				return NewError(ErrorPrecondition, "cleanup_runtime_target_invalid", "Runtime aggregate target fields drifted")
			}
		default:
			return NewError(ErrorInvalidArgument, "cleanup_target_class_invalid", "cleanup target class is unsupported")
		}
	case CleanupTargetAttemptInspectV3:
		if constructedFields || t.JournalProof != nil || t.AttemptRef == nil || t.IntentRef == nil || t.OperationRef == nil {
			return NewError(ErrorPrecondition, "cleanup_attempt_inspect_target_invalid", "attempt-inspect target must carry only the complete stable operation coordinates")
		}
		if t.Class != CleanupTargetControlV3 && t.Class != CleanupTargetHarnessCloseV3 && t.Class != CleanupTargetSandboxFenceV3 && t.Class != CleanupTargetSandboxReleaseV3 && t.Class != CleanupTargetRuntimeAggregateV3 {
			return NewError(ErrorInvalidArgument, "cleanup_target_class_invalid", "cleanup target class is unsupported")
		}
	case CleanupTargetNotConstructedV3:
		if constructedFields || attemptFields || t.JournalProof == nil {
			return NewError(ErrorPrecondition, "cleanup_not_constructed_target_invalid", "not-constructed target must carry only Journal proof")
		}
	default:
		return NewError(ErrorInvalidArgument, "cleanup_target_role_invalid", "cleanup target role is unsupported")
	}
	expected, err := t.digestV3()
	if err != nil {
		return err
	}
	if expected != t.Digest {
		return NewError(ErrorConflict, "cleanup_target_digest_drift", "cleanup target digest drifted")
	}
	return nil
}

type CleanupNodeDispatchEnvelopeV3 struct {
	ContractVersion      string                         `json:"contract_version"`
	ClosureRef           HostCleanupClosureRefV2        `json:"closure_ref"`
	PlanRef              ExactRefV1                     `json:"plan_ref"`
	NodeRef              ExactRefV1                     `json:"node_ref"`
	Target               CleanupNodeTargetV3            `json:"target"`
	AuthorizationCurrent runtimeports.OwnerCurrentRefV1 `json:"authorization_current"`
	FenceCurrent         runtimeports.OwnerCurrentRefV1 `json:"fence_current"`
	PredecessorBarriers  []ExactRefV1                   `json:"predecessor_barriers"`
	CheckedUnixNano      int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                          `json:"expires_unix_nano"`
	TargetDigest         DigestV1                       `json:"target_digest"`
	Digest               DigestV1                       `json:"digest"`
}

func (e CleanupNodeDispatchEnvelopeV3) canonicalV3() CleanupNodeDispatchEnvelopeV3 {
	e.PredecessorBarriers = append([]ExactRefV1{}, e.PredecessorBarriers...)
	sort.Slice(e.PredecessorBarriers, func(i, j int) bool {
		if e.PredecessorBarriers[i].Kind != e.PredecessorBarriers[j].Kind {
			return e.PredecessorBarriers[i].Kind < e.PredecessorBarriers[j].Kind
		}
		return e.PredecessorBarriers[i].ID < e.PredecessorBarriers[j].ID
	})
	return e
}
func (e CleanupNodeDispatchEnvelopeV3) digestV3() (DigestV1, error) {
	e = e.canonicalV3()
	e.Digest = ""
	return DigestJSONV1(struct {
		Domain string                        `json:"domain"`
		Type   string                        `json:"type"`
		Body   CleanupNodeDispatchEnvelopeV3 `json:"body"`
	}{"praxis.agent-host.cleanup-node-dispatch-v3", "CleanupNodeDispatchEnvelopeV3", e})
}
func SealCleanupNodeDispatchEnvelopeV3(e CleanupNodeDispatchEnvelopeV3) (CleanupNodeDispatchEnvelopeV3, error) {
	if e.ContractVersion != "" && e.ContractVersion != CleanupNodeDispatchContractVersionV3 { return CleanupNodeDispatchEnvelopeV3{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "cleanup dispatch contract version drifted") }
	e.ContractVersion = CleanupNodeDispatchContractVersionV3
	e = e.canonicalV3()
	e.TargetDigest = e.Target.Digest
	provided := e.Digest
	e.Digest = ""
	digest, err := e.digestV3()
	if err != nil {
		return CleanupNodeDispatchEnvelopeV3{}, err
	}
	if provided != "" && provided != digest {
		return CleanupNodeDispatchEnvelopeV3{}, NewError(ErrorConflict, "cleanup_dispatch_digest_drift", "cleanup dispatch envelope supplied a wrong digest")
	}
	e.Digest = digest
	return e, e.Validate()
}
func (e CleanupNodeDispatchEnvelopeV3) Validate() error {
	if e.ContractVersion != CleanupNodeDispatchContractVersionV3 || e.CheckedUnixNano <= 0 || e.ExpiresUnixNano <= e.CheckedUnixNano {
		return NewError(ErrorInvalidArgument, "cleanup_dispatch_incomplete", "cleanup dispatch envelope is incomplete")
	}
	if err := e.ClosureRef.Validate(); err != nil {
		return err
	}
	for _, ref := range []ExactRefV1{e.PlanRef, e.NodeRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if e.PlanRef != e.ClosureRef.PlanRef {
		return NewError(ErrorConflict, "cleanup_dispatch_plan_drift", "cleanup dispatch Plan does not match Closure")
	}
	if err := e.Target.Validate(); err != nil {
		return err
	}
	switch e.Target.Class {
	case CleanupTargetControlV3:
		if e.NodeRef.ID == CleanupBarrierHarnessCloseV2 || e.NodeRef.ID == CleanupBarrierSandboxFenceV2 || e.NodeRef.ID == CleanupBarrierSandboxReleaseV2 || e.NodeRef.ID == CleanupBarrierRuntimeCleanupAggregateV2 {
			return NewError(ErrorConflict, "cleanup_dispatch_node_class_drift", "control target cannot name a fixed barrier")
		}
	case CleanupTargetHarnessCloseV3:
		if e.NodeRef.ID != CleanupBarrierHarnessCloseV2 {
			return NewError(ErrorConflict, "cleanup_dispatch_node_class_drift", "Harness target names the wrong node")
		}
	case CleanupTargetSandboxFenceV3:
		if e.NodeRef.ID != CleanupBarrierSandboxFenceV2 {
			return NewError(ErrorConflict, "cleanup_dispatch_node_class_drift", "Sandbox fence target names the wrong node")
		}
	case CleanupTargetSandboxReleaseV3:
		if e.NodeRef.ID != CleanupBarrierSandboxReleaseV2 {
			return NewError(ErrorConflict, "cleanup_dispatch_node_class_drift", "Sandbox release target names the wrong node")
		}
	case CleanupTargetRuntimeAggregateV3:
		if e.NodeRef.ID != CleanupBarrierRuntimeCleanupAggregateV2 {
			return NewError(ErrorConflict, "cleanup_dispatch_node_class_drift", "Runtime aggregate target names the wrong node")
		}
	}
	if e.TargetDigest != e.Target.Digest {
		return NewError(ErrorConflict, "cleanup_dispatch_target_drift", "cleanup dispatch target digest drifted")
	}
	for _, current := range []runtimeports.OwnerCurrentRefV1{e.AuthorizationCurrent, e.FenceCurrent} {
		if err := current.Validate(); err != nil {
			return err
		}
		if e.ExpiresUnixNano > current.ExpiresUnixNano {
			return NewError(ErrorPrecondition, "cleanup_dispatch_ttl_drift", "cleanup dispatch exceeds a fresh current")
		}
	}
	for i, ref := range e.PredecessorBarriers {
		if err := ref.Validate(); err != nil {
			return err
		}
		if i > 0 {
			p := e.PredecessorBarriers[i-1]
			if p.Kind > ref.Kind || (p.Kind == ref.Kind && p.ID >= ref.ID) {
				return NewError(ErrorConflict, "cleanup_dispatch_barrier_duplicate", "cleanup dispatch barriers must be sorted and unique")
			}
		}
	}
	expected, err := e.digestV3()
	if err != nil {
		return err
	}
	if expected != e.Digest {
		return NewError(ErrorConflict, "cleanup_dispatch_digest_drift", "cleanup dispatch envelope digest drifted")
	}
	return nil
}
func (e CleanupNodeDispatchEnvelopeV3) ValidateCurrent(now time.Time) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < e.CheckedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "cleanup dispatch clock regressed")
	}
	if now.UnixNano() >= e.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "cleanup_dispatch_expired", "cleanup dispatch envelope expired")
	}
	return nil
}

// ValidateOwnerDispatchCurrent is the actual-point gate. attempt_inspect and
// not_constructed are valid orchestration records but never grant an Owner call.
func (e CleanupNodeDispatchEnvelopeV3) ValidateOwnerDispatchCurrent(now time.Time) error {
	if err := e.ValidateCurrent(now); err != nil {
		return err
	}
	if e.Target.Role != CleanupTargetConstructedV3 {
		return NewError(ErrorPrecondition, "cleanup_target_not_dispatchable", "only a constructed target permits Owner cleanup dispatch")
	}
	return nil
}

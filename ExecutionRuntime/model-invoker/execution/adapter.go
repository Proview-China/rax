package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type AdapterDescriptor struct {
	Identity       union.VersionedIdentity `json:"identity"`
	Origin         union.EventOrigin       `json:"origin"`
	ExecutionKinds []union.ExecutionKind   `json:"execution_kinds"`
}

func (descriptor AdapterDescriptor) Validate() error {
	if err := descriptor.Identity.Validate("adapter.identity"); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidAdapter, err)
	}
	switch descriptor.Origin {
	case union.EventOriginModel, union.EventOriginProvider, union.EventOriginHarness, union.EventOriginExternal:
	default:
		return fmt.Errorf("%w: adapter origin must be non-Praxis", ErrInvalidAdapter)
	}
	if len(descriptor.ExecutionKinds) == 0 {
		return fmt.Errorf("%w: execution kinds are required", ErrInvalidAdapter)
	}
	seen := make(map[union.ExecutionKind]struct{}, len(descriptor.ExecutionKinds))
	for _, kind := range descriptor.ExecutionKinds {
		if kind != union.ExecutionKindModel && kind != union.ExecutionKindAgent {
			return fmt.Errorf("%w: adapter execution kind is unsupported", ErrInvalidAdapter)
		}
		if _, duplicate := seen[kind]; duplicate {
			return fmt.Errorf("%w: adapter execution kinds contain duplicates", ErrInvalidAdapter)
		}
		seen[kind] = struct{}{}
	}
	return nil
}

func (descriptor AdapterDescriptor) Supports(kind union.ExecutionKind) bool {
	for _, supported := range descriptor.ExecutionKinds {
		if supported == kind {
			return true
		}
	}
	return false
}

type Invocation struct {
	Request union.UnifiedExecutionRequest `json:"request"`
	Plan    union.PreparedExecutionPlan   `json:"plan"`
}

// NewInvocation seals a request and prepared plan into one immutable
// invocation boundary. Profile Compiler output is already sealed; this helper
// primarily supports explicitly constructed plans and rejects conflicting
// pre-existing digests rather than silently replacing them.
func NewInvocation(request union.UnifiedExecutionRequest, plan union.PreparedExecutionPlan) (Invocation, error) {
	requestClone, err := request.Clone()
	if err != nil {
		return Invocation{}, fmt.Errorf("%w: clone request", ErrInvalidInvocation)
	}
	planClone, err := plan.Clone()
	if err != nil {
		return Invocation{}, fmt.Errorf("%w: clone plan", ErrInvalidInvocation)
	}
	requestDigest, err := requestClone.Digest()
	if err != nil {
		return Invocation{}, fmt.Errorf("%w: request digest: %v", ErrInvalidInvocation, err)
	}
	if planClone.Metadata == nil {
		planClone.Metadata = make(map[string]string)
	}
	if pinned := planClone.Metadata["request_digest"]; pinned != "" && pinned != requestDigest {
		return Invocation{}, fmt.Errorf("%w: prepared plan is bound to a different request", ErrInvalidInvocation)
	}
	planClone.Metadata["request_digest"] = requestDigest
	providedPlanDigest := planClone.Digest
	computedPlanDigest, err := planClone.ComputeDigest()
	if err != nil {
		return Invocation{}, fmt.Errorf("%w: plan digest: %v", ErrInvalidInvocation, err)
	}
	if providedPlanDigest != "" && providedPlanDigest != computedPlanDigest {
		return Invocation{}, fmt.Errorf("%w: prepared plan digest differs", ErrInvalidInvocation)
	}
	planClone.Digest = computedPlanDigest
	invocation := Invocation{Request: requestClone, Plan: planClone}
	if err := invocation.Validate(); err != nil {
		return Invocation{}, err
	}
	return invocation, nil
}

func (invocation Invocation) Clone() (Invocation, error) {
	request, err := invocation.Request.Clone()
	if err != nil {
		return Invocation{}, err
	}
	plan, err := invocation.Plan.Clone()
	if err != nil {
		return Invocation{}, err
	}
	return Invocation{Request: request, Plan: plan}, nil
}

func (invocation Invocation) Validate() error {
	if err := invocation.Request.Validate(); err != nil {
		return fmt.Errorf("%w: request: %v", ErrInvalidInvocation, err)
	}
	if err := invocation.Plan.Validate(); err != nil {
		return fmt.Errorf("%w: plan: %v", ErrInvalidInvocation, err)
	}
	if invocation.Request.ExecutionID != invocation.Plan.ExecutionID {
		return fmt.Errorf("%w: execution identity differs", ErrInvalidInvocation)
	}
	if invocation.Request.ExecutionKind != union.ExecutionKindAuto && invocation.Request.ExecutionKind != invocation.Plan.ExecutionKind {
		return fmt.Errorf("%w: execution kind differs", ErrInvalidInvocation)
	}
	requestDigest, err := invocation.Request.Digest()
	if err != nil {
		return fmt.Errorf("%w: request digest: %v", ErrInvalidInvocation, err)
	}
	if invocation.Plan.Metadata["request_digest"] != requestDigest {
		return fmt.Errorf("%w: prepared plan request binding differs", ErrInvalidInvocation)
	}
	if strings.TrimSpace(invocation.Plan.Digest) == "" {
		return fmt.Errorf("%w: prepared plan digest is required", ErrInvalidInvocation)
	}
	computedPlanDigest, err := invocation.Plan.ComputeDigest()
	if err != nil {
		return fmt.Errorf("%w: plan digest: %v", ErrInvalidInvocation, err)
	}
	if computedPlanDigest != invocation.Plan.Digest {
		return fmt.Errorf("%w: prepared plan digest differs", ErrInvalidInvocation)
	}
	if invocation.Request.ProfileSelector.Exact != nil && *invocation.Request.ProfileSelector.Exact != invocation.Plan.Profile {
		return fmt.Errorf("%w: exact profile selector differs from prepared plan", ErrInvalidInvocation)
	}
	requestGraphDigest, err := canonicalIntentGraphDigest(invocation.Request.IntentGraph)
	if err != nil {
		return fmt.Errorf("%w: request intent graph: %v", ErrInvalidInvocation, err)
	}
	planGraphDigest, err := canonicalIntentGraphDigest(invocation.Plan.IntentGraph)
	if err != nil {
		return fmt.Errorf("%w: plan intent graph: %v", ErrInvalidInvocation, err)
	}
	if requestGraphDigest != planGraphDigest {
		return fmt.Errorf("%w: request and prepared plan intent graphs differ", ErrInvalidInvocation)
	}
	if expected := invocation.Request.SessionIntent.ExpectedProfile; !zeroIdentity(expected) && expected != invocation.Plan.Profile {
		return fmt.Errorf("%w: expected session profile differs from prepared plan", ErrInvalidInvocation)
	}
	if expected := invocation.Request.SessionIntent.ExpectedRoute; !zeroIdentity(expected) && expected != invocation.Plan.Route {
		return fmt.Errorf("%w: expected session route differs from prepared plan", ErrInvalidInvocation)
	}
	covered := make(map[union.IntentID]bool, len(invocation.Plan.Mechanisms))
	for _, mechanism := range invocation.Plan.Mechanisms {
		covered[mechanism.IntentID] = true
	}
	for _, intent := range invocation.Plan.IntentGraph.Nodes {
		if !covered[intent.ID] {
			return fmt.Errorf("%w: every intent requires at least one mechanism", ErrInvalidInvocation)
		}
	}
	return nil
}

func canonicalIntentGraphDigest(graph union.IntentGraph) (string, error) {
	if err := graph.Validate(); err != nil {
		return "", err
	}
	clone, err := cloneIntentGraph(graph)
	if err != nil {
		return "", err
	}
	for index := range clone.Nodes {
		sort.Slice(clone.Nodes[index].DependsOn, func(left, right int) bool {
			return clone.Nodes[index].DependsOn[left] < clone.Nodes[index].DependsOn[right]
		})
		sort.Slice(clone.Nodes[index].AcceptedFidelity, func(left, right int) bool {
			return clone.Nodes[index].AcceptedFidelity[left] < clone.Nodes[index].AcceptedFidelity[right]
		})
	}
	sort.Slice(clone.Nodes, func(left, right int) bool { return clone.Nodes[left].ID < clone.Nodes[right].ID })
	return union.StableDigest(clone)
}

func cloneIntentGraph(graph union.IntentGraph) (union.IntentGraph, error) {
	encoded, err := json.Marshal(graph)
	if err != nil {
		return union.IntentGraph{}, err
	}
	var clone union.IntentGraph
	if err := json.Unmarshal(encoded, &clone); err != nil {
		return union.IntentGraph{}, err
	}
	return clone, nil
}

type PreflightReport struct {
	Accepted       bool                         `json:"accepted"`
	ActualManifest union.ContextManifestSummary `json:"actual_manifest"`
	Residuals      []union.Residual             `json:"residuals,omitempty"`
	RejectionCode  string                       `json:"rejection_code,omitempty"`
}

func (report PreflightReport) Validate() error {
	if report.Accepted {
		if strings.TrimSpace(report.RejectionCode) != "" {
			return fmt.Errorf("%w: accepted report contains a rejection code", ErrInvalidAdapter)
		}
		if err := report.ActualManifest.Validate(); err != nil {
			return fmt.Errorf("%w: actual manifest: %v", ErrInvalidAdapter, err)
		}
		return nil
	}
	if strings.TrimSpace(report.RejectionCode) == "" {
		return fmt.Errorf("%w: rejected report requires a stable code", ErrInvalidAdapter)
	}
	return nil
}

type Adapter interface {
	Describe(context.Context) (AdapterDescriptor, error)
	Preflight(context.Context, Invocation) (PreflightReport, error)
	Open(context.Context, Invocation) (Session, error)
}

// PreflightCleaner is implemented by Adapters that retain a probed process or
// session between Preflight and Open. Runtime calls it on every post-preflight
// failure; implementations must be idempotent.
type PreflightCleaner interface {
	ClosePrepared(union.ExecutionID) error
}

type Session interface {
	Receive(context.Context) (union.UnifiedExecutionEvent, error)
	Command(context.Context, union.ExecutionCommand) error
	Close() error
}

// CandidateHeader supplies only adapter-owned metadata. Runtime replaces all
// global identity, ordering, routing, and ingestion fields before committing
// the candidate to the EventLedger.
func CandidateHeader(origin union.EventOrigin, family union.EventFamily) union.EventHeader {
	return union.EventHeader{
		Origin: origin, Family: family,
		Visibility: union.VisibilityAuditOnly, SecurityClassification: union.SecurityInternal,
	}
}

type RouteTerminalCandidate struct {
	Status                union.ExecutionStatus `json:"status"`
	StopReason            string                `json:"stop_reason,omitempty"`
	PendingBackgroundWork int                   `json:"pending_background_work,omitempty"`
	SideEffectState       union.SideEffectState `json:"side_effect_state"`
}

type ReconcileInput struct {
	Invocation Invocation
	Events     []union.UnifiedExecutionEvent
	State      LedgerState
	Candidate  RouteTerminalCandidate
}

type ReconcileReport struct {
	Effects         []union.EffectRecord  `json:"effects,omitempty"`
	SideEffectState union.SideEffectState `json:"side_effect_state"`
	Quiesced        bool                  `json:"quiesced"`
	Residuals       []union.Residual      `json:"residuals,omitempty"`
}

type Reconciler interface {
	Reconcile(context.Context, ReconcileInput) (ReconcileReport, error)
}

type VerifyInput struct {
	Invocation Invocation
	Events     []union.UnifiedExecutionEvent
	Effects    []union.EffectRecord
}

type VerificationReport struct {
	Verifications []union.VerificationRecord `json:"verifications,omitempty"`
	Residuals     []union.Residual           `json:"residuals,omitempty"`
}

type Verifier interface {
	Verify(context.Context, VerifyInput) (VerificationReport, error)
}

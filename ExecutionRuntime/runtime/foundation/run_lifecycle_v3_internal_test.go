package foundation

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type lifecyclePortStubV3 struct {
	terminal   ports.RunLifecycleEnvelopeV3
	closed     ports.RunLifecycleEnvelopeV3
	stops      int
	reconciles int
	stopErr    error
}

func (s *lifecyclePortStubV3) CreatePendingRunV3(context.Context, ports.CreatePendingRunRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidState, "not used")
}
func (s *lifecyclePortStubV3) BeginStopRunV3(context.Context, ports.BeginStopRunRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	return s.terminal, nil
}
func (s *lifecyclePortStubV3) StopAndSettleRunV3(context.Context, ports.BeginStopRunRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	s.stops++
	if s.stopErr != nil {
		return ports.RunLifecycleEnvelopeV3{}, s.stopErr
	}
	return s.terminal, nil
}
func (s *lifecyclePortStubV3) InspectRunLifecycleV3(context.Context, core.ExecutionScope, core.AgentRunID) (ports.RunLifecycleEnvelopeV3, error) {
	return s.terminal, nil
}
func (s *lifecyclePortStubV3) ReconcileRunTerminationV3(context.Context, ports.RunTerminationRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	s.reconciles++
	return s.closed, nil
}
func (s *lifecyclePortStubV3) InspectRunTerminationV3(context.Context, ports.RunTerminationRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	if s.reconciles > 0 {
		return s.closed, nil
	}
	return s.terminal, nil
}

func TestFoundationRunLifecycleV3ResumesTerminalCleanupWithoutExternalReplay(t *testing.T) {
	now := time.Unix(410_000, 0)
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-foundation-v3", ID: "identity-foundation-v3", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-foundation-v3", PlanDigest: core.DigestBytes([]byte("lineage"))},
		Instance:       core.InstanceRef{ID: "instance-foundation-v3", Epoch: 1},
		SandboxLease:   &core.SandboxLeaseRef{ID: "lease-foundation-v3", Epoch: 1},
		AuthorityEpoch: 1,
	}
	aggregate, err := kernel.NewAggregate(scope, core.InstanceState{Phase: core.PhaseRunning, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true})
	if err != nil {
		t.Fatal(err)
	}
	run := core.AgentRunRecord{ID: "run-foundation-v3", Scope: scope, Status: core.RunTerminal, Revision: 4, SessionRef: "runtime-session:foundation-v3", StartedAt: now.Add(-time.Minute), EndedAt: now, Outcome: core.OutcomeCompleted}
	runIdentity, _ := ports.RunIdentityDigestV2(run)
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	closure := ports.RunSettlementClosureRefV3{ID: "closure-foundation-v3", RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScopeDigest: scopeDigest, Attempt: 1, Revision: 1, Digest: digest("closure")}
	decision := ports.RunSettlementDecisionRefV3{ID: "decision-foundation-v3", RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScopeDigest: scopeDigest, Revision: 1, Digest: digest("decision"), Outcome: run.Outcome, Closure: closure}
	progress := ports.RunTerminationProgressRefV3{ID: "progress-foundation-v3", RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScopeDigest: scopeDigest, Revision: 1, Digest: digest("progress-pending"), UnresolvedCount: 1, Decision: decision}
	base := ports.RunLifecycleEnvelopeV3{
		ContractVersion: ports.RunLifecycleContractVersionV3,
		Phase:           ports.RunLifecycleTerminalCleanupV3,
		Run:             run,
		Plan:            ports.RunSettlementPlanLifecycleRefV3{RunSettlementPlanRefV2: ports.RunSettlementPlanRefV2{ID: "plan-foundation-v3", Revision: 1, Digest: digest("plan")}, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScopeDigest: scopeDigest},
		Certification:   ports.RunSettlementPlanCertificationAssociationV3{Certification: ports.RunSettlementPlanCertificationRefV3{ID: "cert-foundation-v3", Revision: 1, Digest: digest("cert")}, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScopeDigest: scopeDigest, Plan: ports.RunSettlementPlanRefV2{ID: "plan-foundation-v3", Revision: 1, Digest: digest("plan")}},
		EffectIndex:     ports.RunEffectIndexRefV3{ID: "index-foundation-v3", Revision: 2, Digest: digest("index"), RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScopeDigest: scopeDigest, Watermark: 2, SegmentCount: 0, EffectCount: 0, HeadDigest: ports.EvidenceGenesisDigestV2, Frozen: true},
		Closure:         &closure,
		Decision:        &decision,
		Progress:        &progress,
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	closed := base
	closed.Phase = ports.RunLifecycleTerminationClosedV3
	closedProgress := progress
	closedProgress.Revision = 2
	closedProgress.Digest = digest("progress-closed")
	closedProgress.UnresolvedCount = 0
	closed.Progress = &closedProgress
	report := ports.RunTerminationReportRefV3{ID: "report-foundation-v3", RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScopeDigest: scopeDigest, Revision: 1, Digest: digest("report"), Decision: decision, Progress: closedProgress}
	closed.Report = &report
	if err := closed.Validate(); err != nil {
		t.Fatal(err)
	}
	port := &lifecyclePortStubV3{terminal: base, closed: closed}
	instance := &Instance{aggregate: aggregate, activeRun: &core.AgentRunRecord{ID: run.ID, Scope: scope, Status: core.RunRunning, Revision: 2, SessionRef: run.SessionRef, StartedAt: run.StartedAt}}
	first := &Coordinator{RunLifecycle: port}
	stop := ports.BeginStopRunRequestV3{ExecutionScope: scope, RunID: run.ID, ExpectedRunRevision: 2}
	terminal, err := first.StopAndSettleRunV3(context.Background(), instance, stop)
	if err != nil || terminal.Progress == nil || terminal.Progress.UnresolvedCount != 1 || instance.aggregate.Snapshot().State.Phase != core.PhaseStopping {
		t.Fatalf("terminal cleanup watermark was not retained: envelope=%#v err=%v", terminal, err)
	}
	// A fresh coordinator resumes only through public lifecycle facts. It has
	// no Execution/Environment port, so successful recovery proves that no
	// Close/Fence/Release action was replayed.
	restarted := &Coordinator{RunLifecycle: port}
	resolved, err := restarted.ReconcileRunTerminationV3(context.Background(), instance, ports.RunTerminationRequestV3{ExecutionScope: scope, RunID: run.ID})
	if err != nil || resolved.Report == nil || instance.aggregate.Snapshot().State.Phase != core.PhaseTerminal || port.stops != 1 || port.reconciles != 1 {
		t.Fatalf("restart did not close termination by inspection only: envelope=%#v stops=%d reconciles=%d err=%v", resolved, port.stops, port.reconciles, err)
	}
}

func TestFoundationRunLifecycleV3PreflightFailureDoesNotMutateLocalInstance(t *testing.T) {
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-foundation-preflight", ID: "identity-foundation-preflight", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-foundation-preflight", PlanDigest: core.DigestBytes([]byte("lineage-preflight"))},
		Instance:       core.InstanceRef{ID: "instance-foundation-preflight", Epoch: 1},
		SandboxLease:   &core.SandboxLeaseRef{ID: "lease-foundation-preflight", Epoch: 1},
		AuthorityEpoch: 1,
	}
	aggregate, err := kernel.NewAggregate(scope, core.InstanceState{Phase: core.PhaseRunning, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true})
	if err != nil {
		t.Fatal(err)
	}
	run := core.AgentRunRecord{ID: "run-foundation-preflight", Scope: scope, Status: core.RunRunning, Revision: 2, SessionRef: "runtime-session:foundation-preflight", StartedAt: time.Unix(420_000, 0)}
	instance := &Instance{aggregate: aggregate, activeRun: &run}
	port := &lifecyclePortStubV3{stopErr: core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "certified lifecycle preflight failed")}
	coordinator := &Coordinator{RunLifecycle: port}
	request := ports.BeginStopRunRequestV3{ExecutionScope: scope, RunID: run.ID, ExpectedRunRevision: run.Revision}
	if _, err := coordinator.StopAndSettleRunV3(context.Background(), instance, request); !core.HasReason(err, core.ReasonRunSettlementPlanConflict) {
		t.Fatalf("unexpected preflight error: %v", err)
	}
	snapshot := instance.Snapshot()
	if snapshot.Kernel.State.Phase != core.PhaseRunning || snapshot.ActiveRun == nil || snapshot.ActiveRun.Status != core.RunRunning || snapshot.ActiveRun.Revision != run.Revision {
		t.Fatalf("failed Runtime preflight mutated local instance state: %#v", snapshot)
	}
	if port.stops != 1 {
		t.Fatalf("expected one certified lifecycle attempt, got %d", port.stops)
	}
}

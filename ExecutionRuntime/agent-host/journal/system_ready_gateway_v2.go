package journal

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// SystemReadyGatewayV2 is a deterministic reference gateway. Attempt truth is
// exclusively read and advanced through SystemReadyAttemptFactPortV2; the
// supplied reference stores remain non-durable and do not claim production.
type SystemReadyGatewayV2 struct {
	clockMu    sync.Mutex
	facts      ports.SystemReadyFactPortV2
	attempts   ports.SystemReadyAttemptFactPortV2
	claims     ports.HostStartClaimCurrentReaderV1
	core       ports.SystemReadyCoreCurrentReadersV2
	components ports.ComponentProductionCurrentReaderRegistryV2
	now        func() time.Time
	last       time.Time
}

func NewSystemReadyGatewayV2(facts ports.SystemReadyFactPortV2, attempts ports.SystemReadyAttemptFactPortV2, claims ports.HostStartClaimCurrentReaderV1, readers ports.SystemReadyCoreCurrentReadersV2, components ports.ComponentProductionCurrentReaderRegistryV2, now func() time.Time) (*SystemReadyGatewayV2, error) {
	if contract.IsTypedNilV1(facts) || contract.IsTypedNilV1(attempts) || contract.IsTypedNilV1(claims) || contract.IsTypedNilV1(readers) || contract.IsTypedNilV1(components) {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "system_ready_gateway_dependency_missing", "all SystemReady gateway dependencies are required")
	}
	if now == nil {
		now = time.Now
	}
	return &SystemReadyGatewayV2{facts: facts, attempts: attempts, claims: claims, core: readers, components: components, now: now}, nil
}
func (g *SystemReadyGatewayV2) freshNow() (time.Time, error) {
	g.clockMu.Lock()
	defer g.clockMu.Unlock()
	n, e := safeNowV1(g.now)
	if e != nil {
		return time.Time{}, e
	}
	if !g.last.IsZero() && n.Before(g.last) {
		return time.Time{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "SystemReady gateway clock regressed")
	}
	g.last = n
	return n, nil
}

func (g *SystemReadyGatewayV2) StartOrInspectSystemReadyV2(ctx context.Context, request contract.SystemReadyEnsureRequestV2) (contract.SystemReadyGatewayResultV2, error) {
	if g == nil || contract.IsTypedNilV1(g.facts) || contract.IsTypedNilV1(g.attempts) {
		return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorUnavailable, "system_ready_gateway_missing", "SystemReady gateway is unavailable")
	}
	if ctx == nil {
		return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := request.Validate(); err != nil {
		return contract.SystemReadyGatewayResultV2{}, err
	}
	key := contract.NewSystemReadyAttemptStepKeyV2(request.HostID, request.StartID, request.AttemptID)
	current, err := g.attempts.InspectSystemReadyAttemptV2(ctx, key)
	if err == nil {
		return g.resumeSystemReadyAttemptV2(ctx, current, request)
	}
	if !contract.HasCode(err, contract.ErrorNotFound) {
		return contract.SystemReadyGatewayResultV2{}, err
	}
	intent, err := g.prepareSystemReadyIntentV2(ctx, request, key)
	if err != nil {
		return contract.SystemReadyGatewayResultV2{}, err
	}
	created, writeErr := g.attempts.CreateSystemReadyAttemptV2(ctx, intent)
	if writeErr != nil {
		actual, inspectErr := g.attempts.InspectSystemReadyAttemptV2(context.WithoutCancel(ctx), key)
		if inspectErr != nil {
			return contract.SystemReadyGatewayResultV2{}, writeErr
		}
		if actual.StepKey != intent.StepKey || actual.Request.RequestDigest != intent.Request.RequestDigest {
			return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_conflict", "attempt ID already binds another request")
		}
		// A recovered or concurrent intent never grants permission to dispatch
		// SystemReady writes; it can only Inspect the recorded candidates.
		return g.resumeSystemReadyAttemptV2(ctx, actual, request)
	}
	if created.Digest != intent.Digest {
		return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_create_drift", "attempt create returned another fact")
	}
	return g.executeOwnedSystemReadyIntentV2(ctx, created)
}

func (g *SystemReadyGatewayV2) InspectSystemReadyV2(ctx context.Context, request contract.SystemReadyInspectRequestV2) (contract.SystemReadyGatewayResultV2, error) {
	if g == nil {
		return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorUnavailable, "system_ready_gateway_missing", "SystemReady gateway unavailable")
	}
	if ctx == nil {
		return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context required")
	}
	if err := request.Validate(); err != nil {
		return contract.SystemReadyGatewayResultV2{}, err
	}
	key := contract.NewSystemReadyAttemptStepKeyV2(request.HostID, request.StartID, request.AttemptID)
	a, err := g.attempts.InspectSystemReadyAttemptV2(ctx, key)
	if err != nil {
		return contract.SystemReadyGatewayResultV2{}, err
	}
	if a.Request.RequestDigest != request.RequestDigest {
		return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_conflict", "attempt digest drifted")
	}
	if a.State != contract.SystemReadyAttemptResultRecordedV2 || a.Result == nil {
		return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorUnknownOutcome, "system_ready_attempt_incomplete", "attempt has no authoritative result")
	}
	return *a.Result, nil
}

func (g *SystemReadyGatewayV2) prepareSystemReadyIntentV2(ctx context.Context, request contract.SystemReadyEnsureRequestV2, key contract.SystemReadyAttemptStepKeyV2) (contract.SystemReadyAttemptFactV2, error) {
	n1, err := g.freshNow()
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	if err = g.inspectAll(ctx, request, n1); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	n2, err := g.freshNow()
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	if err = g.inspectAll(ctx, request, n2); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	fact, err := buildSystemReadyFactV2(request, n2)
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	n3, err := g.freshNow()
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	if err = g.inspectAll(ctx, request, n3); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	current := contract.SystemReadyCurrentV2{Ref: contract.SystemReadyCurrentRefV2{ID: contract.DeriveSystemReadyCurrentIDV2(request.HostID, request.StartID), Revision: 1, Epoch: request.AvailabilityEpoch}, HostID: request.HostID, StartID: request.StartID, FactRef: fact.Ref, State: contract.SystemReadyCurrentReadyV2, CheckedUnixNano: n3.UnixNano(), ExpiresUnixNano: fact.ExpiresUnixNano}
	if request.ExpectedCurrent != nil {
		old, inspectErr := g.facts.InspectSystemReadyCurrentV2(ctx, *request.ExpectedCurrent)
		if inspectErr != nil {
			return contract.SystemReadyAttemptFactV2{}, inspectErr
		}
		if inspectErr = old.ValidateCurrent(*request.ExpectedCurrent, n3); inspectErr != nil {
			return contract.SystemReadyAttemptFactV2{}, inspectErr
		}
		current.Ref.Revision = old.Ref.Revision + 1
		current.Ref.Epoch = old.Ref.Epoch
	}
	current, err = contract.SealSystemReadyCurrentV2(current)
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	return contract.SealSystemReadyAttemptFactV2(contract.SystemReadyAttemptFactV2{StepKey: key, Revision: 1, Request: request, FactCandidate: fact, CurrentCandidate: current, State: contract.SystemReadyAttemptIntentRecordedV2, CreatedUnixNano: n3.UnixNano(), UpdatedUnixNano: n3.UnixNano()})
}

func (g *SystemReadyGatewayV2) executeOwnedSystemReadyIntentV2(ctx context.Context, attempt contract.SystemReadyAttemptFactV2) (contract.SystemReadyGatewayResultV2, error) {
	fact, writeErr := g.facts.CreateSystemReadyFactV2(ctx, attempt.FactCandidate)
	if writeErr != nil {
		fact, _ = g.facts.InspectSystemReadyFactV2(context.WithoutCancel(ctx), attempt.FactCandidate.Ref)
	}
	if fact.Digest != attempt.FactCandidate.Digest {
		_, _ = g.markSystemReadyUnknownV2(context.WithoutCancel(ctx), attempt)
		return contract.SystemReadyGatewayResultV2{}, writeErrOrIndeterminateV2(writeErr, "SystemReady Fact outcome is unknown")
	}
	var current contract.SystemReadyCurrentV2
	if attempt.Request.ExpectedCurrent == nil {
		current, writeErr = g.facts.CreateSystemReadyCurrentV2(ctx, attempt.CurrentCandidate)
	} else {
		current, writeErr = g.facts.CompareAndSwapSystemReadyCurrentV2(ctx, *attempt.Request.ExpectedCurrent, attempt.CurrentCandidate)
	}
	if writeErr != nil {
		current, _ = g.facts.InspectSystemReadyCurrentV2(context.WithoutCancel(ctx), attempt.CurrentCandidate.Ref)
	}
	if current.ProjectionDigest != attempt.CurrentCandidate.ProjectionDigest {
		_, _ = g.markSystemReadyUnknownV2(context.WithoutCancel(ctx), attempt)
		return contract.SystemReadyGatewayResultV2{}, writeErrOrIndeterminateV2(writeErr, "SystemReady Current outcome is unknown")
	}
	latest, err := g.attempts.InspectSystemReadyAttemptV2(context.WithoutCancel(ctx), attempt.StepKey)
	if err != nil {
		return contract.SystemReadyGatewayResultV2{}, err
	}
	return g.recordSystemReadyResultV2(ctx, latest)
}

func writeErrOrIndeterminateV2(err error, message string) error {
	if err != nil {
		return err
	}
	return contract.NewError(contract.ErrorUnknownOutcome, "system_ready_outcome_unknown", message)
}

func (g *SystemReadyGatewayV2) resumeSystemReadyAttemptV2(ctx context.Context, attempt contract.SystemReadyAttemptFactV2, request contract.SystemReadyEnsureRequestV2) (contract.SystemReadyGatewayResultV2, error) {
	if err := attempt.Validate(); err != nil {
		return contract.SystemReadyGatewayResultV2{}, err
	}
	if attempt.Request.RequestDigest != request.RequestDigest {
		return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_conflict", "attempt binds another request")
	}
	if attempt.State == contract.SystemReadyAttemptResultRecordedV2 && attempt.Result != nil {
		return *attempt.Result, nil
	}
	fact, factErr := g.facts.InspectSystemReadyFactV2(ctx, attempt.FactCandidate.Ref)
	current, currentErr := g.facts.InspectSystemReadyCurrentV2(ctx, attempt.CurrentCandidate.Ref)
	if factErr == nil && fact.Digest != attempt.FactCandidate.Digest {
		return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorConflict, "system_ready_fact_recovery_drift", "persisted SystemReady Fact drifted from the attempt candidate")
	}
	if currentErr == nil && current.ProjectionDigest != attempt.CurrentCandidate.ProjectionDigest {
		return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorConflict, "system_ready_current_recovery_drift", "persisted SystemReady Current drifted from the attempt candidate")
	}
	if factErr == nil && currentErr == nil && fact.Digest == attempt.FactCandidate.Digest && current.ProjectionDigest == attempt.CurrentCandidate.ProjectionDigest {
		return g.recordSystemReadyResultV2(ctx, attempt)
	}
	if factErr != nil && !contract.HasCode(factErr, contract.ErrorNotFound) {
		return contract.SystemReadyGatewayResultV2{}, factErr
	}
	if currentErr != nil && !contract.HasCode(currentErr, contract.ErrorNotFound) {
		return contract.SystemReadyGatewayResultV2{}, currentErr
	}
	if attempt.State == contract.SystemReadyAttemptIntentRecordedV2 {
		var err error
		attempt, err = g.markSystemReadyUnknownV2(ctx, attempt)
		if err != nil {
			return contract.SystemReadyGatewayResultV2{}, err
		}
	}
	if attempt.State == contract.SystemReadyAttemptOutcomeUnknownV2 {
		var err error
		attempt, err = g.advanceSystemReadyAttemptV2(ctx, attempt, contract.SystemReadyAttemptReconciliationRequiredV2, nil)
		if err != nil {
			return contract.SystemReadyGatewayResultV2{}, err
		}
	}
	return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorUnknownOutcome, "system_ready_reconciliation_required", "SystemReady attempt requires reconciliation and can only Inspect its recorded candidates")
}

func (g *SystemReadyGatewayV2) markSystemReadyUnknownV2(ctx context.Context, attempt contract.SystemReadyAttemptFactV2) (contract.SystemReadyAttemptFactV2, error) {
	if attempt.State != contract.SystemReadyAttemptIntentRecordedV2 {
		return attempt, nil
	}
	return g.advanceSystemReadyAttemptV2(ctx, attempt, contract.SystemReadyAttemptOutcomeUnknownV2, nil)
}

func (g *SystemReadyGatewayV2) recordSystemReadyResultV2(ctx context.Context, attempt contract.SystemReadyAttemptFactV2) (contract.SystemReadyGatewayResultV2, error) {
	result, err := contract.SealSystemReadyGatewayResultV2(contract.SystemReadyGatewayResultV2{AttemptID: attempt.Request.AttemptID, RequestDigest: attempt.Request.RequestDigest, Fact: attempt.FactCandidate.Ref, Current: attempt.CurrentCandidate.Ref})
	if err != nil {
		return contract.SystemReadyGatewayResultV2{}, err
	}
	for range 4 {
		if attempt.State == contract.SystemReadyAttemptResultRecordedV2 && attempt.Result != nil {
			return *attempt.Result, nil
		}
		next, advanceErr := g.advanceSystemReadyAttemptV2(ctx, attempt, contract.SystemReadyAttemptResultRecordedV2, &result)
		if advanceErr == nil {
			if next.Result == nil {
				return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorUnknownOutcome, "system_ready_result_missing", "SystemReady result successor is missing")
			}
			return *next.Result, nil
		}
		latest, inspectErr := g.attempts.InspectSystemReadyAttemptV2(context.WithoutCancel(ctx), attempt.StepKey)
		if inspectErr != nil {
			return contract.SystemReadyGatewayResultV2{}, advanceErr
		}
		if latest.Request.RequestDigest != attempt.Request.RequestDigest || latest.FactCandidate.Digest != attempt.FactCandidate.Digest || latest.CurrentCandidate.ProjectionDigest != attempt.CurrentCandidate.ProjectionDigest {
			return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_recovery_drift", "attempt advanced with different immutable candidates")
		}
		attempt = latest
	}
	return contract.SystemReadyGatewayResultV2{}, contract.NewError(contract.ErrorUnknownOutcome, "system_ready_result_contention", "SystemReady result could not settle after bounded CAS contention")
}

func (g *SystemReadyGatewayV2) advanceSystemReadyAttemptV2(ctx context.Context, current contract.SystemReadyAttemptFactV2, state contract.SystemReadyAttemptStateV2, result *contract.SystemReadyGatewayResultV2) (contract.SystemReadyAttemptFactV2, error) {
	now, err := g.freshNow()
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	next := current.CloneV2()
	next.Revision++
	next.State = state
	next.Result = result
	next.UpdatedUnixNano = now.UnixNano()
	next.Digest = ""
	next, err = contract.SealSystemReadyAttemptFactV2(next)
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	written, writeErr := g.attempts.CompareAndSwapSystemReadyAttemptV2(ctx, current.RefV2(), next)
	if writeErr == nil {
		if written.Digest != next.Digest {
			return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_successor_drift", "attempt CAS returned another successor")
		}
		return written, nil
	}
	actual, inspectErr := g.attempts.InspectSystemReadyAttemptV2(context.WithoutCancel(ctx), current.StepKey)
	if inspectErr == nil && actual.Digest == next.Digest {
		return actual, nil
	}
	if inspectErr == nil {
		switch state {
		case contract.SystemReadyAttemptOutcomeUnknownV2:
			if actual.State == contract.SystemReadyAttemptOutcomeUnknownV2 || actual.State == contract.SystemReadyAttemptReconciliationRequiredV2 || actual.State == contract.SystemReadyAttemptResultRecordedV2 {
				return actual, nil
			}
		case contract.SystemReadyAttemptReconciliationRequiredV2:
			if actual.State == contract.SystemReadyAttemptReconciliationRequiredV2 || actual.State == contract.SystemReadyAttemptResultRecordedV2 {
				return actual, nil
			}
		case contract.SystemReadyAttemptResultRecordedV2:
			if actual.State == contract.SystemReadyAttemptResultRecordedV2 && actual.Result != nil {
				return actual, nil
			}
		}
	}
	return contract.SystemReadyAttemptFactV2{}, writeErr
}

func (g *SystemReadyGatewayV2) inspectAll(ctx context.Context, r contract.SystemReadyEnsureRequestV2, now time.Time) error {
	claim, err := g.claims.InspectHostStartClaimCurrentV1(ctx, r.Claim)
	if err != nil {
		return err
	}
	ref, err := claim.CurrentRefV1()
	if err != nil {
		return err
	}
	if ref != r.Claim {
		return contract.NewError(contract.ErrorConflict, "system_ready_claim_drift", "claim exact Ref drifted")
	}
	if err = claim.ValidateCurrentV1(now); err != nil {
		return err
	}
	checks := []struct {
		expected runtimeports.OwnerCurrentRefV1
		call     func(context.Context, runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error)
	}{{r.Definition, g.core.InspectDefinitionCurrentV2}, {r.Plan, g.core.InspectPlanCurrentV2}, {r.Assembly, g.core.InspectAssemblyCurrentV2}, {r.BindingSet, g.core.InspectBindingSetCurrentV2}, {r.Activation, g.core.InspectActivationCurrentV2}, {r.GenerationBinding, g.core.InspectGenerationBindingCurrentV2}, {r.ApplicationStart, g.core.InspectApplicationStartCurrentV2}, {r.SandboxLease, g.core.InspectSandboxLeaseCurrentV2}, {r.SandboxActive, g.core.InspectSandboxActiveCurrentV2}, {r.ExecutionReady, g.core.InspectExecutionReadyCurrentV2}}
	for _, c := range checks {
		actual, e := c.call(ctx, c.expected)
		if e != nil {
			return e
		}
		if actual != c.expected {
			return contract.NewError(contract.ErrorConflict, "system_ready_owner_current_drift", "owner current reader returned another exact Ref")
		}
		if now.UnixNano() >= actual.ExpiresUnixNano {
			return contract.NewError(contract.ErrorPrecondition, "system_ready_owner_current_expired", "owner current expired")
		}
	}
	policy, err := g.core.InspectSupervisionPolicyCurrentV2(ctx, r.SupervisionPolicy)
	if err != nil {
		return err
	}
	if err = policy.Validate(); err != nil {
		return err
	}
	if policy.Ref != r.SupervisionPolicy || policy.MinimumReadyWindowNanos != r.MinimumReadyWindowNanos {
		return contract.NewError(contract.ErrorConflict, "system_ready_supervision_policy_drift", "supervision policy projection drifted from the request")
	}
	if now.UnixNano() < policy.CheckedUnixNano {
		return contract.NewError(contract.ErrorPrecondition, "clock_regression", "supervision policy projection was checked in the future")
	}
	if now.UnixNano() >= policy.ExpiresUnixNano {
		return contract.NewError(contract.ErrorPrecondition, "system_ready_owner_current_expired", "supervision policy current expired")
	}
	for _, expected := range r.Components {
		reader, e := g.components.ReaderForComponentProductionCurrentV2(expected.Domain)
		if e != nil {
			return e
		}
		if contract.IsTypedNilV1(reader) {
			return contract.NewError(contract.ErrorUnavailable, "component_current_reader_missing", "component reader is nil")
		}
		actual, e := reader.InspectComponentProductionCurrentV2(ctx, expected)
		if e != nil {
			return e
		}
		if actual != expected {
			return contract.NewError(contract.ErrorConflict, "component_production_current_drift", "component production current drifted")
		}
		if now.UnixNano() >= componentMinimumExpiry(actual) {
			return contract.NewError(contract.ErrorPrecondition, "component_production_current_expired", "component production current expired")
		}
	}
	return nil
}
func componentMinimumExpiry(c contract.ComponentProductionCurrentV2) int64 {
	m := c.ReleaseCurrent.ExpiresUnixNano
	for _, v := range []int64{c.Binding.ExpiresUnixNano, c.GenerationCurrent.ExpiresUnixNano, c.ActivationCurrent.ExpiresUnixNano, c.ProductionCurrent.ExpiresUnixNano} {
		if v < m {
			m = v
		}
	}
	return m
}
func buildSystemReadyFactV2(r contract.SystemReadyEnsureRequestV2, now time.Time) (contract.SystemReadyFactV2, error) {
	expiry := r.Claim.ExpiresUnixNano
	for _, v := range []int64{r.Definition.ExpiresUnixNano, r.Plan.ExpiresUnixNano, r.Assembly.ExpiresUnixNano, r.BindingSet.ExpiresUnixNano, r.Activation.ExpiresUnixNano, r.GenerationBinding.ExpiresUnixNano, r.ApplicationStart.ExpiresUnixNano, r.SandboxLease.ExpiresUnixNano, r.SandboxActive.ExpiresUnixNano, r.ExecutionReady.ExpiresUnixNano, r.SupervisionPolicy.ExpiresUnixNano} {
		if v < expiry {
			expiry = v
		}
	}
	for _, c := range r.Components {
		if v := componentMinimumExpiry(c); v < expiry {
			expiry = v
		}
	}
	return contract.SealSystemReadyFactV2(contract.SystemReadyFactV2{HostID: r.HostID, StartID: r.StartID, HostStartClaim: r.Claim, DefinitionCurrent: r.Definition, PlanCurrent: r.Plan, AssemblyCurrent: r.Assembly, BindingSetCurrent: r.BindingSet, ActivationCurrent: r.Activation, GenerationBindingCurrent: r.GenerationBinding, ApplicationStartCurrent: r.ApplicationStart, SandboxLeaseCurrent: r.SandboxLease, SandboxActiveCurrent: r.SandboxActive, ExecutionReadyCurrent: r.ExecutionReady, SupervisionPolicyCurrent: r.SupervisionPolicy, Components: r.Components, MinimumReadyWindowNanos: r.MinimumReadyWindowNanos, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expiry, Ref: contract.SystemReadyFactRefV2{Revision: 1, ExpiresUnixNano: expiry}})
}

var _ ports.SystemReadyGovernancePortV2 = (*SystemReadyGatewayV2)(nil)

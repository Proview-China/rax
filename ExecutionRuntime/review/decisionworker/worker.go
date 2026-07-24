// Package decisionworker reconciles attested Review Cases into Verdict Owner
// decisions. It is read/decide only: it does not dispatch, commit or mutate any
// non-Review domain.
package decisionworker

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/verdictowner"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type StoreV1 interface {
	ListCasesV1(context.Context, reviewport.ListCasesRequestV1) (reviewport.ListCasesResultV1, error)
	ResolveDecisionCurrentRequestV1(context.Context, reviewport.DecisionCurrentResolveRequestV1) (reviewport.DecisionCurrentRequestV1, error)
}

type VerdictOwnerV1 interface {
	DecideV1(context.Context, verdictowner.DecideCommandV1) (contract.ReviewCaseV1, contract.VerdictV1, error)
}

type Worker struct {
	store  StoreV1
	owner  VerdictOwnerV1
	clock  func() time.Time
	source string
	epoch  core.Epoch
}

func New(store StoreV1, owner VerdictOwnerV1, clock func() time.Time) (*Worker, error) {
	if nilcheck.IsNil(store) || nilcheck.IsNil(owner) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review decision worker requires Store, Verdict Owner and clock")
	}
	return &Worker{store: store, owner: owner, clock: clock, source: "praxis.review/verdict-worker-v1", epoch: 1}, nil
}

type FailureV1 struct {
	CaseID string
	Err    error
}

type RunResultV1 struct {
	Inspected int
	Resolved  int
	Failures  []FailureV1
}

// RunOnceV1 scans one bounded tenant page. Each Case uses a deterministic
// Verdict/Trace identity; reply loss and competing workers therefore converge
// on VerdictOwner's exact Inspect/CAS recovery instead of creating a new Decide.
func (w *Worker) RunOnceV1(ctx context.Context, tenant core.TenantID, afterID string, limit int) (RunResultV1, string, error) {
	page, err := w.store.ListCasesV1(ctx, reviewport.ListCasesRequestV1{TenantID: tenant, States: []contract.CaseStateV1{contract.CaseAttestedV1, contract.CaseDecidingV1}, AfterID: afterID, Limit: limit})
	if err != nil {
		return RunResultV1{}, "", err
	}
	result := RunResultV1{Inspected: len(page.Cases)}
	for _, caseFact := range page.Cases {
		if ctx.Err() != nil {
			return result, page.NextAfterID, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Review decision worker context ended")
		}
		request, resolveErr := w.store.ResolveDecisionCurrentRequestV1(ctx, reviewport.DecisionCurrentResolveRequestV1{TenantID: tenant, CaseID: caseFact.ID, ExpectedCase: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest)})
		if resolveErr != nil {
			result.Failures = append(result.Failures, FailureV1{CaseID: caseFact.ID, Err: resolveErr})
			continue
		}
		command, commandErr := w.commandV1(caseFact, request.AttestationID)
		if commandErr != nil {
			result.Failures = append(result.Failures, FailureV1{CaseID: caseFact.ID, Err: commandErr})
			continue
		}
		if _, _, decideErr := w.owner.DecideV1(ctx, command); decideErr != nil {
			result.Failures = append(result.Failures, FailureV1{CaseID: caseFact.ID, Err: decideErr})
			continue
		}
		result.Resolved++
	}
	return result, page.NextAfterID, nil
}

func (w *Worker) commandV1(caseFact contract.ReviewCaseV1, attestationID string) (verdictowner.DecideCommandV1, error) {
	if err := caseFact.Validate(); err != nil {
		return verdictowner.DecideCommandV1{}, err
	}
	now := w.clock()
	if now.IsZero() || now.UnixNano() < caseFact.UpdatedUnixNano {
		return verdictowner.DecideCommandV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review decision worker clock regressed")
	}
	digest, err := core.CanonicalJSONDigest("praxis.review.verdict-worker", contract.ContractVersionV1, "DecisionWorkKeyV1", struct {
		TenantID      core.TenantID `json:"tenant_id"`
		CaseID        string        `json:"case_id"`
		CaseRevision  core.Revision `json:"case_revision"`
		CaseDigest    core.Digest   `json:"case_digest"`
		AttestationID string        `json:"attestation_id"`
	}{caseFact.TenantID, caseFact.ID, caseFact.Revision, caseFact.Digest, attestationID})
	if err != nil {
		return verdictowner.DecideCommandV1{}, err
	}
	suffix := strings.TrimPrefix(string(digest), "sha256:")
	verdictID := "verdict-" + suffix
	factRefs := []string{verdictID}
	trace, err := contract.SealTraceFactV1(contract.TraceFactV1{
		FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: caseFact.TenantID, ID: "trace-verdict-" + suffix, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		CaseID:         caseFact.ID, CaseRevision: caseFact.Revision, TargetID: caseFact.TargetID, TargetRevision: caseFact.TargetRevision, TargetDigest: caseFact.TargetDigest,
		Event: contract.TraceVerdictV1, SourceID: w.source, SourceEpoch: w.epoch, SourceSequence: uint64(caseFact.Revision), CausationID: verdictID, CorrelationID: caseFact.ID, FactRefs: factRefs,
	})
	if err != nil {
		return verdictowner.DecideCommandV1{}, err
	}
	resolvedRefs := []string{verdictID}
	resolved, err := contract.SealTraceFactV1(contract.TraceFactV1{
		FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: caseFact.TenantID, ID: "trace-resolved-" + suffix, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		CaseID:         caseFact.ID, CaseRevision: caseFact.Revision + 1, TargetID: caseFact.TargetID, TargetRevision: caseFact.TargetRevision, TargetDigest: caseFact.TargetDigest,
		Event: contract.TraceResolvedV1, SourceID: w.source, SourceEpoch: w.epoch, SourceSequence: uint64(caseFact.Revision) + 1, CausationID: verdictID, CorrelationID: caseFact.ID, FactRefs: resolvedRefs,
	})
	if err != nil {
		return verdictowner.DecideCommandV1{}, err
	}
	return verdictowner.DecideCommandV1{TenantID: caseFact.TenantID, CaseID: caseFact.ID, Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), AttestationID: attestationID, VerdictID: verdictID, Trace: trace, AdditionalTraces: []contract.TraceFactV1{resolved}}, nil
}

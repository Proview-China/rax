// Package production contains Review-owned composition roots.  The roots only
// accept public read/owner ports; they do not construct or impersonate facts
// owned by Runtime, Policy, Authority, Scope, Binding, or Evidence.
package production

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/decisioncurrent"
	"github.com/Proview-China/rax/ExecutionRuntime/review/decisionworker"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/verdictowner"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// DecisionPlaneDependenciesV1 is deliberately read-only outside Review.  The
// Store is the single Review Owner mutation boundary; all other dependencies
// are independently owned exact-current Readers.
type DecisionPlaneDependenciesV1 struct {
	Store     DecisionStoreV1
	Binding   runtimeports.ReviewBindingAuthoritativeCurrentReaderV1
	Evidence  runtimeports.ReviewEvidenceApplicabilityCurrentReaderV1
	Policy    runtimeports.ReviewDecisionPolicyCurrentReaderV1
	Authority runtimeports.ReviewDecisionAuthorityCurrentReaderV1
	Scope     runtimeports.ReviewDecisionScopeCurrentReaderV1
	Clock     func() time.Time
}

type DecisionStoreV1 interface {
	reviewport.StoreV1
	decisionworker.StoreV1
}

type DecisionPlaneV1 struct {
	External *decisioncurrent.ExternalSourceV1
	Current  *decisioncurrent.SourceV1
	Verdicts *verdictowner.Owner
	Worker   *decisionworker.Worker
}

// NewDecisionPlaneV1 closes the production Review decision chain:
//
//	Review facts + five Owner current cuts -> Verdict Owner CAS -> worker
//
// It creates no Runtime Authorization, Permit, Dispatch, or external Owner
// fact. Missing or typed-nil dependencies fail closed at construction.
func NewDecisionPlaneV1(d DecisionPlaneDependenciesV1) (*DecisionPlaneV1, error) {
	if nilcheck.IsNil(d.Store) || nilcheck.IsNil(d.Binding) || nilcheck.IsNil(d.Evidence) ||
		nilcheck.IsNil(d.Policy) || nilcheck.IsNil(d.Authority) || nilcheck.IsNil(d.Scope) || d.Clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "production Review decision plane requires Store, five Owner current Readers, and clock")
	}
	external, err := decisioncurrent.NewExternalSourceV1(d.Binding, d.Evidence, d.Policy, d.Authority, d.Scope, d.Clock)
	if err != nil {
		return nil, err
	}
	current, err := decisioncurrent.NewSourceV1(d.Store, external, d.Clock)
	if err != nil {
		return nil, err
	}
	verdicts, err := verdictowner.New(d.Store, current, d.Clock)
	if err != nil {
		return nil, err
	}
	worker, err := decisionworker.New(d.Store, verdicts, d.Clock)
	if err != nil {
		return nil, err
	}
	return &DecisionPlaneV1{External: external, Current: current, Verdicts: verdicts, Worker: worker}, nil
}

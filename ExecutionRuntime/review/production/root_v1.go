package production

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/autoattestation"
	"github.com/Proview-China/rax/ExecutionRuntime/review/autoreviewer"
	"github.com/Proview-China/rax/ExecutionRuntime/review/bypassowner"
	"github.com/Proview-China/rax/ExecutionRuntime/review/decisioncurrent"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	"github.com/Proview-China/rax/ExecutionRuntime/review/multisigowner"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/reviewer"
	"github.com/Proview-China/rax/ExecutionRuntime/review/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// StoreV1 is the one durable Review Owner boundary needed by the complete
// local root. It deliberately contains no Runtime or external Owner mutation
// port.
type StoreV1 interface {
	DecisionStoreV1
	service.StoreV1
	reviewport.StoreV2
	reviewport.TraceEventStoreV2
	reviewport.AutoReviewerStoreV1
	reviewport.BypassStoreV1
}

type RootDependenciesV1 struct {
	Store StoreV1
	Clock func() time.Time

	Binding          runtimeports.ReviewBindingAuthoritativeCurrentReaderV1
	Evidence         runtimeports.ReviewEvidenceApplicabilityCurrentReaderV1
	Policy           runtimeports.ReviewDecisionPolicyCurrentReaderV1
	Authority        runtimeports.ReviewDecisionAuthorityCurrentReaderV1
	Scope            runtimeports.ReviewDecisionScopeCurrentReaderV1
	AutoInvocation   reviewport.AutoReviewerInvocationPortV1
	ReviewerContext  reviewport.ReviewerContextCurrentReaderV1
	AutoOutputSchema reviewport.AutoReviewerOutputSchemaReaderV1

	HumanExternal     multisigowner.ExternalCurrentCutV2
	HumanOrganization reviewport.HumanOrganizationCurrentReaderV2

	BypassExternal bypassowner.ExternalCurrentCutV1

	// RuntimeV5 is the independently-owned read-only closure required to
	// expose accepted Human/conditional and policy-not-required currentness.
	RuntimeV5 decisioncurrent.HumanCurrentSourceDependenciesV5
}

type RootV1 struct {
	Service         *service.Service
	Decision        *DecisionPlaneV1
	AutoReviewer    *autoreviewer.Owner
	AutoAttestation *autoattestation.OwnerV1
	HumanOwner      *multisigowner.Owner
	HumanClaims     *multisigowner.ClaimOwnerV2
	HumanService    *service.HumanMultiSignServiceV2
	BypassOwner     *bypassowner.Owner
	RuntimeV5Source *decisioncurrent.CurrentFactSourceV5
	RuntimeV5Reader *runtimeadapter.ReaderV5
}

// NewRootV1 wires every Review-owned production route. The caller remains the
// trusted host composition root and must supply real public Owner readers and
// the governed Auto invocation port. No fake, fallback, or partial route is
// accepted.
func NewRootV1(d RootDependenciesV1) (*RootV1, error) {
	if nilcheck.IsNil(d.Store) || d.Clock == nil || nilcheck.IsNil(d.AutoInvocation) ||
		nilcheck.IsNil(d.ReviewerContext) || nilcheck.IsNil(d.AutoOutputSchema) ||
		nilcheck.IsNil(d.HumanExternal) || nilcheck.IsNil(d.HumanOrganization) ||
		nilcheck.IsNil(d.BypassExternal) || d.RuntimeV5.Bypass == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "production Review root requires every Review route and public Owner dependency")
	}

	base, err := service.New(d.Store, d.Clock)
	if err != nil {
		return nil, err
	}
	decision, err := NewDecisionPlaneV1(DecisionPlaneDependenciesV1{
		Store: d.Store, Binding: d.Binding, Evidence: d.Evidence,
		Policy: d.Policy, Authority: d.Authority, Scope: d.Scope, Clock: d.Clock,
	})
	if err != nil {
		return nil, err
	}
	auto, err := autoreviewer.NewProduction(d.Store, d.AutoInvocation, d.ReviewerContext, d.AutoOutputSchema, d.Clock)
	if err != nil {
		return nil, err
	}
	autoAttestation, err := autoattestation.NewV1(d.Store, d.Store, d.Clock)
	if err != nil {
		return nil, err
	}
	human, err := multisigowner.New(d.Store, d.HumanExternal, d.Clock)
	if err != nil {
		return nil, err
	}
	claims, err := multisigowner.NewClaimOwnerV2(d.Store, d.HumanOrganization, d.Clock)
	if err != nil {
		return nil, err
	}
	humanService, err := service.NewHumanMultiSignProductionV2(human, claims, d.Store)
	if err != nil {
		return nil, err
	}
	bypass, err := bypassowner.New(d.Store, d.BypassExternal, d.Clock)
	if err != nil {
		return nil, err
	}
	d.RuntimeV5.Facts = d.Store
	d.RuntimeV5.Bypass.Facts = d.Store
	// The complete Review root owns one monotonic clock domain. An injected
	// V5 clock is deliberately ignored so Source and Reader cannot validate
	// the same cut against different time bases.
	d.RuntimeV5.Clock = d.Clock
	sourceV5, err := decisioncurrent.NewCurrentFactSourceV5(d.RuntimeV5)
	if err != nil {
		return nil, err
	}
	readerV5, err := runtimeadapter.NewReaderV5(sourceV5, d.Clock)
	if err != nil {
		return nil, err
	}
	return &RootV1{
		Service: base, Decision: decision, AutoReviewer: auto,
		AutoAttestation: autoAttestation, HumanOwner: human, HumanClaims: claims,
		HumanService: humanService, BypassOwner: bypass,
		RuntimeV5Source: sourceV5, RuntimeV5Reader: readerV5,
	}, nil
}

// Compile-time contract guard: the built-in schema is a valid production
// dependency, but the root never silently chooses it for the host.
var _ reviewport.AutoReviewerOutputSchemaReaderV1 = (*reviewer.BuiltinOutputSchemaReaderV1)(nil)

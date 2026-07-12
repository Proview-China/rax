package routegateway

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

// HostConfig groups every host-owned dependency needed to construct one
// immutable Gateway snapshot. A nil ActivationPlan keeps the base catalog
// fail-closed and cannot activate subscription routes.
type HostConfig struct {
	BaseCatalog                       *catalog.Catalog
	ActivationPlan                    *catalog.ActivationPlan
	BindingResolver                   RuntimeBindingResolver
	SecretResolver                    SecretResolver
	SubscriptionAuthorizationResolver modelinvoker.SubscriptionAuthorizationResolver
	Factories                         *FactoryRegistry
	Clock                             func() time.Time
	HTTPClient                        *http.Client
}

// HostBuildReport is a secret-free projection of the candidate snapshot and
// the outcome of the host construction transaction.
type HostBuildReport struct {
	Activation                    catalog.ActivationReport
	Ready                         bool
	FailureCode                   string
	CallableRouteIDs              []upstream.RouteID
	ActivatedSubscriptionRouteIDs []upstream.RouteID
	DisabledRouteIDs              []upstream.RouteID
	AuditDigest                   string
}

// NewHost applies an optional exact activation plan, validates every callable
// factory and trust dependency, and only then returns a ready Gateway. It never
// mutates BaseCatalog and never returns a partially constructed Gateway.
func NewHost(config HostConfig) (*Gateway, HostBuildReport, error) {
	report := HostBuildReport{}
	if config.BaseCatalog == nil || nilInterface(config.BindingResolver) || nilInterface(config.SecretResolver) || config.Factories == nil {
		return hostBuildFailure(report, "host_config_required", gatewayError(modelinvoker.ErrorInvalidRequest, "host_config_required", "base catalog, binding resolver, secret resolver, and factory registry are required", nil))
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	now := clock()
	if now.IsZero() {
		return hostBuildFailure(report, "host_clock_invalid", gatewayError(modelinvoker.ErrorInvalidRequest, "host_clock_invalid", "host clock returned the zero time", nil))
	}

	candidate, err := catalog.New(config.BaseCatalog.Document(), now)
	if err != nil {
		return hostBuildFailure(report, "host_catalog_invalid", gatewayError(modelinvoker.ErrorPolicyRejected, "host_catalog_invalid", "base catalog is not valid at host construction time", err))
	}
	report.Activation.SchemaVersion = candidate.SchemaVersion()
	if config.ActivationPlan != nil {
		plan := config.ActivationPlan.Clone()
		candidate, report.Activation, err = catalog.ApplyActivationPlan(candidate, plan, now)
		if err != nil {
			return hostBuildFailure(report, "host_activation_failed", gatewayError(modelinvoker.ErrorPolicyRejected, "host_activation_failed", "catalog activation plan was rejected", err))
		}
	}

	activationIDs := make(map[upstream.RouteID]struct{})
	if report.Activation.Applied {
		for _, decision := range report.Activation.Decisions {
			switch decision.Action {
			case catalog.ActivateHostBlockedRoute:
				activationIDs[decision.RouteID] = struct{}{}
			case catalog.DisableRoute:
				if decision.Code == catalog.ActivationCodeDisabled {
					report.DisabledRouteIDs = append(report.DisabledRouteIDs, decision.RouteID)
				}
			}
		}
	}

	entries := candidate.Entries()
	for _, entry := range entries {
		if !entry.Implementation.Callable {
			continue
		}
		report.CallableRouteIDs = append(report.CallableRouteIDs, entry.ID)
		if isSubscription(entry) {
			report.ActivatedSubscriptionRouteIDs = append(report.ActivatedSubscriptionRouteIDs, entry.ID)
		}
	}
	sort.Slice(report.CallableRouteIDs, func(i, j int) bool { return report.CallableRouteIDs[i] < report.CallableRouteIDs[j] })
	sort.Slice(report.ActivatedSubscriptionRouteIDs, func(i, j int) bool {
		return report.ActivatedSubscriptionRouteIDs[i] < report.ActivatedSubscriptionRouteIDs[j]
	})
	sort.Slice(report.DisabledRouteIDs, func(i, j int) bool { return report.DisabledRouteIDs[i] < report.DisabledRouteIDs[j] })

	for _, entry := range entries {
		if !entry.Implementation.Callable {
			continue
		}
		if _, factoryErr := config.Factories.Get(modelinvoker.ProviderID(entry.Implementation.AdapterID)); factoryErr != nil {
			return hostBuildFailure(report, "host_callable_factory_missing", gatewayError(modelinvoker.ErrorUnknownProvider, "host_callable_factory_missing", "a callable catalog route has no adapter factory", nil))
		}
		if !isSubscription(entry) {
			continue
		}
		if _, audited := activationIDs[entry.ID]; !audited {
			return hostBuildFailure(report, "host_activation_plan_required", gatewayError(modelinvoker.ErrorPolicyRejected, "host_activation_plan_required", "every callable subscription route must be activated by the current exact plan", nil))
		}
	}
	if len(report.ActivatedSubscriptionRouteIDs) > 0 && nilInterface(config.SubscriptionAuthorizationResolver) {
		return hostBuildFailure(report, "host_subscription_authorization_required", gatewayError(modelinvoker.ErrorInvalidRequest, "host_subscription_authorization_required", "activated subscription routes require a trusted host authorization resolver", nil))
	}

	options := []Option{WithClock(clock)}
	if config.HTTPClient != nil {
		options = append(options, WithHTTPClient(config.HTTPClient))
	}
	if !nilInterface(config.SubscriptionAuthorizationResolver) {
		options = append(options, WithSubscriptionAuthorizationResolver(config.SubscriptionAuthorizationResolver))
	}
	gateway, err := New(candidate, config.BindingResolver, config.SecretResolver, config.Factories, options...)
	if err != nil {
		return hostBuildFailure(report, "host_gateway_build_failed", err)
	}
	report.Ready = true
	finalizeHostBuildReport(&report)
	return gateway, report, nil
}

func hostBuildFailure(report HostBuildReport, code string, err error) (*Gateway, HostBuildReport, error) {
	report.Ready = false
	report.FailureCode = code
	finalizeHostBuildReport(&report)
	return nil, report, err
}

func finalizeHostBuildReport(report *HostBuildReport) {
	if report == nil {
		return
	}
	payload := struct {
		Activation                    catalog.ActivationReport `json:"activation"`
		Ready                         bool                     `json:"ready"`
		FailureCode                   string                   `json:"failure_code,omitempty"`
		CallableRouteIDs              []upstream.RouteID       `json:"callable_route_ids"`
		ActivatedSubscriptionRouteIDs []upstream.RouteID       `json:"activated_subscription_route_ids"`
		DisabledRouteIDs              []upstream.RouteID       `json:"disabled_route_ids"`
	}{
		Activation: report.Activation, Ready: report.Ready, FailureCode: report.FailureCode,
		CallableRouteIDs: report.CallableRouteIDs, ActivatedSubscriptionRouteIDs: report.ActivatedSubscriptionRouteIDs,
		DisabledRouteIDs: report.DisabledRouteIDs,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic("routegateway: marshal host build report")
	}
	digest := sha256.Sum256(encoded)
	report.AuditDigest = fmt.Sprintf("sha256:%x", digest[:])
}

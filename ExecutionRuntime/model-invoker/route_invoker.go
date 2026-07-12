package modelinvoker

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

const (
	// SemanticPrimitivesCandidateVersion identifies the provider-neutral v1
	// candidate. It remains subject to joint consumer review before v1 freeze.
	SemanticPrimitivesCandidateVersion = "praxis.model-invoker.semantic/v1candidate"
	// RoutePolicyCandidateVersion identifies the RouteID policy/audit contract.
	// Adapter construction and lifecycle belong to the separate route gateway.
	RoutePolicyCandidateVersion = "praxis.model-invoker.route-policy/v1candidate"

	// SemanticPrimitivesVersion is retained as a compatibility alias while the
	// original uncommitted candidate is migrated.
	SemanticPrimitivesVersion = SemanticPrimitivesCandidateVersion
	// RouteFacadeVersion is retained as a compatibility alias. RouteInvoker is
	// a policy/audit invoker, not the complete adapter-construction gateway.
	RouteFacadeVersion = RoutePolicyCandidateVersion
)

// RouteCall combines a provider-neutral semantic request with the explicit
// execution context required to authorize one catalog route.
// Provider, Protocol and Endpoint in Request must be left empty: the facade
// owns those selectors and derives them exclusively from RouteID. Invocation
// and EntitlementState are caller claims and are rejected for subscription
// routes; trusted subscription claims come only from the injected resolver.
type RouteCall struct {
	RouteID          upstream.RouteID
	Invocation       upstream.InvocationContext
	EntitlementState *upstream.EntitlementState
	Request          Request
}

// SubscriptionAuthorizationRequest contains only catalog-anchored identities.
// It deliberately contains no caller-provided identity or entitlement claim.
type SubscriptionAuthorizationRequest struct {
	RouteID           upstream.RouteID
	Identity          upstream.RouteIdentity
	OfferingID        upstream.OfferingID
	CredentialProfile upstream.CredentialProfileID
}

// SubscriptionAuthorization is the trusted claim set returned by a Gateway
// host boundary. Structural, freshness, binding and policy checks still run in
// RouteInvoker after resolution.
type SubscriptionAuthorization struct {
	Invocation  upstream.InvocationContext
	Entitlement upstream.EntitlementState
}

// SubscriptionAuthorizationResolver is a host-injected trust boundary. A
// normal RouteCall cannot implement trust by choosing enum or timestamp values.
type SubscriptionAuthorizationResolver interface {
	ResolveSubscriptionAuthorization(context.Context, SubscriptionAuthorizationRequest) (SubscriptionAuthorization, error)
}

// RouteSelection is the immutable audit projection attached to every routed
// success and to every error that occurs after a route has been selected.
type RouteSelection struct {
	RouteID        upstream.RouteID
	Identity       upstream.RouteIdentity
	EvidenceDigest string
	AdapterID      ProviderID
	Protocol       Protocol
	Endpoint       string
	Model          string
	Policy         upstream.PolicyDecision
	ClientIdentity upstream.ClientIdentity
}

// Clone returns a defensive copy of the route selection.
func (selection RouteSelection) Clone() RouteSelection {
	clone := selection
	clone.Policy = selection.Policy.Clone()
	return clone
}

// RouteResponse keeps the provider-neutral response and its catalog selection
// together without changing the frozen Response primitive.
type RouteResponse struct {
	Response Response
	Route    RouteSelection
}

// RouteError preserves RouteID and, when available, the selected route while
// retaining the underlying provider-neutral error through errors.Unwrap.
type RouteError struct {
	RouteID upstream.RouteID
	Route   RouteSelection
	Err     error
}

func (e *RouteError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	if e.RouteID != "" {
		return fmt.Sprintf("route %q failed", e.RouteID)
	}
	return "route invocation failed"
}

func (e *RouteError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type RouteInvokerOption func(*RouteInvoker) error

// WithRouteClock supplies the clock used for evidence and entitlement checks.
// It exists for deterministic tests and account-local runtime time sources.
func WithRouteClock(clock func() time.Time) RouteInvokerOption {
	return func(invoker *RouteInvoker) error {
		if clock == nil {
			return fmt.Errorf("route clock must not be nil")
		}
		invoker.now = clock
		return nil
	}
}

func WithSubscriptionAuthorizationResolver(resolver SubscriptionAuthorizationResolver) RouteInvokerOption {
	return func(invoker *RouteInvoker) error {
		if nilSubscriptionResolver(resolver) {
			return fmt.Errorf("subscription authorization resolver must not be nil")
		}
		invoker.subscriptionAuthorization = resolver
		return nil
	}
}

// RouteInvoker is the RouteID policy, authorization, selection and audit layer.
// It composes an immutable catalog with a preconstructed provider-neutral
// Invoker. It intentionally does not resolve secrets, construct adapters or own
// adapter lifecycle; those responsibilities belong to routegateway.
type RouteInvoker struct {
	catalog                   *catalog.Catalog
	invoker                   *Invoker
	now                       func() time.Time
	subscriptionAuthorization SubscriptionAuthorizationResolver
}

// RoutePolicyInvoker names the actual responsibility of RouteInvoker while
// preserving the original public type during the candidate migration.
type RoutePolicyInvoker = RouteInvoker

func NewRouteInvoker(routeCatalog *catalog.Catalog, invoker *Invoker, options ...RouteInvokerOption) (*RouteInvoker, error) {
	if routeCatalog == nil {
		return nil, &Error{Kind: ErrorInvalidRequest, Operation: "new_route_invoker", Code: "catalog_required", Message: "route catalog is required"}
	}
	if invoker == nil || invoker.registry == nil {
		return nil, &Error{Kind: ErrorInvalidRequest, Operation: "new_route_invoker", Code: "invoker_required", Message: "initialized invoker is required"}
	}
	routed := &RouteInvoker{catalog: routeCatalog, invoker: invoker, now: time.Now}
	for _, option := range options {
		if option == nil {
			return nil, &Error{Kind: ErrorInvalidRequest, Operation: "new_route_invoker", Code: "option_nil", Message: "route invoker option is nil"}
		}
		if err := option(routed); err != nil {
			return nil, &Error{Kind: ErrorInvalidRequest, Operation: "new_route_invoker", Code: "option_invalid", Message: err.Error(), Err: err}
		}
	}
	return routed, nil
}

// NewRoutePolicyInvoker is the responsibility-accurate constructor alias.
func NewRoutePolicyInvoker(routeCatalog *catalog.Catalog, invoker *Invoker, options ...RouteInvokerOption) (*RoutePolicyInvoker, error) {
	return NewRouteInvoker(routeCatalog, invoker, options...)
}

// Resolve performs the complete offline route and policy preflight without
// invoking Provider.Capabilities, Provider.Invoke or Provider.Stream.
func (i *RouteInvoker) Resolve(call RouteCall) (RouteSelection, error) {
	return i.ResolveContext(context.Background(), call)
}

// ResolveContext performs preflight with a context available to an injected
// subscription authorization resolver.
func (i *RouteInvoker) ResolveContext(ctx context.Context, call RouteCall) (RouteSelection, error) {
	selection, _, err := i.prepare(ctx, call)
	if err != nil {
		return RouteSelection{}, err
	}
	return selection.Clone(), nil
}

func (i *RouteInvoker) Invoke(ctx context.Context, call RouteCall) (RouteResponse, error) {
	selection, request, err := i.prepare(ctx, call)
	if err != nil {
		return RouteResponse{}, err
	}
	response, err := i.invoker.Invoke(ctx, request)
	if err != nil {
		return RouteResponse{Route: selection.Clone()}, wrapRouteError(call.RouteID, selection, err)
	}
	return RouteResponse{Response: response, Route: selection.Clone()}, nil
}

func (i *RouteInvoker) Stream(ctx context.Context, call RouteCall) (*RoutedStream, error) {
	selection, request, err := i.prepare(ctx, call)
	if err != nil {
		return nil, err
	}
	stream, err := i.invoker.Stream(ctx, request)
	if err != nil {
		return nil, wrapRouteError(call.RouteID, selection, err)
	}
	return &RoutedStream{inner: stream, route: selection.Clone()}, nil
}

func (i *RouteInvoker) prepare(ctx context.Context, call RouteCall) (RouteSelection, Request, error) {
	if i == nil || i.catalog == nil || i.invoker == nil || i.invoker.registry == nil || i.now == nil {
		return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, RouteSelection{}, &Error{
			Kind: ErrorInvalidRequest, Operation: "resolve_route", Code: "route_invoker_uninitialized", Message: "route invoker is not initialized",
		})
	}
	if strings.TrimSpace(string(call.RouteID)) == "" {
		return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, RouteSelection{}, &Error{
			Kind: ErrorInvalidRequest, Operation: "resolve_route", Code: "route_id_required", Message: "route ID is required",
		})
	}
	if ctx == nil {
		return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, RouteSelection{}, &Error{
			Kind: ErrorInvalidRequest, Operation: "resolve_route", Code: "context_required", Message: "context is required",
		})
	}
	if err := ctx.Err(); err != nil {
		kind := ErrorCancelled
		if err == context.DeadlineExceeded {
			kind = ErrorTimeout
		}
		return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, RouteSelection{}, &Error{
			Kind: kind, Operation: "resolve_route", Code: "context_done", Message: "route preflight context is done", Err: err,
		})
	}
	entry, found := i.catalog.Get(call.RouteID)
	if !found {
		return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, RouteSelection{}, &Error{
			Kind: ErrorPolicyRejected, Operation: "resolve_route", Code: "route_not_found", Message: "route is not present in the active catalog",
		})
	}
	if call.Request.Provider != "" || call.Request.Protocol != ProtocolAuto || strings.TrimSpace(call.Request.Endpoint) != "" {
		return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, RouteSelection{}, &Error{
			Kind: ErrorInvalidRequest, Operation: "resolve_route", Code: "route_selector_owned", Message: "provider, protocol, and endpoint are owned by RouteID and must not be supplied by the caller",
		})
	}
	if !entry.Implementation.Callable || entry.Implementation.AdapterID == "" || !callableImplementationStatus(entry.Implementation.Status) {
		return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, RouteSelection{}, &Error{
			Kind: ErrorPolicyRejected, Operation: "resolve_route", Code: "route_not_callable", Message: "catalog route is a control record and cannot be invoked",
		})
	}

	now := i.now()
	if now.IsZero() || entry.Evidence.Status != catalog.EvidenceFresh || now.Before(entry.Evidence.CheckedAt) || !now.Before(entry.Evidence.ValidUntil) {
		return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, RouteSelection{}, &Error{
			Kind: ErrorPolicyRejected, Operation: "resolve_route", Code: "route_evidence_unavailable", Message: "route evidence is not fresh at invocation time",
		})
	}

	protocol, ok := runtimeProtocol(entry.Route.Protocol.ID)
	if !ok {
		return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, RouteSelection{}, &Error{
			Kind: ErrorInvalidRequest, Operation: "resolve_route", Code: "route_protocol_unsupported", Message: "catalog protocol has no v1 runtime mapping",
		})
	}
	endpoint, err := entry.Route.Endpoint.ResolveBaseURL(entry.Route.Deployment)
	if err != nil {
		return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, RouteSelection{}, &Error{
			Kind: ErrorInvalidRequest, Operation: "resolve_route", Code: "route_endpoint_invalid", Message: "catalog endpoint cannot be resolved", Err: err,
		})
	}
	adapterID := ProviderID(entry.Implementation.AdapterID)
	selection := RouteSelection{
		RouteID: call.RouteID, Identity: entry.Route.Identity(), EvidenceDigest: entry.Evidence.Digest,
		AdapterID: adapterID, Protocol: protocol, Endpoint: endpoint, Model: call.Request.Model,
	}

	if err := validateRouteModel(entry, call.Request.Model); err != nil {
		return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, selection, &Error{
			Kind: ErrorInvalidRequest, Provider: adapterID, Operation: "resolve_route", Code: "route_model_rejected", Message: err.Error(),
		})
	}
	invocation := call.Invocation
	entitlement := call.EntitlementState
	if subscriptionOffering(entry.Route.Offering.Kind) {
		if call.Invocation != (upstream.InvocationContext{}) || call.EntitlementState != nil {
			return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, selection, &Error{
				Kind: ErrorPolicyRejected, Provider: adapterID, Operation: "authorize_route", Code: "untrusted_subscription_claims", Message: "subscription identity and entitlement claims must come from the trusted host resolver",
			})
		}
		if nilSubscriptionResolver(i.subscriptionAuthorization) {
			return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, selection, &Error{
				Kind: ErrorPolicyRejected, Provider: adapterID, Operation: "authorize_route", Code: "subscription_authorization_resolver_required", Message: "subscription route requires a trusted host authorization resolver",
			})
		}
		trusted, resolveErr := i.subscriptionAuthorization.ResolveSubscriptionAuthorization(ctx, SubscriptionAuthorizationRequest{
			RouteID: entry.ID, Identity: entry.Route.Identity(), OfferingID: entry.Route.Offering.ID, CredentialProfile: entry.Route.Credential.ID,
		})
		if resolveErr != nil {
			return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, selection, &Error{
				Kind: ErrorPolicyRejected, Provider: adapterID, Operation: "authorize_route", Code: "subscription_authorization_resolution_failed", Message: "trusted subscription authorization resolution failed",
			})
		}
		invocation = trusted.Invocation
		entitlement = &trusted.Entitlement
	}
	decision := entry.Route.Authorize(invocation, entitlement, now)
	selection.Policy = decision.Clone()
	selection.ClientIdentity = invocation.ClientIdentity
	if !decision.Allowed {
		code, message := "route_policy_rejected", "route offering policy rejected the invocation"
		if decision.Code != "" {
			code = string(decision.Code)
		}
		if len(decision.Reasons) > 0 && decision.Reasons[0].Message != "" {
			message = decision.Reasons[0].Message
		}
		return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, selection, &Error{
			Kind: ErrorPolicyRejected, Provider: adapterID, Operation: "authorize_route", Code: code, Message: message,
		})
	}
	if decision.AllowsAutomaticPAYGSwitch {
		return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, selection, &Error{
			Kind: ErrorPolicyRejected, Provider: adapterID, Operation: "authorize_route", Code: "automatic_payg_forbidden", Message: "automatic pay-as-you-go fallback is forbidden",
		})
	}
	if _, err := i.invoker.registry.Get(adapterID); err != nil {
		return RouteSelection{}, Request{}, wrapRouteError(call.RouteID, selection, err)
	}

	request := call.Request
	request.Provider = adapterID
	request.Protocol = protocol
	request.Endpoint = endpoint
	return selection, request, nil
}

func subscriptionOffering(kind upstream.OfferingKind) bool {
	return kind == upstream.OfferingTokenPlan || kind == upstream.OfferingCodingPlan
}

func nilSubscriptionResolver(resolver SubscriptionAuthorizationResolver) bool {
	if resolver == nil {
		return true
	}
	value := reflect.ValueOf(resolver)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

func callableImplementationStatus(status catalog.ImplementationStatus) bool {
	switch status {
	case catalog.ImplementationImplementedOffline, catalog.ImplementationLiveVerified, catalog.ImplementationProductionApproved:
		return true
	default:
		return false
	}
}

func runtimeProtocol(protocol upstream.ProtocolID) (Protocol, bool) {
	switch protocol {
	case upstream.ProtocolResponses:
		return ProtocolResponses, true
	case upstream.ProtocolChatCompletions:
		return ProtocolChatCompletions, true
	case upstream.ProtocolMessages:
		return ProtocolMessages, true
	case upstream.ProtocolGenerateContent:
		return ProtocolGenerateContent, true
	case upstream.ProtocolBedrockConverse:
		return ProtocolBedrockConverse, true
	case upstream.ProtocolBedrockInvoke:
		return ProtocolBedrockInvoke, true
	default:
		return ProtocolAuto, false
	}
}

func validateRouteModel(entry catalog.Entry, model string) error {
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("model is required")
	}
	if entry.ModelDiscovery.Method != catalog.ModelDiscoveryStaticCatalog {
		return nil
	}
	if model == entry.Route.Model.ProviderModelRef {
		return nil
	}
	for _, alias := range entry.ModelDiscovery.Aliases {
		if model == alias.ProviderModelRef || entry.ModelDiscovery.AliasPolicy != catalog.ModelAliasExactProviderID && model == alias.Alias {
			return nil
		}
	}
	return fmt.Errorf("model is outside the selected route's static catalog")
}

func wrapRouteError(routeID upstream.RouteID, selection RouteSelection, err error) error {
	if err == nil {
		return nil
	}
	return &RouteError{RouteID: routeID, Route: selection.Clone(), Err: err}
}

// RoutedStream is a provider-neutral stream with an immutable RouteID audit
// selection. It does not alter the existing Stream or StreamEvent primitives.
type RoutedStream struct {
	inner Stream
	route RouteSelection
}

func (stream *RoutedStream) Route() RouteSelection {
	if stream == nil {
		return RouteSelection{}
	}
	return stream.route.Clone()
}

func (stream *RoutedStream) Next() bool {
	return stream != nil && stream.inner != nil && stream.inner.Next()
}

func (stream *RoutedStream) Event() StreamEvent {
	if stream == nil || stream.inner == nil {
		return StreamEvent{}
	}
	return stream.inner.Event()
}

func (stream *RoutedStream) Err() error {
	if stream == nil || stream.inner == nil {
		return nil
	}
	return wrapRouteError(stream.route.RouteID, stream.route, stream.inner.Err())
}

func (stream *RoutedStream) Close() error {
	if stream == nil || stream.inner == nil {
		return nil
	}
	return wrapRouteError(stream.route.RouteID, stream.route, stream.inner.Close())
}

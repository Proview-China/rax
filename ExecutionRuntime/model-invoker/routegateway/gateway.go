package routegateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

type Option func(*Gateway) error

func WithClock(clock func() time.Time) Option {
	return func(gateway *Gateway) error {
		if clock == nil {
			return gatewayError(modelinvoker.ErrorInvalidRequest, "clock_nil", "gateway clock is required", nil)
		}
		gateway.now = clock
		return nil
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(gateway *Gateway) error {
		if client == nil {
			return gatewayError(modelinvoker.ErrorInvalidRequest, "http_client_nil", "HTTP client is required", nil)
		}
		gateway.httpClient = client
		return nil
	}
}

func WithSubscriptionAuthorizationResolver(resolver modelinvoker.SubscriptionAuthorizationResolver) Option {
	return func(gateway *Gateway) error {
		if nilInterface(resolver) {
			return gatewayError(modelinvoker.ErrorInvalidRequest, "subscription_authorization_resolver_nil", "subscription authorization resolver is required", nil)
		}
		gateway.subscriptionAuthorization = resolver
		return nil
	}
}

// WithGovernedModelInvocationsV1 enables the additive provider-neutral
// governed execution surface. Legacy Invoke/Stream remain available but do not
// gain governed or Auto Reviewer production semantics from this option.
func WithGovernedModelInvocationsV1(dependencies GovernedModelInvocationDependenciesV1) Option {
	return func(gateway *Gateway) error {
		if err := dependencies.validate(); err != nil {
			return err
		}
		copy := dependencies
		gateway.governedV1 = &copy
		return nil
	}
}

type Gateway struct {
	catalog                   *catalog.Catalog
	policy                    *modelinvoker.RouteInvoker
	bindings                  RuntimeBindingResolver
	secrets                   SecretResolver
	factories                 *FactoryRegistry
	pool                      *adapterPool
	now                       func() time.Time
	httpClient                *http.Client
	subscriptionAuthorization modelinvoker.SubscriptionAuthorizationResolver
	governedV1                *GovernedModelInvocationDependenciesV1
	closeOnce                 sync.Once
	closeErr                  error
}

func New(routeCatalog *catalog.Catalog, bindings RuntimeBindingResolver, secrets SecretResolver, factories *FactoryRegistry, options ...Option) (*Gateway, error) {
	if routeCatalog == nil || nilInterface(bindings) || nilInterface(secrets) || factories == nil {
		return nil, gatewayError(modelinvoker.ErrorInvalidRequest, "gateway_dependency_required", "catalog, binding resolver, secret resolver, and factory registry are required", nil)
	}
	gateway := &Gateway{catalog: routeCatalog, bindings: bindings, secrets: secrets, factories: factories, pool: newAdapterPool(), now: time.Now}
	for _, option := range options {
		if option == nil {
			return nil, gatewayError(modelinvoker.ErrorInvalidRequest, "option_nil", "gateway option is nil", nil)
		}
		if err := option(gateway); err != nil {
			return nil, err
		}
	}

	providers := make([]modelinvoker.Provider, 0, len(factories.IDs()))
	for _, adapterID := range factories.IDs() {
		providers = append(providers, policyProvider{id: adapterID})
	}
	registry, err := modelinvoker.NewRegistry(providers...)
	if err != nil {
		return nil, gatewayError(modelinvoker.ErrorInvalidRequest, "policy_registry_invalid", "failed to initialize policy registry", err)
	}
	invoker, err := modelinvoker.NewInvoker(registry)
	if err != nil {
		return nil, gatewayError(modelinvoker.ErrorInvalidRequest, "policy_invoker_invalid", "failed to initialize policy invoker", err)
	}
	policyOptions := []modelinvoker.RouteInvokerOption{modelinvoker.WithRouteClock(gateway.now)}
	if !nilInterface(gateway.subscriptionAuthorization) {
		policyOptions = append(policyOptions, modelinvoker.WithSubscriptionAuthorizationResolver(gateway.subscriptionAuthorization))
	}
	gateway.policy, err = modelinvoker.NewRoutePolicyInvoker(routeCatalog, invoker, policyOptions...)
	if err != nil {
		return nil, gatewayError(modelinvoker.ErrorInvalidRequest, "policy_invoker_invalid", "failed to initialize route policy", err)
	}
	for _, entry := range routeCatalog.Entries() {
		if !entry.Implementation.Callable {
			continue
		}
		if _, err := factories.Get(modelinvoker.ProviderID(entry.Implementation.AdapterID)); err != nil {
			return nil, gatewayError(modelinvoker.ErrorUnknownProvider, "callable_factory_missing", "a callable catalog Route has no adapter factory", nil)
		}
		if isSubscription(entry) && nilInterface(gateway.subscriptionAuthorization) {
			return nil, gatewayError(modelinvoker.ErrorInvalidRequest, "subscription_authorization_resolver_required", "callable subscription routes require a trusted host authorization resolver", nil)
		}
	}
	return gateway, nil
}

func (g *Gateway) Resolve(ctx context.Context, call modelinvoker.RouteCall) (Resolution, error) {
	prepared, err := g.prepare(ctx, call)
	if err != nil {
		return Resolution{}, err
	}
	releaseErr := prepared.lease.release()
	if releaseErr != nil {
		return prepared.resolution, wrapRoute(prepared.resolution.Route, releaseErr)
	}
	return prepared.resolution, nil
}

func (g *Gateway) Capabilities(ctx context.Context, call modelinvoker.RouteCall) (CapabilityResult, error) {
	prepared, err := g.prepare(ctx, call)
	if err != nil {
		return CapabilityResult{}, err
	}
	contract, callErr := prepared.lease.provider.Capabilities(ctx, modelinvoker.CapabilityQuery{
		Protocol: prepared.resolution.Route.Protocol, Endpoint: prepared.resolution.Route.Endpoint, Model: prepared.resolution.Route.Model,
	})
	releaseErr := prepared.lease.release()
	if callErr != nil {
		return CapabilityResult{Resolution: prepared.resolution}, wrapRoute(prepared.resolution.Route, errors.Join(callErr, releaseErr))
	}
	if releaseErr != nil {
		return CapabilityResult{Resolution: prepared.resolution, Contract: contract}, wrapRoute(prepared.resolution.Route, releaseErr)
	}
	return CapabilityResult{Resolution: prepared.resolution, Contract: contract}, nil
}

func (g *Gateway) Invoke(ctx context.Context, call modelinvoker.RouteCall) (InvokeResult, error) {
	prepared, err := g.prepare(ctx, call)
	if err != nil {
		return InvokeResult{}, err
	}
	return g.invokePrepared(ctx, prepared)
}

func (g *Gateway) invokePrepared(ctx context.Context, prepared preparedCall) (InvokeResult, error) {
	registry, err := modelinvoker.NewRegistry(prepared.lease.provider)
	if err != nil {
		releaseErr := prepared.lease.release()
		return InvokeResult{Resolution: prepared.resolution}, wrapRoute(prepared.resolution.Route, errors.Join(err, releaseErr))
	}
	invoker, err := modelinvoker.NewInvoker(registry)
	if err != nil {
		releaseErr := prepared.lease.release()
		return InvokeResult{Resolution: prepared.resolution}, wrapRoute(prepared.resolution.Route, errors.Join(err, releaseErr))
	}
	response, callErr := invoker.Invoke(ctx, prepared.request)
	if callErr == nil {
		if modelErr := responseModelError(prepared.request.Model, response.Model); modelErr != nil {
			callErr = modelErr
			response = modelinvoker.Response{}
		} else {
			response = stampGatewayResponse(prepared.resolution, response)
		}
	} else if isResponseModelError(callErr) {
		response = modelinvoker.Response{}
	}
	releaseErr := prepared.lease.release()
	result := InvokeResult{Resolution: prepared.resolution, Response: response}
	if callErr != nil {
		return result, wrapRoute(prepared.resolution.Route, errors.Join(callErr, releaseErr))
	}
	if releaseErr != nil {
		return result, wrapRoute(prepared.resolution.Route, releaseErr)
	}
	return result, nil
}

func (g *Gateway) Stream(ctx context.Context, call modelinvoker.RouteCall) (*Stream, error) {
	prepared, err := g.prepare(ctx, call)
	if err != nil {
		return nil, err
	}
	registry, err := modelinvoker.NewRegistry(prepared.lease.provider)
	if err != nil {
		releaseErr := prepared.lease.release()
		return nil, wrapRoute(prepared.resolution.Route, errors.Join(err, releaseErr))
	}
	invoker, err := modelinvoker.NewInvoker(registry)
	if err != nil {
		releaseErr := prepared.lease.release()
		return nil, wrapRoute(prepared.resolution.Route, errors.Join(err, releaseErr))
	}
	inner, err := invoker.Stream(ctx, prepared.request)
	if err != nil {
		releaseErr := prepared.lease.release()
		return nil, wrapRoute(prepared.resolution.Route, errors.Join(err, releaseErr))
	}
	return &Stream{inner: inner, resolution: prepared.resolution, lease: prepared.lease, requestedModel: prepared.request.Model}, nil
}

func (g *Gateway) Close() error {
	if g == nil {
		return nil
	}
	g.closeOnce.Do(func() { g.closeErr = g.pool.close() })
	return g.closeErr
}

type preparedCall struct {
	resolution Resolution
	request    modelinvoker.Request
	lease      *adapterLease
}

func (g *Gateway) prepare(ctx context.Context, call modelinvoker.RouteCall) (preparedCall, error) {
	if g == nil || g.policy == nil || g.catalog == nil || g.pool == nil || g.now == nil {
		return preparedCall{}, gatewayError(modelinvoker.ErrorInvalidRequest, "gateway_uninitialized", "route gateway is not initialized", nil)
	}
	return g.prepareAt(ctx, call, g.now())
}

func (g *Gateway) prepareAt(ctx context.Context, call modelinvoker.RouteCall, now time.Time) (preparedCall, error) {
	if g == nil || g.policy == nil || g.catalog == nil || g.pool == nil || now.IsZero() {
		return preparedCall{}, gatewayError(modelinvoker.ErrorInvalidRequest, "gateway_uninitialized", "route gateway or preparation clock is not initialized", nil)
	}
	if ctx == nil {
		return preparedCall{}, gatewayError(modelinvoker.ErrorInvalidRequest, "context_nil", "context is required", nil)
	}
	if err := ctx.Err(); err != nil {
		return preparedCall{}, gatewayError(contextKind(err), "context_done", "route gateway context is already done", err)
	}
	selection, err := g.policy.ResolveContext(ctx, call)
	if err != nil {
		return preparedCall{}, err
	}
	entry, ok := g.catalog.Get(call.RouteID)
	if !ok {
		return preparedCall{}, wrapRoute(selection, gatewayError(modelinvoker.ErrorPolicyRejected, "route_not_found", "route is not present in the active catalog", nil))
	}

	binding, err := g.bindings.ResolveBinding(ctx, BindingRequest{Entry: entry})
	if err != nil {
		return preparedCall{}, wrapRoute(selection, gatewayError(modelinvoker.ErrorMapping, "binding_resolution_failed", "runtime binding resolution failed", nil))
	}
	endpoint, err := validateBinding(entry, binding)
	if err != nil {
		return preparedCall{}, wrapRoute(selection, err)
	}
	selection.Endpoint = endpoint

	material, err := g.secrets.ResolveSecret(ctx, SecretRequest{RouteID: entry.ID, Identity: entry.Route.Identity(), Profile: entry.Route.Credential})
	if err != nil {
		return preparedCall{}, wrapRoute(selection, gatewayError(modelinvoker.ErrorAuthentication, "secret_resolution_failed", "credential resolution failed", nil))
	}
	defer material.zero()
	if err := validateSecretMaterial(entry.Route.Credential, material, now); err != nil {
		return preparedCall{}, wrapRoute(selection, err)
	}
	factory, err := g.factories.Get(selection.AdapterID)
	if err != nil {
		return preparedCall{}, wrapRoute(selection, err)
	}
	routeDigest, err := selection.Identity.Digest()
	if err != nil {
		return preparedCall{}, wrapRoute(selection, gatewayError(modelinvoker.ErrorMapping, "route_identity_invalid", "route identity digest failed", err))
	}
	trustedIdentity := selection.ClientIdentity
	identityKey := string(trustedIdentity.Source) + "\x00" + trustedIdentity.Name + "\x00" + trustedIdentity.Version + "\x00" + trustedIdentity.UserAgent
	key := poolKey{routeDigest: routeDigest, evidence: selection.EvidenceDigest, credentialVersion: material.Version, bindingVersion: binding.Version, factoryID: factory.ID(), factoryVersion: factory.Version(), clientIdentity: identityKey}
	lease, err := g.pool.acquire(ctx, key, entry.ID, func(buildContext context.Context) (FactoryResult, error) {
		result, buildErr := factory.Build(buildContext, FactoryInput{Entry: entry, Binding: binding, Endpoint: endpoint, Secret: material, ClientIdentity: trustedIdentity, HTTPClient: g.httpClient})
		if result.Closer == nil && !nilInterface(result.Provider) {
			if closer, ok := result.Provider.(io.Closer); ok {
				result.Closer = closer
			}
		}
		if buildErr != nil {
			var candidateErr *adaptercore.CandidateBindingError
			if errors.As(buildErr, &candidateErr) && candidateErr != nil {
				return FactoryResult{}, errors.Join(candidateErr, closeFactoryResult(result.Closer))
			}
			return FactoryResult{}, errors.Join(
				gatewayError(modelinvoker.ErrorProviderUnavailable, "factory_build_failed", "adapter factory failed", nil),
				closeFactoryResult(result.Closer),
			)
		}
		if identityErr := providerIdentityError(selection.AdapterID, result.Provider); identityErr != nil {
			return FactoryResult{}, errors.Join(identityErr, closeFactoryResult(result.Closer))
		}
		if nilInterface(result.Closer) {
			return FactoryResult{}, gatewayError(modelinvoker.ErrorProviderUnavailable, "factory_closer_nil", "adapter factory returned no lifecycle closer", nil)
		}
		if strings.TrimSpace(result.Endpoint) == "" {
			return FactoryResult{}, errors.Join(
				gatewayError(modelinvoker.ErrorMapping, "factory_endpoint_missing", "adapter factory did not report its concrete protocol endpoint", nil),
				closeFactoryResult(result.Closer),
			)
		}
		if endpointErr := validateFactoryEndpoint(endpoint, result.Endpoint); endpointErr != nil {
			return FactoryResult{}, errors.Join(endpointErr, closeFactoryResult(result.Closer))
		}
		return result, nil
	})
	if err != nil {
		return preparedCall{}, wrapRoute(selection, err)
	}
	selection.Endpoint = lease.endpoint
	request := call.Request
	request.Provider = selection.AdapterID
	request.Protocol = selection.Protocol
	request.Endpoint = selection.Endpoint
	resolution := Resolution{Route: selection.Clone(), BindingVersion: binding.Version, CredentialVersion: material.Version, FactoryID: factory.ID(), FactoryVersion: factory.Version(), ClientIdentity: trustedIdentity}
	return preparedCall{resolution: resolution, request: request, lease: lease}, nil
}

func isSubscription(entry catalog.Entry) bool {
	return entry.Route.Offering.Kind == upstream.OfferingTokenPlan || entry.Route.Offering.Kind == upstream.OfferingCodingPlan
}

func validateBinding(entry catalog.Entry, binding RuntimeBinding) (string, error) {
	identity := entry.Route.Identity()
	if binding.RouteID != entry.ID || binding.Identity != identity || binding.DeploymentID != entry.Route.Deployment.ID || binding.Region != entry.Route.Deployment.Region || !safeVersion(binding.Version) {
		return "", gatewayError(modelinvoker.ErrorMapping, "binding_anchor_mismatch", "runtime binding does not match the selected route identity, deployment, or region", nil)
	}
	for _, item := range []struct{ reference, value string }{
		{entry.Route.Deployment.ProjectRef, binding.Project},
		{entry.Route.Deployment.WorkspaceRef, binding.Workspace},
		{entry.Route.Deployment.ResourceRef, binding.Resource},
		{entry.Route.Deployment.DeploymentName, binding.Deployment},
	} {
		if item.reference != "" && !safeBindingValue(item.value) {
			return "", gatewayError(modelinvoker.ErrorMapping, "binding_value_missing", "a referenced runtime binding value is missing or unsafe", nil)
		}
	}
	deployment := entry.Route.Deployment
	deployment.ProjectRef, deployment.WorkspaceRef, deployment.ResourceRef, deployment.DeploymentName = binding.Project, binding.Workspace, binding.Resource, binding.Deployment
	endpoint, err := entry.Route.Endpoint.ResolveBaseURL(deployment)
	if err != nil {
		return "", gatewayError(modelinvoker.ErrorMapping, "binding_endpoint_invalid", "runtime binding could not produce the catalog endpoint", err)
	}
	return endpoint, nil
}

func validateFactoryEndpoint(catalogEndpoint, factoryEndpoint string) error {
	base, baseErr := url.Parse(catalogEndpoint)
	actual, actualErr := url.Parse(factoryEndpoint)
	if baseErr != nil || actualErr != nil || base.Scheme == "" || base.Host == "" || actual.Scheme == "" || actual.Host == "" ||
		base.User != nil || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" ||
		actual.User != nil || actual.RawQuery != "" || actual.ForceQuery || actual.Fragment != "" {
		return gatewayError(modelinvoker.ErrorMapping, "factory_endpoint_invalid", "adapter factory returned an invalid concrete endpoint", nil)
	}
	if !sameEndpointAuthority(base, actual) {
		return gatewayError(modelinvoker.ErrorMapping, "factory_endpoint_cross_route", "adapter factory endpoint changed the catalog scheme or host", nil)
	}
	basePath, basePathErr := trustedEndpointPath(base)
	actualPath, actualPathErr := trustedEndpointPath(actual)
	if basePathErr != nil || actualPathErr != nil {
		return gatewayError(modelinvoker.ErrorMapping, "factory_endpoint_invalid", "adapter factory returned an unsafe concrete endpoint path", nil)
	}
	if basePath != "" && actualPath != basePath && !strings.HasPrefix(actualPath, basePath+"/") {
		return gatewayError(modelinvoker.ErrorMapping, "factory_endpoint_cross_route", "adapter factory endpoint escaped the catalog base path", nil)
	}
	return nil
}

func trustedEndpointPath(endpoint *url.URL) (string, error) {
	if endpoint == nil || endpoint.RawPath != "" || strings.Contains(endpoint.EscapedPath(), "%") || strings.ContainsAny(endpoint.Path, "\\\x00\r\n") {
		return "", errors.New("unsafe endpoint path")
	}
	value := strings.TrimRight(endpoint.Path, "/")
	if value == "" {
		return "", nil
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." {
			return "", errors.New("unsafe endpoint path segment")
		}
	}
	if path.Clean(value) != value {
		return "", errors.New("non-canonical endpoint path")
	}
	return value, nil
}

func sameEndpointAuthority(left, right *url.URL) bool {
	if left == nil || right == nil || !strings.EqualFold(left.Scheme, right.Scheme) || !strings.EqualFold(left.Hostname(), right.Hostname()) {
		return false
	}
	normalizedPort := func(endpoint *url.URL) string {
		port := endpoint.Port()
		if (strings.EqualFold(endpoint.Scheme, "https") && (port == "" || port == "443")) ||
			(strings.EqualFold(endpoint.Scheme, "http") && (port == "" || port == "80")) {
			return ""
		}
		return port
	}
	return normalizedPort(left) == normalizedPort(right)
}

func closeFactoryResult(closer io.Closer) error {
	return closeAll([]io.Closer{closer})
}

func safeBindingValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 512 {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func validateSecretMaterial(profile upstream.CredentialProfile, material SecretMaterial, now time.Time) error {
	if material.ProfileID != profile.ID || material.Type != profile.Type || !safeVersion(material.Version) {
		return gatewayError(modelinvoker.ErrorAuthentication, "secret_profile_mismatch", "resolved credential material does not match the selected credential profile", nil)
	}
	if profile.Lifecycle == upstream.CredentialLifecycleShortLived && (material.ExpiresAt.IsZero() || !material.ExpiresAt.After(now)) {
		return gatewayError(modelinvoker.ErrorAuthentication, "secret_expired", "resolved short-lived credential is missing a future expiry", nil)
	}
	switch profile.Type {
	case upstream.CredentialAPIKey:
		value, ok := material.value(upstream.CredentialPurposeAPIKey)
		if !ok {
			return gatewayError(modelinvoker.ErrorAuthentication, "secret_value_missing", "resolved API key is missing", nil)
		}
		if err := profile.ValidateResolvedSecret(value); err != nil {
			return gatewayError(modelinvoker.ErrorAuthentication, "secret_value_rejected", "resolved API key does not satisfy the credential profile", nil)
		}
	case upstream.CredentialBearer:
		if _, ok := material.value(upstream.CredentialPurposeBearerToken); !ok {
			return gatewayError(modelinvoker.ErrorAuthentication, "secret_value_missing", "resolved bearer token is missing", nil)
		}
	case upstream.CredentialSigV4:
		_, access := material.value(upstream.CredentialPurposeAccessKeyID)
		_, secret := material.value(upstream.CredentialPurposeSecretAccessKey)
		if !access || !secret {
			return gatewayError(modelinvoker.ErrorAuthentication, "secret_value_missing", "resolved signing credentials are incomplete", nil)
		}
	case upstream.CredentialADC, upstream.CredentialEntraID, upstream.CredentialOAuth:
		if _, ok := material.value(upstream.CredentialPurposeBearerToken); !ok {
			return gatewayError(modelinvoker.ErrorAuthentication, "secret_value_missing", "resolved access token is missing", nil)
		}
		if material.ExpiresAt.IsZero() || !material.ExpiresAt.After(now) {
			return gatewayError(modelinvoker.ErrorAuthentication, "secret_expired", "resolved access token is missing a future expiry", nil)
		}
	case upstream.CredentialAnonymous:
	default:
		return gatewayError(modelinvoker.ErrorAuthentication, "secret_type_unsupported", "credential type is not supported by the route gateway", nil)
	}
	return nil
}

func wrapRoute(selection modelinvoker.RouteSelection, err error) error {
	if err == nil {
		return nil
	}
	return &modelinvoker.RouteError{RouteID: selection.RouteID, Route: selection.Clone(), Err: stampGatewayError(selection, err)}
}

func stampGatewayError(selection modelinvoker.RouteSelection, err error) error {
	if err == nil {
		return nil
	}
	if cause, ok := err.(*lifecycleCause); ok {
		return &lifecycleCause{raw: cause.raw, public: stampLifecycleCause(selection, cause.raw)}
	}
	if adaptercore.IsCandidateLifecycleCause(err) {
		return err
	}
	if invocationError, ok := err.(*modelinvoker.Error); ok && invocationError != nil {
		copy := *invocationError
		copy.Provider = selection.AdapterID
		copy.Message = safeGatewayErrorMessage(copy.Code)
		copy.MappingReport.Provider = selection.AdapterID
		copy.MappingReport.Protocol = selection.Protocol
		copy.MappingReport.Endpoint = selection.Endpoint
		copy.MappingReport.Decisions = append([]modelinvoker.MappingDecision(nil), invocationError.MappingReport.Decisions...)
		if invocationError.Code == "stream_close_failed" {
			copy.Err = adaptercore.SafeCloseCauseOf(invocationError.Err)
		} else {
			copy.Err = stampGatewayError(selection, invocationError.Err)
		}
		return &copy
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		stamped := make([]error, 0, len(children))
		for _, child := range children {
			stamped = append(stamped, stampGatewayError(selection, child))
		}
		return errors.Join(stamped...)
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		if child := wrapped.Unwrap(); child != nil {
			return stampGatewayError(selection, child)
		}
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return &modelinvoker.Error{
		Kind: modelinvoker.ErrorProvider, Provider: selection.AdapterID, Operation: "route_gateway", Code: "provider_failure",
		Message:       "provider operation failed",
		MappingReport: modelinvoker.MappingReport{Provider: selection.AdapterID, Protocol: selection.Protocol, Endpoint: selection.Endpoint},
	}
}

func stampLifecycleCause(selection modelinvoker.RouteSelection, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*modelinvoker.Error); ok {
		return stampGatewayError(selection, err)
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		stamped := make([]error, 0, len(children))
		for _, child := range children {
			stamped = append(stamped, stampLifecycleCause(selection, child))
		}
		return errors.Join(stamped...)
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		if child := wrapped.Unwrap(); child != nil {
			return stampLifecycleCause(selection, child)
		}
	}
	return nil
}

func safeGatewayErrorMessage(code string) string {
	switch code {
	case "response_model_missing":
		return "provider response is missing the exact selected model"
	case "response_model_mismatch":
		return "provider response model does not match the exact selected model"
	default:
		return "route gateway operation failed"
	}
}

type policyProvider struct{ id modelinvoker.ProviderID }

func (p policyProvider) ID() modelinvoker.ProviderID          { return p.id }
func (policyProvider) DefaultProtocol() modelinvoker.Protocol { return modelinvoker.ProtocolResponses }
func (policyProvider) Capabilities(context.Context, modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	return nil, errors.New("routegateway: policy provider must not be called")
}
func (policyProvider) Invoke(context.Context, modelinvoker.Request) (modelinvoker.Response, error) {
	return modelinvoker.Response{}, errors.New("routegateway: policy provider must not be called")
}
func (policyProvider) Stream(context.Context, modelinvoker.Request) (modelinvoker.Stream, error) {
	return nil, errors.New("routegateway: policy provider must not be called")
}

type Stream struct {
	inner          modelinvoker.Stream
	resolution     Resolution
	lease          *adapterLease
	requestedModel string
	current        modelinvoker.StreamEvent
	modelErr       error
	done           bool
	once           sync.Once
	releaseErr     error
}

func (s *Stream) Resolution() Resolution {
	if s == nil {
		return Resolution{}
	}
	return s.resolution
}

func (s *Stream) Next() bool {
	if s == nil || s.inner == nil || s.done {
		return false
	}
	next := s.inner.Next()
	if !next {
		s.release()
		return false
	}
	s.current = s.inner.Event()
	if s.current.Error != nil {
		stamped := stampGatewayError(s.resolution.Route, s.current.Error)
		var invocationError *modelinvoker.Error
		if errors.As(stamped, &invocationError) && invocationError != nil {
			s.current.Error = invocationError
		}
		if isResponseModelError(stamped) {
			s.current.Response = nil
			s.current.Raw = modelinvoker.RawPayload{}
		}
	}
	if s.current.Response != nil {
		if err := responseModelError(s.requestedModel, s.current.Response.Model); err != nil {
			stamped := stampGatewayError(s.resolution.Route, err)
			var eventError *modelinvoker.Error
			if !errors.As(stamped, &eventError) || eventError == nil {
				eventError = gatewayError(modelinvoker.ErrorMapping, "response_model_mismatch", "provider response model does not match the exact selected model", nil)
			}
			s.current = modelinvoker.StreamEvent{
				Type: modelinvoker.StreamEventError, ResponseID: s.current.ResponseID,
				Error: eventError, Sequence: s.current.Sequence,
			}
			closeErr := adaptercore.SafeCloseError(s.resolution.Route.AdapterID, "route_gateway.stream_model", s.inner.Close())
			s.modelErr = errors.Join(stamped, closeErr)
			s.inner = nil
			s.done = true
			s.release()
		} else {
			response := stampGatewayResponse(s.resolution, *s.current.Response)
			s.current.Response = &response
		}
	}
	return true
}

func (s *Stream) Event() modelinvoker.StreamEvent {
	if s == nil {
		return modelinvoker.StreamEvent{}
	}
	return s.current
}

func (s *Stream) Err() error {
	if s == nil {
		return nil
	}
	var innerErr error
	if s.inner != nil {
		innerErr = s.inner.Err()
	}
	return wrapRoute(s.resolution.Route, errors.Join(innerErr, s.modelErr, s.releaseErr))
}

func (s *Stream) Close() error {
	if s == nil {
		return nil
	}
	var innerErr error
	if s.inner != nil {
		innerErr = adaptercore.SafeCloseError(s.resolution.Route.AdapterID, "route_gateway.stream_close", s.inner.Close())
	}
	s.release()
	return wrapRoute(s.resolution.Route, errors.Join(innerErr, s.releaseErr))
}

func (s *Stream) release() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		if s.lease != nil {
			s.releaseErr = s.lease.release()
		}
	})
}

func responseModelError(requested, actual string) *modelinvoker.Error {
	if actual == "" {
		return gatewayError(modelinvoker.ErrorMapping, "response_model_missing", "provider response is missing the exact selected model", nil)
	}
	if actual == requested {
		return nil
	}
	return gatewayError(modelinvoker.ErrorMapping, "response_model_mismatch", "provider response model does not match the exact selected model", nil)
}

func isResponseModelError(err error) bool {
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError == nil {
		return false
	}
	return invocationError.Code == "response_model_missing" || invocationError.Code == "response_model_mismatch"
}

func stampGatewayResponse(resolution Resolution, response modelinvoker.Response) modelinvoker.Response {
	response.Provider = resolution.Route.AdapterID
	response.Protocol = resolution.Route.Protocol
	if response.State != nil {
		state := *response.State
		state.Provider = resolution.Route.AdapterID
		state.Protocol = resolution.Route.Protocol
		response.State = &state
	}
	response.MappingReport.Provider = resolution.Route.AdapterID
	response.MappingReport.Protocol = resolution.Route.Protocol
	response.MappingReport.Endpoint = resolution.Route.Endpoint
	return response
}

var _ modelinvoker.Stream = (*Stream)(nil)

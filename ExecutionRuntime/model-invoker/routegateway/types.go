// Package routegateway composes catalog policy, runtime bindings, typed secret
// resolution, concrete adapter factories, and adapter lifecycle into the
// RouteID-only execution boundary used by upper layers.
package routegateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

const CandidateVersion = "praxis.model-invoker.route-gateway/v1candidate"

// SecretRequest contains only catalog-owned references and route identity.
// It intentionally has no field in which a caller can place plaintext.
type SecretRequest struct {
	RouteID  upstream.RouteID
	Identity upstream.RouteIdentity
	Profile  upstream.CredentialProfile
}

// SecretResolver is the only boundary allowed to turn typed catalog
// CredentialReferences into short-lived secret material.
type SecretResolver interface {
	ResolveSecret(context.Context, SecretRequest) (SecretMaterial, error)
}

// SecretMaterial carries values only between the resolver and an adapter
// factory. Its formatting is permanently redacted. Version is a caller-owned,
// non-secret rotation identifier and must never be derived from secret bytes.
type SecretMaterial struct {
	ProfileID upstream.CredentialProfileID
	Type      upstream.CredentialType
	Version   string
	ExpiresAt time.Time
	values    map[upstream.CredentialPurpose][]byte
}

// NewSecretMaterial defensively copies resolved bytes. Values and Version are
// deliberately separate so pool keys never require hashing a credential.
func NewSecretMaterial(profileID upstream.CredentialProfileID, credentialType upstream.CredentialType, version string, expiresAt time.Time, values map[upstream.CredentialPurpose][]byte) (SecretMaterial, error) {
	if strings.TrimSpace(string(profileID)) == "" || strings.TrimSpace(string(credentialType)) == "" || !safeVersion(version) {
		return SecretMaterial{}, gatewayError(modelinvoker.ErrorInvalidRequest, "secret_material_invalid", "secret material identity, type, and non-secret version are required", nil)
	}
	copyValues := make(map[upstream.CredentialPurpose][]byte, len(values))
	for purpose, value := range values {
		if strings.TrimSpace(string(purpose)) == "" || len(value) == 0 {
			return SecretMaterial{}, gatewayError(modelinvoker.ErrorInvalidRequest, "secret_material_invalid", "secret material contains an empty purpose or value", nil)
		}
		copyValues[purpose] = append([]byte(nil), value...)
	}
	return SecretMaterial{ProfileID: profileID, Type: credentialType, Version: version, ExpiresAt: expiresAt, values: copyValues}, nil
}

func (SecretMaterial) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "routegateway.SecretMaterial([REDACTED])")
}
func (SecretMaterial) GoString() string { return "routegateway.SecretMaterial([REDACTED])" }

func (m SecretMaterial) value(purpose upstream.CredentialPurpose) (string, bool) {
	value, ok := m.values[purpose]
	if !ok || len(value) == 0 {
		return "", false
	}
	return string(value), true
}

func (m SecretMaterial) zero() {
	for _, value := range m.values {
		for index := range value {
			value[index] = 0
		}
	}
}

// BindingRequest asks for non-secret values referenced by one immutable
// catalog entry. The resolver cannot choose a different route or offering.
type BindingRequest struct{ Entry catalog.Entry }

type RuntimeBindingResolver interface {
	ResolveBinding(context.Context, BindingRequest) (RuntimeBinding, error)
}

// RuntimeBinding contains resolved non-secret deployment values. Anchor fields
// must match the catalog entry exactly; only the referenced runtime values may
// be replaced by the resolver.
type RuntimeBinding struct {
	RouteID      upstream.RouteID
	Identity     upstream.RouteIdentity
	DeploymentID upstream.DeploymentID
	Region       string
	Project      string
	Workspace    string
	Resource     string
	Deployment   string
	Version      string
}

// CatalogBindingResolver is a deterministic offline resolver. It treats the
// catalog reference strings as resolved values and is intended for validation
// and pure-construction environments, not production secret/config discovery.
type CatalogBindingResolver struct{}

func (CatalogBindingResolver) ResolveBinding(_ context.Context, request BindingRequest) (RuntimeBinding, error) {
	entry := request.Entry
	return RuntimeBinding{
		RouteID: entry.ID, Identity: entry.Route.Identity(), DeploymentID: entry.Route.Deployment.ID,
		Region: entry.Route.Deployment.Region, Project: entry.Route.Deployment.ProjectRef,
		Workspace: entry.Route.Deployment.WorkspaceRef, Resource: entry.Route.Deployment.ResourceRef,
		Deployment: entry.Route.Deployment.DeploymentName, Version: "catalog:" + entry.Evidence.Digest,
	}, nil
}

type FactoryInput struct {
	Entry          catalog.Entry
	Binding        RuntimeBinding
	Endpoint       string
	Secret         SecretMaterial
	ClientIdentity upstream.ClientIdentity
	HTTPClient     *http.Client
}

type FactoryResult struct {
	Provider modelinvoker.Provider
	Closer   io.Closer
	// Endpoint is the concrete protocol binding owned by the constructed
	// adapter. It may be more specific than the catalog base path (for example,
	// Vertex OpenAPI or Azure legacy deployment paths).
	Endpoint string
}

type AdapterFactory interface {
	ID() string
	Version() string
	AdapterID() modelinvoker.ProviderID
	// Build must return promptly when the context is cancelled. Gateway.Close
	// waits for every in-flight Build so that late FactoryResult closers and
	// their errors remain observable instead of leaking lifecycle work.
	Build(context.Context, FactoryInput) (FactoryResult, error)
}

type Resolution struct {
	Route             modelinvoker.RouteSelection
	BindingVersion    string
	CredentialVersion string
	FactoryID         string
	FactoryVersion    string
	ClientIdentity    upstream.ClientIdentity
}

type CapabilityResult struct {
	Resolution Resolution
	Contract   modelinvoker.CapabilityContract
}

type InvokeResult struct {
	Resolution Resolution
	Response   modelinvoker.Response
}

func safeVersion(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func gatewayError(kind modelinvoker.ErrorKind, code, message string, err error) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Operation: "route_gateway", Code: code, Message: message, Err: err}
}

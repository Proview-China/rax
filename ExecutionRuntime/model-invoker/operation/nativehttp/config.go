// Package nativehttp provides a fail-closed HTTP execution boundary for
// provider-native peripheral operations. It does not infer capabilities from
// a compatible URL: every method, path, model, content type, and result shape
// must be declared by a Spec.
package nativehttp

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
)

type TrustMode string

const (
	TrustOfficial TrustMode = "official"
	TrustRelay    TrustMode = "relay"
	TrustLocal    TrustMode = "local"
)

type AuthMode string

const (
	AuthAnonymous AuthMode = "anonymous"
	AuthBearer    AuthMode = "bearer"
	AuthHeader    AuthMode = "header"
)

type ResponseMode string

const (
	ResponseJSON   ResponseMode = "json"
	ResponseBinary ResponseMode = "binary"
	ResponseSSE    ResponseMode = "sse"
	ResponseNDJSON ResponseMode = "ndjson"
)

type Spec struct {
	Kind               operation.Kind
	Method             string
	Path               string
	Lifecycle          operation.Lifecycle
	Support            operation.SupportLevel
	Models             []string
	ContentTypes       []string
	AllowedQuery       []string
	Headers            http.Header
	ResponseMode       ResponseMode
	ArtifactKind       operation.ArtifactKind
	ArtifactMIME       string
	RequiresResourceID bool
	RequiresParentID   bool
	RequiresModel      bool
	// BodyModelField binds the semantic request model to an exact top-level
	// JSON or multipart form field. Empty means the model is carried by the
	// pinned URL path or the operation is not model-specific.
	BodyModelField string
	IDKeys         []string
	IDPrefix       string
	StatusKeys     []string
	DoneKeys       []string
	FailureKeys    []string
	URLKeys        []string
	Base64Keys     []string
	TranscriptKeys []string
	Limitations    []string
}

type Config struct {
	Provider      modelinvoker.ProviderID
	BaseURL       string
	Trust         TrustMode
	OfficialHosts []string
	Auth          AuthMode
	APIKey        string
	HeaderName    string
	HeaderPrefix  string
	UserAgent     string
	StaticHeaders http.Header
	Specs         []Spec
	HTTPClient    *http.Client
}

func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "nativehttp.Config([REDACTED])")
}
func (Config) GoString() string { return "nativehttp.Config([REDACTED])" }

var placeholderPattern = regexp.MustCompile(`\{(id|parent_id|model)\}`)

func (c Config) validate() (string, map[operation.Kind]Spec, error) {
	if strings.TrimSpace(string(c.Provider)) == "" {
		return "", nil, fmt.Errorf("native operation provider ID is required")
	}
	if c.BaseURL == "" || strings.ContainsAny(c.BaseURL, "\r\n\x00") {
		return "", nil, fmt.Errorf("native operation base URL is required")
	}
	if c.UserAgent != "" && (c.UserAgent != strings.TrimSpace(c.UserAgent) || len(c.UserAgent) > 512 || strings.ContainsAny(c.UserAgent, "\r\n")) {
		return "", nil, fmt.Errorf("native operation user agent is invalid")
	}
	for name, values := range c.StaticHeaders {
		if !validHeaderName(name) || forbiddenStaticHeader(name) {
			return "", nil, fmt.Errorf("native operation static header is invalid or security-sensitive")
		}
		for _, value := range values {
			if strings.ContainsAny(value, "\r\n\x00") {
				return "", nil, fmt.Errorf("native operation static header value is invalid")
			}
		}
	}
	policy := adaptercore.EndpointPolicy{}
	switch c.Trust {
	case TrustOfficial:
		if len(c.OfficialHosts) == 0 {
			return "", nil, fmt.Errorf("official operation provider requires host allowlist")
		}
		policy.OfficialHosts = c.OfficialHosts
		if parsed, err := adaptercore.ParseEndpointForPolicy(c.BaseURL); err == nil {
			if path := strings.TrimRight(parsed.Path, "/"); path != "" {
				policy.OfficialPaths = []string{path}
			}
		}
	case TrustRelay:
		parsed, err := adaptercore.ParseEndpointForPolicy(c.BaseURL)
		if err != nil {
			return "", nil, err
		}
		policy.OfficialHosts, policy.AllowLoopback = []string{parsed.Hostname()}, true
		if path := strings.TrimRight(parsed.Path, "/"); path != "" {
			policy.OfficialPaths = []string{path}
		}
	case TrustLocal:
		policy.AllowLoopback, policy.LoopbackOnly = true, true
	default:
		return "", nil, fmt.Errorf("native operation trust mode is invalid")
	}
	base, err := adaptercore.ValidateEndpoint(c.BaseURL, policy)
	if err != nil {
		return "", nil, fmt.Errorf("native operation base URL is invalid: %w", err)
	}
	switch c.Auth {
	case AuthAnonymous:
		if c.APIKey != "" || c.HeaderName != "" || c.HeaderPrefix != "" {
			return "", nil, fmt.Errorf("anonymous operation auth must not include credentials")
		}
	case AuthBearer:
		if !validSecret(c.APIKey) {
			return "", nil, fmt.Errorf("bearer operation credential is required")
		}
	case AuthHeader:
		if !validSecret(c.APIKey) || !validHeaderName(c.HeaderName) || strings.ContainsAny(c.HeaderPrefix, "\r\n") {
			return "", nil, fmt.Errorf("header operation auth is invalid")
		}
	default:
		return "", nil, fmt.Errorf("native operation auth mode is invalid")
	}
	if len(c.Specs) == 0 {
		return "", nil, fmt.Errorf("native operation provider requires specs")
	}
	specs := make(map[operation.Kind]Spec, len(c.Specs))
	for _, spec := range c.Specs {
		if err := validateSpec(spec); err != nil {
			return "", nil, err
		}
		if _, exists := specs[spec.Kind]; exists {
			return "", nil, fmt.Errorf("native operation spec %s is duplicated", spec.Kind)
		}
		specs[spec.Kind] = cloneSpec(spec)
	}
	return base, specs, nil
}

func validateSpec(spec Spec) error {
	if spec.Kind == "" {
		return fmt.Errorf("native operation spec kind is required")
	}
	method := strings.ToUpper(spec.Method)
	if method != http.MethodGet && method != http.MethodPost && method != http.MethodDelete && method != http.MethodPut && method != http.MethodPatch {
		return fmt.Errorf("native operation %s method is invalid", spec.Kind)
	}
	if spec.Path == "" || !strings.HasPrefix(spec.Path, "/") || strings.Contains(spec.Path, "//") || strings.ContainsAny(spec.Path, "?\r\n\x00") {
		return fmt.Errorf("native operation %s path is invalid", spec.Kind)
	}
	clean := placeholderPattern.ReplaceAllString(spec.Path, "x")
	if strings.Contains(clean, "{") || strings.Contains(clean, "}") || strings.Contains(clean, "/../") || strings.HasSuffix(clean, "/..") {
		return fmt.Errorf("native operation %s path has an unsupported placeholder", spec.Kind)
	}
	if spec.RequiresResourceID != strings.Contains(spec.Path, "{id}") {
		return fmt.Errorf("native operation %s resource placeholder contract differs", spec.Kind)
	}
	if spec.RequiresParentID != strings.Contains(spec.Path, "{parent_id}") {
		return fmt.Errorf("native operation %s parent placeholder contract differs", spec.Kind)
	}
	if spec.RequiresModel != strings.Contains(spec.Path, "{model}") {
		return fmt.Errorf("native operation %s model placeholder contract differs", spec.Kind)
	}
	if spec.BodyModelField != "" && (!validHeaderName(spec.BodyModelField) || len(spec.Models) == 0) {
		return fmt.Errorf("native operation %s body model binding is invalid or lacks an exact allowlist", spec.Kind)
	}
	if spec.IDPrefix != "" && (!strings.HasSuffix(spec.IDPrefix, "/") || strings.HasPrefix(spec.IDPrefix, "/") || strings.ContainsAny(spec.IDPrefix, "\\\r\n\x00") || strings.Contains(spec.IDPrefix, "..")) {
		return fmt.Errorf("native operation %s ID prefix is invalid", spec.Kind)
	}
	switch spec.Lifecycle {
	case operation.LifecycleRequest, operation.LifecycleJob, operation.LifecycleResource:
	default:
		return fmt.Errorf("native operation %s lifecycle is invalid", spec.Kind)
	}
	switch spec.Support {
	case operation.SupportNative, operation.SupportCompatible, operation.SupportPartial:
	default:
		return fmt.Errorf("native operation %s support is invalid", spec.Kind)
	}
	switch spec.ResponseMode {
	case ResponseJSON, ResponseBinary, ResponseSSE, ResponseNDJSON:
	default:
		return fmt.Errorf("native operation %s response mode is invalid", spec.Kind)
	}
	for _, contentType := range spec.ContentTypes {
		if strings.TrimSpace(contentType) == "" || strings.ContainsAny(contentType, "\r\n") {
			return fmt.Errorf("native operation %s content type is invalid", spec.Kind)
		}
	}
	for _, query := range spec.AllowedQuery {
		if strings.TrimSpace(query) == "" || strings.ContainsAny(query, "\r\n") {
			return fmt.Errorf("native operation %s query allowlist is invalid", spec.Kind)
		}
	}
	for name, values := range spec.Headers {
		if !validHeaderName(name) || forbiddenStaticHeader(name) {
			return fmt.Errorf("native operation %s header is invalid or security-sensitive", spec.Kind)
		}
		for _, value := range values {
			if strings.ContainsAny(value, "\r\n\x00") {
				return fmt.Errorf("native operation %s header value is invalid", spec.Kind)
			}
		}
	}
	return nil
}

func validSecret(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "\r\n\x00")
}
func validHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}

func forbiddenStaticHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "proxy-authorization", "cookie", "host", "x-api-key", "api-key":
		return true
	default:
		return false
	}
}

func cloneSpec(spec Spec) Spec {
	spec.Models = append([]string(nil), spec.Models...)
	spec.ContentTypes = append([]string(nil), spec.ContentTypes...)
	spec.AllowedQuery = append([]string(nil), spec.AllowedQuery...)
	spec.Headers = spec.Headers.Clone()
	spec.IDKeys = append([]string(nil), spec.IDKeys...)
	spec.StatusKeys = append([]string(nil), spec.StatusKeys...)
	spec.DoneKeys = append([]string(nil), spec.DoneKeys...)
	spec.FailureKeys = append([]string(nil), spec.FailureKeys...)
	spec.URLKeys = append([]string(nil), spec.URLKeys...)
	spec.Base64Keys = append([]string(nil), spec.Base64Keys...)
	spec.TranscriptKeys = append([]string(nil), spec.TranscriptKeys...)
	spec.Limitations = append([]string(nil), spec.Limitations...)
	return spec
}

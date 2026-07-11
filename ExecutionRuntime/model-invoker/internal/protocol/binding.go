package protocol

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

var (
	runtimeProviderPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	protocolPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9_./-]{0,127}$`)
	headerPattern          = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,127}$`)
)

// Binding is the complete runtime identity needed by a protocol driver.
// Provider is the modelinvoker Registry/Adapter ID, not the commercial
// upstream.ProviderID stored in the catalog. Endpoint is canonicalized once
// by NewBinding. Credentials and SDK clients deliberately have no field here.
type Binding struct {
	Provider         modelinvoker.ProviderID
	Protocol         modelinvoker.Protocol
	Endpoint         string
	RequestIDHeaders []string
}

// NewBinding validates and canonicalizes a protocol binding.
func NewBinding(provider modelinvoker.ProviderID, protocol modelinvoker.Protocol, endpoint string, requestIDHeaders ...string) (Binding, error) {
	binding := Binding{
		Provider:         provider,
		Protocol:         protocol,
		Endpoint:         endpoint,
		RequestIDHeaders: append([]string(nil), requestIDHeaders...),
	}
	canonical, err := binding.canonical()
	if err != nil {
		return Binding{}, err
	}
	return canonical, nil
}

// Validate checks that a Binding is already in its canonical, safe form.
func (b Binding) Validate() error {
	canonical, err := b.canonical()
	if err != nil {
		return err
	}
	if canonical.Provider != b.Provider || canonical.Protocol != b.Protocol || canonical.Endpoint != b.Endpoint || !equalStrings(canonical.RequestIDHeaders, b.RequestIDHeaders) {
		return fmt.Errorf("protocol binding is not canonical; construct it with NewBinding")
	}
	return nil
}

// Clone returns an independent immutable-value copy.
func (b Binding) Clone() Binding {
	clone := b
	clone.RequestIDHeaders = append([]string(nil), b.RequestIDHeaders...)
	return clone
}

// EffectiveEndpoint returns the validated request endpoint when present,
// otherwise the configured binding endpoint.
func (b Binding) EffectiveEndpoint(requested string) string {
	if requested == "" {
		return b.Endpoint
	}
	canonical, err := canonicalEndpoint(requested)
	if err != nil || canonical != b.Endpoint {
		return b.Endpoint
	}
	return canonical
}

// ValidateRequest rejects selection and continuation mismatches before a
// protocol driver calls its SDK client.
func (b Binding) ValidateRequest(request modelinvoker.Request) error {
	if err := b.Validate(); err != nil {
		return bindingError(b.Provider, modelinvoker.ErrorInvalidRequest, "validate", err.Error())
	}
	if err := request.Validate(); err != nil {
		return b.StampError(nil, request, err, "validate")
	}
	if request.Provider != b.Provider {
		return bindingError(b.Provider, modelinvoker.ErrorInvalidRequest, "validate", "request provider does not match the protocol binding")
	}
	if request.Protocol != b.Protocol {
		return bindingError(b.Provider, modelinvoker.ErrorInvalidRequest, "validate", "request protocol does not match the protocol binding")
	}
	if request.Endpoint != "" {
		endpoint, err := canonicalEndpoint(request.Endpoint)
		if err != nil {
			return bindingError(b.Provider, modelinvoker.ErrorInvalidRequest, "validate", "request endpoint is invalid")
		}
		if endpoint != b.Endpoint {
			return bindingError(b.Provider, modelinvoker.ErrorMapping, "validate", "request endpoint does not match the protocol binding")
		}
	}
	for namespace := range request.ProviderOptions {
		if namespace != b.Provider {
			return bindingError(b.Provider, modelinvoker.ErrorInvalidRequest, "validate", "provider options namespace does not match the protocol binding")
		}
	}
	if request.State != nil {
		if request.State.Provider != b.Provider || request.State.Protocol != b.Protocol {
			return bindingError(b.Provider, modelinvoker.ErrorMapping, "validate", "continuation state does not match the protocol binding")
		}
	}
	return nil
}

func (b Binding) canonical() (Binding, error) {
	if !runtimeProviderPattern.MatchString(string(b.Provider)) {
		return Binding{}, fmt.Errorf("protocol binding provider must be a stable runtime ProviderID")
	}
	if b.Protocol == modelinvoker.ProtocolAuto || !protocolPattern.MatchString(string(b.Protocol)) {
		return Binding{}, fmt.Errorf("protocol binding protocol must be an explicit stable identifier")
	}
	endpoint, err := canonicalEndpoint(b.Endpoint)
	if err != nil {
		return Binding{}, fmt.Errorf("protocol binding endpoint: %w", err)
	}
	headers := make([]string, len(b.RequestIDHeaders))
	seen := make(map[string]struct{}, len(headers))
	for index, header := range b.RequestIDHeaders {
		header = strings.TrimSpace(header)
		if header != strings.ToLower(header) || !headerPattern.MatchString(header) {
			return Binding{}, fmt.Errorf("protocol binding request ID header %d is invalid", index)
		}
		if _, exists := seen[header]; exists {
			return Binding{}, fmt.Errorf("protocol binding request ID header %q is duplicated", header)
		}
		seen[header] = struct{}{}
		headers[index] = header
	}
	return Binding{Provider: b.Provider, Protocol: b.Protocol, Endpoint: endpoint, RequestIDHeaders: headers}, nil
}

func canonicalEndpoint(raw string) (string, error) {
	if raw == "" || raw != strings.TrimSpace(raw) || strings.ContainsAny(raw, "\r\n") {
		return "", fmt.Errorf("must be a non-empty absolute URL")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("must be a valid absolute URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", fmt.Errorf("must not contain user info, query, or fragment")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" {
		return "", fmt.Errorf("scheme must be HTTP or HTTPS")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" || strings.Contains(hostname, "%") {
		return "", fmt.Errorf("host is invalid")
	}
	if scheme == "http" && !isLoopbackHost(hostname) {
		return "", fmt.Errorf("plain HTTP is allowed only for loopback endpoints")
	}
	port := parsed.Port()
	if port != "" {
		numericPort, err := strconv.Atoi(port)
		if err != nil || numericPort < 1 || numericPort > 65535 {
			return "", fmt.Errorf("port is invalid")
		}
		port = strconv.Itoa(numericPort)
	}
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	host := hostname
	if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	}
	endpointPath := strings.TrimRight(parsed.EscapedPath(), "/")
	if !safeEndpointPath(endpointPath) {
		return "", fmt.Errorf("path is not a canonical traversal-free API prefix")
	}
	return scheme + "://" + host + endpointPath, nil
}

func safeEndpointPath(value string) bool {
	if value == "" {
		return true
	}
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "//") {
		return false
	}
	decoded := value
	for attempt := 0; attempt < 8; attempt++ {
		next, err := url.PathUnescape(decoded)
		if err != nil {
			return false
		}
		decoded = next
		if strings.Contains(decoded, "//") || strings.ContainsAny(decoded, "\\?#{}\r\n") {
			return false
		}
		for _, character := range decoded {
			if unicode.IsControl(character) {
				return false
			}
		}
		if !strings.Contains(decoded, "%") {
			break
		}
		if attempt == 7 {
			return false
		}
	}
	for _, segment := range strings.Split(strings.Trim(decoded, "/"), "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func bindingError(provider modelinvoker.ProviderID, kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: provider, Operation: operation, Message: message}
}

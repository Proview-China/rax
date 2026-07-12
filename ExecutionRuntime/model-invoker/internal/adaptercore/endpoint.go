package adaptercore

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"unicode"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

// EndpointPolicy is the credential-audience boundary for a public Provider
// Config. Production endpoints must match an exact host or one single DNS
// label below OfficialHostSuffix. Loopback is an explicit test-only exception.
type EndpointPolicy struct {
	OfficialHosts      []string
	OfficialHostSuffix string
	OfficialPaths      []string
	AllowLoopback      bool
	LoopbackOnly       bool
}

// CandidateBindingReceipt is an internal construction receipt used by the
// Route Gateway built-in factories. It exposes only the Adapter-owned concrete
// protocol endpoint and is not a stable public SDK capability.
type CandidateBindingReceipt interface {
	CandidateBindingEndpoint(modelinvoker.Protocol, string) (string, bool)
}

// ValidateEndpoint applies a fail-closed production endpoint policy and
// returns a canonical trailing-slash-free URL. It never resolves DNS: only
// literal localhost and loopback IP addresses qualify for the test exception.
func ValidateEndpoint(raw string, policy EndpointPolicy) (string, error) {
	if raw == "" || raw != strings.TrimSpace(raw) || strings.ContainsAny(raw, "\r\n\x00") {
		return "", fmt.Errorf("endpoint must be a non-empty absolute URL without surrounding whitespace")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("endpoint must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", fmt.Errorf("endpoint must not contain user info, query, or fragment")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("endpoint must use HTTP or HTTPS")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || strings.Contains(host, "%") || strings.HasSuffix(host, ".") {
		return "", fmt.Errorf("endpoint host is not canonical")
	}
	endpointPath, err := trustedConfigPath(parsed)
	if err != nil {
		return "", err
	}
	loopback := IsLoopbackHost(host)
	if loopback {
		if !policy.AllowLoopback {
			return "", fmt.Errorf("loopback endpoint override is not allowed")
		}
		return canonicalEndpointURL(parsed, scheme, endpointPath), nil
	}
	if policy.LoopbackOnly {
		return "", fmt.Errorf("endpoint override is allowed only for loopback tests")
	}
	if scheme != "https" {
		return "", fmt.Errorf("production endpoint must use HTTPS")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return "", fmt.Errorf("production endpoint must not override the official HTTPS port")
	}
	if !officialHostAllowed(host, policy) {
		return "", fmt.Errorf("production endpoint is outside the official credential audience")
	}
	if !officialPathAllowed(endpointPath, policy.OfficialPaths) {
		return "", fmt.Errorf("production endpoint path is outside the official API prefix")
	}
	return canonicalEndpointURL(parsed, scheme, endpointPath), nil
}

func trustedConfigPath(parsed *url.URL) (string, error) {
	if parsed == nil || parsed.RawPath != "" || strings.Contains(parsed.EscapedPath(), "%") || strings.ContainsAny(parsed.Path, "\\\r\n\x00") {
		return "", fmt.Errorf("endpoint path must be unescaped, canonical, and traversal-free")
	}
	if len(parsed.Path) > 1 && strings.HasSuffix(parsed.Path, "//") {
		return "", fmt.Errorf("endpoint path has repeated trailing separators")
	}
	value := strings.TrimSuffix(parsed.Path, "/")
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "//") {
		return "", fmt.Errorf("endpoint path must be an absolute canonical API prefix")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("endpoint path contains a control character")
		}
	}
	for _, segment := range strings.Split(strings.Trim(value, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("endpoint path must not contain empty or traversal segments")
		}
	}
	return value, nil
}

func officialHostAllowed(host string, policy EndpointPolicy) bool {
	for _, allowed := range policy.OfficialHosts {
		if host == strings.ToLower(strings.TrimSpace(allowed)) && allowed != "" {
			return true
		}
	}
	suffix := strings.ToLower(strings.TrimSpace(policy.OfficialHostSuffix))
	if suffix == "" || strings.HasSuffix(suffix, ".") || !strings.HasSuffix(host, "."+suffix) {
		return false
	}
	prefix := strings.TrimSuffix(host, "."+suffix)
	return safeDNSLabel(prefix)
}

func safeDNSLabel(value string) bool {
	if value == "" || len(value) > 63 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

// IsDNSLabel reports whether value is one canonical lowercase ASCII DNS label.
func IsDNSLabel(value string) bool {
	return value == strings.ToLower(value) && value == strings.TrimSpace(value) && safeDNSLabel(value)
}

// IsPathSegment reports whether value is one unescaped canonical ASCII
// identifier segment. It rejects dot segments and every URL reserved byte.
func IsPathSegment(value string) bool {
	if value == "" || len(value) > 128 || value == "." || value == ".." || value != strings.TrimSpace(value) {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && character != '-' && character != '_' && character != '.' {
			return false
		}
	}
	return true
}

// IsCloudRegion is the shared cross-cloud grammar: one canonical lowercase
// ASCII DNS label with no empty hyphen component. Provider-specific code may
// narrow it further without assuming AWS/GCP/Azure share one shape.
func IsCloudRegion(value string) bool {
	if !IsDNSLabel(value) || strings.Contains(value, "--") {
		return false
	}
	for _, part := range strings.Split(value, "-") {
		if part == "" {
			return false
		}
	}
	return true
}

func officialPathAllowed(actual string, allowed []string) bool {
	if len(allowed) == 0 {
		return actual == ""
	}
	for _, candidate := range allowed {
		if actual == strings.TrimRight(candidate, "/") {
			return true
		}
	}
	return false
}

func canonicalEndpointURL(parsed *url.URL, scheme, endpointPath string) string {
	copy := *parsed
	copy.Scheme = scheme
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		copy.Host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		copy.Host = "[" + hostname + "]"
	} else {
		copy.Host = hostname
	}
	copy.Path, copy.RawPath = endpointPath, ""
	copy.RawQuery, copy.Fragment, copy.ForceQuery = "", "", false
	return strings.TrimRight(copy.String(), "/")
}

// NormalizeEndpoint removes only a trailing path slash while preserving the
// remaining URL components. Public request/config validation owns URL safety.
func NormalizeEndpoint(value string) string {
	if value == "" {
		return ""
	}
	u, err := url.Parse(value)
	if err != nil {
		return strings.TrimRight(value, "/")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String()
}

// EffectiveEndpoint reports the request override when present, otherwise the
// adapter's configured endpoint.
func EffectiveEndpoint(requested, configured string) string {
	if requested != "" {
		return NormalizeEndpoint(requested)
	}
	return NormalizeEndpoint(configured)
}

// IsLoopbackHost reports whether host is localhost or a loopback IP address.
func IsLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

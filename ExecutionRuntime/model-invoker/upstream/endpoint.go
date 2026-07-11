package upstream

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	hostPlaceholderPattern = regexp.MustCompile(`\{([a-z][a-z0-9_]*)\}`)
	dnsLabelPattern        = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
)

var supportedHostPlaceholders = map[string]struct{}{
	"deployment": {},
	"location":   {},
	"project":    {},
	"region":     {},
	"resource":   {},
	"workspace":  {},
}

type EndpointValidationError struct {
	Fields []FieldError
}

func (e *EndpointValidationError) Error() string {
	return formatFieldErrors("invalid endpoint", e.Fields)
}

func (e *EndpointValidationError) HasField(field string) bool {
	return hasField(e.Fields, field)
}

// Placeholders returns the unique host-template placeholders in lexical order.
func (endpoint Endpoint) Placeholders() ([]string, error) {
	matches := hostPlaceholderPattern.FindAllStringSubmatch(endpoint.HostTemplate, -1)
	remainder := hostPlaceholderPattern.ReplaceAllString(endpoint.HostTemplate, "placeholder")
	if strings.ContainsAny(remainder, "{}") {
		return nil, fmt.Errorf("host template has malformed placeholders")
	}
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		name := match[1]
		if _, supported := supportedHostPlaceholders[name]; !supported {
			return nil, fmt.Errorf("unsupported host placeholder %q", name)
		}
		seen[name] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

// Validate checks the endpoint template without resolving or contacting it.
func (endpoint Endpoint) Validate(deployment Deployment) error {
	var fields []FieldError
	add := func(field, problem string) { fields = append(fields, FieldError{Field: field, Problem: problem}) }
	validateID(add, "id", string(endpoint.ID))
	if endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && deployment.Kind == DeploymentSelfHosted) {
		add("scheme", "must be https unless the deployment is self-hosted")
	}
	if !hostPattern.MatchString(endpoint.HostTemplate) || strings.ContainsAny(endpoint.HostTemplate, "@*") {
		add("host_template", "must not contain scheme, path, user info, wildcard, or whitespace")
	}
	placeholders, err := endpoint.Placeholders()
	if err != nil {
		add("host_template", err.Error())
	} else {
		for _, placeholder := range placeholders {
			value := deploymentPlaceholderValue(deployment, placeholder)
			if !dnsLabelPattern.MatchString(value) {
				add("host_template", fmt.Sprintf("placeholder %q has no safe deployment binding", placeholder))
			}
		}
	}
	if _, err := resolveHostTemplate(endpoint.HostTemplate, deployment); err != nil {
		add("host_template", err.Error())
	}
	if !safeBasePath(endpoint.BasePath) {
		add("base_path", "must be an absolute traversal-free path without placeholders, query, fragment, or control characters")
	}
	validateText(add, "credential_audience", endpoint.CredentialAudience)
	if len(fields) == 0 {
		return nil
	}
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].Field < fields[j].Field })
	return &EndpointValidationError{Fields: fields}
}

// ResolveBaseURL substitutes only typed deployment fields and returns a safe
// base URL. Arbitrary maps are intentionally not accepted.
func (endpoint Endpoint) ResolveBaseURL(deployment Deployment) (string, error) {
	if err := endpoint.Validate(deployment); err != nil {
		return "", err
	}
	host, err := resolveHostTemplate(endpoint.HostTemplate, deployment)
	if err != nil {
		return "", err
	}
	resolved := url.URL{Scheme: endpoint.Scheme, Host: host, Path: endpoint.BasePath}
	return resolved.String(), nil
}

func resolveHostTemplate(template string, deployment Deployment) (string, error) {
	if template == "" {
		return "", fmt.Errorf("host template is required")
	}
	var replacementError error
	host := hostPlaceholderPattern.ReplaceAllStringFunc(template, func(token string) string {
		match := hostPlaceholderPattern.FindStringSubmatch(token)
		name := match[1]
		if _, supported := supportedHostPlaceholders[name]; !supported {
			replacementError = fmt.Errorf("unsupported host placeholder %q", name)
			return token
		}
		value := deploymentPlaceholderValue(deployment, name)
		if !dnsLabelPattern.MatchString(value) {
			replacementError = fmt.Errorf("placeholder %q has no safe deployment binding", name)
			return token
		}
		return value
	})
	if replacementError != nil {
		return "", replacementError
	}
	if strings.ContainsAny(host, "{}") {
		return "", fmt.Errorf("host template has malformed placeholders")
	}
	parsed, err := url.Parse("https://" + host)
	if err != nil || parsed.Host != host || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Hostname() == "" {
		return "", fmt.Errorf("host template does not resolve to a valid host")
	}
	if strings.Contains(host, "..") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return "", fmt.Errorf("host template resolves to a non-canonical host")
	}
	return host, nil
}

func deploymentPlaceholderValue(deployment Deployment, placeholder string) string {
	switch placeholder {
	case "region", "location":
		return deployment.Region
	case "project":
		return deployment.ProjectRef
	case "workspace":
		return deployment.WorkspaceRef
	case "resource":
		return deployment.ResourceRef
	case "deployment":
		return deployment.DeploymentName
	default:
		return ""
	}
}

func safeBasePath(value string) bool {
	if value == "" {
		return true
	}
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "%") || strings.ContainsAny(value, "\\?#{}\r\n") {
		return false
	}
	decoded := value
	for attempt := 0; attempt < 4; attempt++ {
		next, err := url.PathUnescape(decoded)
		if err != nil {
			return false
		}
		decoded = next
		if strings.ContainsAny(decoded, "\\?#{}\r\n") {
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
	}
	if strings.Contains(decoded, "//") {
		return false
	}
	trimmed := strings.Trim(decoded, "/")
	if trimmed == "" {
		return true
	}
	for _, segment := range strings.Split(trimmed, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

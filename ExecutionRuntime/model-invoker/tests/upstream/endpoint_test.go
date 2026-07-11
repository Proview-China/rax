package upstream_test

import (
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestEndpointTemplateResolutionIsTypedAndDeterministic(t *testing.T) {
	t.Parallel()
	endpoint := upstream.Endpoint{
		ID:                 "azure.openai",
		Scheme:             "https",
		HostTemplate:       "{resource}.openai.azure.com",
		BasePath:           "/openai/v1/",
		CredentialAudience: "azure-openai",
	}
	deployment := upstream.Deployment{ID: "azure.example", Kind: upstream.DeploymentCloudServerless, Region: "eastus", ResourceRef: "praxis-prod"}
	placeholders, err := endpoint.Placeholders()
	if err != nil {
		t.Fatalf("Placeholders() error = %v", err)
	}
	if !reflect.DeepEqual(placeholders, []string{"resource"}) {
		t.Fatalf("Placeholders() = %#v", placeholders)
	}
	first, err := endpoint.ResolveBaseURL(deployment)
	if err != nil {
		t.Fatalf("ResolveBaseURL() error = %v", err)
	}
	second, _ := endpoint.ResolveBaseURL(deployment)
	if first != "https://praxis-prod.openai.azure.com/openai/v1/" || second != first {
		t.Fatalf("ResolveBaseURL() = %q / %q", first, second)
	}
}

func TestEndpointRejectsTemplateAndPathAttacks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		field  string
		mutate func(*upstream.UpstreamRoute)
	}{
		{name: "wildcard", field: "endpoint.host_template", mutate: func(route *upstream.UpstreamRoute) { route.Endpoint.HostTemplate = "*.vendor.example" }},
		{name: "userinfo", field: "endpoint.host_template", mutate: func(route *upstream.UpstreamRoute) { route.Endpoint.HostTemplate = "user@api.vendor.example" }},
		{name: "scheme injection", field: "endpoint.host_template", mutate: func(route *upstream.UpstreamRoute) { route.Endpoint.HostTemplate = "https://api.vendor.example" }},
		{name: "unknown placeholder", field: "endpoint.host_template", mutate: func(route *upstream.UpstreamRoute) { route.Endpoint.HostTemplate = "{secret}.vendor.example" }},
		{name: "unbound placeholder", field: "endpoint.host_template", mutate: func(route *upstream.UpstreamRoute) { route.Endpoint.HostTemplate = "{resource}.vendor.example" }},
		{name: "malformed placeholder", field: "endpoint.host_template", mutate: func(route *upstream.UpstreamRoute) { route.Endpoint.HostTemplate = "{region.vendor.example" }},
		{name: "dot traversal", field: "endpoint.base_path", mutate: func(route *upstream.UpstreamRoute) { route.Endpoint.BasePath = "/v1/../admin" }},
		{name: "encoded traversal", field: "endpoint.base_path", mutate: func(route *upstream.UpstreamRoute) { route.Endpoint.BasePath = "/v1/%2e%2e/admin" }},
		{name: "double encoded traversal", field: "endpoint.base_path", mutate: func(route *upstream.UpstreamRoute) { route.Endpoint.BasePath = "/v1/%252e%252e/admin" }},
		{name: "backslash traversal", field: "endpoint.base_path", mutate: func(route *upstream.UpstreamRoute) { route.Endpoint.BasePath = `/v1\..\admin` }},
		{name: "double slash", field: "endpoint.base_path", mutate: func(route *upstream.UpstreamRoute) { route.Endpoint.BasePath = "/v1//admin" }},
		{name: "query injection", field: "endpoint.base_path", mutate: func(route *upstream.UpstreamRoute) { route.Endpoint.BasePath = "/v1?key=secret" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			route := validRoute()
			test.mutate(&route)
			var validationError *upstream.ValidationError
			if err := route.Validate(); !errors.As(err, &validationError) || !validationError.HasField(test.field) {
				t.Fatalf("Validate() error = %v, want field %q", err, test.field)
			}
		})
	}
}

func FuzzEndpointResolutionNeverReturnsUnsafeURL(f *testing.F) {
	seeds := [][2]string{
		{"api.vendor.example", "/v1"},
		{"{region}.api.vendor.example", "/v1/"},
		{"*.vendor.example", "/v1"},
		{"user@vendor.example", "/../../secret"},
		{"{secret}.vendor.example", "/v1/%252e%252e/admin"},
	}
	for _, seed := range seeds {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, hostTemplate, basePath string) {
		endpoint := upstream.Endpoint{
			ID:                 "fuzz.endpoint",
			Scheme:             "https",
			HostTemplate:       hostTemplate,
			BasePath:           basePath,
			CredentialAudience: "fuzz-audience",
		}
		deployment := upstream.Deployment{ID: "fuzz.deployment", Kind: upstream.DeploymentDirect, Region: "us-east-1", ResourceRef: "resource"}
		resolved, err := endpoint.ResolveBaseURL(deployment)
		if err != nil {
			return
		}
		parsed, err := url.Parse(resolved)
		if err != nil {
			t.Fatalf("resolved URL does not parse: %q: %v", resolved, err)
		}
		decoded, err := url.PathUnescape(parsed.Path)
		if err != nil {
			t.Fatalf("resolved path does not unescape: %q", parsed.Path)
		}
		if parsed.Scheme != "https" || parsed.User != nil || parsed.Host == "" || strings.ContainsAny(parsed.Host, "{}@*") || hasTraversalSegment(decoded) || strings.Contains(decoded, `\`) || parsed.RawQuery != "" || parsed.Fragment != "" {
			t.Fatalf("unsafe resolved URL: %q", resolved)
		}
	})
}

func hasTraversalSegment(path string) bool {
	for _, segment := range strings.Split(strings.Trim(path, "/"), "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

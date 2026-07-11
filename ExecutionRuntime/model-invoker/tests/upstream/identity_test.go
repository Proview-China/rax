package upstream_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

const evidenceDigest = "sha256:597ec867f0351b62c60aadd2f32c240d94a546876da196a26502741d48a1cb8c"

func TestRouteIdentityContainsEveryDimensionDeterministically(t *testing.T) {
	t.Parallel()
	identity := validRoute().Identity()
	if err := identity.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	wantKey := "vendor.direct.payg.responses|vendor-model|vendor|vendor.api.payg|vendor.direct.global|responses|vendor.public|vendor.default"
	key, err := identity.CanonicalKey()
	if err != nil {
		t.Fatalf("CanonicalKey() error = %v", err)
	}
	if key != wantKey {
		t.Fatalf("CanonicalKey() = %q, want %q", key, wantKey)
	}
	first, err := identity.Digest()
	if err != nil {
		t.Fatalf("Digest() error = %v", err)
	}
	second, _ := identity.Digest()
	if first != second || len(first) != len("sha256:")+64 {
		t.Fatalf("Digest() is not deterministic SHA-256: %q / %q", first, second)
	}
}

func TestRouteIdentityRejectsMissingDimensions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		field  string
		mutate func(*upstream.RouteIdentity)
	}{
		{name: "route", field: "route_id", mutate: func(identity *upstream.RouteIdentity) { identity.RouteID = "" }},
		{name: "model", field: "model_family", mutate: func(identity *upstream.RouteIdentity) { identity.ModelFamily = "" }},
		{name: "provider", field: "provider", mutate: func(identity *upstream.RouteIdentity) { identity.Provider = "" }},
		{name: "offering", field: "offering", mutate: func(identity *upstream.RouteIdentity) { identity.Offering = "" }},
		{name: "deployment", field: "deployment", mutate: func(identity *upstream.RouteIdentity) { identity.Deployment = "" }},
		{name: "protocol", field: "protocol", mutate: func(identity *upstream.RouteIdentity) { identity.Protocol = "" }},
		{name: "endpoint", field: "endpoint", mutate: func(identity *upstream.RouteIdentity) { identity.Endpoint = "" }},
		{name: "credential", field: "credential", mutate: func(identity *upstream.RouteIdentity) { identity.Credential = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identity := validRoute().Identity()
			test.mutate(&identity)
			var validationError *upstream.IdentityValidationError
			if err := identity.Validate(); !errors.As(err, &validationError) || !validationError.HasField(test.field) {
				t.Fatalf("Validate() error = %v, want field %q", err, test.field)
			}
		})
	}
}

func TestMappingReportCanonicalAuditDigest(t *testing.T) {
	t.Parallel()
	route := validRoute()
	decisions := []upstream.CapabilityDecision{
		{Capability: "tool_calling", Action: upstream.CapabilityDegraded, ReasonCode: "partial", Limitations: []string{"zeta", "alpha"}},
		{Capability: "text_generation", Action: upstream.CapabilityExact, ReasonCode: "native"},
	}
	reasons := []upstream.MappingReason{{Code: "selected", Detail: "route match"}, {Code: "capability_match"}}
	first, err := upstream.NewMappingReport(route, evidenceDigest, decisions, reasons...)
	if err != nil {
		t.Fatalf("NewMappingReport() error = %v", err)
	}
	second, err := upstream.NewMappingReport(route, evidenceDigest, []upstream.CapabilityDecision{decisions[1], decisions[0]}, reasons[1], reasons[0])
	if err != nil {
		t.Fatalf("NewMappingReport(reordered) error = %v", err)
	}
	firstDigest, err := first.AuditDigest()
	if err != nil {
		t.Fatalf("AuditDigest() error = %v", err)
	}
	secondDigest, err := second.AuditDigest()
	if err != nil {
		t.Fatalf("AuditDigest(reordered) error = %v", err)
	}
	if firstDigest != secondDigest {
		t.Fatalf("audit digests differ by input ordering: %q != %q", firstDigest, secondDigest)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("canonical reports differ:\nfirst=%#v\nsecond=%#v", first, second)
	}
	clone := first.Clone()
	clone.CapabilityDecisions[1].Limitations[0] = "mutated"
	if first.CapabilityDecisions[1].Limitations[0] == "mutated" {
		t.Fatal("Clone() retained capability limitation slice")
	}
}

func TestMappingReportValidationRejectsIncompleteAuditData(t *testing.T) {
	t.Parallel()
	valid, err := upstream.NewMappingReport(validRoute(), evidenceDigest, []upstream.CapabilityDecision{{Capability: "text_generation", Action: upstream.CapabilityExact, ReasonCode: "native"}})
	if err != nil {
		t.Fatalf("NewMappingReport() error = %v", err)
	}
	tests := []struct {
		name   string
		field  string
		mutate func(*upstream.MappingReport)
	}{
		{name: "route mismatch", field: "route_id", mutate: func(report *upstream.MappingReport) { report.RouteID = "other" }},
		{name: "provider mismatch", field: "provider", mutate: func(report *upstream.MappingReport) { report.Provider = "other" }},
		{name: "bad evidence digest", field: "evidence_digest", mutate: func(report *upstream.MappingReport) { report.EvidenceDigest = "sha256:short" }},
		{name: "missing capability decisions", field: "capability_decisions", mutate: func(report *upstream.MappingReport) { report.CapabilityDecisions = nil }},
		{name: "invalid action", field: "capability_decisions", mutate: func(report *upstream.MappingReport) { report.CapabilityDecisions[0].Action = "silent_drop" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := valid.Clone()
			test.mutate(&report)
			var validationError *upstream.MappingValidationError
			if err := report.Validate(); !errors.As(err, &validationError) || !validationError.HasField(test.field) {
				t.Fatalf("Validate() error = %v, want field %q", err, test.field)
			}
		})
	}
}

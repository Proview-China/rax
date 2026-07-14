// Package conformance provides backend-neutral Runtime contract checks. Its
// reports are evidence inputs only; a passing report never writes BindingFact
// or grants dispatch authority.
package conformance

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type SubjectClassV2 string

const (
	SubjectExternalAdapter SubjectClassV2 = "external_adapter"
	SubjectTestFixture     SubjectClassV2 = "test_fixture"
)

var adapterAllowedImportsV2 = [...]string{
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core",
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports",
}

// AdapterAllowedImportsV2 returns a copy of the documented adapter import
// allowlist. It grants no authority; build-time dependency scanning and
// conformance checks perform the actual enforcement.
func AdapterAllowedImportsV2() []string {
	return append([]string(nil), adapterAllowedImportsV2[:]...)
}

// CheckAdapterRuntimeImportsV2 is a build/conformance scanning gate for
// external and custom component adapters. Passing it grants no Binding,
// production or dispatch authority; it only proves the adapter did not import
// Runtime Fact Owners, fakes or coordinator implementations.
func CheckAdapterRuntimeImportsV2(imports []string) error {
	for _, path := range imports {
		if !strings.Contains(path, "/ExecutionRuntime/runtime/") {
			continue
		}
		allowed := false
		for _, candidate := range adapterAllowedImportsV2 {
			if path == candidate {
				allowed = true
				break
			}
		}
		if !allowed {
			return core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "component adapter imports a Runtime implementation package")
		}
	}
	return nil
}

type BindingAdapterCaseV2 struct {
	SubjectClass SubjectClassV2
	Adapter      ports.DescriberV2
	Catalog      ports.GovernanceCatalogV2
	Clock        func() time.Time
}

type BindingAdapterReportV2 struct {
	Registered              bool                   `json:"registered"`
	Probed                  bool                   `json:"probed"`
	DeclaredConformance     ports.ConformanceLevel `json:"declared_conformance"`
	CertificationCandidate  bool                   `json:"certification_candidate"`
	BindingEligible         bool                   `json:"binding_eligible"`
	ProductionClaimEligible bool                   `json:"production_claim_eligible"`
	DispatchEligible        bool                   `json:"dispatch_eligible"`
	ManifestDigest          core.Digest            `json:"manifest_digest"`
}

// CheckBindingAdapterV2 verifies discovery and governance registration. It
// deliberately reports DispatchEligible=false: registration, probing,
// certification and binding are distinct from a future Governance permit.
func CheckBindingAdapterV2(ctx context.Context, testCase BindingAdapterCaseV2) (BindingAdapterReportV2, error) {
	if testCase.SubjectClass != SubjectExternalAdapter && testCase.SubjectClass != SubjectTestFixture {
		return BindingAdapterReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "conformance subject class must be explicit")
	}
	if testCase.Clock == nil {
		return BindingAdapterReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "conformance clock must be injected")
	}
	registry, err := ports.NewComponentRegistryV2(testCase.Catalog)
	if err != nil {
		return BindingAdapterReportV2{}, err
	}
	registered, err := registry.Register(ctx, testCase.Adapter)
	if err != nil {
		return BindingAdapterReportV2{}, err
	}
	now := testCase.Clock()
	probed, err := registry.Probe(ctx, registered.Manifest.ComponentID, now)
	if err != nil {
		return BindingAdapterReportV2{}, err
	}
	candidate := probed.Manifest.Conformance != ports.ConformanceRejected
	return BindingAdapterReportV2{
		Registered: true, Probed: true, DeclaredConformance: probed.Manifest.Conformance,
		CertificationCandidate: candidate, BindingEligible: false,
		ProductionClaimEligible: false,
		DispatchEligible:        false, ManifestDigest: probed.ManifestDigest,
	}, nil
}

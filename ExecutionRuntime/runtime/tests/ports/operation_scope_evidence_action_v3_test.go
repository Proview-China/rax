package ports_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/tests/testsupport"
)

func TestOperationScopeEvidenceActionV3ClosedMatrixAndExactNominalProjection(t *testing.T) {
	if err := ports.OperationScopeEvidenceActionMatrixV3().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, key := range map[string]ports.OperationScopeEvidenceApplicabilityMatrixKeyV3{
		"activation":    {OperationKind: ports.OperationScopeActivationV3, EffectKind: ports.OperationScopeEvidenceActionEffectKindV3, PolicyProfile: ports.OperationScopeEvidenceActionPolicyProfileV3},
		"other_effect":  {OperationKind: ports.OperationScopeRunV3, EffectKind: "praxis.tool/cancel", PolicyProfile: ports.OperationScopeEvidenceActionPolicyProfileV3},
		"other_profile": {OperationKind: ports.OperationScopeRunV3, EffectKind: ports.OperationScopeEvidenceActionEffectKindV3, PolicyProfile: "praxis.tool/custom"},
		"termination":   {OperationKind: ports.OperationScopeTerminationV3, EffectKind: ports.OperationScopeEvidenceActionEffectKindV3, PolicyProfile: ports.OperationScopeEvidenceActionPolicyProfileV3},
	} {
		t.Run(name, func(t *testing.T) {
			if err := key.Validate(); err == nil {
				t.Fatal("unsupported Action matrix row was accepted")
			}
		})
	}
	for _, route := range ports.OperationScopeEvidenceActionRoutesV3() {
		source := ports.OperationScopeEvidenceActionApplicabilitySourceV3{Route: route, ID: "source-" + string(route.Dimension), Revision: 7, Digest: core.DigestBytes([]byte(route.Kind))}
		ref, err := ports.ProjectOperationScopeEvidenceActionApplicabilityRefV3(source)
		if err != nil {
			t.Fatal(err)
		}
		if ref.Kind != source.Route.Kind || ref.ID != source.ID || ref.Revision != source.Revision || ref.Digest != source.Digest {
			t.Fatal("Runtime nominal projection changed an Owner source coordinate")
		}
	}
	wrongVersion := ports.OperationScopeEvidenceActionRoutesV3()[0]
	wrongVersion.OwnerContractVersion = "9.0.0"
	if _, err := ports.ProjectOperationScopeEvidenceActionApplicabilityRefV3(ports.OperationScopeEvidenceActionApplicabilitySourceV3{Route: wrongVersion, ID: "source-wrong-version", Revision: 1, Digest: core.DigestBytes([]byte("wrong-version"))}); err == nil {
		t.Fatal("unregistered Owner contract version was projected")
	}
}

func TestOperationScopeEvidenceActionV3RejectsMissingForbiddenAndTypePunnedDimensions(t *testing.T) {
	values := operationScopeEvidenceActionValuesV3()
	if err := ports.ValidateOperationScopeEvidenceActionApplicabilityV3(values); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func([]ports.OperationScopeEvidenceApplicabilityV3){
		"missing": func(v []ports.OperationScopeEvidenceApplicabilityV3) { v[0] = v[1] },
		"forbidden": func(v []ports.OperationScopeEvidenceApplicabilityV3) {
			v[0].Mode = ports.OperationScopeEvidenceForbiddenV3
			v[0].Fact = nil
		},
		"session_as_turn": func(v []ports.OperationScopeEvidenceApplicabilityV3) {
			v[4].Fact.Kind = ports.OperationScopeEvidenceSessionCurrentKindV3
		},
		"pending_action_as_candidate": func(v []ports.OperationScopeEvidenceApplicabilityV3) {
			v[0].Fact.Kind = "praxis.harness/pending-action"
		},
		"next_frame_as_parent": func(v []ports.OperationScopeEvidenceApplicabilityV3) { v[1].Fact.Kind = "praxis.context/next-frame" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := operationScopeEvidenceActionValuesV3()
			mutate(changed)
			changed = ports.NormalizeOperationScopeEvidenceApplicabilityV3(changed)
			if err := ports.ValidateOperationScopeEvidenceActionApplicabilityV3(changed); err == nil {
				t.Fatal("invalid Action applicability set was accepted")
			}
		})
	}
}

func TestOperationProviderBoundaryV1CanonicalAndExactCrossBinding(t *testing.T) {
	fixture := testsupport.OperationScopeEvidenceActionFixture()
	if err := fixture.Boundary.ValidateCurrent(fixture.Boundary.Ref, fixture.Operation, fixture.ScopeDigest, fixture.Attempt, fixture.Enforcement, fixture.Handoff.RefV3(), fixture.Now); err != nil {
		t.Fatal(err)
	}
	original := fixture.Boundary.Digest
	resealed, err := ports.SealOperationProviderBoundaryCurrentProjectionV1(fixture.Boundary)
	if err != nil || resealed.Digest != original {
		t.Fatalf("boundary projection is not deterministic: %v", err)
	}
	for name, mutate := range map[string]func(*ports.OperationProviderBoundaryCurrentProjectionV1){
		"ref": func(p *ports.OperationProviderBoundaryCurrentProjectionV1) { p.Ref.Revision++ },
		"scope": func(p *ports.OperationProviderBoundaryCurrentProjectionV1) {
			p.OperationScopeDigest = core.DigestBytes([]byte("other-scope"))
		},
		"attempt": func(p *ports.OperationProviderBoundaryCurrentProjectionV1) { p.Attempt.AttemptID = "other-attempt" },
		"enforcement": func(p *ports.OperationProviderBoundaryCurrentProjectionV1) {
			p.ExecuteEnforcement.ReceiptDigest = core.DigestBytes([]byte("other-receipt"))
		},
		"handoff": func(p *ports.OperationProviderBoundaryCurrentProjectionV1) {
			p.ExecuteEvidenceHandoff.ID = "other-handoff"
		},
		"stage": func(p *ports.OperationProviderBoundaryCurrentProjectionV1) { p.Stage = "prepared" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := fixture.Boundary
			mutate(&changed)
			sealed, err := ports.SealOperationProviderBoundaryCurrentProjectionV1(changed)
			if err == nil {
				err = sealed.ValidateCurrent(fixture.Boundary.Ref, fixture.Operation, fixture.ScopeDigest, fixture.Attempt, fixture.Enforcement, fixture.Handoff.RefV3(), fixture.Now)
			}
			if err == nil {
				t.Fatal("cross-boundary drift was accepted as the original current boundary")
			}
		})
	}
}

func TestOperationProviderBoundaryV1AttemptUsesValueSemanticsAfterDeepCopy(t *testing.T) {
	fixture := testsupport.OperationScopeEvidenceActionFixture()
	delegation := ports.ExecutionDelegationRefV2{ID: "delegation-action-test", Revision: 1, Digest: core.DigestBytes([]byte("delegation-action-test"))}
	fixture.Attempt.Delegation = &delegation
	fixture.Boundary.Attempt = fixture.Attempt
	sealed, err := ports.SealOperationProviderBoundaryCurrentProjectionV1(fixture.Boundary)
	if err != nil {
		t.Fatal(err)
	}
	copyDelegation := delegation
	copyAttempt := fixture.Attempt
	copyAttempt.Delegation = &copyDelegation
	if copyAttempt.Delegation == fixture.Attempt.Delegation {
		t.Fatal("test did not create distinct pointer identities")
	}
	if err := sealed.ValidateCurrent(sealed.Ref, fixture.Operation, fixture.ScopeDigest, copyAttempt, fixture.Enforcement, fixture.Handoff.RefV3(), fixture.Now); err != nil {
		t.Fatalf("equal delegation values failed after deep copy: %v", err)
	}
}

func operationScopeEvidenceActionValuesV3() []ports.OperationScopeEvidenceApplicabilityV3 {
	values := make([]ports.OperationScopeEvidenceApplicabilityV3, 0, 5)
	for _, route := range ports.OperationScopeEvidenceActionRoutesV3() {
		ref := ports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: route.Kind, ID: "source-" + string(route.Dimension), Revision: 1, Digest: core.DigestBytes([]byte(route.Kind))}
		values = append(values, ports.OperationScopeEvidenceApplicabilityV3{Dimension: route.Dimension, Mode: ports.OperationScopeEvidenceRequiredV3, Fact: &ref})
	}
	return ports.NormalizeOperationScopeEvidenceApplicabilityV3(values)
}

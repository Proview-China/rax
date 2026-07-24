package ports_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationScopeEvidenceV3CanonicalNilEmptyAndOrdering(t *testing.T) {
	now := time.Unix(910000, 0)
	schema := operationEvidenceSchemaV3()
	ref := ports.OperationScopeEvidenceFactRefV3{ID: "qualification-1", Revision: 1, Digest: operationEvidenceDigestV3("qualification"), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	candidate := ports.OperationScopeEvidenceCandidateV3{ContractVersion: ports.OperationScopeEvidenceContractVersionV3, Qualification: ports.OperationScopeEvidenceQualificationRefV3(ref), Source: ports.OperationScopeEvidenceSourceKeyV3{RegistrationID: "source-1", SourceEpoch: 1, SourceSequence: 1}, EventID: "event-1", TrustClass: ports.EvidenceTrustObservation, Payload: ports.EvidencePayloadRefV2{Schema: schema, ContentDigest: operationEvidenceDigestV3("payload"), Revision: 1, Length: 1, Ref: "evidence://one"}, CorrelationID: "correlation-1", ObservedUnixNano: now.UnixNano()}
	nilDigest, err := candidate.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	candidate.Causation = []ports.EvidenceCausationRefV2{}
	emptyDigest, err := candidate.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	if nilDigest != emptyDigest {
		t.Fatal("nil and empty causation changed canonical digest")
	}
	applicability := []ports.OperationScopeEvidenceApplicabilityV3{{Dimension: ports.OperationScopeEvidenceTurnV3, Mode: ports.OperationScopeEvidenceForbiddenV3}, {Dimension: ports.OperationScopeEvidenceRunV3, Mode: ports.OperationScopeEvidenceForbiddenV3}, {Dimension: ports.OperationScopeEvidenceSessionV3, Mode: ports.OperationScopeEvidenceForbiddenV3}, {Dimension: ports.OperationScopeEvidenceContextV3, Mode: ports.OperationScopeEvidenceForbiddenV3}, {Dimension: ports.OperationScopeEvidenceActionV3, Mode: ports.OperationScopeEvidenceForbiddenV3}}
	fact, err := ports.SealOperationScopeEvidenceApplicabilityPolicyFactV3(ports.OperationScopeEvidenceApplicabilityPolicyFactV3{ID: "app-policy-1", Revision: 1, State: ports.OperationScopeEvidencePolicyActiveV3, OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/allocate", Profile: ports.OperationScopeEvidenceActivationProfileV3, ExecutionScopeDigest: operationEvidenceDigestV3("scope"), Applicability: applicability, ExpiresUnixNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if fact.Applicability[0].Dimension != ports.OperationScopeEvidenceActionV3 || fact.Applicability[4].Dimension != ports.OperationScopeEvidenceTurnV3 {
		t.Fatalf("Seal did not canonicalize applicability: %#v", fact.Applicability)
	}
}

func TestOperationScopeEvidenceV3FirstCutRejectsTerminationAndUnboundedTTL(t *testing.T) {
	now := time.Unix(910000, 0)
	base := ports.OperationScopeEvidencePolicyFactV3{ID: "policy-1", Revision: 1, State: ports.OperationScopeEvidencePolicyActiveV3, OperationKind: ports.OperationScopeTerminationV3, EffectKind: "custom/termination", AllowedPhases: []ports.OperationDispatchEnforcementPhaseV4{ports.OperationDispatchEnforcementPrepareV4}, ExpectedSchema: operationEvidenceSchemaV3(), MaximumPayloadBytes: 1, MaximumQualificationTTL: time.Second, MaximumIngestGrace: time.Second, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	if _, err := ports.SealOperationScopeEvidencePolicyFactV3(base); err == nil {
		t.Fatal("termination_attempt entered activation-only first cut")
	}
	base.OperationKind = ports.OperationScopeActivationV3
	base.MaximumIngestGrace = ports.MaxOperationScopeEvidenceIngestGraceV3 + time.Nanosecond
	if _, err := ports.SealOperationScopeEvidencePolicyFactV3(base); err == nil {
		t.Fatal("unbounded ingest grace was accepted")
	}
}

func TestOperationScopeEvidenceV3ClosedMatrixRejectsCustomKindAndProfile(t *testing.T) {
	for name, key := range map[string]ports.OperationScopeEvidenceApplicabilityMatrixKeyV3{
		"backend_discovery":          {OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/backend-discovery", PolicyProfile: ports.OperationScopeEvidenceActivationProfileV3},
		"discover":                   {OperationKind: ports.OperationScopeActivationV3, EffectKind: "discover", PolicyProfile: ports.OperationScopeEvidenceActivationProfileV3},
		"cancel":                     {OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/cancel", PolicyProfile: ports.OperationScopeEvidenceActivationProfileV3},
		"rollback":                   {OperationKind: ports.OperationScopeActivationV3, EffectKind: "rollback", PolicyProfile: ports.OperationScopeEvidenceActivationProfileV3},
		"close":                      {OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/close", PolicyProfile: ports.OperationScopeEvidenceActivationProfileV3},
		"release":                    {OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/release", PolicyProfile: ports.OperationScopeEvidenceActivationProfileV3},
		"termination":                {OperationKind: ports.OperationScopeTerminationV3, EffectKind: "praxis.sandbox/close", PolicyProfile: ports.OperationScopeEvidenceActivationProfileV3},
		"custom_kind":                {OperationKind: ports.OperationScopeActivationV3, EffectKind: "custom/activation", PolicyProfile: ports.OperationScopeEvidenceActivationProfileV3},
		"custom_profile":             {OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/allocate", PolicyProfile: "custom/self-approved"},
		"allocate_recovery_profile":  {OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/allocate", PolicyProfile: ports.OperationScopeEvidenceRecoveryProfileV3},
		"activate_recovery_profile":  {OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/activate", PolicyProfile: ports.OperationScopeEvidenceRecoveryProfileV3},
		"open_recovery_profile":      {OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/open", PolicyProfile: ports.OperationScopeEvidenceRecoveryProfileV3},
		"inspect_activation_profile": {OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/inspect", PolicyProfile: ports.OperationScopeEvidenceActivationProfileV3},
		"inspect_run_scope":          {OperationKind: ports.OperationScopeRunV3, EffectKind: "praxis.sandbox/inspect", PolicyProfile: ports.OperationScopeEvidenceActivationInspectionProfileV3},
	} {
		t.Run(name, func(t *testing.T) {
			if err := key.Validate(); err == nil {
				t.Fatal("unregistered matrix key was accepted")
			}
		})
	}
	for _, key := range []ports.OperationScopeEvidenceApplicabilityMatrixKeyV3{{OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/allocate", PolicyProfile: ports.OperationScopeEvidenceActivationProfileV3}, {OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/activate", PolicyProfile: ports.OperationScopeEvidenceActivationProfileV3}, {OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/open", PolicyProfile: ports.OperationScopeEvidenceActivationProfileV3}, {OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/inspect", PolicyProfile: ports.OperationScopeEvidenceActivationInspectionProfileV3}} {
		if err := key.Validate(); err != nil {
			t.Fatalf("registered matrix key rejected: %v", err)
		}
	}
}

func TestOperationScopeEvidenceV3SandboxRunTerminationAdminMatrixIsExact(t *testing.T) {
	valid := []ports.OperationScopeEvidenceApplicabilityMatrixKeyV3{
		{OperationKind: ports.OperationScopeRunV3, EffectKind: "praxis.sandbox/cancel", PolicyProfile: ports.OperationScopeEvidenceSandboxRunProfileV3},
		{OperationKind: ports.OperationScopeRunV3, EffectKind: "praxis.sandbox/workspace-commit", PolicyProfile: ports.OperationScopeEvidenceSandboxRunProfileV3},
		{OperationKind: ports.OperationScopeTerminationV3, EffectKind: "praxis.sandbox/close", PolicyProfile: ports.OperationScopeEvidenceSandboxTerminationProfileV3},
		{OperationKind: ports.OperationScopeTerminationV3, EffectKind: "praxis.sandbox/fence", PolicyProfile: ports.OperationScopeEvidenceSandboxTerminationProfileV3},
		{OperationKind: ports.OperationScopeTerminationV3, EffectKind: "praxis.sandbox/release", PolicyProfile: ports.OperationScopeEvidenceSandboxTerminationProfileV3},
		{OperationKind: ports.OperationScopeTerminationV3, EffectKind: "praxis.sandbox/cleanup", PolicyProfile: ports.OperationScopeEvidenceSandboxTerminationProfileV3},
		{OperationKind: ports.OperationScopeAdminV3, EffectKind: "praxis.sandbox/fence", PolicyProfile: ports.OperationScopeEvidenceSandboxAdminProfileV3},
		{OperationKind: ports.OperationScopeAdminV3, EffectKind: "praxis.sandbox/cleanup", PolicyProfile: ports.OperationScopeEvidenceSandboxAdminProfileV3},
		{OperationKind: ports.OperationScopeRunV3, EffectKind: "praxis.sandbox/inspect", PolicyProfile: ports.OperationScopeEvidenceSandboxInspectionProfileV3},
		{OperationKind: ports.OperationScopeTerminationV3, EffectKind: "praxis.sandbox/inspect", PolicyProfile: ports.OperationScopeEvidenceSandboxInspectionProfileV3},
		{OperationKind: ports.OperationScopeAdminV3, EffectKind: "praxis.sandbox/inspect", PolicyProfile: ports.OperationScopeEvidenceSandboxInspectionProfileV3},
	}
	for _, key := range valid {
		if err := key.Validate(); err != nil {
			t.Fatalf("registered Sandbox matrix key rejected: %#v: %v", key, err)
		}
	}
	invalid := []ports.OperationScopeEvidenceApplicabilityMatrixKeyV3{
		{OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/cancel", PolicyProfile: ports.OperationScopeEvidenceSandboxRunProfileV3},
		{OperationKind: ports.OperationScopeRunV3, EffectKind: "praxis.sandbox/close", PolicyProfile: ports.OperationScopeEvidenceSandboxRunProfileV3},
		{OperationKind: ports.OperationScopeTerminationV3, EffectKind: "praxis.sandbox/cancel", PolicyProfile: ports.OperationScopeEvidenceSandboxTerminationProfileV3},
		{OperationKind: ports.OperationScopeAdminV3, EffectKind: "praxis.sandbox/release", PolicyProfile: ports.OperationScopeEvidenceSandboxAdminProfileV3},
		{OperationKind: ports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/inspect", PolicyProfile: ports.OperationScopeEvidenceSandboxInspectionProfileV3},
		{OperationKind: ports.OperationScopeRunV3, EffectKind: "praxis.sandbox/workspace-commit", PolicyProfile: ports.OperationScopeEvidenceActivationProfileV3},
	}
	for _, key := range invalid {
		if err := key.Validate(); err == nil {
			t.Fatalf("cross-scope Sandbox matrix key was accepted: %#v", key)
		}
	}
}

func TestOperationScopeEvidenceV3SandboxPolicySubjectsAreRegisteredWithoutCustomKinds(t *testing.T) {
	now := time.Unix(910000, 0)
	for _, tuple := range []struct {
		operation ports.OperationScopeKindV3
		effect    ports.EffectKindV2
	}{
		{ports.OperationScopeRunV3, "praxis.sandbox/cancel"},
		{ports.OperationScopeRunV3, "praxis.sandbox/workspace-commit"},
		{ports.OperationScopeTerminationV3, "praxis.sandbox/close"},
		{ports.OperationScopeTerminationV3, "praxis.sandbox/release"},
		{ports.OperationScopeAdminV3, "praxis.sandbox/fence"},
		{ports.OperationScopeAdminV3, "praxis.sandbox/cleanup"},
	} {
		_, err := ports.SealOperationScopeEvidencePolicyFactV3(ports.OperationScopeEvidencePolicyFactV3{
			ID: "sandbox-policy-1", Revision: 1, State: ports.OperationScopeEvidencePolicyActiveV3,
			OperationKind: tuple.operation, EffectKind: tuple.effect,
			AllowedPhases:  []ports.OperationDispatchEnforcementPhaseV4{ports.OperationDispatchEnforcementExecuteV4, ports.OperationDispatchEnforcementPrepareV4},
			ExpectedSchema: operationEvidenceSchemaV3(), MaximumPayloadBytes: 1,
			MaximumQualificationTTL: time.Second, MaximumIngestGrace: time.Second,
			ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
		})
		if err != nil {
			t.Fatalf("registered policy subject rejected operation=%s effect=%s: %v", tuple.operation, tuple.effect, err)
		}
	}
}

func TestOperationScopeEvidenceV3RecordDetectsCanonicalTamper(t *testing.T) {
	now := time.Unix(910000, 0)
	schema := operationEvidenceSchemaV3()
	q := ports.OperationScopeEvidenceFactRefV3{ID: "qualification-1", Revision: 1, Digest: operationEvidenceDigestV3("qualification"), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	candidate := ports.OperationScopeEvidenceCandidateV3{ContractVersion: ports.OperationScopeEvidenceContractVersionV3, Qualification: ports.OperationScopeEvidenceQualificationRefV3(q), Source: ports.OperationScopeEvidenceSourceKeyV3{RegistrationID: "source-1", SourceEpoch: 1, SourceSequence: 1}, EventID: "event-1", TrustClass: ports.EvidenceTrustObservation, Payload: ports.EvidencePayloadRefV2{Schema: schema, ContentDigest: operationEvidenceDigestV3("payload"), Revision: 1, Length: 1, Ref: "evidence://one"}, Causation: []ports.EvidenceCausationRefV2{}, CorrelationID: "correlation-1", ObservedUnixNano: now.UnixNano()}
	record, err := ports.SealOperationScopeEvidenceRecordV3(ports.OperationScopeEvidenceRecordV3{Ref: ports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: operationEvidenceDigestV3("ledger"), Sequence: 1}, Candidate: candidate, PreviousRecordDigest: ports.EvidenceGenesisDigestV2, IngestedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	record.Candidate.Payload.ContentDigest = operationEvidenceDigestV3("tampered")
	if err := record.Validate(); err == nil {
		t.Fatal("record accepted changed candidate under old digest")
	}
}

func operationEvidenceSchemaV3() ports.SchemaRefV2 {
	return ports.SchemaRefV2{Namespace: "custom.evidence", Name: "observation", Version: "1.0.0", MediaType: "application/json", ContentDigest: operationEvidenceDigestV3("schema")}
}
func operationEvidenceDigestV3(value string) core.Digest { return core.DigestBytes([]byte(value)) }

package ports_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/tests/testsupport"
)

func TestOperationProviderReceiptProjectionV1ExactBindings(t *testing.T) {
	projection, now := operationProviderReceiptProjectionFixtureV1(t)
	if err := projection.ValidateExact(projection.Ref, now); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*ports.OperationProviderReceiptProjectionV1){
		"receipt":   func(p *ports.OperationProviderReceiptProjectionV1) { p.Ref.Digest = core.DigestBytes([]byte("other")) },
		"operation": func(p *ports.OperationProviderReceiptProjectionV1) { p.Operation.SubjectRevision++ },
		"attempt":   func(p *ports.OperationProviderReceiptProjectionV1) { p.Attempt.AttemptID = "other-attempt" },
		"provider": func(p *ports.OperationProviderReceiptProjectionV1) {
			p.Provider.ArtifactDigest = core.DigestBytes([]byte("other"))
		},
		"payload": func(p *ports.OperationProviderReceiptProjectionV1) {
			p.Payload.ContentDigest = core.DigestBytes([]byte("other"))
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := projection
			mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatal("drifted projection passed")
			}
		})
	}
	if err := projection.ValidateExact(projection.Ref, time.Unix(0, projection.CheckedUnixNano-1)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock regression passed: %v", err)
	}
}

func operationProviderReceiptProjectionFixtureV1(t *testing.T) (ports.OperationProviderReceiptProjectionV1, time.Time) {
	t.Helper()
	base := testsupport.OperationScopeEvidenceActionFixture()
	now := base.Now.Add(time.Second)
	provider := ports.ProviderBindingRefV2{BindingSetID: "binding-provider-receipt", BindingSetRevision: 1, ComponentID: "praxis.tool/engine", ManifestDigest: core.DigestBytes([]byte("manifest")), ArtifactDigest: core.DigestBytes([]byte("artifact")), Capability: ports.CapabilityNameV2(ports.OperationScopeEvidenceActionEffectKindV3)}
	delegation := ports.ExecutionDelegationRefV2{ID: "delegation-provider-receipt", Revision: 2, Digest: core.DigestBytes([]byte("delegation-current"))}
	declared := delegation
	declared.Revision, declared.Digest = 1, core.DigestBytes([]byte("delegation-declared"))
	base.Attempt.Delegation = &delegation
	schema := ports.SchemaRefV2{Namespace: "praxis.tool", Name: "request", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("request-schema"))}
	preparedID, err := ports.DerivePreparedProviderAttemptIDV2(delegation.ID, base.Attempt.PermitID, base.Attempt.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := ports.SealPreparedProviderAttemptRefV2(ports.PreparedProviderAttemptRefV2{ID: preparedID, Revision: 1, DeclaredDelegation: declared, OperationDigest: base.Attempt.OperationDigest, IntentID: base.Attempt.EffectID, IntentRevision: base.Attempt.IntentRevision, IntentDigest: base.Attempt.IntentDigest, PermitID: base.Attempt.PermitID, PermitRevision: base.Attempt.PermitRevision, PermitDigest: base.Attempt.PermitDigest, AttemptID: base.Attempt.AttemptID, Provider: provider, PayloadSchema: schema, PayloadDigest: core.DigestBytes([]byte("request")), PayloadRevision: 1, PreparedUnixNano: base.Now.UnixNano(), ExpiresUnixNano: base.Now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	response := []byte(`{"ok":true}`)
	payload := ports.OpaquePayloadV2{Schema: ports.SchemaRefV2{Namespace: "praxis.tool", Name: "provider-receipt", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("receipt-schema"))}, ContentDigest: core.DigestBytes(response), Length: uint64(len(response)), Ref: "receipt://provider-operation-receipt", LimitPolicy: ports.OpaqueLimitPolicyRefV2{Policy: "praxis.tool/provider-receipt-v1", Digest: core.DigestBytes([]byte("receipt-policy"))}}
	owner := ports.EffectOwnerRefV2{Role: ports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest}
	ref := ports.OperationProviderReceiptRefV1{Owner: owner, Kind: "praxis.mcp/protocol-receipt", ID: "provider-operation-receipt", Revision: 1, Digest: core.DigestBytes([]byte("provider-operation-receipt"))}
	projection, err := ports.SealOperationProviderReceiptProjectionV1(ports.OperationProviderReceiptProjectionV1{Ref: ref, Operation: base.Operation, OperationDigest: base.Attempt.OperationDigest, Prepared: prepared, Attempt: base.Attempt, Provider: provider, ProviderOperationRef: ref.ID, Payload: payload, PayloadRevision: 1, ObservedUnixNano: now.Add(-time.Millisecond).UnixNano(), CheckedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return projection, now
}

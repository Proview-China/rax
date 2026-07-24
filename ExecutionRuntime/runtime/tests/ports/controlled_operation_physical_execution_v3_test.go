package ports_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/tests/testsupport"
)

func TestPreparedDomainCommandAssociationV1CanonicalAndCurrent(t *testing.T) {
	fixture := physicalExecutionFixtureV3(t)
	association := fixture.association
	if err := association.ValidateCurrent(association.Ref, fixture.now); err != nil {
		t.Fatal(err)
	}
	if err := association.ValidateCurrent(association.Ref, time.Unix(0, association.ExpiresUnixNano)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("expired association error=%v", err)
	}
	changed := association
	changed.DomainCommand.Digest = digestPhysicalV3("other-command")
	changed.Ref.Digest, changed.ProjectionDigest = "", ""
	if _, err := ports.SealPreparedDomainCommandAssociationCurrentProjectionV1(changed); err == nil {
		t.Fatal("same association identity accepted another domain command")
	}
}

func TestControlledOperationPhysicalExecutionAuthorizationV3ExactBindings(t *testing.T) {
	fixture := physicalExecutionFixtureV3(t)
	authorization := fixture.authorization
	if err := authorization.ValidateCurrent(fixture.now); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ports.ControlledOperationPhysicalExecutionAuthorizationV3){
		"domain_command": func(a *ports.ControlledOperationPhysicalExecutionAuthorizationV3) {
			a.DomainCommand.Digest = digestPhysicalV3("other-command")
		},
		"domain_command_owner": func(a *ports.ControlledOperationPhysicalExecutionAuthorizationV3) {
			a.DomainCommand.Owner.ComponentID = "praxis.tool/other-provider"
		},
		"provider": func(a *ports.ControlledOperationPhysicalExecutionAuthorizationV3) {
			a.Provider.ArtifactDigest = digestPhysicalV3("other-provider")
		},
		"transport": func(a *ports.ControlledOperationPhysicalExecutionAuthorizationV3) { a.ProviderTransport = a.Provider },
		"attempt": func(a *ports.ControlledOperationPhysicalExecutionAuthorizationV3) {
			a.Attempt.AttemptID = "other-attempt"
		},
		"enforcement": func(a *ports.ControlledOperationPhysicalExecutionAuthorizationV3) {
			a.ExecuteEnforcement.Phase = ports.OperationDispatchEnforcementPrepareV4
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := authorization
			mutate(&changed)
			changed.AuthorizationDigest = ""
			if _, err := ports.SealControlledOperationPhysicalExecutionAuthorizationV3(changed); err == nil {
				t.Fatal("drifted physical authorization was accepted")
			}
		})
	}
	if err := authorization.ValidateCurrent(time.Unix(0, authorization.UnifiedNotAfterUnixNano)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("expired authorization error=%v", err)
	}
}

func TestControlledOperationPhysicalExecutionV3FrozenReaderAndPortSignatures(t *testing.T) {
	type expectedReader interface {
		InspectCurrentPreparedDomainCommandAssociationV1(context.Context, ports.PreparedDomainCommandAssociationRefV1) (ports.PreparedDomainCommandAssociationCurrentProjectionV1, error)
	}
	type expectedPort interface {
		ExecuteControlledOperationPhysicalV3(context.Context, ports.ControlledOperationPhysicalExecutionAuthorizationV3) (ports.ControlledOperationProviderAdmissionReceiptRefV2, error)
	}
	type expectedAuthorizationPort interface {
		AuthorizeControlledOperationPhysicalV3(context.Context, ports.ControlledOperationPhysicalAuthorizationRequestV3) (ports.ControlledOperationPhysicalExecutionAuthorizationV3, error)
	}
	type expectedAssociationPort interface {
		EnsurePreparedDomainCommandAssociationV1(context.Context, ports.EnsurePreparedDomainCommandAssociationRequestV1) (ports.PreparedDomainCommandAssociationCurrentProjectionV1, error)
		InspectCurrentPreparedDomainCommandAssociationV1(context.Context, ports.PreparedDomainCommandAssociationRefV1) (ports.PreparedDomainCommandAssociationCurrentProjectionV1, error)
	}
	var _ expectedReader = (ports.PreparedDomainCommandAssociationCurrentReaderV1)(nil)
	var _ expectedPort = (ports.ControlledOperationPhysicalExecutionPortV3)(nil)
	var _ expectedAuthorizationPort = (ports.ControlledOperationPhysicalAuthorizationPortV3)(nil)
	var _ expectedAssociationPort = (ports.PreparedDomainCommandAssociationPortV1)(nil)

	for value, tags := range map[any][]string{
		ports.OperationDomainCommandRefV1{}:           {"owner", "kind", "id", "revision", "digest"},
		ports.PreparedDomainCommandAssociationRefV1{}: {"id", "revision", "digest"},
	} {
		typeOf := reflect.TypeOf(value)
		if typeOf.NumField() != len(tags) {
			t.Fatalf("%s field count=%d want=%d", typeOf.Name(), typeOf.NumField(), len(tags))
		}
		for index, tag := range tags {
			if got := typeOf.Field(index).Tag.Get("json"); got != tag {
				t.Fatalf("%s field %d tag=%q want=%q", typeOf.Name(), index, got, tag)
			}
		}
	}
}

type physicalExecutionFixtureV3Value struct {
	now           time.Time
	association   ports.PreparedDomainCommandAssociationCurrentProjectionV1
	authorization ports.ControlledOperationPhysicalExecutionAuthorizationV3
}

func physicalExecutionFixtureV3(t *testing.T) physicalExecutionFixtureV3Value {
	t.Helper()
	base := testsupport.OperationScopeEvidenceActionFixture()
	now := base.Now.Add(time.Second)
	expires := now.Add(10 * time.Second).UnixNano()
	provider := ports.ProviderBindingRefV2{BindingSetID: "binding-physical-v3", BindingSetRevision: 1, ComponentID: "praxis.tool/provider", ManifestDigest: digestPhysicalV3("provider-manifest"), ArtifactDigest: digestPhysicalV3("provider-artifact"), Capability: ports.CapabilityNameV2(ports.OperationScopeEvidenceActionEffectKindV3)}
	transport := ports.ProviderBindingRefV2{BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision, ComponentID: "praxis.tool/transport", ManifestDigest: digestPhysicalV3("transport-manifest"), ArtifactDigest: digestPhysicalV3("transport-artifact"), Capability: ports.ControlledOperationProviderTransportCapabilityV2}
	delegation := ports.ExecutionDelegationRefV2{ID: "delegation-physical-v3", Revision: 1, Digest: digestPhysicalV3("delegation")}
	preparedID, err := ports.DerivePreparedProviderAttemptIDV2(delegation.ID, base.Attempt.PermitID, base.Attempt.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	schema := ports.SchemaRefV2{Namespace: "praxis.tool", Name: "mcp-call", Version: "1.0.0", MediaType: "application/json", ContentDigest: digestPhysicalV3("schema")}
	prepared, err := ports.SealPreparedProviderAttemptRefV2(ports.PreparedProviderAttemptRefV2{
		ID: preparedID, Revision: 1, DeclaredDelegation: delegation, OperationDigest: base.Attempt.OperationDigest,
		IntentID: base.Attempt.EffectID, IntentRevision: base.Attempt.IntentRevision, IntentDigest: base.Attempt.IntentDigest,
		PermitID: base.Attempt.PermitID, PermitRevision: base.Attempt.PermitRevision, PermitDigest: base.Attempt.PermitDigest,
		AttemptID: base.Attempt.AttemptID, Provider: provider, PayloadSchema: schema, PayloadDigest: digestPhysicalV3("payload"), PayloadRevision: 1,
		PreparedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := ports.OperationDomainCommandRefV1{Owner: ports.EffectOwnerRefV2{Role: ports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest}, Kind: "praxis.mcp/execution-command", ID: "mcp-command-physical-v3", Revision: 1, Digest: digestPhysicalV3("mcp-command")}
	association, err := ports.SealPreparedDomainCommandAssociationCurrentProjectionV1(ports.PreparedDomainCommandAssociationCurrentProjectionV1{
		Operation: base.Operation, OperationDigest: base.Attempt.OperationDigest, EffectID: base.Attempt.EffectID, EffectRevision: base.Attempt.IntentRevision, IntentDigest: base.Attempt.IntentDigest,
		Prepared: prepared, Attempt: base.Attempt, Provider: provider, PayloadSchema: schema, PayloadDigest: prepared.PayloadDigest, PayloadRevision: prepared.PayloadRevision,
		DomainCommand: command, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := ports.SealControlledOperationPhysicalExecutionAuthorizationV3(ports.ControlledOperationPhysicalExecutionAuthorizationV3{
		UnifiedNotAfterUnixNano: expires, ProviderTransport: transport, Provider: provider, Operation: base.Operation, OperationDigest: base.Attempt.OperationDigest,
		OperationScopeDigest: base.Operation.ExecutionScopeDigest, EffectKind: ports.OperationScopeEvidenceActionEffectKindV3, Prepared: prepared, Attempt: base.Attempt,
		ExecuteEnforcement: base.Enforcement, ExecuteEvidenceHandoff: base.Handoff.RefV3(), Boundary: base.Boundary.Ref, Association: association.Ref, DomainCommand: command,
	})
	if err != nil {
		t.Fatal(err)
	}
	return physicalExecutionFixtureV3Value{now: now, association: association, authorization: authorization}
}

func digestPhysicalV3(value string) core.Digest { return core.DigestBytes([]byte(value)) }

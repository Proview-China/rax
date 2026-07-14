package application_test

import (
	"context"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRuntimeBindingCurrentnessAdapterV3NarrowsTTLWithoutAuthorityUpgrade(t *testing.T) {
	now := time.Unix(1_731_000_000, 0).UTC()
	ref := runtimeBindingRefForApplicationTestV3("user.alpha/provider")
	projection := runtimeBindingProjectionForApplicationTestV3(t, ref, now, now.Add(time.Minute))
	reader := &runtimeBindingCurrentnessStubV3{projection: projection}
	adapter, err := application.NewRuntimeBindingCurrentnessAdapterV3(reader, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	authorization, err := adapter.InspectOperationDomainAdapterCurrentV3(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if authorization.State != applicationports.OperationDomainAdapterAuthorizedV3 || authorization.Revision != projection.BindingRevision || authorization.Adapter != ref {
		t.Fatalf("Runtime projection was upgraded or remapped: %#v", authorization)
	}
	if got, want := authorization.ExpiresUnixNano, now.Add(applicationports.MaxOperationDomainAdapterAuthorizationTTLV3).UnixNano(); got != want {
		t.Fatalf("authorization TTL = %d, want %d", got, want)
	}
	if reader.requested != ref {
		t.Fatalf("Runtime currentness read used another Binding ref: %#v", reader.requested)
	}
}

func TestRuntimeBindingCurrentnessAdapterV3UsesEarlierRuntimeExpiry(t *testing.T) {
	now := time.Unix(1_731_000_000, 0).UTC()
	ref := runtimeBindingRefForApplicationTestV3("user.beta/provider")
	projection := runtimeBindingProjectionForApplicationTestV3(t, ref, now, now.Add(7*time.Second))
	adapter, err := application.NewRuntimeBindingCurrentnessAdapterV3(&runtimeBindingCurrentnessStubV3{projection: projection}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := adapter.InspectOperationDomainAdapterCurrentV3(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if authorization.ExpiresUnixNano != projection.ExpiresUnixNano {
		t.Fatalf("Application outlived Runtime projection: got %d want %d", authorization.ExpiresUnixNano, projection.ExpiresUnixNano)
	}
}

func TestRuntimeBindingCurrentnessAdapterV3FailsClosedOnRuntimeProjectionDrift(t *testing.T) {
	now := time.Unix(1_731_000_000, 0).UTC()
	expected := runtimeBindingRefForApplicationTestV3("user.gamma/provider")
	other := runtimeBindingRefForApplicationTestV3("user.delta/provider")

	tests := []struct {
		name   string
		mutate func(*runtimeports.ProviderBindingCurrentProjectionV2)
	}{
		{name: "binding-ref", mutate: func(value *runtimeports.ProviderBindingCurrentProjectionV2) { value.Ref = other }},
		{name: "revoked", mutate: func(value *runtimeports.ProviderBindingCurrentProjectionV2) {
			value.State = runtimeports.ProviderBindingCurrentRevokedV2
		}},
		{name: "expired-state", mutate: func(value *runtimeports.ProviderBindingCurrentProjectionV2) {
			value.State = runtimeports.ProviderBindingCurrentExpiredV2
		}},
		{name: "expired-time", mutate: func(value *runtimeports.ProviderBindingCurrentProjectionV2) { value.ExpiresUnixNano = now.UnixNano() }},
		{name: "digest", mutate: func(value *runtimeports.ProviderBindingCurrentProjectionV2) {
			value.BindingSetDigest = core.Digest("sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection := runtimeBindingProjectionForApplicationTestV3(t, expected, now, now.Add(time.Minute))
			test.mutate(&projection)
			adapter, err := application.NewRuntimeBindingCurrentnessAdapterV3(&runtimeBindingCurrentnessStubV3{projection: projection}, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			if _, err := adapter.InspectOperationDomainAdapterCurrentV3(context.Background(), expected); err == nil {
				t.Fatal("drifted Runtime projection authorized an Application adapter")
			}
		})
	}
}

func TestRuntimeBindingCurrentnessAdapterV3RejectsInvalidExpectedBeforeBackend(t *testing.T) {
	now := time.Unix(1_731_000_000, 0).UTC()
	reader := &runtimeBindingCurrentnessStubV3{}
	adapter, err := application.NewRuntimeBindingCurrentnessAdapterV3(reader, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.InspectOperationDomainAdapterCurrentV3(context.Background(), runtimeports.ProviderBindingRefV2{}); err == nil {
		t.Fatal("invalid expected Binding reached Runtime currentness backend")
	}
	if reader.backendCalls != 0 {
		t.Fatalf("invalid expected Binding made %d backend calls", reader.backendCalls)
	}
}

type runtimeBindingCurrentnessStubV3 struct {
	projection   runtimeports.ProviderBindingCurrentProjectionV2
	requested    runtimeports.ProviderBindingRefV2
	backendCalls int
}

func (s *runtimeBindingCurrentnessStubV3) InspectProviderBindingCurrentV2(_ context.Context, expected runtimeports.ProviderBindingRefV2) (runtimeports.ProviderBindingCurrentProjectionV2, error) {
	s.backendCalls++
	s.requested = expected
	return s.projection, nil
}

func runtimeBindingRefForApplicationTestV3(component runtimeports.ComponentIDV2) runtimeports.ProviderBindingRefV2 {
	return runtimeports.ProviderBindingRefV2{
		BindingSetID:       "binding-set-custom",
		BindingSetRevision: 7,
		ComponentID:        component,
		ManifestDigest:     core.Digest("sha256:1111111111111111111111111111111111111111111111111111111111111111"),
		ArtifactDigest:     core.Digest("sha256:2222222222222222222222222222222222222222222222222222222222222222"),
		Capability:         "user.operation/execute",
	}
}

func runtimeBindingProjectionForApplicationTestV3(t *testing.T, ref runtimeports.ProviderBindingRefV2, now, expires time.Time) runtimeports.ProviderBindingCurrentProjectionV2 {
	t.Helper()
	projection, err := runtimeports.SealProviderBindingCurrentProjectionV2(runtimeports.ProviderBindingCurrentProjectionV2{
		ContractVersion:          runtimeports.ProviderBindingCurrentnessContractVersionV2,
		Ref:                      ref,
		State:                    runtimeports.ProviderBindingCurrentActiveV2,
		BindingSetDigest:         core.Digest("sha256:3333333333333333333333333333333333333333333333333333333333333333"),
		BindingSetSemanticDigest: core.Digest("sha256:4444444444444444444444444444444444444444444444444444444444444444"),
		BindingID:                "binding-custom",
		BindingRevision:          11,
		GrantDigest:              core.Digest("sha256:5555555555555555555555555555555555555555555555555555555555555555"),
		IssuedUnixNano:           now.Add(-time.Second).UnixNano(),
		ExpiresUnixNano:          expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

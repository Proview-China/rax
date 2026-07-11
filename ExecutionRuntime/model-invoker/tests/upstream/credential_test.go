package upstream_test

import (
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestCredentialAuthAndRouteBindings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		field  string
		mutate func(*upstream.UpstreamRoute)
	}{
		{name: "bad header", field: "credential.auth_header", mutate: func(route *upstream.UpstreamRoute) { route.Credential.AuthHeader = "Authorization\r\nX-Evil" }},
		{name: "wire metadata without placement", field: "credential.auth_placement", mutate: func(route *upstream.UpstreamRoute) { route.Credential.AuthPlacement = "" }},
		{name: "missing lifecycle", field: "credential.lifecycle", mutate: func(route *upstream.UpstreamRoute) { route.Credential.Lifecycle = "" }},
		{name: "missing provider binding", field: "credential.allowed_provider_ids", mutate: func(route *upstream.UpstreamRoute) { route.Credential.AllowedProviderIDs = nil }},
		{name: "missing offering binding", field: "credential.allowed_offering_ids", mutate: func(route *upstream.UpstreamRoute) { route.Credential.AllowedOfferingIDs = nil }},
		{name: "missing deployment binding", field: "credential.allowed_deployment_ids", mutate: func(route *upstream.UpstreamRoute) { route.Credential.AllowedDeploymentIDs = nil }},
		{name: "missing region binding", field: "credential.allowed_regions", mutate: func(route *upstream.UpstreamRoute) { route.Credential.AllowedRegions = nil }},
		{name: "query with header", field: "credential.auth_header", mutate: func(route *upstream.UpstreamRoute) {
			route.Credential.AuthPlacement = upstream.AuthPlacementQuery
			route.Credential.AuthParameter = "key"
		}},
		{name: "wrong provider", field: "credential.allowed_provider_ids", mutate: func(route *upstream.UpstreamRoute) {
			route.Credential.AllowedProviderIDs = []upstream.ProviderID{"other"}
		}},
		{name: "wrong offering", field: "credential.allowed_offering_ids", mutate: func(route *upstream.UpstreamRoute) {
			route.Credential.AllowedOfferingIDs = []upstream.OfferingID{"other.payg"}
		}},
		{name: "wrong deployment", field: "credential.allowed_deployment_ids", mutate: func(route *upstream.UpstreamRoute) {
			route.Credential.AllowedDeploymentIDs = []upstream.DeploymentID{"other.direct"}
		}},
		{name: "wrong region", field: "credential.allowed_regions", mutate: func(route *upstream.UpstreamRoute) { route.Credential.AllowedRegions = []string{"us-east-1"} }},
		{name: "duplicate scope", field: "credential.scopes", mutate: func(route *upstream.UpstreamRoute) {
			route.Credential.Scopes = []string{"models.invoke", "models.invoke"}
		}},
		{name: "invalid key prefix", field: "credential.key_prefixes", mutate: func(route *upstream.UpstreamRoute) { route.Credential.KeyPrefixes = []string{"secret prefix"} }},
		{name: "invalid denied key prefix", field: "credential.denied_key_prefixes", mutate: func(route *upstream.UpstreamRoute) { route.Credential.DeniedKeyPrefixes = []string{"secret prefix"} }},
		{name: "wrong purpose", field: "credential.references", mutate: func(route *upstream.UpstreamRoute) {
			route.Credential.References[0].Purpose = upstream.CredentialPurposeBearerToken
		}},
		{name: "environment value not reference", field: "credential.references", mutate: func(route *upstream.UpstreamRoute) { route.Credential.References[0].Name = "sk-live-secret" }},
		{name: "invalid lifecycle", field: "credential.lifecycle", mutate: func(route *upstream.UpstreamRoute) { route.Credential.Lifecycle = "forever" }},
		{name: "sigv4 missing service", field: "credential.sigv4_service", mutate: func(route *upstream.UpstreamRoute) {
			route.Credential.Type = upstream.CredentialSigV4
			route.Credential.References = []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeWorkloadIdentity, Store: "workload_identity", Name: "aws-default"}}
			route.Credential.AuthPlacement = upstream.AuthPlacementRequestSigning
			route.Credential.AuthHeader = ""
			route.Credential.AuthScheme = ""
			route.Credential.Lifecycle = upstream.CredentialLifecycleWorkloadIdentity
			route.Credential.SigV4Service = ""
		}},
		{name: "oauth missing scopes", field: "credential.scopes", mutate: func(route *upstream.UpstreamRoute) {
			route.Credential.Type = upstream.CredentialOAuth
			route.Credential.References = []upstream.CredentialReference{
				{Purpose: upstream.CredentialPurposeClientID, Store: "env", Name: "OAUTH_CLIENT_ID"},
				{Purpose: upstream.CredentialPurposeClientSecret, Store: "env", Name: "OAUTH_CLIENT_SECRET"},
			}
			route.Credential.AuthPlacement = upstream.AuthPlacementSDK
			route.Credential.AuthHeader = ""
			route.Credential.AuthScheme = ""
			route.Credential.Scopes = nil
			route.Credential.Lifecycle = upstream.CredentialLifecycleRenewable
		}},
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

func TestCredentialPurposeCombinations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		typeID  upstream.CredentialType
		refs    []upstream.CredentialReference
		prepare func(*upstream.CredentialProfile)
		wantErr bool
	}{
		{
			name:   "entra client secret",
			typeID: upstream.CredentialEntraID,
			refs: []upstream.CredentialReference{
				{Purpose: upstream.CredentialPurposeTenantID, Store: "env", Name: "AZURE_TENANT_ID"},
				{Purpose: upstream.CredentialPurposeClientID, Store: "env", Name: "AZURE_CLIENT_ID"},
				{Purpose: upstream.CredentialPurposeClientSecret, Store: "env", Name: "AZURE_CLIENT_SECRET"},
			},
		},
		{
			name:   "entra certificate",
			typeID: upstream.CredentialEntraID,
			refs: []upstream.CredentialReference{
				{Purpose: upstream.CredentialPurposeTenantID, Store: "env", Name: "AZURE_TENANT_ID"},
				{Purpose: upstream.CredentialPurposeClientID, Store: "env", Name: "AZURE_CLIENT_ID"},
				{Purpose: upstream.CredentialPurposeCertificate, Store: "env", Name: "AZURE_CLIENT_CERTIFICATE_REF"},
			},
		},
		{
			name:   "entra workload identity without client or tenant",
			typeID: upstream.CredentialEntraID,
			refs:   []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeWorkloadIdentity, Store: "workload_identity", Name: "azure-default"}},
			prepare: func(profile *upstream.CredentialProfile) {
				profile.Lifecycle = upstream.CredentialLifecycleWorkloadIdentity
			},
		},
		{
			name:   "entra mixed workload and secret",
			typeID: upstream.CredentialEntraID,
			refs: []upstream.CredentialReference{
				{Purpose: upstream.CredentialPurposeWorkloadIdentity, Store: "workload_identity", Name: "azure-default"},
				{Purpose: upstream.CredentialPurposeClientSecret, Store: "env", Name: "AZURE_CLIENT_SECRET"},
			},
			wantErr: true,
		},
		{
			name:   "entra missing tenant",
			typeID: upstream.CredentialEntraID,
			refs: []upstream.CredentialReference{
				{Purpose: upstream.CredentialPurposeClientID, Store: "env", Name: "AZURE_CLIENT_ID"},
				{Purpose: upstream.CredentialPurposeClientSecret, Store: "env", Name: "AZURE_CLIENT_SECRET"},
			},
			wantErr: true,
		},
		{
			name:   "entra secret and certificate conflict",
			typeID: upstream.CredentialEntraID,
			refs: []upstream.CredentialReference{
				{Purpose: upstream.CredentialPurposeTenantID, Store: "env", Name: "AZURE_TENANT_ID"},
				{Purpose: upstream.CredentialPurposeClientID, Store: "env", Name: "AZURE_CLIENT_ID"},
				{Purpose: upstream.CredentialPurposeClientSecret, Store: "env", Name: "AZURE_CLIENT_SECRET"},
				{Purpose: upstream.CredentialPurposeCertificate, Store: "env", Name: "AZURE_CLIENT_CERTIFICATE_REF"},
			},
			wantErr: true,
		},
		{
			name:   "sigv4 key pair",
			typeID: upstream.CredentialSigV4,
			refs: []upstream.CredentialReference{
				{Purpose: upstream.CredentialPurposeAccessKeyID, Store: "env", Name: "AWS_ACCESS_KEY_ID"},
				{Purpose: upstream.CredentialPurposeSecretAccessKey, Store: "env", Name: "AWS_SECRET_ACCESS_KEY"},
			},
			prepare: func(profile *upstream.CredentialProfile) {
				profile.AuthPlacement = upstream.AuthPlacementRequestSigning
				profile.AuthHeader = ""
				profile.AuthScheme = ""
				profile.SigV4Service = "bedrock"
			},
		},
		{
			name:   "sigv4 mixed profile and workload",
			typeID: upstream.CredentialSigV4,
			refs: []upstream.CredentialReference{
				{Purpose: upstream.CredentialPurposeProfile, Store: "env", Name: "AWS_PROFILE"},
				{Purpose: upstream.CredentialPurposeWorkloadIdentity, Store: "workload_identity", Name: "aws-default"},
			},
			prepare: func(profile *upstream.CredentialProfile) {
				profile.AuthPlacement = upstream.AuthPlacementRequestSigning
				profile.AuthHeader = ""
				profile.AuthScheme = ""
				profile.SigV4Service = "bedrock"
			},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			route := validRoute()
			route.Credential.Type = test.typeID
			route.Credential.References = test.refs
			route.Credential.AuthPlacement = upstream.AuthPlacementSDK
			route.Credential.AuthHeader = ""
			route.Credential.AuthScheme = ""
			route.Credential.Scopes = []string{"models.invoke"}
			route.Credential.Lifecycle = upstream.CredentialLifecycleRenewable
			if test.prepare != nil {
				test.prepare(&route.Credential)
			}
			err := route.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

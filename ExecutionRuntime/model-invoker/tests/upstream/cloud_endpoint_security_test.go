package upstream_test

import (
	"net/http/httptest"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	azure "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/azureopenai"
	mantle "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/bedrockmantle"
	bedrock "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/bedrockruntime"
	vertex "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/vertex"
)

func TestCloudPublicConfigsRejectUnsafeDynamicEndpointFields(t *testing.T) {
	dnsUnsafe := []string{"a.b", "value.", "UPPER", " value", "value ", "value@x", "value:443", "value?x", "value#x", `value\\x`, "..", "%2f", ""}
	for _, value := range dnsUnsafe {
		t.Run("azure region/"+value, func(t *testing.T) {
			_, err := azure.New(azure.Config{ResourceEndpoint: "https://resource.openai.azure.com", Region: value, DeploymentName: "deploy-a", CredentialMode: azure.CredentialAPIKey, APIKey: "offline"})
			if err == nil {
				t.Fatal("Azure accepted unsafe region")
			}
		})
		t.Run("vertex project/"+value, func(t *testing.T) {
			_, err := vertex.New(vertex.Config{Project: value, Location: "us-central1", DeploymentMode: vertex.DeploymentServerless, DeploymentRef: "publisher-model", CredentialMode: vertex.CredentialAPIKey, APIKey: "offline"})
			if err == nil {
				t.Fatal("Vertex accepted unsafe project")
			}
		})
		t.Run("vertex location/"+value, func(t *testing.T) {
			_, err := vertex.New(vertex.Config{Project: "project-a", Location: value, DeploymentMode: vertex.DeploymentServerless, DeploymentRef: "publisher-model", CredentialMode: vertex.CredentialAPIKey, APIKey: "offline"})
			if err == nil {
				t.Fatal("Vertex accepted unsafe location")
			}
		})
		t.Run("bedrock runtime region/"+value, func(t *testing.T) {
			_, err := bedrock.New(bedrock.Config{Region: value, CredentialMode: bedrock.CredentialSigV4, AccessKeyID: "AKIAOFFLINE", SecretAccessKey: "offline"})
			if err == nil {
				t.Fatal("Bedrock Runtime accepted unsafe region")
			}
		})
		t.Run("bedrock mantle region/"+value, func(t *testing.T) {
			_, err := mantle.New(mantle.Config{Region: value, ProjectRef: "project-a", CredentialMode: mantle.CredentialAPIKey, APIKey: "offline"})
			if err == nil {
				t.Fatal("Bedrock Mantle accepted unsafe region")
			}
		})
	}
	for _, value := range []string{"", ".", "..", "a/b", `a\\b`, "%2f", "a?b", "a#b", "a b", " a", "a ", "a\n", string(make([]byte, 129))} {
		t.Run("azure deployment/"+value, func(t *testing.T) {
			_, err := azure.New(azure.Config{ResourceEndpoint: "https://resource.openai.azure.com", Region: "eastus", DeploymentName: value, CredentialMode: azure.CredentialAPIKey, APIKey: "offline"})
			if err == nil {
				t.Fatal("Azure accepted unsafe deployment")
			}
		})
		t.Run("vertex deployment/"+value, func(t *testing.T) {
			_, err := vertex.New(vertex.Config{Project: "project-a", Location: "us-central1", DeploymentMode: vertex.DeploymentServerless, DeploymentRef: value, CredentialMode: vertex.CredentialAPIKey, APIKey: "offline"})
			if err == nil {
				t.Fatal("Vertex accepted unsafe deployment reference")
			}
		})
	}
	if _, err := azure.New(azure.Config{ResourceEndpoint: "https://resource.openai.azure.com", Region: "eastus", DeploymentName: "Deploy.v2_A", CredentialMode: azure.CredentialAPIKey, APIKey: "offline"}); err != nil {
		t.Fatalf("Azure rejected safe deployment segment: %v", err)
	}
	if _, err := vertex.New(vertex.Config{Project: "project-a", Location: "us-central1", DeploymentMode: vertex.DeploymentServerless, DeploymentRef: "Publisher.Model_v2", CredentialMode: vertex.CredentialAPIKey, APIKey: "offline"}); err != nil {
		t.Fatalf("Vertex rejected safe deployment segment: %v", err)
	}

	for _, endpoint := range []string{
		"https://resource.openai.azure.com.evil", "https://a.b.openai.azure.com", "https://resource.openai.azure.com:444",
	} {
		if _, err := azure.New(azure.Config{ResourceEndpoint: endpoint, Region: "eastus", DeploymentName: "deploy-a", CredentialMode: azure.CredentialAPIKey, APIKey: "offline"}); err == nil {
			t.Fatalf("Azure accepted %q", endpoint)
		}
	}
	if _, err := vertex.New(vertex.Config{Project: "project-a", Location: "us-central1", DeploymentMode: vertex.DeploymentServerless, DeploymentRef: "publisher-model", BaseURL: "https://evil.example", CredentialMode: vertex.CredentialAPIKey, APIKey: "offline"}); err == nil {
		t.Fatal("Vertex accepted arbitrary remote BaseURL")
	}
	if _, err := bedrock.New(bedrock.Config{Region: "us-east-1", BaseURL: "https://evil.example", CredentialMode: bedrock.CredentialSigV4, AccessKeyID: "AKIAOFFLINE", SecretAccessKey: "offline"}); err == nil {
		t.Fatal("Bedrock Runtime accepted arbitrary remote BaseURL")
	}
	if _, err := mantle.New(mantle.Config{Region: "us-east-1", ProjectRef: "project-a", BaseURL: "https://evil.example", CredentialMode: mantle.CredentialAPIKey, APIKey: "offline"}); err == nil {
		t.Fatal("Bedrock Mantle accepted arbitrary remote BaseURL")
	}
}

func TestCloudConstructionReceiptsComeFromAdapterOwnedBindings(t *testing.T) {
	server := httptest.NewServer(nil)
	defer server.Close()

	azureAdapter, err := azure.New(azure.Config{ResourceEndpoint: server.URL, Region: "eastus", DeploymentName: "deploy-a", CredentialMode: azure.CredentialAPIKey, APIKey: "offline", EnableLegacy: true, LegacyAPIVersion: "2024-10-21", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	assertReceipt(t, azureAdapter, modelinvoker.ProtocolResponses, server.URL+"/openai/v1", server.URL+"/openai/v1")
	assertReceipt(t, azureAdapter, modelinvoker.ProtocolChatCompletions, server.URL+"/openai/deployments", server.URL+"/openai/deployments/deploy-a")
	for _, adjacent := range []string{server.URL + "/openai/deployments-x", server.URL + "/openai/deployments?", server.URL + "/OPENAI/deployments", server.URL + ":443/openai/deployments"} {
		if endpoint, ok := azureAdapter.CandidateBindingEndpoint(modelinvoker.ProtocolChatCompletions, adjacent); ok {
			t.Fatalf("Azure receipt selected legacy for %q: %q", adjacent, endpoint)
		}
	}

	vertexAdapter, err := vertex.New(vertex.Config{Project: "project-a", Location: "us-central1", DeploymentMode: vertex.DeploymentServerless, DeploymentRef: "publisher-model", BaseURL: server.URL, CredentialMode: vertex.CredentialAPIKey, APIKey: "offline", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	assertReceipt(t, vertexAdapter, modelinvoker.ProtocolGenerateContent, server.URL+"/v1", server.URL+"/v1")
	assertReceipt(t, vertexAdapter, modelinvoker.ProtocolChatCompletions, server.URL+"/v1beta1/projects", server.URL+"/v1beta1/projects/project-a/locations/us-central1/endpoints/openapi")

	runtimeAdapter, err := bedrock.New(bedrock.Config{Region: "us-east-1", BaseURL: server.URL, CredentialMode: bedrock.CredentialSigV4, AccessKeyID: "AKIAOFFLINE", SecretAccessKey: "offline", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	assertReceipt(t, runtimeAdapter, modelinvoker.ProtocolBedrockConverse, server.URL, server.URL)

	mantleAdapter, err := mantle.New(mantle.Config{Region: "us-east-1", ProjectRef: "project-a", BaseURL: server.URL, CredentialMode: mantle.CredentialAPIKey, APIKey: "offline", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	assertReceipt(t, mantleAdapter, modelinvoker.ProtocolResponses, server.URL+"/openai/v1", server.URL+"/openai/v1")
}

func assertReceipt(t *testing.T, value adaptercore.CandidateBindingReceipt, protocolID modelinvoker.Protocol, requested, want string) {
	t.Helper()
	got, ok := value.CandidateBindingEndpoint(protocolID, requested)
	if !ok || got != want {
		t.Fatalf("receipt(%q, %q) = %q/%v, want %q/true", protocolID, requested, got, ok, want)
	}
}

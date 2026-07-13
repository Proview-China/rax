package routegateway_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/relaycompat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestOptionalRelayFactoryConstructsEverySupportedProtocolWithoutJoiningBuiltins(t *testing.T) {
	builtins, err := routegateway.NewBuiltinFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtins.Get(relaycompat.ProviderID); err == nil {
		t.Fatal("third-party relay factory was silently enabled in the default registry")
	}
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	for _, test := range []struct {
		protocol modelinvoker.Protocol
		version  string
		endpoint string
	}{
		{modelinvoker.ProtocolChatCompletions, "", server.URL + "/v1"},
		{modelinvoker.ProtocolResponses, "", server.URL + "/v1"},
		{modelinvoker.ProtocolMessages, "", server.URL},
		{modelinvoker.ProtocolGenerateContent, "v1beta", server.URL + "/v1beta"},
	} {
		t.Run(string(test.protocol), func(t *testing.T) {
			routeID := upstream.RouteID("relay.test." + string(test.protocol))
			profileID := upstream.CredentialProfileID("relay.test.key." + string(test.protocol))
			secret, err := routegateway.NewSecretMaterial(profileID, upstream.CredentialAPIKey, "test-v1", time.Now().Add(time.Minute), map[upstream.CredentialPurpose][]byte{
				upstream.CredentialPurposeAPIKey: []byte("relay-factory-test-key"),
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := routegateway.NewRelayCompatFactory().Build(context.Background(), routegateway.FactoryInput{
				Entry: catalog.Entry{
					ID: routeID,
					Route: upstream.UpstreamRoute{
						ID: routeID, Model: upstream.ModelIdentity{CanonicalFamily: "test", ProviderModelRef: "relay-model"},
						Protocol: upstream.ProtocolBinding{ID: upstream.ProtocolID(test.protocol), APIVersion: test.version},
					},
					Implementation: catalog.Implementation{AdapterID: string(relaycompat.ProviderID)},
				},
				Binding: routegateway.RuntimeBinding{RouteID: routeID}, Endpoint: test.endpoint, Secret: secret,
				HTTPClient: server.Client(),
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Provider == nil || result.Closer == nil || result.Provider.ID() != relaycompat.ProviderID || result.Endpoint != test.endpoint {
				t.Fatalf("factory result = provider:%v closer:%v endpoint:%q", result.Provider, result.Closer, result.Endpoint)
			}
			contract, err := result.Provider.Capabilities(context.Background(), modelinvoker.CapabilityQuery{
				Protocol: test.protocol, Endpoint: test.endpoint, Model: "relay-model",
			})
			if err != nil {
				t.Fatal(err)
			}
			if contract[modelinvoker.CapabilityToolCalling].Level != modelinvoker.SupportCompatible {
				t.Fatalf("tool capability = %#v", contract[modelinvoker.CapabilityToolCalling])
			}
			if err := result.Closer.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

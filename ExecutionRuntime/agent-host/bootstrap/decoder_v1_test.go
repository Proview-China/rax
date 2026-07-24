package bootstrap_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/bootstrap"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestBootstrapJSONAndYAMLConvergeV1(t *testing.T) {
	value := validBootstrapV1()
	jsonPayload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	// JSON is a strict subset of YAML 1.2, which also avoids YAML libraries
	// silently changing integer authoring values into floating-point scalars.
	yamlPayload := append([]byte(nil), jsonPayload...)
	fromJSON, err := bootstrap.DecodeJSONV1(jsonPayload)
	if err != nil {
		t.Fatal(err)
	}
	fromYAML, err := bootstrap.DecodeYAMLV1(yamlPayload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromJSON, fromYAML) {
		t.Fatalf("decoded forms differ:\nJSON=%#v\nYAML=%#v", fromJSON, fromYAML)
	}
}

func TestBootstrapStrictDecodeRejectsUnsafeAndOwnerFieldsV1(t *testing.T) {
	cases := map[string][]byte{
		"duplicate": []byte("host_id: host-a\nhost_id: host-b\n"),
		"alias":     []byte("host_id: &host host-a\nlisten_binding_id: *host\n"),
		"merge":     []byte("base: &base {host_id: host-a}\nvalue: {<<: *base}\n"),
		"float":     []byte("created_unix_nano: 1.5\n"),
		"multi":     []byte("host_id: host-a\n---\nhost_id: host-b\n"),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := bootstrap.DecodeYAMLV1(payload); !core.HasCategory(err, core.ErrorInvalidArgument) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	jsonCases := [][]byte{
		[]byte(`{"contract_version":"praxis.agent-host/bootstrap/v1","contract_version":"praxis.agent-host/bootstrap/v1"}`),
		[]byte(`{"contract_version":"praxis.agent-host/bootstrap/v1","constructor":"pkg.New"}`),
		[]byte(`{"contract_version":"praxis.agent-host/bootstrap/v1","secret_value":"plaintext"}`),
	}
	for _, payload := range jsonCases {
		if _, err := bootstrap.DecodeJSONV1(payload); err == nil {
			t.Fatalf("unsafe JSON accepted: %s", payload)
		}
	}
}

func TestBootstrapDecodeBoundsV1(t *testing.T) {
	if _, err := bootstrap.DecodeYAMLV1([]byte(strings.Repeat("a", bootstrap.MaxBootstrapBytesV1+1))); !core.HasReason(err, core.ReasonCanonicalLimitExceeded) {
		t.Fatalf("oversize error = %v", err)
	}
	if _, err := bootstrap.DecodeJSONV1(nil); !core.HasReason(err, core.ReasonCanonicalLimitExceeded) {
		t.Fatalf("empty error = %v", err)
	}
}

func validBootstrapV1() contract.HostBootstrapConfigV1 {
	return contract.HostBootstrapConfigV1{
		ContractVersion:                   contract.HostBootstrapContractVersionV1,
		ObjectKind:                        contract.HostBootstrapObjectKindV1,
		HostID:                            "host-a",
		StatePlaneBindingIDs:              []string{"state-runtime"},
		DefinitionSourceBindingID:         "definition-source",
		CatalogBindingID:                  "catalog",
		ResolutionFactsBindingID:          "resolution-facts",
		SecretBrokerBindingID:             "secret-broker",
		CredentialRegistryBindingID:       "credential-registry",
		ProviderEndpointRegistryBindingID: "provider-registry",
		RuntimeServiceBindingIDs:          []string{"runtime"},
		ApplicationServiceBindingIDs:      []string{"application"},
		HarnessServiceBindingIDs:          []string{"harness"},
		ListenBindingID:                   "listen-local",
		DiagnosticsPolicyBindingID:        "diagnostics",
		ShutdownPolicyBindingID:           "shutdown",
		EnabledControlAPISurfaces:         []string{"validate", "assemble", "run", "inspect", "stop"},
		CreatedUnixNano:                   1_800_000_000_000_000_000,
		NotAfterUnixNano:                  1_800_000_060_000_000_000,
	}
}

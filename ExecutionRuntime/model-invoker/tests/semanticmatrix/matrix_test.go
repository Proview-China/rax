package semanticmatrix_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/semanticmatrix"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

var matrixNow = time.Date(2026, 7, 18, 2, 30, 0, 0, time.UTC)

func TestGeneratedMatrixMatchesCatalogAndRealRuntimeContracts(t *testing.T) {
	routeCatalog, err := catalog.NewDefault(matrixNow)
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := semanticmatrix.Build(routeCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(matrix.Rows) != 39*len(modelinvoker.AllCapabilities()) {
		t.Fatalf("matrix rows = %d", len(matrix.Rows))
	}
	generated, err := matrix.CSV()
	if err != nil {
		t.Fatal(err)
	}
	checkedIn, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".properties.rax", "design", "model-invoker", "semantic-matrix-v1candidate.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, checkedIn) {
		t.Fatal("semantic matrix asset drifted; run cmd/semanticmatrixgen")
	}

	factories, err := routegateway.NewBuiltinFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := routegateway.New(routeCatalog, routegateway.CatalogBindingResolver{}, matrixSecretResolver{}, factories, routegateway.WithClock(func() time.Time { return matrixNow }))
	if err != nil {
		t.Fatal(err)
	}
	defer gateway.Close()
	rows := make(map[upstream.RouteID]map[modelinvoker.Capability]catalog.CapabilitySupport)
	for _, row := range matrix.Rows {
		if rows[row.RouteID] == nil {
			rows[row.RouteID] = map[modelinvoker.Capability]catalog.CapabilitySupport{}
		}
		rows[row.RouteID][row.Capability] = row.Support
	}
	for _, entry := range routeCatalog.Entries() {
		if !entry.Implementation.Callable {
			continue
		}
		model := runtimeModel(entry)
		result, err := gateway.Capabilities(context.Background(), matrixCallForEntry(entry, model))
		if err != nil {
			t.Errorf("Capabilities(%q): %v", entry.ID, err)
			continue
		}
		for _, capability := range modelinvoker.AllCapabilities() {
			catalogSupport := rows[entry.ID][capability]
			runtimeSupport := semanticmatrix.RuntimeSupport(result.Contract, capability)
			if catalogSupport != runtimeSupport {
				t.Errorf("%s %s: catalog=%s runtime=%s", entry.ID, capability, catalogSupport, runtimeSupport)
			}
		}
	}
}

type matrixSecretResolver struct{}

func (matrixSecretResolver) ResolveSecret(_ context.Context, request routegateway.SecretRequest) (routegateway.SecretMaterial, error) {
	values := map[upstream.CredentialPurpose][]byte{}
	switch request.Profile.Type {
	case upstream.CredentialAPIKey:
		value := "offline-api-key"
		if len(request.Profile.KeyPrefixes) > 0 {
			value = request.Profile.KeyPrefixes[0] + "offline-api-key"
		}
		values[upstream.CredentialPurposeAPIKey] = []byte(value)
	case upstream.CredentialBearer:
		values[upstream.CredentialPurposeBearerToken] = []byte("offline-bearer-token")
	case upstream.CredentialSigV4:
		values[upstream.CredentialPurposeAccessKeyID] = []byte("AKIAOFFLINE")
		values[upstream.CredentialPurposeSecretAccessKey] = []byte("offline-secret-access-key")
	case upstream.CredentialADC, upstream.CredentialEntraID, upstream.CredentialOAuth:
		values[upstream.CredentialPurposeBearerToken] = []byte("offline-access-token")
	default:
		return routegateway.SecretMaterial{}, fmt.Errorf("unsupported credential type")
	}
	return routegateway.NewSecretMaterial(request.Profile.ID, request.Profile.Type, "matrix-v1", matrixNow.Add(time.Hour), values)
}

func runtimeModel(entry catalog.Entry) string {
	if entry.ModelDiscovery.Method == catalog.ModelDiscoveryStaticCatalog {
		if len(entry.ModelDiscovery.Aliases) > 0 {
			return entry.ModelDiscovery.Aliases[0].ProviderModelRef
		}
		return entry.Route.Model.ProviderModelRef
	}
	switch entry.Implementation.AdapterID {
	case "deepseek":
		return "deepseek-v4-flash"
	case "kimi":
		return "kimi-k2.5"
	case "zai":
		return "glm-4.7"
	case "minimax":
		return "MiniMax-M3"
	case "xiaomi-mimo":
		return "mimo-v2.5"
	case "qwen":
		return "qwen3.7-max"
	case "xai":
		return "grok-4.5"
	case "azure-openai":
		return entry.Route.Deployment.DeploymentName
	case "kimi-code":
		return "kimi-for-coding"
	case "minimax-token-plan":
		return "MiniMax-M2.7"
	case "mimo-token-plan":
		return "mimo-v2.5"
	case "alibaba-plan":
		return "qwen3.7-max"
	default:
		return "runtime-selected-test-model"
	}
}

func matrixCallForEntry(entry catalog.Entry, model string) modelinvoker.RouteCall {
	call := modelinvoker.RouteCall{RouteID: entry.ID, Invocation: generalInvocation(), Request: modelinvoker.Request{Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "matrix check")}}}
	if entry.Route.Offering.Kind != upstream.OfferingTokenPlan && entry.Route.Offering.Kind != upstream.OfferingCodingPlan {
		return call
	}
	remaining := int64(100)
	call.Invocation = upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationInteractiveCoding, Subject: upstream.SubjectPersonal, Tenancy: upstream.TenancySingle, Execution: upstream.ExecutionForeground, ClientIdentity: upstream.ClientIdentity{Name: "praxis-cli", Version: "v1.0.0", UserAgent: "praxis-cli/v1.0.0", Source: upstream.ClientIdentityBuildManifest}}
	call.EntitlementState = &upstream.EntitlementState{OfferingID: entry.Route.Offering.ID, CredentialProfile: entry.Route.Credential.ID, Status: upstream.EntitlementActive, ObservedAt: matrixNow.Add(-time.Minute), ValidUntil: matrixNow.Add(time.Minute), ExpiresAt: matrixNow.Add(24 * time.Hour), RemainingQuota: &remaining}
	return call
}

func generalInvocation() upstream.InvocationContext {
	return upstream.InvocationContext{Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

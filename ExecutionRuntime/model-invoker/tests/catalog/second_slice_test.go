package catalog_test

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	anthropicadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
	azureadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/azureopenai"
	bedrockmantleadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/bedrockmantle"
	bedrockruntimeadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/bedrockruntime"
	deepseekadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/deepseek"
	geminiadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/gemini"
	kimiadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/kimi"
	mimoadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/mimo"
	minimaxadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/minimax"
	openaiadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/openai"
	qwenadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/qwen"
	vertexadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/vertex"
	xaiadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/xai"
	zaiadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/zai"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestDefaultCatalogRuntimeBindingsDependenciesAndArtifactsMatchLiveModule(t *testing.T) {
	t.Parallel()
	document := catalog.DefaultDocument()
	wantAdapters := map[upstream.ProviderID]string{
		"openai": string(openaiadapter.ProviderID), "anthropic": string(anthropicadapter.ProviderID),
		"google.gemini-developer": string(geminiadapter.ProviderID),
		"aws.bedrock-mantle":      string(bedrockmantleadapter.ProviderID),
		"aws.bedrock-runtime":     string(bedrockruntimeadapter.ProviderID),
		"google.vertex-ai":        string(vertexadapter.ProviderID),
		"azure.openai":            string(azureadapter.ProviderID),
		"deepseek":                string(deepseekadapter.ProviderID),
		"kimi":                    string(kimiadapter.ProviderID),
		"minimax":                 string(minimaxadapter.ProviderID),
		"xiaomi.mimo":             string(mimoadapter.ProviderID),
		"zai":                     string(zaiadapter.ProviderID),
		"alibaba.model-studio":    string(qwenadapter.ProviderID),
		"xai.api":                 string(xaiadapter.ProviderID),
	}
	root := moduleRoot(t)
	moduleVersions := dependencyVersions(t, root)
	for _, entry := range document.Entries {
		if entry.Implementation.Callable {
			if got, want := entry.Implementation.AdapterID, wantAdapters[entry.Route.Provider]; got != want {
				t.Errorf("route %q adapter ID = %q, want live registry ID %q", entry.ID, got, want)
			}
			if _, ok := wantAdapters[entry.Route.Provider]; !ok {
				t.Errorf("route %q has unexpected callable commercial provider %q", entry.ID, entry.Route.Provider)
			}
		} else if entry.Implementation.AdapterID != "" {
			t.Errorf("non-callable route %q unexpectedly has adapter ID %q", entry.ID, entry.Implementation.AdapterID)
		}
		for _, sdk := range entry.SDKs {
			if sdk.Transport != catalog.TransportSDK {
				continue
			}
			if got := moduleVersions[sdk.Package]; got != sdk.Version {
				t.Errorf("route %q SDK %q version = %q, live dependency = %q", entry.ID, sdk.Package, sdk.Version, got)
			}
		}
	}
	if err := catalog.ValidateArtifacts(os.DirFS(root), document); err != nil {
		t.Fatalf("ValidateArtifacts(live module) error = %v", err)
	}
}

func dependencyVersions(t *testing.T, root string) map[string]string {
	t.Helper()
	file, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("open go.mod: %v", err)
	}
	defer file.Close()
	versions := make(map[string]string)
	inRequireBlock := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "//", 2)[0])
		if line == "require (" {
			inRequireBlock = true
			continue
		}
		if inRequireBlock && line == ")" {
			inRequireBlock = false
			continue
		}
		fields := strings.Fields(line)
		if inRequireBlock && len(fields) >= 2 {
			versions[fields[0]] = fields[1]
		} else if len(fields) == 3 && fields[0] == "require" {
			versions[fields[1]] = fields[2]
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan go.mod: %v", err)
	}
	return versions
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve catalog test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
}

func TestDefaultCatalogCapabilitiesExactlyCoverRuntimeVocabulary(t *testing.T) {
	t.Parallel()
	want := make([]string, 0, len(modelinvoker.AllCapabilities()))
	for _, capability := range modelinvoker.AllCapabilities() {
		want = append(want, string(capability))
	}
	sort.Strings(want)
	for _, entry := range catalog.DefaultDocument().Entries {
		got := make([]string, 0, len(entry.Capabilities))
		for _, capability := range entry.Capabilities {
			got = append(got, capability.ID)
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("route %q capabilities = %v, want root vocabulary %v", entry.ID, got, want)
		}
	}
}

func TestValidateTransitionLocksSevenDimensionsAndAdapterID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*catalog.Entry)
	}{
		{name: "model", mutate: func(entry *catalog.Entry) { entry.Route.Model.ProviderModelRef = "other-model" }},
		{name: "provider", mutate: func(entry *catalog.Entry) { entry.Route.Provider = "other" }},
		{name: "offering", mutate: func(entry *catalog.Entry) { entry.Route.Offering.ID = "openai.other" }},
		{name: "deployment", mutate: func(entry *catalog.Entry) { entry.Route.Deployment.ID = "openai.other" }},
		{name: "protocol", mutate: func(entry *catalog.Entry) { entry.Route.Protocol.ID = upstream.ProtocolMessages }},
		{name: "endpoint", mutate: func(entry *catalog.Entry) { entry.Route.Endpoint.ID = "openai.other" }},
		{name: "credential", mutate: func(entry *catalog.Entry) { entry.Route.Credential.ID = "openai.other" }},
		{name: "adapter", mutate: func(entry *catalog.Entry) { entry.Implementation.AdapterID = "other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			previous := catalog.DefaultDocument().Entries[0]
			next := previous.Clone()
			test.mutate(&next)
			if err := catalog.ValidateTransition(previous, next, testNow); !validationHas(err, catalog.IssueInvalidStateTransition) {
				t.Fatalf("ValidateTransition() error = %v, want immutable identity failure", err)
			}
		})
	}
}

func TestCatalogRejectsConflictingSharedDefinitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		code   catalog.IssueCode
		mutate func(*catalog.Entry)
	}{
		{name: "offering", code: catalog.IssueConflictingOfferingID, mutate: func(entry *catalog.Entry) {
			entry.Route.Offering.Entitlement.ClientRestrictions = []string{"different"}
		}},
		{name: "deployment", code: catalog.IssueConflictingDeploymentID, mutate: func(entry *catalog.Entry) {
			entry.Route.Deployment.ResourceRef = "different"
		}},
		{name: "endpoint", code: catalog.IssueConflictingEndpointID, mutate: func(entry *catalog.Entry) {
			entry.Route.Endpoint.BasePath = "/different"
		}},
		{name: "credential", code: catalog.IssueConflictingCredentialID, mutate: func(entry *catalog.Entry) {
			entry.Route.Credential.AuthScheme = "Basic"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := catalog.DefaultDocument()
			test.mutate(&document.Entries[1])
			if err := catalog.Validate(document, testNow); !validationHas(err, test.code) {
				t.Fatalf("Validate() error = %v, want %q", err, test.code)
			}
		})
	}
}

func TestEvidenceTTLBoundariesInvalidationAndDigest(t *testing.T) {
	t.Parallel()
	for class, want := range map[catalog.EvidenceTTLClass]time.Duration{
		catalog.EvidenceTTL7Days:  7 * 24 * time.Hour,
		catalog.EvidenceTTL14Days: 14 * 24 * time.Hour,
		catalog.EvidenceTTL30Days: 30 * 24 * time.Hour,
		catalog.EvidenceTTL90Days: 90 * 24 * time.Hour,
	} {
		if got, ok := class.Duration(); !ok || got != want {
			t.Errorf("TTL %q duration = %v/%v, want %v/true", class, got, ok, want)
		}
	}
	document := catalog.DefaultDocument()
	validUntil := document.Entries[0].Evidence.ValidUntil
	if err := catalog.Validate(document, validUntil.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("Validate(just before expiry) error = %v", err)
	}
	if err := catalog.Validate(document, validUntil); !validationHas(err, catalog.IssueEvidenceExpired) {
		t.Fatalf("Validate(at expiry) error = %v, want expired", err)
	}
	if err := catalog.Validate(document, time.Time{}); !validationHas(err, catalog.IssueInvalidValidationTime) {
		t.Fatalf("Validate(zero time) error = %v, want invalid validation time", err)
	}

	invalidated := catalog.DefaultDocument()
	entry := &invalidated.Entries[0]
	entry.Evidence.Status = catalog.EvidenceInvalidated
	entry.Evidence.InvalidatedBySourceID = "missing.source"
	entry.Implementation.Callable = false
	refreshDigest(t, entry)
	if err := catalog.Validate(invalidated, testNow); !validationHas(err, catalog.IssueInvalidEvidenceStatus) {
		t.Fatalf("Validate(missing invalidating source) error = %v", err)
	}
	entry.Evidence.InvalidatedBySourceID = entry.Sources[0].ID
	refreshDigest(t, entry)
	if err := catalog.Validate(invalidated, testNow); err != nil {
		t.Fatalf("Validate(existing invalidating source) error = %v", err)
	}

	wrongTTL := catalog.DefaultDocument()
	wrongTTL.Entries[0].Evidence.TTLClass = catalog.EvidenceTTL14Days
	refreshDigest(t, &wrongTTL.Entries[0])
	if err := catalog.Validate(wrongTTL, testNow); !validationHas(err, catalog.IssueInvalidEvidenceTTL) {
		t.Fatalf("Validate(TTL mismatch) error = %v, want invalid TTL", err)
	}
}

func TestEvidenceDigestIsDeterministicAndTamperEvident(t *testing.T) {
	t.Parallel()
	entry := catalog.DefaultDocument().Entries[0]
	baseline := entry.Evidence.Digest
	reverse(entry.Sources)
	reverse(entry.Capabilities)
	reverse(entry.StreamEvents)
	reverse(entry.ErrorDialect.RequestIDHeaders)
	got, err := catalog.ComputeEvidenceDigest(entry)
	if err != nil {
		t.Fatalf("ComputeEvidenceDigest(reordered) error = %v", err)
	}
	if got != baseline {
		t.Fatalf("reordered digest = %q, want %q", got, baseline)
	}
	entry.Route.Model.ProviderModelRef = "tampered"
	tampered, err := catalog.ComputeEvidenceDigest(entry)
	if err != nil {
		t.Fatalf("ComputeEvidenceDigest(tampered) error = %v", err)
	}
	if tampered == baseline {
		t.Fatal("source-backed route mutation did not change evidence digest")
	}
	document := catalog.DefaultDocument()
	document.Entries[0].Maturity = catalog.MaturityPreview
	if err := catalog.Validate(document, testNow); !validationHas(err, catalog.IssueInvalidEvidenceDigest) {
		t.Fatalf("Validate(stale digest) error = %v, want invalid digest", err)
	}
}

func reverse[T any](values []T) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func refreshDigest(t *testing.T, entry *catalog.Entry) {
	t.Helper()
	digest, err := catalog.ComputeEvidenceDigest(*entry)
	if err != nil {
		t.Fatalf("ComputeEvidenceDigest() error = %v", err)
	}
	entry.Evidence.Digest = digest
}

func TestUnknownCapabilityAndExplicitRouteMetadata(t *testing.T) {
	t.Parallel()
	document := catalog.DefaultDocument()
	for _, entry := range document.Entries {
		if entry.Maturity == "" || entry.ModelDiscovery.Method == "" || entry.ModelDiscovery.AliasPolicy == "" ||
			len(entry.StreamEvents) == 0 || entry.ErrorDialect.Envelope == "" || entry.ErrorDialect.CodeField == "" ||
			entry.Boundaries.Production == "" || entry.Boundaries.Quota == "" || entry.Boundaries.Expiry == "" {
			t.Errorf("route %q omitted required route metadata: %#v", entry.ID, entry)
		}
	}
	document.Entries[0].Capabilities[0].Support = catalog.CapabilityUnknown
	document.Entries[0].Capabilities[0].Limitations = []string{"awaiting route-specific evidence"}
	refreshDigest(t, &document.Entries[0])
	if err := catalog.Validate(document, testNow); err != nil {
		t.Fatalf("Validate(unknown capability) error = %v", err)
	}
}

func TestSDKOwnershipArtifactPathsAndInjectedFilesystem(t *testing.T) {
	t.Parallel()
	document := catalog.DefaultDocument()
	document.Entries[0].SDKs[0].Ownership = catalog.SDKOwnershipCommunity
	document.Entries[0].SDKs[0].Official = false
	refreshDigest(t, &document.Entries[0])
	if err := catalog.Validate(document, testNow); !validationHas(err, catalog.IssueInvalidSDKMetadata) {
		t.Fatalf("Validate(community callable SDK) error = %v", err)
	}

	unsafe := catalog.DefaultDocument()
	unsafe.Entries[0].Implementation.CodePaths[0] = "../provider/openai"
	if err := catalog.Validate(unsafe, testNow); !validationHas(err, catalog.IssueUnsafeArtifactPath) {
		t.Fatalf("Validate(unsafe path) error = %v", err)
	}

	one := catalog.Document{SchemaVersion: catalog.SchemaVersion, Entries: []catalog.Entry{catalog.DefaultDocument().Entries[0]}}
	filesystem := fstest.MapFS{
		"provider":                               {Mode: fs.ModeDir | 0o755},
		"provider/openai":                        {Mode: fs.ModeDir | 0o755},
		"tests":                                  {Mode: fs.ModeDir | 0o755},
		"tests/openai":                           {Mode: fs.ModeDir | 0o755},
		"tests/openai/provider_contract_test.go": {Data: []byte("package openai_test")},
	}
	if err := catalog.ValidateArtifacts(filesystem, one); err != nil {
		t.Fatalf("ValidateArtifacts(injected fs) error = %v", err)
	}
	delete(filesystem, "tests/openai/provider_contract_test.go")
	if err := catalog.ValidateArtifacts(filesystem, one); !validationHas(err, catalog.IssueMissingArtifact) {
		t.Fatalf("ValidateArtifacts(missing) error = %v, want missing artifact", err)
	}
}

func TestStrictDecodeRejectsDuplicateKeysAndOversizeDocument(t *testing.T) {
	t.Parallel()
	for _, payload := range []string{
		`{"schema_version":"praxis.upstream-catalog/v1","schema_version":"duplicate","entries":[]}`,
		`{"schema_version":"praxis.upstream-catalog/v1","entries":[{"id":"one","id":"two"}]}`,
	} {
		if _, err := catalog.Decode(strings.NewReader(payload)); err == nil || !strings.Contains(err.Error(), "duplicate object key") {
			t.Fatalf("Decode(duplicate) error = %v", err)
		}
	}
	oversize := strings.NewReader(strings.Repeat(" ", int(catalog.MaxDocumentBytes+1)))
	if _, err := catalog.Decode(oversize); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Decode(oversize) error = %v", err)
	}
}

package catalog_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

var testNow = time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)

func validationHas(err error, code catalog.IssueCode) bool {
	var validationError *catalog.ValidationError
	return errors.As(err, &validationError) && validationError.Has(code)
}

func TestDefaultCatalogSeparatesCallableBindingsFromPlannedControlRecords(t *testing.T) {
	t.Parallel()
	catalogSnapshot, err := catalog.NewDefault(testNow)
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	if got := catalogSnapshot.Len(); got != 62 {
		t.Fatalf("Len() = %d, want 39 callable bindings and 23 control-plane records", got)
	}
	wantProtocols := map[upstream.ProtocolID]int{
		upstream.ProtocolResponses: 9, upstream.ProtocolChatCompletions: 16,
		upstream.ProtocolMessages: 7, upstream.ProtocolGenerateContent: 3,
		upstream.ProtocolBedrockConverse: 2, upstream.ProtocolBedrockInvoke: 2,
	}
	gotProtocols := make(map[upstream.ProtocolID]int)
	providers := make(map[upstream.ProviderID]struct{})
	offerings := make(map[upstream.OfferingID]struct{})
	callable := 0
	for _, entry := range catalogSnapshot.Entries() {
		offerings[entry.Route.Offering.ID] = struct{}{}
		if entry.Implementation.Callable {
			callable++
			gotProtocols[entry.Route.Protocol.ID]++
			providers[entry.Route.Provider] = struct{}{}
		}
		if entry.Route.Credential.Type != upstream.CredentialAnonymous && (len(entry.Route.Credential.References) == 0 || entry.Route.Credential.References[0].Name == "") {
			t.Fatalf("route %q has no credential reference", entry.ID)
		}
	}
	if !reflect.DeepEqual(gotProtocols, wantProtocols) {
		t.Fatalf("protocols = %#v, want %#v", gotProtocols, wantProtocols)
	}
	if callable != 39 || len(providers) != 14 {
		t.Fatalf("callable/providers = %d/%#v, want 39 across fourteen catalog providers", callable, providers)
	}
	for _, offering := range []upstream.OfferingID{
		"zai.glm-coding-plan", "kimi.code-membership", "minimax.token-plan", "mimo.token-plan",
		"alibaba.coding-plan", "alibaba.token-plan-team", "xai.consumer-subscription",
	} {
		if _, exists := offerings[offering]; !exists {
			t.Errorf("default catalog is missing control-plane offering %q", offering)
		}
	}
}

func TestCatalogValidationRejectsUnsafeAndIncompleteEntries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		code   catalog.IssueCode
		mutate func(*catalog.Document)
	}{
		{name: "duplicate route", code: catalog.IssueDuplicateRouteID, mutate: func(document *catalog.Document) {
			document.Entries = append(document.Entries, document.Entries[0].Clone())
		}},
		{name: "missing source", code: catalog.IssueMissingSource, mutate: func(document *catalog.Document) { document.Entries[0].Sources = nil }},
		{name: "expired evidence", code: catalog.IssueEvidenceExpired, mutate: func(document *catalog.Document) { document.Entries[0].Evidence.ValidUntil = testNow.Add(-time.Hour) }},
		{name: "invalid evidence status", code: catalog.IssueInvalidEvidenceStatus, mutate: func(document *catalog.Document) { document.Entries[0].Evidence.Status = "freshish" }},
		{name: "invalid implementation status", code: catalog.IssueInvalidImplementationStatus, mutate: func(document *catalog.Document) { document.Entries[0].Implementation.Status = "coded" }},
		{name: "terms blocked callable", code: catalog.IssueTermsBlockedCallable, mutate: func(document *catalog.Document) { document.Entries[0].Evidence.Status = catalog.EvidenceTermsBlocked }},
		{name: "official client callable", code: catalog.IssueUsageBlockedCallable, mutate: func(document *catalog.Document) {
			document.Entries[0].Route.Offering.Entitlement.AllowedUsage = upstream.AllowedUsageOfficialClientOnly
		}},
		{name: "credential endpoint mismatch", code: catalog.IssueInvalidRoute, mutate: func(document *catalog.Document) { document.Entries[0].Route.Credential.Audience = "wrong.example" }},
		{name: "missing test evidence", code: catalog.IssueMissingImplementationEvidence, mutate: func(document *catalog.Document) { document.Entries[0].Implementation.TestEvidence = nil }},
		{name: "missing sdk metadata", code: catalog.IssueInvalidSDKMetadata, mutate: func(document *catalog.Document) { document.Entries[0].SDKs = nil }},
		{name: "invalid capability", code: catalog.IssueInvalidCapabilityMetadata, mutate: func(document *catalog.Document) { document.Entries[0].Capabilities[0].Support = "maybe" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := catalog.DefaultDocument()
			test.mutate(&document)
			err := catalog.Validate(document, testNow)
			if !validationHas(err, test.code) {
				t.Fatalf("Validate() error = %v, want issue %q", err, test.code)
			}
		})
	}
}

func TestCatalogRejectsConflictingOfficialSourceIdentity(t *testing.T) {
	t.Parallel()
	document := catalog.DefaultDocument()
	document.Entries[1].Sources[0].URL = "https://example.invalid/conflict"
	if err := catalog.Validate(document, testNow); !validationHas(err, catalog.IssueConflictingSource) {
		t.Fatalf("Validate() error = %v, want conflicting source", err)
	}
}

func TestCatalogStrictCodecRoundTrip(t *testing.T) {
	t.Parallel()
	document := catalog.DefaultDocument()
	var encoded bytes.Buffer
	if err := catalog.Encode(&encoded, document); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded, err := catalog.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, document) {
		t.Fatal("round-trip document differs")
	}
	for _, malformed := range []string{
		`{"schema_version":"praxis.upstream-catalog/v1","entries":[],"unknown":true}`,
		`{} {}`,
	} {
		if _, err := catalog.Decode(strings.NewReader(malformed)); err == nil {
			t.Fatalf("Decode(%q) unexpectedly succeeded", malformed)
		}
	}
}

func TestCatalogSnapshotDefensiveCopiesAndConcurrentReads(t *testing.T) {
	document := catalog.DefaultDocument()
	snapshot, err := catalog.New(document, testNow)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	originalID := document.Entries[0].ID
	document.Entries[0].Route.Credential.References[0].Name = "MUTATED_INPUT"
	entry, ok := snapshot.Get(originalID)
	if !ok {
		t.Fatalf("Get(%q) not found", originalID)
	}
	if entry.Route.Credential.References[0].Name == "MUTATED_INPUT" {
		t.Fatal("constructor retained caller-owned credential slice")
	}
	entry.Route.Credential.References[0].Name = "MUTATED_OUTPUT"
	again, _ := snapshot.Get(originalID)
	if again.Route.Credential.References[0].Name == "MUTATED_OUTPUT" {
		t.Fatal("Get returned catalog-owned credential slice")
	}

	const workers = 32
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < 200; iteration++ {
				entries := snapshot.Entries()
				entries[0].Capabilities[0].ID = "mutated"
				entries[0].Route.Credential.AllowedEndpointIDs[0] = "mutated"
				if got := snapshot.Len(); got != 62 {
					t.Errorf("Len() = %d, want 62", got)
					return
				}
				if _, ok := snapshot.Get(originalID); !ok {
					t.Errorf("Get(%q) failed", originalID)
					return
				}
			}
		}()
	}
	wait.Wait()
	final, _ := snapshot.Get(originalID)
	if final.Capabilities[0].ID == "mutated" || final.Route.Credential.AllowedEndpointIDs[0] == "mutated" {
		t.Fatal("concurrent returned-copy mutation changed catalog state")
	}
}

func TestStateTransitionsAreReviewedAndMonotonic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		from    catalog.ImplementationStatus
		to      catalog.ImplementationStatus
		wantErr bool
	}{
		{name: "same", from: catalog.ImplementationImplementedOffline, to: catalog.ImplementationImplementedOffline},
		{name: "next", from: catalog.ImplementationImplementedOffline, to: catalog.ImplementationLiveVerified},
		{name: "skip", from: catalog.ImplementationPlanned, to: catalog.ImplementationLiveVerified, wantErr: true},
		{name: "regress", from: catalog.ImplementationLiveVerified, to: catalog.ImplementationImplementedOffline, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			previous := catalog.DefaultDocument().Entries[0]
			next := previous.Clone()
			previous.Implementation.Status = test.from
			next.Implementation.Status = test.to
			if test.to >= catalog.ImplementationLiveVerified {
				next.Implementation.LiveEvidence = []string{"tests/integration/explicit-smoke"}
			}
			err := catalog.ValidateTransition(previous, next, testNow)
			if gotErr := validationHas(err, catalog.IssueInvalidStateTransition); gotErr != test.wantErr {
				t.Fatalf("ValidateTransition() error = %v, invalid transition = %v, want %v", err, gotErr, test.wantErr)
			}
		})
	}
}

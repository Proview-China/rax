package conformance_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/sdk"
)

func TestArtifactRelationRequestIsCoordinateOnly(t *testing.T) {
	typeOf := reflect.TypeOf(ports.CreateArtifactRelationRequestV1{})
	fields := make([]string, 0, typeOf.NumField())
	for i := 0; i < typeOf.NumField(); i++ {
		fields = append(fields, typeOf.Field(i).Name)
	}
	sort.Strings(fields)
	want := []string{
		"ArtifactFactRef", "EvidenceRecordRef", "ExpectedSourceProjectionRef", "IdempotencyKey",
		"Kind", "RelatedFactRef", "RelationID", "Scope",
	}
	sort.Strings(want)
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("caller request field closure changed: got=%v want=%v", fields, want)
	}
	for _, field := range fields {
		name := strings.ToLower(field)
		for _, forbidden := range []string{"storage", "parent", "payload", "trust", "current", "verdict", "outcome"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("caller request contains trusted owner field %s", field)
			}
		}
	}
}

func TestArtifactRelationCapabilityRemainsReferenceOnly(t *testing.T) {
	manifest := conformance.Wave1Manifest()
	if !containsCapabilityV1(manifest.Supported, conformance.CapabilityArtifactRelationV1) {
		t.Fatal("Artifact Relation reference capability is missing")
	}
	if !manifest.ReferenceOnly || manifest.ProductionSLA {
		t.Fatal("Artifact Relation must not upgrade the Wave 1 production claim")
	}
	typeOf := reflect.TypeOf((*sdk.Client)(nil))
	for i := 0; i < typeOf.NumMethod(); i++ {
		name := strings.ToLower(typeOf.Method(i).Name)
		if strings.Contains(name, "attachartifact") || strings.Contains(name, "createartifactrelation") {
			t.Fatalf("read-only SDK exposes direct Artifact write %s", typeOf.Method(i).Name)
		}
	}
}

func containsCapabilityV1(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

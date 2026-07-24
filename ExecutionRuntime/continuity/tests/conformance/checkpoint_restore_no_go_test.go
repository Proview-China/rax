package conformance_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

func TestNoGoCheckpointAndRestoreContractsRemainReferenceOnly(t *testing.T) {
	for _, value := range []any{
		contract.CheckpointManifest{}, contract.SnapshotBinding{}, contract.RestorePlan{}, contract.RecoveryCredentialV1{},
		contract.CheckpointManifestFactV2{}, contract.CheckpointManifestSealFactV2{},
		contract.RestorePlanFactV2{},
	} {
		assertReferenceOnlyType(t, reflect.TypeOf(value), map[reflect.Type]bool{})
	}
	for _, forbidden := range []struct {
		typeOf reflect.Type
		name   string
	}{
		{reflect.TypeOf(contract.CheckpointManifest{}), "RestoreEligible"},
		{reflect.TypeOf(contract.CheckpointManifest{}), "Consistent"},
		{reflect.TypeOf(contract.SnapshotBinding{}), "SnapshotData"},
		{reflect.TypeOf(contract.RestorePlan{}), "Execute"},
		{reflect.TypeOf(contract.RestorePlan{}), "ProviderSession"},
		{reflect.TypeOf(contract.RestorePlan{}), "RuntimeOutcome"},
		{reflect.TypeOf(contract.RecoveryCredentialV1{}), "Secret"},
		{reflect.TypeOf(contract.RecoveryCredentialV1{}), "Permit"},
		{reflect.TypeOf(contract.RecoveryCredentialV1{}), "Fence"},
		{reflect.TypeOf(contract.RecoveryCredentialV1{}), "ProviderSession"},
		{reflect.TypeOf(contract.CheckpointManifestFactV2{}), "Consistent"},
		{reflect.TypeOf(contract.CheckpointManifestFactV2{}), "RestoreEligible"},
		{reflect.TypeOf(contract.CheckpointManifestSealFactV2{}), "RuntimeOutcome"},
		{reflect.TypeOf(contract.CheckpointManifestSealFactV2{}), "ExpiresUnixNano"},
		{reflect.TypeOf(contract.RestorePlanFactV2{}), "Eligibility"},
		{reflect.TypeOf(contract.RestorePlanFactV2{}), "Authorization"},
		{reflect.TypeOf(contract.RestorePlanFactV2{}), "Execute"},
		{reflect.TypeOf(contract.RestorePlanFactV2{}), "Provider"},
	} {
		if _, ok := forbidden.typeOf.FieldByName(forbidden.name); ok {
			t.Fatalf("NO-GO: %s contains executable/owned field %s", forbidden.typeOf.Name(), forbidden.name)
		}
	}
}

func TestNoGoCheckpointRestoreValidationIsPureAndCannotExecute(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	plan := contract.RestorePlan{
		PlanID: "restore-1", RuntimeCheckpointFactRef: "runtime-checkpoint-fact-1",
		ContinuityManifestRef: "manifest-1", SourceInstanceID: "instance-1", SourceInstanceEpoch: 1,
		NewInstanceID: "instance-2", NewInstanceEpoch: 2, NewSandboxLeaseRef: "lease-ref-2",
		RequiredParticipantIDs: []string{"sandbox", "context"},
		CompatibilityFactRefs:  []string{"binding-fact", "authority-fact"},
		ContextMaterialized:    true, RecoveryCredentialRef: "credential-ref-1",
		ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	}
	plan.Digest, _ = plan.CanonicalDigest()
	before := plan
	before.RequiredParticipantIDs = append([]string{}, plan.RequiredParticipantIDs...)
	before.CompatibilityFactRefs = append([]string{}, plan.CompatibilityFactRefs...)
	if err := plan.Validate(now); err != nil {
		t.Fatalf("reference-only plan rejected: %v", err)
	}
	if !reflect.DeepEqual(plan, before) {
		t.Fatalf("validation mutated restore plan: before=%#v after=%#v", before, plan)
	}
	manifest := conformance.Wave1Manifest()
	for _, required := range []string{"continuity/checkpoint-capture", "continuity/restore-execute"} {
		if !containsCapability(manifest.Unsupported, required) {
			t.Fatalf("NO-GO capability %s is not explicitly unsupported", required)
		}
	}
}

func assertReferenceOnlyType(t *testing.T, typeOf reflect.Type, seen map[reflect.Type]bool) {
	t.Helper()
	if seen[typeOf] {
		return
	}
	seen[typeOf] = true
	switch typeOf.Kind() {
	case reflect.Func, reflect.Interface, reflect.Chan, reflect.UnsafePointer:
		t.Fatalf("NO-GO executable/opaque implementation field type %s", typeOf)
	case reflect.Slice, reflect.Array:
		if typeOf.Elem().Kind() == reflect.Uint8 {
			t.Fatalf("NO-GO inline state/blob field type %s", typeOf)
		}
		assertReferenceOnlyType(t, typeOf.Elem(), seen)
	case reflect.Pointer:
		assertReferenceOnlyType(t, typeOf.Elem(), seen)
	case reflect.Struct:
		for i := 0; i < typeOf.NumField(); i++ {
			field := typeOf.Field(i)
			lower := strings.ToLower(field.Name)
			if strings.Contains(lower, "execute") || strings.Contains(lower, "capture") || strings.Contains(lower, "payload") {
				t.Fatalf("NO-GO executable or inline state field %s.%s", typeOf.Name(), field.Name)
			}
			assertReferenceOnlyType(t, field.Type, seen)
		}
	}
}

func containsCapability(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

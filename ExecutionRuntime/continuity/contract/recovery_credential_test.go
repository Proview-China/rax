package contract_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
)

func TestRecoveryCredentialV1ValidatesOnlyForExactPlanShape(t *testing.T) {
	now := time.Date(2026, 7, 17, 0, 30, 0, 0, time.UTC)
	plan := recoveryRestorePlan(t, now)
	credential := recoveryCredentialV1(t, now, plan)

	if err := credential.ValidateForPlan(plan, now); err != nil {
		t.Fatalf("valid recovery credential rejected: %v", err)
	}

	drifted := plan
	drifted.NewInstanceID = "instance-3"
	drifted.Digest = digestRestore(t, drifted)
	if err := credential.ValidateForPlan(drifted, now); !contract.HasCode(err, contract.ErrRestoreIncompatible) {
		t.Fatalf("credential accepted a different restore plan: %v", err)
	}

	drifted = plan
	drifted.RecoveryCredentialRef = "credential-2"
	drifted.Digest = digestRestore(t, drifted)
	if err := credential.ValidateForPlan(drifted, now); !contract.HasCode(err, contract.ErrRestoreIncompatible) {
		t.Fatalf("credential accepted a different credential ref: %v", err)
	}
}

func TestRecoveryCredentialV1CanonicalizesActionsAndClonesWithoutAlias(t *testing.T) {
	now := time.Date(2026, 7, 17, 0, 30, 0, 0, time.UTC)
	plan := recoveryRestorePlan(t, now)
	a := recoveryCredentialV1(t, now, plan)
	b := a.Clone()
	b.AllowedActions = []contract.RecoveryActionV1{
		contract.RecoveryActionStageV1,
		contract.RecoveryActionInspectV1,
	}

	da, err := a.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	db, err := b.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	if da != db {
		t.Fatalf("action order changed canonical digest: %s != %s", da, db)
	}

	clone := a.Clone()
	clone.AllowedActions[0] = contract.RecoveryActionStageV1
	if a.AllowedActions[0] != contract.RecoveryActionInspectV1 {
		t.Fatal("clone exposed an allowed-actions alias")
	}
}

func TestRecoveryCredentialV1FailsClosedForExpiryRevocationAndDrift(t *testing.T) {
	now := time.Date(2026, 7, 17, 0, 30, 0, 0, time.UTC)
	plan := recoveryRestorePlan(t, now)
	base := recoveryCredentialV1(t, now, plan)

	tests := []struct {
		name   string
		mutate func(*contract.RecoveryCredentialV1)
		at     time.Time
		code   contract.ErrorCode
	}{
		{name: "not yet issued", at: now.Add(-2 * time.Minute), code: contract.ErrRestoreIncompatible},
		{name: "expired boundary", at: time.Unix(0, base.ExpiresUnixNano), code: contract.ErrRestoreIncompatible},
		{name: "revoked", at: now, code: contract.ErrRestoreIncompatible, mutate: func(c *contract.RecoveryCredentialV1) { c.RevocationRef = "revocation-fact-1" }},
		{name: "unknown action", at: now, code: contract.ErrInvalidArgument, mutate: func(c *contract.RecoveryCredentialV1) { c.AllowedActions = []contract.RecoveryActionV1{"dispatch"} }},
		{name: "empty action set", at: now, code: contract.ErrInvalidArgument, mutate: func(c *contract.RecoveryCredentialV1) { c.AllowedActions = nil }},
		{name: "scope drift", at: now, code: contract.ErrInvalidArgument, mutate: func(c *contract.RecoveryCredentialV1) { c.SubjectScope.ExecutionScopeDigest = "" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credential := base.Clone()
			if test.mutate != nil {
				test.mutate(&credential)
				credential.Digest = ""
				credential.Digest, _ = credential.CanonicalDigest()
			}
			if err := credential.Validate(test.at); !contract.HasCode(err, test.code) {
				t.Fatalf("expected %s, got %v", test.code, err)
			}
		})
	}
}

func TestRecoveryCredentialV1RejectsDigestTamperAndCarriesNoSecretPayload(t *testing.T) {
	now := time.Date(2026, 7, 17, 0, 30, 0, 0, time.UTC)
	plan := recoveryRestorePlan(t, now)
	credential := recoveryCredentialV1(t, now, plan)
	credential.ParticipantSetDigest = "different-participant-set"
	if err := credential.Validate(now); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("tampered credential digest accepted: %v", err)
	}

	typeOf := reflect.TypeOf(contract.RecoveryCredentialV1{})
	for i := 0; i < typeOf.NumField(); i++ {
		name := strings.ToLower(typeOf.Field(i).Name)
		if strings.Contains(name, "secret") || strings.Contains(name, "token") || strings.Contains(name, "material") {
			t.Fatalf("credential embeds secret-bearing field %s", typeOf.Field(i).Name)
		}
	}
}

func recoveryCredentialV1(t *testing.T, now time.Time, plan contract.RestorePlan) contract.RecoveryCredentialV1 {
	t.Helper()
	credential := contract.RecoveryCredentialV1{
		CredentialID:         plan.RecoveryCredentialRef,
		Revision:             1,
		CheckpointDigest:     "checkpoint-digest-1",
		ManifestDigest:       "manifest-digest-1",
		RestorePlanDigest:    plan.Digest,
		SubjectScope:         testkit.Scope(),
		AuthorityRef:         "authority-fact-1",
		PolicyRef:            "policy-fact-1",
		ReviewRef:            "review-fact-1",
		AllowedActions:       []contract.RecoveryActionV1{contract.RecoveryActionInspectV1, contract.RecoveryActionStageV1},
		ParticipantSetDigest: "participant-set-digest-1",
		IssuedUnixNano:       now.Add(-time.Minute).UnixNano(),
		ExpiresUnixNano:      now.Add(time.Minute).UnixNano(),
	}
	credential.Digest, _ = credential.CanonicalDigest()
	return credential
}

func recoveryRestorePlan(t *testing.T, now time.Time) contract.RestorePlan {
	t.Helper()
	plan := contract.RestorePlan{
		PlanID: "restore-credential-plan-1", RuntimeCheckpointFactRef: "runtime-checkpoint-1",
		ContinuityManifestRef: "manifest-1", SourceInstanceID: "instance-1",
		SourceInstanceEpoch: 3, NewInstanceID: "instance-2", NewInstanceEpoch: 4,
		NewSandboxLeaseRef: "lease-2", RequiredParticipantIDs: []string{"sandbox"},
		CompatibilityFactRefs: []string{"binding-current"}, ContextMaterialized: true,
		RecoveryCredentialRef: "credential-1", ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	}
	plan.Digest = digestRestore(t, plan)
	return plan
}

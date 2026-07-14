package ports_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewSubjectAndPolicyCandidateDigestsArePreallocationSafe(t *testing.T) {
	t.Parallel()
	now := time.Unix(45_000, 0)
	intent, _, _, _, _ := effectPortFixtureV2(t, now)
	intent.CredentialLeases = []ports.CredentialLeaseRefV2{}
	subject, err := intent.ReviewSubjectDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	filled := intent
	filled.Review.Digest = portEffectDigestV2(t, "new-candidate-digest")
	filled.Review.PolicyDigest = portEffectDigestV2(t, "new-review-policy-digest")
	filled.Policy.Digest = portEffectDigestV2(t, "new-dispatch-policy-digest")
	if got, err := filled.ReviewSubjectDigestV2(); err != nil || got != subject {
		t.Fatalf("computed Review and Policy digests must not create a subject cycle: %v %s", err, got)
	}
	persisted := intent
	persisted.CredentialLeases = nil
	if got, err := persisted.ReviewSubjectDigestV2(); err != nil || got != subject {
		t.Fatalf("nil/empty persistence round-trip must preserve subject identity: %v %s", err, got)
	}
	for _, mutate := range []func(*ports.EffectIntentV2){
		func(v *ports.EffectIntentV2) { v.Review.Ref = "review-other" },
		func(v *ports.EffectIntentV2) { v.Review.Revision++ },
		func(v *ports.EffectIntentV2) { v.Target = "remote://other" },
	} {
		drifted := intent
		mutate(&drifted)
		if got, err := drifted.ReviewSubjectDigestV2(); err != nil || got == subject {
			t.Fatalf("case locator and non-Review fields must remain in subject digest: %v %s", err, got)
		}
	}

	pendingPolicy := intent
	pendingPolicy.Policy.Digest = ""
	candidate, err := pendingPolicy.PolicyCandidateDigestV2()
	if err != nil {
		t.Fatalf("preallocated policy ref/revision must permit candidate digest before fact digest exists: %v", err)
	}
	pendingPolicy.Policy.Digest = portEffectDigestV2(t, "final-policy")
	if got, err := pendingPolicy.PolicyCandidateDigestV2(); err != nil || got != candidate {
		t.Fatalf("filling final policy digest must preserve candidate identity: %v %s", err, got)
	}
	missingRef := pendingPolicy
	missingRef.Policy.Ref = ""
	missingRef.Policy.Digest = ""
	if _, err := missingRef.PolicyCandidateDigestV2(); err == nil {
		t.Fatal("policy candidate must still require preallocated ref and revision")
	}
}

func TestReviewEvidenceCanonicalOrderAndDuplicateKeysFailClosed(t *testing.T) {
	t.Parallel()
	one := ports.ReviewEvidenceRefV2{Ref: "evidence-1", Classification: "custom/evidence", Digest: portEffectDigestV2(t, "evidence-1")}
	two := ports.ReviewEvidenceRefV2{Ref: "evidence-2", Classification: "custom/evidence", Digest: portEffectDigestV2(t, "evidence-2")}
	if _, err := ports.DigestReviewEvidenceV2([]ports.ReviewEvidenceRefV2{two, one}); !core.HasReason(err, core.ReasonInvalidCanonicalForm) {
		t.Fatalf("out-of-order evidence must fail canonical validation: %v", err)
	}
	if _, err := ports.DigestReviewEvidenceV2([]ports.ReviewEvidenceRefV2{one, one}); !core.HasReason(err, core.ReasonInvalidCanonicalForm) {
		t.Fatalf("duplicate evidence key must fail canonical validation: %v", err)
	}
}

func FuzzReviewEvidenceCanonicalSetV2(f *testing.F) {
	f.Add(uint8(1), uint8(2))
	f.Add(uint8(7), uint8(7))
	f.Fuzz(func(t *testing.T, left, right uint8) {
		makeEvidence := func(value uint8) ports.ReviewEvidenceRefV2 {
			ref := fmt.Sprintf("evidence-%03d", value)
			return ports.ReviewEvidenceRefV2{Ref: ref, Classification: "custom/evidence", Digest: core.DigestBytes([]byte(ref))}
		}
		values := []ports.ReviewEvidenceRefV2{makeEvidence(left), makeEvidence(right)}
		sort.Slice(values, func(i, j int) bool { return values[i].Ref < values[j].Ref })
		_, err := ports.DigestReviewEvidenceV2(values)
		if left == right {
			if err == nil {
				t.Fatal("duplicate canonical key must never hash")
			}
			return
		}
		if err != nil {
			t.Fatalf("sorted unique evidence must hash deterministically: %v", err)
		}
		first, _ := ports.DigestReviewEvidenceV2(values)
		second, _ := ports.DigestReviewEvidenceV2(append([]ports.ReviewEvidenceRefV2{}, values...))
		if first != second {
			t.Fatal("same canonical evidence set produced different digest")
		}
	})
}

func TestExecutionScopeSemanticEqualityIgnoresPointerIdentityButNotLeaseValue(t *testing.T) {
	t.Parallel()
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: portEffectDigestV2(t, "plan")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox-1", Epoch: 1}, AuthorityEpoch: 1}
	clone := scope
	lease := *scope.SandboxLease
	clone.SandboxLease = &lease
	if !ports.SameExecutionScopeV2(scope, clone) {
		t.Fatal("sandbox lease clone must compare by governed value")
	}
	clone.SandboxLease.Epoch++
	if ports.SameExecutionScopeV2(scope, clone) {
		t.Fatal("sandbox lease epoch drift must change execution scope")
	}
}

func TestReviewCanonicalEmptyCollectionsAreStable(t *testing.T) {
	t.Parallel()
	for _, digest := range []func() (core.Digest, core.Digest, error){
		func() (core.Digest, core.Digest, error) {
			a, err := ports.DigestReviewEvidenceV2(nil)
			if err != nil {
				return "", "", err
			}
			b, err := ports.DigestReviewEvidenceV2([]ports.ReviewEvidenceRefV2{})
			return a, b, err
		},
		func() (core.Digest, core.Digest, error) {
			a, err := ports.DigestReviewConditionsV2(nil)
			if err != nil {
				return "", "", err
			}
			b, err := ports.DigestReviewConditionsV2([]ports.ReviewConditionV2{})
			return a, b, err
		},
		func() (core.Digest, core.Digest, error) {
			a, err := ports.DigestConditionProofsV2(nil)
			if err != nil {
				return "", "", err
			}
			b, err := ports.DigestConditionProofsV2([]ports.ReviewConditionProofV2{})
			return a, b, err
		},
	} {
		left, right, err := digest()
		if err != nil || left != right {
			t.Fatalf("nil and empty canonical sets must be identical: %v %s %s", err, left, right)
		}
	}
}

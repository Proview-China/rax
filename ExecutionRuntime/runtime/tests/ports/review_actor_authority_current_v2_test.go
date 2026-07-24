package ports_test

import (
	"encoding/json"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"strings"
	"testing"
	"time"
)

func TestDispatchAuthorityFactV3SealExactRunCurrentAndClone(t *testing.T) {
	now := time.Unix(2_800_000_000, 0)
	fact := dispatchAuthorityFactV3(t, now, 1, 1, "run-a", true)
	if err := fact.ValidateCurrent(fact.Ref, fact.Scope, "run-a", fact.ActionScopeDigest, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := fact.ValidateCurrent(fact.Ref, fact.Scope, "run-b", fact.ActionScopeDigest, now.Add(time.Second)); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("cross-run Authority accepted: %v", err)
	}
	clone := fact.Clone()
	clone.Scope.SandboxLease.Epoch++
	if clone.Scope.SandboxLease.Epoch == fact.Scope.SandboxLease.Epoch {
		t.Fatal("Dispatch Authority clone leaked SandboxLease alias")
	}
	if err := fact.ValidateCurrent(fact.Ref, fact.Scope, fact.RunID, fact.ActionScopeDigest, time.Unix(0, fact.ExpiresUnixNano)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL boundary accepted: %v", err)
	}
}
func TestDispatchAuthorityFactV3HardNegativesAndPublishShape(t *testing.T) {
	now := time.Unix(2_800_000_000, 0)
	fact := dispatchAuthorityFactV3(t, now, 1, 1, "run-a", true)
	cases := map[string]func(*ports.DispatchAuthorityFactV3){"run": func(v *ports.DispatchAuthorityFactV3) { v.RunID = "" }, "epoch": func(v *ports.DispatchAuthorityFactV3) { v.Scope.AuthorityEpoch++ }, "action": func(v *ports.DispatchAuthorityFactV3) { v.ActionScopeDigest = "" }, "time": func(v *ports.DispatchAuthorityFactV3) { v.ExpiresUnixNano = v.CheckedUnixNano }, "digest": func(v *ports.DispatchAuthorityFactV3) { v.FactDigest = authorityDigestV3(t, "bad") }, "state": func(v *ports.DispatchAuthorityFactV3) { v.State = "unknown" }}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			v := fact.Clone()
			mutate(&v)
			if v.Validate() == nil {
				t.Fatal("invalid V3 fact accepted")
			}
		})
	}
	if err := (ports.DispatchAuthorityFactPublishRequestV3{Value: fact}).Validate(); err != nil {
		t.Fatal(err)
	}
	next := nextDispatchAuthorityFactV3(t, fact, now.Add(time.Second), true)
	if err := (ports.DispatchAuthorityFactPublishRequestV3{Previous: &fact.Ref, Value: next}).Validate(); err != nil {
		t.Fatal(err)
	}
	rollback := next
	rollback.Ref.Revision = 4
	rollback, err := ports.SealDispatchAuthorityFactV3(rollback)
	if err != nil {
		t.Fatal(err)
	}
	if err := (ports.DispatchAuthorityFactPublishRequestV3{Previous: &next.Ref, Value: rollback}).Validate(); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("revision gap accepted: %v", err)
	}
	runDrift := next
	runDrift.RunID = "run-b"
	if ports.SameDispatchAuthorityStableIdentityV3(fact, runDrift) {
		t.Fatal("cross-run Fact reused stable identity")
	}
}

func TestReviewActorAuthorityCurrentV2SealActorOnlyMinTTLAndAlias(t *testing.T) {
	now := time.Unix(2_800_000_000, 0)
	p := reviewActorAuthorityProjectionV2(t, now)
	var v1 ports.ReviewDecisionAuthorityCurrentProjectionRefV1 = p.Ref
	if v1 != p.Ref {
		t.Fatal("actor V2 ref no longer aliases existing exact authority projection ref")
	}
	if err := p.ValidateCurrent(p.Ref, p.Subject, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if p.ExpiresUnixNano != p.Fact.ExpiresUnixNano-1 {
		t.Fatalf("projection did not preserve explicit source minimum TTL: %d", p.ExpiresUnixNano)
	}
	raw, err := json.Marshal(p.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "assignment") || strings.Contains(string(raw), "reviewer") {
		t.Fatalf("actor-only subject leaked Human fields: %s", raw)
	}
	clone := p.Clone()
	clone.Fact.Scope.SandboxLease.Epoch++
	if clone.Fact.Scope.SandboxLease.Epoch == p.Fact.Scope.SandboxLease.Epoch {
		t.Fatal("actor projection clone leaked source Fact alias")
	}
}
func TestReviewActorAuthorityCurrentV2CrossApplicabilityAndStableID(t *testing.T) {
	now := time.Unix(2_800_000_000, 0)
	p := reviewActorAuthorityProjectionV2(t, now)
	cases := map[string]func(*ports.ReviewActorAuthorityCurrentProjectionV2){"tenant": func(v *ports.ReviewActorAuthorityCurrentProjectionV2) { v.Subject.Target.TenantID = "other" }, "run": func(v *ports.ReviewActorAuthorityCurrentProjectionV2) { v.Subject.Target.RunID = "other" }, "epoch": func(v *ports.ReviewActorAuthorityCurrentProjectionV2) { v.Subject.ActorAuthority.Epoch++ }, "action": func(v *ports.ReviewActorAuthorityCurrentProjectionV2) {
		v.Subject.ActionScopeDigest = authorityDigestV3(t, "other")
	}, "authority ref": func(v *ports.ReviewActorAuthorityCurrentProjectionV2) { v.Subject.ActorAuthority.Ref = "other" }, "ttl": func(v *ports.ReviewActorAuthorityCurrentProjectionV2) { v.ExpiresUnixNano = v.Fact.ExpiresUnixNano + 1 }, "truth": func(v *ports.ReviewActorAuthorityCurrentProjectionV2) { v.Current = false }}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			v := p.Clone()
			mutate(&v)
			if v.Validate() == nil {
				t.Fatal("cross-applicability projection accepted")
			}
		})
	}
	nextSubject := p.Subject
	nextSubject.ActorAuthority.Revision++
	nextSubject.ActorAuthority.Digest = authorityDigestV3(t, "renewed")
	nextSubject.ActorAuthority.Epoch++
	id, err := ports.DeriveReviewActorAuthorityCurrentProjectionIDV2(nextSubject)
	if err != nil || id != p.Ref.ID {
		t.Fatalf("actor renewal changed stable ID: %q %v", id, err)
	}
	targetDrift := nextSubject
	targetDrift.Target.Revision++
	id, err = ports.DeriveReviewActorAuthorityCurrentProjectionIDV2(targetDrift)
	if err != nil || id == p.Ref.ID {
		t.Fatalf("Target drift reused stable ID: %q %v", id, err)
	}
}

func reviewActorAuthorityProjectionV2(t *testing.T, now time.Time) ports.ReviewActorAuthorityCurrentProjectionV2 {
	t.Helper()
	fact := dispatchAuthorityFactV3(t, now, 1, 1, "run-a", true)
	subject := ports.ReviewActorAuthorityCurrentSubjectV2{Target: ports.ReviewDecisionTargetRefV1{TenantID: fact.Scope.Identity.TenantID, ID: "target", Revision: 3, Digest: authorityDigestV3(t, "target"), RunID: fact.RunID}, ActorAuthority: fact.Ref, ActionScopeDigest: fact.ActionScopeDigest}
	p, err := ports.SealReviewActorAuthorityCurrentProjectionV2(ports.ReviewActorAuthorityCurrentProjectionV2{Ref: ports.ReviewActorAuthorityCurrentProjectionRefV2{Revision: 1}, Subject: subject, Fact: fact, State: ports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: fact.ExpiresUnixNano - 1})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func dispatchAuthorityFactV3(t *testing.T, now time.Time, revision core.Revision, epoch core.Epoch, run core.AgentRunID, active bool) ports.DispatchAuthorityFactV3 {
	t.Helper()
	tenant := core.TenantID("tenant-authority-v3")
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: tenant, ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: authorityDigestV3(t, "plan")}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox", Epoch: 1}, AuthorityEpoch: epoch}
	state := ports.AuthorityFactActive
	if !active {
		state = ports.AuthorityFactRevoked
	}
	f, err := ports.SealDispatchAuthorityFactV3(ports.DispatchAuthorityFactV3{Ref: ports.AuthorityBindingRefV2{Ref: "authority", Revision: revision, Epoch: epoch}, Scope: scope, RunID: run, ActionScopeDigest: authorityDigestV3(t, "action"), State: state, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func nextDispatchAuthorityFactV3(t *testing.T, current ports.DispatchAuthorityFactV3, now time.Time, active bool) ports.DispatchAuthorityFactV3 {
	t.Helper()
	next := current.Clone()
	next.Ref.Revision++
	next.Ref.Epoch++
	next.Scope.AuthorityEpoch = next.Ref.Epoch
	next.CheckedUnixNano = now.UnixNano()
	next.ExpiresUnixNano = now.Add(30 * time.Second).UnixNano()
	next.State = ports.AuthorityFactActive
	if !active {
		next.State = ports.AuthorityFactRevoked
	}
	next, err := ports.SealDispatchAuthorityFactV3(next)
	if err != nil {
		t.Fatal(err)
	}
	return next
}
func authorityDigestV3(t *testing.T, value string) core.Digest {
	t.Helper()
	d, err := core.CanonicalJSONDigest("authority-v3-test", "v1", "value", value)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

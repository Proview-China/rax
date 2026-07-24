package contract

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewPhaseSourceRefV1ClosedUnionAndCanonicalDigest(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	session := reviewPhaseTerminalSessionV1(t, now, "contract")
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(session.Run.Scope)
	if err != nil {
		t.Fatal(err)
	}
	run := ReviewRunPhaseSourceRefV1{Run: session.Run, ExecutionScopeDigest: scopeDigest, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Phase: session.Phase, Turn: session.Turn, CompletionClaim: session.CompletionClaim}
	ref, err := SealReviewPhaseSourceRefV1(ReviewPhaseSourceRefV1{Kind: ReviewPhaseRunSourceV1, Run: &run})
	if err != nil {
		t.Fatal(err)
	}
	if ref.ContractVersion != ReviewPhaseSourceContractVersionV1 || ref.Digest.Validate() != nil {
		t.Fatalf("sealed ref is incomplete: %#v", ref)
	}
	if ref.ID == "" || ref.Revision != session.Revision {
		t.Fatalf("full exact ref identity is incomplete: %#v", ref)
	}
	nextSession := session.Clone()
	nextSession.Revision++
	nextSession.UpdatedUnixNano++
	nextSession, err = SealGovernedSessionV4(nextSession)
	if err != nil {
		t.Fatal(err)
	}
	nextRun := run
	nextRun.SessionRevision, nextRun.SessionDigest = nextSession.Revision, nextSession.Digest
	nextRef, err := SealReviewPhaseSourceRefV1(ReviewPhaseSourceRefV1{Kind: ReviewPhaseRunSourceV1, Run: &nextRun})
	if err != nil || nextRef.ID != ref.ID || nextRef.Revision != ref.Revision+1 || nextRef.Digest == ref.Digest {
		t.Fatalf("stable ID/monotonic revision failed: next=%#v err=%v", nextRef, err)
	}

	drift := ref.Clone()
	drift.Kind = ReviewPhaseActionSourceV1
	if err := drift.Validate(); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("kind/arm splice error=%v", err)
	}
	twoArms := ref.Clone()
	twoArms.Subagent = &ReviewSubagentPhaseSourceRefV1{ParentRun: session.Run, SourceID: "subagent", SourceRevision: 1, SourceDigest: core.DigestBytes([]byte("subagent"))}
	if err := twoArms.Validate(); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("two-arm union error=%v", err)
	}
	digestDrift := ref.Clone()
	digestDrift.Run.SessionRevision++
	if err := digestDrift.Validate(); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("digest drift error=%v", err)
	}
}

func TestReviewPhaseSourceCurrentProjectionV1RunDeepCloneAndTTL(t *testing.T) {
	now := time.Unix(1_760_000_100, 0)
	session := reviewPhaseTerminalSessionV1(t, now, "projection")
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(session.Run.Scope)
	run := ReviewRunPhaseSourceRefV1{Run: session.Run, ExecutionScopeDigest: scopeDigest, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Phase: session.Phase, Turn: session.Turn, CompletionClaim: session.CompletionClaim}
	ref, _ := SealReviewPhaseSourceRefV1(ReviewPhaseSourceRefV1{Kind: ReviewPhaseRunSourceV1, Run: &run})
	request := ReviewPhaseSourceCurrentRequestV1{Source: ref, RequestedNotAfterUnixNano: now.Add(10 * time.Second).UnixNano()}
	projection, err := SealReviewPhaseSourceCurrentProjectionV1(ReviewPhaseSourceCurrentProjectionV1{Source: ref, Run: session.Run, ExecutionScopeDigest: scopeDigest, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Phase: session.Phase, Turn: session.Turn, CompletionClaim: session.CompletionClaim, RunSession: &session, CheckedUnixNano: now.Add(time.Second).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}, request, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	clone := projection.Clone()
	clone.RunSession.Run.Scope.Identity.ID = "mutated"
	if projection.RunSession.Run.Scope.Identity.ID == "mutated" {
		t.Fatal("projection clone aliases nested ExecutionScope")
	}
	if err := projection.ValidateCurrentFor(request, time.Unix(0, projection.ExpiresUnixNano)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("expiry boundary error=%v", err)
	}
}

func TestReviewPhaseSourceCurrentProjectionV1RejectsForgedLongTTL(t *testing.T) {
	now := time.Unix(1_760_000_150, 0)
	session := reviewPhaseTerminalSessionV1(t, now, "long-ttl")
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(session.Run.Scope)
	run := ReviewRunPhaseSourceRefV1{Run: session.Run, ExecutionScopeDigest: scopeDigest, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Phase: session.Phase, Turn: session.Turn, CompletionClaim: session.CompletionClaim}
	ref, _ := SealReviewPhaseSourceRefV1(ReviewPhaseSourceRefV1{Kind: ReviewPhaseRunSourceV1, Run: &run})
	checked := now.Add(time.Second)
	projection := ReviewPhaseSourceCurrentProjectionV1{Source: ref, Run: session.Run, ExecutionScopeDigest: scopeDigest, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Phase: session.Phase, Turn: session.Turn, CompletionClaim: session.CompletionClaim, RunSession: &session, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: checked.Add(MaxReviewPhaseSourceProjectionTTLV1).Add(time.Nanosecond).UnixNano()}
	_, err := SealReviewPhaseSourceCurrentProjectionV1(projection, ReviewPhaseSourceCurrentRequestV1{Source: ref}, checked)
	if !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("forged long-TTL projection error=%v", err)
	}
}

func TestReviewPhaseSourceCurrentProjectionV1ClosureExcludesFreshTimeButIncludesOwnerSemantics(t *testing.T) {
	now := time.Unix(1_760_000_200, 0)
	session := reviewPhaseTerminalSessionV1(t, now, "closure")
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(session.Run.Scope)
	run := ReviewRunPhaseSourceRefV1{Run: session.Run, ExecutionScopeDigest: scopeDigest, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Phase: session.Phase, Turn: session.Turn, CompletionClaim: session.CompletionClaim}
	ref, _ := SealReviewPhaseSourceRefV1(ReviewPhaseSourceRefV1{Kind: ReviewPhaseRunSourceV1, Run: &run})
	base := ReviewPhaseSourceCurrentProjectionV1{Source: ref, Run: session.Run, ExecutionScopeDigest: scopeDigest, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Phase: session.Phase, Turn: session.Turn, CompletionClaim: session.CompletionClaim, RunSession: &session, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}
	first, err := base.ClosureDigestV1()
	if err != nil {
		t.Fatal(err)
	}
	fresh := base.Clone()
	fresh.CheckedUnixNano = now.Add(time.Second).UnixNano()
	fresh.ExpiresUnixNano = now.Add(9 * time.Second).UnixNano()
	fresh.Digest = core.DigestBytes([]byte("another-projection"))
	second, _ := fresh.ClosureDigestV1()
	if first != second {
		t.Fatal("fresh time or projection digest changed stable closure")
	}
	mutations := map[string]func(*ReviewPhaseSourceCurrentProjectionV1){
		"source": func(p *ReviewPhaseSourceCurrentProjectionV1) {
			p.Source.Digest = core.DigestBytes([]byte("source-drift"))
		},
		"run": func(p *ReviewPhaseSourceCurrentProjectionV1) { p.Run.RunID = "run-drift" },
		"scope": func(p *ReviewPhaseSourceCurrentProjectionV1) {
			p.ExecutionScopeDigest = core.DigestBytes([]byte("scope-drift"))
		},
		"session-id":       func(p *ReviewPhaseSourceCurrentProjectionV1) { p.SessionID += "-drift" },
		"session-revision": func(p *ReviewPhaseSourceCurrentProjectionV1) { p.SessionRevision++ },
		"session-digest": func(p *ReviewPhaseSourceCurrentProjectionV1) {
			p.SessionDigest = core.DigestBytes([]byte("session-drift"))
		},
		"phase":        func(p *ReviewPhaseSourceCurrentProjectionV1) { p.Phase = SessionWaitingInputV2 },
		"turn":         func(p *ReviewPhaseSourceCurrentProjectionV1) { p.Turn++ },
		"claim":        func(p *ReviewPhaseSourceCurrentProjectionV1) { p.CompletionClaim = ClaimFailed },
		"session-body": func(p *ReviewPhaseSourceCurrentProjectionV1) { p.RunSession.CompletionClaim = ClaimFailed },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			drift := base.Clone()
			mutate(&drift)
			third, _ := drift.ClosureDigestV1()
			if third == first {
				t.Fatal("Owner semantic drift did not change closure")
			}
		})
	}
}

func reviewPhaseTerminalSessionV1(t testing.TB, now time.Time, suffix string) GovernedSessionV4 {
	t.Helper()
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: core.TenantID("tenant-" + suffix), ID: core.AgentIdentityID("agent-" + suffix), Epoch: 1}, Lineage: core.LineageRef{ID: core.InstanceLineageID("lineage-" + suffix), PlanDigest: core.DigestBytes([]byte("plan-" + suffix))}, Instance: core.InstanceRef{ID: core.AgentInstanceID("instance-" + suffix), Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: core.SandboxLeaseID("sandbox-" + suffix), Epoch: 1}, AuthorityEpoch: 1}
	binding := runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-" + suffix, BindingSetRevision: 1, ComponentID: "praxis.harness/test", ManifestDigest: core.DigestBytes([]byte("manifest-" + suffix)), ArtifactDigest: core.DigestBytes([]byte("artifact-" + suffix)), Capability: "praxis.harness/session"}
	endpoint, err := NewEndpointRefV2("endpoint-"+suffix, scope, binding)
	if err != nil {
		t.Fatal(err)
	}
	run := RunRef{Scope: scope, RunID: core.AgentRunID("run-" + suffix)}
	session, err := SealGovernedSessionV4(GovernedSessionV4{ID: "session-" + suffix, Revision: 1, Run: run, Endpoint: endpoint, Phase: SessionTerminalV2, CompletionClaim: ClaimCancelled, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

package application_test

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestContinuityWorkflowGatewayV1SubmitInspectAndLostReply(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	request := continuityWorkflowRequestV1(now, contract.ContinuityTimelineProjectV1)
	assembler := &continuityAssemblerV1{now: now}
	gateway, appStore := continuityGatewayFixtureV1(t, now, request, assembler)
	appStore.LoseNextSubmissionReply = true

	accepted, err := gateway.SubmitContinuityWorkflowV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.RequestDigest == "" || accepted.Submission.Ref != request.RequestID || accepted.Command.Ref != request.RequestID || accepted.Outbox.Ref != request.RequestID || accepted.Journal != nil || accepted.Status != contract.WorkflowAcceptedV2 || len(accepted.Steps) != 1 {
		t.Fatalf("unexpected accepted inspection: %#v", accepted)
	}
	historical := request
	historical.RequestedUnixNano = now.Add(-2 * time.Minute).UnixNano()
	historical.NotAfterUnixNano = now.Add(-time.Minute).UnixNano()
	if _, err := gateway.InspectContinuityWorkflowV1(context.Background(), historical); err == nil || core.HasReason(err, core.ReasonCapabilityExpired) {
		// Historical inspection ignores fresh TTL but still rejects body drift.
		t.Fatalf("changed historical request was accepted or treated as a TTL failure: %v", err)
	}
	exact, err := gateway.InspectContinuityWorkflowV1(context.Background(), request)
	if err != nil || exact.RequestDigest != accepted.RequestDigest || exact.Submission != accepted.Submission || assembler.calls.Load() != 1 {
		t.Fatalf("exact inspection drifted: %#v err=%v", exact, err)
	}
	bundle, err := appStore.InspectSubmissionBundleV2(context.Background(), request.Target, request.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(request.Target)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := contract.NewWorkflowJournalV2("workflow-journal:"+string(scopeDigest)+":"+request.RequestID, bundle.Plan, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appStore.CreateWorkflowJournalV2(context.Background(), bundle.Plan, journal); err != nil {
		t.Fatal(err)
	}
	journaled, err := gateway.InspectContinuityWorkflowV1(context.Background(), request)
	if err != nil || journaled.Journal == nil || journaled.Journal.Ref != journal.ID || journaled.Status != contract.WorkflowAcceptedV2 {
		t.Fatalf("journal inspection missing exact ref: %#v err=%v", journaled, err)
	}
}

func TestContinuityWorkflowGatewayV1AllClosedKindsAndAssemblyDrift(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	kinds := []contract.ContinuityWorkflowKindV1{
		contract.ContinuityTimelineProjectV1,
		contract.ContinuityCheckpointCreateV1,
		contract.ContinuityForkV1,
		contract.ContinuityRewindPlanV1,
		contract.ContinuityRestoreV1,
		contract.ContinuityArtifactAttachV1,
		contract.ContinuityRetentionResolveV1,
	}
	for index, kind := range kinds {
		request := continuityWorkflowRequestV1(now.Add(time.Duration(index)*time.Second), kind)
		gateway, _ := continuityGatewayFixtureV1(t, now.Add(time.Duration(index)*time.Second), request, &continuityAssemblerV1{now: now.Add(time.Duration(index) * time.Second)})
		if _, err := gateway.SubmitContinuityWorkflowV1(context.Background(), request); err != nil {
			t.Fatalf("kind %s: %v", kind, err)
		}
	}
	for _, drift := range []string{"digest", "target", "idempotency", "payload", "root-kind", "root-id"} {
		request := continuityWorkflowRequestV1(now, contract.ContinuityTimelineProjectV1)
		gateway, _ := continuityGatewayFixtureV1(t, now, request, &continuityAssemblerV1{now: now, drift: drift})
		if _, err := gateway.SubmitContinuityWorkflowV1(context.Background(), request); err == nil {
			t.Fatalf("assembly drift %s was accepted", drift)
		}
	}
}

func TestContinuityWorkflowGatewayV1SameIDContentConflictAndConcurrentReplay(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	request := continuityWorkflowRequestV1(now, contract.ContinuityCheckpointCreateV1)
	gateway, _ := continuityGatewayFixtureV1(t, now, request, &continuityAssemblerV1{now: now})
	const workers = 64
	var wait sync.WaitGroup
	wait.Add(workers)
	errors := make(chan error, workers)
	digests := make(chan core.Digest, workers)
	for range workers {
		go func() {
			defer wait.Done()
			result, err := gateway.SubmitContinuityWorkflowV1(context.Background(), request)
			if err == nil {
				digests <- result.RequestDigest
			}
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	close(digests)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent exact replay failed: %v", err)
		}
	}
	var winner core.Digest
	for digest := range digests {
		if winner == "" {
			winner = digest
		}
		if digest != winner {
			t.Fatalf("concurrent replay returned multiple request digests: %q %q", winner, digest)
		}
	}
	changed := request
	changed.DomainRequest.Digest = core.DigestBytes([]byte("changed-domain-request"))
	if _, err := gateway.SubmitContinuityWorkflowV1(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same identity changed content did not conflict: %v", err)
	}
	ownerDrifts := []func(*contract.ExternalOwnerBindingV1){
		func(owner *contract.ExternalOwnerBindingV1) { owner.BindingSetID = "binding-set-2" },
		func(owner *contract.ExternalOwnerBindingV1) { owner.BindingSetRevision++ },
		func(owner *contract.ExternalOwnerBindingV1) { owner.ComponentID = "praxis.tool/owner" },
		func(owner *contract.ExternalOwnerBindingV1) {
			owner.ManifestDigest = core.DigestBytes([]byte("manifest-drift"))
		},
		func(owner *contract.ExternalOwnerBindingV1) {
			owner.ArtifactDigest = core.DigestBytes([]byte("artifact-drift"))
		},
		func(owner *contract.ExternalOwnerBindingV1) { owner.Capability = "praxis.continuity/other-capability" },
		func(owner *contract.ExternalOwnerBindingV1) { owner.FactKind = "praxis.continuity/other-request" },
	}
	for index, drift := range ownerDrifts {
		changed = request
		drift(&changed.DomainRequest.Owner)
		if _, err := gateway.SubmitContinuityWorkflowV1(context.Background(), changed); err == nil {
			t.Fatalf("same identity OwnerBinding drift %d was accepted", index)
		}
	}
	crossScope := request
	crossScope.DomainRequest.ScopeDigest = core.DigestBytes([]byte("another-scope"))
	if _, err := gateway.SubmitContinuityWorkflowV1(context.Background(), crossScope); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("cross-scope domain request was accepted: %v", err)
	}
	crossTenant := request
	crossTenant.DomainRequest.TenantID = "tenant-2"
	if _, err := gateway.SubmitContinuityWorkflowV1(context.Background(), crossTenant); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("cross-tenant domain request was accepted: %v", err)
	}
	exact, err := gateway.InspectContinuityWorkflowV1(context.Background(), request)
	if err != nil || exact.RequestDigest != winner {
		t.Fatalf("changed replay altered the winner: %#v err=%v", exact, err)
	}
}

func TestContinuityWorkflowGatewayV1RejectsTypedNilAndUntrustedRequestFields(t *testing.T) {
	var assembler *continuityAssemblerV1
	if _, err := application.NewContinuityWorkflowGatewayV1(application.ContinuityWorkflowGatewayConfigV1{Assembler: assembler}); err == nil {
		t.Fatal("typed-nil assembler was accepted")
	}
	typeOfRequest := reflect.TypeOf(contract.ContinuityWorkflowRequestV1{})
	for _, forbidden := range []string{"Bundle", "Permit", "Review", "Authorization", "Provider", "Sequence", "Trust", "Current", "Outcome", "Settlement"} {
		if _, ok := typeOfRequest.FieldByName(forbidden); ok {
			t.Fatalf("public request exposes forbidden trusted field %s", forbidden)
		}
	}
	now := time.Unix(1_900_000_000, 0)
	request := continuityWorkflowRequestV1(now, contract.ContinuityTimelineProjectV1)
	gateway, _ := continuityGatewayFixtureV1(t, now, request, &continuityAssemblerV1{now: now})
	if _, err := gateway.SubmitContinuityWorkflowV1(nil, request); err == nil {
		t.Fatal("nil context was accepted")
	}
	request.Kind = "praxis.continuity/not-closed"
	if _, err := gateway.SubmitContinuityWorkflowV1(context.Background(), request); err == nil {
		t.Fatal("unknown workflow kind was accepted")
	}
}

type continuityAssemblerV1 struct {
	now   time.Time
	drift string
	calls atomic.Uint64
}

func (a *continuityAssemblerV1) AssembleContinuityWorkflowV1(_ context.Context, request contract.ContinuityWorkflowRequestV1) (contract.ContinuityWorkflowAssemblyV1, error) {
	a.calls.Add(1)
	body, err := request.CanonicalBodyV1()
	if err != nil {
		return contract.ContinuityWorkflowAssemblyV1{}, err
	}
	digest, err := request.DigestV1()
	if err != nil {
		return contract.ContinuityWorkflowAssemblyV1{}, err
	}
	payload := runtimeports.OpaquePayloadV2{
		Schema:        runtimeports.SchemaRefV2{Namespace: "praxis.application", Name: "continuity-workflow-request", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("continuity-workflow-schema-v1"))},
		ContentDigest: core.DigestBytes(body), Length: uint64(len(body)), Inline: append([]byte(nil), body...),
		LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.application/continuity-request-limit", Digest: core.DigestBytes([]byte("continuity-workflow-limit-v1"))},
	}
	leaseEpoch := request.Target.SandboxLease.Epoch
	command := runtimeports.ApplicationCommandEnvelopeV2{
		ID: request.RequestID, Kind: runtimeports.ApplicationCommandProvideInputV2, Target: request.Target,
		Actor: "application-gateway", AuthorityRef: "authority-current:1", Reason: "continuity workflow",
		CanonicalPayloadDigest: payload.ContentDigest,
		Preconditions:          core.ExecutionPreconditions{IdentityEpoch: request.Target.Identity.Epoch, InstanceEpoch: request.Target.Instance.Epoch, LeaseEpoch: &leaseEpoch, AuthorityEpoch: request.Target.AuthorityEpoch, Revision: 1},
		IdempotencyKey:         request.IdempotencyKey, SubmittedAt: time.Unix(0, request.RequestedUnixNano), ExpiresAt: time.Unix(0, request.NotAfterUnixNano),
	}
	payloadFact := contract.CommandPayloadFactV2{ContractVersion: contract.WorkflowContractVersionV2, CommandID: command.ID, Revision: 1, Payload: payload, CreatedUnixNano: a.now.UnixNano()}
	payloadFactDigest, err := payloadFact.DigestV2()
	if err != nil {
		return contract.ContinuityWorkflowAssemblyV1{}, err
	}
	descriptor := contract.StepDescriptorRefV2{Kind: runtimeports.NamespacedNameV2(request.Kind), Revision: 1, Digest: core.DigestBytes([]byte("continuity-step-descriptor-v1")), ExpiresUnixNano: request.NotAfterUnixNano}
	rootStepID := "continuity-root"
	plan := contract.WorkflowPlanV2{
		ContractVersion: contract.WorkflowContractVersionV2, ID: "workflow:" + request.RequestID, Revision: 1, CommandID: command.ID,
		CommandPayloadDigest: payloadFactDigest, Target: request.Target,
		Authority:       runtimeports.AuthorityBindingRefV2{Ref: command.AuthorityRef, Digest: core.DigestBytes([]byte("authority-current-v1")), Revision: 1, Epoch: request.Target.AuthorityEpoch},
		Steps:           []contract.WorkflowStepV2{{ID: rootStepID, Kind: runtimeports.NamespacedNameV2(request.Kind), Descriptor: descriptor, ExecutionClass: contract.StepCoordinationV2, Required: true, Dependencies: []string{}, Payload: payload}},
		CreatedUnixNano: a.now.UnixNano(), ExpiresUnixNano: request.NotAfterUnixNano,
	}
	assembly := contract.ContinuityWorkflowAssemblyV1{RequestDigest: digest, RootStepID: rootStepID, Bundle: contract.SubmissionBundleV2{Command: command, Payload: payloadFact, Plan: plan}, Mutation: runtimeports.DesiredStateMutationV2{Desired: runtimeports.DesiredRunningV2}}
	switch a.drift {
	case "digest":
		assembly.RequestDigest = core.DigestBytes([]byte("drift"))
	case "target":
		assembly.Bundle.Command.Target.Instance.Epoch++
	case "idempotency":
		assembly.Bundle.Command.IdempotencyKey += "-drift"
	case "payload":
		assembly.Bundle.Payload.Payload.Inline = append([]byte(nil), assembly.Bundle.Payload.Payload.Inline...)
		assembly.Bundle.Payload.Payload.Inline[0] ^= 1
	case "root-kind":
		assembly.Bundle.Plan.Steps[0].Kind = "praxis.continuity/fork"
	case "root-id":
		assembly.RootStepID = "another-root"
	}
	return assembly, nil
}

func continuityGatewayFixtureV1(t *testing.T, now time.Time, request contract.ContinuityWorkflowRequestV1, assembler applicationports.ContinuityWorkflowAssemblerV1) (*application.ContinuityWorkflowGatewayV1, *fakes.FactStoreV2) {
	t.Helper()
	runtimeStore := runtimefakes.NewFactStore(func() time.Time { return now })
	if _, err := runtimeStore.CreateDesiredState(context.Background(), runtimeports.DesiredStateSnapshotV2{Scope: request.Target, Desired: runtimeports.DesiredRunningV2, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	appStore := fakes.NewFactStoreV2()
	appStore.Clock = func() time.Time { return now }
	facade, err := application.NewFacadeV2(application.FacadeConfigV2{Commands: runtimeStore, Submissions: appStore, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := application.NewContinuityWorkflowGatewayV1(application.ContinuityWorkflowGatewayConfigV1{Facade: facade, Assembler: assembler, Submissions: appStore, Commands: runtimeStore, Journals: appStore, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return gateway, appStore
}

func continuityWorkflowRequestV1(now time.Time, kind contract.ContinuityWorkflowKindV1) contract.ContinuityWorkflowRequestV1 {
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: digest("lineage-plan")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "lease-1", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	ref := func(id string) contract.ApplicationFactRefV2 {
		return contract.ApplicationFactRefV2{Ref: id, Revision: 1, Digest: digest(id)}
	}
	return contract.ContinuityWorkflowRequestV1{
		ContractVersion: contract.ContinuityWorkflowContractVersionV1,
		RequestID:       "continuity-request-1", IdempotencyKey: "continuity-idempotency-1", Kind: kind, Target: scope,
		DomainRequest: contract.ExternalFactRefV1{ContractVersion: "praxis.continuity/request/v1", SchemaRef: "praxis.continuity/request/v1", Owner: contract.ExternalOwnerBindingV1{BindingSetID: "binding-set-1", BindingSetRevision: 1, ComponentID: "praxis/continuity", ManifestDigest: digest("manifest"), ArtifactDigest: digest("artifact"), Capability: "praxis.continuity/governed-workflow", FactKind: "praxis.continuity/request"}, TenantID: scope.Identity.TenantID, ScopeDigest: scopeDigest, ID: "domain-request-1", Revision: 1, Digest: digest("domain-request-1")},
		CompiledGraph: ref("compiled-graph-1"), Binding: ref("binding-1"), Consumer: ref("consumer-1"),
		RequestedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(time.Hour).UnixNano(),
	}
}
